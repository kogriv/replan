package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kogriv/replan/internal/app"
)

func writeEstimateReportMarkdown(out io.Writer, result app.EstimateReportResult) error {
	if _, err := fmt.Fprintf(out, "# Estimate accuracy: %s\n\n", markdownCell(firstNonEmptyCLI(result.ProjectName, result.ProjectID))); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		for _, diagnostic := range result.Errors {
			if _, err := fmt.Fprintf(out, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(out, "- Baseline: `%s` (`%s`)\n- Counted actual types: `%s`\n\n", result.BaselineID, result.BaselineSealStatus, strings.Join(result.ActualTypes, "`, `")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "| Resource | Unit | Finished baseline | Finished planned | Finished actual | Finished A/B | All baseline | All planned | All actual |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "|---|---|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, row := range result.Resources {
		finishedActual, finishedRatio := estimateNumber(row.Finished.ActualAmount), estimateRatio(row.Finished.ActualBaselineRatio)
		allActual := estimateNumber(row.All.ActualAmount)
		if !row.ActualsRecorded {
			finishedActual, finishedRatio, allActual = "no actuals", "no actuals", "no actuals"
		}
		if _, err := fmt.Fprintf(out, "| `%s` | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			row.ResourceID, row.Unit, estimateNumber(row.Finished.BaselineAmount), estimateNumber(row.Finished.CurrentPlannedAmount),
			finishedActual, finishedRatio, estimateNumber(row.All.BaselineAmount), estimateNumber(row.All.CurrentPlannedAmount), allActual); err != nil {
			return err
		}
	}
	if err := writeEstimateSplitGroupsMarkdown(out, result.SplitGroups); err != nil {
		return err
	}
	if err := writeEstimateCalibrationsMarkdown(out, result.Calibrations); err != nil {
		return err
	}
	for _, task := range result.Tasks {
		if err := writeEstimateTaskMarkdown(out, task); err != nil {
			return err
		}
	}
	return nil
}

func writeEstimateCalibrationsMarkdown(out io.Writer, calibrations []app.CalibrationSummary) error {
	if _, err := fmt.Fprintln(out, "\n### Calibrations from this baseline"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if len(calibrations) == 0 {
		_, err := fmt.Fprintln(out, "No calibrations use this baseline.")
		return err
	}
	for _, calibration := range calibrations {
		if _, err := fmt.Fprintf(out, "- `%s`: factor %s, total ratio %s, p25/p75 %s/%s, %d tasks, seal %s\n", calibration.ID, estimateNumber(calibration.Factor), estimateNumber(calibration.TotalRatio), estimateNumber(calibration.P25), estimateNumber(calibration.P75), calibration.Count, calibration.SealStatus); err != nil {
			return err
		}
		for _, band := range calibration.SizeBands {
			label := "above 4"
			if band.UpTo != nil {
				label = "up to " + estimateNumber(*band.UpTo)
			}
			if _, err := fmt.Fprintf(out, "  - %s: %d tasks, median %s\n", label, band.Count, estimateNumber(band.Median)); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeEstimateSplitGroupsMarkdown(out io.Writer, groups []app.EstimateSplitGroupSummary) error {
	if _, err := fmt.Fprintln(out, "\n### Split groups"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if len(groups) == 0 {
		_, err := fmt.Fprintln(out, "No recognized resource splits.")
		return err
	}
	if _, err := fmt.Fprintln(out, "| Original resource | Split-out resources | Unit | Finished baseline | Finished planned | Finished actual | Finished A/B | All baseline | All planned | All actual |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "|---|---|---|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, group := range groups {
		finishedActual, finishedRatio := estimateNumber(group.Finished.ActualAmount), estimateRatio(group.Finished.ActualBaselineRatio)
		allActual := estimateNumber(group.All.ActualAmount)
		if !group.ActualsRecorded {
			finishedActual, finishedRatio, allActual = "no actuals", "no actuals", "no actuals"
		}
		if _, err := fmt.Fprintf(out, "| `%s` | `%s` | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			group.OriginalResourceID, strings.Join(group.SplitResourceIDs, "`, `"), group.Unit,
			estimateNumber(group.Finished.BaselineAmount), estimateNumber(group.Finished.CurrentPlannedAmount), finishedActual,
			finishedRatio, estimateNumber(group.All.BaselineAmount), estimateNumber(group.All.CurrentPlannedAmount), allActual); err != nil {
			return err
		}
	}
	return nil
}

func writeEstimateTaskMarkdown(out io.Writer, task app.EstimateTask) error {
	if _, err := fmt.Fprintf(out, "\n## %s\n\n", markdownCell(firstNonEmptyCLI(task.Title, task.TaskID))); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "`%s` · status `%s` · open `%t` · actuals partial `%t`\n\n", task.TaskID, task.TrackingStatus, task.Open, task.ActualsPartial); err != nil {
		return err
	}
	schedule := task.Schedule
	if _, err := fmt.Fprintf(out, "Schedule: baseline `%s` → `%s`; actual `%s` (%s) → `%s` (%s); finish variance %s calendar / %s working days.\n\n",
		emptyMarkdownDate(schedule.BaselinePlannedStart), emptyMarkdownDate(schedule.BaselinePlannedFinish), emptyMarkdownDate(schedule.ActualStart),
		schedule.ActualStartSource, emptyMarkdownDate(schedule.ActualFinish), schedule.ActualFinishSource,
		estimateInt(schedule.FinishVarianceCalendarDays), estimateInt(schedule.FinishVarianceWorkingDays)); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "| Demand | State | Resource | Unit | Baseline | Planned | Remaining | Actual | Variance | A/B | Re-estimate |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "|---|---|---|---|---:|---:|---:|---:|---:|---:|---|"); err != nil {
		return err
	}
	for _, demand := range task.Demands {
		if _, err := fmt.Fprintf(out, "| `%s` | %s | `%s` | %s | %s | %s | %s | %s | %s | %s | %t |\n",
			demand.DemandID, demand.State, demand.ResourceID, demand.Unit, estimateNumber(demand.BaselineAmount),
			estimateNumber(demand.CurrentPlannedAmount), estimateNumber(demand.CurrentRemainingAmount), estimateNumber(demand.ActualAmount),
			estimateNumber(demand.VarianceAmount), estimateRatio(demand.ActualBaselineRatio), demand.Reestimated); err != nil {
			return err
		}
	}
	for _, total := range task.Totals {
		if _, err := fmt.Fprintf(out, "| **Total** |  |  | **%s** | **%s** | **%s** | **%s** | **%s** | **%s** | **%s** |  |\n",
			total.Unit, estimateNumber(total.BaselineAmount), estimateNumber(total.CurrentPlannedAmount), estimateNumber(total.CurrentRemainingAmount),
			estimateNumber(total.ActualAmount), estimateNumber(total.VarianceAmount), estimateRatio(total.ActualBaselineRatio)); err != nil {
			return err
		}
	}
	for _, split := range task.SplitComparisons {
		basis := split.BaselineBasis
		if split.BaselineDemandID != "" {
			basis += " `" + split.BaselineDemandID + "`"
		}
		if _, err := fmt.Fprintf(out, "\nSplit comparison: added `%s` (%s %s) uses %s as baseline %s %s.\n",
			split.AddedDemandID, estimateNumber(split.CurrentPlannedAmount), split.Unit, basis, estimateNumber(split.BaselineAmount), split.Unit); err != nil {
			return err
		}
	}
	return nil
}

func estimateNumber(value float64) string {
	formatted := strconv.FormatFloat(value, 'f', 6, 64)
	formatted = strings.TrimRight(formatted, "0")
	return strings.TrimRight(formatted, ".")
}

func estimateRatio(value *float64) string {
	if value == nil {
		return "—"
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

func estimateInt(value *int) string {
	if value == nil {
		return "—"
	}
	return strconv.Itoa(*value)
}

func emptyMarkdownDate(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func markdownCell(value string) string {
	return strings.ReplaceAll(value, "|", "\\|")
}

func firstNonEmptyCLI(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return "—"
}
