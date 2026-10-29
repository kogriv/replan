package app

import (
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func TestPrivateOpenProjectIDDiagnosticsRefuseUnmatchedAndUnknownChanges(t *testing.T) {
	changes := []ImportAcceptChange{
		{Op: "create_evidence", SourceID: "9301"},
		{Op: "create_actual", SourceID: "9301"},
		{Op: "create_future_thing", TargetID: "thing.one"},
		{Op: "update_task_planned_start", TargetID: "task.dated"},
		{Op: "create_resource_demand", TargetID: "resource_demand.one"},
	}

	diagnostics := privateOpenProjectIDDiagnostics(domain.Workspace{}, OpenProjectMappedSnapshot{}, changes)

	if len(diagnostics) != 3 {
		t.Fatalf("diagnostics = %#v, want the two time-entry kinds and the unclassified kind", diagnostics)
	}
	for _, diagnostic := range diagnostics {
		if diagnostic.Code != "openproject_ids_private" {
			t.Fatalf("diagnostic code = %q", diagnostic.Code)
		}
	}
}
