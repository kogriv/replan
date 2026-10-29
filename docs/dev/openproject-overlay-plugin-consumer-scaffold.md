# OpenProject RePlan overlay plugin consumer scaffold

Status: superseded by [OpenProject RePlan plugin redesign](openproject-plugin-redesign-design.md). Index: [OpenProject design index](openproject-design-index.md).

Original status: implementation scaffold.

Date: 2026-08-07.

Related task: `task.self.scaffold-openproject-overlay-plugin-consumer`.

## Purpose

This document defines the first minimal OpenProject plugin consumer for RePlan.

The goal is not to build the final OpenProject integration. The goal is to
prove the smallest useful OpenProject-side renderer over the already generated
RePlan overlay artifact:

```text
reports/latest/raw/openproject_overlay.json
```

The plugin must make RePlan visible from the OpenProject-centered human
workflow without turning RePlan core into an OpenProject plugin and without
duplicating RePlan planning logic.

## Current decision

Build a read-only OpenProject plugin scaffold as a Ruby gem/Rails engine.

The official OpenProject plugin documentation says OpenProject plugins are Ruby
gems, can be included through `Gemfile.plugins`, and the generated plugin is a
Rails engine. It also documents production installation as a deployment step
for the OpenProject runtime.

Implication for RePlan:

- the public RePlan repository may contain a plugin scaffold;
- the running private OpenProject instance must not be modified until the
  scaffold is reviewed;
- production installation, container changes and runtime restart belong to a
  separate deployment task;
- the first plugin should not require a JavaScript frontend build.

## Scope

The first scaffold owns:

- a plugin gem skeleton under `integrations/openproject/plugins/`;
- a project-level OpenProject menu entry;
- a read-only project page;
- a server-side loader for a configured `openproject_overlay.json` file;
- tolerant rendering for missing or invalid overlay data;
- simple resource, warning, dependency and tracking summaries per linked work
  package;
- documentation for how to wire the plugin into OpenProject later.

The first scaffold does not own:

- RePlan canonical data;
- resource calculations;
- planning algorithms;
- actual acceptance;
- proposals;
- OpenProject writes;
- OpenProject Gantt modification;
- work-package detail panel injection;
- browser-side token handling;
- custom authentication;
- role-based access control beyond OpenProject project visibility.

## First placement

Use a project-level RePlan page first.

Reason:

- it avoids coupling to OpenProject Gantt internals;
- it avoids work-package detail-view extension points until the plugin is
  proven;
- it still lets the operator stay inside OpenProject;
- it can render the same rows as the static dashboard and future side panel.

Work-package-level panels and Gantt overlays are later placement tasks.

## Data loading

The plugin reads a local file path configured by environment variable:

```text
REPLAN_OPENPROJECT_OVERLAY_PATH=/path/to/reports/latest/raw/openproject_overlay.json
```

Fallback for development:

```text
public/replan/openproject_overlay.json
```

The plugin must not embed RePlan API tokens in frontend code.

The first loader returns a structured empty state when:

- the file path is missing;
- the file does not exist;
- JSON is invalid;
- `work_packages` is absent or not an array.

This makes the plugin safe to open even before report generation is wired into
deployment.

## Rendering contract

The project page should show:

- RePlan project name;
- generated timestamp;
- overlay week and status date;
- linked work-package count;
- missing-link count;
- one row per linked OpenProject work package;
- work package id;
- RePlan task id;
- title;
- tracking status;
- planned start and finish;
- resource ids;
- resource demand count;
- warning count;
- open dependency count;
- open proposal marker;
- evidence count.

The page may link to generated `index.html` and `gantt.html` artifacts when
the overlay artifact contains those relative references.

## Safety rules

The scaffold must remain read-only.

Allowed:

- read configured overlay JSON;
- render server-side HTML in OpenProject;
- link to static RePlan artifacts.

Forbidden in this slice:

- POST, PATCH, DELETE to OpenProject;
- mutation of RePlan files;
- writing overlay data into OpenProject database;
- background sync jobs;
- browser-visible private tokens;
- live deployment to a private OpenProject instance.

## Verification

The current repository can verify the scaffold without a full OpenProject
runtime by checking:

- Ruby syntax for plugin Ruby files when Ruby is installed;
- gemspec syntax;
- presence of expected plugin files;
- RePlan validation;
- MVP smoke;
- privacy scan for private OpenProject endpoints and tokens.

Full runtime verification inside OpenProject is a later deployment task because
it requires changing the OpenProject runtime and restarting the instance.

## Next tasks

After this scaffold is accepted:

1. design the safe deployment path for an OpenProject runtime;
2. test the plugin in a throwaway OpenProject runtime using the same major
   OpenProject image class;
3. install the plugin into the private dogfood OpenProject instance only after
   the throwaway runtime passes;
4. point it at a generated RePlan overlay artifact through a server-side file
   path;
5. inspect the project-level page from OpenProject;
6. only then evaluate work-package panels or Gantt overlays.

The deployment-path design is documented in
[`openproject-plugin-deployment-path.md`](openproject-plugin-deployment-path.md).
