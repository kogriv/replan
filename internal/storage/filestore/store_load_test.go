package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadProjectRoot(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(t *testing.T, path string)
		wantCode string
	}{
		{name: "missing", wantCode: "project_not_found"},
		{
			name: "regular file",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("not a project directory\n"), 0o600); err != nil {
					t.Fatalf("write regular file: %v", err)
				}
			},
			wantCode: "project_not_found",
		},
		{
			name: "empty directory",
			prepare: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("create empty directory: %v", err)
				}
			},
			wantCode: "missing_required_file",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectPath := filepath.Join(t.TempDir(), "project")
			if test.prepare != nil {
				test.prepare(t, projectPath)
			}

			_, diagnostics := New(projectPath).Load(context.Background())
			if len(diagnostics) == 0 {
				t.Fatal("Load diagnostics are empty")
			}
			if diagnostics[0].Code != test.wantCode {
				t.Fatalf("Load first diagnostic code = %q, want %q", diagnostics[0].Code, test.wantCode)
			}
			if test.wantCode == "project_not_found" && len(diagnostics) != 1 {
				t.Fatalf("Load diagnostics = %d, want exactly one", len(diagnostics))
			}
			if test.wantCode == "project_not_found" {
				diagnostic := diagnostics[0]
				if !strings.Contains(diagnostic.Message, projectPath) {
					t.Fatalf("Load diagnostic message = %q, want project path %q", diagnostic.Message, projectPath)
				}
				if diagnostic.Target.Kind != "file" || diagnostic.Target.ID != projectPath {
					t.Fatalf("Load diagnostic target = %#v, want file %q", diagnostic.Target, projectPath)
				}
			}
		})
	}
}

func TestLoadProjectFormatVersion(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		wantOK    bool
		wantFound string
	}{
		{name: "missing", wantOK: true},
		{name: "one", value: "1", wantOK: true},
		{name: "too new", value: "2", wantFound: "2"},
		{name: "zero", value: "0", wantFound: "0"},
		{name: "negative", value: "-1", wantFound: "-1"},
		{name: "not integer", value: `"x"`, wantFound: `"x"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectPath := copyProjectFixture(t)
			manifestPath := filepath.Join(projectPath, ".replan", "project.yaml")
			data, err := os.ReadFile(manifestPath)
			if err != nil {
				t.Fatal(err)
			}
			text := strings.Replace(string(data), "  format_version: 1\n", "", 1)
			if test.value != "" {
				text = strings.Replace(text, "  timezone:", "  format_version: "+test.value+"\n  timezone:", 1)
			}
			if err := os.WriteFile(manifestPath, []byte(text), 0o644); err != nil {
				t.Fatal(err)
			}

			workspace, diagnostics := New(projectPath).Load(context.Background())
			if test.wantOK {
				if len(diagnostics) != 0 {
					t.Fatalf("Load diagnostics = %#v, want none", diagnostics)
				}
				if workspace.Project.FormatVersion != 1 {
					t.Fatalf("format version = %d, want 1", workspace.Project.FormatVersion)
				}
				return
			}
			if len(diagnostics) != 1 || diagnostics[0].Code != "project_format_unsupported" {
				t.Fatalf("Load diagnostics = %#v, want one project_format_unsupported", diagnostics)
			}
			if !strings.Contains(diagnostics[0].Message, test.wantFound) || !strings.Contains(diagnostics[0].Message, "supports version 1") {
				t.Fatalf("diagnostic message = %q", diagnostics[0].Message)
			}
			if len(workspace.CheckedFiles) != 0 || len(workspace.Tasks) != 0 {
				t.Fatalf("unsupported project loaded additional files: %#v", workspace)
			}
		})
	}
}
