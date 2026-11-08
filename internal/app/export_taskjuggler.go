package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/kogriv/replan/internal/domain"
)

type ExportTaskJugglerRequest struct {
	ScenarioID                   string
	UnrepresentedAssumptionsMode string
	Raw                          bool
	Now                          time.Time
}

type ExportTaskJugglerResult struct {
	Metadata           ExportMetadata                    `json:"metadata"`
	Scenario           string                            `json:"scenario,omitempty"`
	Calibrations       []string                          `json:"calibrations,omitempty"`
	Content            string                            `json:"content,omitempty"`
	Mapping            TaskJugglerExportMapping          `json:"mapping"`
	UnsupportedDemands []TaskJugglerUnsupportedDemandSet `json:"unsupported_demands"`
	Warnings           []Diagnostic                      `json:"warnings"`
	Errors             []Diagnostic                      `json:"errors"`
}

type TaskJugglerUnsupportedDemandSet struct {
	Unit      string   `json:"unit"`
	Count     int      `json:"count"`
	OpenCount int      `json:"open_count"`
	DemandIDs []string `json:"demand_ids"`
}

type TaskJugglerExportMapping struct {
	Version       int                            `json:"version"`
	ProjectPath   string                         `json:"project_path"`
	ScenarioID    string                         `json:"scenario_id,omitempty"`
	TaskIDMap     []TaskJugglerTaskIDMapping     `json:"task_id_map"`
	ResourceIDMap []TaskJugglerResourceIDMapping `json:"resource_id_map"`
}

type TaskJugglerTaskIDMapping struct {
	RePlanTaskID      string `json:"replan_task_id"`
	TaskJugglerTaskID string `json:"taskjuggler_task_id"`
}

type TaskJugglerResourceIDMapping struct {
	RePlanResourceID      string `json:"replan_resource_id"`
	TaskJugglerResourceID string `json:"taskjuggler_resource_id"`
}

type taskJugglerDemand struct {
	resourceID    string
	effort        float64
	rawEffort     float64
	calibrationID string
}

type taskJugglerResourceLimit struct {
	hasWeeklyMax bool
	weeklyMax    float64
}

type taskJugglerWorkingHours struct {
	hasWindows bool
	byDay      map[int][]taskJugglerTimeRange
}

type taskJugglerTimeRange struct {
	start int
	end   int
}

const (
	taskJugglerInternalFlag     = "replan_internal"
	taskJugglerStatusDateTaskID = "replan_status_date"
)

type taskJugglerTaskSchedule struct {
	finished    bool
	start       string
	finish      string
	windowStart string
}

func (s ExportService) TaskJuggler(ctx context.Context, req ExportTaskJugglerRequest) (ExportTaskJugglerResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	result := ExportTaskJugglerResult{
		Metadata: ExportMetadata{
			ID:          "export.replan.taskjuggler." + compactTimestamp(now.Format(time.RFC3339)),
			Format:      "taskjuggler",
			ExportedAt:  now.Format(time.RFC3339),
			ProjectPath: s.loader.ProjectPath(),
		},
		UnsupportedDemands: []TaskJugglerUnsupportedDemandSet{},
		Warnings:           []Diagnostic{},
		Errors:             []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if req.ScenarioID != "" {
		overlay := ApplyScenarioOverlay(ws, req.ScenarioID)
		result.Warnings = append(result.Warnings, overlay.Warnings...)
		result.Errors = append(result.Errors, overlay.Errors...)
		if len(overlay.Errors) == 0 {
			ws = overlay.Workspace
			result.Scenario = overlay.Scenario.ID
			appendTaskJugglerUnrepresentedCapacityDiagnostics(overlay.Workspace, overlay.Scenario, req.UnrepresentedAssumptionsMode, &result.Warnings, &result.Errors)
		}
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	result.Mapping = buildTaskJugglerExportMapping(ws, result.Metadata.ProjectPath, result.Scenario, &result.Errors)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	dependenciesByTask := buildTaskJugglerDependencies(ws, result.Mapping, &result.Warnings, &result.Errors)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	calibrations := map[string]DemandCalibration{}
	if !req.Raw {
		calibrations = ApplicableDemandCalibrations(calibrationSelectionWorkspace(ws, result.Scenario))
	}
	result.Content, result.Calibrations, result.UnsupportedDemands = buildTaskJugglerContent(ws, result.Metadata, result.Scenario, result.Mapping, dependenciesByTask, calibrations, &result.Warnings)
	return result, nil
}

func buildTaskJugglerDependencies(ws domain.Workspace, mapping TaskJugglerExportMapping, warnings, errors *[]Diagnostic) map[string][]string {
	exportedTaskIDs := make(map[string]string, len(mapping.TaskIDMap))
	for _, entry := range mapping.TaskIDMap {
		exportedTaskIDs[entry.RePlanTaskID] = entry.TaskJugglerTaskID
	}

	predecessorsByTask := map[string]map[string]struct{}{}
	graph := map[string][]string{}
	notExportedIDs := []string{}
	reasonCounts := map[string]int{}
	for _, dependency := range ws.Dependencies {
		if !dependencyActive(dependency.Status) || dependency.Type == "relates" {
			continue
		}
		if dependency.Type == "external_dependency" {
			notExportedIDs = append(notExportedIDs, dependency.ID)
			reasonCounts["external_dependency"]++
			continue
		}
		if !taskJugglerOrderingDependency(dependency.Type) {
			continue
		}

		predecessorTaskJugglerID, predecessorOK := exportedTaskIDs[dependency.PredecessorRef.ID]
		predecessorOK = predecessorOK && dependency.PredecessorRef.Kind == "task"
		_, successorOK := exportedTaskIDs[dependency.SuccessorRef.ID]
		successorOK = successorOK && dependency.SuccessorRef.Kind == "task"
		if !predecessorOK || !successorOK {
			notExportedIDs = append(notExportedIDs, dependency.ID)
			if !predecessorOK {
				reasonCounts["predecessor not a task in the export"]++
			}
			if !successorOK {
				reasonCounts["successor not a task in the export"]++
			}
			continue
		}

		if predecessorsByTask[dependency.SuccessorRef.ID] == nil {
			predecessorsByTask[dependency.SuccessorRef.ID] = map[string]struct{}{}
		}
		predecessorsByTask[dependency.SuccessorRef.ID][predecessorTaskJugglerID] = struct{}{}
		graph[dependency.PredecessorRef.ID] = appendTaskJugglerUniqueString(graph[dependency.PredecessorRef.ID], dependency.SuccessorRef.ID)
		if _, ok := graph[dependency.SuccessorRef.ID]; !ok {
			graph[dependency.SuccessorRef.ID] = nil
		}
	}

	appendTaskJugglerDependenciesNotExportedDiagnostic(ws.Project.ID, notExportedIDs, reasonCounts, warnings)
	if cycle := taskJugglerDependencyCycle(graph); len(cycle) > 0 {
		*errors = append(*errors, Diagnostic{
			Code:     "taskjuggler_dependency_cycle",
			Message:  fmt.Sprintf("TaskJuggler dependency cycle contains tasks %s", strings.Join(cycle, " -> ")),
			Severity: "error",
			Target:   Target{Kind: "task", ID: cycle[0]},
		})
	}

	result := make(map[string][]string, len(predecessorsByTask))
	for taskID, predecessors := range predecessorsByTask {
		result[taskID] = make([]string, 0, len(predecessors))
		for predecessorID := range predecessors {
			result[taskID] = append(result[taskID], predecessorID)
		}
		sort.Strings(result[taskID])
	}
	return result
}

func taskJugglerOrderingDependency(dependencyType string) bool {
	switch dependencyType {
	case "finish_to_start", "blocks", "requires_input_from", "requires_resource_from":
		return true
	default:
		return false
	}
}

func appendTaskJugglerDependenciesNotExportedDiagnostic(projectID string, dependencyIDs []string, reasonCounts map[string]int, warnings *[]Diagnostic) {
	if len(dependencyIDs) == 0 {
		return
	}
	sort.Strings(dependencyIDs)
	reasons := make([]string, 0, len(reasonCounts))
	for _, reason := range []string{"external_dependency", "predecessor not a task in the export", "successor not a task in the export"} {
		if count := reasonCounts[reason]; count > 0 {
			reasons = append(reasons, fmt.Sprintf("%s: %d", reason, count))
		}
	}
	examples := dependencyIDs
	if len(examples) > 3 {
		examples = examples[:3]
	}
	message := fmt.Sprintf("%d active dependencies cannot be represented in the TaskJuggler export (%s); they are not exported", len(dependencyIDs), strings.Join(reasons, ", "))
	if len(dependencyIDs) == 1 {
		message = fmt.Sprintf("1 active dependency cannot be represented in the TaskJuggler export (%s); it is not exported", strings.Join(reasons, ", "))
	}
	message += fmt.Sprintf(" (for example %s)", strings.Join(examples, ", "))
	*warnings = append(*warnings, Diagnostic{
		Code:     "taskjuggler_dependencies_not_exported",
		Message:  message,
		Severity: "warning",
		Target:   Target{Kind: "project", ID: projectID},
	})
}

func appendTaskJugglerUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func taskJugglerDependencyCycle(graph map[string][]string) []string {
	const (
		unvisited = iota
		visiting
		visited
	)
	state := map[string]int{}
	stack := []string{}
	stackIndex := map[string]int{}
	var cycle []string
	var visit func(string) bool
	visit = func(taskID string) bool {
		state[taskID] = visiting
		stackIndex[taskID] = len(stack)
		stack = append(stack, taskID)
		successors := append([]string(nil), graph[taskID]...)
		sort.Strings(successors)
		for _, successorID := range successors {
			switch state[successorID] {
			case unvisited:
				if visit(successorID) {
					return true
				}
			case visiting:
				cycle = append(cycle, stack[stackIndex[successorID]:]...)
				cycle = append(cycle, successorID)
				return true
			}
		}
		stack = stack[:len(stack)-1]
		delete(stackIndex, taskID)
		state[taskID] = visited
		return false
	}

	taskIDs := make([]string, 0, len(graph))
	for taskID := range graph {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	for _, taskID := range taskIDs {
		if state[taskID] == unvisited && visit(taskID) {
			return cycle
		}
	}
	return nil
}

func buildTaskJugglerExportMapping(ws domain.Workspace, projectPath, scenarioID string, errors *[]Diagnostic) TaskJugglerExportMapping {
	mapping := TaskJugglerExportMapping{
		Version:       1,
		ProjectPath:   projectPath,
		ScenarioID:    scenarioID,
		TaskIDMap:     []TaskJugglerTaskIDMapping{},
		ResourceIDMap: []TaskJugglerResourceIDMapping{},
	}
	// The status-date milestone's id is reserved.
	taskJugglerTaskIDs := map[string]string{taskJugglerStatusDateTaskID: "the status-date milestone"}
	for _, task := range ws.Tasks {
		taskJugglerTaskID := taskJugglerID(task.ID, "task")
		if previous, exists := taskJugglerTaskIDs[taskJugglerTaskID]; exists {
			*errors = append(*errors, Diagnostic{
				Code:     "taskjuggler_task_id_collision",
				Message:  fmt.Sprintf("tasks %s and %s both map to TaskJuggler id %s", previous, task.ID, taskJugglerTaskID),
				Severity: "error",
				Target:   Target{Kind: "task", ID: task.ID},
			})
			continue
		}
		taskJugglerTaskIDs[taskJugglerTaskID] = task.ID
		mapping.TaskIDMap = append(mapping.TaskIDMap, TaskJugglerTaskIDMapping{
			RePlanTaskID:      task.ID,
			TaskJugglerTaskID: taskJugglerTaskID,
		})
	}
	taskJugglerResourceIDs := map[string]string{}
	for _, resource := range ws.Resources {
		taskJugglerResourceID := taskJugglerID(resource.ID, "resource")
		if previous, exists := taskJugglerResourceIDs[taskJugglerResourceID]; exists {
			*errors = append(*errors, Diagnostic{
				Code:     "taskjuggler_resource_id_collision",
				Message:  fmt.Sprintf("resources %s and %s both map to TaskJuggler id %s", previous, resource.ID, taskJugglerResourceID),
				Severity: "error",
				Target:   Target{Kind: "resource", ID: resource.ID},
			})
			continue
		}
		taskJugglerResourceIDs[taskJugglerResourceID] = resource.ID
		mapping.ResourceIDMap = append(mapping.ResourceIDMap, TaskJugglerResourceIDMapping{
			RePlanResourceID:      resource.ID,
			TaskJugglerResourceID: taskJugglerResourceID,
		})
	}
	return mapping
}

func appendTaskJugglerUnrepresentedCapacityDiagnostics(ws domain.Workspace, scenario domain.Scenario, mode string, warnings, errors *[]Diagnostic) {
	capacities := make(map[string]domain.Capacity, len(ws.Capacities))
	for _, capacity := range ws.Capacities {
		capacities[capacity.ID] = capacity
	}
	severity := "warning"
	target := warnings
	if strings.EqualFold(strings.TrimSpace(mode), "error") {
		severity = "error"
		target = errors
	}
	for _, change := range scenario.Changes {
		if !isSetOperation(change.Op) || !strings.HasPrefix(change.Path, "capacities.") || !strings.HasSuffix(change.Path, ".amount") {
			continue
		}
		capacityID := strings.TrimSuffix(strings.TrimPrefix(change.Path, "capacities."), ".amount")
		if capacityID == "" {
			continue
		}
		capacity, ok := capacities[capacityID]
		if !ok {
			continue
		}
		if isTaskJugglerWeeklyHourCapacity(capacity) {
			continue
		}
		*target = append(*target, Diagnostic{
			Code:     "taskjuggler_capacity_change_not_represented",
			Message:  taskJugglerCapacityChangeWarningMessage(scenario.ID, capacity),
			Severity: severity,
			Target:   Target{Kind: "capacity", ID: capacity.ID},
		})
	}
}

func isTaskJugglerWeeklyHourCapacity(capacity domain.Capacity) bool {
	capacityType := strings.ToLower(strings.TrimSpace(capacity.Type))
	resourceClass := strings.ToLower(strings.TrimSpace(capacity.ResourceClass))
	period := strings.ToLower(strings.TrimSpace(capacity.Period))
	return capacityType == "periodic_capacity" &&
		(resourceClass == "human" || resourceClass == "ai_agent") &&
		isTaskJugglerEffortUnit(capacity.Unit) &&
		period == "week"
}

func taskJugglerCapacityChangeWarningMessage(scenarioID string, capacity domain.Capacity) string {
	details := []string{
		"scenario " + scenarioID + " changes capacity " + capacity.ID,
	}
	if capacity.Type != "" {
		details = append(details, "type "+capacity.Type)
	}
	if capacity.ResourceRef.ID != "" {
		details = append(details, "resource "+capacity.ResourceRef.ID)
	}
	if capacity.Unit != "" {
		details = append(details, fmt.Sprintf("unit %s", capacity.Unit))
	}
	if capacity.Period != "" {
		details = append(details, fmt.Sprintf("period %s", capacity.Period))
	}
	details = append(details, fmt.Sprintf("amount %.2f", capacity.Amount))
	return strings.Join(details, ", ") + "; this RePlan capacity amount is applied by workload reports but is not represented in the generated TaskJuggler .tjp export"
}

func buildTaskJugglerContent(ws domain.Workspace, metadata ExportMetadata, scenarioID string, mapping TaskJugglerExportMapping, dependenciesByTask map[string][]string, calibrations map[string]DemandCalibration, warnings *[]Diagnostic) (string, []string, []TaskJugglerUnsupportedDemandSet) {
	resourceIDs := make(map[string]struct{}, len(ws.Resources))
	for _, resource := range ws.Resources {
		resourceIDs[resource.ID] = struct{}{}
	}
	taskJugglerTaskIDs := map[string]string{}
	for _, entry := range mapping.TaskIDMap {
		taskJugglerTaskIDs[entry.RePlanTaskID] = entry.TaskJugglerTaskID
	}
	taskJugglerResourceIDs := map[string]string{}
	for _, entry := range mapping.ResourceIDMap {
		taskJugglerResourceIDs[entry.RePlanResourceID] = entry.TaskJugglerResourceID
	}
	limitsByResource := taskJugglerLimitsByResource(ws.Capacities, ws.Project.DefaultStatusDate, warnings)
	demandsByTask := make(map[string][]taskJugglerDemand)
	demandCountByTask := map[string]int{}
	loadingDemandCountByTask := map[string]int{}
	unsupportedDemandIDsByUnit := map[string][]string{}
	openUnsupportedDemandIDsByUnit := map[string][]string{}
	tasksByID := make(map[string]domain.Task, len(ws.Tasks))
	demandIDsByTask := make(map[string][]string)
	for _, task := range ws.Tasks {
		tasksByID[task.ID] = task
	}
	for _, demand := range ws.ResourceDemands {
		demandIDsByTask[demand.TaskRef.ID] = append(demandIDsByTask[demand.TaskRef.ID], demand.ID)
	}
	taskSchedules := taskJugglerTaskSchedules(ws, demandIDsByTask, warnings)
	usedCalibrations := map[string]bool{}
	for _, demand := range ws.ResourceDemands {
		demandCountByTask[demand.TaskRef.ID]++
		if domain.DemandLoads(demand.Status) {
			loadingDemandCountByTask[demand.TaskRef.ID]++
		}
		if domain.DemandLoads(demand.Status) && !isTaskJugglerEffortUnit(demand.Unit) {
			unsupportedDemandIDsByUnit[demand.Unit] = append(unsupportedDemandIDsByUnit[demand.Unit], demand.ID)
			if task, ok := tasksByID[demand.TaskRef.ID]; ok && !taskSchedules[task.ID].finished && !isTerminalTrackingStatus(task.TrackingStatus) && taskJugglerDemandHasOpenWork(demand) {
				openUnsupportedDemandIDsByUnit[demand.Unit] = append(openUnsupportedDemandIDsByUnit[demand.Unit], demand.ID)
			}
		}
		mapped, ok := mapTaskJugglerDemand(demand, resourceIDs, calibrations[demand.ID], warnings)
		if ok {
			task := tasksByID[demand.TaskRef.ID]
			if !taskSchedules[demand.TaskRef.ID].finished && !strings.EqualFold(strings.TrimSpace(task.TrackingStatus), "cancelled") {
				demandsByTask[demand.TaskRef.ID] = append(demandsByTask[demand.TaskRef.ID], mapped)
			}
			if mapped.calibrationID != "" && !taskSchedules[demand.TaskRef.ID].finished && !strings.EqualFold(strings.TrimSpace(task.TrackingStatus), "cancelled") {
				usedCalibrations[mapped.calibrationID] = true
			}
		}
	}
	unsupportedDemands := appendTaskJugglerUnsupportedDemandDiagnostics(unsupportedDemandIDsByUnit, openUnsupportedDemandIDsByUnit, warnings)
	appendTaskJugglerTasksWithoutDemandsDiagnostic(ws, demandCountByTask, taskSchedules, warnings)
	taskEfforts, roundedUpTaskIDs, timingResolution := taskJugglerTaskEfforts(ws.Tasks, demandsByTask)
	appendTaskJugglerRoundedUpEffortDiagnostic(ws.Project.ID, roundedUpTaskIDs, warnings)
	workingHoursByResource := taskJugglerWorkingHoursByResource(ws.Availabilities, timingResolution, ws.Project.Timezone, ws.Project.ID, warnings)
	vacationsByResource := taskJugglerVacationsByResource(ws.Availabilities, ws.Project.ID, warnings)

	var b strings.Builder
	fmt.Fprintf(&b, "# Generated by RePlan\n")
	fmt.Fprintf(&b, "# Export ID: %s\n", metadata.ID)
	fmt.Fprintf(&b, "# Exported at: %s\n", metadata.ExportedAt)
	fmt.Fprintf(&b, "# Project path: %s\n\n", metadata.ProjectPath)
	if scenarioID != "" {
		fmt.Fprintf(&b, "# Scenario: %s\n\n", scenarioID)
	}

	statusDate := ws.Project.DefaultStatusDate
	writeTaskJugglerProject(&b, ws.Project, statusDate, taskSchedules, timingResolution, warnings)

	if statusDate != "" {
		fmt.Fprintf(&b, "flags %s\n\n", taskJugglerInternalFlag)
	}
	for _, resource := range ws.Resources {
		writeTaskJugglerResource(&b, resource, taskJugglerResourceIDs[resource.ID], workingHoursByResource[resource.ID], vacationsByResource[resource.ID], limitsByResource[resource.ID])
	}
	if len(ws.Resources) > 0 {
		b.WriteString("\n")
	}
	if statusDate != "" {
		// TaskJuggler takes a task's start together with depends as a fixed
		// date that the dependency may contradict, so open work waits for a
		// status-date milestone instead. The flag hides it from the report
		// that taskjuggler import reads.
		fmt.Fprintf(&b, "task %s \"Status date\" {\n  start %s\n  milestone\n  flags %s\n}\n", taskJugglerStatusDateTaskID, statusDate, taskJugglerInternalFlag)
	}

	for _, task := range ws.Tasks {
		writeTaskJugglerTask(&b, task, taskJugglerTaskIDs[task.ID], taskJugglerResourceIDs, dependenciesByTask[task.ID], demandsByTask[task.ID], taskEfforts[task.ID], loadingDemandCountByTask[task.ID] > 0, taskSchedules[task.ID], statusDate, warnings)
	}
	writeTaskJugglerParseableTaskReport(&b, statusDate != "")

	calibrationIDs := make([]string, 0, len(usedCalibrations))
	for calibrationID := range usedCalibrations {
		calibrationIDs = append(calibrationIDs, calibrationID)
	}
	sort.Strings(calibrationIDs)
	return b.String(), calibrationIDs, unsupportedDemands
}

// writeTaskJugglerProject writes the project block: window, timezone, the
// status date as now and the timing resolution.
func writeTaskJugglerProject(b *strings.Builder, project domain.Project, statusDate string, taskSchedules map[string]taskJugglerTaskSchedule, timingResolution int, warnings *[]Diagnostic) {
	startDate, endDate := taskJugglerProjectWindow(statusDate, taskSchedules)
	if statusDate == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_status_date_missing",
			Message:  "project has no default status date; TaskJuggler keeps its run-time today as the project start and progress date",
			Severity: "warning",
			Target:   Target{Kind: "project", ID: project.ID},
		})
	}
	timezone := project.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	fmt.Fprintf(b, "project %s %q \"1.0\" %s %s {\n", taskJugglerID(project.ID, "project_replan"), project.Name, startDate, taskJugglerProjectEnd(endDate))
	fmt.Fprintf(b, "  timezone %q\n", timezone)
	if statusDate != "" {
		fmt.Fprintf(b, "  now %s\n", statusDate)
	}
	if timingResolution < 60 {
		fmt.Fprintf(b, "  timingresolution %dmin\n", timingResolution)
	}
	fmt.Fprintf(b, "}\n\n")
}

func taskJugglerTaskSchedules(ws domain.Workspace, demandIDsByTask map[string][]string, warnings *[]Diagnostic) map[string]taskJugglerTaskSchedule {
	schedules := make(map[string]taskJugglerTaskSchedule, len(ws.Tasks))
	withoutDates := []string{}
	for _, task := range ws.Tasks {
		finished := taskJugglerTaskFinished(task, ws.ResourceDemands)
		actual := deriveTaskActualWindow(ws, task.ID, demandIDsByTask[task.ID], false)
		windowStart := task.PlannedStart
		if actual.Start != "" && (windowStart == "" || actual.Start < windowStart) {
			windowStart = actual.Start
		}
		schedule := taskJugglerTaskSchedule{finished: finished, start: actual.Start, finish: actual.Finish, windowStart: windowStart}
		if schedule.start == "" {
			schedule.start = task.PlannedStart
		}
		if schedule.finish == "" {
			schedule.finish = task.PlannedFinish
		}
		// A cancelled task with dates stays where it was, so a run does not
		// propose moving it; one without dates is exported as before.
		if strings.EqualFold(strings.TrimSpace(task.TrackingStatus), "cancelled") && (schedule.start != "" || schedule.finish != "") {
			finished = true
			schedule.finished = true
		}
		if finished {
			if schedule.start == "" && schedule.finish == "" {
				schedule.start, schedule.finish = ws.Project.DefaultStatusDate, ws.Project.DefaultStatusDate
				if schedule.start == "" {
					schedule.start, schedule.finish = "today", "today"
				}
				withoutDates = append(withoutDates, task.ID)
			} else if schedule.start == "" {
				schedule.start = schedule.finish
			} else if schedule.finish == "" || schedule.finish < schedule.start {
				// An actual start after the planned finish leaves no planned
				// finish to trust; TaskJuggler refuses an end before the start.
				schedule.finish = schedule.start
			}
		}
		schedules[task.ID] = schedule
	}
	if len(withoutDates) > 0 {
		sort.Strings(withoutDates)
		examples := withoutDates
		if len(examples) > 3 {
			examples = examples[:3]
		}
		message := fmt.Sprintf("%d finished tasks have no actual or planned dates; they are fixed at the TaskJuggler status date", len(withoutDates))
		if len(withoutDates) == 1 {
			message = "1 finished task has no actual or planned dates; it is fixed at the TaskJuggler status date"
		}
		message += fmt.Sprintf(" (for example %s)", strings.Join(examples, ", "))
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_finished_task_without_dates",
			Message:  message,
			Severity: "warning",
			Target:   Target{Kind: "project", ID: ws.Project.ID},
		})
	}
	return schedules
}

func taskJugglerTaskFinished(task domain.Task, demands []domain.ResourceDemand) bool {
	switch strings.ToLower(strings.TrimSpace(task.TrackingStatus)) {
	case "accepted", "done":
		return true
	case "review_ready":
		for _, demand := range demands {
			if demand.TaskRef.ID == task.ID && domain.DemandLoads(demand.Status) && taskJugglerDemandHasOpenWork(demand) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func taskJugglerProjectWindow(statusDate string, schedules map[string]taskJugglerTaskSchedule) (string, string) {
	if statusDate == "" {
		return "today", "+1y"
	}
	start := statusDate
	for _, schedule := range schedules {
		if schedule.windowStart != "" && schedule.windowStart < start {
			start = schedule.windowStart
		}
	}
	// The project line is read before its timezone, as UTC, so a task date
	// at local midnight east of UTC would fall before the window: start the
	// window a day early.
	first, _ := time.Parse("2006-01-02", start)
	parsed, _ := time.Parse("2006-01-02", statusDate)
	return first.AddDate(0, 0, -1).Format("2006-01-02"), parsed.AddDate(1, 0, 0).Format("2006-01-02")
}

// taskJugglerProjectEnd writes the project end: a relative duration such as
// +1y follows the start directly, an absolute date needs the "- " TaskJuggler
// expects between the two dates.
func taskJugglerProjectEnd(end string) string {
	if strings.HasPrefix(end, "+") {
		return end
	}
	return "- " + end
}

func taskJugglerTaskEfforts(tasks []domain.Task, demandsByTask map[string][]taskJugglerDemand) (map[string]float64, map[string]struct{}, int) {
	efforts := make(map[string]float64, len(tasks))
	roundedUpTaskIDs := map[string]struct{}{}
	minimumMinutes := 60.0
	for _, task := range tasks {
		for _, demand := range demandsByTask[task.ID] {
			efforts[task.ID] += demand.effort
		}
		if efforts[task.ID] <= 0 {
			continue
		}
		minutes := float64(taskJugglerEffortMinutes(efforts[task.ID]))
		if minutes < minimumMinutes {
			minimumMinutes = minutes
		}
		if minutes == 5 && efforts[task.ID]*60 < 5 {
			roundedUpTaskIDs[task.ID] = struct{}{}
		}
	}
	for _, resolution := range []int{60, 30, 20, 15, 10, 5} {
		if float64(resolution) <= minimumMinutes {
			return efforts, roundedUpTaskIDs, resolution
		}
	}
	return efforts, roundedUpTaskIDs, 5
}

// taskJugglerEffortMinutes writes a sub-hour effort as whole minutes,
// rounded up so the scheduled work is never smaller than the plan, and never
// below TaskJuggler's 5-minute minimum timing resolution.
func taskJugglerEffortMinutes(hours float64) int {
	minutes := int(math.Ceil(hours*60 - 1e-9))
	if minutes < 5 {
		return 5
	}
	return minutes
}

func appendTaskJugglerRoundedUpEffortDiagnostic(projectID string, roundedUpTaskIDs map[string]struct{}, warnings *[]Diagnostic) {
	if len(roundedUpTaskIDs) == 0 {
		return
	}
	taskIDs := make([]string, 0, len(roundedUpTaskIDs))
	for taskID := range roundedUpTaskIDs {
		taskIDs = append(taskIDs, taskID)
	}
	sort.Strings(taskIDs)
	examples := taskIDs
	if len(examples) > 3 {
		examples = examples[:3]
	}
	message := fmt.Sprintf("%d exported task efforts are below TaskJuggler's 5-minute minimum and were rounded up to 5min", len(taskIDs))
	if len(taskIDs) == 1 {
		message = "1 exported task effort is below TaskJuggler's 5-minute minimum and was rounded up to 5min"
	}
	message += fmt.Sprintf(" (for example %s)", strings.Join(examples, ", "))
	*warnings = append(*warnings, Diagnostic{
		Code:     "taskjuggler_effort_rounded_up",
		Message:  message,
		Severity: "warning",
		Target:   Target{Kind: "project", ID: projectID},
	})
}

func appendTaskJugglerTasksWithoutDemandsDiagnostic(ws domain.Workspace, demandCountByTask map[string]int, schedules map[string]taskJugglerTaskSchedule, warnings *[]Diagnostic) {
	taskIDs := []string{}
	for _, task := range ws.Tasks {
		if demandCountByTask[task.ID] == 0 && !schedules[task.ID].finished && !isTerminalTrackingStatus(task.TrackingStatus) {
			taskIDs = append(taskIDs, task.ID)
		}
	}
	if len(taskIDs) == 0 {
		return
	}
	sort.Strings(taskIDs)
	examples := taskIDs
	if len(examples) > 3 {
		examples = examples[:3]
	}
	message := fmt.Sprintf("%d non-terminal tasks have no resource demands; no effort is exported for them", len(taskIDs))
	if len(taskIDs) == 1 {
		message = "1 non-terminal task has no resource demands; no effort is exported for it"
	}
	message += fmt.Sprintf(" (for example %s)", strings.Join(examples, ", "))
	*warnings = append(*warnings, Diagnostic{
		Code:     "taskjuggler_tasks_without_demands",
		Message:  message,
		Severity: "warning",
		Target:   Target{Kind: "project", ID: ws.Project.ID},
	})
}

func isTerminalTrackingStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "accepted", "cancelled":
		return true
	default:
		return false
	}
}

func appendTaskJugglerUnsupportedDemandDiagnostics(demandIDsByUnit, openDemandIDsByUnit map[string][]string, warnings *[]Diagnostic) []TaskJugglerUnsupportedDemandSet {
	units := make([]string, 0, len(demandIDsByUnit))
	for unit := range demandIDsByUnit {
		units = append(units, unit)
	}
	sort.Strings(units)

	unsupportedDemands := make([]TaskJugglerUnsupportedDemandSet, 0, len(units))
	for _, unit := range units {
		demandIDs := demandIDsByUnit[unit]
		sort.Strings(demandIDs)
		openDemandIDs := openDemandIDsByUnit[unit]
		sort.Strings(openDemandIDs)
		unsupportedDemands = append(unsupportedDemands, TaskJugglerUnsupportedDemandSet{
			Unit:      unit,
			Count:     len(demandIDs),
			OpenCount: len(openDemandIDs),
			DemandIDs: demandIDs,
		})

		if len(openDemandIDs) == 0 {
			continue
		}
		examples := openDemandIDs
		if len(examples) > 3 {
			examples = examples[:3]
		}
		message := fmt.Sprintf("%d resource demands with open work use unit %q, which TaskJuggler effort cannot represent; they are not exported", len(openDemandIDs), unit)
		if len(openDemandIDs) == 1 {
			message = fmt.Sprintf("1 resource demand uses unit %q, which TaskJuggler effort cannot represent; it is not exported", unit)
		}
		if len(examples) > 0 {
			message += fmt.Sprintf(" (for example %s)", strings.Join(examples, ", "))
		}
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_unsupported_demand_unit",
			Message:  message,
			Severity: "warning",
			Target:   Target{Kind: "unit", ID: unit},
		})
	}
	return unsupportedDemands
}

func taskJugglerDemandHasOpenWork(demand domain.ResourceDemand) bool {
	if !domain.DemandLoads(demand.Status) {
		return false
	}
	if demand.HasRemaining() {
		return demand.Remaining() > 0
	}
	return demand.PlannedAmount > 0
}

// taskJugglerLimitsByResource maps weekly hour capacities to weeklymax. The
// export has one limit per resource, so a capacity with effective dates
// contributes only if it is in effect on the status date, with a warning that
// the change over time is not exported.
func taskJugglerLimitsByResource(capacities []domain.Capacity, statusDate string, warnings *[]Diagnostic) map[string]taskJugglerResourceLimit {
	out := map[string]taskJugglerResourceLimit{}
	for _, capacity := range capacities {
		if !isTaskJugglerWeeklyHourCapacity(capacity) || capacity.ResourceRef.ID == "" {
			continue
		}
		amount := capacityOnDate(capacity, statusDate)
		if capacity.EffectiveTo != "" || (capacity.EffectiveFrom != "" && amount == 0) {
			*warnings = append(*warnings, Diagnostic{
				Code: "taskjuggler_capacity_effective_range_simplified",
				Message: fmt.Sprintf("capacity %s is in effect %s..%s; the export uses the weekly limit in effect on the status date %s for the whole plan",
					capacity.ID, orOpen(capacity.EffectiveFrom), orOpen(capacity.EffectiveTo), orOpen(statusDate)),
				Severity: "warning",
				Target:   Target{Kind: "capacity", ID: capacity.ID},
			})
		}
		if amount == 0 {
			continue
		}
		limit := out[capacity.ResourceRef.ID]
		limit.hasWeeklyMax = true
		limit.weeklyMax += amount
		out[capacity.ResourceRef.ID] = limit
	}
	return out
}

func orOpen(date string) string {
	if date == "" {
		return "open"
	}
	return date
}

func mapTaskJugglerDemand(demand domain.ResourceDemand, resourceIDs map[string]struct{}, calibration DemandCalibration, warnings *[]Diagnostic) (taskJugglerDemand, bool) {
	if !domain.DemandLoads(demand.Status) {
		return taskJugglerDemand{}, false
	}
	if !isTaskJugglerEffortUnit(demand.Unit) {
		return taskJugglerDemand{}, false
	}
	if demand.ResourceRef.ID == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_demand_without_concrete_resource",
			Message:  fmt.Sprintf("resource demand %s has no concrete resource_ref; pool and selector allocation is not exported to TaskJuggler yet", demand.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: demand.ID},
		})
		return taskJugglerDemand{}, false
	}
	if _, ok := resourceIDs[demand.ResourceRef.ID]; !ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_unknown_resource",
			Message:  fmt.Sprintf("resource demand %s references unknown resource %s", demand.ID, demand.ResourceRef.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: demand.ID},
		})
		return taskJugglerDemand{}, false
	}
	// An explicit remaining amount of zero means nothing is left to schedule.
	// Only a missing remaining amount falls back to the planned amount.
	if demand.HasRemaining() && demand.Remaining() == 0 {
		return taskJugglerDemand{}, false
	}
	effort := demand.Remaining()
	if !demand.HasRemaining() {
		effort = demand.PlannedAmount
	}
	if effort <= 0 {
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_missing_effort",
			Message:  fmt.Sprintf("resource demand %s has no positive planned or remaining effort", demand.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: demand.ID},
		})
		return taskJugglerDemand{}, false
	}
	rawEffort := effort
	calibrationID := ""
	if calibration.Calibration.ID != "" {
		effort = calibratedLoadAmount(effort, calibration.Factor)
		calibrationID = calibration.Calibration.ID
	}
	if effort <= 0 {
		return taskJugglerDemand{}, false
	}
	return taskJugglerDemand{
		resourceID:    demand.ResourceRef.ID,
		effort:        effort,
		rawEffort:     rawEffort,
		calibrationID: calibrationID,
	}, true
}

func writeTaskJugglerResource(b *strings.Builder, resource domain.Resource, taskJugglerResourceID string, workingHours taskJugglerWorkingHours, vacations []string, limit taskJugglerResourceLimit) {
	fmt.Fprintf(b, "resource %s %q {\n", taskJugglerResourceID, resource.Name)
	if limit.hasWeeklyMax {
		fmt.Fprintf(b, "  limits { weeklymax %.2fh }\n", limit.weeklyMax)
	}
	writeTaskJugglerWorkingHours(b, workingHours)
	for _, date := range vacations {
		fmt.Fprintf(b, "  vacation %s\n", date)
	}
	fmt.Fprintf(b, "}\n")
}

func writeTaskJugglerTask(b *strings.Builder, task domain.Task, taskJugglerTaskID string, taskJugglerResourceIDs map[string]string, predecessorIDs []string, demands []taskJugglerDemand, totalEffort float64, hasLoadingDemands bool, schedule taskJugglerTaskSchedule, statusDate string, warnings *[]Diagnostic) {
	fmt.Fprintf(b, "task %s %q {\n", taskJugglerTaskID, task.Title)
	if task.Description != "" {
		fmt.Fprintf(b, "  note %q\n", task.Description)
	}
	if schedule.finished {
		// Finished work is fixed at its own dates; a dependency on it, or
		// from it on open work, cannot move it.
		fmt.Fprintf(b, "  start %s\n", schedule.start)
		fmt.Fprintf(b, "  end %s\n", schedule.finish)
	} else {
		for _, predecessorID := range predecessorIDs {
			fmt.Fprintf(b, "  depends %s\n", predecessorID)
		}
		if statusDate != "" {
			fmt.Fprintf(b, "  depends %s\n", taskJugglerStatusDateTaskID)
		}
	}
	allocated := map[string]struct{}{}
	for _, demand := range demands {
		if demand.calibrationID != "" {
			fmt.Fprintf(b, "  # calibrated by %s: %.2fh -> %.2fh\n", demand.calibrationID, demand.rawEffort, demand.effort)
		}
		allocated[demand.resourceID] = struct{}{}
	}
	if totalEffort > 0 {
		if totalEffort < 1 {
			fmt.Fprintf(b, "  effort %dmin\n", taskJugglerEffortMinutes(totalEffort))
		} else {
			fmt.Fprintf(b, "  effort %.2fh\n", totalEffort)
		}
	}
	resourceIDs := make([]string, 0, len(allocated))
	for resourceID := range allocated {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs)
	for _, resourceID := range resourceIDs {
		fmt.Fprintf(b, "  allocate %s\n", taskJugglerResourceIDs[resourceID])
	}
	if len(demands) == 0 && hasLoadingDemands && !schedule.finished && !isTerminalTrackingStatus(task.TrackingStatus) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_task_without_supported_demands",
			Message:  fmt.Sprintf("task %s has no supported concrete hour-based resource demands for TaskJuggler export", task.ID),
			Severity: "warning",
			Target:   Target{Kind: "task", ID: task.ID},
		})
	}
	fmt.Fprintf(b, "}\n")
}

func writeTaskJugglerParseableTaskReport(b *strings.Builder, hideInternal bool) {
	fmt.Fprintf(b, "\ntaskreport replan_task_schedule \"reports/replan-task-schedule\" {\n")
	fmt.Fprintf(b, "  formats csv\n")
	fmt.Fprintf(b, "  columns id, name, start, end, effort, effortleft, resources, status\n")
	fmt.Fprintf(b, "  sorttasks seqno.up\n")
	if hideInternal {
		fmt.Fprintf(b, "  hidetask %s\n", taskJugglerInternalFlag)
	}
	fmt.Fprintf(b, "}\n")
}

func isTaskJugglerEffortUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "hour", "hours", "h", "agent_hours":
		return true
	default:
		return false
	}
}

func taskJugglerVacationsByResource(availabilities []domain.Availability, projectID string, warnings *[]Diagnostic) map[string][]string {
	out := map[string][]string{}
	seen := map[string]map[string]struct{}{}
	notExportedCounts := map[string]int{}
	notExportedAvailabilityIDs := map[string]struct{}{}
	for _, availability := range availabilities {
		resourceID := availability.ResourceRef.ID
		if resourceID == "" {
			continue
		}
		for _, exception := range availability.Exceptions {
			exceptionType := strings.ToLower(strings.TrimSpace(exception.Type))
			switch exceptionType {
			case "reduced_capacity", "extra_capacity", "quota_reset":
				notExportedCounts[exceptionType]++
				notExportedAvailabilityIDs[availability.ID] = struct{}{}
				continue
			case "unavailable", "access_closed", "maintenance", "manual_hold":
			default:
				continue
			}
			if exception.Date == "" {
				continue
			}
			if seen[resourceID] == nil {
				seen[resourceID] = map[string]struct{}{}
			}
			if _, ok := seen[resourceID][exception.Date]; ok {
				continue
			}
			seen[resourceID][exception.Date] = struct{}{}
			out[resourceID] = append(out[resourceID], exception.Date)
		}
	}
	for resourceID := range out {
		sort.Strings(out[resourceID])
	}
	appendTaskJugglerExceptionsNotExportedDiagnostic(projectID, notExportedCounts, notExportedAvailabilityIDs, warnings)
	return out
}

func taskJugglerWorkingHoursByResource(availabilities []domain.Availability, resolution int, projectTimezone, projectID string, warnings *[]Diagnostic) map[string]taskJugglerWorkingHours {
	dayIndexes := map[string]int{
		"monday": 0, "tuesday": 1, "wednesday": 2, "thursday": 3,
		"friday": 4, "saturday": 5, "sunday": 6,
	}
	out := map[string]taskJugglerWorkingHours{}
	roundedIDs := map[string]struct{}{}
	timezoneIDs := map[string]struct{}{}
	if strings.TrimSpace(projectTimezone) == "" {
		projectTimezone = "UTC"
	}
	for _, availability := range availabilities {
		if availability.ResourceRef.ID == "" {
			continue
		}
		if timezone := strings.TrimSpace(availability.Timezone); len(availability.Windows) > 0 && timezone != "" && timezone != projectTimezone {
			timezoneIDs[availability.ID] = struct{}{}
		}
		for _, window := range availability.Windows {
			start, startOK := taskJugglerTimeMinutes(window.StartTime)
			end, endOK := taskJugglerTimeMinutes(window.EndTime)
			if !startOK || !endOK {
				continue
			}
			originalStart, originalEnd := start, end
			start = start / resolution * resolution
			endOfDay := strings.TrimSpace(window.EndTime) == "23:59"
			if endOfDay {
				end = 24 * 60
			} else if end%resolution != 0 {
				end = (end/resolution + 1) * resolution
			}
			if end > 24*60 {
				end = 24 * 60
			}
			if start != originalStart || (!endOfDay && end != originalEnd) {
				roundedIDs[availability.ID] = struct{}{}
			}
			if start >= end {
				continue
			}
			hours := out[availability.ResourceRef.ID]
			if hours.byDay == nil {
				hours.byDay = map[int][]taskJugglerTimeRange{}
			}
			for _, day := range window.Days {
				dayIndex, ok := dayIndexes[strings.ToLower(strings.TrimSpace(day))]
				if !ok {
					continue
				}
				hours.hasWindows = true
				hours.byDay[dayIndex] = append(hours.byDay[dayIndex], taskJugglerTimeRange{start: start, end: end})
			}
			out[availability.ResourceRef.ID] = hours
		}
	}
	for resourceID, hours := range out {
		for day, ranges := range hours.byDay {
			hours.byDay[day] = mergeTaskJugglerTimeRanges(ranges)
		}
		out[resourceID] = hours
	}
	appendTaskJugglerAvailabilityIDDiagnostic("taskjuggler_working_hours_rounded", "availability working hours were rounded to the export timing resolution", projectID, roundedIDs, warnings)
	appendTaskJugglerAvailabilityIDDiagnostic("taskjuggler_availability_timezone_differs", "availability timezone differs from the TaskJuggler project timezone; working hours are exported as written without conversion", projectID, timezoneIDs, warnings)
	return out
}

func taskJugglerTimeMinutes(value string) (int, bool) {
	parsed, err := time.Parse("15:04", strings.TrimSpace(value))
	if err != nil {
		return 0, false
	}
	return parsed.Hour()*60 + parsed.Minute(), true
}

func mergeTaskJugglerTimeRanges(ranges []taskJugglerTimeRange) []taskJugglerTimeRange {
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			return ranges[i].end < ranges[j].end
		}
		return ranges[i].start < ranges[j].start
	})
	merged := make([]taskJugglerTimeRange, 0, len(ranges))
	for _, current := range ranges {
		if len(merged) == 0 || current.start > merged[len(merged)-1].end {
			merged = append(merged, current)
			continue
		}
		if current.end > merged[len(merged)-1].end {
			merged[len(merged)-1].end = current.end
		}
	}
	return merged
}

func writeTaskJugglerWorkingHours(b *strings.Builder, hours taskJugglerWorkingHours) {
	if !hours.hasWindows {
		return
	}
	dayNames := []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	groups := map[string][]string{}
	order := []string{}
	for day := 0; day < len(dayNames); day++ {
		key := taskJugglerRangeList(hours.byDay[day])
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], dayNames[day])
	}
	for _, ranges := range order {
		days := strings.Join(groups[ranges], ", ")
		if ranges == "" {
			fmt.Fprintf(b, "  workinghours %s off\n", days)
		} else {
			fmt.Fprintf(b, "  workinghours %s %s\n", days, ranges)
		}
	}
}

func taskJugglerRangeList(ranges []taskJugglerTimeRange) string {
	parts := make([]string, 0, len(ranges))
	for _, timeRange := range ranges {
		parts = append(parts, taskJugglerClock(timeRange.start)+" - "+taskJugglerClock(timeRange.end))
	}
	return strings.Join(parts, ", ")
}

func taskJugglerClock(minutes int) string {
	return fmt.Sprintf("%d:%02d", minutes/60, minutes%60)
}

func appendTaskJugglerAvailabilityIDDiagnostic(code, description, projectID string, ids map[string]struct{}, warnings *[]Diagnostic) {
	if len(ids) == 0 {
		return
	}
	sorted := make([]string, 0, len(ids))
	for id := range ids {
		sorted = append(sorted, id)
	}
	sort.Strings(sorted)
	examples := sorted
	if len(examples) > 3 {
		examples = examples[:3]
	}
	records := "availability records"
	if len(sorted) == 1 {
		records = "availability record"
	}
	*warnings = append(*warnings, Diagnostic{
		Code:     code,
		Message:  fmt.Sprintf("%d %s: %s (for example %s)", len(sorted), records, description, strings.Join(examples, ", ")),
		Severity: "warning",
		Target:   Target{Kind: "project", ID: projectID},
	})
}

func appendTaskJugglerExceptionsNotExportedDiagnostic(projectID string, counts map[string]int, availabilityIDs map[string]struct{}, warnings *[]Diagnostic) {
	total := 0
	parts := []string{}
	for _, exceptionType := range []string{"reduced_capacity", "extra_capacity", "quota_reset"} {
		if count := counts[exceptionType]; count > 0 {
			total += count
			parts = append(parts, fmt.Sprintf("%s: %d", exceptionType, count))
		}
	}
	if total == 0 {
		return
	}
	ids := make([]string, 0, len(availabilityIDs))
	for id := range availabilityIDs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	if len(ids) > 3 {
		ids = ids[:3]
	}
	message := fmt.Sprintf("%d availability exceptions cannot be represented in TaskJuggler and were not exported (%s; for example %s)", total, strings.Join(parts, ", "), strings.Join(ids, ", "))
	if total == 1 {
		message = fmt.Sprintf("1 availability exception cannot be represented in TaskJuggler and was not exported (%s; for example %s)", strings.Join(parts, ", "), strings.Join(ids, ", "))
	}
	*warnings = append(*warnings, Diagnostic{
		Code:     "taskjuggler_availability_exceptions_not_exported",
		Message:  message,
		Severity: "warning",
		Target:   Target{Kind: "project", ID: projectID},
	})
}

func taskJugglerID(id, fallback string) string {
	var b strings.Builder
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('_')
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		out = fallback
	}
	first, _ := utf8FirstRune(out)
	if unicode.IsDigit(first) {
		out = fallback + "_" + out
	}
	return out
}

func utf8FirstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}
