# OpenProject RePlan Plugin Deployment Path

Status: current. Index: [OpenProject design index](openproject-design-index.md).

Original status: accepted deployment design.

Date: 2026-08-07.

Related task: `task.self.design-openproject-plugin-deployment-path`.

## Purpose

This document defines the safe path from the current read-only OpenProject
plugin scaffold to the first runtime installation.

The goal is not to turn RePlan into an OpenProject plugin.

The goal is to install the smallest RePlan overlay renderer into an
OpenProject runtime without breaking the existing OpenProject instance,
leaking private infrastructure details or losing the PM-agnostic RePlan core.

## Current Input

The current plugin scaffold lives at:

```text
integrations/openproject/plugins/openproject-replan_overlay/
```

It is a read-only Ruby gem/Rails engine scaffold that renders:

```text
reports/latest/raw/openproject_overlay.json
```

It does not write to OpenProject. It does not write to RePlan. It does not own
resource calculation, scheduling, scenarios, actuals or proposals.

## Official OpenProject Deployment Facts

Official OpenProject documentation checked on 2026-08-07 says:

- OpenProject plugins are Ruby gems and are included through
  `Gemfile.plugins`;
- generated plugins are Rails engines;
- production plugin use requires adding the plugin to the OpenProject runtime;
- the stock Docker image does not support plugins directly;
- Docker deployments with plugins should use a custom image that copies
  `Gemfile.plugins`, installs gems and, for the slim image, precompiles assets
  in a builder stage before copying the bundle and assets into the final slim
  image.

References:

- https://www.openproject.org/docs/development/create-openproject-plugin/
- https://www.openproject.org/docs/installation-and-operations/installation/docker/#openproject-plugins

Implication:

Do not try to install the RePlan plugin by editing files inside a running
container. The correct runtime path is a custom OpenProject image or a
controlled packaged-install equivalent.

## Observed Runtime Class

The current private dogfood OpenProject instance is in the deployment class:

```text
Docker Compose, OpenProject 17 slim image, multiple Rails processes
```

The relevant process roles are:

- web;
- worker;
- cron;
- seeder.

All Rails processes must use the same plugin-capable image. Installing the
plugin only for the web process is not an accepted deployment shape because it
can create boot, job, migration or code-loading mismatches.

Committed documentation must keep private hostnames, private paths, live ids,
secrets and tokens out of this repository. The exact private compose directory,
domain, Basic Auth details and local storage paths belong in private
infrastructure documentation.

## Accepted Deployment Shape

Use a custom OpenProject image for the plugin runtime.

The custom image should:

1. start from the same OpenProject major version as the running instance;
2. copy the RePlan plugin source into the image;
3. copy a minimal `Gemfile.plugins` into `/app/`, with an explicit
   `require: "open_project/replan_overlay"` entry for the RePlan plugin;
4. run `bundle install` in the builder stage;
5. precompile assets when using an OpenProject slim image;
6. copy the resulting bundle and assets into the final slim image;
7. tag the image with an explicit local tag such as
   `openproject-with-replan-overlay:<openproject-version>-<replan-revision>`;
8. configure every OpenProject Rails service to use the same custom image.

The custom image should not bake a live `openproject_overlay.json` artifact
into the image. The overlay is runtime data and must be mounted or copied into
a runtime data location.

## Overlay Artifact Delivery

The first deployment keeps Shape A from the feasibility design:

```text
RePlan generates a static overlay artifact.
OpenProject plugin reads that artifact from a configured server-side file path.
```

Preferred first live dogfood path:

1. generate RePlan dashboard artifacts on the operator machine or a controlled
   build host;
2. copy or sync only the generated public-safe runtime artifacts to a private
   OpenProject-readable directory;
3. mount that directory read-only into all OpenProject Rails containers;
4. set `REPLAN_OPENPROJECT_OVERLAY_PATH` to the mounted
   `openproject_overlay.json` path;
5. refresh the artifact out of band after RePlan plan/report generation.

The mounted artifact directory should contain generated report outputs only,
not RePlan source, private bindings, tokens, dotenv files, Git metadata or
operator shell history.

The first runtime version may tolerate manual artifact refresh. Automatic
refresh is a separate task after the plugin page proves useful.

## First Runtime Test Choice

Do not install directly into the production-like live OpenProject instance as
the first runtime test.

The accepted first runtime test is:

```text
throwaway OpenProject runtime using the same OpenProject major image class
```

Reason:

- plugin boot failure can break OpenProject startup;
- Docker image build and asset precompile can fail for plugin-specific reasons;
- project-menu registration must be checked inside a real OpenProject Rails
  runtime;
- the live instance already carries the dogfood backlog and should not be the
  first place where plugin boot is tested.

The throwaway runtime may use a fresh database and a generated minimal
OpenProject project. It only needs enough data to verify that:

- OpenProject boots with the custom image;
- the RePlan project menu item appears;
- the RePlan plugin page opens;
- a valid overlay file renders;
- a missing overlay file renders a safe empty state;
- logs contain no boot exceptions;
- no browser-visible token or private path appears in HTML.

After this succeeds, install into the private dogfood OpenProject instance
during a maintenance window.

## Live Deployment Preflight

Before touching the live OpenProject instance:

1. confirm the current OpenProject image tag and major version;
2. confirm the current compose files and any override files;
3. confirm the current Rails service list that must share the custom image;
4. confirm the mounted asset/data volume location;
5. confirm there is a recent OpenProject backup or snapshot;
6. confirm the rollback image tag;
7. confirm the RePlan plugin source revision;
8. confirm the generated overlay artifact exists and validates as JSON;
9. run RePlan validation, smoke and plugin scaffold checks;
10. run OpenProject drift check before deployment;
11. record the maintenance window and expected downtime;
12. keep private operational details only in private infrastructure docs.

## Build Procedure

The public procedure is intentionally generic:

```text
1. Create a private deployment build directory outside the public RePlan repo.
2. Copy or checkout the intended RePlan plugin source revision.
3. Write Gemfile.plugins with the local plugin path.
4. Write a Dockerfile based on the official OpenProject plugin Docker pattern.
5. Build the custom image with an explicit tag.
6. Run a throwaway OpenProject container or compose stack with that image.
7. Verify boot, page rendering, empty state and logs.
8. Only after that, update the private OpenProject compose override to use the
   custom image for web, worker, cron and seeder.
```

The private deployment repository or private infrastructure directory should
own the concrete Dockerfile, concrete compose override and concrete secrets.

## Live Deployment Procedure

The first live deployment should be a manual, guarded maintenance operation:

1. generate and validate `reports/latest/raw/openproject_overlay.json`;
2. publish the artifact to the private mounted overlay directory;
3. build or pull the custom OpenProject image;
4. update the private compose override to use the custom image for all Rails
   services;
5. restart OpenProject through Docker Compose;
6. wait for OpenProject health checks;
7. open the normal OpenProject project page;
8. open the RePlan plugin page;
9. verify rendered counts against the RePlan dashboard artifact;
10. run the RePlan OpenProject drift check;
11. inspect logs for plugin errors;
12. record accepted evidence.

Do not add OpenProject write-back, background jobs or automatic artifact sync
in this deployment task.

## Rollback Procedure

Rollback must be explicit before the first live install.

The rollback procedure is:

1. restore the previous OpenProject image setting in the private compose
   override;
2. remove or disable `REPLAN_OPENPROJECT_OVERLAY_PATH`;
3. recreate the OpenProject Rails services through Docker Compose;
4. verify OpenProject health checks;
5. verify the ordinary OpenProject project and Gantt views;
6. run RePlan OpenProject drift check;
7. keep the failed custom image and logs for diagnosis unless they contain
   secrets.

Data rollback should not be needed for the first plugin because it is read-only
and does not add database migrations. If a future plugin version adds
migrations or writes, that version requires a stronger rollback plan and a
separate design review.

## Security Boundary

The first runtime plugin relies on OpenProject authentication and project
visibility. If a user can see the OpenProject project and the RePlan menu item,
the user can see the RePlan overlay summary for that project.

Not accepted for the first runtime install:

- separate RePlan login inside OpenProject;
- browser-visible RePlan tokens;
- browser fetches to private RePlan APIs;
- storing private bindings in OpenProject;
- reading `.env.local`;
- mounting the full RePlan repository into OpenProject;
- serving artifacts outside the same private boundary as OpenProject.

Before using the plugin in a multi-client or strict-resource-secrecy setting,
RePlan needs a separate authorization design for resource-level visibility.

## Acceptance Criteria

This deployment-path task is accepted when:

- the deployment shape is documented;
- the first runtime test target is chosen;
- the OpenProject slim-image implication is explicit;
- all Rails services that need the custom image are listed;
- overlay artifact delivery is defined;
- preflight, deploy and rollback procedures are documented;
- live installation is explicitly left to a separate task;
- no private token, private endpoint or live work-package id is committed.

## Next Task

After this design is accepted, the next implementation task should be:

```text
Install and verify read-only RePlan overlay plugin in a throwaway OpenProject runtime
```

Only after the throwaway runtime passes should the private dogfood OpenProject
instance be modified.
