package app

import (
	"context"
	"fmt"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

// ProjectWriter changes project-level settings in .replan/project.yaml.
type ProjectWriter interface {
	SetProjectStatusDate(date string) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
}

type ProjectMigrationWriter interface {
	ProjectFormatVersion() (version int, explicit bool, err error)
	SetProjectFormatVersion(version int) (string, error)
	AppendAudit(record domain.AuditRecord) (string, error)
}

type ProjectService struct {
	loader WorkspaceLoader
}

func NewProjectService(loader WorkspaceLoader) ProjectService {
	return ProjectService{loader: loader}
}

type ProjectSetStatusDateRequest struct {
	// Date is YYYY-MM-DD, or "today" for the current date in the project
	// time zone.
	Date       string
	Reason     string
	ActorID    string
	OccurredAt string
	DryRun     bool
	Now        time.Time
}

type ProjectSetStatusDateResult struct {
	DryRun             bool               `json:"dry_run"`
	Applied            bool               `json:"applied"`
	ProjectID          string             `json:"project_id"`
	PreviousDate       string             `json:"previous_date,omitempty"`
	NewDate            string             `json:"new_date"`
	Week               string             `json:"week"`
	Reason             string             `json:"reason,omitempty"`
	PlannedWrites      []PlannedWrite     `json:"planned_writes"`
	WrittenFiles       []string           `json:"written_files,omitempty"`
	AuditRecordPreview domain.AuditRecord `json:"audit_record_preview"`
	Validation         *ValidateResult    `json:"validation,omitempty"`
	Warnings           []Diagnostic       `json:"warnings"`
	Errors             []Diagnostic       `json:"errors"`
}

type ProjectMigrateRequest struct {
	Reason     string
	ActorID    string
	OccurredAt string
	DryRun     bool
	Now        time.Time
}

type ProjectMigrationStep struct {
	Name        string `json:"name"`
	FromVersion int    `json:"from_version"`
	ToVersion   int    `json:"to_version"`
	Description string `json:"description"`
}

type ProjectMigrateResult struct {
	DryRun             bool                   `json:"dry_run"`
	Applied            bool                   `json:"applied"`
	ProjectID          string                 `json:"project_id"`
	FromVersion        int                    `json:"from_version"`
	ToVersion          int                    `json:"to_version"`
	Steps              []ProjectMigrationStep `json:"steps"`
	PlannedWrites      []PlannedWrite         `json:"planned_writes"`
	WrittenFiles       []string               `json:"written_files,omitempty"`
	AuditRecordPreview *domain.AuditRecord    `json:"audit_record_preview,omitempty"`
	Validation         *ValidateResult        `json:"validation,omitempty"`
	Warnings           []Diagnostic           `json:"warnings"`
	Errors             []Diagnostic           `json:"errors"`
}

type projectMigrationStepDefinition struct {
	step  ProjectMigrationStep
	apply func(ProjectMigrationWriter) (string, error)
}

var projectFormatMigrationSteps = map[int]projectMigrationStepDefinition{}

// PlanProjectFormatMigration builds the ordered N-to-N+1 chain. Adding a
// future format requires one entry in projectFormatMigrationSteps.
func PlanProjectFormatMigration(fromVersion, toVersion int) ([]ProjectMigrationStep, error) {
	if fromVersion > toVersion {
		return nil, fmt.Errorf("project format migrations never downgrade from %d to %d", fromVersion, toVersion)
	}
	steps := []ProjectMigrationStep{}
	for version := fromVersion; version < toVersion; version++ {
		definition, ok := projectFormatMigrationSteps[version]
		if !ok || definition.step.FromVersion != version || definition.step.ToVersion != version+1 || definition.apply == nil {
			return nil, fmt.Errorf("no project format migration step from %d to %d", version, version+1)
		}
		steps = append(steps, definition.step)
	}
	return steps, nil
}

// Migrate upgrades the project manifest to the current format and records one
// audit event for the complete migration chain.
func (s ProjectService) Migrate(ctx context.Context, req ProjectMigrateRequest) (ProjectMigrateResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ProjectMigrateResult{
		DryRun: req.DryRun, ProjectID: ws.Project.ID,
		FromVersion: ws.Project.FormatVersion, ToVersion: domain.CurrentProjectFormatVersion,
		Steps: []ProjectMigrationStep{}, PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{},
		Warnings: []Diagnostic{}, Errors: []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	reader, ok := s.loader.(ProjectMigrationWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "storage_write_not_supported", Message: "configured project store does not support project migration", Severity: "error", Target: Target{Kind: "project", ID: s.loader.ProjectPath()}})
		return result, ErrValidationFailed
	}
	fromVersion, explicit, err := reader.ProjectFormatVersion()
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "project_format_read_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "file", ID: ".replan/project.yaml"}})
		return result, err
	}
	result.FromVersion = fromVersion
	chain, err := PlanProjectFormatMigration(fromVersion, domain.CurrentProjectFormatVersion)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "project_migration_path_missing", Message: err.Error(), Severity: "error", Target: Target{Kind: "project", ID: ws.Project.ID}})
		return result, ErrValidationFailed
	}
	definitions := make([]projectMigrationStepDefinition, 0, len(chain)+1)
	if !explicit {
		step := ProjectMigrationStep{Name: "declare_format_version", FromVersion: fromVersion, ToVersion: fromVersion, Description: fmt.Sprintf("write project.format_version: %d", fromVersion)}
		result.Steps = append(result.Steps, step)
		definitions = append(definitions, projectMigrationStepDefinition{step: step, apply: func(writer ProjectMigrationWriter) (string, error) {
			return writer.SetProjectFormatVersion(fromVersion)
		}})
	}
	for _, step := range chain {
		result.Steps = append(result.Steps, step)
		definitions = append(definitions, projectFormatMigrationSteps[step.FromVersion])
	}
	if req.ActorID == "" {
		result.Errors = append(result.Errors, Diagnostic{Code: "missing_actor", Message: "actor is required", Severity: "error", Target: Target{Kind: "actor"}})
	} else if !hasActor(ws.Actors, req.ActorID) {
		result.Errors = append(result.Errors, Diagnostic{Code: "actor_not_found", Message: fmt.Sprintf("actor %s not found", req.ActorID), Severity: "error", Target: Target{Kind: "actor", ID: req.ActorID}})
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	occurredAt := req.OccurredAt
	if occurredAt == "" {
		occurredAt = now.Format(time.RFC3339)
	} else if _, err := time.Parse(time.RFC3339, occurredAt); err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "invalid_occurred_at", Message: fmt.Sprintf("occurred-at %q is not RFC3339", occurredAt), Severity: "error", Target: Target{Kind: "project", ID: ws.Project.ID}})
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	if len(definitions) == 0 {
		return result, nil
	}

	patchYAML, err := yaml.Marshal(map[string]any{"op": "add", "path": "project/format_version", "value": domain.CurrentProjectFormatVersion})
	if err != nil {
		return result, err
	}
	audit := domain.AuditRecord{ID: "audit." + compactTimestamp(occurredAt) + ".project-migrate." + slugID(ws.Project.ID), OccurredAt: occurredAt, ActorRef: domain.Ref{Kind: "actor", ID: req.ActorID}, Operation: "project_migrate", TargetRef: domain.Ref{Kind: "project", ID: ws.Project.ID}, Reason: req.Reason, Result: "success"}
	if req.DryRun {
		audit.Result = "dry_run"
	} else if audit.Reason == "" {
		audit.Reason = fmt.Sprintf("Migrated the project format from %d to %d.", fromVersion, domain.CurrentProjectFormatVersion)
	}
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{Path: ".replan/project.yaml", Action: "insert_yaml_field", Format: "yaml-patch-preview", Content: string(patchYAML)}, auditPlannedWrite(audit))
	result.AuditRecordPreview = &audit
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(ProjectMigrationWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "storage_write_not_supported", Message: "configured project store does not support project migration", Severity: "error", Target: Target{Kind: "project", ID: s.loader.ProjectPath()}})
		return result, ErrValidationFailed
	}
	for _, definition := range definitions {
		path, err := definition.apply(writer)
		if err != nil {
			result.Errors = append(result.Errors, Diagnostic{Code: "project_migration_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "file", ID: path}})
			return result, err
		}
		result.WrittenFiles = append(result.WrittenFiles, path)
	}
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "audit_write_failed", Message: err.Error(), Severity: "error", Target: Target{Kind: "audit_record", ID: audit.ID}})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
	validation, written, err := session.finish(ctx, result.WrittenFiles)
	result.Validation, result.WrittenFiles = &validation, written
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

// SetStatusDate moves the project status date, which sets the default week of
// report state, next and tracking suggest, and decides which tasks are past
// their planned finish.
func (s ProjectService) SetStatusDate(ctx context.Context, req ProjectSetStatusDateRequest) (ProjectSetStatusDateResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ProjectSetStatusDateResult{
		DryRun:        req.DryRun,
		ProjectID:     ws.Project.ID,
		PreviousDate:  ws.Project.DefaultStatusDate,
		Reason:        req.Reason,
		PlannedWrites: []PlannedWrite{},
		WrittenFiles:  []string{},
		Warnings:      []Diagnostic{},
		Errors:        []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)

	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	date, err := resolveStatusDate(req.Date, ws.Project.Timezone, now)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "invalid_status_date", Message: err.Error(), Severity: "error",
			Target: Target{Kind: "project", ID: ws.Project.ID}})
	}
	result.NewDate = date
	result.Week = isoWeekString(date)
	if req.ActorID == "" {
		result.Errors = append(result.Errors, Diagnostic{Code: "missing_actor", Message: "actor is required", Severity: "error", Target: Target{Kind: "actor"}})
	} else if !hasActor(ws.Actors, req.ActorID) {
		result.Errors = append(result.Errors, Diagnostic{Code: "actor_not_found", Message: fmt.Sprintf("actor %s not found", req.ActorID), Severity: "error",
			Target: Target{Kind: "actor", ID: req.ActorID}})
	}
	if err == nil && date == ws.Project.DefaultStatusDate {
		result.Errors = append(result.Errors, Diagnostic{Code: "status_date_unchanged",
			Message: fmt.Sprintf("status date is already %s", date), Severity: "error", Target: Target{Kind: "project", ID: ws.Project.ID}})
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	if ws.Project.DefaultStatusDate != "" && date < ws.Project.DefaultStatusDate {
		result.Warnings = append(result.Warnings, Diagnostic{Code: "status_date_moved_backwards",
			Message:  fmt.Sprintf("status date moves back from %s to %s; reports will show an earlier week", ws.Project.DefaultStatusDate, date),
			Severity: "warning", Target: Target{Kind: "project", ID: ws.Project.ID}})
	}

	occurredAt := req.OccurredAt
	if occurredAt == "" {
		occurredAt = now.Format(time.RFC3339)
	}
	patch := map[string]any{
		"op":     "replace",
		"path":   "project/default_status_date",
		"value":  date,
		"before": ws.Project.DefaultStatusDate,
	}
	if req.Reason != "" {
		patch["reason"] = req.Reason
	}
	patchYAML, err := yaml.Marshal(patch)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "yaml_render_error",
			Message: fmt.Sprintf("failed to render status-date patch yaml: %v", err), Severity: "error"})
		return result, ErrValidationFailed
	}
	audit := domain.AuditRecord{
		ID:         "audit." + compactTimestamp(occurredAt) + ".project-set-status-date." + slugID(ws.Project.ID),
		OccurredAt: occurredAt,
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:  "project_set_status_date",
		TargetRef:  domain.Ref{Kind: "project", ID: ws.Project.ID},
		Reason:     req.Reason,
		Result:     "success",
	}
	if req.DryRun {
		audit.Result = "dry_run"
	} else if audit.Reason == "" {
		audit.Reason = fmt.Sprintf("Moved the project status date to %s.", date)
	}
	result.PlannedWrites = append(result.PlannedWrites, PlannedWrite{
		Path:    ".replan/project.yaml",
		Action:  "replace_yaml_field",
		Format:  "yaml-patch-preview",
		Content: string(patchYAML),
	})
	result.AuditRecordPreview = audit
	if req.DryRun {
		return result, nil
	}

	session := beginWrite(s.loader)
	writer, ok := session.target.(ProjectWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "storage_write_not_supported",
			Message: "configured project store does not support project writes", Severity: "error",
			Target: Target{Kind: "project", ID: s.loader.ProjectPath()}})
		return result, ErrValidationFailed
	}
	projectPath, err := writer.SetProjectStatusDate(date)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "status_date_write_failed", Message: err.Error(), Severity: "error",
			Target: Target{Kind: "project", ID: ws.Project.ID}})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, projectPath)
	auditPath, err := writer.AppendAudit(audit)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "audit_write_failed", Message: err.Error(), Severity: "error",
			Target: Target{Kind: "audit_record", ID: audit.ID}})
		return result, err
	}
	result.WrittenFiles = append(result.WrittenFiles, auditPath)
	validation, written, err := session.finish(ctx, result.WrittenFiles)
	result.Validation = &validation
	result.WrittenFiles = written
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return result, err
	}
	result.Applied = true
	return result, nil
}

// resolveStatusDate accepts YYYY-MM-DD or "today" in the project time zone;
// an unknown or empty time zone falls back to the local zone.
func resolveStatusDate(value, timezone string, now time.Time) (string, error) {
	if value == "" {
		return "", fmt.Errorf("date is required: YYYY-MM-DD or today")
	}
	if value == "today" {
		if loc, err := time.LoadLocation(timezone); timezone != "" && err == nil {
			now = now.In(loc)
		}
		return now.Format("2006-01-02"), nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil || parsed.Format("2006-01-02") != value {
		return value, fmt.Errorf("date %q is not YYYY-MM-DD or today", value)
	}
	return value, nil
}
