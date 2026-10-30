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
