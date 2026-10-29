package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestImportAcceptDryRunFromOpenProjectSnapshot(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	importResult, err := app.NewOpenProjectImportService(store).Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		Now:      time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: importResult.Snapshot.ID,
		DryRun:     true,
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if result.MappedPath != "imports/openproject/20260710T123000/mapped.json" {
		t.Fatalf("MappedPath = %q", result.MappedPath)
	}
	if len(result.Changes) != 3 {
		t.Fatalf("Changes len = %d, want 3: %#v", len(result.Changes), result.Changes)
	}
	if !hasImportChange(result.Changes, "update_task_tracking_status", "task.op.1234") {
		t.Fatalf("expected update_task_tracking_status for task.op.1234: %#v", result.Changes)
	}
	if !hasImportChange(result.Changes, "update_task_source_refs", "task.op.1234") {
		t.Fatalf("expected update_task_source_refs for task.op.1234: %#v", result.Changes)
	}
	if !hasImportChange(result.Changes, "create_task", "task.op.1235") {
		t.Fatalf("expected create_task for task.op.1235: %#v", result.Changes)
	}
	if len(result.PlannedWrites) != 1 {
		t.Fatalf("PlannedWrites len = %d, want 1: %#v", len(result.PlannedWrites), result.PlannedWrites)
	}
	if result.PlannedWrites[0].Path != "canonical/tasks.yaml" {
		t.Fatalf("planned write path = %q, want canonical/tasks.yaml", result.PlannedWrites[0].Path)
	}
}

func TestImportAcceptPrivateBindingsRefusesEveryIDCarryingChangeKind(t *testing.T) {
	type acceptCase struct {
		name      string
		snapshot  func(*testing.T, string) string
		configure func(*app.ImportAcceptRequest)
		wantOps   []string
	}
	cases := []acceptCase{
		{name: "tasks", snapshot: func(t *testing.T, dir string) string {
			result, err := app.NewOpenProjectImportService(filestore.New(dir)).Import(context.Background(), app.OpenProjectImportRequest{FromFile: "../../examples/imports/openproject/work-packages.sample.json", Now: time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC)})
			if err != nil {
				t.Fatal(err)
			}
			return result.Snapshot.ID
		}, configure: func(*app.ImportAcceptRequest) {}, wantOps: []string{"update_task_source_refs", "create_task"}},
		{name: "resource link", snapshot: func(t *testing.T, dir string) string {
			return writeOpenProjectMappedSnapshotForResourceLink(t, dir, "resource.human.ivan", "11")
		}, configure: func(req *app.ImportAcceptRequest) { req.AcceptResourceLinks = true }, wantOps: []string{"add_resource_source_ref"}},
		{name: "time entry", snapshot: writeOpenProjectMappedSnapshotForTimeEntry, configure: func(req *app.ImportAcceptRequest) { req.AcceptTimeEntries = true }, wantOps: []string{"create_evidence", "create_actual"}},
		{name: "new dependency", snapshot: func(t *testing.T, dir string) string {
			return writeOpenProjectMappedSnapshotForDependency(t, dir, "safe_to_accept", true, "task.op.1234", "task.offline.repair-pump", "88")
		}, configure: func(req *app.ImportAcceptRequest) { req.AcceptDependencies = true }, wantOps: []string{"create_dependency"}},
		{name: "existing dependency source ref", snapshot: func(t *testing.T, dir string) string {
			return writeOpenProjectMappedSnapshotForDependency(t, dir, "safe_to_accept", true, "task.offline.repair-pump", "task.op.1234", "88")
		}, configure: func(req *app.ImportAcceptRequest) { req.AcceptDependencies = true }, wantOps: []string{"update_dependency_source_refs"}},
	}
	for _, tc := range cases {
		for _, apply := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/apply=%t", tc.name, apply), func(t *testing.T) {
				dir := copyProjectFixture(t)
				id := tc.snapshot(t, dir)
				setProjectOpenProjectIDs(t, dir, domain.OpenProjectIDsPrivateBindings)
				before := snapshotFiles(t, dir)
				req := app.ImportAcceptRequest{SnapshotID: id, DryRun: !apply, Apply: apply, ActorID: "actor.human.operator"}
				tc.configure(&req)
				result, err := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), req)
				if !errors.Is(err, app.ErrValidationFailed) {
					t.Fatalf("Accept error = %v, want ErrValidationFailed", err)
				}
				if result.Applied || len(result.WrittenFiles) != 0 {
					t.Fatalf("refused accept wrote files: %#v", result)
				}
				for _, op := range tc.wantOps {
					if !hasPrivateIDDiagnosticForOp(result.Errors, op) {
						t.Fatalf("missing openproject_ids_private diagnostic for %s: %#v", op, result.Errors)
					}
				}
				if after := snapshotFiles(t, dir); !reflect.DeepEqual(after, before) {
					t.Fatalf("refused accept changed project files")
				}
			})
		}
	}
}

func TestImportAcceptPrivateBindingsAllowsNonIDChangesToApply(t *testing.T) {
	cases := []struct {
		name      string
		snapshot  func(*testing.T, string) string
		configure func(*app.ImportAcceptRequest)
	}{
		{"dates", func(t *testing.T, dir string) string {
			return writeOpenProjectMappedSnapshotForDates(t, dir, "task.op.1234", "2026-07-08", "2026-07-12")
		}, func(req *app.ImportAcceptRequest) { req.AcceptDates = true }},
		{"tracking status", func(t *testing.T, dir string) string {
			return writeOpenProjectMappedSnapshotForTrackingStatus(t, dir, "in_progress", "task.op.1234")
		}, func(req *app.ImportAcceptRequest) { req.AcceptTrackingStatus = true }},
		{"resource demand", func(t *testing.T, dir string) string {
			return writeOpenProjectMappedSnapshotForResourceDemand(t, dir, "safe_to_accept", "task.op.1234", "resource.human.ivan")
		}, func(req *app.ImportAcceptRequest) { req.AcceptResourceDemands = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := copyProjectFixture(t)
			id := tc.snapshot(t, dir)
			setProjectOpenProjectIDs(t, dir, domain.OpenProjectIDsPrivateBindings)
			req := app.ImportAcceptRequest{SnapshotID: id, Apply: true, ActorID: "actor.human.operator"}
			tc.configure(&req)
			result, err := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), req)
			if err != nil || !result.Applied {
				t.Fatalf("Accept = %#v, error = %v", result, err)
			}
		})
	}
}

func TestImportAcceptRequiresDryRun(t *testing.T) {
	service := app.NewImportAcceptService(filestore.New("../../examples/small-team"))

	result, err := service.Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: "source_snapshot.openproject.20260710T123000",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Accept error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "dry_run_required") {
		t.Fatalf("expected dry_run_required error")
	}
}

func TestImportAcceptApplyWritesSafeTaskChanges(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	importResult, err := app.NewOpenProjectImportService(store).Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		Now:      time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: importResult.Snapshot.ID,
		Apply:      true,
		ActorID:    "actor.human.operator",
		Reason:     "Accept OpenProject import for test.",
		Now:        time.Date(2026, 7, 10, 12, 45, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if len(result.AppliedChanges) != 2 {
		t.Fatalf("AppliedChanges len = %d, want 2: %#v", len(result.AppliedChanges), result.AppliedChanges)
	}
	if !hasImportChange(result.AppliedChanges, "update_task_source_refs", "task.op.1234") {
		t.Fatalf("expected applied source ref update: %#v", result.AppliedChanges)
	}
	if !hasImportChange(result.AppliedChanges, "create_task", "task.op.1235") {
		t.Fatalf("expected applied create task: %#v", result.AppliedChanges)
	}
	if hasImportChange(result.AppliedChanges, "update_task_tracking_status", "task.op.1234") {
		t.Fatalf("status update must not be applied in first write-mode slice")
	}
	tasksYAML, err := os.ReadFile(filepath.Join(projectDir, "canonical/tasks.yaml"))
	if err != nil {
		t.Fatalf("read tasks: %v", err)
	}
	tasksText := string(tasksYAML)
	if !containsString(tasksText, "id: task.op.1235") {
		t.Fatalf("tasks.yaml does not contain created task:\n%s", tasksText)
	}
	if !containsString(tasksText, "object_id: \"1234\"") {
		t.Fatalf("tasks.yaml does not contain source ref for 1234:\n%s", tasksText)
	}
	if !containsString(tasksText, "tracking_status: ready") {
		t.Fatalf("existing ready status should remain present:\n%s", tasksText)
	}
	auditJSONL, err := os.ReadFile(filepath.Join(projectDir, "audit/audit.2026-07.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !containsString(string(auditJSONL), `"operation":"import_accept"`) {
		t.Fatalf("audit does not contain import_accept:\n%s", auditJSONL)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected valid validation result, got %#v", result.Validation)
	}
}

func TestImportAcceptTimeEntriesDryRun(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForTimeEntry(t, projectDir)

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:        snapshotID,
		DryRun:            true,
		AcceptTimeEntries: true,
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !hasImportChange(result.Changes, "create_evidence", "evidence.openproject.time-entry.77") {
		t.Fatalf("expected create_evidence: %#v", result.Changes)
	}
	if !hasImportChange(result.Changes, "create_actual", "actual.openproject.time-entry.77.work-consumed") {
		t.Fatalf("expected create_actual: %#v", result.Changes)
	}
	if hasImportChange(result.Changes, "update_task_tracking_status", "task.op.1234") {
		t.Fatalf("time-entry accept mode must not include ordinary task changes: %#v", result.Changes)
	}
	if !hasPlannedWrite(result.PlannedWrites, "evidence/evidence.2026-W30.yaml") {
		t.Fatalf("expected evidence planned write: %#v", result.PlannedWrites)
	}
	if !hasPlannedWrite(result.PlannedWrites, "actuals/actuals.2026-W30.yaml") {
		t.Fatalf("expected actuals planned write: %#v", result.PlannedWrites)
	}
}

func TestImportAcceptApplyTimeEntriesWritesActualEvidenceAndAudit(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForTimeEntry(t, projectDir)

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:        snapshotID,
		Apply:             true,
		AcceptTimeEntries: true,
		ActorID:           "actor.human.operator",
		Reason:            "Accept OpenProject time entries for test.",
		Now:               time.Date(2026, 7, 23, 18, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if !hasImportChange(result.AppliedChanges, "create_evidence", "evidence.openproject.time-entry.77") {
		t.Fatalf("expected applied create_evidence: %#v", result.AppliedChanges)
	}
	if !hasImportChange(result.AppliedChanges, "create_actual", "actual.openproject.time-entry.77.work-consumed") {
		t.Fatalf("expected applied create_actual: %#v", result.AppliedChanges)
	}
	evidenceYAML, err := os.ReadFile(filepath.Join(projectDir, "evidence/evidence.2026-W30.yaml"))
	if err != nil {
		t.Fatalf("read evidence: %v", err)
	}
	if !containsString(string(evidenceYAML), "id: evidence.openproject.time-entry.77") {
		t.Fatalf("evidence file missing time entry evidence:\n%s", evidenceYAML)
	}
	if !containsString(string(evidenceYAML), "object_type: time_entry") {
		t.Fatalf("evidence file missing source ref:\n%s", evidenceYAML)
	}
	actualsYAML, err := os.ReadFile(filepath.Join(projectDir, "actuals/actuals.2026-W30.yaml"))
	if err != nil {
		t.Fatalf("read actuals: %v", err)
	}
	if !containsString(string(actualsYAML), "id: actual.openproject.time-entry.77.work-consumed") {
		t.Fatalf("actuals file missing imported actual:\n%s", actualsYAML)
	}
	if !containsString(string(actualsYAML), "id: resource_demand.task-op-1234.implementer") {
		t.Fatalf("actuals file missing demand ref:\n%s", actualsYAML)
	}
	auditJSONL, err := os.ReadFile(filepath.Join(projectDir, "audit/audit.2026-07.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !containsString(string(auditJSONL), `"operation":"import_accept"`) {
		t.Fatalf("audit does not contain import_accept:\n%s", auditJSONL)
	}

	duplicate, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:        snapshotID,
		Apply:             true,
		AcceptTimeEntries: true,
		ActorID:           "actor.human.operator",
	})
	if err != nil {
		t.Fatalf("duplicate Accept returned error: %v", err)
	}
	if duplicate.Applied {
		t.Fatalf("duplicate Applied = true, want false")
	}
	if !hasDiagnostic(duplicate.Warnings, "openproject_time_entry_already_imported") {
		t.Fatalf("expected duplicate warning: %#v", duplicate.Warnings)
	}
}

func TestImportAcceptResourceDemandsDryRun(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForResourceDemand(t, projectDir, "safe_to_accept", "task.op.1234", "resource.human.ivan")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:            snapshotID,
		DryRun:                true,
		AcceptResourceDemands: true,
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !hasImportChange(result.Changes, "create_resource_demand", "resource_demand.op-1234.ivan.sidecar") {
		t.Fatalf("expected create_resource_demand: %#v", result.Changes)
	}
	if hasImportChange(result.Changes, "create_evidence", "evidence.openproject.time-entry.77") {
		t.Fatalf("resource-demand accept mode must not include time-entry changes: %#v", result.Changes)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/resource_demands.yaml") {
		t.Fatalf("expected resource demands planned write: %#v", result.PlannedWrites)
	}
}

func TestImportAcceptApplyResourceDemandsWritesDemandAndAudit(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForResourceDemand(t, projectDir, "safe_to_accept", "task.op.1234", "resource.human.ivan")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:            snapshotID,
		Apply:                 true,
		AcceptResourceDemands: true,
		ActorID:               "actor.human.operator",
		Reason:                "Accept OpenProject resource demands for test.",
		Now:                   time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if !hasImportChange(result.AppliedChanges, "create_resource_demand", "resource_demand.op-1234.ivan.sidecar") {
		t.Fatalf("expected applied create_resource_demand: %#v", result.AppliedChanges)
	}
	demandsYAML, err := os.ReadFile(filepath.Join(projectDir, "canonical/resource_demands.yaml"))
	if err != nil {
		t.Fatalf("read resource demands: %v", err)
	}
	if !containsString(string(demandsYAML), "id: resource_demand.op-1234.ivan.sidecar") {
		t.Fatalf("resource_demands.yaml missing imported demand:\n%s", demandsYAML)
	}
	if !containsString(string(demandsYAML), "planned_amount: 6") {
		t.Fatalf("resource_demands.yaml missing planned amount:\n%s", demandsYAML)
	}
	auditJSONL, err := os.ReadFile(filepath.Join(projectDir, "audit/audit.2026-07.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !containsString(string(auditJSONL), `"operation":"import_accept"`) {
		t.Fatalf("audit does not contain import_accept:\n%s", auditJSONL)
	}

	duplicate, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:            snapshotID,
		Apply:                 true,
		AcceptResourceDemands: true,
		ActorID:               "actor.human.operator",
	})
	if err != nil {
		t.Fatalf("duplicate Accept returned error: %v", err)
	}
	if duplicate.Applied {
		t.Fatalf("duplicate Applied = true, want false")
	}
	if !hasDiagnostic(duplicate.Warnings, "openproject_resource_demand_already_exists") {
		t.Fatalf("expected duplicate warning: %#v", duplicate.Warnings)
	}
}

func TestImportAcceptResourceDemandsSkipsUnsafeCandidates(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForResourceDemand(t, projectDir, "resource_unmatched", "task.op.1234", "resource.human.missing")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:            snapshotID,
		Apply:                 true,
		AcceptResourceDemands: true,
		ActorID:               "actor.human.operator",
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if !hasDiagnostic(result.Warnings, "openproject_resource_demand_not_safe_to_accept") {
		t.Fatalf("expected not safe warning: %#v", result.Warnings)
	}
}

func TestImportAcceptResourceLinksDryRun(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForResourceLink(t, projectDir, "resource.human.ivan", "11")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:          snapshotID,
		DryRun:              true,
		AcceptResourceLinks: true,
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !hasImportChange(result.Changes, "add_resource_source_ref", "resource.human.ivan") {
		t.Fatalf("expected add_resource_source_ref: %#v", result.Changes)
	}
	if hasImportChange(result.Changes, "create_resource_demand", "resource_demand.op-1234.ivan.sidecar") {
		t.Fatalf("resource-link accept mode must not include resource-demand changes: %#v", result.Changes)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/resources.yaml") {
		t.Fatalf("expected resources planned write: %#v", result.PlannedWrites)
	}
}

func TestImportAcceptApplyResourceLinksWritesSourceRefAndAudit(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForResourceLink(t, projectDir, "resource.human.ivan", "11")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:          snapshotID,
		Apply:               true,
		AcceptResourceLinks: true,
		ActorID:             "actor.human.operator",
		Reason:              "Accept OpenProject resource links for test.",
		Now:                 time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if !hasImportChange(result.AppliedChanges, "add_resource_source_ref", "resource.human.ivan") {
		t.Fatalf("expected applied add_resource_source_ref: %#v", result.AppliedChanges)
	}
	resourcesYAML, err := os.ReadFile(filepath.Join(projectDir, "canonical/resources.yaml"))
	if err != nil {
		t.Fatalf("read resources: %v", err)
	}
	resourcesText := string(resourcesYAML)
	if !containsString(resourcesText, "object_type: user") {
		t.Fatalf("resources.yaml missing OpenProject user source type:\n%s", resourcesText)
	}
	if !containsString(resourcesText, "object_id: \"11\"") {
		t.Fatalf("resources.yaml missing OpenProject user source id:\n%s", resourcesText)
	}
	if !containsString(resourcesText, "url: /api/v3/users/11") {
		t.Fatalf("resources.yaml missing OpenProject user source url:\n%s", resourcesText)
	}
	auditJSONL, err := os.ReadFile(filepath.Join(projectDir, "audit/audit.2026-07.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !containsString(string(auditJSONL), `"operation":"import_accept"`) {
		t.Fatalf("audit does not contain import_accept:\n%s", auditJSONL)
	}

	duplicate, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:          snapshotID,
		Apply:               true,
		AcceptResourceLinks: true,
		ActorID:             "actor.human.operator",
	})
	if err != nil {
		t.Fatalf("duplicate Accept returned error: %v", err)
	}
	if duplicate.Applied {
		t.Fatalf("duplicate Applied = true, want false")
	}
	if !hasDiagnostic(duplicate.Warnings, "openproject_resource_link_already_exists") {
		t.Fatalf("expected duplicate warning: %#v", duplicate.Warnings)
	}
}

func TestImportAcceptResourceLinksSkipsUnsafeCandidates(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForResourceLink(t, projectDir, "", "11")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:          snapshotID,
		Apply:               true,
		AcceptResourceLinks: true,
		ActorID:             "actor.human.operator",
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if !hasDiagnostic(result.Warnings, "openproject_resource_link_resource_unmatched") {
		t.Fatalf("expected unmatched resource warning: %#v", result.Warnings)
	}
}

func TestImportAcceptResourceLinksRequiresExplicitResourceLinkMatch(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForResourceLink(t, projectDir, "resource.human.ivan", "11")
	rewriteMappedSnapshotResourceLinkMatchReason(t, projectDir, "20260725T120000", "name")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:          snapshotID,
		Apply:               true,
		AcceptResourceLinks: true,
		ActorID:             "actor.human.operator",
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if !hasDiagnostic(result.Warnings, "openproject_resource_link_not_explicit") {
		t.Fatalf("expected explicit-link warning: %#v", result.Warnings)
	}
}

func TestImportAcceptTrackingStatusDryRun(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForTrackingStatus(t, projectDir, "in_progress", "task.op.1234")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:           snapshotID,
		DryRun:               true,
		AcceptTrackingStatus: true,
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !hasImportChange(result.Changes, "update_task_tracking_status", "task.op.1234") {
		t.Fatalf("expected update_task_tracking_status: %#v", result.Changes)
	}
	if hasImportChange(result.Changes, "update_task_source_refs", "task.op.1234") {
		t.Fatalf("tracking-status accept mode must not include source-ref updates: %#v", result.Changes)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/tasks.yaml") {
		t.Fatalf("expected tasks planned write: %#v", result.PlannedWrites)
	}
}

func TestImportAcceptApplyTrackingStatusWritesStatusAndAudit(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForTrackingStatus(t, projectDir, "in_progress", "task.op.1234")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:           snapshotID,
		Apply:                true,
		AcceptTrackingStatus: true,
		ActorID:              "actor.human.operator",
		Reason:               "Accept OpenProject tracking status for test.",
		Now:                  time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if !hasImportChange(result.AppliedChanges, "update_task_tracking_status", "task.op.1234") {
		t.Fatalf("expected applied status update: %#v", result.AppliedChanges)
	}
	tasksYAML, err := os.ReadFile(filepath.Join(projectDir, "canonical/tasks.yaml"))
	if err != nil {
		t.Fatalf("read tasks: %v", err)
	}
	if !containsString(string(tasksYAML), "tracking_status: in_progress") {
		t.Fatalf("tasks.yaml missing updated tracking status:\n%s", tasksYAML)
	}
	auditJSONL, err := os.ReadFile(filepath.Join(projectDir, "audit/audit.2026-07.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !containsString(string(auditJSONL), `"operation":"import_accept"`) {
		t.Fatalf("audit does not contain import_accept:\n%s", auditJSONL)
	}

	duplicate, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:           snapshotID,
		Apply:                true,
		AcceptTrackingStatus: true,
		ActorID:              "actor.human.operator",
	})
	if err != nil {
		t.Fatalf("duplicate Accept returned error: %v", err)
	}
	if duplicate.Applied {
		t.Fatalf("duplicate Applied = true, want false")
	}
	if !hasDiagnostic(duplicate.Warnings, "openproject_tracking_status_already_current") {
		t.Fatalf("expected duplicate warning: %#v", duplicate.Warnings)
	}
}

func TestImportAcceptTrackingStatusSkipsUnsafeCandidates(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForTrackingStatus(t, projectDir, "in_progress", "")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:           snapshotID,
		Apply:                true,
		AcceptTrackingStatus: true,
		ActorID:              "actor.human.operator",
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if !hasDiagnostic(result.Warnings, "openproject_tracking_status_task_unmatched") {
		t.Fatalf("expected unmatched task warning: %#v", result.Warnings)
	}
}

func TestImportAcceptDatesDryRunShowsEachChangedField(t *testing.T) {
	projectDir := copyProjectFixture(t)
	snapshotID := writeOpenProjectMappedSnapshotForDates(t, projectDir, "task.op.1234", "2026-07-08", "2026-07-12")

	result, err := app.NewImportAcceptService(filestore.New(projectDir)).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: snapshotID, DryRun: true, AcceptDates: true,
	})
	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if result.Applied {
		t.Fatal("dry run applied changes")
	}
	if !hasImportChange(result.Changes, "update_task_planned_start", "task.op.1234") || !hasImportChange(result.Changes, "update_task_planned_finish", "task.op.1234") {
		t.Fatalf("expected both date changes: %#v", result.Changes)
	}
	for _, change := range result.Changes {
		if change.Field == "" || change.From == nil || change.To == nil {
			t.Fatalf("date change does not name field/from/to: %#v", change)
		}
	}
	tasks, err := os.ReadFile(filepath.Join(projectDir, "canonical/tasks.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(tasks), "planned_finish: \"2026-07-12\"") {
		t.Fatal("dry run changed tasks.yaml")
	}
}

func TestImportAcceptDatesApplyWritesDatesAndOneAudit(t *testing.T) {
	projectDir := copyProjectFixture(t)
	snapshotID := writeOpenProjectMappedSnapshotForDates(t, projectDir, "task.op.1234", "2026-07-08", "2026-07-12")

	result, err := app.NewImportAcceptService(filestore.New(projectDir)).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: snapshotID, Apply: true, AcceptDates: true, ActorID: "actor.human.operator",
		Reason: "Accept reviewed OpenProject dates.", Now: time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied || len(result.AppliedChanges) != 2 {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	tasks, err := os.ReadFile(filepath.Join(projectDir, "canonical/tasks.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(tasks), "planned_start: \"2026-07-08\"") || !strings.Contains(string(tasks), "planned_finish: \"2026-07-12\"") {
		t.Fatalf("tasks.yaml missing accepted dates:\n%s", tasks)
	}
	audit, err := os.ReadFile(filepath.Join(projectDir, "audit/audit.2026-07.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(audit), `"operation":"import_accept"`) != 1 || !strings.Contains(string(audit), `"actor_ref":{"kind":"actor","id":"actor.human.operator"}`) {
		t.Fatalf("expected one actor-attributed import audit record:\n%s", audit)
	}
}

func TestImportAcceptDatesReportsNoChange(t *testing.T) {
	projectDir := copyProjectFixture(t)
	if _, err := filestore.New(projectDir).SetTaskDates("task.op.1234", "2026-07-06", "2026-07-10"); err != nil {
		t.Fatal(err)
	}
	snapshotID := writeOpenProjectMappedSnapshotForDates(t, projectDir, "task.op.1234", "2026-07-06", "2026-07-10")
	result, err := app.NewImportAcceptService(filestore.New(projectDir)).Accept(context.Background(), app.ImportAcceptRequest{SnapshotID: snapshotID, DryRun: true, AcceptDates: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 0 || !hasDiagnostic(result.Warnings, "openproject_dates_already_current") {
		t.Fatalf("expected already-current skip: %#v", result)
	}
}

func TestImportAcceptDatesReportsCandidateWithNoDates(t *testing.T) {
	projectDir := copyProjectFixture(t)
	snapshotID := writeOpenProjectMappedSnapshotForDates(t, projectDir, "task.op.1234", "", "")
	result, err := app.NewImportAcceptService(filestore.New(projectDir)).Accept(context.Background(), app.ImportAcceptRequest{SnapshotID: snapshotID, DryRun: true, AcceptDates: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 0 || !hasDiagnostic(result.Warnings, "openproject_dates_missing") {
		t.Fatalf("expected no-dates skip: %#v", result)
	}
}

func TestImportAcceptDatesReportsUnmatchedCandidate(t *testing.T) {
	projectDir := copyProjectFixture(t)
	snapshotID := writeOpenProjectMappedSnapshotForDates(t, projectDir, "", "2026-07-08", "2026-07-12")
	result, err := app.NewImportAcceptService(filestore.New(projectDir)).Accept(context.Background(), app.ImportAcceptRequest{SnapshotID: snapshotID, DryRun: true, AcceptDates: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 0 || !hasDiagnostic(result.Warnings, "openproject_dates_task_unmatched") {
		t.Fatalf("expected unmatched skip: %#v", result)
	}
}

func TestImportAcceptDatesReportsBadRange(t *testing.T) {
	projectDir := copyProjectFixture(t)
	snapshotID := writeOpenProjectMappedSnapshotForDates(t, projectDir, "task.op.1234", "2026-07-12", "2026-07-08")
	result, err := app.NewImportAcceptService(filestore.New(projectDir)).Accept(context.Background(), app.ImportAcceptRequest{SnapshotID: snapshotID, DryRun: true, AcceptDates: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 0 || !hasDiagnostic(result.Warnings, "openproject_dates_finish_before_start") {
		t.Fatalf("expected bad-range skip: %#v", result)
	}
}

func TestImportAcceptDependenciesDryRun(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForDependency(t, projectDir, "safe_to_accept", true, "task.op.1234", "task.offline.repair-pump", "88")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         snapshotID,
		DryRun:             true,
		AcceptDependencies: true,
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !hasImportChange(result.Changes, "create_dependency", "dependency.openproject.relation.88") {
		t.Fatalf("expected create_dependency: %#v", result.Changes)
	}
	if hasImportChange(result.Changes, "update_task_source_refs", "task.op.1234") {
		t.Fatalf("dependency accept mode must not include ordinary task changes: %#v", result.Changes)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/dependencies.yaml") {
		t.Fatalf("expected dependencies planned write: %#v", result.PlannedWrites)
	}
}

func TestImportAcceptApplyDependenciesWritesDependencyAndAudit(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForDependency(t, projectDir, "safe_to_accept", true, "task.op.1234", "task.offline.repair-pump", "88")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         snapshotID,
		Apply:              true,
		AcceptDependencies: true,
		ActorID:            "actor.human.operator",
		Reason:             "Accept OpenProject dependencies for test.",
		Now:                time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if !hasImportChange(result.AppliedChanges, "create_dependency", "dependency.openproject.relation.88") {
		t.Fatalf("expected applied create_dependency: %#v", result.AppliedChanges)
	}
	dependenciesYAML, err := os.ReadFile(filepath.Join(projectDir, "canonical/dependencies.yaml"))
	if err != nil {
		t.Fatalf("read dependencies: %v", err)
	}
	dependenciesText := string(dependenciesYAML)
	if !containsString(dependenciesText, "id: dependency.openproject.relation.88") {
		t.Fatalf("dependencies.yaml missing imported dependency:\n%s", dependenciesText)
	}
	if !containsString(dependenciesText, "object_type: relation") {
		t.Fatalf("dependencies.yaml missing OpenProject relation source ref:\n%s", dependenciesText)
	}
	if !containsString(dependenciesText, "object_id: \"88\"") {
		t.Fatalf("dependencies.yaml missing OpenProject relation id:\n%s", dependenciesText)
	}
	auditJSONL, err := os.ReadFile(filepath.Join(projectDir, "audit/audit.2026-07.jsonl"))
	if err != nil {
		t.Fatalf("read audit: %v", err)
	}
	if !containsString(string(auditJSONL), `"operation":"import_accept"`) {
		t.Fatalf("audit does not contain import_accept:\n%s", auditJSONL)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected valid validation result, got %#v", result.Validation)
	}

	duplicate, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         snapshotID,
		Apply:              true,
		AcceptDependencies: true,
		ActorID:            "actor.human.operator",
	})
	if err != nil {
		t.Fatalf("duplicate Accept returned error: %v", err)
	}
	if duplicate.Applied {
		t.Fatalf("duplicate Applied = true, want false")
	}
	if !hasDiagnostic(duplicate.Warnings, "openproject_dependency_already_exists") {
		t.Fatalf("expected duplicate dependency warning: %#v", duplicate.Warnings)
	}
}

func TestImportAcceptDependenciesDryRunLinksExistingDependencySourceRef(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForDependency(t, projectDir, "safe_to_accept", true, "task.offline.repair-pump", "task.op.1234", "88")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         snapshotID,
		DryRun:             true,
		AcceptDependencies: true,
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !hasImportChange(result.Changes, "update_dependency_source_refs", "dependency.repair-pump-before-validation") {
		t.Fatalf("expected update_dependency_source_refs: %#v", result.Changes)
	}
	if hasImportChange(result.Changes, "create_dependency", "dependency.openproject.relation.88") {
		t.Fatalf("expected no duplicate dependency creation: %#v", result.Changes)
	}
	if hasDiagnostic(result.Warnings, "openproject_dependency_duplicate_triple") {
		t.Fatalf("expected duplicate triple to be linkable, got warnings: %#v", result.Warnings)
	}
	if !hasPlannedWrite(result.PlannedWrites, "canonical/dependencies.yaml") {
		t.Fatalf("expected dependencies planned write: %#v", result.PlannedWrites)
	}
}

func TestImportAcceptApplyDependenciesLinksExistingDependencySourceRef(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForDependency(t, projectDir, "safe_to_accept", true, "task.offline.repair-pump", "task.op.1234", "88")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         snapshotID,
		Apply:              true,
		AcceptDependencies: true,
		ActorID:            "actor.human.operator",
		Reason:             "Link OpenProject relation to existing dependency for test.",
		Now:                time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if !hasImportChange(result.AppliedChanges, "update_dependency_source_refs", "dependency.repair-pump-before-validation") {
		t.Fatalf("expected applied update_dependency_source_refs: %#v", result.AppliedChanges)
	}
	dependenciesYAML, err := os.ReadFile(filepath.Join(projectDir, "canonical/dependencies.yaml"))
	if err != nil {
		t.Fatalf("read dependencies: %v", err)
	}
	dependenciesText := string(dependenciesYAML)
	if containsString(dependenciesText, "id: dependency.openproject.relation.88") {
		t.Fatalf("dependencies.yaml contains unexpected duplicate dependency:\n%s", dependenciesText)
	}
	if !containsString(dependenciesText, "id: dependency.repair-pump-before-validation") {
		t.Fatalf("dependencies.yaml missing existing dependency:\n%s", dependenciesText)
	}
	if !containsString(dependenciesText, "object_type: relation") {
		t.Fatalf("dependencies.yaml missing OpenProject relation source ref:\n%s", dependenciesText)
	}
	if !containsString(dependenciesText, "object_id: \"88\"") {
		t.Fatalf("dependencies.yaml missing OpenProject relation id:\n%s", dependenciesText)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected valid validation result, got %#v", result.Validation)
	}

	duplicate, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         snapshotID,
		Apply:              true,
		AcceptDependencies: true,
		ActorID:            "actor.human.operator",
	})
	if err != nil {
		t.Fatalf("duplicate Accept returned error: %v", err)
	}
	if duplicate.Applied {
		t.Fatalf("duplicate Applied = true, want false")
	}
	if !hasDiagnostic(duplicate.Warnings, "openproject_dependency_source_already_imported") {
		t.Fatalf("expected already imported dependency warning: %#v", duplicate.Warnings)
	}
}

func TestImportAcceptDependenciesSkipsUnsafeCandidates(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	snapshotID := writeOpenProjectMappedSnapshotForDependency(t, projectDir, "task_unmatched", false, "task.op.1234", "", "88")

	result, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         snapshotID,
		Apply:              true,
		AcceptDependencies: true,
		ActorID:            "actor.human.operator",
	})

	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if result.Applied {
		t.Fatalf("Applied = true, want false")
	}
	if !hasDiagnostic(result.Warnings, "openproject_dependency_not_safe_to_accept") {
		t.Fatalf("expected not safe warning: %#v", result.Warnings)
	}
}

func TestImportAcceptApplyRequiresActor(t *testing.T) {
	service := app.NewImportAcceptService(filestore.New("../../examples/small-team"))

	result, err := service.Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: "source_snapshot.openproject.20260710T123000",
		Apply:      true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Accept error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "actor_required") {
		t.Fatalf("expected actor_required error")
	}
}

func TestImportAcceptRejectsConflictingModes(t *testing.T) {
	service := app.NewImportAcceptService(filestore.New("../../examples/small-team"))

	result, err := service.Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: "source_snapshot.openproject.20260710T123000",
		DryRun:     true,
		Apply:      true,
		ActorID:    "actor.human.operator",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Accept error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "conflicting_import_accept_modes") {
		t.Fatalf("expected conflicting_import_accept_modes error")
	}
}

func TestImportAcceptRejectsConflictingAcceptKinds(t *testing.T) {
	service := app.NewImportAcceptService(filestore.New("../../examples/small-team"))

	result, err := service.Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:           "source_snapshot.openproject.20260710T123000",
		DryRun:               true,
		AcceptTimeEntries:    true,
		AcceptTrackingStatus: true,
		AcceptDependencies:   true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Accept error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "conflicting_import_accept_modes") {
		t.Fatalf("expected conflicting_import_accept_modes error")
	}
}

func TestImportAcceptAuditIDsDifferAcrossAcceptKindsInSameSecond(t *testing.T) {
	projectDir := copyProjectFixture(t)
	store := filestore.New(projectDir)
	timeSnapshotID := writeOpenProjectMappedSnapshotForTimeEntry(t, projectDir)
	demandSnapshotID := writeOpenProjectMappedSnapshotForResourceDemand(t, projectDir, "safe_to_accept", "task.op.1234", "resource.human.ivan")
	linkSnapshotID := writeOpenProjectMappedSnapshotForResourceLink(t, projectDir, "resource.human.ivan", "11")
	statusSnapshotID := writeOpenProjectMappedSnapshotForTrackingStatus(t, projectDir, "in_progress", "task.op.1234")
	dependencySnapshotID := writeOpenProjectMappedSnapshotForDependency(t, projectDir, "safe_to_accept", true, "task.op.1234", "task.offline.repair-pump", "88")
	when := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)

	taskResult, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: demandSnapshotID,
		DryRun:     true,
		Now:        when,
	})
	if err != nil {
		t.Fatalf("task Accept returned error: %v", err)
	}
	demandResult, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:            demandSnapshotID,
		DryRun:                true,
		AcceptResourceDemands: true,
		Now:                   when,
	})
	if err != nil {
		t.Fatalf("demand Accept returned error: %v", err)
	}
	timeResult, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:        timeSnapshotID,
		DryRun:            true,
		AcceptTimeEntries: true,
		Now:               when,
	})
	if err != nil {
		t.Fatalf("time Accept returned error: %v", err)
	}
	linkResult, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:          linkSnapshotID,
		DryRun:              true,
		AcceptResourceLinks: true,
		Now:                 when,
	})
	if err != nil {
		t.Fatalf("link Accept returned error: %v", err)
	}
	statusResult, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:           statusSnapshotID,
		DryRun:               true,
		AcceptTrackingStatus: true,
		Now:                  when,
	})
	if err != nil {
		t.Fatalf("status Accept returned error: %v", err)
	}
	dependencyResult, err := app.NewImportAcceptService(store).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:         dependencySnapshotID,
		DryRun:             true,
		AcceptDependencies: true,
		Now:                when,
	})
	if err != nil {
		t.Fatalf("dependency Accept returned error: %v", err)
	}
	if taskResult.AuditRecordPreview.ID == demandResult.AuditRecordPreview.ID {
		t.Fatalf("task and demand audit ids must differ")
	}
	if demandResult.AuditRecordPreview.ID == timeResult.AuditRecordPreview.ID {
		t.Fatalf("demand and time audit ids must differ")
	}
	if linkResult.AuditRecordPreview.ID == demandResult.AuditRecordPreview.ID || linkResult.AuditRecordPreview.ID == timeResult.AuditRecordPreview.ID || linkResult.AuditRecordPreview.ID == taskResult.AuditRecordPreview.ID {
		t.Fatalf("link audit id must differ from other accept kinds")
	}
	if statusResult.AuditRecordPreview.ID == demandResult.AuditRecordPreview.ID || statusResult.AuditRecordPreview.ID == timeResult.AuditRecordPreview.ID || statusResult.AuditRecordPreview.ID == linkResult.AuditRecordPreview.ID || statusResult.AuditRecordPreview.ID == taskResult.AuditRecordPreview.ID {
		t.Fatalf("status audit id must differ from other accept kinds")
	}
	if dependencyResult.AuditRecordPreview.ID == demandResult.AuditRecordPreview.ID || dependencyResult.AuditRecordPreview.ID == timeResult.AuditRecordPreview.ID || dependencyResult.AuditRecordPreview.ID == linkResult.AuditRecordPreview.ID || dependencyResult.AuditRecordPreview.ID == statusResult.AuditRecordPreview.ID || dependencyResult.AuditRecordPreview.ID == taskResult.AuditRecordPreview.ID {
		t.Fatalf("dependency audit id must differ from other accept kinds")
	}
}

func TestImportAcceptSnapshotNotFound(t *testing.T) {
	service := app.NewImportAcceptService(filestore.New("../../examples/small-team"))

	result, err := service.Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID: "source_snapshot.openproject.20990101T000000",
		DryRun:     true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Accept error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "source_snapshot_not_found") {
		t.Fatalf("expected source_snapshot_not_found error")
	}
}

func hasImportChange(changes []app.ImportAcceptChange, op string, targetID string) bool {
	for _, change := range changes {
		if change.Op == op && change.TargetID == targetID {
			return true
		}
	}
	return false
}

func hasPlannedWrite(writes []app.PlannedWrite, path string) bool {
	for _, write := range writes {
		if write.Path == path {
			return true
		}
	}
	return false
}

func writeOpenProjectMappedSnapshotForTimeEntry(t *testing.T, projectDir string) string {
	t.Helper()
	snapshotID := "source_snapshot.openproject.20260723T120000"
	key := "20260723T120000"
	mapped := app.OpenProjectMappedSnapshot{
		Snapshot: app.OpenProjectSnapshotSummary{
			ID:          snapshotID,
			Key:         key,
			LoadedCount: 1,
			SourceMode:  "api",
		},
		TaskCandidates: []app.OpenProjectTaskCandidate{{
			SourceID:      "1234",
			SourceURL:     "/api/v3/work_packages/1234",
			Title:         "Implement validation command",
			SourceStatus:  "In progress",
			MappedStatus:  "in_progress",
			AssigneeName:  "Ivan",
			AssigneeHref:  "/api/v3/users/11",
			MatchedTaskID: "task.op.1234",
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "work_package",
				ObjectID:   "1234",
				URL:        "/api/v3/work_packages/1234",
			}},
		}},
		TimeEntryCandidates: []app.OpenProjectTimeEntryCandidate{{
			SourceID:           "77",
			SourceURL:          "/api/v3/time_entries/77",
			WorkPackageID:      "1234",
			WorkPackageHref:    "/api/v3/work_packages/1234",
			UserName:           "Ivan",
			UserHref:           "/api/v3/users/11",
			ActivityName:       "Development",
			ActivityHref:       "/api/v3/time_entries/activities/3",
			Hours:              2.5,
			SpentOn:            "2026-07-23",
			Comment:            "Implemented API import",
			WouldCreateActual:  false,
			ActualAcceptStatus: "proposal_required",
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "time_entry",
				ObjectID:   "77",
				URL:        "/api/v3/time_entries/77",
			}},
		}},
	}
	data, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatalf("marshal mapped snapshot: %v", err)
	}
	dir := filepath.Join(projectDir, "imports/openproject", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mapped.json"), data, 0o644); err != nil {
		t.Fatalf("write mapped snapshot: %v", err)
	}
	return snapshotID
}

func writeOpenProjectMappedSnapshotForResourceDemand(t *testing.T, projectDir string, acceptStatus string, taskID string, resourceID string) string {
	t.Helper()
	snapshotID := "source_snapshot.openproject.20260724T120000"
	key := "20260724T120000"
	mapped := app.OpenProjectMappedSnapshot{
		Snapshot: app.OpenProjectSnapshotSummary{
			ID:          snapshotID,
			Key:         key,
			LoadedCount: 1,
			SourceMode:  "api",
		},
		TaskCandidates: []app.OpenProjectTaskCandidate{{
			SourceID:      "1234",
			SourceURL:     "/api/v3/work_packages/1234",
			Title:         "Implement validation command",
			MatchedTaskID: taskID,
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "work_package",
				ObjectID:   "1234",
				URL:        "/api/v3/work_packages/1234",
			}},
		}},
		ResourceDemandCandidates: []app.OpenProjectResourceDemandCandidate{{
			Source:        "openproject_sidecar",
			SourceID:      "resource_demand.op-1234.ivan.sidecar",
			WorkPackageID: "1234",
			MatchedTaskID: taskID,
			WouldCreate:   true,
			AcceptStatus:  acceptStatus,
			Canonical: domain.ResourceDemand{
				ID:              "resource_demand.op-1234.ivan.sidecar",
				TaskRef:         domain.Ref{Kind: "task", ID: taskID},
				ResourceRef:     domain.Ref{Kind: "resource", ID: resourceID},
				ResourceClass:   "human",
				Role:            "implementer",
				Unit:            "hours",
				PlannedAmount:   6,
				RemainingAmount: domain.Amount(6),
				Criticality:     "required",
				Status:          "active",
			},
		}},
	}
	data, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatalf("marshal mapped snapshot: %v", err)
	}
	dir := filepath.Join(projectDir, "imports/openproject", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mapped.json"), data, 0o644); err != nil {
		t.Fatalf("write mapped snapshot: %v", err)
	}
	return snapshotID
}

func writeOpenProjectMappedSnapshotForResourceLink(t *testing.T, projectDir string, matchedResourceID string, sourceID string) string {
	t.Helper()
	snapshotID := "source_snapshot.openproject.20260725T120000"
	key := "20260725T120000"
	mapped := app.OpenProjectMappedSnapshot{
		Snapshot: app.OpenProjectSnapshotSummary{
			ID:          snapshotID,
			Key:         key,
			LoadedCount: 1,
			SourceMode:  "api",
		},
		ResourceCandidates: []app.OpenProjectResourceCandidate{{
			SourceID:     sourceID,
			SourceHref:   "/api/v3/users/" + sourceID,
			DisplayName:  "Ivan",
			MatchedRefID: matchedResourceID,
			MatchReason:  "resource_link",
			WouldCreate:  false,
		}},
	}
	data, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatalf("marshal mapped snapshot: %v", err)
	}
	dir := filepath.Join(projectDir, "imports/openproject", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mapped.json"), data, 0o644); err != nil {
		t.Fatalf("write mapped snapshot: %v", err)
	}
	return snapshotID
}

func writeOpenProjectMappedSnapshotForTrackingStatus(t *testing.T, projectDir string, mappedStatus string, taskID string) string {
	t.Helper()
	snapshotID := "source_snapshot.openproject.20260726T120000"
	key := "20260726T120000"
	mapped := app.OpenProjectMappedSnapshot{
		Snapshot: app.OpenProjectSnapshotSummary{
			ID:          snapshotID,
			Key:         key,
			LoadedCount: 1,
			SourceMode:  "api",
		},
		TaskCandidates: []app.OpenProjectTaskCandidate{{
			SourceID:      "1234",
			SourceURL:     "/api/v3/work_packages/1234",
			Title:         "Implement validation command",
			SourceStatus:  "In progress",
			MappedStatus:  mappedStatus,
			MatchedTaskID: taskID,
			WouldCreate:   false,
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "work_package",
				ObjectID:   "1234",
				URL:        "/api/v3/work_packages/1234",
			}},
		}},
	}
	data, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatalf("marshal mapped snapshot: %v", err)
	}
	dir := filepath.Join(projectDir, "imports/openproject", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mapped.json"), data, 0o644); err != nil {
		t.Fatalf("write mapped snapshot: %v", err)
	}
	return snapshotID
}

func writeOpenProjectMappedSnapshotForDates(t *testing.T, projectDir, taskID, start, finish string) string {
	t.Helper()
	snapshotID := "source_snapshot.openproject.20260728T120000"
	key := "20260728T120000"
	mapped := app.OpenProjectMappedSnapshot{
		Snapshot: app.OpenProjectSnapshotSummary{ID: snapshotID, Key: key, LoadedCount: 1, SourceMode: "api"},
		TaskCandidates: []app.OpenProjectTaskCandidate{{
			SourceID: "1234", SourceURL: "/api/v3/work_packages/1234", Title: "Implement validation command",
			MatchedTaskID: taskID, StartDate: start, DueDate: finish,
			SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "1234", URL: "/api/v3/work_packages/1234"}},
		}},
	}
	data, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(projectDir, "imports/openproject", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mapped.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return snapshotID
}

func writeOpenProjectMappedSnapshotForDependency(t *testing.T, projectDir string, acceptStatus string, wouldCreate bool, predecessorTaskID string, successorTaskID string, sourceID string) string {
	t.Helper()
	snapshotID := "source_snapshot.openproject.20260727T120000"
	key := "20260727T120000"
	mapped := app.OpenProjectMappedSnapshot{
		Snapshot: app.OpenProjectSnapshotSummary{
			ID:          snapshotID,
			Key:         key,
			LoadedCount: 2,
			SourceMode:  "api",
		},
		TaskCandidates: []app.OpenProjectTaskCandidate{{
			SourceID:      "1234",
			SourceURL:     "/api/v3/work_packages/1234",
			Title:         "Implement validation command",
			MatchedTaskID: predecessorTaskID,
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "work_package",
				ObjectID:   "1234",
				URL:        "/api/v3/work_packages/1234",
			}},
		}, {
			SourceID:      "2000",
			SourceURL:     "/api/v3/work_packages/2000",
			Title:         "Repair pump",
			MatchedTaskID: successorTaskID,
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "work_package",
				ObjectID:   "2000",
				URL:        "/api/v3/work_packages/2000",
			}},
		}},
		DependencyCandidates: []app.OpenProjectDependencyCandidate{{
			SourceID:          sourceID,
			SourceURL:         "/api/v3/relations/" + sourceID,
			RelationType:      "follows",
			FromWorkPackageID: "2000",
			ToWorkPackageID:   "1234",
			PredecessorTaskID: predecessorTaskID,
			SuccessorTaskID:   successorTaskID,
			DependencyID:      "dependency.openproject.relation." + sourceID,
			DependencyType:    "finish_to_start",
			WouldCreate:       wouldCreate,
			AcceptStatus:      acceptStatus,
		}},
	}
	data, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatalf("marshal mapped snapshot: %v", err)
	}
	dir := filepath.Join(projectDir, "imports/openproject", key)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir snapshot: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mapped.json"), data, 0o644); err != nil {
		t.Fatalf("write mapped snapshot: %v", err)
	}
	return snapshotID
}

func rewriteMappedSnapshotResourceLinkMatchReason(t *testing.T, projectDir string, key string, matchReason string) {
	t.Helper()
	path := filepath.Join(projectDir, "imports/openproject", key, "mapped.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read mapped snapshot: %v", err)
	}
	var mapped app.OpenProjectMappedSnapshot
	if err := json.Unmarshal(data, &mapped); err != nil {
		t.Fatalf("parse mapped snapshot: %v", err)
	}
	for i := range mapped.ResourceCandidates {
		mapped.ResourceCandidates[i].MatchReason = matchReason
	}
	data, err = json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		t.Fatalf("marshal mapped snapshot: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mapped snapshot: %v", err)
	}
}

func setProjectOpenProjectIDs(t *testing.T, projectDir, policy string) {
	t.Helper()
	path := filepath.Join(projectDir, ".replan", "project.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "  openproject_ids:") {
		lines := strings.Split(text, "\n")
		for i, line := range lines {
			if strings.HasPrefix(line, "  openproject_ids:") {
				lines[i] = "  openproject_ids: " + policy
			}
		}
		text = strings.Join(lines, "\n")
	} else {
		text = strings.Replace(text, "  timezone:", "  openproject_ids: "+policy+"\n  timezone:", 1)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasPrivateIDDiagnosticForOp(diags []app.Diagnostic, op string) bool {
	for _, diag := range diags {
		if diag.Code == "openproject_ids_private" && strings.Contains(diag.Message, "refusing "+op+" ") {
			return true
		}
	}
	return false
}
