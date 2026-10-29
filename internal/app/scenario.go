package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type ScenarioService struct {
	loader WorkspaceLoader
}

type ScenarioListRequest struct{}

type ScenarioShowRequest struct {
	ID string
}

type ScenarioDiffRequest struct {
	ID      string
	Against string
	Week    string
}

type ScenarioApplyRequest struct {
	ID      string
	ActorID string
	Reason  string
	// EvidenceID names the evidence of the decision the scenario records; a
	// scenario that cancels a dependency requires it.
	EvidenceID string
	DryRun     bool
	Now        time.Time
}

type ScenarioListResult struct {
	Scenarios []ScenarioSummary `json:"scenarios"`
	Warnings  []Diagnostic      `json:"warnings"`
	Errors    []Diagnostic      `json:"errors"`
}

type ScenarioSummary struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	BaseKind  string `json:"base_kind,omitempty"`
	BaseID    string `json:"base_id,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
	CreatedBy string `json:"created_by,omitempty"`
	Changes   int    `json:"changes"`
}

type ScenarioShowResult struct {
	Scenario domain.Scenario `json:"scenario"`
	Warnings []Diagnostic    `json:"warnings"`
	Errors   []Diagnostic    `json:"errors"`
}

type ScenarioDiffResult struct {
	ScenarioID        string                  `json:"scenario_id"`
	ScenarioName      string                  `json:"scenario_name"`
	Against           string                  `json:"against"`
	Week              string                  `json:"week,omitempty"`
	Changes           []domain.ScenarioChange `json:"changes"`
	AffectedResources []string                `json:"affected_resources"`
	WorkloadBefore    []ResourceWorkload      `json:"workload_before"`
	WorkloadAfter     []ResourceWorkload      `json:"workload_after"`
	Warnings          []Diagnostic            `json:"warnings"`
	Errors            []Diagnostic            `json:"errors"`
}

type ScenarioApplyResult struct {
	DryRun             bool                  `json:"dry_run"`
	Applied            bool                  `json:"applied"`
	ScenarioID         string                `json:"scenario_id"`
	ScenarioName       string                `json:"scenario_name"`
	PreviousStatus     string                `json:"previous_status,omitempty"`
	NewStatus          string                `json:"new_status,omitempty"`
	Changes            []ScenarioApplyChange `json:"changes"`
	PlannedWrites      []PlannedWrite        `json:"planned_writes"`
	WrittenFiles       []string              `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord    `json:"audit_record_preview"`
	Validation         *ValidateResult       `json:"validation,omitempty"`
	Warnings           []Diagnostic          `json:"warnings"`
	Errors             []Diagnostic          `json:"errors"`
}

type ScenarioApplyChange struct {
	Op            string `json:"op"`
	Path          string `json:"path"`
	TargetKind    string `json:"target_kind"`
	TargetID      string `json:"target_id"`
	Field         string `json:"field"`
	PreviousValue any    `json:"previous_value"`
	NewValue      any    `json:"new_value"`
	Description   string `json:"description"`
}

type ScenarioWriter interface {
	ApplyScenarioChanges(scenarioID string, changes []ScenarioApplyChange, audit domain.AuditRecord) ([]string, error)
}

func NewScenarioService(loader WorkspaceLoader) ScenarioService {
	return ScenarioService{loader: loader}
}

func (s ScenarioService) List(ctx context.Context, _ ScenarioListRequest) (ScenarioListResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ScenarioListResult{
		Scenarios: []ScenarioSummary{},
		Warnings:  []Diagnostic{},
		Errors:    []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	for _, scenario := range ws.Scenarios {
		result.Scenarios = append(result.Scenarios, scenarioSummary(scenario))
	}
	sort.Slice(result.Scenarios, func(i, j int) bool {
		return result.Scenarios[i].ID < result.Scenarios[j].ID
	})
	return result, nil
}

func (s ScenarioService) Show(ctx context.Context, req ScenarioShowRequest) (ScenarioShowResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ScenarioShowResult{
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	scenario, ok := findScenario(ws.Scenarios, req.ID)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "scenario_not_found",
			Message:  fmt.Sprintf("scenario %s not found", req.ID),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: req.ID},
		})
		return result, ErrValidationFailed
	}
	result.Scenario = scenario
	return result, nil
}

func (s ScenarioService) Diff(ctx context.Context, req ScenarioDiffRequest) (ScenarioDiffResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ScenarioDiffResult{
		ScenarioID:        req.ID,
		Against:           req.Against,
		Week:              req.Week,
		Changes:           []domain.ScenarioChange{},
		AffectedResources: []string{},
		WorkloadBefore:    []ResourceWorkload{},
		WorkloadAfter:     []ResourceWorkload{},
		Warnings:          []Diagnostic{},
		Errors:            []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	scenario, ok := findScenario(ws.Scenarios, req.ID)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "scenario_not_found",
			Message:  fmt.Sprintf("scenario %s not found", req.ID),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: req.ID},
		})
		return result, ErrValidationFailed
	}

	workload, err := NewWorkloadService(s.loader).Calculate(ctx, WorkloadRequest{Week: req.Week})
	result.ScenarioName = scenario.Name
	result.Changes = scenario.Changes
	result.WorkloadBefore = workload.Resources
	result.Warnings = append(result.Warnings, workload.Warnings...)
	result.Errors = append(result.Errors, workload.Errors...)
	if err != nil {
		return result, err
	}

	overlay := ApplyScenarioOverlay(ws, scenario.ID)
	result.Warnings = append(result.Warnings, overlay.Warnings...)
	result.Errors = append(result.Errors, overlay.Errors...)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	after, err := NewWorkloadService(workspaceInMemory{ws: overlay.Workspace, projectPath: s.loader.ProjectPath()}).Calculate(ctx, WorkloadRequest{Week: req.Week})
	result.WorkloadAfter = after.Resources
	// Keep the established diff meaning for unavailable-resource assumptions:
	// the selected resource has no capacity in the compared window.
	for resourceID := range scenarioUnavailableResources(scenario) {
		applyUnavailableResource(result.WorkloadAfter, resourceID)
	}
	result.Warnings = append(result.Warnings, after.Warnings...)
	result.Errors = append(result.Errors, after.Errors...)
	if err != nil {
		return result, err
	}

	affected := scenarioAffectedResources(ws, scenario)
	for resourceID := range affected {
		result.AffectedResources = append(result.AffectedResources, resourceID)
	}
	sort.Strings(result.AffectedResources)
	return result, nil
}

func (s ScenarioService) Apply(ctx context.Context, req ScenarioApplyRequest) (ScenarioApplyResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ScenarioApplyResult{
		DryRun:        req.DryRun,
		ScenarioID:    req.ID,
		NewStatus:     "applied",
		Changes:       []ScenarioApplyChange{},
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	scenario, ok := findScenario(ws.Scenarios, req.ID)
	if strings.TrimSpace(req.ID) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "scenario_id_required",
			Message:  "scenario apply requires scenario id",
			Severity: "error",
			Target:   Target{Kind: "scenario"},
		})
	} else if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "scenario_not_found",
			Message:  fmt.Sprintf("scenario %s not found", req.ID),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: req.ID},
		})
	}
	if !req.DryRun && strings.TrimSpace(req.ActorID) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actor_required",
			Message:  "scenario apply requires --actor unless --dry-run is used",
			Severity: "error",
			Target:   Target{Kind: "actor"},
		})
	}
	if req.ActorID != "" && !hasActor(ws.Actors, req.ActorID) {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actor_not_found",
			Message:  fmt.Sprintf("actor %s not found", req.ActorID),
			Severity: "error",
			Target:   Target{Kind: "actor", ID: req.ActorID},
		})
	}
	if req.EvidenceID != "" && !hasEvidence(ws.Evidence, req.EvidenceID) {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "evidence_not_found",
			Message:  fmt.Sprintf("evidence %s not found", req.EvidenceID),
			Severity: "error",
			Target:   Target{Kind: "evidence", ID: req.EvidenceID},
		})
	}
	if ok && scenario.Status == "applied" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "scenario_already_applied",
			Message:  fmt.Sprintf("scenario %s is already applied", scenario.ID),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		})
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	result.ScenarioName = scenario.Name
	result.PreviousStatus = scenario.Status
	result.Changes = buildScenarioApplyChanges(ws, scenario, &result.Errors)
	result.Errors = append(result.Errors, scenarioTaskDateRangeDiagnostics(ws, result.Changes)...)
	if scenarioApplyTouches(result.Changes, "dependency") && req.EvidenceID == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "evidence_required",
			Message:  fmt.Sprintf("scenario %s cancels a dependency; name the evidence of that decision with --evidence", scenario.ID),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		})
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	if len(result.Changes) == 0 {
		result.Warnings = append(result.Warnings, Diagnostic{
			Code:     "scenario_apply_no_changes",
			Message:  fmt.Sprintf("scenario %s has no applyable changes", scenario.ID),
			Severity: "warning",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		})
	}
	audit := scenarioApplyAuditRecord(req, scenario, len(result.Changes))
	result.AuditRecordPreview = audit
	result.PlannedWrites = scenarioApplyPlannedWrites(scenario, result.Changes, audit)
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(ScenarioWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support scenario writes",
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
		})
		return result, ErrValidationFailed
	}
	writtenFiles, err := writer.ApplyScenarioChanges(scenario.ID, result.Changes, audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "scenario_apply_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		})
		return result, err
	}
	result.WrittenFiles = writtenFiles
	validation, written, err := session.finish(ctx, result.WrittenFiles)
	result.Validation = &validation
	result.WrittenFiles = written
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

func scenarioSummary(scenario domain.Scenario) ScenarioSummary {
	return ScenarioSummary{
		ID:        scenario.ID,
		Name:      scenario.Name,
		Status:    scenario.Status,
		BaseKind:  scenario.BaseRef.Kind,
		BaseID:    scenario.BaseRef.ID,
		CreatedAt: scenario.CreatedAt,
		CreatedBy: scenario.CreatedBy.ID,
		Changes:   len(scenario.Changes),
	}
}

func buildScenarioApplyChanges(ws domain.Workspace, scenario domain.Scenario, errorsOut *[]Diagnostic) []ScenarioApplyChange {
	var changes []ScenarioApplyChange
	for _, change := range scenario.Changes {
		applyChange, ok := scenarioApplyDemandAmountChange(ws, scenario, change)
		if ok {
			changes = append(changes, applyChange)
			continue
		}
		applyChange, ok = scenarioApplyTaskDateChange(ws, scenario, change)
		if ok {
			changes = append(changes, applyChange)
			continue
		}
		applyChange, ok = scenarioApplyDependencyCancel(ws, change)
		if ok {
			changes = append(changes, applyChange)
			continue
		}
		*errorsOut = append(*errorsOut, unsupportedScenarioApplyChange(scenario, change))
	}
	return changes
}

func scenarioApplyTaskDateChange(ws domain.Workspace, scenario domain.Scenario, change domain.ScenarioChange) (ScenarioApplyChange, bool) {
	if !isSetOperation(change.Op) || !strings.HasPrefix(change.Path, "tasks.") {
		return ScenarioApplyChange{}, false
	}
	field := ""
	switch {
	case strings.HasSuffix(change.Path, ".planned_start"):
		field = "planned_start"
	case strings.HasSuffix(change.Path, ".planned_finish"):
		field = "planned_finish"
	default:
		return ScenarioApplyChange{}, false
	}
	taskID := strings.TrimSuffix(strings.TrimPrefix(change.Path, "tasks."), "."+field)
	if taskID == "" {
		return ScenarioApplyChange{}, false
	}
	value, ok := change.Value.(string)
	if !ok || !validDate(value) {
		return ScenarioApplyChange{}, false
	}
	for _, task := range ws.Tasks {
		if task.ID != taskID {
			continue
		}
		previous := task.PlannedStart
		if field == "planned_finish" {
			previous = task.PlannedFinish
		}
		return ScenarioApplyChange{
			Op:            change.Op,
			Path:          change.Path,
			TargetKind:    "task",
			TargetID:      taskID,
			Field:         field,
			PreviousValue: previous,
			NewValue:      value,
			Description:   fmt.Sprintf("Set %s %s from %q to %q.", taskID, field, previous, value),
		}, true
	}
	return ScenarioApplyChange{}, false
}

func scenarioTaskDateRangeDiagnostics(ws domain.Workspace, changes []ScenarioApplyChange) []Diagnostic {
	changed := map[string]bool{}
	cloned := cloneWorkspace(ws)
	for _, change := range changes {
		if change.TargetKind != "task" {
			continue
		}
		for i := range cloned.Tasks {
			if cloned.Tasks[i].ID != change.TargetID {
				continue
			}
			value, _ := change.NewValue.(string)
			switch change.Field {
			case "planned_start":
				cloned.Tasks[i].PlannedStart = value
			case "planned_finish":
				cloned.Tasks[i].PlannedFinish = value
			}
			changed[change.TargetID] = true
		}
	}
	var diagnostics []Diagnostic
	for _, task := range cloned.Tasks {
		if !changed[task.ID] {
			continue
		}
		before := len(diagnostics)
		validateTaskDates(&diagnostics, task.PlannedStart, task.PlannedFinish)
		for i := before; i < len(diagnostics); i++ {
			diagnostics[i].Target = Target{Kind: "task", ID: task.ID}
		}
	}
	return diagnostics
}

// scenarioApplyDependencyCancel accepts dependencies.<id>.status set to
// cancelled: the operator let the successor go ahead (GAP-032). Other
// dependency changes stay with proposals.
func scenarioApplyDependencyCancel(ws domain.Workspace, change domain.ScenarioChange) (ScenarioApplyChange, bool) {
	if !isSetOperation(change.Op) || !strings.HasPrefix(change.Path, "dependencies.") || !strings.HasSuffix(change.Path, ".status") {
		return ScenarioApplyChange{}, false
	}
	dependencyID := strings.TrimSuffix(strings.TrimPrefix(change.Path, "dependencies."), ".status")
	value, ok := change.Value.(string)
	if dependencyID == "" || !ok || value != "cancelled" {
		return ScenarioApplyChange{}, false
	}
	for _, dependency := range ws.Dependencies {
		if dependency.ID != dependencyID || dependency.Status == "cancelled" {
			continue
		}
		return ScenarioApplyChange{
			Op:            change.Op,
			Path:          change.Path,
			TargetKind:    "dependency",
			TargetID:      dependencyID,
			Field:         "status",
			PreviousValue: dependency.Status,
			NewValue:      value,
			Description:   fmt.Sprintf("Cancel dependency %s, now %s.", dependencyID, dependency.Status),
		}, true
	}
	return ScenarioApplyChange{}, false
}

func scenarioApplyDemandAmountChange(ws domain.Workspace, scenario domain.Scenario, change domain.ScenarioChange) (ScenarioApplyChange, bool) {
	if !isSetOperation(change.Op) || !strings.HasPrefix(change.Path, "resource_demands.") {
		return ScenarioApplyChange{}, false
	}
	field := ""
	switch {
	case strings.HasSuffix(change.Path, ".remaining_amount"):
		field = "remaining_amount"
	case strings.HasSuffix(change.Path, ".planned_amount"):
		field = "planned_amount"
	default:
		return ScenarioApplyChange{}, false
	}
	demandID := strings.TrimSuffix(strings.TrimPrefix(change.Path, "resource_demands."), "."+field)
	if demandID == "" {
		return ScenarioApplyChange{}, false
	}
	amount, ok := scenarioNumericAmount(change.Value)
	if !ok || amount < 0 {
		return ScenarioApplyChange{}, false
	}
	for _, demand := range ws.ResourceDemands {
		if demand.ID != demandID {
			continue
		}
		previous := demand.Remaining()
		if field == "planned_amount" {
			previous = demand.PlannedAmount
		}
		return ScenarioApplyChange{
			Op:            change.Op,
			Path:          change.Path,
			TargetKind:    "resource_demand",
			TargetID:      demandID,
			Field:         field,
			PreviousValue: previous,
			NewValue:      amount,
			Description:   fmt.Sprintf("Set %s %s from %g to %g.", demandID, field, previous, amount),
		}, true
	}
	return ScenarioApplyChange{}, false
}

func scenarioApplyAuditRecord(req ScenarioApplyRequest, scenario domain.Scenario, changeCount int) domain.AuditRecord {
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	reason := req.Reason
	if reason == "" {
		reason = fmt.Sprintf("Applied scenario %s with %d supported changes.", scenario.ID, changeCount)
	}
	result := "success"
	if req.DryRun {
		result = "dry_run"
	}
	record := domain.AuditRecord{
		ID:         "audit." + compactTimestamp(now.Format(time.RFC3339)) + ".scenario-apply." + slugID(scenario.ID),
		OccurredAt: now.Format(time.RFC3339),
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:  "scenario_apply",
		TargetRef:  domain.Ref{Kind: "scenario", ID: scenario.ID},
		Result:     result,
		Reason:     reason,
	}
	if req.EvidenceID != "" {
		record.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}
	return record
}

func scenarioApplyPlannedWrites(scenario domain.Scenario, changes []ScenarioApplyChange, audit domain.AuditRecord) []PlannedWrite {
	appliedScenario := scenario
	appliedScenario.Status = "applied"
	scenarioContent, _ := yaml.Marshal(struct {
		Scenario domain.Scenario `yaml:"scenario"`
	}{Scenario: appliedScenario})
	writes := []PlannedWrite{}
	if scenarioApplyTouches(changes, "resource_demand") {
		writes = append(writes, PlannedWrite{
			Path:   "canonical/resource_demands.yaml",
			Action: "update",
			Format: "yaml",
		})
	}
	if scenarioApplyTouches(changes, "task") {
		writes = append(writes, PlannedWrite{
			Path:   "canonical/tasks.yaml",
			Action: "update",
			Format: "yaml",
		})
	}
	if scenarioApplyTouches(changes, "dependency") {
		writes = append(writes, PlannedWrite{
			Path:   "canonical/dependencies.yaml",
			Action: "update",
			Format: "yaml",
		})
	}
	writes = append(writes, PlannedWrite{
		Path:    "scenarios/" + scenario.ID + ".yaml",
		Action:  "update",
		Format:  "yaml",
		Content: string(scenarioContent),
	})
	writes = append(writes, PlannedWrite{
		Path:   AuditShardPath(audit.OccurredAt),
		Action: "append_jsonl_record",
		Format: "jsonl",
	})
	return writes
}

func scenarioApplyTouches(changes []ScenarioApplyChange, targetKind string) bool {
	for _, change := range changes {
		if change.TargetKind == targetKind {
			return true
		}
	}
	return false
}

func findScenario(scenarios []domain.Scenario, id string) (domain.Scenario, bool) {
	for _, scenario := range scenarios {
		if scenario.ID == id {
			return scenario, true
		}
	}
	return domain.Scenario{}, false
}

func scenarioAffectedResources(ws domain.Workspace, scenario domain.Scenario) map[string]bool {
	affected := map[string]bool{}
	demandsByTask := map[string][]domain.ResourceDemand{}
	for _, demand := range ws.ResourceDemands {
		demandsByTask[demand.TaskRef.ID] = append(demandsByTask[demand.TaskRef.ID], demand)
	}
	for _, change := range scenario.Changes {
		if resourceID, _, ok := scenarioAvailabilityException(change.Value); ok {
			affected[resourceID] = true
		}
		if ref, ok := scenarioResourceRef(change.Value); ok {
			affected[ref.ID] = true
		}
		if strings.HasPrefix(change.Path, "resource_demands.") {
			for _, demand := range ws.ResourceDemands {
				prefix := "resource_demands." + demand.ID + "."
				if strings.HasPrefix(change.Path, prefix) {
					if resourceID := demandTargetIDOrEmpty(demand); resourceID != "" {
						affected[resourceID] = true
					}
				}
			}
		}
		if strings.HasPrefix(change.Path, "capacities.") {
			for _, capacity := range ws.Capacities {
				if strings.HasPrefix(change.Path, "capacities."+capacity.ID+".") {
					affected[capacity.ResourceRef.ID] = true
				}
			}
		}
		if applyChange, ok := scenarioApplyTaskDateChange(ws, scenario, change); ok {
			for _, demand := range demandsByTask[applyChange.TargetID] {
				if resourceID := demandTargetIDOrEmpty(demand); resourceID != "" {
					affected[resourceID] = true
				}
			}
		}
	}
	return affected
}

func scenarioUnavailableResources(scenario domain.Scenario) map[string]bool {
	resources := map[string]bool{}
	for _, change := range scenario.Changes {
		if change.Op != "add" || change.Path != "availability.exceptions" {
			continue
		}
		if resourceID, exception, ok := scenarioAvailabilityException(change.Value); ok && exception.Type == "unavailable" {
			resources[resourceID] = true
		}
	}
	return resources
}

func applyUnavailableResource(rows []ResourceWorkload, resourceID string) {
	for i := range rows {
		if rows[i].ResourceID != resourceID {
			continue
		}
		rows[i].Capacity = 0
		load := rows[i].Reserved
		if load == 0 {
			load = rows[i].Remaining
		}
		rows[i].Overload = load
	}
}

func demandTargetIDOrEmpty(demand domain.ResourceDemand) string {
	if id, ok := demandTargetID(demand); ok {
		return id
	}
	return ""
}

func appendLoadDiagnostics(warnings, errors *[]Diagnostic, diags []Diagnostic) {
	for _, d := range diags {
		if d.Severity == "warning" {
			*warnings = append(*warnings, d)
			continue
		}
		*errors = append(*errors, d)
	}
}
