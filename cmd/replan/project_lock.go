package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
)

const (
	projectWriteLockRelativePath = ".replan/write.lock"
	projectLockedCode            = "project_locked"
)

var errProjectLockTimeout = errors.New("timed out waiting for the project write lock")

type projectWriteIntent struct {
	write         bool
	command       string
	projectPath   string
	format        string
	bindingsLayer bool
}

// projectWriteIntentForArgs is the single CLI-level inventory of commands
// that can write inside a project. It runs before command dispatch so loading,
// planning, validation, and commit all happen while the lock is held.
func projectWriteIntentForArgs(args []string) projectWriteIntent {
	intent := projectWriteIntent{
		command:     commandName(args),
		projectPath: flagString(args, ".", "project", "p"),
		format:      strings.ToLower(flagString(args, defaultCommandFormat(args), "format")),
	}
	if value, ok := flagStringSet(args, "bindings"); ok {
		intent.bindingsLayer = value != ""
	} else {
		intent.bindingsLayer = os.Getenv("REPLAN_OPENPROJECT_BINDINGS_LAYER") != ""
	}

	dryRun := flagBool(args, false, "dry-run")
	switch intent.command {
	case "init":
		// init creates .replan atomically and acquires the lock itself. Keeping
		// this out of the generic pre-dispatch lock is what makes two first
		// initializations race on one mkdir, with exactly one winner.
		intent.write = false
	case "import openproject", "import github":
		intent.write = !dryRun
	case "tracking suggest", "proposal suggest-dependencies":
		intent.write = flagBool(args, false, "write-candidates")
	case "taskjuggler import-result":
		intent.write = flagBool(args, false, "write-candidate")
	case "report weekly":
		intent.write = flagBool(args, false, "write")
	case "report dashboard":
		if output, ok := flagStringSet(args, "output"); ok && output != "" {
			intent.write = pathInsideProject(intent.projectPath, output)
		}
	case "export json", "export csv", "export taskjuggler":
		for _, name := range []string{"output", "manifest"} {
			if output, ok := flagStringSet(args, name); ok && output != "" && pathInsideProjectFromWorkingDirectory(intent.projectPath, output) {
				intent.write = true
			}
		}
	case "taskjuggler run":
		output, set := flagStringSet(args, "output-dir")
		intent.write = !set || strings.TrimSpace(output) == "" || pathInsideProjectFromWorkingDirectory(intent.projectPath, output)
	default:
		if isCanonicalWriteCommand(intent.command) {
			// Explicit dry-run wins over REPLAN_APPLY, matching the apply gate.
			intent.write = !dryRun && (flagBool(args, false, "apply") || os.Getenv("REPLAN_APPLY") == "1")
		}
	}
	return intent
}

func commandName(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
		candidate := args[0] + " " + args[1]
		if isCanonicalWriteCommand(candidate) {
			return candidate
		}
		switch candidate {
		case "import openproject", "import github", "tracking suggest", "proposal suggest-dependencies", "taskjuggler import-result", "taskjuggler run", "report weekly", "report dashboard", "export json", "export csv", "export taskjuggler":
			return candidate
		}
	}
	return args[0]
}

func defaultCommandFormat(args []string) string {
	switch commandName(args) {
	case "report weekly":
		return "markdown"
	case "report dashboard":
		return "html"
	case "export taskjuggler":
		return "text"
	default:
		return "json"
	}
}

func flagBool(args []string, fallback bool, name string) bool {
	value := fallback
	prefixes := []string{"--" + name, "-" + name}
	for _, arg := range args {
		for _, prefix := range prefixes {
			switch {
			case arg == prefix:
				value = true
			case strings.HasPrefix(arg, prefix+"="):
				if parsed, err := strconv.ParseBool(strings.TrimPrefix(arg, prefix+"=")); err == nil {
					value = parsed
				}
			}
		}
	}
	return value
}

func flagString(args []string, fallback string, names ...string) string {
	if value, ok := flagStringSet(args, names...); ok {
		return value
	}
	return fallback
}

func flagStringSet(args []string, names ...string) (string, bool) {
	value := ""
	set := false
	for i := 0; i < len(args); i++ {
		for _, name := range names {
			for _, prefix := range []string{"--" + name, "-" + name} {
				if args[i] == prefix && i+1 < len(args) {
					value, set = args[i+1], true
				} else if strings.HasPrefix(args[i], prefix+"=") {
					value, set = strings.TrimPrefix(args[i], prefix+"="), true
				}
			}
		}
	}
	return value, set
}

func pathInsideProjectFromWorkingDirectory(projectPath, outputPath string) bool {
	absolute, err := filepath.Abs(outputPath)
	if err != nil {
		return false
	}
	return pathInsideProject(projectPath, absolute)
}

func projectWriteLockTimeout() time.Duration {
	const defaultTimeout = 30 * time.Second
	raw := strings.TrimSpace(os.Getenv("REPLAN_LOCK_TIMEOUT_SECONDS"))
	if raw == "" {
		return defaultTimeout
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil || seconds <= 0 {
		return defaultTimeout
	}
	return time.Duration(seconds * float64(time.Second))
}

type projectWriteLock struct {
	file *os.File
}

func acquireProjectWriteLock(projectPath string, timeout time.Duration) (*projectWriteLock, error) {
	metadataDir := filepath.Join(projectPath, ".replan")
	if info, err := os.Stat(metadataDir); err != nil || !info.IsDir() {
		// Preserve the command's existing project-not-found or validation error;
		// do not create metadata in a path that is not already a project.
		return nil, nil
	}
	lockPath := filepath.Join(projectPath, filepath.FromSlash(projectWriteLockRelativePath))
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", lockPath, err)
	}
	deadline := time.Now().Add(timeout)
	for {
		locked, lockErr := tryProjectFileLock(file)
		if lockErr != nil {
			_ = file.Close()
			return nil, fmt.Errorf("lock %s: %w", lockPath, lockErr)
		}
		if locked {
			return &projectWriteLock{file: file}, nil
		}
		if !time.Now().Before(deadline) {
			_ = file.Close()
			return nil, errProjectLockTimeout
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (lock *projectWriteLock) close() {
	if lock == nil || lock.file == nil {
		return
	}
	_ = unlockProjectFile(lock.file)
	_ = lock.file.Close()
}

func printProjectLocked(stdout, stderr *os.File, intent projectWriteIntent, lockErr error) int {
	lockPath := filepath.Join(intent.projectPath, filepath.FromSlash(projectWriteLockRelativePath))
	message := fmt.Sprintf("project write lock %s is unavailable: %v", lockPath, lockErr)
	if intent.format == "json" {
		env := envelope{OK: false, Command: intent.command, ProjectPath: intent.projectPath, Result: nil, Warnings: []app.Diagnostic{}, Errors: []app.Diagnostic{{Code: projectLockedCode, Message: message, Severity: "error"}}}
		if err := printEnvelope(stdout, "json", env); err != nil {
			fmt.Fprintf(stderr, "failed to write output: %v\n", err)
			return exitGeneralError
		}
	} else {
		fmt.Fprintln(stderr, message)
	}
	return exitConflict
}
