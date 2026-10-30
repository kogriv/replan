package app

import (
	"fmt"

	"github.com/kogriv/replan/internal/domain"
)

// validateApprovingActor applies the local policy guard shared by commands
// that approve or apply reviewed state. Callers retain their command-specific
// rules about whether an actor is required for a preview.
func validateApprovingActor(ws domain.Workspace, actorID string) *Diagnostic {
	actor, ok := findActor(ws.Actors, actorID)
	if !ok {
		return &Diagnostic{
			Code: "actor_not_found", Message: fmt.Sprintf("actor %s not found", actorID), Severity: "error",
			Target: Target{Kind: "actor", ID: actorID},
		}
	}
	if ws.Policy.EffectiveAIAgentMutationMode() == domain.AIAgentMutationProposalOnly && actor.Kind != "human" {
		return &Diagnostic{
			Code:     "approver_not_human",
			Message:  fmt.Sprintf("approving actor %s is %s, not human under ai_agents.default_mutation_mode %s", actorID, actor.Kind, domain.AIAgentMutationProposalOnly),
			Severity: "error", Target: Target{Kind: "actor", ID: actorID},
		}
	}
	return nil
}

func appendNamedApprovingActorDiagnostic(errors *[]Diagnostic, ws domain.Workspace, actorID string) {
	if actorID == "" {
		return
	}
	if diagnostic := validateApprovingActor(ws, actorID); diagnostic != nil {
		*errors = append(*errors, *diagnostic)
	}
}
