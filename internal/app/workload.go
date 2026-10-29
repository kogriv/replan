package app

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

type WorkloadService struct {
	loader WorkspaceLoader
}

type WorkloadRequest struct {
	ProjectPath string
	Week        string
	ResourceID  string
	// ResourceClass and Unit keep only rows of that resource class or unit;
	// they filter the result, not the calculation.
	ResourceClass string
	Unit          string
	ScenarioID    string
	// WithDemands lists in every row the demands that make it up.
	WithDemands bool
}

type WorkloadResult struct {
	ProjectPath string             `json:"project_path"`
	Week        string             `json:"week,omitempty"`
	ScenarioID  string             `json:"scenario_id,omitempty"`
	Resources   []ResourceWorkload `json:"resources"`
	Warnings    []Diagnostic       `json:"warnings"`
	Errors      []Diagnostic       `json:"errors"`
}

type ResourceWorkload struct {
	ResourceID    string  `json:"resource_id"`
	ResourceName  string  `json:"resource_name,omitempty"`
	ResourceClass string  `json:"resource_class,omitempty"`
	Unit          string  `json:"unit"`
	Capacity      float64 `json:"capacity"`
	Planned       float64 `json:"planned"`
	Reserved      float64 `json:"reserved"`
	Actual        float64 `json:"actual"`
	Remaining     float64 `json:"remaining"`
	Overload      float64 `json:"overload"`
	// Demands is filled only on request (WorkloadRequest.WithDemands).
	Demands []WorkloadDemandShare `json:"demands,omitempty"`
}

// WorkloadDemandShare is one demand's part of a workload row. Share is the
// fraction of the demand that falls in the requested week (1 without a week).
// A demand on a concurrent resource is not prorated: its amounts are its own,
// and the row holds the peak of the demands active on the same day.
type WorkloadDemandShare struct {
	ResourceDemandID string  `json:"resource_demand_id"`
	TaskID           string  `json:"task_id,omitempty"`
	Status           string  `json:"status,omitempty"`
	Concurrent       bool    `json:"concurrent,omitempty"`
	Share            float64 `json:"share"`
	Planned          float64 `json:"planned"`
	Reserved         float64 `json:"reserved"`
	Remaining        float64 `json:"remaining"`
}

func NewWorkloadService(loader WorkspaceLoader) WorkloadService {
	return WorkloadService{loader: loader}
}

func (s WorkloadService) Calculate(ctx context.Context, req WorkloadRequest) (WorkloadResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := WorkloadResult{
		ProjectPath: s.loader.ProjectPath(),
		Week:        req.Week,
		ScenarioID:  req.ScenarioID,
		Resources:   []ResourceWorkload{},
		Warnings:    []Diagnostic{},
		Errors:      []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	state := newWorkloadCalculation(req, ws, &result)
	if err := state.applyScenario(); err != nil {
		return result, err
	}
	state.indexWorkspace()
	state.addCapacities()
	state.addActuals()
	state.addDemands()
	state.finishRows()
	return result, nil
}

type workloadCalculation struct {
	req                    WorkloadRequest
	workspace              domain.Workspace
	result                 *WorkloadResult
	resources              map[string]domain.Resource
	pools                  map[string]domain.ResourcePool
	tasks                  map[string]domain.Task
	days                   map[string]map[time.Weekday]bool
	capacityByResourceUnit map[string]float64
	actualDemandIDs        map[string]bool
	rows                   map[string]*ResourceWorkload
	concurrent             map[string][]concurrentUse
	weekStart              time.Time
	weekEnd                time.Time
	hasWeekFilter          bool
}

func newWorkloadCalculation(req WorkloadRequest, ws domain.Workspace, result *WorkloadResult) *workloadCalculation {
	start, end, filtered := workloadWeekRange(req.Week)
	return &workloadCalculation{
		req: req, workspace: ws, result: result, resources: map[string]domain.Resource{}, pools: map[string]domain.ResourcePool{},
		tasks: map[string]domain.Task{}, capacityByResourceUnit: map[string]float64{}, actualDemandIDs: map[string]bool{},
		rows: map[string]*ResourceWorkload{}, concurrent: map[string][]concurrentUse{}, weekStart: start, weekEnd: end, hasWeekFilter: filtered,
	}
}

func (c *workloadCalculation) applyScenario() error {
	if c.req.ScenarioID == "" {
		return nil
	}
	overlay := ApplyScenarioOverlay(c.workspace, c.req.ScenarioID)
	c.result.Warnings = append(c.result.Warnings, overlay.Warnings...)
	c.result.Errors = append(c.result.Errors, overlay.Errors...)
	if len(c.result.Errors) > 0 {
		return ErrValidationFailed
	}
	c.workspace = overlay.Workspace
	c.result.ScenarioID = overlay.Scenario.ID
	return nil
}

func (c *workloadCalculation) indexWorkspace() {
	for _, resource := range c.workspace.Resources {
		c.resources[resource.ID] = resource
	}
	for _, pool := range c.workspace.ResourcePools {
		c.pools[pool.ID] = pool
	}
	for _, task := range c.workspace.Tasks {
		c.tasks[task.ID] = task
	}
	c.days = workingDays(c.workspace.Availabilities)
	for _, capacity := range c.workspace.Capacities {
		key := capacity.ResourceRef.ID + "\x00" + capacity.Unit
		c.capacityByResourceUnit[key] += c.capacityAmount(capacity)
	}
}

func (c workloadCalculation) capacityAmount(capacity domain.Capacity) float64 {
	if c.hasWeekFilter {
		return capacityInWeek(capacity, c.weekStart, c.weekEnd, c.days[capacity.ResourceRef.ID])
	}
	return capacityOnDate(capacity, c.workspace.Project.DefaultStatusDate)
}

func (c *workloadCalculation) rowFor(resourceID, unit string) *ResourceWorkload {
	key := resourceID + "\x00" + unit
	if row, ok := c.rows[key]; ok {
		return row
	}
	resource := c.resources[resourceID]
	row := &ResourceWorkload{ResourceID: resourceID, ResourceName: resource.Name, ResourceClass: resource.Class, Unit: unit}
	if pool, ok := c.pools[resourceID]; ok {
		row.ResourceName, row.ResourceClass = pool.Name, pool.ResourceClass
		for _, member := range pool.Members {
			row.Capacity += c.capacityByResourceUnit[member.ID+"\x00"+unit]
		}
	}
	c.rows[key] = row
	return row
}

func (c *workloadCalculation) addCapacities() {
	for _, capacity := range c.workspace.Capacities {
		if c.req.ResourceID == "" || capacity.ResourceRef.ID == c.req.ResourceID {
			c.rowFor(capacity.ResourceRef.ID, capacity.Unit).Capacity += c.capacityAmount(capacity)
		}
	}
}

func (c *workloadCalculation) addActuals() {
	for _, actual := range c.workspace.Actuals {
		if actual.Type != "work_consumed" || (c.hasWeekFilter && !actualIntersectsWeek(actual, c.weekStart, c.weekEnd)) || actual.ResourceRef.ID == "" || actual.Unit == "" {
			continue
		}
		if c.req.ResourceID != "" && actual.ResourceRef.ID != c.req.ResourceID {
			continue
		}
		c.rowFor(actual.ResourceRef.ID, actual.Unit).Actual += actual.Amount
		if actual.DemandRef.ID != "" {
			c.actualDemandIDs[actual.DemandRef.ID] = true
		}
	}
}

func (c *workloadCalculation) addDemands() {
	for _, demand := range c.workspace.ResourceDemands {
		c.addDemand(demand)
	}
	for key, uses := range c.concurrent {
		row := c.rows[key]
		row.Planned, row.Remaining, row.Reserved = peakConcurrentUse(uses, c.weekStart, c.weekEnd, c.hasWeekFilter)
	}
}

func (c *workloadCalculation) addDemand(demand domain.ResourceDemand) {
	span := demandWindow(demand, c.tasks)
	if c.hasWeekFilter && !span.intersects(c.weekStart, c.weekEnd) {
		return
	}
	targetID, ok := demandTargetID(demand)
	if !ok {
		c.result.Warnings = append(c.result.Warnings, Diagnostic{Code: "unresolved_resource", Message: fmt.Sprintf("resource demand %s has no concrete resource or pool target", demand.ID), Severity: "warning", Target: Target{Kind: "resource_demand", ID: demand.ID}})
		return
	}
	if c.req.ResourceID != "" && targetID != c.req.ResourceID {
		return
	}
	row := c.rowFor(targetID, demand.Unit)
	share := 1.0
	if c.hasWeekFilter {
		share = spanShare(span, c.days[targetID], c.weekStart, c.weekEnd)
	}
	if isConcurrentResourceUnit(c.workspace.Capacities, targetID, demand.Unit) {
		key := targetID + "\x00" + demand.Unit
		c.concurrent[key] = append(c.concurrent[key], concurrentUse{span: span, planned: demand.PlannedAmount, remaining: demand.Remaining(), reserved: demand.ReservedAmount})
		if !c.actualDemandIDs[demand.ID] {
			row.Actual += demand.ActualAmount
		}
		if c.req.WithDemands {
			row.Demands = append(row.Demands, demandShare(demand, share, 1, true))
		}
		return
	}
	if c.req.WithDemands {
		row.Demands = append(row.Demands, demandShare(demand, share, share, false))
	}
	row.Planned += demand.PlannedAmount * share
	row.Reserved += demand.ReservedAmount * share
	if !c.actualDemandIDs[demand.ID] {
		row.Actual += demand.ActualAmount * share
	}
	row.Remaining += demand.Remaining() * share
}

func (c *workloadCalculation) finishRows() {
	for _, row := range c.rows {
		if (c.req.ResourceClass != "" && row.ResourceClass != c.req.ResourceClass) || (c.req.Unit != "" && row.Unit != c.req.Unit) {
			continue
		}
		if row.Capacity == 0 && (row.Planned > 0 || row.Remaining > 0 || row.Reserved > 0) {
			c.result.Warnings = append(c.result.Warnings, Diagnostic{Code: "missing_capacity", Message: fmt.Sprintf("no capacity found for %s in unit %s", row.ResourceID, row.Unit), Severity: "warning", Target: Target{Kind: "resource", ID: row.ResourceID}})
		}
		row.Planned, row.Reserved = roundDashboardFloat(row.Planned), roundDashboardFloat(row.Reserved)
		row.Actual, row.Remaining = roundDashboardFloat(row.Actual), roundDashboardFloat(row.Remaining)
		row.Capacity = roundDashboardFloat(row.Capacity)
		sort.Slice(row.Demands, func(i, j int) bool { return row.Demands[i].ResourceDemandID < row.Demands[j].ResourceDemandID })
		load := row.Reserved
		if load == 0 {
			load = row.Remaining
		}
		if row.Capacity > 0 && load > row.Capacity {
			row.Overload = load - row.Capacity
		}
		c.result.Resources = append(c.result.Resources, *row)
	}
	sort.Slice(c.result.Resources, func(i, j int) bool {
		if c.result.Resources[i].ResourceID == c.result.Resources[j].ResourceID {
			return c.result.Resources[i].Unit < c.result.Resources[j].Unit
		}
		return c.result.Resources[i].ResourceID < c.result.Resources[j].ResourceID
	})
	sort.Slice(c.result.Warnings, func(i, j int) bool {
		if c.result.Warnings[i].Code == c.result.Warnings[j].Code {
			return c.result.Warnings[i].Target.ID < c.result.Warnings[j].Target.ID
		}
		return c.result.Warnings[i].Code < c.result.Warnings[j].Code
	})
}

// capacityInWeek is the part of a capacity in effect between weekStart and
// weekEnd, inclusive: a whole week, or a part of one at the edge of a range.
// A weekly capacity counts its share of the resource's working days, one
// seventh a day for a resource without working days, so a change on a
// Wednesday splits that week and a Monday-to-Friday range holds a Monday-to-
// Friday worker's whole week. Any other period counts in full if the capacity
// is in effect on any day of the range.
func capacityInWeek(capacity domain.Capacity, weekStart, weekEnd time.Time, working map[time.Weekday]bool) float64 {
	from, to := capacityEffectiveRange(capacity)
	days := 0
	for day := weekStart; !day.After(weekEnd); day = day.AddDate(0, 0, 1) {
		if !day.Before(from) && !day.After(to) && (len(working) == 0 || working[day.Weekday()]) {
			days++
		}
	}
	perWeek := 7
	if len(working) > 0 {
		perWeek = len(working)
	}
	switch {
	case days == 0:
		return 0
	case capacity.Period == "week":
		return capacity.Amount * float64(days) / float64(perWeek)
	default:
		return capacity.Amount
	}
}

// capacityOnDate is the capacity in effect on the given date, the project
// status date for workload without a week. Without a date every capacity
// counts, as before effective dates were read.
func capacityOnDate(capacity domain.Capacity, date string) float64 {
	day, err := time.Parse("2006-01-02", date)
	if err != nil {
		return capacity.Amount
	}
	from, to := capacityEffectiveRange(capacity)
	if day.Before(from) || day.After(to) {
		return 0
	}
	return capacity.Amount
}

// capacityEffectiveRange reads effective_from and effective_to as inclusive
// dates. A missing or malformed bound is open; validate reports malformed
// ones.
func capacityEffectiveRange(capacity domain.Capacity) (time.Time, time.Time) {
	from := time.Date(1, time.January, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(9999, time.December, 31, 0, 0, 0, 0, time.UTC)
	if parsed, err := time.Parse("2006-01-02", capacity.EffectiveFrom); err == nil {
		from = parsed
	}
	if parsed, err := time.Parse("2006-01-02", capacity.EffectiveTo); err == nil {
		to = parsed
	}
	return from, to
}

func workloadWeekRange(week string) (time.Time, time.Time, bool) {
	if len(week) != 8 || week[4] != '-' || week[5] != 'W' {
		return time.Time{}, time.Time{}, false
	}
	year, err := strconv.Atoi(week[0:4])
	if err != nil {
		return time.Time{}, time.Time{}, false
	}
	weekNumber, err := strconv.Atoi(week[6:8])
	if err != nil || weekNumber < 1 || weekNumber > 53 {
		return time.Time{}, time.Time{}, false
	}

	// January 4 is always in ISO week 1. Move to that week's Monday, then add weeks.
	jan4 := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	weekday := int(jan4.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	start := jan4.AddDate(0, 0, -(weekday-1)+(weekNumber-1)*7)
	end := start.AddDate(0, 0, 6)
	return start, end, true
}

// demandShare describes a demand's part of a row: share is the fraction in
// the week, scale what its amounts are multiplied by.
func demandShare(demand domain.ResourceDemand, share, scale float64, concurrent bool) WorkloadDemandShare {
	return WorkloadDemandShare{
		ResourceDemandID: demand.ID,
		TaskID:           demand.TaskRef.ID,
		Status:           demand.Status,
		Concurrent:       concurrent,
		Share:            roundDashboardFloat(share),
		Planned:          roundDashboardFloat(demand.PlannedAmount * scale),
		Reserved:         roundDashboardFloat(demand.ReservedAmount * scale),
		Remaining:        roundDashboardFloat(demand.Remaining() * scale),
	}
}

func parseWorkloadDate(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

func actualIntersectsWeek(actual domain.Actual, weekStart, weekEnd time.Time) bool {
	dateText := strings.TrimSpace(actual.StatusDate)
	if dateText == "" {
		dateText = datePart(actual.OccurredAt)
	}
	date, ok := parseWorkloadDate(dateText)
	if !ok {
		return true
	}
	return !date.Before(weekStart) && !date.After(weekEnd)
}

func demandTargetID(demand domain.ResourceDemand) (string, bool) {
	if demand.ResourceRef.ID != "" {
		return demand.ResourceRef.ID, true
	}
	if demand.ResourcePoolRef.ID != "" {
		return demand.ResourcePoolRef.ID, true
	}
	return "", false
}
