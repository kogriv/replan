package app_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func taskAddRequest() app.TaskAddRequest {
	return app.TaskAddRequest{
		ID: "task.op.new-command", Title: "Exercise task command", Description: "Created in a test.", Type: "implementation",
		PlannedStart: "2026-07-13", PlannedFinish: "2026-07-14", Tags: []string{"cli", "test"}, AssigneeID: "resource.human.ivan",
		DemandSpecs: []string{"implementer=resource.human.ivan:3:hours", "agent=resource.ai.codex.local:2:agent_hours:ai_coding_agent"},
		ActorID:     "actor.human.operator", EvidenceID: "evidence.github.pr.45", OccurredAt: "2026-10-26T12:00:00+07:00",
	}
}

func TestTaskAddWritesThreeCanonicalFilesAuditAndValidates(t *testing.T) {
	project := copyAppProjectFixture(t)
	beforeTasks := readTaskTestFile(t, project, "canonical/tasks.yaml")
	beforeDemands := readTaskTestFile(t, project, "canonical/resource_demands.yaml")
	beforePlan := readTaskTestFile(t, project, "canonical/current_plan.yaml")
	req := taskAddRequest()
	result, err := app.NewTaskService(filestore.New(project)).Add(context.Background(), req)
	if err != nil {
		t.Fatalf("Add: %v; diagnostics=%#v", err, result.Errors)
	}
	if !result.Applied || result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("not applied and validated: %#v", result)
	}
	wantFiles := []string{"canonical/current_plan.yaml", "canonical/resource_demands.yaml", "canonical/tasks.yaml", "audit/audit.2026-10.jsonl"}
	if !reflect.DeepEqual(result.WrittenFiles, wantFiles) {
		t.Fatalf("written files = %#v, want %#v", result.WrittenFiles, wantFiles)
	}
	afterTasks := readTaskTestFile(t, project, "canonical/tasks.yaml")
	afterDemands := readTaskTestFile(t, project, "canonical/resource_demands.yaml")
	if !strings.HasPrefix(afterTasks, beforeTasks) {
		t.Fatal("task append changed existing task bytes")
	}
	if !strings.HasPrefix(afterDemands, beforeDemands) {
		t.Fatal("demand append changed existing demand bytes")
	}
	for _, want := range []string{"id: task.op.new-command", "tracking_status: not_started", "resource_demand.new-command.implementer", "resource_demand.new-command.agent"} {
		if !strings.Contains(afterTasks+afterDemands, want) {
			t.Errorf("written yaml missing %q", want)
		}
	}
	if !strings.Contains(afterDemands, "remaining_amount: 3") || !strings.Contains(afterDemands, "role: implementer") {
		t.Fatal("planned/remaining amount or default role missing")
	}
	plan := readTaskTestFile(t, project, "canonical/current_plan.yaml")
	if !strings.HasSuffix(strings.TrimSpace(plan), "id: task.op.new-command") {
		t.Fatal("task was not appended to current plan")
	}
	if strings.TrimSuffix(plan, "    - kind: task\n      id: task.op.new-command\n") != beforePlan {
		t.Fatal("current-plan append changed existing bytes")
	}
	audit := readTaskTestFile(t, project, "audit/audit.2026-10.jsonl")
	for _, want := range []string{`"operation":"task_add"`, `"id":"evidence.github.pr.45"`, `"result":"success"`} {
		if !strings.Contains(audit, want) {
			t.Errorf("audit missing %s", want)
		}
	}
}

func TestTaskAddInsertsAfterAndPreservesPlanBytes(t *testing.T) {
	project := copyAppProjectFixture(t)
	before := readTaskTestFile(t, project, "canonical/current_plan.yaml")
	req := taskAddRequest()
	req.AfterTaskID = "task.offline.repair-pump"
	result, err := app.NewTaskService(filestore.New(project)).Add(context.Background(), req)
	if err != nil {
		t.Fatalf("Add: %v; %#v", err, result.Errors)
	}
	plan := readTaskTestFile(t, project, "canonical/current_plan.yaml")
	want := "id: task.offline.repair-pump\n    - kind: task\n      id: task.op.new-command\n    - kind: task\n      id: task.op.1234"
	if !strings.Contains(plan, want) {
		t.Fatalf("wrong plan position:\n%s", plan)
	}
	inserted := "    - kind: task\n      id: task.op.new-command\n"
	if strings.Replace(plan, inserted, "", 1) != before {
		t.Fatal("current-plan insertion changed existing bytes")
	}
}

func TestTaskAddDryRunWritesNothing(t *testing.T) {
	project := copyAppProjectFixture(t)
	before := snapshotTaskProject(t, project)
	req := taskAddRequest()
	req.DryRun = true
	result, err := app.NewTaskService(filestore.New(project)).Add(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied || len(result.PlannedWrites) != 4 {
		t.Fatalf("unexpected dry run: %#v", result)
	}
	if !reflect.DeepEqual(before, snapshotTaskProject(t, project)) {
		t.Fatal("dry run changed project")
	}
}

func TestTaskAddRefusals(t *testing.T) {
	tests := []struct {
		name, code string
		edit       func(*app.TaskAddRequest)
	}{
		{"duplicate task", "task_already_exists", func(r *app.TaskAddRequest) { r.ID = "task.op.1234" }},
		{"duplicate generated demand", "duplicate_demand_id", func(r *app.TaskAddRequest) { r.DemandSpecs = append(r.DemandSpecs, r.DemandSpecs[0]) }},
		{"unknown resource", "resource_not_found", func(r *app.TaskAddRequest) { r.DemandSpecs = []string{"x=resource.missing:1:hours"} }},
		{"unknown assignee", "assignee_not_found", func(r *app.TaskAddRequest) { r.AssigneeID = "resource.missing" }},
		{"unknown actor", "actor_not_found", func(r *app.TaskAddRequest) { r.ActorID = "actor.missing" }},
		{"unknown evidence", "evidence_not_found", func(r *app.TaskAddRequest) { r.EvidenceID = "evidence.missing" }},
		{"unknown after", "after_task_not_found", func(r *app.TaskAddRequest) { r.AfterTaskID = "task.missing" }},
		{"bad unit", "invalid_resource_unit", func(r *app.TaskAddRequest) { r.DemandSpecs = []string{"x=resource.human.ivan:1:tokens"} }},
		{"bad amount", "invalid_demand_amount", func(r *app.TaskAddRequest) { r.DemandSpecs = []string{"x=resource.human.ivan:zero:hours"} }},
		{"bad spec", "invalid_demand_spec", func(r *app.TaskAddRequest) { r.DemandSpecs = []string{"broken"} }},
		{"bad start", "invalid_planned_start", func(r *app.TaskAddRequest) { r.PlannedStart = "2026-02-30" }},
		{"finish before start", "planned_finish_before_start", func(r *app.TaskAddRequest) { r.PlannedFinish = "2026-07-12" }},
		{"no demand", "missing_demand", func(r *app.TaskAddRequest) { r.DemandSpecs = nil }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			project := copyAppProjectFixture(t)
			req := taskAddRequest()
			tc.edit(&req)
			before := snapshotTaskProject(t, project)
			result, err := app.NewTaskService(filestore.New(project)).Add(context.Background(), req)
			if err == nil {
				t.Fatal("expected refusal")
			}
			assertTaskDiag(t, result.Errors, tc.code)
			if !reflect.DeepEqual(before, snapshotTaskProject(t, project)) {
				t.Fatal("refusal wrote files")
			}
		})
	}
}

func TestTaskSetDatesSuccessDryRunAndRefusals(t *testing.T) {
	project := copyAppProjectFixture(t)
	req := app.TaskSetDatesRequest{TaskID: "task.op.1234", PlannedStart: "2026-07-14", PlannedFinish: "2026-07-16", Reason: "Accept scheduler shift.", ActorID: "actor.human.operator", EvidenceID: "evidence.github.pr.45", OccurredAt: "2026-10-26T13:00:00+07:00"}
	before := readTaskTestFile(t, project, "canonical/tasks.yaml")
	result, err := app.NewTaskService(filestore.New(project)).SetDates(context.Background(), req)
	if err != nil {
		t.Fatalf("SetDates: %v; %#v", err, result.Errors)
	}
	after := readTaskTestFile(t, project, "canonical/tasks.yaml")
	if !strings.Contains(after, `planned_start: "2026-07-14"`) || !strings.Contains(after, `planned_finish: "2026-07-16"`) {
		t.Fatal("dates not changed")
	}
	if !strings.Contains(before, "    title: Replace greenhouse pump\n") || !strings.Contains(after, "    title: Replace greenhouse pump\n") {
		t.Fatal("unrelated task bytes not preserved")
	}
	audit := readTaskTestFile(t, project, "audit/audit.2026-10.jsonl")
	for _, want := range []string{`"operation":"task_set_dates"`, `Accept scheduler shift. Planned dates: start (unset) -\u003e 2026-07-14; finish (unset) -\u003e 2026-07-16.`} {
		if !strings.Contains(audit, want) {
			t.Errorf("audit missing %q: %s", want, audit)
		}
	}

	dryProject := copyAppProjectFixture(t)
	dryBefore := snapshotTaskProject(t, dryProject)
	req.DryRun = true
	dry, err := app.NewTaskService(filestore.New(dryProject)).SetDates(context.Background(), req)
	if err != nil || dry.Applied || len(dry.PlannedWrites) != 2 {
		t.Fatalf("bad dry run: %#v %v", dry, err)
	}
	if !reflect.DeepEqual(dryBefore, snapshotTaskProject(t, dryProject)) {
		t.Fatal("date dry run wrote")
	}

	refusals := []struct {
		name, code string
		edit       func(*app.TaskSetDatesRequest)
	}{
		{"unknown task", "task_not_found", func(r *app.TaskSetDatesRequest) { r.TaskID = "task.missing" }}, {"no date", "missing_planned_date", func(r *app.TaskSetDatesRequest) { r.PlannedStart = ""; r.PlannedFinish = "" }}, {"bad date", "invalid_planned_start", func(r *app.TaskSetDatesRequest) { r.PlannedStart = "tomorrow" }}, {"finish before start", "planned_finish_before_start", func(r *app.TaskSetDatesRequest) { r.PlannedStart = "2026-07-20"; r.PlannedFinish = "2026-07-19" }}, {"missing reason", "missing_reason", func(r *app.TaskSetDatesRequest) { r.Reason = "" }}, {"unknown actor", "actor_not_found", func(r *app.TaskSetDatesRequest) { r.ActorID = "actor.missing" }}, {"unknown evidence", "evidence_not_found", func(r *app.TaskSetDatesRequest) { r.EvidenceID = "evidence.missing" }},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			p := copyAppProjectFixture(t)
			r := req
			r.DryRun = false
			tc.edit(&r)
			res, e := app.NewTaskService(filestore.New(p)).SetDates(context.Background(), r)
			if e == nil {
				t.Fatal("expected refusal")
			}
			assertTaskDiag(t, res.Errors, tc.code)
		})
	}
}

func TestTaskRecordActualWindowDerivationRefusalsAndWarnings(t *testing.T) {
	base := domain.Workspace{
		Project: domain.Project{ID: "project.test"},
		Tasks:   []domain.Task{{ID: "task.done", Title: "Done", TrackingStatus: "accepted", PlannedStart: "2026-11-01", PlannedFinish: "2026-11-02"}},
		Actors:  []domain.Actor{{ID: "actor.test"}}, Evidence: []domain.Evidence{{ID: "evidence.test"}},
		ResourceDemands: []domain.ResourceDemand{{ID: "demand.current", TaskRef: domain.Ref{ID: "task.done"}}},
	}
	req := app.TaskRecordActualWindowRequest{TaskID: "task.done", ActorID: "actor.test", EvidenceID: "evidence.test", DryRun: true}

	t.Run("audit", func(t *testing.T) {
		ws := base
		ws.AuditRecords = []domain.AuditRecord{
			{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.done"}, NewStatus: "in_progress", StatusDate: "2026-10-02"},
			{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.done"}, NewStatus: "accepted", StatusDate: "2026-10-05"},
		}
		result, err := app.NewTaskService(stubLoader{ws: ws}).RecordActualWindow(context.Background(), req)
		if err != nil || result.NewStart != "2026-10-02" || result.NewFinish != "2026-10-05" || result.ActualStartSource != "audit" || result.ActualFinishSource != "audit" {
			t.Fatalf("result=%#v err=%v", result, err)
		}
	})

	t.Run("consumption and start audit fallback", func(t *testing.T) {
		ws := base
		ws.AuditRecords = []domain.AuditRecord{{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.done"}, NewStatus: "in_progress", StatusDate: "2026-10-02"}}
		ws.Actuals = []domain.Actual{{Type: "work_consumed", DemandRef: domain.Ref{ID: "demand.current"}, StatusDate: "2026-10-04"}}
		result, err := app.NewTaskService(stubLoader{ws: ws}).RecordActualWindow(context.Background(), req)
		if err != nil || result.NewStart != "2026-10-02" || result.NewFinish != "2026-10-04" || result.ActualStartSource != "audit" || result.ActualFinishSource != "consumption_actual" {
			t.Fatalf("result=%#v err=%v", result, err)
		}
	})

	t.Run("finish only falls back", func(t *testing.T) {
		ws := base
		ws.AuditRecords = []domain.AuditRecord{{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.done"}, NewStatus: "accepted", StatusDate: "2026-10-05"}}
		result, err := app.NewTaskService(stubLoader{ws: ws}).RecordActualWindow(context.Background(), req)
		if err != nil || result.NewStart != "2026-10-05" || result.NewFinish != "2026-10-05" || result.ActualStartSource != "audit" {
			t.Fatalf("result=%#v err=%v", result, err)
		}
	})

	for _, tc := range []struct {
		name, code string
		edit       func(*domain.Workspace, *app.TaskRecordActualWindowRequest)
	}{
		{"unknown task", "task_not_found", func(_ *domain.Workspace, r *app.TaskRecordActualWindowRequest) { r.TaskID = "task.missing" }},
		{"not accepted", "task_not_finished", func(ws *domain.Workspace, _ *app.TaskRecordActualWindowRequest) {
			ws.Tasks[0].TrackingStatus = "in_progress"
		}},
		{"no dates", "actual_window_unknown", func(_ *domain.Workspace, _ *app.TaskRecordActualWindowRequest) {}},
		{"unknown evidence", "evidence_not_found", func(_ *domain.Workspace, r *app.TaskRecordActualWindowRequest) { r.EvidenceID = "evidence.missing" }},
		{"inconsistent", "actual_window_inconsistent", func(ws *domain.Workspace, _ *app.TaskRecordActualWindowRequest) {
			ws.AuditRecords = []domain.AuditRecord{{Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.done"}, NewStatus: "in_progress", StatusDate: "2026-10-06"}, {Operation: "actual_add_status", TargetRef: domain.Ref{ID: "task.done"}, NewStatus: "accepted", StatusDate: "2026-10-05"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ws, request := base, req
			ws.Tasks = append([]domain.Task(nil), base.Tasks...)
			tc.edit(&ws, &request)
			result, err := app.NewTaskService(stubLoader{ws: ws}).RecordActualWindow(context.Background(), request)
			if err == nil {
				t.Fatal("expected refusal")
			}
			assertTaskDiag(t, result.Errors, tc.code)
		})
	}

	t.Run("unchanged and non-working warning", func(t *testing.T) {
		ws := base
		ws.Tasks = append([]domain.Task(nil), base.Tasks...)
		ws.Tasks[0].PlannedStart, ws.Tasks[0].PlannedFinish = "2026-10-03", "2026-10-04"
		ws.Actuals = []domain.Actual{{Type: "consumption", TaskRef: domain.Ref{ID: "task.done"}, StatusDate: "2026-10-03"}, {Type: "consumption", TaskRef: domain.Ref{ID: "task.done"}, StatusDate: "2026-10-04"}}
		ws.Availabilities = []domain.Availability{{ResourceRef: domain.Ref{ID: "resource.test"}, Windows: []domain.AvailabilityWindow{{Days: []string{"monday", "tuesday", "wednesday", "thursday", "friday"}}}}}
		result, err := app.NewTaskService(stubLoader{ws: ws}).RecordActualWindow(context.Background(), req)
		if err != nil || result.Changed || len(result.PlannedWrites) != 0 || result.AuditRecordPreview != nil || len(result.Warnings) != 2 {
			t.Fatalf("result=%#v err=%v", result, err)
		}
	})
}

func TestTaskRecordActualWindowDryRunAndApply(t *testing.T) {
	prepare := func(t *testing.T) string {
		project := copyAppProjectFixture(t)
		path := filepath.Join(project, "canonical/tasks.yaml")
		text := readTaskTestFile(t, project, "canonical/tasks.yaml")
		text = strings.Replace(text, "tracking_status: ready", "tracking_status: accepted", 1)
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		actual := "actuals:\n  - id: actual.task-window-test\n    type: work_consumed\n    task_ref: {kind: task, id: task.op.1234}\n    amount: 1\n    unit: hours\n    status_date: 2026-10-03\n    accepted_by: {kind: actor, id: actor.human.operator}\n    evidence_refs: [{kind: evidence, id: evidence.github.pr.45}]\n"
		if err := os.WriteFile(filepath.Join(project, "actuals/actual.task-window-test.yaml"), []byte(actual), 0o600); err != nil {
			t.Fatal(err)
		}
		return project
	}
	req := app.TaskRecordActualWindowRequest{TaskID: "task.op.1234", ActorID: "actor.human.operator", EvidenceID: "evidence.github.pr.45", OccurredAt: "2026-10-28T15:00:00+07:00"}

	dryProject := prepare(t)
	before := snapshotTaskProject(t, dryProject)
	dryReq := req
	dryReq.DryRun = true
	dry, err := app.NewTaskService(filestore.New(dryProject)).RecordActualWindow(context.Background(), dryReq)
	if err != nil || dry.Applied || !dry.Changed || len(dry.PlannedWrites) != 2 || !reflect.DeepEqual(before, snapshotTaskProject(t, dryProject)) {
		t.Fatalf("dry=%#v err=%v", dry, err)
	}

	project := prepare(t)
	result, err := app.NewTaskService(filestore.New(project)).RecordActualWindow(context.Background(), req)
	if err != nil || !result.Applied || result.NewStart != "2026-10-03" || result.NewFinish != "2026-10-03" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	tasks := readTaskTestFile(t, project, "canonical/tasks.yaml")
	if !strings.Contains(tasks, `planned_start: "2026-10-03"`) || !strings.Contains(tasks, `planned_finish: "2026-10-03"`) {
		t.Fatalf("tasks not updated: %s", tasks)
	}
	audit := readTaskTestFile(t, project, "audit/audit.2026-10.jsonl")
	if strings.Count(audit, `"operation":"task_record_actual_window"`) != 1 || !strings.Contains(audit, `"actual_start_source":"consumption_actual"`) {
		t.Fatalf("audit missing actual window: %s", audit)
	}
}

func TestTaskSetTitleApplyDryRunNoOpAndRefusals(t *testing.T) {
	req := app.TaskSetTitleRequest{TaskID: "task.op.1234", Title: "Replace greenhouse pump safely", Reason: "Correct the task title.", ActorID: "actor.human.operator", EvidenceID: "evidence.github.pr.45", OccurredAt: "2026-10-28T13:00:00+07:00"}
	project := copyAppProjectFixture(t)
	result, err := app.NewTaskService(filestore.New(project)).SetTitle(context.Background(), req)
	if err != nil || !result.Applied || result.PreviousTitle != "Implement validation command" || result.NewTitle != req.Title {
		t.Fatalf("SetTitle: result=%#v err=%v", result, err)
	}
	tasks := readTaskTestFile(t, project, "canonical/tasks.yaml")
	if !strings.Contains(tasks, "title: Replace greenhouse pump safely") {
		t.Fatal("title not changed")
	}
	audit := readTaskTestFile(t, project, "audit/audit.2026-10.jsonl")
	for _, want := range []string{`"operation":"task_set_title"`, `Correct the task title. Task title: \"Implement validation command\" -\u003e \"Replace greenhouse pump safely\".`} {
		if !strings.Contains(audit, want) {
			t.Errorf("audit missing %q: %s", want, audit)
		}
	}

	dryProject := copyAppProjectFixture(t)
	before := snapshotTaskProject(t, dryProject)
	dryReq := req
	dryReq.DryRun = true
	dry, err := app.NewTaskService(filestore.New(dryProject)).SetTitle(context.Background(), dryReq)
	if err != nil || dry.Applied || dry.NoOp || len(dry.PlannedWrites) != 2 {
		t.Fatalf("bad dry run: %#v %v", dry, err)
	}
	if !reflect.DeepEqual(before, snapshotTaskProject(t, dryProject)) {
		t.Fatal("title dry run wrote")
	}

	noOpReq := req
	noOpReq.Title = "Implement validation command"
	noOp, err := app.NewTaskService(filestore.New(dryProject)).SetTitle(context.Background(), noOpReq)
	if err != nil || !noOp.NoOp || noOp.Applied || len(noOp.PlannedWrites) != 0 || len(noOp.WrittenFiles) != 0 {
		t.Fatalf("bad no-op: %#v %v", noOp, err)
	}
	if !reflect.DeepEqual(before, snapshotTaskProject(t, dryProject)) {
		t.Fatal("title no-op wrote")
	}

	refusals := []struct {
		name, code string
		edit       func(*app.TaskSetTitleRequest)
	}{
		{"unknown task", "task_not_found", func(r *app.TaskSetTitleRequest) { r.TaskID = "task.missing" }},
		{"missing evidence", "missing_evidence", func(r *app.TaskSetTitleRequest) { r.EvidenceID = "" }},
		{"empty title", "missing_task_title", func(r *app.TaskSetTitleRequest) { r.Title = " \t" }},
	}
	for _, tc := range refusals {
		t.Run(tc.name, func(t *testing.T) {
			p := copyAppProjectFixture(t)
			r := req
			tc.edit(&r)
			res, e := app.NewTaskService(filestore.New(p)).SetTitle(context.Background(), r)
			if e == nil {
				t.Fatal("expected refusal")
			}
			assertTaskDiag(t, res.Errors, tc.code)
		})
	}
}

func readTaskTestFile(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func assertTaskDiag(t *testing.T, ds []app.Diagnostic, code string) {
	t.Helper()
	for _, d := range ds {
		if d.Code == code {
			return
		}
	}
	t.Fatalf("diagnostics %#v missing %s", ds, code)
}
func snapshotTaskProject(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if e.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	return out
}
