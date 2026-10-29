package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEveryCanonicalWriterUsesSharedApplyGate(t *testing.T) {
	t.Setenv("REPLAN_APPLY", "")

	for _, command := range canonicalWriteCommands {
		command := command
		t.Run(command, func(t *testing.T) {
			for _, tc := range []struct {
				name string
				args []string
				code string
			}{
				{name: "choice required", code: applyChoiceRequiredCode},
				{name: "choice conflict", args: []string{"--dry-run", "--apply"}, code: applyChoiceConflictCode},
			} {
				t.Run(tc.name, func(t *testing.T) {
					stdout, stderr := tempOutputFiles(t)
					args := append(strings.Fields(command), tc.args...)
					args = append(args, "--format", "json")
					if code := run(args, stdout, stderr); code != exitInvalidUsage {
						t.Fatalf("run(%q) exit = %d, want %d; stdout=%s stderr=%s", args, code, exitInvalidUsage, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
					}
					env := readCLIEnvelope(t, stdout.Name())
					if env.OK || !hasDiagnosticCode(env.Errors, tc.code) {
						t.Fatalf("run(%q) errors = %#v, want %s", args, env.Errors, tc.code)
					}
				})
			}

			stdout, stderr := tempOutputFiles(t)
			helpArgs := append(strings.Fields(command), "--help")
			if code := run(helpArgs, stdout, stderr); code != exitSuccess {
				t.Fatalf("run(%q) exit = %d, want %d", helpArgs, code, exitSuccess)
			}
			help := readFileString(t, stdout.Name())
			for _, flagName := range []string{"--apply", "--dry-run"} {
				if !strings.Contains(help, flagName) {
					t.Errorf("help missing %s:\n%s", flagName, help)
				}
			}
		})
	}
}

func TestCanonicalWriteChoiceControlsMutation(t *testing.T) {
	tests := []struct {
		name        string
		env         string
		flags       []string
		wantExit    int
		wantCode    string
		wantApplied bool
	}{
		{name: "neither refused", wantExit: exitInvalidUsage, wantCode: applyChoiceRequiredCode},
		{name: "both refused", flags: []string{"--dry-run", "--apply"}, wantExit: exitInvalidUsage, wantCode: applyChoiceConflictCode},
		{name: "dry run writes nothing", flags: []string{"--dry-run"}, wantExit: exitSuccess},
		{name: "apply writes", flags: []string{"--apply"}, wantExit: exitSuccess, wantApplied: true},
		{name: "environment apply writes", env: "1", wantExit: exitSuccess, wantApplied: true},
		{name: "dry run wins over environment", env: "1", flags: []string{"--dry-run"}, wantExit: exitSuccess},
		{name: "environment zero refused", env: "0", wantExit: exitInvalidUsage, wantCode: applyChoiceRequiredCode},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("REPLAN_APPLY", tc.env)
			projectDir := copyCmdProjectFixture(t)
			before := snapshotProjectFiles(t, projectDir)
			args := []string{
				"actual", "add-status",
				"--project", projectDir,
				"--task", "task.op.1234",
				"--status", "in_progress",
				"--reason", "Apply gate test.",
				"--actor", "actor.human.operator",
				"--occurred-at", "2026-10-26T12:00:00+07:00",
				"--status-date", "2026-10-26",
			}
			args = append(args, tc.flags...)
			args = append(args, "--format", "json")

			stdout, stderr := tempOutputFiles(t)
			if code := run(args, stdout, stderr); code != tc.wantExit {
				t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", code, tc.wantExit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
			}
			env := readCLIEnvelope(t, stdout.Name())
			if tc.wantCode != "" {
				if !hasDiagnosticCode(env.Errors, tc.wantCode) {
					t.Fatalf("errors = %#v, want %s", env.Errors, tc.wantCode)
				}
				if tc.wantCode == applyChoiceRequiredCode {
					message := env.Errors[0].Message
					for _, required := range []string{"--dry-run", "--apply", "REPLAN_APPLY=1"} {
						if !strings.Contains(message, required) {
							t.Errorf("refusal message %q does not name %s", message, required)
						}
					}
				}
			}

			after := snapshotProjectFiles(t, projectDir)
			changed := !reflect.DeepEqual(before, after)
			if changed != tc.wantApplied {
				t.Fatalf("project changed = %t, want %t", changed, tc.wantApplied)
			}
			if tc.wantExit == exitSuccess {
				assertResultField(t, env, "dry_run", !tc.wantApplied)
				assertResultField(t, env, "applied", tc.wantApplied)
			}
		})
	}
}

func snapshotProjectFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatalf("snapshot project %s: %v", root, err)
	}
	return result
}
