package app_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestExportTaskJugglerSmallTeamFixture(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		Now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	if result.Metadata.ID != "export.replan.taskjuggler.20260710T120000Z" {
		t.Fatalf("Metadata.ID = %q", result.Metadata.ID)
	}
	assertContains(t, result.Content, `project project_replan_mvp "RePlan MVP example" "1.0" 2026-07-06 +1y`)
	assertContains(t, result.Content, `resource resource_human_ivan "Ivan"`)
	assertContains(t, result.Content, `limits { weeklymax 32.00h }`)
	assertNotContains(t, result.Content, `note "RePlan class: human"`)
	assertContains(t, result.Content, `task task_op_1234 "Implement validation command"`)
	assertContains(t, result.Content, `effort 10.00h`)
	assertContains(t, result.Content, `allocate resource_human_ivan`)
	assertContains(t, result.Content, `allocate resource_ai_codex_local`)
	assertContains(t, result.Content, `taskreport replan_task_schedule "reports/replan-task-schedule"`)
	assertContains(t, result.Content, `columns id, name, start, end, effort, effortleft, resources, status`)
	if !hasTaskJugglerTaskMapping(result.Mapping, "task.op.1234", "task_op_1234") {
		t.Fatalf("expected task mapping for task.op.1234, got %#v", result.Mapping.TaskIDMap)
	}
	if !hasTaskJugglerResourceMapping(result.Mapping, "resource.human.ivan", "resource_human_ivan") {
		t.Fatalf("expected resource mapping for resource.human.ivan, got %#v", result.Mapping.ResourceIDMap)
	}
	if len(result.Warnings) == 0 {
		t.Fatalf("expected warnings for unsupported non-hour or non-concrete demands")
	}
	if !hasDiagnosticCode(result.Warnings, "taskjuggler_unsupported_demand_unit") {
		t.Fatalf("expected unsupported demand unit warning, got %#v", result.Warnings)
	}
}

func TestExportTaskJugglerRawExportsAgentHours(t *testing.T) {
	result, err := app.NewExportService(filestore.New("../../examples/small-team")).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		Raw: true,
		Now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, `effort 10.00h`)
	assertContains(t, result.Content, `allocate resource_ai_codex_local`)
	assertContains(t, result.Content, `limits { weeklymax 20.00h }`)
}

func TestExportTaskJugglerUsesCalibratedEffortAndNamesCalibration(t *testing.T) {
	ws := calibratedTaskJugglerWorkspace(t)
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		Now: time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, `# calibrated by calibration.agent: 4.00h -> 0.75h`)
	assertContains(t, result.Content, `effort 0.75h`)
	assertContains(t, result.Content, `limits { weeklymax 10.00h }`)
	if len(result.Calibrations) != 1 || result.Calibrations[0] != "calibration.agent" {
		t.Fatalf("calibrations = %#v", result.Calibrations)
	}
}

func TestExportTaskJugglerRawRestoresUncalibratedOutput(t *testing.T) {
	ws := calibratedTaskJugglerWorkspace(t)
	withoutCalibration := ws
	withoutCalibration.Calibrations = nil
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	raw, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{Raw: true, Now: now})
	if err != nil {
		t.Fatal(err)
	}
	want, err := app.NewExportService(staticWorkspaceLoader{workspace: withoutCalibration}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{Now: now})
	if err != nil {
		t.Fatal(err)
	}
	if raw.Content != want.Content {
		t.Fatalf("raw export changed output\nraw:\n%s\nwant:\n%s", raw.Content, want.Content)
	}
	assertContains(t, raw.Content, `effort 4.00h`)
	assertContains(t, raw.Content, `allocate resource_agent`)
	assertContains(t, raw.Content, `limits { weeklymax 10.00h }`)
	assertNotContains(t, raw.Content, `# calibrated by`)
	if hasDiagnosticCode(raw.Warnings, "taskjuggler_unsupported_demand_unit") {
		t.Fatalf("agent_hours demand reported as unsupported: %#v", raw.Warnings)
	}
	if len(raw.Calibrations) != 0 {
		t.Fatalf("raw calibrations = %#v", raw.Calibrations)
	}
}

func TestExportTaskJugglerRawTreatsAgentHoursCapacityScenarioAsRepresented(t *testing.T) {
	ws := calibratedTaskJugglerWorkspace(t)
	ws.Scenarios = []domain.Scenario{{
		ID: "scenario.agent-capacity",
		Changes: []domain.ScenarioChange{{
			Op: "set", Path: "capacities.capacity.agent.amount", Value: 6,
		}},
	}}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID:                   "scenario.agent-capacity",
		UnrepresentedAssumptionsMode: "error",
		Raw:                          true,
	})
	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v; errors=%#v", err, result.Errors)
	}
	if hasDiagnosticCode(result.Warnings, "taskjuggler_capacity_change_not_represented") || hasDiagnosticCode(result.Errors, "taskjuggler_capacity_change_not_represented") {
		t.Fatalf("agent_hours capacity should be represented: warnings=%#v errors=%#v", result.Warnings, result.Errors)
	}
	assertContains(t, result.Content, `limits { weeklymax 6.00h }`)
}

func TestExportTaskJugglerDoesNotReapplyScenarioAppliedCalibration(t *testing.T) {
	ws := calibratedTaskJugglerWorkspace(t)
	ws.Scenarios = []domain.Scenario{{
		ID: "scenario.calibrated", CalibrationRef: domain.Ref{ID: "calibration.agent"},
		Changes: []domain.ScenarioChange{{Op: "replace", Path: "resource_demands.demand.agent.remaining_amount", Value: .75}},
	}}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{ScenarioID: "scenario.calibrated"})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, `effort 0.75h`)
	assertNotContains(t, result.Content, `# calibrated by`)
	if len(result.Calibrations) != 0 {
		t.Fatalf("calibrations = %#v", result.Calibrations)
	}
}

func TestExportTaskJugglerClosedDemandIsNotCalibratedOrScheduled(t *testing.T) {
	ws := calibratedTaskJugglerWorkspace(t)
	ws.ResourceDemands[0].Status = "closed"
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, result.Content, `# calibrated by`)
	assertNotContains(t, result.Content, `effort 0.75h`)
	if len(result.Calibrations) != 0 {
		t.Fatalf("calibrations = %#v", result.Calibrations)
	}
}

func calibratedTaskJugglerWorkspace(t *testing.T) domain.Workspace {
	t.Helper()
	calibration := domain.Calibration{
		ID: "calibration.agent", ApprovedAt: "2026-11-04T10:00:00Z",
		Scope:  domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "agent_hours"},
		Result: domain.CalibrationResult{Factor: .21},
	}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	return domain.Workspace{
		Project:         domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-11-04"},
		Resources:       []domain.Resource{{ID: "resource.agent", Class: "ai_agent", Name: "Agent"}},
		Tasks:           []domain.Task{{ID: "task.agent", Title: "Agent task", ResourceDemands: []domain.Ref{{ID: "demand.agent"}}}},
		ResourceDemands: []domain.ResourceDemand{{ID: "demand.agent", TaskRef: domain.Ref{ID: "task.agent"}, ResourceRef: domain.Ref{ID: "resource.agent"}, ResourceClass: "ai_agent", Unit: "agent_hours", PlannedAmount: 4, RemainingAmount: domain.Amount(4), Status: "active"}},
		Calibrations:    []domain.Calibration{calibration},
		Capacities:      []domain.Capacity{{ID: "capacity.agent", ResourceRef: domain.Ref{ID: "resource.agent"}, ResourceClass: "ai_agent", Type: "periodic_capacity", Unit: "agent_hours", Amount: 10, Period: "week"}},
	}
}

func TestExportTaskJugglerOmitsRelates(t *testing.T) {
	service := app.NewExportService(staticWorkspaceLoader{workspace: domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-07-10"},
		Tasks:   []domain.Task{{ID: "task.a", Title: "A", TrackingStatus: "ready"}, {ID: "task.b", Title: "B", TrackingStatus: "ready"}},
		Dependencies: []domain.Dependency{{
			ID: "dependency.related", Type: "relates", Status: "open", Criticality: "optional",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
		}},
	}})
	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{Now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	assertNotContains(t, result.Content, "dependency.related")
	assertNotContains(t, result.Content, "relates")
}

func TestExportTaskJugglerAppliesScenarioAvailabilityOverlay(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.ivan-unavailable-week-28",
		Now:        time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	if result.Scenario != "scenario.ivan-unavailable-week-28" {
		t.Fatalf("Scenario = %q", result.Scenario)
	}
	assertContains(t, result.Content, `# Scenario: scenario.ivan-unavailable-week-28`)
	assertContains(t, result.Content, `resource resource_human_ivan "Ivan"`)
	assertContains(t, result.Content, `vacation 2026-07-09`)
}

func TestExportTaskJugglerAcceptsTaskDateScenarioOverlay(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	configureMovedTaskDateFixture(t, projectPath)
	result, err := app.NewExportService(filestore.New(projectPath)).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.move-validation-to-week-29",
		Now:        time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v; errors=%#v", err, result.Errors)
	}
	assertContains(t, result.Content, `# Scenario: scenario.move-validation-to-week-29`)
}

func TestExportTaskJugglerAppliesScenarioRemainingAmountOverlay(t *testing.T) {
	service := app.NewExportService(staticWorkspaceLoader{
		projectPath: "test-project",
		workspace: domain.Workspace{
			Project: domain.Project{
				ID:                "project.test",
				Name:              "Test project",
				Timezone:          "UTC",
				DefaultStatusDate: "2026-07-06",
			},
			Resources: []domain.Resource{{
				ID:    "resource.human.ivan",
				Class: "human",
				Name:  "Ivan",
			}},
			Tasks: []domain.Task{{
				ID:    "task.op.1234",
				Title: "Implement validation command",
			}},
			ResourceDemands: []domain.ResourceDemand{{
				ID:              "resource_demand.task-op-1234.implementer",
				TaskRef:         domain.Ref{Kind: "task", ID: "task.op.1234"},
				ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
				ResourceClass:   "human",
				Role:            "implementer",
				Unit:            "hours",
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
		},
	})

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.remaining-5",
		Now:        time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	assertContains(t, result.Content, `# Scenario: scenario.remaining-5`)
	assertContains(t, result.Content, `effort 5.00h`)
	assertContains(t, result.Content, `allocate resource_human_ivan`)
}

func TestExportTaskJugglerSkipsClosedDemandEffort(t *testing.T) {
	for _, status := range []string{"cancelled", "superseded", "released"} {
		t.Run(status, func(t *testing.T) {
			ws := taskJugglerWorkspaceWithDemandStatus(status)
			result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{Now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(result.Content, "effort 8.00h") || strings.Contains(result.Content, "allocate resource_human_ivan") {
				t.Fatalf("closed demand exported for status %q:\n%s", status, result.Content)
			}
		})
	}
}

func taskJugglerWorkspaceWithDemandStatus(status string) domain.Workspace {
	return domain.Workspace{
		Project:         domain.Project{ID: "project.test", Name: "Test project", Timezone: "UTC", DefaultStatusDate: "2026-07-06"},
		Resources:       []domain.Resource{{ID: "resource.human.ivan", Class: "human", Name: "Ivan"}},
		Tasks:           []domain.Task{{ID: "task.test", Title: "Test task"}},
		ResourceDemands: []domain.ResourceDemand{{ID: "demand.test", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.human.ivan"}, ResourceClass: "human", Unit: "hours", PlannedAmount: 8, RemainingAmount: domain.Amount(8), Status: status}},
	}
}

func TestExportTaskJugglerAppliesScenarioPlannedAmountOverlayAsFallbackEffort(t *testing.T) {
	service := app.NewExportService(staticWorkspaceLoader{
		projectPath: "test-project",
		workspace: domain.Workspace{
			Project: domain.Project{
				ID:                "project.test",
				Name:              "Test project",
				Timezone:          "UTC",
				DefaultStatusDate: "2026-07-06",
			},
			Resources: []domain.Resource{{
				ID:    "resource.human.ivan",
				Class: "human",
				Name:  "Ivan",
			}},
			Tasks: []domain.Task{{
				ID:    "task.op.1234",
				Title: "Implement validation command",
			}},
			ResourceDemands: []domain.ResourceDemand{{
				ID:            "resource_demand.task-op-1234.implementer",
				TaskRef:       domain.Ref{Kind: "task", ID: "task.op.1234"},
				ResourceRef:   domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
				ResourceClass: "human",
				Role:          "implementer",
				Unit:          "hours",
				PlannedAmount: 8,
			}},
			Scenarios: []domain.Scenario{{
				ID: "scenario.planned-10",
				Changes: []domain.ScenarioChange{{
					Op:    "replace",
					Path:  "resource_demands.resource_demand.task-op-1234.implementer.planned_amount",
					Value: 10,
				}},
			}},
		},
	})

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.planned-10",
		Now:        time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	assertContains(t, result.Content, `# Scenario: scenario.planned-10`)
	assertContains(t, result.Content, `effort 10.00h`)
	assertContains(t, result.Content, `allocate resource_human_ivan`)
}

func TestExportTaskJugglerAppliesScenarioResourceReassignmentOverlay(t *testing.T) {
	service := app.NewExportService(staticWorkspaceLoader{
		projectPath: "test-project",
		workspace: domain.Workspace{
			Project: domain.Project{
				ID:                "project.test",
				Name:              "Test project",
				Timezone:          "UTC",
				DefaultStatusDate: "2026-07-06",
			},
			Resources: []domain.Resource{
				{ID: "resource.human.ivan", Class: "human", Name: "Ivan"},
				{ID: "resource.human.petr", Class: "human", Name: "Petr"},
			},
			Tasks: []domain.Task{{
				ID:    "task.op.1234",
				Title: "Implement validation command",
			}},
			ResourceDemands: []domain.ResourceDemand{{
				ID:              "resource_demand.task-op-1234.implementer",
				TaskRef:         domain.Ref{Kind: "task", ID: "task.op.1234"},
				ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
				ResourceClass:   "human",
				Role:            "implementer",
				Unit:            "hours",
				PlannedAmount:   8,
				RemainingAmount: domain.Amount(8),
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
		},
	})

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.reassign-petr",
		Now:        time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	assertContains(t, result.Content, `# Scenario: scenario.reassign-petr`)
	assertContains(t, result.Content, `effort 8.00h`)
	assertContains(t, result.Content, `allocate resource_human_petr`)
	assertNotContains(t, result.Content, `allocate resource_human_ivan`)
}

func TestExportTaskJugglerMapsHumanWeeklyCapacityScenarioLimit(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.ivan-capacity-6",
		Now:        time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	if result.Scenario != "scenario.ivan-capacity-6" {
		t.Fatalf("Scenario = %q", result.Scenario)
	}
	if hasDiagnosticTarget(result.Warnings, "taskjuggler_capacity_change_not_represented", "capacity", "capacity.human-ivan.hours.weekly") {
		t.Fatalf("Ivan weekly hour capacity should be represented, got warnings %#v", result.Warnings)
	}
	assertContains(t, result.Content, `# Scenario: scenario.ivan-capacity-6`)
	assertContains(t, result.Content, `resource resource_human_ivan "Ivan"`)
	assertContains(t, result.Content, `limits { weeklymax 6.00h }`)
}

func TestExportTaskJugglerStrictModePassesForMappedHumanCapacityScenario(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID:                   "scenario.ivan-capacity-6",
		UnrepresentedAssumptionsMode: "error",
		Now:                          time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	if hasDiagnosticTarget(result.Errors, "taskjuggler_capacity_change_not_represented", "capacity", "capacity.human-ivan.hours.weekly") {
		t.Fatalf("Ivan weekly hour capacity should be represented, got errors %#v", result.Errors)
	}
	assertContains(t, result.Content, `# Scenario: scenario.ivan-capacity-6`)
	assertContains(t, result.Content, `limits { weeklymax 6.00h }`)
}

func TestExportTaskJugglerMapsAIAgentWeeklyHourCapacityLimit(t *testing.T) {
	service := app.NewExportService(staticWorkspaceLoader{
		projectPath: "test-project",
		workspace: domain.Workspace{
			Project: domain.Project{
				ID:                "project.test",
				Name:              "Test project",
				Timezone:          "UTC",
				DefaultStatusDate: "2026-07-06",
			},
			Resources: []domain.Resource{{
				ID:    "resource.ai.coder",
				Class: "ai_agent",
				Name:  "AI coder",
			}},
			Capacities: []domain.Capacity{{
				ID:            "capacity.ai-coder.hours.weekly",
				ResourceRef:   domain.Ref{Kind: "resource", ID: "resource.ai.coder"},
				ResourceClass: "ai_agent",
				Type:          "periodic_capacity",
				Unit:          "hours",
				Amount:        12,
				Period:        "week",
			}},
		},
	})

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		Now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	assertContains(t, result.Content, `resource resource_ai_coder "AI coder"`)
	assertContains(t, result.Content, `limits { weeklymax 12.00h }`)
}

func TestExportTaskJugglerWarnsWhenQuotaScenarioIsNotRepresented(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.openai-quota-300k",
		Now:        time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	if result.Scenario != "scenario.openai-quota-300k" {
		t.Fatalf("Scenario = %q", result.Scenario)
	}
	if !hasDiagnosticCode(result.Warnings, "taskjuggler_capacity_change_not_represented") {
		t.Fatalf("expected unrepresented capacity warning, got %#v", result.Warnings)
	}
	if !hasDiagnosticTarget(result.Warnings, "taskjuggler_capacity_change_not_represented", "capacity", "capacity.openai.tokens.monthly") {
		t.Fatalf("expected warning for OpenAI quota capacity, got %#v", result.Warnings)
	}
	assertContains(t, result.Content, `# Scenario: scenario.openai-quota-300k`)
}

func TestExportTaskJugglerCanFailOnUnrepresentedCapacityScenario(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID:                   "scenario.openai-quota-300k",
		UnrepresentedAssumptionsMode: "error",
		Now:                          time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("TaskJuggler error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticTarget(result.Errors, "taskjuggler_capacity_change_not_represented", "capacity", "capacity.openai.tokens.monthly") {
		t.Fatalf("expected error for OpenAI quota capacity, got %#v", result.Errors)
	}
	if result.Content != "" {
		t.Fatalf("Content should be empty when strict unrepresented assumptions fail, got:\n%s", result.Content)
	}
}

func TestExportTaskJugglerReportsMissingScenario(t *testing.T) {
	service := app.NewExportService(filestore.New("../../examples/small-team"))

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID: "scenario.missing",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("TaskJuggler error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(result.Errors, "scenario_not_found") {
		t.Fatalf("expected scenario_not_found error, got %#v", result.Errors)
	}
}

func TestExportTaskJugglerReportsTaskIDCollision(t *testing.T) {
	service := app.NewExportService(staticWorkspaceLoader{
		projectPath: "test-project",
		workspace: domain.Workspace{
			Project: domain.Project{
				ID:                "project.test",
				Name:              "Test project",
				Timezone:          "UTC",
				DefaultStatusDate: "2026-07-06",
			},
			Tasks: []domain.Task{
				{ID: "task.a-b", Title: "Task A"},
				{ID: "task.a_b", Title: "Task B"},
			},
		},
	})

	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("TaskJuggler error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(result.Errors, "taskjuggler_task_id_collision") {
		t.Fatalf("expected taskjuggler_task_id_collision error, got %#v", result.Errors)
	}
	if result.Content != "" {
		t.Fatalf("Content should be empty when mapping collides, got:\n%s", result.Content)
	}
}

type staticWorkspaceLoader struct {
	projectPath string
	workspace   domain.Workspace
	diagnostics []app.Diagnostic
}

func (l staticWorkspaceLoader) Load(context.Context) (domain.Workspace, []app.Diagnostic) {
	return l.workspace, l.diagnostics
}

func (l staticWorkspaceLoader) ProjectPath() string {
	return l.projectPath
}

func assertContains(t *testing.T, value, want string) {
	t.Helper()
	if !strings.Contains(value, want) {
		t.Fatalf("expected content to contain %q\ncontent:\n%s", want, value)
	}
}

func assertNotContains(t *testing.T, value, unwanted string) {
	t.Helper()
	if strings.Contains(value, unwanted) {
		t.Fatalf("expected content not to contain %q\ncontent:\n%s", unwanted, value)
	}
}

func hasTaskJugglerTaskMapping(mapping app.TaskJugglerExportMapping, replanID, taskJugglerID string) bool {
	for _, entry := range mapping.TaskIDMap {
		if entry.RePlanTaskID == replanID && entry.TaskJugglerTaskID == taskJugglerID {
			return true
		}
	}
	return false
}

func hasTaskJugglerResourceMapping(mapping app.TaskJugglerExportMapping, replanID, taskJugglerID string) bool {
	for _, entry := range mapping.ResourceIDMap {
		if entry.RePlanResourceID == replanID && entry.TaskJugglerResourceID == taskJugglerID {
			return true
		}
	}
	return false
}

func hasDiagnosticCode(diags []app.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func hasDiagnosticTarget(diags []app.Diagnostic, code, kind, id string) bool {
	for _, diag := range diags {
		if diag.Code == code && diag.Target.Kind == kind && diag.Target.ID == id {
			return true
		}
	}
	return false
}

func TestExportTaskJugglerSkipsDemandWithExplicitZeroRemaining(t *testing.T) {
	service := app.NewExportService(staticWorkspaceLoader{
		projectPath: "test-project",
		workspace: domain.Workspace{
			Project:   domain.Project{ID: "project.test", Name: "Test project", Timezone: "UTC", DefaultStatusDate: "2026-07-06"},
			Resources: []domain.Resource{{ID: "resource.human.ivan", Class: "human", Name: "Ivan"}},
			Tasks:     []domain.Task{{ID: "task.op.1234", Title: "Finished work"}},
			ResourceDemands: []domain.ResourceDemand{{
				ID:              "resource_demand.task-op-1234.implementer",
				TaskRef:         domain.Ref{Kind: "task", ID: "task.op.1234"},
				ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
				ResourceClass:   "human",
				Role:            "implementer",
				Unit:            "hours",
				PlannedAmount:   8,
				RemainingAmount: domain.Amount(0),
			}},
		},
	})
	result, err := service.TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{Now: time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v", err)
	}
	if strings.Contains(result.Content, "effort 8.00h") || strings.Contains(result.Content, "allocate resource_human_ivan") {
		t.Fatalf("a demand with nothing left was scheduled:\n%s", result.Content)
	}
}
