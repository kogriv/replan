package app

import (
	"context"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

type ExportService struct {
	loader WorkspaceLoader
}

type ExportJSONRequest struct {
	Format string
	Now    time.Time
}

type ExportJSONResult struct {
	Metadata  ExportMetadata  `json:"metadata"`
	Workspace ExportWorkspace `json:"workspace"`
	Warnings  []Diagnostic    `json:"warnings"`
	Errors    []Diagnostic    `json:"errors"`
}

type ExportMetadata struct {
	ID          string `json:"id"`
	Format      string `json:"format"`
	ExportedAt  string `json:"exported_at"`
	ProjectPath string `json:"project_path"`
}

type ExportWorkspace struct {
	Project         domain.Project          `json:"project"`
	Actors          []domain.Actor          `json:"actors"`
	Resources       []domain.Resource       `json:"resources"`
	ResourcePools   []domain.ResourcePool   `json:"resource_pools"`
	Tasks           []domain.Task           `json:"tasks"`
	ResourceDemands []domain.ResourceDemand `json:"resource_demands"`
	Dependencies    []domain.Dependency     `json:"dependencies"`
	Capacities      []domain.Capacity       `json:"capacities"`
	Availability    []domain.Availability   `json:"availability"`
	CurrentPlan     domain.CurrentPlan      `json:"current_plan"`
	Baselines       []domain.Baseline       `json:"baselines"`
	Calibrations    []domain.Calibration    `json:"calibrations"`
	Actuals         []domain.Actual         `json:"actuals"`
	Evidence        []domain.Evidence       `json:"evidence"`
	Proposals       []domain.Proposal       `json:"proposals"`
	Scenarios       []domain.Scenario       `json:"scenarios"`
	AuditRecords    []domain.AuditRecord    `json:"audit_records"`
	CheckedFiles    []string                `json:"checked_files"`
}

func NewExportService(loader WorkspaceLoader) ExportService {
	return ExportService{loader: loader}
}

func (s ExportService) JSON(ctx context.Context, req ExportJSONRequest) (ExportJSONResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	format := req.Format
	if format == "" {
		format = "json"
	}
	result := ExportJSONResult{
		Metadata: ExportMetadata{
			ID:          "export.replan." + compactTimestamp(now.Format(time.RFC3339)),
			Format:      format,
			ExportedAt:  now.Format(time.RFC3339),
			ProjectPath: s.loader.ProjectPath(),
		},
		Workspace: ExportWorkspace{
			Project:         ws.Project,
			Actors:          ws.Actors,
			Resources:       ws.Resources,
			ResourcePools:   ws.ResourcePools,
			Tasks:           ws.Tasks,
			ResourceDemands: ws.ResourceDemands,
			Dependencies:    ws.Dependencies,
			Capacities:      ws.Capacities,
			Availability:    ws.Availabilities,
			CurrentPlan:     ws.CurrentPlan,
			Baselines:       ws.Baselines,
			Calibrations:    ws.Calibrations,
			Actuals:         ws.Actuals,
			Evidence:        ws.Evidence,
			Proposals:       ws.Proposals,
			Scenarios:       ws.Scenarios,
			AuditRecords:    ws.AuditRecords,
			CheckedFiles:    ws.CheckedFiles,
		},
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	return result, nil
}
