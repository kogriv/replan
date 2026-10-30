package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kogriv/replan/internal/app"
)

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
