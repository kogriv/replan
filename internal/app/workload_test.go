package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
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

func TestWorkloadDemandStatusLoadRule(t *testing.T) {
	statuses := []string{"", "draft", "active", "planned", "requested", "reserved", "partially_consumed", "consumed", "released", "cancelled", "blocked", "superseded"}
	for _, status := range statuses {
		status := status
		name := status
		if name == "" {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			ws := domain.Workspace{
				Resources: []domain.Resource{{ID: "resource.test", Class: "human"}},
				ResourceDemands: []domain.ResourceDemand{{
					ID: "demand.test", ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "hours", Status: status,
					PlannedAmount: 6, RemainingAmount: domain.Amount(5), ReservedAmount: 4, ActualAmount: 3,
				}},
				Capacities: []domain.Capacity{{ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "hours", Amount: 10, Period: "week"}},
			}
			result, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), app.WorkloadRequest{WithDemands: true})
			if err != nil {
				t.Fatal(err)
			}
			if len(result.Resources) != 1 || len(result.Resources[0].Demands) != 1 {
				t.Fatalf("result = %#v", result)
			}
			wantPlanned, wantRemaining, wantReserved := 6.0, 5.0, 4.0
			if !domain.DemandLoads(status) {
				wantPlanned, wantRemaining, wantReserved = 0, 0, 0
			}
			row := result.Resources[0]
			if row.Planned != wantPlanned || row.Remaining != wantRemaining || row.Reserved != wantReserved || row.Actual != 3 {
				t.Fatalf("row for status %q = %#v, want planned/remaining/reserved/actual %g/%g/%g/3", status, row, wantPlanned, wantRemaining, wantReserved)
			}
			share := row.Demands[0]
			if share.Status != status || share.Planned != wantPlanned || share.Remaining != wantRemaining || share.Reserved != wantReserved {
				t.Fatalf("demand share for status %q = %#v", status, share)
			}
		})
	}
}

func TestWorkloadSeparatesUnlevelledHoursAndKeepsActuals(t *testing.T) {
	ws := unlevelledLoadWorkspace()
	calibration := domain.Calibration{ID: "calibration.person", ApprovedAt: "2026-11-01T10:00:00Z", Scope: domain.CalibrationScope{Resources: []string{"resource.person"}, Unit: "hours"}, Result: domain.CalibrationResult{Factor: 1}}
	resealWorkloadCalibration(t, &calibration)
	ws.Calibrations = []domain.Calibration{calibration}
	result, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), app.WorkloadRequest{Week: "2026-W45", WithDemands: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources = %#v", result.Resources)
	}
	row := result.Resources[0]
	if row.Planned != 12 || row.Reserved != 12 || row.Remaining != 12 || row.Overload != 2 {
		t.Fatalf("levelled load = %#v", row)
	}
	if row.UnlevelledPlanned != 20 || row.UnlevelledRemaining != 15 || row.Actual != 3 {
		t.Fatalf("unlevelled/actual = %#v", row)
	}
	if row.RawPlanned == nil || *row.RawPlanned != 12 || row.RawRemaining == nil || *row.RawRemaining != 12 {
		t.Fatalf("unlevelled demand entered raw calibrated figures: %#v", row)
	}
	if len(row.Demands) != 2 {
		t.Fatalf("demand shares = %#v", row.Demands)
	}
	var unlevelled app.WorkloadDemandShare
	for _, demand := range row.Demands {
		if demand.ResourceDemandID == "demand.unlevelled" {
			unlevelled = demand
		}
	}
	if unlevelled.Levelled == nil || *unlevelled.Levelled || unlevelled.Planned != 20 || unlevelled.Remaining != 15 {
		t.Fatalf("unlevelled share = %#v", unlevelled)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(encoded), `"levelled":false`) != 1 || strings.Contains(string(encoded), `"levelled":true`) {
		t.Fatalf("levelled flags = %s", encoded)
	}
}

func unlevelledLoadWorkspace() domain.Workspace {
	levelled := false
	return domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-11-04"},
		Resources: []domain.Resource{{ID: "resource.person", Name: "Person", Class: "human"}},
		Tasks: []domain.Task{
			{ID: "task.levelled", Title: "Levelled", TrackingStatus: "ready", PlannedStart: "2026-11-02", PlannedFinish: "2026-11-06", ResourceDemands: []domain.Ref{{ID: "demand.levelled"}}, SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "1"}}},
			{ID: "task.unlevelled", Title: "Unlevelled", TrackingStatus: "ready", PlannedStart: "2026-11-02", PlannedFinish: "2026-11-06", ResourceDemands: []domain.Ref{{ID: "demand.unlevelled"}}, SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "2"}}},
		},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.levelled", TaskRef: domain.Ref{ID: "task.levelled"}, ResourceRef: domain.Ref{ID: "resource.person"}, ResourceClass: "human", Unit: "hours", Status: "active", PlannedAmount: 12, ReservedAmount: 12, RemainingAmount: domain.Amount(12)},
			{ID: "demand.unlevelled", TaskRef: domain.Ref{ID: "task.unlevelled"}, ResourceRef: domain.Ref{ID: "resource.person"}, ResourceClass: "human", Unit: "hours", Status: "active", PlannedAmount: 20, RemainingAmount: domain.Amount(15), ActualAmount: 3, Levelled: &levelled},
		},
		Capacities: []domain.Capacity{{ResourceRef: domain.Ref{ID: "resource.person"}, Unit: "hours", Amount: 10, Period: "week"}},
	}
}

func TestWorkloadUsesApprovedCalibrationSelection(t *testing.T) {
	low, middle, high := .5, .25, .2
	calibration := sealedWorkloadCalibration(t, "calibration.agent", "2026-11-01T10:00:00Z", "", high, &low, &middle, &high)
	ws := calibratedWorkloadWorkspace(calibration)

	result, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), app.WorkloadRequest{WithDemands: true})
	if err != nil {
		t.Fatal(err)
	}
	row := result.Resources[0]
	if row.Planned != 1.25 || row.Remaining != 1 || row.Reserved != .75 || row.Overload != 0 {
		t.Fatalf("calibrated row = %#v", row)
	}
	if !reflect.DeepEqual(row.CalibrationIDs, []string{"calibration.agent"}) || row.RawPlanned == nil || *row.RawPlanned != 6 || row.RawRemaining == nil || *row.RawRemaining != 5 {
		t.Fatalf("calibration metadata = %#v", row)
	}
	if len(row.Demands) != 1 || row.Demands[0].RawPlanned == nil || *row.Demands[0].RawPlanned != 6 || row.Demands[0].Planned != 1.25 {
		t.Fatalf("calibrated demand = %#v", row.Demands)
	}

	raw, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), app.WorkloadRequest{Raw: true})
	if err != nil {
		t.Fatal(err)
	}
	rawRow := raw.Resources[0]
	if rawRow.Planned != 6 || rawRow.Remaining != 5 || rawRow.Reserved != 4 || rawRow.Overload != 2 || len(rawRow.CalibrationIDs) != 0 || rawRow.RawPlanned != nil {
		t.Fatalf("raw row = %#v", rawRow)
	}
}

func TestPriorCalibrationWritesReloadsAndScalesLoad(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	factor := .21
	result, err := app.NewCalibrationService(store).Calibrate(context.Background(), app.CalibrationCalibrateRequest{
		Prior: &factor, PriorSource: "replan-self, 39 tasks, 2026-11-02",
		ResourceIDs: []string{"resource.ai.codex.local"}, Unit: "agent_hours",
		ID: "calibration.prior", Name: "Starting factor", ApprovedBy: "actor.human.operator",
		OccurredAt: "2026-11-04T10:00:00Z",
	})
	if err != nil || !result.Applied || result.Calibration.Result.Method != "prior" || result.Calibration.Result.Count != 0 || len(result.Calibration.Sample.Tasks) != 0 || len(result.Calibration.SourceBaselines) != 0 {
		t.Fatalf("calibrate err=%v result=%#v", err, result)
	}
	workspace, diagnostics := store.Load(context.Background())
	if len(diagnostics) != 0 || len(workspace.Calibrations) != 1 {
		t.Fatalf("reload diagnostics=%#v calibrations=%#v", diagnostics, workspace.Calibrations)
	}
	loaded := workspace.Calibrations[0]
	digest, err := domain.CalibrationDigest(loaded)
	if err != nil || digest != loaded.SealSHA256 {
		t.Fatalf("seal digest=%q want=%q err=%v", digest, loaded.SealSHA256, err)
	}
	load, err := app.NewWorkloadService(store).Calculate(context.Background(), app.WorkloadRequest{Week: "2026-W28"})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range load.Resources {
		if row.ResourceID == "resource.ai.codex.local" && (row.Planned != .5 || !reflect.DeepEqual(row.CalibrationIDs, []string{"calibration.prior"})) {
			t.Fatalf("prior-calibrated load=%#v", row)
		}
	}
}

func TestWorkloadCalibrationSelectionExclusionsAndLatest(t *testing.T) {
	tests := []struct {
		name   string
		change func(*domain.Workspace)
		want   float64
		wantID string
	}{
		{name: "tag scope skips untagged task", change: func(ws *domain.Workspace) {
			ws.Calibrations[0].Scope.Tag = "agent"
			resealWorkloadCalibration(t, &ws.Calibrations[0])
		}, want: 6},
		{name: "applied scenario disables calibration", change: func(ws *domain.Workspace) {
			ws.Scenarios = []domain.Scenario{{ID: "scenario.applied", Status: "applied", CalibrationRef: domain.Ref{ID: "calibration.agent"}}}
		}, want: 6},
		{name: "broken seal is ignored", change: func(ws *domain.Workspace) { ws.Calibrations[0].Result.Factor = .9 }, want: 6},
		{name: "latest calibration wins", change: func(ws *domain.Workspace) {
			latest := sealedWorkloadCalibration(t, "calibration.latest", "2026-11-02T10:00:00Z", "", .25, nil, nil, nil)
			ws.Calibrations = append(ws.Calibrations, latest)
		}, want: 1.5, wantID: "calibration.latest"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ws := calibratedWorkloadWorkspace(sealedWorkloadCalibration(t, "calibration.agent", "2026-11-01T10:00:00Z", "", .5, nil, nil, nil))
			tt.change(&ws)
			result, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), app.WorkloadRequest{})
			if err != nil {
				t.Fatal(err)
			}
			row := result.Resources[0]
			if row.Planned != tt.want {
				t.Fatalf("planned = %g, want %g; row %#v", row.Planned, tt.want, row)
			}
			if tt.wantID != "" && !reflect.DeepEqual(row.CalibrationIDs, []string{tt.wantID}) {
				t.Fatalf("calibration ids = %#v, want %s", row.CalibrationIDs, tt.wantID)
			}
		})
	}
}

func TestConsumedDemandIsNotSelectedForCalibration(t *testing.T) {
	ws := calibratedWorkloadWorkspace(sealedWorkloadCalibration(t, "calibration.agent", "2026-11-01T10:00:00Z", "", .2, nil, nil, nil))
	ws.ResourceDemands[0].Status = "consumed"
	if selected := app.ApplicableDemandCalibrations(ws); len(selected) != 0 {
		t.Fatalf("consumed demand selected: %#v", selected)
	}
}

func calibratedWorkloadWorkspace(calibration domain.Calibration) domain.Workspace {
	remaining := 5.0
	return domain.Workspace{
		Resources:    []domain.Resource{{ID: "resource.agent", Class: "ai_agent", Name: "Agent"}},
		Project:      domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-11-04"},
		Tasks:        []domain.Task{{ID: "task.agent", PlannedStart: "2026-11-02", PlannedFinish: "2026-11-06", ResourceDemands: []domain.Ref{{ID: "demand.agent"}}}},
		Calibrations: []domain.Calibration{calibration},
		ResourceDemands: []domain.ResourceDemand{{
			ID: "demand.agent", TaskRef: domain.Ref{ID: "task.agent"}, ResourceRef: domain.Ref{ID: "resource.agent"},
			Unit: "agent_hours", Status: "active", PlannedAmount: 6, RemainingAmount: &remaining, ReservedAmount: 4,
		}},
		Capacities: []domain.Capacity{{ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "agent_hours", Amount: 2, Period: "week"}},
	}
}

func sealedWorkloadCalibration(t *testing.T, id, approvedAt, tag string, factor float64, low, middle, high *float64) domain.Calibration {
	t.Helper()
	calibration := domain.Calibration{
		ID: id, ApprovedAt: approvedAt, Scope: domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "agent_hours", Tag: tag},
		Result: domain.CalibrationResult{Factor: factor},
	}
	if low != nil {
		two, four := 2.0, 4.0
		calibration.Result.Method = "size_bands"
		calibration.Result.SizeBands = []domain.CalibrationSizeBand{{UpTo: &two, Factor: low}, {UpTo: &four, Factor: middle}, {Factor: high}}
	}
	resealWorkloadCalibration(t, &calibration)
	return calibration
}

func resealWorkloadCalibration(t *testing.T, calibration *domain.Calibration) {
	t.Helper()
	calibration.SealSHA256 = ""
	if err := domain.SealCalibration(calibration); err != nil {
		t.Fatal(err)
	}
}

func TestWorkloadClosedDemandDoesNotAffectConcurrentPeak(t *testing.T) {
	ws := domain.Workspace{
		Resources: []domain.Resource{{ID: "resource.environment", Class: "environment"}},
		Tasks:     []domain.Task{{ID: "task.old", PlannedStart: "2026-09-28", PlannedFinish: "2026-09-28"}, {ID: "task.new", PlannedStart: "2026-09-28", PlannedFinish: "2026-09-28"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.old", TaskRef: domain.Ref{ID: "task.old"}, ResourceRef: domain.Ref{ID: "resource.environment"}, Unit: "concurrent_slots", Status: "superseded", PlannedAmount: 2, RemainingAmount: domain.Amount(2), ReservedAmount: 2},
			{ID: "demand.new", TaskRef: domain.Ref{ID: "task.new"}, ResourceRef: domain.Ref{ID: "resource.environment"}, Unit: "concurrent_slots", Status: "active", PlannedAmount: 1, RemainingAmount: domain.Amount(1), ReservedAmount: 1},
		},
		Capacities: []domain.Capacity{{ResourceRef: domain.Ref{ID: "resource.environment"}, Type: "concurrent", Unit: "concurrent_slots", Amount: 1, Period: "instant"}},
	}
	result, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), app.WorkloadRequest{Week: "2026-W40", WithDemands: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Resources) != 1 {
		t.Fatalf("resources = %#v", result.Resources)
	}
	row := result.Resources[0]
	if row.Planned != 1 || row.Remaining != 1 || row.Reserved != 1 || row.Overload != 0 {
		t.Fatalf("concurrent row = %#v", row)
	}
	if len(row.Demands) != 2 || row.Demands[0].ResourceDemandID != "demand.new" || row.Demands[1].ResourceDemandID != "demand.old" || row.Demands[1].Planned != 0 || row.Demands[1].Remaining != 0 || row.Demands[1].Reserved != 0 {
		t.Fatalf("concurrent demand shares = %#v", row.Demands)
	}
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
