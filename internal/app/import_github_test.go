package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

const (
	githubSample  = "../../examples/imports/github/issues.sample.json"
	githubLinks   = "../../examples/imports/github/resource-links.sample.yaml"
	githubDemands = "../../examples/imports/github/resource-demands.sample.yaml"
)

func TestGitHubImportMapsApprovedRows(t *testing.T) {
	result := importGitHubSample(t, filestore.New("../../examples/small-team"), true, true, "")
	if result.Snapshot.LoadedCount != 4 {
		t.Fatalf("LoadedCount = %d, want 4 (pull request filtered)", result.Snapshot.LoadedCount)
	}
	byID := map[string]app.OpenProjectTaskCandidate{}
	for _, candidate := range result.TaskCandidates {
		byID[candidate.SourceID] = candidate
	}
	issue := byID["example-org/planning-demo#101"]
	if issue.NumericID != 1000101 || issue.MappedStatus != "in_progress" || issue.MatchedTaskID != "task.op.1234" {
		t.Fatalf("issue 101 mapping = %#v", issue)
	}
	if issue.StartDate != "2026-10-29" || issue.DueDate != "2026-11-03" {
		t.Fatalf("issue 101 dates = %s..%s", issue.StartDate, issue.DueDate)
	}
	if parent := byID["example-org/planning-demo#103"].ParentRef; parent == nil || parent.ObjectID != "example-org/planning-demo#101" {
		t.Fatalf("sub-issue parent = %#v", parent)
	}
	if byID["example-org/planning-demo#102"].MappedStatus != "accepted" || byID["example-org/planning-demo#105"].MappedStatus != "cancelled" {
		t.Fatalf("duplicate status = %q", byID["example-org/planning-demo#105"].MappedStatus)
	}
	if len(result.DependencyCandidates) != 1 || result.DependencyCandidates[0].DependencyType != "finish_to_start" {
		t.Fatalf("dependencies = %#v", result.DependencyCandidates)
	}
	if !diagnosticsHaveCode(result.Warnings, "github_cross_repository_blocked_by_skipped") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	if len(result.ResourceCandidates) != 2 || result.ResourceCandidates[0].MatchReason != "resource_link" {
		t.Fatalf("resource candidates = %#v", result.ResourceCandidates)
	}
	if len(result.ResourceDemandCandidates) != 1 || result.ResourceDemandCandidates[0].Canonical.PlannedAmount != 13 {
		t.Fatalf("resource demand candidates = %#v", result.ResourceDemandCandidates)
	}
}

func TestGitHubImportFiltersSinceAndClosed(t *testing.T) {
	result := importGitHubSample(t, filestore.New("../../examples/small-team"), false, false, "2026-10-29")
	if len(result.TaskCandidates) != 1 || result.TaskCandidates[0].SourceID != "example-org/planning-demo#103" {
		t.Fatalf("filtered candidates = %#v", result.TaskCandidates)
	}
}

func TestGitHubImportReportsUnknownStatusAndMissingSidecarLink(t *testing.T) {
	dir := t.TempDir()
	doc := `{"format":"replan.github-issues.v1","repository":{"owner":"example-org","name":"planning-demo"},"issues":[{"id":9,"number":9,"title":"Unknown state","state":"queued","updated_at":"2026-10-29T00:00:00Z","assignees":[{"login":"unmapped-user"}]}]}`
	path := filepath.Join(dir, "input.json")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := app.NewGitHubImportService(filestore.New("../../examples/small-team")).Import(context.Background(), app.GitHubImportRequest{FromFile: path, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !diagnosticsHaveCode(result.Warnings, "unmapped_github_status") || !diagnosticsHaveCode(result.Warnings, "github_assignee_link_missing") {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

func TestGitHubImportWritesSnapshot(t *testing.T) {
	dir := copyProjectFixture(t)
	result, err := app.NewGitHubImportService(filestore.New(dir)).Import(context.Background(), githubImportRequest(false, true, true, ""))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Applied || len(result.WrittenFiles) != 3 {
		t.Fatalf("result = %#v", result)
	}
	for _, rel := range result.WrittenFiles {
		if _, err := os.Stat(filepath.Join(dir, rel)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestGitHubSnapshotAcceptModesGolden(t *testing.T) {
	modes := []struct {
		name      string
		set       func(*app.ImportAcceptRequest)
		errorCode string
	}{
		{"tasks", func(*app.ImportAcceptRequest) {}, ""},
		{"resource-links", func(req *app.ImportAcceptRequest) { req.AcceptResourceLinks = true }, ""},
		{"tracking-status", func(req *app.ImportAcceptRequest) { req.AcceptTrackingStatus = true }, ""},
		{"dates", func(req *app.ImportAcceptRequest) { req.AcceptDates = true }, ""},
		{"dependencies", func(req *app.ImportAcceptRequest) { req.AcceptDependencies = true }, ""},
		{"resource-demands", func(req *app.ImportAcceptRequest) { req.AcceptResourceDemands = true }, ""},
		{"time-entries", func(req *app.ImportAcceptRequest) { req.AcceptTimeEntries = true }, "import_accept_time_entries_unsupported_source"},
	}
	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			dir := copyProjectFixture(t)
			imported, err := app.NewGitHubImportService(filestore.New(dir)).Import(context.Background(), githubImportRequest(false, true, true, ""))
			if err != nil {
				t.Fatal(err)
			}
			req := app.ImportAcceptRequest{SnapshotID: imported.Snapshot.ID, DryRun: true, ActorID: "actor.human.operator", Reason: "GitHub golden run.", Now: time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)}
			mode.set(&req)
			result, acceptErr := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), req)
			record := map[string]any{"result": result, "error": errorString(acceptErr)}
			got, err := json.MarshalIndent(record, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			got = []byte(strings.ReplaceAll(string(got), dir, "<project>") + "\n")
			path := filepath.Join("testdata", "github-import-accept", mode.name+".dry-run.json")
			if os.Getenv("REPLAN_UPDATE_GOLDEN") == "1" {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden %s: %v (REPLAN_UPDATE_GOLDEN=1 writes it)", path, err)
			}
			if string(got) != string(want) {
				t.Fatalf("result differs from %s", path)
			}
			if mode.errorCode == "" && acceptErr != nil {
				t.Fatal(acceptErr)
			}
			if mode.errorCode != "" {
				if !errors.Is(acceptErr, app.ErrValidationFailed) || !diagnosticsHaveCode(result.Errors, mode.errorCode) {
					t.Fatalf("accept error = %v, diagnostics = %#v", acceptErr, result.Errors)
				}
			}
		})
	}
}

func TestGitHubSnapshotAcceptModesApply(t *testing.T) {
	modes := []func(*app.ImportAcceptRequest){
		func(*app.ImportAcceptRequest) {},
		func(req *app.ImportAcceptRequest) { req.AcceptResourceLinks = true },
		func(req *app.ImportAcceptRequest) { req.AcceptTrackingStatus = true },
		func(req *app.ImportAcceptRequest) { req.AcceptDates = true },
		func(req *app.ImportAcceptRequest) { req.AcceptDependencies = true },
		func(req *app.ImportAcceptRequest) { req.AcceptResourceDemands = true },
	}
	for index, set := range modes {
		dir := copyProjectFixture(t)
		imported, err := app.NewGitHubImportService(filestore.New(dir)).Import(context.Background(), githubImportRequest(false, true, true, ""))
		if err != nil {
			t.Fatal(err)
		}
		req := app.ImportAcceptRequest{SnapshotID: imported.Snapshot.ID, Apply: true, ActorID: "actor.human.operator", Now: time.Date(2026, 10, 29, 12, index, 0, 0, time.UTC)}
		set(&req)
		result, err := app.NewImportAcceptService(filestore.New(dir)).Accept(context.Background(), req)
		if err != nil {
			t.Fatalf("mode %d: %v (%#v)", index, err, result.Errors)
		}
		if !result.Applied {
			t.Fatalf("mode %d was not applied: %#v", index, result.Warnings)
		}
	}
}

func importGitHubSample(t *testing.T, store app.WorkspaceLoader, includeClosed, includeRelations bool, since string) app.GitHubImportResult {
	t.Helper()
	result, err := app.NewGitHubImportService(store).Import(context.Background(), githubImportRequest(true, includeClosed, includeRelations, since))
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func githubImportRequest(dryRun, includeClosed, includeRelations bool, since string) app.GitHubImportRequest {
	return app.GitHubImportRequest{FromFile: githubSample, DryRun: dryRun, Profile: "github.demo", Since: since, IncludeClosed: includeClosed, IncludeRelations: includeRelations, ResourceLinksSidecar: absoluteGitHubFixture(githubLinks), ResourceDemandsSidecar: absoluteGitHubFixture(githubDemands), Now: time.Date(2026, 10, 29, 11, 0, 0, 0, time.UTC)}
}

func absoluteGitHubFixture(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

func diagnosticsHaveCode(diagnostics []app.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
