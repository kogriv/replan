package app_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
)

// spreadWorkspace has a person working Monday to Friday at 5 hours a week
// until Sunday 2026-10-04 and 10 hours from Monday 2026-10-05, one
// development slot, and these tasks:
//
//	task.long   Mon 2026-11-09 .. Fri 2026-11-20, 8 hours: two working weeks
//	task.a      Mon 2026-09-28 .. Tue 2026-09-29, slot
//	task.b      Wed 2026-09-30 .. Thu 2026-10-01, slot, after task.a
//	task.c      Thu 2026-10-01 .. Fri 2026-10-02, slot, overlaps task.b
//	task.wkend  Sat 2026-10-10 .. Mon 2026-10-12, 3 hours, one working day
func spreadWorkspace(withOverlap bool) domain.Workspace {
	person := domain.Ref{Kind: "resource", ID: "resource.human.person"}
	slot := domain.Ref{Kind: "resource", ID: "resource.environment.slot"}
	task := func(id, start, finish string) domain.Task {
		return domain.Task{ID: id, Title: id, TrackingStatus: "not_started", PlannedStart: start, PlannedFinish: finish}
	}
	demand := func(id, taskID string, resource domain.Ref, unit string, amount float64) domain.ResourceDemand {
		return domain.ResourceDemand{ID: id, TaskRef: domain.Ref{Kind: "task", ID: taskID}, ResourceRef: resource, Unit: unit,
			PlannedAmount: amount, RemainingAmount: &amount, Status: "active", Criticality: "required"}
	}
	ws := domain.Workspace{
		Project:   domain.Project{ID: "project.spread", Name: "Spread", Timezone: "UTC", DefaultStatusDate: "2026-09-28"},
		Resources: []domain.Resource{{ID: person.ID, Class: "human", Name: "Person"}, {ID: slot.ID, Class: "environment", Name: "Slot"}},
		Capacities: []domain.Capacity{
			{ID: "capacity.person.early", ResourceRef: person, Type: "periodic_capacity", Unit: "hours", Amount: 5, Period: "week", EffectiveTo: "2026-10-04"},
			{ID: "capacity.person.late", ResourceRef: person, Type: "periodic_capacity", Unit: "hours", Amount: 10, Period: "week", EffectiveFrom: "2026-10-05"},
			{ID: "capacity.slot", ResourceRef: slot, Type: "concurrent_capacity", Unit: "concurrent_slots", Amount: 1, Period: "concurrent"},
		},
		Availabilities: []domain.Availability{{ID: "availability.person", ResourceRef: person,
			Windows: []domain.AvailabilityWindow{{Days: []string{"monday", "tuesday", "wednesday", "thursday", "friday"}}}}},
		Tasks: []domain.Task{
			task("task.long", "2026-11-09", "2026-11-20"), task("task.a", "2026-09-28", "2026-09-29"),
			task("task.b", "2026-09-30", "2026-10-01"), task("task.wkend", "2026-10-10", "2026-10-12"),
		},
		ResourceDemands: []domain.ResourceDemand{
			demand("resource_demand.long", "task.long", person, "hours", 8),
			demand("resource_demand.a", "task.a", slot, "concurrent_slots", 1),
			demand("resource_demand.b", "task.b", slot, "concurrent_slots", 1),
			demand("resource_demand.wkend", "task.wkend", person, "hours", 3),
		},
	}
	if withOverlap {
		ws.Tasks = append(ws.Tasks, task("task.c", "2026-10-01", "2026-10-02"))
		ws.ResourceDemands = append(ws.ResourceDemands, demand("resource_demand.c", "task.c", slot, "concurrent_slots", 1))
	}
	for i := range ws.Tasks {
		for _, d := range ws.ResourceDemands {
			if d.TaskRef.ID == ws.Tasks[i].ID {
				ws.Tasks[i].ResourceDemands = append(ws.Tasks[i].ResourceDemands, domain.Ref{Kind: "resource_demand", ID: d.ID})
			}
		}
	}
	return ws
}

func workloadRow(t *testing.T, ws domain.Workspace, week, resourceID string) app.ResourceWorkload {
	t.Helper()
	result, err := app.NewWorkloadService(stubLoader{ws: ws}).Calculate(context.Background(), app.WorkloadRequest{Week: week})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	for _, row := range result.Resources {
		if row.ResourceID == resourceID {
			return row
		}
	}
	return app.ResourceWorkload{}
}

func TestWorkloadSpreadsMultiWeekDemandOverWorkingDays(t *testing.T) {
	ws := spreadWorkspace(false)
	for _, week := range []string{"2026-W46", "2026-W47"} {
		row := workloadRow(t, ws, week, "resource.human.person")
		if row.Planned != 4 || row.Remaining != 4 || row.Overload != 0 {
			t.Errorf("%s: planned %v remaining %v overload %v, want 4, 4, 0", week, row.Planned, row.Remaining, row.Overload)
		}
	}
	// Saturday to Monday holds one working day: all 3 hours fall on Monday,
	// in 2026-W42, none in the weekend's 2026-W41.
	if row := workloadRow(t, ws, "2026-W41", "resource.human.person"); row.Planned != 0 {
		t.Errorf("weekend week: planned %v, want 0", row.Planned)
	}
	if row := workloadRow(t, ws, "2026-W42", "resource.human.person"); row.Planned != 3 {
		t.Errorf("Monday week: planned %v, want 3", row.Planned)
	}
	// The all-weeks view still sums everything.
	if row := workloadRow(t, ws, "", "resource.human.person"); row.Planned != 11 {
		t.Errorf("all weeks: planned %v, want 11", row.Planned)
	}
}

func TestWorkloadChecksSlotsByOverlappingDays(t *testing.T) {
	sequential := workloadRow(t, spreadWorkspace(false), "2026-W40", "resource.environment.slot")
	if sequential.Remaining != 1 || sequential.Overload != 0 {
		t.Errorf("tasks one after another: remaining %v overload %v, want 1 and 0", sequential.Remaining, sequential.Overload)
	}
	overlapping := workloadRow(t, spreadWorkspace(true), "2026-W40", "resource.environment.slot")
	if overlapping.Remaining != 2 || overlapping.Overload != 1 {
		t.Errorf("tasks sharing 2026-10-01: remaining %v overload %v, want 2 and 1", overlapping.Remaining, overlapping.Overload)
	}
	if all := workloadRow(t, spreadWorkspace(true), "", "resource.environment.slot"); all.Remaining != 2 {
		t.Errorf("all weeks: peak %v, want 2", all.Remaining)
	}
}

func TestDashboardLoadUsesWorkingDaysPeaksAndWeeklyCapacity(t *testing.T) {
	overview, err := app.NewReportService(stubLoader{ws: spreadWorkspace(true)}).Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: "spread", GeneratedAt: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	loads := map[string]reports.DashboardGanttResourceLoad{}
	for _, load := range overview.Gantt.ResourceLoads {
		loads[load.ResourceID] = load
	}
	buckets := func(id string) map[string]reports.DashboardGanttResourceLoadBucket {
		out := map[string]reports.DashboardGanttResourceLoadBucket{}
		for _, bucket := range loads[id].Buckets {
			out[bucket.Label] = bucket
		}
		return out
	}

	person := buckets("resource.human.person")
	for label, want := range map[string]struct{ planned, capacity float64 }{
		"2026-W40": {0, 5}, "2026-W41": {0, 10}, "2026-W42": {3, 10}, "2026-W46": {4, 10}, "2026-W47": {4, 10},
	} {
		got := person[label]
		if math.Abs(got.Planned-want.planned) > 1e-4 || math.Abs(got.Capacity-want.capacity) > 1e-4 {
			t.Errorf("person %s: planned %v capacity %v, want %v and %v", label, got.Planned, got.Capacity, want.planned, want.capacity)
		}
	}
	start, _ := time.Parse("2006-01-02", overview.Gantt.StartDate)
	for i, amount := range loads["resource.human.person"].DailyAmounts {
		if weekday := start.AddDate(0, 0, i).Weekday(); amount != 0 && (weekday == time.Saturday || weekday == time.Sunday) {
			t.Errorf("person has %v hours on %s", amount, start.AddDate(0, 0, i).Format("2006-01-02 Mon"))
		}
	}

	slot := loads["resource.environment.slot"]
	if !slot.Concurrent {
		t.Fatalf("slot load not marked concurrent")
	}
	if got := buckets("resource.environment.slot")["2026-W40"]; got.Planned != 2 || got.Capacity != 1 || got.Overload != 1 {
		t.Errorf("slot 2026-W40: planned %v capacity %v overload %v, want peak 2, 1, 1", got.Planned, got.Capacity, got.Overload)
	}
}

// A row lists the demands it is made of: prorated ones with their part of the
// week, concurrent ones with their own amounts beside the row's daily peak.
func TestWorkloadListsTheDemandsOfEachRow(t *testing.T) {
	ws := spreadWorkspace(false)
	rows := func(week string, with bool) map[string]app.ResourceWorkload {
		result, err := app.NewWorkloadService(stubLoader{ws: ws}).Calculate(context.Background(), app.WorkloadRequest{Week: week, WithDemands: with})
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]app.ResourceWorkload{}
		for _, row := range result.Resources {
			out[row.ResourceID] = row
		}
		return out
	}

	slot := rows("2026-W40", true)["resource.environment.slot"]
	if slot.Remaining != 1 || slot.Overload != 0 || len(slot.Demands) != 2 {
		t.Fatalf("slot row: remaining %v overload %v demands %+v", slot.Remaining, slot.Overload, slot.Demands)
	}
	for i, id := range []string{"resource_demand.a", "resource_demand.b"} {
		d := slot.Demands[i]
		if d.ResourceDemandID != id || !d.Concurrent || d.Remaining != 1 || d.Planned != 1 || d.Status != "active" {
			t.Fatalf("slot demand %d: %+v", i, d)
		}
	}

	person := rows("2026-W46", true)["resource.human.person"]
	if len(person.Demands) != 1 {
		t.Fatalf("person demands: %+v", person.Demands)
	}
	if d := person.Demands[0]; d.ResourceDemandID != "resource_demand.long" || d.TaskID != "task.long" || d.Concurrent || d.Share != 0.5 || d.Remaining != 4 || d.Remaining != person.Remaining {
		t.Fatalf("person demand: %+v, row remaining %v", d, person.Remaining)
	}

	if plain := rows("2026-W40", false)["resource.environment.slot"]; plain.Demands != nil {
		t.Fatalf("demands listed without being asked: %+v", plain.Demands)
	}
}
