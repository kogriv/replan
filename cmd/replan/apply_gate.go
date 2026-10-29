package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

const (
	applyChoiceRequiredCode = "apply_choice_required"
	applyChoiceConflictCode = "apply_choice_conflict"
	bindingsReadOnlyCode    = "bindings_layer_read_only"
)

var canonicalWriteCommands = []string{
	"actual acknowledge-overrun",
	"actual add-consumption",
	"actual add-status",
	"actual close-demand",
	"actual reopen-demand",
	"actual set-remaining",
	"baseline create",
	"evidence add",
	"import accept",
	"project migrate",
	"project set-status-date",
	"proposal accept",
	"proposal reject",
	"scenario apply",
	"task add",
	"task record-actual-window",
	"task set-dates",
	"task set-title",
}

type canonicalWriteFlags struct {
	dryRun *bool
	apply  *bool
}

func addCanonicalWriteFlags(fs *flag.FlagSet) canonicalWriteFlags {
	return canonicalWriteFlags{
		dryRun: fs.Bool("dry-run", false, "preview without writing canonical state; choose exactly one of --dry-run or --apply (REPLAN_APPLY=1 supplies --apply, but --dry-run wins)"),
		apply:  fs.Bool("apply", false, "write canonical state; choose exactly one of --dry-run or --apply (REPLAN_APPLY=1 supplies --apply)"),
	}
}

func requireCanonicalWriteChoice(command, projectPath, format string, flags canonicalWriteFlags, stdout, stderr *os.File) (dryRun bool, code int, done bool) {
	if !isCanonicalWriteCommand(command) {
		fmt.Fprintf(stderr, "internal error: unregistered canonical writer %q\n", command)
		return false, exitGeneralError, true
	}
	if code, done := refuseBindingsLayerWrite(command, projectPath, format, stdout, stderr); done {
		return false, code, true
	}
	if *flags.dryRun && *flags.apply {
		message := fmt.Sprintf("%s writes canonical state: --dry-run and --apply cannot be used together", command)
		return false, printApplyChoiceError(command, projectPath, format, applyChoiceConflictCode, message, stdout, stderr), true
	}
	if *flags.dryRun {
		return true, exitSuccess, false
	}
	if *flags.apply || os.Getenv("REPLAN_APPLY") == "1" {
		return false, exitSuccess, false
	}
	message := fmt.Sprintf("%s writes canonical state: pass --dry-run to preview or --apply (or REPLAN_APPLY=1) to write", command)
	return false, printApplyChoiceError(command, projectPath, format, applyChoiceRequiredCode, message, stdout, stderr), true
}

// refuseBindingsLayerWrite stops any command that would write project files
// while the read-only OpenProject bindings layer is active.
func refuseBindingsLayerWrite(command, projectPath, format string, stdout, stderr *os.File) (int, bool) {
	if !bindingsLayerActive() {
		return exitSuccess, false
	}
	message := fmt.Sprintf("%s cannot write project files with the read-only OpenProject bindings layer; drop --bindings or unset REPLAN_OPENPROJECT_BINDINGS_LAYER", command)
	return printApplyChoiceError(command, projectPath, format, bindingsReadOnlyCode, message, stdout, stderr), true
}

func bindingsLayerActive() bool {
	return os.Getenv("REPLAN_OPENPROJECT_BINDINGS_LAYER") != ""
}

// newStore opens the project store, with the read-only OpenProject bindings
// layer when --bindings or REPLAN_OPENPROJECT_BINDINGS_LAYER asks for it.
func newStore(projectPath string) filestore.Store {
	if path := os.Getenv("REPLAN_OPENPROJECT_BINDINGS_LAYER"); path != "" {
		return filestore.NewWithOpenProjectBindings(projectPath, path)
	}
	return filestore.New(projectPath)
}

func isCanonicalWriteCommand(command string) bool {
	for _, candidate := range canonicalWriteCommands {
		if command == candidate {
			return true
		}
	}
	return false
}

func printApplyChoiceError(command, projectPath, format, diagnosticCode, message string, stdout, stderr *os.File) int {
	if strings.EqualFold(format, "json") {
		env := envelope{
			OK:          false,
			Command:     command,
			ProjectPath: projectPath,
			Result:      nil,
			Warnings:    []app.Diagnostic{},
			Errors: []app.Diagnostic{{
				Code:     diagnosticCode,
				Message:  message,
				Severity: "error",
			}},
		}
		if err := printEnvelope(stdout, "json", env); err != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", err)
			return exitGeneralError
		}
	} else {
		fmt.Fprintln(stderr, message)
	}
	return exitInvalidUsage
}
