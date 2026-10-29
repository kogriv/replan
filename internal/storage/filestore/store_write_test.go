package filestore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func TestStoreAppendActualAndAudit(t *testing.T) {
	projectPath := copyProjectFixture(t)
	store := New(projectPath)

	actual := domain.Actual{
		ID:          "actual.test.append.1",
		Type:        "work_consumed",
		TaskRef:     domain.Ref{Kind: "task", ID: "task.op.1234"},
		DemandRef:   domain.Ref{Kind: "resource_demand", ID: "resource_demand.task-op-1234.implementer"},
		ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
		Amount:      3,
		Unit:        "hours",
		OccurredAt:  "2026-07-06T16:00:00+07:00",
		StatusDate:  "2026-07-06",
		AcceptedBy:  domain.Ref{Kind: "actor", ID: "actor.human.operator"},
	}
	actualPath, err := store.AppendActual(actual.StatusDate, actual)
	if err != nil {
		t.Fatalf("AppendActual returned error: %v", err)
	}
	if actualPath != "actuals/actuals.2026-W28.yaml" {
		t.Fatalf("unexpected actual path: %s", actualPath)
	}

	audit := domain.AuditRecord{
		ID:         "audit.test.append-actual",
		OccurredAt: "2026-07-06T16:00:00+07:00",
		ActorRef:   domain.Ref{Kind: "actor", ID: "actor.human.operator"},
		Operation:  "actual_add_consumption",
		TargetRef:  domain.Ref{Kind: "actual", ID: actual.ID},
		Result:     "success",
	}
	auditPath, err := store.AppendAudit(audit)
	if err != nil {
		t.Fatalf("AppendAudit returned error: %v", err)
	}
	if auditPath != "audit/audit.2026-07.jsonl" {
		t.Fatalf("unexpected audit path: %s", auditPath)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	if !hasActual(ws.Actuals, actual.ID) {
		t.Fatalf("appended actual was not loaded")
	}
	if !hasAudit(ws.AuditRecords, audit.ID) {
		t.Fatalf("appended audit was not loaded")
	}
}

func TestStoreSetResourceDemandRemaining(t *testing.T) {
	projectPath := copyProjectFixture(t)
	store := New(projectPath)

	path, err := store.SetResourceDemandRemaining("resource_demand.task-op-1234.implementer", 5)
	if err != nil {
		t.Fatalf("SetResourceDemandRemaining returned error: %v", err)
	}
	if path != "canonical/resource_demands.yaml" {
		t.Fatalf("unexpected path: %s", path)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	for _, demand := range ws.ResourceDemands {
		if demand.ID == "resource_demand.task-op-1234.implementer" {
			if demand.Remaining() != 5 {
				t.Fatalf("remaining amount was not updated, got %v", demand.Remaining())
			}
			return
		}
	}
	t.Fatal("resource demand was not loaded")
}

func TestStoreSetResourceDemandStatus(t *testing.T) {
	projectPath := copyProjectFixture(t)
	store := New(projectPath)

	path, err := store.SetResourceDemandStatus("resource_demand.task-op-1234.implementer", "consumed")
	if err != nil {
		t.Fatalf("SetResourceDemandStatus returned error: %v", err)
	}
	if path != "canonical/resource_demands.yaml" {
		t.Fatalf("unexpected path: %s", path)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	for _, demand := range ws.ResourceDemands {
		if demand.ID == "resource_demand.task-op-1234.implementer" {
			if demand.Status != "consumed" {
				t.Fatalf("status was not updated, got %q", demand.Status)
			}
			return
		}
	}
	t.Fatal("resource demand was not loaded")
}

func TestStoreSetTaskTrackingStatus(t *testing.T) {
	projectPath := copyProjectFixture(t)
	store := New(projectPath)

	path, err := store.SetTaskTrackingStatus("task.op.1234", "review_ready")
	if err != nil {
		t.Fatalf("SetTaskTrackingStatus returned error: %v", err)
	}
	if path != "canonical/tasks.yaml" {
		t.Fatalf("unexpected path: %s", path)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	for _, task := range ws.Tasks {
		if task.ID == "task.op.1234" {
			if task.TrackingStatus != "review_ready" {
				t.Fatalf("tracking status was not updated, got %s", task.TrackingStatus)
			}
			return
		}
	}
	t.Fatal("task was not loaded")
}

func TestStoreLoadAllowsMissingDependenciesFile(t *testing.T) {
	projectPath := copyProjectFixture(t)
	if err := os.Remove(filepath.Join(projectPath, "canonical", "dependencies.yaml")); err != nil {
		t.Fatalf("remove dependencies file: %v", err)
	}
	store := New(projectPath)

	ws, diags := store.Load(context.Background())

	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	if len(ws.Dependencies) != 0 {
		t.Fatalf("Dependencies len = %d, want 0", len(ws.Dependencies))
	}
}

func TestStoreMoveProposal(t *testing.T) {
	projectPath := copyProjectFixture(t)
	store := New(projectPath)

	audit := domain.AuditRecord{
		ID:         "audit.test.proposal-accepted",
		OccurredAt: "2026-07-15T09:00:00Z",
		ActorRef:   domain.Ref{Kind: "actor", ID: "actor.human.operator"},
		Operation:  "proposal_accepted",
		TargetRef:  domain.Ref{Kind: "proposal", ID: "proposal.adjust-remaining-task-op-1234.20260706"},
		Result:     "success",
	}
	written, err := store.MoveProposal("proposal.adjust-remaining-task-op-1234.20260706", "accepted", audit)
	if err != nil {
		t.Fatalf("MoveProposal returned error: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("written len = %d, want 3: %#v", len(written), written)
	}
	if _, err := os.Stat(filepath.Join(projectPath, "proposals", "open", "proposal.adjust-remaining-task-op-1234.20260706.yaml")); !os.IsNotExist(err) {
		t.Fatalf("open proposal still exists or stat failed unexpectedly: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectPath, "proposals", "accepted", "proposal.adjust-remaining-task-op-1234.20260706.yaml")); err != nil {
		t.Fatalf("accepted proposal missing: %v", err)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	if !hasProposal(ws.Proposals, "proposal.adjust-remaining-task-op-1234.20260706", "accepted") {
		t.Fatalf("moved proposal was not loaded with accepted status")
	}
	if !hasAudit(ws.AuditRecords, "audit.test.proposal-accepted") {
		t.Fatalf("proposal audit was not loaded")
	}
}

func TestStoreApplyProposalDecisionUpdatesRemainingWork(t *testing.T) {
	projectPath := copyProjectFixture(t)
	store := New(projectPath)

	audit := domain.AuditRecord{
		ID:         "audit.test.proposal-accepted",
		OccurredAt: "2026-07-15T09:00:00Z",
		ActorRef:   domain.Ref{Kind: "actor", ID: "actor.human.operator"},
		Operation:  "proposal_accepted",
		TargetRef:  domain.Ref{Kind: "proposal", ID: "proposal.adjust-remaining-task-op-1234.20260706"},
		Result:     "success",
	}
	written, err := store.ApplyProposalDecision(
		"proposal.adjust-remaining-task-op-1234.20260706",
		"accepted",
		nil,
		nil,
		[]app.ProposalResourceDemandChange{{
			DemandID:      "resource_demand.task-op-1234.implementer",
			Field:         "remaining_amount",
			PreviousValue: 8,
			NewValue:      5,
		}},
		audit,
	)
	if err != nil {
		t.Fatalf("ApplyProposalDecision returned error: %v", err)
	}
	if len(written) != 4 {
		t.Fatalf("written len = %d, want 4: %#v", len(written), written)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	if amount := findRemaining(ws.ResourceDemands, "resource_demand.task-op-1234.implementer"); amount != 5 {
		t.Fatalf("remaining amount = %g, want 5", amount)
	}
	if !hasProposal(ws.Proposals, "proposal.adjust-remaining-task-op-1234.20260706", "accepted") {
		t.Fatalf("accepted proposal was not loaded")
	}
	if !hasAudit(ws.AuditRecords, "audit.test.proposal-accepted") {
		t.Fatalf("proposal audit was not loaded")
	}
}

func TestStoreApplyScenarioChanges(t *testing.T) {
	projectPath := copyProjectFixture(t)
	store := New(projectPath)

	audit := domain.AuditRecord{
		ID:         "audit.test.scenario-apply",
		OccurredAt: "2026-07-15T09:00:00Z",
		ActorRef:   domain.Ref{Kind: "actor", ID: "actor.human.operator"},
		Operation:  "scenario_apply",
		TargetRef:  domain.Ref{Kind: "scenario", ID: "scenario.task-op-1234-remaining-5"},
		Result:     "success",
	}
	written, err := store.ApplyScenarioChanges("scenario.task-op-1234-remaining-5", []app.ScenarioApplyChange{{
		TargetID:   "resource_demand.task-op-1234.implementer",
		TargetKind: "resource_demand",
		Field:      "remaining_amount",
		NewValue:   float64(5),
	}}, audit)
	if err != nil {
		t.Fatalf("ApplyScenarioChanges returned error: %v", err)
	}
	if len(written) != 3 {
		t.Fatalf("written len = %d, want 3: %#v", len(written), written)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	if amount := findRemaining(ws.ResourceDemands, "resource_demand.task-op-1234.implementer"); amount != 5 {
		t.Fatalf("remaining amount = %g, want 5", amount)
	}
	if !hasScenario(ws.Scenarios, "scenario.task-op-1234-remaining-5", "applied") {
		t.Fatalf("scenario was not marked applied")
	}
	if !hasAudit(ws.AuditRecords, "audit.test.scenario-apply") {
		t.Fatalf("scenario apply audit was not loaded")
	}
}

func copyProjectFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Clean("../../../examples/small-team")
	dst := filepath.Join(t.TempDir(), "small-team")
	if err := copyDir(src, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func copyDir(src, dst string) error {
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

func hasActual(actuals []domain.Actual, id string) bool {
	for _, actual := range actuals {
		if actual.ID == id {
			return true
		}
	}
	return false
}

func hasAudit(records []domain.AuditRecord, id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}

func hasProposal(proposals []domain.Proposal, id, status string) bool {
	for _, proposal := range proposals {
		if proposal.ID == id && proposal.Status == status {
			return true
		}
	}
	return false
}

func hasScenario(scenarios []domain.Scenario, id, status string) bool {
	for _, scenario := range scenarios {
		if scenario.ID == id && scenario.Status == status {
			return true
		}
	}
	return false
}

func findRemaining(demands []domain.ResourceDemand, id string) float64 {
	for _, demand := range demands {
		if demand.ID == id {
			return demand.Remaining()
		}
	}
	return -1
}
