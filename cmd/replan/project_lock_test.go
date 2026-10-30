package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestConcurrentCLIWritersAreSerialized(t *testing.T) {
	const writers = 24
	project := copyCmdProjectFixture(t)

	type outcome struct {
		id     string
		exit   int
		stdout string
		stderr string
	}
	start := make(chan struct{})
	outcomes := make(chan outcome, writers)
	var group sync.WaitGroup
	for i := 0; i < writers; i++ {
		id := fmt.Sprintf("evidence.concurrent.%02d", i)
		group.Add(1)
		go func() {
			defer group.Done()
			stdout, stderr := tempOutputFiles(t)
			defer stdout.Close()
			defer stderr.Close()
			<-start
			exit := run([]string{
				"evidence", "add",
				"--project", project,
				"--id", id,
				"--type", "test_result",
				"--actor", "actor.human.operator",
				"--observed-at", "2026-10-30T12:00:00+07:00",
				"--summary", "Concurrent project writer regression test.",
				"--target", "task:task.op.1234",
				"--apply",
				"--format", "json",
			}, stdout, stderr)
			outcomes <- outcome{id: id, exit: exit, stdout: readFileString(t, stdout.Name()), stderr: readFileString(t, stderr.Name())}
		}()
	}
	close(start)
	group.Wait()
	close(outcomes)
	for result := range outcomes {
		if result.exit != exitSuccess {
			t.Errorf("writer %s exit = %d; stdout=%s stderr=%s", result.id, result.exit, result.stdout, result.stderr)
		}
	}
	if t.Failed() {
		return
	}

	workspace, diagnostics := filestore.New(project).Load(context.Background())
	for _, diagnostic := range diagnostics {
		if diagnostic.Severity == "error" {
			t.Fatalf("load diagnostic: %#v", diagnostic)
		}
	}
	evidenceIDs := make(map[string]bool, len(workspace.Evidence))
	for _, evidence := range workspace.Evidence {
		evidenceIDs[evidence.ID] = true
	}
	auditTargets := make(map[string]int, writers)
	for _, record := range workspace.AuditRecords {
		if record.Operation == "evidence_add" {
			auditTargets[record.TargetRef.ID]++
		}
	}
	for i := 0; i < writers; i++ {
		id := fmt.Sprintf("evidence.concurrent.%02d", i)
		if !evidenceIDs[id] {
			t.Errorf("missing evidence record %s", id)
		}
		if auditTargets[id] != 1 {
			t.Errorf("audit records targeting %s = %d, want 1", id, auditTargets[id])
		}
	}

	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()
	if exit := run([]string{"validate", project, "--format", "json"}, stdout, stderr); exit != exitSuccess {
		t.Fatalf("validate exit = %d; stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
}

func TestProjectWriteLockTimeoutRefusesWithoutChangingProject(t *testing.T) {
	project := copyCmdProjectFixture(t)
	held, err := acquireProjectWriteLock(project, time.Second)
	if err != nil || held == nil {
		t.Fatalf("acquire held lock: lock=%v err=%v", held, err)
	}
	defer held.close()
	before := snapshotProjectFiles(t, project)
	t.Setenv("REPLAN_LOCK_TIMEOUT_SECONDS", "0.1")

	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()
	exit := run([]string{
		"evidence", "add", "--project", project,
		"--id", "evidence.lock-timeout", "--type", "test_result",
		"--actor", "actor.human.operator", "--observed-at", "2026-10-30T12:00:00+07:00",
		"--summary", "Must not be written.", "--target", "task:task.op.1234",
		"--apply", "--format", "json",
	}, stdout, stderr)
	if exit != exitConflict {
		t.Fatalf("exit = %d, want %d; stdout=%s stderr=%s", exit, exitConflict, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	if !hasDiagnosticCode(env.Errors, projectLockedCode) {
		t.Fatalf("errors = %#v, want %s", env.Errors, projectLockedCode)
	}
	if !strings.Contains(env.Errors[0].Message, filepath.FromSlash(projectWriteLockRelativePath)) {
		t.Fatalf("message %q does not name lock file", env.Errors[0].Message)
	}
	after := snapshotProjectFiles(t, project)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("project changed while lock was held")
	}
}

func TestDryRunDoesNotWaitForProjectWriteLock(t *testing.T) {
	project := copyCmdProjectFixture(t)
	held, err := acquireProjectWriteLock(project, time.Second)
	if err != nil || held == nil {
		t.Fatalf("acquire held lock: lock=%v err=%v", held, err)
	}
	defer held.close()
	t.Setenv("REPLAN_LOCK_TIMEOUT_SECONDS", "2")

	stdout, stderr := tempOutputFiles(t)
	defer stdout.Close()
	defer stderr.Close()
	started := time.Now()
	exit := run([]string{
		"evidence", "add", "--project", project,
		"--id", "evidence.dry-run-under-lock", "--type", "test_result",
		"--actor", "actor.human.operator", "--observed-at", "2026-10-30T12:00:00+07:00",
		"--summary", "Dry run must not wait.", "--target", "task:task.op.1234",
		"--dry-run", "--format", "json",
	}, stdout, stderr)
	if exit != exitSuccess {
		t.Fatalf("exit = %d; stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("dry run waited %s for held project lock", elapsed)
	}
}

func TestProjectWriteIntentInventory(t *testing.T) {
	project := filepath.Join(string(os.PathSeparator), "tmp", "project")
	inside := filepath.Join(project, "exports", "result.json")
	outside := filepath.Join(string(os.PathSeparator), "tmp", "result.json")
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"canonical apply", []string{"evidence", "add", "--project", project, "--apply"}, true},
		{"canonical dry run", []string{"evidence", "add", "--project", project, "--dry-run"}, false},
		{"openproject snapshot", []string{"import", "openproject", "--project", project}, true},
		{"openproject dry run", []string{"import", "openproject", "--project", project, "--dry-run"}, false},
		{"github snapshot", []string{"import", "github", "--project", project}, true},
		{"tracking candidates", []string{"tracking", "suggest", "--project", project, "--write-candidates"}, true},
		{"dependency candidates", []string{"proposal", "suggest-dependencies", "--project", project, "--write-candidates"}, true},
		{"taskjuggler candidate", []string{"taskjuggler", "import-result", "--project", project, "--write-candidate"}, true},
		{"weekly report", []string{"report", "weekly", "--project", project, "--write"}, true},
		{"dashboard inside", []string{"report", "dashboard", "--project", project, "--output", inside}, true},
		{"dashboard outside", []string{"report", "dashboard", "--project", project, "--output", outside}, false},
		{"export inside", []string{"export", "json", "--project", project, "--output", inside}, true},
		{"export outside", []string{"export", "json", "--project", project, "--output", outside}, false},
		{"taskjuggler default run", []string{"taskjuggler", "run", "--project", project}, true},
		{"taskjuggler outside run", []string{"taskjuggler", "run", "--project", project, "--output-dir", outside}, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := projectWriteIntentForArgs(test.args).write; got != test.want {
				t.Fatalf("write intent = %t, want %t", got, test.want)
			}
		})
	}
}
