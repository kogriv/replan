package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/buildinfo"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
	"github.com/kogriv/replan/internal/storage/filestore"
	"gopkg.in/yaml.v3"
)

const (
	exitSuccess          = 0
	exitGeneralError     = 1
	exitValidationFailed = 2
	exitPolicyDenied     = 3
	exitNotFound         = 4
	exitConflict         = 5
	exitExternalSource   = 6
	exitInvalidUsage     = 8
	bindingsOutputCode   = "bindings_layer_output_in_project"
)

type envelope struct {
	OK          bool              `json:"ok"`
	Command     string            `json:"command"`
	ProjectPath string            `json:"project_path,omitempty"`
	Result      any               `json:"result"`
	Warnings    []app.Diagnostic  `json:"warnings"`
	Errors      []app.Diagnostic  `json:"errors"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

type weeklyReportWriteResult struct {
	Report     reports.WeeklyReport `json:"report"`
	OutputPath string               `json:"output_path"`
}

type dashboardReportWriteResult struct {
	Overview     reports.DashboardOverview `json:"overview"`
	OutputDir    string                    `json:"output_dir"`
	Artifacts    []string                  `json:"artifacts"`
	ManifestPath string                    `json:"manifest_path"`
}

type ExportManifest struct {
	ID          string           `json:"id"`
	Command     string           `json:"command"`
	Format      string           `json:"format"`
	Report      string           `json:"report,omitempty"`
	ProjectPath string           `json:"project_path"`
	OutputPath  string           `json:"output_path"`
	ScenarioID  string           `json:"scenario_id,omitempty"`
	Week        string           `json:"week,omitempty"`
	SHA256      string           `json:"sha256"`
	CreatedAt   string           `json:"created_at"`
	Warnings    []app.Diagnostic `json:"warnings"`
	Errors      []app.Diagnostic `json:"errors"`
}

type TaskJugglerValidationResult struct {
	ProjectPath     string `json:"project_path"`
	ScenarioID      string `json:"scenario_id,omitempty"`
	TaskJugglerPath string `json:"taskjuggler_path,omitempty"`
	ArtifactPath    string `json:"artifact_path,omitempty"`
	Skipped         bool   `json:"skipped"`
	Validated       bool   `json:"validated"`
	Output          string `json:"output,omitempty"`
}

type TaskJugglerRunResult struct {
	ID                     string   `json:"id"`
	ProjectPath            string   `json:"project_path"`
	ScenarioID             string   `json:"scenario_id,omitempty"`
	OutputDir              string   `json:"output_dir"`
	TaskJugglerPath        string   `json:"taskjuggler_path,omitempty"`
	TaskJugglerVersion     string   `json:"taskjuggler_version,omitempty"`
	ProjectTJPPath         string   `json:"project_tjp_path,omitempty"`
	ManifestPath           string   `json:"manifest_path,omitempty"`
	StdoutPath             string   `json:"stdout_path,omitempty"`
	StderrPath             string   `json:"stderr_path,omitempty"`
	VersionPath            string   `json:"version_path,omitempty"`
	MappingPath            string   `json:"mapping_path,omitempty"`
	TaskScheduleReportPath string   `json:"task_schedule_report_path,omitempty"`
	ExitCode               int      `json:"exit_code"`
	Artifacts              []string `json:"artifacts"`
	CanonicalStateMutated  bool     `json:"canonical_state_mutated"`
}

type TaskJugglerRunReportArtifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type TaskJugglerRunManifest struct {
	ID                       string                         `json:"id"`
	Command                  string                         `json:"command"`
	ProjectPath              string                         `json:"project_path"`
	ScenarioID               string                         `json:"scenario_id,omitempty"`
	OutputDir                string                         `json:"output_dir"`
	TaskJugglerPath          string                         `json:"taskjuggler_path"`
	TaskJugglerVersion       string                         `json:"taskjuggler_version,omitempty"`
	TaskJugglerExitCode      int                            `json:"taskjuggler_exit_code"`
	UnrepresentedAssumptions string                         `json:"unrepresented_assumptions"`
	ProjectTJPPath           string                         `json:"project_tjp_path"`
	ProjectTJPSHA256         string                         `json:"project_tjp_sha256"`
	StdoutPath               string                         `json:"stdout_path"`
	StderrPath               string                         `json:"stderr_path"`
	VersionPath              string                         `json:"version_path"`
	MappingPath              string                         `json:"mapping_path,omitempty"`
	MappingSHA256            string                         `json:"mapping_sha256,omitempty"`
	Reports                  []TaskJugglerRunReportArtifact `json:"reports"`
	CreatedAt                string                         `json:"created_at"`
	CanonicalStateMutated    bool                           `json:"canonical_state_mutated"`
	Warnings                 []app.Diagnostic               `json:"warnings"`
	Errors                   []app.Diagnostic               `json:"errors"`
}

type TaskJugglerImportResult struct {
	RunDir           string                       `json:"run_dir"`
	ManifestPath     string                       `json:"manifest_path"`
	ReportPath       string                       `json:"report_path"`
	MappingPath      string                       `json:"mapping_path"`
	RowsRead         int                          `json:"rows_read"`
	CandidateChanges int                          `json:"candidate_changes"`
	Candidate        TaskJugglerScenarioCandidate `json:"candidate"`
	ScenarioPreview  domain.Scenario              `json:"scenario_preview,omitempty"`
	ProposalPreview  domain.Proposal              `json:"proposal_preview,omitempty"`
	PlannedWrites    []app.PlannedWrite           `json:"planned_writes"`
	WrittenFiles     []string                     `json:"written_files,omitempty"`
	PreviewOnly      bool                         `json:"preview_only"`
	Applied          bool                         `json:"applied"`
	Warnings         []app.Diagnostic             `json:"warnings"`
	Errors           []app.Diagnostic             `json:"errors"`
}

type TaskJugglerScenarioCandidate struct {
	ID          string                       `json:"id"`
	Source      string                       `json:"source"`
	SourceRunID string                       `json:"source_run_id"`
	Changes     []TaskJugglerCandidateChange `json:"changes"`
}

type TaskJugglerCandidateChange struct {
	Op            string `json:"op"`
	Path          string `json:"path"`
	Value         any    `json:"value,omitempty"`
	PreviousValue any    `json:"previous_value,omitempty"`
}

type repeatedStringFlag []string

func (values *repeatedStringFlag) String() string { return strings.Join(*values, ",") }

func (values *repeatedStringFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func main() {
	code := run(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}

func run(args []string, stdout, stderr *os.File) (code int) {
	originalBindings, hadBindings := os.LookupEnv("REPLAN_OPENPROJECT_BINDINGS_LAYER")
	defer func() {
		if hadBindings {
			_ = os.Setenv("REPLAN_OPENPROJECT_BINDINGS_LAYER", originalBindings)
		} else {
			_ = os.Unsetenv("REPLAN_OPENPROJECT_BINDINGS_LAYER")
		}
	}()
	intent := projectWriteIntentForArgs(args)
	if intent.write && !intent.bindingsLayer {
		lock, err := acquireProjectWriteLock(intent.projectPath, projectWriteLockTimeout())
		if err != nil {
			return printProjectLocked(stdout, stderr, intent, err)
		}
		if lock != nil {
			defer lock.close()
		}
	}
	code = runCommand(args, stdout, stderr)
	if code != exitSuccess {
		if path := os.Getenv("REPLAN_OPENPROJECT_BINDINGS_LAYER"); path != "" && filestore.OpenProjectBindingsError(path) != nil {
			return exitInvalidUsage
		}
	}
	return code
}

func runCommand(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		printUsage(stderr)
		return exitInvalidUsage
	}

	switch args[0] {
	case "init":
		return runInit(args[1:], stdout, stderr)
	case "version":
		return runVersion(args[1:], stdout, stderr, true)
	case "--version":
		return runVersion(args[1:], stdout, stderr, false)
	case "validate":
		return runValidate(args[1:], stdout, stderr)
	case "workload":
		return runWorkload(args[1:], stdout, stderr)
	case "report":
		return runReport(args[1:], stdout, stderr)
	case "next":
		return runNext(args[1:], stdout, stderr)
	case "project":
		return runProject(args[1:], stdout, stderr)
	case "tracking":
		return runTracking(args[1:], stdout, stderr)
	case "scenario":
		return runScenario(args[1:], stdout, stderr)
	case "proposal":
		return runProposal(args[1:], stdout, stderr)
	case "dependency":
		return runDependency(args[1:], stdout, stderr)
	case "baseline":
		return runBaseline(args[1:], stdout, stderr)
	case "estimate":
		return runEstimate(args[1:], stdout, stderr)
	case "audit":
		return runAudit(args[1:], stdout, stderr)
	case "actual":
		return runActual(args[1:], stdout, stderr)
	case "evidence":
		return runEvidence(args[1:], stdout, stderr)
	case "task":
		return runTask(args[1:], stdout, stderr)
	case "resource":
		return runResource(args[1:], stdout, stderr)
	case "capacity":
		return runCapacity(args[1:], stdout, stderr)
	case "import":
		return runImport(args[1:], stdout, stderr)
	case "export":
		return runExport(args[1:], stdout, stderr)
	case "taskjuggler":
		return runTaskJuggler(args[1:], stdout, stderr)
	case "dev":
		return runDev(args[1:], stdout, stderr)
	case "help", "--help", "-h":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown command: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

type versionResult struct {
	buildinfo.Info
	ProjectFormatVersion int `json:"project_format_version"`
}

func runVersion(args []string, stdout, stderr *os.File, includeFormatLine bool) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	format := fs.String("format", "text", "output format: text, json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "version takes no arguments")
		return exitInvalidUsage
	}
	result := versionResult{Info: buildinfo.Current(), ProjectFormatVersion: domain.CurrentProjectFormatVersion}
	switch strings.ToLower(*format) {
	case "json":
		if !includeFormatLine {
			fmt.Fprintln(stderr, "--version only supports text output")
			return exitInvalidUsage
		}
		env := envelope{OK: true, Command: "version", Result: result, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{}}
		if err := printEnvelope(stdout, "json", env); err != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", err)
			return exitGeneralError
		}
	case "text":
		fmt.Fprintln(stdout, versionTextLine(result.Info))
		if includeFormatLine {
			fmt.Fprintf(stdout, "project format version: %d\n", result.ProjectFormatVersion)
		}
	default:
		fmt.Fprintln(stderr, "supported version formats are text and json")
		return exitInvalidUsage
	}
	return exitSuccess
}

func versionTextLine(info buildinfo.Info) string {
	parts := make([]string, 0, 3)
	if commit := info.ShortCommit(); commit != "" {
		parts = append(parts, commit)
	}
	if date := info.Date(); date != "" {
		parts = append(parts, date)
	}
	parts = append(parts, info.GoVersion+" "+info.OSArch)
	return fmt.Sprintf("replan %s (%s)", info.Version, strings.Join(parts, ", "))
}

func newCLIFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Func("bindings", "path to a read-only OpenProject private bindings layer (overrides REPLAN_OPENPROJECT_BINDINGS_LAYER)", func(value string) error {
		return os.Setenv("REPLAN_OPENPROJECT_BINDINGS_LAYER", value)
	})
	return fs
}

func refuseBindingsOutputInsideProject(command, projectPath, outputPath string, stdout, stderr *os.File) (int, bool) {
	if !bindingsLayerActive() || !pathInsideProject(projectPath, outputPath) {
		return exitSuccess, false
	}
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Warnings:    []app.Diagnostic{},
		Errors: []app.Diagnostic{{
			Code:     bindingsOutputCode,
			Message:  "the read-only OpenProject bindings layer cannot write output inside the project; choose an output path outside the project, drop --bindings, or unset REPLAN_OPENPROJECT_BINDINGS_LAYER",
			Severity: "error",
		}},
	}
	if err := printEnvelope(stdout, "json", env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitInvalidUsage, true
}

func refuseScenarioReportWrite(command, projectPath, scenarioID, outputPath string, stdout, stderr *os.File) (int, bool) {
	if scenarioID == "" || !pathInsideProject(projectPath, outputPath) {
		return exitSuccess, false
	}
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Warnings:    []app.Diagnostic{},
		Errors: []app.Diagnostic{{
			Code:     "scenario_report_write",
			Message:  "a scenario report is a what-if and cannot be written inside the project as the plan; choose an output path outside the project",
			Severity: "error",
			Target:   app.Target{Kind: "scenario", ID: scenarioID},
		}},
	}
	if err := printEnvelope(stdout, "json", env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitInvalidUsage, true
}

// refuseBindingsExportInsideProject checks export files, whose relative paths
// resolve against the working directory rather than the project.
func refuseBindingsExportInsideProject(command, projectPath string, stdout, stderr *os.File, paths ...string) (int, bool) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			absolute = path
		}
		if code, done := refuseBindingsOutputInsideProject(command, projectPath, absolute, stdout, stderr); done {
			return code, true
		}
	}
	return exitSuccess, false
}

func pathInsideProject(projectPath, outputPath string) bool {
	projectAbs, err := filepath.Abs(projectPath)
	if err != nil {
		return false
	}
	outputAbs := outputPath
	if !filepath.IsAbs(outputAbs) {
		outputAbs = filepath.Join(projectAbs, outputAbs)
	}
	outputAbs, err = filepath.Abs(outputAbs)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(projectAbs, outputAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// parseCommandFlags keeps flag package output on the channel that matches the
// result: help on stdout with exit 0, every other parse error on stderr with
// exit 8. Buffering avoids flag.Parse writing help to stderr before it returns
// flag.ErrHelp.
func parseCommandFlags(fs *flag.FlagSet, args []string, stdout, stderr *os.File) (int, bool) {
	var output bytes.Buffer
	fs.SetOutput(&output)
	err := fs.Parse(args)
	if err == nil {
		return exitSuccess, false
	}
	target := stderr
	code := exitInvalidUsage
	if errors.Is(err, flag.ErrHelp) {
		target = stdout
		code = exitSuccess
	}
	_, _ = target.Write(output.Bytes())
	return code, true
}

func runDependency(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing dependency subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runDependencyList(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown dependency subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runBaseline(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing baseline subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "create":
		return runBaselineCreate(args[1:], stdout, stderr)
	case "list":
		return runBaselineList(args[1:], stdout, stderr)
	case "show":
		return runBaselineShow(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown baseline subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runBaselineCreate(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("baseline create")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	id := fs.String("id", "", "required baseline id")
	name := fs.String("name", "", "required baseline name")
	approvedBy := fs.String("approved-by", "", "required existing approving actor id")
	statusDate := fs.String("status-date", "", "optional status date (YYYY-MM-DD)")
	var assumptions repeatedStringFlag
	fs.Var(&assumptions, "assumption", "optional repeatable baseline assumption")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	formatValue := strings.ToLower(*format)
	if formatValue != "json" && formatValue != "text" {
		return printTaskFormatError("baseline create", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("baseline create", *projectPath, formatValue, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewBaselineService(newStore(*projectPath)).Create(context.Background(), app.BaselineCreateRequest{
		ID: *id, Name: *name, ApprovedBy: *approvedBy, StatusDate: *statusDate,
		Assumptions: assumptions, OccurredAt: *occurredAt, DryRun: dryRun,
	})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "baseline create", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if formatValue == "text" && err == nil {
		if printErr := writeBaselineCreateText(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, formatValue, env, err)
}

func runBaselineList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("baseline list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewBaselineService(newStore(*projectPath)).List(context.Background())
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "baseline list", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "markdown") || strings.EqualFold(*format, "text") {
		if printErr := writeBaselineListMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runBaselineShow(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("baseline show")
	projectPath, format := addProjectFormatFlags(fs, "json")
	leadingID, filteredArgs := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, filteredArgs, stdout, stderr); done {
		return code
	}
	id := positionalArg(leadingID, fs)
	result, err := app.NewBaselineService(newStore(*projectPath)).Show(context.Background(), id)
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "baseline show", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "markdown") || strings.EqualFold(*format, "text") {
		if printErr := writeBaselineShowMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runTracking(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing tracking subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "suggest":
		return runTrackingSuggest(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown tracking subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runTrackingSuggest(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("tracking suggest")
	projectPath, format := addProjectFormatFlags(fs, "json")
	week := fs.String("week", "", "ISO week, for example 2026-W31")
	actorID := fs.String("actor", "", "actor id creating proposal candidates")
	writeCandidates := fs.Bool("write-candidates", false, "write supported tracking suggestions as reviewable proposal candidate files")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	formatValue := strings.ToLower(*format)
	if formatValue != "json" && formatValue != "markdown" && formatValue != "text" {
		env := envelope{
			OK:          false,
			Command:     "tracking suggest",
			ProjectPath: *projectPath,
			Result:      nil,
			Warnings:    []app.Diagnostic{},
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}

	if *week != "" && !validISOWeek(*week) {
		return printInvalidWeek(stdout, "tracking suggest", *projectPath, *week)
	}

	if *writeCandidates {
		if code, done := refuseBindingsLayerWrite("tracking suggest", *projectPath, formatValue, stdout, stderr); done {
			return code
		}
	}
	result, err := app.NewTrackingService(newStore(*projectPath)).Suggest(context.Background(), app.TrackingSuggestRequest{
		ProjectPath:     *projectPath,
		Week:            *week,
		ActorID:         *actorID,
		WriteCandidates: *writeCandidates,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "tracking suggest",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if code, done := printProjectNotFound(stdout, stderr, formatValue, env, err); done {
		return code
	}
	var printErr error
	switch formatValue {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "markdown", "text":
		printErr = writeTrackingSuggestMarkdown(stdout, result)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runDependencyList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("dependency list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewDependencyService(newStore(*projectPath)).List(context.Background(), app.DependencyListRequest{ProjectPath: *projectPath})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "dependency list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeDependencyListMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runDev(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing dev subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "validate-taskjuggler":
		return runDevValidateTaskJuggler(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown dev subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runExport(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing export subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "json":
		return runExportJSON(args[1:], stdout, stderr)
	case "taskjuggler":
		return runExportTaskJuggler(args[1:], stdout, stderr)
	case "csv":
		return runExportCSV(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown export subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runTaskJuggler(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing taskjuggler subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "run":
		return runTaskJugglerRun(args[1:], stdout, stderr)
	case "import-result":
		return runTaskJugglerImportResult(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown taskjuggler subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runDevValidateTaskJuggler(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("dev validate-taskjuggler")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	format := fs.String("format", "json", "output format: json, markdown or text")
	taskJugglerBinary := fs.String("taskjuggler-bin", "tj3", "TaskJuggler binary to run")
	unrepresentedAssumptions := fs.String("unrepresented-assumptions", "warn", "how to handle scenario assumptions not represented in TaskJuggler .tjp: warn or error")
	keepTemp := fs.Bool("keep-temp", false, "keep generated temporary .tjp artifact")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	*format = strings.ToLower(strings.TrimSpace(*format))
	if *format == "" {
		*format = "json"
	}
	if *format != "json" && *format != "markdown" && *format != "text" {
		env := envelope{
			OK:          false,
			Command:     "dev validate-taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	*unrepresentedAssumptions = strings.ToLower(strings.TrimSpace(*unrepresentedAssumptions))
	if *unrepresentedAssumptions == "" {
		*unrepresentedAssumptions = "warn"
	}
	if *unrepresentedAssumptions != "warn" && *unrepresentedAssumptions != "error" {
		env := envelope{
			OK:          false,
			Command:     "dev validate-taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_unrepresented_assumptions_mode",
				Message:  "supported --unrepresented-assumptions modes are warn and error",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}

	result, warnings, errorsOut, err := runTaskJugglerValidation(context.Background(), taskJugglerValidationRequest{
		ProjectPath:                  *projectPath,
		ScenarioID:                   *scenarioID,
		TaskJugglerBinary:            *taskJugglerBinary,
		UnrepresentedAssumptionsMode: *unrepresentedAssumptions,
		KeepTemp:                     *keepTemp,
	})
	env := envelope{
		OK:          err == nil,
		Command:     "dev validate-taskjuggler",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    warnings,
		Errors:      errorsOut,
	}
	if printErr := printEnvelope(stdout, *format, env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err == nil {
		return exitSuccess
	}
	if errors.Is(err, app.ErrValidationFailed) {
		return validationFailureExit(errorsOut)
	}
	return exitGeneralError
}

type taskJugglerValidationRequest struct {
	ProjectPath                  string
	ScenarioID                   string
	TaskJugglerBinary            string
	UnrepresentedAssumptionsMode string
	KeepTemp                     bool
}

type taskJugglerRunRequest struct {
	ProjectPath                  string
	ScenarioID                   string
	OutputDir                    string
	TaskJugglerBinary            string
	UnrepresentedAssumptionsMode string
	Now                          time.Time
}

func runTaskJugglerRun(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("taskjuggler run")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	outputDir := fs.String("output-dir", "", "directory for TaskJuggler run artifacts")
	format := fs.String("format", "json", "output format: json, markdown or text")
	taskJugglerBinary := fs.String("taskjuggler-bin", "tj3", "TaskJuggler binary to run")
	unrepresentedAssumptions := fs.String("unrepresented-assumptions", "warn", "how to handle scenario assumptions not represented in TaskJuggler .tjp: warn or error")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	*format = strings.ToLower(strings.TrimSpace(*format))
	if *format == "" {
		*format = "json"
	}
	if *format != "json" && *format != "markdown" && *format != "text" {
		env := envelope{
			OK:          false,
			Command:     "taskjuggler run",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	*unrepresentedAssumptions = strings.ToLower(strings.TrimSpace(*unrepresentedAssumptions))
	if *unrepresentedAssumptions == "" {
		*unrepresentedAssumptions = "warn"
	}
	if *unrepresentedAssumptions != "warn" && *unrepresentedAssumptions != "error" {
		env := envelope{
			OK:          false,
			Command:     "taskjuggler run",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_unrepresented_assumptions_mode",
				Message:  "supported --unrepresented-assumptions modes are warn and error",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	// The default run directory is inside the project; an explicit one is
	// relative to the working directory.
	requestedOutputDir := *projectPath
	if strings.TrimSpace(*outputDir) != "" {
		requestedOutputDir = strings.TrimSpace(*outputDir)
	}
	if code, done := refuseBindingsExportInsideProject("taskjuggler run", *projectPath, stdout, stderr, requestedOutputDir); done {
		return code
	}

	result, warnings, errorsOut, err := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:                  *projectPath,
		ScenarioID:                   *scenarioID,
		OutputDir:                    *outputDir,
		TaskJugglerBinary:            *taskJugglerBinary,
		UnrepresentedAssumptionsMode: *unrepresentedAssumptions,
		Now:                          time.Now(),
	})
	env := envelope{
		OK:          err == nil,
		Command:     "taskjuggler run",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    warnings,
		Errors:      errorsOut,
	}
	if printErr := printEnvelope(stdout, *format, env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err == nil {
		return exitSuccess
	}
	if errors.Is(err, app.ErrValidationFailed) {
		return validationFailureExit(errorsOut)
	}
	return exitGeneralError
}

func runTaskJugglerRunArtifacts(ctx context.Context, req taskJugglerRunRequest) (TaskJugglerRunResult, []app.Diagnostic, []app.Diagnostic, error) {
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	timestamp := now.UTC().Format("20060102T150405Z")
	runID := "taskjuggler.run." + timestamp
	outputDir := strings.TrimSpace(req.OutputDir)
	if outputDir == "" {
		outputDir = defaultTaskJugglerRunOutputDir(req.ProjectPath, req.ScenarioID, timestamp)
	}
	result := TaskJugglerRunResult{
		ID:                    runID,
		ProjectPath:           req.ProjectPath,
		ScenarioID:            req.ScenarioID,
		OutputDir:             outputDir,
		ExitCode:              -1,
		Artifacts:             []string{},
		CanonicalStateMutated: false,
	}
	warnings := []app.Diagnostic{}
	errorsOut := []app.Diagnostic{}

	exportResult, exportErr := app.NewExportService(newStore(req.ProjectPath)).TaskJuggler(ctx, app.ExportTaskJugglerRequest{
		ScenarioID:                   req.ScenarioID,
		UnrepresentedAssumptionsMode: req.UnrepresentedAssumptionsMode,
	})
	warnings = append(warnings, exportResult.Warnings...)
	errorsOut = append(errorsOut, exportResult.Errors...)
	if exportErr != nil {
		return result, warnings, errorsOut, exportErr
	}

	taskJugglerPath, lookErr := exec.LookPath(req.TaskJugglerBinary)
	if lookErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_binary_not_found",
			Message:  fmt.Sprintf("TaskJuggler binary %q was not found; run artifacts were not created", req.TaskJugglerBinary),
			Severity: "error",
		})
		return result, warnings, errorsOut, app.ErrValidationFailed
	}
	result.TaskJugglerPath = taskJugglerPath

	if mkErr := os.MkdirAll(outputDir, 0o755); mkErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_output_dir_failed",
			Message:  fmt.Sprintf("failed to create TaskJuggler run output directory: %v", mkErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, mkErr
	}

	projectTJPPath := filepath.Join(outputDir, "project.tjp")
	stdoutPath := filepath.Join(outputDir, "stdout.txt")
	stderrPath := filepath.Join(outputDir, "stderr.txt")
	versionPath := filepath.Join(outputDir, "taskjuggler.version.txt")
	mappingPath := filepath.Join(outputDir, "taskjuggler.mapping.json")
	reportsDir := filepath.Join(outputDir, "reports")
	taskScheduleReportPath := filepath.Join(reportsDir, "replan-task-schedule.csv")
	manifestPath := filepath.Join(outputDir, "run.manifest.json")
	result.ProjectTJPPath = projectTJPPath
	result.StdoutPath = stdoutPath
	result.StderrPath = stderrPath
	result.VersionPath = versionPath
	result.MappingPath = mappingPath
	result.TaskScheduleReportPath = taskScheduleReportPath
	result.ManifestPath = manifestPath
	result.Artifacts = []string{projectTJPPath, stdoutPath, stderrPath, versionPath, mappingPath, taskScheduleReportPath, manifestPath}

	if writeErr := writeTextFile(projectTJPPath, exportResult.Content); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_project_write_failed",
			Message:  fmt.Sprintf("failed to write generated TaskJuggler project: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	if writeErr := writeJSONFile(mappingPath, exportResult.Mapping); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_mapping_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler mapping artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	if mkErr := os.MkdirAll(reportsDir, 0o755); mkErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_reports_dir_failed",
			Message:  fmt.Sprintf("failed to create TaskJuggler reports directory: %v", mkErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, mkErr
	}

	versionOutput, versionErr := exec.CommandContext(ctx, taskJugglerPath, "--version").CombinedOutput()
	result.TaskJugglerVersion = strings.TrimSpace(string(versionOutput))
	if versionErr != nil {
		warnings = append(warnings, app.Diagnostic{
			Code:     "taskjuggler_version_failed",
			Message:  fmt.Sprintf("failed to read TaskJuggler version: %v", versionErr),
			Severity: "warning",
		})
	}
	if writeErr := writeTextFile(versionPath, string(versionOutput)); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_version_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler version artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, taskJugglerPath, "project.tjp")
	cmd.Dir = outputDir
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()
	result.ExitCode = 0
	if runErr != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_failed",
			Message:  fmt.Sprintf("TaskJuggler run failed: %v", runErr),
			Severity: "error",
		})
	}

	if writeErr := writeTextFile(stdoutPath, stdoutBuf.String()); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_stdout_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler stdout artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	if writeErr := writeTextFile(stderrPath, stderrBuf.String()); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_stderr_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler stderr artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	reportSHA256 := ""
	if runErr == nil {
		reportContent, readErr := os.ReadFile(taskScheduleReportPath)
		if readErr != nil {
			errorsOut = append(errorsOut, app.Diagnostic{
				Code:     "taskjuggler_report_missing",
				Message:  fmt.Sprintf("expected TaskJuggler task schedule report was not created: %v", readErr),
				Severity: "error",
			})
			runErr = app.ErrValidationFailed
		} else {
			reportSHA256 = sha256Hex(reportContent)
		}
	}
	mappingSHA256 := ""
	if mappingContent, readErr := os.ReadFile(mappingPath); readErr == nil {
		mappingSHA256 = sha256Hex(mappingContent)
	}
	reports := []TaskJugglerRunReportArtifact{}
	if reportSHA256 != "" {
		reports = append(reports, TaskJugglerRunReportArtifact{
			Kind:   "taskjuggler_task_schedule_csv",
			Path:   filepath.Join("reports", "replan-task-schedule.csv"),
			SHA256: reportSHA256,
		})
	}

	manifest := TaskJugglerRunManifest{
		ID:                       runID,
		Command:                  "taskjuggler run",
		ProjectPath:              req.ProjectPath,
		ScenarioID:               req.ScenarioID,
		OutputDir:                outputDir,
		TaskJugglerPath:          taskJugglerPath,
		TaskJugglerVersion:       result.TaskJugglerVersion,
		TaskJugglerExitCode:      result.ExitCode,
		UnrepresentedAssumptions: req.UnrepresentedAssumptionsMode,
		ProjectTJPPath:           projectTJPPath,
		ProjectTJPSHA256:         sha256Hex([]byte(exportResult.Content)),
		StdoutPath:               stdoutPath,
		StderrPath:               stderrPath,
		VersionPath:              versionPath,
		MappingPath:              mappingPath,
		MappingSHA256:            mappingSHA256,
		Reports:                  reports,
		CreatedAt:                now.Format(time.RFC3339),
		CanonicalStateMutated:    false,
		Warnings:                 warnings,
		Errors:                   errorsOut,
	}
	if writeErr := writeJSONFile(manifestPath, manifest); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_manifest_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler run manifest: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}

	if runErr != nil {
		return result, warnings, errorsOut, app.ErrValidationFailed
	}
	return result, warnings, errorsOut, nil
}

func runTaskJugglerImportResult(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("taskjuggler import-result")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	runDir := fs.String("run-dir", "", "TaskJuggler run artifact directory")
	scenarioID := fs.String("scenario-id", "", "scenario candidate id to use in dry-run output")
	dryRun := fs.Bool("dry-run", false, "preview imported TaskJuggler result without writing canonical state")
	writeCandidate := fs.Bool("write-candidate", false, "write reviewable scenario/proposal candidate files without mutating canonical state")
	actorID := fs.String("actor", "", "actor id creating candidate files; required with --write-candidate")
	format := fs.String("format", "json", "output format: json, markdown or text")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		ProjectPath:    *projectPath,
		RunDir:         *runDir,
		ScenarioID:     *scenarioID,
		DryRun:         *dryRun,
		WriteCandidate: *writeCandidate,
		ActorID:        *actorID,
		Now:            time.Now(),
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "taskjuggler import-result",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

type taskJugglerImportRequest struct {
	ProjectPath    string
	RunDir         string
	ScenarioID     string
	DryRun         bool
	WriteCandidate bool
	ActorID        string
	Now            time.Time
}

func importTaskJugglerResult(ctx context.Context, req taskJugglerImportRequest) (TaskJugglerImportResult, error) {
	result := TaskJugglerImportResult{
		RunDir:        req.RunDir,
		PreviewOnly:   req.DryRun,
		PlannedWrites: []app.PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []app.Diagnostic{},
		Errors:        []app.Diagnostic{},
		Candidate: TaskJugglerScenarioCandidate{
			Source:  "taskjuggler",
			Changes: []TaskJugglerCandidateChange{},
		},
	}
	state := taskJugglerImportState{ctx: ctx, req: req, result: &result}
	steps := []func() error{
		state.validateRequest,
		state.loadManifest,
		state.loadMapping,
		state.loadReport,
		state.loadWorkspace,
		state.buildCandidateChanges,
		state.preparePreviews,
		state.writeCandidate,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return result, err
		}
	}
	return result, nil
}

type taskJugglerImportState struct {
	ctx       context.Context
	req       taskJugglerImportRequest
	result    *TaskJugglerImportResult
	manifest  TaskJugglerRunManifest
	taskIDMap map[string]string
	rows      []taskJugglerTaskScheduleRow
	workspace domain.Workspace
}

func (s *taskJugglerImportState) validateRequest() error {
	if s.req.DryRun && s.req.WriteCandidate {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "conflicting_taskjuggler_import_modes",
			Message:  "taskjuggler import-result accepts either --dry-run or --write-candidate, not both",
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	if !s.req.DryRun && !s.req.WriteCandidate {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "dry_run_required",
			Message:  "taskjuggler import-result requires --dry-run or explicit --write-candidate",
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	if s.req.WriteCandidate && strings.TrimSpace(s.req.ActorID) == "" {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "actor_required",
			Message:  "taskjuggler import-result --write-candidate requires --actor",
			Severity: "error",
			Target:   app.Target{Kind: "actor"},
		})
		return app.ErrValidationFailed
	}
	if strings.TrimSpace(s.req.RunDir) == "" {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "taskjuggler_run_dir_required",
			Message:  "--run-dir is required",
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	return nil
}

func (s *taskJugglerImportState) loadManifest() error {
	manifestPath := filepath.Join(s.req.RunDir, "run.manifest.json")
	s.result.ManifestPath = manifestPath
	if err := readJSONFile(manifestPath, &s.manifest); err != nil {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "taskjuggler_manifest_missing",
			Message:  fmt.Sprintf("failed to read TaskJuggler run manifest: %v", err),
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	s.result.Candidate.SourceRunID = s.manifest.ID
	if s.req.ScenarioID != "" {
		s.result.Candidate.ID = s.req.ScenarioID
	} else {
		s.result.Candidate.ID = defaultTaskJugglerScenarioCandidateID(s.manifest.ID)
	}
	return nil
}

func (s *taskJugglerImportState) loadMapping() error {
	mappingPath := resolveRunArtifactPath(s.req.RunDir, s.manifest.MappingPath)
	if mappingPath == "" {
		mappingPath = filepath.Join(s.req.RunDir, "taskjuggler.mapping.json")
	}
	s.result.MappingPath = mappingPath
	var mapping app.TaskJugglerExportMapping
	if err := readJSONFile(mappingPath, &mapping); err != nil {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "taskjuggler_mapping_missing",
			Message:  fmt.Sprintf("failed to read TaskJuggler mapping: %v", err),
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	var mappingErrors []app.Diagnostic
	s.taskIDMap, mappingErrors = taskJugglerTaskIDMap(mapping)
	s.result.Errors = append(s.result.Errors, mappingErrors...)
	if len(s.result.Errors) > 0 {
		return app.ErrValidationFailed
	}
	return nil
}

func (s *taskJugglerImportState) loadReport() error {
	reportPath := taskJugglerTaskScheduleReportPath(s.req.RunDir, s.manifest)
	s.result.ReportPath = reportPath
	var diagnostics []app.Diagnostic
	s.rows, diagnostics = readTaskJugglerTaskScheduleCSV(reportPath)
	if diagnostics != nil {
		s.result.Errors = append(s.result.Errors, diagnostics...)
		return app.ErrValidationFailed
	}
	s.result.RowsRead = len(s.rows)
	return nil
}

func (s *taskJugglerImportState) loadWorkspace() error {
	var loadDiags []app.Diagnostic
	s.workspace, loadDiags = newStore(s.req.ProjectPath).Load(s.ctx)
	for _, diag := range loadDiags {
		if diag.Severity == "error" {
			s.result.Errors = append(s.result.Errors, diag)
		} else {
			s.result.Warnings = append(s.result.Warnings, diag)
		}
	}
	if len(s.result.Errors) > 0 {
		return app.ErrValidationFailed
	}
	if s.req.WriteCandidate && !hasActor(s.workspace, s.req.ActorID) {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "actor_not_found",
			Message:  fmt.Sprintf("actor %s not found", s.req.ActorID),
			Severity: "error",
			Target:   app.Target{Kind: "actor", ID: s.req.ActorID},
		})
		return app.ErrValidationFailed
	}
	return nil
}

func (s *taskJugglerImportState) buildCandidateChanges() error {
	taskIDs := map[string]struct{}{}
	for _, task := range s.workspace.Tasks {
		taskIDs[task.ID] = struct{}{}
	}
	for _, row := range s.rows {
		replanTaskID, ok := s.taskIDMap[row.ID]
		if !ok {
			s.result.Warnings = append(s.result.Warnings, app.Diagnostic{
				Code:     "taskjuggler_report_task_unmapped",
				Message:  fmt.Sprintf("TaskJuggler report row %s has no RePlan task mapping", row.ID),
				Severity: "warning",
				Target:   app.Target{Kind: "taskjuggler_task", ID: row.ID},
			})
			continue
		}
		if _, exists := taskIDs[replanTaskID]; !exists {
			s.result.Warnings = append(s.result.Warnings, app.Diagnostic{
				Code:     "taskjuggler_report_task_missing",
				Message:  fmt.Sprintf("mapped RePlan task %s is not present in the project", replanTaskID),
				Severity: "warning",
				Target:   app.Target{Kind: "task", ID: replanTaskID},
			})
			continue
		}
		if row.Start != "" {
			s.result.Candidate.Changes = append(s.result.Candidate.Changes, TaskJugglerCandidateChange{
				Op:            "set",
				Path:          fmt.Sprintf("tasks.%s.planned_start", replanTaskID),
				Value:         row.Start,
				PreviousValue: "",
			})
		}
		if row.End != "" {
			s.result.Candidate.Changes = append(s.result.Candidate.Changes, TaskJugglerCandidateChange{
				Op:            "set",
				Path:          fmt.Sprintf("tasks.%s.planned_finish", replanTaskID),
				Value:         row.End,
				PreviousValue: "",
			})
		}
	}
	s.result.CandidateChanges = len(s.result.Candidate.Changes)
	if s.result.CandidateChanges == 0 {
		s.result.Warnings = append(s.result.Warnings, app.Diagnostic{
			Code:     "taskjuggler_report_no_candidate_changes",
			Message:  "TaskJuggler report produced no candidate changes",
			Severity: "warning",
		})
		if s.req.WriteCandidate {
			return app.ErrValidationFailed
		}
	}
	return nil
}

func (s *taskJugglerImportState) preparePreviews() error {
	now := s.req.Now
	if now.IsZero() {
		now = time.Now()
	}
	actorID := s.req.ActorID
	if actorID == "" {
		actorID = "actor.system.replan"
	}
	s.result.ScenarioPreview = taskJugglerScenarioPreview(s.result.Candidate, s.manifest, actorID, now)
	s.result.ProposalPreview = taskJugglerProposalPreview(s.result.ScenarioPreview, s.manifest, actorID, now)
	s.result.PlannedWrites = taskJugglerCandidatePlannedWrites(s.result.ScenarioPreview, s.result.ProposalPreview)
	return nil
}

func (s *taskJugglerImportState) writeCandidate() error {
	if s.req.WriteCandidate {
		_, written, diags, writeErr := app.CreateProjectFiles(context.Background(), newStore(s.req.ProjectPath), s.result.PlannedWrites)
		s.result.WrittenFiles = written
		if writeErr != nil {
			s.result.Errors = append(s.result.Errors, app.Diagnostic{
				Code:     "taskjuggler_candidate_write_failed",
				Message:  writeErr.Error(),
				Severity: "error",
			})
			s.result.Errors = append(s.result.Errors, diags...)
			return writeErr
		}
		s.result.Applied = true
	}
	return nil
}

type taskJugglerTaskScheduleRow struct {
	ID         string
	Name       string
	Start      string
	End        string
	Effort     string
	EffortLeft string
	Resources  string
	Status     string
}

func readTaskJugglerTaskScheduleCSV(path string) ([]taskJugglerTaskScheduleRow, []app.Diagnostic) {
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, []app.Diagnostic{{
			Code:     "taskjuggler_report_missing",
			Message:  fmt.Sprintf("failed to read TaskJuggler task schedule report: %v", readErr),
			Severity: "error",
		}}
	}
	reader := csv.NewReader(strings.NewReader(string(content)))
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	records, parseErr := reader.ReadAll()
	if parseErr != nil {
		return nil, []app.Diagnostic{{
			Code:     "taskjuggler_report_row_invalid",
			Message:  fmt.Sprintf("failed to parse TaskJuggler CSV report: %v", parseErr),
			Severity: "error",
		}}
	}
	if len(records) == 0 {
		return nil, []app.Diagnostic{{
			Code:     "taskjuggler_report_header_invalid",
			Message:  "TaskJuggler CSV report is empty",
			Severity: "error",
		}}
	}
	header := map[string]int{}
	for i, column := range records[0] {
		header[strings.TrimSpace(column)] = i
	}
	required := []string{"Id", "Name", "Start", "End", "Effort", "Effort Left", "Resources", "Status"}
	for _, column := range required {
		if _, ok := header[column]; !ok {
			return nil, []app.Diagnostic{{
				Code:     "taskjuggler_report_header_invalid",
				Message:  fmt.Sprintf("TaskJuggler CSV report is missing required column %q", column),
				Severity: "error",
			}}
		}
	}
	rows := []taskJugglerTaskScheduleRow{}
	diagnostics := []app.Diagnostic{}
	for i, record := range records[1:] {
		value := func(column string) string {
			idx := header[column]
			if idx >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[idx])
		}
		row := taskJugglerTaskScheduleRow{
			ID:         value("Id"),
			Name:       value("Name"),
			Start:      value("Start"),
			End:        value("End"),
			Effort:     value("Effort"),
			EffortLeft: value("Effort Left"),
			Resources:  value("Resources"),
			Status:     value("Status"),
		}
		if row.ID == "" {
			diagnostics = append(diagnostics, app.Diagnostic{
				Code:     "taskjuggler_report_row_invalid",
				Message:  fmt.Sprintf("TaskJuggler CSV row %d has empty Id", i+2),
				Severity: "error",
			})
			continue
		}
		if diag, ok := validateTaskJugglerReportDate(row.Start, row.ID, "Start"); ok {
			diagnostics = append(diagnostics, diag)
		}
		if diag, ok := validateTaskJugglerReportDate(row.End, row.ID, "End"); ok {
			diagnostics = append(diagnostics, diag)
		}
		rows = append(rows, row)
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return rows, nil
}

func validateTaskJugglerReportDate(value, taskID, column string) (app.Diagnostic, bool) {
	if value == "" {
		return app.Diagnostic{}, false
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return app.Diagnostic{
			Code:     "taskjuggler_report_date_invalid",
			Message:  fmt.Sprintf("TaskJuggler task %s has invalid %s date %q", taskID, column, value),
			Severity: "error",
			Target:   app.Target{Kind: "taskjuggler_task", ID: taskID},
		}, true
	}
	return app.Diagnostic{}, false
}

func taskJugglerTaskIDMap(mapping app.TaskJugglerExportMapping) (map[string]string, []app.Diagnostic) {
	out := map[string]string{}
	diagnostics := []app.Diagnostic{}
	for _, entry := range mapping.TaskIDMap {
		if entry.TaskJugglerTaskID == "" || entry.RePlanTaskID == "" {
			diagnostics = append(diagnostics, app.Diagnostic{
				Code:     "taskjuggler_mapping_invalid",
				Message:  "TaskJuggler mapping contains an empty task id",
				Severity: "error",
			})
			continue
		}
		if previous, exists := out[entry.TaskJugglerTaskID]; exists && previous != entry.RePlanTaskID {
			diagnostics = append(diagnostics, app.Diagnostic{
				Code:     "taskjuggler_mapping_duplicate",
				Message:  fmt.Sprintf("TaskJuggler task id %s maps to both %s and %s", entry.TaskJugglerTaskID, previous, entry.RePlanTaskID),
				Severity: "error",
				Target:   app.Target{Kind: "taskjuggler_task", ID: entry.TaskJugglerTaskID},
			})
			continue
		}
		out[entry.TaskJugglerTaskID] = entry.RePlanTaskID
	}
	return out, diagnostics
}

func taskJugglerTaskScheduleReportPath(runDir string, manifest TaskJugglerRunManifest) string {
	for _, report := range manifest.Reports {
		if report.Kind == "taskjuggler_task_schedule_csv" && strings.TrimSpace(report.Path) != "" {
			return resolveRunArtifactPath(runDir, report.Path)
		}
	}
	return filepath.Join(runDir, "reports", "replan-task-schedule.csv")
}

func resolveRunArtifactPath(runDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(runDir, path)
}

func defaultTaskJugglerScenarioCandidateID(runID string) string {
	if strings.HasPrefix(runID, "taskjuggler.run.") {
		return "scenario.taskjuggler." + strings.TrimPrefix(runID, "taskjuggler.run.")
	}
	if strings.TrimSpace(runID) == "" {
		return "scenario.taskjuggler." + time.Now().UTC().Format("20060102T150405Z")
	}
	return "scenario.taskjuggler." + safeTaskJugglerPathSegment(runID)
}

func taskJugglerScenarioPreview(candidate TaskJugglerScenarioCandidate, manifest TaskJugglerRunManifest, actorID string, now time.Time) domain.Scenario {
	changes := make([]domain.ScenarioChange, 0, len(candidate.Changes))
	for _, change := range candidate.Changes {
		changes = append(changes, domain.ScenarioChange{
			Op:    change.Op,
			Path:  change.Path,
			Value: change.Value,
		})
	}
	baseRef := domain.Ref{Kind: "current_plan", ID: "current"}
	if manifest.ScenarioID != "" {
		baseRef = domain.Ref{Kind: "scenario", ID: manifest.ScenarioID}
	}
	return domain.Scenario{
		ID:        candidate.ID,
		Name:      "TaskJuggler schedule " + now.UTC().Format("2006-01-02 15:04:05Z"),
		Status:    "proposed",
		BaseRef:   baseRef,
		CreatedAt: now.Format(time.RFC3339),
		CreatedBy: domain.Ref{Kind: "actor", ID: actorID},
		Changes:   changes,
	}
}

func taskJugglerProposalPreview(scenario domain.Scenario, manifest TaskJugglerRunManifest, actorID string, now time.Time) domain.Proposal {
	return domain.Proposal{
		ID:        "proposal.taskjuggler." + strings.TrimPrefix(scenario.ID, "scenario.taskjuggler."),
		Type:      "taskjuggler_schedule_candidate",
		Status:    "open",
		CreatedAt: now.Format(time.RFC3339),
		CreatedBy: domain.Ref{Kind: "actor", ID: actorID},
		TargetRef: domain.Ref{Kind: "scenario", ID: scenario.ID},
		SuggestedChange: domain.SuggestedChange{
			Op:    "create",
			Path:  filepath.ToSlash(filepath.Join("scenarios", scenario.ID+".yaml")),
			Value: scenario.ID,
		},
		Rationale:            fmt.Sprintf("TaskJuggler run %s produced %d planned task date candidate changes.", manifest.ID, len(scenario.Changes)),
		RequiresConfirmation: true,
	}
}

func taskJugglerCandidatePlannedWrites(scenario domain.Scenario, proposal domain.Proposal) []app.PlannedWrite {
	scenarioContent, _ := yaml.Marshal(struct {
		Scenario domain.Scenario `yaml:"scenario"`
	}{Scenario: scenario})
	proposalContent, _ := yaml.Marshal(struct {
		Proposal domain.Proposal `yaml:"proposal"`
	}{Proposal: proposal})
	return []app.PlannedWrite{
		{
			Path:    filepath.ToSlash(filepath.Join("scenarios", scenario.ID+".yaml")),
			Action:  "create",
			Format:  "yaml",
			Content: string(scenarioContent),
		},
		{
			Path:    filepath.ToSlash(filepath.Join("proposals", "open", proposal.ID+".yaml")),
			Action:  "create",
			Format:  "yaml",
			Content: string(proposalContent),
		},
	}
}

func hasActor(ws domain.Workspace, actorID string) bool {
	for _, actor := range ws.Actors {
		if actor.ID == actorID {
			return true
		}
	}
	return false
}

func readJSONFile(path string, value any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, value)
}

func defaultTaskJugglerRunOutputDir(projectPath, scenarioID, timestamp string) string {
	scope := "current"
	if strings.TrimSpace(scenarioID) != "" {
		scope = safeTaskJugglerPathSegment(scenarioID)
		if !strings.HasPrefix(scope, "scenario.") {
			scope = "scenario." + scope
		}
	}
	return filepath.Join(projectPath, "exports", "taskjuggler", scope, "run."+timestamp)
}

func safeTaskJugglerPathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unnamed"
	}
	return b.String()
}

func runTaskJugglerValidation(ctx context.Context, req taskJugglerValidationRequest) (TaskJugglerValidationResult, []app.Diagnostic, []app.Diagnostic, error) {
	result := TaskJugglerValidationResult{
		ProjectPath: req.ProjectPath,
		ScenarioID:  req.ScenarioID,
	}
	warnings := []app.Diagnostic{}
	errorsOut := []app.Diagnostic{}

	exportResult, exportErr := app.NewExportService(newStore(req.ProjectPath)).TaskJuggler(ctx, app.ExportTaskJugglerRequest{
		ScenarioID:                   req.ScenarioID,
		UnrepresentedAssumptionsMode: req.UnrepresentedAssumptionsMode,
	})
	warnings = append(warnings, exportResult.Warnings...)
	errorsOut = append(errorsOut, exportResult.Errors...)
	if exportErr != nil {
		return result, warnings, errorsOut, exportErr
	}

	taskJugglerPath, lookErr := exec.LookPath(req.TaskJugglerBinary)
	if lookErr != nil {
		result.Skipped = true
		warnings = append(warnings, app.Diagnostic{
			Code:     "taskjuggler_binary_not_found",
			Message:  fmt.Sprintf("TaskJuggler binary %q was not found; generated .tjp runtime validation was skipped", req.TaskJugglerBinary),
			Severity: "warning",
		})
		return result, warnings, errorsOut, nil
	}
	result.TaskJugglerPath = taskJugglerPath

	tmpDir, createErr := os.MkdirTemp("", "replan-taskjuggler-*")
	if createErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_tempfile_failed",
			Message:  fmt.Sprintf("failed to create temporary TaskJuggler directory: %v", createErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, createErr
	}
	tmpName := filepath.Join(tmpDir, "project.tjp")
	reportsDir := filepath.Join(tmpDir, "reports")
	if req.KeepTemp {
		result.ArtifactPath = tmpName
	}
	cleanup := !req.KeepTemp
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	if mkErr := os.MkdirAll(reportsDir, 0o755); mkErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_tempfile_failed",
			Message:  fmt.Sprintf("failed to create temporary TaskJuggler reports directory: %v", mkErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, mkErr
	}
	if writeErr := writeTextFile(tmpName, exportResult.Content); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_tempfile_failed",
			Message:  fmt.Sprintf("failed to write temporary .tjp file: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}

	cmd := exec.CommandContext(ctx, taskJugglerPath, "project.tjp")
	cmd.Dir = tmpDir
	output, runErr := cmd.CombinedOutput()
	result.Output = string(output)
	if runErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_failed",
			Message:  fmt.Sprintf("TaskJuggler validation failed: %v", runErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, app.ErrValidationFailed
	}
	result.Validated = true
	return result, warnings, errorsOut, nil
}

func runExportJSON(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("export json")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	output := fs.String("output", "", "path to write JSON export; stdout when omitted")
	manifest := fs.String("manifest", "", "path to write export manifest JSON; used only with --output")
	format := fs.String("format", "json", "output format: json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *manifest != "" && *output == "" {
		return printExportManifestRequiresOutput(stdout, stderr, "export json", *projectPath)
	}
	if strings.ToLower(*format) != "json" {
		env := envelope{
			OK:          false,
			Command:     "export json",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "export json supports only json format",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	if code, done := refuseBindingsExportInsideProject("export json", *projectPath, stdout, stderr, *output, *manifest); done {
		return code
	}
	result, err := app.NewExportService(newStore(*projectPath)).JSON(context.Background(), app.ExportJSONRequest{Format: "json"})
	if err != nil && diagnosticsContainNotFound(result.Errors) {
		env := envelope{OK: false, Command: "export json", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitNotFound
	}
	if *output != "" && err == nil {
		content, renderErr := jsonBytes(result)
		if renderErr != nil {
			fmt.Fprintf(stderr, "failed to render export: %v\n", renderErr)
			return exitGeneralError
		}
		if writeErr := writeTextFile(*output, string(content)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write export: %v\n", writeErr)
			return exitGeneralError
		}
		if *manifest != "" {
			if writeErr := writeExportManifestFile(*manifest, ExportManifest{
				ID:          result.Metadata.ID,
				Command:     "export json",
				Format:      "json",
				ProjectPath: *projectPath,
				OutputPath:  *output,
				SHA256:      sha256Hex(content),
				CreatedAt:   result.Metadata.ExportedAt,
				Warnings:    result.Warnings,
				Errors:      result.Errors,
			}); writeErr != nil {
				fmt.Fprintf(stderr, "failed to write manifest: %v\n", writeErr)
				return exitGeneralError
			}
		}
	} else {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if printErr := enc.Encode(result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runExportCSV(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("export csv")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	reportName := fs.String("report", "", "report to export: workload")
	week := fs.String("week", "", "ISO week, for example 2026-W28")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	output := fs.String("output", "", "path to write CSV export; stdout when omitted")
	manifest := fs.String("manifest", "", "path to write export manifest JSON; used only with --output")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *manifest != "" && *output == "" {
		return printExportManifestRequiresOutput(stdout, stderr, "export csv", *projectPath)
	}
	if *reportName != "workload" {
		env := envelope{
			OK:          false,
			Command:     "export csv",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_export_report",
				Message:  "export csv currently supports only --report workload",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	if *week != "" && !validISOWeek(*week) {
		return printInvalidWeek(stdout, "export csv", *projectPath, *week)
	}
	if code, done := refuseBindingsExportInsideProject("export csv", *projectPath, stdout, stderr, *output, *manifest); done {
		return code
	}
	result, err := app.NewWorkloadService(newStore(*projectPath)).Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: *projectPath,
		Week:        *week,
		ScenarioID:  *scenarioID,
	})
	if err == nil && *output != "" {
		var buf strings.Builder
		if writeErr := reports.WriteWorkloadCSV(&buf, workloadCSV(result)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to render csv: %v\n", writeErr)
			return exitGeneralError
		}
		content := []byte(buf.String())
		if writeErr := writeTextFile(*output, string(content)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write export: %v\n", writeErr)
			return exitGeneralError
		}
		if *manifest != "" {
			if writeErr := writeExportManifestFile(*manifest, ExportManifest{
				ID:          exportManifestID("csv", "workload", result.Week, result.ScenarioID),
				Command:     "export csv",
				Format:      "csv",
				Report:      *reportName,
				ProjectPath: *projectPath,
				OutputPath:  *output,
				ScenarioID:  result.ScenarioID,
				Week:        result.Week,
				SHA256:      sha256Hex(content),
				CreatedAt:   "",
				Warnings:    result.Warnings,
				Errors:      result.Errors,
			}); writeErr != nil {
				fmt.Fprintf(stderr, "failed to write manifest: %v\n", writeErr)
				return exitGeneralError
			}
		}
	} else if err == nil {
		if writeErr := reports.WriteWorkloadCSV(stdout, workloadCSV(result)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", writeErr)
			return exitGeneralError
		}
	}
	if err != nil {
		env := envelope{
			OK:          false,
			Command:     "export csv",
			ProjectPath: *projectPath,
			Result:      result,
			Warnings:    result.Warnings,
			Errors:      result.Errors,
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func workloadCSV(result app.WorkloadResult) reports.WorkloadCSV {
	out := reports.WorkloadCSV{
		Week:       result.Week,
		ScenarioID: result.ScenarioID,
		Resources:  make([]reports.WorkloadCSVRow, 0, len(result.Resources)),
	}
	for _, row := range result.Resources {
		out.Resources = append(out.Resources, reports.WorkloadCSVRow{
			ResourceID:    row.ResourceID,
			ResourceName:  row.ResourceName,
			ResourceClass: row.ResourceClass,
			Unit:          row.Unit,
			Capacity:      row.Capacity,
			Planned:       row.Planned,
			Reserved:      row.Reserved,
			Actual:        row.Actual,
			Remaining:     row.Remaining,
			Overload:      row.Overload,
		})
	}
	return out
}

func runExportTaskJuggler(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("export taskjuggler")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	output := fs.String("output", "", "path to write TaskJuggler .tjp export; stdout when omitted")
	manifest := fs.String("manifest", "", "path to write export manifest JSON; used only with --output")
	format := fs.String("format", "text", "output format: text or json")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	unrepresentedAssumptions := fs.String("unrepresented-assumptions", "warn", "how to handle scenario assumptions not represented in TaskJuggler .tjp: warn or error")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *manifest != "" && *output == "" {
		return printExportManifestRequiresOutput(stdout, stderr, "export taskjuggler", *projectPath)
	}
	*format = strings.ToLower(*format)
	if *format != "text" && *format != "json" {
		env := envelope{
			OK:          false,
			Command:     "export taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are text and json",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	*unrepresentedAssumptions = strings.ToLower(strings.TrimSpace(*unrepresentedAssumptions))
	if *unrepresentedAssumptions == "" {
		*unrepresentedAssumptions = "warn"
	}
	if *unrepresentedAssumptions != "warn" && *unrepresentedAssumptions != "error" {
		env := envelope{
			OK:          false,
			Command:     "export taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_unrepresented_assumptions_mode",
				Message:  "supported --unrepresented-assumptions modes are warn and error",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	if code, done := refuseBindingsExportInsideProject("export taskjuggler", *projectPath, stdout, stderr, *output, *manifest); done {
		return code
	}
	result, err := app.NewExportService(newStore(*projectPath)).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID:                   *scenarioID,
		UnrepresentedAssumptionsMode: *unrepresentedAssumptions,
	})
	if err != nil && diagnosticsContainNotFound(result.Errors) {
		env := envelope{OK: false, Command: "export taskjuggler", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
		if printErr := printEnvelope(stdout, *format, env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitNotFound
	}
	if *output != "" && err == nil {
		content := []byte(result.Content)
		if writeErr := writeTextFile(*output, result.Content); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write export: %v\n", writeErr)
			return exitGeneralError
		}
		if *manifest != "" {
			if writeErr := writeExportManifestFile(*manifest, ExportManifest{
				ID:          result.Metadata.ID,
				Command:     "export taskjuggler",
				Format:      "taskjuggler",
				ProjectPath: *projectPath,
				OutputPath:  *output,
				ScenarioID:  result.Scenario,
				SHA256:      sha256Hex(content),
				CreatedAt:   result.Metadata.ExportedAt,
				Warnings:    result.Warnings,
				Errors:      result.Errors,
			}); writeErr != nil {
				fmt.Fprintf(stderr, "failed to write manifest: %v\n", writeErr)
				return exitGeneralError
			}
		}
	}
	if *format == "json" {
		if printErr := printExportTaskJugglerJSON(stdout, result, *output); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
	} else if *output == "" {
		if _, printErr := fmt.Fprint(stdout, result.Content); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runImport(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing import subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "github":
		return runImportGitHub(args[1:], stdout, stderr)
	case "openproject":
		return runImportOpenProject(args[1:], stdout, stderr)
	case "accept":
		return runImportAccept(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown import subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runImportGitHub(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("import github")
	defaultRequestTimeout, defaultRequestTimeoutErr := defaultOpenProjectRequestTimeoutSeconds()
	defaultRequestRetries, defaultRequestRetriesErr := defaultOpenProjectRequestRetries()
	defaultRequestRetryDelay, defaultRequestRetryDelayErr := defaultOpenProjectRequestRetryDelaySeconds()
	projectPath, format := addProjectFormatFlags(fs, "json")
	fromFile := fs.String("from-file", "", "versioned GitHub Issues import JSON document")
	profile := fs.String("profile", "", "GitHub source profile id from .replan/project.yaml")
	repository := fs.String("repository", "", "GitHub repository in owner/repo form")
	baseURL := fs.String("base-url", "", "GitHub API base URL (default https://api.github.com)")
	tokenEnv := fs.String("token-env", "", "environment variable containing a GitHub token (default GITHUB_TOKEN; empty disables token lookup)")
	since := fs.String("since", "", "import issues updated since YYYY-MM-DD or RFC3339")
	includeClosed := fs.Bool("include-closed", false, "include closed issues")
	includeRelations := fs.Bool("include-relations", false, "include blocked-by links as dependency candidates")
	resourceLinksSidecar := fs.String("resource-links-sidecar", "", "GitHub resource-link sidecar path")
	resourceDemandsSidecar := fs.String("resource-demands-sidecar", "", "GitHub resource-demand sidecar path")
	requestTimeout := fs.Float64("request-timeout", defaultRequestTimeout, "GitHub API request timeout in seconds; default can be set with REPLAN_OPENPROJECT_TIMEOUT_SECONDS")
	requestRetries := fs.Int("request-retries", defaultRequestRetries, "GitHub API transient GET retry count; default can be set with REPLAN_OPENPROJECT_RETRIES")
	requestRetryDelay := fs.Float64("request-retry-delay", defaultRequestRetryDelay, "GitHub API retry delay in seconds; default can be set with REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS")
	dryRun := fs.Bool("dry-run", false, "preview the import without writing files")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if defaultRequestTimeoutErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestTimeoutErr)
		return exitInvalidUsage
	}
	if defaultRequestRetriesErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetriesErr)
		return exitInvalidUsage
	}
	if defaultRequestRetryDelayErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetryDelayErr)
		return exitInvalidUsage
	}
	if *requestTimeout <= 0 {
		fmt.Fprintln(stderr, "--request-timeout must be positive")
		return exitInvalidUsage
	}
	if *requestRetries < 0 {
		fmt.Fprintln(stderr, "--request-retries must be non-negative")
		return exitInvalidUsage
	}
	if *requestRetryDelay <= 0 {
		fmt.Fprintln(stderr, "--request-retry-delay must be positive")
		return exitInvalidUsage
	}
	tokenEnvSet := false
	liveFlagsSet := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == "token-env" {
			tokenEnvSet = true
		}
		switch flag.Name {
		case "repository", "base-url", "token-env", "request-timeout", "request-retries", "request-retry-delay":
			liveFlagsSet = true
		}
	})
	if !*dryRun {
		if code, done := refuseBindingsLayerWrite("import github", *projectPath, *format, stdout, stderr); done {
			return code
		}
	}
	result, err := app.NewGitHubImportService(newStore(*projectPath)).Import(context.Background(), app.GitHubImportRequest{
		FromFile: *fromFile, DryRun: *dryRun, Profile: *profile, Repository: *repository, BaseURL: *baseURL,
		TokenEnv: *tokenEnv, TokenEnvSet: tokenEnvSet, LiveFlagsSet: liveFlagsSet,
		UserAgent: "replan/" + buildinfo.Current().Version, Since: *since,
		IncludeClosed: *includeClosed, IncludeRelations: *includeRelations,
		ResourceLinksSidecar: *resourceLinksSidecar, ResourceDemandsSidecar: *resourceDemandsSidecar,
		RequestTimeoutSeconds: *requestTimeout, RequestRetries: *requestRetries, RequestRetryDelaySeconds: *requestRetryDelay,
	})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "import github", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if *format == "markdown" || *format == "text" {
		if printErr := writeGitHubImportMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if diagnosticsContainExternal(result.Errors) {
				return exitExternalSource
			}
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runImportOpenProject(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("import openproject")
	defaultRequestTimeout, defaultRequestTimeoutErr := defaultOpenProjectRequestTimeoutSeconds()
	defaultRequestRetries, defaultRequestRetriesErr := defaultOpenProjectRequestRetries()
	defaultRequestRetryDelay, defaultRequestRetryDelayErr := defaultOpenProjectRequestRetryDelaySeconds()
	projectPath, format := addProjectFormatFlags(fs, "json")
	fromFile := fs.String("from-file", "", "OpenProject work packages JSON snapshot")
	profile := fs.String("profile", "", "OpenProject source profile id from .replan/project.yaml")
	baseURL := fs.String("base-url", "", "OpenProject base URL")
	projectID := fs.String("project-id", "", "OpenProject project id or identifier")
	tokenEnv := fs.String("token-env", "", "environment variable containing OpenProject API token")
	hostHeader := fs.String("host-header", "", "HTTP Host header override for tunneled OpenProject API access")
	forwardedProto := fs.String("forwarded-proto", "", "X-Forwarded-Proto header override for tunneled OpenProject API access")
	perimeterKeyEnv := fs.String("perimeter-key-env", app.DefaultOpenProjectPerimeterKeyEnv, "environment variable containing the key a reverse proxy in front of OpenProject asks of API clients; ignored when unset")
	perimeterHeader := fs.String("perimeter-header", os.Getenv("REPLAN_OPENPROJECT_PERIMETER_HEADER"), "request header that carries the perimeter key (default X-Perimeter-Key)")
	since := fs.String("since", "", "import work packages updated since date or timestamp")
	workPackageID := fs.String("work-package", "", "import one OpenProject work package id")
	pageSize := fs.Int("page-size", 100, "OpenProject API page size")
	requestTimeout := fs.Float64("request-timeout", defaultRequestTimeout, "OpenProject API request timeout in seconds; default can be set with REPLAN_OPENPROJECT_TIMEOUT_SECONDS")
	requestRetries := fs.Int("request-retries", defaultRequestRetries, "OpenProject API transient GET retry count; default can be set with REPLAN_OPENPROJECT_RETRIES")
	requestRetryDelay := fs.Float64("request-retry-delay", defaultRequestRetryDelay, "OpenProject API retry delay in seconds; default can be set with REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS")
	includeClosed := fs.Bool("include-closed", false, "include closed work packages when supported by the source query")
	includeTimeEntries := fs.Bool("include-time-entries", false, "include OpenProject time entries as read-only actual candidates")
	includeRelations := fs.Bool("include-relations", false, "include OpenProject relations as read-only dependency candidates")
	resourceLinksSidecar := fs.String("resource-links-sidecar", "", "OpenProject resource-link sidecar path")
	resourceDemandsSidecar := fs.String("resource-demands-sidecar", "", "OpenProject resource-demand sidecar path")
	dryRun := fs.Bool("dry-run", false, "preview the import without writing files")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if defaultRequestTimeoutErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestTimeoutErr)
		return exitInvalidUsage
	}
	if defaultRequestRetriesErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetriesErr)
		return exitInvalidUsage
	}
	if defaultRequestRetryDelayErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetryDelayErr)
		return exitInvalidUsage
	}
	if *requestTimeout <= 0 {
		fmt.Fprintln(stderr, "--request-timeout must be positive")
		return exitInvalidUsage
	}
	if *requestRetries < 0 {
		fmt.Fprintln(stderr, "--request-retries must be non-negative")
		return exitInvalidUsage
	}
	if *requestRetryDelay <= 0 {
		fmt.Fprintln(stderr, "--request-retry-delay must be positive")
		return exitInvalidUsage
	}
	if !*dryRun {
		if code, done := refuseBindingsLayerWrite("import openproject", *projectPath, *format, stdout, stderr); done {
			return code
		}
	}
	result, err := app.NewOpenProjectImportService(newStore(*projectPath)).Import(context.Background(), app.OpenProjectImportRequest{
		FromFile:                 *fromFile,
		DryRun:                   *dryRun,
		Profile:                  *profile,
		BaseURL:                  *baseURL,
		ProjectID:                *projectID,
		TokenEnv:                 *tokenEnv,
		HostHeader:               *hostHeader,
		ForwardedProto:           *forwardedProto,
		PerimeterKeyEnv:          *perimeterKeyEnv,
		PerimeterHeader:          *perimeterHeader,
		Since:                    *since,
		IncludeClosed:            *includeClosed,
		IncludeTimeEntries:       *includeTimeEntries,
		IncludeRelations:         *includeRelations,
		ResourceLinksSidecar:     *resourceLinksSidecar,
		ResourceDemandsSidecar:   *resourceDemandsSidecar,
		WorkPackageID:            *workPackageID,
		PageSize:                 *pageSize,
		RequestTimeoutSeconds:    *requestTimeout,
		RequestRetries:           *requestRetries,
		RequestRetryDelaySeconds: *requestRetryDelay,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "import openproject",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeOpenProjectImportMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if diagnosticsContainPrivateOpenProjectIDs(result.Errors) {
				return exitInvalidUsage
			}
			if diagnosticsContainExternal(result.Errors) {
				return exitExternalSource
			}
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func defaultOpenProjectRequestTimeoutSeconds() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("REPLAN_OPENPROJECT_TIMEOUT_SECONDS"))
	if raw == "" {
		return 60, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 60, fmt.Errorf("REPLAN_OPENPROJECT_TIMEOUT_SECONDS must be numeric")
	}
	if value <= 0 {
		return 60, fmt.Errorf("REPLAN_OPENPROJECT_TIMEOUT_SECONDS must be positive")
	}
	return value, nil
}

func defaultOpenProjectRequestRetries() (int, error) {
	raw := strings.TrimSpace(os.Getenv("REPLAN_OPENPROJECT_RETRIES"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("REPLAN_OPENPROJECT_RETRIES must be an integer")
	}
	if value < 0 {
		return 0, fmt.Errorf("REPLAN_OPENPROJECT_RETRIES must be non-negative")
	}
	return value, nil
}

func defaultOpenProjectRequestRetryDelaySeconds() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS"))
	if raw == "" {
		return 2, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 2, fmt.Errorf("REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS must be numeric")
	}
	if value <= 0 {
		return 2, fmt.Errorf("REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS must be positive")
	}
	return value, nil
}

func runImportAccept(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("import accept")
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	acceptTimeEntries := fs.Bool("accept-time-entries", false, "accept tracker time-entry candidates as actual consumption with evidence (currently OpenProject snapshots only)")
	acceptResourceDemands := fs.Bool("accept-resource-demands", false, "accept tracker resource-demand candidates into canonical resource demands")
	acceptResourceLinks := fs.Bool("accept-resource-links", false, "accept tracker resource-link candidates as canonical resource source refs")
	acceptTrackingStatus := fs.Bool("accept-tracking-status", false, "accept mapped tracker statuses as canonical task tracking statuses")
	acceptDates := fs.Bool("accept-dates", false, "accept tracker start and due dates as canonical task planned dates")
	acceptDependencies := fs.Bool("accept-dependencies", false, "accept tracker dependency candidates into canonical dependencies")
	actorID := fs.String("actor", "", "actor id applying the import acceptance")
	reason := fs.String("reason", "", "reason for accepting the import")
	positionalSnapshotID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("import accept", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	snapshotID := positionalArg(positionalSnapshotID, fs)
	result, err := app.NewImportAcceptService(newStore(*projectPath)).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:            snapshotID,
		DryRun:                dryRun,
		Apply:                 !dryRun,
		AcceptTimeEntries:     *acceptTimeEntries,
		AcceptResourceDemands: *acceptResourceDemands,
		AcceptResourceLinks:   *acceptResourceLinks,
		AcceptTrackingStatus:  *acceptTrackingStatus,
		AcceptDates:           *acceptDates,
		AcceptDependencies:    *acceptDependencies,
		ActorID:               *actorID,
		Reason:                *reason,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "import accept",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeImportAcceptMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if diagnosticsContainPrivateOpenProjectIDs(result.Errors) {
				return exitInvalidUsage
			}
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runActual(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing actual subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runActualList(args[1:], stdout, stderr)
	case "add-consumption":
		return runActualAddConsumption(args[1:], stdout, stderr)
	case "set-remaining":
		return runActualSetRemaining(args[1:], stdout, stderr)
	case "close-demand":
		return runActualCloseDemand(args[1:], stdout, stderr)
	case "add-status":
		return runActualAddStatus(args[1:], stdout, stderr)
	case "reopen-demand":
		return runActualReopenDemand(args[1:], stdout, stderr)
	case "acknowledge-overrun":
		return runActualAcknowledgeOverrun(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown actual subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runEvidence(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing evidence subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "add":
		return runEvidenceAdd(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown evidence subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runTask(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing task subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "add":
		return runTaskAdd(args[1:], stdout, stderr)
	case "set-dates":
		return runTaskSetDates(args[1:], stdout, stderr)
	case "record-actual-window":
		return runTaskRecordActualWindow(args[1:], stdout, stderr)
	case "set-title":
		return runTaskSetTitle(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown task subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runTaskAdd(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("task add")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	id := fs.String("id", "", "required task id")
	title := fs.String("title", "", "required task title")
	description := fs.String("description", "", "optional task description")
	taskType := fs.String("type", "implementation", "optional task type (default implementation)")
	plannedStart := fs.String("planned-start", "", "optional planned start date (YYYY-MM-DD)")
	plannedFinish := fs.String("planned-finish", "", "optional planned finish date (YYYY-MM-DD)")
	var tags repeatedStringFlag
	fs.Var(&tags, "tag", "optional repeatable task tag")
	assignee := fs.String("assignee", "", "optional existing resource id used as assignee_hint")
	after := fs.String("after", "", "optional existing task after which to insert the current-plan reference; default is the end")
	var demands repeatedStringFlag
	fs.Var(&demands, "demand", "required repeatable SUFFIX=RESOURCE_ID:AMOUNT:UNIT[:ROLE]; id is resource_demand.<task id after task.<namespace>.>.<SUFFIX>; role defaults to SUFFIX; unit must match a resource capacity")
	actor := fs.String("actor", "", "required existing actor id; also used as the audit actor")
	evidence := fs.String("evidence", "", "required existing evidence id for the audit record")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("task add", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("task add", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewTaskService(newStore(*projectPath)).Add(context.Background(), app.TaskAddRequest{ID: *id, Title: *title, Description: *description, Type: *taskType, PlannedStart: *plannedStart, PlannedFinish: *plannedFinish, Tags: tags, AssigneeID: *assignee, AfterTaskID: *after, DemandSpecs: demands, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "task add", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "text") && err == nil {
		fmt.Fprintf(stdout, "Task `%s`\n", result.Task.ID)
		hints := map[string]app.TaskCalibratedValue{}
		for _, hint := range result.CalibratedValues {
			hints[hint.DemandID] = hint
		}
		for _, demand := range result.Demands {
			fmt.Fprintf(stdout, "- `%s`: %g %s", demand.ID, demand.PlannedAmount, demand.Unit)
			if hint, ok := hints[demand.ID]; ok {
				fmt.Fprintf(stdout, ", calibrated %g (%s)", hint.Calibrated, hint.CalibrationID)
			}
			fmt.Fprintln(stdout)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runTaskSetDates(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("task set-dates")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	taskID := fs.String("task", "", "required existing task id")
	plannedStart := fs.String("planned-start", "", "optional new planned start date (YYYY-MM-DD); at least one date is required")
	plannedFinish := fs.String("planned-finish", "", "optional new planned finish date (YYYY-MM-DD); at least one date is required")
	reason := fs.String("reason", "", "required reason for moving the dates")
	actor := fs.String("actor", "", "required existing actor id; also used as the audit actor")
	evidence := fs.String("evidence", "", "required existing evidence id for the audit record")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("task set-dates", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("task set-dates", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewTaskService(newStore(*projectPath)).SetDates(context.Background(), app.TaskSetDatesRequest{TaskID: *taskID, PlannedStart: *plannedStart, PlannedFinish: *plannedFinish, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "task set-dates", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runTaskRecordActualWindow(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("task record-actual-window")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	taskID := fs.String("task", "", "required existing task id")
	reason := fs.String("reason", "", "optional reason for recording the actual window")
	actor := fs.String("actor", "", "required existing actor id; also used as the audit actor")
	evidence := fs.String("evidence", "", "required existing evidence id for the audit record")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("task record-actual-window", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("task record-actual-window", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewTaskService(newStore(*projectPath)).RecordActualWindow(context.Background(), app.TaskRecordActualWindowRequest{TaskID: *taskID, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "task record-actual-window", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runTaskSetTitle(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("task set-title")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	taskID := fs.String("task", "", "required existing task id")
	title := fs.String("title", "", "required new task title")
	reason := fs.String("reason", "", "required reason for changing the title")
	actor := fs.String("actor", "", "required existing actor id; also used as the audit actor")
	evidence := fs.String("evidence", "", "required existing evidence id for the audit record")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("task set-title", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("task set-title", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewTaskService(newStore(*projectPath)).SetTitle(context.Background(), app.TaskSetTitleRequest{TaskID: *taskID, Title: *title, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "task set-title", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func supportedTaskFormat(format string) bool {
	format = strings.ToLower(format)
	return format == "json" || format == "text"
}

func printTaskFormatError(command, projectPath string, stdout, stderr *os.File) int {
	env := envelope{OK: false, Command: command, ProjectPath: projectPath, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{{Code: "unsupported_format", Message: "supported formats are json and text", Severity: "error"}}}
	if err := printEnvelope(stdout, "json", env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError
	}
	return exitInvalidUsage
}

func runEvidenceAdd(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("evidence add")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	id := fs.String("id", "", "required evidence id; ids are never generated")
	evidenceType := fs.String("type", "", "required evidence type (free text accepted by replan validate)")
	subtype := fs.String("subtype", "", "optional evidence subtype (free text accepted by replan validate)")
	source := fs.String("source", "", "optional evidence source (free text accepted by replan validate)")
	url := fs.String("url", "", "optional evidence URL or local reference")
	actorID := fs.String("actor", "", "required id of an existing actor; also used as the audit actor")
	observedAt := fs.String("observed-at", "", "required RFC3339 timestamp with an offset")
	confidence := fs.String("confidence", "", "optional confidence (free text accepted by replan validate)")
	summary := fs.String("summary", "", "required evidence summary")
	var rawTargets repeatedStringFlag
	fs.Var(&rawTargets, "target", "required repeatable KIND:ID target; kinds: project, task, resource, resource_demand, actual, proposal, dependency, scenario, capacity, baseline, audit_record")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if strings.ToLower(*format) != "json" && strings.ToLower(*format) != "text" {
		env := envelope{OK: false, Command: "evidence add", ProjectPath: *projectPath, Result: nil, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{{Code: "unsupported_format", Message: "supported formats are json and text", Severity: "error"}}}
		if err := printEnvelope(stdout, "json", env); err != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", err)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	dryRun, code, done := requireCanonicalWriteChoice("evidence add", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	targets := make([]domain.Ref, 0, len(rawTargets))
	for _, raw := range rawTargets {
		kind, targetID, found := strings.Cut(raw, ":")
		if !found {
			kind = raw
			targetID = ""
		}
		targets = append(targets, domain.Ref{Kind: kind, ID: targetID})
	}
	result, err := app.NewEvidenceService(newStore(*projectPath)).Add(context.Background(), app.EvidenceAddRequest{
		ID: *id, Type: *evidenceType, Subtype: *subtype, Source: *source, URL: *url,
		ActorID: *actorID, ObservedAt: *observedAt, Confidence: *confidence, Summary: *summary,
		Targets: targets, DryRun: dryRun,
	})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "evidence add", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if *format == "text" {
		if err != nil {
			if printErr := printEnvelope(stdout, "text", env); printErr != nil {
				fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
				return exitGeneralError
			}
			if diagnosticsContainNotFound(result.Errors) {
				return exitNotFound
			}
			return exitValidationFailed
		}
		if printErr := writeEvidenceAddText(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func writeEvidenceAddText(stdout *os.File, result app.EvidenceAddResult) error {
	data, err := yaml.Marshal(result.Evidence)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "Evidence file: %s\nDry run: %t\nApplied: %t\n\n%s", result.FilePath, result.DryRun, result.Applied, data)
	return err
}

func runActualList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("actual list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	actualType := fs.String("type", "", "filter by actual type")
	taskID := fs.String("task", "", "filter by task id")
	demandID := fs.String("demand", "", "filter by resource demand id")
	resourceID := fs.String("resource", "", "filter by resource id")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewActualService(newStore(*projectPath)).List(context.Background(), app.ActualListRequest{
		Type:       *actualType,
		TaskID:     *taskID,
		DemandID:   *demandID,
		ResourceID: *resourceID,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "actual list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeActualListMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runActualAddConsumption(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("actual add-consumption")
	projectPath, format := addProjectFormatFlags(fs, "json")
	taskID := fs.String("task", "", "task id")
	demandID := fs.String("demand", "", "resource demand id")
	resourceID := fs.String("resource", "", "resource id")
	amount := fs.Float64("amount", 0, "consumed amount")
	unit := fs.String("unit", "", "consumed amount unit")
	evidenceID := fs.String("evidence", "", "evidence id")
	actorID := fs.String("actor", "", "actor id accepting the actual")
	occurredAt := fs.String("occurred-at", "", "RFC3339 occurrence timestamp")
	statusDate := fs.String("status-date", "", "status date in YYYY-MM-DD format")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("actual add-consumption", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewActualService(newStore(*projectPath)).AddConsumption(context.Background(), app.ActualAddConsumptionRequest{
		TaskID:     *taskID,
		DemandID:   *demandID,
		ResourceID: *resourceID,
		Amount:     *amount,
		Unit:       *unit,
		EvidenceID: *evidenceID,
		ActorID:    *actorID,
		OccurredAt: *occurredAt,
		StatusDate: *statusDate,
		DryRun:     dryRun,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "actual add-consumption",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeActualAddConsumptionDryRunMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runActualSetRemaining(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("actual set-remaining")
	projectPath, format := addProjectFormatFlags(fs, "json")
	demandID := fs.String("demand", "", "resource demand id")
	amount := fs.Float64("amount", -1, "remaining amount")
	unit := fs.String("unit", "", "remaining amount unit")
	reason := fs.String("reason", "", "reason for remaining-work update")
	evidenceID := fs.String("evidence", "", "evidence id")
	actorID := fs.String("actor", "", "actor id accepting the update")
	occurredAt := fs.String("occurred-at", "", "RFC3339 occurrence timestamp")
	statusDate := fs.String("status-date", "", "status date in YYYY-MM-DD format")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("actual set-remaining", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewActualService(newStore(*projectPath)).SetRemaining(context.Background(), app.ActualSetRemainingRequest{
		DemandID:   *demandID,
		Amount:     *amount,
		Unit:       *unit,
		Reason:     *reason,
		EvidenceID: *evidenceID,
		ActorID:    *actorID,
		OccurredAt: *occurredAt,
		StatusDate: *statusDate,
		DryRun:     dryRun,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "actual set-remaining",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeActualSetRemainingDryRunMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runActualCloseDemand(args []string, stdout, stderr *os.File) int {
	spec := actualLifecycleCommandSpec{"actual close-demand", "demand", "resource demand id", "new closed demand status: consumed or cancelled", "reason for demand lifecycle update"}
	return runActualLifecycleCommand(args, stdout, stderr, spec, func(projectPath string, flags actualLifecycleFlags, dryRun bool) (app.ActualCloseDemandResult, error) {
		return app.NewActualService(newStore(projectPath)).CloseDemand(context.Background(), app.ActualCloseDemandRequest{
			DemandID: *flags.targetID, Status: *flags.status, Reason: *flags.reason, EvidenceID: *flags.evidenceID,
			ActorID: *flags.actorID, OccurredAt: *flags.occurredAt, StatusDate: *flags.statusDate, DryRun: dryRun,
		})
	}, func(result app.ActualCloseDemandResult) ([]app.Diagnostic, []app.Diagnostic) {
		return result.Warnings, result.Errors
	}, writeActualCloseDemandDryRunMarkdown)
}

func runActualAddStatus(args []string, stdout, stderr *os.File) int {
	spec := actualLifecycleCommandSpec{"actual add-status", "task", "task id", "tracking status", "reason for task-status update"}
	return runActualLifecycleCommand(args, stdout, stderr, spec, func(projectPath string, flags actualLifecycleFlags, dryRun bool) (app.ActualAddStatusResult, error) {
		return app.NewActualService(newStore(projectPath)).AddStatus(context.Background(), app.ActualAddStatusRequest{
			TaskID: *flags.targetID, Status: *flags.status, Reason: *flags.reason, EvidenceID: *flags.evidenceID,
			ActorID: *flags.actorID, OccurredAt: *flags.occurredAt, StatusDate: *flags.statusDate, DryRun: dryRun,
		})
	}, func(result app.ActualAddStatusResult) ([]app.Diagnostic, []app.Diagnostic) {
		return result.Warnings, result.Errors
	}, writeActualAddStatusDryRunMarkdown)
}

type actualLifecycleCommandSpec struct {
	command, targetName, targetUsage, statusUsage, reasonUsage string
}

func runActualLifecycleCommand[T any](args []string, stdout, stderr *os.File, spec actualLifecycleCommandSpec, execute func(string, actualLifecycleFlags, bool) (T, error), diagnostics func(T) ([]app.Diagnostic, []app.Diagnostic), writeMarkdown func(*os.File, T) error) int {
	fs := newCLIFlagSet(spec.command)
	projectPath, format := addProjectFormatFlags(fs, "json")
	flags := addActualLifecycleFlags(fs, spec.targetName, spec.targetUsage, spec.statusUsage, spec.reasonUsage)
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice(spec.command, *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := execute(*projectPath, flags, dryRun)
	warnings, errorsOut := diagnostics(result)
	env := envelope{OK: err == nil && len(errorsOut) == 0, Command: spec.command, ProjectPath: *projectPath, Result: result, Warnings: warnings, Errors: errorsOut}
	return printCommandResult(stdout, stderr, *format, result, env, err, writeMarkdown)
}

type actualLifecycleFlags struct {
	targetID   *string
	status     *string
	reason     *string
	evidenceID *string
	actorID    *string
	occurredAt *string
	statusDate *string
}

func addActualLifecycleFlags(fs *flag.FlagSet, targetName, targetUsage, statusUsage, reasonUsage string) actualLifecycleFlags {
	return actualLifecycleFlags{
		targetID:   fs.String(targetName, "", targetUsage),
		status:     fs.String("status", "", statusUsage),
		reason:     fs.String("reason", "", reasonUsage),
		evidenceID: fs.String("evidence", "", "evidence id"),
		actorID:    fs.String("actor", "", "actor id accepting the update"),
		occurredAt: fs.String("occurred-at", "", "RFC3339 occurrence timestamp"),
		statusDate: fs.String("status-date", "", "status date in YYYY-MM-DD format"),
	}
}

func runProposal(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing proposal subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runProposalList(args[1:], stdout, stderr)
	case "show":
		return runProposalShow(args[1:], stdout, stderr)
	case "suggest-dependencies":
		return runProposalSuggestDependencies(args[1:], stdout, stderr)
	case "accept":
		return runProposalAccept(args[1:], stdout, stderr)
	case "reject":
		return runProposalReject(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown proposal subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runProposalList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("proposal list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	status := fs.String("status", "", "filter by proposal status")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewProposalService(newStore(*projectPath)).List(context.Background(), app.ProposalListRequest{Status: *status})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printCommandResult(stdout, stderr, *format, result, env, err, writeProposalListMarkdown)
}

func runProposalShow(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("proposal show")
	projectPath, format := addProjectFormatFlags(fs, "json")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID = positionalArg(positionalID, fs)
	if positionalID == "" {
		fmt.Fprintln(stderr, "missing proposal id")
		return exitInvalidUsage
	}
	result, err := app.NewProposalService(newStore(*projectPath)).Show(context.Background(), app.ProposalShowRequest{ID: positionalID})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal show",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeProposalShowMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runProposalSuggestDependencies(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("proposal suggest-dependencies")
	projectPath, format := addProjectFormatFlags(fs, "json")
	actorID := fs.String("actor", "", "actor id preparing proposal candidates")
	dryRun := fs.Bool("dry-run", true, "preview dependency proposal candidates without writing state")
	writeCandidates := fs.Bool("write-candidates", false, "write dependency proposal candidate files without mutating canonical state")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if !*dryRun && !*writeCandidates {
		fmt.Fprintln(stderr, "proposal suggest-dependencies is preview-only; --dry-run=false is not supported")
		return exitInvalidUsage
	}
	result, err := app.NewProposalService(newStore(*projectPath)).SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID:         *actorID,
		WriteCandidates: *writeCandidates,
		Now:             time.Now(),
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal suggest-dependencies",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeProposalSuggestDependenciesMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runProposalAccept(args []string, stdout, stderr *os.File) int {
	return runProposalDecision(args, stdout, stderr, "accept")
}

func runProposalReject(args []string, stdout, stderr *os.File) int {
	return runProposalDecision(args, stdout, stderr, "reject")
}

func runProposalDecision(args []string, stdout, stderr *os.File, decision string) int {
	fs := newCLIFlagSet("proposal " + decision)
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	actorID := fs.String("actor", "", "actor id performing proposal decision; required unless --dry-run is used")
	reason := fs.String("reason", "", "decision reason")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("proposal "+decision, *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	positionalID = positionalArg(positionalID, fs)
	if positionalID == "" {
		fmt.Fprintln(stderr, "missing proposal id")
		return exitInvalidUsage
	}
	req := app.ProposalDecisionRequest{
		ID:      positionalID,
		ActorID: *actorID,
		Reason:  *reason,
		DryRun:  dryRun,
		Now:     time.Now(),
	}
	service := app.NewProposalService(newStore(*projectPath))
	var (
		result app.ProposalDecisionResult
		err    error
	)
	if decision == "accept" {
		result, err = service.Accept(context.Background(), req)
	} else {
		result, err = service.Reject(context.Background(), req)
	}
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal " + decision,
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeProposalDecisionMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runAudit(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing audit subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runAuditList(args[1:], stdout, stderr)
	case "verify":
		return runAuditVerify(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown audit subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runAuditList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("audit list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	since := fs.String("since", "", "include records at or after this timestamp/date prefix")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewAuditService(newStore(*projectPath)).List(context.Background(), app.AuditListRequest{Since: *since})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "audit list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printCommandResult(stdout, stderr, *format, result, env, err, writeAuditListMarkdown)
}

func printCommandResult[T any](stdout, stderr *os.File, format string, result T, env envelope, commandErr error, writeMarkdown func(*os.File, T) error) int {
	if format != "markdown" && format != "text" {
		return printScenarioEnvelope(stdout, stderr, format, env, commandErr)
	}
	if err := writeMarkdown(stdout, result); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError
	}
	if commandErr != nil {
		return validationFailureExit(env.Errors)
	}
	return exitSuccess
}

func runAuditVerify(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("audit verify")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewAuditService(newStore(*projectPath)).Verify(context.Background(), app.AuditVerifyRequest{})
	env := envelope{
		OK:          err == nil && result.Valid,
		Command:     "audit verify",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runScenario(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing scenario subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runScenarioList(args[1:], stdout, stderr)
	case "show":
		return runScenarioShow(args[1:], stdout, stderr)
	case "diff":
		return runScenarioDiff(args[1:], stdout, stderr)
	case "apply":
		return runScenarioApply(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown scenario subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runScenarioList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("scenario list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewScenarioService(newStore(*projectPath)).List(context.Background(), app.ScenarioListRequest{})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "scenario list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runScenarioShow(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("scenario show")
	projectPath, format := addProjectFormatFlags(fs, "json")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID = positionalArg(positionalID, fs)
	if positionalID == "" {
		fmt.Fprintln(stderr, "missing scenario id")
		return exitInvalidUsage
	}

	result, err := app.NewScenarioService(newStore(*projectPath)).Show(context.Background(), app.ScenarioShowRequest{ID: positionalID})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "scenario show",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runScenarioDiff(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("scenario diff")
	projectPath, format := addProjectFormatFlags(fs, "json")
	against := fs.String("against", "current", "comparison base")
	week := fs.String("week", "", "ISO week, for example 2026-W28")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID = positionalArg(positionalID, fs)
	if positionalID == "" {
		fmt.Fprintln(stderr, "missing scenario id")
		return exitInvalidUsage
	}

	if *week != "" && !validISOWeek(*week) {
		return printInvalidWeek(stdout, "scenario diff", *projectPath, *week)
	}

	result, err := app.NewScenarioService(newStore(*projectPath)).Diff(context.Background(), app.ScenarioDiffRequest{
		ID:      positionalID,
		Against: *against,
		Week:    *week,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "scenario diff",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeScenarioDiffMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runScenarioApply(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("scenario apply")
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	actorID := fs.String("actor", "", "actor id applying the scenario; required unless --dry-run is used")
	reason := fs.String("reason", "", "apply reason")
	evidenceID := fs.String("evidence", "", "evidence id of the decision; required when the scenario cancels a dependency")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("scenario apply", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	positionalID = positionalArg(positionalID, fs)
	if positionalID == "" {
		fmt.Fprintln(stderr, "missing scenario id")
		return exitInvalidUsage
	}
	result, err := app.NewScenarioService(newStore(*projectPath)).Apply(context.Background(), app.ScenarioApplyRequest{
		ID:         positionalID,
		ActorID:    *actorID,
		Reason:     *reason,
		EvidenceID: *evidenceID,
		DryRun:     dryRun,
		Now:        time.Now(),
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "scenario apply",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeScenarioApplyMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func addProjectFormatFlags(fs *flag.FlagSet, defaultFormat string) (*string, *string) {
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", defaultFormat, "output format: json, markdown, text")
	return projectPath, format
}

func splitSinglePositional(args []string, fs *flag.FlagSet) (string, []string) {
	positional := ""
	filtered := make([]string, 0, len(args))
	expectFlagValue := false
	for i, arg := range args {
		if expectFlagValue {
			filtered = append(filtered, arg)
			expectFlagValue = false
			continue
		}
		if arg == "--" {
			filtered = append(filtered, arg)
			if positional == "" && i+1 < len(args) {
				positional = args[i+1]
				filtered = append(filtered, args[i+2:]...)
				return positional, filtered
			}
			continue
		}
		if isFlagToken(arg) {
			filtered = append(filtered, arg)
			name, hasInlineValue := flagName(arg)
			if !hasInlineValue && flagRequiresValue(fs, name) {
				expectFlagValue = true
			}
			continue
		}
		if positional == "" {
			positional = arg
			continue
		}
		filtered = append(filtered, arg)
	}
	return positional, filtered
}

func isFlagToken(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func flagName(arg string) (string, bool) {
	name := strings.TrimLeft(arg, "-")
	if i := strings.Index(name, "="); i >= 0 {
		return name[:i], true
	}
	return name, false
}

func flagRequiresValue(fs *flag.FlagSet, name string) bool {
	flagValue := fs.Lookup(name)
	if flagValue == nil {
		return false
	}
	boolFlag, ok := flagValue.Value.(interface{ IsBoolFlag() bool })
	return !ok || !boolFlag.IsBoolFlag()
}

func positionalArg(leading string, fs *flag.FlagSet) string {
	if leading != "" {
		return leading
	}
	if fs.NArg() == 0 {
		return ""
	}
	return fs.Arg(0)
}

func printScenarioEnvelope(stdout, stderr *os.File, format string, env envelope, err error) int {
	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "text" {
		env.OK = false
		env.Errors = []app.Diagnostic{{
			Code:     "unsupported_format",
			Message:  "supported formats are json, markdown and text",
			Severity: "error",
		}}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	var printErr error
	if env.OK && format != "json" {
		switch result := env.Result.(type) {
		case app.ScenarioListResult:
			if format == "markdown" {
				printErr = writeScenarioListMarkdown(stdout, result)
			} else {
				printErr = writeScenarioListText(stdout, result)
			}
		case app.ScenarioShowResult:
			if format == "markdown" {
				printErr = writeScenarioShowMarkdown(stdout, result)
			} else {
				printErr = writeScenarioShowText(stdout, result)
			}
		default:
			printErr = printEnvelope(stdout, format, env)
		}
	} else {
		printErr = printEnvelope(stdout, format, env)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			if firstDiagnostic(env.Errors, "approver_not_human") != nil {
				return exitPolicyDenied
			}
			if diagnosticsContainPrivateOpenProjectIDs(env.Errors) {
				return exitInvalidUsage
			}
			if diagnosticsContainExternal(env.Errors) {
				return exitExternalSource
			}
			if diagnosticsContainNotFound(env.Errors) {
				return exitNotFound
			}
			return exitValidationFailed
		}
		return exitGeneralError
	}
	return exitSuccess
}

func diagnosticsContainPrivateOpenProjectIDs(diags []app.Diagnostic) bool {
	for _, diag := range diags {
		if diag.Code == "openproject_ids_private" {
			return true
		}
	}
	return false
}

// diagnosticsContainExternal reports a failure of the external source itself,
// OpenProject unreachable, refusing or answering garbage, as opposed to an
// invalid plan or sidecar (exit 6 in docs/spec/cli.md).
func diagnosticsContainExternal(diags []app.Diagnostic) bool {
	for _, diag := range diags {
		switch diag.Code {
		case "openproject_network_error",
			"openproject_request_error",
			"openproject_api_error",
			"openproject_authentication_failed",
			"openproject_permission_denied",
			"openproject_rate_limited",
			"openproject_malformed_response",
			"openproject_project_not_found",
			"source_unavailable",
			"source_unauthorized",
			"source_not_found",
			"source_rate_limited":
			return true
		}
	}
	return false
}

func diagnosticsContainNotFound(diags []app.Diagnostic) bool {
	// An unsupported format leaves the workspace empty, so the references a
	// command checks next are "not found"; the format is the one answer.
	if firstDiagnostic(diags, "project_format_unsupported") != nil {
		return false
	}
	for _, diag := range diags {
		if diagnosticIsNotFound(diag.Code) {
			return true
		}
	}
	return false
}

func validationFailureExit(diags []app.Diagnostic) int {
	if diagnosticsContainNotFound(diags) {
		return exitNotFound
	}
	return exitValidationFailed
}

func printProjectNotFound(stdout, stderr *os.File, format string, env envelope, commandErr error) (int, bool) {
	if commandErr == nil || firstDiagnostic(env.Errors, "project_not_found") == nil {
		return 0, false
	}
	if err := printEnvelope(stdout, format, env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitNotFound, true
}

func printProjectFormatUnsupported(stdout, stderr *os.File, format string, env envelope, commandErr error) (int, bool) {
	if commandErr == nil || firstDiagnostic(env.Errors, "project_format_unsupported") == nil {
		return 0, false
	}
	if err := printEnvelope(stdout, format, env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitValidationFailed, true
}

func diagnosticIsNotFound(code string) bool {
	switch code {
	case "project_not_found",
		"baseline_not_found",
		"calibration_not_found",
		"scenario_not_found",
		"proposal_not_found",
		"source_profile_not_found",
		"source_snapshot_not_found",
		"task_not_found",
		"demand_not_found",
		"resource_not_found",
		"assignee_not_found",
		"after_task_not_found",
		"after_task_not_in_current_plan",
		"actor_not_found",
		"evidence_not_found",
		"evidence_target_not_found":
		return true
	default:
		return false
	}
}

func printExportManifestRequiresOutput(stdout, stderr *os.File, command, projectPath string) int {
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Errors: []app.Diagnostic{{
			Code:     "manifest_requires_output",
			Message:  "--manifest requires --output so the manifest can describe a written artifact",
			Severity: "error",
		}},
		Warnings: []app.Diagnostic{},
	}
	if printErr := printEnvelope(stdout, "json", env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	return exitInvalidUsage
}

func runReport(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing report subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "weekly":
		return runReportWeekly(args[1:], stdout, stderr)
	case "estimates":
		return runReportEstimates(args[1:], stdout, stderr)
	case "state":
		return runReportState(args[1:], stdout, stderr)
	case "dashboard":
		return runReportDashboard(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown report subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runReportEstimates(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report estimates")
	var projectPath, baselineID, format string
	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&baselineID, "baseline", "", "baseline id")
	fs.StringVar(&format, "format", "markdown", "output format: json or markdown")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if baselineID == "" || (format != "json" && format != "markdown") {
		message := "--baseline is required"
		code := "missing_baseline"
		if baselineID != "" {
			message = "supported formats are json and markdown"
			code = "unsupported_format"
		}
		_ = printEnvelope(stdout, "json", envelope{OK: false, Command: "report estimates", ProjectPath: projectPath, Result: nil,
			Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{{Code: code, Message: message, Severity: "error"}}})
		return exitInvalidUsage
	}

	result, err := app.NewReportService(newStore(projectPath)).Estimates(context.Background(), app.EstimateReportRequest{BaselineID: baselineID})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "report estimates", ProjectPath: projectPath,
		Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}
	var printErr error
	if format == "json" {
		printErr = printEnvelope(stdout, "json", env)
	} else {
		printErr = writeEstimateReportMarkdown(stdout, result)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runReportState(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report state")

	var (
		projectPath string
		format      string
		week        string
		scenarioID  string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "markdown", "output format: json, markdown or text")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W31")
	fs.StringVar(&scenarioID, "scenario", "", "scenario id to apply as an in-memory overlay")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "text" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "report state",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "report state", projectPath, week)
	}

	store := newStore(projectPath)
	service := app.NewReportService(store)
	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		GeneratedAt: time.Now(),
		Command:     "report state",
		ScenarioID:  scenarioID,
	})
	env := envelope{
		OK:          err == nil && len(overview.Errors) == 0,
		Command:     "report state",
		ProjectPath: overview.ProjectPath,
		Result:      overview,
		Warnings:    reportIssuesToDiagnostics(overview.Warnings),
		Errors:      reportIssuesToDiagnostics(overview.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "markdown", "text":
		printErr = reports.WriteProjectStateMarkdown(stdout, overview)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(reportIssuesToDiagnostics(overview.Errors))
		}
		return exitGeneralError
	}
	return exitSuccess
}

// runNext prints the short operator view of report state: what waits on a
// person, what an agent can prepare, and one health line.
func runNext(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("next")

	var (
		projectPath string
		format      string
		week        string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "text", "output format: text or json")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W31")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "json" && format != "text" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "next",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are text and json",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}
	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "next", projectPath, week)
	}

	store := newStore(projectPath)
	overview, err := app.NewReportService(store).Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		GeneratedAt: time.Now(),
		Command:     "next",
	})
	brief := reports.BuildNextBrief(overview)
	env := envelope{
		OK:          err == nil && len(overview.Errors) == 0,
		Command:     "next",
		ProjectPath: overview.ProjectPath,
		Result:      brief,
		Warnings:    reportIssuesToDiagnostics(overview.Warnings),
		Errors:      reportIssuesToDiagnostics(overview.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "text":
		printErr = reports.WriteNextBriefText(stdout, brief)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(reportIssuesToDiagnostics(overview.Errors))
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runReportDashboard(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report dashboard")

	var (
		projectPath string
		format      string
		outputDir   string
		week        string
		scenarioID  string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "html", "output format: html or json")
	fs.StringVar(&outputDir, "output", "", "directory to write generated dashboard artifacts")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W31")
	fs.StringVar(&scenarioID, "scenario", "", "scenario id to apply as an in-memory overlay")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "html" && format != "json" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "report dashboard",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are html and json",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "report dashboard", projectPath, week)
	}
	if outputDir != "" {
		if code, done := refuseScenarioReportWrite("report dashboard", projectPath, scenarioID, outputDir, stdout, stderr); done {
			return code
		}
		if code, done := refuseBindingsOutputInsideProject("report dashboard", projectPath, outputDir, stdout, stderr); done {
			return code
		}
	}

	store := newStore(projectPath)
	service := app.NewReportService(store)
	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		GeneratedAt: time.Now(),
		Command:     "report dashboard",
		ScenarioID:  scenarioID,
	})
	env := envelope{
		OK:          err == nil && len(overview.Errors) == 0,
		Command:     "report dashboard",
		ProjectPath: overview.ProjectPath,
		Result:      overview,
		Warnings:    reportIssuesToDiagnostics(overview.Warnings),
		Errors:      reportIssuesToDiagnostics(overview.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}
	if code, done := printProjectFormatUnsupported(stdout, stderr, format, env, err); done {
		return code
	}

	if outputDir != "" {
		result, writeErr := writeDashboardArtifacts(projectPath, outputDir, overview)
		if writeErr != nil {
			fmt.Fprintf(stderr, "failed to write dashboard: %v\n", writeErr)
			return exitGeneralError
		}
		env.Result = result
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if errors.Is(err, app.ErrValidationFailed) {
				return validationFailureExit(env.Errors)
			}
			return exitGeneralError
		}
		return exitSuccess
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "html":
		printErr = reports.WriteDashboardHTML(stdout, overview)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(env.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runReportWeekly(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report weekly")

	var (
		projectPath string
		format      string
		week        string
		write       bool
		scenarioID  string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "markdown", "output format: json, markdown, csv, html, text")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W28")
	fs.BoolVar(&write, "write", false, "write generated report artifact into the project reports directory")
	fs.StringVar(&scenarioID, "scenario", "", "scenario id to apply as an in-memory overlay")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "csv" && format != "html" && format != "text" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "report weekly",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown, csv, html and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "report weekly", projectPath, week)
	}
	if write {
		if code, done := refuseScenarioReportWrite("report weekly", projectPath, scenarioID, "reports", stdout, stderr); done {
			return code
		}
		if code, done := refuseBindingsOutputInsideProject("report weekly", projectPath, "reports", stdout, stderr); done {
			return code
		}
	}

	store := newStore(projectPath)
	service := app.NewReportService(store)
	report, err := service.Weekly(context.Background(), app.WeeklyReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		ScenarioID:  scenarioID,
	})
	env := envelope{
		OK:          err == nil && len(report.Errors) == 0,
		Command:     "report weekly",
		ProjectPath: report.ProjectPath,
		Result:      report,
		Warnings:    reportIssuesToDiagnostics(report.Warnings),
		Errors:      reportIssuesToDiagnostics(report.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}
	if code, done := printProjectFormatUnsupported(stdout, stderr, format, env, err); done {
		return code
	}

	if write {
		outputPath, writeErr := writeWeeklyReportArtifact(projectPath, week, format, report)
		if writeErr != nil {
			fmt.Fprintf(stderr, "failed to write report: %v\n", writeErr)
			return exitGeneralError
		}
		env.Result = weeklyReportWriteResult{
			Report:     report,
			OutputPath: outputPath,
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if errors.Is(err, app.ErrValidationFailed) {
				return validationFailureExit(env.Errors)
			}
			return exitGeneralError
		}
		return exitSuccess
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "csv":
		printErr = reports.WriteWeeklyCSV(stdout, report)
	case "html":
		printErr = reports.WriteWeeklyHTML(stdout, report)
	case "markdown", "text":
		printErr = reports.WriteWeeklyMarkdown(stdout, report)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(env.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func writeWeeklyReportArtifact(projectPath, week, format string, report reports.WeeklyReport) (string, error) {
	if week == "" {
		week = "unspecified-week"
	}
	var b strings.Builder
	extension := format
	switch format {
	case "json":
		content, err := jsonBytes(envelope{
			OK:          len(report.Errors) == 0,
			Command:     "report weekly",
			ProjectPath: report.ProjectPath,
			Result:      report,
			Warnings:    reportIssuesToDiagnostics(report.Warnings),
			Errors:      reportIssuesToDiagnostics(report.Errors),
		})
		if err != nil {
			return "", err
		}
		b.Write(content)
	case "csv":
		if err := reports.WriteWeeklyCSV(&b, report); err != nil {
			return "", err
		}
	case "html":
		if err := reports.WriteWeeklyHTML(&b, report); err != nil {
			return "", err
		}
	case "markdown", "text":
		extension = "md"
		if err := reports.WriteWeeklyMarkdown(&b, report); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported report artifact format %s", format)
	}
	relPath := filepath.ToSlash(filepath.Join("reports", "weekly", week+"."+extension))
	outputPath := filepath.Join(projectPath, filepath.FromSlash(relPath))
	if err := writeTextFile(outputPath, b.String()); err != nil {
		return "", err
	}
	return relPath, nil
}

func writeDashboardArtifacts(projectPath, outputDir string, overview reports.DashboardOverview) (dashboardReportWriteResult, error) {
	baseDir := outputDir
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(projectPath, filepath.FromSlash(outputDir))
	}
	publicationOverview, err := dashboardPublicationOverview(overview)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts := map[string][]byte{}
	var htmlOut strings.Builder
	if err := reports.WriteDashboardHTML(&htmlOut, publicationOverview); err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts["index.html"] = []byte(htmlOut.String())
	var ganttOut strings.Builder
	if err := reports.WriteDashboardGanttHTML(&ganttOut, publicationOverview); err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts["gantt.html"] = []byte(ganttOut.String())
	pageWriters := []struct {
		path  string
		write func(*strings.Builder) error
	}{
		{path: "workload.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardWorkloadHTML(out, publicationOverview.WorkloadPage)
		}},
		{path: "scenarios.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardScenariosHTML(out, publicationOverview.ScenariosPage)
		}},
		{path: "proposals.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardProposalsHTML(out, publicationOverview.ProposalsPage)
		}},
		{path: "estimates.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardEstimatesHTML(out, publicationOverview.EstimatesPage)
		}},
	}
	for _, page := range pageWriters {
		var out strings.Builder
		if err := page.write(&out); err != nil {
			return dashboardReportWriteResult{}, err
		}
		artifacts[page.path] = []byte(out.String())
	}

	overviewBytes, err := jsonBytes(publicationOverview)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts[filepath.ToSlash(filepath.Join("raw", "overview.json"))] = overviewBytes
	ganttBytes, err := jsonBytes(publicationOverview.Gantt)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts[filepath.ToSlash(filepath.Join("raw", "gantt.json"))] = ganttBytes
	openProjectOverlayBytes, err := jsonBytes(publicationOverview.OpenProjectOverlay)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts[filepath.ToSlash(filepath.Join("raw", "openproject_overlay.json"))] = openProjectOverlayBytes
	pageJSON := []struct {
		path  string
		value any
	}{
		{path: "workload.json", value: publicationOverview.WorkloadPage},
		{path: "scenarios.json", value: publicationOverview.ScenariosPage},
		{path: "proposals.json", value: publicationOverview.ProposalsPage},
		{path: "estimates.json", value: publicationOverview.EstimatesPage},
	}
	for _, page := range pageJSON {
		content, marshalErr := jsonBytes(page.value)
		if marshalErr != nil {
			return dashboardReportWriteResult{}, marshalErr
		}
		artifacts[filepath.ToSlash(filepath.Join("raw", page.path))] = content
	}

	manifest := reports.DashboardManifest{
		SchemaVersion: 1,
		GeneratedAt:   publicationOverview.GeneratedAt,
		Command:       "replan report dashboard",
		ProjectPath:   publicationOverview.ProjectPath,
		ProjectID:     publicationOverview.ProjectID,
		StatusDate:    publicationOverview.StatusDate,
		ScenarioID:    publicationOverview.ScenarioID,
		Artifacts:     []reports.DashboardManifestArtifact{},
		WarningsCount: len(publicationOverview.Warnings),
		ErrorsCount:   len(publicationOverview.Errors),
	}
	artifactPaths := []string{}
	for rel, content := range artifacts {
		artifactPaths = append(artifactPaths, rel)
		manifest.Artifacts = append(manifest.Artifacts, reports.DashboardManifestArtifact{
			Path:   rel,
			SHA256: sha256Hex(content),
		})
	}
	sort.Strings(artifactPaths)
	sort.Slice(manifest.Artifacts, func(i, j int) bool { return manifest.Artifacts[i].Path < manifest.Artifacts[j].Path })
	manifest.Artifacts = append(manifest.Artifacts, reports.DashboardManifestArtifact{Path: filepath.ToSlash(filepath.Join("raw", "manifest.json"))})
	manifestBytes, err := jsonBytes(manifest)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	manifestRel := filepath.ToSlash(filepath.Join("raw", "manifest.json"))
	artifacts[manifestRel] = manifestBytes
	artifactPaths = append(artifactPaths, manifestRel)
	sort.Strings(artifactPaths)

	for _, rel := range artifactPaths {
		if err := writeTextFile(filepath.Join(baseDir, filepath.FromSlash(rel)), string(artifacts[rel])); err != nil {
			return dashboardReportWriteResult{}, err
		}
	}
	return dashboardReportWriteResult{
		Overview:     overview,
		OutputDir:    filepath.ToSlash(outputDir),
		Artifacts:    artifactPaths,
		ManifestPath: manifestRel,
	}, nil
}

func dashboardPublicationOverview(overview reports.DashboardOverview) (reports.DashboardOverview, error) {
	if overview.ProjectPath == "" {
		return overview, nil
	}
	content, err := json.Marshal(overview)
	if err != nil {
		return reports.DashboardOverview{}, err
	}
	encodedPath, err := json.Marshal(overview.ProjectPath)
	if err != nil {
		return reports.DashboardOverview{}, err
	}
	encodedPlaceholder, err := json.Marshal("<project>")
	if err != nil {
		return reports.DashboardOverview{}, err
	}
	// Replace the path only where it stands as a path of its own, with no
	// path character next to it (a slash may follow). A bare replace of a
	// short path such as "." would rewrite every dot, numbers included.
	inner := regexp.QuoteMeta(string(encodedPath[1 : len(encodedPath)-1]))
	standalone := regexp.MustCompile(`(^|[^A-Za-z0-9._~/-])` + inner + `($|[^A-Za-z0-9._~-])`)
	placeholder := string(encodedPlaceholder[1 : len(encodedPlaceholder)-1])
	for {
		replaced := standalone.ReplaceAll(content, []byte("${1}"+placeholder+"${2}"))
		if bytes.Equal(replaced, content) {
			break
		}
		content = replaced
	}
	var publicationOverview reports.DashboardOverview
	if err := json.Unmarshal(content, &publicationOverview); err != nil {
		return reports.DashboardOverview{}, err
	}
	publicationOverview.WorkloadPage = overview.WorkloadPage
	publicationOverview.ScenariosPage = overview.ScenariosPage
	publicationOverview.ProposalsPage = overview.ProposalsPage
	publicationOverview.EstimatesPage = overview.EstimatesPage
	return publicationOverview, nil
}

func runWorkload(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("workload")

	var (
		projectPath   string
		format        string
		week          string
		resourceID    string
		resourceClass string
		unit          string
		scenarioID    string
		withDemands   bool
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "json", "output format: json, markdown, text")
	fs.BoolVar(&withDemands, "demands", false, "list in every row the demands that make it up")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W28; without it, totals over all weeks")
	fs.StringVar(&resourceID, "resource", "", "filter by resource id")
	fs.StringVar(&resourceClass, "resource-class", "", "keep only rows of this resource class, for example human")
	fs.StringVar(&unit, "unit", "", "keep only rows in this unit, for example hours")
	fs.StringVar(&scenarioID, "scenario", "", "scenario id to apply as an in-memory overlay")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}

	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "text" {
		printEnvelope(stdout, format, envelope{
			OK:          false,
			Command:     "workload",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "workload", projectPath, week)
	}

	store := newStore(projectPath)
	service := app.NewWorkloadService(store)
	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath:   projectPath,
		Week:          week,
		ResourceID:    resourceID,
		ResourceClass: resourceClass,
		Unit:          unit,
		ScenarioID:    scenarioID,
		WithDemands:   withDemands,
	})

	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "workload",
		ProjectPath: result.ProjectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	var printErr error
	switch format {
	case "markdown":
		printErr = writeWorkloadMarkdown(stdout, result)
	case "text":
		printErr = writeWorkloadText(stdout, result)
	default:
		printErr = printEnvelope(stdout, format, env)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runValidate(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("validate")

	var (
		projectPath string
		format      string
		strict      bool
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "text", "output format: json, markdown, text")
	fs.BoolVar(&strict, "strict", false, "treat validation warnings as errors")

	positionalProject := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalProject = args[0]
		args = args[1:]
	}

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}

	if projectPath == "" && positionalProject != "" {
		projectPath = positionalProject
	}
	if projectPath == "" && fs.NArg() > 0 {
		projectPath = fs.Arg(0)
	}
	if projectPath == "" {
		projectPath = "."
	}

	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "text" {
		printEnvelope(stdout, format, envelope{
			OK:          false,
			Command:     "validate",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	store := newStore(projectPath)
	service := app.NewValidateService(store)
	result, err := service.Validate(context.Background(), app.ValidateRequest{
		ProjectPath:       projectPath,
		WarningsAsErrors:  strict,
		IncludeObjectScan: true,
	})

	env := envelope{
		OK:          err == nil && result.Valid,
		Command:     "validate",
		ProjectPath: result.ProjectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}

	if printErr := printEnvelope(stdout, format, env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}

	if err == nil && result.Valid {
		return exitSuccess
	}
	if errors.Is(err, app.ErrValidationFailed) || !result.Valid {
		return validationFailureExit(result.Errors)
	}
	return exitGeneralError
}

func printEnvelope(stdout *os.File, format string, env envelope) error {
	if projectNotFound := firstDiagnostic(env.Errors, "project_not_found"); projectNotFound != nil {
		env.OK = false
		env.Errors = []app.Diagnostic{*projectNotFound}
	} else if unsupportedFormat := firstDiagnostic(env.Errors, "project_format_unsupported"); unsupportedFormat != nil {
		env.OK = false
		env.Errors = []app.Diagnostic{*unsupportedFormat}
	}
	switch format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	case "markdown":
		fmt.Fprintf(stdout, "# RePlan %s\n\n", env.Command)
		if env.OK {
			fmt.Fprintln(stdout, "Status: OK")
		} else {
			fmt.Fprintln(stdout, "Status: FAILED")
		}
		if env.ProjectPath != "" {
			fmt.Fprintf(stdout, "\nProject: `%s`\n", env.ProjectPath)
		}
		if len(env.Errors) > 0 {
			fmt.Fprintln(stdout, "\n## Errors")
			for _, d := range env.Errors {
				fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message)
			}
		}
		if len(env.Warnings) > 0 {
			fmt.Fprintln(stdout, "\n## Warnings")
			for _, d := range env.Warnings {
				fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message)
			}
		}
		return nil
	default:
		if env.OK {
			fmt.Fprintf(stdout, "%s ok\n", env.Command)
		} else {
			fmt.Fprintf(stdout, "%s failed\n", env.Command)
		}
		for _, d := range env.Errors {
			fmt.Fprintf(stdout, "error %s: %s\n", d.Code, d.Message)
		}
		for _, d := range env.Warnings {
			fmt.Fprintf(stdout, "warning %s: %s\n", d.Code, d.Message)
		}
		return nil
	}
}

func firstDiagnostic(diags []app.Diagnostic, code string) *app.Diagnostic {
	for i := range diags {
		if diags[i].Code == code {
			return &diags[i]
		}
	}
	return nil
}

type exportTaskJugglerCLIResult struct {
	Metadata   app.ExportMetadata `json:"metadata"`
	Scenario   string             `json:"scenario,omitempty"`
	Content    string             `json:"content,omitempty"`
	OutputPath string             `json:"output_path,omitempty"`
	Warnings   []app.Diagnostic   `json:"warnings"`
	Errors     []app.Diagnostic   `json:"errors"`
}

func printExportTaskJugglerJSON(stdout *os.File, result app.ExportTaskJugglerResult, outputPath string) error {
	content := result.Content
	if outputPath != "" {
		content = ""
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(exportTaskJugglerCLIResult{
		Metadata:   result.Metadata,
		Scenario:   result.Scenario,
		Content:    content,
		OutputPath: outputPath,
		Warnings:   result.Warnings,
		Errors:     result.Errors,
	})
}

func jsonBytes(value any) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

func exportManifestID(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, slugIDForManifest(part))
		}
	}
	if len(cleaned) == 0 {
		return "export.manifest." + time.Now().UTC().Format("20060102T150405Z")
	}
	return "export.manifest." + strings.Join(cleaned, ".") + "." + time.Now().UTC().Format("20060102T150405Z")
}

func slugIDForManifest(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "_", "-")
	value = strings.ToLower(replacer.Replace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func writeExportManifestFile(path string, manifest ExportManifest) error {
	if manifest.CreatedAt == "" {
		manifest.CreatedAt = time.Now().Format(time.RFC3339)
	}
	if manifest.Warnings == nil {
		manifest.Warnings = []app.Diagnostic{}
	}
	if manifest.Errors == nil {
		manifest.Errors = []app.Diagnostic{}
	}
	content, err := jsonBytes(manifest)
	if err != nil {
		return err
	}
	return writeTextFile(path, string(content))
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func writeTextFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

// validISOWeek reports whether week is a well-formed ISO week label such as
// 2026-W31. An empty string is not well-formed; callers that treat an empty
// week as "use the project default" must check emptiness separately.
func validISOWeek(week string) bool {
	if len(week) != 8 || week[4] != '-' || week[5] != 'W' {
		return false
	}
	year, err := strconv.Atoi(week[0:4])
	if err != nil || year < 1 {
		return false
	}
	number, err := strconv.Atoi(week[6:8])
	if err != nil || number < 1 || number > 53 {
		return false
	}
	return true
}

// printInvalidWeek emits a stable JSON envelope for a malformed --week value and
// returns the invalid-usage exit code.
func printInvalidWeek(stdout *os.File, command, projectPath, week string) int {
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Warnings:    []app.Diagnostic{},
		Errors: []app.Diagnostic{{
			Code:     "invalid_week",
			Message:  fmt.Sprintf("week %q is not a valid ISO week label; expected YYYY-Www such as 2026-W31", week),
			Severity: "error",
		}},
	}
	_ = printEnvelope(stdout, "json", env)
	return exitInvalidUsage
}

func printUsage(out *os.File) {
	fmt.Fprintln(out, "Usage:")
	fmt.Fprintln(out, "  Workspace-loading commands accept [--bindings <path>] (or REPLAN_OPENPROJECT_BINDINGS_LAYER; flag wins).")
	fmt.Fprintln(out, "  replan version [--format text|json]")
	fmt.Fprintln(out, "  replan init <dir> --name <name> --owner <display-name> [--id <project-id>] [--timezone <IANA-zone>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan validate [project-path] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan workload --project <path> [--week <YYYY-Www>] [--resource <id>] [--resource-class <class>] [--unit <unit>] [--scenario <id>] [--demands] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan report weekly --project <path> --week <YYYY-Www> [--format json|markdown|csv|html] [--write]")
	fmt.Fprintln(out, "  replan report estimates --project <path> --baseline <id> [--format json|markdown]")
	fmt.Fprintln(out, "  replan estimate calibrate --project <path> --baseline <id> [--baseline <id> ...] --resource <id> [--resource <id> ...] --unit <unit> [--tag <tag>] [--exclude <task-id>=<reason> ...] --id <id> --name <name> --approved-by <actor-id> (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan estimate list --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan estimate show --project <path> <id> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan estimate apply --project <path> --calibration <id> --scenario-id <id> --actor <id> (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan next --project <path> [--week <YYYY-Www>] [--format text|json]")
	fmt.Fprintln(out, "  replan project migrate --project <path> --actor <id> [--reason <text>] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan project set-status-date --project <path> --date <YYYY-MM-DD|today> --actor <id> [--reason <text>] (--dry-run|--apply) [--format json|markdown]")
	fmt.Fprintln(out, "  replan report state --project <path> [--week <YYYY-Www>] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan report dashboard --project <path> [--week <YYYY-Www>] [--output <dir>] [--format html|json]")
	fmt.Fprintln(out, "  replan tracking suggest --project <path> [--week <YYYY-Www>] [--write-candidates --actor <id>] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan scenario list|show|diff|apply --project <path> [--dry-run|--apply for apply] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan proposal list|show|suggest-dependencies|accept|reject --project <path> [--dry-run|--apply for accept/reject] [--format json|markdown]")
	fmt.Fprintln(out, "  replan dependency list --project <path> [--format json|markdown]")
	fmt.Fprintln(out, "  replan baseline create --project <path> --id <id> --name <name> --approved-by <actor-id> [--status-date <YYYY-MM-DD>] [--assumption <text> ...] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan baseline list --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan baseline show <id> --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan audit list|verify --project <path> [--format json|markdown]")
	fmt.Fprintln(out, "  replan actual list|add-consumption|set-remaining|close-demand|reopen-demand|acknowledge-overrun|add-status --project <path> [--dry-run|--apply for mutations] [--format json|markdown]")
	fmt.Fprintln(out, "  replan evidence add --project <path> --id <id> --type <type> [--subtype <subtype>] [--source <source>] [--url <url>] --actor <id> --observed-at <RFC3339> [--confidence <confidence>] --summary <text> --target <kind:id> [--target <kind:id> ...] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan resource add --project <path> --id <resource-id> --class <class> --name <name> [--capability <name> ...] [--with-actor <trust-level>] --actor <id> [--evidence <id>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan resource list --project <path> [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan capacity set --project <path> --resource <resource-id> --unit <unit> --amount <n> [--period week] [--effective-from <YYYY-MM-DD>] --actor <id> [--evidence <id>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan task add --project <path> --id <task-id> --title <title> [--description <text>] [--type <type>] [--planned-start <YYYY-MM-DD>] [--planned-finish <YYYY-MM-DD>] [--tag <tag> ...] [--assignee <resource-id>] [--after <task-id>] --demand <SUFFIX=RESOURCE_ID:AMOUNT:UNIT[:ROLE]> [--demand ...] --actor <id> --evidence <id> [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text] (does not create an OpenProject work package; next use scripts/apply-openproject-bootstrap.sh or toolkit work-package create + bind)")
	fmt.Fprintln(out, "  replan task set-dates --project <path> --task <task-id> [--planned-start <YYYY-MM-DD>] [--planned-finish <YYYY-MM-DD>] --reason <text> --actor <id> --evidence <id> [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text] (push to OpenProject separately with scripts/push-openproject-date-drift.sh)")
	fmt.Fprintln(out, "  replan task record-actual-window --project <path> --task <task-id> --actor <id> --evidence <id> [--reason <text>] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan task set-title --project <path> --task <task-id> --title <title> --reason <text> --actor <id> --evidence <id> [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|text]")
	fmt.Fprintln(out, "  replan import github --project <path> (--from-file <path>|--profile <id>|--repository <owner/repo>) [--token-env <env>] [--base-url <url>] [--since <date>] [--include-closed] [--include-relations] [--resource-links-sidecar <path>] [--resource-demands-sidecar <path>] [--request-timeout <seconds>] [--request-retries <n>] [--request-retry-delay <seconds>] [--dry-run] [--format json|markdown]")
	fmt.Fprintln(out, "  replan import openproject --project <path> (--from-file <path>|--profile <id>|--base-url <url> --project-id <id> --token-env <env>) [--work-package <id>] [--since <date>] [--include-closed] [--include-relations] [--include-time-entries] [--resource-demands-sidecar <path>] [--resource-links-sidecar <path>] [--page-size <n>] [--request-timeout <seconds>] [--request-retries <n>] [--request-retry-delay <seconds>] [--host-header <host>] [--forwarded-proto <proto>] [--perimeter-key-env <env>] [--perimeter-header <name>] [--dry-run] [--format json|markdown]")
	fmt.Fprintln(out, "  replan import accept <source-snapshot-id> --project <path> (--dry-run|--apply --actor <id> [--reason <text>]) [--accept-dates|--accept-dependencies|--accept-tracking-status|--accept-resource-links|--accept-resource-demands|--accept-time-entries] [--format json|markdown] (tracker candidates; time entries currently require an OpenProject snapshot)")
	fmt.Fprintln(out, "  replan export json --project <path> [--output <path>] [--manifest <path>] [--format json]")
	fmt.Fprintln(out, "  replan export taskjuggler --project <path> [--scenario <id>] [--output <path>] [--manifest <path>] [--format text|json] [--unrepresented-assumptions warn|error]")
	fmt.Fprintln(out, "  replan export csv --project <path> --report workload --week <YYYY-Www> [--scenario <id>] [--output <path>] [--manifest <path>]")
	fmt.Fprintln(out, "  replan taskjuggler run --project <path> [--scenario <id>] [--output-dir <path>] [--taskjuggler-bin tj3] [--unrepresented-assumptions warn|error] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan taskjuggler import-result --project <path> --run-dir <path> (--dry-run|--write-candidate --actor <id>) [--scenario-id <id>] [--format json|markdown|text]")
	fmt.Fprintln(out, "  replan dev validate-taskjuggler --project <path> [--scenario <id>] [--taskjuggler-bin tj3] [--keep-temp] [--format json|markdown|text]")
}

func writeBaselineCreateText(stdout *os.File, result app.BaselineCreateResult) error {
	mode := "Dry run"
	if result.Applied {
		mode = "Applied"
	}
	if _, err := fmt.Fprintf(stdout, "Baseline create: %s\nFile: %s\nTasks: %d\nDemands: %d\n\nPreview:\n", mode, result.FilePath, result.TaskCount, result.DemandCount); err != nil {
		return err
	}
	data, err := yaml.Marshal(struct {
		Baseline domain.Baseline `yaml:"baseline"`
	}{Baseline: result.Baseline})
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	const previewLines = 24
	if len(lines) > previewLines {
		lines = append(lines[:previewLines], fmt.Sprintf("... (%d more lines)", len(lines)-previewLines))
	}
	_, err = fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	return err
}

func writeBaselineListMarkdown(stdout *os.File, result app.BaselineListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Baselines"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		for _, diagnostic := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if len(result.Baselines) == 0 {
		_, err := fmt.Fprintln(stdout, "No baselines.")
		return err
	}
	if _, err := fmt.Fprintln(stdout, "| ID | Status date | Tasks | Demands | Estimates | Seal |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "|---|---|---:|---:|---|---|"); err != nil {
		return err
	}
	for _, baseline := range result.Baselines {
		if _, err := fmt.Fprintf(stdout, "| `%s` | %s | %d | %d | %t | %s |\n", baseline.ID, emptyDash(baseline.StatusDate), baseline.Tasks, baseline.Demands, baseline.CarriesEstimates, baseline.SealStatus); err != nil {
			return err
		}
	}
	return nil
}

func writeBaselineShowMarkdown(stdout *os.File, result app.BaselineShowResult) error {
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "# Baseline"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
		for _, diagnostic := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "# Baseline: %s\n\n- Estimates: `%t`\n- Seal: `%s`\n\n```yaml\n", result.Baseline.ID, result.CarriesEstimates, result.SealStatus); err != nil {
		return err
	}
	data, err := yaml.Marshal(struct {
		Baseline domain.Baseline `yaml:"baseline"`
	}{Baseline: result.Baseline})
	if err != nil {
		return err
	}
	if _, err := stdout.Write(data); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "```")
	return err
}

func writeTrackingSuggestMarkdown(stdout *os.File, result app.TrackingSuggestResult) error {
	if _, err := fmt.Fprintf(stdout, "# RePlan Tracking Suggestions: %s\n\n", result.ProjectName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Project: `%s`\n", result.ProjectID); err != nil {
		return err
	}
	if result.Week != "" {
		if _, err := fmt.Fprintf(stdout, "- Week: `%s`\n", result.Week); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "- Suggestions: %d\n", len(result.Suggestions)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Dry run: `%t`\n", result.DryRun); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Proposal candidates: %d\n", len(result.Proposals)); err != nil {
		return err
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintf(stdout, "- Written files: %d\n", len(result.WrittenFiles)); err != nil {
			return err
		}
	}
	if len(result.Suggestions) == 0 {
		_, err := fmt.Fprintln(stdout, "\nNo tracking suggestions.")
		return err
	}
	if _, err := fmt.Fprintln(stdout, "\n| Type | Task | Demand | Resource | Planned | Actual | Remaining | Forecast | Variance |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "|---|---|---|---|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, suggestion := range result.Suggestions {
		if _, err := fmt.Fprintf(stdout, "| `%s` | `%s` | `%s` | `%s` | %s | %s | %s | %s | %s |\n",
			suggestion.Type,
			emptyDash(suggestion.TaskID),
			emptyDash(suggestion.DemandID),
			emptyDash(suggestion.ResourceID),
			formatFloat(suggestion.Planned),
			formatFloat(suggestion.Actual),
			formatFloat(suggestion.Remaining),
			formatFloat(suggestion.Forecast),
			formatFloat(suggestion.Variance)); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout, "\n## Details"); err != nil {
		return err
	}
	for _, suggestion := range result.Suggestions {
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s): %s\n", suggestion.ID, suggestion.Severity, suggestion.Rationale); err != nil {
			return err
		}
		if suggestion.RecommendedCommand != "" {
			if _, err := fmt.Fprintf(stdout, "  - `%s`\n", suggestion.RecommendedCommand); err != nil {
				return err
			}
		}
	}
	if len(result.Proposals) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Proposal Candidates"); err != nil {
			return err
		}
		for _, proposal := range result.Proposals {
			if _, err := fmt.Fprintf(stdout, "- `%s` (%s -> `%s`): %s\n",
				proposal.ID,
				proposal.Type,
				proposal.TargetRef.ID,
				proposal.Rationale); err != nil {
				return err
			}
		}
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	}
	return nil
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func reportIssuesToDiagnostics(issues []reports.ReportIssue) []app.Diagnostic {
	out := make([]app.Diagnostic, 0, len(issues))
	for _, issue := range issues {
		out = append(out, app.Diagnostic{
			Code:     issue.Code,
			Message:  issue.Message,
			Severity: issue.Severity,
			Target:   app.Target{ID: issue.TargetID},
		})
	}
	return out
}

//nolint:gocognit,gocyclo // This flat renderer deliberately follows output order; section extraction would make the byte-level Markdown contract harder to audit.
func writeOpenProjectImportMarkdown(stdout *os.File, result app.OpenProjectImportResult) error {
	if _, err := fmt.Fprintln(stdout, "# OpenProject Import Dry Run"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Source: `%s`\n", result.Source); err != nil {
		return err
	}
	if result.FromFile != "" {
		if _, err := fmt.Fprintf(stdout, "- From file: `%s`\n", result.FromFile); err != nil {
			return err
		}
	}
	if result.Profile != nil {
		if _, err := fmt.Fprintf(stdout, "- Profile: `%s`\n", result.Profile.ID); err != nil {
			return err
		}
		if result.Profile.BaseURL != "" {
			if _, err := fmt.Fprintf(stdout, "- Base URL: `%s`\n", result.Profile.BaseURL); err != nil {
				return err
			}
		}
		if result.Profile.ProjectID != "" {
			if _, err := fmt.Fprintf(stdout, "- Source project: `%s`\n", result.Profile.ProjectID); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(stdout, "- Loaded work packages: `%d`\n", result.Snapshot.LoadedCount); err != nil {
		return err
	}
	if result.Snapshot.ID != "" {
		if _, err := fmt.Fprintf(stdout, "- Snapshot: `%s`\n", result.Snapshot.ID); err != nil {
			return err
		}
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` %s %s\n", write.Path, write.Action, write.Format); err != nil {
				return err
			}
		}
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Warnings"); err != nil {
			return err
		}
		for _, d := range result.Warnings {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(stdout, "\n## Task Candidates"); err != nil {
		return err
	}
	if len(result.TaskCandidates) == 0 {
		_, err := fmt.Fprintln(stdout, "- none")
		return err
	}
	for _, candidate := range result.TaskCandidates {
		target := "new task"
		if candidate.MatchedTaskID != "" {
			target = "`" + candidate.MatchedTaskID + "`"
		}
		if _, err := fmt.Fprintf(stdout, "- `%s` %s -> %s", candidate.SourceID, candidate.Title, target); err != nil {
			return err
		}
		if candidate.MappedStatus != "" {
			if _, err := fmt.Fprintf(stdout, " status `%s`", candidate.MappedStatus); err != nil {
				return err
			}
		}
		if candidate.EstimatedHours > 0 {
			if _, err := fmt.Fprintf(stdout, " estimate `%.2f h`", candidate.EstimatedHours); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}
	return nil
}

func writeGitHubImportMarkdown(stdout *os.File, result app.GitHubImportResult) error {
	if _, err := fmt.Fprintln(stdout, "# GitHub Import"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "\n- Source: `%s`\n- From file: `%s`\n- Loaded issues: `%d`\n- Snapshot: `%s`\n", result.Source, result.FromFile, result.Snapshot.LoadedCount, result.Snapshot.ID); err != nil {
		return err
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` %s %s\n", write.Path, write.Action, write.Format); err != nil {
				return err
			}
		}
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Warnings"); err != nil {
			return err
		}
		for _, diagnostic := range result.Warnings {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Errors"); err != nil {
			return err
		}
		for _, diagnostic := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(stdout, "\n## Task Candidates"); err != nil {
		return err
	}
	if len(result.TaskCandidates) == 0 {
		_, err := fmt.Fprintln(stdout, "- none")
		return err
	}
	for _, candidate := range result.TaskCandidates {
		target := "new task"
		if candidate.MatchedTaskID != "" {
			target = "`" + candidate.MatchedTaskID + "`"
		}
		if _, err := fmt.Fprintf(stdout, "- `%s` %s -> %s", candidate.SourceID, candidate.Title, target); err != nil {
			return err
		}
		if candidate.MappedStatus != "" {
			if _, err := fmt.Fprintf(stdout, " status `%s`", candidate.MappedStatus); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocognit,gocyclo // This flat renderer deliberately follows output order; section extraction would make the byte-level Markdown contract harder to audit.
func writeImportAcceptMarkdown(stdout *os.File, result app.ImportAcceptResult) error {
	title := "# Import Accept Dry Run"
	if result.Applied {
		title = "# Import Accept Apply"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if result.SnapshotID != "" {
		if _, err := fmt.Fprintf(stdout, "- Snapshot: `%s`\n", result.SnapshotID); err != nil {
			return err
		}
	}
	if result.MappedPath != "" {
		if _, err := fmt.Fprintf(stdout, "- Mapped: `%s`\n", result.MappedPath); err != nil {
			return err
		}
	}
	if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` %s %s\n", write.Path, write.Action, write.Format); err != nil {
				return err
			}
		}
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	}
	if len(result.AppliedChanges) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Applied Changes"); err != nil {
			return err
		}
		for _, change := range result.AppliedChanges {
			target := change.TargetKind
			if change.TargetID != "" {
				target += " `" + change.TargetID + "`"
			}
			if _, err := fmt.Fprintf(stdout, "- `%s` %s: %s\n", change.Op, target, change.Description); err != nil {
				return err
			}
		}
	}
	if len(result.Changes) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Changes"); err != nil {
			return err
		}
		for _, change := range result.Changes {
			target := change.TargetKind
			if change.TargetID != "" {
				target += " `" + change.TargetID + "`"
			}
			if _, err := fmt.Fprintf(stdout, "- `%s` %s: %s\n", change.Op, target, change.Description); err != nil {
				return err
			}
		}
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Warnings"); err != nil {
			return err
		}
		for _, d := range result.Warnings {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeScenarioDiffMarkdown(stdout *os.File, diff app.ScenarioDiffResult) error {
	if _, err := fmt.Fprintf(stdout, "# Scenario Diff: %s\n\n", diff.ScenarioName); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Scenario: `%s`\n", diff.ScenarioID); err != nil {
		return err
	}
	if diff.Against != "" {
		if _, err := fmt.Fprintf(stdout, "- Against: `%s`\n", diff.Against); err != nil {
			return err
		}
	}
	if diff.Week != "" {
		if _, err := fmt.Fprintf(stdout, "- Week: `%s`\n", diff.Week); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout, "\n## Affected Resources"); err != nil {
		return err
	}
	if len(diff.AffectedResources) == 0 {
		if _, err := fmt.Fprintln(stdout, "- none"); err != nil {
			return err
		}
	}
	for _, resourceID := range diff.AffectedResources {
		if _, err := fmt.Fprintf(stdout, "- `%s`\n", resourceID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout, "\n## Workload Changes"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "| Resource | Unit | Capacity Before | Capacity After | Overload Before | Overload After |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "|---|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	before := map[string]app.ResourceWorkload{}
	for _, row := range diff.WorkloadBefore {
		before[row.ResourceID+"\x00"+row.Unit] = row
	}
	for _, after := range diff.WorkloadAfter {
		key := after.ResourceID + "\x00" + after.Unit
		prior := before[key]
		if prior.Capacity == after.Capacity && prior.Overload == after.Overload {
			continue
		}
		name := after.ResourceName
		if name == "" {
			name = after.ResourceID
		}
		if _, err := fmt.Fprintf(stdout, "| `%s` | %s | %g | %g | %g | %g |\n",
			name, after.Unit, prior.Capacity, after.Capacity, prior.Overload, after.Overload); err != nil {
			return err
		}
	}
	if len(diff.Warnings) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Warnings"); err != nil {
			return err
		}
		for _, warning := range diff.Warnings {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", warning.Code, warning.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeWorkloadMarkdown(stdout *os.File, result app.WorkloadResult) error {
	var b strings.Builder
	b.WriteString("# RePlan Workload\n\n")
	week := result.Week
	if week == "" {
		week = "all weeks"
	}
	fmt.Fprintf(&b, "- Week: `%s`\n", week)
	if result.ScenarioID != "" {
		fmt.Fprintf(&b, "- Scenario: `%s`\n", result.ScenarioID)
	}
	b.WriteString("\n| Resource | Class | Unit | Capacity | Planned | Actual | Remaining | Overload |\n")
	b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|\n")
	for _, row := range result.Resources {
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		fmt.Fprintf(&b, "| `%s` | %s | %s | %g | %g | %g | %g | %g |\n",
			markdownCell(name), markdownCell(row.ResourceClass), markdownCell(row.Unit), row.Capacity, row.Planned, row.Actual, row.Remaining, row.Overload)
		for _, demand := range row.Demands {
			label := "↳ " + demand.ResourceDemandID
			if demand.TaskID != "" {
				label += " (" + demand.TaskID + ")"
			}
			fmt.Fprintf(&b, "| `%s` | demand | %s | - | %g | - | %g | - |\n",
				markdownCell(label), markdownCell(row.Unit), demand.Planned, demand.Remaining)
		}
	}
	writeDiagnosticsMarkdown(&b, result.Warnings, result.Errors)
	_, err := fmt.Fprint(stdout, b.String())
	return err
}

func writeWorkloadText(stdout *os.File, result app.WorkloadResult) error {
	week := result.Week
	if week == "" {
		week = "all weeks"
	}
	if _, err := fmt.Fprintf(stdout, "Workload: %s\n", week); err != nil {
		return err
	}
	if result.ScenarioID != "" {
		if _, err := fmt.Fprintf(stdout, "Scenario: %s\n", result.ScenarioID); err != nil {
			return err
		}
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "RESOURCE\tCLASS\tUNIT\tCAPACITY\tPLANNED\tACTUAL\tREMAINING\tOVERLOAD"); err != nil {
		return err
	}
	for _, row := range result.Resources {
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%g\t%g\t%g\t%g\t%g\n", name, row.ResourceClass, row.Unit, row.Capacity, row.Planned, row.Actual, row.Remaining, row.Overload); err != nil {
			return err
		}
		for _, demand := range row.Demands {
			label := "  -> " + demand.ResourceDemandID
			if demand.TaskID != "" {
				label += " (" + demand.TaskID + ")"
			}
			if _, err := fmt.Fprintf(tw, "%s\tdemand\t%s\t-\t%g\t-\t%g\t-\n", label, row.Unit, demand.Planned, demand.Remaining); err != nil {
				return err
			}
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return writeDiagnosticsText(stdout, result.Warnings, result.Errors)
}

func writeScenarioListMarkdown(stdout *os.File, result app.ScenarioListResult) error {
	var b strings.Builder
	b.WriteString("# RePlan Scenario List\n\n| ID | Name | Status | Changes |\n|---|---|---|---:|\n")
	for _, scenario := range result.Scenarios {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %d |\n", markdownCell(scenario.ID), markdownCell(scenario.Name), markdownCell(scenario.Status), scenario.Changes)
	}
	writeDiagnosticsMarkdown(&b, result.Warnings, result.Errors)
	_, err := fmt.Fprint(stdout, b.String())
	return err
}

func writeScenarioListText(stdout *os.File, result app.ScenarioListResult) error {
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "ID\tNAME\tSTATUS\tCHANGES"); err != nil {
		return err
	}
	for _, scenario := range result.Scenarios {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%d\n", scenario.ID, scenario.Name, scenario.Status, scenario.Changes); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return writeDiagnosticsText(stdout, result.Warnings, result.Errors)
}

func writeScenarioShowMarkdown(stdout *os.File, result app.ScenarioShowResult) error {
	var b strings.Builder
	scenario := result.Scenario
	fmt.Fprintf(&b, "# RePlan Scenario: %s\n\n- ID: `%s`\n- Name: %s\n- Status: %s\n- Description: %s\n",
		markdownCell(scenario.Name), markdownCell(scenario.ID), markdownCell(scenario.Name), markdownCell(scenario.Status), markdownCell(firstNonEmptyCLI(scenario.Description)))
	b.WriteString("\n## Changes\n\n| Target | Field | From | To |\n|---|---|---|---|\n")
	for _, change := range result.DisplayChanges {
		fmt.Fprintf(&b, "| `%s` | %s | %s | %s |\n", markdownCell(change.Target), markdownCell(change.Field), markdownCell(change.From), markdownCell(change.To))
	}
	writeDiagnosticsMarkdown(&b, result.Warnings, result.Errors)
	_, err := fmt.Fprint(stdout, b.String())
	return err
}

func writeScenarioShowText(stdout *os.File, result app.ScenarioShowResult) error {
	scenario := result.Scenario
	if _, err := fmt.Fprintf(stdout, "ID: %s\nName: %s\nStatus: %s\nDescription: %s\n\n", scenario.ID, scenario.Name, scenario.Status, firstNonEmptyCLI(scenario.Description)); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "TARGET\tFIELD\tFROM\tTO"); err != nil {
		return err
	}
	for _, change := range result.DisplayChanges {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", change.Target, change.Field, change.From, change.To); err != nil {
			return err
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	return writeDiagnosticsText(stdout, result.Warnings, result.Errors)
}

func writeDiagnosticsMarkdown(b *strings.Builder, warnings, errors []app.Diagnostic) {
	if len(warnings) > 0 {
		b.WriteString("\n## Warnings\n")
		for _, diagnostic := range warnings {
			fmt.Fprintf(b, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message)
		}
	}
	if len(errors) > 0 {
		b.WriteString("\n## Errors\n")
		for _, diagnostic := range errors {
			fmt.Fprintf(b, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message)
		}
	}
}

func writeDiagnosticsText(stdout *os.File, warnings, errors []app.Diagnostic) error {
	for _, diagnostic := range warnings {
		if _, err := fmt.Fprintf(stdout, "warning %s: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
			return err
		}
	}
	for _, diagnostic := range errors {
		if _, err := fmt.Fprintf(stdout, "error %s: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
			return err
		}
	}
	return nil
}

func writeScenarioApplyMarkdown(stdout *os.File, result app.ScenarioApplyResult) error {
	title := "# Scenario Apply"
	if result.DryRun {
		title += " Dry Run"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Scenario: `%s`\n", result.ScenarioID); err != nil {
		return err
	}
	if result.ScenarioName != "" {
		if _, err := fmt.Fprintf(stdout, "- Name: %s\n", result.ScenarioName); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "- Status: `%s` -> `%s`\n", result.PreviousStatus, result.NewStatus); err != nil {
		return err
	}
	if len(result.Changes) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Changes"); err != nil {
			return err
		}
		for _, change := range result.Changes {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s` %g -> %g\n", change.TargetID, change.Field, change.PreviousValue, change.NewValue); err != nil {
				return err
			}
		}
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	if result.AuditRecordPreview.ID != "" {
		heading := "Audit Preview"
		if result.Applied {
			heading = "Audit"
		}
		if _, err := fmt.Fprintf(stdout, "\n## %s\n", heading); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", result.AuditRecordPreview.ID, result.AuditRecordPreview.Operation); err != nil {
			return err
		}
	}
	return nil
}

func writeProposalListMarkdown(stdout *os.File, result app.ProposalListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Proposals"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Proposals) == 0 {
		_, err := fmt.Fprintln(stdout, "No proposals.")
		return err
	}
	for _, proposal := range result.Proposals {
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s, %s): %s\n", proposal.ID, proposal.Type, proposal.Status, proposal.Rationale); err != nil {
			return err
		}
	}
	return nil
}

func writeProposalShowMarkdown(stdout *os.File, result app.ProposalShowResult) error {
	proposal := result.Proposal
	if _, err := fmt.Fprintf(stdout, "# Proposal: %s\n\n", proposal.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Type: `%s`\n", proposal.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Status: `%s`\n", proposal.Status); err != nil {
		return err
	}
	if proposal.CreatedBy.ID != "" {
		if _, err := fmt.Fprintf(stdout, "- Created by: `%s`\n", proposal.CreatedBy.ID); err != nil {
			return err
		}
	}
	if proposal.TargetRef.ID != "" {
		if _, err := fmt.Fprintf(stdout, "- Target: `%s/%s`\n", proposal.TargetRef.Kind, proposal.TargetRef.ID); err != nil {
			return err
		}
	}
	if proposal.Rationale != "" {
		if _, err := fmt.Fprintf(stdout, "\n## Rationale\n\n%s\n", proposal.Rationale); err != nil {
			return err
		}
	}
	if proposal.SuggestedChange.Op != "" {
		if _, err := fmt.Fprintf(stdout, "\n## Suggested Change\n\n- Operation: `%s`\n- Path: `%s`\n", proposal.SuggestedChange.Op, proposal.SuggestedChange.Path); err != nil {
			return err
		}
	}
	if len(proposal.EvidenceRefs) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Evidence"); err != nil {
			return err
		}
		for _, ref := range proposal.EvidenceRefs {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", ref.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeProposalSuggestDependenciesMarkdown(stdout *os.File, result app.ProposalSuggestDependenciesResult) error {
	if _, err := fmt.Fprintln(stdout, "# Dependency Proposal Candidates"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Dry run: `%t`\n", result.DryRun); err != nil {
		return err
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "- Applied: `true`"); err != nil {
			return err
		}
	}
	if len(result.Proposals) == 0 {
		_, err := fmt.Fprintln(stdout, "\nNo dependency proposal candidates.")
		return err
	}
	if _, err := fmt.Fprintln(stdout, "\n## Candidates"); err != nil {
		return err
	}
	for _, proposal := range result.Proposals {
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s): %s\n", proposal.ID, proposal.Type, proposal.Rationale); err != nil {
			return err
		}
		if proposal.SuggestedChange.Op != "" {
			if _, err := fmt.Fprintf(stdout, "  - Change: `%s` `%s`\n", proposal.SuggestedChange.Op, proposal.SuggestedChange.Path); err != nil {
				return err
			}
		}
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeProposalDecisionMarkdown(stdout *os.File, result app.ProposalDecisionResult) error {
	title := "# Proposal " + titleWord(result.Decision)
	if result.DryRun {
		title += " Dry Run"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Proposal: `%s`\n", result.Proposal.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Status: `%s` -> `%s`\n", result.PreviousStatus, result.NewStatus); err != nil {
		return err
	}
	if result.Reason != "" {
		if _, err := fmt.Fprintf(stdout, "- Reason: %s\n", result.Reason); err != nil {
			return err
		}
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	if result.AuditRecordPreview.ID != "" {
		heading := "Audit Preview"
		if result.Applied {
			heading = "Audit"
		}
		if _, err := fmt.Fprintf(stdout, "\n## %s\n", heading); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", result.AuditRecordPreview.ID, result.AuditRecordPreview.Operation); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // This flat renderer writes three short ordered sections; splitting it would hide the exact output sequence behind indirection.
func writeDependencyListMarkdown(stdout *os.File, result app.DependencyListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Dependencies"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Dependencies: `%d`\n", result.Summary.Dependencies); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Required open: `%d`\n", result.Summary.RequiredOpen); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Active blockers: `%d`\n", result.Summary.ActiveBlockers); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Resolvable: `%d`\n", result.Summary.Resolvable); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Ready to unblock tasks: `%d`\n", result.Summary.ReadyToUnblockTasks); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(stdout, "\n## Active Blockers"); err != nil {
		return err
	}
	wrote := false
	for _, row := range result.Dependencies {
		if !row.Blocking {
			continue
		}
		wrote = true
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s, %s) -> `%s`", row.ID, row.Type, row.Status, row.SuccessorID); err != nil {
			return err
		}
		if row.SuccessorStatus != "" {
			if _, err := fmt.Fprintf(stdout, " [%s]", row.SuccessorStatus); err != nil {
				return err
			}
		}
		if len(row.OpenProposalIDs) > 0 {
			if _, err := fmt.Fprintf(stdout, " proposals=%s", strings.Join(row.OpenProposalIDs, ",")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}
	if !wrote {
		if _, err := fmt.Fprintln(stdout, "No active blockers."); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(stdout, "\n## Resolvable Dependencies"); err != nil {
		return err
	}
	wrote = false
	for _, row := range result.Dependencies {
		if !row.Resolvable {
			continue
		}
		wrote = true
		if _, err := fmt.Fprintf(stdout, "- `%s` predecessor `%s` can resolve\n", row.ID, row.PredecessorID); err != nil {
			return err
		}
	}
	if !wrote {
		if _, err := fmt.Fprintln(stdout, "No resolvable dependencies."); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(stdout, "\n## Blocked Successors"); err != nil {
		return err
	}
	if len(result.BlockedSuccessors) == 0 {
		if _, err := fmt.Fprintln(stdout, "No blocked successors with required dependencies."); err != nil {
			return err
		}
		return nil
	}
	for _, row := range result.BlockedSuccessors {
		state := "blocked"
		if row.ReadyToUnblock {
			state = "ready_to_unblock"
		}
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s): required=%s open=%s\n", row.TaskID, state, strings.Join(row.RequiredDependencyIDs, ","), strings.Join(row.OpenDependencyIDs, ",")); err != nil {
			return err
		}
	}
	return nil
}

func titleWord(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func writeAuditListMarkdown(stdout *os.File, result app.AuditListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Audit Records"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Records) == 0 {
		_, err := fmt.Fprintln(stdout, "No audit records.")
		return err
	}
	for _, record := range result.Records {
		if _, err := fmt.Fprintf(stdout, "- `%s` %s by `%s`: `%s` -> `%s`\n",
			record.OccurredAt,
			record.ID,
			record.ActorID,
			record.Operation,
			record.TargetID); err != nil {
			return err
		}
	}
	return nil
}

func writeActualListMarkdown(stdout *os.File, result app.ActualListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Actuals"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Actuals) == 0 {
		_, err := fmt.Fprintln(stdout, "No actuals.")
		return err
	}
	if _, err := fmt.Fprintln(stdout, "| Actual | Type | Task | Resource | Amount | Unit | Occurred | Accepted By |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "|---|---|---|---|---:|---|---|---|"); err != nil {
		return err
	}
	for _, actual := range result.Actuals {
		if _, err := fmt.Fprintf(stdout, "| `%s` | `%s` | `%s` | `%s` | %g | %s | %s | `%s` |\n",
			actual.ID,
			actual.Type,
			actual.TaskID,
			actual.ResourceID,
			actual.Amount,
			actual.Unit,
			actual.OccurredAt,
			actual.AcceptedBy); err != nil {
			return err
		}
	}
	return nil
}

func writeActualAddConsumptionDryRunMarkdown(stdout *os.File, result app.ActualAddConsumptionResult) error {
	title := "# Actual Add Consumption"
	if result.DryRun {
		title += " Dry Run"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	actual := result.ActualSummary
	if _, err := fmt.Fprintf(stdout, "- Actual: `%s`\n", actual.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Task: `%s`\n", actual.TaskID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Demand: `%s`\n", actual.DemandID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Resource: `%s`\n", actual.ResourceID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Amount: `%g %s`\n", actual.Amount, actual.Unit); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Accepted by: `%s`\n", actual.AcceptedBy); err != nil {
		return err
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	if result.AuditRecordPreview.ID != "" {
		heading := "Audit Preview"
		if result.Applied {
			heading = "Audit"
		}
		if _, err := fmt.Fprintf(stdout, "\n## %s\n", heading); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", result.AuditRecordPreview.ID, result.AuditRecordPreview.Operation); err != nil {
			return err
		}
	}
	if result.Validation != nil {
		if _, err := fmt.Fprintln(stdout, "\n## Validation"); err != nil {
			return err
		}
		if result.Validation.Valid {
			_, err := fmt.Fprintln(stdout, "- valid")
			return err
		}
		for _, d := range result.Validation.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

//nolint:gocyclo // This flat renderer keeps every conditional output line in contract order; helper boundaries would make byte-level review harder.
func writeActualSetRemainingDryRunMarkdown(stdout *os.File, result app.ActualSetRemainingResult) error {
	title := "# Actual Set Remaining"
	if result.DryRun {
		title += " Dry Run"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Demand: `%s`\n", result.DemandID); err != nil {
		return err
	}
	if result.TaskID != "" {
		if _, err := fmt.Fprintf(stdout, "- Task: `%s`\n", result.TaskID); err != nil {
			return err
		}
	}
	if result.ResourceID != "" {
		if _, err := fmt.Fprintf(stdout, "- Resource: `%s`\n", result.ResourceID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "- Remaining: `%g %s` -> `%g %s`\n", result.PreviousAmount, result.Unit, result.NewAmount, result.Unit); err != nil {
		return err
	}
	if result.Reason != "" {
		if _, err := fmt.Fprintf(stdout, "- Reason: %s\n", result.Reason); err != nil {
			return err
		}
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	if result.AuditRecordPreview.ID != "" {
		heading := "Audit Preview"
		if result.Applied {
			heading = "Audit"
		}
		if _, err := fmt.Fprintf(stdout, "\n## %s\n", heading); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", result.AuditRecordPreview.ID, result.AuditRecordPreview.Operation); err != nil {
			return err
		}
	}
	if result.Validation != nil {
		if _, err := fmt.Fprintln(stdout, "\n## Validation"); err != nil {
			return err
		}
		if result.Validation.Valid {
			_, err := fmt.Fprintln(stdout, "- valid")
			return err
		}
		for _, d := range result.Validation.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

//nolint:gocyclo // This flat renderer keeps every conditional output line in contract order; helper boundaries would make byte-level review harder.
func writeActualCloseDemandDryRunMarkdown(stdout *os.File, result app.ActualCloseDemandResult) error {
	title := "# Actual Close Demand"
	if result.DryRun {
		title += " Dry Run"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Demand: `%s`\n", result.DemandID); err != nil {
		return err
	}
	if result.TaskID != "" {
		if _, err := fmt.Fprintf(stdout, "- Task: `%s`\n", result.TaskID); err != nil {
			return err
		}
	}
	if result.ResourceID != "" {
		if _, err := fmt.Fprintf(stdout, "- Resource: `%s`\n", result.ResourceID); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(stdout, "- Status: `%s` -> `%s`\n", result.PreviousStatus, result.NewStatus); err != nil {
		return err
	}
	if result.RemainingCleared {
		before := "not estimated"
		if result.PreviousRemaining != nil {
			before = strconv.FormatFloat(*result.PreviousRemaining, 'f', -1, 64)
		}
		if _, err := fmt.Fprintf(stdout, "- Remaining: %s -> 0\n", before); err != nil {
			return err
		}
	}
	if result.Reason != "" {
		if _, err := fmt.Fprintf(stdout, "- Reason: %s\n", result.Reason); err != nil {
			return err
		}
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	}
	if result.AuditRecordPreview.ID != "" {
		heading := "Audit Preview"
		if result.Applied {
			heading = "Audit"
		}
		if _, err := fmt.Fprintf(stdout, "\n## %s\n", heading); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", result.AuditRecordPreview.ID, result.AuditRecordPreview.Operation); err != nil {
			return err
		}
	}
	if result.Validation != nil {
		if _, err := fmt.Fprintln(stdout, "\n## Validation"); err != nil {
			return err
		}
		if result.Validation.Valid {
			_, err := fmt.Fprintln(stdout, "- valid")
			return err
		}
		for _, d := range result.Validation.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeActualAddStatusDryRunMarkdown(stdout *os.File, result app.ActualAddStatusResult) error {
	title := "# Actual Add Status"
	if result.DryRun {
		title += " Dry Run"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Task: `%s`\n", result.TaskID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Status: `%s` -> `%s`\n", result.PreviousStatus, result.NewStatus); err != nil {
		return err
	}
	if result.Reason != "" {
		if _, err := fmt.Fprintf(stdout, "- Reason: %s\n", result.Reason); err != nil {
			return err
		}
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	if result.AuditRecordPreview.ID != "" {
		heading := "Audit Preview"
		if result.Applied {
			heading = "Audit"
		}
		if _, err := fmt.Fprintf(stdout, "\n## %s\n", heading); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", result.AuditRecordPreview.ID, result.AuditRecordPreview.Operation); err != nil {
			return err
		}
	}
	if result.Validation != nil {
		if _, err := fmt.Fprintln(stdout, "\n## Validation"); err != nil {
			return err
		}
		if result.Validation.Valid {
			_, err := fmt.Fprintln(stdout, "- valid")
			return err
		}
		for _, d := range result.Validation.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	return nil
}
