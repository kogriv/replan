# OpenProject functionality and technical pitfalls

Status: current. Index: [OpenProject design index](openproject-design-index.md).

Date: 2026-08-04

Original status: accepted reconstruction for the OpenProject preflight infrastructure
build.

Related task:
`task.self.reconstruct-openproject-functionality-and-pitfalls`.

## Purpose

This document reconstructs what RePlan already learned about OpenProject during
the pilot, self-dogfood bootstrap and live synchronization work.

The goal is practical:

```text
Make OpenProject comfortable enough for humans and AI CLI agents that RePlan
can actually dogfood an OpenProject-first workflow.
```

This is not a proposal to replace OpenProject. OpenProject remains the human
project-management surface. RePlan remains the resource-planning sidecar.

## Current Working Model

OpenProject owns:

- project identity;
- work package identity;
- work package title;
- work package description;
- human-visible task status;
- human-visible task dates;
- assignee;
- relations;
- Gantt and ordinary project views;
- comments and ordinary collaboration;
- time entries when humans record time there.

RePlan owns:

- resource identities that are not native OpenProject users;
- abstract resources such as reviewer roles;
- AI agent resources;
- subscription, quota, API, CI and environment resources;
- resource demands;
- resource capacities and availability;
- workload and overload analysis;
- scenarios;
- remaining-work calibration;
- accepted actuals and evidence;
- proposals;
- audit records;
- AI-safe command flow.

The boundary is intentional. OpenProject is the source of truth for task
management. RePlan is the source of truth for resource planning and audited
resource decisions.

## Functions We Actually Need From OpenProject

The current dogfood workflow needs only a narrow OpenProject subset:

- list projects;
- list work packages for the `replan-self` project;
- read one work package by id;
- create missing work packages during controlled bootstrap;
- update work package subject, description, status and dates when reviewed;
- read available statuses and type links;
- read assignee/user links;
- read and create relations;
- import closed work packages for historical alignment;
- import time entries as candidate evidence and actuals;
- preserve work package identity across RePlan sidecar changes.

Anything outside that set is not part of the next preflight implementation.

Examples of intentionally out-of-scope OpenProject functionality:

- project deletion;
- work package deletion;
- broad bulk rewriting;
- custom workflow administration;
- role and permission administration;
- agile board configuration;
- wiki/document management;
- budget, cost and enterprise reporting modules;
- plugin development.

## Technical Pitfalls Already Observed

### Optimistic Locking

OpenProject work package updates require current object state. The live API
uses optimistic locking through `lockVersion`. A safe writer must read the
current work package immediately before a `PATCH` and send the current
`lockVersion`.

Impact:

- cached work package JSON is not enough for writes;
- AI agents must not generate blind `PATCH` requests;
- helper commands need an inspect or preflight step before apply;
- stale `lockVersion` must block mutation and produce a clear retry action.

Reference:

- OpenProject API documentation:
  `https://www.openproject.org/docs/api/example/`.

### HATEOAS Links And Schemas

OpenProject API payloads are link-heavy. Many fields are represented through
`_links` rather than plain ids. The allowed values depend on project type,
workflow and permissions.

Impact:

- status and type updates should use links such as `/api/v3/statuses/7`;
- relation payloads should use work package links;
- assignee updates should use user links;
- a generic JSON patcher is risky without schema awareness;
- helpers should inspect available project types and statuses before writes.

### Relation Direction

RePlan dependency direction and OpenProject relation direction are easy to
confuse.

Current mapping:

| RePlan dependency | OpenProject relation | OpenProject `from` | OpenProject `to` |
| --- | --- | --- | --- |
| `finish_to_start` | `follows` | successor work package | predecessor work package |
| `blocks` | `blocks` | predecessor work package | successor work package |

Impact:

- relation writers must not infer direction from title or visual order;
- helper output must show predecessor task, successor task, `from` work package
  and `to` work package together;
- drift checks must compare normalized relation tuples.

### Date Normalization And Scheduling Side Effects

OpenProject may adjust dates after relations are created or when work packages
are connected through scheduling rules. This was observed during the preflight
backlog materialization: a `follows` relation from a closed checkpoint with a
future finish date moved successor tasks forward.

Impact:

- date drift after relation creation is normal and must be reviewed;
- completed task date changes require a separate review because they rewrite
  history;
- the operator may need either to accept OpenProject's schedule into RePlan or
  push reviewed RePlan dates back into OpenProject;
- preflight helpers must show exactly which side owns the next date decision.

### Closed Task Date Changes

The existing date push helper deliberately refuses to apply completed-task date
changes without review. This is correct. Completed task dates are historical
facts for the human project-management surface.

Impact:

- a completed task with a wrong date can block the schedule;
- correcting it is allowed only as an explicit operator decision;
- future helpers should keep this distinction rather than silently overriding
  completed work.

### Status Mapping Is Not Task Ownership

OpenProject status is a source fact. RePlan `tracking_status` is accepted into
canonical state only through explicit import acceptance or a reviewed local
actual/status operation.

Impact:

- import may show `mapped_status`;
- `mapped_status` should not silently mutate RePlan tasks;
- status push to OpenProject must remain narrow and reviewed;
- the full dogfood cycle needs a clear rule for when an OpenProject status
  change becomes an accepted RePlan tracking fact.

### Private Binding Layer

Committed RePlan examples use sanitized source refs such as
`op.example.internal` and fake work package ids. Live OpenProject ids, URLs and
relation ids belong in ignored private storage:

```text
private/openproject/replan-self-bindings.yaml
```

Impact:

- public canonical data can stay reproducible;
- live dogfood can still preserve exact OpenProject identity;
- helpers must load private bindings for live drift and writes;
- examples must never be regenerated from live bindings without sanitization.

### Delete And Recreate Is The Wrong Default

Creating a fresh work package is acceptable during controlled bootstrap when no
matching live work package exists. It is not acceptable as the default way to
clean up an existing plan.

Impact:

- live `replan-self` refactor must preserve existing work package ids;
- destructive operations require a separate explicit gate;
- duplicate work packages are a hard drift signal;
- title matching is only a bootstrap aid, not a permanent identity mechanism.

### Live Access Is Operational State

Live OpenProject access uses runtime credentials and environment variables.
The public repository must not contain tokens, private endpoint details or real
private pilot narrative.

Impact:

- live commands should accept `OPENPROJECT_TOKEN` or configured token env names;
- helper output should avoid printing secrets;
- private outputs belong under ignored `private/` or `tmp/` paths;
- documentation should use generic URLs and sanitized ids.

### OpenProject Web UI Is Useful But Not Enough For Agents

The OpenProject web UI is the right human surface for task lists, statuses,
dates, relations and Gantt. It is not the right primary interface for AI CLI
agents.

Impact:

- humans should keep using OpenProject views;
- agents should operate through stable CLI commands and structured JSON;
- browser automation is a last resort for inspection, not the main mutation
  path;
- RePlan must make the operator loop reproducible from the repository.

## Existing RePlan Capabilities

RePlan already has enough primitives to build the preflight layer:

- `replan import openproject` reads OpenProject work packages from file or live
  API;
- `--include-closed` imports closed work packages;
- `--include-relations` imports relation candidates;
- `--include-time-entries` imports time-entry candidates;
- `replan import accept` can accept selected local candidates with dry-run and
  apply separation;
- `scripts/check-openproject-live-alignment.sh` checks live task matching;
- `scripts/check-openproject-drift.sh` checks task, status, date and relation
  drift against private bindings;
- `scripts/apply-openproject-bootstrap.sh` can create or match work packages
  during controlled bootstrap;
- `scripts/apply-openproject-relations.sh` creates missing relations after task
  bindings exist;
- `scripts/push-openproject-date-drift.sh` pushes reviewed date corrections;
- `scripts/push-openproject-status-drift.sh` pushes reviewed status
  corrections;
- `scripts/check-resource-schedule-drift.sh` verifies the RePlan resource side
  after task/date changes.

The gap is ergonomics and command shape, not lack of all primitives.

## External Tool Findings

### Official OpenProject MCP Server

OpenProject documents an MCP server integration. The currently relevant point
for RePlan is that it is useful as a read-oriented AI context surface, but it is
not the main write path for the RePlan dogfood loop.

Fit:

- good candidate for read-only project context;
- useful for AI assistants that need project/task visibility;
- not sufficient as the audited writer for resource-sensitive plan changes.

Reference:

- `https://www.openproject.org/docs/system-admin-guide/integrations/mcp-server/`.

### Official OpenProject CLI

The `opf/openproject-cli` repository exists, but it should be treated as an
auxiliary operator tool rather than the foundation of RePlan's automation
contract.

Fit:

- useful to inspect command ideas and API coverage;
- potentially useful for manual operations;
- not a replacement for RePlan-owned guarded helpers because the RePlan loop
  needs resource-aware dry-runs, private binding handling, audit evidence and
  drift-specific decisions.

Reference:

- `https://github.com/opf/openproject-cli`.

### Direct OpenProject API

Direct API use is necessary under the hood, but it should be hidden behind
small helper commands.

Fit:

- best low-level integration substrate;
- good for exact control over status, date and relation operations;
- risky when used as ad hoc `curl` snippets by agents.

## Required AI CLI First Operating Layer

The next design step should define a small command surface with these
properties:

- inspect current live project;
- list mapped work packages;
- list unmapped live work packages;
- list local tasks without live binding;
- show task, status, date and relation drift;
- preview safe OpenProject updates;
- apply only after explicit operator approval;
- reject destructive operations by default;
- emit stable JSON envelopes;
- include enough identifiers for traceability without leaking private data into
  committed examples;
- write or reference RePlan evidence and audit records for accepted changes.

## Recommended Minimum Commands

The exact names are for the next design task, but the operating layer should
cover these workflows:

```text
inspect live OpenProject project
show mapped and unmapped work packages
show OpenProject/RePlan drift
preview title, status, date and relation updates
apply one reviewed update family
record evidence for the mutation
rerun drift and resource schedule checks
```

The implementation can initially be scripts around the existing Go CLI. It does
not need to become a new daemon, plugin or full SDK.

## Decision

Proceed with the next preflight task:

```text
task.self.evaluate-openproject-agent-interfaces
```

That task should compare the official MCP server, official CLI, direct API and
RePlan-owned helpers in more detail, then feed the design of:

```text
task.self.design-openproject-ai-cli-operating-layer
```

The live `replan-self` OpenProject project should not be refactored until this
preflight sequence gives agents a reliable read/preview/apply workflow.
