package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

// A batch recorded as one decision shares one --occurred-at. Every command in
// it must still get its own audit id (GAP-015).
func TestActualBatchWithSharedOccurredAtKeepsAuditIDsUnique(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	service := app.NewActualService(filestore.New(projectPath))
	const occurredAt = "2026-07-06T16:00:00+07:00"

	for _, demandID := range []string{"resource_demand.task-op-1234.implementer", "resource_demand.task-op-1234.review"} {
		result, err := service.SetRemaining(context.Background(), app.ActualSetRemainingRequest{
			DemandID:   demandID,
			Amount:     1,
			Unit:       unitOf(t, projectPath, demandID),
			Reason:     "batch review",
			ActorID:    "actor.human.operator",
			OccurredAt: occurredAt,
		})
		if err != nil || !result.Applied {
			t.Fatalf("SetRemaining %s: err=%v applied=%t errors=%#v", demandID, err, result.Applied, result.Errors)
		}
		if !strings.HasSuffix(result.AuditRecordPreview.ID, "."+strings.ReplaceAll(strings.TrimPrefix(demandID, "resource_demand."), ".", "-")) {
			t.Fatalf("audit id %s does not name demand %s", result.AuditRecordPreview.ID, demandID)
		}
	}
	for _, taskID := range []string{"task.op.1234", "task.offline.repair-pump"} {
		result, err := service.AddStatus(context.Background(), app.ActualAddStatusRequest{
			TaskID:     taskID,
			Status:     "review_ready",
			Reason:     "batch review",
			ActorID:    "actor.human.operator",
			OccurredAt: occurredAt,
		})
		if err != nil || !result.Applied {
			t.Fatalf("AddStatus %s: err=%v applied=%t errors=%#v", taskID, err, result.Applied, result.Errors)
		}
	}
	validation, err := app.NewValidateService(filestore.New(projectPath)).Validate(context.Background(), app.ValidateRequest{})
	if err != nil || !validation.Valid {
		t.Fatalf("project invalid after batch: err=%v errors=%#v", err, validation.Errors)
	}
}

// A command whose result would not validate must leave every file untouched
// and say so (GAP-015).
func TestActualCommandThatFailsValidationWritesNothing(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	const occurredAt = "2026-07-06T16:00:00+07:00"
	const demandID = "resource_demand.task-op-1234.implementer"
	collidingID := "audit.20260706T1600000700.actual-set-remaining.task-op-1234-implementer"

	auditRel := app.AuditShardPath(occurredAt)
	preexisting, _ := json.Marshal(domain.AuditRecord{
		ID:         collidingID,
		OccurredAt: occurredAt,
		ActorRef:   domain.Ref{Kind: "actor", ID: "actor.human.operator"},
		Operation:  "manual_note",
		TargetRef:  domain.Ref{Kind: "resource_demand", ID: demandID},
		Result:     "success",
	})
	appendFile(t, filepath.Join(projectPath, auditRel), append(preexisting, '\n'))
	before := snapshotFiles(t, projectPath)

	result, err := app.NewActualService(filestore.New(projectPath)).SetRemaining(context.Background(), app.ActualSetRemainingRequest{
		DemandID:   demandID,
		Amount:     3,
		Unit:       "hours",
		Reason:     "would collide",
		ActorID:    "actor.human.operator",
		OccurredAt: occurredAt,
	})
	if err == nil {
		t.Fatal("expected a validation error for the duplicate audit id")
	}
	if result.Applied {
		t.Fatal("Applied = true for a command that failed validation")
	}
	if len(result.WrittenFiles) != 0 {
		t.Fatalf("WrittenFiles = %#v, want none", result.WrittenFiles)
	}
	if diff := diffSnapshots(before, snapshotFiles(t, projectPath)); diff != "" {
		t.Fatalf("files changed although nothing was applied: %s", diff)
	}
}

// An explicit zero remaining amount must survive later rewrites of the file
// (GAP-016).
func TestRemainingZeroSurvivesLaterRewrite(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	service := app.NewActualService(filestore.New(projectPath))
	if _, err := service.SetRemaining(context.Background(), app.ActualSetRemainingRequest{
		DemandID: "resource_demand.task-op-1234.implementer", Amount: 0, Unit: "hours",
		Reason: "done", ActorID: "actor.human.operator", OccurredAt: "2026-07-06T16:00:00+07:00",
	}); err != nil {
		t.Fatalf("SetRemaining: %v", err)
	}
	if _, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID: "resource_demand.task-op-1234.review", Status: "cancelled",
		Reason: "not needed", ActorID: "actor.human.operator", OccurredAt: "2026-07-06T16:01:00+07:00",
	}); err != nil {
		t.Fatalf("CloseDemand: %v", err)
	}
	ws, _ := filestore.New(projectPath).Load(context.Background())
	demand := demandByID(t, ws, "resource_demand.task-op-1234.implementer")
	if !demand.HasRemaining() || demand.Remaining() != 0 {
		t.Fatalf("remaining = %v, want explicit 0", demand.RemainingAmount)
	}
	data, err := os.ReadFile(filepath.Join(projectPath, "canonical/resource_demands.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("remaining_amount: 0\n")) {
		t.Fatal("explicit remaining_amount: 0 is not in the rewritten file")
	}
}

func TestActualReopenDemand(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	service := app.NewActualService(filestore.New(projectPath))
	const demandID = "resource_demand.task-op-1234.review"

	notClosed, err := service.ReopenDemand(context.Background(), app.ActualReopenDemandRequest{
		DemandID: demandID, Reason: "reopen", ActorID: "actor.human.operator",
	})
	if err == nil || !hasErrorCode(notClosed.Errors, "demand_not_closed") {
		t.Fatalf("expected demand_not_closed for an active demand, got err=%v %#v", err, notClosed.Errors)
	}

	if _, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID: demandID, Status: "consumed", Reason: "closed too early",
		ActorID: "actor.human.operator", OccurredAt: "2026-07-06T16:00:00+07:00",
	}); err != nil {
		t.Fatalf("CloseDemand: %v", err)
	}
	remaining := 0.5
	result, err := service.ReopenDemand(context.Background(), app.ActualReopenDemandRequest{
		DemandID: demandID, Remaining: &remaining, Reason: "task still open",
		ActorID: "actor.human.operator", OccurredAt: "2026-07-06T16:01:00+07:00",
	})
	if err != nil || !result.Applied {
		t.Fatalf("ReopenDemand: err=%v errors=%#v", err, result.Errors)
	}
	if result.PreviousStatus != "consumed" || result.NewStatus != "active" {
		t.Fatalf("status %s -> %s", result.PreviousStatus, result.NewStatus)
	}
	if result.AuditRecordPreview.ID != "audit.20260706T1601000700.actual-reopen-demand.task-op-1234-review" {
		t.Fatalf("audit id = %s", result.AuditRecordPreview.ID)
	}
	ws, _ := filestore.New(projectPath).Load(context.Background())
	demand := demandByID(t, ws, demandID)
	if demand.Status != "active" || demand.Remaining() != 0.5 {
		t.Fatalf("demand after reopen: status=%s remaining=%v", demand.Status, demand.RemainingAmount)
	}
}

func TestActualAcknowledgeOverrunSilencesTrackingUntilForecastGrows(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)
	const demandID = "resource_demand.repair-pump.operator"

	noOverrun, err := service.AcknowledgeOverrun(context.Background(), app.ActualAcknowledgeOverrunRequest{
		DemandID: demandID, Reason: "check", ActorID: "actor.human.operator",
	})
	if err == nil || !hasErrorCode(noOverrun.Errors, "no_overrun") {
		t.Fatalf("expected no_overrun, got err=%v %#v", err, noOverrun.Errors)
	}

	setRemaining := func(amount float64, at string) {
		t.Helper()
		if _, err := service.SetRemaining(context.Background(), app.ActualSetRemainingRequest{
			DemandID: demandID, Amount: amount, Unit: "hours", Reason: "re-estimate",
			ActorID: "actor.human.operator", OccurredAt: at,
		}); err != nil {
			t.Fatalf("SetRemaining: %v", err)
		}
	}
	setRemaining(5, "2026-07-06T16:00:00+07:00")
	if !hasOverrunSuggestion(t, store, demandID) {
		t.Fatal("expected a review_forecast_over_plan suggestion before acknowledgement")
	}

	ack, err := service.AcknowledgeOverrun(context.Background(), app.ActualAcknowledgeOverrunRequest{
		DemandID: demandID, Reason: "pump needed a second visit", ActorID: "actor.human.operator",
		OccurredAt: "2026-07-06T16:01:00+07:00",
	})
	if err != nil || !ack.Applied {
		t.Fatalf("AcknowledgeOverrun: err=%v errors=%#v", err, ack.Errors)
	}
	if ack.Forecast != 7 || ack.Review == nil || ack.Review.AcknowledgedForecast != 7 {
		t.Fatalf("unexpected acknowledgement: %#v", ack)
	}
	if hasOverrunSuggestion(t, store, demandID) {
		t.Fatal("acknowledged overrun is still suggested")
	}

	again, err := service.AcknowledgeOverrun(context.Background(), app.ActualAcknowledgeOverrunRequest{
		DemandID: demandID, Reason: "again", ActorID: "actor.human.operator",
	})
	if err == nil || !hasErrorCode(again.Errors, "overrun_already_acknowledged") {
		t.Fatalf("expected overrun_already_acknowledged, got err=%v %#v", err, again.Errors)
	}

	setRemaining(6, "2026-07-06T16:02:00+07:00")
	if !hasOverrunSuggestion(t, store, demandID) {
		t.Fatal("a forecast above the acknowledged amount must be suggested again")
	}
}

func TestSuggestDependenciesResolvesSatisfiedPreferredDependency(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	replaceInFile(t, filepath.Join(projectPath, "canonical/dependencies.yaml"),
		"    status: open\n    criticality: required\n    notes: Offline repair",
		"    status: open\n    criticality: preferred\n    notes: Offline repair")
	replaceInFile(t, filepath.Join(projectPath, "canonical/tasks.yaml"),
		"    tracking_status: in_progress", "    tracking_status: accepted")

	result, err := app.NewProposalService(filestore.New(projectPath)).SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{})
	if err != nil {
		t.Fatalf("SuggestDependencies: %v %#v", err, result.Errors)
	}
	for _, proposal := range result.Proposals {
		if proposal.Type == "mark_dependency_resolved" && proposal.TargetRef.ID == "dependency.repair-pump-before-validation" {
			return
		}
	}
	t.Fatalf("no mark_dependency_resolved proposal for the satisfied preferred dependency: %#v", result.Proposals)
}

func hasOverrunSuggestion(t *testing.T, store filestore.Store, demandID string) bool {
	t.Helper()
	result, err := app.NewTrackingService(store).Suggest(context.Background(), app.TrackingSuggestRequest{})
	if err != nil {
		t.Fatalf("Suggest: %v %#v", err, result.Errors)
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.Type == "review_forecast_over_plan" && suggestion.DemandID == demandID {
			return true
		}
	}
	return false
}

func hasErrorCode(diags []app.Diagnostic, code string) bool {
	for _, d := range diags {
		if d.Code == code {
			return true
		}
	}
	return false
}

func demandByID(t *testing.T, ws domain.Workspace, id string) domain.ResourceDemand {
	t.Helper()
	for _, demand := range ws.ResourceDemands {
		if demand.ID == id {
			return demand
		}
	}
	t.Fatalf("resource demand %s not found", id)
	return domain.ResourceDemand{}
}

func unitOf(t *testing.T, projectPath, demandID string) string {
	t.Helper()
	ws, _ := filestore.New(projectPath).Load(context.Background())
	return demandByID(t, ws, demandID).Unit
}

func appendFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		t.Fatal(err)
	}
}

func replaceInFile(t *testing.T, path, old, new string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte(old)) {
		t.Fatalf("%s does not contain %q", path, old)
	}
	if err := os.WriteFile(path, bytes.Replace(data, []byte(old), []byte(new), 1), 0o644); err != nil {
		t.Fatal(err)
	}
}

func snapshotFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func diffSnapshots(before, after map[string]string) string {
	var changes []string
	for rel, data := range after {
		if old, ok := before[rel]; !ok {
			changes = append(changes, "added "+rel)
		} else if old != data {
			changes = append(changes, "changed "+rel)
		}
	}
	for rel := range before {
		if _, ok := after[rel]; !ok {
			changes = append(changes, "removed "+rel)
		}
	}
	return strings.Join(changes, ", ")
}

// A commit that fails part way must say so and list what reached the disk.
func TestActualCommitFailureIsReportedWithWrittenFiles(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	auditDir := filepath.Join(projectPath, "audit")
	if err := os.MkdirAll(auditDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(auditDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(auditDir, 0o755) })
	if f, err := os.CreateTemp(auditDir, "probe"); err == nil {
		f.Close()
		_ = os.Remove(f.Name())
		t.Skip("directory permissions are not enforced for this user")
	}

	result, err := app.NewActualService(filestore.New(projectPath)).SetRemaining(context.Background(), app.ActualSetRemainingRequest{
		DemandID: "resource_demand.task-op-1234.implementer", Amount: 4, Unit: "hours",
		Reason: "commit will fail", ActorID: "actor.human.operator", OccurredAt: "2026-07-06T16:00:00+07:00",
	})
	if err == nil {
		t.Fatal("expected a commit error")
	}
	if result.Applied {
		t.Fatal("Applied = true after a failed commit")
	}
	if !hasErrorCode(result.Errors, "commit_failed") {
		t.Fatalf("commit failure not reported: %#v", result.Errors)
	}
	if len(result.WrittenFiles) != 1 || result.WrittenFiles[0] != "canonical/resource_demands.yaml" {
		t.Fatalf("WrittenFiles = %#v, want the demands file that did reach the disk", result.WrittenFiles)
	}
}

func TestProposalAcceptThatFailsValidationWritesNothing(t *testing.T) {
	projectPath := copyProposalProjectFixture(t)
	service := app.NewProposalService(filestore.New(projectPath))
	req := app.ProposalDecisionRequest{
		ID:      "proposal.adjust-remaining-task-op-1234.20260706",
		ActorID: "actor.human.operator",
		Reason:  "Operator accepted proposal after review.",
		Now:     fixedProposalTestTime(),
	}
	dry := req
	dry.DryRun = true
	preview, err := service.Accept(context.Background(), dry)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	colliding, _ := json.Marshal(preview.AuditRecordPreview)
	appendFile(t, filepath.Join(projectPath, app.AuditShardPath(preview.AuditRecordPreview.OccurredAt)), append(colliding, '\n'))
	before := snapshotFiles(t, projectPath)

	result, err := service.Accept(context.Background(), req)
	if err == nil || result.Applied || len(result.WrittenFiles) != 0 {
		t.Fatalf("expected a validation failure with nothing written: err=%v applied=%t written=%#v", err, result.Applied, result.WrittenFiles)
	}
	if diff := diffSnapshots(before, snapshotFiles(t, projectPath)); diff != "" {
		t.Fatalf("files changed although nothing was applied: %s", diff)
	}
}

func TestDemandLifecycleCommandsRejectBadInputAndDryRunWritesNothing(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	service := app.NewActualService(filestore.New(projectPath))
	ctx := context.Background()

	nan := math.NaN()
	if _, err := service.CloseDemand(ctx, app.ActualCloseDemandRequest{
		DemandID: "resource_demand.task-op-1234.review", Status: "cancelled",
		Reason: "setup", ActorID: "actor.human.operator", OccurredAt: "2026-07-06T16:00:00+07:00",
	}); err != nil {
		t.Fatalf("CloseDemand: %v", err)
	}
	reopen, err := service.ReopenDemand(ctx, app.ActualReopenDemandRequest{
		DemandID: "resource_demand.task-op-1234.review", Remaining: &nan, Reason: "bad", ActorID: "actor.human.operator",
	})
	if err == nil || !hasErrorCode(reopen.Errors, "invalid_remaining") {
		t.Fatalf("NaN remaining accepted: %#v", reopen.Errors)
	}
	setNaN, err := service.SetRemaining(ctx, app.ActualSetRemainingRequest{
		DemandID: "resource_demand.task-op-1234.implementer", Amount: math.Inf(1), Unit: "hours",
		Reason: "bad", ActorID: "actor.human.operator",
	})
	if err == nil || !hasErrorCode(setNaN.Errors, "invalid_amount") {
		t.Fatalf("infinite remaining accepted: %#v", setNaN.Errors)
	}
	ack, err := service.AcknowledgeOverrun(ctx, app.ActualAcknowledgeOverrunRequest{
		DemandID: "resource_demand.task-op-1234.review", Reason: "cancelled", ActorID: "actor.human.operator",
	})
	if err == nil || !hasErrorCode(ack.Errors, "demand_cancelled") {
		t.Fatalf("acknowledging a cancelled demand was not refused: %#v", ack.Errors)
	}

	before := snapshotFiles(t, projectPath)
	one := 1.0
	dry, err := service.ReopenDemand(ctx, app.ActualReopenDemandRequest{
		DemandID: "resource_demand.task-op-1234.review", Remaining: &one, Reason: "preview",
		ActorID: "actor.human.operator", DryRun: true,
	})
	if err != nil || dry.Applied || len(dry.PlannedWrites) != 2 {
		t.Fatalf("reopen dry run: err=%v applied=%t planned=%d", err, dry.Applied, len(dry.PlannedWrites))
	}
	if diff := diffSnapshots(before, snapshotFiles(t, projectPath)); diff != "" {
		t.Fatalf("dry run changed files: %s", diff)
	}
}
