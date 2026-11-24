package filestore

import (
	"context"
	"errors"
	"fmt"
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

func TestOpenProjectBindingsDisplayBase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bindings.yaml")
	const base = "https://bound.example.test/pm"
	if err := os.WriteFile(path, []byte("openproject_private_bindings:\n  project:\n    base_url: "+base+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	workspace, diagnostics := NewWithOpenProjectBindings(copyProjectFixture(t), path).Load(context.Background())
	if len(diagnostics) != 0 {
		t.Fatalf("load binding layer: %#v", diagnostics)
	}
	if workspace.OpenProjectBindingBaseURL != base {
		t.Fatalf("binding display base = %q, want %q", workspace.OpenProjectBindingBaseURL, base)
	}
}

func TestOpenProjectBindingsReportDisplayURL(t *testing.T) {
	project := copyProjectFixture(t)
	const bindingBase = "https://api.example.test/pm"
	const displayURL = "https://display.example.test/rebased/work_packages/42"
	for _, tc := range []struct{ name, base, raw, want string }{
		{"separate display host", bindingBase, "  HTTPS://display.example.test/rebased/work_packages/42  ", displayURL},
		{"omitted base", "", displayURL, displayURL},
		{"relative API link", bindingBase, "/pm/api/v3/work_packages/42", bindingBase + "/work_packages/42"},
		{"absolute API link", bindingBase, "https://display.example.test/pm/api/v3/work_packages/42", bindingBase + "/work_packages/42"},
		{"mismatched ID", bindingBase, "https://display.example.test/rebased/work_packages/43", bindingBase + "/work_packages/42"},
		{"invalid host without binding base", "", "https://bad..test/work_packages/42", "https://pm.example.test/work_packages/42"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			document := "openproject_private_bindings:\n"
			if tc.base != "" {
				document += "  project:\n    base_url: " + tc.base + "\n"
			}
			document += fmt.Sprintf(`  tasks:
    - replan_task_id: task.op.1234
      source_ref:
        object_id: "42"
        url: %q
`, tc.raw)
			bindings := filepath.Join(t.TempDir(), "bindings.yaml")
			if err := os.WriteFile(bindings, []byte(document), 0o600); err != nil {
				t.Fatal(err)
			}
			store := NewWithOpenProjectBindings(project, bindings)
			ws, diagnostics := store.Load(context.Background())
			if len(diagnostics) != 0 || !ws.OpenProjectBindingsLoaded {
				t.Fatalf("binding provenance missing, diagnostics = %#v", diagnostics)
			}
			report, err := app.NewReportService(store).Dashboard(context.Background(), app.DashboardReportRequest{Week: "2026-W30"})
			if err != nil {
				t.Fatal(err)
			}
			rows := report.OpenProjectOverlay.WorkPackages
			if len(rows) != 1 || rows[0].WorkPackageID != "42" || rows[0].WorkPackageURL != tc.want {
				t.Fatalf("overlay work packages = %#v, want one row for 42 with URL %q", rows, tc.want)
			}
		})
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
