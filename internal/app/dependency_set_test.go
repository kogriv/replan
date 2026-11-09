package app_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestDependencySetApplyMinimalDiffAuditAndNoOp(t *testing.T) {
	project := copyAppProjectFixture(t)
	path := filepath.Join(project, "canonical", "dependencies.yaml")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	status, requiredState := "resolved", "review_ready"
	result, err := app.NewDependencyService(filestore.New(project)).Set(context.Background(), app.DependencySetRequest{
		Items:  []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &status, RequiredState: &requiredState}},
		Reason: "Reviewed predecessor state", ActorID: "actor.human.operator", EvidenceID: "evidence.github.pr.45",
		OccurredAt: "2026-11-09T10:00:00+07:00",
	})
	if err != nil || !result.Applied || result.NoOp || len(result.AuditRecordPreviews) != 1 {
		t.Fatalf("Set: err=%v result=%#v", err, result)
	}
	change := result.Changes[0]
	if !reflect.DeepEqual(change.ChangedFields, []string{"status", "required_state"}) || len(change.UnchangedFields) != 0 {
		t.Fatalf("change fields = %#v", change)
	}
	if got := result.AuditRecordPreviews[0]; got.Operation != "dependency_set" || got.TargetRef.ID != change.After.ID || len(got.EvidenceRefs) != 1 || !strings.Contains(got.Reason, `status: "open" -> "resolved"`) || !strings.Contains(got.Reason, `required_state: "accepted" -> "review_ready"`) {
		t.Fatalf("audit = %#v", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(before), "    required_state: accepted\n    status: open\n", "    required_state: review_ready\n    status: resolved\n", 1)
	if string(after) != want {
		t.Fatalf("dependency write was not minimal\nwant:\n%s\ngot:\n%s", want, after)
	}
	formatted, err := app.NewProjectService(filestore.New(project)).Format(context.Background(), app.ProjectFormatRequest{Reason: "Check canonical formatting", ActorID: "actor.human.operator", DryRun: true})
	if err != nil || formatted.ChangedFiles != 0 || formatted.ChangedLines != 0 {
		t.Fatalf("project format would change dependency set output: err=%v result=%#v", err, formatted)
	}

	snapshot := snapshotFiles(t, project)
	noOp, err := app.NewDependencyService(filestore.New(project)).Set(context.Background(), app.DependencySetRequest{
		Items:  []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &status}},
		Reason: "Confirm resolved", ActorID: "actor.human.operator",
	})
	if err != nil || !noOp.NoOp || noOp.Applied || len(noOp.AuditRecordPreviews) != 0 || len(noOp.PlannedWrites) != 0 || !reflect.DeepEqual(snapshot, snapshotFiles(t, project)) {
		t.Fatalf("no-op: err=%v result=%#v", err, noOp)
	}
	if !reflect.DeepEqual(noOp.Changes[0].UnchangedFields, []string{"status"}) {
		t.Fatalf("no-op fields = %#v", noOp.Changes[0])
	}
}

func TestDependencySetBatchValidationIsAllOrNothing(t *testing.T) {
	project := copyAppProjectFixture(t)
	before := snapshotFiles(t, project)
	resolved, invalid := "resolved", "urgent"
	result, err := app.NewDependencyService(filestore.New(project)).Set(context.Background(), app.DependencySetRequest{
		Items: []app.DependencySetItem{
			{ID: "dependency.repair-pump-before-validation", Status: &resolved},
			{ID: "dependency.openai-quota-blocks-validation", Criticality: &invalid},
		},
		Reason: "Batch review", ActorID: "actor.human.operator",
	})
	if err == nil || !hasErrorCode(result.Errors, "invalid_dependency_criticality") || !reflect.DeepEqual(before, snapshotFiles(t, project)) {
		t.Fatalf("all-or-nothing: err=%v result=%#v", err, result)
	}

	result, err = app.NewDependencyService(filestore.New(project)).Set(context.Background(), app.DependencySetRequest{
		Items: []app.DependencySetItem{
			{ID: "dependency.repair-pump-before-validation", Status: &resolved},
			{ID: "dependency.repair-pump-before-validation", Status: &resolved},
		},
		Reason: "Duplicate", ActorID: "actor.human.operator", DryRun: true,
	})
	if err == nil || !hasErrorCode(result.Errors, "duplicate_dependency_id") {
		t.Fatalf("duplicate: err=%v result=%#v", err, result)
	}
}

func TestDependencySetRefusalsAndBatchAuditReasons(t *testing.T) {
	project := copyAppProjectFixture(t)
	resolved, invalidStatus := "resolved", "blocked"
	tests := []struct {
		name, code string
		req        app.DependencySetRequest
	}{
		{"unknown dependency", "dependency_not_found", app.DependencySetRequest{Items: []app.DependencySetItem{{ID: "dependency.missing", Status: &resolved}}, Reason: "Review", ActorID: "actor.human.operator", DryRun: true}},
		{"unknown actor", "actor_not_found", app.DependencySetRequest{Items: []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &resolved}}, Reason: "Review", ActorID: "actor.missing", DryRun: true}},
		{"unknown evidence", "evidence_not_found", app.DependencySetRequest{Items: []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &resolved}}, Reason: "Review", ActorID: "actor.human.operator", EvidenceID: "evidence.missing", DryRun: true}},
		{"invalid value", "invalid_dependency_status", app.DependencySetRequest{Items: []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &invalidStatus}}, Reason: "Review", ActorID: "actor.human.operator", DryRun: true}},
		{"nothing requested", "missing_dependency_change", app.DependencySetRequest{Items: []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation"}}, Reason: "Review", ActorID: "actor.human.operator", DryRun: true}},
		{"missing reason", "missing_reason", app.DependencySetRequest{Items: []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &resolved}}, ActorID: "actor.human.operator", DryRun: true}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := app.NewDependencyService(filestore.New(project)).Set(context.Background(), tc.req)
			if err == nil || !hasErrorCode(result.Errors, tc.code) {
				t.Fatalf("err=%v errors=%#v, want %s", err, result.Errors, tc.code)
			}
		})
	}

	cancelled := "cancelled"
	result, err := app.NewDependencyService(filestore.New(project)).Set(context.Background(), app.DependencySetRequest{
		Items: []app.DependencySetItem{
			{ID: "dependency.repair-pump-before-validation", Status: &resolved, Reason: "Predecessor was reviewed"},
			{ID: "dependency.openai-quota-blocks-validation", Status: &cancelled},
		},
		Reason: "Shared batch reason", ActorID: "actor.human.operator", OccurredAt: "2026-11-09T12:00:00+07:00", DryRun: true,
	})
	if err != nil || len(result.AuditRecordPreviews) != 2 || result.AuditRecordPreviews[0].ID == result.AuditRecordPreviews[1].ID {
		t.Fatalf("batch audits: err=%v result=%#v", err, result)
	}
	if !strings.HasPrefix(result.AuditRecordPreviews[0].Reason, "Predecessor was reviewed.") || !strings.HasPrefix(result.AuditRecordPreviews[1].Reason, "Shared batch reason.") {
		t.Fatalf("batch audit reasons = %#v", result.AuditRecordPreviews)
	}
}

func TestDependencySetClosedAndReviewReadyEffects(t *testing.T) {
	project := copyAppProjectFixture(t)
	service := app.NewDependencyService(filestore.New(project))
	resolved := "resolved"
	if _, err := service.Set(context.Background(), app.DependencySetRequest{
		Items:  []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &resolved}},
		Reason: "Dependency is closed", ActorID: "actor.human.operator",
	}); err != nil {
		t.Fatal(err)
	}
	validation, err := app.NewValidateService(filestore.New(project)).Validate(context.Background(), app.ValidateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, warning := range validation.Warnings {
		if warning.Code == "required_dependency_unresolved" && warning.Target.ID == "dependency.repair-pump-before-validation" {
			t.Fatalf("closed dependency still warns: %#v", warning)
		}
	}
	suggestions, err := app.NewProposalService(filestore.New(project)).SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, proposal := range suggestions.Proposals {
		if proposal.TargetRef.ID == "dependency.repair-pump-before-validation" {
			t.Fatalf("closed dependency still suggests resolution: %#v", proposal)
		}
	}

	tasksPath := filepath.Join(project, "canonical", "tasks.yaml")
	tasks, err := os.ReadFile(tasksPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.Replace(string(tasks), "tracking_status: in_progress", "tracking_status: review_ready", 1)
	if err := os.WriteFile(tasksPath, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	open, reviewReady := "open", "review_ready"
	if _, err := service.Set(context.Background(), app.DependencySetRequest{
		Items:  []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &open, RequiredState: &reviewReady}},
		Reason: "Reopen against review readiness", ActorID: "actor.human.operator",
	}); err != nil {
		t.Fatal(err)
	}
	listed, err := service.List(context.Background(), app.DependencyListRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasDependencyRow(listed.Dependencies, "dependency.repair-pump-before-validation", true, true) {
		t.Fatalf("review_ready did not satisfy review_ready: %#v", listed.Dependencies)
	}
}

func TestDependencySetBatchItemReasonsReplaceTheSharedReason(t *testing.T) {
	project := copyAppProjectFixture(t)
	status := "resolved"
	result, err := app.NewDependencyService(filestore.New(project)).Set(context.Background(), app.DependencySetRequest{
		Items:   []app.DependencySetItem{{ID: "dependency.repair-pump-before-validation", Status: &status, Reason: "Successor finished first"}},
		ActorID: "actor.human.operator", OccurredAt: "2026-11-09T10:00:00+07:00", DryRun: true,
	})
	if err != nil || len(result.Errors) != 0 || len(result.AuditRecordPreviews) != 1 || !strings.HasPrefix(result.AuditRecordPreviews[0].Reason, "Successor finished first.") {
		t.Fatalf("Set: err=%v result=%#v", err, result)
	}
}
