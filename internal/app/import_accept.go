package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

type ImportAcceptService struct {
	loader WorkspaceLoader
}

type ImportSnapshotReader interface {
	ReadImportMappedSnapshot(snapshotID string) ([]byte, string, error)
}

type ImportAcceptWriter interface {
	ApplyImportAcceptTasks(changes []ImportAcceptTaskChange, audit domain.AuditRecord) ([]string, error)
}

type ImportAcceptActualWriter interface {
	ApplyImportAcceptActuals(changes []ImportAcceptActualChange, audit domain.AuditRecord) ([]string, error)
}

type ImportAcceptResourceDemandWriter interface {
	ApplyImportAcceptResourceDemands(changes []ImportAcceptResourceDemandChange, audit domain.AuditRecord) ([]string, error)
}

type ImportAcceptResourceLinkWriter interface {
	ApplyImportAcceptResourceLinks(changes []ImportAcceptResourceLinkChange, audit domain.AuditRecord) ([]string, error)
}

type ImportAcceptTrackingStatusWriter interface {
	ApplyImportAcceptTrackingStatuses(changes []ImportAcceptTrackingStatusChange, audit domain.AuditRecord) ([]string, error)
}

type ImportAcceptDateWriter interface {
	ApplyImportAcceptDates(changes []ImportAcceptDateChange, audit domain.AuditRecord) ([]string, error)
}

type ImportAcceptDependencyWriter interface {
	ApplyImportAcceptDependencies(changes []ImportAcceptDependencyChange, audit domain.AuditRecord) ([]string, error)
}

type ImportAcceptRequest struct {
	SnapshotID            string
	DryRun                bool
	Apply                 bool
	ActorID               string
	Reason                string
	AcceptTimeEntries     bool
	AcceptResourceDemands bool
	AcceptResourceLinks   bool
	AcceptTrackingStatus  bool
	AcceptDates           bool
	AcceptDependencies    bool
	Now                   time.Time
}

type ImportAcceptResult struct {
	Applied            bool                 `json:"applied"`
	DryRun             bool                 `json:"dry_run"`
	SnapshotID         string               `json:"snapshot_id"`
	MappedPath         string               `json:"mapped_path,omitempty"`
	PlannedWrites      []PlannedWrite       `json:"planned_writes"`
	WrittenFiles       []string             `json:"written_files,omitempty"`
	Changes            []ImportAcceptChange `json:"changes"`
	AppliedChanges     []ImportAcceptChange `json:"applied_changes,omitempty"`
	AuditRecordPreview domain.AuditRecord   `json:"audit_record_preview,omitempty"`
	Validation         *ValidateResult      `json:"validation,omitempty"`
	Warnings           []Diagnostic         `json:"warnings"`
	Errors             []Diagnostic         `json:"errors"`
}

type ImportAcceptChange struct {
	Op          string  `json:"op"`
	TargetKind  string  `json:"target_kind"`
	TargetID    string  `json:"target_id,omitempty"`
	SourceID    string  `json:"source_id,omitempty"`
	Path        string  `json:"path,omitempty"`
	Field       string  `json:"field,omitempty"`
	From        *string `json:"from,omitempty"`
	To          *string `json:"to,omitempty"`
	Description string  `json:"description"`
}

type ImportAcceptTaskChange struct {
	Op        string           `json:"op"`
	TaskID    string           `json:"task_id"`
	Task      domain.Task      `json:"task,omitempty"`
	SourceRef domain.SourceRef `json:"source_ref,omitempty"`
}

type ImportAcceptActualChange struct {
	TimeEntryID string          `json:"time_entry_id"`
	Actual      domain.Actual   `json:"actual"`
	Evidence    domain.Evidence `json:"evidence"`
}

type ImportAcceptResourceDemandChange struct {
	SourceID string                `json:"source_id"`
	Demand   domain.ResourceDemand `json:"demand"`
}

type ImportAcceptResourceLinkChange struct {
	ResourceID string           `json:"resource_id"`
	SourceRef  domain.SourceRef `json:"source_ref"`
}

type ImportAcceptTrackingStatusChange struct {
	TaskID         string `json:"task_id"`
	TrackingStatus string `json:"tracking_status"`
}

type ImportAcceptDateChange struct {
	TaskID        string `json:"task_id"`
	PlannedStart  string `json:"planned_start"`
	PlannedFinish string `json:"planned_finish"`
}

type ImportAcceptDependencyChange struct {
	Op         string            `json:"op"`
	SourceID   string            `json:"source_id"`
	Dependency domain.Dependency `json:"dependency"`
	SourceRef  domain.SourceRef  `json:"source_ref,omitempty"`
}

type openProjectTimeEntryMatch struct {
	TaskID     string
	ResourceID string
	DemandID   string
}

func NewImportAcceptService(loader WorkspaceLoader) ImportAcceptService {
	return ImportAcceptService{loader: loader}
}

func (s ImportAcceptService) Accept(ctx context.Context, req ImportAcceptRequest) (ImportAcceptResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ImportAcceptResult{
		DryRun:         req.DryRun,
		SnapshotID:     req.SnapshotID,
		PlannedWrites:  []PlannedWrite{},
		WrittenFiles:   []string{},
		Changes:        []ImportAcceptChange{},
		AppliedChanges: []ImportAcceptChange{},
		Warnings:       []Diagnostic{},
		Errors:         []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if strings.TrimSpace(req.SnapshotID) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "snapshot_id_required",
			Message:  "import accept requires source snapshot id",
			Severity: "error",
		})
	}
	if req.DryRun && req.Apply {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "conflicting_import_accept_modes",
			Message:  "import accept accepts either --dry-run or --apply, not both",
			Severity: "error",
		})
	}
	acceptKinds := 0
	if req.AcceptTimeEntries {
		acceptKinds++
	}
	if req.AcceptResourceDemands {
		acceptKinds++
	}
	if req.AcceptResourceLinks {
		acceptKinds++
	}
	if req.AcceptTrackingStatus {
		acceptKinds++
	}
	if req.AcceptDates {
		acceptKinds++
	}
	if req.AcceptDependencies {
		acceptKinds++
	}
	if acceptKinds > 1 {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "conflicting_import_accept_modes",
			Message:  "import accept accepts only one specialized accept mode",
			Severity: "error",
		})
	}
	if !req.DryRun && !req.Apply {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "dry_run_required",
			Message:  "import accept requires --dry-run or explicit --apply",
			Severity: "error",
		})
	}
	if req.Apply && strings.TrimSpace(req.ActorID) == "" {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "actor_required",
			Message:  "import accept --apply requires --actor",
			Severity: "error",
			Target:   Target{Kind: "actor"},
		})
	}
	reader, ok := s.loader.(ImportSnapshotReader)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "snapshot_reader_unavailable",
			Message:  "project store does not support import snapshot reads",
			Severity: "error",
		})
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	data, mappedPath, err := reader.ReadImportMappedSnapshot(req.SnapshotID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			result.Errors = append(result.Errors, Diagnostic{
				Code:     "source_snapshot_not_found",
				Message:  fmt.Sprintf("source snapshot %s not found: %v", req.SnapshotID, err),
				Severity: "error",
				Target:   Target{Kind: "source_snapshot", ID: req.SnapshotID},
			})
			return result, ErrValidationFailed
		}
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "import_snapshot_read_error",
			Message:  fmt.Sprintf("failed to read import snapshot %s: %v", req.SnapshotID, err),
			Severity: "error",
			Target:   Target{Kind: "source_snapshot", ID: req.SnapshotID},
		})
		return result, ErrValidationFailed
	}
	result.MappedPath = mappedPath
	var mapped OpenProjectMappedSnapshot
	if err := json.Unmarshal(data, &mapped); err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "import_snapshot_parse_error",
			Message:  fmt.Sprintf("failed to parse mapped snapshot %s: %v", req.SnapshotID, err),
			Severity: "error",
			Target:   Target{Kind: "source_snapshot", ID: req.SnapshotID},
		})
		return result, ErrValidationFailed
	}
	if mapped.Snapshot.ID != "" && mapped.Snapshot.ID != req.SnapshotID {
		result.Warnings = append(result.Warnings, Diagnostic{
			Code:     "snapshot_id_mismatch",
			Message:  fmt.Sprintf("requested snapshot %s but mapped snapshot contains %s", req.SnapshotID, mapped.Snapshot.ID),
			Severity: "warning",
			Target:   Target{Kind: "source_snapshot", ID: req.SnapshotID},
		})
	}

	mode := selectImportAcceptMode(req)
	result.Changes = mode.preview(ws, mapped, &result.Warnings)
	result.PlannedWrites = importAcceptPlannedWrites(result.Changes)
	if ws.Project.EffectiveOpenProjectIDs() == domain.OpenProjectIDsPrivateBindings {
		result.Errors = append(result.Errors, privateOpenProjectIDDiagnostics(ws, mapped, mode.applied(result.Changes))...)
		if len(result.Errors) > 0 {
			return result, ErrValidationFailed
		}
	}
	if len(result.Changes) == 0 {
		result.Warnings = append(result.Warnings, Diagnostic{
			Code:     "import_accept_no_changes",
			Message:  "import accept dry-run produced no canonical changes",
			Severity: "warning",
			Target:   Target{Kind: "source_snapshot", ID: req.SnapshotID},
		})
	}
	result.AuditRecordPreview = importAcceptAuditRecord(req, len(result.Changes))
	if !req.Apply {
		return result, nil
	}
	if err := s.applyMode(ctx, req, mode, ws, mapped, &result); err != nil {
		return result, err
	}
	return result, nil
}

// openProjectIDFreeChangeOps are the import accept changes whose writers
// persist no OpenProject object id.
var openProjectIDFreeChangeOps = map[string]bool{
	"create_resource_demand":      true,
	"update_task_tracking_status": true,
	"update_task_planned_start":   true,
	"update_task_planned_finish":  true,
}

// privateOpenProjectIDDiagnostics rejects only changes whose corresponding
// writer persists an OpenProject object id. Dates, tracking statuses and
// resource demands contain no such id and therefore produce no diagnostics.
func privateOpenProjectIDDiagnostics(ws domain.Workspace, mapped OpenProjectMappedSnapshot, changes []ImportAcceptChange) []Diagnostic {
	type refusedKind struct {
		kind string
		ids  map[string]bool
	}
	order := []string{}
	refused := map[string]*refusedKind{}
	add := func(op, kind string, ids ...string) {
		item, ok := refused[op]
		if !ok {
			item = &refusedKind{kind: kind, ids: map[string]bool{}}
			refused[op] = item
			order = append(order, op)
		}
		for _, id := range ids {
			if strings.TrimSpace(id) != "" {
				item.ids[id] = true
			}
		}
	}
	for _, change := range changes {
		switch change.Op {
		case "create_task", "update_task_source_refs", "add_task_source_ref":
			add(change.Op, "task", change.TargetID)
		case "add_resource_source_ref":
			add(change.Op, "resource", change.TargetID)
		case "create_dependency", "update_dependency_source_refs":
			add(change.Op, "dependency", change.TargetID)
		case "create_evidence", "create_actual":
			// Refused even when the entry no longer matches a task; the ids
			// in the diagnostic are only for the operator.
			add(change.Op, "task/resource")
			for _, entry := range mapped.TimeEntryCandidates {
				if entry.SourceID != change.SourceID {
					continue
				}
				var ignored []Diagnostic
				if match, ok := matchOpenProjectTimeEntry(ws, mapped, entry, &ignored); ok {
					add(change.Op, "task/resource", match.TaskID, match.ResourceID)
				}
				break
			}
		default:
			// A change kind added later is refused until it is classified
			// here, so a new writer cannot leak ids by default.
			if !openProjectIDFreeChangeOps[change.Op] {
				add(change.Op, "change", change.TargetID)
			}
		}
	}
	diagnostics := make([]Diagnostic, 0, len(order))
	for _, op := range order {
		item := refused[op]
		ids := make([]string, 0, len(item.ids))
		for id := range item.ids {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		diagnostics = append(diagnostics, Diagnostic{
			Code:     "openproject_ids_private",
			Message:  fmt.Sprintf("refusing %s for %s %s: OpenProject object ids belong in the private bindings file; scripts/openproject-work-package-toolkit.sh binds work packages; dates and tracking statuses can still be accepted", op, item.kind, strings.Join(ids, ", ")),
			Severity: "error",
			Target:   Target{Kind: item.kind},
		})
	}
	return diagnostics
}

// importAcceptMode is one way to accept a snapshot: the default task and
// source-ref changes, or one of the specialized modes. Each mode keeps its
// own change builders, write action, diagnostics and store writer; the flow
// around them is applyMode.
type importAcceptMode struct {
	// preview lists the changes a dry run reports.
	preview func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptChange
	// applied picks the reported changes an apply writes.
	applied func(changes []ImportAcceptChange) []ImportAcceptChange
	// action is the planned write action of an apply.
	action string
	// prepare builds the changes an apply writes and returns how many there
	// are and the write; write reports false when the store cannot do it.
	prepare func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, actorID string) (int, importAcceptWrite)
	// nothingCode and nothingMessage warn when an apply has nothing safe to
	// write.
	nothingCode, nothingMessage string
	// unsupported names the writes in storage_write_not_supported.
	unsupported string
	// failedCode reports a failed write.
	failedCode string
}

type importAcceptWrite func(target any, audit domain.AuditRecord) ([]string, bool, error)

func selectImportAcceptMode(req ImportAcceptRequest) importAcceptMode {
	switch {
	case req.AcceptDates:
		return importAcceptDateMode
	case req.AcceptTrackingStatus:
		return importAcceptTrackingStatusMode
	case req.AcceptDependencies:
		return importAcceptDependencyMode
	case req.AcceptResourceLinks:
		return importAcceptResourceLinkMode
	case req.AcceptResourceDemands:
		return importAcceptResourceDemandMode
	case req.AcceptTimeEntries:
		return importAcceptTimeEntryMode
	default:
		return importAcceptTaskMode
	}
}

var importAcceptTaskMode = importAcceptMode{
	preview: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, _ *[]Diagnostic) []ImportAcceptChange {
		return buildImportAcceptChanges(ws, mapped)
	},
	applied: filterAppliedImportAcceptChanges,
	action:  "update",
	prepare: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, _ string) (int, importAcceptWrite) {
		changes := buildSafeImportAcceptTaskChanges(ws, mapped)
		return len(changes), func(target any, audit domain.AuditRecord) ([]string, bool, error) {
			writer, ok := target.(ImportAcceptWriter)
			if !ok {
				return nil, false, nil
			}
			files, err := writer.ApplyImportAcceptTasks(changes, audit)
			return files, true, err
		}
	},
	nothingCode:    "import_accept_no_applicable_changes",
	nothingMessage: "import accept --apply has no safe task/source-ref changes to write",
	unsupported:    "import accept writes",
	failedCode:     "import_accept_write_failed",
}

var importAcceptTimeEntryMode = importAcceptMode{
	preview: buildImportAcceptTimeEntryChanges,
	applied: filterActualImportAcceptChanges,
	action:  "append",
	prepare: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, actorID string) (int, importAcceptWrite) {
		var warnings []Diagnostic
		changes := buildSafeImportAcceptActualChanges(ws, mapped, actorID, &warnings)
		return len(changes), func(target any, audit domain.AuditRecord) ([]string, bool, error) {
			writer, ok := target.(ImportAcceptActualWriter)
			if !ok {
				return nil, false, nil
			}
			files, err := writer.ApplyImportAcceptActuals(changes, audit)
			return files, true, err
		}
	},
	nothingCode:    "import_accept_no_applicable_time_entries",
	nothingMessage: "import accept --apply --accept-time-entries has no safe time-entry actuals to write",
	unsupported:    "import time-entry actual writes",
	failedCode:     "import_accept_actual_write_failed",
}

var importAcceptResourceDemandMode = importAcceptMode{
	preview: buildImportAcceptResourceDemandChanges,
	applied: filterResourceDemandImportAcceptChanges,
	action:  "append",
	prepare: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, _ string) (int, importAcceptWrite) {
		var warnings []Diagnostic
		changes := buildSafeImportAcceptResourceDemandChanges(ws, mapped, &warnings)
		return len(changes), func(target any, audit domain.AuditRecord) ([]string, bool, error) {
			writer, ok := target.(ImportAcceptResourceDemandWriter)
			if !ok {
				return nil, false, nil
			}
			files, err := writer.ApplyImportAcceptResourceDemands(changes, audit)
			return files, true, err
		}
	},
	nothingCode:    "import_accept_no_applicable_resource_demands",
	nothingMessage: "import accept --apply --accept-resource-demands has no safe resource demands to write",
	unsupported:    "import resource-demand writes",
	failedCode:     "import_accept_resource_demand_write_failed",
}

var importAcceptResourceLinkMode = importAcceptMode{
	preview: buildImportAcceptResourceLinkChanges,
	applied: filterResourceLinkImportAcceptChanges,
	action:  "append",
	prepare: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, _ string) (int, importAcceptWrite) {
		var warnings []Diagnostic
		changes := buildSafeImportAcceptResourceLinkChanges(ws, mapped, &warnings)
		return len(changes), func(target any, audit domain.AuditRecord) ([]string, bool, error) {
			writer, ok := target.(ImportAcceptResourceLinkWriter)
			if !ok {
				return nil, false, nil
			}
			files, err := writer.ApplyImportAcceptResourceLinks(changes, audit)
			return files, true, err
		}
	},
	nothingCode:    "import_accept_no_applicable_resource_links",
	nothingMessage: "import accept --apply --accept-resource-links has no safe resource links to write",
	unsupported:    "import resource-link writes",
	failedCode:     "import_accept_resource_link_write_failed",
}

var importAcceptTrackingStatusMode = importAcceptMode{
	preview: buildImportAcceptTrackingStatusChanges,
	applied: filterTrackingStatusImportAcceptChanges,
	action:  "update",
	prepare: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, _ string) (int, importAcceptWrite) {
		var warnings []Diagnostic
		changes := buildSafeImportAcceptTrackingStatusChanges(ws, mapped, &warnings)
		return len(changes), func(target any, audit domain.AuditRecord) ([]string, bool, error) {
			writer, ok := target.(ImportAcceptTrackingStatusWriter)
			if !ok {
				return nil, false, nil
			}
			files, err := writer.ApplyImportAcceptTrackingStatuses(changes, audit)
			return files, true, err
		}
	},
	nothingCode:    "import_accept_no_applicable_tracking_statuses",
	nothingMessage: "import accept --apply --accept-tracking-status has no safe tracking statuses to write",
	unsupported:    "import tracking-status writes",
	failedCode:     "import_accept_tracking_status_write_failed",
}

var importAcceptDateMode = importAcceptMode{
	preview: buildImportAcceptDateChanges,
	applied: filterDateImportAcceptChanges,
	action:  "update",
	prepare: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, _ string) (int, importAcceptWrite) {
		var warnings []Diagnostic
		changes := buildSafeImportAcceptDateChanges(ws, mapped, &warnings)
		return len(changes), func(target any, audit domain.AuditRecord) ([]string, bool, error) {
			writer, ok := target.(ImportAcceptDateWriter)
			if !ok {
				return nil, false, nil
			}
			files, err := writer.ApplyImportAcceptDates(changes, audit)
			return files, true, err
		}
	},
	nothingCode:    "import_accept_no_applicable_dates",
	nothingMessage: "import accept --apply --accept-dates has no safe planned dates to write",
	unsupported:    "import date writes",
	failedCode:     "import_accept_date_write_failed",
}

var importAcceptDependencyMode = importAcceptMode{
	preview: buildImportAcceptDependencyChanges,
	applied: filterDependencyImportAcceptChanges,
	action:  "update",
	prepare: func(ws domain.Workspace, mapped OpenProjectMappedSnapshot, _ string) (int, importAcceptWrite) {
		var warnings []Diagnostic
		changes := buildSafeImportAcceptDependencyChanges(ws, mapped, &warnings)
		return len(changes), func(target any, audit domain.AuditRecord) ([]string, bool, error) {
			writer, ok := target.(ImportAcceptDependencyWriter)
			if !ok {
				return nil, false, nil
			}
			files, err := writer.ApplyImportAcceptDependencies(changes, audit)
			return files, true, err
		}
	},
	nothingCode:    "import_accept_no_applicable_dependencies",
	nothingMessage: "import accept --apply --accept-dependencies has no safe dependencies to write",
	unsupported:    "import dependency writes",
	failedCode:     "import_accept_dependency_write_failed",
}

// applyMode writes the safe changes of a mode through a staged write session
// and validates the result.
func (s ImportAcceptService) applyMode(ctx context.Context, req ImportAcceptRequest, mode importAcceptMode, ws domain.Workspace, mapped OpenProjectMappedSnapshot, result *ImportAcceptResult) error {
	count, write := mode.prepare(ws, mapped, req.ActorID)
	result.AppliedChanges = mode.applied(result.Changes)
	result.PlannedWrites = importAcceptPlannedWrites(result.AppliedChanges)
	for i := range result.PlannedWrites {
		result.PlannedWrites[i].Action = mode.action
	}
	if count == 0 {
		result.Warnings = append(result.Warnings, Diagnostic{
			Code:     mode.nothingCode,
			Message:  mode.nothingMessage,
			Severity: "warning",
			Target:   Target{Kind: "source_snapshot", ID: req.SnapshotID},
		})
		return nil
	}
	session := beginWrite(s.loader)
	writtenFiles, supported, err := write(session.target, result.AuditRecordPreview)
	if !supported {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "storage_write_not_supported",
			Message:  "configured project store does not support " + mode.unsupported,
			Severity: "error",
			Target:   Target{Kind: "project", ID: s.loader.ProjectPath()},
		})
		return ErrValidationFailed
	}
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     mode.failedCode,
			Message:  err.Error(),
			Severity: "error",
			Target:   Target{Kind: "source_snapshot", ID: req.SnapshotID},
		})
		return err
	}
	result.WrittenFiles = writtenFiles
	validation, written, err := session.finish(ctx, result.WrittenFiles)
	result.Validation = &validation
	result.WrittenFiles = written
	if err != nil {
		result.Errors = append(result.Errors, validation.Errors...)
		return err
	}
	result.Applied = true
	return nil
}

func buildImportAcceptChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot) []ImportAcceptChange {
	var changes []ImportAcceptChange
	for _, candidate := range mapped.TaskCandidates {
		if candidate.MatchedTaskID == "" || candidate.WouldCreate {
			targetID := canonicalTaskIDForOpenProject(candidate.SourceID)
			changes = append(changes, ImportAcceptChange{
				Op:          "create_task",
				TargetKind:  "task",
				TargetID:    targetID,
				SourceID:    candidate.SourceID,
				Path:        "canonical/tasks.yaml",
				Description: fmt.Sprintf("Create task %s from OpenProject work package %s.", targetID, candidate.SourceID),
			})
			continue
		}
		task, ok := findTaskByID(ws.Tasks, candidate.MatchedTaskID)
		if !ok {
			continue
		}
		if !taskHasSourceRef(task, "openproject", "work_package", candidate.SourceID) {
			changes = append(changes, ImportAcceptChange{
				Op:          "update_task_source_refs",
				TargetKind:  "task",
				TargetID:    candidate.MatchedTaskID,
				SourceID:    candidate.SourceID,
				Path:        "canonical/tasks.yaml",
				Description: fmt.Sprintf("Attach OpenProject source reference %s to task %s.", candidate.SourceID, candidate.MatchedTaskID),
			})
		}
		if candidate.MappedStatus != "" && task.TrackingStatus != candidate.MappedStatus {
			changes = append(changes, ImportAcceptChange{
				Op:          "update_task_tracking_status",
				TargetKind:  "task",
				TargetID:    candidate.MatchedTaskID,
				SourceID:    candidate.SourceID,
				Path:        "canonical/tasks.yaml",
				Description: fmt.Sprintf("Update task %s tracking_status from %s to %s.", candidate.MatchedTaskID, task.TrackingStatus, candidate.MappedStatus),
			})
		}
	}
	for _, candidate := range mapped.ResourceCandidates {
		if candidate.MatchedRefID == "" || candidate.WouldCreate {
			targetID := canonicalResourceIDForOpenProject(candidate.DisplayName, candidate.SourceID)
			changes = append(changes, ImportAcceptChange{
				Op:          "create_resource",
				TargetKind:  "resource",
				TargetID:    targetID,
				SourceID:    candidate.SourceID,
				Path:        "canonical/resources.yaml",
				Description: fmt.Sprintf("Create resource %s for OpenProject assignee %s.", targetID, candidate.DisplayName),
			})
		}
	}
	return changes
}

func buildImportAcceptTimeEntryChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptChange {
	var changes []ImportAcceptChange
	for _, entry := range mapped.TimeEntryCandidates {
		match, ok := matchOpenProjectTimeEntry(ws, mapped, entry, warnings)
		if !ok {
			continue
		}
		evidenceID := evidenceIDForOpenProjectTimeEntry(entry.SourceID)
		actualID := actualIDForOpenProjectTimeEntry(entry.SourceID)
		changes = append(changes, ImportAcceptChange{
			Op:          "create_evidence",
			TargetKind:  "evidence",
			TargetID:    evidenceID,
			SourceID:    entry.SourceID,
			Path:        evidencePath(entry.SpentOn),
			Description: fmt.Sprintf("Create evidence %s from OpenProject time entry %s.", evidenceID, entry.SourceID),
		})
		changes = append(changes, ImportAcceptChange{
			Op:          "create_actual",
			TargetKind:  "actual",
			TargetID:    actualID,
			SourceID:    entry.SourceID,
			Path:        actualsPath(entry.SpentOn),
			Description: fmt.Sprintf("Create actual %s for task %s, resource %s and demand %s from OpenProject time entry %s.", actualID, match.TaskID, match.ResourceID, match.DemandID, entry.SourceID),
		})
	}
	return changes
}

func buildImportAcceptResourceDemandChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptChange {
	var changes []ImportAcceptChange
	for _, candidate := range mapped.ResourceDemandCandidates {
		if !safeOpenProjectResourceDemandCandidate(ws, candidate, warnings) {
			continue
		}
		changes = append(changes, ImportAcceptChange{
			Op:          "create_resource_demand",
			TargetKind:  "resource_demand",
			TargetID:    candidate.Canonical.ID,
			SourceID:    candidate.SourceID,
			Path:        "canonical/resource_demands.yaml",
			Description: fmt.Sprintf("Create resource demand %s from OpenProject sidecar demand %s.", candidate.Canonical.ID, candidate.SourceID),
		})
	}
	return changes
}

func buildImportAcceptResourceLinkChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptChange {
	var changes []ImportAcceptChange
	for _, candidate := range mapped.ResourceCandidates {
		if !safeOpenProjectResourceLinkCandidate(ws, candidate, warnings) {
			continue
		}
		changes = append(changes, ImportAcceptChange{
			Op:          "add_resource_source_ref",
			TargetKind:  "resource",
			TargetID:    candidate.MatchedRefID,
			SourceID:    candidate.SourceID,
			Path:        "canonical/resources.yaml",
			Description: fmt.Sprintf("Attach OpenProject user %s source reference to resource %s.", candidate.SourceID, candidate.MatchedRefID),
		})
	}
	return changes
}

func buildImportAcceptTrackingStatusChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptChange {
	var changes []ImportAcceptChange
	for _, candidate := range mapped.TaskCandidates {
		task, ok := safeOpenProjectTrackingStatusCandidate(ws, candidate, warnings)
		if !ok {
			continue
		}
		changes = append(changes, ImportAcceptChange{
			Op:          "update_task_tracking_status",
			TargetKind:  "task",
			TargetID:    candidate.MatchedTaskID,
			SourceID:    candidate.SourceID,
			Path:        "canonical/tasks.yaml",
			Description: fmt.Sprintf("Update task %s tracking_status from %s to %s.", candidate.MatchedTaskID, task.TrackingStatus, candidate.MappedStatus),
		})
	}
	return changes
}

func buildImportAcceptDateChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptChange {
	var changes []ImportAcceptChange
	for _, candidate := range mapped.TaskCandidates {
		task, start, finish, ok := safeOpenProjectDateCandidate(ws, candidate, warnings)
		if !ok {
			continue
		}
		if start != "" && start != task.PlannedStart {
			changes = append(changes, ImportAcceptChange{
				Op: "update_task_planned_start", TargetKind: "task", TargetID: task.ID,
				SourceID: candidate.SourceID, Path: "canonical/tasks.yaml", Field: "planned_start",
				From: stringPointer(task.PlannedStart), To: stringPointer(start),
				Description: fmt.Sprintf("Update task %s planned_start from %s to %s.", task.ID, displayEmptyDate(task.PlannedStart), start),
			})
		}
		if finish != "" && finish != task.PlannedFinish {
			changes = append(changes, ImportAcceptChange{
				Op: "update_task_planned_finish", TargetKind: "task", TargetID: task.ID,
				SourceID: candidate.SourceID, Path: "canonical/tasks.yaml", Field: "planned_finish",
				From: stringPointer(task.PlannedFinish), To: stringPointer(finish),
				Description: fmt.Sprintf("Update task %s planned_finish from %s to %s.", task.ID, displayEmptyDate(task.PlannedFinish), finish),
			})
		}
	}
	return changes
}

func stringPointer(value string) *string { return &value }

func buildImportAcceptDependencyChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptChange {
	var changes []ImportAcceptChange
	seen := newDependencyAcceptSeen()
	for _, candidate := range mapped.DependencyCandidates {
		dependency, op, ok := safeOpenProjectDependencyCandidate(ws, candidate, warnings)
		if !ok {
			continue
		}
		if !seen.add(candidate, dependency, op, warnings) {
			continue
		}
		description := fmt.Sprintf("Create dependency %s from OpenProject relation %s.", dependency.ID, candidate.SourceID)
		if op == "update_dependency_source_refs" {
			description = fmt.Sprintf("Attach OpenProject relation %s source reference to dependency %s.", candidate.SourceID, dependency.ID)
		}
		changes = append(changes, ImportAcceptChange{
			Op:          op,
			TargetKind:  "dependency",
			TargetID:    dependency.ID,
			SourceID:    candidate.SourceID,
			Path:        "canonical/dependencies.yaml",
			Description: description,
		})
	}
	return changes
}

func buildSafeImportAcceptTaskChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot) []ImportAcceptTaskChange {
	var changes []ImportAcceptTaskChange
	for _, candidate := range mapped.TaskCandidates {
		sourceRef := domain.SourceRef{
			Source:     "openproject",
			ObjectType: "work_package",
			ObjectID:   candidate.SourceID,
			URL:        candidate.SourceURL,
		}
		if candidate.MatchedTaskID == "" || candidate.WouldCreate {
			taskID := canonicalTaskIDForOpenProject(candidate.SourceID)
			if _, exists := findTaskByID(ws.Tasks, taskID); exists {
				continue
			}
			changes = append(changes, ImportAcceptTaskChange{
				Op:     "create_task",
				TaskID: taskID,
				Task: domain.Task{
					ID:             taskID,
					Title:          candidate.Title,
					Description:    candidate.Description,
					Type:           "imported",
					TrackingStatus: "ready",
					Tags:           []string{"openproject", "imported"},
					SourceRefs:     []domain.SourceRef{sourceRef},
				},
			})
			continue
		}
		task, ok := findTaskByID(ws.Tasks, candidate.MatchedTaskID)
		if !ok {
			continue
		}
		if !taskHasSourceRef(task, "openproject", "work_package", candidate.SourceID) {
			changes = append(changes, ImportAcceptTaskChange{
				Op:        "add_task_source_ref",
				TaskID:    candidate.MatchedTaskID,
				SourceRef: sourceRef,
			})
		}
	}
	return changes
}

func buildSafeImportAcceptResourceDemandChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptResourceDemandChange {
	var changes []ImportAcceptResourceDemandChange
	for _, candidate := range mapped.ResourceDemandCandidates {
		if !safeOpenProjectResourceDemandCandidate(ws, candidate, warnings) {
			continue
		}
		changes = append(changes, ImportAcceptResourceDemandChange{
			SourceID: candidate.SourceID,
			Demand:   candidate.Canonical,
		})
	}
	return changes
}

func buildSafeImportAcceptResourceLinkChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptResourceLinkChange {
	var changes []ImportAcceptResourceLinkChange
	for _, candidate := range mapped.ResourceCandidates {
		if !safeOpenProjectResourceLinkCandidate(ws, candidate, warnings) {
			continue
		}
		changes = append(changes, ImportAcceptResourceLinkChange{
			ResourceID: candidate.MatchedRefID,
			SourceRef: domain.SourceRef{
				Source:     "openproject",
				ObjectType: "user",
				ObjectID:   candidate.SourceID,
				URL:        candidate.SourceHref,
			},
		})
	}
	return changes
}

func buildSafeImportAcceptTrackingStatusChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptTrackingStatusChange {
	var changes []ImportAcceptTrackingStatusChange
	for _, candidate := range mapped.TaskCandidates {
		if _, ok := safeOpenProjectTrackingStatusCandidate(ws, candidate, warnings); !ok {
			continue
		}
		changes = append(changes, ImportAcceptTrackingStatusChange{
			TaskID:         candidate.MatchedTaskID,
			TrackingStatus: candidate.MappedStatus,
		})
	}
	return changes
}

func buildSafeImportAcceptDateChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptDateChange {
	var changes []ImportAcceptDateChange
	for _, candidate := range mapped.TaskCandidates {
		task, start, finish, ok := safeOpenProjectDateCandidate(ws, candidate, warnings)
		if !ok {
			continue
		}
		changes = append(changes, ImportAcceptDateChange{TaskID: task.ID, PlannedStart: start, PlannedFinish: finish})
	}
	return changes
}

func buildSafeImportAcceptDependencyChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, warnings *[]Diagnostic) []ImportAcceptDependencyChange {
	var changes []ImportAcceptDependencyChange
	seen := newDependencyAcceptSeen()
	for _, candidate := range mapped.DependencyCandidates {
		dependency, op, ok := safeOpenProjectDependencyCandidate(ws, candidate, warnings)
		if !ok {
			continue
		}
		if !seen.add(candidate, dependency, op, warnings) {
			continue
		}
		changes = append(changes, ImportAcceptDependencyChange{
			Op:         op,
			SourceID:   candidate.SourceID,
			Dependency: dependency,
			SourceRef:  openProjectDependencySourceRef(candidate),
		})
	}
	return changes
}

func buildSafeImportAcceptActualChanges(ws domain.Workspace, mapped OpenProjectMappedSnapshot, actorID string, warnings *[]Diagnostic) []ImportAcceptActualChange {
	var changes []ImportAcceptActualChange
	for _, entry := range mapped.TimeEntryCandidates {
		match, ok := matchOpenProjectTimeEntry(ws, mapped, entry, warnings)
		if !ok {
			continue
		}
		evidenceID := evidenceIDForOpenProjectTimeEntry(entry.SourceID)
		actualID := actualIDForOpenProjectTimeEntry(entry.SourceID)
		occurredAt := occurredAtForOpenProjectSpentOn(entry.SpentOn)
		evidence := domain.Evidence{
			ID:         evidenceID,
			Type:       "pm_time_entry",
			Subtype:    "openproject_time_entry",
			Source:     "openproject",
			SourceRef:  domain.SourceRef{Source: "openproject", ObjectType: "time_entry", ObjectID: entry.SourceID, URL: entry.SourceURL},
			ObservedAt: occurredAt,
			Confidence: "observed",
			Summary:    openProjectTimeEntryEvidenceSummary(entry),
			Targets: []domain.Ref{
				{Kind: "task", ID: match.TaskID},
				{Kind: "resource", ID: match.ResourceID},
			},
		}
		actual := domain.Actual{
			ID:          actualID,
			Type:        "work_consumed",
			TaskRef:     domain.Ref{Kind: "task", ID: match.TaskID},
			DemandRef:   domain.Ref{Kind: "resource_demand", ID: match.DemandID},
			ResourceRef: domain.Ref{Kind: "resource", ID: match.ResourceID},
			Amount:      entry.Hours,
			Unit:        "hours",
			OccurredAt:  occurredAt,
			StatusDate:  entry.SpentOn,
			AcceptedBy:  domain.Ref{Kind: "actor", ID: actorID},
			EvidenceRefs: []domain.Ref{{
				Kind: "evidence",
				ID:   evidenceID,
			}},
		}
		changes = append(changes, ImportAcceptActualChange{
			TimeEntryID: entry.SourceID,
			Actual:      actual,
			Evidence:    evidence,
		})
	}
	return changes
}

func filterAppliedImportAcceptChanges(changes []ImportAcceptChange) []ImportAcceptChange {
	var out []ImportAcceptChange
	for _, change := range changes {
		if change.Op == "create_task" || change.Op == "update_task_source_refs" {
			out = append(out, change)
		}
	}
	return out
}

func filterActualImportAcceptChanges(changes []ImportAcceptChange) []ImportAcceptChange {
	var out []ImportAcceptChange
	for _, change := range changes {
		if change.Op == "create_evidence" || change.Op == "create_actual" {
			out = append(out, change)
		}
	}
	return out
}

func filterResourceDemandImportAcceptChanges(changes []ImportAcceptChange) []ImportAcceptChange {
	var out []ImportAcceptChange
	for _, change := range changes {
		if change.Op == "create_resource_demand" {
			out = append(out, change)
		}
	}
	return out
}

func filterResourceLinkImportAcceptChanges(changes []ImportAcceptChange) []ImportAcceptChange {
	var out []ImportAcceptChange
	for _, change := range changes {
		if change.Op == "add_resource_source_ref" {
			out = append(out, change)
		}
	}
	return out
}

func filterTrackingStatusImportAcceptChanges(changes []ImportAcceptChange) []ImportAcceptChange {
	var out []ImportAcceptChange
	for _, change := range changes {
		if change.Op == "update_task_tracking_status" {
			out = append(out, change)
		}
	}
	return out
}

func filterDateImportAcceptChanges(changes []ImportAcceptChange) []ImportAcceptChange {
	var out []ImportAcceptChange
	for _, change := range changes {
		if change.Op == "update_task_planned_start" || change.Op == "update_task_planned_finish" {
			out = append(out, change)
		}
	}
	return out
}

func filterDependencyImportAcceptChanges(changes []ImportAcceptChange) []ImportAcceptChange {
	var out []ImportAcceptChange
	for _, change := range changes {
		if change.Op == "create_dependency" || change.Op == "update_dependency_source_refs" {
			out = append(out, change)
		}
	}
	return out
}

func importAcceptAuditRecord(req ImportAcceptRequest, changeCount int) domain.AuditRecord {
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	reason := req.Reason
	if reason == "" {
		reason = fmt.Sprintf("Accepted import snapshot %s with %d preview changes.", req.SnapshotID, changeCount)
	}
	result := "dry_run"
	if req.Apply {
		result = "success"
	}
	return domain.AuditRecord{
		ID:         "audit." + now.UTC().Format("20060102T150405000000000Z") + ".import-accept-" + importAcceptAuditMode(req),
		OccurredAt: now.Format(time.RFC3339Nano),
		ActorRef:   domain.Ref{Kind: "actor", ID: req.ActorID},
		Operation:  "import_accept",
		TargetRef:  domain.Ref{Kind: "source_snapshot", ID: req.SnapshotID},
		Result:     result,
		Reason:     reason,
	}
}

func importAcceptAuditMode(req ImportAcceptRequest) string {
	if req.AcceptDates {
		return "dates"
	}
	if req.AcceptDependencies {
		return "dependencies"
	}
	if req.AcceptTrackingStatus {
		return "tracking-status"
	}
	if req.AcceptResourceLinks {
		return "resource-links"
	}
	if req.AcceptResourceDemands {
		return "resource-demands"
	}
	if req.AcceptTimeEntries {
		return "time-entries"
	}
	return "tasks"
}

func safeOpenProjectDateCandidate(ws domain.Workspace, candidate OpenProjectTaskCandidate, warnings *[]Diagnostic) (domain.Task, string, string, bool) {
	if strings.TrimSpace(candidate.SourceID) == "" {
		return domain.Task{}, "", "", false
	}
	if strings.TrimSpace(candidate.MatchedTaskID) == "" || candidate.WouldCreate {
		*warnings = append(*warnings, Diagnostic{Code: "openproject_dates_task_unmatched", Message: fmt.Sprintf("OpenProject work package %s is not matched to an existing canonical task", candidate.SourceID), Severity: "warning", Target: Target{Kind: "openproject_work_package", ID: candidate.SourceID}})
		return domain.Task{}, "", "", false
	}
	task, ok := findTaskByID(ws.Tasks, candidate.MatchedTaskID)
	if !ok {
		*warnings = append(*warnings, Diagnostic{Code: "openproject_dates_task_unknown", Message: fmt.Sprintf("OpenProject work package %s references unknown task %s", candidate.SourceID, candidate.MatchedTaskID), Severity: "warning", Target: Target{Kind: "task", ID: candidate.MatchedTaskID}})
		return domain.Task{}, "", "", false
	}
	start, finish := strings.TrimSpace(candidate.StartDate), strings.TrimSpace(candidate.DueDate)
	if start == "" && finish == "" {
		*warnings = append(*warnings, Diagnostic{Code: "openproject_dates_missing", Message: fmt.Sprintf("OpenProject work package %s has no start or due date", candidate.SourceID), Severity: "warning", Target: Target{Kind: "openproject_work_package", ID: candidate.SourceID}})
		return domain.Task{}, "", "", false
	}
	if start != "" {
		if parsed, err := time.Parse("2006-01-02", start); err != nil || parsed.Format("2006-01-02") != start {
			*warnings = append(*warnings, Diagnostic{Code: "openproject_planned_start_invalid", Message: fmt.Sprintf("OpenProject work package %s start date %q must use YYYY-MM-DD", candidate.SourceID, start), Severity: "warning", Target: Target{Kind: "openproject_work_package", ID: candidate.SourceID}})
			return domain.Task{}, "", "", false
		}
	} else {
		start = task.PlannedStart
	}
	if finish != "" {
		if parsed, err := time.Parse("2006-01-02", finish); err != nil || parsed.Format("2006-01-02") != finish {
			*warnings = append(*warnings, Diagnostic{Code: "openproject_planned_finish_invalid", Message: fmt.Sprintf("OpenProject work package %s due date %q must use YYYY-MM-DD", candidate.SourceID, finish), Severity: "warning", Target: Target{Kind: "openproject_work_package", ID: candidate.SourceID}})
			return domain.Task{}, "", "", false
		}
	} else {
		finish = task.PlannedFinish
	}
	if start != "" && finish != "" && finish < start {
		*warnings = append(*warnings, Diagnostic{Code: "openproject_dates_finish_before_start", Message: fmt.Sprintf("OpenProject work package %s would set planned_finish %s before planned_start %s", candidate.SourceID, finish, start), Severity: "warning", Target: Target{Kind: "openproject_work_package", ID: candidate.SourceID}})
		return domain.Task{}, "", "", false
	}
	if start == task.PlannedStart && finish == task.PlannedFinish {
		*warnings = append(*warnings, Diagnostic{Code: "openproject_dates_already_current", Message: fmt.Sprintf("task %s already has the imported OpenProject dates", task.ID), Severity: "warning", Target: Target{Kind: "task", ID: task.ID}})
		return domain.Task{}, "", "", false
	}
	return task, start, finish, true
}

func importAcceptPlannedWrites(changes []ImportAcceptChange) []PlannedWrite {
	seen := map[string]bool{}
	var writes []PlannedWrite
	for _, change := range changes {
		if change.Path == "" || seen[change.Path] {
			continue
		}
		seen[change.Path] = true
		writes = append(writes, PlannedWrite{
			Path:   change.Path,
			Action: "preview_update",
			Format: fileFormat(change.Path),
		})
	}
	return writes
}

func findTaskByID(tasks []domain.Task, id string) (domain.Task, bool) {
	for _, task := range tasks {
		if task.ID == id {
			return task, true
		}
	}
	return domain.Task{}, false
}

func taskHasSourceRef(task domain.Task, source, objectType, objectID string) bool {
	for _, ref := range task.SourceRefs {
		if ref.Source == source && ref.ObjectType == objectType && ref.ObjectID == objectID {
			return true
		}
	}
	return false
}

func resourceHasSourceRef(resource domain.Resource, source, objectType, objectID string) bool {
	for _, ref := range resource.SourceRefs {
		if ref.Source == source && ref.ObjectType == objectType && ref.ObjectID == objectID {
			return true
		}
	}
	return false
}

func findResourceByID(resources []domain.Resource, id string) (domain.Resource, bool) {
	for _, resource := range resources {
		if resource.ID == id {
			return resource, true
		}
	}
	return domain.Resource{}, false
}

func safeOpenProjectTrackingStatusCandidate(ws domain.Workspace, candidate OpenProjectTaskCandidate, warnings *[]Diagnostic) (domain.Task, bool) {
	if strings.TrimSpace(candidate.SourceID) == "" {
		return domain.Task{}, false
	}
	if strings.TrimSpace(candidate.MatchedTaskID) == "" || candidate.WouldCreate {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_tracking_status_task_unmatched",
			Message:  fmt.Sprintf("OpenProject work package %s is not matched to an existing canonical task", candidate.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_work_package", ID: candidate.SourceID},
		})
		return domain.Task{}, false
	}
	if strings.TrimSpace(candidate.MappedStatus) == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_tracking_status_unmapped",
			Message:  fmt.Sprintf("OpenProject work package %s has no mapped RePlan tracking status", candidate.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_work_package", ID: candidate.SourceID},
		})
		return domain.Task{}, false
	}
	task, ok := findTaskByID(ws.Tasks, candidate.MatchedTaskID)
	if !ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_tracking_status_task_unknown",
			Message:  fmt.Sprintf("OpenProject work package %s references unknown task %s", candidate.SourceID, candidate.MatchedTaskID),
			Severity: "warning",
			Target:   Target{Kind: "task", ID: candidate.MatchedTaskID},
		})
		return domain.Task{}, false
	}
	if task.TrackingStatus == candidate.MappedStatus {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_tracking_status_already_current",
			Message:  fmt.Sprintf("task %s already has tracking_status %s", candidate.MatchedTaskID, candidate.MappedStatus),
			Severity: "warning",
			Target:   Target{Kind: "task", ID: candidate.MatchedTaskID},
		})
		return domain.Task{}, false
	}
	return task, true
}

func safeOpenProjectResourceLinkCandidate(ws domain.Workspace, candidate OpenProjectResourceCandidate, warnings *[]Diagnostic) bool {
	if strings.TrimSpace(candidate.SourceID) == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_link_user_missing",
			Message:  fmt.Sprintf("OpenProject resource candidate %q has no user id", candidate.DisplayName),
			Severity: "warning",
			Target:   Target{Kind: "openproject_user", ID: candidate.DisplayName},
		})
		return false
	}
	if strings.TrimSpace(candidate.MatchedRefID) == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_link_resource_unmatched",
			Message:  fmt.Sprintf("OpenProject user %s is not matched to a canonical resource", candidate.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_user", ID: candidate.SourceID},
		})
		return false
	}
	if candidate.MatchReason != "resource_link" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_link_not_explicit",
			Message:  fmt.Sprintf("OpenProject user %s is matched by %q, not by explicit resource link", candidate.SourceID, candidate.MatchReason),
			Severity: "warning",
			Target:   Target{Kind: "openproject_user", ID: candidate.SourceID},
		})
		return false
	}
	resource, ok := findResourceByID(ws.Resources, candidate.MatchedRefID)
	if !ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_link_resource_unknown",
			Message:  fmt.Sprintf("OpenProject user %s is matched to unknown resource %s", candidate.SourceID, candidate.MatchedRefID),
			Severity: "warning",
			Target:   Target{Kind: "resource", ID: candidate.MatchedRefID},
		})
		return false
	}
	if resourceHasSourceRef(resource, "openproject", "user", candidate.SourceID) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_link_already_exists",
			Message:  fmt.Sprintf("resource %s already has OpenProject user source ref %s", candidate.MatchedRefID, candidate.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "resource", ID: candidate.MatchedRefID},
		})
		return false
	}
	return true
}

func safeOpenProjectResourceDemandCandidate(ws domain.Workspace, candidate OpenProjectResourceDemandCandidate, warnings *[]Diagnostic) bool {
	if candidate.AcceptStatus != "safe_to_accept" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_not_safe_to_accept",
			Message:  fmt.Sprintf("OpenProject resource-demand candidate %s has accept_status %q", candidate.SourceID, candidate.AcceptStatus),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: candidate.Canonical.ID},
		})
		return false
	}
	if candidate.Canonical.ID == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_id_missing",
			Message:  "OpenProject resource-demand candidate has no canonical id",
			Severity: "warning",
			Target:   Target{Kind: "openproject_resource_demand", ID: candidate.SourceID},
		})
		return false
	}
	if hasResourceDemandID(ws.ResourceDemands, candidate.Canonical.ID) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_already_exists",
			Message:  fmt.Sprintf("resource demand %s already exists", candidate.Canonical.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: candidate.Canonical.ID},
		})
		return false
	}
	if _, ok := findTaskByID(ws.Tasks, candidate.Canonical.TaskRef.ID); !ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_task_unmatched",
			Message:  fmt.Sprintf("resource demand %s cannot be accepted because task %s does not exist", candidate.Canonical.ID, candidate.Canonical.TaskRef.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: candidate.Canonical.ID},
		})
		return false
	}
	hasResource := strings.TrimSpace(candidate.Canonical.ResourceRef.ID) != ""
	hasPool := strings.TrimSpace(candidate.Canonical.ResourcePoolRef.ID) != ""
	if !hasResource && !hasPool {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_resource_missing",
			Message:  fmt.Sprintf("resource demand %s has no resource or resource pool", candidate.Canonical.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: candidate.Canonical.ID},
		})
		return false
	}
	if hasResource && !resourceExists(ws.Resources, candidate.Canonical.ResourceRef.ID) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_resource_unknown",
			Message:  fmt.Sprintf("resource demand %s references unknown resource %s", candidate.Canonical.ID, candidate.Canonical.ResourceRef.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource", ID: candidate.Canonical.ResourceRef.ID},
		})
		return false
	}
	if hasPool && !resourcePoolExists(ws.ResourcePools, candidate.Canonical.ResourcePoolRef.ID) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_resource_unknown",
			Message:  fmt.Sprintf("resource demand %s references unknown resource pool %s", candidate.Canonical.ID, candidate.Canonical.ResourcePoolRef.ID),
			Severity: "warning",
			Target:   Target{Kind: "resource_pool", ID: candidate.Canonical.ResourcePoolRef.ID},
		})
		return false
	}
	return true
}

func safeOpenProjectDependencyCandidate(ws domain.Workspace, candidate OpenProjectDependencyCandidate, warnings *[]Diagnostic) (domain.Dependency, string, bool) {
	dependency := dependencyFromOpenProjectCandidate(candidate)
	if candidate.AcceptStatus != "safe_to_accept" || !candidate.WouldCreate {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_not_safe_to_accept",
			Message:  fmt.Sprintf("OpenProject dependency candidate %s has accept_status %q", candidate.SourceID, candidate.AcceptStatus),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: candidate.SourceID},
		})
		return domain.Dependency{}, "", false
	}
	if strings.TrimSpace(candidate.SourceID) == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_source_missing",
			Message:  "OpenProject dependency candidate has no relation source id",
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation"},
		})
		return domain.Dependency{}, "", false
	}
	if strings.TrimSpace(dependency.ID) == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_id_missing",
			Message:  fmt.Sprintf("OpenProject relation %s has no dependency id", candidate.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: candidate.SourceID},
		})
		return domain.Dependency{}, "", false
	}
	if hasDependencyID(ws.Dependencies, dependency.ID) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_already_exists",
			Message:  fmt.Sprintf("dependency %s already exists", dependency.ID),
			Severity: "warning",
			Target:   Target{Kind: "dependency", ID: dependency.ID},
		})
		return domain.Dependency{}, "", false
	}
	if hasDependencySourceRef(ws.Dependencies, "openproject", "relation", candidate.SourceID) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_source_already_imported",
			Message:  fmt.Sprintf("OpenProject relation %s already has an accepted dependency", candidate.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: candidate.SourceID},
		})
		return domain.Dependency{}, "", false
	}
	if !domain.ValidDependencyType(dependency.Type) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_type_unsupported",
			Message:  fmt.Sprintf("OpenProject relation %s maps to unsupported dependency type %q", candidate.SourceID, dependency.Type),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: candidate.SourceID},
		})
		return domain.Dependency{}, "", false
	}
	if _, ok := findTaskByID(ws.Tasks, dependency.PredecessorRef.ID); !ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_predecessor_unmatched",
			Message:  fmt.Sprintf("OpenProject relation %s cannot be accepted because predecessor task %s does not exist", candidate.SourceID, dependency.PredecessorRef.ID),
			Severity: "warning",
			Target:   Target{Kind: "task", ID: dependency.PredecessorRef.ID},
		})
		return domain.Dependency{}, "", false
	}
	if _, ok := findTaskByID(ws.Tasks, dependency.SuccessorRef.ID); !ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_successor_unmatched",
			Message:  fmt.Sprintf("OpenProject relation %s cannot be accepted because successor task %s does not exist", candidate.SourceID, dependency.SuccessorRef.ID),
			Severity: "warning",
			Target:   Target{Kind: "task", ID: dependency.SuccessorRef.ID},
		})
		return domain.Dependency{}, "", false
	}
	if dependency.PredecessorRef.ID == dependency.SuccessorRef.ID {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_self_reference",
			Message:  fmt.Sprintf("OpenProject relation %s maps to a self-dependency on task %s", candidate.SourceID, dependency.PredecessorRef.ID),
			Severity: "warning",
			Target:   Target{Kind: "dependency", ID: dependency.ID},
		})
		return domain.Dependency{}, "", false
	}
	if existing, ok := findDependencyByTriple(ws.Dependencies, dependency.Type, dependency.PredecessorRef, dependency.SuccessorRef); ok {
		if dependencyHasSourceRef(existing, "openproject", "relation", candidate.SourceID) {
			*warnings = append(*warnings, Diagnostic{
				Code:     "openproject_dependency_source_already_imported",
				Message:  fmt.Sprintf("OpenProject relation %s is already linked to dependency %s", candidate.SourceID, existing.ID),
				Severity: "warning",
				Target:   Target{Kind: "dependency", ID: existing.ID},
			})
			return domain.Dependency{}, "", false
		}
		existing.SourceRefs = append(existing.SourceRefs, openProjectDependencySourceRef(candidate))
		return existing, "update_dependency_source_refs", true
	}
	return dependency, "create_dependency", true
}

func dependencyFromOpenProjectCandidate(candidate OpenProjectDependencyCandidate) domain.Dependency {
	requiredState := ""
	if candidate.DependencyType == "finish_to_start" {
		requiredState = "accepted"
	}
	return domain.Dependency{
		ID:             candidate.DependencyID,
		Type:           candidate.DependencyType,
		PredecessorRef: domain.Ref{Kind: "task", ID: candidate.PredecessorTaskID},
		SuccessorRef:   domain.Ref{Kind: "task", ID: candidate.SuccessorTaskID},
		RequiredState:  requiredState,
		Status:         "open",
		Criticality:    "required",
		SourceRefs:     []domain.SourceRef{openProjectDependencySourceRef(candidate)},
		Notes:          fmt.Sprintf("Imported from OpenProject %s relation %s.", candidate.RelationType, candidate.SourceID),
	}
}

func openProjectDependencySourceRef(candidate OpenProjectDependencyCandidate) domain.SourceRef {
	return domain.SourceRef{
		Source:     "openproject",
		ObjectType: "relation",
		ObjectID:   candidate.SourceID,
		URL:        candidate.SourceURL,
	}
}

type dependencyAcceptSeen struct {
	ids     map[string]string
	sources map[string]string
	triples map[string]string
}

func newDependencyAcceptSeen() dependencyAcceptSeen {
	return dependencyAcceptSeen{
		ids:     map[string]string{},
		sources: map[string]string{},
		triples: map[string]string{},
	}
}

func (s dependencyAcceptSeen) add(candidate OpenProjectDependencyCandidate, dependency domain.Dependency, op string, warnings *[]Diagnostic) bool {
	if op == "create_dependency" {
		if prev, ok := s.ids[dependency.ID]; ok {
			*warnings = append(*warnings, Diagnostic{
				Code:     "openproject_dependency_duplicate_candidate",
				Message:  fmt.Sprintf("OpenProject relation %s duplicates dependency candidate id already produced by relation %s", candidate.SourceID, prev),
				Severity: "warning",
				Target:   Target{Kind: "dependency", ID: dependency.ID},
			})
			return false
		}
	}
	sourceKey := "openproject:relation:" + candidate.SourceID
	if prev, ok := s.sources[sourceKey]; ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_duplicate_candidate",
			Message:  fmt.Sprintf("OpenProject relation %s duplicates source relation already produced by relation %s", candidate.SourceID, prev),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: candidate.SourceID},
		})
		return false
	}
	tripleKey := dependency.Type + ":" + dependency.PredecessorRef.Kind + ":" + dependency.PredecessorRef.ID + ":" + dependency.SuccessorRef.Kind + ":" + dependency.SuccessorRef.ID
	if prev, ok := s.triples[tripleKey]; ok {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_dependency_duplicate_candidate",
			Message:  fmt.Sprintf("OpenProject relation %s duplicates predecessor/successor/type triple already produced by relation %s", candidate.SourceID, prev),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: candidate.SourceID},
		})
		return false
	}
	if op == "create_dependency" {
		s.ids[dependency.ID] = candidate.SourceID
	}
	s.sources[sourceKey] = candidate.SourceID
	s.triples[tripleKey] = candidate.SourceID
	return true
}

func matchOpenProjectTimeEntry(ws domain.Workspace, mapped OpenProjectMappedSnapshot, entry OpenProjectTimeEntryCandidate, warnings *[]Diagnostic) (openProjectTimeEntryMatch, bool) {
	if entry.SourceID == "" {
		return openProjectTimeEntryMatch{}, false
	}
	if hasEvidenceSourceRef(ws.Evidence, "openproject", "time_entry", entry.SourceID) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_time_entry_already_imported",
			Message:  fmt.Sprintf("OpenProject time entry %s already has imported evidence", entry.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_time_entry", ID: entry.SourceID},
		})
		return openProjectTimeEntryMatch{}, false
	}
	if hasActualID(ws.Actuals, actualIDForOpenProjectTimeEntry(entry.SourceID)) {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_time_entry_actual_already_exists",
			Message:  fmt.Sprintf("actual for OpenProject time entry %s already exists", entry.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_time_entry", ID: entry.SourceID},
		})
		return openProjectTimeEntryMatch{}, false
	}
	taskID := matchTaskForOpenProjectTimeEntry(ws, mapped, entry)
	if taskID == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_time_entry_task_unmatched",
			Message:  fmt.Sprintf("OpenProject time entry %s cannot be accepted because work package %s is not matched to a RePlan task", entry.SourceID, entry.WorkPackageID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_time_entry", ID: entry.SourceID},
		})
		return openProjectTimeEntryMatch{}, false
	}
	resourceID := matchResourceForOpenProjectTimeEntry(ws, entry)
	if resourceID == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_time_entry_resource_unmatched",
			Message:  fmt.Sprintf("OpenProject time entry %s cannot be accepted because user %q is not matched to a RePlan resource", entry.SourceID, entry.UserName),
			Severity: "warning",
			Target:   Target{Kind: "openproject_time_entry", ID: entry.SourceID},
		})
		return openProjectTimeEntryMatch{}, false
	}
	demandID := matchDemandForOpenProjectTimeEntry(ws.ResourceDemands, taskID, resourceID)
	if demandID == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_time_entry_demand_unmatched",
			Message:  fmt.Sprintf("OpenProject time entry %s cannot be accepted because no hour demand matches task %s and resource %s", entry.SourceID, taskID, resourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_time_entry", ID: entry.SourceID},
		})
		return openProjectTimeEntryMatch{}, false
	}
	return openProjectTimeEntryMatch{TaskID: taskID, ResourceID: resourceID, DemandID: demandID}, true
}

func matchTaskForOpenProjectTimeEntry(ws domain.Workspace, mapped OpenProjectMappedSnapshot, entry OpenProjectTimeEntryCandidate) string {
	for _, task := range ws.Tasks {
		if taskHasSourceRef(task, "openproject", "work_package", entry.WorkPackageID) {
			return task.ID
		}
	}
	for _, candidate := range mapped.TaskCandidates {
		if candidate.SourceID != entry.WorkPackageID {
			continue
		}
		if candidate.MatchedTaskID != "" {
			return candidate.MatchedTaskID
		}
		canonicalID := canonicalTaskIDForOpenProject(candidate.SourceID)
		if _, ok := findTaskByID(ws.Tasks, canonicalID); ok {
			return canonicalID
		}
	}
	return ""
}

func matchResourceForOpenProjectTimeEntry(ws domain.Workspace, entry OpenProjectTimeEntryCandidate) string {
	for _, resource := range ws.Resources {
		for _, ref := range resource.SourceRefs {
			if ref.Source == "openproject" && ref.ObjectType == "user" && ref.ObjectID == lastPathSegment(entry.UserHref) && ref.ObjectID != "" {
				return resource.ID
			}
		}
	}
	return matchResourceByName(ws.Resources, entry.UserName)
}

func matchDemandForOpenProjectTimeEntry(demands []domain.ResourceDemand, taskID, resourceID string) string {
	for _, demand := range demands {
		if demand.TaskRef.ID == taskID && demand.ResourceRef.ID == resourceID && demand.Unit == "hours" {
			return demand.ID
		}
	}
	return ""
}

func hasEvidenceSourceRef(evidence []domain.Evidence, source, objectType, objectID string) bool {
	for _, item := range evidence {
		ref := item.SourceRef
		if ref.Source == source && ref.ObjectType == objectType && ref.ObjectID == objectID {
			return true
		}
	}
	return false
}

func hasResourceDemandID(demands []domain.ResourceDemand, id string) bool {
	for _, demand := range demands {
		if demand.ID == id {
			return true
		}
	}
	return false
}

func hasDependencyID(dependencies []domain.Dependency, id string) bool {
	for _, dependency := range dependencies {
		if dependency.ID == id {
			return true
		}
	}
	return false
}

func hasDependencySourceRef(dependencies []domain.Dependency, source, objectType, objectID string) bool {
	for _, dependency := range dependencies {
		if dependencyHasSourceRef(dependency, source, objectType, objectID) {
			return true
		}
	}
	return false
}

func dependencyHasSourceRef(dependency domain.Dependency, source, objectType, objectID string) bool {
	for _, ref := range dependency.SourceRefs {
		if ref.Source == source && ref.ObjectType == objectType && ref.ObjectID == objectID {
			return true
		}
	}
	return false
}

func findDependencyByTriple(dependencies []domain.Dependency, dependencyType string, predecessor, successor domain.Ref) (domain.Dependency, bool) {
	for _, dependency := range dependencies {
		if dependency.Type == dependencyType &&
			dependency.PredecessorRef.Kind == predecessor.Kind &&
			dependency.PredecessorRef.ID == predecessor.ID &&
			dependency.SuccessorRef.Kind == successor.Kind &&
			dependency.SuccessorRef.ID == successor.ID {
			return dependency, true
		}
	}
	return domain.Dependency{}, false
}

func canonicalTaskIDForOpenProject(sourceID string) string {
	return "task.op." + strings.ToLower(strings.TrimSpace(sourceID))
}

func canonicalResourceIDForOpenProject(displayName string, sourceID string) string {
	slug := strings.ToLower(strings.TrimSpace(displayName))
	slug = strings.ReplaceAll(slug, " ", "-")
	if slug == "" {
		slug = sourceID
	}
	return "resource.openproject." + slug
}

func evidenceIDForOpenProjectTimeEntry(sourceID string) string {
	return "evidence.openproject.time-entry." + strings.ToLower(strings.TrimSpace(sourceID))
}

func actualIDForOpenProjectTimeEntry(sourceID string) string {
	return "actual.openproject.time-entry." + strings.ToLower(strings.TrimSpace(sourceID)) + ".work-consumed"
}

func occurredAtForOpenProjectSpentOn(spentOn string) string {
	if strings.TrimSpace(spentOn) == "" {
		return time.Now().UTC().Format(time.RFC3339)
	}
	return spentOn + "T12:00:00Z"
}

// evidencePath returns the ISO-week shard for either an RFC3339 observation
// timestamp or the date-only value used by imported OpenProject time entries.
func evidencePath(statusDate string) string {
	if len(statusDate) >= len("2006-01-02") {
		statusDate = statusDate[:len("2006-01-02")]
	}
	t, err := time.Parse("2006-01-02", statusDate)
	if err != nil {
		return "evidence/evidence.unscheduled.yaml"
	}
	year, week := t.ISOWeek()
	return fmt.Sprintf("evidence/evidence.%04d-W%02d.yaml", year, week)
}

func openProjectTimeEntryEvidenceSummary(entry OpenProjectTimeEntryCandidate) string {
	parts := []string{fmt.Sprintf("OpenProject time entry %s", entry.SourceID)}
	if entry.Hours > 0 {
		parts = append(parts, fmt.Sprintf("%.2f hours", entry.Hours))
	}
	if entry.UserName != "" {
		parts = append(parts, "by "+entry.UserName)
	}
	if entry.ActivityName != "" {
		parts = append(parts, "activity "+entry.ActivityName)
	}
	if entry.Comment != "" {
		parts = append(parts, "comment: "+entry.Comment)
	}
	return strings.Join(parts, "; ")
}

func fileFormat(path string) string {
	if strings.HasSuffix(path, ".json") {
		return "json"
	}
	return "yaml"
}
