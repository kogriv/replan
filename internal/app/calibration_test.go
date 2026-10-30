package app

import (
	"context"
	"fmt"
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

type calibrationTestLoader struct{ ws domain.Workspace }

func (l calibrationTestLoader) Load(context.Context) (domain.Workspace, []Diagnostic) {
	return l.ws, nil
}
func (l calibrationTestLoader) ProjectPath() string { return "fixture" }

func TestCalibrationObservationExclusionReasons(t *testing.T) {
	baseline := domain.Baseline{ID: "baseline.test", ApprovedAt: "2026-01-02T09:00:00Z"}
	statuses := map[string]string{
		"explicit": "accepted", "unfinished": "in_progress", "tag": "accepted", "unknown": "accepted",
		"late": "accepted", "zero": "accepted", "noactual": "accepted", "clean": "accepted",
	}
	ws := domain.Workspace{}
	for id, status := range statuses {
		taskID, demandID := "task."+id, "demand."+id
		ws.Tasks = append(ws.Tasks, domain.Task{ID: taskID, TrackingStatus: status, Tags: []string{"wanted"}})
		ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: demandID, TaskRef: domain.Ref{ID: taskID}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours"})
		amount := 2.0
		if id == "zero" {
			amount = 0
		}
		baseline.IncludedTasks = append(baseline.IncludedTasks, domain.BaselineTaskEntry{Ref: domain.Ref{ID: taskID}})
		baseline.IncludedDemands = append(baseline.IncludedDemands, domain.BaselineDemandEntry{Ref: domain.Ref{ID: demandID}, Task: taskID, Resource: "resource.agent", Unit: "hours", PlannedAmount: &amount})
		if id != "unknown" && id != "noactual" {
			date := "2026-01-03"
			if id == "late" {
				date = "2026-01-01"
			}
			ws.Actuals = append(ws.Actuals, domain.Actual{ID: "actual." + id, Type: "work_consumed", TaskRef: domain.Ref{ID: taskID}, DemandRef: domain.Ref{ID: demandID}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", Amount: 1, StatusDate: date})
		}
	}
	for i := range ws.Tasks {
		if ws.Tasks[i].ID == "task.tag" {
			ws.Tasks[i].Tags = []string{"other"}
		}
	}
	ws.AuditRecords = append(ws.AuditRecords, domain.AuditRecord{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.noactual"}, NewStatus: "in_progress", StatusDate: "2026-01-03"})
	observations, excluded := calibrationObservations(ws, []domain.Baseline{baseline}, domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours", Tag: "wanted"}, map[string]string{"task.explicit": "actuals_judged"})
	if len(observations) != 1 || observations[0].taskID != "task.clean" {
		t.Fatalf("observations=%#v", observations)
	}
	got := map[string]string{}
	for _, exclusion := range excluded {
		got[exclusion.Task] = exclusion.Reason
	}
	want := map[string]string{"task.explicit": "actuals_judged", "task.unfinished": "not_finished", "task.tag": "tag_mismatch", "task.unknown": "actual_start_unknown", "task.late": "estimated_after_start", "task.zero": "baseline_amount_not_positive", "task.noactual": "no_actuals"}
	for taskID, reason := range want {
		if got[taskID] != reason {
			t.Errorf("%s reason=%q want %q; all=%v", taskID, got[taskID], reason, got)
		}
	}
}

func TestCalibrationObservationUsesSameDayActualTimestamp(t *testing.T) {
	amount := 2.0
	baseline := domain.Baseline{
		ID: "baseline.same-day", ApprovedAt: "2026-10-15T12:00:00+07:00",
		IncludedTasks: []domain.BaselineTaskEntry{{Ref: domain.Ref{ID: "task.before"}}, {Ref: domain.Ref{ID: "task.after"}}},
		IncludedDemands: []domain.BaselineDemandEntry{
			{Ref: domain.Ref{ID: "demand.before"}, Task: "task.before", Resource: "resource.agent", Unit: "hours", PlannedAmount: &amount},
			{Ref: domain.Ref{ID: "demand.after"}, Task: "task.after", Resource: "resource.agent", Unit: "hours", PlannedAmount: &amount},
		},
	}
	ws := domain.Workspace{
		Tasks: []domain.Task{{ID: "task.before", TrackingStatus: "accepted"}, {ID: "task.after", TrackingStatus: "accepted"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.before", TaskRef: domain.Ref{ID: "task.before"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours"},
			{ID: "demand.after", TaskRef: domain.Ref{ID: "task.after"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours"},
		},
		Actuals: []domain.Actual{
			{ID: "actual.before", Type: "work_consumed", TaskRef: domain.Ref{ID: "task.before"}, DemandRef: domain.Ref{ID: "demand.before"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", Amount: 1, StatusDate: "2026-10-15", OccurredAt: "2026-10-15T13:20:00+07:00"},
			{ID: "actual.after", Type: "work_consumed", TaskRef: domain.Ref{ID: "task.after"}, DemandRef: domain.Ref{ID: "demand.after"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", Amount: 1, StatusDate: "2026-10-15", OccurredAt: "2026-10-15T11:20:00+07:00"},
		},
	}
	observations, excluded := calibrationObservations(ws, []domain.Baseline{baseline}, domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours"}, nil)
	if len(observations) != 1 || observations[0].taskID != "task.before" {
		t.Fatalf("observations=%#v", observations)
	}
	if len(excluded) != 1 || excluded[0].Task != "task.after" || excluded[0].Reason != "estimated_after_start" {
		t.Fatalf("excluded=%#v", excluded)
	}
}

func TestCalibrationUsesCorrectedActualAmount(t *testing.T) {
	baselineAmount, corrected := 2.0, 0.1
	baseline := domain.Baseline{ID: "baseline.corrected", ApprovedAt: "2026-01-01T09:00:00Z", IncludedTasks: []domain.BaselineTaskEntry{{Ref: domain.Ref{ID: "task.corrected"}}}, IncludedDemands: []domain.BaselineDemandEntry{{Ref: domain.Ref{ID: "demand.corrected"}, Task: "task.corrected", Resource: "resource.agent", Unit: "hours", PlannedAmount: &baselineAmount}}}
	ws := domain.Workspace{Tasks: []domain.Task{{ID: "task.corrected", TrackingStatus: "accepted"}}, ResourceDemands: []domain.ResourceDemand{{ID: "demand.corrected", TaskRef: domain.Ref{ID: "task.corrected"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours"}}, Actuals: []domain.Actual{
		{ID: "actual.corrected", Type: "work_consumed", TaskRef: domain.Ref{ID: "task.corrected"}, DemandRef: domain.Ref{ID: "demand.corrected"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", Amount: .33, StatusDate: "2026-01-02"},
		{ID: "correction.corrected", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: "actual.corrected"}, CorrectedAmount: &corrected, DemandRef: domain.Ref{ID: "demand.corrected"}, Unit: "hours", OccurredAt: "2026-01-03T12:00:00Z"},
	}}
	observations, excluded := calibrationObservations(ws, []domain.Baseline{baseline}, domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours"}, nil)
	if len(excluded) != 0 || len(observations) != 1 || observations[0].actual != corrected {
		t.Fatalf("observations=%#v excluded=%#v", observations, excluded)
	}
}

func TestCalibrationStatisticsBandsAndRounding(t *testing.T) {
	observations := []calibrationObservation{
		{baseline: 1, actual: 1, ratio: 1},
		{baseline: 2, actual: 4, ratio: 2},
		{baseline: 3, actual: 9, ratio: 3},
		{baseline: 5, actual: 20, ratio: 4},
	}
	result := summarizeCalibration(observations)
	if result.Factor != 2.5 || result.P25 != 1.75 || result.P75 != 3.25 || result.TotalRatio != 3.090909 {
		t.Fatalf("result=%#v", result)
	}
	if len(result.SizeBands) != 3 || result.SizeBands[0].Count != 2 || result.SizeBands[1].Median != 3 || result.SizeBands[2].Median != 4 {
		t.Fatalf("bands=%#v", result.SizeBands)
	}
	for input, want := range map[float64]float64{0: .25, .1: .25, .24: .25, .37: .25, .38: .5, 1.12: 1, 1.13: 1.25} {
		if got := RoundCalibrationAmount(input); got != want {
			t.Errorf("RoundCalibrationAmount(%g)=%g want %g", input, got, want)
		}
	}
}

func TestCalibrationSplitGroupCountsAddedResourceActuals(t *testing.T) {
	amount := 4.0
	baseline := domain.Baseline{ID: "baseline.test", ApprovedAt: "2026-01-01T00:00:00Z", IncludedTasks: []domain.BaselineTaskEntry{{Ref: domain.Ref{ID: "task.test"}}}, IncludedDemands: []domain.BaselineDemandEntry{{Ref: domain.Ref{ID: "demand.original"}, Task: "task.test", Resource: "resource.executor", Unit: "hours", PlannedAmount: &amount}}}
	ws := domain.Workspace{
		Tasks: []domain.Task{{ID: "task.test", TrackingStatus: "accepted"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.original", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours"},
			{ID: "demand.split", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.codex"}, Unit: "hours"},
		},
		Actuals: []domain.Actual{{Type: "work_consumed", TaskRef: domain.Ref{ID: "task.test"}, DemandRef: domain.Ref{ID: "demand.split"}, ResourceRef: domain.Ref{ID: "resource.codex"}, Unit: "hours", Amount: 1, StatusDate: "2026-01-02"}},
	}
	observations, excluded := calibrationObservations(ws, []domain.Baseline{baseline}, domain.CalibrationScope{Resources: []string{"resource.executor", "resource.codex"}, Unit: "hours"}, nil)
	if len(excluded) != 0 || len(observations) != 1 || observations[0].ratio != .25 {
		t.Fatalf("observations=%#v excluded=%#v", observations, excluded)
	}
}

func TestCalibrationApproverMustBeHuman(t *testing.T) {
	service := NewCalibrationService(calibrationTestLoader{ws: domain.Workspace{Actors: []domain.Actor{{ID: "actor.agent", Kind: "ai_agent"}}}})
	result, err := service.Calibrate(context.Background(), CalibrationCalibrateRequest{ID: "calibration.test", Name: "Test", ApprovedBy: "actor.agent"})
	if err == nil || !hasDiagnosticCode(result.Errors, "approver_not_human") {
		t.Fatalf("err=%v errors=%#v", err, result.Errors)
	}
}

func TestCalibrationApplySkipsAndBuildsScenario(t *testing.T) {
	calibration := domain.Calibration{ID: "calibration.test", Name: "Test", Scope: domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours"}, Result: domain.CalibrationResult{Factor: .5}}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	four := 4.0
	ws := domain.Workspace{
		CurrentPlan: domain.CurrentPlan{ID: "current"}, Actors: []domain.Actor{{ID: "actor.operator", Kind: "human"}}, Calibrations: []domain.Calibration{calibration},
		Tasks: []domain.Task{{ID: "task.open"}, {ID: "task.closed"}, {ID: "task.overrun"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.open", TaskRef: domain.Ref{ID: "task.open"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: 4, RemainingAmount: &four, Status: "active"},
			{ID: "demand.closed", TaskRef: domain.Ref{ID: "task.closed"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: 4, RemainingAmount: &zero, Status: "consumed"},
			{ID: "demand.overrun", TaskRef: domain.Ref{ID: "task.overrun"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: 4, RemainingAmount: &four, Status: "active"},
		},
		Actuals: []domain.Actual{{Type: "work_consumed", DemandRef: domain.Ref{ID: "demand.overrun"}, Unit: "hours", Amount: 3}},
	}
	result, err := NewCalibrationService(calibrationTestLoader{ws: ws}).Apply(context.Background(), CalibrationApplyRequest{CalibrationID: calibration.ID, ScenarioID: "scenario.calibrated", ActorID: "actor.operator", DryRun: true, OccurredAt: "2026-01-02T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Scenario.CalibrationRef.ID != calibration.ID || len(result.Scenario.Changes) != 2 {
		t.Fatalf("scenario=%#v", result.Scenario)
	}
	if result.Scenario.Changes[0].Value != float64(2) || result.Scenario.Changes[1].Value != float64(2) {
		t.Fatalf("changes=%#v", result.Scenario.Changes)
	}
	got := map[string]string{}
	for _, skip := range result.Skipped {
		got[skip.DemandID] = skip.Reason
	}
	if got["demand.closed"] != "closed" || got["demand.overrun"] != "actuals_above_scaled_amount" {
		t.Fatalf("skips=%v", got)
	}
}

func TestTaskAddReportsCalibrationWithoutChangingTypedAmount(t *testing.T) {
	calibration := domain.Calibration{ID: "calibration.test", ApprovedAt: "2026-01-01T00:00:00Z", Scope: domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours", Tag: "calibrated"}, Result: domain.CalibrationResult{Factor: .23}}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	ws := domain.Workspace{
		Actors: []domain.Actor{{ID: "actor.operator", Kind: "human"}}, Resources: []domain.Resource{{ID: "resource.agent", Class: "ai_agent"}},
		Capacities: []domain.Capacity{{ID: "capacity.agent", ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours"}}, Evidence: []domain.Evidence{{ID: "evidence.test"}},
		Calibrations: []domain.Calibration{calibration}, CurrentPlan: domain.CurrentPlan{ID: "current", IncludedTasks: []domain.Ref{{ID: "task.existing"}}}, Tasks: []domain.Task{{ID: "task.existing"}},
	}
	result, err := NewTaskService(calibrationTestLoader{ws: ws}).Add(context.Background(), TaskAddRequest{ID: "task.new", Title: "New", Tags: []string{"calibrated"}, DemandSpecs: []string{"agent=resource.agent:4:hours"}, ActorID: "actor.operator", EvidenceID: "evidence.test", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.CalibratedValues) != 1 || result.Demands[0].PlannedAmount != 4 || result.CalibratedValues[0].Calibrated != 1 {
		t.Fatalf("result=%#v", result)
	}
}

func TestCalibrationSuggestionsUseMaximalScopeAndUnusedTasks(t *testing.T) {
	scopes := map[calibrationSuggestionKey][]string{
		{baseline: "baseline.test", unit: "hours", resources: "resource.executor"}:                   {"resource.executor"},
		{baseline: "baseline.test", unit: "hours", resources: "resource.codex\x00resource.executor"}: {"resource.codex", "resource.executor"},
	}
	memberKey := calibrationSuggestionKey{baseline: "baseline.test", unit: "hours", resources: "resource.executor"}
	if !calibrationScopeHasStrictSuperset(memberKey, scopes[memberKey], scopes) {
		t.Fatal("member scope was not suppressed by its split group")
	}

	amount := 2.0
	baseline := domain.Baseline{ID: "baseline.test", ApprovedAt: "2026-01-01T00:00:00Z"}
	ws := domain.Workspace{}
	for i := 1; i <= 16; i++ {
		taskID := fmt.Sprintf("task.%02d", i)
		demandID := fmt.Sprintf("demand.%02d", i)
		ws.Tasks = append(ws.Tasks, domain.Task{ID: taskID, TrackingStatus: "accepted"})
		ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: demandID, TaskRef: domain.Ref{ID: taskID}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours"})
		ws.Actuals = append(ws.Actuals, domain.Actual{Type: "work_consumed", TaskRef: domain.Ref{ID: taskID}, DemandRef: domain.Ref{ID: demandID}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours", Amount: 1, StatusDate: "2026-01-02"})
		baseline.IncludedTasks = append(baseline.IncludedTasks, domain.BaselineTaskEntry{Ref: domain.Ref{ID: taskID}})
		baseline.IncludedDemands = append(baseline.IncludedDemands, domain.BaselineDemandEntry{Ref: domain.Ref{ID: demandID}, Task: taskID, Resource: "resource.executor", Unit: "hours", PlannedAmount: &amount})
	}
	calibration := domain.Calibration{ID: "calibration.group", Scope: domain.CalibrationScope{Resources: []string{"resource.executor", "resource.codex"}, Unit: "hours"}}
	for i := 1; i <= 8; i++ {
		calibration.Sample.Tasks = append(calibration.Sample.Tasks, fmt.Sprintf("task.%02d", i))
	}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	ws.Calibrations = []domain.Calibration{calibration}
	candidates := map[calibrationSuggestionKey]CalibrationSuggestion{}
	addCalibrationSuggestion(candidates, ws, baseline, []string{"resource.executor"}, "hours")
	for _, suggestion := range candidates {
		if suggestion.Count != 8 {
			t.Fatalf("suggestion=%#v", suggestion)
		}
	}
	if len(candidates) != 1 {
		t.Fatalf("candidates=%#v", candidates)
	}
}

func TestCalibrationOpenDemandGroupsAggregateAndHideUsedCalibration(t *testing.T) {
	calibration := domain.Calibration{ID: "calibration.test", Scope: domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours"}, Result: domain.CalibrationResult{Factor: .5}}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	ws := domain.Workspace{Calibrations: []domain.Calibration{calibration}}
	for i := 1; i <= 5; i++ {
		ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: fmt.Sprintf("demand.%d", i), ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: float64(i), Status: "active"})
	}
	groups := CalibrationOpenDemandGroups(ws)
	if len(groups) != 1 || len(groups[0].Demands) != 5 || groups[0].Demands[0].Calibrated != .5 {
		t.Fatalf("groups=%#v", groups)
	}
	ws.Scenarios = []domain.Scenario{{ID: "scenario.used", CalibrationRef: domain.Ref{ID: calibration.ID}}}
	if groups := CalibrationOpenDemandGroups(ws); len(groups) != 0 {
		t.Fatalf("groups after scenario=%#v", groups)
	}
}
