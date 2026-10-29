# OpenProject RePlan plugin UI audit 2026-10-15

Status: dated record (2026-10-15). Index: [OpenProject design index](openproject-design-index.md).

Original status: agent audit; input to `task.self.deploy-plugin-milestone-3-and-accept-track`.

A rerun of the audit of
[2026-09-25](openproject-plugin-ui-audit-2026-09-25.md) against the finished
plugin. Like that one, it was made in the live OpenProject with Playwright, as
the read-only non-admin review user, at 1600 and 900 pixels wide. The live
image was built from the plugin code in this repository; the only plugin
change since it is a README.

## Measurements

```text
page                         status  DOM nodes  HTML    load
overview, 1600 px            200     638        129 KB  3.7 s
Gantt, 1600 px               200     2 648      317 KB  3.3 s
task page, open task         200     679        138 KB  3.7 s
task page, accepted task     200     719        142 KB  4.1 s
task page, unknown package   404     516        109 KB  3.5 s
same pages at 900 px         same    same       same    1.9-4.6 s
horizontal page scroll       none, at either width
console errors               Gravatar avatars only, OpenProject's, not the plugin's
```

On 2026-09-25 the project page was 28 screens at 1600 pixels and 3 013 DOM
nodes, and it scrolled sideways at 900. The overview now fits one screen at
1600 pixels. The Gantt scrolls inside its own frame. The task pages are one to
two screens.

## The 2026-09-25 findings, one by one

```text
finding (2026-09-25)                                 now
28 screens, text-first project page                  overview of one screen: five
                                                     counters, "Needs attention", a
                                                     pointer to the Gantt
counters nobody acts on (287 demands)                waiting on a person, overloaded
                                                     weeks ahead, open tasks,
                                                     warnings, open proposals
resource cards without time                          gone; a load profile by week
                                                     with the 100% line on the Gantt
dateless task track                                  month and week axis, status-date
                                                     line, range and scale controls
strips spanning the full width                       lanes placed by participation
                                                     window, under the task bar
10-pixel strips, 140% clipped                        lanes and load bars with a dashed
                                                     100% line; overload is a red hump
accepted tasks fill 26 screens                       collapsed "Done (n)" group on the
                                                     Gantt
ids where names exist                                names, ids in tooltips
evidence and actuals only as counts                  "What was done" with dated
                                                     evidence summaries, "Time booked"
                                                     with dates, planned / booked /
                                                     remaining per resource
no way in from the work package                      "RePlan" link field on every
                                                     bound work package
unknown package served as 200                        404 with the explanation
fixed light colours                                  Primer variables only
sidebar glyph over the title at 900 px               OpenProject page header and
                                                     breadcrumbs, no overlap
no rendered page in any check                        rendered-page checks as member,
                                                     outsider and signed-out visitor;
                                                     browser checks of the script
```

## Assessment

```text
criterion                               2026-09-25  now   note
works for the operator                  3/5         5/5   overview, Gantt, task pages,
                                                          link from the work package
shows what the operator asked for       1/5         5/5   every recorded wish, see the
                                                          list below
readable at a glance                    1/5         4/5   one screen of overview; the
                                                          Gantt is dense at 900 px
fits OpenProject visually               3/5         5/5   page header, tabs, tables and
                                                          colours are OpenProject's
data behind the view                    4/5         4/5   current, as long as it is
                                                          published (see below)
tested as a user would use it           1/5         4/5   rendered and browser checks
                                                          on a throwaway instance
```

## Against the operator's wishes

The wishes recorded in the
[review of 2026-09-25](openproject-plugin-review-2026-09-25.md), July to
September, and the verdict of that day:

```text
wish                                                  where it is
one place: tasks and their resources in OpenProject   RePlan module in the project
                                                      menu; link field on the work
                                                      package
the Gantt as the main view, the same look as          plugin Gantt: 40-pixel rows,
OpenProject's                                         Monday lines, dotted status date
a lane per resource over its participation window     lanes under each task bar
bar height is intensity; a load profile with a        "Load by week", dashed 100%
100% waterline, overload as a hump                    line, red hump while overloaded
graphs first, text behind a disclosure                numbers in tooltips and
                                                      disclosures
vertical day grid lines                               a days scale on the Gantt; task
                                                      pages draw days up to 45 days
dark theme: colours follow OpenProject and Dark       Primer variables only
Reader
every resource a task uses can be shown               all demands are lanes; filtering
                                                      left for later, as decided
closed tasks visible, history not hidden              "Done (n)" on the Gantt, task
                                                      pages for accepted tasks
```

Left open by the operator's own decisions: resource filtering ("decided
later"). The native OpenProject Gantt stays unchanged ("only if justified").

## Found in this rerun

In the plugin, both fixed with this audit:

1. An unknown work package page opened a task tab, and the tab stayed
   in the remembered tabs on every page afterwards. Such a page now opens no
   tab and drops a tab remembered for it.
2. The "Work package" column header of "Needs attention" wrapped into two
   lines at 1600 pixels; kept on one line, it ran into the next header, as
   the number column is narrow. The header is now "ID", as in OpenProject's
   own work package table, with the full name in its tooltip.

In the operating loop, which the plugin cannot see:

3. The live overlay was three days old and two tasks short. The page of an
   accepted task answered 404 although its work package linked to it.
   Publishing the overlay was a manual step outside the loop. It is now a
   loop step in the [backlog convention](backlog-operating-convention.md),
   after every added, rescheduled or accepted task.
4. The status date had stayed at the previous Monday. The accepted task's
   page warned that it finished after the status date and asked for a
   scenario: a false alarm. Moving the status date is now part of the loop.
5. The task under audit was started a month early. Its window still showed
   the planned November date until a scenario moved it.

Seen in OpenProject, outside the plugin:

- The "RePlan" link field shows the whole URL as its text; a link custom
  field in OpenProject 17.6 has no label of its own.
- A closed work package still shows its full remaining work and 0%
  complete, because RePlan does not push remaining work to OpenProject.

Not rechecked: the dark themes. The colours have been Primer variables since
2026-09-25, and changing the review user's theme would be a write to the live
instance.
