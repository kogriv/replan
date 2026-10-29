package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/kogriv/replan/internal/domain"
)

type ExportTaskJugglerRequest struct {
	ScenarioID                   string
	UnrepresentedAssumptionsMode string
	Now                          time.Time
}

type ExportTaskJugglerResult struct {
	Metadata ExportMetadata           `json:"metadata"`
	Scenario string                   `json:"scenario,omitempty"`
	Content  string                   `json:"content,omitempty"`
	Mapping  TaskJugglerExportMapping `json:"mapping"`
	Warnings []Diagnostic             `json:"warnings"`
	Errors   []Diagnostic             `json:"errors"`
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
	resourceID string
	effort     float64
}

type taskJugglerResourceLimit struct {
	hasWeeklyMax bool
	weeklyMax    float64
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
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
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
	result.Content = buildTaskJugglerContent(ws, result.Metadata, result.Scenario, result.Mapping, &result.Warnings)
	return result, nil
}

func buildTaskJugglerExportMapping(ws domain.Workspace, projectPath, scenarioID string, errors *[]Diagnostic) TaskJugglerExportMapping {
	mapping := TaskJugglerExportMapping{
		Version:       1,
		ProjectPath:   projectPath,
		ScenarioID:    scenarioID,
		TaskIDMap:     []TaskJugglerTaskIDMapping{},
		ResourceIDMap: []TaskJugglerResourceIDMapping{},
	}
	taskJugglerTaskIDs := map[string]string{}
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

func buildTaskJugglerContent(ws domain.Workspace, metadata ExportMetadata, scenarioID string, mapping TaskJugglerExportMapping, warnings *[]Diagnostic) string {
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
	vacationsByResource := taskJugglerVacationsByResource(ws.Availabilities)
	limitsByResource := taskJugglerLimitsByResource(ws.Capacities, ws.Project.DefaultStatusDate, warnings)
	demandsByTask := make(map[string][]taskJugglerDemand)
	for _, demand := range ws.ResourceDemands {
		mapped, ok := mapTaskJugglerDemand(demand, resourceIDs, warnings)
		if ok {
			demandsByTask[demand.TaskRef.ID] = append(demandsByTask[demand.TaskRef.ID], mapped)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Generated by RePlan\n")
	fmt.Fprintf(&b, "# Export ID: %s\n", metadata.ID)
	fmt.Fprintf(&b, "# Exported at: %s\n", metadata.ExportedAt)
	fmt.Fprintf(&b, "# Project path: %s\n\n", metadata.ProjectPath)
	if scenarioID != "" {
		fmt.Fprintf(&b, "# Scenario: %s\n\n", scenarioID)
	}

	startDate := ws.Project.DefaultStatusDate
	if startDate == "" {
		startDate = "today"
	}
	timezone := ws.Project.Timezone
	if timezone == "" {
		timezone = "UTC"
	}
	fmt.Fprintf(&b, "project %s %q \"1.0\" %s +1y {\n", taskJugglerID(ws.Project.ID, "project_replan"), ws.Project.Name, startDate)
	fmt.Fprintf(&b, "  timezone %q\n", timezone)
	fmt.Fprintf(&b, "}\n\n")

	for _, resource := range ws.Resources {
		writeTaskJugglerResource(&b, resource, taskJugglerResourceIDs[resource.ID], vacationsByResource[resource.ID], limitsByResource[resource.ID])
	}
	if len(ws.Resources) > 0 {
		b.WriteString("\n")
	}

	for _, task := range ws.Tasks {
		writeTaskJugglerTask(&b, task, taskJugglerTaskIDs[task.ID], taskJugglerResourceIDs, demandsByTask[task.ID], warnings)
	}
	writeTaskJugglerParseableTaskReport(&b)

	return b.String()
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

func mapTaskJugglerDemand(demand domain.ResourceDemand, resourceIDs map[string]struct{}, warnings *[]Diagnostic) (taskJugglerDemand, bool) {
	if !isTaskJugglerEffortUnit(demand.Unit) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_unsupported_demand_unit",
			Message:  fmt.Sprintf("resource demand %s uses unit %q, which cannot be represented as TaskJuggler effort in this export slice", demand.ID, demand.Unit),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: demand.ID},
		})
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
	return taskJugglerDemand{
		resourceID: demand.ResourceRef.ID,
		effort:     effort,
	}, true
}

func writeTaskJugglerResource(b *strings.Builder, resource domain.Resource, taskJugglerResourceID string, vacations []string, limit taskJugglerResourceLimit) {
	fmt.Fprintf(b, "resource %s %q {\n", taskJugglerResourceID, resource.Name)
	if limit.hasWeeklyMax {
		fmt.Fprintf(b, "  limits { weeklymax %.2fh }\n", limit.weeklyMax)
	}
	for _, date := range vacations {
		fmt.Fprintf(b, "  vacation %s\n", date)
	}
	fmt.Fprintf(b, "}\n")
}

func writeTaskJugglerTask(b *strings.Builder, task domain.Task, taskJugglerTaskID string, taskJugglerResourceIDs map[string]string, demands []taskJugglerDemand, warnings *[]Diagnostic) {
	fmt.Fprintf(b, "task %s %q {\n", taskJugglerTaskID, task.Title)
	if task.Description != "" {
		fmt.Fprintf(b, "  note %q\n", task.Description)
	}
	totalEffort := 0.0
	allocated := map[string]struct{}{}
	for _, demand := range demands {
		totalEffort += demand.effort
		allocated[demand.resourceID] = struct{}{}
	}
	if totalEffort > 0 {
		fmt.Fprintf(b, "  effort %.2fh\n", totalEffort)
	}
	resourceIDs := make([]string, 0, len(allocated))
	for resourceID := range allocated {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs)
	for _, resourceID := range resourceIDs {
		fmt.Fprintf(b, "  allocate %s\n", taskJugglerResourceIDs[resourceID])
	}
	if len(demands) == 0 {
		*warnings = append(*warnings, Diagnostic{
			Code:     "taskjuggler_task_without_supported_demands",
			Message:  fmt.Sprintf("task %s has no supported concrete hour-based resource demands for TaskJuggler export", task.ID),
			Severity: "warning",
			Target:   Target{Kind: "task", ID: task.ID},
		})
	}
	fmt.Fprintf(b, "}\n")
}

func writeTaskJugglerParseableTaskReport(b *strings.Builder) {
	fmt.Fprintf(b, "\ntaskreport replan_task_schedule \"reports/replan-task-schedule\" {\n")
	fmt.Fprintf(b, "  formats csv\n")
	fmt.Fprintf(b, "  columns id, name, start, end, effort, effortleft, resources, status\n")
	fmt.Fprintf(b, "  sorttasks seqno.up\n")
	fmt.Fprintf(b, "}\n")
}

func isTaskJugglerEffortUnit(unit string) bool {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "hour", "hours", "h":
		return true
	default:
		return false
	}
}

func taskJugglerVacationsByResource(availabilities []domain.Availability) map[string][]string {
	out := map[string][]string{}
	seen := map[string]map[string]struct{}{}
	for _, availability := range availabilities {
		resourceID := availability.ResourceRef.ID
		if resourceID == "" {
			continue
		}
		for _, exception := range availability.Exceptions {
			if strings.ToLower(exception.Type) != "unavailable" || exception.Date == "" {
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
	return out
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
