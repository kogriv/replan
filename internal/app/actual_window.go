package app

import (
	"sort"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

type taskActualWindow struct {
	Start, Finish             string
	StartSource, FinishSource string
	StartOccurredAt           string
	ConsumptionDates          []string
}

type actualWindowRecord struct {
	date       string
	occurredAt string
}

// deriveTaskActualWindow shares the report and mutation rule. demandIDs is
// explicit because estimate reports include baseline demands while the task
// command deliberately supplies only the task's current demands.
func deriveTaskActualWindow(ws domain.Workspace, taskID string, demandIDs []string, finishFallback bool) taskActualWindow {
	window := taskActualWindow{StartSource: "none", FinishSource: "none"}
	var starts []actualWindowRecord
	var finishes []string
	for _, audit := range ws.AuditRecords {
		if audit.Operation != "actual_add_status" || audit.TargetRef.ID != taskID || audit.NewStatus == "" {
			continue
		}
		date := firstNonEmpty(audit.StatusDate, datePart(audit.OccurredAt))
		if date == "" {
			continue
		}
		if audit.NewStatus == "in_progress" {
			starts = append(starts, actualWindowRecord{date: date, occurredAt: audit.OccurredAt})
		}
		if estimateTaskFinished(audit.NewStatus) {
			finishes = append(finishes, date)
		}
	}
	if len(starts) > 0 {
		sortActualWindowRecords(starts)
		window.Start, window.StartOccurredAt, window.StartSource = starts[0].date, validRFC3339(starts[0].occurredAt), "audit"
	}
	if len(finishes) > 0 {
		sort.Strings(finishes)
		window.Finish, window.FinishSource = finishes[0], "audit"
	}

	demands := make(map[string]bool, len(demandIDs))
	for _, id := range demandIDs {
		demands[id] = true
	}
	consumptionRecords := map[string]actualWindowRecord{}
	for _, actual := range EffectiveActuals(ws.Actuals) {
		if !estimateConsumptionTypes[actual.Type] || (actual.TaskRef.ID != taskID && !demands[actual.DemandRef.ID]) {
			continue
		}
		if date := firstNonEmpty(actual.StatusDate, datePart(actual.OccurredAt)); date != "" {
			record := actualWindowRecord{date: date, occurredAt: actual.OccurredAt}
			if existing, ok := consumptionRecords[date]; !ok || actualWindowRecordLess(record, existing) {
				consumptionRecords[date] = record
			}
		}
	}
	for date := range consumptionRecords {
		window.ConsumptionDates = append(window.ConsumptionDates, date)
	}
	sort.Strings(window.ConsumptionDates)
	if window.Start == "" && len(window.ConsumptionDates) > 0 {
		window.Start, window.StartSource = window.ConsumptionDates[0], "consumption_actual"
		window.StartOccurredAt = validRFC3339(consumptionRecords[window.Start].occurredAt)
	}
	if window.Finish == "" && len(window.ConsumptionDates) > 0 {
		window.Finish, window.FinishSource = window.ConsumptionDates[len(window.ConsumptionDates)-1], "consumption_actual"
	}
	if finishFallback && window.Start == "" && window.Finish != "" {
		window.Start, window.StartSource = window.Finish, window.FinishSource
	}
	return window
}

func sortActualWindowRecords(records []actualWindowRecord) {
	sort.Slice(records, func(i, j int) bool { return actualWindowRecordLess(records[i], records[j]) })
}

func actualWindowRecordLess(left, right actualWindowRecord) bool {
	if left.date != right.date {
		return left.date < right.date
	}
	leftTime, leftErr := time.Parse(time.RFC3339, left.occurredAt)
	rightTime, rightErr := time.Parse(time.RFC3339, right.occurredAt)
	if leftErr == nil && rightErr == nil {
		return leftTime.Before(rightTime)
	}
	return leftErr == nil && rightErr != nil
}

func validRFC3339(value string) string {
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return value
	}
	return ""
}

func projectWorkingDays(availabilities []domain.Availability) map[time.Weekday]bool {
	days := map[time.Weekday]bool{}
	for _, resourceDays := range workingDays(availabilities) {
		for day := range resourceDays {
			days[day] = true
		}
	}
	return days
}
