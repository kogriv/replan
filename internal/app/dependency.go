package app

import (
	"context"
	"sort"

	"github.com/kogriv/replan/internal/domain"
)

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
