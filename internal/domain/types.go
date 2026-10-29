package domain

type Ref struct {
	Kind string `json:"kind" yaml:"kind"`
	ID   string `json:"id" yaml:"id"`
}

type SourceRef struct {
	Source     string `json:"source" yaml:"source"`
	ObjectType string `json:"object_type" yaml:"object_type"`
	ObjectID   string `json:"object_id" yaml:"object_id"`
	URL        string `json:"url,omitempty" yaml:"url,omitempty"`
}

type SourceProfile struct {
	ID                   string            `json:"id" yaml:"id"`
	Source               string            `json:"source" yaml:"source"`
	BaseURL              string            `json:"base_url,omitempty" yaml:"base_url,omitempty"`
	ProjectID            string            `json:"project_id,omitempty" yaml:"project_id,omitempty"`
	TokenEnv             string            `json:"token_env,omitempty" yaml:"token_env,omitempty"`
	CredentialProfileRef string            `json:"credential_profile_ref,omitempty" yaml:"credential_profile_ref,omitempty"`
	DefaultQuery         map[string]string `json:"default_query,omitempty" yaml:"default_query,omitempty"`
	Repository           string            `json:"repository,omitempty" yaml:"repository,omitempty"`
	StatusMap            map[string]string `json:"status_map,omitempty" yaml:"status_map,omitempty"`
	StartField           string            `json:"start_field,omitempty" yaml:"start_field,omitempty"`
	FinishField          string            `json:"finish_field,omitempty" yaml:"finish_field,omitempty"`
	EstimateField        string            `json:"estimate_field,omitempty" yaml:"estimate_field,omitempty"`
}

type Project struct {
	ID                string          `json:"id" yaml:"id"`
	Name              string          `json:"name" yaml:"name"`
	FormatVersion     int             `json:"format_version" yaml:"format_version"`
	Timezone          string          `json:"timezone" yaml:"timezone"`
	DefaultStatusDate string          `json:"default_status_date,omitempty" yaml:"default_status_date,omitempty"`
	OpenProjectIDs    string          `json:"openproject_ids,omitempty" yaml:"openproject_ids,omitempty"`
	SourceRefs        []SourceRef     `json:"source_refs,omitempty" yaml:"source_refs,omitempty"`
	SourceProfiles    []SourceProfile `json:"source_profiles,omitempty" yaml:"source_profiles,omitempty"`
	PolicyRef         Ref             `json:"policy_ref,omitempty" yaml:"policy_ref,omitempty"`
}

const (
	CurrentProjectFormatVersion   = 1
	OpenProjectIDsCanonical       = "canonical"
	OpenProjectIDsPrivateBindings = "private_bindings"
)

// EffectiveOpenProjectIDs returns the project policy, preserving canonical
// storage as the default for projects created before the policy was added.
func (p Project) EffectiveOpenProjectIDs() string {
	if p.OpenProjectIDs == "" {
		return OpenProjectIDsCanonical
	}
	return p.OpenProjectIDs
}

type Actor struct {
	ID           string `json:"id" yaml:"id"`
	Kind         string `json:"kind" yaml:"kind"`
	DisplayName  string `json:"display_name" yaml:"display_name"`
	TrustLevel   string `json:"trust_level" yaml:"trust_level"`
	ResourceRef  Ref    `json:"resource_ref,omitempty" yaml:"resource_ref,omitempty"`
	PrincipalRef []Ref  `json:"principal_refs,omitempty" yaml:"principal_refs,omitempty"`
}

type Resource struct {
	ID           string      `json:"id" yaml:"id"`
	Class        string      `json:"class" yaml:"class"`
	Name         string      `json:"name" yaml:"name"`
	Capabilities []string    `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Tags         []string    `json:"tags,omitempty" yaml:"tags,omitempty"`
	ActorRef     Ref         `json:"actor_ref,omitempty" yaml:"actor_ref,omitempty"`
	SourceRefs   []SourceRef `json:"source_refs,omitempty" yaml:"source_refs,omitempty"`
}

type ResourcePool struct {
	ID            string   `json:"id" yaml:"id"`
	Name          string   `json:"name" yaml:"name"`
	ResourceClass string   `json:"resource_class" yaml:"resource_class"`
	Members       []Ref    `json:"members,omitempty" yaml:"members,omitempty"`
	Capabilities  []string `json:"capabilities,omitempty" yaml:"capabilities,omitempty"`
	Unresolved    bool     `json:"unresolved,omitempty" yaml:"unresolved,omitempty"`
}

type Task struct {
	ID              string      `json:"id" yaml:"id"`
	Title           string      `json:"title" yaml:"title"`
	Description     string      `json:"description,omitempty" yaml:"description,omitempty"`
	Type            string      `json:"type,omitempty" yaml:"type,omitempty"`
	TrackingStatus  string      `json:"tracking_status" yaml:"tracking_status"`
	PlannedStart    string      `json:"planned_start,omitempty" yaml:"planned_start,omitempty"`
	PlannedFinish   string      `json:"planned_finish,omitempty" yaml:"planned_finish,omitempty"`
	AssigneeHint    Ref         `json:"assignee_hint,omitempty" yaml:"assignee_hint,omitempty"`
	ResourceDemands []Ref       `json:"resource_demands,omitempty" yaml:"resource_demands,omitempty"`
	Dependencies    []Ref       `json:"dependencies,omitempty" yaml:"dependencies,omitempty"`
	Deadlines       []Ref       `json:"deadlines,omitempty" yaml:"deadlines,omitempty"`
	Tags            []string    `json:"tags,omitempty" yaml:"tags,omitempty"`
	SourceRefs      []SourceRef `json:"source_refs,omitempty" yaml:"source_refs,omitempty"`
}

type ResourceDemand struct {
	ID              string  `json:"id" yaml:"id"`
	TaskRef         Ref     `json:"task_ref" yaml:"task_ref"`
	ResourceRef     Ref     `json:"resource_ref,omitempty" yaml:"resource_ref,omitempty"`
	ResourcePoolRef Ref     `json:"resource_pool_ref,omitempty" yaml:"resource_pool_ref,omitempty"`
	ResourceClass   string  `json:"resource_class" yaml:"resource_class"`
	Role            string  `json:"role" yaml:"role"`
	Unit            string  `json:"unit" yaml:"unit"`
	PlannedAmount   float64 `json:"planned_amount,omitempty" yaml:"planned_amount,omitempty"`
	// RemainingAmount is a pointer so that an explicit zero ("estimated,
	// nothing left") survives a rewrite and stays distinct from "not
	// estimated" (GAP-016). Read it through Remaining and HasRemaining.
	RemainingAmount *float64       `json:"remaining_amount,omitempty" yaml:"remaining_amount,omitempty"`
	ReservedAmount  float64        `json:"reserved_amount,omitempty" yaml:"reserved_amount,omitempty"`
	ActualAmount    float64        `json:"actual_amount,omitempty" yaml:"actual_amount,omitempty"`
	Criticality     string         `json:"criticality,omitempty" yaml:"criticality,omitempty"`
	Status          string         `json:"status,omitempty" yaml:"status,omitempty"`
	Timing          Timing         `json:"timing,omitempty" yaml:"timing,omitempty"`
	OverrunReview   *OverrunReview `json:"overrun_review,omitempty" yaml:"overrun_review,omitempty"`
}

// OverrunReview records that an operator reviewed a forecast above plan and
// accepted it as real. Tracking stays quiet while the forecast does not exceed
// AcknowledgedForecast, and raises the suggestion again when it grows past it.
type OverrunReview struct {
	AcknowledgedForecast float64 `json:"acknowledged_forecast" yaml:"acknowledged_forecast"`
	Unit                 string  `json:"unit" yaml:"unit"`
	Reason               string  `json:"reason" yaml:"reason"`
	ReviewedBy           Ref     `json:"reviewed_by" yaml:"reviewed_by"`
	ReviewedAt           string  `json:"reviewed_at" yaml:"reviewed_at"`
	EvidenceRefs         []Ref   `json:"evidence_refs,omitempty" yaml:"evidence_refs,omitempty"`
}

// Amount returns a pointer to v, for optional amount fields.
func Amount(v float64) *float64 {
	return &v
}

// Remaining returns the remaining amount, or zero when it is not estimated.
func (d ResourceDemand) Remaining() float64 {
	if d.RemainingAmount == nil {
		return 0
	}
	return *d.RemainingAmount
}

// HasRemaining reports whether a remaining amount is recorded, including an
// explicit zero.
func (d ResourceDemand) HasRemaining() bool {
	return d.RemainingAmount != nil
}

type Dependency struct {
	ID                     string      `json:"id" yaml:"id"`
	Type                   string      `json:"type" yaml:"type"`
	PredecessorRef         Ref         `json:"predecessor_ref,omitempty" yaml:"predecessor_ref,omitempty"`
	SuccessorRef           Ref         `json:"successor_ref" yaml:"successor_ref"`
	ExternalRef            Ref         `json:"external_ref,omitempty" yaml:"external_ref,omitempty"`
	Description            string      `json:"description,omitempty" yaml:"description,omitempty"`
	RequiredState          string      `json:"required_state,omitempty" yaml:"required_state,omitempty"`
	Status                 string      `json:"status" yaml:"status"`
	Criticality            string      `json:"criticality" yaml:"criticality"`
	OwnerRef               Ref         `json:"owner_ref,omitempty" yaml:"owner_ref,omitempty"`
	ExpectedResolutionDate string      `json:"expected_resolution_date,omitempty" yaml:"expected_resolution_date,omitempty"`
	SourceRefs             []SourceRef `json:"source_refs,omitempty" yaml:"source_refs,omitempty"`
	Notes                  string      `json:"notes,omitempty" yaml:"notes,omitempty"`
}

type Timing struct {
	EarliestStart   string `json:"earliest_start,omitempty" yaml:"earliest_start,omitempty"`
	LatestFinish    string `json:"latest_finish,omitempty" yaml:"latest_finish,omitempty"`
	PreferredStart  string `json:"preferred_start,omitempty" yaml:"preferred_start,omitempty"`
	PreferredFinish string `json:"preferred_finish,omitempty" yaml:"preferred_finish,omitempty"`
	Spread          string `json:"spread,omitempty" yaml:"spread,omitempty"`
	Timezone        string `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}

type Capacity struct {
	ID            string  `json:"id" yaml:"id"`
	ResourceRef   Ref     `json:"resource_ref" yaml:"resource_ref"`
	ResourceClass string  `json:"resource_class,omitempty" yaml:"resource_class,omitempty"`
	Type          string  `json:"type" yaml:"type"`
	Unit          string  `json:"unit" yaml:"unit"`
	Amount        float64 `json:"amount" yaml:"amount"`
	Period        string  `json:"period,omitempty" yaml:"period,omitempty"`
	EffectiveFrom string  `json:"effective_from,omitempty" yaml:"effective_from,omitempty"`
	EffectiveTo   string  `json:"effective_to,omitempty" yaml:"effective_to,omitempty"`
	Timezone      string  `json:"timezone,omitempty" yaml:"timezone,omitempty"`
}

type Availability struct {
	ID          string                  `json:"id" yaml:"id"`
	ResourceRef Ref                     `json:"resource_ref" yaml:"resource_ref"`
	Timezone    string                  `json:"timezone,omitempty" yaml:"timezone,omitempty"`
	Windows     []AvailabilityWindow    `json:"windows,omitempty" yaml:"windows,omitempty"`
	Exceptions  []AvailabilityException `json:"exceptions,omitempty" yaml:"exceptions,omitempty"`
}

type AvailabilityWindow struct {
	Days      []string `json:"days,omitempty" yaml:"days,omitempty"`
	StartTime string   `json:"start_time,omitempty" yaml:"start_time,omitempty"`
	EndTime   string   `json:"end_time,omitempty" yaml:"end_time,omitempty"`
}

type AvailabilityException struct {
	Date   string `json:"date" yaml:"date"`
	Type   string `json:"type" yaml:"type"`
	Reason string `json:"reason,omitempty" yaml:"reason,omitempty"`
}

type CurrentPlan struct {
	ID              string `json:"id" yaml:"id"`
	Version         int    `json:"version" yaml:"version"`
	UpdatedAt       string `json:"updated_at,omitempty" yaml:"updated_at,omitempty"`
	UpdatedBy       Ref    `json:"updated_by,omitempty" yaml:"updated_by,omitempty"`
	BasedOnBaseline Ref    `json:"based_on_baseline,omitempty" yaml:"based_on_baseline,omitempty"`
	IncludedTasks   []Ref  `json:"included_tasks,omitempty" yaml:"included_tasks,omitempty"`
}

type Baseline struct {
	ID              string                `json:"id" yaml:"id"`
	Name            string                `json:"name,omitempty" yaml:"name,omitempty"`
	CreatedAt       string                `json:"created_at" yaml:"created_at"`
	ApprovedAt      string                `json:"approved_at,omitempty" yaml:"approved_at,omitempty"`
	ApprovedBy      Ref                   `json:"approved_by" yaml:"approved_by"`
	StatusDate      string                `json:"status_date,omitempty" yaml:"status_date,omitempty"`
	IncludedTasks   []BaselineTaskEntry   `json:"included_tasks,omitempty" yaml:"included_tasks,omitempty"`
	IncludedDemands []BaselineDemandEntry `json:"included_demands,omitempty" yaml:"included_demands,omitempty"`
	Assumptions     []string              `json:"assumptions,omitempty" yaml:"assumptions,omitempty"`
	Seal            *BaselineSeal         `json:"seal,omitempty" yaml:"seal,omitempty"`
}

type BaselineTaskEntry struct {
	Ref           `yaml:",inline"`
	PlannedStart  string `json:"planned_start,omitempty" yaml:"planned_start,omitempty"`
	PlannedFinish string `json:"planned_finish,omitempty" yaml:"planned_finish,omitempty"`
}

type BaselineDemandEntry struct {
	Ref           `yaml:",inline"`
	Task          string   `json:"task,omitempty" yaml:"task,omitempty"`
	Resource      string   `json:"resource,omitempty" yaml:"resource,omitempty"`
	Unit          string   `json:"unit,omitempty" yaml:"unit,omitempty"`
	PlannedAmount *float64 `json:"planned_amount,omitempty" yaml:"planned_amount,omitempty"`
}

type BaselineSeal struct {
	Algorithm string `json:"algorithm" yaml:"algorithm"`
	Digest    string `json:"digest" yaml:"digest"`
}

type Calibration struct {
	ID              string                      `json:"id" yaml:"id"`
	Name            string                      `json:"name" yaml:"name"`
	ApprovedBy      Ref                         `json:"approved_by" yaml:"approved_by"`
	ApprovedAt      string                      `json:"approved_at" yaml:"approved_at"`
	SourceBaselines []CalibrationSourceBaseline `json:"source_baselines" yaml:"source_baselines"`
	Scope           CalibrationScope            `json:"scope" yaml:"scope"`
	Sample          CalibrationSample           `json:"sample" yaml:"sample"`
	Result          CalibrationResult           `json:"result" yaml:"result"`
	SealSHA256      string                      `json:"seal_sha256" yaml:"seal_sha256"`
}

type CalibrationSourceBaseline struct {
	ID         string `json:"id" yaml:"id"`
	SealSHA256 string `json:"seal_sha256" yaml:"seal_sha256"`
}

type CalibrationScope struct {
	Resources []string `json:"resources" yaml:"resources"`
	Unit      string   `json:"unit" yaml:"unit"`
	Tag       string   `json:"tag,omitempty" yaml:"tag,omitempty"`
}

type CalibrationSample struct {
	Tasks    []string               `json:"tasks" yaml:"tasks"`
	Excluded []CalibrationExclusion `json:"excluded" yaml:"excluded"`
}

type CalibrationExclusion struct {
	Task   string `json:"task" yaml:"task"`
	Reason string `json:"reason" yaml:"reason"`
}

type CalibrationResult struct {
	Factor     float64               `json:"factor" yaml:"factor"`
	TotalRatio float64               `json:"total_ratio" yaml:"total_ratio"`
	P25        float64               `json:"p25" yaml:"p25"`
	P75        float64               `json:"p75" yaml:"p75"`
	Count      int                   `json:"count" yaml:"count"`
	SizeBands  []CalibrationSizeBand `json:"size_bands" yaml:"size_bands"`
}

type CalibrationSizeBand struct {
	UpTo   *float64 `json:"up_to,omitempty" yaml:"up_to,omitempty"`
	Count  int      `json:"count" yaml:"count"`
	Median float64  `json:"median" yaml:"median"`
}

type Actual struct {
	ID           string  `json:"id" yaml:"id"`
	Type         string  `json:"type" yaml:"type"`
	TaskRef      Ref     `json:"task_ref,omitempty" yaml:"task_ref,omitempty"`
	DemandRef    Ref     `json:"demand_ref,omitempty" yaml:"demand_ref,omitempty"`
	ResourceRef  Ref     `json:"resource_ref,omitempty" yaml:"resource_ref,omitempty"`
	Amount       float64 `json:"amount,omitempty" yaml:"amount,omitempty"`
	Unit         string  `json:"unit,omitempty" yaml:"unit,omitempty"`
	OccurredAt   string  `json:"occurred_at,omitempty" yaml:"occurred_at,omitempty"`
	StatusDate   string  `json:"status_date,omitempty" yaml:"status_date,omitempty"`
	AcceptedBy   Ref     `json:"accepted_by" yaml:"accepted_by"`
	EvidenceRefs []Ref   `json:"evidence_refs,omitempty" yaml:"evidence_refs,omitempty"`
}

type Evidence struct {
	ID         string    `json:"id" yaml:"id"`
	Type       string    `json:"type" yaml:"type"`
	Subtype    string    `json:"subtype,omitempty" yaml:"subtype,omitempty"`
	Source     string    `json:"source,omitempty" yaml:"source,omitempty"`
	SourceRef  SourceRef `json:"source_ref,omitempty" yaml:"source_ref,omitempty"`
	URL        string    `json:"url,omitempty" yaml:"url,omitempty"`
	ActorRef   Ref       `json:"actor_ref,omitempty" yaml:"actor_ref,omitempty"`
	ObservedAt string    `json:"observed_at,omitempty" yaml:"observed_at,omitempty"`
	Confidence string    `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	Summary    string    `json:"summary,omitempty" yaml:"summary,omitempty"`
	Targets    []Ref     `json:"targets,omitempty" yaml:"targets,omitempty"`
}

type Proposal struct {
	ID                   string          `json:"id" yaml:"id"`
	Type                 string          `json:"type" yaml:"type"`
	Status               string          `json:"status" yaml:"status"`
	CreatedAt            string          `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	CreatedBy            Ref             `json:"created_by,omitempty" yaml:"created_by,omitempty"`
	TargetRef            Ref             `json:"target_ref,omitempty" yaml:"target_ref,omitempty"`
	SuggestedChange      SuggestedChange `json:"suggested_change,omitempty" yaml:"suggested_change,omitempty"`
	EvidenceRefs         []Ref           `json:"evidence_refs,omitempty" yaml:"evidence_refs,omitempty"`
	Rationale            string          `json:"rationale,omitempty" yaml:"rationale,omitempty"`
	RequiresConfirmation bool            `json:"requires_confirmation,omitempty" yaml:"requires_confirmation,omitempty"`
}

type SuggestedChange struct {
	Op    string `json:"op" yaml:"op"`
	Path  string `json:"path" yaml:"path"`
	Value any    `json:"value,omitempty" yaml:"value,omitempty"`
}

type Scenario struct {
	ID             string           `json:"id" yaml:"id"`
	Name           string           `json:"name" yaml:"name"`
	Status         string           `json:"status" yaml:"status"`
	BaseRef        Ref              `json:"base_ref" yaml:"base_ref"`
	CreatedAt      string           `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	CreatedBy      Ref              `json:"created_by,omitempty" yaml:"created_by,omitempty"`
	CalibrationRef Ref              `json:"calibration_ref,omitempty" yaml:"calibration_ref,omitempty"`
	Changes        []ScenarioChange `json:"changes,omitempty" yaml:"changes,omitempty"`
}

type ScenarioChange struct {
	Op    string `json:"op" yaml:"op"`
	Path  string `json:"path" yaml:"path"`
	Value any    `json:"value,omitempty" yaml:"value,omitempty"`
}

type AuditRecord struct {
	ID                 string `json:"id" yaml:"id"`
	OccurredAt         string `json:"occurred_at" yaml:"occurred_at"`
	ActorRef           Ref    `json:"actor_ref" yaml:"actor_ref"`
	Operation          string `json:"operation" yaml:"operation"`
	TargetRef          Ref    `json:"target_ref,omitempty" yaml:"target_ref,omitempty"`
	NewStatus          string `json:"new_status,omitempty" yaml:"new_status,omitempty"`
	StatusDate         string `json:"status_date,omitempty" yaml:"status_date,omitempty"`
	ActualStart        string `json:"actual_start,omitempty" yaml:"actual_start,omitempty"`
	ActualFinish       string `json:"actual_finish,omitempty" yaml:"actual_finish,omitempty"`
	ActualStartSource  string `json:"actual_start_source,omitempty" yaml:"actual_start_source,omitempty"`
	ActualFinishSource string `json:"actual_finish_source,omitempty" yaml:"actual_finish_source,omitempty"`
	Result             string `json:"result,omitempty" yaml:"result,omitempty"`
	Reason             string `json:"reason,omitempty" yaml:"reason,omitempty"`
	EvidenceRefs       []Ref  `json:"evidence_refs,omitempty" yaml:"evidence_refs,omitempty"`
}

type Workspace struct {
	Project         Project
	Actors          []Actor
	Resources       []Resource
	ResourcePools   []ResourcePool
	Tasks           []Task
	ResourceDemands []ResourceDemand
	Dependencies    []Dependency
	Capacities      []Capacity
	Availabilities  []Availability
	CurrentPlan     CurrentPlan
	Baselines       []Baseline
	Calibrations    []Calibration
	Actuals         []Actual
	Evidence        []Evidence
	Proposals       []Proposal
	Scenarios       []Scenario
	AuditRecords    []AuditRecord
	CheckedFiles    []string
}
