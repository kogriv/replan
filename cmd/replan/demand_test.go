package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemandCommandsApplyBatchDryRunAndRefusal(t *testing.T) {
	project := copyCmdProjectFixture(t)
	common := []string{"--project", project, "--reason", "Reviewed change", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--occurred-at", "2026-07-08T10:00:00+07:00", "--format", "json"}
	add := append([]string{"demand", "add"}, common...)
	add = append(add, "--task", "task.op.1234", "--suffix", "docs", "--resource", "resource.human.petr", "--role", "writer", "--unit", "hours", "--amount", "3", "--apply")
	stdout, stderr := tempOutputFiles(t)
	if code := run(add, stdout, stderr); code != exitSuccess {
		t.Fatalf("add exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	for _, check := range []struct{ path, text string }{
		{"canonical/resource_demands.yaml", "  - id: resource_demand.1234.docs\n"},
		{"canonical/tasks.yaml", "id: resource_demand.1234.docs\n"},
		{"audit/audit.2026-07.jsonl", `"operation":"demand_add"`},
	} {
		if got := readFileString(t, filepath.Join(project, check.path)); !strings.Contains(got, check.text) {
			t.Fatalf("%s lacks %q:\n%s", check.path, check.text, got)
		}
	}

	batch := filepath.Join(t.TempDir(), "set.yaml")
	if err := os.WriteFile(batch, []byte("demands:\n  - id: resource_demand.1234.docs\n    amount: 4\n    remaining: 2\n  - id: resource_demand.missing\n    amount: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before := readFileString(t, filepath.Join(project, "canonical/resource_demands.yaml"))
	set := append([]string{"demand", "set"}, common...)
	set = append(set, "--from-file", batch, "--apply")
	stdout, stderr = tempOutputFiles(t)
	if code := run(set, stdout, stderr); code != exitNotFound {
		t.Fatalf("batch refusal exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if got := readFileString(t, filepath.Join(project, "canonical/resource_demands.yaml")); got != before {
		t.Fatal("failed batch wrote demand file")
	}

	move := append([]string{"demand", "move", "resource_demand.1234.docs"}, common...)
	move = append(move, "--suffix", "docs-ivan", "--to-resource", "resource.human.ivan", "--dry-run")
	stdout, stderr = tempOutputFiles(t)
	if code := run(move, stdout, stderr); code != exitSuccess {
		t.Fatalf("move dry-run exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}
	if got := readFileString(t, filepath.Join(project, "canonical/resource_demands.yaml")); got != before {
		t.Fatal("move dry run wrote demand file")
	}

	set = append([]string{"demand", "set", "resource_demand.1234.docs"}, common...)
	set = append(set, "--status", "unknown", "--dry-run")
	stdout, stderr = tempOutputFiles(t)
	if code := run(set, stdout, stderr); code != exitValidationFailed {
		t.Fatalf("invalid status exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}
	var env cliTestEnvelope
	if err := json.Unmarshal([]byte(readFileString(t, stdout.Name())), &env); err != nil {
		t.Fatal(err)
	}
	if firstDiagnostic(env.Errors, "invalid_demand_status") == nil {
		t.Fatalf("errors=%#v", env.Errors)
	}
}
