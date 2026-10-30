package app_test

import (
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestTrackingSuggestFindsStaleRemainingCandidates(t *testing.T) {
	projectPath := copyTrackingProjectFixture(t)
	removeTrackingProposalArtifacts(t, projectPath)
	markTrackingProposalDemandsStale(t, projectPath)
	service := app.NewTrackingService(filestore.New(projectPath))
	result, err := service.Suggest(context.Background(), app.TrackingSuggestRequest{
		ProjectPath: projectPath,
		Week:        "2026-W31",
	})
	if err != nil {
		t.Fatalf("Suggest returned error: %v", err)
	}
	if result.Week != "2026-W31" {
		t.Fatalf("week = %q, want 2026-W31", result.Week)
	}
	if len(result.Suggestions) == 0 {
		t.Fatalf("expected tracking suggestions")
	}
	if len(result.Proposals) == 0 {
		t.Fatalf("expected proposal candidates")
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.Type == "set_remaining_to_zero_candidate" {
			if suggestion.RecommendedCommand == "" {
				t.Fatalf("set_remaining_to_zero_candidate missing recommended command: %#v", suggestion)
			}
			return
		}
	}
	t.Fatalf("missing set_remaining_to_zero_candidate in %#v", result.Suggestions)
}

func TestTrackingSuggestWriteCandidates(t *testing.T) {
	projectPath := copyTrackingProjectFixture(t)
	removeTrackingProposalArtifacts(t, projectPath)
	markTrackingProposalDemandsStale(t, projectPath)
	service := app.NewTrackingService(filestore.New(projectPath))
	result, err := service.Suggest(context.Background(), app.TrackingSuggestRequest{
		Week:            "2026-W31",
		ActorID:         "actor.ai.codex.local",
		WriteCandidates: true,
		Now:             fixedTrackingTestTime(),
	})
	if err != nil {
		t.Fatalf("Suggest write returned error: %v", err)
	}
	if result.DryRun {
		t.Fatalf("DryRun = true, want false")
	}
	if !result.Applied {
		t.Fatalf("Applied = false, want true")
	}
	if len(result.WrittenFiles) == 0 {
		t.Fatalf("expected written proposal files")
	}
	proposalPath := filepath.Join(projectPath, "proposals", "open", "proposal.tracking.resource_demand-tracking-proposals-maintainer.set-remaining-zero.yaml")
	data, err := os.ReadFile(proposalPath)
	if err != nil {
		t.Fatalf("read proposal candidate: %v", err)
	}
	for _, want := range []string{
		"type: update_remaining_work",
		"created_at: \"2026-07-24T07:00:00Z\"",
		"id: actor.ai.codex.local",
		"path: remaining_amount",
		"value: 0",
		"requires_confirmation: true",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("proposal missing %q:\n%s", want, data)
		}
	}
}

func TestTrackingSuggestWriteCandidatesRequiresActor(t *testing.T) {
	service := app.NewTrackingService(filestore.New("../../examples/replan-self"))
	result, err := service.Suggest(context.Background(), app.TrackingSuggestRequest{
		Week:            "2026-W31",
		WriteCandidates: true,
	})
	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("error = %v, want ErrValidationFailed", err)
	}
	if !hasTrackingDiagnostic(result.Errors, "actor_required") {
		t.Fatalf("expected actor_required, got %#v", result.Errors)
	}
}

func TestTrackingSuggestForecastUsesCorrectedAmount(t *testing.T) {
	corrected := .1
	ws := domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}, Resources: []domain.Resource{{ID: "resource.agent", Class: "ai_agent"}}, Tasks: []domain.Task{{ID: "task.test", Title: "Test", TrackingStatus: "in_progress"}}, ResourceDemands: []domain.ResourceDemand{{ID: "demand.test", TaskRef: domain.Ref{ID: "task.test"}, ResourceRef: domain.Ref{ID: "resource.agent"}, ResourceClass: "ai_agent", Unit: "agent_hours", PlannedAmount: .25, RemainingAmount: domain.Amount(.2)}}, Actuals: []domain.Actual{
		{ID: "actual.test", Type: "work_consumed", TaskRef: domain.Ref{ID: "task.test"}, DemandRef: domain.Ref{ID: "demand.test"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Amount: .33, Unit: "agent_hours", EvidenceRefs: []domain.Ref{{ID: "evidence.test"}}},
		{ID: "correction.test", Type: "correction", CorrectsRef: domain.Ref{Kind: "actual", ID: "actual.test"}, CorrectedAmount: &corrected, TaskRef: domain.Ref{ID: "task.test"}, DemandRef: domain.Ref{ID: "demand.test"}, ResourceRef: domain.Ref{ID: "resource.agent"}, Unit: "agent_hours", OccurredAt: "2026-10-30T12:00:00Z", EvidenceRefs: []domain.Ref{{ID: "evidence.test"}}},
	}}
	result, err := app.NewTrackingService(staticWorkspaceLoader{workspace: ws}).Suggest(context.Background(), app.TrackingSuggestRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, suggestion := range result.Suggestions {
		if suggestion.DemandID == "demand.test" {
			if suggestion.Actual != corrected || math.Abs(suggestion.Forecast-.3) > 1e-9 {
				t.Fatalf("suggestion = %#v", suggestion)
			}
			return
		}
	}
	t.Fatalf("forecast suggestion missing: %#v", result.Suggestions)
}

func TestTrackingSuggestValidationPrecedenceStopsBeforeAnalysis(t *testing.T) {
	service := app.NewTrackingService(staticWorkspaceLoader{
		projectPath: "memory",
		workspace:   domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}},
		diagnostics: []app.Diagnostic{{Code: "load_failed_first", Severity: "error"}},
	})

	result, err := service.Suggest(context.Background(), app.TrackingSuggestRequest{WriteCandidates: true})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Suggest error = %v, want ErrValidationFailed", err)
	}
	want := []string{"load_failed_first", "actor_required"}
	if got := diagnosticCodeList(result.Errors); !reflect.DeepEqual(got, want) {
		t.Fatalf("error codes = %#v, want %#v", got, want)
	}
	if len(result.Suggestions) != 0 || len(result.Proposals) != 0 || len(result.PlannedWrites) != 0 {
		t.Fatalf("partial result advanced past validation: %#v", result)
	}
}

func TestTrackingSuggestFindsMissingActualsAndEvidence(t *testing.T) {
	service := app.NewTrackingService(staticWorkspaceLoader{
		projectPath: "memory",
		workspace: domain.Workspace{
			Project: domain.Project{ID: "project.test", Name: "Test project", DefaultStatusDate: "2026-07-24"},
			Tasks: []domain.Task{
				{ID: "task.accepted-without-actuals", Title: "Accepted without actuals", TrackingStatus: "accepted"},
				{ID: "task.actual-without-evidence", Title: "Actual without evidence", TrackingStatus: "accepted"},
			},
			ResourceDemands: []domain.ResourceDemand{
				{
					ID:              "demand.accepted-without-actuals",
					TaskRef:         domain.Ref{Kind: "task", ID: "task.accepted-without-actuals"},
					ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
					ResourceClass:   "human",
					Role:            "implementer",
					Unit:            "hours",
					PlannedAmount:   1,
					RemainingAmount: domain.Amount(1),
				},
				{
					ID:              "demand.actual-without-evidence",
					TaskRef:         domain.Ref{Kind: "task", ID: "task.actual-without-evidence"},
					ResourceRef:     domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
					ResourceClass:   "human",
					Role:            "implementer",
					Unit:            "hours",
					PlannedAmount:   1,
					RemainingAmount: domain.Amount(0),
				},
			},
			Actuals: []domain.Actual{{
				ID:          "actual.without-evidence",
				Type:        "consumption",
				TaskRef:     domain.Ref{Kind: "task", ID: "task.actual-without-evidence"},
				DemandRef:   domain.Ref{Kind: "resource_demand", ID: "demand.actual-without-evidence"},
				ResourceRef: domain.Ref{Kind: "resource", ID: "resource.human.ivan"},
				Amount:      1,
				Unit:        "hours",
				AcceptedBy:  domain.Ref{Kind: "actor", ID: "actor.human.operator"},
			}},
		},
	})
	result, err := service.Suggest(context.Background(), app.TrackingSuggestRequest{})
	if err != nil {
		t.Fatalf("Suggest returned error: %v", err)
	}
	if !hasTrackingSuggestionType(result.Suggestions, "review_missing_actuals") {
		t.Fatalf("missing review_missing_actuals in %#v", result.Suggestions)
	}
	if !hasTrackingSuggestionType(result.Suggestions, "review_missing_evidence") {
		t.Fatalf("missing review_missing_evidence in %#v", result.Suggestions)
	}
}

func fixedTrackingTestTime() time.Time {
	return time.Date(2026, 7, 24, 7, 0, 0, 0, time.UTC)
}

func copyTrackingProjectFixture(t *testing.T) string {
	t.Helper()
	src := filepath.Clean("../../examples/replan-self")
	dst := filepath.Join(t.TempDir(), "replan-self")
	if err := filepath.WalkDir(src, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(dstPath, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	}); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	return dst
}

func hasTrackingSuggestionType(suggestions []app.TrackingSuggestion, want string) bool {
	for _, suggestion := range suggestions {
		if suggestion.Type == want {
			return true
		}
	}
	return false
}

func hasTrackingDiagnostic(diags []app.Diagnostic, want string) bool {
	for _, diag := range diags {
		if diag.Code == want {
			return true
		}
	}
	return false
}

func diagnosticCodeList(diags []app.Diagnostic) []string {
	codes := make([]string, 0, len(diags))
	for _, diag := range diags {
		codes = append(codes, diag.Code)
	}
	return codes
}
