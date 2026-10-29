package app_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func TestApplyScenarioOverlaySetsTaskPlannedDatesWithoutMutatingInput(t *testing.T) {
	ws := domain.Workspace{
		Tasks: []domain.Task{{ID: "task.a", PlannedStart: "2026-07-06", PlannedFinish: "2026-07-10"}},
		Scenarios: []domain.Scenario{{
			ID: "scenario.move-task",
			Changes: []domain.ScenarioChange{
				{Op: "set", Path: "tasks.task.a.planned_start", Value: "2026-07-13"},
				{Op: "replace", Path: "tasks.task.a.planned_finish", Value: "2026-07-17"},
			},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.move-task")

	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %#v", result.Errors)
	}
	if ws.Tasks[0].PlannedStart != "2026-07-06" || ws.Tasks[0].PlannedFinish != "2026-07-10" {
		t.Fatalf("input workspace was mutated: %#v", ws.Tasks[0])
	}
	if got := result.Workspace.Tasks[0]; got.PlannedStart != "2026-07-13" || got.PlannedFinish != "2026-07-17" {
		t.Fatalf("overlaid task = %#v", got)
	}
}

func TestScenarioTaskDateOverlayMatchesApplyValidation(t *testing.T) {
	tests := []struct {
		name    string
		tasks   []domain.Task
		changes []domain.ScenarioChange
		want    string
	}{
		{name: "only start", tasks: []domain.Task{{ID: "task.a", PlannedStart: "2026-07-06", PlannedFinish: "2026-07-17"}}, changes: []domain.ScenarioChange{{Op: "set", Path: "tasks.task.a.planned_start", Value: "2026-07-13"}}},
		{name: "only finish", tasks: []domain.Task{{ID: "task.a", PlannedStart: "2026-07-06", PlannedFinish: "2026-07-10"}}, changes: []domain.ScenarioChange{{Op: "set", Path: "tasks.task.a.planned_finish", Value: "2026-07-17"}}},
		{name: "finish before start", tasks: []domain.Task{{ID: "task.a", PlannedStart: "2026-07-06", PlannedFinish: "2026-07-10"}}, changes: []domain.ScenarioChange{{Op: "set", Path: "tasks.task.a.planned_start", Value: "2026-07-11"}}, want: "planned_finish_before_start"},
		{name: "unknown task", tasks: []domain.Task{{ID: "task.a"}}, changes: []domain.ScenarioChange{{Op: "set", Path: "tasks.task.missing.planned_start", Value: "2026-07-13"}}, want: "unsupported_scenario_apply_change"},
		{name: "bad date", tasks: []domain.Task{{ID: "task.a"}}, changes: []domain.ScenarioChange{{Op: "set", Path: "tasks.task.a.planned_start", Value: "tomorrow"}}, want: "unsupported_scenario_apply_change"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			scenario := domain.Scenario{ID: "scenario.test", Changes: tc.changes}
			ws := domain.Workspace{Tasks: tc.tasks, Scenarios: []domain.Scenario{scenario}}
			overlay := app.ApplyScenarioOverlay(ws, scenario.ID)
			apply := app.NewScenarioService(scenarioStaticLoader{ws: ws})
			applied, _ := apply.Apply(t.Context(), app.ScenarioApplyRequest{ID: scenario.ID, DryRun: true})
			if !reflect.DeepEqual(overlay.Errors, applied.Errors) {
				t.Fatalf("overlay errors = %#v, apply errors = %#v", overlay.Errors, applied.Errors)
			}
			if tc.want == "" {
				if len(overlay.Errors) != 0 {
					t.Fatalf("unexpected errors: %#v", overlay.Errors)
				}
			} else if !hasDiagnosticCode(overlay.Errors, tc.want) {
				t.Fatalf("expected %s, got %#v", tc.want, overlay.Errors)
			}
		})
	}
}

type scenarioStaticLoader struct{ ws domain.Workspace }

func (l scenarioStaticLoader) Load(_ context.Context) (domain.Workspace, []app.Diagnostic) {
	return l.ws, nil
}

func (l scenarioStaticLoader) ProjectPath() string { return "" }

func TestApplyScenarioOverlayAddsAvailabilityExceptionWithoutMutatingInput(t *testing.T) {
	ws := domain.Workspace{
		Project: domain.Project{Timezone: "UTC"},
		Resources: []domain.Resource{{
			ID:    "resource.human.ivan",
			Class: "human",
			Name:  "Ivan",
		}},
		Scenarios: []domain.Scenario{{
			ID: "scenario.ivan-unavailable",
			Changes: []domain.ScenarioChange{{
				Op:   "add",
				Path: "availability.exceptions",
				Value: map[string]any{
					"resource_ref": map[string]any{
						"kind": "resource",
						"id":   "resource.human.ivan",
					},
					"date":   "2026-07-09",
					"type":   "unavailable",
					"reason": "scenario assumption",
				},
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.ivan-unavailable")

	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %#v", result.Errors)
	}
	if len(ws.Availabilities) != 0 {
		t.Fatalf("input workspace was mutated: %#v", ws.Availabilities)
	}
	if len(result.Workspace.Availabilities) != 1 {
		t.Fatalf("Availabilities len = %d, want 1", len(result.Workspace.Availabilities))
	}
	exception := result.Workspace.Availabilities[0].Exceptions[0]
	if exception.Date != "2026-07-09" || exception.Type != "unavailable" {
		t.Fatalf("Exception = %#v", exception)
	}
}

func TestApplyScenarioOverlaySetsRemainingAmountWithoutMutatingInput(t *testing.T) {
	ws := domain.Workspace{
		ResourceDemands: []domain.ResourceDemand{{
			ID:              "resource_demand.task-op-1234.implementer",
			PlannedAmount:   8,
			RemainingAmount: domain.Amount(8),
		}},
		Scenarios: []domain.Scenario{{
			ID: "scenario.remaining-5",
			Changes: []domain.ScenarioChange{{
				Op:    "set",
				Path:  "resource_demands.resource_demand.task-op-1234.implementer.remaining_amount",
				Value: 5,
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.remaining-5")

	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %#v", result.Errors)
	}
	if ws.ResourceDemands[0].Remaining() != 8 {
		t.Fatalf("input workspace was mutated: %#v", ws.ResourceDemands[0])
	}
	if result.Workspace.ResourceDemands[0].Remaining() != 5 {
		t.Fatalf("RemainingAmount = %v, want 5", result.Workspace.ResourceDemands[0].Remaining())
	}
}

func TestApplyScenarioOverlaySetsPlannedAmountWithoutMutatingInput(t *testing.T) {
	ws := domain.Workspace{
		ResourceDemands: []domain.ResourceDemand{{
			ID:              "resource_demand.task-op-1234.implementer",
			PlannedAmount:   8,
			RemainingAmount: domain.Amount(0),
		}},
		Scenarios: []domain.Scenario{{
			ID: "scenario.planned-10",
			Changes: []domain.ScenarioChange{{
				Op:    "replace",
				Path:  "resource_demands.resource_demand.task-op-1234.implementer.planned_amount",
				Value: 10,
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.planned-10")

	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %#v", result.Errors)
	}
	if ws.ResourceDemands[0].PlannedAmount != 8 {
		t.Fatalf("input workspace was mutated: %#v", ws.ResourceDemands[0])
	}
	if result.Workspace.ResourceDemands[0].PlannedAmount != 10 {
		t.Fatalf("PlannedAmount = %v, want 10", result.Workspace.ResourceDemands[0].PlannedAmount)
	}
}

func TestApplyScenarioOverlaySetsResourceRefWithoutMutatingInput(t *testing.T) {
	ws := domain.Workspace{
		Resources: []domain.Resource{
			{ID: "resource.human.ivan", Class: "human", Name: "Ivan"},
			{ID: "resource.human.petr", Class: "human", Name: "Petr"},
		},
		ResourceDemands: []domain.ResourceDemand{{
			ID:          "resource_demand.task-op-1234.implementer",
			ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
		}},
		Scenarios: []domain.Scenario{{
			ID: "scenario.reassign-petr",
			Changes: []domain.ScenarioChange{{
				Op:   "set",
				Path: "resource_demands.resource_demand.task-op-1234.implementer.resource_ref",
				Value: map[string]any{
					"kind": "resource",
					"id":   "resource.human.petr",
				},
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.reassign-petr")

	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %#v", result.Errors)
	}
	if ws.ResourceDemands[0].ResourceRef.ID != "resource.human.ivan" {
		t.Fatalf("input workspace was mutated: %#v", ws.ResourceDemands[0])
	}
	if result.Workspace.ResourceDemands[0].ResourceRef.ID != "resource.human.petr" {
		t.Fatalf("ResourceRef = %#v", result.Workspace.ResourceDemands[0].ResourceRef)
	}
}

func TestApplyScenarioOverlaySetsCapacityAmountWithoutMutatingInput(t *testing.T) {
	ws := domain.Workspace{
		Capacities: []domain.Capacity{{
			ID:     "capacity.human-ivan.hours.weekly",
			Amount: 32,
			Unit:   "hours",
		}},
		Scenarios: []domain.Scenario{{
			ID: "scenario.ivan-capacity-6",
			Changes: []domain.ScenarioChange{{
				Op:    "set",
				Path:  "capacities.capacity.human-ivan.hours.weekly.amount",
				Value: 6,
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.ivan-capacity-6")

	if len(result.Errors) != 0 {
		t.Fatalf("Errors = %#v", result.Errors)
	}
	if ws.Capacities[0].Amount != 32 {
		t.Fatalf("input workspace was mutated: %#v", ws.Capacities[0])
	}
	if result.Workspace.Capacities[0].Amount != 6 {
		t.Fatalf("Capacity amount = %v, want 6", result.Workspace.Capacities[0].Amount)
	}
}

func TestApplyScenarioOverlayReportsUnsupportedChange(t *testing.T) {
	ws := domain.Workspace{
		Scenarios: []domain.Scenario{{
			ID: "scenario.unsupported",
			Changes: []domain.ScenarioChange{{
				Op:   "replace",
				Path: "resource_demands.resource_demand.task-op-1234.implementer.resource_pool_ref",
				Value: map[string]any{
					"kind": "resource_pool",
					"id":   "resource_pool.reviewers",
				},
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.unsupported")

	if !hasDiagnosticCode(result.Errors, "unsupported_scenario_change") {
		t.Fatalf("expected unsupported_scenario_change, got %#v", result.Errors)
	}
}

func TestApplyScenarioOverlayReportsUnknownCapacity(t *testing.T) {
	ws := domain.Workspace{
		Scenarios: []domain.Scenario{{
			ID: "scenario.unknown-capacity",
			Changes: []domain.ScenarioChange{{
				Op:    "set",
				Path:  "capacities.capacity.missing.amount",
				Value: 6,
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.unknown-capacity")

	if !hasDiagnosticCode(result.Errors, "scenario_unknown_capacity") {
		t.Fatalf("expected scenario_unknown_capacity, got %#v", result.Errors)
	}
}

func TestApplyScenarioOverlayReportsUnknownResourceForReassignment(t *testing.T) {
	ws := domain.Workspace{
		ResourceDemands: []domain.ResourceDemand{{
			ID:          "resource_demand.task-op-1234.implementer",
			ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
		}},
		Scenarios: []domain.Scenario{{
			ID: "scenario.unknown-resource",
			Changes: []domain.ScenarioChange{{
				Op:   "set",
				Path: "resource_demands.resource_demand.task-op-1234.implementer.resource_ref",
				Value: map[string]any{
					"kind": "resource",
					"id":   "resource.human.missing",
				},
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.unknown-resource")

	if !hasDiagnosticCode(result.Errors, "scenario_unknown_resource") {
		t.Fatalf("expected scenario_unknown_resource, got %#v", result.Errors)
	}
}

func TestApplyScenarioOverlayReportsUnknownResourceDemand(t *testing.T) {
	ws := domain.Workspace{
		Scenarios: []domain.Scenario{{
			ID: "scenario.unknown-demand",
			Changes: []domain.ScenarioChange{{
				Op:    "set",
				Path:  "resource_demands.resource_demand.missing.remaining_amount",
				Value: 5,
			}},
		}},
	}

	result := app.ApplyScenarioOverlay(ws, "scenario.unknown-demand")

	if !hasDiagnosticCode(result.Errors, "scenario_unknown_resource_demand") {
		t.Fatalf("expected scenario_unknown_resource_demand, got %#v", result.Errors)
	}
}
