package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kogriv/replan/internal/domain"
)

var ErrValidationFailed = errors.New("validation failed")

type WorkspaceLoader interface {
	Load(ctx context.Context) (domain.Workspace, []Diagnostic)
	ProjectPath() string
}

type ValidateService struct {
	loader WorkspaceLoader
}

type ValidateRequest struct {
	ProjectPath       string
	WarningsAsErrors  bool
	IncludeObjectScan bool
}

type ValidateResult struct {
	Valid        bool           `json:"valid"`
	ProjectPath  string         `json:"project_path"`
	CheckedFiles int            `json:"checked_files"`
	ObjectCounts map[string]int `json:"object_counts"`
	Warnings     []Diagnostic   `json:"warnings"`
	Errors       []Diagnostic   `json:"errors"`
}

func NewValidateService(loader WorkspaceLoader) ValidateService {
	return ValidateService{loader: loader}
}

func (s ValidateService) Validate(ctx context.Context, req ValidateRequest) (ValidateResult, error) {
	ws, loadDiags := s.loader.Load(ctx)
	result := ValidateResult{
		Valid:        true,
		ProjectPath:  s.loader.ProjectPath(),
		CheckedFiles: len(ws.CheckedFiles),
		ObjectCounts: map[string]int{
			"actors":           len(ws.Actors),
			"resources":        len(ws.Resources),
			"resource_pools":   len(ws.ResourcePools),
			"tasks":            len(ws.Tasks),
			"resource_demands": len(ws.ResourceDemands),
			"dependencies":     len(ws.Dependencies),
			"capacities":       len(ws.Capacities),
			"availability":     len(ws.Availabilities),
			"baselines":        len(ws.Baselines),
			"calibrations":     len(ws.Calibrations),
			"actuals":          len(ws.Actuals),
			"evidence":         len(ws.Evidence),
			"proposals":        len(ws.Proposals),
			"scenarios":        len(ws.Scenarios),
			"audit_records":    len(ws.AuditRecords),
		},
		Warnings: []Diagnostic{},
		Errors:   []Diagnostic{},
	}

	for _, d := range loadDiags {
		addDiagnostic(&result, d)
	}
	if hasDiagnosticCode(loadDiags, "project_format_unsupported") {
		result.Valid = false
		return result, ErrValidationFailed
	}

	validateWorkspace(&result, ws)

	if req.WarningsAsErrors && len(result.Warnings) > 0 {
		for _, warning := range result.Warnings {
			warning.Severity = "error"
			result.Errors = append(result.Errors, warning)
		}
		result.Warnings = []Diagnostic{}
	}

	result.Valid = len(result.Errors) == 0
	if !result.Valid {
		return result, ErrValidationFailed
	}
	return result, nil
}

func hasDiagnosticCode(diagnostics []Diagnostic, code string) bool {
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
}

// workspaceValidator checks the records of a workspace one kind at a time.
// Kinds are checked in a fixed order and a reference resolves only to a
// record registered before it, so the order of the calls in
// validateWorkspace is part of the diagnostics.
type workspaceValidator struct {
	result *ValidateResult
	ids    map[string]string
}

func validateWorkspace(result *ValidateResult, ws domain.Workspace) {
	v := &workspaceValidator{result: result, ids: map[string]string{}}
	v.project(ws.Project)
	v.actors(ws.Actors)
	v.resources(ws.Resources)
	v.resourcePools(ws.ResourcePools)
	v.tasks(ws.Tasks)
	v.resourceDemands(ws.ResourceDemands)
	v.dependencies(ws.Dependencies)
	v.taskDatesAndRefs(ws.Tasks)
	v.capacities(ws.Capacities)
	v.availabilities(ws.Availabilities)
	v.proposals(ws.Proposals)
	v.actuals(ws.Actuals)
	v.evidence(ws.Evidence)
	v.baselines(ws.Baselines)
	v.calibrations(ws.Calibrations, ws.Baselines)
	v.scenarios(ws.Scenarios)
	v.currentPlan(ws.CurrentPlan)
	v.auditRecords(ws.AuditRecords)
	v.overrunReviews(ws.ResourceDemands)
	for _, diagnostic := range dependencyPlanningDiagnostics(ws) {
		addDiagnostic(result, diagnostic)
	}
	for _, diagnostic := range taskPlanningDiagnostics(ws) {
		addDiagnostic(result, diagnostic)
	}
}

func (v *workspaceValidator) errorf(code, kind, id, format string, args ...any) {
	addError(v.result, code, fmt.Sprintf(format, args...), kind, id)
}

func (v *workspaceValidator) warn(code, message, kind, id string) {
	addWarning(v.result, code, message, kind, id)
}

// register records an id of a kind; a missing or repeated id is an error.
func (v *workspaceValidator) register(kind, id string) {
	if id == "" {
		v.errorf("missing_id", kind, "", "%s id is required", kind)
		return
	}
	key := kind + ":" + id
	if prev, ok := v.ids[key]; ok {
		v.errorf("duplicate_id", kind, id, "duplicate %s id %q already seen in %s", kind, id, prev)
		return
	}
	v.ids[key] = filepath.ToSlash(kind)
}

// resolves reports an unresolved_reference unless ref names a registered
// record.
func (v *workspaceValidator) resolves(ref domain.Ref, kind, id, what string) {
	if !exists(v.ids, ref) {
		v.errorf("unresolved_reference", kind, id, "%s %q does not exist", what, ref.ID)
	}
}

func (v *workspaceValidator) project(project domain.Project) {
	if project.ID == "" {
		v.errorf("missing_project_id", "project", ".replan/project.yaml", "project id is required")
	}
	if project.Name == "" {
		v.errorf("missing_project_name", "project", ".replan/project.yaml", "project name is required")
	}
	if project.Timezone == "" {
		v.warn("missing_project_timezone", "project timezone is recommended", "project", project.ID)
	}
	if policy := project.EffectiveOpenProjectIDs(); policy != domain.OpenProjectIDsCanonical && policy != domain.OpenProjectIDsPrivateBindings {
		v.errorf("invalid_openproject_ids_policy", "project", project.ID, "project openproject_ids must be %q or %q, got %q", domain.OpenProjectIDsCanonical, domain.OpenProjectIDsPrivateBindings, project.OpenProjectIDs)
	}
	validateSourceProfiles(v.result, project.SourceProfiles)
}

func (v *workspaceValidator) actors(actors []domain.Actor) {
	for _, actor := range actors {
		v.register("actor", actor.ID)
		if actor.Kind == "" {
			v.errorf("missing_actor_kind", "actor", actor.ID, "actor kind is required")
		}
		if actor.TrustLevel == "" {
			v.warn("missing_actor_trust_level", "actor trust level is recommended", "actor", actor.ID)
		}
	}
}

func (v *workspaceValidator) resources(resources []domain.Resource) {
	for _, resource := range resources {
		v.register("resource", resource.ID)
		if resource.Class == "" {
			v.errorf("missing_resource_class", "resource", resource.ID, "resource class is required")
		}
		if resource.Name == "" {
			v.errorf("missing_resource_name", "resource", resource.ID, "resource name is required")
		}
		if resource.ActorRef.ID != "" {
			v.resolves(resource.ActorRef, "resource", resource.ID, "resource actor_ref")
		}
	}
}

func (v *workspaceValidator) resourcePools(pools []domain.ResourcePool) {
	for _, pool := range pools {
		v.register("resource_pool", pool.ID)
		if pool.ResourceClass == "" {
			v.errorf("missing_resource_pool_class", "resource_pool", pool.ID, "resource pool resource_class is required")
		}
		for _, member := range pool.Members {
			v.resolves(member, "resource_pool", pool.ID, "resource pool member")
		}
	}
}

func (v *workspaceValidator) tasks(tasks []domain.Task) {
	for _, task := range tasks {
		v.register("task", task.ID)
		if task.Title == "" {
			v.errorf("missing_task_title", "task", task.ID, "task title is required")
		}
		if !domain.ValidTrackingStatus(task.TrackingStatus) {
			v.errorf("invalid_tracking_status", "task", task.ID, "invalid tracking_status %q", task.TrackingStatus)
		}
		if len(task.ResourceDemands) == 0 {
			v.warn("task_without_resource_demands", "task has no resource demands", "task", task.ID)
		}
	}
}

func (v *workspaceValidator) resourceDemands(demands []domain.ResourceDemand) {
	for _, demand := range demands {
		v.resourceDemand(demand)
	}
}

func (v *workspaceValidator) resourceDemand(demand domain.ResourceDemand) {
	const kind = "resource_demand"
	v.register(kind, demand.ID)
	v.resolves(demand.TaskRef, kind, demand.ID, "demand task_ref")
	if demand.Status != "" && !domain.ValidResourceDemandStatus(demand.Status) {
		v.errorf("invalid_resource_demand_status", kind, demand.ID, "invalid resource demand status %q", demand.Status)
	}
	if demand.Unit == "" {
		v.errorf("missing_unit", kind, demand.ID, "resource demand unit is required")
	}
	if !validAmount(demand.PlannedAmount) {
		v.errorf("invalid_planned_amount", kind, demand.ID, "resource demand planned_amount %v must be a finite number, zero or greater", demand.PlannedAmount)
	}
	if demand.RemainingAmount != nil && !validAmount(*demand.RemainingAmount) {
		v.errorf("invalid_remaining_amount", kind, demand.ID, "resource demand remaining_amount %v must be a finite number, zero or greater", *demand.RemainingAmount)
	}
	if demand.ResourceClass == "" {
		v.errorf("missing_resource_class", kind, demand.ID, "resource demand resource_class is required")
	}
	hasResource := demand.ResourceRef.ID != ""
	hasPool := demand.ResourcePoolRef.ID != ""
	if !hasResource && !hasPool {
		v.warn("unresolved_resource", "resource demand has no resource_ref or resource_pool_ref", kind, demand.ID)
	}
	if hasResource {
		v.resolves(demand.ResourceRef, kind, demand.ID, "demand resource_ref")
	}
	if hasPool {
		v.resolves(demand.ResourcePoolRef, kind, demand.ID, "demand resource_pool_ref")
	}
}

// dependencies checks each dependency and that no OpenProject relation or
// predecessor/successor/type triple is used twice.
func (v *workspaceValidator) dependencies(dependencies []domain.Dependency) {
	sourceRefs := map[string]string{}
	triples := map[string]string{}
	for _, dependency := range dependencies {
		v.register("dependency", dependency.ID)
		v.dependencyFields(dependency)
		v.dependencyEnds(dependency)
		v.dependencySourceRefs(dependency, sourceRefs)
		v.dependencyTriple(dependency, triples)
	}
}

func (v *workspaceValidator) dependencyFields(dependency domain.Dependency) {
	const kind = "dependency"
	if !domain.ValidDependencyType(dependency.Type) {
		v.errorf("invalid_dependency_type", kind, dependency.ID, "invalid dependency type %q", dependency.Type)
	}
	if !domain.ValidDependencyStatus(dependency.Status) {
		v.errorf("invalid_dependency_status", kind, dependency.ID, "invalid dependency status %q", dependency.Status)
	}
	if !domain.ValidDependencyCriticality(dependency.Criticality) {
		v.errorf("invalid_dependency_criticality", kind, dependency.ID, "invalid dependency criticality %q", dependency.Criticality)
	}
}

// dependencyEnds checks the successor, the predecessor or the external
// party, the owner and the expected resolution date.
func (v *workspaceValidator) dependencyEnds(dependency domain.Dependency) {
	const kind = "dependency"
	if dependency.SuccessorRef.ID == "" {
		v.errorf("missing_dependency_successor", kind, dependency.ID, "dependency successor_ref is required")
	} else {
		v.resolves(dependency.SuccessorRef, kind, dependency.ID, "dependency successor_ref")
	}
	switch {
	case dependency.Type == "external_dependency":
		if dependency.ExternalRef.ID == "" && dependency.Description == "" && dependency.OwnerRef.ID == "" && dependency.ExpectedResolutionDate == "" {
			v.warn("incomplete_external_dependency", "external dependency should define external_ref, description, owner_ref or expected_resolution_date", kind, dependency.ID)
		}
	case dependency.PredecessorRef.ID == "":
		v.errorf("missing_dependency_predecessor", kind, dependency.ID, "dependency predecessor_ref is required for task-to-task dependency")
	default:
		v.resolves(dependency.PredecessorRef, kind, dependency.ID, "dependency predecessor_ref")
	}
	if dependency.PredecessorRef.Kind == "task" && dependency.SuccessorRef.Kind == "task" && dependency.PredecessorRef.ID == dependency.SuccessorRef.ID {
		v.errorf("self_dependency", kind, dependency.ID, "dependency predecessor_ref and successor_ref must not reference the same task")
	}
	if dependency.OwnerRef.ID != "" {
		v.resolves(dependency.OwnerRef, kind, dependency.ID, "dependency owner_ref")
	}
	if dependency.ExpectedResolutionDate != "" && !validDate(dependency.ExpectedResolutionDate) {
		v.errorf("invalid_dependency_expected_resolution_date", kind, dependency.ID, "dependency expected_resolution_date %q must use YYYY-MM-DD", dependency.ExpectedResolutionDate)
	}
}

func (v *workspaceValidator) dependencySourceRefs(dependency domain.Dependency, seen map[string]string) {
	for _, sourceRef := range dependency.SourceRefs {
		if sourceRef.Source == "" || sourceRef.ObjectType == "" || sourceRef.ObjectID == "" {
			v.errorf("invalid_source_ref", "dependency", dependency.ID, "dependency source_refs require source, object_type and object_id")
			continue
		}
		key := sourceRef.Source + ":" + sourceRef.ObjectType + ":" + sourceRef.ObjectID
		if prev, ok := seen[key]; ok {
			v.errorf("duplicate_dependency_source_ref", "dependency", dependency.ID, "dependency source_ref %s already used by %s", key, prev)
			continue
		}
		seen[key] = dependency.ID
	}
}

func (v *workspaceValidator) dependencyTriple(dependency domain.Dependency, seen map[string]string) {
	if dependency.PredecessorRef.ID == "" || dependency.SuccessorRef.ID == "" {
		return
	}
	key := dependency.Type + ":" + dependency.PredecessorRef.Kind + ":" + dependency.PredecessorRef.ID + ":" + dependency.SuccessorRef.Kind + ":" + dependency.SuccessorRef.ID
	if prev, ok := seen[key]; ok {
		v.errorf("duplicate_dependency_triple", "dependency", dependency.ID, "dependency predecessor/successor/type triple already used by %s", prev)
		return
	}
	seen[key] = dependency.ID
}

// taskDatesAndRefs runs after demands and dependencies are registered, so a
// task's references to them resolve.
func (v *workspaceValidator) taskDatesAndRefs(tasks []domain.Task) {
	for _, task := range tasks {
		if task.PlannedStart != "" && !validDate(task.PlannedStart) {
			v.errorf("invalid_task_planned_start", "task", task.ID, "task planned_start %q must use YYYY-MM-DD", task.PlannedStart)
		}
		if task.PlannedFinish != "" && !validDate(task.PlannedFinish) {
			v.errorf("invalid_task_planned_finish", "task", task.ID, "task planned_finish %q must use YYYY-MM-DD", task.PlannedFinish)
		}
		for _, demandRef := range task.ResourceDemands {
			v.resolves(demandRef, "task", task.ID, "task resource demand")
		}
		for _, dependencyRef := range task.Dependencies {
			v.resolves(dependencyRef, "task", task.ID, "task dependency")
		}
	}
}

func (v *workspaceValidator) capacities(capacities []domain.Capacity) {
	for _, capacity := range capacities {
		v.register("capacity", capacity.ID)
		v.resolves(capacity.ResourceRef, "capacity", capacity.ID, "capacity resource_ref")
		if capacity.Unit == "" {
			v.errorf("missing_unit", "capacity", capacity.ID, "capacity unit is required")
		}
		if capacity.Type == "" {
			v.errorf("missing_capacity_type", "capacity", capacity.ID, "capacity type is required")
		}
		validateCapacityEffectiveDates(v.result, capacity)
	}
}

func (v *workspaceValidator) availabilities(availabilities []domain.Availability) {
	for _, availability := range availabilities {
		v.register("availability", availability.ID)
		v.resolves(availability.ResourceRef, "availability", availability.ID, "availability resource_ref")
	}
}

func (v *workspaceValidator) proposals(proposals []domain.Proposal) {
	for _, proposal := range proposals {
		v.register("proposal", proposal.ID)
		if !domain.ValidProposalStatus(proposal.Status) {
			v.errorf("invalid_proposal_status", "proposal", proposal.ID, "invalid proposal status %q", proposal.Status)
		}
	}
}

func (v *workspaceValidator) scenarios(scenarios []domain.Scenario) {
	for _, scenario := range scenarios {
		v.register("scenario", scenario.ID)
		if !domain.ValidScenarioStatus(scenario.Status) {
			v.errorf("invalid_scenario_status", "scenario", scenario.ID, "invalid scenario status %q", scenario.Status)
		}
		if scenario.CalibrationRef.ID != "" {
			v.resolves(scenario.CalibrationRef, "scenario", scenario.ID, "scenario calibration_ref")
		}
	}
}

func (v *workspaceValidator) calibrations(calibrations []domain.Calibration, baselines []domain.Baseline) {
	baselineSeals := map[string]string{}
	for _, baseline := range baselines {
		if baseline.Seal != nil {
			baselineSeals[baseline.ID] = baseline.Seal.Digest
		}
	}
	for _, calibration := range calibrations {
		v.register("calibration", calibration.ID)
		v.resolves(calibration.ApprovedBy, "calibration", calibration.ID, "calibration approved_by")
		for _, source := range calibration.SourceBaselines {
			v.resolves(domain.Ref{Kind: "baseline", ID: source.ID}, "calibration", calibration.ID, "calibration source baseline")
			if digest, ok := baselineSeals[source.ID]; ok && source.SealSHA256 != digest {
				v.errorf("calibration_source_baseline_changed", "calibration", calibration.ID, "source baseline %s seal no longer matches the calibration record", source.ID)
			}
		}
		if calibration.Result.Count != len(calibration.Sample.Tasks) {
			v.errorf("invalid_calibration_count", "calibration", calibration.ID, "calibration count %d does not match %d sample tasks", calibration.Result.Count, len(calibration.Sample.Tasks))
		}
		if calibration.SealSHA256 == "" {
			v.errorf("missing_calibration_seal", "calibration", calibration.ID, "calibration seal_sha256 is required")
		} else if digest, err := domain.CalibrationDigest(calibration); err != nil || digest != calibration.SealSHA256 {
			v.errorf("calibration_modified", "calibration", calibration.ID, "sealed calibration content does not match its digest")
		}
	}
}

func (v *workspaceValidator) actuals(actuals []domain.Actual) {
	for _, actual := range actuals {
		v.register("actual", actual.ID)
		if actual.AcceptedBy.ID == "" {
			v.errorf("missing_accepted_actor", "actual", actual.ID, "actual accepted_by is required")
		}
		if actual.Unit == "" && actual.Amount != 0 {
			v.errorf("missing_unit", "actual", actual.ID, "actual unit is required when amount is present")
		}
	}
}

func (v *workspaceValidator) evidence(evidence []domain.Evidence) {
	for _, item := range evidence {
		v.register("evidence", item.ID)
		if item.Type == "" {
			v.errorf("missing_evidence_type", "evidence", item.ID, "evidence type is required")
		}
	}
}

func (v *workspaceValidator) baselines(baselines []domain.Baseline) {
	for _, baseline := range baselines {
		v.register("baseline", baseline.ID)
		if baseline.ApprovedBy.ID == "" {
			v.errorf("missing_approved_actor", "baseline", baseline.ID, "baseline approved_by is required")
		} else {
			v.resolves(baseline.ApprovedBy, "baseline", baseline.ID, "baseline approved_by")
		}
		for _, task := range baseline.IncludedTasks {
			v.resolves(task.Ref, "baseline", baseline.ID, "baseline included task")
			startValid := task.PlannedStart == "" || validDate(task.PlannedStart)
			finishValid := task.PlannedFinish == "" || validDate(task.PlannedFinish)
			if !startValid {
				v.errorf("invalid_baseline_date", "baseline", baseline.ID, "included task %s planned_start %q must use YYYY-MM-DD", task.ID, task.PlannedStart)
			}
			if !finishValid {
				v.errorf("invalid_baseline_date", "baseline", baseline.ID, "included task %s planned_finish %q must use YYYY-MM-DD", task.ID, task.PlannedFinish)
			}
			if startValid && finishValid && task.PlannedStart != "" && task.PlannedFinish != "" && task.PlannedStart > task.PlannedFinish {
				v.errorf("invalid_baseline_date_range", "baseline", baseline.ID, "included task %s planned_start %s is after planned_finish %s", task.ID, task.PlannedStart, task.PlannedFinish)
			}
		}
		for _, demand := range baseline.IncludedDemands {
			v.resolves(demand.Ref, "baseline", baseline.ID, "baseline included demand")
			hasEstimate := demand.Task != "" || demand.Resource != "" || demand.Unit != "" || demand.PlannedAmount != nil
			if !hasEstimate {
				continue
			}
			if demand.Task == "" {
				v.errorf("missing_baseline_task", "baseline", baseline.ID, "included demand %s task is required with estimate fields", demand.ID)
			} else {
				v.resolves(domain.Ref{Kind: "task", ID: demand.Task}, "baseline", baseline.ID, "baseline demand task")
			}
			if demand.Resource == "" {
				v.errorf("missing_baseline_resource", "baseline", baseline.ID, "included demand %s resource is required with estimate fields", demand.ID)
			} else if !exists(v.ids, domain.Ref{Kind: "resource", ID: demand.Resource}) && !exists(v.ids, domain.Ref{Kind: "resource_pool", ID: demand.Resource}) {
				v.errorf("unresolved_reference", "baseline", baseline.ID, "baseline demand resource %q does not exist", demand.Resource)
			}
			if demand.Unit == "" {
				v.errorf("missing_baseline_unit", "baseline", baseline.ID, "included demand %s unit is required with estimate fields", demand.ID)
			}
			if demand.PlannedAmount == nil {
				v.errorf("missing_baseline_amount", "baseline", baseline.ID, "included demand %s planned_amount is required with estimate fields", demand.ID)
			} else if !validAmount(*demand.PlannedAmount) {
				v.errorf("invalid_baseline_amount", "baseline", baseline.ID, "included demand %s planned_amount must be a finite number, zero or greater", demand.ID)
			}
		}
		if baseline.Seal != nil {
			if baseline.Seal.Algorithm != "sha256" {
				v.errorf("invalid_baseline_seal", "baseline", baseline.ID, "baseline seal algorithm must be sha256")
			} else if digest, err := domain.BaselineDigest(baseline); err != nil || digest != baseline.Seal.Digest {
				v.errorf("baseline_modified", "baseline", baseline.ID, "sealed baseline content does not match its digest")
			}
		}
	}
}

func (v *workspaceValidator) currentPlan(plan domain.CurrentPlan) {
	if plan.BasedOnBaseline.ID != "" {
		v.resolves(plan.BasedOnBaseline, "current_plan", plan.ID, "current plan based_on_baseline")
	}
}

func (v *workspaceValidator) auditRecords(records []domain.AuditRecord) {
	for _, audit := range records {
		v.register("audit_record", audit.ID)
		if audit.ActorRef.ID == "" {
			v.errorf("missing_audit_actor", "audit_record", audit.ID, "audit actor_ref is required")
		}
		if audit.Operation == "" {
			v.errorf("missing_audit_operation", "audit_record", audit.ID, "audit operation is required")
		}
	}
}

// overrunReviews runs last, when the actors and evidence its reviews name
// are registered.
func (v *workspaceValidator) overrunReviews(demands []domain.ResourceDemand) {
	for _, demand := range demands {
		review := demand.OverrunReview
		if review == nil {
			continue
		}
		if !validAmount(review.AcknowledgedForecast) {
			v.errorf("invalid_overrun_review", "resource_demand", demand.ID, "overrun_review acknowledged_forecast must be a finite number, zero or greater")
		}
		if review.Unit != demand.Unit {
			v.errorf("overrun_review_unit_mismatch", "resource_demand", demand.ID, "overrun_review unit %q differs from demand unit %q", review.Unit, demand.Unit)
		}
		v.resolves(review.ReviewedBy, "resource_demand", demand.ID, "overrun_review reviewed_by")
		for _, ref := range review.EvidenceRefs {
			v.resolves(ref, "resource_demand", demand.ID, "overrun_review evidence")
		}
	}
}

// taskPlanningDiagnostics warns about accepted tasks whose planned finish is
// after the project status date (GAP-028). Such a window is stale: the work is
// done, and once relations are pushed to OpenProject its scheduling moves the
// successors after the stale finish.
func taskPlanningDiagnostics(ws domain.Workspace) []Diagnostic {
	statusDate := ws.Project.DefaultStatusDate
	if statusDate == "" {
		return nil
	}
	var diagnostics []Diagnostic
	for _, task := range ws.Tasks {
		if task.TrackingStatus != "accepted" || task.PlannedFinish == "" || task.PlannedFinish <= statusDate {
			continue
		}
		diagnostics = append(diagnostics, Diagnostic{
			Code: "accepted_task_planned_after_status_date",
			Message: fmt.Sprintf("task %s is accepted but its planned finish %s is after the status date %s; run replan task record-actual-window",
				task.ID, task.PlannedFinish, statusDate),
			Severity: "warning",
			Target:   Target{Kind: "task", ID: task.ID},
		})
	}
	return diagnostics
}

// validAmount accepts finite amounts of zero or more.
func validAmount(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0
}

func dependencyPlanningDiagnostics(ws domain.Workspace) []Diagnostic {
	taskByID := map[string]domain.Task{}
	for _, task := range ws.Tasks {
		taskByID[task.ID] = task
	}
	var diagnostics []Diagnostic
	for _, dependency := range ws.Dependencies {
		if dependency.Criticality == "required" && dependency.Status != "resolved" && dependency.Status != "cancelled" {
			if reason, due := unresolvedDependencyReason(dependency, taskByID, ws.Project.DefaultStatusDate); due {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "required_dependency_unresolved",
					Message:  fmt.Sprintf("required dependency %s is %s%s", dependency.ID, dependency.Status, reason),
					Severity: "warning",
					Target:   Target{Kind: "dependency", ID: dependency.ID},
				})
			}
		}
		switch dependency.Type {
		case "finish_to_start":
			if !dependencyActive(dependency.Status) {
				continue
			}
			predecessor, predecessorOK := taskByID[dependency.PredecessorRef.ID]
			successor, successorOK := taskByID[dependency.SuccessorRef.ID]
			if predecessorOK && predecessor.PlannedFinish == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "dependency_predecessor_finish_unknown",
					Message:  fmt.Sprintf("dependency %s predecessor task %s has no planned_finish", dependency.ID, predecessor.ID),
					Severity: "warning",
					Target:   Target{Kind: "dependency", ID: dependency.ID},
				})
			}
			if successorOK && successor.PlannedStart == "" {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "dependency_successor_start_unknown",
					Message:  fmt.Sprintf("dependency %s successor task %s has no planned_start", dependency.ID, successor.ID),
					Severity: "warning",
					Target:   Target{Kind: "dependency", ID: dependency.ID},
				})
			}
			if predecessorOK && successorOK && predecessor.PlannedFinish != "" && successor.PlannedStart != "" && successor.PlannedStart < predecessor.PlannedFinish {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "dependency_successor_starts_before_predecessor_finish",
					Message:  fmt.Sprintf("dependency %s successor task %s starts before predecessor task %s planned_finish", dependency.ID, successor.ID, predecessor.ID),
					Severity: "warning",
					Target:   Target{Kind: "dependency", ID: dependency.ID},
				})
			}
		case "blocks":
			successor, successorOK := taskByID[dependency.SuccessorRef.ID]
			if successorOK && dependencyActive(dependency.Status) && successorBlockedByOpenDependencyStatus(successor.TrackingStatus) {
				diagnostics = append(diagnostics, Diagnostic{
					Code:     "dependency_blocks_active_successor",
					Message:  fmt.Sprintf("dependency %s is %s while successor task %s is %s", dependency.ID, dependency.Status, successor.ID, successor.TrackingStatus),
					Severity: "warning",
					Target:   Target{Kind: "dependency", ID: dependency.ID},
				})
			}
		}
	}
	return diagnostics
}

// unresolvedDependencyReason decides whether an unresolved required
// dependency is worth a warning now. An open dependency whose successor is a
// task that has not started and is planned after the status date is the
// normal state of a plan made ahead, so it is quiet until then. Any other
// status, a stale record, a started or due successor, or a missing date or
// status date still warns.
func unresolvedDependencyReason(dependency domain.Dependency, taskByID map[string]domain.Task, statusDate string) (string, bool) {
	if predecessorSatisfiesDependency(dependency, taskByID) {
		return ": the predecessor is done, record the dependency as resolved", true
	}
	if dependency.Status != "open" || dependency.SuccessorRef.Kind != "task" {
		return "", true
	}
	successor, ok := taskByID[dependency.SuccessorRef.ID]
	if !ok {
		return "", true
	}
	switch successor.TrackingStatus {
	case "in_progress", "review_ready", "done", "accepted":
		return fmt.Sprintf(" while successor task %s is %s", successor.ID, successor.TrackingStatus), true
	}
	if successor.PlannedStart == "" || statusDate == "" {
		return "", true
	}
	if successor.PlannedStart <= statusDate {
		return fmt.Sprintf(" and successor task %s is planned to start on %s", successor.ID, successor.PlannedStart), true
	}
	return "", false
}

func dependencyActive(status string) bool {
	return status == "open" || status == "blocked" || status == "unknown"
}

func successorBlockedByOpenDependencyStatus(status string) bool {
	return status == "ready" || status == "in_progress" || status == "review_ready"
}

func validateSourceProfiles(result *ValidateResult, profiles []domain.SourceProfile) {
	seen := map[string]bool{}
	for _, profile := range profiles {
		targetID := profile.ID
		if targetID == "" {
			targetID = profile.Source
		}
		if profile.ID == "" {
			addError(result, "missing_source_profile_id", "source profile id is required", "source_profile", targetID)
		} else if seen[profile.ID] {
			addError(result, "duplicate_source_profile_id", fmt.Sprintf("duplicate source profile id %q", profile.ID), "source_profile", profile.ID)
		}
		seen[profile.ID] = true
		if profile.Source == "" {
			addError(result, "missing_source_profile_source", "source profile source is required", "source_profile", targetID)
			continue
		}
		if profile.Source != "openproject" {
			addError(result, "unsupported_source_profile_source", fmt.Sprintf("unsupported source profile source %q", profile.Source), "source_profile", targetID)
			continue
		}
		validateOpenProjectSourceProfile(result, profile, targetID)
	}
}

func validateOpenProjectSourceProfile(result *ValidateResult, profile domain.SourceProfile, targetID string) {
	if profile.BaseURL == "" {
		addError(result, "missing_openproject_base_url", "OpenProject source profile base_url is required", "source_profile", targetID)
	} else if !validHTTPURL(profile.BaseURL) {
		addError(result, "invalid_openproject_base_url", "OpenProject source profile base_url must be an http or https URL", "source_profile", targetID)
	}
	if profile.ProjectID == "" {
		addError(result, "missing_openproject_project_id", "OpenProject source profile project_id is required", "source_profile", targetID)
	}
	hasTokenEnv := profile.TokenEnv != ""
	hasCredentialProfile := profile.CredentialProfileRef != ""
	if !hasTokenEnv && !hasCredentialProfile {
		addWarning(result, "missing_openproject_credential_ref", "OpenProject source profile should reference token_env or credential_profile_ref", "source_profile", targetID)
	}
	if hasTokenEnv && !validEnvName(profile.TokenEnv) {
		addError(result, "invalid_openproject_token_env", "OpenProject source profile token_env must be an environment variable name, not a token value", "source_profile", targetID)
	}
	if strings.Contains(profile.BaseURL, "@") {
		addError(result, "secret_like_openproject_base_url", "OpenProject source profile base_url must not contain embedded credentials", "source_profile", targetID)
	}
}

func validHTTPURL(value string) bool {
	parsed, err := url.Parse(value)
	if err != nil {
		return false
	}
	return parsed.Host != "" && (parsed.Scheme == "http" || parsed.Scheme == "https")
}

func validEnvName(value string) bool {
	return regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`).MatchString(value)
}

func exists(ids map[string]string, ref domain.Ref) bool {
	if ref.Kind == "" || ref.ID == "" {
		return false
	}
	_, ok := ids[ref.Kind+":"+ref.ID]
	return ok
}

func addDiagnostic(result *ValidateResult, d Diagnostic) {
	if d.Severity == "warning" {
		result.Warnings = append(result.Warnings, d)
		return
	}
	result.Errors = append(result.Errors, d)
}

func addError(result *ValidateResult, code, message, kind, id string) {
	result.Errors = append(result.Errors, Diagnostic{
		Code:     code,
		Message:  message,
		Severity: "error",
		Target:   Target{Kind: kind, ID: id},
	})
}

func addWarning(result *ValidateResult, code, message, kind, id string) {
	result.Warnings = append(result.Warnings, Diagnostic{
		Code:     code,
		Message:  message,
		Severity: "warning",
		Target:   Target{Kind: kind, ID: id},
	})
}

func validDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return false
	}
	return parsed.Format("2006-01-02") == value
}

// validateCapacityEffectiveDates rejects effective dates that workload would
// have to ignore: a malformed date, or a range that ends before it starts.
func validateCapacityEffectiveDates(result *ValidateResult, capacity domain.Capacity) {
	valid := true
	for _, field := range []struct{ name, value string }{
		{"effective_from", capacity.EffectiveFrom},
		{"effective_to", capacity.EffectiveTo},
	} {
		if field.value == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", field.value); err != nil {
			addError(result, "invalid_capacity_date", fmt.Sprintf("capacity %s %q is not YYYY-MM-DD", field.name, field.value), "capacity", capacity.ID)
			valid = false
		}
	}
	if valid && capacity.EffectiveFrom != "" && capacity.EffectiveTo != "" && capacity.EffectiveTo < capacity.EffectiveFrom {
		addError(result, "invalid_capacity_range", fmt.Sprintf("capacity effective_to %s is before effective_from %s", capacity.EffectiveTo, capacity.EffectiveFrom), "capacity", capacity.ID)
	}
}
