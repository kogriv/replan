# Tracking events and execution evidence

Date: 2026-07-04

Status: approved tracking lifecycle.

Note:

The tracking lifecycle is approved here. The exact internal task state model is
defined in [`task-state-model.md`](task-state-model.md).

Purpose:

Define how RePlan can track plan execution through events from PM systems,
git hosts, CI systems, AI agents, gateways and manual updates.

This document extends the MVP boundary with a key principle:

```text
events are evidence, not automatic truth
```

## Core decision

RePlan should support event-assisted execution tracking.

It should not blindly infer final progress from external events.

Correct model:

```text
OpenProject / Plane / GitHub / GitLab / CI / agents / gateways
    emit or expose events

RePlan
    imports events
    links events to tasks/resources
    turns events into evidence
    may create tracking proposals
    updates actual state only through confirmation or trusted policy
    recalculates variance and forecast
```

Incorrect model:

```text
pull request exists
    -> task is done
    -> hours are spent
    -> plan is changed
```

That would recreate the same class of false-progress and phantom-DONE problems
that OrCode is meant to avoid.

## Relationship to PM systems

RePlan should not be understood in isolation from a task/PM substrate.

The expected product shape is:

```text
PM system
    task identity, assignment, status, dates, comments

Git host
    branches, commits, pull requests, merge requests, reviews

CI / automation
    checks, test runs, build artifacts, previews

Agent runtime
    tool execution, token usage, cost, proposed updates, evidence

RePlan
    resource plan, baseline/current/actual comparison, capacity,
    scenarios, variance, tracking proposals, reports
```

OpenProject is the first likely PM substrate.

YAML fixtures are acceptable for MVP because they reduce integration cost while
the model is being proven. But the product design assumes that RePlan becomes
more useful when connected to a real task system.

Rule:

```text
MVP may start with YAML fixtures.
Product design assumes PM/task-substrate operation.
```

## Source of truth boundaries

### PM system owns task identity

OpenProject or another PM system should own:

- task ID;
- task title;
- task status;
- assignee;
- task comments;
- high-level dates where available;
- work package / issue lifecycle.

### Git host owns code evidence

GitHub or GitLab should own:

- branch;
- commit;
- pull request or merge request;
- review state;
- merge state;
- CI check linkage.

### CI owns build/test evidence

CI should own:

- test run status;
- build status;
- preview deployment status;
- logs and artifacts;
- check URLs.

### Agent runtime owns agent consumption evidence

Agent runtime or wrapper should own:

- model/tool used;
- elapsed runtime;
- token usage;
- cost;
- output artifacts;
- proposed status;
- evidence references.

### RePlan owns resource/tracking intelligence

RePlan should own:

- baseline plan snapshot;
- current plan snapshot;
- resource capacity model;
- planned reservations;
- actual consumption records;
- remaining work records;
- variance calculations;
- evidence linkage;
- tracking proposals;
- scenario comparisons;
- reports;
- audit log.

## Tracking lifecycle

The approved tracking lifecycle is:

```text
external event
  -> normalized event
  -> evidence
  -> actual / proposal / warning / no-op
  -> accepted change
  -> current plan and reports
```

The lifecycle deliberately separates signal from truth.

An external event is not automatically an accepted fact.

Evidence is not automatically completion.

Agent output is not automatically an accepted update.

A pull request, delivery photo, invoice, message, sensor reading or calendar
event may all be useful evidence, but each still needs policy and context before
it changes the resource plan.

### 1. Baseline exists

The plan is captured:

```yaml
task_id: openproject:123
resource_id: dev_ivan
planned_effort: 12h
start_date: 2026-07-06
finish_date: 2026-07-09
deadline: 2026-07-12
```

### 2. Work starts

OpenProject event:

```yaml
source: openproject
event_type: status_changed
task_id: openproject:123
actor_id: dev_ivan
timestamp: 2026-07-06T10:00:00+07:00
from: ready
to: in_progress
confidence: observed
```

RePlan records:

- evidence that work started;
- possible `actual_start`;
- no automatic spent-hours conclusion.

### 3. Execution evidence appears

GitHub event:

```yaml
source: github
event_type: pull_request_opened
task_id: openproject:123
actor_id: dev_ivan
timestamp: 2026-07-06T15:00:00+07:00
evidence:
  type: pull_request
  url: https://github.com/example/project/pull/45
confidence: observed
```

RePlan records:

- PR evidence link;
- possible `review_ready` proposal;
- no automatic `done`.

### 4. Actual usage is reported or inferred

Manual or PM time tracking:

```yaml
source: manual
event_type: actual_work_reported
task_id: openproject:123
resource_id: dev_ivan
actor_id: dev_ivan
timestamp: 2026-07-06T18:00:00+07:00
actual:
  unit: hours
  amount: 5
remaining:
  unit: hours
  amount: 7
confidence: confirmed
```

Agent runtime:

```yaml
source: agent_runtime
event_type: resource_consumed
task_id: openproject:456
resource_id: agent_coder_1
actor_id: agent_coder_1
timestamp: 2026-07-06T18:00:00+07:00
actual:
  - unit: agent_hours
    amount: 2
  - unit: tokens
    amount: 82000
  - unit: usd
    amount: 1.40
confidence: observed
```

### 5. RePlan creates tracking state

RePlan may update actual state directly only if event policy allows it.

Examples:

- manual confirmed actual work can update actual consumption;
- OpenProject logged time can update actual consumption;
- agent token usage can update actual token consumption;
- PR opened should create evidence and maybe proposal, not `done`;
- CI passed should create evidence and maybe proposal, not `accepted`.

### 6. RePlan reports variance

Example:

```text
OP-123
  baseline effort: 12h
  actual spent: 5h
  remaining: 10h
  forecast total: 15h
  effort variance: +3h
  baseline finish: 2026-07-09
  current forecast finish: 2026-07-10
  schedule variance: +1 day
  evidence: PR #45
  decision needed: review or replan
```

## Event types

### PM/task events

- `task_created`;
- `task_updated`;
- `status_changed`;
- `assignee_changed`;
- `priority_changed`;
- `estimate_changed`;
- `dates_changed`;
- `deadline_changed`;
- `dependency_changed`;
- `progress_updated`;
- `time_logged`;
- `comment_added`;
- `evidence_attached`.

### Git events

- `branch_created`;
- `commit_pushed`;
- `pull_request_opened`;
- `pull_request_updated`;
- `pull_request_reviewed`;
- `pull_request_merged`;
- `merge_request_opened`;
- `merge_request_updated`;
- `merge_request_reviewed`;
- `merge_request_merged`;
- `tag_created`;
- `release_created`.

### CI and delivery events

- `ci_started`;
- `ci_passed`;
- `ci_failed`;
- `build_created`;
- `artifact_uploaded`;
- `preview_deployed`;
- `preview_failed`;
- `smoke_test_passed`;
- `smoke_test_failed`;
- `deployment_started`;
- `deployment_finished`.

### Agent events

- `agent_claimed_task`;
- `agent_started_task`;
- `agent_paused_task`;
- `agent_proposed_update`;
- `agent_proposed_done`;
- `agent_attached_evidence`;
- `agent_reported_consumption`;
- `agent_failed`;
- `agent_handed_off`.

### Resource/gateway events

- `resource_reserved`;
- `resource_reservation_rejected`;
- `resource_consumed`;
- `quota_read`;
- `quota_exceeded`;
- `budget_warning`;
- `budget_exceeded`;
- `subscription_limit_reached`;
- `environment_reserved`;
- `environment_released`;
- `equipment_reserved`;
- `equipment_released`.

## Event confidence

RePlan should distinguish confidence levels.

### observed

The event was observed from an external source.

Examples:

- OpenProject status changed;
- GitHub PR opened;
- CI passed;
- provider reported token usage.

### inferred

RePlan inferred a possible tracking update from one or more events.

Example:

```text
Task moved to In Progress at 10:00 and PR opened at 15:00.
Suggested actual work window: 5 elapsed hours.
Confidence: low.
```

### reported

An actor reported progress.

Example:

```text
Ivan reports 5h spent and 7h remaining.
```

### confirmed

A trusted actor or policy confirmed the update.

Example:

```text
Operator accepted agent's proposed_done after reviewing PR and tests.
```

## Tracking proposals

Some events should create proposals instead of changing actual state.

Example derived actual proposal:

```yaml
id: proposal-001
type: actual_work_estimate
task_id: openproject:123
resource_id: dev_ivan
derived_from:
  - event-001
  - event-002
suggested_actual:
  unit: hours
  amount: 5
confidence: low
requires_confirmation: true
```

Example review-ready proposal:

```yaml
id: proposal-002
type: status_change
task_id: openproject:123
suggested_status: review_ready
derived_from:
  - pull_request_opened
  - ci_passed
confidence: medium
requires_confirmation: true
```

## Automatic updates allowed in MVP

MVP may automatically record:

- imported events;
- evidence links;
- observed agent runtime consumption;
- observed token/cost consumption;
- observed CI status;
- observed PR/MR status;
- manual confirmed actual updates;
- OpenProject logged time if imported from a trusted field.

MVP may automatically calculate:

- planned vs actual variance;
- forecast total from actual plus remaining;
- schedule variance against baseline/current plan;
- missing progress updates;
- no-evidence warnings;
- slipping task warnings.

## Automatic updates forbidden in MVP

MVP should not automatically:

- mark a task done because a PR exists;
- mark a task done because CI passed;
- close or reopen tasks in OpenProject;
- change OpenProject status;
- change baseline;
- assign human hours from elapsed time without confirmation;
- assign payroll/timesheet values;
- infer human productivity;
- move deadlines;
- apply rescheduling;
- write back to PM system without explicit approval.

## Status date

Tracking reports should support a `status_date`.

Meaning:

```text
show plan state as of this date
```

Questions answered:

- what should have been done by this date;
- what was actually done by this date;
- what is late;
- what lacks evidence;
- what changed since baseline;
- what requires decision now.

Example:

```bash
replan report weekly --status-date 2026-07-08
```

## CLI implications

Candidate MVP commands:

```bash
replan event import --from openproject --since 2026-07-01 --format json
replan event import --from github --since 2026-07-01 --format json
replan event list --task openproject:123 --format json
replan update progress openproject:123 --spent 5h --remaining 7h --actor dev_ivan
replan evidence attach openproject:123 --type pull_request --url https://github.com/example/project/pull/45
replan tracking proposals --format markdown
replan tracking accept proposal-001 --actor operator --dry-run
replan report weekly --status-date 2026-07-08 --format markdown
```

MVP does not need all commands immediately, but the model should make them
possible.

## Report implications

Weekly report should include tracking sections:

```text
Planned vs actual
    resource workload variance

Slipping tasks
    baseline/current/actual date differences

Missing progress updates
    tasks in progress without recent actuals/evidence

Evidence waiting for review
    PRs, CI, previews, agent outputs

Agent proposals
    proposed_done, estimate changes, remaining-work updates

Decisions needed
    replan, review, approve, reject, ask for update
```

## OpenProject-specific tracking

OpenProject can provide:

- task status;
- assignee;
- start/finish/due dates;
- estimates;
- spent time if used;
- comments;
- relations/dependencies;
- custom fields;
- attachments/links.

RePlan should use OpenProject as task truth and status source, but should not
assume OpenProject has all resource-tracking intelligence.

Initial OpenProject tracking integration should be read-only:

- import current task state;
- import changes on refresh;
- import logged time if present;
- import comments/evidence links if configured;
- compare with RePlan baseline/current/actual state.

Write-back comes later and must be explicit.

## GitHub/GitLab-specific tracking

Git hosts can provide execution evidence:

- branch exists;
- commits pushed;
- pull request opened;
- review requested;
- review approved;
- CI status;
- merge status.

Mapping to task IDs can happen through:

- branch naming;
- PR title/body;
- issue/work package links;
- commit messages;
- OpenProject-GitHub integration metadata;
- explicit RePlan evidence attach command.

Git evidence should not automatically equal task completion.

## AI-agent tracking

AI agents should report:

- task claimed;
- task started;
- task paused;
- proposed estimate change;
- proposed remaining work;
- consumed runtime;
- consumed tokens;
- consumed budget;
- generated evidence;
- proposed done;
- handoff notes.

Agent outputs should be policy-constrained.

Default rule:

```text
agent may propose
operator or trusted policy confirms
```

## Offline or non-code tasks

Not every task ends with a pull request.

RePlan must support software tasks and offline tasks through the same tracking
model.

Examples of offline or non-code tasks:

- logistics;
- repair;
- construction;
- watering or maintenance;
- procurement;
- cable laying;
- installation;
- cleaning;
- inspection;
- delivery;
- inventory;
- legal or accounting preparation;
- physical meeting;
- medical or wellness appointment such as massage;
- vendor negotiation;
- equipment setup;
- field work;
- travel;
- training.

The key point is:

```text
offline tasks still have resources, capacity, planned work, actual work,
evidence, remaining work, deadlines and acceptance criteria.
```

They simply use different evidence sources.

### Offline task shape

An offline task may have:

- human resources;
- vehicle resources;
- equipment resources;
- tool resources;
- material resources;
- location resources;
- environment resources;
- access-window constraints;
- vendor dependencies;
- procurement dependencies;
- weather constraints;
- service-window constraints;
- safety constraints;
- budget constraints;
- route constraints;
- permit or approval constraints;
- physical completion criteria.

Examples:

```yaml
task_id: openproject:repair-17
task_type: offline_repair
title: Replace broken pump
resource_demands:
  - resource_id: plumber_alex
    unit: hours
    planned: 4
  - resource_id: service_vehicle_1
    unit: hours
    planned: 4
  - resource_id: spare_pump_model_x
    unit: pieces
    planned: 1
constraints:
  location: site_north
  access_window: 2026-07-06T09:00:00+07:00/2026-07-06T18:00:00+07:00
  requires_photo_evidence: true
```

```yaml
task_id: local:watering-2026-07-04
task_type: maintenance
title: Water greenhouse section A
resource_demands:
  - resource_id: worker_ivan
    unit: hours
    planned: 1
  - resource_id: water_supply_greenhouse
    unit: liters
    planned: 300
completion_criteria:
  - checklist_item: section_a_watered
  - checklist_item: no_visible_dry_zones
```

```yaml
task_id: openproject:procurement-44
task_type: procurement
title: Buy cable and connectors
resource_demands:
  - resource_id: buyer_anna
    unit: hours
    planned: 2
  - resource_id: project_budget
    unit: rub
    planned: 18000
completion_criteria:
  - receipt_attached
  - materials_delivered_to_site
```

### Offline evidence types

Offline/non-code tasks may use other milestones and evidence types:

- document delivered;
- meeting completed;
- vendor response received;
- design approved;
- equipment reserved;
- photo/report attached;
- manual checklist completed;
- operator confirmation.

Additional offline evidence types:

- photo;
- video;
- signed act;
- receipt;
- invoice;
- delivery note;
- geolocation check-in;
- geolocation check-out;
- calendar event;
- call log;
- chat message;
- email;
- checklist;
- inspection record;
- sensor reading;
- meter reading;
- equipment telemetry;
- access-control event;
- route log;
- material issue record;
- warehouse movement;
- payment confirmation;
- permit approval;
- customer acceptance note.

Tracking model should support arbitrary evidence types:

```yaml
evidence:
  type: document
  url: file-or-web-link
```

```yaml
evidence:
  type: operator_confirmation
  note: "Vendor contract reviewed and accepted"
```

More offline examples:

```yaml
source: manual
event_type: evidence_attached
task_id: openproject:repair-17
actor_id: plumber_alex
timestamp: 2026-07-06T14:20:00+07:00
evidence:
  type: photo
  url: file:///evidence/repair-17/installed-pump.jpg
  note: Pump installed and leak test completed
confidence: reported
```

```yaml
source: procurement_system
event_type: material_delivered
task_id: openproject:procurement-44
actor_id: supplier_acme
timestamp: 2026-07-05T12:10:00+07:00
evidence:
  type: delivery_note
  url: https://vendor.example/delivery/7781
actual:
  - resource_id: cable_cat6
    unit: meters
    amount: 500
confidence: observed
```

```yaml
source: manual
event_type: actual_work_reported
task_id: local:watering-2026-07-04
resource_id: worker_ivan
actor_id: worker_ivan
timestamp: 2026-07-04T19:00:00+07:00
actual:
  unit: hours
  amount: 1.25
remaining:
  unit: hours
  amount: 0
evidence:
  type: checklist
  checklist:
    section_a_watered: true
    no_visible_dry_zones: true
confidence: reported
```

```yaml
source: manual
event_type: service_completed
task_id: openproject:wellness-09
actor_id: operator
timestamp: 2026-07-04T17:00:00+07:00
actual:
  - resource_id: therapist_maria
    unit: hours
    amount: 1
  - resource_id: wellness_budget
    unit: rub
    amount: 3500
evidence:
  type: operator_confirmation
  note: Session completed and paid
confidence: confirmed
```

### Offline actuals

For offline tasks, actuals may include:

- human hours;
- equipment hours;
- vehicle hours;
- room or site occupancy time;
- material quantity consumed;
- water quantity consumed;
- fuel quantity consumed;
- money spent;
- vendor service units;
- distance traveled;
- number of items moved;
- number of units installed;
- number of inspections completed;
- blocked time;
- waiting time;
- rework time.

The unit does not have to be `hours`.

The general resource model must allow different units because resource planning
for physical work often depends on materials, tools, locations and windows, not
only people.

### Offline progress reporting

Offline task progress can be reported through:

- manual CLI update;
- operator confirmation;
- mobile-friendly form later;
- imported PM-system status;
- imported time entry;
- imported checklist;
- imported procurement record;
- imported calendar event;
- imported delivery event;
- imported equipment telemetry;
- imported sensor reading;
- imported file/photo evidence;
- agent-created proposal based on available evidence.

For MVP, the minimum viable mechanism is manual CLI update plus evidence link.

Example:

```bash
replan update progress openproject:repair-17 \
  --spent 3h \
  --remaining 1h \
  --actor plumber_alex \
  --evidence photo:file:///evidence/repair-17/pump-installed.jpg
```

The exact command syntax is not fixed here, but the lifecycle must support this
operation.

### Offline completion

Offline completion should be explicit.

Possible completion signals:

- checklist completed;
- required evidence attached;
- operator accepted result;
- customer accepted result;
- inspector accepted result;
- vendor delivered required item;
- payment completed;
- site restored;
- equipment returned;
- no remaining work.

As with code tasks, evidence alone should not always mean completion.

Examples:

- a photo of a repaired pump does not prove that it passed a leak test;
- a receipt does not prove that materials arrived at the correct site;
- a calendar event does not prove that a meeting achieved its goal;
- a sensor reading may prove water flow but not necessarily correct watering;
- a massage appointment in a calendar does not prove that the session was
  completed and paid.

Therefore RePlan should usually turn offline completion signals into proposals
unless the policy says the source is trusted.

### Offline policies

Offline events can be classified by policy:

- trusted direct actual;
- reported actual requiring confirmation;
- evidence only;
- proposal trigger;
- warning trigger;
- ignored event.

Examples:

- an operator-confirmed CLI actual can directly update actuals;
- a procurement system delivery note may directly update material quantity if
  the integration is trusted;
- a worker's message may create a progress proposal;
- a photo may attach evidence and create an acceptance proposal;
- a calendar event may create evidence but should not automatically record
  completed work;
- a geolocation check-in may suggest start time but should not automatically
  infer productive hours.

### Offline non-goals

RePlan should not become a surveillance product.

For offline tasks, RePlan should not try to infer:

- worker productivity from location traces;
- payroll from elapsed presence;
- detailed employee monitoring;
- medical or personal service details beyond task-level planning facts;
- exact physical completion when the evidence is ambiguous.

The right MVP posture is:

```text
record reported/observed facts, preserve evidence, create proposals, ask for
confirmation when the change matters.
```

## MVP boundary

MVP should implement tracking lightly.

Required:

- actual consumption fields;
- remaining work field;
- status date in reports;
- evidence link model;
- manual progress update;
- event model in schema/design;
- tracking section in weekly report.

Optional in first implementation:

- real OpenProject event import;
- real GitHub/GitLab event import;
- proposal accept/reject workflow;
- agent runtime integration.

Later:

- webhooks;
- near-real-time sync;
- write-back;
- advanced policy;
- automatic source linking;
- richer event store;
- dashboards.

## Final position

RePlan should support automatic and interactive tracking by design.

But the trust model must remain conservative:

```text
external events create evidence and proposals
confirmed actuals update tracking state
reports show variance and decisions needed
write-back requires explicit approval
```
