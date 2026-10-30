package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/yamlformat"
)

// ResourceClasses is the set of first-class resource classes accepted by the
// onboarding CLI. role is retained because it is used by the self-dogfood
// project as a schedulable reviewer resource.
var ResourceClasses = []string{
	"human", "ai_agent", "role", "service_quota", "rate_limit",
	"subscription_entitlement", "budget", "equipment", "environment",
	"material", "location", "vehicle", "robot", "vendor", "approval", "generic",
}

var resourceTrustLevels = map[string]bool{
	"owner": true, "trusted": true, "limited": true, "untrusted": true, "external": true,
}

type ResourceWriter interface {
	AddResource(resource domain.Resource, actor *domain.Actor, audit domain.AuditRecord) ([]string, error)
}

type CapacityWriter interface {
	SetCapacity(previousID, previousEffectiveTo string, capacity domain.Capacity, audit domain.AuditRecord) ([]string, error)
}

type ResourceAddRequest struct {
	ID, Class, Name, WithActor, ActorID, EvidenceID, OccurredAt string
	Capabilities                                                []string
	DryRun                                                      bool
}

type ResourceAddResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	Resource           domain.Resource    `json:"resource"`
	Actor              *domain.Actor      `json:"actor,omitempty"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ResourceListResult struct {
	Resources []domain.Resource `json:"resources"`
	Warnings  []Diagnostic      `json:"warnings"`
	Errors    []Diagnostic      `json:"errors"`
}

type CapacitySetRequest struct {
	ResourceID, Unit, Period, EffectiveFrom, ActorID, EvidenceID, OccurredAt string
	Amount                                                                   float64
	DryRun                                                                   bool
}

type CapacitySetResult struct {
	DryRun              bool               `json:"dry_run"`
	Applied             bool               `json:"applied"`
	Capacity            domain.Capacity    `json:"capacity"`
	ClosedCapacityID    string             `json:"closed_capacity_id,omitempty"`
	PreviousEffectiveTo string             `json:"previous_effective_to,omitempty"`
	PlannedWrites       []PlannedWrite     `json:"planned_writes"`
	WrittenFiles        []string           `json:"written_files,omitempty"`
	AuditRecordPreview  domain.AuditRecord `json:"audit_record_preview"`
	Validation          *ValidateResult    `json:"validation,omitempty"`
	Warnings            []Diagnostic       `json:"warnings"`
	Errors              []Diagnostic       `json:"errors"`
}

type ResourceService struct{ loader WorkspaceLoader }

func NewResourceService(loader WorkspaceLoader) ResourceService {
	return ResourceService{loader: loader}
}

func (s ResourceService) List(ctx context.Context) (ResourceListResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := ResourceListResult{Resources: append([]domain.Resource(nil), ws.Resources...), Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, diags)
	sort.Slice(result.Resources, func(i, j int) bool { return result.Resources[i].ID < result.Resources[j].ID })
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	return result, nil
}

func (s ResourceService) Add(ctx context.Context, req ResourceAddRequest) (ResourceAddResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := ResourceAddResult{DryRun: req.DryRun, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, diags)
	validateResourceAdd(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	resource := domain.Resource{ID: req.ID, Class: req.Class, Name: req.Name, Capabilities: append([]string(nil), req.Capabilities...)}
	var actor *domain.Actor
	if req.WithActor != "" {
		actorID := "actor." + strings.TrimPrefix(req.ID, "resource.")
		created := domain.Actor{ID: actorID, Kind: req.Class, DisplayName: req.Name, TrustLevel: req.WithActor, ResourceRef: domain.Ref{Kind: "resource", ID: req.ID}}
		actor = &created
		resource.ActorRef = domain.Ref{Kind: "actor", ID: actorID}
	}
	result.Resource, result.Actor = resource, actor
	occurredAt := onboardingOccurredAt(req.OccurredAt)
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".resource-add." + slugID(req.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "resource_add", TargetRef: domain.Ref{Kind: "resource", ID: req.ID}, Result: "success", Reason: "Created resource."}
	if req.EvidenceID != "" {
		audit.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}
	if req.DryRun {
		audit.Result, audit.Reason = "dry_run", "Preview resource creation."
	}
	result.AuditRecordPreview = audit
	plannedWrites, err := resourceAddPlannedWrites(resource, actor, audit)
	if err != nil {
		result.Errors = append(result.Errors, onboardingDiag("resource_render_failed", err.Error(), "resource", req.ID))
		return result, err
	}
	result.PlannedWrites = plannedWrites
	if req.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, ok := session.target.(ResourceWriter)
	if !ok {
		result.Errors = append(result.Errors, onboardingDiag("storage_write_not_supported", "configured project store does not support resource writes", "project", s.loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	written, err := writer.AddResource(resource, actor, audit)
	if err != nil {
		result.Errors = append(result.Errors, onboardingDiag("resource_write_failed", err.Error(), "resource", req.ID))
		return result, err
	}
	validation, committed, err := session.finish(ctx, written)
	result.Validation, result.WrittenFiles = &validation, committed
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

func (s ResourceService) SetCapacity(ctx context.Context, req CapacitySetRequest) (CapacitySetResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := CapacitySetResult{DryRun: req.DryRun, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, diags)
	resource, previous, effectiveFrom := validateCapacitySet(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	capacity := domain.Capacity{
		ID:          "capacity." + slugID(req.ResourceID) + "." + slugID(req.Unit) + "." + strings.ReplaceAll(effectiveFrom, "-", ""),
		ResourceRef: domain.Ref{Kind: "resource", ID: req.ResourceID}, ResourceClass: resource.Class,
		Type: "periodic_capacity", Unit: req.Unit, Amount: req.Amount, Period: req.Period,
		EffectiveFrom: effectiveFrom, Timezone: ws.Project.Timezone,
	}
	previousTo := ""
	if previous != nil {
		day, _ := time.Parse("2006-01-02", effectiveFrom)
		previousTo = day.AddDate(0, 0, -1).Format("2006-01-02")
		result.ClosedCapacityID, result.PreviousEffectiveTo = previous.ID, previousTo
	}
	result.Capacity = capacity
	occurredAt := onboardingOccurredAt(req.OccurredAt)
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".capacity-set." + slugID(capacity.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "capacity_set", TargetRef: domain.Ref{Kind: "capacity", ID: capacity.ID}, Result: "success", Reason: "Set periodic resource capacity."}
	if req.EvidenceID != "" {
		audit.EvidenceRefs = []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}
	}
	if req.DryRun {
		audit.Result, audit.Reason = "dry_run", "Preview periodic resource capacity."
	}
	result.AuditRecordPreview = audit
	plannedWrites, err := capacitySetPlannedWrites(capacity, previous, previousTo, audit)
	if err != nil {
		result.Errors = append(result.Errors, onboardingDiag("capacity_render_failed", err.Error(), "capacity", capacity.ID))
		return result, err
	}
	result.PlannedWrites = plannedWrites
	if req.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, ok := session.target.(CapacityWriter)
	if !ok {
		result.Errors = append(result.Errors, onboardingDiag("storage_write_not_supported", "configured project store does not support capacity writes", "project", s.loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	previousID := ""
	if previous != nil {
		previousID = previous.ID
	}
	written, err := writer.SetCapacity(previousID, previousTo, capacity, audit)
	if err != nil {
		result.Errors = append(result.Errors, onboardingDiag("capacity_write_failed", err.Error(), "capacity", capacity.ID))
		return result, err
	}
	validation, committed, err := session.finish(ctx, written)
	result.Validation, result.WrittenFiles = &validation, committed
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

func validateResourceAdd(errors *[]Diagnostic, ws domain.Workspace, req ResourceAddRequest) {
	if strings.TrimSpace(req.ID) == "" {
		*errors = append(*errors, onboardingDiag("missing_resource_id", "resource id is required", "resource", ""))
	}
	if strings.TrimSpace(req.Name) == "" {
		*errors = append(*errors, onboardingDiag("missing_resource_name", "resource name is required", "resource", req.ID))
	}
	if !containsString(ResourceClasses, req.Class) {
		*errors = append(*errors, onboardingDiag("invalid_resource_class", fmt.Sprintf("resource class %q is not supported; expected one of %s", req.Class, strings.Join(ResourceClasses, ", ")), "resource", req.ID))
	}
	for _, resource := range ws.Resources {
		if resource.ID == req.ID {
			*errors = append(*errors, onboardingDiag("duplicate_resource_id", fmt.Sprintf("resource %s already exists", req.ID), "resource", req.ID))
		}
	}
	if !hasActor(ws.Actors, req.ActorID) {
		*errors = append(*errors, onboardingDiag("actor_not_found", fmt.Sprintf("actor %s does not exist", req.ActorID), "actor", req.ActorID))
	}
	if req.EvidenceID != "" && !hasEvidence(ws.Evidence, req.EvidenceID) {
		*errors = append(*errors, onboardingDiag("evidence_not_found", fmt.Sprintf("evidence %s does not exist", req.EvidenceID), "evidence", req.EvidenceID))
	}
	if req.WithActor != "" {
		if req.Class != "human" && req.Class != "ai_agent" {
			*errors = append(*errors, onboardingDiag("actor_not_allowed_for_resource_class", "--with-actor is allowed only for human and ai_agent resources", "resource", req.ID))
		}
		if !resourceTrustLevels[req.WithActor] {
			*errors = append(*errors, onboardingDiag("invalid_actor_trust_level", fmt.Sprintf("invalid actor trust level %q", req.WithActor), "actor", ""))
		}
		actorID := "actor." + strings.TrimPrefix(req.ID, "resource.")
		if hasActor(ws.Actors, actorID) {
			*errors = append(*errors, onboardingDiag("duplicate_actor_id", fmt.Sprintf("actor %s already exists", actorID), "actor", actorID))
		}
	}
}

func validateCapacitySet(errors *[]Diagnostic, ws domain.Workspace, req CapacitySetRequest) (domain.Resource, *domain.Capacity, string) {
	resource, ok := findResource(ws.Resources, req.ResourceID)
	if !ok {
		*errors = append(*errors, onboardingDiag("resource_not_found", fmt.Sprintf("resource %s does not exist", req.ResourceID), "resource", req.ResourceID))
	}
	if strings.TrimSpace(req.Unit) == "" {
		*errors = append(*errors, onboardingDiag("missing_capacity_unit", "capacity unit is required", "capacity", ""))
	}
	if math.IsNaN(req.Amount) || math.IsInf(req.Amount, 0) || req.Amount < 0 {
		*errors = append(*errors, onboardingDiag("invalid_capacity_amount", "capacity amount must be a finite number, zero or greater", "capacity", ""))
	}
	if req.Period != "week" {
		*errors = append(*errors, onboardingDiag("invalid_capacity_period", "capacity period must be week", "capacity", ""))
	}
	if !hasActor(ws.Actors, req.ActorID) {
		*errors = append(*errors, onboardingDiag("actor_not_found", fmt.Sprintf("actor %s does not exist", req.ActorID), "actor", req.ActorID))
	}
	if req.EvidenceID != "" && !hasEvidence(ws.Evidence, req.EvidenceID) {
		*errors = append(*errors, onboardingDiag("evidence_not_found", fmt.Sprintf("evidence %s does not exist", req.EvidenceID), "evidence", req.EvidenceID))
	}
	effectiveFrom := req.EffectiveFrom
	if effectiveFrom == "" {
		effectiveFrom = ws.Project.DefaultStatusDate
		if effectiveFrom == "" {
			effectiveFrom = todayInTimezone(ws.Project.Timezone)
		}
	}
	if parsed, err := time.Parse("2006-01-02", effectiveFrom); err != nil || parsed.Format("2006-01-02") != effectiveFrom {
		*errors = append(*errors, onboardingDiag("invalid_capacity_effective_from", fmt.Sprintf("effective-from %q must use YYYY-MM-DD", effectiveFrom), "capacity", ""))
	}
	var latest *domain.Capacity
	for i := range ws.Capacities {
		capacity := &ws.Capacities[i]
		if capacity.ResourceRef.ID != req.ResourceID || capacity.Unit != req.Unit || capacity.Type != "periodic_capacity" {
			continue
		}
		if latest == nil || capacity.EffectiveFrom > latest.EffectiveFrom {
			latest = capacity
		}
	}
	if latest != nil && effectiveFrom <= latest.EffectiveFrom {
		*errors = append(*errors, onboardingDiag("capacity_effective_from_not_after_latest", fmt.Sprintf("effective-from %s must be later than latest period %s", effectiveFrom, latest.EffectiveFrom), "capacity", latest.ID))
	}
	return resource, latest, effectiveFrom
}

func onboardingOccurredAt(value string) string {
	if value != "" {
		return value
	}
	return time.Now().Format(time.RFC3339)
}
func todayInTimezone(zone string) string {
	location, err := time.LoadLocation(zone)
	if err != nil {
		location = time.Local
	}
	return time.Now().In(location).Format("2006-01-02")
}
func onboardingDiag(code, message, kind, id string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}}
}

func resourceAddPlannedWrites(resource domain.Resource, actor *domain.Actor, audit domain.AuditRecord) ([]PlannedWrite, error) {
	resourceYAML, err := yamlformat.Render(struct {
		Resources []domain.Resource `yaml:"resources"`
	}{[]domain.Resource{resource}})
	if err != nil {
		return nil, fmt.Errorf("render resource preview: %w", err)
	}
	writes := []PlannedWrite{{Path: "canonical/resources.yaml", Action: "append_yaml_record", Format: "yaml", Content: string(resourceYAML)}}
	if actor != nil {
		actorYAML, err := yamlformat.Render(struct {
			Actors []domain.Actor `yaml:"actors"`
		}{[]domain.Actor{*actor}})
		if err != nil {
			return nil, fmt.Errorf("render actor preview: %w", err)
		}
		writes = append(writes, PlannedWrite{Path: "canonical/actors.yaml", Action: "append_yaml_record", Format: "yaml", Content: string(actorYAML)})
	}
	return append(writes, auditPlannedWrite(audit)), nil
}

func capacitySetPlannedWrites(capacity domain.Capacity, previous *domain.Capacity, previousTo string, audit domain.AuditRecord) ([]PlannedWrite, error) {
	data, err := yamlformat.Render(struct {
		Capacities []domain.Capacity `yaml:"capacities"`
	}{[]domain.Capacity{capacity}})
	if err != nil {
		return nil, fmt.Errorf("render capacity preview: %w", err)
	}
	action := "append_yaml_record"
	if previous != nil {
		action = fmt.Sprintf("set %s effective_to=%s and append_yaml_record", previous.ID, previousTo)
	}
	return []PlannedWrite{{Path: "canonical/capacities.yaml", Action: action, Format: "yaml", Content: string(data)}, auditPlannedWrite(audit)}, nil
}

// Slug converts a display/path component to a stable lowercase id component.
func Slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(value, "-")
	value = strings.Trim(value, "-")
	if value == "" {
		return "project"
	}
	return value
}

// AuditJSONLine renders an audit record for project initialization previews.
func AuditJSONLine(record domain.AuditRecord) string {
	data, _ := json.Marshal(record)
	return string(data) + "\n"
}
