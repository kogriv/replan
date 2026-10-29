package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestScenarioListShowDiffSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewScenarioService(store)

	list, err := service.List(context.Background(), app.ScenarioListRequest{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(list.Scenarios) != 6 {
		t.Fatalf("expected 6 scenarios, got %d", len(list.Scenarios))
	}
	if !hasScenarioSummary(list.Scenarios, "scenario.ivan-unavailable-week-28") {
		t.Fatalf("scenario.ivan-unavailable-week-28 not listed: %#v", list.Scenarios)
	}
	if !hasScenarioSummary(list.Scenarios, "scenario.task-op-1234-remaining-5") {
		t.Fatalf("scenario.task-op-1234-remaining-5 not listed: %#v", list.Scenarios)
	}
	if !hasScenarioSummary(list.Scenarios, "scenario.task-op-1234-planned-10") {
		t.Fatalf("scenario.task-op-1234-planned-10 not listed: %#v", list.Scenarios)
	}
	if !hasScenarioSummary(list.Scenarios, "scenario.task-op-1234-reassign-petr") {
		t.Fatalf("scenario.task-op-1234-reassign-petr not listed: %#v", list.Scenarios)
	}
	if !hasScenarioSummary(list.Scenarios, "scenario.ivan-capacity-6") {
		t.Fatalf("scenario.ivan-capacity-6 not listed: %#v", list.Scenarios)
	}
	if !hasScenarioSummary(list.Scenarios, "scenario.openai-quota-300k") {
		t.Fatalf("scenario.openai-quota-300k not listed: %#v", list.Scenarios)
	}

	show, err := service.Show(context.Background(), app.ScenarioShowRequest{ID: "scenario.ivan-unavailable-week-28"})
	if err != nil {
		t.Fatalf("Show returned error: %v", err)
	}
	if show.Scenario.Name != "Ivan unavailable in week 28" {
		t.Fatalf("unexpected scenario name: %s", show.Scenario.Name)
	}

	diff, err := service.Diff(context.Background(), app.ScenarioDiffRequest{
		ID:      "scenario.ivan-unavailable-week-28",
		Against: "current",
		Week:    "2026-W28",
	})
	if err != nil {
		t.Fatalf("Diff returned error: %v", err)
	}
	if len(diff.AffectedResources) != 1 || diff.AffectedResources[0] != "resource.human.ivan" {
		t.Fatalf("unexpected affected resources: %#v", diff.AffectedResources)
	}
	before := findWorkloadRow(t, diff.WorkloadBefore, "resource.human.ivan", "hours")
	after := findWorkloadRow(t, diff.WorkloadAfter, "resource.human.ivan", "hours")
	if before.Capacity != 32 {
		t.Fatalf("expected Ivan capacity before 32, got %g", before.Capacity)
	}
	if after.Capacity != 0 {
		t.Fatalf("expected Ivan capacity after 0, got %g", after.Capacity)
	}
	if after.Overload != 8 {
		t.Fatalf("expected Ivan overload after 8, got %g", after.Overload)
	}
}

func TestScenarioApplyDryRun(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewScenarioService(store)

	result, err := service.Apply(context.Background(), app.ScenarioApplyRequest{
		ID:     "scenario.task-op-1234-remaining-5",
		DryRun: true,
		Now:    fixedScenarioApplyTestTime(),
	})

	if err != nil {
		t.Fatalf("Apply dry-run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if len(result.Changes) != 1 {
		t.Fatalf("Changes len = %d, want 1: %#v", len(result.Changes), result.Changes)
	}
	change := result.Changes[0]
	if change.TargetID != "resource_demand.task-op-1234.implementer" || change.Field != "remaining_amount" || change.PreviousValue != float64(8) || change.NewValue != float64(5) {
		t.Fatalf("unexpected change: %#v", change)
	}
	if len(result.PlannedWrites) != 3 {
		t.Fatalf("PlannedWrites len = %d, want 3: %#v", len(result.PlannedWrites), result.PlannedWrites)
	}
	if result.AuditRecordPreview.Result != "dry_run" {
		t.Fatalf("audit result = %q, want dry_run", result.AuditRecordPreview.Result)
	}
}

func TestScenarioApplyWritesCanonicalScenarioAndAudit(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewScenarioService(store)

	result, err := service.Apply(context.Background(), app.ScenarioApplyRequest{
		ID:      "scenario.task-op-1234-remaining-5",
		ActorID: "actor.human.operator",
		Reason:  "Apply remaining-work scenario in test.",
		Now:     fixedScenarioApplyTestTime(),
	})

	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if len(result.WrittenFiles) != 3 {
		t.Fatalf("WrittenFiles len = %d, want 3: %#v", len(result.WrittenFiles), result.WrittenFiles)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if amount := findDemandRemaining(t, ws.ResourceDemands, "resource_demand.task-op-1234.implementer"); amount != 5 {
		t.Fatalf("remaining amount = %g, want 5", amount)
	}
	if !hasScenarioStatus(ws.Scenarios, "scenario.task-op-1234-remaining-5", "applied") {
		t.Fatalf("scenario was not marked applied")
	}
	if !hasScenarioAudit(ws.AuditRecords, "audit.20260715T090000Z.scenario-apply.scenario-task-op-1234-remaining-5") {
		t.Fatalf("scenario apply audit was not loaded")
	}
}

func TestScenarioApplyWritesTaskPlannedDates(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	writeScenarioFixture(t, projectPath, "scenario.taskjuggler.date-apply", `scenario:
  id: scenario.taskjuggler.date-apply
  name: TaskJuggler date apply
  status: proposed
  base_ref:
    kind: current_plan
    id: current
  created_at: "2026-07-15T09:00:00Z"
  created_by:
    kind: actor
    id: actor.human.operator
  changes:
    - op: set
      path: tasks.task.op.1234.planned_start
      value: "2026-07-06"
    - op: set
      path: tasks.task.op.1234.planned_finish
      value: "2026-07-13"
`)
	store := filestore.New(projectPath)
	service := app.NewScenarioService(store)

	result, err := service.Apply(context.Background(), app.ScenarioApplyRequest{
		ID:      "scenario.taskjuggler.date-apply",
		ActorID: "actor.human.operator",
		Now:     fixedScenarioApplyTestTime(),
	})

	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if len(result.Changes) != 2 {
		t.Fatalf("Changes len = %d, want 2: %#v", len(result.Changes), result.Changes)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	task := findTask(t, ws.Tasks, "task.op.1234")
	if task.PlannedStart != "2026-07-06" || task.PlannedFinish != "2026-07-13" {
		t.Fatalf("planned dates = %s/%s, want 2026-07-06/2026-07-13", task.PlannedStart, task.PlannedFinish)
	}
	if !hasScenarioStatus(ws.Scenarios, "scenario.taskjuggler.date-apply", "applied") {
		t.Fatalf("scenario was not marked applied")
	}
}

func TestMovedTaskDateScenarioPreviewMatchesApply(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	configureMovedTaskDateFixture(t, projectPath)
	store := filestore.New(projectPath)
	ctx := context.Background()

	currentWeek28, err := app.NewWorkloadService(store).Calculate(ctx, app.WorkloadRequest{Week: "2026-W28"})
	if err != nil {
		t.Fatal(err)
	}
	previewWeek28, err := app.NewWorkloadService(store).Calculate(ctx, app.WorkloadRequest{Week: "2026-W28", ScenarioID: "scenario.move-validation-to-week-29"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findWorkloadRow(t, currentWeek28.Resources, "resource.human.ivan", "hours").Remaining; got != 8 {
		t.Fatalf("current W28 remaining = %v, want 8", got)
	}
	if got := findWorkloadRow(t, previewWeek28.Resources, "resource.human.ivan", "hours").Remaining; got != 0 {
		t.Fatalf("scenario W28 remaining = %v, want 0", got)
	}

	preview, err := app.NewWorkloadService(store).Calculate(ctx, app.WorkloadRequest{Week: "2026-W29", ScenarioID: "scenario.move-validation-to-week-29"})
	if err != nil {
		t.Fatal(err)
	}
	previewIvan := findWorkloadRow(t, preview.Resources, "resource.human.ivan", "hours")
	if previewIvan.Remaining != 36 || previewIvan.Capacity != 32 || previewIvan.Overload != 4 {
		t.Fatalf("preview Ivan row = %#v, want remaining/capacity/overload 36/32/4", previewIvan)
	}

	currentDashboard, err := app.NewReportService(store).Dashboard(ctx, app.DashboardReportRequest{
		ProjectPath: projectPath, Week: "2026-W29", GeneratedAt: fixedScenarioApplyTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	dashboard, err := app.NewReportService(store).Dashboard(ctx, app.DashboardReportRequest{
		ProjectPath: projectPath, Week: "2026-W29", ScenarioID: "scenario.move-validation-to-week-29", GeneratedAt: fixedScenarioApplyTestTime(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.ScenarioID != "scenario.move-validation-to-week-29" || dashboard.OpenProjectOverlay.ScenarioID != dashboard.ScenarioID {
		t.Fatalf("scenario markers = overview %q overlay %q", dashboard.ScenarioID, dashboard.OpenProjectOverlay.ScenarioID)
	}
	var moved reports.DashboardGanttTask
	for _, task := range dashboard.Gantt.Tasks {
		if task.ID == "task.op.1234" {
			moved = task
			break
		}
	}
	if moved.PlannedStart != "2026-07-13" || moved.PlannedFinish != "2026-07-17" {
		t.Fatalf("scenario Gantt task = %#v", moved)
	}
	if len(moved.Allocations) == 0 || moved.Allocations[0].ParticipationWindowSource != "task_window" || moved.Allocations[0].StartDate != "2026-07-13" {
		t.Fatalf("scenario Gantt allocations = %#v", moved.Allocations)
	}
	if !hasReportIssue(currentDashboard.Warnings, "dependency_successor_starts_before_predecessor_finish") || hasReportIssue(dashboard.Warnings, "dependency_successor_starts_before_predecessor_finish") {
		t.Fatalf("dependency warning did not follow moved dates: current=%#v scenario=%#v", currentDashboard.Warnings, dashboard.Warnings)
	}
	forecastFound := false
	for _, row := range dashboard.Tracking {
		if row.TaskID != "task.op.1234" {
			continue
		}
		for _, demand := range row.Demands {
			if demand.DemandID == "resource_demand.task-op-1234.implementer" && demand.Forecast == 8 {
				forecastFound = true
			}
		}
	}
	if !forecastFound {
		t.Fatalf("scenario dashboard forecast did not include moved task: %#v", dashboard.Tracking)
	}

	diff, err := app.NewScenarioService(store).Diff(ctx, app.ScenarioDiffRequest{ID: "scenario.move-validation-to-week-29", Against: "current", Week: "2026-W29"})
	if err != nil {
		t.Fatal(err)
	}
	if got := findWorkloadRow(t, diff.WorkloadBefore, "resource.human.ivan", "hours").Remaining; got != 28 {
		t.Fatalf("diff before remaining = %v, want 28", got)
	}
	if got := findWorkloadRow(t, diff.WorkloadAfter, "resource.human.ivan", "hours"); got.Remaining != 36 || got.Overload != 4 {
		t.Fatalf("diff after Ivan row = %#v", got)
	}

	apply, err := app.NewScenarioService(store).Apply(ctx, app.ScenarioApplyRequest{
		ID: "scenario.move-validation-to-week-29", ActorID: "actor.human.operator", Now: fixedScenarioApplyTestTime(),
	})
	if err != nil || !apply.Applied {
		t.Fatalf("apply result = %#v, err = %v", apply, err)
	}
	after, err := app.NewWorkloadService(store).Calculate(ctx, app.WorkloadRequest{Week: "2026-W29"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(preview.Resources, after.Resources) {
		t.Fatalf("preview resources = %#v\nafter apply = %#v", preview.Resources, after.Resources)
	}
}

func configureMovedTaskDateFixture(t *testing.T, projectPath string) {
	t.Helper()
	tasksPath := filepath.Join(projectPath, "canonical", "tasks.yaml")
	tasks, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(tasks), "    tracking_status: in_progress\n", "    tracking_status: in_progress\n    planned_start: \"2026-07-08\"\n    planned_finish: \"2026-07-12\"\n", 1)
	text = strings.Replace(text, "    tracking_status: ready\n", "    tracking_status: ready\n    planned_start: \"2026-07-06\"\n    planned_finish: \"2026-07-10\"\n", 1)
	text += `  - id: task.existing-week-29-load
    title: Existing week 29 load
    tracking_status: ready
    planned_start: "2026-07-13"
    planned_finish: "2026-07-17"
    resource_demands:
      - kind: resource_demand
        id: resource_demand.existing-week-29-load.ivan
`
	if err := os.WriteFile(tasksPath, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	demandsPath := filepath.Join(projectPath, "canonical", "resource_demands.yaml")
	demands, err := os.ReadFile(demandsPath)
	if err != nil {
		t.Fatal(err)
	}
	demandText := string(demands) + `  - id: resource_demand.existing-week-29-load.ivan
    task_ref:
      kind: task
      id: task.existing-week-29-load
    resource_ref:
      kind: resource
      id: resource.human.ivan
    resource_class: human
    role: implementer
    unit: hours
    planned_amount: 28
    remaining_amount: 28
    criticality: required
    status: active
`
	if err := os.WriteFile(demandsPath, []byte(demandText), 0o644); err != nil {
		t.Fatal(err)
	}
	writeScenarioFixture(t, projectPath, "scenario.move-validation-to-week-29", `scenario:
  id: scenario.move-validation-to-week-29
  name: Move validation into loaded week
  status: proposed
  base_ref:
    kind: current_plan
    id: current
  changes:
    - op: set
      path: tasks.task.op.1234.planned_start
      value: "2026-07-13"
    - op: set
      path: tasks.task.op.1234.planned_finish
      value: "2026-07-17"
`)
}

func hasReportIssue(issues []reports.ReportIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func TestScenarioApplyRejectsInvalidTaskDate(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	writeScenarioFixture(t, projectPath, "scenario.taskjuggler.invalid-date", `scenario:
  id: scenario.taskjuggler.invalid-date
  name: Invalid TaskJuggler date
  status: proposed
  base_ref:
    kind: current_plan
    id: current
  changes:
    - op: set
      path: tasks.task.op.1234.planned_start
      value: "not-a-date"
`)
	store := filestore.New(projectPath)
	service := app.NewScenarioService(store)

	result, err := service.Apply(context.Background(), app.ScenarioApplyRequest{
		ID:     "scenario.taskjuggler.invalid-date",
		DryRun: true,
		Now:    fixedScenarioApplyTestTime(),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasScenarioDiagnostic(result.Errors, "unsupported_scenario_apply_change") {
		t.Fatalf("expected unsupported_scenario_apply_change, got %#v", result.Errors)
	}
}

func TestScenarioApplyReportsUnsupportedChange(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewScenarioService(store)

	result, err := service.Apply(context.Background(), app.ScenarioApplyRequest{
		ID:     "scenario.ivan-unavailable-week-28",
		DryRun: true,
		Now:    fixedScenarioApplyTestTime(),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasScenarioDiagnostic(result.Errors, "unsupported_scenario_apply_change") {
		t.Fatalf("expected unsupported_scenario_apply_change, got %#v", result.Errors)
	}
}

func writeScenarioFixture(t *testing.T, projectPath, scenarioID, content string) {
	t.Helper()
	path := filepath.Join(projectPath, "scenarios", scenarioID+".yaml")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write scenario fixture: %v", err)
	}
}

func hasScenarioSummary(scenarios []app.ScenarioSummary, id string) bool {
	for _, scenario := range scenarios {
		if scenario.ID == id {
			return true
		}
	}
	return false
}

func fixedScenarioApplyTestTime() time.Time {
	return time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
}

func copyScenarioProjectFixture(t *testing.T) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), "small-team")
	if err := copyScenarioDir("../../examples/small-team", dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func copyScenarioDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func findDemandRemaining(t *testing.T, demands []domain.ResourceDemand, id string) float64 {
	t.Helper()
	for _, demand := range demands {
		if demand.ID == id {
			return demand.Remaining()
		}
	}
	t.Fatalf("resource demand %s not found", id)
	return 0
}

func findTask(t *testing.T, tasks []domain.Task, id string) domain.Task {
	t.Helper()
	for _, task := range tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not found", id)
	return domain.Task{}
}

func hasScenarioStatus(scenarios []domain.Scenario, id, status string) bool {
	for _, scenario := range scenarios {
		if scenario.ID == id && scenario.Status == status {
			return true
		}
	}
	return false
}

func hasScenarioAudit(records []domain.AuditRecord, id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}

func hasScenarioDiagnostic(diags []app.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func findWorkloadRow(t *testing.T, rows []app.ResourceWorkload, resourceID, unit string) app.ResourceWorkload {
	t.Helper()
	for _, row := range rows {
		if row.ResourceID == resourceID && row.Unit == unit {
			return row
		}
	}
	t.Fatalf("workload row not found for %s/%s", resourceID, unit)
	return app.ResourceWorkload{}
}

const cancelDependencyScenario = `scenario:
  id: scenario.cancel-repair-pump-dependency
  name: Validation goes ahead of the pump repair
  status: proposed
  base_ref:
    kind: current_plan
    id: current
  created_at: "2026-07-15T09:00:00Z"
  created_by:
    kind: actor
    id: actor.human.operator
  changes:
    - op: set
      path: dependencies.dependency.repair-pump-before-validation.status
      value: cancelled
`

// GAP-032: the operator lets a successor go ahead of its predecessor; a
// scenario cancels the dependency and the audit record names the decision.
func TestScenarioApplyCancelsDependencyWithEvidence(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	writeScenarioFixture(t, projectPath, "scenario.cancel-repair-pump-dependency", cancelDependencyScenario)
	store := filestore.New(projectPath)

	result, err := app.NewScenarioService(store).Apply(context.Background(), app.ScenarioApplyRequest{
		ID:         "scenario.cancel-repair-pump-dependency",
		ActorID:    "actor.human.operator",
		EvidenceID: "evidence.github.pr.45",
		Now:        fixedScenarioApplyTestTime(),
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v %#v", err, result.Errors)
	}
	if len(result.Changes) != 1 || result.Changes[0].TargetKind != "dependency" || result.Changes[0].PreviousValue != "open" || result.Changes[0].NewValue != "cancelled" {
		t.Fatalf("unexpected changes: %#v", result.Changes)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/dependencies.yaml") {
		t.Fatalf("dependencies file not planned: %#v", result.PlannedWrites)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	for _, dependency := range ws.Dependencies {
		if dependency.ID == "dependency.repair-pump-before-validation" && dependency.Status != "cancelled" {
			t.Fatalf("dependency status = %s, want cancelled", dependency.Status)
		}
	}
	var audit *domain.AuditRecord
	for i := range ws.AuditRecords {
		if ws.AuditRecords[i].TargetRef.ID == "scenario.cancel-repair-pump-dependency" {
			audit = &ws.AuditRecords[i]
		}
	}
	if audit == nil || len(audit.EvidenceRefs) != 1 || audit.EvidenceRefs[0].ID != "evidence.github.pr.45" {
		t.Fatalf("audit record does not name the decision evidence: %#v", audit)
	}
}

func TestScenarioApplyCancelDependencyRequiresEvidence(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	writeScenarioFixture(t, projectPath, "scenario.cancel-repair-pump-dependency", cancelDependencyScenario)
	service := app.NewScenarioService(filestore.New(projectPath))

	result, err := service.Apply(context.Background(), app.ScenarioApplyRequest{
		ID:      "scenario.cancel-repair-pump-dependency",
		ActorID: "actor.human.operator",
		Now:     fixedScenarioApplyTestTime(),
	})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "evidence_required") {
		t.Fatalf("want evidence_required, got %v %#v", err, result.Errors)
	}

	result, err = service.Apply(context.Background(), app.ScenarioApplyRequest{
		ID:         "scenario.cancel-repair-pump-dependency",
		ActorID:    "actor.human.operator",
		EvidenceID: "evidence.missing",
		Now:        fixedScenarioApplyTestTime(),
	})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "evidence_not_found") {
		t.Fatalf("want evidence_not_found, got %v %#v", err, result.Errors)
	}
}

func TestScenarioApplyRefusesOtherDependencyStatuses(t *testing.T) {
	projectPath := copyScenarioProjectFixture(t)
	writeScenarioFixture(t, projectPath, "scenario.cancel-repair-pump-dependency",
		strings.Replace(cancelDependencyScenario, "value: cancelled", "value: resolved", 1))

	result, err := app.NewScenarioService(filestore.New(projectPath)).Apply(context.Background(), app.ScenarioApplyRequest{
		ID:         "scenario.cancel-repair-pump-dependency",
		ActorID:    "actor.human.operator",
		EvidenceID: "evidence.github.pr.45",
		Now:        fixedScenarioApplyTestTime(),
	})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "unsupported_scenario_apply_change") {
		t.Fatalf("want unsupported_scenario_apply_change, got %v %#v", err, result.Errors)
	}
}
