package app_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/filestore"
)

func TestProjectOpenProjectIDsPolicyParseAndValidate(t *testing.T) {
	for _, tc := range []struct {
		name, value, effective, errorCode string
	}{
		{name: "default", effective: domain.OpenProjectIDsCanonical},
		{name: "canonical", value: domain.OpenProjectIDsCanonical, effective: domain.OpenProjectIDsCanonical},
		{name: "private bindings", value: domain.OpenProjectIDsPrivateBindings, effective: domain.OpenProjectIDsPrivateBindings},
		{name: "invalid", value: "elsewhere", effective: "elsewhere", errorCode: "invalid_openproject_ids_policy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			projectDir := copyProjectFixture(t)
			if tc.value != "" {
				path := filepath.Join(projectDir, ".replan", "project.yaml")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				data = []byte(strings.Replace(string(data), "  timezone:", "  openproject_ids: "+tc.value+"\n  timezone:", 1))
				if err := os.WriteFile(path, data, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			store := filestore.New(projectDir)
			ws, diags := store.Load(context.Background())
			if len(diags) != 0 {
				t.Fatalf("load diagnostics: %#v", diags)
			}
			if got := ws.Project.EffectiveOpenProjectIDs(); got != tc.effective {
				t.Fatalf("effective policy = %q, want %q", got, tc.effective)
			}
			result, err := app.NewValidateService(store).Validate(context.Background(), app.ValidateRequest{})
			if tc.errorCode == "" {
				if err != nil {
					t.Fatalf("Validate returned error: %v %#v", err, result.Errors)
				}
				return
			}
			if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, tc.errorCode) {
				t.Fatalf("Validate error = %v, errors = %#v", err, result.Errors)
			}
		})
	}
}

type stubLoader struct {
	ws domain.Workspace
}

func (l stubLoader) Load(ctx context.Context) (domain.Workspace, []app.Diagnostic) {
	_ = ctx
	return l.ws, nil
}

func (l stubLoader) ProjectPath() string {
	return "stub"
}

func TestValidateSmallTeamFixture(t *testing.T) {
	store := filestore.New("../../examples/small-team")
	service := app.NewValidateService(store)

	result, err := service.Validate(context.Background(), app.ValidateRequest{
		ProjectPath:       "../../examples/small-team",
		IncludeObjectScan: true,
	})
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if !result.Valid {
		t.Fatalf("expected fixture to be valid, errors: %#v", result.Errors)
	}
	if got := result.ObjectCounts["tasks"]; got != 2 {
		t.Fatalf("expected 2 tasks, got %d", got)
	}
	if got := result.ObjectCounts["resources"]; got != 7 {
		t.Fatalf("expected 7 resources, got %d", got)
	}
	if got := result.ObjectCounts["resource_demands"]; got != 7 {
		t.Fatalf("expected 7 resource demands, got %d", got)
	}
	if got := result.ObjectCounts["dependencies"]; got != 2 {
		t.Fatalf("expected 2 dependencies, got %d", got)
	}
	if len(result.Warnings) != 5 {
		t.Fatalf("expected 5 dependency warnings, got %#v", result.Warnings)
	}
	for _, code := range []string{
		"required_dependency_unresolved",
		"dependency_predecessor_finish_unknown",
		"dependency_successor_start_unknown",
		"dependency_blocks_active_successor",
	} {
		if !hasDiagnostic(result.Warnings, code) {
			t.Fatalf("expected warning %s, got %#v", code, result.Warnings)
		}
	}
}

func TestValidateSchedulingRules(t *testing.T) {
	falseValue := false
	tests := []struct {
		name, code string
		task       domain.Task
		demand     domain.ResourceDemand
		warning    bool
	}{
		{name: "both lead-time units", code: "invalid_task_lead_time", task: domain.Task{LeadTime: &domain.LeadTime{WorkingDays: 1, CalendarDays: 1}}},
		{name: "neither lead-time unit", code: "invalid_task_lead_time", task: domain.Task{LeadTime: &domain.LeadTime{}}},
		{name: "non-positive lead time", code: "invalid_task_lead_time", task: domain.Task{LeadTime: &domain.LeadTime{WorkingDays: -1}}},
		{name: "priority below range", code: "invalid_task_priority", task: domain.Task{Priority: -1}},
		{name: "priority above range", code: "invalid_task_priority", task: domain.Task{Priority: 1001}},
		{name: "unlevelled non-hour unit", code: "invalid_unlevelled_demand_unit", demand: domain.ResourceDemand{Unit: "tokens", Levelled: &falseValue}},
		{name: "calendar span warning", code: "task_planned_span_shorter_than_lead_time", warning: true, task: domain.Task{PlannedStart: "2026-11-02", PlannedFinish: "2026-11-03", LeadTime: &domain.LeadTime{CalendarDays: 3}}},
		{name: "calendar span is elapsed days", code: "task_planned_span_shorter_than_lead_time", warning: true, task: domain.Task{PlannedStart: "2026-11-02", PlannedFinish: "2026-11-03", LeadTime: &domain.LeadTime{CalendarDays: 2}}},
		{name: "working span warning", code: "task_planned_span_shorter_than_lead_time", warning: true, task: domain.Task{PlannedStart: "2026-11-06", PlannedFinish: "2026-11-09", LeadTime: &domain.LeadTime{WorkingDays: 3}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			task := tc.task
			task.ID, task.Title, task.TrackingStatus = "task.test", "Test", "not_started"
			ws := domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}, Tasks: []domain.Task{task}}
			if tc.demand.Unit != "" {
				tc.demand.ID, tc.demand.TaskRef, tc.demand.ResourceClass = "demand.test", domain.Ref{Kind: "task", ID: task.ID}, "service"
				ws.ResourceDemands = []domain.ResourceDemand{tc.demand}
			}
			result, _ := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
			diagnostics := result.Errors
			if tc.warning {
				diagnostics = result.Warnings
			}
			if !hasDiagnostic(diagnostics, tc.code) {
				t.Fatalf("missing %s: errors=%#v warnings=%#v", tc.code, result.Errors, result.Warnings)
			}
		})
	}
}

func TestValidateAllowsUnlevelledHourUnits(t *testing.T) {
	falseValue := false
	for _, unit := range []string{"hours", "hour", "h", "agent_hours", "minutes"} {
		ws := domain.Workspace{Project: domain.Project{ID: "project.test", Name: "Test"}, Tasks: []domain.Task{{ID: "task.test", Title: "Test", TrackingStatus: "not_started"}}, ResourceDemands: []domain.ResourceDemand{{ID: "demand.test", TaskRef: domain.Ref{Kind: "task", ID: "task.test"}, ResourceClass: "human", Unit: unit, Levelled: &falseValue}}}
		result, _ := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
		if hasDiagnostic(result.Errors, "invalid_unlevelled_demand_unit") {
			t.Errorf("unit %s was rejected: %#v", unit, result.Errors)
		}
	}
}

func TestValidateRejectsMissingCurrentPlanBaseline(t *testing.T) {
	ws := domain.Workspace{
		Project:     domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC"},
		CurrentPlan: domain.CurrentPlan{ID: "current", BasedOnBaseline: domain.Ref{Kind: "baseline", ID: "baseline.missing"}},
	}
	result, err := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "unresolved_reference") {
		t.Fatalf("Validate error = %v, errors = %#v", err, result.Errors)
	}
}

func TestValidateWarnsAboutDemandTaskOwnership(t *testing.T) {
	ref := func(kind, id string) domain.Ref { return domain.Ref{Kind: kind, ID: id} }
	ws := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC"},
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "ready", ResourceDemands: []domain.Ref{ref("resource_demand", "demand.b")}},
			{ID: "task.b", Title: "B", TrackingStatus: "ready", ResourceDemands: []domain.Ref{ref("resource_demand", "demand.b")}},
		},
		Resources: []domain.Resource{{ID: "resource.a", Class: "human", Name: "A"}},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.a", TaskRef: ref("task", "task.a"), ResourceRef: ref("resource", "resource.a"), ResourceClass: "human", Role: "work", Unit: "hours", PlannedAmount: 1, Status: "active"},
			{ID: "demand.b", TaskRef: ref("task", "task.b"), ResourceRef: ref("resource", "resource.a"), ResourceClass: "human", Role: "work", Unit: "hours", PlannedAmount: 1, Status: "active"},
		},
	}
	result, err := app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{})
	if err != nil {
		t.Fatalf("Validate error=%v errors=%#v", err, result.Errors)
	}
	if !hasDiagnostic(result.Warnings, "demand_not_listed_by_task") || !hasDiagnostic(result.Warnings, "task_lists_demand_of_other_task") {
		t.Fatalf("ownership warnings=%#v", result.Warnings)
	}
}

func TestValidateRejectsPriorCalibrationWithoutSource(t *testing.T) {
	calibration := domain.Calibration{
		ID: "calibration.prior", ApprovedBy: domain.Ref{Kind: "actor", ID: "actor.operator"},
		Result: domain.CalibrationResult{Method: "prior", Factor: .21},
	}
	if err := domain.SealCalibration(&calibration); err != nil {
		t.Fatal(err)
	}
	result, err := app.NewValidateService(stubLoader{ws: domain.Workspace{
		Actors: []domain.Actor{{ID: "actor.operator", Kind: "human"}}, Calibrations: []domain.Calibration{calibration},
	}}).Validate(context.Background(), app.ValidateRequest{})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "calibration_prior_source_required") {
		t.Fatalf("Validate error=%v errors=%#v", err, result.Errors)
	}
}

func TestValidateRejectsSecretLikeOpenProjectTokenEnv(t *testing.T) {
	service := app.NewValidateService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:       "project.test",
			Name:     "Test",
			Timezone: "UTC",
			SourceProfiles: []domain.SourceProfile{{
				ID:        "openproject.test",
				Source:    "openproject",
				BaseURL:   "https://pm.example.test",
				ProjectID: "demo",
				TokenEnv:  "op-token-secret-value",
			}},
		},
	}})

	result, err := service.Validate(context.Background(), app.ValidateRequest{})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Validate error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "invalid_openproject_token_env") {
		t.Fatalf("expected invalid_openproject_token_env error")
	}
}

func TestValidateRejectsInvalidTaskPlannedDate(t *testing.T) {
	service := app.NewValidateService(stubLoader{ws: domain.Workspace{
		Tasks: []domain.Task{{
			ID:             "task.invalid-date",
			Title:          "Invalid planned date",
			TrackingStatus: "ready",
			PlannedStart:   "2026-99-99",
		}},
	}})

	result, err := service.Validate(context.Background(), app.ValidateRequest{})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Validate error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "invalid_task_planned_start") {
		t.Fatalf("expected invalid_task_planned_start error, got %#v", result.Errors)
	}
}

func TestValidateSkipsDateOrderWarningsForResolvedFinishToStartDependency(t *testing.T) {
	service := app.NewValidateService(stubLoader{ws: domain.Workspace{
		Project: domain.Project{
			ID:       "project.test",
			Name:     "Test",
			Timezone: "UTC",
		},
		Tasks: []domain.Task{
			{
				ID:             "task.predecessor",
				Title:          "Predecessor",
				TrackingStatus: "accepted",
				PlannedFinish:  "2026-08-14",
			},
			{
				ID:             "task.successor",
				Title:          "Successor",
				TrackingStatus: "in_progress",
				PlannedStart:   "2026-07-30",
			},
		},
		Dependencies: []domain.Dependency{{
			ID:             "dependency.resolved-history",
			Type:           "finish_to_start",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.predecessor"},
			SuccessorRef:   domain.Ref{Kind: "task", ID: "task.successor"},
			Status:         "resolved",
			Criticality:    "required",
		}},
	}})

	result, err := service.Validate(context.Background(), app.ValidateRequest{})

	if err != nil {
		t.Fatalf("Validate returned error: %v; errors: %#v", err, result.Errors)
	}
	if hasDiagnostic(result.Warnings, "dependency_successor_starts_before_predecessor_finish") {
		t.Fatalf("resolved dependency should not emit date-order warning: %#v", result.Warnings)
	}
}

func TestValidateRejectsInvalidDependencyReferences(t *testing.T) {
	service := app.NewValidateService(stubLoader{ws: domain.Workspace{
		Tasks: []domain.Task{{
			ID:             "task.successor",
			Title:          "Successor",
			TrackingStatus: "ready",
			Dependencies: []domain.Ref{{
				Kind: "dependency",
				ID:   "dependency.missing",
			}},
		}},
		Dependencies: []domain.Dependency{{
			ID:             "dependency.invalid",
			Type:           "finish_to_start",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.missing"},
			SuccessorRef:   domain.Ref{Kind: "task", ID: "task.successor"},
			Status:         "open",
			Criticality:    "required",
		}},
	}})

	result, err := service.Validate(context.Background(), app.ValidateRequest{})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Validate error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "unresolved_reference") {
		t.Fatalf("expected unresolved_reference error, got %#v", result.Errors)
	}
}

func TestValidateRejectsDuplicateDependencySourceRef(t *testing.T) {
	sourceRef := domain.SourceRef{Source: "openproject", ObjectType: "relation", ObjectID: "8"}
	service := app.NewValidateService(stubLoader{ws: domain.Workspace{
		Tasks: []domain.Task{{
			ID:             "task.a",
			Title:          "A",
			TrackingStatus: "ready",
		}, {
			ID:             "task.b",
			Title:          "B",
			TrackingStatus: "ready",
		}},
		Dependencies: []domain.Dependency{{
			ID:             "dependency.one",
			Type:           "finish_to_start",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"},
			SuccessorRef:   domain.Ref{Kind: "task", ID: "task.b"},
			Status:         "open",
			Criticality:    "required",
			SourceRefs:     []domain.SourceRef{sourceRef},
		}, {
			ID:             "dependency.two",
			Type:           "blocks",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"},
			SuccessorRef:   domain.Ref{Kind: "task", ID: "task.b"},
			Status:         "open",
			Criticality:    "required",
			SourceRefs:     []domain.SourceRef{sourceRef},
		}},
	}})

	result, err := service.Validate(context.Background(), app.ValidateRequest{})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Validate error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "duplicate_dependency_source_ref") {
		t.Fatalf("expected duplicate_dependency_source_ref error, got %#v", result.Errors)
	}
}

func TestValidateRejectsDuplicateDependencyTriple(t *testing.T) {
	service := app.NewValidateService(stubLoader{ws: domain.Workspace{
		Tasks: []domain.Task{{
			ID:             "task.a",
			Title:          "A",
			TrackingStatus: "ready",
		}, {
			ID:             "task.b",
			Title:          "B",
			TrackingStatus: "ready",
		}},
		Dependencies: []domain.Dependency{{
			ID:             "dependency.one",
			Type:           "finish_to_start",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"},
			SuccessorRef:   domain.Ref{Kind: "task", ID: "task.b"},
			Status:         "open",
			Criticality:    "required",
		}, {
			ID:             "dependency.two",
			Type:           "finish_to_start",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"},
			SuccessorRef:   domain.Ref{Kind: "task", ID: "task.b"},
			Status:         "open",
			Criticality:    "required",
		}},
	}})

	result, err := service.Validate(context.Background(), app.ValidateRequest{})

	if !errors.Is(err, app.ErrValidationFailed) {
		t.Fatalf("Validate error = %v, want ErrValidationFailed", err)
	}
	if !hasDiagnostic(result.Errors, "duplicate_dependency_triple") {
		t.Fatalf("expected duplicate_dependency_triple error, got %#v", result.Errors)
	}
}

func TestValidateRelatesIsNonSchedulingAndRejectsReversedDuplicate(t *testing.T) {
	workspace := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", DefaultStatusDate: "2026-07-10"},
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "accepted", PlannedStart: "2026-07-10", PlannedFinish: "2026-07-12"},
			{ID: "task.b", Title: "B", TrackingStatus: "in_progress", PlannedStart: "2026-07-09", PlannedFinish: "2026-07-11"},
		},
		Dependencies: []domain.Dependency{{
			ID: "dependency.related", Type: "relates", Status: "open", Criticality: "required",
			PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"},
		}},
	}
	result, err := app.NewValidateService(stubLoader{ws: workspace}).Validate(context.Background(), app.ValidateRequest{})
	if err != nil {
		t.Fatalf("Validate returned error: %v; errors=%#v", err, result.Errors)
	}
	for _, code := range []string{"required_dependency_unresolved", "dependency_successor_starts_before_predecessor_finish", "dependency_blocks_active_successor"} {
		if hasDiagnostic(result.Warnings, code) {
			t.Fatalf("relates emitted %s: %#v", code, result.Warnings)
		}
	}

	workspace.Dependencies = append(workspace.Dependencies, domain.Dependency{
		ID: "dependency.related.reversed", Type: "relates", Status: "open", Criticality: "optional",
		PredecessorRef: domain.Ref{Kind: "task", ID: "task.b"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.a"},
	})
	result, err = app.NewValidateService(stubLoader{ws: workspace}).Validate(context.Background(), app.ValidateRequest{})
	if !errors.Is(err, app.ErrValidationFailed) || !hasDiagnostic(result.Errors, "duplicate_dependency_triple") {
		t.Fatalf("reversed relates duplicate was accepted: err=%v errors=%#v", err, result.Errors)
	}
}

func TestValidateWarnsWhenTaskPairHasSeveralDependencyLinks(t *testing.T) {
	workspace := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test"},
		Tasks:   []domain.Task{{ID: "task.a", Title: "A", TrackingStatus: "ready"}, {ID: "task.b", Title: "B", TrackingStatus: "ready"}},
		Dependencies: []domain.Dependency{
			{ID: "dependency.blocks", Type: "blocks", Status: "open", Criticality: "optional", PredecessorRef: domain.Ref{Kind: "task", ID: "task.a"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.b"}},
			{ID: "dependency.related", Type: "relates", Status: "open", Criticality: "optional", PredecessorRef: domain.Ref{Kind: "task", ID: "task.b"}, SuccessorRef: domain.Ref{Kind: "task", ID: "task.a"}},
		},
	}
	result, err := app.NewValidateService(stubLoader{ws: workspace}).Validate(context.Background(), app.ValidateRequest{})
	if err != nil {
		t.Fatalf("Validate returned error: %v; errors=%#v", err, result.Errors)
	}
	if !hasDiagnostic(result.Warnings, "dependency_pair_has_several_links") {
		t.Fatalf("expected dependency_pair_has_several_links warning, got %#v", result.Warnings)
	}
}

// GAP-028: an accepted task whose planned finish is after the status date is
// flagged, in validation and as an agent next action; open tasks and tasks
// already inside the status date are not.
func TestAcceptedTaskPlannedAfterStatusDate(t *testing.T) {
	task := func(id, status, finish string) domain.Task {
		return domain.Task{ID: id, Title: id, Type: "implementation", TrackingStatus: status, PlannedStart: "2026-08-01", PlannedFinish: finish}
	}
	ws := domain.Workspace{
		Project: domain.Project{ID: "project.test", Name: "Test", Timezone: "UTC", DefaultStatusDate: "2026-09-25"},
		Tasks: []domain.Task{
			task("t.stale", "accepted", "2026-10-07"),
			task("t.done", "accepted", "2026-09-25"),
			task("t.open", "in_progress", "2026-10-07"),
		},
	}
	loader := stubLoader{ws: ws}

	result, _ := app.NewValidateService(loader).Validate(context.Background(), app.ValidateRequest{ProjectPath: "stub"})
	flagged := []string{}
	for _, warning := range result.Warnings {
		if warning.Code == "accepted_task_planned_after_status_date" {
			flagged = append(flagged, warning.Target.ID)
		}
	}
	if len(flagged) != 1 || flagged[0] != "t.stale" {
		t.Fatalf("flagged %v, want only t.stale", flagged)
	}

	overview, err := app.NewReportService(loader).Dashboard(context.Background(), app.DashboardReportRequest{ProjectPath: "stub"})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	found := false
	for _, action := range overview.NextActions {
		if action.Code == "record_actual_window" {
			if action.Target != "t.stale" || action.Actor != "agent" || action.Date != "2026-10-07" || !strings.Contains(action.Label, "replan task record-actual-window") || !strings.Contains(action.Command, "replan task record-actual-window") {
				t.Fatalf("unexpected record_actual_window action: %#v", action)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("no record_actual_window next action in %#v", overview.NextActions)
	}

	ws.Project.DefaultStatusDate = ""
	result, _ = app.NewValidateService(stubLoader{ws: ws}).Validate(context.Background(), app.ValidateRequest{ProjectPath: "stub"})
	for _, warning := range result.Warnings {
		if warning.Code == "accepted_task_planned_after_status_date" {
			t.Fatalf("flagged without a status date: %#v", warning)
		}
	}
}
