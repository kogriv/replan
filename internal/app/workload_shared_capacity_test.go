package app_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func sharedCapacityWorkspace() domain.Workspace {
	a, b := domain.Ref{ID: "a"}, domain.Ref{ID: "b"}
	return domain.Workspace{
		Resources:     []domain.Resource{{ID: a.ID, Class: "human"}, {ID: b.ID, Class: "human"}},
		ResourcePools: []domain.ResourcePool{{ID: "p", ResourceClass: "review", Members: []domain.Ref{a}}},
		Capacities: []domain.Capacity{
			{ResourceRef: a, Period: "week", Unit: "hours", Amount: 10},
			{ResourceRef: b, Period: "week", Unit: "hours", Amount: 10},
		},
		ResourceDemands: []domain.ResourceDemand{
			sharedCapacityDemand("a", false, 8),
			sharedCapacityDemand("p", true, 8),
		},
	}
}

func sharedCapacityDemand(id string, pool bool, amount float64) domain.ResourceDemand {
	d := domain.ResourceDemand{
		ID: "demand." + id, Unit: "hours", Status: "active", RemainingAmount: domain.Amount(amount),
		Timing: domain.Timing{PreferredStart: "2026-07-06", PreferredFinish: "2026-07-06"},
	}
	if pool {
		d.ResourcePoolRef = domain.Ref{ID: id}
	} else {
		d.ResourceRef = domain.Ref{ID: id}
	}
	return d
}

func TestWorkloadSharedCapacity(t *testing.T) {
	cases := []struct {
		name    string
		edit    func(*domain.Workspace)
		pools   []string
		members []string
		demand  float64
		usable  float64
	}{
		{name: "one", pools: []string{"p"}, members: []string{"a"}, demand: 8, usable: 2},
		{name: "zero-fit", edit: func(ws *domain.Workspace) {
			ws.Capacities[0].Amount = 0
			ws.ResourceDemands[1].RemainingAmount = domain.Amount(0)
		}},
		{name: "exact", edit: func(ws *domain.Workspace) { ws.Capacities[0].Amount = 16 }},
		{name: "rounded", edit: func(ws *domain.Workspace) { ws.Capacities[0].Amount = 15.99998 }},
		{name: "two-fit", edit: func(ws *domain.Workspace) {
			ws.ResourcePools[0].Members = append(ws.ResourcePools[0].Members, domain.Ref{ID: "b"})
		}},
		{name: "two-short", edit: func(ws *domain.Workspace) {
			ws.ResourcePools[0].Members = append(ws.ResourcePools[0].Members, domain.Ref{ID: "b"})
			ws.Capacities[1].Amount = 5
		}, pools: []string{"p"}, members: []string{"a", "b"}, demand: 8, usable: 7},
		{name: "overlap-short", edit: func(ws *domain.Workspace) {
			ws.ResourcePools = append(ws.ResourcePools, domain.ResourcePool{ID: "q", Members: []domain.Ref{{ID: "a"}}})
			ws.ResourceDemands[1].RemainingAmount = domain.Amount(2)
			ws.ResourceDemands = append(ws.ResourceDemands, sharedCapacityDemand("q", true, 2))
		}, pools: []string{"p", "q"}, members: []string{"a"}, demand: 4, usable: 2},
		{name: "restricted", edit: func(ws *domain.Workspace) {
			ws.ResourcePools = append(ws.ResourcePools, domain.ResourcePool{ID: "q", Members: []domain.Ref{{ID: "a"}, {ID: "b"}}})
			ws.ResourceDemands = append(ws.ResourceDemands, sharedCapacityDemand("q", true, 2))
		}, pools: []string{"p"}, members: []string{"a"}, demand: 8, usable: 2},
		{name: "reroute", edit: func(ws *domain.Workspace) {
			ws.ResourcePools[0].Members = append(ws.ResourcePools[0].Members, domain.Ref{ID: "b"})
			ws.ResourcePools = append(ws.ResourcePools, domain.ResourcePool{ID: "q", Members: []domain.Ref{{ID: "a"}}})
			ws.ResourceDemands = append(ws.ResourceDemands, sharedCapacityDemand("q", true, 2))
		}},
		{name: "missing", edit: func(ws *domain.Workspace) { ws.Capacities = ws.Capacities[1:] }},
		{name: "monthly", edit: func(ws *domain.Workspace) { ws.Capacities[0].Period = "month" }},
		{name: "concurrent", edit: func(ws *domain.Workspace) { ws.Capacities[0].Period = "concurrent" }},
		{name: "outside", edit: func(ws *domain.Workspace) { ws.Capacities[0].EffectiveTo = "2026-07-05" }},
		{name: "missing-member", edit: func(ws *domain.Workspace) {
			ws.ResourcePools[0].Members = append(ws.ResourcePools[0].Members, domain.Ref{ID: "missing"})
		}},
		{name: "zero", edit: func(ws *domain.Workspace) {
			ws.Capacities[0].Amount = 0
		}, pools: []string{"p"}, members: []string{"a"}, demand: 8, usable: 0},
		{name: "duplicate", edit: func(ws *domain.Workspace) {
			ws.ResourcePools[0].Members = append(ws.ResourcePools[0].Members, domain.Ref{ID: "a"})
		}, pools: []string{"p"}, members: []string{"a"}, demand: 8, usable: 2},
		{name: "reserved", edit: func(ws *domain.Workspace) {
			ws.ResourceDemands[0].ReservedAmount = 3
			ws.ResourceDemands[1].ReservedAmount = 9
		}, pools: []string{"p"}, members: []string{"a"}, demand: 9, usable: 7},
		{name: "units", edit: func(ws *domain.Workspace) { ws.ResourceDemands[0].Unit = "tokens" }},
		{name: "terminal", edit: func(ws *domain.Workspace) { ws.ResourceDemands[0].Status = "cancelled" }},
		{name: "unlevelled", edit: func(ws *domain.Workspace) {
			levelled := false
			ws.ResourceDemands[0].Levelled = &levelled
		}},
		{name: "unscheduled", edit: func(ws *domain.Workspace) { ws.ResourceDemands[0].Timing = domain.Timing{} }},
		{name: "other-week", edit: func(ws *domain.Workspace) {
			ws.ResourceDemands[0].Timing = domain.Timing{PreferredStart: "2026-07-13", PreferredFinish: "2026-07-13"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ws := sharedCapacityWorkspace()
			if tc.edit != nil {
				tc.edit(&ws)
			}
			result := sharedCapacityResult(t, ws, app.WorkloadRequest{Week: "2026-W28"})
			got := sharedCapacityWarnings(result)
			if tc.pools == nil {
				if len(got) != 0 {
					t.Fatalf("unexpected warnings: %#v", result.Warnings)
				}
				return
			}
			want := &app.SharedCapacityOverload{PoolIDs: tc.pools, MemberIDs: tc.members, Unit: "hours",
				Demand: tc.demand, UsableCapacity: tc.usable, Shortfall: tc.demand - tc.usable}
			if !reflect.DeepEqual(got, []*app.SharedCapacityOverload{want}) {
				t.Fatalf("warnings = %#v, want %+v", result.Warnings, want)
			}
			if tc.name == "one" {
				assertWorkload(t, result, "a", "hours", 10, 0, 8, 0)
				assertWorkload(t, result, "p", "hours", 10, 0, 8, 0)
			}
		})
	}
}

func TestWorkloadSharedFilters(t *testing.T) {
	ws := sharedCapacityWorkspace()
	ws.ResourcePools[0].ResourceClass = "review"
	ws.ResourcePools = append(ws.ResourcePools,
		domain.ResourcePool{ID: "q", ResourceClass: "human", Members: []domain.Ref{{ID: "a"}}},
		domain.ResourcePool{ID: "r", ResourceClass: "other", Members: []domain.Ref{{ID: "b"}}})
	ws.ResourceDemands[0].RemainingAmount = domain.Amount(0)
	ws.ResourceDemands = append(ws.ResourceDemands, sharedCapacityDemand("q", true, 8), sharedCapacityDemand("r", true, 15))

	cases := []struct {
		name  string
		req   app.WorkloadRequest
		first string
		count int
	}{
		{"all", app.WorkloadRequest{}, "p", 2},
		{"pool", app.WorkloadRequest{ResourceID: "p"}, "p", 1},
		{"member", app.WorkloadRequest{ResourceID: "a"}, "p", 1},
		{"class", app.WorkloadRequest{ResourceClass: "review"}, "p", 1},
		{"other", app.WorkloadRequest{ResourceID: "b"}, "r", 1},
		{"unit", app.WorkloadRequest{Unit: "tokens"}, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Week = "2026-W28"
			result := sharedCapacityResult(t, ws, tc.req)
			got := sharedCapacityWarnings(result)
			if len(got) != tc.count {
				t.Fatalf("warnings = %#v, want %d", result.Warnings, tc.count)
			}
			if len(got) > 0 && got[0].PoolIDs[0] != tc.first {
				t.Fatalf("first pools = %v, want %s", got[0].PoolIDs, tc.first)
			}
			if tc.req.ResourceID != "" && (len(result.Resources) != 1 || result.Resources[0].ResourceID != tc.req.ResourceID) {
				t.Fatalf("filtered rows = %#v", result.Resources)
			}
		})
	}
}

func sharedCapacityResult(t *testing.T, ws domain.Workspace, req app.WorkloadRequest) app.WorkloadResult {
	t.Helper()
	result, err := app.NewWorkloadService(staticWorkspaceLoader{workspace: ws}).Calculate(context.Background(), req)
	if err != nil {
		t.Fatalf("Calculate: %v (%#v)", err, result.Errors)
	}
	return result
}

func sharedCapacityWarnings(result app.WorkloadResult) []*app.SharedCapacityOverload {
	var details []*app.SharedCapacityOverload
	for _, warning := range result.Warnings {
		if warning.Code == "shared_capacity_overload" {
			details = append(details, warning.SharedCapacity)
		}
	}
	return details
}

func TestWorkloadSharedViews(t *testing.T) {
	ws := sharedCapacityWorkspace()
	ws.Capacities[0].ID = "capacity.a"
	ws.Scenarios = []domain.Scenario{{ID: "scenario.more", Changes: []domain.ScenarioChange{
		{Op: "set", Path: "capacities.capacity.a.amount", Value: 16},
	}}}
	ws.Tasks = []domain.Task{{ID: "task.a", PlannedStart: "2026-07-06"}}
	ws.ResourceDemands[0].TaskRef = domain.Ref{ID: "task.a"}
	ws.ResourceDemands[0].PlannedAmount = 8
	calibration := sealedWorkloadCalibration(t, "calibration.a", "2026-07-06T00:00:00Z", "", .25, nil, nil, nil)
	calibration.Scope = domain.CalibrationScope{Resources: []string{"a"}, Unit: "hours"}
	resealWorkloadCalibration(t, &calibration)
	ws.Calibrations = []domain.Calibration{calibration}

	for _, tc := range []struct {
		name string
		req  app.WorkloadRequest
		want int
	}{
		{"raw", app.WorkloadRequest{Raw: true}, 1},
		{"calibrated", app.WorkloadRequest{}, 0},
		{"scenario", app.WorkloadRequest{Raw: true, ScenarioID: "scenario.more"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.req.Week = "2026-W28"
			tc.req.ResourceID = "p"
			got := sharedCapacityWarnings(sharedCapacityResult(t, ws, tc.req))
			if len(got) != tc.want {
				t.Fatalf("got %d warnings, want %d", len(got), tc.want)
			}
		})
	}
}
