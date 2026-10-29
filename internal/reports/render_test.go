package reports_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/reports"
)

func TestScenarioReportMarkersAndNotices(t *testing.T) {
	const scenarioID = "scenario.move-task"
	const notice = "What-if: scenario " + scenarioID + " applied in memory; this is not the plan."
	weekly := reports.WeeklyReport{ProjectID: "project.test", ProjectName: "Test", Week: "2026-W28", ScenarioID: scenarioID, Resources: []reports.ResourceRow{{ResourceID: "resource.test", Unit: "hours"}}}
	overview := reports.DashboardOverview{ProjectID: "project.test", ProjectName: "Test", ScenarioID: scenarioID}

	for name, render := range map[string]func(*bytes.Buffer) error{
		"weekly markdown": func(b *bytes.Buffer) error { return reports.WriteWeeklyMarkdown(b, weekly) },
		"weekly html":     func(b *bytes.Buffer) error { return reports.WriteWeeklyHTML(b, weekly) },
		"state markdown":  func(b *bytes.Buffer) error { return reports.WriteProjectStateMarkdown(b, overview) },
		"dashboard html":  func(b *bytes.Buffer) error { return reports.WriteDashboardHTML(b, overview) },
		"gantt html":      func(b *bytes.Buffer) error { return reports.WriteDashboardGanttHTML(b, overview) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := render(&out); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), notice) {
				t.Fatalf("output missing scenario notice:\n%s", out.String())
			}
		})
	}
	var csv bytes.Buffer
	if err := reports.WriteWeeklyCSV(&csv, weekly); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(csv.String(), "scenario_id,") || !strings.Contains(csv.String(), scenarioID) {
		t.Fatalf("scenario CSV is unmarked:\n%s", csv.String())
	}
	plainJSON, _ := json.Marshal(reports.WeeklyReport{})
	if strings.Contains(string(plainJSON), "scenario_id") {
		t.Fatalf("empty scenario_id was serialized: %s", plainJSON)
	}
}

func TestWeeklyMarkdownAndCSV(t *testing.T) {
	report := reports.WeeklyReport{
		ProjectID:   "project.test",
		ProjectName: "Test project",
		Week:        "2026-W28",
		Summary: reports.WeeklySummary{
			ResourceRows:  1,
			OpenProposals: 1,
			EvidenceCount: 1,
		},
		Resources: []reports.ResourceRow{
			{
				ResourceID:    "resource.human.ivan",
				ResourceName:  "Ivan",
				ResourceClass: "human",
				Unit:          "hours",
				Capacity:      32,
				Planned:       8,
				Reserved:      4,
				Actual:        2,
				Remaining:     8,
				Overload:      1,
			},
		},
		Warnings:  []reports.ReportIssue{{Code: "resource_overloaded", Severity: "warning", Message: "Ivan is overloaded.", TargetID: "resource.human.ivan"}},
		Proposals: []reports.ProposalRow{{ID: "proposal.1", Type: "update_remaining_work", Rationale: "Needs review."}},
		Evidence:  []reports.EvidenceRow{{ID: "evidence.1", Type: "git_event", Summary: "Pull request opened."}},
	}

	var md bytes.Buffer
	if err := reports.WriteWeeklyMarkdown(&md, report); err != nil {
		t.Fatalf("WriteWeeklyMarkdown returned error: %v", err)
	}
	if !strings.Contains(md.String(), "# Weekly RePlan Report: 2026-W28") {
		t.Fatalf("markdown missing title: %s", md.String())
	}
	if !strings.Contains(md.String(), "Open Proposals") {
		t.Fatalf("markdown missing proposals: %s", md.String())
	}

	var csv bytes.Buffer
	if err := reports.WriteWeeklyCSV(&csv, report); err != nil {
		t.Fatalf("WriteWeeklyCSV returned error: %v", err)
	}
	if !strings.Contains(csv.String(), "resource_id,resource_name,resource_class") {
		t.Fatalf("csv missing header: %s", csv.String())
	}
	if !strings.Contains(csv.String(), "resource.human.ivan,Ivan,human") {
		t.Fatalf("csv missing row: %s", csv.String())
	}

	var html bytes.Buffer
	if err := reports.WriteWeeklyHTML(&html, report); err != nil {
		t.Fatalf("WriteWeeklyHTML returned error: %v", err)
	}
	if !strings.Contains(html.String(), "<title>RePlan Weekly Report 2026-W28</title>") {
		t.Fatalf("html missing title: %s", html.String())
	}
	if !strings.Contains(html.String(), "<h2>Workload</h2>") {
		t.Fatalf("html missing workload section: %s", html.String())
	}
	if !strings.Contains(html.String(), "data-theme-choice=\"dark\"") {
		t.Fatalf("html missing theme switcher: %s", html.String())
	}
	if !strings.Contains(html.String(), "resource_overloaded") {
		t.Fatalf("html missing warnings: %s", html.String())
	}
	if !strings.Contains(html.String(), "Open Proposals") {
		t.Fatalf("html missing proposals: %s", html.String())
	}
}

func TestWorkloadCSV(t *testing.T) {
	result := reports.WorkloadCSV{
		Week:       "2026-W28",
		ScenarioID: "scenario.openai-quota-300k",
		Resources: []reports.WorkloadCSVRow{{
			ResourceID:    "resource.service.openai.tokens",
			ResourceName:  "OpenAI project token quota",
			ResourceClass: "service_quota",
			Unit:          "tokens",
			Capacity:      300000,
			Planned:       500000,
			Remaining:     500000,
			Overload:      200000,
		}},
	}

	var csv bytes.Buffer
	if err := reports.WriteWorkloadCSV(&csv, result); err != nil {
		t.Fatalf("WriteWorkloadCSV returned error: %v", err)
	}
	if !strings.Contains(csv.String(), "scenario_id,week,resource_id,resource_name") {
		t.Fatalf("csv missing header: %s", csv.String())
	}
	if !strings.Contains(csv.String(), "scenario.openai-quota-300k,2026-W28,resource.service.openai.tokens") {
		t.Fatalf("csv missing scenario row: %s", csv.String())
	}
	if !strings.Contains(csv.String(), ",300000,500000,0,0,500000,200000") {
		t.Fatalf("csv missing amount columns: %s", csv.String())
	}
}

func dashboardHTMLFixture() reports.DashboardOverview {
	return reports.DashboardOverview{
		SchemaVersion: 1,
		ProjectID:     "project.test",
		ProjectName:   "Test project",
		StatusDate:    "2026-07-20",
		GeneratedAt:   "2026-07-20T10:00:00+07:00",
		Health: reports.DashboardHealthSummary{
			TaskCount:           2,
			OverloadedResources: 1,
			OpenProposals:       1,
		},
		Workload: []reports.ResourceRow{{
			ResourceID:    "resource.human.ivan",
			ResourceName:  "Ivan",
			ResourceClass: "human",
			Unit:          "hours",
			Capacity:      24,
			Remaining:     30,
			Overload:      6,
		}},
		Tasks: []reports.DashboardTaskRow{{
			ID:                  "task.1",
			Title:               "Task 1",
			TrackingStatus:      "in_progress",
			ResourceDemandCount: 1,
		}},
		Gantt: reports.DashboardGanttView{
			SchemaVersion: 1,
			ProjectID:     "project.test",
			ProjectName:   "Test project",
			StatusDate:    "2026-07-20",
			Week:          "2026-W30",
			StartDate:     "2026-07-20",
			FinishDate:    "2026-07-22",
			DayCount:      3,
			StatusOffset:  0,
			TaskCount:     1,
			Tasks: []reports.DashboardGanttTask{{
				ID:                    "task.1",
				Title:                 "Task 1",
				TrackingStatus:        "in_progress",
				PlannedStart:          "2026-07-20",
				PlannedFinish:         "2026-07-22",
				DurationDays:          3,
				ResourceIDs:           []string{"resource.human.ivan"},
				OverloadedResourceIDs: []string{"resource.human.ivan"},
				DependencyCount:       2,
				OpenDependencyCount:   1,
				Source:                "openproject",
				SourceID:              "2101",
				SourceURL:             "https://op.example.internal/work_packages/2101",
			}},
		},
		Dependencies: []reports.DashboardDependency{{
			ID:          "dependency.1",
			Type:        "finish_to_start",
			SuccessorID: "task.1",
			Status:      "open",
		}},
		TrackingLoop: reports.DashboardTrackingLoop{
			StaleRemainingCount:   1,
			OpenProposalCount:     1,
			AcceptedProposalCount: 0,
			CoveredDemandCount:    1,
			ActionableCount:       0,
			Rows: []reports.DashboardTrackingLoopRow{{
				TaskID:             "task.1",
				TaskTitle:          "Task 1",
				DemandID:           "resource_demand.task-1.ivan",
				ResourceID:         "resource.human.ivan",
				Remaining:          5,
				Forecast:           7,
				Status:             "open",
				Priority:           "medium",
				Reason:             "proposal already open",
				ProposalID:         "proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero",
				ProposalStatus:     "open",
				RecommendedCommand: "replan proposal accept --project . proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero --dry-run --format json",
			}},
		},
		Warnings:    []reports.ReportIssue{{Code: "required_dependency_unresolved", Severity: "warning", Message: "Dependency is open.", TargetID: "dependency.1"}},
		Proposals:   []reports.ProposalRow{{ID: "proposal.1", Type: "capacity_adjustment", Rationale: "Increase capacity."}},
		Evidence:    []reports.EvidenceRow{{ID: "evidence.1", Type: "workspace_event", Summary: "Observed."}},
		NextActions: []reports.DashboardNextAction{{Code: "review_overload", Label: "Review overload.", Target: "resource.human.ivan"}},
		SourceHealth: reports.DashboardSourceHealth{
			TasksWithSourceRefs: 1,
			LinkedTasks: []reports.DashboardSourceTask{{
				TaskID:         "task.1",
				Title:          "Task 1",
				Source:         "openproject",
				SourceID:       "2101",
				SourceURL:      "https://op.example.internal/work_packages/2101",
				TrackingStatus: "in_progress",
			}},
		},
		OpenProject: reports.DashboardOpenProject{
			Enabled:                true,
			ProjectID:              "replan-self",
			TaskCount:              1,
			LinkedTaskCount:        1,
			WorkPackageURLCount:    1,
			TasksWithResourceCount: 1,
			Links: []reports.DashboardOpenProjectLink{{
				TaskID:              "task.1",
				Title:               "Task 1",
				TrackingStatus:      "in_progress",
				OpenProjectID:       "2101",
				OpenProjectURL:      "https://op.example.internal/work_packages/2101",
				PlannedStart:        "2026-07-20",
				PlannedFinish:       "2026-07-22",
				ResourceIDs:         []string{"resource.human.ivan"},
				ResourceDemandCount: 1,
				WarningCount:        1,
				OpenDependencyCount: 1,
			}},
		},
	}
}

func TestDashboardHTML(t *testing.T) {
	overview := dashboardHTMLFixture()

	var html bytes.Buffer
	if err := reports.WriteDashboardHTML(&html, overview); err != nil {
		t.Fatalf("WriteDashboardHTML returned error: %v", err)
	}
	for _, want := range []string{
		"<title>RePlan Dashboard: Test project</title>",
		"Next Actions",
		"Tracking Proposal Loop",
		"Stale remaining",
		"proposal already open",
		"medium",
		"proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero",
		"data-theme-choice=\"dark\"",
		"<h2>Workload</h2>",
		"<h2>Tasks</h2>",
		"<h2>Dependencies</h2>",
		"required_dependency_unresolved",
		"Open Proposals",
		"Evidence",
		"gantt.html",
		"raw/gantt.json",
		"raw/openproject_overlay.json",
		"raw/overview.json",
		"OpenProject Native Surface",
		"Linked work packages",
		"Tasks with resources",
		"openproject:2101",
		"OpenProject Links",
		"2101",
	} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("dashboard html missing %q:\n%s", want, html.String())
		}
	}
}

func projectStateFixture() reports.DashboardOverview {
	return reports.DashboardOverview{
		SchemaVersion: 1,
		ProjectID:     "project.test",
		ProjectName:   "Test project",
		StatusDate:    "2026-07-20",
		Week:          "2026-W30",
		CurrentPlan:   "current",
		BaselineID:    "baseline.1",
		Health: reports.DashboardHealthSummary{
			TaskCount:              2,
			AcceptedTasks:          1,
			ActiveTasks:            1,
			OpenDependencies:       1,
			OverloadedResources:    1,
			WarningCount:           1,
			OpenProposals:          1,
			EvidenceCount:          1,
			ScenarioCount:          1,
			MissingDemandTaskCount: 0,
		},
		SourceHealth: reports.DashboardSourceHealth{
			Profiles:               []reports.DashboardSourceProfile{{ID: "openproject.test", Source: "openproject"}},
			TasksWithSourceRefs:    2,
			TasksWithoutSourceRefs: 0,
		},
		Workload: []reports.ResourceRow{{
			ResourceID:   "resource.human.ivan",
			ResourceName: "Ivan",
			Unit:         "hours",
			Capacity:     24,
			Planned:      30,
			Actual:       6,
			Remaining:    24,
			Overload:     6,
		}},
		Tracking: []reports.DashboardTrackingRow{{
			TaskID:         "task.1",
			Title:          "Task 1",
			TrackingStatus: "accepted",
			ActualCount:    1,
			EvidenceCount:  1,
			Flags:          []string{"forecast_over_plan"},
			Demands: []reports.DashboardDemandTracking{{
				DemandID:      "resource_demand.task-1.ivan",
				ResourceID:    "resource.human.ivan",
				ResourceClass: "human",
				Role:          "developer",
				Unit:          "hours",
				Planned:       6,
				Actual:        2,
				Remaining:     5,
				Forecast:      7,
				Variance:      1,
				EvidenceCount: 1,
				Flags:         []string{"forecast_over_plan"},
			}},
		}},
		TrackingLoop: reports.DashboardTrackingLoop{
			StaleRemainingCount:   1,
			OpenProposalCount:     1,
			AcceptedProposalCount: 0,
			CoveredDemandCount:    1,
			ActionableCount:       0,
			Rows: []reports.DashboardTrackingLoopRow{{
				TaskID:             "task.1",
				DemandID:           "resource_demand.task-1.ivan",
				Remaining:          5,
				Forecast:           7,
				Status:             "open",
				Priority:           "medium",
				Reason:             "proposal already open",
				ProposalID:         "proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero",
				ProposalStatus:     "open",
				RecommendedCommand: "replan proposal accept --project . proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero --dry-run --format json",
			}},
		},
		Warnings: []reports.ReportIssue{{Code: "required_dependency_unresolved", Message: "Dependency is open.", TargetID: "dependency.1"}},
		NextActions: []reports.DashboardNextAction{
			{Code: "accept_or_return_task", Label: "Review the result, then accept the task or return it to work.", Target: "task.2", Actor: reports.NextActionActorPerson, Date: "2026-07-17"},
			{Code: "review_overload", Label: "Review overload.", Target: "resource.human.ivan", Command: "replan workload --project . --resource resource.human.ivan --format json", Actor: reports.NextActionActorPerson},
			{Code: "review_tracking", Label: "Review actuals.", Target: "task.1", Command: "replan actual list --project . --task task.1 --format json", Actor: reports.NextActionActorAgent, Date: "2026-07-10"},
		},
	}
}

func TestProjectStateMarkdown(t *testing.T) {
	overview := projectStateFixture()

	var md bytes.Buffer
	if err := reports.WriteProjectStateMarkdown(&md, overview); err != nil {
		t.Fatalf("WriteProjectStateMarkdown returned error: %v", err)
	}
	for _, want := range []string{
		"# RePlan Project State: Test project",
		"## Sync Health",
		"Tasks with source refs: 2",
		"## Planning Health",
		"Open dependencies: 1",
		"## Tracking Health",
		"Evidence records: 1",
		"## Plan vs Actual",
		"`resource_demand.task-1.ivan`",
		"forecast_over_plan",
		"## Tracking Proposal Loop",
		"proposal already open",
		"medium",
		"`proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero`",
		"## Resource Pressure",
		"`Ivan`",
		"## Next Actions",
		"`review_overload` `resource.human.ivan`",
	} {
		if !strings.Contains(md.String(), want) {
			t.Fatalf("project state markdown missing %q:\n%s", want, md.String())
		}
	}
}

func dashboardGanttFixture() reports.DashboardOverview {
	return reports.DashboardOverview{
		SchemaVersion: 1,
		ProjectID:     "project.test",
		ProjectName:   "Test project",
		StatusDate:    "2026-07-20",
		Week:          "2026-W30",
		Gantt: reports.DashboardGanttView{
			SchemaVersion: 1,
			ProjectID:     "project.test",
			ProjectName:   "Test project",
			StatusDate:    "2026-07-20",
			Week:          "2026-W30",
			StartDate:     "2026-07-20",
			FinishDate:    "2026-07-22",
			DayCount:      3,
			StatusInRange: true,
			TaskCount:     1,
			Tasks: []reports.DashboardGanttTask{{
				ID:                    "task.1",
				Title:                 "Task 1",
				TrackingStatus:        "in_progress",
				PlannedStart:          "2026-07-20",
				PlannedFinish:         "2026-07-22",
				DurationDays:          3,
				ResourceIDs:           []string{"resource.human.ivan"},
				OverloadedResourceIDs: []string{"resource.human.ivan"},
				DependencyCount:       2,
				OpenDependencyCount:   1,
				WarningCodes:          []string{"required_dependency_unresolved"},
				Source:                "openproject",
				SourceID:              "2101",
				SourceURL:             "https://op.example.internal/work_packages/2101",
				Allocations: []reports.DashboardGanttResourceAllocation{{
					DemandID:                  "resource_demand.task-1.ivan",
					ResourceID:                "resource.human.ivan",
					ResourceClass:             "human",
					Role:                      "developer",
					Unit:                      "hours",
					PlannedAmount:             6,
					StartDate:                 "2026-07-20",
					FinishDate:                "2026-07-22",
					DurationDays:              3,
					ParticipationWindowSource: "task_window",
					WindowCapacity:            8,
					IntensityRatio:            0.75,
					IntensityPercent:          75,
					CapacityKnown:             true,
					AmountPerDay:              2,
					Overloaded:                true,
				}},
			}},
			ResourceLoads: []reports.DashboardGanttResourceLoad{{
				ResourceID:          "resource.human.ivan",
				ResourceName:        "Ivan",
				ResourceClass:       "human",
				Unit:                "hours",
				Capacity:            8,
				Planned:             10,
				Overload:            2,
				BucketType:          "week",
				WaterlineRatio:      1,
				MaxUtilizationRatio: 1.5,
				DailyAmounts:        []float64{2, 2, 2},
				PeakDaily:           2,
				CapacityPerDay:      1,
				Buckets: []reports.DashboardGanttResourceLoadBucket{{
					Label:            "2026-W30",
					StartDate:        "2026-07-20",
					FinishDate:       "2026-07-22",
					Capacity:         4,
					Planned:          6,
					UtilizationRatio: 1.5,
					UtilizationPct:   150,
					Overload:         2,
					OverCapacity:     true,
				}},
			}},
		},
		OpenProject: reports.DashboardOpenProject{
			Enabled:                true,
			ProjectID:              "replan-self",
			LinkedTaskCount:        1,
			WorkPackageURLCount:    1,
			TasksWithResourceCount: 1,
			Links: []reports.DashboardOpenProjectLink{{
				TaskID:              "task.1",
				Title:               "Task 1",
				TrackingStatus:      "in_progress",
				OpenProjectID:       "2101",
				OpenProjectURL:      "https://op.example.internal/work_packages/2101",
				PlannedStart:        "2026-07-20",
				PlannedFinish:       "2026-07-22",
				ResourceIDs:         []string{"resource.human.ivan"},
				ResourceDemandCount: 1,
				WarningCount:        1,
				OpenDependencyCount: 1,
			}},
		},
	}
}

func TestDashboardGanttHTML(t *testing.T) {
	overview := dashboardGanttFixture()

	var html bytes.Buffer
	if err := reports.WriteDashboardGanttHTML(&html, overview); err != nil {
		t.Fatalf("WriteDashboardGanttHTML returned error: %v", err)
	}
	for _, want := range []string{
		"<title>RePlan Gantt: Test project</title>",
		"OpenProject Native Surface",
		"<h2>Gantt</h2>",
		"data-theme-choice=\"dark\"",
		"Task 1",
		"openproject:2101",
		"resources 1",
		"overload 1",
		"deps 1/2",
		"warnings 1",
		"--gantt-days: 3",
		"Resource Load",
		"resource_demand.task-1.ivan",
		"developer · 6 hours",
		"window task_window",
		"intensity 75%",
		"--allocation-fill: 75.0%",
		"100% capacity waterline",
		"bucket type week",
		"2026-W30",
		"utilization 150%",
		"Ivan",
		"resource.human.ivan",
		"required_dependency_unresolved",
		"status date 2026-07-20",
		"raw/gantt.json",
		"raw/openproject_overlay.json",
	} {
		if !strings.Contains(html.String(), want) {
			t.Fatalf("gantt html missing %q:\n%s", want, html.String())
		}
	}
}

var updateGolden = flag.Bool("update", false, "rewrite golden files")

// projectStateEdgeFixture reaches the fallbacks the main fixture skips: an
// unnamed overloaded resource next to one that is not overloaded, a loop row
// without status, priority, reason, proposal or command, a warning and an
// action without a target, and a stale count without loop rows.
func projectStateEdgeFixture() reports.DashboardOverview {
	overview := projectStateFixture()
	overview.Workload = []reports.ResourceRow{
		{ResourceID: "resource.unnamed", Unit: "hours", Capacity: 2, Planned: 5, Remaining: 5, Overload: 3},
		{ResourceID: "resource.idle", ResourceName: "Idle", Unit: "hours", Capacity: 8},
	}
	overview.TrackingLoop.Rows = append(overview.TrackingLoop.Rows, reports.DashboardTrackingLoopRow{TaskID: "task.3", DemandID: "resource_demand.3"})
	overview.Warnings = append(overview.Warnings, reports.ReportIssue{Code: "untargeted_warning", Message: "No target."})
	overview.NextActions = append(overview.NextActions, reports.DashboardNextAction{Code: "inspect_project", Label: "Nothing waits."})
	return overview
}

// The golden files pin the whole state Markdown, so a change anywhere in
// WriteProjectStateMarkdown shows up as a reviewable diff (GAP-005). The
// empty project must keep only the header, the health sections and the
// "none" resource row.
func TestProjectStateMarkdownGolden(t *testing.T) {
	staleOnly := reports.DashboardOverview{ProjectID: "project.stale", ProjectName: "Stale only"}
	staleOnly.TrackingLoop.StaleRemainingCount = 2
	for _, page := range []struct {
		file     string
		overview reports.DashboardOverview
	}{
		{"project_state.golden.md", projectStateFixture()},
		{"project_state_edges.golden.md", projectStateEdgeFixture()},
		{"project_state_empty.golden.md", reports.DashboardOverview{ProjectID: "project.empty", ProjectName: "Empty project"}},
		{"project_state_stale_only.golden.md", staleOnly},
	} {
		var md bytes.Buffer
		if err := reports.WriteProjectStateMarkdown(&md, page.overview); err != nil {
			t.Fatalf("%s: WriteProjectStateMarkdown returned error: %v", page.file, err)
		}
		path := filepath.Join("testdata", page.file)
		if *updateGolden {
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, md.Bytes(), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read golden file (run go test ./internal/reports -run Golden -update): %v", err)
		}
		if !bytes.Equal(md.Bytes(), want) {
			t.Fatalf("project state markdown differs from %s; rerun with -update and review the diff:\n%s", path, md.String())
		}
	}
}

func TestNextBriefTextGolden(t *testing.T) {
	overview := projectStateFixture()
	overview.NextActions = append(overview.NextActions, reports.DashboardNextAction{Code: "apply_calibration", Label: "Apply calibration calibration.test to 5 open demands", Target: "calibration.test", Command: "replan estimate apply --project . --calibration calibration.test --scenario-id <scenario-id> --actor <actor-id> --dry-run --format json", Actor: reports.NextActionActorAgent})
	overview.NextActions = append(overview.NextActions, reports.DashboardNextAction{Code: "inspect_project", Label: "Nothing waits."})
	overview.Tasks = []reports.DashboardTaskRow{{ID: "task.2", Title: "Ship the second task"}}
	brief := reports.BuildNextBrief(overview)
	if len(brief.WaitingOnPerson) != 2 || len(brief.AgentCanPrepare) != 2 {
		t.Fatalf("brief groups = %d person, %d agent; want 2 and 2 (inspect_project dropped)", len(brief.WaitingOnPerson), len(brief.AgentCanPrepare))
	}
	var text bytes.Buffer
	if err := reports.WriteNextBriefText(&text, brief); err != nil {
		t.Fatalf("WriteNextBriefText returned error: %v", err)
	}
	path := filepath.Join("testdata", "next_brief.golden.txt")
	if *updateGolden {
		if err := os.WriteFile(path, text.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden file (run go test ./internal/reports -run Golden -update): %v", err)
	}
	if !bytes.Equal(text.Bytes(), want) {
		t.Fatalf("next brief text differs from %s; rerun with -update and review the diff:\n%s", path, text.String())
	}
}
