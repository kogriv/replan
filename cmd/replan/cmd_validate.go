package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/kogriv/replan/internal/app"
)

func runValidate(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("validate")

	var (
		projectPath string
		format      string
		strict      bool
	)

	fs.StringVar(&projectPath, "project", "", "path to RePlan project")
	fs.StringVar(&projectPath, "p", "", "path to RePlan project")
	fs.StringVar(&format, "format", "text", "output format: json, markdown, text")
	fs.BoolVar(&strict, "strict", false, "treat validation warnings as errors")

	positionalProject := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		positionalProject = args[0]
		args = args[1:]
	}

	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}

	if projectPath == "" && positionalProject != "" {
		projectPath = positionalProject
	}
	if projectPath == "" && fs.NArg() > 0 {
		projectPath = fs.Arg(0)
	}
	if projectPath == "" {
		projectPath = "."
	}

	format = strings.ToLower(format)
	if format != "json" && format != "markdown" && format != "text" {
		printEnvelope(stdout, format, envelope{
			OK:          false,
			Command:     "validate",
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

	store := newStore(projectPath)
	service := app.NewValidateService(store)
	result, err := service.Validate(context.Background(), app.ValidateRequest{
		ProjectPath:       projectPath,
		WarningsAsErrors:  strict,
		IncludeObjectScan: true,
	})

	env := envelope{
		OK:          err == nil && result.Valid,
		Command:     "validate",
		ProjectPath: result.ProjectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}

	if printErr := printEnvelope(stdout, format, env); printErr != nil {
		fmt.Fprintf(stderr, "failed to write output: %v\n", printErr)
		return exitGeneralError
	}

	if err == nil && result.Valid {
		return exitSuccess
	}
	if errors.Is(err, app.ErrValidationFailed) || !result.Valid {
		return validationFailureExit(result.Errors)
	}
	return exitGeneralError
}
