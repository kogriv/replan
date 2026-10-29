package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestOpenProjectImportDryRunFromFile(t *testing.T) {
	service := app.NewOpenProjectImportService(filestore.New("../../examples/small-team"))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
		Profile:  "openproject.demo",
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if result.Snapshot.LoadedCount != 2 {
		t.Fatalf("LoadedCount = %d, want 2", result.Snapshot.LoadedCount)
	}
	if result.Snapshot.SourceMode != "file" {
		t.Fatalf("SourceMode = %q, want file", result.Snapshot.SourceMode)
	}
	if len(result.PlannedWrites) != 3 {
		t.Fatalf("PlannedWrites len = %d, want 3", len(result.PlannedWrites))
	}
	if result.Profile == nil {
		t.Fatalf("Profile is nil")
	}
	if result.Profile.ID != "openproject.demo" {
		t.Fatalf("Profile.ID = %q, want openproject.demo", result.Profile.ID)
	}
	if result.Profile.TokenEnv != "OPENPROJECT_TOKEN" {
		t.Fatalf("Profile.TokenEnv = %q, want OPENPROJECT_TOKEN", result.Profile.TokenEnv)
	}
	if len(result.TaskCandidates) != 2 {
		t.Fatalf("TaskCandidates len = %d, want 2", len(result.TaskCandidates))
	}
	first := result.TaskCandidates[0]
	if first.SourceID != "1234" {
		t.Fatalf("first SourceID = %q, want 1234", first.SourceID)
	}
	if first.MatchedTaskID != "task.op.1234" {
		t.Fatalf("first MatchedTaskID = %q, want task.op.1234", first.MatchedTaskID)
	}
	if first.MatchReason != "title" {
		t.Fatalf("first MatchReason = %q, want title", first.MatchReason)
	}
	if first.MappedStatus != "in_progress" {
		t.Fatalf("first MappedStatus = %q, want in_progress", first.MappedStatus)
	}
	if first.EstimatedHours != 8 {
		t.Fatalf("first EstimatedHours = %v, want 8", first.EstimatedHours)
	}
	if first.SpentHours != 3 {
		t.Fatalf("first SpentHours = %v, want 3", first.SpentHours)
	}
	second := result.TaskCandidates[1]
	if !second.WouldCreate {
		t.Fatalf("second WouldCreate = false, want true")
	}
	if second.EstimatedHours != 2.5 {
		t.Fatalf("second EstimatedHours = %v, want 2.5", second.EstimatedHours)
	}
	if !hasDiagnostic(result.Warnings, "unmatched_work_package") {
		t.Fatalf("expected unmatched_work_package warning")
	}
}

func TestOpenProjectImportUnknownProfile(t *testing.T) {
	service := app.NewOpenProjectImportService(filestore.New("../../examples/small-team"))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
		Profile:  "missing.profile",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "source_profile_not_found") {
		t.Fatalf("expected source_profile_not_found error")
	}
}

func TestOpenProjectImportValidationPrecedenceReturnsAllPreloadErrors(t *testing.T) {
	service := app.NewOpenProjectImportService(staticWorkspaceLoader{
		projectPath: "memory",
		workspace: domain.Workspace{Project: domain.Project{
			ID:             "project.test",
			OpenProjectIDs: domain.OpenProjectIDsPrivateBindings,
		}},
	})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		Profile: "missing.profile",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	want := []string{"openproject_ids_private", "source_profile_not_found"}
	if got := diagnosticCodeList(result.Errors); !reflect.DeepEqual(got, want) {
		t.Fatalf("error codes = %#v, want %#v", got, want)
	}
	if result.Snapshot.ID != "" || len(result.TaskCandidates) != 0 {
		t.Fatalf("partial result advanced past preload validation: %#v", result)
	}
}

func TestOpenProjectImportMapsStandardOpenProjectStatuses(t *testing.T) {
	projectPath := copyProjectFixture(t)
	snapshotPath := filepath.Join(t.TempDir(), "work-packages.json")
	raw := `{
  "_type": "Collection",
  "total": 6,
  "count": 6,
  "pageSize": 100,
  "_embedded": {
    "elements": [
      {
        "id": 2000,
        "subject": "Created but not yet ready status",
        "status": {"name": "New"},
        "assignee": {"name": "Ivan"},
        "estimatedTime": "PT1H",
        "_links": {
          "self": {"href": "/api/v3/work_packages/2000"},
          "status": {"title": "New"},
          "assignee": {"title": "Ivan", "href": "/api/v3/users/6"}
        }
      },
      {
        "id": 2001,
        "subject": "Ready planning status",
        "status": {"name": "To be scheduled"},
        "assignee": {"name": "Ivan"},
        "estimatedTime": "PT1H",
        "_links": {
          "self": {"href": "/api/v3/work_packages/2001"},
          "status": {"title": "To be scheduled"},
          "assignee": {"title": "Ivan", "href": "/api/v3/users/6"}
        }
      },
      {
        "id": 2002,
        "subject": "Review status",
        "status": {"name": "Developed"},
        "assignee": {"name": "Ivan"},
        "estimatedTime": "PT1H",
        "_links": {
          "self": {"href": "/api/v3/work_packages/2002"},
          "status": {"title": "Developed"},
          "assignee": {"title": "Ivan", "href": "/api/v3/users/6"}
        }
      },
      {
        "id": 2003,
        "subject": "Blocked status",
        "status": {"name": "On hold"},
        "assignee": {"name": "Codex"},
        "estimatedTime": "PT1H",
        "_links": {
          "self": {"href": "/api/v3/work_packages/2003"},
          "status": {"title": "On hold"},
          "assignee": {"title": "Codex", "href": "/api/v3/users/7"}
        }
      },
      {
        "id": 2004,
        "subject": "Accepted status",
        "status": {"name": "Closed"},
        "assignee": {"name": "Ivan"},
        "estimatedTime": "PT1H",
        "_links": {
          "self": {"href": "/api/v3/work_packages/2004"},
          "status": {"title": "Closed"},
          "assignee": {"title": "Ivan", "href": "/api/v3/users/6"}
        }
      },
      {
        "id": 2005,
        "subject": "In progress status",
        "status": {"name": "In progress"},
        "assignee": {"name": "Codex"},
        "estimatedTime": "PT1H",
        "_links": {
          "self": {"href": "/api/v3/work_packages/2005"},
          "status": {"title": "In progress"},
          "assignee": {"title": "Codex", "href": "/api/v3/users/7"}
        }
      }
    ]
  }
}`
	if err := os.WriteFile(snapshotPath, []byte(raw), 0o600); err != nil {
		t.Fatalf("write snapshot: %v", err)
	}
	service := app.NewOpenProjectImportService(filestore.New(projectPath))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: snapshotPath,
		DryRun:   true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	want := map[string]string{
		"2000": "not_started",
		"2001": "ready",
		"2002": "review_ready",
		"2003": "blocked",
		"2004": "accepted",
		"2005": "in_progress",
	}
	for _, candidate := range result.TaskCandidates {
		if candidate.MappedStatus != want[candidate.SourceID] {
			t.Fatalf("candidate %s MappedStatus = %q, want %q", candidate.SourceID, candidate.MappedStatus, want[candidate.SourceID])
		}
	}
	if hasDiagnostic(result.Warnings, "unmapped_openproject_status") {
		t.Fatalf("did not expect unmapped_openproject_status warning: %#v", result.Warnings)
	}
}

func TestOpenProjectImportWritesSnapshotFromFile(t *testing.T) {
	projectDir := copyProjectFixture(t)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		Now:      time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if result.Snapshot.Key != "20260710T123000" {
		t.Fatalf("Snapshot.Key = %q, want 20260710T123000", result.Snapshot.Key)
	}
	if len(result.WrittenFiles) != 3 {
		t.Fatalf("WrittenFiles len = %d, want 3: %#v", len(result.WrittenFiles), result.WrittenFiles)
	}
	for _, rel := range []string{
		"imports/openproject/20260710T123000/manifest.yaml",
		"imports/openproject/20260710T123000/raw.json",
		"imports/openproject/20260710T123000/mapped.json",
	} {
		if _, err := os.Stat(filepath.Join(projectDir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(projectDir, "imports/openproject/20260710T123000/manifest.yaml"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	if !containsString(string(manifest), "source_snapshot.openproject.20260710T123000") {
		t.Fatalf("manifest does not contain snapshot id:\n%s", manifest)
	}
	mapped, err := os.ReadFile(filepath.Join(projectDir, "imports/openproject/20260710T123000/mapped.json"))
	if err != nil {
		t.Fatalf("read mapped: %v", err)
	}
	if !containsString(string(mapped), "task_candidates") {
		t.Fatalf("mapped does not contain task candidates:\n%s", mapped)
	}
}

func TestOpenProjectImportDefaultMissingResourceDemandSidecarIsIgnored(t *testing.T) {
	projectDir := copyProjectFixture(t)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.ResourceDemandCandidates) != 0 {
		t.Fatalf("ResourceDemandCandidates len = %d, want 0: %#v", len(result.ResourceDemandCandidates), result.ResourceDemandCandidates)
	}
	if hasDiagnostic(result.Warnings, "openproject_resource_demand_sidecar_not_found") {
		t.Fatalf("default missing sidecar must not warn: %#v", result.Warnings)
	}
}

func TestOpenProjectImportExplicitMissingResourceDemandSidecarFails(t *testing.T) {
	projectDir := copyProjectFixture(t)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile:               "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:                 true,
		ResourceDemandsSidecar: "integrations/openproject/missing.yaml",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "openproject_resource_demand_sidecar_not_found") {
		t.Fatalf("expected openproject_resource_demand_sidecar_not_found error: %#v", result.Errors)
	}
}

func TestOpenProjectImportExplicitMissingResourceLinksSidecarFails(t *testing.T) {
	projectDir := copyProjectFixture(t)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile:             "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:               true,
		ResourceLinksSidecar: "integrations/openproject/missing-links.yaml",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "openproject_resource_link_sidecar_not_found") {
		t.Fatalf("expected openproject_resource_link_sidecar_not_found error: %#v", result.Errors)
	}
}

func TestOpenProjectImportResourceLinksSidecarMatchesResourceCandidates(t *testing.T) {
	projectDir := copyProjectFixture(t)
	writeOpenProjectResourceLinksSidecar(t, projectDir, `openproject_resource_links:
  - user_id: "11"
    user_href: /api/v3/users/11
    display_name: "Ivan"
    resource_ref:
      kind: resource
      id: resource.human.operator
`)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.ResourceCandidates) != 1 {
		t.Fatalf("ResourceCandidates len = %d, want 1: %#v", len(result.ResourceCandidates), result.ResourceCandidates)
	}
	candidate := result.ResourceCandidates[0]
	if candidate.SourceID != "11" {
		t.Fatalf("SourceID = %q, want 11", candidate.SourceID)
	}
	if candidate.MatchedRefID != "resource.human.operator" {
		t.Fatalf("MatchedRefID = %q, want resource.human.operator", candidate.MatchedRefID)
	}
	if candidate.MatchReason != "resource_link" {
		t.Fatalf("MatchReason = %q, want resource_link", candidate.MatchReason)
	}
	if candidate.WouldCreate {
		t.Fatalf("WouldCreate = true, want false")
	}
}

func TestOpenProjectImportInvalidResourceLinksSidecarDiagnostics(t *testing.T) {
	projectDir := copyProjectFixture(t)
	writeOpenProjectResourceLinksSidecar(t, projectDir, `openproject_resource_links:
  - user_id: "11"
    resource_ref:
      kind: resource
      id: resource.human.missing
`)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "openproject_resource_link_resource_unknown") {
		t.Fatalf("expected openproject_resource_link_resource_unknown error: %#v", result.Errors)
	}
}

func TestOpenProjectImportMapsResourceDemandSidecarCandidates(t *testing.T) {
	projectDir := copyProjectFixture(t)
	writeOpenProjectResourceDemandSidecar(t, projectDir, `openproject_resource_demands:
  - work_package_id: "1234"
    demands:
      - id: resource_demand.op-1234.ivan.implementation
        resource_ref:
          kind: resource
          id: resource.human.ivan
        resource_class: human
        planned_amount: 6
        remaining_amount: 6
        unit: hours
        role: implementer
        criticality: required
        status: active
        confidence: operator_estimate
`)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.ResourceDemandCandidates) != 1 {
		t.Fatalf("ResourceDemandCandidates len = %d, want 1: %#v", len(result.ResourceDemandCandidates), result.ResourceDemandCandidates)
	}
	candidate := result.ResourceDemandCandidates[0]
	if candidate.Source != "openproject_sidecar" {
		t.Fatalf("Source = %q", candidate.Source)
	}
	if candidate.WorkPackageID != "1234" {
		t.Fatalf("WorkPackageID = %q, want 1234", candidate.WorkPackageID)
	}
	if candidate.MatchedTaskID != "task.op.1234" {
		t.Fatalf("MatchedTaskID = %q, want task.op.1234", candidate.MatchedTaskID)
	}
	if !candidate.WouldCreate {
		t.Fatalf("WouldCreate = false, want true")
	}
	if candidate.AcceptStatus != "safe_to_accept" {
		t.Fatalf("AcceptStatus = %q, want safe_to_accept: %#v", candidate.AcceptStatus, candidate.Diagnostics)
	}
	if candidate.Canonical.TaskRef.ID != "task.op.1234" || candidate.Canonical.ResourceRef.ID != "resource.human.ivan" {
		t.Fatalf("unexpected canonical demand: %#v", candidate.Canonical)
	}
}

func TestOpenProjectImportKeepsExplicitZeroSidecarRemaining(t *testing.T) {
	projectDir := copyProjectFixture(t)
	writeOpenProjectResourceDemandSidecar(t, projectDir, `openproject_resource_demands:
  - work_package_id: "1234"
    demands:
      - id: resource_demand.op-1234.ivan.done
        resource_ref:
          kind: resource
          id: resource.human.ivan
        resource_class: human
        planned_amount: 6
        remaining_amount: 0
        unit: hours
        role: implementer
        criticality: required
        status: active
      - id: resource_demand.op-1234.ivan.unestimated
        resource_ref:
          kind: resource
          id: resource.human.ivan
        resource_class: human
        planned_amount: 4
        unit: hours
        role: reviewer
        criticality: required
        status: active
`)
	result, err := app.NewOpenProjectImportService(filestore.New(projectDir)).Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	remaining := map[string]*float64{}
	for _, candidate := range result.ResourceDemandCandidates {
		remaining[candidate.Canonical.ID] = candidate.Canonical.RemainingAmount
	}
	if v := remaining["resource_demand.op-1234.ivan.done"]; v == nil || *v != 0 {
		t.Fatalf("explicit zero remaining was not kept: %v", v)
	}
	if v := remaining["resource_demand.op-1234.ivan.unestimated"]; v == nil || *v != 4 {
		t.Fatalf("missing remaining did not default to planned: %v", v)
	}
}

func TestOpenProjectImportWritesResourceDemandCandidatesToMappedSnapshot(t *testing.T) {
	projectDir := copyProjectFixture(t)
	writeOpenProjectResourceDemandSidecar(t, projectDir, `openproject_resource_demands:
  - work_package_id: "1234"
    demands:
      - id: resource_demand.op-1234.ivan.implementation
        resource_ref:
          kind: resource
          id: resource.human.ivan
        resource_class: human
        planned_amount: 6
        unit: hours
        role: implementer
        criticality: required
        status: active
        confidence: operator_estimate
`)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		Now:      time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	mapped, err := os.ReadFile(filepath.Join(projectDir, "imports/openproject/20260710T123000/mapped.json"))
	if err != nil {
		t.Fatalf("read mapped: %v", err)
	}
	if !containsString(string(mapped), `"resource_demand_candidates"`) {
		t.Fatalf("mapped does not contain resource_demand_candidates:\n%s", mapped)
	}
	if !containsString(string(mapped), `"resource_demand.op-1234.ivan.implementation"`) {
		t.Fatalf("mapped does not contain sidecar demand id:\n%s", mapped)
	}
}

func TestOpenProjectImportInvalidResourceDemandSidecarDiagnostics(t *testing.T) {
	projectDir := copyProjectFixture(t)
	writeOpenProjectResourceDemandSidecar(t, projectDir, `openproject_resource_demands:
  - work_package_id: "1234"
    demands:
      - id: resource_demand.op-1234.unknown.implementation
        resource_ref:
          kind: resource
          id: resource.human.missing
        resource_class: human
        planned_amount: -1
        unit: parsecs
        role: implementer
        criticality: required
        status: active
        confidence: operator_estimate
`)
	service := app.NewOpenProjectImportService(filestore.New(projectDir))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:   true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "openproject_resource_demand_resource_unknown") {
		t.Fatalf("expected unknown resource error: %#v", result.Errors)
	}
	if !hasDiagnostic(result.Errors, "openproject_resource_demand_amount_invalid") {
		t.Fatalf("expected invalid amount error: %#v", result.Errors)
	}
	if !hasDiagnostic(result.Errors, "openproject_resource_demand_unit_unknown") {
		t.Fatalf("expected unknown unit error: %#v", result.Errors)
	}
	if len(result.ResourceDemandCandidates) != 1 || result.ResourceDemandCandidates[0].AcceptStatus != "invalid_sidecar" {
		t.Fatalf("expected invalid sidecar candidate: %#v", result.ResourceDemandCandidates)
	}
}

func TestOpenProjectImportMissingFile(t *testing.T) {
	service := app.NewOpenProjectImportService(filestore.New("../../examples/small-team"))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/missing.json",
		DryRun:   true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "openproject_file_read_error") {
		t.Fatalf("expected openproject_file_read_error error")
	}
}

func TestOpenProjectImportDryRunFromAPI(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	var requestedOffsets []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "apikey" || password != "test-token" {
			t.Fatalf("unexpected basic auth username=%q password=%q ok=%v", username, password, ok)
		}
		if r.URL.Path != "/api/v3/projects/demo/work_packages" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if r.URL.Query().Get("pageSize") != "1" {
			t.Fatalf("pageSize = %q, want 1", r.URL.Query().Get("pageSize"))
		}
		if r.URL.Query().Get("filters") == "" {
			t.Fatalf("expected filters query for --since")
		}
		var filters []map[string]struct {
			Operator string   `json:"operator"`
			Values   []string `json:"values"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil {
			t.Fatalf("failed to parse filters query: %v", err)
		}
		updated := filters[0]["updated_at"]
		if updated.Operator != "<>d" {
			t.Fatalf("updated_at operator = %q, want <>d", updated.Operator)
		}
		if got := fmt.Sprint(updated.Values); got != "[2026-07-01 2099-12-31]" {
			t.Fatalf("updated_at values = %s, want [2026-07-01 2099-12-31]", got)
		}
		offset := r.URL.Query().Get("offset")
		requestedOffsets = append(requestedOffsets, offset)
		w.Header().Set("Content-Type", "application/json")
		switch offset {
		case "1":
			fmt.Fprint(w, openProjectCollectionJSON(2, 1, 1, 1234, "Implement validation command", "In progress", "Ivan", "/api/v3/users/11"))
		case "2":
			fmt.Fprint(w, openProjectCollectionJSON(2, 1, 1, 1235, "Unknown task from API", "New", "", ""))
		default:
			t.Fatalf("unexpected offset %q", offset)
		}
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:   "project.test",
			Name: "Test",
			SourceProfiles: []domain.SourceProfile{{
				ID:        "openproject.test",
				Source:    "openproject",
				BaseURL:   server.URL,
				ProjectID: "demo",
				TokenEnv:  "REPLAN_TEST_OPENPROJECT_TOKEN",
			}},
		},
		Tasks: []domain.Task{{
			ID:             "task.op.1234",
			Title:          "Implement validation command",
			TrackingStatus: "ready",
		}},
		Resources: []domain.Resource{{
			ID:    "resource.human.ivan",
			Class: "human",
			Name:  "Ivan",
		}},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:   true,
		Profile:  "openproject.test",
		Since:    "2026-07-01",
		PageSize: 1,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if result.Snapshot.SourceMode != "api" {
		t.Fatalf("SourceMode = %q, want api", result.Snapshot.SourceMode)
	}
	if result.Snapshot.PagesLoaded != 2 {
		t.Fatalf("PagesLoaded = %d, want 2", result.Snapshot.PagesLoaded)
	}
	if got := fmt.Sprint(requestedOffsets); got != "[1 2]" {
		t.Fatalf("requested offsets = %s, want [1 2]", got)
	}
	if len(result.TaskCandidates) != 2 {
		t.Fatalf("TaskCandidates len = %d, want 2", len(result.TaskCandidates))
	}
	if result.TaskCandidates[0].MatchedTaskID != "task.op.1234" {
		t.Fatalf("first MatchedTaskID = %q, want task.op.1234", result.TaskCandidates[0].MatchedTaskID)
	}
	if len(result.ResourceCandidates) != 1 || result.ResourceCandidates[0].MatchedRefID != "resource.human.ivan" {
		t.Fatalf("unexpected resource candidates: %#v", result.ResourceCandidates)
	}
}

func TestOpenProjectImportPrivateBindingsRefusesLiveWriteButAllowsDryRun(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"_type":"Collection","total":0,"count":0,"pageSize":100,"offset":1,"_embedded":{"elements":[]}}`)
	}))
	defer server.Close()
	loader := stubLoader{ws: domain.Workspace{Project: domain.Project{
		ID: "project.test", Name: "Test", OpenProjectIDs: domain.OpenProjectIDsPrivateBindings,
	}}}
	request := app.OpenProjectImportRequest{
		BaseURL: server.URL, ProjectID: "demo", TokenEnv: "REPLAN_TEST_OPENPROJECT_TOKEN",
		RequestTimeoutSeconds: 2,
	}
	result, err := app.NewOpenProjectImportService(loader).Import(context.Background(), request)
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "openproject_ids_private") {
		t.Fatalf("write import error = %v, errors = %#v", err, result.Errors)
	}
	if requests != 0 {
		t.Fatalf("refused live write made %d API requests", requests)
	}
	request.DryRun = true
	result, err = app.NewOpenProjectImportService(loader).Import(context.Background(), request)
	if err != nil {
		t.Fatalf("dry-run import returned error: %v %#v", err, result.Errors)
	}
	if requests != 1 || result.Snapshot.SourceMode != "api" {
		t.Fatalf("dry-run requests = %d, result = %#v", requests, result)
	}
}

func TestOpenProjectImportPrivateBindingsAllowsFileSnapshotWrite(t *testing.T) {
	projectDir := copyProjectFixture(t)
	setProjectOpenProjectIDs(t, projectDir, domain.OpenProjectIDsPrivateBindings)
	result, err := app.NewOpenProjectImportService(filestore.New(projectDir)).Import(context.Background(), app.OpenProjectImportRequest{
		FromFile: "../../examples/imports/openproject/work-packages.sample.json",
		Now:      time.Date(2026, 7, 10, 12, 30, 0, 0, time.UTC),
	})
	if err != nil || !result.Applied {
		t.Fatalf("file import = %#v, error = %v", result, err)
	}
	for _, path := range result.WrittenFiles {
		if _, err := os.Stat(filepath.Join(projectDir, path)); err != nil {
			t.Fatalf("written file %s: %v", path, err)
		}
	}
}

func TestOpenProjectImportIncludeClosedUsesGlobalProjectFilter(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v3/projects/demo" {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"_type":"Project","id":3,"identifier":"demo","name":"Demo"}`)
			return
		}
		if r.URL.Path != "/api/v3/work_packages" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		var filters []map[string]struct {
			Operator string   `json:"operator"`
			Values   []string `json:"values"`
		}
		if err := json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters); err != nil {
			t.Fatalf("failed to parse filters query: %v", err)
		}
		if len(filters) == 0 {
			t.Fatalf("expected project filter")
		}
		project := filters[0]["project"]
		if project.Operator != "=" {
			t.Fatalf("project operator = %q, want =", project.Operator)
		}
		if got := fmt.Sprint(project.Values); got != "[3]" {
			t.Fatalf("project values = %s, want [3]", got)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, openProjectCollectionJSON(1, 1, 100, 1234, "Closed task", "Closed", "Ivan", "/api/v3/users/11"))
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:   "project.test",
			Name: "Test",
			SourceProfiles: []domain.SourceProfile{{
				ID:        "openproject.test",
				Source:    "openproject",
				BaseURL:   server.URL,
				ProjectID: "demo",
				TokenEnv:  "REPLAN_TEST_OPENPROJECT_TOKEN",
			}},
		},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:        true,
		Profile:       "openproject.test",
		IncludeClosed: true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.TaskCandidates) != 1 {
		t.Fatalf("TaskCandidates len = %d, want 1", len(result.TaskCandidates))
	}
	if result.TaskCandidates[0].SourceStatus != "Closed" {
		t.Fatalf("SourceStatus = %q, want Closed", result.TaskCandidates[0].SourceStatus)
	}
}

func TestOpenProjectImportRetriesTransientGETThenSucceeds(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if requests == 1 {
			http.Error(w, "warming up", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"_type":"Collection","total":0,"count":0,"pageSize":100,"offset":1,"_embedded":{"elements":[]}}`)
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun: true, BaseURL: server.URL, ProjectID: "demo",
		TokenEnv: "REPLAN_TEST_OPENPROJECT_TOKEN", RequestTimeoutSeconds: 2,
		RequestRetries: 1, RequestRetryDelaySeconds: 0.001,
	})
	if err != nil {
		t.Fatalf("Import returned error: %v (%#v)", err, result.Errors)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want 2", requests)
	}
	if result.OpenProjectRequest.Retries != 1 || result.OpenProjectRequest.RetryDelaySeconds != 0.001 {
		t.Fatalf("request metadata = %#v", result.OpenProjectRequest)
	}
}

func TestOpenProjectImportRetryTimeoutAppliesToEachAttempt(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			time.Sleep(300 * time.Millisecond)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"_type":"Collection","total":0,"count":0,"pageSize":100,"offset":1,"_embedded":{"elements":[]}}`)
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun: true, BaseURL: server.URL, ProjectID: "demo",
		TokenEnv: "REPLAN_TEST_OPENPROJECT_TOKEN", RequestTimeoutSeconds: 0.2,
		RequestRetries: 1, RequestRetryDelaySeconds: 0.001,
	})
	if err != nil {
		t.Fatalf("Import returned error: %v (%#v)", err, result.Errors)
	}
	if requests.Load() != 2 {
		t.Fatalf("requests = %d, want 2", requests.Load())
	}
}

func TestOpenProjectImportRetryHonorsDelay(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	const retryDelay = 80 * time.Millisecond
	requestTimes := make(chan time.Time, 2)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestTimes <- time.Now()
		if requests.Add(1) == 1 {
			http.Error(w, "warming up", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"_type":"Collection","total":0,"count":0,"pageSize":100,"offset":1,"_embedded":{"elements":[]}}`)
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun: true, BaseURL: server.URL, ProjectID: "demo",
		TokenEnv: "REPLAN_TEST_OPENPROJECT_TOKEN", RequestTimeoutSeconds: 1,
		RequestRetries: 1, RequestRetryDelaySeconds: retryDelay.Seconds(),
	})
	if err != nil {
		t.Fatalf("Import returned error: %v (%#v)", err, result.Errors)
	}
	first, second := <-requestTimes, <-requestTimes
	if elapsed := second.Sub(first); elapsed < retryDelay {
		t.Fatalf("time between attempts = %s, want at least %s", elapsed, retryDelay)
	}
}

func TestOpenProjectImportDoesNotRetryByDefault(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		http.Error(w, "warming up", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}}})

	_, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun: true, BaseURL: server.URL, ProjectID: "demo",
		TokenEnv: "REPLAN_TEST_OPENPROJECT_TOKEN", RequestTimeoutSeconds: 2,
	})
	if err == nil {
		t.Fatal("Import returned no error")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}

func TestOpenProjectImportAppliesTunnelHeaders(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	pathsSeen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Host != "op.example.internal" {
			t.Fatalf("Host = %q, want op.example.internal", r.Host)
		}
		if got := r.Header.Get("X-Forwarded-Proto"); got != "https" {
			t.Fatalf("X-Forwarded-Proto = %q, want https", got)
		}
		pathsSeen[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/projects/demo":
			fmt.Fprint(w, `{"_type":"Project","id":3,"identifier":"demo","name":"Demo"}`)
		case "/api/v3/work_packages":
			fmt.Fprint(w, openProjectCollectionJSON(1, 1, 100, 1234, "Header tunnel task", "New", "", ""))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:   "project.test",
			Name: "Test",
		},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:         true,
		BaseURL:        server.URL,
		ProjectID:      "demo",
		TokenEnv:       "REPLAN_TEST_OPENPROJECT_TOKEN",
		HostHeader:     "op.example.internal",
		ForwardedProto: "https",
		IncludeClosed:  true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if result.Snapshot.SourceMode != "api" {
		t.Fatalf("SourceMode = %q, want api", result.Snapshot.SourceMode)
	}
	for _, path := range []string{"/api/v3/projects/demo", "/api/v3/work_packages"} {
		if !pathsSeen[path] {
			t.Fatalf("path %q was not requested", path)
		}
	}
}

func TestOpenProjectImportIncludesTimeEntryCandidates(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "apikey" || password != "test-token" {
			t.Fatalf("unexpected basic auth username=%q password=%q ok=%v", username, password, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/projects/demo/work_packages":
			fmt.Fprint(w, openProjectCollectionJSON(1, 1, 100, 1234, "Implement validation command", "In progress", "Ivan", "/api/v3/users/11"))
		case "/api/v3/time_entries":
			fmt.Fprint(w, `{
  "_type": "Collection",
  "total": 3,
  "count": 3,
  "pageSize": 100,
  "_embedded": {
    "elements": [
      {
        "id": 77,
        "hours": "PT2H30M",
        "spentOn": "2026-07-23",
        "comment": {"raw": "Implemented API import"},
        "_links": {
          "self": {"href": "/api/v3/time_entries/77"},
          "entity": {"href": "/api/v3/work_packages/1234", "title": "Implement validation command"},
          "project": {"href": "/api/v3/projects/3", "title": "Demo"},
          "user": {"href": "/api/v3/users/11", "title": "Ivan"},
          "activity": {"href": "/api/v3/time_entries/activities/3", "title": "Development"}
        }
      },
      {
        "id": 78,
        "hours": "PT1H",
        "spentOn": "2026-07-24",
        "_links": {
          "self": {"href": "/api/v3/time_entries/78"},
          "entity": {"href": "/api/v3/work_packages/9999", "title": "Other"},
          "project": {"href": "/api/v3/projects/3", "title": "Demo"},
          "user": {"href": "/api/v3/users/12", "title": "Other"},
          "activity": {"href": "/api/v3/time_entries/activities/3", "title": "Development"}
        }
      }
    ]
  }
}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:   "project.test",
			Name: "Test",
			SourceProfiles: []domain.SourceProfile{{
				ID:        "openproject.test",
				Source:    "openproject",
				BaseURL:   server.URL,
				ProjectID: "demo",
				TokenEnv:  "REPLAN_TEST_OPENPROJECT_TOKEN",
			}},
		},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:             true,
		Profile:            "openproject.test",
		IncludeTimeEntries: true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.TimeEntryCandidates) != 1 {
		t.Fatalf("TimeEntryCandidates len = %d, want 1: %#v", len(result.TimeEntryCandidates), result.TimeEntryCandidates)
	}
	entry := result.TimeEntryCandidates[0]
	if entry.SourceID != "77" {
		t.Fatalf("SourceID = %q, want 77", entry.SourceID)
	}
	if entry.WorkPackageID != "1234" {
		t.Fatalf("WorkPackageID = %q, want 1234", entry.WorkPackageID)
	}
	if entry.Hours != 2.5 {
		t.Fatalf("Hours = %v, want 2.5", entry.Hours)
	}
	if entry.ActualAcceptStatus != "proposal_required" || entry.WouldCreateActual {
		t.Fatalf("unexpected actual acceptance flags: %#v", entry)
	}
}

func TestOpenProjectImportIncludesTimeEntryCandidatesFromFile(t *testing.T) {
	service := app.NewOpenProjectImportService(filestore.New("../../examples/replan-self"))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile:           "../../examples/imports/openproject/replan-self.work-packages.sample.json",
		DryRun:             true,
		IncludeTimeEntries: true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.TimeEntryCandidates) != 1 {
		t.Fatalf("TimeEntryCandidates len = %d, want 1: %#v", len(result.TimeEntryCandidates), result.TimeEntryCandidates)
	}
	entry := result.TimeEntryCandidates[0]
	if entry.SourceID != "9201" {
		t.Fatalf("SourceID = %q, want 9201", entry.SourceID)
	}
	if entry.WorkPackageID != "2105" {
		t.Fatalf("WorkPackageID = %q, want 2105", entry.WorkPackageID)
	}
	if entry.UserName != "RePlan maintainer" {
		t.Fatalf("UserName = %q, want RePlan maintainer", entry.UserName)
	}
	if entry.Hours != 1.5 {
		t.Fatalf("Hours = %v, want 1.5", entry.Hours)
	}
}

func TestOpenProjectImportIncludesDependencyCandidates(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		if !ok || username != "apikey" || password != "test-token" {
			t.Fatalf("unexpected basic auth username=%q password=%q ok=%v", username, password, ok)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/projects/demo/work_packages":
			fmt.Fprint(w, `{
  "_type": "Collection",
  "total": 2,
  "count": 2,
  "pageSize": 100,
  "_embedded": {
    "elements": [
      {
        "id": 1234,
        "subject": "Implement validation command",
        "status": {"name": "In progress"},
        "_links": {
          "self": {"href": "/api/v3/work_packages/1234"},
          "status": {"title": "In progress"}
        }
      },
      {
        "id": 1235,
        "subject": "Review validation command",
        "status": {"name": "New"},
        "_links": {
          "self": {"href": "/api/v3/work_packages/1235"},
          "status": {"title": "New"}
        }
      }
    ]
  }
}`)
		case "/api/v3/relations":
			fmt.Fprint(w, `{
  "_type": "Collection",
  "total": 2,
  "count": 2,
  "pageSize": 100,
  "_embedded": {
    "elements": [
      {
        "id": 7,
        "type": "follows",
        "reverseType": "precedes",
        "_links": {
          "self": {"href": "/api/v3/relations/7"},
          "from": {"href": "/api/v3/work_packages/1235", "title": "Review validation command"},
          "to": {"href": "/api/v3/work_packages/1234", "title": "Implement validation command"}
        }
      },
      {
        "id": 8,
        "type": "duplicates",
        "_links": {
          "self": {"href": "/api/v3/relations/8"},
          "from": {"href": "/api/v3/work_packages/1235"},
          "to": {"href": "/api/v3/work_packages/9999"}
        }
      },
      {
        "id": 9,
        "type": "precedes",
        "_links": {
          "self": {"href": "/api/v3/relations/9"},
          "from": {"href": "/api/v3/work_packages/1235"},
          "to": {"href": "/api/v3/work_packages/9999"}
        }
      }
    ]
  }
}`)
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:   "project.test",
			Name: "Test",
			SourceProfiles: []domain.SourceProfile{{
				ID:        "openproject.test",
				Source:    "openproject",
				BaseURL:   server.URL,
				ProjectID: "demo",
				TokenEnv:  "REPLAN_TEST_OPENPROJECT_TOKEN",
			}},
		},
		Tasks: []domain.Task{{
			ID:             "task.op.1234",
			Title:          "Implement validation command",
			TrackingStatus: "ready",
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "work_package",
				ObjectID:   "1234",
			}},
		}, {
			ID:             "task.op.1235",
			Title:          "Review validation command",
			TrackingStatus: "ready",
			SourceRefs: []domain.SourceRef{{
				Source:     "openproject",
				ObjectType: "work_package",
				ObjectID:   "1235",
			}},
		}},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:           true,
		Profile:          "openproject.test",
		IncludeRelations: true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.DependencyCandidates) != 2 {
		t.Fatalf("DependencyCandidates len = %d, want 2: %#v", len(result.DependencyCandidates), result.DependencyCandidates)
	}
	candidate := result.DependencyCandidates[0]
	if candidate.SourceID != "7" {
		t.Fatalf("SourceID = %q, want 7", candidate.SourceID)
	}
	if candidate.DependencyType != "finish_to_start" {
		t.Fatalf("DependencyType = %q, want finish_to_start", candidate.DependencyType)
	}
	if candidate.PredecessorTaskID != "task.op.1234" || candidate.SuccessorTaskID != "task.op.1235" {
		t.Fatalf("unexpected dependency direction: %#v", candidate)
	}
	if candidate.AcceptStatus != "safe_to_accept" || !candidate.WouldCreate {
		t.Fatalf("unexpected accept fields: %#v", candidate)
	}
	unmatched := result.DependencyCandidates[1]
	if unmatched.SourceID != "9" {
		t.Fatalf("unmatched SourceID = %q, want 9", unmatched.SourceID)
	}
	if unmatched.AcceptStatus != "task_unmatched" || unmatched.WouldCreate {
		t.Fatalf("unexpected unmatched accept fields: %#v", unmatched)
	}
	if !hasDiagnostic(result.Warnings, "openproject_relation_type_unsupported") {
		t.Fatalf("expected unsupported relation warning: %#v", result.Warnings)
	}
	if !hasDiagnostic(result.Warnings, "openproject_relation_task_unmatched") {
		t.Fatalf("expected unmatched relation warning: %#v", result.Warnings)
	}
}

func TestOpenProjectImportIncludeRelationsFileModeWithoutRelationsIsEmpty(t *testing.T) {
	service := app.NewOpenProjectImportService(filestore.New("../../examples/small-team"))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile:         "../../examples/imports/openproject/work-packages.sample.json",
		DryRun:           true,
		IncludeRelations: true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.DependencyCandidates) != 0 {
		t.Fatalf("DependencyCandidates len = %d, want 0", len(result.DependencyCandidates))
	}
	if hasDiagnostic(result.Warnings, "openproject_relations_file_mode_unsupported") {
		t.Fatalf("did not expect file-mode unsupported warning: %#v", result.Warnings)
	}
}

func TestOpenProjectImportIncludesDependencyCandidatesFromFile(t *testing.T) {
	service := app.NewOpenProjectImportService(filestore.New("../../examples/replan-self"))

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		FromFile:         "../../examples/imports/openproject/replan-self.work-packages.sample.json",
		DryRun:           true,
		IncludeRelations: true,
	})

	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if len(result.DependencyCandidates) != 2 {
		t.Fatalf("DependencyCandidates len = %d, want 2: %#v", len(result.DependencyCandidates), result.DependencyCandidates)
	}
	first := result.DependencyCandidates[0]
	if first.SourceID != "9101" {
		t.Fatalf("first SourceID = %q, want 9101", first.SourceID)
	}
	if first.DependencyType != "finish_to_start" {
		t.Fatalf("first DependencyType = %q, want finish_to_start", first.DependencyType)
	}
	if first.PredecessorTaskID != "task.self.calibrate-plan-and-resources" || first.SuccessorTaskID != "task.self.expand-openproject-fixtures" {
		t.Fatalf("unexpected first dependency direction: %#v", first)
	}
	if first.AcceptStatus != "safe_to_accept" || !first.WouldCreate {
		t.Fatalf("unexpected first accept fields: %#v", first)
	}
	second := result.DependencyCandidates[1]
	if second.SourceID != "9102" {
		t.Fatalf("second SourceID = %q, want 9102", second.SourceID)
	}
	if second.DependencyType != "blocks" {
		t.Fatalf("second DependencyType = %q, want blocks", second.DependencyType)
	}
	if second.PredecessorTaskID != "task.self.write-oss-readiness-audit" || second.SuccessorTaskID != "task.self.prepare-release-story" {
		t.Fatalf("unexpected second dependency direction: %#v", second)
	}
	if second.AcceptStatus != "safe_to_accept" || !second.WouldCreate {
		t.Fatalf("unexpected second accept fields: %#v", second)
	}
	if hasDiagnostic(result.Warnings, "openproject_relations_file_mode_unsupported") {
		t.Fatalf("did not expect file-mode unsupported warning: %#v", result.Warnings)
	}
}

func TestOpenProjectImportAPIMissingToken(t *testing.T) {
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:   "project.test",
			Name: "Test",
			SourceProfiles: []domain.SourceProfile{{
				ID:        "openproject.test",
				Source:    "openproject",
				BaseURL:   "https://pm.example.test",
				ProjectID: "demo",
				TokenEnv:  "REPLAN_TEST_MISSING_OPENPROJECT_TOKEN",
			}},
		},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:  true,
		Profile: "openproject.test",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "openproject_token_missing") {
		t.Fatalf("expected openproject_token_missing error")
	}
}

func TestOpenProjectImportAPIStatusDiagnostic(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		fmt.Fprint(w, `{"_type":"Error","errorIdentifier":"forbidden","message":"not allowed"}`)
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:   "project.test",
			Name: "Test",
			SourceProfiles: []domain.SourceProfile{{
				ID:        "openproject.test",
				Source:    "openproject",
				BaseURL:   server.URL,
				ProjectID: "demo",
				TokenEnv:  "REPLAN_TEST_OPENPROJECT_TOKEN",
			}},
		},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:  true,
		Profile: "openproject.test",
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Import error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "openproject_permission_denied") {
		t.Fatalf("expected openproject_permission_denied error")
	}
}

func openProjectCollectionJSON(total, count, pageSize, id int, subject, status, assigneeName, assigneeHref string) string {
	assigneeJSON := "{}"
	assigneeLinkJSON := "{}"
	if assigneeName != "" {
		assigneeJSON = fmt.Sprintf(`{"name":%q}`, assigneeName)
		assigneeLinkJSON = fmt.Sprintf(`{"title":%q,"href":%q}`, assigneeName, assigneeHref)
	}
	return fmt.Sprintf(`{
  "_type": "Collection",
  "total": %d,
  "count": %d,
  "pageSize": %d,
  "_embedded": {
    "elements": [
      {
        "id": %d,
        "subject": %q,
        "description": {"raw": "Imported from API"},
        "status": {"name": %q},
        "assignee": %s,
        "estimatedTime": "PT1H",
        "spentTime": "PT0H",
        "_links": {
          "self": {"href": "/api/v3/work_packages/%s"},
          "status": {"title": %q},
          "assignee": %s
        }
      }
    ]
  }
}`, total, count, pageSize, id, subject, status, assigneeJSON, strconv.Itoa(id), status, assigneeLinkJSON)
}

func hasDiagnostic(diags []app.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func copyProjectFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	if err := copyDir("../../examples/small-team", dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func copyDir(src, dst string) error {
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
			continue
		}
		data, err := os.ReadFile(srcPath)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if err := os.WriteFile(dstPath, data, info.Mode()); err != nil {
			return err
		}
	}
	return nil
}

func writeOpenProjectResourceDemandSidecar(t *testing.T, projectDir string, content string) {
	t.Helper()
	path := filepath.Join(projectDir, "integrations/openproject/resource-demands.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir sidecar: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}

func writeOpenProjectResourceLinksSidecar(t *testing.T, projectDir string, content string) {
	t.Helper()
	path := filepath.Join(projectDir, "integrations/openproject/resource-links.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir resource links sidecar: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write resource links sidecar: %v", err)
	}
}

func containsString(haystack, needle string) bool {
	return strings.Contains(haystack, needle)
}

// A reverse proxy in front of OpenProject may ask API clients for a key in a
// request header; with it, the client's own token reaches OpenProject.
func TestOpenProjectImportSendsThePerimeterKeyOnEveryRequest(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	t.Setenv("REPLAN_TEST_PERIMETER_KEY", "perimeter-secret")
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if got := r.Header.Get("X-Perimeter-Key"); got != "perimeter-secret" {
			t.Fatalf("X-Perimeter-Key = %q on %s, want the key", got, r.URL.Path)
		}
		if user, token, ok := r.BasicAuth(); !ok || user != "apikey" || token != "test-token" {
			t.Fatalf("the client's own token did not travel beside the key on %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v3/projects/demo":
			fmt.Fprint(w, `{"_type":"Project","id":3,"identifier":"demo","name":"Demo"}`)
		case "/api/v3/work_packages":
			fmt.Fprint(w, openProjectCollectionJSON(1, 1, 100, 1234, "Perimeter task", "New", "", ""))
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test"},
	}})

	result, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun:          true,
		BaseURL:         server.URL,
		ProjectID:       "demo",
		TokenEnv:        "REPLAN_TEST_OPENPROJECT_TOKEN",
		PerimeterKeyEnv: "REPLAN_TEST_PERIMETER_KEY",
		IncludeClosed:   true,
	})
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if requests < 2 {
		t.Fatalf("only %d requests were made", requests)
	}
	encoded, _ := json.Marshal(result)
	if strings.Contains(string(encoded), "perimeter-secret") {
		t.Fatalf("the perimeter key leaked into the import result")
	}
}

func TestOpenProjectImportSendsNoPerimeterHeaderWithoutAKey(t *testing.T) {
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, present := r.Header["X-Perimeter-Key"]; present {
			t.Fatalf("a perimeter header was sent without a key")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v3/projects/demo" {
			fmt.Fprint(w, `{"_type":"Project","id":3,"identifier":"demo","name":"Demo"}`)
			return
		}
		fmt.Fprint(w, openProjectCollectionJSON(1, 1, 100, 1234, "Plain task", "New", "", ""))
	}))
	defer server.Close()
	service := app.NewOpenProjectImportService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test"},
	}})
	if _, err := service.Import(context.Background(), app.OpenProjectImportRequest{
		DryRun: true, BaseURL: server.URL, ProjectID: "demo", TokenEnv: "REPLAN_TEST_OPENPROJECT_TOKEN",
		PerimeterKeyEnv: "REPLAN_TEST_PERIMETER_KEY_UNSET", IncludeClosed: true,
	}); err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
}
