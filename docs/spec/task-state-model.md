# Task state model

Date: 2026-07-04

Status: approved functional design.

## Purpose

This document defines the internal RePlan task state model.

It answers:

- which task statuses RePlan understands internally;
- how RePlan state differs from OpenProject, GitHub, GitLab, Plane, Jira or
  other source-system status;
- why proposed status is separate from accepted status;
- why `review_ready` exists before `accepted`;
- how complex situations are represented without creating too many statuses;
- how the model applies to code and offline tasks.

This document does not define the full canonical data schema. The canonical
model will use this task state model as one of its inputs.

## Core decision

RePlan must have a small internal task state model.

RePlan must not directly copy the status model of OpenProject, Jira, GitHub,
GitLab, Plane or any other source system.

Source systems may provide status signals, but those signals are evidence or
mapping hints. They are not automatically RePlan truth.

The approved internal statuses for MVP are:

```text
not_started
ready
in_progress
blocked
review_ready
accepted
cancelled
```

The approved state layers are:

```text
source_status
tracking_status
proposals
flags
warnings
blockers
evidence
actuals
```

## Why RePlan needs its own statuses

Different source systems use different lifecycle models.

Examples:

- OpenProject may use `New`, `In progress`, `On hold`, `Rejected`, `Closed`.
- GitHub Issues may only use `open` and `closed`.
- GitLab Issues may use labels, issue state and merge requests.
- Plane may use project-specific states.
- A construction task may be `materials delivered`, `crew dispatched`,
  `waiting for access`, `ready for inspection`.
- A procurement task may be `quote requested`, `invoice received`,
  `paid`, `delivered`.
- A maintenance task may be `scheduled`, `performed`, `confirmed`.

If RePlan adopts one source-system status model as its core model, it stops
being source-agnostic.

Therefore RePlan needs a small internal state model that can represent the
planning and tracking meaning of work across different task substrates.

## Status definitions

### not_started

The task is known to RePlan, but work has not started.

This status can apply when:

- the task was imported from a source system;
- the task exists in local fixture data;
- no start evidence exists;
- no actor has claimed or begun work;
- the task is not yet ready because planning data is incomplete.

Examples:

- an OpenProject work package exists but no resource has started work;
- a GitHub issue exists but no branch or progress report exists;
- a procurement task exists but no buyer has requested a quote;
- a repair task exists but nobody has visited the site.

### ready

The task is ready to start.

This status means:

```text
RePlan has enough planning information for the task to be started, and no known
blocker currently prevents execution.
```

Ready does not mean work has started.

Examples:

- the task has an estimate and assigned resource demand;
- required materials are available;
- a work window exists;
- a developer has enough acceptance criteria to start;
- an AI agent can be assigned safely;
- a vendor quote task has the needed input documents.

### in_progress

Work has started or has been confirmed as active.

This status may be reached through:

- operator confirmation;
- trusted time entry;
- trusted source status mapping;
- agent started event under policy;
- manual CLI update;
- accepted proposal created from evidence.

Examples:

- OpenProject changed to `In progress` and policy accepts that as start;
- a developer reports two hours spent;
- an AI agent reports task start;
- a repair technician checks in and operator confirms work started;
- a procurement actor sends the first request to a vendor.

`in_progress` does not imply that the task is on schedule.

Schedule risk, overload, missing evidence and missing actuals are represented
through flags or warnings.

### blocked

The task cannot continue without an external event, missing resource, decision
or dependency.

Examples:

- waiting for customer answer;
- waiting for vendor response;
- waiting for API quota reset;
- waiting for access to a site;
- waiting for materials;
- waiting for code review;
- waiting for CI failure resolution;
- waiting for equipment;
- waiting for budget approval;
- blocked by another task.

Blocked status should usually include blocker records.

Example:

```yaml
tracking_status: blocked
blockers:
  - type: external_dependency
    description: Waiting for supplier delivery confirmation
    since: 2026-07-04T12:00:00+07:00
    evidence:
      - email:supplier-thread-778
```

### review_ready

The performer or system has enough evidence to say that the work appears ready
for review or acceptance.

This status intentionally sits between `in_progress` and `accepted`.

It is required because:

```text
"the performer says it is done" is different from "the result is accepted".
```

For code tasks, `review_ready` may mean:

- pull request opened;
- merge request opened;
- CI passed;
- agent proposed done;
- reviewer requested;
- implementation evidence attached.

For offline tasks, `review_ready` may mean:

- repair photo attached;
- delivery note attached;
- checklist completed;
- invoice received;
- inspection requested;
- worker reports done;
- vendor says work is complete;
- materials delivered according to supplier.

`review_ready` is not final completion.

Examples where `review_ready` should not become `accepted` automatically:

- a pull request exists but review has not happened;
- a photo exists but the work has not been inspected;
- a receipt exists but materials are not at the correct location;
- a vendor reports delivery but the receiving actor has not confirmed;
- an AI agent says done but tests or human validation are missing.

### accepted

The result has been accepted.

This is the RePlan equivalent of completion from a resource planning and
tracking perspective.

Accepted means:

- the work result is accepted by an appropriate actor or trusted policy;
- required acceptance evidence exists or was explicitly waived;
- remaining work is zero or intentionally closed;
- final actuals are recorded or a missing-actuals warning exists;
- the task no longer consumes planned execution capacity except follow-up work.

For code tasks, acceptance may require:

- review approval;
- merge;
- tests passing;
- operator acceptance;
- deployment or release evidence if relevant.

For offline tasks, acceptance may require:

- operator confirmation;
- customer acceptance;
- inspection acceptance;
- signed act;
- checklist completion;
- receipt and delivery confirmation;
- equipment returned;
- site restored;
- payment confirmation when payment is part of completion.

### cancelled

The task is intentionally excluded from execution.

Cancelled means:

- RePlan should not plan remaining execution work for this task;
- previous actuals remain historical facts;
- baseline comparison can still show that the task was cancelled after being
  planned;
- linked source status may or may not also be cancelled.

Examples:

- scope removed;
- duplicate task;
- customer no longer needs the work;
- task superseded by another task;
- procurement is no longer needed;
- repair replaced by full equipment replacement.

## Statuses not approved for MVP

The following statuses should not be first-class RePlan internal statuses in
MVP:

- `done`;
- `closed`;
- `merged`;
- `deployed`;
- `paid`;
- `delivered`;
- `scheduled`;
- `waiting`;
- `paused`;
- `on_hold`;
- `qa`;
- `review`;
- `rejected`.

This does not mean those concepts are unimportant.

They should be represented through:

- source status;
- evidence;
- flags;
- warnings;
- blockers;
- proposals;
- acceptance policy;
- adapter mapping.

For example:

- `merged` is Git evidence;
- `paid` is procurement or finance evidence;
- `delivered` is logistics evidence;
- `waiting` is usually `blocked` plus blocker details;
- `scheduled` is planning data, not tracking status;
- `rejected` may become `blocked`, `cancelled` or a proposal depending on
  context.

## Why `done` is not a core status

The word `done` is ambiguous.

It can mean:

- performer thinks the work is done;
- pull request exists;
- pull request merged;
- vendor says done;
- checklist completed;
- customer accepted;
- task closed in OpenProject;
- no more budget will be spent;
- no more work remains.

RePlan should avoid this ambiguity.

Use:

```text
review_ready
```

when work appears ready for acceptance.

Use:

```text
accepted
```

when the result has been accepted.

If a source system has `Done` or `Closed`, adapter mapping must decide whether
that is only source evidence, a `review_ready` hint or a trusted `accepted`
transition.

## Source status

`source_status` records what the source system says.

Example:

```yaml
source_status:
  source: openproject
  raw_status: "In progress"
  mapped_hint: in_progress
  synchronized_at: 2026-07-04T12:00:00+07:00

tracking_status: blocked
```

This example is valid.

It means:

- OpenProject says the task is in progress;
- RePlan currently treats it as blocked;
- RePlan should show the difference rather than hide it.

Source status is useful for:

- synchronization;
- reporting;
- evidence;
- warnings;
- adapter-specific mapping;
- source write-back decisions later.

Source status is not automatically authoritative for RePlan tracking state.

## Tracking status

`tracking_status` is RePlan's accepted internal task state.

It must be one of:

```text
not_started
ready
in_progress
blocked
review_ready
accepted
cancelled
```

Only accepted changes should update `tracking_status`.

An accepted change may come from:

- operator command;
- trusted source policy;
- accepted proposal;
- trusted imported actual;
- explicit local planning file change;
- later, an approved write-back or synchronization workflow.

## Proposed status

Proposed status is not the same as tracking status.

Example:

```yaml
tracking_status: in_progress

proposals:
  - id: proposal-002
    type: status_change
    suggested_status: review_ready
    derived_from:
      - github:pull-request:45
      - github:check-run:991
    confidence: medium
    requires_confirmation: true
```

This means:

- RePlan currently accepts that the task is `in_progress`;
- evidence suggests it may be `review_ready`;
- the proposal must be accepted before tracking status changes.

This separation is essential for AI-agent work.

Agents should usually be able to propose state changes without getting broad
permission to mutate accepted planning state.

## Proposal types

The minimum status-related proposal types are:

```text
proposed_start
proposed_progress_update
proposed_blocked
proposed_unblocked
proposed_review_ready
proposed_accepted
proposed_cancelled
proposed_estimate_change
proposed_remaining_change
proposed_reassignment
proposed_replan
```

Proposal type names may be normalized later in the canonical schema.

The important functional rule is:

```text
proposal means suggested change, not accepted fact
```

## Flags and warnings

RePlan should avoid status explosion.

Complex conditions should be represented through flags, warnings, blockers,
evidence and proposals instead of creating many compound statuses.

Bad model:

```text
in_progress_but_waiting_for_review_and_over_capacity_and_missing_receipt
```

Good model:

```yaml
tracking_status: in_progress
flags:
  - missing_actuals
  - deadline_risk
  - over_capacity
warnings:
  - type: no_recent_progress_update
    severity: medium
  - type: baseline_finish_slip
    severity: high
```

Another example:

```yaml
tracking_status: blocked
blockers:
  - type: vendor_response
    since: 2026-07-04
    owner: buyer_anna
```

Candidate flags:

- `missing_actuals`;
- `deadline_risk`;
- `over_capacity`;
- `under_capacity`;
- `missing_evidence`;
- `waiting_for_review`;
- `waiting_for_acceptance`;
- `source_status_mismatch`;
- `needs_replan`;
- `stale_progress`;
- `quota_risk`;
- `budget_risk`;
- `resource_unavailable`.

Candidate warnings:

- no recent progress update;
- source status differs from tracking status;
- task accepted without actuals;
- task closed in source without RePlan acceptance;
- actuals exceed baseline;
- remaining work increased;
- forecast finish exceeds deadline;
- required evidence missing;
- blocker has no owner;
- proposal waiting too long.

## OpenProject mapping

OpenProject status mapping belongs to the OpenProject adapter, not the RePlan
core.

For MVP, a simple default mapping can be:

```text
OpenProject New         -> not_started
OpenProject Open        -> not_started
OpenProject Scheduled   -> ready
OpenProject Confirmed   -> ready
OpenProject In progress -> in_progress
OpenProject On hold     -> blocked
OpenProject Closed      -> review_ready or accepted, depending on policy
OpenProject Rejected    -> cancelled or blocked, depending on policy
```

This mapping is intentionally not absolute. The important MVP rule is that
mere OpenProject visibility (`New` or `Open`) does not imply RePlan readiness;
RePlan `ready` should require a more explicit planning status or an accepted
RePlan decision after dependency and resource review.

Examples:

```yaml
source_status:
  source: openproject
  raw_status: "Closed"
  mapped_hint: accepted
tracking_status: review_ready
warnings:
  - type: closed_without_actuals
```

```yaml
source_status:
  source: openproject
  raw_status: "In progress"
  mapped_hint: in_progress
tracking_status: blocked
blockers:
  - type: ci_failure
```

```yaml
source_status:
  source: openproject
  raw_status: "New"
  mapped_hint: not_started
tracking_status: ready
```

These examples are valid because source status and tracking status are separate
layers.

## GitHub and GitLab mapping

GitHub and GitLab usually do not provide enough task lifecycle state by
themselves.

They provide strong evidence.

Examples:

- branch created;
- commit pushed;
- pull request opened;
- merge request opened;
- review requested;
- review approved;
- changes requested;
- CI passed;
- CI failed;
- pull request merged;
- issue closed.

Default mapping guidance:

```text
branch/commit evidence       -> possible proposed_start or progress evidence
pull request opened          -> possible proposed_review_ready
CI passed                    -> evidence, maybe strengthen proposed_review_ready
review approved              -> evidence, maybe proposed_accepted
pull request merged          -> evidence, maybe proposed_accepted
issue closed                 -> source status/evidence, policy decides
```

Pull request merged should not always equal `accepted`.

Some projects require additional acceptance, deployment, customer validation or
resource accounting.

## Offline task mapping

Offline tasks use the same state model.

Examples:

### Repair

```text
repair task created          -> not_started
parts and technician ready   -> ready
technician starts work       -> in_progress
waiting for missing part     -> blocked
photo and checklist attached -> review_ready
operator accepts repair      -> accepted
repair no longer needed      -> cancelled
```

### Procurement

```text
purchase task created        -> not_started
budget and item list ready   -> ready
quote requested              -> in_progress
waiting for supplier answer  -> blocked
receipt and delivery note    -> review_ready
materials confirmed on site  -> accepted
purchase cancelled           -> cancelled
```

### Logistics

```text
delivery task created        -> not_started
route and vehicle available  -> ready
vehicle dispatched           -> in_progress
waiting for site access      -> blocked
delivery photo attached      -> review_ready
receiver confirms delivery   -> accepted
delivery cancelled           -> cancelled
```

### Appointment or service

```text
appointment task created     -> not_started
time and provider confirmed  -> ready
session starts               -> in_progress
provider unavailable         -> blocked
provider reports complete    -> review_ready
operator confirms complete   -> accepted
appointment cancelled        -> cancelled
```

The same model covers code, physical work, procurement, logistics and services
without creating domain-specific core statuses.

## State transitions

Typical transitions:

```text
not_started -> ready
not_started -> in_progress
ready -> in_progress
ready -> blocked
in_progress -> blocked
blocked -> ready
blocked -> in_progress
in_progress -> review_ready
review_ready -> accepted
review_ready -> in_progress
not_started -> cancelled
ready -> cancelled
in_progress -> cancelled
blocked -> cancelled
review_ready -> cancelled
```

Allowed exceptional transitions:

```text
not_started -> accepted
ready -> accepted
blocked -> accepted
accepted -> in_progress
accepted -> cancelled
cancelled -> ready
cancelled -> in_progress
```

Exceptional transitions should be allowed only with explicit reason and audit
record.

Examples:

- a trivial task is imported after it has already been completed;
- source history was imported late;
- accepted work is reopened due to defect;
- cancelled work is restored to scope.

## Audit requirements

Every accepted tracking status change should record:

- previous tracking status;
- new tracking status;
- actor;
- timestamp;
- reason;
- evidence references when available;
- source event references when available;
- whether the change was manual, imported, policy-driven or proposal-driven.

Example:

```yaml
status_change:
  task_id: openproject:123
  from: in_progress
  to: review_ready
  actor_id: operator
  timestamp: 2026-07-04T18:30:00+07:00
  reason: PR opened, CI passed, reviewer requested
  evidence:
    - github:pull-request:45
    - github:check-run:991
  source: accepted_proposal
```

For MVP, audit can be file-based.

The important part is that status changes are explainable.

## Report behavior

Reports should show source status and tracking status separately when both are
known.

Example:

```text
OP-123: Implement billing adapter
  source status: OpenProject In progress
  tracking status: blocked
  reason: CI failing, waiting for external API credentials
  proposals: 1
  warnings: source status mismatch, deadline risk
```

For compact reports, RePlan may show only tracking status by default, but a
detail view should expose source status and mapping hints.

## MVP requirements

MVP should implement:

- the seven internal tracking statuses;
- source status stored separately from tracking status;
- proposed status stored separately from tracking status;
- flags and warnings separate from status;
- blockers separate from status;
- status change audit records;
- report output that can show source status and tracking status separately;
- mapping hooks for OpenProject and local fixtures;
- enough examples to test code and offline tasks.

MVP does not need:

- complex workflow editor;
- arbitrary custom status graph;
- source write-back;
- enterprise approval workflow;
- per-project graphical status configuration.

## Approved decisions

- RePlan has a small internal status model.
- RePlan does not copy OpenProject or any other source status model.
- The approved internal statuses are `not_started`, `ready`, `in_progress`,
  `blocked`, `review_ready`, `accepted` and `cancelled`.
- `done` is not an approved internal status because it is ambiguous.
- `review_ready` means work appears ready for acceptance.
- `accepted` means the result has been accepted.
- Source status is evidence or mapping hint, not automatic truth.
- Tracking status is the accepted RePlan task state.
- Proposed status is separate from tracking status.
- AI agents should normally create proposals rather than mutating accepted
  tracking status directly.
- Complex conditions are represented through flags, warnings, blockers,
  evidence and proposals, not through many compound statuses.
- OpenProject mapping belongs to the OpenProject adapter.
- GitHub and GitLab provide evidence, not a complete lifecycle.
- Offline tasks use the same state model as code tasks.
- Every accepted tracking status change should be auditable.
