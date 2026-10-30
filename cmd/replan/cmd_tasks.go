package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

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
