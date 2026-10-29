package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func runEstimate(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing estimate subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "calibrate":
		return runEstimateCalibrate(args[1:], stdout, stderr)
	case "list":
		return runEstimateList(args[1:], stdout, stderr)
	case "show":
		return runEstimateShow(args[1:], stdout, stderr)
	case "apply":
		return runEstimateApply(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown estimate subcommand: %s\n", args[0])
		return exitInvalidUsage
	}
}

func runEstimateCalibrate(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("estimate calibrate")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json or markdown")
	var baselines, resources, exclusions repeatedStringFlag
	fs.Var(&baselines, "baseline", "required repeatable source baseline id")
	fs.Var(&resources, "resource", "required repeatable resource id; multiple resources form one split group")
	fs.Var(&exclusions, "exclude", "optional repeatable task-id=reason exclusion")
	unit := fs.String("unit", "", "required calibration unit")
	tag := fs.String("tag", "", "optional task tag scope")
	id := fs.String("id", "", "required calibration id")
	name := fs.String("name", "", "required calibration name")
	approvedBy := fs.String("approved-by", "", "required human approving actor id")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 approval time; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	*format = strings.ToLower(*format)
	if *format != "json" && *format != "markdown" {
		return printTaskFormatError("estimate calibrate", *projectPath, stdout, stderr)
	}
	excluded, parseErr := parseCalibrationExclusions(exclusions)
	if parseErr != nil {
		fmt.Fprintln(stderr, parseErr)
		return exitInvalidUsage
	}
	dryRun, code, done := requireCanonicalWriteChoice("estimate calibrate", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewCalibrationService(newStore(*projectPath)).Calibrate(context.Background(), app.CalibrationCalibrateRequest{
		BaselineIDs: baselines, ResourceIDs: resources, Unit: *unit, Tag: *tag, Exclusions: excluded,
		ID: *id, Name: *name, ApprovedBy: *approvedBy, OccurredAt: *occurredAt, DryRun: dryRun,
	})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "estimate calibrate", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if *format == "markdown" && err == nil {
		writeCalibrationMarkdown(stdout, result.Calibration)
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runEstimateList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("estimate list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewCalibrationService(newStore(*projectPath)).List(context.Background())
	if (strings.EqualFold(*format, "markdown") || strings.EqualFold(*format, "text")) && err == nil {
		fmt.Fprintln(stdout, "# Estimate Calibrations")
		for _, row := range result.Calibrations {
			fmt.Fprintf(stdout, "- `%s`: factor %.6g, %d tasks, %s (%s)\n", row.ID, row.Factor, row.Count, row.Unit, row.SealStatus)
		}
		return exitSuccess
	}
	env := envelope{OK: err == nil, Command: "estimate list", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runEstimateShow(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("estimate show")
	projectPath, format := addProjectFormatFlags(fs, "json")
	leadingID, filteredArgs := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, filteredArgs, stdout, stderr); done {
		return code
	}
	id := positionalArg(leadingID, fs)
	result, err := app.NewCalibrationService(newStore(*projectPath)).Show(context.Background(), id)
	if (strings.EqualFold(*format, "markdown") || strings.EqualFold(*format, "text")) && err == nil {
		writeCalibrationMarkdown(stdout, result.Calibration)
		return exitSuccess
	}
	env := envelope{OK: err == nil, Command: "estimate show", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runEstimateApply(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("estimate apply")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json or markdown")
	calibrationID := fs.String("calibration", "", "required calibration id")
	scenarioID := fs.String("scenario-id", "", "required new scenario id")
	actorID := fs.String("actor", "", "required actor id")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 creation time; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("estimate apply", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewCalibrationService(newStore(*projectPath)).Apply(context.Background(), app.CalibrationApplyRequest{CalibrationID: *calibrationID, ScenarioID: *scenarioID, ActorID: *actorID, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil, Command: "estimate apply", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "markdown") && err == nil {
		fmt.Fprintf(stdout, "# Calibration scenario `%s`\n\n- Calibration: `%s`\n- Changes: %d\n- Applied: %t\n\n", result.Scenario.ID, result.CalibrationID, len(result.Scenario.Changes), result.Applied)
		fmt.Fprintln(stdout, "## Skipped demands")
		if len(result.Skipped) == 0 {
			fmt.Fprintln(stdout, "No demands skipped.")
		}
		for _, skip := range result.Skipped {
			fmt.Fprintf(stdout, "- `%s`: %s\n", skip.DemandID, skip.Reason)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func parseCalibrationExclusions(values []string) (map[string]string, error) {
	result := map[string]string{}
	for _, value := range values {
		taskID, reason, ok := strings.Cut(value, "=")
		if !ok || strings.TrimSpace(taskID) == "" || strings.TrimSpace(reason) == "" {
			return nil, fmt.Errorf("--exclude must use task-id=reason")
		}
		result[taskID] = reason
	}
	return result, nil
}

func writeCalibrationMarkdown(out *os.File, calibration domain.Calibration) {
	fmt.Fprintf(out, "# %s\n\n", calibration.Name)
	fmt.Fprintf(out, "- ID: `%s`\n- Factor: %.6g\n- Total ratio: %.6g\n- P25 / P75: %.6g / %.6g\n- Count: %d\n- Unit: `%s`\n- Resources: `%s`\n\n", calibration.ID, calibration.Result.Factor, calibration.Result.TotalRatio, calibration.Result.P25, calibration.Result.P75, calibration.Result.Count, calibration.Scope.Unit, strings.Join(calibration.Scope.Resources, "`, `"))
	fmt.Fprintln(out, "## Size bands")
	for _, band := range calibration.Result.SizeBands {
		label := "above 4"
		if band.UpTo != nil {
			label = fmt.Sprintf("up to %.6g", *band.UpTo)
		}
		fmt.Fprintf(out, "- %s: %d tasks, median %.6g\n", label, band.Count, band.Median)
	}
	if len(calibration.Sample.Excluded) > 0 {
		fmt.Fprintln(out, "\n## Excluded")
		for _, exclusion := range calibration.Sample.Excluded {
			fmt.Fprintf(out, "- `%s`: %s\n", exclusion.Task, exclusion.Reason)
		}
	}
}
