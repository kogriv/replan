package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func runDependency(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing dependency subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "add":
		return runDependencyAdd(args[1:], stdout, stderr)
	case "set":
		return runDependencySet(args[1:], stdout, stderr)
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

type dependencySetBatchFile struct {
	Dependencies []struct {
		ID            string  `yaml:"id"`
		Status        *string `yaml:"status"`
		RequiredState *string `yaml:"required_state"`
		Criticality   *string `yaml:"criticality"`
		Reason        string  `yaml:"reason"`
	} `yaml:"dependencies"`
}

type dependencyBatchFile struct {
	Dependencies []struct {
		ID                     string `yaml:"id"`
		Type                   string `yaml:"type"`
		Predecessor            string `yaml:"predecessor"`
		Successor              string `yaml:"successor"`
		External               bool   `yaml:"external"`
		Criticality            string `yaml:"criticality"`
		Status                 string `yaml:"status"`
		Description            string `yaml:"description"`
		RequiredState          string `yaml:"required_state"`
		ExpectedResolutionDate string `yaml:"expected_resolution_date"`
	} `yaml:"dependencies"`
}

func runDependencyAdd(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("dependency add")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	id := fs.String("id", "", "optional dependency id; derived when absent")
	dependencyType := fs.String("type", "finish_to_start", "dependency type")
	predecessor := fs.String("predecessor", "", "predecessor task id")
	successor := fs.String("successor", "", "successor task id")
	external := fs.Bool("external", false, "create an external_dependency without a predecessor task")
	criticality := fs.String("criticality", "", "dependency criticality; defaults to required, or optional for relates")
	status := fs.String("status", "open", "dependency status")
	description := fs.String("description", "", "optional description; required with --external")
	requiredState := fs.String("required-state", "", "optional predecessor state required")
	expectedResolutionDate := fs.String("expected-resolution-date", "", "optional YYYY-MM-DD")
	fromFile := fs.String("from-file", "", "YAML file containing a dependencies list")
	reason := fs.String("reason", "", "required audit reason")
	actor := fs.String("actor", "", "required existing audit actor")
	evidence := fs.String("evidence", "", "required existing evidence id shared by the batch")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if strings.ToLower(*format) != "json" && strings.ToLower(*format) != "text" {
		return printTaskFormatError("dependency add", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("dependency add", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	items := []app.DependencyAddItem{{ID: *id, Type: *dependencyType, PredecessorID: *predecessor, SuccessorID: *successor, External: *external, Criticality: *criticality, Status: *status, Description: *description, RequiredState: *requiredState, ExpectedResolutionDate: *expectedResolutionDate}}
	if *fromFile != "" {
		var batch dependencyBatchFile
		readErr := readYAMLBatch(*fromFile, &batch)
		if readErr != nil {
			return printInputFileError("dependency add", *projectPath, *format, *fromFile, readErr, stdout, stderr)
		}
		items = make([]app.DependencyAddItem, 0, len(batch.Dependencies))
		for _, item := range batch.Dependencies {
			items = append(items, app.DependencyAddItem{ID: item.ID, Type: item.Type, PredecessorID: item.Predecessor, SuccessorID: item.Successor, External: item.External, Criticality: item.Criticality, Status: item.Status, Description: item.Description, RequiredState: item.RequiredState, ExpectedResolutionDate: item.ExpectedResolutionDate})
		}
	}
	result, err := app.NewDependencyService(newStore(*projectPath)).Add(context.Background(), app.DependencyAddRequest{Items: items, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "dependency add", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "text") && err == nil {
		return writeAddItemsText(stdout, "Dependencies", result.DryRun, result.Applied, dependencyIDs(result.Dependencies))
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runDependencySet(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("dependency set")
	setCommandUsage(fs, "replan dependency set <dependency-id> [flags]")
	projectPath, format := addProjectFormatFlags(fs, "json")
	status := fs.String("status", "", "new status: open, resolved or cancelled")
	requiredState := fs.String("required-state", "", "new predecessor state required")
	criticality := fs.String("criticality", "", "new criticality")
	fromFile := fs.String("from-file", "", "YAML or JSON file containing a dependencies list")
	reason := fs.String("reason", "", "required audit reason; batch item reasons override it")
	actor := fs.String("actor", "", "required existing audit actor")
	evidence := fs.String("evidence", "", "optional existing evidence id shared by the batch")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset")
	writeFlags := addCanonicalWriteFlags(fs)
	positional, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	var code int
	var done bool
	positional, code, done = requireSinglePositional(fs, positional, "dependency set", "<dependency-id>", *projectPath, *format, *fromFile == "", stdout, stderr)
	if done {
		return code
	}
	if !supportedTaskFormat(*format) {
		return printTaskFormatError("dependency set", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("dependency set", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	set := visitedFlags(fs)
	items := []app.DependencySetItem{{ID: positional, Status: stringPtrIf(set["status"], *status), RequiredState: stringPtrIf(set["required-state"], *requiredState), Criticality: stringPtrIf(set["criticality"], *criticality)}}
	if *fromFile != "" {
		var batch dependencySetBatchFile
		if err := readYAMLBatch(*fromFile, &batch); err != nil {
			return printInputFileError("dependency set", *projectPath, *format, *fromFile, err, stdout, stderr)
		}
		items = make([]app.DependencySetItem, 0, len(batch.Dependencies))
		for _, item := range batch.Dependencies {
			items = append(items, app.DependencySetItem{ID: item.ID, Status: item.Status, RequiredState: item.RequiredState, Criticality: item.Criticality, Reason: item.Reason})
		}
	}
	result, err := app.NewDependencyService(newStore(*projectPath)).Set(context.Background(), app.DependencySetRequest{Items: items, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "dependency set", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "text") && err == nil {
		fmt.Fprintf(stdout, "Dependencies\nDry run: %t\nApplied: %t\nNo-op: %t\n", result.DryRun, result.Applied, result.NoOp)
		for _, change := range result.Changes {
			fmt.Fprintf(stdout, "- %s: changed=%s unchanged=%s\n", change.After.ID, strings.Join(change.ChangedFields, ","), strings.Join(change.UnchangedFields, ","))
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func dependencyIDs(items []domain.Dependency) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func printInputFileError(command, projectPath, format, path string, inputErr error, stdout, stderr *os.File) int {
	env := envelope{OK: false, Command: command, ProjectPath: projectPath, Result: nil, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{{Code: "batch_file_invalid", Message: fmt.Sprintf("failed to read or parse %s: %v", path, inputErr), Severity: "error"}}}
	if err := printEnvelope(stdout, format, env); err != nil {
		fmt.Fprintln(stderr, err)
		return exitGeneralError
	}
	return exitInvalidUsage
}

func writeAddItemsText(stdout *os.File, title string, dryRun, applied bool, ids []string) int {
	fmt.Fprintf(stdout, "%s\nDry run: %t\nApplied: %t\n", title, dryRun, applied)
	for _, id := range ids {
		fmt.Fprintf(stdout, "- %s\n", id)
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
	setCommandUsage(fs, "replan proposal show <proposal-id> [flags]")
	projectPath, format := addProjectFormatFlags(fs, "json")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID, code, done := requireSinglePositional(fs, positionalID, "proposal show", "<proposal-id>", *projectPath, *format, true, stdout, stderr)
	if done {
		return code
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
	setCommandUsage(fs, "replan proposal "+decision+" <proposal-id> [flags]")
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	actorID := fs.String("actor", "", "actor id performing proposal decision; required unless --dry-run is used")
	reason := fs.String("reason", "", "decision reason")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID, code, done := requireSinglePositional(fs, positionalID, "proposal "+decision, "<proposal-id>", *projectPath, *format, true, stdout, stderr)
	if done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("proposal "+decision, *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
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
