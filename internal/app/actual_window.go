package app

import (
	"sort"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

type taskActualWindow struct {
	Start, Finish             string
	StartSource, FinishSource string
	ConsumptionDates          []string
}

// deriveTaskActualWindow shares the report and mutation rule. demandIDs is
// explicit because estimate reports include baseline demands while the task
// command deliberately supplies only the task's current demands.
func deriveTaskActualWindow(ws domain.Workspace, taskID string, demandIDs []string, finishFallback bool) taskActualWindow {
	window := taskActualWindow{StartSource: "none", FinishSource: "none"}
	var starts, finishes []string
	for _, audit := range ws.AuditRecords {
		if audit.Operation != "actual_add_status" || audit.TargetRef.ID != taskID || audit.NewStatus == "" {
			continue
		}
		date := firstNonEmpty(audit.StatusDate, datePart(audit.OccurredAt))
		if date == "" {
			continue
		}
		if audit.NewStatus == "in_progress" {
			starts = append(starts, date)
		}
		if estimateTaskFinished(audit.NewStatus) {
			finishes = append(finishes, date)
		}
	}
	if len(starts) > 0 {
		sort.Strings(starts)
		window.Start, window.StartSource = starts[0], "audit"
	}
	if len(finishes) > 0 {
		sort.Strings(finishes)
		window.Finish, window.FinishSource = finishes[0], "audit"
	}

	demands := make(map[string]bool, len(demandIDs))
	for _, id := range demandIDs {
		demands[id] = true
	}
	seen := map[string]bool{}
	for _, actual := range ws.Actuals {
		if !estimateConsumptionTypes[actual.Type] || (actual.TaskRef.ID != taskID && !demands[actual.DemandRef.ID]) {
			continue
		}
		if date := firstNonEmpty(actual.StatusDate, datePart(actual.OccurredAt)); date != "" {
			seen[date] = true
		}
	}
	for date := range seen {
		window.ConsumptionDates = append(window.ConsumptionDates, date)
	}
	sort.Strings(window.ConsumptionDates)
	if window.Start == "" && len(window.ConsumptionDates) > 0 {
		window.Start, window.StartSource = window.ConsumptionDates[0], "consumption_actual"
	}
	if window.Finish == "" && len(window.ConsumptionDates) > 0 {
		window.Finish, window.FinishSource = window.ConsumptionDates[len(window.ConsumptionDates)-1], "consumption_actual"
	}
	if finishFallback && window.Start == "" && window.Finish != "" {
		window.Start, window.StartSource = window.Finish, window.FinishSource
	}
	return window
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
