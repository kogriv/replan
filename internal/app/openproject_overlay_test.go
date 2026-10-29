package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
)

// overlayWorkspace is spreadWorkspace on 2026-09-30, linked to OpenProject,
// with the facts a task page shows: task.a is done and booked, task.b waits on it through a
// dependency still recorded as open, and a proposal is open on task.b's
// demand.
func overlayWorkspace() domain.Workspace {
	ws := spreadWorkspace(true)
	ws.Project.DefaultStatusDate = "2026-09-30"
	for i := range ws.Tasks {
		ws.Tasks[i].SourceRefs = []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "wp-" + ws.Tasks[i].ID}}
		switch ws.Tasks[i].ID {
		case "task.a":
			ws.Tasks[i].TrackingStatus = "accepted"
		case "task.b":
			ws.Tasks[i].TrackingStatus = "review_ready"
		}
	}
	// task.done plans 12 hours in a 10-hour week, all of them already done:
	// planned load above capacity, nothing remaining.
	done, zero := 12.0, 0.0
	ws.Tasks = append(ws.Tasks, domain.Task{ID: "task.done", Title: "task.done", TrackingStatus: "in_progress", PlannedStart: "2026-10-05", PlannedFinish: "2026-10-09",
		ResourceDemands: []domain.Ref{{Kind: "resource_demand", ID: "resource_demand.done"}},
		SourceRefs:      []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "wp-task.done"}}})
	ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: "resource_demand.done", TaskRef: domain.Ref{Kind: "task", ID: "task.done"},
		ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.person"}, Unit: "hours", PlannedAmount: done, RemainingAmount: &zero,
		Status: "active", Criticality: "required"})
	ws.Dependencies = []domain.Dependency{{
		ID: "dependency.b-after-a", Type: "finish_to_start", Status: "open", Criticality: "required",
		PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
	}}
	ws.Evidence = []domain.Evidence{
		{ID: "evidence.a.second", Type: "decision", Subtype: "operator_acceptance", ObservedAt: "2026-09-29T18:00:00+07:00", Summary: "Accepted.",
			Targets: []domain.Ref{{Kind: "task", ID: "task.a"}}},
		{ID: "evidence.a.first", Type: "command_result", ObservedAt: "2026-09-28T10:00:00+07:00", Summary: "Built.",
			Targets: []domain.Ref{{Kind: "task", ID: "task.a"}}},
	}
	ws.Actuals = []domain.Actual{
		{ID: "actual.a.2", Type: "work_consumed", TaskRef: domain.Ref{Kind: "task", ID: "task.a"}, DemandRef: domain.Ref{Kind: "resource_demand", ID: "resource_demand.a"},
			ResourceRef: domain.Ref{Kind: "resource", ID: "resource.environment.slot"}, Amount: 1, Unit: "concurrent_slots", StatusDate: "2026-09-29"},
		{ID: "actual.a.1", Type: "work_consumed", DemandRef: domain.Ref{Kind: "resource_demand", ID: "resource_demand.a"},
			Amount: 1, Unit: "concurrent_slots", OccurredAt: "2026-09-28T09:00:00+07:00"},
	}
	ws.Proposals = []domain.Proposal{
		{ID: "proposal.b.remaining", Type: "remaining_update", Status: "open", TargetRef: domain.Ref{Kind: "resource_demand", ID: "resource_demand.b"}, Rationale: "Less left."},
		{ID: "proposal.b.closed", Type: "remaining_update", Status: "accepted", TargetRef: domain.Ref{Kind: "task", ID: "task.b"}},
	}
	return ws
}

func overlayFor(t *testing.T, ws domain.Workspace) reports.DashboardOpenProjectOverlay {
	t.Helper()
	overview, err := app.NewReportService(stubLoader{ws: ws}).Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: "overlay", GeneratedAt: time.Date(2026, 9, 28, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	return overview.OpenProjectOverlay
}

func overlayRow(t *testing.T, overlay reports.DashboardOpenProjectOverlay, taskID string) reports.DashboardOpenProjectOverlayWorkPackage {
	t.Helper()
	for _, row := range overlay.WorkPackages {
		if row.RePlanTaskID == taskID {
			return row
		}
	}
	t.Fatalf("no overlay row for %s", taskID)
	return reports.DashboardOpenProjectOverlayWorkPackage{}
}

func TestOverlayVersion2ProjectFields(t *testing.T) {
	overlay := overlayFor(t, overlayWorkspace())
	if overlay.SchemaVersion != 2 {
		t.Fatalf("schema version %d, want 2", overlay.SchemaVersion)
	}
	if overlay.Span.StartDate != "2026-09-28" || overlay.Span.FinishDate != "2026-11-20" || overlay.Span.StatusOffsetDays != 2 || !overlay.Span.StatusInRange {
		t.Errorf("span %+v", overlay.Span)
	}
	if overlay.WarningCount != 1 || overlay.OpenProposalCount != 1 {
		t.Errorf("warnings %d proposals %d, want 1 and 1", overlay.WarningCount, overlay.OpenProposalCount)
	}
	if len(overlay.PersonActions) == 0 {
		t.Fatalf("no person actions; task.b waits on review")
	}
	var reviewAction bool
	for _, action := range overlay.PersonActions {
		if action.TaskID == "task.b" && action.WorkPackageID == "wp-task.b" {
			reviewAction = true
		}
	}
	if !reviewAction {
		t.Errorf("person actions %+v lack task.b with its work package", overlay.PersonActions)
	}

	resources := map[string]reports.DashboardOpenProjectOverlayResource{}
	for _, resource := range overlay.Resources {
		resources[resource.ID] = resource
	}
	person := resources["resource.human.person"]
	if person.Name != "Person" || person.Class != "human" || len(person.WorkingWeekdays) != 5 || person.WorkingWeekdays[0] != "monday" {
		t.Errorf("person %+v", person)
	}
	weeks := map[string]reports.DashboardOpenProjectOverlayWeekBucket{}
	for _, week := range person.Weeks {
		weeks[week.Label] = week
	}
	if weeks["2026-W40"].Capacity != 5 || weeks["2026-W42"].Capacity != 10 || weeks["2026-W42"].Planned != 3 {
		t.Errorf("person weeks W40 %+v W42 %+v", weeks["2026-W40"], weeks["2026-W42"])
	}
	if slot := resources["resource.environment.slot"]; !slot.Concurrent {
		t.Errorf("slot not concurrent: %+v", slot)
	}

	// The slot is double-booked on 2026-10-01 by task.b and task.c. The
	// person's 2026-W41 is over capacity in plan only: task.done has nothing
	// remaining, so, as in `replan workload`, it is not an overloaded week.
	if weeks["2026-W41"].Overload == 0 {
		t.Errorf("person 2026-W41 bucket %+v should show the planned overload", weeks["2026-W41"])
	}
	if len(overlay.OverloadedWeeks) != 1 {
		t.Fatalf("overloaded weeks %+v, want the slot in 2026-W40", overlay.OverloadedWeeks)
	}
	if got := overlay.OverloadedWeeks[0]; got.ResourceID != "resource.environment.slot" || got.Week != "2026-W40" || got.Load != 2 || got.Capacity != 1 || got.Overload != 1 {
		t.Errorf("overloaded week %+v", got)
	}
}

func TestOverlayVersion2TaskFields(t *testing.T) {
	overlay := overlayFor(t, overlayWorkspace())

	a := overlayRow(t, overlay, "task.a")
	if len(a.Evidence) != 2 || a.Evidence[0].ID != "evidence.a.first" || a.Evidence[0].Date != "2026-09-28" || a.Evidence[1].Kind != "operator_acceptance" {
		t.Errorf("evidence not dated and in order: %+v", a.Evidence)
	}
	if len(a.Actuals) != 2 || a.Actuals[0].Date != "2026-09-28" || a.Actuals[0].ResourceName != "Slot" || a.Actuals[1].Date != "2026-09-29" {
		t.Errorf("actuals: %+v", a.Actuals)
	}
	if a.FirstActualDate != "2026-09-28" || a.LastActualDate != "2026-09-29" {
		t.Errorf("actual window %s..%s", a.FirstActualDate, a.LastActualDate)
	}
	if len(a.Dependencies) != 1 || a.Dependencies[0].Direction != "successor" || a.Dependencies[0].TaskTitle != "task.b" || a.Dependencies[0].WorkPackageID != "wp-task.b" {
		t.Errorf("task.a dependencies %+v", a.Dependencies)
	}
	if len(a.Warnings) != 0 || len(a.OpenProposals) != 0 {
		t.Errorf("task.a warnings %+v proposals %+v", a.Warnings, a.OpenProposals)
	}

	b := overlayRow(t, overlay, "task.b")
	if len(b.Dependencies) != 1 || b.Dependencies[0].Direction != "predecessor" || b.Dependencies[0].TaskStatus != "accepted" || b.Dependencies[0].Status != "open" {
		t.Errorf("task.b dependencies %+v", b.Dependencies)
	}
	// The open dependency on a done predecessor is validate's warning, and
	// it belongs to the task that waits.
	if len(b.Warnings) != 1 || b.Warnings[0].Code != "required_dependency_unresolved" {
		t.Errorf("task.b warnings %+v", b.Warnings)
	}
	if len(b.OpenProposals) != 1 || b.OpenProposals[0].ID != "proposal.b.remaining" || b.OpenProposals[0].TargetID != "resource_demand.b" {
		t.Errorf("task.b proposals %+v", b.OpenProposals)
	}
	if len(b.Evidence) != 0 || len(b.Actuals) != 0 || b.FirstActualDate != "" {
		t.Errorf("task.b has facts it should not: %+v %+v", b.Evidence, b.Actuals)
	}
}

func TestOverlayVersion2LaneCapacity(t *testing.T) {
	overlay := overlayFor(t, overlayWorkspace())
	lane := func(taskID string) reports.DashboardOpenProjectOverlayResourceDemand {
		t.Helper()
		row := overlayRow(t, overlay, taskID)
		if len(row.ResourceDemands) != 1 {
			t.Fatalf("%s lanes %+v", taskID, row.ResourceDemands)
		}
		return row.ResourceDemands[0]
	}
	// 8 hours over ten working days at 10 hours a week of five days.
	if got := lane("task.long"); got.ResourceName != "Person" || got.AmountPerDay != 0.8 || got.DailyCapacity != 2 || got.Concurrent {
		t.Errorf("task.long lane %+v", got)
	}
	// The window opens on a Saturday; the capacity is Monday's.
	if got := lane("task.wkend"); got.DailyCapacity != 2 || got.AmountPerDay != 3 {
		t.Errorf("task.wkend lane %+v", got)
	}
	// A slot demand holds its slot every day against one slot.
	if got := lane("task.b"); !got.Concurrent || got.DailyCapacity != 1 || got.AmountPerDay != 1 || got.ResourceName != "Slot" {
		t.Errorf("task.b lane %+v", got)
	}
}
