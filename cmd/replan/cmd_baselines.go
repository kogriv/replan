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

func runBaseline(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing baseline subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "create":
		return runBaselineCreate(args[1:], stdout, stderr)
	case "list":
		return runBaselineList(args[1:], stdout, stderr)
	case "show":
		return runBaselineShow(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown baseline subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runBaselineCreate(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("baseline create")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", "json", "output format: json, text")
	id := fs.String("id", "", "required baseline id")
	name := fs.String("name", "", "required baseline name")
	approvedBy := fs.String("approved-by", "", "required existing approving actor id")
	statusDate := fs.String("status-date", "", "optional status date (YYYY-MM-DD)")
	var assumptions repeatedStringFlag
	fs.Var(&assumptions, "assumption", "optional repeatable baseline assumption")
	occurredAt := fs.String("occurred-at", "", "optional RFC3339 timestamp; defaults to now")
	writeFlags := addCanonicalWriteFlags(fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	formatValue := strings.ToLower(*format)
	if formatValue != "json" && formatValue != "text" {
		return printTaskFormatError("baseline create", *projectPath, stdout, stderr)
	}
	dryRun, code, done := requireCanonicalWriteChoice("baseline create", *projectPath, formatValue, writeFlags, stdout, stderr)
	if done {
		return code
	}
	result, err := app.NewBaselineService(newStore(*projectPath)).Create(context.Background(), app.BaselineCreateRequest{
		ID: *id, Name: *name, ApprovedBy: *approvedBy, StatusDate: *statusDate,
		Assumptions: assumptions, OccurredAt: *occurredAt, DryRun: dryRun,
	})
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "baseline create", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if formatValue == "text" && err == nil {
		if printErr := writeBaselineCreateText(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, formatValue, env, err)
}

func runBaselineList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("baseline list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewBaselineService(newStore(*projectPath)).List(context.Background())
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "baseline list", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "markdown") || strings.EqualFold(*format, "text") {
		if printErr := writeBaselineListMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runBaselineShow(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("baseline show")
	projectPath, format := addProjectFormatFlags(fs, "json")
	leadingID, filteredArgs := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, filteredArgs, stdout, stderr); done {
		return code
	}
	id := positionalArg(leadingID, fs)
	result, err := app.NewBaselineService(newStore(*projectPath)).Show(context.Background(), id)
	env := envelope{OK: err == nil && len(result.Errors) == 0, Command: "baseline show", ProjectPath: *projectPath, Result: result, Warnings: result.Warnings, Errors: result.Errors}
	if strings.EqualFold(*format, "markdown") || strings.EqualFold(*format, "text") {
		if printErr := writeBaselineShowMarkdown(stdout, result); printErr != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
			return exitGeneralError
		}
		if err != nil {
			return validationFailureExit(result.Errors)
		}
		return exitSuccess
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func runAudit(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing audit subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runAuditList(args[1:], stdout, stderr)
	case "verify":
		return runAuditVerify(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown audit subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runAuditList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("audit list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	since := fs.String("since", "", "include records at or after this timestamp/date prefix")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewAuditService(newStore(*projectPath)).List(context.Background(), app.AuditListRequest{Since: *since})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "audit list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printCommandResult(stdout, stderr, *format, result, env, err, writeAuditListMarkdown)
}

func runAuditVerify(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("audit verify")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewAuditService(newStore(*projectPath)).Verify(context.Background(), app.AuditVerifyRequest{})
	env := envelope{
		OK:          err == nil && result.Valid,
		Command:     "audit verify",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

func writeBaselineCreateText(stdout *os.File, result app.BaselineCreateResult) error {
	mode := "Dry run"
	if result.Applied {
		mode = "Applied"
	}
	if _, err := fmt.Fprintf(stdout, "Baseline create: %s\nFile: %s\nTasks: %d\nDemands: %d\n\nPreview:\n", mode, result.FilePath, result.TaskCount, result.DemandCount); err != nil {
		return err
	}
	data, err := yaml.Marshal(struct {
		Baseline domain.Baseline `yaml:"baseline"`
	}{Baseline: result.Baseline})
	if err != nil {
		return err
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	const previewLines = 24
	if len(lines) > previewLines {
		lines = append(lines[:previewLines], fmt.Sprintf("... (%d more lines)", len(lines)-previewLines))
	}
	_, err = fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	return err
}

func writeBaselineListMarkdown(stdout *os.File, result app.BaselineListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Baselines"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		for _, diagnostic := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if len(result.Baselines) == 0 {
		_, err := fmt.Fprintln(stdout, "No baselines.")
		return err
	}
	if _, err := fmt.Fprintln(stdout, "| ID | Status date | Tasks | Demands | Estimates | Seal |"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, "|---|---|---:|---:|---|---|"); err != nil {
		return err
	}
	for _, baseline := range result.Baselines {
		if _, err := fmt.Fprintf(stdout, "| `%s` | %s | %d | %d | %t | %s |\n", baseline.ID, emptyDash(baseline.StatusDate), baseline.Tasks, baseline.Demands, baseline.CarriesEstimates, baseline.SealStatus); err != nil {
			return err
		}
	}
	return nil
}

func writeBaselineShowMarkdown(stdout *os.File, result app.BaselineShowResult) error {
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "# Baseline"); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
		for _, diagnostic := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", diagnostic.Code, diagnostic.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "# Baseline: %s\n\n- Estimates: `%t`\n- Seal: `%s`\n\n```yaml\n", result.Baseline.ID, result.CarriesEstimates, result.SealStatus); err != nil {
		return err
	}
	data, err := yaml.Marshal(struct {
		Baseline domain.Baseline `yaml:"baseline"`
	}{Baseline: result.Baseline})
	if err != nil {
		return err
	}
	if _, err := stdout.Write(data); err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, "```")
	return err
}

func writeAuditListMarkdown(stdout *os.File, result app.AuditListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Audit Records"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Records) == 0 {
		_, err := fmt.Fprintln(stdout, "No audit records.")
		return err
	}
	for _, record := range result.Records {
		if _, err := fmt.Fprintf(stdout, "- `%s` %s by `%s`: `%s` -> `%s`\n",
			record.OccurredAt,
			record.ID,
			record.ActorID,
			record.Operation,
			record.TargetID); err != nil {
			return err
		}
	}
	return nil
}
