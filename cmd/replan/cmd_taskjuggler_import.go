package main

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type TaskJugglerImportResult struct {
	RunDir           string                       `json:"run_dir"`
	ManifestPath     string                       `json:"manifest_path"`
	ReportPath       string                       `json:"report_path"`
	MappingPath      string                       `json:"mapping_path"`
	RowsRead         int                          `json:"rows_read"`
	CandidateChanges int                          `json:"candidate_changes"`
	Candidate        TaskJugglerScenarioCandidate `json:"candidate"`
	ScenarioPreview  domain.Scenario              `json:"scenario_preview,omitempty"`
	ProposalPreview  domain.Proposal              `json:"proposal_preview,omitempty"`
	PlannedWrites    []app.PlannedWrite           `json:"planned_writes"`
	WrittenFiles     []string                     `json:"written_files,omitempty"`
	PreviewOnly      bool                         `json:"preview_only"`
	Applied          bool                         `json:"applied"`
	Warnings         []app.Diagnostic             `json:"warnings"`
	Errors           []app.Diagnostic             `json:"errors"`
}

type TaskJugglerScenarioCandidate struct {
	ID          string                       `json:"id"`
	Source      string                       `json:"source"`
	SourceRunID string                       `json:"source_run_id"`
	Changes     []TaskJugglerCandidateChange `json:"changes"`
}

type TaskJugglerCandidateChange struct {
	Op            string `json:"op"`
	Path          string `json:"path"`
	Value         any    `json:"value,omitempty"`
	PreviousValue any    `json:"previous_value,omitempty"`
}

func runTaskJugglerImportResult(args []string, stdout, stderr *os.File) int {
	fs := newCLIFlagSet("taskjuggler import-result")
	projectPath := fs.String("project", ".", "path to RePlan project")
	fs.StringVar(projectPath, "p", ".", "path to RePlan project")
	runDir := fs.String("run-dir", "", "TaskJuggler run artifact directory")
	scenarioID := fs.String("scenario-id", "", "scenario candidate id to use in dry-run output")
	dryRun := fs.Bool("dry-run", false, "preview imported TaskJuggler result without writing canonical state")
	writeCandidate := fs.Bool("write-candidate", false, "write reviewable scenario/proposal candidate files without mutating canonical state")
	actorID := fs.String("actor", "", "actor id creating candidate files; required with --write-candidate")
	format := fs.String("format", "json", "output format: json, markdown or text")
	if code, done := parseCommandFlags(fs, args, stdout, stderr); done {
		return code
	}
	result, err := importTaskJugglerResult(context.Background(), taskJugglerImportRequest{
		ProjectPath:    *projectPath,
		RunDir:         *runDir,
		ScenarioID:     *scenarioID,
		DryRun:         *dryRun,
		WriteCandidate: *writeCandidate,
		ActorID:        *actorID,
		Now:            time.Now(),
	})
	env := envelope{
		OK:          err == nil && len(result.Errors) == 0,
		Command:     "taskjuggler import-result",
		ProjectPath: *projectPath,
		Result:      result,
		Warnings:    result.Warnings,
		Errors:      result.Errors,
	}
	return printScenarioEnvelope(stdout, stderr, *format, env, err)
}

type taskJugglerImportRequest struct {
	ProjectPath    string
	RunDir         string
	ScenarioID     string
	DryRun         bool
	WriteCandidate bool
	ActorID        string
	Now            time.Time
}

type taskJugglerImportState struct {
	ctx       context.Context
	req       taskJugglerImportRequest
	result    *TaskJugglerImportResult
	manifest  TaskJugglerRunManifest
	taskIDMap map[string]string
	rows      []taskJugglerTaskScheduleRow
	workspace domain.Workspace
}

func (s *taskJugglerImportState) validateRequest() error {
	if s.req.DryRun && s.req.WriteCandidate {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "conflicting_taskjuggler_import_modes",
			Message:  "taskjuggler import-result accepts either --dry-run or --write-candidate, not both",
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	if !s.req.DryRun && !s.req.WriteCandidate {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "dry_run_required",
			Message:  "taskjuggler import-result requires --dry-run or explicit --write-candidate",
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	if s.req.WriteCandidate && strings.TrimSpace(s.req.ActorID) == "" {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "actor_required",
			Message:  "taskjuggler import-result --write-candidate requires --actor",
			Severity: "error",
			Target:   app.Target{Kind: "actor"},
		})
		return app.ErrValidationFailed
	}
	if strings.TrimSpace(s.req.RunDir) == "" {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "taskjuggler_run_dir_required",
			Message:  "--run-dir is required",
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	return nil
}

func (s *taskJugglerImportState) loadManifest() error {
	manifestPath := filepath.Join(s.req.RunDir, "run.manifest.json")
	s.result.ManifestPath = manifestPath
	if err := readJSONFile(manifestPath, &s.manifest); err != nil {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "taskjuggler_manifest_missing",
			Message:  fmt.Sprintf("failed to read TaskJuggler run manifest: %v", err),
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	s.result.Candidate.SourceRunID = s.manifest.ID
	if s.req.ScenarioID != "" {
		s.result.Candidate.ID = s.req.ScenarioID
	} else {
		s.result.Candidate.ID = defaultTaskJugglerScenarioCandidateID(s.manifest.ID)
	}
	return nil
}

func (s *taskJugglerImportState) loadMapping() error {
	mappingPath := resolveRunArtifactPath(s.req.RunDir, s.manifest.MappingPath)
	if mappingPath == "" {
		mappingPath = filepath.Join(s.req.RunDir, "taskjuggler.mapping.json")
	}
	s.result.MappingPath = mappingPath
	var mapping app.TaskJugglerExportMapping
	if err := readJSONFile(mappingPath, &mapping); err != nil {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "taskjuggler_mapping_missing",
			Message:  fmt.Sprintf("failed to read TaskJuggler mapping: %v", err),
			Severity: "error",
		})
		return app.ErrValidationFailed
	}
	var mappingErrors []app.Diagnostic
	s.taskIDMap, mappingErrors = taskJugglerTaskIDMap(mapping)
	s.result.Errors = append(s.result.Errors, mappingErrors...)
	if len(s.result.Errors) > 0 {
		return app.ErrValidationFailed
	}
	return nil
}

func (s *taskJugglerImportState) loadReport() error {
	reportPath := taskJugglerTaskScheduleReportPath(s.req.RunDir, s.manifest)
	s.result.ReportPath = reportPath
	var diagnostics []app.Diagnostic
	s.rows, diagnostics = readTaskJugglerTaskScheduleCSV(reportPath)
	if diagnostics != nil {
		s.result.Errors = append(s.result.Errors, diagnostics...)
		return app.ErrValidationFailed
	}
	s.result.RowsRead = len(s.rows)
	return nil
}

func (s *taskJugglerImportState) loadWorkspace() error {
	var loadDiags []app.Diagnostic
	s.workspace, loadDiags = newStore(s.req.ProjectPath).Load(s.ctx)
	for _, diag := range loadDiags {
		if diag.Severity == "error" {
			s.result.Errors = append(s.result.Errors, diag)
		} else {
			s.result.Warnings = append(s.result.Warnings, diag)
		}
	}
	if len(s.result.Errors) > 0 {
		return app.ErrValidationFailed
	}
	if s.req.WriteCandidate && !hasActor(s.workspace, s.req.ActorID) {
		s.result.Errors = append(s.result.Errors, app.Diagnostic{
			Code:     "actor_not_found",
			Message:  fmt.Sprintf("actor %s not found", s.req.ActorID),
			Severity: "error",
			Target:   app.Target{Kind: "actor", ID: s.req.ActorID},
		})
		return app.ErrValidationFailed
	}
	return nil
}

func (s *taskJugglerImportState) buildCandidateChanges() error {
	taskIDs := map[string]struct{}{}
	for _, task := range s.workspace.Tasks {
		taskIDs[task.ID] = struct{}{}
	}
	for _, row := range s.rows {
		replanTaskID, ok := s.taskIDMap[row.ID]
		if !ok {
			s.result.Warnings = append(s.result.Warnings, app.Diagnostic{
				Code:     "taskjuggler_report_task_unmapped",
				Message:  fmt.Sprintf("TaskJuggler report row %s has no RePlan task mapping", row.ID),
				Severity: "warning",
				Target:   app.Target{Kind: "taskjuggler_task", ID: row.ID},
			})
			continue
		}
		if _, exists := taskIDs[replanTaskID]; !exists {
			s.result.Warnings = append(s.result.Warnings, app.Diagnostic{
				Code:     "taskjuggler_report_task_missing",
				Message:  fmt.Sprintf("mapped RePlan task %s is not present in the project", replanTaskID),
				Severity: "warning",
				Target:   app.Target{Kind: "task", ID: replanTaskID},
			})
			continue
		}
		if row.Start != "" {
			s.result.Candidate.Changes = append(s.result.Candidate.Changes, TaskJugglerCandidateChange{
				Op:            "set",
				Path:          fmt.Sprintf("tasks.%s.planned_start", replanTaskID),
				Value:         row.Start,
				PreviousValue: "",
			})
		}
		if row.End != "" {
			s.result.Candidate.Changes = append(s.result.Candidate.Changes, TaskJugglerCandidateChange{
				Op:            "set",
				Path:          fmt.Sprintf("tasks.%s.planned_finish", replanTaskID),
				Value:         row.End,
				PreviousValue: "",
			})
		}
	}
	s.result.CandidateChanges = len(s.result.Candidate.Changes)
	if s.result.CandidateChanges == 0 {
		s.result.Warnings = append(s.result.Warnings, app.Diagnostic{
			Code:     "taskjuggler_report_no_candidate_changes",
			Message:  "TaskJuggler report produced no candidate changes",
			Severity: "warning",
		})
		if s.req.WriteCandidate {
			return app.ErrValidationFailed
		}
	}
	return nil
}

func (s *taskJugglerImportState) preparePreviews() error {
	now := s.req.Now
	if now.IsZero() {
		now = time.Now()
	}
	actorID := s.req.ActorID
	if actorID == "" {
		actorID = "actor.system.replan"
	}
	s.result.ScenarioPreview = taskJugglerScenarioPreview(s.result.Candidate, s.manifest, actorID, now)
	s.result.ProposalPreview = taskJugglerProposalPreview(s.result.ScenarioPreview, s.manifest, actorID, now)
	s.result.PlannedWrites = taskJugglerCandidatePlannedWrites(s.result.ScenarioPreview, s.result.ProposalPreview)
	return nil
}

func (s *taskJugglerImportState) writeCandidate() error {
	if s.req.WriteCandidate {
		_, written, diags, writeErr := app.CreateProjectFiles(context.Background(), newStore(s.req.ProjectPath), s.result.PlannedWrites)
		s.result.WrittenFiles = written
		if writeErr != nil {
			s.result.Errors = append(s.result.Errors, app.Diagnostic{
				Code:     "taskjuggler_candidate_write_failed",
				Message:  writeErr.Error(),
				Severity: "error",
			})
			s.result.Errors = append(s.result.Errors, diags...)
			return writeErr
		}
		s.result.Applied = true
	}
	return nil
}

type taskJugglerTaskScheduleRow struct {
	ID         string
	Name       string
	Start      string
	End        string
	Effort     string
	EffortLeft string
	Resources  string
	Status     string
}

func readTaskJugglerTaskScheduleCSV(path string) ([]taskJugglerTaskScheduleRow, []app.Diagnostic) {
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, []app.Diagnostic{{
			Code:     "taskjuggler_report_missing",
			Message:  fmt.Sprintf("failed to read TaskJuggler task schedule report: %v", readErr),
			Severity: "error",
		}}
	}
	reader := csv.NewReader(strings.NewReader(string(content)))
	reader.Comma = ';'
	reader.FieldsPerRecord = -1
	records, parseErr := reader.ReadAll()
	if parseErr != nil {
		return nil, []app.Diagnostic{{
			Code:     "taskjuggler_report_row_invalid",
			Message:  fmt.Sprintf("failed to parse TaskJuggler CSV report: %v", parseErr),
			Severity: "error",
		}}
	}
	if len(records) == 0 {
		return nil, []app.Diagnostic{{
			Code:     "taskjuggler_report_header_invalid",
			Message:  "TaskJuggler CSV report is empty",
			Severity: "error",
		}}
	}
	header := map[string]int{}
	for i, column := range records[0] {
		header[strings.TrimSpace(column)] = i
	}
	required := []string{"Id", "Name", "Start", "End", "Effort", "Effort Left", "Resources", "Status"}
	for _, column := range required {
		if _, ok := header[column]; !ok {
			return nil, []app.Diagnostic{{
				Code:     "taskjuggler_report_header_invalid",
				Message:  fmt.Sprintf("TaskJuggler CSV report is missing required column %q", column),
				Severity: "error",
			}}
		}
	}
	rows := []taskJugglerTaskScheduleRow{}
	diagnostics := []app.Diagnostic{}
	for i, record := range records[1:] {
		value := func(column string) string {
			idx := header[column]
			if idx >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[idx])
		}
		row := taskJugglerTaskScheduleRow{
			ID:         value("Id"),
			Name:       value("Name"),
			Start:      value("Start"),
			End:        value("End"),
			Effort:     value("Effort"),
			EffortLeft: value("Effort Left"),
			Resources:  value("Resources"),
			Status:     value("Status"),
		}
		if row.ID == "" {
			diagnostics = append(diagnostics, app.Diagnostic{
				Code:     "taskjuggler_report_row_invalid",
				Message:  fmt.Sprintf("TaskJuggler CSV row %d has empty Id", i+2),
				Severity: "error",
			})
			continue
		}
		if diag, ok := validateTaskJugglerReportDate(row.Start, row.ID, "Start"); ok {
			diagnostics = append(diagnostics, diag)
		}
		if diag, ok := validateTaskJugglerReportDate(row.End, row.ID, "End"); ok {
			diagnostics = append(diagnostics, diag)
		}
		rows = append(rows, row)
	}
	if len(diagnostics) > 0 {
		return nil, diagnostics
	}
	return rows, nil
}

func validateTaskJugglerReportDate(value, taskID, column string) (app.Diagnostic, bool) {
	if value == "" {
		return app.Diagnostic{}, false
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return app.Diagnostic{
			Code:     "taskjuggler_report_date_invalid",
			Message:  fmt.Sprintf("TaskJuggler task %s has invalid %s date %q", taskID, column, value),
			Severity: "error",
			Target:   app.Target{Kind: "taskjuggler_task", ID: taskID},
		}, true
	}
	return app.Diagnostic{}, false
}

func taskJugglerTaskIDMap(mapping app.TaskJugglerExportMapping) (map[string]string, []app.Diagnostic) {
	out := map[string]string{}
	diagnostics := []app.Diagnostic{}
	for _, entry := range mapping.TaskIDMap {
		if entry.TaskJugglerTaskID == "" || entry.RePlanTaskID == "" {
			diagnostics = append(diagnostics, app.Diagnostic{
				Code:     "taskjuggler_mapping_invalid",
				Message:  "TaskJuggler mapping contains an empty task id",
				Severity: "error",
			})
			continue
		}
		if previous, exists := out[entry.TaskJugglerTaskID]; exists && previous != entry.RePlanTaskID {
			diagnostics = append(diagnostics, app.Diagnostic{
				Code:     "taskjuggler_mapping_duplicate",
				Message:  fmt.Sprintf("TaskJuggler task id %s maps to both %s and %s", entry.TaskJugglerTaskID, previous, entry.RePlanTaskID),
				Severity: "error",
				Target:   app.Target{Kind: "taskjuggler_task", ID: entry.TaskJugglerTaskID},
			})
			continue
		}
		out[entry.TaskJugglerTaskID] = entry.RePlanTaskID
	}
	return out, diagnostics
}

func taskJugglerTaskScheduleReportPath(runDir string, manifest TaskJugglerRunManifest) string {
	for _, report := range manifest.Reports {
		if report.Kind == "taskjuggler_task_schedule_csv" && strings.TrimSpace(report.Path) != "" {
			return resolveRunArtifactPath(runDir, report.Path)
		}
	}
	return filepath.Join(runDir, "reports", "replan-task-schedule.csv")
}

func defaultTaskJugglerScenarioCandidateID(runID string) string {
	if strings.HasPrefix(runID, "taskjuggler.run.") {
		return "scenario.taskjuggler." + strings.TrimPrefix(runID, "taskjuggler.run.")
	}
	if strings.TrimSpace(runID) == "" {
		return "scenario.taskjuggler." + time.Now().UTC().Format("20060102T150405Z")
	}
	return "scenario.taskjuggler." + safeTaskJugglerPathSegment(runID)
}

func taskJugglerScenarioPreview(candidate TaskJugglerScenarioCandidate, manifest TaskJugglerRunManifest, actorID string, now time.Time) domain.Scenario {
	changes := make([]domain.ScenarioChange, 0, len(candidate.Changes))
	for _, change := range candidate.Changes {
		changes = append(changes, domain.ScenarioChange{
			Op:    change.Op,
			Path:  change.Path,
			Value: change.Value,
		})
	}
	baseRef := domain.Ref{Kind: "current_plan", ID: "current"}
	if manifest.ScenarioID != "" {
		baseRef = domain.Ref{Kind: "scenario", ID: manifest.ScenarioID}
	}
	return domain.Scenario{
		ID:        candidate.ID,
		Name:      "TaskJuggler schedule " + now.UTC().Format("2006-01-02 15:04:05Z"),
		Status:    "proposed",
		BaseRef:   baseRef,
		CreatedAt: now.Format(time.RFC3339),
		CreatedBy: domain.Ref{Kind: "actor", ID: actorID},
		Changes:   changes,
	}
}

func taskJugglerProposalPreview(scenario domain.Scenario, manifest TaskJugglerRunManifest, actorID string, now time.Time) domain.Proposal {
	return domain.Proposal{
		ID:        "proposal.taskjuggler." + strings.TrimPrefix(scenario.ID, "scenario.taskjuggler."),
		Type:      "taskjuggler_schedule_candidate",
		Status:    "open",
		CreatedAt: now.Format(time.RFC3339),
		CreatedBy: domain.Ref{Kind: "actor", ID: actorID},
		TargetRef: domain.Ref{Kind: "scenario", ID: scenario.ID},
		SuggestedChange: domain.SuggestedChange{
			Op:    "create",
			Path:  filepath.ToSlash(filepath.Join("scenarios", scenario.ID+".yaml")),
			Value: scenario.ID,
		},
		Rationale:            fmt.Sprintf("TaskJuggler run %s produced %d planned task date candidate changes.", manifest.ID, len(scenario.Changes)),
		RequiresConfirmation: true,
	}
}

func taskJugglerCandidatePlannedWrites(scenario domain.Scenario, proposal domain.Proposal) []app.PlannedWrite {
	scenarioContent, _ := yaml.Marshal(struct {
		Scenario domain.Scenario `yaml:"scenario"`
	}{Scenario: scenario})
	proposalContent, _ := yaml.Marshal(struct {
		Proposal domain.Proposal `yaml:"proposal"`
	}{Proposal: proposal})
	return []app.PlannedWrite{
		{
			Path:    filepath.ToSlash(filepath.Join("scenarios", scenario.ID+".yaml")),
			Action:  "create",
			Format:  "yaml",
			Content: string(scenarioContent),
		},
		{
			Path:    filepath.ToSlash(filepath.Join("proposals", "open", proposal.ID+".yaml")),
			Action:  "create",
			Format:  "yaml",
			Content: string(proposalContent),
		},
	}
}

func hasActor(ws domain.Workspace, actorID string) bool {
	for _, actor := range ws.Actors {
		if actor.ID == actorID {
			return true
		}
	}
	return false
}
