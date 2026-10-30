package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/buildinfo"
	"github.com/kogriv/replan/internal/domain"
)

type versionResult struct {
	buildinfo.Info
	ProjectFormatVersion int `json:"project_format_version"`
}

func runVersion(args []string, stdout, stderr *os.File, includeFormatLine bool) int {
	fs := flag.NewFlagSet("version", flag.ContinueOnError)
	format := fs.String("format", "text", "output format: text, json")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(stderr, "version takes no arguments")
		return exitInvalidUsage
	}
	result := versionResult{Info: buildinfo.Current(), ProjectFormatVersion: domain.CurrentProjectFormatVersion}
	switch strings.ToLower(*format) {
	case "json":
		if !includeFormatLine {
			fmt.Fprintln(stderr, "--version only supports text output")
			return exitInvalidUsage
		}
		env := envelope{OK: true, Command: "version", Result: result, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{}}
		if err := printEnvelope(stdout, "json", env); err != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", err)
			return exitGeneralError
		}
	case "text":
		fmt.Fprintln(stdout, versionTextLine(result.Info))
		if includeFormatLine {
			fmt.Fprintf(stdout, "project format version: %d\n", result.ProjectFormatVersion)
		}
	default:
		fmt.Fprintln(stderr, "supported version formats are text and json")
		return exitInvalidUsage
	}
	return exitSuccess
}

func versionTextLine(info buildinfo.Info) string {
	parts := make([]string, 0, 3)
	if commit := info.ShortCommit(); commit != "" {
		parts = append(parts, commit)
	}
	if date := info.Date(); date != "" {
		parts = append(parts, date)
	}
	parts = append(parts, info.GoVersion+" "+info.OSArch)
	return fmt.Sprintf("replan %s (%s)", info.Version, strings.Join(parts, ", "))
}
