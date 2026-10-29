package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestProposalListShowSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewProposalService(store)

	list, err := service.List(context.Background(), app.ProposalListRequest{Status: "open"})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(list.Proposals) != 1 {
		t.Fatalf("expected 1 open proposal, got %d", len(list.Proposals))
	}
	if list.Proposals[0].ID != "proposal.adjust-remaining-task-op-1234.20260706" {
		t.Fatalf("unexpected proposal id: %s", list.Proposals[0].ID)
	}

	show, err := service.Show(context.Background(), app.ProposalShowRequest{ID: "proposal.adjust-remaining-task-op-1234.20260706"})
	if err != nil {
		t.Fatalf("Show returned error: %v", err)
	}
	if show.Proposal.Type != "update_remaining_work" {
		t.Fatalf("unexpected proposal type: %s", show.Proposal.Type)
	}
}

func TestProposalSuggestDependenciesDryRun(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewProposalService(store)

	result, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID: "actor.ai.codex.local",
		Now:     fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("SuggestDependencies returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if len(result.Proposals) != 4 {
		t.Fatalf("Proposals len = %d, want 4: %#v", len(result.Proposals), result.Proposals)
	}
	if !hasSuggestedProposal(result.Proposals, "proposal.dependency.dependency.openai-quota-blocks-validation.mark-successor-blocked", "mark_successor_blocked", "tasks.task.op.1234.tracking_status") {
		t.Fatalf("expected mark successor blocked proposal, got %#v", result.Proposals)
	}
	if !hasSuggestedProposal(result.Proposals, "proposal.dependency.dependency.repair-pump-before-validation.review-dependency-schedule", "review_dependency_schedule", "dependencies.dependency.repair-pump-before-validation.schedule") {
		t.Fatalf("expected dependency schedule review proposal, got %#v", result.Proposals)
	}
	for _, proposal := range result.Proposals {
		if proposal.Status != "open" || !proposal.RequiresConfirmation {
			t.Fatalf("proposal should be open and require confirmation: %#v", proposal)
		}
		if proposal.CreatedAt != fixedProposalTestTime().Format(time.RFC3339) {
			t.Fatalf("proposal CreatedAt = %q", proposal.CreatedAt)
		}
		if proposal.CreatedBy.ID != "actor.ai.codex.local" {
			t.Fatalf("proposal CreatedBy = %#v", proposal.CreatedBy)
		}
	}
}

func TestProposalSuggestDependenciesWriteCandidates(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewProposalService(store)

	result, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID:         "actor.ai.codex.local",
		WriteCandidates: true,
		Now:             fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("SuggestDependencies write returned error: %v", err)
	}
	if result.DryRun {
		t.Fatalf("DryRun = true, want false")
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if len(result.WrittenFiles) != 4 {
		t.Fatalf("WrittenFiles len = %d, want 4: %#v", len(result.WrittenFiles), result.WrittenFiles)
	}
	writtenPath := filepath.Join(projectPath, "proposals", "open", "proposal.dependency.dependency.openai-quota-blocks-validation.mark-successor-blocked.yaml")
	data, err := os.ReadFile(writtenPath)
	if err != nil {
		t.Fatalf("read written proposal: %v", err)
	}
	if !strings.Contains(string(data), "type: mark_successor_blocked") {
		t.Fatalf("written proposal content unexpected:\n%s", data)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected successful validation, got %#v", result.Validation)
	}

	second, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID:         "actor.ai.codex.local",
		WriteCandidates: true,
		Now:             fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("second SuggestDependencies write returned error: %v", err)
	}
	if len(second.WrittenFiles) != 0 {
		t.Fatalf("second write should skip duplicates, wrote %#v", second.WrittenFiles)
	}
	if len(second.Proposals) != 0 {
		t.Fatalf("second write should have no new proposals, got %#v", second.Proposals)
	}
	if !hasProposalDiagnostic(second.Warnings, "proposal_candidate_already_exists") {
		t.Fatalf("expected duplicate proposal warning, got %#v", second.Warnings)
	}
}

func TestProposalSuggestDependenciesWriteRequiresActor(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewProposalService(store)

	result, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		WriteCandidates: true,
		Now:             fixedProposalTestTime(),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasProposalDiagnostic(result.Errors, "actor_required") {
		t.Fatalf("expected actor_required, got %#v", result.Errors)
	}
}

func TestProposalSuggestDependenciesCreatesResolutionCandidate(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	markRepairPumpAccepted(t, projectPath)
	store := filestore.New(projectPath)
	service := app.NewProposalService(store)

	result, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID: "actor.ai.codex.local",
		Now:     fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("SuggestDependencies returned error: %v", err)
	}
	if !hasSuggestedProposal(result.Proposals, "proposal.dependency.dependency.repair-pump-before-validation.mark-dependency-resolved", "mark_dependency_resolved", "dependencies.dependency.repair-pump-before-validation.status") {
		t.Fatalf("expected mark dependency resolved proposal, got %#v", result.Proposals)
	}
	if hasSuggestedProposal(result.Proposals, "proposal.dependency.dependency.repair-pump-before-validation.review-dependency-resolution", "review_dependency_resolution", "dependencies.dependency.repair-pump-before-validation.status") {
		t.Fatalf("did not expect review resolution proposal when predecessor is accepted: %#v", result.Proposals)
	}
}

func TestProposalSuggestDependenciesCreatesSuccessorReadyCandidate(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	markValidationBlockedAndDependenciesClosed(t, projectPath)
	store := filestore.New(projectPath)
	service := app.NewProposalService(store)

	result, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID: "actor.ai.codex.local",
		Now:     fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("SuggestDependencies returned error: %v", err)
	}
	if !hasSuggestedProposal(result.Proposals, "proposal.task.task.op.1234.mark-successor-ready", "mark_successor_ready", "tasks.task.op.1234.tracking_status") {
		t.Fatalf("expected mark successor ready proposal, got %#v", result.Proposals)
	}
}

func TestProposalSuggestDependenciesSkipsScheduleCandidateForResolvedDependency(t *testing.T) {
	service := app.NewProposalService(staticWorkspaceLoader{
		projectPath: "memory",
		workspace: domain.Workspace{
			Project: domain.Project{
				ID:       "project.test",
				Name:     "Test",
				Timezone: "UTC",
			},
			Tasks: []domain.Task{
				{
					ID:             "task.predecessor",
					Title:          "Predecessor",
					TrackingStatus: "accepted",
					PlannedFinish:  "2026-08-14",
				},
				{
					ID:             "task.successor",
					Title:          "Successor",
					TrackingStatus: "in_progress",
					PlannedStart:   "2026-07-30",
				},
			},
			Dependencies: []domain.Dependency{{
				ID:             "dependency.resolved-history",
				Type:           "finish_to_start",
				PredecessorRef: domain.Ref{Kind: "task", ID: "task.predecessor"},
				SuccessorRef:   domain.Ref{Kind: "task", ID: "task.successor"},
				Status:         "resolved",
				Criticality:    "required",
			}},
		},
	})

	result, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		Now: fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("SuggestDependencies returned error: %v", err)
	}
	if len(result.Proposals) != 0 {
		t.Fatalf("resolved dependency should not generate proposals, got %#v", result.Proposals)
	}
}

func TestProposalAcceptDryRun(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewProposalService(store)

	result, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:     "proposal.adjust-remaining-task-op-1234.20260706",
		DryRun: true,
		Now:    fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("Accept dry-run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatalf("DryRun = false, want true")
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if result.PreviousStatus != "open" || result.NewStatus != "accepted" {
		t.Fatalf("unexpected status transition %s -> %s", result.PreviousStatus, result.NewStatus)
	}
	if len(result.PlannedWrites) != 4 {
		t.Fatalf("PlannedWrites len = %d, want 4: %#v", len(result.PlannedWrites), result.PlannedWrites)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/resource_demands.yaml") {
		t.Fatalf("expected canonical/resource_demands.yaml planned write: %#v", result.PlannedWrites)
	}
	if result.AuditRecordPreview.Result != "dry_run" {
		t.Fatalf("audit result = %q, want dry_run", result.AuditRecordPreview.Result)
	}
}

func TestProposalAcceptAppliesDecisionAndAudit(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewProposalService(store)

	result, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:      "proposal.adjust-remaining-task-op-1234.20260706",
		ActorID: "actor.human.operator",
		Reason:  "Operator accepted proposal after review.",
		Now:     fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if len(result.WrittenFiles) != 4 {
		t.Fatalf("WrittenFiles len = %d, want 4: %#v", len(result.WrittenFiles), result.WrittenFiles)
	}
	if _, err := os.Stat(filepath.Join(projectPath, "proposals", "open", "proposal.adjust-remaining-task-op-1234.20260706.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open proposal still exists or stat failed unexpectedly: %v", err)
	}
	acceptedPath := filepath.Join(projectPath, "proposals", "accepted", "proposal.adjust-remaining-task-op-1234.20260706.yaml")
	data, err := os.ReadFile(acceptedPath)
	if err != nil {
		t.Fatalf("read accepted proposal: %v", err)
	}
	if !strings.Contains(string(data), "status: accepted") {
		t.Fatalf("accepted proposal status not updated:\n%s", data)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if !hasProposalStatus(ws.Proposals, "proposal.adjust-remaining-task-op-1234.20260706", "accepted") {
		t.Fatalf("accepted proposal was not loaded")
	}
	if amount := findProposalRemaining(ws.ResourceDemands, "resource_demand.task-op-1234.implementer"); amount != 5 {
		t.Fatalf("remaining amount = %g, want 5", amount)
	}
	if !hasAuditRecord(ws.AuditRecords, "audit.20260715T090000Z.proposal-accepted.proposal-adjust-remaining-task-op-1234-20260706") {
		t.Fatalf("proposal acceptance audit was not loaded")
	}
}

func TestProposalAcceptAppliesTrackingRemainingWorkCandidate(t *testing.T) {
	projectPath := copyProposalProjectFromFixture(t, "../../examples/replan-self", "replan-self")
	removeTrackingProposalArtifacts(t, projectPath)
	markTrackingProposalDemandsStale(t, projectPath)
	tracking := app.NewTrackingService(filestore.New(projectPath))
	_, err := tracking.Suggest(context.Background(), app.TrackingSuggestRequest{
		Week:            "2026-W31",
		ActorID:         "actor.ai.codex.local",
		WriteCandidates: true,
		Now:             fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("tracking suggest write returned error: %v", err)
	}
	service := app.NewProposalService(filestore.New(projectPath))
	proposalID := "proposal.tracking.resource_demand-tracking-proposals-maintainer.set-remaining-zero"

	dryRun, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:     proposalID,
		DryRun: true,
		Now:    fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("Accept dry-run returned error: %v", err)
	}
	if !dryRun.SuggestedApplied {
		t.Fatalf("SuggestedApplied = false, want true")
	}
	if !hasPlannedWrite(dryRun.PlannedWrites, "canonical/resource_demands.yaml") {
		t.Fatalf("expected canonical/resource_demands.yaml planned write: %#v", dryRun.PlannedWrites)
	}

	result, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:      proposalID,
		ActorID: "actor.human.maintainer",
		Reason:  "Operator accepted stale remaining-work tracking proposal.",
		Now:     fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied || !result.SuggestedApplied {
		t.Fatalf("unexpected apply flags: applied=%t suggested_applied=%t", result.Applied, result.SuggestedApplied)
	}
	if !hasWrittenFile(result.WrittenFiles, "canonical/resource_demands.yaml") {
		t.Fatalf("expected canonical/resource_demands.yaml written file: %#v", result.WrittenFiles)
	}
	ws, diags := filestore.New(projectPath).Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if amount := findProposalRemaining(ws.ResourceDemands, "resource_demand.tracking-proposals.maintainer"); amount != 0 {
		t.Fatalf("tracking proposal remaining amount = %g, want 0", amount)
	}
	if !hasProposalStatus(ws.Proposals, proposalID, "accepted") {
		t.Fatalf("tracking proposal was not accepted")
	}

	secondProposalID := "proposal.tracking.resource_demand-tracking-proposals-codex.set-remaining-zero"
	second, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:      secondProposalID,
		ActorID: "actor.human.maintainer",
		Reason:  "Operator accepted second stale remaining-work tracking proposal.",
		Now:     fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("second Accept returned error: %v", err)
	}
	if !second.Applied || !second.SuggestedApplied {
		t.Fatalf("unexpected second apply flags: applied=%t suggested_applied=%t", second.Applied, second.SuggestedApplied)
	}
	ws, diags = filestore.New(projectPath).Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics after second accept: %#v", diags)
	}
	firstAuditID := "audit.20260715T090000Z.proposal-accepted.proposal-tracking-resource-demand-tracking-proposals-maintainer-set-remaining-zero"
	secondAuditID := "audit.20260715T090000Z.proposal-accepted.proposal-tracking-resource-demand-tracking-proposals-codex-set-remaining-zero"
	if !hasAuditRecord(ws.AuditRecords, firstAuditID) || !hasAuditRecord(ws.AuditRecords, secondAuditID) {
		t.Fatalf("expected distinct tracking proposal audit ids %q and %q", firstAuditID, secondAuditID)
	}
}

func TestProposalAcceptAppliesMarkSuccessorBlocked(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewProposalService(store)

	_, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID:         "actor.ai.codex.local",
		WriteCandidates: true,
		Now:             fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("SuggestDependencies write returned error: %v", err)
	}

	result, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:      "proposal.dependency.dependency.openai-quota-blocks-validation.mark-successor-blocked",
		ActorID: "actor.human.operator",
		Reason:  "Dependency is still open; block successor before execution.",
		Now:     fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied || !result.SuggestedApplied {
		t.Fatalf("Applied/SuggestedApplied = %t/%t, want true/true", result.Applied, result.SuggestedApplied)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/tasks.yaml") {
		t.Fatalf("expected canonical/tasks.yaml planned write, got %#v", result.PlannedWrites)
	}
	tasksYAML, err := os.ReadFile(filepath.Join(projectPath, "canonical", "tasks.yaml"))
	if err != nil {
		t.Fatalf("read tasks: %v", err)
	}
	if !strings.Contains(string(tasksYAML), "tracking_status: blocked") {
		t.Fatalf("task status was not blocked:\n%s", tasksYAML)
	}
	acceptedPath := filepath.Join(projectPath, "proposals", "accepted", "proposal.dependency.dependency.openai-quota-blocks-validation.mark-successor-blocked.yaml")
	if _, err := os.Stat(acceptedPath); err != nil {
		t.Fatalf("accepted proposal missing: %v", err)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if !hasTaskStatus(ws.Tasks, "task.op.1234", "blocked") {
		t.Fatalf("task.op.1234 was not blocked")
	}
	if !hasAuditRecord(ws.AuditRecords, "audit.20260715T090000Z.proposal-accepted.proposal-dependency-dependency-openai-quota-blocks-validation-mark-successor-blocked") {
		t.Fatalf("proposal acceptance audit was not loaded")
	}
}

func TestProposalAcceptAppliesMarkDependencyResolved(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	markRepairPumpAccepted(t, projectPath)
	store := filestore.New(projectPath)
	service := app.NewProposalService(store)

	_, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID:         "actor.ai.codex.local",
		WriteCandidates: true,
		Now:             fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("SuggestDependencies write returned error: %v", err)
	}

	result, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:      "proposal.dependency.dependency.repair-pump-before-validation.mark-dependency-resolved",
		ActorID: "actor.human.operator",
		Reason:  "Predecessor task is accepted; resolve dependency.",
		Now:     fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied || !result.SuggestedApplied {
		t.Fatalf("Applied/SuggestedApplied = %t/%t, want true/true", result.Applied, result.SuggestedApplied)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/dependencies.yaml") {
		t.Fatalf("expected canonical/dependencies.yaml planned write, got %#v", result.PlannedWrites)
	}
	dependenciesYAML, err := os.ReadFile(filepath.Join(projectPath, "canonical", "dependencies.yaml"))
	if err != nil {
		t.Fatalf("read dependencies: %v", err)
	}
	if !strings.Contains(string(dependenciesYAML), "status: resolved") {
		t.Fatalf("dependency status was not resolved:\n%s", dependenciesYAML)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if !hasDependencyStatus(ws.Dependencies, "dependency.repair-pump-before-validation", "resolved") {
		t.Fatalf("dependency was not resolved")
	}
	if !hasProposalStatus(ws.Proposals, "proposal.dependency.dependency.repair-pump-before-validation.mark-dependency-resolved", "accepted") {
		t.Fatalf("resolution proposal was not accepted")
	}
}

func TestProposalAcceptAppliesMarkSuccessorReady(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	markValidationBlockedAndDependenciesClosed(t, projectPath)
	store := filestore.New(projectPath)
	service := app.NewProposalService(store)

	_, err := service.SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID:         "actor.ai.codex.local",
		WriteCandidates: true,
		Now:             fixedProposalTestTime(),
	})
	if err != nil {
		t.Fatalf("SuggestDependencies write returned error: %v", err)
	}

	result, err := service.Accept(context.Background(), app.ProposalDecisionRequest{
		ID:      "proposal.task.task.op.1234.mark-successor-ready",
		ActorID: "actor.human.operator",
		Reason:  "All required dependencies are closed; return task to ready.",
		Now:     fixedProposalTestTime(),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied || !result.SuggestedApplied {
		t.Fatalf("Applied/SuggestedApplied = %t/%t, want true/true", result.Applied, result.SuggestedApplied)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/tasks.yaml") {
		t.Fatalf("expected canonical/tasks.yaml planned write, got %#v", result.PlannedWrites)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diags)
	}
	if !hasTaskStatus(ws.Tasks, "task.op.1234", "ready") {
		t.Fatalf("task.op.1234 was not marked ready")
	}
	if !hasProposalStatus(ws.Proposals, "proposal.task.task.op.1234.mark-successor-ready", "accepted") {
		t.Fatalf("successor ready proposal was not accepted")
	}
}

func TestProposalRejectRequiresReason(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewProposalService(store)

	result, err := service.Reject(context.Background(), app.ProposalDecisionRequest{
		ID:      "proposal.adjust-remaining-task-op-1234.20260706",
		ActorID: "actor.human.operator",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasProposalDiagnostic(result.Errors, "reason_required") {
		t.Fatalf("expected reason_required, got %#v", result.Errors)
	}
}

func fixedProposalTestTime() time.Time {
	return time.Date(2026, 7, 15, 9, 0, 0, 0, time.UTC)
}

func copyProposalProjectFixture(t *testing.T) string {
	t.Helper()
	return copyProposalProjectFromFixture(t, "../../examples/small-team", "small-team")
}

func copyProposalProjectFromFixture(t *testing.T, sourcePath, targetName string) string {
	t.Helper()
	dst := filepath.Join(t.TempDir(), targetName)
	if err := copyAppProjectDir(sourcePath, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func markRepairPumpAccepted(t *testing.T, projectPath string) {
	t.Helper()
	path := filepath.Join(projectPath, "canonical", "tasks.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tasks: %v", err)
	}
	updated := strings.Replace(string(data), "tracking_status: in_progress", "tracking_status: accepted", 1)
	if updated == string(data) {
		t.Fatalf("repair pump tracking_status was not updated in test fixture")
	}
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatalf("write tasks: %v", err)
	}
}

func markValidationBlockedAndDependenciesClosed(t *testing.T, projectPath string) {
	t.Helper()
	tasksPath := filepath.Join(projectPath, "canonical", "tasks.yaml")
	tasksData, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatalf("read tasks: %v", err)
	}
	tasksUpdated := strings.Replace(string(tasksData), "tracking_status: ready", "tracking_status: blocked", 1)
	if tasksUpdated == string(tasksData) {
		t.Fatalf("validation task tracking_status was not updated in test fixture")
	}
	if err := os.WriteFile(tasksPath, []byte(tasksUpdated), 0o644); err != nil {
		t.Fatalf("write tasks: %v", err)
	}

	dependenciesPath := filepath.Join(projectPath, "canonical", "dependencies.yaml")
	dependenciesData, err := os.ReadFile(dependenciesPath)
	if err != nil {
		t.Fatalf("read dependencies: %v", err)
	}
	dependenciesText := string(dependenciesData)
	dependenciesText = strings.Replace(dependenciesText, "status: open", "status: resolved", 1)
	dependenciesText = strings.Replace(dependenciesText, "status: open", "status: cancelled", 1)
	if err := os.WriteFile(dependenciesPath, []byte(dependenciesText), 0o644); err != nil {
		t.Fatalf("write dependencies: %v", err)
	}
}

func removeTrackingProposalArtifacts(t *testing.T, projectPath string) {
	t.Helper()
	for _, dir := range []string{"open", "accepted", "rejected"} {
		matches, err := filepath.Glob(filepath.Join(projectPath, "proposals", dir, "proposal.tracking.resource_demand-tracking-proposals-*.yaml"))
		if err != nil {
			t.Fatalf("glob tracking proposal artifacts: %v", err)
		}
		for _, match := range matches {
			if err := os.Remove(match); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("remove tracking proposal artifact: %v", err)
			}
		}
	}
}

func markTrackingProposalDemandsStale(t *testing.T, projectPath string) {
	t.Helper()
	path := filepath.Join(projectPath, "canonical", "resource_demands.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read resource demands: %v", err)
	}
	text := string(data)
	text = replaceOnceForTest(t, text,
		"id: resource_demand.tracking-proposals.maintainer\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.human.maintainer\n      resource_class: human\n      role: implementer\n      unit: hours\n      planned_amount: 3\n      criticality: required",
		"id: resource_demand.tracking-proposals.maintainer\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.human.maintainer\n      resource_class: human\n      role: implementer\n      unit: hours\n      planned_amount: 3\n      remaining_amount: 3\n      criticality: required")
	text = replaceOnceForTest(t, text,
		"id: resource_demand.tracking-proposals.codex\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.ai.codex.local\n      resource_class: ai_agent\n      role: ai_coding_agent\n      unit: agent_hours\n      planned_amount: 3\n      criticality: preferred",
		"id: resource_demand.tracking-proposals.codex\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.ai.codex.local\n      resource_class: ai_agent\n      role: ai_coding_agent\n      unit: agent_hours\n      planned_amount: 3\n      remaining_amount: 3\n      criticality: preferred")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write resource demands: %v", err)
	}
}

func replaceOnceForTest(t *testing.T, text, old, new string) string {
	t.Helper()
	if !strings.Contains(text, old) {
		t.Fatalf("fixture text not found:\n%s", old)
	}
	return strings.Replace(text, old, new, 1)
}

func copyAppProjectDir(src, dst string) error {
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

func hasProposalStatus(proposals []domain.Proposal, id, status string) bool {
	for _, proposal := range proposals {
		if proposal.ID == id && proposal.Status == status {
			return true
		}
	}
	return false
}

func hasAuditRecord(records []domain.AuditRecord, id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}

func hasProposalDiagnostic(diags []app.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func hasSuggestedProposal(proposals []domain.Proposal, id, proposalType, path string) bool {
	for _, proposal := range proposals {
		if proposal.ID == id && proposal.Type == proposalType && proposal.SuggestedChange.Path == path {
			return true
		}
	}
	return false
}

func hasTaskStatus(tasks []domain.Task, id, status string) bool {
	for _, task := range tasks {
		if task.ID == id && task.TrackingStatus == status {
			return true
		}
	}
	return false
}

func hasDependencyStatus(dependencies []domain.Dependency, id, status string) bool {
	for _, dependency := range dependencies {
		if dependency.ID == id && dependency.Status == status {
			return true
		}
	}
	return false
}

func hasWrittenFile(paths []string, path string) bool {
	for _, written := range paths {
		if written == path {
			return true
		}
	}
	return false
}

func findProposalRemaining(demands []domain.ResourceDemand, id string) float64 {
	for _, demand := range demands {
		if demand.ID == id {
			return demand.Remaining()
		}
	}
	return -1
}
