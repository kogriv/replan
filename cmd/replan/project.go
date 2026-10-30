package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func runProject(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing project subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "migrate":
		return runProjectMigrate(args[1:], stdout, stderr)
	case "recover":
		return runProjectRecover(args[1:], stdout, stderr)
	case "set-status-date":
		return runProjectSetStatusDate(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown project subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

type projectRecoverResult struct {
	DryRun            bool     `json:"dry_run"`
	Applied           bool     `json:"applied"`
	Pending           bool     `json:"pending"`
	Complete          bool     `json:"complete"`
	IncompleteRemoved bool     `json:"incomplete_removed,omitempty"`
	RecoveredFiles    []string `json:"recovered_files"`
}

func runProjectRecover(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("project recover")
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("project recover", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	metadataInfo, statErr := os.Stat(filepath.Join(*projectPath, ".replan"))
	if statErr != nil || !metadataInfo.IsDir() {
		message := fmt.Sprintf("project path %s was not found", *projectPath)
		if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
			message = fmt.Sprintf("failed to stat project path %s: %v", *projectPath, statErr)
		}
		env := envelope{OK: false, Command: "project recover", ProjectPath: *projectPath, Result: nil, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{{Code: "project_not_found", Message: message, Severity: "error", Target: app.Target{Kind: "file", ID: *projectPath}}}}
		if printErr := printEnvelope(stdout, *format, env); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitNotFound
	}
	var (
		recovery filestore.CommitRecovery
		err      error
	)
	if dryRun {
		recovery, err = filestore.InspectCommit(*projectPath)
	} else {
		recovery, err = filestore.RecoverCommit(*projectPath)
	}
	result := projectRecoverResult{DryRun: dryRun, Pending: recovery.Pending, Complete: recovery.Complete, IncompleteRemoved: recovery.IncompleteRemoved, RecoveredFiles: recovery.RecoveredFiles}
	warnings := []app.Diagnostic{}
	errorsOut := []app.Diagnostic{}
	if err != nil {
		code := "commit_recovery_failed"
		if errors.Is(err, filestore.ErrCommitJournalCorrupt) {
			code = "commit_journal_corrupt"
		}
		errorsOut = append(errorsOut, app.Diagnostic{Code: code, Message: err.Error(), Severity: "error", Target: app.Target{Kind: "project", ID: *projectPath}})
	} else if recovery.Pending {
		result.Applied = !dryRun
		diagnosticCode := "recovered_commit"
		message := fmt.Sprintf("recovered interrupted commit for project %s; files: %v", *projectPath, recovery.RecoveredFiles)
		if dryRun {
			diagnosticCode = "interrupted_commit_pending"
			message = fmt.Sprintf("project %s has an interrupted commit pending; recovery would process files: %v", *projectPath, recovery.RecoveredFiles)
		} else if recovery.IncompleteRemoved {
			message = fmt.Sprintf("removed incomplete commit journal for project %s; files: []", *projectPath)
		}
		warnings = append(warnings, app.Diagnostic{Code: diagnosticCode, Message: message, Severity: "warning", Target: app.Target{Kind: "project", ID: *projectPath}})
	}
	env := envelope{OK: err == nil, Command: "project recover", ProjectPath: *projectPath, Result: result, Warnings: warnings, Errors: errorsOut}
	if printErr := printEnvelope(stdout, *format, env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}
	if errors.Is(err, filestore.ErrCommitJournalCorrupt) {
		return exitValidationFailed
	}
	if err != nil {
		return exitGeneralError
	}
	return exitSuccess
}

func runProjectMigrate(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("project migrate")
	projectPath, format := addProjectFormatFlags(fs, "json")
	reason := fs.String("reason", "", "why the project format is migrated")
	actorID := fs.String("actor", "", "actor id migrating the project")
	occurredAt := fs.String("occurred-at", "", "RFC3339 occurrence timestamp")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("project migrate", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewProjectService(newStore(*projectPath)).Migrate(context.Background(), app.ProjectMigrateRequest{
		Reason: *reason, ActorID: *actorID, OccurredAt: *occurredAt, DryRun: dryRun,
	})
	lines := []string{
		fmt.Sprintf("- Project: `%s`", result.ProjectID),
		fmt.Sprintf("- Format version: `%d` -> `%d`", result.FromVersion, result.ToVersion),
		fmt.Sprintf("- Steps: `%d`", len(result.Steps)),
	}
	for _, step := range result.Steps {
		lines = append(lines, fmt.Sprintf("- `%s`: %s", step.Name, step.Description))
	}
	return finishLifecycleCommand(stdout, stderr, *format, "project migrate", "Project Migrate", *projectPath, result.DryRun, result.Applied, result.WrittenFiles, lines, result, result.Warnings, result.Errors, err)
}

func runProjectSetStatusDate(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("project set-status-date")
	projectPath, format := addProjectFormatFlags(fs, "json")
	date := fs.String("date", "", "new status date: YYYY-MM-DD or today (in the project time zone)")
	reason := fs.String("reason", "", "why the status date moves")
	actorID := fs.String("actor", "", "actor id moving the status date")
	occurredAt := fs.String("occurred-at", "", "RFC3339 occurrence timestamp")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("project set-status-date", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewProjectService(newStore(*projectPath)).SetStatusDate(context.Background(), app.ProjectSetStatusDateRequest{
		Date:       *date,
		Reason:     *reason,
		ActorID:    *actorID,
		OccurredAt: *occurredAt,
		DryRun:     dryRun,
	})
	lines := []string{
		fmt.Sprintf("- Project: `%s`", result.ProjectID),
		fmt.Sprintf("- Status date: `%s` -> `%s`", result.PreviousDate, result.NewDate),
		fmt.Sprintf("- Default week: `%s`", result.Week),
	}
	for _, warning := range result.Warnings {
		lines = append(lines, fmt.Sprintf("- Warning `%s`: %s", warning.Code, warning.Message))
	}
	return finishLifecycleCommand(stdout, stderr, *format, "project set-status-date", "Project Set Status Date", *projectPath, result.DryRun, result.Applied, result.WrittenFiles, lines, result, result.Warnings, result.Errors, err)
}
