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

type ProposalService struct {
	loader WorkspaceLoader
}

type ProposalListRequest struct {
	Status string
}

type ProposalShowRequest struct {
	ID string
}

type ProposalDecisionRequest struct {
	ID      string
	ActorID string
	Reason  string
	DryRun  bool
	Now     time.Time
}

type ProposalSuggestDependenciesRequest struct {
	ActorID         string
	WriteCandidates bool
	Now             time.Time
}

type ProposalListResult struct {
	Proposals []ProposalSummary `json:"proposals"`
	Warnings  []Diagnostic      `json:"warnings"`
	Errors    []Diagnostic      `json:"errors"`
}

type ProposalSummary struct {
	ID                   string `json:"id"`
	Type                 string `json:"type"`
	Status               string `json:"status"`
	CreatedAt            string `json:"created_at,omitempty"`
	CreatedBy            string `json:"created_by,omitempty"`
	TargetKind           string `json:"target_kind,omitempty"`
	TargetID             string `json:"target_id,omitempty"`
	Rationale            string `json:"rationale,omitempty"`
	RequiresConfirmation bool   `json:"requires_confirmation"`
}

type ProposalShowResult struct {
	Proposal domain.Proposal `json:"proposal"`
	Warnings []Diagnostic    `json:"warnings"`
	Errors   []Diagnostic    `json:"errors"`
}

type ProposalDecisionResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	SuggestedApplied   bool               `json:"suggested_applied"`
	Decision           string             `json:"decision"`
	Proposal           domain.Proposal    `json:"proposal"`
	PreviousStatus     string             `json:"previous_status,omitempty"`
	NewStatus          string             `json:"new_status"`
	Reason             string             `json:"reason,omitempty"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ProposalSuggestDependenciesResult struct {
	DryRun        bool              `json:"dry_run"`
	Applied       bool              `json:"applied"`
	Proposals     []domain.Proposal `json:"proposals"`
	PlannedWrites []PlannedWrite    `json:"planned_writes"`
	WrittenFiles  []string          `json:"written_files,omitempty"`
	Validation    *ValidateResult   `json:"validation,omitempty"`
	Warnings      []Diagnostic      `json:"warnings"`
	Errors        []Diagnostic      `json:"errors"`
}

type ProposalWriter interface {
	MoveProposal(proposalID string, targetStatus string, audit domain.AuditRecord) ([]string, error)
}

type ProposalTaskStatusChange struct {
	TaskID         string
	PreviousStatus string
	NewStatus      string
}

type ProposalDependencyStatusChange struct {
	DependencyID   string
	PreviousStatus string
	NewStatus      string
}

type ProposalResourceDemandChange struct {
	DemandID      string
	Field         string
	PreviousValue float64
	NewValue      float64
}

type ProposalApplyWriter interface {
	ApplyProposalDecision(proposalID string, targetStatus string, taskStatusChanges []ProposalTaskStatusChange, dependencyStatusChanges []ProposalDependencyStatusChange, resourceDemandChanges []ProposalResourceDemandChange, audit domain.AuditRecord) ([]string, error)
}

type ProposalCandidateWriter interface {
	WriteProposals(proposals []domain.Proposal) ([]string, error)
}

func NewProposalService(loader WorkspaceLoader) ProposalService {
	return ProposalService{loader: loader}
}

func (s ProposalService) List(ctx context.Context, req ProposalListRequest) (ProposalListResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ProposalListResult{
		Proposals: []ProposalSummary{},
		Warnings:  []Diagnostic{},
		Errors:    []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	for _, proposal := range ws.Proposals {
		if req.Status != "" && proposal.Status != req.Status {
			continue
		}
		result.Proposals = append(result.Proposals, proposalSummary(proposal))
	}
	sort.Slice(result.Proposals, func(i, j int) bool {
		return result.Proposals[i].ID < result.Proposals[j].ID
	})
	return result, nil
}

func (s ProposalService) Show(ctx context.Context, req ProposalShowRequest) (ProposalShowResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ProposalShowResult{
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	for _, proposal := range ws.Proposals {
		if proposal.ID == req.ID {
			result.Proposal = proposal
			return result, nil
		}
	}
	result.Errors = append(result.Errors, Diagnostic{
		Code:     "proposal_not_found",
		Message:  fmt.Sprintf("proposal %s not found", req.ID),
		Severity: "error",
		Target:   Target{Kind: "proposal", ID: req.ID},
	})
	return result, ErrValidationFailed
}

func (s ProposalService) Accept(ctx context.Context, req ProposalDecisionRequest) (ProposalDecisionResult, error) {
	return s.decide(ctx, req, "accepted")
}

func (s ProposalService) Reject(ctx context.Context, req ProposalDecisionRequest) (ProposalDecisionResult, error) {
	return s.decide(ctx, req, "rejected")
}

func (s ProposalService) SuggestDependencies(ctx context.Context, req ProposalSuggestDependenciesRequest) (ProposalSuggestDependenciesResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ProposalSuggestDependenciesResult{
		DryRun:        !req.WriteCandidates,
		Proposals:     []domain.Proposal{},
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	if req.WriteCandidates && strings.TrimSpace(req.ActorID) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actor_required",
			Message:  "proposal suggest-dependencies --write-candidates requires --actor",
			Severity: "error",
			Target:   Target{Kind: "actor"},
		})
		return result, ErrValidationFailed
	}
	if strings.TrimSpace(req.ActorID) != "" && !hasActor(ws.Actors, req.ActorID) {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actor_not_found",
			Message:  fmt.Sprintf("actor %s not found", req.ActorID),
			Severity: "error",
			Target:   Target{Kind: "actor", ID: req.ActorID},
		})
		return result, ErrValidationFailed
	}

	taskByID := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		taskByID[task.ID] = task
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	createdBy := domain.Ref{}
	if strings.TrimSpace(req.ActorID) != "" {
		createdBy = domain.Ref{Kind: "actor", ID: req.ActorID}
	}
	for _, dependency := range ws.Dependencies {
		result.Proposals = append(result.Proposals, dependencyProposalCandidates(dependency, taskByID, ws.Project.DefaultStatusDate, createdBy, now)...)
	}
	result.Proposals = append(result.Proposals, successorReadyProposalCandidates(ws, createdBy, now)...)
	result.Proposals = filterExistingProposalCandidates(result.Proposals, ws.Proposals, &result.Warnings)
	sort.Slice(result.Proposals, func(i, j int) bool {
		return result.Proposals[i].ID < result.Proposals[j].ID
	})
	result.PlannedWrites = proposalCandidatePlannedWrites(result.Proposals)
	if !req.WriteCandidates {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, ok := session.target.(ProposalCandidateWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support proposal candidate writes",
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
		})
		return result, ErrValidationFailed
	}
	writtenFiles, err := writer.WriteProposals(result.Proposals)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "proposal_candidate_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
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
	result.Applied = len(writtenFiles) > 0
	return result, nil
}

func (s ProposalService) decide(ctx context.Context, req ProposalDecisionRequest, decision string) (ProposalDecisionResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ProposalDecisionResult{
		DryRun:        req.DryRun,
		Decision:      decision,
		NewStatus:     decision,
		Reason:        req.Reason,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	proposal, ok := findProposalByID(ws.Proposals, req.ID)
	if strings.TrimSpace(req.ID) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "proposal_id_required",
			Message:  "proposal decision requires proposal id",
			Severity: "error",
			Target:   Target{Kind: "proposal"},
		})
	} else if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "proposal_not_found",
			Message:  fmt.Sprintf("proposal %s not found", req.ID),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: req.ID},
		})
	}
	if !req.DryRun && strings.TrimSpace(req.ActorID) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actor_required",
			Message:  "proposal decision requires --actor unless --dry-run is used",
			Severity: "error",
			Target:   Target{Kind: "actor"},
		})
	}
	if req.ActorID != "" {
		if diagnostic := validateApprovingActor(ws, req.ActorID); diagnostic != nil {
			result.Errors = append(result.Errors, *diagnostic)
		}
	}
	if ok && proposal.Status != "open" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "proposal_not_open",
			Message:  fmt.Sprintf("proposal %s has status %s", proposal.ID, proposal.Status),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
	}
	if decision == "rejected" && strings.TrimSpace(req.Reason) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "reason_required",
			Message:  "proposal reject requires --reason",
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: req.ID},
		})
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	result.Proposal = proposal
	result.PreviousStatus = proposal.Status
	decided := proposal
	decided.Status = decision
	result.Proposal = decided
	audit := proposalDecisionAuditRecord(req, decided, decision)
	result.AuditRecordPreview = audit
	taskStatusChanges, taskStatusErrors := proposalTaskStatusApplyChanges(ws, decided, decision)
	result.Errors = append(result.Errors, taskStatusErrors...)
	dependencyStatusChanges, dependencyStatusErrors := proposalDependencyStatusApplyChanges(ws, decided, decision)
	result.Errors = append(result.Errors, dependencyStatusErrors...)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	result.SuggestedApplied = len(taskStatusChanges) > 0 || len(dependencyStatusChanges) > 0
	resourceDemandChanges, resourceDemandErrors := proposalResourceDemandApplyChanges(ws, decided, decision)
	result.Errors = append(result.Errors, resourceDemandErrors...)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	result.SuggestedApplied = result.SuggestedApplied || len(resourceDemandChanges) > 0
	result.PlannedWrites = proposalDecisionPlannedWrites(proposal, decided, audit, taskStatusChanges, dependencyStatusChanges, resourceDemandChanges)
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writtenFiles, err := writeProposalDecision(session.target, proposal.ID, decision, taskStatusChanges, dependencyStatusChanges, resourceDemandChanges, audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "proposal_decision_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
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

func writeProposalDecision(target WorkspaceLoader, proposalID, decision string, taskStatusChanges []ProposalTaskStatusChange, dependencyStatusChanges []ProposalDependencyStatusChange, resourceDemandChanges []ProposalResourceDemandChange, audit domain.AuditRecord) ([]string, error) {
	if len(taskStatusChanges) > 0 || len(dependencyStatusChanges) > 0 || len(resourceDemandChanges) > 0 {
		writer, ok := target.(ProposalApplyWriter)
		if !ok {
			return nil, fmt.Errorf("configured project store does not support proposal apply writes")
		}
		return writer.ApplyProposalDecision(proposalID, decision, taskStatusChanges, dependencyStatusChanges, resourceDemandChanges, audit)
	}
	writer, ok := target.(ProposalWriter)
	if !ok {
		return nil, fmt.Errorf("configured project store does not support proposal writes")
	}
	return writer.MoveProposal(proposalID, decision, audit)
}

func proposalSummary(proposal domain.Proposal) ProposalSummary {
	return ProposalSummary{
		ID:                   proposal.ID,
		Type:                 proposal.Type,
		Status:               proposal.Status,
		CreatedAt:            proposal.CreatedAt,
		CreatedBy:            proposal.CreatedBy.ID,
		TargetKind:           proposal.TargetRef.Kind,
		TargetID:             proposal.TargetRef.ID,
		Rationale:            proposal.Rationale,
		RequiresConfirmation: proposal.RequiresConfirmation,
	}
}

func findProposalByID(proposals []domain.Proposal, id string) (domain.Proposal, bool) {
	for _, proposal := range proposals {
		if proposal.ID == id {
			return proposal, true
		}
	}
	return domain.Proposal{}, false
}

func dependencyProposalCandidates(dependency domain.Dependency, taskByID map[string]domain.Task, statusDate string, createdBy domain.Ref, now time.Time) []domain.Proposal {
	var proposals []domain.Proposal
	if dependency.Type == "relates" {
		return proposals
	}
	open := dependency.Status != "resolved" && dependency.Status != "cancelled"
	// A satisfied dependency of any criticality can be marked resolved through
	// an audited proposal (GAP-024). Only required dependencies also get a
	// review proposal while their predecessor is not yet in the required state,
	// and only when the dependency matters at the status date, by the rule of
	// the required_dependency_unresolved warning: an open dependency of a task
	// planned later is the normal state of a plan made ahead.
	if open && predecessorSatisfiesDependency(dependency, taskByID) {
		criticality := dependency.Criticality
		if criticality == "" {
			criticality = "unspecified"
		}
		proposals = append(proposals, newDependencyProposal(
			dependency,
			"mark-dependency-resolved",
			"mark_dependency_resolved",
			"set",
			fmt.Sprintf("dependencies.%s.status", dependency.ID),
			"resolved",
			fmt.Sprintf("Dependency %s (%s) can be marked resolved because predecessor %s is in the required state.", dependency.ID, criticality, dependency.PredecessorRef.ID),
			createdBy,
			now,
		))
	} else if _, due := unresolvedDependencyReason(dependency, taskByID, statusDate); open && dependency.Criticality == "required" && due {
		proposals = append(proposals, newDependencyProposal(
			dependency,
			"review-dependency-resolution",
			"review_dependency_resolution",
			"review",
			fmt.Sprintf("dependencies.%s.status", dependency.ID),
			map[string]any{
				"current_status": dependency.Status,
				"expected":       "resolved_or_cancelled",
			},
			fmt.Sprintf("Required dependency %s is %s and needs explicit resolution review before the successor is treated as safe.", dependency.ID, dependency.Status),
			createdBy,
			now,
		))
	}
	switch dependency.Type {
	case "finish_to_start":
		if !dependencyActive(dependency.Status) {
			break
		}
		predecessor, predecessorOK := taskByID[dependency.PredecessorRef.ID]
		successor, successorOK := taskByID[dependency.SuccessorRef.ID]
		if predecessorOK && successorOK && predecessor.PlannedFinish != "" && successor.PlannedStart != "" && successor.PlannedStart < predecessor.PlannedFinish {
			proposals = append(proposals, newDependencyProposal(
				dependency,
				"adjust-successor-start",
				"adjust_successor_start",
				"set",
				fmt.Sprintf("tasks.%s.planned_start", successor.ID),
				predecessor.PlannedFinish,
				fmt.Sprintf("Successor task %s starts before predecessor task %s planned_finish; preview moving planned_start to %s.", successor.ID, predecessor.ID, predecessor.PlannedFinish),
				createdBy,
				now,
			))
		} else {
			missing := []string{}
			if predecessorOK && predecessor.PlannedFinish == "" {
				missing = append(missing, "predecessor.planned_finish")
			}
			if successorOK && successor.PlannedStart == "" {
				missing = append(missing, "successor.planned_start")
			}
			if len(missing) > 0 {
				proposals = append(proposals, newDependencyProposal(
					dependency,
					"review-dependency-schedule",
					"review_dependency_schedule",
					"review",
					fmt.Sprintf("dependencies.%s.schedule", dependency.ID),
					map[string]any{"missing_fields": missing},
					fmt.Sprintf("Finish-to-start dependency %s cannot be checked fully because %s is missing.", dependency.ID, strings.Join(missing, ", ")),
					createdBy,
					now,
				))
			}
		}
	case "blocks":
		successor, successorOK := taskByID[dependency.SuccessorRef.ID]
		if successorOK && dependencyActive(dependency.Status) && successorBlockedByOpenDependencyStatus(successor.TrackingStatus) {
			proposals = append(proposals, newDependencyProposal(
				dependency,
				"mark-successor-blocked",
				"mark_successor_blocked",
				"set",
				fmt.Sprintf("tasks.%s.tracking_status", successor.ID),
				"blocked",
				fmt.Sprintf("Active dependency %s blocks successor task %s, which is currently %s.", dependency.ID, successor.ID, successor.TrackingStatus),
				createdBy,
				now,
			))
		}
	}
	return proposals
}

func successorReadyProposalCandidates(ws domain.Workspace, createdBy domain.Ref, now time.Time) []domain.Proposal {
	dependenciesBySuccessor := map[string][]domain.Dependency{}
	for _, dependency := range ws.Dependencies {
		if dependency.Type != "relates" && dependency.SuccessorRef.Kind == "task" && dependency.Criticality == "required" {
			dependenciesBySuccessor[dependency.SuccessorRef.ID] = append(dependenciesBySuccessor[dependency.SuccessorRef.ID], dependency)
		}
	}
	var proposals []domain.Proposal
	for _, task := range ws.Tasks {
		if task.TrackingStatus != "blocked" {
			continue
		}
		dependencies := dependenciesBySuccessor[task.ID]
		if len(dependencies) == 0 || !allDependenciesResolvedOrCancelled(dependencies) {
			continue
		}
		proposals = append(proposals, newTaskProposal(
			task,
			"mark-successor-ready",
			"mark_successor_ready",
			"set",
			fmt.Sprintf("tasks.%s.tracking_status", task.ID),
			"ready",
			fmt.Sprintf("Task %s can be marked ready because all required dependencies targeting it are resolved or cancelled.", task.ID),
			createdBy,
			now,
		))
	}
	return proposals
}

func allDependenciesResolvedOrCancelled(dependencies []domain.Dependency) bool {
	for _, dependency := range dependencies {
		if dependency.Status != "resolved" && dependency.Status != "cancelled" {
			return false
		}
	}
	return true
}

func predecessorSatisfiesDependency(dependency domain.Dependency, taskByID map[string]domain.Task) bool {
	if dependency.PredecessorRef.Kind != "task" || dependency.PredecessorRef.ID == "" {
		return false
	}
	task, ok := taskByID[dependency.PredecessorRef.ID]
	if !ok {
		return false
	}
	requiredState := dependency.RequiredState
	if requiredState == "" {
		requiredState = "accepted"
	}
	return task.TrackingStatus == requiredState
}

func newTaskProposal(task domain.Task, idSuffix, proposalType, op, path string, value any, rationale string, createdBy domain.Ref, now time.Time) domain.Proposal {
	return domain.Proposal{
		ID:        fmt.Sprintf("proposal.task.%s.%s", task.ID, idSuffix),
		Type:      proposalType,
		Status:    "open",
		CreatedAt: now.Format(time.RFC3339),
		CreatedBy: createdBy,
		TargetRef: domain.Ref{
			Kind: "task",
			ID:   task.ID,
		},
		SuggestedChange: domain.SuggestedChange{
			Op:    op,
			Path:  path,
			Value: value,
		},
		Rationale:            rationale,
		RequiresConfirmation: true,
	}
}

func newDependencyProposal(dependency domain.Dependency, idSuffix, proposalType, op, path string, value any, rationale string, createdBy domain.Ref, now time.Time) domain.Proposal {
	return domain.Proposal{
		ID:        fmt.Sprintf("proposal.dependency.%s.%s", dependency.ID, idSuffix),
		Type:      proposalType,
		Status:    "open",
		CreatedAt: now.Format(time.RFC3339),
		CreatedBy: createdBy,
		TargetRef: domain.Ref{
			Kind: "dependency",
			ID:   dependency.ID,
		},
		SuggestedChange: domain.SuggestedChange{
			Op:    op,
			Path:  path,
			Value: value,
		},
		Rationale:            rationale,
		RequiresConfirmation: true,
	}
}

func filterExistingProposalCandidates(candidates []domain.Proposal, existing []domain.Proposal, warnings *[]Diagnostic) []domain.Proposal {
	existingIDs := map[string]bool{}
	for _, proposal := range existing {
		existingIDs[proposal.ID] = true
	}
	out := []domain.Proposal{}
	for _, candidate := range candidates {
		if existingIDs[candidate.ID] {
			*warnings = append(*warnings, Diagnostic{
				Code:     "proposal_candidate_already_exists",
				Message:  fmt.Sprintf("proposal candidate %s already exists", candidate.ID),
				Severity: "warning",
				Target:   Target{Kind: "proposal", ID: candidate.ID},
			})
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func proposalCandidatePlannedWrites(proposals []domain.Proposal) []PlannedWrite {
	writes := []PlannedWrite{}
	for _, proposal := range proposals {
		content, _ := yaml.Marshal(struct {
			Proposal domain.Proposal `yaml:"proposal"`
		}{Proposal: proposal})
		writes = append(writes, PlannedWrite{
			Path:    proposalStatusPath(proposal.Status, proposal.ID),
			Action:  "create",
			Format:  "yaml",
			Content: string(content),
		})
	}
	return writes
}

func proposalTaskStatusApplyChanges(ws domain.Workspace, proposal domain.Proposal, decision string) ([]ProposalTaskStatusChange, []Diagnostic) {
	if decision != "accepted" || !proposalTaskStatusApplyType(proposal.Type) {
		return nil, nil
	}
	var errors []Diagnostic
	expectedStatus := proposalTaskStatusApplyValue(proposal.Type)
	if proposal.SuggestedChange.Op != "set" {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change op %q is not supported for %s", proposal.ID, proposal.SuggestedChange.Op, proposal.Type),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	taskID, ok := parseTaskTrackingStatusProposalPath(proposal.SuggestedChange.Path)
	if !ok {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change path %q is not supported for %s", proposal.ID, proposal.SuggestedChange.Path, proposal.Type),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	newStatus, ok := proposal.SuggestedChange.Value.(string)
	if !ok || newStatus != expectedStatus {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change value must be %s for %s", proposal.ID, expectedStatus, proposal.Type),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	if !domain.ValidTrackingStatus(newStatus) {
		errors = append(errors, Diagnostic{
			Code:     "invalid_tracking_status",
			Message:  fmt.Sprintf("invalid tracking_status %q", newStatus),
			Severity: "error",
			Target:   Target{Kind: "task", ID: taskID},
		})
		return nil, errors
	}
	for _, task := range ws.Tasks {
		if task.ID == taskID {
			if task.TrackingStatus == newStatus {
				return nil, nil
			}
			return []ProposalTaskStatusChange{{
				TaskID:         task.ID,
				PreviousStatus: task.TrackingStatus,
				NewStatus:      newStatus,
			}}, nil
		}
	}
	errors = append(errors, Diagnostic{
		Code:     "task_not_found",
		Message:  fmt.Sprintf("task %s not found for proposal %s", taskID, proposal.ID),
		Severity: "error",
		Target:   Target{Kind: "task", ID: taskID},
	})
	return nil, errors
}

func proposalTaskStatusApplyType(proposalType string) bool {
	return proposalType == "mark_successor_blocked" || proposalType == "mark_successor_ready"
}

func proposalTaskStatusApplyValue(proposalType string) string {
	if proposalType == "mark_successor_ready" {
		return "ready"
	}
	return "blocked"
}

func parseTaskTrackingStatusProposalPath(path string) (string, bool) {
	const prefix = "tasks."
	const suffix = ".tracking_status"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	taskID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return taskID, taskID != ""
}

func proposalDependencyStatusApplyChanges(ws domain.Workspace, proposal domain.Proposal, decision string) ([]ProposalDependencyStatusChange, []Diagnostic) {
	if decision != "accepted" || proposal.Type != "mark_dependency_resolved" {
		return nil, nil
	}
	var errors []Diagnostic
	if proposal.SuggestedChange.Op != "set" {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change op %q is not supported for mark_dependency_resolved", proposal.ID, proposal.SuggestedChange.Op),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	dependencyID, ok := parseDependencyStatusProposalPath(proposal.SuggestedChange.Path)
	if !ok {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change path %q is not supported for mark_dependency_resolved", proposal.ID, proposal.SuggestedChange.Path),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	newStatus, ok := proposal.SuggestedChange.Value.(string)
	if !ok || newStatus != "resolved" {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change value must be resolved for mark_dependency_resolved", proposal.ID),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	for _, dependency := range ws.Dependencies {
		if dependency.ID == dependencyID {
			if dependency.Status == newStatus {
				return nil, nil
			}
			return []ProposalDependencyStatusChange{{
				DependencyID:   dependency.ID,
				PreviousStatus: dependency.Status,
				NewStatus:      newStatus,
			}}, nil
		}
	}
	errors = append(errors, Diagnostic{
		Code:     "dependency_not_found",
		Message:  fmt.Sprintf("dependency %s not found for proposal %s", dependencyID, proposal.ID),
		Severity: "error",
		Target:   Target{Kind: "dependency", ID: dependencyID},
	})
	return nil, errors
}

func parseDependencyStatusProposalPath(path string) (string, bool) {
	const prefix = "dependencies."
	const suffix = ".status"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, suffix) {
		return "", false
	}
	dependencyID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), suffix)
	return dependencyID, dependencyID != ""
}

func proposalResourceDemandApplyChanges(ws domain.Workspace, proposal domain.Proposal, decision string) ([]ProposalResourceDemandChange, []Diagnostic) {
	if decision != "accepted" || proposal.Type != "update_remaining_work" {
		return nil, nil
	}
	var errors []Diagnostic
	if proposal.SuggestedChange.Op != "set" {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change op %q is not supported for update_remaining_work", proposal.ID, proposal.SuggestedChange.Op),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	if proposal.SuggestedChange.Path != "remaining_amount" {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change path %q is not supported for update_remaining_work", proposal.ID, proposal.SuggestedChange.Path),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	if proposal.TargetRef.Kind != "resource_demand" || proposal.TargetRef.ID == "" {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_target",
			Message:  fmt.Sprintf("proposal %s target must be resource_demand", proposal.ID),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	newValue, ok := numericSuggestedValue(proposal.SuggestedChange.Value)
	if !ok {
		errors = append(errors, Diagnostic{
			Code:     "unsupported_proposal_suggested_change",
			Message:  fmt.Sprintf("proposal %s suggested_change value must be numeric for update_remaining_work", proposal.ID),
			Severity: "error",
			Target:   Target{Kind: "proposal", ID: proposal.ID},
		})
		return nil, errors
	}
	if newValue < 0 {
		errors = append(errors, Diagnostic{
			Code:     "invalid_remaining_amount",
			Message:  fmt.Sprintf("proposal %s suggested remaining_amount must be non-negative", proposal.ID),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: proposal.TargetRef.ID},
		})
		return nil, errors
	}
	for _, demand := range ws.ResourceDemands {
		if demand.ID == proposal.TargetRef.ID {
			if demand.HasRemaining() && demand.Remaining() == newValue {
				return nil, nil
			}
			return []ProposalResourceDemandChange{{
				DemandID:      demand.ID,
				Field:         "remaining_amount",
				PreviousValue: demand.Remaining(),
				NewValue:      newValue,
			}}, nil
		}
	}
	errors = append(errors, Diagnostic{
		Code:     "resource_demand_not_found",
		Message:  fmt.Sprintf("resource demand %s not found for proposal %s", proposal.TargetRef.ID, proposal.ID),
		Severity: "error",
		Target:   Target{Kind: "resource_demand", ID: proposal.TargetRef.ID},
	})
	return nil, errors
}

func numericSuggestedValue(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	default:
		return 0, false
	}
}

func proposalDecisionAuditRecord(req ProposalDecisionRequest, proposal domain.Proposal, decision string) domain.AuditRecord {
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	reason := req.Reason
	if reason == "" {
		reason = fmt.Sprintf("Proposal %s marked %s.", proposal.ID, decision)
	}
	result := "success"
	if req.DryRun {
		result = "dry_run"
	}
	return domain.AuditRecord{
		ID:           "audit." + compactTimestamp(now.Format(time.RFC3339)) + ".proposal-" + decision + "." + slugID(proposal.ID),
		OccurredAt:   now.Format(time.RFC3339),
		ActorRef:     domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:    "proposal_" + decision,
		TargetRef:    domain.Ref{Kind: "proposal", ID: proposal.ID},
		Result:       result,
		Reason:       reason,
		EvidenceRefs: proposal.EvidenceRefs,
	}
}

func proposalDecisionPlannedWrites(previous domain.Proposal, decided domain.Proposal, audit domain.AuditRecord, taskStatusChanges []ProposalTaskStatusChange, dependencyStatusChanges []ProposalDependencyStatusChange, resourceDemandChanges []ProposalResourceDemandChange) []PlannedWrite {
	proposalContent, _ := yaml.Marshal(struct {
		Proposal domain.Proposal `yaml:"proposal"`
	}{Proposal: decided})
	writes := []PlannedWrite{}
	if len(taskStatusChanges) > 0 {
		writes = append(writes, PlannedWrite{
			Path:   "canonical/tasks.yaml",
			Action: "update",
			Format: "yaml",
		})
	}
	if len(dependencyStatusChanges) > 0 {
		writes = append(writes, PlannedWrite{
			Path:   "canonical/dependencies.yaml",
			Action: "update",
			Format: "yaml",
		})
	}
	if len(resourceDemandChanges) > 0 {
		writes = append(writes, PlannedWrite{
			Path:   "canonical/resource_demands.yaml",
			Action: "update",
			Format: "yaml",
		})
	}
	writes = append(writes, []PlannedWrite{
		{
			Path:   proposalStatusPath(previous.Status, previous.ID),
			Action: "delete",
			Format: "yaml",
		},
		{
			Path:    proposalStatusPath(decided.Status, decided.ID),
			Action:  "create",
			Format:  "yaml",
			Content: string(proposalContent),
		},
		{
			Path:   AuditShardPath(audit.OccurredAt),
			Action: "append_jsonl_record",
			Format: "jsonl",
		},
	}...)
	return writes
}

func proposalStatusPath(status string, id string) string {
	return "proposals/" + status + "/" + id + ".yaml"
}

// AuditShardPath returns the month-sharded audit log path for a record with the
// given RFC3339 occurrence timestamp. It is the single source of truth for the
// audit shard layout; the storage adapter reuses it so previews and writes
// never diverge.
func AuditShardPath(occurredAt string) string {
	timestamp, err := time.Parse(time.RFC3339, occurredAt)
	if err != nil {
		return "audit/audit.unscheduled.jsonl"
	}
	return fmt.Sprintf("audit/audit.%04d-%02d.jsonl", timestamp.Year(), timestamp.Month())
}
