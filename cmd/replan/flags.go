package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kogriv/replan/internal/app"
)

type repeatedStringFlag []string

func (values *repeatedStringFlag) String() string { return strings.Join(*values, ",") }

func (values *repeatedStringFlag) Set(value string) error {
	*values = append(*values, value)
	return nil
}

func newCLIFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Func("bindings", "path to a read-only OpenProject private bindings layer (overrides REPLAN_OPENPROJECT_BINDINGS_LAYER)", func(value string) error {
		return os.Setenv("REPLAN_OPENPROJECT_BINDINGS_LAYER", value)
	})
	return fs
}

func refuseBindingsOutputInsideProject(command, projectPath, outputPath string, stdout, stderr *os.File) (int, bool) {
	if !bindingsLayerActive() || !pathInsideProject(projectPath, outputPath) {
		return exitSuccess, false
	}
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Warnings:    []app.Diagnostic{},
		Errors: []app.Diagnostic{{
			Code:     bindingsOutputCode,
			Message:  "the read-only OpenProject bindings layer cannot write output inside the project; choose an output path outside the project, drop --bindings, or unset REPLAN_OPENPROJECT_BINDINGS_LAYER",
			Severity: "error",
		}},
	}
	if err := printEnvelope(stdout, "json", env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitInvalidUsage, true
}

func refuseScenarioReportWrite(command, projectPath, scenarioID, outputPath string, stdout, stderr *os.File) (int, bool) {
	if scenarioID == "" || !pathInsideProject(projectPath, outputPath) {
		return exitSuccess, false
	}
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Warnings:    []app.Diagnostic{},
		Errors: []app.Diagnostic{{
			Code:     "scenario_report_write",
			Message:  "a scenario report is a what-if and cannot be written inside the project as the plan; choose an output path outside the project",
			Severity: "error",
			Target:   app.Target{Kind: "scenario", ID: scenarioID},
		}},
	}
	if err := printEnvelope(stdout, "json", env); err != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", err)
		return exitGeneralError, true
	}
	return exitInvalidUsage, true
}

// refuseBindingsExportInsideProject checks export files, whose relative paths
// resolve against the working directory rather than the project.
func refuseBindingsExportInsideProject(command, projectPath string, stdout, stderr *os.File, paths ...string) (int, bool) {
	for _, path := range paths {
		if path == "" {
			continue
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			absolute = path
		}
		if code, done := refuseBindingsOutputInsideProject(command, projectPath, absolute, stdout, stderr); done {
			return code, true
		}
	}
	return exitSuccess, false
}

func pathInsideProject(projectPath, outputPath string) bool {
	projectAbs, err := filepath.Abs(projectPath)
	if err != nil {
		return false
	}
	outputAbs := outputPath
	if !filepath.IsAbs(outputAbs) {
		outputAbs = filepath.Join(projectAbs, outputAbs)
	}
	outputAbs, err = filepath.Abs(outputAbs)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(projectAbs, outputAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// parseCommandFlags keeps flag package output on the channel that matches the
// result: help on stdout with exit 0, every other parse error on stderr with
// exit 8. Buffering avoids flag.Parse writing help to stderr before it returns
// flag.ErrHelp.
func parseCommandFlags(fs *flag.FlagSet, args []string, stdout, stderr *os.File) (int, bool) {
	var output bytes.Buffer
	fs.SetOutput(&output)
	err := fs.Parse(args)
	if err == nil {
		return exitSuccess, false
	}
	target := stderr
	code := exitInvalidUsage
	if errors.Is(err, flag.ErrHelp) {
		target = stdout
		code = exitSuccess
	}
	_, _ = target.Write(output.Bytes())
	return code, true
}

func addProjectFormatFlags(fs *flag.FlagSet, defaultFormat string) (*string, *string) {
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	format := fs.String("format", defaultFormat, "output format: json, markdown, text")
	return projectPath, format
}

func splitSinglePositional(args []string, fs *flag.FlagSet) (string, []string) {
	positional := ""
	filtered := make([]string, 0, len(args))
	expectFlagValue := false
	for i, arg := range args {
		if expectFlagValue {
			filtered = append(filtered, arg)
			expectFlagValue = false
			continue
		}
		if arg == "--" {
			filtered = append(filtered, arg)
			if positional == "" && i+1 < len(args) {
				positional = args[i+1]
				filtered = append(filtered, args[i+2:]...)
				return positional, filtered
			}
			continue
		}
		if isFlagToken(arg) {
			filtered = append(filtered, arg)
			name, hasInlineValue := flagName(arg)
			if !hasInlineValue && flagRequiresValue(fs, name) {
				expectFlagValue = true
			}
			continue
		}
		if positional == "" {
			positional = arg
			continue
		}
		filtered = append(filtered, arg)
	}
	return positional, filtered
}

func isFlagToken(arg string) bool {
	return strings.HasPrefix(arg, "-") && arg != "-"
}

func flagName(arg string) (string, bool) {
	name := strings.TrimLeft(arg, "-")
	if i := strings.Index(name, "="); i >= 0 {
		return name[:i], true
	}
	return name, false
}

func flagRequiresValue(fs *flag.FlagSet, name string) bool {
	flagValue := fs.Lookup(name)
	if flagValue == nil {
		return false
	}
	boolFlag, ok := flagValue.Value.(interface{ IsBoolFlag() bool })
	return !ok || !boolFlag.IsBoolFlag()
}

func positionalArg(leading string, fs *flag.FlagSet) string {
	if leading != "" {
		return leading
	}
	if fs.NArg() == 0 {
		return ""
	}
	return fs.Arg(0)
}

// validISOWeek reports whether week is a well-formed ISO week label such as
// 2026-W31. An empty string is not well-formed; callers that treat an empty
// week as "use the project default" must check emptiness separately.
func validISOWeek(week string) bool {
	if len(week) != 8 || week[4] != '-' || week[5] != 'W' {
		return false
	}
	year, err := strconv.Atoi(week[0:4])
	if err != nil || year < 1 {
		return false
	}
	number, err := strconv.Atoi(week[6:8])
	if err != nil || number < 1 || number > 53 {
		return false
	}
	return true
}

// printInvalidWeek emits a stable JSON envelope for a malformed --week value and
// returns the invalid-usage exit code.
func printInvalidWeek(stdout *os.File, command, projectPath, week string) int {
	env := envelope{
		OK:          false,
		Command:     command,
		ProjectPath: projectPath,
		Warnings:    []app.Diagnostic{},
		Errors: []app.Diagnostic{{
			Code:     "invalid_week",
			Message:  fmt.Sprintf("week %q is not a valid ISO week label; expected YYYY-Www such as 2026-W31", week),
			Severity: "error",
		}},
	}
	_ = printEnvelope(stdout, "json", env)
	return exitInvalidUsage
}
