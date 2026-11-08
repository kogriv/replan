package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestProjectFormatDryRunAndApplyAudit(t *testing.T) {
	project := copyCmdProjectFixture(t)
	tasksPath := filepath.Join(project, "canonical", "tasks.yaml")
	original, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(original, &root); err != nil {
		t.Fatal(err)
	}
	fourSpace, err := yaml.Marshal(&root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tasksPath, fourSpace, 0o644); err != nil {
		t.Fatal(err)
	}

	args := []string{"project", "format", "--project", project, "--actor", "actor.human.operator", "--reason", "Normalize YAML", "--format", "json"}
	stdout, stderr := tempOutputFiles(t)
	if code := run(append(args, "--dry-run"), stdout, stderr); code != exitSuccess {
		t.Fatalf("dry run exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if got, err := os.ReadFile(tasksPath); err != nil || string(got) != string(fourSpace) {
		t.Fatalf("dry run changed tasks: err=%v", err)
	}
	var dry cliTestEnvelope
	if err := json.Unmarshal([]byte(readFileString(t, stdout.Name())), &dry); err != nil {
		t.Fatal(err)
	}
	result := resultObject(t, dry)
	if result["changed_files"].(float64) < 1 || result["changed_lines"].(float64) < 1 {
		t.Fatalf("dry-run result = %#v", result)
	}

	beforeAudit := auditLineCount(t, project)
	stdout, stderr = tempOutputFiles(t)
	if code := run(append(args, "--apply"), stdout, stderr); code != exitSuccess {
		t.Fatalf("apply exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	after, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "tasks:\n  - id:") || strings.Contains(string(after), "tasks:\n    - id:") {
		t.Fatalf("tasks not canonical after apply:\n%s", after)
	}
	if got := auditLineCount(t, project); got != beforeAudit+1 {
		t.Fatalf("audit lines = %d, want %d", got, beforeAudit+1)
	}
	if !auditContainsOperation(t, project, "project_format") {
		t.Fatal("project_format audit record not found")
	}
}

func TestProjectFormatLeavesFixtureWithoutSchedulingFieldsByteIdentical(t *testing.T) {
	project := copyCmdProjectFixture(t)
	before := snapshotProjectFiles(t, project)
	removeFormatArtifacts(before)
	stdout, stderr := tempOutputFiles(t)
	args := []string{"project", "format", "--project", project, "--actor", "actor.human.operator", "--reason", "Compatibility check", "--apply", "--format", "json"}
	if code := run(args, stdout, stderr); code != exitSuccess {
		t.Fatalf("format exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	after := snapshotProjectFiles(t, project)
	removeFormatArtifacts(after)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("format changed byte-canonical fixture without scheduling fields")
	}
}

func removeFormatArtifacts(files map[string]string) {
	for path := range files {
		if path == filepath.Join(".replan", "write.lock") || strings.HasPrefix(filepath.ToSlash(path), "audit/") {
			delete(files, path)
		}
	}
}

func auditLineCount(t *testing.T, project string) int {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(project, "audit", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		count += len(strings.FieldsFunc(string(data), func(r rune) bool { return r == '\n' }))
	}
	return count
}

func auditContainsOperation(t *testing.T, project, operation string) bool {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(project, "audit", "*.jsonl"))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), `"operation":"`+operation+`"`) {
			return true
		}
	}
	return false
}
