# OpenProject RePlan plugin UI audit 2026-09-25

Status: dated record (2026-09-25). Index: [OpenProject design index](openproject-design-index.md).

Original status: agent audit; input to `task.self.design-openproject-plugin-gantt-view`.

Follows the review of the same day
([`openproject-plugin-review-2026-09-25.md`](openproject-plugin-review-2026-09-25.md))
and the permission fix that made the work-package panel reachable. Everything
here was seen in the live OpenProject with Playwright as the read-only
non-admin review user, at 1600 and 900 pixels wide.

## Measurements

```text
page                      height     DOM nodes  HTML   notes
project page, 1600 px     26 427 px  3 013      406 KB 28 screens of scrolling
project page, 900 px      33 358 px  3 013      406 KB horizontal overflow
work-package panel        ~1 050 px  85-91      -      fits about one screen
panel, unknown package    one line   8          -      HTTP 200, not 404
load time (both pages)    about 2 s through the protected connection, networkidle
console errors            6 x 404 from Gravatar avatars, OpenProject's, not the plugin's
```

## Project page, top to bottom

1. Header. "RePlan overlay" with week, status date and generation time; a line
   "RePlan artifacts: index.html gantt.html raw/…json" that names files the
   operator cannot open from here: plain text, not links.
2. Six counters: linked tasks 84/84, active tasks, planned resources, resource
   demands (287), warnings, open proposals. Only warnings and proposals ask
   for action; 287 demands is a number nobody acts on, and nothing says how
   loaded anyone is this week.
3. Resource summary: one card per resource with the technical id as the title,
   class, unit, task count, "max intensity", and planned, actual and remaining
   sums over the whole project history. At 900 px the ids are cut off
   ("resource.human.mainta…"). There is no time in these cards: they cannot
   show whether a resource is overloaded now, next week or never.
4. Current attention: a clean table of open tasks with status, demand count
   and signals. The most useful block on the page, and the one closest to
   what the operator does next.
5. Task resource timeline, 84 tasks, open first, then all accepted ones, 26
   screens. Per task: a grey track with a small blue bar, then three to five
   resource rows, each a text block and a green strip.
   - The track has irregular vertical lines but no dates, no months or weeks,
     no status-date line. A two-day task is a dot.
   - Every resource strip spans the full width, whatever the task window, so
     it never lines up with the task bar above it.
   - The strip is about ten pixels high and its fill height is the intensity,
     so 10% and 60% look nearly the same; 140% turns the strip red and is
     otherwise clipped. Agent intensities against a 40-hour week read 1-20%
     and carry no meaning.
   - The text beside each strip repeats role, unit, planned, remaining and
     intensity as a sentence; this text is most of the page height.

## Work-package panel

- Header links back to the project page and to the work package; status,
  window, actuals and evidence counts as pills; five counters.
- "Task window": the same dateless track with one bar.
- "Resources": per demand a text block with pills (planned, actual,
  remaining, evidence) and the same full-width strip.
- Evidence and actuals appear only as counts. The panel of an accepted task
  cannot say what was delivered or when; the summaries are in RePlan, not here.
- There is no link to the panel from the OpenProject work package itself; it
  is reachable only from the RePlan project page.
- Unknown package: a clear warning with a missing space after the bold
  sentence, served as HTTP 200.

## Cross-cutting

- Colours are fixed light values inside the templates. The operator reads
  OpenProject through the Dark Reader extension; values taken from
  OpenProject's own CSS variables would recolour consistently with the rest
  of the page, and with any OpenProject theme later.
- Ids everywhere where names exist: resource ids in cards and rows, task ids
  under every title.
- At 900 px a sidebar glyph overlaps the page title and the page scrolls
  sideways.
- Tests: the scaffold check greps source, the runtime check boots the plugin
  and loads the overlay, but nothing renders either page. The permission
  defect lived for six weeks because no check requested a page as a member.

## Assessment

```text
criterion                               score  note
works for the operator                  3/5    project page yes; panel only since today
shows what the operator asked for       1/5    no Gantt, no windows, no load profile
readable at a glance                    1/5    28 screens, text-first, unreadable strips
fits OpenProject visually               3/5    tables and cards look native; fixed colours
data behind the view                    4/5    overlay already has windows, intensity,
                                               capacity, tracking; lacks load profile
                                               and names
tested as a user would use it           1/5    no rendered page in any check
```

The plugin is a correct data list inside OpenProject with decorative bars. The
foundation, a read-only page, a panel, a permission, an overlay with most of
the data, is sound; the presentation is where the work is.

## Improvements

Quick, inside the current pages, no design decision needed:

1. Names instead of ids for resources and tasks; ids in tooltips. Needs the
   resource name in the overlay.
2. A date axis on every track: month and week ticks and the status-date line,
   the dates of the project span shown at the ends.
3. Resource strips placed by their participation window (`start_offset_days`,
   `duration_days` are already in the overlay), so they line up with the task
   bar.
4. Taller strips with a visible 100% line and room above it, so height reads
   as intensity and overload shows as a hump, not only a colour.
5. The per-demand sentence behind a disclosure; the strip is the default.
6. Accepted tasks grouped under a collapsed "Done" section; still visible, as
   the operator asked, but not 26 screens by default.
7. Colours from OpenProject variables; no fixed hex values.
8. Artifact line: links, or remove it.
9. Panel: list the evidence summaries and the actuals with dates, not counts;
   HTTP 404 for an unknown package; the missing space.
10. A rendered-page check in the runtime test, as a non-admin member.

For the Gantt design:

- a per-resource load profile over weeks with the 100% waterline, for every
  resource a task uses (the operator's answer 2), filterable later;
- counters that answer "who is overloaded, when" instead of totals;
- the panel reachable from the work package itself.

## Progress

- 2026-09-25: improvement 10 done by `task.self.add-plugin-rendered-page-checks`;
  the throwaway runtime test now requests both pages over HTTP as a member, an
  outsider and a signed-out visitor, 47 checks. The 404 for an unknown package
  from improvement 9 came with it. See
  [`openproject-plugin-throwaway-runtime-test.md`](openproject-plugin-throwaway-runtime-test.md).
- 2026-09-25: improvement 7 and the 900-pixel defects done by
  `task.self.theme-plugin-views-with-openproject-variables`. Every plugin
  colour is an OpenProject Primer variable, both pages use OpenProject's page
  header with breadcrumbs, so the sidebar toggle no longer covers the title,
  and long ids wrap. Screenshots at 1600 and 900 pixels in OpenProject's light
  theme, its dark theme and under the Dark Reader engine: no horizontal
  scroll, no clipped element, readable text in all three. Before, the dark
  theme left the headings dark grey on black and the cards white.
- 2026-09-26: improvements 1, 5, 6 and 8 and the counters done by
  `task.self.restructure-plugin-project-page`. The project page opens with
  five counters that answer questions, "Needs attention" lists open tasks by
  finish date with names, windows, resources and signals in words, resources
  and lanes show names with ids in tooltips, amounts sit behind a disclosure,
  accepted tasks under a collapsed "Done (n)", and the artifact line is gone.
  The page is 4.4 screens at 1600 pixels instead of 28. Improvements 2 to 4,
  the axis and the placed lanes, belong to the Gantt page.
