package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func TestWaitReportMeasuresAndAggregatesStatusHistory(t *testing.T) {
	ws := waitReportWorkspace()
	result, err := app.NewReportService(stubLoader{ws: ws}).Waits(context.Background(), app.WaitReportRequest{Now: mustWaitTime("2026-11-12T12:00:00+07:00")})
	if err != nil {
		t.Fatalf("Waits returned error: %v; diagnostics=%#v", err, result.Errors)
	}
	if result.NoStatusHistoryCount != 1 || len(result.NoStatusHistoryTasks) != 1 || result.NoStatusHistoryTasks[0] != "task.no-history" {
		t.Fatalf("no-history result = %#v", result)
	}
	if len(result.OutOfOrderTasks) != 1 || result.OutOfOrderTasks[0] != "task.out-of-order" {
		t.Fatalf("out-of-order tasks = %#v", result.OutOfOrderTasks)
	}
	review := waitTaskByID(t, result.Tasks, "task.review")
	reviewSpan := waitSpanByKind(t, review.Spans, "review_wait")
	if reviewSpan.From != "2026-11-06" || reviewSpan.To != "2026-11-06" || reviewSpan.Hours != 2.5 || reviewSpan.TimePrecision != "timestamp" || reviewSpan.WorkingDays != 0 || reviewSpan.CalendarDays != 0 || reviewSpan.Open {
		t.Fatalf("review span = %#v", reviewSpan)
	}
	if len(review.OpenSpans) != 0 {
		t.Fatalf("terminal accepted status was reported as open: %#v", review.OpenSpans)
	}
	start := waitSpanByKind(t, review.Spans, "start_wait")
	if start.WorkingDays != 1 || start.CalendarDays != 1 {
		t.Fatalf("start span = %#v", start)
	}
	open := waitTaskByID(t, result.Tasks, "task.open")
	if len(open.OpenSpans) != 1 || !open.OpenSpans[0].Open || open.OpenSpans[0].From != "2026-11-09" || open.OpenSpans[0].To != ws.Project.DefaultStatusDate || open.OpenSpans[0].ToAt != "2026-11-12T12:00:00+07:00" {
		t.Fatalf("open spans = %#v", open.OpenSpans)
	}
	legacy := waitTaskByID(t, result.Tasks, "task.legacy")
	legacySpan := waitSpanByKind(t, legacy.Spans, "review_wait")
	if legacySpan.TimePrecision != "date" || legacySpan.Hours != 24 || legacySpan.FromAt != "2026-11-05T00:00:00+07:00" {
		t.Fatalf("legacy precision span = %#v", legacySpan)
	}
	reopened := waitTaskByID(t, result.Tasks, "task.reopened")
	if span := waitSpanByStatus(t, reopened.Spans, "accepted"); span.Open || span.Hours != 24 {
		t.Fatalf("closed terminal span before reopen = %#v", span)
	}
	for _, aggregate := range result.Aggregates {
		if aggregate.WaitKind == "review_wait" && (aggregate.Stats.Count != 4 || aggregate.Stats.MedianHours != 24 || aggregate.Stats.P75Hours != 24) {
			t.Fatalf("open review wait entered aggregate: %#v", aggregate)
		}
		if aggregate.WaitKind == "in_progress" && (aggregate.Stats.Count != 2 || aggregate.Stats.MedianHours != 50.75 || aggregate.Stats.MaxHours != 77.5) {
			t.Fatalf("work-time aggregate = %#v", aggregate)
		}
	}
	blocked := waitSpanByKind(t, waitTaskByID(t, result.Tasks, "task.blocked-1").Spans, "blocked_wait")
	if blocked.WorkingDays != 1 || blocked.CalendarDays != 3 {
		t.Fatalf("weekend span = %#v", blocked)
	}
	if len(result.TagAggregates) != 1 || result.TagAggregates[0].Name != "frequent" || result.TagAggregates[0].SpanCount != 7 {
		t.Fatalf("tag threshold aggregates = %#v", result.TagAggregates)
	}
	if len(result.AssigneeAggregates) == 0 || result.AssigneeAggregates[0].Name != "resource.owner" {
		t.Fatalf("assignee aggregates = %#v", result.AssigneeAggregates)
	}
	if len(result.LeadTimeComparisons) != 1 {
		t.Fatalf("lead comparisons = %#v", result.LeadTimeComparisons)
	}
	comparison := result.LeadTimeComparisons[0]
	if comparison.TaskID != "task.review" || comparison.WaitKind != "review_wait" || comparison.PlannedAmount != 2 || comparison.MeasuredAmount != 0 || comparison.Difference != -2 {
		t.Fatalf("lead comparison = %#v", comparison)
	}
	if len(result.MissingReviewLeadTime) != 3 || result.MissingReviewLeadTime[2].TaskID != "task.review-unplanned" || result.MissingReviewLeadTime[2].MedianWorkingDays != 1 {
		t.Fatalf("missing lead-time hints = %#v", result.MissingReviewLeadTime)
	}
}

func TestWaitReportSinceUntilAndTagFilter(t *testing.T) {
	result, err := app.NewReportService(stubLoader{ws: waitReportWorkspace()}).Waits(context.Background(), app.WaitReportRequest{Since: "2026-11-06", Until: "2026-11-06", Tag: "review", Now: mustWaitTime("2026-11-12T12:00:00+07:00")})
	if err != nil {
		t.Fatal(err)
	}
	if result.NoStatusHistoryCount != 0 || len(result.TagAggregates) != 0 || len(result.Tasks) != 1 {
		t.Fatalf("filtered result = %#v", result)
	}
	span := waitSpanByKind(t, result.Tasks[0].Spans, "review_wait")
	if span.From != "2026-11-06" || span.To != "2026-11-06" || span.Hours != 2.5 || result.FilterAttribution != "span_start" {
		t.Fatalf("start-attributed span = %#v result=%#v", span, result)
	}
}

func waitReportWorkspace() domain.Workspace {
	tasks := []domain.Task{
		{ID: "task.review", Title: "Review", TrackingStatus: "accepted", Tags: []string{"review", "frequent"}, AssigneeHint: domain.Ref{Kind: "resource", ID: "resource.owner"}, LeadTime: &domain.LeadTime{WorkingDays: 2, Reason: "owner review"}},
		{ID: "task.open", Title: "Open review", TrackingStatus: "review_ready", Tags: []string{"open"}, AssigneeHint: domain.Ref{Kind: "resource", ID: "resource.owner"}},
		{ID: "task.review-unplanned", Title: "Unplanned review", TrackingStatus: "accepted", Tags: []string{"frequent"}, AssigneeHint: domain.Ref{Kind: "resource", ID: "resource.owner"}},
		{ID: "task.blocked-1", TrackingStatus: "in_progress", Tags: []string{"frequent"}, AssigneeHint: domain.Ref{ID: "resource.owner"}},
		{ID: "task.blocked-2", TrackingStatus: "in_progress", Tags: []string{"frequent"}, AssigneeHint: domain.Ref{ID: "resource.owner"}},
		{ID: "task.blocked-3", TrackingStatus: "in_progress", Tags: []string{"frequent"}, AssigneeHint: domain.Ref{ID: "resource.owner"}},
		{ID: "task.blocked-4", TrackingStatus: "in_progress", Tags: []string{"frequent"}, AssigneeHint: domain.Ref{ID: "resource.owner"}},
		{ID: "task.out-of-order", TrackingStatus: "in_progress", Tags: []string{"frequent"}, AssigneeHint: domain.Ref{ID: "resource.owner"}},
		{ID: "task.legacy", TrackingStatus: "accepted"},
		{ID: "task.reopened", TrackingStatus: "in_progress"},
		{ID: "task.historical-append", TrackingStatus: "accepted"},
		{ID: "task.no-history", TrackingStatus: "not_started"},
	}
	audits := []domain.AuditRecord{
		waitStatusAt("review.plan", "task.review", "planned", "2026-11-02", "2026-11-02T09:00:00+07:00"),
		waitStatusAt("review.start", "task.review", "in_progress", "2026-11-03", "2026-11-03T10:00:00+07:00"),
		waitStatusAt("review.ready", "task.review", "review_ready", "2026-11-06", "2026-11-06T15:30:00+07:00"),
		waitStatusAt("review.accept", "task.review", "accepted", "2026-11-06", "2026-11-06T18:00:00+07:00"),
		waitStatusAt("open.ready", "task.open", "review_ready", "2026-11-10", "2026-11-09T10:00:00+07:00"),
		{ID: "open.ready.correction", Operation: "actual_correct_status", CorrectsRef: &domain.Ref{Kind: "audit_record", ID: "open.ready"}, StatusDate: "2026-11-09", OccurredAt: "2026-11-11T12:00:00Z"},
		waitStatus("unplanned.ready", "task.review-unplanned", "review_ready", "2026-11-05"),
		waitStatus("unplanned.accept", "task.review-unplanned", "accepted", "2026-11-06"),
		waitStatusAt("legacy.ready", "task.legacy", "review_ready", "2026-11-05", "2026-11-05"),
		waitStatusAt("legacy.accept", "task.legacy", "accepted", "2026-11-06", "2026-11-06"),
		waitStatus("reopen.work", "task.reopened", "in_progress", "2026-11-01"),
		waitStatus("reopen.accept", "task.reopened", "accepted", "2026-11-02"),
		waitStatus("reopen.again", "task.reopened", "in_progress", "2026-11-03"),
		// Newer fact appears first in file order; effective history sorts by occurred_at.
		waitStatusAt("historical.accept", "task.historical-append", "accepted", "2026-11-06", "2026-11-06T12:00:00+07:00"),
		waitStatusAt("historical.ready", "task.historical-append", "review_ready", "2026-11-05", "2026-11-05T12:00:00+07:00"),
	}
	for index := 1; index <= 4; index++ {
		id := "task.blocked-" + string(rune('0'+index))
		if index == 1 {
			audits = append(audits, waitStatusAt(id+".blocked", id, "blocked", "2026-11-06", "2026-11-06T12:00:00+07:00"), waitStatusAt(id+".start", id, "in_progress", "2026-11-09", "2026-11-09T12:00:00+07:00"))
		} else {
			audits = append(audits, waitStatus(id+".blocked", id, "blocked", "2026-11-03"), waitStatus(id+".start", id, "in_progress", "2026-11-04"))
		}
	}
	audits = append(audits,
		waitStatusAt("order.first", "task.out-of-order", "blocked", "2026-11-05", "2026-11-04T10:00:00+07:00"),
		waitStatusAt("order.second", "task.out-of-order", "in_progress", "2026-11-04", "2026-11-04T11:00:00+07:00"),
	)
	return domain.Workspace{
		Project: domain.Project{ID: "project.waits", Name: "Wait fixture", Timezone: "Asia/Novosibirsk", DefaultStatusDate: "2026-11-12"},
		Tasks:   tasks, AuditRecords: audits,
		Availabilities: []domain.Availability{{ResourceRef: domain.Ref{ID: "resource.owner"}, Windows: []domain.AvailabilityWindow{{Days: []string{"monday", "tuesday", "wednesday", "thursday", "friday"}}}}},
	}
}

func waitStatus(id, taskID, status, date string) domain.AuditRecord {
	return waitStatusAt(id, taskID, status, date, date+"T12:00:00+07:00")
}

func waitStatusAt(id, taskID, status, date, occurredAt string) domain.AuditRecord {
	return domain.AuditRecord{ID: id, Operation: "actual_add_status", TargetRef: domain.Ref{Kind: "task", ID: taskID}, NewStatus: status, StatusDate: date, OccurredAt: occurredAt}
}

func waitTaskByID(t *testing.T, tasks []app.WaitTask, id string) app.WaitTask {
	t.Helper()
	for _, task := range tasks {
		if task.TaskID == id {
			return task
		}
	}
	t.Fatalf("task %s not found in %#v", id, tasks)
	return app.WaitTask{}
}

func waitSpanByKind(t *testing.T, spans []app.WaitSpan, kind string) app.WaitSpan {
	t.Helper()
	for _, span := range spans {
		if span.WaitKind == kind {
			return span
		}
	}
	t.Fatalf("span kind %s not found in %#v", kind, spans)
	return app.WaitSpan{}
}

func waitSpanByStatus(t *testing.T, spans []app.WaitSpan, status string) app.WaitSpan {
	t.Helper()
	for _, span := range spans {
		if span.Status == status {
			return span
		}
	}
	t.Fatalf("span status %s not found in %#v", status, spans)
	return app.WaitSpan{}
}

func mustWaitTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return parsed
}
