package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

const waitDateLayout = "2006-01-02"

type WaitReportRequest struct {
	Since string
	Until string
	Tag   string
	Now   time.Time
}

type WaitReportResult struct {
	ProjectID              string                      `json:"project_id"`
	ProjectName            string                      `json:"project_name"`
	StatusDate             string                      `json:"status_date"`
	Since                  string                      `json:"since,omitempty"`
	Until                  string                      `json:"until,omitempty"`
	Tag                    string                      `json:"tag,omitempty"`
	FilterAttribution      string                      `json:"filter_attribution,omitempty"`
	LeadTimeComparisonRule string                      `json:"lead_time_comparison_rule"`
	NoStatusHistoryCount   int                         `json:"no_status_history_count"`
	NoStatusHistoryTasks   []string                    `json:"no_status_history_tasks"`
	OutOfOrderTasks        []string                    `json:"out_of_order_tasks"`
	Tasks                  []WaitTask                  `json:"tasks"`
	Aggregates             []WaitAggregate             `json:"aggregates"`
	TagAggregates          []WaitGroup                 `json:"tag_aggregates"`
	AssigneeAggregates     []WaitGroup                 `json:"assignee_aggregates"`
	LeadTimeComparisons    []WaitLeadTimeComparison    `json:"lead_time_comparisons"`
	MissingReviewLeadTime  []WaitMissingReviewLeadTime `json:"missing_review_lead_time"`
	Warnings               []Diagnostic                `json:"warnings"`
	Errors                 []Diagnostic                `json:"errors"`
}

type WaitTask struct {
	TaskID     string     `json:"task_id"`
	Title      string     `json:"title,omitempty"`
	Tags       []string   `json:"tags"`
	AssigneeID string     `json:"assignee_hint_resource,omitempty"`
	Spans      []WaitSpan `json:"spans"`
	OpenSpans  []WaitSpan `json:"open_spans"`
}

type WaitSpan struct {
	Status        string  `json:"status"`
	From          string  `json:"from"`
	To            string  `json:"to"`
	FromAt        string  `json:"from_at"`
	ToAt          string  `json:"to_at"`
	Hours         float64 `json:"hours"`
	TimePrecision string  `json:"time_precision"`
	WorkingDays   int     `json:"working_days"`
	CalendarDays  int     `json:"calendar_days"`
	Open          bool    `json:"open"`
	WaitKind      string  `json:"wait_kind,omitempty"`
	elapsedHours  float64
}

type WaitStatistics struct {
	Count              int     `json:"count"`
	MedianWorkingDays  float64 `json:"median_working_days"`
	P75WorkingDays     float64 `json:"p75_working_days"`
	MaxWorkingDays     int     `json:"max_working_days"`
	MedianCalendarDays float64 `json:"median_calendar_days"`
	P75CalendarDays    float64 `json:"p75_calendar_days"`
	MaxCalendarDays    int     `json:"max_calendar_days"`
	MedianHours        float64 `json:"median_hours"`
	P75Hours           float64 `json:"p75_hours"`
	MaxHours           float64 `json:"max_hours"`
}

type WaitAggregate struct {
	WaitKind string         `json:"wait_kind"`
	Stats    WaitStatistics `json:"statistics"`
}

type WaitGroup struct {
	Name       string          `json:"name"`
	SpanCount  int             `json:"span_count"`
	Aggregates []WaitAggregate `json:"aggregates"`
}

type WaitLeadTimeComparison struct {
	TaskID         string  `json:"task_id"`
	WaitKind       string  `json:"wait_kind"`
	Status         string  `json:"status"`
	From           string  `json:"from"`
	To             string  `json:"to"`
	PlannedAmount  float64 `json:"planned_amount"`
	PlannedUnit    string  `json:"planned_unit"`
	MeasuredAmount int     `json:"measured_amount"`
	Difference     float64 `json:"difference"`
	LeadTimeReason string  `json:"lead_time_reason,omitempty"`
}

type WaitMissingReviewLeadTime struct {
	TaskID             string  `json:"task_id"`
	Title              string  `json:"title,omitempty"`
	ReviewWaitCount    int     `json:"review_wait_count"`
	MedianWorkingDays  float64 `json:"median_working_days"`
	MedianCalendarDays float64 `json:"median_calendar_days"`
}

type waitMetricSpan struct {
	span       WaitSpan
	taskID     string
	tags       []string
	assigneeID string
}

type waitEvent struct {
	status    string
	date      time.Time
	at        time.Time
	precision string
	id        string
}

//nolint:gocyclo // Validation, history projection, aggregation, and diagnostics are kept together as one read-only report transaction.
func (s ReportService) Waits(ctx context.Context, req WaitReportRequest) (WaitReportResult, error) {
	ws, loadDiags := loadPlanningWorkspace(ctx, s.loader)
	result := WaitReportResult{
		ProjectID: ws.Project.ID, ProjectName: ws.Project.Name, StatusDate: ws.Project.DefaultStatusDate,
		Since: req.Since, Until: req.Until, Tag: req.Tag,
		LeadTimeComparisonRule: "lead_time reason containing review, block, or start/queue selects review_wait, blocked_wait, or start_wait; an unclassified reason compares with review_wait",
		NoStatusHistoryTasks:   []string{}, OutOfOrderTasks: []string{}, Tasks: []WaitTask{}, Aggregates: []WaitAggregate{}, TagAggregates: []WaitGroup{}, AssigneeAggregates: []WaitGroup{}, LeadTimeComparisons: []WaitLeadTimeComparison{}, MissingReviewLeadTime: []WaitMissingReviewLeadTime{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	since, sinceOK := waitOptionalDate(req.Since)
	until, untilOK := waitOptionalDate(req.Until)
	statusDate, statusOK := waitOptionalDate(result.StatusDate)
	if req.Since != "" && !sinceOK {
		result.Errors = append(result.Errors, waitReportDiagnostic("invalid_since", "--since must be YYYY-MM-DD"))
	}
	if req.Until != "" && !untilOK {
		result.Errors = append(result.Errors, waitReportDiagnostic("invalid_until", "--until must be YYYY-MM-DD"))
	}
	if sinceOK && untilOK && since.After(until) {
		result.Errors = append(result.Errors, waitReportDiagnostic("invalid_date_range", "--since must not be after --until"))
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	if sinceOK || untilOK {
		result.FilterAttribution = "span_start"
	}
	if !statusOK {
		result.Warnings = append(result.Warnings, Diagnostic{Code: "missing_status_date", Message: "project has no valid default_status_date; open status spans are not measured", Severity: "warning", Target: Target{Kind: "project", ID: ws.Project.ID}})
	}

	working := projectWorkingDays(ws.Availabilities)
	location := waitProjectLocation(ws.Project)
	runAt := req.Now
	if runAt.IsZero() {
		runAt = time.Now()
	}
	statusByTask := map[string][]domain.AuditRecord{}
	for _, audit := range EffectiveStatusAudits(ws.AuditRecords) {
		if audit.TargetRef.ID != "" && audit.NewStatus != "" {
			statusByTask[audit.TargetRef.ID] = append(statusByTask[audit.TargetRef.ID], audit)
		}
	}
	creationByTask := waitTaskCreations(ws.Project, ws.AuditRecords, location)
	var metrics []waitMetricSpan
	for _, task := range ws.Tasks {
		if req.Tag != "" && !waitHasTag(task.Tags, req.Tag) {
			continue
		}
		records := statusByTask[task.ID]
		if len(records) == 0 {
			result.NoStatusHistoryTasks = append(result.NoStatusHistoryTasks, task.ID)
			continue
		}
		events, outOfOrder := waitEvents(ws.Project, records, location)
		if outOfOrder {
			result.OutOfOrderTasks = append(result.OutOfOrderTasks, task.ID)
		}
		if created, ok := creationByTask[task.ID]; ok && len(events) > 0 && created.at.Before(events[0].at) && events[0].status == "in_progress" {
			events = append([]waitEvent{created}, events...)
		}
		firstStartBoundary := waitFirstStartBoundary(events)
		row := WaitTask{TaskID: task.ID, Title: task.Title, Tags: append([]string{}, task.Tags...), AssigneeID: task.AssigneeHint.ID, Spans: []WaitSpan{}, OpenSpans: []WaitSpan{}}
		for i, event := range events {
			end, endAt, endPrecision, open := statusDate, time.Time{}, "timestamp", true
			nextStatus := ""
			if i+1 < len(events) {
				end, endAt, endPrecision, open, nextStatus = events[i+1].date, events[i+1].at, events[i+1].precision, false, events[i+1].status
			} else if waitTerminalStatus(event.status) {
				continue
			} else if statusOK {
				endAt = waitOpenEnd(statusDate, location, runAt)
			}
			if end.IsZero() || endAt.IsZero() || end.Before(event.date) || endAt.Before(event.at) {
				continue
			}
			kind := waitKind(event.status, nextStatus)
			if kind == "start_wait" && i != firstStartBoundary {
				kind = "status_span"
			}
			if !waitStartInRange(event.date, since, sinceOK, until, untilOK) {
				continue
			}
			precision := "timestamp"
			if event.precision == "date" || endPrecision == "date" {
				precision = "date"
			}
			elapsedHours := endAt.Sub(event.at).Hours()
			span := WaitSpan{Status: event.status, From: event.date.Format(waitDateLayout), To: end.Format(waitDateLayout), FromAt: event.at.Format(time.RFC3339), ToAt: endAt.Format(time.RFC3339), Hours: waitRound(elapsedHours), TimePrecision: precision, WorkingDays: waitWorkingDays(event.date, end, working), CalendarDays: int(end.Sub(event.date).Hours() / 24), Open: open, WaitKind: kind, elapsedHours: elapsedHours}
			if open {
				row.OpenSpans = append(row.OpenSpans, span)
			} else {
				row.Spans = append(row.Spans, span)
				metrics = append(metrics, waitMetricSpan{span: span, taskID: task.ID, tags: task.Tags, assigneeID: task.AssigneeHint.ID})
			}
		}
		if len(row.Spans) > 0 || len(row.OpenSpans) > 0 {
			result.Tasks = append(result.Tasks, row)
		}
	}
	result.NoStatusHistoryCount = len(result.NoStatusHistoryTasks)
	sort.Strings(result.NoStatusHistoryTasks)
	sort.Strings(result.OutOfOrderTasks)
	if result.NoStatusHistoryCount > 0 {
		result.Warnings = append(result.Warnings, Diagnostic{Code: "no_status_history", Message: fmt.Sprintf("%d tasks have no tracking status history", result.NoStatusHistoryCount), Severity: "warning", Target: Target{Kind: "project", ID: ws.Project.ID}})
	}
	for _, taskID := range result.OutOfOrderTasks {
		result.Warnings = append(result.Warnings, Diagnostic{Code: "status_history_out_of_order", Message: fmt.Sprintf("task %s status history is not in status-date order", taskID), Severity: "warning", Target: Target{Kind: "task", ID: taskID}})
	}
	sort.Slice(result.Tasks, func(i, j int) bool { return result.Tasks[i].TaskID < result.Tasks[j].TaskID })
	result.Aggregates = waitAggregates(metrics)
	if req.Tag == "" {
		result.TagAggregates = waitTagGroups(metrics)
	}
	result.AssigneeAggregates = waitAssigneeGroups(metrics)
	result.LeadTimeComparisons, result.MissingReviewLeadTime = waitLeadTimeRows(ws.Tasks, metrics)
	return result, nil
}

func waitFirstStartBoundary(events []waitEvent) int {
	for index, event := range events {
		if event.status == "planned" || event.status == "not_started" {
			return index
		}
	}
	return -1
}

func waitReportDiagnostic(code, message string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error"}
}

func waitOptionalDate(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(waitDateLayout, value)
	return parsed, err == nil
}

func waitTaskCreations(project domain.Project, records []domain.AuditRecord, location *time.Location) map[string]waitEvent {
	result := map[string]waitEvent{}
	for _, record := range records {
		if record.Operation != "task_add" || record.TargetRef.ID == "" {
			continue
		}
		event, ok := waitAuditEvent(project, record, location)
		if ok {
			event.status = "not_started"
			if previous, exists := result[record.TargetRef.ID]; exists && !event.at.Before(previous.at) {
				continue
			}
			result[record.TargetRef.ID] = event
		}
	}
	return result
}

func waitEvents(project domain.Project, records []domain.AuditRecord, location *time.Location) ([]waitEvent, bool) {
	result := make([]waitEvent, 0, len(records))
	outOfOrder := false
	var previous time.Time
	for _, record := range records {
		event, ok := waitAuditEvent(project, record, location)
		if !ok {
			continue
		}
		if !previous.IsZero() && event.date.Before(previous) {
			outOfOrder = true
		}
		previous = event.date
		result = append(result, event)
	}
	return result, outOfOrder
}

func waitAuditEvent(project domain.Project, record domain.AuditRecord, location *time.Location) (waitEvent, bool) {
	date, ok := waitOptionalDate(firstNonEmpty(record.StatusDate, projectDate(project, record.OccurredAt)))
	if !ok {
		return waitEvent{}, false
	}
	at, precision := waitEventInstant(record.OccurredAt, date, location)
	return waitEvent{status: record.NewStatus, date: date, at: at, precision: precision, id: record.ID}, true
}

func waitEventInstant(value string, date time.Time, location *time.Location) (time.Time, string) {
	if instant, err := time.Parse(time.RFC3339, value); err == nil {
		return instant, "timestamp"
	}
	if parsed, err := time.ParseInLocation(waitDateLayout, value, location); err == nil {
		return parsed, "date"
	}
	return time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, location), "date"
}

func waitProjectLocation(project domain.Project) *time.Location {
	if location, err := time.LoadLocation(project.Timezone); err == nil {
		return location
	}
	return time.UTC
}

func waitOpenEnd(statusDate time.Time, location *time.Location, runAt time.Time) time.Time {
	end := time.Date(statusDate.Year(), statusDate.Month(), statusDate.Day(), 0, 0, 0, 0, location).AddDate(0, 0, 1).Add(-time.Second)
	if runAt.Before(end) {
		return runAt.In(location)
	}
	return end
}

func waitKind(status, next string) string {
	switch status {
	case "in_progress":
		return "in_progress"
	case "review_ready":
		return "review_wait"
	case "blocked":
		return "blocked_wait"
	case "planned", "not_started":
		if next == "in_progress" {
			return "start_wait"
		}
	}
	return "status_span"
}

func waitStartInRange(start, since time.Time, sinceOK bool, until time.Time, untilOK bool) bool {
	return (!sinceOK || !start.Before(since)) && (!untilOK || !start.After(until))
}

func waitTerminalStatus(status string) bool {
	switch status {
	case "accepted", "done", "closed", "cancelled":
		return true
	default:
		return false
	}
}

func waitRound(value float64) float64 { return math.Round(value*100) / 100 }

func waitWorkingDays(from, to time.Time, working map[time.Weekday]bool) int {
	count := 0
	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		if len(working) == 0 || working[day.Weekday()] {
			count++
		}
	}
	return count
}

func waitHasTag(tags []string, wanted string) bool {
	for _, tag := range tags {
		if tag == wanted {
			return true
		}
	}
	return false
}

func waitAggregates(metrics []waitMetricSpan) []WaitAggregate {
	byKind := map[string][]WaitSpan{}
	for _, metric := range metrics {
		if waitAggregateKind(metric.span.WaitKind) {
			byKind[metric.span.WaitKind] = append(byKind[metric.span.WaitKind], metric.span)
		}
	}
	result := []WaitAggregate{}
	for _, kind := range []string{"in_progress", "review_wait", "blocked_wait", "start_wait"} {
		if spans := byKind[kind]; len(spans) > 0 {
			result = append(result, WaitAggregate{WaitKind: kind, Stats: waitStats(spans)})
		}
	}
	return result
}

func waitStats(spans []WaitSpan) WaitStatistics {
	working := make([]int, len(spans))
	calendar := make([]int, len(spans))
	hours := make([]float64, len(spans))
	for i, span := range spans {
		working[i], calendar[i] = span.WorkingDays, span.CalendarDays
		hours[i] = span.elapsedHours
	}
	sort.Ints(working)
	sort.Ints(calendar)
	sort.Float64s(hours)
	return WaitStatistics{Count: len(spans), MedianWorkingDays: waitMedian(working), P75WorkingDays: waitP75(working), MaxWorkingDays: working[len(working)-1], MedianCalendarDays: waitMedian(calendar), P75CalendarDays: waitP75(calendar), MaxCalendarDays: calendar[len(calendar)-1], MedianHours: waitRound(waitMedianFloat(hours)), P75Hours: waitRound(waitP75Float(hours)), MaxHours: waitRound(hours[len(hours)-1])}
}

func waitMedian(values []int) float64 {
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return float64(values[middle])
	}
	return float64(values[middle-1]+values[middle]) / 2
}

func waitP75(values []int) float64 {
	return float64(values[int(math.Ceil(float64(len(values))*0.75))-1])
}

func waitMedianFloat(values []float64) float64 {
	middle := len(values) / 2
	if len(values)%2 == 1 {
		return values[middle]
	}
	return (values[middle-1] + values[middle]) / 2
}

func waitP75Float(values []float64) float64 {
	return values[int(math.Ceil(float64(len(values))*0.75))-1]
}

func waitAggregateKind(kind string) bool {
	return kind == "in_progress" || waitNamedKind(kind)
}

func waitNamedKind(kind string) bool {
	return kind == "review_wait" || kind == "blocked_wait" || kind == "start_wait"
}

func waitTagGroups(metrics []waitMetricSpan) []WaitGroup {
	groups := map[string][]waitMetricSpan{}
	for _, metric := range metrics {
		if waitAggregateKind(metric.span.WaitKind) {
			for _, tag := range metric.tags {
				groups[tag] = append(groups[tag], metric)
			}
		}
	}
	return waitGroups(groups, 5, true)
}

func waitAssigneeGroups(metrics []waitMetricSpan) []WaitGroup {
	groups := map[string][]waitMetricSpan{}
	for _, metric := range metrics {
		if waitAggregateKind(metric.span.WaitKind) && metric.assigneeID != "" {
			groups[metric.assigneeID] = append(groups[metric.assigneeID], metric)
		}
	}
	return waitGroups(groups, 1, false)
}

func waitGroups(groups map[string][]waitMetricSpan, minimum int, namedWaitThreshold bool) []WaitGroup {
	result := []WaitGroup{}
	for name, metrics := range groups {
		spanCount := len(metrics)
		if namedWaitThreshold {
			spanCount = 0
			for _, metric := range metrics {
				if waitNamedKind(metric.span.WaitKind) {
					spanCount++
				}
			}
		}
		if spanCount < minimum {
			continue
		}
		result = append(result, WaitGroup{Name: name, SpanCount: spanCount, Aggregates: waitAggregates(metrics)})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func waitLeadTimeRows(tasks []domain.Task, metrics []waitMetricSpan) ([]WaitLeadTimeComparison, []WaitMissingReviewLeadTime) {
	byTask := map[string][]waitMetricSpan{}
	for _, metric := range metrics {
		byTask[metric.taskID] = append(byTask[metric.taskID], metric)
	}
	comparisons := []WaitLeadTimeComparison{}
	missing := []WaitMissingReviewLeadTime{}
	for _, task := range tasks {
		rows := byTask[task.ID]
		if task.LeadTime != nil {
			kind := waitLeadKind(task.LeadTime.Reason)
			for _, metric := range rows {
				if metric.span.WaitKind != kind {
					continue
				}
				planned, unit, measured := task.LeadTime.WorkingDays, "working_days", metric.span.WorkingDays
				if task.LeadTime.CalendarDays != 0 {
					planned, unit, measured = task.LeadTime.CalendarDays, "calendar_days", metric.span.CalendarDays
				}
				comparisons = append(comparisons, WaitLeadTimeComparison{TaskID: task.ID, WaitKind: kind, Status: metric.span.Status, From: metric.span.From, To: metric.span.To, PlannedAmount: planned, PlannedUnit: unit, MeasuredAmount: measured, Difference: float64(measured) - planned, LeadTimeReason: task.LeadTime.Reason})
			}
			continue
		}
		var reviews []WaitSpan
		for _, metric := range rows {
			if metric.span.WaitKind == "review_wait" {
				reviews = append(reviews, metric.span)
			}
		}
		if len(reviews) > 0 {
			stats := waitStats(reviews)
			missing = append(missing, WaitMissingReviewLeadTime{TaskID: task.ID, Title: task.Title, ReviewWaitCount: len(reviews), MedianWorkingDays: stats.MedianWorkingDays, MedianCalendarDays: stats.MedianCalendarDays})
		}
	}
	sort.Slice(comparisons, func(i, j int) bool {
		if comparisons[i].TaskID != comparisons[j].TaskID {
			return comparisons[i].TaskID < comparisons[j].TaskID
		}
		return comparisons[i].From < comparisons[j].From
	})
	sort.Slice(missing, func(i, j int) bool { return missing[i].TaskID < missing[j].TaskID })
	return comparisons, missing
}

func waitLeadKind(reason string) string {
	lower := strings.ToLower(reason)
	switch {
	case strings.Contains(lower, "block"):
		return "blocked_wait"
	case strings.Contains(lower, "start"), strings.Contains(lower, "queue"):
		return "start_wait"
	default:
		return "review_wait"
	}
}
