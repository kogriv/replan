package main

import (
	"context"
	"fmt"
	"os"

	"github.com/kogriv/replan/internal/app"
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
