# OpenProject RePlan Plugin Throwaway Runtime Test

Status: dated record (2026-08-07). Index: [OpenProject design index](openproject-design-index.md).

Original status: completed runtime proof.

Date: 2026-08-07.

Related task: `task.self.verify-openproject-plugin-throwaway-runtime`.

## Purpose

This document records the first real OpenProject runtime verification for the
read-only RePlan overlay plugin.

The goal was to prove that the plugin is not only a repository scaffold, but
can be packaged into an OpenProject Docker runtime, loaded by Rails and pointed
at a generated RePlan overlay artifact.

This task deliberately did not install the plugin into the private live
OpenProject dogfood instance.

## Verification Command

The repeatable heavy check is:

```sh
./scripts/check-openproject-plugin-throwaway-runtime.sh
```

This check is intentionally not part of `./scripts/smoke-mvp.sh` because it
builds a custom OpenProject image, starts disposable Docker services and
initializes a clean OpenProject database.

## What The Check Does

The script performs the following sequence:

1. copies the RePlan OpenProject plugin scaffold into a temporary Docker build
   context;
2. writes a minimal `Gemfile.plugins` for OpenProject;
3. builds a custom OpenProject image through
   `scripts/build-openproject-plugin-image.sh`, using the optimized dependency
   base plus code-only layer when available;
4. verifies that Bundler can see the `openproject-replan_overlay` gem;
5. generates `reports/latest/raw/openproject_overlay.json` from
   `examples/replan-self`;
6. creates a disposable Docker network;
7. starts disposable PostgreSQL and Memcached containers;
8. runs the OpenProject production seeder against the clean database;
9. runs a Rails runner check with the generated overlay mounted read-only;
10. verifies that the plugin route, permission and project menu are registered;
11. verifies that the overlay loader returns work-package rows;
12. runs a second Rails runner check with a missing overlay path;
13. verifies that the missing-overlay state returns `overlay_file_missing`
    instead of crashing.

Since 2026-09-25 the check goes on, in the same disposable services:

14. creates a project with the RePlan module, a member who is not an admin
    and holds only `view_replan_overlay` with `view_project`, and an outsider
    member whose role lacks `view_replan_overlay`;
15. starts the OpenProject web server with the overlay directory mounted, and
    runs `scripts/check-openproject-plugin-rendered-pages.sh` against it.

## Rendered Page Checks

Task: `task.self.add-plugin-rendered-page-checks`, from the plugin UI audit of
2026-09-25. Until then no check rendered either plugin page, which is how a
permission that refused the work-package panel to every non-admin lived for
six weeks on the live instance.

The script signs in through the login form, as a browser does, and requests
both pages over HTTP. It checks:

| Case | Project page | Work-package panel |
|---|---|---|
| Member, valid overlay | 200; the five counters, "Needs attention", "Resources", "Tasks" with the collapsed "Done (n)" and the "Amounts" disclosures; resources by name, not id; no artifact line; the RePlan item in the project menu | 200; "Task window", "Resources", the work package number, the link back |
| Markup in a task title | escaped, never raw | escaped, never raw |
| Colours | OpenProject page header; no fixed colour in the plugin's styles | the same |
| Work package unknown to the overlay | | 404 with "No RePlan overlay for this work package." |
| Gantt page, member | 200; axis, bars, lanes, status line, "Open (n)", the Gantt item in the project menu, range presets, from and to, scale, jumps, the time slider hidden; days and months scales; lanes hidden with `lanes=0`; a malformed range still renders | |
| Script | on every page, carrying the nonce of the page's Content Security Policy header | the same |
| Member without `view_replan_overlay` | 403, and no RePlan item in the project menu | 403 |
| Signed out | 302 to the login | 302 to the login |
| Overlay file missing | 200 with the reason | 200 with the reason |
| Overlay not valid JSON | 200 with the reason | 200 with the reason |
| Overlay without a `work_packages` array | 200 with the reason | 200 with the reason |
| Overlay of a schema version the plugin does not read | 200 with the reason | 200 with the reason |

The plugin reads the overlay on every request, so one server serves all the
cases: the script swaps the file in the mounted directory between requests and
restores the valid overlay at the end. Setting `REPLAN_RENDERED_FAILED_PAGES`
to a directory keeps the body of every page a check failed on.

The unknown work package used to answer 200; it answers 404 now, as the
accepted plugin design specifies.

The checks were proved against the defects they exist for. With the
permission reduced to `show` again, as it was before 2026-09-25, and one task
title rendered raw, the run failed: the panel answered 500 to the member, as
it did on the live instance, and the raw `<script>` title was found. With both
restored, 47 checks passed; the overlay version check of
`task.self.extend-overlay-contract-for-plugin-views` brought them to 53, and
the colour checks of `task.self.theme-plugin-views-with-openproject-variables`
to 55, and the restructured project page of
`task.self.restructure-plugin-project-page` to 64, and the Gantt page of
`task.self.implement-openproject-plugin-gantt-view`, checked for every case
above, to 95, and the navigation of `task.self.add-plugin-gantt-navigation`
to 109, and the weekly load profile of
`task.self.add-plugin-resource-load-profile` to 120: its rows, waterline and
jump, a red hump only for the resource-week still overloaded, a grey one where
the plan went over but the work is done, and `load=0`. A first run matched the
profile's marker in the page's own script and so failed `load=0` falsely; the
checks now look for markup only. The task page of
`task.self.show-evidence-and-actuals-in-plugin-panel` brought them to 135:
its sections and its own chart on the open task, whose title, an evidence
summary and a booked resource name carry the hostile markup, and a second
task with evidence, booked time and a predecessor linked to its own page. Removing milestone 1's task list and
resource cards from the overview, `task.self.remove-old-task-list-from-plugin-overview`,
made them 137: the overview must point to the Gantt and must not carry the
old sections; the check matches their headings, which the attention table's
own "Resources" column does not.

The script's behaviour needs a browser. `test/browser/navigation_check.cjs`
in the plugin runs 23 checks with Playwright against the held server: the
script runs under OpenProject's policy, folding the controls and the page
header gives the chart room and is remembered, the time slider and the jumps
move the chart, the load profile's waterline sits at 55% of its row and a red
hump starts on it, full screen opens and Esc leaves it, at most five task tabs
stay open with the least recently used dropped, and closing the current tab
returns to where it was opened from. With the nonce removed, OpenProject
refused the script and the checks failed at the first control. It is not in
CI, which has no browser. The presenter has its own
Minitest checks, run by `scripts/check-openproject-plugin-scaffold.sh` and so
by the smoke: counters, the version 1 fallback, the attention order and its
signals, names, and the Done grouping.

For screenshots and a look in a browser, `REPLAN_OPENPROJECT_RUNTIME_PORT`
fixes the web server's port, which also becomes OpenProject's host name, and
`REPLAN_OPENPROJECT_RUNTIME_HOLD` keeps the server up for that many seconds
after the checks, printing the address and the member's login.

Later plugin tasks extend this script with the checks the design names for
their views, starting with the redesigned project page and the Gantt.

Expected success output includes:

```json
{"ok":true,"check":"valid_overlay","work_packages":45,"routes":["project_replan_overlay"],"plugin":"openproject_replan_overlay","permission":"view_replan_overlay","menu":"replan_overlay"}
{"ok":true,"check":"missing_overlay","available":false,"error":"overlay_file_missing"}
openproject plugin rendered pages ok: 164 checks
openproject replan link checks ok: 8 checks
openproject agent kit live checks ok: 51 checks
openproject plugin throwaway runtime ok
```

## Runtime Findings

The runtime check found and fixed two real packaging defects.

First, `Gemfile.plugins` must explicitly require the RePlan plugin entrypoint:

```ruby
group :opf_plugins do
  gem "openproject-replan_overlay",
      path: "/app/vendor/plugins/openproject-replan_overlay",
      require: "open_project/replan_overlay"
end
```

Without the explicit `require`, Bundler can install the gem, but OpenProject
does not load the plugin engine, so Rails does not register the route, menu or
permission.

Second, the plugin route must be added to `Rails.application.routes`, not only
to the isolated engine route set. OpenProject plugin route files are loaded
into the application route set.

The plugin scaffold now uses:

```ruby
Rails.application.routes.draw do
  scope "projects/:project_id", as: "project" do
    get "replan_overlay",
        to: "open_project/replan_overlay/overlays#show",
        as: :replan_overlay
  end
end
```

## Accepted Result

The throwaway runtime proof is accepted.

Confirmed:

- the custom OpenProject image builds;
- Bundler sees the RePlan plugin gem;
- a clean OpenProject database can be initialized with the custom image;
- the RePlan plugin route is visible as `project_replan_overlay`;
- the RePlan project permission is visible as `view_replan_overlay`;
- the RePlan project menu item is visible as `replan_overlay`;
- a generated overlay artifact loads successfully;
- the current self-dogfood overlay contains 45 OpenProject-linked work-package
  rows;
- a missing overlay path produces a safe `overlay_file_missing` state.

## Build Helper

The runtime check no longer carries its own Dockerfile template. It delegates
image construction to:

```sh
./scripts/build-openproject-plugin-image.sh
```

This keeps throwaway verification and private live operations on the same image
construction path. The helper can still perform the older full build, but its
normal mode is `auto`: build the heavy dependency image once when missing, then
build fast code-only images for ordinary plugin route, controller, presenter
and ERB changes.

Observed non-blocking build warning:

```text
Error loading RubyGems plugin ... rdoc_plugin.rb ... LoadError
```

This warning appeared during `bundle install`, but the build completed and the
runtime check passed.

## Limits

This test does not prove live production readiness.

Not yet covered:

- installing the plugin into the private live OpenProject instance;
- browser-login verification through the real reverse proxy;
- visual review of the OpenProject page in a browser, which each plugin
  milestone deploy records with screenshots;
- private artifact publishing and refresh automation;
- rollback of the private live instance.

Those are separate live-deployment tasks.

## Next Step

The next OpenProject plugin task is a private live deployment preflight and
install plan:

```text
task.self.prepare-live-openproject-plugin-installation
```

That task should reuse this runtime proof, but must additionally handle backup,
maintenance window, private compose override, artifact mount, rollback image,
real project role permissions and browser verification.
