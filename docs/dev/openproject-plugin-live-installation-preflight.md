# OpenProject RePlan Plugin Live Installation Preflight

Status: dated record (2026-08-07). Index: [OpenProject design index](openproject-design-index.md).

Original status: accepted preflight, live installation not yet applied.

Date: 2026-08-07.

Related completed task:
`task.self.prepare-live-openproject-plugin-installation`.

Related next task:
`task.self.execute-live-openproject-plugin-installation`.

## Purpose

This document records the guarded path from the successful throwaway
OpenProject runtime proof to the first private live installation of the
read-only RePlan overlay plugin.

The goal of this task was preparation, not a blind live mutation. The private
dogfood OpenProject instance carries the human-facing RePlan backlog, so the
first live plugin installation must be a controlled maintenance operation with
a prepared image, prepared overlay artifact, explicit rollback and evidence.

## Boundary

This document is intentionally public-safe.

It must not contain:

- live OpenProject tokens;
- private OpenProject work-package ids;
- private hostnames;
- private filesystem paths;
- VPS inventory details;
- Basic Auth credentials;
- private pilot narrative.

Concrete host paths, domain names, credentials, compose directory names and
operator-only commands belong in the private infrastructure repository.

## Confirmed Deployment Class

The private dogfood OpenProject instance is in this deployment class:

```text
Docker Compose, OpenProject 17 slim runtime, multiple Rails services.
```

The relevant Rails services are:

- `web`;
- `worker`;
- `cron`;
- `seeder`.

All Rails services must use the same plugin-capable OpenProject image. A shape
where only `web` uses the plugin image is not accepted because background jobs,
scheduled jobs, one-off seeding and Rails boot paths can load different code.

The supporting services stay on their stock images:

- database;
- cache;
- proxy;
- collaborative editing service;
- auto-heal or equivalent watchdog service.

## Image Preflight Result

A private live candidate image was built on the target host without switching
the running OpenProject services.

Sanitized tag shape:

```text
replan/openproject-replan-overlay:17-<replan-revision>
```

The candidate image includes:

- OpenProject 17 slim as the final runtime class;
- the RePlan read-only overlay plugin gem;
- `Gemfile.plugins` with explicit
  `require: "open_project/replan_overlay"`;
- OpenProject modules copied from the builder stage;
- Bundler output and precompiled assets from the builder stage.

Verification performed:

```sh
docker run --rm --entrypoint sh \
  replan/openproject-replan-overlay:17-<replan-revision> \
  -lc 'bundle info openproject-replan_overlay >/dev/null && grep openproject-replan_overlay /app/Gemfile.lock >/dev/null'
```

Result:

```text
plugin-present
```

No running live OpenProject container was replaced during this preflight.

## Preflight Defect Found And Fixed

The first live candidate image build exposed a real packaging defect.

The OpenProject full builder image and slim final image can differ in bundled
path-gem modules. The builder-stage `Gemfile.lock` referenced an OpenProject
module path that was not present in the first final image shape. This caused a
Bundler check in the final image to fail before the RePlan plugin could be
validated.

Fix:

The custom-image Dockerfile pattern now copies `/app/modules` from the builder
stage into the final slim image, in addition to the RePlan plugin, Bundler
directories, assets and `Gemfile.*` files.

The repeatable throwaway runtime script was updated so future checks use the
same safer image shape:

```text
scripts/check-openproject-plugin-throwaway-runtime.sh
```

This was found before touching the live OpenProject services, which is exactly
why the private live preflight exists.

## Build Layering Update

The OpenProject plugin image build now has a public-safe helper:

```sh
./scripts/build-openproject-plugin-image.sh
```

The helper supports these build modes through
`REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE`:

- `auto`: use the fast code-only build when the dependency image already
  exists, otherwise build the dependency image first;
- `both`: build the heavy dependency image and then the thin code image;
- `deps-base`: build only the heavy dependency image;
- `code`: build only the thin code image from an existing dependency image;
- `full`: use the older one-shot build shape for compatibility and diagnosis.

The dependency image is intentionally heavy. It keeps the exact OpenProject
version pair, Bundler output, OpenProject modules, precompiled assets and the
plugin source as it existed when the dependency image was built.

Default dependency image tag:

```text
replan/openproject-replan-overlay:17.6.0-deps
```

The fast code image starts from that dependency image and copies only:

```text
integrations/openproject/plugins/openproject-replan_overlay/
```

Use the fast `code` path for ordinary plugin implementation changes: routes,
controllers, presenters, Ruby helpers, ERB views and inline ERB styles.

Rebuild the dependency image when dependency-sensitive inputs change:
OpenProject image versions, `Gemfile.plugins`, plugin gemspec dependency
declarations, plugin load-path behavior or plugin asset pipeline behavior that
requires a fresh OpenProject asset precompile.

RePlan overlay data changes do not require any image build. Refresh only the
mounted generated artifact directory when the change is limited to RePlan
plans, resources, actuals, evidence, warnings or proposals.

## Overlay Artifact Preflight Result

A current RePlan overlay artifact was generated from the self-dogfood project
without committing generated reports:

```sh
go run ./cmd/replan report dashboard \
  --project examples/replan-self \
  --week 2026-W42 \
  --output <temporary-report-directory> \
  --format json
```

The generated file exists and is non-empty:

```text
raw/openproject_overlay.json
```

The live plugin installation must not mount the full RePlan repository into
OpenProject. It should mount only a private generated report artifact directory
read-only into the Rails services.

## Accepted Runtime Shape

The first live installation should use this shape:

```text
RePlan CLI generates overlay artifact.
Private infrastructure copies artifact to an OpenProject-readable directory.
OpenProject Rails containers mount the artifact directory read-only.
The plugin reads REPLAN_OPENPROJECT_OVERLAY_PATH.
```

The overlay artifact is runtime data. It must not be baked into the image.

The custom image is application code. It must not contain live tokens, private
bindings, `.env.local`, Git history or generated private reports.

## Generic Compose Override Shape

The private compose override should follow this public-safe shape:

```yaml
services:
  web:
    image: replan/openproject-replan-overlay:17-<replan-revision>
    environment:
      REPLAN_OPENPROJECT_OVERLAY_PATH: /run/replan/openproject_overlay.json
    volumes:
      - type: bind
        source: <private-generated-overlay-directory>
        target: /run/replan
        read_only: true

  worker:
    image: replan/openproject-replan-overlay:17-<replan-revision>
    environment:
      REPLAN_OPENPROJECT_OVERLAY_PATH: /run/replan/openproject_overlay.json
    volumes:
      - type: bind
        source: <private-generated-overlay-directory>
        target: /run/replan
        read_only: true

  cron:
    image: replan/openproject-replan-overlay:17-<replan-revision>
    environment:
      REPLAN_OPENPROJECT_OVERLAY_PATH: /run/replan/openproject_overlay.json
    volumes:
      - type: bind
        source: <private-generated-overlay-directory>
        target: /run/replan
        read_only: true

  seeder:
    image: replan/openproject-replan-overlay:17-<replan-revision>
    environment:
      REPLAN_OPENPROJECT_OVERLAY_PATH: /run/replan/openproject_overlay.json
    volumes:
      - type: bind
        source: <private-generated-overlay-directory>
        target: /run/replan
        read_only: true
```

The exact compose filename, host directory and domain belong outside the public
repository.

## Backup And Rollback Requirements

Before applying the live installation:

1. confirm a recent database backup or provider snapshot exists;
2. confirm the mounted asset directory is included in the operational backup
   plan;
3. record the current stock OpenProject image tag;
4. keep the stock compose configuration available;
5. keep the custom image tag and build context for diagnosis;
6. keep a command sequence that restores the stock image for every Rails
   service;
7. verify ordinary OpenProject health before and after the change.

The first plugin is read-only and does not introduce database migrations. Data
restore should not be part of the normal rollback path. Data restore is a last
resort if the maintenance operation exposes an unrelated corruption or operator
mistake.

## Apply Gate

The live installation is not applied by this task.

The next task must require an explicit operator decision before restarting the
live OpenProject Rails services:

```text
task.self.execute-live-openproject-plugin-installation
```

Minimum apply sequence for that task:

1. regenerate the current overlay artifact;
2. copy the artifact to the private mounted directory;
3. confirm the custom image exists on the target host;
4. write or update the private compose override;
5. run `docker compose config`;
6. confirm backup or snapshot readiness;
7. restart Rails services during the maintenance window;
8. verify OpenProject health;
9. verify the RePlan plugin page in the browser;
10. run OpenProject/RePlan drift check;
11. inspect logs;
12. update OpenProject and RePlan task status;
13. record actuals and evidence.

## Accepted Outcome

The preflight is accepted because:

- the live deployment class is known;
- the Rails service set that must share the custom image is known;
- a custom live candidate image was built on the target host;
- the candidate image was fixed after a real module-copy defect;
- Bundler confirms the RePlan plugin gem is present in the final image;
- the current RePlan overlay artifact can be generated;
- the compose override shape is defined;
- rollback requirements are defined;
- the actual live restart is split into a separate gated task.

The plugin is not yet live in OpenProject.
