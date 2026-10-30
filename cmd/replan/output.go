package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/reports"
)

type envelope struct {
	OK          bool              `json:"ok"`
	Command     string            `json:"command"`
	ProjectPath string            `json:"project_path,omitempty"`
	Result      any               `json:"result"`
	Warnings    []app.Diagnostic  `json:"warnings"`
	Errors      []app.Diagnostic  `json:"errors"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

var runtimeWarnings = struct {
	sync.Mutex
	byOutput map[*os.File][]app.Diagnostic
}{byOutput: map[*os.File][]app.Diagnostic{}}

func addRuntimeWarning(output *os.File, diagnostic app.Diagnostic) {
	runtimeWarnings.Lock()
	defer runtimeWarnings.Unlock()
	runtimeWarnings.byOutput[output] = append(runtimeWarnings.byOutput[output], diagnostic)
}

func clearRuntimeWarnings(output *os.File) {
	runtimeWarnings.Lock()
	defer runtimeWarnings.Unlock()
	delete(runtimeWarnings.byOutput, output)
}

func appendRuntimeWarnings(output *os.File, warnings []app.Diagnostic) []app.Diagnostic {
	runtimeWarnings.Lock()
	defer runtimeWarnings.Unlock()
	return append(warnings, runtimeWarnings.byOutput[output]...)
}

func printCommandResult[T any](stdout, stderr *os.File, format string, result T, env envelope, commandErr error, writeMarkdown func(*os.File, T) error) int {
	if format != "markdown" && format != "text" {
		return printScenarioEnvelope(stdout, stderr, format, env, commandErr)
	}
	if err := writeMarkdown(stdout, result); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError
	}
	if commandErr != nil {
		return validationFailureExit(env.Errors)
	}
	return exitSuccess
}

func printScenarioEnvelope(stdout, stderr *os.File, format string, env envelope, err error) int {
	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "text" {
		env.OK = false
		env.Errors = []app.Diagnostic{{
			Code:     "unsupported_format",
			Message:  "supported formats are json, markdown and text",
			Severity: "error",
		}}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitInvalidUsage
	}
	var printErr error
	if env.OK && format != "json" {
		switch result := env.Result.(type) {
		case app.ScenarioListResult:
			if format == "markdown" {
				printErr = writeScenarioListMarkdown(stdout, result)
			} else {
				printErr = writeScenarioListText(stdout, result)
			}
		case app.ScenarioShowResult:
			if format == "markdown" {
				printErr = writeScenarioShowMarkdown(stdout, result)
			} else {
				printErr = writeScenarioShowText(stdout, result)
			}
		default:
			printErr = printEnvelope(stdout, format, env)
		}
	} else {
		printErr = printEnvelope(stdout, format, env)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			if firstDiagnostic(env.Errors, "approver_not_human") != nil {
				return exitPolicyDenied
			}
			if diagnosticsContainPrivateOpenProjectIDs(env.Errors) {
				return exitInvalidUsage
			}
			if diagnosticsContainExternal(env.Errors) {
				return exitExternalSource
			}
			if diagnosticsContainNotFound(env.Errors) {
				return exitNotFound
			}
			return exitValidationFailed
		}
		return exitGeneralError
	}
	return exitSuccess
}

func diagnosticsContainPrivateOpenProjectIDs(diags []app.Diagnostic) bool {
	for _, diag := range diags {
		if diag.Code == "openproject_ids_private" {
			return true
		}
	}
	return false
}

// diagnosticsContainExternal reports a failure of the external source itself,
// OpenProject unreachable, refusing or answering garbage, as opposed to an
// invalid plan or sidecar (exit 6 in docs/spec/cli.md).
func diagnosticsContainExternal(diags []app.Diagnostic) bool {
	for _, diag := range diags {
		switch diag.Code {
		case "openproject_network_error",
			"openproject_request_error",
			"openproject_api_error",
			"openproject_authentication_failed",
			"openproject_permission_denied",
			"openproject_rate_limited",
			"openproject_malformed_response",
			"openproject_project_not_found",
			"source_unavailable",
			"source_unauthorized",
			"source_not_found",
			"source_rate_limited":
			return true
		}
	}
	return false
}

func diagnosticsContainNotFound(diags []app.Diagnostic) bool {
	// An unsupported format leaves the workspace empty, so the references a
	// command checks next are "not found"; the format is the one answer.
	if firstDiagnostic(diags, "project_format_unsupported") != nil {
		return false
	}
	for _, diag := range diags {
		if diagnosticIsNotFound(diag.Code) {
			return true
		}
	}
	return false
}

func validationFailureExit(diags []app.Diagnostic) int {
	if diagnosticsContainNotFound(diags) {
		return exitNotFound
	}
	return exitValidationFailed
}

func printProjectNotFound(stdout, stderr *os.File, format string, env envelope, commandErr error) (int, bool) {
	if commandErr == nil || firstDiagnostic(env.Errors, "project_not_found") == nil {
		return 0, false
	}
	if err := printEnvelope(stdout, format, env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitNotFound, true
}

func printProjectFormatUnsupported(stdout, stderr *os.File, format string, env envelope, commandErr error) (int, bool) {
	if commandErr == nil || firstDiagnostic(env.Errors, "project_format_unsupported") == nil {
		return 0, false
	}
	if err := printEnvelope(stdout, format, env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitValidationFailed, true
}

func diagnosticIsNotFound(code string) bool {
	switch code {
	case "project_not_found",
		"baseline_not_found",
		"calibration_not_found",
		"scenario_not_found",
		"proposal_not_found",
		"source_profile_not_found",
		"source_snapshot_not_found",
		"task_not_found",
		"demand_not_found",
		"resource_not_found",
		"assignee_not_found",
		"after_task_not_found",
		"after_task_not_in_current_plan",
		"actor_not_found",
		"evidence_not_found",
		"evidence_target_not_found":
		return true
	default:
		return false
	}
}

func printEnvelope(stdout *os.File, format string, env envelope) error {
	env.Warnings = appendRuntimeWarnings(stdout, env.Warnings)
	if projectNotFound := firstDiagnostic(env.Errors, "project_not_found"); projectNotFound != nil {
		env.OK = false
		env.Errors = []app.Diagnostic{*projectNotFound}
	} else if unsupportedFormat := firstDiagnostic(env.Errors, "project_format_unsupported"); unsupportedFormat != nil {
		env.OK = false
		env.Errors = []app.Diagnostic{*unsupportedFormat}
	}
	switch format {
	case "json":
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(env)
	case "markdown":
		fmt.Fprintf(stdout, "# RePlan %s\n\n", env.Command)
		if env.OK {
			fmt.Fprintln(stdout, "Status: OK")
		} else {
			fmt.Fprintln(stdout, "Status: FAILED")
		}
		if env.ProjectPath != "" {
			fmt.Fprintf(stdout, "\nProject: `%s`\n", env.ProjectPath)
		}
		if len(env.Errors) > 0 {
			fmt.Fprintln(stdout, "\n## Errors")
			for _, d := range env.Errors {
				fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message)
			}
		}
		if len(env.Warnings) > 0 {
			fmt.Fprintln(stdout, "\n## Warnings")
			for _, d := range env.Warnings {
				fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message)
			}
		}
		return nil
	default:
		if env.OK {
			fmt.Fprintf(stdout, "%s ok\n", env.Command)
		} else {
			fmt.Fprintf(stdout, "%s failed\n", env.Command)
		}
		for _, d := range env.Errors {
			fmt.Fprintf(stdout, "error %s: %s\n", d.Code, d.Message)
		}
		for _, d := range env.Warnings {
			fmt.Fprintf(stdout, "warning %s: %s\n", d.Code, d.Message)
		}
		return nil
	}
}

func firstDiagnostic(diags []app.Diagnostic, code string) *app.Diagnostic {
	for i := range diags {
		if diags[i].Code == code {
			return &diags[i]
		}
	}
	return nil
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func reportIssuesToDiagnostics(issues []reports.ReportIssue) []app.Diagnostic {
	out := make([]app.Diagnostic, 0, len(issues))
	for _, issue := range issues {
		out = append(out, app.Diagnostic{
			Code:     issue.Code,
			Message:  issue.Message,
			Severity: issue.Severity,
			Target:   app.Target{ID: issue.TargetID},
		})
	}
	return out
}

func writeDiagnosticsMarkdown(b *strings.Builder, warnings, errors []app.Diagnostic) {
	if len(warnings) > 0 {
		b.WriteString("\n## Warnings\n")
		for _, diagnostic := range warnings {
			fmt.Fprintf(b, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message)
		}
	}
	if len(errors) > 0 {
		b.WriteString("\n## Errors\n")
		for _, diagnostic := range errors {
			fmt.Fprintf(b, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message)
		}
	}
}

func writeDiagnosticsText(stdout *os.File, warnings, errors []app.Diagnostic) error {
	for _, diagnostic := range warnings {
		if _, err := fmt.Fprintf(stdout, "warning %s: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
			return err
		}
	}
	for _, diagnostic := range errors {
		if _, err := fmt.Fprintf(stdout, "error %s: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
			return err
		}
	}
	return nil
}

func titleWord(value string) string {
	if value == "" {
		return value
	}
	return strings.ToUpper(value[:1]) + value[1:]
}
