package filestore

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

type Store struct {
	projectPath string
	fs          fileSystem
	bindings    *openProjectBindingsDoc
	bindingsErr *app.Diagnostic
}

func New(projectPath string) Store {
	return Store{projectPath: projectPath, fs: diskFS{root: projectPath}}
}

type openProjectBindingsDoc struct {
	Root openProjectBindingsRoot `yaml:"openproject_private_bindings"`
}

type openProjectBindingsRoot struct {
	Tasks        []openProjectTaskBinding `yaml:"tasks"`
	Dependencies []any                    `yaml:"dependencies"`
	Resources    []any                    `yaml:"resources"`
}

type openProjectTaskBinding struct {
	TaskID    string           `yaml:"replan_task_id"`
	SourceRef domain.SourceRef `yaml:"source_ref"`
}

// NewWithOpenProjectBindings configures a read-only task source-ref layer.
// The file is read once here; Load applies its in-memory representation only,
// and every write through the store is refused.
func NewWithOpenProjectBindings(projectPath, bindingsPath string) Store {
	store := Store{projectPath: projectPath, fs: readOnlyFS{diskFS{root: projectPath}}}
	document, diagnostic := loadOpenProjectBindings(bindingsPath)
	store.bindings = document
	store.bindingsErr = diagnostic
	return store
}

// OpenProjectBindingsError validates a requested layer for CLI usage checks.
func OpenProjectBindingsError(bindingsPath string) *app.Diagnostic {
	_, diagnostic := loadOpenProjectBindings(bindingsPath)
	return diagnostic
}

func loadOpenProjectBindings(bindingsPath string) (*openProjectBindingsDoc, *app.Diagnostic) {
	data, err := os.ReadFile(bindingsPath)
	if err != nil {
		diagnostic := diag("bindings_layer_missing", fmt.Sprintf("failed to read OpenProject bindings layer: %v", err), "error", "file", "")
		return nil, &diagnostic
	}
	var node yaml.Node
	if err := yaml.Unmarshal(data, &node); err != nil {
		diagnostic := diag("bindings_layer_invalid", fmt.Sprintf("failed to parse OpenProject bindings layer: %v", err), "error", "file", "")
		return nil, &diagnostic
	}
	root := mappingValue(&node, "openproject_private_bindings")
	if root == nil || root.Kind != yaml.MappingNode {
		diagnostic := diag("bindings_layer_invalid", "OpenProject bindings layer must contain an openproject_private_bindings mapping", "error", "file", "")
		return nil, &diagnostic
	}
	for _, key := range []string{"tasks", "dependencies", "resources"} {
		if value := mappingValue(root, key); value != nil && value.Kind != yaml.SequenceNode {
			diagnostic := diag("bindings_layer_invalid", fmt.Sprintf("OpenProject bindings layer %s must be a list", key), "error", "file", "")
			return nil, &diagnostic
		}
	}
	var document openProjectBindingsDoc
	if err := node.Decode(&document); err != nil {
		diagnostic := diag("bindings_layer_invalid", fmt.Sprintf("failed to decode OpenProject bindings layer: %v", err), "error", "file", "")
		return nil, &diagnostic
	}
	return &document, nil
}

func (s Store) layered() bool {
	return s.bindings != nil || s.bindingsErr != nil
}

func (s Store) ProjectPath() string {
	return s.projectPath
}

// StagedStore is a Store whose writes stay in memory until Commit. Loading
// through it returns the would-be project, so a command can validate the
// result of its writes before any file changes on disk.
type StagedStore struct {
	Store
	stage *stagedFS
}

// StageWrites returns a staged view of the project. It implements
// app.WriteStager.
func (s Store) StageWrites() app.StagedWrites {
	disk := diskFS{root: s.projectPath}
	stage := newStagedFS(disk)
	return &StagedStore{Store: Store{projectPath: s.projectPath, fs: stage, bindings: s.bindings, bindingsErr: s.bindingsErr}, stage: stage}
}

// Pending lists the paths the stage would write or remove.
func (s *StagedStore) Pending() []string {
	return s.stage.pending()
}

// Commit writes every staged change to disk and returns the paths written.
func (s *StagedStore) Commit() ([]string, error) {
	if s.layered() {
		return nil, ErrBindingsLayerReadOnly
	}
	return s.stage.commit()
}

type tasksDoc struct {
	Tasks []domain.Task `yaml:"tasks"`
}

type demandsDoc struct {
	ResourceDemands []domain.ResourceDemand `yaml:"resource_demands"`
}

type dependenciesDoc struct {
	Dependencies []domain.Dependency `yaml:"dependencies"`
}

type resourcesDoc struct {
	Resources []domain.Resource `yaml:"resources"`
}

type actualsDoc struct {
	Actuals []domain.Actual `yaml:"actuals"`
}

type evidenceDoc struct {
	Evidence []domain.Evidence `yaml:"evidence"`
}

type proposalDoc struct {
	Proposal domain.Proposal `yaml:"proposal"`
}

type scenarioDoc struct {
	Scenario domain.Scenario `yaml:"scenario"`
}

type projectDoc struct {
	Project domain.Project `yaml:"project"`
}

const (
	tasksRel        = "canonical/tasks.yaml"
	demandsRel      = "canonical/resource_demands.yaml"
	dependenciesRel = "canonical/dependencies.yaml"
	resourcesRel    = "canonical/resources.yaml"
)

// updateYAML is the single read-modify-write path for YAML files: read and
// parse the document, apply mutate, render and write it back through the
// store's file system (GAP-019). With missingOK a missing file starts as the
// zero document.
func updateYAML[T any](s Store, rel string, missingOK bool, mutate func(*T) error) error {
	var doc T
	data, err := s.fs.read(rel)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parse %s: %w", rel, err)
		}
	case errors.Is(err, os.ErrNotExist) && missingOK:
		data = nil
	default:
		return fmt.Errorf("read %s: %w", rel, err)
	}
	if err := mutate(&doc); err != nil {
		return err
	}
	return writeYAML(s, rel, data, doc)
}

// writeYAML writes doc to rel. original is the file doc was read from, or
// nil for a new file; records and fields that did not change keep their
// original bytes (GAP-022).
func writeYAML(s Store, rel string, original []byte, doc any) error {
	out, err := renderYAML(original, doc)
	if err != nil {
		return fmt.Errorf("render %s: %w", rel, err)
	}
	return s.fs.write(rel, out)
}

func taskIndex(tasks []domain.Task) map[string]int {
	index := make(map[string]int, len(tasks))
	for i, task := range tasks {
		index[task.ID] = i
	}
	return index
}

func demandIndex(demands []domain.ResourceDemand) map[string]int {
	index := make(map[string]int, len(demands))
	for i, demand := range demands {
		index[demand.ID] = i
	}
	return index
}

func dependencyIndex(dependencies []domain.Dependency) map[string]int {
	index := make(map[string]int, len(dependencies))
	for i, dependency := range dependencies {
		index[dependency.ID] = i
	}
	return index
}

// updateDemand applies mutate to one resource demand.
func (s Store) updateDemand(demandID string, mutate func(*domain.ResourceDemand)) (string, error) {
	err := updateYAML(s, demandsRel, false, func(doc *demandsDoc) error {
		i, ok := demandIndex(doc.ResourceDemands)[demandID]
		if !ok {
			return fmt.Errorf("resource demand %s not found", demandID)
		}
		mutate(&doc.ResourceDemands[i])
		return nil
	})
	return demandsRel, err
}

func (s Store) AppendActual(statusDate string, actual domain.Actual) (string, error) {
	rel := actualsPath(statusDate)
	err := updateYAML(s, rel, true, func(doc *actualsDoc) error {
		doc.Actuals = append(doc.Actuals, actual)
		return nil
	})
	return rel, err
}

func (s Store) SetResourceDemandRemaining(demandID string, amount float64) (string, error) {
	return s.updateDemand(demandID, func(demand *domain.ResourceDemand) {
		demand.RemainingAmount = domain.Amount(amount)
	})
}

func (s Store) SetResourceDemandStatus(demandID string, status string) (string, error) {
	return s.updateDemand(demandID, func(demand *domain.ResourceDemand) {
		demand.Status = status
	})
}

func (s Store) SetResourceDemandOverrunReview(demandID string, review domain.OverrunReview) (string, error) {
	return s.updateDemand(demandID, func(demand *domain.ResourceDemand) {
		demand.OverrunReview = &review
	})
}

func (s Store) SetTaskTrackingStatus(taskID string, status string) (string, error) {
	err := updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
		i, ok := taskIndex(doc.Tasks)[taskID]
		if !ok {
			return fmt.Errorf("task %s not found", taskID)
		}
		doc.Tasks[i].TrackingStatus = status
		return nil
	})
	return tasksRel, err
}

func (s Store) AddTask(task domain.Task, demands []domain.ResourceDemand, afterTaskID string) ([]string, error) {
	if err := updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
		doc.Tasks = append(doc.Tasks, task)
		return nil
	}); err != nil {
		return nil, err
	}
	if err := updateYAML(s, demandsRel, false, func(doc *demandsDoc) error {
		doc.ResourceDemands = append(doc.ResourceDemands, demands...)
		return nil
	}); err != nil {
		return []string{tasksRel}, err
	}
	const planRel = "canonical/current_plan.yaml"
	if err := s.insertCurrentPlanTask(planRel, task.ID, afterTaskID); err != nil {
		return []string{tasksRel, demandsRel}, err
	}
	return []string{tasksRel, demandsRel, planRel}, nil
}

// insertCurrentPlanTask is a narrow splice for the ordered included_tasks
// sequence. It inserts two lines and leaves every existing byte untouched.
func (s Store) insertCurrentPlanTask(rel, taskID, afterTaskID string) error {
	data, err := s.fs.read(rel)
	if err != nil {
		return fmt.Errorf("read %s: %w", rel, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return fmt.Errorf("parse %s: %w", rel, err)
	}
	plan := mappingValue(&root, "current_plan")
	included := mappingValue(plan, "included_tasks")
	if included == nil || included.Kind != yaml.SequenceNode || len(included.Content) == 0 {
		return fmt.Errorf("%s has no non-empty current_plan.included_tasks sequence", rel)
	}
	insertLine := len(splitLines(data))
	if afterTaskID != "" {
		found := false
		for i, item := range included.Content {
			if value := mappingValue(item, "id"); value != nil && value.Value == afterTaskID {
				found = true
				if i+1 < len(included.Content) {
					insertLine = included.Content[i+1].Line - 1
				}
				break
			}
		}
		if !found {
			return fmt.Errorf("after task %s is not included in current plan", afterTaskID)
		}
	}
	if insertLine == len(splitLines(data)) {
		// included_tasks is currently the last current_plan field in canonical
		// projects. If a later sibling exists, insert before it.
		for i := 0; i+1 < len(plan.Content); i += 2 {
			if plan.Content[i].Value == "included_tasks" && i+2 < len(plan.Content) {
				insertLine = plan.Content[i+2].Line - 1
				break
			}
		}
	}
	lines := splitLines(data)
	firstLine := lines[included.Content[0].Line-1]
	indent := len(firstLine) - len(strings.TrimLeft(firstLine, " "))
	var ref yaml.Node
	if err := ref.Encode(domain.Ref{Kind: "task", ID: taskID}); err != nil {
		return fmt.Errorf("render current-plan task reference: %w", err)
	}
	text, ok := renderSequenceItem("included_tasks", nil, &ref, indent)
	if !ok {
		return fmt.Errorf("render current-plan task reference %s", taskID)
	}
	lines = append(lines, make([]string, len(text))...)
	copy(lines[insertLine+len(text):], lines[insertLine:])
	copy(lines[insertLine:], text)
	return s.fs.write(rel, []byte(strings.Join(lines, "")))
}

func (s Store) SetTaskDates(taskID, plannedStart, plannedFinish string) (string, error) {
	err := updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
		i, ok := taskIndex(doc.Tasks)[taskID]
		if !ok {
			return fmt.Errorf("task %s not found", taskID)
		}
		doc.Tasks[i].PlannedStart = plannedStart
		doc.Tasks[i].PlannedFinish = plannedFinish
		return nil
	})
	return tasksRel, err
}

func (s Store) SetTaskTitle(taskID, title string) (string, error) {
	err := updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
		i, ok := taskIndex(doc.Tasks)[taskID]
		if !ok {
			return fmt.Errorf("task %s not found", taskID)
		}
		doc.Tasks[i].Title = title
		return nil
	})
	return tasksRel, err
}

const projectRel = ".replan/project.yaml"

// ProjectFormatVersion reads the declared project format. Projects created
// before the field existed are format 1.
func (s Store) ProjectFormatVersion() (version int, explicit bool, err error) {
	data, err := s.fs.read(projectRel)
	if err != nil {
		return 0, false, fmt.Errorf("read %s: %w", projectRel, err)
	}
	version, explicit, diagnostic := parseProjectFormatVersion(data)
	if diagnostic != nil {
		return 0, explicit, errors.New(diagnostic.Message)
	}
	return version, explicit, nil
}

// SetProjectFormatVersion uses the canonical YAML splicer, so adding the
// field leaves every unrelated byte in project.yaml untouched.
func (s Store) SetProjectFormatVersion(version int) (string, error) {
	err := updateYAML(s, projectRel, false, func(doc *projectDoc) error {
		doc.Project.FormatVersion = version
		return nil
	})
	return projectRel, err
}

// SetProjectStatusDate changes only project.default_status_date. It edits the
// YAML node tree instead of round-tripping a struct, so fields and comments
// the domain type does not model survive.
func (s Store) SetProjectStatusDate(date string) (string, error) {
	data, err := s.fs.read(projectRel)
	if err != nil {
		return projectRel, fmt.Errorf("read %s: %w", projectRel, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return projectRel, fmt.Errorf("parse %s: %w", projectRel, err)
	}
	project := mappingValue(&root, "project")
	if project == nil || project.Kind != yaml.MappingNode {
		return projectRel, fmt.Errorf("%s has no project mapping", projectRel)
	}
	if value := mappingValue(project, "default_status_date"); value != nil {
		value.Kind, value.Tag, value.Value, value.Style = yaml.ScalarNode, "!!str", date, yaml.DoubleQuotedStyle
	} else {
		project.Content = append(project.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "default_status_date"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: date, Style: yaml.DoubleQuotedStyle})
	}
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return projectRel, fmt.Errorf("render %s: %w", projectRel, err)
	}
	if err := encoder.Close(); err != nil {
		return projectRel, fmt.Errorf("render %s: %w", projectRel, err)
	}
	return projectRel, s.fs.write(projectRel, out.Bytes())
}

// mappingValue returns the value node for key in a mapping node, or in the
// mapping of a document node.
func mappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) > 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func (s Store) AppendAudit(record domain.AuditRecord) (string, error) {
	rel := app.AuditShardPath(record.OccurredAt)
	line, err := json.Marshal(record)
	if err != nil {
		return rel, fmt.Errorf("render audit record: %w", err)
	}
	existing, err := s.fs.read(rel)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return rel, fmt.Errorf("read audit file %s: %w", rel, err)
	}
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		existing = append(existing, '\n')
	}
	data := append(existing, append(line, '\n')...)
	if err := s.fs.write(rel, data); err != nil {
		return rel, fmt.Errorf("append audit file %s: %w", rel, err)
	}
	return rel, nil
}

// moveOpenProposal rewrites an open proposal under targetStatus and removes
// the open file. It returns the source and target paths.
func (s Store) moveOpenProposal(proposalID string, targetStatus string) (string, string, error) {
	sourceRel, proposal, err := s.readProposalByID(proposalID)
	if err != nil {
		return "", "", err
	}
	if proposal.Status != "open" {
		return "", "", fmt.Errorf("proposal %s has status %s", proposalID, proposal.Status)
	}
	targetRel := proposalPath(targetStatus, proposalID)
	exists, err := s.fs.exists(targetRel)
	if err != nil {
		return "", "", fmt.Errorf("stat proposal target file %s: %w", targetRel, err)
	}
	if exists {
		return "", "", fmt.Errorf("proposal target file %s already exists", targetRel)
	}
	proposal.Status = targetStatus
	source, err := s.fs.read(sourceRel)
	if err != nil {
		return "", "", fmt.Errorf("read proposal file %s: %w", sourceRel, err)
	}
	if err := writeYAML(s, targetRel, source, proposalDoc{Proposal: proposal}); err != nil {
		return "", "", fmt.Errorf("write proposal file %s: %w", targetRel, err)
	}
	if err := s.fs.remove(sourceRel); err != nil {
		return sourceRel, targetRel, fmt.Errorf("remove proposal file %s: %w", sourceRel, err)
	}
	return sourceRel, targetRel, nil
}

func (s Store) MoveProposal(proposalID string, targetStatus string, audit domain.AuditRecord) ([]string, error) {
	sourceRel, targetRel, err := s.moveOpenProposal(proposalID, targetStatus)
	if err != nil {
		return nil, err
	}
	auditRel, err := s.AppendAudit(audit)
	if err != nil {
		return []string{sourceRel, targetRel}, err
	}
	return []string{sourceRel, targetRel, auditRel}, nil
}

func (s Store) WriteProposals(proposals []domain.Proposal) ([]string, error) {
	written := []string{}
	for _, proposal := range proposals {
		if proposal.Status == "" {
			proposal.Status = "open"
		}
		rel := proposalPath(proposal.Status, proposal.ID)
		exists, err := s.fs.exists(rel)
		if err != nil {
			return written, fmt.Errorf("stat proposal file %s: %w", rel, err)
		}
		if exists {
			return written, fmt.Errorf("proposal file %s already exists", rel)
		}
		if err := writeYAML(s, rel, nil, proposalDoc{Proposal: proposal}); err != nil {
			return written, fmt.Errorf("write proposal file %s: %w", rel, err)
		}
		written = append(written, rel)
	}
	return written, nil
}

func (s Store) ApplyProposalDecision(proposalID string, targetStatus string, taskStatusChanges []app.ProposalTaskStatusChange, dependencyStatusChanges []app.ProposalDependencyStatusChange, resourceDemandChanges []app.ProposalResourceDemandChange, audit domain.AuditRecord) ([]string, error) {
	written := []string{}
	if _, proposal, err := s.readProposalByID(proposalID); err != nil {
		return written, err
	} else if proposal.Status != "open" {
		return written, fmt.Errorf("proposal %s has status %s", proposalID, proposal.Status)
	}
	targetRel := proposalPath(targetStatus, proposalID)
	if exists, err := s.fs.exists(targetRel); err != nil {
		return written, fmt.Errorf("stat proposal target file %s: %w", targetRel, err)
	} else if exists {
		return written, fmt.Errorf("proposal target file %s already exists", targetRel)
	}
	if len(taskStatusChanges) > 0 {
		err := updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
			index := taskIndex(doc.Tasks)
			for _, change := range taskStatusChanges {
				i, ok := index[change.TaskID]
				if !ok {
					return fmt.Errorf("task %s not found", change.TaskID)
				}
				doc.Tasks[i].TrackingStatus = change.NewStatus
			}
			return nil
		})
		if err != nil {
			return written, err
		}
		written = append(written, tasksRel)
	}
	if len(dependencyStatusChanges) > 0 {
		err := updateYAML(s, dependenciesRel, false, func(doc *dependenciesDoc) error {
			index := dependencyIndex(doc.Dependencies)
			for _, change := range dependencyStatusChanges {
				i, ok := index[change.DependencyID]
				if !ok {
					return fmt.Errorf("dependency %s not found", change.DependencyID)
				}
				doc.Dependencies[i].Status = change.NewStatus
			}
			return nil
		})
		if err != nil {
			return written, err
		}
		written = append(written, dependenciesRel)
	}
	if len(resourceDemandChanges) > 0 {
		err := updateYAML(s, demandsRel, false, func(doc *demandsDoc) error {
			index := demandIndex(doc.ResourceDemands)
			for _, change := range resourceDemandChanges {
				i, ok := index[change.DemandID]
				if !ok {
					return fmt.Errorf("resource demand %s not found", change.DemandID)
				}
				switch change.Field {
				case "remaining_amount":
					doc.ResourceDemands[i].RemainingAmount = domain.Amount(change.NewValue)
				default:
					return fmt.Errorf("unsupported proposal resource demand field %s", change.Field)
				}
			}
			return nil
		})
		if err != nil {
			return written, err
		}
		written = append(written, demandsRel)
	}
	sourceRel, targetRel, err := s.moveOpenProposal(proposalID, targetStatus)
	if targetRel != "" {
		written = append(written, targetRel)
	}
	if err != nil {
		return written, err
	}
	written = append(written, sourceRel)
	auditRel, err := s.AppendAudit(audit)
	if err != nil {
		return written, err
	}
	return append(written, auditRel), nil
}

func (s Store) ApplyScenarioChanges(scenarioID string, changes []app.ScenarioApplyChange, audit domain.AuditRecord) ([]string, error) {
	written := []string{}
	if touchesScenarioTargetKind(changes, "task") {
		err := s.applyScenarioTaskChanges(changes)
		if err != nil {
			return written, err
		}
		written = append(written, tasksRel)
	}
	if touchesScenarioTargetKind(changes, "resource_demand") {
		err := s.applyScenarioDemandChanges(changes)
		if err != nil {
			return written, err
		}
		written = append(written, demandsRel)
	}

	if touchesScenarioTargetKind(changes, "dependency") {
		err := s.applyScenarioDependencyChanges(changes)
		if err != nil {
			return written, err
		}
		written = append(written, dependenciesRel)
	}

	scenarioRel, _, err := s.readScenarioByID(scenarioID)
	if err != nil {
		return written, err
	}
	if err := updateYAML(s, scenarioRel, false, func(doc *scenarioDoc) error {
		doc.Scenario.Status = "applied"
		return nil
	}); err != nil {
		return written, fmt.Errorf("write scenario %s: %w", scenarioID, err)
	}
	written = append(written, scenarioRel)
	auditRel, err := s.AppendAudit(audit)
	if err != nil {
		return written, err
	}
	return append(written, auditRel), nil
}

func (s Store) applyScenarioTaskChanges(changes []app.ScenarioApplyChange) error {
	return updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
		index := taskIndex(doc.Tasks)
		for _, change := range changes {
			if change.TargetKind != "task" {
				continue
			}
			i, ok := index[change.TargetID]
			if !ok {
				return fmt.Errorf("task %s not found", change.TargetID)
			}
			value, ok := change.NewValue.(string)
			if !ok {
				return fmt.Errorf("task %s field %s value must be string", change.TargetID, change.Field)
			}
			switch change.Field {
			case "planned_start":
				doc.Tasks[i].PlannedStart = value
			case "planned_finish":
				doc.Tasks[i].PlannedFinish = value
			default:
				return fmt.Errorf("unsupported scenario apply field %s", change.Field)
			}
		}
		return nil
	})
}

func (s Store) applyScenarioDemandChanges(changes []app.ScenarioApplyChange) error {
	return updateYAML(s, demandsRel, false, func(doc *demandsDoc) error {
		index := demandIndex(doc.ResourceDemands)
		for _, change := range changes {
			if change.TargetKind != "resource_demand" {
				continue
			}
			i, ok := index[change.TargetID]
			if !ok {
				return fmt.Errorf("resource demand %s not found", change.TargetID)
			}
			value, ok := change.NewValue.(float64)
			if !ok {
				return fmt.Errorf("resource demand %s field %s value must be numeric", change.TargetID, change.Field)
			}
			switch change.Field {
			case "remaining_amount":
				doc.ResourceDemands[i].RemainingAmount = domain.Amount(value)
			case "planned_amount":
				doc.ResourceDemands[i].PlannedAmount = value
			default:
				return fmt.Errorf("unsupported scenario apply field %s", change.Field)
			}
		}
		return nil
	})
}

func (s Store) applyScenarioDependencyChanges(changes []app.ScenarioApplyChange) error {
	return updateYAML(s, dependenciesRel, false, func(doc *dependenciesDoc) error {
		index := dependencyIndex(doc.Dependencies)
		for _, change := range changes {
			if change.TargetKind != "dependency" {
				continue
			}
			i, ok := index[change.TargetID]
			if !ok {
				return fmt.Errorf("dependency %s not found", change.TargetID)
			}
			value, ok := change.NewValue.(string)
			if !ok || change.Field != "status" {
				return fmt.Errorf("unsupported scenario apply field %s for dependency %s", change.Field, change.TargetID)
			}
			doc.Dependencies[i].Status = value
		}
		return nil
	})
}

func (s Store) WriteOpenProjectImportSnapshot(snapshot app.OpenProjectImportSnapshotWrite) ([]string, error) {
	base := filepath.ToSlash(filepath.Join("imports", "openproject", snapshot.SnapshotKey))
	files := []struct {
		rel  string
		data []byte
	}{
		{rel: filepath.ToSlash(filepath.Join(base, "manifest.yaml")), data: snapshot.ManifestYAML},
		{rel: filepath.ToSlash(filepath.Join(base, "raw.json")), data: snapshot.RawJSON},
		{rel: filepath.ToSlash(filepath.Join(base, "mapped.json")), data: snapshot.MappedJSON},
	}
	written := make([]string, 0, len(files))
	for _, file := range files {
		if err := s.fs.write(file.rel, file.data); err != nil {
			return written, err
		}
		written = append(written, file.rel)
	}
	return written, nil
}

func (s Store) ReadImportMappedSnapshot(snapshotID string) ([]byte, string, error) {
	const prefix = "source_snapshot.openproject."
	if !strings.HasPrefix(snapshotID, prefix) {
		return nil, "", fmt.Errorf("source snapshot %q: %w", snapshotID, os.ErrNotExist)
	}
	key := strings.TrimPrefix(snapshotID, prefix)
	if key == "" || strings.ContainsAny(key, `/\.`) {
		return nil, "", fmt.Errorf("invalid snapshot key %q", key)
	}
	rel := filepath.ToSlash(filepath.Join("imports", "openproject", key, "mapped.json"))
	data, err := s.fs.read(rel)
	if err != nil {
		return nil, rel, err
	}
	return data, rel, nil
}

// CreateProjectFile writes a new project file and refuses to replace one.
func (s Store) CreateProjectFile(rel string, content []byte) (string, error) {
	exists, err := s.fs.exists(rel)
	if err != nil {
		return rel, fmt.Errorf("stat %s: %w", rel, err)
	}
	if exists {
		return rel, fmt.Errorf("candidate file %s already exists", rel)
	}
	if err := s.fs.write(rel, content); err != nil {
		return rel, fmt.Errorf("write %s: %w", rel, err)
	}
	return rel, nil
}

// withAudit appends the audit record after a successful canonical write.
func (s Store) withAudit(rel string, err error, audit domain.AuditRecord) ([]string, error) {
	if err != nil {
		return nil, err
	}
	auditRel, err := s.AppendAudit(audit)
	if err != nil {
		return []string{rel}, err
	}
	return []string{rel, auditRel}, nil
}

func (s Store) ApplyImportAcceptTasks(changes []app.ImportAcceptTaskChange, audit domain.AuditRecord) ([]string, error) {
	err := updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
		index := taskIndex(doc.Tasks)
		for _, change := range changes {
			switch change.Op {
			case "create_task":
				if _, exists := index[change.TaskID]; exists {
					continue
				}
				doc.Tasks = append(doc.Tasks, change.Task)
				index[change.TaskID] = len(doc.Tasks) - 1
			case "add_task_source_ref":
				i, exists := index[change.TaskID]
				if !exists {
					return fmt.Errorf("task %s not found", change.TaskID)
				}
				if !hasSourceRef(doc.Tasks[i], change.SourceRef) {
					doc.Tasks[i].SourceRefs = append(doc.Tasks[i].SourceRefs, change.SourceRef)
				}
			default:
				return fmt.Errorf("unsupported import accept task change %s", change.Op)
			}
		}
		return nil
	})
	return s.withAudit(tasksRel, err, audit)
}

func (s Store) ApplyImportAcceptTrackingStatuses(changes []app.ImportAcceptTrackingStatusChange, audit domain.AuditRecord) ([]string, error) {
	err := updateYAML(s, tasksRel, false, func(doc *tasksDoc) error {
		index := taskIndex(doc.Tasks)
		for _, change := range changes {
			i, exists := index[change.TaskID]
			if !exists {
				return fmt.Errorf("task %s not found", change.TaskID)
			}
			doc.Tasks[i].TrackingStatus = change.TrackingStatus
		}
		return nil
	})
	return s.withAudit(tasksRel, err, audit)
}

func (s Store) ApplyImportAcceptDates(changes []app.ImportAcceptDateChange, audit domain.AuditRecord) ([]string, error) {
	for _, change := range changes {
		if _, err := s.SetTaskDates(change.TaskID, change.PlannedStart, change.PlannedFinish); err != nil {
			return nil, err
		}
	}
	return s.withAudit(tasksRel, nil, audit)
}

func (s Store) ApplyImportAcceptActuals(changes []app.ImportAcceptActualChange, audit domain.AuditRecord) ([]string, error) {
	writtenSet := map[string]bool{}
	written := []string{}
	note := func(rel string) {
		if !writtenSet[rel] {
			writtenSet[rel] = true
			written = append(written, rel)
		}
	}
	for _, change := range changes {
		evidenceRel, err := s.AppendEvidence(change.Evidence.ObservedAt, change.Evidence)
		if err != nil {
			return written, err
		}
		note(evidenceRel)
		actualRel, err := s.AppendActual(change.Actual.StatusDate, change.Actual)
		if err != nil {
			return written, err
		}
		note(actualRel)
	}
	auditRel, err := s.AppendAudit(audit)
	if err != nil {
		return written, err
	}
	note(auditRel)
	return written, nil
}

func (s Store) ApplyImportAcceptResourceDemands(changes []app.ImportAcceptResourceDemandChange, audit domain.AuditRecord) ([]string, error) {
	err := updateYAML(s, demandsRel, false, func(doc *demandsDoc) error {
		index := demandIndex(doc.ResourceDemands)
		for _, change := range changes {
			if _, exists := index[change.Demand.ID]; exists {
				continue
			}
			doc.ResourceDemands = append(doc.ResourceDemands, change.Demand)
			index[change.Demand.ID] = len(doc.ResourceDemands) - 1
		}
		return nil
	})
	return s.withAudit(demandsRel, err, audit)
}

func (s Store) ApplyImportAcceptResourceLinks(changes []app.ImportAcceptResourceLinkChange, audit domain.AuditRecord) ([]string, error) {
	err := updateYAML(s, resourcesRel, false, func(doc *resourcesDoc) error {
		index := make(map[string]int, len(doc.Resources))
		for i, resource := range doc.Resources {
			index[resource.ID] = i
		}
		for _, change := range changes {
			i, exists := index[change.ResourceID]
			if !exists {
				return fmt.Errorf("resource %s not found", change.ResourceID)
			}
			if !resourceHasSourceRef(doc.Resources[i], change.SourceRef) {
				doc.Resources[i].SourceRefs = append(doc.Resources[i].SourceRefs, change.SourceRef)
			}
		}
		return nil
	})
	return s.withAudit(resourcesRel, err, audit)
}

func (s Store) ApplyImportAcceptDependencies(changes []app.ImportAcceptDependencyChange, audit domain.AuditRecord) ([]string, error) {
	err := updateYAML(s, dependenciesRel, true, func(doc *dependenciesDoc) error {
		index := dependencyIndex(doc.Dependencies)
		for _, change := range changes {
			switch change.Op {
			case "create_dependency", "":
				if _, exists := index[change.Dependency.ID]; exists {
					continue
				}
				doc.Dependencies = append(doc.Dependencies, change.Dependency)
				index[change.Dependency.ID] = len(doc.Dependencies) - 1
			case "update_dependency_source_refs":
				i, exists := index[change.Dependency.ID]
				if !exists {
					return fmt.Errorf("dependency %s not found", change.Dependency.ID)
				}
				if !dependencyHasSourceRef(doc.Dependencies[i], change.SourceRef) {
					doc.Dependencies[i].SourceRefs = append(doc.Dependencies[i].SourceRefs, change.SourceRef)
				}
			default:
				return fmt.Errorf("unsupported import accept dependency change %s", change.Op)
			}
		}
		return nil
	})
	return s.withAudit(dependenciesRel, err, audit)
}

func (s Store) AppendEvidence(observedAt string, evidence domain.Evidence) (string, error) {
	rel := evidencePath(observedAt)
	err := updateYAML(s, rel, true, func(doc *evidenceDoc) error {
		doc.Evidence = append(doc.Evidence, evidence)
		return nil
	})
	return rel, err
}

func (s Store) Load(ctx context.Context) (domain.Workspace, []app.Diagnostic) {
	_ = ctx
	var (
		ws    domain.Workspace
		diags []app.Diagnostic
	)
	info, err := os.Stat(s.projectPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ws, []app.Diagnostic{diag("project_not_found", fmt.Sprintf("project path %s was not found", s.projectPath), "error", "file", s.projectPath)}
		}
		return ws, []app.Diagnostic{diag("file_stat_error", fmt.Sprintf("failed to stat project path %s: %v", s.projectPath, err), "error", "file", s.projectPath)}
	}
	if !info.IsDir() {
		return ws, []app.Diagnostic{diag("project_not_found", fmt.Sprintf("project path %s is not a directory", s.projectPath), "error", "file", s.projectPath)}
	}

	projectData, err := s.fs.read(projectRel)
	if err == nil {
		version, _, diagnostic := parseProjectFormatVersion(projectData)
		if diagnostic != nil {
			return ws, []app.Diagnostic{*diagnostic}
		}
		if err := yaml.Unmarshal(projectData, &struct {
			Project *domain.Project `yaml:"project"`
		}{Project: &ws.Project}); err != nil {
			return ws, []app.Diagnostic{diag("yaml_parse_error", fmt.Sprintf("failed to parse %s: %v", projectRel, err), "error", "file", projectRel)}
		}
		ws.Project.FormatVersion = version
	} else if !errors.Is(err, os.ErrNotExist) {
		return ws, []app.Diagnostic{diag("file_read_error", fmt.Sprintf("failed to read %s: %v", projectRel, err), "error", "file", projectRel)}
	}

	required := []string{
		".replan/project.yaml",
		".replan/policy.yaml",
		"canonical/actors.yaml",
		"canonical/resources.yaml",
		"canonical/resource_pools.yaml",
		"canonical/tasks.yaml",
		"canonical/resource_demands.yaml",
		"canonical/capacities.yaml",
		"canonical/availability.yaml",
		"canonical/current_plan.yaml",
	}

	for _, rel := range required {
		exists, err := s.fs.exists(rel)
		if err != nil {
			diags = append(diags, diag("file_stat_error", fmt.Sprintf("failed to stat %s: %v", rel, err), "error", "file", rel))
			continue
		}
		if !exists {
			diags = append(diags, diag("missing_required_file", fmt.Sprintf("%s is missing", rel), "error", "file", rel))
			continue
		}
		ws.CheckedFiles = append(ws.CheckedFiles, rel)
	}

	var actors struct {
		Actors []domain.Actor `yaml:"actors"`
	}
	readYAML(s, &diags, "canonical/actors.yaml", &actors)
	ws.Actors = actors.Actors

	var resources struct {
		Resources []domain.Resource `yaml:"resources"`
	}
	readYAML(s, &diags, "canonical/resources.yaml", &resources)
	ws.Resources = resources.Resources

	var pools struct {
		ResourcePools []domain.ResourcePool `yaml:"resource_pools"`
	}
	readYAML(s, &diags, "canonical/resource_pools.yaml", &pools)
	ws.ResourcePools = pools.ResourcePools

	var tasks struct {
		Tasks []domain.Task `yaml:"tasks"`
	}
	readYAML(s, &diags, "canonical/tasks.yaml", &tasks)
	ws.Tasks = tasks.Tasks
	if s.bindingsErr != nil {
		diags = append(diags, *s.bindingsErr)
	} else if s.bindings != nil {
		applyOpenProjectBindings(&ws, s.bindings, &diags)
	}

	var demands struct {
		ResourceDemands []domain.ResourceDemand `yaml:"resource_demands"`
	}
	readYAML(s, &diags, "canonical/resource_demands.yaml", &demands)
	ws.ResourceDemands = demands.ResourceDemands

	var dependencies struct {
		Dependencies []domain.Dependency `yaml:"dependencies"`
	}
	readOptionalYAML(s, &diags, "canonical/dependencies.yaml", &dependencies, &ws.CheckedFiles)
	ws.Dependencies = dependencies.Dependencies

	var capacities struct {
		Capacities []domain.Capacity `yaml:"capacities"`
	}
	readYAML(s, &diags, "canonical/capacities.yaml", &capacities)
	ws.Capacities = capacities.Capacities

	var availability struct {
		Availability []domain.Availability `yaml:"availability"`
	}
	readYAML(s, &diags, "canonical/availability.yaml", &availability)
	ws.Availabilities = availability.Availability

	readYAML(s, &diags, "canonical/current_plan.yaml", &struct {
		CurrentPlan *domain.CurrentPlan `yaml:"current_plan"`
	}{CurrentPlan: &ws.CurrentPlan})

	ws.Baselines = readYAMLGlob[domain.Baseline](s, &diags, "baselines/*.yaml", "baseline")
	ws.Calibrations = readYAMLGlob[domain.Calibration](s, &diags, "calibrations/*.yaml", "calibration")
	ws.Actuals = readYAMLGlobList[domain.Actual](s, &diags, "actuals/*.yaml", "actuals")
	ws.Evidence = readYAMLGlobList[domain.Evidence](s, &diags, "evidence/*.yaml", "evidence")
	ws.Proposals = readProposalDirs(s, &diags)
	ws.Scenarios = readYAMLGlob[domain.Scenario](s, &diags, "scenarios/*.yaml", "scenario")
	ws.AuditRecords = readAudit(s, &diags)

	return ws, diags
}

func parseProjectFormatVersion(data []byte) (int, bool, *app.Diagnostic) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		diagnostic := diag("yaml_parse_error", fmt.Sprintf("failed to parse %s: %v", projectRel, err), "error", "file", projectRel)
		return 0, false, &diagnostic
	}
	project := mappingValue(&root, "project")
	value := mappingValue(project, "format_version")
	if value == nil {
		return 1, false, nil
	}
	found := value.Value
	var version int
	err := value.Decode(&version)
	if value.Kind != yaml.ScalarNode || value.Tag != "!!int" || err != nil {
		return 0, true, unsupportedProjectFormatDiagnostic(strconv.Quote(found), true)
	}
	if version < 1 {
		return version, true, unsupportedProjectFormatDiagnostic(strconv.Itoa(version), true)
	}
	if version > domain.CurrentProjectFormatVersion {
		return version, true, unsupportedProjectFormatDiagnostic(strconv.Itoa(version), false)
	}
	return version, true, nil
}

func unsupportedProjectFormatDiagnostic(found string, badValue bool) *app.Diagnostic {
	action := "upgrade replan to a version that supports this project format"
	if badValue {
		action = "fix project.format_version in .replan/project.yaml"
	}
	diagnostic := diag("project_format_unsupported", fmt.Sprintf("project format version %s is unsupported; this binary supports version %d; %s", found, domain.CurrentProjectFormatVersion, action), "error", "file", projectRel)
	return &diagnostic
}

func applyOpenProjectBindings(ws *domain.Workspace, document *openProjectBindingsDoc, diags *[]app.Diagnostic) {
	bindings := make(map[string]domain.SourceRef, len(document.Root.Tasks))
	for _, binding := range document.Root.Tasks {
		if binding.TaskID == "" || binding.SourceRef.ObjectID == "" {
			continue
		}
		ref := binding.SourceRef
		if ref.Source == "" {
			ref.Source = "openproject"
		}
		if ref.ObjectType == "" {
			ref.ObjectType = "work_package"
		}
		bindings[binding.TaskID] = ref
	}
	tasks := make(map[string]bool, len(ws.Tasks))
	for i := range ws.Tasks {
		task := &ws.Tasks[i]
		tasks[task.ID] = true
		refs := make([]domain.SourceRef, 0, len(task.SourceRefs))
		for _, ref := range task.SourceRefs {
			if ref.Source != "openproject" {
				refs = append(refs, ref)
			}
		}
		if binding, ok := bindings[task.ID]; ok {
			refs = append(refs, binding)
		}
		task.SourceRefs = refs
	}
	warned := make(map[string]bool)
	for _, binding := range document.Root.Tasks {
		taskID := binding.TaskID
		if _, applied := bindings[taskID]; applied && !tasks[taskID] && !warned[taskID] {
			*diags = append(*diags, diag("binding_unknown_task", fmt.Sprintf("OpenProject binding references unknown task %s", taskID), "warning", "task", taskID))
			warned[taskID] = true
		}
	}
}

func (s Store) readProposalByID(proposalID string) (string, domain.Proposal, error) {
	for _, status := range []string{"open", "accepted", "rejected", "superseded"} {
		rel := proposalPath(status, proposalID)
		data, err := s.fs.read(rel)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return rel, domain.Proposal{}, fmt.Errorf("read proposal file %s: %w", rel, err)
		}
		var wrapper struct {
			Proposal domain.Proposal `yaml:"proposal"`
		}
		if err := yaml.Unmarshal(data, &wrapper); err != nil {
			return rel, domain.Proposal{}, fmt.Errorf("parse proposal file %s: %w", rel, err)
		}
		if wrapper.Proposal.ID != proposalID {
			return rel, domain.Proposal{}, fmt.Errorf("proposal file %s contains id %s", rel, wrapper.Proposal.ID)
		}
		return rel, wrapper.Proposal, nil
	}
	return "", domain.Proposal{}, fmt.Errorf("proposal %s not found", proposalID)
}

func (s Store) readScenarioByID(scenarioID string) (string, domain.Scenario, error) {
	rel := filepath.ToSlash(filepath.Join("scenarios", scenarioID+".yaml"))
	data, err := s.fs.read(rel)
	if err != nil {
		return rel, domain.Scenario{}, fmt.Errorf("read scenario file %s: %w", rel, err)
	}
	var wrapper struct {
		Scenario domain.Scenario `yaml:"scenario"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return rel, domain.Scenario{}, fmt.Errorf("parse scenario file %s: %w", rel, err)
	}
	if wrapper.Scenario.ID != scenarioID {
		return rel, domain.Scenario{}, fmt.Errorf("scenario file %s contains id %s", rel, wrapper.Scenario.ID)
	}
	return rel, wrapper.Scenario, nil
}

func touchesScenarioTargetKind(changes []app.ScenarioApplyChange, targetKind string) bool {
	for _, change := range changes {
		if change.TargetKind == targetKind {
			return true
		}
	}
	return false
}

func readYAML(s Store, diags *[]app.Diagnostic, rel string, target any) {
	data, err := s.fs.read(rel)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			*diags = append(*diags, diag("file_read_error", fmt.Sprintf("failed to read %s: %v", rel, err), "error", "file", rel))
		}
		return
	}
	if err := yaml.Unmarshal(data, target); err != nil {
		*diags = append(*diags, diag("yaml_parse_error", fmt.Sprintf("failed to parse %s: %v", rel, err), "error", "file", rel))
	}
}

func readOptionalYAML(s Store, diags *[]app.Diagnostic, rel string, target any, checkedFiles *[]string) {
	data, err := s.fs.read(rel)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			*diags = append(*diags, diag("file_read_error", fmt.Sprintf("failed to read %s: %v", rel, err), "error", "file", rel))
		}
		return
	}
	*checkedFiles = append(*checkedFiles, rel)
	if err := yaml.Unmarshal(data, target); err != nil {
		*diags = append(*diags, diag("yaml_parse_error", fmt.Sprintf("failed to parse %s: %v", rel, err), "error", "file", rel))
	}
}

func readYAMLGlob[T any](s Store, diags *[]app.Diagnostic, pattern, key string) []T {
	matches, err := s.fs.glob(pattern)
	if err != nil {
		*diags = append(*diags, diag("invalid_glob", fmt.Sprintf("invalid glob %s: %v", pattern, err), "error", "file", pattern))
		return nil
	}
	var out []T
	for _, match := range matches {
		var wrapper map[string]T
		data, err := s.fs.read(match)
		if err != nil {
			*diags = append(*diags, diag("file_read_error", fmt.Sprintf("failed to read %s: %v", match, err), "error", "file", match))
			continue
		}
		if err := yaml.Unmarshal(data, &wrapper); err != nil {
			*diags = append(*diags, diag("yaml_parse_error", fmt.Sprintf("failed to parse %s: %v", match, err), "error", "file", match))
			continue
		}
		out = append(out, wrapper[key])
	}
	return out
}

func readYAMLGlobList[T any](s Store, diags *[]app.Diagnostic, pattern, key string) []T {
	matches, err := s.fs.glob(pattern)
	if err != nil {
		*diags = append(*diags, diag("invalid_glob", fmt.Sprintf("invalid glob %s: %v", pattern, err), "error", "file", pattern))
		return nil
	}
	var out []T
	for _, match := range matches {
		var wrapper map[string][]T
		data, err := s.fs.read(match)
		if err != nil {
			*diags = append(*diags, diag("file_read_error", fmt.Sprintf("failed to read %s: %v", match, err), "error", "file", match))
			continue
		}
		if err := yaml.Unmarshal(data, &wrapper); err != nil {
			*diags = append(*diags, diag("yaml_parse_error", fmt.Sprintf("failed to parse %s: %v", match, err), "error", "file", match))
			continue
		}
		out = append(out, wrapper[key]...)
	}
	return out
}

func readProposalDirs(s Store, diags *[]app.Diagnostic) []domain.Proposal {
	var out []domain.Proposal
	for _, status := range []string{"open", "accepted", "rejected", "superseded"} {
		out = append(out, readYAMLGlob[domain.Proposal](s, diags, filepath.ToSlash(filepath.Join("proposals", status, "*.yaml")), "proposal")...)
	}
	return out
}

func proposalPath(status string, proposalID string) string {
	return filepath.ToSlash(filepath.Join("proposals", status, proposalID+".yaml"))
}

func readAudit(s Store, diags *[]app.Diagnostic) []domain.AuditRecord {
	matches, err := s.fs.glob("audit/*.jsonl")
	if err != nil {
		*diags = append(*diags, diag("invalid_glob", fmt.Sprintf("invalid audit glob: %v", err), "error", "file", "audit/*.jsonl"))
		return nil
	}
	var records []domain.AuditRecord
	for _, match := range matches {
		data, err := s.fs.read(match)
		if err != nil {
			*diags = append(*diags, diag("file_read_error", fmt.Sprintf("failed to read %s: %v", match, err), "error", "file", match))
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(data))
		scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
		line := 0
		for scanner.Scan() {
			line++
			if scanner.Text() == "" {
				continue
			}
			var record domain.AuditRecord
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
				*diags = append(*diags, diag("json_parse_error", fmt.Sprintf("failed to parse %s line %d: %v", match, line, err), "error", "file", match))
				continue
			}
			records = append(records, record)
		}
		if err := scanner.Err(); err != nil {
			*diags = append(*diags, diag("file_read_error", fmt.Sprintf("failed to scan %s: %v", match, err), "error", "file", match))
		}
	}
	return records
}

func hasSourceRef(task domain.Task, ref domain.SourceRef) bool {
	for _, existing := range task.SourceRefs {
		if existing.Source == ref.Source && existing.ObjectType == ref.ObjectType && existing.ObjectID == ref.ObjectID {
			return true
		}
	}
	return false
}

func resourceHasSourceRef(resource domain.Resource, ref domain.SourceRef) bool {
	for _, existing := range resource.SourceRefs {
		if existing.Source == ref.Source && existing.ObjectType == ref.ObjectType && existing.ObjectID == ref.ObjectID {
			return true
		}
	}
	return false
}

func dependencyHasSourceRef(dependency domain.Dependency, ref domain.SourceRef) bool {
	for _, existing := range dependency.SourceRefs {
		if existing.Source == ref.Source && existing.ObjectType == ref.ObjectType && existing.ObjectID == ref.ObjectID {
			return true
		}
	}
	return false
}

func diag(code, message, severity, kind, id string) app.Diagnostic {
	return app.Diagnostic{
		Code:     code,
		Message:  message,
		Severity: severity,
		Target:   app.Target{Kind: kind, ID: id},
	}
}

func actualsPath(statusDate string) string {
	t, err := time.Parse("2006-01-02", statusDate)
	if err != nil {
		return "actuals/actuals.unscheduled.yaml"
	}
	year, week := t.ISOWeek()
	return fmt.Sprintf("actuals/actuals.%04d-W%02d.yaml", year, week)
}

func evidencePath(observedAt string) string {
	statusDate := observedAt
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

func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create directory for %s: %w", path, err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file for %s: %w", path, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp file for %s: %w", path, err)
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp file for %s: %w", path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file for %s: %w", path, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	cleanup = false
	return nil
}
