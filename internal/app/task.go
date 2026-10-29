package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type TaskWriter interface {
	AddTask(task domain.Task, demands []domain.ResourceDemand, afterTaskID string) ([]string, error)
	SetTaskDates(taskID, plannedStart, plannedFinish string) (string, error)
	SetTaskTitle(taskID, title string) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
}

type TaskAddRequest struct {
	ID, Title, Description, Type string
	PlannedStart, PlannedFinish  string
	Tags                         []string
	AssigneeID, AfterTaskID      string
	DemandSpecs                  []string
	ActorID, EvidenceID          string
	OccurredAt                   string
	DryRun                       bool
}

type TaskAddResult struct {
	DryRun             bool                    `json:"dry_run"`
	Applied            bool                    `json:"applied"`
	Task               domain.Task             `json:"task"`
	Demands            []domain.ResourceDemand `json:"demands"`
	CalibratedValues   []TaskCalibratedValue   `json:"calibrated_values"`
	AfterTaskID        string                  `json:"after_task_id,omitempty"`
	PlannedWrites      []PlannedWrite          `json:"planned_writes"`
	WrittenFiles       []string                `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord      `json:"audit_record_preview"`
	Validation         *ValidateResult         `json:"validation,omitempty"`
	Warnings           []Diagnostic            `json:"warnings"`
	Errors             []Diagnostic            `json:"errors"`
}

type TaskCalibratedValue struct {
	DemandID      string  `json:"demand_id"`
	TypedAmount   float64 `json:"typed_amount"`
	Calibrated    float64 `json:"calibrated_amount"`
	Unit          string  `json:"unit"`
	CalibrationID string  `json:"calibration_id"`
}

type TaskSetDatesRequest struct {
	TaskID, PlannedStart, PlannedFinish string
	Reason, ActorID, EvidenceID         string
	OccurredAt                          string
	DryRun                              bool
}

type TaskSetDatesResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	TaskID             string             `json:"task_id"`
	PreviousStart      string             `json:"previous_start,omitempty"`
	PreviousFinish     string             `json:"previous_finish,omitempty"`
	NewStart           string             `json:"new_start,omitempty"`
	NewFinish          string             `json:"new_finish,omitempty"`
	Reason             string             `json:"reason"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type TaskRecordActualWindowRequest struct {
	TaskID, Reason, ActorID, EvidenceID string
	OccurredAt                          string
	DryRun                              bool
}

type TaskRecordActualWindowResult struct {
	DryRun             bool                `json:"dry_run"`
	Applied            bool                `json:"applied"`
	TaskID             string              `json:"task_id"`
	PreviousStart      string              `json:"previous_planned_start"`
	PreviousFinish     string              `json:"previous_planned_finish"`
	NewStart           string              `json:"new_planned_start"`
	NewFinish          string              `json:"new_planned_finish"`
	ActualStartSource  string              `json:"actual_start_source"`
	ActualFinishSource string              `json:"actual_finish_source"`
	Changed            bool                `json:"changed"`
	PlannedWrites      []PlannedWrite      `json:"planned_writes"`
	WrittenFiles       []string            `json:"written_files,omitempty"`
	AuditRecordPreview *domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult     `json:"validation,omitempty"`
	Warnings           []Diagnostic        `json:"warnings"`
	Errors             []Diagnostic        `json:"errors"`
}

type TaskSetTitleRequest struct {
	TaskID, Title, Reason, ActorID, EvidenceID string
	OccurredAt                                 string
	DryRun                                     bool
}

type TaskSetTitleResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	NoOp               bool               `json:"no_op"`
	TaskID             string             `json:"task_id"`
	PreviousTitle      string             `json:"previous_title"`
	NewTitle           string             `json:"new_title"`
	Reason             string             `json:"reason"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview,omitempty"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type TaskService struct{ loader WorkspaceLoader }

func NewTaskService(loader WorkspaceLoader) TaskService { return TaskService{loader: loader} }

var demandSuffixPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (s TaskService) Add(ctx context.Context, req TaskAddRequest) (TaskAddResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := TaskAddResult{DryRun: req.DryRun, AfterTaskID: req.AfterTaskID, Demands: []domain.ResourceDemand{}, CalibratedValues: []TaskCalibratedValue{}, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	validateTaskAddBasics(&result.Errors, ws, req)
	demands := parseTaskDemandSpecs(&result.Errors, ws, req.ID, req.DemandSpecs)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	refs := make([]domain.Ref, 0, len(demands))
	for _, demand := range demands {
		refs = append(refs, domain.Ref{Kind: "resource_demand", ID: demand.ID})
	}
	task := domain.Task{ID: req.ID, Title: req.Title, Description: req.Description, Type: req.Type, TrackingStatus: "not_started", PlannedStart: req.PlannedStart, PlannedFinish: req.PlannedFinish, ResourceDemands: refs, Tags: append([]string(nil), req.Tags...)}
	if req.AssigneeID != "" {
		task.AssigneeHint = domain.Ref{Kind: "resource", ID: req.AssigneeID}
	}
	result.Task, result.Demands = task, demands
	result.CalibratedValues = calibratedTaskValues(ws.Calibrations, task, demands)

	occurredAt := taskOccurredAt(req.OccurredAt)
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".task-add." + slugID(req.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "task_add", TargetRef: domain.Ref{Kind: "task", ID: req.ID}, Reason: "Created task and resource demands.", EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}, Result: "success"}
	if req.DryRun {
		audit.Result, audit.Reason = "dry_run", "Preview task and resource-demand creation."
	}
	result.AuditRecordPreview = audit
	result.PlannedWrites = taskAddPlannedWrites(task, demands, req.AfterTaskID, audit)
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(TaskWriter)
	if !ok {
		return taskStorageUnsupported(result)
	}
	written, err := writer.AddTask(task, demands, req.AfterTaskID)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("task_write_failed", err.Error(), "task", req.ID))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, written...)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("audit_write_failed", err.Error(), "audit_record", audit.ID))
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

func calibratedTaskValues(calibrations []domain.Calibration, task domain.Task, demands []domain.ResourceDemand) []TaskCalibratedValue {
	values := []TaskCalibratedValue{}
	for _, demand := range demands {
		var selected *domain.Calibration
		for i := range calibrations {
			calibration := &calibrations[i]
			if calibrationSealStatus(*calibration) != "verified" || calibration.Scope.Unit != demand.Unit || !containsString(calibration.Scope.Resources, demandResourceID(demand)) {
				continue
			}
			if calibration.Scope.Tag != "" && !containsString(task.Tags, calibration.Scope.Tag) {
				continue
			}
			if selected == nil || calibration.ApprovedAt > selected.ApprovedAt {
				selected = calibration
			}
		}
		if selected != nil {
			values = append(values, TaskCalibratedValue{DemandID: demand.ID, TypedAmount: demand.PlannedAmount, Calibrated: RoundCalibrationAmount(demand.PlannedAmount * selected.Result.Factor), Unit: demand.Unit, CalibrationID: selected.ID})
		}
	}
	return values
}

func (s TaskService) SetDates(ctx context.Context, req TaskSetDatesRequest) (TaskSetDatesResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := TaskSetDatesResult{DryRun: req.DryRun, TaskID: req.TaskID, Reason: req.Reason, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	task, ok := validateTaskSetDates(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	result.PreviousStart, result.PreviousFinish = task.PlannedStart, task.PlannedFinish
	result.NewStart, result.NewFinish = task.PlannedStart, task.PlannedFinish
	if req.PlannedStart != "" {
		result.NewStart = req.PlannedStart
	}
	if req.PlannedFinish != "" {
		result.NewFinish = req.PlannedFinish
	}
	if !ok {
		return result, ErrValidationFailed
	}

	occurredAt := taskOccurredAt(req.OccurredAt)
	auditReason := fmt.Sprintf("%s. Planned dates: start %s -> %s; finish %s -> %s.", strings.TrimRight(strings.TrimSpace(req.Reason), "."), displayEmptyDate(task.PlannedStart), displayEmptyDate(result.NewStart), displayEmptyDate(task.PlannedFinish), displayEmptyDate(result.NewFinish))
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".task-set-dates." + slugID(req.TaskID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "task_set_dates", TargetRef: domain.Ref{Kind: "task", ID: req.TaskID}, Reason: auditReason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}, Result: "success"}
	if req.DryRun {
		audit.Result = "dry_run"
	}
	result.AuditRecordPreview = audit
	patch, _ := yaml.Marshal(map[string]any{"task": req.TaskID, "planned_start": map[string]string{"before": task.PlannedStart, "after": result.NewStart}, "planned_finish": map[string]string{"before": task.PlannedFinish, "after": result.NewFinish}, "reason": req.Reason})
	result.PlannedWrites = []PlannedWrite{{Path: "canonical/tasks.yaml", Action: "replace_yaml_fields", Format: "yaml-patch-preview", Content: string(patch)}, auditPlannedWrite(audit)}
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, supported := session.target.(TaskWriter)
	if !supported {
		result.Errors = append(result.Errors, taskDiag("storage_write_not_supported", "configured project store does not support task writes", "project", s.loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	path, err := writer.SetTaskDates(req.TaskID, result.NewStart, result.NewFinish)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("task_write_failed", err.Error(), "task", req.TaskID))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, path)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("audit_write_failed", err.Error(), "audit_record", audit.ID))
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

func (s TaskService) RecordActualWindow(ctx context.Context, req TaskRecordActualWindowRequest) (TaskRecordActualWindowResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := TaskRecordActualWindowResult{DryRun: req.DryRun, TaskID: req.TaskID, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	task, ok := validateTaskRecordActualWindow(&result.Errors, ws, req)
	if len(result.Errors) > 0 || !ok {
		return result, ErrValidationFailed
	}
	result.PreviousStart, result.PreviousFinish = task.PlannedStart, task.PlannedFinish
	demandIDs := []string{}
	for _, demand := range ws.ResourceDemands {
		if demand.TaskRef.ID == task.ID {
			demandIDs = append(demandIDs, demand.ID)
		}
	}
	window := deriveTaskActualWindow(ws, task.ID, demandIDs, true)
	result.NewStart, result.NewFinish = window.Start, window.Finish
	result.ActualStartSource, result.ActualFinishSource = window.StartSource, window.FinishSource
	if window.Start == "" && window.Finish == "" {
		result.Errors = append(result.Errors, taskDiag("actual_window_unknown", fmt.Sprintf("task %s has no status or consumption actual dates", task.ID), "task", task.ID))
		return result, ErrValidationFailed
	}
	if window.Start != "" && window.Finish != "" && window.Start > window.Finish {
		result.Errors = append(result.Errors, taskDiag("actual_window_inconsistent", fmt.Sprintf("task %s actual start %s (%s) is after actual finish %s (%s)", task.ID, window.Start, window.StartSource, window.Finish, window.FinishSource), "task", task.ID))
		return result, ErrValidationFailed
	}
	working := projectWorkingDays(ws.Availabilities)
	for _, derived := range []struct{ name, date, source string }{{"start", window.Start, window.StartSource}, {"finish", window.Finish, window.FinishSource}} {
		if date, parsed := parseWorkloadDate(derived.date); parsed && len(working) > 0 && !working[date.Weekday()] {
			result.Warnings = append(result.Warnings, Diagnostic{Code: "actual_window_on_non_working_day", Message: fmt.Sprintf("task %s actual %s %s (%s) is on a non-working day; the date is not changed", task.ID, derived.name, derived.date, derived.source), Severity: "warning", Target: Target{Kind: "task", ID: task.ID}})
		}
	}
	result.Changed = task.PlannedStart != window.Start || task.PlannedFinish != window.Finish
	if !result.Changed {
		return result, nil
	}

	occurredAt := taskOccurredAt(req.OccurredAt)
	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = "Recorded task actual window."
	}
	auditReason := fmt.Sprintf("%s. Planned dates: start %s -> %s (%s); finish %s -> %s (%s).", strings.TrimRight(reason, "."), displayEmptyDate(task.PlannedStart), displayEmptyDate(window.Start), window.StartSource, displayEmptyDate(task.PlannedFinish), displayEmptyDate(window.Finish), window.FinishSource)
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".task-record-actual-window." + slugID(req.TaskID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "task_record_actual_window", TargetRef: domain.Ref{Kind: "task", ID: req.TaskID}, ActualStart: window.Start, ActualFinish: window.Finish, ActualStartSource: window.StartSource, ActualFinishSource: window.FinishSource, Reason: auditReason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}, Result: "success"}
	if req.DryRun {
		audit.Result = "dry_run"
	}
	result.AuditRecordPreview = &audit
	patch, _ := yaml.Marshal(map[string]any{"task": req.TaskID, "planned_start": map[string]string{"before": task.PlannedStart, "after": window.Start}, "planned_finish": map[string]string{"before": task.PlannedFinish, "after": window.Finish}, "actual_start_source": window.StartSource, "actual_finish_source": window.FinishSource, "reason": req.Reason})
	result.PlannedWrites = []PlannedWrite{{Path: "canonical/tasks.yaml", Action: "replace_yaml_fields", Format: "yaml-patch-preview", Content: string(patch)}, auditPlannedWrite(audit)}
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, supported := session.target.(TaskWriter)
	if !supported {
		result.Errors = append(result.Errors, taskDiag("storage_write_not_supported", "configured project store does not support task writes", "project", s.loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	path, err := writer.SetTaskDates(req.TaskID, window.Start, window.Finish)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("task_write_failed", err.Error(), "task", req.TaskID))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, path)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("audit_write_failed", err.Error(), "audit_record", audit.ID))
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

func (s TaskService) SetTitle(ctx context.Context, req TaskSetTitleRequest) (TaskSetTitleResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := TaskSetTitleResult{DryRun: req.DryRun, TaskID: req.TaskID, NewTitle: req.Title, Reason: req.Reason, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	task, ok := validateTaskSetTitle(&result.Errors, ws, req)
	if len(result.Errors) > 0 || !ok {
		return result, ErrValidationFailed
	}
	result.PreviousTitle = task.Title
	if task.Title == req.Title {
		result.NoOp = true
		return result, nil
	}

	occurredAt := taskOccurredAt(req.OccurredAt)
	auditReason := fmt.Sprintf("%s. Task title: %q -> %q.", strings.TrimRight(strings.TrimSpace(req.Reason), "."), task.Title, req.Title)
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".task-set-title." + slugID(req.TaskID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "task_set_title", TargetRef: domain.Ref{Kind: "task", ID: req.TaskID}, Reason: auditReason, EvidenceRefs: []domain.Ref{{Kind: "evidence", ID: req.EvidenceID}}, Result: "success"}
	if req.DryRun {
		audit.Result = "dry_run"
	}
	result.AuditRecordPreview = audit
	patch, _ := yaml.Marshal(map[string]any{"task": req.TaskID, "title": map[string]string{"before": task.Title, "after": req.Title}, "reason": req.Reason})
	result.PlannedWrites = []PlannedWrite{{Path: "canonical/tasks.yaml", Action: "replace_yaml_field", Format: "yaml-patch-preview", Content: string(patch)}, auditPlannedWrite(audit)}
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, supported := session.target.(TaskWriter)
	if !supported {
		result.Errors = append(result.Errors, taskDiag("storage_write_not_supported", "configured project store does not support task writes", "project", s.loader.ProjectPath()))
		return result, ErrValidationFailed
	}
	path, err := writer.SetTaskTitle(req.TaskID, req.Title)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("task_write_failed", err.Error(), "task", req.TaskID))
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, path)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, taskDiag("audit_write_failed", err.Error(), "audit_record", audit.ID))
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

func validateTaskAddBasics(errors *[]Diagnostic, ws domain.Workspace, req TaskAddRequest) {
	if req.ID == "" {
		*errors = append(*errors, taskDiag("missing_task_id", "task id is required", "task", ""))
	} else if _, ok := findTask(ws.Tasks, req.ID); ok {
		*errors = append(*errors, taskDiag("task_already_exists", fmt.Sprintf("task %s already exists", req.ID), "task", req.ID))
	}
	if req.Title == "" {
		*errors = append(*errors, taskDiag("missing_task_title", "task title is required", "task", req.ID))
	}
	if req.ActorID == "" {
		*errors = append(*errors, taskDiag("missing_actor", "actor is required", "actor", ""))
	} else if !hasActor(ws.Actors, req.ActorID) {
		*errors = append(*errors, taskDiag("actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID))
	}
	if req.EvidenceID == "" {
		*errors = append(*errors, taskDiag("missing_evidence", "evidence is required", "evidence", ""))
	} else if !hasEvidence(ws.Evidence, req.EvidenceID) {
		*errors = append(*errors, taskDiag("evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID))
	}
	if req.AssigneeID != "" && !hasResource(ws.Resources, req.AssigneeID) {
		*errors = append(*errors, taskDiag("assignee_not_found", fmt.Sprintf("assignee resource %s not found", req.AssigneeID), "resource", req.AssigneeID))
	}
	if req.AfterTaskID != "" {
		if _, ok := findTask(ws.Tasks, req.AfterTaskID); !ok {
			*errors = append(*errors, taskDiag("after_task_not_found", fmt.Sprintf("after task %s not found", req.AfterTaskID), "task", req.AfterTaskID))
		} else if !refListHasID(ws.CurrentPlan.IncludedTasks, req.AfterTaskID) {
			*errors = append(*errors, taskDiag("after_task_not_in_current_plan", fmt.Sprintf("after task %s is not included in the current plan", req.AfterTaskID), "task", req.AfterTaskID))
		}
	}
	validateTaskDates(errors, req.PlannedStart, req.PlannedFinish)
	if req.OccurredAt != "" {
		validateTaskOccurredAt(errors, req.OccurredAt, req.ID)
	}
	if len(req.DemandSpecs) == 0 {
		*errors = append(*errors, taskDiag("missing_demand", "at least one demand is required", "task", req.ID))
	}
}

func parseTaskDemandSpecs(errors *[]Diagnostic, ws domain.Workspace, taskID string, specs []string) []domain.ResourceDemand {
	result := []domain.ResourceDemand{}
	seen := map[string]bool{}
	for _, raw := range specs {
		left, right, found := strings.Cut(raw, "=")
		parts := strings.Split(right, ":")
		if !found || !demandSuffixPattern.MatchString(left) || (len(parts) != 3 && len(parts) != 4) || parts[0] == "" || parts[1] == "" || parts[2] == "" {
			*errors = append(*errors, taskDiag("invalid_demand_spec", fmt.Sprintf("demand %q must be SUFFIX=RESOURCE_ID:AMOUNT:UNIT[:ROLE]", raw), "resource_demand", raw))
			continue
		}
		amount, err := strconv.ParseFloat(parts[1], 64)
		if err != nil || math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
			*errors = append(*errors, taskDiag("invalid_demand_amount", fmt.Sprintf("demand %q amount must be a finite number greater than zero", raw), "resource_demand", raw))
			continue
		}
		id := "resource_demand." + taskDemandSlug(taskID) + "." + left
		if seen[id] {
			*errors = append(*errors, taskDiag("duplicate_demand_id", fmt.Sprintf("demand %s is specified more than once", id), "resource_demand", id))
			continue
		}
		seen[id] = true
		if _, ok := findDemand(ws.ResourceDemands, id); ok {
			*errors = append(*errors, taskDiag("demand_already_exists", fmt.Sprintf("resource demand %s already exists", id), "resource_demand", id))
			continue
		}
		resource, ok := findResource(ws.Resources, parts[0])
		if !ok {
			*errors = append(*errors, taskDiag("resource_not_found", fmt.Sprintf("resource %s not found", parts[0]), "resource", parts[0]))
			continue
		}
		if !resourceHasCapacityUnit(ws, resource.ID, parts[2]) {
			*errors = append(*errors, taskDiag("invalid_resource_unit", fmt.Sprintf("resource %s has no capacity in unit %s", resource.ID, parts[2]), "resource", resource.ID))
			continue
		}
		role := left
		if len(parts) == 4 {
			role = parts[3]
		}
		if role == "" {
			*errors = append(*errors, taskDiag("invalid_demand_role", fmt.Sprintf("demand %q role must not be empty", raw), "resource_demand", id))
			continue
		}
		result = append(result, domain.ResourceDemand{ID: id, TaskRef: domain.Ref{Kind: "task", ID: taskID}, ResourceRef: domain.Ref{Kind: "resource", ID: resource.ID}, ResourceClass: resource.Class, Role: role, Unit: parts[2], PlannedAmount: amount, RemainingAmount: domain.Amount(amount), Criticality: "required", Status: "active"})
	}
	return result
}

func validateTaskSetDates(errors *[]Diagnostic, ws domain.Workspace, req TaskSetDatesRequest) (domain.Task, bool) {
	if req.TaskID == "" {
		*errors = append(*errors, taskDiag("missing_task", "task is required", "task", ""))
	}
	task, ok := findTask(ws.Tasks, req.TaskID)
	if req.TaskID != "" && !ok {
		*errors = append(*errors, taskDiag("task_not_found", fmt.Sprintf("task %s not found", req.TaskID), "task", req.TaskID))
	}
	if req.PlannedStart == "" && req.PlannedFinish == "" {
		*errors = append(*errors, taskDiag("missing_planned_date", "at least one of planned-start or planned-finish is required", "task", req.TaskID))
	}
	if req.Reason == "" {
		*errors = append(*errors, taskDiag("missing_reason", "reason is required", "task", req.TaskID))
	}
	if req.ActorID == "" {
		*errors = append(*errors, taskDiag("missing_actor", "actor is required", "actor", ""))
	} else if !hasActor(ws.Actors, req.ActorID) {
		*errors = append(*errors, taskDiag("actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID))
	}
	if req.EvidenceID == "" {
		*errors = append(*errors, taskDiag("missing_evidence", "evidence is required", "evidence", ""))
	} else if !hasEvidence(ws.Evidence, req.EvidenceID) {
		*errors = append(*errors, taskDiag("evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID))
	}
	if req.OccurredAt != "" {
		validateTaskOccurredAt(errors, req.OccurredAt, req.TaskID)
	}
	start, finish := task.PlannedStart, task.PlannedFinish
	if req.PlannedStart != "" {
		start = req.PlannedStart
	}
	if req.PlannedFinish != "" {
		finish = req.PlannedFinish
	}
	validateTaskDates(errors, start, finish)
	if ok && start == task.PlannedStart && finish == task.PlannedFinish {
		*errors = append(*errors, taskDiag("task_dates_unchanged", "planned dates are unchanged", "task", req.TaskID))
	}
	return task, ok
}

func validateTaskRecordActualWindow(errors *[]Diagnostic, ws domain.Workspace, req TaskRecordActualWindowRequest) (domain.Task, bool) {
	if req.TaskID == "" {
		*errors = append(*errors, taskDiag("missing_task", "task is required", "task", ""))
	}
	task, ok := findTask(ws.Tasks, req.TaskID)
	if req.TaskID != "" && !ok {
		*errors = append(*errors, taskDiag("task_not_found", fmt.Sprintf("task %s not found", req.TaskID), "task", req.TaskID))
	}
	if ok && !estimateTaskFinished(task.TrackingStatus) {
		*errors = append(*errors, taskDiag("task_not_finished", fmt.Sprintf("task %s has tracking status %s; expected accepted, done or closed", req.TaskID, task.TrackingStatus), "task", req.TaskID))
	}
	if req.ActorID == "" {
		*errors = append(*errors, taskDiag("missing_actor", "actor is required", "actor", ""))
	} else if !hasActor(ws.Actors, req.ActorID) {
		*errors = append(*errors, taskDiag("actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID))
	}
	if req.EvidenceID == "" {
		*errors = append(*errors, taskDiag("missing_evidence", "evidence is required", "evidence", ""))
	} else if !hasEvidence(ws.Evidence, req.EvidenceID) {
		*errors = append(*errors, taskDiag("evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID))
	}
	if req.OccurredAt != "" {
		validateTaskOccurredAt(errors, req.OccurredAt, req.TaskID)
	}
	return task, ok
}

func validateTaskSetTitle(errors *[]Diagnostic, ws domain.Workspace, req TaskSetTitleRequest) (domain.Task, bool) {
	if req.TaskID == "" {
		*errors = append(*errors, taskDiag("missing_task", "task is required", "task", ""))
	}
	task, ok := findTask(ws.Tasks, req.TaskID)
	if req.TaskID != "" && !ok {
		*errors = append(*errors, taskDiag("task_not_found", fmt.Sprintf("task %s not found", req.TaskID), "task", req.TaskID))
	}
	if strings.TrimSpace(req.Title) == "" {
		*errors = append(*errors, taskDiag("missing_task_title", "title must not be empty or whitespace", "task", req.TaskID))
	}
	if strings.TrimSpace(req.Reason) == "" {
		*errors = append(*errors, taskDiag("missing_reason", "reason is required", "task", req.TaskID))
	}
	if req.ActorID == "" {
		*errors = append(*errors, taskDiag("missing_actor", "actor is required", "actor", ""))
	} else if !hasActor(ws.Actors, req.ActorID) {
		*errors = append(*errors, taskDiag("actor_not_found", fmt.Sprintf("actor %s not found", req.ActorID), "actor", req.ActorID))
	}
	if req.EvidenceID == "" {
		*errors = append(*errors, taskDiag("missing_evidence", "evidence is required", "evidence", ""))
	} else if !hasEvidence(ws.Evidence, req.EvidenceID) {
		*errors = append(*errors, taskDiag("evidence_not_found", fmt.Sprintf("evidence %s not found", req.EvidenceID), "evidence", req.EvidenceID))
	}
	if req.OccurredAt != "" {
		validateTaskOccurredAt(errors, req.OccurredAt, req.TaskID)
	}
	return task, ok
}

func validateTaskDates(errors *[]Diagnostic, start, finish string) {
	validStart, validFinish := true, true
	if start != "" {
		if t, err := time.Parse("2006-01-02", start); err != nil || t.Format("2006-01-02") != start {
			*errors = append(*errors, taskDiag("invalid_planned_start", fmt.Sprintf("planned-start %q must use YYYY-MM-DD", start), "task", ""))
			validStart = false
		}
	}
	if finish != "" {
		if t, err := time.Parse("2006-01-02", finish); err != nil || t.Format("2006-01-02") != finish {
			*errors = append(*errors, taskDiag("invalid_planned_finish", fmt.Sprintf("planned-finish %q must use YYYY-MM-DD", finish), "task", ""))
			validFinish = false
		}
	}
	if validStart && validFinish && start != "" && finish != "" && finish < start {
		*errors = append(*errors, taskDiag("planned_finish_before_start", fmt.Sprintf("planned-finish %s is before planned-start %s", finish, start), "task", ""))
	}
}

func validateTaskOccurredAt(errors *[]Diagnostic, value, taskID string) {
	if _, err := time.Parse(time.RFC3339, value); err != nil {
		*errors = append(*errors, taskDiag("invalid_occurred_at", fmt.Sprintf("occurred-at %q must be RFC3339 with an offset", value), "task", taskID))
	}
}
func taskOccurredAt(value string) string {
	if value != "" {
		return value
	}
	return time.Now().Format(time.RFC3339)
}
func taskDemandSlug(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) >= 3 && parts[0] == "task" {
		return strings.Join(parts[2:], ".")
	}
	return slugID(id)
}
func findResource(resources []domain.Resource, id string) (domain.Resource, bool) {
	for _, r := range resources {
		if r.ID == id {
			return r, true
		}
	}
	return domain.Resource{}, false
}
func resourceHasCapacityUnit(ws domain.Workspace, resourceID, unit string) bool {
	for _, c := range ws.Capacities {
		if c.ResourceRef.ID == resourceID && c.Unit == unit {
			return true
		}
	}
	return false
}
func refListHasID(refs []domain.Ref, id string) bool {
	for _, ref := range refs {
		if ref.ID == id {
			return true
		}
	}
	return false
}
func displayEmptyDate(value string) string {
	if value == "" {
		return "(unset)"
	}
	return value
}
func taskDiag(code, message, kind, id string) Diagnostic {
	return Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}}
}

func taskAddPlannedWrites(task domain.Task, demands []domain.ResourceDemand, after string, audit domain.AuditRecord) []PlannedWrite {
	taskYAML, _ := yaml.Marshal(struct {
		Tasks []domain.Task `yaml:"tasks"`
	}{[]domain.Task{task}})
	demandYAML, _ := yaml.Marshal(struct {
		ResourceDemands []domain.ResourceDemand `yaml:"resource_demands"`
	}{demands})
	planYAML, _ := yaml.Marshal(map[string]any{"insert_task": task.ID, "after": after})
	return []PlannedWrite{{Path: "canonical/tasks.yaml", Action: "append_yaml_record", Format: "yaml", Content: string(taskYAML)}, {Path: "canonical/resource_demands.yaml", Action: "append_yaml_records", Format: "yaml", Content: string(demandYAML)}, {Path: "canonical/current_plan.yaml", Action: "insert_yaml_reference", Format: "yaml-patch-preview", Content: string(planYAML)}, auditPlannedWrite(audit)}
}
func auditPlannedWrite(audit domain.AuditRecord) PlannedWrite {
	data, _ := json.Marshal(audit)
	return PlannedWrite{Path: AuditShardPath(audit.OccurredAt), Action: "append_json_record", Format: "jsonl", Content: string(data) + "\n"}
}
func taskStorageUnsupported(result TaskAddResult) (TaskAddResult, error) {
	result.Errors = append(result.Errors, taskDiag("storage_write_not_supported", "configured project store does not support task writes", "project", ""))
	return result, ErrValidationFailed
}
