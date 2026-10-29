package app

import (
	"context"
	"fmt"
	"math"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

const CalibrationMinimumSample = 8

type CalibrationService struct{ loader WorkspaceLoader }

type CalibrationCalibrateRequest struct {
	BaselineIDs []string
	ResourceIDs []string
	Unit        string
	Tag         string
	Exclusions  map[string]string
	ID          string
	Name        string
	ApprovedBy  string
	OccurredAt  string
	DryRun      bool
}

type CalibrationCalibrateResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	FilePath           string             `json:"file_path"`
	Calibration        domain.Calibration `json:"calibration"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type CalibrationSummary struct {
	ID         string                       `json:"id"`
	Name       string                       `json:"name"`
	ApprovedAt string                       `json:"approved_at"`
	Resources  []string                     `json:"resources"`
	Unit       string                       `json:"unit"`
	Tag        string                       `json:"tag,omitempty"`
	Factor     float64                      `json:"factor"`
	TotalRatio float64                      `json:"total_ratio"`
	P25        float64                      `json:"p25"`
	P75        float64                      `json:"p75"`
	Count      int                          `json:"count"`
	SizeBands  []domain.CalibrationSizeBand `json:"size_bands"`
	SealStatus string                       `json:"seal_status"`
}

type CalibrationListResult struct {
	Calibrations []CalibrationSummary `json:"calibrations"`
	Warnings     []Diagnostic         `json:"warnings"`
	Errors       []Diagnostic         `json:"errors"`
}

type CalibrationShowResult struct {
	Calibration domain.Calibration `json:"calibration"`
	SealStatus  string             `json:"seal_status"`
	Warnings    []Diagnostic       `json:"warnings"`
	Errors      []Diagnostic       `json:"errors"`
}

type CalibrationApplyRequest struct {
	CalibrationID string
	ScenarioID    string
	ActorID       string
	OccurredAt    string
	DryRun        bool
}

type CalibrationApplySkip struct {
	DemandID string `json:"demand_id"`
	Reason   string `json:"reason"`
}

type CalibrationApplyResult struct {
	DryRun             bool                   `json:"dry_run"`
	Applied            bool                   `json:"applied"`
	CalibrationID      string                 `json:"calibration_id"`
	Scenario           domain.Scenario        `json:"scenario"`
	Skipped            []CalibrationApplySkip `json:"skipped"`
	PlannedWrites      []PlannedWrite         `json:"planned_writes"`
	WrittenFiles       []string               `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord     `json:"audit_record_preview"`
	Validation         *ValidateResult        `json:"validation,omitempty"`
	Warnings           []Diagnostic           `json:"warnings"`
	Errors             []Diagnostic           `json:"errors"`
}

type CalibrationSuggestion struct {
	BaselineID string
	Resources  []string
	Unit       string
	Count      int
}

type CalibratedDemandValue struct {
	DemandID      string
	TypedAmount   float64
	Calibrated    float64
	Unit          string
	CalibrationID string
}

type CalibrationOpenDemands struct {
	CalibrationID string
	Demands       []CalibratedDemandValue
}

type calibrationSuggestionKey struct {
	baseline, unit, resources string
}

type CalibrationWriter interface {
	CreateProjectFile(rel string, content []byte) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
}

func NewCalibrationService(loader WorkspaceLoader) CalibrationService {
	return CalibrationService{loader: loader}
}

func (s CalibrationService) Calibrate(ctx context.Context, req CalibrationCalibrateRequest) (CalibrationCalibrateResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := CalibrationCalibrateResult{DryRun: req.DryRun, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	baselines := validateCalibrationRequest(&result.Errors, ws, req)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	approvedAt := req.OccurredAt
	if approvedAt == "" {
		approvedAt = time.Now().Format(time.RFC3339)
	}
	calibration := buildCalibration(ws, baselines, req, approvedAt)
	if calibration.Result.Count < CalibrationMinimumSample {
		result.Calibration = calibration
		result.Errors = append(result.Errors, Diagnostic{
			Code: "calibration_sample_too_small", Message: fmt.Sprintf("calibration %s has %d clean tasks; at least %d are required", req.ID, calibration.Result.Count, CalibrationMinimumSample),
			Severity: "error", Target: Target{Kind: "calibration", ID: req.ID},
		})
		return result, ErrValidationFailed
	}
	if err := domain.SealCalibration(&calibration); err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "calibration_seal_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "calibration", ID: req.ID}})
		return result, err
	}
	result.Calibration = calibration
	path := filepath.ToSlash(filepath.Join("calibrations", req.ID+".yaml"))
	data, err := yaml.Marshal(struct {
		Calibration domain.Calibration `yaml:"calibration"`
	}{Calibration: calibration})
	if err != nil {
		return result, err
	}
	audit := domain.AuditRecord{
		ID: "audit." + compactTimestamp(approvedAt) + ".estimate-calibrate." + slugID(req.ID), OccurredAt: approvedAt,
		ActorRef: domain.Ref{Kind: "actor", ID: req.ApprovedBy}, Operation: "estimate_calibrate", TargetRef: domain.Ref{Kind: "calibration", ID: req.ID},
		Result: "success", Reason: fmt.Sprintf("Approved estimate calibration from %d clean tasks.", calibration.Result.Count),
	}
	if req.DryRun {
		audit.Result = "dry_run"
		audit.Reason = fmt.Sprintf("Preview estimate calibration from %d clean tasks.", calibration.Result.Count)
	}
	result.FilePath, result.AuditRecordPreview = path, audit
	result.PlannedWrites = []PlannedWrite{{Path: path, Action: "create", Format: "yaml", Content: string(data)}, auditPlannedWrite(audit)}
	if req.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, ok := session.target.(CalibrationWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "storage_write_not_supported", Message: "configured project store does not support calibration writes", Severity: "error"})
		return result, ErrValidationFailed
	}
	calibrationPath, err := writer.CreateProjectFile(path, data)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "calibration_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "calibration", ID: req.ID}})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, calibrationPath)
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

func validateCalibrationRequest(errors *[]Diagnostic, ws domain.Workspace, req CalibrationCalibrateRequest) []domain.Baseline {
	add := func(code, message, kind, id string) {
		*errors = append(*errors, Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: kind, ID: id}})
	}
	if req.ID == "" {
		add("missing_calibration_id", "calibration id is required", "calibration", "")
	}
	if req.Name == "" {
		add("missing_calibration_name", "calibration name is required", "calibration", req.ID)
	}
	if len(req.BaselineIDs) == 0 {
		add("missing_baseline_id", "at least one baseline is required", "baseline", "")
	}
	if len(req.ResourceIDs) == 0 {
		add("missing_calibration_resource", "at least one resource is required", "calibration", req.ID)
	}
	if req.Unit == "" {
		add("missing_calibration_unit", "calibration unit is required", "calibration", req.ID)
	}
	if req.OccurredAt != "" {
		if _, err := time.Parse(time.RFC3339, req.OccurredAt); err != nil {
			add("invalid_occurred_at", fmt.Sprintf("occurred-at %q must be RFC3339", req.OccurredAt), "calibration", req.ID)
		}
	}
	for _, existing := range ws.Calibrations {
		if existing.ID == req.ID {
			add("calibration_already_exists", fmt.Sprintf("calibration %s already exists", req.ID), "calibration", req.ID)
		}
	}
	actor, actorOK := findActor(ws.Actors, req.ApprovedBy)
	if req.ApprovedBy == "" || !actorOK {
		add("actor_not_found", fmt.Sprintf("actor %s not found", req.ApprovedBy), "actor", req.ApprovedBy)
	} else if actor.Kind != "human" {
		add("approver_not_human", fmt.Sprintf("calibration approver %s is %s, not human", req.ApprovedBy, actor.Kind), "actor", req.ApprovedBy)
	}
	resourceSet := map[string]bool{}
	for _, resource := range ws.Resources {
		resourceSet[resource.ID] = true
	}
	for _, id := range req.ResourceIDs {
		if !resourceSet[id] {
			add("resource_not_found", fmt.Sprintf("resource %s not found", id), "resource", id)
		}
	}
	var baselines []domain.Baseline
	for _, id := range req.BaselineIDs {
		baseline, ok := estimateBaseline(ws.Baselines, id)
		if !ok {
			add("baseline_not_found", fmt.Sprintf("baseline %s not found", id), "baseline", id)
			continue
		}
		if !domain.BaselineCarriesEstimates(baseline) {
			add("baseline_has_no_estimates", fmt.Sprintf("baseline %s has no estimate snapshot", id), "baseline", id)
			continue
		}
		if status := baselineSealStatus(baseline); status != "verified" {
			add("baseline_modified", fmt.Sprintf("baseline %s seal does not verify (status %s)", id, status), "baseline", id)
			continue
		}
		baselines = append(baselines, baseline)
	}
	return baselines
}

type calibrationObservation struct {
	taskID                  string
	baseline, actual, ratio float64
}

func buildCalibration(ws domain.Workspace, baselines []domain.Baseline, req CalibrationCalibrateRequest, approvedAt string) domain.Calibration {
	resources := uniqueSorted(req.ResourceIDs)
	calibration := domain.Calibration{
		ID: req.ID, Name: req.Name, ApprovedBy: domain.Ref{Kind: "actor", ID: req.ApprovedBy}, ApprovedAt: approvedAt,
		Scope:  domain.CalibrationScope{Resources: resources, Unit: req.Unit, Tag: req.Tag},
		Sample: domain.CalibrationSample{Tasks: []string{}, Excluded: []domain.CalibrationExclusion{}},
		Result: domain.CalibrationResult{SizeBands: []domain.CalibrationSizeBand{}},
	}
	for _, baseline := range baselines {
		calibration.SourceBaselines = append(calibration.SourceBaselines, domain.CalibrationSourceBaseline{ID: baseline.ID, SealSHA256: baseline.Seal.Digest})
	}
	sort.Slice(calibration.SourceBaselines, func(i, j int) bool { return calibration.SourceBaselines[i].ID < calibration.SourceBaselines[j].ID })
	observations, excluded := calibrationObservations(ws, baselines, calibration.Scope, req.Exclusions)
	for _, observation := range observations {
		calibration.Sample.Tasks = append(calibration.Sample.Tasks, observation.taskID)
	}
	calibration.Sample.Excluded = excluded
	calibration.Result = summarizeCalibration(observations)
	return calibration
}

func calibrationObservations(ws domain.Workspace, baselines []domain.Baseline, scope domain.CalibrationScope, explicit map[string]string) ([]calibrationObservation, []domain.CalibrationExclusion) {
	tasks := map[string]domain.Task{}
	demands := map[string]domain.ResourceDemand{}
	demandIDsByTask := map[string][]string{}
	for _, task := range ws.Tasks {
		tasks[task.ID] = task
	}
	for _, demand := range ws.ResourceDemands {
		demands[demand.ID] = demand
		demandIDsByTask[demand.TaskRef.ID] = append(demandIDsByTask[demand.TaskRef.ID], demand.ID)
	}
	// A task present in several source baselines uses the latest baseline that
	// was still approved before its start.
	type candidate struct {
		baseline  domain.Baseline
		amount    float64
		demandIDs []string
	}
	byTask := map[string][]candidate{}
	resourceSet := stringSet(scope.Resources)
	for _, baseline := range baselines {
		baselineTasks := map[string]bool{}
		for _, task := range baseline.IncludedTasks {
			baselineTasks[task.ID] = true
		}
		amounts := map[string]float64{}
		ids := map[string][]string{}
		for _, demand := range baseline.IncludedDemands {
			if demand.Unit == scope.Unit && resourceSet[demand.Resource] {
				amounts[demand.Task] += estimateBaselineAmount(demand)
				ids[demand.Task] = append(ids[demand.Task], demand.ID)
			}
		}
		for taskID := range baselineTasks {
			byTask[taskID] = append(byTask[taskID], candidate{baseline: baseline, amount: amounts[taskID], demandIDs: ids[taskID]})
		}
	}
	var observations []calibrationObservation
	var excluded []domain.CalibrationExclusion
	for taskID, candidates := range byTask {
		task, exists := tasks[taskID]
		reason := ""
		if explicit[taskID] != "" {
			reason = explicit[taskID]
		} else if !exists || !estimateTaskFinished(task.TrackingStatus) {
			reason = "not_finished"
		} else if scope.Tag != "" && !containsString(task.Tags, scope.Tag) {
			reason = "tag_mismatch"
		}
		allDemandIDs := append([]string(nil), demandIDsByTask[taskID]...)
		for _, c := range candidates {
			allDemandIDs = append(allDemandIDs, c.demandIDs...)
		}
		window := deriveTaskActualWindow(ws, taskID, uniqueSorted(allDemandIDs), false)
		if reason == "" && window.Start == "" {
			reason = "actual_start_unknown"
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].baseline.ApprovedAt > candidates[j].baseline.ApprovedAt })
		var selected *candidate
		if reason == "" {
			for i := range candidates {
				if baselineApprovedBeforeStart(candidates[i].baseline, window) {
					selected = &candidates[i]
					break
				}
			}
			if selected == nil {
				reason = "estimated_after_start"
			}
		}
		if reason == "" && selected.amount <= 0 {
			reason = "baseline_amount_not_positive"
		}
		actual, recorded := calibrationTaskActual(ws, taskID, scope, demands)
		if reason == "" && !recorded {
			reason = "no_actuals"
		}
		if reason != "" {
			excluded = append(excluded, domain.CalibrationExclusion{Task: taskID, Reason: reason})
			continue
		}
		observations = append(observations, calibrationObservation{taskID: taskID, baseline: selected.amount, actual: actual, ratio: actual / selected.amount})
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].taskID < observations[j].taskID })
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].Task < excluded[j].Task })
	return observations, excluded
}

func baselineApprovedBeforeStart(baseline domain.Baseline, window taskActualWindow) bool {
	approvedAt := firstNonEmpty(baseline.ApprovedAt, baseline.CreatedAt)
	if approved, approvedErr := time.Parse(time.RFC3339, approvedAt); approvedErr == nil && window.StartOccurredAt != "" {
		started, startedErr := time.Parse(time.RFC3339, window.StartOccurredAt)
		return startedErr == nil && approved.Before(started)
	}
	approvedDate := datePart(approvedAt)
	return approvedDate != "" && approvedDate < window.Start
}

func calibrationTaskActual(ws domain.Workspace, taskID string, scope domain.CalibrationScope, demands map[string]domain.ResourceDemand) (float64, bool) {
	resources := stringSet(scope.Resources)
	amount, recorded := 0.0, false
	for _, actual := range ws.Actuals {
		if !estimateConsumptionTypes[actual.Type] || actual.Unit != scope.Unit {
			continue
		}
		demand := demands[actual.DemandRef.ID]
		actualTask := actual.TaskRef.ID
		if actualTask == "" {
			actualTask = demand.TaskRef.ID
		}
		if actualTask != taskID {
			continue
		}
		resourceID := actual.ResourceRef.ID
		if resourceID == "" {
			resourceID = demandResourceID(demand)
		}
		if resources[resourceID] {
			amount += actual.Amount
			recorded = true
		}
	}
	return amount, recorded
}

func summarizeCalibration(observations []calibrationObservation) domain.CalibrationResult {
	result := domain.CalibrationResult{Count: len(observations), SizeBands: []domain.CalibrationSizeBand{}}
	if len(observations) == 0 {
		return result
	}
	ratios := make([]float64, 0, len(observations))
	totalBaseline, totalActual := 0.0, 0.0
	for _, observation := range observations {
		ratios = append(ratios, observation.ratio)
		totalBaseline += observation.baseline
		totalActual += observation.actual
	}
	sort.Float64s(ratios)
	result.Factor = roundEstimate(percentile(ratios, .5))
	result.TotalRatio = roundEstimate(totalActual / totalBaseline)
	result.P25 = roundEstimate(percentile(ratios, .25))
	result.P75 = roundEstimate(percentile(ratios, .75))
	bounds := []*float64{floatPointer(2), floatPointer(4), nil}
	for bandIndex, bound := range bounds {
		var values []float64
		for _, observation := range observations {
			if calibrationBandIndex(observation.baseline) == bandIndex {
				values = append(values, observation.ratio)
			}
		}
		if len(values) == 0 {
			continue
		}
		sort.Float64s(values)
		result.SizeBands = append(result.SizeBands, domain.CalibrationSizeBand{UpTo: bound, Count: len(values), Median: roundEstimate(percentile(values, .5))})
	}
	return result
}

func percentile(sorted []float64, quantile float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := quantile * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func calibrationBandIndex(amount float64) int {
	if amount <= 2 {
		return 0
	}
	if amount <= 4 {
		return 1
	}
	return 2
}

func (s CalibrationService) List(ctx context.Context) (CalibrationListResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := CalibrationListResult{Calibrations: []CalibrationSummary{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, diags)
	for _, calibration := range ws.Calibrations {
		result.Calibrations = append(result.Calibrations, calibrationSummary(calibration))
	}
	sort.Slice(result.Calibrations, func(i, j int) bool { return result.Calibrations[i].ID < result.Calibrations[j].ID })
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	return result, nil
}

func (s CalibrationService) Show(ctx context.Context, id string) (CalibrationShowResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := CalibrationShowResult{Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, diags)
	for _, calibration := range ws.Calibrations {
		if calibration.ID == id {
			result.Calibration, result.SealStatus = calibration, calibrationSealStatus(calibration)
			if result.SealStatus != "verified" {
				result.Errors = append(result.Errors, Diagnostic{Code: "calibration_modified", Message: fmt.Sprintf("calibration %s seal does not verify", id), Severity: "error", Target: Target{Kind: "calibration", ID: id}})
				return result, ErrValidationFailed
			}
			return result, nil
		}
	}
	result.Errors = append(result.Errors, Diagnostic{Code: "calibration_not_found", Message: fmt.Sprintf("calibration %s not found", id), Severity: "error", Target: Target{Kind: "calibration", ID: id}})
	return result, ErrValidationFailed
}

func (s CalibrationService) Apply(ctx context.Context, req CalibrationApplyRequest) (CalibrationApplyResult, error) {
	ws, diags := s.loader.Load(ctx)
	result := CalibrationApplyResult{DryRun: req.DryRun, CalibrationID: req.CalibrationID, Skipped: []CalibrationApplySkip{}, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, diags)
	calibration, ok := findCalibration(ws.Calibrations, req.CalibrationID)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "calibration_not_found", Message: fmt.Sprintf("calibration %s not found", req.CalibrationID), Severity: "error", Target: Target{Kind: "calibration", ID: req.CalibrationID}})
	}
	if ok && calibrationSealStatus(calibration) != "verified" {
		result.Errors = append(result.Errors, Diagnostic{Code: "calibration_modified", Message: fmt.Sprintf("calibration %s seal does not verify", req.CalibrationID), Severity: "error", Target: Target{Kind: "calibration", ID: req.CalibrationID}})
	}
	if req.ScenarioID == "" {
		result.Errors = append(result.Errors, Diagnostic{Code: "scenario_id_required", Message: "scenario-id is required", Severity: "error"})
	}
	if _, exists := findScenario(ws.Scenarios, req.ScenarioID); exists {
		result.Errors = append(result.Errors, Diagnostic{Code: "scenario_already_exists", Message: fmt.Sprintf("scenario %s already exists", req.ScenarioID), Severity: "error", Target: Target{Kind: "scenario", ID: req.ScenarioID}})
	}
	if !hasActor(ws.Actors, req.ActorID) {
		result.Errors = append(result.Errors, Diagnostic{Code: "actor_not_found", Message: fmt.Sprintf("actor %s not found", req.ActorID), Severity: "error", Target: Target{Kind: "actor", ID: req.ActorID}})
	}
	if req.OccurredAt != "" {
		if _, err := time.Parse(time.RFC3339, req.OccurredAt); err != nil {
			result.Errors = append(result.Errors, Diagnostic{Code: "invalid_occurred_at", Message: fmt.Sprintf("occurred-at %q must be RFC3339", req.OccurredAt), Severity: "error", Target: Target{Kind: "scenario", ID: req.ScenarioID}})
		}
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	occurredAt := req.OccurredAt
	if occurredAt == "" {
		occurredAt = time.Now().Format(time.RFC3339)
	}
	scenario := buildCalibrationScenario(ws, calibration, req.ScenarioID, req.ActorID, occurredAt, &result.Skipped)
	result.Scenario = scenario
	data, err := yaml.Marshal(struct {
		Scenario domain.Scenario `yaml:"scenario"`
	}{Scenario: scenario})
	if err != nil {
		return result, err
	}
	path := filepath.ToSlash(filepath.Join("scenarios", scenario.ID+".yaml"))
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".estimate-apply." + slugID(scenario.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "estimate_apply", TargetRef: domain.Ref{Kind: "scenario", ID: scenario.ID}, Result: "success", Reason: fmt.Sprintf("Created scenario from calibration %s.", calibration.ID)}
	if req.DryRun {
		audit.Result = "dry_run"
		audit.Reason = fmt.Sprintf("Preview scenario from calibration %s.", calibration.ID)
	}
	result.AuditRecordPreview = audit
	result.PlannedWrites = []PlannedWrite{{Path: path, Action: "create", Format: "yaml", Content: string(data)}, auditPlannedWrite(audit)}
	if req.DryRun {
		return result, nil
	}
	session := beginWrite(s.loader)
	writer, supported := session.target.(CalibrationWriter)
	if !supported {
		return result, ErrValidationFailed
	}
	scenarioPath, err := writer.CreateProjectFile(path, data)
	if err != nil {
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, scenarioPath)
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

func buildCalibrationScenario(ws domain.Workspace, calibration domain.Calibration, scenarioID, actorID, occurredAt string, skipped *[]CalibrationApplySkip) domain.Scenario {
	scenario := domain.Scenario{ID: scenarioID, Name: "Apply " + calibration.Name, Status: "draft", BaseRef: domain.Ref{Kind: "current_plan", ID: ws.CurrentPlan.ID}, CreatedAt: occurredAt, CreatedBy: domain.Ref{Kind: "actor", ID: actorID}, CalibrationRef: domain.Ref{Kind: "calibration", ID: calibration.ID}, Changes: []domain.ScenarioChange{}}
	tasks := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		tasks[task.ID] = task
	}
	actualByDemand := map[string]float64{}
	for _, actual := range ws.Actuals {
		if estimateConsumptionTypes[actual.Type] && actual.Unit == calibration.Scope.Unit {
			actualByDemand[actual.DemandRef.ID] += actual.Amount
		}
	}
	resources := stringSet(calibration.Scope.Resources)
	for _, demand := range ws.ResourceDemands {
		if !resources[demandResourceID(demand)] || demand.Unit != calibration.Scope.Unit || (calibration.Scope.Tag != "" && !containsString(tasks[demand.TaskRef.ID].Tags, calibration.Scope.Tag)) {
			continue
		}
		if calibrationDemandClosed(demand.Status) {
			*skipped = append(*skipped, CalibrationApplySkip{DemandID: demand.ID, Reason: "closed"})
			continue
		}
		scaledPlanned := RoundCalibrationAmount(demand.PlannedAmount * calibration.Result.Factor)
		if actualByDemand[demand.ID] > scaledPlanned {
			*skipped = append(*skipped, CalibrationApplySkip{DemandID: demand.ID, Reason: "actuals_above_scaled_amount"})
			continue
		}
		if scaledPlanned != demand.PlannedAmount {
			scenario.Changes = append(scenario.Changes, domain.ScenarioChange{Op: "replace", Path: "resource_demands." + demand.ID + ".planned_amount", Value: scaledPlanned})
		}
		if demand.HasRemaining() {
			scaledRemaining := RoundCalibrationAmount(demand.Remaining() * calibration.Result.Factor)
			if scaledRemaining != demand.Remaining() {
				scenario.Changes = append(scenario.Changes, domain.ScenarioChange{Op: "replace", Path: "resource_demands." + demand.ID + ".remaining_amount", Value: scaledRemaining})
			}
		}
	}
	sort.Slice(scenario.Changes, func(i, j int) bool { return scenario.Changes[i].Path < scenario.Changes[j].Path })
	sort.Slice(*skipped, func(i, j int) bool { return (*skipped)[i].DemandID < (*skipped)[j].DemandID })
	return scenario
}

func RoundCalibrationAmount(value float64) float64 {
	rounded := math.Round(value*4) / 4
	if rounded < .25 {
		return .25
	}
	return rounded
}

func calibrationSummary(calibration domain.Calibration) CalibrationSummary {
	return CalibrationSummary{ID: calibration.ID, Name: calibration.Name, ApprovedAt: calibration.ApprovedAt, Resources: append([]string(nil), calibration.Scope.Resources...), Unit: calibration.Scope.Unit, Tag: calibration.Scope.Tag, Factor: calibration.Result.Factor, TotalRatio: calibration.Result.TotalRatio, P25: calibration.Result.P25, P75: calibration.Result.P75, Count: calibration.Result.Count, SizeBands: append([]domain.CalibrationSizeBand(nil), calibration.Result.SizeBands...), SealStatus: calibrationSealStatus(calibration)}
}

func calibrationSealStatus(calibration domain.Calibration) string {
	if calibration.SealSHA256 == "" {
		return "unsealed"
	}
	digest, err := domain.CalibrationDigest(calibration)
	if err != nil || digest != calibration.SealSHA256 {
		return "modified"
	}
	return "verified"
}

func findCalibration(calibrations []domain.Calibration, id string) (domain.Calibration, bool) {
	for _, calibration := range calibrations {
		if calibration.ID == id {
			return calibration, true
		}
	}
	return domain.Calibration{}, false
}

func findActor(actors []domain.Actor, id string) (domain.Actor, bool) {
	for _, actor := range actors {
		if actor.ID == id {
			return actor, true
		}
	}
	return domain.Actor{}, false
}

func uniqueSorted(values []string) []string {
	set := stringSet(values)
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func stringSet(values []string) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			result[value] = true
		}
	}
	return result
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func floatPointer(value float64) *float64 { return &value }

func CalibratedDemandValues(ws domain.Workspace) []CalibratedDemandValue {
	tasks := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		tasks[task.ID] = task
	}
	values := []CalibratedDemandValue{}
	for _, demand := range ws.ResourceDemands {
		if calibrationDemandClosed(demand.Status) {
			continue
		}
		var selected *domain.Calibration
		for i := range ws.Calibrations {
			calibration := &ws.Calibrations[i]
			if calibrationSealStatus(*calibration) != "verified" || calibration.Scope.Unit != demand.Unit || !containsString(calibration.Scope.Resources, demandResourceID(demand)) {
				continue
			}
			if calibration.Scope.Tag != "" && !containsString(tasks[demand.TaskRef.ID].Tags, calibration.Scope.Tag) {
				continue
			}
			if selected == nil || calibration.ApprovedAt > selected.ApprovedAt {
				selected = calibration
			}
		}
		if selected != nil {
			values = append(values, CalibratedDemandValue{DemandID: demand.ID, TypedAmount: demand.PlannedAmount, Calibrated: RoundCalibrationAmount(demand.PlannedAmount * selected.Result.Factor), Unit: demand.Unit, CalibrationID: selected.ID})
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].DemandID < values[j].DemandID })
	return values
}

// CalibrationOpenDemandGroups returns one application candidate per sealed
// calibration. A calibration disappears after any scenario records its ref.
func CalibrationOpenDemandGroups(ws domain.Workspace) []CalibrationOpenDemands {
	used := map[string]bool{}
	for _, scenario := range ws.Scenarios {
		if scenario.CalibrationRef.ID != "" {
			used[scenario.CalibrationRef.ID] = true
		}
	}
	tasks := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		tasks[task.ID] = task
	}
	groups := []CalibrationOpenDemands{}
	for _, calibration := range ws.Calibrations {
		if used[calibration.ID] || calibrationSealStatus(calibration) != "verified" {
			continue
		}
		group := CalibrationOpenDemands{CalibrationID: calibration.ID, Demands: []CalibratedDemandValue{}}
		for _, demand := range ws.ResourceDemands {
			if calibrationDemandClosed(demand.Status) || demand.Unit != calibration.Scope.Unit || !containsString(calibration.Scope.Resources, demandResourceID(demand)) {
				continue
			}
			if calibration.Scope.Tag != "" && !containsString(tasks[demand.TaskRef.ID].Tags, calibration.Scope.Tag) {
				continue
			}
			group.Demands = append(group.Demands, CalibratedDemandValue{DemandID: demand.ID, TypedAmount: demand.PlannedAmount, Calibrated: RoundCalibrationAmount(demand.PlannedAmount * calibration.Result.Factor), Unit: demand.Unit, CalibrationID: calibration.ID})
		}
		if len(group.Demands) > 0 {
			sort.Slice(group.Demands, func(i, j int) bool { return group.Demands[i].DemandID < group.Demands[j].DemandID })
			groups = append(groups, group)
		}
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].CalibrationID < groups[j].CalibrationID })
	return groups
}

// CalibrationSuggestions finds uncalibrated baseline scopes with enough
// clean finished tasks. Explicit judgement exclusions remain an operator
// choice, so next suggests the command rather than creating a record.
func CalibrationSuggestions(ws domain.Workspace) []CalibrationSuggestion {
	candidates := map[calibrationSuggestionKey]CalibrationSuggestion{}
	for _, baseline := range ws.Baselines {
		if !domain.BaselineCarriesEstimates(baseline) || baselineSealStatus(baseline) != "verified" {
			continue
		}
		scopes := map[calibrationSuggestionKey][]string{}
		for _, demand := range baseline.IncludedDemands {
			if estimateBaselineAmount(demand) > 0 {
				key := calibrationSuggestionKey{baseline: baseline.ID, unit: demand.Unit, resources: demand.Resource}
				scopes[key] = []string{demand.Resource}
			}
		}
		report := EstimateReportResult{Resources: []EstimateResourceSummary{}, SplitGroups: []EstimateSplitGroupSummary{}, Tasks: []EstimateTask{}}
		buildEstimateReport(&report, ws, baseline)
		for _, group := range report.SplitGroups {
			resources := uniqueSorted(append([]string{group.OriginalResourceID}, group.SplitResourceIDs...))
			key := calibrationSuggestionKey{baseline: baseline.ID, unit: group.Unit, resources: strings.Join(resources, "\x00")}
			scopes[key] = resources
		}
		for key, resources := range scopes {
			if calibrationScopeHasStrictSuperset(key, resources, scopes) {
				continue
			}
			addCalibrationSuggestion(candidates, ws, baseline, resources, key.unit)
		}
	}
	result := []CalibrationSuggestion{}
	for _, suggestion := range candidates {
		result = append(result, suggestion)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].BaselineID != result[j].BaselineID {
			return result[i].BaselineID < result[j].BaselineID
		}
		if result[i].Unit != result[j].Unit {
			return result[i].Unit < result[j].Unit
		}
		return strings.Join(result[i].Resources, "\x00") < strings.Join(result[j].Resources, "\x00")
	})
	return result
}

func addCalibrationSuggestion(candidates map[calibrationSuggestionKey]CalibrationSuggestion, ws domain.Workspace, baseline domain.Baseline, resources []string, unit string) {
	resources = uniqueSorted(resources)
	observations, _ := calibrationObservations(ws, []domain.Baseline{baseline}, domain.CalibrationScope{Resources: resources, Unit: unit}, nil)
	count := 0
	for _, observation := range observations {
		if !calibrationTaskSampleUsed(ws.Calibrations, observation.taskID, resources, unit) {
			count++
		}
	}
	if count < CalibrationMinimumSample {
		return
	}
	key := calibrationSuggestionKey{baseline: baseline.ID, unit: unit, resources: strings.Join(resources, "\x00")}
	candidates[key] = CalibrationSuggestion{BaselineID: baseline.ID, Resources: resources, Unit: unit, Count: count}
}

func calibrationScopeHasStrictSuperset(key calibrationSuggestionKey, resources []string, scopes map[calibrationSuggestionKey][]string) bool {
	for otherKey, otherResources := range scopes {
		if key.baseline == otherKey.baseline && key.unit == otherKey.unit && len(otherResources) > len(resources) && resourceSuperset(otherResources, resources) {
			return true
		}
	}
	return false
}

func calibrationTaskSampleUsed(calibrations []domain.Calibration, taskID string, resources []string, unit string) bool {
	for _, calibration := range calibrations {
		if calibrationSealStatus(calibration) != "verified" || calibration.Scope.Unit != unit || !resourceSuperset(calibration.Scope.Resources, resources) {
			continue
		}
		if containsString(calibration.Sample.Tasks, taskID) {
			return true
		}
	}
	return false
}

func resourceSuperset(superset, subset []string) bool {
	values := stringSet(superset)
	for _, value := range subset {
		if !values[value] {
			return false
		}
	}
	return true
}

func calibrationDemandClosed(status string) bool {
	switch status {
	case "consumed", "released", "cancelled", "superseded", "closed":
		return true
	default:
		return false
	}
}
