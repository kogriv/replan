package app_test

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

// capacityChangeWorkspace has the maintainer at 10 hours a week until Sunday
// 2026-09-27 and at 5 hours from Monday 2026-09-28, plus a local slot that
// exists only from Wednesday 2026-09-30.
func capacityChangeWorkspace(statusDate, oldTo, newFrom string) domain.Workspace {
	maintainer := domain.Ref{Kind: "resource", ID: "resource.human.maintainer"}
	return domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: statusDate},
		Resources: []domain.Resource{{ID: maintainer.ID, Class: "human", Name: "Maintainer"}, {ID: "resource.environment.local", Class: "environment"}},
		Capacities: []domain.Capacity{
			{ID: "capacity.maintainer.old", ResourceRef: maintainer, ResourceClass: "human", Type: "periodic_capacity", Unit: "hours", Amount: 10, Period: "week", EffectiveFrom: "2026-07-20", EffectiveTo: oldTo},
			{ID: "capacity.maintainer.new", ResourceRef: maintainer, ResourceClass: "human", Type: "periodic_capacity", Unit: "hours", Amount: 5, Period: "week", EffectiveFrom: newFrom},
			{ID: "capacity.local.slot", ResourceRef: domain.Ref{Kind: "resource", ID: "resource.environment.local"}, Type: "concurrent_capacity", Unit: "concurrent_slots", Amount: 1, Period: "concurrent", EffectiveFrom: "2026-09-30"},
		},
	}
}

func workloadCapacity(t *testing.T, ws domain.Workspace, week, resourceID string) float64 {
	t.Helper()
	result, err := app.NewWorkloadService(stubLoader{ws: ws}).Calculate(context.Background(), app.WorkloadRequest{Week: week})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	for _, row := range result.Resources {
		if row.ResourceID == resourceID {
			return row.Capacity
		}
	}
	return 0
}

func TestWorkloadCountsCapacityOnlyWhileInEffect(t *testing.T) {
	ws := capacityChangeWorkspace("2026-09-25", "2026-09-27", "2026-09-28")
	for _, tc := range []struct {
		name, week, resource string
		want                 float64
	}{
		{"week before the change", "2026-W39", "resource.human.maintainer", 10},
		{"week after the change", "2026-W40", "resource.human.maintainer", 5},
		{"week before the old capacity", "2026-W29", "resource.human.maintainer", 0},
		{"no week uses the status date", "", "resource.human.maintainer", 10},
		{"slot not yet in effect", "2026-W39", "resource.environment.local", 0},
		{"slot in effect for part of the week counts whole", "2026-W40", "resource.environment.local", 1},
	} {
		if got := workloadCapacity(t, ws, tc.week, tc.resource); got != tc.want {
			t.Errorf("%s: capacity = %v, want %v", tc.name, got, tc.want)
		}
	}

	later := capacityChangeWorkspace("2026-10-01", "2026-09-27", "2026-09-28")
	if got := workloadCapacity(t, later, "", "resource.human.maintainer"); got != 5 {
		t.Errorf("status date after the change: capacity = %v, want 5", got)
	}
	undated := capacityChangeWorkspace("", "2026-09-27", "2026-09-28")
	if got := workloadCapacity(t, undated, "", "resource.human.maintainer"); got != 15 {
		t.Errorf("no status date keeps summing every capacity: capacity = %v, want 15", got)
	}
}

func TestWorkloadSplitsWeeklyCapacityChangedMidWeek(t *testing.T) {
	// 10 hours for Monday and Tuesday, 5 hours for Wednesday to Sunday; workload
	// rounds to four places.
	ws := capacityChangeWorkspace("2026-09-25", "2026-09-29", "2026-09-30")
	got := workloadCapacity(t, ws, "2026-W40", "resource.human.maintainer")
	if want := 10*2.0/7 + 5*5.0/7; math.Abs(got-want) > 1e-4 {
		t.Fatalf("capacity = %v, want %v", got, want)
	}
}

func TestValidateRejectsBadCapacityEffectiveDates(t *testing.T) {
	for _, tc := range []struct {
		name, from, to, code string
	}{
		{"malformed from", "2026-9-28", "", "invalid_capacity_date"},
		{"malformed to", "", "soon", "invalid_capacity_date"},
		{"range ends before it starts", "2026-09-28", "2026-09-27", "invalid_capacity_range"},
	} {
		ws := capacityChangeWorkspace("2026-09-25", "2026-09-27", "2026-09-28")
		ws.Capacities[1].EffectiveFrom, ws.Capacities[1].EffectiveTo = tc.from, tc.to
		result, err := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
		if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, tc.code) {
			t.Errorf("%s: want %s, got err %v and %#v", tc.name, tc.code, err, result.Errors)
		}
	}
	ws := capacityChangeWorkspace("2026-09-25", "2026-09-27", "2026-09-28")
	result, _ := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
	if hasDiagnostic(result.Errors, "invalid_capacity_date") || hasDiagnostic(result.Errors, "invalid_capacity_range") {
		t.Fatalf("valid effective dates reported: %#v", result.Errors)
	}
}

func TestExportTaskJugglerUsesCapacityInEffectOnStatusDate(t *testing.T) {
	for _, tc := range []struct {
		statusDate, limit string
	}{
		{"2026-09-25", "limits { weeklymax 10.00h }"},
		{"2026-10-01", "limits { weeklymax 5.00h }"},
	} {
		ws := capacityChangeWorkspace(tc.statusDate, "2026-09-27", "2026-09-28")
		result, err := app.NewExportService(staticWorkspaceLoader{projectPath: "test-project", workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
			Now: time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC),
		})
		if err != nil {
			t.Fatalf("TaskJuggler returned error: %v", err)
		}
		assertContains(t, result.Content, tc.limit)
		if !hasDiagnosticTarget(result.Warnings, "taskjuggler_capacity_effective_range_simplified", "capacity", "capacity.maintainer.old") {
			t.Errorf("status date %s: missing simplification warning, got %#v", tc.statusDate, result.Warnings)
		}
	}
}
