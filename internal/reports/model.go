package reports

type WeeklyReport struct {
	ProjectPath string        `json:"project_path"`
	ProjectID   string        `json:"project_id"`
	ProjectName string        `json:"project_name"`
	Week        string        `json:"week"`
	ScenarioID  string        `json:"scenario_id,omitempty"`
	BaselineID  string        `json:"baseline_id,omitempty"`
	CurrentPlan string        `json:"current_plan,omitempty"`
	Summary     WeeklySummary `json:"summary"`
	Resources   []ResourceRow `json:"resources"`
	Proposals   []ProposalRow `json:"proposals"`
	Evidence    []EvidenceRow `json:"evidence"`
	Warnings    []ReportIssue `json:"warnings"`
	Errors      []ReportIssue `json:"errors"`
}

type WeeklySummary struct {
	ResourceRows  int `json:"resource_rows"`
	Overloaded    int `json:"overloaded"`
	WarningCount  int `json:"warning_count"`
	ErrorCount    int `json:"error_count"`
	OpenProposals int `json:"open_proposals"`
	EvidenceCount int `json:"evidence_count"`
}

type ResourceRow struct {
	ResourceID    string  `json:"resource_id"`
	ResourceName  string  `json:"resource_name,omitempty"`
	ResourceClass string  `json:"resource_class,omitempty"`
	Unit          string  `json:"unit"`
	Capacity      float64 `json:"capacity"`
	Planned       float64 `json:"planned"`
	Reserved      float64 `json:"reserved"`
	Actual        float64 `json:"actual"`
	Remaining     float64 `json:"remaining"`
	Overload      float64 `json:"overload"`
}

type ProposalRow struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Status    string `json:"status"`
	TargetID  string `json:"target_id,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

type EvidenceRow struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Subtype string `json:"subtype,omitempty"`
	Summary string `json:"summary,omitempty"`
}

type ReportIssue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	Severity string `json:"severity"`
	TargetID string `json:"target_id,omitempty"`
}

type DashboardOverview struct {
	SchemaVersion      int                         `json:"schema_version"`
	GeneratedAt        string                      `json:"generated_at,omitempty"`
	ProjectPath        string                      `json:"project_path"`
	ProjectID          string                      `json:"project_id"`
	ProjectName        string                      `json:"project_name"`
	StatusDate         string                      `json:"status_date,omitempty"`
	Week               string                      `json:"week,omitempty"`
	ScenarioID         string                      `json:"scenario_id,omitempty"`
	CurrentPlan        string                      `json:"current_plan,omitempty"`
	BaselineID         string                      `json:"baseline_id,omitempty"`
	SourceHealth       DashboardSourceHealth       `json:"source_health"`
	Health             DashboardHealthSummary      `json:"health"`
	OpenProject        DashboardOpenProject        `json:"openproject,omitempty"`
	OpenProjectOverlay DashboardOpenProjectOverlay `json:"openproject_overlay,omitempty"`
	Workload           []ResourceRow               `json:"workload"`
	Tasks              []DashboardTaskRow          `json:"tasks"`
	Tracking           []DashboardTrackingRow      `json:"tracking,omitempty"`
	TrackingLoop       DashboardTrackingLoop       `json:"tracking_proposal_loop,omitempty"`
	Gantt              DashboardGanttView          `json:"gantt"`
	Dependencies       []DashboardDependency       `json:"dependencies"`
	Warnings           []ReportIssue               `json:"warnings"`
	Errors             []ReportIssue               `json:"errors"`
	Proposals          []ProposalRow               `json:"proposals"`
	Evidence           []EvidenceRow               `json:"evidence"`
	Scenarios          []DashboardScenarioRow      `json:"scenarios"`
	NextActions        []DashboardNextAction       `json:"next_actions"`
	WorkloadPage       DashboardWorkloadPage       `json:"-"`
	ScenariosPage      DashboardScenariosPage      `json:"-"`
	ProposalsPage      DashboardProposalsPage      `json:"-"`
	EstimatesPage      DashboardEstimatesPage      `json:"-"`
}

// DashboardPageMetadata is repeated in every page-specific raw artifact so
// each file remains useful when copied away from the rest of the dashboard.
type DashboardPageMetadata struct {
	SchemaVersion      int           `json:"schema_version"`
	GeneratedAt        string        `json:"generated_at"`
	ProjectID          string        `json:"project_id"`
	ProjectName        string        `json:"project_name"`
	StatusDate         string        `json:"status_date,omitempty"`
	SourceSnapshotRefs []string      `json:"source_snapshot_refs"`
	Warnings           []ReportIssue `json:"warnings"`
}

type DashboardWorkloadPage struct {
	DashboardPageMetadata
	Data DashboardWorkloadData `json:"data"`
}

type DashboardWorkloadData struct {
	ScenarioID   string                      `json:"scenario_id,omitempty"`
	WindowStart  string                      `json:"window_start"`
	WindowFinish string                      `json:"window_finish"`
	Weeks        []string                    `json:"weeks"`
	Resources    []DashboardWorkloadResource `json:"resources"`
}

type DashboardWorkloadResource struct {
	ResourceID    string                  `json:"resource_id"`
	ResourceName  string                  `json:"resource_name,omitempty"`
	ResourceClass string                  `json:"resource_class,omitempty"`
	Unit          string                  `json:"unit"`
	Weeks         []DashboardWorkloadCell `json:"weeks"`
}

type DashboardWorkloadCell struct {
	Week        string                    `json:"week"`
	Capacity    float64                   `json:"capacity"`
	Planned     float64                   `json:"planned"`
	Reserved    float64                   `json:"reserved"`
	Actual      float64                   `json:"actual"`
	Remaining   float64                   `json:"remaining"`
	Overload    float64                   `json:"overload"`
	Shortage    float64                   `json:"shortage"`
	Utilization *float64                  `json:"utilization"`
	Demands     []DashboardWorkloadDemand `json:"demands"`
}

type DashboardWorkloadDemand struct {
	ResourceDemandID string  `json:"resource_demand_id"`
	TaskID           string  `json:"task_id,omitempty"`
	TaskTitle        string  `json:"task_title,omitempty"`
	Status           string  `json:"status,omitempty"`
	Concurrent       bool    `json:"concurrent,omitempty"`
	Share            float64 `json:"share"`
	Planned          float64 `json:"planned"`
	Reserved         float64 `json:"reserved"`
	Remaining        float64 `json:"remaining"`
}

type DashboardScenariosPage struct {
	DashboardPageMetadata
	Data DashboardScenariosData `json:"data"`
}

type DashboardScenariosData struct {
	Scenarios []DashboardScenarioDetail `json:"scenarios"`
}

type DashboardScenarioDetail struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	Status            string                    `json:"status"`
	CreatedAt         string                    `json:"created_at,omitempty"`
	CreatedBy         string                    `json:"created_by,omitempty"`
	CalibrationRef    string                    `json:"calibration_ref,omitempty"`
	Changes           []DashboardScenarioChange `json:"changes"`
	AffectedResources []string                  `json:"affected_resources"`
	AffectedTasks     []string                  `json:"affected_tasks"`
	WorkloadBefore    []ResourceRow             `json:"workload_before"`
	WorkloadAfter     []ResourceRow             `json:"workload_after"`
	Warnings          []ReportIssue             `json:"warnings"`
	Errors            []ReportIssue             `json:"errors"`
	ApplyCommand      string                    `json:"apply_command"`
}

type DashboardScenarioChange struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}

type DashboardProposalsPage struct {
	DashboardPageMetadata
	Data DashboardProposalsData `json:"data"`
}

type DashboardProposalsData struct {
	Proposals []DashboardProposalDetail `json:"proposals"`
}

type DashboardProposalDetail struct {
	ID                   string                      `json:"id"`
	Type                 string                      `json:"type"`
	Status               string                      `json:"status"`
	CreatedAt            string                      `json:"created_at,omitempty"`
	CreatedBy            string                      `json:"created_by,omitempty"`
	TargetKind           string                      `json:"target_kind,omitempty"`
	TargetID             string                      `json:"target_id,omitempty"`
	Rationale            string                      `json:"rationale,omitempty"`
	RequiresConfirmation bool                        `json:"requires_confirmation"`
	Impact               DashboardProposalImpact     `json:"impact"`
	Evidence             []DashboardProposalEvidence `json:"evidence"`
	AcceptCommand        string                      `json:"accept_command"`
	RejectCommand        string                      `json:"reject_command"`
}

type DashboardProposalImpact struct {
	Op    string `json:"op,omitempty"`
	Path  string `json:"path,omitempty"`
	Value any    `json:"value,omitempty"`
}

type DashboardProposalEvidence struct {
	ID         string `json:"id"`
	Type       string `json:"type"`
	Subtype    string `json:"subtype,omitempty"`
	Summary    string `json:"summary,omitempty"`
	ObservedAt string `json:"observed_at,omitempty"`
}

type DashboardEstimatesPage struct {
	DashboardPageMetadata
	Data DashboardEstimatesData `json:"data"`
}

type DashboardEstimatesData struct {
	Baselines []DashboardEstimateBaseline `json:"baselines"`
}

type DashboardEstimateBaseline struct {
	ID           string                         `json:"id"`
	Name         string                         `json:"name,omitempty"`
	StatusDate   string                         `json:"status_date,omitempty"`
	SealStatus   string                         `json:"seal_status"`
	Resources    []DashboardEstimateResource    `json:"resources"`
	SplitGroups  []DashboardEstimateSplitGroup  `json:"split_groups"`
	Tasks        []DashboardEstimateTask        `json:"tasks"`
	Calibrations []DashboardEstimateCalibration `json:"calibrations"`
	Warnings     []ReportIssue                  `json:"warnings"`
	Errors       []ReportIssue                  `json:"errors"`
}

type DashboardEstimateResource struct {
	ResourceID         string   `json:"resource_id"`
	ResourceName       string   `json:"resource_name,omitempty"`
	Unit               string   `json:"unit"`
	ActualsRecorded    bool     `json:"actuals_recorded"`
	BaselineAmount     float64  `json:"baseline_amount"`
	ActualAmount       float64  `json:"actual_amount"`
	Ratio              *float64 `json:"actual_baseline_ratio"`
	AddedAfterBaseline bool     `json:"added_after_baseline,omitempty"`
}

type DashboardEstimateSplitGroup struct {
	OriginalResourceID   string   `json:"original_resource_id"`
	OriginalResourceName string   `json:"original_resource_name,omitempty"`
	SplitResourceIDs     []string `json:"split_resource_ids"`
	Unit                 string   `json:"unit"`
	ActualsRecorded      bool     `json:"actuals_recorded"`
	BaselineAmount       float64  `json:"baseline_amount"`
	CurrentPlannedAmount float64  `json:"current_planned_amount"`
	ActualAmount         float64  `json:"actual_amount"`
	Ratio                *float64 `json:"actual_baseline_ratio"`
}

type DashboardEstimateTask struct {
	TaskID         string                   `json:"task_id"`
	Title          string                   `json:"title,omitempty"`
	TrackingStatus string                   `json:"tracking_status,omitempty"`
	Open           bool                     `json:"open"`
	Totals         []DashboardEstimateTotal `json:"totals"`
}

type DashboardEstimateTotal struct {
	Unit           string   `json:"unit"`
	BaselineAmount float64  `json:"baseline_amount"`
	ActualAmount   float64  `json:"actual_amount"`
	VarianceAmount float64  `json:"variance_amount"`
	Ratio          *float64 `json:"actual_baseline_ratio"`
}

type DashboardEstimateCalibration struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Resources  []string `json:"resources"`
	Unit       string   `json:"unit"`
	Factor     float64  `json:"factor"`
	TotalRatio float64  `json:"total_ratio"`
	Count      int      `json:"count"`
}

type DashboardHealthSummary struct {
	TaskCount              int `json:"task_count"`
	AcceptedTasks          int `json:"accepted_tasks"`
	ActiveTasks            int `json:"active_tasks"`
	OpenDependencies       int `json:"open_dependencies"`
	ResolvedDependencies   int `json:"resolved_dependencies"`
	RelatedLinks           int `json:"related_links"`
	OverloadedResources    int `json:"overloaded_resources"`
	WarningCount           int `json:"warning_count"`
	ErrorCount             int `json:"error_count"`
	OpenProposals          int `json:"open_proposals"`
	EvidenceCount          int `json:"evidence_count"`
	ScenarioCount          int `json:"scenario_count"`
	MissingDemandTaskCount int `json:"missing_demand_task_count"`
}

type DashboardSourceHealth struct {
	Profiles                   []DashboardSourceProfile `json:"profiles"`
	SourceRefs                 int                      `json:"source_refs"`
	TasksWithSourceRefs        int                      `json:"tasks_with_source_refs"`
	TasksWithoutSourceRefs     int                      `json:"tasks_without_source_refs"`
	ResourcesWithSourceRefs    int                      `json:"resources_with_source_refs"`
	DependenciesWithSourceRefs int                      `json:"dependencies_with_source_refs"`
	LinkedTasks                []DashboardSourceTask    `json:"linked_tasks"`
	UnlinkedTasks              []DashboardSourceTask    `json:"unlinked_tasks"`
}

type DashboardSourceProfile struct {
	ID        string `json:"id"`
	Source    string `json:"source"`
	BaseURL   string `json:"base_url,omitempty"`
	ProjectID string `json:"project_id,omitempty"`
}

type DashboardSourceTask struct {
	TaskID         string `json:"task_id"`
	Title          string `json:"title"`
	Source         string `json:"source,omitempty"`
	SourceID       string `json:"source_id,omitempty"`
	SourceURL      string `json:"source_url,omitempty"`
	TrackingStatus string `json:"tracking_status,omitempty"`
}

type DashboardOpenProject struct {
	Enabled                bool                          `json:"enabled"`
	ProjectID              string                        `json:"project_id,omitempty"`
	TaskCount              int                           `json:"task_count"`
	LinkedTaskCount        int                           `json:"linked_task_count"`
	MissingLinkCount       int                           `json:"missing_link_count"`
	WorkPackageURLCount    int                           `json:"work_package_url_count"`
	TasksWithResourceCount int                           `json:"tasks_with_resource_count"`
	Links                  []DashboardOpenProjectLink    `json:"links,omitempty"`
	MissingLinks           []DashboardOpenProjectMissing `json:"missing_links,omitempty"`
}

type DashboardOpenProjectLink struct {
	TaskID              string   `json:"task_id"`
	Title               string   `json:"title"`
	TrackingStatus      string   `json:"tracking_status,omitempty"`
	OpenProjectID       string   `json:"openproject_id"`
	OpenProjectURL      string   `json:"openproject_url,omitempty"`
	PlannedStart        string   `json:"planned_start,omitempty"`
	PlannedFinish       string   `json:"planned_finish,omitempty"`
	ResourceIDs         []string `json:"resource_ids,omitempty"`
	ResourceDemandCount int      `json:"resource_demand_count"`
	WarningCount        int      `json:"warning_count"`
	OpenDependencyCount int      `json:"open_dependency_count"`
}

type DashboardOpenProjectMissing struct {
	TaskID         string `json:"task_id"`
	Title          string `json:"title"`
	TrackingStatus string `json:"tracking_status,omitempty"`
}

type DashboardOpenProjectOverlay struct {
	SchemaVersion        int                                      `json:"schema_version"`
	GeneratedAt          string                                   `json:"generated_at,omitempty"`
	ProjectID            string                                   `json:"project_id"`
	ProjectName          string                                   `json:"project_name"`
	StatusDate           string                                   `json:"status_date,omitempty"`
	Week                 string                                   `json:"week,omitempty"`
	ScenarioID           string                                   `json:"scenario_id,omitempty"`
	OpenProjectProjectID string                                   `json:"openproject_project_id,omitempty"`
	ArtifactRefs         DashboardOpenProjectOverlayArtifacts     `json:"artifact_refs"`
	WorkPackageCount     int                                      `json:"work_package_count"`
	LinkedTaskCount      int                                      `json:"linked_task_count"`
	MissingLinkCount     int                                      `json:"missing_link_count"`
	WorkPackages         []DashboardOpenProjectOverlayWorkPackage `json:"work_packages"`
	MissingLinks         []DashboardOpenProjectMissing            `json:"missing_links,omitempty"`

	// Version 2: what the redesigned plugin views draw, all derived by
	// RePlan so that the plugin does no planning arithmetic.
	Span              DashboardOpenProjectOverlaySpan           `json:"span"`
	WarningCount      int                                       `json:"warning_count"`
	OpenProposalCount int                                       `json:"open_proposal_count"`
	PersonActions     []DashboardOpenProjectOverlayAction       `json:"person_actions"`
	Resources         []DashboardOpenProjectOverlayResource     `json:"resources"`
	OverloadedWeeks   []DashboardOpenProjectOverlayOverloadWeek `json:"overloaded_weeks"`
}

// DashboardOpenProjectOverlaySpan is the plan's date axis.
type DashboardOpenProjectOverlaySpan struct {
	StartDate        string `json:"start_date,omitempty"`
	FinishDate       string `json:"finish_date,omitempty"`
	DayCount         int    `json:"day_count"`
	StatusOffsetDays int    `json:"status_offset_days"`
	StatusInRange    bool   `json:"status_in_range"`
}

// DashboardOpenProjectOverlayAction is a next action waiting on a person.
type DashboardOpenProjectOverlayAction struct {
	Code          string `json:"code"`
	Label         string `json:"label"`
	TaskID        string `json:"task_id,omitempty"`
	WorkPackageID string `json:"work_package_id,omitempty"`
	Date          string `json:"date,omitempty"`
}

// DashboardOpenProjectOverlayResource is one resource and unit the plan
// uses, with its planned load and capacity by ISO week.
type DashboardOpenProjectOverlayResource struct {
	ID              string                                  `json:"id"`
	Name            string                                  `json:"name"`
	Class           string                                  `json:"class,omitempty"`
	Unit            string                                  `json:"unit,omitempty"`
	Concurrent      bool                                    `json:"concurrent,omitempty"`
	WorkingWeekdays []string                                `json:"working_weekdays,omitempty"`
	Weeks           []DashboardOpenProjectOverlayWeekBucket `json:"weeks"`
}

// DashboardOpenProjectOverlayWeekBucket is a resource's week: planned load
// against the capacity in effect that week. For a concurrent resource the
// planned value is the busiest day.
type DashboardOpenProjectOverlayWeekBucket struct {
	Label      string  `json:"label"`
	StartDate  string  `json:"start_date"`
	FinishDate string  `json:"finish_date"`
	Capacity   float64 `json:"capacity"`
	Planned    float64 `json:"planned"`
	Overload   float64 `json:"overload,omitempty"`
}

// DashboardOpenProjectOverlayOverloadWeek is a resource-week from the status
// week on that `replan workload --week` reports as overloaded: its load is
// the work still remaining, or the reserved amount, against the capacity in
// effect that week.
type DashboardOpenProjectOverlayOverloadWeek struct {
	ResourceID   string  `json:"resource_id"`
	ResourceName string  `json:"resource_name"`
	Unit         string  `json:"unit,omitempty"`
	Week         string  `json:"week"`
	Capacity     float64 `json:"capacity"`
	Load         float64 `json:"load"`
	Overload     float64 `json:"overload"`
}

type DashboardOpenProjectOverlayArtifacts struct {
	OverviewJSON  string `json:"overview_json"`
	GanttJSON     string `json:"gantt_json"`
	OverlayJSON   string `json:"overlay_json"`
	DashboardHTML string `json:"dashboard_html"`
	GanttHTML     string `json:"gantt_html"`
}

type DashboardOpenProjectOverlayWorkPackage struct {
	WorkPackageID         string                                      `json:"work_package_id"`
	WorkPackageURL        string                                      `json:"work_package_url,omitempty"`
	RePlanTaskID          string                                      `json:"replan_task_id"`
	Title                 string                                      `json:"title"`
	Type                  string                                      `json:"type,omitempty"`
	TrackingStatus        string                                      `json:"tracking_status,omitempty"`
	PlannedStart          string                                      `json:"planned_start,omitempty"`
	PlannedFinish         string                                      `json:"planned_finish,omitempty"`
	StartOffsetDays       int                                         `json:"start_offset_days,omitempty"`
	DurationDays          int                                         `json:"duration_days,omitempty"`
	ResourceIDs           []string                                    `json:"resource_ids,omitempty"`
	OverloadedResourceIDs []string                                    `json:"overloaded_resource_ids,omitempty"`
	ResourceDemands       []DashboardOpenProjectOverlayResourceDemand `json:"resource_demands,omitempty"`
	WarningCodes          []string                                    `json:"warning_codes,omitempty"`
	DependencyCount       int                                         `json:"dependency_count"`
	OpenDependencyCount   int                                         `json:"open_dependency_count"`
	HasOpenProposal       bool                                        `json:"has_open_proposal,omitempty"`
	Tracking              DashboardOpenProjectOverlayTracking         `json:"tracking"`

	// Version 2.
	FirstActualDate string                                  `json:"first_actual_date,omitempty"`
	LastActualDate  string                                  `json:"last_actual_date,omitempty"`
	Evidence        []DashboardOpenProjectOverlayEvidence   `json:"evidence"`
	Actuals         []DashboardOpenProjectOverlayActual     `json:"actuals"`
	Warnings        []ReportIssue                           `json:"warnings"`
	OpenProposals   []DashboardOpenProjectOverlayProposal   `json:"open_proposals"`
	Dependencies    []DashboardOpenProjectOverlayDependency `json:"dependencies"`
}

// DashboardOpenProjectOverlayEvidence is what was done, as recorded.
type DashboardOpenProjectOverlayEvidence struct {
	ID      string `json:"id"`
	Date    string `json:"date,omitempty"`
	Kind    string `json:"kind,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// DashboardOpenProjectOverlayActual is booked consumption.
type DashboardOpenProjectOverlayActual struct {
	Date         string  `json:"date,omitempty"`
	DemandID     string  `json:"demand_id,omitempty"`
	ResourceID   string  `json:"resource_id,omitempty"`
	ResourceName string  `json:"resource_name,omitempty"`
	Amount       float64 `json:"amount"`
	Unit         string  `json:"unit,omitempty"`
}

// DashboardOpenProjectOverlayProposal is an open proposal about the task or
// one of its demands.
type DashboardOpenProjectOverlayProposal struct {
	ID        string `json:"id"`
	Type      string `json:"type,omitempty"`
	TargetID  string `json:"target_id,omitempty"`
	Rationale string `json:"rationale,omitempty"`
}

// DashboardOpenProjectOverlayDependency is a task dependency seen from one
// side: the task waits on a predecessor or a successor waits on it.
type DashboardOpenProjectOverlayDependency struct {
	ID            string `json:"id"`
	Direction     string `json:"direction"`
	TaskID        string `json:"task_id,omitempty"`
	TaskTitle     string `json:"task_title,omitempty"`
	WorkPackageID string `json:"work_package_id,omitempty"`
	TaskStatus    string `json:"task_status,omitempty"`
	Status        string `json:"status"`
	Criticality   string `json:"criticality,omitempty"`
}

type DashboardOpenProjectOverlayResourceDemand struct {
	DemandID                  string   `json:"demand_id"`
	ResourceID                string   `json:"resource_id,omitempty"`
	ResourceClass             string   `json:"resource_class,omitempty"`
	Role                      string   `json:"role,omitempty"`
	Unit                      string   `json:"unit,omitempty"`
	PlannedAmount             float64  `json:"planned_amount,omitempty"`
	ActualAmount              float64  `json:"actual_amount,omitempty"`
	RemainingAmount           float64  `json:"remaining_amount,omitempty"`
	ForecastAmount            float64  `json:"forecast_amount,omitempty"`
	VarianceAmount            float64  `json:"variance_amount,omitempty"`
	EvidenceCount             int      `json:"evidence_count,omitempty"`
	StartDate                 string   `json:"start_date,omitempty"`
	FinishDate                string   `json:"finish_date,omitempty"`
	StartOffsetDays           int      `json:"start_offset_days,omitempty"`
	DurationDays              int      `json:"duration_days,omitempty"`
	ParticipationWindowSource string   `json:"participation_window_source,omitempty"`
	WindowCapacity            float64  `json:"window_capacity,omitempty"`
	IntensityRatio            float64  `json:"intensity_ratio,omitempty"`
	IntensityPercent          float64  `json:"intensity_percent,omitempty"`
	CapacityKnown             bool     `json:"capacity_known"`
	AmountPerDay              float64  `json:"amount_per_day,omitempty"`
	Overloaded                bool     `json:"overloaded,omitempty"`
	OverCapacity              bool     `json:"over_capacity,omitempty"`
	Flags                     []string `json:"flags,omitempty"`
	// Version 2: the lane's name, whether it is a slot, and the daily
	// capacity in effect on the window's first working day.
	ResourceName  string  `json:"resource_name,omitempty"`
	Concurrent    bool    `json:"concurrent,omitempty"`
	DailyCapacity float64 `json:"daily_capacity,omitempty"`
}

type DashboardOpenProjectOverlayTracking struct {
	ActualCount   int      `json:"actual_count"`
	EvidenceCount int      `json:"evidence_count"`
	Flags         []string `json:"flags,omitempty"`
}

type DashboardTaskRow struct {
	ID                  string   `json:"id"`
	Title               string   `json:"title"`
	Type                string   `json:"type,omitempty"`
	TrackingStatus      string   `json:"tracking_status"`
	PlannedStart        string   `json:"planned_start,omitempty"`
	PlannedFinish       string   `json:"planned_finish,omitempty"`
	AssigneeID          string   `json:"assignee_id,omitempty"`
	ResourceDemandCount int      `json:"resource_demand_count"`
	DependencyCount     int      `json:"dependency_count"`
	WarningCount        int      `json:"warning_count"`
	SourceRefCount      int      `json:"source_ref_count"`
	Tags                []string `json:"tags,omitempty"`
}

type DashboardTrackingRow struct {
	TaskID         string                    `json:"task_id"`
	Title          string                    `json:"title"`
	TrackingStatus string                    `json:"tracking_status"`
	PlannedStart   string                    `json:"planned_start,omitempty"`
	PlannedFinish  string                    `json:"planned_finish,omitempty"`
	ActualCount    int                       `json:"actual_count"`
	EvidenceCount  int                       `json:"evidence_count"`
	Flags          []string                  `json:"flags,omitempty"`
	Demands        []DashboardDemandTracking `json:"demands,omitempty"`
}

type DashboardDemandTracking struct {
	DemandID      string   `json:"demand_id"`
	ResourceID    string   `json:"resource_id,omitempty"`
	ResourceClass string   `json:"resource_class,omitempty"`
	Role          string   `json:"role,omitempty"`
	Unit          string   `json:"unit,omitempty"`
	Planned       float64  `json:"planned,omitempty"`
	Actual        float64  `json:"actual,omitempty"`
	Remaining     float64  `json:"remaining,omitempty"`
	Forecast      float64  `json:"forecast,omitempty"`
	Variance      float64  `json:"variance,omitempty"`
	EvidenceCount int      `json:"evidence_count"`
	Flags         []string `json:"flags,omitempty"`
}

type DashboardTrackingLoop struct {
	StaleRemainingCount   int                        `json:"stale_remaining_count"`
	OpenProposalCount     int                        `json:"open_proposal_count"`
	AcceptedProposalCount int                        `json:"accepted_proposal_count"`
	CoveredDemandCount    int                        `json:"covered_demand_count"`
	ActionableCount       int                        `json:"actionable_count"`
	Rows                  []DashboardTrackingLoopRow `json:"rows,omitempty"`
}

type DashboardTrackingLoopRow struct {
	TaskID             string  `json:"task_id"`
	TaskTitle          string  `json:"task_title,omitempty"`
	DemandID           string  `json:"demand_id"`
	ResourceID         string  `json:"resource_id,omitempty"`
	Unit               string  `json:"unit,omitempty"`
	Planned            float64 `json:"planned,omitempty"`
	Actual             float64 `json:"actual,omitempty"`
	Remaining          float64 `json:"remaining,omitempty"`
	Forecast           float64 `json:"forecast,omitempty"`
	Variance           float64 `json:"variance,omitempty"`
	Status             string  `json:"status"`
	Priority           string  `json:"priority"`
	Reason             string  `json:"reason,omitempty"`
	ProposalID         string  `json:"proposal_id,omitempty"`
	ProposalStatus     string  `json:"proposal_status,omitempty"`
	RecommendedCommand string  `json:"recommended_command,omitempty"`
}

type DashboardGanttView struct {
	SchemaVersion int                          `json:"schema_version"`
	ProjectID     string                       `json:"project_id"`
	ProjectName   string                       `json:"project_name"`
	StatusDate    string                       `json:"status_date,omitempty"`
	Week          string                       `json:"week,omitempty"`
	StartDate     string                       `json:"start_date,omitempty"`
	FinishDate    string                       `json:"finish_date,omitempty"`
	DayCount      int                          `json:"day_count"`
	StatusOffset  int                          `json:"status_offset_days"`
	StatusInRange bool                         `json:"status_in_range"`
	TaskCount     int                          `json:"task_count"`
	Tasks         []DashboardGanttTask         `json:"tasks"`
	ResourceLoads []DashboardGanttResourceLoad `json:"resource_loads,omitempty"`
	Unscheduled   []DashboardGanttTask         `json:"unscheduled,omitempty"`
}

type DashboardGanttTask struct {
	ID                    string                             `json:"id"`
	Title                 string                             `json:"title"`
	Type                  string                             `json:"type,omitempty"`
	TrackingStatus        string                             `json:"tracking_status"`
	PlannedStart          string                             `json:"planned_start,omitempty"`
	PlannedFinish         string                             `json:"planned_finish,omitempty"`
	StartOffsetDays       int                                `json:"start_offset_days"`
	DurationDays          int                                `json:"duration_days"`
	ResourceIDs           []string                           `json:"resource_ids,omitempty"`
	OverloadedResourceIDs []string                           `json:"overloaded_resource_ids,omitempty"`
	WarningCodes          []string                           `json:"warning_codes,omitempty"`
	DependencyCount       int                                `json:"dependency_count"`
	OpenDependencyCount   int                                `json:"open_dependency_count"`
	HasOpenProposal       bool                               `json:"has_open_proposal,omitempty"`
	Source                string                             `json:"source,omitempty"`
	SourceID              string                             `json:"source_id,omitempty"`
	SourceURL             string                             `json:"source_url,omitempty"`
	Allocations           []DashboardGanttResourceAllocation `json:"allocations,omitempty"`
}

type DashboardGanttResourceAllocation struct {
	DemandID                  string  `json:"demand_id"`
	ResourceID                string  `json:"resource_id"`
	ResourceClass             string  `json:"resource_class,omitempty"`
	Role                      string  `json:"role,omitempty"`
	Unit                      string  `json:"unit,omitempty"`
	PlannedAmount             float64 `json:"planned_amount,omitempty"`
	RemainingAmount           float64 `json:"remaining_amount,omitempty"`
	StartDate                 string  `json:"start_date,omitempty"`
	FinishDate                string  `json:"finish_date,omitempty"`
	StartOffsetDays           int     `json:"start_offset_days"`
	DurationDays              int     `json:"duration_days"`
	ParticipationWindowSource string  `json:"participation_window_source,omitempty"`
	WindowCapacity            float64 `json:"window_capacity,omitempty"`
	IntensityRatio            float64 `json:"intensity_ratio,omitempty"`
	IntensityPercent          float64 `json:"intensity_percent,omitempty"`
	CapacityKnown             bool    `json:"capacity_known"`
	AmountPerDay              float64 `json:"amount_per_day,omitempty"`
	Overloaded                bool    `json:"overloaded,omitempty"`
	OverCapacity              bool    `json:"over_capacity,omitempty"`
}

type DashboardGanttResourceLoad struct {
	ResourceID    string  `json:"resource_id"`
	ResourceName  string  `json:"resource_name,omitempty"`
	ResourceClass string  `json:"resource_class,omitempty"`
	Unit          string  `json:"unit,omitempty"`
	Capacity      float64 `json:"capacity,omitempty"`
	Planned       float64 `json:"planned,omitempty"`
	Overload      float64 `json:"overload,omitempty"`
	BucketType    string  `json:"bucket_type,omitempty"`
	// Concurrent marks slot capacity: a bucket holds the peak daily use, not
	// the sum.
	Concurrent          bool                               `json:"concurrent,omitempty"`
	WaterlineRatio      float64                            `json:"waterline_ratio,omitempty"`
	MaxUtilizationRatio float64                            `json:"max_utilization_ratio,omitempty"`
	Buckets             []DashboardGanttResourceLoadBucket `json:"buckets,omitempty"`
	DailyAmounts        []float64                          `json:"daily_amounts,omitempty"`
	PeakDaily           float64                            `json:"peak_daily,omitempty"`
	CapacityPerDay      float64                            `json:"capacity_per_day,omitempty"`
}

type DashboardGanttResourceLoadBucket struct {
	Label            string  `json:"label"`
	StartDate        string  `json:"start_date"`
	FinishDate       string  `json:"finish_date"`
	Capacity         float64 `json:"capacity,omitempty"`
	Planned          float64 `json:"planned,omitempty"`
	UtilizationRatio float64 `json:"utilization_ratio,omitempty"`
	UtilizationPct   float64 `json:"utilization_percent,omitempty"`
	Overload         float64 `json:"overload,omitempty"`
	OverCapacity     bool    `json:"over_capacity,omitempty"`
}

type DashboardDependency struct {
	ID            string `json:"id"`
	Type          string `json:"type"`
	PredecessorID string `json:"predecessor_id,omitempty"`
	SuccessorID   string `json:"successor_id,omitempty"`
	Status        string `json:"status"`
	Criticality   string `json:"criticality,omitempty"`
	Notes         string `json:"notes,omitempty"`
}

type DashboardScenarioRow struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Status      string `json:"status"`
	ChangeCount int    `json:"change_count"`
}

// Next-action actors: who can act on the item.
const (
	NextActionActorPerson = "person"
	NextActionActorAgent  = "agent"
)

type DashboardNextAction struct {
	Code    string `json:"code"`
	Label   string `json:"label"`
	Target  string `json:"target,omitempty"`
	Command string `json:"command,omitempty"`
	// Actor is who can act: a person decides, accepts or does human work; an
	// agent can prepare it through commands and proposals.
	Actor string `json:"actor,omitempty"`
	// Date orders actions within an actor: the task's planned finish where
	// there is one.
	Date string `json:"date,omitempty"`
	// Details carries machine-readable values for an aggregate action.
	Details []CalibrationDemandDetail `json:"details,omitempty"`
}

type CalibrationDemandDetail struct {
	DemandID        string  `json:"demand_id"`
	TypedAmount     float64 `json:"typed_amount"`
	CalibratedValue float64 `json:"calibrated_value"`
	Unit            string  `json:"unit"`
}

type DashboardManifest struct {
	SchemaVersion int                         `json:"schema_version"`
	GeneratedAt   string                      `json:"generated_at"`
	Command       string                      `json:"command"`
	ProjectPath   string                      `json:"project_path"`
	ProjectID     string                      `json:"project_id"`
	StatusDate    string                      `json:"status_date,omitempty"`
	ScenarioID    string                      `json:"scenario_id,omitempty"`
	Artifacts     []DashboardManifestArtifact `json:"artifacts"`
	WarningsCount int                         `json:"warnings_count"`
	ErrorsCount   int                         `json:"errors_count"`
}

type DashboardManifestArtifact struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256,omitempty"`
}
