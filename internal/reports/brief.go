package reports

import (
	"fmt"
	"io"
	"strings"
)

// NextBrief is the short operator view of a project-state overview: what
// waits on a person, what an agent can prepare, and one health line. It is
// built from the same DashboardOverview as the full state report.
type NextBrief struct {
	ProjectID       string          `json:"project_id"`
	ProjectName     string          `json:"project_name"`
	StatusDate      string          `json:"status_date,omitempty"`
	Week            string          `json:"week,omitempty"`
	WaitingOnPerson []NextBriefItem `json:"waiting_on_person"`
	AgentCanPrepare []NextBriefItem `json:"agent_can_prepare"`
	Health          NextBriefHealth `json:"health"`
}

type NextBriefItem struct {
	Code    string                    `json:"code"`
	What    string                    `json:"what"`
	Subject string                    `json:"subject,omitempty"`
	Target  string                    `json:"target,omitempty"`
	Date    string                    `json:"date,omitempty"`
	Command string                    `json:"command,omitempty"`
	Details []CalibrationDemandDetail `json:"details,omitempty"`
}

type NextBriefHealth struct {
	Tasks               int `json:"tasks"`
	ActiveTasks         int `json:"active_tasks"`
	OverloadedResources int `json:"overloaded_resources"`
	OpenProposals       int `json:"open_proposals"`
	Warnings            int `json:"warnings"`
	Errors              int `json:"errors"`
}

// nextBriefWhat is the short verb phrase shown in front of the subject.
var nextBriefWhat = map[string]string{
	"accept_or_return_task": "Accept or return",
	"do_human_work":         "Human work left on",
	"past_planned_finish":   "Past planned finish",
	"review_proposal":       "Decide on proposal",
	"review_overload":       "Overloaded resource",
	"resolve_dependency":    "Resolve dependency",
	"review_tracking":       "Check actuals of",
	"record_actual_window":  "Record actual dates of",
	"suggest_calibration":   "Calibrate estimate scope",
}

func BuildNextBrief(overview DashboardOverview) NextBrief {
	titles := map[string]string{}
	for _, task := range overview.Tasks {
		titles[task.ID] = task.Title
	}
	brief := NextBrief{
		ProjectID:       overview.ProjectID,
		ProjectName:     overview.ProjectName,
		StatusDate:      overview.StatusDate,
		Week:            overview.Week,
		WaitingOnPerson: []NextBriefItem{},
		AgentCanPrepare: []NextBriefItem{},
		Health: NextBriefHealth{
			Tasks:               overview.Health.TaskCount,
			ActiveTasks:         overview.Health.ActiveTasks,
			OverloadedResources: overview.Health.OverloadedResources,
			OpenProposals:       overview.Health.OpenProposals,
			Warnings:            overview.Health.WarningCount,
			Errors:              overview.Health.ErrorCount,
		},
	}
	for _, action := range overview.NextActions {
		what, known := nextBriefWhat[action.Code]
		if action.Code == "apply_calibration" {
			what, known = action.Label, true
		}
		if !known {
			// inspect_project and any future code without a short phrase.
			continue
		}
		item := NextBriefItem{
			Code:    action.Code,
			What:    what,
			Subject: titles[action.Target],
			Target:  action.Target,
			Date:    action.Date,
			Command: action.Command,
			Details: action.Details,
		}
		switch action.Actor {
		case NextActionActorPerson:
			brief.WaitingOnPerson = append(brief.WaitingOnPerson, item)
		case NextActionActorAgent:
			brief.AgentCanPrepare = append(brief.AgentCanPrepare, item)
		}
	}
	return brief
}

// WriteNextBriefText renders the brief for a terminal: no tables, task titles
// instead of ids where there is a title, ids on a second line for copying.
func WriteNextBriefText(w io.Writer, brief NextBrief) error {
	var b strings.Builder
	b.WriteString(brief.ProjectName)
	if brief.StatusDate != "" {
		fmt.Fprintf(&b, " | status date %s", brief.StatusDate)
	}
	if brief.Week != "" {
		fmt.Fprintf(&b, " | week %s", brief.Week)
	}
	b.WriteString("\n")
	writeNextBriefGroup(&b, "Waiting on a person", brief.WaitingOnPerson)
	writeNextBriefGroup(&b, "An agent can prepare", brief.AgentCanPrepare)
	h := brief.Health
	fmt.Fprintf(&b, "\nHealth: %d tasks, %d active, %d overloaded resources, %d open proposals, %d warnings, %d errors\n",
		h.Tasks, h.ActiveTasks, h.OverloadedResources, h.OpenProposals, h.Warnings, h.Errors)
	_, err := io.WriteString(w, b.String())
	return err
}

func writeNextBriefGroup(b *strings.Builder, title string, items []NextBriefItem) {
	fmt.Fprintf(b, "\n%s (%d)\n", title, len(items))
	if len(items) == 0 {
		b.WriteString("  nothing\n")
		return
	}
	for i, item := range items {
		if item.Code == "apply_calibration" {
			fmt.Fprintf(b, "  %d. %s\n", i+1, item.What)
			continue
		}
		subject := item.Subject
		if subject == "" {
			subject = item.Target
		}
		due := ""
		if item.Date != "" {
			due = " (due " + item.Date + ")"
		}
		fmt.Fprintf(b, "  %d. %s: %s%s\n", i+1, item.What, subject, due)
		if item.Subject != "" && item.Target != "" {
			fmt.Fprintf(b, "     %s\n", item.Target)
		}
	}
}
