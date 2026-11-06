package filestore

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestFormatCanonicalYAMLPreviewAndApply(t *testing.T) {
	root := t.TempDir()
	threeSpace := "# keep this comment\ntasks:\n   - id: task.a\n     title: One\n     planned_start: \"2026-11-05\"\n"
	fourSpace := "resources:\n    - id: resource.a\n      class: human\n      name: A\n"
	writeHandFile(t, root, tasksRel, threeSpace)
	writeHandFile(t, root, resourcesRel, fourSpace)

	store := New(root)
	changes, err := store.FormatCanonicalYAML(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0].ChangedLines == 0 || changes[1].ChangedLines == 0 {
		t.Fatalf("preview changes = %#v", changes)
	}
	if got := readHandFile(t, root, tasksRel); got != threeSpace {
		t.Fatalf("dry run wrote tasks:\n%s", got)
	}

	changes, err = store.FormatCanonicalYAML(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 {
		t.Fatalf("apply changes = %#v", changes)
	}
	for rel, before := range map[string]string{tasksRel: threeSpace, resourcesRel: fourSpace} {
		after := readHandFile(t, root, rel)
		if !strings.Contains(after, "  - id:") {
			t.Fatalf("%s is not canonical two-space YAML:\n%s", rel, after)
		}
		if rel == tasksRel && (!strings.Contains(after, "# keep this comment") || !strings.Contains(after, `planned_start: "2026-11-05"`)) {
			t.Fatalf("comment or date quoting was lost:\n%s", after)
		}
		assertYAMLDataEqual(t, []byte(before), []byte(after))
	}
}

func TestFormatCanonicalYAMLRefusesDataChange(t *testing.T) {
	root := t.TempDir()
	writeHandFile(t, root, tasksRel, "tasks:\n    - id: task.a\n      title: One\n")
	original := formatDataEqual
	formatDataEqual = func(_, _ []byte) bool { return false }
	t.Cleanup(func() { formatDataEqual = original })

	_, err := New(root).FormatCanonicalYAML(true)
	var refusal FormatWouldChangeDataError
	if !errors.As(err, &refusal) {
		t.Fatalf("error = %T %v, want FormatWouldChangeDataError", err, err)
	}
	if got := readHandFile(t, root, tasksRel); !strings.Contains(got, "    - id:") {
		t.Fatalf("refusal wrote file:\n%s", got)
	}
}

func assertYAMLDataEqual(t *testing.T, before, after []byte) {
	t.Helper()
	var a, b any
	if err := yaml.Unmarshal(before, &a); err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(after, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("data differs: %#v != %#v", a, b)
	}
}

func TestCreateProjectFileCanonicalizesYAML(t *testing.T) {
	root := t.TempDir()
	content := []byte("scenario:\n    id: scenario.a\n    name: A\n")
	if _, err := New(root).CreateProjectFile("scenarios/scenario.a.yaml", content); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "scenarios", "scenario.a.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "    id:") || !strings.Contains(string(got), "  id: scenario.a") {
		t.Fatalf("new YAML file is not canonical:\n%s", got)
	}
}
