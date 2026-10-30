package main

import (
	"strings"
	"testing"
)

func TestActualCorrectCLIApplyAndList(t *testing.T) {
	project := copyCmdProjectFixture(t)
	stdout, stderr := tempOutputFiles(t)
	args := []string{"actual", "correct", "--project", project, "--actual", "actual.repair-pump.petr.20260706.1", "--amount", "0.1", "--reason", "Correct guessed duration.", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--occurred-at", "2026-10-30T10:00:00+07:00", "--apply", "--format", "json"}
	if code := run(args, stdout, stderr); code != exitSuccess {
		t.Fatalf("correct exit = %d; stdout=%s stderr=%s", code, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	assertResultField(t, env, "applied", true)

	stdout, stderr = tempOutputFiles(t)
	if code := run([]string{"actual", "list", "--project", project, "--format", "markdown"}, stdout, stderr); code != exitSuccess {
		t.Fatalf("list exit = %d; stderr=%s", code, readFileString(t, stderr.Name()))
	}
	listed := readFileString(t, stdout.Name())
	for _, want := range []string{"Corrected To", "actual.repair-pump.petr.20260706.1", "correction", "0.1"} {
		if !strings.Contains(listed, want) {
			t.Fatalf("actual list missing %q:\n%s", want, listed)
		}
	}
}

func TestActualCorrectCLIRequiresExactlyOneAmountChoice(t *testing.T) {
	project := copyCmdProjectFixture(t)
	for _, extra := range [][]string{nil, {"--amount", "1", "--void"}} {
		stdout, stderr := tempOutputFiles(t)
		args := []string{"actual", "correct", "--project", project, "--actual", "actual.repair-pump.petr.20260706.1", "--reason", "reason", "--actor", "actor.human.operator", "--evidence", "evidence.github.pr.45", "--dry-run", "--format", "json"}
		args = append(args, extra...)
		if code := run(args, stdout, stderr); code != exitInvalidUsage {
			t.Fatalf("exit = %d, stdout=%s", code, readFileString(t, stdout.Name()))
		}
		env := readCLIEnvelope(t, stdout.Name())
		if !hasDiagnosticCode(env.Errors, "actual_correction_choice_required") {
			t.Fatalf("errors = %#v", env.Errors)
		}
	}
}
