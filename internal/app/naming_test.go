package app_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestResourceSetName(t *testing.T) {
	base := app.ResourceSetNameRequest{ResourceID: "resource.human.ivan", Name: "Ivan Petrov", Reason: "Record the new role name.", ActorID: "actor.human.operator", EvidenceID: "evidence.github.pr.45", OccurredAt: "2026-10-28T14:00:00+07:00"}

	t.Run("dry run", func(t *testing.T) {
		project := copyAppProjectFixture(t)
		before := snapshotTaskProject(t, project)
		req := base
		req.DryRun = true
		result, err := app.NewResourceService(filestore.New(project)).SetName(context.Background(), req)
		if err != nil || result.Applied || result.NoOp || len(result.PlannedWrites) != 2 {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		if !reflect.DeepEqual(before, snapshotTaskProject(t, project)) {
			t.Fatal("dry run wrote project files")
		}
	})

	t.Run("apply changes one line and audits", func(t *testing.T) {
		project := copyAppProjectFixture(t)
		path := filepath.Join(project, "canonical", "resources.yaml")
		before := readTaskTestFile(t, project, "canonical/resources.yaml")
		result, err := app.NewResourceService(filestore.New(project)).SetName(context.Background(), base)
		if err != nil || !result.Applied || result.PreviousName != "Ivan" || result.NewName != "Ivan Petrov" {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		afterBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		assertOneChangedLine(t, before, string(afterBytes), "    name: Ivan", "    name: Ivan Petrov")
		audit := readTaskTestFile(t, project, "audit/audit.2026-10.jsonl")
		if strings.Count(audit, `"operation":"resource_set_name"`) != 1 || !strings.Contains(audit, `Resource name: \"Ivan\" -\u003e \"Ivan Petrov\".`) {
			t.Fatalf("audit does not contain one old/new name record: %s", audit)
		}
	})

	t.Run("unknown empty and same", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			edit func(*app.ResourceSetNameRequest)
			code string
		}{
			{name: "unknown", edit: func(req *app.ResourceSetNameRequest) { req.ResourceID = "resource.missing" }, code: "resource_not_found"},
			{name: "empty", edit: func(req *app.ResourceSetNameRequest) { req.Name = " \t" }, code: "missing_resource_name"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				project := copyAppProjectFixture(t)
				req := base
				tc.edit(&req)
				result, err := app.NewResourceService(filestore.New(project)).SetName(context.Background(), req)
				if err == nil {
					t.Fatal("expected refusal")
				}
				assertTaskDiag(t, result.Errors, tc.code)
			})
		}
		project := copyAppProjectFixture(t)
		before := snapshotTaskProject(t, project)
		req := base
		req.Name = "Ivan"
		result, err := app.NewResourceService(filestore.New(project)).SetName(context.Background(), req)
		if err != nil || !result.NoOp || result.Applied || len(result.PlannedWrites) != 0 || !reflect.DeepEqual(before, snapshotTaskProject(t, project)) {
			t.Fatalf("result=%#v err=%v", result, err)
		}
	})
}

func TestActorSetName(t *testing.T) {
	base := app.ActorSetNameRequest{TargetActorID: "actor.human.ivan", Name: "Ivan Petrov", Reason: "Record the new role name.", ActorID: "actor.ai.codex.local", EvidenceID: "evidence.github.pr.45", OccurredAt: "2026-10-28T14:05:00+07:00"}

	t.Run("dry run", func(t *testing.T) {
		project := copyAppProjectFixture(t)
		before := snapshotTaskProject(t, project)
		req := base
		req.DryRun = true
		result, err := app.NewActorService(filestore.New(project)).SetName(context.Background(), req)
		if err != nil || result.Applied || result.NoOp || len(result.PlannedWrites) != 2 {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		if !reflect.DeepEqual(before, snapshotTaskProject(t, project)) {
			t.Fatal("dry run wrote project files")
		}
	})

	t.Run("apply changes one line and audits", func(t *testing.T) {
		project := copyAppProjectFixture(t)
		path := filepath.Join(project, "canonical", "actors.yaml")
		before := readTaskTestFile(t, project, "canonical/actors.yaml")
		result, err := app.NewActorService(filestore.New(project)).SetName(context.Background(), base)
		if err != nil || !result.Applied || result.PreviousName != "Ivan" || result.NewName != "Ivan Petrov" {
			t.Fatalf("result=%#v err=%v", result, err)
		}
		afterBytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		assertOneChangedLine(t, before, string(afterBytes), "    display_name: Ivan", "    display_name: Ivan Petrov")
		audit := readTaskTestFile(t, project, "audit/audit.2026-10.jsonl")
		if strings.Count(audit, `"operation":"actor_set_name"`) != 1 || !strings.Contains(audit, `Actor display name: \"Ivan\" -\u003e \"Ivan Petrov\".`) {
			t.Fatalf("audit does not contain one old/new name record: %s", audit)
		}
	})

	t.Run("unknown empty and same", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			edit func(*app.ActorSetNameRequest)
			code string
		}{
			{name: "unknown", edit: func(req *app.ActorSetNameRequest) { req.TargetActorID = "actor.missing" }, code: "actor_not_found"},
			{name: "empty", edit: func(req *app.ActorSetNameRequest) { req.Name = " \t" }, code: "missing_actor_name"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				project := copyAppProjectFixture(t)
				req := base
				tc.edit(&req)
				result, err := app.NewActorService(filestore.New(project)).SetName(context.Background(), req)
				if err == nil {
					t.Fatal("expected refusal")
				}
				assertTaskDiag(t, result.Errors, tc.code)
			})
		}
		project := copyAppProjectFixture(t)
		before := snapshotTaskProject(t, project)
		req := base
		req.Name = "Ivan"
		result, err := app.NewActorService(filestore.New(project)).SetName(context.Background(), req)
		if err != nil || !result.NoOp || result.Applied || len(result.PlannedWrites) != 0 || !reflect.DeepEqual(before, snapshotTaskProject(t, project)) {
			t.Fatalf("result=%#v err=%v", result, err)
		}
	})
}

func assertOneChangedLine(t *testing.T, before, after, wantBefore, wantAfter string) {
	t.Helper()
	oldLines, newLines := strings.Split(before, "\n"), strings.Split(after, "\n")
	if len(oldLines) != len(newLines) {
		t.Fatalf("line count changed: %d -> %d", len(oldLines), len(newLines))
	}
	changed := 0
	for i := range oldLines {
		if oldLines[i] == newLines[i] {
			continue
		}
		changed++
		if oldLines[i] != wantBefore || newLines[i] != wantAfter {
			t.Fatalf("unexpected line %d change: %q -> %q", i+1, oldLines[i], newLines[i])
		}
	}
	if changed != 1 {
		t.Fatalf("changed lines = %d, want 1", changed)
	}
}
