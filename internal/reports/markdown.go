package reports

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

//nolint:gocyclo // This renderer writes the weekly document section by section; splitting one-line output steps would hide the byte-stable order.
func WriteWeeklyMarkdown(w io.Writer, report WeeklyReport) error {
	if _, err := fmt.Fprintf(w, "# Weekly RePlan Report: %s\n\n", report.Week); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- Project: `%s` (%s)\n", report.ProjectName, report.ProjectID); err != nil {
		return err
	}
	if report.ScenarioID != "" {
		if _, err := fmt.Fprintf(w, "> What-if: scenario %s applied in memory; this is not the plan.\n", report.ScenarioID); err != nil {
			return err
		}
	}
	if report.BaselineID != "" {
		if _, err := fmt.Fprintf(w, "- Baseline: `%s`\n", report.BaselineID); err != nil {
			return err
		}
	}
	if report.CurrentPlan != "" {
		if _, err := fmt.Fprintf(w, "- Current plan: `%s`\n", report.CurrentPlan); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\n## Summary"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- Resource rows: %d\n", report.Summary.ResourceRows); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- Overloaded rows: %d\n", report.Summary.Overloaded); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- Warnings: %d\n", report.Summary.WarningCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- Open proposals: %d\n", report.Summary.OpenProposals); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- Evidence records: %d\n", report.Summary.EvidenceCount); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "\n## Workload"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "| Resource | Class | Unit | Capacity | Planned | Actual | Remaining | Overload |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "|---|---|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, row := range report.Resources {
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		if _, err := fmt.Fprintf(w, "| `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
			name,
			row.ResourceClass,
			row.Unit,
			formatAmount(row.Capacity),
			formatAmount(row.Planned),
			formatAmount(row.Actual),
			formatAmount(row.Remaining),
			formatAmount(row.Overload)); err != nil {
			return err
		}
	}

	if len(report.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "\n## Warnings"); err != nil {
			return err
		}
		for _, warning := range report.Warnings {
			if _, err := fmt.Fprintf(w, "- `%s`: %s\n", warning.Code, warning.Message); err != nil {
				return err
			}
		}
	}

	if len(report.Proposals) > 0 {
		if _, err := fmt.Fprintln(w, "\n## Open Proposals"); err != nil {
			return err
		}
		for _, proposal := range report.Proposals {
			if _, err := fmt.Fprintf(w, "- `%s` (%s): %s\n", proposal.ID, proposal.Type, proposal.Rationale); err != nil {
				return err
			}
		}
	}

	if len(report.Evidence) > 0 {
		if _, err := fmt.Fprintln(w, "\n## Evidence"); err != nil {
			return err
		}
		for _, evidence := range report.Evidence {
			if _, err := fmt.Fprintf(w, "- `%s` (%s): %s\n", evidence.ID, evidence.Type, evidence.Summary); err != nil {
				return err
			}
		}
	}

	return nil
}

// WriteProjectStateMarkdown renders the full state report. Each section has
// its own writer; the page is built in memory and written once.
func WriteProjectStateMarkdown(w io.Writer, overview DashboardOverview) error {
	var b strings.Builder
	writeStateHeader(&b, overview)
	writeStateHealth(&b, overview)
	writeStateTracking(&b, overview.Tracking)
	writeStateTrackingLoop(&b, overview.TrackingLoop)
	writeStateResourcePressure(&b, overview.Workload)
	writeStateWarnings(&b, overview.Warnings)
	writeStateNextActions(&b, overview.NextActions)
	_, err := io.WriteString(w, b.String())
	return err
}

func writeStateHeader(b *strings.Builder, overview DashboardOverview) {
	fmt.Fprintf(b, "# RePlan Project State: %s\n\n", overview.ProjectName)
	fmt.Fprintf(b, "- Project: `%s`\n", overview.ProjectID)
	if overview.ScenarioID != "" {
		fmt.Fprintf(b, "> What-if: scenario %s applied in memory; this is not the plan.\n", overview.ScenarioID)
	}
	for _, field := range []struct{ label, value string }{
		{"Week", overview.Week},
		{"Status date", overview.StatusDate},
		{"Current plan", overview.CurrentPlan},
		{"Baseline", overview.BaselineID},
	} {
		if field.value != "" {
			fmt.Fprintf(b, "- %s: `%s`\n", field.label, field.value)
		}
	}
}

func writeStateHealth(b *strings.Builder, overview DashboardOverview) {
	source, health := overview.SourceHealth, overview.Health
	b.WriteString("\n## Sync Health\n")
	fmt.Fprintf(b, "- Source profiles: %d\n", len(source.Profiles))
	fmt.Fprintf(b, "- Tasks with source refs: %d\n", source.TasksWithSourceRefs)
	fmt.Fprintf(b, "- Tasks without source refs: %d\n", source.TasksWithoutSourceRefs)
	fmt.Fprintf(b, "- Resources with source refs: %d\n", source.ResourcesWithSourceRefs)

	b.WriteString("\n## Planning Health\n")
	fmt.Fprintf(b, "- Tasks: %d\n", health.TaskCount)
	fmt.Fprintf(b, "- Accepted tasks: %d\n", health.AcceptedTasks)
	fmt.Fprintf(b, "- Active tasks: %d\n", health.ActiveTasks)
	fmt.Fprintf(b, "- Open dependencies: %d\n", health.OpenDependencies)
	fmt.Fprintf(b, "- Related links: %d\n", health.RelatedLinks)
	fmt.Fprintf(b, "- Missing-demand tasks: %d\n", health.MissingDemandTaskCount)
	fmt.Fprintf(b, "- Open proposals: %d\n", health.OpenProposals)
	fmt.Fprintf(b, "- Scenarios: %d\n", health.ScenarioCount)

	b.WriteString("\n## Tracking Health\n")
	fmt.Fprintf(b, "- Evidence records: %d\n", health.EvidenceCount)
	fmt.Fprintf(b, "- Warnings: %d\n", health.WarningCount)
	fmt.Fprintf(b, "- Errors: %d\n", health.ErrorCount)
}

func writeStateTracking(b *strings.Builder, rows []DashboardTrackingRow) {
	if len(rows) == 0 {
		return
	}
	b.WriteString("\n## Plan vs Actual\n")
	b.WriteString("| Task | Status | Actuals | Evidence | Flags |\n")
	b.WriteString("|---|---|---:|---:|---|\n")
	for _, row := range rows {
		fmt.Fprintf(b, "| `%s` | %s | %d | %d | %s |\n",
			row.TaskID, row.TrackingStatus, row.ActualCount, row.EvidenceCount, joinedOrDash(row.Flags))
	}
	b.WriteString("\n| Task | Demand | Unit | Planned | Actual | Remaining | Forecast | Variance | Flags |\n")
	b.WriteString("|---|---|---:|---:|---:|---:|---:|---:|---|\n")
	for _, row := range rows {
		for _, demand := range row.Demands {
			fmt.Fprintf(b, "| `%s` | `%s` | %s | %s | %s | %s | %s | %s | %s |\n",
				row.TaskID,
				demand.DemandID,
				demand.Unit,
				formatAmount(demand.Planned),
				formatAmount(demand.Actual),
				formatAmount(demand.Remaining),
				formatAmount(demand.Forecast),
				formatAmount(demand.Variance),
				joinedOrDash(demand.Flags))
		}
	}
}

func writeStateTrackingLoop(b *strings.Builder, loop DashboardTrackingLoop) {
	if len(loop.Rows) == 0 && loop.StaleRemainingCount == 0 {
		return
	}
	b.WriteString("\n## Tracking Proposal Loop\n")
	fmt.Fprintf(b, "- Stale remaining rows: %d\n", loop.StaleRemainingCount)
	fmt.Fprintf(b, "- Open tracking proposals: %d\n", loop.OpenProposalCount)
	fmt.Fprintf(b, "- Accepted tracking proposals: %d\n", loop.AcceptedProposalCount)
	fmt.Fprintf(b, "- Covered demand rows: %d\n", loop.CoveredDemandCount)
	fmt.Fprintf(b, "- Actionable demand rows: %d\n", loop.ActionableCount)
	b.WriteString("\n| Task | Demand | Status | Priority | Reason | Proposal | Remaining | Forecast | Next |\n")
	b.WriteString("|---|---|---|---|---|---|---:|---:|---|\n")
	for _, row := range loop.Rows {
		status := row.Status
		if status == "" {
			status = "actionable"
		}
		fmt.Fprintf(b, "| `%s` | `%s` | %s | %s | %s | `%s` | %s | %s | `%s` |\n",
			row.TaskID,
			row.DemandID,
			status,
			orDash(row.Priority),
			orDash(row.Reason),
			orDash(row.ProposalID),
			formatAmount(row.Remaining),
			formatAmount(row.Forecast),
			orDash(row.RecommendedCommand))
	}
}

// writeStateResourcePressure lists overloaded resources only, with a "none"
// row when nothing is overloaded.
func writeStateResourcePressure(b *strings.Builder, workload []ResourceRow) {
	b.WriteString("\n## Resource Pressure\n")
	b.WriteString("| Resource | Unit | Capacity | Planned | Actual | Remaining | Overload |\n")
	b.WriteString("|---|---:|---:|---:|---:|---:|---:|\n")
	overloaded := 0
	for _, row := range workload {
		if row.Overload <= 0 {
			continue
		}
		overloaded++
		name := row.ResourceName
		if name == "" {
			name = row.ResourceID
		}
		fmt.Fprintf(b, "| `%s` | %s | %s | %s | %s | %s | %s |\n",
			name,
			row.Unit,
			formatAmount(row.Capacity),
			formatAmount(row.Planned),
			formatAmount(row.Actual),
			formatAmount(row.Remaining),
			formatAmount(row.Overload))
	}
	if overloaded == 0 {
		b.WriteString("| none |  |  |  |  |  | 0 |\n")
	}
}

func writeStateWarnings(b *strings.Builder, warnings []ReportIssue) {
	if len(warnings) == 0 {
		return
	}
	b.WriteString("\n## Warnings\n")
	for _, warning := range warnings {
		fmt.Fprintf(b, "- `%s`%s: %s\n", warning.Code, codeSuffix(warning.TargetID), warning.Message)
	}
}

// writeStateNextActions groups consecutive actions under their actor heading;
// the actions arrive sorted, person first.
func writeStateNextActions(b *strings.Builder, actions []DashboardNextAction) {
	if len(actions) == 0 {
		return
	}
	b.WriteString("\n## Next Actions\n")
	group := ""
	for i, action := range actions {
		if title := NextActionGroupTitle(action.Actor); i == 0 || title != group {
			group = title
			if group != "" {
				fmt.Fprintf(b, "\n### %s\n\n", group)
			}
		}
		date := ""
		if action.Date != "" {
			date = " (" + action.Date + ")"
		}
		fmt.Fprintf(b, "- `%s`%s%s: %s\n", action.Code, codeSuffix(action.Target), date, action.Label)
		if action.Command != "" {
			fmt.Fprintf(b, "  - `%s`\n", action.Command)
		}
	}
}

// codeSuffix renders an optional id as " `id`", or nothing.
func codeSuffix(id string) string {
	if id == "" {
		return ""
	}
	return " `" + id + "`"
}

func orDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func joinedOrDash(values []string) string {
	return orDash(strings.Join(values, ", "))
}

// NextActionGroupTitle names the group a next action is listed under; an
// action without an actor, such as the empty-state hint, gets no heading.
func NextActionGroupTitle(actor string) string {
	switch actor {
	case NextActionActorPerson:
		return "Waiting on a person"
	case NextActionActorAgent:
		return "An agent can prepare"
	default:
		return ""
	}
}

func formatAmount(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
