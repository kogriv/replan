package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

// A required dependency that is still open warns only when it matters now:
// a plan made ahead has open dependencies by design.
func TestRequiredDependencyWarnsOnlyWhenDue(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		statusDate, predecessor, successor    string
		successorStart, dependencyStatus, msg string
		warn                                  bool
	}{
		{name: "successor starts later", statusDate: "2026-09-25", predecessor: "not_started", successor: "not_started", successorStart: "2026-09-30", dependencyStatus: "open"},
		{name: "successor starts on the status date", statusDate: "2026-09-30", predecessor: "not_started", successor: "not_started", successorStart: "2026-09-30", dependencyStatus: "open", warn: true, msg: "planned to start on 2026-09-30"},
		{name: "successor started early", statusDate: "2026-09-25", predecessor: "in_progress", successor: "in_progress", successorStart: "2026-09-30", dependencyStatus: "open", warn: true, msg: "is in_progress"},
		{name: "predecessor done, record stale", statusDate: "2026-09-25", predecessor: "accepted", successor: "not_started", successorStart: "2026-09-30", dependencyStatus: "open", warn: true, msg: "record the dependency as resolved"},
		{name: "successor without a start", statusDate: "2026-09-25", predecessor: "not_started", successor: "not_started", dependencyStatus: "open", warn: true},
		{name: "project without a status date", predecessor: "not_started", successor: "not_started", successorStart: "2026-09-30", dependencyStatus: "open", warn: true},
		{name: "blocked dependency", statusDate: "2026-09-25", predecessor: "not_started", successor: "not_started", successorStart: "2026-09-30", dependencyStatus: "blocked", warn: true},
	} {
		ws := domain.Workspace{
			Project: domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: tc.statusDate},
			Tasks: []domain.Task{
				{ID: "task.a", Title: "A", TrackingStatus: tc.predecessor, PlannedStart: "2026-09-28", PlannedFinish: "2026-09-29"},
				{ID: "task.b", Title: "B", TrackingStatus: tc.successor, PlannedStart: tc.successorStart, PlannedFinish: "2026-10-02"},
			},
			Dependencies: []domain.Dependency{{
				ID: "dependency.a-before-b", Type: "finish_to_start", Criticality: "required", Status: tc.dependencyStatus, RequiredState: "accepted",
				PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
			}},
		}
		result, _ := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
		var message string
		for _, warning := range result.Warnings {
			if warning.Code == "required_dependency_unresolved" {
				message = warning.Message
			}
		}
		if (message != "") != tc.warn {
			t.Errorf("%s: warned = %v, want %v (%#v)", tc.name, message != "", tc.warn, result.Warnings)
			continue
		}
		if tc.msg != "" && !strings.Contains(message, tc.msg) {
			t.Errorf("%s: message %q does not contain %q", tc.name, message, tc.msg)
		}
	}
}

// Review proposals follow the same rule as the warning: a plan made ahead
// does not ask for a review of every future dependency.
func TestSuggestDependenciesReviewsOnlyDueDependencies(t *testing.T) {
	for _, tc := range []struct {
		name, statusDate string
		want             bool
	}{
		{"successor starts later", "2026-09-25", false},
		{"successor due by the status date", "2026-09-30", true},
		{"project without a status date", "", true},
	} {
		ws := domain.Workspace{
			Project: domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: tc.statusDate},
			Actors:  []domain.Actor{{ID: "actor.agent", Kind: "ai_agent"}},
			Tasks: []domain.Task{
				{ID: "task.a", Title: "A", TrackingStatus: "not_started", PlannedStart: "2026-09-28", PlannedFinish: "2026-09-29"},
				{ID: "task.b", Title: "B", TrackingStatus: "not_started", PlannedStart: "2026-09-30", PlannedFinish: "2026-10-02"},
			},
			Dependencies: []domain.Dependency{{
				ID: "dependency.a-before-b", Type: "finish_to_start", Criticality: "required", Status: "open", RequiredState: "accepted",
				PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
			}},
		}
		result, err := app.NewProposalService(stubLoader{ws: ws}).SuggestDependencies(context.Background(), app.ProposalSuggestDependenciesRequest{ActorID: "actor.agent"})
		if err != nil {
			t.Fatalf("%s: SuggestDependencies returned error: %v", tc.name, err)
		}
		got := false
		for _, proposal := range result.Proposals {
			if proposal.Type == "review_dependency_resolution" {
				got = true
			}
		}
		if got != tc.want {
			t.Errorf("%s: review proposal = %v, want %v", tc.name, got, tc.want)
		}
	}
}
