package app

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kogriv/replan/internal/domain"
)

type ScenarioOverlayResult struct {
	Workspace domain.Workspace
	Scenario  domain.Scenario
	Warnings  []Diagnostic
	Errors    []Diagnostic
}

func ApplyScenarioOverlay(ws domain.Workspace, scenarioID string) ScenarioOverlayResult {
	result := ScenarioOverlayResult{
		Workspace: cloneWorkspace(ws),
		Warnings:  []Diagnostic{},
		Errors:    []Diagnostic{},
	}
	scenario, ok := findScenario(ws.Scenarios, scenarioID)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "scenario_not_found",
			Message:  fmt.Sprintf("scenario %s not found", scenarioID),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenarioID},
		})
		return result
	}
	result.Scenario = scenario
	for _, change := range scenario.Changes {
		if errDiag, ok := applyScenarioChange(&result.Workspace, scenario, change); !ok {
			result.Errors = append(result.Errors, errDiag)
		}
	}
	if len(result.Errors) == 0 {
		changes := buildScenarioApplyChangesForTaskDates(ws, scenario)
		result.Errors = append(result.Errors, scenarioTaskDateRangeDiagnostics(ws, changes)...)
	}
	return result
}

func applyScenarioChange(ws *domain.Workspace, scenario domain.Scenario, change domain.ScenarioChange) (Diagnostic, bool) {
	if change.Op == "add" && change.Path == "availability.exceptions" {
		return applyScenarioAvailabilityException(ws, scenario, change)
	}
	if isSetOperation(change.Op) && strings.HasPrefix(change.Path, "resource_demands.") && strings.HasSuffix(change.Path, ".remaining_amount") {
		return applyScenarioDemandAmount(ws, scenario, change, "remaining_amount")
	}
	if isSetOperation(change.Op) && strings.HasPrefix(change.Path, "resource_demands.") && strings.HasSuffix(change.Path, ".planned_amount") {
		return applyScenarioDemandAmount(ws, scenario, change, "planned_amount")
	}
	if isSetOperation(change.Op) && strings.HasPrefix(change.Path, "resource_demands.") && strings.HasSuffix(change.Path, ".resource_ref") {
		return applyScenarioDemandResourceRef(ws, scenario, change)
	}
	if isSetOperation(change.Op) && strings.HasPrefix(change.Path, "capacities.") && strings.HasSuffix(change.Path, ".amount") {
		return applyScenarioCapacityAmount(ws, scenario, change)
	}
	if isSetOperation(change.Op) && strings.HasPrefix(change.Path, "tasks.") && (strings.HasSuffix(change.Path, ".planned_start") || strings.HasSuffix(change.Path, ".planned_finish")) {
		applyChange, ok := scenarioApplyTaskDateChange(*ws, scenario, change)
		if !ok {
			return unsupportedScenarioApplyChange(scenario, change), false
		}
		for i := range ws.Tasks {
			if ws.Tasks[i].ID != applyChange.TargetID {
				continue
			}
			value := applyChange.NewValue.(string)
			if applyChange.Field == "planned_start" {
				ws.Tasks[i].PlannedStart = value
			} else {
				ws.Tasks[i].PlannedFinish = value
			}
			return Diagnostic{}, true
		}
	}
	return Diagnostic{
		Code:     "unsupported_scenario_change",
		Message:  fmt.Sprintf("scenario %s change %s %s is not supported by the in-memory overlay yet", scenario.ID, change.Op, change.Path),
		Severity: "error",
		Target:   Target{Kind: "scenario", ID: scenario.ID},
	}, false
}

func buildScenarioApplyChangesForTaskDates(ws domain.Workspace, scenario domain.Scenario) []ScenarioApplyChange {
	var changes []ScenarioApplyChange
	for _, change := range scenario.Changes {
		if applyChange, ok := scenarioApplyTaskDateChange(ws, scenario, change); ok {
			changes = append(changes, applyChange)
		}
	}
	return changes
}

func unsupportedScenarioApplyChange(scenario domain.Scenario, change domain.ScenarioChange) Diagnostic {
	return Diagnostic{
		Code:     "unsupported_scenario_apply_change",
		Message:  fmt.Sprintf("scenario %s change %s %s cannot be applied by this slice", scenario.ID, change.Op, change.Path),
		Severity: "error",
		Target:   Target{Kind: "scenario", ID: scenario.ID},
	}
}

func applyScenarioAvailabilityException(ws *domain.Workspace, scenario domain.Scenario, change domain.ScenarioChange) (Diagnostic, bool) {
	resourceID, exception, ok := scenarioAvailabilityException(change.Value)
	if !ok {
		return Diagnostic{
			Code:     "invalid_scenario_change_value",
			Message:  fmt.Sprintf("scenario %s availability exception change has invalid value shape", scenario.ID),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		}, false
	}
	if !workspaceHasResource(*ws, resourceID) {
		return Diagnostic{
			Code:     "scenario_unknown_resource",
			Message:  fmt.Sprintf("scenario %s references unknown resource %s", scenario.ID, resourceID),
			Severity: "error",
			Target:   Target{Kind: "resource", ID: resourceID},
		}, false
	}
	addAvailabilityException(ws, resourceID, exception)
	return Diagnostic{}, true
}

func applyScenarioDemandAmount(ws *domain.Workspace, scenario domain.Scenario, change domain.ScenarioChange, field string) (Diagnostic, bool) {
	demandID := strings.TrimSuffix(strings.TrimPrefix(change.Path, "resource_demands."), "."+field)
	if demandID == "" {
		return Diagnostic{
			Code:     "invalid_scenario_change_path",
			Message:  fmt.Sprintf("scenario %s %s change has invalid path %s", scenario.ID, field, change.Path),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		}, false
	}
	amount, ok := scenarioNumericAmount(change.Value)
	if !ok || amount < 0 {
		return Diagnostic{
			Code:     "invalid_scenario_change_value",
			Message:  fmt.Sprintf("scenario %s %s change for %s must be a non-negative number", scenario.ID, field, demandID),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: demandID},
		}, false
	}
	for i := range ws.ResourceDemands {
		if ws.ResourceDemands[i].ID != demandID {
			continue
		}
		switch field {
		case "remaining_amount":
			ws.ResourceDemands[i].RemainingAmount = domain.Amount(amount)
		case "planned_amount":
			ws.ResourceDemands[i].PlannedAmount = amount
		}
		return Diagnostic{}, true
	}
	return Diagnostic{
		Code:     "scenario_unknown_resource_demand",
		Message:  fmt.Sprintf("scenario %s references unknown resource demand %s", scenario.ID, demandID),
		Severity: "error",
		Target:   Target{Kind: "resource_demand", ID: demandID},
	}, false
}

func applyScenarioDemandResourceRef(ws *domain.Workspace, scenario domain.Scenario, change domain.ScenarioChange) (Diagnostic, bool) {
	demandID := strings.TrimSuffix(strings.TrimPrefix(change.Path, "resource_demands."), ".resource_ref")
	if demandID == "" {
		return Diagnostic{
			Code:     "invalid_scenario_change_path",
			Message:  fmt.Sprintf("scenario %s resource_ref change has invalid path %s", scenario.ID, change.Path),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		}, false
	}
	ref, ok := scenarioResourceRef(change.Value)
	if !ok {
		return Diagnostic{
			Code:     "invalid_scenario_change_value",
			Message:  fmt.Sprintf("scenario %s resource_ref change for %s must contain a resource id", scenario.ID, demandID),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: demandID},
		}, false
	}
	if !workspaceHasResource(*ws, ref.ID) {
		return Diagnostic{
			Code:     "scenario_unknown_resource",
			Message:  fmt.Sprintf("scenario %s references unknown resource %s", scenario.ID, ref.ID),
			Severity: "error",
			Target:   Target{Kind: "resource", ID: ref.ID},
		}, false
	}
	for i := range ws.ResourceDemands {
		if ws.ResourceDemands[i].ID != demandID {
			continue
		}
		ws.ResourceDemands[i].ResourceRef = ref
		ws.ResourceDemands[i].ResourcePoolRef = domain.Ref{}
		return Diagnostic{}, true
	}
	return Diagnostic{
		Code:     "scenario_unknown_resource_demand",
		Message:  fmt.Sprintf("scenario %s references unknown resource demand %s", scenario.ID, demandID),
		Severity: "error",
		Target:   Target{Kind: "resource_demand", ID: demandID},
	}, false
}

func applyScenarioCapacityAmount(ws *domain.Workspace, scenario domain.Scenario, change domain.ScenarioChange) (Diagnostic, bool) {
	capacityID := strings.TrimSuffix(strings.TrimPrefix(change.Path, "capacities."), ".amount")
	if capacityID == "" {
		return Diagnostic{
			Code:     "invalid_scenario_change_path",
			Message:  fmt.Sprintf("scenario %s capacity amount change has invalid path %s", scenario.ID, change.Path),
			Severity: "error",
			Target:   Target{Kind: "scenario", ID: scenario.ID},
		}, false
	}
	amount, ok := scenarioNumericAmount(change.Value)
	if !ok || amount < 0 {
		return Diagnostic{
			Code:     "invalid_scenario_change_value",
			Message:  fmt.Sprintf("scenario %s capacity amount change for %s must be a non-negative number", scenario.ID, capacityID),
			Severity: "error",
			Target:   Target{Kind: "capacity", ID: capacityID},
		}, false
	}
	for i := range ws.Capacities {
		if ws.Capacities[i].ID != capacityID {
			continue
		}
		ws.Capacities[i].Amount = amount
		return Diagnostic{}, true
	}
	return Diagnostic{
		Code:     "scenario_unknown_capacity",
		Message:  fmt.Sprintf("scenario %s references unknown capacity %s", scenario.ID, capacityID),
		Severity: "error",
		Target:   Target{Kind: "capacity", ID: capacityID},
	}, false
}

func scenarioAvailabilityException(value any) (string, domain.AvailabilityException, bool) {
	valueMap, ok := value.(map[string]any)
	if !ok {
		return "", domain.AvailabilityException{}, false
	}
	resourceRef, ok := valueMap["resource_ref"].(map[string]any)
	if !ok {
		return "", domain.AvailabilityException{}, false
	}
	resourceID, ok := resourceRef["id"].(string)
	if !ok || resourceID == "" {
		return "", domain.AvailabilityException{}, false
	}
	date, ok := valueMap["date"].(string)
	if !ok || date == "" {
		return "", domain.AvailabilityException{}, false
	}
	exceptionType, ok := valueMap["type"].(string)
	if !ok || exceptionType == "" {
		return "", domain.AvailabilityException{}, false
	}
	reason, _ := valueMap["reason"].(string)
	return resourceID, domain.AvailabilityException{
		Date:   date,
		Type:   exceptionType,
		Reason: reason,
	}, true
}

func scenarioResourceRef(value any) (domain.Ref, bool) {
	valueMap, ok := value.(map[string]any)
	if !ok {
		return domain.Ref{}, false
	}
	id, ok := valueMap["id"].(string)
	if !ok || id == "" {
		return domain.Ref{}, false
	}
	kind, _ := valueMap["kind"].(string)
	if kind == "" {
		kind = "resource"
	}
	if kind != "resource" {
		return domain.Ref{}, false
	}
	return domain.Ref{Kind: "resource", ID: id}, true
}

func addAvailabilityException(ws *domain.Workspace, resourceID string, exception domain.AvailabilityException) {
	for i := range ws.Availabilities {
		if ws.Availabilities[i].ResourceRef.ID != resourceID {
			continue
		}
		ws.Availabilities[i].Exceptions = append(ws.Availabilities[i].Exceptions, exception)
		return
	}
	ws.Availabilities = append(ws.Availabilities, domain.Availability{
		ID:          "availability." + slugID(resourceID) + ".scenario-overlay",
		ResourceRef: domain.Ref{Kind: "resource", ID: resourceID},
		Timezone:    ws.Project.Timezone,
		Exceptions:  []domain.AvailabilityException{exception},
	})
}

func isSetOperation(op string) bool {
	return op == "set" || op == "replace"
}

func scenarioNumericAmount(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case string:
		parsed, err := strconv.ParseFloat(v, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

func workspaceHasResource(ws domain.Workspace, resourceID string) bool {
	for _, resource := range ws.Resources {
		if resource.ID == resourceID {
			return true
		}
	}
	return false
}

func cloneWorkspace(ws domain.Workspace) domain.Workspace {
	cloned := ws
	cloned.Actors = append([]domain.Actor(nil), ws.Actors...)
	cloned.Resources = append([]domain.Resource(nil), ws.Resources...)
	cloned.ResourcePools = append([]domain.ResourcePool(nil), ws.ResourcePools...)
	cloned.Tasks = append([]domain.Task(nil), ws.Tasks...)
	cloned.ResourceDemands = append([]domain.ResourceDemand(nil), ws.ResourceDemands...)
	cloned.Capacities = append([]domain.Capacity(nil), ws.Capacities...)
	cloned.Availabilities = cloneAvailabilities(ws.Availabilities)
	cloned.Baselines = append([]domain.Baseline(nil), ws.Baselines...)
	cloned.Actuals = append([]domain.Actual(nil), ws.Actuals...)
	cloned.Evidence = append([]domain.Evidence(nil), ws.Evidence...)
	cloned.Proposals = append([]domain.Proposal(nil), ws.Proposals...)
	cloned.Scenarios = append([]domain.Scenario(nil), ws.Scenarios...)
	cloned.AuditRecords = append([]domain.AuditRecord(nil), ws.AuditRecords...)
	cloned.CheckedFiles = append([]string(nil), ws.CheckedFiles...)
	return cloned
}

func cloneAvailabilities(availabilities []domain.Availability) []domain.Availability {
	cloned := make([]domain.Availability, len(availabilities))
	for i, availability := range availabilities {
		cloned[i] = availability
		cloned[i].Windows = append([]domain.AvailabilityWindow(nil), availability.Windows...)
		cloned[i].Exceptions = append([]domain.AvailabilityException(nil), availability.Exceptions...)
	}
	return cloned
}
