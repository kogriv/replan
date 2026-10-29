# Dashboard load profile semantics design

Status: accepted design for `task.self.reconcile-dashboard-load-semantics`.

Decision date: 2026-08-06.

## Purpose

The current generated RePlan Gantt can show resource rows under tasks and a
resource load summary below the task timeline. The existing visualization is
useful, but its daily bars can be read too literally.

The authoritative MVP planning model is still period-based. In the current
dogfood project that usually means weekly capacity and task-window resource
demands. RePlan often knows:

```text
this task needs 8 hours of maintainer work across this task window
```

It usually does not yet know:

```text
the maintainer works exactly 2 hours on Monday, 1 hour on Tuesday,
4 hours on Wednesday and 1 hour on Thursday
```

The dashboard must therefore avoid visual language that implies exact daily
execution timing when the data only supports a task-window or week-level
allocation.

## Core Visual Rule

Use different visual dimensions for different meanings.

```text
horizontal length = when the resource participates
vertical height   = intensity or utilization
```

Do not encode resource amount by shortening the bar unless the resource demand
has its own explicit timing window.

## Task Rows

The task bar continues to mean task duration.

```text
Task A        |====================|  task window
```

A resource row under the task means that the resource participates in that
task. If the resource demand has no explicit timing, the resource row inherits
the whole task window.

```text
Task A        |====================|  task window
maintainer    |--------------------|  participates for the task window
codex         |--------------------|  participates for the task window
```

The visual height inside the resource row means intensity relative to the
available capacity for the same time window.

```text
Task A        |====================|
maintainer    |______##############|  70 percent intensity
codex         |___________#########|  45 percent intensity
```

The empty upper part is not idle time at the end of the task. It is unused
capacity within the row height.

If a resource demand has explicit timing, then and only then the resource bar
may be shorter than the task bar.

```text
Task A        |====================|  task window
codex         |    ##########      |  explicit resource window
```

This means:

```text
length = participation window
height = intensity
```

It does not mean:

```text
length = planned amount
```

## Task-row Height Calculation

For the first implementation, task-level resource row intensity should be a
deterministic approximation.

Inputs:

- resource demand planned amount;
- demand unit;
- task or demand window;
- resource capacity for the matching unit and period, when available.

Suggested first calculation:

```text
demand_window_days = inclusive days in the inherited or explicit demand window
demand_window_weeks = max(1, demand_window_days / 7)
period_capacity = matching weekly capacity for that resource and unit
available_capacity_for_window = period_capacity * demand_window_weeks
intensity = planned_amount / available_capacity_for_window
```

Clamp only the displayed task-row fill height, not the underlying value:

```text
visible task fill height = min(intensity, 100 percent)
tooltip/details intensity = actual intensity, including values above 100 percent
```

If no matching capacity exists, render a neutral minimum-height bar and mark
the value as capacity unknown in details or tooltip.

## Resource Summary

The resource summary is resource-centric, not task-centric.

It should show the summed load profile for each resource across the timeline.

```text
Resource Load

maintainer
150% |        ##
125% |      ####
100% |------####--------- waterline
 75% |    #######
 50% |  #########
 25% | ###########
  0% +-------------------

codex
150% |
125% |
100% |------------------- waterline
 75% |     ####
 50% |   ########
 25% | ###########
  0% +-------------------
```

The 100 percent waterline must remain visible even when load goes above it.

Overload must be allowed to extend above the 100 percent line. Do not cap the
visual at 100 percent, because capping hides the exact situation the operator
needs to see.

## Summary Bucket Rule

The summary timeline should use the authoritative planning bucket.

For the MVP self-dogfood project, that means weekly buckets.

```text
W34       W35       W36       W37
[ 47% ]   [ 33% ]   [  0% ]   [ 80% ]
```

The implementation may render buckets as adjacent vertical profile segments or
as a continuous stepped profile. The important rule is that the visual is
bucket-based, not fake exact daily data.

Daily-looking interpolation may be used only as a visual smoothing layer if the
legend and raw fields clearly say that the authoritative bucket is weekly.

## Text and Details

Numbers should be secondary.

The default view should prioritize the graph:

- task duration bars;
- resource participation bars;
- resource intensity height;
- resource summary profile;
- 100 percent waterline;
- overload above waterline.

Exact numeric details should be available through:

- `title` or accessible tooltip text;
- expandable details later;
- raw JSON;
- labels only where the row has enough room.

Do not replace the graph with a mostly textual summary.

## JSON Contract

The report model should expose enough fields for both the static dashboard and
future OpenProject plugin work.

Task-level allocation fields:

- demand id;
- resource id;
- resource class;
- unit;
- planned amount;
- remaining amount;
- inherited or explicit start date;
- inherited or explicit finish date;
- participation window source: `task_window` or `explicit_demand_window`;
- capacity for the window when known;
- intensity ratio;
- intensity percent;
- over-capacity flag.

Resource summary fields:

- resource id;
- resource name;
- unit;
- bucket type, initially `week`;
- bucket start date;
- bucket finish date;
- bucket capacity;
- planned load;
- actual load when available;
- remaining load when available;
- utilization ratio;
- overload amount;
- maximum utilization ratio for the rendered chart.

## HTML Contract

Task resource rows:

- bar starts at inherited or explicit demand start;
- bar ends at inherited or explicit demand finish;
- bar length does not represent planned amount;
- inner fill height represents intensity;
- full row height represents 100 percent capacity;
- over-capacity state is represented in details first, and may later get a
  visual overflow marker if needed.

Resource summary:

- each resource row has a fixed visible graph height;
- 100 percent waterline is drawn across the graph;
- load profile can extend above waterline;
- chart scale expands when overload is present;
- text values are in tooltip/details/raw JSON, not the primary signal.

## Explicit Non-goals

Do not implement in this slice:

- manual daily allocation editing;
- drag-and-drop resource scheduling;
- exact time tracking;
- payroll or timesheets;
- full resource leveling;
- OpenProject plugin rendering;
- broad OpenProject write-back.

## Accepted Implementation Direction

The next implementation should replace the misleading daily bar semantics with
capacity-height profiles:

1. task-level resource rows keep participation length and encode intensity by
   fill height;
2. resource summary uses weekly bucket load profiles;
3. summary rows draw a visible 100 percent waterline;
4. overload is allowed to extend above the waterline;
5. raw JSON exposes the ratios and bucket values used by the HTML.


## Update 2026-09-25: Working Days, Slots And Weekly Capacity

`task.self.honest-weekly-workload-for-multi-week-and-concurrent-demands`
(GAP-031) changed three things behind the profile, with the same rule as
`replan workload` (see the workload section of `docs/spec/cli.md`):

1. daily amounts fall on the resource's working days only; a Monday-to-Friday
   person has no load on weekends, and a task's amount per day is its amount
   divided by its working days;
2. a slot resource (`period: concurrent` capacity) has `concurrent: true` on
   its load, its daily amount is the slots in use, and a weekly bucket holds
   the peak day, not the sum;
3. each weekly bucket's capacity is the capacity in effect in that week,
   prorated by working days for a partial week at the edge of the range. Before
   this, every bucket used the status week's capacity, so a capacity change
   was invisible in the profile.
