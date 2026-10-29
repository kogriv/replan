package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kogriv/replan/internal/app"
)

func runActualReopenDemand(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("actual reopen-demand")
	projectPath, format := addProjectFormatFlags(fs, "json")
	demandID := fs.String("demand", "", "resource demand id")
	remaining := fs.String("remaining", "", "optional remaining amount to record, including 0")
	reason := fs.String("reason", "", "reason for reopening the demand")
	evidenceID := fs.String("evidence", "", "evidence id")
	actorID := fs.String("actor", "", "actor id accepting the update")
	occurredAt := fs.String("occurred-at", "", "RFC3339 occurrence timestamp")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("actual reopen-demand", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	req := app.ActualReopenDemandRequest{
		DemandID:   *demandID,
		Reason:     *reason,
		EvidenceID: *evidenceID,
		ActorID:    *actorID,
		OccurredAt: *occurredAt,
		DryRun:     dryRun,
	}
	if strings.TrimSpace(*remaining) != "" {
		value, err := strconv.ParseFloat(strings.TrimSpace(*remaining), 64)
		if err != nil {
			fmt.Fprintf(stderr, "invalid --remaining %q: %v\n", *remaining, err)
			return exitInvalidUsage
		}
		req.Remaining = &value
	}
	result, err := app.NewActualService(newStore(*projectPath)).ReopenDemand(context.Background(), req)
	lines := []string{
		fmt.Sprintf("- Demand: `%s`", result.DemandID),
		fmt.Sprintf("- Task: `%s`", result.TaskID),
		fmt.Sprintf("- Status: `%s` -> `%s`", result.PreviousStatus, result.NewStatus),
	}
	if result.NewRemaining != nil {
		lines = append(lines, fmt.Sprintf("- Remaining: %s -> %g", formatOptionalAmount(result.PreviousRemaining), *result.NewRemaining))
	}
	return finishLifecycleCommand(stdout, stderr, *format, "actual reopen-demand", "Actual Reopen Demand", *projectPath, result.DryRun, result.Applied, result.WrittenFiles, lines, result, result.Warnings, result.Errors, err)
}

func runActualAcknowledgeOverrun(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("actual acknowledge-overrun")
	projectPath, format := addProjectFormatFlags(fs, "json")
	demandID := fs.String("demand", "", "resource demand id")
	reason := fs.String("reason", "", "why the overrun is real and accepted")
	evidenceID := fs.String("evidence", "", "evidence id")
	actorID := fs.String("actor", "", "actor id acknowledging the overrun")
	occurredAt := fs.String("occurred-at", "", "RFC3339 occurrence timestamp")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("actual acknowledge-overrun", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewActualService(newStore(*projectPath)).AcknowledgeOverrun(context.Background(), app.ActualAcknowledgeOverrunRequest{
		DemandID:   *demandID,
		Reason:     *reason,
		EvidenceID: *evidenceID,
		ActorID:    *actorID,
		OccurredAt: *occurredAt,
		DryRun:     dryRun,
	})
	lines := []string{
		fmt.Sprintf("- Demand: `%s`", result.DemandID),
		fmt.Sprintf("- Task: `%s`", result.TaskID),
		fmt.Sprintf("- Planned %g, actual %g, remaining %g, forecast %g, variance %g", result.Planned, result.Actual, result.Remaining, result.Forecast, result.Variance),
	}
	return finishLifecycleCommand(stdout, stderr, *format, "actual acknowledge-overrun", "Actual Acknowledge Overrun", *projectPath, result.DryRun, result.Applied, result.WrittenFiles, lines, result, result.Warnings, result.Errors, err)
}

func finishLifecycleCommand(stdout, stderr *os.File, format, command, title, projectPath string, dryRun, applied bool, written []string, lines []string, result any, warnings, errors []app.Diagnostic, err error) int {
	if format == "markdown" || format == "text" {
		var b strings.Builder
		heading := "# " + title
		if dryRun {
			heading += " Dry Run"
		}
		b.WriteString(heading + "\n\n")
		if len(errors) > 0 {
			b.WriteString("## Errors\n")
			for _, d := range errors {
				fmt.Fprintf(&b, "- `%s`: %s\n", d.Code, d.Message)
			}
		} else {
			for _, line := range lines {
				b.WriteString(line + "\n")
			}
			fmt.Fprintf(&b, "- Applied: %t\n", applied)
			for _, path := range written {
				fmt.Fprintf(&b, "- Wrote `%s`\n", path)
			}
		}
		if _, printErr := fmt.Fprint(stdout, b.String()); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(errors)
		}
		return exitSuccess
	}
	env := envelope{
		OK:          err == nil && len(errors) == 0,
		Command:     command,
		ProjectPath: projectPath,
		Result:      result,
		Warnings:    warnings,
		Errors:      errors,
	}
	return printScenarioEnvelope(stdout, stderr, format, env, err)
}

func formatOptionalAmount(v *float64) string {
	if v == nil {
		return "unset"
	}
	return strconv.FormatFloat(*v, 'g', -1, 64)
}
