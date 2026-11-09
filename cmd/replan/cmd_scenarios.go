package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/kogriv/replan/internal/app"
)

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
	setCommandUsage(fs, "replan scenario show <scenario-id> [flags]")
	projectPath, format := addProjectFormatFlags(fs, "json")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID, code, done := requireSinglePositional(fs, positionalID, "scenario show", "<scenario-id>", *projectPath, *format, true, stdout, stderr)
	if done {
		return code
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
	setCommandUsage(fs, "replan scenario diff <scenario-id> [flags]")
	projectPath, format := addProjectFormatFlags(fs, "json")
	against := fs.String("against", "current", "comparison base")
	week := fs.String("week", "", "ISO week, for example 2026-W28")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID, code, done := requireSinglePositional(fs, positionalID, "scenario diff", "<scenario-id>", *projectPath, *format, true, stdout, stderr)
	if done {
		return code
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
		var printErr error
		if *format == "markdown" {
			printErr = writeScenarioDiffMarkdown(stdout, result)
		} else {
			printErr = writeScenarioDiffText(stdout, result)
		}
		if printErr != nil {
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
	setCommandUsage(fs, "replan scenario apply <scenario-id> [flags]")
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	actorID := fs.String("actor", "", "actor id applying the scenario; required unless --dry-run is used")
	reason := fs.String("reason", "", "apply reason")
	evidenceID := fs.String("evidence", "", "evidence id of the decision; required when the scenario cancels a dependency")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID, code, done := requireSinglePositional(fs, positionalID, "scenario apply", "<scenario-id>", *projectPath, *format, true, stdout, stderr)
	if done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("scenario apply", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
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
	if len(diff.TaskChanges) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Task Changes"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, "| Task ID | Title | Field | Current | Scenario |"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, "|---|---|---|---|---|"); err != nil {
			return err
		}
		for _, change := range diff.TaskChanges {
			if _, err := fmt.Fprintf(stdout, "| `%s` | %s | `%s` | %s | %s |\n",
				markdownCell(change.TaskID), markdownCell(change.TaskTitle), markdownCell(change.Field), markdownCell(change.CurrentValue), markdownCell(change.ScenarioValue)); err != nil {
				return err
			}
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
	loadChanges := scenarioWorkloadChanges(diff)
	if len(loadChanges) == 0 {
		if _, err := fmt.Fprintln(stdout, "- No load changes."); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "| Resource | Unit | Capacity Before | Capacity After | Overload Before | Overload After |"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, "|---|---:|---:|---:|---:|---:|"); err != nil {
			return err
		}
		for _, change := range loadChanges {
			prior, after := change.Before, change.After
			name := after.ResourceName
			if name == "" {
				name = after.ResourceID
			}
			if _, err := fmt.Fprintf(stdout, "| `%s` | %s | %g | %g | %g | %g |\n",
				name, after.Unit, prior.Capacity, after.Capacity, prior.Overload, after.Overload); err != nil {
				return err
			}
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

type scenarioWorkloadChange struct {
	Before app.ResourceWorkload
	After  app.ResourceWorkload
}

func scenarioWorkloadChanges(diff app.ScenarioDiffResult) []scenarioWorkloadChange {
	before := map[string]app.ResourceWorkload{}
	for _, row := range diff.WorkloadBefore {
		before[row.ResourceID+"\x00"+row.Unit] = row
	}
	changes := []scenarioWorkloadChange{}
	for _, after := range diff.WorkloadAfter {
		prior := before[after.ResourceID+"\x00"+after.Unit]
		if prior.Capacity != after.Capacity || prior.Overload != after.Overload {
			changes = append(changes, scenarioWorkloadChange{Before: prior, After: after})
		}
	}
	return changes
}

func writeScenarioDiffText(stdout *os.File, diff app.ScenarioDiffResult) error {
	if _, err := fmt.Fprintf(stdout, "Scenario: %s\nName: %s\nAgainst: %s\n", diff.ScenarioID, diff.ScenarioName, diff.Against); err != nil {
		return err
	}
	if diff.Week != "" {
		if _, err := fmt.Fprintf(stdout, "Week: %s\n", diff.Week); err != nil {
			return err
		}
	}
	if len(diff.TaskChanges) > 0 {
		if _, err := fmt.Fprintln(stdout, "\nTASK CHANGES"); err != nil {
			return err
		}
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(tw, "TASK ID\tTITLE\tFIELD\tCURRENT\tSCENARIO"); err != nil {
			return err
		}
		for _, change := range diff.TaskChanges {
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", change.TaskID, change.TaskTitle, change.Field, change.CurrentValue, change.ScenarioValue); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(stdout, "\nAFFECTED RESOURCES"); err != nil {
		return err
	}
	if len(diff.AffectedResources) == 0 {
		if _, err := fmt.Fprintln(stdout, "none"); err != nil {
			return err
		}
	} else {
		for _, resourceID := range diff.AffectedResources {
			if _, err := fmt.Fprintln(stdout, resourceID); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(stdout, "\nWORKLOAD CHANGES"); err != nil {
		return err
	}
	loadChanges := scenarioWorkloadChanges(diff)
	if len(loadChanges) == 0 {
		if _, err := fmt.Fprintln(stdout, "No load changes."); err != nil {
			return err
		}
	} else {
		tw := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		if _, err := fmt.Fprintln(tw, "RESOURCE\tUNIT\tCAPACITY BEFORE\tCAPACITY AFTER\tOVERLOAD BEFORE\tOVERLOAD AFTER"); err != nil {
			return err
		}
		for _, change := range loadChanges {
			name := change.After.ResourceName
			if name == "" {
				name = change.After.ResourceID
			}
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%g\t%g\t%g\t%g\n", name, change.After.Unit, change.Before.Capacity, change.After.Capacity, change.Before.Overload, change.After.Overload); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}
	return writeDiagnosticsText(stdout, diff.Warnings, diff.Errors)
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
	if err := writeScenarioApplyLinkedProposals(stdout, result.LinkedProposals); err != nil {
		return err
	}
	if len(result.Changes) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Changes"); err != nil {
			return err
		}
		for _, change := range result.Changes {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s` %v -> %v\n", change.TargetID, change.Field, change.PreviousValue, change.NewValue); err != nil {
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
		if err := writeScenarioApplyProposalAudits(stdout, result.LinkedProposals); err != nil {
			return err
		}
	}
	return nil
}

func writeScenarioApplyLinkedProposals(stdout *os.File, proposals []app.ScenarioLinkedProposal) error {
	for _, proposal := range proposals {
		if _, err := fmt.Fprintf(stdout, "- Linked proposal: `%s` (`%s` -> `%s`)\n", proposal.ID, proposal.PreviousStatus, proposal.NewStatus); err != nil {
			return err
		}
	}
	return nil
}

func writeScenarioApplyProposalAudits(stdout *os.File, proposals []app.ScenarioLinkedProposal) error {
	for _, proposal := range proposals {
		if proposal.DecisionAuditRecordPreview == nil {
			continue
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s` for `%s`\n", proposal.DecisionAuditRecordPreview.ID, proposal.DecisionAuditRecordPreview.Operation, proposal.ID); err != nil {
			return err
		}
	}
	return nil
}
