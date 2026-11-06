package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type ResourceNameWriter interface {
	SetResourceName(resourceID, name string) (string, error)
	AppendAudit(record *domain.AuditRecord) (string, error)
}

type ActorNameWriter interface {
	SetActorDisplayName(actorID, name string) (string, error)
	AppendAudit(record *domain.AuditRecord) (string, error)
}

type ResourceSetNameRequest struct {
	ResourceID, Name, Reason, ActorID, EvidenceID, OccurredAt string
	DryRun                                                    bool
}

type SetNameResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	NoOp               bool               `json:"no_op"`
	PreviousName       string             `json:"previous_name"`
	NewName            string             `json:"new_name"`
	Reason             string             `json:"reason"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview,omitempty"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ResourceSetNameResult struct {
	SetNameResult
	ResourceID string `json:"resource_id"`
}

type ActorSetNameRequest struct {
	TargetActorID, Name, Reason, ActorID, EvidenceID, OccurredAt string
	DryRun                                                       bool
}

type ActorSetNameResult struct {
	SetNameResult
	ActorID string `json:"actor_id"`
}

type ActorService struct{ loader WorkspaceLoader }

func NewActorService(loader WorkspaceLoader) ActorService { return ActorService{loader: loader} }

func (s ResourceService) SetName(ctx context.Context, req ResourceSetNameRequest) (ResourceSetNameResult, error) {
	result, err := setCanonicalName(ctx, s.loader, setCanonicalNameRequest{TargetID: req.ResourceID, Kind: "resource", Name: req.Name, Reason: req.Reason, ActorID: req.ActorID, EvidenceID: req.EvidenceID, OccurredAt: req.OccurredAt, DryRun: req.DryRun})
	return ResourceSetNameResult{SetNameResult: result, ResourceID: req.ResourceID}, err
}

func (s ActorService) SetName(ctx context.Context, req ActorSetNameRequest) (ActorSetNameResult, error) {
	result, err := setCanonicalName(ctx, s.loader, setCanonicalNameRequest{TargetID: req.TargetActorID, Kind: "actor", Name: req.Name, Reason: req.Reason, ActorID: req.ActorID, EvidenceID: req.EvidenceID, OccurredAt: req.OccurredAt, DryRun: req.DryRun})
	return ActorSetNameResult{SetNameResult: result, ActorID: req.TargetActorID}, err
}

type setCanonicalNameRequest struct {
	TargetID, Kind, Name, Reason, ActorID, EvidenceID, OccurredAt string
	DryRun                                                        bool
}

func setCanonicalName(ctx context.Context, loader WorkspaceLoader, req setCanonicalNameRequest) (SetNameResult, error) {
	ws, diags := loader.Load(ctx)
	result := SetNameResult{DryRun: req.DryRun, NewName: req.Name, Reason: req.Reason, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, diags)
	oldName, exists := canonicalName(ws, req.Kind, req.TargetID)
	validateSetName(&result.Errors, ws, req.TargetID, req.Kind, exists, req.Name, req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	result.PreviousName = oldName
	if oldName == req.Name {
		result.NoOp = true
		return result, nil
	}

	audit := nameChangeAudit(req.Kind, req.TargetID, oldName, req.Name, req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt, req.DryRun)
	result.AuditRecordPreview = audit
	field, path := "name", "canonical/resources.yaml"
	if req.Kind == "actor" {
		field, path = "display_name", "canonical/actors.yaml"
	}
	patch, _ := yaml.Marshal(map[string]any{req.Kind: req.TargetID, field: map[string]string{"before": oldName, "after": req.Name}, "reason": req.Reason})
	result.PlannedWrites = []PlannedWrite{{Path: path, Action: "replace_yaml_field", Format: "yaml-patch-preview", Content: string(patch)}, auditPlannedWrite(audit)}
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(loader)
	writtenPath, appender, supported, err := writeCanonicalName(session.target, req.Kind, req.TargetID, req.Name)
	if !supported {
		result.Errors = append(result.Errors, namingDiag("storage_write_not_supported", fmt.Sprintf("configured project store does not support %s name writes", req.Kind), "project", loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	if err != nil {
		result.Errors = append(result.Errors, namingDiag(req.Kind+"_write_failed", err.Error(), req.Kind, req.TargetID))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, writtenPath)
	auditPath, err := appender.AppendAudit(&audit)
	result.AuditRecordPreview = audit
	if err != nil {
		result.Errors = append(result.Errors, namingDiag("audit_write_failed", err.Error(), "audit_record", audit.ID))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
	validation, committed, err := session.finish(ctx, result.WrittenFiles)
	result.Validation, result.WrittenFiles = &validation, committed
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

type auditAppender interface {
	AppendAudit(record *domain.AuditRecord) (string, error)
}

func canonicalName(ws domain.Workspace, kind, id string) (string, bool) {
	if kind == "resource" {
		resource, ok := findResource(ws.Resources, id)
		return resource.Name, ok
	}
	actor, ok := findActor(ws.Actors, id)
	return actor.DisplayName, ok
}

func writeCanonicalName(loader WorkspaceLoader, kind, id, name string) (string, auditAppender, bool, error) {
	if kind == "resource" {
		writer, ok := loader.(ResourceNameWriter)
		if !ok {
			return "", nil, false, nil
		}
		path, err := writer.SetResourceName(id, name)
		return path, writer, true, err
	}
	writer, ok := loader.(ActorNameWriter)
	if !ok {
		return "", nil, false, nil
	}
	path, err := writer.SetActorDisplayName(id, name)
	return path, writer, true, err
}

func validateSetName(errors *[]Diagnostic, ws domain.Workspace, targetID, targetKind string, targetExists bool, name, reason, actorID, evidenceID, occurredAt string) {
	if strings.TrimSpace(targetID) == "" {
		*errors = append(*errors, namingDiag("missing_"+targetKind, targetKind+" is required", targetKind, ""))
	} else if !targetExists {
		*errors = append(*errors, namingDiag(targetKind+"_not_found", fmt.Sprintf("%s %s not found", targetKind, targetID), targetKind, targetID))
	}
	if strings.TrimSpace(name) == "" {
		*errors = append(*errors, namingDiag("missing_"+targetKind+"_name", "name must not be empty or whitespace", targetKind, targetID))
	}
	if strings.TrimSpace(reason) == "" {
		*errors = append(*errors, namingDiag("missing_reason", "reason is required", targetKind, targetID))
	}
	if actorID == "" {
		*errors = append(*errors, namingDiag("missing_actor", "actor is required", "actor", ""))
	} else if !hasActor(ws.Actors, actorID) {
		*errors = append(*errors, namingDiag("actor_not_found", fmt.Sprintf("actor %s not found", actorID), "actor", actorID))
	}
	if evidenceID == "" {
		*errors = append(*errors, namingDiag("missing_evidence", "evidence is required", "evidence", ""))
	} else if !hasEvidence(ws.Evidence, evidenceID) {
		*errors = append(*errors, namingDiag("evidence_not_found", fmt.Sprintf("evidence %s not found", evidenceID), "evidence", evidenceID))
	}
	if occurredAt != "" {
		if _, err := time.Parse(time.RFC3339, occurredAt); err != nil {
			*errors = append(*errors, namingDiag("invalid_occurred_at", fmt.Sprintf("occurred-at %q must be RFC3339 with an offset", occurredAt), targetKind, targetID))
		}
	}
}

func nameChangeAudit(kind, id, oldName, newName, reason, actorID, evidenceID, occurredAt string, dryRun bool) domain.AuditRecord {
	if occurredAt == "" {
		occurredAt = time.Now().Format(time.RFC3339)
	}
	label := "Resource name"
	operation := "resource_set_name"
	if kind == "actor" {
		label = "Actor display name"
		operation = "actor_set_name"
	}
	auditReason := fmt.Sprintf("%s. %s: %q -> %q.", strings.TrimRight(strings.TrimSpace(reason), "."), label, oldName, newName)
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + "." + strings.ReplaceAll(operation, "_", "-") + "." + slugID(id), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: actorID}, Operation: operation, TargetRef: domain.Ref{Kind: kind, ID: id}, Reason: auditReason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: evidenceID}}, Result: "success"}
	if dryRun {
		audit.Result = "dry_run"
	}
	return audit
}

func namingDiag(code, message, kind, id string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}}
}
