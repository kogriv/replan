package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/buildinfo"
)

func runImport(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing import subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "github":
		return runImportGitHub(args[1:], stdout, stderr)
	case "openproject":
		return runImportOpenProject(args[1:], stdout, stderr)
	case "accept":
		return runImportAccept(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown import subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runImportGitHub(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("import github")
	defaultRequestTimeout, defaultRequestTimeoutErr := defaultOpenProjectRequestTimeoutSeconds()
	defaultRequestRetries, defaultRequestRetriesErr := defaultOpenProjectRequestRetries()
	defaultRequestRetryDelay, defaultRequestRetryDelayErr := defaultOpenProjectRequestRetryDelaySeconds()
	projectPath, format := addProjectFormatFlags(fs, "json")
	fromFile := fs.String("from-file", "", "versioned GitHub Issues import JSON document")
	profile := fs.String("profile", "", "GitHub source profile id from .replan/project.yaml")
	repository := fs.String("repository", "", "GitHub repository in owner/repo form")
	baseURL := fs.String("base-url", "", "GitHub API base URL (default https://api.github.com)")
	tokenEnv := fs.String("token-env", "", "environment variable containing a GitHub token (default GITHUB_TOKEN; empty disables token lookup)")
	since := fs.String("since", "", "import issues updated since YYYY-MM-DD or RFC3339")
	includeClosed := fs.Bool("include-closed", false, "include closed issues")
	includeRelations := fs.Bool("include-relations", false, "include blocked-by links as dependency candidates")
	resourceLinksSidecar := fs.String("resource-links-sidecar", "", "GitHub resource-link sidecar path")
	resourceDemandsSidecar := fs.String("resource-demands-sidecar", "", "GitHub resource-demand sidecar path")
	requestTimeout := fs.Float64("request-timeout", defaultRequestTimeout, "GitHub API request timeout in seconds; default can be set with REPLAN_OPENPROJECT_TIMEOUT_SECONDS")
	requestRetries := fs.Int("request-retries", defaultRequestRetries, "GitHub API transient GET retry count; default can be set with REPLAN_OPENPROJECT_RETRIES")
	requestRetryDelay := fs.Float64("request-retry-delay", defaultRequestRetryDelay, "GitHub API retry delay in seconds; default can be set with REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS")
	dryRun := fs.Bool("dry-run", false, "preview the import without writing files")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if defaultRequestTimeoutErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestTimeoutErr)
		return exitInvalidUsage
	}
	if defaultRequestRetriesErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetriesErr)
		return exitInvalidUsage
	}
	if defaultRequestRetryDelayErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetryDelayErr)
		return exitInvalidUsage
	}
	if *requestTimeout <= 0 {
		fmt.Fprintln(stderr, "--request-timeout must be positive")
		return exitInvalidUsage
	}
	if *requestRetries < 0 {
		fmt.Fprintln(stderr, "--request-retries must be non-negative")
		return exitInvalidUsage
	}
	if *requestRetryDelay <= 0 {
		fmt.Fprintln(stderr, "--request-retry-delay must be positive")
		return exitInvalidUsage
	}
	tokenEnvSet := false
	liveFlagsSet := false
	fs.Visit(func(flag *flag.Flag) {
		if flag.Name == "token-env" {
			tokenEnvSet = true
		}
		switch flag.Name {
		case "repository", "base-url", "token-env", "request-timeout", "request-retries", "request-retry-delay":
			liveFlagsSet = true
		}
	})
	if !*dryRun {
		if code, done := refuseBindingsLayerWrite("import github", *projectPath, *format, stdout, stderr); done {
			return code
		}
	}
	result, err := app.NewGitHubImportService(newStore(*projectPath)).Import(context.Background(), app.GitHubImportRequest{
		FromFile: *fromFile, DryRun: *dryRun, Profile: *profile, Repository: *repository, BaseURL: *baseURL,
		TokenEnv: *tokenEnv, TokenEnvSet: tokenEnvSet, LiveFlagsSet: liveFlagsSet,
		UserAgent: "replan/" + buildinfo.Current().Version, Since: *since,
		IncludeClosed: *includeClosed, IncludeRelations: *includeRelations,
		ResourceLinksSidecar: *resourceLinksSidecar, ResourceDemandsSidecar: *resourceDemandsSidecar,
		RequestTimeoutSeconds: *requestTimeout, RequestRetries: *requestRetries, RequestRetryDelaySeconds: *requestRetryDelay,
	})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "import github", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if *format == "markdown" || *format == "text" {
		if printErr := writeGitHubImportMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if diagnosticsContainExternal(result.Errors) {
				return exitExternalSource
			}
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runImportOpenProject(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("import openproject")
	defaultRequestTimeout, defaultRequestTimeoutErr := defaultOpenProjectRequestTimeoutSeconds()
	defaultRequestRetries, defaultRequestRetriesErr := defaultOpenProjectRequestRetries()
	defaultRequestRetryDelay, defaultRequestRetryDelayErr := defaultOpenProjectRequestRetryDelaySeconds()
	projectPath, format := addProjectFormatFlags(fs, "json")
	fromFile := fs.String("from-file", "", "OpenProject work packages JSON snapshot")
	profile := fs.String("profile", "", "OpenProject source profile id from .replan/project.yaml")
	baseURL := fs.String("base-url", "", "OpenProject base URL")
	projectID := fs.String("project-id", "", "OpenProject project id or identifier")
	tokenEnv := fs.String("token-env", "", "environment variable containing OpenProject API token")
	hostHeader := fs.String("host-header", "", "HTTP Host header override for tunneled OpenProject API access")
	forwardedProto := fs.String("forwarded-proto", "", "X-Forwarded-Proto header override for tunneled OpenProject API access")
	perimeterKeyEnv := fs.String("perimeter-key-env", app.DefaultOpenProjectPerimeterKeyEnv, "environment variable containing the key a reverse proxy in front of OpenProject asks of API clients; ignored when unset")
	perimeterHeader := fs.String("perimeter-header", os.Getenv("REPLAN_OPENPROJECT_PERIMETER_HEADER"), "request header that carries the perimeter key (default X-Perimeter-Key)")
	since := fs.String("since", "", "import work packages updated since date or timestamp")
	workPackageID := fs.String("work-package", "", "import one OpenProject work package id")
	pageSize := fs.Int("page-size", 100, "OpenProject API page size")
	requestTimeout := fs.Float64("request-timeout", defaultRequestTimeout, "OpenProject API request timeout in seconds; default can be set with REPLAN_OPENPROJECT_TIMEOUT_SECONDS")
	requestRetries := fs.Int("request-retries", defaultRequestRetries, "OpenProject API transient GET retry count; default can be set with REPLAN_OPENPROJECT_RETRIES")
	requestRetryDelay := fs.Float64("request-retry-delay", defaultRequestRetryDelay, "OpenProject API retry delay in seconds; default can be set with REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS")
	includeClosed := fs.Bool("include-closed", false, "include closed work packages when supported by the source query")
	includeTimeEntries := fs.Bool("include-time-entries", false, "include OpenProject time entries as read-only actual candidates")
	includeRelations := fs.Bool("include-relations", false, "include OpenProject relations as read-only dependency candidates")
	resourceLinksSidecar := fs.String("resource-links-sidecar", "", "OpenProject resource-link sidecar path")
	resourceDemandsSidecar := fs.String("resource-demands-sidecar", "", "OpenProject resource-demand sidecar path")
	dryRun := fs.Bool("dry-run", false, "preview the import without writing files")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if defaultRequestTimeoutErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestTimeoutErr)
		return exitInvalidUsage
	}
	if defaultRequestRetriesErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetriesErr)
		return exitInvalidUsage
	}
	if defaultRequestRetryDelayErr != nil {
		fmt.Fprintf(stderr, "%v\n", defaultRequestRetryDelayErr)
		return exitInvalidUsage
	}
	if *requestTimeout <= 0 {
		fmt.Fprintln(stderr, "--request-timeout must be positive")
		return exitInvalidUsage
	}
	if *requestRetries < 0 {
		fmt.Fprintln(stderr, "--request-retries must be non-negative")
		return exitInvalidUsage
	}
	if *requestRetryDelay <= 0 {
		fmt.Fprintln(stderr, "--request-retry-delay must be positive")
		return exitInvalidUsage
	}
	if !*dryRun {
		if code, done := refuseBindingsLayerWrite("import openproject", *projectPath, *format, stdout, stderr); done {
			return code
		}
	}
	result, err := app.NewOpenProjectImportService(newStore(*projectPath)).Import(context.Background(), app.OpenProjectImportRequest{
		FromFile:                 *fromFile,
		DryRun:                   *dryRun,
		Profile:                  *profile,
		BaseURL:                  *baseURL,
		ProjectID:                *projectID,
		TokenEnv:                 *tokenEnv,
		HostHeader:               *hostHeader,
		ForwardedProto:           *forwardedProto,
		PerimeterKeyEnv:          *perimeterKeyEnv,
		PerimeterHeader:          *perimeterHeader,
		Since:                    *since,
		IncludeClosed:            *includeClosed,
		IncludeTimeEntries:       *includeTimeEntries,
		IncludeRelations:         *includeRelations,
		ResourceLinksSidecar:     *resourceLinksSidecar,
		ResourceDemandsSidecar:   *resourceDemandsSidecar,
		WorkPackageID:            *workPackageID,
		PageSize:                 *pageSize,
		RequestTimeoutSeconds:    *requestTimeout,
		RequestRetries:           *requestRetries,
		RequestRetryDelaySeconds: *requestRetryDelay,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "import openproject",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeOpenProjectImportMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if diagnosticsContainPrivateOpenProjectIDs(result.Errors) {
				return exitInvalidUsage
			}
			if diagnosticsContainExternal(result.Errors) {
				return exitExternalSource
			}
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func defaultOpenProjectRequestTimeoutSeconds() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("REPLAN_OPENPROJECT_TIMEOUT_SECONDS"))
	if raw == "" {
		return 60, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 60, fmt.Errorf("REPLAN_OPENPROJECT_TIMEOUT_SECONDS must be numeric")
	}
	if value <= 0 {
		return 60, fmt.Errorf("REPLAN_OPENPROJECT_TIMEOUT_SECONDS must be positive")
	}
	return value, nil
}

func defaultOpenProjectRequestRetries() (int, error) {
	raw := strings.TrimSpace(os.Getenv("REPLAN_OPENPROJECT_RETRIES"))
	if raw == "" {
		return 0, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("REPLAN_OPENPROJECT_RETRIES must be an integer")
	}
	if value < 0 {
		return 0, fmt.Errorf("REPLAN_OPENPROJECT_RETRIES must be non-negative")
	}
	return value, nil
}

func defaultOpenProjectRequestRetryDelaySeconds() (float64, error) {
	raw := strings.TrimSpace(os.Getenv("REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS"))
	if raw == "" {
		return 2, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 2, fmt.Errorf("REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS must be numeric")
	}
	if value <= 0 {
		return 2, fmt.Errorf("REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS must be positive")
	}
	return value, nil
}

func runImportAccept(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("import accept")
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	acceptTimeEntries := fs.Bool("accept-time-entries", false, "accept tracker time-entry candidates as actual consumption with evidence (currently OpenProject snapshots only)")
	acceptResourceDemands := fs.Bool("accept-resource-demands", false, "accept tracker resource-demand candidates into canonical resource demands")
	acceptResourceLinks := fs.Bool("accept-resource-links", false, "accept tracker resource-link candidates as canonical resource source refs")
	acceptTrackingStatus := fs.Bool("accept-tracking-status", false, "accept mapped tracker statuses as canonical task tracking statuses")
	acceptDates := fs.Bool("accept-dates", false, "accept tracker start and due dates as canonical task planned dates")
	acceptDependencies := fs.Bool("accept-dependencies", false, "accept tracker dependency candidates into canonical dependencies")
	actorID := fs.String("actor", "", "actor id applying the import acceptance")
	reason := fs.String("reason", "", "reason for accepting the import")
	positionalSnapshotID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("import accept", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	snapshotID := positionalArg(positionalSnapshotID, fs)
	result, err := app.NewImportAcceptService(newStore(*projectPath)).Accept(context.Background(), app.ImportAcceptRequest{
		SnapshotID:            snapshotID,
		DryRun:                dryRun,
		Apply:                 !dryRun,
		AcceptTimeEntries:     *acceptTimeEntries,
		AcceptResourceDemands: *acceptResourceDemands,
		AcceptResourceLinks:   *acceptResourceLinks,
		AcceptTrackingStatus:  *acceptTrackingStatus,
		AcceptDates:           *acceptDates,
		AcceptDependencies:    *acceptDependencies,
		ActorID:               *actorID,
		Reason:                *reason,
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "import accept",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeImportAcceptMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			if diagnosticsContainPrivateOpenProjectIDs(result.Errors) {
				return exitInvalidUsage
			}
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

//nolint:gocognit,gocyclo // This flat renderer deliberately follows output order; section extraction would make the byte-level Markdown contract harder to audit.
func writeOpenProjectImportMarkdown(stdout *os.File, result app.OpenProjectImportResult) error {
	if _, err := fmt.Fprintln(stdout, "# OpenProject Import Dry Run"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Source: `%s`\n", result.Source); err != nil {
		return err
	}
	if result.FromFile != "" {
		if _, err := fmt.Fprintf(stdout, "- From file: `%s`\n", result.FromFile); err != nil {
			return err
		}
	}
	if result.Profile != nil {
		if _, err := fmt.Fprintf(stdout, "- Profile: `%s`\n", result.Profile.ID); err != nil {
			return err
		}
		if result.Profile.BaseURL != "" {
			if _, err := fmt.Fprintf(stdout, "- Base URL: `%s`\n", result.Profile.BaseURL); err != nil {
				return err
			}
		}
		if result.Profile.ProjectID != "" {
			if _, err := fmt.Fprintf(stdout, "- Source project: `%s`\n", result.Profile.ProjectID); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintf(stdout, "- Loaded work packages: `%d`\n", result.Snapshot.LoadedCount); err != nil {
		return err
	}
	if result.Snapshot.ID != "" {
		if _, err := fmt.Fprintf(stdout, "- Snapshot: `%s`\n", result.Snapshot.ID); err != nil {
			return err
		}
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` %s %s\n", write.Path, write.Action, write.Format); err != nil {
				return err
			}
		}
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Warnings"); err != nil {
			return err
		}
		for _, d := range result.Warnings {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(stdout, "\n## Task Candidates"); err != nil {
		return err
	}
	if len(result.TaskCandidates) == 0 {
		_, err := fmt.Fprintln(stdout, "- none")
		return err
	}
	for _, candidate := range result.TaskCandidates {
		target := "new task"
		if candidate.MatchedTaskID != "" {
			target = "`" + candidate.MatchedTaskID + "`"
		}
		if _, err := fmt.Fprintf(stdout, "- `%s` %s -> %s", candidate.SourceID, candidate.Title, target); err != nil {
			return err
		}
		if candidate.MappedStatus != "" {
			if _, err := fmt.Fprintf(stdout, " status `%s`", candidate.MappedStatus); err != nil {
				return err
			}
		}
		if candidate.EstimatedHours > 0 {
			if _, err := fmt.Fprintf(stdout, " estimate `%.2f h`", candidate.EstimatedHours); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}
	return nil
}

func writeGitHubImportMarkdown(stdout *os.File, result app.GitHubImportResult) error {
	if _, err := fmt.Fprintln(stdout, "# GitHub Import"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "\n- Source: `%s`\n- From file: `%s`\n- Loaded issues: `%d`\n- Snapshot: `%s`\n", result.Source, result.FromFile, result.Snapshot.LoadedCount, result.Snapshot.ID); err != nil {
		return err
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` %s %s\n", write.Path, write.Action, write.Format); err != nil {
				return err
			}
		}
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Warnings"); err != nil {
			return err
		}
		for _, diagnostic := range result.Warnings {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Errors"); err != nil {
			return err
		}
		for _, diagnostic := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(stdout, "\n## Task Candidates"); err != nil {
		return err
	}
	if len(result.TaskCandidates) == 0 {
		_, err := fmt.Fprintln(stdout, "- none")
		return err
	}
	for _, candidate := range result.TaskCandidates {
		target := "new task"
		if candidate.MatchedTaskID != "" {
			target = "`" + candidate.MatchedTaskID + "`"
		}
		if _, err := fmt.Fprintf(stdout, "- `%s` %s -> %s", candidate.SourceID, candidate.Title, target); err != nil {
			return err
		}
		if candidate.MappedStatus != "" {
			if _, err := fmt.Fprintf(stdout, " status `%s`", candidate.MappedStatus); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocognit,gocyclo // This flat renderer deliberately follows output order; section extraction would make the byte-level Markdown contract harder to audit.
func writeImportAcceptMarkdown(stdout *os.File, result app.ImportAcceptResult) error {
	title := "# Import Accept Dry Run"
	if result.Applied {
		title = "# Import Accept Apply"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if result.SnapshotID != "" {
		if _, err := fmt.Fprintf(stdout, "- Snapshot: `%s`\n", result.SnapshotID); err != nil {
			return err
		}
	}
	if result.MappedPath != "" {
		if _, err := fmt.Fprintf(stdout, "- Mapped: `%s`\n", result.MappedPath); err != nil {
			return err
		}
	}
	if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` %s %s\n", write.Path, write.Action, write.Format); err != nil {
				return err
			}
		}
	}
	if len(result.WrittenFiles) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	}
	if len(result.AppliedChanges) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Applied Changes"); err != nil {
			return err
		}
		for _, change := range result.AppliedChanges {
			target := change.TargetKind
			if change.TargetID != "" {
				target += " `" + change.TargetID + "`"
			}
			if _, err := fmt.Fprintf(stdout, "- `%s` %s: %s\n", change.Op, target, change.Description); err != nil {
				return err
			}
		}
	}
	if len(result.Changes) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Changes"); err != nil {
			return err
		}
		for _, change := range result.Changes {
			target := change.TargetKind
			if change.TargetID != "" {
				target += " `" + change.TargetID + "`"
			}
			if _, err := fmt.Fprintf(stdout, "- `%s` %s: %s\n", change.Op, target, change.Description); err != nil {
				return err
			}
		}
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	if len(result.Warnings) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Warnings"); err != nil {
			return err
		}
		for _, d := range result.Warnings {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
	}
	return nil
}
