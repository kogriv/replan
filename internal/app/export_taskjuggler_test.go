package app_test

import (
	"context"
	"errors"
	"reflect"
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
	assertContains(t, result.Content, `project project_replan_mvp "RePlan MVP example" "1.0" 2026-07-05 - 2027-07-06`)
	assertContains(t, result.Content, "  now 2026-07-06\n")
	assertContains(t, result.Content, `resource resource_human_ivan "Ivan"`)
	assertContains(t, result.Content, `limits { weeklymax 32.00h }`)
	assertNotContains(t, result.Content, `note "RePlan class: human"`)
	assertContains(t, result.Content, `task task_op_1234 "Implement validation command"`)
	assertContains(t, result.Content, `effort 10.00h`)
	assertNotContains(t, result.Content, `timingresolution`)
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

func TestExportTaskJugglerProjectWindowIncludesEarlierTaskStarts(t *testing.T) {
	for _, tc := range []struct {
		name      string
		task      domain.Task
		audits    []domain.AuditRecord
		wantStart string
	}{
		{
			name:      "planned start",
			task:      domain.Task{ID: "task.test", Title: "Test", TrackingStatus: "ready", PlannedStart: "2026-11-01"},
			wantStart: "2026-10-31",
		},
		{
			name: "actual start",
			task: domain.Task{ID: "task.test", Title: "Test", TrackingStatus: "in_progress", PlannedStart: "2026-11-07"},
			audits: []domain.AuditRecord{{
				Operation: "actual_add_status", TargetRef: domain.Ref{Kind: "task", ID: "task.test"}, NewStatus: "in_progress", StatusDate: "2026-10-30",
			}},
			wantStart: "2026-10-29",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws := domain.Workspace{
				Project:      domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-11-08"},
				Tasks:        []domain.Task{tc.task},
				AuditRecords: tc.audits,
			}
			result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
			if err != nil {
				t.Fatal(err)
			}
			assertContains(t, result.Content, `"1.0" `+tc.wantStart+` - 2027-11-08 {`)
			assertContains(t, result.Content, "  now 2026-11-08\n")
		})
	}
}

func TestExportTaskJugglerFinishedTasksUseActualThenPlannedDatesWithoutEffort(t *testing.T) {
	remaining := 8.0
	ws := domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-11-08"},
		Resources: []domain.Resource{{ID: "resource.test", Name: "Test"}},
		Tasks: []domain.Task{
			{ID: "task.accepted", Title: "Accepted", TrackingStatus: "accepted", PlannedStart: "2026-11-01", PlannedFinish: "2026-11-02"},
			{ID: "task.done", Title: "Done", TrackingStatus: "done", PlannedStart: "2026-11-03", PlannedFinish: "2026-11-04"},
			{ID: "task.review", Title: "Review ready", TrackingStatus: "review_ready", PlannedStart: "2026-11-05", PlannedFinish: "2026-11-06"},
		},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.accepted", TaskRef: domain.Ref{ID: "task.accepted"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "hours", PlannedAmount: 8, RemainingAmount: &remaining, Status: "active"},
			{ID: "demand.review", TaskRef: domain.Ref{ID: "task.review"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "hours", PlannedAmount: 2, RemainingAmount: domain.Amount(0), Status: "active"},
		},
		AuditRecords: []domain.AuditRecord{
			{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.accepted"}, NewStatus: "in_progress", StatusDate: "2026-10-28"},
			{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.accepted"}, NewStatus: "accepted", StatusDate: "2026-10-31"},
		},
	}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, "task task_accepted \"Accepted\" {\n  start 2026-10-28\n  end 2026-10-31\n}")
	assertContains(t, result.Content, "task task_done \"Done\" {\n  start 2026-11-03\n  end 2026-11-04\n}")
	assertContains(t, result.Content, "task task_review \"Review ready\" {\n  start 2026-11-05\n  end 2026-11-06\n}")
	assertNotContains(t, result.Content, "effort 8.00h")
	assertNotContains(t, result.Content, "allocate resource_test")
}

func TestExportTaskJugglerOpenTaskStartsAtStatusDate(t *testing.T) {
	ws := taskJugglerWorkspaceWithEffort(8)
	ws.Tasks[0].TrackingStatus = "ready"
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, "task replan_status_date \"Status date\" {\n  start 2026-11-08\n  milestone\n  flags replan_internal\n}\n")
	assertContains(t, result.Content, "task task_test \"Test task\" {\n  depends replan_status_date\n  effort 8.00h")
	assertContains(t, result.Content, "  hidetask replan_internal\n")
}

func TestExportTaskJugglerFinishedTaskWithoutDatesUsesStatusDateWarning(t *testing.T) {
	ws := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-11-08"},
		Tasks:   []domain.Task{{ID: "task.accepted", Title: "Accepted", TrackingStatus: "accepted"}},
	}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, "  start 2026-11-08\n  end 2026-11-08\n")
	if warnings := diagnosticsWithCode(result.Warnings, "taskjuggler_finished_task_without_dates"); len(warnings) != 1 || !strings.Contains(warnings[0].Message, "1 finished task") {
		t.Fatalf("finished-task warnings = %#v", warnings)
	}
}

func TestExportTaskJugglerWarnsWhenStatusDateMissing(t *testing.T) {
	ws := domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, `"1.0" today +1y {`)
	assertNotContains(t, result.Content, "  now ")
	assertNotContains(t, result.Content, "replan_status_date")
	assertNotContains(t, result.Content, "replan_internal")
	if !hasDiagnosticCode(result.Warnings, "taskjuggler_status_date_missing") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestExportTaskJugglerChoosesTimingResolutionFromSmallestTaskEffort(t *testing.T) {
	tests := []struct {
		name       string
		effort     float64
		resolution string
		written    string
	}{
		{name: "quarter hour", effort: .25, resolution: "15min", written: "15min"},
		{name: "twenty four minutes", effort: .40, resolution: "20min", written: "24min"},
		{name: "a third of an hour rounds up to whole minutes", effort: 1.0 / 3, resolution: "20min", written: "20min"},
		{name: "a fraction of a minute rounds up", effort: .301, resolution: "15min", written: "19min"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := app.NewExportService(staticWorkspaceLoader{workspace: taskJugglerWorkspaceWithEffort(tt.effort)}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
			if err != nil {
				t.Fatal(err)
			}
			assertContains(t, result.Content, "  timingresolution "+tt.resolution+"\n")
			assertContains(t, result.Content, "  effort "+tt.written+"\n")
			if hasDiagnosticCode(result.Warnings, "taskjuggler_effort_rounded_up") {
				t.Fatalf("unexpected rounded-up warning: %#v", result.Warnings)
			}
		})
	}
}

func TestExportTaskJugglerRoundsEffortBelowFiveMinutes(t *testing.T) {
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: taskJugglerWorkspaceWithEffort(.05)}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, "  timingresolution 5min\n")
	assertContains(t, result.Content, "  effort 5min\n")
	warnings := diagnosticsWithCode(result.Warnings, "taskjuggler_effort_rounded_up")
	if len(warnings) != 1 {
		t.Fatalf("rounded-up warnings = %#v, want one", warnings)
	}
	if warnings[0].Message != "1 exported task effort is below TaskJuggler's 5-minute minimum and was rounded up to 5min (for example task.test)" {
		t.Fatalf("warning message = %q", warnings[0].Message)
	}
	if warnings[0].Target != (app.Target{Kind: "project", ID: "project.test"}) {
		t.Fatalf("warning target = %#v", warnings[0].Target)
	}
}

func taskJugglerWorkspaceWithEffort(effort float64) domain.Workspace {
	return domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-11-08"},
		Resources: []domain.Resource{{ID: "resource.test", Class: "human", Name: "Test resource"}},
		Tasks:     []domain.Task{{ID: "task.test", Title: "Test task"}},
		ResourceDemands: []domain.ResourceDemand{{
			ID: "demand.test", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"},
			ResourceClass: "human", Unit: "hours", PlannedAmount: effort, RemainingAmount: domain.Amount(effort), Status: "active",
		}},
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

func TestExportTaskJugglerGroupsUnsupportedDemandsByUnit(t *testing.T) {
	ws := domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-11-06"},
		Resources: []domain.Resource{{ID: "resource.test", Class: "service", Name: "Test resource"}},
		Tasks:     []domain.Task{{ID: "task.test", Title: "Test task"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.ci.z", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "ci_minutes", PlannedAmount: 1},
			{ID: "demand.slot.b", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "concurrent_slots", PlannedAmount: 1},
			{ID: "demand.ci.a", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "ci_minutes", PlannedAmount: 1},
			{ID: "demand.ci.d", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "ci_minutes", PlannedAmount: 1},
			{ID: "demand.slot.a", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "concurrent_slots", PlannedAmount: 1},
			{ID: "demand.ci.c", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "ci_minutes", PlannedAmount: 1},
			{ID: "demand.cancelled", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "tokens", PlannedAmount: 1, Status: "cancelled"},
		},
	}
	wantUnsupported := []app.TaskJugglerUnsupportedDemandSet{
		{Unit: "ci_minutes", Count: 4, OpenCount: 4, DemandIDs: []string{"demand.ci.a", "demand.ci.c", "demand.ci.d", "demand.ci.z"}},
		{Unit: "concurrent_slots", Count: 2, OpenCount: 2, DemandIDs: []string{"demand.slot.a", "demand.slot.b"}},
	}

	for _, raw := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "raw"}[raw], func(t *testing.T) {
			result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{Raw: raw})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(result.UnsupportedDemands, wantUnsupported) {
				t.Fatalf("UnsupportedDemands = %#v, want %#v", result.UnsupportedDemands, wantUnsupported)
			}
			warnings := diagnosticsWithCode(result.Warnings, "taskjuggler_unsupported_demand_unit")
			if len(warnings) != 2 {
				t.Fatalf("unsupported warnings = %#v, want two", warnings)
			}
			if warnings[0].Target.Kind != "unit" || warnings[0].Target.ID != "ci_minutes" || warnings[1].Target.Kind != "unit" || warnings[1].Target.ID != "concurrent_slots" {
				t.Fatalf("unsupported warning targets = %#v", warnings)
			}
			if warnings[0].Message != `4 resource demands with open work use unit "ci_minutes", which TaskJuggler effort cannot represent; they are not exported (for example demand.ci.a, demand.ci.c, demand.ci.d)` {
				t.Fatalf("first warning message = %q", warnings[0].Message)
			}
			if warnings[1].Message != `2 resource demands with open work use unit "concurrent_slots", which TaskJuggler effort cannot represent; they are not exported (for example demand.slot.a, demand.slot.b)` {
				t.Fatalf("second warning message = %q", warnings[1].Message)
			}
		})
	}
}

func TestExportTaskJugglerUnsupportedDemandWarningsCountOnlyOpenWork(t *testing.T) {
	ws := domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-11-06"},
		Resources: []domain.Resource{{ID: "resource.test", Class: "service", Name: "Test resource"}},
		Tasks: []domain.Task{
			{ID: "task.finished", Title: "Finished", TrackingStatus: "accepted"},
			{ID: "task.open", Title: "Open", TrackingStatus: "in_progress"},
		},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.consumed", TaskRef: domain.Ref{ID: "task.finished"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "ci_minutes", PlannedAmount: 10, RemainingAmount: domain.Amount(0), Status: "consumed"},
			{ID: "demand.open", TaskRef: domain.Ref{ID: "task.open"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "tokens", PlannedAmount: 100, RemainingAmount: domain.Amount(50), Status: "active"},
		},
	}

	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	want := []app.TaskJugglerUnsupportedDemandSet{
		{Unit: "ci_minutes", Count: 1, OpenCount: 0, DemandIDs: []string{"demand.consumed"}},
		{Unit: "tokens", Count: 1, OpenCount: 1, DemandIDs: []string{"demand.open"}},
	}
	if !reflect.DeepEqual(result.UnsupportedDemands, want) {
		t.Fatalf("UnsupportedDemands = %#v, want %#v", result.UnsupportedDemands, want)
	}
	warnings := diagnosticsWithCode(result.Warnings, "taskjuggler_unsupported_demand_unit")
	if len(warnings) != 1 {
		t.Fatalf("unsupported warnings = %#v, want one", warnings)
	}
	if warnings[0].Target != (app.Target{Kind: "unit", ID: "tokens"}) {
		t.Fatalf("warning target = %#v", warnings[0].Target)
	}
	if warnings[0].Message != `1 resource demand uses unit "tokens", which TaskJuggler effort cannot represent; it is not exported (for example demand.open)` {
		t.Fatalf("warning message = %q", warnings[0].Message)
	}
}

func TestExportTaskJugglerKeepsSupportedAndPerDemandWarnings(t *testing.T) {
	ws := domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-11-06"},
		Resources: []domain.Resource{{ID: "resource.human", Class: "human", Name: "Human"}},
		Tasks:     []domain.Task{{ID: "task.test", Title: "Test task"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.supported", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.human"}, Unit: "hours", PlannedAmount: 2},
			{ID: "demand.no-resource", TaskRef: domain.Ref{ID: "task.test"}, Unit: "hours", PlannedAmount: 1},
			{ID: "demand.unknown", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.unknown"}, Unit: "hours", PlannedAmount: 1},
			{ID: "demand.unsupported", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.human"}, Unit: "review_hours", PlannedAmount: 1},
		},
	}

	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, `effort 2.00h`)
	assertContains(t, result.Content, `allocate resource_human`)
	if !hasDiagnosticTarget(result.Warnings, "taskjuggler_demand_without_concrete_resource", "resource_demand", "demand.no-resource") {
		t.Fatalf("missing per-demand no-resource warning: %#v", result.Warnings)
	}
	if !hasDiagnosticTarget(result.Warnings, "taskjuggler_unknown_resource", "resource_demand", "demand.unknown") {
		t.Fatalf("missing per-demand unknown-resource warning: %#v", result.Warnings)
	}
	if warnings := diagnosticsWithCode(result.Warnings, "taskjuggler_unsupported_demand_unit"); len(warnings) != 1 || warnings[0].Target != (app.Target{Kind: "unit", ID: "review_hours"}) {
		t.Fatalf("unsupported warnings = %#v", warnings)
	}
}

func TestExportTaskJugglerWarnsOnlyForOpenTasksNeedingDemandAttention(t *testing.T) {
	ws := domain.Workspace{
		Project:   domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-11-06"},
		Resources: []domain.Resource{{ID: "resource.test", Class: "human", Name: "Test resource"}},
		Tasks: []domain.Task{
			{ID: "task.finished", Title: "Finished", TrackingStatus: "accepted"},
			{ID: "task.unsupported", Title: "Unsupported", TrackingStatus: "in_progress"},
			{ID: "task.no-demand.z", Title: "No demand Z", TrackingStatus: "ready"},
			{ID: "task.no-demand.a", Title: "No demand A", TrackingStatus: "not_started"},
			{ID: "task.terminal-no-demand", Title: "Cancelled", TrackingStatus: "cancelled"},
			{ID: "task.cancelled-demand", Title: "Cancelled demand", TrackingStatus: "ready"},
		},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.finished", TaskRef: domain.Ref{ID: "task.finished"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "hours", PlannedAmount: 2, RemainingAmount: domain.Amount(0), Status: "consumed"},
			{ID: "demand.unsupported", TaskRef: domain.Ref{ID: "task.unsupported"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "ci_minutes", PlannedAmount: 10, Status: "active"},
			{ID: "demand.cancelled", TaskRef: domain.Ref{ID: "task.cancelled-demand"}, ResourceRef: domain.Ref{ID: "resource.test"}, Unit: "ci_minutes", PlannedAmount: 10, Status: "cancelled"},
		},
	}

	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if hasDiagnosticTarget(result.Warnings, "taskjuggler_task_without_supported_demands", "task", "task.finished") {
		t.Fatalf("finished task received unsupported-demands warning: %#v", result.Warnings)
	}
	if hasDiagnosticTarget(result.Warnings, "taskjuggler_task_without_supported_demands", "task", "task.cancelled-demand") {
		t.Fatalf("task with no loading demands received unsupported-demands warning: %#v", result.Warnings)
	}
	if !hasDiagnosticTarget(result.Warnings, "taskjuggler_task_without_supported_demands", "task", "task.unsupported") {
		t.Fatalf("open task with unsupported loading demand did not receive warning: %#v", result.Warnings)
	}
	grouped := diagnosticsWithCode(result.Warnings, "taskjuggler_tasks_without_demands")
	if len(grouped) != 1 {
		t.Fatalf("tasks-without-demands warnings = %#v, want one", grouped)
	}
	if grouped[0].Message != "2 non-terminal tasks have no resource demands; no effort is exported for them (for example task.no-demand.a, task.no-demand.z)" {
		t.Fatalf("grouped warning message = %q", grouped[0].Message)
	}
	if grouped[0].Target != (app.Target{Kind: "project", ID: "project.test"}) {
		t.Fatalf("grouped warning target = %#v", grouped[0].Target)
	}
	assertContains(t, result.Content, `task task_finished "Finished"`)
	assertContains(t, result.Content, `task task_unsupported "Unsupported"`)
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
	assertContains(t, result.Content, `effort 45min`)
	assertContains(t, result.Content, `timingresolution 30min`)
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
	assertContains(t, result.Content, `effort 45min`)
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
	assertNotContains(t, result.Content, `effort 45min`)
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

func TestExportTaskJugglerExportsOrderingDependencies(t *testing.T) {
	for _, dependencyType := range []string{"finish_to_start", "blocks", "requires_input_from", "requires_resource_from"} {
		t.Run(dependencyType, func(t *testing.T) {
			ws := taskJugglerDependencyWorkspace()
			ws.Dependencies = []domain.Dependency{{
				ID: "dependency.ordering", Type: dependencyType, Status: "open", Criticality: "required",
				PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
			}}
			result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
			if err != nil {
				t.Fatalf("TaskJuggler returned error: %v; errors=%#v", err, result.Errors)
			}
			assertContains(t, result.Content, "task task_b \"B\" {\n  depends task_a\n")
		})
	}
}

func TestExportTaskJugglerSkipsInactiveDependencies(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Dependencies = []domain.Dependency{
		{ID: "dependency.resolved", Type: "finish_to_start", Status: "resolved", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
		{ID: "dependency.cancelled", Type: "blocks", Status: "cancelled", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
	}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertNotContains(t, result.Content, "depends task_")
	if hasDiagnosticCode(result.Warnings, "taskjuggler_dependencies_not_exported") {
		t.Fatalf("inactive dependencies should not warn: %#v", result.Warnings)
	}
}

func TestExportTaskJugglerDeduplicatesAndSortsPredecessors(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Tasks = append(ws.Tasks, domain.Task{ID: "task.c", Title: "C", TrackingStatus: "ready"})
	ws.Dependencies = []domain.Dependency{
		{ID: "dependency.c-b", Type: "finish_to_start", Status: "open", PredecessorRef: domain.Ref{Kind: "task", ID: "task.c"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
		{ID: "dependency.a-b.blocks", Type: "blocks", Status: "blocked", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
		{ID: "dependency.a-b.input", Type: "requires_input_from", Status: "unknown", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
	}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(result.Content, "  depends task_a\n") != 1 {
		t.Fatalf("duplicate pair was not collapsed:\n%s", result.Content)
	}
	assertContains(t, result.Content, "task task_b \"B\" {\n  depends task_a\n  depends task_c\n")
}

func TestExportTaskJugglerWarnsOnceForDependenciesNotExported(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Dependencies = []domain.Dependency{
		{ID: "dependency.external", Type: "external_dependency", Status: "open", SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
		{ID: "dependency.missing-predecessor", Type: "finish_to_start", Status: "blocked", PredecessorRef: domain.Ref{Kind: "task", ID: "task.missing"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
		{ID: "dependency.missing-successor", Type: "requires_resource_from", Status: "unknown", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "resource", ID: "resource.external"}},
	}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	warnings := diagnosticsWithCode(result.Warnings, "taskjuggler_dependencies_not_exported")
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want one count warning", warnings)
	}
	want := "3 active dependencies cannot be represented in the TaskJuggler export (external_dependency: 1, predecessor not a task in the export: 1, successor not a task in the export: 1); they are not exported (for example dependency.external, dependency.missing-predecessor, dependency.missing-successor)"
	if warnings[0].Message != want {
		t.Fatalf("message = %q, want %q", warnings[0].Message, want)
	}
}

func TestExportTaskJugglerRefusesDependencyCycle(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Dependencies = []domain.Dependency{
		{ID: "dependency.a-b", Type: "finish_to_start", Status: "open", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
		{ID: "dependency.b-a", Type: "blocks", Status: "open", PredecessorRef: domain.Ref{Kind: "task", ID: "task.b"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.a"}},
	}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	cycleErrors := diagnosticsWithCode(result.Errors, "taskjuggler_dependency_cycle")
	if len(cycleErrors) != 1 || !strings.Contains(cycleErrors[0].Message, "task.a -> task.b -> task.a") {
		t.Fatalf("cycle errors = %#v", cycleErrors)
	}
	if result.Content != "" {
		t.Fatalf("content generated for cyclic dependencies:\n%s", result.Content)
	}
}

func TestExportTaskJugglerRefusesDependencySelfLoop(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Dependencies = []domain.Dependency{{
		ID: "dependency.self", Type: "finish_to_start", Status: "open",
		PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.a"},
	}}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	cycleErrors := diagnosticsWithCode(result.Errors, "taskjuggler_dependency_cycle")
	if len(cycleErrors) != 1 || !strings.Contains(cycleErrors[0].Message, "task.a -> task.a") {
		t.Fatalf("cycle errors = %#v", cycleErrors)
	}
}

func TestExportTaskJugglerHonoursScenarioDependencyStatus(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Dependencies = []domain.Dependency{{
		ID: "dependency.a-b", Type: "finish_to_start", Status: "resolved",
		PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
	}}
	ws.Scenarios = []domain.Scenario{{
		ID: "scenario.reopen-dependency", Status: "proposed",
		Changes: []domain.ScenarioChange{{Op: "set", Path: "dependencies.dependency.a-b.status", Value: "open"}},
	}}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{ScenarioID: "scenario.reopen-dependency"})
	if err != nil {
		t.Fatalf("TaskJuggler returned error: %v; errors=%#v", err, result.Errors)
	}
	assertContains(t, result.Content, "task task_b \"B\" {\n  depends task_a\n")
}

func TestExportTaskJugglerFinishedSuccessorKeepsItsDatesWithoutDepends(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Tasks[1].TrackingStatus = "accepted"
	ws.Tasks[1].PlannedStart, ws.Tasks[1].PlannedFinish = "2026-07-01", "2026-07-02"
	ws.Dependencies = []domain.Dependency{
		{ID: "dependency.open", Type: "finish_to_start", Status: "open", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
	}
	result, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if err != nil {
		t.Fatal(err)
	}
	assertContains(t, result.Content, "task task_b \"B\" {\n  start 2026-07-01\n  end 2026-07-02\n}")
	assertContains(t, result.Content, "task task_a \"A\" {\n  depends replan_status_date\n}")
}

func TestExportTaskJugglerReservesTheStatusDateTaskID(t *testing.T) {
	ws := taskJugglerDependencyWorkspace()
	ws.Tasks = append(ws.Tasks, domain.Task{ID: "replan.status-date", Title: "Clash", TrackingStatus: "ready"})
	_, err := app.NewExportService(staticWorkspaceLoader{workspace: ws}).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{})
	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("err = %v, want validation failure for the reserved id", err)
	}
}

func taskJugglerDependencyWorkspace() domain.Workspace {
	return domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-07-10"},
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "ready"},
			{ID: "task.b", Title: "B", TrackingStatus: "ready"},
		},
	}
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

func diagnosticsWithCode(diags []app.Diagnostic, code string) []app.Diagnostic {
	var matches []app.Diagnostic
	for _, diag := range diags {
		if diag.Code == code {
			matches = append(matches, diag)
		}
	}
	return matches
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
