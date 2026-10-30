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
