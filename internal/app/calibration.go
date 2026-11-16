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
	"github.com/kogriv/replan/internal/storage/yamlformat"
)

const CalibrationMinimumSample = 8

type CalibrationService struct{ loader WorkspaceLoader }

type CalibrationCalibrateRequest struct {
	BaselineIDs        []string
	AllBaselines       bool
	Prior              *float64
	PriorSource        string
	ResourceIDs        []string
	Unit               string
	Tag                string
	Since              string
	IncludeReviewReady bool
	Exclusions         map[string]string
	ID                 string
	Name               string
	ApprovedBy         string
	OccurredAt         string
	DryRun             bool
	BySize             bool
}

type CalibrationCalibrateResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	FilePath           string             `json:"file_path"`
	Calibration        domain.Calibration `json:"calibration"`
	RequireMeasurement []string           `json:"require_measurement"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type CalibrationSummary struct {
	ID               string                       `json:"id"`
	Name             string                       `json:"name"`
	ApprovedAt       string                       `json:"approved_at"`
	Resources        []string                     `json:"resources"`
	Unit             string                       `json:"unit"`
	Tag              string                       `json:"tag,omitempty"`
	Since            string                       `json:"since,omitempty"`
	Factor           float64                      `json:"factor"`
	Method           string                       `json:"method,omitempty"`
	PriorSource      string                       `json:"prior_source,omitempty"`
	TotalRatio       float64                      `json:"total_ratio"`
	P25              float64                      `json:"p25"`
	P75              float64                      `json:"p75"`
	Count            int                          `json:"count"`
	ReviewReadyCount int                          `json:"review_ready_count,omitempty"`
	SizeBands        []domain.CalibrationSizeBand `json:"size_bands"`
	SealStatus       string                       `json:"seal_status"`
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
	BaselineID         string
	AllBaselines       bool
	Resources          []string
	Unit               string
	Tag                string
	Count              int
	ReplacesPriorID    string
	Since              string
	ReplacesID         string
	BySize             bool
	IncludeReviewReady bool
}

type CalibratedDemandValue struct {
	DemandID      string  `json:"demand_id"`
	TypedAmount   float64 `json:"typed_amount"`
	Calibrated    float64 `json:"calibrated_amount"`
	Unit          string  `json:"unit"`
	CalibrationID string  `json:"calibration_id"`
	Factor        float64 `json:"factor"`
	Band          string  `json:"band,omitempty"`
}

type CalibrationOpenDemands struct {
	CalibrationID           string
	SupersedesCalibrationID string
	Demands                 []CalibratedDemandValue
}

// DemandCalibration is the approved calibration selected for one open demand.
// TaskScopeAmount is the task's total planned amount in the calibration scope.
type DemandCalibration struct {
	Calibration     domain.Calibration
	TaskScopeAmount float64
	Factor          float64
	Band            string
}

type calibrationSuggestionKey struct {
	baseline, unit, resources string
}

type CalibrationWriter interface {
	CreateProjectFile(rel string, content []byte) (string, error)
	AppendAudit(record *domain.AuditRecord) (string, error)
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
	result.RequireMeasurement = append([]string(nil), calibration.Scope.RequireMeasurement...)
	if req.Prior == nil && calibration.Result.Count < CalibrationMinimumSample {
		result.Calibration = calibration
		result.Errors = append(result.Errors, Diagnostic{
			Code: "calibration_sample_too_small", Message: fmt.Sprintf("calibration %s has %d clean tasks; at least %d are required", req.ID, calibration.Result.Count, CalibrationMinimumSample),
			Severity: "error", Target: Target{Kind: "calibration", ID: req.ID},
		})
		return result, ErrValidationFailed
	}
	if req.BySize {
		anyBand := false
		counts := make([]string, 0, len(calibration.Result.SizeBands))
		for i, band := range calibration.Result.SizeBands {
			anyBand = anyBand || band.Count >= CalibrationMinimumSample
			counts = append(counts, fmt.Sprintf("%s=%d", calibrationBandName(i), band.Count))
		}
		if !anyBand {
			result.Warnings = append(result.Warnings, Diagnostic{Code: "calibration_size_bands_below_minimum", Message: "no size band reaches the minimum sample: " + strings.Join(counts, ", "), Severity: "warning", Target: Target{Kind: "calibration", ID: req.ID}})
		}
	}
	if err := domain.SealCalibration(&calibration); err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "calibration_seal_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "calibration", ID: req.ID}})
		return result, err
	}
	result.Calibration = calibration
	path := filepath.ToSlash(filepath.Join("calibrations", req.ID+".yaml"))
	data, err := yamlformat.Render(struct {
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
	if req.Prior != nil {
		audit.Reason = fmt.Sprintf("Approved starting estimate factor from %s.", calibration.Result.PriorSource)
	}
	if req.DryRun {
		audit.Result = "dry_run"
		audit.Reason = fmt.Sprintf("Preview estimate calibration from %d clean tasks.", calibration.Result.Count)
		if req.Prior != nil {
			audit.Reason = fmt.Sprintf("Preview starting estimate factor from %s.", calibration.Result.PriorSource)
		}
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
	auditPath, err := writer.AppendAudit(&audit)
	result.AuditRecordPreview = audit
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
	validateCalibrationPriorRequest(errors, req)
	if len(req.BaselineIDs) == 0 && req.Prior == nil {
		if !req.AllBaselines {
			add("missing_baseline_id", "at least one baseline is required", "baseline", "")
		}
	}
	if req.AllBaselines && len(req.BaselineIDs) > 0 {
		add("calibration_baseline_flags_conflict", "--all-baselines cannot be combined with --baseline", "calibration", req.ID)
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
	if req.Since != "" {
		if _, err := time.Parse(calendarDateLayout, req.Since); err != nil {
			add("invalid_since", fmt.Sprintf("since %q must be YYYY-MM-DD", req.Since), "calibration", req.ID)
		} else if ws.Project.DefaultStatusDate != "" && req.Since > ws.Project.DefaultStatusDate {
			add("invalid_since", fmt.Sprintf("since %s is after project status date %s", req.Since, ws.Project.DefaultStatusDate), "calibration", req.ID)
		}
	}
	for _, existing := range ws.Calibrations {
		if existing.ID == req.ID {
			add("calibration_already_exists", fmt.Sprintf("calibration %s already exists", req.ID), "calibration", req.ID)
		}
	}
	if diagnostic := validateApprovingActor(ws, req.ApprovedBy); diagnostic != nil {
		*errors = append(*errors, *diagnostic)
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
	if req.AllBaselines {
		for _, baseline := range ws.Baselines {
			if !domain.BaselineCarriesEstimates(baseline) {
				continue
			}
			if status := baselineSealStatus(baseline); status != "verified" {
				add("baseline_modified", fmt.Sprintf("baseline %s seal does not verify (status %s)", baseline.ID, status), "baseline", baseline.ID)
				continue
			}
			baselines = append(baselines, baseline)
		}
		return baselines
	}
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

func validateCalibrationPriorRequest(errors *[]Diagnostic, req CalibrationCalibrateRequest) {
	if req.Prior == nil {
		return
	}
	add := func(code, message string) {
		*errors = append(*errors, Diagnostic{Code: code, Message: message, Severity: "error", Target: Target{Kind: "calibration", ID: req.ID}})
	}
	if math.IsNaN(*req.Prior) || math.IsInf(*req.Prior, 0) || *req.Prior <= 0 || *req.Prior > 10 {
		add("calibration_prior_factor_invalid", "--prior must be greater than 0 and at most 10")
	}
	if strings.TrimSpace(req.PriorSource) == "" {
		add("calibration_prior_source_required", "--source is required with --prior")
	}
	if len(req.BaselineIDs) > 0 || req.AllBaselines || req.BySize || len(req.Exclusions) > 0 || req.Since != "" || req.IncludeReviewReady {
		add("calibration_prior_flags_conflict", "--prior cannot be combined with --baseline, --all-baselines, --by-size, --exclude, --since or --include-review-ready")
	}
}

type calibrationObservation struct {
	taskID                  string
	baseline, actual, ratio float64
	reviewReady             bool
}

func calibrationTaskFinished(status string, includeReviewReady bool) bool {
	return estimateTaskFinished(status) || includeReviewReady && strings.EqualFold(status, "review_ready")
}

func buildCalibration(ws domain.Workspace, baselines []domain.Baseline, req CalibrationCalibrateRequest, approvedAt string) domain.Calibration {
	resources := uniqueSorted(req.ResourceIDs)
	requireMeasurement := calibrationRequiredMeasurement(ws.Project, resources)
	calibration := domain.Calibration{
		ID: req.ID, Name: req.Name, ApprovedBy: domain.Ref{Kind: "actor", ID: req.ApprovedBy}, ApprovedAt: approvedAt,
		SourceBaselines: []domain.CalibrationSourceBaseline{},
		Scope:           domain.CalibrationScope{Resources: resources, Unit: req.Unit, Tag: req.Tag, Since: req.Since, IncludeReviewReady: req.IncludeReviewReady, RequireMeasurement: requireMeasurement},
		Sample:          domain.CalibrationSample{Tasks: []string{}, Excluded: []domain.CalibrationExclusion{}},
		Result:          domain.CalibrationResult{SizeBands: []domain.CalibrationSizeBand{}},
	}
	if req.Prior != nil {
		calibration.Result.Method = "prior"
		calibration.Result.PriorSource = strings.TrimSpace(req.PriorSource)
		calibration.Result.Factor = *req.Prior
		return calibration
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
	calibration.Result = summarizeCalibration(observations, req.BySize)
	return calibration
}

func calibrationObservations(ws domain.Workspace, baselines []domain.Baseline, scope domain.CalibrationScope, explicit map[string]string) ([]calibrationObservation, []domain.CalibrationExclusion) {
	scope.RequireMeasurement = calibrationRequiredMeasurement(ws.Project, scope.Resources)
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
		} else if !exists || !calibrationTaskFinished(task.TrackingStatus, scope.IncludeReviewReady) {
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
		if reason == "" && scope.Since != "" && window.Start < scope.Since {
			reason = "started_before_window"
		}
		sort.Slice(candidates, func(i, j int) bool { return candidates[i].baseline.ApprovedAt > candidates[j].baseline.ApprovedAt })
		var selected *candidate
		if reason == "" {
			for i := range candidates {
				if baselineApprovedBeforeStart(ws.Project, candidates[i].baseline, window) {
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
		actual, recorded, actualReason := calibrationTaskActual(ws, taskID, scope, demands)
		reason = calibrationActualExclusion(reason, actualReason, recorded)
		if reason != "" {
			excluded = append(excluded, domain.CalibrationExclusion{Task: taskID, Reason: reason})
			continue
		}
		observations = append(observations, calibrationObservation{taskID: taskID, baseline: selected.amount, actual: actual, ratio: actual / selected.amount, reviewReady: strings.EqualFold(task.TrackingStatus, "review_ready")})
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].taskID < observations[j].taskID })
	sort.Slice(excluded, func(i, j int) bool { return excluded[i].Task < excluded[j].Task })
	return observations, excluded
}

func calibrationActualExclusion(existing, actualReason string, recorded bool) string {
	if existing != "" {
		return existing
	}
	if actualReason != "" {
		return actualReason
	}
	if !recorded {
		return "no_actuals"
	}
	return ""
}

func baselineApprovedBeforeStart(project domain.Project, baseline domain.Baseline, window taskActualWindow) bool {
	approvedAt := firstNonEmpty(baseline.ApprovedAt, baseline.CreatedAt)
	if approved, approvedErr := time.Parse(time.RFC3339, approvedAt); approvedErr == nil && window.StartOccurredAt != "" {
		started, startedErr := time.Parse(time.RFC3339, window.StartOccurredAt)
		return startedErr == nil && approved.Before(started)
	}
	approvedDate := projectDate(project, approvedAt)
	return approvedDate != "" && approvedDate < window.Start
}

func calibrationRequiredMeasurement(project domain.Project, resources []string) []string {
	scopeResources := stringSet(resources)
	required := []string{}
	if project.Calibration == nil {
		return required
	}
	for _, resourceID := range project.Calibration.RequireMeasurement {
		if scopeResources[resourceID] {
			required = append(required, resourceID)
		}
	}
	return uniqueSorted(required)
}

func calibrationTaskActual(ws domain.Workspace, taskID string, scope domain.CalibrationScope, demands map[string]domain.ResourceDemand) (float64, bool, string) {
	resources := stringSet(scope.Resources)
	requireMeasurement := stringSet(scope.RequireMeasurement)
	amount, recorded := 0.0, false
	for _, actual := range EffectiveActuals(ws.Actuals) {
		if !estimateConsumptionTypes[actual.Type] {
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
		if actual.Unit != scope.Unit {
			continue
		}
		resourceID := actual.ResourceRef.ID
		if resourceID == "" {
			resourceID = demandResourceID(demand)
		}
		if resources[resourceID] {
			// A judged amount spoils only the scope it is counted in: a
			// judged CI amount does not exclude the task's agent hours.
			if actual.Measurement == "judged" {
				return 0, false, "actuals_judged"
			}
			if requireMeasurement[resourceID] && actual.Measurement != "measured" && actual.Measurement != "stated" {
				return 0, false, "actuals_unmeasured"
			}
			amount += actual.Amount
			recorded = true
		}
	}
	return amount, recorded, ""
}

func summarizeCalibration(observations []calibrationObservation, bySize bool) domain.CalibrationResult {
	result := domain.CalibrationResult{Count: len(observations), SizeBands: []domain.CalibrationSizeBand{}}
	if bySize {
		result.Method = "size_bands"
	}
	if len(observations) == 0 {
		return result
	}
	ratios := make([]float64, 0, len(observations))
	totalBaseline, totalActual := 0.0, 0.0
	for _, observation := range observations {
		if observation.reviewReady {
			result.ReviewReadyCount++
		}
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
		if len(values) == 0 && !bySize {
			continue
		}
		sort.Float64s(values)
		band := domain.CalibrationSizeBand{UpTo: bound, Count: len(values)}
		if len(values) > 0 {
			band.Median = roundEstimate(percentile(values, .5))
		}
		if bySize {
			factor, source := result.Factor, "overall"
			if len(values) >= CalibrationMinimumSample {
				factor, source = band.Median, "band"
			}
			band.Factor, band.FactorSource = floatPointer(factor), source
		}
		result.SizeBands = append(result.SizeBands, band)
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

// CalibrationFactor returns the factor applicable to a task's total amount in
// the calibration scope. Old records and incomplete band records use the
// overall factor.
func CalibrationFactor(calibration domain.Calibration, taskScopeAmount float64) (float64, string) {
	if calibration.Result.Method == "prior" {
		return calibration.Result.Factor, ""
	}
	if calibration.Result.Method != "size_bands" {
		return calibration.Result.Factor, ""
	}
	index := calibrationBandIndex(taskScopeAmount)
	bandName := calibrationBandName(index)
	for _, band := range calibration.Result.SizeBands {
		if calibrationBandIndexForBound(band.UpTo) == index && band.Factor != nil {
			return *band.Factor, bandName
		}
	}
	return calibration.Result.Factor, bandName
}

func calibrationBandIndexForBound(upTo *float64) int {
	if upTo == nil {
		return 2
	}
	if *upTo <= 2 {
		return 0
	}
	return 1
}

func calibrationBandName(index int) string {
	if index == 0 {
		return "up to 2"
	}
	if index == 1 {
		return "up to 4"
	}
	return "above 4"
}

func calibrationTaskScopeAmounts(demands []domain.ResourceDemand, calibration domain.Calibration) map[string]float64 {
	amounts := map[string]float64{}
	resources := stringSet(calibration.Scope.Resources)
	for _, demand := range demands {
		if demand.Unit == calibration.Scope.Unit && resources[demandResourceID(demand)] {
			amounts[demand.TaskRef.ID] += demand.PlannedAmount
		}
	}
	return amounts
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
	data, err := yamlformat.Render(struct {
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
	auditPath, err := writer.AppendAudit(&audit)
	result.AuditRecordPreview = audit
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
	for _, actual := range EffectiveActuals(ws.Actuals) {
		if estimateConsumptionTypes[actual.Type] && actual.Unit == calibration.Scope.Unit {
			actualByDemand[actual.DemandRef.ID] += actual.Amount
		}
	}
	resources := stringSet(calibration.Scope.Resources)
	taskAmounts := calibrationTaskScopeAmounts(ws.ResourceDemands, calibration)
	for _, demand := range ws.ResourceDemands {
		if !resources[demandResourceID(demand)] || demand.Unit != calibration.Scope.Unit || (calibration.Scope.Tag != "" && !containsString(tasks[demand.TaskRef.ID].Tags, calibration.Scope.Tag)) {
			continue
		}
		if calibrationDemandClosed(demand.Status) {
			*skipped = append(*skipped, CalibrationApplySkip{DemandID: demand.ID, Reason: "closed"})
			continue
		}
		factor, _ := CalibrationFactor(calibration, taskAmounts[demand.TaskRef.ID])
		scaledPlanned := RoundCalibrationAmount(demand.PlannedAmount * factor)
		if actualByDemand[demand.ID] > scaledPlanned {
			*skipped = append(*skipped, CalibrationApplySkip{DemandID: demand.ID, Reason: "actuals_above_scaled_amount"})
			continue
		}
		if scaledPlanned != demand.PlannedAmount {
			scenario.Changes = append(scenario.Changes, domain.ScenarioChange{Op: "replace", Path: "resource_demands." + demand.ID + ".planned_amount", Value: scaledPlanned})
		}
		if demand.HasRemaining() {
			scaledRemaining := RoundCalibrationAmount(demand.Remaining() * factor)
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
	return CalibrationSummary{ID: calibration.ID, Name: calibration.Name, ApprovedAt: calibration.ApprovedAt, Resources: append([]string(nil), calibration.Scope.Resources...), Unit: calibration.Scope.Unit, Tag: calibration.Scope.Tag, Since: calibration.Scope.Since, Method: calibration.Result.Method, PriorSource: calibration.Result.PriorSource, Factor: calibration.Result.Factor, TotalRatio: calibration.Result.TotalRatio, P25: calibration.Result.P25, P75: calibration.Result.P75, Count: calibration.Result.Count, ReviewReadyCount: calibration.Result.ReviewReadyCount, SizeBands: append([]domain.CalibrationSizeBand(nil), calibration.Result.SizeBands...), SealStatus: calibrationSealStatus(calibration)}
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
	values := []CalibratedDemandValue{}
	selected := ApplicableDemandCalibrations(ws)
	for _, demand := range ws.ResourceDemands {
		if match, ok := selected[demand.ID]; ok {
			values = append(values, CalibratedDemandValue{DemandID: demand.ID, TypedAmount: demand.PlannedAmount, Calibrated: calibratedLoadAmount(demand.PlannedAmount, match.Factor), Unit: demand.Unit, CalibrationID: match.Calibration.ID, Factor: match.Factor, Band: match.Band})
		}
	}
	sort.Slice(values, func(i, j int) bool { return values[i].DemandID < values[j].DemandID })
	return values
}

// ApplicableDemandCalibrations applies the single selection rule used by
// estimate hints and calculated load. Closed demands remain history, and a
// calibration already applied to the plan through a scenario is not applied
// a second time.
func ApplicableDemandCalibrations(ws domain.Workspace) map[string]DemandCalibration {
	tasks := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		tasks[task.ID] = task
	}
	applied := map[string]bool{}
	for _, scenario := range ws.Scenarios {
		if scenario.Status == "applied" && scenario.CalibrationRef.ID != "" {
			applied[scenario.CalibrationRef.ID] = true
		}
	}
	amountsByCalibration := map[string]map[string]float64{}
	result := map[string]DemandCalibration{}
	for _, demand := range ws.ResourceDemands {
		if calibrationDemandClosed(demand.Status) {
			continue
		}
		var selected *domain.Calibration
		for i := range ws.Calibrations {
			calibration := &ws.Calibrations[i]
			if applied[calibration.ID] || calibrationSealStatus(*calibration) != "verified" || calibration.Scope.Unit != demand.Unit || !containsString(calibration.Scope.Resources, demandResourceID(demand)) {
				continue
			}
			if calibration.Scope.Tag != "" && !containsString(tasks[demand.TaskRef.ID].Tags, calibration.Scope.Tag) {
				continue
			}
			if selected == nil || calibrationApprovedAfter(*calibration, *selected) {
				selected = calibration
			}
		}
		if selected == nil {
			continue
		}
		amounts := amountsByCalibration[selected.ID]
		if amounts == nil {
			amounts = calibrationTaskScopeAmounts(ws.ResourceDemands, *selected)
			amountsByCalibration[selected.ID] = amounts
		}
		taskAmount := amounts[demand.TaskRef.ID]
		factor, band := CalibrationFactor(*selected, taskAmount)
		result[demand.ID] = DemandCalibration{Calibration: *selected, TaskScopeAmount: taskAmount, Factor: factor, Band: band}
	}
	return result
}

func calibrationApprovedAfter(left, right domain.Calibration) bool {
	leftTime, leftErr := time.Parse(time.RFC3339, left.ApprovedAt)
	rightTime, rightErr := time.Parse(time.RFC3339, right.ApprovedAt)
	if leftErr == nil && rightErr == nil && !leftTime.Equal(rightTime) {
		return leftTime.After(rightTime)
	}
	if left.ApprovedAt != right.ApprovedAt {
		return left.ApprovedAt > right.ApprovedAt
	}
	return left.ID > right.ID
}

// calibrationSelectionWorkspace treats a calibration scenario selected as an
// in-memory overlay like an applied scenario for this calculation. Its changes
// already contain the scaled amounts and must not be scaled again.
func calibrationSelectionWorkspace(ws domain.Workspace, scenarioID string) domain.Workspace {
	if scenarioID == "" {
		return ws
	}
	ws.Scenarios = append([]domain.Scenario(nil), ws.Scenarios...)
	for i := range ws.Scenarios {
		if ws.Scenarios[i].ID == scenarioID && ws.Scenarios[i].CalibrationRef.ID != "" {
			ws.Scenarios[i].Status = "applied"
		}
	}
	return ws
}

func calibratedLoadAmount(value, factor float64) float64 {
	if value == 0 {
		return 0
	}
	return RoundCalibrationAmount(value * factor)
}

// CalibrationOpenDemandGroups returns one application candidate per calibration
// selected by the load path. A calibration disappears after any scenario records
// its ref.
func CalibrationOpenDemandGroups(ws domain.Workspace) []CalibrationOpenDemands {
	used := map[string]bool{}
	for _, scenario := range ws.Scenarios {
		if scenario.CalibrationRef.ID != "" {
			used[scenario.CalibrationRef.ID] = true
		}
	}
	selected := ApplicableDemandCalibrations(ws)
	groupsByCalibration := map[string]*CalibrationOpenDemands{}
	for _, demand := range ws.ResourceDemands {
		match, ok := selected[demand.ID]
		if !ok || used[match.Calibration.ID] {
			continue
		}
		group := groupsByCalibration[match.Calibration.ID]
		if group == nil {
			group = &CalibrationOpenDemands{
				CalibrationID:           match.Calibration.ID,
				SupersedesCalibrationID: supersededCalibrationID(ws.Calibrations, match.Calibration),
				Demands:                 []CalibratedDemandValue{},
			}
			groupsByCalibration[match.Calibration.ID] = group
		}
		group.Demands = append(group.Demands, CalibratedDemandValue{DemandID: demand.ID, TypedAmount: demand.PlannedAmount, Calibrated: calibratedLoadAmount(demand.PlannedAmount, match.Factor), Unit: demand.Unit, CalibrationID: match.Calibration.ID, Factor: match.Factor, Band: match.Band})
	}
	groups := make([]CalibrationOpenDemands, 0, len(groupsByCalibration))
	for _, group := range groupsByCalibration {
		sort.Slice(group.Demands, func(i, j int) bool { return group.Demands[i].DemandID < group.Demands[j].DemandID })
		groups = append(groups, *group)
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].CalibrationID < groups[j].CalibrationID })
	return groups
}

func supersededCalibrationID(calibrations []domain.Calibration, selected domain.Calibration) string {
	var superseded domain.Calibration
	found := false
	selectedResources := strings.Join(uniqueSorted(selected.Scope.Resources), "\x00")
	for _, calibration := range calibrations {
		sameScope := calibration.Scope.Unit == selected.Scope.Unit && calibration.Scope.Tag == selected.Scope.Tag && strings.Join(uniqueSorted(calibration.Scope.Resources), "\x00") == selectedResources
		if calibration.ID == selected.ID || !sameScope || calibrationSealStatus(calibration) != "verified" || !calibrationApprovedAfter(selected, calibration) {
			continue
		}
		if !found || calibrationApprovedAfter(calibration, superseded) {
			superseded, found = calibration, true
		}
	}
	if !found {
		return ""
	}
	return superseded.ID
}

// CalibrationSuggestions finds uncalibrated baseline scopes with enough
// clean finished tasks. Explicit judgement exclusions remain an operator
// choice, so next suggests the command rather than creating a record.
func CalibrationSuggestions(ws domain.Workspace) []CalibrationSuggestion {
	candidates := map[calibrationSuggestionKey]CalibrationSuggestion{}
	limited := []domain.Baseline{}
	for _, baseline := range ws.Baselines {
		if !domain.BaselineCarriesEstimates(baseline) || baselineSealStatus(baseline) != "verified" {
			continue
		}
		if baseline.Scope == "tasks" {
			limited = append(limited, baseline)
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
	addAllBaselineSuggestions(candidates, ws, limited)
	result := []CalibrationSuggestion{}
	for _, suggestion := range candidates {
		result = append(result, suggestion)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].AllBaselines != result[j].AllBaselines {
			return result[i].AllBaselines
		}
		if result[i].BaselineID != result[j].BaselineID {
			return result[i].BaselineID < result[j].BaselineID
		}
		if result[i].Unit != result[j].Unit {
			return result[i].Unit < result[j].Unit
		}
		return strings.Join(result[i].Resources, "\x00") < strings.Join(result[j].Resources, "\x00")
	})
	unique := result[:0]
	seenScopes := map[string]bool{}
	for _, suggestion := range result {
		scope := suggestion.Unit + "\x00" + suggestion.Tag + "\x00" + strings.Join(suggestion.Resources, "\x00")
		if seenScopes[scope] {
			continue
		}
		seenScopes[scope] = true
		if latest, ok := latestMeasuredCalibrationForScope(ws.Calibrations, suggestion.Resources, suggestion.Unit, suggestion.Tag); ok {
			suggestion = windowedCalibrationSuggestion(ws, suggestion, latest)
			if suggestion.Count < CalibrationMinimumSample {
				continue
			}
		} else if latest, ok := latestCalibrationForScope(ws.Calibrations, suggestion.Resources, suggestion.Unit, suggestion.Tag); ok && latest.Result.Method == "prior" {
			suggestion.ReplacesPriorID = latest.ID
		}
		unique = append(unique, suggestion)
	}
	unique = appendPriorReplacementSuggestions(unique, ws)
	unique = appendMeasuredReplacementSuggestions(unique, ws)
	sort.Slice(unique, func(i, j int) bool {
		left := unique[i].Unit + "\x00" + unique[i].Tag + "\x00" + strings.Join(unique[i].Resources, "\x00")
		right := unique[j].Unit + "\x00" + unique[j].Tag + "\x00" + strings.Join(unique[j].Resources, "\x00")
		return left < right
	})
	return unique
}

func latestMeasuredCalibrationForScope(calibrations []domain.Calibration, resources []string, unit, tag string) (domain.Calibration, bool) {
	var latest domain.Calibration
	found := false
	for _, calibration := range calibrations {
		if calibration.Result.Method == "prior" || calibrationSealStatus(calibration) != "verified" || calibration.Scope.Unit != unit || calibration.Scope.Tag != tag || !resourceSuperset(calibration.Scope.Resources, resources) {
			continue
		}
		if !found || calibrationApprovedAfter(calibration, latest) {
			latest, found = calibration, true
		}
	}
	return latest, found
}

func windowedCalibrationSuggestion(ws domain.Workspace, suggestion CalibrationSuggestion, calibration domain.Calibration) CalibrationSuggestion {
	since := projectDate(ws.Project, calibration.ApprovedAt)
	baselines := suggestionBaselines(ws, suggestion)
	scope := domain.CalibrationScope{Resources: suggestion.Resources, Unit: suggestion.Unit, Tag: suggestion.Tag, Since: since}
	observations, _ := calibrationObservations(ws, baselines, scope, nil)
	count := countNewCalibrationObservations(observations, calibration.Sample.Tasks)
	if count < CalibrationMinimumSample {
		scope.IncludeReviewReady = true
		observations, _ = calibrationObservations(ws, baselines, scope, nil)
		count = countNewCalibrationObservations(observations, calibration.Sample.Tasks)
		suggestion.IncludeReviewReady = count >= CalibrationMinimumSample
	}
	suggestion.Count = count
	suggestion.Since = since
	suggestion.ReplacesID = calibration.ID
	suggestion.BySize = calibration.Result.Method == "size_bands"
	return suggestion
}

func countNewCalibrationObservations(observations []calibrationObservation, sampled []string) int {
	count := 0
	for _, observation := range observations {
		if !containsString(sampled, observation.taskID) {
			count++
		}
	}
	return count
}

func suggestionBaselines(ws domain.Workspace, suggestion CalibrationSuggestion) []domain.Baseline {
	var baselines []domain.Baseline
	for _, baseline := range ws.Baselines {
		if !domain.BaselineCarriesEstimates(baseline) || baselineSealStatus(baseline) != "verified" {
			continue
		}
		if suggestion.AllBaselines || baseline.ID == suggestion.BaselineID {
			baselines = append(baselines, baseline)
		}
	}
	return baselines
}

func appendMeasuredReplacementSuggestions(suggestions []CalibrationSuggestion, ws domain.Workspace) []CalibrationSuggestion {
	seen := map[string]bool{}
	for _, suggestion := range suggestions {
		seen[suggestion.Unit+"\x00"+suggestion.Tag+"\x00"+strings.Join(uniqueSorted(suggestion.Resources), "\x00")] = true
	}
	for _, calibration := range ws.Calibrations {
		if calibration.Result.Method == "prior" || calibrationSealStatus(calibration) != "verified" {
			continue
		}
		resources := uniqueSorted(calibration.Scope.Resources)
		key := calibration.Scope.Unit + "\x00" + calibration.Scope.Tag + "\x00" + strings.Join(resources, "\x00")
		if seen[key] {
			continue
		}
		latest, ok := latestMeasuredCalibrationForScope(ws.Calibrations, resources, calibration.Scope.Unit, calibration.Scope.Tag)
		if !ok || latest.ID != calibration.ID {
			continue
		}
		suggestion := CalibrationSuggestion{AllBaselines: true, Resources: resources, Unit: calibration.Scope.Unit, Tag: calibration.Scope.Tag}
		suggestion = windowedCalibrationSuggestion(ws, suggestion, calibration)
		if suggestion.Count < CalibrationMinimumSample {
			continue
		}
		suggestions = append(suggestions, suggestion)
		seen[key] = true
	}
	return suggestions
}

func latestCalibrationForScope(calibrations []domain.Calibration, resources []string, unit, tag string) (domain.Calibration, bool) {
	var latest domain.Calibration
	found := false
	for _, calibration := range calibrations {
		if calibrationSealStatus(calibration) != "verified" || calibration.Scope.Unit != unit || (calibration.Scope.Tag != "" && calibration.Scope.Tag != tag) || !resourceSuperset(calibration.Scope.Resources, resources) {
			continue
		}
		if !found || calibrationApprovedAfter(calibration, latest) {
			latest, found = calibration, true
		}
	}
	return latest, found
}

func appendPriorReplacementSuggestions(suggestions []CalibrationSuggestion, ws domain.Workspace) []CalibrationSuggestion {
	baselines := make([]domain.Baseline, 0, len(ws.Baselines))
	for _, baseline := range ws.Baselines {
		if domain.BaselineCarriesEstimates(baseline) && baselineSealStatus(baseline) == "verified" {
			baselines = append(baselines, baseline)
		}
	}
	for _, prior := range ws.Calibrations {
		if prior.Result.Method != "prior" || calibrationSealStatus(prior) != "verified" {
			continue
		}
		latest, ok := latestCalibrationForScope(ws.Calibrations, prior.Scope.Resources, prior.Scope.Unit, prior.Scope.Tag)
		if !ok || latest.ID != prior.ID {
			continue
		}
		resources := uniqueSorted(prior.Scope.Resources)
		observations, _ := calibrationObservations(ws, baselines, prior.Scope, nil)
		includeReviewReady := false
		if len(observations) < CalibrationMinimumSample {
			scope := prior.Scope
			scope.IncludeReviewReady = true
			observations, _ = calibrationObservations(ws, baselines, scope, nil)
			includeReviewReady = len(observations) >= CalibrationMinimumSample
		}
		if len(observations) < CalibrationMinimumSample {
			continue
		}
		filtered := suggestions[:0]
		for _, suggestion := range suggestions {
			sameResources := strings.Join(uniqueSorted(suggestion.Resources), "\x00") == strings.Join(resources, "\x00")
			sameScope := suggestion.Unit == prior.Scope.Unit && suggestion.Tag == prior.Scope.Tag && sameResources
			untaggedDuplicate := prior.Scope.Tag != "" && suggestion.Unit == prior.Scope.Unit && suggestion.Tag == "" && sameResources
			if sameScope || untaggedDuplicate {
				continue
			}
			filtered = append(filtered, suggestion)
		}
		suggestions = filtered
		suggestions = append(suggestions, CalibrationSuggestion{AllBaselines: true, Resources: resources, Unit: prior.Scope.Unit, Tag: prior.Scope.Tag, Count: len(observations), ReplacesPriorID: prior.ID, IncludeReviewReady: includeReviewReady})
	}
	return suggestions
}

func addAllBaselineSuggestions(candidates map[calibrationSuggestionKey]CalibrationSuggestion, ws domain.Workspace, baselines []domain.Baseline) {
	if len(baselines) == 0 {
		return
	}
	scopes := map[calibrationSuggestionKey][]string{}
	for _, baseline := range baselines {
		for _, demand := range baseline.IncludedDemands {
			if estimateBaselineAmount(demand) > 0 {
				key := calibrationSuggestionKey{baseline: "*", unit: demand.Unit, resources: demand.Resource}
				scopes[key] = []string{demand.Resource}
			}
		}
		report := EstimateReportResult{Resources: []EstimateResourceSummary{}, SplitGroups: []EstimateSplitGroupSummary{}, Tasks: []EstimateTask{}}
		buildEstimateReport(&report, ws, baseline)
		for _, group := range report.SplitGroups {
			resources := uniqueSorted(append([]string{group.OriginalResourceID}, group.SplitResourceIDs...))
			key := calibrationSuggestionKey{baseline: "*", unit: group.Unit, resources: strings.Join(resources, "\x00")}
			scopes[key] = resources
		}
	}
	for key, resources := range scopes {
		if calibrationScopeHasStrictSuperset(key, resources, scopes) {
			continue
		}
		resources = uniqueSorted(resources)
		scope := domain.CalibrationScope{Resources: resources, Unit: key.unit}
		observations, _ := calibrationObservations(ws, baselines, scope, nil)
		count := 0
		for _, observation := range observations {
			if !calibrationTaskSampleUsed(ws.Calibrations, observation.taskID, resources, key.unit) {
				count++
			}
		}
		includeReviewReady := false
		if count < CalibrationMinimumSample {
			scope.IncludeReviewReady = true
			observations, _ = calibrationObservations(ws, baselines, scope, nil)
			count = 0
			for _, observation := range observations {
				if !calibrationTaskSampleUsed(ws.Calibrations, observation.taskID, resources, key.unit) {
					count++
				}
			}
			includeReviewReady = count >= CalibrationMinimumSample
		}
		if count < CalibrationMinimumSample {
			continue
		}
		resourceKey := strings.Join(resources, "\x00")
		for candidateKey, suggestion := range candidates {
			if suggestion.Unit == key.unit && strings.Join(suggestion.Resources, "\x00") == resourceKey {
				delete(candidates, candidateKey)
			}
		}
		candidates[calibrationSuggestionKey{baseline: "*", unit: key.unit, resources: resourceKey}] = CalibrationSuggestion{AllBaselines: true, Resources: resources, Unit: key.unit, Count: count, IncludeReviewReady: includeReviewReady}
	}
}

func addCalibrationSuggestion(candidates map[calibrationSuggestionKey]CalibrationSuggestion, ws domain.Workspace, baseline domain.Baseline, resources []string, unit string) {
	resources = uniqueSorted(resources)
	scope := domain.CalibrationScope{Resources: resources, Unit: unit}
	observations, _ := calibrationObservations(ws, []domain.Baseline{baseline}, scope, nil)
	count := 0
	for _, observation := range observations {
		if !calibrationTaskSampleUsed(ws.Calibrations, observation.taskID, resources, unit) {
			count++
		}
	}
	includeReviewReady := false
	if count < CalibrationMinimumSample {
		scope.IncludeReviewReady = true
		observations, _ = calibrationObservations(ws, []domain.Baseline{baseline}, scope, nil)
		count = 0
		for _, observation := range observations {
			if !calibrationTaskSampleUsed(ws.Calibrations, observation.taskID, resources, unit) {
				count++
			}
		}
		includeReviewReady = count >= CalibrationMinimumSample
	}
	if count < CalibrationMinimumSample {
		return
	}
	key := calibrationSuggestionKey{baseline: baseline.ID, unit: unit, resources: strings.Join(resources, "\x00")}
	candidates[key] = CalibrationSuggestion{BaselineID: baseline.ID, Resources: resources, Unit: unit, Count: count, IncludeReviewReady: includeReviewReady}
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
