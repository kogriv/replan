# OpenProject RePlan Plugin Live Installation Result

Status: dated record (2026-08-07). Index: [OpenProject design index](openproject-design-index.md).

Original status: completed live installation.

Date: 2026-08-07.

Related task: `task.self.execute-live-openproject-plugin-installation`.

## Purpose

This document records the first private live installation of the read-only
RePlan overlay plugin into the dogfood OpenProject instance.

The installation was intentionally guarded:

- a backup was created before changing Rails services;
- the generated RePlan overlay artifact was published separately from the
  RePlan source repository;
- the compose override changed only OpenProject Rails services;
- a failed first image attempt was rolled back before continuing;
- the final installation was verified through OpenProject health checks and
  Rails-level plugin checks.

## Public Boundary

This document is public-safe.

It does not record:

- live token values;
- private hostnames;
- private file paths;
- private work-package ids;
- private credentials;
- private pilot narrative.

Concrete operational paths and backup locations belong in the private
infrastructure repository.

## Backup

Before live apply, a private backup directory was created on the OpenProject
host.

It contains:

- PostgreSQL `pg_dumpall` output compressed with gzip;
- OpenProject uploaded assets archive;
- current compose image snapshot;
- current compose service status snapshot.

The first backup attempt produced an invalid database dump because the command
implicitly tried to connect as the container root user. This was caught before
the plugin apply. The accepted backup uses the explicit PostgreSQL role that
can read the OpenProject database.

## Overlay Artifact

The RePlan overlay artifact was regenerated from the current self-dogfood
project before installation.

Verified overlay summary:

```text
work_package_count: 47
linked_task_count: 47
missing_link_count: 0
```

Only generated report artifacts were copied into the private OpenProject
runtime mount. The full RePlan repository, private bindings, dotenv files,
tokens and Git metadata were not mounted into OpenProject.

## First Apply Attempt And Rollback

The first custom image attempt used a floating OpenProject builder tag while
the live runtime was on an exact OpenProject 17.6.0 slim image.

Result:

- Rails services were recreated on the custom image;
- OpenProject web health became unhealthy;
- logs showed a Bundler production error caused by `Gemfile` and
  `Gemfile.lock` mismatch;
- the mismatch came from using a newer floating builder image with an older
  live final image.

Rollback:

- the compose override was disabled;
- Rails services were recreated on the stock OpenProject image;
- OpenProject web returned to healthy;
- the ordinary OpenProject health check passed.

No database restore was needed because the plugin is read-only and the failed
attempt did not introduce migrations.

## Fix

The live custom image must use the exact OpenProject version pair that matches
the running instance:

```text
openproject/openproject:17.6.0
openproject/openproject:17.6.0-slim
```

The repeatable throwaway runtime script now defaults to those exact tags for
this dogfood deployment class.

The custom image also keeps the earlier preflight fix:

```text
copy /app/modules from the builder stage into the final slim image
```

Without this, the copied `Gemfile.lock` may reference OpenProject path-gem
modules that are not present in the final image.

## Final Apply

The accepted live image tag shape is:

```text
replan/openproject-replan-overlay:17.6.0-<replan-revision>
```

The compose override applies that image to:

- `web`;
- `worker`;
- `cron`;
- `seeder`.

Supporting services remain stock:

- database;
- cache;
- proxy;
- collaborative editing service;
- auto-heal or equivalent watchdog service.

After applying the exact-version image:

- OpenProject web became healthy;
- the standard OpenProject health check passed;
- `web`, `worker` and `cron` were running the custom image;
- the generated overlay artifact was mounted read-only;
- the plugin route was present.

## Verification

Rails verification confirmed:

```json
{
  "ok": true,
  "work_packages": 47,
  "routes": ["project_replan_overlay"],
  "plugin": "openproject_replan_overlay",
  "permission": "view_replan_overlay",
  "menu": "replan_overlay"
}
```

HTTP route verification with the correct OpenProject host/protocol headers
returned a redirect to login for:

```text
/projects/replan-self/replan_overlay
```

That confirms the route exists in the live OpenProject web process and is
protected by normal OpenProject authentication instead of returning a missing
route.

## Result

The first private live installation is accepted.

The task:

```text
task.self.execute-live-openproject-plugin-installation
```

is now complete.

The release-story dependency:

```text
dependency.live-plugin-installation-before-release-story
```

is resolved. Other release-story gates may still remain open.

## Follow-Up

Do not treat this as a full OpenProject plugin product yet.

The next useful product steps are:

- browser review of the plugin page after normal OpenProject login;
- decide whether the page is sufficient for Level 3 OpenProject-native
  visibility;
- improve the plugin page only from observed operator friction;
- define artifact refresh procedure or automation;
- keep the plugin read-only until a separate write-path design is accepted.

## First Browser Review Follow-Up

After normal login, the RePlan project-menu item appeared in the live
OpenProject project and rendered the first read-only table view.

That proves the live plugin route, project menu, permission, module activation
and overlay loading path. It does not prove that the view is useful enough for
day-to-day human resource planning.

The accepted next backlog item is:

```text
task.self.improve-live-openproject-plugin-resource-view
```

This task should improve the project-level plugin page from a plain table into
the smallest useful resource-aware view. It should stay read-only and consume
the RePlan overlay contract instead of moving RePlan planning logic into
OpenProject.

The task should decide, from the live operator review, which signals belong in
the first project-level plugin view:

- task-linked resource demand summary;
- planned, actual and remaining resource effort;
- warning and proposal markers;
- resource-load or timeline hints that can fit safely in a project tab;
- links to the fuller generated RePlan dashboard and Gantt artifacts.

The task should also explicitly defer anything that needs deeper OpenProject
frontend coupling, such as modifying the native OpenProject Gantt view, until a
separate plugin surface decision is made.

## Resource View Update

Original status: accepted as sufficient for the next panel-design step.

Date: 2026-08-07.

The first follow-up implementation updated the read-only project-level plugin
page from a plain table into a resource-aware project view.

Implemented behavior:

- summary metrics for linked tasks, active tasks, resources, resource demands,
  warnings and open proposals;
- resource aggregate blocks grouped by resource and unit;
- current-attention rows for active tasks, warnings, proposals and open
  dependencies;
- compact task/resource timeline rows driven by the existing overlay contract;
- resource demand intensity bars using vertical fill height, preserving the
  already accepted RePlan load-profile semantics;
- tolerant rendering over the same mounted `openproject_overlay.json`
  artifact, without browser-visible tokens and without OpenProject writes.

Deployment result:

- a new exact-version OpenProject 17.6.0 custom image was built for the private
  dogfood instance;
- OpenProject Rails services were recreated on that image;
- the web service returned healthy;
- Rails verification loaded the overlay and presenter successfully;
- the mounted overlay artifact was refreshed to 48 linked work packages and 0
  missing links.

The task is accepted as sufficient to unblock the next dogfood step. This does
not mean the project-level view is final. It means the first resource-aware
plugin surface is good enough for OpenProject-side comparison and for learning
what belongs in the work-package-level RePlan panel.

Follow-up surface decision:

- native OpenProject Gantt modification is technically possible, but it is a
  later and higher-risk integration because it likely couples RePlan to
  OpenProject frontend internals and version-specific asset behavior;
- the next OpenProject-native UI sequence should be project-level plugin view,
  then work-package-level RePlan panel, then a controlled Gantt-like RePlan
  view inside the plugin;
- a true native OpenProject Gantt overlay should be evaluated only after those
  surfaces prove that the extra coupling is worth the maintenance cost;
- the RePlan overlay contract should still preserve timeline,
  participation-window, capacity and warning fields so a future native Gantt
  overlay remains possible.

The next work-package-level panel design task was then created and synced into
the live OpenProject/RePlan loop. The mounted overlay artifact was refreshed
again and the live plugin can now see 49 linked work packages with 0 missing
links.

## Work-Package Panel Update 2026-08-15

Original status: installed and healthy.

The read-only RePlan overlay plugin was rebuilt and redeployed with the first
work-package-level RePlan panel route.

The update added:

- a live project-level route that still renders the resource-aware RePlan
  project view;
- a live work-package-level route for one OpenProject work package;
- presenter lookup by OpenProject work-package id;
- resource-demand rows, planned/actual/remaining amounts, tracking evidence,
  proposal and dependency summaries, and compact timeline bars for that work
  package.

The mounted overlay artifact was refreshed from the current RePlan self-dogfood
project and the live Rails process verified:

- OpenProject web health returned healthy on the updated image;
- the RePlan plugin exposes both the project-level and work-package-level
  routes;
- the overlay loader reads 51 work packages;
- the work-package presenter can load a row with resource demands.

Operational finding:

- OpenProject Rails boot after replacing the image can exceed the Docker
  health and autoheal window. The first attempt was rolled back to the previous
  image, then the update was retried with autoheal temporarily stopped until
  the web service answered the health endpoint. Autoheal was restored after the
  service became healthy.

Remaining synchronization note:

- RePlan marks the implementation task accepted, but the live OpenProject task
  status update remains pending because the guarded OpenProject API helper
  timed out after route verification. No direct database or unreviewed
  OpenProject write was used.
