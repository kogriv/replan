package main

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

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
		{name: "apply", id: "scenario.task-op-1234-remaining-5", tail: []string{"--dry-run"}},
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
	for _, command := range []string{"show", "diff", "apply"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			args := []string{"scenario", command, "scenario.positional", "--scenario-id", "scenario.flag", "--format", "json"}
			if command == "apply" {
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
	for _, command := range []string{"show", "diff", "apply"} {
		t.Run(command, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			args := []string{"scenario", command, "--format", "json"}
			if command == "apply" {
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
	runApply := func(args ...string) map[string]any {
		t.Helper()
		stdout, stderr := tempOutputFiles(t)
		base := []string{"scenario", "apply", "--project", "../../examples/small-team", "--dry-run", "--format", "json"}
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
