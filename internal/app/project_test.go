package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestProjectSetCalibrationLifecycle(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	manifestPath := filepath.Join(projectPath, ".replan", "project.yaml")
	before, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	service := app.NewProjectService(filestore.New(projectPath))
	ctx := context.Background()

	dry, err := service.SetCalibration(ctx, app.ProjectSetCalibrationRequest{
		RequireMeasurement: []string{"resource.service.openai.tokens", "resource.ai.codex.local", "resource.service.openai.tokens"},
		ActorID:            "actor.human.operator", EvidenceID: "evidence.github.pr.45", Reason: "Require trustworthy calibration actuals.",
		OccurredAt: "2026-11-16T10:00:00+07:00", DryRun: true,
	})
	if err != nil {
		t.Fatalf("SetCalibration dry run: %v %#v", err, dry.Errors)
	}
	if !dry.Changed || dry.Applied || len(dry.WrittenFiles) != 0 || len(dry.PlannedWrites) != 2 {
		t.Fatalf("unexpected dry run: %#v", dry)
	}
	if got := dry.NewRequireMeasurement; len(got) != 2 || got[0] != "resource.ai.codex.local" || got[1] != "resource.service.openai.tokens" {
		t.Fatalf("sorted/deduplicated list = %#v", got)
	}
	if after, err := os.ReadFile(manifestPath); err != nil || string(after) != string(before) {
		t.Fatalf("dry run changed project.yaml: %v\n%s", err, after)
	}

	dry.DryRun = false
	result, err := service.SetCalibration(ctx, app.ProjectSetCalibrationRequest{
		RequireMeasurement: []string{"resource.service.openai.tokens", "resource.ai.codex.local", "resource.service.openai.tokens"},
		ActorID:            "actor.human.operator", EvidenceID: "evidence.github.pr.45", Reason: "Require trustworthy calibration actuals.",
		OccurredAt: "2026-11-16T10:00:00+07:00",
	})
	if err != nil {
		t.Fatalf("SetCalibration apply: %v %#v", err, result.Errors)
	}
	if !result.Applied || !result.Changed || len(result.WrittenFiles) != 2 {
		t.Fatalf("unexpected apply: %#v", result)
	}
	afterSet, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	wantSet := strings.Replace(string(before), "  policy_ref:\n", "  calibration:\n    require_measurement:\n      - resource.ai.codex.local\n      - resource.service.openai.tokens\n  policy_ref:\n", 1)
	if string(afterSet) != wantSet {
		t.Fatalf("project.yaml changed beyond inserted calibration field:\n--- want\n%s\n--- got\n%s", wantSet, afterSet)
	}

	replaced, err := service.SetCalibration(ctx, app.ProjectSetCalibrationRequest{RequireMeasurement: []string{"resource.human.ivan"}, ActorID: "actor.human.operator", OccurredAt: "2026-11-16T11:00:00+07:00"})
	if err != nil || !replaced.Applied || len(replaced.PreviousRequireMeasurement) != 2 || len(replaced.NewRequireMeasurement) != 1 {
		t.Fatalf("replace: err=%v result=%#v", err, replaced)
	}

	unchangedBefore, _ := os.ReadFile(manifestPath)
	unchanged, err := service.SetCalibration(ctx, app.ProjectSetCalibrationRequest{RequireMeasurement: []string{"resource.human.ivan", "resource.human.ivan"}, ActorID: "actor.human.operator"})
	if err != nil || unchanged.Changed || unchanged.Applied || len(unchanged.PlannedWrites) != 0 || unchanged.AuditRecordPreview != nil {
		t.Fatalf("unchanged: err=%v result=%#v", err, unchanged)
	}
	unchangedAfter, _ := os.ReadFile(manifestPath)
	if string(unchangedAfter) != string(unchangedBefore) {
		t.Fatal("unchanged apply wrote project.yaml")
	}

	cleared, err := service.SetCalibration(ctx, app.ProjectSetCalibrationRequest{ClearRequireMeasurement: true, ActorID: "actor.human.operator", OccurredAt: "2026-11-16T12:00:00+07:00"})
	if err != nil || !cleared.Applied || len(cleared.NewRequireMeasurement) != 0 {
		t.Fatalf("clear: err=%v result=%#v", err, cleared)
	}
	afterClear, _ := os.ReadFile(manifestPath)
	if string(afterClear) != string(before) {
		t.Fatalf("clear did not restore original project bytes:\n--- original\n%s\n--- got\n%s", before, afterClear)
	}

	ws, diags := filestore.New(projectPath).Load(ctx)
	if len(diags) != 0 {
		t.Fatalf("load diagnostics: %#v", diags)
	}
	var matching []json.RawMessage
	for _, record := range ws.AuditRecords {
		if record.Operation == "project_set_calibration" {
			encoded, _ := json.Marshal(record)
			matching = append(matching, encoded)
			if record.Before == nil || record.After == nil {
				t.Fatalf("audit lacks before/after values: %s", encoded)
			}
		}
	}
	if len(matching) != 3 {
		t.Fatalf("project_set_calibration audit count = %d, want 3", len(matching))
	}
}

func TestProjectSetCalibrationRefusals(t *testing.T) {
	service := app.NewProjectService(filestore.New(copyAppProjectFixture(t)))
	for _, tc := range []struct {
		name string
		req  app.ProjectSetCalibrationRequest
		code string
	}{
		{"missing setting", app.ProjectSetCalibrationRequest{ActorID: "actor.human.operator"}, "missing_setting"},
		{"both flags", app.ProjectSetCalibrationRequest{RequireMeasurement: []string{"resource.ai.codex.local"}, ClearRequireMeasurement: true, ActorID: "actor.human.operator"}, "conflicting_settings"},
		{"unknown resource", app.ProjectSetCalibrationRequest{RequireMeasurement: []string{"resource.missing"}, ActorID: "actor.human.operator"}, "resource_not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := service.SetCalibration(context.Background(), tc.req)
			if err == nil || !hasProjectDiagnostic(result.Errors, tc.code) {
				t.Fatalf("want %s, got err=%v errors=%#v", tc.code, err, result.Errors)
			}
		})
	}
}

func hasProjectDiagnostic(diagnostics []app.Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

func TestProjectSetStatusDateChangesOnlyTheDate(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	before, err := os.ReadFile(filepath.Join(projectPath, ".replan", "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	store := filestore.New(projectPath)
	result, err := app.NewProjectService(store).SetStatusDate(context.Background(), app.ProjectSetStatusDateRequest{
		Date:       "2026-07-13",
		Reason:     "Weekly status date",
		ActorID:    "actor.human.operator",
		OccurredAt: "2026-07-13T09:00:00+07:00",
	})
	if err != nil {
		t.Fatalf("SetStatusDate returned error: %v %#v", err, result.Errors)
	}
	if !result.Applied || result.PreviousDate != "2026-07-06" || result.NewDate != "2026-07-13" || result.Week != "2026-W29" {
		t.Fatalf("unexpected result: %#v", result)
	}
	after, err := os.ReadFile(filepath.Join(projectPath, ".replan", "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(string(before), `"2026-07-06"`, `"2026-07-13"`, 1); string(after) != want {
		t.Fatalf("project.yaml changed beyond the status date:\n--- before\n%s\n--- after\n%s", before, after)
	}
	ws, diags := store.Load(context.Background())
	if len(diags) != 0 {
		t.Fatalf("unexpected load diagnostics: %#v", diags)
	}
	if ws.Project.DefaultStatusDate != "2026-07-13" {
		t.Fatalf("status date = %q", ws.Project.DefaultStatusDate)
	}
	audits := 0
	for _, record := range ws.AuditRecords {
		if record.Operation == "project_set_status_date" && record.TargetRef.ID == ws.Project.ID {
			audits++
		}
	}
	if audits != 1 {
		t.Fatalf("expected one project_set_status_date audit record, got %d", audits)
	}
}

func TestProjectSetStatusDateRules(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	service := app.NewProjectService(filestore.New(projectPath))
	ctx := context.Background()

	// "today" uses the project time zone: 2026-07-19 20:00 UTC is already
	// 2026-07-20 in Asia/Novosibirsk (UTC+7).
	now := time.Date(2026, 7, 19, 20, 0, 0, 0, time.UTC)
	result, err := service.SetStatusDate(ctx, app.ProjectSetStatusDateRequest{Date: "today", ActorID: "actor.human.operator", DryRun: true, Now: now})
	if err != nil {
		t.Fatalf("dry run today returned error: %v %#v", err, result.Errors)
	}
	if result.NewDate != "2026-07-20" {
		t.Fatalf("today resolved to %q, want 2026-07-20 in the project time zone", result.NewDate)
	}
	if result.Applied || len(result.WrittenFiles) != 0 {
		t.Fatalf("dry run wrote files: %#v", result)
	}

	for _, tc := range []struct{ date, actor, code string }{
		{"2026-07-06", "actor.human.operator", "status_date_unchanged"},
		{"2026-02-30", "actor.human.operator", "invalid_status_date"},
		{"", "actor.human.operator", "invalid_status_date"},
		{"2026-07-13", "", "missing_actor"},
		{"2026-07-13", "actor.nobody", "actor_not_found"},
	} {
		result, err := service.SetStatusDate(ctx, app.ProjectSetStatusDateRequest{Date: tc.date, ActorID: tc.actor})
		if err == nil || len(result.Errors) == 0 || result.Errors[0].Code != tc.code {
			t.Fatalf("date %q actor %q: want %s, got err=%v errors=%#v", tc.date, tc.actor, tc.code, err, result.Errors)
		}
	}

	result, err = service.SetStatusDate(ctx, app.ProjectSetStatusDateRequest{Date: "2026-07-01", ActorID: "actor.human.operator", OccurredAt: "2026-07-06T10:00:00+07:00"})
	if err != nil {
		t.Fatalf("moving back returned error: %v", err)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "status_date_moved_backwards" {
		t.Fatalf("expected status_date_moved_backwards warning, got %#v", result.Warnings)
	}
}

func TestProjectMigrateDryRunAndApply(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	manifestPath := filepath.Join(projectPath, ".replan", "project.yaml")
	beforeWithVersion, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	before := []byte(strings.Replace(string(beforeWithVersion), "  format_version: 1\n", "", 1))
	if err := os.WriteFile(manifestPath, before, 0o644); err != nil {
		t.Fatal(err)
	}
	store := filestore.New(projectPath)
	service := app.NewProjectService(store)
	req := app.ProjectMigrateRequest{ActorID: "actor.human.operator", Reason: "Declare the public format.", OccurredAt: "2026-10-28T12:00:00+07:00", DryRun: true}

	dryRun, err := service.Migrate(context.Background(), req)
	if err != nil {
		t.Fatalf("Migrate dry run returned error: %v %#v", err, dryRun.Errors)
	}
	if dryRun.FromVersion != 1 || dryRun.ToVersion != 1 || len(dryRun.Steps) != 1 || dryRun.Steps[0].Name != "declare_format_version" {
		t.Fatalf("unexpected dry-run result: %#v", dryRun)
	}
	if dryRun.Applied || len(dryRun.WrittenFiles) != 0 || len(dryRun.PlannedWrites) != 2 {
		t.Fatalf("dry run write state: %#v", dryRun)
	}
	if afterDryRun, err := os.ReadFile(manifestPath); err != nil || string(afterDryRun) != string(before) {
		t.Fatalf("dry run changed project.yaml: err=%v\n%s", err, afterDryRun)
	}

	req.DryRun = false
	result, err := service.Migrate(context.Background(), req)
	if err != nil {
		t.Fatalf("Migrate apply returned error: %v %#v", err, result.Errors)
	}
	if !result.Applied || len(result.WrittenFiles) != 2 {
		t.Fatalf("unexpected apply result: %#v", result)
	}
	after, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(before), "  name: RePlan MVP example\n", "  name: RePlan MVP example\n  format_version: 1\n", 1)
	if string(after) != want {
		t.Fatalf("project.yaml changed beyond the added line:\n--- want\n%s\n--- got\n%s", want, after)
	}
	workspace, diagnostics := store.Load(context.Background())
	if len(diagnostics) != 0 {
		t.Fatalf("load diagnostics: %#v", diagnostics)
	}
	audits := 0
	for _, record := range workspace.AuditRecords {
		if record.Operation == "project_migrate" && record.TargetRef.ID == workspace.Project.ID {
			audits++
		}
	}
	if audits != 1 {
		t.Fatalf("project_migrate audit records = %d, want 1", audits)
	}
}

func TestProjectMigrateAlreadyCurrentDoesNothing(t *testing.T) {
	projectPath := copyAppProjectFixture(t)
	before, err := os.ReadFile(filepath.Join(projectPath, ".replan", "project.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	result, err := app.NewProjectService(filestore.New(projectPath)).Migrate(context.Background(), app.ProjectMigrateRequest{
		ActorID: "actor.human.operator", OccurredAt: "2026-10-28T12:00:00+07:00",
	})
	if err != nil {
		t.Fatalf("Migrate returned error: %v %#v", err, result.Errors)
	}
	if result.Applied || len(result.Steps) != 0 || len(result.PlannedWrites) != 0 || len(result.WrittenFiles) != 0 || result.AuditRecordPreview != nil {
		t.Fatalf("current project was not a no-op: %#v", result)
	}
	after, err := os.ReadFile(filepath.Join(projectPath, ".replan", "project.yaml"))
	if err != nil || string(after) != string(before) {
		t.Fatalf("current project changed: err=%v", err)
	}
}

func TestPlanProjectFormatMigrationCurrentTable(t *testing.T) {
	steps, err := app.PlanProjectFormatMigration(1, 1)
	if err != nil || len(steps) != 0 {
		t.Fatalf("PlanProjectFormatMigration(1, 1) = %#v, %v", steps, err)
	}
	if _, err := app.PlanProjectFormatMigration(1, 2); err == nil || !strings.Contains(err.Error(), "no project format migration step from 1 to 2") {
		t.Fatalf("missing chain step error = %v", err)
	}
	if _, err := app.PlanProjectFormatMigration(2, 1); err == nil || !strings.Contains(err.Error(), "never downgrade") {
		t.Fatalf("downgrade error = %v", err)
	}
}
