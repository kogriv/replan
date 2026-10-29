# PM-system interaction behavior

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how RePlan interacts with project-management systems.

It answers:

- which data is owned by OpenProject or another PM system;
- which data is owned by RePlan;
- what happens when source status differs from RePlan tracking status;
- how links between OpenProject, GitHub, GitLab and RePlan are represented;
- what read-only first means;
- what write-back later means.

This document is functional design. The exact OpenProject adapter schema will
be defined later in `docs/spec/openproject-adapter.md`.

## Core decision

RePlan is adjacent to project-management systems.

It is not a replacement task tracker.

Approved rule:

```text
PM system = task identity and collaboration surface
RePlan = resource planning, tracking intelligence and decision layer
```

OpenProject is the first likely real PM substrate.

OpenProject is not RePlan's domain model.

RePlan must remain adapter-based so that Plane, GitHub Issues, GitLab Issues,
YouTrack, Jira or local fixtures can be supported later.

## Ownership boundaries

### PM system owns ordinary task management

OpenProject or another PM system should own:

- task identifier;
- title;
- description;
- source status;
- source assignee;
- comments;
- attachments;
- labels;
- categories;
- project structure;
- source permissions;
- ordinary task lifecycle;
- collaboration history.

### RePlan owns planning intelligence

RePlan should own:

- task projection;
- resource demands;
- resource capacity;
- availability;
- baseline;
- current plan;
- actuals;
- forecast;
- scenarios;
- workload calculations;
- warnings;
- evidence links;
- proposals;
- RePlan audit;
- reports;
- AI-agent interaction surface.

## Task projection

RePlan stores a planning projection of source tasks.

The projection links source task identity to RePlan planning objects.

Conceptual model:

```yaml
replan_task:
  id: task-abc
  primary_source_ref:
    system: openproject
    type: work_package
    id: "123"
    url: https://op.example/work_packages/123
  source_refs:
    - system: openproject
      type: work_package
      id: "123"
    - system: github
      type: issue
      id: "77"
  title_cache: Implement billing adapter
  source_status:
    source: openproject
    raw_status: In progress
    mapped_hint: in_progress
  tracking_status: blocked
```

The title and source status may be cached for reporting, but the PM system
remains the owner of ordinary task data.

## Source references

RePlan should use normalized source references.

Example:

```yaml
source_ref:
  system: openproject
  type: work_package
  id: "123"
  url: https://op.example/work_packages/123
```

GitHub example:

```yaml
source_ref:
  system: github
  type: pull_request
  repository: org/repo
  id: "45"
  url: https://github.com/org/repo/pull/45
```

GitLab example:

```yaml
source_ref:
  system: gitlab
  type: merge_request
  project: group/project
  id: "12"
  url: https://gitlab.com/group/project/-/merge_requests/12
```

One RePlan task projection may have multiple source references.

It should have one primary PM-system source when possible.

## Source status versus RePlan tracking status

Source status and RePlan tracking status are separate.

Source status is what the PM system says.

Tracking status is what RePlan accepts for planning and execution tracking.

These may differ.

Example:

```text
OpenProject: Closed
RePlan: review_ready
Reason: accepted actuals or acceptance evidence are missing
```

Example:

```text
OpenProject: In progress
RePlan: blocked
Reason: CI failing and external dependency unresolved
```

RePlan should show mismatch as a warning.

RePlan should not force both systems into one truth automatically.

## Read-only first

MVP integration should be read-only.

Allowed in MVP:

- import source tasks;
- import source statuses;
- import assignees;
- import dates;
- import estimates;
- import relations/dependencies where available;
- import spent time if available and configured;
- import comments or links as evidence if configured;
- create local RePlan task projections;
- create local planning assumptions;
- generate reports;
- generate warnings;
- generate proposals.

Not allowed by default in MVP:

- change OpenProject status;
- change assignee;
- change due date;
- write comments;
- close tasks;
- create tasks in PM system;
- create baselines in PM system;
- automatically apply RePlan proposals back to PM system.

This keeps MVP safe and makes AI-CLI-first operation less risky.

## Write-back later

Write-back is useful but must be explicit and audited.

Possible future write-back actions:

- add comment with RePlan report summary;
- add link to RePlan report;
- add link to evidence;
- update custom field;
- propose source status update;
- propose due date update;
- create relation/dependency;
- create task;
- change assignee;
- attach generated report artifact.

Approved write-back flow:

```text
proposal -> dry-run -> explicit apply -> audit
```

No automatic source write-back by default.

## Adapter model

Each PM-system adapter should provide:

- source task import;
- source status import;
- source status normalization;
- assignee hint mapping;
- estimate mapping;
- date mapping;
- dependency/relation mapping;
- link/evidence mapping;
- source metadata;
- unsupported-field reporting;
- optional write-back proposal preparation.

OpenProject adapter is first.

Plane, GitHub Issues, GitLab Issues, YouTrack and Jira can be added later.

## OpenProject mapping

Initial OpenProject adapter should support:

- work package identifier;
- title;
- description cache if needed;
- status;
- assignee;
- start date;
- due date;
- estimated time where available;
- spent time where available;
- relations;
- links;
- comments or attachments as evidence if configured;
- custom fields later.

OpenProject status should map to a hint, not absolute truth.

Example:

```yaml
source_status:
  source: openproject
  raw_status: Closed
  mapped_hint: accepted
tracking_status: review_ready
warnings:
  - type: source_status_mismatch
```

## Assignee mapping

Source assignee should be imported as `assignee_hint`.

The adapter may create an initial human resource demand from assignee and
estimate.

Example:

```yaml
assignee_hint: dev_ivan
resource_demands:
  - resource_ref:
      type: resource
      id: dev_ivan
    resource_class: human
    role: implementer
    unit: hours
    planned_amount: 8
```

This generated demand should be marked as adapter-derived.

It should not prevent adding reviewer, AI-agent, quota, environment, budget or
offline resource demands.

## Links between systems

RePlan should store links between:

- PM-system task;
- git issue;
- pull request or merge request;
- commit;
- CI run;
- deployment;
- evidence artifact;
- RePlan task projection;
- RePlan proposal;
- RePlan report.

Evidence link example:

```yaml
evidence:
  type: pull_request
  source: github
  url: https://github.com/org/repo/pull/45
  task_refs:
    - openproject:123
```

Links should be explicit enough for reports and AI agents to follow.

## Conflict handling

Source changes may conflict with RePlan planning assumptions.

Examples:

- OpenProject assignee changed, but RePlan has a different resource demand;
- OpenProject due date changed, but current plan has another deadline;
- OpenProject task closed, but RePlan has missing actuals;
- source estimate changed, but RePlan has an accepted local estimate;
- source relation deleted, but RePlan has dependency warning.

Approved rule:

```text
do not overwrite local planning assumptions silently
create warning or proposal
```

Example:

```text
OpenProject assignee changed Ivan -> Anna
RePlan demand: dev_ivan 8h
Action: source_mismatch warning or reassignment proposal
```

## Refresh behavior

Read-only refresh should:

- update cached source fields;
- update source status;
- update source metadata;
- import new source tasks;
- mark missing source tasks as source-missing or removed-from-source;
- create source mismatch warnings;
- create proposals for meaningful planning changes;
- preserve RePlan actuals;
- preserve baselines;
- preserve audit;
- preserve local planning assumptions unless explicit policy says otherwise.

Refresh must not silently:

- delete RePlan planning data;
- delete actuals;
- rewrite baseline;
- apply source status as tracking status;
- write back to source.

## Source mismatch warnings

MVP should generate warnings for:

- source status differs from tracking status;
- source assignee differs from RePlan primary human demand;
- source due date differs from RePlan deadline;
- source estimate differs from RePlan current planned amount;
- source task closed while RePlan actuals are missing;
- source task removed while RePlan has active planning data;
- source dependency differs from RePlan dependency;
- source permission or visibility issue prevents refresh.

## Reports

Reports should show:

- source reference;
- source status;
- RePlan tracking status;
- source mismatch warnings;
- imported timestamp;
- adapter used;
- unsupported source fields when relevant;
- write-back proposals when present.

Example:

```text
OP-123 Implement billing adapter
  source status: OpenProject In progress
  RePlan tracking status: blocked
  source mismatch: yes
  reason: external dependency unresolved
```

## AI-CLI behavior

AI agents should be able to:

- inspect source references;
- inspect source status;
- inspect RePlan tracking status;
- inspect mismatch warnings;
- propose adapter mappings;
- propose source write-back;
- dry-run source write-back;
- generate report summaries.

AI agents should not silently:

- write back to PM systems;
- change source status;
- change assignee;
- close tasks;
- overwrite RePlan planning assumptions from source import.

## MVP requirements

MVP should implement or model:

- source references;
- primary source reference;
- task projection;
- source status storage;
- status mapping hints;
- assignee hint import;
- assignee hint to resource demand generation;
- estimate import;
- date import;
- dependency import where available;
- evidence link import where configured;
- source mismatch warnings;
- read-only refresh;
- report showing source status and RePlan tracking status;
- unsupported field reporting;
- no automatic write-back.

## Explicit MVP non-goals

MVP should not implement:

- automatic write-back;
- two-way sync;
- conflict auto-resolution;
- source permission mirroring;
- full OpenProject workflow clone;
- full Jira support;
- full Plane support;
- full GitHub Issues parity;
- full GitLab Issues parity;
- source-side custom workflow editor;
- automatic task closure in PM system.

## Relationship to other documents

This document depends on:

- [`planning-lifecycle.md`](planning-lifecycle.md);
- [`task-state-model.md`](task-state-model.md);
- [`resource-demand-model.md`](resource-demand-model.md);
- [`tracking-events.md`](tracking-events.md);
- [`evidence-proposal-behavior.md`](evidence-proposal-behavior.md);
- [`report-behavior.md`](report-behavior.md).

Future document:

- `docs/spec/openproject-adapter.md`.

## Approved decisions

- RePlan is adjacent to PM systems, not a replacement task tracker.
- OpenProject is the first likely real PM substrate.
- OpenProject is not RePlan's domain model.
- RePlan remains adapter-based.
- PM system owns ordinary task management.
- RePlan owns planning intelligence.
- Source status and RePlan tracking status are separate.
- Source mismatch should be shown as warning.
- MVP integration is read-only first.
- MVP must not write back to OpenProject by default.
- Write-back later must be proposal-driven, dry-run capable, explicit and
  audited.
- Refresh must not silently overwrite local planning assumptions, actuals,
  baselines or audit.
