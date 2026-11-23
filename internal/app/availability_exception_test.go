package app_test

import (
	"context"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/kogriv/replan/internal/app"
	"github.com/kogriv/replan/internal/domain"
)

func availabilityExceptionWorkspace(exceptionType, exceptionDate string) domain.Workspace {
	resource := domain.Ref{Kind: "resource", ID: "resource.person"}
	amount := 10.0
	return domain.Workspace{
		Project:    domain.Project{ID: "project.exceptions", Name: "Exceptions", Timezone: "UTC", DefaultStatusDate: "2026-07-08"},
		Resources:  []domain.Resource{{ID: resource.ID, Class: "human", Name: "Person"}},
		Capacities: []domain.Capacity{{ID: "capacity.person", ResourceRef: resource, Type: "periodic_capacity", Unit: "hours", Amount: 40, Period: "week"}},
		Availabilities: []domain.Availability{{
			ID: "availability.person", ResourceRef: resource,
			Windows:    []domain.AvailabilityWindow{{Days: []string{"monday", "tuesday", "wednesday", "thursday", "friday"}}},
			Exceptions: []domain.AvailabilityException{{Date: exceptionDate, Type: exceptionType, Reason: "test"}},
		}},
		Tasks: []domain.Task{{
			ID: "task.week", Title: "Week", TrackingStatus: "not_started", PlannedStart: "2026-07-06", PlannedFinish: "2026-07-10",
			ResourceDemands: []domain.Ref{{Kind: "resource_demand", ID: "demand.week"}},
		}},
		ResourceDemands: []domain.ResourceDemand{{
			ID: "demand.week", TaskRef: domain.Ref{Kind: "task", ID: "task.week"}, ResourceRef: resource,
			Unit: "hours", PlannedAmount: amount, RemainingAmount: &amount, Status: "active", Criticality: "required",
		}},
	}
}

func calculateExceptionWorkload(t *testing.T, ws domain.Workspace) app.WorkloadResult {
	t.Helper()
	result, err := app.NewWorkloadService(stubLoader{ws: ws}).Calculate(context.Background(), app.WorkloadRequest{Week: "2026-W28"})
	if err != nil {
		t.Fatalf("Calculate returned error: %v", err)
	}
	return result
}

func TestWholeDayAvailabilityExceptionsReduceWeeklyCapacity(t *testing.T) {
	for _, exceptionType := range []string{"unavailable", "maintenance", "access_closed", "manual_hold"} {
		t.Run(exceptionType, func(t *testing.T) {
			result := calculateExceptionWorkload(t, availabilityExceptionWorkspace(exceptionType, "2026-07-08"))
			row := findWorkloadRow(t, result.Resources, "resource.person", "hours")
			if row.Capacity != 32 || row.Planned != 10 || row.Remaining != 10 {
				t.Fatalf("row = %#v, want capacity 32 and unchanged demand totals 10", row)
			}
			if len(result.Warnings) != 0 {
				t.Fatalf("warnings = %#v, want none", result.Warnings)
			}
		})
	}
}

func TestAmountLessAndNonWorkingDayExceptionsDoNotChangeCapacity(t *testing.T) {
	reduced := calculateExceptionWorkload(t, availabilityExceptionWorkspace("reduced_capacity", "2026-07-08"))
	if row := findWorkloadRow(t, reduced.Resources, "resource.person", "hours"); row.Capacity != 40 {
		t.Fatalf("reduced-capacity row = %#v, want capacity 40", row)
	}
	if len(reduced.Warnings) != 1 || reduced.Warnings[0].Code != "availability_exceptions_not_applied" || reduced.Warnings[0].Target.ID != "resource.person" {
		t.Fatalf("reduced-capacity warnings = %#v", reduced.Warnings)
	}

	weekend := calculateExceptionWorkload(t, availabilityExceptionWorkspace("unavailable", "2026-07-11"))
	if row := findWorkloadRow(t, weekend.Resources, "resource.person", "hours"); row.Capacity != 40 {
		t.Fatalf("weekend row = %#v, want capacity 40", row)
	}
}

func TestAmountLessAvailabilityExceptionsAreGroupedByResource(t *testing.T) {
	ws := availabilityExceptionWorkspace("reduced_capacity", "2026-07-08")
	ws.Availabilities[0].Exceptions = append(ws.Availabilities[0].Exceptions,
		domain.AvailabilityException{Date: "2026-07-09", Type: "extra_capacity"},
		domain.AvailabilityException{Date: "2026-07-10", Type: "quota_reset"},
	)
	result := calculateExceptionWorkload(t, ws)
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "availability_exceptions_not_applied" {
		t.Fatalf("warnings = %#v, want one grouped diagnostic", result.Warnings)
	}
	for _, want := range []string{"reduced_capacity: 1", "extra_capacity: 1", "quota_reset: 1"} {
		if !strings.Contains(result.Warnings[0].Message, want) {
			t.Errorf("warning %q does not contain %q", result.Warnings[0].Message, want)
		}
	}
}

func TestAvailabilityExceptionCombinesWithMidweekCapacityChange(t *testing.T) {
	ws := availabilityExceptionWorkspace("maintenance", "2026-07-08")
	resource := ws.Capacities[0].ResourceRef
	ws.Capacities = []domain.Capacity{
		{ID: "capacity.person.old", ResourceRef: resource, Type: "periodic_capacity", Unit: "hours", Amount: 40, Period: "week", EffectiveTo: "2026-07-07"},
		{ID: "capacity.person.new", ResourceRef: resource, Type: "periodic_capacity", Unit: "hours", Amount: 20, Period: "week", EffectiveFrom: "2026-07-08"},
	}
	row := findWorkloadRow(t, calculateExceptionWorkload(t, ws).Resources, "resource.person", "hours")
	if row.Capacity != 24 { // 40 * Mon-Tue / 5 + 20 * Thu-Fri / 5
		t.Fatalf("capacity = %v, want 24", row.Capacity)
	}
}

func TestDemandSpreadSkipsRemovedDayAndKeepsTotal(t *testing.T) {
	ws := availabilityExceptionWorkspace("unavailable", "2026-07-08")
	overview, err := app.NewReportService(stubLoader{ws: ws}).Dashboard(context.Background(), app.DashboardReportRequest{
		ProjectPath: "exceptions", GeneratedAt: time.Date(2026, 7, 6, 9, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Dashboard returned error: %v", err)
	}
	if len(overview.Gantt.ResourceLoads) != 1 {
		t.Fatalf("resource loads = %#v", overview.Gantt.ResourceLoads)
	}
	load := overview.Gantt.ResourceLoads[0]
	start, _ := time.Parse("2006-01-02", overview.Gantt.StartDate)
	total := 0.0
	for i, amount := range load.DailyAmounts {
		day := start.AddDate(0, 0, i)
		if day.Format("2006-01-02") == "2026-07-08" && amount != 0 {
			t.Fatalf("removed Wednesday has load %v", amount)
		}
		total += amount
	}
	if math.Abs(total-10) > 1e-9 {
		t.Fatalf("daily load total = %v, want 10", total)
	}
}

func TestDemandEntirelyOnRemovedDayIsRetainedWithDiagnostic(t *testing.T) {
	ws := availabilityExceptionWorkspace("unavailable", "2026-07-08")
	ws.Tasks[0].PlannedStart, ws.Tasks[0].PlannedFinish = "2026-07-08", "2026-07-08"
	result := calculateExceptionWorkload(t, ws)
	row := findWorkloadRow(t, result.Resources, "resource.person", "hours")
	if row.Planned != 10 || row.Remaining != 10 {
		t.Fatalf("row = %#v, want retained load 10", row)
	}
	if len(result.Warnings) != 1 || result.Warnings[0].Code != "demand_window_unavailable" || result.Warnings[0].Target.ID != "demand.week" {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
}

// Without a week the row shows the weekly amount in effect on the status
// date; an absence or a weekend on that date must not zero it.
func TestWorkloadWithoutWeekKeepsCapacityOnAbsenceOrWeekendStatusDate(t *testing.T) {
	for _, statusDate := range []string{"2026-07-08", "2026-07-11"} {
		ws := availabilityExceptionWorkspace("unavailable", "2026-07-08")
		ws.Project.DefaultStatusDate = statusDate
		result, err := app.NewWorkloadService(stubLoader{ws: ws}).Calculate(context.Background(), app.WorkloadRequest{})
		if err != nil {
			t.Fatalf("Calculate returned error: %v", err)
		}
		if row := findWorkloadRow(t, result.Resources, "resource.person", "hours"); row.Capacity != 40 {
			t.Fatalf("status date %s: row = %#v, want capacity 40", statusDate, row)
		}
	}
}
