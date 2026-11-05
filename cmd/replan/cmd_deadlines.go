package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

func runDeadline(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing deadline subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "add":
		return runDeadlineAdd(args[1:], stdout, stderr)
	case "list":
		return runDeadlineList(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown deadline subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

type deadlineBatchFile struct {
	Deadlines []struct {
		ID          string `yaml:"id"`
		Task        string `yaml:"task"`
		Milestone   string `yaml:"milestone"`
		Date        string `yaml:"date"`
		Type        string `yaml:"type"`
		Criticality string `yaml:"criticality"`
		Timezone    string `yaml:"timezone"`
		Source      string `yaml:"source"`
		Description string `yaml:"description"`
	} `yaml:"deadlines"`
}

func runDeadlineAdd(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("deadline add")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	id := fs.String("id", "", "required deadline id")
	task := fs.String("task", "", "target task id")
	milestone := fs.String("milestone", "", "target milestone task id")
	date := fs.String("date", "", "YYYY-MM-DD, or RFC3339 for due_timestamp")
	deadlineType := fs.String("type", "due_date", "deadline type")
	criticality := fs.String("criticality", "hard", "deadline criticality")
	timezone := fs.String("timezone", "", "optional IANA timezone")
	source := fs.String("source", "", "optional source label")
	description := fs.String("description", "", "optional description")
	fromFile := fs.String("from-file", "", "YAML file containing a deadlines list")
	reason := fs.String("reason", "", "required audit reason")
	actor := fs.String("actor", "", "required existing audit actor")
	evidence := fs.String("evidence", "", "required existing evidence id shared by the batch")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp with an offset")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if strings.ToLower(*format) != "json" && strings.ToLower(*format) != "text" {
		return printTaskFormatError("deadline add", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("deadline add", *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	items := []app.DeadlineAddItem{{ID: *id, TaskID: *task, MilestoneID: *milestone, Date: *date, Type: *deadlineType, Criticality: *criticality, Timezone: *timezone, Source: *source, Description: *description}}
	if *fromFile != "" {
		data, readErr := os.ReadFile(*fromFile)
		var batch deadlineBatchFile
		if readErr == nil {
			readErr = yaml.Unmarshal(data, &batch)
		}
		if readErr != nil {
			return printInputFileError("deadline add", *projectPath, *format, *fromFile, readErr, stdout, stderr)
		}
		items = make([]app.DeadlineAddItem, 0, len(batch.Deadlines))
		for _, item := range batch.Deadlines {
			items = append(items, app.DeadlineAddItem{ID: item.ID, TaskID: item.Task, MilestoneID: item.Milestone, Date: item.Date, Type: item.Type, Criticality: item.Criticality, Timezone: item.Timezone, Source: item.Source, Description: item.Description})
		}
	}
	result, err := app.NewDeadlineService(newStore(*projectPath)).Add(context.Background(), app.DeadlineAddRequest{Items: items, Reason: *reason, ActorID: *actor, EvidenceID: *evidence, OccurredAt: *occurredAt, DryRun: dryRun})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "deadline add", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "text") && err == nil {
		return writeAddItemsText(stdout, "Deadlines", result.DryRun, result.Applied, deadlineIDs(result.Deadlines))
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func deadlineIDs(items []domain.Deadline) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.ID)
	}
	return ids
}

func runDeadlineList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("deadline list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewDeadlineService(newStore(*projectPath)).List(context.Background())
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "deadline list", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if *format == "markdown" {
		if printErr := writeDeadlineListMarkdown(stdout, result); printErr != nil {
			fmt.Fprintln(stderr, printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func writeDeadlineListMarkdown(stdout *os.File, result app.DeadlineListResult) error {
	if _, err := fmt.Fprintf(stdout, "# Deadlines\n\n- Deadlines: `%d`\n- Hard: `%d`\n\n", result.Summary.Deadlines, result.Summary.Hard); err != nil {
		return err
	}
	if len(result.Deadlines) == 0 {
		_, err := fmt.Fprintln(stdout, "No deadlines.")
		return err
	}
	for _, deadline := range result.Deadlines {
		date := deadline.DueDate
		if date == "" {
			date = deadline.DueTimestamp
		}
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s, %s): `%s` -> `%s`\n", deadline.ID, deadline.Type, deadline.Criticality, deadline.TargetRef.ID, date); err != nil {
			return err
		}
	}
	return nil
}
