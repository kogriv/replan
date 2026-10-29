package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func TestEstimateReportDemandVarianceDatesSummariesAndSplit(t *testing.T) {
	ws := estimateReportWorkspace(t)
	result, err := app.NewReportService(stubLoader{ws: ws}).Estimates(context.Background(), app.EstimateReportRequest{BaselineID: "baseline.estimates"})
	if err != nil {
		t.Fatalf("Estimates returned error: %v; diagnostics=%#v", err, result.Errors)
	}
	if result.BaselineSealStatus != "verified" || !result.BaselineHasEstimates || len(result.Tasks) != 2 {
		t.Fatalf("unexpected report header: %#v", result)
	}

	finished := estimateTaskByID(t, result.Tasks, "task.finished")
	if finished.Open || finished.ActualsPartial || finished.Schedule.ActualStart != "2026-10-02" || finished.Schedule.ActualStartSource != "audit" ||
		finished.Schedule.ActualFinish != "2026-10-07" || finished.Schedule.ActualFinishSource != "audit" {
		t.Fatalf("unexpected finished task state or dates: %#v", finished)
	}
	if finished.Schedule.FinishVarianceCalendarDays == nil || *finished.Schedule.FinishVarianceCalendarDays != 2 ||
		finished.Schedule.FinishVarianceWorkingDays == nil || *finished.Schedule.FinishVarianceWorkingDays != 2 {
		t.Fatalf("unexpected schedule variance: %#v", finished.Schedule)
	}
	agent := estimateDemandByID(t, finished.Demands, "demand.finished.agent")
	if agent.State != "current" || agent.BaselineAmount != 4 || agent.CurrentPlannedAmount != 1.5 || agent.ActualAmount != 0.75 || !agent.Reestimated ||
		agent.ActualBaselineRatio == nil || !closeFloat(*agent.ActualBaselineRatio, 0.1875) {
		t.Fatalf("unexpected agent demand: %#v", agent)
	}
	maintainer := estimateDemandByID(t, finished.Demands, "demand.finished.maintainer")
	if !maintainer.Reestimated || maintainer.BaselineAmount != 3 || maintainer.CurrentPlannedAmount != 4 || maintainer.ActualAmount != 3.5 {
		t.Fatalf("unexpected re-estimate: %#v", maintainer)
	}
	removed := estimateDemandByID(t, finished.Demands, "demand.finished.ci")
	if removed.State != "removed" || removed.CurrentPlannedAmount != 0 || removed.ActualAmount != 0 || removed.ActualsRecorded {
		t.Fatalf("unexpected removed demand: %#v", removed)
	}
	added := estimateDemandByID(t, finished.Demands, "demand.finished.codex")
	if added.State != "added" || added.BaselineAmount != 0 || added.CurrentPlannedAmount != 2.5 || added.ActualAmount != 2 {
		t.Fatalf("unexpected added demand: %#v", added)
	}
	if len(finished.SplitComparisons) != 1 || finished.SplitComparisons[0].BaselineBasis != "reduced_original_demand" ||
		finished.SplitComparisons[0].BaselineDemandID != "demand.finished.agent" || finished.SplitComparisons[0].BaselineAmount != 4 {
		t.Fatalf("unexpected split comparison: %#v", finished.SplitComparisons)
	}

	open := estimateTaskByID(t, result.Tasks, "task.open")
	if !open.Open || !open.ActualsPartial || open.Schedule.ActualStart != "2026-10-08" || open.Schedule.ActualStartSource != "consumption_actual" ||
		open.Schedule.ActualFinish != "2026-10-09" || open.Schedule.ActualFinishSource != "consumption_actual" {
		t.Fatalf("audit without status facts should fall back to consumption actuals: %#v", open)
	}
	openAgent := estimateDemandByID(t, open.Demands, "demand.open.agent")
	if openAgent.ActualAmount != 1.16 || openAgent.VarianceAmount != -0.84 || openAgent.ActualBaselineRatio == nil || *openAgent.ActualBaselineRatio != 0.58 {
		t.Fatalf("float-noise amounts were not rounded at the result boundary: %#v", openAgent)
	}

	agentSummary := estimateResourceByID(t, result.Resources, "resource.agent", "agent_hours")
	if agentSummary.Finished.BaselineAmount != 4 || agentSummary.Finished.CurrentPlannedAmount != 1.5 || agentSummary.Finished.ActualAmount != 0.75 ||
		agentSummary.Finished.ActualBaselineRatio == nil || !closeFloat(*agentSummary.Finished.ActualBaselineRatio, 0.1875) ||
		agentSummary.All.BaselineAmount != 6 || agentSummary.All.CurrentPlannedAmount != 3.5 || agentSummary.All.ActualAmount != 1.91 || !agentSummary.ActualsRecorded {
		t.Fatalf("unexpected agent summary: %#v", agentSummary)
	}
	codexSummary := estimateResourceByID(t, result.Resources, "resource.codex", "agent_hours")
	if codexSummary.Finished.BaselineAmount != 0 || codexSummary.Finished.CurrentPlannedAmount != 2.5 || codexSummary.Finished.ActualBaselineRatio != nil {
		t.Fatalf("unexpected added-resource summary: %#v", codexSummary)
	}
	ciSummary := estimateResourceByID(t, result.Resources, "resource.ci", "ci_minutes")
	if ciSummary.ActualsRecorded || ciSummary.Finished.ActualBaselineRatio != nil || ciSummary.All.ActualBaselineRatio != nil {
		t.Fatalf("resource without counted actuals looks like a recorded zero: %#v", ciSummary)
	}
	if len(result.SplitGroups) != 1 {
		t.Fatalf("split groups=%#v, want one", result.SplitGroups)
	}
	group := result.SplitGroups[0]
	if group.OriginalResourceID != "resource.agent" || len(group.SplitResourceIDs) != 1 || group.SplitResourceIDs[0] != "resource.codex" ||
		group.Finished.BaselineAmount != 4 || group.Finished.CurrentPlannedAmount != 4 || group.Finished.ActualAmount != 2.75 ||
		group.Finished.ActualBaselineRatio == nil || *group.Finished.ActualBaselineRatio != 0.6875 || !group.ActualsRecorded ||
		group.All.BaselineAmount != 6 || group.All.CurrentPlannedAmount != 6 || group.All.ActualAmount != 3.91 ||
		group.All.ActualBaselineRatio == nil || *group.All.ActualBaselineRatio != 0.651667 {
		t.Fatalf("unexpected split group: %#v", group)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "1.160000") || !strings.Contains(string(encoded), `"actual_amount":1.16`) {
		t.Fatalf("JSON contains float noise or lacks rounded amount: %s", encoded)
	}
}

func TestEstimateReportRefusals(t *testing.T) {
	refOnly := domain.Baseline{ID: "baseline.ref", CreatedAt: "2026-10-01T00:00:00Z", ApprovedBy: domain.Ref{Kind: "actor", ID: "actor.test"},
		IncludedTasks: []domain.BaselineTaskEntry{{Ref: domain.Ref{Kind: "task", ID: "task.finished"}}}}
	if err := domain.SealBaseline(&refOnly); err != nil {
		t.Fatal(err)
	}
	modified := domain.Baseline{ID: "baseline.modified", CreatedAt: "2026-10-01T00:00:00Z", ApprovedBy: domain.Ref{Kind: "actor", ID: "actor.test"},
		IncludedTasks: []domain.BaselineTaskEntry{{Ref: domain.Ref{Kind: "task", ID: "task.finished"}, PlannedStart: "2026-10-01"}},
		Seal:          &domain.BaselineSeal{Algorithm: "sha256", Digest: "wrong"}}
	service := app.NewReportService(stubLoader{ws: domain.Workspace{Project: domain.Project{ID: "project.test"}, Baselines: []domain.Baseline{refOnly, modified}}})
	for _, test := range []struct{ id, code string }{
		{id: "baseline.missing", code: "baseline_not_found"},
		{id: "baseline.ref", code: "baseline_has_no_estimates"},
		{id: "baseline.modified", code: "baseline_modified"},
	} {
		t.Run(test.code, func(t *testing.T) {
			result, err := service.Estimates(context.Background(), app.EstimateReportRequest{BaselineID: test.id})
			if !errors.Is(err, app.ErrValidationFailed) || !hasAppDiagnostic(result.Errors, test.code) {
				t.Fatalf("error=%v diagnostics=%#v, want %s", err, result.Errors, test.code)
			}
		})
	}
}

func estimateReportWorkspace(t *testing.T) domain.Workspace {
	t.Helper()
	remaining := func(value float64) *float64 { return &value }
	baseline := domain.Baseline{
		ID: "baseline.estimates", CreatedAt: "2026-10-01T00:00:00Z", ApprovedBy: domain.Ref{Kind: "actor", ID: "actor.test"},
		IncludedTasks: []domain.BaselineTaskEntry{
			{Ref: domain.Ref{Kind: "task", ID: "task.finished"}, PlannedStart: "2026-10-01", PlannedFinish: "2026-10-05"},
			{Ref: domain.Ref{Kind: "task", ID: "task.open"}, PlannedStart: "2026-10-08", PlannedFinish: "2026-10-12"},
		},
		IncludedDemands: []domain.BaselineDemandEntry{
			{Ref: domain.Ref{Kind: "resource_demand", ID: "demand.finished.agent"}, Task: "task.finished", Resource: "resource.agent", Unit: "agent_hours", PlannedAmount: domain.Amount(4)},
			{Ref: domain.Ref{Kind: "resource_demand", ID: "demand.finished.maintainer"}, Task: "task.finished", Resource: "resource.maintainer", Unit: "hours", PlannedAmount: domain.Amount(3)},
			{Ref: domain.Ref{Kind: "resource_demand", ID: "demand.finished.ci"}, Task: "task.finished", Resource: "resource.ci", Unit: "ci_minutes", PlannedAmount: domain.Amount(2)},
			{Ref: domain.Ref{Kind: "resource_demand", ID: "demand.open.agent"}, Task: "task.open", Resource: "resource.agent", Unit: "agent_hours", PlannedAmount: domain.Amount(2)},
		},
	}
	if err := domain.SealBaseline(&baseline); err != nil {
		t.Fatal(err)
	}
	return domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test"}, Baselines: []domain.Baseline{baseline},
		Resources: []domain.Resource{{ID: "resource.agent", Name: "Agent"}, {ID: "resource.codex", Name: "Codex"}, {ID: "resource.maintainer", Name: "Maintainer"}, {ID: "resource.ci", Name: "CI"}},
		Tasks:     []domain.Task{{ID: "task.finished", Title: "Finished", TrackingStatus: "accepted"}, {ID: "task.open", Title: "Open", TrackingStatus: "in_progress"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.finished.agent", TaskRef: domain.Ref{ID: "task.finished"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "agent_hours", PlannedAmount: 1.5, RemainingAmount: remaining(0)},
			{ID: "demand.finished.codex", TaskRef: domain.Ref{ID: "task.finished"}, ResourceRef: domain.Ref{ID: "resource.codex"}, Unit: "agent_hours", PlannedAmount: 2.5, RemainingAmount: remaining(0)},
			{ID: "demand.finished.maintainer", TaskRef: domain.Ref{ID: "task.finished"}, ResourceRef: domain.Ref{ID: "resource.maintainer"}, Unit: "hours", PlannedAmount: 4, RemainingAmount: remaining(0)},
			{ID: "demand.open.agent", TaskRef: domain.Ref{ID: "task.open"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "agent_hours", PlannedAmount: 2, RemainingAmount: remaining(0.5)},
		},
		Actuals: []domain.Actual{
			{ID: "actual.agent", Type: "work_consumed", DemandRef: domain.Ref{ID: "demand.finished.agent"}, Amount: 0.75, Unit: "agent_hours", StatusDate: "2026-10-03"},
			{ID: "actual.codex", Type: "runtime_consumed", DemandRef: domain.Ref{ID: "demand.finished.codex"}, Amount: 2, Unit: "agent_hours", StatusDate: "2026-10-04"},
			{ID: "actual.maintainer", Type: "consumption", DemandRef: domain.Ref{ID: "demand.finished.maintainer"}, Amount: 3.5, Unit: "hours", StatusDate: "2026-10-06"},
			{ID: "actual.open.1", Type: "consumption", DemandRef: domain.Ref{ID: "demand.open.agent"}, Amount: 1.1, Unit: "agent_hours", StatusDate: "2026-10-08"},
			{ID: "actual.open.2", Type: "work_consumed", TaskRef: domain.Ref{ID: "task.open"}, DemandRef: domain.Ref{ID: "demand.open.agent"}, Amount: 0.06, Unit: "agent_hours", OccurredAt: "2026-10-09T12:00:00Z"},
		},
		AuditRecords: []domain.AuditRecord{
			{ID: "audit.start", Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.finished"}, NewStatus: "in_progress", StatusDate: "2026-10-02"},
			{ID: "audit.finish", Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.finished"}, NewStatus: "accepted", StatusDate: "2026-10-07"},
			{ID: "audit.old", Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.open"}, OccurredAt: "2026-10-01T12:00:00Z"},
		},
		Availabilities: []domain.Availability{{ResourceRef: domain.Ref{ID: "resource.agent"}, Windows: []domain.AvailabilityWindow{{Days: []string{"monday", "tuesday", "wednesday", "thursday", "friday"}}}}},
	}
}

func estimateTaskByID(t *testing.T, rows []app.EstimateTask, id string) app.EstimateTask {
	t.Helper()
	for _, row := range rows {
		if row.TaskID == id {
			return row
		}
	}
	t.Fatalf("task %s not found", id)
	return app.EstimateTask{}
}

func estimateDemandByID(t *testing.T, rows []app.EstimateDemand, id string) app.EstimateDemand {
	t.Helper()
	for _, row := range rows {
		if row.DemandID == id {
			return row
		}
	}
	t.Fatalf("demand %s not found", id)
	return app.EstimateDemand{}
}

func estimateResourceByID(t *testing.T, rows []app.EstimateResourceSummary, id, unit string) app.EstimateResourceSummary {
	t.Helper()
	for _, row := range rows {
		if row.ResourceID == id && row.Unit == unit {
			return row
		}
	}
	t.Fatalf("resource %s/%s not found", id, unit)
	return app.EstimateResourceSummary{}
}

func closeFloat(got, want float64) bool { return math.Abs(got-want) < 1e-9 }
