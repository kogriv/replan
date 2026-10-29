package domain

import "testing"

func TestValidTrackingStatus(t *testing.T) {
	valid := []string{"not_started", "ready", "in_progress", "blocked", "review_ready", "accepted", "cancelled"}
	invalid := []string{"", "done", "on_hold", "todo", "Accepted", "in-progress"}
	assertPredicate(t, "ValidTrackingStatus", ValidTrackingStatus, valid, invalid)
}

func TestValidProposalStatus(t *testing.T) {
	valid := []string{"open", "accepted", "rejected", "superseded", "expired", "applied"}
	invalid := []string{"", "closed", "draft", "Open"}
	assertPredicate(t, "ValidProposalStatus", ValidProposalStatus, valid, invalid)
}

func TestValidScenarioStatus(t *testing.T) {
	valid := []string{"draft", "calculated", "proposed", "accepted", "rejected", "applied", "archived"}
	invalid := []string{"", "open", "expired", "Draft"}
	assertPredicate(t, "ValidScenarioStatus", ValidScenarioStatus, valid, invalid)
}

func TestValidResourceDemandStatus(t *testing.T) {
	valid := []string{"draft", "active", "planned", "requested", "reserved", "partially_consumed", "consumed", "released", "cancelled", "blocked", "superseded"}
	invalid := []string{"", "done", "closed", "Active", "partially-consumed"}
	assertPredicate(t, "ValidResourceDemandStatus", ValidResourceDemandStatus, valid, invalid)
}

func TestValidDependencyType(t *testing.T) {
	valid := []string{"finish_to_start", "blocks", "requires_input_from", "requires_resource_from", "external_dependency"}
	invalid := []string{"", "start_to_start", "depends_on", "blocked"}
	assertPredicate(t, "ValidDependencyType", ValidDependencyType, valid, invalid)
}

func TestValidDependencyStatus(t *testing.T) {
	valid := []string{"open", "resolved", "blocked", "unknown", "cancelled"}
	invalid := []string{"", "closed", "done", "Resolved"}
	assertPredicate(t, "ValidDependencyStatus", ValidDependencyStatus, valid, invalid)
}

func TestValidDependencyCriticality(t *testing.T) {
	valid := []string{"required", "preferred", "optional"}
	invalid := []string{"", "critical", "nice_to_have", "Required"}
	assertPredicate(t, "ValidDependencyCriticality", ValidDependencyCriticality, valid, invalid)
}

func assertPredicate(t *testing.T, name string, predicate func(string) bool, valid, invalid []string) {
	t.Helper()
	for _, value := range valid {
		if !predicate(value) {
			t.Errorf("%s(%q) = false, want true", name, value)
		}
	}
	for _, value := range invalid {
		if predicate(value) {
			t.Errorf("%s(%q) = true, want false", name, value)
		}
	}
}
