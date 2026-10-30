package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/reports"
)

type weeklyReportWriteResult struct {
	Report     reports.WeeklyReport `json:"report"`
	OutputPath string               `json:"output_path"`
}

type dashboardReportWriteResult struct {
	Overview     reports.DashboardOverview `json:"overview"`
	OutputDir    string                    `json:"output_dir"`
	Artifacts    []string                  `json:"artifacts"`
	ManifestPath string                    `json:"manifest_path"`
}

func runReport(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing report subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "weekly":
		return runReportWeekly(args[1:], stdout, stderr)
	case "estimates":
		return runReportEstimates(args[1:], stdout, stderr)
	case "state":
		return runReportState(args[1:], stdout, stderr)
	case "dashboard":
		return runReportDashboard(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown report subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runReportEstimates(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report estimates")
	var projectPath, baselineID, format string
	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&baselineID, "baseline", "", "baseline id")
	fs.StringVar(&format, "format", "markdown", "output format: json or markdown")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if baselineID == "" || (format != "json" && format != "markdown") {
		message := "--baseline is required"
		code := "missing_baseline"
		if baselineID != "" {
			message = "supported formats are json and markdown"
			code = "unsupported_format"
		}
		_ = printEnvelope(stdout, "json", envelope{OK: false, Command: "report estimates", ProjectPath: projectPath, Result: nil,
			Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{{Code: code, Message: message, Severity: "error"}}})
		return exitInvalidUsage
	}

	result, err := app.NewReportService(newStore(projectPath)).Estimates(context.Background(), app.EstimateReportRequest{BaselineID: baselineID})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "report estimates", ProjectPath: projectPath,
		Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}
	var printErr error
	if format == "json" {
		printErr = printEnvelope(stdout, "json", env)
	} else {
		printErr = writeEstimateReportMarkdown(stdout, result)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(result.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runReportState(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report state")

	var (
		projectPath string
		format      string
		week        string
		scenarioID  string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "markdown", "output format: json, markdown or text")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W31")
	fs.StringVar(&scenarioID, "scenario", "", "scenario id to apply as an in-memory overlay")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "text" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "report state",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "report state", projectPath, week)
	}

	store := newStore(projectPath)
	service := app.NewReportService(store)
	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		GeneratedAt: time.Now(),
		Command:     "report state",
		ScenarioID:  scenarioID,
	})
	env := envelope{
		OK:          err == nil && len(overview.Errors) == 0,
		Command:     "report state",
		ProjectPath: overview.ProjectPath,
		Result:      overview,
		Warnings:    reportIssuesToDiagnostics(overview.Warnings),
		Errors:      reportIssuesToDiagnostics(overview.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "markdown", "text":
		printErr = reports.WriteProjectStateMarkdown(stdout, overview)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(reportIssuesToDiagnostics(overview.Errors))
		}
		return exitGeneralError
	}
	return exitSuccess
}

// runNext prints the short operator view of report state: what waits on a
// person, what an agent can prepare, and one health line.
func runNext(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("next")

	var (
		projectPath string
		format      string
		week        string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "text", "output format: text or json")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W31")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "json" && format != "text" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "next",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are text and json",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}
	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "next", projectPath, week)
	}

	store := newStore(projectPath)
	overview, err := app.NewReportService(store).Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		GeneratedAt: time.Now(),
		Command:     "next",
	})
	brief := reports.BuildNextBrief(overview)
	env := envelope{
		OK:          err == nil && len(overview.Errors) == 0,
		Command:     "next",
		ProjectPath: overview.ProjectPath,
		Result:      brief,
		Warnings:    reportIssuesToDiagnostics(overview.Warnings),
		Errors:      reportIssuesToDiagnostics(overview.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "text":
		printErr = reports.WriteNextBriefText(stdout, brief)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(reportIssuesToDiagnostics(overview.Errors))
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runReportDashboard(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report dashboard")

	var (
		projectPath string
		format      string
		outputDir   string
		week        string
		scenarioID  string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "html", "output format: html or json")
	fs.StringVar(&outputDir, "output", "", "directory to write generated dashboard artifacts")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W31")
	fs.StringVar(&scenarioID, "scenario", "", "scenario id to apply as an in-memory overlay")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "html" && format != "json" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "report dashboard",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are html and json",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "report dashboard", projectPath, week)
	}
	if outputDir != "" {
		if code, done := refuseScenarioReportWrite("report dashboard", projectPath, scenarioID, outputDir, stdout, stderr); done {
			return code
		}
		if code, done := refuseBindingsOutputInsideProject("report dashboard", projectPath, outputDir, stdout, stderr); done {
			return code
		}
	}

	store := newStore(projectPath)
	service := app.NewReportService(store)
	overview, err := service.Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		GeneratedAt: time.Now(),
		Command:     "report dashboard",
		ScenarioID:  scenarioID,
	})
	env := envelope{
		OK:          err == nil && len(overview.Errors) == 0,
		Command:     "report dashboard",
		ProjectPath: overview.ProjectPath,
		Result:      overview,
		Warnings:    reportIssuesToDiagnostics(overview.Warnings),
		Errors:      reportIssuesToDiagnostics(overview.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}
	if code, done := printProjectFormatUnsupported(stdout, stderr, format, env, err); done {
		return code
	}

	if outputDir != "" {
		result, writeErr := writeDashboardArtifacts(projectPath, outputDir, overview)
		if writeErr != nil {
			fmt.Fprintf(stderr, "failed to write dashboard: %v\n", writeErr)
			return exitGeneralError
		}
		env.Result = result
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if errors.Is(err, app.ErrValidationFailed) {
				return validationFailureExit(env.Errors)
			}
			return exitGeneralError
		}
		return exitSuccess
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "html":
		printErr = reports.WriteDashboardHTML(stdout, overview)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(env.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func runReportWeekly(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("report weekly")

	var (
		projectPath string
		format      string
		week        string
		write       bool
		scenarioID  string
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "markdown", "output format: json, markdown, csv, html, text")
	fs.StringVar(&week, "week", "", "ISO week, for example 2026-W28")
	fs.BoolVar(&write, "write", false, "write generated report artifact into the project reports directory")
	fs.StringVar(&scenarioID, "scenario", "", "scenario id to apply as an in-memory overlay")

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if projectPath == "" {
		projectPath = "."
	}
	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "csv" && format != "html" && format != "text" {
		printEnvelope(stdout, "json", envelope{
			OK:          false,
			Command:     "report weekly",
			ProjectPath: projectPath,
			Result:      nil,
			Errors: []app.Diagnostic{{
				Code:     "unsupported_format",
				Message:  "supported formats are json, markdown, csv, html and text",
				Severity: "error",
			}},
			Warnings: []app.Diagnostic{},
		})
		return exitInvalidUsage
	}

	if week != "" && !validISOWeek(week) {
		return printInvalidWeek(stdout, "report weekly", projectPath, week)
	}
	if write {
		if code, done := refuseScenarioReportWrite("report weekly", projectPath, scenarioID, "reports", stdout, stderr); done {
			return code
		}
		if code, done := refuseBindingsOutputInsideProject("report weekly", projectPath, "reports", stdout, stderr); done {
			return code
		}
	}

	store := newStore(projectPath)
	service := app.NewReportService(store)
	report, err := service.Weekly(context.Background(), app.WeeklyReportRequest{
		ProjectPath: projectPath,
		Week:        week,
		ScenarioID:  scenarioID,
	})
	env := envelope{
		OK:          err == nil && len(report.Errors) == 0,
		Command:     "report weekly",
		ProjectPath: report.ProjectPath,
		Result:      report,
		Warnings:    reportIssuesToDiagnostics(report.Warnings),
		Errors:      reportIssuesToDiagnostics(report.Errors),
	}
	if code, done := printProjectNotFound(stdout, stderr, format, env, err); done {
		return code
	}
	if code, done := printProjectFormatUnsupported(stdout, stderr, format, env, err); done {
		return code
	}

	if write {
		outputPath, writeErr := writeWeeklyReportArtifact(projectPath, week, format, report)
		if writeErr != nil {
			fmt.Fprintf(stderr, "failed to write report: %v\n", writeErr)
			return exitGeneralError
		}
		env.Result = weeklyReportWriteResult{
			Report:     report,
			OutputPath: outputPath,
		}
		if printErr := printEnvelope(stdout, "json", env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if errors.Is(err, app.ErrValidationFailed) {
				return validationFailureExit(env.Errors)
			}
			return exitGeneralError
		}
		return exitSuccess
	}

	var printErr error
	switch format {
	case "json":
		printErr = printEnvelope(stdout, "json", env)
	case "csv":
		printErr = reports.WriteWeeklyCSV(stdout, report)
	case "html":
		printErr = reports.WriteWeeklyHTML(stdout, report)
	case "markdown", "text":
		printErr = reports.WriteWeeklyMarkdown(stdout, report)
	}
	if printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if err != nil {
		if errors.Is(err, app.ErrValidationFailed) {
			return validationFailureExit(env.Errors)
		}
		return exitGeneralError
	}
	return exitSuccess
}

func writeWeeklyReportArtifact(projectPath, week, format string, report reports.WeeklyReport) (string, error) {
	if week == "" {
		week = "unspecified-week"
	}
	var b strings.Builder
	extension := format
	switch format {
	case "json":
		content, err := jsonBytes(envelope{
			OK:          len(report.Errors) == 0,
			Command:     "report weekly",
			ProjectPath: report.ProjectPath,
			Result:      report,
			Warnings:    reportIssuesToDiagnostics(report.Warnings),
			Errors:      reportIssuesToDiagnostics(report.Errors),
		})
		if err != nil {
			return "", err
		}
		b.Write(content)
	case "csv":
		if err := reports.WriteWeeklyCSV(&b, report); err != nil {
			return "", err
		}
	case "html":
		if err := reports.WriteWeeklyHTML(&b, report); err != nil {
			return "", err
		}
	case "markdown", "text":
		extension = "md"
		if err := reports.WriteWeeklyMarkdown(&b, report); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported report artifact format %s", format)
	}
	relPath := filepath.ToSlash(filepath.Join("reports", "weekly", week+"."+extension))
	outputPath := filepath.Join(projectPath, filepath.FromSlash(relPath))
	if err := writeTextFile(outputPath, b.String()); err != nil {
		return "", err
	}
	return relPath, nil
}

func writeDashboardArtifacts(projectPath, outputDir string, overview reports.DashboardOverview) (dashboardReportWriteResult, error) {
	baseDir := outputDir
	if !filepath.IsAbs(baseDir) {
		baseDir = filepath.Join(projectPath, filepath.FromSlash(outputDir))
	}
	publicationOverview, err := dashboardPublicationOverview(overview)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts := map[string][]byte{}
	var htmlOut strings.Builder
	if err := reports.WriteDashboardHTML(&htmlOut, publicationOverview); err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts["index.html"] = []byte(htmlOut.String())
	var ganttOut strings.Builder
	if err := reports.WriteDashboardGanttHTML(&ganttOut, publicationOverview); err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts["gantt.html"] = []byte(ganttOut.String())
	pageWriters := []struct {
		path  string
		write func(*strings.Builder) error
	}{
		{path: "workload.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardWorkloadHTML(out, publicationOverview.WorkloadPage)
		}},
		{path: "scenarios.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardScenariosHTML(out, publicationOverview.ScenariosPage)
		}},
		{path: "proposals.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardProposalsHTML(out, publicationOverview.ProposalsPage)
		}},
		{path: "estimates.html", write: func(out *strings.Builder) error {
			return reports.WriteDashboardEstimatesHTML(out, publicationOverview.EstimatesPage)
		}},
	}
	for _, page := range pageWriters {
		var out strings.Builder
		if err := page.write(&out); err != nil {
			return dashboardReportWriteResult{}, err
		}
		artifacts[page.path] = []byte(out.String())
	}

	overviewBytes, err := jsonBytes(publicationOverview)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts[filepath.ToSlash(filepath.Join("raw", "overview.json"))] = overviewBytes
	ganttBytes, err := jsonBytes(publicationOverview.Gantt)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts[filepath.ToSlash(filepath.Join("raw", "gantt.json"))] = ganttBytes
	openProjectOverlayBytes, err := jsonBytes(publicationOverview.OpenProjectOverlay)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	artifacts[filepath.ToSlash(filepath.Join("raw", "openproject_overlay.json"))] = openProjectOverlayBytes
	pageJSON := []struct {
		path  string
		value any
	}{
		{path: "workload.json", value: publicationOverview.WorkloadPage},
		{path: "scenarios.json", value: publicationOverview.ScenariosPage},
		{path: "proposals.json", value: publicationOverview.ProposalsPage},
		{path: "estimates.json", value: publicationOverview.EstimatesPage},
	}
	for _, page := range pageJSON {
		content, marshalErr := jsonBytes(page.value)
		if marshalErr != nil {
			return dashboardReportWriteResult{}, marshalErr
		}
		artifacts[filepath.ToSlash(filepath.Join("raw", page.path))] = content
	}

	manifest := reports.DashboardManifest{
		SchemaVersion: 1,
		GeneratedAt:   publicationOverview.GeneratedAt,
		Command:       "replan report dashboard",
		ProjectPath:   publicationOverview.ProjectPath,
		ProjectID:     publicationOverview.ProjectID,
		StatusDate:    publicationOverview.StatusDate,
		ScenarioID:    publicationOverview.ScenarioID,
		Artifacts:     []reports.DashboardManifestArtifact{},
		WarningsCount: len(publicationOverview.Warnings),
		ErrorsCount:   len(publicationOverview.Errors),
	}
	artifactPaths := []string{}
	for rel, content := range artifacts {
		artifactPaths = append(artifactPaths, rel)
		manifest.Artifacts = append(manifest.Artifacts, reports.DashboardManifestArtifact{
			Path:   rel,
			SHA256: sha256Hex(content),
		})
	}
	sort.Strings(artifactPaths)
	sort.Slice(manifest.Artifacts, func(i, j int) bool { return manifest.Artifacts[i].Path < manifest.Artifacts[j].Path })
	manifest.Artifacts = append(manifest.Artifacts, reports.DashboardManifestArtifact{Path: filepath.ToSlash(filepath.Join("raw", "manifest.json"))})
	manifestBytes, err := jsonBytes(manifest)
	if err != nil {
		return dashboardReportWriteResult{}, err
	}
	manifestRel := filepath.ToSlash(filepath.Join("raw", "manifest.json"))
	artifacts[manifestRel] = manifestBytes
	artifactPaths = append(artifactPaths, manifestRel)
	sort.Strings(artifactPaths)

	for _, rel := range artifactPaths {
		if err := writeTextFile(filepath.Join(baseDir, filepath.FromSlash(rel)), string(artifacts[rel])); err != nil {
			return dashboardReportWriteResult{}, err
		}
	}
	return dashboardReportWriteResult{
		Overview:     overview,
		OutputDir:    filepath.ToSlash(outputDir),
		Artifacts:    artifactPaths,
		ManifestPath: manifestRel,
	}, nil
}

func dashboardPublicationOverview(overview reports.DashboardOverview) (reports.DashboardOverview, error) {
	if overview.ProjectPath == "" {
		return overview, nil
	}
	content, err := json.Marshal(overview)
	if err != nil {
		return reports.DashboardOverview{}, err
	}
	encodedPath, err := json.Marshal(overview.ProjectPath)
	if err != nil {
		return reports.DashboardOverview{}, err
	}
	encodedPlaceholder, err := json.Marshal("<project>")
	if err != nil {
		return reports.DashboardOverview{}, err
	}
	// Replace the path only where it stands as a path of its own, with no
	// path character next to it (a slash may follow). A bare replace of a
	// short path such as "." would rewrite every dot, numbers included.
	inner := regexp.QuoteMeta(string(encodedPath[1 : len(encodedPath)-1]))
	standalone := regexp.MustCompile(`(^|[^A-Za-z0-9._~/-])` + inner + `($|[^A-Za-z0-9._~-])`)
	placeholder := string(encodedPlaceholder[1 : len(encodedPlaceholder)-1])
	for {
		replaced := standalone.ReplaceAll(content, []byte("${1}"+placeholder+"${2}"))
		if bytes.Equal(replaced, content) {
			break
		}
		content = replaced
	}
	var publicationOverview reports.DashboardOverview
	if err := json.Unmarshal(content, &publicationOverview); err != nil {
		return reports.DashboardOverview{}, err
	}
	publicationOverview.WorkloadPage = overview.WorkloadPage
	publicationOverview.ScenariosPage = overview.ScenariosPage
	publicationOverview.ProposalsPage = overview.ProposalsPage
	publicationOverview.EstimatesPage = overview.EstimatesPage
	return publicationOverview, nil
}
