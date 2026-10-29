package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
