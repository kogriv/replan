package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/reports"
)

type ReportService struct {
	loader WorkspaceLoader
}

type WeeklyReportRequest struct {
	ProjectPath string
	Week        string
	ScenarioID  string
}

type DashboardReportRequest struct {
	ProjectPath  string
	Week         string
	GeneratedAt  time.Time
	Command      string
	ArtifactBase string
	ScenarioID   string
}

func NewReportService(loader WorkspaceLoader) ReportService {
	return ReportService{loader: loader}
}

func (s ReportService) Weekly(ctx context.Context, req WeeklyReportRequest) (reports.WeeklyReport, error) {
	ws, loadDiags := s.loader.Load(ctx)
	report := reports.WeeklyReport{
		ProjectPath: s.loader.ProjectPath(),
		ProjectID:   ws.Project.ID,
		ProjectName: ws.Project.Name,
		Week:        req.Week,
		ScenarioID:  req.ScenarioID,
		CurrentPlan: ws.CurrentPlan.ID,
		Resources:   []reports.ResourceRow{},
		Proposals:   []reports.ProposalRow{},
		Evidence:    []reports.EvidenceRow{},
		Warnings:    []reports.ReportIssue{},
		Errors:      []reports.ReportIssue{},
	}
	if hasDiagnosticCode(loadDiags, "project_format_unsupported") {
		for _, diagnostic := range loadDiags {
			appendReportIssue(&report, diagnostic)
		}
		report.Summary.ErrorCount = len(report.Errors)
		return report, ErrValidationFailed
	}
	reportLoader := s.loader
	overlayWarnings, overlayErrors := []Diagnostic{}, []Diagnostic{}
	if req.ScenarioID != "" {
		overlay := ApplyScenarioOverlay(ws, req.ScenarioID)
		overlayWarnings, overlayErrors = overlay.Warnings, overlay.Errors
		if len(overlayErrors) == 0 {
			ws = overlay.Workspace
			reportLoader = workspaceInMemory{ws: ws, projectPath: s.loader.ProjectPath()}
		}
	}
	workload, err := NewWorkloadService(reportLoader).Calculate(ctx, WorkloadRequest{
		ProjectPath: req.ProjectPath,
		Week:        req.Week,
	})
	if len(overlayErrors) > 0 {
		err = ErrValidationFailed
	}

	if baseline, ok := reportBaseline(ws); ok {
		report.BaselineID = baseline.ID
	}

	for _, row := range workload.Resources {
		report.Resources = append(report.Resources, reports.ResourceRow{
			ResourceID:    row.ResourceID,
			ResourceName:  row.ResourceName,
			ResourceClass: row.ResourceClass,
			Unit:          row.Unit,
			Capacity:      row.Capacity,
			Planned:       row.Planned,
			Reserved:      row.Reserved,
			Actual:        row.Actual,
			Remaining:     row.Remaining,
			Overload:      row.Overload,
		})
		if row.Overload > 0 {
			report.Summary.Overloaded++
		}
	}

	for _, proposal := range ws.Proposals {
		if proposal.Status != "open" {
			continue
		}
		report.Proposals = append(report.Proposals, reports.ProposalRow{
			ID:        proposal.ID,
			Type:      proposal.Type,
			Status:    proposal.Status,
			TargetID:  proposal.TargetRef.ID,
			Rationale: proposal.Rationale,
		})
	}

	for _, evidence := range ws.Evidence {
		report.Evidence = append(report.Evidence, reports.EvidenceRow{
			ID:      evidence.ID,
			Type:    evidence.Type,
			Subtype: evidence.Subtype,
			Summary: evidence.Summary,
		})
	}

	for _, d := range loadDiags {
		appendReportIssue(&report, d)
	}
	for _, d := range overlayWarnings {
		appendReportIssue(&report, d)
	}
	for _, d := range overlayErrors {
		appendReportIssue(&report, d)
	}
	for _, d := range workload.Warnings {
		appendReportIssue(&report, d)
	}
	for _, d := range workload.Errors {
		appendReportIssue(&report, d)
	}
	for _, d := range dependencyPlanningDiagnostics(ws) {
		appendReportIssue(&report, d)
	}
	for _, d := range taskPlanningDiagnostics(ws) {
		appendReportIssue(&report, d)
	}

	sort.Slice(report.Proposals, func(i, j int) bool { return report.Proposals[i].ID < report.Proposals[j].ID })
	sort.Slice(report.Evidence, func(i, j int) bool { return report.Evidence[i].ID < report.Evidence[j].ID })

	report.Summary.ResourceRows = len(report.Resources)
	report.Summary.WarningCount = len(report.Warnings)
	report.Summary.ErrorCount = len(report.Errors)
	report.Summary.OpenProposals = len(report.Proposals)
	report.Summary.EvidenceCount = len(report.Evidence)

	if err != nil {
		return report, err
	}
	return report, nil
}

func (s ReportService) Dashboard(ctx context.Context, req DashboardReportRequest) (reports.DashboardOverview, error) {
	ws, loadDiags := s.loader.Load(ctx)
	canonicalWS := ws
	generatedAt := req.GeneratedAt
	if generatedAt.IsZero() {
		generatedAt = time.Now()
	}
	if hasDiagnosticCode(loadDiags, "project_format_unsupported") {
		overview := reports.DashboardOverview{
			SchemaVersion: 1,
			GeneratedAt:   generatedAt.Format(time.RFC3339),
			ProjectPath:   s.loader.ProjectPath(),
			Week:          req.Week,
			Warnings:      []reports.ReportIssue{},
			Errors:        []reports.ReportIssue{},
		}
		for _, diagnostic := range loadDiags {
			overview.Errors = append(overview.Errors, diagnosticToReportIssue(diagnostic))
		}
		overview.Health.ErrorCount = len(overview.Errors)
		return overview, ErrValidationFailed
	}
	overlayErrors := []Diagnostic{}
	if req.ScenarioID != "" {
		overlay := ApplyScenarioOverlay(ws, req.ScenarioID)
		overlayErrors = overlay.Errors
		if len(overlayErrors) == 0 {
			ws = overlay.Workspace
		}
	}
	week := req.Week
	if week == "" {
		week = defaultDashboardWeek(ws)
	}
	weekly, err := s.Weekly(ctx, WeeklyReportRequest{
		ProjectPath: req.ProjectPath,
		Week:        week,
		ScenarioID:  req.ScenarioID,
	})
	overview := reports.DashboardOverview{
		SchemaVersion: 1,
		GeneratedAt:   generatedAt.Format(time.RFC3339),
		ProjectPath:   s.loader.ProjectPath(),
		ProjectID:     ws.Project.ID,
		ProjectName:   ws.Project.Name,
		StatusDate:    ws.Project.DefaultStatusDate,
		Week:          week,
		ScenarioID:    req.ScenarioID,
		CurrentPlan:   ws.CurrentPlan.ID,
		BaselineID:    weekly.BaselineID,
		SourceHealth:  dashboardSourceHealth(ws),
		Health: reports.DashboardHealthSummary{
			TaskCount:              len(ws.Tasks),
			ResolvedDependencies:   dependencyStatusCount(ws.Dependencies, "resolved"),
			OpenDependencies:       dependencyStatusCount(ws.Dependencies, "open"),
			OverloadedResources:    weekly.Summary.Overloaded,
			WarningCount:           len(weekly.Warnings),
			ErrorCount:             len(weekly.Errors),
			OpenProposals:          len(weekly.Proposals),
			EvidenceCount:          len(weekly.Evidence),
			ScenarioCount:          len(ws.Scenarios),
			MissingDemandTaskCount: missingDemandTaskCount(ws.Tasks),
		},
		Workload:     weekly.Resources,
		Tasks:        dashboardTaskRows(ws.Tasks, weekly.Warnings),
		Tracking:     dashboardTrackingRows(ws),
		Dependencies: dashboardDependencyRows(ws.Dependencies),
		Warnings:     weekly.Warnings,
		Errors:       weekly.Errors,
		Proposals:    weekly.Proposals,
		Evidence:     weekly.Evidence,
		Scenarios:    dashboardScenarioRows(ws.Scenarios),
	}
	overview.TrackingLoop = dashboardTrackingProposalLoop(overview.Tracking, ws.Proposals, s.loader.ProjectPath())
	overview.Gantt = dashboardGanttView(ws, overview, weekly.Resources, weekly.Warnings, weekly.Proposals)
	overview.OpenProject = dashboardOpenProject(overview.SourceHealth, overview.Gantt)
	for _, task := range ws.Tasks {
		switch task.TrackingStatus {
		case "accepted":
			overview.Health.AcceptedTasks++
		case "in_progress", "ready", "review_ready", "blocked":
			overview.Health.ActiveTasks++
		}
	}
	for _, d := range loadDiags {
		if d.Severity == "warning" {
			overview.Warnings = append(overview.Warnings, diagnosticToReportIssue(d))
		} else {
			overview.Errors = append(overview.Errors, diagnosticToReportIssue(d))
		}
	}
	if len(overlayErrors) > 0 {
		err = ErrValidationFailed
	}
	overview.Health.WarningCount = len(overview.Warnings)
	overview.Health.ErrorCount = len(overview.Errors)
	overview.NextActions = dashboardNextActions(ws, overview)
	overview.WorkloadPage, overview.ScenariosPage, overview.ProposalsPage, overview.EstimatesPage = s.dashboardPages(ctx, canonicalWS, overview)
	// The overlay comes last: it carries the final warnings and the person
	// actions to the OpenProject plugin.
	overview.OpenProjectOverlay = dashboardOpenProjectOverlay(ws, overview)
	return overview, err
}

func dashboardTrackingRows(ws domain.Workspace) []reports.DashboardTrackingRow {
	index := newDashboardTrackingIndex(ws)
	rows := make([]reports.DashboardTrackingRow, 0, len(ws.Tasks))
	for _, task := range ws.Tasks {
		rows = append(rows, dashboardTrackingRow(task, index))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].TaskID < rows[j].TaskID })
	return rows
}

type dashboardTrackingIndex struct {
	demandsByTaskID   map[string][]domain.ResourceDemand
	actualsByTaskID   map[string][]domain.Actual
	actualsByDemandID map[string][]domain.Actual
	evidenceByTaskID  map[string]map[string]bool
}

func newDashboardTrackingIndex(ws domain.Workspace) dashboardTrackingIndex {
	index := dashboardTrackingIndex{
		demandsByTaskID:   map[string][]domain.ResourceDemand{},
		actualsByTaskID:   map[string][]domain.Actual{},
		actualsByDemandID: map[string][]domain.Actual{},
		evidenceByTaskID:  map[string]map[string]bool{},
	}
	for _, demand := range ws.ResourceDemands {
		index.demandsByTaskID[demand.TaskRef.ID] = append(index.demandsByTaskID[demand.TaskRef.ID], demand)
	}
	for _, actual := range ws.Actuals {
		if actual.TaskRef.ID != "" {
			index.actualsByTaskID[actual.TaskRef.ID] = append(index.actualsByTaskID[actual.TaskRef.ID], actual)
		}
		if actual.DemandRef.ID != "" {
			index.actualsByDemandID[actual.DemandRef.ID] = append(index.actualsByDemandID[actual.DemandRef.ID], actual)
		}
	}
	for _, evidence := range ws.Evidence {
		for _, target := range evidence.Targets {
			if target.Kind != "task" || target.ID == "" {
				continue
			}
			if index.evidenceByTaskID[target.ID] == nil {
				index.evidenceByTaskID[target.ID] = map[string]bool{}
			}
			index.evidenceByTaskID[target.ID][evidence.ID] = true
		}
	}
	for _, actual := range ws.Actuals {
		if actual.TaskRef.ID == "" {
			continue
		}
		if index.evidenceByTaskID[actual.TaskRef.ID] == nil {
			index.evidenceByTaskID[actual.TaskRef.ID] = map[string]bool{}
		}
		for _, evidenceRef := range actual.EvidenceRefs {
			if evidenceRef.ID != "" {
				index.evidenceByTaskID[actual.TaskRef.ID][evidenceRef.ID] = true
			}
		}
	}

	return index
}

func dashboardTrackingRow(task domain.Task, index dashboardTrackingIndex) reports.DashboardTrackingRow {
	row := reports.DashboardTrackingRow{
		TaskID:         task.ID,
		Title:          task.Title,
		TrackingStatus: task.TrackingStatus,
		PlannedStart:   task.PlannedStart,
		PlannedFinish:  task.PlannedFinish,
		ActualCount:    len(index.actualsByTaskID[task.ID]),
		EvidenceCount:  len(index.evidenceByTaskID[task.ID]),
		Demands:        []reports.DashboardDemandTracking{},
	}
	actualsMissingEvidence := false
	for _, actual := range index.actualsByTaskID[task.ID] {
		if len(actual.EvidenceRefs) == 0 {
			actualsMissingEvidence = true
			break
		}
	}
	for _, demand := range index.demandsByTaskID[task.ID] {
		row.Demands = append(row.Demands, dashboardDemandTracking(demand, index.actualsByDemandID[demand.ID]))
	}
	if row.ActualCount == 0 && expectsActuals(row) {
		row.Flags = append(row.Flags, "no_actuals")
	}
	if isAcceptedTaskStatus(task.TrackingStatus) && row.ActualCount == 0 && expectsActuals(row) {
		row.Flags = append(row.Flags, "accepted_without_actuals")
	}
	if actualsMissingEvidence {
		row.Flags = append(row.Flags, "actuals_without_evidence")
	}
	for _, demand := range row.Demands {
		for _, flag := range demand.Flags {
			if flag == "over_consumed" || flag == "forecast_over_plan" {
				row.Flags = appendUniqueString(row.Flags, flag)
			}
		}
	}
	sort.Slice(row.Demands, func(i, j int) bool {
		if row.Demands[i].ResourceID == row.Demands[j].ResourceID {
			return row.Demands[i].DemandID < row.Demands[j].DemandID
		}
		return row.Demands[i].ResourceID < row.Demands[j].ResourceID
	})
	return row
}

func dashboardDemandTracking(demand domain.ResourceDemand, actuals []domain.Actual) reports.DashboardDemandTracking {
	actualAmount := 0.0
	evidenceIDs := map[string]bool{}
	for _, actual := range actuals {
		if actual.Unit == "" || actual.Unit == demand.Unit {
			actualAmount += actual.Amount
		}
		for _, evidenceRef := range actual.EvidenceRefs {
			if evidenceRef.ID != "" {
				evidenceIDs[evidenceRef.ID] = true
			}
		}
	}
	forecast := actualAmount + demand.Remaining()
	variance := forecast - demand.PlannedAmount
	flags := []string{}
	if len(actuals) == 0 && demand.PlannedAmount > 0 {
		flags = append(flags, "no_actuals")
	}
	if actualAmount > demand.PlannedAmount && demand.PlannedAmount > 0 {
		flags = append(flags, "over_consumed")
	}
	if demand.Status == "cancelled" {
		flags = append(flags, "cancelled")
	}
	if variance > 0 {
		flags = append(flags, "forecast_over_plan")
		if overrunAcknowledged(demand.OverrunReview, demand.Unit, forecast) {
			flags = append(flags, "overrun_acknowledged")
		}
	}
	return reports.DashboardDemandTracking{
		DemandID: demand.ID, ResourceID: demandResourceID(demand), ResourceClass: demand.ResourceClass,
		Role: demand.Role, Unit: demand.Unit, Planned: demand.PlannedAmount, Actual: actualAmount,
		Remaining: demand.Remaining(), Forecast: forecast, Variance: variance,
		EvidenceCount: len(evidenceIDs), Flags: flags,
	}
}

func isAcceptedTaskStatus(status string) bool {
	return status == "accepted"
}

func dashboardTrackingProposalLoop(tracking []reports.DashboardTrackingRow, proposals []domain.Proposal, projectPath string) reports.DashboardTrackingLoop {
	loop := reports.DashboardTrackingLoop{
		Rows: []reports.DashboardTrackingLoopRow{},
	}
	trackingProposalByDemandID := map[string]domain.Proposal{}
	for _, proposal := range proposals {
		if proposal.Type != "update_remaining_work" || proposal.TargetRef.Kind != "resource_demand" || proposal.TargetRef.ID == "" {
			continue
		}
		if !strings.HasPrefix(proposal.ID, "proposal.tracking.") {
			continue
		}
		if proposal.Status == "open" {
			loop.OpenProposalCount++
		}
		if proposal.Status == "accepted" {
			loop.AcceptedProposalCount++
		}
		existing, exists := trackingProposalByDemandID[proposal.TargetRef.ID]
		if !exists || trackingProposalStatusRank(proposal.Status) > trackingProposalStatusRank(existing.Status) {
			trackingProposalByDemandID[proposal.TargetRef.ID] = proposal
		}
	}
	for _, row := range tracking {
		for _, demand := range row.Demands {
			if !isAcceptedTaskStatus(row.TrackingStatus) || demand.Actual <= 0 || demand.Remaining <= 0 || demand.Variance <= 0 {
				continue
			}
			loop.StaleRemainingCount++
			proposal := trackingProposalByDemandID[demand.DemandID]
			loopRow := reports.DashboardTrackingLoopRow{
				TaskID:     row.TaskID,
				TaskTitle:  row.Title,
				DemandID:   demand.DemandID,
				ResourceID: demand.ResourceID,
				Unit:       demand.Unit,
				Planned:    demand.Planned,
				Actual:     demand.Actual,
				Remaining:  demand.Remaining,
				Forecast:   demand.Forecast,
				Variance:   demand.Variance,
			}
			if proposal.ID != "" {
				loopRow.ProposalID = proposal.ID
				loopRow.ProposalStatus = proposal.Status
				loopRow.Status = proposal.Status
				loop.CoveredDemandCount++
				switch proposal.Status {
				case "open":
					loopRow.Priority = "medium"
					loopRow.Reason = "proposal already open"
					loopRow.RecommendedCommand = fmt.Sprintf("replan proposal accept --project %s %s --dry-run --format json", projectPath, proposal.ID)
				case "accepted":
					loopRow.Priority = "low"
					loopRow.Reason = "proposal accepted but stale row still present"
				default:
					loopRow.Priority = "low"
					loopRow.Reason = "proposal already exists"
				}
			} else {
				loopRow.Status = "actionable"
				loopRow.Priority = "high"
				loopRow.Reason = "accepted task has stale remaining and forecast is over planned"
				loop.ActionableCount++
				loopRow.RecommendedCommand = fmt.Sprintf("replan tracking suggest --project %s --write-candidates --actor <actor-id> --format json", projectPath)
			}
			loop.Rows = append(loop.Rows, loopRow)
		}
	}
	sort.Slice(loop.Rows, func(i, j int) bool {
		if loop.Rows[i].Status != loop.Rows[j].Status {
			return trackingLoopRowStatusRank(loop.Rows[i].Status) < trackingLoopRowStatusRank(loop.Rows[j].Status)
		}
		if loop.Rows[i].Priority != loop.Rows[j].Priority {
			return trackingLoopRowPriorityRank(loop.Rows[i].Priority) < trackingLoopRowPriorityRank(loop.Rows[j].Priority)
		}
		if loop.Rows[i].Variance != loop.Rows[j].Variance {
			return loop.Rows[i].Variance > loop.Rows[j].Variance
		}
		if loop.Rows[i].Remaining != loop.Rows[j].Remaining {
			return loop.Rows[i].Remaining > loop.Rows[j].Remaining
		}
		if loop.Rows[i].TaskID != loop.Rows[j].TaskID {
			return loop.Rows[i].TaskID < loop.Rows[j].TaskID
		}
		return loop.Rows[i].DemandID < loop.Rows[j].DemandID
	})
	return loop
}

func trackingProposalStatusRank(status string) int {
	switch status {
	case "accepted":
		return 3
	case "open":
		return 2
	default:
		return 1
	}
}

func trackingLoopRowStatusRank(status string) int {
	switch status {
	case "actionable", "":
		return 0
	case "open":
		return 1
	case "accepted":
		return 2
	default:
		return 3
	}
}

func trackingLoopRowPriorityRank(priority string) int {
	switch priority {
	case "high":
		return 0
	case "medium":
		return 1
	case "low":
		return 2
	default:
		return 3
	}
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func appendReportIssue(report *reports.WeeklyReport, d Diagnostic) {
	issue := diagnosticToReportIssue(d)
	if d.Severity == "warning" {
		report.Warnings = append(report.Warnings, issue)
		return
	}
	report.Errors = append(report.Errors, issue)
}

func diagnosticToReportIssue(d Diagnostic) reports.ReportIssue {
	return reports.ReportIssue{
		Code:     d.Code,
		Message:  d.Message,
		Severity: d.Severity,
		TargetID: d.Target.ID,
	}
}

// defaultDashboardWeek is the week of the project status date. The report
// baseline's status date is only a fallback: it records when the plan was
// frozen, and reading it first showed a July week at a September status date.
func defaultDashboardWeek(ws domain.Workspace) string {
	if week := isoWeekString(ws.Project.DefaultStatusDate); week != "" {
		return week
	}
	if baseline, ok := reportBaseline(ws); ok {
		return isoWeekString(baseline.StatusDate)
	}
	return ""
}

// reportBaseline resolves the baseline reports compare against. An explicit
// current-plan pin wins. Legacy plans without a pin use the latest approval,
// then creation time, then id; file load order is never significant.
func reportBaseline(ws domain.Workspace) (domain.Baseline, bool) {
	if id := ws.CurrentPlan.BasedOnBaseline.ID; id != "" {
		for _, baseline := range ws.Baselines {
			if baseline.ID == id {
				return baseline, true
			}
		}
	}
	if len(ws.Baselines) == 0 {
		return domain.Baseline{}, false
	}
	latest := ws.Baselines[0]
	for _, baseline := range ws.Baselines[1:] {
		if baselineLaterThan(baseline, latest) {
			latest = baseline
		}
	}
	return latest, true
}

func baselineLaterThan(candidate, current domain.Baseline) bool {
	if comparison := compareBaselineTimestamp(candidate.ApprovedAt, current.ApprovedAt); comparison != 0 {
		return comparison > 0
	}
	if comparison := compareBaselineTimestamp(candidate.CreatedAt, current.CreatedAt); comparison != 0 {
		return comparison > 0
	}
	return candidate.ID > current.ID
}

func compareBaselineTimestamp(left, right string) int {
	leftTime, leftErr := time.Parse(time.RFC3339, left)
	rightTime, rightErr := time.Parse(time.RFC3339, right)
	switch {
	case leftErr == nil && rightErr == nil:
		return leftTime.Compare(rightTime)
	case leftErr == nil:
		return 1
	case rightErr == nil:
		return -1
	default:
		return strings.Compare(left, right)
	}
}

func isoWeekString(date string) string {
	if date == "" {
		return ""
	}
	t, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ""
	}
	year, week := t.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func dashboardSourceHealth(ws domain.Workspace) reports.DashboardSourceHealth {
	health := reports.DashboardSourceHealth{
		Profiles:   []reports.DashboardSourceProfile{},
		SourceRefs: len(ws.Project.SourceRefs),
	}
	for _, profile := range ws.Project.SourceProfiles {
		health.Profiles = append(health.Profiles, reports.DashboardSourceProfile{
			ID:        profile.ID,
			Source:    profile.Source,
			BaseURL:   profile.BaseURL,
			ProjectID: profile.ProjectID,
		})
	}
	for _, task := range ws.Tasks {
		if len(task.SourceRefs) > 0 {
			health.TasksWithSourceRefs++
			for _, sourceRef := range task.SourceRefs {
				health.LinkedTasks = append(health.LinkedTasks, reports.DashboardSourceTask{
					TaskID:         task.ID,
					Title:          task.Title,
					Source:         sourceRef.Source,
					SourceID:       sourceRef.ObjectID,
					SourceURL:      sourceRef.URL,
					TrackingStatus: task.TrackingStatus,
				})
			}
			continue
		}
		health.TasksWithoutSourceRefs++
		health.UnlinkedTasks = append(health.UnlinkedTasks, reports.DashboardSourceTask{
			TaskID:         task.ID,
			Title:          task.Title,
			TrackingStatus: task.TrackingStatus,
		})
	}
	for _, resource := range ws.Resources {
		if len(resource.SourceRefs) > 0 {
			health.ResourcesWithSourceRefs++
		}
	}
	for _, dependency := range ws.Dependencies {
		if len(dependency.SourceRefs) > 0 {
			health.DependenciesWithSourceRefs++
		}
	}
	sort.Slice(health.Profiles, func(i, j int) bool { return health.Profiles[i].ID < health.Profiles[j].ID })
	sort.Slice(health.LinkedTasks, func(i, j int) bool {
		if health.LinkedTasks[i].Source == health.LinkedTasks[j].Source {
			return health.LinkedTasks[i].TaskID < health.LinkedTasks[j].TaskID
		}
		return health.LinkedTasks[i].Source < health.LinkedTasks[j].Source
	})
	sort.Slice(health.UnlinkedTasks, func(i, j int) bool { return health.UnlinkedTasks[i].TaskID < health.UnlinkedTasks[j].TaskID })
	return health
}

func dependencyStatusCount(dependencies []domain.Dependency, status string) int {
	count := 0
	for _, dependency := range dependencies {
		if dependency.Status == status {
			count++
		}
	}
	return count
}

func missingDemandTaskCount(tasks []domain.Task) int {
	count := 0
	for _, task := range tasks {
		if len(task.ResourceDemands) == 0 {
			count++
		}
	}
	return count
}

func dashboardTaskRows(tasks []domain.Task, warnings []reports.ReportIssue) []reports.DashboardTaskRow {
	warningsByTarget := map[string]int{}
	for _, warning := range warnings {
		warningsByTarget[warning.TargetID]++
	}
	rows := make([]reports.DashboardTaskRow, 0, len(tasks))
	for _, task := range tasks {
		rows = append(rows, reports.DashboardTaskRow{
			ID:                  task.ID,
			Title:               task.Title,
			Type:                task.Type,
			TrackingStatus:      task.TrackingStatus,
			PlannedStart:        task.PlannedStart,
			PlannedFinish:       task.PlannedFinish,
			AssigneeID:          task.AssigneeHint.ID,
			ResourceDemandCount: len(task.ResourceDemands),
			DependencyCount:     len(task.Dependencies),
			WarningCount:        warningsByTarget[task.ID],
			SourceRefCount:      len(task.SourceRefs),
			Tags:                append([]string{}, task.Tags...),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func dashboardOpenProject(sourceHealth reports.DashboardSourceHealth, gantt reports.DashboardGanttView) reports.DashboardOpenProject {
	taskByID := map[string]reports.DashboardGanttTask{}
	for _, task := range gantt.Tasks {
		taskByID[task.ID] = task
	}
	for _, task := range gantt.Unscheduled {
		taskByID[task.ID] = task
	}

	view := reports.DashboardOpenProject{
		TaskCount: sourceHealth.TasksWithSourceRefs + sourceHealth.TasksWithoutSourceRefs,
	}
	for _, profile := range sourceHealth.Profiles {
		if profile.Source == "openproject" {
			view.Enabled = true
			view.ProjectID = profile.ProjectID
			break
		}
	}
	for _, task := range sourceHealth.LinkedTasks {
		if task.Source != "openproject" {
			continue
		}
		view.Enabled = true
		if task.SourceURL != "" {
			view.WorkPackageURLCount++
		}
		ganttTask := taskByID[task.TaskID]
		if len(ganttTask.ResourceIDs) > 0 {
			view.TasksWithResourceCount++
		}
		view.Links = append(view.Links, reports.DashboardOpenProjectLink{
			TaskID:              task.TaskID,
			Title:               task.Title,
			TrackingStatus:      task.TrackingStatus,
			OpenProjectID:       task.SourceID,
			OpenProjectURL:      task.SourceURL,
			PlannedStart:        ganttTask.PlannedStart,
			PlannedFinish:       ganttTask.PlannedFinish,
			ResourceIDs:         append([]string{}, ganttTask.ResourceIDs...),
			ResourceDemandCount: len(ganttTask.Allocations),
			WarningCount:        len(ganttTask.WarningCodes),
			OpenDependencyCount: ganttTask.OpenDependencyCount,
		})
	}
	for _, task := range sourceHealth.UnlinkedTasks {
		view.MissingLinks = append(view.MissingLinks, reports.DashboardOpenProjectMissing{
			TaskID:         task.TaskID,
			Title:          task.Title,
			TrackingStatus: task.TrackingStatus,
		})
	}
	view.LinkedTaskCount = len(view.Links)
	view.MissingLinkCount = len(view.MissingLinks)
	sort.Slice(view.Links, func(i, j int) bool {
		if view.Links[i].OpenProjectID == view.Links[j].OpenProjectID {
			return view.Links[i].TaskID < view.Links[j].TaskID
		}
		return view.Links[i].OpenProjectID < view.Links[j].OpenProjectID
	})
	sort.Slice(view.MissingLinks, func(i, j int) bool { return view.MissingLinks[i].TaskID < view.MissingLinks[j].TaskID })
	return view
}

// expectsActuals reports whether a task row has at least one demand that is
// not cancelled. A task whose demands were all cancelled expects no actuals.
func expectsActuals(row reports.DashboardTrackingRow) bool {
	for _, demand := range row.Demands {
		cancelled := false
		for _, flag := range demand.Flags {
			if flag == "cancelled" {
				cancelled = true
				break
			}
		}
		if !cancelled {
			return true
		}
	}
	return false
}

func firstNonZero(values ...float64) float64 {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func dashboardGanttView(ws domain.Workspace, overview reports.DashboardOverview, workload []reports.ResourceRow, warnings []reports.ReportIssue, proposals []reports.ProposalRow) reports.DashboardGanttView {
	view := reports.DashboardGanttView{
		SchemaVersion: 1,
		ProjectID:     ws.Project.ID,
		ProjectName:   ws.Project.Name,
		StatusDate:    ws.Project.DefaultStatusDate,
		Week:          overview.Week,
		Tasks:         []reports.DashboardGanttTask{},
		Unscheduled:   []reports.DashboardGanttTask{},
	}

	demandsByID := map[string]domain.ResourceDemand{}
	for _, demand := range ws.ResourceDemands {
		demandsByID[demand.ID] = demand
	}
	overloadedResources := map[string]bool{}
	for _, row := range workload {
		if row.Overload > 0 {
			overloadedResources[row.ResourceID] = true
		}
	}
	warningsByTaskID := map[string][]string{}
	for _, warning := range warnings {
		warningsByTaskID[warning.TargetID] = append(warningsByTaskID[warning.TargetID], warning.Code)
	}
	dependencyCountByTaskID := map[string]int{}
	openDependencyCountByTaskID := map[string]int{}
	for _, dependency := range ws.Dependencies {
		taskID := dependency.SuccessorRef.ID
		if taskID == "" {
			continue
		}
		dependencyCountByTaskID[taskID]++
		if dependency.Status == "open" {
			openDependencyCountByTaskID[taskID]++
		}
		if dependency.Status != "resolved" {
			warningsByTaskID[taskID] = append(warningsByTaskID[taskID], "dependency_"+dependency.Status)
		}
	}
	openProposalByTaskID := map[string]bool{}
	for _, proposal := range proposals {
		if proposal.Status == "open" && proposal.TargetID != "" {
			openProposalByTaskID[proposal.TargetID] = true
		}
	}

	var minStart time.Time
	var maxFinish time.Time
	dated := []struct {
		task   domain.Task
		start  time.Time
		finish time.Time
	}{}
	for _, task := range ws.Tasks {
		start, startErr := time.Parse("2006-01-02", task.PlannedStart)
		finish, finishErr := time.Parse("2006-01-02", task.PlannedFinish)
		if startErr != nil || finishErr != nil {
			view.Unscheduled = append(view.Unscheduled, dashboardGanttTask(task, demandsByID, overloadedResources, warningsByTaskID, dependencyCountByTaskID, openDependencyCountByTaskID, openProposalByTaskID))
			continue
		}
		if finish.Before(start) {
			finish = start
		}
		if minStart.IsZero() || start.Before(minStart) {
			minStart = start
		}
		if maxFinish.IsZero() || finish.After(maxFinish) {
			maxFinish = finish
		}
		dated = append(dated, struct {
			task   domain.Task
			start  time.Time
			finish time.Time
		}{task: task, start: start, finish: finish})
	}

	sort.Slice(dated, func(i, j int) bool {
		if dated[i].start.Equal(dated[j].start) {
			if dated[i].finish.Equal(dated[j].finish) {
				return dated[i].task.ID < dated[j].task.ID
			}
			return dated[i].finish.Before(dated[j].finish)
		}
		return dated[i].start.Before(dated[j].start)
	})
	for _, item := range dated {
		row := dashboardGanttTask(item.task, demandsByID, overloadedResources, warningsByTaskID, dependencyCountByTaskID, openDependencyCountByTaskID, openProposalByTaskID)
		row.StartOffsetDays = int(item.start.Sub(minStart).Hours() / 24)
		row.DurationDays = int(item.finish.Sub(item.start).Hours()/24) + 1
		if row.DurationDays < 1 {
			row.DurationDays = 1
		}
		row.Allocations = dashboardGanttAllocations(ws, item.task, demandsByID, workload, overloadedResources, minStart)
		view.Tasks = append(view.Tasks, row)
	}
	sort.Slice(view.Unscheduled, func(i, j int) bool { return view.Unscheduled[i].ID < view.Unscheduled[j].ID })

	if !minStart.IsZero() {
		view.StartDate = minStart.Format("2006-01-02")
		view.FinishDate = maxFinish.Format("2006-01-02")
		view.DayCount = int(maxFinish.Sub(minStart).Hours()/24) + 1
		if statusDate, err := time.Parse("2006-01-02", view.StatusDate); err == nil && !statusDate.Before(minStart) && !statusDate.After(maxFinish) {
			view.StatusOffset = int(statusDate.Sub(minStart).Hours() / 24)
			view.StatusInRange = true
		}
	}
	view.TaskCount = len(view.Tasks) + len(view.Unscheduled)
	view.ResourceLoads = dashboardGanttResourceLoads(ws, view, workload)
	return view
}

func dashboardGanttTask(
	task domain.Task,
	demandsByID map[string]domain.ResourceDemand,
	overloadedResources map[string]bool,
	warningsByTaskID map[string][]string,
	dependencyCountByTaskID map[string]int,
	openDependencyCountByTaskID map[string]int,
	openProposalByTaskID map[string]bool,
) reports.DashboardGanttTask {
	resourceSet := map[string]bool{}
	overloadedSet := map[string]bool{}
	for _, demandRef := range task.ResourceDemands {
		demand := demandsByID[demandRef.ID]
		resourceID := demandResourceID(demand)
		if resourceID == "" {
			continue
		}
		resourceSet[resourceID] = true
		if overloadedResources[resourceID] {
			overloadedSet[resourceID] = true
		}
	}
	resourceIDs := sortedMapKeys(resourceSet)
	overloadedResourceIDs := sortedMapKeys(overloadedSet)
	warningCodes := append([]string{}, warningsByTaskID[task.ID]...)
	sort.Strings(warningCodes)
	source := domain.SourceRef{}
	for _, sourceRef := range task.SourceRefs {
		source = sourceRef
		if sourceRef.Source == "openproject" {
			break
		}
	}
	return reports.DashboardGanttTask{
		ID:                    task.ID,
		Title:                 task.Title,
		Type:                  task.Type,
		TrackingStatus:        task.TrackingStatus,
		PlannedStart:          task.PlannedStart,
		PlannedFinish:         task.PlannedFinish,
		ResourceIDs:           resourceIDs,
		OverloadedResourceIDs: overloadedResourceIDs,
		WarningCodes:          warningCodes,
		DependencyCount:       dependencyCountByTaskID[task.ID],
		OpenDependencyCount:   openDependencyCountByTaskID[task.ID],
		HasOpenProposal:       openProposalByTaskID[task.ID],
		Source:                source.Source,
		SourceID:              source.ObjectID,
		SourceURL:             source.URL,
	}
}

// dashboardGanttAllocations places each demand of a task on the Gantt. A
// period demand's amount per day is spread over its working days, a
// concurrent demand occupies its full amount every day (GAP-031).
func dashboardGanttAllocations(ws domain.Workspace, task domain.Task, demandsByID map[string]domain.ResourceDemand, workload []reports.ResourceRow, overloadedResources map[string]bool, minStart time.Time) []reports.DashboardGanttResourceAllocation {
	days := workingDays(ws.Availabilities)
	allocations := []reports.DashboardGanttResourceAllocation{}
	capacityByResourceUnit := map[string]float64{}
	for _, row := range workload {
		capacityByResourceUnit[resourceUnitKey(row.ResourceID, row.Unit)] = row.Capacity
	}
	for _, demandRef := range task.ResourceDemands {
		demand := demandsByID[demandRef.ID]
		resourceID := demandResourceID(demand)
		if resourceID == "" {
			continue
		}
		windowSource := "task_window"
		if demandHasExplicitTiming(demand) {
			windowSource = "explicit_demand_window"
		}
		startDate := firstNonEmpty(demand.Timing.PreferredStart, demand.Timing.EarliestStart, task.PlannedStart)
		finishDate := firstNonEmpty(demand.Timing.PreferredFinish, demand.Timing.LatestFinish, task.PlannedFinish, startDate)
		start, startErr := time.Parse("2006-01-02", startDate)
		finish, finishErr := time.Parse("2006-01-02", finishDate)
		if startErr != nil || finishErr != nil {
			continue
		}
		if finish.Before(start) {
			finish = start
			finishDate = startDate
		}
		duration := int(finish.Sub(start).Hours()/24) + 1
		if duration < 1 {
			duration = 1
		}
		amountPerDay := 0.0
		switch {
		case demand.PlannedAmount <= 0:
		case isConcurrentResourceUnit(ws.Capacities, resourceID, demand.Unit):
			amountPerDay = demand.PlannedAmount
		default:
			spread := spreadDays(demandSpan{start: start, finish: finish, dated: true}, days[resourceID])
			amountPerDay = demand.PlannedAmount / float64(len(spread))
		}
		weeklyCapacity := capacityByResourceUnit[resourceUnitKey(resourceID, demand.Unit)]
		windowCapacity := 0.0
		intensityRatio := 0.0
		capacityKnown := weeklyCapacity > 0
		if capacityKnown {
			windowWeeks := float64(duration) / 7
			if windowWeeks < 1 {
				windowWeeks = 1
			}
			windowCapacity = weeklyCapacity * windowWeeks
			if windowCapacity > 0 {
				intensityRatio = demand.PlannedAmount / windowCapacity
			}
		}
		allocations = append(allocations, reports.DashboardGanttResourceAllocation{
			DemandID:                  demand.ID,
			ResourceID:                resourceID,
			ResourceClass:             demand.ResourceClass,
			Role:                      demand.Role,
			Unit:                      demand.Unit,
			PlannedAmount:             demand.PlannedAmount,
			RemainingAmount:           demand.Remaining(),
			StartDate:                 startDate,
			FinishDate:                finishDate,
			StartOffsetDays:           int(start.Sub(minStart).Hours() / 24),
			DurationDays:              duration,
			ParticipationWindowSource: windowSource,
			WindowCapacity:            roundDashboardFloat(windowCapacity),
			IntensityRatio:            roundDashboardFloat(intensityRatio),
			IntensityPercent:          roundDashboardFloat(intensityRatio * 100),
			CapacityKnown:             capacityKnown,
			AmountPerDay:              amountPerDay,
			Overloaded:                overloadedResources[resourceID],
			OverCapacity:              intensityRatio > 1,
		})
	}
	sort.Slice(allocations, func(i, j int) bool {
		if allocations[i].StartOffsetDays == allocations[j].StartOffsetDays {
			if allocations[i].ResourceID == allocations[j].ResourceID {
				return allocations[i].DemandID < allocations[j].DemandID
			}
			return allocations[i].ResourceID < allocations[j].ResourceID
		}
		return allocations[i].StartOffsetDays < allocations[j].StartOffsetDays
	})
	return allocations
}

func dashboardGanttResourceLoads(ws domain.Workspace, view reports.DashboardGanttView, workload []reports.ResourceRow) []reports.DashboardGanttResourceLoad {
	if view.DayCount <= 0 {
		return nil
	}
	workloadByResourceID := map[string]reports.ResourceRow{}
	for _, row := range workload {
		workloadByResourceID[row.ResourceID] = row
	}
	resourceByID := map[string]domain.Resource{}
	for _, resource := range ws.Resources {
		resourceByID[resource.ID] = resource
	}
	days := workingDays(ws.Availabilities)
	viewStart, _ := time.Parse("2006-01-02", view.StartDate)
	loadByResourceID := map[string]*reports.DashboardGanttResourceLoad{}
	for _, task := range view.Tasks {
		for _, allocation := range task.Allocations {
			load := loadByResourceID[allocation.ResourceID]
			if load == nil {
				resource := resourceByID[allocation.ResourceID]
				weekly := workloadByResourceID[allocation.ResourceID]
				capacityPerDay := 0.0
				if weekly.Capacity > 0 {
					capacityPerDay = weekly.Capacity / 7
				}
				load = &reports.DashboardGanttResourceLoad{
					ResourceID:     allocation.ResourceID,
					ResourceName:   resource.Name,
					ResourceClass:  firstNonEmpty(resource.Class, allocation.ResourceClass, weekly.ResourceClass),
					Unit:           firstNonEmpty(allocation.Unit, weekly.Unit),
					Capacity:       weekly.Capacity,
					Planned:        weekly.Planned,
					Overload:       weekly.Overload,
					BucketType:     "week",
					WaterlineRatio: 1,
					DailyAmounts:   make([]float64, view.DayCount),
					CapacityPerDay: capacityPerDay,
				}
				load.Concurrent = isConcurrentResourceUnit(ws.Capacities, allocation.ResourceID, load.Unit)
				loadByResourceID[allocation.ResourceID] = load
			}
			working := days[allocation.ResourceID]
			if load.Concurrent {
				working = nil
			} else if start, err := time.Parse("2006-01-02", allocation.StartDate); err == nil {
				// The allocation's amount per day was spread over these days.
				finish, _ := time.Parse("2006-01-02", allocation.FinishDate)
				if len(spreadDays(demandSpan{start: start, finish: finish, dated: true}, working)) == allocation.DurationDays {
					working = nil
				}
			}
			for day := 0; day < allocation.DurationDays; day++ {
				index := allocation.StartOffsetDays + day
				if index < 0 || index >= view.DayCount {
					continue
				}
				if working != nil && !working[viewStart.AddDate(0, 0, index).Weekday()] {
					continue
				}
				load.DailyAmounts[index] += allocation.AmountPerDay
				if load.DailyAmounts[index] > load.PeakDaily {
					load.PeakDaily = load.DailyAmounts[index]
				}
			}
		}
	}
	loads := make([]reports.DashboardGanttResourceLoad, 0, len(loadByResourceID))
	for _, load := range loadByResourceID {
		load.Buckets = dashboardGanttWeeklyLoadBuckets(ws, view, *load)
		load.MaxUtilizationRatio = 1
		for _, bucket := range load.Buckets {
			if bucket.UtilizationRatio > load.MaxUtilizationRatio {
				load.MaxUtilizationRatio = bucket.UtilizationRatio
			}
		}
		loads = append(loads, *load)
	}
	sort.Slice(loads, func(i, j int) bool {
		if loads[i].Overload == loads[j].Overload {
			return loads[i].ResourceID < loads[j].ResourceID
		}
		return loads[i].Overload > loads[j].Overload
	})
	return loads
}

// dashboardGanttWeeklyLoadBuckets sums a resource's daily load per ISO week,
// or takes the peak day for slot capacity, against the capacity in effect in
// that week, not the status week's (GAP-031).
func dashboardGanttWeeklyLoadBuckets(ws domain.Workspace, view reports.DashboardGanttView, load reports.DashboardGanttResourceLoad) []reports.DashboardGanttResourceLoadBucket {
	start, err := time.Parse("2006-01-02", view.StartDate)
	if err != nil || len(load.DailyAmounts) == 0 {
		return nil
	}
	type bucketState struct {
		start    time.Time
		finish   time.Time
		planned  float64
		dayCount int
	}
	orderedKeys := []string{}
	bucketsByKey := map[string]*bucketState{}
	for offset, amount := range load.DailyAmounts {
		day := start.AddDate(0, 0, offset)
		year, week := day.ISOWeek()
		key := fmt.Sprintf("%04d-W%02d", year, week)
		bucket := bucketsByKey[key]
		if bucket == nil {
			bucket = &bucketState{start: day, finish: day}
			bucketsByKey[key] = bucket
			orderedKeys = append(orderedKeys, key)
		}
		if day.Before(bucket.start) {
			bucket.start = day
		}
		if day.After(bucket.finish) {
			bucket.finish = day
		}
		if load.Concurrent {
			bucket.planned = max(bucket.planned, amount)
		} else {
			bucket.planned += amount
		}
		bucket.dayCount++
	}
	result := make([]reports.DashboardGanttResourceLoadBucket, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		bucket := bucketsByKey[key]
		capacity := resourceCapacityBetween(ws, load.ResourceID, load.Unit, bucket.start, bucket.finish)
		utilizationRatio := 0.0
		if capacity > 0 {
			utilizationRatio = bucket.planned / capacity
		}
		overload := 0.0
		if capacity > 0 && bucket.planned > capacity {
			overload = bucket.planned - capacity
		}
		result = append(result, reports.DashboardGanttResourceLoadBucket{
			Label:            key,
			StartDate:        bucket.start.Format("2006-01-02"),
			FinishDate:       bucket.finish.Format("2006-01-02"),
			Capacity:         roundDashboardFloat(capacity),
			Planned:          roundDashboardFloat(bucket.planned),
			UtilizationRatio: roundDashboardFloat(utilizationRatio),
			UtilizationPct:   roundDashboardFloat(utilizationRatio * 100),
			Overload:         roundDashboardFloat(overload),
			OverCapacity:     overload > 0,
		})
	}
	return result
}

func demandResourceID(demand domain.ResourceDemand) string {
	if demand.ResourceRef.ID != "" {
		return demand.ResourceRef.ID
	}
	return demand.ResourcePoolRef.ID
}

func demandHasExplicitTiming(demand domain.ResourceDemand) bool {
	return demand.Timing.PreferredStart != "" ||
		demand.Timing.PreferredFinish != "" ||
		demand.Timing.EarliestStart != "" ||
		demand.Timing.LatestFinish != ""
}

func resourceUnitKey(resourceID, unit string) string {
	return resourceID + "\x00" + unit
}

func roundDashboardFloat(value float64) float64 {
	return math.Round(value*10000) / 10000
}

func sortedMapKeys(values map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func dashboardDependencyRows(dependencies []domain.Dependency) []reports.DashboardDependency {
	rows := make([]reports.DashboardDependency, 0, len(dependencies))
	for _, dependency := range dependencies {
		rows = append(rows, reports.DashboardDependency{
			ID:            dependency.ID,
			Type:          dependency.Type,
			PredecessorID: dependency.PredecessorRef.ID,
			SuccessorID:   dependency.SuccessorRef.ID,
			Status:        dependency.Status,
			Criticality:   dependency.Criticality,
			Notes:         dependency.Notes,
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func dashboardScenarioRows(scenarios []domain.Scenario) []reports.DashboardScenarioRow {
	rows := make([]reports.DashboardScenarioRow, 0, len(scenarios))
	for _, scenario := range scenarios {
		rows = append(rows, reports.DashboardScenarioRow{
			ID:          scenario.ID,
			Name:        scenario.Name,
			Status:      scenario.Status,
			ChangeCount: len(scenario.Changes),
		})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows
}

func dashboardNextActions(ws domain.Workspace, overview reports.DashboardOverview) []reports.DashboardNextAction {
	person, agent := reports.NextActionActorPerson, reports.NextActionActorAgent
	actions := []reports.DashboardNextAction{}
	for _, group := range CalibrationOpenDemandGroups(ws) {
		details := make([]reports.CalibrationDemandDetail, 0, len(group.Demands))
		for _, value := range group.Demands {
			details = append(details, reports.CalibrationDemandDetail{DemandID: value.DemandID, TypedAmount: value.TypedAmount, CalibratedValue: value.Calibrated, Unit: value.Unit})
		}
		actions = append(actions, reports.DashboardNextAction{
			Code: "apply_calibration", Label: fmt.Sprintf("Apply calibration %s to %d open demands", group.CalibrationID, len(group.Demands)),
			Target: group.CalibrationID, Command: fmt.Sprintf("replan estimate apply --project %s --calibration %s --scenario-id <scenario-id> --actor <actor-id> --dry-run --format json", overview.ProjectPath, group.CalibrationID),
			Actor: reports.NextActionActorAgent, Details: details,
		})
	}
	for _, suggestion := range CalibrationSuggestions(ws) {
		resourceFlags := ""
		for _, resourceID := range suggestion.Resources {
			resourceFlags += " --resource " + resourceID
		}
		actions = append(actions, reports.DashboardNextAction{
			Code: "suggest_calibration", Label: fmt.Sprintf("%d clean finished tasks can calibrate this estimate scope.", suggestion.Count),
			Target:  suggestion.BaselineID + ":" + strings.Join(suggestion.Resources, ",") + ":" + suggestion.Unit,
			Command: fmt.Sprintf("replan estimate calibrate --project %s --baseline %s%s --unit %s --id <id> --name <name> --approved-by <human-actor-id> --dry-run --format markdown", overview.ProjectPath, suggestion.BaselineID, resourceFlags, suggestion.Unit),
			Actor:   reports.NextActionActorAgent,
		})
	}
	for _, row := range overview.Workload {
		if row.Overload <= 0 {
			continue
		}
		actions = append(actions, reports.DashboardNextAction{
			Code:    "review_overload",
			Label:   "Review overloaded resource capacity or remaining demand.",
			Target:  row.ResourceID,
			Command: fmt.Sprintf("replan workload --project %s --week %s --resource %s --format json", overview.ProjectPath, overview.Week, row.ResourceID),
			Actor:   person,
		})
	}
	for _, proposal := range overview.Proposals {
		actions = append(actions, reports.DashboardNextAction{
			Code:    "review_proposal",
			Label:   "Review open proposal.",
			Target:  proposal.ID,
			Command: fmt.Sprintf("replan proposal show --project %s --format markdown %s", overview.ProjectPath, proposal.ID),
			Actor:   person,
		})
	}
	actions = append(actions, waitingOnPersonActions(ws, overview.StatusDate)...)
	finishByTaskID := map[string]string{}
	for _, task := range ws.Tasks {
		finishByTaskID[task.ID] = task.PlannedFinish
	}
	for _, row := range overview.Tracking {
		if !trackingRowNeedsReview(row) {
			continue
		}
		actions = append(actions, reports.DashboardNextAction{
			Code:    "review_tracking",
			Label:   "Review actuals, remaining work and execution evidence for this task.",
			Target:  row.TaskID,
			Command: fmt.Sprintf("replan actual list --project %s --task %s --format json", overview.ProjectPath, row.TaskID),
			Actor:   agent,
			Date:    finishByTaskID[row.TaskID],
		})
	}
	taskByID := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		taskByID[task.ID] = task
	}
	for _, d := range taskPlanningDiagnostics(ws) {
		actions = append(actions, reports.DashboardNextAction{
			Code:    "record_actual_window",
			Label:   "Accepted, but the plan still runs past the status date: run replan task record-actual-window.",
			Target:  d.Target.ID,
			Command: fmt.Sprintf("replan task record-actual-window --project %s --task %s --actor <actor-id> --evidence <evidence-id> --dry-run --format json", overview.ProjectPath, d.Target.ID),
			Actor:   agent,
			Date:    finishByTaskID[d.Target.ID],
		})
	}
	for _, dependency := range ws.Dependencies {
		if dependency.Status != "open" {
			continue
		}
		// A dependency on a task that is not done yet is not an action of its
		// own: the predecessor's actions cover it, and the warning stays.
		if dependency.PredecessorRef.Kind == "task" {
			if !predecessorSatisfiesDependency(dependency, taskByID) {
				continue
			}
			actions = append(actions, reports.DashboardNextAction{
				Code:    "resolve_dependency",
				Label:   "Predecessor is done: resolve the dependency through a proposal.",
				Target:  dependency.ID,
				Command: fmt.Sprintf("replan proposal suggest-dependencies --project %s --write-candidates --actor <actor-id> --format markdown", overview.ProjectPath),
				Actor:   agent,
			})
			continue
		}
		actions = append(actions, reports.DashboardNextAction{
			Code:   "resolve_dependency",
			Label:  "Resolve or document open external dependency.",
			Target: dependency.ID,
			Actor:  person,
		})
	}
	if len(actions) == 0 {
		actions = append(actions, reports.DashboardNextAction{
			Code:  "inspect_project",
			Label: "Nothing waits on a person or an agent; keep the current plan up to date.",
		})
	}
	sortNextActions(actions)
	return actions
}

// waitingOnPersonActions lists work that only a person can move: tasks
// waiting for acceptance, tasks whose remaining work is all human, and tasks
// past their planned finish at the status date.
func waitingOnPersonActions(ws domain.Workspace, statusDate string) []reports.DashboardNextAction {
	demandsByTaskID := map[string][]domain.ResourceDemand{}
	for _, demand := range ws.ResourceDemands {
		demandsByTaskID[demand.TaskRef.ID] = append(demandsByTaskID[demand.TaskRef.ID], demand)
	}
	actions := []reports.DashboardNextAction{}
	for _, task := range ws.Tasks {
		switch task.TrackingStatus {
		case "accepted", "cancelled":
			continue
		case "review_ready":
			actions = append(actions, reports.DashboardNextAction{
				Code:   "accept_or_return_task",
				Label:  "Review the result, then accept the task or return it to work.",
				Target: task.ID,
				Actor:  reports.NextActionActorPerson,
				Date:   task.PlannedFinish,
			})
			continue
		case "in_progress":
			if onlyHumanWorkRemains(demandsByTaskID[task.ID]) {
				actions = append(actions, reports.DashboardNextAction{
					Code:   "do_human_work",
					Label:  "Only human work remains on this task.",
					Target: task.ID,
					Actor:  reports.NextActionActorPerson,
					Date:   task.PlannedFinish,
				})
			}
		}
		if statusDate != "" && task.PlannedFinish != "" && task.PlannedFinish < statusDate {
			actions = append(actions, reports.DashboardNextAction{
				Code:   "past_planned_finish",
				Label:  "Planned finish has passed: finish, reschedule through a scenario, or cancel.",
				Target: task.ID,
				Actor:  reports.NextActionActorPerson,
				Date:   task.PlannedFinish,
			})
		}
	}
	return actions
}

func onlyHumanWorkRemains(demands []domain.ResourceDemand) bool {
	human := 0
	for _, demand := range demands {
		if demand.Status != "active" || !demandHasWorkLeft(demand) {
			continue
		}
		if demand.ResourceClass != "human" && demand.ResourceClass != "role" {
			return false
		}
		human++
	}
	return human > 0
}

// demandHasWorkLeft treats a demand without a remaining estimate as not
// started: its planned amount is still ahead.
func demandHasWorkLeft(demand domain.ResourceDemand) bool {
	if demand.HasRemaining() {
		return demand.Remaining() > 0
	}
	return demand.PlannedAmount > 0
}

// sortNextActions puts what waits on a person first, then orders by date
// (earliest first, undated last), code and target.
func sortNextActions(actions []reports.DashboardNextAction) {
	rank := func(actor string) int {
		if actor == reports.NextActionActorPerson {
			return 0
		}
		return 1
	}
	sort.SliceStable(actions, func(i, j int) bool {
		a, b := actions[i], actions[j]
		if rank(a.Actor) != rank(b.Actor) {
			return rank(a.Actor) < rank(b.Actor)
		}
		if a.Date != b.Date {
			if a.Date == "" || b.Date == "" {
				return b.Date == ""
			}
			return a.Date < b.Date
		}
		if a.Code != b.Code {
			return a.Code < b.Code
		}
		return a.Target < b.Target
	})
}

// trackingRowNeedsReview skips overruns an operator already acknowledged:
// those were reviewed and stay visible in Plan vs Actual.
func trackingRowNeedsReview(row reports.DashboardTrackingRow) bool {
	for _, flag := range row.Flags {
		if flag == "accepted_without_actuals" || flag == "actuals_without_evidence" {
			return true
		}
	}
	for _, demand := range row.Demands {
		overrun, acknowledged := false, false
		for _, flag := range demand.Flags {
			switch flag {
			case "forecast_over_plan", "over_consumed":
				overrun = true
			case "overrun_acknowledged":
				acknowledged = true
			}
		}
		if overrun && !acknowledged {
			return true
		}
	}
	return false
}

// resourceCapacityBetween is the capacity of a resource, or of a pool's
// members, for the days from start to finish inclusive: weekly capacity
// prorated by day and by effective dates, slot capacity in full.
func resourceCapacityBetween(ws domain.Workspace, resourceID, unit string, start, finish time.Time) float64 {
	members := []string{resourceID}
	for _, pool := range ws.ResourcePools {
		if pool.ID == resourceID {
			members = members[:0]
			for _, member := range pool.Members {
				members = append(members, member.ID)
			}
		}
	}
	days := workingDays(ws.Availabilities)
	total := 0.0
	for _, member := range members {
		for _, capacity := range ws.Capacities {
			if capacity.ResourceRef.ID != member || capacity.Unit != unit {
				continue
			}
			total += capacityInWeek(capacity, start, finish, days[member])
		}
	}
	return total
}
