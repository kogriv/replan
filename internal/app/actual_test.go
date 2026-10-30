package app_test

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestEffectiveActualsLatestCorrectionSupersedesAndVoidDrops(t *testing.T) {
	down, up, zero := 1.0, 1.5, 0.0
	original := domain.Actual{ID: "actual.original", Type: "work_consumed", Amount: 2, Unit: "hours"}
	actuals := []domain.Actual{
		original,
		{ID: "correction.1", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: original.ID}, CorrectedAmount: &down, OccurredAt: "2026-10-30T10:00:00Z"},
		{ID: "correction.2", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: original.ID}, CorrectedAmount: &up, OccurredAt: "2026-10-30T11:00:00Z"},
	}
	effective := app.EffectiveActuals(actuals)
	if len(effective) != 1 || effective[0].Amount != up || effective[0].CorrectedAmount == nil || *effective[0].CorrectedAmount != up {
		t.Fatalf("effective actuals = %#v", effective)
	}
	actuals = append(actuals, domain.Actual{ID: "correction.3", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: original.ID}, CorrectedAmount: &zero, OccurredAt: "2026-10-30T12:00:00Z"})
	if got := app.EffectiveActuals(actuals); len(got) != 0 {
		t.Fatalf("voided effective actuals = %#v", got)
	}
}

func TestActualCorrectApplySupersedeVoidListAndAudit(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)
	originalID := "actual.repair-pump.petr.20260706.1"
	for index, amount := range []float64{0.1, 0.25, 0} {
		result, err := service.Correct(context.Background(), app.ActualCorrectRequest{ActualID: originalID, CorrectedAmount: &amount, Reason: "Correct guessed duration.", EvidenceID: "evidence.github.pr.45", ActorID: "actor.human.operator", OccurredAt: []string{"2026-10-30T10:00:00+07:00", "2026-10-30T11:00:00+07:00", "2026-10-30T12:00:00+07:00"}[index]})
		if err != nil || !result.Applied {
			t.Fatalf("correction %d: result=%#v err=%v", index, result, err)
		}
		if result.AuditRecordPreview.Operation != "actual_correct" {
			t.Fatalf("audit operation = %q", result.AuditRecordPreview.Operation)
		}
	}
	list, err := service.List(context.Background(), app.ActualListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Actuals) != 4 {
		t.Fatalf("listed actuals = %#v", list.Actuals)
	}
	if list.Actuals[0].ID != originalID || list.Actuals[0].Amount != 2 || list.Actuals[0].CorrectedTo == nil || *list.Actuals[0].CorrectedTo != 0 {
		t.Fatalf("listed original = %#v", list.Actuals[0])
	}
	verify, err := app.NewAuditService(store).Verify(context.Background(), app.AuditVerifyRequest{})
	if err != nil || !verify.Valid {
		t.Fatalf("audit verify = %#v, %v", verify, err)
	}
}

func TestActualCorrectRefusals(t *testing.T) {
	amount, same, negative, nan := 1.0, 2.0, -1.0, math.NaN()
	base := domain.Workspace{
		Actors:   []domain.Actor{{ID: "actor.test"}},
		Evidence: []domain.Evidence{{ID: "evidence.test", Type: "operator_note"}},
		Actuals: []domain.Actual{
			{ID: "actual.work", Type: "work_consumed", Amount: 2, Unit: "hours", AcceptedBy: domain.Ref{ID: "actor.test"}},
			{ID: "actual.status", Type: "status", AcceptedBy: domain.Ref{ID: "actor.test"}},
		},
	}
	tests := []struct {
		name, id, code string
		value          *float64
	}{
		{"unknown", "actual.missing", "actual_not_found", &amount},
		{"status", "actual.status", "actual_not_correctable", &amount},
		{"no change", "actual.work", "actual_correction_no_change", &same},
		{"negative", "actual.work", "invalid_amount", &negative},
		{"non finite", "actual.work", "invalid_amount", &nan},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := app.NewActualService(stubLoader{ws: base}).Correct(context.Background(), app.ActualCorrectRequest{ActualID: tc.id, CorrectedAmount: tc.value, Reason: "reason", EvidenceID: "evidence.test", ActorID: "actor.test", DryRun: true})
			if err == nil || !hasAppDiagnosticCode(result.Errors, tc.code) {
				t.Fatalf("errors = %#v, err=%v", result.Errors, err)
			}
		})
	}
}

func TestValidateRejectsDanglingAndMismatchedCorrection(t *testing.T) {
	amount := 1.0
	ws := domain.Workspace{Actors: []domain.Actor{{ID: "actor.test"}}, Actuals: []domain.Actual{{ID: "actual.correction", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: "actual.missing"}, CorrectedAmount: &amount, DemandRef: domain.Ref{ID: "demand.wrong"}, Unit: "hours", AcceptedBy: domain.Ref{ID: "actor.test"}}}}
	result, err := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
	if err == nil || !hasAppDiagnosticCode(result.Errors, "unresolved_reference") {
		t.Fatalf("validation errors = %#v, err=%v", result.Errors, err)
	}
}

func hasAppDiagnosticCode(diags []app.Diagnostic, code string) bool {
	for _, diag := range diags {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func TestActualListSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewActualService(store)

	result, err := service.List(context.Background(), app.ActualListRequest{})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Actuals) != 1 {
		t.Fatalf("expected 1 actual, got %d", len(result.Actuals))
	}
	actual := result.Actuals[0]
	if actual.ID != "actual.repair-pump.petr.20260706.1" {
		t.Fatalf("unexpected actual id: %s", actual.ID)
	}
	if actual.Type != "work_consumed" {
		t.Fatalf("unexpected actual type: %s", actual.Type)
	}
	if actual.Amount != 2 {
		t.Fatalf("unexpected actual amount: %v", actual.Amount)
	}
}

func TestActualListFiltersSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewActualService(store)

	result, err := service.List(context.Background(), app.ActualListRequest{
		ResourceID: "resource.human.petr",
		Type:       "work_consumed",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(result.Actuals) != 1 {
		t.Fatalf("expected 1 filtered actual, got %d", len(result.Actuals))
	}

	empty, err := service.List(context.Background(), app.ActualListRequest{
		ResourceID: "resource.human.ivan",
	})
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(empty.Actuals) != 0 {
		t.Fatalf("expected no filtered actuals, got %d", len(empty.Actuals))
	}
}

func TestActualAddConsumptionDryRunSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewActualService(store)

	result, err := service.AddConsumption(context.Background(), app.ActualAddConsumptionRequest{
		TaskID:     "task.op.1234",
		DemandID:   "resource_demand.task-op-1234.implementer",
		ResourceID: "resource.human.ivan",
		Amount:     3,
		Unit:       "hours",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:00:00+07:00",
		StatusDate: "2026-07-06",
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("AddConsumption dry run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry run result")
	}
	if result.Actual.ID != "actual.op-1234.human-ivan.20260706T1600000700.work-consumed" {
		t.Fatalf("unexpected actual id: %s", result.Actual.ID)
	}
	if len(result.PlannedWrites) != 1 {
		t.Fatalf("expected 1 planned write, got %d", len(result.PlannedWrites))
	}
	if result.PlannedWrites[0].Path != "actuals/actuals.2026-W28.yaml" {
		t.Fatalf("unexpected planned write path: %s", result.PlannedWrites[0].Path)
	}
	if result.AuditRecordPreview.ActorRef.ID != "actor.human.operator" {
		t.Fatalf("unexpected audit actor: %s", result.AuditRecordPreview.ActorRef.ID)
	}
}

func TestActualAddConsumptionWriteSmallTeamFixture(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)

	result, err := service.AddConsumption(context.Background(), app.ActualAddConsumptionRequest{
		TaskID:     "task.op.1234",
		DemandID:   "resource_demand.task-op-1234.implementer",
		ResourceID: "resource.human.ivan",
		Amount:     3,
		Unit:       "hours",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:00:00+07:00",
		StatusDate: "2026-07-06",
	})
	if err != nil {
		t.Fatalf("AddConsumption returned error: %v", err)
	}
	if result.DryRun {
		t.Fatal("did not expect dry run result")
	}
	if !result.Applied {
		t.Fatalf("expected applied result, errors: %#v", result.Errors)
	}
	if len(result.WrittenFiles) != 2 {
		t.Fatalf("expected actual and audit written files, got %#v", result.WrittenFiles)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected valid post-write validation, got %#v", result.Validation)
	}

	list, err := service.List(context.Background(), app.ActualListRequest{ResourceID: "resource.human.ivan"})
	if err != nil {
		t.Fatalf("List returned error after write: %v", err)
	}
	if len(list.Actuals) != 1 {
		t.Fatalf("expected written actual to be listed, got %d", len(list.Actuals))
	}
	if list.Actuals[0].ID != result.Actual.ID {
		t.Fatalf("unexpected written actual id: %s", list.Actuals[0].ID)
	}

	duplicate, err := service.AddConsumption(context.Background(), app.ActualAddConsumptionRequest{
		TaskID:     "task.op.1234",
		DemandID:   "resource_demand.task-op-1234.implementer",
		ResourceID: "resource.human.ivan",
		Amount:     3,
		Unit:       "hours",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:00:00+07:00",
		StatusDate: "2026-07-06",
	})
	if err == nil {
		t.Fatal("expected duplicate actual error")
	}
	if len(duplicate.Errors) == 0 || duplicate.Errors[0].Code != "actual_already_exists" {
		t.Fatalf("expected actual_already_exists error, got %#v", duplicate.Errors)
	}
}

func TestActualSetRemainingDryRunSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewActualService(store)

	result, err := service.SetRemaining(context.Background(), app.ActualSetRemainingRequest{
		DemandID:   "resource_demand.task-op-1234.implementer",
		Amount:     5,
		Unit:       "hours",
		Reason:     "Operator review after pull request",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:30:00+07:00",
		StatusDate: "2026-07-06",
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("SetRemaining dry run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry run result")
	}
	if result.PreviousAmount != 8 {
		t.Fatalf("unexpected previous amount: %v", result.PreviousAmount)
	}
	if result.NewAmount != 5 {
		t.Fatalf("unexpected new amount: %v", result.NewAmount)
	}
	if len(result.PlannedWrites) != 1 {
		t.Fatalf("expected 1 planned write, got %d", len(result.PlannedWrites))
	}
	if result.PlannedWrites[0].Path != "canonical/resource_demands.yaml" {
		t.Fatalf("unexpected planned write path: %s", result.PlannedWrites[0].Path)
	}
	if result.AuditRecordPreview.Operation != "actual_set_remaining" {
		t.Fatalf("unexpected audit operation: %s", result.AuditRecordPreview.Operation)
	}
}

func TestActualSetRemainingWriteSmallTeamFixture(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)

	result, err := service.SetRemaining(context.Background(), app.ActualSetRemainingRequest{
		DemandID:   "resource_demand.task-op-1234.implementer",
		Amount:     5,
		Unit:       "hours",
		Reason:     "Operator review after pull request",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:30:00+07:00",
		StatusDate: "2026-07-06",
	})
	if err != nil {
		t.Fatalf("SetRemaining returned error: %v", err)
	}
	if result.DryRun {
		t.Fatal("did not expect dry run result")
	}
	if !result.Applied {
		t.Fatalf("expected applied result, errors: %#v", result.Errors)
	}
	if len(result.WrittenFiles) != 2 {
		t.Fatalf("expected demand and audit written files, got %#v", result.WrittenFiles)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected valid post-write validation, got %#v", result.Validation)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics after write: %#v", diags)
	}
	for _, demand := range ws.ResourceDemands {
		if demand.ID == "resource_demand.task-op-1234.implementer" {
			if demand.Remaining() != 5 {
				t.Fatalf("remaining amount was not updated, got %v", demand.Remaining())
			}
			break
		}
	}
	foundAudit := false
	for _, record := range ws.AuditRecords {
		if record.Operation == "actual_set_remaining" && record.TargetRef.ID == "resource_demand.task-op-1234.implementer" {
			foundAudit = true
			break
		}
	}
	if !foundAudit {
		t.Fatal("expected actual_set_remaining audit record")
	}
}

func TestActualCloseDemandDryRunSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewActualService(store)

	result, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID:   "resource_demand.task-op-1234.implementer",
		Status:     "consumed",
		Reason:     "Operator reviewed demand lifecycle",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:45:00+07:00",
		StatusDate: "2026-07-06",
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("CloseDemand dry run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry run result")
	}
	if result.PreviousStatus != "active" {
		t.Fatalf("unexpected previous status: %s", result.PreviousStatus)
	}
	if result.NewStatus != "consumed" {
		t.Fatalf("unexpected new status: %s", result.NewStatus)
	}
	if len(result.PlannedWrites) != 1 {
		t.Fatalf("expected 1 planned write, got %d", len(result.PlannedWrites))
	}
	if result.PlannedWrites[0].Path != "canonical/resource_demands.yaml" {
		t.Fatalf("unexpected planned write path: %s", result.PlannedWrites[0].Path)
	}
	if result.AuditRecordPreview.Operation != "actual_close_demand" {
		t.Fatalf("unexpected audit operation: %s", result.AuditRecordPreview.Operation)
	}
}

func TestActualCloseDemandWriteSmallTeamFixture(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)

	result, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID:   "resource_demand.task-op-1234.implementer",
		Status:     "consumed",
		Reason:     "Operator reviewed demand lifecycle",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:45:00+07:00",
		StatusDate: "2026-07-06",
	})
	if err != nil {
		t.Fatalf("CloseDemand returned error: %v", err)
	}
	if result.DryRun {
		t.Fatal("did not expect dry run result")
	}
	if !result.Applied {
		t.Fatalf("expected applied result, errors: %#v", result.Errors)
	}
	if len(result.WrittenFiles) != 2 {
		t.Fatalf("expected demand and audit written files, got %#v", result.WrittenFiles)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected valid post-write validation, got %#v", result.Validation)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics after write: %#v", diags)
	}
	foundDemand := false
	for _, demand := range ws.ResourceDemands {
		if demand.ID == "resource_demand.task-op-1234.implementer" {
			foundDemand = true
			if demand.Status != "consumed" {
				t.Fatalf("status was not updated, got %q", demand.Status)
			}
			break
		}
	}
	if !foundDemand {
		t.Fatal("expected demand in workspace")
	}
	foundAudit := false
	for _, record := range ws.AuditRecords {
		if record.Operation == "actual_close_demand" && record.TargetRef.ID == "resource_demand.task-op-1234.implementer" {
			foundAudit = true
			break
		}
	}
	if !foundAudit {
		t.Fatal("expected actual_close_demand audit record")
	}
}

func TestActualCloseDemandAuditIDsIncludeDemand(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)

	first, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID:   "resource_demand.task-op-1234.implementer",
		Status:     "consumed",
		Reason:     "Operator reviewed demand lifecycle",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:45:00+07:00",
		StatusDate: "2026-07-06",
	})
	if err != nil {
		t.Fatalf("first CloseDemand returned error: %v", err)
	}
	second, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID:   "resource_demand.task-op-1234.review",
		Status:     "cancelled",
		Reason:     "Operator reviewed demand lifecycle",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:45:00+07:00",
		StatusDate: "2026-07-06",
	})
	if err != nil {
		t.Fatalf("second CloseDemand returned error: %v", err)
	}
	if first.AuditRecordPreview.ID == second.AuditRecordPreview.ID {
		t.Fatalf("audit ids collided: %s", first.AuditRecordPreview.ID)
	}
}

func TestActualAddStatusDryRunSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewActualService(store)

	result, err := service.AddStatus(context.Background(), app.ActualAddStatusRequest{
		TaskID:     "task.op.1234",
		Status:     "review_ready",
		Reason:     "Pull request is open and ready for review",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T17:00:00+07:00",
		StatusDate: "2026-07-06",
		DryRun:     true,
	})
	if err != nil {
		t.Fatalf("AddStatus dry run returned error: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected dry run result")
	}
	if result.PreviousStatus != "ready" {
		t.Fatalf("unexpected previous status: %s", result.PreviousStatus)
	}
	if result.NewStatus != "review_ready" {
		t.Fatalf("unexpected new status: %s", result.NewStatus)
	}
	if len(result.PlannedWrites) != 1 {
		t.Fatalf("expected 1 planned write, got %d", len(result.PlannedWrites))
	}
	if result.PlannedWrites[0].Path != "canonical/tasks.yaml" {
		t.Fatalf("unexpected planned write path: %s", result.PlannedWrites[0].Path)
	}
	if result.AuditRecordPreview.Operation != "actual_add_status" {
		t.Fatalf("unexpected audit operation: %s", result.AuditRecordPreview.Operation)
	}
	if result.AuditRecordPreview.NewStatus != "review_ready" || result.AuditRecordPreview.StatusDate != "2026-07-06" {
		t.Fatalf("audit status facts were not retained: %#v", result.AuditRecordPreview)
	}
}

func TestActualAddStatusRejectsInvalidStatus(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewActualService(store)

	result, err := service.AddStatus(context.Background(), app.ActualAddStatusRequest{
		TaskID:  "task.op.1234",
		Status:  "waiting_for_magic",
		ActorID: "actor.human.operator",
		DryRun:  true,
	})
	if err == nil {
		t.Fatal("expected invalid tracking status error")
	}
	if len(result.Errors) == 0 || result.Errors[0].Code != "invalid_tracking_status" {
		t.Fatalf("expected invalid_tracking_status error, got %#v", result.Errors)
	}
}

func TestActualAddStatusWriteSmallTeamFixture(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)

	result, err := service.AddStatus(context.Background(), app.ActualAddStatusRequest{
		TaskID:     "task.op.1234",
		Status:     "review_ready",
		Reason:     "Pull request is open and ready for review",
		EvidenceID: "evidence.github.pr.45",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T17:00:00+07:00",
		StatusDate: "2026-07-06",
	})
	if err != nil {
		t.Fatalf("AddStatus returned error: %v", err)
	}
	if result.DryRun {
		t.Fatal("did not expect dry run result")
	}
	if !result.Applied {
		t.Fatalf("expected applied result, errors: %#v", result.Errors)
	}
	if len(result.WrittenFiles) != 2 {
		t.Fatalf("expected task and audit written files, got %#v", result.WrittenFiles)
	}
	if result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("expected valid post-write validation, got %#v", result.Validation)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics after write: %#v", diags)
	}
	for _, task := range ws.Tasks {
		if task.ID == "task.op.1234" {
			if task.TrackingStatus != "review_ready" {
				t.Fatalf("tracking status was not updated, got %s", task.TrackingStatus)
			}
			break
		}
	}
	foundAudit := false
	for _, record := range ws.AuditRecords {
		if record.Operation == "actual_add_status" && record.TargetRef.ID == "task.op.1234" {
			if record.NewStatus != "review_ready" || record.StatusDate != "2026-07-06" {
				t.Fatalf("persisted audit status facts are incomplete: %#v", record)
			}
			foundAudit = true
			break
		}
	}
	if !foundAudit {
		t.Fatal("expected actual_add_status audit record")
	}
}

func copyAppProjectFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Clean("../../examples/small-team")
	dst := filepath.Join(t.TempDir(), "small-team")
	if err := copyAppDir(src, dst); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func copyAppDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

// GAP-029: closing as consumed must not leave a stale remaining amount;
// closing as cancelled keeps it as the work that was not done.
func TestActualCloseDemandConsumedClearsRemaining(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	store := filestore.New(projectPath)
	service := app.NewActualService(store)

	result, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID:   "resource_demand.task-op-1234.implementer",
		Status:     "consumed",
		Reason:     "Work done",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:45:00+07:00",
	})
	if err != nil {
		t.Fatalf("CloseDemand consumed returned error: %v", err)
	}
	if !result.RemainingCleared || result.PreviousRemaining == nil || *result.PreviousRemaining != 8 {
		t.Fatalf("expected remaining 8 cleared, got cleared=%v previous=%v", result.RemainingCleared, result.PreviousRemaining)
	}
	if _, err := service.CloseDemand(context.Background(), app.ActualCloseDemandRequest{
		DemandID:   "resource_demand.task-op-1234.review",
		Status:     "cancelled",
		Reason:     "Review dropped",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-06T16:46:00+07:00",
	}); err != nil {
		t.Fatalf("CloseDemand cancelled returned error: %v", err)
	}

	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics after write: %#v", diags)
	}
	for _, demand := range ws.ResourceDemands {
		switch demand.ID {
		case "resource_demand.task-op-1234.implementer":
			if demand.Status != "consumed" || !demand.HasRemaining() || demand.Remaining() != 0 {
				t.Fatalf("consumed demand: status %q, remaining set %v, remaining %v; want consumed with explicit 0",
					demand.Status, demand.HasRemaining(), demand.Remaining())
			}
		case "resource_demand.task-op-1234.review":
			if demand.Status != "cancelled" || demand.Remaining() != 2 {
				t.Fatalf("cancelled demand: status %q, remaining %v; want cancelled with 2 kept", demand.Status, demand.Remaining())
			}
		}
	}
	closeAudits := 0
	for _, record := range ws.AuditRecords {
		if record.Operation == "actual_close_demand" && record.TargetRef.ID == "resource_demand.task-op-1234.implementer" {
			closeAudits++
		}
	}
	if closeAudits != 1 {
		t.Fatalf("expected one audit record for the consumed close, got %d", closeAudits)
	}
}
