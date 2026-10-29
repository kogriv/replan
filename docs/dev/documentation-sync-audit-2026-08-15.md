# RePlan documentation synchronization audit

Status: completed documentation synchronization pass.

Date: 2026-08-15.

Related task: `task.self.audit-and-sync-project-documentation`.

OpenProject sync: pending.

Reason: live OpenProject drift check failed with `openproject_network_error`
during this documentation pass. The task is recorded in RePlan canonical data
and in the public sanitized fixture, but live OpenProject should be
synchronized when the operator loop next uses the live instance.

## Purpose

This audit checks whether the main project documents still describe the current
RePlan/OpenProject dogfood state after the recent plugin and work-package panel
work.

The problem is not that the documentation is unusable. The problem is that the
project moved through several fast stages:

- generated static reports;
- OpenProject Level 0 and Level 1 links and adjacent dashboard;
- read-only plugin feasibility decision;
- plugin scaffold;
- throwaway runtime proof;
- private live plugin installation;
- project-level resource-aware plugin view;
- work-package-level panel design.

Older documents still contained correct historical decisions, but some of their
"next step" wording made the plugin sound like a future optional path rather
than the already-installed current OpenProject-native surface.

## Current Authoritative State

The current product and dogfood state is:

- OpenProject is the primary human task, date, relation, status and Gantt
  surface.
- RePlan is the resource planning, actuals, evidence, warning, proposal,
  scenario and audit sidecar.
- The OpenProject/RePlan backlog rule is active: non-trivial agreed work must
  be represented in the OpenProject/RePlan dogfood loop.
- The public repository must not contain live tokens, private OpenProject
  endpoints, private work-package ids, VPS details or private pilot narrative.
- The read-only OpenProject plugin path is mandatory for the
  OpenProject-centered human workflow.
- The plugin is installed in the private dogfood OpenProject runtime.
- The project-level plugin view now renders a resource-aware RePlan view over
  `reports/latest/raw/openproject_overlay.json`.
- The next OpenProject-native surface is a read-only work-package-level RePlan
  panel.
- Modifying native OpenProject Gantt remains technically possible, but it is a
  later high-coupling path after simpler plugin surfaces prove the required
  resource signals.

## Documents Reviewed

Reviewed during this pass:

- `README.md`;
- `docs/README.md`;
- `docs/dev/backlog-operating-convention.md`;
- `docs/dev/openproject-native-replan-surface-design.md`;
- `docs/dev/openproject-read-only-plugin-feasibility.md`;
- `docs/dev/openproject-plugin-live-installation-result.md`;
- `docs/dev/openproject-work-package-replan-panel-design.md`;
- `docs/spec/openproject-overlay-strategy.md`;
- `docs/spec/static-observability-dashboard.md`;
- `docs/roadmap/self-dogfood-planning-calibration.md`;
- `examples/replan-self/canonical/tasks.yaml`;
- `examples/replan-self/canonical/current_plan.yaml`;
- `examples/replan-self/canonical/resource_demands.yaml`;
- `examples/replan-self/canonical/dependencies.yaml`.

## Findings

### Finding 1. Plugin Status Wording Drift

Some documents still described the plugin path as future or optional because
they were written before the live installation.

Correction:

- keep those historical documents intact where they describe an accepted
  decision at that time;
- add explicit current-state updates where a reader would otherwise choose the
  wrong next step;
- make clear that the project-level plugin view exists now, while the
  work-package panel and controlled plugin Gantt-like view are still future
  slices.

### Finding 2. Static Dashboard Documentation Needs Current Boundary

The static dashboard remains important for OSS demos, raw artifacts and AI CLI
agent inspection.

However, it is no longer the only human observability layer. It should now be
described as the PM-agnostic reference renderer and artifact producer that feeds
the OpenProject plugin, not as the current final human workflow.

Correction:

- keep static dashboard semantics as authoritative for report artifacts;
- state that OpenProject plugin surfaces consume the same artifacts;
- state that the project-level live plugin view is already implemented.

### Finding 3. Roadmap Step Number Drift

The roadmap accumulated entries out of chronological order. After Step 33, the
document contained new sections numbered Step 28 and Step 29.

Correction:

- renumber the late plugin resource-view and work-package panel design entries
  so the roadmap chronology remains readable.

### Finding 4. Work-Package Panel Design Is The Current Review Gate

The active design document correctly says the work-package panel is a draft for
operator review.

The related roadmap and index documents should make the same state clear:

- the project-level plugin resource view is implemented and deployed for
  operator review;
- the work-package panel design exists and is the current decision point;
- implementation should not begin until the first placement/content decisions
  are accepted or explicitly narrowed.

### Finding 5. Backlog Needs A Documentation Synchronization Task

This documentation pass is non-trivial agreed work. Per the backlog operating
convention, it must not live only in chat.

Correction:

- add `task.self.audit-and-sync-project-documentation`;
- add resource demands for maintainer, Codex and reviewer role;
- update the public sanitized OpenProject fixture and example binding so local
  smoke tests remain deterministic;
- mark live OpenProject synchronization as pending until live OP is explicitly
  reached again;
- record the task as completed in RePlan canonical state after this pass.

### Finding 6. Resource View Acceptance State Drift

The project-level live plugin resource view had already been implemented and
used as the basis for the work-package panel design, but the RePlan backlog
still left `task.self.improve-live-openproject-plugin-resource-view` and
`dependency.resource-view-before-work-package-panel-design` open.

Correction:

- mark the project-level resource view task accepted for the purpose of
  unblocking the work-package-level panel design;
- resolve the dependency from resource view to panel design;
- record evidence that the view is accepted as sufficient for the next step,
  not as a final UI.

## Updated Direction

The current OpenProject-native UI sequence is:

1. Keep the project-level read-only plugin view as the current OpenProject
   entry point for RePlan resource information.
2. Accept or revise the work-package-level panel design.
3. Implement the first read-only work-package panel route.
4. Link project-level plugin rows to the per-work-package panel.
5. Extend the overlay contract only when the plugin needs data that RePlan
   reports should own.
6. Build a controlled Gantt-like RePlan view inside the plugin after the
   project-level and work-package-level surfaces prove the useful signals.
7. Evaluate true native OpenProject Gantt modification only after those lower
   coupling surfaces are useful.

Backlog synchronization performed in this pass:

- the project-level resource view backlog state was aligned with the fact that
  work-package panel design has already started from that surface;
- the documentation synchronization task itself was added to the RePlan
  current plan with OpenProject sync pending for the private live instance;
- the public sanitized OpenProject fixture and example private-binding file now
  include the documentation synchronization task, so CI and local smoke do not
  depend on live OpenProject availability.

## Non-Goals For This Pass

This audit does not:

- rewrite historical design logs as if earlier decisions never happened;
- remove valid history from the roadmap;
- expose private live OpenProject ids or endpoints;
- create a public release story;
- implement the work-package panel;
- modify native OpenProject Gantt;
- perform broad OpenProject/RePlan bidirectional synchronization.

## Follow-Up

Recommended next project step after this documentation synchronization:

- finish the operator review of
  `docs/dev/openproject-work-package-replan-panel-design.md`;
- decide the first panel implementation scope;
- create the implementation backlog item;
- then implement the read-only Placement A prototype.
