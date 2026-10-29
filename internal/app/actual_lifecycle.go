package app

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

// DemandLifecycleWriter covers demand lifecycle writes that have no older
// home in ActualWriter.
type DemandLifecycleWriter interface {
	SetResourceDemandStatus(demandID string, status string) (string, error)
	SetResourceDemandRemaining(demandID string, amount float64) (string, error)
	SetResourceDemandOverrunReview(demandID string, review domain.OverrunReview) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
}

// ActualReopenDemandRequest returns a consumed or cancelled demand to active.
// Remaining optionally records the work still expected; nil leaves the
// recorded remaining amount unchanged.
type ActualReopenDemandRequest struct {
	DemandID   string
	Remaining  *float64
	Reason     string
	EvidenceID string
	ActorID    string
	OccurredAt string
	DryRun     bool
	Now        time.Time
}

type ActualReopenDemandResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	DemandID           string             `json:"demand_id"`
	TaskID             string             `json:"task_id,omitempty"`
	PreviousStatus     string             `json:"previous_status,omitempty"`
	NewStatus          string             `json:"new_status"`
	PreviousRemaining  *float64           `json:"previous_remaining,omitempty"`
	NewRemaining       *float64           `json:"new_remaining,omitempty"`
	Reason             string             `json:"reason,omitempty"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

// ActualAcknowledgeOverrunRequest records that an operator reviewed a
// forecast above plan and accepts it as real.
type ActualAcknowledgeOverrunRequest struct {
	DemandID   string
	Reason     string
	EvidenceID string
	ActorID    string
	OccurredAt string
	DryRun     bool
	Now        time.Time
}

type ActualAcknowledgeOverrunResult struct {
	DryRun             bool                  `json:"dry_run"`
	Applied            bool                  `json:"applied"`
	DemandID           string                `json:"demand_id"`
	TaskID             string                `json:"task_id,omitempty"`
	Planned            float64               `json:"planned"`
	Actual             float64               `json:"actual"`
	Remaining          float64               `json:"remaining"`
	Forecast           float64               `json:"forecast"`
	Variance           float64               `json:"variance"`
	Review             *domain.OverrunReview `json:"review,omitempty"`
	PreviousReview     *domain.OverrunReview `json:"previous_review,omitempty"`
	PlannedWrites      []PlannedWrite        `json:"planned_writes"`
	WrittenFiles       []string              `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord    `json:"audit_record_preview"`
	Validation         *ValidateResult       `json:"validation,omitempty"`
	Warnings           []Diagnostic          `json:"warnings"`
	Errors             []Diagnostic          `json:"errors"`
}

func (s ActualService) ReopenDemand(ctx context.Context, req ActualReopenDemandRequest) (ActualReopenDemandResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualReopenDemandResult{
		DryRun:        req.DryRun,
		DemandID:      req.DemandID,
		NewStatus:     "active",
		NewRemaining:  req.Remaining,
		Reason:        req.Reason,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	demand, ok := validateDemandLifecycleRequest(&result.Errors, ws, req.DemandID, req.Reason, req.ActorID, req.EvidenceID)
	if ok {
		result.TaskID = demand.TaskRef.ID
		result.PreviousStatus = demand.Status
		result.PreviousRemaining = demand.RemainingAmount
		if demand.Status != "consumed" && demand.Status != "cancelled" {
			addActualRequestError(&result.Errors, "demand_not_closed", fmt.Sprintf("resource demand %s has status %q; only consumed or cancelled demands can be reopened", demand.ID, demand.Status), "resource_demand", demand.ID)
		}
	}
	if req.Remaining != nil && !validAmount(*req.Remaining) {
		addActualRequestError(&result.Errors, "invalid_remaining", "remaining must be a finite number, zero or greater", "resource_demand", req.DemandID)
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	occurredAt := lifecycleOccurredAt(req.OccurredAt, req.Now)
	audit := lifecycleAudit(occurredAt, "actual-reopen-demand", "actual_reopen_demand", req.DemandID, req.ActorID, req.Reason, req.EvidenceID, req.DryRun)
	result.AuditRecordPreview = audit
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
		Path:    "canonical/resource_demands.yaml",
		Action:  "replace_yaml_field",
		Format:  "yaml-patch-preview",
		Content: fmt.Sprintf("op: replace\npath: canonical/resource_demands/%s/status\nbefore: %s\nvalue: active\n", req.DemandID, demand.Status),
	})
	if req.Remaining != nil {
		result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
			Path:    "canonical/resource_demands.yaml",
			Action:  "replace_yaml_field",
			Format:  "yaml-patch-preview",
			Content: fmt.Sprintf("op: replace\npath: canonical/resource_demands/%s/remaining_amount\nvalue: %g\n", req.DemandID, *req.Remaining),
		})
	}
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(DemandLifecycleWriter)
	if !ok {
		return result, lifecycleWriteUnsupported(&result.Errors, s.loader.ProjectPath())
	}
	collected := []string{}
	path, err := writer.SetResourceDemandStatus(req.DemandID, "active")
	if err != nil {
		return result, lifecycleWriteFailed(&result.Errors, "demand_status_write_failed", err, "resource_demand", req.DemandID)
	}
	collected = append(collected, path)
	if req.Remaining != nil {
		if _, err := writer.SetResourceDemandRemaining(req.DemandID, *req.Remaining); err != nil {
			return result, lifecycleWriteFailed(&result.Errors, "demand_remaining_write_failed", err, "resource_demand", req.DemandID)
		}
	}
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		return result, lifecycleWriteFailed(&result.Errors, "audit_write_failed", err, "audit_record", audit.ID)
	}
	collected = append(collected, auditPath)
	validation, written, err := session.finish(ctx, collected)
	result.Validation = &validation
	result.WrittenFiles = written
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

func (s ActualService) AcknowledgeOverrun(ctx context.Context, req ActualAcknowledgeOverrunRequest) (ActualAcknowledgeOverrunResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ActualAcknowledgeOverrunResult{
		DryRun:        req.DryRun,
		DemandID:      req.DemandID,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	demand, ok := validateDemandLifecycleRequest(&result.Errors, ws, req.DemandID, req.Reason, req.ActorID, req.EvidenceID)
	if ok {
		result.TaskID = demand.TaskRef.ID
		result.PreviousReview = demand.OverrunReview
		if demand.Status == "cancelled" {
			addActualRequestError(&result.Errors, "demand_cancelled", fmt.Sprintf("resource demand %s is cancelled; a cancelled demand has no overrun to acknowledge", demand.ID), "resource_demand", demand.ID)
		}
		tracking, found := demandTrackingFor(ws, demand.ID)
		if !found {
			addActualRequestError(&result.Errors, "demand_tracking_not_found", fmt.Sprintf("resource demand %s has no tracking row; its task may be missing from the current plan", demand.ID), "resource_demand", demand.ID)
		} else {
			result.Planned = tracking.Planned
			result.Actual = tracking.Actual
			result.Remaining = tracking.Remaining
			result.Forecast = tracking.Forecast
			result.Variance = tracking.Variance
			if tracking.Variance <= 0 {
				addActualRequestError(&result.Errors, "no_overrun", fmt.Sprintf("resource demand %s forecast %g does not exceed plan %g; there is nothing to acknowledge", demand.ID, tracking.Forecast, tracking.Planned), "resource_demand", demand.ID)
			}
			if demand.OverrunReview != nil && sameAmount(demand.OverrunReview.AcknowledgedForecast, tracking.Forecast) {
				addActualRequestError(&result.Errors, "overrun_already_acknowledged", fmt.Sprintf("resource demand %s forecast %g is already acknowledged", demand.ID, tracking.Forecast), "resource_demand", demand.ID)
			}
		}
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	occurredAt := lifecycleOccurredAt(req.OccurredAt, req.Now)
	review := domain.OverrunReview{
		AcknowledgedForecast: result.Forecast,
		Unit:                 demand.Unit,
		Reason:               req.Reason,
		ReviewedBy:           domain.Ref{Kind: "actor", ID: req.ActorID},
		ReviewedAt:           occurredAt,
	}
	if req.EvidenceID != "" {
		review.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}
	result.Review = &review
	audit := lifecycleAudit(occurredAt, "actual-acknowledge-overrun", "actual_acknowledge_overrun", req.DemandID, req.ActorID, req.Reason, req.EvidenceID, req.DryRun)
	result.AuditRecordPreview = audit
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
		Path:    "canonical/resource_demands.yaml",
		Action:  "replace_yaml_field",
		Format:  "yaml-patch-preview",
		Content: fmt.Sprintf("op: replace\npath: canonical/resource_demands/%s/overrun_review\nvalue:\n  acknowledged_forecast: %g\n  unit: %s\n", req.DemandID, review.AcknowledgedForecast, review.Unit),
	})
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(DemandLifecycleWriter)
	if !ok {
		return result, lifecycleWriteUnsupported(&result.Errors, s.loader.ProjectPath())
	}
	path, err := writer.SetResourceDemandOverrunReview(req.DemandID, review)
	if err != nil {
		return result, lifecycleWriteFailed(&result.Errors, "demand_overrun_review_write_failed", err, "resource_demand", req.DemandID)
	}
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		return result, lifecycleWriteFailed(&result.Errors, "audit_write_failed", err, "audit_record", audit.ID)
	}
	validation, written, err := session.finish(ctx, []string{path, auditPath})
	result.Validation = &validation
	result.WrittenFiles = written
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

func validateDemandLifecycleRequest(errors *[]Diagnostic, ws domain.Workspace, demandID, reason, actorID, evidenceID string) (domain.ResourceDemand, bool) {
	if demandID == "" {
		addActualRequestError(errors, "missing_demand", "demand is required", "resource_demand", "")
	}
	if reason == "" {
		addActualRequestError(errors, "missing_reason", "reason is required", "resource_demand", demandID)
	}
	if actorID == "" {
		addActualRequestError(errors, "missing_actor", "actor is required", "actor", "")
	}
	demand, demandOK := findDemand(ws.ResourceDemands, demandID)
	if demandID != "" && !demandOK {
		addActualRequestError(errors, "demand_not_found", fmt.Sprintf("resource demand %s not found", demandID), "resource_demand", demandID)
	}
	if actorID != "" && !hasActor(ws.Actors, actorID) {
		addActualRequestError(errors, "actor_not_found", fmt.Sprintf("actor %s not found", actorID), "actor", actorID)
	}
	if evidenceID != "" && !hasEvidence(ws.Evidence, evidenceID) {
		addActualRequestError(errors, "evidence_not_found", fmt.Sprintf("evidence %s not found", evidenceID), "evidence", evidenceID)
	}
	return demand, demandOK
}

func demandTrackingFor(ws domain.Workspace, demandID string) (demandTrackingRow, bool) {
	for _, row := range dashboardTrackingRows(ws) {
		for _, demand := range row.Demands {
			if demand.DemandID == demandID {
				return demandTrackingRow{
					Planned:   demand.Planned,
					Actual:    demand.Actual,
					Remaining: demand.Remaining,
					Forecast:  demand.Forecast,
					Variance:  demand.Variance,
				}, true
			}
		}
	}
	return demandTrackingRow{}, false
}

type demandTrackingRow struct {
	Planned, Actual, Remaining, Forecast, Variance float64
}

// overrunAcknowledged reports whether a reviewed overrun still covers the
// current forecast.
func overrunAcknowledged(review *domain.OverrunReview, unit string, forecast float64) bool {
	return review != nil && review.Unit == unit && forecast <= review.AcknowledgedForecast+amountEpsilon
}

const amountEpsilon = 1e-9

func sameAmount(a, b float64) bool {
	return math.Abs(a-b) <= amountEpsilon
}

func lifecycleOccurredAt(occurredAt string, now time.Time) string {
	if occurredAt != "" {
		return occurredAt
	}
	if now.IsZero() {
		now = time.Now()
	}
	return now.Format(time.RFC3339)
}

func lifecycleAudit(occurredAt, idKind, operation, demandID, actorID, reason, evidenceID string, dryRun bool) domain.AuditRecord {
	audit := domain.AuditRecord{
		ID:         "audit." + compactTimestamp(occurredAt) + "." + idKind + "." + slugID(demandID),
		OccurredAt: occurredAt,
		ActorRef:   domain.Ref{Kind: "actor", ID: actorID},
		Operation:  operation,
		TargetRef:  domain.Ref{Kind: "resource_demand", ID: demandID},
		Reason:     reason,
		Result:     "success",
	}
	if dryRun {
		audit.Result = "dry_run"
	}
	if evidenceID != "" {
		audit.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: evidenceID}}
	}
	return audit
}

func lifecycleWriteUnsupported(errors *[]Diagnostic, projectPath string) error {
	*errors = append(*errors, Diagnostic{
		Code:     "storage_write_not_supported",
		Message:  "configured project store does not support demand lifecycle writes",
		Severity: "error",
		Target:   Target{Kind: "project", ID: projectPath},
	})
	return ErrValidationFailed
}

func lifecycleWriteFailed(errors *[]Diagnostic, code string, err error, kind, id string) error {
	*errors = append(*errors, Diagnostic{
		Code:     code,
		Message:  err.Error(),
		Severity: "error",
		Target:   Target{Kind: kind, ID: id},
	})
	return err
}
