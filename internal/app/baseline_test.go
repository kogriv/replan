package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
	"gopkg.in/yaml.v3"
)

func baselineRequest() app.BaselineCreateRequest {
	return app.BaselineCreateRequest{
		ID: "baseline.2026-07-08.test", Name: "Test baseline", ApprovedBy: "actor.human.operator",
		StatusDate: "2026-07-08", Assumptions: []string{"Fixture estimates are approved."},
		OccurredAt: "2026-07-08T10:00:00+07:00",
	}
}

func TestBaselineCreateDryRunSnapshotsPlanWithoutWriting(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	req := baselineRequest()
	req.DryRun = true
	result, err := app.NewBaselineService(filestore.New(projectPath)).Create(context.Background(), req)
	if err != nil {
		t.Fatalf("Create returned error: %v; diagnostics=%#v", err, result.Errors)
	}
	if result.Applied || result.TaskCount != 2 || result.DemandCount != 7 || len(result.PlannedWrites) != 2 {
		t.Fatalf("unexpected dry-run result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(projectPath, result.FilePath)); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote baseline: %v", err)
	}
	if result.Baseline.Seal == nil || result.Baseline.Seal.Digest == "" {
		t.Fatal("dry-run baseline has no seal")
	}
	for _, demand := range result.Baseline.IncludedDemands {
		if demand.Task == "" || demand.Resource == "" || demand.Unit == "" || demand.PlannedAmount == nil {
			t.Fatalf("demand snapshot is incomplete: %#v", demand)
		}
	}
}

func TestBaselineCreateApplyWritesSealedFileAuditAndValidation(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	result, err := app.NewBaselineService(filestore.New(projectPath)).Create(context.Background(), baselineRequest())
	if err != nil {
		t.Fatalf("Create returned error: %v; diagnostics=%#v", err, result.Errors)
	}
	if !result.Applied || result.Validation == nil || !result.Validation.Valid {
		t.Fatalf("baseline was not applied and validated: %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(projectPath, result.FilePath))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "planned_amount:") || !strings.Contains(string(data), "algorithm: sha256") {
		t.Fatalf("baseline file lacks estimates or seal:\n%s", data)
	}
	audit, err := os.ReadFile(filepath.Join(projectPath, "audit", "audit.2026-07.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(audit), `"operation":"baseline_create"`) || !strings.Contains(string(audit), `"result":"success"`) {
		t.Fatalf("baseline audit missing:\n%s", audit)
	}
}

func TestBaselineCreateRefusesExistingIDAndUnknownActor(t *testing.T) {
	service := app.NewBaselineService(filestore.New(copyAppProjectFixture(t)))
	tests := []struct {
		name string
		edit func(*app.BaselineCreateRequest)
		code string
	}{
		{name: "existing id", edit: func(req *app.BaselineCreateRequest) { req.ID = "baseline.2026-07-06.initial" }, code: "baseline_already_exists"},
		{name: "unknown actor", edit: func(req *app.BaselineCreateRequest) { req.ApprovedBy = "actor.missing" }, code: "actor_not_found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := baselineRequest()
			tc.edit(&req)
			result, err := service.Create(context.Background(), req)
			if err == nil || !hasAppDiagnostic(result.Errors, tc.code) {
				t.Fatalf("error=%v diagnostics=%#v, want %s", err, result.Errors, tc.code)
			}
		})
	}
}

func TestValidateDetectsTamperedSealedBaseline(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	result, err := app.NewBaselineService(filestore.New(projectPath)).Create(context.Background(), baselineRequest())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projectPath, result.FilePath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), "name: Test baseline", "name: Tampered baseline", 1))
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := app.NewValidateService(filestore.New(projectPath)).Validate(context.Background(), app.ValidateRequest{})
	if err == nil || !hasAppDiagnostic(validation.Errors, "baseline_modified") {
		t.Fatalf("error=%v diagnostics=%#v", err, validation.Errors)
	}
}

func TestValidateAcceptsOldUnsealedBaseline(t *testing.T) {
	result, err := app.NewValidateService(filestore.New("../../examples/small-team")).Validate(context.Background(), app.ValidateRequest{})
	if err != nil || hasAppDiagnostic(result.Errors, "baseline_modified") {
		t.Fatalf("old fixture should remain valid: error=%v diagnostics=%#v", err, result.Errors)
	}
}

func TestValidateRejectsDuplicateBaselineIDs(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	data, err := os.ReadFile(filepath.Join(projectPath, "baselines", "baseline.2026-07-06.initial.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "baselines", "duplicate.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := app.NewValidateService(filestore.New(projectPath)).Validate(context.Background(), app.ValidateRequest{})
	if err == nil || !hasAppDiagnostic(validation.Errors, "duplicate_id") {
		t.Fatalf("error=%v diagnostics=%#v", err, validation.Errors)
	}
}

func TestValidateRejectsBadBaselineEstimateFields(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	negative := -1.0
	baseline := domain.Baseline{
		ID: "baseline.bad-estimates", CreatedAt: "2026-07-08T10:00:00+07:00",
		ApprovedBy:      domain.Ref{Kind: "actor", ID: "actor.human.operator"},
		IncludedTasks:   []domain.BaselineTaskEntry{{Ref: domain.Ref{Kind: "task", ID: "task.op.1234"}, PlannedStart: "2026-07-10", PlannedFinish: "2026-07-09"}},
		IncludedDemands: []domain.BaselineDemandEntry{{Ref: domain.Ref{Kind: "resource_demand", ID: "resource_demand.task-op-1234.implementer"}, Task: "task.op.1234", Resource: "resource.human.ivan", PlannedAmount: &negative}},
	}
	data, err := yaml.Marshal(struct {
		Baseline domain.Baseline `yaml:"baseline"`
	}{Baseline: baseline})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "baselines", baseline.ID+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	validation, err := app.NewValidateService(filestore.New(projectPath)).Validate(context.Background(), app.ValidateRequest{})
	if err == nil || !hasAppDiagnostic(validation.Errors, "invalid_baseline_date_range") || !hasAppDiagnostic(validation.Errors, "invalid_baseline_amount") || !hasAppDiagnostic(validation.Errors, "missing_baseline_unit") {
		t.Fatalf("error=%v diagnostics=%#v", err, validation.Errors)
	}
}

func TestConvertedSelfDogfoodBaselineSealVerifies(t *testing.T) {
	result, err := app.NewBaselineService(filestore.New("../../examples/replan-self")).Show(context.Background(), "baseline.2026-10-15.tooling-consolidation")
	if err != nil {
		t.Fatalf("Show returned error: %v; diagnostics=%#v", err, result.Errors)
	}
	if result.SealStatus != "verified" || !result.CarriesEstimates || len(result.Baseline.IncludedTasks) != 38 || len(result.Baseline.IncludedDemands) != 152 {
		t.Fatalf("unexpected converted baseline: %#v", result)
	}
}

func hasAppDiagnostic(diagnostics []app.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}
