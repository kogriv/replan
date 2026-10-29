# OpenProject read-only RePlan plugin feasibility

Status: current. Index: [OpenProject design index](openproject-design-index.md).

Original status: accepted design decision.

Decision date: 2026-08-06.

Related task: `task.self.design-openproject-read-only-plugin-feasibility`.

## Purpose

The Level 0 and Level 1 OpenProject-native RePlan surface is already accepted
and implemented as shared identity, OpenProject links, adjacent generated
dashboard views and raw JSON artifacts.

This document answers the next question: how should RePlan reach a mandatory
OpenProject plugin path without turning RePlan core into an OpenProject-only
application?

## Current Facts

OpenProject remains the primary human project-management surface. RePlan
remains the resource-planning sidecar.

The live dogfood loop is now structurally aligned:

- RePlan planned tasks map to live OpenProject work packages;
- RePlan dependencies map to live OpenProject relations;
- date, title and status drift are zero after accepting OpenProject-owned date
  changes;
- agents can use the guarded OpenProject helper layer through a protected
  connection without browser automation.

The practical pain is human observability. The adjacent RePlan dashboard is
usable, but the operator still has to mentally compare an OpenProject work
package or Gantt row with a separate RePlan report.

## Official OpenProject Plugin Model

OpenProject plugins are Ruby gems and can be included in `Gemfile.plugins`.
The official development guide describes generation through the OpenProject
Rails generator and notes that a generated plugin is a Rails engine that can be
published as a gem.

Relevant official references checked on 2026-08-06:

- OpenProject plugin development guide:
  https://www.openproject.org/docs/development/create-openproject-plugin/
- OpenProject system administration plugin guide:
  https://www.openproject.org/docs/system-admin-guide/plugins/
- OpenProject development documentation:
  https://www.openproject.org/docs/development/
- OpenProject release notes:
  https://www.openproject.org/docs/release-notes/

Important implications:

- a real plugin is deploy-time coupling to OpenProject;
- production deployment requires adding the plugin to the OpenProject runtime,
  not just dropping static files next to RePlan;
- plugin code can add routes, views, models or override existing behavior, so
  the surface area is powerful but easy to overuse;
- plugin release work has normal gem/review/licensing/versioning overhead;
- OpenProject continues to move quickly, so plugin compatibility must be
  treated as version-sensitive.

## Decision

RePlan must get an OpenProject plugin for the OpenProject-centered human
workflow.

Do not jump directly to a full OpenProject plugin implementation.

The mandatory plugin path starts with a minimal read-only plugin, but only as
an integration shell over an already-stable RePlan overlay contract.

The plugin must not own:

- RePlan canonical data;
- resource scheduling;
- scenario logic;
- actuals;
- proposals;
- OpenProject write-back policy;
- authentication for RePlan core;
- broad bidirectional sync.

The plugin may own:

- project-level navigation entry;
- work-package-level read-only side panel or tab;
- rendering of RePlan overlay JSON;
- links back to generated RePlan reports;
- display of resource pressure, warnings and proposal counts.

## Minimum Useful Plugin

The minimum plugin worth building is a read-only OpenProject-side renderer.

This is not optional product polish. For a human operator using OpenProject as
the main task and Gantt surface, RePlan resource signals need to appear in
OpenProject instead of forcing constant comparison between separate interfaces.

It should answer, for the current OpenProject project or work package:

- is this work package linked to a RePlan task?
- what resources are planned for it?
- is any resource overloaded in the task window?
- how much planned, actual and remaining resource demand exists?
- are there warnings or unresolved proposals?
- where is the full RePlan dashboard or raw JSON?

It should not edit OpenProject or RePlan in the first version.

## Candidate Placement

Preferred first placement:

- project-level RePlan tab or menu item that shows project summary and links to
  per-task RePlan rows;
- optional work-package detail side panel only after project-level tab proves
  useful.

Do not start with a Gantt overlay plugin.

Reason: Gantt overlay is the most attractive human view, but it is also likely
to couple most tightly to OpenProject frontend internals. A project tab or
read-only work-package panel can prove the value with less UI risk.

Native OpenProject Gantt modification remains a real option, not a rejected
direction. It is staged later because it probably means working against
OpenProject frontend internals, frontend asset builds and version-specific
behavior. Before taking that cost, RePlan should first prove the resource
overlay contract through simpler plugin surfaces:

1. project-level RePlan view;
2. work-package-level RePlan panel;
3. controlled RePlan Gantt-like view inside the plugin;
4. only then a true native OpenProject Gantt overlay or patch.

## Data Contract

The plugin should consume a static or served RePlan overlay artifact, not query
RePlan internals directly.

Implemented artifact:

```text
reports/latest/raw/openproject_overlay.json
```

The first contract contains:

- project id;
- generated timestamp;
- OpenProject project identifier;
- OpenProject-linked work-package rows;
- RePlan task id;
- title;
- tracking status;
- planned start and planned finish;
- resource ids;
- resource classes;
- planned, actual and remaining amounts;
- participation windows when known;
- load intensity values;
- warning codes;
- open proposal marker;
- evidence count;
- dashboard URL or relative artifact path.

The plugin must tolerate missing rows. Missing RePlan data should render as
`No RePlan overlay for this work package`, not as an error.

## Deployment Shape

For dogfood, there are three possible deployment shapes.

### Shape A: External Static Artifact

RePlan generates static JSON and HTML. OpenProject plugin reads a configured
URL or mounted file path.

Pros:

- keeps RePlan headless;
- minimal runtime coupling;
- works with CLI-first flow;
- easiest OSS story.

Cons:

- freshness depends on report generation;
- plugin needs configuration;
- access control around static artifacts must be considered.

### Shape B: Local RePlan HTTP Sidecar

RePlan runs a small local service and the plugin reads it.

Pros:

- fresher data;
- better later path to interactive status.

Cons:

- introduces daemon/service operation earlier;
- raises auth and availability questions;
- less aligned with the current lightweight CLI-first design.

### Shape C: Plugin-owned Storage

The plugin stores imported RePlan overlay data inside OpenProject.

Pros:

- native OpenProject deployment and permissions;
- simple reads from plugin UI.

Cons:

- risks turning the plugin into a second RePlan datastore;
- creates migration/versioning work;
- weakens PM-agnostic RePlan core.

Accepted first direction: Shape A.

## Authentication And Authorization

The first plugin should rely on OpenProject session authentication for seeing
the plugin page. It should not introduce a separate user database.

For artifact access:

- public OSS examples can use local files or generated static artifacts;
- private/live deployments should serve artifacts only behind the same private
  boundary as OpenProject or through an operator-approved internal path;
- tokens must not be embedded in the plugin frontend;
- if the plugin fetches an external artifact over HTTP, the first dogfood
  version should use a server-side fetch or static mount rather than exposing a
  RePlan API token to the browser.

RBAC is read-only in the first version:

- if a user can view the OpenProject project, they can view the RePlan overlay
  summary for that project;
- finer-grained resource secrecy is out of scope for the first plugin and must
  be handled before using the plugin in a multi-client or restricted-resource
  environment.

## Why This Is Not Wasted Work

This design does not replace the static dashboard. It clarifies what the
dashboard must export so the OpenProject plugin can exist without duplicating
RePlan logic.

The near-term implementation path is therefore:

1. stabilize `openproject_overlay.json` as the shared read-only contract;
2. keep rendering the same contract in the generated dashboard;
3. scaffold a read-only OpenProject plugin that consumes the same contract;
4. iterate toward richer OpenProject placement only after the first plugin view
   proves the contract and deployment shape.

The dashboard remains valuable as the PM-agnostic reference renderer and OSS
artifact. The OpenProject plugin is the OpenProject-specific renderer needed
for a usable human workflow in the current dogfood stack.

## Acceptance Criteria

This design is accepted when:

- the plugin is explicitly scoped as read-only;
- Shape A is accepted as the first deployment shape;
- the plugin consumes RePlan overlay artifacts instead of owning RePlan data;
- Gantt overlay is staged behind project-level tab or work-package panel;
- auth assumptions are explicit;
- the overlay JSON contract exists before the full plugin implementation starts.

## Next Step

The overlay contract implementation task is:

```text
task.self.define-openproject-overlay-json-contract
```

That task adds `reports/latest/raw/openproject_overlay.json` and keeps it useful
for both the generated dashboard and the planned read-only OpenProject plugin.
The next implementation step after that task is to design and scaffold the
plugin consumer against this artifact instead of letting the plugin query or
recompute RePlan internals.
