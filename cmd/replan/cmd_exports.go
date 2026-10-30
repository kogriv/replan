package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/reports"
)

type ExportManifest struct {
	ID          string           `json:"id"`
	Command     string           `json:"command"`
	Format      string           `json:"format"`
	Report      string           `json:"report,omitempty"`
	ProjectPath string           `json:"project_path"`
	OutputPath  string           `json:"output_path"`
	ScenarioID  string           `json:"scenario_id,omitempty"`
	Week        string           `json:"week,omitempty"`
	SHA256      string           `json:"sha256"`
	CreatedAt   string           `json:"created_at"`
	Warnings    []app.Diagnostic `json:"warnings"`
	Errors      []app.Diagnostic `json:"errors"`
}

func runExport(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing export subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "json":
		return runExportJSON(args[1:], stdout, stderr)
	case "taskjuggler":
		return runExportTaskJuggler(args[1:], stdout, stderr)
	case "csv":
		return runExportCSV(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown export subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runExportJSON(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("export json")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	output := fs.String("output", "", "path to write JSON export; stdout when omitted")
	manifest := fs.String("manifest", "", "path to write export manifest JSON; used only with --output")
	format := fs.String("format", "json", "output format: json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *manifest != "" && *output == "" {
		return printExportManifestRequiresOutput(stdout, stderr, "export json", *projectPath)
	}
	if strings.ToLower(*format) != "json" {
		env := envelope{
			OK:          false,
			Command:     "export json",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "export json supports only json format",
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
	if code, done := refuseBindingsExportInsideProject("export json", *projectPath, stdout, stderr, *output, *manifest); done {
		return code
	}
	result, err := app.NewExportService(newStore(*projectPath)).JSON(context.Background(), app.ExportJSONRequest{Format: "json"})
	if err != nil && diagnosticsContainNotFound(result.Errors) {
		env := envelope{OK: false, Command: "export json", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitNotFound
	}
	if *output != "" && err == nil {
		content, renderErr := jsonBytes(result)
		if renderErr != nil {
			fmt.Fprintf(stderr, "failed to render export: %v\n", renderErr)
			return exitGeneralError
		}
		if writeErr := writeTextFile(*output, string(content)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write export: %v\n", writeErr)
			return exitGeneralError
		}
		if *manifest != "" {
			if writeErr := writeExportManifestFile(*manifest, ExportManifest{
				ID:          result.Metadata.ID,
				Command:     "export json",
				Format:      "json",
				ProjectPath: *projectPath,
				OutputPath:  *output,
				SHA256:      sha256Hex(content),
				CreatedAt:   result.Metadata.ExportedAt,
				Warnings:    result.Warnings,
				Errors:      result.Errors,
			}); writeErr != nil {
				fmt.Fprintf(stderr, "failed to write manifest: %v\n", writeErr)
				return exitGeneralError
			}
		}
	} else {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		if printErr := enc.Encode(result); printErr != nil {
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

func runExportCSV(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("export csv")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	reportName := fs.String("report", "", "report to export: workload")
	week := fs.String("week", "", "ISO week, for example 2026-W28")
	scenarioID := fs.String("scenario", "", "scenario id to apply as an in-memory overlay")
	output := fs.String("output", "", "path to write CSV export; stdout when omitted")
	manifest := fs.String("manifest", "", "path to write export manifest JSON; used only with --output")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if *manifest != "" && *output == "" {
		return printExportManifestRequiresOutput(stdout, stderr, "export csv", *projectPath)
	}
	if *reportName != "workload" {
		env := envelope{
			OK:          false,
			Command:     "export csv",
			ProjectPath: *projectPath,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_export_report",
				Message:  "export csv currently supports only --report workload",
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
	if *week != "" && !validISOWeek(*week) {
		return printInvalidWeek(stdout, "export csv", *projectPath, *week)
	}
	if code, done := refuseBindingsExportInsideProject("export csv", *projectPath, stdout, stderr, *output, *manifest); done {
		return code
	}
	result, err := app.NewWorkloadService(newStore(*projectPath)).Calculate(context.Background(), app.WorkloadRequest{
		ProjectPath: *projectPath,
		Week:        *week,
		ScenarioID:  *scenarioID,
	})
	if err == nil && *output != "" {
		var buf strings.Builder
		if writeErr := reports.WriteWorkloadCSV(&buf, workloadCSV(result)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to render csv: %v\n", writeErr)
			return exitGeneralError
		}
		content := []byte(buf.String())
		if writeErr := writeTextFile(*output, string(content)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write export: %v\n", writeErr)
			return exitGeneralError
		}
		if *manifest != "" {
			if writeErr := writeExportManifestFile(*manifest, ExportManifest{
				ID:          exportManifestID("csv", "workload", result.Week, result.ScenarioID),
				Command:     "export csv",
				Format:      "csv",
				Report:      *reportName,
				ProjectPath: *projectPath,
				OutputPath:  *output,
				ScenarioID:  result.ScenarioID,
				Week:        result.Week,
				SHA256:      sha256Hex(content),
				CreatedAt:   "",
				Warnings:    result.Warnings,
				Errors:      result.Errors,
			}); writeErr != nil {
				fmt.Fprintf(stderr, "failed to write manifest: %v\n", writeErr)
				return exitGeneralError
			}
		}
	} else if err == nil {
		if writeErr := reports.WriteWorkloadCSV(stdout, workloadCSV(result)); writeErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", writeErr)
			return exitGeneralError
		}
	}
	if err != nil {
		env := envelope{
			OK:          false,
			Command:     "export csv",
			ProjectPath: *projectPath,
			Result:      result,
			Warnings:    result.Warnings,
			Errors:      result.Errors,
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func workloadCSV(result app.WorkloadResult) reports.WorkloadCSV {
	out := reports.WorkloadCSV{
		Week:       result.Week,
		ScenarioID: result.ScenarioID,
		Resources:  make([]reports.WorkloadCSVRow, 0, len(result.Resources)),
	}
	for _, row := range result.Resources {
		out.Resources = append(out.Resources, reports.WorkloadCSVRow{
			ResourceID:    row.ResourceID,
			ResourceName:  row.ResourceName,
			ResourceClass: row.ResourceClass,
			Unit:          row.Unit,
			Capacity:      row.Capacity,
			Planned:       row.Planned,
			Reserved:      row.Reserved,
			Actual:        row.Actual,
			Remaining:     row.Remaining,
			Overload:      row.Overload,
		})
	}
	return out
}

func printExportManifestRequiresOutput(stdout, stderr *os.File, command, projectPath string) int {
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Errors: []app.Diagnostic{{
			Code:     "manifest_requires_output",
			Message:  "--manifest requires --output so the manifest can describe a written artifact",
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

func jsonBytes(value any) ([]byte, error) {
	var buf strings.Builder
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return []byte(buf.String()), nil
}

func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return fmt.Sprintf("%x", sum[:])
}

func exportManifestID(parts ...string) string {
	cleaned := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, slugIDForManifest(part))
		}
	}
	if len(cleaned) == 0 {
		return "export.manifest." + time.Now().UTC().Format("20060102T150405Z")
	}
	return "export.manifest." + strings.Join(cleaned, ".") + "." + time.Now().UTC().Format("20060102T150405Z")
}

func slugIDForManifest(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "_", "-")
	value = strings.ToLower(replacer.Replace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func writeExportManifestFile(path string, manifest ExportManifest) error {
	if manifest.CreatedAt == "" {
		manifest.CreatedAt = time.Now().Format(time.RFC3339)
	}
	if manifest.Warnings == nil {
		manifest.Warnings = []app.Diagnostic{}
	}
	if manifest.Errors == nil {
		manifest.Errors = []app.Diagnostic{}
	}
	content, err := jsonBytes(manifest)
	if err != nil {
		return err
	}
	return writeTextFile(path, string(content))
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func writeTextFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	cleanup = false
	return nil
}
