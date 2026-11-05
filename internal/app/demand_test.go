package app_test

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

const demandEvidence = "evidence.github.pr.45"
const demandActor = "actor.human.operator"

func TestDemandAddSetMoveApplyAndDryRun(t *testing.T) {
	project := copyProjectFixture(t)
	service := app.NewDemandService(filestore.New(project))
	add, err := service.Add(context.Background(), app.DemandAddRequest{Items: []app.DemandAddItem{{TaskID: "task.op.1234", Suffix: "docs", ResourceID: "resource.human.petr", Role: "writer", Unit: "hours", Amount: 3, EarliestStart: "2026-07-08", LatestFinish: "2026-07-10"}}, Reason: "Add docs", ActorID: demandActor, EvidenceID: demandEvidence, OccurredAt: "2026-07-08T10:00:00+07:00"})
	if err != nil || !add.Applied {
		t.Fatalf("add err=%v result=%#v", err, add)
	}
	demands := readTestFile(t, filepath.Join(project, "canonical/resource_demands.yaml"))
	tasks := readTestFile(t, filepath.Join(project, "canonical/tasks.yaml"))
	if !strings.Contains(demands, "  - id: resource_demand.1234.docs\n") || !strings.Contains(tasks, "id: resource_demand.1234.docs\n") {
		t.Fatalf("add did not update both files\ndemands=%s\ntasks=%s", demands, tasks)
	}
	foundTaskPreview := false
	for _, write := range add.PlannedWrites {
		if write.Path == "canonical/tasks.yaml" && (!strings.Contains(write.Content, "id: task.op.1234") || !strings.Contains(write.Content, "id: resource_demand.1234.docs") || strings.Contains(write.Content, "planned_amount")) {
			t.Fatalf("task preview contains more than appended refs: %s", write.Content)
		}
		if write.Path == "canonical/tasks.yaml" {
			foundTaskPreview = true
		}
	}
	if !foundTaskPreview {
		t.Fatal("task reference preview is missing")
	}
	amount, remaining, role := 4.0, 2.0, "documentation"
	set, err := service.Set(context.Background(), app.DemandSetRequest{Items: []app.DemandSetItem{{ID: "resource_demand.1234.docs", Amount: &amount, Remaining: &remaining, Role: &role}}, Reason: "Refine", ActorID: demandActor, EvidenceID: demandEvidence, OccurredAt: "2026-07-08T11:00:00+07:00"})
	if err != nil || !set.Applied || len(set.Changes) != 1 || set.Changes[0].Before.PlannedAmount != 3 || set.Changes[0].After.PlannedAmount != 4 {
		t.Fatalf("set err=%v result=%#v", err, set)
	}
	if got, want := set.AuditRecordPreviews[0].Reason, `Refine. planned_amount: 3 -> 4; remaining_amount: 3 -> 2; role: "writer" -> "documentation".`; got != want {
		t.Fatalf("set audit reason = %q, want %q", got, want)
	}
	moveAmount := 1.5
	move, err := service.Move(context.Background(), app.DemandMoveRequest{ID: "resource_demand.1234.docs", Suffix: "docs-ivan", ToResourceID: "resource.human.ivan", Amount: &moveAmount, Reason: "Move", ActorID: demandActor, EvidenceID: demandEvidence, OccurredAt: "2026-07-08T12:00:00+07:00"})
	if err != nil || !move.Applied {
		t.Fatalf("move err=%v result=%#v", err, move)
	}
	if got, want := move.AuditRecordPreviews[0].Reason, "Move. Moved to resource.human.ivan as resource_demand.1234.docs-ivan; remaining 1.5."; got != want {
		t.Fatalf("move audit reason = %q, want %q", got, want)
	}
	for _, write := range move.PlannedWrites {
		if write.Path == "canonical/tasks.yaml" && (!strings.Contains(write.Content, "id: resource_demand.1234.docs-ivan") || strings.Contains(write.Content, "planned_amount") || strings.Contains(write.Content, "superseded")) {
			t.Fatalf("move task preview contains more than the new ref: %s", write.Content)
		}
	}
	ws, diags := filestore.New(project).Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %#v", diags)
	}
	old, newDemand := demandByID(t, ws, "resource_demand.1234.docs"), demandByID(t, ws, "resource_demand.1234.docs-ivan")
	if old.Status != "superseded" || old.Remaining() != 0 || old.SupersededBy.ID != newDemand.ID || newDemand.Supersedes.ID != old.ID || newDemand.PlannedAmount != 1.5 {
		t.Fatalf("move result old=%#v new=%#v", old, newDemand)
	}
	before := readTestFile(t, filepath.Join(project, "canonical/resource_demands.yaml"))
	_, err = service.Set(context.Background(), app.DemandSetRequest{Items: []app.DemandSetItem{{ID: newDemand.ID, Amount: &amount}}, Reason: "Preview", ActorID: demandActor, EvidenceID: demandEvidence, DryRun: true})
	if err != nil || readTestFile(t, filepath.Join(project, "canonical/resource_demands.yaml")) != before {
		t.Fatalf("dry run wrote or failed: %v", err)
	}
}

func TestDemandAddPreservesSmallTeamYAMLBytes(t *testing.T) {
	project := copyProjectFixture(t)
	demandsPath := filepath.Join(project, "canonical/resource_demands.yaml")
	tasksPath := filepath.Join(project, "canonical/tasks.yaml")
	demandsBefore := readTestFile(t, demandsPath)
	tasksBefore := readTestFile(t, tasksPath)

	_, err := app.NewDemandService(filestore.New(project)).Add(context.Background(), app.DemandAddRequest{
		Items:  []app.DemandAddItem{{TaskID: "task.offline.repair-pump", Suffix: "review", ResourceID: "resource.human.ivan", Role: "reviewer", Unit: "hours", Amount: 2}},
		Reason: "Add review", ActorID: demandActor, EvidenceID: demandEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDemands := demandsBefore + `  - id: resource_demand.repair-pump.review
    task_ref:
      kind: task
      id: task.offline.repair-pump
    resource_ref:
      kind: resource
      id: resource.human.ivan
    resource_class: human
    role: reviewer
    unit: hours
    planned_amount: 2
    remaining_amount: 2
    criticality: required
    status: active
`
	wantTasks := replaceDemandTestOnce(t, tasksBefore,
		"      - kind: resource_demand\n        id: resource_demand.repair-pump.budget\n",
		"      - kind: resource_demand\n        id: resource_demand.repair-pump.budget\n      - kind: resource_demand\n        id: resource_demand.repair-pump.review\n")
	assertDemandTestFile(t, demandsPath, wantDemands)
	assertDemandTestFile(t, tasksPath, wantTasks)
}

func TestDemandSetPreservesSmallTeamYAMLBytes(t *testing.T) {
	project := copyProjectFixture(t)
	demandsPath := filepath.Join(project, "canonical/resource_demands.yaml")
	tasksPath := filepath.Join(project, "canonical/tasks.yaml")
	demandsBefore := readTestFile(t, demandsPath)
	tasksBefore := readTestFile(t, tasksPath)
	amount := 6.0

	_, err := app.NewDemandService(filestore.New(project)).Set(context.Background(), app.DemandSetRequest{
		Items:  []app.DemandSetItem{{ID: "resource_demand.repair-pump.operator", Amount: &amount}},
		Reason: "Refine operator", ActorID: demandActor, EvidenceID: demandEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDemands := replaceDemandTestOnce(t, demandsBefore, "    planned_amount: 4\n", "    planned_amount: 6\n")
	assertDemandTestFile(t, demandsPath, wantDemands)
	assertDemandTestFile(t, tasksPath, tasksBefore)
}

func TestDemandMovePreservesSmallTeamYAMLBytes(t *testing.T) {
	project := copyProjectFixture(t)
	demandsPath := filepath.Join(project, "canonical/resource_demands.yaml")
	tasksPath := filepath.Join(project, "canonical/tasks.yaml")
	demandsBefore := readTestFile(t, demandsPath)
	tasksBefore := readTestFile(t, tasksPath)

	_, err := app.NewDemandService(filestore.New(project)).Move(context.Background(), app.DemandMoveRequest{
		ID: "resource_demand.repair-pump.operator", ToResourceID: "resource.human.ivan", Suffix: "operator-ivan",
		Reason: "Move operator", ActorID: demandActor, EvidenceID: demandEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	wantDemands := replaceDemandTestOnce(t, demandsBefore,
		"    remaining_amount: 2\n    actual_amount: 2\n    criticality: required\n    status: partially_consumed\n",
		"    remaining_amount: 0\n    actual_amount: 2\n    criticality: required\n    status: superseded\n    superseded_by:\n      kind: resource_demand\n      id: resource_demand.repair-pump.operator-ivan\n") + `  - id: resource_demand.repair-pump.operator-ivan
    task_ref:
      kind: task
      id: task.offline.repair-pump
    resource_ref:
      kind: resource
      id: resource.human.ivan
    resource_class: human
    role: operator
    unit: hours
    planned_amount: 2
    remaining_amount: 2
    criticality: required
    status: active
    supersedes:
      kind: resource_demand
      id: resource_demand.repair-pump.operator
`
	wantTasks := replaceDemandTestOnce(t, tasksBefore,
		"      - kind: resource_demand\n        id: resource_demand.repair-pump.budget\n",
		"      - kind: resource_demand\n        id: resource_demand.repair-pump.budget\n      - kind: resource_demand\n        id: resource_demand.repair-pump.operator-ivan\n")
	assertDemandTestFile(t, demandsPath, wantDemands)
	assertDemandTestFile(t, tasksPath, wantTasks)
}

func TestDemandBatchIsAllOrNothingAndRefusals(t *testing.T) {
	project := copyProjectFixture(t)
	service := app.NewDemandService(filestore.New(project))
	path := filepath.Join(project, "canonical/resource_demands.yaml")
	before := readTestFile(t, path)
	result, err := service.Add(context.Background(), app.DemandAddRequest{Items: []app.DemandAddItem{
		{TaskID: "task.op.1234", Suffix: "ok", ResourceID: "resource.human.petr", Role: "writer", Unit: "hours", Amount: 1},
		{TaskID: "task.op.1234", Suffix: "bad", ResourceID: "resource.human.petr", Role: "writer", Unit: "days", Amount: 1},
	}, Reason: "Batch", ActorID: demandActor, EvidenceID: demandEvidence})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "invalid_resource_unit") || readTestFile(t, path) != before {
		t.Fatalf("batch was not atomic: err=%v errors=%#v", err, result.Errors)
	}
	for _, tc := range []struct {
		name, code string
		item       app.DemandSetItem
	}{
		{"unknown", "demand_not_found", app.DemandSetItem{ID: "missing", Amount: floatTestPtr(1)}},
		{"negative", "invalid_demand_amount", app.DemandSetItem{ID: "resource_demand.task-op-1234.implementer", Amount: floatTestPtr(-1)}},
		{"nonfinite", "invalid_demand_amount", app.DemandSetItem{ID: "resource_demand.task-op-1234.implementer", Amount: floatTestPtr(math.NaN())}},
		{"no change", "missing_demand_change", app.DemandSetItem{ID: "resource_demand.task-op-1234.implementer"}},
		{"same remaining", "demand_unchanged", app.DemandSetItem{ID: "resource_demand.task-op-1234.implementer", Remaining: floatTestPtr(8)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.Set(context.Background(), app.DemandSetRequest{Items: []app.DemandSetItem{tc.item}, Reason: "Refuse", ActorID: demandActor, EvidenceID: demandEvidence, DryRun: true})
			if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, tc.code) {
				t.Fatalf("err=%v errors=%#v", err, result.Errors)
			}
		})
	}
}

func TestDemandMoveRefusesNothingToMove(t *testing.T) {
	project := copyProjectFixture(t)
	service := app.NewDemandService(filestore.New(project))
	zero := 0.0
	if _, err := service.Set(context.Background(), app.DemandSetRequest{Items: []app.DemandSetItem{{ID: "resource_demand.task-op-1234.implementer", Remaining: &zero}}, Reason: "Zero remaining", ActorID: demandActor, EvidenceID: demandEvidence}); err != nil {
		t.Fatal(err)
	}
	result, err := service.Move(context.Background(), app.DemandMoveRequest{ID: "resource_demand.task-op-1234.implementer", Suffix: "nothing", ToResourceID: "resource.human.petr", Reason: "Move", ActorID: demandActor, EvidenceID: demandEvidence, DryRun: true})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "demand_nothing_to_move") {
		t.Fatalf("err=%v errors=%#v", err, result.Errors)
	}
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func replaceDemandTestOnce(t *testing.T, original, old, replacement string) string {
	t.Helper()
	if strings.Count(original, old) != 1 {
		t.Fatalf("test setup: replacement target occurs %d times", strings.Count(original, old))
	}
	return strings.Replace(original, old, replacement, 1)
}

func assertDemandTestFile(t *testing.T, path, want string) {
	t.Helper()
	if got := readTestFile(t, path); got != want {
		t.Fatalf("%s bytes differ\n--- got\n%s--- want\n%s", path, got, want)
	}
}

func floatTestPtr(value float64) *float64 { return &value }
