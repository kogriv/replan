package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type DemandWriter interface {
	WriteResourceDemands([]domain.ResourceDemand) ([]string, error)
	AppendAudit(domain.AuditRecord) (string, error)
}

type DemandAddItem struct {
	ID, Suffix, TaskID, ResourceID, PoolID string
	ResourceClass, Role, Unit              string
	Amount                                 float64
	Criticality, Status                    string
	EarliestStart, LatestFinish            string
}

type DemandAddRequest struct {
	Items                       []DemandAddItem
	Reason, ActorID, EvidenceID string
	OccurredAt                  string
	DryRun                      bool
}

type DemandSetItem struct {
	ID                              string
	Amount, Remaining               *float64
	Unit, Role, Criticality, Status *string
	EarliestStart, LatestFinish     *string
}

type DemandSetRequest struct {
	Items                       []DemandSetItem
	Reason, ActorID, EvidenceID string
	OccurredAt                  string
	DryRun                      bool
}

type DemandMoveRequest struct {
	ID, NewID, Suffix, ToResourceID, ToPoolID string
	Amount                                    *float64
	Reason, ActorID, EvidenceID               string
	OccurredAt                                string
	DryRun                                    bool
}

type DemandChange struct {
	Before domain.ResourceDemand `json:"before"`
	After  domain.ResourceDemand `json:"after"`
}

type DemandResult struct {
	DryRun              bool                    `json:"dry_run"`
	Applied             bool                    `json:"applied"`
	Demands             []domain.ResourceDemand `json:"demands,omitempty"`
	Changes             []DemandChange          `json:"changes,omitempty"`
	PlannedWrites       []PlannedWrite          `json:"planned_writes"`
	WrittenFiles        []string                `json:"written_files,omitempty"`
	AuditRecordPreviews []domain.AuditRecord    `json:"audit_record_previews"`
	Validation          *ValidateResult         `json:"validation,omitempty"`
	Warnings            []Diagnostic            `json:"warnings"`
	Errors              []Diagnostic            `json:"errors"`
}

type DemandService struct{ loader WorkspaceLoader }

func NewDemandService(loader WorkspaceLoader) DemandService { return DemandService{loader: loader} }

func (s DemandService) Add(ctx context.Context, req DemandAddRequest) (DemandResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := newDemandResult(req.DryRun, diags)
	validateConstraintAddCommon(&result.Errors, ws, len(req.Items), "demand", req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt)
	seen := demandIDSet(ws.ResourceDemands)
	for _, item := range req.Items {
		demand := demandFromAddItem(ws, item)
		validateDemandAddItem(&result.Errors, ws, demand, item, seen)
		if demand.ID != "" {
			seen[demand.ID] = true
		}
		result.Demands = append(result.Demands, demand)
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	result.AuditRecordPreviews = demandAudits("demand_add", result.Demands, nil, req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt, req.DryRun)
	result.PlannedWrites = demandPlannedWrites(result.Demands, true, result.Demands, result.AuditRecordPreviews)
	return s.write(ctx, result, result.Demands)
}

func (s DemandService) Set(ctx context.Context, req DemandSetRequest) (DemandResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := newDemandResult(req.DryRun, diags)
	validateConstraintAddCommon(&result.Errors, ws, len(req.Items), "demand", req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt)
	seen := map[string]bool{}
	for _, item := range req.Items {
		if seen[item.ID] {
			result.Errors = append(result.Errors, demandDiag("duplicate_demand_id", fmt.Sprintf("demand %s is specified more than once", item.ID), item.ID))
			continue
		}
		seen[item.ID] = true
		before, ok := findDemand(ws.ResourceDemands, item.ID)
		if !ok {
			result.Errors = append(result.Errors, demandDiag("demand_not_found", fmt.Sprintf("resource demand %s not found", item.ID), item.ID))
			continue
		}
		after := before
		applyDemandSetItem(&after, item)
		validateDemandSetItem(&result.Errors, ws, before, after, item)
		result.Changes = append(result.Changes, DemandChange{Before: before, After: after})
		result.Demands = append(result.Demands, after)
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	result.AuditRecordPreviews = demandAudits("demand_set", result.Demands, result.Changes, req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt, req.DryRun)
	result.PlannedWrites = demandPlannedWrites(result.Demands, false, nil, result.AuditRecordPreviews)
	return s.write(ctx, result, result.Demands)
}

func (s DemandService) Move(ctx context.Context, req DemandMoveRequest) (DemandResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := newDemandResult(req.DryRun, diags)
	validateConstraintAddCommon(&result.Errors, ws, 1, "demand", req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt)
	old, ok := findDemand(ws.ResourceDemands, req.ID)
	if !ok {
		result.Errors = append(result.Errors, demandDiag("demand_not_found", fmt.Sprintf("resource demand %s not found", req.ID), req.ID))
		return result, ErrValidationFailed
	}
	newID := req.NewID
	if newID == "" && req.Suffix != "" {
		newID = demandID(old.TaskRef.ID, req.Suffix)
	}
	validateDemandMove(&result.Errors, ws, old, newID, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	amount := old.Remaining()
	if req.Amount != nil {
		amount = *req.Amount
	}
	newDemand := old
	newDemand.ID = newID
	newDemand.PlannedAmount = amount
	newDemand.RemainingAmount = domain.Amount(amount)
	newDemand.ReservedAmount = 0
	newDemand.ActualAmount = 0
	newDemand.Status = "active"
	newDemand.Supersedes = domain.Ref{Kind: "resource_demand", ID: old.ID}
	newDemand.SupersededBy = domain.Ref{}
	newDemand.OverrunReview = nil
	newDemand.ResourceClass = ""
	setDemandTarget(&newDemand, ws, req.ToResourceID, req.ToPoolID)
	oldAfter := old
	oldAfter.Status = "superseded"
	oldAfter.RemainingAmount = domain.Amount(0)
	oldAfter.SupersededBy = domain.Ref{Kind: "resource_demand", ID: newDemand.ID}
	result.Demands = []domain.ResourceDemand{oldAfter, newDemand}
	result.Changes = []DemandChange{{Before: old, After: oldAfter}}
	result.AuditRecordPreviews = []domain.AuditRecord{demandMoveAudit(old, newDemand, req.Reason, req.ActorID, req.EvidenceID, req.OccurredAt, req.DryRun)}
	result.PlannedWrites = demandPlannedWrites(result.Demands, true, []domain.ResourceDemand{newDemand}, result.AuditRecordPreviews)
	return s.write(ctx, result, result.Demands)
}

func newDemandResult(dryRun bool, diags []Diagnostic) DemandResult {
	r := DemandResult{DryRun: dryRun, Demands: []domain.ResourceDemand{}, Changes: []DemandChange{}, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, AuditRecordPreviews: []domain.AuditRecord{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&r.Warnings, &r.Errors, diags)
	return r
}

func (s DemandService) write(ctx context.Context, result DemandResult, demands []domain.ResourceDemand) (DemandResult, error) {
	if result.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, ok := session.target.(DemandWriter)
	if !ok {
		result.Errors = append(result.Errors, demandDiag("storage_write_not_supported", "configured project store does not support demand writes", ""))
		return result, ErrValidationFailed
	}
	paths, err := writer.WriteResourceDemands(demands)
	if err != nil {
		result.Errors = append(result.Errors, demandDiag("demand_write_failed", err.Error(), ""))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, paths...)
	for _, audit := range result.AuditRecordPreviews {
		path, appendErr := writer.AppendAudit(audit)
		if appendErr != nil {
			result.Errors = append(result.Errors, demandDiag("audit_write_failed", appendErr.Error(), audit.ID))
			return result, appendErr
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

func demandFromAddItem(ws domain.Workspace, item DemandAddItem) domain.ResourceDemand {
	id := item.ID
	if id == "" && item.TaskID != "" && item.Suffix != "" {
		id = demandID(item.TaskID, item.Suffix)
	}
	criticality := item.Criticality
	if criticality == "" {
		criticality = "required"
	}
	status := item.Status
	if status == "" {
		status = "active"
	}
	role := item.Role
	if role == "" {
		role = item.Suffix
	}
	demand := domain.ResourceDemand{ID: id, TaskRef: domain.Ref{Kind: "task", ID: item.TaskID}, ResourceClass: item.ResourceClass, Role: role, Unit: item.Unit, PlannedAmount: item.Amount, RemainingAmount: domain.Amount(item.Amount), Criticality: criticality, Status: status, Timing: domain.Timing{EarliestStart: item.EarliestStart, LatestFinish: item.LatestFinish}}
	setDemandTarget(&demand, ws, item.ResourceID, item.PoolID)
	return demand
}

func setDemandTarget(demand *domain.ResourceDemand, ws domain.Workspace, resourceID, poolID string) {
	demand.ResourceRef, demand.ResourcePoolRef = domain.Ref{}, domain.Ref{}
	if resourceID != "" {
		demand.ResourceRef = domain.Ref{Kind: "resource", ID: resourceID}
		if demand.ResourceClass == "" {
			if resource, ok := findResource(ws.Resources, resourceID); ok {
				demand.ResourceClass = resource.Class
			}
		}
	}
	if poolID != "" {
		demand.ResourcePoolRef = domain.Ref{Kind: "resource_pool", ID: poolID}
		if demand.ResourceClass == "" {
			if pool, ok := findPool(ws.ResourcePools, poolID); ok {
				demand.ResourceClass = pool.ResourceClass
			}
		}
	}
}

func validateDemandAddItem(errors *[]Diagnostic, ws domain.Workspace, demand domain.ResourceDemand, item DemandAddItem, seen map[string]bool) {
	if item.TaskID == "" {
		*errors = append(*errors, demandDiag("missing_task", "task is required", demand.ID))
	} else if _, ok := findTask(ws.Tasks, item.TaskID); !ok {
		*errors = append(*errors, demandDiag("task_not_found", fmt.Sprintf("task %s not found", item.TaskID), demand.ID))
	}
	if demand.ID == "" {
		*errors = append(*errors, demandDiag("missing_demand_id", "demand id or suffix is required", ""))
	} else if seen[demand.ID] {
		*errors = append(*errors, demandDiag("duplicate_demand_id", fmt.Sprintf("resource demand %s already exists or is repeated", demand.ID), demand.ID))
	}
	if item.Suffix != "" && !demandSuffixPattern.MatchString(item.Suffix) {
		*errors = append(*errors, demandDiag("invalid_demand_suffix", "suffix must contain lowercase letters, digits, underscores or hyphens", demand.ID))
	}
	validateDemandFields(errors, ws, demand, true)
}

func validateDemandFields(errors *[]Diagnostic, ws domain.Workspace, demand domain.ResourceDemand, positiveAmount bool) {
	if (demand.ResourceRef.ID == "") == (demand.ResourcePoolRef.ID == "") {
		*errors = append(*errors, demandDiag("invalid_demand_target", "exactly one of resource or pool is required", demand.ID))
	} else if demand.ResourceRef.ID != "" {
		if _, ok := findResource(ws.Resources, demand.ResourceRef.ID); !ok {
			*errors = append(*errors, demandDiag("resource_not_found", fmt.Sprintf("resource %s not found", demand.ResourceRef.ID), demand.ID))
		} else if !resourceHasCapacityUnit(ws, demand.ResourceRef.ID, demand.Unit) {
			*errors = append(*errors, demandDiag("invalid_resource_unit", fmt.Sprintf("resource %s has no capacity in unit %s", demand.ResourceRef.ID, demand.Unit), demand.ID))
		}
	} else if pool, ok := findPool(ws.ResourcePools, demand.ResourcePoolRef.ID); !ok {
		*errors = append(*errors, demandDiag("resource_pool_not_found", fmt.Sprintf("resource pool %s not found", demand.ResourcePoolRef.ID), demand.ID))
	} else if !poolHasCapacityUnit(ws, pool, demand.Unit) {
		*errors = append(*errors, demandDiag("invalid_resource_unit", fmt.Sprintf("resource pool %s has no member capacity in unit %s", pool.ID, demand.Unit), demand.ID))
	}
	if !validAmount(demand.PlannedAmount) || positiveAmount && demand.PlannedAmount == 0 {
		message := "planned amount must be finite, zero or greater"
		if positiveAmount {
			message = "planned amount must be a finite number greater than zero"
		}
		*errors = append(*errors, demandDiag("invalid_demand_amount", message, demand.ID))
	}
	if demand.RemainingAmount != nil && !validAmount(*demand.RemainingAmount) {
		*errors = append(*errors, demandDiag("invalid_remaining_amount", "remaining amount must be finite, zero or greater", demand.ID))
	}
	if strings.TrimSpace(demand.Role) == "" {
		*errors = append(*errors, demandDiag("invalid_demand_role", "role must not be empty", demand.ID))
	}
	if demand.ResourceClass == "" {
		*errors = append(*errors, demandDiag("missing_resource_class", "resource class is required", demand.ID))
	}
	if demand.Criticality != "" && !validDemandCriticality(demand.Criticality) {
		*errors = append(*errors, demandDiag("invalid_demand_criticality", fmt.Sprintf("invalid demand criticality %q", demand.Criticality), demand.ID))
	}
	if demand.Status != "" && !domain.ValidResourceDemandStatus(demand.Status) {
		*errors = append(*errors, demandDiag("invalid_demand_status", fmt.Sprintf("invalid demand status %q", demand.Status), demand.ID))
	}
	validateDemandTiming(errors, demand)
}

func applyDemandSetItem(demand *domain.ResourceDemand, item DemandSetItem) {
	if item.Amount != nil {
		demand.PlannedAmount = *item.Amount
	}
	if item.Remaining != nil {
		demand.RemainingAmount = domain.Amount(*item.Remaining)
	}
	if item.Unit != nil {
		demand.Unit = *item.Unit
	}
	if item.Role != nil {
		demand.Role = *item.Role
	}
	if item.Criticality != nil {
		demand.Criticality = *item.Criticality
	}
	if item.Status != nil {
		demand.Status = *item.Status
	}
	if item.EarliestStart != nil {
		demand.Timing.EarliestStart = *item.EarliestStart
	}
	if item.LatestFinish != nil {
		demand.Timing.LatestFinish = *item.LatestFinish
	}
}

func validateDemandSetItem(errors *[]Diagnostic, ws domain.Workspace, before, after domain.ResourceDemand, item DemandSetItem) {
	if item.Amount == nil && item.Remaining == nil && item.Unit == nil && item.Role == nil && item.Criticality == nil && item.Status == nil && item.EarliestStart == nil && item.LatestFinish == nil {
		*errors = append(*errors, demandDiag("missing_demand_change", "at least one demand field change is required", before.ID))
		return
	}
	validateDemandFields(errors, ws, after, false)
	if item.Criticality != nil && !validDemandCriticality(*item.Criticality) && after.Criticality == "" {
		*errors = append(*errors, demandDiag("invalid_demand_criticality", "demand criticality must not be empty", before.ID))
	}
	if item.Status != nil && !domain.ValidResourceDemandStatus(*item.Status) && after.Status == "" {
		*errors = append(*errors, demandDiag("invalid_demand_status", "demand status must not be empty", before.ID))
	}
	if resourceDemandsEqual(before, after) {
		*errors = append(*errors, demandDiag("demand_unchanged", "demand values are unchanged", before.ID))
	}
}

func resourceDemandsEqual(before, after domain.ResourceDemand) bool {
	beforeRemaining, afterRemaining := before.RemainingAmount, after.RemainingAmount
	before.RemainingAmount, after.RemainingAmount = nil, nil
	if before != after {
		return false
	}
	if beforeRemaining == nil || afterRemaining == nil {
		return beforeRemaining == nil && afterRemaining == nil
	}
	return *beforeRemaining == *afterRemaining
}

func validateDemandMove(errors *[]Diagnostic, ws domain.Workspace, old domain.ResourceDemand, newID string, req DemandMoveRequest) {
	if old.Status == "superseded" || old.Status == "cancelled" || old.Status == "consumed" {
		*errors = append(*errors, demandDiag("demand_not_movable", fmt.Sprintf("resource demand %s has status %s", old.ID, old.Status), old.ID))
	}
	if req.Amount == nil && (!old.HasRemaining() || old.Remaining() == 0) {
		*errors = append(*errors, demandDiag("demand_nothing_to_move", fmt.Sprintf("resource demand %s has no remaining amount to move", old.ID), old.ID))
	}
	if newID == "" {
		*errors = append(*errors, demandDiag("missing_demand_id", "new demand id or suffix is required", old.ID))
	} else if _, ok := findDemand(ws.ResourceDemands, newID); ok {
		*errors = append(*errors, demandDiag("duplicate_demand_id", fmt.Sprintf("resource demand %s already exists", newID), newID))
	}
	if req.Suffix != "" && !demandSuffixPattern.MatchString(req.Suffix) {
		*errors = append(*errors, demandDiag("invalid_demand_suffix", "suffix must contain lowercase letters, digits, underscores or hyphens", newID))
	}
	if (req.ToResourceID == "") == (req.ToPoolID == "") {
		*errors = append(*errors, demandDiag("invalid_demand_target", "exactly one of to-resource or to-pool is required", old.ID))
		return
	}
	if req.ToResourceID != "" && old.ResourceRef.ID == req.ToResourceID || req.ToPoolID != "" && old.ResourcePoolRef.ID == req.ToPoolID {
		*errors = append(*errors, demandDiag("same_demand_target", "new demand target must differ from the old target", old.ID))
	}
	probe := old
	probe.ID = newID
	probe.Status = "active"
	probe.ResourceClass = ""
	setDemandTarget(&probe, ws, req.ToResourceID, req.ToPoolID)
	if req.Amount != nil {
		probe.PlannedAmount, probe.RemainingAmount = *req.Amount, domain.Amount(*req.Amount)
	}
	validateDemandFields(errors, ws, probe, false)
}

func validateDemandTiming(errors *[]Diagnostic, demand domain.ResourceDemand) {
	startValid, finishValid := true, true
	if demand.Timing.EarliestStart != "" && !validDate(demand.Timing.EarliestStart) {
		*errors = append(*errors, demandDiag("invalid_demand_earliest_start", "earliest-start must use YYYY-MM-DD", demand.ID))
		startValid = false
	}
	if demand.Timing.LatestFinish != "" && !validDate(demand.Timing.LatestFinish) {
		*errors = append(*errors, demandDiag("invalid_demand_latest_finish", "latest-finish must use YYYY-MM-DD", demand.ID))
		finishValid = false
	}
	if startValid && finishValid && demand.Timing.EarliestStart != "" && demand.Timing.LatestFinish != "" && demand.Timing.LatestFinish < demand.Timing.EarliestStart {
		*errors = append(*errors, demandDiag("invalid_demand_timing_range", "latest-finish is before earliest-start", demand.ID))
	}
}

func demandAudits(operation string, demands []domain.ResourceDemand, changes []DemandChange, reason, actor, evidence, occurredAt string, dryRun bool) []domain.AuditRecord {
	if occurredAt == "" {
		occurredAt = time.Now().Format(time.RFC3339)
	}
	changeByID := map[string]DemandChange{}
	for _, change := range changes {
		changeByID[change.Before.ID] = change
	}
	audits := make([]domain.AuditRecord, 0, len(demands))
	for _, demand := range demands {
		auditReason := strings.TrimRight(strings.TrimSpace(reason), ".") + "."
		if change, ok := changeByID[demand.ID]; ok {
			auditReason += " " + demandSetChangeSummary(change.Before, change.After)
		}
		audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + "." + strings.ReplaceAll(operation, "_", "-") + "." + slugID(demand.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: actor}, Operation: operation, TargetRef: domain.Ref{Kind: "resource_demand", ID: demand.ID}, Reason: auditReason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: evidence}}, Result: "success"}
		if dryRun {
			audit.Result = "dry_run"
		}
		audits = append(audits, audit)
	}
	return audits
}

func demandSetChangeSummary(before, after domain.ResourceDemand) string {
	changes := []string{}
	if before.PlannedAmount != after.PlannedAmount {
		changes = append(changes, fmt.Sprintf("planned_amount: %g -> %g", before.PlannedAmount, after.PlannedAmount))
	}
	if !optionalAmountsEqual(before.RemainingAmount, after.RemainingAmount) {
		changes = append(changes, fmt.Sprintf("remaining_amount: %s -> %s", formatOptionalAmount(before.RemainingAmount), formatOptionalAmount(after.RemainingAmount)))
	}
	for _, field := range []struct{ name, before, after string }{
		{"unit", before.Unit, after.Unit},
		{"role", before.Role, after.Role},
		{"criticality", before.Criticality, after.Criticality},
		{"status", before.Status, after.Status},
		{"earliest_start", before.Timing.EarliestStart, after.Timing.EarliestStart},
		{"latest_finish", before.Timing.LatestFinish, after.Timing.LatestFinish},
	} {
		if field.before != field.after {
			changes = append(changes, fmt.Sprintf("%s: %q -> %q", field.name, field.before, field.after))
		}
	}
	return strings.Join(changes, "; ") + "."
}

func optionalAmountsEqual(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func formatOptionalAmount(amount *float64) string {
	if amount == nil {
		return "(unset)"
	}
	return fmt.Sprintf("%g", *amount)
}

func demandMoveAudit(old, replacement domain.ResourceDemand, reason, actor, evidence, occurredAt string, dryRun bool) domain.AuditRecord {
	if occurredAt == "" {
		occurredAt = time.Now().Format(time.RFC3339)
	}
	target := replacement.ResourceRef.ID
	if target == "" {
		target = replacement.ResourcePoolRef.ID
	}
	auditReason := fmt.Sprintf("%s. Moved to %s as %s; remaining %g.", strings.TrimRight(strings.TrimSpace(reason), "."), target, replacement.ID, replacement.Remaining())
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".demand-move." + slugID(old.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: actor}, Operation: "demand_move", TargetRef: domain.Ref{Kind: "resource_demand", ID: old.ID}, Reason: auditReason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: evidence}}, Result: "success"}
	if dryRun {
		audit.Result = "dry_run"
	}
	return audit
}

func demandPlannedWrites(demands []domain.ResourceDemand, adds bool, taskAdditions []domain.ResourceDemand, audits []domain.AuditRecord) []PlannedWrite {
	content, _ := yaml.Marshal(struct {
		ResourceDemands []domain.ResourceDemand `yaml:"resource_demands"`
	}{demands})
	action := "replace_yaml_records"
	if adds {
		action = "append_or_replace_yaml_records"
	}
	writes := []PlannedWrite{{Path: "canonical/resource_demands.yaml", Action: action, Format: "yaml", Content: string(content)}}
	if len(taskAdditions) > 0 {
		taskContent, _ := yaml.Marshal(demandTaskReferencePreviewFor(taskAdditions))
		writes = append(writes, PlannedWrite{Path: "canonical/tasks.yaml", Action: "append_yaml_references", Format: "yaml-patch-preview", Content: string(taskContent)})
	}
	for _, audit := range audits {
		writes = append(writes, auditPlannedWrite(audit))
	}
	return writes
}

type demandTaskReferencePreviewDoc struct {
	Tasks []demandTaskReferencePreview `yaml:"tasks"`
}

type demandTaskReferencePreview struct {
	ID              string       `yaml:"id"`
	ResourceDemands []domain.Ref `yaml:"append_resource_demands"`
}

func demandTaskReferencePreviewFor(demands []domain.ResourceDemand) demandTaskReferencePreviewDoc {
	preview := demandTaskReferencePreviewDoc{}
	indexes := map[string]int{}
	for _, demand := range demands {
		i, ok := indexes[demand.TaskRef.ID]
		if !ok {
			preview.Tasks = append(preview.Tasks, demandTaskReferencePreview{ID: demand.TaskRef.ID})
			i = len(preview.Tasks) - 1
			indexes[demand.TaskRef.ID] = i
		}
		preview.Tasks[i].ResourceDemands = append(preview.Tasks[i].ResourceDemands, domain.Ref{Kind: "resource_demand", ID: demand.ID})
	}
	return preview
}

func demandID(taskID, suffix string) string {
	return "resource_demand." + taskDemandSlug(taskID) + "." + suffix
}
func demandIDSet(demands []domain.ResourceDemand) map[string]bool {
	out := map[string]bool{}
	for _, demand := range demands {
		out[demand.ID] = true
	}
	return out
}
func validDemandCriticality(value string) bool {
	return value == "required" || value == "preferred" || value == "optional" || value == "substitute"
}
func demandDiag(code, message, id string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: "resource_demand", ID: id}}
}
func findPool(pools []domain.ResourcePool, id string) (domain.ResourcePool, bool) {
	for _, pool := range pools {
		if pool.ID == id {
			return pool, true
		}
	}
	return domain.ResourcePool{}, false
}
func poolHasCapacityUnit(ws domain.Workspace, pool domain.ResourcePool, unit string) bool {
	for _, member := range pool.Members {
		if resourceHasCapacityUnit(ws, member.ID, unit) {
			return true
		}
	}
	return false
}
