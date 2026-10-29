# OpenProject work-package RePlan panel design

Status: superseded by [OpenProject RePlan plugin redesign](openproject-plugin-redesign-design.md). Index: [OpenProject design index](openproject-design-index.md).

Original status: accepted first implementation design.

Date: 2026-08-11.

Accepted: 2026-08-15.

Related task:
`task.self.design-openproject-work-package-replan-panel`.

## Purpose

The project-level RePlan plugin page proves that RePlan can be shown inside
OpenProject without moving RePlan planning logic into OpenProject.

The next human workflow problem is more specific: when the operator opens one
OpenProject work package, the resource context for that exact work package is
still not visible in the same place.

The work-package-level panel should answer:

- is this OpenProject work package linked to a RePlan task?
- which RePlan task is it?
- what resources are planned for this task?
- how much planned, actual and remaining demand exists per resource?
- are there warnings, open proposals or missing evidence for this task?
- where can the operator open the fuller RePlan dashboard or Gantt-like view?

The panel is not a replacement for OpenProject work-package details. It is a
read-only RePlan resource context surface attached to the OpenProject task
workflow.

## Product Decision

Build the work-package panel as the next OpenProject-native surface after the
project-level RePlan tab.

Keep the first version read-only.

The panel should consume the same RePlan overlay artifact family as the
project-level plugin page:

```text
reports/latest/raw/openproject_overlay.json
```

If the current overlay contract lacks fields needed by the panel, extend the
artifact first in RePlan reports and keep the plugin as a renderer. Do not make
the plugin compute RePlan planning state.

## Placement Levels

There are three realistic placement levels.

### Placement A: Work-Package Linked Page

Add a plugin route that can be opened for one work package, for example:

```text
/projects/<project_identifier>/replan_overlay/work_packages/<work_package_id>
```

The project-level RePlan page can link every row to this route. The route
renders one work-package detail panel using the overlay row for that work
package.

Pros:

- easiest first prototype;
- no OpenProject frontend internals;
- no injection into native work-package UI;
- reuses the current plugin architecture;
- good AI/debug path because each route is deterministic.

Cons:

- the operator must click from the OpenProject work package to the RePlan panel
  or from the project-level RePlan page to the per-work-package panel;
- it is not visually embedded inside the native OpenProject work-package detail
  page.

### Placement B: Work-Package Detail Link Or Tab

Add a visible RePlan link, tab or action near the OpenProject work-package
detail view. The target may still be the plugin route from Placement A.

Pros:

- starts from the real OpenProject work-package screen;
- keeps the operator closer to the task context;
- may avoid deep frontend injection if OpenProject exposes a stable extension
  point.

Cons:

- requires OpenProject extension-point research;
- may require frontend or Rails view integration depending on how the current
  OpenProject version renders work-package details;
- more version-sensitive than Placement A.

### Placement C: Embedded Work-Package Panel

Render the RePlan content directly inside the native OpenProject work-package
detail screen.

Pros:

- best human workflow if stable;
- the operator sees OpenProject task fields and RePlan resource state in one
  place;
- less context switching.

Cons:

- highest coupling to OpenProject internals in this slice;
- may require frontend asset work;
- may break on OpenProject upgrades;
- should not be the first implementation unless Placement B reveals a stable
  extension point.

Accepted first implementation target:

```text
Placement A first, then evaluate Placement B, then only later Placement C.
```

This keeps the next slice useful while still preserving the path toward a
proper embedded OpenProject task surface.

## Panel Content

The first panel should show a compact, task-focused view.

Header:

- OpenProject work-package id;
- work-package title;
- RePlan task id;
- RePlan tracking status;
- planned start and finish;
- generated-at timestamp for the overlay artifact;
- link back to the project-level RePlan page;
- links to generated RePlan dashboard artifacts when available.

Resource section:

- one row per resource demand;
- resource id;
- resource class;
- role;
- unit;
- planned amount;
- actual amount;
- remaining amount;
- intensity percent when available;
- flags and warning codes.

Tracking section:

- evidence count;
- latest evidence summary when available;
- open proposal marker;
- proposal ids or links when the overlay contract supports them;
- warning codes;
- open dependency count.

Timeline section:

- compact task window bar;
- resource demand bars using the same visual semantics as the project-level
  resource view;
- explicit indication when a resource demand inherits the whole task window;
- explicit indication when a demand has its own participation window.

Empty and degraded states:

- if the work package has no RePlan row, render `No RePlan overlay for this
  work package`;
- if the overlay file is missing, render the same safe empty state as the
  project-level plugin page;
- if a field is absent, omit that field or mark it as `not available`; do not
  fail the page.

## Overlay Contract Needs

The current `openproject_overlay.json` already contains enough for a first
Placement A prototype:

- work-package id;
- RePlan task id;
- title;
- tracking status;
- planned start and finish;
- resource demands;
- warning codes;
- proposal marker;
- evidence count;
- open dependency count.

Useful contract extensions for a better panel:

- stable per-work-package panel URL;
- dashboard artifact links;
- Gantt artifact links;
- latest evidence summary per task;
- proposal ids per task;
- proposal severity or priority per task;
- resource capacity waterline context for the task window;
- explicit `participation_window_source` per demand;
- direct OpenProject work-package URL as seen by the operator.

These fields should be added by RePlan report generation before the plugin
depends on them.

## Safety And Ownership

OpenProject owns:

- work-package identity;
- title;
- status;
- dates;
- assignee;
- relations;
- native comments and collaboration.

RePlan owns:

- resource demands;
- capacities;
- actuals accepted into RePlan;
- evidence;
- warnings;
- proposals;
- generated overlay artifacts;
- audit records.

The panel must not:

- write OpenProject;
- write RePlan canonical files;
- accept proposals;
- record actuals;
- infer progress automatically;
- expose private OpenProject token or VPS details in browser output;
- make RePlan unusable without OpenProject.

Mutation remains CLI-first and guarded:

- RePlan proposal acceptance stays in RePlan commands;
- OpenProject status/date/comment changes stay in the reviewed helper layer;
- every OpenProject write remains dry-run by default with explicit apply gates.

## Implementation Plan

First implementation slice:

1. Add a server-side route for one work-package overlay row.
2. Add presenter methods for finding a work package row by id.
3. Add a per-work-package ERB view using the same visual language as the
   project-level RePlan plugin view.
4. Add links from the project-level table/timeline rows to the panel route.
5. Extend scaffold checks to verify the route, presenter lookup and view
   syntax.
6. Regenerate and republish the live overlay artifact.
7. Verify the live OpenProject route while keeping the panel read-only.

Deferred implementation slices:

- OpenProject work-package detail link or tab discovery;
- embedded panel proof of concept if a stable extension point exists;
- controlled Gantt-like RePlan view inside the plugin;
- native OpenProject Gantt overlay research.

## Acceptance Criteria

This design is accepted when:

- the operator agrees that Placement A is the correct first prototype;
- the panel remains read-only;
- the plugin remains an overlay renderer, not a RePlan planning engine;
- missing overlay data has safe degraded behavior;
- native OpenProject Gantt and embedded work-package injection stay deferred;
- the next implementation task is explicit and small enough to execute without
  refactoring OpenProject internals.

## Open Questions

Resolved for the first implementation:

- the first per-work-package panel should be reachable from the project-level
  RePlan plugin page;
- immediate native OpenProject work-package action/link research is deferred;
- the first useful panel must include resource rows plus tracking, proposal and
  evidence summaries, because resource planning without execution context is
  not enough for the dogfood loop;
- the first panel should include compact timeline bars using the same visual
  semantics as the project-level resource view;
- the first implementation must stay read-only and must not mutate
  OpenProject, RePlan canonical data, proposals or actuals.

Next implementation task:

```text
task.self.implement-openproject-work-package-replan-panel
```

OpenProject sync: pending for the private live instance if live access is not
available during backlog creation. Public sanitized fixture data may still use
a sanitized work-package reference so local smoke tests remain deterministic.

## Implementation Status

Implementation status, 2026-08-15:

- the first read-only route, controller action, presenter lookup and ERB view
  are implemented locally;
- project-level rows link to the per-work-package panel;
- the plugin scaffold check verifies the new route/action/presenter/view/link
  and runs ERB syntax checks;
- the private live OpenProject plugin image was rebuilt and deployed on the
  exact OpenProject 17.6.0 image pair;
- the live mounted overlay artifact was refreshed from the RePlan self-dogfood
  project;
- live Rails verification confirms the project-level RePlan route, the
  work-package-level RePlan route, overlay loading and presenter lookup for a
  work package with resource demands.

The implementation task is accepted in RePlan. The live OpenProject work
package status update remains `openproject_sync_pending` because the
OpenProject API helper timed out after live route verification; the task should
be synchronized through the guarded helper when API access is stable again.

Operational note: OpenProject Rails boot after image replacement can exceed the
normal Docker health and autoheal window. During this deployment the first
attempt was rolled back, then retried with autoheal temporarily stopped until
the web service answered the normal health endpoint. Autoheal was restored
after the service became healthy.
