# RePlan canonical model

Date: 2026-07-06

Status: approved implementation specification.

## Purpose

This document defines the first RePlan canonical model.

The canonical model is the internal RePlan contract between local files,
validation, workload calculation, reports, scenarios, evidence, proposals,
audit, source adapters, future TaskJuggler integration, future daemon, future
server API, future human user interface and AI CLI agents.

This document converts approved functional design into implementation-ready
entities and invariants.

This document does not define the final Go struct names, database tables,
storage file paths or exact CLI commands. Those belong to later documents:

- `docs/spec/storage-layout.md`;
- `docs/spec/cli.md`;
- `docs/spec/implementation-sequence.md`.

## Core decision

RePlan must have a source-agnostic canonical planning model.

Approved rule:

```text
external systems -> adapters -> RePlan canonical model -> calculations/reports/proposals
```

External systems are not the canonical model.

OpenProject, Plane, GitHub, GitLab, Jira, TaskJuggler, spreadsheets, local YAML
fixtures and future systems can all feed RePlan, but RePlan must not copy any
one system's schema as its core.

## Design principles

### Source-agnostic core

Core entities must not contain OpenProject-only, Plane-only, GitHub-only or
TaskJuggler-only behavior.

Allowed:

```yaml
source_refs:
  - source: openproject
    object_type: work_package
    object_id: "1234"
    url: "https://pm.example.test/work_packages/1234"
```

Not allowed in core:

```yaml
openproject_work_package_status_id: 7
```

Source-specific fields may be preserved in raw import snapshots or adapter
metadata, but calculation and reporting should work on canonical fields.

### Stable identifiers

Every persisted canonical object must have a stable identifier.

The identifier must not depend on line number, array position or generated
report order.

Examples:

```text
project.orcode-bootstrap
task.op.1234
resource.human.ivan
demand.task-op-1234.implementer
baseline.2026-07-06.initial
scenario.ivan-unavailable-week-28
evidence.github.pr.45
proposal.2026-07-06.adjust-estimate-op-1234
```

The exact identifier format may be constrained later by storage rules, but the
concept is mandatory.

### Explicit references

References must be explicit objects or explicit typed strings.

RePlan should not rely on fuzzy name matching.

Example:

```yaml
resource_ref:
  kind: resource
  id: resource.human.ivan
```

### Unknown is not zero

Missing data must not silently become zero, unlimited capacity, infinite quota
or no risk.

Examples:

```text
unknown remaining work != 0 remaining work
unknown capacity != infinite capacity
unknown deadline != no deadline
unknown rate limit != unlimited rate limit
unknown resource assignment != no resource needed
```

If unknown data affects planning, RePlan should emit structured warnings.

### Separate baseline, current, actual and forecast

RePlan must always separate:

```text
baseline = approved snapshot of planning commitments
current  = mutable working version of the plan
actual   = accepted execution facts
forecast = actual + remaining according to current knowledge
```

Tracking events do not rewrite the baseline.

Actuals do not rewrite the baseline.

Scenarios do not rewrite accepted actuals.

### Evidence and proposals before risky mutation

External events, imported source changes and AI-agent outputs are not
automatically true.

Approved flow:

```text
event -> evidence -> proposal / actual / warning / no-op -> accept/reject -> audit
```

When the risk is non-trivial, RePlan should create a proposal instead of
silently mutating current plan.

### Resource is generic

Resource means anything that constrains work.

RePlan resources include humans, AI agents, reviewers, API quotas, rate limits,
subscription entitlements, SaaS seats, budgets, equipment, environments,
locations, vehicles, robots, materials, vendor capacity, approval capacity and
generic constraints.

The canonical model must not be designed around human hours only.

### Actor and resource are separate

Actor identity and schedulable resource are separate concepts.

A human developer may be both an actor who performs RePlan operations and a
resource with capacity.

An AI coding agent may be both an actor that creates proposals and a resource
with modeled capacity.

A GPU server may be only a resource.

An OpenProject sync process may be only an actor.

### Policy and audit exist from the beginning

MVP does not implement enterprise authentication or full role-based access
control.

However, canonical mutations must still carry actor, policy and audit context.

Reason:

RePlan must not later rewrite its core model merely to add self-hosted team
mode, agent permissions, source write-back safety or compliance-oriented audit.

## Common primitives

### Identifier

Conceptual type:

```yaml
id: resource.human.ivan
```

Rules:

- required for persisted canonical objects;
- stable across imports when possible;
- unique within validation scope;
- suitable for CLI use;
- suitable for JSON output;
- suitable for Git diff review.

Recommended character set for MVP:

```text
a-z
0-9
.
_
-
:
/
#
```

Avoid spaces in canonical identifiers.

### Object kind

Canonical references should identify target kind.

Approved starting kinds:

```text
project
task
resource
resource_pool
resource_demand
capacity
availability
reservation
actual
baseline
current_plan
scenario
dependency
deadline
evidence
proposal
warning
actor
policy
audit_record
report_snapshot
source_snapshot
```

The list may grow.

### Reference

Conceptual model:

```yaml
ref:
  kind: task
  id: task.op.1234
```

Optional source hint:

```yaml
ref:
  kind: task
  id: task.op.1234
  source_ref:
    source: openproject
    object_type: work_package
    object_id: "1234"
```

### SourceRef

`SourceRef` points to an object in an external system or raw import snapshot.

Conceptual model:

```yaml
source_ref:
  source: openproject
  object_type: work_package
  object_id: "1234"
  project_id: "42"
  url: "https://pm.example.test/work_packages/1234"
  imported_at: "2026-07-06T12:00:00+07:00"
```

Examples of `source`:

```text
openproject
plane
github
gitlab
gitea
forgejo
taskjuggler
local_yaml
csv
manual
agent
ci
billing
inventory
sensor
```

### Time values

Use explicit time formats:

- timestamp: RFC 3339 with timezone;
- date: ISO date;
- week: ISO week where needed;
- period: explicit start and end;
- timezone: explicit when local working windows matter.

Example:

```yaml
period:
  start: "2026-07-06T00:00:00+07:00"
  end: "2026-07-13T00:00:00+07:00"
timezone: "Asia/Novosibirsk"
```

Rules:

- do not infer timezone silently;
- do not store ambiguous local timestamps without timezone;
- date-only values are acceptable for coarse planning;
- availability windows should use timezone when time of day matters.

### Amount and unit

Conceptual model:

```yaml
amount:
  value: 8
  unit: hours
```

Rules:

- amount must always include unit;
- unit must not be assumed from resource class;
- decimal values are allowed;
- unknown amount must be absent or explicitly unknown, not `0`.

Examples of units:

```text
hours
agent_hours
review_hours
tokens
requests
requests_per_minute
ci_minutes
rub
usd
seats
concurrent_slots
gpu_hours
device_hours
liters
meters
items
approvals
vendor_days
```

Unit class helps calculation and validation.

Examples:

```text
time
money
quota
rate
count
concurrency
stock
material
approval
generic
```

MVP may keep unit definitions simple, but the model must leave room for unit
metadata.

### Status date

Reports and comparisons must be calculated as of an explicit status date.

Example:

```yaml
status_date: "2026-07-06"
```

Meaning:

```text
show plan, actuals, remaining work, evidence and forecast known as of this date
```

## Project

`Project` defines a RePlan planning scope.

It is not necessarily the same as an OpenProject project or Git repository.

Conceptual model:

```yaml
project:
  id: project.orcode-bootstrap
  name: OrCode bootstrap planning
  format_version: 1
  timezone: Asia/Novosibirsk
  default_status_date: "2026-07-06"
  source_refs:
    - source: openproject
      object_type: project
      object_id: "orcode"
  policy_ref:
    kind: policy
    id: policy.local-trusted
```

Required MVP fields:

- `id`;
- `name`;
- `format_version` (a missing value is read as format `1` for compatibility);
- `timezone`.

Recommended MVP fields:

- `source_refs`;
- `policy_ref`;
- `default_status_date`.

## Actor

`Actor` is an entity that can perform actions in RePlan.

Conceptual model:

```yaml
actor:
  id: actor.human.operator
  kind: human
  display_name: Operator
  principal_refs:
    - kind: local_cli_user
      id: kogriv
  resource_ref:
    kind: resource
    id: resource.human.operator
  trust_level: owner
```

Actor kinds:

```text
human
ai_agent
service_account
sync_worker
ci_worker
robot_controller
external_system
unknown
```

Trust levels:

```text
owner
trusted
limited
untrusted
external
```

Required MVP fields:

- `id`;
- `kind`;
- `display_name`;
- `trust_level`.

Rules:

- actor may reference a resource, but does not have to;
- resource may reference an actor, but does not have to;
- mutating commands should record actor id in audit records;
- AI agents should normally create proposals, not apply changes silently.

## Resource

`Resource` is something whose capacity, availability or consumption can
constrain work.

Conceptual model:

```yaml
resource:
  id: resource.human.ivan
  class: human
  name: Ivan
  capabilities:
    - backend
    - code_review
  tags:
    - internal
  actor_ref:
    kind: actor
    id: actor.human.ivan
  source_refs:
    - source: openproject
      object_type: user
      object_id: "17"
```

Resource classes:

```text
human
ai_agent
service_quota
rate_limit
subscription_entitlement
budget
equipment
environment
material
location
vehicle
robot
vendor
approval
generic
```

The list is extensible.

Required MVP fields:

- `id`;
- `class`;
- `name`.

Recommended MVP fields:

- `capabilities`;
- `tags`;
- `actor_ref`;
- `source_refs`;
- `owner_actor_ref`;
- `policy_ref`.

Rules:

- resource does not imply actor identity;
- human resource capacity is not hard-coded as eight hours per day;
- AI agent resource capacity is separate from the agent's permission to mutate
  RePlan;
- service quota and rate limit resources are first-class resources;
- physical and offline resources use the same resource abstraction.

## ResourcePool

`ResourcePool` is a named group of resources or a planning placeholder for
resources not yet selected.

Conceptual model:

```yaml
resource_pool:
  id: pool.backend_developers
  name: Backend developers
  resource_class: human
  members:
    - kind: resource
      id: resource.human.ivan
    - kind: resource
      id: resource.human.petr
  capabilities:
    - backend
```

Unresolved pool example:

```yaml
resource_pool:
  id: pool.any_backend_developer
  name: Any backend developer
  resource_class: human
  members: []
  unresolved: true
```

Rules:

- resource demand may target a concrete resource or a pool;
- unresolved pools are allowed and should produce planning warnings when they
  block calculation;
- pools are useful before exact assignment.

## Task

`Task` is RePlan's canonical representation of work to be planned and tracked.

It is not a clone of an OpenProject work package, GitHub issue or Plane issue.

Conceptual model:

```yaml
task:
  id: task.op.1234
  title: Implement OpenProject import proof of concept
  description: Read-only import of work packages into RePlan canonical model.
  type: software
  source_refs:
    - source: openproject
      object_type: work_package
      object_id: "1234"
      url: "https://pm.example.test/work_packages/1234"
  source_status:
    source: openproject
    raw_status: In progress
    observed_at: "2026-07-06T09:00:00+07:00"
  tracking_status: in_progress
  planned_start: "2026-07-06"
  planned_finish: "2026-07-13"
  assignee_hint:
    kind: resource
    id: resource.human.ivan
  resource_demands:
    - kind: resource_demand
      id: demand.task-op-1234.implementer
```

Task types:

```text
software
research
review
operations
procurement
logistics
construction
repair
maintenance
administrative
approval
offline
generic
```

Tracking statuses:

```text
not_started
ready
in_progress
blocked
review_ready
accepted
cancelled
```

Required MVP fields:

- `id`;
- `title`;
- `tracking_status`.

Recommended MVP fields:

- `description`;
- `type`;
- `source_refs`;
- `source_status`;
- `assignee_hint`;
- `resource_demands`;
- `dependencies`;
- `deadlines`;
- `tags`;
- `warnings`.

Rules:

- source status is a signal, not accepted RePlan truth;
- `assignee_hint` is for compatibility and display, not the resource planning
  model;
- real planning demand is represented by `resource_demands`;
- a task may have many resource demands;
- a task may be a code task or offline task;
- `done` is not a canonical status because it is ambiguous.

## SourceStatus

`SourceStatus` stores source-system state as evidence or mapping hint.

Conceptual model:

```yaml
source_status:
  source: openproject
  raw_status: In progress
  mapped_hint: in_progress
  observed_at: "2026-07-06T09:00:00+07:00"
  evidence_ref:
    kind: evidence
    id: evidence.openproject.status-change.1234.20260706
```

Rules:

- source status must not overwrite `tracking_status` by default;
- adapters may map source status to a hint;
- accepted state changes require policy or explicit acceptance;
- conflicting source statuses should create warnings or proposals.

## ResourceDemand

`ResourceDemand` describes what a task needs.

Conceptual model:

```yaml
resource_demand:
  id: demand.task-op-1234.implementer
  task_ref:
    kind: task
    id: task.op.1234
  resource_ref:
    kind: resource
    id: resource.human.ivan
  resource_class: human
  role: implementer
  unit: hours
  planned_amount: 8
  remaining_amount: 5
  reserved_amount: 8
  actual_amount: 3
  criticality: required
  status: active
  timing:
    earliest_start: "2026-07-06"
    latest_finish: "2026-07-10"
    spread: even
```

Resource reference may target:

```text
resource
resource_pool
unresolved
```

Criticality values:

```text
required
preferred
optional
substitute
```

Demand statuses:

```text
draft
active
reserved
partially_consumed
consumed
cancelled
superseded
```

Required MVP fields:

- `id`;
- `task_ref`;
- `resource_class`;
- `role`;
- `unit`;
- one resource target: concrete resource, resource pool or unresolved need;
- at least one amount field when the demand participates in calculation.

Rules:

- unit must be explicit;
- planned, remaining, reserved and actual are separate values;
- actual consumption may be stored as separate `Actual` records and summarized
  into `actual_amount`;
- unknown remaining amount should produce warning when it affects forecast;
- demand may represent quota, budget, material, equipment, location or approval,
  not only time.

## Timing

`Timing` describes when a demand or task may happen.

Conceptual model:

```yaml
timing:
  earliest_start: "2026-07-06"
  latest_start: "2026-07-08"
  earliest_finish: "2026-07-07"
  latest_finish: "2026-07-10"
  preferred_start: "2026-07-06"
  preferred_finish: "2026-07-09"
  spread: even
  timezone: Asia/Novosibirsk
```

Spread values:

```text
even
front_loaded
back_loaded
fixed_window
as_available
manual
unknown
```

Rules:

- task dates and demand timing are related but not identical;
- a task may have broad dates while a specific demand has a narrower window;
- offline work often needs explicit access windows;
- service quotas and rate limits often need period-based timing.

## Capacity

`Capacity` describes how much a resource can provide.

Conceptual model:

```yaml
capacity:
  id: capacity.resource-human-ivan.weekly-hours
  resource_ref:
    kind: resource
    id: resource.human.ivan
  resource_class: human
  type: periodic_capacity
  unit: hours
  amount: 32
  period: week
  effective_from: "2026-07-01"
  timezone: Asia/Novosibirsk
```

Capacity types:

```text
periodic_capacity
availability_window
concurrent_capacity
quota_capacity
rate_limit
stock_capacity
budget_capacity
approval_capacity
```

Required MVP fields:

- `id`;
- `resource_ref`;
- `type`;
- `unit`;
- `amount`.

Rules:

- capacity is supply;
- capacity is not reservation;
- capacity is not actual consumption;
- capacity may be periodic, concurrent, stock-like, budget-like or quota-like;
- missing capacity is unknown, not infinite;
- `effective_from` and `effective_to` are optional inclusive `YYYY-MM-DD`
  dates; a missing bound is open. A change of capacity is recorded as a new
  record, with `effective_to` set on the old one, so past weeks keep the
  capacity they had. Several records in effect on the same day add up.

## Availability

`Availability` describes when capacity may be used.

Conceptual model:

```yaml
availability:
  id: availability.resource-human-ivan.default
  resource_ref:
    kind: resource
    id: resource.human.ivan
  timezone: Asia/Novosibirsk
  windows:
    - days: [monday, tuesday, wednesday, thursday]
      start_time: "10:00"
      end_time: "18:00"
  exceptions:
    - date: "2026-07-09"
      type: unavailable
      reason: personal leave
```

Availability exception types:

```text
unavailable
reduced_capacity
extra_capacity
maintenance
access_closed
quota_reset
manual_hold
```

Rules:

- capacity answers how much;
- availability answers when;
- exceptions are required from the beginning;
- working day means a date with positive relevant capacity, not hard-coded
  eight hours;
- offline work may depend on access windows, vendor windows or site windows.

## Reservation

`Reservation` describes planned occupancy of a resource by the current plan or
scenario.

Conceptual model:

```yaml
reservation:
  id: reservation.task-op-1234.ivan.week-28
  demand_ref:
    kind: resource_demand
    id: demand.task-op-1234.implementer
  resource_ref:
    kind: resource
    id: resource.human.ivan
  amount: 8
  unit: hours
  period:
    start: "2026-07-06T00:00:00+07:00"
    end: "2026-07-13T00:00:00+07:00"
  source: current_plan
```

Reservation sources:

```text
current_plan
baseline
scenario
manual_hold
external_calendar
imported_booking
```

Rules:

- reservation is planned allocation;
- reservation is not actual consumption;
- reservations should be derivable from current plan in simple MVP cases;
- explicit reservations are useful for fixed windows, equipment, environments
  and offline work.

## Actual

`Actual` is accepted execution fact.

Conceptual model:

```yaml
actual:
  id: actual.task-op-1234.ivan.20260706.1
  task_ref:
    kind: task
    id: task.op.1234
  demand_ref:
    kind: resource_demand
    id: demand.task-op-1234.implementer
  resource_ref:
    kind: resource
    id: resource.human.ivan
  amount: 3
  unit: hours
  occurred_at: "2026-07-06T15:00:00+07:00"
  status_date: "2026-07-06"
  accepted_by:
    kind: actor
    id: actor.human.operator
  evidence_refs:
    - kind: evidence
      id: evidence.manual.time-entry.20260706.1
```

Actual types:

```text
work_consumed
quota_consumed
budget_spent
material_consumed
equipment_used
environment_used
task_started
task_review_ready
task_accepted
task_blocked
task_cancelled
remaining_work_updated
```

Rules:

- actual is accepted fact, not raw event;
- actual should point to evidence where possible;
- actual may represent consumption or state transition;
- actual does not rewrite baseline;
- actual contributes to forecast and variance.

## Baseline

`Baseline` is an approved snapshot of planning commitments.

Conceptual model:

```yaml
baseline:
  id: baseline.2026-07-06.initial
  name: Initial MVP planning baseline
  created_at: "2026-07-06T12:00:00+07:00"
  approved_at: "2026-07-06T12:30:00+07:00"
  approved_by:
    kind: actor
    id: actor.human.operator
  status_date: "2026-07-06"
  included_tasks:
    - kind: task
      id: task.op.1234
      planned_start: "2026-07-06"
      planned_finish: "2026-07-08"
  included_demands:
    - kind: resource_demand
      id: demand.task-op-1234.implementer
      task: task.op.1234
      resource: resource.human.ivan
      unit: hours
      planned_amount: 12
  assumptions:
    - Ivan available 32 hours per week
    - OpenProject import is read-only
  seal:
    algorithm: sha256
    digest: 46d2...trimmed
```

Rules:

- baseline is immutable after approval;
- task entries may retain `planned_start` and `planned_finish`; demand entries
  may retain the task id, resource or resource-pool id, unit and original
  `planned_amount`;
- old ref-only entries and old unsealed baseline files remain valid;
- a new baseline is sealed with SHA-256. The digest input is the exact compact
  UTF-8 JSON returned by Go `encoding/json.Marshal` for the `Baseline` domain
  struct after setting `seal` to absent (`nil`). Struct fields use their JSON
  names and declared order, `omitempty` fields are omitted, arrays retain their
  stored order, and no maps occur in this schema. The digest is the lowercase
  hexadecimal SHA-256 of those bytes. YAML formatting, key order, quoting and
  comments therefore do not affect verification;
- changing any sealed domain value produces `baseline_modified`; repair means
  creating a new baseline, never resealing the old file;
- changing plan creates current plan changes, not baseline mutation;
- formal re-approval creates a new baseline;
- reports compare against `current_plan.based_on_baseline` when it resolves;
  a legacy plan without that pin falls back to the greatest `approved_at`,
  then `created_at`, then baseline id. File order is never meaningful;
- reports must state which baseline they compare against.

## CurrentPlan

`CurrentPlan` is the mutable working plan.

Conceptual model:

```yaml
current_plan:
  id: current
  version: 7
  updated_at: "2026-07-06T16:00:00+07:00"
  updated_by:
    kind: actor
    id: actor.human.operator
  based_on_baseline:
    kind: baseline
    id: baseline.2026-07-06.initial
  included_tasks:
    - kind: task
      id: task.op.1234
```

Rules:

- current plan may change during normal work;
- current plan changes should be audited;
- accepted proposals may mutate current plan;
- actuals influence forecast, but actuals remain separate records;
- current plan may diverge from baseline.

## Forecast

`Forecast` is usually a calculated model, not hand-authored state.

Conceptual formula:

```text
forecast_total = actual_amount + remaining_amount
```

If remaining amount is unknown:

```text
forecast_total = unknown
```

Rules:

- forecast must identify inputs;
- forecast must emit warnings when inputs are unknown;
- forecast should not be stored as accepted fact unless captured inside a
  report snapshot.

## Dependency

`Dependency` describes ordering or blocking relationship between tasks or other
planning objects.

Conceptual model:

```yaml
dependency:
  id: dependency.task-op-1234-after-task-op-1200
  type: finish_to_start
  predecessor_ref:
    kind: task
    id: task.op.1200
  successor_ref:
    kind: task
    id: task.op.1234
  required_state: accepted
  criticality: required
  source_refs:
    - source: openproject
      object_type: relation
      object_id: "991"
```

Dependency types:

```text
finish_to_start
start_to_start
finish_to_finish
start_to_finish
blocks
blocked_by
requires_resource
requires_approval
requires_delivery
external
generic
```

MVP persistence subset:

```text
finish_to_start
blocks
relates
requires_input_from
requires_resource_from
external_dependency
```

The broader list above is future-compatible conceptual vocabulary. The first
`canonical/dependencies.yaml` implementation should use the MVP subset defined
in `docs/spec/dependency-deadline-behavior.md`.

OpenProject relation accept initially needs only:

```text
finish_to_start
blocks
relates
```

Rules:

- dependency may come from PM system, manual planning or evidence;
- dependency should produce warnings when violated;
- dependency does not automatically move dates unless calculation or scenario
  explicitly proposes it.
- accepted dependencies are stored in `canonical/dependencies.yaml`;
- source-system relations become canonical dependencies only through explicit
  accept, not during read-only import;
- duplicate detection should check both source relation reference and canonical
  predecessor/successor/type triple. For `relates`, endpoint order is ignored.
- `relates` is a symmetric link with no scheduling or blocking meaning. Its
  normal persisted defaults are `criticality: optional`, no `required_state`,
  and `status: open`.

## Deadline

`Deadline` describes a due date, due timestamp or constraint date.

Conceptual model:

```yaml
deadline:
  id: deadline.task-op-1234.mvp
  target_ref:
    kind: task
    id: task.op.1234
  type: due_date
  due_date: "2026-07-12"
  timezone: Asia/Novosibirsk
  criticality: hard
  source: operator
```

Deadline types:

```text
due_date
due_timestamp
start_no_earlier_than
start_no_later_than
finish_no_earlier_than
finish_no_later_than
access_window_end
quota_reset_date
contractual
soft_target
```

Criticality values:

```text
hard
soft
informational
```

Rules:

- deadline feasibility is calculated against current plan and capacity;
- missing deadline is not automatically a problem;
- source-system due date should be mapped into deadline;
- deadline violation should produce structured warning.

## Scenario

`Scenario` is an isolated what-if overlay.

Conceptual model:

```yaml
scenario:
  id: scenario.ivan-unavailable-week-28
  name: Ivan unavailable in week 28
  status: draft
  base_ref:
    kind: current_plan
    id: current
  created_at: "2026-07-06T17:00:00+07:00"
  created_by:
    kind: actor
    id: actor.ai.codex.local
  changes:
    - op: add
      path: availability.exceptions
      value:
        resource_ref:
          kind: resource
          id: resource.human.ivan
        date: "2026-07-08"
        type: unavailable
        reason: scenario assumption
```

Scenario statuses:

```text
draft
calculated
proposed
accepted
rejected
applied
archived
```

Rules:

- scenario is overlay, not full project clone;
- scenario can change planning assumptions;
- scenario cannot change accepted actuals;
- scenario cannot rewrite historical evidence;
- scenario cannot rewrite old baselines;
- applying a scenario should create audited current plan changes.

## Evidence

`Evidence` is a source, signal or artifact that explains or supports something.

Conceptual model:

```yaml
evidence:
  id: evidence.github.pr.45
  type: git_event
  subtype: pull_request_opened
  source: github
  source_ref:
    source: github
    object_type: pull_request
    object_id: "45"
    url: "https://github.com/example/project/pull/45"
  actor_ref:
    kind: actor
    id: actor.human.ivan
  observed_at: "2026-07-06T15:00:00+07:00"
  confidence: observed
  summary: Pull request opened for task OP-1234
  targets:
    - kind: task
      id: task.op.1234
```

Evidence types:

```text
pm_event
git_event
ci_event
deployment_event
agent_report
usage_record
time_entry
manual_note
offline_artifact
operator_confirmation
customer_acceptance
sensor_record
communication_link
document
financial_record
inspection_record
```

Confidence values:

```text
observed
reported
inferred
trusted
untrusted
disputed
```

Rules:

- evidence is not automatically actual;
- evidence may support actuals, proposals or warnings;
- evidence should keep links to external artifacts when possible;
- offline artifacts such as photos, receipts, delivery notes and inspection
  records are valid evidence;
- evidence should not contain secrets.

## Proposal

`Proposal` is a suggested change that is not accepted truth yet.

Conceptual model:

```yaml
proposal:
  id: proposal.adjust-remaining-task-op-1234.20260706
  type: update_remaining_work
  status: open
  created_at: "2026-07-06T15:10:00+07:00"
  created_by:
    kind: actor
    id: actor.ai.codex.local
  target_ref:
    kind: resource_demand
    id: demand.task-op-1234.implementer
  suggested_change:
    op: set
    path: remaining_amount
    value: 5
  evidence_refs:
    - kind: evidence
      id: evidence.github.pr.45
  rationale: Pull request exists, but review is not complete.
  requires_confirmation: true
```

Proposal types:

```text
update_task_status
update_remaining_work
update_resource_demand
add_resource_demand
remove_resource_demand
change_capacity
add_availability_exception
update_deadline
update_dependency
create_actual
create_scenario
apply_scenario
write_back_to_source
```

Proposal statuses:

```text
open
accepted
rejected
superseded
expired
applied
```

Rules:

- AI agents may create proposals;
- risky changes should be proposals before mutation;
- proposal acceptance must create audit record;
- proposal rejection should preserve rationale when available;
- write-back to source systems should be proposal-driven or dry-run-first.

## Warning

`Warning` is a structured planning or tracking issue.

Conceptual model:

```yaml
warning:
  id: warning.task-op-1234.missing-reviewer
  type: missing_resource_demand
  severity: warning
  target_ref:
    kind: task
    id: task.op.1234
  message: Task has implementer demand but no reviewer demand.
  suggested_action: Add reviewer demand or explicitly mark review as not required.
```

Warning severities:

```text
info
warning
error
critical
```

Warning types:

```text
missing_estimate
missing_remaining_work
missing_capacity
unknown_resource
unresolved_resource_pool
overload
quota_exceeded
rate_limit_exceeded
budget_exceeded
deadline_risk
dependency_risk
missing_evidence
conflicting_source_status
unsafe_write_back
policy_denied
invalid_model
```

Rules:

- warnings should be machine-readable;
- warnings should be useful to AI CLI agents;
- errors block validation success;
- warnings may allow calculation to continue;
- every warning should have target reference where possible.

## AuditRecord

`AuditRecord` records accepted mutations and important decisions.

Conceptual model:

```yaml
audit_record:
  id: audit.20260706T151500.adjust-remaining-task-op-1234
  occurred_at: "2026-07-06T15:15:00+07:00"
  actor_ref:
    kind: actor
    id: actor.human.operator
  operation: accept_proposal
  target_ref:
    kind: proposal
    id: proposal.adjust-remaining-task-op-1234.20260706
  result: success
  reason: Operator accepted updated remaining work after reviewing pull request.
  evidence_refs:
    - kind: evidence
      id: evidence.github.pr.45
```

Audit operation families:

```text
create
update
delete
accept_proposal
reject_proposal
apply_scenario
create_baseline
import_source
export_source
write_back_to_source
policy_decision
validation_run
report_generated
```

Rules:

- mutating commands should produce audit records;
- audit records should include actor;
- audit records should include target;
- audit records should include reason when available;
- audit should not store secrets;
- local MVP audit can be append-only file-based audit.

## PolicyRef

`PolicyRef` points to the policy context used for authorization or safety
decision.

Conceptual model:

```yaml
policy_ref:
  kind: policy
  id: policy.local-trusted
```

MVP policy does not need full role-based access control.

MVP policy should still support:

- actor identity;
- allowed command families;
- proposal versus direct mutation distinction;
- dry-run requirement for risky operations;
- write-back safety rules;
- audit requirement.

## SourceSnapshot

`SourceSnapshot` records imported source data.

Conceptual model:

```yaml
source_snapshot:
  id: source_snapshot.openproject.20260706T120000
  source: openproject
  imported_at: "2026-07-06T12:00:00+07:00"
  imported_by:
    kind: actor
    id: actor.sync.openproject
  raw_location: imports/openproject/20260706T120000/raw.json
  adapter_version: replan-openproject-adapter-v0
  object_count: 42
```

Rules:

- raw imported data should be preserved separately from canonical model;
- canonical objects should reference source snapshots where useful;
- source snapshot is useful for debugging adapter mappings;
- source snapshot is not itself the current plan.

## ReportSnapshot

`ReportSnapshot` records a generated report result.

Conceptual model:

```yaml
report_snapshot:
  id: report.weekly.2026-W28.20260706T180000
  type: weekly_workload
  generated_at: "2026-07-06T18:00:00+07:00"
  generated_by:
    kind: actor
    id: actor.human.operator
  status_date: "2026-07-06"
  baseline_ref:
    kind: baseline
    id: baseline.2026-07-06.initial
  current_plan_ref:
    kind: current_plan
    id: current
  output_refs:
    - format: markdown
      path: reports/weekly/2026-W28.md
    - format: json
      path: reports/weekly/2026-W28.json
```

Rules:

- report snapshot is generated output metadata;
- report snapshot may be committed to Git for review;
- report snapshot should identify status date, baseline and current plan;
- JSON report output is the canonical machine-readable report output.

## Minimal MVP object set

The first useful implementation should support these persisted object types:

- `Project`;
- `Actor`;
- `Resource`;
- `ResourcePool`;
- `Task`;
- `ResourceDemand`;
- `Capacity`;
- `Availability`;
- `Actual`;
- `Baseline`;
- `CurrentPlan`;
- `Scenario`;
- `Evidence`;
- `Proposal`;
- `Warning`;
- `AuditRecord`;
- `SourceSnapshot`;
- `ReportSnapshot`.

The first implementation may represent `Reservation`, `Forecast`, `Dependency`
and `Deadline` with simpler embedded structures if that reduces implementation
overhead.

However, the conceptual model must not lose those entities. They are part of
the canonical design because later reports, scenarios and adapters need them.

## MVP validation invariants

Validation must reject:

- duplicate identifiers within validation scope;
- references to missing required objects;
- persisted objects without required identifiers;
- task without `tracking_status`;
- resource demand without task reference;
- resource demand without explicit unit;
- amount with unit missing;
- capacity without resource reference;
- capacity without unit;
- invalid tracking status;
- invalid proposal status;
- invalid scenario status;
- actual without accepted actor or policy-approved origin;
- audit record without actor or operation;
- source reference without source and object id when object id is available.

Validation should warn:

- task without resource demands;
- resource demand with unknown amount;
- resource demand targeting unresolved pool;
- task with source status that conflicts with tracking status;
- resource without capacity;
- capacity without availability;
- missing remaining amount for active work;
- missing deadline where project policy expects deadline;
- missing evidence for actual;
- scenario with changes that cannot be calculated;
- proposal without evidence for risky mutation;
- source-specific metadata placed inside canonical fields.

## Mapping rules

### OpenProject

OpenProject work package maps to canonical `Task`.

OpenProject user may map to canonical `Actor`, `Resource` or both.

OpenProject status maps to `SourceStatus` and may produce proposal to update
`tracking_status`.

OpenProject estimate maps to `ResourceDemand` when enough mapping information
exists.

OpenProject assignee maps to `assignee_hint` and may help create a default
human resource demand.

OpenProject relations map to `Dependency`.

OpenProject due dates map to `Deadline`.

OpenProject raw data should be preserved in `SourceSnapshot`.

### GitHub and GitLab

Issue maps to `Task` if used as a task source.

Pull request or merge request maps to `Evidence`.

CI status maps to `Evidence`.

Closed issue or merged pull request does not automatically mean `accepted`.

Git host events may create proposals:

- update status to `review_ready`;
- create actual work evidence;
- update remaining work;
- warn about missing review.

### TaskJuggler

TaskJuggler should be treated as optional scheduling engine and benchmark.

RePlan exports canonical tasks, demands, capacities and dependencies to
TaskJuggler format.

TaskJuggler result may be imported as scenario result or scheduling evidence.

TaskJuggler must not become required runtime for the MVP.

### Local YAML

Local YAML fixtures may create canonical objects directly.

Local YAML must still respect canonical invariants.

Fixture convenience must not introduce fields that bypass the canonical model.

## Example small project sketch

This sketch is not final storage layout.

It shows the intended canonical relationships.

```yaml
project:
  id: project.replan-mvp
  name: RePlan MVP
  timezone: Asia/Novosibirsk

actors:
  - id: actor.human.operator
    kind: human
    display_name: Operator
    trust_level: owner
    resource_ref:
      kind: resource
      id: resource.human.operator
  - id: actor.ai.codex.local
    kind: ai_agent
    display_name: Codex local agent
    trust_level: limited
    resource_ref:
      kind: resource
      id: resource.ai.codex.local

resources:
  - id: resource.human.operator
    class: human
    name: Operator
    capabilities: [architecture, review, operations]
  - id: resource.human.ivan
    class: human
    name: Ivan
    capabilities: [backend]
  - id: resource.ai.codex.local
    class: ai_agent
    name: Codex local agent
    capabilities: [coding, documentation]
  - id: resource.service.openai.tokens
    class: service_quota
    name: OpenAI project token quota
  - id: resource.environment.staging
    class: environment
    name: Staging environment

tasks:
  - id: task.op.1234
    title: Implement validation command
    type: software
    tracking_status: ready
    assignee_hint:
      kind: resource
      id: resource.human.ivan
    resource_demands:
      - kind: resource_demand
        id: demand.task-op-1234.implementer
      - kind: resource_demand
        id: demand.task-op-1234.review
      - kind: resource_demand
        id: demand.task-op-1234.ai-agent

resource_demands:
  - id: demand.task-op-1234.implementer
    task_ref:
      kind: task
      id: task.op.1234
    resource_ref:
      kind: resource
      id: resource.human.ivan
    resource_class: human
    role: implementer
    unit: hours
    planned_amount: 8
    remaining_amount: 8
    criticality: required
  - id: demand.task-op-1234.ai-agent
    task_ref:
      kind: task
      id: task.op.1234
    resource_ref:
      kind: resource
      id: resource.ai.codex.local
    resource_class: ai_agent
    role: ai_coding_agent
    unit: agent_hours
    planned_amount: 2
    remaining_amount: 2
    criticality: preferred
  - id: demand.task-op-1234.openai-quota
    task_ref:
      kind: task
      id: task.op.1234
    resource_ref:
      kind: resource
      id: resource.service.openai.tokens
    resource_class: service_quota
    role: api_provider
    unit: tokens
    planned_amount: 500000
    remaining_amount: 500000
    criticality: required
```

## Open questions for later specs

These questions should be resolved in storage, CLI or implementation sequence
documents:

- exact file layout for each object type;
- whether identifiers are globally unique or unique by kind;
- exact patch format for scenarios and proposals;
- exact JSON schema format;
- exact Go package layout and struct names;
- whether `Reservation` is stored or derived in MVP;
- whether `Forecast` is stored only in reports or also cached;
- how much validation belongs in core domain versus application service;
- whether SQLite remains cache-only or becomes canonical storage later.

## Decisions captured

- RePlan canonical model is source-agnostic.
- `assignee_hint` is not the core planning model.
- `resource_demands` are the core planning model.
- Resources are generic and not limited to humans or hours.
- AI agents, service quotas, subscriptions, rate limits, budgets, equipment,
  environments and offline resources are first-class planning concepts.
- Actor and resource are separate.
- Policy and audit context exist from the beginning.
- Baseline, current plan, actuals and forecast remain separate.
- Evidence and proposals are safety mechanisms.
- Unknown data must not silently become zero or unlimited.
- TaskJuggler is optional engine or benchmark, not the canonical model.
- OpenProject is an important reference adapter, not the canonical model.
