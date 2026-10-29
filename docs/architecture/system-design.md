# RePlan system design

Date: 2026-07-02

Status: initial architecture.

## Core architectural decision

RePlan must be source-agnostic.

OpenProject may be the first real task source, but RePlan must not become an
OpenProject plugin or an OpenProject-shaped database.

Rule:

```text
OpenProject is the first supported source, not the domain model.
```

Second rule:

```text
external authentication first, internal policy boundary always
```

RePlan should not implement its own authentication provider in the MVP. It
should still define principals, actors, resources, scopes, capabilities and
audit records from the beginning. See
[`../security/access-and-actor-model.md`](../security/access-and-actor-model.md).

Third rule:

```text
resource abstraction first, resource class enumeration second
```

RePlan should not try to list every possible kind of resource up front. The
core model should represent capacity, units, reservations, consumption,
constraints, gateways and policy. Built-in resource kinds are templates. See
[`resource-abstraction.md`](resource-abstraction.md).

Fourth rule:

```text
local-first is the first deployment mode, not a permanent architectural prison
```

RePlan MVP should run as a local CLI/library with file-based state and
Git-backed collaboration where useful. A future client-server deployment must
remain possible by keeping domain logic, application services, storage,
adapters, reports and UI/API layers separate. See
[`deployment-architecture.md`](deployment-architecture.md).

Fifth rule:

```text
headless-first, CLI-required, daemon-optional, UI-later
```

RePlan's required MVP runtime should be a CLI over reusable
core/application services. A local daemon/service can be added later for
watching, caching, background imports and local API support. Desktop/web UI is
a later shell over the same operations and report models. See
[`application-runtime.md`](application-runtime.md).

## High-level architecture

```text
Task sources
    OpenProject
    Plane
    GitHub Issues
    GitLab Issues
    YAML fixtures
    CSV exports
    future systems

        |
        v

Source adapters
    read tasks
    read users/resources
    read dependencies
    map statuses
    write approved changes

        |
        v

Canonical planning model
    tasks
    resources
    calendars
    estimates
    dependencies
    assignments
    scenarios
    baselines

        |
        v

Resource planning engine
    workload calculation
    capacity comparison
    overload detection
    dependency checks
    scenario calculation
    baseline comparison

        |
        +----------------------+
        |                      |
        v                      v

Reports                 Optional schedulers
    markdown                TaskJuggler
    CSV                     internal scheduler
    JSON                    future engines
    HTML

        |
        v

Write-back adapters
    dry-run first
    approved changes only
    comments
    proposed dates
    custom fields
    scenario IDs
```

## Why source-agnostic design matters

If RePlan becomes OpenProject-specific too early, it will be only a workaround
for OpenProject Community limitations.

That would be too narrow.

The real problem is broader:

- issue trackers track work;
- planning engines calculate schedules;
- teams need resource/capacity visibility across tools;
- AI agents need safe operational interfaces.

Therefore RePlan should use a canonical planning model that can be populated
from several sources.

## Source adapters

Each source adapter should be responsible for all source-specific behavior.

An adapter reads from an external system and maps data into the canonical model.

An adapter may also write approved changes back to the external system.

### OpenProject adapter

Responsibilities:

- read work packages;
- read assignee;
- read status;
- read priority;
- read start date;
- read finish date;
- read due date if available;
- read estimated time;
- read spent time if available;
- read relations/dependencies;
- read custom fields;
- map OpenProject users to RePlan resources;
- write comments;
- write approved planning dates;
- write approved custom fields;
- store scenario ID or planning note.

The OpenProject adapter should not define the RePlan task model.

### Plane adapter

Responsibilities:

- read issues;
- read state;
- read assignee;
- read priority;
- read labels;
- read project/module/cycle if useful;
- map Plane users to RePlan resources;
- write comments or proposed changes if supported.

Plane is likely interesting for agent workflows, but it should not be the first
resource-planning source unless OpenProject proves unusable.

### GitHub Issues adapter

Responsibilities:

- read issues;
- read labels;
- read assignees;
- read milestones;
- read linked pull requests if available;
- parse estimate/deadline metadata from labels, issue body or project fields;
- write planning comments;
- optionally update labels or project fields.

GitHub Issues is convenient but does not naturally carry rich scheduling data.
It is a good secondary source, not necessarily a first-class resource-planning
environment.

### GitLab Issues adapter

Similar to GitHub Issues.

Potential advantages:

- self-hosting;
- issue weights;
- milestones;
- labels;
- merge request linkage.

### YAML/CSV adapter

This adapter is essential even if it is not the long-term user-facing source.

Reasons:

- deterministic tests;
- quick examples;
- no dependency on OpenProject availability;
- easy documentation;
- import from spreadsheets;
- manual prototyping.

The first MVP can start from YAML/CSV fixtures even before the OpenProject API
is wired.

## Canonical model

The canonical model should be the center of RePlan.

External systems map into it. Reports and schedulers read from it. Write-back
diffs are generated from it.

### Resource

A resource may be:

- human developer;
- operator;
- reviewer;
- designer;
- DevOps person;
- AI coding agent;
- AI research agent;
- external contractor;
- constrained machine;
- robot;
- lab equipment;
- deployment environment;
- external contractor block of hours.
- API quota;
- LLM token budget;
- cloud budget;
- SaaS subscription seat pool;
- rate-limited external service;
- CI/build minutes.

Fields:

- `id`;
- `display_name`;
- `kind`;
- `role`;
- `skills`;
- `timezone`;
- `default_weekly_capacity_hours`;
- `default_daily_capacity_hours`;
- `active`;
- `cost_rate` optional;
- `risk_profile` optional;
- `approval_required` optional.
- `scope` optional.

Some resources are consumable or entitlement-based rather than time-calendar
based. Examples include API quotas, subscriptions, token budgets, cloud spend
and CI minutes. They still belong in the resource model because they constrain
work and agent behavior. See
[`../security/access-and-actor-model.md#consumable-and-entitlement-resources`](../security/access-and-actor-model.md#consumable-and-entitlement-resources).

The deeper resource abstraction is documented in
[`resource-abstraction.md`](resource-abstraction.md).

Example:

```yaml
id: dev_ivan
display_name: Ivan
kind: human
role: developer
skills: [python, frontend, devops]
timezone: Asia/Novosibirsk
default_weekly_capacity_hours: 24
default_daily_capacity_hours: 5
active: true
```

Example AI resource:

```yaml
id: agent_coder_1
display_name: Agent Coder 1
kind: ai_agent
role: coding_agent
skills: [python, tests, docs]
timezone: UTC
default_weekly_capacity_hours: 30
default_daily_capacity_hours: 6
active: true
approval_required: true
risk_profile: medium
```

### Availability exception

Fields:

- `resource_id`;
- `date`;
- `capacity_hours`;
- `reason`;
- `source`;

Example:

```yaml
- resource_id: dev_ivan
  date: 2026-07-10
  capacity_hours: 0
  reason: vacation
  source: manual
```

### Task

Fields:

- `canonical_id`;
- `source`;
- `source_id`;
- `title`;
- `project`;
- `status`;
- `priority`;
- `type`;
- `assignee_resource_id`;
- `estimate_hours`;
- `remaining_hours`;
- `spent_hours`;
- `start_date`;
- `finish_date`;
- `deadline`;
- `dependencies`;
- `required_skills`;
- `fixed_date_policy`;
- `planning_mode`;
- `external_url`;
- `pr_url`;
- `evidence_url`;
- `metadata`;

Example from OpenProject:

```yaml
canonical_id: openproject:1234
source: openproject
source_id: "1234"
title: Add importer retry policy
project: demo
status: ready
priority: high
type: task
assignee_resource_id: dev_ivan
estimate_hours: 12
remaining_hours: 12
spent_hours: 0
start_date: 2026-07-06
finish_date: 2026-07-09
deadline: 2026-07-12
dependencies:
  - openproject:1228
  - openproject:1230
required_skills: [python]
fixed_date_policy: flexible
planning_mode: manual
external_url: https://openproject.example/work_packages/1234
pr_url: null
evidence_url: null
metadata: {}
```

Example from GitHub:

```yaml
canonical_id: github:example/orcode#42
source: github
source_id: example/orcode#42
title: Add importer retry policy
project: orcode
status: ready
priority: high
type: issue
assignee_resource_id: dev_ivan
estimate_hours: 12
remaining_hours: 12
spent_hours: null
start_date: null
finish_date: null
deadline: 2026-07-12
dependencies: []
required_skills: [python]
fixed_date_policy: flexible
planning_mode: inferred
external_url: https://github.com/example/orcode/issues/42
pr_url: null
evidence_url: null
metadata:
  labels: [planning, backend]
```

### Scenario

Fields:

- `scenario_id`;
- `name`;
- `created_at`;
- `created_by`;
- `source_snapshot_id`;
- `description`;
- `assumptions`;
- `changes`;
- `outputs`;
- `status`;

Scenarios are central because RePlan is not only about current truth. It is
about asking controlled "what if" questions.

Example:

```yaml
scenario_id: scenario-2026-07-02-ivan-unavailable
name: Ivan unavailable for three days
created_at: 2026-07-02T12:00:00+07:00
created_by: operator
source_snapshot_id: snapshot-2026-07-02-base
description: Test impact of Ivan being unavailable from July 8 to July 10.
assumptions:
  - dev_ivan capacity is zero for three days
changes:
  availability:
    - resource_id: dev_ivan
      date: 2026-07-08
      capacity_hours: 0
    - resource_id: dev_ivan
      date: 2026-07-09
      capacity_hours: 0
    - resource_id: dev_ivan
      date: 2026-07-10
      capacity_hours: 0
outputs: {}
status: draft
```

## Internal calculation without TaskJuggler

RePlan should have a simple internal calculation mode.

Minimum algorithm:

1. Import tasks and resources.
2. Determine remaining hours for each task.
3. Determine assigned resource.
4. Determine task date range.
5. Spread remaining hours across available workdays.
6. Sum planned hours by resource/day/week.
7. Compare planned hours with available capacity.
8. Emit overload warnings.
9. Emit missing-data warnings.
10. Emit dependency warnings.

This is not full project scheduling. It is still useful because it answers the
first operational question:

```text
who is overloaded next week?
```

## Optional TaskJuggler calculation

When internal calculation is not enough, RePlan can export a scenario to
TaskJuggler.

TaskJuggler then calculates a resource-aware schedule. RePlan imports the result
as another scenario and presents it as a proposal.

TaskJuggler output should not become source truth automatically.

## Write-back design

Write-back must be explicit, adapter-based and safe by default.

### Allowed first write-back operations

- add planning comment;
- write proposed start date;
- write proposed finish date;
- write scenario ID to custom field;
- write load warning to custom field;
- attach generated report URL;
- update planning metadata field.

### Forbidden first write-back operations

- silently change task status;
- silently reassign another human's work;
- silently move deadlines;
- delete tasks;
- mass-update schedules without approval;
- let AI actors apply changes directly.

### Dry-run diff

Every write-back should support dry-run output:

```text
Task openproject:1234
  start_date: 2026-07-06 -> 2026-07-10
  finish_date: 2026-07-09 -> 2026-07-15
  comment: add scenario explanation
```

Only after review should write-back happen.

## Agent interface

The CLI and API should be designed so that AI actors can use them safely.

Agent identity and schedulable resource identity should be separate concepts.
An AI agent can be both an actor and a resource, but a sync worker can be only
an actor, and a GPU machine can be only a resource.

### Read commands

Examples:

```bash
replan task list --assigned-to agent_coder_1
replan workload --resource agent_coder_1 --week 2026-W28
replan scenario show scenario-001
```

### Proposal commands

Examples:

```bash
replan propose estimate openproject:1234 --remaining-hours 16
replan propose assignment openproject:1234 --resource agent_coder_1
replan propose dates openproject:1234 --start 2026-07-10 --finish 2026-07-15
```

### Apply commands

AI actors should not have apply permission by default.

Human operator:

```bash
replan apply scenario-001 --dry-run
replan apply scenario-001 --write-openproject
```

### Authorization boundary

All proposal and apply commands should go through one central authorization
check:

```text
authorize(principal, actor, action, target, context)
```

MVP policy can be local YAML. Later policy can move to an external engine such
as OPA without changing the command model.

## Storage

Initial storage options:

- YAML files for fixtures and examples;
- JSON snapshots for deterministic imports;
- SQLite for local scenarios, audit log and calculated reports.

Recommendation:

- start with files plus SQLite;
- avoid requiring a server database for the CLI MVP;
- make export/import transparent.

## Reports

Minimum reports:

- weekly workload by resource;
- overload warnings;
- missing estimates;
- missing dates;
- blocked tasks;
- dependency violations;
- scenario comparison;
- TaskJuggler export summary;
- write-back dry-run summary.

Formats:

- markdown;
- CSV;
- JSON;
- later HTML.

Markdown is important because it can be read in Obsidian and committed to git.
CSV is important because spreadsheet review remains useful.
JSON is important because agents and tests need structured data.

## Design summary

RePlan should be:

- OpenProject-first, not OpenProject-only;
- TaskJuggler-compatible, not TaskJuggler-dependent;
- CLI-first, not UI-first;
- FOSS-first, not teaser-free;
- external-auth-first, not custom-auth-first;
- policy-boundary-aware, not enterprise-RBAC-heavy;
- resource-abstraction-first, not resource-class-enum-first;
- event-assisted for tracking, not event-naive;
- scenario-centered, not static-plan-centered;
- safe for AI proposals, not open-ended AI mutation;
- focused on resources, not a full PM-suite clone.
