package app_test

import (
	"context"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func constraintWorkspace() domain.Workspace {
	return domain.Workspace{
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "not_started"},
			{ID: "task.b", Title: "B", TrackingStatus: "not_started"},
			{ID: "task.c", Title: "C", TrackingStatus: "not_started"},
		},
		Actors:   []domain.Actor{{ID: "actor.operator", Kind: "human"}},
		Evidence: []domain.Evidence{{ID: "evidence.call", Type: "customer_call"}},
	}
}

func TestDependencyAddDryRunDefaultsAndCycleRefusal(t *testing.T) {
	ws := constraintWorkspace()
	ws.Dependencies = []domain.Dependency{
		{ID: "dependency.a-b", Type: "finish_to_start", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}, Status: "open", Criticality: "required"},
		{ID: "dependency.b-c", Type: "finish_to_start", PredecessorRef: domain.Ref{Kind: "task", ID: "task.b"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.c"}, Status: "open", Criticality: "required"},
	}
	service := app.NewDependencyService(stubLoader{ws: ws})
	result, err := service.Add(context.Background(), app.DependencyAddRequest{Items: []app.DependencyAddItem{{PredecessorID: "task.c", SuccessorID: "task.a"}}, Reason: "Customer call", ActorID: "actor.operator", EvidenceID: "evidence.call", DryRun: true})
	if err == nil || !hasErrorCode(result.Errors, "dependency_cycle") {
		t.Fatalf("err=%v errors=%#v", err, result.Errors)
	}

	result, err = service.Add(context.Background(), app.DependencyAddRequest{Items: []app.DependencyAddItem{{Type: "relates", PredecessorID: "task.a", SuccessorID: "task.c"}}, Reason: "Customer call", ActorID: "actor.operator", EvidenceID: "evidence.call", DryRun: true})
	if err != nil {
		t.Fatalf("Add returned error: %v (%#v)", err, result.Errors)
	}
	if len(result.Dependencies) != 1 || result.Dependencies[0].Criticality != "optional" || result.Dependencies[0].Status != "open" || result.Dependencies[0].ID == "" || len(result.AuditRecordPreviews) != 1 {
		t.Fatalf("result=%#v", result)
	}
}

func TestDeadlineAddDryRunAndList(t *testing.T) {
	ws := constraintWorkspace()
	service := app.NewDeadlineService(stubLoader{ws: ws})
	result, err := service.Add(context.Background(), app.DeadlineAddRequest{Items: []app.DeadlineAddItem{{ID: "deadline.a", TaskID: "task.a", Date: "2026-11-10"}}, Reason: "Customer call", ActorID: "actor.operator", EvidenceID: "evidence.call", DryRun: true})
	if err != nil {
		t.Fatalf("Add returned error: %v (%#v)", err, result.Errors)
	}
	if got := result.Deadlines[0]; got.Type != "due_date" || got.Criticality != "hard" || got.DueDate != "2026-11-10" {
		t.Fatalf("deadline=%#v", got)
	}
	ws.Deadlines = result.Deadlines
	listed, err := app.NewDeadlineService(stubLoader{ws: ws}).List(context.Background())
	if err != nil || listed.Summary.Deadlines != 1 || listed.Summary.Hard != 1 {
		t.Fatalf("listed=%#v err=%v", listed, err)
	}
}
