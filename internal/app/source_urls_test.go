package app

import (
	"testing"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
)

func TestReportSourceURLs(t *testing.T) {
	ws := domain.Workspace{
		Project: domain.Project{SourceProfiles: []domain.SourceProfile{
			{Source: "openproject", BaseURL: "https://public.example.test/pm"},
		}},
		OpenProjectBindingBaseURL: "  HtTpS://bound.example.test/openproject/  ",
	}
	for _, tc := range []struct{ id, raw, want string }{
		{"42", "/openproject/api/v3/work_packages/42", "https://bound.example.test/openproject/work_packages/42"},
		{"42", "https://evidence.example.test/api/v3/work_packages/42", "https://bound.example.test/openproject/work_packages/42"},
		{"42", "https://evidence.example.test/work_packages/42", "https://bound.example.test/openproject/work_packages/42"},
		{"42", "", "https://bound.example.test/openproject/work_packages/42"},
		{"42/extra", "", ""},
	} {
		ref := domain.SourceRef{Source: "openproject", ObjectType: "work_package", ObjectID: tc.id, URL: tc.raw}
		task := domain.Task{ID: "task.link", SourceRefs: []domain.SourceRef{ref}}
		ws.Tasks = []domain.Task{task}
		health := dashboardSourceHealth(ws)
		ganttTask := dashboardGanttTask(task, ws, nil, nil, nil, nil, nil, nil)
		openProject := dashboardOpenProject(health, reports.DashboardGanttView{})
		overlay := dashboardOpenProjectOverlay(ws, reports.DashboardOverview{OpenProject: openProject}, true)
		if health.LinkedTasks[0].SourceURL != tc.want || ganttTask.SourceURL != tc.want ||
			openProject.Links[0].OpenProjectURL != tc.want || overlay.WorkPackages[0].WorkPackageURL != tc.want {
			t.Errorf("unexpected display URLs for %q (%q): summary %q, Gantt %q, overview %q, overlay %q", tc.id, tc.raw,
				health.LinkedTasks[0].SourceURL, ganttTask.SourceURL, openProject.Links[0].OpenProjectURL, overlay.WorkPackages[0].WorkPackageURL)
		}
		if ws.Tasks[0].SourceRefs[0] != ref {
			t.Error("report modified source evidence")
		}
	}
	ref := domain.SourceRef{Source: "openproject", ObjectType: "work_package", ObjectID: "42", URL: "https://evidence.example.test/work_packages/42"}
	ws.OpenProjectBindingBaseURL = ""
	if got := reportSourceURL(ref, ws); got != "https://public.example.test/pm/work_packages/42" {
		t.Errorf("profile display URL = %q", got)
	}
	for _, base := range []string{"", "//example.test", "https://user@example.test", "https://example.test/pm?x=1"} {
		ws.Project.SourceProfiles[0].BaseURL = base
		if got := reportSourceURL(ref, ws); got != "" {
			t.Errorf("display URL from unsupported base %q = %q", base, got)
		}
	}
}

func TestOverlayDisplayURLBoundary(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"  HTTPS://example.test/pm/work_packages/42  ", "https://example.test/pm/work_packages/42"},
		{"/api/v3/work_packages/42", ""},
		{"https://example.test/pm/api/v3/work_packages/42", ""},
		{"https://example.test:bad/", ""},
		{"data:", ""},
	} {
		overview := reports.DashboardOverview{OpenProject: reports.DashboardOpenProject{
			Links: []reports.DashboardOpenProjectLink{{TaskID: "task.link", OpenProjectID: "42", OpenProjectURL: tc.raw}},
		}}
		got := dashboardOpenProjectOverlay(domain.Workspace{}, overview, true).WorkPackages[0].WorkPackageURL
		if got != tc.want {
			t.Errorf("overlay URL for %q = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestSourceRefURLWarnings(t *testing.T) {
	bad := domain.SourceRef{Source: "github", ObjectType: "issue", ObjectID: "42", URL: "//example.test/42"}
	ws := domain.Workspace{
		Project: domain.Project{ID: "project.sample", SourceRefs: []domain.SourceRef{bad}},
		Tasks: []domain.Task{
			{ID: "task.bad", SourceRefs: []domain.SourceRef{bad}},
			{ID: "task.api", SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "42", URL: "/pm/api/v3/work_packages/42"}}},
			{ID: "task.empty", SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "42"}}},
			{ID: "task.good", SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "42", URL: "  HTTPS://example.test/work_packages/42  "}}},
		},
		Resources:    []domain.Resource{{ID: "resource.bad", SourceRefs: []domain.SourceRef{bad}}},
		Dependencies: []domain.Dependency{{ID: "dependency.bad", SourceRefs: []domain.SourceRef{bad}}},
		Evidence: []domain.Evidence{
			{ID: "evidence.bad", SourceRef: bad},
			{ID: "evidence.time-entry", SourceRef: domain.SourceRef{Source: "openproject", ObjectType: "time_entry", ObjectID: "77", URL: "/api/v3/time_entries/77"}},
		},
	}
	var result ValidateResult
	validateWorkspace(&result, ws)
	want := map[string]bool{"project.sample": true, "task.bad": true, "resource.bad": true, "dependency.bad": true, "evidence.bad": true}
	for _, warning := range result.Warnings {
		if warning.Code == "invalid_source_ref_url" {
			if !want[warning.Target.ID] {
				t.Errorf("unexpected source URL warning for %s", warning.Target.ID)
			}
			delete(want, warning.Target.ID)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing source URL warnings for %v", want)
	}
	if hasDiagnosticCode(result.Errors, "invalid_source_ref_url") {
		t.Error("SourceRef URLs should only generate warnings")
	}
}
