package app

import (
	"encoding/json"
	"os"
	"testing"
)

func TestOpenProjectStatusMapMatchesSharedMapping(t *testing.T) {
	data, err := os.ReadFile("../../scripts/lib/openproject_status_map.json")
	if err != nil {
		t.Fatalf("read shared OpenProject status map: %v", err)
	}
	var pairs []struct {
		TrackingStatus    string `json:"tracking_status"`
		OpenProjectStatus string `json:"openproject_status"`
	}
	if err := json.Unmarshal(data, &pairs); err != nil {
		t.Fatalf("decode shared OpenProject status map: %v", err)
	}
	for _, pair := range pairs {
		if got := mapOpenProjectStatus(pair.OpenProjectStatus); got != pair.TrackingStatus {
			t.Errorf("mapOpenProjectStatus(%q) = %q, want %q", pair.OpenProjectStatus, got, pair.TrackingStatus)
		}
	}
}
