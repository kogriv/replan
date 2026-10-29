# Resource planner lite — design note, 2026-07-02

## Status

Draft decision note.

This document refines the previous PM-substrate research after discovering that
the OpenProject features closest to the operator's MS Project expectation are
not fully available in the free Community edition.

Main decision:

**Do not fork OpenProject or Plane first. Do not use Obsidian as the operational
PM core. Build a narrow external resource-planning layer on top of an existing
task substrate.**

## Context

The operator's actual requirement is not just "Jira alternative".

The requirement is:

- plan real work for operator + a small number of human developers;
- understand who is overloaded and when;
- use task estimates, start dates, finish dates, dependencies, and priorities;
- keep the workflow compatible with future AI actors;
- avoid expensive Enterprise plans when the team has only 3-5 active people;
- keep data portable and inspectable;
- avoid a large maintenance fork.

OpenProject Community is still attractive because it provides:

- self-hosting;
- work packages;
- statuses, types, priorities, custom fields;
- Gantt and dependencies;
- time tracking fields;
- REST API and webhooks;
- GitHub/GitLab integration;
- mature project model.

However, the following facts change the plan:

- OpenProject Team Planner is an Enterprise add-on.
- OpenProject baseline comparison is an Enterprise add-on.
- OpenProject MCP Server is an Enterprise add-on.
- OpenProject's own resource-management documentation says it does not provide
  automated detailed resource management or employee capacity calculations. It
  recommends configured views, sums, Gantt, and Team Planner as a workaround.
- OpenProject Enterprise on-premises has minimum-user pricing that is too high
  for the current team size.

So OpenProject remains useful as a task/project substrate, but not sufficient as
the whole resource-planning answer.

## Problem statement

We need a low-cost, self-hostable resource-planning system that can answer:

1. Who is assigned to what?
2. How many hours are planned per person per week?
3. Where is a person over capacity?
4. Which tasks are blocked by dependencies?
5. Which deadlines are impossible under current capacity?
6. What changes if a task estimate changes?
7. What changes if a human developer is unavailable for several days?
8. Can an AI agent create/update work items and add evidence without corrupting
   the planning model?

Neither a plain issue tracker nor Obsidian plugins answer this robustly.

## Non-goals

This is not a plan to build a full OpenProject clone.

Non-goals:

- full issue tracker;
- full wiki;
- full kanban board;
- authentication and permissions system;
- rich enterprise reporting;
- chat/comments system;
- CI/CD integration;
- replacing git hosting;
- replacing OpenProject or Plane as task substrate;
- implementing a complete MS Project clone.

The resource-planning layer should be intentionally narrow.

## Options considered

### Option A — Pay for OpenProject Enterprise

Pros:

- official Team Planner;
- official baseline comparison;
- official MCP Server;
- less custom code;
- single product for many PM concerns.

Cons:

- expensive for 3-5 people because of minimum-user pricing;
- still does not provide full MS Project-style automated resource leveling;
- Enterprise dependency creates future cost pressure;
- MCP becomes paywalled.

Conclusion:

Use Enterprise trial for evaluation only. Do not assume Enterprise as default
operating mode.

### Option B — Fork OpenProject

Pros:

- OpenProject is GPL and source-available;
- in theory, Enterprise add-ons are also open-source;
- possible to customize exactly what is needed.

Cons:

- large Rails + Angular codebase;
- high maintenance cost;
- hard upgrades;
- Enterprise feature boundaries and token logic create social/legal/maintenance
  friction even if GPL rights exist;
- resource planning is still not a full scheduling engine;
- building inside OpenProject makes the team dependent on OpenProject internals.

Conclusion:

Do not fork first. A fork is a last resort after a long pilot proves OpenProject
is the right base and the missing feature is small and stable.

### Option C — Fork Plane

Pros:

- modern UX;
- strong REST API, webhooks, MCP, and agent fit;
- probably easier developer experience than OpenProject.

Cons:

- Plane is an issue/product workflow tool, not a resource-planning system;
- adding MS Project-like planning would be a large product inside another
  product;
- risk of fighting Plane's data model;
- still need scheduling/resource engine.

Conclusion:

Do not fork first. Use Plane as an agent-flow challenger, not as the resource
planning base.

### Option D — Obsidian with plugins

Possible plugins/classes of plugins:

- Tasks;
- Dataview;
- Kanban;
- Calendar / Full Calendar;
- Projects;
- charts/tables plugins;
- custom scripts over markdown.

Pros:

- excellent personal thinking environment;
- markdown-first;
- local-first;
- easy to inspect and version;
- good for weekly reviews, decision logs, design notes, and research.

Cons:

- weak multi-user workflow;
- weak audit trail for operational task state;
- no robust permission/status model;
- resource planning becomes custom convention in markdown;
- agents must parse and mutate free-form text;
- easy to create a beautiful but non-operational knowledge base;
- conflict resolution and concurrent updates are painful.

Conclusion:

Use Obsidian as the knowledge and review layer, not as the operational PM core.
Obsidian-only is likely a trap for this use case.

### Option E — Desktop MS Project-like FOSS tools

Candidates:

- ProjectLibre Desktop;
- GanttProject;
- TaskJuggler.

Pros:

- strong planning concepts compared to issue trackers;
- ProjectLibre and GanttProject are familiar MS Project-like desktop tools;
- TaskJuggler has automatic resource leveling and conflict resolution;
- low or zero cost.

Cons:

- mostly not collaborative web substrates;
- not natural for AI actors;
- weak API/MCP story;
- hard to make them the team source of truth;
- desktop files can become bottlenecks.

Conclusion:

Use them as references or engines, not as the whole PM stack. TaskJuggler is the
most interesting because it can be used as a scheduling/resource-leveling
engine from generated text files.

### Option F — Build a narrow external resource-planner-lite

Pros:

- targets exactly the missing part;
- avoids forking OpenProject/Plane;
- can use OpenProject Community or Plane as task source;
- can be markdown/export friendly;
- can use TaskJuggler as a calculation engine;
- can expose its own CLI/API for AI agents;
- controllable scope.

Cons:

- still custom software;
- requires disciplined schema;
- will not be polished at first;
- needs sync logic and data ownership rules;
- scheduling algorithms can become complex if scope is allowed to grow.

Conclusion:

This is the recommended direction.

## Recommended architecture

Use a layered stack:

```text
OpenProject Community or Plane
    task identity, status, assignee, basic planning fields, comments, links

resource-planner-lite
    capacity model, workload calculation, overload detection, schedule scenarios

TaskJuggler or internal scheduler
    optional resource-leveling / conflict-resolution engine

Git host
    branches, pull requests, code review, CI, release evidence

Obsidian
    research, decision logs, architecture, weekly reviews, exported summaries

AI actors
    operate through explicit APIs/CLI, not free-form markdown mutation
```

The important separation:

- task substrate owns task identity and status;
- resource planner owns capacity and scheduling analysis;
- git host owns code execution truth;
- Obsidian owns human-readable thinking and decisions.

## Data ownership

### Source of truth

OpenProject or Plane should own:

- task ID;
- task title;
- task status;
- task type;
- task priority;
- assignee;
- project;
- links to PRs and artifacts;
- high-level start/finish fields if available.

resource-planner-lite should own:

- person capacity calendar;
- holidays and unavailability;
- role/skill tags;
- planning scenario versions;
- calculated workload by person/day/week;
- overload warnings;
- suggested schedule changes;
- mapping of task estimate to scheduling assumptions.

Git host should own:

- repository;
- branch;
- pull request;
- CI status;
- code review;
- merge status.

Obsidian should own:

- reasoning;
- ADRs;
- research notes;
- weekly review narratives;
- manual reflections;
- exported snapshots.

### Avoid split-brain

The resource planner may write back only a small controlled set of fields:

- proposed start date;
- proposed finish date;
- proposed assignee;
- planning note/comment;
- custom field with schedule scenario ID;
- custom field with calculated load warning.

It should not independently change task status unless explicitly configured.

## Minimal data model

### Person

Fields:

- `id`;
- `display_name`;
- `role`;
- `skills`;
- `default_weekly_capacity_hours`;
- `default_daily_capacity_hours`;
- `timezone`;
- `active`.

Example:

```yaml
id: dev_ivan
display_name: Ivan
role: developer
skills: [python, frontend, devops]
default_weekly_capacity_hours: 24
default_daily_capacity_hours: 5
timezone: Asia/Novosibirsk
active: true
```

### Availability exception

Fields:

- `person_id`;
- `date`;
- `capacity_hours`;
- `reason`.

Examples:

```yaml
- person_id: dev_ivan
  date: 2026-07-10
  capacity_hours: 0
  reason: vacation

- person_id: operator
  date: 2026-07-11
  capacity_hours: 2
  reason: meetings
```

### Task planning record

Fields:

- `external_system`;
- `external_id`;
- `title`;
- `project`;
- `status`;
- `priority`;
- `assignee_id`;
- `estimate_hours`;
- `remaining_hours`;
- `start_date`;
- `finish_date`;
- `deadline`;
- `dependencies`;
- `required_skills`;
- `fixed_date_policy`;
- `planning_mode`;
- `pr_url`;
- `evidence_url`;

Example:

```yaml
external_system: openproject
external_id: 1234
title: Add importer retry policy
project: demo
status: ready
priority: high
assignee_id: dev_ivan
estimate_hours: 12
remaining_hours: 12
start_date: 2026-07-06
finish_date: 2026-07-09
deadline: 2026-07-12
dependencies: [1228, 1230]
required_skills: [python]
fixed_date_policy: flexible
planning_mode: manual
pr_url: null
evidence_url: null
```

### Scenario

Fields:

- `scenario_id`;
- `created_at`;
- `source_snapshot_id`;
- `description`;
- `assumptions`;
- `status`;
- `outputs`.

Scenarios matter because planning is not just current truth. We need "what if"
analysis:

- what if Ivan is unavailable next week?
- what if task A doubles in estimate?
- what if agent-coder can take low-risk tasks?
- what if release deadline is fixed?

## Core functions

### Import

Inputs:

- OpenProject API or Plane API;
- optional git host API;
- local capacity config.

Outputs:

- normalized task planning records;
- person records;
- dependency graph.

The importer must be deterministic and repeatable.

### Workload calculation

For each task:

1. Determine remaining hours.
2. Determine assignee.
3. Determine date range.
4. Spread hours across available working days.
5. Sum by person/day/week.
6. Compare with capacity.
7. Emit overload warnings.

Initial spreading can be simple:

```text
remaining_hours / number_of_available_days_in_task_range
```

Later spreading can account for:

- priority;
- fixed deadline;
- task type;
- focus blocks;
- unavailable days;
- dependency order.

### Dependency analysis

Required checks:

- dependency is missing;
- dependency is not done before dependent task starts;
- dependency chain makes deadline impossible;
- circular dependencies;
- blocked task assigned too early.

### Schedule suggestions

The system should not auto-write schedule changes initially.

It should propose:

- move task start date;
- move task finish date;
- split task;
- reassign task;
- reduce scope;
- move deadline;
- use agent-coder for specific low-risk task.

Human approval remains required.

### Write-back

Write-back should be controlled by explicit commands:

```bash
resource-planner apply scenario-2026-07-02-a --dry-run
resource-planner apply scenario-2026-07-02-a --write-openproject
```

Write-back records should include:

- who/what applied it;
- source scenario ID;
- changed fields;
- before/after values;
- timestamp.

### Export

Required exports:

- markdown weekly resource report;
- CSV workload table;
- JSON normalized snapshot;
- optional TaskJuggler project file;
- optional HTML static report.

## TaskJuggler integration

TaskJuggler is interesting because it is explicitly a Free and Open Source
project scheduling tool with automatic resource leveling and conflict
resolution.

Possible use:

1. Export normalized tasks/resources to `.tjp`.
2. Run TaskJuggler.
3. Parse generated reports.
4. Import suggested schedule back into resource-planner-lite as a scenario.
5. Human reviews the scenario.
6. Approved changes are written back to OpenProject/Plane.

TaskJuggler should not be the UI. It should be an engine.

Risks:

- syntax/model learning curve;
- mismatch between TaskJuggler scheduling model and OpenProject/Plane task
  model;
- report parsing may be brittle;
- too much automation can produce schedules that are technically valid but
  operationally unrealistic.

Mitigation:

- start with export-only proof of concept;
- keep human approval;
- use TaskJuggler outputs as suggestions, not truth.

## UI options

### Phase 1 UI

No complex UI.

Use:

- CLI;
- generated markdown reports;
- generated CSV;
- maybe static HTML.

### Phase 2 UI

Simple web UI:

- resource heatmap by week;
- task table grouped by assignee;
- overload warnings;
- dependency warnings;
- scenario comparison;
- "copy suggestion" or "apply with approval" buttons.

Possible frontend libraries:

- simple server-rendered HTML;
- React/Vue/Svelte if already used in adjacent project;
- Frappe Gantt for simple Gantt visualization.

Avoid building a full PM UI.

## CLI design

Examples:

```bash
resource-planner import --from openproject --project demo
resource-planner snapshot create --name 2026-07-02-base
resource-planner workload --week 2026-W28
resource-planner report weekly --format markdown
resource-planner scenario create --move-task OP-1234 --start 2026-07-10
resource-planner scenario compare base scenario-001
resource-planner export taskjuggler --scenario scenario-001
resource-planner taskjuggler run --scenario scenario-001
resource-planner apply scenario-001 --dry-run
resource-planner apply scenario-001 --write-openproject
```

The CLI is important because AI agents can use it safely.

## AI actor interface

AI actors should not edit markdown planning files directly.

Allowed actions:

- read current assigned tasks;
- claim a task if allowed;
- write progress comment;
- attach evidence;
- propose estimate change;
- propose schedule change;
- request human approval for risky changes.

Forbidden initially:

- changing deadlines;
- changing another human's assignments;
- mass rescheduling;
- deleting tasks;
- changing project status;
- applying scenarios without human approval.

The resource planner should expose explicit commands or API endpoints for AI
actors. This keeps the model from inventing planning rules.

## Obsidian role

Obsidian remains valuable, but only in the right layer.

Use Obsidian for:

- PM strategy notes;
- weekly review;
- decision records;
- research;
- personal prioritization;
- links to reports;
- imported markdown snapshots from resource-planner-lite.

Do not use Obsidian for:

- canonical task status;
- multi-user assignment truth;
- capacity calculations;
- automated scheduling;
- concurrent agent updates;
- operational audit log.

Recommended Obsidian integration:

- resource-planner-lite writes `weekly-resource-report.md`;
- Obsidian indexes that report;
- operator writes narrative review next to it;
- decisions are linked back to OpenProject/Plane task IDs.

## Plugin strategy

Do not start with plugin development.

Preferred order:

1. External CLI and reports.
2. Static HTML report.
3. Small web UI.
4. Optional Obsidian plugin only if the report workflow proves valuable.
5. Optional OpenProject/Plane plugin only after the data model is stable.

Reason:

Plugins bind us to a host application's UI and lifecycle too early.

## Implementation phases

### Phase A — Manual model validation

Duration: 2-3 days.

Actions:

- create 10-20 real tasks in OpenProject Community or a YAML fixture;
- define 3 people and their weekly capacity;
- manually calculate workload in a spreadsheet or simple script;
- identify required fields and annoying edge cases.

Exit criteria:

- we know the minimum schema;
- we can produce a useful weekly load table;
- operator confirms the view is operationally useful.

### Phase B — CLI snapshot and report

Duration: 3-5 days.

Actions:

- implement importer for OpenProject API or fixture first;
- normalize tasks/resources into SQLite or JSON;
- generate weekly markdown/CSV report;
- detect overload by person/week;
- detect missing estimates and missing dates.

Exit criteria:

- one command generates a report that answers "who is overloaded next week?";
- report is readable in Obsidian;
- no write-back yet.

### Phase C — Scenario engine

Duration: 1 week.

Actions:

- add scenario records;
- support moving task dates in a scenario;
- support changing assignee in a scenario;
- compare base vs scenario;
- generate before/after workload table.

Exit criteria:

- operator can test "what if Ivan is unavailable?" without changing the source
  PM tool.

### Phase D — Write-back with approval

Duration: 2-4 days.

Actions:

- implement dry-run diff;
- implement limited OpenProject write-back for approved fields;
- write comments with scenario ID and reason;
- keep audit log.

Exit criteria:

- a scenario can be safely applied to OpenProject with clear before/after
  output.

### Phase E — TaskJuggler proof of concept

Duration: 3-5 days.

Actions:

- export `.tjp`;
- run TaskJuggler;
- inspect generated schedule/resource reports;
- decide whether TaskJuggler is useful as an engine.

Exit criteria:

- clear decision: use TaskJuggler as engine, or keep internal simple scheduler.

### Phase F — Agent interface

Duration: 1 week.

Actions:

- define CLI commands safe for AI actors;
- add read-only commands first;
- add proposal commands;
- reject direct apply by default;
- document policy.

Exit criteria:

- an AI CLI agent can read assignments and propose planning changes without
  corrupting source-of-truth state.

## Suggested repository placement

Two viable locations:

1. Inside `orcode` as an experiment:

```text
experiments/resource-planner-lite/
```

2. Separate project:

```text
resource-planner-lite/
```

Recommendation:

Start inside `orcode` as an experiment until the schema and value are proven.
Split into a separate project only after it becomes an actual reusable tool.

## Decision rules

### Continue with resource-planner-lite if

- OpenProject Community or Plane is good enough for task identity/status;
- resource reports are useful after one week;
- operator actually uses the weekly workload view;
- implementation stays below roughly 1-2 weeks for MVP;
- no full PM UI creep appears.

### Stop and choose another tool if

- the resource model becomes too complex immediately;
- OpenProject/Plane API friction dominates;
- manual work remains higher than MS Project/ProjectLibre;
- operator does not use the reports;
- scheduling requires full-blown project-management theory before any value.

### Consider paying for OpenProject Enterprise only if

- Community plus resource-planner-lite fails specifically because Team Planner
  or MCP is essential;
- Enterprise trial proves the feature solves the problem cleanly;
- real team size or revenue justifies the minimum-user cost.

### Consider forking only if

- we have used the host tool for several months;
- the missing feature is small and well-defined;
- external API integration cannot solve it;
- we are ready to maintain a long-lived fork.

This is deliberately a high bar.

## Current recommendation

The pragmatic path is:

1. Use OpenProject Community as the task/project substrate.
2. Keep Obsidian as the thinking and review layer.
3. Build `resource-planner-lite` as a narrow external planning/reporting layer.
4. Use TaskJuggler as a candidate scheduling engine, not as the UI.
5. Keep Plane/YouTrack as challengers for agent-native workflows.
6. Avoid forks and plugins until the data model proves itself.

This preserves the useful parts of OpenProject without paying Enterprise prices
or taking on a maintenance fork.

