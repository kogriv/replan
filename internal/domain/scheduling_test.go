package domain_test

import (
	"strings"
	"testing"

	"github.com/kogriv/replan/internal/domain"
	"gopkg.in/yaml.v3"
)

func TestSchedulingFieldsYAMLRoundTripAndDefaults(t *testing.T) {
	levelled := false
	doc := struct {
		Tasks   []domain.Task           `yaml:"tasks"`
		Demands []domain.ResourceDemand `yaml:"resource_demands"`
	}{
		Tasks: []domain.Task{{ID: "task.test", LeadTime: &domain.LeadTime{WorkingDays: 2.5, Reason: "review"}, Priority: 800}},
		Demands: []domain.ResourceDemand{
			{ID: "demand.explicit", Levelled: &levelled},
			{ID: "demand.default"},
		},
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "working_days: 2.5") || !strings.Contains(string(data), "priority: 800") || !strings.Contains(string(data), "levelled: false") {
		t.Fatalf("missing scheduling fields:\n%s", data)
	}
	var roundTrip struct {
		Tasks   []domain.Task           `yaml:"tasks"`
		Demands []domain.ResourceDemand `yaml:"resource_demands"`
	}
	if err := yaml.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	if roundTrip.Tasks[0].EffectivePriority() != 800 || roundTrip.Tasks[0].LeadTime.WorkingDays != 2.5 {
		t.Fatalf("task round trip = %#v", roundTrip.Tasks[0])
	}
	if roundTrip.Demands[0].Levelled == nil || *roundTrip.Demands[0].Levelled || roundTrip.Demands[0].IsLevelled() {
		t.Fatalf("explicit false lost: %#v", roundTrip.Demands[0])
	}
	if roundTrip.Demands[1].Levelled != nil || !roundTrip.Demands[1].IsLevelled() {
		t.Fatalf("unset default changed: %#v", roundTrip.Demands[1])
	}
	if (domain.Task{}).EffectivePriority() != 500 {
		t.Fatal("unset priority must default to 500")
	}
}
