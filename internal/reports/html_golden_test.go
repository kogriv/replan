package reports_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/reports"
)

func TestDashboardRawJSONContracts(t *testing.T) {
	fixture := htmlGoldenFixture()
	for name, page := range map[string]any{
		"workload":  fixture.WorkloadPage,
		"scenarios": fixture.ScenariosPage,
		"proposals": fixture.ProposalsPage,
		"estimates": fixture.EstimatesPage,
	} {
		t.Run(name, func(t *testing.T) {
			data, err := json.Marshal(page)
			if err != nil {
				t.Fatal(err)
			}
			var contract map[string]any
			if err := json.Unmarshal(data, &contract); err != nil {
				t.Fatal(err)
			}
			if name == "workload" && !bytes.Contains(data, []byte(`"utilization":1.1667`)) {
				t.Errorf("raw workload lost source precision: %s", data)
			}
			for _, field := range []string{"schema_version", "generated_at", "project_id", "project_name", "status_date", "source_snapshot_refs", "warnings", "data"} {
				if _, ok := contract[field]; !ok {
					t.Errorf("contract missing %q: %s", field, data)
				}
			}
		})
	}
}

// htmlGoldenFixture merges the dashboard and Gantt fixtures and adds what
// neither reaches: every tracking status, a task without a work-package URL,
// an open-proposal badge, unscheduled tasks, a resource without a name, a
// truncated tracking-loop table, source profiles, unlinked and missing-link
// tasks, errors, and text that must be escaped (GAP-005).
func htmlGoldenFixture() reports.DashboardOverview {
	overview := dashboardHTMLFixture()
	gantt := dashboardGanttFixture()
	overview.Week = gantt.Week
	overview.Gantt = gantt.Gantt
	overview.OpenProject = gantt.OpenProject

	const nasty = `Fix <b>"quotes"</b> & 'apostrophes'`
	overview.ProjectName = "Test <project> & co"
	overview.Gantt.ProjectName = overview.ProjectName

	statuses := []string{"accepted", "done", "review_ready", "ready", "not_started", "blocked", "on_hold", "unknown_status"}
	for i, status := range statuses {
		day := fmt.Sprintf("2026-07-%02d", 20+i%3)
		task := reports.DashboardGanttTask{
			ID:              fmt.Sprintf("task.status.%d", i),
			Title:           "Status " + status,
			TrackingStatus:  status,
			PlannedStart:    day,
			PlannedFinish:   day,
			StartOffsetDays: i % 3,
			DurationDays:    1,
		}
		switch i {
		case 0:
			task.Title = nasty
			task.Source, task.SourceID = "openproject", "2102"
		case 1:
			task.HasOpenProposal = true
			task.WarningCodes = []string{"accepted_task_planned_after_status_date"}
		}
		overview.Gantt.Tasks = append(overview.Gantt.Tasks, task)
	}
	// A twelve-day range gets Monday tick labels; the edge task starts before
	// the range and its allocations overrun both ends, with intensities below
	// and above the drawable bounds.
	overview.Gantt.FinishDate, overview.Gantt.DayCount = "2026-07-31", 12
	overview.Gantt.Tasks = append(overview.Gantt.Tasks, reports.DashboardGanttTask{
		ID: "task.edge", Title: "Edge task", TrackingStatus: "in_progress",
		PlannedStart: "2026-07-18", PlannedFinish: "2026-07-21", StartOffsetDays: -2, DurationDays: 4,
		Allocations: []reports.DashboardGanttResourceAllocation{
			{DemandID: "resource_demand.edge.low", ResourceID: "resource.human.ivan", Unit: "hours", PlannedAmount: 1,
				StartOffsetDays: -1, DurationDays: 20, CapacityKnown: true, IntensityPercent: 3},
			{DemandID: "resource_demand.edge.high", ResourceID: "resource.human.ivan", Unit: "hours", PlannedAmount: 30,
				StartOffsetDays: 10, DurationDays: 5, CapacityKnown: true, IntensityPercent: 250, Overloaded: true, OverCapacity: true},
		},
	})
	overview.Gantt.TaskCount = len(overview.Gantt.Tasks)
	overview.Gantt.Unscheduled = []reports.DashboardGanttTask{
		{ID: "task.unscheduled.1", Title: nasty, TrackingStatus: "not_started", Source: "openproject", SourceID: "2103", ResourceIDs: []string{"resource.human.ivan", "resource.ai.agent"}},
		{ID: "task.unscheduled.2", Title: "Unscheduled without source", TrackingStatus: "ready"},
	}
	unnamed := overview.Gantt.ResourceLoads[0]
	unnamed.ResourceID, unnamed.ResourceName, unnamed.Overload = "resource.unnamed", "", 0
	overview.Gantt.ResourceLoads = append(overview.Gantt.ResourceLoads, unnamed)

	overview.Workload = append(overview.Workload, reports.ResourceRow{ResourceID: "resource.unnamed", ResourceClass: "role", Unit: "review_hours", Capacity: 2, Remaining: 1})
	for i := 0; i < 21; i++ {
		row := reports.DashboardTrackingLoopRow{TaskID: fmt.Sprintf("task.loop.%02d", i), DemandID: fmt.Sprintf("resource_demand.loop.%02d", i), Remaining: 1, Forecast: 1}
		if i == 0 {
			row.TaskTitle, row.Reason = nasty, nasty
		}
		overview.TrackingLoop.Rows = append(overview.TrackingLoop.Rows, row)
	}
	overview.SourceHealth.Profiles = []reports.DashboardSourceProfile{{ID: "openproject.test", Source: "openproject", BaseURL: "https://op.example.internal/?a=1&b=2", ProjectID: "test"}}
	overview.SourceHealth.UnlinkedTasks = []reports.DashboardSourceTask{{TaskID: "task.unlinked", Title: nasty, TrackingStatus: "ready"}}
	overview.OpenProject.MissingLinks = []reports.DashboardOpenProjectMissing{{TaskID: "task.unlinked", Title: nasty, TrackingStatus: "ready"}}
	overview.OpenProject.MissingLinkCount = 1
	overview.Errors = []reports.ReportIssue{{Code: "unresolved_reference", Message: nasty, TargetID: "task.unlinked"}}
	overview.NextActions = []reports.DashboardNextAction{
		{Code: "accept_or_return_task", Label: nasty, Target: "task.status.2", Actor: reports.NextActionActorPerson, Date: "2026-07-22"},
		{Code: "review_tracking", Label: "Review actuals.", Target: "task.1", Command: "replan actual list --project . --task task.1 --format json", Actor: reports.NextActionActorAgent},
		{Code: "inspect_project", Label: "Nothing waits."},
	}
	overview.WorkloadPage = reports.DashboardWorkloadPage{DashboardPageMetadata: reports.DashboardPageMetadata{SchemaVersion: 1, GeneratedAt: overview.GeneratedAt, ProjectID: overview.ProjectID, ProjectName: overview.ProjectName, StatusDate: overview.StatusDate, SourceSnapshotRefs: []string{}, Warnings: []reports.ReportIssue{}}, Data: reports.DashboardWorkloadData{WindowStart: "2026-W30", WindowFinish: "2026-W30", Weeks: []string{"2026-W30"}, Resources: []reports.DashboardWorkloadResource{{ResourceID: "resource.human.ivan", ResourceName: nasty, ResourceClass: "human", Unit: "hours", Weeks: []reports.DashboardWorkloadCell{{Week: "2026-W30", Capacity: 20.1234, Reserved: 24.5678, Remaining: 26.3456, Overload: 4.4444, Utilization: floatPointer(1.1667), Demands: []reports.DashboardWorkloadDemand{{ResourceDemandID: "demand.1", TaskID: "task.1", TaskTitle: nasty, Remaining: 26.3456}}}}}}}}
	overview.ScenariosPage = reports.DashboardScenariosPage{DashboardPageMetadata: overview.WorkloadPage.DashboardPageMetadata, Data: reports.DashboardScenariosData{Scenarios: []reports.DashboardScenarioDetail{{ID: "scenario.1", Name: nasty, Status: "draft", CalibrationRef: "calibration.1", Changes: []reports.DashboardScenarioChange{{Op: "replace", Path: "/tasks/task.1/planned_finish", Value: nasty}}, AffectedResources: []string{"resource.human.ivan"}, AffectedTasks: []string{"task.1"}, WorkloadBefore: []reports.ResourceRow{{ResourceID: "resource.human.ivan", Unit: "hours", Capacity: 20, Remaining: 18}}, WorkloadAfter: []reports.ResourceRow{{ResourceID: "resource.human.ivan", Unit: "hours", Capacity: 20, Remaining: 26, Overload: 6}}, Warnings: []reports.ReportIssue{}, Errors: []reports.ReportIssue{}, ApplyCommand: "replan scenario apply --project <project> --dry-run --format json scenario.1"}, {ID: "scenario.archived", Name: "Archived scenario", Status: "applied", Changes: []reports.DashboardScenarioChange{}, WorkloadBefore: []reports.ResourceRow{}, WorkloadAfter: []reports.ResourceRow{}, Warnings: []reports.ReportIssue{}, Errors: []reports.ReportIssue{}, ApplyCommand: "replan scenario apply --project <project> --dry-run --format json scenario.archived"}}}}
	overview.ProposalsPage = reports.DashboardProposalsPage{DashboardPageMetadata: overview.WorkloadPage.DashboardPageMetadata, Data: reports.DashboardProposalsData{Proposals: []reports.DashboardProposalDetail{{ID: "proposal.1", Type: "tracking", Status: "open", CreatedBy: "actor.agent", TargetID: "task.1", Rationale: nasty, Impact: reports.DashboardProposalImpact{Op: "replace", Path: "/remaining_amount", Value: 2}, Evidence: []reports.DashboardProposalEvidence{{ID: "evidence.1", Type: "pull_request", Summary: nasty}}, AcceptCommand: "replan proposal accept --project <project> --actor <actor-id> --reason \"<reason>\" --apply --format json proposal.1", RejectCommand: "replan proposal reject --project <project> --actor <actor-id> --reason \"<reason>\" --apply --format json proposal.1"}}}}
	overview.EstimatesPage = reports.DashboardEstimatesPage{DashboardPageMetadata: overview.WorkloadPage.DashboardPageMetadata, Data: reports.DashboardEstimatesData{Baselines: []reports.DashboardEstimateBaseline{{ID: "baseline.1", Name: nasty, SealStatus: "verified", Resources: []reports.DashboardEstimateResource{{ResourceID: "resource.ai.agent", ResourceName: nasty, Unit: "agent_hours", ActualsRecorded: true, BaselineAmount: 10, ActualAmount: 4.567, Ratio: floatPointer(0.4567)}, {ResourceID: "resource.ai.codex.local", Unit: "agent_hours", ActualsRecorded: true, ActualAmount: 7.23, AddedAfterBaseline: true}}, SplitGroups: []reports.DashboardEstimateSplitGroup{{OriginalResourceID: "resource.ai.agent", SplitResourceIDs: []string{"resource.ai.codex.local"}, Unit: "agent_hours", ActualsRecorded: true, BaselineAmount: 10, CurrentPlannedAmount: 12, ActualAmount: 7.23, Ratio: floatPointer(0.723)}}, Tasks: []reports.DashboardEstimateTask{}, Calibrations: []reports.DashboardEstimateCalibration{{ID: "calibration.1", Name: nasty, Resources: []string{"resource.ai.agent"}, Unit: "agent_hours", Factor: 0.4, TotalRatio: 0.4, Count: 8}}, Warnings: []reports.ReportIssue{}, Errors: []reports.ReportIssue{}}}}}
	return overview
}

func floatPointer(value float64) *float64 { return &value }

// htmlEmptyFixture is a project with nothing to show: every optional section
// must stay out of the page.
func htmlEmptyFixture() reports.DashboardOverview {
	return reports.DashboardOverview{SchemaVersion: 1, ProjectID: "project.empty", ProjectName: "Empty project"}
}

// The golden files pin both pages whole, so a lost section, a broken column,
// a reordered row or missing escaping shows up as a reviewable diff.
func TestDashboardHTMLGolden(t *testing.T) {
	dashboard := func(b *bytes.Buffer, o reports.DashboardOverview) error { return reports.WriteDashboardHTML(b, o) }
	gantt := func(b *bytes.Buffer, o reports.DashboardOverview) error { return reports.WriteDashboardGanttHTML(b, o) }
	workload := func(b *bytes.Buffer, o reports.DashboardOverview) error {
		return reports.WriteDashboardWorkloadHTML(b, o.WorkloadPage)
	}
	scenarios := func(b *bytes.Buffer, o reports.DashboardOverview) error {
		return reports.WriteDashboardScenariosHTML(b, o.ScenariosPage)
	}
	proposals := func(b *bytes.Buffer, o reports.DashboardOverview) error {
		return reports.WriteDashboardProposalsHTML(b, o.ProposalsPage)
	}
	estimates := func(b *bytes.Buffer, o reports.DashboardOverview) error {
		return reports.WriteDashboardEstimatesHTML(b, o.EstimatesPage)
	}
	for _, page := range []struct {
		file     string
		overview reports.DashboardOverview
		write    func(*bytes.Buffer, reports.DashboardOverview) error
	}{
		{"dashboard.golden.html", htmlGoldenFixture(), dashboard},
		{"gantt.golden.html", htmlGoldenFixture(), gantt},
		{"workload.golden.html", htmlGoldenFixture(), workload},
		{"scenarios.golden.html", htmlGoldenFixture(), scenarios},
		{"proposals.golden.html", htmlGoldenFixture(), proposals},
		{"estimates.golden.html", htmlGoldenFixture(), estimates},
		{"dashboard_empty.golden.html", htmlEmptyFixture(), dashboard},
		{"gantt_empty.golden.html", htmlEmptyFixture(), gantt},
		{"workload_empty.golden.html", htmlEmptyFixture(), workload},
		{"scenarios_empty.golden.html", htmlEmptyFixture(), scenarios},
		{"proposals_empty.golden.html", htmlEmptyFixture(), proposals},
		{"estimates_empty.golden.html", htmlEmptyFixture(), estimates},
	} {
		var out bytes.Buffer
		if err := page.write(&out, page.overview); err != nil {
			t.Fatalf("%s: write returned error: %v", page.file, err)
		}
		if strings.Contains(out.String(), "<b>") || strings.Contains(out.String(), "Test <project>") {
			t.Fatalf("%s: unescaped fixture text in output", page.file)
		}
		if page.file == "workload.golden.html" && (strings.Contains(out.String(), "20.1234") || !strings.Contains(out.String(), "20.12")) {
			t.Fatalf("%s: workload display numbers are not rounded to two decimals", page.file)
		}
		path := filepath.Join("testdata", page.file)
		if *updateGolden {
			if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read golden file (run go test ./internal/reports -run Golden -update): %v", err)
		}
		if !bytes.Equal(out.Bytes(), want) {
			t.Fatalf("%s differs from the golden file; rerun with -update and review the diff", page.file)
		}
	}
}
