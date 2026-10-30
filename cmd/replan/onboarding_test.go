package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestInitDryRunApplyValidateAndProjectExists(t *testing.T) {
	project := filepath.Join(t.TempDir(), "My First Project")
	code, stdout, stderr := runOnboardingCLI(t, "init", project, "--name", "First", "--owner", "Ada Lovelace", "--timezone", "UTC", "--dry-run", "--format", "json")
	if code != exitSuccess || stderr != "" || !strings.Contains(stdout, `"canonical/tasks.yaml"`) {
		t.Fatalf("dry run exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if _, err := os.Stat(project); !os.IsNotExist(err) {
		t.Fatalf("dry run created project: %v", err)
	}

	code, stdout, stderr = runOnboardingCLI(t, "init", project, "--name", "First", "--owner", "Ada Lovelace", "--timezone", "UTC", "--apply", "--format", "json")
	if code != exitSuccess || stderr != "" {
		t.Fatalf("apply exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr = runOnboardingCLI(t, "validate", project, "--format", "json")
	if code != exitSuccess || stderr != "" {
		t.Fatalf("validate exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	var validated struct {
		Result app.ValidateResult `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &validated); err != nil || !validated.Result.Valid || len(validated.Result.Warnings) != 0 {
		t.Fatalf("validate result=%#v err=%v", validated, err)
	}
	data, err := os.ReadFile(filepath.Join(project, "canonical", "capacities.yaml"))
	if err != nil || string(data) != "capacities: []\n" {
		t.Fatalf("owner should have no initial capacity: %q, %v", data, err)
	}

	code, stdout, _ = runOnboardingCLI(t, "init", project, "--name", "Again", "--owner", "Ada", "--timezone", "UTC", "--apply", "--format", "json")
	if code != exitValidationFailed || !strings.Contains(stdout, `"project_exists"`) {
		t.Fatalf("second init exit=%d stdout=%s", code, stdout)
	}
}

func TestInitImportAcceptsDependenciesFromSameSnapshot(t *testing.T) {
	tests := []struct {
		name         string
		command      string
		input        string
		dependencyID string
	}{
		{
			name:    "openproject",
			command: "openproject",
			input: `{
  "_type": "Collection",
  "total": 2,
  "count": 2,
  "_embedded": {
    "elements": [
      {"id": 11, "subject": "Prepare release", "status": {"name": "In progress"}, "startDate": "2026-10-30", "dueDate": "2026-10-31", "_links": {"self": {"href": "/api/v3/work_packages/11"}}},
      {"id": 12, "subject": "Ship release", "status": {"name": "New"}, "startDate": "2026-11-01", "dueDate": "2026-11-02", "_links": {"self": {"href": "/api/v3/work_packages/12"}}}
    ],
    "relations": {"elements": [
      {"id": 91, "type": "blocks", "_links": {"self": {"href": "/api/v3/relations/91"}, "from": {"href": "/api/v3/work_packages/11"}, "to": {"href": "/api/v3/work_packages/12"}}}
    ]}
  }
}`,
			dependencyID: "dependency.openproject.relation.91",
		},
		{
			name:    "github",
			command: "github",
			input: `{
  "format": "replan.github-issues.v1",
  "repository": {"owner": "example-org", "name": "same-snapshot"},
  "issues": [
    {"id": 101, "number": 1, "title": "Prepare release", "state": "open", "html_url": "https://github.com/example-org/same-snapshot/issues/1"},
    {"id": 102, "number": 2, "title": "Ship release", "state": "open", "html_url": "https://github.com/example-org/same-snapshot/issues/2"}
  ],
  "blocked_by": [
    {"issue_number": 2, "issues": [{"repository": "example-org/same-snapshot", "number": 1, "id": 101}]}
  ]
}`,
			dependencyID: "dependency.github.blocked-by.example-org.same-snapshot.1.example-org.same-snapshot.2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			project := initOnboardingProject(t)
			inputPath := filepath.Join(t.TempDir(), tc.command+".json")
			if err := os.WriteFile(inputPath, []byte(tc.input), 0o600); err != nil {
				t.Fatal(err)
			}
			importArgs := []string{"import", tc.command, "--project", project, "--from-file", inputPath, "--include-relations", "--format", "json"}
			if tc.name == "openproject" {
				sidecarPath := filepath.Join(t.TempDir(), "resource-demands.yaml")
				sidecar := `openproject_resource_demands:
  - work_package_id: "11"
    demands:
      - id: resource_demand.same-snapshot.owner
        resource_ref:
          kind: resource
          id: resource.human.owner
        resource_class: human
        role: implementer
        unit: hours
        planned_amount: 1
        remaining_amount: 1
        criticality: required
        status: planned
`
				if err := os.WriteFile(sidecarPath, []byte(sidecar), 0o600); err != nil {
					t.Fatal(err)
				}
				importArgs = append(importArgs, "--resource-demands-sidecar", sidecarPath)
			}
			code, stdout, stderr := runOnboardingCLI(t, importArgs...)
			if code != exitSuccess || stderr != "" {
				t.Fatalf("import exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			snapshotID := onboardingImportSnapshotID(t, stdout)
			code, stdout, stderr = runOnboardingCLI(t, "import", "accept", snapshotID, "--project", project, "--actor", "actor.human.owner", "--apply", "--format", "json")
			if code != exitSuccess || stderr != "" {
				t.Fatalf("task accept exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			if tc.name == "openproject" {
				for _, mode := range []struct {
					flag string
					op   string
				}{
					{"--accept-dates", "update_task_planned_start"},
					{"--accept-tracking-status", "update_task_tracking_status"},
					{"--accept-resource-demands", "create_resource_demand"},
				} {
					code, stdout, stderr = runOnboardingCLI(t, "import", "accept", snapshotID, "--project", project, mode.flag, "--actor", "actor.human.owner", "--apply", "--format", "json")
					if code != exitSuccess || stderr != "" || !strings.Contains(stdout, `"op": "`+mode.op+`"`) {
						t.Fatalf("%s exit=%d stdout=%s stderr=%s", mode.flag, code, stdout, stderr)
					}
				}
			}
			code, stdout, stderr = runOnboardingCLI(t, "import", "accept", snapshotID, "--project", project, "--accept-dependencies", "--actor", "actor.human.owner", "--apply", "--format", "json")
			if code != exitSuccess || stderr != "" || !strings.Contains(stdout, `"op": "create_dependency"`) {
				t.Fatalf("dependency accept exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
			dependencies, err := os.ReadFile(filepath.Join(project, "canonical", "dependencies.yaml"))
			if err != nil || !strings.Contains(string(dependencies), "id: "+tc.dependencyID) {
				t.Fatalf("dependency %s not written: %v\n%s", tc.dependencyID, err, dependencies)
			}
			code, stdout, stderr = runOnboardingCLI(t, "validate", project, "--format", "json")
			if code != exitSuccess || stderr != "" || !strings.Contains(stdout, `"valid": true`) {
				t.Fatalf("validate exit=%d stdout=%s stderr=%s", code, stdout, stderr)
			}
		})
	}
}

func TestImportDependencyRematchNamesEndpointThatMustBeAccepted(t *testing.T) {
	project := initOnboardingProject(t)
	inputPath := filepath.Join(t.TempDir(), "partial-openproject.json")
	input := `{
  "_type": "Collection",
  "total": 1,
  "count": 1,
  "_embedded": {
    "elements": [
      {"id": 11, "subject": "Prepare release", "status": {"name": "New"}, "_links": {"self": {"href": "/api/v3/work_packages/11"}}}
    ],
    "relations": {"elements": [
      {"id": 92, "type": "blocks", "_links": {"self": {"href": "/api/v3/relations/92"}, "from": {"href": "/api/v3/work_packages/11"}, "to": {"href": "/api/v3/work_packages/12"}}}
    ]}
  }
}`
	if err := os.WriteFile(inputPath, []byte(input), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runOnboardingCLI(t, "import", "openproject", "--project", project, "--from-file", inputPath, "--include-relations", "--format", "json")
	if code != exitSuccess || stderr != "" {
		t.Fatalf("import exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	snapshotID := onboardingImportSnapshotID(t, stdout)
	code, stdout, stderr = runOnboardingCLI(t, "import", "accept", snapshotID, "--project", project, "--actor", "actor.human.owner", "--apply", "--format", "json")
	if code != exitSuccess || stderr != "" {
		t.Fatalf("task accept exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	code, stdout, stderr = runOnboardingCLI(t, "import", "accept", snapshotID, "--project", project, "--accept-dependencies", "--actor", "actor.human.owner", "--apply", "--format", "json")
	if code != exitSuccess || stderr != "" {
		t.Fatalf("dependency accept exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"code": "openproject_dependency_not_safe_to_accept"`) ||
		!strings.Contains(stdout, "successor work package 12") || !strings.Contains(stdout, "accept that task first") {
		t.Fatalf("unmatched endpoint warning is unclear: %s", stdout)
	}
	dependencies, err := os.ReadFile(filepath.Join(project, "canonical", "dependencies.yaml"))
	if err != nil || strings.Contains(string(dependencies), "dependency.openproject.relation.92") {
		t.Fatalf("unsafe dependency was written: %v\n%s", err, dependencies)
	}
}

func onboardingImportSnapshotID(t *testing.T, stdout string) string {
	t.Helper()
	var envelope struct {
		Result struct {
			Snapshot struct {
				ID string `json:"id"`
			} `json:"snapshot"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(stdout), &envelope); err != nil || envelope.Result.Snapshot.ID == "" {
		t.Fatalf("import result has no snapshot id: %v\n%s", err, stdout)
	}
	return envelope.Result.Snapshot.ID
}

func TestInitRequiresTimezoneWhenDetectionFails(t *testing.T) {
	old := detectMachineIANATimezone
	detectMachineIANATimezone = func() string { return "" }
	t.Cleanup(func() { detectMachineIANATimezone = old })
	code, stdout, _ := runOnboardingCLI(t, "init", filepath.Join(t.TempDir(), "project"), "--name", "First", "--owner", "Ada", "--dry-run", "--format", "json")
	if code != exitInvalidUsage || !strings.Contains(stdout, "--timezone") {
		t.Fatalf("exit=%d stdout=%s", code, stdout)
	}
}

func TestConcurrentInitOnlyOneSucceeds(t *testing.T) {
	project := filepath.Join(t.TempDir(), "concurrent")
	start := make(chan struct{})
	codes := make(chan int, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			code, _, _ := runOnboardingCLI(t, "init", project, "--name", "Concurrent", "--owner", "Owner", "--timezone", "UTC", "--apply", "--format", "json")
			codes <- code
		}()
	}
	close(start)
	wg.Wait()
	close(codes)
	counts := map[int]int{}
	for code := range codes {
		counts[code]++
	}
	if counts[exitSuccess] != 1 || counts[exitValidationFailed] != 1 {
		t.Fatalf("exit counts=%v", counts)
	}
}

func TestResourceClassesActorsDuplicatesAndCapacityPeriods(t *testing.T) {
	project := initOnboardingProject(t)
	for _, class := range app.ResourceClasses {
		id := "resource." + strings.ReplaceAll(class, "_", "-")
		code, stdout, stderr := runOnboardingCLI(t, "resource", "add", "--project", project, "--id", id, "--class", class, "--name", class, "--actor", "actor.human.owner", "--apply", "--format", "json")
		if code != exitSuccess {
			t.Fatalf("add %s exit=%d stdout=%s stderr=%s", class, code, stdout, stderr)
		}
	}
	code, stdout, _ := runOnboardingCLI(t, "resource", "add", "--project", project, "--id", "resource.ai.bot", "--class", "ai_agent", "--name", "Bot", "--with-actor", "limited", "--actor", "actor.human.owner", "--apply", "--format", "json")
	if code != exitSuccess || !strings.Contains(stdout, `"id": "actor.ai.bot"`) {
		t.Fatalf("with actor exit=%d stdout=%s", code, stdout)
	}
	code, stdout, _ = runOnboardingCLI(t, "resource", "add", "--project", project, "--id", "resource.ai.bot", "--class", "ai_agent", "--name", "Bot", "--actor", "actor.human.owner", "--dry-run", "--format", "json")
	if code != exitValidationFailed || !strings.Contains(stdout, "duplicate_resource_id") {
		t.Fatalf("duplicate exit=%d stdout=%s", code, stdout)
	}
	code, stdout, _ = runOnboardingCLI(t, "resource", "add", "--project", project, "--id", "resource.environment.bad-actor", "--class", "environment", "--name", "Bad", "--with-actor", "trusted", "--actor", "actor.human.owner", "--dry-run", "--format", "json")
	if code != exitValidationFailed || !strings.Contains(stdout, "actor_not_allowed_for_resource_class") {
		t.Fatalf("non-person actor exit=%d stdout=%s", code, stdout)
	}

	for _, args := range [][]string{
		{"capacity", "set", "--project", project, "--resource", "resource.human.owner", "--unit", "hours", "--amount", "8", "--effective-from", "2026-10-26", "--actor", "actor.human.owner", "--apply", "--format", "json"},
		{"capacity", "set", "--project", project, "--resource", "resource.human.owner", "--unit", "hours", "--amount", "4", "--effective-from", "2026-11-02", "--actor", "actor.human.owner", "--apply", "--format", "json"},
	} {
		code, stdout, stderr := runOnboardingCLI(t, args...)
		if code != exitSuccess {
			t.Fatalf("capacity exit=%d stdout=%s stderr=%s", code, stdout, stderr)
		}
	}
	data, err := os.ReadFile(filepath.Join(project, "canonical", "capacities.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Capacities []domain.Capacity `yaml:"capacities"`
	}
	if err := yaml.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if len(document.Capacities) != 2 || document.Capacities[0].EffectiveTo != "2026-11-01" {
		t.Fatalf("capacities=%#v", document.Capacities)
	}
	code, stdout, _ = runOnboardingCLI(t, "capacity", "set", "--project", project, "--resource", "resource.human.owner", "--unit", "hours", "--amount", "3", "--effective-from", "2026-10-30", "--actor", "actor.human.owner", "--dry-run", "--format", "json")
	if code != exitValidationFailed || !strings.Contains(stdout, "capacity_effective_from_not_after_latest") {
		t.Fatalf("earlier exit=%d stdout=%s", code, stdout)
	}
}

func TestOnboardingEndToEndWorkloadUsesCapacityAndOverloads(t *testing.T) {
	project := initOnboardingProject(t)
	commands := [][]string{
		{"resource", "add", "--project", project, "--id", "resource.ai.bot", "--class", "ai_agent", "--name", "Bot", "--with-actor", "limited", "--actor", "actor.human.owner", "--apply", "--format", "json"},
		{"capacity", "set", "--project", project, "--resource", "resource.human.owner", "--unit", "hours", "--amount", "2", "--effective-from", "2026-10-26", "--actor", "actor.human.owner", "--apply", "--format", "json"},
		{"capacity", "set", "--project", project, "--resource", "resource.ai.bot", "--unit", "agent_hours", "--amount", "2", "--effective-from", "2026-10-26", "--actor", "actor.human.owner", "--apply", "--format", "json"},
		{"evidence", "add", "--project", project, "--id", "evidence.first-plan", "--type", "planning_decision", "--actor", "actor.human.owner", "--observed-at", "2026-10-26T10:00:00Z", "--summary", "First plan", "--target", "project:project.e2e", "--apply", "--format", "json"},
		{"task", "add", "--project", project, "--id", "task.e2e.first", "--title", "First task", "--planned-start", "2026-10-26", "--planned-finish", "2026-11-01", "--demand", "owner=resource.human.owner:3:hours", "--demand", "bot=resource.ai.bot:3:agent_hours", "--actor", "actor.human.owner", "--evidence", "evidence.first-plan", "--apply", "--format", "json"},
	}
	for _, args := range commands {
		code, stdout, stderr := runOnboardingCLI(t, args...)
		if code != exitSuccess {
			t.Fatalf("%v exit=%d stdout=%s stderr=%s", args[:2], code, stdout, stderr)
		}
	}
	code, stdout, stderr := runOnboardingCLI(t, "workload", "--project", project, "--week", "2026-W44", "--format", "json")
	if code != exitSuccess || stderr != "" || strings.Count(stdout, `"overload": 1`) != 2 {
		t.Fatalf("workload exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
}

func TestOnboardingYAMLUsesCanonicalTwoSpaceSequenceIndentation(t *testing.T) {
	project := initOnboardingProject(t)
	assertNoFourSpaceSequence(t,
		filepath.Join(project, ".replan", "project.yaml"),
		filepath.Join(project, "canonical", "actors.yaml"),
		filepath.Join(project, "canonical", "resources.yaml"),
	)

	code, stdout, stderr := runOnboardingCLI(t, "resource", "add", "--project", project,
		"--id", "resource.ai.style", "--class", "ai_agent", "--name", "Style bot",
		"--capability", "formatting", "--with-actor", "limited",
		"--actor", "actor.human.owner", "--apply", "--format", "json")
	if code != exitSuccess {
		t.Fatalf("resource add exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertNoFourSpaceSequence(t,
		filepath.Join(project, "canonical", "actors.yaml"),
		filepath.Join(project, "canonical", "resources.yaml"),
	)

	code, stdout, stderr = runOnboardingCLI(t, "capacity", "set", "--project", project,
		"--resource", "resource.ai.style", "--unit", "agent_hours", "--amount", "8",
		"--effective-from", "2026-10-26", "--actor", "actor.human.owner",
		"--apply", "--format", "json")
	if code != exitSuccess {
		t.Fatalf("capacity set exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	assertNoFourSpaceSequence(t, filepath.Join(project, "canonical", "capacities.yaml"))
}

func assertNoFourSpaceSequence(t *testing.T, paths ...string) {
	t.Helper()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "    - ") {
				t.Fatalf("%s contains four-space sequence indentation: %q\n%s", path, line, data)
			}
		}
	}
}

func initOnboardingProject(t *testing.T) string {
	t.Helper()
	project := filepath.Join(t.TempDir(), "e2e")
	code, stdout, stderr := runOnboardingCLI(t, "init", project, "--name", "E2E", "--id", "project.e2e", "--owner", "Owner", "--timezone", "UTC", "--apply", "--format", "json")
	if code != exitSuccess {
		t.Fatalf("init exit=%d stdout=%s stderr=%s", code, stdout, stderr)
	}
	return project
}

func runOnboardingCLI(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	stdout, stderr := tempOutputFiles(t)
	code := run(args, stdout, stderr)
	out, errOut := readFileString(t, stdout.Name()), readFileString(t, stderr.Name())
	_ = stdout.Close()
	_ = stderr.Close()
	return code, out, errOut
}
