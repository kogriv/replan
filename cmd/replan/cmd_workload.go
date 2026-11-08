package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/kogriv/replan/internal/app"
)

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
		raw           bool
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "json", "output format: json, markdown, text")
	fs.BoolVar(&withDemands, "demands", false, "list in every row the demands that make it up")
	fs.BoolVar(&raw, "raw", false, "show uncalibrated demand amounts")
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
		Raw:           raw,
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
	hasUnlevelled := workloadHasUnlevelled(result.Resources)
	if hasUnlevelled {
		b.WriteString("\n| Resource | Class | Unit | Capacity | Planned | Actual | Remaining | Unlevelled Planned | Unlevelled Remaining | Overload |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---:|---:|\n")
	} else {
		b.WriteString("\n| Resource | Class | Unit | Capacity | Planned | Actual | Remaining | Overload |\n")
		b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|\n")
	}
	for _, row := range result.Resources {
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		if hasUnlevelled {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %g | %g | %g | %g | %g | %g | %g |\n",
				markdownCell(name), markdownCell(row.ResourceClass), markdownCell(row.Unit), row.Capacity, row.Planned, row.Actual, row.Remaining, row.UnlevelledPlanned, row.UnlevelledRemaining, row.Overload)
		} else {
			fmt.Fprintf(&b, "| `%s` | %s | %s | %g | %g | %g | %g | %g |\n",
				markdownCell(name), markdownCell(row.ResourceClass), markdownCell(row.Unit), row.Capacity, row.Planned, row.Actual, row.Remaining, row.Overload)
		}
		for _, demand := range row.Demands {
			label := "↳ " + demand.ResourceDemandID
			if demand.TaskID != "" {
				label += " (" + demand.TaskID + ")"
			}
			planned, remaining := fmt.Sprintf("%g", demand.Planned), fmt.Sprintf("%g", demand.Remaining)
			if demand.CalibrationID != "" {
				planned = fmt.Sprintf("%g typed; %g calibrated", *demand.RawPlanned, demand.Planned)
				remaining = fmt.Sprintf("%g typed; %g calibrated", *demand.RawRemaining, demand.Remaining)
			}
			if hasUnlevelled {
				unlevelledPlanned, unlevelledRemaining := "-", "-"
				if demand.Levelled != nil && !*demand.Levelled {
					unlevelledPlanned, unlevelledRemaining = planned, remaining
					planned, remaining = "-", "-"
				}
				fmt.Fprintf(&b, "| `%s` | demand | %s | - | %s | - | %s | %s | %s | - |\n", markdownCell(label), markdownCell(row.Unit), planned, remaining, unlevelledPlanned, unlevelledRemaining)
			} else {
				fmt.Fprintf(&b, "| `%s` | demand | %s | - | %s | - | %s | - |\n", markdownCell(label), markdownCell(row.Unit), planned, remaining)
			}
		}
	}
	writeWorkloadCalibrationNotes(&b, result.Resources)
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
	hasUnlevelled := workloadHasUnlevelled(result.Resources)
	header := "RESOURCE\tCLASS\tUNIT\tCAPACITY\tPLANNED\tACTUAL\tREMAINING\tOVERLOAD"
	if hasUnlevelled {
		header = "RESOURCE\tCLASS\tUNIT\tCAPACITY\tPLANNED\tACTUAL\tREMAINING\tUNLEVELLED PLANNED\tUNLEVELLED REMAINING\tOVERLOAD"
	}
	if _, err := fmt.Fprintln(tw, header); err != nil {
		return err
	}
	for _, row := range result.Resources {
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		format := "%s\t%s\t%s\t%g\t%g\t%g\t%g\t%g\n"
		args := []any{name, row.ResourceClass, row.Unit, row.Capacity, row.Planned, row.Actual, row.Remaining, row.Overload}
		if hasUnlevelled {
			format = "%s\t%s\t%s\t%g\t%g\t%g\t%g\t%g\t%g\t%g\n"
			args = []any{name, row.ResourceClass, row.Unit, row.Capacity, row.Planned, row.Actual, row.Remaining, row.UnlevelledPlanned, row.UnlevelledRemaining, row.Overload}
		}
		if _, err := fmt.Fprintf(tw, format, args...); err != nil {
			return err
		}
		for _, demand := range row.Demands {
			label := "  -> " + demand.ResourceDemandID
			if demand.TaskID != "" {
				label += " (" + demand.TaskID + ")"
			}
			planned, remaining := fmt.Sprintf("%g", demand.Planned), fmt.Sprintf("%g", demand.Remaining)
			if demand.CalibrationID != "" {
				planned = fmt.Sprintf("%g typed; %g calibrated", *demand.RawPlanned, demand.Planned)
				remaining = fmt.Sprintf("%g typed; %g calibrated", *demand.RawRemaining, demand.Remaining)
			}
			format := "%s\tdemand\t%s\t-\t%s\t-\t%s\t-\n"
			args := []any{label, row.Unit, planned, remaining}
			if hasUnlevelled {
				unlevelledPlanned, unlevelledRemaining := "-", "-"
				if demand.Levelled != nil && !*demand.Levelled {
					unlevelledPlanned, unlevelledRemaining = planned, remaining
					planned, remaining = "-", "-"
				}
				format = "%s\tdemand\t%s\t-\t%s\t-\t%s\t%s\t%s\t-\n"
				args = []any{label, row.Unit, planned, remaining, unlevelledPlanned, unlevelledRemaining}
			}
			if _, err := fmt.Fprintf(tw, format, args...); err != nil {
				return err
			}
		}
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	var notes strings.Builder
	writeWorkloadCalibrationNotes(&notes, result.Resources)
	if _, err := fmt.Fprint(stdout, notes.String()); err != nil {
		return err
	}
	return writeDiagnosticsText(stdout, result.Warnings, result.Errors)
}

func workloadHasUnlevelled(rows []app.ResourceWorkload) bool {
	for _, row := range rows {
		if row.UnlevelledPlanned != 0 || row.UnlevelledRemaining != 0 {
			return true
		}
	}
	return false
}

func writeWorkloadCalibrationNotes(b *strings.Builder, rows []app.ResourceWorkload) {
	for _, row := range rows {
		if len(row.CalibrationIDs) == 0 {
			continue
		}
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		fmt.Fprintf(b, "\n%s (%s): calibrated by %s; raw planned %g, raw remaining %g.\n", name, row.Unit, strings.Join(row.CalibrationIDs, ", "), *row.RawPlanned, *row.RawRemaining)
	}
}
