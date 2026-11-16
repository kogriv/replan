package app

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
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

func TestCalibrationReviewReadyOptInStillRequiresActuals(t *testing.T) {
	amount := 2.0
	baseline := domain.Baseline{ID: "baseline.review-ready", ApprovedAt: "2026-01-01T00:00:00Z"}
	ws := domain.Workspace{}
	for _, id := range []string{"with-actuals", "without-actuals"} {
		taskID, demandID := "task."+id, "demand."+id
		ws.Tasks = append(ws.Tasks, domain.Task{ID: taskID, TrackingStatus: "review_ready"})
		ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: demandID, TaskRef: domain.Ref{ID: taskID}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours"})
		baseline.IncludedTasks = append(baseline.IncludedTasks, domain.BaselineTaskEntry{Ref: domain.Ref{ID: taskID}})
		baseline.IncludedDemands = append(baseline.IncludedDemands, domain.BaselineDemandEntry{Ref: domain.Ref{ID: demandID}, Task: taskID, Resource: "resource.agent", Unit: "hours", PlannedAmount: &amount})
	}
	ws.Actuals = append(ws.Actuals, domain.Actual{ID: "actual.with-actuals", Type: "work_consumed", TaskRef: domain.Ref{ID: "task.with-actuals"}, DemandRef: domain.Ref{ID: "demand.with-actuals"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", Amount: 1, StatusDate: "2026-01-02"})
	ws.AuditRecords = append(ws.AuditRecords, domain.AuditRecord{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.without-actuals"}, NewStatus: "in_progress", StatusDate: "2026-01-02"})

	observations, excluded := calibrationObservations(ws, []domain.Baseline{baseline}, domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours"}, nil)
	if len(observations) != 0 || len(excluded) != 2 || excluded[0].Reason != "not_finished" || excluded[1].Reason != "not_finished" {
		t.Fatalf("default observations=%#v excluded=%#v", observations, excluded)
	}

	observations, excluded = calibrationObservations(ws, []domain.Baseline{baseline}, domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours", IncludeReviewReady: true}, nil)
	if len(observations) != 1 || observations[0].taskID != "task.with-actuals" || !observations[0].reviewReady {
		t.Fatalf("opt-in observations=%#v", observations)
	}
	if len(excluded) != 1 || excluded[0].Task != "task.without-actuals" || excluded[0].Reason != "no_actuals" {
		t.Fatalf("opt-in excluded=%#v", excluded)
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

func TestCalibrationSinceFiltersActualStartAndIncludesBoundary(t *testing.T) {
	amount := 2.0
	baseline := domain.Baseline{ID: "baseline.window", ApprovedAt: "2026-01-01T00:00:00Z"}
	ws := domain.Workspace{
		Project:   domain.Project{DefaultStatusDate: "2026-01-10"},
		Actors:    []domain.Actor{{ID: "actor.operator", Kind: "human"}},
		Resources: []domain.Resource{{ID: "resource.agent"}},
	}
	for i := 0; i < 9; i++ {
		taskID, demandID := fmt.Sprintf("task.window.%d", i), fmt.Sprintf("demand.window.%d", i)
		start := "2026-01-03"
		if i == 0 {
			start = "2026-01-02"
		}
		status := "accepted"
		if i == 1 {
			status = "review_ready"
		}
		ws.Tasks = append(ws.Tasks, domain.Task{ID: taskID, TrackingStatus: status})
		ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: demandID, TaskRef: domain.Ref{ID: taskID}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours"})
		ws.Actuals = append(ws.Actuals, domain.Actual{ID: "actual." + taskID, Type: "work_consumed", TaskRef: domain.Ref{ID: taskID}, DemandRef: domain.Ref{ID: demandID}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", Amount: 1, StatusDate: start})
		baseline.IncludedTasks = append(baseline.IncludedTasks, domain.BaselineTaskEntry{Ref: domain.Ref{ID: taskID}})
		baseline.IncludedDemands = append(baseline.IncludedDemands, domain.BaselineDemandEntry{Ref: domain.Ref{ID: demandID}, Task: taskID, Resource: "resource.agent", Unit: "hours", PlannedAmount: &amount})
	}
	if err := domain.SealBaseline(&baseline); err != nil {
		t.Fatal(err)
	}
	ws.Baselines = []domain.Baseline{baseline}
	result, err := NewCalibrationService(calibrationTestLoader{ws: ws}).Calibrate(context.Background(), CalibrationCalibrateRequest{
		BaselineIDs: []string{baseline.ID}, ResourceIDs: []string{"resource.agent"}, Unit: "hours", Since: "2026-01-03", IncludeReviewReady: true,
		ID: "calibration.window", Name: "Window", ApprovedBy: "actor.operator", DryRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Calibration.Scope.Since != "2026-01-03" || !result.Calibration.Scope.IncludeReviewReady || result.Calibration.Result.Count != 8 || result.Calibration.Result.ReviewReadyCount != 1 {
		t.Fatalf("calibration=%#v", result.Calibration)
	}
	if containsString(result.Calibration.Sample.Tasks, "task.window.0") || !containsString(result.Calibration.Sample.Tasks, "task.window.1") {
		t.Fatalf("boundary sample=%v", result.Calibration.Sample.Tasks)
	}
	if len(result.Calibration.Sample.Excluded) != 1 || result.Calibration.Sample.Excluded[0].Reason != "started_before_window" {
		t.Fatalf("excluded=%#v", result.Calibration.Sample.Excluded)
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
	result := summarizeCalibration(observations, false)
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

func TestCalibrationSizeBandFactorsAndFallback(t *testing.T) {
	observations := []calibrationObservation{}
	for i := 0; i < 8; i++ {
		observations = append(observations, calibrationObservation{baseline: 1, actual: .5, ratio: .5})
	}
	observations = append(observations, calibrationObservation{baseline: 3, actual: .6, ratio: .2})
	result := summarizeCalibration(observations, true)
	if result.Method != "size_bands" || len(result.SizeBands) != 3 {
		t.Fatalf("result=%#v", result)
	}
	if result.SizeBands[0].Factor == nil || *result.SizeBands[0].Factor != .5 || result.SizeBands[0].FactorSource != "band" {
		t.Fatalf("qualified band=%#v", result.SizeBands[0])
	}
	if result.SizeBands[1].Factor == nil || *result.SizeBands[1].Factor != result.Factor || result.SizeBands[1].FactorSource != "overall" {
		t.Fatalf("fallback band=%#v", result.SizeBands[1])
	}
	if result.SizeBands[2].Count != 0 || result.SizeBands[2].Factor == nil || result.SizeBands[2].FactorSource != "overall" {
		t.Fatalf("empty band=%#v", result.SizeBands[2])
	}
}

func TestCalibrationFactorUsesSplitTaskScopeAmount(t *testing.T) {
	low, high := .5, .2
	calibration := domain.Calibration{Scope: domain.CalibrationScope{Resources: []string{"resource.executor", "resource.codex"}, Unit: "hours"}, Result: domain.CalibrationResult{Method: "size_bands", Factor: .3, SizeBands: []domain.CalibrationSizeBand{{UpTo: floatPointer(2), Factor: &low}, {UpTo: floatPointer(4), Factor: &high}, {Factor: &high}}}}
	demands := []domain.ResourceDemand{
		{TaskRef: domain.Ref{ID: "task.split"}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours", PlannedAmount: 2},
		{TaskRef: domain.Ref{ID: "task.split"}, ResourceRef: domain.Ref{ID: "resource.codex"}, Unit: "hours", PlannedAmount: 1, Status: "consumed"},
	}
	amounts := calibrationTaskScopeAmounts(demands, calibration)
	factor, band := CalibrationFactor(calibration, amounts["task.split"])
	if amounts["task.split"] != 3 || factor != .2 || band != "up to 4" {
		t.Fatalf("amount=%g factor=%g band=%q", amounts["task.split"], factor, band)
	}
}

func TestSizeBandFactorUsedByApplyValuesAndNext(t *testing.T) {
	low, high := .5, .2
	calibration := domain.Calibration{ID: "calibration.bands", Name: "Bands", Scope: domain.CalibrationScope{Resources: []string{"resource.agent"}, Unit: "hours"}, Result: domain.CalibrationResult{Method: "size_bands", Factor: .3, SizeBands: []domain.CalibrationSizeBand{{UpTo: floatPointer(2), Factor: &low}, {UpTo: floatPointer(4), Factor: &high}, {Factor: &high}}}}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	four := 4.0
	ws := domain.Workspace{CurrentPlan: domain.CurrentPlan{ID: "current"}, Actors: []domain.Actor{{ID: "actor.operator", Kind: "human"}}, Tasks: []domain.Task{{ID: "task.open"}}, Calibrations: []domain.Calibration{calibration}, ResourceDemands: []domain.ResourceDemand{{ID: "demand.open", TaskRef: domain.Ref{ID: "task.open"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: 4, RemainingAmount: &four}}}
	values := CalibratedDemandValues(ws)
	groups := CalibrationOpenDemandGroups(ws)
	result, err := NewCalibrationService(calibrationTestLoader{ws: ws}).Apply(context.Background(), CalibrationApplyRequest{CalibrationID: calibration.ID, ScenarioID: "scenario.bands", ActorID: "actor.operator", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(values) != 1 || values[0].Factor != .2 || values[0].Band != "up to 4" || values[0].Calibrated != .75 {
		t.Fatalf("values=%#v", values)
	}
	if len(groups) != 1 || groups[0].Demands[0].Factor != .2 {
		t.Fatalf("groups=%#v", groups)
	}
	if len(result.Scenario.Changes) != 2 || result.Scenario.Changes[0].Value != .75 {
		t.Fatalf("scenario=%#v", result.Scenario)
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

func TestPriorCalibrationRefusals(t *testing.T) {
	valid := .21
	ws := domain.Workspace{
		Actors:    []domain.Actor{{ID: "actor.operator", Kind: "human"}},
		Resources: []domain.Resource{{ID: "resource.agent"}},
	}
	tests := []struct {
		name   string
		change func(*CalibrationCalibrateRequest)
		code   string
	}{
		{name: "zero factor", change: func(req *CalibrationCalibrateRequest) { zero := 0.0; req.Prior = &zero }, code: "calibration_prior_factor_invalid"},
		{name: "factor above ten", change: func(req *CalibrationCalibrateRequest) { high := 10.01; req.Prior = &high }, code: "calibration_prior_factor_invalid"},
		{name: "empty source", change: func(req *CalibrationCalibrateRequest) { req.PriorSource = "  " }, code: "calibration_prior_source_required"},
		{name: "baseline conflict", change: func(req *CalibrationCalibrateRequest) { req.BaselineIDs = []string{"baseline.test"} }, code: "calibration_prior_flags_conflict"},
		{name: "all baselines conflict", change: func(req *CalibrationCalibrateRequest) { req.AllBaselines = true }, code: "calibration_prior_flags_conflict"},
		{name: "by size conflict", change: func(req *CalibrationCalibrateRequest) { req.BySize = true }, code: "calibration_prior_flags_conflict"},
		{name: "exclude conflict", change: func(req *CalibrationCalibrateRequest) { req.Exclusions = map[string]string{"task.test": "judged"} }, code: "calibration_prior_flags_conflict"},
		{name: "since conflict", change: func(req *CalibrationCalibrateRequest) { req.Since = "2026-01-01" }, code: "calibration_prior_flags_conflict"},
		{name: "review-ready conflict", change: func(req *CalibrationCalibrateRequest) { req.IncludeReviewReady = true }, code: "calibration_prior_flags_conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := CalibrationCalibrateRequest{Prior: &valid, PriorSource: "other project", ResourceIDs: []string{"resource.agent"}, Unit: "hours", ID: "calibration.prior", Name: "Prior", ApprovedBy: "actor.operator", DryRun: true}
			test.change(&req)
			result, err := NewCalibrationService(calibrationTestLoader{ws: ws}).Calibrate(context.Background(), req)
			if err == nil || !hasDiagnosticCode(result.Errors, test.code) {
				t.Fatalf("err=%v errors=%#v, want %s", err, result.Errors, test.code)
			}
		})
	}
}

func TestCalibrationSinceRefusals(t *testing.T) {
	ws := limitedCalibrationWorkspace(t)
	ws.Project.DefaultStatusDate = "2026-01-10"
	for _, since := range []string{"not-a-date", "2026-01-11"} {
		result, err := NewCalibrationService(calibrationTestLoader{ws: ws}).Calibrate(context.Background(), CalibrationCalibrateRequest{
			AllBaselines: true, ResourceIDs: []string{"resource.executor"}, Unit: "hours", Since: since,
			ID: "calibration.bad-since", Name: "Bad since", ApprovedBy: "actor.operator", DryRun: true,
		})
		if err == nil || !hasDiagnosticCode(result.Errors, "invalid_since") {
			t.Fatalf("since=%q err=%v errors=%#v", since, err, result.Errors)
		}
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

func TestNextSuggestsWindowedSizeBandRecalibrationAtMinimum(t *testing.T) {
	amount := 2.0
	baseline := domain.Baseline{ID: "baseline.recalibrate", ApprovedAt: "2025-12-31T00:00:00Z"}
	ws := domain.Workspace{Project: domain.Project{DefaultStatusDate: "2026-01-10"}}
	calibration := domain.Calibration{
		ID: "calibration.previous", ApprovedAt: "2026-01-03T09:00:00Z",
		Scope:  domain.CalibrationScope{Resources: []string{"resource.executor"}, Unit: "hours"},
		Result: domain.CalibrationResult{Method: "size_bands"},
	}
	for i := 0; i < 16; i++ {
		taskID, demandID := fmt.Sprintf("task.recalibrate.%02d", i), fmt.Sprintf("demand.recalibrate.%02d", i)
		start := "2026-01-02"
		if i >= 8 {
			start = "2026-01-03"
		} else {
			calibration.Sample.Tasks = append(calibration.Sample.Tasks, taskID)
		}
		ws.Tasks = append(ws.Tasks, domain.Task{ID: taskID, TrackingStatus: "accepted"})
		ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: demandID, TaskRef: domain.Ref{ID: taskID}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours"})
		ws.Actuals = append(ws.Actuals, domain.Actual{ID: "actual." + taskID, Type: "work_consumed", TaskRef: domain.Ref{ID: taskID}, DemandRef: domain.Ref{ID: demandID}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours", Amount: 1, StatusDate: start})
		baseline.IncludedTasks = append(baseline.IncludedTasks, domain.BaselineTaskEntry{Ref: domain.Ref{ID: taskID}})
		baseline.IncludedDemands = append(baseline.IncludedDemands, domain.BaselineDemandEntry{Ref: domain.Ref{ID: demandID}, Task: taskID, Resource: "resource.executor", Unit: "hours", PlannedAmount: &amount})
	}
	calibration.Result.Count = len(calibration.Sample.Tasks)
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	if err := domain.SealBaseline(&baseline); err != nil {
		t.Fatal(err)
	}
	ws.Calibrations, ws.Baselines = []domain.Calibration{calibration}, []domain.Baseline{baseline}
	actions := dashboardNextActions(ws, reports.DashboardOverview{ProjectPath: "fixture"})
	var suggestion *reports.DashboardNextAction
	for i := range actions {
		if actions[i].Code == "suggest_calibration" {
			suggestion = &actions[i]
		}
	}
	if suggestion == nil {
		t.Fatalf("actions=%#v", actions)
	}
	if suggestion.Count != 8 || suggestion.Since != "2026-01-03" || suggestion.ReplacesCalibrationID != calibration.ID {
		t.Fatalf("suggestion=%#v", suggestion)
	}
	if !strings.Contains(suggestion.Command, "--since 2026-01-03") || !strings.Contains(suggestion.Command, "--by-size") {
		t.Fatalf("command=%q", suggestion.Command)
	}
	if !strings.Contains(suggestion.Label, "8 clean tasks started since 2026-01-03, not in calibration.previous") {
		t.Fatalf("label=%q", suggestion.Label)
	}
	brief := reports.BuildNextBrief(reports.DashboardOverview{NextActions: actions})
	var briefSuggestion *reports.NextBriefItem
	for i := range brief.AgentCanPrepare {
		if brief.AgentCanPrepare[i].Code == "suggest_calibration" {
			briefSuggestion = &brief.AgentCanPrepare[i]
		}
	}
	if briefSuggestion == nil || briefSuggestion.What != suggestion.Label || briefSuggestion.Since != suggestion.Since {
		t.Fatalf("brief=%#v", brief)
	}

	ws.Actuals = ws.Actuals[:len(ws.Actuals)-1]
	if actions := dashboardNextActions(ws, reports.DashboardOverview{ProjectPath: "fixture"}); countNextActions(actions, "suggest_calibration") != 0 {
		t.Fatalf("suggested below minimum: %#v", actions)
	}
}

func limitedCalibrationWorkspace(t *testing.T) domain.Workspace {
	t.Helper()
	ws := domain.Workspace{
		Actors:    []domain.Actor{{ID: "actor.operator", Kind: "human"}},
		Resources: []domain.Resource{{ID: "resource.executor"}},
	}
	amount := 2.0
	for group := 0; group < 2; group++ {
		baseline := domain.Baseline{ID: fmt.Sprintf("baseline.%d", group), Scope: "tasks", ApprovedAt: "2026-01-01T00:00:00Z", CreatedAt: "2026-01-01T00:00:00Z", ApprovedBy: domain.Ref{ID: "actor.operator"}}
		for item := 0; item < 4; item++ {
			index := group*4 + item
			taskID, demandID := fmt.Sprintf("task.%d", index), fmt.Sprintf("demand.%d", index)
			ws.Tasks = append(ws.Tasks, domain.Task{ID: taskID, TrackingStatus: "accepted"})
			ws.ResourceDemands = append(ws.ResourceDemands, domain.ResourceDemand{ID: demandID, TaskRef: domain.Ref{ID: taskID}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours"})
			ws.Actuals = append(ws.Actuals, domain.Actual{ID: "actual." + taskID, Type: "work_consumed", TaskRef: domain.Ref{ID: taskID}, DemandRef: domain.Ref{ID: demandID}, ResourceRef: domain.Ref{ID: "resource.executor"}, Unit: "hours", Amount: 1, StatusDate: "2026-01-02"})
			baseline.IncludedTasks = append(baseline.IncludedTasks, domain.BaselineTaskEntry{Ref: domain.Ref{ID: taskID}})
			baseline.IncludedDemands = append(baseline.IncludedDemands, domain.BaselineDemandEntry{Ref: domain.Ref{ID: demandID}, Task: taskID, Resource: "resource.executor", Unit: "hours", PlannedAmount: &amount})
		}
		if err := domain.SealBaseline(&baseline); err != nil {
			t.Fatal(err)
		}
		ws.Baselines = append(ws.Baselines, baseline)
	}
	return ws
}

func TestCalibrationAllBaselinesCombinesLimitedSamples(t *testing.T) {
	ws := limitedCalibrationWorkspace(t)
	ws.Baselines = append(ws.Baselines, domain.Baseline{ID: "baseline.ref-only"})
	result, err := NewCalibrationService(calibrationTestLoader{ws: ws}).Calibrate(context.Background(), CalibrationCalibrateRequest{
		AllBaselines: true, ResourceIDs: []string{"resource.executor"}, Unit: "hours", ID: "calibration.all", Name: "All", ApprovedBy: "actor.operator", DryRun: true,
	})
	if err != nil || result.Calibration.Result.Count != 8 || len(result.Calibration.SourceBaselines) != 2 {
		t.Fatalf("error=%v result=%#v", err, result)
	}
}

func TestCalibrationBaselineFlagsConflict(t *testing.T) {
	ws := limitedCalibrationWorkspace(t)
	result, err := NewCalibrationService(calibrationTestLoader{ws: ws}).Calibrate(context.Background(), CalibrationCalibrateRequest{
		AllBaselines: true, BaselineIDs: []string{"baseline.0"}, ResourceIDs: []string{"resource.executor"}, Unit: "hours", ID: "calibration.conflict", Name: "Conflict", ApprovedBy: "actor.operator", DryRun: true,
	})
	if err == nil || len(result.Errors) == 0 || result.Errors[0].Code != "calibration_baseline_flags_conflict" {
		t.Fatalf("error=%v diagnostics=%#v", err, result.Errors)
	}
}

func TestCalibrationSuggestionsCombineLimitedBaselines(t *testing.T) {
	ws := limitedCalibrationWorkspace(t)
	suggestions := CalibrationSuggestions(ws)
	if len(suggestions) != 1 || !suggestions[0].AllBaselines || suggestions[0].Count != 8 {
		t.Fatalf("suggestions=%#v", suggestions)
	}
}

func TestCalibrationSuggestionsOptIntoReviewReadyOnlyWhenNeeded(t *testing.T) {
	ws := limitedCalibrationWorkspace(t)
	for i := 4; i < len(ws.Tasks); i++ {
		ws.Tasks[i].TrackingStatus = "review_ready"
	}
	suggestions := CalibrationSuggestions(ws)
	if len(suggestions) != 1 || suggestions[0].Count != 8 || !suggestions[0].IncludeReviewReady {
		t.Fatalf("review-ready suggestions=%#v", suggestions)
	}
	actions := dashboardNextActions(ws, reports.DashboardOverview{ProjectPath: "fixture"})
	var calibrationAction *reports.DashboardNextAction
	for i := range actions {
		if actions[i].Code == "suggest_calibration" {
			calibrationAction = &actions[i]
		}
	}
	if calibrationAction == nil || !calibrationAction.IncludeReviewReady || !strings.Contains(calibrationAction.Command, "--include-review-ready") || !strings.Contains(calibrationAction.Label, "including review-ready tasks") {
		t.Fatalf("review-ready actions=%#v", actions)
	}
	brief := reports.BuildNextBrief(reports.DashboardOverview{NextActions: actions})
	if len(brief.AgentCanPrepare) != 1 || !brief.AgentCanPrepare[0].IncludeReviewReady {
		t.Fatalf("review-ready brief=%#v", brief)
	}

	for i := range ws.Tasks {
		ws.Tasks[i].TrackingStatus = "accepted"
	}
	suggestions = CalibrationSuggestions(ws)
	if len(suggestions) != 1 || suggestions[0].IncludeReviewReady {
		t.Fatalf("accepted suggestions=%#v", suggestions)
	}
}

func TestNextSuggestsAllLimitedBaselinesTogether(t *testing.T) {
	ws := limitedCalibrationWorkspace(t)
	actions := dashboardNextActions(ws, reports.DashboardOverview{ProjectPath: "fixture"})
	var suggestions []reports.DashboardNextAction
	for _, action := range actions {
		if action.Code == "suggest_calibration" {
			suggestions = append(suggestions, action)
		}
	}
	if len(suggestions) != 1 || !strings.Contains(suggestions[0].Command, "--all-baselines") || strings.Contains(suggestions[0].Command, "--baseline ") {
		t.Fatalf("suggestions=%#v", suggestions)
	}
}

func TestNextSuggestsReplacingLatestPriorOnceSampleIsLargeEnough(t *testing.T) {
	ws := limitedCalibrationWorkspace(t)
	for index := range ws.Tasks {
		ws.Tasks[index].Tags = []string{"calibration"}
	}
	prior := domain.Calibration{
		ID: "calibration.prior", ApprovedAt: "2026-01-03T00:00:00Z",
		Scope:  domain.CalibrationScope{Resources: []string{"resource.executor"}, Unit: "hours", Tag: "calibration"},
		Result: domain.CalibrationResult{Method: "prior", PriorSource: "another project", Factor: .21},
	}
	if err := domain.SealCalibration(&prior); err != nil {
		t.Fatal(err)
	}
	ws.Calibrations = []domain.Calibration{prior}
	ws.Actuals = ws.Actuals[:7]
	if actions := dashboardNextActions(ws, reports.DashboardOverview{ProjectPath: "fixture"}); countNextActions(actions, "suggest_calibration") != 0 {
		t.Fatalf("suggested before minimum: %#v", actions)
	}
	ws = limitedCalibrationWorkspace(t)
	for index := range ws.Tasks {
		ws.Tasks[index].Tags = []string{"calibration"}
	}
	ws.Calibrations = []domain.Calibration{prior}
	actions := dashboardNextActions(ws, reports.DashboardOverview{ProjectPath: "fixture"})
	if countNextActions(actions, "suggest_calibration") != 1 {
		t.Fatalf("replacement suggestions=%#v", actions)
	}
	for _, action := range actions {
		if action.Code == "suggest_calibration" && !strings.Contains(action.Label, "replace prior calibration calibration.prior") {
			t.Fatalf("replacement label=%q", action.Label)
		}
		if action.Code == "suggest_calibration" && !strings.Contains(action.Command, "--tag calibration") {
			t.Fatalf("replacement command=%q", action.Command)
		}
	}
}

func countNextActions(actions []reports.DashboardNextAction, code string) int {
	count := 0
	for _, action := range actions {
		if action.Code == code {
			count++
		}
	}
	return count
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

func TestCalibrationOpenDemandGroupsOfferLatestScopeAndReportSuperseded(t *testing.T) {
	older := sealedTestCalibration(t, "calibration.older", "2026-11-02T09:00:00Z", []string{"resource.agent"}, "hours", "", .5)
	newer := sealedTestCalibration(t, "calibration.newer", "2026-11-16T09:00:00Z", []string{"resource.agent"}, "hours", "", .25)
	ws := domain.Workspace{
		Calibrations: []domain.Calibration{older, newer},
		Tasks:        []domain.Task{{ID: "task.open"}},
		ResourceDemands: []domain.ResourceDemand{{
			ID: "demand.open", TaskRef: domain.Ref{ID: "task.open"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: 4, Status: "active",
		}},
	}

	groups := CalibrationOpenDemandGroups(ws)
	if len(groups) != 1 || groups[0].CalibrationID != newer.ID || groups[0].SupersedesCalibrationID != older.ID || len(groups[0].Demands) != 1 || groups[0].Demands[0].Calibrated != 1 {
		t.Fatalf("groups=%#v", groups)
	}
	actions := dashboardNextActions(ws, reports.DashboardOverview{ProjectPath: "fixture"})
	if len(actions) != 1 || actions[0].Code != "apply_calibration" || actions[0].SupersedesCalibrationID != older.ID || actions[0].Label != "Apply calibration calibration.newer to 1 open demands, replacing calibration.older" {
		t.Fatalf("actions=%#v", actions)
	}
	actionJSON, err := json.Marshal(actions[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(actionJSON), `"supersedes_calibration_id":"calibration.older"`) {
		t.Fatalf("action json=%s", actionJSON)
	}
	brief := reports.BuildNextBrief(reports.DashboardOverview{NextActions: actions})
	if len(brief.AgentCanPrepare) != 1 || brief.AgentCanPrepare[0].SupersedesCalibrationID != older.ID || brief.AgentCanPrepare[0].What != actions[0].Label {
		t.Fatalf("brief=%#v", brief)
	}
	var rendered strings.Builder
	if err := reports.WriteNextBriefText(&rendered, brief); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "Apply calibration calibration.newer to 1 open demands, replacing calibration.older") || strings.Contains(rendered.String(), "Apply calibration calibration.older") {
		t.Fatalf("brief text=%q", rendered.String())
	}
}

func TestCalibrationOpenDemandGroupsKeepLatestForEachScope(t *testing.T) {
	agentOlder := sealedTestCalibration(t, "calibration.agent.older", "2026-11-02T09:00:00Z", []string{"resource.agent"}, "agent_hours", "", .5)
	agentNewer := sealedTestCalibration(t, "calibration.agent.newer", "2026-11-16T09:00:00Z", []string{"resource.agent"}, "agent_hours", "", .25)
	ci := sealedTestCalibration(t, "calibration.ci", "2026-11-03T09:00:00Z", []string{"resource.ci"}, "minutes", "", .75)
	ws := domain.Workspace{
		Calibrations: []domain.Calibration{agentOlder, ci, agentNewer},
		Tasks:        []domain.Task{{ID: "task.agent"}, {ID: "task.ci"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.agent", TaskRef: domain.Ref{ID: "task.agent"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "agent_hours", PlannedAmount: 4},
			{ID: "demand.ci", TaskRef: domain.Ref{ID: "task.ci"}, ResourceRef: domain.Ref{ID: "resource.ci"}, Unit: "minutes", PlannedAmount: 10},
		},
	}

	groups := CalibrationOpenDemandGroups(ws)
	if len(groups) != 2 || groups[0].CalibrationID != agentNewer.ID || groups[0].SupersedesCalibrationID != agentOlder.ID || groups[1].CalibrationID != ci.ID || groups[1].SupersedesCalibrationID != "" {
		t.Fatalf("groups=%#v", groups)
	}
}

func TestCalibrationOpenDemandGroupsFollowLoadSelectionForTaggedScope(t *testing.T) {
	for _, test := range []struct {
		name             string
		untaggedApproved string
		taggedApproved   string
		wantGroups       []string
	}{
		{name: "newer tagged applies only to tagged demand", untaggedApproved: "2026-11-02T09:00:00Z", taggedApproved: "2026-11-16T09:00:00Z", wantGroups: []string{"calibration.tagged", "calibration.untagged"}},
		{name: "newer untagged applies to both demands", untaggedApproved: "2026-11-16T09:00:00Z", taggedApproved: "2026-11-02T09:00:00Z", wantGroups: []string{"calibration.untagged"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			untagged := sealedTestCalibration(t, "calibration.untagged", test.untaggedApproved, []string{"resource.agent"}, "hours", "", .5)
			tagged := sealedTestCalibration(t, "calibration.tagged", test.taggedApproved, []string{"resource.agent"}, "hours", "special", .25)
			ws := domain.Workspace{
				Calibrations: []domain.Calibration{tagged, untagged},
				Tasks:        []domain.Task{{ID: "task.plain"}, {ID: "task.tagged", Tags: []string{"special"}}},
				ResourceDemands: []domain.ResourceDemand{
					{ID: "demand.plain", TaskRef: domain.Ref{ID: "task.plain"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: 4},
					{ID: "demand.tagged", TaskRef: domain.Ref{ID: "task.tagged"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "hours", PlannedAmount: 4},
				},
			}
			selected := ApplicableDemandCalibrations(ws)
			groups := CalibrationOpenDemandGroups(ws)
			gotGroups := make([]string, 0, len(groups))
			for _, group := range groups {
				gotGroups = append(gotGroups, group.CalibrationID)
				for _, demand := range group.Demands {
					if selected[demand.DemandID].Calibration.ID != group.CalibrationID {
						t.Fatalf("group %s disagrees with load selection %#v", group.CalibrationID, selected[demand.DemandID])
					}
				}
			}
			if strings.Join(gotGroups, ",") != strings.Join(test.wantGroups, ",") {
				t.Fatalf("groups=%#v", groups)
			}
		})
	}
}

func sealedTestCalibration(t *testing.T, id, approvedAt string, resources []string, unit, tag string, factor float64) domain.Calibration {
	t.Helper()
	calibration := domain.Calibration{ID: id, ApprovedAt: approvedAt, Scope: domain.CalibrationScope{Resources: resources, Unit: unit, Tag: tag}, Result: domain.CalibrationResult{Factor: factor}}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	return calibration
}
