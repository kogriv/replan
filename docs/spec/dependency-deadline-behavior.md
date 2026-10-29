# Dependency and deadline behavior

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how RePlan handles dependencies and deadlines.

It answers:

- whether dependencies are part of MVP;
- which dependency types are needed;
- which dependency warnings are required;
- whether dependencies affect scheduling or only warnings in MVP;
- how deadline risk is calculated;
- what the minimal "this deadline looks impossible" heuristic is;
- what should be delegated to an advanced scheduling engine later.

This document is functional design. The exact canonical schema will be defined
later in `docs/spec/canonical-model.md`.

Update 2026-07-17:

- OpenProject relation import now exposes read-only `dependency_candidates`.
- Private validation confirmed that in-scope OpenProject relations can become
  safe dependency candidates after ordinary task accept; the sanitized
  conclusions are in
  [`../dev/openproject-pilot-lessons.md`](../dev/openproject-pilot-lessons.md).
- Canonical dependency persistence foundation and
  `import accept --accept-dependencies` are implemented.
- The live dependency accept pilot against a temporary project copy passed.
- Dependency apply remains manual while relation quality is reviewed across
  more pilot runs.
- Dependency planning warnings are surfaced by `replan validate` and
  `replan report weekly`.
- Dependency proposal candidates are previewed by
  `replan proposal suggest-dependencies`; candidates may now also be
  materialized explicitly with `--write-candidates --actor <id>`.

## Core decision

Dependencies and deadlines are part of MVP.

In MVP, they affect:

- warnings;
- feasibility checks;
- report output;
- scenario comparison;
- decision prompts.

In MVP, they should not automatically reschedule the plan.

Approved rule:

```text
MVP dependencies affect warnings and feasibility checks.
MVP dependencies do not automatically reschedule the plan.
Advanced dependency scheduling can later be delegated to TaskJuggler or another
engine.
```

## Why this boundary

Dependencies and deadlines are too important to ignore.

Without them, RePlan cannot warn about:

- a task scheduled before its predecessor is done;
- a blocked dependency that makes downstream work risky;
- a deadline that forecast already misses;
- a required input that has no owner;
- an impossible-looking plan.

But full dependency scheduling can become a large solver problem.

Full solver behavior includes:

- critical path method;
- automatic date propagation;
- lead and lag constraints;
- resource leveling;
- constraint optimization;
- automatic rescheduling;
- cycle resolution;
- complex calendar interactions.

That is not MVP.

The MVP should be useful by being honest and explainable:

```text
here are the dependencies;
here are the deadlines;
here is what looks risky, inconsistent or impossible;
the plan was not silently changed.
```

## Dependency

A dependency is a relationship between one task and another task, external
object or resource condition.

Examples:

- task B cannot start until task A finishes;
- task B requires input from task A;
- task C is blocked by supplier response;
- task D requires staging environment after task E releases it;
- task F requires budget approval;
- task G requires site access;
- task H requires material delivery.

## Dependency types

Approved MVP dependency types:

```text
finish_to_start
blocks
requires_input_from
requires_resource_from
external_dependency
```

### finish_to_start

Task B should not start before task A is accepted or otherwise considered ready
by policy.

Example:

```text
database migration must finish before application deployment starts
```

### blocks

One task blocks another task.

This is a simpler and more general dependency.

Example:

```text
supplier answer blocks procurement decision
```

### requires_input_from

A task requires a deliverable, answer, document, design, API, decision or other
input from another task or external party.

Example:

```text
frontend implementation requires API contract from backend task
```

### requires_resource_from

A task requires a resource released or produced by another task.

Example:

```text
test task requires staging environment after deployment task finishes
```

### external_dependency

A dependency on something outside the task system.

Examples:

- vendor response;
- customer approval;
- permit;
- site access;
- material delivery;
- weather window;
- external API availability.

## Dependency record

Conceptual model:

```yaml
dependency:
  id: dep-001
  type: finish_to_start
  predecessor: openproject:100
  successor: openproject:123
  required_state: accepted
  status: open
  criticality: required
  evidence:
    - openproject:relation:55
```

External dependency example:

```yaml
dependency:
  id: dep-002
  type: external_dependency
  successor: openproject:procurement-44
  description: Supplier must confirm delivery date
  owner: buyer_anna
  status: open
  criticality: required
  expected_resolution_date: 2026-07-08
```

The exact field names may change in the canonical schema.

The approved concepts are:

- dependency identifier;
- type;
- predecessor or external source;
- successor;
- required state or condition;
- status;
- criticality;
- owner when applicable;
- expected resolution date when applicable;
- evidence links.

## Canonical dependency persistence design entry point

This section defines the starting point for the first
`canonical/dependencies.yaml` implementation slice.

The goal of the first persistence slice is deliberately narrow:

- store accepted dependency records in canonical YAML;
- support dependency warnings and reports;
- support OpenProject relation acceptance after explicit operator approval;
- keep dependency accept separate from ordinary task accept, resource-link
  accept, resource-demand accept, time-entry accept and tracking-status accept;
- avoid automatic schedule mutation.

The first persistence slice should not implement:

- automatic date propagation;
- critical path calculation;
- resource leveling;
- source-system write-back;
- automatic dependency deletion when a source relation disappears;
- complex lead or lag constraints.

### MVP canonical file

Dependency records should live in:

```text
canonical/dependencies.yaml
```

The file should be deterministic, readable and friendly to human review and AI
CLI agents.

Recommended first shape:

```yaml
dependencies:
  - id: dependency.openproject.relation.8
    type: finish_to_start
    predecessor_ref:
      kind: task
      id: task.op.37
    successor_ref:
      kind: task
      id: task.op.38
    required_state: accepted
    status: open
    criticality: required
    source_refs:
      - source: openproject
        object_type: relation
        object_id: "8"
        url: /api/v3/relations/8
    notes: "Imported from OpenProject relation 8."
```

For a blocker dependency:

```yaml
dependencies:
  - id: dependency.openproject.relation.9
    type: blocks
    predecessor_ref:
      kind: task
      id: task.op.40
    successor_ref:
      kind: task
      id: task.op.41
    status: open
    criticality: required
    source_refs:
      - source: openproject
        object_type: relation
        object_id: "9"
        url: /api/v3/relations/9
```

The names `predecessor_ref` and `successor_ref` are intentionally used for both
`finish_to_start` and `blocks`.

For `finish_to_start`, the meaning is chronological:

```text
predecessor should finish before successor starts
```

For `blocks`, the meaning is dependency direction:

```text
predecessor blocks successor
```

This keeps import, validation and reporting simple.

### Required first fields

The first canonical dependency schema should require:

- `id`;
- `type`;
- `successor_ref`;
- `status`;
- `criticality`.

For task-to-task dependencies, it should also require:

- `predecessor_ref`.

For external dependencies, it should require at least one of:

- `external_ref`;
- `description`;
- `owner_ref`;
- `expected_resolution_date`.

Recommended fields:

- `required_state`;
- `source_refs`;
- `notes`;
- `created_at`;
- `created_by`;
- `updated_at`;
- `updated_by`.

Audit records remain the authoritative mutation log. Timestamp fields inside
the dependency file are optional convenience metadata, not a replacement for
audit.

### MVP type subset

The approved functional dependency types are:

```text
finish_to_start
blocks
requires_input_from
requires_resource_from
external_dependency
```

The first OpenProject accept implementation only needs to write:

```text
finish_to_start
blocks
```

Other approved types may be supported manually before source-system import
supports them.

### OpenProject relation mapping

The OpenProject mapper produces read-only `dependency_candidates`.

Accepted OpenProject mapping for MVP:

| OpenProject relation type | RePlan dependency type | Direction |
| --- | --- | --- |
| `precedes` | `finish_to_start` | `from` is predecessor, `to` is successor |
| `follows` | `finish_to_start` | `to` is predecessor, `from` is successor |
| `blocks` | `blocks` | `from` blocks successor `to` |
| `blocked` | `blocks` | `to` blocks successor `from` |

OpenProject may normalize relation creation into the reverse relation type.
For example, a requested `precedes` relation may be returned as `follows` with
the `from` and `to` sides reversed. RePlan should trust the relation record
returned by the OpenProject API and apply the mapping table above.

Unsupported OpenProject relation types should not be accepted until explicitly
designed.

### Dependency accept workflow

Implemented command:

```bash
replan import accept <snapshot-id> --accept-dependencies --dry-run --format json
replan import accept <snapshot-id> --accept-dependencies --apply --actor actor.human.operator --format json
```

Rules:

- `--accept-dependencies` is a separate import accept mode;
- it must be mutually exclusive with ordinary task accept and all other
  specialized accept modes;
- `--apply` requires `--actor`;
- dry-run must list the exact dependency records that would be appended;
- apply must write `canonical/dependencies.yaml`;
- apply must append audit;
- apply must run validation after writing;
- apply must not mutate task dates, task status, baselines, current plan dates,
  actuals or source systems.

Safe acceptance requires:

- candidate `accept_status` is `safe_to_accept`;
- dependency type is supported;
- predecessor task exists in canonical tasks for task-to-task dependencies;
- successor task exists in canonical tasks;
- no existing dependency has the same source relation source reference;
- no existing dependency has the same canonical predecessor, successor and type
  unless the existing record is intentionally being updated by a separately
  designed workflow.

Unsafe candidates should remain visible in dry-run output with diagnostics.

### Duplicate detection

The first implementation should check duplicates in two layers.

Source duplicate:

```text
source_refs contains source=openproject, object_type=relation, object_id=<id>
```

Canonical duplicate:

```text
type + predecessor_ref + successor_ref
```

Source duplicate prevents accepting the same OpenProject relation twice.

Canonical duplicate prevents creating two RePlan dependency records that mean
the same thing even if they came from different source systems or from a manual
record plus source import.

For MVP, duplicate candidates should be skipped with warnings rather than
merged automatically.

### Validation rules

Validation should reject or warn on:

- missing `id`;
- duplicate dependency id;
- unsupported dependency type;
- missing successor reference;
- missing predecessor reference for task-to-task dependency types;
- reference to missing canonical task;
- dependency from a task to itself;
- duplicate source relation reference;
- duplicate predecessor/successor/type triple;
- invalid dependency status;
- invalid criticality;
- simple dependency cycle, if cycle detection is implemented in the slice.

Cycle handling should be conservative. If full graph validation is too large
for the first slice, document it as a warning-only follow-up instead of hiding
the limitation.

### Cross-project and partial imports

OpenProject relations may point outside the imported work-package set.

RePlan should keep those relation candidates visible as `task_unmatched` when
one side cannot be matched to a canonical task.

It should not silently drop them because they may represent real cross-project,
vendor, customer, platform or operations dependencies.

The first `--accept-dependencies` implementation should not create placeholder
tasks for unmatched relation sides. Placeholder creation is a separate design
problem.

### Source deletion and drift

The first implementation should not delete canonical dependencies just because
a source relation disappears.

Possible later behavior:

- report source drift;
- mark a dependency as `unknown`;
- propose cancellation;
- require explicit operator approval before changing canonical dependency
  status.

For MVP, import accept only appends safe new dependency records.

## Dependency status

Approved dependency statuses:

```text
open
resolved
blocked
unknown
cancelled
```

Meanings:

- `open` — dependency is known and not yet resolved;
- `resolved` — dependency condition is satisfied;
- `blocked` — dependency is stuck and needs intervention;
- `unknown` — dependency exists, but its state is not known;
- `cancelled` — dependency no longer applies.

Dependency status is separate from task tracking status.

## Dependency criticality

Approved criticality values:

```text
required
preferred
optional
```

For MVP, required dependencies should generate stronger warnings.

Optional dependencies may be reported without blocking feasibility.

## Dependency resolution

A dependency is resolved when its required condition is satisfied.

Examples:

- predecessor task is `accepted`;
- predecessor task is `review_ready`, if policy allows review-ready as input;
- vendor answered;
- material delivered;
- site access granted;
- approval received;
- API credentials issued;
- environment released.

Resolution should be evidence-backed when possible.

## Dependencies and scheduling in MVP

In MVP, dependencies do not automatically move task dates.

They produce warnings when current dates conflict with dependency state.

Example:

```text
Task B is scheduled to start on 2026-07-07.
Task A is a required predecessor and is forecast to finish on 2026-07-09.
Warning: successor starts before required predecessor forecast finish.
```

The plan remains unchanged.

The report should show the issue and let the operator or agent create a
proposal.

Possible proposal:

```text
propose moving Task B start to 2026-07-10
```

## Deadline

A deadline is a time constraint.

It can attach to:

- task;
- milestone;
- dependency;
- resource demand;
- scenario;
- project;
- external obligation.

Deadline examples:

- task must finish by 2026-07-12;
- procurement must be completed before site work starts;
- deployment window closes at 18:00;
- API quota resets on a specific date;
- permit expires on 2026-08-01;
- customer acceptance must happen before invoice period closes.

## Deadline record

Conceptual model:

```yaml
deadline:
  id: deadline-001
  target_ref: openproject:123
  target_type: task
  due_at: 2026-07-12T18:00:00+07:00
  criticality: required
  source: openproject
  evidence:
    - openproject:work-package:123
```

Date-only deadline:

```yaml
deadline:
  id: deadline-002
  target_ref: openproject:procurement-44
  target_type: task
  due_date: 2026-07-10
  timezone: Asia/Novosibirsk
  criticality: required
```

## Deadline criticality

Approved values:

```text
required
target
soft
informational
```

Meanings:

- `required` — missing it is a serious problem;
- `target` — expected target date, but negotiable;
- `soft` — useful planning signal;
- `informational` — display and report only.

## Deadline risk

Minimum MVP rule:

```text
if forecast_finish > deadline:
  deadline_risk = true
```

Example:

```text
forecast finish: 2026-07-14
deadline: 2026-07-12
deadline risk: forecast misses deadline by 2 days
```

If forecast finish is unknown:

```text
deadline risk = unknown
warning: deadline risk unknown because forecast finish is unknown
```

## Minimal impossible-deadline heuristic

The MVP should support a simple impossible-looking deadline warning.

Conceptual rule:

```text
if earliest_possible_finish > deadline:
  emit impossible_deadline_warning
```

`earliest_possible_finish` is a rough heuristic, not a solver result.

It may use:

- remaining workload;
- available capacity;
- unresolved required dependencies;
- required availability windows;
- known blockers;
- required resource demand availability.

Example:

```text
remaining work: 24h
available capacity before deadline: 8h
required dependencies: resolved
deadline: 2026-07-08
warning: deadline appears impossible with known capacity
```

If data is incomplete:

```text
deadline feasibility unknown
```

and a warning should explain why.

Examples:

- capacity unknown;
- remaining work unknown;
- dependency state unknown;
- no scheduled date range;
- required window unknown.

## Dependency warnings

MVP should generate dependency warnings for:

- required dependency unresolved;
- required dependency status unknown;
- successor scheduled before predecessor forecast finish;
- successor started while required dependency unresolved;
- predecessor forecast finish later than successor start;
- dependency owner missing;
- external dependency expected resolution date missed;
- blocked dependency has no proposal;
- dependency cycle detected if simple cycle detection exists;
- dependency references missing task or resource;
- dependency state conflicts with source system.

Implemented MVP dependency warnings:

```text
required_dependency_unresolved
dependency_predecessor_finish_unknown
dependency_successor_start_unknown
dependency_successor_starts_before_predecessor_finish
dependency_blocks_active_successor
```

`required_dependency_unresolved` warns only when the dependency matters at
the project status date. An `open` dependency whose successor is a task that
has not started and is planned to start after the status date is the normal
state of a plan made ahead and does not warn. It warns when:

- the predecessor already reached the required state, so the record is stale;
- the successor is `in_progress`, `review_ready`, `done` or `accepted`;
- the successor's planned start is on or before the status date;
- the successor has no planned start, the project has no status date, the
  successor is not a task, or the dependency is `blocked` or `unknown`.

The message names the reason. Next actions for open dependencies are not
affected by this rule.

`proposal suggest-dependencies` follows the same rule for its
`review_dependency_resolution` candidates: it writes one only when the warning
above would be raised. `mark_dependency_resolved` candidates for satisfied
dependencies are unchanged.

These warnings are informational planning signals. They do not mutate task
dates, dependency status, task tracking status, baselines or current plan.

## Dependency proposal preview

Dependency warnings may be converted into reviewable proposal candidates.

Implemented first slice:

```text
replan proposal suggest-dependencies --dry-run
replan proposal suggest-dependencies --write-candidates --actor <id>
```

This command is intentionally a proposal-candidate layer.

It returns proposal-shaped objects so AI command-line agents and human
operators can inspect possible next actions using the same proposal vocabulary
as the rest of RePlan.

In dry-run mode it does not write files.

In write-candidates mode it writes only reviewable `proposals/open/*.yaml`
candidate files.

It does not:

- accept proposals;
- mutate canonical tasks;
- mutate canonical dependencies;
- move dates;
- change task tracking status;
- write back to OpenProject.

Implemented proposal candidate types:

```text
review_dependency_resolution
review_dependency_schedule
adjust_successor_start
mark_successor_blocked
mark_dependency_resolved
mark_successor_ready
```

Meaning:

- `review_dependency_resolution` is created when a required dependency is still
  open, blocked or unknown;
- `review_dependency_schedule` is created when a `finish_to_start` dependency
  cannot be checked because the predecessor planned finish or successor planned
  start is missing;
- `adjust_successor_start` is created when a `finish_to_start` successor starts
  before predecessor planned finish;
- `mark_successor_blocked` is created when an active `blocks` dependency affects
  a successor task that is ready, in progress or review-ready;
- `mark_dependency_resolved` is created when a task-to-task dependency
  predecessor is already in the required state. If `required_state` is not set,
  the MVP default is `accepted`;
- `mark_successor_ready` is created when a task is blocked and all required
  dependencies targeting that task are resolved or cancelled.

All candidates require confirmation.

Write-candidates rules:

- a known actor is required;
- existing proposal identifiers are skipped with
  `proposal_candidate_already_exists`;
- the project is validated after files are written;
- creating candidate files does not mean the proposal is accepted.

The next design step is proposal application: how accepted dependency proposal
types should mutate task dates, task tracking status or dependency status, and
which of those mutations should remain manual.

## Dependency proposal accept/apply

Implemented first accept/apply slice:

```text
replan proposal accept <mark_successor_blocked proposal id> --actor <id> --apply
```

`mark_successor_blocked`, `mark_dependency_resolved` and
`mark_successor_ready` are mutating in the current slice.

Accepting a `mark_successor_blocked` proposal:

- moves the proposal from `open` to `accepted`;
- updates the successor task `tracking_status` to `blocked`;
- appends audit;
- validates the project after write.

Accepting a `mark_dependency_resolved` proposal:

- moves the proposal from `open` to `accepted`;
- updates the dependency `status` to `resolved`;
- appends audit;
- validates the project after write.

Accepting a `mark_successor_ready` proposal:

- moves the proposal from `open` to `accepted`;
- updates the successor task `tracking_status` to `ready`;
- appends audit;
- validates the project after write.

The following dependency proposal types are accepted-only review decisions in
this slice:

```text
review_dependency_resolution
review_dependency_schedule
adjust_successor_start
```

They are not applied automatically because:

- review proposals do not identify a single canonical mutation;
- dependency resolution should be evidence-backed and may require dependency
  status changes, notes or external confirmation;
- schedule/date movement must be designed together with current plan,
  baselines, scenario flow and TaskJuggler interaction.

Accepting dependency proposals still does not:

- move dates for `adjust_successor_start`;
- apply TaskJuggler results;
- infer external dependency resolution;
- infer resource-demand dependency resolution;
- infer `in_progress`, `review_ready` or accepted work;
- write back to OpenProject.

## Deadline warnings

MVP should generate deadline warnings for:

- forecast finish exceeds required deadline;
- current planned finish exceeds required deadline;
- baseline finish exceeds deadline;
- deadline exists but forecast finish is unknown;
- impossible-looking deadline based on known remaining work and capacity;
- deadline risk unknown because capacity is unknown;
- deadline risk unknown because remaining amount is unknown;
- deadline risk unknown because dependency state is unknown;
- task has deadline but no scheduled date range;
- deadline timezone missing when local time matters.

## Dependency and task status

Dependency state may influence task warnings and proposals.

It should not directly overwrite task tracking status.

Examples:

- unresolved required dependency may create a `blocked` proposal;
- resolved dependency may create an `unblocked` proposal;
- dependency conflict may create a replan proposal.

But dependency state should not silently change:

```text
tracking_status
baseline
current plan dates
```

unless explicit policy allows it.

## Dependency and scenarios

Scenarios should be able to change:

- dependency assumptions;
- dependency resolution date;
- deadline;
- task dates;
- resource availability;
- remaining work.

Scenario examples:

- "What if supplier answers two days later?"
- "What if API credentials arrive today?"
- "What if we move deadline by one week?"
- "What if we add a second reviewer?"

Scenario calculation should show changed dependency and deadline warnings.

## Relationship to TaskJuggler

TaskJuggler can later be useful for advanced scheduling.

Possible future use:

- export tasks, resources and dependencies to TaskJuggler;
- run advanced schedule calculation;
- import or compare calculated schedule;
- use TaskJuggler as optional advanced engine.

MVP should not depend on TaskJuggler for basic warnings.

RePlan must be useful without TaskJuggler for:

- listing dependencies;
- detecting obvious dependency/date conflicts;
- deadline risk warnings;
- impossible-looking deadline heuristic;
- reports.

## MVP requirements

MVP should implement or model:

- dependency records;
- dependency type;
- dependency status;
- dependency criticality;
- dependency owner;
- predecessor/successor relation;
- external dependency;
- deadline records;
- deadline criticality;
- forecast-finish-vs-deadline warning;
- unresolved dependency warning;
- successor-before-predecessor warning;
- unknown dependency state warning;
- deadline feasibility unknown warning;
- simple impossible-looking deadline heuristic;
- proposal hooks for replan, blocked and unblocked.

## Explicit MVP non-goals

MVP should not implement:

- full critical path method;
- automatic dependency scheduling;
- automatic date propagation;
- automatic resource leveling;
- optimal schedule solver;
- complex lead/lag constraints;
- complex dependency calendars;
- full TaskJuggler clone;
- automatic plan mutation;
- automatic baseline mutation.

## Approved decisions

- Dependencies are part of MVP.
- Deadlines are part of MVP.
- MVP dependencies affect warnings and feasibility checks.
- MVP dependencies do not automatically reschedule the plan.
- Dependency types include `finish_to_start`, `blocks`,
  `requires_input_from`, `requires_resource_from` and `external_dependency`.
- Dependency statuses include `open`, `resolved`, `blocked`, `unknown` and
  `cancelled`.
- Deadline criticality includes `required`, `target`, `soft` and
  `informational`.
- Minimum deadline risk is forecast finish after deadline.
- Missing forecast, capacity, remaining work or dependency state should produce
  unknown-risk warnings instead of false confidence.
- Minimal impossible-deadline heuristic is based on earliest possible finish
  using known remaining work, capacity, dependencies and windows.
- Dependency state may create proposals, but should not silently overwrite task
  status, current plan, baseline or dates.
- Advanced scheduling may later use TaskJuggler or another engine.
