package app_test

import (
	"context"
	"fmt"
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
		{name: "successor without a start", statusDate: "2026-09-25", predecessor: "not_started", successor: "not_started", dependencyStatus: "open"},
		{name: "project without a status date", predecessor: "not_started", successor: "not_started", successorStart: "2026-09-30", dependencyStatus: "open"},
		{name: "started successor, no status date", predecessor: "not_started", successor: "in_progress", successorStart: "2026-09-30", dependencyStatus: "open", warn: true},
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
		{"project without a status date", "", false},
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

// Open dependencies of undated tasks are counted in one warning, not one each.
func TestUndatedSuccessorsWarnOnce(t *testing.T) {
	ws := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-09-25"},
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "not_started"},
			{ID: "task.b", Title: "B", TrackingStatus: "not_started"},
			{ID: "task.c", Title: "C", TrackingStatus: "not_started"},
			{ID: "task.d", Title: "D", TrackingStatus: "in_progress"},
		},
	}
	for i, successor := range []string{"task.b", "task.c", "task.d"} {
		ws.Dependencies = append(ws.Dependencies, domain.Dependency{
			ID: fmt.Sprintf("dependency.%d", i), Type: "requires_input_from", Criticality: "required", Status: "open", RequiredState: "accepted",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: successor},
		})
	}
	result, _ := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
	var unresolved, undated []string
	for _, warning := range result.Warnings {
		switch warning.Code {
		case "required_dependency_unresolved":
			unresolved = append(unresolved, warning.Target.ID)
		case "required_dependencies_undated":
			undated = append(undated, warning.Message)
		}
	}
	// The started successor still warns on its own; the two undated ones are counted once.
	if len(unresolved) != 1 || unresolved[0] != "dependency.2" {
		t.Fatalf("unresolved warnings = %v", unresolved)
	}
	if len(undated) != 1 || !strings.HasPrefix(undated[0], "2 open required dependencies") {
		t.Fatalf("undated warnings = %v", undated)
	}
}

// Without a status date, dependencies of work not yet started are counted once.
func TestDependenciesWithoutStatusDateWarnOnce(t *testing.T) {
	ws := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test"},
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "not_started", PlannedStart: "2026-09-28", PlannedFinish: "2026-09-29"},
			{ID: "task.b", Title: "B", TrackingStatus: "not_started", PlannedStart: "2026-09-30", PlannedFinish: "2026-10-01"},
			{ID: "task.c", Title: "C", TrackingStatus: "not_started", PlannedStart: "2026-10-02", PlannedFinish: "2026-10-03"},
		},
	}
	for i, successor := range []string{"task.b", "task.c"} {
		ws.Dependencies = append(ws.Dependencies, domain.Dependency{
			ID: fmt.Sprintf("dependency.%d", i), Type: "finish_to_start", Criticality: "required", Status: "open", RequiredState: "accepted",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: successor},
		})
	}
	result, _ := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
	counts := map[string]int{}
	for _, warning := range result.Warnings {
		counts[warning.Code]++
	}
	if counts["required_dependency_unresolved"] != 0 || counts["required_dependencies_without_status_date"] != 1 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}
