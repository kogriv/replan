package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func evidenceRequest() app.EvidenceAddRequest {
	return app.EvidenceAddRequest{
		ID:         "evidence.test.cli-add",
		Type:       "test_result",
		Subtype:    "unit_test",
		Source:     "local",
		URL:        "https://example.test/evidence/1",
		ActorID:    "actor.human.operator",
		ObservedAt: "2026-07-08T10:00:00+07:00",
		Confidence: "observed",
		Summary:    "Evidence add test passed.",
		Targets:    []domain.Ref{{Kind: "task", ID: "task.op.1234"}, {Kind: "resource_demand", ID: "resource_demand.task-op-1234.implementer"}},
	}
}

func TestEvidenceAddExistingWeekPreservesBytesAndValidates(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	path := filepath.Join(projectPath, "evidence", "evidence.2026-W28.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.NewEvidenceService(filestore.New(projectPath)).Add(context.Background(), evidenceRequest())
	if err != nil {
		t.Fatalf("Add returned error: %v; diagnostics=%#v", err, result.Errors)
	}
	if !result.Applied || result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("write was not applied and validated: %#v", result)
	}
	if result.FilePath != "evidence/evidence.2026-W28.yaml" {
		t.Fatalf("file path = %q", result.FilePath)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(after), string(before)) {
		t.Fatal("existing evidence file bytes changed")
	}
	if !strings.Contains(string(after[len(before):]), "  - id: evidence.test.cli-add\n") {
		t.Fatalf("new evidence was not appended at the end:\n%s", after[len(before):])
	}
	audit, err := os.ReadFile(filepath.Join(projectPath, "audit", "audit.2026-07.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"operation":"evidence_add"`, `"kind":"actor","id":"actor.human.operator"`, `"kind":"evidence","id":"evidence.test.cli-add"`, `"result":"success"`} {
		if !strings.Contains(string(audit), want) {
			t.Errorf("audit does not contain %s", want)
		}
	}
}

func TestEvidenceAddCreatesNewWeekFile(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	req := evidenceRequest()
	req.ID = "evidence.test.new-week"
	req.ObservedAt = "2030-01-08T10:00:00Z"
	result, err := app.NewEvidenceService(filestore.New(projectPath)).Add(context.Background(), req)
	if err != nil {
		t.Fatalf("Add returned error: %v; diagnostics=%#v", err, result.Errors)
	}
	data, err := os.ReadFile(filepath.Join(projectPath, result.FilePath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "evidence:\n") || !strings.Contains(string(data), "- id: evidence.test.new-week\n") {
		t.Fatalf("new week file has unexpected content:\n%s", data)
	}
}

func TestEvidenceAddDryRunWritesNothing(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	before, err := os.ReadFile(filepath.Join(projectPath, "evidence", "evidence.2026-W28.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	req := evidenceRequest()
	req.DryRun = true
	result, err := app.NewEvidenceService(filestore.New(projectPath)).Add(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if result.Applied || result.FilePath != "evidence/evidence.2026-W28.yaml" || len(result.PlannedWrites) != 1 {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	after, err := os.ReadFile(filepath.Join(projectPath, "evidence", "evidence.2026-W28.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("dry run changed evidence file")
	}
}

func TestEvidenceAddRefusals(t *testing.T) {
	tests := []struct {
		name string
		edit func(*app.EvidenceAddRequest)
		code string
	}{
		{name: "duplicate id", edit: func(r *app.EvidenceAddRequest) { r.ID = "evidence.github.pr.45" }, code: "evidence_already_exists"},
		{name: "unknown target", edit: func(r *app.EvidenceAddRequest) { r.Targets = []domain.Ref{{Kind: "task", ID: "task.missing"}} }, code: "evidence_target_not_found"},
		{name: "unknown target kind", edit: func(r *app.EvidenceAddRequest) { r.Targets = []domain.Ref{{Kind: "widget", ID: "widget.1"}} }, code: "invalid_evidence_target_kind"},
		{name: "unknown actor", edit: func(r *app.EvidenceAddRequest) { r.ActorID = "actor.missing" }, code: "actor_not_found"},
		{name: "bad time", edit: func(r *app.EvidenceAddRequest) { r.ObservedAt = "2026-07-08 10:00:00" }, code: "invalid_observed_at"},
		{name: "missing required type", edit: func(r *app.EvidenceAddRequest) { r.Type = "" }, code: "missing_evidence_type"},
		{name: "missing target", edit: func(r *app.EvidenceAddRequest) { r.Targets = nil }, code: "missing_target"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectPath := copyAppProjectFixture(t)
			req := evidenceRequest()
			tc.edit(&req)
			result, err := app.NewEvidenceService(filestore.New(projectPath)).Add(context.Background(), req)
			if err == nil {
				t.Fatal("expected refusal")
			}
			found := false
			for _, diagnostic := range result.Errors {
				if diagnostic.Code == tc.code {
					found = true
				}
			}
			if !found {
				t.Fatalf("errors = %#v, want %s", result.Errors, tc.code)
			}
		})
	}
}
