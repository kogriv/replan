package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/kogriv/replan/internal/app"
)

func writeWaitReportMarkdown(out io.Writer, result app.WaitReportResult) error {
	if err := writeWaitHeader(out, result); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return nil
	}
	if err := writeWaitGroupSections(out, result); err != nil {
		return err
	}
	if err := writeWaitTasks(out, result.Tasks); err != nil {
		return err
	}
	if err := writeWaitComparisonSections(out, result); err != nil {
		return err
	}
	return writeWaitWarnings(out, result.Warnings)
}

func writeWaitHeader(out io.Writer, result app.WaitReportResult) error {
	if _, err := fmt.Fprintf(out, "# Measured waits: %s\n\n", markdownCell(firstNonEmptyCLI(result.ProjectName, result.ProjectID))); err != nil {
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
	if _, err := fmt.Fprintf(out, "- Status date: `%s`\n", emptyMarkdownDate(result.StatusDate)); err != nil {
		return err
	}
	if result.Since != "" {
		if _, err := fmt.Fprintf(out, "- Since bound: span start date on or after `%s`\n", result.Since); err != nil {
			return err
		}
	}
	if result.Until != "" {
		if _, err := fmt.Fprintf(out, "- Until bound: span start date on or before `%s`\n", result.Until); err != nil {
			return err
		}
	}
	if result.Tag != "" {
		if _, err := fmt.Fprintf(out, "- Tag: `%s`\n", result.Tag); err != nil {
			return err
		}
	}
	if result.FilterAttribution != "" {
		if _, err := fmt.Fprintln(out, "- Filter attribution: spans are attributed by their start date; durations are not clipped"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(out, "- Tasks without status history: %d\n", result.NoStatusHistoryCount); err != nil {
		return err
	}
	return nil
}

func writeWaitGroupSections(out io.Writer, result app.WaitReportResult) error {
	if err := writeWaitAggregateTable(out, "Overall waits and work time", result.Aggregates); err != nil {
		return err
	}
	if len(result.TagAggregates) > 0 {
		if _, err := fmt.Fprintln(out, "\n## Per tag (at least 5 measured waits)"); err != nil {
			return err
		}
		if err := writeWaitGroupedAggregateTable(out, "Tag", result.TagAggregates); err != nil {
			return err
		}
	}
	if len(result.AssigneeAggregates) > 0 {
		if _, err := fmt.Fprintln(out, "\n## Per assignee hint resource"); err != nil {
			return err
		}
		if err := writeWaitGroupedAggregateTable(out, "Assignee", result.AssigneeAggregates); err != nil {
			return err
		}
	}
	return nil
}

func writeWaitTasks(out io.Writer, tasks []app.WaitTask) error {
	if _, err := fmt.Fprintln(out, "\n## Task spans"); err != nil {
		return err
	}
	for _, task := range tasks {
		if _, err := fmt.Fprintf(out, "\n### %s\n\n`%s`\n\n", markdownCell(firstNonEmptyCLI(task.Title, task.TaskID)), task.TaskID); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "| Status | Kind | From date | To date | From at | To at | Hours | Precision | Working days | Calendar days | Open |"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "|---|---|---|---|---|---|---:|---|---:|---:|---|"); err != nil {
			return err
		}
		for _, span := range append(append([]app.WaitSpan{}, task.Spans...), task.OpenSpans...) {
			kind := span.WaitKind
			if kind == "status_span" {
				kind = "generic"
			}
			kind = waitKindLabel(kind)
			if _, err := fmt.Fprintf(out, "| `%s` | %s | %s | %s | %s | %s | %s | %s | %d | %d | %t |\n", span.Status, kind, span.From, span.To, span.FromAt, span.ToAt, waitHours(span.Hours), span.TimePrecision, span.WorkingDays, span.CalendarDays, span.Open); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeWaitComparisonSections(out io.Writer, result app.WaitReportResult) error {
	if len(result.LeadTimeComparisons) > 0 {
		if _, err := fmt.Fprintln(out, "\n## Planned lead time comparison\n\nLead time is compared with the measured wait selected by its reason; an unclassified reason models `review_wait`.\n\n| Task | Kind | Measured span | Planned | Measured | Difference | Reason |\n|---|---|---|---:|---:|---:|---|"); err != nil {
			return err
		}
		for _, row := range result.LeadTimeComparisons {
			if _, err := fmt.Fprintf(out, "| `%s` | %s | %s → %s | %s %s | %d %s | %s %s | %s |\n", row.TaskID, row.WaitKind, row.From, row.To, waitNumber(row.PlannedAmount), row.PlannedUnit, row.MeasuredAmount, row.PlannedUnit, waitNumber(row.Difference), row.PlannedUnit, markdownCell(row.LeadTimeReason)); err != nil {
				return err
			}
		}
	}
	if len(result.MissingReviewLeadTime) > 0 {
		if _, err := fmt.Fprintln(out, "\n## Review waits without planned lead time\n\nThese are hints only; the report makes no proposal or write.\n\n| Task | Review waits | Median working days | Median calendar days |\n|---|---:|---:|---:|"); err != nil {
			return err
		}
		for _, row := range result.MissingReviewLeadTime {
			if _, err := fmt.Fprintf(out, "| `%s` | %d | %s | %s |\n", row.TaskID, row.ReviewWaitCount, waitNumber(row.MedianWorkingDays), waitNumber(row.MedianCalendarDays)); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeWaitWarnings(out io.Writer, warnings []app.Diagnostic) error {
	if len(warnings) > 0 {
		if _, err := fmt.Fprintln(out, "\n## Warnings"); err != nil {
			return err
		}
		for _, warning := range warnings {
			if _, err := fmt.Fprintf(out, "- `%s`: %s\n", warning.Code, warning.Message); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeWaitAggregateTable(out io.Writer, title string, rows []app.WaitAggregate) error {
	if _, err := fmt.Fprintf(out, "\n## %s\n\n", title); err != nil {
		return err
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintln(out, "No completed named waits.")
		return err
	}
	if _, err := fmt.Fprintln(out, "| Kind | Count | Hours median | p75 | max | Working median | p75 | max | Calendar median | p75 | max |\n|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, row := range rows {
		s := row.Stats
		if _, err := fmt.Fprintf(out, "| %s | %d | %s | %s | %s | %s | %s | %d | %s | %s | %d |\n", waitKindLabel(row.WaitKind), s.Count, waitHours(s.MedianHours), waitHours(s.P75Hours), waitHours(s.MaxHours), waitNumber(s.MedianWorkingDays), waitNumber(s.P75WorkingDays), s.MaxWorkingDays, waitNumber(s.MedianCalendarDays), waitNumber(s.P75CalendarDays), s.MaxCalendarDays); err != nil {
			return err
		}
	}
	return nil
}

func writeWaitGroupedAggregateTable(out io.Writer, groupLabel string, groups []app.WaitGroup) error {
	if _, err := fmt.Fprintf(out, "\n| %s | Kind | Count | Hours median | p75 | max | Working median | p75 | max | Calendar median | p75 | max |\n", groupLabel); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(out, "|---|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|"); err != nil {
		return err
	}
	for _, group := range groups {
		for _, row := range group.Aggregates {
			s := row.Stats
			if _, err := fmt.Fprintf(out, "| `%s` | %s | %d | %s | %s | %s | %s | %s | %d | %s | %s | %d |\n", group.Name, waitKindLabel(row.WaitKind), s.Count, waitHours(s.MedianHours), waitHours(s.P75Hours), waitHours(s.MaxHours), waitNumber(s.MedianWorkingDays), waitNumber(s.P75WorkingDays), s.MaxWorkingDays, waitNumber(s.MedianCalendarDays), waitNumber(s.P75CalendarDays), s.MaxCalendarDays); err != nil {
				return err
			}
		}
	}
	return nil
}

func waitKindLabel(kind string) string {
	if kind == "in_progress" {
		return "in_progress (work time)"
	}
	return kind
}

func waitNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(strconv.FormatFloat(value, 'f', 2, 64), "0"), ".")
}

func waitHours(value float64) string { return strconv.FormatFloat(value, 'f', 2, 64) }
