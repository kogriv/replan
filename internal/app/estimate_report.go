package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

var estimateConsumptionTypes = map[string]bool{
	"consumption":      true,
	"runtime_consumed": true,
	"work_consumed":    true,
}

type EstimateReportRequest struct {
	BaselineID string
}

type EstimateReportResult struct {
	ProjectID            string                      `json:"project_id"`
	ProjectName          string                      `json:"project_name"`
	BaselineID           string                      `json:"baseline_id"`
	BaselineSealStatus   string                      `json:"baseline_seal_status"`
	BaselineHasEstimates bool                        `json:"baseline_has_estimates"`
	ActualTypes          []string                    `json:"actual_types"`
	SplitRule            string                      `json:"split_rule"`
	Resources            []EstimateResourceSummary   `json:"resources"`
	SplitGroups          []EstimateSplitGroupSummary `json:"split_groups"`
	Calibrations         []CalibrationSummary        `json:"calibrations"`
	Tasks                []EstimateTask              `json:"tasks"`
	Warnings             []Diagnostic                `json:"warnings"`
	Errors               []Diagnostic                `json:"errors"`
}

type EstimateTask struct {
	TaskID           string                    `json:"task_id"`
	Title            string                    `json:"title,omitempty"`
	TrackingStatus   string                    `json:"tracking_status,omitempty"`
	Open             bool                      `json:"open"`
	ActualsPartial   bool                      `json:"actuals_partial"`
	Schedule         EstimateSchedule          `json:"schedule"`
	Demands          []EstimateDemand          `json:"demands"`
	Totals           []EstimateUnitTotals      `json:"totals"`
	SplitComparisons []EstimateSplitComparison `json:"split_comparisons"`
}

type EstimateSchedule struct {
	BaselinePlannedStart       string `json:"baseline_planned_start,omitempty"`
	BaselinePlannedFinish      string `json:"baseline_planned_finish,omitempty"`
	ActualStart                string `json:"actual_start,omitempty"`
	ActualStartSource          string `json:"actual_start_source"`
	ActualFinish               string `json:"actual_finish,omitempty"`
	ActualFinishSource         string `json:"actual_finish_source"`
	FinishVarianceCalendarDays *int   `json:"finish_variance_calendar_days,omitempty"`
	FinishVarianceWorkingDays  *int   `json:"finish_variance_working_days,omitempty"`
}

type EstimateDemand struct {
	DemandID               string   `json:"demand_id"`
	State                  string   `json:"state"`
	ResourceID             string   `json:"resource_id,omitempty"`
	BaselineResourceID     string   `json:"baseline_resource_id,omitempty"`
	Unit                   string   `json:"unit"`
	BaselineAmount         float64  `json:"baseline_amount"`
	CurrentPlannedAmount   float64  `json:"current_planned_amount"`
	CurrentRemainingAmount float64  `json:"current_remaining_amount"`
	ActualAmount           float64  `json:"actual_amount"`
	ActualsRecorded        bool     `json:"-"`
	VarianceAmount         float64  `json:"variance_amount"`
	ActualBaselineRatio    *float64 `json:"actual_baseline_ratio"`
	Reestimated            bool     `json:"reestimated"`
}

type EstimateUnitTotals struct {
	Unit                   string   `json:"unit"`
	BaselineAmount         float64  `json:"baseline_amount"`
	CurrentPlannedAmount   float64  `json:"current_planned_amount"`
	CurrentRemainingAmount float64  `json:"current_remaining_amount"`
	ActualAmount           float64  `json:"actual_amount"`
	VarianceAmount         float64  `json:"variance_amount"`
	ActualBaselineRatio    *float64 `json:"actual_baseline_ratio"`
}

type EstimateSplitComparison struct {
	AddedDemandID        string  `json:"added_demand_id"`
	AddedResourceID      string  `json:"added_resource_id"`
	Unit                 string  `json:"unit"`
	CurrentPlannedAmount float64 `json:"current_planned_amount"`
	BaselineBasis        string  `json:"baseline_basis"`
	BaselineDemandID     string  `json:"baseline_demand_id,omitempty"`
	BaselineResourceID   string  `json:"baseline_resource_id,omitempty"`
	BaselineAmount       float64 `json:"baseline_amount"`
}

type EstimateResourceAmounts struct {
	BaselineAmount       float64  `json:"baseline_amount"`
	CurrentPlannedAmount float64  `json:"current_planned_amount"`
	ActualAmount         float64  `json:"actual_amount"`
	ActualBaselineRatio  *float64 `json:"actual_baseline_ratio"`
}

type EstimateResourceSummary struct {
	ResourceID      string                  `json:"resource_id"`
	ResourceName    string                  `json:"resource_name,omitempty"`
	Unit            string                  `json:"unit"`
	ActualsRecorded bool                    `json:"actuals_recorded"`
	Finished        EstimateResourceAmounts `json:"finished_tasks"`
	All             EstimateResourceAmounts `json:"all_tasks"`
}

type EstimateSplitGroupSummary struct {
	OriginalResourceID   string                  `json:"original_resource_id"`
	OriginalResourceName string                  `json:"original_resource_name,omitempty"`
	SplitResourceIDs     []string                `json:"split_resource_ids"`
	Unit                 string                  `json:"unit"`
	ActualsRecorded      bool                    `json:"actuals_recorded"`
	Finished             EstimateResourceAmounts `json:"finished_tasks"`
	All                  EstimateResourceAmounts `json:"all_tasks"`
}

func (s ReportService) Estimates(ctx context.Context, req EstimateReportRequest) (EstimateReportResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := EstimateReportResult{
		ProjectID: ws.Project.ID, ProjectName: ws.Project.Name, BaselineID: req.BaselineID,
		ActualTypes: []string{"consumption", "runtime_consumed", "work_consumed"},
		SplitRule:   "For a resource absent from the baseline, an added demand uses the same-task, same-unit baseline demand with the largest positive planned reduction; ties use demand id. Otherwise it uses the task baseline total for the unit.",
		Resources:   []EstimateResourceSummary{}, SplitGroups: []EstimateSplitGroupSummary{}, Calibrations: []CalibrationSummary{}, Tasks: []EstimateTask{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	baseline, ok := estimateBaseline(ws.Baselines, req.BaselineID)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "baseline_not_found", Message: fmt.Sprintf("baseline %s not found", req.BaselineID), Severity: "error", Target: Target{Kind: "baseline", ID: req.BaselineID}})
		return result, ErrValidationFailed
	}
	result.BaselineHasEstimates = domain.BaselineCarriesEstimates(baseline)
	result.BaselineSealStatus = baselineSealStatus(baseline)
	if !result.BaselineHasEstimates {
		result.Errors = append(result.Errors, Diagnostic{Code: "baseline_has_no_estimates", Message: fmt.Sprintf("baseline %s has no estimate snapshot; create one with replan baseline create", baseline.ID), Severity: "error", Target: Target{Kind: "baseline", ID: baseline.ID}})
		return result, ErrValidationFailed
	}
	if result.BaselineSealStatus != "verified" {
		result.Errors = append(result.Errors, Diagnostic{Code: "baseline_modified", Message: fmt.Sprintf("baseline %s seal does not verify (status %s)", baseline.ID, result.BaselineSealStatus), Severity: "error", Target: Target{Kind: "baseline", ID: baseline.ID}})
		return result, ErrValidationFailed
	}

	buildEstimateReport(&result, ws, baseline)
	for _, calibration := range ws.Calibrations {
		for _, source := range calibration.SourceBaselines {
			if source.ID == baseline.ID {
				result.Calibrations = append(result.Calibrations, calibrationSummary(calibration))
				break
			}
		}
	}
	sort.Slice(result.Calibrations, func(i, j int) bool { return result.Calibrations[i].ID < result.Calibrations[j].ID })
	return result, nil
}

func estimateBaseline(baselines []domain.Baseline, id string) (domain.Baseline, bool) {
	for _, baseline := range baselines {
		if baseline.ID == id {
			return baseline, true
		}
	}
	return domain.Baseline{}, false
}

type estimateIndex struct {
	tasks             map[string]domain.Task
	demands           map[string]domain.ResourceDemand
	demandsByTask     map[string][]domain.ResourceDemand
	actualsByDemand   map[string][]domain.Actual
	actualsByTask     map[string][]domain.Actual
	baselineResources map[string]bool
	resourceNames     map[string]string
	projectDays       map[time.Weekday]bool
}

func buildEstimateReport(result *EstimateReportResult, ws domain.Workspace, baseline domain.Baseline) {
	index := newEstimateIndex(ws, baseline)
	baselineByTask := map[string][]domain.BaselineDemandEntry{}
	for _, demand := range baseline.IncludedDemands {
		baselineByTask[demand.Task] = append(baselineByTask[demand.Task], demand)
	}
	resourceRows := map[string]*EstimateResourceSummary{}
	for _, baselineTask := range baseline.IncludedTasks {
		row := estimateTaskRow(ws, baselineTask, baselineByTask[baselineTask.ID], index)
		result.Tasks = append(result.Tasks, row)
		addEstimateResourceRows(resourceRows, row, index)
	}
	for _, row := range resourceRows {
		if row.ActualsRecorded {
			row.Finished.ActualBaselineRatio = amountRatio(row.Finished.ActualAmount, row.Finished.BaselineAmount)
			row.All.ActualBaselineRatio = amountRatio(row.All.ActualAmount, row.All.BaselineAmount)
		}
		result.Resources = append(result.Resources, *row)
	}
	sort.Slice(result.Resources, func(i, j int) bool {
		if result.Resources[i].ResourceID != result.Resources[j].ResourceID {
			return result.Resources[i].ResourceID < result.Resources[j].ResourceID
		}
		return result.Resources[i].Unit < result.Resources[j].Unit
	})
	result.SplitGroups = estimateSplitGroups(result.Tasks, result.Resources, index)
	roundEstimateResult(result)
}

func newEstimateIndex(ws domain.Workspace, baseline domain.Baseline) estimateIndex {
	index := estimateIndex{
		tasks: map[string]domain.Task{}, demands: map[string]domain.ResourceDemand{}, demandsByTask: map[string][]domain.ResourceDemand{},
		actualsByDemand: map[string][]domain.Actual{}, actualsByTask: map[string][]domain.Actual{}, baselineResources: map[string]bool{},
		resourceNames: map[string]string{}, projectDays: map[time.Weekday]bool{},
	}
	for _, task := range ws.Tasks {
		index.tasks[task.ID] = task
	}
	for _, demand := range ws.ResourceDemands {
		index.demands[demand.ID] = demand
		index.demandsByTask[demand.TaskRef.ID] = append(index.demandsByTask[demand.TaskRef.ID], demand)
	}
	for _, actual := range ws.Actuals {
		if !estimateConsumptionTypes[actual.Type] {
			continue
		}
		if actual.DemandRef.ID != "" {
			index.actualsByDemand[actual.DemandRef.ID] = append(index.actualsByDemand[actual.DemandRef.ID], actual)
		}
		if actual.TaskRef.ID != "" {
			index.actualsByTask[actual.TaskRef.ID] = append(index.actualsByTask[actual.TaskRef.ID], actual)
		}
	}
	for _, demand := range baseline.IncludedDemands {
		index.baselineResources[demand.Resource] = true
	}
	for _, resource := range ws.Resources {
		index.resourceNames[resource.ID] = resource.Name
	}
	index.projectDays = projectWorkingDays(ws.Availabilities)
	return index
}

func estimateTaskRow(ws domain.Workspace, baselineTask domain.BaselineTaskEntry, baselineDemands []domain.BaselineDemandEntry, index estimateIndex) EstimateTask {
	task, exists := index.tasks[baselineTask.ID]
	row := EstimateTask{
		TaskID: baselineTask.ID, Title: task.Title, TrackingStatus: task.TrackingStatus,
		Open: !exists || !estimateTaskFinished(task.TrackingStatus), Demands: []EstimateDemand{}, Totals: []EstimateUnitTotals{},
		SplitComparisons: []EstimateSplitComparison{},
	}
	row.ActualsPartial = row.Open
	row.Schedule = estimateScheduleRow(ws, baselineTask, baselineDemands, index)
	baselineIDs := map[string]bool{}
	baselineTotals := map[string]float64{}
	for _, baselineDemand := range baselineDemands {
		baselineIDs[baselineDemand.ID] = true
		amount := estimateBaselineAmount(baselineDemand)
		baselineTotals[baselineDemand.Unit] += amount
		current, currentOK := index.demands[baselineDemand.ID]
		actual, actualsRecorded := estimateDemandActual(index.actualsByDemand[baselineDemand.ID], baselineDemand.Unit)
		demand := EstimateDemand{
			DemandID: baselineDemand.ID, State: "current", ResourceID: current.ResourceRef.ID,
			BaselineResourceID: baselineDemand.Resource, Unit: baselineDemand.Unit, BaselineAmount: amount,
			CurrentPlannedAmount: current.PlannedAmount, CurrentRemainingAmount: current.Remaining(), ActualAmount: actual, ActualsRecorded: actualsRecorded,
		}
		if !currentOK {
			demand.State = "removed"
			demand.ResourceID = baselineDemand.Resource
		}
		demand.VarianceAmount = demand.ActualAmount - demand.BaselineAmount
		demand.ActualBaselineRatio = amountRatio(demand.ActualAmount, demand.BaselineAmount)
		demand.Reestimated = currentOK && current.PlannedAmount != amount
		row.Demands = append(row.Demands, demand)
	}
	for _, current := range index.demandsByTask[baselineTask.ID] {
		if baselineIDs[current.ID] {
			continue
		}
		actual, actualsRecorded := estimateDemandActual(index.actualsByDemand[current.ID], current.Unit)
		row.Demands = append(row.Demands, EstimateDemand{
			DemandID: current.ID, State: "added", ResourceID: demandResourceID(current), Unit: current.Unit,
			CurrentPlannedAmount: current.PlannedAmount, CurrentRemainingAmount: current.Remaining(), ActualAmount: actual, ActualsRecorded: actualsRecorded,
			VarianceAmount: actual,
		})
		if !index.baselineResources[demandResourceID(current)] {
			row.SplitComparisons = append(row.SplitComparisons, estimateSplitComparison(current, baselineDemands, index, baselineTotals[current.Unit]))
		}
	}
	sort.Slice(row.Demands, func(i, j int) bool { return row.Demands[i].DemandID < row.Demands[j].DemandID })
	sort.Slice(row.SplitComparisons, func(i, j int) bool {
		return row.SplitComparisons[i].AddedDemandID < row.SplitComparisons[j].AddedDemandID
	})
	row.Totals = estimateTaskTotals(row.Demands)
	return row
}

func estimateScheduleRow(ws domain.Workspace, task domain.BaselineTaskEntry, baselineDemands []domain.BaselineDemandEntry, index estimateIndex) EstimateSchedule {
	row := EstimateSchedule{BaselinePlannedStart: task.PlannedStart, BaselinePlannedFinish: task.PlannedFinish, ActualStartSource: "none", ActualFinishSource: "none"}
	demandIDs := make([]string, 0, len(index.demandsByTask[task.ID])+len(baselineDemands))
	for _, demand := range index.demandsByTask[task.ID] {
		demandIDs = append(demandIDs, demand.ID)
	}
	for _, demand := range baselineDemands {
		demandIDs = append(demandIDs, demand.ID)
	}
	actual := deriveTaskActualWindow(ws, task.ID, demandIDs, false)
	row.ActualStart, row.ActualStartSource = actual.Start, actual.StartSource
	row.ActualFinish, row.ActualFinishSource = actual.Finish, actual.FinishSource
	if finish, ok := parseWorkloadDate(row.ActualFinish); ok {
		if planned, plannedOK := parseWorkloadDate(task.PlannedFinish); plannedOK {
			calendar := int(finish.Sub(planned).Hours() / 24)
			working := signedWorkingDays(planned, finish, index.projectDays)
			row.FinishVarianceCalendarDays, row.FinishVarianceWorkingDays = &calendar, &working
		}
	}
	return row
}

func estimateSplitComparison(added domain.ResourceDemand, baseline []domain.BaselineDemandEntry, index estimateIndex, fallback float64) EstimateSplitComparison {
	row := EstimateSplitComparison{
		AddedDemandID: added.ID, AddedResourceID: demandResourceID(added), Unit: added.Unit,
		CurrentPlannedAmount: added.PlannedAmount, BaselineBasis: "task_unit_total", BaselineAmount: fallback,
	}
	type candidate struct {
		demand    domain.BaselineDemandEntry
		reduction float64
	}
	var candidates []candidate
	for _, original := range baseline {
		if original.Unit != added.Unit {
			continue
		}
		current, ok := index.demands[original.ID]
		if !ok {
			continue
		}
		reduction := estimateBaselineAmount(original) - current.PlannedAmount
		if reduction > 0 {
			candidates = append(candidates, candidate{demand: original, reduction: reduction})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].reduction != candidates[j].reduction {
			return candidates[i].reduction > candidates[j].reduction
		}
		return candidates[i].demand.ID < candidates[j].demand.ID
	})
	if len(candidates) > 0 {
		original := candidates[0].demand
		row.BaselineBasis, row.BaselineDemandID, row.BaselineResourceID = "reduced_original_demand", original.ID, original.Resource
		row.BaselineAmount = estimateBaselineAmount(original)
	}
	return row
}

func estimateTaskTotals(demands []EstimateDemand) []EstimateUnitTotals {
	byUnit := map[string]*EstimateUnitTotals{}
	for _, demand := range demands {
		row := byUnit[demand.Unit]
		if row == nil {
			row = &EstimateUnitTotals{Unit: demand.Unit}
			byUnit[demand.Unit] = row
		}
		row.BaselineAmount += demand.BaselineAmount
		row.CurrentPlannedAmount += demand.CurrentPlannedAmount
		row.CurrentRemainingAmount += demand.CurrentRemainingAmount
		row.ActualAmount += demand.ActualAmount
	}
	var rows []EstimateUnitTotals
	for _, row := range byUnit {
		row.VarianceAmount = row.ActualAmount - row.BaselineAmount
		row.ActualBaselineRatio = amountRatio(row.ActualAmount, row.BaselineAmount)
		rows = append(rows, *row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Unit < rows[j].Unit })
	return rows
}

func addEstimateResourceRows(rows map[string]*EstimateResourceSummary, task EstimateTask, index estimateIndex) {
	for _, demand := range task.Demands {
		resourceID := demand.ResourceID
		if resourceID == "" {
			resourceID = demand.BaselineResourceID
		}
		key := resourceID + "\x00" + demand.Unit
		row := rows[key]
		if row == nil {
			row = &EstimateResourceSummary{ResourceID: resourceID, ResourceName: index.resourceNames[resourceID], Unit: demand.Unit}
			rows[key] = row
		}
		row.All.BaselineAmount += demand.BaselineAmount
		row.All.CurrentPlannedAmount += demand.CurrentPlannedAmount
		row.All.ActualAmount += demand.ActualAmount
		row.ActualsRecorded = row.ActualsRecorded || demand.ActualsRecorded
		if !task.Open {
			row.Finished.BaselineAmount += demand.BaselineAmount
			row.Finished.CurrentPlannedAmount += demand.CurrentPlannedAmount
			row.Finished.ActualAmount += demand.ActualAmount
		}
	}
}

func estimateBaselineAmount(demand domain.BaselineDemandEntry) float64 {
	if demand.PlannedAmount == nil {
		return 0
	}
	return *demand.PlannedAmount
}

func estimateDemandActual(actuals []domain.Actual, unit string) (float64, bool) {
	var amount float64
	recorded := false
	for _, actual := range actuals {
		if estimateConsumptionTypes[actual.Type] && actual.Unit == unit {
			amount += actual.Amount
			recorded = true
		}
	}
	return amount, recorded
}

func amountRatio(actual, baseline float64) *float64 {
	if baseline <= 0 {
		return nil
	}
	ratio := actual / baseline
	return &ratio
}

func estimateSplitGroups(tasks []EstimateTask, resources []EstimateResourceSummary, index estimateIndex) []EstimateSplitGroupSummary {
	type splitGroupState struct {
		row            EstimateSplitGroupSummary
		groupResources map[string]bool
	}
	states := map[string]*splitGroupState{}
	for _, task := range tasks {
		for _, split := range task.SplitComparisons {
			if split.BaselineBasis != "reduced_original_demand" || split.BaselineResourceID == "" {
				continue
			}
			key := split.BaselineResourceID + "\x00" + split.Unit
			state := states[key]
			if state == nil {
				state = &splitGroupState{
					row:            EstimateSplitGroupSummary{OriginalResourceID: split.BaselineResourceID, OriginalResourceName: index.resourceNames[split.BaselineResourceID], Unit: split.Unit},
					groupResources: map[string]bool{split.BaselineResourceID: true},
				}
				states[key] = state
			}
			state.groupResources[split.AddedResourceID] = true
		}
	}
	rows := make([]EstimateSplitGroupSummary, 0, len(states))
	for _, state := range states {
		for resourceID := range state.groupResources {
			if resourceID != state.row.OriginalResourceID {
				state.row.SplitResourceIDs = append(state.row.SplitResourceIDs, resourceID)
			}
		}
		sort.Strings(state.row.SplitResourceIDs)
		for _, resource := range resources {
			if resource.Unit != state.row.Unit || !state.groupResources[resource.ResourceID] || !resource.ActualsRecorded {
				continue
			}
			state.row.ActualsRecorded = true
			addEstimateResourceAmounts(&state.row.Finished, resource.Finished)
			addEstimateResourceAmounts(&state.row.All, resource.All)
		}
		if state.row.ActualsRecorded {
			state.row.Finished.ActualBaselineRatio = amountRatio(state.row.Finished.ActualAmount, state.row.Finished.BaselineAmount)
			state.row.All.ActualBaselineRatio = amountRatio(state.row.All.ActualAmount, state.row.All.BaselineAmount)
		}
		rows = append(rows, state.row)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].OriginalResourceID != rows[j].OriginalResourceID {
			return rows[i].OriginalResourceID < rows[j].OriginalResourceID
		}
		return rows[i].Unit < rows[j].Unit
	})
	return rows
}

func addEstimateResourceAmounts(target *EstimateResourceAmounts, source EstimateResourceAmounts) {
	target.BaselineAmount += source.BaselineAmount
	target.CurrentPlannedAmount += source.CurrentPlannedAmount
	target.ActualAmount += source.ActualAmount
}

func roundEstimateResult(result *EstimateReportResult) {
	for i := range result.Resources {
		roundEstimateResourceAmounts(&result.Resources[i].Finished)
		roundEstimateResourceAmounts(&result.Resources[i].All)
	}
	for i := range result.SplitGroups {
		roundEstimateResourceAmounts(&result.SplitGroups[i].Finished)
		roundEstimateResourceAmounts(&result.SplitGroups[i].All)
	}
	for i := range result.Tasks {
		for j := range result.Tasks[i].Demands {
			demand := &result.Tasks[i].Demands[j]
			demand.BaselineAmount = roundEstimate(demand.BaselineAmount)
			demand.CurrentPlannedAmount = roundEstimate(demand.CurrentPlannedAmount)
			demand.CurrentRemainingAmount = roundEstimate(demand.CurrentRemainingAmount)
			demand.ActualAmount = roundEstimate(demand.ActualAmount)
			demand.VarianceAmount = roundEstimate(demand.VarianceAmount)
			demand.ActualBaselineRatio = roundEstimatePointer(demand.ActualBaselineRatio)
		}
		for j := range result.Tasks[i].Totals {
			total := &result.Tasks[i].Totals[j]
			total.BaselineAmount = roundEstimate(total.BaselineAmount)
			total.CurrentPlannedAmount = roundEstimate(total.CurrentPlannedAmount)
			total.CurrentRemainingAmount = roundEstimate(total.CurrentRemainingAmount)
			total.ActualAmount = roundEstimate(total.ActualAmount)
			total.VarianceAmount = roundEstimate(total.VarianceAmount)
			total.ActualBaselineRatio = roundEstimatePointer(total.ActualBaselineRatio)
		}
		for j := range result.Tasks[i].SplitComparisons {
			split := &result.Tasks[i].SplitComparisons[j]
			split.CurrentPlannedAmount = roundEstimate(split.CurrentPlannedAmount)
			split.BaselineAmount = roundEstimate(split.BaselineAmount)
		}
	}
}

func roundEstimateResourceAmounts(amounts *EstimateResourceAmounts) {
	amounts.BaselineAmount = roundEstimate(amounts.BaselineAmount)
	amounts.CurrentPlannedAmount = roundEstimate(amounts.CurrentPlannedAmount)
	amounts.ActualAmount = roundEstimate(amounts.ActualAmount)
	amounts.ActualBaselineRatio = roundEstimatePointer(amounts.ActualBaselineRatio)
}

func roundEstimatePointer(value *float64) *float64 {
	if value == nil {
		return nil
	}
	rounded := roundEstimate(*value)
	return &rounded
}

func roundEstimate(value float64) float64 {
	const precision = 1_000_000
	return math.Round(value*precision) / precision
}

func estimateTaskFinished(status string) bool {
	switch strings.ToLower(status) {
	case "accepted", "done", "closed":
		return true
	default:
		return false
	}
}

func signedWorkingDays(from, to time.Time, days map[time.Weekday]bool) int {
	if from.Equal(to) {
		return 0
	}
	sign := 1
	if to.Before(from) {
		from, to, sign = to, from, -1
	}
	count := 0
	for day := from.AddDate(0, 0, 1); !day.After(to); day = day.AddDate(0, 0, 1) {
		if len(days) == 0 || days[day.Weekday()] {
			count++
		}
	}
	return sign * count
}
