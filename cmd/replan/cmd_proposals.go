package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
)

func runDependency(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing dependency subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runDependencyList(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown dependency subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runDependencyList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("dependency list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewDependencyService(newStore(*projectPath)).List(context.Background(), app.DependencyListRequest{ProjectPath: *projectPath})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "dependency list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeDependencyListMarkdown(stdout, result); printErr != nil {
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

func runProposal(args []string, stdout, stderr *os.File) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "missing proposal subcommand")
		printUsage(stderr)
		return exitInvalidUsage
	}
	switch args[0] {
	case "list":
		return runProposalList(args[1:], stdout, stderr)
	case "show":
		return runProposalShow(args[1:], stdout, stderr)
	case "suggest-dependencies":
		return runProposalSuggestDependencies(args[1:], stdout, stderr)
	case "accept":
		return runProposalAccept(args[1:], stdout, stderr)
	case "reject":
		return runProposalReject(args[1:], stdout, stderr)
	case "-h", "--help":
		printUsage(stdout)
		return exitSuccess
	default:
		fmt.Fprintf(stderr, "unknown proposal subcommand: %s\n", args[0])
		printUsage(stderr)
		return exitInvalidUsage
	}
}

func runProposalList(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("proposal list")
	projectPath, format := addProjectFormatFlags(fs, "json")
	status := fs.String("status", "", "filter by proposal status")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := app.NewProposalService(newStore(*projectPath)).List(context.Background(), app.ProposalListRequest{Status: *status})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal list",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printCommandResult(stdout, stderr, *format, result, env, err, writeProposalListMarkdown)
}

func runProposalShow(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("proposal show")
	projectPath, format := addProjectFormatFlags(fs, "json")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	positionalID = positionalArg(positionalID, fs)
	if positionalID == "" {
		fmt.Fprintln(stderr, "missing proposal id")
		return exitInvalidUsage
	}
	result, err := app.NewProposalService(newStore(*projectPath)).Show(context.Background(), app.ProposalShowRequest{ID: positionalID})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal show",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeProposalShowMarkdown(stdout, result); printErr != nil {
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

func runProposalSuggestDependencies(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("proposal suggest-dependencies")
	projectPath, format := addProjectFormatFlags(fs, "json")
	actorID := fs.String("actor", "", "actor id preparing proposal candidates")
	dryRun := fs.Bool("dry-run", true, "preview dependency proposal candidates without writing state")
	writeCandidates := fs.Bool("write-candidates", false, "write dependency proposal candidate files without mutating canonical state")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if !*dryRun && !*writeCandidates {
		fmt.Fprintln(stderr, "proposal suggest-dependencies is preview-only; --dry-run=false is not supported")
		return exitInvalidUsage
	}
	result, err := app.NewProposalService(newStore(*projectPath)).SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{
		ActorID:         *actorID,
		WriteCandidates: *writeCandidates,
		Now:             time.Now(),
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal suggest-dependencies",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeProposalSuggestDependenciesMarkdown(stdout, result); printErr != nil {
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

func runProposalAccept(args []string, stdout, stderr *os.File) int {
	return runProposalDecision(args, stdout, stderr, "accept")
}

func runProposalReject(args []string, stdout, stderr *os.File) int {
	return runProposalDecision(args, stdout, stderr, "reject")
}

func runProposalDecision(args []string, stdout, stderr *os.File, decision string) int {
	fs := newCLIFlagSet("proposal " + decision)
	projectPath, format := addProjectFormatFlags(fs, "json")
	writeFlags := addCanonicalWriteFlags(fs)
	actorID := fs.String("actor", "", "actor id performing proposal decision; required unless --dry-run is used")
	reason := fs.String("reason", "", "decision reason")

	positionalID, args := splitSinglePositional(args, fs)
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	dryRun, code, done := requireCanonicalWriteChoice("proposal "+decision, *projectPath, *format, writeFlags, stdout, stderr)
	if done {
		return code
	}
	positionalID = positionalArg(positionalID, fs)
	if positionalID == "" {
		fmt.Fprintln(stderr, "missing proposal id")
		return exitInvalidUsage
	}
	req := app.ProposalDecisionRequest{
		ID:      positionalID,
		ActorID: *actorID,
		Reason:  *reason,
		DryRun:  dryRun,
		Now:     time.Now(),
	}
	service := app.NewProposalService(newStore(*projectPath))
	var (
		result app.ProposalDecisionResult
		err    error
	)
	if decision == "accept" {
		result, err = service.Accept(context.Background(), req)
	} else {
		result, err = service.Reject(context.Background(), req)
	}
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "proposal " + decision,
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	if *format == "markdown" || *format == "text" {
		if printErr := writeProposalDecisionMarkdown(stdout, result); printErr != nil {
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

func writeProposalListMarkdown(stdout *os.File, result app.ProposalListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Proposals"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Proposals) == 0 {
		_, err := fmt.Fprintln(stdout, "No proposals.")
		return err
	}
	for _, proposal := range result.Proposals {
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s, %s): %s\n", proposal.ID, proposal.Type, proposal.Status, proposal.Rationale); err != nil {
			return err
		}
	}
	return nil
}

func writeProposalShowMarkdown(stdout *os.File, result app.ProposalShowResult) error {
	proposal := result.Proposal
	if _, err := fmt.Fprintf(stdout, "# Proposal: %s\n\n", proposal.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Type: `%s`\n", proposal.Type); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Status: `%s`\n", proposal.Status); err != nil {
		return err
	}
	if proposal.CreatedBy.ID != "" {
		if _, err := fmt.Fprintf(stdout, "- Created by: `%s`\n", proposal.CreatedBy.ID); err != nil {
			return err
		}
	}
	if proposal.TargetRef.ID != "" {
		if _, err := fmt.Fprintf(stdout, "- Target: `%s/%s`\n", proposal.TargetRef.Kind, proposal.TargetRef.ID); err != nil {
			return err
		}
	}
	if proposal.Rationale != "" {
		if _, err := fmt.Fprintf(stdout, "\n## Rationale\n\n%s\n", proposal.Rationale); err != nil {
			return err
		}
	}
	if proposal.SuggestedChange.Op != "" {
		if _, err := fmt.Fprintf(stdout, "\n## Suggested Change\n\n- Operation: `%s`\n- Path: `%s`\n", proposal.SuggestedChange.Op, proposal.SuggestedChange.Path); err != nil {
			return err
		}
	}
	if len(proposal.EvidenceRefs) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Evidence"); err != nil {
			return err
		}
		for _, ref := range proposal.EvidenceRefs {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", ref.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeProposalSuggestDependenciesMarkdown(stdout *os.File, result app.ProposalSuggestDependenciesResult) error {
	if _, err := fmt.Fprintln(stdout, "# Dependency Proposal Candidates"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Dry run: `%t`\n", result.DryRun); err != nil {
		return err
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "- Applied: `true`"); err != nil {
			return err
		}
	}
	if len(result.Proposals) == 0 {
		_, err := fmt.Fprintln(stdout, "\nNo dependency proposal candidates.")
		return err
	}
	if _, err := fmt.Fprintln(stdout, "\n## Candidates"); err != nil {
		return err
	}
	for _, proposal := range result.Proposals {
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s): %s\n", proposal.ID, proposal.Type, proposal.Rationale); err != nil {
			return err
		}
		if proposal.SuggestedChange.Op != "" {
			if _, err := fmt.Fprintf(stdout, "  - Change: `%s` `%s`\n", proposal.SuggestedChange.Op, proposal.SuggestedChange.Path); err != nil {
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
	} else if len(result.PlannedWrites) > 0 {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s` `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	return nil
}

func writeProposalDecisionMarkdown(stdout *os.File, result app.ProposalDecisionResult) error {
	title := "# Proposal " + titleWord(result.Decision)
	if result.DryRun {
		title += " Dry Run"
	}
	if _, err := fmt.Fprintln(stdout, title); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Proposal: `%s`\n", result.Proposal.ID); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Status: `%s` -> `%s`\n", result.PreviousStatus, result.NewStatus); err != nil {
		return err
	}
	if result.Reason != "" {
		if _, err := fmt.Fprintf(stdout, "- Reason: %s\n", result.Reason); err != nil {
			return err
		}
	}
	if result.Applied {
		if _, err := fmt.Fprintln(stdout, "\n## Written Files"); err != nil {
			return err
		}
		for _, path := range result.WrittenFiles {
			if _, err := fmt.Fprintf(stdout, "- `%s`\n", path); err != nil {
				return err
			}
		}
	} else {
		if _, err := fmt.Fprintln(stdout, "\n## Planned Writes"); err != nil {
			return err
		}
		for _, write := range result.PlannedWrites {
			if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", write.Action, write.Path); err != nil {
				return err
			}
		}
	}
	if result.AuditRecordPreview.ID != "" {
		heading := "Audit Preview"
		if result.Applied {
			heading = "Audit"
		}
		if _, err := fmt.Fprintf(stdout, "\n## %s\n", heading); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "- `%s`: `%s`\n", result.AuditRecordPreview.ID, result.AuditRecordPreview.Operation); err != nil {
			return err
		}
	}
	return nil
}

//nolint:gocyclo // This flat renderer writes three short ordered sections; splitting it would hide the exact output sequence behind indirection.
func writeDependencyListMarkdown(stdout *os.File, result app.DependencyListResult) error {
	if _, err := fmt.Fprintln(stdout, "# Dependencies"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		if _, err := fmt.Fprintln(stdout, "## Errors"); err != nil {
			return err
		}
		for _, d := range result.Errors {
			if _, err := fmt.Fprintf(stdout, "- `%s`: %s\n", d.Code, d.Message); err != nil {
				return err
			}
		}
		return nil
	}
	if _, err := fmt.Fprintf(stdout, "- Dependencies: `%d`\n", result.Summary.Dependencies); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Required open: `%d`\n", result.Summary.RequiredOpen); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Active blockers: `%d`\n", result.Summary.ActiveBlockers); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Resolvable: `%d`\n", result.Summary.Resolvable); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "- Ready to unblock tasks: `%d`\n", result.Summary.ReadyToUnblockTasks); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(stdout, "\n## Active Blockers"); err != nil {
		return err
	}
	wrote := false
	for _, row := range result.Dependencies {
		if !row.Blocking {
			continue
		}
		wrote = true
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s, %s) -> `%s`", row.ID, row.Type, row.Status, row.SuccessorID); err != nil {
			return err
		}
		if row.SuccessorStatus != "" {
			if _, err := fmt.Fprintf(stdout, " [%s]", row.SuccessorStatus); err != nil {
				return err
			}
		}
		if len(row.OpenProposalIDs) > 0 {
			if _, err := fmt.Fprintf(stdout, " proposals=%s", strings.Join(row.OpenProposalIDs, ",")); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(stdout); err != nil {
			return err
		}
	}
	if !wrote {
		if _, err := fmt.Fprintln(stdout, "No active blockers."); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(stdout, "\n## Resolvable Dependencies"); err != nil {
		return err
	}
	wrote = false
	for _, row := range result.Dependencies {
		if !row.Resolvable {
			continue
		}
		wrote = true
		if _, err := fmt.Fprintf(stdout, "- `%s` predecessor `%s` can resolve\n", row.ID, row.PredecessorID); err != nil {
			return err
		}
	}
	if !wrote {
		if _, err := fmt.Fprintln(stdout, "No resolvable dependencies."); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(stdout, "\n## Blocked Successors"); err != nil {
		return err
	}
	if len(result.BlockedSuccessors) == 0 {
		if _, err := fmt.Fprintln(stdout, "No blocked successors with required dependencies."); err != nil {
			return err
		}
		return nil
	}
	for _, row := range result.BlockedSuccessors {
		state := "blocked"
		if row.ReadyToUnblock {
			state = "ready_to_unblock"
		}
		if _, err := fmt.Fprintf(stdout, "- `%s` (%s): required=%s open=%s\n", row.TaskID, state, strings.Join(row.RequiredDependencyIDs, ","), strings.Join(row.OpenDependencyIDs, ",")); err != nil {
			return err
		}
	}
	return nil
}
