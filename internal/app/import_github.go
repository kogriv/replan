package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

const GitHubImportFormatV1 = "replan.github-issues.v1"

type GitHubImportService struct {
	loader WorkspaceLoader
}

type GitHubImportRequest struct {
	FromFile               string
	DryRun                 bool
	Profile                string
	Since                  string
	IncludeClosed          bool
	IncludeRelations       bool
	ResourceLinksSidecar   string
	ResourceDemandsSidecar string
	Now                    time.Time
}

type GitHubImportResult struct {
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
	PlannedWrites            []PlannedWrite                       `json:"planned_writes"`
	WrittenFiles             []string                             `json:"written_files,omitempty"`
	Warnings                 []Diagnostic                         `json:"warnings"`
	Errors                   []Diagnostic                         `json:"errors"`
}

// GitHubImportDocument is the network-neutral document consumed by both the
// file adapter and the later live reader.
type GitHubImportDocument struct {
	Format       string              `json:"format"`
	Repository   GitHubRepository    `json:"repository"`
	Issues       []GitHubIssue       `json:"issues"`
	BlockedBy    []GitHubBlockedBy   `json:"blocked_by,omitempty"`
	ProjectItems []GitHubProjectItem `json:"project_items,omitempty"`
}

type GitHubRepository struct {
	Owner   string `json:"owner"`
	Name    string `json:"name"`
	HTMLURL string `json:"html_url,omitempty"`
}

type GitHubIssue struct {
	ID          int64            `json:"id"`
	Number      int              `json:"number"`
	Title       string           `json:"title"`
	State       string           `json:"state"`
	StateReason string           `json:"state_reason,omitempty"`
	HTMLURL     string           `json:"html_url,omitempty"`
	UpdatedAt   string           `json:"updated_at,omitempty"`
	Labels      []GitHubLabel    `json:"labels,omitempty"`
	Assignees   []GitHubAssignee `json:"assignees,omitempty"`
	PullRequest json.RawMessage  `json:"pull_request,omitempty"`
	Parent      *GitHubIssueRef  `json:"parent,omitempty"`
}

type GitHubLabel struct {
	Name string `json:"name"`
}

type GitHubAssignee struct {
	Login   string `json:"login"`
	HTMLURL string `json:"html_url,omitempty"`
}

type GitHubIssueRef struct {
	Repository string `json:"repository"`
	Number     int    `json:"number"`
	ID         int64  `json:"id,omitempty"`
	HTMLURL    string `json:"html_url,omitempty"`
}

type GitHubBlockedBy struct {
	IssueNumber int              `json:"issue_number"`
	Issues      []GitHubIssueRef `json:"issues"`
}

type GitHubProjectItem struct {
	IssueNumber int            `json:"issue_number"`
	Fields      map[string]any `json:"fields"`
}

type githubResourceLinksSidecar struct {
	Links []githubResourceLink `yaml:"github_resource_links"`
}

type githubResourceLink struct {
	Login       string     `yaml:"login"`
	ResourceRef domain.Ref `yaml:"resource_ref"`
}

type githubResourceDemandsSidecar struct {
	Entries []githubResourceDemandEntry `yaml:"github_resource_demands"`
}

type githubResourceDemandEntry struct {
	Issue   string                        `yaml:"issue"`
	Demands []githubResourceSidecarDemand `yaml:"demands"`
}

type githubResourceSidecarDemand struct {
	ID                 string     `yaml:"id"`
	ResourceRef        domain.Ref `yaml:"resource_ref"`
	ResourcePoolRef    domain.Ref `yaml:"resource_pool_ref"`
	ResourceClass      string     `yaml:"resource_class"`
	PlannedAmount      float64    `yaml:"planned_amount"`
	RemainingAmount    *float64   `yaml:"remaining_amount"`
	Unit               string     `yaml:"unit"`
	Role               string     `yaml:"role"`
	Criticality        string     `yaml:"criticality"`
	Status             string     `yaml:"status"`
	ProjectNumberField string     `yaml:"project_number_field"`
}

type gitHubImportManifest struct {
	SourceSnapshot gitHubImportManifestBody `yaml:"source_snapshot"`
}

type gitHubImportManifestBody struct {
	ID               string `yaml:"id"`
	Source           string `yaml:"source"`
	SourceMode       string `yaml:"source_mode"`
	ImportedAt       string `yaml:"imported_at"`
	ProfileID        string `yaml:"profile_id,omitempty"`
	Repository       string `yaml:"repository"`
	Since            string `yaml:"since,omitempty"`
	IncludeClosed    bool   `yaml:"include_closed,omitempty"`
	IncludeRelations bool   `yaml:"include_relations,omitempty"`
	RawLocation      string `yaml:"raw_location"`
	MappedLocation   string `yaml:"mapped_location"`
	AdapterVersion   string `yaml:"adapter_version"`
	ObjectCount      int    `yaml:"object_count"`
	WarningCount     int    `yaml:"warning_count"`
}

func NewGitHubImportService(loader WorkspaceLoader) GitHubImportService {
	return GitHubImportService{loader: loader}
}

func (s GitHubImportService) Import(ctx context.Context, req GitHubImportRequest) (GitHubImportResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := GitHubImportResult{
		DryRun: req.DryRun, Source: "github", FromFile: req.FromFile,
		TaskCandidates: []OpenProjectTaskCandidate{}, ResourceCandidates: []OpenProjectResourceCandidate{},
		ResourceDemandCandidates: []OpenProjectResourceDemandCandidate{}, DependencyCandidates: []OpenProjectDependencyCandidate{},
		PlannedWrites: []PlannedWrite{}, WrittenFiles: []string{}, Warnings: []Diagnostic{}, Errors: []Diagnostic{},
	}
	appendLoadDiagnostics(&result.Warnings, &result.Errors, loadDiags)
	if strings.TrimSpace(req.FromFile) == "" {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_from_file_required", Message: "GitHub import currently requires --from-file; live input is a separate task", Severity: "error"})
	}
	profile, profileSummary, profileDiagnostic := resolveGitHubProfile(ws.Project.SourceProfiles, req.Profile)
	result.Profile = profileSummary
	if profileDiagnostic != nil {
		result.Errors = append(result.Errors, *profileDiagnostic)
	}
	if len(result.Errors) > 0 {
		return result, ErrValidationFailed
	}
	doc, raw, err := readGitHubImportDocument(req.FromFile)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_file_read_error", Message: fmt.Sprintf("failed to read GitHub import file: %v", err), Severity: "error", Target: Target{Kind: "file", ID: req.FromFile}})
		return result, ErrValidationFailed
	}
	repository := doc.Repository.Owner + "/" + doc.Repository.Name
	if profile.Repository != "" && !strings.EqualFold(profile.Repository, repository) {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_repository_mismatch", Message: fmt.Sprintf("input repository %s does not match source profile repository %s", repository, profile.Repository), Severity: "error"})
		return result, ErrValidationFailed
	}
	links, err := readGitHubYAML[githubResourceLinksSidecar](s.loader.ProjectPath(), req.ResourceLinksSidecar, "integrations/github/resource-links.yaml")
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_resource_link_sidecar_read_error", Message: err.Error(), Severity: "error"})
		return result, ErrValidationFailed
	}
	demands, err := readGitHubYAML[githubResourceDemandsSidecar](s.loader.ProjectPath(), req.ResourceDemandsSidecar, "integrations/github/resource-demands.yaml")
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_resource_demand_sidecar_read_error", Message: err.Error(), Severity: "error"})
		return result, ErrValidationFailed
	}
	linkIndex := map[string]domain.Ref{}
	if links != nil {
		for _, link := range links.Links {
			linkIndex[strings.ToLower(strings.TrimSpace(link.Login))] = link.ResourceRef
		}
	}
	projectFields := map[int]map[string]any{}
	for _, item := range doc.ProjectItems {
		projectFields[item.IssueNumber] = item.Fields
	}
	since, sinceErr := parseGitHubSince(req.Since)
	if sinceErr != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_since_invalid", Message: sinceErr.Error(), Severity: "error"})
		return result, ErrValidationFailed
	}
	issueIndex := map[int]OpenProjectTaskCandidate{}
	for _, issue := range doc.Issues {
		if len(issue.PullRequest) > 0 && string(issue.PullRequest) != "null" {
			continue
		}
		if issue.State == "closed" && !req.IncludeClosed {
			continue
		}
		if !since.IsZero() && !githubIssueAtOrAfter(issue.UpdatedAt, since) {
			continue
		}
		candidate := mapGitHubIssue(ws, repository, issue, profile, projectFields[issue.Number], &result.Warnings)
		result.TaskCandidates = append(result.TaskCandidates, candidate)
		issueIndex[issue.Number] = candidate
		if len(issue.Assignees) > 0 {
			assignee := issue.Assignees[0]
			resource := OpenProjectResourceCandidate{Source: "github", ObjectType: "user", SourceID: assignee.Login, SourceHref: assignee.HTMLURL, DisplayName: assignee.Login}
			if ref, ok := linkIndex[strings.ToLower(assignee.Login)]; ok {
				resource.MatchedRefID, resource.MatchReason = ref.ID, "resource_link"
			} else {
				resource.WouldCreate = true
				result.Warnings = append(result.Warnings, Diagnostic{Code: "github_assignee_link_missing", Message: fmt.Sprintf("GitHub assignee %s has no resource-links sidecar entry", assignee.Login), Severity: "warning", Target: Target{Kind: "github_user", ID: assignee.Login}})
			}
			if !containsGitHubResourceCandidate(result.ResourceCandidates, resource.SourceID) {
				result.ResourceCandidates = append(result.ResourceCandidates, resource)
			}
		}
	}
	if demands != nil {
		result.ResourceDemandCandidates = mapGitHubResourceDemands(ws, issueIndex, projectFields, profile, demands.Entries, &result.Warnings)
	}
	if req.IncludeRelations {
		result.DependencyCandidates = mapGitHubBlockedBy(ws, repository, issueIndex, doc.BlockedBy, &result.Warnings)
	}
	sort.Slice(result.TaskCandidates, func(i, j int) bool { return result.TaskCandidates[i].NumericID < result.TaskCandidates[j].NumericID })
	sort.Slice(result.ResourceCandidates, func(i, j int) bool {
		return result.ResourceCandidates[i].SourceID < result.ResourceCandidates[j].SourceID
	})
	importedAt := req.Now
	if importedAt.IsZero() {
		importedAt = time.Now()
	}
	key := importedAt.Format("20060102T150405")
	result.Snapshot = OpenProjectSnapshotSummary{ID: "source_snapshot.github." + key, Key: key, CollectionType: "GitHubIssues", DeclaredTotal: len(doc.Issues), DeclaredCount: len(doc.Issues), LoadedCount: len(result.TaskCandidates), SourceMode: "file"}
	write, err := buildGitHubSnapshotWrite(result, req, profile, repository, raw, importedAt)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_snapshot_render_error", Message: err.Error(), Severity: "error"})
		return result, ErrValidationFailed
	}
	result.PlannedWrites = githubSnapshotPlannedWrites(key)
	if req.DryRun {
		return result, nil
	}
	writer, ok := s.loader.(ImportSnapshotWriter)
	if !ok {
		result.Errors = append(result.Errors, Diagnostic{Code: "snapshot_writer_unavailable", Message: "project store does not support import snapshot writes", Severity: "error"})
		return result, ErrValidationFailed
	}
	result.WrittenFiles, err = writer.WriteImportSnapshot("github", write)
	if err != nil {
		result.Errors = append(result.Errors, Diagnostic{Code: "github_snapshot_write_error", Message: err.Error(), Severity: "error"})
		return result, err
	}
	result.Applied = true
	return result, nil
}

func resolveGitHubProfile(profiles []domain.SourceProfile, id string) (domain.SourceProfile, *SourceProfileSummary, *Diagnostic) {
	if strings.TrimSpace(id) == "" {
		return domain.SourceProfile{}, nil, nil
	}
	profile, ok := findSourceProfile(profiles, id, "github")
	if !ok {
		return domain.SourceProfile{}, nil, &Diagnostic{Code: "source_profile_not_found", Message: fmt.Sprintf("GitHub source profile %q was not found", id), Severity: "error", Target: Target{Kind: "source_profile", ID: id}}
	}
	return profile, sourceProfileSummary(profile), nil
}

func readGitHubImportDocument(path string) (GitHubImportDocument, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return GitHubImportDocument{}, nil, err
	}
	var doc GitHubImportDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return doc, nil, err
	}
	if doc.Format != GitHubImportFormatV1 {
		return doc, nil, fmt.Errorf("unsupported GitHub input format %q", doc.Format)
	}
	if strings.TrimSpace(doc.Repository.Owner) == "" || strings.TrimSpace(doc.Repository.Name) == "" {
		return doc, nil, fmt.Errorf("repository owner and name are required")
	}
	return doc, raw, nil
}

func readGitHubYAML[T any](projectPath, requestedPath, defaultPath string) (*T, error) {
	explicit := strings.TrimSpace(requestedPath) != ""
	path := strings.TrimSpace(requestedPath)
	if path == "" {
		path = filepath.Join(projectPath, defaultPath)
	} else if !filepath.IsAbs(path) {
		path = filepath.Join(projectPath, path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && !explicit {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var value T
	if err := yaml.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &value, nil
}

func mapGitHubIssue(ws domain.Workspace, repository string, issue GitHubIssue, profile domain.SourceProfile, fields map[string]any, warnings *[]Diagnostic) OpenProjectTaskCandidate {
	sourceID := repository + "#" + strconv.Itoa(issue.Number)
	statusMap := githubStatusMap(profile)
	statusKey := githubStatusKey(issue, statusMap)
	mapped := statusMap[statusKey]
	if mapped == "" {
		*warnings = append(*warnings, Diagnostic{Code: "unmapped_github_status", Message: fmt.Sprintf("GitHub status %q for issue %s is not mapped", statusKey, sourceID), Severity: "warning", Target: Target{Kind: "github_issue", ID: sourceID}})
	}
	candidate := OpenProjectTaskCandidate{Source: "github", ObjectType: "issue", NumericID: issue.ID, SourceID: sourceID, SourceURL: issue.HTMLURL, Title: issue.Title, SourceStatus: statusKey, MappedStatus: mapped, SourceRefs: []domain.SourceRef{{Source: "github", ObjectType: "issue", ObjectID: sourceID, URL: issue.HTMLURL}}}
	if issue.Parent != nil {
		parentRepo := issue.Parent.Repository
		if parentRepo == "" {
			parentRepo = repository
		}
		candidate.ParentRef = &domain.SourceRef{Source: "github", ObjectType: "issue", ObjectID: parentRepo + "#" + strconv.Itoa(issue.Parent.Number), URL: issue.Parent.HTMLURL}
	}
	if len(issue.Assignees) > 0 {
		candidate.AssigneeName, candidate.AssigneeHref = issue.Assignees[0].Login, issue.Assignees[0].HTMLURL
	}
	if profile.StartField != "" {
		candidate.StartDate = githubStringField(fields, profile.StartField)
	}
	if profile.FinishField != "" {
		candidate.DueDate = githubStringField(fields, profile.FinishField)
	}
	for _, task := range ws.Tasks {
		if taskHasSourceRef(task, "github", "issue", sourceID) {
			candidate.MatchedTaskID, candidate.MatchReason = task.ID, "source_ref"
			return candidate
		}
	}
	for _, task := range ws.Tasks {
		if task.Title == issue.Title {
			candidate.MatchedTaskID, candidate.MatchReason = task.ID, "title"
			return candidate
		}
	}
	candidate.WouldCreate = true
	*warnings = append(*warnings, Diagnostic{Code: "unmatched_github_issue", Message: fmt.Sprintf("GitHub issue %s has no matching RePlan task", sourceID), Severity: "warning", Target: Target{Kind: "github_issue", ID: sourceID}})
	return candidate
}

func githubStatusMap(profile domain.SourceProfile) map[string]string {
	result := map[string]string{"open": "not_started", "closed:completed": "accepted", "closed:not_planned": "cancelled", "closed:duplicate": "cancelled"}
	for key, value := range profile.StatusMap {
		result[strings.ToLower(strings.TrimSpace(key))] = value
	}
	return result
}

func githubStatusKey(issue GitHubIssue, statusMap map[string]string) string {
	state := strings.ToLower(strings.TrimSpace(issue.State))
	labels := map[string]bool{}
	for _, label := range issue.Labels {
		labels[strings.ToLower(strings.TrimSpace(label.Name))] = true
	}
	if state == "open" {
		keys := make([]string, 0, len(labels))
		for label := range labels {
			keys = append(keys, label)
		}
		sort.Strings(keys)
		for _, label := range keys {
			key := "open:label:" + label
			if statusMap[key] != "" {
				return key
			}
		}
		return "open"
	}
	if state == "closed" && labels["duplicate"] {
		return "closed:duplicate"
	}
	if reason := strings.ToLower(strings.TrimSpace(issue.StateReason)); reason != "" {
		return state + ":" + reason
	}
	return state
}

func parseGitHubSince(value string) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, nil
	}
	if t, err := time.Parse("2006-01-02", value); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf("--since must use YYYY-MM-DD or RFC3339")
}

func githubIssueAtOrAfter(updated string, since time.Time) bool {
	t, err := time.Parse(time.RFC3339, updated)
	return err == nil && !t.Before(since)
}

func githubStringField(fields map[string]any, name string) string {
	if value, ok := fields[name].(string); ok {
		return strings.TrimSpace(value)
	}
	return ""
}

func githubNumberField(fields map[string]any, name string) (float64, bool) {
	value, ok := fields[name]
	if !ok {
		return 0, false
	}
	switch number := value.(type) {
	case float64:
		return number, true
	case json.Number:
		v, err := number.Float64()
		return v, err == nil
	default:
		return 0, false
	}
}

func containsGitHubResourceCandidate(candidates []OpenProjectResourceCandidate, sourceID string) bool {
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.SourceID, sourceID) {
			return true
		}
	}
	return false
}

func mapGitHubResourceDemands(ws domain.Workspace, issues map[int]OpenProjectTaskCandidate, fields map[int]map[string]any, profile domain.SourceProfile, entries []githubResourceDemandEntry, warnings *[]Diagnostic) []OpenProjectResourceDemandCandidate {
	byID := map[string]OpenProjectTaskCandidate{}
	byNumber := map[string]int{}
	for number, candidate := range issues {
		byID[candidate.SourceID], byNumber[candidate.SourceID] = candidate, number
	}
	var result []OpenProjectResourceDemandCandidate
	for _, entry := range entries {
		candidate, ok := byID[entry.Issue]
		if !ok {
			*warnings = append(*warnings, Diagnostic{Code: "github_resource_demand_issue_unknown", Message: fmt.Sprintf("GitHub resource-demand sidecar issue %s is not imported", entry.Issue), Severity: "warning"})
			continue
		}
		for _, item := range entry.Demands {
			amount := item.PlannedAmount
			fieldName := item.ProjectNumberField
			if fieldName == "" {
				fieldName = profile.EstimateField
			}
			if fieldName != "" {
				if value, ok := githubNumberField(fields[byNumber[entry.Issue]], fieldName); ok {
					amount = value
				} else {
					*warnings = append(*warnings, Diagnostic{Code: "github_project_estimate_missing", Message: fmt.Sprintf("GitHub issue %s has no numeric Project field %q", entry.Issue, fieldName), Severity: "warning"})
				}
			}
			remaining := item.RemainingAmount
			if remaining == nil {
				v := amount
				remaining = &v
			}
			canonical := domain.ResourceDemand{ID: item.ID, TaskRef: domain.Ref{Kind: "task", ID: candidate.MatchedTaskID}, ResourceRef: item.ResourceRef, ResourcePoolRef: item.ResourcePoolRef, ResourceClass: item.ResourceClass, Role: item.Role, Unit: item.Unit, PlannedAmount: amount, RemainingAmount: remaining, Criticality: item.Criticality, Status: item.Status}
			accept := "safe_to_accept"
			if candidate.MatchedTaskID == "" {
				accept = "task_unmatched"
			}
			result = append(result, OpenProjectResourceDemandCandidate{TrackerSource: "github", Source: "github_sidecar", SourceID: entry.Issue + ":" + item.ID, WorkPackageID: entry.Issue, MatchedTaskID: candidate.MatchedTaskID, WouldCreate: accept == "safe_to_accept", AcceptStatus: accept, Canonical: canonical, Diagnostics: []Diagnostic{}})
		}
	}
	return result
}

func mapGitHubBlockedBy(ws domain.Workspace, repository string, issues map[int]OpenProjectTaskCandidate, relations []GitHubBlockedBy, warnings *[]Diagnostic) []OpenProjectDependencyCandidate {
	var result []OpenProjectDependencyCandidate
	for _, relation := range relations {
		successor, ok := issues[relation.IssueNumber]
		if !ok {
			continue
		}
		for _, blocking := range relation.Issues {
			blockingRepo := blocking.Repository
			if blockingRepo == "" {
				blockingRepo = repository
			}
			if !strings.EqualFold(blockingRepo, repository) {
				*warnings = append(*warnings, Diagnostic{Code: "github_cross_repository_blocked_by_skipped", Message: fmt.Sprintf("GitHub issue %s#%d is outside imported repository %s", blockingRepo, blocking.Number, repository), Severity: "warning", Target: Target{Kind: "github_issue", ID: blockingRepo + "#" + strconv.Itoa(blocking.Number)}})
				continue
			}
			predecessor, ok := issues[blocking.Number]
			if !ok {
				continue
			}
			sourceID := successor.SourceID + ":blocked_by:" + predecessor.SourceID
			accept := "safe_to_accept"
			if predecessor.MatchedTaskID == "" || successor.MatchedTaskID == "" {
				accept = "task_unmatched"
			}
			result = append(result, OpenProjectDependencyCandidate{Source: "github", ObjectType: "blocked_by", SourceID: sourceID, SourceURL: successor.SourceURL, RelationType: "blocked_by", FromWorkPackageID: predecessor.SourceID, ToWorkPackageID: successor.SourceID, PredecessorTaskID: predecessor.MatchedTaskID, SuccessorTaskID: successor.MatchedTaskID, DependencyID: "dependency.github.blocked-by." + importIDSlug(predecessor.SourceID) + "." + importIDSlug(successor.SourceID), DependencyType: "finish_to_start", WouldCreate: accept == "safe_to_accept", AcceptStatus: accept, Diagnostics: []Diagnostic{}})
		}
	}
	_ = ws
	return result
}

func buildGitHubSnapshotWrite(result GitHubImportResult, req GitHubImportRequest, profile domain.SourceProfile, repository string, raw []byte, importedAt time.Time) (OpenProjectImportSnapshotWrite, error) {
	base := "imports/github/" + result.Snapshot.Key + "/"
	mapped := OpenProjectMappedSnapshot{Source: "github", Snapshot: result.Snapshot, Profile: result.Profile, TaskCandidates: result.TaskCandidates, ResourceCandidates: result.ResourceCandidates, ResourceDemandCandidates: result.ResourceDemandCandidates, DependencyCandidates: result.DependencyCandidates, TimeEntryCandidates: []OpenProjectTimeEntryCandidate{}}
	mappedJSON, err := json.MarshalIndent(mapped, "", "  ")
	if err != nil {
		return OpenProjectImportSnapshotWrite{}, err
	}
	mappedJSON = append(mappedJSON, '\n')
	manifest := gitHubImportManifest{SourceSnapshot: gitHubImportManifestBody{ID: result.Snapshot.ID, Source: "github", SourceMode: "file", ImportedAt: importedAt.Format(time.RFC3339), ProfileID: profile.ID, Repository: repository, Since: req.Since, IncludeClosed: req.IncludeClosed, IncludeRelations: req.IncludeRelations, RawLocation: base + "raw.json", MappedLocation: base + "mapped.json", AdapterVersion: "replan-github-adapter-v0", ObjectCount: len(result.TaskCandidates), WarningCount: len(result.Warnings)}}
	manifestYAML, err := yaml.Marshal(manifest)
	if err != nil {
		return OpenProjectImportSnapshotWrite{}, err
	}
	return OpenProjectImportSnapshotWrite{SnapshotKey: result.Snapshot.Key, ManifestYAML: manifestYAML, RawJSON: raw, MappedJSON: mappedJSON}, nil
}

func githubSnapshotPlannedWrites(key string) []PlannedWrite {
	base := "imports/github/" + key + "/"
	return []PlannedWrite{{Path: base + "manifest.yaml", Action: "create", Format: "yaml"}, {Path: base + "raw.json", Action: "create", Format: "json"}, {Path: base + "mapped.json", Action: "create", Format: "json"}}
}
