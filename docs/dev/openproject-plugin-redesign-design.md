# OpenProject RePlan plugin redesign

Status: current. Index: [OpenProject design index](openproject-design-index.md).

Original status: accepted by the operator on 2026-09-25 with the amendments below.

Task: `task.self.design-openproject-plugin-gantt-view`.

Inputs:

- [`openproject-plugin-review-2026-09-25.md`](openproject-plugin-review-2026-09-25.md):
  the operator's wishes since July and their verdict of 2026-09-25;
- [`openproject-plugin-ui-audit-2026-09-25.md`](openproject-plugin-ui-audit-2026-09-25.md):
  the visual audit of the live pages;
- [`dashboard-load-profile-semantics-design.md`](dashboard-load-profile-semantics-design.md):
  bar length is the participation window, bar height is intensity, a weekly
  profile with a 100% waterline, overload drawn above it;
- a working prototype on the live self-dogfood data, generated from the
  overlay artifacts and shown to the operator as a private page. The
  prototype is the reference for layout and drawing rules; the plugin renders
  the same thing server-side.

## Principles

1. One place: everything is inside OpenProject, read-only, from the overlay.
2. The eye sees the same thing as in the OpenProject Gantt: 40-pixel rows,
   14-pixel task bars with 2-pixel corners, Monday week lines, a dotted
   status-date line, OpenProject's own colours.
3. Graphs first, text on demand: every number is in a tooltip or a
   disclosure, never the default body of the page.
4. Every resource a task uses can be seen (operator, 2026-09-25). Filtering is
   a later decision; nothing is hidden by the design.
5. Closed tasks stay available (operator, 2026-09-25), grouped under a
   collapsed "Done" section so the page opens on current work.
6. Colours come from OpenProject's Primer variables (`--fgColor-default`,
   `--bgColor-muted`, `--borderColor-default`, `--bgColor-accent-emphasis`,
   `--bgColor-danger-emphasis`, `--bgColor-attention-muted` and their
   neighbours), never hex values, so Dark Reader, which the operator uses, and
   any future OpenProject theme recolour the pages consistently.
7. Names, not ids: resources and tasks by name, work packages by number, ids in
   tooltips only.

## Project page

Top to bottom:

1. Header: "RePlan", project, status date, week, generation time. The artifact
   file names go, because OpenProject cannot serve them.
2. Five counters, each answering a question:
   - waiting on a person: the person actions of `replan next`;
   - overloaded resource-weeks ahead: weekly buckets from the status week on
     whose planned load exceeds the capacity in effect, with the list in a
     tooltip; red when above zero;
   - open tasks;
   - warnings, from `validate`, so the plugin and the CLI agree;
   - open proposals.
3. Needs attention: open tasks, soonest first, with work package, name,
   status pill, window, resources by name, and signals in words ("waits on
   you", "waits for 2 tasks", "1 warning").
4. Plan, the Gantt-like view:
   - one shared axis: month labels, Monday week lines with day numbers, the
     status-date line;
   - range control: "4 weeks back and ahead" (default) or "whole project";
   - per task a 40-pixel row with work package, name, status and window on the
     left and a 14-pixel bar on the axis; closed tasks in grey under "Done
     (n)", collapsed;
   - under each task, one 26-pixel lane per demand, toggled by "resource
     lanes": a box on the demand's participation window, filled to a height
     of daily load over daily capacity; 100% sits at 60% of the lane height,
     so load up to about 165% is drawn and overload turns red; a demand
     without known capacity shows an empty box;
   - tooltips carry the amounts, units, days and the percentage.
5. Load by week: every resource the plan uses, grouped as people, roles, AI
   agents, environments, services and quotas, release windows. One row per
   resource on the same axis, one bar per ISO week: height is planned load
   over the capacity in effect that week, the dashed line is 100%, load above
   it is a red hump; slot resources show their busiest day. Capacity changes,
   such as the operator's 10 to 5 hours on 2026-09-28, show as a change in
   what 100% means from that week on.

## Work-package panel

- Header: work package number and name, status, planned window, planned
  amounts per resource, links to the OpenProject work package and back to the
  project page.
- The task on its own axis, from a week before to a week after its window,
  with every resource lane, drawn exactly as on the project page.
- What was done: evidence summaries with date and kind.
- Time booked: actuals with date, resource and amount.
- Warnings, proposals and dependencies in words, with task names.
- An unknown work package answers 404 with the existing explanation.
- Reached from the project page now, and from the OpenProject work package
  itself once `task.self.link-openproject-work-package-to-replan-panel` lands.

## Drawing rules

- Positions are percentages of the visible range, computed server-side; the
  range control and the lane toggle are the only client-side behaviour, in a
  small inline script with no library. Without script the page shows the
  default range with lanes.
- Daily load of a period demand is its amount spread over its resource's
  working days (GAP-031 rule); daily capacity is the week's capacity in effect
  over the resource's working days. Slot demands use slots over slot capacity.
- OpenProject's Content Security Policy refuses inline scripts without its
  nonce (found on 2026-09-25, when a script injected into the page was
  refused). The plugin's inline script carries the nonce through Rails'
  `javascript_tag nonce: true`, and the rendered-page test checks that it
  does.
- Weekly buckets come from the overlay, computed by RePlan with the same rule
  as `replan workload`; the plugin does no planning arithmetic of its own.
- DOM budget: the prototype draws 96 tasks and about 380 lanes in the whole
  project range; the default range draws open tasks and their lanes. Rows
  outside the range are not rendered.

## Overlay contract, version 2

Added for `task.self.extend-overlay-contract-for-plugin-views`, all derived by
RePlan, none computed in the plugin:

- `resources`: id, name, class, unit, working weekdays, and weekly buckets
  (label, start, finish, capacity in effect, planned, overload, concurrent);
- per work package: task name, planned and actual window, lanes (demand,
  resource, role, unit, amount, window, amount per working day, daily capacity
  in effect at the window start, concurrent), evidence (date, kind, summary),
  actuals (date, resource, amount, unit), warnings from `validate` (code and
  message), open proposals (id and label), dependencies (other task's name,
  number, status, required or preferred);
- project: date span, status-date offset, and the person actions of
  `replan next` for the first counter;
- `schema_version: 2`; the plugin reads version 1 fields until the views move,
  and shows a clear notice for a version it does not know.

Implemented on 2026-09-25; the field reference is in
[`../spec/openproject-overlay-strategy.md`](../spec/openproject-overlay-strategy.md#overlay-json-contract).
Two choices made while building it: the "overloaded resource-weeks ahead"
counter reads `overloaded_weeks`, computed with `replan workload --week` from
the status week on, so work already done does not count as overload, while
the weekly bars keep planned load; and a warning about a dependency belongs to
the task that waits on it. The actual window is the first and last booked
consumption, because task status changes live in the audit log, not in
actuals.

## Tests

- rendered-page checks (`task.self.add-plugin-rendered-page-checks`): both
  pages requested over HTTP as a non-admin member; status codes, sections,
  escaping of task names, the 404 panel, missing and invalid overlays;
- presenter unit checks for positions at the range edges, the 60% line, the
  overload cap and the Done grouping;
- a screenshot of both pages at 1600 and 900 pixels after every milestone
  deploy, compared with the prototype by eye and recorded in the milestone's
  evidence.

## Out of scope

- changing OpenProject's own Gantt, still deferred;
- editing anything from the plugin;
- filters and saved views, decided after milestone 2 with the operator.

## Questions for the operator

1. Does the prototype show what you meant by the Gantt with resources and the
   load profile?
2. Default range: four weeks back and everything ahead, or something else?
3. Resource lanes on by default, or off with a switch?

## Amendments after the operator's review, 2026-09-25

The operator looked at the prototype, found it fine overall, and asked for
four things, all adopted and shown in the second version of the prototype:

1. The Gantt is its own page in the RePlan menu, next to the overview, so its
   date header stays pinned while the rows scroll; the task column stays
   pinned on the left while time scrolls sideways.
2. Flexible range: presets (2 weeks, month, quarter, whole project) and free
   "from" and "to" dates, plus a scale in days, weeks or months.
3. Moving around: a slider for moving along time under the chart, and buttons
   to jump to the status date, to the range start and to the top.
4. Drill-down: every task name and bar, on the overview and on the Gantt,
   opens that task's own page, with a way back to where the operator came
   from. The task page shows the task and its lanes on their own axis, what
   was done, time booked and what it depends on, with the predecessor as a
   link to its own page.

Consequences:

- the overview page keeps the counters and the attention table; the Gantt page
  holds the plan and the load by week on one axis;
- weekends are shaded when the scale shows days or weeks;
- the page is rendered server-side for the requested range and scale; range,
  scale and toggles are URL parameters, so a view can be bookmarked and the
  back button works; the slider and jump buttons are a small inline script;
- new backlog task: `task.self.add-plugin-gantt-navigation`, after the Gantt
  build and before the load profile.

## Second round of amendments, 2026-09-25

After the second prototype version the operator reported one defect and asked
for two more things, all shown in the third version:

1. Defect: on scrolling right, the pinned task column slid away and the grid
   stopped part way. Each row was only as wide as the visible area, so the
   pinned cell, which is pinned inside its row, left with the row's end. Rows
   are now as wide as the whole canvas. The plugin must render rows at full
   canvas width; the rendered-page test checks the pinned column's position at
   the far right of the widest range.
2. Several task pages at once: up to five task pages stay open as tabs next to
   Overview and Gantt, each with a close button; opening a sixth replaces the
   one used least recently; closing the current tab returns to where it was
   opened from. In the plugin these are separate URLs, so the tab row is kept
   in the page and survives navigation within RePlan pages.
3. Full screen: a button on the Gantt page puts the Gantt, with its toolbar,
   into the browser's full-screen mode; where the browser refuses, the Gantt
   fills the window instead. Esc leaves both.

All three belong to `task.self.add-plugin-gantt-navigation`.

## Third round, 2026-09-25: room for the chart

The operator asked for more viewing area. The fourth prototype version:

- folds the Gantt controls (range, scale, toggles, jumps, legend) into one
  slim bar with "Controls", "Hide page header", "Full screen" and a line that
  names the current range and scale;
- turns the extra time slider off by default, with a "time slider" toggle
  among the controls, since the browser's own scrollbar does the same;
- fits the chart's height to the window whenever the controls, the header,
  the slider or full screen change. At a window 800 pixels high the chart
  grows from 493 to 669 pixels with everything folded.

In the plugin the folded state and the slider toggle are remembered per
browser, and the rendered-page test checks the default state. Part of
`task.self.add-plugin-gantt-navigation`.

## Milestone 1 project page, 2026-09-26

`task.self.restructure-plugin-project-page` built items 1 to 3 of the project
page and kept, until the Gantt page lands, the task list with resource lanes,
now with names, amounts behind a disclosure and accepted tasks under "Done".
Decisions made while building it:

- the overloaded-weeks counter lists its weeks in the card as well as in the
  tooltip, because a red number alone does not say who and when;
- "Waiting on a person" is also a list under the counters, each item a link
  to its task page;
- OpenProject's tables cut cell text with an ellipsis and a 300-pixel cap; the
  attention table wraps task names and resources instead and scrolls inside
  its own box if it must;
- a version 1 overlay still renders, with a dash for the counters it cannot
  answer and a notice saying so.

## Deploying by part, 2026-09-26

After milestone 1 went live the operator found no trace of the approved
prototype on the live page: milestone 1 only rebuilt the top of the project
page, and the old resource strips remained below. Agreed the same day:
milestone 1 is accepted as it is, the Gantt page starts at once, and each
later part, the Gantt page, its navigation, the load profile and the panel,
goes live as soon as the operator accepts it, with the same guarded
procedure, instead of waiting for the milestone 2 deploy. Reports name what a
deploy contains and what it does not yet contain.

## Gantt page, 2026-09-26

`task.self.implement-openproject-plugin-gantt-view` built the Gantt page as a
child of RePlan in the project menu, with an Overview and Gantt switch on both
pages. It is rendered on the server by `GanttPresenter`, plain Ruby with its
own tests: the default range from four weeks before the status date to the
end of the plan, 14 pixels a day, month and Monday ticks, weekends, the
status line, 40-pixel task rows with 14-pixel bars, 26-pixel resource lanes
whose fill is daily load over daily capacity with 100% at 60% of the lane,
and closed tasks under a collapsed "Done". Rows outside the range are not
rendered: the self plan draws 254 rows and 197 lanes. The date axis and the
task column stay pinned; after scrolling to the far right and the bottom both
were measured in place. `from`, `to` and `lanes=0` are URL parameters already;
the controls for them, the scale, tabs and full screen are
`task.self.add-plugin-gantt-navigation`, and the weekly load profile is
`task.self.add-plugin-resource-load-profile`.

The scaffold check now reads every public controller action and fails when
the permission does not grant it, instead of matching a fixed list, which the
new action would have broken the same way the panel once was.

## Gantt navigation, 2026-09-26

`task.self.add-plugin-gantt-navigation` added, on the server: range presets
(2 weeks, month, quarter, whole project around the status date), free from and
to dates, a days, weeks or months scale and the resource-lane toggle, all as
URL parameters in a plain form that works without script. The one script of
the RePlan pages, carrying OpenProject's nonce, adds jumps to the status date,
the range start and the top, a time slider that is off by default, folding of
the controls and the page header into the slim bar, full screen with a
maximised fallback, a chart height fitted to the window, and task tabs: up to
five task pages stay open across the Overview, Gantt and task pages, the
least recently used is dropped, and closing the current one returns to where
it was opened from. The folded state, the slider and the tabs are remembered
per browser. OpenProject navigates with Turbo, which leaves the referrer
empty, so the script remembers the last RePlan page itself.

## Load profile, 2026-09-26

`task.self.add-plugin-resource-load-profile` added "Load by week" under the
tasks on the Gantt page: a row per resource the plan uses, grouped as people,
roles, AI agents, environments, services and quotas, and release windows,
drawn from the overlay's weekly buckets on the same axis. A week's bar height
is its planned load over its capacity, the dashed waterline is 100% at 55% of
the row, and a week above capacity grows a hump above the waterline, capped at
180%. The tooltip gives the amounts.

Decision made while building it: the overlay's buckets are the plan, done
work included, so many past weeks and the status week are above capacity by
plan while RePlan's workload, which counts only remaining work, finds a single
overloaded resource-week ahead. Drawing every such hump red would contradict
the overview's counter. A hump is red only for a resource-week the overlay
lists as still overloaded, the same ones the counter counts; other humps are
grey with a tooltip saying the plan went over but no overload remains. Past
weeks are faded. A week with planned load and no capacity gets a low neutral
bar marked "no capacity set".

`load=0` hides the profile like `lanes=0` hides the lanes, with a checkbox in
the controls, and a "Load by week" jump scrolls it under the axis.

## Task page, 2026-09-26

`task.self.show-evidence-and-actuals-in-plugin-panel` replaced the old panel,
which showed counts, a task window without dates and resource strips that
ignored their windows. `TaskPresenter`, plain Ruby with its own tests, builds
the page from the overlay row:

- the status, planned window and the dates time was booked, with a link to
  the work package in OpenProject;
- warnings and open proposals in words, and overloaded resources;
- the task and its resource lanes on an axis of their own, a week before and
  after the task window, in days up to 45 days and in weeks beyond, drawn by
  the Gantt page's own presenter and markup, so a lane means the same on both
  pages;
- what was done, newest first, with date, kind and summary;
- planned, booked and remaining amounts and the window of every resource
  demand, and every booking with date, resource and amount;
- what the task depends on and what waits on it, each as a link to its own
  task page, with the dependency's criticality and state and the other
  task's status.

The Gantt's styles and date axis became partials shared by both pages. An
unknown work package answers 404 with the explanation on the page, and a
version 1 overlay still renders the chart with a notice that the rest needs
version 2.

## Overview without the old list, 2026-09-27

`task.self.remove-old-task-list-from-plugin-overview` removed what milestone 1
had kept until the Gantt page landed: the task list with resource strips and
the resource cards with planned and remaining totals. The overview is now the
design's items 1 to 3, the header, the counters with the waiting list and the
attention table, and a line pointing to the Gantt for the plan, closed tasks
and the load by week. The presenter code, the partial and the styles that
only served them went with them. The scaffold check now checks every view
and partial in the directory instead of a list, which had missed the Gantt's
new shared partials.

## One overlay per project, 2026-10-12

`task.self.scope-plugin-overlay-to-its-project`. Until now the plugin read one
overlay file and showed it on the RePlan pages of every project with the
module enabled, so a second project in the same OpenProject would have shown
the first project's plan to its own members.

`OverlayLoader` now takes the page's project and shows an overlay only to the
project it names in `openproject_project_id`:

- a server may hold an overlay per project in a directory,
  `REPLAN_OPENPROJECT_OVERLAY_DIR/<project identifier>/openproject_overlay.json`,
  or with a `raw/` level, as `replan report dashboard` writes it;
- the single file of `REPLAN_OPENPROJECT_OVERLAY_PATH` still works, for the
  project it belongs to only, so an existing installation needs no change;
- a project without an overlay of its own gets "This project has no RePlan
  overlay yet", on the overview, the Gantt and every task page, with no task,
  no name of the other project and no file path;
- a file placed in another project's directory, and an overlay that names no
  project, are refused the same way;
- the identifier goes into a path only when it is letters, digits, dashes and
  underscores.

The loader no longer needs Rails to be loaded, so its seven tests run with the
presenters' in the scaffold check. The rendered checks, now 164, add a second
project whose member is the same user: it sees none of the first project's
tasks on four pages, sees its own overlay once one is in the directory, and
the first project keeps its own beside it.

### Publishing a project's overlay

`task.self.serve-plugin-overlays-per-project-live` set the directory up on the
live instance: the compose override names it in
`REPLAN_OPENPROJECT_OVERLAY_DIR` and mounts it read-only into web, worker,
cron and seeder, beside the single file, which stays as the fallback.

A project publishes its plan in two steps, from a checkout of this
repository with the project's own settings:

```sh
scripts/build-openproject-live-overlay.sh          # build, with real work-package ids
scripts/publish-openproject-overlay.sh             # dry run
REPLAN_APPLY=1 scripts/publish-openproject-overlay.sh
```

`REPLAN_PROJECT`, `REPLAN_OPENPROJECT_PROJECT_ID` and
`REPLAN_OPENPROJECT_BINDINGS_PATH` select the plan; the private
`REPLAN_OVERLAY_PUBLISH_TARGET` is the overlay directory as rsync addresses
it. The overlay goes to `<target>/<project identifier>/` only, so one project
cannot overwrite another's. The helper refuses an overlay that names another
project or none; the name comes from `project_id` of the plan's OpenProject
source profile in `.replan/project.yaml`. No restart is needed: the plugin
reads the file on every request.
