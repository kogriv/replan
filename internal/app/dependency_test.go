package app_test

import (
	"context"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestDependencyListSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewDependencyService(store)

	result, err := service.List(context.Background(), app.DependencyListRequest{ProjectPath: "../../examples/small-team"})

	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if result.Summary.Dependencies != 2 {
		t.Fatalf("Dependencies = %d, want 2", result.Summary.Dependencies)
	}
	if result.Summary.RequiredOpen != 2 {
		t.Fatalf("RequiredOpen = %d, want 2", result.Summary.RequiredOpen)
	}
	if result.Summary.ActiveBlockers != 2 {
		t.Fatalf("ActiveBlockers = %d, want 2", result.Summary.ActiveBlockers)
	}
	if result.Summary.ReadyToUnblockTasks != 0 {
		t.Fatalf("ReadyToUnblockTasks = %d, want 0", result.Summary.ReadyToUnblockTasks)
	}
	if !hasDependencyRow(result.Dependencies, "dependency.openai-quota-blocks-validation", true, false) {
		t.Fatalf("expected active quota blocker row, got %#v", result.Dependencies)
	}
}

func TestDependencyListDetectsReadyToUnblock(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	markValidationBlockedAndDependenciesClosed(t, projectPath)
	store := filestore.New(projectPath)
	service := app.NewDependencyService(store)

	result, err := service.List(context.Background(), app.DependencyListRequest{ProjectPath: projectPath})

	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if result.Summary.ActiveBlockers != 0 {
		t.Fatalf("ActiveBlockers = %d, want 0", result.Summary.ActiveBlockers)
	}
	if result.Summary.ReadyToUnblockTasks != 1 {
		t.Fatalf("ReadyToUnblockTasks = %d, want 1", result.Summary.ReadyToUnblockTasks)
	}
	if len(result.BlockedSuccessors) != 1 || !result.BlockedSuccessors[0].ReadyToUnblock {
		t.Fatalf("expected ready-to-unblock successor, got %#v", result.BlockedSuccessors)
	}
}

func hasDependencyRow(rows []app.DependencyRow, id string, blocking bool, resolvable bool) bool {
	for _, row := range rows {
		if row.ID == id && row.Blocking == blocking && row.Resolvable == resolvable {
			return true
		}
	}
	return false
}
