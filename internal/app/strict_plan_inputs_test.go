package app

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func strictInputWorkspace() domain.Workspace {
	resource := domain.Ref{Kind: "resource", ID: "resource.test"}
	actor := domain.Ref{Kind: "actor", ID: "actor.test"}
	task := domain.Ref{Kind: "task", ID: "task.test"}
	demand := domain.Ref{Kind: "resource_demand", ID: "demand.test"}
	return domain.Workspace{
		Project:         domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-11-24"},
		Actors:          []domain.Actor{{ID: actor.ID, Kind: "human", TrustLevel: "high"}},
		Resources:       []domain.Resource{{ID: resource.ID, Name: "Test resource", Class: "human", ActorRef: actor}},
		Tasks:           []domain.Task{{ID: task.ID, Title: "Test task", TrackingStatus: "not_started", PlannedStart: "2026-11-23", PlannedFinish: "2026-11-27", ResourceDemands: []domain.Ref{demand}}},
		ResourceDemands: []domain.ResourceDemand{{ID: demand.ID, TaskRef: task, ResourceRef: resource, ResourceClass: "human", Unit: "hours", PlannedAmount: 4, RemainingAmount: domain.Amount(4)}},
		Capacities:      []domain.Capacity{{ID: "capacity.test", ResourceRef: resource, Type: "periodic_capacity", Unit: "hours", Period: "week", Amount: 10}},
		Availabilities: []domain.Availability{{ID: "availability.test", ResourceRef: resource,
			Windows:    []domain.AvailabilityWindow{{Days: []string{"monday", "tuesday", "wednesday", "thursday", "friday"}, StartTime: "00:00", EndTime: "24:00"}},
			Exceptions: []domain.AvailabilityException{{Date: "2026-11-30", Type: "maintenance"}},
		}},
		Actuals:     []domain.Actual{{ID: "actual.test", Type: "work_consumed", TaskRef: task, DemandRef: demand, ResourceRef: resource, Unit: "hours", Amount: 1, AcceptedBy: actor, OccurredAt: "2026-11-24T02:00:00Z", StatusDate: "2026-11-24"}},
		CurrentPlan: domain.CurrentPlan{ID: "current_plan.test", IncludedTasks: []domain.Ref{task}},
	}
}

type strictInputCase struct {
	name, code string
	change     func(*domain.Workspace)
}

func strictInputCases() []strictInputCase {
	return []strictInputCase{
		{"negative capacity", "invalid_capacity_amount", func(w *domain.Workspace) { w.Capacities[0].Amount = -10 }},
		{"NaN capacity", "invalid_capacity_amount", func(w *domain.Workspace) { w.Capacities[0].Amount = math.NaN() }},
		{"infinite capacity", "invalid_capacity_amount", func(w *domain.Workspace) { w.Capacities[0].Amount = math.Inf(1) }},
		{"daily period", "unsupported_capacity_period", func(w *domain.Workspace) { w.Capacities[0].Period = "day" }},
		{"unknown period", "unsupported_capacity_period", func(w *domain.Workspace) { w.Capacities[0].Period = "weeks" }},
		{"capacity date", "invalid_capacity_date", func(w *domain.Workspace) { w.Capacities[0].EffectiveFrom = "today" }},
		{"capacity range", "invalid_capacity_range", func(w *domain.Workspace) {
			w.Capacities[0].EffectiveFrom, w.Capacities[0].EffectiveTo = "2026-11-25", "2026-11-24"
		}},
		{"record timezone", "invalid_plan_record_timezone", func(w *domain.Workspace) { w.Availabilities[0].Timezone = "Unknown/Zone" }},
		{"unknown weekday", "invalid_availability_weekday", func(w *domain.Workspace) { w.Availabilities[0].Windows[0].Days[0] = "munday" }},
		{"invalid clock", "invalid_availability_window_time", func(w *domain.Workspace) { w.Availabilities[0].Windows[0].EndTime = "25:61" }},
		{"missing clock", "invalid_availability_window_time", func(w *domain.Workspace) { w.Availabilities[0].Windows[0].EndTime = "" }},
		{"overnight range", "invalid_availability_window_time", func(w *domain.Workspace) {
			w.Availabilities[0].Windows[0].StartTime, w.Availabilities[0].Windows[0].EndTime = "20:00", "08:00"
		}},
		{"exception date", "invalid_availability_exception_date", func(w *domain.Workspace) { w.Availabilities[0].Exceptions[0].Date = "2026-11-31" }},
		{"exception type", "invalid_availability_exception_type", func(w *domain.Workspace) { w.Availabilities[0].Exceptions[0].Type = "absent" }},
		{"project timezone", "invalid_project_timezone", func(w *domain.Workspace) { w.Project.Timezone = "Unknown/Zone" }},
		{"process timezone", "invalid_project_timezone", func(w *domain.Workspace) { w.Project.Timezone = "Local" }},
		{"project date", "invalid_project_status_date", func(w *domain.Workspace) { w.Project.DefaultStatusDate = "today" }},
		{"task reversed dates", "invalid_task_planned_range", func(w *domain.Workspace) { w.Tasks[0].PlannedFinish = "2026-11-22" }},
		{"preferred start shadows valid earliest", "invalid_demand_preferred_start", func(w *domain.Workspace) {
			w.ResourceDemands[0].Timing = domain.Timing{EarliestStart: "2026-11-24", PreferredStart: "later"}
		}},
		{"bad preferred finish", "invalid_demand_preferred_finish", func(w *domain.Workspace) { w.ResourceDemands[0].Timing.PreferredFinish = "2026-11-31" }},
		{"preferred window reversed", "invalid_demand_window", func(w *domain.Workspace) {
			w.ResourceDemands[0].Timing = domain.Timing{PreferredStart: "2026-11-25", PreferredFinish: "2026-11-24"}
		}},
		{"mixed window reversed", "invalid_demand_window", func(w *domain.Workspace) { w.ResourceDemands[0].Timing.PreferredStart = "2026-11-28" }},
		{"unknown spread", "invalid_demand_spread", func(w *domain.Workspace) { w.ResourceDemands[0].Timing.Spread = "other" }},
		{"manual spread", "unsupported_demand_spread", func(w *domain.Workspace) { w.ResourceDemands[0].Timing.Spread = "manual" }},
		{"contiguous spread", "unsupported_demand_spread", func(w *domain.Workspace) { w.ResourceDemands[0].Timing.Spread = "contiguous" }},
		{"nonfinite reserved amount", "invalid_reserved_amount", func(w *domain.Workspace) { w.ResourceDemands[0].ReservedAmount = math.Inf(1) }},
		{"nonfinite aggregate actual", "invalid_demand_actual_amount", func(w *domain.Workspace) { w.ResourceDemands[0].ActualAmount = math.NaN() }},
		{"actual date", "invalid_actual_status_date", func(w *domain.Workspace) { w.Actuals[0].StatusDate = "July 6" }},
		{"actual timestamp", "invalid_actual_occurred_at", func(w *domain.Workspace) { w.Actuals[0].OccurredAt = "2026-11-24 invalid" }},
		{"nonfinite actual amount", "invalid_actual_amount", func(w *domain.Workspace) { w.Actuals[0].Amount = math.NaN() }},
		{"correction date", "invalid_status_date", func(w *domain.Workspace) {
			w.Actuals = append(w.Actuals, domain.Actual{ID: "actual.correction", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: "actual.test"}, CorrectedStatusDate: "today"})
		}},
	}
}

func TestStrictPlanInputsRefuseCalculation(t *testing.T) {
	ctx := context.Background()
	for _, tc := range strictInputCases() {
		t.Run(tc.name, func(t *testing.T) {
			ws := strictInputWorkspace()
			tc.change(&ws)
			loader := workspaceInMemory{ws: ws}
			validation, err := NewValidateService(loader).Validate(ctx, ValidateRequest{})
			if !errors.Is(err, ErrValidationFailed) || !hasDiagnosticCode(validation.Errors, tc.code) {
				t.Fatalf("validate: %v, %+v; expected %s", err, validation.Errors, tc.code)
			}
			workload, err := NewWorkloadService(loader).Calculate(ctx, WorkloadRequest{Week: "2026-W48"})
			if !errors.Is(err, ErrValidationFailed) || !hasDiagnosticCode(workload.Errors, tc.code) || len(workload.Resources) != 0 {
				t.Fatalf("workload: %v, %+v; expected %s and no numbers", err, workload, tc.code)
			}
			if _, err := json.Marshal(workload); err != nil {
				t.Fatalf("workload diagnostics must be serializable: %v", err)
			}
			overview, err := NewReportService(loader).Dashboard(ctx, DashboardReportRequest{Week: "2026-W48"})
			if !errors.Is(err, ErrValidationFailed) || len(overview.Workload) != 0 || len(overview.Gantt.Tasks) != 0 || len(overview.Errors) != 1 || overview.Errors[0].Code != tc.code {
				t.Fatalf("dashboard: %v, %+v; expected %s and no computed views", err, overview.Errors, tc.code)
			}
			export, err := NewExportService(loader).TaskJuggler(ctx, ExportTaskJugglerRequest{})
			if !errors.Is(err, ErrValidationFailed) || !hasDiagnosticCode(export.Errors, tc.code) || export.Content != "" {
				t.Fatalf("TaskJuggler: %v, %+v; expected %s and no export", err, export.Errors, tc.code)
			}
		})
	}
}

func TestStrictPlanInputCompatibleDefaults(t *testing.T) {
	ws := strictInputWorkspace()
	ws.Project.Timezone = ""
	ws.ResourceDemands[0].Timing = domain.Timing{Spread: "even", PreferredStart: "2026-11-24", PreferredFinish: "2026-11-24"}
	ws.Availabilities[0].Windows = append(ws.Availabilities[0].Windows, domain.AvailabilityWindow{Days: []string{"Saturday"}})
	ws.Actuals[0].OccurredAt = "2026-11-24"
	result, err := NewValidateService(workspaceInMemory{ws: ws}).Validate(context.Background(), ValidateRequest{})
	if err != nil || !hasDiagnosticCode(result.Warnings, "missing_project_timezone") {
		t.Fatalf("expected valid plan with a missing project timezone warning: %v, %+v", err, result)
	}
}

func TestWorkloadZeroCapacityVsUnknown(t *testing.T) {
	for _, tc := range []struct {
		name     string
		change   func(*domain.Workspace)
		warning  string
		overload float64
	}{
		{"explicit zero", func(w *domain.Workspace) { w.Capacities[0].Amount = 0 }, "", 4},
		{"absent", func(w *domain.Workspace) { w.Capacities = nil }, "missing_capacity", 0},
		{"out of effect", func(w *domain.Workspace) { w.Capacities[0].EffectiveFrom = "2026-11-30" }, "missing_capacity", 0},
		{"monthly only", func(w *domain.Workspace) { w.Capacities[0].Period = "month" }, "monthly_capacity_not_weekly", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := strictInputWorkspace()
			tc.change(&ws)
			got, err := NewWorkloadService(workspaceInMemory{ws: ws}).Calculate(context.Background(), WorkloadRequest{Week: "2026-W48"})
			if err != nil || len(got.Resources) != 1 || got.Resources[0].Capacity != 0 || got.Resources[0].Overload != tc.overload {
				t.Fatalf("workload: %v, %+v", err, got)
			}
			if tc.warning == "" && len(got.Warnings) != 0 || tc.warning != "" && (len(got.Warnings) != 1 || got.Warnings[0].Code != tc.warning) {
				t.Fatalf("warnings: %+v; want %q", got.Warnings, tc.warning)
			}
		})
	}
}

func TestStrictPlanIds(t *testing.T) {
	ctx := context.Background()
	ws := strictInputWorkspace()
	ws.ResourceDemands[0].ID = "demand.line\nbreak"
	ws.Tasks[0].ResourceDemands[0].ID = ws.ResourceDemands[0].ID
	result, err := NewExportService(workspaceInMemory{ws: ws}).TaskJuggler(ctx, ExportTaskJugglerRequest{})
	if !errors.Is(err, ErrValidationFailed) || !hasDiagnosticCode(result.Errors, "invalid_id") || result.Content != "" {
		t.Fatalf("TaskJuggler: %v, %+v", err, result.Errors)
	}
	if text := taskJugglerCommentText("path\r\nwith\u2028breaks"); invalidPlanID(text) {
		t.Fatalf("comment was not escaped: %q", text)
	}
	for _, id := range []string{".", "group/child", `group\child`, "name.", "name:copy", "NUL.test"} {
		t.Run(id, func(t *testing.T) {
			res, err := NewBaselineService(workspaceInMemory{ws: strictInputWorkspace()}).Create(ctx, BaselineCreateRequest{ID: id, Name: "Test", ApprovedBy: "actor.test", DryRun: true})
			if !errors.Is(err, ErrValidationFailed) || !hasDiagnosticCode(res.Errors, "invalid_baseline_id") || len(res.PlannedWrites) != 0 {
				t.Fatalf("baseline: %v, %+v", err, res)
			}
		})
	}
}

func TestStrictPlanWorkloadWeekAndUndatedActual(t *testing.T) {
	ctx := context.Background()
	ws := strictInputWorkspace()
	ws.Actuals[0].OccurredAt, ws.Actuals[0].StatusDate = "", ""
	service := NewWorkloadService(workspaceInMemory{ws: ws})
	for _, tc := range []struct {
		week   string
		actual float64
	}{
		{"2026-W48", 0}, {"2026-W53", 0}, {"", 1},
	} {
		got, err := service.Calculate(ctx, WorkloadRequest{Week: tc.week})
		if err != nil || len(got.Resources) != 1 || got.Resources[0].Actual != tc.actual || hasDiagnosticCode(got.Warnings, "undated_actual") != (tc.week != "") {
			t.Fatalf("%q: workload %v, %+v", tc.week, err, got)
		}
	}
	got, err := service.Calculate(ctx, WorkloadRequest{Week: "2025-W53"})
	if !errors.Is(err, ErrValidationFailed) || !hasDiagnosticCode(got.Errors, "invalid_week") || len(got.Resources) != 0 {
		t.Fatalf("invalid week: workload %v, %+v", err, got)
	}
}
