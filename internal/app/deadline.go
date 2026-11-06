package app

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type DeadlineWriter interface {
	AddDeadlines([]domain.Deadline) (string, error)
	AppendAudit(*domain.AuditRecord) (string, error)
}

type DeadlineAddItem struct {
	ID, TaskID, MilestoneID, Date string
	Type, Criticality, Timezone   string
	Source, Description           string
}

type DeadlineAddRequest struct {
	Items                       []DeadlineAddItem
	Reason, ActorID, EvidenceID string
	OccurredAt                  string
	DryRun                      bool
}

type DeadlineAddResult struct {
	DryRun              bool                 `json:"dry_run"`
	Applied             bool                 `json:"applied"`
	Deadlines           []domain.Deadline    `json:"deadlines"`
	PlannedWrites       []PlannedWrite       `json:"planned_writes"`
	WrittenFiles        []string             `json:"written_files,omitempty"`
	AuditRecordPreviews []domain.AuditRecord `json:"audit_record_previews"`
	Validation          *ValidateResult      `json:"validation,omitempty"`
	Warnings            []Diagnostic         `json:"warnings"`
	Errors              []Diagnostic         `json:"errors"`
}

type DeadlineListResult struct {
	Deadlines []domain.Deadline `json:"deadlines"`
	Summary   struct {
		Deadlines int `json:"deadlines"`
		Hard      int `json:"hard"`
	} `json:"summary"`
	Warnings []Diagnostic `json:"warnings"`
	Errors   []Diagnostic `json:"errors"`
}

type DeadlineService struct{ loader WorkspaceLoader }

func NewDeadlineService(loader WorkspaceLoader) DeadlineService {
	return DeadlineService{loader: loader}
}

func (s DeadlineService) List(ctx context.Context) (DeadlineListResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := DeadlineListResult{Deadlines: append([]domain.Deadline{}, ws.Deadlines...), Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	sort.Slice(result.Deadlines, func(i, j int) bool { return result.Deadlines[i].ID < result.Deadlines[j].ID })
	result.Summary.Deadlines = len(result.Deadlines)
	for _, deadline := range result.Deadlines {
		if deadline.Criticality == "hard" {
			result.Summary.Hard++
		}
	}
	return result, nil
}

func (s DeadlineService) Add(ctx context.Context, req DeadlineAddRequest) (DeadlineAddResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := DeadlineAddResult{DryRun: req.DryRun, Deadlines: []domain.Deadline{}, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, AuditRecordPreviews: []domain.AuditRecord{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	validateConstraintAddCommon(&result.Errors, ws, len(req.Items), "deadline", req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt)
	seen := map[string]bool{}
	for _, deadline := range ws.Deadlines {
		seen[deadline.ID] = true
	}
	for _, item := range req.Items {
		deadline := deadlineFromAddItem(item)
		validateDeadlineAddItem(&result.Errors, ws, item, deadline, seen)
		seen[deadline.ID] = true
		result.Deadlines = append(result.Deadlines, deadline)
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	occurredAt := req.OccurredAt
	if occurredAt == "" {
		occurredAt = time.Now().Format(time.RFC3339)
	}
	for _, deadline := range result.Deadlines {
		audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".deadline-add." + slugID(deadline.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "deadline_add", TargetRef: domain.Ref{Kind: "deadline", ID: deadline.ID}, Result: "success", Reason: req.Reason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}}
		if req.DryRun {
			audit.Result = "dry_run"
		}
		result.AuditRecordPreviews = append(result.AuditRecordPreviews, audit)
	}
	content, _ := yaml.Marshal(struct {
		Deadlines []domain.Deadline `yaml:"deadlines"`
	}{result.Deadlines})
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{Path: "canonical/deadlines.yaml", Action: "append_yaml_items", Format: "yaml", Content: string(content)})
	for _, audit := range result.AuditRecordPreviews {
		result.PlannedWrites = append(result.PlannedWrites, auditPlannedWrite(audit))
	}
	if req.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, ok := session.target.(DeadlineWriter)
	if !ok {
		result.Errors = append(result.Errors, deadlineDiag("storage_write_not_supported", "configured project store does not support deadline writes", "project", s.loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	path, err := writer.AddDeadlines(result.Deadlines)
	if err != nil {
		result.Errors = append(result.Errors, deadlineDiag("deadline_write_failed", err.Error(), "deadline", ""))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, path)
	for i := range result.AuditRecordPreviews {
		audit := &result.AuditRecordPreviews[i]
		path, err = writer.AppendAudit(audit)
		if err != nil {
			result.Errors = append(result.Errors, deadlineDiag("audit_write_failed", err.Error(), "audit_record", audit.ID))
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

func deadlineFromAddItem(item DeadlineAddItem) domain.Deadline {
	deadlineType := item.Type
	if deadlineType == "" {
		deadlineType = "due_date"
	}
	criticality := item.Criticality
	if criticality == "" {
		criticality = "hard"
	}
	target := item.TaskID
	if target == "" {
		target = item.MilestoneID
	}
	deadline := domain.Deadline{ID: item.ID, TargetRef: domain.Ref{Kind: "task", ID: target}, Type: deadlineType, Timezone: item.Timezone, Criticality: criticality, Source: item.Source, Description: item.Description}
	if deadlineType == "due_timestamp" {
		deadline.DueTimestamp = item.Date
	} else {
		deadline.DueDate = item.Date
	}
	return deadline
}

func validateDeadlineAddItem(errors *[]Diagnostic, ws domain.Workspace, item DeadlineAddItem, deadline domain.Deadline, seen map[string]bool) {
	if deadline.ID == "" {
		*errors = append(*errors, deadlineDiag("missing_deadline_id", "deadline id is required", "deadline", ""))
	} else if seen[deadline.ID] {
		*errors = append(*errors, deadlineDiag("duplicate_deadline_id", fmt.Sprintf("deadline id %s already exists", deadline.ID), "deadline", deadline.ID))
	}
	if (item.TaskID == "") == (item.MilestoneID == "") {
		*errors = append(*errors, deadlineDiag("invalid_deadline_target", "exactly one of task or milestone is required", "deadline", deadline.ID))
	}
	if !dependencyHasTask(ws.Tasks, deadline.TargetRef.ID) {
		*errors = append(*errors, deadlineDiag("deadline_target_not_found", fmt.Sprintf("deadline target task %s not found", deadline.TargetRef.ID), "task", deadline.TargetRef.ID))
	}
	if !domain.ValidDeadlineType(deadline.Type) {
		*errors = append(*errors, deadlineDiag("invalid_deadline_type", fmt.Sprintf("invalid deadline type %q", deadline.Type), "deadline", deadline.ID))
	}
	if !domain.ValidDeadlineCriticality(deadline.Criticality) {
		*errors = append(*errors, deadlineDiag("invalid_deadline_criticality", fmt.Sprintf("invalid deadline criticality %q", deadline.Criticality), "deadline", deadline.ID))
	}
	if deadline.Type == "due_timestamp" {
		if _, err := time.Parse(time.RFC3339, deadline.DueTimestamp); err != nil {
			*errors = append(*errors, deadlineDiag("invalid_deadline_timestamp", "date must be RFC3339 with an offset for due_timestamp", "deadline", deadline.ID))
		}
	} else if !validDate(deadline.DueDate) {
		*errors = append(*errors, deadlineDiag("invalid_deadline_date", "date must use YYYY-MM-DD", "deadline", deadline.ID))
	}
}

func deadlineDiag(code, message, kind, id string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}}
}
