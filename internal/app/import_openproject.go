package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type OpenProjectImportService struct {
	loader WorkspaceLoader
}

type OpenProjectImportRequest struct {
	FromFile       string
	DryRun         bool
	Profile        string
	BaseURL        string
	ProjectID      string
	TokenEnv       string
	HostHeader     string
	ForwardedProto string
	// PerimeterKeyEnv names the environment variable that holds the key a
	// reverse proxy in front of OpenProject asks of API clients; the key
	// is sent in PerimeterHeader and never printed.
	PerimeterKeyEnv          string
	PerimeterHeader          string
	Since                    string
	IncludeClosed            bool
	IncludeTimeEntries       bool
	IncludeRelations         bool
	ResourceLinksSidecar     string
	ResourceDemandsSidecar   string
	WorkPackageID            string
	PageSize                 int
	RequestTimeoutSeconds    float64
	RequestRetries           int
	RequestRetryDelaySeconds float64
	Now                      time.Time
}

type OpenProjectImportResult struct {
	Applied                  bool                                 `json:"applied"`
	DryRun                   bool                                 `json:"dry_run"`
	Source                   string                               `json:"source"`
	FromFile                 string                               `json:"from_file,omitempty"`
	Profile                  *SourceProfileSummary                `json:"profile,omitempty"`
	Snapshot                 OpenProjectSnapshotSummary           `json:"snapshot"`
	TaskCandidates           []OpenProjectTaskCandidate           `json:"task_candidates"`
	ResourceCandidates       []OpenProjectResourceCandidate       `json:"resource_candidates"`
	ResourceDemandCandidates []OpenProjectResourceDemandCandidate `json:"resource_demand_candidates,omitempty"`
	DependencyCandidates     []OpenProjectDependencyCandidate     `json:"dependency_candidates,omitempty"`
	TimeEntryCandidates      []OpenProjectTimeEntryCandidate      `json:"time_entry_candidates,omitempty"`
	PlannedWrites            []PlannedWrite                       `json:"planned_writes"`
	WrittenFiles             []string                             `json:"written_files,omitempty"`
	Warnings                 []Diagnostic                         `json:"warnings"`
	Errors                   []Diagnostic                         `json:"errors"`
	OpenProjectRequest       OpenProjectRequestMetadata           `json:"openproject_request"`
}

type OpenProjectRequestMetadata struct {
	TimeoutSeconds    float64 `json:"timeout_seconds"`
	Retries           int     `json:"retries"`
	RetryDelaySeconds float64 `json:"retry_delay_seconds"`
}

type OpenProjectSnapshotSummary struct {
	ID             string `json:"id,omitempty"`
	Key            string `json:"key,omitempty"`
	CollectionType string `json:"collection_type,omitempty"`
	DeclaredTotal  int    `json:"declared_total,omitempty"`
	DeclaredCount  int    `json:"declared_count,omitempty"`
	LoadedCount    int    `json:"loaded_count"`
	SourceMode     string `json:"source_mode,omitempty"`
	PageSize       int    `json:"page_size,omitempty"`
	PagesLoaded    int    `json:"pages_loaded,omitempty"`
}

type SourceProfileSummary struct {
	ID                   string            `json:"id"`
	Source               string            `json:"source"`
	BaseURL              string            `json:"base_url,omitempty"`
	ProjectID            string            `json:"project_id,omitempty"`
	TokenEnv             string            `json:"token_env,omitempty"`
	CredentialProfileRef string            `json:"credential_profile_ref,omitempty"`
	DefaultQuery         map[string]string `json:"default_query,omitempty"`
	Repository           string            `json:"repository,omitempty"`
	StatusMap            map[string]string `json:"status_map,omitempty"`
	StartField           string            `json:"start_field,omitempty"`
	FinishField          string            `json:"finish_field,omitempty"`
	EstimateField        string            `json:"estimate_field,omitempty"`
}

type OpenProjectImportSnapshotWriter interface {
	WriteOpenProjectImportSnapshot(snapshot OpenProjectImportSnapshotWrite) ([]string, error)
}

type ImportSnapshotWriter interface {
	WriteImportSnapshot(source string, snapshot OpenProjectImportSnapshotWrite) ([]string, error)
}

type OpenProjectImportSnapshotWrite struct {
	SnapshotKey  string
	ManifestYAML []byte
	RawJSON      []byte
	MappedJSON   []byte
}

type OpenProjectImportManifest struct {
	SourceSnapshot OpenProjectImportManifestBody `json:"source_snapshot" yaml:"source_snapshot"`
}

type OpenProjectImportManifestBody struct {
	ID                   string                         `json:"id" yaml:"id"`
	Source               string                         `json:"source" yaml:"source"`
	SourceMode           string                         `json:"source_mode" yaml:"source_mode"`
	ImportedAt           string                         `json:"imported_at" yaml:"imported_at"`
	ProfileID            string                         `json:"profile_id,omitempty" yaml:"profile_id,omitempty"`
	BaseURL              string                         `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	ProjectID            string                         `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	TokenEnv             string                         `json:"token_env,omitempty" yaml:"token_env,omitempty"`
	CredentialProfileRef string                         `json:"credential_profile_ref,omitempty" yaml:"credential_profile_ref,omitempty"`
	Query                OpenProjectImportManifestQuery `json:"query" yaml:"query"`
	RawLocation          string                         `json:"raw_location" yaml:"raw_location"`
	MappedLocation       string                         `json:"mapped_location" yaml:"mapped_location"`
	AdapterVersion       string                         `json:"adapter_version" yaml:"adapter_version"`
	ObjectCount          int                            `json:"object_count" yaml:"object_count"`
	WarningCount         int                            `json:"warning_count" yaml:"warning_count"`
}

type OpenProjectImportManifestQuery struct {
	Since                  string `json:"since,omitempty" yaml:"since,omitempty"`
	WorkPackageID          string `json:"work_package_id,omitempty" yaml:"work_package_id,omitempty"`
	PageSize               int    `json:"page_size,omitempty" yaml:"page_size,omitempty"`
	IncludeClosed          bool   `json:"include_closed,omitempty" yaml:"include_closed,omitempty"`
	IncludeTimeEntries     bool   `json:"include_time_entries,omitempty" yaml:"include_time_entries,omitempty"`
	IncludeRelations       bool   `json:"include_relations,omitempty" yaml:"include_relations,omitempty"`
	ResourceLinksSidecar   string `json:"resource_links_sidecar,omitempty" yaml:"resource_links_sidecar,omitempty"`
	ResourceDemandsSidecar string `json:"resource_demands_sidecar,omitempty" yaml:"resource_demands_sidecar,omitempty"`
}

type OpenProjectMappedSnapshot struct {
	Source                   string                               `json:"source,omitempty"`
	Snapshot                 OpenProjectSnapshotSummary           `json:"snapshot"`
	Profile                  *SourceProfileSummary                `json:"profile,omitempty"`
	TaskCandidates           []OpenProjectTaskCandidate           `json:"task_candidates"`
	ResourceCandidates       []OpenProjectResourceCandidate       `json:"resource_candidates"`
	ResourceDemandCandidates []OpenProjectResourceDemandCandidate `json:"resource_demand_candidates,omitempty"`
	DependencyCandidates     []OpenProjectDependencyCandidate     `json:"dependency_candidates,omitempty"`
	TimeEntryCandidates      []OpenProjectTimeEntryCandidate      `json:"time_entry_candidates,omitempty"`
}

// ImportMappedSnapshot is the source-neutral snapshot contract consumed by
// import accept. The alias preserves the existing OpenProject adapter API.
type ImportMappedSnapshot = OpenProjectMappedSnapshot

type OpenProjectTaskCandidate struct {
	Source         string             `json:"source,omitempty"`
	ObjectType     string             `json:"object_type,omitempty"`
	NumericID      int64              `json:"numeric_id,omitempty"`
	ParentRef      *domain.SourceRef  `json:"parent_ref,omitempty"`
	SourceID       string             `json:"source_id"`
	SourceURL      string             `json:"source_url,omitempty"`
	Title          string             `json:"title"`
	Description    string             `json:"description,omitempty"`
	SourceStatus   string             `json:"source_status,omitempty"`
	MappedStatus   string             `json:"mapped_status,omitempty"`
	AssigneeName   string             `json:"assignee_name,omitempty"`
	AssigneeHref   string             `json:"assignee_href,omitempty"`
	EstimatedHours float64            `json:"estimated_hours,omitempty"`
	SpentHours     float64            `json:"spent_hours,omitempty"`
	StartDate      string             `json:"start_date,omitempty"`
	DueDate        string             `json:"due_date,omitempty"`
	MatchedTaskID  string             `json:"matched_task_id,omitempty"`
	MatchReason    string             `json:"match_reason,omitempty"`
	WouldCreate    bool               `json:"would_create"`
	SourceRefs     []domain.SourceRef `json:"source_refs"`
}

type OpenProjectResourceCandidate struct {
	Source       string `json:"source,omitempty"`
	ObjectType   string `json:"object_type,omitempty"`
	SourceID     string `json:"source_id,omitempty"`
	SourceHref   string `json:"source_href,omitempty"`
	DisplayName  string `json:"display_name"`
	MatchedRefID string `json:"matched_ref_id,omitempty"`
	MatchReason  string `json:"match_reason,omitempty"`
	WouldCreate  bool   `json:"would_create"`
}

type OpenProjectTimeEntryCandidate struct {
	SourceID           string             `json:"source_id"`
	SourceURL          string             `json:"source_url,omitempty"`
	WorkPackageID      string             `json:"work_package_id,omitempty"`
	WorkPackageHref    string             `json:"work_package_href,omitempty"`
	UserName           string             `json:"user_name,omitempty"`
	UserHref           string             `json:"user_href,omitempty"`
	ActivityName       string             `json:"activity_name,omitempty"`
	ActivityHref       string             `json:"activity_href,omitempty"`
	Hours              float64            `json:"hours,omitempty"`
	SpentOn            string             `json:"spent_on,omitempty"`
	Comment            string             `json:"comment,omitempty"`
	WouldCreateActual  bool               `json:"would_create_actual"`
	ActualAcceptStatus string             `json:"actual_accept_status"`
	SourceRefs         []domain.SourceRef `json:"source_refs"`
}

type OpenProjectResourceDemandCandidate struct {
	TrackerSource string                `json:"tracker_source,omitempty"`
	Source        string                `json:"source"`
	SourceID      string                `json:"source_id"`
	WorkPackageID string                `json:"work_package_id"`
	MatchedTaskID string                `json:"matched_task_id,omitempty"`
	WouldCreate   bool                  `json:"would_create"`
	AcceptStatus  string                `json:"accept_status"`
	Canonical     domain.ResourceDemand `json:"canonical"`
	Diagnostics   []Diagnostic          `json:"diagnostics,omitempty"`
}

type OpenProjectDependencyCandidate struct {
	Source            string       `json:"source,omitempty"`
	ObjectType        string       `json:"object_type,omitempty"`
	SourceID          string       `json:"source_id"`
	SourceURL         string       `json:"source_url,omitempty"`
	RelationType      string       `json:"relation_type"`
	FromWorkPackageID string       `json:"from_work_package_id"`
	ToWorkPackageID   string       `json:"to_work_package_id"`
	PredecessorTaskID string       `json:"predecessor_task_id,omitempty"`
	SuccessorTaskID   string       `json:"successor_task_id,omitempty"`
	DependencyID      string       `json:"dependency_id"`
	DependencyType    string       `json:"dependency_type"`
	WouldCreate       bool         `json:"would_create"`
	AcceptStatus      string       `json:"accept_status"`
	Diagnostics       []Diagnostic `json:"diagnostics,omitempty"`
}

type openProjectResourceDemandSidecar struct {
	OpenProjectResourceDemands []openProjectResourceDemandSidecarEntry `yaml:"openproject_resource_demands"`
}

type openProjectResourceDemandSidecarEntry struct {
	WorkPackageID  string                     `yaml:"work_package_id"`
	WorkPackageRef domain.SourceRef           `yaml:"work_package_ref"`
	Note           string                     `yaml:"note"`
	Demands        []openProjectSidecarDemand `yaml:"demands"`
}

type openProjectSidecarDemand struct {
	ID              string     `yaml:"id"`
	ResourceRef     domain.Ref `yaml:"resource_ref"`
	ResourcePoolRef domain.Ref `yaml:"resource_pool_ref"`
	ResourceClass   string     `yaml:"resource_class"`
	PlannedAmount   float64    `yaml:"planned_amount"`
	RemainingAmount *float64   `yaml:"remaining_amount"`
	Unit            string     `yaml:"unit"`
	Role            string     `yaml:"role"`
	Criticality     string     `yaml:"criticality"`
	Status          string     `yaml:"status"`
	Confidence      string     `yaml:"confidence"`
	Source          string     `yaml:"source"`
}

type openProjectResourceLinksSidecar struct {
	OpenProjectResourceLinks []openProjectResourceLink `yaml:"openproject_resource_links"`
}

type openProjectResourceLink struct {
	UserID      string     `yaml:"user_id"`
	UserHref    string     `yaml:"user_href"`
	DisplayName string     `yaml:"display_name"`
	ResourceRef domain.Ref `yaml:"resource_ref"`
}

type openProjectCollection struct {
	Type     string `json:"_type"`
	Total    int    `json:"total"`
	Count    int    `json:"count"`
	PageSize int    `json:"pageSize"`
	Offset   int    `json:"offset"`
	Embedded struct {
		Elements  []openProjectWorkPackage `json:"elements"`
		Relations struct {
			Elements []openProjectRelation `json:"elements"`
		} `json:"relations"`
		TimeEntries struct {
			Elements []openProjectTimeEntry `json:"elements"`
		} `json:"time_entries"`
	} `json:"_embedded"`
}

type openProjectAPIError struct {
	Type            string `json:"_type"`
	ErrorIdentifier string `json:"errorIdentifier"`
	Message         string `json:"message"`
}

type openProjectText struct {
	Raw  string `json:"raw"`
	HTML string `json:"html"`
}

type openProjectNamedValue struct {
	Name string `json:"name"`
}

type openProjectLink struct {
	Href  string `json:"href"`
	Title string `json:"title"`
}

type openProjectWorkPackageLinks struct {
	Self     openProjectLink `json:"self"`
	Status   openProjectLink `json:"status"`
	Assignee openProjectLink `json:"assignee"`
}

type openProjectWorkPackage struct {
	ID            int                         `json:"id"`
	Subject       string                      `json:"subject"`
	Description   openProjectText             `json:"description"`
	Status        openProjectNamedValue       `json:"status"`
	Assignee      openProjectNamedValue       `json:"assignee"`
	EstimatedTime string                      `json:"estimatedTime"`
	SpentTime     string                      `json:"spentTime"`
	StartDate     string                      `json:"startDate"`
	DueDate       string                      `json:"dueDate"`
	Links         openProjectWorkPackageLinks `json:"_links"`
}

type openProjectTimeEntryCollection struct {
	Type     string `json:"_type"`
	Total    int    `json:"total"`
	Count    int    `json:"count"`
	PageSize int    `json:"pageSize"`
	Offset   int    `json:"offset"`
	Embedded struct {
		Elements []openProjectTimeEntry `json:"elements"`
	} `json:"_embedded"`
}

type openProjectProject struct {
	ID         int    `json:"id"`
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
}

type openProjectTimeEntryLinks struct {
	Self     openProjectLink `json:"self"`
	Project  openProjectLink `json:"project"`
	Entity   openProjectLink `json:"entity"`
	User     openProjectLink `json:"user"`
	Activity openProjectLink `json:"activity"`
}

type openProjectTimeEntry struct {
	ID        int                       `json:"id"`
	Comment   openProjectText           `json:"comment"`
	SpentOn   string                    `json:"spentOn"`
	Hours     string                    `json:"hours"`
	CreatedAt string                    `json:"createdAt"`
	UpdatedAt string                    `json:"updatedAt"`
	Links     openProjectTimeEntryLinks `json:"_links"`
}

type openProjectRelationCollection struct {
	Type     string `json:"_type"`
	Total    int    `json:"total"`
	Count    int    `json:"count"`
	PageSize int    `json:"pageSize"`
	Offset   int    `json:"offset"`
	Embedded struct {
		Elements []openProjectRelation `json:"elements"`
	} `json:"_embedded"`
}

type openProjectRelationLinks struct {
	Self openProjectLink `json:"self"`
	From openProjectLink `json:"from"`
	To   openProjectLink `json:"to"`
}

type openProjectRelation struct {
	ID          int                      `json:"id"`
	Name        string                   `json:"name"`
	Type        string                   `json:"type"`
	ReverseType string                   `json:"reverseType"`
	Lag         any                      `json:"lag"`
	Description any                      `json:"description"`
	Links       openProjectRelationLinks `json:"_links"`
}

func NewOpenProjectImportService(loader WorkspaceLoader) OpenProjectImportService {
	return OpenProjectImportService{loader: loader}
}

func (s OpenProjectImportService) Import(ctx context.Context, req OpenProjectImportRequest) (OpenProjectImportResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := OpenProjectImportResult{
		DryRun:                   req.DryRun,
		Source:                   "openproject",
		FromFile:                 req.FromFile,
		TaskCandidates:           []OpenProjectTaskCandidate{},
		ResourceCandidates:       []OpenProjectResourceCandidate{},
		ResourceDemandCandidates: []OpenProjectResourceDemandCandidate{},
		DependencyCandidates:     []OpenProjectDependencyCandidate{},
		TimeEntryCandidates:      []OpenProjectTimeEntryCandidate{},
		PlannedWrites:            []PlannedWrite{},
		WrittenFiles:             []string{},
		Warnings:                 []Diagnostic{},
		Errors:                   []Diagnostic{},
		OpenProjectRequest: OpenProjectRequestMetadata{
			TimeoutSeconds:    req.RequestTimeoutSeconds,
			Retries:           req.RequestRetries,
			RetryDelaySeconds: req.RequestRetryDelaySeconds,
		},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if !req.DryRun && strings.TrimSpace(req.FromFile) == "" && ws.Project.EffectiveOpenProjectIDs() == domain.OpenProjectIDsPrivateBindings {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "openproject_ids_private",
			Message:  "live OpenProject snapshots contain object ids that belong in the private bindings file; use --dry-run, which does not write the snapshot",
			Severity: "error",
			Target:   Target{Kind: "project", ID: ws.Project.ID},
		})
	}
	var selectedProfile domain.SourceProfile
	if strings.TrimSpace(req.Profile) != "" {
		profile, ok := findSourceProfile(ws.Project.SourceProfiles, req.Profile, "openproject")
		if !ok {
			result.Errors = append(result.Errors, Diagnostic{
				Code:     "source_profile_not_found",
				Message:  fmt.Sprintf("OpenProject source profile %q was not found", req.Profile),
				Severity: "error",
				Target:   Target{Kind: "source_profile", ID: req.Profile},
			})
		} else {
			selectedProfile = profile
			result.Profile = sourceProfileSummary(profile)
		}
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}

	collection, rawJSON, sourceMode, pagesLoaded, errDiag := s.loadOpenProjectCollection(ctx, req, selectedProfile)
	if errDiag != nil {
		result.Errors = append(result.Errors, *errDiag)
		return result, ErrValidationFailed
	}
	importedAt := req.Now
	if importedAt.IsZero() {
		importedAt = time.Now()
	}
	snapshotKey := importedAt.Format("20060102T150405")
	snapshotID := "source_snapshot.openproject." + snapshotKey

	result.Snapshot = OpenProjectSnapshotSummary{
		ID:             snapshotID,
		Key:            snapshotKey,
		CollectionType: collection.Type,
		DeclaredTotal:  collection.Total,
		DeclaredCount:  collection.Count,
		LoadedCount:    len(collection.Embedded.Elements),
		SourceMode:     sourceMode,
		PageSize:       collection.PageSize,
		PagesLoaded:    pagesLoaded,
	}
	resourceLinks, resourceLinksPath, resourceLinksExplicit, diag := readOpenProjectResourceLinksSidecar(s.loader.ProjectPath(), req.ResourceLinksSidecar)
	if diag != nil {
		result.Errors = append(result.Errors, *diag)
		return result, ErrValidationFailed
	}
	resourceLinkIndex := mapOpenProjectResourceLinks(ws, resourceLinks, resourceLinksPath, &result.Warnings, &result.Errors)
	if resourceLinks == nil && resourceLinksExplicit {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "openproject_resource_link_sidecar_not_found",
			Message:  fmt.Sprintf("OpenProject resource-link sidecar %s was not found", req.ResourceLinksSidecar),
			Severity: "error",
			Target:   Target{Kind: "file", ID: req.ResourceLinksSidecar},
		})
		return result, ErrValidationFailed
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	resourceCandidates := map[string]OpenProjectResourceCandidate{}
	for _, wp := range collection.Embedded.Elements {
		candidate := mapOpenProjectWorkPackage(ws, wp, &result.Warnings)
		result.TaskCandidates = append(result.TaskCandidates, candidate)
		if candidate.AssigneeName != "" {
			key := candidate.AssigneeHref
			if key == "" {
				key = candidate.AssigneeName
			}
			if _, exists := resourceCandidates[key]; !exists {
				resourceCandidates[key] = OpenProjectResourceCandidate{
					SourceID:    lastPathSegment(candidate.AssigneeHref),
					SourceHref:  candidate.AssigneeHref,
					DisplayName: candidate.AssigneeName,
				}
				resourceCandidates[key] = matchOpenProjectResourceCandidate(ws, resourceCandidates[key], resourceLinkIndex)
			}
		}
	}
	for _, candidate := range resourceCandidates {
		candidate.WouldCreate = candidate.MatchedRefID == ""
		result.ResourceCandidates = append(result.ResourceCandidates, candidate)
	}
	sort.Slice(result.ResourceCandidates, func(i, j int) bool {
		return result.ResourceCandidates[i].DisplayName < result.ResourceCandidates[j].DisplayName
	})
	sidecar, sidecarPath, sidecarExplicit, diag := readOpenProjectResourceDemandSidecar(s.loader.ProjectPath(), req.ResourceDemandsSidecar)
	if diag != nil {
		result.Errors = append(result.Errors, *diag)
		return result, ErrValidationFailed
	}
	if sidecar != nil {
		result.ResourceDemandCandidates = mapOpenProjectResourceDemandSidecar(ws, result.TaskCandidates, *sidecar, sidecarPath, &result.Warnings, &result.Errors)
	} else if sidecarExplicit {
		result.Errors = append(result.Errors, Diagnostic{
			Code:     "openproject_resource_demand_sidecar_not_found",
			Message:  fmt.Sprintf("OpenProject resource-demand sidecar %s was not found", req.ResourceDemandsSidecar),
			Severity: "error",
			Target:   Target{Kind: "file", ID: req.ResourceDemandsSidecar},
		})
		return result, ErrValidationFailed
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	if err := s.loadOpenProjectOptionalCandidates(ctx, req, selectedProfile, ws, sourceMode, collection, &result); err != nil {
		return result, err
	}
	if collection.Count != 0 && collection.Count != len(collection.Embedded.Elements) {
		result.Warnings = append(result.Warnings, Diagnostic{
			Code:     "openproject_count_mismatch",
			Message:  fmt.Sprintf("declared count %d differs from loaded element count %d", collection.Count, len(collection.Embedded.Elements)),
			Severity: "warning",
			Target:   Target{Kind: "import", ID: req.FromFile},
		})
	}
	if err := s.finishOpenProjectSnapshot(req, selectedProfile, rawJSON, importedAt, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s OpenProjectImportService) loadOpenProjectOptionalCandidates(ctx context.Context, req OpenProjectImportRequest, profile domain.SourceProfile, ws domain.Workspace, sourceMode string, collection openProjectCollection, result *OpenProjectImportResult) error {
	if req.IncludeRelations {
		relations := collection.Embedded.Relations.Elements
		if sourceMode == "api" {
			apiReq, token, diag := openProjectAPIRequestFromImportRequest(req, profile)
			if diag != nil {
				result.Errors = append(result.Errors, *diag)
				return ErrValidationFailed
			}
			relations, _, diag = fetchOpenProjectRelations(ctx, openProjectHTTPClient(req), apiReq, token, collection.Embedded.Elements)
			if diag != nil {
				result.Errors = append(result.Errors, *diag)
				return ErrValidationFailed
			}
		}
		result.DependencyCandidates = mapOpenProjectDependencyCandidates(ws, result.TaskCandidates, relations, &result.Warnings)
	}
	if !req.IncludeTimeEntries {
		return nil
	}
	timeEntries := collection.Embedded.TimeEntries.Elements
	if sourceMode == "api" {
		apiReq, token, diag := openProjectAPIRequestFromImportRequest(req, profile)
		if diag != nil {
			result.Errors = append(result.Errors, *diag)
			return ErrValidationFailed
		}
		timeEntries, _, diag = fetchOpenProjectTimeEntries(ctx, openProjectHTTPClient(req), apiReq, token, collection.Embedded.Elements)
		if diag != nil {
			result.Errors = append(result.Errors, *diag)
			return ErrValidationFailed
		}
	}
	for _, entry := range timeEntries {
		result.TimeEntryCandidates = append(result.TimeEntryCandidates, mapOpenProjectTimeEntry(entry))
	}
	return nil
}

func (s OpenProjectImportService) finishOpenProjectSnapshot(req OpenProjectImportRequest, profile domain.SourceProfile, rawJSON []byte, importedAt time.Time, result *OpenProjectImportResult) error {
	write, err := buildOpenProjectSnapshotWrite(*result, req, profile, rawJSON, importedAt)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "openproject_snapshot_render_error", Message: fmt.Sprintf("failed to render OpenProject import snapshot: %v", err), Severity: "error"})
		return ErrValidationFailed
	}
	result.PlannedWrites = openProjectSnapshotPlannedWrites(result.Snapshot.Key)
	if req.DryRun {
		return nil
	}
	writer, ok := s.loader.(OpenProjectImportSnapshotWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "snapshot_writer_unavailable", Message: "project store does not support OpenProject import snapshot writes", Severity: "error"})
		return ErrValidationFailed
	}
	writtenFiles, err := writer.WriteOpenProjectImportSnapshot(write)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "openproject_snapshot_write_error", Message: fmt.Sprintf("failed to write OpenProject import snapshot: %v", err), Severity: "error"})
		return ErrValidationFailed
	}
	result.Applied = true
	result.WrittenFiles = writtenFiles
	return nil
}

func (s OpenProjectImportService) loadOpenProjectCollection(ctx context.Context, req OpenProjectImportRequest, profile domain.SourceProfile) (openProjectCollection, []byte, string, int, *Diagnostic) {
	if strings.TrimSpace(req.FromFile) != "" {
		collection, rawJSON, err := readOpenProjectCollection(req.FromFile)
		if err != nil {
			return openProjectCollection{}, nil, "file", 0, &Diagnostic{
				Code:     "openproject_file_read_error",
				Message:  fmt.Sprintf("failed to read OpenProject import file: %v", err),
				Severity: "error",
			}
		}
		return collection, rawJSON, "file", 0, nil
	}
	apiReq, token, diag := openProjectAPIRequestFromImportRequest(req, profile)
	if diag != nil {
		return openProjectCollection{}, nil, "api", 0, diag
	}
	collection, pagesLoaded, diag := fetchOpenProjectCollection(ctx, openProjectHTTPClient(req), apiReq, token)
	if diag != nil {
		return openProjectCollection{}, nil, "api", pagesLoaded, diag
	}
	rawJSON, err := json.MarshalIndent(collection, "", "  ")
	if err != nil {
		return openProjectCollection{}, nil, "api", pagesLoaded, &Diagnostic{Code: "openproject_snapshot_render_error", Message: fmt.Sprintf("failed to render OpenProject raw snapshot: %v", err), Severity: "error"}
	}
	return collection, rawJSON, "api", pagesLoaded, nil
}

func openProjectHTTPClient(req OpenProjectImportRequest) *http.Client {
	if req.RequestRetries <= 0 {
		if req.RequestTimeoutSeconds <= 0 {
			return http.DefaultClient
		}
		return &http.Client{Timeout: time.Duration(req.RequestTimeoutSeconds * float64(time.Second))}
	}
	return &http.Client{Transport: openProjectRetryTransport{
		base:       http.DefaultTransport,
		retries:    req.RequestRetries,
		retryDelay: time.Duration(req.RequestRetryDelaySeconds * float64(time.Second)),
		timeout:    time.Duration(req.RequestTimeoutSeconds * float64(time.Second)),
	}}
}

type openProjectCancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (b *openProjectCancelBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		b.once.Do(b.cancel)
	}
	return n, err
}

func (b *openProjectCancelBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cancel)
	return err
}

var openProjectTransientHTTPStatuses = map[int]bool{
	http.StatusRequestTimeout:      true,
	http.StatusTooManyRequests:     true,
	http.StatusInternalServerError: true,
	http.StatusBadGateway:          true,
	http.StatusServiceUnavailable:  true,
	http.StatusGatewayTimeout:      true,
}

type openProjectRetryTransport struct {
	base       http.RoundTripper
	retries    int
	retryDelay time.Duration
	timeout    time.Duration
}

func (t openProjectRetryTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	attempts := 1
	if req.Method == http.MethodGet {
		attempts += t.retries
	}
	for attempt := 0; ; attempt++ {
		attemptReq := req.Clone(req.Context())
		cancel := func() {}
		if t.timeout > 0 {
			attemptCtx, attemptCancel := context.WithTimeout(req.Context(), t.timeout)
			attemptReq = req.Clone(attemptCtx)
			cancel = attemptCancel
		}
		resp, err := t.base.RoundTrip(attemptReq)
		if err != nil {
			cancel()
		} else if resp != nil {
			resp.Body = &openProjectCancelBody{ReadCloser: resp.Body, cancel: cancel}
		}
		transientStatus := resp != nil && openProjectTransientHTTPStatuses[resp.StatusCode]
		if attempt+1 >= attempts || (err == nil && !transientStatus) {
			return resp, err
		}
		if resp != nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		if t.retryDelay <= 0 {
			continue
		}
		timer := time.NewTimer(t.retryDelay)
		select {
		case <-req.Context().Done():
			timer.Stop()
			return nil, req.Context().Err()
		case <-timer.C:
		}
	}
}

func openProjectAPIRequestFromImportRequest(req OpenProjectImportRequest, profile domain.SourceProfile) (openProjectAPIRequest, string, *Diagnostic) {
	apiReq := openProjectAPIRequest{
		BaseURL:         firstNonEmpty(req.BaseURL, profile.BaseURL),
		ProjectID:       firstNonEmpty(req.ProjectID, profile.ProjectID),
		TokenEnv:        firstNonEmpty(req.TokenEnv, profile.TokenEnv),
		HostHeader:      req.HostHeader,
		ForwardedProto:  req.ForwardedProto,
		PerimeterKey:    perimeterKey(req.PerimeterKeyEnv),
		PerimeterHeader: firstNonEmpty(req.PerimeterHeader, DefaultOpenProjectPerimeterHeader),
		Since:           req.Since,
		IncludeClosed:   req.IncludeClosed,
		WorkPackageID:   req.WorkPackageID,
		PageSize:        req.PageSize,
	}
	if apiReq.BaseURL == "" {
		return openProjectAPIRequest{}, "", &Diagnostic{Code: "openproject_base_url_required", Message: "OpenProject API import requires base_url from --base-url or source profile", Severity: "error"}
	}
	if apiReq.ProjectID == "" && apiReq.WorkPackageID == "" {
		return openProjectAPIRequest{}, "", &Diagnostic{Code: "openproject_project_id_required", Message: "OpenProject API import requires project_id from --project-id or source profile", Severity: "error"}
	}
	if apiReq.TokenEnv == "" {
		if profile.CredentialProfileRef != "" {
			return openProjectAPIRequest{}, "", &Diagnostic{Code: "credential_profile_not_implemented", Message: "external credential profiles are not implemented yet; use token_env", Severity: "error", Target: Target{Kind: "source_profile", ID: profile.ID}}
		}
		return openProjectAPIRequest{}, "", &Diagnostic{Code: "openproject_token_env_required", Message: "OpenProject API import requires token_env from --token-env or source profile", Severity: "error"}
	}
	token := os.Getenv(apiReq.TokenEnv)
	if token == "" {
		return openProjectAPIRequest{}, "", &Diagnostic{Code: "openproject_token_missing", Message: fmt.Sprintf("environment variable %s is not set", apiReq.TokenEnv), Severity: "error", Target: Target{Kind: "env", ID: apiReq.TokenEnv}}
	}
	return apiReq, token, nil
}

func readOpenProjectCollection(path string) (openProjectCollection, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return openProjectCollection{}, nil, err
	}
	var collection openProjectCollection
	if err := json.Unmarshal(data, &collection); err != nil {
		return openProjectCollection{}, nil, err
	}
	return collection, data, nil
}

func readOpenProjectResourceDemandSidecar(projectPath, requestedPath string) (*openProjectResourceDemandSidecar, string, bool, *Diagnostic) {
	return readOpenProjectSidecar[openProjectResourceDemandSidecar](projectPath, requestedPath, "integrations/openproject/resource-demands.yaml", "resource-demand")
}

func readOpenProjectResourceLinksSidecar(projectPath, requestedPath string) (*openProjectResourceLinksSidecar, string, bool, *Diagnostic) {
	return readOpenProjectSidecar[openProjectResourceLinksSidecar](projectPath, requestedPath, "integrations/openproject/resource-links.yaml", "resource-link")
}

func readOpenProjectSidecar[T any](projectPath, requestedPath, defaultPath, label string) (*T, string, bool, *Diagnostic) {
	explicit := strings.TrimSpace(requestedPath) != ""
	sidecarPath := strings.TrimSpace(requestedPath)
	if sidecarPath == "" {
		if strings.TrimSpace(projectPath) == "" {
			return nil, "", false, nil
		}
		sidecarPath = filepath.Join(projectPath, defaultPath)
	} else if !filepath.IsAbs(sidecarPath) && strings.TrimSpace(projectPath) != "" {
		sidecarPath = filepath.Join(projectPath, sidecarPath)
	}
	data, err := os.ReadFile(sidecarPath)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return nil, sidecarPath, false, nil
		}
		code := "openproject_" + strings.ReplaceAll(label, "-", "_") + "_sidecar_read_error"
		if os.IsNotExist(err) {
			code = "openproject_" + strings.ReplaceAll(label, "-", "_") + "_sidecar_not_found"
		}
		return nil, sidecarPath, explicit, &Diagnostic{
			Code:     code,
			Message:  fmt.Sprintf("failed to read OpenProject %s sidecar %s: %v", label, sidecarPath, err),
			Severity: "error",
			Target:   Target{Kind: "file", ID: sidecarPath},
		}
	}
	var sidecar T
	if err := yaml.Unmarshal(data, &sidecar); err != nil {
		return nil, sidecarPath, explicit, &Diagnostic{
			Code:     "openproject_" + strings.ReplaceAll(label, "-", "_") + "_sidecar_parse_error",
			Message:  fmt.Sprintf("failed to parse OpenProject %s sidecar %s: %v", label, sidecarPath, err),
			Severity: "error",
			Target:   Target{Kind: "file", ID: sidecarPath},
		}
	}
	return &sidecar, sidecarPath, explicit, nil
}

func buildOpenProjectSnapshotWrite(result OpenProjectImportResult, req OpenProjectImportRequest, profile domain.SourceProfile, rawJSON []byte, importedAt time.Time) (OpenProjectImportSnapshotWrite, error) {
	key := result.Snapshot.Key
	rawPath := "imports/openproject/" + key + "/raw.json"
	mappedPath := "imports/openproject/" + key + "/mapped.json"
	manifest := OpenProjectImportManifest{SourceSnapshot: OpenProjectImportManifestBody{
		ID:                   result.Snapshot.ID,
		Source:               "openproject",
		SourceMode:           result.Snapshot.SourceMode,
		ImportedAt:           importedAt.Format(time.RFC3339),
		ProfileID:            profile.ID,
		BaseURL:              firstNonEmpty(req.BaseURL, profile.BaseURL),
		ProjectID:            firstNonEmpty(req.ProjectID, profile.ProjectID),
		TokenEnv:             firstNonEmpty(req.TokenEnv, profile.TokenEnv),
		CredentialProfileRef: profile.CredentialProfileRef,
		Query: OpenProjectImportManifestQuery{
			Since:                  req.Since,
			WorkPackageID:          req.WorkPackageID,
			PageSize:               result.Snapshot.PageSize,
			IncludeClosed:          req.IncludeClosed,
			IncludeTimeEntries:     req.IncludeTimeEntries,
			IncludeRelations:       req.IncludeRelations,
			ResourceLinksSidecar:   req.ResourceLinksSidecar,
			ResourceDemandsSidecar: req.ResourceDemandsSidecar,
		},
		RawLocation:    rawPath,
		MappedLocation: mappedPath,
		AdapterVersion: "replan-openproject-adapter-v0",
		ObjectCount:    result.Snapshot.LoadedCount,
		WarningCount:   len(result.Warnings),
	}}
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		return OpenProjectImportSnapshotWrite{}, err
	}
	mapped := OpenProjectMappedSnapshot{
		Snapshot:                 result.Snapshot,
		Profile:                  result.Profile,
		TaskCandidates:           result.TaskCandidates,
		ResourceCandidates:       result.ResourceCandidates,
		ResourceDemandCandidates: result.ResourceDemandCandidates,
		DependencyCandidates:     result.DependencyCandidates,
		TimeEntryCandidates:      result.TimeEntryCandidates,
	}
	mappedJSON, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		return OpenProjectImportSnapshotWrite{}, err
	}
	return OpenProjectImportSnapshotWrite{
		SnapshotKey:  key,
		ManifestYAML: manifestYAML,
		RawJSON:      rawJSON,
		MappedJSON:   mappedJSON,
	}, nil
}

func openProjectSnapshotPlannedWrites(snapshotKey string) []PlannedWrite {
	base := "imports/openproject/" + snapshotKey + "/"
	return []PlannedWrite{
		{Path: base + "manifest.yaml", Action: "write", Format: "yaml"},
		{Path: base + "raw.json", Action: "write", Format: "json"},
		{Path: base + "mapped.json", Action: "write", Format: "json"},
	}
}

type openProjectAPIRequest struct {
	BaseURL         string
	ProjectID       string
	TokenEnv        string
	HostHeader      string
	ForwardedProto  string
	PerimeterKey    string
	PerimeterHeader string
	Since           string
	IncludeClosed   bool
	WorkPackageID   string
	PageSize        int
}

// Defaults for the reverse-proxy perimeter key; the scripts read the same
// environment variables.
const (
	DefaultOpenProjectPerimeterKeyEnv = "REPLAN_OPENPROJECT_PERIMETER_KEY"
	DefaultOpenProjectPerimeterHeader = "X-Perimeter-Key"
)

func perimeterKey(envName string) string {
	if envName == "" {
		return ""
	}
	return os.Getenv(envName)
}

func applyOpenProjectHeaders(httpReq *http.Request, req openProjectAPIRequest) {
	if req.HostHeader != "" {
		httpReq.Host = req.HostHeader
		httpReq.Header.Set("Host", req.HostHeader)
	}
	if req.ForwardedProto != "" {
		httpReq.Header.Set("X-Forwarded-Proto", req.ForwardedProto)
	}
	if req.PerimeterKey != "" && req.PerimeterHeader != "" {
		httpReq.Header.Set(req.PerimeterHeader, req.PerimeterKey)
	}
}

func fetchOpenProjectCollection(ctx context.Context, client *http.Client, req openProjectAPIRequest, token string) (openProjectCollection, int, *Diagnostic) {
	if client == nil {
		client = http.DefaultClient
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	if req.WorkPackageID != "" {
		wp, diag := fetchOpenProjectWorkPackage(ctx, client, req, token)
		if diag != nil {
			return openProjectCollection{}, 0, diag
		}
		collection := openProjectCollection{Type: "Collection", Total: 1, Count: 1, PageSize: 1, Offset: 1}
		collection.Embedded.Elements = []openProjectWorkPackage{wp}
		return collection, 1, nil
	}
	pageReq := req
	if req.IncludeClosed {
		project, diag := fetchOpenProjectProject(ctx, client, req, token)
		if diag != nil {
			return openProjectCollection{}, 0, diag
		}
		pageReq.ProjectID = strconv.Itoa(project.ID)
	}
	var out openProjectCollection
	loaded := 0
	for offset := 1; ; offset++ {
		pageURL, err := openProjectPageURL(pageReq, offset, pageSize)
		if err != nil {
			return openProjectCollection{}, 0, &Diagnostic{Code: "openproject_url_error", Message: fmt.Sprintf("failed to build OpenProject URL: %v", err), Severity: "error"}
		}
		page, diag := fetchOpenProjectCollectionPage(ctx, client, pageReq, pageURL, token)
		if diag != nil {
			return openProjectCollection{}, loaded, diag
		}
		if offset == 1 {
			out = page
			out.Embedded.Elements = nil
			out.PageSize = pageSize
		}
		out.Embedded.Elements = append(out.Embedded.Elements, page.Embedded.Elements...)
		loaded++
		if page.Count == 0 || len(out.Embedded.Elements) >= page.Total {
			break
		}
	}
	out.Count = len(out.Embedded.Elements)
	return out, loaded, nil
}

func fetchOpenProjectProject(ctx context.Context, client *http.Client, req openProjectAPIRequest, token string) (openProjectProject, *Diagnostic) {
	base, err := url.Parse(strings.TrimRight(req.BaseURL, "/"))
	if err != nil {
		return openProjectProject{}, &Diagnostic{Code: "openproject_url_error", Message: fmt.Sprintf("failed to parse OpenProject base_url: %v", err), Severity: "error"}
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v3/projects/" + url.PathEscape(req.ProjectID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return openProjectProject{}, &Diagnostic{Code: "openproject_request_error", Message: fmt.Sprintf("failed to build OpenProject project request: %v", err), Severity: "error"}
	}
	httpReq.SetBasicAuth("apikey", token)
	httpReq.Header.Set("Accept", "application/json")
	applyOpenProjectHeaders(httpReq, req)
	resp, err := client.Do(httpReq)
	if err != nil {
		return openProjectProject{}, &Diagnostic{Code: "openproject_network_error", Message: fmt.Sprintf("OpenProject project request failed: %v", err), Severity: "error"}
	}
	defer resp.Body.Close()
	if diag := openProjectHTTPStatusDiagnostic(resp); diag != nil {
		return openProjectProject{}, diag
	}
	var project openProjectProject
	if err := json.NewDecoder(resp.Body).Decode(&project); err != nil {
		return openProjectProject{}, &Diagnostic{Code: "openproject_malformed_response", Message: fmt.Sprintf("failed to parse OpenProject project response: %v", err), Severity: "error"}
	}
	if project.ID == 0 {
		return openProjectProject{}, &Diagnostic{Code: "openproject_project_id_missing", Message: "OpenProject project response did not contain a numeric id", Severity: "error"}
	}
	return project, nil
}

func fetchOpenProjectWorkPackage(ctx context.Context, client *http.Client, req openProjectAPIRequest, token string) (openProjectWorkPackage, *Diagnostic) {
	base, err := url.Parse(strings.TrimRight(req.BaseURL, "/"))
	if err != nil {
		return openProjectWorkPackage{}, &Diagnostic{Code: "openproject_url_error", Message: fmt.Sprintf("failed to parse OpenProject base_url: %v", err), Severity: "error"}
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v3/work_packages/" + url.PathEscape(req.WorkPackageID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return openProjectWorkPackage{}, &Diagnostic{Code: "openproject_request_error", Message: fmt.Sprintf("failed to build OpenProject request: %v", err), Severity: "error"}
	}
	httpReq.SetBasicAuth("apikey", token)
	httpReq.Header.Set("Accept", "application/json")
	applyOpenProjectHeaders(httpReq, req)
	resp, err := client.Do(httpReq)
	if err != nil {
		return openProjectWorkPackage{}, &Diagnostic{Code: "openproject_network_error", Message: fmt.Sprintf("OpenProject request failed: %v", err), Severity: "error"}
	}
	defer resp.Body.Close()
	if diag := openProjectHTTPStatusDiagnostic(resp); diag != nil {
		return openProjectWorkPackage{}, diag
	}
	var wp openProjectWorkPackage
	if err := json.NewDecoder(resp.Body).Decode(&wp); err != nil {
		return openProjectWorkPackage{}, &Diagnostic{Code: "openproject_malformed_response", Message: fmt.Sprintf("failed to parse OpenProject work package response: %v", err), Severity: "error"}
	}
	return wp, nil
}

func fetchOpenProjectCollectionPage(ctx context.Context, client *http.Client, req openProjectAPIRequest, pageURL string, token string) (openProjectCollection, *Diagnostic) {
	return fetchOpenProjectPage[openProjectCollection](ctx, client, req, pageURL, token, "", "collection")
}

func fetchOpenProjectTimeEntries(ctx context.Context, client *http.Client, req openProjectAPIRequest, token string, workPackages []openProjectWorkPackage) ([]openProjectTimeEntry, int, *Diagnostic) {
	if client == nil {
		client = http.DefaultClient
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	workPackageHrefs := map[string]bool{}
	for _, wp := range workPackages {
		if wp.Links.Self.Href != "" {
			workPackageHrefs[wp.Links.Self.Href] = true
		}
	}
	var out []openProjectTimeEntry
	pagesLoaded := 0
	for offset := 1; ; offset++ {
		pageURL, err := openProjectTimeEntriesURL(req, offset, pageSize)
		if err != nil {
			return nil, pagesLoaded, &Diagnostic{Code: "openproject_url_error", Message: fmt.Sprintf("failed to build OpenProject time entries URL: %v", err), Severity: "error"}
		}
		page, diag := fetchOpenProjectTimeEntryPage(ctx, client, req, pageURL, token)
		if diag != nil {
			return nil, pagesLoaded, diag
		}
		for _, entry := range page.Embedded.Elements {
			if len(workPackageHrefs) == 0 || workPackageHrefs[entry.Links.Entity.Href] {
				out = append(out, entry)
			}
		}
		pagesLoaded++
		if page.Count == 0 || offset*pageSize >= page.Total {
			break
		}
	}
	return out, pagesLoaded, nil
}

func fetchOpenProjectTimeEntryPage(ctx context.Context, client *http.Client, req openProjectAPIRequest, pageURL string, token string) (openProjectTimeEntryCollection, *Diagnostic) {
	return fetchOpenProjectPage[openProjectTimeEntryCollection](ctx, client, req, pageURL, token, "time entries ", "time entries")
}

func fetchOpenProjectPage[T any](ctx context.Context, client *http.Client, req openProjectAPIRequest, pageURL, token, requestLabel, responseLabel string) (T, *Diagnostic) {
	var zero T
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL, nil)
	if err != nil {
		return zero, &Diagnostic{Code: "openproject_request_error", Message: fmt.Sprintf("failed to build OpenProject %srequest: %v", requestLabel, err), Severity: "error"}
	}
	httpReq.SetBasicAuth("apikey", token)
	httpReq.Header.Set("Accept", "application/json")
	applyOpenProjectHeaders(httpReq, req)
	resp, err := client.Do(httpReq)
	if err != nil {
		return zero, &Diagnostic{Code: "openproject_network_error", Message: fmt.Sprintf("OpenProject %srequest failed: %v", requestLabel, err), Severity: "error"}
	}
	defer resp.Body.Close()
	if diag := openProjectHTTPStatusDiagnostic(resp); diag != nil {
		return zero, diag
	}
	var collection T
	if err := json.NewDecoder(resp.Body).Decode(&collection); err != nil {
		return zero, &Diagnostic{Code: "openproject_malformed_response", Message: fmt.Sprintf("failed to parse OpenProject %s response: %v", responseLabel, err), Severity: "error"}
	}
	return collection, nil
}

func fetchOpenProjectRelations(ctx context.Context, client *http.Client, req openProjectAPIRequest, token string, workPackages []openProjectWorkPackage) ([]openProjectRelation, int, *Diagnostic) {
	if client == nil {
		client = http.DefaultClient
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 100
	}
	workPackageIDs := map[string]bool{}
	for _, wp := range workPackages {
		workPackageIDs[strconv.Itoa(wp.ID)] = true
	}
	var out []openProjectRelation
	pagesLoaded := 0
	for offset := 1; ; offset++ {
		pageURL, err := openProjectRelationsURL(req, offset, pageSize)
		if err != nil {
			return nil, pagesLoaded, &Diagnostic{Code: "openproject_url_error", Message: fmt.Sprintf("failed to build OpenProject relations URL: %v", err), Severity: "error"}
		}
		page, diag := fetchOpenProjectRelationPage(ctx, client, req, pageURL, token)
		if diag != nil {
			return nil, pagesLoaded, diag
		}
		for _, relation := range page.Embedded.Elements {
			fromID := lastPathSegment(relation.Links.From.Href)
			toID := lastPathSegment(relation.Links.To.Href)
			if len(workPackageIDs) == 0 || workPackageIDs[fromID] || workPackageIDs[toID] {
				out = append(out, relation)
			}
		}
		pagesLoaded++
		if page.Count == 0 || offset*pageSize >= page.Total {
			break
		}
	}
	return out, pagesLoaded, nil
}

func fetchOpenProjectRelationPage(ctx context.Context, client *http.Client, req openProjectAPIRequest, pageURL string, token string) (openProjectRelationCollection, *Diagnostic) {
	return fetchOpenProjectPage[openProjectRelationCollection](ctx, client, req, pageURL, token, "relations ", "relations")
}

func openProjectPageURL(req openProjectAPIRequest, offset, pageSize int) (string, error) {
	base, err := url.Parse(strings.TrimRight(req.BaseURL, "/"))
	if err != nil {
		return "", err
	}
	if req.IncludeClosed {
		base.Path = strings.TrimRight(base.Path, "/") + "/api/v3/work_packages"
	} else {
		base.Path = strings.TrimRight(base.Path, "/") + "/api/v3/projects/" + url.PathEscape(req.ProjectID) + "/work_packages"
	}
	query := base.Query()
	query.Set("offset", strconv.Itoa(offset))
	query.Set("pageSize", strconv.Itoa(pageSize))
	if req.IncludeClosed {
		query.Set("filters", openProjectWorkPackageFilters(req.ProjectID, req.Since))
	} else if req.Since != "" {
		query.Set("filters", openProjectUpdatedSinceFilter(req.Since))
	}
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func openProjectRelationsURL(req openProjectAPIRequest, offset, pageSize int) (string, error) {
	base, err := url.Parse(strings.TrimRight(req.BaseURL, "/"))
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v3/relations"
	query := base.Query()
	query.Set("offset", strconv.Itoa(offset))
	query.Set("pageSize", strconv.Itoa(pageSize))
	query.Set("filters", "[]")
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func openProjectTimeEntriesURL(req openProjectAPIRequest, offset, pageSize int) (string, error) {
	base, err := url.Parse(strings.TrimRight(req.BaseURL, "/"))
	if err != nil {
		return "", err
	}
	base.Path = strings.TrimRight(base.Path, "/") + "/api/v3/time_entries"
	query := base.Query()
	query.Set("offset", strconv.Itoa(offset))
	query.Set("pageSize", strconv.Itoa(pageSize))
	base.RawQuery = query.Encode()
	return base.String(), nil
}

func openProjectUpdatedSinceFilter(since string) string {
	value := since
	if len(since) == len("2006-01-02") {
		if parsed, err := time.Parse("2006-01-02", since); err == nil {
			value = parsed.Format("2006-01-02")
		}
	} else if parsed, err := time.Parse(time.RFC3339, since); err == nil {
		value = parsed.Format("2006-01-02")
	}
	filter := []map[string]map[string]any{{
		"updated_at": {
			"operator": "<>d",
			"values":   []string{value, "2099-12-31"},
		},
	}}
	data, err := json.Marshal(filter)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func openProjectWorkPackageFilters(projectID, since string) string {
	filter := []map[string]map[string]any{}
	if strings.TrimSpace(projectID) != "" {
		filter = append(filter, map[string]map[string]any{
			"project": {
				"operator": "=",
				"values":   []string{projectID},
			},
		})
	}
	if strings.TrimSpace(since) != "" {
		var sinceFilter []map[string]map[string]any
		if err := json.Unmarshal([]byte(openProjectUpdatedSinceFilter(since)), &sinceFilter); err == nil {
			filter = append(filter, sinceFilter...)
		}
	}
	data, err := json.Marshal(filter)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func openProjectHTTPStatusDiagnostic(resp *http.Response) *Diagnostic {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	message := strings.TrimSpace(string(body))
	var apiErr openProjectAPIError
	if json.Unmarshal(body, &apiErr) == nil && apiErr.Message != "" {
		message = apiErr.Message
	}
	if message == "" {
		message = resp.Status
	}
	code := "openproject_api_error"
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		code = "openproject_authentication_failed"
	case http.StatusForbidden:
		code = "openproject_permission_denied"
	case http.StatusNotFound:
		code = "openproject_project_not_found"
	case http.StatusTooManyRequests:
		code = "openproject_rate_limited"
	}
	return &Diagnostic{
		Code:     code,
		Message:  fmt.Sprintf("OpenProject API returned HTTP %d: %s", resp.StatusCode, message),
		Severity: "error",
	}
}

func findSourceProfile(profiles []domain.SourceProfile, id string, source string) (domain.SourceProfile, bool) {
	for _, profile := range profiles {
		if profile.ID == id && profile.Source == source {
			return profile, true
		}
	}
	return domain.SourceProfile{}, false
}

func sourceProfileSummary(profile domain.SourceProfile) *SourceProfileSummary {
	defaultQuery := map[string]string{}
	for key, value := range profile.DefaultQuery {
		defaultQuery[key] = value
	}
	if len(defaultQuery) == 0 {
		defaultQuery = nil
	}
	return &SourceProfileSummary{
		ID:                   profile.ID,
		Source:               profile.Source,
		BaseURL:              profile.BaseURL,
		ProjectID:            profile.ProjectID,
		TokenEnv:             profile.TokenEnv,
		CredentialProfileRef: profile.CredentialProfileRef,
		DefaultQuery:         defaultQuery,
		Repository:           profile.Repository,
		StatusMap:            profile.StatusMap,
		StartField:           profile.StartField,
		FinishField:          profile.FinishField,
		EstimateField:        profile.EstimateField,
	}
}

func mapOpenProjectWorkPackage(ws domain.Workspace, wp openProjectWorkPackage, warnings *[]Diagnostic) OpenProjectTaskCandidate {
	sourceID := strconv.Itoa(wp.ID)
	sourceURL := wp.Links.Self.Href
	status := firstNonEmpty(wp.Status.Name, wp.Links.Status.Title)
	mappedStatus := mapOpenProjectStatus(status)
	candidate := OpenProjectTaskCandidate{
		SourceID:       sourceID,
		SourceURL:      sourceURL,
		Title:          wp.Subject,
		Description:    wp.Description.Raw,
		SourceStatus:   status,
		MappedStatus:   mappedStatus,
		AssigneeName:   firstNonEmpty(wp.Assignee.Name, wp.Links.Assignee.Title),
		AssigneeHref:   wp.Links.Assignee.Href,
		EstimatedHours: parseOpenProjectDurationHours(wp.EstimatedTime),
		SpentHours:     parseOpenProjectDurationHours(wp.SpentTime),
		StartDate:      wp.StartDate,
		DueDate:        wp.DueDate,
		SourceRefs: []domain.SourceRef{{
			Source:     "openproject",
			ObjectType: "work_package",
			ObjectID:   sourceID,
			URL:        sourceURL,
		}},
	}
	if candidate.Description == "" && wp.Description.HTML != "" {
		candidate.Description = wp.Description.HTML
	}
	if matchedID, reason := matchOpenProjectTask(ws.Tasks, sourceID, wp.Subject); matchedID != "" {
		candidate.MatchedTaskID = matchedID
		candidate.MatchReason = reason
	} else {
		candidate.WouldCreate = true
		*warnings = append(*warnings, Diagnostic{
			Code:     "unmatched_work_package",
			Message:  fmt.Sprintf("OpenProject work package %s has no matching RePlan task", sourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_work_package", ID: sourceID},
		})
	}
	if mappedStatus == "" && status != "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "unmapped_openproject_status",
			Message:  fmt.Sprintf("OpenProject status %q is not mapped to a RePlan tracking status", status),
			Severity: "warning",
			Target:   Target{Kind: "openproject_work_package", ID: sourceID},
		})
	}
	if candidate.AssigneeName == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "missing_openproject_assignee",
			Message:  fmt.Sprintf("OpenProject work package %s has no assignee", sourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_work_package", ID: sourceID},
		})
	}
	if wp.EstimatedTime == "" {
		*warnings = append(*warnings, Diagnostic{
			Code:     "missing_openproject_estimate",
			Message:  fmt.Sprintf("OpenProject work package %s has no estimate", sourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_work_package", ID: sourceID},
		})
	}
	return candidate
}

func mapOpenProjectTimeEntry(entry openProjectTimeEntry) OpenProjectTimeEntryCandidate {
	sourceID := strconv.Itoa(entry.ID)
	sourceURL := entry.Links.Self.Href
	workPackageHref := entry.Links.Entity.Href
	return OpenProjectTimeEntryCandidate{
		SourceID:           sourceID,
		SourceURL:          sourceURL,
		WorkPackageID:      lastPathSegment(workPackageHref),
		WorkPackageHref:    workPackageHref,
		UserName:           entry.Links.User.Title,
		UserHref:           entry.Links.User.Href,
		ActivityName:       entry.Links.Activity.Title,
		ActivityHref:       entry.Links.Activity.Href,
		Hours:              parseOpenProjectDurationHours(entry.Hours),
		SpentOn:            entry.SpentOn,
		Comment:            firstNonEmpty(entry.Comment.Raw, entry.Comment.HTML),
		WouldCreateActual:  false,
		ActualAcceptStatus: "proposal_required",
		SourceRefs: []domain.SourceRef{{
			Source:     "openproject",
			ObjectType: "time_entry",
			ObjectID:   sourceID,
			URL:        sourceURL,
		}},
	}
}

func mapOpenProjectDependencyCandidates(ws domain.Workspace, taskCandidates []OpenProjectTaskCandidate, relations []openProjectRelation, warnings *[]Diagnostic) []OpenProjectDependencyCandidate {
	var out []OpenProjectDependencyCandidate
	for _, relation := range relations {
		candidate := mapOpenProjectDependencyCandidate(ws, taskCandidates, relation)
		*warnings = append(*warnings, candidate.Diagnostics...)
		if candidate.AcceptStatus == "unsupported_relation_type" {
			continue
		}
		out = append(out, candidate)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].SourceID < out[j].SourceID
	})
	return out
}

func mapOpenProjectDependencyCandidate(ws domain.Workspace, taskCandidates []OpenProjectTaskCandidate, relation openProjectRelation) OpenProjectDependencyCandidate {
	sourceID := strconv.Itoa(relation.ID)
	fromID := lastPathSegment(relation.Links.From.Href)
	toID := lastPathSegment(relation.Links.To.Href)
	predecessorWP, successorWP, dependencyType, ok := mapOpenProjectRelationType(relation.Type, fromID, toID)
	candidate := OpenProjectDependencyCandidate{
		SourceID:          sourceID,
		SourceURL:         relation.Links.Self.Href,
		RelationType:      relation.Type,
		FromWorkPackageID: fromID,
		ToWorkPackageID:   toID,
		DependencyID:      "dependency.openproject.relation." + sourceID,
		DependencyType:    dependencyType,
		WouldCreate:       true,
		AcceptStatus:      "safe_to_accept",
		Diagnostics:       []Diagnostic{},
	}
	if !ok {
		candidate.AcceptStatus = "unsupported_relation_type"
		candidate.WouldCreate = false
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_relation_type_unsupported",
			Message:  fmt.Sprintf("OpenProject relation %s has unsupported type %q", sourceID, relation.Type),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: sourceID},
		})
		return candidate
	}
	candidate.PredecessorTaskID = matchTaskForOpenProjectResourceDemand(ws, taskCandidates, predecessorWP)
	candidate.SuccessorTaskID = matchTaskForOpenProjectResourceDemand(ws, taskCandidates, successorWP)
	if candidate.PredecessorTaskID == "" || candidate.SuccessorTaskID == "" {
		candidate.AcceptStatus = "task_unmatched"
		candidate.WouldCreate = false
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_relation_task_unmatched",
			Message:  fmt.Sprintf("OpenProject relation %s cannot be accepted because one or both work packages are not matched to RePlan tasks", sourceID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_relation", ID: sourceID},
		})
	}
	return candidate
}

func mapOpenProjectRelationType(relationType, fromID, toID string) (string, string, string, bool) {
	switch strings.TrimSpace(relationType) {
	case "precedes":
		return fromID, toID, "finish_to_start", true
	case "follows":
		return toID, fromID, "finish_to_start", true
	case "blocks":
		return fromID, toID, "blocks", true
	case "blocked":
		return toID, fromID, "blocks", true
	default:
		return "", "", "", false
	}
}

func mapOpenProjectResourceDemandSidecar(ws domain.Workspace, taskCandidates []OpenProjectTaskCandidate, sidecar openProjectResourceDemandSidecar, sidecarPath string, warnings *[]Diagnostic, errors *[]Diagnostic) []OpenProjectResourceDemandCandidate {
	var out []OpenProjectResourceDemandCandidate
	seenIDs := map[string]bool{}
	seenTuples := map[string]bool{}
	if len(sidecar.OpenProjectResourceDemands) == 0 {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_demand_empty",
			Message:  fmt.Sprintf("OpenProject resource-demand sidecar %s contains no openproject_resource_demands", sidecarPath),
			Severity: "warning",
			Target:   Target{Kind: "file", ID: sidecarPath},
		})
		return out
	}
	for _, entry := range sidecar.OpenProjectResourceDemands {
		workPackageID := strings.TrimSpace(entry.WorkPackageID)
		if workPackageID == "" {
			*errors = append(*errors, Diagnostic{
				Code:     "openproject_resource_demand_work_package_missing",
				Message:  "OpenProject resource-demand sidecar entry is missing work_package_id",
				Severity: "error",
				Target:   Target{Kind: "file", ID: sidecarPath},
			})
			continue
		}
		if len(entry.Demands) == 0 {
			*warnings = append(*warnings, Diagnostic{
				Code:     "openproject_resource_demand_empty",
				Message:  fmt.Sprintf("OpenProject work package %s has no sidecar demands", workPackageID),
				Severity: "warning",
				Target:   Target{Kind: "openproject_work_package", ID: workPackageID},
			})
			continue
		}
		taskID := matchTaskForOpenProjectResourceDemand(ws, taskCandidates, workPackageID)
		workPackageKnown := openProjectWorkPackageKnown(taskCandidates, workPackageID)
		if !workPackageKnown {
			*warnings = append(*warnings, Diagnostic{
				Code:     "openproject_resource_demand_work_package_unknown",
				Message:  fmt.Sprintf("OpenProject resource-demand sidecar references work package %s that is not present in the mapped snapshot", workPackageID),
				Severity: "warning",
				Target:   Target{Kind: "openproject_work_package", ID: workPackageID},
			})
		}
		for _, demand := range entry.Demands {
			candidate := mapOpenProjectSidecarDemand(ws, demand, workPackageID, taskID, sidecarPath, seenIDs, seenTuples, warnings, errors)
			out = append(out, candidate)
		}
	}
	return out
}

func mapOpenProjectSidecarDemand(ws domain.Workspace, demand openProjectSidecarDemand, workPackageID, taskID, sidecarPath string, seenIDs, seenTuples map[string]bool, warnings *[]Diagnostic, errors *[]Diagnostic) OpenProjectResourceDemandCandidate {
	candidate := OpenProjectResourceDemandCandidate{
		Source:        "openproject_sidecar",
		SourceID:      strings.TrimSpace(demand.ID),
		WorkPackageID: workPackageID,
		MatchedTaskID: taskID,
		WouldCreate:   true,
		AcceptStatus:  "safe_to_accept",
		Diagnostics:   []Diagnostic{},
	}
	if candidate.SourceID == "" {
		candidate.SourceID = "resource_demand.op-" + strings.ToLower(strings.TrimSpace(workPackageID)) + ".missing-id"
	}
	// An explicit remaining amount, including 0, is kept as given. Only a
	// missing one defaults to the planned amount (GAP-016).
	remainingAmount := demand.RemainingAmount
	if remainingAmount == nil && demand.PlannedAmount != 0 {
		remainingAmount = domain.Amount(demand.PlannedAmount)
	}
	candidate.Canonical = domain.ResourceDemand{
		ID:              candidate.SourceID,
		TaskRef:         domain.Ref{Kind: "task", ID: taskID},
		ResourceRef:     demand.ResourceRef,
		ResourcePoolRef: demand.ResourcePoolRef,
		ResourceClass:   strings.TrimSpace(demand.ResourceClass),
		Role:            strings.TrimSpace(demand.Role),
		Unit:            strings.TrimSpace(demand.Unit),
		PlannedAmount:   demand.PlannedAmount,
		RemainingAmount: remainingAmount,
		Criticality:     strings.TrimSpace(demand.Criticality),
		Status:          strings.TrimSpace(demand.Status),
	}
	if taskID == "" {
		candidate.AcceptStatus = "task_unmatched"
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_task_unmatched",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s cannot be accepted because work package %s is not matched to a RePlan task", candidate.SourceID, workPackageID),
			Severity: "warning",
			Target:   Target{Kind: "openproject_work_package", ID: workPackageID},
		})
	}
	if existingDemandID(ws.ResourceDemands, candidate.SourceID) {
		candidate.WouldCreate = false
		candidate.AcceptStatus = "already_exists"
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_already_exists",
			Message:  fmt.Sprintf("resource demand %s already exists", candidate.SourceID),
			Severity: "warning",
			Target:   Target{Kind: "resource_demand", ID: candidate.SourceID},
		})
	}
	appendSidecarDemandDiagnostics(ws, &candidate, sidecarPath, seenIDs, seenTuples)
	for _, diag := range candidate.Diagnostics {
		if diag.Severity == "error" {
			candidate.AcceptStatus = "invalid_sidecar"
			break
		}
	}
	for _, diag := range candidate.Diagnostics {
		switch diag.Severity {
		case "warning":
			*warnings = append(*warnings, diag)
		case "error":
			*errors = append(*errors, diag)
		}
	}
	return candidate
}

func appendSidecarDemandDiagnostics(ws domain.Workspace, candidate *OpenProjectResourceDemandCandidate, sidecarPath string, seenIDs, seenTuples map[string]bool) {
	if candidate.SourceID == "" || strings.HasSuffix(candidate.SourceID, ".missing-id") {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_id_missing",
			Message:  "OpenProject sidecar demand is missing id",
			Severity: "error",
			Target:   Target{Kind: "file", ID: sidecarPath},
		})
	}
	if seenIDs[candidate.SourceID] {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_duplicate_id",
			Message:  fmt.Sprintf("OpenProject sidecar demand id %s is duplicated", candidate.SourceID),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: candidate.SourceID},
		})
	}
	seenIDs[candidate.SourceID] = true
	hasResource := strings.TrimSpace(candidate.Canonical.ResourceRef.ID) != ""
	hasPool := strings.TrimSpace(candidate.Canonical.ResourcePoolRef.ID) != ""
	if !hasResource && !hasPool {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_resource_missing",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s has no resource_ref or resource_pool_ref", candidate.SourceID),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: candidate.SourceID},
		})
	} else if hasResource && !resourceExists(ws.Resources, candidate.Canonical.ResourceRef.ID) {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_resource_unknown",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s references unknown resource %s", candidate.SourceID, candidate.Canonical.ResourceRef.ID),
			Severity: "error",
			Target:   Target{Kind: "resource", ID: candidate.Canonical.ResourceRef.ID},
		})
	} else if hasPool && !resourcePoolExists(ws.ResourcePools, candidate.Canonical.ResourcePoolRef.ID) {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_resource_unknown",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s references unknown resource pool %s", candidate.SourceID, candidate.Canonical.ResourcePoolRef.ID),
			Severity: "error",
			Target:   Target{Kind: "resource_pool", ID: candidate.Canonical.ResourcePoolRef.ID},
		})
	}
	if candidate.Canonical.PlannedAmount == 0 {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_amount_missing",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s has no planned_amount", candidate.SourceID),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: candidate.SourceID},
		})
	} else if candidate.Canonical.PlannedAmount < 0 {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_amount_invalid",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s has invalid planned_amount %v", candidate.SourceID, candidate.Canonical.PlannedAmount),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: candidate.SourceID},
		})
	}
	if !allowedOpenProjectResourceDemandUnit(candidate.Canonical.Unit) {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_unit_unknown",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s has unsupported unit %q", candidate.SourceID, candidate.Canonical.Unit),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: candidate.SourceID},
		})
	}
	if strings.TrimSpace(candidate.Canonical.ResourceClass) == "" {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{
			Code:     "openproject_resource_demand_resource_class_missing",
			Message:  fmt.Sprintf("OpenProject sidecar demand %s has no resource_class", candidate.SourceID),
			Severity: "error",
			Target:   Target{Kind: "resource_demand", ID: candidate.SourceID},
		})
	}
	if strings.TrimSpace(candidate.Canonical.Role) == "" {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{Code: "openproject_resource_demand_role_missing", Message: fmt.Sprintf("OpenProject sidecar demand %s has no role", candidate.SourceID), Severity: "warning", Target: Target{Kind: "resource_demand", ID: candidate.SourceID}})
	}
	if strings.TrimSpace(candidate.Canonical.Criticality) == "" {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{Code: "openproject_resource_demand_criticality_missing", Message: fmt.Sprintf("OpenProject sidecar demand %s has no criticality", candidate.SourceID), Severity: "warning", Target: Target{Kind: "resource_demand", ID: candidate.SourceID}})
	}
	if strings.TrimSpace(candidate.Canonical.Status) == "" {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{Code: "openproject_resource_demand_status_missing", Message: fmt.Sprintf("OpenProject sidecar demand %s has no status", candidate.SourceID), Severity: "warning", Target: Target{Kind: "resource_demand", ID: candidate.SourceID}})
	}
	tuple := candidate.WorkPackageID + "|" + candidate.Canonical.ResourceRef.ID + "|" + candidate.Canonical.ResourcePoolRef.ID + "|" + candidate.Canonical.Role + "|" + candidate.Canonical.Unit
	if seenTuples[tuple] {
		candidate.Diagnostics = append(candidate.Diagnostics, Diagnostic{Code: "openproject_resource_demand_duplicate_tuple", Message: fmt.Sprintf("OpenProject sidecar demand %s duplicates work-package/resource/role/unit tuple", candidate.SourceID), Severity: "warning", Target: Target{Kind: "resource_demand", ID: candidate.SourceID}})
	}
	seenTuples[tuple] = true
}

func matchOpenProjectTask(tasks []domain.Task, sourceID, title string) (string, string) {
	normalizedTitle := strings.ToLower(strings.TrimSpace(title))
	for _, task := range tasks {
		for _, ref := range task.SourceRefs {
			if ref.Source == "openproject" && ref.ObjectType == "work_package" && ref.ObjectID == sourceID {
				return task.ID, "source_ref"
			}
		}
	}
	for _, task := range tasks {
		if strings.ToLower(strings.TrimSpace(task.Title)) == normalizedTitle && normalizedTitle != "" {
			return task.ID, "title"
		}
	}
	return "", ""
}

func matchTaskForOpenProjectResourceDemand(ws domain.Workspace, candidates []OpenProjectTaskCandidate, workPackageID string) string {
	for _, task := range ws.Tasks {
		for _, ref := range task.SourceRefs {
			if ref.Source == "openproject" && ref.ObjectType == "work_package" && ref.ObjectID == workPackageID {
				return task.ID
			}
		}
	}
	for _, candidate := range candidates {
		if candidate.SourceID != workPackageID {
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

func openProjectWorkPackageKnown(candidates []OpenProjectTaskCandidate, workPackageID string) bool {
	for _, candidate := range candidates {
		if candidate.SourceID == workPackageID {
			return true
		}
	}
	return false
}

func existingDemandID(demands []domain.ResourceDemand, id string) bool {
	for _, demand := range demands {
		if demand.ID == id {
			return true
		}
	}
	return false
}

func resourceExists(resources []domain.Resource, id string) bool {
	for _, resource := range resources {
		if resource.ID == id {
			return true
		}
	}
	return false
}

func resourcePoolExists(pools []domain.ResourcePool, id string) bool {
	for _, pool := range pools {
		if pool.ID == id {
			return true
		}
	}
	return false
}

func allowedOpenProjectResourceDemandUnit(unit string) bool {
	switch strings.TrimSpace(unit) {
	case "hours", "review_hours", "agent_hours", "tokens", "api_requests", "requests_per_minute", "concurrent_slots", "slots", "pieces":
		return true
	default:
		return false
	}
}

func mapOpenProjectResourceLinks(ws domain.Workspace, sidecar *openProjectResourceLinksSidecar, sidecarPath string, warnings *[]Diagnostic, errors *[]Diagnostic) map[string]string {
	links := map[string]string{}
	if sidecar == nil {
		return links
	}
	if len(sidecar.OpenProjectResourceLinks) == 0 {
		*warnings = append(*warnings, Diagnostic{
			Code:     "openproject_resource_link_empty",
			Message:  fmt.Sprintf("OpenProject resource-link sidecar %s contains no openproject_resource_links", sidecarPath),
			Severity: "warning",
			Target:   Target{Kind: "file", ID: sidecarPath},
		})
		return links
	}
	seenUsers := map[string]bool{}
	for _, link := range sidecar.OpenProjectResourceLinks {
		userID := strings.TrimSpace(link.UserID)
		if userID == "" {
			userID = lastPathSegment(link.UserHref)
		}
		if userID == "" {
			*errors = append(*errors, Diagnostic{
				Code:     "openproject_resource_link_user_missing",
				Message:  "OpenProject resource-link sidecar entry is missing user_id",
				Severity: "error",
				Target:   Target{Kind: "file", ID: sidecarPath},
			})
			continue
		}
		if seenUsers[userID] {
			*errors = append(*errors, Diagnostic{
				Code:     "openproject_resource_link_duplicate_user",
				Message:  fmt.Sprintf("OpenProject resource-link sidecar contains duplicate user_id %s", userID),
				Severity: "error",
				Target:   Target{Kind: "openproject_user", ID: userID},
			})
			continue
		}
		seenUsers[userID] = true
		if strings.TrimSpace(link.ResourceRef.Kind) != "resource" {
			*errors = append(*errors, Diagnostic{
				Code:     "openproject_resource_link_kind_unsupported",
				Message:  fmt.Sprintf("OpenProject resource link for user %s must reference kind resource", userID),
				Severity: "error",
				Target:   Target{Kind: "openproject_user", ID: userID},
			})
			continue
		}
		if strings.TrimSpace(link.ResourceRef.ID) == "" {
			*errors = append(*errors, Diagnostic{
				Code:     "openproject_resource_link_resource_missing",
				Message:  fmt.Sprintf("OpenProject resource link for user %s has no resource_ref.id", userID),
				Severity: "error",
				Target:   Target{Kind: "openproject_user", ID: userID},
			})
			continue
		}
		if !resourceExists(ws.Resources, link.ResourceRef.ID) {
			*errors = append(*errors, Diagnostic{
				Code:     "openproject_resource_link_resource_unknown",
				Message:  fmt.Sprintf("OpenProject resource link for user %s references unknown resource %s", userID, link.ResourceRef.ID),
				Severity: "error",
				Target:   Target{Kind: "resource", ID: link.ResourceRef.ID},
			})
			continue
		}
		links[userID] = link.ResourceRef.ID
	}
	return links
}

func matchOpenProjectResourceCandidate(ws domain.Workspace, candidate OpenProjectResourceCandidate, resourceLinks map[string]string) OpenProjectResourceCandidate {
	if matchedID := matchResourceByOpenProjectUserSourceRef(ws.Resources, candidate.SourceID); matchedID != "" {
		candidate.MatchedRefID = matchedID
		candidate.MatchReason = "source_ref"
		return candidate
	}
	if matchedID := resourceLinks[candidate.SourceID]; matchedID != "" {
		candidate.MatchedRefID = matchedID
		candidate.MatchReason = "resource_link"
		return candidate
	}
	if matchedID := matchResourceByName(ws.Resources, candidate.DisplayName); matchedID != "" {
		candidate.MatchedRefID = matchedID
		candidate.MatchReason = "name"
		return candidate
	}
	return candidate
}

func matchResourceByOpenProjectUserSourceRef(resources []domain.Resource, userID string) string {
	if strings.TrimSpace(userID) == "" {
		return ""
	}
	for _, resource := range resources {
		for _, ref := range resource.SourceRefs {
			if ref.Source == "openproject" && ref.ObjectType == "user" && ref.ObjectID == userID {
				return resource.ID
			}
		}
	}
	return ""
}

func matchResourceByName(resources []domain.Resource, name string) string {
	normalizedName := strings.ToLower(strings.TrimSpace(name))
	for _, resource := range resources {
		if strings.ToLower(strings.TrimSpace(resource.Name)) == normalizedName && normalizedName != "" {
			return resource.ID
		}
	}
	return ""
}

func mapOpenProjectStatus(status string) string {
	switch normalizeStatus(status) {
	case "new", "open":
		return "not_started"
	case "tobescheduled", "scheduled", "inspecification", "specified", "confirmed":
		return "ready"
	case "inprogress", "progress", "development", "developing":
		return "in_progress"
	case "blocked", "onhold":
		return "blocked"
	case "review", "readyforreview", "developed", "intesting", "tested":
		return "review_ready"
	case "closed", "done", "resolved", "accepted":
		return "accepted"
	case "rejected", "cancelled", "canceled":
		return "cancelled"
	default:
		return ""
	}
}

func normalizeStatus(status string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(status) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func parseOpenProjectDurationHours(value string) float64 {
	value = strings.TrimSpace(strings.ToUpper(value))
	if value == "" || !strings.HasPrefix(value, "PT") {
		return 0
	}
	value = strings.TrimPrefix(value, "PT")
	total := 0.0
	number := strings.Builder{}
	for _, r := range value {
		switch {
		case unicode.IsDigit(r) || r == '.':
			number.WriteRune(r)
		case r == 'H' || r == 'M':
			if number.Len() == 0 {
				continue
			}
			parsed, err := strconv.ParseFloat(number.String(), 64)
			if err == nil {
				if r == 'H' {
					total += parsed
				} else {
					total += parsed / 60
				}
			}
			number.Reset()
		default:
			number.Reset()
		}
	}
	return total
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func lastPathSegment(path string) string {
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	return parts[len(parts)-1]
}
