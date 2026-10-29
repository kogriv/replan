# RePlan product positioning

Date: 2026-07-02

Status: working product direction.

## One-sentence positioning

RePlan is a FOSS-first resource management and capacity replanning layer for
software teams that coordinate human developers and AI agents through existing
task systems.

## Short version

RePlan should not compete with OpenProject, Plane, Jira, GitHub Issues or GitLab
Issues as a general task tracker.

RePlan should compete with the missing resource-planning layer around those
systems.

The product should answer:

- who can do the work;
- when the work can realistically happen;
- who is overloaded;
- what schedule becomes possible after replanning;
- what changed between baseline and current plan;
- which changes are only suggestions and need human approval.

## Why this project exists

The original investigation started from the need for an analogue of MS Project
and Jira/OpenProject for a small software effort:

- operator plus a few human developers;
- possible future AI actors;
- need for task planning;
- need for resource planning;
- need for capacity visibility;
- need for dependency and deadline reasoning;
- need for tool access through CLI, API or MCP-style interfaces.

OpenProject is still attractive as a task/project substrate. However, the
research found that the OpenProject features closest to this requirement are
not fully available in the free Community edition:

- Team Planner is an Enterprise add-on;
- baseline comparison is an Enterprise add-on;
- MCP Server is an Enterprise add-on;
- OpenProject documentation describes resource management as configured views,
  sums, Gantt and Team Planner rather than full automatic capacity planning and
  resource leveling.

For a tiny team, OpenProject Enterprise on-premises pricing is too high to be
the default direction.

TaskJuggler is attractive because it is a serious FOSS scheduling engine with
resource leveling, but it is not a modern multi-user operational product around
software-team tasks, evidence, approvals, pull requests and AI actors.

Therefore the gap is:

```text
existing task system
    good at identity, status, comments, issue workflow
    weak or expensive at resource/capacity planning

TaskJuggler
    good at scheduling/resource constraints
    weak as day-to-day team product and integration surface

RePlan
    resource/capacity layer between task systems and scheduling engines
```

The market is not empty. Dedicated resource-management products such as Runn,
Float, Resource Guru, Tempo Capacity Planner, Teamdeck, eResource Scheduler and
Scoro already exist. FOSS and desktop references such as TaskJuggler,
ProjectLibre, GanttProject and Quollnet Resource Planner also exist.

The RePlan thesis is narrower: a FOSS-first, self-hostable, source-agnostic
resource/capacity layer for software teams and AI agents. See
[`../research/market-gap-and-alternatives-2026-07-02.md`](../research/market-gap-and-alternatives-2026-07-02.md).

## Relationship to OrCode

RePlan is not an unrelated side project.

RePlan is the task/resource infrastructure prerequisite that emerged from the
OrCode research phase.

Current strategic relationship:

- OrCode remains paused.
- OrCode should not resume implementation before the task/resource/agent
  planning substrate is proven.
- RePlan is the extracted project that should prove that substrate.
- If RePlan fails to create useful operational planning, OrCode should not
  assume that a larger agentic development harness will be manageable.

Reason:

OrCode is not only a coding harness. In practice it needs an operating model for
human and AI actors:

- where tasks live;
- how tasks are assigned;
- how estimates are represented;
- how capacity is calculated;
- how overloads are detected;
- how AI agents propose changes;
- how humans approve changes;
- how execution evidence is linked back to planning.

RePlan is intended to solve that layer first.

The intended sequence is:

```text
1. RePlan proves task/resource planning on a real project.
2. RePlan proves safe human + AI actor planning workflows.
3. OrCode resumes implementation using that proven planning substrate.
```

## Target users

Initial user:

- one operator or technical lead;
- two to five human developers;
- several repositories;
- tasks in OpenProject, Plane, GitHub Issues, GitLab Issues or YAML fixtures;
- AI CLI agents beginning to participate in implementation;
- need to understand workload and schedule realism every week.

Later users:

- small software agencies;
- open-source maintainers coordinating humans and AI helpers;
- internal platform teams;
- research labs using human plus AI workflows;
- small product teams that dislike expensive enterprise PM tooling.

## Primary jobs to be done

### Weekly capacity review

As an operator, I want to see planned workload by person and week so that I can
detect overload before work fails.

### Scenario planning

As an operator, I want to test changes without modifying the real task tracker:

- person unavailable;
- estimate increased;
- task moved;
- task reassigned;
- deadline fixed;
- AI actor added;
- release scope reduced.

### Safe write-back

As an operator, I want RePlan to show a dry-run diff before changing OpenProject
or another task system so that planning remains auditable.

### AI-safe planning interface

As an operator, I want AI agents to read assignments and propose changes without
being able to silently corrupt dates, deadlines, assignments or project status.

## Non-goals

RePlan should not start as:

- a full Jira clone;
- a full OpenProject clone;
- a full MS Project clone;
- a full wiki;
- a chat system;
- a source-code hosting system;
- a CI/CD system;
- a permission-heavy enterprise suite;
- an Obsidian plugin as the core;
- an OpenProject plugin as the core.

The product should remain focused on resource planning and replanning.

## Product category

Possible category names:

- resource planning sidecar;
- capacity planning layer;
- resource-aware replanning engine;
- project scheduling adapter layer;
- human and AI workload planner.

The strongest category phrase is probably:

```text
resource-aware planning for human and AI teams
```

It explains why this is not just another resource scheduler. The AI-agent angle
is real because AI actors need explicit capacity, permissions, assignment rules,
evidence requirements and approval boundaries.

## Differentiation

### Against OpenProject Enterprise

OpenProject is a full PM suite. RePlan should not compete with that.

Difference:

- OpenProject sells many general PM capabilities;
- RePlan keeps resource/capacity planning as the core FOSS capability;
- OpenProject remains a possible task substrate;
- RePlan can work with other task substrates too;
- RePlan should expose CLI/API workflows designed for agents.

### Against Plane

Plane is a modern issue/product workflow tool. It is attractive for agent-native
task flows, but it is not primarily an MS Project-style resource planner.

Difference:

- Plane can own task flow;
- RePlan owns workload, capacity, scenarios and scheduling analysis.

### Against TaskJuggler

TaskJuggler is a powerful scheduling engine and project-as-code tool.

Difference:

- TaskJuggler is not the operational source of task truth for a software team;
- RePlan should import tasks from existing systems;
- RePlan should produce reports and approval workflows;
- RePlan can call TaskJuggler when advanced resource leveling is needed.

### Against Obsidian plugins

Obsidian is excellent for notes, decision records and weekly reviews.

Difference:

- Obsidian should consume RePlan reports;
- Obsidian should not be the source of operational task and capacity truth;
- AI agents should not freely mutate planning markdown as the primary API.

## Product principles

### Resource planning must remain core FOSS

The product exists because important resource planning features are often
missing or paywalled elsewhere. Therefore RePlan should not paywall the basic
ability to plan resources, calculate workload, compare scenarios or detect
overload.

### Source-agnostic core

OpenProject may be the first supported source, but the RePlan domain model must
not be OpenProject-specific.

### Explicit adapters

Every external system should be integrated through an adapter:

- OpenProject adapter;
- Plane adapter;
- GitHub Issues adapter;
- GitLab Issues adapter;
- YAML/CSV fixture adapter;
- TaskJuggler export/import adapter.

### Safe changes by default

Read-only import and generated reports should come first.

Write-back should start with:

- dry-run;
- comments;
- custom fields;
- proposed dates;
- scenario identifiers;
- explicit human approval.

### Human approval remains central

RePlan may suggest schedule changes. It should not silently move human work or
deadlines in production task systems.

### AI agents are resources and actors

AI agents can be modeled as resources with:

- capacity;
- allowed task types;
- cost;
- risk profile;
- review requirement;
- evidence requirement;
- approval boundary.

They are also actors that can call CLI/API commands. These commands must be
safe and explicit.

## Main risk

The main risk is scope creep into a complete PM system.

The project should not try to own everything. The first durable value is:

```text
import tasks -> calculate workload -> show overloads -> create scenarios ->
compare scenarios -> optionally export to TaskJuggler -> optionally write back
approved changes
```

If this loop is useful, product expansion becomes justified. If this loop is not
useful, a bigger UI will not save the project.
