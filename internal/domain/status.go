package domain

func ValidTrackingStatus(status string) bool {
	switch status {
	case "not_started", "ready", "in_progress", "blocked", "review_ready", "accepted", "cancelled":
		return true
	default:
		return false
	}
}

func ValidProposalStatus(status string) bool {
	switch status {
	case "open", "accepted", "rejected", "superseded", "expired", "applied":
		return true
	default:
		return false
	}
}

func ValidScenarioStatus(status string) bool {
	switch status {
	case "draft", "calculated", "proposed", "accepted", "rejected", "applied", "archived":
		return true
	default:
		return false
	}
}

func ValidResourceDemandStatus(status string) bool {
	switch status {
	case "draft", "active", "planned", "requested", "reserved", "partially_consumed", "consumed", "released", "cancelled", "blocked", "superseded":
		return true
	default:
		return false
	}
}

func ValidDependencyType(dependencyType string) bool {
	switch dependencyType {
	case "finish_to_start", "blocks", "relates", "requires_input_from", "requires_resource_from", "external_dependency":
		return true
	default:
		return false
	}
}

func ValidDependencyStatus(status string) bool {
	switch status {
	case "open", "resolved", "blocked", "unknown", "cancelled":
		return true
	default:
		return false
	}
}

func ValidDependencyCriticality(criticality string) bool {
	switch criticality {
	case "required", "preferred", "optional":
		return true
	default:
		return false
	}
}
