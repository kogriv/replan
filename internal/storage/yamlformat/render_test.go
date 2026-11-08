package yamlformat_test

import (
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/domain"
	"github.com/kogriv/replan/internal/storage/yamlformat"
)

func TestSchedulingFieldsRenderInCanonicalOrder(t *testing.T) {
	falseValue := false
	data, err := yamlformat.Render(struct {
		Task   domain.Task           `yaml:"task"`
		Demand domain.ResourceDemand `yaml:"demand"`
	}{
		Task:   domain.Task{ID: "task.test", PlannedStart: "2026-11-01", PlannedFinish: "2026-11-02", LeadTime: &domain.LeadTime{CalendarDays: 1}, Priority: 800, AssigneeHint: domain.Ref{ID: "resource.test"}},
		Demand: domain.ResourceDemand{ID: "demand.test", Criticality: "required", Status: "active", Levelled: &falseValue, Timing: domain.Timing{EarliestStart: "2026-11-01"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	assertOrder := func(keys ...string) {
		previous := -1
		for _, key := range keys {
			index := strings.Index(text, key)
			if index < 0 || index <= previous {
				t.Fatalf("keys %v not in order:\n%s", keys, text)
			}
			previous = index
		}
	}
	assertOrder("planned_start:", "planned_finish:", "lead_time:", "priority:", "assignee_hint:")
	assertOrder("\n  criticality:", "\n  status:", "\n  levelled:", "\n  timing:")
}
