package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
)

type TaskJugglerValidationResult struct {
	ProjectPath     string `json:"project_path"`
	ScenarioID      string `json:"scenario_id,omitempty"`
	TaskJugglerPath string `json:"taskjuggler_path,omitempty"`
	ArtifactPath    string `json:"artifact_path,omitempty"`
	Skipped         bool   `json:"skipped"`
	Validated       bool   `json:"validated"`
	Output          string `json:"output,omitempty"`
}

type TaskJugglerRunResult struct {
	ID                     string   `json:"id"`
	ProjectPath            string   `json:"project_path"`
	ScenarioID             string   `json:"scenario_id,omitempty"`
	OutputDir              string   `json:"output_dir"`
	TaskJugglerPath        string   `json:"taskjuggler_path,omitempty"`
	TaskJugglerVersion     string   `json:"taskjuggler_version,omitempty"`
	ProjectTJPPath         string   `json:"project_tjp_path,omitempty"`
	ManifestPath           string   `json:"manifest_path,omitempty"`
	StdoutPath             string   `json:"stdout_path,omitempty"`
	StderrPath             string   `json:"stderr_path,omitempty"`
	VersionPath            string   `json:"version_path,omitempty"`
	MappingPath            string   `json:"mapping_path,omitempty"`
	TaskScheduleReportPath string   `json:"task_schedule_report_path,omitempty"`
	ExitCode               int      `json:"exit_code"`
	Artifacts              []string `json:"artifacts"`
	CanonicalStateMutated  bool     `json:"canonical_state_mutated"`
}

type TaskJugglerRunReportArtifact struct {
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type TaskJugglerRunManifest struct {
	ID                       string                         `json:"id"`
	Command                  string                         `json:"command"`
	ProjectPath              string                         `json:"project_path"`
	ScenarioID               string                         `json:"scenario_id,omitempty"`
	OutputDir                string                         `json:"output_dir"`
	TaskJugglerPath          string                         `json:"taskjuggler_path"`
	TaskJugglerVersion       string                         `json:"taskjuggler_version,omitempty"`
	TaskJugglerExitCode      int                            `json:"taskjuggler_exit_code"`
	UnrepresentedAssumptions string                         `json:"unrepresented_assumptions"`
	ProjectTJPPath           string                         `json:"project_tjp_path"`
	ProjectTJPSHA256         string                         `json:"project_tjp_sha256"`
	StdoutPath               string                         `json:"stdout_path"`
	StderrPath               string                         `json:"stderr_path"`
	VersionPath              string                         `json:"version_path"`
	MappingPath              string                         `json:"mapping_path,omitempty"`
	MappingSHA256            string                         `json:"mapping_sha256,omitempty"`
	Reports                  []TaskJugglerRunReportArtifact `json:"reports"`
	CreatedAt                string                         `json:"created_at"`
	CanonicalStateMutated    bool                           `json:"canonical_state_mutated"`
	Warnings                 []app.Diagnostic               `json:"warnings"`
	Errors                   []app.Diagnostic               `json:"errors"`
}

func runDev(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing dev subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "validate-taskjuggler":
		return runDevValidateTaskJuggler(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown dev subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runTaskJuggler(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing taskjuggler subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "run":
		return runTaskJugglerRun(args[1:], stdout, stderr)
	case "import-result":
		return runTaskJugglerImportResult(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown taskjuggler subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runDevValidateTaskJuggler(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("dev validate-taskjuggler")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	format := fs.String("format", "json", "output format: json, markdown or text")
	taskJugglerBinary := fs.String("taskjuggler-bin", "tj3", "TaskJuggler binary to run")
	unrepresentedAssumptions := fs.String("unrepresented-assumptions", "warn", "how to handle scenario assumptions not represented in TaskJuggler .tjp: warn or error")
	keepTemp := fs.Bool("keep-temp", false, "keep generated temporary .tjp artifact")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	*format = strings.ToLower(strings.TrimSpace(*format))
	if *format == "" {
		*format = "json"
	}
	if *format != "json" && *format != "markdown" && *format != "text" {
		env := envelope{
			OK:          false,
			Command:     "dev validate-taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	*unrepresentedAssumptions = strings.ToLower(strings.TrimSpace(*unrepresentedAssumptions))
	if *unrepresentedAssumptions == "" {
		*unrepresentedAssumptions = "warn"
	}
	if *unrepresentedAssumptions != "warn" && *unrepresentedAssumptions != "error" {
		env := envelope{
			OK:          false,
			Command:     "dev validate-taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_unrepresented_assumptions_mode",
				Message:  "supported --unrepresented-assumptions modes are warn and error",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}

	result, warnings, errorsOut, err := runTaskJugglerValidation(context.Background(), taskJugglerValidationRequest{
		ProjectPath:                  *projectPath,
		ScenarioID:                   *scenarioID,
		TaskJugglerBinary:            *taskJugglerBinary,
		UnrepresentedAssumptionsMode: *unrepresentedAssumptions,
		KeepTemp:                     *keepTemp,
	})
	env := envelope{
		OK:          err == nil,
		Command:     "dev validate-taskjuggler",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    warnings,
		Errors:      errorsOut,
	}
	if printErr := printEnvelope(stdout, *format, env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err == nil {
		return exitSuccess
	}
	if errors.Is(err, app.ErrValidationFailed) {
		return validationFailureExit(errorsOut)
	}
	return exitGeneralError
}

type taskJugglerValidationRequest struct {
	ProjectPath                  string
	ScenarioID                   string
	TaskJugglerBinary            string
	UnrepresentedAssumptionsMode string
	KeepTemp                     bool
}

type taskJugglerRunRequest struct {
	ProjectPath                  string
	ScenarioID                   string
	OutputDir                    string
	TaskJugglerBinary            string
	UnrepresentedAssumptionsMode string
	Now                          time.Time
}

func runTaskJugglerRun(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("taskjuggler run")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	outputDir := fs.String("output-dir", "", "directory for TaskJuggler run artifacts")
	format := fs.String("format", "json", "output format: json, markdown or text")
	taskJugglerBinary := fs.String("taskjuggler-bin", "tj3", "TaskJuggler binary to run")
	unrepresentedAssumptions := fs.String("unrepresented-assumptions", "warn", "how to handle scenario assumptions not represented in TaskJuggler .tjp: warn or error")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	*format = strings.ToLower(strings.TrimSpace(*format))
	if *format == "" {
		*format = "json"
	}
	if *format != "json" && *format != "markdown" && *format != "text" {
		env := envelope{
			OK:          false,
			Command:     "taskjuggler run",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	*unrepresentedAssumptions = strings.ToLower(strings.TrimSpace(*unrepresentedAssumptions))
	if *unrepresentedAssumptions == "" {
		*unrepresentedAssumptions = "warn"
	}
	if *unrepresentedAssumptions != "warn" && *unrepresentedAssumptions != "error" {
		env := envelope{
			OK:          false,
			Command:     "taskjuggler run",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_unrepresented_assumptions_mode",
				Message:  "supported --unrepresented-assumptions modes are warn and error",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	// The default run directory is inside the project; an explicit one is
	// relative to the working directory.
	requestedOutputDir := *projectPath
	if strings.TrimSpace(*outputDir) != "" {
		requestedOutputDir = strings.TrimSpace(*outputDir)
	}
	if code, done := refuseBindingsExportInsideProject("taskjuggler run", *projectPath, stdout, stderr, requestedOutputDir); done {
		return code
	}

	result, warnings, errorsOut, err := runTaskJugglerRunArtifacts(context.Background(), taskJugglerRunRequest{
		ProjectPath:                  *projectPath,
		ScenarioID:                   *scenarioID,
		OutputDir:                    *outputDir,
		TaskJugglerBinary:            *taskJugglerBinary,
		UnrepresentedAssumptionsMode: *unrepresentedAssumptions,
		Now:                          time.Now(),
	})
	env := envelope{
		OK:          err == nil,
		Command:     "taskjuggler run",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    warnings,
		Errors:      errorsOut,
	}
	if printErr := printEnvelope(stdout, *format, env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err == nil {
		return exitSuccess
	}
	if errors.Is(err, app.ErrValidationFailed) {
		return validationFailureExit(errorsOut)
	}
	return exitGeneralError
}

func runTaskJugglerRunArtifacts(ctx context.Context, req taskJugglerRunRequest) (TaskJugglerRunResult, []app.Diagnostic, []app.Diagnostic, error) {
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	timestamp := now.UTC().Format("20060102T150405Z")
	runID := "taskjuggler.run." + timestamp
	outputDir := strings.TrimSpace(req.OutputDir)
	if outputDir == "" {
		outputDir = defaultTaskJugglerRunOutputDir(req.ProjectPath, req.ScenarioID, timestamp)
	}
	result := TaskJugglerRunResult{
		ID:                    runID,
		ProjectPath:           req.ProjectPath,
		ScenarioID:            req.ScenarioID,
		OutputDir:             outputDir,
		ExitCode:              -1,
		Artifacts:             []string{},
		CanonicalStateMutated: false,
	}
	warnings := []app.Diagnostic{}
	errorsOut := []app.Diagnostic{}

	exportResult, exportErr := app.NewExportService(newStore(req.ProjectPath)).TaskJuggler(ctx, app.ExportTaskJugglerRequest{
		ScenarioID:                   req.ScenarioID,
		UnrepresentedAssumptionsMode: req.UnrepresentedAssumptionsMode,
	})
	warnings = append(warnings, exportResult.Warnings...)
	errorsOut = append(errorsOut, exportResult.Errors...)
	if exportErr != nil {
		return result, warnings, errorsOut, exportErr
	}

	taskJugglerPath, lookErr := exec.LookPath(req.TaskJugglerBinary)
	if lookErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_binary_not_found",
			Message:  fmt.Sprintf("TaskJuggler binary %q was not found; run artifacts were not created", req.TaskJugglerBinary),
			Severity: "error",
		})
		return result, warnings, errorsOut, app.ErrValidationFailed
	}
	result.TaskJugglerPath = taskJugglerPath

	if mkErr := os.MkdirAll(outputDir, 0o755); mkErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_output_dir_failed",
			Message:  fmt.Sprintf("failed to create TaskJuggler run output directory: %v", mkErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, mkErr
	}

	projectTJPPath := filepath.Join(outputDir, "project.tjp")
	stdoutPath := filepath.Join(outputDir, "stdout.txt")
	stderrPath := filepath.Join(outputDir, "stderr.txt")
	versionPath := filepath.Join(outputDir, "taskjuggler.version.txt")
	mappingPath := filepath.Join(outputDir, "taskjuggler.mapping.json")
	reportsDir := filepath.Join(outputDir, "reports")
	taskScheduleReportPath := filepath.Join(reportsDir, "replan-task-schedule.csv")
	manifestPath := filepath.Join(outputDir, "run.manifest.json")
	result.ProjectTJPPath = projectTJPPath
	result.StdoutPath = stdoutPath
	result.StderrPath = stderrPath
	result.VersionPath = versionPath
	result.MappingPath = mappingPath
	result.TaskScheduleReportPath = taskScheduleReportPath
	result.ManifestPath = manifestPath
	result.Artifacts = []string{projectTJPPath, stdoutPath, stderrPath, versionPath, mappingPath, taskScheduleReportPath, manifestPath}

	if writeErr := writeTextFile(projectTJPPath, exportResult.Content); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_project_write_failed",
			Message:  fmt.Sprintf("failed to write generated TaskJuggler project: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	if writeErr := writeJSONFile(mappingPath, exportResult.Mapping); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_mapping_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler mapping artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	if mkErr := os.MkdirAll(reportsDir, 0o755); mkErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_reports_dir_failed",
			Message:  fmt.Sprintf("failed to create TaskJuggler reports directory: %v", mkErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, mkErr
	}

	versionOutput, versionErr := exec.CommandContext(ctx, taskJugglerPath, "--version").CombinedOutput()
	result.TaskJugglerVersion = strings.TrimSpace(string(versionOutput))
	if versionErr != nil {
		warnings = append(warnings, app.Diagnostic{
			Code:     "taskjuggler_version_failed",
			Message:  fmt.Sprintf("failed to read TaskJuggler version: %v", versionErr),
			Severity: "warning",
		})
	}
	if writeErr := writeTextFile(versionPath, string(versionOutput)); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_version_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler version artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, taskJugglerPath, "project.tjp")
	cmd.Dir = outputDir
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()
	result.ExitCode = 0
	if runErr != nil {
		result.ExitCode = 1
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
		}
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_failed",
			Message:  fmt.Sprintf("TaskJuggler run failed: %v", runErr),
			Severity: "error",
		})
	}

	if writeErr := writeTextFile(stdoutPath, stdoutBuf.String()); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_stdout_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler stdout artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	if writeErr := writeTextFile(stderrPath, stderrBuf.String()); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_stderr_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler stderr artifact: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}
	reportSHA256 := ""
	if runErr == nil {
		reportContent, readErr := os.ReadFile(taskScheduleReportPath)
		if readErr != nil {
			errorsOut = append(errorsOut, app.Diagnostic{
				Code:     "taskjuggler_report_missing",
				Message:  fmt.Sprintf("expected TaskJuggler task schedule report was not created: %v", readErr),
				Severity: "error",
			})
			runErr = app.ErrValidationFailed
		} else {
			reportSHA256 = sha256Hex(reportContent)
		}
	}
	mappingSHA256 := ""
	if mappingContent, readErr := os.ReadFile(mappingPath); readErr == nil {
		mappingSHA256 = sha256Hex(mappingContent)
	}
	reports := []TaskJugglerRunReportArtifact{}
	if reportSHA256 != "" {
		reports = append(reports, TaskJugglerRunReportArtifact{
			Kind:   "taskjuggler_task_schedule_csv",
			Path:   filepath.Join("reports", "replan-task-schedule.csv"),
			SHA256: reportSHA256,
		})
	}

	manifest := TaskJugglerRunManifest{
		ID:                       runID,
		Command:                  "taskjuggler run",
		ProjectPath:              req.ProjectPath,
		ScenarioID:               req.ScenarioID,
		OutputDir:                outputDir,
		TaskJugglerPath:          taskJugglerPath,
		TaskJugglerVersion:       result.TaskJugglerVersion,
		TaskJugglerExitCode:      result.ExitCode,
		UnrepresentedAssumptions: req.UnrepresentedAssumptionsMode,
		ProjectTJPPath:           projectTJPPath,
		ProjectTJPSHA256:         sha256Hex([]byte(exportResult.Content)),
		StdoutPath:               stdoutPath,
		StderrPath:               stderrPath,
		VersionPath:              versionPath,
		MappingPath:              mappingPath,
		MappingSHA256:            mappingSHA256,
		Reports:                  reports,
		CreatedAt:                now.Format(time.RFC3339),
		CanonicalStateMutated:    false,
		Warnings:                 warnings,
		Errors:                   errorsOut,
	}
	if writeErr := writeJSONFile(manifestPath, manifest); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_run_manifest_write_failed",
			Message:  fmt.Sprintf("failed to write TaskJuggler run manifest: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}

	if runErr != nil {
		return result, warnings, errorsOut, app.ErrValidationFailed
	}
	return result, warnings, errorsOut, nil
}

func importTaskJugglerResult(ctx context.Context, req taskJugglerImportRequest) (TaskJugglerImportResult, error) {
	result := TaskJugglerImportResult{
		RunDir:        req.RunDir,
		PreviewOnly:   req.DryRun,
		PlannedWrites: []app.PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []app.Diagnostic{},
		Errors:        []app.Diagnostic{},
		Candidate: TaskJugglerScenarioCandidate{
			Source:  "taskjuggler",
			Changes: []TaskJugglerCandidateChange{},
		},
	}
	state := taskJugglerImportState{ctx: ctx, req: req, result: &result}
	steps := []func() error{
		state.validateRequest,
		state.loadManifest,
		state.loadMapping,
		state.loadReport,
		state.loadWorkspace,
		state.buildCandidateChanges,
		state.preparePreviews,
		state.writeCandidate,
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return result, err
		}
	}
	return result, nil
}

func resolveRunArtifactPath(runDir, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(runDir, path)
}

func readJSONFile(path string, value any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(content, value)
}

func defaultTaskJugglerRunOutputDir(projectPath, scenarioID, timestamp string) string {
	scope := "current"
	if strings.TrimSpace(scenarioID) != "" {
		scope = safeTaskJugglerPathSegment(scenarioID)
		if !strings.HasPrefix(scope, "scenario.") {
			scope = "scenario." + scope
		}
	}
	return filepath.Join(projectPath, "exports", "taskjuggler", scope, "run."+timestamp)
}

func safeTaskJugglerPathSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.' || r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	if b.Len() == 0 {
		return "unnamed"
	}
	return b.String()
}

func runTaskJugglerValidation(ctx context.Context, req taskJugglerValidationRequest) (TaskJugglerValidationResult, []app.Diagnostic, []app.Diagnostic, error) {
	result := TaskJugglerValidationResult{
		ProjectPath: req.ProjectPath,
		ScenarioID:  req.ScenarioID,
	}
	warnings := []app.Diagnostic{}
	errorsOut := []app.Diagnostic{}

	exportResult, exportErr := app.NewExportService(newStore(req.ProjectPath)).TaskJuggler(ctx, app.ExportTaskJugglerRequest{
		ScenarioID:                   req.ScenarioID,
		UnrepresentedAssumptionsMode: req.UnrepresentedAssumptionsMode,
	})
	warnings = append(warnings, exportResult.Warnings...)
	errorsOut = append(errorsOut, exportResult.Errors...)
	if exportErr != nil {
		return result, warnings, errorsOut, exportErr
	}

	taskJugglerPath, lookErr := exec.LookPath(req.TaskJugglerBinary)
	if lookErr != nil {
		result.Skipped = true
		warnings = append(warnings, app.Diagnostic{
			Code:     "taskjuggler_binary_not_found",
			Message:  fmt.Sprintf("TaskJuggler binary %q was not found; generated .tjp runtime validation was skipped", req.TaskJugglerBinary),
			Severity: "warning",
		})
		return result, warnings, errorsOut, nil
	}
	result.TaskJugglerPath = taskJugglerPath

	tmpDir, createErr := os.MkdirTemp("", "replan-taskjuggler-*")
	if createErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_tempfile_failed",
			Message:  fmt.Sprintf("failed to create temporary TaskJuggler directory: %v", createErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, createErr
	}
	tmpName := filepath.Join(tmpDir, "project.tjp")
	reportsDir := filepath.Join(tmpDir, "reports")
	if req.KeepTemp {
		result.ArtifactPath = tmpName
	}
	cleanup := !req.KeepTemp
	defer func() {
		if cleanup {
			_ = os.RemoveAll(tmpDir)
		}
	}()
	if mkErr := os.MkdirAll(reportsDir, 0o755); mkErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_tempfile_failed",
			Message:  fmt.Sprintf("failed to create temporary TaskJuggler reports directory: %v", mkErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, mkErr
	}
	if writeErr := writeTextFile(tmpName, exportResult.Content); writeErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_tempfile_failed",
			Message:  fmt.Sprintf("failed to write temporary .tjp file: %v", writeErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, writeErr
	}

	cmd := exec.CommandContext(ctx, taskJugglerPath, "project.tjp")
	cmd.Dir = tmpDir
	output, runErr := cmd.CombinedOutput()
	result.Output = string(output)
	if runErr != nil {
		errorsOut = append(errorsOut, app.Diagnostic{
			Code:     "taskjuggler_validation_failed",
			Message:  fmt.Sprintf("TaskJuggler validation failed: %v", runErr),
			Severity: "error",
		})
		return result, warnings, errorsOut, app.ErrValidationFailed
	}
	result.Validated = true
	return result, warnings, errorsOut, nil
}

func runExportTaskJuggler(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("export taskjuggler")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	output := fs.String("output", "", "path to write TaskJuggler .tjp export; stdout when omitted")
	manifest := fs.String("manifest", "", "path to write export manifest JSON; used only with --output")
	format := fs.String("format", "text", "output format: text or json")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	unrepresentedAssumptions := fs.String("unrepresented-assumptions", "warn", "how to handle scenario assumptions not represented in TaskJuggler .tjp: warn or error")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *manifest != "" && *output == "" {
		return printExportManifestRequiresOutput(stdout, stderr, "export taskjuggler", *projectPath)
	}
	*format = strings.ToLower(*format)
	if *format != "text" && *format != "json" {
		env := envelope{
			OK:          false,
			Command:     "export taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are text and json",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	*unrepresentedAssumptions = strings.ToLower(strings.TrimSpace(*unrepresentedAssumptions))
	if *unrepresentedAssumptions == "" {
		*unrepresentedAssumptions = "warn"
	}
	if *unrepresentedAssumptions != "warn" && *unrepresentedAssumptions != "error" {
		env := envelope{
			OK:          false,
			Command:     "export taskjuggler",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_unrepresented_assumptions_mode",
				Message:  "supported --unrepresented-assumptions modes are warn and error",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	if code, done := refuseBindingsExportInsideProject("export taskjuggler", *projectPath, stdout, stderr, *output, *manifest); done {
		return code
	}
	result, err := app.NewExportService(newStore(*projectPath)).TaskJuggler(context.Background(), app.ExportTaskJugglerRequest{
		ScenarioID:                   *scenarioID,
		UnrepresentedAssumptionsMode: *unrepresentedAssumptions,
	})
	if err != nil && diagnosticsContainNotFound(result.Errors) {
		env := envelope{OK: false, Command: "export taskjuggler", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
		if printErr := printEnvelope(stdout, *format, env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitNotFound
	}
	if *output != "" && err == nil {
		content := []byte(result.Content)
		if writeErr := writeTextFile(*output, result.Content); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write export: %v\n", writeErr)
			return exitGeneralError
		}
		if *manifest != "" {
			if writeErr := writeExportManifestFile(*manifest, ExportManifest{
				ID:          result.Metadata.ID,
				Command:     "export taskjuggler",
				Format:      "taskjuggler",
				ProjectPath: *projectPath,
				OutputPath:  *output,
				ScenarioID:  result.Scenario,
				SHA256:      sha256Hex(content),
				CreatedAt:   result.Metadata.ExportedAt,
				Warnings:    result.Warnings,
				Errors:      result.Errors,
			}); writeErr != nil {
				fmt.Fprintf(stderr, "failed to write manifest: %v\n", writeErr)
				return exitGeneralError
			}
		}
	}
	if *format == "json" {
		if printErr := printExportTaskJugglerJSON(stdout, result, *output); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
	} else if *output == "" {
		if _, printErr := fmt.Fprint(stdout, result.Content); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

type exportTaskJugglerCLIResult struct {
	Metadata   app.ExportMetadata `json:"metadata"`
	Scenario   string             `json:"scenario,omitempty"`
	Content    string             `json:"content,omitempty"`
	OutputPath string             `json:"output_path,omitempty"`
	Warnings   []app.Diagnostic   `json:"warnings"`
	Errors     []app.Diagnostic   `json:"errors"`
}

func printExportTaskJugglerJSON(stdout *os.File, result app.ExportTaskJugglerResult, outputPath string) error {
	content := result.Content
	if outputPath != "" {
		content = ""
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(exportTaskJugglerCLIResult{
		Metadata:   result.Metadata,
		Scenario:   result.Scenario,
		Content:    content,
		OutputPath: outputPath,
		Warnings:   result.Warnings,
		Errors:     result.Errors,
	})
}
