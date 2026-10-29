# Quollnet Resource Planner deep dive, 2026-07-02

Status: research note.

Question:

Can Quollnet Resource Planner be used as the base for RePlan instead of building
our own resource-planning layer?

Short answer:

No, not as the core/base.

Yes, as a useful reference for a lightweight browser UI, JSON import/export,
timeline interaction and simple human/equipment allocation model.

## Sources inspected

Public pages:

- https://github.com/quollnet/resource-planner
- https://quollnet.com/article/resource-planner-open-source-app
- https://www.quollnet.com/resource-planner/

Local code inspection:

```bash
git clone --depth 1 https://github.com/quollnet/resource-planner.git /tmp/quollnet-resource-planner
```

Repository state observed:

- GitHub stars: 5;
- forks: 1;
- commits: 7;
- latest shallow commit inspected: `ef08e64 delegating to GPT-5.4`;
- no releases;
- no package registry package;
- static files only;
- approximately 4,058 lines across README, HTML, CSS, schema, sample JSON and
  JavaScript files.

## What it is

Quollnet Resource Planner is a small static browser application.

Technology:

- HTML;
- CSS;
- JavaScript ES modules;
- Bootstrap 5;
- `vis-timeline`;
- browser `localStorage`;
- JSON import/export.

Run model:

```text
open index.html in a browser
or deploy the static files through GitHub Pages
```

There is no backend server.

There is no database.

There is no API server.

There is no authentication.

There is no multi-user model.

There is no task-source integration.

## License status

README and website state that the project is open-source and available under
the MIT license.

However, the inspected repository did not contain a `LICENSE` file.

This is not necessarily fatal, but it matters if we consider reuse or fork.

Before reusing code directly, clarify license by:

- checking whether GitHub metadata later shows a license;
- asking upstream to add `LICENSE`;
- opening an issue or PR adding the MIT license file if the maintainer agrees.

Until then, treat the licensing as probably MIT but not perfectly packaged.

## Data model

The model is intentionally simple.

Top-level object:

```json
{
  "meta": {},
  "resources": [],
  "allocations": []
}
```

### Resource

Required by schema:

- `id`;
- `name`;
- `cost_per_hour`.

Other observed fields:

- `class`;
- `description`;
- `start`;
- `end`.

Example resource classes in the sample file:

- human;
- construction equipment - heavy;
- construction equipment - light;
- spaces;
- vehicles.

This is a strong point: Quollnet models people and non-human resources in one
resource list.

### Allocation

Required by schema:

- `id`;
- `resource_id`;
- `task`;
- `start`;
- `allocation_pct`;
- `baseline_start`;
- `cost`.

Other fields:

- `end`;
- `notes`;
- `baseline_end`;
- `baseline_cost`.

An allocation is basically:

```text
resource X is booked for task text Y from start to end at N percent
```

There is no separate Task entity.

There is no Project entity beyond `meta.planner_name`.

There is no dependency graph.

There is no assignee vs resource distinction.

There is no external source mapping.

## Functional surface

Useful features:

- add/edit/delete resources;
- add/edit/delete allocations;
- drag allocations on a timeline;
- drag allocations between resource rows;
- store plan in browser localStorage;
- import/export JSON plan files;
- calculate cost from hours and hourly rate;
- show baseline shadow on timeline;
- reset baseline;
- highlight overbooked intervals;
- calculate utilization;
- calculate idle hours;
- calculate schedule overrun from baseline end dates;
- calculate budget variance from baseline/current costs;
- display resource details and KPI drawers;
- support people, rooms, vehicles and equipment as resources.

## What is technically good

### Small and understandable

The codebase is small enough to understand quickly.

This is useful as a reference for the first RePlan UI because it demonstrates
that a browser-only visual scheduler can be built with modest code.

### Static deployment

It works as a local/static page.

This aligns with RePlan's preference to avoid heavy server infrastructure at
the beginning.

### JSON import/export

JSON plan import/export is useful and consistent with RePlan's preference for
portable, inspectable data.

### Non-human resources

The sample data includes people, heavy equipment, light equipment, rooms and
vehicles.

This is relevant to RePlan's actor/resource model because RePlan also wants to
support non-human resources eventually.

### Timeline interaction

Using `vis-timeline` gives:

- rows by resource;
- draggable allocations;
- baseline visualization;
- overbook background bands.

This is useful UI inspiration.

## What is missing for RePlan

### No backend

There is no server.

This means:

- no shared team state;
- no API;
- no connector jobs;
- no audit log;
- no external write-back;
- no long-running imports;
- no controlled agent interface.

For RePlan, backend-less can be useful for a demo, but not for the planning
substrate.

### No authentication or authorization

There is no auth.

No:

- users;
- principals;
- roles;
- scopes;
- capabilities;
- agent trust levels;
- connector identities.

This is acceptable for a static toy/single-user tool. It is not enough for
RePlan's intended access model.

### No canonical task model

Allocations contain a `task` string.

There is no:

- task ID;
- task status;
- task priority;
- task type;
- remaining estimate;
- dependency list;
- external URL;
- evidence URL;
- PR URL;
- source system mapping;
- task lifecycle.

This is the biggest model gap.

RePlan needs task identity and external-source mapping because it is meant to
connect OpenProject, Plane, GitHub, GitLab and fixtures.

### No source adapters

There are no adapters for:

- OpenProject;
- Plane;
- GitHub Issues;
- GitLab Issues;
- TaskJuggler;
- calendars;
- HR systems;
- equipment gateways.

The article claims modular design and possible API integration, but the
inspected repository is currently a static browser app with JSON/localStorage.

### No write-back model

There is no concept of:

- dry-run;
- approved write-back;
- comments to source systems;
- custom fields;
- scenario ID;
- before/after diff;
- connector actor;
- audit event.

This is central for RePlan.

### No scenario engine

There is baseline/current comparison, but not a scenario system.

Missing:

- named scenario;
- scenario created from snapshot;
- scenario assumptions;
- scenario diff;
- scenario approval;
- multiple what-if branches;
- scenario export/import;
- TaskJuggler-generated scenario.

### No dependency model

There is no dependency graph.

Missing:

- predecessor/successor tasks;
- critical path;
- blocked task warnings;
- circular dependency detection;
- dependency-aware schedule suggestions.

### No automatic scheduling or resource leveling

Quollnet can detect overbooking and show utilization.

It does not appear to:

- automatically resolve resource conflicts;
- level resources;
- move tasks based on dependencies;
- optimize schedule;
- allocate by skills;
- generate feasible schedules.

TaskJuggler remains much stronger for advanced scheduling.

### Hardcoded calendar assumptions

Working hours are hardcoded:

```text
Monday-Friday: 8 hours
Saturday: 4 hours
Sunday: 0 hours
```

There is no flexible calendar model for:

- per-resource daily capacity;
- holidays;
- vacations;
- regional calendars;
- timezone-specific working patterns;
- AI-agent capacity profiles;
- equipment maintenance windows.

### No tests or build discipline

Observed:

- no package manager manifest;
- no test suite;
- no lint/format config;
- no CI;
- no typed model;
- no releases.

This is fine for a small static app, but weak as a foundation for a planning
infrastructure project.

## Fit matrix

| Requirement | Quollnet fit | Notes |
| --- | --- | --- |
| FOSS/open-source | Partial | README says MIT, but no `LICENSE` file observed |
| Self-hostable | Yes | Static files can be served anywhere |
| Local-first | Yes | Browser localStorage and JSON files |
| Multi-user | No | No backend or shared persistence |
| External auth | No | Not designed for auth boundary |
| RBAC/capabilities | No | No access model |
| Human resources | Yes | Simple resources |
| Non-human resources | Yes, simple | Sample includes equipment, rooms, vehicles |
| AI agents as resources | Not explicitly | Could be represented as resource class, but no trust/action model |
| AI agents as actors | No | No API/CLI/policy |
| Task source adapters | No | No OpenProject/Plane/GitHub/GitLab |
| Canonical task identity | No | Allocation only has task text |
| Dependencies | No | No graph |
| Baseline | Partial | Baseline start/end/cost per allocation |
| Scenarios | No | No named scenario model |
| Overload detection | Partial | Overbook bands and utilization over 100 percent |
| Capacity calendars | Weak | Hardcoded working hours |
| Automatic resource leveling | No | Detection, not scheduling |
| TaskJuggler integration | No | Could be added externally |
| Safe write-back | No | No source systems |
| Audit log | No | Browser state only |
| UI inspiration | Yes | Timeline/resources/KPI layout worth studying |

## Reuse options

### Option A: adopt as RePlan base

Not recommended.

Reasons:

- wrong core model;
- no backend/API/CLI;
- no task-source adapters;
- no security boundary;
- no scenario engine;
- no scheduling engine;
- no tests;
- unclear packaged license file.

Adopting it as the base would likely mean rewriting most of it.

### Option B: fork and extend

Not recommended as first path.

Reasons:

- fork would inherit a UI-first static architecture;
- RePlan needs model-first CLI/API architecture;
- substantial changes would fight the existing shape;
- adding backend/auth/adapters/scenarios would be a rewrite.

This may be useful only if RePlan later wants a simple static demo UI and the
license is clarified.

### Option C: use ideas, not code

Recommended.

Ideas worth borrowing:

- resource timeline grouped by resource;
- allocation percentage;
- overbook background bands;
- baseline shadow;
- JSON import/export;
- people/equipment/rooms/vehicles as resources;
- simple static single-user demo mode;
- low-friction browser experience.

### Option D: use as comparison fixture

Recommended.

Create a RePlan import experiment:

```text
Quollnet JSON -> RePlan canonical model
```

This would test whether RePlan's model can ingest lightweight resource plans
from simple external tools.

## Potential import mapping

Quollnet resource:

```json
{
  "id": "heavy-eq-1",
  "name": "Excavator XL-3000",
  "class": "construction equipment - heavy",
  "description": "Large hydraulic excavator",
  "cost_per_hour": 250
}
```

RePlan resource:

```yaml
id: quollnet:heavy-eq-1
display_name: Excavator XL-3000
kind: equipment
role: construction equipment - heavy
skills: []
cost_rate: 250
metadata:
  source: quollnet
  original_class: construction equipment - heavy
  description: Large hydraulic excavator
```

Quollnet allocation:

```json
{
  "id": "alloc-5",
  "resource_id": "heavy-eq-1",
  "task": "Foundation Excavation",
  "start": "2025-08-15T08:00:00Z",
  "end": "2025-09-15T16:00:00Z",
  "allocation_pct": 100,
  "baseline_start": "2025-08-15T08:00:00Z",
  "baseline_end": "2025-09-15T16:00:00Z",
  "cost": 80000
}
```

RePlan task planning record:

```yaml
canonical_id: quollnet:alloc-5
source: quollnet
source_id: alloc-5
title: Foundation Excavation
assignee_resource_id: quollnet:heavy-eq-1
start_date: 2025-08-15
finish_date: 2025-09-15
allocation_pct: 100
baseline:
  start_date: 2025-08-15
  finish_date: 2025-09-15
  cost: 80000
metadata:
  source_resource_id: heavy-eq-1
```

This mapping is possible, but it is lossy because Quollnet does not have real
task IDs, dependencies or status.

## What Quollnet changes in RePlan plan

Quollnet does not remove the need for RePlan.

It does suggest two adjustments:

### 1. Keep a static-demo path

RePlan could eventually have:

```text
replan report static-ui --snapshot snapshot.json --output public/
```

This would generate a browser-only interactive view of resources and
allocations.

Quollnet proves this style is useful.

### 2. Treat non-human resources as first-class earlier

Quollnet's construction-oriented sample includes equipment, rooms and vehicles.

This validates the RePlan decision to separate actor identity from schedulable
resource and to support non-human resources from the schema stage.

## Verdict

Quollnet Resource Planner is not a ready base for RePlan.

It is a small MIT-stated static resource scheduler with useful UI ideas.

It is closer to:

```text
single-user visual resource allocation board
```

than to:

```text
source-agnostic resource/capacity planning infrastructure for software teams
and AI agents
```

RePlan should:

- not fork it now;
- not build on it as core;
- study its timeline/resource UI;
- consider importing its JSON format later;
- possibly reuse ideas after license clarification;
- keep TaskJuggler as the serious scheduling engine reference.

## Follow-up tasks

- Check whether upstream adds a `LICENSE` file.
- Inspect Quollnet live UI manually before designing RePlan's first static UI.
- Consider `quollnet-json` as a low-priority import adapter after YAML fixtures
  and OpenProject import.
- Re-check repository activity if RePlan reaches UI implementation phase.
