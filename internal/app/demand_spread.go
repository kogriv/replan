package app

import (
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

// Weekly workload, the dashboard load profile and the overlay share one rule
// for where a demand's amount falls in time (GAP-031):
//
//   - a period demand (hours, agent hours, minutes, tokens) is spread evenly
//     over the working days of its window, the days its resource's
//     availability windows name, or every day for a resource without them; a
//     week counts only its own share;
//   - a concurrent demand (a slot) occupies its amount on every day of its
//     window, so a week or a span counts the peak of simultaneous use, never
//     a sum.

// demandSpan is a demand's window, from its own timing or its task's planned
// dates. An undated demand counts everywhere.
type demandSpan struct {
	start, finish time.Time
	dated         bool
}

func demandBounds(demand domain.ResourceDemand, task domain.Task) (string, string) {
	return firstNonEmpty(demand.Timing.PreferredStart, demand.Timing.EarliestStart, task.PlannedStart),
		firstNonEmpty(demand.Timing.PreferredFinish, demand.Timing.LatestFinish, task.PlannedFinish)
}

func demandWindow(demand domain.ResourceDemand, taskByID map[string]domain.Task) demandSpan {
	startText, finishText := demandBounds(demand, taskByID[demand.TaskRef.ID])
	start, startOK := parseWorkloadDate(startText)
	finish, finishOK := parseWorkloadDate(finishText)
	switch {
	case !startOK && !finishOK:
		return demandSpan{}
	case !startOK:
		start = finish
	case !finishOK:
		finish = start
	}
	return demandSpan{start: start, finish: finish, dated: true}
}

func (s demandSpan) intersects(from, to time.Time) bool {
	return !s.dated || (!s.finish.Before(from) && !s.start.After(to))
}

// resourceAvailability is the per-resource calendar rule shared by capacity
// and demand spreading. An empty weekdays map means every weekday. Removed
// dates are whole-day exceptions; exception types without an amount stay in
// the model but do not alter the calendar.
type resourceAvailability struct {
	weekdays     map[time.Weekday]bool
	removedDates map[string]bool
}

func (a resourceAvailability) regularDay(day time.Time) bool {
	return len(a.weekdays) == 0 || a.weekdays[day.Weekday()]
}

func (a resourceAvailability) availableDay(day time.Time) bool {
	return a.regularDay(day) && !a.removedDates[day.Format("2006-01-02")]
}

func (a resourceAvailability) regularDaysPerWeek() int {
	if len(a.weekdays) == 0 {
		return 7
	}
	return len(a.weekdays)
}

func removesAvailabilityDay(exceptionType string) bool {
	switch strings.ToLower(strings.TrimSpace(exceptionType)) {
	case "unavailable", "access_closed", "maintenance", "manual_hold":
		return true
	default:
		return false
	}
}

var availabilityWeekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
}

// workingDays maps each resource to its regular weekdays and dated removals.
// A resource without windows works every day.
func workingDays(availabilities []domain.Availability) map[string]resourceAvailability {
	out := map[string]resourceAvailability{}
	for _, availability := range availabilities {
		resourceID := availability.ResourceRef.ID
		rule := out[resourceID]
		for _, window := range availability.Windows {
			for _, day := range window.Days {
				weekday, ok := availabilityWeekdays[strings.ToLower(strings.TrimSpace(day))]
				if !ok {
					continue
				}
				if rule.weekdays == nil {
					rule.weekdays = map[time.Weekday]bool{}
				}
				rule.weekdays[weekday] = true
			}
		}
		for _, exception := range availability.Exceptions {
			if !removesAvailabilityDay(exception.Type) || strings.TrimSpace(exception.Date) == "" {
				continue
			}
			if rule.removedDates == nil {
				rule.removedDates = map[string]bool{}
			}
			rule.removedDates[exception.Date] = true
		}
		out[resourceID] = rule
	}
	return out
}

// spreadDays lists the available days of a dated span that carry its amount,
// or every day when the resource has no regular working days in the span, so
// that a weekend-only window does not lose its demand.
func spreadDays(span demandSpan, availability resourceAvailability) []time.Time {
	days, _ := spreadDaysWithFallback(span, availability)
	return days
}

// spreadDaysWithFallback also reports when regular working days existed but
// every one was removed by an exception. In that case it retains the regular
// days so the demand remains visible instead of disappearing.
func spreadDaysWithFallback(span demandSpan, availability resourceAvailability) ([]time.Time, bool) {
	var all, regular, available []time.Time
	for day := span.start; !day.After(span.finish); day = day.AddDate(0, 0, 1) {
		all = append(all, day)
		if availability.regularDay(day) {
			regular = append(regular, day)
			if availability.availableDay(day) {
				available = append(available, day)
			}
		}
	}
	if len(available) > 0 {
		return available, false
	}
	if len(regular) > 0 && len(availability.removedDates) > 0 {
		return regular, true
	}
	return all, false
}

// spanShare is the part of a period demand's amount that falls between from
// and to, inclusive.
func spanShare(span demandSpan, availability resourceAvailability, from, to time.Time) float64 {
	if !span.dated {
		return 1
	}
	spread := spreadDays(span, availability)
	inside := 0
	for _, day := range spread {
		if !day.Before(from) && !day.After(to) {
			inside++
		}
	}
	return float64(inside) / float64(len(spread))
}

// isConcurrentResourceUnit tells whether a resource's capacity in a unit is a
// number of simultaneous slots rather than an amount per period.
func isConcurrentResourceUnit(capacities []domain.Capacity, resourceID, unit string) bool {
	for _, capacity := range capacities {
		if capacity.ResourceRef.ID == resourceID && capacity.Unit == unit && capacity.Period == "concurrent" {
			return true
		}
	}
	return unit == "concurrent_slots"
}

// concurrentUse is one concurrent demand's occupation of a resource.
type concurrentUse struct {
	span                         demandSpan
	planned, remaining, reserved float64
	days                         map[string]bool
}

// peakConcurrentUse returns the highest simultaneous planned, remaining and
// reserved use on any day between from and to; an undated use counts on every
// day. Without bounds it scans the dated uses' own span.
func peakConcurrentUse(uses []concurrentUse, from, to time.Time, bounded bool) (planned, remaining, reserved float64) {
	var base concurrentUse
	var dated []concurrentUse
	for _, use := range uses {
		if !use.span.dated {
			base.planned += use.planned
			base.remaining += use.remaining
			base.reserved += use.reserved
			continue
		}
		dated = append(dated, use)
	}
	planned, remaining, reserved = base.planned, base.remaining, base.reserved
	if !bounded {
		for i, use := range dated {
			if i == 0 || use.span.start.Before(from) {
				from = use.span.start
			}
			if i == 0 || use.span.finish.After(to) {
				to = use.span.finish
			}
		}
	}
	for day := from; !day.After(to) && len(dated) > 0; day = day.AddDate(0, 0, 1) {
		p, r, s := base.planned, base.remaining, base.reserved
		for _, use := range dated {
			if !day.Before(use.span.start) && !day.After(use.span.finish) && (use.days == nil || use.days[day.Format("2006-01-02")]) {
				p += use.planned
				r += use.remaining
				s += use.reserved
			}
		}
		planned, remaining, reserved = max(planned, p), max(remaining, r), max(reserved, s)
	}
	return planned, remaining, reserved
}
