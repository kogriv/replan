package filestore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func TestStagedWritesStayInMemoryUntilCommit(t *testing.T) {
	projectPath := copyProjectFixture(t)
	demandsPath := filepath.Join(projectPath, "canonical/resource_demands.yaml")
	before, err := os.ReadFile(demandsPath)
	if err != nil {
		t.Fatal(err)
	}

	staged := New(projectPath).StageWrites().(*StagedStore)
	if _, err := staged.SetResourceDemandRemaining("resource_demand.task-op-1234.implementer", 0); err != nil {
		t.Fatalf("SetResourceDemandRemaining: %v", err)
	}
	actual := domain.Actual{
		ID:         "actual.staged.1",
		Type:       "work_consumed",
		TaskRef:    domain.Ref{Kind: "task", ID: "task.op.1234"},
		DemandRef:  domain.Ref{Kind: "resource_demand", ID: "resource_demand.task-op-1234.implementer"},
		Amount:     1,
		Unit:       "hours",
		OccurredAt: "2031-01-06T10:00:00Z",
		StatusDate: "2031-01-06",
	}
	actualRel, err := staged.AppendActual(actual.StatusDate, actual)
	if err != nil {
		t.Fatalf("AppendActual: %v", err)
	}
	if _, err := staged.AppendAudit(domain.AuditRecord{ID: "audit.staged.1", OccurredAt: "2031-01-06T10:00:00Z", Operation: "test", Result: "success"}); err != nil {
		t.Fatalf("AppendAudit: %v", err)
	}

	after, _ := os.ReadFile(demandsPath)
	if string(after) != string(before) {
		t.Fatal("a staged write reached the disk before commit")
	}
	if _, err := os.Stat(filepath.Join(projectPath, filepath.FromSlash(actualRel))); !os.IsNotExist(err) {
		t.Fatalf("new actuals file exists before commit: %v", err)
	}

	ws, diags := staged.Load(context.Background())
	for _, d := range diags {
		if d.Severity == "error" {
			t.Fatalf("staged load error: %#v", d)
		}
	}
	if !hasActualID(ws.Actuals, "actual.staged.1") {
		t.Fatal("staged load does not see the new actuals file through glob")
	}
	if !hasAuditID(ws.AuditRecords, "audit.staged.1") {
		t.Fatal("staged load does not see the staged audit record")
	}

	pending := staged.Pending()
	if len(pending) != 3 || !strings.HasPrefix(pending[len(pending)-1], "audit/") {
		t.Fatalf("pending = %#v, want three paths with audit last", pending)
	}
	written, err := staged.Commit()
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if strings.Join(written, ",") != strings.Join(pending, ",") {
		t.Fatalf("written = %#v, pending was %#v", written, pending)
	}
	committed, _ := os.ReadFile(demandsPath)
	if !strings.Contains(string(committed), "remaining_amount: 0\n") {
		t.Fatal("committed demands file lacks the explicit zero")
	}
	if _, err := os.Stat(filepath.Join(projectPath, filepath.FromSlash(actualRel))); err != nil {
		t.Fatalf("actuals file missing after commit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectPath, filepath.FromSlash(commitJournalRelativePath))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("commit journal remains after successful commit: %v", err)
	}
}

func TestInterruptedCommitRollsForwardAtEveryWriteBoundary(t *testing.T) {
	for failAfter := 0; failAfter < 2; failAfter++ {
		t.Run(string(rune('0'+failAfter)), func(t *testing.T) {
			projectPath := copyProjectFixture(t)
			staged := New(projectPath).StageWrites().(*StagedStore)
			evidence := domain.Evidence{ID: "evidence.interrupted", Type: "test_result", ActorRef: domain.Ref{Kind: "actor", ID: "actor.human.operator"}, ObservedAt: "2031-01-06T10:00:00Z", Summary: "Interrupted commit recovery test.", Targets: []domain.Ref{{Kind: "task", ID: "task.op.1234"}}}
			if _, err := staged.AppendEvidence(evidence.ObservedAt, evidence); err != nil {
				t.Fatalf("AppendEvidence: %v", err)
			}
			audit := domain.AuditRecord{ID: "audit.interrupted", OccurredAt: evidence.ObservedAt, ActorRef: domain.Ref{Kind: "actor", ID: "actor.human.operator"}, Operation: "evidence_add", TargetRef: domain.Ref{Kind: "evidence", ID: evidence.ID}, Result: "success"}
			if _, err := staged.AppendAudit(audit); err != nil {
				t.Fatalf("AppendAudit: %v", err)
			}

			staged.stage.disk.commitOperationHook = func(completed int, _ string) error {
				if completed == failAfter {
					return errors.New("injected interruption")
				}
				return nil
			}
			written, err := staged.Commit()
			if err == nil || len(written) != failAfter {
				t.Fatalf("Commit written=%v err=%v, want %d writes and an error", written, err, failAfter)
			}

			_, loadDiags := New(projectPath).Load(context.Background())
			if !hasDiagnostic(loadDiags, "interrupted_commit_pending") {
				t.Fatalf("load diagnostics = %#v, want interrupted_commit_pending", loadDiags)
			}
			recovery, err := RecoverCommit(projectPath)
			if err != nil {
				t.Fatalf("RecoverCommit: %v", err)
			}
			if !recovery.Pending || !recovery.Complete || len(recovery.RecoveredFiles) != 2 {
				t.Fatalf("recovery = %#v", recovery)
			}
			validation, err := app.NewValidateService(New(projectPath)).Validate(context.Background(), app.ValidateRequest{IncludeObjectScan: true})
			if err != nil || !validation.Valid {
				t.Fatalf("validation valid=%t err=%v errors=%#v", validation.Valid, err, validation.Errors)
			}
			verify, err := app.NewAuditService(New(projectPath)).Verify(context.Background(), app.AuditVerifyRequest{})
			if err != nil || !verify.Valid {
				t.Fatalf("audit verify valid=%t err=%v errors=%#v", verify.Valid, err, verify.Errors)
			}
			workspace, diags := New(projectPath).Load(context.Background())
			if len(diags) != 0 {
				t.Fatalf("load diagnostics after recovery: %#v", diags)
			}
			if countEvidenceID(workspace.Evidence, evidence.ID) != 1 || countAuditID(workspace.AuditRecords, audit.ID) != 1 {
				t.Fatalf("evidence or audit was not recovered exactly once")
			}
		})
	}
}

func TestRecoverManifestlessJournalRemovesItWithoutTouchingProject(t *testing.T) {
	projectPath := copyProjectFixture(t)
	tasksPath := filepath.Join(projectPath, "canonical/tasks.yaml")
	before, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(projectPath, filepath.FromSlash(commitJournalRelativePath))
	if err := os.MkdirAll(filepath.Join(journalPath, "blobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalPath, "blobs", "000000"), []byte("incomplete"), 0o600); err != nil {
		t.Fatal(err)
	}
	recovery, err := RecoverCommit(projectPath)
	if err != nil {
		t.Fatalf("RecoverCommit: %v", err)
	}
	if !recovery.Pending || recovery.Complete || !recovery.IncompleteRemoved || len(recovery.RecoveredFiles) != 0 {
		t.Fatalf("recovery = %#v", recovery)
	}
	after, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("project file changed while removing manifest-less journal")
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal still exists: %v", err)
	}
}

func TestCorruptCommitJournalRefusesBeforeChangingProject(t *testing.T) {
	projectPath := copyProjectFixture(t)
	staged := New(projectPath).StageWrites().(*StagedStore)
	evidence := domain.Evidence{ID: "evidence.corrupt", Type: "test_result", ActorRef: domain.Ref{Kind: "actor", ID: "actor.human.operator"}, ObservedAt: "2031-01-06T10:00:00Z", Summary: "Must not be recovered.", Targets: []domain.Ref{{Kind: "task", ID: "task.op.1234"}}}
	if _, err := staged.AppendEvidence(evidence.ObservedAt, evidence); err != nil {
		t.Fatal(err)
	}
	staged.stage.disk.commitOperationHook = func(int, string) error { return errors.New("injected interruption") }
	if _, err := staged.Commit(); err == nil {
		t.Fatal("Commit succeeded, want injected interruption")
	}

	evidencePath := filepath.Join(projectPath, filepath.FromSlash(evidencePath(evidence.ObservedAt)))
	before, err := os.ReadFile(evidencePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	blobPath := filepath.Join(projectPath, filepath.FromSlash(commitJournalRelativePath), "blobs", "000000")
	if err := os.WriteFile(blobPath, []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoverCommit(projectPath); !errors.Is(err, ErrCommitJournalCorrupt) || !strings.Contains(err.Error(), ".replan/commit-pending/") {
		t.Fatalf("RecoverCommit error = %v", err)
	}
	after, err := os.ReadFile(evidencePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("corrupt recovery changed a project file")
	}
}

func hasDiagnostic(diagnostics []app.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func countEvidenceID(evidence []domain.Evidence, id string) int {
	count := 0
	for _, record := range evidence {
		if record.ID == id {
			count++
		}
	}
	return count
}

func countAuditID(records []domain.AuditRecord, id string) int {
	count := 0
	for _, record := range records {
		if record.ID == id {
			count++
		}
	}
	return count
}

func TestStagedProposalMoveRemovesSourceOnlyOnCommit(t *testing.T) {
	projectPath := copyProjectFixture(t)
	matches, _ := filepath.Glob(filepath.Join(projectPath, "proposals/open/*.yaml"))
	if len(matches) == 0 {
		t.Skip("fixture has no open proposal")
	}
	proposalID := strings.TrimSuffix(filepath.Base(matches[0]), ".yaml")

	staged := New(projectPath).StageWrites().(*StagedStore)
	if _, err := staged.MoveProposal(proposalID, "rejected", domain.AuditRecord{ID: "audit.staged.move", OccurredAt: "2031-01-06T10:00:00Z", Operation: "test", Result: "success"}); err != nil {
		t.Fatalf("MoveProposal: %v", err)
	}
	if _, err := os.Stat(matches[0]); err != nil {
		t.Fatalf("source proposal removed before commit: %v", err)
	}
	ws, _ := staged.Load(context.Background())
	for _, proposal := range ws.Proposals {
		if proposal.ID == proposalID && proposal.Status != "rejected" {
			t.Fatalf("staged load still sees the proposal as %s", proposal.Status)
		}
	}
	if _, err := staged.Commit(); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if _, err := os.Stat(matches[0]); !os.IsNotExist(err) {
		t.Fatalf("source proposal still on disk after commit: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projectPath, "proposals/rejected", proposalID+".yaml")); err != nil {
		t.Fatalf("target proposal missing after commit: %v", err)
	}
}

func hasActualID(actuals []domain.Actual, id string) bool {
	for _, actual := range actuals {
		if actual.ID == id {
			return true
		}
	}
	return false
}

func hasAuditID(records []domain.AuditRecord, id string) bool {
	for _, record := range records {
		if record.ID == id {
			return true
		}
	}
	return false
}
