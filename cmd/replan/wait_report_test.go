package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
)

func TestWaitReportCLIIsReadOnlyAndValidatesDates(t *testing.T) {
	project := copyCmdProjectFixture(t)
	manifest := filepath.Join(project, ".replan", "project.yaml")
	before := readFileString(t, manifest)
	stdout, stderr := tempOutputFiles(t)
	if code := run([]string{"report", "waits", "--project", project, "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	output := readFileString(t, stdout.Name())
	if !strings.Contains(output, `"command": "report waits"`) || !strings.Contains(output, `"no_status_history_count": 2`) {
		t.Fatalf("unexpected output: %s", output)
	}
	if after := readFileString(t, manifest); after != before {
		t.Fatalf("report changed project manifest")
	}

	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"report", "waits", "--project", project, "--since", "2026-11-02", "--until", "2026-11-01", "--format", "json"}, stdout, stderr); code != exitValidationFailed {
		t.Fatalf("invalid range exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if output := readFileString(t, stdout.Name()); !strings.Contains(output, `"code": "invalid_date_range"`) {
		t.Fatalf("missing refusal: %s", output)
	}
}

func TestWaitReportMarkdownGolden(t *testing.T) {
	result := app.WaitReportResult{
		ProjectID: "project.waits", ProjectName: "Wait fixture", StatusDate: "2026-11-12", Since: "2026-11-01", Until: "2026-11-30", FilterAttribution: "span_start", NoStatusHistoryCount: 1,
		Aggregates: []app.WaitAggregate{
			{WaitKind: "in_progress", Stats: app.WaitStatistics{Count: 1, MedianHours: 77.5, P75Hours: 77.5, MaxHours: 77.5, MedianWorkingDays: 3, P75WorkingDays: 3, MaxWorkingDays: 3, MedianCalendarDays: 3, P75CalendarDays: 3, MaxCalendarDays: 3}},
			{WaitKind: "review_wait", Stats: app.WaitStatistics{Count: 1, MedianHours: 2.5, P75Hours: 2.5, MaxHours: 2.5}},
		},
		TagAggregates: []app.WaitGroup{{Name: "frequent", SpanCount: 5, Aggregates: []app.WaitAggregate{
			{WaitKind: "in_progress", Stats: app.WaitStatistics{Count: 1, MedianHours: 77.5, P75Hours: 77.5, MaxHours: 77.5, MedianWorkingDays: 3, P75WorkingDays: 3, MaxWorkingDays: 3, MedianCalendarDays: 3, P75CalendarDays: 3, MaxCalendarDays: 3}},
			{WaitKind: "blocked_wait", Stats: app.WaitStatistics{Count: 5, MedianHours: 24, P75Hours: 24, MaxHours: 72, MedianWorkingDays: 1, P75WorkingDays: 2, MaxWorkingDays: 3, MedianCalendarDays: 1, P75CalendarDays: 2, MaxCalendarDays: 3}},
		}}},
		AssigneeAggregates: []app.WaitGroup{{Name: "resource.owner", SpanCount: 2, Aggregates: []app.WaitAggregate{
			{WaitKind: "in_progress", Stats: app.WaitStatistics{Count: 1, MedianHours: 77.5, P75Hours: 77.5, MaxHours: 77.5, MedianWorkingDays: 3, P75WorkingDays: 3, MaxWorkingDays: 3, MedianCalendarDays: 3, P75CalendarDays: 3, MaxCalendarDays: 3}},
			{WaitKind: "review_wait", Stats: app.WaitStatistics{Count: 1, MedianHours: 2.5, P75Hours: 2.5, MaxHours: 2.5}},
		}}},
		Tasks:                 []app.WaitTask{{TaskID: "task.review", Title: "Owner review", Tags: []string{"frequent"}, AssigneeID: "resource.owner", Spans: []app.WaitSpan{{Status: "review_ready", WaitKind: "review_wait", From: "2026-11-06", To: "2026-11-06", FromAt: "2026-11-06T15:30:00+07:00", ToAt: "2026-11-06T18:00:00+07:00", Hours: 2.5, TimePrecision: "timestamp"}}, OpenSpans: []app.WaitSpan{}}},
		LeadTimeComparisons:   []app.WaitLeadTimeComparison{{TaskID: "task.review", WaitKind: "review_wait", Status: "review_ready", From: "2026-11-06", To: "2026-11-06", PlannedAmount: 2, PlannedUnit: "working_days", MeasuredAmount: 0, Difference: -2, LeadTimeReason: "owner review"}},
		MissingReviewLeadTime: []app.WaitMissingReviewLeadTime{{TaskID: "task.unplanned-review", ReviewWaitCount: 2, MedianWorkingDays: 2.5, MedianCalendarDays: 4}},
		Warnings:              []app.Diagnostic{{Code: "no_status_history", Message: "1 tasks have no tracking status history", Severity: "warning"}}, Errors: []app.Diagnostic{},
	}
	var output bytes.Buffer
	if err := writeWaitReportMarkdown(&output, result); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join("testdata", "waits.golden.md")
	if os.Getenv("REPLAN_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, output.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (REPLAN_UPDATE_GOLDEN=1 writes it)", path, err)
	}
	if output.String() != string(want) {
		t.Fatalf("markdown differs from %s\n--- got ---\n%s\n--- want ---\n%s", path, output.String(), want)
	}
}
