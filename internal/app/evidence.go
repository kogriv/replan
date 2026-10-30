package app

import (
	"context"
	"fmt"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type EvidenceService struct {
	loader WorkspaceLoader
}

type EvidenceWriter interface {
	AppendEvidence(observedAt string, evidence domain.Evidence) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
}

type EvidenceAddRequest struct {
	ID         string
	Type       string
	Subtype    string
	Source     string
	URL        string
	ActorID    string
	ObservedAt string
	Confidence string
	Summary    string
	Targets    []domain.Ref
	DryRun     bool
}

type EvidenceAddResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	FilePath           string             `json:"file_path"`
	Evidence           domain.Evidence    `json:"evidence"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

func NewEvidenceService(loader WorkspaceLoader) EvidenceService {
	return EvidenceService{loader: loader}
}

func (s EvidenceService) Add(ctx context.Context, req EvidenceAddRequest) (EvidenceAddResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := EvidenceAddResult{
		DryRun:        req.DryRun,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	validateEvidenceAddRequest(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	item := domain.Evidence{
		ID:         req.ID,
		Type:       req.Type,
		Subtype:    req.Subtype,
		Source:     req.Source,
		URL:        req.URL,
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		ObservedAt: req.ObservedAt,
		Confidence: req.Confidence,
		Summary:    req.Summary,
		Targets:    append([]domain.Ref(nil), req.Targets...),
	}
	path := evidencePath(req.ObservedAt)
	recordYAML, err := yaml.Marshal(struct {
		Evidence []domain.Evidence `yaml:"evidence"`
	}{Evidence: []domain.Evidence{item}})
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "yaml_render_error", Message: fmt.Sprintf("failed to render evidence yaml: %v", err), Severity: "error", Target: Target{Kind: "evidence", ID: req.ID}})
		return result, ErrValidationFailed
	}
	audit := domain.AuditRecord{
		ID:           "audit." + compactTimestamp(req.ObservedAt) + ".evidence-add." + slugID(req.ID),
		OccurredAt:   req.ObservedAt,
		ActorRef:     domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:    "evidence_add",
		TargetRef:    domain.Ref{Kind: "evidence", ID: req.ID},
		Reason:       "Preview evidence record.",
		EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.ID}},
		Result:       "dry_run",
	}
	if !req.DryRun {
		audit.Reason = "Recorded evidence."
		audit.Result = "success"
	}
	result.FilePath = path
	result.Evidence = item
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{Path: path, Action: "append_yaml_record", Format: "yaml", Content: string(recordYAML)})
	result.AuditRecordPreview = audit
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(EvidenceWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "storage_write_not_supported", Message: "configured project store does not support evidence writes", Severity: "error", Target: Target{Kind: "project", ID: s.loader.ProjectPath()}})
		return result, ErrValidationFailed
	}
	evidencePath, err := writer.AppendEvidence(req.ObservedAt, item)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "evidence_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "evidence", ID: req.ID}})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, evidencePath)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "audit_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "audit_record", ID: audit.ID}})
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

func validateEvidenceAddRequest(errors *[]Diagnostic, ws domain.Workspace, req EvidenceAddRequest) {
	if req.ID == "" {
		addEvidenceRequestError(errors, "missing_evidence_id", "evidence id is required", "evidence", "")
	}
	if req.Type == "" {
		addEvidenceRequestError(errors, "missing_evidence_type", "evidence type is required", "evidence", req.ID)
	}
	if req.ActorID == "" {
		addEvidenceRequestError(errors, "missing_actor", "actor is required", "actor", "")
	}
	if req.ObservedAt == "" {
		addEvidenceRequestError(errors, "missing_observed_at", "observed-at is required", "evidence", req.ID)
	} else if _, err := time.Parse(time.RFC3339, req.ObservedAt); err != nil {
		addEvidenceRequestError(errors, "invalid_observed_at", fmt.Sprintf("observed-at %q must be RFC3339 with an offset", req.ObservedAt), "evidence", req.ID)
	}
	if req.Summary == "" {
		addEvidenceRequestError(errors, "missing_summary", "summary is required", "evidence", req.ID)
	}
	if len(req.Targets) == 0 {
		addEvidenceRequestError(errors, "missing_target", "at least one target is required", "evidence", req.ID)
	}
	if req.ID != "" && hasEvidence(ws.Evidence, req.ID) {
		addEvidenceRequestError(errors, "evidence_already_exists", fmt.Sprintf("evidence %s already exists", req.ID), "evidence", req.ID)
	}
	if req.ActorID != "" && !hasActor(ws.Actors, req.ActorID) {
		addEvidenceRequestError(errors, "actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID)
	}
	for _, target := range req.Targets {
		if !validEvidenceTargetKind(target.Kind) {
			addEvidenceRequestError(errors, "invalid_evidence_target_kind", fmt.Sprintf("unsupported evidence target kind %q", target.Kind), target.Kind, target.ID)
			continue
		}
		if target.ID == "" {
			addEvidenceRequestError(errors, "missing_evidence_target_id", fmt.Sprintf("evidence target %s requires an id", target.Kind), target.Kind, "")
			continue
		}
		if !evidenceTargetExists(ws, target) {
			addEvidenceRequestError(errors, "evidence_target_not_found", fmt.Sprintf("%s %s not found", target.Kind, target.ID), target.Kind, target.ID)
		}
	}
}

func addEvidenceRequestError(errors *[]Diagnostic, code, message, kind, id string) {
	*errors = append(*errors, Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}})
}

func validEvidenceTargetKind(kind string) bool {
	switch kind {
	case "project", "task", "resource", "resource_demand", "actual", "proposal", "dependency", "scenario", "capacity", "baseline", "audit_record":
		return true
	default:
		return false
	}
}

func evidenceTargetExists(ws domain.Workspace, ref domain.Ref) bool {
	switch ref.Kind {
	case "project":
		return ws.Project.ID == ref.ID
	case "task":
		_, ok := findTask(ws.Tasks, ref.ID)
		return ok
	case "resource":
		return hasResource(ws.Resources, ref.ID)
	case "resource_demand":
		_, ok := findDemand(ws.ResourceDemands, ref.ID)
		return ok
	case "actual":
		return hasID(ws.Actuals, ref.ID, func(item domain.Actual) string { return item.ID })
	case "proposal":
		return hasID(ws.Proposals, ref.ID, func(item domain.Proposal) string { return item.ID })
	case "dependency":
		return hasID(ws.Dependencies, ref.ID, func(item domain.Dependency) string { return item.ID })
	case "scenario":
		return hasID(ws.Scenarios, ref.ID, func(item domain.Scenario) string { return item.ID })
	case "capacity":
		return hasID(ws.Capacities, ref.ID, func(item domain.Capacity) string { return item.ID })
	case "baseline":
		return hasID(ws.Baselines, ref.ID, func(item domain.Baseline) string { return item.ID })
	case "audit_record":
		return hasID(ws.AuditRecords, ref.ID, func(item domain.AuditRecord) string { return item.ID })
	default:
		return false
	}
}

func hasID[T any](items []T, id string, itemID func(T) string) bool {
	for _, item := range items {
		if itemID(item) == id {
			return true
		}
	}
	return false
}
