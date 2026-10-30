package app

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type ActualService struct {
	loader WorkspaceLoader
}

type ActualWriter interface {
	AppendActual(statusDate string, actual domain.Actual) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
	SetResourceDemandRemaining(demandID string, amount float64) (string, error)
	SetResourceDemandStatus(demandID string, status string) (string, error)
	SetTaskTrackingStatus(taskID string, status string) (string, error)
}

type ActualListRequest struct {
	Type       string
	TaskID     string
	DemandID   string
	ResourceID string
}

type ActualListResult struct {
	Actuals  []ActualSummary `json:"actuals"`
	Warnings []Diagnostic    `json:"warnings"`
	Errors   []Diagnostic    `json:"errors"`
}

type ActualAddConsumptionRequest struct {
	TaskID     string
	DemandID   string
	ResourceID string
	Amount     float64
	Unit       string
	EvidenceID string
	ActorID    string
	OccurredAt string
	StatusDate string
	DryRun     bool
	Now        time.Time
}

type ActualAddConsumptionResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	Actual             domain.Actual      `json:"actual"`
	ActualSummary      ActualSummary      `json:"actual_summary"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ActualCorrectRequest struct {
	ActualID        string
	CorrectedAmount *float64
	Reason          string
	EvidenceID      string
	ActorID         string
	OccurredAt      string
	DryRun          bool
	Now             time.Time
}

type ActualCorrectResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	Actual             domain.Actual      `json:"actual"`
	ActualSummary      ActualSummary      `json:"actual_summary"`
	PreviousAmount     float64            `json:"previous_amount"`
	CorrectedAmount    float64            `json:"corrected_amount"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ActualSetRemainingRequest struct {
	DemandID   string
	Amount     float64
	Unit       string
	Reason     string
	EvidenceID string
	ActorID    string
	OccurredAt string
	StatusDate string
	DryRun     bool
	Now        time.Time
}

type ActualSetRemainingResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	DemandID           string             `json:"demand_id"`
	TaskID             string             `json:"task_id,omitempty"`
	ResourceID         string             `json:"resource_id,omitempty"`
	PreviousAmount     float64            `json:"previous_amount"`
	NewAmount          float64            `json:"new_amount"`
	Unit               string             `json:"unit"`
	Reason             string             `json:"reason,omitempty"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ActualCloseDemandRequest struct {
	DemandID   string
	Status     string
	Reason     string
	EvidenceID string
	ActorID    string
	OccurredAt string
	StatusDate string
	DryRun     bool
	Now        time.Time
}

type ActualCloseDemandResult struct {
	DryRun         bool   `json:"dry_run"`
	Applied        bool   `json:"applied"`
	DemandID       string `json:"demand_id"`
	TaskID         string `json:"task_id,omitempty"`
	ResourceID     string `json:"resource_id,omitempty"`
	PreviousStatus string `json:"previous_status,omitempty"`
	NewStatus      string `json:"new_status"`
	Reason         string `json:"reason,omitempty"`
	// RemainingCleared reports that closing as consumed also set the remaining
	// amount to an explicit zero in the same write (GAP-029).
	RemainingCleared   bool               `json:"remaining_cleared,omitempty"`
	PreviousRemaining  *float64           `json:"previous_remaining,omitempty"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ActualAddStatusRequest struct {
	TaskID     string
	Status     string
	Reason     string
	EvidenceID string
	ActorID    string
	OccurredAt string
	StatusDate string
	DryRun     bool
	Now        time.Time
}

type ActualAddStatusResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	TaskID             string             `json:"task_id"`
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

type PlannedWrite struct {
	Path    string `json:"path"`
	Action  string `json:"action"`
	Format  string `json:"format"`
	Content string `json:"content"`
}

type ActualSummary struct {
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	TaskID       string   `json:"task_id,omitempty"`
	DemandID     string   `json:"demand_id,omitempty"`
	ResourceID   string   `json:"resource_id,omitempty"`
	Amount       float64  `json:"amount,omitempty"`
	CorrectsID   string   `json:"corrects_id,omitempty"`
	CorrectedTo  *float64 `json:"corrected_to,omitempty"`
	Unit         string   `json:"unit,omitempty"`
	OccurredAt   string   `json:"occurred_at,omitempty"`
	StatusDate   string   `json:"status_date,omitempty"`
	AcceptedBy   string   `json:"accepted_by,omitempty"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
}

func NewActualService(loader WorkspaceLoader) ActualService {
	return ActualService{loader: loader}
}

func (s ActualService) List(ctx context.Context, req ActualListRequest) (ActualListResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualListResult{
		Actuals:  []ActualSummary{},
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	latest := LatestActualCorrections(ws.Actuals)
	for _, actual := range ws.Actuals {
		if req.Type != "" && actual.Type != req.Type {
			continue
		}
		if req.TaskID != "" && actual.TaskRef.ID != req.TaskID {
			continue
		}
		if req.DemandID != "" && actual.DemandRef.ID != req.DemandID {
			continue
		}
		if req.ResourceID != "" && actual.ResourceRef.ID != req.ResourceID {
			continue
		}
		summary := actualSummary(actual)
		if actual.Type == "correction" {
			summary.CorrectedTo = actual.CorrectedAmount
		} else if correction, ok := latest[actual.ID]; ok {
			// Amount stays as booked; corrected_to is what reports count.
			summary.CorrectedTo = correction.CorrectedAmount
		}
		result.Actuals = append(result.Actuals, summary)
	}
	sort.Slice(result.Actuals, func(i, j int) bool {
		if result.Actuals[i].OccurredAt == result.Actuals[j].OccurredAt {
			return result.Actuals[i].ID < result.Actuals[j].ID
		}
		return result.Actuals[i].OccurredAt < result.Actuals[j].OccurredAt
	})
	return result, nil
}

func (s ActualService) Correct(ctx context.Context, req ActualCorrectRequest) (ActualCorrectResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualCorrectResult{DryRun: req.DryRun, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	original, _ := validateCorrectActualRequest(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	occurredAt := req.OccurredAt
	if occurredAt == "" {
		now := req.Now
		if now.IsZero() {
			now = time.Now()
		}
		occurredAt = now.Format(time.RFC3339)
	}
	latest := LatestActualCorrections(ws.Actuals)
	previousAmount := original.Amount
	if correction, exists := latest[original.ID]; exists && correction.CorrectedAmount != nil {
		previousAmount = *correction.CorrectedAmount
	}
	if previousAmount == *req.CorrectedAmount {
		addActualRequestError(&result.Errors, "actual_correction_no_change", fmt.Sprintf("actual %s effective amount is already %g", original.ID, previousAmount), "actual", original.ID)
		return result, ErrValidationFailed
	}
	actual := domain.Actual{
		ID: "actual." + slugID(original.ID) + ".correction." + compactTimestamp(occurredAt), Type: "correction",
		CorrectsRef: domain.Ref{Kind: "actual", ID: original.ID}, CorrectedAmount: req.CorrectedAmount,
		TaskRef: original.TaskRef, DemandRef: original.DemandRef, ResourceRef: original.ResourceRef, Unit: original.Unit,
		OccurredAt: occurredAt, StatusDate: datePart(occurredAt), AcceptedBy: domain.Ref{Kind: "actor", ID: req.ActorID}, Reason: req.Reason,
		EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}},
	}
	if hasActualID(ws.Actuals, actual.ID) {
		addActualRequestError(&result.Errors, "actual_already_exists", fmt.Sprintf("actual %s already exists", actual.ID), "actual", actual.ID)
		return result, ErrValidationFailed
	}
	recordYAML, err := yaml.Marshal(struct {
		Actuals []domain.Actual `yaml:"actuals"`
	}{[]domain.Actual{actual}})
	if err != nil {
		return result, err
	}
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".actual-correct." + slugID(original.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "actual_correct", TargetRef: domain.Ref{Kind: "actual", ID: original.ID}, Reason: req.Reason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}, Result: "success"}
	if req.DryRun {
		audit.Result = "dry_run"
	}
	result.Actual, result.ActualSummary = actual, actualSummary(actual)
	result.ActualSummary.CorrectedTo = actual.CorrectedAmount
	result.PreviousAmount, result.CorrectedAmount = previousAmount, *req.CorrectedAmount
	result.PlannedWrites = []PlannedWrite{{Path: actualsPath(actual.StatusDate), Action: "append_yaml_record", Format: "yaml", Content: string(recordYAML)}, auditPlannedWrite(audit)}
	result.AuditRecordPreview = audit
	if req.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, supported := session.target.(ActualWriter)
	if !supported {
		addActualRequestError(&result.Errors, "storage_write_not_supported", "configured project store does not support actual writes", "project", s.loader.ProjectPath())
		return result, ErrValidationFailed
	}
	actualPath, err := writer.AppendActual(actual.StatusDate, actual)
	if err != nil {
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, actualPath)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
	validation, written, err := session.finish(ctx, result.WrittenFiles)
	result.Validation, result.WrittenFiles = &validation, written
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

func (s ActualService) AddConsumption(ctx context.Context, req ActualAddConsumptionRequest) (ActualAddConsumptionResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualAddConsumptionResult{
		DryRun:        req.DryRun,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	validateAddConsumptionRequest(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	occurredAt := req.OccurredAt
	if occurredAt == "" {
		now := req.Now
		if now.IsZero() {
			now = time.Now()
		}
		occurredAt = now.Format(time.RFC3339)
	}
	statusDate := req.StatusDate
	if statusDate == "" {
		statusDate = datePart(occurredAt)
	}
	actual := domain.Actual{
		ID:          actualConsumptionID(req.TaskID, req.ResourceID, occurredAt),
		Type:        "work_consumed",
		TaskRef:     domain.Ref{Kind: "task", ID: req.TaskID},
		DemandRef:   domain.Ref{Kind: "resource_demand", ID: req.DemandID},
		ResourceRef: domain.Ref{Kind: "resource", ID: req.ResourceID},
		Amount:      req.Amount,
		Unit:        req.Unit,
		OccurredAt:  occurredAt,
		StatusDate:  statusDate,
		AcceptedBy:  domain.Ref{Kind: "actor", ID: req.ActorID},
	}
	if req.EvidenceID != "" {
		actual.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}
	if hasActualID(ws.Actuals, actual.ID) {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actual_already_exists",
			Message:  fmt.Sprintf("actual %s already exists", actual.ID),
			Severity: "error",
			Target:   Target{Kind: "actual", ID: actual.ID},
		})
		return result, ErrValidationFailed
	}

	recordYAML, err := yaml.Marshal(struct {
		Actuals []domain.Actual `yaml:"actuals"`
	}{Actuals: []domain.Actual{actual}})
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "yaml_render_error",
			Message:  fmt.Sprintf("failed to render actual yaml: %v", err),
			Severity: "error",
		})
		return result, ErrValidationFailed
	}
	audit := domain.AuditRecord{
		ID:         "audit." + compactTimestamp(occurredAt) + ".actual-add-consumption." + slugID(req.DemandID),
		OccurredAt: occurredAt,
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:  "actual_add_consumption",
		TargetRef:  domain.Ref{Kind: "actual", ID: actual.ID},
		Reason:     "Preview actual consumption record.",
	}
	if req.DryRun {
		audit.Result = "dry_run"
	} else {
		audit.Result = "success"
		audit.Reason = "Recorded actual consumption."
	}
	if req.EvidenceID != "" {
		audit.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}

	result.Actual = actual
	result.ActualSummary = actualSummary(actual)
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
		Path:    actualsPath(statusDate),
		Action:  "append_yaml_record",
		Format:  "yaml",
		Content: string(recordYAML),
	})
	result.AuditRecordPreview = audit
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(ActualWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support actual writes",
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
		})
		return result, ErrValidationFailed
	}
	actualPath, err := writer.AppendActual(statusDate, actual)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actual_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "actual", ID: actual.ID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, actualPath)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "audit_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "audit_record", ID: audit.ID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
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

func (s ActualService) SetRemaining(ctx context.Context, req ActualSetRemainingRequest) (ActualSetRemainingResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualSetRemainingResult{
		DryRun:        req.DryRun,
		DemandID:      req.DemandID,
		NewAmount:     req.Amount,
		Unit:          req.Unit,
		Reason:        req.Reason,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	demand, ok := validateSetRemainingRequest(&result.Errors, ws, req)
	if ok {
		result.TaskID = demand.TaskRef.ID
		result.ResourceID = demand.ResourceRef.ID
		result.PreviousAmount = demand.Remaining()
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	occurredAt := req.OccurredAt
	if occurredAt == "" {
		now := req.Now
		if now.IsZero() {
			now = time.Now()
		}
		occurredAt = now.Format(time.RFC3339)
	}
	statusDate := req.StatusDate
	if statusDate == "" {
		statusDate = datePart(occurredAt)
	}
	patch := map[string]any{
		"op":    "replace",
		"path":  fmt.Sprintf("canonical/resource_demands/%s/remaining_amount", req.DemandID),
		"value": req.Amount,
		"unit":  req.Unit,
	}
	if req.Reason != "" {
		patch["reason"] = req.Reason
	}
	if statusDate != "" {
		patch["status_date"] = statusDate
	}
	patchYAML, err := yaml.Marshal(patch)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "yaml_render_error",
			Message:  fmt.Sprintf("failed to render remaining-work patch yaml: %v", err),
			Severity: "error",
		})
		return result, ErrValidationFailed
	}
	audit := domain.AuditRecord{
		ID:         "audit." + compactTimestamp(occurredAt) + ".actual-set-remaining." + slugID(req.DemandID),
		OccurredAt: occurredAt,
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:  "actual_set_remaining",
		TargetRef:  domain.Ref{Kind: "resource_demand", ID: req.DemandID},
		Reason:     req.Reason,
	}
	if req.DryRun {
		audit.Result = "dry_run"
	} else {
		audit.Result = "success"
		if audit.Reason == "" {
			audit.Reason = "Updated remaining work."
		}
	}
	if req.EvidenceID != "" {
		audit.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}

	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
		Path:    "canonical/resource_demands.yaml",
		Action:  "replace_yaml_field",
		Format:  "yaml-patch-preview",
		Content: string(patchYAML),
	})
	result.AuditRecordPreview = audit
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(ActualWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support actual writes",
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
		})
		return result, ErrValidationFailed
	}
	demandPath, err := writer.SetResourceDemandRemaining(req.DemandID, req.Amount)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "remaining_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: req.DemandID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, demandPath)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "audit_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "audit_record", ID: audit.ID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
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

func (s ActualService) CloseDemand(ctx context.Context, req ActualCloseDemandRequest) (ActualCloseDemandResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualCloseDemandResult{
		DryRun:        req.DryRun,
		DemandID:      req.DemandID,
		NewStatus:     req.Status,
		Reason:        req.Reason,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	demand, ok := validateCloseDemandRequest(&result.Errors, ws, req)
	if ok {
		result.TaskID = demand.TaskRef.ID
		result.ResourceID = demand.ResourceRef.ID
		result.PreviousStatus = demand.Status
		// Consumed means nothing is left. Keeping the old remaining amount made
		// the demand read as stale work (GAP-029). Cancelled keeps it: that is
		// the work that was not done.
		if req.Status == "consumed" && (!demand.HasRemaining() || demand.Remaining() != 0) {
			result.RemainingCleared = true
			result.PreviousRemaining = demand.RemainingAmount
		}
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	occurredAt := req.OccurredAt
	if occurredAt == "" {
		now := req.Now
		if now.IsZero() {
			now = time.Now()
		}
		occurredAt = now.Format(time.RFC3339)
	}
	statusDate := req.StatusDate
	if statusDate == "" {
		statusDate = datePart(occurredAt)
	}
	patch := map[string]any{
		"op":          "replace",
		"path":        fmt.Sprintf("canonical/resource_demands/%s/status", req.DemandID),
		"value":       req.Status,
		"before":      demand.Status,
		"status_date": statusDate,
	}
	if req.Reason != "" {
		patch["reason"] = req.Reason
	}
	if result.RemainingCleared {
		patch["remaining_amount"] = map[string]any{"before": result.PreviousRemaining, "value": 0}
	}
	patchYAML, err := yaml.Marshal(patch)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "yaml_render_error",
			Message:  fmt.Sprintf("failed to render demand-status patch yaml: %v", err),
			Severity: "error",
		})
		return result, ErrValidationFailed
	}
	audit := domain.AuditRecord{
		ID:         "audit." + compactTimestamp(occurredAt) + ".actual-close-demand." + slugID(req.DemandID),
		OccurredAt: occurredAt,
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:  "actual_close_demand",
		TargetRef:  domain.Ref{Kind: "resource_demand", ID: req.DemandID},
		Reason:     req.Reason,
	}
	if req.DryRun {
		audit.Result = "dry_run"
	} else {
		audit.Result = "success"
		if audit.Reason == "" {
			audit.Reason = "Updated resource demand lifecycle status."
		}
	}
	if req.EvidenceID != "" {
		audit.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
		Path:    "canonical/resource_demands.yaml",
		Action:  "replace_yaml_field",
		Format:  "yaml-patch-preview",
		Content: string(patchYAML),
	})
	result.AuditRecordPreview = audit
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(ActualWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support actual writes",
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
		})
		return result, ErrValidationFailed
	}
	demandPath, err := writer.SetResourceDemandStatus(req.DemandID, req.Status)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "demand_status_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: req.DemandID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, demandPath)
	if result.RemainingCleared {
		if _, err := writer.SetResourceDemandRemaining(req.DemandID, 0); err != nil {
			result.Errors = append(result.Errors, Diagnostic{
				Code:     "demand_remaining_write_failed",
				Message:  err.Error(),
				Severity: "error",
				Target:   Target{Kind: "resource_demand", ID: req.DemandID},
			})
			return result, err
		}
	}
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "audit_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "audit_record", ID: audit.ID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
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

func (s ActualService) AddStatus(ctx context.Context, req ActualAddStatusRequest) (ActualAddStatusResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualAddStatusResult{
		DryRun:        req.DryRun,
		TaskID:        req.TaskID,
		NewStatus:     req.Status,
		Reason:        req.Reason,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	task, ok := validateAddStatusRequest(&result.Errors, ws, req)
	if ok {
		result.PreviousStatus = task.TrackingStatus
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	occurredAt := req.OccurredAt
	if occurredAt == "" {
		now := req.Now
		if now.IsZero() {
			now = time.Now()
		}
		occurredAt = now.Format(time.RFC3339)
	}
	statusDate := req.StatusDate
	if statusDate == "" {
		statusDate = datePart(occurredAt)
	}
	patch := map[string]any{
		"op":     "replace",
		"path":   fmt.Sprintf("canonical/tasks/%s/tracking_status", req.TaskID),
		"value":  req.Status,
		"before": task.TrackingStatus,
	}
	if req.Reason != "" {
		patch["reason"] = req.Reason
	}
	if statusDate != "" {
		patch["status_date"] = statusDate
	}
	patchYAML, err := yaml.Marshal(patch)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "yaml_render_error",
			Message:  fmt.Sprintf("failed to render task-status patch yaml: %v", err),
			Severity: "error",
		})
		return result, ErrValidationFailed
	}
	audit := domain.AuditRecord{
		ID:         "audit." + compactTimestamp(occurredAt) + ".actual-add-status." + slugID(req.TaskID),
		OccurredAt: occurredAt,
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:  "actual_add_status",
		TargetRef:  domain.Ref{Kind: "task", ID: req.TaskID},
		NewStatus:  req.Status,
		StatusDate: statusDate,
		Reason:     req.Reason,
	}
	if req.DryRun {
		audit.Result = "dry_run"
	} else {
		audit.Result = "success"
		if audit.Reason == "" {
			audit.Reason = "Updated task tracking status."
		}
	}
	if req.EvidenceID != "" {
		audit.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
		Path:    "canonical/tasks.yaml",
		Action:  "replace_yaml_field",
		Format:  "yaml-patch-preview",
		Content: string(patchYAML),
	})
	result.AuditRecordPreview = audit
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(ActualWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support actual writes",
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
		})
		return result, ErrValidationFailed
	}
	taskPath, err := writer.SetTaskTrackingStatus(req.TaskID, req.Status)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "status_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "task", ID: req.TaskID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, taskPath)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "audit_write_failed",
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "audit_record", ID: audit.ID},
		})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
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

func actualSummary(actual domain.Actual) ActualSummary {
	evidenceRefs := make([]string, 0, len(actual.EvidenceRefs))
	for _, ref := range actual.EvidenceRefs {
		evidenceRefs = append(evidenceRefs, ref.ID)
	}
	return ActualSummary{
		ID:           actual.ID,
		Type:         actual.Type,
		TaskID:       actual.TaskRef.ID,
		DemandID:     actual.DemandRef.ID,
		ResourceID:   actual.ResourceRef.ID,
		Amount:       actual.Amount,
		CorrectsID:   actual.CorrectsRef.ID,
		CorrectedTo:  actual.CorrectedAmount,
		Unit:         actual.Unit,
		OccurredAt:   actual.OccurredAt,
		StatusDate:   actual.StatusDate,
		AcceptedBy:   actual.AcceptedBy.ID,
		EvidenceRefs: evidenceRefs,
	}
}

func validateCorrectActualRequest(errors *[]Diagnostic, ws domain.Workspace, req ActualCorrectRequest) (domain.Actual, bool) {
	if req.ActualID == "" {
		addActualRequestError(errors, "missing_actual", "actual is required", "actual", "")
	}
	if req.CorrectedAmount == nil {
		addActualRequestError(errors, "missing_corrected_amount", "choose an amount or void", "actual", req.ActualID)
	} else if !validAmount(*req.CorrectedAmount) {
		addActualRequestError(errors, "invalid_amount", "corrected amount must be a finite number, zero or greater", "actual", req.ActualID)
	}
	if strings.TrimSpace(req.Reason) == "" {
		addActualRequestError(errors, "missing_reason", "reason is required", "actual", req.ActualID)
	}
	if req.ActorID == "" {
		addActualRequestError(errors, "missing_actor", "actor is required", "actor", "")
	} else if !hasActor(ws.Actors, req.ActorID) {
		addActualRequestError(errors, "actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID)
	}
	if req.EvidenceID == "" {
		addActualRequestError(errors, "missing_evidence", "evidence is required", "evidence", "")
	} else if !hasEvidence(ws.Evidence, req.EvidenceID) {
		addActualRequestError(errors, "evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID)
	}
	if req.OccurredAt != "" {
		if _, err := time.Parse(time.RFC3339, req.OccurredAt); err != nil {
			addActualRequestError(errors, "invalid_occurred_at", fmt.Sprintf("occurred-at %q must be RFC3339", req.OccurredAt), "actual", req.ActualID)
		}
	}
	for _, actual := range ws.Actuals {
		if actual.ID != req.ActualID {
			continue
		}
		if actual.Type == "correction" || !estimateConsumptionTypes[actual.Type] {
			addActualRequestError(errors, "actual_not_correctable", fmt.Sprintf("actual %s of type %s is not correctable", actual.ID, actual.Type), "actual", actual.ID)
			return actual, false
		}
		return actual, true
	}
	if req.ActualID != "" {
		addActualRequestError(errors, "actual_not_found", fmt.Sprintf("actual %s not found", req.ActualID), "actual", req.ActualID)
	}
	return domain.Actual{}, false
}

func validateAddStatusRequest(errors *[]Diagnostic, ws domain.Workspace, req ActualAddStatusRequest) (domain.Task, bool) {
	if req.TaskID == "" {
		addActualRequestError(errors, "missing_task", "task is required", "task", "")
	}
	if req.Status == "" {
		addActualRequestError(errors, "missing_status", "status is required", "task", req.TaskID)
	}
	if req.Status != "" && !domain.ValidTrackingStatus(req.Status) {
		addActualRequestError(errors, "invalid_tracking_status", fmt.Sprintf("invalid tracking status %q", req.Status), "task", req.TaskID)
	}
	if req.ActorID == "" {
		addActualRequestError(errors, "missing_actor", "actor is required", "actor", "")
	}
	task, taskOK := findTask(ws.Tasks, req.TaskID)
	if req.TaskID != "" && !taskOK {
		addActualRequestError(errors, "task_not_found", fmt.Sprintf("task %s not found", req.TaskID), "task", req.TaskID)
	}
	if req.ActorID != "" && !hasActor(ws.Actors, req.ActorID) {
		addActualRequestError(errors, "actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID)
	}
	if req.ActorID != "" && (req.Status == "accepted" || req.Status == "cancelled") {
		if diagnostic := validateApprovingActor(ws, req.ActorID); diagnostic != nil {
			// Avoid duplicating actor_not_found from the general actor check.
			if diagnostic.Code != "actor_not_found" {
				*errors = append(*errors, *diagnostic)
			}
		}
	}
	if req.EvidenceID != "" && !hasEvidence(ws.Evidence, req.EvidenceID) {
		addActualRequestError(errors, "evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID)
	}
	return task, taskOK
}

func validateSetRemainingRequest(errors *[]Diagnostic, ws domain.Workspace, req ActualSetRemainingRequest) (domain.ResourceDemand, bool) {
	if req.DemandID == "" {
		addActualRequestError(errors, "missing_demand", "demand is required", "resource_demand", "")
	}
	if !validAmount(req.Amount) {
		addActualRequestError(errors, "invalid_amount", "remaining amount must be a finite number, zero or greater", "resource_demand", req.DemandID)
	}
	if req.Unit == "" {
		addActualRequestError(errors, "missing_unit", "unit is required", "resource_demand", req.DemandID)
	}
	if req.ActorID == "" {
		addActualRequestError(errors, "missing_actor", "actor is required", "actor", "")
	}
	demand, demandOK := findDemand(ws.ResourceDemands, req.DemandID)
	if req.DemandID != "" && !demandOK {
		addActualRequestError(errors, "demand_not_found", fmt.Sprintf("resource demand %s not found", req.DemandID), "resource_demand", req.DemandID)
	}
	if req.ActorID != "" && !hasActor(ws.Actors, req.ActorID) {
		addActualRequestError(errors, "actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID)
	}
	if req.EvidenceID != "" && !hasEvidence(ws.Evidence, req.EvidenceID) {
		addActualRequestError(errors, "evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID)
	}
	if demandOK && req.Unit != "" && demand.Unit != req.Unit {
		addActualRequestError(errors, "unit_mismatch", fmt.Sprintf("resource demand %s uses unit %s, not %s", demand.ID, demand.Unit, req.Unit), "resource_demand", demand.ID)
	}
	return demand, demandOK
}

func validateCloseDemandRequest(errors *[]Diagnostic, ws domain.Workspace, req ActualCloseDemandRequest) (domain.ResourceDemand, bool) {
	if req.DemandID == "" {
		addActualRequestError(errors, "missing_demand", "demand is required", "resource_demand", "")
	}
	if req.Status == "" {
		addActualRequestError(errors, "missing_status", "status is required", "resource_demand", req.DemandID)
	}
	if req.Status != "" && req.Status != "consumed" && req.Status != "cancelled" {
		addActualRequestError(errors, "invalid_resource_demand_close_status", fmt.Sprintf("resource demand close status must be consumed or cancelled, not %q", req.Status), "resource_demand", req.DemandID)
	}
	if req.ActorID == "" {
		addActualRequestError(errors, "missing_actor", "actor is required", "actor", "")
	}
	demand, demandOK := findDemand(ws.ResourceDemands, req.DemandID)
	if req.DemandID != "" && !demandOK {
		addActualRequestError(errors, "demand_not_found", fmt.Sprintf("resource demand %s not found", req.DemandID), "resource_demand", req.DemandID)
	}
	if req.ActorID != "" && !hasActor(ws.Actors, req.ActorID) {
		addActualRequestError(errors, "actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID)
	}
	if req.EvidenceID != "" && !hasEvidence(ws.Evidence, req.EvidenceID) {
		addActualRequestError(errors, "evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID)
	}
	if demandOK && demand.Status == req.Status {
		addActualRequestError(errors, "demand_status_unchanged", fmt.Sprintf("resource demand %s already has status %s", demand.ID, req.Status), "resource_demand", demand.ID)
	}
	return demand, demandOK
}

func validateAddConsumptionRequest(errors *[]Diagnostic, ws domain.Workspace, req ActualAddConsumptionRequest) {
	if req.TaskID == "" {
		addActualRequestError(errors, "missing_task", "task is required", "task", "")
	}
	if req.DemandID == "" {
		addActualRequestError(errors, "missing_demand", "demand is required", "resource_demand", "")
	}
	if req.ResourceID == "" {
		addActualRequestError(errors, "missing_resource", "resource is required", "resource", "")
	}
	if !validAmount(req.Amount) || req.Amount == 0 {
		addActualRequestError(errors, "invalid_amount", "amount must be a finite number greater than zero", "actual", "")
	}
	if req.Unit == "" {
		addActualRequestError(errors, "missing_unit", "unit is required", "actual", "")
	}
	if req.ActorID == "" {
		addActualRequestError(errors, "missing_actor", "actor is required", "actor", "")
	}

	task, taskOK := findTask(ws.Tasks, req.TaskID)
	demand, demandOK := findDemand(ws.ResourceDemands, req.DemandID)
	if req.TaskID != "" && !taskOK {
		addActualRequestError(errors, "task_not_found", fmt.Sprintf("task %s not found", req.TaskID), "task", req.TaskID)
	}
	if req.DemandID != "" && !demandOK {
		addActualRequestError(errors, "demand_not_found", fmt.Sprintf("resource demand %s not found", req.DemandID), "resource_demand", req.DemandID)
	}
	if req.ResourceID != "" && !hasResource(ws.Resources, req.ResourceID) {
		addActualRequestError(errors, "resource_not_found", fmt.Sprintf("resource %s not found", req.ResourceID), "resource", req.ResourceID)
	}
	if req.ActorID != "" && !hasActor(ws.Actors, req.ActorID) {
		addActualRequestError(errors, "actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID)
	}
	if req.EvidenceID != "" && !hasEvidence(ws.Evidence, req.EvidenceID) {
		addActualRequestError(errors, "evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID)
	}
	if taskOK && demandOK {
		if demand.TaskRef.ID != task.ID {
			addActualRequestError(errors, "demand_task_mismatch", fmt.Sprintf("resource demand %s belongs to task %s, not %s", demand.ID, demand.TaskRef.ID, task.ID), "resource_demand", demand.ID)
		}
		if demand.ResourceRef.ID != "" && demand.ResourceRef.ID != req.ResourceID {
			addActualRequestError(errors, "demand_resource_mismatch", fmt.Sprintf("resource demand %s targets resource %s, not %s", demand.ID, demand.ResourceRef.ID, req.ResourceID), "resource_demand", demand.ID)
		}
		if demand.ResourcePoolRef.ID != "" {
			addActualRequestError(errors, "pooled_demand_not_supported", fmt.Sprintf("resource demand %s targets a resource pool; direct consumption for pool demands is not supported yet", demand.ID), "resource_demand", demand.ID)
		}
		if req.Unit != "" && demand.Unit != req.Unit {
			addActualRequestError(errors, "unit_mismatch", fmt.Sprintf("resource demand %s uses unit %s, not %s", demand.ID, demand.Unit, req.Unit), "resource_demand", demand.ID)
		}
	}
}

func addActualRequestError(errors *[]Diagnostic, code, message, kind, id string) {
	*errors = append(*errors, Diagnostic{
		Code:     code,
		Message:  message,
		Severity: "error",
		Target:   Target{Kind: kind, ID: id},
	})
}

func findTask(tasks []domain.Task, id string) (domain.Task, bool) {
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return domain.Task{}, false
}

func findDemand(demands []domain.ResourceDemand, id string) (domain.ResourceDemand, bool) {
	for _, demand := range demands {
		if demand.ID == id {
			return demand, true
		}
	}
	return domain.ResourceDemand{}, false
}

func hasResource(resources []domain.Resource, id string) bool {
	for _, resource := range resources {
		if resource.ID == id {
			return true
		}
	}
	return false
}

func hasActor(actors []domain.Actor, id string) bool {
	for _, actor := range actors {
		if actor.ID == id {
			return true
		}
	}
	return false
}

func hasEvidence(evidence []domain.Evidence, id string) bool {
	for _, item := range evidence {
		if item.ID == id {
			return true
		}
	}
	return false
}

func hasActualID(actuals []domain.Actual, id string) bool {
	for _, actual := range actuals {
		if actual.ID == id {
			return true
		}
	}
	return false
}

func actualsPath(statusDate string) string {
	t, err := time.Parse("2006-01-02", statusDate)
	if err != nil {
		return "actuals/actuals.unscheduled.yaml"
	}
	year, week := t.ISOWeek()
	return fmt.Sprintf("actuals/actuals.%04d-W%02d.yaml", year, week)
}

func datePart(timestamp string) string {
	if len(timestamp) >= len("2006-01-02") {
		return timestamp[:len("2006-01-02")]
	}
	return timestamp
}

func actualConsumptionID(taskID, resourceID, occurredAt string) string {
	return "actual." + slugID(taskID) + "." + slugID(resourceID) + "." + compactTimestamp(occurredAt) + ".work-consumed"
}

func compactTimestamp(timestamp string) string {
	replacer := strings.NewReplacer("-", "", ":", "", "+", "", "T", "T")
	out := replacer.Replace(timestamp)
	if idx := strings.Index(out, "."); idx >= 0 {
		out = out[:idx]
	}
	if idx := strings.Index(out, "Z"); idx >= 0 {
		out = out[:idx] + "Z"
	}
	return out
}

var slugPattern = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func slugID(id string) string {
	id = strings.TrimPrefix(id, "task.")
	id = strings.TrimPrefix(id, "resource.")
	id = strings.TrimPrefix(id, "resource_demand.")
	slug := strings.Trim(slugPattern.ReplaceAllString(id, "-"), "-")
	if slug == "" {
		return "unknown"
	}
	return strings.ToLower(slug)
}
