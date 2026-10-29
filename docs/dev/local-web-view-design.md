# Local web view decision

Date: 2026-10-28

Status: proposed; waits for the operator's approval
(`task.self.design-local-web-view`). Implementation is
`task.self.implement-local-web-view`, rescoped by this decision.

## Question

The public roadmap lists "a small web view: the weekly heatmap, scenario
comparison and proposal review in a browser, only once the CLI reports have
shown which views matter". Which views have mattered, and does RePlan need a
served web application for them?

## What was used during the dogfood cycle

RePlan records no usage telemetry, so this is read from the work itself,
July to October 2026:

- **The operator** looked at RePlan through OpenProject: the RePlan project
  page and the per-task panel of the read-only plugin, refreshed by the
  overlay publish after each task. Decisions (accept, approve a baseline,
  choose an option) were given in chat and recorded by the agent through the
  CLI and helpers.
- **The agents** used CLI JSON only: `replan next`, `report state`,
  `report estimates`, `validate`, the drift checks and the helper envelopes.
  None of them read the HTML dashboard.
- **The static dashboard** (`index.html`, `gantt.html`) was used in the
  plugin UI audits of September and October as the reference rendering, not
  as a working surface.
- **Scenario comparison** happened in the CLI (`scenario diff`), mostly for
  done-early scenarios that recorded actual dates, now replaced by
  `replan task record-actual-window`.
- **Proposal review** was dependency proposals after each acceptance, a few
  per task, accepted in the CLI.
- **Weekly load** questions were answered from `workload --demands` and the
  overload explainer; the OpenProject plugin shows weekly load per resource.

## Decision

Do not build a served web application now. Complete the static dashboard
instead with the pages its own design already specifies
([`static-observability-dashboard.md`](../spec/static-observability-dashboard.md))
and that have not been built:

1. `workload.html`: the weekly heatmap, resource by week, with the demands
   behind each overloaded cell (the overload explainer's data);
2. `scenarios.html`: each scenario's changes and its diff against the
   current plan, with the `scenario apply` command to copy;
3. `proposals.html`: the open proposals with their evidence and impact, and
   the `proposal accept` and `reject` commands to copy;
4. `estimates.html`: the estimate accuracy of each sealed baseline, and the
   calibrations once `task.self.implement-estimate-calibration` lands.

All four stay read-only, work from disk, and need no login. Accepting and
applying remain CLI commands run by a person or an agent with an actor id,
under the same approval rules.

## Why not a served application

- The approval rules (actor, evidence, dry run, audit) live in the CLI. A
  browser with write buttons needs a local server, a way to say who clicks,
  and protection against cross-site requests; that is a second safety
  surface to keep equal to the first.
- The operator's working surface is OpenProject. A view the operator would
  open daily belongs in the plugin, which already has permissions, sign-in
  and the task context.
- Nothing in the dogfood cycle was slow or error-prone because a decision
  had to be given in chat or the CLI. The readable summaries were what was
  missing, and static pages give them.

## When to revisit

Revisit a served view when either holds:

- a second person approves proposals or scenarios regularly and the chat
  plus CLI path becomes the bottleneck; or
- a team without OpenProject uses RePlan, so the plugin is not their
  surface.

## Rescoping the implementation task

`task.self.implement-local-web-view` becomes "Add the workload, scenario,
proposal and estimate pages to the static dashboard", with HTML goldens like
the existing pages, the raw JSON each page reads, and the smoke generating
them from a temporary copy of `examples/replan-self`. Its estimate stays.

## Operator decisions

Agreed by the operator on 2026-10-29:

1. Static pages instead of a served view for now.
2. None of the pages goes into the OpenProject plugin yet; the plugin's
   resource view covers weekly load.
