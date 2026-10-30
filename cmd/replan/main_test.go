package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/buildinfo"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
)

type cliTestEnvelope struct {
	OK          bool             `json:"ok"`
	Command     string           `json:"command"`
	ProjectPath string           `json:"project_path"`
	Result      json.RawMessage  `json:"result"`
	Warnings    []app.Diagnostic `json:"warnings"`
	Errors      []app.Diagnostic `json:"errors"`
}

func TestVersionTextJSONAndGlobalFlag(t *testing.T) {
	oldVersion, oldCommit, oldDate := buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate
	t.Cleanup(func() { buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = oldVersion, oldCommit, oldDate })
	buildinfo.Version, buildinfo.Commit, buildinfo.BuildDate = "v0.1.0", "abc123456789", "2026-11-03T12:00:00Z"

	stdout, stderr := tempOutputFiles(t)
	if code := run([]string{"version", "--format", "text"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("version text exit = %d, stderr = %s", code, readFileString(t, stderr.Name()))
	}
	text := readFileString(t, stdout.Name())
	if !strings.Contains(text, "replan v0.1.0 (abc1234, 2026-11-03, ") || !strings.Contains(text, "project format version: 1\n") {
		t.Fatalf("version text = %q", text)
	}

	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"version", "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("version json exit = %d, stderr = %s", code, readFileString(t, stderr.Name()))
	}
	var env struct {
		OK       bool             `json:"ok"`
		Command  string           `json:"command"`
		Result   versionResult    `json:"result"`
		Warnings []app.Diagnostic `json:"warnings"`
		Errors   []app.Diagnostic `json:"errors"`
	}
	if err := json.Unmarshal([]byte(readFileString(t, stdout.Name())), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Command != "version" || env.Result.Version != "v0.1.0" || env.Result.ProjectFormatVersion != domain.CurrentProjectFormatVersion || env.Warnings == nil || env.Errors == nil {
		t.Fatalf("version envelope = %#v", env)
	}

	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"--version"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("--version exit = %d, stderr = %s", code, readFileString(t, stderr.Name()))
	}
	if got := readFileString(t, stdout.Name()); strings.Count(got, "\n") != 1 || !strings.HasPrefix(got, "replan v0.1.0 ") {
		t.Fatalf("--version output = %q", got)
	}
}

func TestVersionTextLines(t *testing.T) {
	tests := []struct {
		name string
		info buildinfo.Info
		want string
	}{
		{
			name: "go install",
			info: buildinfo.Info{Version: "v0.1.0", Commit: "unknown", BuildDate: "unknown", GoVersion: "go1.24.1", OSArch: "linux/amd64"},
			want: "replan v0.1.0 (go1.24.1 linux/amd64)",
		},
		{
			name: "release linker values",
			info: buildinfo.Info{Version: "v0.1.0", Commit: "abc123456789", BuildDate: "2026-10-29T12:00:00Z", GoVersion: "go1.24.1", OSArch: "linux/amd64"},
			want: "replan v0.1.0 (abc1234, 2026-10-29, go1.24.1 linux/amd64)",
		},
		{
			name: "plain devel build",
			info: buildinfo.Info{Version: "dev", Commit: "def567890123", BuildDate: "2026-10-28T12:00:00Z", GoVersion: "go1.24.1", OSArch: "linux/amd64"},
			want: "replan dev (def5678, 2026-10-28, go1.24.1 linux/amd64)",
		},
		{
			name: "unknown date",
			info: buildinfo.Info{Version: "v0.1.0", Commit: "abc123456789", BuildDate: "unknown", GoVersion: "go1.24.1", OSArch: "linux/amd64"},
			want: "replan v0.1.0 (abc1234, go1.24.1 linux/amd64)",
		},
		{
			name: "unknown commit",
			info: buildinfo.Info{Version: "v0.1.0", Commit: "unknown", BuildDate: "2026-10-29T12:00:00Z", GoVersion: "go1.24.1", OSArch: "linux/amd64"},
			want: "replan v0.1.0 (2026-10-29, go1.24.1 linux/amd64)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := versionTextLine(tt.info); got != tt.want {
				t.Fatalf("versionTextLine() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCLIHelpUsesStdoutAndSucceeds(t *testing.T) {
	paths, families := helpPathsFromUsage(t)
	for _, path := range append(paths, families...) {
		args := append(append([]string{}, path...), "--help")
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			defer stdout.Close()
			defer stderr.Close()
			if code := run(args, stdout, stderr); code != exitSuccess {
				t.Fatalf("run(%q) exit = %d, want 0", args, code)
			}
			if got := readFileString(t, stdout.Name()); strings.TrimSpace(got) == "" {
				t.Fatal("stdout is empty")
			}
			if got := readFileString(t, stderr.Name()); got != "" {
				t.Fatalf("stderr = %q, want empty", got)
			}
		})
	}
}

func TestCLIHelpAfterAnotherFlagUsesStdoutAndSucceeds(t *testing.T) {
	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()
	args := []string{"actual", "add-status", "--project", "project", "--help"}
	if code := run(args, stdout, stderr); code != exitSuccess {
		t.Fatalf("run(%q) exit = %d, want 0", args, code)
	}
	if got := readFileString(t, stdout.Name()); strings.TrimSpace(got) == "" {
		t.Fatal("stdout is empty")
	}
	if got := readFileString(t, stderr.Name()); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestBaselineCreateListAndShowTextOutput(t *testing.T) {
	projectPath := copyCmdProjectFixture(t)
	stdout, stderr := tempOutputFiles(t)
	args := []string{"baseline", "create", "--project", projectPath, "--id", "baseline.2026-07-08.cli", "--name", "CLI baseline", "--approved-by", "actor.human.operator", "--status-date", "2026-07-08", "--occurred-at", "2026-07-08T10:00:00+07:00", "--dry-run", "--format", "text"}
	if code := run(args, stdout, stderr); code != exitSuccess {
		t.Fatalf("dry run exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	output := readFileString(t, stdout.Name())
	if !strings.Contains(output, "Tasks: 2") || !strings.Contains(output, "Demands: 7") || !strings.Contains(output, "... (") {
		t.Fatalf("unexpected trimmed dry-run output:\n%s", output)
	}

	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"baseline", "list", "--project", projectPath, "--format", "text"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("list exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if output := readFileString(t, stdout.Name()); !strings.Contains(output, "baseline.2026-07-06.initial") || !strings.Contains(output, "unsealed") || !strings.Contains(output, "false") {
		t.Fatalf("unexpected list output:\n%s", output)
	}

	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"baseline", "show", "baseline.2026-07-06.initial", "--project", projectPath, "--format", "text"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("show exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if output := readFileString(t, stdout.Name()); !strings.Contains(output, "Estimates: `false`") || !strings.Contains(output, "Seal: `unsealed`") {
		t.Fatalf("unexpected show output:\n%s", output)
	}
}

func TestEstimateCalibrationCLIWorkflow(t *testing.T) {
	projectPath := copyCmdProjectFixtureFrom(t, "../../examples/estimate-calibration", "estimate-calibration")
	base := []string{"estimate", "calibrate", "--project", projectPath, "--baseline", "baseline.fixture.original", "--resource", "resource.ai.executor", "--resource", "resource.ai.codex", "--unit", "agent_hours", "--tag", "calibration", "--exclude", "task.cal.09=actuals_judged", "--id", "calibration.cli.agents", "--name", "CLI calibration", "--approved-by", "actor.human.operator", "--occurred-at", "2026-10-12T09:00:00Z", "--format", "json"}
	stdout, stderr := tempOutputFiles(t)
	if code := run(append(append([]string{}, base...), "--dry-run"), stdout, stderr); code != exitSuccess {
		t.Fatalf("calibrate dry-run exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if output := readFileString(t, stdout.Name()); !strings.Contains(output, `"count": 8`) || !strings.Contains(output, `"actuals_judged"`) {
		t.Fatalf("unexpected dry-run output: %s", output)
	}
	stdout, stderr = tempOutputFiles(t)
	if code := run(append(append([]string{}, base...), "--apply"), stdout, stderr); code != exitSuccess {
		t.Fatalf("calibrate apply exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	assertFileContains(t, filepath.Join(projectPath, "calibrations", "calibration.cli.agents.yaml"), "seal_sha256:")
	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"next", "--project", projectPath, "--format", "text"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("next exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if output := readFileString(t, stdout.Name()); !strings.Contains(output, "Apply calibration calibration.cli.agents to 1 open demands") {
		t.Fatalf("next omitted aggregate calibration action: %s", output)
	}
	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"next", "--project", projectPath, "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("next json exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if output := readFileString(t, stdout.Name()); !strings.Contains(output, `"demand_id": "demand.cal.open"`) || !strings.Contains(output, `"calibrated_value": 1.25`) || !strings.Contains(output, `--scenario-id \u003cscenario-id\u003e`) {
		t.Fatalf("next json omitted calibration details or command: %s", output)
	}

	for _, args := range [][]string{
		{"estimate", "list", "--project", projectPath, "--format", "json"},
		{"estimate", "show", "--project", projectPath, "calibration.cli.agents", "--format", "json"},
		{"estimate", "apply", "--project", projectPath, "--calibration", "calibration.cli.agents", "--scenario-id", "scenario.cli.calibrated", "--actor", "actor.human.operator", "--occurred-at", "2026-10-12T10:00:00Z", "--dry-run", "--format", "json"},
	} {
		stdout, stderr = tempOutputFiles(t)
		if code := run(args, stdout, stderr); code != exitSuccess {
			t.Fatalf("run(%v) exit=%d stdout=%s stderr=%s", args, code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
		}
	}
	stdout, stderr = tempOutputFiles(t)
	applyArgs := []string{"estimate", "apply", "--project", projectPath, "--calibration", "calibration.cli.agents", "--scenario-id", "scenario.cli.calibrated", "--actor", "actor.human.operator", "--occurred-at", "2026-10-12T10:00:00Z", "--apply", "--format", "json"}
	if code := run(applyArgs, stdout, stderr); code != exitSuccess {
		t.Fatalf("estimate apply exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	assertFileContains(t, filepath.Join(projectPath, "scenarios", "scenario.cli.calibrated.yaml"), "calibration_ref:")
	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"next", "--project", projectPath, "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("next after scenario exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if output := readFileString(t, stdout.Name()); strings.Contains(output, `"code": "apply_calibration"`) {
		t.Fatalf("next retained calibration action after scenario: %s", output)
	}
	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"scenario", "diff", "--project", projectPath, "scenario.cli.calibrated", "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("scenario diff exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
}

func TestEstimateCalibrationRejectsAgentApproverWithExitThree(t *testing.T) {
	stdout, stderr := tempOutputFiles(t)
	args := []string{"estimate", "calibrate", "--project", "../../examples/estimate-calibration", "--baseline", "baseline.fixture.original", "--resource", "resource.ai.executor", "--unit", "agent_hours", "--id", "calibration.bad", "--name", "Bad", "--approved-by", "actor.ai.executor", "--dry-run", "--format", "json"}
	if code := run(args, stdout, stderr); code != exitPolicyDenied {
		t.Fatalf("exit=%d want %d; stdout=%s stderr=%s", code, exitPolicyDenied, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if !strings.Contains(readFileString(t, stdout.Name()), `"approver_not_human"`) {
		t.Fatalf("stdout=%s", readFileString(t, stdout.Name()))
	}
}

func TestDocumentedImplementedFlagsExist(t *testing.T) {
	documented := documentedImplementedFlags(t, "../../docs/spec/cli.md")
	paths, _ := helpPathsFromUsage(t)
	for _, path := range paths {
		command := strings.Join(path, " ")
		flags, ok := documented[command]
		if !ok {
			t.Errorf("docs/spec/cli.md has no implemented command flags line for %q", command)
			continue
		}

		stdout, stderr := tempOutputFiles(t)
		args := append(append([]string{}, path...), "--help")
		if code := run(args, stdout, stderr); code != exitSuccess {
			stdout.Close()
			stderr.Close()
			t.Fatalf("run(%q) exit = %d, want 0", args, code)
		}
		helpFlags := flagsFromHelp(readFileString(t, stdout.Name()))
		stdout.Close()
		stderr.Close()
		for flagName := range flags {
			if !helpFlags[flagName] {
				t.Errorf("docs/spec/cli.md documents replan %s flag --%s as implemented, but command help does not define it", command, flagName)
			}
		}
		for flagName := range helpFlags {
			if !flags[flagName] {
				t.Errorf("replan %s help defines flag --%s, but docs/spec/cli.md does not list it in the implemented command flags block", command, flagName)
			}
		}
	}
}

func TestImportAcceptHelpIsTrackerNeutral(t *testing.T) {
	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()
	if code := run([]string{"import", "accept", "--help"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("import accept --help exit = %d, stderr = %s", code, readFileString(t, stderr.Name()))
	}
	help := readFileString(t, stdout.Name())
	for _, phrase := range []string{"tracker start and due dates", "mapped tracker statuses", "tracker dependency candidates", "tracker resource-demand candidates", "tracker resource-link candidates", "tracker time-entry candidates"} {
		if !strings.Contains(help, phrase) {
			t.Errorf("import accept --help does not contain %q: %s", phrase, help)
		}
	}
	for _, phrase := range []string{"OpenProject start and due dates", "mapped OpenProject statuses", "OpenProject dependency candidates", "OpenProject resource-demand candidates", "OpenProject resource-link candidates"} {
		if strings.Contains(help, phrase) {
			t.Errorf("import accept --help still contains source-specific phrase %q: %s", phrase, help)
		}
	}
}

func TestReportEstimatesJSONMarkdownAndRefusalExits(t *testing.T) {
	projectPath := "../../examples/replan-self"
	baselineID := "baseline.2026-10-15.tooling-consolidation"
	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()
	if code := run([]string{"report", "estimates", "--project", projectPath, "--baseline", baselineID, "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("json exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	var env cliTestEnvelope
	if err := json.Unmarshal([]byte(readFileString(t, stdout.Name())), &env); err != nil {
		t.Fatal(err)
	}
	var result app.EstimateReportResult
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !env.OK || result.BaselineSealStatus != "verified" || len(result.Tasks) != 38 {
		t.Fatalf("unexpected estimate envelope/result: %#v %#v", env, result)
	}
	var executor app.EstimateResourceSummary
	for _, row := range result.Resources {
		if row.ResourceID == "resource.ai.agent.executor" && row.Unit == "agent_hours" {
			executor = row
		}
	}
	if executor.Finished.BaselineAmount != 138 || executor.Finished.CurrentPlannedAmount != 68.8 || executor.Finished.ActualAmount != 28.04 {
		t.Fatalf("unexpected executor summary: %#v", executor)
	}
	if len(result.SplitGroups) != 1 || result.SplitGroups[0].Finished.BaselineAmount != 138 ||
		result.SplitGroups[0].Finished.CurrentPlannedAmount != 142 || result.SplitGroups[0].Finished.ActualAmount != 35.27 ||
		result.SplitGroups[0].Finished.ActualBaselineRatio == nil || *result.SplitGroups[0].Finished.ActualBaselineRatio != 0.25558 {
		t.Fatalf("unexpected split-group summary: %#v", result.SplitGroups)
	}
	for _, row := range result.Resources {
		if row.ResourceID == "resource.ci.github-actions" && row.ActualsRecorded {
			t.Fatalf("CI row unexpectedly reports actuals: %#v", row)
		}
	}

	markdownOut, markdownErr := tempOutputFiles(t)
	defer markdownOut.Close()
	defer markdownErr.Close()
	if code := run([]string{"report", "estimates", "--project", projectPath, "--baseline", baselineID}, markdownOut, markdownErr); code != exitSuccess {
		t.Fatalf("markdown exit=%d stdout=%s stderr=%s", code, readFileString(t, markdownOut.Name()), readFileString(t, markdownErr.Name()))
	}
	markdown := readFileString(t, markdownOut.Name())
	if !strings.Contains(markdown, "| Resource | Unit | Finished baseline") ||
		!strings.Contains(markdown, "| `resource.ci.github-actions` | ci_minutes | 380 | 380 | no actuals | no actuals") ||
		!strings.Contains(markdown, "| `resource.ai.agent.executor` | `resource.ai.codex.local` | agent_hours | 138 | 142 | 35.27 | 0.26") ||
		!strings.Contains(markdown, "## Give every OpenProject helper one JSON envelope") {
		t.Fatalf("unexpected markdown output:\n%s", markdown)
	}

	for _, test := range []struct {
		name, project, baseline, code string
		wantExit                      int
	}{
		{name: "unknown", project: projectPath, baseline: "baseline.missing", code: "baseline_not_found", wantExit: exitNotFound},
		{name: "ref only", project: "../../examples/small-team", baseline: "baseline.2026-07-06.initial", code: "baseline_has_no_estimates", wantExit: exitValidationFailed},
	} {
		t.Run(test.name, func(t *testing.T) {
			out, errOut := tempOutputFiles(t)
			defer out.Close()
			defer errOut.Close()
			if code := run([]string{"report", "estimates", "--project", test.project, "--baseline", test.baseline, "--format", "json"}, out, errOut); code != test.wantExit {
				t.Fatalf("exit=%d want=%d stdout=%s stderr=%s", code, test.wantExit, readFileString(t, out.Name()), readFileString(t, errOut.Name()))
			}
			var refused cliTestEnvelope
			if err := json.Unmarshal([]byte(readFileString(t, out.Name())), &refused); err != nil {
				t.Fatal(err)
			}
			if len(refused.Errors) != 1 || refused.Errors[0].Code != test.code {
				t.Fatalf("unexpected refusal: %#v", refused.Errors)
			}
		})
	}
}

// documentedImplementedFlags deliberately reads only the simple inventory
// blocks introduced by "Implemented command flags:". It does not interpret
// design examples or the wider, explicitly not-implemented contract.
func documentedImplementedFlags(t *testing.T, path string) map[string]map[string]bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	implementedPaths, _ := helpPathsFromUsage(t)
	known := make(map[string]bool, len(implementedPaths))
	for _, commandPath := range implementedPaths {
		known[strings.Join(commandPath, " ")] = true
	}

	result := map[string]map[string]bool{}
	lines := strings.Split(string(data), "\n")
	inInventory := false
	inFence := false
	for _, line := range lines {
		if line == "Implemented command flags:" {
			inInventory = true
			continue
		}
		if !inInventory {
			continue
		}
		if strings.HasPrefix(line, "```") {
			if !inFence {
				inFence = true
				continue
			}
			inInventory = false
			inFence = false
			continue
		}
		if !inFence || !strings.HasPrefix(line, "replan ") {
			continue
		}
		fields := strings.Fields(line)
		flagAt := len(fields)
		for i := 1; i < len(fields); i++ {
			if strings.HasPrefix(fields[i], "-") {
				flagAt = i
				break
			}
		}
		command := strings.Join(fields[1:flagAt], " ")
		if !known[command] {
			t.Errorf("docs/spec/cli.md implemented flag inventory names unknown command %q", command)
			continue
		}
		if result[command] == nil {
			result[command] = map[string]bool{}
		}
		for _, field := range fields[flagAt:] {
			if strings.HasPrefix(field, "--") {
				result[command][strings.TrimPrefix(field, "--")] = true
			} else if strings.HasPrefix(field, "-") {
				result[command][strings.TrimPrefix(field, "-")] = true
			}
		}
	}
	return result
}

func flagsFromHelp(help string) map[string]bool {
	flags := map[string]bool{}
	for _, line := range strings.Split(help, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "-") {
			continue
		}
		name := strings.TrimLeft(strings.Fields(line)[0], "-")
		flags[name] = true
	}
	return flags
}

func helpPathsFromUsage(t *testing.T) ([][]string, [][]string) {
	t.Helper()
	usage, stderr := tempOutputFiles(t)
	defer usage.Close()
	defer stderr.Close()
	printUsage(usage)
	lines := strings.Split(readFileString(t, usage.Name()), "\n")
	var paths [][]string
	familySet := map[string]bool{}
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[0] != "replan" {
			continue
		}
		if len(fields) < 3 || strings.HasPrefix(fields[2], "[") || strings.HasPrefix(fields[2], "--") {
			paths = append(paths, []string{fields[1]})
			continue
		}
		familySet[fields[1]] = true
		for _, command := range strings.Split(fields[2], "|") {
			paths = append(paths, []string{fields[1], command})
		}
	}
	var families [][]string
	for family := range familySet {
		families = append(families, []string{family})
	}
	sort.Slice(families, func(i, j int) bool { return families[i][0] < families[j][0] })
	return paths, families
}

func TestCLIFlagErrorStillUsesStderrAndExitEight(t *testing.T) {
	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()
	if code := run([]string{"workload", "--not-a-flag"}, stdout, stderr); code != exitInvalidUsage {
		t.Fatalf("exit = %d, want %d", code, exitInvalidUsage)
	}
	if got := readFileString(t, stdout.Name()); got != "" {
		t.Fatalf("stdout = %q, want empty", got)
	}
	if got := readFileString(t, stderr.Name()); !strings.Contains(got, "flag provided but not defined") {
		t.Fatalf("stderr = %q, want flag error", got)
	}
}

func TestWorkspaceLoadingCommandsReportMissingProject(t *testing.T) {
	runDir := t.TempDir()
	writeTaskJugglerImportFixture(t, runDir, `"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"
"task_op_1234";"Task";"2026-07-13";"2026-07-14";"8h";"4h";"resource_human_ivan";"in progress"
`)

	tests := []struct {
		name     string
		noFormat bool
		text     bool
		args     func(projectPath string) []string
	}{
		{name: "validate", text: true, args: func(path string) []string { return []string{"validate", path} }},
		{name: "workload", text: true, args: func(path string) []string { return []string{"workload", "--project", path} }},
		{name: "report weekly", text: true, args: func(path string) []string {
			return []string{"report", "weekly", "--project", path, "--week", "2026-W44"}
		}},
		{name: "report weekly write", text: true, args: func(path string) []string {
			return []string{"report", "weekly", "--project", path, "--week", "2026-W44", "--write"}
		}},
		{name: "report state", text: true, args: func(path string) []string { return []string{"report", "state", "--project", path} }},
		{name: "report dashboard", args: func(path string) []string { return []string{"report", "dashboard", "--project", path} }},
		{name: "report dashboard write", args: func(path string) []string {
			return []string{"report", "dashboard", "--project", path, "--output", filepath.Join(path, "reports", "dashboard")}
		}},
		{name: "next", text: true, args: func(path string) []string { return []string{"next", "--project", path} }},
		{name: "project set-status-date", args: func(path string) []string {
			return []string{"project", "set-status-date", "--project", path, "--date", "2026-10-27", "--actor", "actor.test", "--apply"}
		}},
		{name: "project migrate", args: func(path string) []string {
			return []string{"project", "migrate", "--project", path, "--actor", "actor.test", "--apply"}
		}},
		{name: "tracking suggest", text: true, args: func(path string) []string { return []string{"tracking", "suggest", "--project", path} }},
		{name: "tracking suggest write", text: true, args: func(path string) []string {
			return []string{"tracking", "suggest", "--project", path, "--write-candidates", "--actor", "actor.test"}
		}},
		{name: "scenario list", args: func(path string) []string { return []string{"scenario", "list", "--project", path} }},
		{name: "scenario show", args: func(path string) []string { return []string{"scenario", "show", "scenario.test", "--project", path} }},
		{name: "scenario diff", args: func(path string) []string { return []string{"scenario", "diff", "scenario.test", "--project", path} }},
		{name: "scenario apply", args: func(path string) []string {
			return []string{"scenario", "apply", "scenario.test", "--project", path, "--actor", "actor.test", "--apply"}
		}},
		{name: "proposal list", args: func(path string) []string { return []string{"proposal", "list", "--project", path} }},
		{name: "proposal show", args: func(path string) []string { return []string{"proposal", "show", "proposal.test", "--project", path} }},
		{name: "proposal suggest-dependencies", args: func(path string) []string { return []string{"proposal", "suggest-dependencies", "--project", path} }},
		{name: "proposal accept", args: func(path string) []string {
			return []string{"proposal", "accept", "proposal.test", "--project", path, "--actor", "actor.test", "--apply"}
		}},
		{name: "proposal reject", args: func(path string) []string {
			return []string{"proposal", "reject", "proposal.test", "--project", path, "--actor", "actor.test", "--apply"}
		}},
		{name: "dependency list", args: func(path string) []string { return []string{"dependency", "list", "--project", path} }},
		{name: "audit list", args: func(path string) []string { return []string{"audit", "list", "--project", path} }},
		{name: "audit verify", args: func(path string) []string { return []string{"audit", "verify", "--project", path} }},
		{name: "actual list", args: func(path string) []string { return []string{"actual", "list", "--project", path} }},
		{name: "actual add-consumption", args: func(path string) []string {
			return []string{"actual", "add-consumption", "--project", path, "--task", "task.test", "--demand", "demand.test", "--resource", "resource.test", "--amount", "1", "--unit", "hours", "--evidence", "evidence.test", "--actor", "actor.test", "--occurred-at", "2026-10-27T12:00:00Z", "--status-date", "2026-10-27", "--apply"}
		}},
		{name: "actual set-remaining", args: func(path string) []string {
			return []string{"actual", "set-remaining", "--project", path, "--demand", "demand.test", "--amount", "1", "--unit", "hours", "--reason", "test", "--evidence", "evidence.test", "--actor", "actor.test", "--occurred-at", "2026-10-27T12:00:00Z", "--status-date", "2026-10-27", "--apply"}
		}},
		{name: "actual close-demand", args: func(path string) []string {
			return []string{"actual", "close-demand", "--project", path, "--demand", "demand.test", "--status", "consumed", "--reason", "test", "--evidence", "evidence.test", "--actor", "actor.test", "--occurred-at", "2026-10-27T12:00:00Z", "--status-date", "2026-10-27", "--apply"}
		}},
		{name: "actual reopen-demand", args: func(path string) []string {
			return []string{"actual", "reopen-demand", "--project", path, "--demand", "demand.test", "--reason", "test", "--evidence", "evidence.test", "--actor", "actor.test", "--occurred-at", "2026-10-27T12:00:00Z", "--apply"}
		}},
		{name: "actual acknowledge-overrun", args: func(path string) []string {
			return []string{"actual", "acknowledge-overrun", "--project", path, "--demand", "demand.test", "--reason", "test", "--evidence", "evidence.test", "--actor", "actor.test", "--occurred-at", "2026-10-27T12:00:00Z", "--apply"}
		}},
		{name: "actual add-status", args: func(path string) []string {
			return []string{"actual", "add-status", "--project", path, "--task", "task.test", "--status", "in_progress", "--reason", "test", "--evidence", "evidence.test", "--actor", "actor.test", "--occurred-at", "2026-10-27T12:00:00Z", "--status-date", "2026-10-27", "--apply"}
		}},
		{name: "evidence add", text: true, args: func(path string) []string {
			return []string{"evidence", "add", "--project", path, "--id", "evidence.test", "--type", "observation", "--actor", "actor.test", "--observed-at", "2026-10-27T12:00:00Z", "--summary", "test", "--target", "task:task.test", "--apply"}
		}},
		{name: "task add", text: true, args: func(path string) []string {
			return []string{"task", "add", "--project", path, "--id", "task.test", "--title", "Test", "--demand", "implementer=resource.test:1:hours", "--actor", "actor.test", "--evidence", "evidence.test", "--apply"}
		}},
		{name: "task set-dates", text: true, args: func(path string) []string {
			return []string{"task", "set-dates", "--project", path, "--task", "task.test", "--planned-start", "2026-10-27", "--reason", "test", "--actor", "actor.test", "--evidence", "evidence.test", "--apply"}
		}},
		{name: "task set-title", text: true, args: func(path string) []string {
			return []string{"task", "set-title", "--project", path, "--task", "task.test", "--title", "Test", "--reason", "test", "--actor", "actor.test", "--evidence", "evidence.test", "--apply"}
		}},
		{name: "import openproject", args: func(path string) []string {
			return []string{"import", "openproject", "--project", path, "--from-file", "../../examples/imports/openproject/work-packages.sample.json"}
		}},
		{name: "import accept", args: func(path string) []string {
			return []string{"import", "accept", "source_snapshot.openproject.test", "--project", path, "--accept-dates", "--actor", "actor.test", "--apply"}
		}},
		{name: "export json", args: func(path string) []string { return []string{"export", "json", "--project", path} }},
		{name: "export taskjuggler", text: true, args: func(path string) []string { return []string{"export", "taskjuggler", "--project", path} }},
		{name: "export csv", noFormat: true, args: func(path string) []string {
			return []string{"export", "csv", "--project", path, "--report", "workload", "--week", "2026-W44"}
		}},
		{name: "taskjuggler run", text: true, args: func(path string) []string { return []string{"taskjuggler", "run", "--project", path} }},
		{name: "taskjuggler import-result", text: true, args: func(path string) []string {
			return []string{"taskjuggler", "import-result", "--project", path, "--run-dir", runDir, "--write-candidate", "--actor", "actor.test"}
		}},
		{name: "dev validate-taskjuggler", text: true, args: func(path string) []string { return []string{"dev", "validate-taskjuggler", "--project", path} }},
	}

	for _, test := range tests {
		formats := []string{"json"}
		if test.text {
			formats = append(formats, "text")
		}
		for _, format := range formats {
			t.Run(test.name+" "+format, func(t *testing.T) {
				projectPath := filepath.Join(t.TempDir(), "missing-project")
				args := test.args(projectPath)
				if !test.noFormat {
					args = append(args, "--format", format)
				}
				stdout, stderr := tempOutputFiles(t)
				defer stdout.Close()
				defer stderr.Close()

				if code := run(args, stdout, stderr); code != exitNotFound {
					t.Fatalf("run(%q) exit = %d, want %d; stderr=%s stdout=%s", args, code, exitNotFound, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
				}
				if format == "json" {
					env := readCLIEnvelope(t, stdout.Name())
					if env.OK {
						t.Fatalf("ok = true, want false; envelope=%s", readFileString(t, stdout.Name()))
					}
					if len(env.Errors) != 1 || env.Errors[0].Code != "project_not_found" {
						t.Fatalf("errors = %#v, want one project_not_found diagnostic; envelope=%s", env.Errors, readFileString(t, stdout.Name()))
					}
				} else if output := readFileString(t, stdout.Name()); !strings.Contains(output, "project_not_found") {
					t.Fatalf("text output does not contain project_not_found: %s", output)
				}
				if _, err := os.Stat(projectPath); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("missing project path was created or stat failed unexpectedly: %v", err)
				}
			})
		}
	}
}

func TestTooNewProjectFormatIsOneValidationError(t *testing.T) {
	projectPath := copyCmdProjectFixture(t)
	manifestPath := filepath.Join(projectPath, ".replan", "project.yaml")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "  format_version: 1\n", "  format_version: 2\n", 1))
	if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	assertUnsupported := func(t *testing.T, args []string) {
		t.Helper()
		stdout, stderr := tempOutputFiles(t)
		defer stdout.Close()
		defer stderr.Close()
		if code := run(args, stdout, stderr); code != exitValidationFailed {
			t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitValidationFailed, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
		}
		env := readCLIEnvelope(t, stdout.Name())
		if env.OK || len(env.Errors) != 1 || env.Errors[0].Code != "project_format_unsupported" {
			t.Fatalf("envelope = %s, want one project_format_unsupported", readFileString(t, stdout.Name()))
		}
	}
	tests := []struct {
		name string
		args []string
	}{
		{name: "validate", args: []string{"validate", projectPath, "--format", "json"}},
		{name: "workload", args: []string{"workload", "--project", projectPath, "--week", "2026-W28", "--format", "json"}},
		{name: "report weekly", args: []string{"report", "weekly", "--project", projectPath, "--week", "2026-W28", "--format", "json"}},
		// Mutations check references that an unread project cannot resolve;
		// the format still decides the exit code.
		{name: "task set-dates", args: []string{"task", "set-dates", "--project", projectPath, "--task", "task.op.1234", "--planned-start", "2026-07-06", "--reason", "Format check.", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--format", "json"}},
		{name: "actual add-status", args: []string{"actual", "add-status", "--project", projectPath, "--task", "task.op.1234", "--status", "in_progress", "--reason", "Format check.", "--actor", "actor.human.operator", "--dry-run", "--format", "json"}},
		{name: "evidence add", args: []string{"evidence", "add", "--project", projectPath, "--id", "evidence.format-check", "--type", "decision", "--actor", "actor.human.operator", "--observed-at", "2026-07-06T10:00:00+07:00", "--summary", "Format check.", "--target", "task:task.op.1234", "--dry-run", "--format", "json"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertUnsupported(t, test.args)
		})
	}

	t.Run("dashboard output writes nothing", func(t *testing.T) {
		outputDir := filepath.Join(projectPath, "unsupported-dashboard")
		assertUnsupported(t, []string{"report", "dashboard", "--project", projectPath, "--output", outputDir, "--format", "json"})
		if _, err := os.Stat(outputDir); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("dashboard output exists or stat failed unexpectedly: %v", err)
		}
	})

	t.Run("weekly write writes nothing", func(t *testing.T) {
		outputPath := filepath.Join(projectPath, "reports", "weekly", "2026-W28.json")
		assertUnsupported(t, []string{"report", "weekly", "--project", projectPath, "--week", "2026-W28", "--write", "--format", "json"})
		if _, err := os.Stat(outputPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("weekly report exists or stat failed unexpectedly: %v", err)
		}
	})
}

func TestArgumentErrorsPrecedeMissingProject(t *testing.T) {
	tests := []struct {
		name string
		args func(projectPath string) []string
	}{
		{
			name: "malformed week",
			args: func(path string) []string {
				return []string{"workload", "--project", path, "--week", "not-a-week", "--format", "json"}
			},
		},
		{
			name: "missing required id",
			args: func(path string) []string {
				return []string{"proposal", "show", "--project", path, "--format", "json"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectPath := filepath.Join(t.TempDir(), "missing-project")
			stdout, stderr := tempOutputFiles(t)
			defer stdout.Close()
			defer stderr.Close()
			if code := run(test.args(projectPath), stdout, stderr); code != exitInvalidUsage {
				t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitInvalidUsage, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
			}
			if _, err := os.Stat(projectPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("missing project path was created or stat failed unexpectedly: %v", err)
			}
		})
	}
}

func TestReportWritesRemainAvailableForInvalidPresentProject(t *testing.T) {
	tests := []struct {
		name   string
		run    func(t *testing.T, projectPath string, stdout, stderr *os.File) int
		assert func(t *testing.T, projectPath string)
	}{
		{
			name: "dashboard output",
			run: func(t *testing.T, projectPath string, stdout, stderr *os.File) int {
				t.Helper()
				outputDir := filepath.Join(projectPath, "dashboard-output")
				return run([]string{"report", "dashboard", "--project", projectPath, "--output", outputDir, "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, projectPath string) {
				t.Helper()
				for _, rel := range []string{"dashboard-output/index.html", "dashboard-output/raw/overview.json", "dashboard-output/raw/manifest.json"} {
					if _, err := os.Stat(filepath.Join(projectPath, filepath.FromSlash(rel))); err != nil {
						t.Fatalf("expected dashboard artifact %s: %v", rel, err)
					}
				}
			},
		},
		{
			name: "weekly write",
			run: func(t *testing.T, projectPath string, stdout, stderr *os.File) int {
				t.Helper()
				return run([]string{"report", "weekly", "--project", projectPath, "--week", "2026-W28", "--write", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, projectPath string) {
				t.Helper()
				if _, err := os.Stat(filepath.Join(projectPath, "reports", "weekly", "2026-W28.json")); err != nil {
					t.Fatalf("expected weekly report artifact: %v", err)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projectPath := copyCmdProjectFixture(t)
			if err := os.Remove(filepath.Join(projectPath, "canonical", "availability.yaml")); err != nil {
				t.Fatalf("remove availability fixture: %v", err)
			}
			stdout, stderr := tempOutputFiles(t)
			defer stdout.Close()
			defer stderr.Close()
			if code := test.run(t, projectPath, stdout, stderr); code != exitValidationFailed {
				t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitValidationFailed, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if !hasDiagnosticCode(env.Errors, "missing_required_file") {
				t.Fatalf("errors = %#v, want missing_required_file", env.Errors)
			}
			test.assert(t, projectPath)
		})
	}
}

func TestScenarioNotFoundUsesExitFourAcrossCommands(t *testing.T) {
	projectPath := "../../examples/small-team"
	tests := []struct {
		name string
		args []string
	}{
		{name: "workload", args: []string{"workload", "--project", projectPath, "--scenario", "scenario.unknown", "--format", "json"}},
		{name: "report weekly", args: []string{"report", "weekly", "--project", projectPath, "--week", "2026-W28", "--scenario", "scenario.unknown", "--format", "json"}},
		{name: "report dashboard", args: []string{"report", "dashboard", "--project", projectPath, "--scenario", "scenario.unknown", "--format", "json"}},
		{name: "taskjuggler run", args: []string{"taskjuggler", "run", "--project", projectPath, "--scenario", "scenario.unknown", "--format", "json"}},
		{name: "export taskjuggler", args: []string{"export", "taskjuggler", "--project", projectPath, "--scenario", "scenario.unknown", "--format", "json"}},
		{name: "export csv", args: []string{"export", "csv", "--project", projectPath, "--report", "workload", "--week", "2026-W28", "--scenario", "scenario.unknown"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			defer stdout.Close()
			defer stderr.Close()
			if code := run(test.args, stdout, stderr); code != exitNotFound {
				t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitNotFound, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if !hasDiagnosticCode(env.Errors, "scenario_not_found") {
				t.Fatalf("errors = %#v, want scenario_not_found; envelope=%s", env.Errors, readFileString(t, stdout.Name()))
			}
		})
	}
}

func TestImportAcceptSnapshotReadFailures(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		stdout, stderr := tempOutputFiles(t)
		defer stdout.Close()
		defer stderr.Close()
		args := []string{"import", "accept", "unknown-snapshot-id", "--project", "../../examples/small-team", "--dry-run", "--format", "json"}
		if code := run(args, stdout, stderr); code != exitNotFound {
			t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitNotFound, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
		}
		env := readCLIEnvelope(t, stdout.Name())
		if !hasDiagnosticCode(env.Errors, "source_snapshot_not_found") {
			t.Fatalf("errors = %#v, want source_snapshot_not_found", env.Errors)
		}
	})

	t.Run("existing but unreadable", func(t *testing.T) {
		projectPath := copyCmdProjectFixture(t)
		mappedPath := filepath.Join(projectPath, "imports", "openproject", "unreadable", "mapped.json")
		if err := os.MkdirAll(mappedPath, 0o755); err != nil {
			t.Fatalf("create unreadable mapped snapshot: %v", err)
		}
		stdout, stderr := tempOutputFiles(t)
		defer stdout.Close()
		defer stderr.Close()
		args := []string{"import", "accept", "source_snapshot.openproject.unreadable", "--project", projectPath, "--dry-run", "--format", "json"}
		if code := run(args, stdout, stderr); code != exitValidationFailed {
			t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitValidationFailed, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
		}
		env := readCLIEnvelope(t, stdout.Name())
		if !hasDiagnosticCode(env.Errors, "import_snapshot_read_error") {
			t.Fatalf("errors = %#v, want import_snapshot_read_error", env.Errors)
		}
	})

	t.Run("existing but invalid", func(t *testing.T) {
		projectPath := copyCmdProjectFixture(t)
		mappedPath := filepath.Join(projectPath, "imports", "openproject", "invalid", "mapped.json")
		if err := os.MkdirAll(filepath.Dir(mappedPath), 0o755); err != nil {
			t.Fatalf("create invalid snapshot directory: %v", err)
		}
		if err := os.WriteFile(mappedPath, []byte("not json\n"), 0o600); err != nil {
			t.Fatalf("write invalid mapped snapshot: %v", err)
		}
		stdout, stderr := tempOutputFiles(t)
		defer stdout.Close()
		defer stderr.Close()
		args := []string{"import", "accept", "source_snapshot.openproject.invalid", "--project", projectPath, "--dry-run", "--format", "json"}
		if code := run(args, stdout, stderr); code != exitValidationFailed {
			t.Fatalf("exit = %d, want %d; stderr=%s stdout=%s", code, exitValidationFailed, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
		}
		env := readCLIEnvelope(t, stdout.Name())
		if !hasDiagnosticCode(env.Errors, "import_snapshot_parse_error") {
			t.Fatalf("errors = %#v, want import_snapshot_parse_error", env.Errors)
		}
	})
}

func TestBindingsLayerUsageAndPrecedence(t *testing.T) {
	project := copyCmdProjectFixture(t)
	settingBindings := writeCLIBindings(t, "task.op.1234", "91")
	flagBindings := writeCLIBindings(t, "task.op.1234", "92")

	t.Run("missing and malformed are usage errors", func(t *testing.T) {
		malformed := filepath.Join(t.TempDir(), "malformed.yaml")
		if err := os.WriteFile(malformed, []byte("openproject_private_bindings: [unterminated\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, test := range []struct{ path, code string }{
			{filepath.Join(t.TempDir(), "missing.yaml"), "bindings_layer_missing"},
			{malformed, "bindings_layer_invalid"},
		} {
			stdout, stderr := tempOutputFiles(t)
			if code := run([]string{"validate", project, "--bindings", test.path, "--format", "json"}, stdout, stderr); code != exitInvalidUsage {
				t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, exitInvalidUsage, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			if env := readCLIEnvelope(t, stdout.Name()); !hasDiagnosticCode(env.Errors, test.code) {
				t.Fatalf("errors = %#v, want %s", env.Errors, test.code)
			}
		}
	})

	t.Run("flag wins over setting", func(t *testing.T) {
		t.Setenv("REPLAN_OPENPROJECT_BINDINGS_LAYER", settingBindings)
		stdout, stderr := tempOutputFiles(t)
		if code := run([]string{"export", "json", "--project", project, "--bindings", flagBindings, "--format", "json"}, stdout, stderr); code != exitSuccess {
			t.Fatalf("exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
		}
		got := readFileString(t, stdout.Name())
		if !strings.Contains(got, `"object_id": "92"`) || strings.Contains(got, `"object_id": "91"`) {
			t.Fatalf("output did not use flag binding: %s", got)
		}
	})

	t.Run("helper setting alone has no effect", func(t *testing.T) {
		t.Setenv("REPLAN_OPENPROJECT_BINDINGS_LAYER", "")
		baselineOut, baselineErr := tempOutputFiles(t)
		if code := run([]string{"export", "json", "--project", project, "--format", "json"}, baselineOut, baselineErr); code != exitSuccess {
			t.Fatal(code)
		}
		t.Setenv("REPLAN_OPENPROJECT_BINDINGS_PATH", flagBindings)
		actualOut, actualErr := tempOutputFiles(t)
		if code := run([]string{"export", "json", "--project", project, "--format", "json"}, actualOut, actualErr); code != exitSuccess {
			t.Fatal(code)
		}
		// The two exports may fall into different seconds; the export id and
		// time carry the second.
		exportClock := regexp.MustCompile(`"(exported_at|id)": "(export\.replan\.)?[0-9T:+-]*"`)
		got := exportClock.ReplaceAllString(readFileString(t, actualOut.Name()), "")
		want := exportClock.ReplaceAllString(readFileString(t, baselineOut.Name()), "")
		if got != want {
			t.Fatalf("output changed from REPLAN_OPENPROJECT_BINDINGS_PATH alone")
		}
	})
}

func TestBindingsLayerRefusesWritesAndProjectReportOutput(t *testing.T) {
	project := copyCmdProjectFixture(t)
	bindings := writeCLIBindings(t, "task.op.1234", "92")

	stdout, stderr := tempOutputFiles(t)
	args := []string{"task", "set-dates", "--project", project, "--task", "task.op.1234", "--planned-start", "2026-07-14", "--reason", "test", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--bindings", bindings, "--format", "json"}
	if code := run(args, stdout, stderr); code != exitInvalidUsage {
		t.Fatalf("writer exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if env := readCLIEnvelope(t, stdout.Name()); !hasDiagnosticCode(env.Errors, bindingsReadOnlyCode) {
		t.Fatalf("writer errors = %#v", env.Errors)
	}

	stdout, stderr = tempOutputFiles(t)
	inside := filepath.Join(project, "reports", "private")
	if code := run([]string{"report", "dashboard", "--project", project, "--bindings", bindings, "--output", inside, "--format", "json"}, stdout, stderr); code != exitInvalidUsage {
		t.Fatalf("inside exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if env := readCLIEnvelope(t, stdout.Name()); !hasDiagnosticCode(env.Errors, bindingsOutputCode) {
		t.Fatalf("inside errors = %#v", env.Errors)
	}

	stdout, stderr = tempOutputFiles(t)
	outside := filepath.Join(t.TempDir(), "dashboard")
	if code := run([]string{"report", "dashboard", "--project", project, "--bindings", bindings, "--output", outside, "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("outside exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if _, err := os.Stat(filepath.Join(outside, "raw", "openproject_overlay.json")); err != nil {
		t.Fatalf("outside dashboard: %v", err)
	}
}

func TestBindingsLayerRefusesSnapshotCandidateAndExportWrites(t *testing.T) {
	project := copyCmdProjectFixture(t)
	bindings := writeCLIBindings(t, "task.op.1234", "92")
	for _, test := range []struct {
		name string
		args []string
		code string
	}{
		{"import snapshot", []string{"import", "openproject", "--project", project, "--from-file", filepath.Join("..", "..", "examples", "imports", "openproject", "work-packages.sample.json"), "--bindings", bindings, "--format", "json"}, bindingsReadOnlyCode},
		{"tracking candidates", []string{"tracking", "suggest", "--project", project, "--actor", "actor.human.operator", "--write-candidates", "--bindings", bindings, "--format", "json"}, bindingsReadOnlyCode},
		{"export inside project", []string{"export", "json", "--project", project, "--output", filepath.Join(project, "export.json"), "--bindings", bindings, "--format", "json"}, bindingsOutputCode},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			if code := run(test.args, stdout, stderr); code != exitInvalidUsage {
				t.Fatalf("exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			if env := readCLIEnvelope(t, stdout.Name()); !hasDiagnosticCode(env.Errors, test.code) {
				t.Fatalf("errors = %#v, want %s", env.Errors, test.code)
			}
		})
	}

	stdout, stderr := tempOutputFiles(t)
	outside := filepath.Join(t.TempDir(), "export.json")
	if code := run([]string{"export", "json", "--project", project, "--output", outside, "--bindings", bindings, "--format", "json"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("outside export exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
}

func writeCLIBindings(t *testing.T, taskID, objectID string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bindings.yaml")
	content := "openproject_private_bindings:\n  tasks:\n    - replan_task_id: " + taskID + "\n      source_ref:\n        source: openproject\n        object_type: work_package\n        object_id: \"" + objectID + "\"\n  dependencies: []\n  resources: []\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTaskCommandRefusalExitClasses(t *testing.T) {
	project := copyCmdProjectFixture(t)
	baseAdd := []string{"task", "add", "--project", project, "--id", "task.op.cli-test", "--title", "CLI test", "--demand", "implementer=resource.human.ivan:1:hours", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--format", "json"}
	tests := []struct {
		name, code string
		wantExit   int
		args       []string
	}{
		{name: "validation", code: "invalid_resource_unit", wantExit: exitValidationFailed, args: replaceCLIFlagValue(baseAdd, "--demand", "implementer=resource.human.ivan:1:tokens")},
		{name: "not found", code: "resource_not_found", wantExit: exitNotFound, args: replaceCLIFlagValue(baseAdd, "--demand", "implementer=resource.missing:1:hours")},
		{name: "set dates not found", code: "task_not_found", wantExit: exitNotFound, args: []string{"task", "set-dates", "--project", project, "--task", "task.missing", "--planned-start", "2026-07-14", "--reason", "test", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--format", "json"}},
		{name: "set title not found", code: "task_not_found", wantExit: exitNotFound, args: []string{"task", "set-title", "--project", project, "--task", "task.missing", "--title", "Missing", "--reason", "test", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--format", "json"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			if code := run(tc.args, stdout, stderr); code != tc.wantExit {
				t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, tc.wantExit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if !hasDiagnosticCode(env.Errors, tc.code) {
				t.Fatalf("diagnostics %#v missing %s", env.Errors, tc.code)
			}
		})
	}
}

func TestTaskRecordActualWindowEnvelopeAndExitCodes(t *testing.T) {
	project := copyCmdProjectFixture(t)
	tasksPath := filepath.Join(project, "canonical", "tasks.yaml")
	tasks, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(tasks), "tracking_status: ready", "tracking_status: accepted", 1)
	if err := os.WriteFile(tasksPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	actual := "actuals:\n  - id: actual.cli-window\n    type: consumption\n    task_ref: {kind: task, id: task.op.1234}\n    amount: 1\n    unit: hours\n    status_date: 2026-10-03\n    accepted_by: {kind: actor, id: actor.human.operator}\n    evidence_refs: [{kind: evidence, id: evidence.github.pr.45}]\n"
	if err := os.WriteFile(filepath.Join(project, "actuals/actual.cli-window.yaml"), []byte(actual), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"task", "record-actual-window", "--project", project, "--task", "task.op.1234", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--format", "json"}
	stdout, stderr := tempOutputFiles(t)
	if code := run(base, stdout, stderr); code != exitSuccess {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	var result app.TaskRecordActualWindowResult
	if err := json.Unmarshal(env.Result, &result); err != nil || !env.OK || env.Command != "task record-actual-window" || result.NewStart != "2026-10-03" || !result.Changed {
		t.Fatalf("env=%#v result=%#v err=%v", env, result, err)
	}

	for _, tc := range []struct {
		name, task string
		want       int
		code       string
	}{{"not found", "task.missing", exitNotFound, "task_not_found"}, {"not finished", "task.offline.repair-pump", exitValidationFailed, "task_not_finished"}} {
		t.Run(tc.name, func(t *testing.T) {
			args := replaceCLIFlagValue(base, "--task", tc.task)
			out, errOut := tempOutputFiles(t)
			if code := run(args, out, errOut); code != tc.want {
				t.Fatalf("exit=%d want=%d stdout=%s", code, tc.want, readFileString(t, out.Name()))
			}
			if got := readCLIEnvelope(t, out.Name()); !hasDiagnosticCode(got.Errors, tc.code) {
				t.Fatalf("errors=%#v missing %s", got.Errors, tc.code)
			}
		})
	}
}

func replaceCLIFlagValue(args []string, flagName, value string) []string {
	result := append([]string(nil), args...)
	for i := 0; i+1 < len(result); i++ {
		if result[i] == flagName {
			result[i+1] = value
			break
		}
	}
	return result
}

func TestCLIJSONEnvelopeContract(t *testing.T) {
	cases := []struct {
		name        string
		command     string
		projectPath string
		run         func(stdout, stderr *os.File) int
		assert      func(t *testing.T, env cliTestEnvelope)
	}{
		{
			name:        "validate",
			command:     "validate",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runValidate([]string{"../../examples/small-team", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "valid", true)
				assertResultNumberField(t, env, "checked_files", 11)
			},
		},
		{
			name:        "workload",
			command:     "workload",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runWorkload([]string{"--project", "../../examples/small-team", "--week", "2026-W28", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "week", "2026-W28")
				assertResultArrayNotEmpty(t, env, "resources")
			},
		},
		{
			name:        "report weekly",
			command:     "report weekly",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runReportWeekly([]string{"--project", "../../examples/small-team", "--week", "2026-W28", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "week", "2026-W28")
				assertResultArrayNotEmpty(t, env, "resources")
			},
		},
		{
			name:        "report state",
			command:     "report state",
			projectPath: "../../examples/replan-self",
			run: func(stdout, stderr *os.File) int {
				return runReportState([]string{"--project", "../../examples/replan-self", "--week", "2026-W31", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "week", "2026-W31")
				assertResultObjectField(t, env, "health")
				assertResultObjectField(t, env, "source_health")
				assertResultArrayNotEmpty(t, env, "tracking")
				assertResultArrayNotEmpty(t, env, "next_actions")
			},
		},
		{
			name:        "next",
			command:     "next",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runNext([]string{"--project", "../../examples/small-team", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "week", "2026-W28")
				assertResultArrayNotEmpty(t, env, "waiting_on_person")
				assertResultArrayField(t, env, "agent_can_prepare")
				assertResultObjectField(t, env, "health")
			},
		},
		{
			name:        "tracking suggest",
			command:     "tracking suggest",
			projectPath: "../../examples/replan-self",
			run: func(stdout, stderr *os.File) int {
				return runTrackingSuggest([]string{"--project", "../../examples/replan-self", "--week", "2026-W31", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "week", "2026-W31")
				assertResultArrayField(t, env, "suggestions")
			},
		},
		{
			name:        "scenario diff",
			command:     "scenario diff",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runScenarioDiff([]string{"--project", "../../examples/small-team", "--format", "json", "scenario.ivan-unavailable-week-28"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "scenario_id", "scenario.ivan-unavailable-week-28")
				assertResultArrayNotEmpty(t, env, "changes")
			},
		},
		{
			name:        "proposal list",
			command:     "proposal list",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runProposalList([]string{"--project", "../../examples/small-team", "--status", "open", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultArrayNotEmpty(t, env, "proposals")
			},
		},
		{
			name:        "proposal suggest-dependencies",
			command:     "proposal suggest-dependencies",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runProposalSuggestDependencies([]string{"--project", "../../examples/small-team", "--actor", "actor.ai.codex.local", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "dry_run", true)
				assertResultArrayNotEmpty(t, env, "proposals")
			},
		},
		{
			name:        "dependency list",
			command:     "dependency list",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runDependencyList([]string{"--project", "../../examples/small-team", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultArrayNotEmpty(t, env, "dependencies")
			},
		},
		{
			name:        "actual add-consumption dry-run",
			command:     "actual add-consumption",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runActualAddConsumption([]string{
					"--project", "../../examples/small-team",
					"--task", "task.op.1234",
					"--demand", "resource_demand.task-op-1234.implementer",
					"--resource", "resource.human.ivan",
					"--amount", "1",
					"--unit", "hours",
					"--actor", "actor.human.operator",
					"--occurred-at", "2026-07-08T10:00:00+07:00",
					"--status-date", "2026-07-08",
					"--dry-run",
					"--format", "json",
				}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "dry_run", true)
				assertResultField(t, env, "applied", false)
			},
		},
		{
			name:        "evidence add dry-run",
			command:     "evidence add",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runEvidenceAdd([]string{
					"--project", "../../examples/small-team",
					"--id", "evidence.test.cli",
					"--type", "test_result",
					"--subtype", "cli",
					"--source", "local",
					"--url", "https://example.test/evidence/cli",
					"--actor", "actor.human.operator",
					"--observed-at", "2026-07-08T10:00:00+07:00",
					"--confidence", "observed",
					"--summary", "CLI dry run.",
					"--target", "task:task.op.1234",
					"--target", "resource_demand:resource_demand.task-op-1234.implementer",
					"--dry-run",
					"--format", "json",
				}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "dry_run", true)
				assertResultField(t, env, "file_path", "evidence/evidence.2026-W28.yaml")
				assertResultObjectField(t, env, "evidence")
			},
		},
		{
			name:        "import openproject dry-run",
			command:     "import openproject",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runImportOpenProject([]string{"--project", "../../examples/small-team", "--from-file", "../../examples/imports/openproject/work-packages.sample.json", "--dry-run", "--format", "json"}, stdout, stderr)
			},
			assert: func(t *testing.T, env cliTestEnvelope) {
				assertResultField(t, env, "dry_run", true)
				assertResultArrayNotEmpty(t, env, "task_candidates")
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			exitCode := tc.run(stdout, stderr)
			if exitCode != exitSuccess {
				t.Fatalf("exit code = %d, want %d; stderr=%s", exitCode, exitSuccess, readFileString(t, stderr.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if !env.OK {
				t.Fatalf("ok = false, want true; envelope=%s", readFileString(t, stdout.Name()))
			}
			if env.Command != tc.command {
				t.Fatalf("command = %q, want %q", env.Command, tc.command)
			}
			if env.ProjectPath != tc.projectPath {
				t.Fatalf("project_path = %q, want %q", env.ProjectPath, tc.projectPath)
			}
			if env.Result == nil || string(env.Result) == "null" {
				t.Fatalf("result must be non-null: %s", readFileString(t, stdout.Name()))
			}
			if env.Warnings == nil {
				t.Fatalf("warnings must be present array")
			}
			if env.Errors == nil {
				t.Fatalf("errors must be present array")
			}
			if len(env.Errors) != 0 {
				t.Fatalf("errors = %#v, want empty", env.Errors)
			}
			tc.assert(t, env)
		})
	}
}

func TestWorkloadAndScenarioHumanReadableResults(t *testing.T) {
	tests := []struct {
		name   string
		args   []string
		format string
		wants  []string
	}{
		{name: "workload markdown", args: []string{"workload", "--project", "../../examples/small-team", "--week", "2026-W28", "--demands"}, format: "markdown", wants: []string{"- Week: `2026-W28`", "| Resource | Class | Unit | Capacity", "Ivan", "resource_demand.task-op-1234.implementer"}},
		{name: "workload text", args: []string{"workload", "--project", "../../examples/small-team", "--week", "2026-W28", "--demands"}, format: "text", wants: []string{"Workload: 2026-W28", "RESOURCE", "CAPACITY", "Ivan", "resource_demand.task-op-1234.implementer"}},
		{name: "scenario list markdown", args: []string{"scenario", "list", "--project", "../../examples/small-team"}, format: "markdown", wants: []string{"| ID | Name | Status | Changes |", "scenario.ivan-capacity-6", "Ivan capacity is six hours", "draft"}},
		{name: "scenario list text", args: []string{"scenario", "list", "--project", "../../examples/small-team"}, format: "text", wants: []string{"ID", "NAME", "STATUS", "CHANGES", "scenario.ivan-capacity-6"}},
		{name: "scenario show markdown", args: []string{"scenario", "show", "scenario.ivan-capacity-6", "--project", "../../examples/small-team"}, format: "markdown", wants: []string{"- ID: `scenario.ivan-capacity-6`", "- Description: —", "| Target | Field | From | To |", "capacity.human-ivan.hours.weekly", "amount", "32", "6"}},
		{name: "scenario show text", args: []string{"scenario", "show", "scenario.ivan-capacity-6", "--project", "../../examples/small-team"}, format: "text", wants: []string{"ID: scenario.ivan-capacity-6", "Description: —", "TARGET", "FIELD", "FROM", "TO", "capacity.human-ivan.hours.weekly", "32", "6"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			args := append(append([]string(nil), tc.args...), "--format", tc.format)
			if code := run(args, stdout, stderr); code != exitSuccess {
				t.Fatalf("exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			output := readFileString(t, stdout.Name())
			for _, want := range tc.wants {
				if !strings.Contains(output, want) {
					t.Errorf("output missing %q:\n%s", want, output)
				}
			}
		})
	}
}

func TestProposalSuggestDependenciesWriteCandidatesCLI(t *testing.T) {
	projectDir := copyCmdProjectFixture(t)
	stdout, stderr := tempOutputFiles(t)
	exitCode := runProposalSuggestDependencies([]string{
		"--project", projectDir,
		"--write-candidates",
		"--actor", "actor.ai.codex.local",
		"--format", "json",
	}, stdout, stderr)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exitCode, exitSuccess, readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	if !env.OK {
		t.Fatalf("ok = false, want true; envelope=%s", readFileString(t, stdout.Name()))
	}
	assertResultField(t, env, "dry_run", false)
	assertResultField(t, env, "applied", true)
	assertResultArrayNotEmpty(t, env, "written_files")
	proposalPath := filepath.Join(projectDir, "proposals", "open", "proposal.dependency.dependency.openai-quota-blocks-validation.mark-successor-blocked.yaml")
	assertFileContains(t, proposalPath, "type: mark_successor_blocked")
}

func TestTrackingSuggestWriteCandidatesCLI(t *testing.T) {
	projectDir := copyCmdProjectFixtureFrom(t, "../../examples/replan-self", "replan-self")
	removeCmdTrackingProposalArtifacts(t, projectDir)
	markCmdTrackingProposalDemandsStale(t, projectDir)
	stdout, stderr := tempOutputFiles(t)
	exitCode := runTrackingSuggest([]string{
		"--project", projectDir,
		"--week", "2026-W31",
		"--write-candidates",
		"--actor", "actor.ai.codex.local",
		"--format", "json",
	}, stdout, stderr)

	if exitCode != exitSuccess {
		t.Fatalf("exit code = %d, want %d; stderr=%s", exitCode, exitSuccess, readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	if !env.OK {
		t.Fatalf("ok = false, want true; envelope=%s", readFileString(t, stdout.Name()))
	}
	assertResultField(t, env, "dry_run", false)
	assertResultField(t, env, "applied", true)
	assertResultArrayNotEmpty(t, env, "proposals")
	assertResultArrayNotEmpty(t, env, "written_files")
	proposalPath := filepath.Join(projectDir, "proposals", "open", "proposal.tracking.resource_demand-tracking-proposals-maintainer.set-remaining-zero.yaml")
	assertFileContains(t, proposalPath, "type: update_remaining_work")
	assertFileContains(t, proposalPath, "path: remaining_amount")
	assertFileContains(t, proposalPath, "value: 0")
}

func TestProposalAcceptSupportsInterspersedPositionalCLI(t *testing.T) {
	projectDir := copyCmdProjectFixture(t)
	stdout, stderr := tempOutputFiles(t)
	exitCode := runProposalSuggestDependencies([]string{
		"--project", projectDir,
		"--write-candidates",
		"--actor", "actor.ai.codex.local",
		"--format", "json",
	}, stdout, stderr)
	if exitCode != exitSuccess {
		t.Fatalf("suggest exit code = %d, want %d; stderr=%s", exitCode, exitSuccess, readFileString(t, stderr.Name()))
	}

	stdout, stderr = tempOutputFiles(t)
	exitCode = runProposalAccept([]string{
		"--project", projectDir,
		"proposal.dependency.dependency.openai-quota-blocks-validation.mark-successor-blocked",
		"--actor", "actor.human.operator",
		"--reason", "Block successor from accepted dependency proposal.",
		"--apply",
		"--format", "json",
	}, stdout, stderr)

	if exitCode != exitSuccess {
		t.Fatalf("accept exit code = %d, want %d; stderr=%s stdout=%s", exitCode, exitSuccess, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	if !env.OK {
		t.Fatalf("ok = false, want true; envelope=%s", readFileString(t, stdout.Name()))
	}
	assertResultField(t, env, "suggested_applied", true)
	assertFileContains(t, filepath.Join(projectDir, "canonical", "tasks.yaml"), "tracking_status: blocked")
}

func TestPositionalArgSupportsBeforeAndAfterFlags(t *testing.T) {
	fs := flag.NewFlagSet("leading", flag.ContinueOnError)
	project := fs.String("project", "", "project")
	leading, args := splitSinglePositional([]string{"scenario.1", "--project", "examples/small-team"}, fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse leading args returned error: %v", err)
	}
	if got := positionalArg(leading, fs); got != "scenario.1" {
		t.Fatalf("leading positional = %q, want scenario.1", got)
	}
	if *project != "examples/small-team" {
		t.Fatalf("project = %q, want examples/small-team", *project)
	}

	fs = flag.NewFlagSet("interspersed", flag.ContinueOnError)
	project = fs.String("project", "", "project")
	actor := fs.String("actor", "", "actor")
	leading, args = splitSinglePositional([]string{"--project", "examples/small-team", "scenario.2", "--actor", "actor.human.operator"}, fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse interspersed args returned error: %v", err)
	}
	if got := positionalArg(leading, fs); got != "scenario.2" {
		t.Fatalf("interspersed positional = %q, want scenario.2", got)
	}
	if *project != "examples/small-team" {
		t.Fatalf("project = %q, want examples/small-team", *project)
	}
	if *actor != "actor.human.operator" {
		t.Fatalf("actor = %q, want actor.human.operator", *actor)
	}

	fs = flag.NewFlagSet("inline", flag.ContinueOnError)
	project = fs.String("project", "", "project")
	leading, args = splitSinglePositional([]string{"--project=examples/small-team", "scenario.3"}, fs)
	if err := fs.Parse(args); err != nil {
		t.Fatalf("Parse inline args returned error: %v", err)
	}
	if got := positionalArg(leading, fs); got != "scenario.3" {
		t.Fatalf("inline positional = %q, want scenario.3", got)
	}
	if *project != "examples/small-team" {
		t.Fatalf("project = %q, want examples/small-team", *project)
	}
}

func TestCLIExitCodesClassifyNotFound(t *testing.T) {
	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()

	exitCode := runScenarioShow([]string{"--project", "../../examples/small-team", "--format", "json", "missing.scenario"}, stdout, stderr)
	if exitCode != exitNotFound {
		t.Fatalf("scenario show missing exit = %d, want %d", exitCode, exitNotFound)
	}

	exitCode = runProposalShow([]string{"--project", "../../examples/small-team", "--format", "json", "missing.proposal"}, stdout, stderr)
	if exitCode != exitNotFound {
		t.Fatalf("proposal show missing exit = %d, want %d", exitCode, exitNotFound)
	}

	exitCode = runImportAccept([]string{"--project", "../../examples/small-team", "--dry-run", "--format", "json", "source_snapshot.openproject.20990101T000000"}, stdout, stderr)
	if exitCode != exitNotFound {
		t.Fatalf("import accept missing snapshot exit = %d, want %d", exitCode, exitNotFound)
	}

	exitCode = runImportOpenProject([]string{"--project", "../../examples/small-team", "--from-file", "../../examples/imports/openproject/work-packages.sample.json", "--profile", "missing.profile", "--dry-run", "--format", "json"}, stdout, stderr)
	if exitCode != exitNotFound {
		t.Fatalf("import openproject missing profile exit = %d, want %d", exitCode, exitNotFound)
	}
}

func TestCLIExitCodesKeepValidationAndUsageSeparate(t *testing.T) {
	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()

	exitCode := runReportWeekly([]string{"--project", "../../examples/small-team", "--week", "2026-W28", "--format", "xml"}, stdout, stderr)
	if exitCode != exitInvalidUsage {
		t.Fatalf("unsupported report format exit = %d, want %d", exitCode, exitInvalidUsage)
	}

	exitCode = runActualAddConsumption([]string{
		"--project", "../../examples/small-team",
		"--task", "task.op.1234",
		"--demand", "resource_demand.task-op-1234.implementer",
		"--resource", "resource.human.ivan",
		"--amount", "1",
		"--unit", "hours",
		"--dry-run",
		"--format", "json",
	}, stdout, stderr)
	if exitCode != exitValidationFailed {
		t.Fatalf("actual missing actor exit = %d, want %d", exitCode, exitValidationFailed)
	}
}

func TestEvidenceAddRefusalExitCodes(t *testing.T) {
	validArgs := func() []string {
		return []string{
			"--project", "../../examples/small-team",
			"--id", "evidence.test.refusal",
			"--type", "test_result",
			"--actor", "actor.human.operator",
			"--observed-at", "2026-07-08T10:00:00+07:00",
			"--summary", "Refusal test.",
			"--target", "task:task.op.1234",
			"--dry-run", "--format", "json",
		}
	}
	tests := []struct {
		name     string
		args     func() []string
		wantExit int
		wantCode string
	}{
		{name: "duplicate", wantExit: exitValidationFailed, wantCode: "evidence_already_exists", args: func() []string {
			args := validArgs()
			args[3] = "evidence.github.pr.45"
			return args
		}},
		{name: "unknown target", wantExit: exitNotFound, wantCode: "evidence_target_not_found", args: func() []string {
			args := validArgs()
			args[13] = "task:task.missing"
			return args
		}},
		{name: "unknown target kind", wantExit: exitValidationFailed, wantCode: "invalid_evidence_target_kind", args: func() []string {
			args := validArgs()
			args[13] = "widget:widget.1"
			return args
		}},
		{name: "unknown actor", wantExit: exitNotFound, wantCode: "actor_not_found", args: func() []string {
			args := validArgs()
			args[7] = "actor.missing"
			return args
		}},
		{name: "bad time", wantExit: exitValidationFailed, wantCode: "invalid_observed_at", args: func() []string {
			args := validArgs()
			args[9] = "not-a-time"
			return args
		}},
		{name: "missing required target", wantExit: exitValidationFailed, wantCode: "missing_target", args: func() []string {
			args := validArgs()
			return append(args[:12], args[14:]...)
		}},
		{name: "missing required type", wantExit: exitValidationFailed, wantCode: "missing_evidence_type", args: func() []string {
			args := validArgs()
			return append(args[:4], args[6:]...)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			if code := runEvidenceAdd(tc.args(), stdout, stderr); code != tc.wantExit {
				t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, tc.wantExit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if !hasDiagnosticCode(env.Errors, tc.wantCode) {
				t.Fatalf("errors = %#v, want %s", env.Errors, tc.wantCode)
			}
		})
	}
}

// OpenProject failing, unreachable or refusing the token, is an external
// source error, not an invalid plan (finding C-08 of the 2026-10-15 review).
func TestCLIExitCodesClassifyOpenProjectFailureAsExternal(t *testing.T) {
	refused := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/hal+json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"_type":"Error","errorIdentifier":"urn:openproject-org:api:v3:errors:Unauthenticated","message":"no"}`))
	}))
	defer refused.Close()
	closed := httptest.NewServer(http.NotFoundHandler())
	closedURL := closed.URL
	closed.Close()
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	t.Setenv("REPLAN_OPENPROJECT_TIMEOUT_SECONDS", "2")

	for name, baseURL := range map[string]string{"refused token": refused.URL, "unreachable": closedURL} {
		stdout, stderr := tempOutputFiles(t)
		exitCode := runImportOpenProject([]string{"--project", "../../examples/small-team", "--base-url", baseURL, "--project-id", "demo",
			"--token-env", "REPLAN_TEST_OPENPROJECT_TOKEN", "--dry-run", "--format", "json"}, stdout, stderr)
		if exitCode != exitExternalSource {
			t.Fatalf("%s: import openproject exit = %d, want %d; stdout=%s", name, exitCode, exitExternalSource, readFileString(t, stdout.Name()))
		}
		stdout.Close()
		stderr.Close()
	}
}

func TestCLIPrivateOpenProjectIDsRefusalsExitInvalidUsage(t *testing.T) {
	projectDir := copyCmdProjectFixture(t)
	stdout, stderr := tempOutputFiles(t)
	if code := runImportOpenProject([]string{
		"--project", projectDir,
		"--from-file", "../../examples/imports/openproject/work-packages.sample.json",
		"--format", "json",
	}, stdout, stderr); code != exitSuccess {
		t.Fatalf("file import exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	var importResult struct {
		Result struct {
			Snapshot struct {
				ID string `json:"id"`
			} `json:"snapshot"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(readFileString(t, stdout.Name())), &importResult); err != nil {
		t.Fatal(err)
	}
	stdout.Close()
	stderr.Close()
	setCmdProjectOpenProjectIDs(t, projectDir, "private_bindings")

	stdout, stderr = tempOutputFiles(t)
	code := runImportAccept([]string{"--project", projectDir, "--dry-run", "--format", "json", importResult.Result.Snapshot.ID}, stdout, stderr)
	if code != exitInvalidUsage {
		t.Fatalf("accept exit = %d, want %d; stdout=%s", code, exitInvalidUsage, readFileString(t, stdout.Name()))
	}
	if env := readCLIEnvelope(t, stdout.Name()); !hasDiagnosticCode(env.Errors, "openproject_ids_private") {
		t.Fatalf("accept errors = %#v", env.Errors)
	}
	stdout.Close()
	stderr.Close()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requests++ }))
	defer server.Close()
	t.Setenv("REPLAN_TEST_OPENPROJECT_TOKEN", "test-token")
	stdout, stderr = tempOutputFiles(t)
	code = runImportOpenProject([]string{"--project", projectDir, "--base-url", server.URL, "--project-id", "demo", "--token-env", "REPLAN_TEST_OPENPROJECT_TOKEN", "--format", "json"}, stdout, stderr)
	if code != exitInvalidUsage {
		t.Fatalf("live import exit = %d, want %d; stdout=%s", code, exitInvalidUsage, readFileString(t, stdout.Name()))
	}
	if requests != 0 {
		t.Fatalf("refused live import made %d requests", requests)
	}
}

func TestCLIRejectsBadOpenProjectRetryEnvironment(t *testing.T) {
	tests := []struct {
		name    string
		setting string
		value   string
		message string
	}{
		{"retries not integer", "REPLAN_OPENPROJECT_RETRIES", "one", "REPLAN_OPENPROJECT_RETRIES must be an integer"},
		{"retries negative", "REPLAN_OPENPROJECT_RETRIES", "-1", "REPLAN_OPENPROJECT_RETRIES must be non-negative"},
		{"delay not numeric", "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS", "soon", "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS must be numeric"},
		{"delay not positive", "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS", "0", "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS must be positive"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("REPLAN_OPENPROJECT_RETRIES", "0")
			t.Setenv("REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS", "2")
			t.Setenv(test.setting, test.value)
			stdout, stderr := tempOutputFiles(t)
			defer stdout.Close()
			defer stderr.Close()
			exitCode := runImportOpenProject([]string{
				"--project", "../../examples/small-team",
				"--from-file", "../../examples/imports/openproject/work-packages.sample.json",
				"--dry-run", "--format", "json",
			}, stdout, stderr)
			if exitCode != exitInvalidUsage {
				t.Fatalf("exit = %d, want %d", exitCode, exitInvalidUsage)
			}
			if got := readFileString(t, stderr.Name()); !strings.Contains(got, test.message) {
				t.Fatalf("stderr = %q, want %q", got, test.message)
			}
		})
	}
}

func TestCLIRejectsMalformedWeek(t *testing.T) {
	cases := []struct {
		name        string
		command     string
		projectPath string
		run         func(stdout, stderr *os.File) int
	}{
		{
			name:        "workload",
			command:     "workload",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runWorkload([]string{"--project", "../../examples/small-team", "--week", "banana", "--format", "json"}, stdout, stderr)
			},
		},
		{
			name:        "report weekly",
			command:     "report weekly",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runReportWeekly([]string{"--project", "../../examples/small-team", "--week", "banana", "--format", "json"}, stdout, stderr)
			},
		},
		{
			name:        "report state",
			command:     "report state",
			projectPath: "../../examples/replan-self",
			run: func(stdout, stderr *os.File) int {
				return runReportState([]string{"--project", "../../examples/replan-self", "--week", "banana", "--format", "json"}, stdout, stderr)
			},
		},
		{
			name:        "report dashboard",
			command:     "report dashboard",
			projectPath: "../../examples/replan-self",
			run: func(stdout, stderr *os.File) int {
				return runReportDashboard([]string{"--project", "../../examples/replan-self", "--week", "banana", "--format", "json"}, stdout, stderr)
			},
		},
		{
			name:        "next",
			command:     "next",
			projectPath: "../../examples/replan-self",
			run: func(stdout, stderr *os.File) int {
				return runNext([]string{"--project", "../../examples/replan-self", "--week", "banana", "--format", "json"}, stdout, stderr)
			},
		},
		{
			name:        "tracking suggest",
			command:     "tracking suggest",
			projectPath: "../../examples/replan-self",
			run: func(stdout, stderr *os.File) int {
				return runTrackingSuggest([]string{"--project", "../../examples/replan-self", "--week", "banana", "--format", "json"}, stdout, stderr)
			},
		},
		{
			name:        "scenario diff",
			command:     "scenario diff",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				return runScenarioDiff([]string{"--project", "../../examples/small-team", "scenario.ivan-unavailable-week-28", "--against", "current", "--week", "banana", "--format", "json"}, stdout, stderr)
			},
		},
		{
			name:        "export csv",
			command:     "export csv",
			projectPath: "../../examples/small-team",
			run: func(stdout, stderr *os.File) int {
				outputPath := filepath.Join(t.TempDir(), "workload.csv")
				return runExportCSV([]string{"--project", "../../examples/small-team", "--report", "workload", "--week", "banana", "--output", outputPath}, stdout, stderr)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			exitCode := tc.run(stdout, stderr)
			if exitCode != exitInvalidUsage {
				t.Fatalf("exit code = %d, want %d; stderr=%s stdout=%s", exitCode, exitInvalidUsage, readFileString(t, stderr.Name()), readFileString(t, stdout.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if env.OK {
				t.Fatalf("ok = true, want false; envelope=%s", readFileString(t, stdout.Name()))
			}
			if env.Command != tc.command {
				t.Fatalf("command = %q, want %q", env.Command, tc.command)
			}
			if env.ProjectPath != tc.projectPath {
				t.Fatalf("project_path = %q, want %q", env.ProjectPath, tc.projectPath)
			}
			if len(env.Errors) != 1 || env.Errors[0].Code != "invalid_week" {
				t.Fatalf("errors = %#v, want one invalid_week diagnostic", env.Errors)
			}
			if len(env.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want empty", env.Warnings)
			}
		})
	}
}

func TestWriteWeeklyReportArtifactWritesHTML(t *testing.T) {
	projectDir := t.TempDir()
	report := reports.WeeklyReport{
		ProjectPath: projectDir,
		ProjectID:   "project.test",
		ProjectName: "Test project",
		Week:        "2026-W28",
		Summary: reports.WeeklySummary{
			ResourceRows: 1,
			Overloaded:   1,
		},
		Resources: []reports.ResourceRow{{
			ResourceID:    "resource.human.ivan",
			ResourceName:  "Ivan",
			ResourceClass: "human",
			Unit:          "hours",
			Capacity:      32,
			Planned:       40,
			Remaining:     40,
			Overload:      8,
		}},
		Warnings: []reports.ReportIssue{{
			Code:     "resource_overloaded",
			Severity: "warning",
			Message:  "Ivan is overloaded.",
			TargetID: "resource.human.ivan",
		}},
	}

	relPath, err := writeWeeklyReportArtifact(projectDir, "2026-W28", "html", report)

	if err != nil {
		t.Fatalf("writeWeeklyReportArtifact returned error: %v", err)
	}
	if relPath != "reports/weekly/2026-W28.html" {
		t.Fatalf("relPath = %q, want reports/weekly/2026-W28.html", relPath)
	}
	assertFileContains(t, filepath.Join(projectDir, filepath.FromSlash(relPath)), "<h2>Workload</h2>")
	assertFileContains(t, filepath.Join(projectDir, filepath.FromSlash(relPath)), "resource_overloaded")
}

func TestWriteDashboardArtifacts(t *testing.T) {
	projectDir := t.TempDir()
	overview := reports.DashboardOverview{
		SchemaVersion: 1,
		GeneratedAt:   "2026-07-20T10:00:00+07:00",
		ProjectPath:   projectDir,
		ProjectID:     "project.test",
		ProjectName:   "Test project",
		StatusDate:    "2026-07-20",
		Health: reports.DashboardHealthSummary{
			TaskCount: 1,
		},
		Gantt: reports.DashboardGanttView{
			SchemaVersion: 1,
			ProjectID:     "project.test",
			ProjectName:   "Test project",
			StartDate:     "2026-07-20",
			FinishDate:    "2026-07-20",
			DayCount:      1,
			TaskCount:     1,
			Tasks: []reports.DashboardGanttTask{{
				ID:             "task.1",
				Title:          "Task 1",
				TrackingStatus: "ready",
				PlannedStart:   "2026-07-20",
				PlannedFinish:  "2026-07-20",
				DurationDays:   1,
			}},
		},
		Workload: []reports.ResourceRow{{
			ResourceID: "resource.human.ivan",
			Unit:       "hours",
			Capacity:   24,
		}},
	}
	metadata := reports.DashboardPageMetadata{SchemaVersion: 1, GeneratedAt: overview.GeneratedAt, ProjectID: overview.ProjectID, ProjectName: overview.ProjectName, StatusDate: overview.StatusDate, SourceSnapshotRefs: []string{}, Warnings: []reports.ReportIssue{}}
	overview.WorkloadPage = reports.DashboardWorkloadPage{DashboardPageMetadata: metadata, Data: reports.DashboardWorkloadData{Weeks: []string{"2026-W30"}, Resources: []reports.DashboardWorkloadResource{{ResourceID: "resource.human.ivan", Unit: "hours", Weeks: []reports.DashboardWorkloadCell{{Week: "2026-W30", Capacity: 24, Demands: []reports.DashboardWorkloadDemand{}}}}}}}
	overview.ScenariosPage = reports.DashboardScenariosPage{DashboardPageMetadata: metadata, Data: reports.DashboardScenariosData{Scenarios: []reports.DashboardScenarioDetail{{ID: "scenario.test", Changes: []reports.DashboardScenarioChange{}, WorkloadBefore: []reports.ResourceRow{}, WorkloadAfter: []reports.ResourceRow{}, Warnings: []reports.ReportIssue{}, Errors: []reports.ReportIssue{}}}}}
	overview.ProposalsPage = reports.DashboardProposalsPage{DashboardPageMetadata: metadata, Data: reports.DashboardProposalsData{Proposals: []reports.DashboardProposalDetail{{ID: "proposal.test", Evidence: []reports.DashboardProposalEvidence{}}}}}
	overview.EstimatesPage = reports.DashboardEstimatesPage{DashboardPageMetadata: metadata, Data: reports.DashboardEstimatesData{Baselines: []reports.DashboardEstimateBaseline{{ID: "baseline.test", Resources: []reports.DashboardEstimateResource{}, Tasks: []reports.DashboardEstimateTask{}, Calibrations: []reports.DashboardEstimateCalibration{}, Warnings: []reports.ReportIssue{}, Errors: []reports.ReportIssue{}}}}}

	result, err := writeDashboardArtifacts(projectDir, "reports/latest", overview)

	if err != nil {
		t.Fatalf("writeDashboardArtifacts returned error: %v", err)
	}
	if result.OutputDir != "reports/latest" {
		t.Fatalf("OutputDir = %q, want reports/latest", result.OutputDir)
	}
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/index.html"), "RePlan Dashboard: Test project")
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/gantt.html"), "RePlan Gantt: Test project")
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/workload.html"), "resource.human.ivan")
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/scenarios.html"), "scenario.test")
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/proposals.html"), "proposal.test")
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/estimates.html"), "baseline.test")
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/overview.json"), `"schema_version": 1`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/gantt.json"), `"task_count": 1`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/workload.json"), `"resource_id": "resource.human.ivan"`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/scenarios.json"), `"id": "scenario.test"`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/proposals.json"), `"id": "proposal.test"`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/estimates.json"), `"id": "baseline.test"`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/manifest.json"), `"gantt.html"`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/manifest.json"), `"raw/gantt.json"`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/manifest.json"), `"index.html"`)
	assertFileContains(t, filepath.Join(projectDir, "reports/latest/raw/manifest.json"), `"raw/estimates.json"`)
}

func TestScenarioReportWritesAreRefusedInsideProject(t *testing.T) {
	projectDir := copyCmdProjectFixture(t)
	for _, tc := range []struct {
		name string
		run  func(*os.File, *os.File) int
	}{
		{name: "weekly", run: func(stdout, stderr *os.File) int {
			return runReportWeekly([]string{"--project", projectDir, "--week", "2026-W28", "--scenario", "scenario.task-op-1234-reassign-petr", "--write", "--format", "html"}, stdout, stderr)
		}},
		{name: "dashboard", run: func(stdout, stderr *os.File) int {
			return runReportDashboard([]string{"--project", projectDir, "--week", "2026-W28", "--scenario", "scenario.task-op-1234-reassign-petr", "--output", "reports/what-if", "--format", "html"}, stdout, stderr)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			if code := tc.run(stdout, stderr); code != exitInvalidUsage {
				t.Fatalf("exit = %d, want %d", code, exitInvalidUsage)
			}
			env := readCLIEnvelope(t, stdout.Name())
			if !hasDiagnosticCode(env.Errors, "scenario_report_write") {
				t.Fatalf("errors = %#v", env.Errors)
			}
		})
	}
}

func TestScenarioReportJSONCarriesScenarioID(t *testing.T) {
	const scenarioID = "scenario.task-op-1234-reassign-petr"
	for _, tc := range []struct {
		name string
		run  func(*os.File, *os.File) int
	}{
		{name: "weekly", run: func(stdout, stderr *os.File) int {
			return runReportWeekly([]string{"--project", "../../examples/small-team", "--week", "2026-W28", "--scenario", scenarioID, "--format", "json"}, stdout, stderr)
		}},
		{name: "state", run: func(stdout, stderr *os.File) int {
			return runReportState([]string{"--project", "../../examples/small-team", "--week", "2026-W28", "--scenario", scenarioID, "--format", "json"}, stdout, stderr)
		}},
		{name: "dashboard", run: func(stdout, stderr *os.File) int {
			return runReportDashboard([]string{"--project", "../../examples/small-team", "--week", "2026-W28", "--scenario", scenarioID, "--format", "json"}, stdout, stderr)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr := tempOutputFiles(t)
			if code := tc.run(stdout, stderr); code != exitSuccess {
				t.Fatalf("exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			assertResultField(t, readCLIEnvelope(t, stdout.Name()), "scenario_id", scenarioID)
		})
	}
}

func TestScenarioDashboardWritesOutsideProject(t *testing.T) {
	projectDir := copyCmdProjectFixture(t)
	outputDir := filepath.Join(t.TempDir(), "what-if-dashboard")
	stdout, stderr := tempOutputFiles(t)
	code := runReportDashboard([]string{"--project", projectDir, "--week", "2026-W28", "--scenario", "scenario.task-op-1234-reassign-petr", "--output", outputDir, "--format", "html"}, stdout, stderr)
	if code != exitSuccess {
		t.Fatalf("exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	assertFileContains(t, filepath.Join(outputDir, "index.html"), "What-if: scenario scenario.task-op-1234-reassign-petr applied in memory; this is not the plan.")
	assertFileContains(t, filepath.Join(outputDir, "raw", "manifest.json"), `"scenario_id": "scenario.task-op-1234-reassign-petr"`)
	assertFileContains(t, filepath.Join(outputDir, "raw", "openproject_overlay.json"), `"scenario_id": "scenario.task-op-1234-reassign-petr"`)
	walkErr := fs.WalkDir(os.DirFS(outputDir), ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		content := readFileString(t, filepath.Join(outputDir, filepath.FromSlash(path)))
		if strings.Contains(content, projectDir) {
			t.Errorf("%s leaks absolute project path %q", path, projectDir)
		}
		return nil
	})
	if walkErr != nil {
		t.Fatal(walkErr)
	}
	assertFileContains(t, filepath.Join(outputDir, "raw", "manifest.json"), `"project_path": "\u003cproject\u003e"`)
}

func TestRunTaskJugglerRunArtifactsWritesRunArtifacts(t *testing.T) {
	fakeTJ3 := writeFakeTaskJuggler(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "TaskJuggler vFAKE"
  exit 0
fi
grep 'weeklymax 6.00h' "$1" >/dev/null
grep 'taskreport replan_task_schedule' "$1" >/dev/null
cat > reports/replan-task-schedule.csv <<'REPORT'
"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"
"task_op_1234";"Implement validation command";"2026-07-06";"2026-07-13";1.0;0.0;"Ivan (resource_human_ivan)";"done"
REPORT
echo run stdout
echo run stderr >&2
`)
	outputDir := filepath.Join(t.TempDir(), "run")

	result, warnings, errorsOut, err := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:                  "../../examples/small-team",
		ScenarioID:                   "scenario.ivan-capacity-6",
		OutputDir:                    outputDir,
		TaskJugglerBinary:            fakeTJ3,
		UnrepresentedAssumptionsMode: "error",
		Now:                          fixedTaskJugglerRunTime(),
	})

	if err != nil {
		t.Fatalf("runTaskJugglerRunArtifacts returned error: %v", err)
	}
	if result.ID != "taskjuggler.run.20260713T120000Z" {
		t.Fatalf("ID = %q", result.ID)
	}
	if result.OutputDir != outputDir {
		t.Fatalf("OutputDir = %q, want %q", result.OutputDir, outputDir)
	}
	if result.ExitCode != 0 {
		t.Fatalf("ExitCode = %d, want 0", result.ExitCode)
	}
	if result.CanonicalStateMutated {
		t.Fatalf("CanonicalStateMutated = true, want false")
	}
	if len(errorsOut) != 0 {
		t.Fatalf("expected no errors, got %#v", errorsOut)
	}
	if hasDiagnosticCode(warnings, "taskjuggler_capacity_change_not_represented") {
		t.Fatalf("human weekly capacity should be represented, got warnings %#v", warnings)
	}
	assertFileContains(t, result.ProjectTJPPath, "weeklymax 6.00h")
	assertFileContains(t, result.StdoutPath, "run stdout")
	assertFileContains(t, result.StderrPath, "run stderr")
	assertFileContains(t, result.VersionPath, "TaskJuggler vFAKE")
	assertFileContains(t, result.MappingPath, `"replan_task_id": "task.op.1234"`)
	assertFileContains(t, result.MappingPath, `"taskjuggler_task_id": "task_op_1234"`)
	assertFileContains(t, result.TaskScheduleReportPath, `"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"`)
	assertFileContains(t, result.ManifestPath, `"taskjuggler_exit_code": 0`)
	assertFileContains(t, result.ManifestPath, `"mapping_path":`)
	assertFileContains(t, result.ManifestPath, `"kind": "taskjuggler_task_schedule_csv"`)
	assertFileContains(t, result.ManifestPath, `"path": "reports/replan-task-schedule.csv"`)
	assertFileContains(t, result.ManifestPath, `"canonical_state_mutated": false`)
}

func TestImportTaskJugglerResultDryRunBuildsScenarioCandidate(t *testing.T) {
	fakeTJ3 := writeFakeTaskJuggler(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "TaskJuggler vFAKE"
  exit 0
fi
cat > reports/replan-task-schedule.csv <<'REPORT'
"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"
"task_op_1234";"Implement validation command";"2026-07-06";"2026-07-13";1.0;0.0;"Ivan (resource_human_ivan)";"done"
REPORT
`)
	outputDir := filepath.Join(t.TempDir(), "run")
	_, _, _, runErr := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:       "../../examples/small-team",
		OutputDir:         outputDir,
		TaskJugglerBinary: fakeTJ3,
		Now:               fixedTaskJugglerRunTime(),
	})
	if runErr != nil {
		t.Fatalf("runTaskJugglerRunArtifacts returned error: %v", runErr)
	}

	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		ProjectPath: "../../examples/small-team",
		RunDir:      outputDir,
		DryRun:      true,
	})

	if err != nil {
		t.Fatalf("importTaskJugglerResult returned error: %v", err)
	}
	if !result.PreviewOnly {
		t.Fatalf("PreviewOnly = false, want true")
	}
	if result.Candidate.ID != "scenario.taskjuggler.20260713T120000Z" {
		t.Fatalf("Candidate.ID = %q", result.Candidate.ID)
	}
	if result.Candidate.SourceRunID != "taskjuggler.run.20260713T120000Z" {
		t.Fatalf("SourceRunID = %q", result.Candidate.SourceRunID)
	}
	if result.CandidateChanges != 2 {
		t.Fatalf("CandidateChanges = %d, want 2; changes %#v", result.CandidateChanges, result.Candidate.Changes)
	}
	assertCandidateChange(t, result.Candidate.Changes, "tasks.task.op.1234.planned_start", "2026-07-06")
	assertCandidateChange(t, result.Candidate.Changes, "tasks.task.op.1234.planned_finish", "2026-07-13")
}

func TestImportTaskJugglerResultRequiresDryRun(t *testing.T) {
	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		ProjectPath: "../../examples/small-team",
		RunDir:      t.TempDir(),
		DryRun:      false,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(result.Errors, "dry_run_required") {
		t.Fatalf("expected dry_run_required error, got %#v", result.Errors)
	}
}

func TestImportTaskJugglerResultConflictingModePrecedesOtherValidation(t *testing.T) {
	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		DryRun:         true,
		WriteCandidate: true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	want := []string{"conflicting_taskjuggler_import_modes"}
	if got := diagnosticCodeList(result.Errors); !reflect.DeepEqual(got, want) {
		t.Fatalf("error codes = %#v, want %#v", got, want)
	}
	if result.ManifestPath != "" || result.MappingPath != "" || result.ReportPath != "" {
		t.Fatalf("partial result advanced past mode validation: %#v", result)
	}
}

func diagnosticCodeList(diags []app.Diagnostic) []string {
	codes := make([]string, 0, len(diags))
	for _, diag := range diags {
		codes = append(codes, diag.Code)
	}
	return codes
}

func TestImportTaskJugglerResultWriteCandidateWritesScenarioAndProposal(t *testing.T) {
	projectDir := copyCmdProjectFixture(t)
	runDir := t.TempDir()
	writeTaskJugglerImportFixture(t, runDir, `"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"
"task_op_1234";"Implement validation command";"2026-07-06";"2026-07-13";1.0;0.0;"Ivan (resource_human_ivan)";"done"
`)

	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		ProjectPath:    projectDir,
		RunDir:         runDir,
		WriteCandidate: true,
		ActorID:        "actor.human.operator",
		Now:            fixedTaskJugglerRunTime(),
	})

	if err != nil {
		t.Fatalf("importTaskJugglerResult returned error: %v", err)
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if result.PreviewOnly {
		t.Fatalf("PreviewOnly = true, want false")
	}
	if len(result.WrittenFiles) != 2 {
		t.Fatalf("WrittenFiles len = %d, want 2: %#v", len(result.WrittenFiles), result.WrittenFiles)
	}
	scenarioPath := filepath.Join(projectDir, "scenarios", "scenario.taskjuggler.20260713T120000Z.yaml")
	proposalPath := filepath.Join(projectDir, "proposals", "open", "proposal.taskjuggler.20260713T120000Z.yaml")
	assertFileContains(t, scenarioPath, "id: scenario.taskjuggler.20260713T120000Z")
	assertFileContains(t, scenarioPath, "path: tasks.task.op.1234.planned_start")
	assertFileContains(t, proposalPath, "type: taskjuggler_schedule_candidate")
	assertFileContains(t, proposalPath, "requires_confirmation: true")
	assertFileContains(t, filepath.Join(projectDir, "canonical", "tasks.yaml"), "id: task.op.1234")
}

func TestImportTaskJugglerResultWriteCandidateRequiresActor(t *testing.T) {
	runDir := t.TempDir()
	writeTaskJugglerImportFixture(t, runDir, `"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"
"task_op_1234";"Implement validation command";"2026-07-06";"2026-07-13";1.0;0.0;"Ivan (resource_human_ivan)";"done"
`)

	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		ProjectPath:    "../../examples/small-team",
		RunDir:         runDir,
		WriteCandidate: true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(result.Errors, "actor_required") {
		t.Fatalf("expected actor_required error, got %#v", result.Errors)
	}
}

func TestImportTaskJugglerResultReportsInvalidDate(t *testing.T) {
	runDir := t.TempDir()
	writeTaskJugglerImportFixture(t, runDir, `"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"
"task_op_1234";"Implement validation command";"not-a-date";"2026-07-13";1.0;0.0;"Ivan (resource_human_ivan)";"done"
`)

	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		ProjectPath: "../../examples/small-team",
		RunDir:      runDir,
		DryRun:      true,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(result.Errors, "taskjuggler_report_date_invalid") {
		t.Fatalf("expected taskjuggler_report_date_invalid error, got %#v", result.Errors)
	}
}

func TestRunTaskJugglerRunArtifactsReportsMissingBinary(t *testing.T) {
	result, warnings, errorsOut, err := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:       "../../examples/small-team",
		OutputDir:         filepath.Join(t.TempDir(), "run"),
		TaskJugglerBinary: "replan-test-missing-tj3",
		Now:               fixedTaskJugglerRunTime(),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if result.TaskJugglerPath != "" {
		t.Fatalf("TaskJugglerPath = %q, want empty", result.TaskJugglerPath)
	}
	if hasDiagnosticCode(warnings, "taskjuggler_binary_not_found") {
		t.Fatalf("missing binary must be an error for taskjuggler run, got warnings %#v", warnings)
	}
	if !hasDiagnosticCode(errorsOut, "taskjuggler_binary_not_found") {
		t.Fatalf("expected taskjuggler_binary_not_found error, got %#v", errorsOut)
	}
	if _, statErr := os.Stat(result.OutputDir); !os.IsNotExist(statErr) {
		t.Fatalf("output dir should not be created when binary is missing, stat error %v", statErr)
	}
}

func TestRunTaskJugglerRunArtifactsFailsBeforeRunForStrictUnrepresentedAssumptions(t *testing.T) {
	fakeTJ3 := writeFakeTaskJuggler(t, `#!/bin/sh
echo should not run
`)
	outputDir := filepath.Join(t.TempDir(), "run")

	result, _, errorsOut, err := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:                  "../../examples/small-team",
		ScenarioID:                   "scenario.openai-quota-300k",
		OutputDir:                    outputDir,
		TaskJugglerBinary:            fakeTJ3,
		UnrepresentedAssumptionsMode: "error",
		Now:                          fixedTaskJugglerRunTime(),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(errorsOut, "taskjuggler_capacity_change_not_represented") {
		t.Fatalf("expected taskjuggler_capacity_change_not_represented error, got %#v", errorsOut)
	}
	if _, statErr := os.Stat(result.OutputDir); !os.IsNotExist(statErr) {
		t.Fatalf("output dir should not be created when export fails, stat error %v", statErr)
	}
}

func TestRunTaskJugglerRunArtifactsReportsTaskJugglerFailure(t *testing.T) {
	fakeTJ3 := writeFakeTaskJuggler(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "TaskJuggler vFAKE"
  exit 0
fi
echo failed stdout
echo failed stderr >&2
exit 3
`)
	outputDir := filepath.Join(t.TempDir(), "run")

	result, _, errorsOut, err := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:       "../../examples/small-team",
		OutputDir:         outputDir,
		TaskJugglerBinary: fakeTJ3,
		Now:               fixedTaskJugglerRunTime(),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if result.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", result.ExitCode)
	}
	if !hasDiagnosticCode(errorsOut, "taskjuggler_run_failed") {
		t.Fatalf("expected taskjuggler_run_failed error, got %#v", errorsOut)
	}
	assertFileContains(t, result.StdoutPath, "failed stdout")
	assertFileContains(t, result.StderrPath, "failed stderr")
	assertFileContains(t, result.ManifestPath, `"taskjuggler_exit_code": 3`)
	assertFileContains(t, result.ManifestPath, `"code": "taskjuggler_run_failed"`)
}

func TestRunTaskJugglerRunArtifactsRequiresTaskScheduleReport(t *testing.T) {
	fakeTJ3 := writeFakeTaskJuggler(t, `#!/bin/sh
if [ "$1" = "--version" ]; then
  echo "TaskJuggler vFAKE"
  exit 0
fi
exit 0
`)
	outputDir := filepath.Join(t.TempDir(), "run")

	result, _, errorsOut, err := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:       "../../examples/small-team",
		OutputDir:         outputDir,
		TaskJugglerBinary: fakeTJ3,
		Now:               fixedTaskJugglerRunTime(),
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnosticCode(errorsOut, "taskjuggler_report_missing") {
		t.Fatalf("expected taskjuggler_report_missing error, got %#v", errorsOut)
	}
	assertFileContains(t, result.MappingPath, `"version": 1`)
	assertFileContains(t, result.ManifestPath, `"code": "taskjuggler_report_missing"`)
}

func TestDefaultTaskJugglerRunOutputDirUsesScenarioIDAsScope(t *testing.T) {
	got := defaultTaskJugglerRunOutputDir("project", "scenario.ivan-capacity-6", "20260713T120000Z")
	want := filepath.Join("project", "exports", "taskjuggler", "scenario.ivan-capacity-6", "run.20260713T120000Z")
	if got != want {
		t.Fatalf("defaultTaskJugglerRunOutputDir = %q, want %q", got, want)
	}
}

func TestRunTaskJugglerValidationSkipsWhenBinaryMissing(t *testing.T) {
	result, warnings, errorsOut, err := runTaskJugglerValidation(context.Background(), taskJugglerValidationRequest{
		ProjectPath:       "../../examples/small-team",
		TaskJugglerBinary: "replan-test-missing-tj3",
	})

	if err != nil {
		t.Fatalf("runTaskJugglerValidation returned error: %v", err)
	}
	if !result.Skipped {
		t.Fatalf("Skipped = false, want true")
	}
	if result.Validated {
		t.Fatalf("Validated = true, want false")
	}
	if !hasDiagnosticCode(warnings, "taskjuggler_binary_not_found") {
		t.Fatalf("expected taskjuggler_binary_not_found warning, got %#v", warnings)
	}
	if len(errorsOut) != 0 {
		t.Fatalf("expected no errors, got %#v", errorsOut)
	}
}

func TestRunTaskJugglerValidationRunsTaskJugglerBinary(t *testing.T) {
	fakeTJ3 := writeFakeTaskJuggler(t, `#!/bin/sh
grep 'weeklymax 6.00h' "$1" >/dev/null
`)

	result, warnings, errorsOut, err := runTaskJugglerValidation(context.Background(), taskJugglerValidationRequest{
		ProjectPath:                  "../../examples/small-team",
		ScenarioID:                   "scenario.ivan-capacity-6",
		TaskJugglerBinary:            fakeTJ3,
		UnrepresentedAssumptionsMode: "error",
	})

	if err != nil {
		t.Fatalf("runTaskJugglerValidation returned error: %v", err)
	}
	if result.Skipped {
		t.Fatalf("Skipped = true, want false")
	}
	if !result.Validated {
		t.Fatalf("Validated = false, want true")
	}
	if len(errorsOut) != 0 {
		t.Fatalf("expected no errors, got %#v", errorsOut)
	}
	if hasDiagnosticCode(warnings, "taskjuggler_capacity_change_not_represented") {
		t.Fatalf("human weekly capacity should be represented, got warnings %#v", warnings)
	}
}

func TestRunTaskJugglerValidationReportsTaskJugglerFailure(t *testing.T) {
	fakeTJ3 := writeFakeTaskJuggler(t, `#!/bin/sh
echo validation failed >&2
exit 3
`)

	result, _, errorsOut, err := runTaskJugglerValidation(context.Background(), taskJugglerValidationRequest{
		ProjectPath:       "../../examples/small-team",
		TaskJugglerBinary: fakeTJ3,
	})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if result.Validated {
		t.Fatalf("Validated = true, want false")
	}
	if !strings.Contains(result.Output, "validation failed") {
		t.Fatalf("expected TaskJuggler output to be captured, got %q", result.Output)
	}
	if !hasDiagnosticCode(errorsOut, "taskjuggler_validation_failed") {
		t.Fatalf("expected taskjuggler_validation_failed error, got %#v", errorsOut)
	}
}

func writeFakeTaskJuggler(t *testing.T, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tj3")
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake tj3: %v", err)
	}
	return path
}

func fixedTaskJugglerRunTime() time.Time {
	return time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC)
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(content), want) {
		t.Fatalf("%s does not contain %q; content:\n%s", path, want, string(content))
	}
}

func assertCandidateChange(t *testing.T, changes []TaskJugglerCandidateChange, path, value string) {
	t.Helper()
	for _, change := range changes {
		if change.Path == path && change.Value == value {
			return
		}
	}
	t.Fatalf("expected candidate change %s=%s, got %#v", path, value, changes)
}

func writeTaskJugglerImportFixture(t *testing.T, runDir, report string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(runDir, "reports"), 0o755); err != nil {
		t.Fatalf("mkdir reports: %v", err)
	}
	if err := writeJSONFile(filepath.Join(runDir, "run.manifest.json"), TaskJugglerRunManifest{
		ID:          "taskjuggler.run.20260713T120000Z",
		MappingPath: filepath.Join(runDir, "taskjuggler.mapping.json"),
		Reports: []TaskJugglerRunReportArtifact{{
			Kind: "taskjuggler_task_schedule_csv",
			Path: filepath.Join("reports", "replan-task-schedule.csv"),
		}},
	}); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	if err := writeJSONFile(filepath.Join(runDir, "taskjuggler.mapping.json"), app.TaskJugglerExportMapping{
		Version:     1,
		ProjectPath: "../../examples/small-team",
		TaskIDMap: []app.TaskJugglerTaskIDMapping{{
			RePlanTaskID:      "task.op.1234",
			TaskJugglerTaskID: "task_op_1234",
		}},
	}); err != nil {
		t.Fatalf("write mapping: %v", err)
	}
	if err := writeTextFile(filepath.Join(runDir, "reports", "replan-task-schedule.csv"), report); err != nil {
		t.Fatalf("write report: %v", err)
	}
}

func tempOutputFiles(t *testing.T) (*os.File, *os.File) {
	t.Helper()
	dir := t.TempDir()
	stdout, err := os.Create(filepath.Join(dir, "stdout.txt"))
	if err != nil {
		t.Fatalf("create stdout: %v", err)
	}
	stderr, err := os.Create(filepath.Join(dir, "stderr.txt"))
	if err != nil {
		_ = stdout.Close()
		t.Fatalf("create stderr: %v", err)
	}
	return stdout, stderr
}

func readCLIEnvelope(t *testing.T, path string) cliTestEnvelope {
	t.Helper()
	var env cliTestEnvelope
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read envelope: %v", err)
	}
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("parse envelope %s: %v\n%s", path, err, data)
	}
	return env
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file %s: %v", path, err)
	}
	return string(data)
}

func resultObject(t *testing.T, env cliTestEnvelope) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatalf("parse result object: %v\n%s", err, env.Result)
	}
	return result
}

func assertResultField(t *testing.T, env cliTestEnvelope, key string, want any) {
	t.Helper()
	result := resultObject(t, env)
	got, ok := result[key]
	if !ok {
		t.Fatalf("result missing field %q: %#v", key, result)
	}
	switch want := want.(type) {
	case bool:
		if got != want {
			t.Fatalf("result[%s] = %#v, want %#v", key, got, want)
		}
	case string:
		if got != want {
			t.Fatalf("result[%s] = %#v, want %#v", key, got, want)
		}
	default:
		t.Fatalf("unsupported assertResultField type %T", want)
	}
}

func assertResultNumberField(t *testing.T, env cliTestEnvelope, key string, want float64) {
	t.Helper()
	result := resultObject(t, env)
	got, ok := result[key].(float64)
	if !ok {
		t.Fatalf("result[%s] = %#v, want number", key, result[key])
	}
	if got != want {
		t.Fatalf("result[%s] = %v, want %v", key, got, want)
	}
}

func assertResultArrayNotEmpty(t *testing.T, env cliTestEnvelope, key string) {
	t.Helper()
	result := resultObject(t, env)
	got, ok := result[key].([]any)
	if !ok {
		t.Fatalf("result[%s] = %#v, want array", key, result[key])
	}
	if len(got) == 0 {
		t.Fatalf("result[%s] is empty, want non-empty", key)
	}
}

func assertResultArrayField(t *testing.T, env cliTestEnvelope, key string) {
	t.Helper()
	result := resultObject(t, env)
	if _, ok := result[key].([]any); !ok {
		t.Fatalf("result[%s] = %#v, want array", key, result[key])
	}
}

func assertResultObjectField(t *testing.T, env cliTestEnvelope, key string) {
	t.Helper()
	result := resultObject(t, env)
	if _, ok := result[key].(map[string]any); !ok {
		t.Fatalf("result[%s] = %#v, want object", key, result[key])
	}
}

func copyCmdProjectFixture(t *testing.T) string {
	t.Helper()
	return copyCmdProjectFixtureFrom(t, "../../examples/small-team", "small-team")
}

func copyCmdProjectFixtureFrom(t *testing.T, sourcePath, targetName string) string {
	t.Helper()
	src := filepath.Clean(sourcePath)
	dst := filepath.Join(t.TempDir(), targetName)
	if err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	}); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func setCmdProjectOpenProjectIDs(t *testing.T, projectPath, policy string) {
	t.Helper()
	path := filepath.Join(projectPath, ".replan", "project.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(string(data), "  timezone:", "  openproject_ids: "+policy+"\n  timezone:", 1)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func removeCmdTrackingProposalArtifacts(t *testing.T, projectPath string) {
	t.Helper()
	for _, dir := range []string{"open", "accepted", "rejected"} {
		matches, err := filepath.Glob(filepath.Join(projectPath, "proposals", dir, "proposal.tracking.resource_demand-tracking-proposals-*.yaml"))
		if err != nil {
			t.Fatalf("glob tracking proposal artifacts: %v", err)
		}
		for _, match := range matches {
			if err := os.Remove(match); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("remove tracking proposal artifact: %v", err)
			}
		}
	}
}

func markCmdTrackingProposalDemandsStale(t *testing.T, projectPath string) {
	t.Helper()
	path := filepath.Join(projectPath, "canonical", "resource_demands.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read resource demands: %v", err)
	}
	text := string(data)
	text = replaceCmdOnceForTest(t, text,
		"id: resource_demand.tracking-proposals.maintainer\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.human.maintainer\n      resource_class: human\n      role: implementer\n      unit: hours\n      planned_amount: 3\n      criticality: required",
		"id: resource_demand.tracking-proposals.maintainer\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.human.maintainer\n      resource_class: human\n      role: implementer\n      unit: hours\n      planned_amount: 3\n      remaining_amount: 3\n      criticality: required")
	text = replaceCmdOnceForTest(t, text,
		"id: resource_demand.tracking-proposals.codex\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.ai.codex.local\n      resource_class: ai_agent\n      role: ai_coding_agent\n      unit: agent_hours\n      planned_amount: 3\n      criticality: preferred",
		"id: resource_demand.tracking-proposals.codex\n      task_ref:\n        kind: task\n        id: task.self.add-tracking-proposal-candidates\n      resource_ref:\n        kind: resource\n        id: resource.ai.codex.local\n      resource_class: ai_agent\n      role: ai_coding_agent\n      unit: agent_hours\n      planned_amount: 3\n      remaining_amount: 3\n      criticality: preferred")
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write resource demands: %v", err)
	}
}

func replaceCmdOnceForTest(t *testing.T, text, old, new string) string {
	t.Helper()
	if !strings.Contains(text, old) {
		t.Fatalf("fixture text not found:\n%s", old)
	}
	return strings.Replace(text, old, new, 1)
}

func hasDiagnosticCode(diags []app.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}
