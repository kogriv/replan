# Functional planning and tracking design checklist

Date: 2026-07-04

Status: approved functional planning/tracking design.

Purpose:

Track whether the functional design of RePlan planning and execution tracking
is closed enough for implementation.

This is not the general product roadmap.

This checklist is specifically about the functional behavior of:

- planning;
- resource allocation;
- capacity;
- tracking;
- baseline/current/actual comparison;
- evidence;
- proposals;
- reporting;
- PM-system interaction.

## Closure rule

The functional design of planning/tracking is closed only when every section
below is one of:

- approved and documented;
- explicitly deferred with rationale;
- explicitly excluded from MVP with rationale.

All sections below are now approved and documented.

## Status legend

- `pending` — not discussed yet;
- `in discussion` — currently being discussed;
- `approved` — decision accepted and documented;
- `amended` — accepted after changes and documented;
- `deferred` — intentionally postponed;
- `excluded from MVP` — explicitly outside MVP with rationale;
- `rejected` — explicitly rejected.

## A. Planning lifecycle

Status: approved.

Questions:

- How does a task enter RePlan?
- When does it become part of a baseline?
- When does it become part of the current plan?
- When is a plan snapshot created?
- What changes baseline, and what never changes baseline?
- What is the difference between re-planning and scenario planning?

Expected deliverables:

- [`docs/spec/planning-lifecycle.md`](planning-lifecycle.md)
- `docs/spec/canonical-model.md`
- `docs/spec/scenario-model.md`

Decision notes:

- Approved in [`planning-lifecycle.md`](planning-lifecycle.md).
- RePlan is a planning layer above external project-management systems, not a
  replacement task tracker.
- The lifecycle separates `baseline`, `current plan`, `actuals` and `scenario`.
- Baseline changes only through an explicit new baseline operation.
- Current plan is mutable and represents the current working intent.
- Actuals are execution facts and do not rewrite baseline.
- Scenarios are temporary alternative planning branches and do not mutate
  accepted actuals, historical evidence or old baselines.

## B. Tracking lifecycle

Status: approved.

Questions:

- How does work start?
- How does actual progress get reported?
- Which events become evidence?
- Which events create proposals?
- Which events can update actual state directly?
- What requires human/operator confirmation?
- What is reported as of `status_date`?

Expected deliverables:

- `docs/spec/tracking-events.md`
- `docs/spec/canonical-model.md`

Decision notes:

- Approved in [`tracking-events.md`](tracking-events.md).
- Tracking lifecycle is `external event -> normalized event -> evidence ->
  actual/proposal/warning/no-op -> accepted change -> current plan and reports`.
- External events are evidence, not automatic truth.
- Pull requests, delivery photos, invoices, calendar entries, agent reports and
  sensor readings may support tracking, but do not automatically complete work.
- RePlan supports code and offline/non-code tasks through the same tracking
  model.
- Offline tasks can use evidence such as photos, receipts, delivery notes,
  checklists, operator confirmations, inspection records, telemetry and manual
  progress reports.
- Baseline is not changed by tracking events.
- Reports must support `status_date`.
- Exact internal task statuses remain open under item `C. Task state model`.

## C. Task state model

Status: approved.

Questions:

- What task statuses does RePlan understand internally?
- How are OpenProject statuses mapped?
- What is the difference between `in_progress`, `review_ready`,
  `proposed_done`, `accepted`, `blocked`, `cancelled`?
- Can RePlan have a tracking status different from the PM-system status?
- Which statuses are facts and which are proposals?

Expected deliverables:

- [`docs/spec/task-state-model.md`](task-state-model.md)
- `docs/spec/canonical-model.md`
- `docs/spec/openproject-adapter.md`
- `docs/spec/tracking-events.md`

Decision notes:

- Approved in [`task-state-model.md`](task-state-model.md).
- RePlan has a small internal tracking status model:
  `not_started`, `ready`, `in_progress`, `blocked`, `review_ready`,
  `accepted`, `cancelled`.
- RePlan does not copy OpenProject, GitHub, GitLab, Plane, Jira or any other
  source-system status model.
- Source status is evidence or mapping hint, not automatic truth.
- Tracking status is the accepted RePlan task state.
- Proposed status is separate from tracking status.
- `done` is not an approved internal status because it is ambiguous.
- Complex situations are represented through flags, warnings, blockers,
  evidence and proposals instead of many compound statuses.

## D. Resource demand model

Status: approved.

Questions:

- Is a task assigned to one resource or can it demand several resources?
- How does `assignee` differ from `resource_demands`?
- How are reviewer time, AI-agent time, service quota, budget and environment
  reservations represented?
- Which resource demands are required and which are optional?
- How are planned, reserved, consumed and remaining amounts represented?

Expected deliverables:

- [`docs/spec/resource-demand-model.md`](resource-demand-model.md)
- `docs/spec/canonical-model.md`
- `docs/spec/workload-calculation.md`

Decision notes:

- Approved in [`resource-demand-model.md`](resource-demand-model.md).
- `assignee` is a PM-system compatibility hint, not the core RePlan planning
  model.
- `resource_demands[]` is the core model for task resource requirements.
- A task may demand multiple resources: implementer time, reviewer time,
  AI-agent time, API quota, rate limit, subscription capacity, budget,
  environment, equipment, material, location, vendor service or another
  constraining resource.
- A demand may point to a concrete resource, a pool or an unresolved resource
  need.
- Planned, remaining, reserved and actual amounts are separate fields.
- Units are not limited to hours.
- Criticality supports `required`, `preferred`, `optional` and `substitute`.
- MVP should avoid a full assignment solver and represent alternatives mostly
  through scenarios.

## E. Capacity and availability rules

Status: approved.

Questions:

- What is a working day?
- Is capacity daily, weekly, monthly or arbitrary period-based?
- How are vacations, absences and reduced days represented?
- How are AI-agent capacity and service quotas represented?
- What is the minimal calendar model for MVP?
- What is explicitly not supported in MVP?

Expected deliverables:

- [`docs/spec/capacity-availability-rules.md`](capacity-availability-rules.md)
- `docs/spec/canonical-model.md`
- `docs/spec/workload-calculation.md`

Decision notes:

- Approved in [`capacity-availability-rules.md`](capacity-availability-rules.md).
- Resource demand describes what a task needs.
- Resource capacity describes how much a resource can provide.
- Resource availability describes when that capacity can be used.
- Reservation/allocation describes what is already occupied by the plan.
- Actual consumption describes what was actually consumed.
- Capacity is generic and is not limited to human hours.
- A working day is not hard-coded as 8 hours; for MVP, a working day is a date
  with positive relevant capacity.
- Exceptions are required from the beginning.
- Missing capacity is unknown capacity, not infinite capacity, and should
  produce warnings.
- MVP should support periodic capacity, windows, exceptions, reservations,
  quotas, rate limits, concurrent slots, stock, budget and warnings.

## F. Workload calculation rules

Status: approved.

Questions:

- How is remaining work spread across dates?
- What happens if a task has no dates?
- What happens if a task has no estimate?
- How is overload calculated?
- How is underuse calculated?
- How are non-hour resources calculated?
- What warnings are generated?

Expected deliverables:

- [`docs/spec/workload-calculation.md`](workload-calculation.md)

Decision notes:

- Approved in [`workload-calculation.md`](workload-calculation.md).
- Workload calculation in MVP is deterministic rule-based reporting, not an
  automatic optimizer.
- Workload is calculated by distributing resource demands into buckets and
  comparing them with capacity.
- MVP bucket types are day, period/month, window and unscheduled.
- MVP spread modes are `even`, `contiguous` and `manual`.
- If a required demand has no dates, it goes to `unscheduled` and generates a
  warning.
- If a required demand has no estimate, RePlan must not assume zero.
- Forecast total defaults to `actual_amount + remaining_amount`.
- Unknown capacity is not treated as infinite capacity.
- Reports must support `status_date`.
- Structured warnings are required for human review and AI-CLI agents.

## G. Planned/current/actual comparison

Status: approved.

Questions:

- What exactly is baseline?
- What exactly is current plan?
- What exactly is actual execution?
- How are effort variance, schedule variance, quota variance and budget
  variance calculated?
- What is a forecast total?
- How are remaining work and actual consumption combined?

Expected deliverables:

- [`docs/spec/planned-current-actual-comparison.md`](planned-current-actual-comparison.md)
- `docs/spec/canonical-model.md`
- `docs/spec/workload-calculation.md`
- `docs/spec/tracking-events.md`

Decision notes:

- Approved in
  [`planned-current-actual-comparison.md`](planned-current-actual-comparison.md).
- RePlan separates `baseline`, `current`, `actual` and `forecast`.
- Baseline is an approved snapshot.
- Current plan is the mutable working plan.
- Actuals are accepted execution facts.
- Forecast defaults to `actual_amount + remaining_amount`.
- Variance compares current or forecast values with baseline.
- Capacity shortage compares load or forecast with available capacity.
- Variance and capacity shortage are different signals.
- Primary effort variance in MVP is `forecast_total -
  baseline_planned_amount`.
- Primary schedule variance in MVP is forecast finish minus baseline finish.
- MVP should not implement full Earned Value Management.

## H. Dependency and deadline behavior

Status: approved.

Questions:

- Are dependencies part of MVP?
- Which dependency warnings are required?
- Do dependencies affect scheduling or only warnings?
- How is deadline risk calculated?
- What is the minimal "this deadline looks impossible" heuristic?

Expected deliverables:

- [`docs/spec/dependency-deadline-behavior.md`](dependency-deadline-behavior.md)
- `docs/spec/workload-calculation.md`
- later `docs/spec/taskjuggler-adapter.md`

Decision notes:

- Approved in [`dependency-deadline-behavior.md`](dependency-deadline-behavior.md).
- Dependencies and deadlines are part of MVP.
- MVP dependencies affect warnings and feasibility checks.
- MVP dependencies do not automatically reschedule the plan.
- Dependency types include `finish_to_start`, `blocks`,
  `requires_input_from`, `requires_resource_from` and `external_dependency`.
- Minimum deadline risk is forecast finish after deadline.
- Missing forecast, capacity, remaining work or dependency state should produce
  unknown-risk warnings instead of false confidence.
- Minimal impossible-deadline heuristic is based on earliest possible finish
  using known remaining work, capacity, dependencies and windows.
- Advanced dependency scheduling can later be delegated to TaskJuggler or
  another engine.

## I. Scenario behavior

Status: approved.

Questions:

- What can a scenario change?
- Can a scenario change actuals or only plan assumptions?
- How does scenario comparison work?
- Can a scenario be created from tracking events?
- What makes a scenario approved, rejected or applied?

Expected deliverables:

- [`docs/spec/scenario-model.md`](scenario-model.md)

Decision notes:

- Approved in [`scenario-model.md`](scenario-model.md).
- A scenario is an isolated what-if overlay.
- A scenario is a base reference plus explicit changes plus calculated result.
- Scenarios can change planning assumptions.
- Scenarios cannot change accepted actuals, historical evidence, old baselines,
  source-system history, immutable event records, accepted status history or
  audit log.
- Scenario calculation is read-only.
- Scenario application is explicit.
- Accepting a scenario is not the same as applying it.
- Applying a scenario changes current plan or creates current-plan proposals,
  not baseline.
- Creating a new baseline after scenario application is a separate explicit
  action.
- AI agents may create, calculate and propose scenarios by default, but should
  not silently apply them.

## J. Evidence and proposal behavior

Status: approved.

Questions:

- What evidence types exist?
- How does evidence attach to tasks/resources/actuals?
- How does a proposal become accepted?
- Can agents create proposals directly?
- What proposal types are allowed in MVP?
- What must be audited?

Expected deliverables:

- [`docs/spec/evidence-proposal-behavior.md`](evidence-proposal-behavior.md)
- `docs/spec/tracking-events.md`
- [`docs/security/access-and-actor-model.md`](../security/access-and-actor-model.md)

Decision notes:

- Approved in [`evidence-proposal-behavior.md`](evidence-proposal-behavior.md).
- Evidence explains why RePlan believes, suggests or warns about something.
- Proposal suggests changes without making them true.
- Accepted change mutates RePlan state and creates audit.
- Evidence does not automatically change state.
- Evidence can attach to tasks, resources, demands, actuals, proposals,
  dependencies, deadlines, scenarios, warnings, baseline decisions and current
  plan changes.
- AI agents are propose-only by default.
- Only low-risk trusted changes may apply automatically in MVP.
- Audit is required for accepted/rejected/applied proposals and important state
  changes.

## K. Report behavior

Status: approved.

Questions:

- What must the weekly report show?
- What must the scenario report show?
- What must machine-readable JSON include?
- What must CSV include?
- Is static HTML required in MVP or only optional?
- Which report sections are required for human validation?

Expected deliverables:

- [`docs/spec/report-behavior.md`](report-behavior.md)
- `docs/spec/cli.md`
- `docs/spec/storage-layout.md`
- `docs/spec/mvp-boundary.md`

Decision notes:

- Approved in [`report-behavior.md`](report-behavior.md).
- Reports are a primary RePlan interface for human validation and AI-CLI
  operation.
- JSON is the canonical machine-readable report output.
- Markdown is the primary human-readable output.
- CSV is required for tabular export.
- Static HTML is optional in MVP.
- Weekly report must show status, workload, overloads, shortages, deadline
  risks, missing data, proposals, evidence and decisions needed.
- Scenario report must show explicit changes, calculation results, diffs,
  warning deltas and apply preview.
- Reports must not hide unknown values.

## L. PM-system interaction behavior

Status: approved.

Questions:

- Which data is owned by OpenProject?
- Which data is owned by RePlan?
- What happens when OpenProject status differs from RePlan tracking status?
- How are links between OpenProject, GitHub/GitLab and RePlan represented?
- What is read-only first?
- What is write-back later?

Expected deliverables:

- [`docs/spec/pm-system-interaction.md`](pm-system-interaction.md)
- `docs/spec/openproject-adapter.md`
- `docs/spec/tracking-events.md`

Decision notes:

- Approved in [`pm-system-interaction.md`](pm-system-interaction.md).
- RePlan is adjacent to PM systems, not a replacement task tracker.
- OpenProject is the first likely real PM substrate, but not RePlan's domain
  model.
- PM systems own ordinary task management.
- RePlan owns planning intelligence.
- Source status and RePlan tracking status are separate.
- Source mismatch should be shown as warning.
- MVP integration is read-only first.
- Write-back later must be proposal-driven, dry-run capable, explicit and
  audited.
- Refresh must not silently overwrite local planning assumptions, actuals,
  baselines or audit.

## Current next item

All functional planning/tracking checklist items A-L are approved and
documented.

```text
Functional planning/tracking design closed for MVP specification purposes.
```
