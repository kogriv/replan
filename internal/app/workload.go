package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
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
	// Raw disables estimate calibration for this calculation.
	Raw bool
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
	ResourceID          string   `json:"resource_id"`
	ResourceName        string   `json:"resource_name,omitempty"`
	ResourceClass       string   `json:"resource_class,omitempty"`
	Unit                string   `json:"unit"`
	Capacity            float64  `json:"capacity"`
	Planned             float64  `json:"planned"`
	Reserved            float64  `json:"reserved"`
	Actual              float64  `json:"actual"`
	Remaining           float64  `json:"remaining"`
	Overload            float64  `json:"overload"`
	UnlevelledPlanned   float64  `json:"unlevelled_planned,omitempty"`
	UnlevelledRemaining float64  `json:"unlevelled_remaining,omitempty"`
	CalibrationIDs      []string `json:"calibration_ids,omitempty"`
	RawPlanned          *float64 `json:"raw_planned,omitempty"`
	RawRemaining        *float64 `json:"raw_remaining,omitempty"`
	// Demands is filled only on request (WorkloadRequest.WithDemands).
	Demands          []WorkloadDemandShare `json:"demands,omitempty"`
	Unscheduled      []WorkloadDemandShare `json:"unscheduled,omitempty"`
	RemainingPartial bool                  `json:"remaining_partial,omitempty"`
	capacityKnown    bool
	monthlyCapacity  bool
}

// WorkloadDemandShare is one demand's part of a workload row. Share is the
// fraction of the demand that falls in the requested week (1 without a week).
// A demand on a concurrent resource is not prorated: its amounts are its own,
// and the row holds the peak of the demands active on the same day.
type WorkloadDemandShare = reports.WorkloadDemandShare

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
		if errs := planInputErrors(ws); len(errs) != 0 {
			result.Errors = append(result.Errors, errs...)
			return result, ErrInvalidPlanInputs
		}
		return result, ErrValidationFailed
	}
	state := newWorkloadCalculation(req, ws, &result)
	if req.Week != "" && !state.hasWeekFilter {
		result.Errors = append(result.Errors, Diagnostic{Code: "invalid_week", Severity: "error", Message: fmt.Sprintf("week %q is not a valid ISO week", req.Week)})
		return result, ErrInvalidPlanInputs
	}
	if err := state.applyScenario(); err != nil {
		return result, err
	}
	if errs := planInputErrors(state.workspace); len(errs) != 0 {
		result.Errors = append(result.Errors, errs...)
		return result, ErrInvalidPlanInputs
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
	days                   map[string]resourceAvailability
	capacityByResourceUnit map[string]float64
	monthlyCapacity        map[string]bool
	actualDemandIDs        map[string]bool
	rows                   map[string]*ResourceWorkload
	concurrent             map[string][]concurrentUse
	rawConcurrent          map[string][]concurrentUse
	calibrations           map[string]DemandCalibration
	calibrationIDs         map[string]map[string]bool
	rawPlanned             map[string]float64
	rawRemaining           map[string]float64
	weekStart              time.Time
	weekEnd                time.Time
	hasWeekFilter          bool
}

func newWorkloadCalculation(req WorkloadRequest, ws domain.Workspace, result *WorkloadResult) *workloadCalculation {
	start, end, filtered := workloadWeekRange(req.Week)
	return &workloadCalculation{
		req: req, workspace: ws, result: result, resources: map[string]domain.Resource{}, pools: map[string]domain.ResourcePool{},
		tasks: map[string]domain.Task{}, capacityByResourceUnit: map[string]float64{}, monthlyCapacity: map[string]bool{}, actualDemandIDs: map[string]bool{},
		rows: map[string]*ResourceWorkload{}, concurrent: map[string][]concurrentUse{}, rawConcurrent: map[string][]concurrentUse{},
		calibrations: map[string]DemandCalibration{}, calibrationIDs: map[string]map[string]bool{}, rawPlanned: map[string]float64{}, rawRemaining: map[string]float64{},
		weekStart: start, weekEnd: end, hasWeekFilter: filtered,
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
	if !c.req.Raw {
		c.calibrations = ApplicableDemandCalibrations(calibrationSelectionWorkspace(c.workspace, c.req.ScenarioID))
	}
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
	c.addAvailabilityExceptionDiagnostics()
	for _, capacity := range c.workspace.Capacities {
		if !c.capacityApplies(capacity) {
			continue
		}
		key := capacity.ResourceRef.ID + "\x00" + capacity.Unit
		if c.hasWeekFilter && capacity.Period == "month" {
			c.monthlyCapacity[key] = true
			continue
		}
		c.capacityByResourceUnit[key] += c.capacityAmount(capacity)
	}
}

func (c workloadCalculation) capacityApplies(capacity domain.Capacity) bool {
	from, to := capacityEffectiveRange(capacity)
	if c.hasWeekFilter {
		return !from.After(c.weekEnd) && !to.Before(c.weekStart)
	}
	day, ok := parseWorkloadDate(c.workspace.Project.DefaultStatusDate)
	return !ok || (!day.Before(from) && !day.After(to))
}

func (c workloadCalculation) capacityAmount(capacity domain.Capacity) float64 {
	if c.hasWeekFilter {
		return capacityInWeek(capacity, c.weekStart, c.weekEnd, c.days[capacity.ResourceRef.ID])
	}
	return capacityOnDate(capacity, c.workspace.Project.DefaultStatusDate)
}

func (c *workloadCalculation) addAvailabilityExceptionDiagnostics() {
	counts := map[string]map[string]int{}
	for _, availability := range c.workspace.Availabilities {
		resourceID := availability.ResourceRef.ID
		for _, exception := range availability.Exceptions {
			exceptionType := strings.ToLower(strings.TrimSpace(exception.Type))
			switch exceptionType {
			case "reduced_capacity", "extra_capacity", "quota_reset":
			default:
				continue
			}
			if c.hasWeekFilter {
				day, ok := parseWorkloadDate(exception.Date)
				if !ok || day.Before(c.weekStart) || day.After(c.weekEnd) {
					continue
				}
			}
			if counts[resourceID] == nil {
				counts[resourceID] = map[string]int{}
			}
			counts[resourceID][exceptionType]++
		}
	}
	for resourceID, byType := range counts {
		parts := []string{}
		total := 0
		for _, exceptionType := range []string{"reduced_capacity", "extra_capacity", "quota_reset"} {
			if count := byType[exceptionType]; count > 0 {
				total += count
				parts = append(parts, fmt.Sprintf("%s: %d", exceptionType, count))
			}
		}
		message := fmt.Sprintf("%d availability exceptions have no numeric effect because the model carries no amount (%s)", total, strings.Join(parts, ", "))
		if total == 1 {
			message = fmt.Sprintf("1 availability exception has no numeric effect because the model carries no amount (%s)", strings.Join(parts, ", "))
		}
		c.result.Warnings = append(c.result.Warnings, Diagnostic{
			Code: "availability_exceptions_not_applied", Message: message, Severity: "warning",
			Target: Target{Kind: "resource", ID: resourceID},
		})
	}
}

func (c *workloadCalculation) rowFor(resourceID, unit string) *ResourceWorkload {
	key := resourceID + "\x00" + unit
	if row, ok := c.rows[key]; ok {
		return row
	}
	resource := c.resources[resourceID]
	row := &ResourceWorkload{ResourceID: resourceID, ResourceName: resource.Name, ResourceClass: resource.Class, Unit: unit}
	row.Capacity, row.capacityKnown = c.capacityByResourceUnit[key]
	row.monthlyCapacity = c.monthlyCapacity[key]
	if pool, ok := c.pools[resourceID]; ok {
		row.ResourceName, row.ResourceClass = pool.Name, pool.ResourceClass
		for _, member := range pool.Members {
			memberKey := member.ID + "\x00" + unit
			amount, known := c.capacityByResourceUnit[memberKey]
			row.Capacity += amount
			row.capacityKnown = row.capacityKnown || known
			row.monthlyCapacity = row.monthlyCapacity || c.monthlyCapacity[memberKey]
		}
	}
	c.rows[key] = row
	return row
}

func (c *workloadCalculation) addCapacities() {
	for _, capacity := range c.workspace.Capacities {
		if c.req.ResourceID == "" || capacity.ResourceRef.ID == c.req.ResourceID {
			c.rowFor(capacity.ResourceRef.ID, capacity.Unit)
		}
	}
}

func (c *workloadCalculation) addActuals() {
	for _, actual := range EffectiveActuals(c.workspace.Actuals) {
		if actual.Type != "work_consumed" || actual.ResourceRef.ID == "" || actual.Unit == "" {
			continue
		}
		if c.req.ResourceID != "" && actual.ResourceRef.ID != c.req.ResourceID {
			continue
		}
		if c.hasWeekFilter {
			dateText := actual.StatusDate
			if dateText == "" {
				dateText = projectDate(c.workspace.Project, actual.OccurredAt)
			}
			date, ok := parseWorkloadDate(dateText)
			if !ok {
				c.result.Warnings = append(c.result.Warnings, Diagnostic{Code: "undated_actual", Severity: "warning", Target: Target{Kind: "actual", ID: actual.ID}, Message: "actual without a date was omitted from weekly workload"})
				continue
			}
			if date.Before(c.weekStart) || date.After(c.weekEnd) {
				continue
			}
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
		rawPlanned, rawRemaining, _ := peakConcurrentUse(c.rawConcurrent[key], c.weekStart, c.weekEnd, c.hasWeekFilter)
		c.rawPlanned[key], c.rawRemaining[key] = rawPlanned, rawRemaining
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
	if (c.req.ResourceClass != "" && row.ResourceClass != c.req.ResourceClass) || (c.req.Unit != "" && row.Unit != c.req.Unit) {
		return
	}
	key := targetID + "\x00" + demand.Unit
	loadScale := 1.0
	if !domain.DemandAddsLevelledLoad(demand) {
		loadScale = 0
	}
	lifecycleScale := 1.0
	if !domain.DemandLoads(demand.Status) {
		lifecycleScale = 0
	}
	share := 1.0
	spread, unavailableFallback := spreadDaysWithFallback(span, c.days[targetID])
	if span.dated && unavailableFallback {
		c.result.Warnings = append(c.result.Warnings, Diagnostic{
			Code:     "demand_window_unavailable",
			Message:  fmt.Sprintf("resource demand %s falls entirely on days removed by availability exceptions; its load is retained on the regular working days", demand.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: demand.ID},
		})
	}
	if c.hasWeekFilter && span.dated {
		share = spanShare(span, c.days[targetID], c.weekStart, c.weekEnd)
	}
	planned, remaining, reserved := demand.PlannedAmount, demand.Remaining(), demand.ReservedAmount
	calibrationID := ""
	if match, ok := c.calibrations[demand.ID]; ok && loadScale != 0 {
		calibrationID = match.Calibration.ID
		planned = calibratedLoadAmount(planned, match.Factor)
		remaining = calibratedLoadAmount(remaining, match.Factor)
		reserved = calibratedLoadAmount(reserved, match.Factor)
	}
	concurrent := isConcurrentResourceUnit(c.workspace.Capacities, targetID, demand.Unit)
	unknown := demandRemainingUnknown(demand)
	target := Target{Kind: "resource_demand", ID: demand.ID}
	if !span.dated {
		scale := loadScale
		if !demand.IsLevelled() {
			scale = lifecycleScale
		}
		row.Unscheduled = append(row.Unscheduled, demandShare(demand, 1, scale, concurrent, planned, remaining, reserved, calibrationID))
		if !calibrationDemandClosed(demand.Status) && demand.Criticality == "required" {
			c.result.Warnings = append(c.result.Warnings, Diagnostic{Code: "unscheduled_required_demand", Severity: "warning", Target: target, Message: "required demand has no scheduled date range"})
		}
	}
	if unknown && demand.Criticality != "optional" {
		message := "remaining amount unknown; remaining totals show only known amounts"
		if c.hasWeekFilter && !span.dated {
			message = "remaining amount unknown in unscheduled work"
		}
		c.result.Warnings = append(c.result.Warnings, Diagnostic{Code: "remaining_amount_unknown", Severity: "warning", Target: target, Message: message})
	}
	if c.hasWeekFilter && !span.dated {
		// Preserve the legacy embedded actual fallback independently of planned load.
		if !c.actualDemandIDs[demand.ID] {
			row.Actual += demand.ActualAmount
		}
		return
	}
	row.RemainingPartial = row.RemainingPartial || (unknown && (share > 0 || concurrent))
	if !demand.IsLevelled() {
		row.UnlevelledPlanned += planned * share * lifecycleScale
		row.UnlevelledRemaining += remaining * share * lifecycleScale
	}
	if calibrationID != "" {
		if c.calibrationIDs[key] == nil {
			c.calibrationIDs[key] = map[string]bool{}
		}
		c.calibrationIDs[key][calibrationID] = true
	}
	if concurrent {
		spreadMap := spreadDayMap(span, spread)
		c.concurrent[key] = append(c.concurrent[key], concurrentUse{span: span, planned: planned * loadScale, remaining: remaining * loadScale, reserved: reserved * loadScale, days: spreadMap})
		c.rawConcurrent[key] = append(c.rawConcurrent[key], concurrentUse{span: span, planned: demand.PlannedAmount * loadScale, remaining: demand.Remaining() * loadScale, reserved: demand.ReservedAmount * loadScale, days: spreadMap})
		if !c.actualDemandIDs[demand.ID] {
			row.Actual += demand.ActualAmount
		}
		if c.req.WithDemands {
			demandScale := loadScale
			if !demand.IsLevelled() {
				demandScale = lifecycleScale
			}
			row.Demands = append(row.Demands, demandShare(demand, share, demandScale, true, planned, remaining, reserved, calibrationID))
		}
		return
	}
	if c.req.WithDemands {
		demandScale := share * loadScale
		if !demand.IsLevelled() {
			demandScale = share * lifecycleScale
		}
		row.Demands = append(row.Demands, demandShare(demand, share, demandScale, false, planned, remaining, reserved, calibrationID))
	}
	row.Planned += planned * share * loadScale
	row.Reserved += reserved * share * loadScale
	c.rawPlanned[key] += demand.PlannedAmount * share * loadScale
	c.rawRemaining[key] += demand.Remaining() * share * loadScale
	if !c.actualDemandIDs[demand.ID] {
		row.Actual += demand.ActualAmount * share
	}
	row.Remaining += remaining * share * loadScale
}

func (c *workloadCalculation) finishRows() {
	for _, row := range c.rows {
		key := row.ResourceID + "\x00" + row.Unit
		if (c.req.ResourceClass != "" && row.ResourceClass != c.req.ResourceClass) || (c.req.Unit != "" && row.Unit != c.req.Unit) {
			continue
		}
		if row.monthlyCapacity {
			c.result.Warnings = append(c.result.Warnings, Diagnostic{Code: "monthly_capacity_not_weekly", Message: fmt.Sprintf("monthly capacity for %s in unit %s is not a weekly entitlement; omitted from weekly capacity and overload", row.ResourceID, row.Unit), Severity: "warning", Target: Target{Kind: "resource", ID: row.ResourceID}})
		} else if !row.capacityKnown && (row.Planned > 0 || row.Remaining > 0 || row.Reserved > 0) {
			c.result.Warnings = append(c.result.Warnings, Diagnostic{Code: "missing_capacity", Message: fmt.Sprintf("no capacity found for %s in unit %s", row.ResourceID, row.Unit), Severity: "warning", Target: Target{Kind: "resource", ID: row.ResourceID}})
		}
		row.Planned, row.Reserved = roundDashboardFloat(row.Planned), roundDashboardFloat(row.Reserved)
		row.Actual, row.Remaining = roundDashboardFloat(row.Actual), roundDashboardFloat(row.Remaining)
		row.UnlevelledPlanned = roundDashboardFloat(row.UnlevelledPlanned)
		row.UnlevelledRemaining = roundDashboardFloat(row.UnlevelledRemaining)
		row.Capacity = roundDashboardFloat(row.Capacity)
		if ids := c.calibrationIDs[key]; len(ids) > 0 {
			row.CalibrationIDs = sortedMapKeys(ids)
			rawPlanned, rawRemaining := roundDashboardFloat(c.rawPlanned[key]), roundDashboardFloat(c.rawRemaining[key])
			row.RawPlanned, row.RawRemaining = &rawPlanned, &rawRemaining
		}
		sort.Slice(row.Unscheduled, func(i, j int) bool { return row.Unscheduled[i].ResourceDemandID < row.Unscheduled[j].ResourceDemandID })
		sort.Slice(row.Demands, func(i, j int) bool { return row.Demands[i].ResourceDemandID < row.Demands[j].ResourceDemandID })
		load := row.Reserved
		if load == 0 {
			load = row.Remaining
		}
		if row.capacityKnown && load > row.Capacity {
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
// Friday worker's whole week. Unscoped or concurrent capacity counts in full
// when in effect. A monthly limit cannot be used as weekly capacity.
func capacityInWeek(capacity domain.Capacity, weekStart, weekEnd time.Time, availability resourceAvailability) float64 {
	if capacity.Period == "month" {
		return 0
	}
	from, to := capacityEffectiveRange(capacity)
	days := 0
	for day := weekStart; !day.After(weekEnd); day = day.AddDate(0, 0, 1) {
		if !day.Before(from) && !day.After(to) && availability.availableDay(day) {
			days++
		}
	}
	perWeek := availability.regularDaysPerWeek()
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
	// Without a week the row shows the record's amount in effect on the
	// status date, not that day's availability: a status date on a weekend
	// or an absence day must not turn a weekly amount into zero.
	return capacity.Amount
}

func spreadDayMap(span demandSpan, days []time.Time) map[string]bool {
	if !span.dated {
		return nil
	}
	out := make(map[string]bool, len(days))
	for _, day := range days {
		out[day.Format("2006-01-02")] = true
	}
	return out
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
	return domain.ISOWeekRange(week)
}

// demandShare describes a demand's part of a row: share is the fraction in
// the week, scale what its amounts are multiplied by.
func demandShare(demand domain.ResourceDemand, share, scale float64, concurrent bool, planned, remaining, reserved float64, calibrationID string) WorkloadDemandShare {
	result := WorkloadDemandShare{
		RemainingUnknown: demandRemainingUnknown(demand),
		ResourceDemandID: demand.ID,
		TaskID:           demand.TaskRef.ID,
		Status:           demand.Status,
		Concurrent:       concurrent,
		Share:            roundDashboardFloat(share),
		Planned:          roundDashboardFloat(planned * scale),
		Reserved:         roundDashboardFloat(reserved * scale),
		Remaining:        roundDashboardFloat(remaining * scale),
		CalibrationID:    calibrationID,
	}
	if !demand.IsLevelled() {
		levelled := false
		result.Levelled = &levelled
	}
	if calibrationID != "" {
		rawPlanned, rawRemaining := roundDashboardFloat(demand.PlannedAmount*scale), roundDashboardFloat(demand.Remaining()*scale)
		result.RawPlanned, result.RawRemaining = &rawPlanned, &rawRemaining
	}
	return result
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

func demandTargetID(demand domain.ResourceDemand) (string, bool) {
	if demand.ResourceRef.ID != "" {
		return demand.ResourceRef.ID, true
	}
	if demand.ResourcePoolRef.ID != "" {
		return demand.ResourcePoolRef.ID, true
	}
	return "", false
}

func demandRemainingUnknown(demand domain.ResourceDemand) bool {
	return domain.DemandLoads(demand.Status) && !calibrationDemandClosed(demand.Status) && !demand.HasRemaining()
}
