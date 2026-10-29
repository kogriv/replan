# OpenProject RePlan plugin review 2026-09-25

Status: dated record (2026-09-25). Index: [OpenProject design index](openproject-design-index.md).

Original status: done; the operator gave their verdict on 2026-09-25 (section 6).

Task: `task.self.review-openproject-replan-plugin-views`, started early on the
operator's request.

## Method

- requirements: the design documents of the plugin track and the operator's
  own words in earlier sessions since July, collected into section 1;
- code: the whole `openproject-replan_overlay` plugin, about 1 000 lines, and
  the overlay JSON it reads;
- visuals: the live OpenProject, opened with Playwright as a dedicated
  read-only user (`replan-ui-review`, role `RePlan UI reviewer`: the Reader
  permissions without comment and message rights, plus
  `view_replan_overlay`; two-factor authentication like every user). This
  sees the pages the way a non-admin project member sees them.

## 1. What was asked

The operator's wishes, in their own words, from July to September:

- one place, not two: "надо держать все в одном месте… видеть сами задачи и
  как они снабжены ресурсами" ("we need to keep everything in one place… see
  the tasks themselves and how they are supplied with resources", 2026-08-06),
  the plugin is mandatory;
- the Gantt is the favourite view, and the eye should see the same thing in
  OpenProject and in RePlan (2026-07-20);
- per task, a lane for each resource with its own time bar, only over the
  resource's participation window (2026-07-22, 2026-08-06);
- bar height is intensity; a summary that adds heights into a load profile
  with a 100% waterline, and overload visible as a hump above it
  (2026-08-06);
- graphs, not text; text behind a disclosure; no colour games yet;
- vertical day grid lines; a dark theme (2026-07-20, 2026-07-22);
- after the first plugin table: "вьюху надо дорабатывать… можем ли мы менять
  гант-вью" ("the view needs more work… can we change the Gantt view?",
  2026-08-07).

The accepted sequencing is project page, then work-package panel, then a
Gantt-like view inside the plugin, then the native Gantt only if justified.
The operator never gave the review of the first two surfaces that Step 43
asked for.

## 2. What is there

- a project page `RePlan` in the project menu: six summary counters, a card
  per resource, a "Current attention" table, and a "Task resource timeline";
- a work-package page `RePlan panel`, linked from every row of the project
  page, not from the OpenProject work package itself.

## 3. Findings

Defects:

1. The work-package panel returns HTTP 500 for every user who is not an
   OpenProject admin. `view_replan_overlay` grants only the `show` action;
   `work_package` is not in the permission, OpenProject logs "Used permission
   … that is not defined" and fails. The operator is not an admin, so every
   `RePlan panel` link on the project page is broken for them. The August
   live check ran as an admin and could not see it.
2. The timeline is not a timeline. The task bar has no date axis, no day or
   week grid and no status-date line. The resource bars under it always span
   the full width, whatever the task or participation window, so they do not
   line up with the task bar. The overlay already carries per-demand
   `start_offset_days` and `duration_days`; the view ignores them.
3. Intensity is not readable. Each resource bar is a strip about ten pixels
   high whose fill height is the intensity, so 5% and 100% differ by a few
   pixels. Values over 100% are clipped. Intensity against a weekly agent
   capacity of 40 hours gives numbers like 1.3% that say nothing.
4. There is no load profile. No resource shows load over time, no 100%
   waterline, no overload hump: the one view the operator described in the
   most detail. The dashboard's `gantt.json` has daily resource loads; the
   overlay the plugin reads does not.
5. The page shows the whole history: all 83 tasks, 74 of them accepted,
   each with three to five text-heavy resource rows. The OpenProject Gantt
   next to it shows the seven open tasks with an axis, a today line and
   relations.
6. Warnings disagree with `validate`: the overlay marks the two plugin tasks
   `dependency_open` while `validate` is quiet about the same dependencies
   since the due-only rule of 2026-09-25.
7. Colours are hard-coded light values; there is no dark theme and no use of
   OpenProject's own theme variables.
8. Ids instead of names: `resource.ai.agent.executor` rather than the
   resource name, in every card and row.

Smaller:

- the overlay is one global file, so every project with the module enabled
  shows the RePlan self-dogfood data;
- the summary card "283 resource demands" is a count nobody acts on;
- no link from the OpenProject work package to its RePlan panel (placement B,
  deferred by design).

What works:

- menu entry, module and permission on the project page; the page loads for
  a non-admin member;
- the data contract is richer than the view: windows, intensity, capacity,
  evidence and tracking are all in the overlay;
- read-only, no tokens in the browser, safe empty states.

## 4. Assessment

Against the operator's own target, a Gantt-style view of tasks with their
resources, windows, intensity and a load profile under a 100% waterline, the
plugin is at the first step: it proves that RePlan data can live inside
OpenProject and it lists the data, but it does not yet show it as the
operator asked. The project page is a text list with decorative bars. The
work-package panel, the second step of the sequence, is broken for the
operator.

Recommendation for the design task: fix the permission first (a one-line
change and a non-admin check), then design the Gantt-like view around three
things only, open tasks by default with a date axis and status line, resource
lanes positioned by participation window with readable height, and a weekly
load profile per human resource with the waterline, and move the rest behind
disclosures. The overlay needs the load profile added.

## 5. Questions for the operator

1. Do the findings match what you see and feel on the page?
2. Which resources matter in the view: people only, or also agents, CI and
   the development slot?
3. Default scope: open tasks only, or open plus the last two weeks?
4. Do you use OpenProject in the dark theme?

## 6. Operator verdict, 2026-09-25

1. The findings match what the operator sees on the page.
2. The view must be able to show every resource a task uses, as recorded in
   its demands: people, agents, CI, environments. Filtering is decided later.
3. Closed tasks may be shown too; history is not to be hidden by default.
4. OpenProject has no dark theme switch the operator found; they use the
   Dark Reader browser extension. The plugin should therefore take its colours
   from OpenProject's own CSS variables rather than hard-coded values, so that
   an extension or a future OpenProject theme recolours it consistently.

Consequences for the design task: all demands of a task are lanes, closed
tasks stay visible, colours come from OpenProject variables, and the
permission defect is fixed first as its own task.
