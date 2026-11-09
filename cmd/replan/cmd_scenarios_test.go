package main

import (
	"os"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
)

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
