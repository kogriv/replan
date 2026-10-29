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

func demandWindow(demand domain.ResourceDemand, taskByID map[string]domain.Task) demandSpan {
	startText := firstNonEmpty(strings.TrimSpace(demand.Timing.PreferredStart), strings.TrimSpace(demand.Timing.EarliestStart))
	finishText := firstNonEmpty(strings.TrimSpace(demand.Timing.PreferredFinish), strings.TrimSpace(demand.Timing.LatestFinish))
	if task, ok := taskByID[demand.TaskRef.ID]; ok {
		startText = firstNonEmpty(startText, strings.TrimSpace(task.PlannedStart))
		finishText = firstNonEmpty(finishText, strings.TrimSpace(task.PlannedFinish))
	}
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
	if finish.Before(start) {
		start, finish = finish, start
	}
	return demandSpan{start: start, finish: finish, dated: true}
}

func (s demandSpan) intersects(from, to time.Time) bool {
	return !s.dated || (!s.finish.Before(from) && !s.start.After(to))
}

// workingDays maps each resource with availability windows to the weekdays
// those windows name. A resource that is absent works every day.
func workingDays(availabilities []domain.Availability) map[string]map[time.Weekday]bool {
	names := map[string]time.Weekday{
		"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
		"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
	}
	out := map[string]map[time.Weekday]bool{}
	for _, availability := range availabilities {
		for _, window := range availability.Windows {
			for _, day := range window.Days {
				weekday, ok := names[strings.ToLower(strings.TrimSpace(day))]
				if !ok {
					continue
				}
				if out[availability.ResourceRef.ID] == nil {
					out[availability.ResourceRef.ID] = map[time.Weekday]bool{}
				}
				out[availability.ResourceRef.ID][weekday] = true
			}
		}
	}
	return out
}

// spreadDays lists the days of a dated span that carry its amount: the
// working days, or every day when the resource has no working days in the
// span, so that a weekend-only window does not lose its demand.
func spreadDays(span demandSpan, days map[time.Weekday]bool) []time.Time {
	var all, working []time.Time
	for day := span.start; !day.After(span.finish); day = day.AddDate(0, 0, 1) {
		all = append(all, day)
		if len(days) == 0 || days[day.Weekday()] {
			working = append(working, day)
		}
	}
	if len(working) == 0 {
		return all
	}
	return working
}

// spanShare is the part of a period demand's amount that falls between from
// and to, inclusive.
func spanShare(span demandSpan, days map[time.Weekday]bool, from, to time.Time) float64 {
	if !span.dated {
		return 1
	}
	spread := spreadDays(span, days)
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
			if !day.Before(use.span.start) && !day.After(use.span.finish) {
				p += use.planned
				r += use.remaining
				s += use.reserved
			}
		}
		planned, remaining, reserved = max(planned, p), max(remaining, r), max(reserved, s)
	}
	return planned, remaining, reserved
}
