package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestWeeklyReportSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewReportService(store)

	report, err := service.Weekly(context.Background(), app.WeeklyReportRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
	})
	if err != nil {
		t.Fatalf("Weekly returned error: %v", err)
	}
	if report.ProjectID != "project.replan-mvp" {
		t.Fatalf("unexpected project id: %s", report.ProjectID)
	}
	if report.BaselineID != "baseline.2026-07-06.initial" {
		t.Fatalf("baseline = %q, want current-plan baseline", report.BaselineID)
	}
	if report.Summary.ResourceRows != 8 {
		t.Fatalf("expected 8 resource rows, got %d", report.Summary.ResourceRows)
	}
	if report.Summary.OpenProposals != 1 {
		t.Fatalf("expected 1 open proposal, got %d", report.Summary.OpenProposals)
	}
	if report.Summary.EvidenceCount != 2 {
		t.Fatalf("expected 2 evidence records, got %d", report.Summary.EvidenceCount)
	}
	if len(report.Warnings) != 5 || len(report.Errors) != 0 {
		t.Fatalf("expected dependency warning report, warnings=%#v errors=%#v", report.Warnings, report.Errors)
	}
	if !hasReportWarning(report.Warnings, "dependency_blocks_active_successor") {
		t.Fatalf("expected dependency block warning, got %#v", report.Warnings)
	}
}

func TestWeeklyReportSelfFixtureUsesCurrentPlanBaseline(t *testing.T) {
	report, err := app.NewReportService(filestore.New("../../examples/replan-self")).Weekly(context.Background(), app.WeeklyReportRequest{
		ProjectPath: "../../examples/replan-self",
		Week:        "2026-W44",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.BaselineID != "baseline.2026-07-20.self-dogfood" {
		t.Fatalf("baseline = %q, want current-plan baseline", report.BaselineID)
	}
}

func TestReportBaselineUsesCurrentPlanPin(t *testing.T) {
	ws := domain.Workspace{
		Project:     domain.Project{ID: "project.test", Name: "Test"},
		CurrentPlan: domain.CurrentPlan{ID: "current", BasedOnBaseline: domain.Ref{Kind: "baseline", ID: "baseline.pinned"}},
		Baselines: []domain.Baseline{
			{ID: "baseline.pinned", ApprovedAt: "2026-07-20T12:00:00+07:00", StatusDate: "2026-07-20"},
			{ID: "baseline.newer", ApprovedAt: "2026-10-20T12:00:00+07:00", StatusDate: "2026-10-20"},
		},
	}
	service := app.NewReportService(stubLoader{ws: ws})
	weekly, err := service.Weekly(context.Background(), app.WeeklyReportRequest{ProjectPath: "stub", Week: "2026-W44"})
	if err != nil {
		t.Fatal(err)
	}
	if weekly.BaselineID != "baseline.pinned" {
		t.Fatalf("baseline = %q, want pinned baseline", weekly.BaselineID)
	}
	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{ProjectPath: "stub"})
	if err != nil {
		t.Fatal(err)
	}
	if overview.BaselineID != "baseline.pinned" || overview.Week != "2026-W30" {
		t.Fatalf("dashboard baseline/week = %q/%q, want pinned baseline and its status week", overview.BaselineID, overview.Week)
	}
}

func TestReportBaselineFallbackOrderWithoutPin(t *testing.T) {
	tests := []struct {
		name      string
		baselines []domain.Baseline
		want      string
	}{
		{
			name: "approved at",
			baselines: []domain.Baseline{
				{ID: "baseline.approved-later", ApprovedAt: "2026-10-20T06:00:00Z", CreatedAt: "2026-10-01T12:00:00+07:00"},
				{ID: "baseline.created-later", ApprovedAt: "2026-10-19T12:00:00+07:00", CreatedAt: "2026-10-30T12:00:00+07:00"},
			},
			want: "baseline.approved-later",
		},
		{
			name: "created at",
			baselines: []domain.Baseline{
				{ID: "baseline.b", ApprovedAt: "2026-10-20T05:00:00Z", CreatedAt: "2026-10-20T12:00:00+07:00"},
				{ID: "baseline.a", ApprovedAt: "2026-10-20T12:00:00+07:00", CreatedAt: "2026-10-19T12:00:00+07:00"},
			},
			want: "baseline.b",
		},
		{
			name: "id",
			baselines: []domain.Baseline{
				{ID: "baseline.z", ApprovedAt: "2026-10-20T05:00:00Z", CreatedAt: "2026-10-20T04:00:00Z"},
				{ID: "baseline.a", ApprovedAt: "2026-10-20T12:00:00+07:00", CreatedAt: "2026-10-20T11:00:00+07:00"},
			},
			want: "baseline.z",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ws := domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}, CurrentPlan: domain.CurrentPlan{ID: "current"}, Baselines: tc.baselines}
			report, err := app.NewReportService(stubLoader{ws: ws}).Weekly(context.Background(), app.WeeklyReportRequest{ProjectPath: "stub", Week: "2026-W44"})
			if err != nil {
				t.Fatal(err)
			}
			if report.BaselineID != tc.want {
				t.Fatalf("baseline = %q, want %q", report.BaselineID, tc.want)
			}
		})
	}
}

func TestDashboardTrackingSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewReportService(store)

	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: "../../examples/small-team",
		Week:        "2026-W28",
	})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	row, ok := findTrackingRow(overview.Tracking, "task.offline.repair-pump")
	if !ok {
		t.Fatalf("tracking row for task.offline.repair-pump not found: %#v", overview.Tracking)
	}
	if row.ActualCount != 1 {
		t.Fatalf("ActualCount = %d, want 1", row.ActualCount)
	}
	if row.EvidenceCount != 1 {
		t.Fatalf("EvidenceCount = %d, want 1", row.EvidenceCount)
	}
	demand, ok := findDemandTracking(row.Demands, "resource_demand.repair-pump.operator")
	if !ok {
		t.Fatalf("demand tracking row not found: %#v", row.Demands)
	}
	if demand.Planned != 4 || demand.Actual != 2 || demand.Remaining != 2 || demand.Forecast != 4 || demand.Variance != 0 {
		t.Fatalf("unexpected demand tracking values: %#v", demand)
	}
}

func TestDashboardTrackingProposalLoopSelfFixture(t *testing.T) {
	projectPath := copyTrackingProjectFixture(t)
	removeTrackingProposalArtifacts(t, projectPath)
	markTrackingProposalDemandsStale(t, projectPath)
	store := filestore.New(projectPath)
	service := app.NewReportService(store)

	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: projectPath,
		Week:        "2026-W31",
	})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if overview.TrackingLoop.StaleRemainingCount == 0 {
		t.Fatalf("expected stale remaining rows")
	}
	if overview.TrackingLoop.ActionableCount == 0 {
		t.Fatalf("expected actionable stale remaining rows")
	}
	if overview.TrackingLoop.OpenProposalCount != 0 || overview.TrackingLoop.AcceptedProposalCount == 0 {
		t.Fatalf("unexpected tracking proposal counts: %#v", overview.TrackingLoop)
	}
	if len(overview.TrackingLoop.Rows) != overview.TrackingLoop.StaleRemainingCount {
		t.Fatalf("rows len = %d, want %d", len(overview.TrackingLoop.Rows), overview.TrackingLoop.StaleRemainingCount)
	}
	if overview.TrackingLoop.Rows[0].RecommendedCommand == "" {
		t.Fatalf("expected recommended command: %#v", overview.TrackingLoop.Rows[0])
	}
	if overview.TrackingLoop.Rows[0].Status != "actionable" || overview.TrackingLoop.Rows[0].Priority != "high" {
		t.Fatalf("unexpected first row status/priority: %#v", overview.TrackingLoop.Rows[0])
	}
	if overview.TrackingLoop.Rows[0].Reason == "" {
		t.Fatalf("expected first row reason: %#v", overview.TrackingLoop.Rows[0])
	}
}

func TestDashboardOpenProjectOverlaySelfFixture(t *testing.T) {
	store := filestore.New("../../examples/replan-self")
	service := app.NewReportService(store)
	workspace, diagnostics := store.Load(context.Background())
	if len(diagnostics) != 0 {
		t.Fatalf("load self fixture: %#v", diagnostics)
	}
	expectedLinkedTasks := openProjectLinkedTasks(workspace.Tasks)

	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: "../../examples/replan-self",
		Week:        "2026-W32",
	})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if overview.WorkloadPage.Data.WindowStart != "2026-W40" || overview.WorkloadPage.Data.WindowFinish < "2026-W52" {
		t.Fatalf("workload window = %s..%s, want 2026-W40 through at least 2026-W52", overview.WorkloadPage.Data.WindowStart, overview.WorkloadPage.Data.WindowFinish)
	}
	if len(overview.WorkloadPage.Data.Weeks) == 0 || overview.WorkloadPage.Data.Weeks[0] >= overview.WorkloadPage.Data.WindowStart {
		t.Fatalf("raw workload weeks do not retain pre-window plan history: %#v", overview.WorkloadPage.Data.Weeks)
	}
	if len(overview.ScenariosPage.Data.Scenarios) == 0 || overview.ScenariosPage.Data.Scenarios[0].Status != "draft" {
		t.Fatalf("open scenario is not first: %#v", overview.ScenariosPage.Data.Scenarios)
	}
	hasSplitGroup := false
	for _, baseline := range overview.EstimatesPage.Data.Baselines {
		hasSplitGroup = hasSplitGroup || len(baseline.SplitGroups) > 0
	}
	if !hasSplitGroup {
		t.Fatalf("estimate page has no recognized split groups: %#v", overview.EstimatesPage.Data.Baselines)
	}
	overlay := overview.OpenProjectOverlay
	if overlay.SchemaVersion != 2 {
		t.Fatalf("unexpected overlay schema version: %d", overlay.SchemaVersion)
	}
	if overlay.WorkPackageCount != len(expectedLinkedTasks) || overlay.LinkedTaskCount != len(expectedLinkedTasks) || overlay.MissingLinkCount != 0 {
		t.Fatalf("OpenProject overlay counts = work packages %d, linked tasks %d, missing %d; want %d canonical tasks with OpenProject source refs and no missing links", overlay.WorkPackageCount, overlay.LinkedTaskCount, overlay.MissingLinkCount, len(expectedLinkedTasks))
	}
	if overlay.ArtifactRefs.OverlayJSON != "raw/openproject_overlay.json" {
		t.Fatalf("unexpected overlay artifact refs: %#v", overlay.ArtifactRefs)
	}
	workPackage, ok := findOverlayWorkPackage(overlay.WorkPackages, "2142")
	if !ok {
		t.Fatalf("expected sanitized work package 2142 in overlay")
	}
	if workPackage.RePlanTaskID != "task.self.define-openproject-overlay-json-contract" {
		t.Fatalf("unexpected RePlan task for work package 2142: %#v", workPackage)
	}
	if len(workPackage.ResourceDemands) != 3 {
		t.Fatalf("expected three resource demands for work package 2142, got %#v", workPackage.ResourceDemands)
	}
	if !hasOverlayDemand(workPackage.ResourceDemands, "resource_demand.openproject-overlay-json-contract.codex") {
		t.Fatalf("expected codex demand in overlay: %#v", workPackage.ResourceDemands)
	}
}

func hasReportWarning(warnings []reports.ReportIssue, code string) bool {
	for _, warning := range warnings {
		if warning.Code == code {
			return true
		}
	}
	return false
}

func findTrackingRow(rows []reports.DashboardTrackingRow, taskID string) (reports.DashboardTrackingRow, bool) {
	for _, row := range rows {
		if row.TaskID == taskID {
			return row, true
		}
	}
	return reports.DashboardTrackingRow{}, false
}

func findDemandTracking(rows []reports.DashboardDemandTracking, demandID string) (reports.DashboardDemandTracking, bool) {
	for _, row := range rows {
		if row.DemandID == demandID {
			return row, true
		}
	}
	return reports.DashboardDemandTracking{}, false
}

func findOverlayWorkPackage(rows []reports.DashboardOpenProjectOverlayWorkPackage, workPackageID string) (reports.DashboardOpenProjectOverlayWorkPackage, bool) {
	for _, row := range rows {
		if row.WorkPackageID == workPackageID {
			return row, true
		}
	}
	return reports.DashboardOpenProjectOverlayWorkPackage{}, false
}

func hasOverlayDemand(rows []reports.DashboardOpenProjectOverlayResourceDemand, demandID string) bool {
	for _, row := range rows {
		if row.DemandID == demandID {
			return true
		}
	}
	return false
}

func TestDashboardNextActionsWaitOnPersonFirst(t *testing.T) {
	amount := func(v float64) *float64 { return &v }
	task := func(id, status, finish string) domain.Task {
		return domain.Task{ID: id, Title: id, TrackingStatus: status, PlannedStart: "2026-09-01", PlannedFinish: finish}
	}
	demand := func(id, taskID, class, status string, planned float64, remaining *float64) domain.ResourceDemand {
		return domain.ResourceDemand{ID: id, TaskRef: domain.Ref{Kind: "task", ID: taskID}, ResourceRef: domain.Ref{Kind: "resource", ID: "resource." + class},
			ResourceClass: class, Unit: "hours", PlannedAmount: planned, RemainingAmount: remaining, Status: status}
	}
	actual := func(taskID, demandID string, value float64) domain.Actual {
		return domain.Actual{ID: "actual." + demandID, Type: "work_consumed", TaskRef: domain.Ref{Kind: "task", ID: taskID},
			DemandRef: domain.Ref{Kind: "resource_demand", ID: demandID}, Amount: value, Unit: "hours",
			EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: "evidence.1"}}}
	}
	acked := demand("d.acked", "t.acked", "ai_agent", "consumed", 1, amount(0))
	acked.OverrunReview = &domain.OverrunReview{AcknowledgedForecast: 2, Unit: "hours", Reason: "real work"}
	ws := domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-09-24"},
		Baselines: []domain.Baseline{{ID: "baseline.1", StatusDate: "2026-07-20"}},
		Tasks: []domain.Task{
			task("t.review", "review_ready", "2026-09-30"),
			task("t.human", "in_progress", "2026-10-05"),
			task("t.agentwork", "in_progress", "2026-10-05"),
			task("t.late", "not_started", "2026-09-20"),
			task("t.acked", "accepted", "2026-09-10"),
			task("t.overrun", "accepted", "2026-09-12"),
		},
		ResourceDemands: []domain.ResourceDemand{
			demand("d.human.agent", "t.human", "ai_agent", "consumed", 2, amount(0)),
			demand("d.human.person", "t.human", "human", "active", 1, amount(1)),
			demand("d.agentwork.agent", "t.agentwork", "ai_agent", "active", 2, amount(2)),
			demand("d.agentwork.person", "t.agentwork", "role", "active", 1, amount(1)),
			demand("d.late", "t.late", "human", "active", 1, nil),
			acked,
			demand("d.overrun", "t.overrun", "ai_agent", "consumed", 1, amount(0)),
		},
		Actuals: []domain.Actual{actual("t.acked", "d.acked", 2), actual("t.overrun", "d.overrun", 2)},
		Dependencies: []domain.Dependency{
			{ID: "dep.done", Type: "finish_to_start", Status: "open", RequiredState: "accepted",
				PredecessorRef: domain.Ref{Kind: "task", ID: "t.acked"}, SuccessorRef: domain.Ref{Kind: "task", ID: "t.human"}},
			{ID: "dep.waiting", Type: "finish_to_start", Status: "open", RequiredState: "accepted",
				PredecessorRef: domain.Ref{Kind: "task", ID: "t.late"}, SuccessorRef: domain.Ref{Kind: "task", ID: "t.human"}},
		},
	}

	overview, err := app.NewReportService(stubLoader{ws: ws}).Dashboard(context.Background(), app.DashboardReportRequest{ProjectPath: "stub"})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if overview.Week != "2026-W39" {
		t.Fatalf("default week = %q, want the status date's 2026-W39, not the baseline's", overview.Week)
	}
	got := []string{}
	for _, action := range overview.NextActions {
		got = append(got, action.Actor+" "+action.Code+" "+action.Target+" "+action.Date)
	}
	want := []string{
		"person past_planned_finish t.late 2026-09-20",
		"person accept_or_return_task t.review 2026-09-30",
		"person do_human_work t.human 2026-10-05",
		"agent review_tracking t.overrun 2026-09-12",
		"agent resolve_dependency dep.done ",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("next actions:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestDashboardTreatsRelatesAsLinkNotBlockingWork(t *testing.T) {
	ws := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-09-24"},
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "accepted"},
			{ID: "task.b", Title: "B", TrackingStatus: "in_progress"},
		},
		Dependencies: []domain.Dependency{{
			ID: "dependency.related", Type: "relates", Status: "open", Criticality: "required",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
		}},
	}
	overview, err := app.NewReportService(stubLoader{ws: ws}).Dashboard(context.Background(), app.DashboardReportRequest{ProjectPath: "stub"})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if len(overview.Dependencies) != 1 || overview.Dependencies[0].Type != "relates" {
		t.Fatalf("dashboard did not list relates link: %#v", overview.Dependencies)
	}
	if overview.Health.OpenDependencies != 0 || overview.Health.RelatedLinks != 1 || overview.Gantt.Unscheduled[1].OpenDependencyCount != 0 {
		t.Fatalf("relates counted as open blocking work: health=%#v gantt=%#v", overview.Health, overview.Gantt)
	}
	for _, action := range overview.NextActions {
		if action.Target == "dependency.related" {
			t.Fatalf("relates appeared in next actions: %#v", overview.NextActions)
		}
	}
}
