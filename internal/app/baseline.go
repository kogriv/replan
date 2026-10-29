package app

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type BaselineService struct {
	loader WorkspaceLoader
}

type BaselineCreateRequest struct {
	ID          string
	Name        string
	ApprovedBy  string
	StatusDate  string
	Assumptions []string
	OccurredAt  string
	DryRun      bool
}

type BaselineCreateResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	FilePath           string             `json:"file_path"`
	TaskCount          int                `json:"task_count"`
	DemandCount        int                `json:"demand_count"`
	Baseline           domain.Baseline    `json:"baseline"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type BaselineSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name,omitempty"`
	StatusDate       string `json:"status_date,omitempty"`
	ApprovedBy       string `json:"approved_by,omitempty"`
	Tasks            int    `json:"tasks"`
	Demands          int    `json:"demands"`
	CarriesEstimates bool   `json:"carries_estimates"`
	SealStatus       string `json:"seal_status"`
}

type BaselineListResult struct {
	Baselines []BaselineSummary `json:"baselines"`
	Warnings  []Diagnostic      `json:"warnings"`
	Errors    []Diagnostic      `json:"errors"`
}

type BaselineShowResult struct {
	Baseline         domain.Baseline `json:"baseline"`
	CarriesEstimates bool            `json:"carries_estimates"`
	SealStatus       string          `json:"seal_status"`
	Warnings         []Diagnostic    `json:"warnings"`
	Errors           []Diagnostic    `json:"errors"`
}

type BaselineWriter interface {
	CreateProjectFile(rel string, content []byte) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
}

func NewBaselineService(loader WorkspaceLoader) BaselineService {
	return BaselineService{loader: loader}
}

func (s BaselineService) Create(ctx context.Context, req BaselineCreateRequest) (BaselineCreateResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := BaselineCreateResult{DryRun: req.DryRun, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	validateBaselineCreateRequest(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	occurredAt := req.OccurredAt
	if occurredAt == "" {
		occurredAt = time.Now().Format(time.RFC3339)
	}
	tasksByID := make(map[string]domain.Task, len(ws.Tasks))
	for _, task := range ws.Tasks {
		tasksByID[task.ID] = task
	}
	baseline := domain.Baseline{
		ID: req.ID, Name: req.Name, CreatedAt: occurredAt, ApprovedAt: occurredAt,
		ApprovedBy: domain.Ref{Kind: "actor", ID: req.ApprovedBy}, StatusDate: req.StatusDate,
		IncludedTasks: []domain.BaselineTaskEntry{}, IncludedDemands: []domain.BaselineDemandEntry{},
		Assumptions: append([]string(nil), req.Assumptions...),
	}
	included := make(map[string]bool, len(ws.CurrentPlan.IncludedTasks))
	for _, ref := range ws.CurrentPlan.IncludedTasks {
		task := tasksByID[ref.ID]
		baseline.IncludedTasks = append(baseline.IncludedTasks, domain.BaselineTaskEntry{
			Ref: domain.Ref{Kind: "task", ID: ref.ID}, PlannedStart: task.PlannedStart, PlannedFinish: task.PlannedFinish,
		})
		included[ref.ID] = true
	}
	for _, demand := range ws.ResourceDemands {
		if !included[demand.TaskRef.ID] {
			continue
		}
		resourceID := demand.ResourceRef.ID
		if resourceID == "" {
			resourceID = demand.ResourcePoolRef.ID
		}
		amount := demand.PlannedAmount
		baseline.IncludedDemands = append(baseline.IncludedDemands, domain.BaselineDemandEntry{
			Ref: domain.Ref{Kind: "resource_demand", ID: demand.ID}, Task: demand.TaskRef.ID,
			Resource: resourceID, Unit: demand.Unit, PlannedAmount: &amount,
		})
	}
	if err := domain.SealBaseline(&baseline); err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "baseline_seal_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "baseline", ID: req.ID}})
		return result, err
	}

	path := filepath.ToSlash(filepath.Join("baselines", req.ID+".yaml"))
	data, err := yaml.Marshal(struct {
		Baseline domain.Baseline `yaml:"baseline"`
	}{Baseline: baseline})
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "yaml_render_error", Message: fmt.Sprintf("failed to render baseline yaml: %v", err), Severity: "error", Target: Target{Kind: "baseline", ID: req.ID}})
		return result, err
	}
	audit := domain.AuditRecord{
		ID: "audit." + compactTimestamp(occurredAt) + ".baseline-create." + slugID(req.ID), OccurredAt: occurredAt,
		ActorRef: domain.Ref{Kind: "actor", ID: req.ApprovedBy}, Operation: "baseline_create",
		TargetRef: domain.Ref{Kind: "baseline", ID: req.ID}, Result: "dry_run", Reason: "Preview baseline snapshot.",
	}
	if !req.DryRun {
		audit.Result = "success"
		audit.Reason = "Created approved baseline snapshot."
	}
	result.FilePath = path
	result.TaskCount = len(baseline.IncludedTasks)
	result.DemandCount = len(baseline.IncludedDemands)
	result.Baseline = baseline
	result.AuditRecordPreview = audit
	result.PlannedWrites = []PlannedWrite{
		{Path: path, Action: "create", Format: "yaml", Content: string(data)},
		auditPlannedWrite(audit),
	}
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(BaselineWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "storage_write_not_supported", Message: "configured project store does not support baseline writes", Severity: "error", Target: Target{Kind: "project", ID: s.loader.ProjectPath()}})
		return result, ErrValidationFailed
	}
	baselinePath, err := writer.CreateProjectFile(path, data)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "baseline_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "baseline", ID: req.ID}})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, baselinePath)
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

func (s BaselineService) List(ctx context.Context) (BaselineListResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := BaselineListResult{Baselines: []BaselineSummary{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	for _, baseline := range ws.Baselines {
		result.Baselines = append(result.Baselines, baselineSummary(baseline))
	}
	sort.Slice(result.Baselines, func(i, j int) bool { return result.Baselines[i].ID < result.Baselines[j].ID })
	return result, nil
}

func (s BaselineService) Show(ctx context.Context, id string) (BaselineShowResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := BaselineShowResult{Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	for _, baseline := range ws.Baselines {
		if baseline.ID == id {
			result.Baseline = baseline
			result.CarriesEstimates = domain.BaselineCarriesEstimates(baseline)
			result.SealStatus = baselineSealStatus(baseline)
			return result, nil
		}
	}
	result.Errors = append(result.Errors, Diagnostic{Code: "baseline_not_found", Message: fmt.Sprintf("baseline %s not found", id), Severity: "error", Target: Target{Kind: "baseline", ID: id}})
	return result, ErrValidationFailed
}

func validateBaselineCreateRequest(errors *[]Diagnostic, ws domain.Workspace, req BaselineCreateRequest) {
	add := func(code, message, kind, id string) {
		*errors = append(*errors, Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}})
	}
	if strings.TrimSpace(req.ID) == "" {
		add("missing_baseline_id", "baseline id is required", "baseline", "")
	}
	if strings.TrimSpace(req.Name) == "" {
		add("missing_baseline_name", "baseline name is required", "baseline", req.ID)
	}
	for _, baseline := range ws.Baselines {
		if baseline.ID == req.ID {
			add("baseline_already_exists", fmt.Sprintf("baseline %s already exists", req.ID), "baseline", req.ID)
			break
		}
	}
	if req.ApprovedBy == "" {
		add("missing_approved_actor", "approved-by is required", "actor", "")
	} else if !hasActor(ws.Actors, req.ApprovedBy) {
		add("actor_not_found", fmt.Sprintf("actor %s not found", req.ApprovedBy), "actor", req.ApprovedBy)
	}
	if req.StatusDate != "" && !validDate(req.StatusDate) {
		add("invalid_status_date", fmt.Sprintf("status-date %q must use YYYY-MM-DD", req.StatusDate), "baseline", req.ID)
	}
	if req.OccurredAt != "" {
		if _, err := time.Parse(time.RFC3339, req.OccurredAt); err != nil {
			add("invalid_occurred_at", fmt.Sprintf("occurred-at %q must be RFC3339", req.OccurredAt), "baseline", req.ID)
		}
	}
	if len(ws.CurrentPlan.IncludedTasks) == 0 {
		add("empty_current_plan", "current plan has no included tasks", "current_plan", ws.CurrentPlan.ID)
	}
}

func baselineSummary(baseline domain.Baseline) BaselineSummary {
	return BaselineSummary{
		ID: baseline.ID, Name: baseline.Name, StatusDate: baseline.StatusDate, ApprovedBy: baseline.ApprovedBy.ID,
		Tasks: len(baseline.IncludedTasks), Demands: len(baseline.IncludedDemands),
		CarriesEstimates: domain.BaselineCarriesEstimates(baseline), SealStatus: baselineSealStatus(baseline),
	}
}

func baselineSealStatus(baseline domain.Baseline) string {
	if baseline.Seal == nil {
		return "unsealed"
	}
	if baseline.Seal.Algorithm != "sha256" {
		return "unsupported"
	}
	digest, err := domain.BaselineDigest(baseline)
	if err != nil || digest != baseline.Seal.Digest {
		return "modified"
	}
	return "verified"
}
