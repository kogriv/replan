package filestore

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func TestApplyOpenProjectBindingsReplacesTaskRefsAndWarnsForUnknownTask(t *testing.T) {
	workspace := domain.Workspace{Tasks: []domain.Task{
		{ID: "task.bound", SourceRefs: []domain.SourceRef{
			{Source: "openproject", ObjectType: "work_package", ObjectID: "sanitized"},
			{Source: "github", ObjectType: "issue", ObjectID: "9"},
		}},
		{ID: "task.unbound", SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "work_package", ObjectID: "sanitized-2"}}},
	}}
	document := &openProjectBindingsDoc{Root: openProjectBindingsRoot{Tasks: []openProjectTaskBinding{
		{TaskID: "task.bound", SourceRef: domain.SourceRef{Source: "openproject", ObjectType: "work_package", ObjectID: "83", URL: "https://op.example.internal/work_packages/83"}},
		{TaskID: "task.unknown", SourceRef: domain.SourceRef{ObjectID: "84"}},
	}}}
	var diagnostics []app.Diagnostic

	applyOpenProjectBindings(&workspace, document, &diagnostics)

	if got := workspace.Tasks[0].SourceRefs; len(got) != 2 || got[0].Source != "github" || got[1].ObjectID != "83" {
		t.Fatalf("bound source refs = %#v", got)
	}
	if got := workspace.Tasks[1].SourceRefs; len(got) != 0 {
		t.Fatalf("unbound source refs = %#v, want none", got)
	}
	if len(diagnostics) != 1 || diagnostics[0].Code != "binding_unknown_task" || diagnostics[0].Target.ID != "task.unknown" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
}

func TestOpenProjectBindingsFileErrorsBecomeLoadDiagnostics(t *testing.T) {
	project := copyProjectFixture(t)
	for _, test := range []struct {
		name, path, content, code string
	}{
		{name: "missing", path: filepath.Join(t.TempDir(), "missing.yaml"), code: "bindings_layer_missing"},
		{name: "malformed", path: filepath.Join(t.TempDir(), "bad.yaml"), content: "openproject_private_bindings: [unterminated\n", code: "bindings_layer_invalid"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.content != "" {
				if err := os.WriteFile(test.path, []byte(test.content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			_, diagnostics := NewWithOpenProjectBindings(project, test.path).Load(context.Background())
			for _, diagnostic := range diagnostics {
				if diagnostic.Code == test.code {
					return
				}
			}
			t.Fatalf("diagnostics = %#v, want %s", diagnostics, test.code)
		})
	}
}

func TestOpenProjectBindingsLayerRefusesEveryWrite(t *testing.T) {
	project := copyProjectFixture(t)
	bindings := filepath.Join(t.TempDir(), "bindings.yaml")
	if err := os.WriteFile(bindings, []byte("openproject_private_bindings:\n  tasks: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewWithOpenProjectBindings(project, bindings)

	if _, err := store.SetProjectStatusDate("2026-07-20"); !errors.Is(err, ErrBindingsLayerReadOnly) {
		t.Fatalf("direct write error = %v, want ErrBindingsLayerReadOnly", err)
	}
	staged := store.StageWrites()
	if _, err := staged.Commit(); !errors.Is(err, ErrBindingsLayerReadOnly) {
		t.Fatalf("staged commit error = %v, want ErrBindingsLayerReadOnly", err)
	}
}
