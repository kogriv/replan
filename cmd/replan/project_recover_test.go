package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestProjectRecoverApplyCompletesPendingDataAndAudit(t *testing.T) {
	project := copyCmdProjectFixture(t)
	files := interruptedEvidenceFiles(t, project, "explicit")
	writeTestCommitJournal(t, project, files)

	stdout, stderr := tempOutputFiles(t)
	if exit := run([]string{"validate", project, "--format", "json"}, stdout, stderr); exit != exitSuccess {
		t.Fatalf("validate pending exit=%d stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if env := readCLIEnvelope(t, stdout.Name()); !hasDiagnosticCode(env.Warnings, "interrupted_commit_pending") {
		t.Fatalf("warnings = %#v, want interrupted_commit_pending", env.Warnings)
	}

	stdout, stderr = tempOutputFiles(t)
	if exit := run([]string{"project", "recover", "--project", project, "--apply", "--format", "json"}, stdout, stderr); exit != exitSuccess {
		t.Fatalf("project recover exit=%d stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	if !hasDiagnosticCode(env.Warnings, "recovered_commit") {
		t.Fatalf("warnings = %#v, want recovered_commit", env.Warnings)
	}
	var result projectRecoverResult
	if err := json.Unmarshal(env.Result, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Applied || len(result.RecoveredFiles) != 2 {
		t.Fatalf("result = %#v", result)
	}
	assertRecoveredProjectValid(t, project, "explicit")
}

func TestNextOrdinaryWriteRecoversPendingCommitFirst(t *testing.T) {
	project := copyCmdProjectFixture(t)
	writeTestCommitJournal(t, project, interruptedEvidenceFiles(t, project, "automatic"))

	stdout, stderr := tempOutputFiles(t)
	exit := run([]string{
		"evidence", "add", "--project", project,
		"--id", "evidence.after-recovery", "--type", "test_result",
		"--actor", "actor.human.operator", "--observed-at", "2031-01-07T10:00:00Z",
		"--summary", "Ordinary write after recovery.", "--target", "task:task.op.1234",
		"--apply", "--format", "json",
	}, stdout, stderr)
	if exit != exitSuccess {
		t.Fatalf("ordinary write exit=%d stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	if !hasDiagnosticCode(env.Warnings, "recovered_commit") {
		t.Fatalf("warnings = %#v, want recovered_commit", env.Warnings)
	}
	assertRecoveredProjectValid(t, project, "automatic")
	workspace, _ := filestore.New(project).Load(context.Background())
	if countCLIRecords(workspace.Evidence, func(id string) bool { return id == "evidence.after-recovery" }) != 1 {
		t.Fatal("ordinary evidence write is not present exactly once")
	}
}

func TestProjectRecoverCorruptJournalRefusesWithoutChanges(t *testing.T) {
	project := copyCmdProjectFixture(t)
	writeTestCommitJournal(t, project, interruptedEvidenceFiles(t, project, "corrupt"))
	blob := filepath.Join(project, ".replan", "commit-pending", "blobs", "000000")
	if err := os.WriteFile(blob, []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireProjectWriteLock(project, projectWriteLockTimeout())
	if err != nil {
		t.Fatal(err)
	}
	lock.close()
	before := snapshotProjectFiles(t, project)

	stdout, stderr := tempOutputFiles(t)
	exit := run([]string{"project", "recover", "--project", project, "--apply", "--format", "json"}, stdout, stderr)
	if exit != exitValidationFailed {
		t.Fatalf("exit=%d stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	env := readCLIEnvelope(t, stdout.Name())
	if !hasDiagnosticCode(env.Errors, "commit_journal_corrupt") || !strings.Contains(env.Errors[0].Message, ".replan/commit-pending/") {
		t.Fatalf("errors = %#v", env.Errors)
	}
	after := snapshotProjectFiles(t, project)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("corrupt recovery changed project or journal files")
	}
}

func TestProjectRecoverNothingPendingAndDryRunCreateNoJournal(t *testing.T) {
	project := copyCmdProjectFixture(t)
	stdout, stderr := tempOutputFiles(t)
	if exit := run([]string{"project", "recover", "--project", project, "--apply", "--format", "json"}, stdout, stderr); exit != exitSuccess {
		t.Fatalf("empty recover exit=%d stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	var result projectRecoverResult
	if err := json.Unmarshal(readCLIEnvelope(t, stdout.Name()).Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.Pending || result.Applied || len(result.RecoveredFiles) != 0 {
		t.Fatalf("result = %#v", result)
	}

	stdout, stderr = tempOutputFiles(t)
	if exit := run([]string{
		"evidence", "add", "--project", project,
		"--id", "evidence.dry-run-journal", "--type", "test_result",
		"--actor", "actor.human.operator", "--observed-at", "2031-01-07T10:00:00Z",
		"--summary", "Dry run.", "--target", "task:task.op.1234",
		"--dry-run", "--format", "json",
	}, stdout, stderr); exit != exitSuccess {
		t.Fatalf("dry run exit=%d stdout=%s stderr=%s", exit, readFileString(t, stdout.Name()), readFileString(t, stderr.Name()))
	}
	if _, err := os.Stat(filepath.Join(project, ".replan", "commit-pending")); !os.IsNotExist(err) {
		t.Fatalf("dry run left commit journal: %v", err)
	}
}

func interruptedEvidenceFiles(t *testing.T, project, suffix string) map[string][]byte {
	t.Helper()
	evidenceID := "evidence.interrupted-" + suffix
	evidence := []byte("evidence:\n  - id: " + evidenceID + "\n    type: test_result\n    actor_ref:\n      kind: actor\n      id: actor.human.operator\n    observed_at: 2031-01-06T10:00:00Z\n    summary: Recovered evidence.\n    targets:\n      - kind: task\n        id: task.op.1234\n")
	auditID := "audit.interrupted-" + suffix
	auditRecord := map[string]any{
		"id": auditID, "occurred_at": "2031-01-06T10:00:00Z",
		"actor_ref": map[string]string{"kind": "actor", "id": "actor.human.operator"},
		"operation": "evidence_add", "target_ref": map[string]string{"kind": "evidence", "id": evidenceID}, "result": "success",
	}
	audit, err := json.Marshal(auditRecord)
	if err != nil {
		t.Fatal(err)
	}
	return map[string][]byte{"evidence/evidence.2031-W02.yaml": evidence, "audit/audit.2031-01.jsonl": append(audit, '\n')}
}

func writeTestCommitJournal(t *testing.T, project string, files map[string][]byte) {
	t.Helper()
	type write struct {
		Path   string `json:"path"`
		Blob   string `json:"blob"`
		SHA256 string `json:"sha256"`
	}
	type operation struct {
		Action string `json:"action"`
		Path   string `json:"path"`
	}
	type manifest struct {
		Version  int         `json:"version"`
		Writes   []write     `json:"writes"`
		Removals []string    `json:"removals"`
		Order    []operation `json:"order"`
	}
	journal := filepath.Join(project, ".replan", "commit-pending")
	if err := os.MkdirAll(filepath.Join(journal, "blobs"), 0o700); err != nil {
		t.Fatal(err)
	}
	paths := []string{"evidence/evidence.2031-W02.yaml", "audit/audit.2031-01.jsonl"}
	doc := manifest{Version: 1, Writes: []write{}, Removals: []string{}, Order: []operation{}}
	for i, rel := range paths {
		content := files[rel]
		blob := filepath.ToSlash(filepath.Join("blobs", "00000"+string(rune('0'+i))))
		if err := os.WriteFile(filepath.Join(journal, filepath.FromSlash(blob)), content, 0o600); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(content)
		doc.Writes = append(doc.Writes, write{Path: rel, Blob: blob, SHA256: hex.EncodeToString(sum[:])})
		doc.Order = append(doc.Order, operation{Action: "write", Path: rel})
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journal, "manifest.json"), append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
}

func assertRecoveredProjectValid(t *testing.T, project, suffix string) {
	t.Helper()
	validation, err := app.NewValidateService(filestore.New(project)).Validate(context.Background(), app.ValidateRequest{IncludeObjectScan: true})
	if err != nil || !validation.Valid {
		t.Fatalf("validate valid=%t err=%v errors=%#v", validation.Valid, err, validation.Errors)
	}
	verify, err := app.NewAuditService(filestore.New(project)).Verify(context.Background(), app.AuditVerifyRequest{})
	if err != nil || !verify.Valid {
		t.Fatalf("audit verify valid=%t err=%v errors=%#v", verify.Valid, err, verify.Errors)
	}
	workspace, diags := filestore.New(project).Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %#v", diags)
	}
	if countCLIRecords(workspace.Evidence, func(id string) bool { return id == "evidence.interrupted-"+suffix }) != 1 {
		t.Fatal("recovered evidence is not present exactly once")
	}
	count := 0
	for _, record := range workspace.AuditRecords {
		if record.ID == "audit.interrupted-"+suffix {
			count++
		}
	}
	if count != 1 {
		t.Fatal("recovered audit is not present exactly once")
	}
}

func countCLIRecords(records []domain.Evidence, match func(string) bool) int {
	count := 0
	for _, record := range records {
		if match(record.ID) {
			count++
		}
	}
	return count
}
