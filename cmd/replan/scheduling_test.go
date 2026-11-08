package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTaskSchedulingCLIFlagsDryRunApplyAuditAndRefusals(t *testing.T) {
	project := copyCmdProjectFixture(t)
	common := []string{"--project", project, "--task", "task.op.1234", "--reason", "Reviewed scheduling", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--occurred-at", "2026-11-08T10:00:00+07:00", "--format", "json"}
	before := readFileString(t, filepath.Join(project, "canonical/tasks.yaml"))
	args := append([]string{"task", "set-scheduling"}, common...)
	args = append(args, "--priority", "800", "--lead-time-working-days", "2.5", "--lead-time-reason", "review", "--dry-run")
	stdout, stderr := tempOutputFiles(t)
	if code := run(args, stdout, stderr); code != exitSuccess {
		t.Fatalf("dry run exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if got := readFileString(t, filepath.Join(project, "canonical/tasks.yaml")); got != before {
		t.Fatal("dry run wrote task file")
	}

	args[len(args)-1] = "--apply"
	stdout, stderr = tempOutputFiles(t)
	if code := run(args, stdout, stderr); code != exitSuccess {
		t.Fatalf("apply exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	written := readFileString(t, filepath.Join(project, "canonical/tasks.yaml"))
	for _, want := range []string{"lead_time:", "working_days: 2.5", "reason: review", "priority: 800"} {
		if !strings.Contains(written, want) {
			t.Errorf("tasks missing %q", want)
		}
	}
	audit := readFileString(t, filepath.Join(project, "audit/audit.2026-11.jsonl"))
	if !strings.Contains(audit, `"operation":"task_set_scheduling"`) || !strings.Contains(audit, "priority 0 -\\u003e 800") {
		t.Fatalf("audit missing old/new values: %s", audit)
	}
	stdout, stderr = tempOutputFiles(t)
	if code := run(args, stdout, stderr); code == exitSuccess || !strings.Contains(readFileString(t, stdout.Name()), "task_scheduling_unchanged") {
		t.Fatalf("repeating the same change exit=%d stdout=%s, want task_scheduling_unchanged", code, readFileString(t, stdout.Name()))
	}
	stdout, stderr = tempOutputFiles(t)
	clearArgs := append([]string{"task", "set-scheduling"}, common...)
	clearArgs = append(clearArgs, "--clear-priority", "--clear-lead-time", "--dry-run")
	if code := run(clearArgs, stdout, stderr); code != exitSuccess || !strings.Contains(readFileString(t, stdout.Name()), `"new_priority": 0`) || !strings.Contains(readFileString(t, stdout.Name()), `"new_lead_time": null`) {
		t.Fatalf("clear dry run exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}

	for _, extra := range [][]string{{"--priority", "0", "--dry-run"}, {"--priority", "700", "--clear-priority", "--dry-run"}, {"--lead-time-reason", "orphan", "--dry-run"}} {
		stdout, stderr = tempOutputFiles(t)
		if code := run(append(append([]string{"task", "set-scheduling"}, common...), extra...), stdout, stderr); code != exitValidationFailed {
			t.Fatalf("refusal %v exit=%d stdout=%s", extra, code, readFileString(t, stdout.Name()))
		}
	}
}

func TestTaskAddAndDemandLevelledCLIFlags(t *testing.T) {
	project := copyCmdProjectFixture(t)
	stdout, stderr := tempOutputFiles(t)
	addTask := []string{"task", "add", "--project", project, "--id", "task.scheduling", "--title", "Scheduling", "--priority", "750", "--lead-time-calendar-days", "3", "--lead-time-reason", "supplier", "--demand", "owner=resource.human.ivan:1:hours", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--format", "json"}
	if code := run(addTask, stdout, stderr); code != exitSuccess {
		t.Fatalf("task add exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}
	out := readFileString(t, stdout.Name())
	if !strings.Contains(out, `"priority": 750`) || !strings.Contains(out, `"calendar_days": 3`) {
		t.Fatalf("task flags missing: %s", out)
	}

	common := []string{"--project", project, "--reason", "Reviewed", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--format", "json"}
	add := append([]string{"demand", "add"}, common...)
	add = append(add, "--task", "task.op.1234", "--suffix", "unlevelled", "--resource", "resource.human.ivan", "--unit", "hours", "--amount", "1", "--levelled", "false", "--apply")
	stdout, stderr = tempOutputFiles(t)
	if code := run(add, stdout, stderr); code != exitSuccess {
		t.Fatalf("demand add exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}
	if !strings.Contains(readFileString(t, filepath.Join(project, "canonical/resource_demands.yaml")), "levelled: false") {
		t.Fatal("explicit false not persisted")
	}
	set := append([]string{"demand", "set", "resource_demand.1234.unlevelled"}, common...)
	set = append(set, "--levelled", "true", "--apply")
	stdout, stderr = tempOutputFiles(t)
	if code := run(set, stdout, stderr); code != exitSuccess {
		t.Fatalf("demand set exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}
	addBatch := filepath.Join(t.TempDir(), "add-levelled.yaml")
	if err := os.WriteFile(addBatch, []byte("demands:\n  - task: task.op.1234\n    suffix: batch-unlevelled\n    resource: resource.human.ivan\n    unit: hours\n    amount: 1\n    levelled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr = tempOutputFiles(t)
	addBatchArgs := append([]string{"demand", "add"}, common...)
	addBatchArgs = append(addBatchArgs, "--from-file", addBatch, "--dry-run")
	if code := run(addBatchArgs, stdout, stderr); code != exitSuccess || !strings.Contains(readFileString(t, stdout.Name()), `"levelled": false`) {
		t.Fatalf("add batch exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}

	batch := filepath.Join(t.TempDir(), "levelled.yaml")
	if err := os.WriteFile(batch, []byte("demands:\n  - id: resource_demand.1234.unlevelled\n    levelled: false\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout, stderr = tempOutputFiles(t)
	batchArgs := append([]string{"demand", "set"}, common...)
	batchArgs = append(batchArgs, "--from-file", batch, "--dry-run")
	if code := run(batchArgs, stdout, stderr); code != exitSuccess || !strings.Contains(readFileString(t, stdout.Name()), `"levelled": false`) {
		t.Fatalf("batch exit=%d stdout=%s", code, readFileString(t, stdout.Name()))
	}
}
