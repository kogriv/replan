package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestActualSetRemainingFromFileApplyAndRefuseInvalidBatch(t *testing.T) {
	project := copyCmdProjectFixture(t)
	batch := filepath.Join(t.TempDir(), "remaining.yaml")
	if err := os.WriteFile(batch, []byte("demands:\n  - demand: resource_demand.task-op-1234.implementer\n    amount: 0\n    unit: hours\n  - demand: resource_demand.task-op-1234.review\n    amount: 0\n    unit: review_hours\n    reason: Review complete\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args := []string{"actual", "set-remaining", "--project", project, "--from-file", batch, "--reason", "Finished", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--occurred-at", "2026-07-06T16:30:00+07:00", "--apply", "--format", "json"}
	stdout, stderr := tempOutputFiles(t)
	if code := run(args, stdout, stderr); code != exitSuccess {
		t.Fatalf("batch apply exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	var output struct {
		Result struct {
			Changes      []struct{ DemandID, Reason string } `json:"changes"`
			Audits       []struct{ ID string }               `json:"audit_record_previews"`
			WrittenFiles []string                            `json:"written_files"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(readFileString(t, stdout.Name())), &output); err != nil {
		t.Fatal(err)
	}
	if len(output.Result.Changes) != 2 || len(output.Result.Audits) != 2 || len(output.Result.WrittenFiles) != 2 {
		t.Fatalf("unexpected batch result: %#v", output.Result)
	}
	if output.Result.Changes[0].Reason != "Finished" || output.Result.Changes[1].Reason != "Review complete" {
		t.Fatalf("reason precedence: %#v", output.Result.Changes)
	}

	invalid := filepath.Join(t.TempDir(), "invalid.json")
	if err := os.WriteFile(invalid, []byte(`{"demands":[{"demand":"resource_demand.task-op-1234.implementer","amount":1,"unit":"days"},{"demand":"resource_demand.task-op-1234.implementer","amount":-1,"unit":"hours"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	before := readFileString(t, filepath.Join(project, "canonical", "resource_demands.yaml"))
	stdout, stderr = tempOutputFiles(t)
	args = []string{"actual", "set-remaining", "--project", project, "--from-file", invalid, "--reason", "Invalid", "--actor", "actor.human.operator", "--apply", "--format", "json"}
	if code := run(args, stdout, stderr); code != exitValidationFailed {
		t.Fatalf("invalid batch exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	got := readFileString(t, stdout.Name())
	if !strings.Contains(got, `row 1 demand \"resource_demand.task-op-1234.implementer\"`) || !strings.Contains(got, `row 2 demand \"resource_demand.task-op-1234.implementer\"`) {
		t.Fatalf("errors lack row and demand: %s", got)
	}
	if after := readFileString(t, filepath.Join(project, "canonical", "resource_demands.yaml")); after != before {
		t.Fatal("invalid batch wrote canonical state")
	}
}

func TestActualSetRemainingFromFileConflictsWithSingleFlags(t *testing.T) {
	batch := filepath.Join(t.TempDir(), "remaining.yaml")
	if err := os.WriteFile(batch, []byte("demands: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := tempOutputFiles(t)
	code := run([]string{"actual", "set-remaining", "--project", "../../examples/small-team", "--from-file", batch, "--demand", "resource_demand.task-op-1234.implementer", "--dry-run", "--format", "json"}, stdout, stderr)
	if code != exitInvalidUsage || !strings.Contains(readFileString(t, stdout.Name()), "actual_set_remaining_input_conflict") {
		t.Fatalf("conflict exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
}

func TestActualSetRemainingFromFileRequiresAmountAndReason(t *testing.T) {
	batch := filepath.Join(t.TempDir(), "remaining.yaml")
	if err := os.WriteFile(batch, []byte("demands:\n  - demand: resource_demand.task-op-1234.implementer\n    unit: hours\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr := tempOutputFiles(t)
	code := run([]string{"actual", "set-remaining", "--project", "../../examples/small-team", "--from-file", batch, "--actor", "actor.human.operator", "--dry-run", "--format", "json"}, stdout, stderr)
	got := readFileString(t, stdout.Name())
	if code != exitValidationFailed || !strings.Contains(got, `"code": "invalid_amount"`) || !strings.Contains(got, `"code": "missing_reason"`) || !strings.Contains(got, `row 1 demand \"resource_demand.task-op-1234.implementer\"`) {
		t.Fatalf("missing fields exit=%d stdout=%s stderr=%s", code, got, readFileString(t, stderr.Name()))
	}
}
