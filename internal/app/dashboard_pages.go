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

func (s ReportService) dashboardPages(ctx context.Context, ws domain.Workspace, overview reports.DashboardOverview) (reports.DashboardWorkloadPage, reports.DashboardScenariosPage, reports.DashboardProposalsPage, reports.DashboardEstimatesPage) {
	metadata := dashboardPageMetadata(overview)
	pageService := NewReportService(workspaceInMemory{ws: ws, projectPath: s.loader.ProjectPath()})
	return pageService.dashboardWorkloadPage(ctx, ws, overview, metadata),
		pageService.dashboardScenariosPage(ctx, ws, overview, metadata),
		dashboardProposalsPage(ws, metadata),
		pageService.dashboardEstimatesPage(ctx, ws, metadata)
}

func dashboardPageMetadata(overview reports.DashboardOverview) reports.DashboardPageMetadata {
	return reports.DashboardPageMetadata{
		SchemaVersion: 1, GeneratedAt: overview.GeneratedAt, ProjectID: overview.ProjectID,
		ProjectName: overview.ProjectName, StatusDate: overview.StatusDate,
		SourceSnapshotRefs: []string{}, Warnings: []reports.ReportIssue{},
	}
}

func (s ReportService) dashboardWorkloadPage(ctx context.Context, ws domain.Workspace, overview reports.DashboardOverview, metadata reports.DashboardPageMetadata) reports.DashboardWorkloadPage {
	weeks, windowStart, windowFinish := dashboardWorkloadWeeks(overview)
	page := reports.DashboardWorkloadPage{DashboardPageMetadata: metadata, Data: reports.DashboardWorkloadData{
		ScenarioID: overview.ScenarioID, WindowStart: windowStart, WindowFinish: windowFinish, Weeks: weeks, Resources: []reports.DashboardWorkloadResource{},
	}}
	resources := map[string]*reports.DashboardWorkloadResource{}
	taskTitles := map[string]string{}
	for _, task := range ws.Tasks {
		taskTitles[task.ID] = task.Title
	}
	for _, week := range page.Data.Weeks {
		result, _ := NewWorkloadService(s.loader).Calculate(ctx, WorkloadRequest{Week: week, ScenarioID: overview.ScenarioID, WithDemands: true})
		for _, warning := range result.Warnings {
			page.Warnings = append(page.Warnings, diagnosticToReportIssue(warning))
		}
		for _, row := range result.Resources {
			key := row.ResourceID + "\x00" + row.Unit
			resource := resources[key]
			if resource == nil {
				resource = &reports.DashboardWorkloadResource{ResourceID: row.ResourceID, ResourceName: row.ResourceName, ResourceClass: row.ResourceClass, Unit: row.Unit, Weeks: []reports.DashboardWorkloadCell{}}
				resources[key] = resource
			}
			load := row.Reserved
			if load == 0 {
				load = row.Remaining
			}
			var utilization *float64
			if row.Capacity > 0 {
				value := roundDashboardFloat(load / row.Capacity)
				utilization = &value
			}
			cell := reports.DashboardWorkloadCell{Week: week, Capacity: row.Capacity, Planned: row.Planned, Reserved: row.Reserved, Actual: row.Actual, Remaining: row.Remaining, Overload: row.Overload, Utilization: utilization, Demands: []reports.DashboardWorkloadDemand{}}
			if row.Capacity == 0 && load > 0 {
				cell.Shortage = load
			}
			for _, demand := range row.Demands {
				cell.Demands = append(cell.Demands, reports.DashboardWorkloadDemand{
					ResourceDemandID: demand.ResourceDemandID, TaskID: demand.TaskID, TaskTitle: taskTitles[demand.TaskID], Status: demand.Status,
					Concurrent: demand.Concurrent, Share: demand.Share, Planned: demand.Planned, Reserved: demand.Reserved, Remaining: demand.Remaining,
				})
			}
			resource.Weeks = append(resource.Weeks, cell)
		}
	}
	keys := make([]string, 0, len(resources))
	for key := range resources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		resource := resources[key]
		byWeek := map[string]reports.DashboardWorkloadCell{}
		for _, cell := range resource.Weeks {
			byWeek[cell.Week] = cell
		}
		resource.Weeks = resource.Weeks[:0]
		for _, week := range page.Data.Weeks {
			cell, ok := byWeek[week]
			if !ok {
				cell = reports.DashboardWorkloadCell{Week: week, Demands: []reports.DashboardWorkloadDemand{}}
			}
			resource.Weeks = append(resource.Weeks, cell)
		}
		page.Data.Resources = append(page.Data.Resources, *resource)
	}
	for _, resource := range page.Data.Resources {
		for _, cell := range resource.Weeks {
			if cell.Planned > 0 && isoWeekAfter(cell.Week, page.Data.WindowFinish) {
				page.Data.WindowFinish = cell.Week
			}
		}
	}
	return page
}

func dashboardWorkloadWeeks(overview reports.DashboardOverview) ([]string, string, string) {
	statusWeek := isoWeekString(overview.StatusDate)
	if statusWeek == "" {
		statusWeek = overview.Week
	}
	statusStart, _, ok := workloadWeekRange(statusWeek)
	if !ok {
		return []string{overview.Week}, overview.Week, overview.Week
	}
	windowStartDate := statusStart.AddDate(0, 0, -4*7)
	windowFinishDate := statusStart.AddDate(0, 0, 8*7)
	rawStart, rawFinish := windowStartDate, windowFinishDate
	for _, load := range overview.Gantt.ResourceLoads {
		for _, bucket := range load.Buckets {
			start, _, valid := workloadWeekRange(bucket.Label)
			if !valid {
				continue
			}
			if start.Before(rawStart) {
				rawStart = start
			}
			if start.After(rawFinish) {
				rawFinish = start
			}
		}
	}
	weeks := []string{}
	for week := rawStart; !week.After(rawFinish); week = week.AddDate(0, 0, 7) {
		weeks = append(weeks, isoWeekForTime(week))
	}
	return weeks, isoWeekForTime(windowStartDate), isoWeekForTime(windowFinishDate)
}

func isoWeekForTime(value time.Time) string {
	year, week := value.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}

func isoWeekAfter(left, right string) bool {
	leftStart, _, leftOK := workloadWeekRange(left)
	rightStart, _, rightOK := workloadWeekRange(right)
	return leftOK && rightOK && leftStart.After(rightStart)
}

func (s ReportService) dashboardScenariosPage(ctx context.Context, ws domain.Workspace, overview reports.DashboardOverview, metadata reports.DashboardPageMetadata) reports.DashboardScenariosPage {
	page := reports.DashboardScenariosPage{DashboardPageMetadata: metadata, Data: reports.DashboardScenariosData{Scenarios: []reports.DashboardScenarioDetail{}}}
	for _, scenario := range ws.Scenarios {
		diff, _ := NewScenarioService(s.loader).Diff(ctx, ScenarioDiffRequest{ID: scenario.ID, Against: "current", Week: overview.Week})
		detail := reports.DashboardScenarioDetail{
			ID: scenario.ID, Name: scenario.Name, Status: scenario.Status, CreatedAt: scenario.CreatedAt, CreatedBy: scenario.CreatedBy.ID,
			CalibrationRef: scenario.CalibrationRef.ID, Changes: []reports.DashboardScenarioChange{}, AffectedResources: diff.AffectedResources,
			AffectedTasks: scenarioAffectedTaskIDs(scenario), WorkloadBefore: workloadRows(diff.WorkloadBefore), WorkloadAfter: workloadRows(diff.WorkloadAfter),
			Warnings: diagnosticsToReportIssues(diff.Warnings), Errors: diagnosticsToReportIssues(diff.Errors),
			ApplyCommand: fmt.Sprintf("replan scenario apply --project <project> --dry-run --format json %s", scenario.ID),
		}
		for _, change := range scenario.Changes {
			detail.Changes = append(detail.Changes, reports.DashboardScenarioChange{Op: change.Op, Path: change.Path, Value: change.Value})
		}
		page.Data.Scenarios = append(page.Data.Scenarios, detail)
	}
	sort.Slice(page.Data.Scenarios, func(i, j int) bool {
		left, right := page.Data.Scenarios[i], page.Data.Scenarios[j]
		if scenarioArchived(left.Status) != scenarioArchived(right.Status) {
			return !scenarioArchived(left.Status)
		}
		if left.CreatedAt != right.CreatedAt {
			return scenarioCreatedAfter(left.CreatedAt, right.CreatedAt)
		}
		return left.ID > right.ID
	})
	return page
}

func scenarioArchived(status string) bool {
	return status == "applied" || status == "rejected"
}

func scenarioCreatedAfter(left, right string) bool {
	leftTime, leftErr := time.Parse(time.RFC3339, left)
	rightTime, rightErr := time.Parse(time.RFC3339, right)
	if leftErr == nil && rightErr == nil {
		return leftTime.After(rightTime)
	}
	return left > right
}

func scenarioAffectedTaskIDs(scenario domain.Scenario) []string {
	set := map[string]bool{}
	for _, change := range scenario.Changes {
		parts := strings.Split(strings.Trim(change.Path, "/"), "/")
		for i := 0; i+1 < len(parts); i++ {
			if parts[i] == "tasks" || parts[i] == "task" {
				set[parts[i+1]] = true
			}
		}
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func workloadRows(rows []ResourceWorkload) []reports.ResourceRow {
	result := make([]reports.ResourceRow, 0, len(rows))
	for _, row := range rows {
		result = append(result, reports.ResourceRow{ResourceID: row.ResourceID, ResourceName: row.ResourceName, ResourceClass: row.ResourceClass, Unit: row.Unit, Capacity: row.Capacity, Planned: row.Planned, Reserved: row.Reserved, Actual: row.Actual, Remaining: row.Remaining, Overload: row.Overload})
	}
	return result
}

func dashboardProposalsPage(ws domain.Workspace, metadata reports.DashboardPageMetadata) reports.DashboardProposalsPage {
	page := reports.DashboardProposalsPage{DashboardPageMetadata: metadata, Data: reports.DashboardProposalsData{Proposals: []reports.DashboardProposalDetail{}}}
	evidence := map[string]domain.Evidence{}
	for _, item := range ws.Evidence {
		evidence[item.ID] = item
	}
	for _, proposal := range ws.Proposals {
		if proposal.Status != "open" {
			continue
		}
		detail := reports.DashboardProposalDetail{
			ID: proposal.ID, Type: proposal.Type, Status: proposal.Status, CreatedAt: proposal.CreatedAt, CreatedBy: proposal.CreatedBy.ID,
			TargetKind: proposal.TargetRef.Kind, TargetID: proposal.TargetRef.ID, Rationale: proposal.Rationale, RequiresConfirmation: proposal.RequiresConfirmation,
			Impact:        reports.DashboardProposalImpact{Op: proposal.SuggestedChange.Op, Path: proposal.SuggestedChange.Path, Value: proposal.SuggestedChange.Value},
			Evidence:      []reports.DashboardProposalEvidence{},
			AcceptCommand: fmt.Sprintf("replan proposal accept --project <project> --actor <actor-id> --reason \"<reason>\" --apply --format json %s", proposal.ID),
			RejectCommand: fmt.Sprintf("replan proposal reject --project <project> --actor <actor-id> --reason \"<reason>\" --apply --format json %s", proposal.ID),
		}
		for _, ref := range proposal.EvidenceRefs {
			item := evidence[ref.ID]
			detail.Evidence = append(detail.Evidence, reports.DashboardProposalEvidence{ID: ref.ID, Type: item.Type, Subtype: item.Subtype, Summary: item.Summary, ObservedAt: item.ObservedAt})
		}
		page.Data.Proposals = append(page.Data.Proposals, detail)
	}
	sort.Slice(page.Data.Proposals, func(i, j int) bool { return page.Data.Proposals[i].ID < page.Data.Proposals[j].ID })
	return page
}

func (s ReportService) dashboardEstimatesPage(ctx context.Context, ws domain.Workspace, metadata reports.DashboardPageMetadata) reports.DashboardEstimatesPage {
	page := reports.DashboardEstimatesPage{DashboardPageMetadata: metadata, Data: reports.DashboardEstimatesData{Baselines: []reports.DashboardEstimateBaseline{}}}
	for _, baseline := range ws.Baselines {
		if baseline.Seal == nil || !domain.BaselineCarriesEstimates(baseline) {
			continue
		}
		report, _ := s.Estimates(ctx, EstimateReportRequest{BaselineID: baseline.ID})
		item := reports.DashboardEstimateBaseline{ID: baseline.ID, Name: baseline.Name, StatusDate: baseline.StatusDate, SealStatus: report.BaselineSealStatus, Resources: []reports.DashboardEstimateResource{}, SplitGroups: []reports.DashboardEstimateSplitGroup{}, Tasks: []reports.DashboardEstimateTask{}, Calibrations: []reports.DashboardEstimateCalibration{}, Warnings: diagnosticsToReportIssues(report.Warnings), Errors: diagnosticsToReportIssues(report.Errors)}
		for _, resource := range report.Resources {
			item.Resources = append(item.Resources, reports.DashboardEstimateResource{ResourceID: resource.ResourceID, ResourceName: resource.ResourceName, Unit: resource.Unit, ActualsRecorded: resource.ActualsRecorded, BaselineAmount: resource.All.BaselineAmount, ActualAmount: resource.All.ActualAmount, Ratio: resource.All.ActualBaselineRatio, AddedAfterBaseline: resource.All.BaselineAmount == 0 && resource.ActualsRecorded})
		}
		for _, group := range report.SplitGroups {
			item.SplitGroups = append(item.SplitGroups, reports.DashboardEstimateSplitGroup{OriginalResourceID: group.OriginalResourceID, OriginalResourceName: group.OriginalResourceName, SplitResourceIDs: group.SplitResourceIDs, Unit: group.Unit, ActualsRecorded: group.ActualsRecorded, BaselineAmount: group.All.BaselineAmount, CurrentPlannedAmount: group.All.CurrentPlannedAmount, ActualAmount: group.All.ActualAmount, Ratio: group.All.ActualBaselineRatio})
		}
		for _, task := range report.Tasks {
			row := reports.DashboardEstimateTask{TaskID: task.TaskID, Title: task.Title, TrackingStatus: task.TrackingStatus, Open: task.Open, Totals: []reports.DashboardEstimateTotal{}}
			for _, total := range task.Totals {
				row.Totals = append(row.Totals, reports.DashboardEstimateTotal{Unit: total.Unit, BaselineAmount: total.BaselineAmount, ActualAmount: total.ActualAmount, VarianceAmount: total.VarianceAmount, Ratio: total.ActualBaselineRatio})
			}
			item.Tasks = append(item.Tasks, row)
		}
		for _, calibration := range report.Calibrations {
			item.Calibrations = append(item.Calibrations, reports.DashboardEstimateCalibration{ID: calibration.ID, Name: calibration.Name, Resources: calibration.Resources, Unit: calibration.Unit, Factor: calibration.Factor, TotalRatio: calibration.TotalRatio, Count: calibration.Count})
		}
		page.Data.Baselines = append(page.Data.Baselines, item)
	}
	sort.Slice(page.Data.Baselines, func(i, j int) bool { return page.Data.Baselines[i].ID < page.Data.Baselines[j].ID })
	return page
}

func diagnosticsToReportIssues(diagnostics []Diagnostic) []reports.ReportIssue {
	issues := make([]reports.ReportIssue, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		issues = append(issues, diagnosticToReportIssue(diagnostic))
	}
	return issues
}
