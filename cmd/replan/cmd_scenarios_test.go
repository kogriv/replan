package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
)

func TestScenarioCommandsAcceptScenarioIDFlagAndPositional(t *testing.T) {
	tests := []struct {
		name string
		id   string
		tail []string
	}{
		{name: "show", id: "scenario.ivan-unavailable-week-28"},
		{name: "diff", id: "scenario.ivan-unavailable-week-28"},
		{name: "accept", id: "scenario.task-op-1234-remaining-5", tail: []string{"--dry-run"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			forms := []struct {
				name string
				args []string
			}{
				{name: "positional", args: []string{tc.id, "--project", "../../examples/small-team", "--format", "json"}},
				{name: "flag", args: []string{"--scenario-id", tc.id, "--project", "../../examples/small-team", "--format", "json"}},
				{name: "both equal", args: []string{tc.id, "--scenario-id", tc.id, "--project", "../../examples/small-team", "--format", "json"}},
				{name: "inline flag", args: []string{"--scenario-id=" + tc.id, "--project", "../../examples/small-team", "--format", "json"}},
				{name: "flag after other flags", args: []string{"--project", "../../examples/small-team", "--format", "json", "--scenario-id", tc.id}},
			}
			for _, form := range forms {
				t.Run(form.name, func(t *testing.T) {
					stdout, stderr := tempOutputFiles(t)
					args := append([]string{"scenario", tc.name}, form.args...)
					args = append(args, tc.tail...)
					if code := run(args, stdout, stderr); code != exitSuccess {
						t.Fatalf("%v exit = %d; stdout=%s stderr=%s", args, code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
					}
					if env := readCLIEnvelope(t, stdout.Name()); !env.OK {
						t.Fatalf("%v returned ok=false: %s", args, readFileString(t, stdout.Name()))
					}
				})
			}
		})
	}
}

func TestScenarioCommandsRejectConflictingScenarioIDs(t *testing.T) {
	for _, command := range []string{"show", "diff", "accept", "apply"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			args := []string{"scenario", command, "scenario.positional", "--scenario-id", "scenario.flag", "--format", "json"}
			if command == "apply" || command == "accept" {
				args = append(args, "--dry-run")
			}
			if code := run(args, stdout, stderr); code != exitInvalidUsage {
				t.Fatalf("%v exit = %d, want %d; stdout=%s stderr=%s", args, code, exitInvalidUsage, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if len(env.Errors) != 1 || env.Errors[0].Code != "conflicting_argument" || !strings.Contains(env.Errors[0].Message, `"scenario.positional"`) || !strings.Contains(env.Errors[0].Message, `"scenario.flag"`) {
				t.Fatalf("conflict envelope = %s", readFileString(t, stdout.Name()))
			}
		})
	}
}

func TestScenarioCommandsMissingIDNamesBothForms(t *testing.T) {
	for _, command := range []string{"show", "diff", "accept", "apply"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			args := []string{"scenario", command, "--format", "json"}
			if command == "apply" || command == "accept" {
				args = append(args, "--dry-run")
			}
			if code := run(args, stdout, stderr); code != exitInvalidUsage {
				t.Fatalf("%v exit = %d, want %d", args, code, exitInvalidUsage)
			}
			env := readCLIEnvelope(t, stdout.Name())
			want := "scenario " + command + " needs the scenario-id as the argument or with --scenario-id"
			if len(env.Errors) != 1 || env.Errors[0].Code != "missing_argument" || env.Errors[0].Message != want {
				t.Fatalf("missing envelope = %s", readFileString(t, stdout.Name()))
			}
		})
	}
}

func TestScenarioApplyFlagDryRunMatchesPositionalResult(t *testing.T) {
	project := copyCmdProjectFixture(t)
	approveCmdScenario(t, project)
	runApply := func(args ...string) map[string]any {
		t.Helper()
		stdout, stderr := tempOutputFiles(t)
		base := []string{"scenario", "apply", "--project", project, "--dry-run", "--format", "json"}
		if code := run(append(base, args...), stdout, stderr); code != exitSuccess {
			t.Fatalf("apply exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
		}
		env := readCLIEnvelope(t, stdout.Name())
		var result map[string]any
		if err := json.Unmarshal(env.Result, &result); err != nil {
			t.Fatal(err)
		}
		for _, unstable := range []string{"audit_record_preview", "planned_writes"} {
			delete(result, unstable)
		}
		return result
	}
	positional := runApply("scenario.task-op-1234-remaining-5")
	flagged := runApply("--scenario-id", "scenario.task-op-1234-remaining-5")
	if !reflect.DeepEqual(positional, flagged) {
		t.Fatalf("flag dry-run result differs from positional result:\npositional=%#v\nflag=%#v", positional, flagged)
	}
}

func TestScenarioDiffTaskJugglerCandidateRendersTaskChangesWithoutEmptyLoadTable(t *testing.T) {
	diff := app.ScenarioDiffResult{
		ScenarioID:   "scenario.taskjuggler.run-20261109",
		ScenarioName: "TaskJuggler schedule",
		Against:      "current",
		TaskChanges: []app.ScenarioTaskChange{
			{TaskID: "task.a", TaskTitle: "First task", Field: "planned_finish", CurrentValue: "2026-11-10", ScenarioValue: "2026-11-12"},
			{TaskID: "task.a", TaskTitle: "First task", Field: "planned_start", CurrentValue: "2026-11-09", ScenarioValue: "2026-11-11"},
		},
		WorkloadBefore: []app.ResourceWorkload{{ResourceID: "resource.agent", Capacity: 40, Overload: 0}},
		WorkloadAfter:  []app.ResourceWorkload{{ResourceID: "resource.agent", Capacity: 40, Overload: 0}},
	}

	markdown := renderScenarioDiffForTest(t, diff, writeScenarioDiffMarkdown)
	for _, want := range []string{"## Task Changes", "`task.a`", "First task", "`planned_start`", "2026-11-11", "No load changes."} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("Markdown output missing %q:\n%s", want, markdown)
		}
	}
	if strings.Contains(markdown, "| Resource | Unit |") {
		t.Fatalf("Markdown output contains an empty workload table:\n%s", markdown)
	}

	text := renderScenarioDiffForTest(t, diff, writeScenarioDiffText)
	for _, want := range []string{"TASK CHANGES", "TASK ID", "task.a", "First task", "planned_finish", "No load changes."} {
		if !strings.Contains(text, want) {
			t.Fatalf("Text output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "CAPACITY BEFORE") {
		t.Fatalf("Text output contains an empty workload table:\n%s", text)
	}
}

func renderScenarioDiffForTest(t *testing.T, diff app.ScenarioDiffResult, render func(*os.File, app.ScenarioDiffResult) error) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "scenario-diff-*")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := render(file, diff); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func approveCmdScenario(t *testing.T, project string) {
	t.Helper()
	stdout, stderr := tempOutputFiles(t)
	if code := run([]string{"scenario", "accept", "scenario.task-op-1234-remaining-5", "--actor", "actor.human.operator", "--project", project, "--apply"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("scenario accept exit=%d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
}

func TestScenarioLifecycleStatuses(t *testing.T) {
	for _, status := range []string{"draft", "calculated", "proposed", "accepted", "rejected", "applied", "archived", "reviewed", "unknown"} {
		t.Run(status, func(t *testing.T) {
			project := copyCmdProjectFixture(t)
			const id = "scenario.task-op-1234-remaining-5"
			path := filepath.Join(project, "scenarios", id+".yaml")
			original := readFileString(t, path)
			if err := os.WriteFile(path, []byte(strings.Replace(original, "status: draft", "status: "+status, 1)), 0o644); err != nil {
				t.Fatal(err)
			}
			before := snapshotProjectFiles(t, project)
			call := func(operation, mode, diagnostic string) {
				t.Helper()
				stdout, stderr := tempOutputFiles(t)
				exit := run([]string{"scenario", operation, id, "--project", project, "--actor", "actor.human.operator", "--format", "json", mode}, stdout, stderr)
				env := readCLIEnvelope(t, stdout.Name())
				if diagnostic == "" {
					if exit != exitSuccess || !env.OK {
						t.Fatalf("%s failed: %s", operation, readFileString(t, stdout.Name()))
					}
				} else if exit == exitSuccess || !hasDiagnosticCode(env.Errors, diagnostic) {
					t.Fatalf("%s expected %s: %s", operation, diagnostic, readFileString(t, stdout.Name()))
				}
			}
			if status == "applied" {
				stdout, stderr := tempOutputFiles(t)
				if exit := run([]string{"scenario", "show", id, "--project", project}, stdout, stderr); exit != exitSuccess {
					t.Fatalf("historical applied scenario did not load: %s", readFileString(t, stdout.Name()))
				}
			}
			applyDiagnostic := "scenario_not_accepted"
			if status == "accepted" {
				applyDiagnostic = ""
			} else if status == "applied" {
				applyDiagnostic = "scenario_already_applied"
			}
			call("apply", "--dry-run", applyDiagnostic)
			if applyDiagnostic != "" {
				call("apply", "--apply", applyDiagnostic)
			}
			if !reflect.DeepEqual(before, snapshotProjectFiles(t, project)) {
				t.Fatal("application preview or refusal changed project files")
			}

			switch status {
			case "draft", "calculated", "proposed":
				call("accept", "--dry-run", "")
				if !reflect.DeepEqual(before, snapshotProjectFiles(t, project)) {
					t.Fatal("approval preview changed project files")
				}
				call("accept", "--apply", "")
				if got := readFileString(t, path); got != strings.Replace(original, "status: draft", "status: accepted", 1) {
					t.Fatalf("approval should update only the scenario status: %s", got)
				}
				after := snapshotProjectFiles(t, project)
				for file, digest := range after {
					if file != "scenarios/"+id+".yaml" && !strings.HasPrefix(file, "audit/") && before[file] != digest {
						t.Fatalf("approval changed %s", file)
					}
				}
				call("accept", "--apply", "scenario_invalid_accept_status")
				if !reflect.DeepEqual(after, snapshotProjectFiles(t, project)) {
					t.Fatal("repeat approval changed project files")
				}
				call("apply", "--apply", "")
				if !strings.Contains(readFileString(t, path), "status: applied") {
					t.Fatal("scenario was not applied after approval")
				}
			default:
				call("accept", "--dry-run", "scenario_invalid_accept_status")
				call("accept", "--apply", "scenario_invalid_accept_status")
				if !reflect.DeepEqual(before, snapshotProjectFiles(t, project)) {
					t.Fatal("refused approval changed project files")
				}
			}
		})
	}
}

func TestScenarioAcceptCLIWriteControls(t *testing.T) {
	project := copyCmdProjectFixture(t)
	t.Setenv("REPLAN_APPLY", "1")
	t.Setenv("REPLAN_LOCK_TIMEOUT_SECONDS", "0.1")
	held, err := acquireProjectWriteLock(project, time.Second)
	if err != nil || held == nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer func() {
		if held != nil {
			held.close()
		}
	}()
	before := snapshotProjectFiles(t, project)
	base := []string{"scenario", "accept", "scenario.task-op-1234-remaining-5", "--project", project}
	for _, tt := range []struct {
		args    []string
		exit    int
		message string
	}{
		{[]string{"--actor", "actor.human.operator", "--reason", "--dry-run"}, exitConflict, projectLockedCode},
		{[]string{"--dry-run", "--format", "markdown"}, exitSuccess, "Would accept"},
		{[]string{"--dry-run", "--format", "text"}, exitSuccess, "Would accept"},
		{[]string{"--apply", "--bindings", "bindings.yaml"}, exitInvalidUsage, bindingsReadOnlyCode},
	} {
		stdout, stderr := tempOutputFiles(t)
		if got := run(append(base, tt.args...), stdout, stderr); got != tt.exit || !strings.Contains(readFileString(t, stdout.Name()), tt.message) {
			t.Fatalf("%v exit=%d: %s %s", tt.args, got, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
		}
	}
	if !reflect.DeepEqual(before, snapshotProjectFiles(t, project)) {
		t.Fatal("blocked or previewed approvals changed project files")
	}
	held.close()
	held = nil
	for _, tt := range []struct{ actor, diagnostic string }{{"", "actor_required"}, {"actor.unknown", "actor_not_found"}, {"actor.human.operator", ""}} {
		stdout, stderr := tempOutputFiles(t)
		args := base
		if tt.actor != "" {
			args = append(args, "--actor", tt.actor)
		}
		code := run(args, stdout, stderr)
		env := readCLIEnvelope(t, stdout.Name())
		if tt.diagnostic != "" {
			if code == exitSuccess || !hasDiagnosticCode(env.Errors, tt.diagnostic) || !reflect.DeepEqual(before, snapshotProjectFiles(t, project)) {
				t.Fatalf("invalid actor changed files or gave wrong diagnostic: %s", readFileString(t, stdout.Name()))
			}
		} else if code != exitSuccess || !env.OK {
			t.Fatalf("environment apply failed: %s", readFileString(t, stdout.Name()))
		}
	}
}
