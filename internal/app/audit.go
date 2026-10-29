package app

import (
	"context"
	"sort"

	"github.com/kogriv/replan/internal/domain"
)

type AuditService struct {
	loader WorkspaceLoader
}

type AuditListRequest struct {
	Since string
}

type AuditVerifyRequest struct{}

type AuditListResult struct {
	Records  []AuditRecordSummary `json:"records"`
	Warnings []Diagnostic         `json:"warnings"`
	Errors   []Diagnostic         `json:"errors"`
}

type AuditRecordSummary struct {
	ID         string `json:"id"`
	OccurredAt string `json:"occurred_at"`
	ActorID    string `json:"actor_id"`
	Operation  string `json:"operation"`
	TargetKind string `json:"target_kind,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
	Result     string `json:"result,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type AuditVerifyResult struct {
	Valid    bool         `json:"valid"`
	Records  int          `json:"records"`
	Warnings []Diagnostic `json:"warnings"`
	Errors   []Diagnostic `json:"errors"`
}

func NewAuditService(loader WorkspaceLoader) AuditService {
	return AuditService{loader: loader}
}

func (s AuditService) List(ctx context.Context, req AuditListRequest) (AuditListResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := AuditListResult{
		Records:  []AuditRecordSummary{},
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	for _, record := range ws.AuditRecords {
		if req.Since != "" && record.OccurredAt < req.Since {
			continue
		}
		result.Records = append(result.Records, auditRecordSummary(record))
	}
	sort.Slice(result.Records, func(i, j int) bool {
		return result.Records[i].OccurredAt < result.Records[j].OccurredAt
	})
	return result, nil
}

func (s AuditService) Verify(ctx context.Context, _ AuditVerifyRequest) (AuditVerifyResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := AuditVerifyResult{
		Valid:    true,
		Records:  len(ws.AuditRecords),
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	for _, record := range ws.AuditRecords {
		if record.ID == "" {
			result.Errors = append(result.Errors, Diagnostic{
				Code:     "missing_audit_id",
				Message:  "audit record id is required",
				Severity: "error",
				Target:   Target{Kind: "audit_record"},
			})
		}
		if record.ActorRef.ID == "" {
			result.Errors = append(result.Errors, Diagnostic{
				Code:     "missing_audit_actor",
				Message:  "audit actor_ref is required",
				Severity: "error",
				Target:   Target{Kind: "audit_record", ID: record.ID},
			})
		}
		if record.Operation == "" {
			result.Errors = append(result.Errors, Diagnostic{
				Code:     "missing_audit_operation",
				Message:  "audit operation is required",
				Severity: "error",
				Target:   Target{Kind: "audit_record", ID: record.ID},
			})
		}
	}
	result.Valid = len(result.Errors) == 0
	if !result.Valid {
		return result, ErrValidationFailed
	}
	return result, nil
}

func auditRecordSummary(record domain.AuditRecord) AuditRecordSummary {
	return AuditRecordSummary{
		ID:         record.ID,
		OccurredAt: record.OccurredAt,
		ActorID:    record.ActorRef.ID,
		Operation:  record.Operation,
		TargetKind: record.TargetRef.Kind,
		TargetID:   record.TargetRef.ID,
		Result:     record.Result,
		Reason:     record.Reason,
	}
}
