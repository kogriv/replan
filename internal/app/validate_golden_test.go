package app

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

// everyDiagnosticWorkspace is a workspace in which every diagnostic code of
// validateWorkspace and its helpers fires at least once. Its diagnostics,
// in order, are the golden file testdata/validate-every-diagnostic.json.
func everyDiagnosticWorkspace() domain.Workspace {
	nan := math.NaN()
	ref := func(kind, id string) domain.Ref { return domain.Ref{Kind: kind, ID: id} }
	return domain.Workspace{
		Project: domain.Project{
			DefaultStatusDate: "2026-10-10",
			OpenProjectIDs:    "elsewhere",
			SourceProfiles: []domain.SourceProfile{
				{Source: "openproject", BaseURL: "https://user:pw@op.example.internal", TokenEnv: "abc-123"},
				{ID: "op.dup", Source: "openproject", BaseURL: "ftp://op.example.internal", ProjectID: "p"},
				{ID: "op.dup", Source: "openproject", ProjectID: "p", TokenEnv: "OPENPROJECT_TOKEN"},
				{ID: "jira"},
				{ID: "jira.other", Source: "jira"},
			},
		},
		Actors: []domain.Actor{
			{ID: "actor.ok", Kind: "human", TrustLevel: "high"},
			{ID: "actor.bare"},
			{},
			{ID: "actor.ok", Kind: "human", TrustLevel: "high"},
		},
		Resources: []domain.Resource{
			{ID: "resource.ok", Class: "human", Name: "OK", ActorRef: ref("actor", "actor.ok")},
			{ID: "resource.bare", ActorRef: ref("actor", "actor.missing")},
		},
		ResourcePools: []domain.ResourcePool{
			{ID: "pool.bare", Members: []domain.Ref{ref("resource", "resource.missing")}},
		},
		Tasks: []domain.Task{
			{ID: "task.a", Title: "A", TrackingStatus: "accepted", PlannedStart: "2026-10-01", PlannedFinish: "2026-10-20",
				ResourceDemands: []domain.Ref{ref("resource_demand", "demand.a"), ref("resource_demand", "demand.missing")},
				Dependencies:    []domain.Ref{ref("dependency", "dependency.missing")}},
			{ID: "task.b", TrackingStatus: "nonsense", PlannedStart: "10/01/2026", PlannedFinish: "soon"},
			{ID: "task.c", Title: "C", TrackingStatus: "in_progress", PlannedStart: "2026-10-05", PlannedFinish: "2026-10-06",
				ResourceDemands: []domain.Ref{ref("resource_demand", "demand.b")}},
			{ID: "task.d", Title: "D", TrackingStatus: "not_started",
				ResourceDemands: []domain.Ref{ref("resource_demand", "demand.c")}},
		},
		ResourceDemands: []domain.ResourceDemand{
			{ID: "demand.a", TaskRef: ref("task", "task.a"), ResourceRef: ref("resource", "resource.ok"), ResourceClass: "human", Unit: "hours", PlannedAmount: 2,
				OverrunReview: &domain.OverrunReview{AcknowledgedForecast: -1, Unit: "days", ReviewedBy: ref("actor", "actor.missing"),
					EvidenceRefs: []domain.Ref{ref("evidence", "evidence.missing")}}},
			{ID: "demand.b", TaskRef: ref("task", "task.missing"), Status: "weird", PlannedAmount: nan, RemainingAmount: &nan},
			{ID: "demand.c", TaskRef: ref("task", "task.d"), ResourceRef: ref("resource", "resource.missing"), ResourcePoolRef: ref("resource_pool", "pool.missing"), ResourceClass: "human", Unit: "hours"},
		},
		Dependencies: []domain.Dependency{
			{ID: "dependency.bad", Type: "sideways", Status: "maybe", Criticality: "kinda", OwnerRef: ref("actor", "actor.missing"), ExpectedResolutionDate: "next week",
				SourceRefs: []domain.SourceRef{{Source: "openproject"}}},
			{ID: "dependency.external", Type: "external_dependency", Status: "open", Criticality: "required", SuccessorRef: ref("task", "task.missing")},
			{ID: "dependency.self", Type: "finish_to_start", Status: "open", Criticality: "required", PredecessorRef: ref("task", "task.c"), SuccessorRef: ref("task", "task.c"),
				SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "relation", ObjectID: "1"}}},
			{ID: "dependency.fs", Type: "finish_to_start", Status: "open", Criticality: "required", PredecessorRef: ref("task", "task.c"), SuccessorRef: ref("task", "task.a"),
				SourceRefs: []domain.SourceRef{{Source: "openproject", ObjectType: "relation", ObjectID: "1"}}},
			{ID: "dependency.fs2", Type: "finish_to_start", Status: "open", Criticality: "optional", PredecessorRef: ref("task", "task.c"), SuccessorRef: ref("task", "task.a")},
			{ID: "dependency.unknown-dates", Type: "finish_to_start", Status: "open", Criticality: "optional", PredecessorRef: ref("task", "task.d"), SuccessorRef: ref("task", "task.d")},
			{ID: "dependency.blocks", Type: "blocks", Status: "open", Criticality: "optional", PredecessorRef: ref("task", "task.a"), SuccessorRef: ref("task", "task.c")},
			{ID: "dependency.no-pred", Type: "finish_to_start", Status: "open", Criticality: "optional", PredecessorRef: ref("task", "task.missing")},
			{ID: "dependency.no-pred-id", Type: "finish_to_start", Status: "open", Criticality: "optional", SuccessorRef: ref("task", "task.c")},
		},
		Capacities: []domain.Capacity{
			{ID: "capacity.bad", ResourceRef: ref("resource", "resource.missing"), EffectiveFrom: "2026-10-10", EffectiveTo: "2026-10-01"},
			{ID: "capacity.date", ResourceRef: ref("resource", "resource.ok"), Unit: "hours", Type: "weekly", EffectiveFrom: "tomorrow"},
		},
		Availabilities: []domain.Availability{{ID: "availability.bad", ResourceRef: ref("resource", "resource.missing")}},
		Baselines: []domain.Baseline{
			{ID: "baseline.bad",
				IncludedTasks: []domain.BaselineTaskEntry{
					{Ref: ref("task", "task.a"), PlannedStart: "soon"},
					{Ref: ref("task", "task.c"), PlannedStart: "2026-10-10", PlannedFinish: "2026-10-01"},
				},
				IncludedDemands: []domain.BaselineDemandEntry{
					{Ref: ref("resource_demand", "demand.a"), Resource: "resource.ok"},
					{Ref: ref("resource_demand", "demand.b"), Task: "task.b", PlannedAmount: &nan},
				},
				Seal: &domain.BaselineSeal{Algorithm: "md5", Digest: "bad"},
			},
			{ID: "baseline.modified", ApprovedBy: ref("actor", "actor.ok"), Seal: &domain.BaselineSeal{Algorithm: "sha256", Digest: "bad"}},
		},
		Calibrations: []domain.Calibration{
			{ID: "calibration.unsealed", ApprovedBy: ref("actor", "actor.ok"), Result: domain.CalibrationResult{Count: 1}},
			{ID: "calibration.modified", ApprovedBy: ref("actor", "actor.ok"), SourceBaselines: []domain.CalibrationSourceBaseline{{ID: "baseline.modified", SealSHA256: "other"}}, SealSHA256: "bad"},
		},
		Actuals:      []domain.Actual{{ID: "actual.bad", Amount: 1}},
		Evidence:     []domain.Evidence{{ID: "evidence.bad"}},
		Proposals:    []domain.Proposal{{ID: "proposal.bad", Status: "pending"}},
		Scenarios:    []domain.Scenario{{ID: "scenario.bad", Status: "pending"}},
		AuditRecords: []domain.AuditRecord{{ID: "audit.bad"}},
	}
}

func validateForGolden(ws domain.Workspace) ValidateResult {
	result := ValidateResult{Warnings: []Diagnostic{}, Errors: []Diagnostic{}}
	validateWorkspace(&result, ws)
	return result
}

func checkGolden(t *testing.T, name string, value any) {
	t.Helper()
	got, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", name)
	if os.Getenv("REPLAN_UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v (REPLAN_UPDATE_GOLDEN=1 writes it)", path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("diagnostics differ from %s", path)
	}
}

func TestValidateEveryDiagnosticGolden(t *testing.T) {
	checkGolden(t, "validate-every-diagnostic.json", validateForGolden(everyDiagnosticWorkspace()))
}

// The fixture covers every code the validator source can emit.
func TestValidateFixtureTriggersEveryDiagnosticCode(t *testing.T) {
	source, err := os.ReadFile("validate.go")
	if err != nil {
		t.Fatal(err)
	}
	all := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?:add(?:Error|Warning)\(result, |v\.(?:errorf|warn)\(|Code:\s*)"([a-z_]+)"`).FindAllSubmatch(source, -1) {
		all[string(m[1])] = true
	}
	result := validateForGolden(everyDiagnosticWorkspace())
	seen := map[string]bool{}
	for _, d := range append(append([]Diagnostic{}, result.Errors...), result.Warnings...) {
		seen[d.Code] = true
	}
	var missing []string
	for code := range all {
		if !seen[code] {
			missing = append(missing, code)
		}
	}
	sort.Strings(missing)
	if len(all) < 50 || len(missing) > 0 {
		t.Fatalf("codes in validate.go: %d; not triggered by the fixture: %v", len(all), missing)
	}
}
