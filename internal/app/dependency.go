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

type DependencyWriter interface {
	AddDependencies([]domain.Dependency) (string, error)
	AppendAudit(domain.AuditRecord) (string, error)
}

type DependencyAddItem struct {
	ID, Type, PredecessorID, SuccessorID  string
	Criticality, Status, Description      string
	RequiredState, ExpectedResolutionDate string
	External                              bool
}

type DependencyAddRequest struct {
	Items                       []DependencyAddItem
	Reason, ActorID, EvidenceID string
	OccurredAt                  string
	DryRun                      bool
}

type DependencyAddResult struct {
	DryRun              bool                 `json:"dry_run"`
	Applied             bool                 `json:"applied"`
	Dependencies        []domain.Dependency  `json:"dependencies"`
	PlannedWrites       []PlannedWrite       `json:"planned_writes"`
	WrittenFiles        []string             `json:"written_files,omitempty"`
	AuditRecordPreviews []domain.AuditRecord `json:"audit_record_previews"`
	Validation          *ValidateResult      `json:"validation,omitempty"`
	Warnings            []Diagnostic         `json:"warnings"`
	Errors              []Diagnostic         `json:"errors"`
}

type DependencyService struct {
	loader WorkspaceLoader
}

type DependencyListRequest struct {
	ProjectPath string
}

type DependencyListResult struct {
	Dependencies      []DependencyRow       `json:"dependencies"`
	BlockedSuccessors []BlockedSuccessorRow `json:"blocked_successors"`
	Summary           DependencySummary     `json:"summary"`
	Warnings          []Diagnostic          `json:"warnings"`
	Errors            []Diagnostic          `json:"errors"`
}

type DependencySummary struct {
	Dependencies        int `json:"dependencies"`
	RequiredOpen        int `json:"required_open"`
	ActiveBlockers      int `json:"active_blockers"`
	Resolvable          int `json:"resolvable"`
	ReadyToUnblockTasks int `json:"ready_to_unblock_tasks"`
	OpenProposals       int `json:"open_proposals"`
}

type DependencyRow struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"`
	Status          string   `json:"status"`
	Criticality     string   `json:"criticality"`
	PredecessorKind string   `json:"predecessor_kind,omitempty"`
	PredecessorID   string   `json:"predecessor_id,omitempty"`
	SuccessorKind   string   `json:"successor_kind"`
	SuccessorID     string   `json:"successor_id"`
	SuccessorStatus string   `json:"successor_status,omitempty"`
	Blocking        bool     `json:"blocking"`
	Resolvable      bool     `json:"resolvable"`
	OpenProposalIDs []string `json:"open_proposal_ids"`
}

type BlockedSuccessorRow struct {
	TaskID                string   `json:"task_id"`
	TaskTitle             string   `json:"task_title,omitempty"`
	RequiredDependencyIDs []string `json:"required_dependency_ids"`
	OpenDependencyIDs     []string `json:"open_dependency_ids"`
	ReadyToUnblock        bool     `json:"ready_to_unblock"`
	OpenProposalIDs       []string `json:"open_proposal_ids"`
}

func NewDependencyService(loader WorkspaceLoader) DependencyService {
	return DependencyService{loader: loader}
}

func (s DependencyService) Add(ctx context.Context, req DependencyAddRequest) (DependencyAddResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := DependencyAddResult{DryRun: req.DryRun, Dependencies: []domain.Dependency{}, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, AuditRecordPreviews: []domain.AuditRecord{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	validateConstraintAddCommon(&result.Errors, ws, len(req.Items), "dependency", req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt)
	seenIDs := map[string]bool{}
	seenTriples := map[string]bool{}
	for _, existing := range ws.Dependencies {
		seenIDs[existing.ID] = true
		if existing.PredecessorRef.ID != "" {
			seenTriples[dependencyTripleKey(existing.Type, existing.PredecessorRef, existing.SuccessorRef)] = true
		}
	}
	for _, item := range req.Items {
		dependency := dependencyFromAddItem(item)
		validateDependencyAddItem(&result.Errors, ws, dependency, item.External, seenIDs, seenTriples)
		if dependency.ID != "" {
			seenIDs[dependency.ID] = true
		}
		if dependency.PredecessorRef.ID != "" {
			seenTriples[dependencyTripleKey(dependency.Type, dependency.PredecessorRef, dependency.SuccessorRef)] = true
		}
		result.Dependencies = append(result.Dependencies, dependency)
	}
	if len(result.Errors) == 0 && finishToStartCycle(append(append([]domain.Dependency{}, ws.Dependencies...), result.Dependencies...)) {
		result.Errors = append(result.Errors, dependencyAddDiag("dependency_cycle", "finish_to_start dependencies would contain a cycle", "dependency", ""))
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	occurredAt := req.OccurredAt
	if occurredAt == "" {
		occurredAt = time.Now().Format(time.RFC3339)
	}
	for _, dependency := range result.Dependencies {
		audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".dependency-add." + slugID(dependency.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "dependency_add", TargetRef: domain.Ref{Kind: "dependency", ID: dependency.ID}, Result: "success", Reason: req.Reason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}}
		if req.DryRun {
			audit.Result = "dry_run"
		}
		result.AuditRecordPreviews = append(result.AuditRecordPreviews, audit)
	}
	content, _ := yaml.Marshal(struct {
		Dependencies []domain.Dependency `yaml:"dependencies"`
	}{result.Dependencies})
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{Path: "canonical/dependencies.yaml", Action: "append_yaml_items", Format: "yaml", Content: string(content)})
	for _, audit := range result.AuditRecordPreviews {
		result.PlannedWrites = append(result.PlannedWrites, auditPlannedWrite(audit))
	}
	if req.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, ok := session.target.(DependencyWriter)
	if !ok {
		result.Errors = append(result.Errors, dependencyAddDiag("storage_write_not_supported", "configured project store does not support dependency writes", "project", s.loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	path, err := writer.AddDependencies(result.Dependencies)
	if err != nil {
		result.Errors = append(result.Errors, dependencyAddDiag("dependency_write_failed", err.Error(), "dependency", ""))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, path)
	for _, audit := range result.AuditRecordPreviews {
		path, err = writer.AppendAudit(audit)
		if err != nil {
			result.Errors = append(result.Errors, dependencyAddDiag("audit_write_failed", err.Error(), "audit_record", audit.ID))
			return result, err
		}
		result.WrittenFiles = append(result.WrittenFiles, path)
	}
	validation, committed, err := session.finish(ctx, result.WrittenFiles)
	result.Validation, result.WrittenFiles = &validation, committed
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

func dependencyFromAddItem(item DependencyAddItem) domain.Dependency {
	dependencyType := item.Type
	if dependencyType == "" {
		dependencyType = "finish_to_start"
	}
	criticality := item.Criticality
	if criticality == "" {
		criticality = "required"
		if dependencyType == "relates" {
			criticality = "optional"
		}
	}
	status := item.Status
	if status == "" {
		status = "open"
	}
	id := item.ID
	if id == "" && item.SuccessorID != "" {
		left := item.PredecessorID
		if item.External {
			left = "external"
		}
		id = "dependency." + slugID(dependencyType) + "." + slugID(left) + "." + slugID(item.SuccessorID)
	}
	dependency := domain.Dependency{ID: id, Type: dependencyType, SuccessorRef: domain.Ref{Kind: "task", ID: item.SuccessorID}, Description: item.Description, RequiredState: item.RequiredState, Status: status, Criticality: criticality, ExpectedResolutionDate: item.ExpectedResolutionDate}
	if !item.External {
		dependency.PredecessorRef = domain.Ref{Kind: "task", ID: item.PredecessorID}
	}
	return dependency
}

func validateConstraintAddCommon(errors *[]Diagnostic, ws domain.Workspace, itemCount int, kind, reason, actorID, evidenceID, occurredAt string) {
	if itemCount == 0 {
		*errors = append(*errors, dependencyAddDiag("missing_"+kind, "at least one "+kind+" is required", kind, ""))
	}
	if strings.TrimSpace(reason) == "" {
		*errors = append(*errors, dependencyAddDiag("missing_reason", "reason is required", kind, ""))
	}
	if actorID == "" || !hasActor(ws.Actors, actorID) {
		code := "actor_not_found"
		if actorID == "" {
			code = "missing_actor"
		}
		*errors = append(*errors, dependencyAddDiag(code, "actor must name an existing actor", "actor", actorID))
	}
	if evidenceID == "" || !hasEvidence(ws.Evidence, evidenceID) {
		code := "evidence_not_found"
		if evidenceID == "" {
			code = "missing_evidence"
		}
		*errors = append(*errors, dependencyAddDiag(code, "evidence must name existing evidence", "evidence", evidenceID))
	}
	if occurredAt != "" {
		if _, err := time.Parse(time.RFC3339, occurredAt); err != nil {
			*errors = append(*errors, dependencyAddDiag("invalid_occurred_at", "occurred-at must be RFC3339 with an offset", kind, ""))
		}
	}
}

func validateDependencyAddItem(errors *[]Diagnostic, ws domain.Workspace, dependency domain.Dependency, external bool, seenIDs, seenTriples map[string]bool) {
	if dependency.ID == "" {
		*errors = append(*errors, dependencyAddDiag("missing_dependency_id", "dependency id could not be derived", "dependency", ""))
	} else if seenIDs[dependency.ID] {
		*errors = append(*errors, dependencyAddDiag("duplicate_dependency_id", fmt.Sprintf("dependency id %s already exists", dependency.ID), "dependency", dependency.ID))
	}
	if !domain.ValidDependencyType(dependency.Type) {
		*errors = append(*errors, dependencyAddDiag("invalid_dependency_type", fmt.Sprintf("invalid dependency type %q", dependency.Type), "dependency", dependency.ID))
	}
	if !domain.ValidDependencyStatus(dependency.Status) {
		*errors = append(*errors, dependencyAddDiag("invalid_dependency_status", fmt.Sprintf("invalid dependency status %q", dependency.Status), "dependency", dependency.ID))
	}
	if !domain.ValidDependencyCriticality(dependency.Criticality) {
		*errors = append(*errors, dependencyAddDiag("invalid_dependency_criticality", fmt.Sprintf("invalid dependency criticality %q", dependency.Criticality), "dependency", dependency.ID))
	}
	if external != (dependency.Type == "external_dependency") {
		*errors = append(*errors, dependencyAddDiag("invalid_external_dependency", "--external and type external_dependency must be used together", "dependency", dependency.ID))
	}
	if external && strings.TrimSpace(dependency.Description) == "" {
		*errors = append(*errors, dependencyAddDiag("missing_external_description", "external dependency description is required", "dependency", dependency.ID))
	}
	if !external && !dependencyHasTask(ws.Tasks, dependency.PredecessorRef.ID) {
		*errors = append(*errors, dependencyAddDiag("predecessor_task_not_found", fmt.Sprintf("predecessor task %s not found", dependency.PredecessorRef.ID), "task", dependency.PredecessorRef.ID))
	}
	if !dependencyHasTask(ws.Tasks, dependency.SuccessorRef.ID) {
		*errors = append(*errors, dependencyAddDiag("successor_task_not_found", fmt.Sprintf("successor task %s not found", dependency.SuccessorRef.ID), "task", dependency.SuccessorRef.ID))
	}
	if !external && dependency.PredecessorRef.ID == dependency.SuccessorRef.ID && dependency.PredecessorRef.ID != "" {
		*errors = append(*errors, dependencyAddDiag("self_dependency", "predecessor and successor must differ", "dependency", dependency.ID))
	}
	if dependency.ExpectedResolutionDate != "" && !validDate(dependency.ExpectedResolutionDate) {
		*errors = append(*errors, dependencyAddDiag("invalid_dependency_expected_resolution_date", "expected-resolution-date must use YYYY-MM-DD", "dependency", dependency.ID))
	}
	if dependency.PredecessorRef.ID != "" && seenTriples[dependencyTripleKey(dependency.Type, dependency.PredecessorRef, dependency.SuccessorRef)] {
		*errors = append(*errors, dependencyAddDiag("duplicate_dependency", "dependency with the same type, predecessor and successor already exists", "dependency", dependency.ID))
	}
}

func finishToStartCycle(dependencies []domain.Dependency) bool {
	edges := map[string][]string{}
	for _, dependency := range dependencies {
		if dependency.Type == "finish_to_start" && dependency.PredecessorRef.Kind == "task" && dependency.SuccessorRef.Kind == "task" {
			edges[dependency.PredecessorRef.ID] = append(edges[dependency.PredecessorRef.ID], dependency.SuccessorRef.ID)
		}
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(node string) bool {
		if state[node] == 1 {
			return true
		}
		if state[node] == 2 {
			return false
		}
		state[node] = 1
		for _, next := range edges[node] {
			if visit(next) {
				return true
			}
		}
		state[node] = 2
		return false
	}
	for node := range edges {
		if visit(node) {
			return true
		}
	}
	return false
}

func dependencyAddDiag(code, message, kind, id string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}}
}

func dependencyHasTask(tasks []domain.Task, id string) bool {
	_, ok := findTask(tasks, id)
	return ok
}

func (s DependencyService) List(ctx context.Context, req DependencyListRequest) (DependencyListResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := DependencyListResult{
		Dependencies:      []DependencyRow{},
		BlockedSuccessors: []BlockedSuccessorRow{},
		Warnings:          []Diagnostic{},
		Errors:            []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	taskByID := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		taskByID[task.ID] = task
	}
	openProposalsByTarget := openProposalsByTargetID(ws.Proposals)
	requiredDependenciesBySuccessor := map[string][]domain.Dependency{}
	for _, dependency := range ws.Dependencies {
		if dependency.Type != "relates" && dependency.SuccessorRef.Kind == "task" && dependency.Criticality == "required" {
			requiredDependenciesBySuccessor[dependency.SuccessorRef.ID] = append(requiredDependenciesBySuccessor[dependency.SuccessorRef.ID], dependency)
		}
	}

	for _, dependency := range ws.Dependencies {
		row := DependencyRow{
			ID:              dependency.ID,
			Type:            dependency.Type,
			Status:          dependency.Status,
			Criticality:     dependency.Criticality,
			PredecessorKind: dependency.PredecessorRef.Kind,
			PredecessorID:   dependency.PredecessorRef.ID,
			SuccessorKind:   dependency.SuccessorRef.Kind,
			SuccessorID:     dependency.SuccessorRef.ID,
			Blocking:        dependencyBlocksSuccessor(dependency),
			Resolvable:      dependency.Type != "relates" && dependency.Criticality == "required" && dependency.Status != "resolved" && dependency.Status != "cancelled" && predecessorSatisfiesDependency(dependency, taskByID),
			OpenProposalIDs: stringSliceOrEmpty(openProposalsByTarget[dependency.ID]),
		}
		if dependency.SuccessorRef.Kind == "task" {
			if task, ok := taskByID[dependency.SuccessorRef.ID]; ok {
				row.SuccessorStatus = task.TrackingStatus
			}
		}
		result.Dependencies = append(result.Dependencies, row)
		if row.Type != "relates" && row.Criticality == "required" && dependencyActive(row.Status) {
			result.Summary.RequiredOpen++
		}
		if row.Blocking {
			result.Summary.ActiveBlockers++
		}
		if row.Resolvable {
			result.Summary.Resolvable++
		}
		result.Summary.OpenProposals += len(row.OpenProposalIDs)
	}

	for _, task := range ws.Tasks {
		if task.TrackingStatus != "blocked" {
			continue
		}
		dependencies := requiredDependenciesBySuccessor[task.ID]
		if len(dependencies) == 0 {
			continue
		}
		row := BlockedSuccessorRow{
			TaskID:                task.ID,
			TaskTitle:             task.Title,
			RequiredDependencyIDs: []string{},
			OpenDependencyIDs:     []string{},
			ReadyToUnblock:        allDependenciesResolvedOrCancelled(dependencies),
			OpenProposalIDs:       stringSliceOrEmpty(openProposalsByTarget[task.ID]),
		}
		for _, dependency := range dependencies {
			row.RequiredDependencyIDs = append(row.RequiredDependencyIDs, dependency.ID)
			if dependencyActive(dependency.Status) {
				row.OpenDependencyIDs = append(row.OpenDependencyIDs, dependency.ID)
			}
		}
		sort.Strings(row.RequiredDependencyIDs)
		sort.Strings(row.OpenDependencyIDs)
		result.BlockedSuccessors = append(result.BlockedSuccessors, row)
		if row.ReadyToUnblock {
			result.Summary.ReadyToUnblockTasks++
		}
		result.Summary.OpenProposals += len(row.OpenProposalIDs)
	}

	sort.Slice(result.Dependencies, func(i, j int) bool {
		return result.Dependencies[i].ID < result.Dependencies[j].ID
	})
	sort.Slice(result.BlockedSuccessors, func(i, j int) bool {
		return result.BlockedSuccessors[i].TaskID < result.BlockedSuccessors[j].TaskID
	})
	result.Summary.Dependencies = len(result.Dependencies)
	return result, nil
}

func stringSliceOrEmpty(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func dependencyBlocksSuccessor(dependency domain.Dependency) bool {
	if dependency.Type == "relates" || !dependencyActive(dependency.Status) {
		return false
	}
	return dependency.Criticality == "required" || dependency.Type == "blocks"
}

func openProposalsByTargetID(proposals []domain.Proposal) map[string][]string {
	out := map[string][]string{}
	for _, proposal := range proposals {
		if proposal.Status != "open" || proposal.TargetRef.ID == "" {
			continue
		}
		out[proposal.TargetRef.ID] = append(out[proposal.TargetRef.ID], proposal.ID)
	}
	for id := range out {
		sort.Strings(out[id])
	}
	return out
}
