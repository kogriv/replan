package app_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestWorkloadSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewWorkloadService(store)

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
	})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	if len(result.Errors) != 0 {
		t.Fatalf("expected no errors, got %#v", result.Errors)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", result.Warnings)
	}

	assertWorkload(t, result, "resource.human.ivan", "hours", 32, 8, 8, 0)
	assertWorkload(t, result, "resource.ai.codex.local", "agent_hours", 20, 2, 2, 0)
	assertWorkload(t, result, "resource.service.openai.tokens", "tokens", 10000000, 500000, 500000, 0)
	assertWorkload(t, result, "resource.project.budget", "rub", 250000, 12000, 12000, 0)
	assertWorkload(t, result, "resource.environment.staging", "concurrent_slots", 1, 1, 1, 0)
	assertWorkload(t, result, "resource_pool.reviewers", "review_hours", 8, 2, 2, 0)
}

func TestWorkloadUsesCorrectedActualAmount(t *testing.T) {
	corrected := 0.1
	ws := domain.Workspace{Project: domain.Project{ID: "project.test"}, Resources: []domain.Resource{{ID: "resource.agent", Name: "Agent"}}, Actuals: []domain.Actual{
		{ID: "actual.work", Type: "work_consumed", ResourceRef: domain.Ref{ID: "resource.agent"}, DemandRef: domain.Ref{ID: "demand.work"}, Amount: 0.33, Unit: "agent_hours", StatusDate: "2026-10-30"},
		{ID: "actual.correction", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: "actual.work"}, CorrectedAmount: &corrected, ResourceRef: domain.Ref{ID: "resource.agent"}, DemandRef: domain.Ref{ID: "demand.work"}, Unit: "agent_hours", OccurredAt: "2026-10-30T12:00:00Z"},
	}}
	result, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), app.WorkloadRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertWorkloadWithActual(t, result, "resource.agent", "agent_hours", 0, 0, corrected, 0, 0)
}

func TestWorkloadAppliesRemainingAmountScenario(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewWorkloadService(store)

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
		ScenarioID:  "scenario.task-op-1234-remaining-5",
	})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	if result.ScenarioID != "scenario.task-op-1234-remaining-5" {
		t.Fatalf("ScenarioID = %q", result.ScenarioID)
	}
	assertWorkload(t, result, "resource.human.ivan", "hours", 32, 8, 5, 0)
}

func TestWorkloadAppliesResourceReassignmentScenario(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewWorkloadService(store)

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
		ScenarioID:  "scenario.task-op-1234-reassign-petr",
	})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	assertWorkload(t, result, "resource.human.ivan", "hours", 32, 0, 0, 0)
	assertWorkloadWithActual(t, result, "resource.human.petr", "hours", 24, 12, 2, 10, 0)
}

func TestWorkloadAppliesCapacityAmountScenario(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewWorkloadService(store)

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
		ScenarioID:  "scenario.ivan-capacity-6",
	})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	assertWorkload(t, result, "resource.human.ivan", "hours", 6, 8, 8, 2)
}

func TestWorkloadAppliesQuotaCapacityScenario(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewWorkloadService(store)

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
		ScenarioID:  "scenario.openai-quota-300k",
	})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	assertWorkload(t, result, "resource.service.openai.tokens", "tokens", 300000, 500000, 500000, 200000)
}

func TestWorkloadWeekFiltersDatedDemandsAndKeepsUnscheduledDemands(t *testing.T) {
	service := app.NewWorkloadService(staticWorkspaceLoader{
		projectPath: "memory",
		workspace: domain.Workspace{
			Resources: []domain.Resource{
				{ID: "resource.human.maintainer", Class: "human", Name: "Maintainer"},
			},
			Tasks: []domain.Task{
				{
					ID:             "task.in-week",
					Title:          "In week",
					TrackingStatus: "ready",
					PlannedStart:   "2026-07-29",
					PlannedFinish:  "2026-07-30",
				},
				{
					ID:             "task.out-of-week",
					Title:          "Out of week",
					TrackingStatus: "ready",
					PlannedStart:   "2026-08-10",
					PlannedFinish:  "2026-08-10",
				},
				{
					ID:             "task.unscheduled",
					Title:          "Unscheduled",
					TrackingStatus: "ready",
				},
			},
			ResourceDemands: []domain.ResourceDemand{
				{
					ID:              "resource_demand.in-week",
					TaskRef:         domain.Ref{Kind: "task", ID: "task.in-week"},
					ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.maintainer"},
					ResourceClass:   "human",
					Role:            "maintainer",
					Unit:            "hours",
					PlannedAmount:   4,
					RemainingAmount: domain.Amount(4),
					Status:          "active",
				},
				{
					ID:              "resource_demand.out-of-week",
					TaskRef:         domain.Ref{Kind: "task", ID: "task.out-of-week"},
					ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.maintainer"},
					ResourceClass:   "human",
					Role:            "maintainer",
					Unit:            "hours",
					PlannedAmount:   8,
					RemainingAmount: domain.Amount(8),
					Status:          "active",
				},
				{
					ID:              "resource_demand.unscheduled",
					TaskRef:         domain.Ref{Kind: "task", ID: "task.unscheduled"},
					ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.maintainer"},
					ResourceClass:   "human",
					Role:            "maintainer",
					Unit:            "hours",
					PlannedAmount:   2,
					RemainingAmount: domain.Amount(2),
					Status:          "active",
				},
			},
			Actuals: []domain.Actual{
				{
					ID:          "actual.in-week",
					Type:        "work_consumed",
					TaskRef:     domain.Ref{Kind: "task", ID: "task.in-week"},
					DemandRef:   domain.Ref{Kind: "resource_demand", ID: "resource_demand.in-week"},
					ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.maintainer"},
					Amount:      3,
					Unit:        "hours",
					StatusDate:  "2026-07-30",
					AcceptedBy:  domain.Ref{Kind: "actor", ID: "actor.human.maintainer"},
				},
				{
					ID:          "actual.out-of-week",
					Type:        "work_consumed",
					TaskRef:     domain.Ref{Kind: "task", ID: "task.out-of-week"},
					DemandRef:   domain.Ref{Kind: "resource_demand", ID: "resource_demand.out-of-week"},
					ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.maintainer"},
					Amount:      8,
					Unit:        "hours",
					StatusDate:  "2026-08-10",
					AcceptedBy:  domain.Ref{Kind: "actor", ID: "actor.human.maintainer"},
				},
			},
			Capacities: []domain.Capacity{
				{
					ID:          "capacity.maintainer.weekly",
					ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.maintainer"},
					Type:        "renewable",
					Unit:        "hours",
					Amount:      10,
					Period:      "week",
				},
			},
		},
	})

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: "memory",
		Week:        "2026-W31",
	})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	assertWorkloadWithActual(t, result, "resource.human.maintainer", "hours", 10, 6, 3, 6, 0)
}

func TestWorkloadReportsMissingScenario(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewWorkloadService(store)

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
		ScenarioID:  "scenario.missing",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Calculate error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(result.Errors, "scenario_not_found") {
		t.Fatalf("expected scenario_not_found, got %#v", result.Errors)
	}
}

func TestWorkloadLoadFailurePrecedesScenarioResolution(t *testing.T) {
	service := app.NewWorkloadService(staticWorkspaceLoader{
		projectPath: "memory",
		diagnostics: []app.Diagnostic{{Code: "load_failed_first", Severity: "error"}},
	})

	result, err := service.Calculate(context.Background(), app.WorkloadRequest{ScenarioID: "scenario.missing"})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Calculate error = %v, want ErrValidationFailed", err)
	}
	want := []string{"load_failed_first"}
	if got := diagnosticCodeList(result.Errors); !reflect.DeepEqual(got, want) {
		t.Fatalf("error codes = %#v, want %#v", got, want)
	}
	if len(result.Resources) != 0 {
		t.Fatalf("partial result advanced past load validation: %#v", result.Resources)
	}
}

func assertWorkload(t *testing.T, result app.WorkloadResult, resourceID, unit string, capacity, planned, remaining, overload float64) {
	t.Helper()
	assertWorkloadWithActual(t, result, resourceID, unit, capacity, planned, 0, remaining, overload)
}

func assertWorkloadWithActual(t *testing.T, result app.WorkloadResult, resourceID, unit string, capacity, planned, actual, remaining, overload float64) {
	t.Helper()
	for _, row := range result.Resources {
		if row.ResourceID == resourceID && row.Unit == unit {
			if row.Capacity != capacity || row.Planned != planned || row.Actual != actual || row.Remaining != remaining || row.Overload != overload {
				t.Fatalf("unexpected workload for %s/%s: %#v", resourceID, unit, row)
			}
			return
		}
	}
	t.Fatalf("workload row not found for %s/%s", resourceID, unit)
}

// GAP-011: --resource-class and --unit keep only matching rows; they do not
// change the numbers of the rows they keep.
func TestWorkloadResourceClassAndUnitFilters(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewWorkloadService(store)
	ctx := context.Background()

	all, err := service.Calculate(ctx, app.WorkloadRequest{ProjectPath: "../../examples/small-team", Week: "2026-W28"})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	filtered, err := service.Calculate(ctx, app.WorkloadRequest{ProjectPath: "../../examples/small-team", Week: "2026-W28", ResourceClass: "human", Unit: "hours"})
	if err != nil {
		t.Fatalf("Calculate with filters returned error: %v", err)
	}
	want := []app.ResourceWorkload{}
	for _, row := range all.Resources {
		if row.ResourceClass == "human" && row.Unit == "hours" {
			want = append(want, row)
		}
	}
	if len(want) == 0 || len(want) == len(all.Resources) {
		t.Fatalf("fixture no longer exercises the filter: %d of %d rows match", len(want), len(all.Resources))
	}
	if !reflect.DeepEqual(filtered.Resources, want) {
		t.Fatalf("filtered rows:\n%#v\nwant:\n%#v", filtered.Resources, want)
	}
	kept := map[string]bool{}
	for _, row := range want {
		kept[row.ResourceID] = true
	}
	for _, warning := range filtered.Warnings {
		if warning.Code == "missing_capacity" && !kept[warning.Target.ID] {
			t.Fatalf("warning for a filtered-out row: %#v", warning)
		}
	}
}
