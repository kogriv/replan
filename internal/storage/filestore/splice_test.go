package filestore

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

// Hand-maintained files: two-space indentation, single and double quotes,
// comments, blank lines and keys in an order of their own. A command write
// must change only the bytes of what it changes (GAP-022).

const handTasks = `# Tasks kept by hand.
tasks:
  - id: task.a
    title: 'First task, with a comma'
    description: "Quoted with double quotes"   # line comment
    type: implementation
    tracking_status: not_started
    planned_start: 2026-10-01
    planned_finish: "2026-10-02"

  # The second task.
  - id: task.b
    title: Second task
    tracking_status: in_progress
    planned_start: "2026-10-03"
    planned_finish: "2026-10-04"
    tags: [one, two]
`

const handDemands = `resource_demands:
  - id: demand.a
    task_ref: {kind: task, id: task.a}
    resource_ref:
      kind: resource
      id: resource.x
    resource_class: human
    role: implementer
    unit: hours
    planned_amount: 2
    remaining_amount: 2
    status: active
  - id: demand.b
    task_ref:
      kind: task
      id: task.b
    resource_ref:
      kind: resource
      id: resource.x
    resource_class: human
    role: reviewer
    unit: hours
    planned_amount: 1
    remaining_amount: 1
    status: active # still open
`

const handActuals = `actuals:
  - id: actual.a
    type: work_consumed
    task_ref: {kind: task, id: task.a}
    amount: 1
    unit: 'hours'
    occurred_at: "2026-10-01T10:00:00+07:00"
    status_date: '2026-10-01'
    accepted_by: {kind: actor, id: actor.human.operator}
`

const handScenario = `scenario:
  id: scenario.hand
  # Why this scenario exists.
  name: 'Move task.a, written by hand'
  status: draft
  base_ref:
    kind: current_plan
    id: current
  changes:
    - op: replace
      path: tasks.task.a.planned_start
      value: "2026-10-05"
`

func writeHandFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readHandFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// expectOnly fails unless got equals want with exactly the replacements
// given applied once each.
func expectOnly(t *testing.T, got, original string, replacements ...string) {
	t.Helper()
	want := original
	for i := 0; i+1 < len(replacements); i += 2 {
		if strings.Count(want, replacements[i]) != 1 {
			t.Fatalf("test setup: %q is not unique in the original", replacements[i])
		}
		want = strings.Replace(want, replacements[i], replacements[i+1], 1)
	}
	if got != want {
		t.Fatalf("unexpected bytes\n--- got\n%s\n--- want\n%s", got, want)
	}
}

func TestSpliceTaskStatusChangesOneLine(t *testing.T) {
	root := t.TempDir()
	writeHandFile(t, root, tasksRel, handTasks)
	if _, err := New(root).SetTaskTrackingStatus("task.b", "review_ready"); err != nil {
		t.Fatal(err)
	}
	expectOnly(t, readHandFile(t, root, tasksRel), handTasks,
		"    tracking_status: in_progress\n", "    tracking_status: review_ready\n")
}

func TestSpliceDemandStatusKeepsOtherRecordsAndComments(t *testing.T) {
	root := t.TempDir()
	writeHandFile(t, root, demandsRel, handDemands)
	if _, err := New(root).SetResourceDemandRemaining("demand.a", 0); err != nil {
		t.Fatal(err)
	}
	expectOnly(t, readHandFile(t, root, demandsRel), handDemands,
		"    remaining_amount: 2\n", "    remaining_amount: 0\n")
}

func TestSpliceOverrunReviewAddsOnlyItsLines(t *testing.T) {
	root := t.TempDir()
	writeHandFile(t, root, demandsRel, handDemands)
	review := domain.OverrunReview{
		AcknowledgedForecast: 3,
		Unit:                 "hours",
		Reason:               "Real work",
		ReviewedBy:           domain.Ref{Kind: "actor", ID: "actor.human.operator"},
		ReviewedAt:           "2026-10-02T10:00:00+07:00",
	}
	if _, err := New(root).SetResourceDemandOverrunReview("demand.a", review); err != nil {
		t.Fatal(err)
	}
	got := readHandFile(t, root, demandsRel)
	if !strings.HasPrefix(got, strings.SplitAfter(handDemands, "    status: active\n")[0]) {
		t.Fatalf("lines before the new field changed:\n%s", got)
	}
	if !strings.HasSuffix(got, handDemands[strings.Index(handDemands, "  - id: demand.b"):]) {
		t.Fatalf("the other record changed:\n%s", got)
	}
	if !strings.Contains(got, "    overrun_review:\n") || !strings.Contains(got, "reason: Real work") {
		t.Fatalf("overrun review missing:\n%s", got)
	}
}

func TestSpliceAppendActualKeepsExistingBytes(t *testing.T) {
	root := t.TempDir()
	rel := actualsPath("2026-10-01")
	writeHandFile(t, root, rel, handActuals)
	actual := domain.Actual{
		ID:         "actual.b",
		Type:       "work_consumed",
		TaskRef:    domain.Ref{Kind: "task", ID: "task.b"},
		Amount:     2,
		Unit:       "hours",
		OccurredAt: "2026-10-01T11:00:00+07:00",
		StatusDate: "2026-10-01",
		AcceptedBy: domain.Ref{Kind: "actor", ID: "actor.human.operator"},
	}
	if _, err := New(root).AppendActual("2026-10-01", actual); err != nil {
		t.Fatal(err)
	}
	got := readHandFile(t, root, rel)
	if !strings.HasPrefix(got, handActuals) {
		t.Fatalf("existing record changed:\n%s", got)
	}
	added := strings.TrimPrefix(got, handActuals)
	if !strings.HasPrefix(added, "  - id: actual.b\n") || !strings.Contains(added, "    status_date: \"2026-10-01\"\n") {
		t.Fatalf("appended record not at the file's indentation:\n%s", added)
	}
}

func TestSpliceScenarioApplyChangesOnlyItsStatus(t *testing.T) {
	root := t.TempDir()
	writeHandFile(t, root, tasksRel, handTasks)
	writeHandFile(t, root, "scenarios/scenario.hand.yaml", handScenario)
	changes := []app.ScenarioApplyChange{{TargetKind: "task", TargetID: "task.a", Field: "planned_start", NewValue: "2026-10-05"}}
	audit := domain.AuditRecord{ID: "audit.hand", OccurredAt: "2026-10-02T10:00:00+07:00", Operation: "scenario_apply", Result: "success"}
	if _, err := New(root).ApplyScenarioChanges("scenario.hand", changes, audit); err != nil {
		t.Fatal(err)
	}
	expectOnly(t, readHandFile(t, root, "scenarios/scenario.hand.yaml"), handScenario,
		"  status: draft\n", "  status: applied\n")
	expectOnly(t, readHandFile(t, root, tasksRel), handTasks,
		"    planned_start: 2026-10-01\n", "    planned_start: \"2026-10-05\"\n")
}

func TestSpliceFallsBackToWholeFileForFlowLists(t *testing.T) {
	root := t.TempDir()
	flow := "tasks: [{id: task.a, title: A, tracking_status: not_started}]\n"
	writeHandFile(t, root, tasksRel, flow)
	if _, err := New(root).SetTaskTrackingStatus("task.a", "in_progress"); err != nil {
		t.Fatal(err)
	}
	got := readHandFile(t, root, tasksRel)
	if !strings.Contains(got, "tracking_status: in_progress") || strings.Contains(got, "[{") {
		t.Fatalf("flow list was not rendered as a whole file:\n%s", got)
	}
}

func TestRenderYAMLMatchesWholeFileForCanonicalExamples(t *testing.T) {
	// Every committed file renders to its own bytes when nothing changes.
	for _, rel := range []string{"canonical/tasks.yaml", "canonical/resource_demands.yaml", "canonical/dependencies.yaml"} {
		path := filepath.Join("..", "..", "..", "examples", "replan-self", rel)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var out []byte
		switch rel {
		case "canonical/tasks.yaml":
			var doc tasksDoc
			mustUnmarshal(t, data, &doc)
			out, err = renderYAML(data, doc)
		case "canonical/resource_demands.yaml":
			var doc demandsDoc
			mustUnmarshal(t, data, &doc)
			out, err = renderYAML(data, doc)
		default:
			var doc dependenciesDoc
			mustUnmarshal(t, data, &doc)
			out, err = renderYAML(data, doc)
		}
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != string(data) {
			t.Fatalf("%s changed although nothing was modified", rel)
		}
	}
}

func mustUnmarshal(t *testing.T, data []byte, target any) {
	t.Helper()
	if err := yaml.Unmarshal(data, target); err != nil {
		t.Fatal(err)
	}
}

func TestSpliceAppendEvidenceToHandAppendedDogfoodFile(t *testing.T) {
	// The self-dogfood evidence file of 2026-W42 was appended by hand; a
	// whole-file render rewrites about 700 of its lines.
	rel := "evidence/evidence.2026-W42.yaml"
	original, err := os.ReadFile(filepath.Join("..", "..", "..", "examples", "replan-self", rel))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeHandFile(t, root, rel, string(original))
	evidence := domain.Evidence{
		ID:         "evidence.splice.probe",
		Type:       "command_result",
		Source:     "local",
		ObservedAt: "2026-10-15T10:00:00+07:00",
		Summary:    "Probe",
	}
	if _, err := New(root).AppendEvidence(evidence.ObservedAt, evidence); err != nil {
		t.Fatal(err)
	}
	got := readHandFile(t, root, rel)
	if !strings.HasPrefix(got, string(original)) {
		t.Fatal("existing evidence records changed")
	}
	if !strings.HasPrefix(strings.TrimPrefix(got, string(original)), "  - id: evidence.splice.probe\n") {
		t.Fatalf("appended record:\n%s", strings.TrimPrefix(got, string(original)))
	}
}

func TestSpliceKeepsKeysTheDomainDoesNotModel(t *testing.T) {
	root := t.TempDir()
	withExtra := strings.Replace(handTasks, "    tags: [one, two]\n", "    tags: [one, two]\n    team_note: kept by hand\n", 1)
	writeHandFile(t, root, tasksRel, withExtra)
	if _, err := New(root).SetTaskTrackingStatus("task.b", "review_ready"); err != nil {
		t.Fatal(err)
	}
	expectOnly(t, readHandFile(t, root, tasksRel), withExtra,
		"    tracking_status: in_progress\n", "    tracking_status: review_ready\n")
}
