# Lead time and queue order

Date: 2026-11-08

Status: proposed design, for the maintainer to discuss before any model
change.

## Purpose

The check whether RePlan can draft a consumer plan's dates
(`taskjuggler-run-import-experiment.md`, "Drafting a real plan's dates")
found two scheduling rules RePlan cannot express. This document proposes how
the canonical model, `validate`, `workload` and the TaskJuggler export would
express each, compares the options with TaskJuggler's own attributes, and
lists the implementation tasks it would take.

The two rules:

- **Lead time without load.** A task takes a number of working days but does
  not consume levelled capacity: waiting for a supplier or a review, or a
  person's decision whose hours the plan does not level. Today every hour
  demand is levelled, so the owner's decisions queue behind each other in
  TaskJuggler, and a wait can only be faked as hours of someone's time.
- **Queue order within a resource.** The consumer's scheduler runs each
  resource's tasks strictly in the order of its list. TaskJuggler's ASAP
  scheduling orders them by priority and path criticality instead, so the
  same tasks can come out in another order.

## What TaskJuggler offers

Checked with tj3 3.8 on 2026-11-08:

| Attribute | Meaning | Observed |
| --- | --- | --- |
| `effort 6h` + `allocate r` | work booked on the resource, levelled against its hours and limits | the only form the export writes today |
| `length 3d` | working days of the project calendar, no resource needed | Monday start, ends Wednesday |
| `duration 3d` | calendar days, weekends included | Monday start, ends Thursday |
| `length 1d` + `allocate r` | books the resource for every working hour of that day | a 1-day decision consumed 4 h of the owner, so it is levelled after all |
| `priority 900` | ASAP order among tasks that compete for a resource (default 500) | a later task with 900 ran before two earlier ones with 500 |
| `depends` | finish-to-start | can force strict order, at the price of idle capacity |

A task has exactly one of `effort`, `length`, `duration` or `milestone`
(tj3: "This duration criteria is overwriting a previously provided
criteria"). A wait followed by work therefore needs two TaskJuggler tasks.

## Lead time without load

### Options

1. **Task lead time.** A task field `lead_time` with `working_days` (or
   `calendar_days`) and an optional `reason`. The task occupies that span on
   the plan; it consumes no capacity.
2. **Non-levelled demand.** A demand flag `levelled: false`: the hours are
   recorded (and tracked as actuals) but neither workload overload nor the
   export levels them.
3. **A calendar pseudo-resource** with unlimited capacity that every wait is
   allocated to. It needs no model change, but it hides the meaning in
   resource data, distorts workload and dashboards, and still has no unit
   for "days".

Option 3 is rejected. Options 1 and 2 answer different halves of the rule
and are proposed together:

- a supplier wait is a task with `lead_time` and no levelled demands;
- an owner's decision is a task with `lead_time` (how long it takes to be
  made) and a non-levelled demand (the owner's hours, kept for tracking and
  calibration).

### Proposed model

```yaml
tasks:
  - id: task.approve-design
    title: Owner approves the design
    lead_time:
      working_days: 2
      reason: owner decision, not levelled
    resource_demands:
      - kind: resource_demand
        id: resource_demand.approve-design.owner

resource_demands:
  - id: resource_demand.approve-design.owner
    task_ref: { kind: task, id: task.approve-design }
    resource_ref: { kind: resource, id: resource.human.owner }
    unit: hours
    planned_amount: 1
    levelled: false
```

Rules:

- `lead_time.working_days` and `calendar_days` are positive numbers, at most
  one of them; working days follow the project calendar, Monday to Friday
  unless a project calendar is added later;
- a task with `lead_time` and levelled demands means "wait, then work": the
  wait comes first;
- `levelled` defaults to true, so every existing plan keeps its meaning.

### Effects

- **validate:** refuse both `working_days` and `calendar_days`, a value of 0
  or less, and `levelled: false` on a non-hour unit (slots and budgets are
  never levelled in time anyway). Warn when a task with `lead_time` has
  planned dates shorter than the lead time.
- **workload and dashboard:** non-levelled hours are reported in their own
  column and never count toward overload; lead time adds no load. Weekly
  reports show the task as in flight across its span.
- **TaskJuggler export:**
  - a lead-time task without levelled demands becomes `length Nd` (or
    `duration Nd`) with no `allocate`;
  - a lead-time task with levelled demands becomes a container with two
    children, `wait` (`length`) and `work` (`effort` + `allocate`,
    `depends !wait`); the mapping records the container, and the importer
    takes the container's start and end;
  - non-levelled demands are left out of `allocate` and `effort`, and named
    in a task comment so the `.tjp` still explains them.
- **OpenProject:** no new field; the dates show the span, and the reason can
  go into the description by hand.
- **tracking:** actuals on a non-levelled demand work as today, so
  estimate calibration still sees the owner's hours.

## Queue order within a resource

### Options

1. **Task priority.** A task field `priority` (1 to 1000, default 500)
   exported as TaskJuggler `priority`. It is soft: a blocked high-priority
   task lets lower ones use the idle capacity, which is usually what people
   want.
2. **Strict queue.** A resource setting `queue: strict` plus an order (an
   explicit `queue_rank` on tasks, or the order of `tasks.yaml`). The export
   chains each strict resource's open tasks with synthetic `depends`, so
   none starts before the previous one ends, even when capacity is idle.
3. **Derived priority.** Write `priority` from the order of `tasks.yaml`
   with no model change. It is cheap, but it gives meaning to the file
   order, which `project format` and imports do not preserve as a promise
   today.

### Proposed model

Option 1 now, option 2 only if a consumer needs the strict rule after trying
priorities:

```yaml
tasks:
  - id: task.migrate-schema
    priority: 800

resources:
  - id: resource.ai.agent.implementer
    queue: strict        # option 2, later
```

### Effects

- **validate:** `priority` between 1 and 1000. For a strict queue, warn when
  the queue order contradicts a dependency (the export's existing cycle
  check, `taskjuggler_dependency_cycle`, would refuse the export).
- **workload, `next`, dashboard:** `next` and the dashboard's next actions
  sort by priority after the person-waits-first rule; workload is unchanged
  because priority moves work, not load.
- **TaskJuggler export:** `priority N` per task; for a strict queue,
  synthetic `depends` between consecutive open tasks whose only levelled
  resource is that resource (a task with several levelled resources is left
  out of the chain, with a count warning).
- **OpenProject:** OpenProject has its own priority (Low, Normal, High,
  Immediate). The import could map it to 300, 500, 700 and 900 and the
  reviewed helpers could push it back; both are separate decisions.

## How close this gets to the consumer's scheduler

The comparison on 2026-11-08 left the 23 open tasks between 28 days earlier
and 42 days later than the consumer's script (median 6 days earlier). That
comparison did not split the difference by cause; task 4 below reruns it
with lead time and priority to measure what is left. Lead time with
non-levelled owner hours addresses owner decisions queueing behind each
other; priority, and a strict queue if needed, address reordering. The
script's "estimate times a tempo plus a fixed review
share" is already a calibration factor in RePlan and needs nothing new.

## Proposed implementation tasks

1. Canonical model and validate: `lead_time`, `levelled`, `priority`, with
   format, migrate and the OpenProject sample unchanged for plans that do
   not use them. About 2 agent hours.
2. Workload, dashboard and `next`: non-levelled column, lead-time spans,
   priority sort. About 2 agent hours.
3. TaskJuggler export and import: `length`/`duration`, wait-then-work
   containers and their mapping, non-levelled demands left out, `priority`.
   About 2.5 agent hours.
4. Rerun the consumer comparison on a copy and write down the remaining
   difference. About 1 agent hour.
5. Later, only on a consumer's request: strict queues (about 1.5 agent
   hours) and the OpenProject priority mapping (about 1 agent hour).

## Questions for the maintainer

- Lead time on the task, or as a demand of a new "time" unit? The task field
  is proposed because a wait has no resource.
- Is `levelled: false` the right word, or should it be `load: none` to match
  `DemandLoads`, which already means "counts toward capacity" for statuses?
- Start with soft priority only, and leave strict queues until someone needs
  them?
