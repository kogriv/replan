package app

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/kogriv/replan/internal/domain"
)

func TestOpenProjectStatusMapMatchesSharedMapping(t *testing.T) {
	data, err := os.ReadFile("../../scripts/lib/openproject_status_map.json")
	if err != nil {
		t.Fatalf("read shared OpenProject status map: %v", err)
	}
	var pairs []struct {
		TrackingStatus    string   `json:"tracking_status"`
		OpenProjectStatus string   `json:"openproject_status"`
		Aliases           []string `json:"aliases"`
	}
	if err := json.Unmarshal(data, &pairs); err != nil {
		t.Fatalf("decode shared OpenProject status map: %v", err)
	}
	for _, pair := range pairs {
		for _, name := range append([]string{pair.OpenProjectStatus}, pair.Aliases...) {
			if got := mapOpenProjectStatus(name); got != pair.TrackingStatus {
				t.Errorf("mapOpenProjectStatus(%q) = %q, want %q", name, got, pair.TrackingStatus)
			}
		}
	}
}

func TestValidateOpenProjectSourceProfileRejectsInvalidStatusMapTarget(t *testing.T) {
	result := ValidateResult{}
	validateOpenProjectSourceProfile(&result, domain.SourceProfile{
		BaseURL: "https://op.example.test", ProjectID: "demo", TokenEnv: "OPENPROJECT_TOKEN",
		StatusMap: map[string]string{"Quality gate": "unknown"},
	}, "openproject.test")
	if !hasDiagnosticCode(result.Errors, "invalid_openproject_status_map") {
		t.Fatalf("errors = %#v, want invalid_openproject_status_map", result.Errors)
	}
}

func TestMapOpenProjectStatusUsesProfileOverride(t *testing.T) {
	statusMap := map[string]string{"Quality gate": "review_ready", "In specification": "in_progress"}
	if got := mapOpenProjectStatusWithOverrides("quality-gate", statusMap); got != "review_ready" {
		t.Fatalf("custom status = %q, want review_ready", got)
	}
	if got := mapOpenProjectStatusWithOverrides("In specification", statusMap); got != "in_progress" {
		t.Fatalf("overridden alias = %q, want in_progress", got)
	}
}
