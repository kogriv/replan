package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
	"github.com/kogriv/replan/internal/storage/yamlformat"
)

type initResult struct {
	DryRun             bool                `json:"dry_run"`
	Applied            bool                `json:"applied"`
	Project            domain.Project      `json:"project"`
	Owner              domain.Actor        `json:"owner"`
	Resource           domain.Resource     `json:"resource"`
	PlannedWrites      []app.PlannedWrite  `json:"planned_writes"`
	WrittenFiles       []string            `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord  `json:"audit_record_preview"`
	Validation         *app.ValidateResult `json:"validation,omitempty"`
	Warnings           []app.Diagnostic    `json:"warnings"`
	Errors             []app.Diagnostic    `json:"errors"`
}

var initCanonicalPaths = []string{
	".replan/project.yaml", ".replan/policy.yaml", "canonical/actors.yaml",
	"canonical/resources.yaml", "canonical/resource_pools.yaml", "canonical/tasks.yaml",
	"canonical/resource_demands.yaml", "canonical/capacities.yaml", "canonical/availability.yaml",
	"canonical/dependencies.yaml", "canonical/deadlines.yaml", "canonical/current_plan.yaml",
}

func runInit(args []string, stdout, stderr *os.File) int {
	leading := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		leading, args = args[0], args[1:]
	}
	fs := newCLIFlagSet("init")
	name := fs.String("name", "", "required project name")
	ownerName := fs.String("owner", "", "required owner display name")
	id := fs.String("id", "", "optional project id; default project.<directory slug>")
	timezone := fs.String("timezone", "", "optional IANA time zone; required when the machine zone cannot be detected")
	format := fs.String("format", "json", "output format: json, text")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	projectPath := leading
	if projectPath == "" && fs.NArg() > 0 {
		projectPath = fs.Arg(0)
	}
	if *format != "json" && *format != "text" {
		return printInitUsageError("supported formats are json and text", projectPath, *format, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("init", projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	if projectPath == "" || fs.NArg() > 0 {
		return printInitUsageError("init requires exactly one directory argument", projectPath, *format, stdout, stderr)
	}
	if strings.TrimSpace(*name) == "" {
		return printInitUsageError("--name is required", projectPath, *format, stdout, stderr)
	}
	if strings.TrimSpace(*ownerName) == "" {
		return printInitUsageError("--owner is required", projectPath, *format, stdout, stderr)
	}
	zone := strings.TrimSpace(*timezone)
	if zone == "" {
		zone = detectMachineIANATimezone()
	}
	if zone == "" {
		return printInitUsageError("machine IANA time zone is unknown; pass --timezone <IANA zone>", projectPath, *format, stdout, stderr)
	}
	if _, err := time.LoadLocation(zone); err != nil {
		return printInitUsageError(fmt.Sprintf("--timezone %q is not a valid IANA time zone", zone), projectPath, *format, stdout, stderr)
	}
	if collision, err := initCollision(projectPath); err != nil {
		return printInitError(projectPath, *format, "project_check_failed", err.Error(), exitGeneralError, stdout, stderr)
	} else if collision != "" {
		return printInitError(projectPath, *format, "project_exists", fmt.Sprintf("%s already exists; refusing to initialize over a project or canonical file", collision), exitValidationFailed, stdout, stderr)
	}

	absolute, err := filepath.Abs(projectPath)
	if err != nil {
		return printInitError(projectPath, *format, "project_path_invalid", err.Error(), exitInvalidUsage, stdout, stderr)
	}
	projectID := strings.TrimSpace(*id)
	if projectID == "" {
		projectID = "project." + app.Slug(filepath.Base(absolute))
	}
	ownerSlug := app.Slug(*ownerName)
	actorID, resourceID := "actor.human."+ownerSlug, "resource.human."+ownerSlug
	project := domain.Project{ID: projectID, Name: *name, FormatVersion: domain.CurrentProjectFormatVersion, Timezone: zone, PolicyRef: domain.Ref{Kind: "policy", ID: "policy.local-trusted"}}
	owner := domain.Actor{ID: actorID, Kind: "human", DisplayName: *ownerName, TrustLevel: "owner", ResourceRef: domain.Ref{Kind: "resource", ID: resourceID}}
	resource := domain.Resource{ID: resourceID, Class: "human", Name: *ownerName, ActorRef: domain.Ref{Kind: "actor", ID: actorID}}
	occurredAt := time.Now().Format(time.RFC3339)
	audit := domain.AuditRecord{ID: "audit." + initCompactTimestamp(occurredAt) + ".project-init." + app.Slug(projectID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: actorID}, Operation: "project_init", TargetRef: domain.Ref{Kind: "project", ID: projectID}, Result: "success", Reason: "Initialized project."}
	if dryRun {
		audit.Result, audit.Reason = "dry_run", "Preview project initialization."
	}
	writes, err := initPlannedWrites(project, owner, resource, audit)
	if err != nil {
		return printInitError(projectPath, *format, "project_render_failed", err.Error(), exitGeneralError, stdout, stderr)
	}
	result := initResult{DryRun: dryRun, Project: project, Owner: owner, Resource: resource, PlannedWrites: writes, AuditRecordPreview: audit, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{}}
	if dryRun {
		return printInitResult(stdout, stderr, *format, projectPath, result, nil)
	}
	return applyInitProject(projectPath, *format, writes, result, stdout, stderr)
}

func applyInitProject(projectPath, format string, writes []app.PlannedWrite, result initResult, stdout, stderr *os.File) int {
	createdDirectory := false
	if _, statErr := os.Stat(projectPath); errors.Is(statErr, os.ErrNotExist) {
		if err := os.MkdirAll(projectPath, 0o755); err != nil {
			return printInitError(projectPath, format, "project_create_failed", err.Error(), exitGeneralError, stdout, stderr)
		}
		createdDirectory = true
	}
	metadataPath := filepath.Join(projectPath, ".replan")
	if err := os.Mkdir(metadataPath, 0o755); err != nil {
		if errors.Is(err, os.ErrExist) {
			return printInitError(projectPath, format, "project_exists", fmt.Sprintf("%s already exists", metadataPath), exitValidationFailed, stdout, stderr)
		}
		return printInitError(projectPath, format, "project_create_failed", err.Error(), exitGeneralError, stdout, stderr)
	}
	lock, err := acquireProjectWriteLock(projectPath, projectWriteLockTimeout())
	if err != nil {
		return printProjectLocked(stdout, stderr, projectWriteIntent{write: true, command: "init", projectPath: projectPath, format: format}, err)
	}
	if lock != nil {
		defer lock.close()
	}
	validation, written, diagnostics, err := app.CreateProjectFiles(context.Background(), filestore.New(projectPath), writes)
	result.Validation, result.WrittenFiles = &validation, written
	if err != nil {
		result.Errors = append(result.Errors, diagnostics...)
		if len(written) == 0 {
			_ = os.Remove(filepath.Join(metadataPath, "write.lock"))
			_ = os.Remove(metadataPath)
			if createdDirectory {
				_ = os.Remove(projectPath)
			}
		}
		return printInitResult(stdout, stderr, format, projectPath, result, err)
	}
	result.Applied = true
	return printInitResult(stdout, stderr, format, projectPath, result, nil)
}

func initPlannedWrites(project domain.Project, owner domain.Actor, resource domain.Resource, audit domain.AuditRecord) ([]app.PlannedWrite, error) {
	projectYAML, err := yamlformat.Render(struct {
		Project domain.Project `yaml:"project"`
	}{project})
	if err != nil {
		return nil, fmt.Errorf("render project metadata: %w", err)
	}
	policyYAML := fmt.Sprintf("policy:\n  id: policy.local-trusted\n  mode: local_trusted\n  default_actor: %s\n  require_dry_run_for:\n    - write_back_to_source\n    - apply_scenario\n  ai_agents:\n    default_mutation_mode: proposal_only\n", owner.ID)
	actorsYAML, err := yamlformat.Render(struct {
		Actors []domain.Actor `yaml:"actors"`
	}{[]domain.Actor{owner}})
	if err != nil {
		return nil, fmt.Errorf("render owner actor: %w", err)
	}
	resourcesYAML, err := yamlformat.Render(struct {
		Resources []domain.Resource `yaml:"resources"`
	}{[]domain.Resource{resource}})
	if err != nil {
		return nil, fmt.Errorf("render owner resource: %w", err)
	}
	planYAML, err := yamlformat.Render(struct {
		CurrentPlan domain.CurrentPlan `yaml:"current_plan"`
	}{domain.CurrentPlan{ID: "current", Version: 1, UpdatedAt: audit.OccurredAt, UpdatedBy: domain.Ref{Kind: "actor", ID: owner.ID}, IncludedTasks: []domain.Ref{}}})
	if err != nil {
		return nil, fmt.Errorf("render current plan: %w", err)
	}
	contents := map[string]string{
		".replan/project.yaml": string(projectYAML), ".replan/policy.yaml": policyYAML,
		"canonical/actors.yaml": string(actorsYAML), "canonical/resources.yaml": string(resourcesYAML),
		"canonical/resource_pools.yaml": "resource_pools: []\n", "canonical/tasks.yaml": "tasks: []\n",
		"canonical/resource_demands.yaml": "resource_demands: []\n", "canonical/capacities.yaml": "capacities: []\n",
		"canonical/availability.yaml": "availability: []\n", "canonical/dependencies.yaml": "dependencies: []\n",
		"canonical/deadlines.yaml": "deadlines: []\n", "canonical/current_plan.yaml": string(planYAML),
		app.AuditShardPath(audit.OccurredAt): app.AuditJSONLine(audit),
	}
	paths := make([]string, 0, len(contents))
	for path := range contents {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	writes := make([]app.PlannedWrite, 0, len(paths))
	for _, path := range paths {
		format := "yaml"
		if strings.HasSuffix(path, ".jsonl") {
			format = "jsonl"
		}
		writes = append(writes, app.PlannedWrite{Path: path, Action: "create", Format: format, Content: contents[path]})
	}
	return writes, nil
}

func initCollision(projectPath string) (string, error) {
	metadata := filepath.Join(projectPath, ".replan")
	if _, err := os.Stat(metadata); err == nil {
		return metadata, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, rel := range initCanonicalPaths[2:] {
		path := filepath.Join(projectPath, filepath.FromSlash(rel))
		if _, err := os.Stat(path); err == nil {
			return path, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return "", nil
}

func machineIANATimezone() string {
	if zone := strings.TrimPrefix(strings.TrimSpace(os.Getenv("TZ")), ":"); zone != "" && !filepath.IsAbs(zone) {
		if _, err := time.LoadLocation(zone); err == nil {
			return zone
		}
	}
	target, err := filepath.EvalSymlinks("/etc/localtime")
	if err != nil {
		return ""
	}
	const marker = "/zoneinfo/"
	index := strings.Index(filepath.ToSlash(target), marker)
	if index < 0 {
		return ""
	}
	zone := filepath.ToSlash(target)[index+len(marker):]
	if _, err := time.LoadLocation(zone); err != nil {
		return ""
	}
	return zone
}

var detectMachineIANATimezone = machineIANATimezone

func initCompactTimestamp(value string) string {
	replacer := strings.NewReplacer("-", "", ":", "", "+", "", ".", "")
	return replacer.Replace(value)
}

func printInitUsageError(message, projectPath, format string, stdout, stderr *os.File) int {
	return printInitError(projectPath, format, "invalid_usage", message, exitInvalidUsage, stdout, stderr)
}
func printInitError(projectPath, format, code, message string, exit int, stdout, stderr *os.File) int {
	diagnostic := app.Diagnostic{Code: code, Message: message, Severity: "error", Target: app.Target{Kind: "project", ID: projectPath}}
	if format == "json" {
		_ = printEnvelope(stdout, "json", envelope{OK: false, Command: "init", ProjectPath: projectPath, Result: nil, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{diagnostic}})
	} else {
		fmt.Fprintf(stderr, "%s: %s\n", code, message)
	}
	return exit
}
func printInitResult(stdout, stderr *os.File, format, projectPath string, result initResult, err error) int {
	if format == "text" && err == nil {
		mode := "Dry run"
		if result.Applied {
			mode = "Applied"
		}
		fmt.Fprintf(stdout, "%s: initialize %s\n", mode, projectPath)
		for _, write := range result.PlannedWrites {
			fmt.Fprintf(stdout, "- %s\n", write.Path)
		}
		return exitSuccess
	}
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "init", ProjectPath: projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	return printScenarioEnvelope(stdout, stderr, format, env, err)
}

func runResource(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing resource subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "add":
		return runResourceAdd(args[1:], stdout, stderr)
	case "list":
		return runResourceList(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown resource subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runResourceAdd(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("resource add")
	project := fs.String("project", ".", "path to RePlan project")
	id := fs.String("id", "", "required resource id")
	class := fs.String("class", "", "required resource class")
	name := fs.String("name", "", "required resource name")
	var capabilities repeatedStringFlag
	fs.Var(&capabilities, "capability", "optional repeatable capability")
	withActor := fs.String("with-actor", "", "optional trust level for a matching human or ai_agent actor")
	actor := fs.String("actor", "", "required existing audit actor id")
	evidence := fs.String("evidence", "", "optional existing evidence id")
	format := fs.String("format", "json", "output format: json, text")
	flags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *format != "json" && *format != "text" {
		return printTaskFormatError("resource add", *project, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("resource add", *project, *format, flags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewResourceService(newStore(*project)).Add(context.Background(), app.ResourceAddRequest{ID: *id, Class: *class, Name: *name, Capabilities: capabilities, WithActor: *withActor, ActorID: *actor, EvidenceID: *evidence, DryRun: dryRun})
	if *format == "text" && err == nil {
		mode := "Dry run"
		if result.Applied {
			mode = "Applied"
		}
		fmt.Fprintf(stdout, "%s: resource %s (%s)\n", mode, result.Resource.ID, result.Resource.Class)
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, envelope{OK: err == nil, Command: "resource add", ProjectPath: *project, Result: result, Warnings: result.Warnings, Errors: result.Errors}, err)
}

func runResourceList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("resource list")
	project := fs.String("project", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, markdown, text")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewResourceService(newStore(*project)).List(context.Background())
	if *format != "json" && *format != "markdown" && *format != "text" {
		return printTaskFormatError("resource list", *project, stdout, stderr)
	}
	if err == nil && *format != "json" {
		return printResourceList(stdout, *format, result)
	}
	return printScenarioEnvelope(stdout, stderr, *format, envelope{OK: err == nil, Command: "resource list", ProjectPath: *project, Result: result, Warnings: result.Warnings, Errors: result.Errors}, err)
}

func printResourceList(out *os.File, format string, result app.ResourceListResult) int {
	if format == "markdown" {
		fmt.Fprintln(out, "# Resources\n\n| ID | Class | Name | Capabilities | Actor |\n|---|---|---|---|---|")
		for _, resource := range result.Resources {
			fmt.Fprintf(out, "| `%s` | %s | %s | %s | `%s` |\n", resource.ID, resource.Class, resource.Name, strings.Join(resource.Capabilities, ", "), resource.ActorRef.ID)
		}
		return exitSuccess
	}
	tw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tCLASS\tNAME\tCAPABILITIES\tACTOR")
	for _, resource := range result.Resources {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", resource.ID, resource.Class, resource.Name, strings.Join(resource.Capabilities, ","), resource.ActorRef.ID)
	}
	_ = tw.Flush()
	return exitSuccess
}

func runCapacity(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing capacity subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	if args[0] == "set" {
		return runCapacitySet(args[1:], stdout, stderr)
	}
	if args[0] == "-h" || args[0] == "--help" {
		printUsage(stdout)
		return exitSuccess
	}
	fmt.Fprintf(stderr, "unknown capacity subcommand: %s\n", args[0])
	printUsage(stderr)
	return exitInvalidUsage
}

func runCapacitySet(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("capacity set")
	project := fs.String("project", ".", "path to RePlan project")
	resource := fs.String("resource", "", "required existing resource id")
	unit := fs.String("unit", "", "required capacity unit")
	amount := fs.String("amount", "", "required capacity amount")
	period := fs.String("period", "week", "capacity period (week)")
	effectiveFrom := fs.String("effective-from", "", "optional effective date (YYYY-MM-DD); defaults to project status date or today")
	actor := fs.String("actor", "", "required existing audit actor id")
	evidence := fs.String("evidence", "", "optional existing evidence id")
	format := fs.String("format", "json", "output format: json, text")
	flags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *format != "json" && *format != "text" {
		return printTaskFormatError("capacity set", *project, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("capacity set", *project, *format, flags, stdout, stderr)
	if done {
		return code
	}
	parsedAmount, parseErr := strconv.ParseFloat(*amount, 64)
	if *amount == "" || parseErr != nil {
		diagnostic := app.Diagnostic{Code: "invalid_capacity_amount", Message: "--amount must be a number", Severity: "error", Target: app.Target{Kind: "capacity"}}
		return printScenarioEnvelope(stdout, stderr, *format, envelope{OK: false, Command: "capacity set", ProjectPath: *project, Result: nil, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{diagnostic}}, app.ErrValidationFailed)
	}
	result, err := app.NewResourceService(newStore(*project)).SetCapacity(context.Background(), app.CapacitySetRequest{ResourceID: *resource, Unit: *unit, Amount: parsedAmount, Period: *period, EffectiveFrom: *effectiveFrom, ActorID: *actor, EvidenceID: *evidence, DryRun: dryRun})
	if *format == "text" && err == nil {
		mode := "Dry run"
		if result.Applied {
			mode = "Applied"
		}
		fmt.Fprintf(stdout, "%s: capacity %s %g %s/%s from %s\n", mode, result.Capacity.ResourceRef.ID, result.Capacity.Amount, result.Capacity.Unit, result.Capacity.Period, result.Capacity.EffectiveFrom)
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, envelope{OK: err == nil, Command: "capacity set", ProjectPath: *project, Result: result, Warnings: result.Warnings, Errors: result.Errors}, err)
}
