package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

type TrackingService struct {
	loader WorkspaceLoader
}

type TrackingSuggestRequest struct {
	ProjectPath     string
	Week            string
	ActorID         string
	WriteCandidates bool
	Now             time.Time
}

type TrackingSuggestResult struct {
	ProjectPath   string               `json:"project_path"`
	ProjectID     string               `json:"project_id"`
	ProjectName   string               `json:"project_name"`
	Week          string               `json:"week,omitempty"`
	DryRun        bool                 `json:"dry_run"`
	Applied       bool                 `json:"applied"`
	Suggestions   []TrackingSuggestion `json:"suggestions"`
	Proposals     []domain.Proposal    `json:"proposals,omitempty"`
	PlannedWrites []PlannedWrite       `json:"planned_writes,omitempty"`
	WrittenFiles  []string             `json:"written_files,omitempty"`
	Validation    *ValidateResult      `json:"validation,omitempty"`
	Warnings      []Diagnostic         `json:"warnings"`
	Errors        []Diagnostic         `json:"errors"`
}

type TrackingSuggestion struct {
	ID                 string  `json:"id"`
	Type               string  `json:"type"`
	Severity           string  `json:"severity"`
	TaskID             string  `json:"task_id,omitempty"`
	TaskTitle          string  `json:"task_title,omitempty"`
	DemandID           string  `json:"demand_id,omitempty"`
	ActualID           string  `json:"actual_id,omitempty"`
	ResourceID         string  `json:"resource_id,omitempty"`
	ResourceClass      string  `json:"resource_class,omitempty"`
	Role               string  `json:"role,omitempty"`
	Unit               string  `json:"unit,omitempty"`
	Planned            float64 `json:"planned,omitempty"`
	Actual             float64 `json:"actual,omitempty"`
	Remaining          float64 `json:"remaining,omitempty"`
	Forecast           float64 `json:"forecast,omitempty"`
	Variance           float64 `json:"variance,omitempty"`
	Rationale          string  `json:"rationale"`
	RecommendedCommand string  `json:"recommended_command,omitempty"`
}

func NewTrackingService(loader WorkspaceLoader) TrackingService {
	return TrackingService{loader: loader}
}

func (s TrackingService) Suggest(ctx context.Context, req TrackingSuggestRequest) (TrackingSuggestResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	week := req.Week
	if week == "" {
		week = defaultDashboardWeek(ws)
	}
	result := TrackingSuggestResult{
		ProjectPath:   s.loader.ProjectPath(),
		ProjectID:     ws.Project.ID,
		ProjectName:   ws.Project.Name,
		Week:          week,
		DryRun:        !req.WriteCandidates,
		Suggestions:   []TrackingSuggestion{},
		Proposals:     []domain.Proposal{},
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	state := trackingSuggestState{service: s, ctx: ctx, req: req, workspace: ws, result: &result}
	if err := state.validate(); err != nil {
		return result, err
	}
	state.collectSuggestions()
	state.prepareProposals()
	if !req.WriteCandidates {
		return result, nil
	}
	if err := state.writeProposals(); err != nil {
		return result, err
	}
	return result, nil
}

type trackingSuggestState struct {
	service   TrackingService
	ctx       context.Context
	req       TrackingSuggestRequest
	workspace domain.Workspace
	result    *TrackingSuggestResult
}

func (s trackingSuggestState) validate() error {
	if s.req.WriteCandidates && strings.TrimSpace(s.req.ActorID) == "" {
		s.result.Errors = append(s.result.Errors, Diagnostic{
			Code:     "actor_required",
			Message:  "tracking suggest --write-candidates requires --actor",
			Severity: "error",
			Target:   Target{Kind: "actor"},
		})
	}
	if strings.TrimSpace(s.req.ActorID) != "" && !hasActor(s.workspace.Actors, s.req.ActorID) {
		s.result.Errors = append(s.result.Errors, Diagnostic{
			Code:     "actor_not_found",
			Message:  fmt.Sprintf("actor %s not found", s.req.ActorID),
			Severity: "error",
			Target:   Target{Kind: "actor", ID: s.req.ActorID},
		})
	}
	if len(s.result.Errors) > 0 {
		return ErrValidationFailed
	}
	return nil
}

func (s trackingSuggestState) collectSuggestions() {
	actualsByTaskID := map[string]int{}
	actualsMissingEvidenceByTaskID := map[string][]string{}
	for _, actual := range s.workspace.Actuals {
		if actual.TaskRef.ID == "" {
			continue
		}
		actualsByTaskID[actual.TaskRef.ID]++
		if len(actual.EvidenceRefs) == 0 {
			actualsMissingEvidenceByTaskID[actual.TaskRef.ID] = append(actualsMissingEvidenceByTaskID[actual.TaskRef.ID], actual.ID)
		}
	}

	overrunReviews := map[string]*domain.OverrunReview{}
	for _, demand := range s.workspace.ResourceDemands {
		if demand.OverrunReview != nil {
			overrunReviews[demand.ID] = demand.OverrunReview
		}
	}

	for _, row := range dashboardTrackingRows(s.workspace) {
		if isAcceptedTaskStatus(row.TrackingStatus) && actualsByTaskID[row.TaskID] == 0 && expectsActuals(row) {
			s.result.Suggestions = append(s.result.Suggestions, TrackingSuggestion{
				ID:        trackingSuggestionID("review_missing_actuals", row.TaskID, ""),
				Type:      "review_missing_actuals",
				Severity:  "review",
				TaskID:    row.TaskID,
				TaskTitle: row.Title,
				Rationale: "Task is accepted or done, but RePlan has no actual consumption records for it. Review whether actual work should be imported from the PM system or entered manually.",
			})
		}
		for _, actualID := range actualsMissingEvidenceByTaskID[row.TaskID] {
			s.result.Suggestions = append(s.result.Suggestions, TrackingSuggestion{
				ID:        trackingSuggestionID("review_missing_evidence", row.TaskID, actualID),
				Type:      "review_missing_evidence",
				Severity:  "review",
				TaskID:    row.TaskID,
				TaskTitle: row.Title,
				ActualID:  actualID,
				Rationale: "Actual consumption exists without evidence references. Link it to a pull request, OpenProject time entry, operator note, offline milestone or another evidence record before trusting it for replanning.",
			})
		}
		for _, demand := range row.Demands {
			if demand.Variance <= 0 {
				continue
			}
			suggestion := TrackingSuggestion{
				ID:            trackingSuggestionID("review_forecast_over_plan", row.TaskID, demand.DemandID),
				Type:          "review_forecast_over_plan",
				Severity:      "review",
				TaskID:        row.TaskID,
				TaskTitle:     row.Title,
				DemandID:      demand.DemandID,
				ResourceID:    demand.ResourceID,
				ResourceClass: demand.ResourceClass,
				Role:          demand.Role,
				Unit:          demand.Unit,
				Planned:       demand.Planned,
				Actual:        demand.Actual,
				Remaining:     demand.Remaining,
				Forecast:      demand.Forecast,
				Variance:      demand.Variance,
				Rationale:     "Forecast is above the planned amount. Review whether this is a real overrun, stale remaining work, duplicate actuals or an incomplete task estimate.",
			}
			staleRemaining := isAcceptedTaskStatus(row.TrackingStatus) && demand.Actual > 0 && demand.Remaining > 0
			if !staleRemaining && overrunAcknowledged(overrunReviews[demand.DemandID], demand.Unit, demand.Forecast) {
				// Reviewed and accepted as real by an operator (GAP-016).
				continue
			}
			if staleRemaining {
				suggestion.ID = trackingSuggestionID("set_remaining_to_zero_candidate", row.TaskID, demand.DemandID)
				suggestion.Type = "set_remaining_to_zero_candidate"
				suggestion.Rationale = "Task is accepted or done, actual consumption exists, and remaining work is still positive. This often means remaining work was not closed after acceptance; verify and, if correct, set remaining to zero."
				suggestion.RecommendedCommand = fmt.Sprintf("replan actual set-remaining --project %s --demand %s --amount 0 --unit %s --reason \"Accepted task has actuals; operator reviewed stale remaining work\" --actor <actor-id> --dry-run --format json",
					s.result.ProjectPath,
					demand.DemandID,
					demand.Unit,
				)
			}
			s.result.Suggestions = append(s.result.Suggestions, suggestion)
		}
	}

	sort.Slice(s.result.Suggestions, func(i, j int) bool {
		left := s.result.Suggestions[i]
		right := s.result.Suggestions[j]
		if left.Type != right.Type {
			return left.Type < right.Type
		}
		if left.TaskID != right.TaskID {
			return left.TaskID < right.TaskID
		}
		if left.DemandID != right.DemandID {
			return left.DemandID < right.DemandID
		}
		if left.ActualID != right.ActualID {
			return left.ActualID < right.ActualID
		}
		return left.ID < right.ID
	})
}

func (s trackingSuggestState) prepareProposals() {
	now := s.req.Now
	if now.IsZero() {
		now = time.Now()
	}
	createdBy := domain.Ref{}
	if strings.TrimSpace(s.req.ActorID) != "" {
		createdBy = domain.Ref{Kind: "actor", ID: s.req.ActorID}
	}
	s.result.Proposals = trackingSuggestionProposalCandidates(s.result.Suggestions, createdBy, now)
	s.result.Proposals = filterExistingProposalCandidates(s.result.Proposals, s.workspace.Proposals, &s.result.Warnings)
	s.result.PlannedWrites = proposalCandidatePlannedWrites(s.result.Proposals)
}

func (s trackingSuggestState) writeProposals() error {
	session := beginWrite(s.service.loader)
	writer, ok := session.target.(ProposalCandidateWriter)
	if !ok {
		s.result.Errors = append(s.result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support proposal candidate writes",
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.service.loader.ProjectPath()},
		})
		return ErrValidationFailed
	}
	writtenFiles, err := writer.WriteProposals(s.result.Proposals)
	if err != nil {
		s.result.Errors = append(s.result.Errors, Diagnostic{
			Code:     "proposal_candidate_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.service.loader.ProjectPath()},
		})
		return err
	}
	s.result.WrittenFiles = writtenFiles
	validation, written, err := session.finish(s.ctx, s.result.WrittenFiles)
	s.result.Validation = &validation
	s.result.WrittenFiles = written
	if err != nil {
		s.result.Errors = append(s.result.Errors, validation.Errors...)
		return err
	}
	s.result.Applied = len(writtenFiles) > 0
	if len(s.result.Errors) > 0 {
		return ErrValidationFailed
	}
	return nil
}

func trackingSuggestionProposalCandidates(suggestions []TrackingSuggestion, createdBy domain.Ref, now time.Time) []domain.Proposal {
	proposals := []domain.Proposal{}
	for _, suggestion := range suggestions {
		if suggestion.Type != "set_remaining_to_zero_candidate" || suggestion.DemandID == "" {
			continue
		}
		proposals = append(proposals, domain.Proposal{
			ID:        "proposal.tracking." + trackingIDPart(suggestion.DemandID) + ".set-remaining-zero",
			Type:      "update_remaining_work",
			Status:    "open",
			CreatedAt: now.Format(time.RFC3339),
			CreatedBy: createdBy,
			TargetRef: domain.Ref{Kind: "resource_demand", ID: suggestion.DemandID},
			SuggestedChange: domain.SuggestedChange{
				Op:    "set",
				Path:  "remaining_amount",
				Value: 0,
			},
			Rationale:            suggestion.Rationale,
			RequiresConfirmation: true,
		})
	}
	sort.Slice(proposals, func(i, j int) bool { return proposals[i].ID < proposals[j].ID })
	return proposals
}

func trackingSuggestionID(kind, taskID, targetID string) string {
	parts := []string{"tracking_suggestion", kind, trackingIDPart(taskID)}
	if targetID != "" {
		parts = append(parts, trackingIDPart(targetID))
	}
	return strings.Join(parts, ".")
}

func trackingIDPart(value string) string {
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "unknown"
	}
	return out
}
