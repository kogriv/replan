# Evidence and proposal behavior

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how RePlan handles evidence, proposals, accepted changes
and audit records.

It answers:

- what evidence types exist;
- how evidence attaches to tasks, resources, actuals, proposals and other
  planning objects;
- how a proposal becomes accepted or rejected;
- whether AI agents can create proposals directly;
- which proposal types are allowed in MVP;
- what must be audited.

This document is functional design. The exact canonical schema will be defined
later in `docs/spec/canonical-model.md`.

## Core decision

Evidence and proposals are the main safety mechanism for RePlan.

Approved rule:

```text
evidence = source, signal or artifact that explains or supports something
proposal = suggested change that is not accepted truth yet
accepted change = explicitly accepted mutation with audit trail
```

Core flow:

```text
event -> evidence -> proposal / actual / warning / no-op -> accept/reject ->
audit
```

External events, agent outputs and imported data should help RePlan, but should
not silently corrupt the plan.

## Evidence

Evidence is a record that helps explain why RePlan believes something, suggests
something or warns about something.

Evidence may come from:

- project-management systems;
- git hosts;
- CI systems;
- deployment systems;
- agent runtimes;
- API providers;
- billing systems;
- procurement systems;
- inventory systems;
- sensors;
- manual operator input;
- offline work artifacts.

Evidence does not necessarily change state.

Examples:

```text
pull request opened != accepted task
photo attached != accepted repair
calendar event != completed work
agent says done != accepted result
vendor says shipped != received on site
```

Evidence is context, support and traceability.

It becomes state only through trusted policy or explicit acceptance.

## Evidence types

Approved evidence type families:

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

Examples:

- OpenProject status change;
- GitHub pull request;
- GitLab merge request;
- commit;
- CI run;
- deployment;
- agent report;
- token usage record;
- API billing record;
- time entry;
- manual note;
- photo;
- receipt;
- invoice;
- delivery note;
- checklist;
- operator confirmation;
- customer acceptance;
- sensor reading;
- geolocation check-in;
- calendar event;
- email link;
- chat link;
- signed act;
- inspection record.

The evidence type list should be extensible.

The model should not require every future evidence class to be predefined.

## Evidence record

Conceptual model:

```yaml
evidence:
  id: evidence-001
  type: git_event
  subtype: pull_request_opened
  source: github
  source_ref: github:example/project:pull/45
  url: https://github.com/example/project/pull/45
  actor_id: dev_ivan
  observed_at: 2026-07-06T15:00:00+07:00
  confidence: observed
  summary: Pull request opened for OP-123
  targets:
    - type: task
      id: openproject:123
```

Offline example:

```yaml
evidence:
  id: evidence-042
  type: offline_artifact
  subtype: photo
  source: manual
  url: file:///evidence/repair-17/pump-installed.jpg
  actor_id: plumber_alex
  observed_at: 2026-07-06T14:20:00+07:00
  confidence: reported
  summary: Pump installed and leak test completed
  targets:
    - type: task
      id: openproject:repair-17
```

The exact field names may change.

Approved concepts:

- evidence identifier;
- type;
- subtype;
- source;
- source reference;
- URL or local reference when available;
- actor;
- timestamp;
- confidence;
- summary;
- targets;
- optional structured payload.

## Evidence targets

Evidence may attach to:

- task;
- resource;
- resource demand;
- actual record;
- proposal;
- dependency;
- deadline;
- scenario;
- warning;
- baseline decision;
- current-plan change;
- status change;
- source write-back;
- audit record.

Evidence can have multiple targets.

Example:

```text
API usage record attaches to both task and quota actual.
```

## Evidence confidence

Evidence should carry confidence.

Approved confidence values:

```text
observed
reported
inferred
confirmed
untrusted
```

Meanings:

- `observed` — imported or observed from source system;
- `reported` — actor reported it;
- `inferred` — RePlan or agent inferred it;
- `confirmed` — trusted actor or policy confirmed it;
- `untrusted` — available as context but not trusted for automatic changes.

Confidence is not the same as permission.

An observed event may still require confirmation before changing plan state.

## Proposal

A proposal is a suggested change.

It is not accepted truth.

Examples:

- change task status to `review_ready`;
- record 3 hours actual work;
- change remaining work from 5h to 8h;
- add reviewer demand;
- change assignee;
- add API quota demand;
- mark dependency resolved;
- move deadline;
- create scenario;
- apply scenario;
- create new baseline;
- write a comment back to OpenProject.

Proposals let humans, agents and integrations contribute without silently
mutating important state.

## Proposal types

Approved MVP proposal types:

```text
status_change
actual_record
remaining_change
estimate_change
resource_demand_change
assignment_change
capacity_change
dependency_change
deadline_change
scenario_create
scenario_apply
baseline_create
warning_resolution
source_writeback
review_dependency_resolution
review_dependency_schedule
adjust_successor_start
mark_successor_blocked
mark_dependency_resolved
mark_successor_ready
```

This set is enough for MVP planning and tracking.

More specialized proposal types can be added later.

Dependency proposal preview update, 2026-07-17:

- dependency warning signals may be converted into proposal-shaped preview
  candidates by `replan proposal suggest-dependencies`;
- preview candidates are not accepted truth;
- preview candidates are not automatically persisted as `proposals/open/*.yaml`;
- preview candidates require confirmation before any later materialization or
  application decision;
- the first implemented dependency preview types are
  `review_dependency_resolution`, `review_dependency_schedule`,
  `adjust_successor_start`, `mark_successor_blocked` and
  `mark_dependency_resolved`, and `mark_successor_ready`.

## Proposal record

Conceptual model:

```yaml
proposal:
  id: proposal-001
  type: status_change
  status: open
  created_by: agent_coder_1
  actor_type: ai_agent
  trust_level: propose_only
  created_at: 2026-07-06T15:20:00+07:00
  target:
    type: task
    id: openproject:123
  suggested_change:
    field: tracking_status
    from: in_progress
    to: review_ready
  evidence:
    - evidence-001
    - evidence-002
  requires_confirmation: true
  rationale: PR opened and CI passed
```

The exact schema may change.

Approved concepts:

- proposal identifier;
- type;
- status;
- creator;
- actor type;
- trust level;
- created timestamp;
- target;
- suggested change;
- evidence references;
- confirmation requirement;
- rationale.

## Proposal lifecycle

Approved lifecycle:

```text
draft
open
accepted
rejected
superseded
applied
expired
```

### draft

Proposal is being built and is not ready for review.

### open

Proposal is ready for review.

### accepted

Decision was accepted.

Accepted does not always mean applied.

### rejected

Proposal was rejected.

### superseded

Proposal was replaced by another proposal.

### applied

Proposal changes were applied to RePlan state.

### expired

Proposal is no longer relevant.

## Accepted versus applied

RePlan should distinguish:

```text
accepted proposal != applied proposal
```

For simple MVP proposal types, accepting may immediately apply.

But the model must allow separation because some proposals require follow-up
operations.

Examples:

- source write-back may be accepted but not yet performed;
- scenario apply may be accepted but applied later;
- baseline creation may be accepted but created as a separate action;
- capacity change may need operator confirmation and later sync.

## Accept proposal

Accepting a proposal should create an audit record.

Audit should include:

- proposal identifier;
- actor;
- timestamp;
- decision;
- reason;
- evidence used;
- changes applied or approved;
- previous values;
- new values;
- whether source write-back was performed;
- whether a follow-up action remains.

Example:

```yaml
audit:
  type: proposal_decision
  proposal_id: proposal-001
  decision: accepted
  actor_id: operator
  timestamp: 2026-07-06T16:00:00+07:00
  reason: PR reviewed and CI passed
  evidence:
    - evidence-001
    - evidence-002
  applied_changes:
    - target: openproject:123
      field: tracking_status
      from: in_progress
      to: review_ready
```

## Reject proposal

Rejecting a proposal should also create audit.

Audit should include:

- proposal identifier;
- actor;
- timestamp;
- decision;
- reason;
- superseding proposal if any.

Example:

```yaml
audit:
  type: proposal_decision
  proposal_id: proposal-004
  decision: rejected
  actor_id: operator
  timestamp: 2026-07-06T16:10:00+07:00
  reason: PR does not cover acceptance criteria
```

## Automatic application policy

MVP may automatically apply only low-risk changes.

Allowed automatic changes:

- evidence link creation;
- warning creation;
- no-op classification;
- trusted actual usage from configured source;
- trusted API or token usage;
- trusted time entry;
- deterministic calculation output.

Forbidden automatic changes by default:

- accepted or done task status;
- baseline creation;
- source write-back;
- reassignment;
- deadline change;
- capacity change;
- scenario application;
- large remaining or estimate changes;
- changes from untrusted AI agents;
- deletion of actuals or evidence;
- mutation of old baselines.

## Actor trust and policy

Proposal and evidence records should include:

```text
created_by
actor_type
trust_level
requires_confirmation
```

Candidate actor types:

```text
human_operator
human_worker
ai_agent
adapter
webhook
external_vendor
system
```

Candidate trust levels:

```text
read_only
evidence_only
propose_only
trusted_actuals
trusted_mutation
operator
```

Default posture:

- human operator can apply allowed changes;
- trusted import can apply configured actual usage;
- AI agent is propose-only by default;
- webhook is evidence-only by default;
- external vendor is evidence/proposal only by default.

The detailed access model belongs to
[`../security/access-and-actor-model.md`](../security/access-and-actor-model.md).

## AI-agent behavior

AI agents should be able to:

- read plan state;
- inspect warnings;
- attach evidence;
- create proposals;
- create scenarios;
- calculate scenarios;
- propose scenario application;
- produce explanations.

AI agents should not silently:

- change baseline;
- change accepted actuals;
- mark work accepted;
- write back to PM systems;
- apply scenarios;
- reassign work;
- delete evidence;
- erase audit.

Default rule:

```text
agent may read, calculate, attach evidence and propose; mutation requires
explicit permission or accepted policy.
```

## Audit requirements

Audit is required for:

- accepted proposal;
- rejected proposal;
- applied proposal;
- applied scenario;
- baseline creation;
- status change;
- actual record creation;
- actual record correction;
- source write-back;
- capacity override;
- deadline change;
- dependency resolution;
- evidence deletion if deletion is ever allowed.

Audit records should include:

- actor;
- actor type;
- timestamp;
- operation;
- target;
- previous value when applicable;
- new value when applicable;
- evidence references;
- proposal reference when applicable;
- reason.

For MVP, audit can be local and file-based.

The important requirement is explainability.

## MVP requirements

MVP should implement or model:

- evidence records;
- evidence type and subtype;
- evidence source reference;
- evidence targets;
- evidence confidence;
- proposal records;
- proposal type;
- proposal lifecycle;
- proposal accept/reject;
- proposal apply for limited safe types;
- actor type;
- trust level;
- `requires_confirmation`;
- audit records;
- AI-agent propose-only default;
- structured JSON output;
- human-readable Markdown output.

## Explicit MVP non-goals

MVP should not implement:

- full workflow engine;
- enterprise approval chains;
- fine-grained RBAC implementation;
- source write-back automation;
- cryptographic evidence notarization;
- complex policy language;
- legal evidence custody;
- multi-party signature flows;
- deletion workflows for audit records.

## Relationship to other documents

This document depends on:

- [`tracking-events.md`](tracking-events.md);
- [`task-state-model.md`](task-state-model.md);
- [`scenario-model.md`](scenario-model.md);
- [`planning-lifecycle.md`](planning-lifecycle.md);
- [`workload-calculation.md`](workload-calculation.md);
- [`../security/access-and-actor-model.md`](../security/access-and-actor-model.md).

The next design item is:

```text
K. Report behavior
```

## Approved decisions

- Evidence explains why RePlan believes, suggests or warns about something.
- Proposal suggests changes without making them true.
- Accepted change mutates RePlan state and creates audit.
- Evidence can attach to tasks, resources, demands, actuals, proposals,
  dependencies, deadlines, scenarios, warnings, baseline decisions and current
  plan changes.
- Evidence does not automatically change state.
- MVP proposal types include status, actuals, remaining, estimate, resource
  demand, assignment, capacity, dependency, deadline, scenario, baseline,
  warning, source write-back and dependency-warning preview proposals.
- Proposal lifecycle distinguishes draft, open, accepted, rejected, superseded,
  applied and expired.
- Accepted and applied are different states, even if some MVP proposal types
  combine them operationally.
- AI agents are propose-only by default.
- Only low-risk trusted changes may apply automatically in MVP.
- Audit is required for accepted/rejected/applied proposals and important state
  changes.
