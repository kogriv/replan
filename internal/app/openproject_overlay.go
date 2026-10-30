package app

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
)

// overlaySchemaVersion is the version of raw/openproject_overlay.json.
// Version 2 adds what the redesigned plugin views draw; every version 1 field
// is kept, so a plugin that reads version 1 keeps working.
const overlaySchemaVersion = 2

// dashboardOpenProjectOverlay builds the artifact the OpenProject plugin
// reads. Everything in it is derived here; the plugin only lays it out.
func dashboardOpenProjectOverlay(ws domain.Workspace, overview reports.DashboardOverview) reports.DashboardOpenProjectOverlay {
	taskByID := dashboardGanttTasksByID(overview.Gantt)
	trackingByTaskID := map[string]reports.DashboardTrackingRow{}
	for _, row := range overview.Tracking {
		trackingByTaskID[row.TaskID] = row
	}
	index := newOverlayIndex(ws, overview)

	overlay := reports.DashboardOpenProjectOverlay{
		SchemaVersion:        overlaySchemaVersion,
		GeneratedAt:          overview.GeneratedAt,
		ProjectID:            overview.ProjectID,
		ProjectName:          overview.ProjectName,
		StatusDate:           overview.StatusDate,
		Week:                 overview.Week,
		ScenarioID:           overview.ScenarioID,
		OpenProjectProjectID: overview.OpenProject.ProjectID,
		ArtifactRefs: reports.DashboardOpenProjectOverlayArtifacts{
			OverviewJSON:  filepath.ToSlash(filepath.Join("raw", "overview.json")),
			GanttJSON:     filepath.ToSlash(filepath.Join("raw", "gantt.json")),
			OverlayJSON:   filepath.ToSlash(filepath.Join("raw", "openproject_overlay.json")),
			DashboardHTML: "index.html",
			GanttHTML:     "gantt.html",
		},
		MissingLinks: append([]reports.DashboardOpenProjectMissing{}, overview.OpenProject.MissingLinks...),
		Span: reports.DashboardOpenProjectOverlaySpan{
			StartDate:        overview.Gantt.StartDate,
			FinishDate:       overview.Gantt.FinishDate,
			DayCount:         overview.Gantt.DayCount,
			StatusOffsetDays: overview.Gantt.StatusOffset,
			StatusInRange:    overview.Gantt.StatusInRange,
		},
		WarningCount:  len(overview.Warnings),
		PersonActions: overlayPersonActions(overview.NextActions, index),
	}
	for _, proposal := range ws.Proposals {
		if proposal.Status == "open" {
			overlay.OpenProposalCount++
		}
	}
	overlay.Resources, overlay.OverloadedWeeks = overlayResources(ws, overview, index)

	for _, link := range overview.OpenProject.Links {
		ganttTask := taskByID[link.TaskID]
		tracking := trackingByTaskID[link.TaskID]
		row := reports.DashboardOpenProjectOverlayWorkPackage{
			WorkPackageID:         link.OpenProjectID,
			WorkPackageURL:        link.OpenProjectURL,
			RePlanTaskID:          link.TaskID,
			Title:                 firstNonEmpty(ganttTask.Title, link.Title),
			Type:                  ganttTask.Type,
			TrackingStatus:        firstNonEmpty(ganttTask.TrackingStatus, link.TrackingStatus),
			PlannedStart:          firstNonEmpty(ganttTask.PlannedStart, link.PlannedStart),
			PlannedFinish:         firstNonEmpty(ganttTask.PlannedFinish, link.PlannedFinish),
			StartOffsetDays:       ganttTask.StartOffsetDays,
			DurationDays:          ganttTask.DurationDays,
			ResourceIDs:           append([]string{}, ganttTask.ResourceIDs...),
			OverloadedResourceIDs: append([]string{}, ganttTask.OverloadedResourceIDs...),
			ResourceDemands:       dashboardOpenProjectOverlayDemands(ganttTask.Allocations, tracking.Demands),
			WarningCodes:          append([]string{}, ganttTask.WarningCodes...),
			DependencyCount:       ganttTask.DependencyCount,
			OpenDependencyCount:   ganttTask.OpenDependencyCount,
			HasOpenProposal:       ganttTask.HasOpenProposal,
			Tracking: reports.DashboardOpenProjectOverlayTracking{
				ActualCount:   tracking.ActualCount,
				EvidenceCount: tracking.EvidenceCount,
				Flags:         append([]string{}, tracking.Flags...),
			},
		}
		index.describeWorkPackage(&row)
		overlay.WorkPackages = append(overlay.WorkPackages, row)
	}
	sort.Slice(overlay.WorkPackages, func(i, j int) bool {
		if overlay.WorkPackages[i].WorkPackageID == overlay.WorkPackages[j].WorkPackageID {
			return overlay.WorkPackages[i].RePlanTaskID < overlay.WorkPackages[j].RePlanTaskID
		}
		return overlay.WorkPackages[i].WorkPackageID < overlay.WorkPackages[j].WorkPackageID
	})
	overlay.WorkPackageCount = len(overlay.WorkPackages)
	overlay.LinkedTaskCount = overview.OpenProject.LinkedTaskCount
	overlay.MissingLinkCount = overview.OpenProject.MissingLinkCount
	return overlay
}

// overlayIndex holds the workspace lookups the version 2 fields need.
type overlayIndex struct {
	ws                domain.Workspace
	tasks             map[string]domain.Task
	demands           map[string]domain.ResourceDemand
	resourceNames     map[string]string
	workPackages      map[string]string
	days              map[string]map[time.Weekday]bool
	evidence          map[string][]reports.DashboardOpenProjectOverlayEvidence
	actuals           map[string][]reports.DashboardOpenProjectOverlayActual
	warnings          map[string][]reports.ReportIssue
	proposals         map[string][]reports.DashboardOpenProjectOverlayProposal
	dependencies      map[string][]reports.DashboardOpenProjectOverlayDependency
	demandTaskIDs     map[string]string
	dependencyTaskIDs map[string]string
}

func newOverlayIndex(ws domain.Workspace, overview reports.DashboardOverview) overlayIndex {
	index := overlayIndex{
		ws:                ws,
		tasks:             map[string]domain.Task{},
		demands:           map[string]domain.ResourceDemand{},
		resourceNames:     map[string]string{},
		workPackages:      map[string]string{},
		days:              workingDays(ws.Availabilities),
		evidence:          map[string][]reports.DashboardOpenProjectOverlayEvidence{},
		actuals:           map[string][]reports.DashboardOpenProjectOverlayActual{},
		warnings:          map[string][]reports.ReportIssue{},
		proposals:         map[string][]reports.DashboardOpenProjectOverlayProposal{},
		dependencies:      map[string][]reports.DashboardOpenProjectOverlayDependency{},
		demandTaskIDs:     map[string]string{},
		dependencyTaskIDs: map[string]string{},
	}
	for _, task := range ws.Tasks {
		index.tasks[task.ID] = task
	}
	for _, demand := range ws.ResourceDemands {
		index.demands[demand.ID] = demand
		index.demandTaskIDs[demand.ID] = demand.TaskRef.ID
	}
	for _, dependency := range ws.Dependencies {
		if dependency.SuccessorRef.Kind == "task" {
			index.dependencyTaskIDs[dependency.ID] = dependency.SuccessorRef.ID
		}
	}
	for _, resource := range ws.Resources {
		index.resourceNames[resource.ID] = firstNonEmpty(resource.Name, resource.ID)
	}
	for _, pool := range ws.ResourcePools {
		if _, ok := index.resourceNames[pool.ID]; !ok {
			index.resourceNames[pool.ID] = firstNonEmpty(pool.Name, pool.ID)
		}
	}
	for _, link := range overview.OpenProject.Links {
		index.workPackages[link.TaskID] = link.OpenProjectID
	}

	index.indexFacts(ws)
	index.indexSignals(ws, overview)
	return index
}

// indexFacts collects each task's evidence and booked consumption, oldest
// first.
func (index overlayIndex) indexFacts(ws domain.Workspace) {
	for _, evidence := range ws.Evidence {
		for _, target := range evidence.Targets {
			if target.Kind != "task" {
				continue
			}
			index.evidence[target.ID] = append(index.evidence[target.ID], reports.DashboardOpenProjectOverlayEvidence{
				ID:      evidence.ID,
				Date:    datePart(evidence.ObservedAt),
				Kind:    firstNonEmpty(evidence.Subtype, evidence.Type),
				Summary: evidence.Summary,
			})
		}
	}
	for taskID, rows := range index.evidence {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Date == rows[j].Date {
				return rows[i].ID < rows[j].ID
			}
			return rows[i].Date < rows[j].Date
		})
		index.evidence[taskID] = rows
	}

	for _, actual := range EffectiveActuals(ws.Actuals) {
		taskID := firstNonEmpty(actual.TaskRef.ID, index.demandTaskIDs[actual.DemandRef.ID])
		if taskID == "" || actual.Amount == 0 {
			continue
		}
		resourceID := firstNonEmpty(actual.ResourceRef.ID, demandResourceID(index.demands[actual.DemandRef.ID]))
		index.actuals[taskID] = append(index.actuals[taskID], reports.DashboardOpenProjectOverlayActual{
			Date:         firstNonEmpty(actual.StatusDate, datePart(actual.OccurredAt)),
			DemandID:     actual.DemandRef.ID,
			ResourceID:   resourceID,
			ResourceName: index.resourceName(resourceID),
			Amount:       actual.Amount,
			Unit:         actual.Unit,
		})
	}
	for taskID, rows := range index.actuals {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Date == rows[j].Date {
				return rows[i].ResourceID < rows[j].ResourceID
			}
			return rows[i].Date < rows[j].Date
		})
		index.actuals[taskID] = rows
	}

}

// indexSignals collects each task's warnings, open proposals and
// dependencies.
func (index overlayIndex) indexSignals(ws domain.Workspace, overview reports.DashboardOverview) {
	// Warnings are the ones validate reports, so the plugin and the CLI
	// agree; a warning about a demand belongs to its task.
	for _, warning := range overview.Warnings {
		if taskID := index.targetTaskID(warning.TargetID); taskID != "" {
			index.warnings[taskID] = append(index.warnings[taskID], warning)
		}
	}
	for _, proposal := range ws.Proposals {
		if proposal.Status != "open" {
			continue
		}
		if taskID := index.targetTaskID(proposal.TargetRef.ID); taskID != "" {
			index.proposals[taskID] = append(index.proposals[taskID], reports.DashboardOpenProjectOverlayProposal{
				ID:        proposal.ID,
				Type:      proposal.Type,
				TargetID:  proposal.TargetRef.ID,
				Rationale: proposal.Rationale,
			})
		}
	}

	for _, dependency := range ws.Dependencies {
		predecessor, successor := dependency.PredecessorRef, dependency.SuccessorRef
		if successor.Kind == "task" && successor.ID != "" {
			index.dependencies[successor.ID] = append(index.dependencies[successor.ID],
				index.dependencyView(dependency, "predecessor", predecessor))
		}
		if predecessor.Kind == "task" && predecessor.ID != "" {
			index.dependencies[predecessor.ID] = append(index.dependencies[predecessor.ID],
				index.dependencyView(dependency, "successor", successor))
		}
	}
	for taskID, rows := range index.dependencies {
		sort.SliceStable(rows, func(i, j int) bool {
			if rows[i].Direction == rows[j].Direction {
				return rows[i].ID < rows[j].ID
			}
			return rows[i].Direction < rows[j].Direction
		})
		index.dependencies[taskID] = rows
	}
}

func (index overlayIndex) resourceName(resourceID string) string {
	if name, ok := index.resourceNames[resourceID]; ok {
		return name
	}
	return resourceID
}

// targetTaskID maps a task, demand or dependency id to the task it concerns;
// a dependency concerns the task that waits on it.
func (index overlayIndex) targetTaskID(targetID string) string {
	if _, ok := index.tasks[targetID]; ok {
		return targetID
	}
	if taskID, ok := index.demandTaskIDs[targetID]; ok {
		return taskID
	}
	return index.dependencyTaskIDs[targetID]
}

// dependencyView describes the other side of a dependency. The other side is
// a task, or something outside the plan described by the dependency itself.
func (index overlayIndex) dependencyView(dependency domain.Dependency, direction string, other domain.Ref) reports.DashboardOpenProjectOverlayDependency {
	view := reports.DashboardOpenProjectOverlayDependency{
		ID:          dependency.ID,
		Direction:   direction,
		Status:      dependency.Status,
		Criticality: dependency.Criticality,
	}
	if other.Kind != "task" {
		view.TaskTitle = firstNonEmpty(dependency.Description, dependency.ExternalRef.ID, other.ID)
		return view
	}
	view.TaskID = other.ID
	view.WorkPackageID = index.workPackages[other.ID]
	if task, ok := index.tasks[other.ID]; ok {
		view.TaskTitle = task.Title
		view.TaskStatus = task.TrackingStatus
	}
	return view
}

// describeWorkPackage fills a work package's version 2 fields.
func (index overlayIndex) describeWorkPackage(row *reports.DashboardOpenProjectOverlayWorkPackage) {
	taskID := row.RePlanTaskID
	row.Evidence = append([]reports.DashboardOpenProjectOverlayEvidence{}, index.evidence[taskID]...)
	row.Actuals = append([]reports.DashboardOpenProjectOverlayActual{}, index.actuals[taskID]...)
	row.Warnings = append([]reports.ReportIssue{}, index.warnings[taskID]...)
	row.OpenProposals = append([]reports.DashboardOpenProjectOverlayProposal{}, index.proposals[taskID]...)
	row.Dependencies = append([]reports.DashboardOpenProjectOverlayDependency{}, index.dependencies[taskID]...)
	for _, actual := range row.Actuals {
		if actual.Date == "" {
			continue
		}
		if row.FirstActualDate == "" || actual.Date < row.FirstActualDate {
			row.FirstActualDate = actual.Date
		}
		if actual.Date > row.LastActualDate {
			row.LastActualDate = actual.Date
		}
	}
	for i := range row.ResourceDemands {
		lane := &row.ResourceDemands[i]
		lane.ResourceName = index.resourceName(lane.ResourceID)
		lane.Concurrent = isConcurrentResourceUnit(index.ws.Capacities, lane.ResourceID, lane.Unit)
		lane.DailyCapacity = roundDashboardFloat(index.dailyCapacity(lane.ResourceID, lane.Unit, lane.StartDate, lane.FinishDate))
	}
}

// dailyCapacity is the capacity in effect on the first working day of a
// window, for one working day: a week's capacity is shared by its working
// days, a slot capacity is the number of slots.
func (index overlayIndex) dailyCapacity(resourceID, unit, startText, finishText string) float64 {
	start, startOK := parseWorkloadDate(startText)
	finish, finishOK := parseWorkloadDate(finishText)
	if !startOK {
		return 0
	}
	if !finishOK || finish.Before(start) {
		finish = start
	}
	for day := start; !day.After(finish); day = day.AddDate(0, 0, 1) {
		if capacity := resourceCapacityBetween(index.ws, resourceID, unit, day, day); capacity > 0 {
			return capacity
		}
	}
	return 0
}

// overlayPersonActions keeps the next actions a person has to take.
func overlayPersonActions(actions []reports.DashboardNextAction, index overlayIndex) []reports.DashboardOpenProjectOverlayAction {
	result := []reports.DashboardOpenProjectOverlayAction{}
	for _, action := range actions {
		if action.Actor != reports.NextActionActorPerson {
			continue
		}
		view := reports.DashboardOpenProjectOverlayAction{Code: action.Code, Label: action.Label, Date: action.Date}
		if taskID := index.targetTaskID(action.Target); taskID != "" {
			view.TaskID = taskID
			view.WorkPackageID = index.workPackages[taskID]
		}
		result = append(result, view)
	}
	return result
}

// overlayResources lists every resource and unit the plan uses with its
// weeks, and the overloaded resource-weeks that have not ended by the status
// date, busiest first.
func overlayResources(ws domain.Workspace, overview reports.DashboardOverview, index overlayIndex) ([]reports.DashboardOpenProjectOverlayResource, []reports.DashboardOpenProjectOverlayOverloadWeek) {
	classes := map[string]string{}
	for _, resource := range ws.Resources {
		classes[resource.ID] = resource.Class
	}
	resources := []reports.DashboardOpenProjectOverlayResource{}
	for _, load := range overview.Gantt.ResourceLoads {
		resource := reports.DashboardOpenProjectOverlayResource{
			ID:              load.ResourceID,
			Name:            firstNonEmpty(load.ResourceName, index.resourceName(load.ResourceID)),
			Class:           firstNonEmpty(load.ResourceClass, classes[load.ResourceID]),
			Unit:            load.Unit,
			Concurrent:      load.Concurrent,
			WorkingWeekdays: weekdayNames(index.days[load.ResourceID]),
			Weeks:           []reports.DashboardOpenProjectOverlayWeekBucket{},
		}
		for _, bucket := range load.Buckets {
			resource.Weeks = append(resource.Weeks, reports.DashboardOpenProjectOverlayWeekBucket{
				Label:      bucket.Label,
				StartDate:  bucket.StartDate,
				FinishDate: bucket.FinishDate,
				Capacity:   bucket.Capacity,
				Planned:    bucket.Planned,
				Overload:   bucket.Overload,
			})
		}
		resources = append(resources, resource)
	}
	return resources, overloadedWeeksAhead(ws, overview, index)
}

// overloadedWeeksAhead runs the weekly workload for every week of the plan
// from the status week on, so the plugin's counter and `replan workload`
// agree: done work does not count, remaining work does.
func overloadedWeeksAhead(ws domain.Workspace, overview reports.DashboardOverview, index overlayIndex) []reports.DashboardOpenProjectOverlayOverloadWeek {
	weeks := map[string]bool{}
	for _, load := range overview.Gantt.ResourceLoads {
		for _, bucket := range load.Buckets {
			if bucket.FinishDate >= overview.StatusDate {
				weeks[bucket.Label] = true
			}
		}
	}
	labels := make([]string, 0, len(weeks))
	for label := range weeks {
		labels = append(labels, label)
	}
	sort.Strings(labels)
	service := NewWorkloadService(workspaceInMemory{ws: ws})
	result := []reports.DashboardOpenProjectOverlayOverloadWeek{}
	for _, label := range labels {
		workload, err := service.Calculate(context.Background(), WorkloadRequest{Week: label})
		if err != nil {
			continue
		}
		for _, row := range workload.Resources {
			if row.Overload <= 0 {
				continue
			}
			load := row.Reserved
			if load == 0 {
				load = row.Remaining
			}
			result = append(result, reports.DashboardOpenProjectOverlayOverloadWeek{
				ResourceID:   row.ResourceID,
				ResourceName: firstNonEmpty(row.ResourceName, index.resourceName(row.ResourceID)),
				Unit:         row.Unit,
				Week:         label,
				Capacity:     row.Capacity,
				Load:         load,
				Overload:     roundDashboardFloat(row.Overload),
			})
		}
	}
	return result
}

// workspaceInMemory serves an already loaded workspace to a service.
type workspaceInMemory struct {
	ws          domain.Workspace
	projectPath string
}

func (l workspaceInMemory) Load(context.Context) (domain.Workspace, []Diagnostic) {
	return l.ws, nil
}

func (l workspaceInMemory) ProjectPath() string { return l.projectPath }

// weekdayNames lists working weekdays Monday first; none means every day.
func weekdayNames(days map[time.Weekday]bool) []string {
	var names []string
	for _, day := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
		if days[day] {
			names = append(names, strings.ToLower(day.String()))
		}
	}
	return names
}

func dashboardGanttTasksByID(gantt reports.DashboardGanttView) map[string]reports.DashboardGanttTask {
	result := map[string]reports.DashboardGanttTask{}
	for _, task := range gantt.Tasks {
		result[task.ID] = task
	}
	for _, task := range gantt.Unscheduled {
		result[task.ID] = task
	}
	return result
}

func dashboardOpenProjectOverlayDemands(allocations []reports.DashboardGanttResourceAllocation, tracking []reports.DashboardDemandTracking) []reports.DashboardOpenProjectOverlayResourceDemand {
	trackingByDemandID := map[string]reports.DashboardDemandTracking{}
	for _, demand := range tracking {
		trackingByDemandID[demand.DemandID] = demand
	}
	seen := map[string]bool{}
	result := []reports.DashboardOpenProjectOverlayResourceDemand{}
	for _, allocation := range allocations {
		demandTracking := trackingByDemandID[allocation.DemandID]
		result = append(result, reports.DashboardOpenProjectOverlayResourceDemand{
			DemandID:                  allocation.DemandID,
			ResourceID:                firstNonEmpty(allocation.ResourceID, demandTracking.ResourceID),
			ResourceClass:             firstNonEmpty(allocation.ResourceClass, demandTracking.ResourceClass),
			Role:                      firstNonEmpty(allocation.Role, demandTracking.Role),
			Unit:                      firstNonEmpty(allocation.Unit, demandTracking.Unit),
			PlannedAmount:             firstNonZero(allocation.PlannedAmount, demandTracking.Planned),
			ActualAmount:              demandTracking.Actual,
			RemainingAmount:           firstNonZero(allocation.RemainingAmount, demandTracking.Remaining),
			ForecastAmount:            demandTracking.Forecast,
			VarianceAmount:            demandTracking.Variance,
			EvidenceCount:             demandTracking.EvidenceCount,
			StartDate:                 allocation.StartDate,
			FinishDate:                allocation.FinishDate,
			StartOffsetDays:           allocation.StartOffsetDays,
			DurationDays:              allocation.DurationDays,
			ParticipationWindowSource: allocation.ParticipationWindowSource,
			WindowCapacity:            allocation.WindowCapacity,
			IntensityRatio:            allocation.IntensityRatio,
			IntensityPercent:          allocation.IntensityPercent,
			CapacityKnown:             allocation.CapacityKnown,
			AmountPerDay:              allocation.AmountPerDay,
			Overloaded:                allocation.Overloaded,
			OverCapacity:              allocation.OverCapacity,
			Flags:                     append([]string{}, demandTracking.Flags...),
		})
		seen[allocation.DemandID] = true
	}
	for _, demandTracking := range tracking {
		if seen[demandTracking.DemandID] {
			continue
		}
		result = append(result, reports.DashboardOpenProjectOverlayResourceDemand{
			DemandID:        demandTracking.DemandID,
			ResourceID:      demandTracking.ResourceID,
			ResourceClass:   demandTracking.ResourceClass,
			Role:            demandTracking.Role,
			Unit:            demandTracking.Unit,
			PlannedAmount:   demandTracking.Planned,
			ActualAmount:    demandTracking.Actual,
			RemainingAmount: demandTracking.Remaining,
			ForecastAmount:  demandTracking.Forecast,
			VarianceAmount:  demandTracking.Variance,
			EvidenceCount:   demandTracking.EvidenceCount,
			Flags:           append([]string{}, demandTracking.Flags...),
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].ResourceID == result[j].ResourceID {
			return result[i].DemandID < result[j].DemandID
		}
		return result[i].ResourceID < result[j].ResourceID
	})
	return result
}
