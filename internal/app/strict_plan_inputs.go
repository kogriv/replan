package app

import (
	"fmt"
	"strings"
	"time"
	_ "time/tzdata" // Standalone binaries also need IANA zones without host tzdata.
	"unicode"

	"github.com/kogriv/replan/internal/domain"
)

// ErrInvalidPlanInputs distinguishes an unsafe calculation from a report that
// can still be written as a diagnostic artifact for a missing project file.
var ErrInvalidPlanInputs = fmt.Errorf("invalid planning inputs: %w", ErrValidationFailed)

// Check effective records, including records changed by a scenario, before
// interpreting a calendar or an amount. The full validator also checks refs.
func validatePlanInputs(result *ValidateResult, ws domain.Workspace) {
	v := workspaceValidator{result: result}
	if zone := ws.Project.Timezone; zone != "" && !validPlanTimezone(zone) {
		v.errorf("invalid_project_timezone", "project", ws.Project.ID, "project timezone %q must be an explicit IANA timezone", zone)
	}
	if date := ws.Project.DefaultStatusDate; date != "" && !validDate(date) {
		v.errorf("invalid_project_status_date", "project", ws.Project.ID, "default_status_date %q must use YYYY-MM-DD", date)
	}
	tasks := make(map[string]domain.Task, len(ws.Tasks))
	for _, task := range ws.Tasks {
		tasks[task.ID] = task
		v.planTaskDates(task)
	}
	for _, demand := range ws.ResourceDemands {
		v.planDemand(demand, tasks[demand.TaskRef.ID])
	}
	for _, capacity := range ws.Capacities {
		v.planCapacity(capacity)
	}
	for _, availability := range ws.Availabilities {
		v.planAvailability(availability)
	}
	for _, actual := range ws.Actuals {
		v.planActual(actual)
	}
}

func planInputErrors(ws domain.Workspace) []Diagnostic {
	var result ValidateResult
	validatePlanInputs(&result, ws)
	return result.Errors
}

func validPlanTimezone(zone string) bool {
	if zone == "Local" || zone == "" {
		return false
	}
	_, err := time.LoadLocation(zone)
	return err == nil
}

func invalidPlanID(id string) bool {
	return strings.ContainsFunc(id, func(r rune) bool {
		return unicode.IsControl(r) || r == '\u2028' || r == '\u2029'
	})
}

func safeBaselineID(id string) bool {
	if id == "" || id[0] == '.' || strings.HasSuffix(id, ".") {
		return false
	}
	for _, c := range id {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '.':
		default:
			return false
		}
	}
	stem, _, _ := strings.Cut(strings.ToUpper(id), ".")
	switch stem {
	case "CON", "PRN", "AUX", "NUL":
		return false
	}
	return len(stem) != 4 || stem[3] < '0' || stem[3] > '9' || (!strings.HasPrefix(stem, "COM") && !strings.HasPrefix(stem, "LPT"))
}

func (v *workspaceValidator) planTaskDates(task domain.Task) {
	start, finish := task.PlannedStart, task.PlannedFinish
	startOK, finishOK := validDate(start), validDate(finish)
	if start != "" && !startOK {
		v.errorf("invalid_task_planned_start", "task", task.ID, "task planned_start %q must use YYYY-MM-DD", start)
	}
	if finish != "" && !finishOK {
		v.errorf("invalid_task_planned_finish", "task", task.ID, "task planned_finish %q must use YYYY-MM-DD", finish)
	}
	if startOK && finishOK && finish < start {
		v.errorf("invalid_task_planned_range", "task", task.ID, "task planned_finish %s is before planned_start %s", finish, start)
	}
}

func (v *workspaceValidator) planDemand(demand domain.ResourceDemand, task domain.Task) {
	const kind = "resource_demand"
	if !validAmount(demand.PlannedAmount) {
		v.errorf("invalid_planned_amount", kind, demand.ID, "resource demand planned_amount %v must be a finite number, zero or greater", demand.PlannedAmount)
	}
	if demand.RemainingAmount != nil && !validAmount(*demand.RemainingAmount) {
		v.errorf("invalid_remaining_amount", kind, demand.ID, "resource demand remaining_amount %v must be a finite number, zero or greater", *demand.RemainingAmount)
	}
	if !validAmount(demand.ActualAmount) {
		v.errorf("invalid_demand_actual_amount", kind, demand.ID, "resource demand actual_amount %v must be a finite number, zero or greater", demand.ActualAmount)
	}
	if !validAmount(demand.ReservedAmount) {
		v.errorf("invalid_reserved_amount", kind, demand.ID, "resource demand reserved_amount %v must be a finite number, zero or greater", demand.ReservedAmount)
	}
	t := demand.Timing
	for _, field := range []struct{ field, value, code string }{
		{"earliest_start", t.EarliestStart, "invalid_demand_earliest_start"},
		{"latest_finish", t.LatestFinish, "invalid_demand_latest_finish"},
		{"preferred_start", t.PreferredStart, "invalid_demand_preferred_start"},
		{"preferred_finish", t.PreferredFinish, "invalid_demand_preferred_finish"},
	} {
		if field.value != "" && !validDate(field.value) {
			v.errorf(field.code, kind, demand.ID, "resource demand %s %q must use YYYY-MM-DD", field.field, field.value)
		}
	}
	start, finish := demandBounds(demand, task)
	switch {
	case reversedDates(t.EarliestStart, t.LatestFinish):
		v.errorf("invalid_demand_timing_range", kind, demand.ID, "resource demand latest_finish %s is before earliest_start %s", t.LatestFinish, t.EarliestStart)
	case reversedDates(start, finish) && !reversedDates(task.PlannedStart, task.PlannedFinish):
		v.errorf("invalid_demand_window", kind, demand.ID, "resolved demand finish %s is before start %s", finish, start)
	}
	switch t.Spread {
	case "", "even":
	case "contiguous", "manual":
		v.errorf("unsupported_demand_spread", kind, demand.ID, "demand spread %q cannot be calculated; only even spreading is implemented", t.Spread)
	default:
		v.errorf("invalid_demand_spread", kind, demand.ID, "unknown demand spread %q", t.Spread)
	}
	v.planRecordTimezone(kind, demand.ID, t.Timezone)
}

func reversedDates(start, finish string) bool {
	return finish < start && validDate(start) && validDate(finish)
}

func (v *workspaceValidator) planCapacity(capacity domain.Capacity) {
	if !validAmount(capacity.Amount) {
		v.errorf("invalid_capacity_amount", "capacity", capacity.ID, "capacity amount %v must be a finite number, zero or greater", capacity.Amount)
	}
	switch capacity.Period {
	case "", "week", "month", "concurrent", "instant":
	default:
		v.errorf("unsupported_capacity_period", "capacity", capacity.ID, "capacity period %q is not implemented", capacity.Period)
	}
	v.planRecordTimezone("capacity", capacity.ID, capacity.Timezone)
	validateCapacityEffectiveDates(v.result, capacity)
}

func (v *workspaceValidator) planAvailability(a domain.Availability) {
	const kind = "availability"
	v.planRecordTimezone(kind, a.ID, a.Timezone)
	for i, w := range a.Windows {
		for _, day := range w.Days {
			if _, ok := availabilityWeekdays[strings.ToLower(strings.TrimSpace(day))]; !ok {
				v.errorf("invalid_availability_weekday", kind, a.ID, "availability window %d has unknown weekday %q", i+1, day)
			}
		}
		if w.StartTime == "" && w.EndTime == "" {
			continue
		}
		start, startOK := availabilityClockMinutes(w.StartTime)
		end, endOK := availabilityClockMinutes(w.EndTime)
		if !startOK || !endOK || start >= end {
			v.errorf("invalid_availability_window_time", kind, a.ID, "availability window %d needs increasing HH:MM bounds within one day, got %q to %q", i+1, w.StartTime, w.EndTime)
		}
	}
	for i, e := range a.Exceptions {
		if !validDate(e.Date) {
			v.errorf("invalid_availability_exception_date", kind, a.ID, "availability exception %d date %q must use YYYY-MM-DD", i+1, e.Date)
		}
		if !removesAvailabilityDay(e.Type) {
			switch strings.ToLower(strings.TrimSpace(e.Type)) {
			case "reduced_capacity", "extra_capacity", "quota_reset":
			default:
				v.errorf("invalid_availability_exception_type", kind, a.ID, "availability exception %d has unknown type %q", i+1, e.Type)
			}
		}
	}
}

func availabilityClockMinutes(value string) (int, bool) {
	if value == "24:00" {
		return 24 * 60, true
	}
	clock, err := time.Parse("15:04", value)
	if err != nil || clock.Format("15:04") != value {
		return 0, false
	}
	return clock.Hour()*60 + clock.Minute(), true
}

func (v *workspaceValidator) planRecordTimezone(kind, id, zone string) {
	if zone != "" && !validPlanTimezone(zone) {
		v.errorf("invalid_plan_record_timezone", kind, id, "%s timezone %q must be an explicit IANA timezone", kind, zone)
	}
}

func (v *workspaceValidator) planActual(actual domain.Actual) {
	if date := actual.StatusDate; date != "" && !validDate(date) {
		v.errorf("invalid_actual_status_date", "actual", actual.ID, "actual status_date %q must use YYYY-MM-DD", date)
	}
	if stamp := actual.OccurredAt; stamp != "" && !validDate(stamp) {
		if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			v.errorf("invalid_actual_occurred_at", "actual", actual.ID, "actual occurred_at %q must use RFC3339 or YYYY-MM-DD", stamp)
		}
	}
	if !validAmount(actual.Amount) {
		v.errorf("invalid_actual_amount", "actual", actual.ID, "actual amount %v must be a finite number, zero or greater", actual.Amount)
	}
	if actual.Type == "correction" {
		if actual.CorrectedAmount != nil && !validAmount(*actual.CorrectedAmount) {
			v.errorf("invalid_amount", "actual", actual.ID, "correction corrected_amount must be finite, zero or greater")
		}
		if actual.CorrectedStatusDate != "" && !validDate(actual.CorrectedStatusDate) {
			v.errorf("invalid_status_date", "actual", actual.ID, "correction corrected_status_date %q must use YYYY-MM-DD", actual.CorrectedStatusDate)
		}
	}
}
