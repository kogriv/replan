# RePlan TaskJuggler capacity and quota mapping

Date: 2026-07-12

Status: approved implementation design.

## Purpose

This document defines how RePlan should handle capacity and quota assumptions
when exporting a RePlan workspace or scenario to TaskJuggler.

It answers:

- which RePlan capacity concepts can be represented in TaskJuggler;
- which capacity concepts must stay in RePlan workload reports;
- how scenario changes to `capacities.<capacity_id>.amount` should behave in
  `replan export taskjuggler`;
- when TaskJuggler export should warn instead of silently dropping information;
- what must not be implemented until the mapping is proven safe.

This document depends on:

- `docs/architecture/taskjuggler-integration.md`;
- `docs/spec/capacity-availability-rules.md`;
- `docs/spec/workload-calculation.md`;
- `docs/spec/scenario-model.md`;
- `docs/spec/cli.md`.

External reference:

- TaskJuggler user manual, version 3.7.2:
  `https://taskjuggler.org/manual/toc.html`;
- TaskJuggler `resource` property:
  `https://taskjuggler.org/tj3/manual/resource.html`;
- TaskJuggler `vacation` property:
  `https://taskjuggler.org/manual/vacation.html`;
- TaskJuggler `limits` property:
  `https://taskjuggler.org/tj3/manual/limits.html`.

## Core decision

TaskJuggler export must be loss-aware.

Approved rule:

```text
RePlan capacity assumption -> TaskJuggler mapping only when semantics are safe
RePlan capacity assumption -> warning when semantics are not represented
RePlan capacity assumption -> workload report remains authoritative
```

TaskJuggler is an optional scheduling engine.

RePlan remains the source of truth for resource abstractions, scenario
assumptions, capacity comparison, quota comparison, workload warnings and review
workflow.

## Why this is necessary

RePlan capacity is broader than TaskJuggler resource scheduling.

RePlan has first-class capacity types such as:

- periodic human hours;
- AI-agent execution capacity;
- review capacity;
- service quota;
- rate limit;
- concurrent environment slots;
- stock;
- budget;
- approval capacity.

TaskJuggler is strong for task scheduling, resources, allocations, calendars,
working time, leaves, per-interval allocation limits and generated reports. It
is not a universal quota, subscription, budget, rate-limit and operational
resource ledger.

Therefore, RePlan must not imply that every RePlan capacity assumption is
preserved when generating `.tjp`.

## Current implemented behavior

Current `replan export taskjuggler` supports:

- RePlan resources as TaskJuggler resources;
- RePlan tasks as TaskJuggler tasks;
- concrete hour-based resource demands as TaskJuggler `effort`;
- concrete resource assignments as TaskJuggler `allocate`;
- scenario changes to `resource_demands.<id>.remaining_amount`;
- scenario changes to `resource_demands.<id>.planned_amount`;
- scenario changes to `resource_demands.<id>.resource_ref`;
- scenario changes to `tasks.<id>.planned_start` and
  `tasks.<id>.planned_finish` in the shared workspace overlay;
- scenario `add availability.exceptions` for unavailable dates, exported as
  TaskJuggler `vacation`.

Current `replan workload --scenario <id>` supports capacity amount changes:

```text
capacities.<capacity_id>.amount
```

This includes human capacity and service quota capacity.

Current `replan export taskjuggler --scenario <id>` applies the scenario overlay
in memory. Task date changes therefore reach every date-aware export input;
the current `.tjp` writer still lets TaskJuggler calculate task dates rather
than emitting canonical planned dates. Capacity amount changes are not yet
represented in `.tjp`.

Approved immediate behavior:

- do not silently pretend that capacity changes are represented;
- emit an explicit warning for each scenario capacity amount change that is not
  represented in `.tjp`;
- keep generating the `.tjp` when the rest of the scenario is exportable;
- keep workload CSV as the correct export for capacity and quota comparison.

## Mapping classes

### Class A: already represented

Already represented in the first TaskJuggler export slices:

```text
availability unavailable date -> TaskJuggler vacation date
remaining hour demand -> TaskJuggler effort
planned hour demand -> TaskJuggler effort fallback
concrete resource assignment -> TaskJuggler allocate
```

These are safe because the generated `.tjp` carries a direct scheduling
meaning close to the RePlan meaning.

### Class B: narrow implemented capacity limit mapping

Implemented narrow mapping:

```text
periodic human hours per week -> TaskJuggler limits weeklymax
periodic AI-agent hours per week -> TaskJuggler limits weeklymax
```

Supported RePlan capacity shape:

```text
capacity.type == periodic_capacity
capacity.resource_class in [human, ai_agent]
capacity.unit in [hour, hours, h]
capacity.period == week
```

Generated TaskJuggler representation:

```text
resource resource_human_ivan "Ivan" {
  limits { weeklymax 6.00h }
}
```

Rules:

- multiple matching weekly hour capacities for the same resource are summed;
- zero capacity is still represented as `weeklymax 0.00h`;
- the mapping is emitted on the resource block;
- a scenario change to such a capacity is considered represented in `.tjp`;
- `taskjuggler_capacity_change_not_represented` must not be emitted for this
  narrow mapped case.

Still not implemented:

```text
periodic human hours per day
periodic human hours per month
periodic AI-agent hours per day
periodic AI-agent hours per month
review_hours
agent_hours
effective date ranges
availability-window to shift conversion
reduced-capacity exception to limit conversion
```

These remain deferred because the mapping must decide:

- whether daily/monthly capacity should become `dailymax`, `monthlymax`,
  working hours, explicit shifts or generated leaves;
- how effective date ranges map to TaskJuggler intervals;
- how RePlan availability windows combine with limits;
- how review bottlenecks differ from ordinary schedulable work;
- how to avoid generating a syntactically valid but semantically misleading
  schedule.

### Class C: not TaskJuggler-native for MVP

Do not map to `.tjp` in the MVP:

```text
service quota
API token quota
API request quota
rate limit
subscription entitlement
budget
stock
material amount
procurement limit
cloud spend cap
```

These should stay in RePlan workload, shortage, variance and report outputs.

Reason:

- TaskJuggler can schedule work performed by resources;
- quota and budget resources often constrain consumption totals, resets,
  subscriptions, billing periods or external provider limits;
- mapping them as TaskJuggler people or generic resources would produce a fake
  schedule rather than a reliable capacity analysis.

### Class D: possible future external-engine input

Some non-human capacities may later be exported to a different solver or a
custom RePlan scheduler:

```text
concurrent environment slots
machine slots
robot work cells
limited equipment
vehicle availability
rate-limited batch windows
```

Do not force these into TaskJuggler until there is a tested model.

## Scenario capacity changes

Scenario capacity changes use the path:

```text
capacities.<capacity_id>.amount
```

Example:

```yaml
changes:
  - op: set
    path: capacities.capacity.human-ivan.hours.weekly.amount
    value: 6
```

Example:

```yaml
changes:
  - op: set
    path: capacities.capacity.openai.tokens.monthly.amount
    value: 300000
```

Required behavior for `replan workload --scenario <id>`:

- apply the scenario capacity amount change in memory;
- recalculate workload and overload;
- do not mutate canonical files.

Required behavior for `replan export csv --report workload --scenario <id>`:

- apply the scenario capacity amount change in memory;
- export workload rows with changed capacity, overload and shortage;
- include scenario id and manifest metadata when writing files.

Required behavior for `replan export taskjuggler --scenario <id>` in the
current implementation stage:

- apply the scenario overlay in memory so other supported changes are visible;
- do not represent capacity amount changes in `.tjp` yet;
- emit warning `taskjuggler_capacity_change_not_represented`;
- identify the scenario id, capacity id, capacity type, resource id, unit,
  period and amount in the warning message when known;
- keep generating `.tjp` unless the scenario has an unsupported operation,
  invalid value or unknown referenced object.

## Warning contract

Warning code:

```text
taskjuggler_capacity_change_not_represented
```

Severity:

```text
warning
```

Target:

```text
kind: capacity
id: <capacity_id>
```

Meaning:

```text
The selected scenario changes a RePlan capacity amount. The change is applied
for RePlan workload calculations, but this TaskJuggler export does not encode
that capacity change in generated .tjp content.
```

By default this diagnostic is a warning.

Reason:

- the scenario may still have useful TaskJuggler-exportable changes;
- the workload report remains the correct output for capacity and quota review;
- blocking `.tjp` generation would make mixed scenarios less useful.

Strict policy is available when an operator or AI CLI agent needs to guarantee
that no known scenario assumption is silently absent from `.tjp`:

```bash
replan export taskjuggler --scenario <id> --unrepresented-assumptions error
```

Supported modes:

```text
warn  -> emit warning and keep generating `.tjp`
error -> promote the diagnostic to an error and do not generate successful `.tjp`
```

Default mode:

```text
warn
```

The flag name uses `assumptions`, not `capacity`, because future loss-aware
diagnostics may cover assumptions beyond capacity amount.

## Human and AI-agent weekly capacity mapping

Implemented subset:

```text
capacity.type == periodic_capacity
capacity.resource_class in [human, ai_agent]
capacity.unit in [hour, hours, h]
capacity.period == week
```

TaskJuggler representation:

```text
week  -> limits { weeklymax <amount>h }
```

The first implementation treats `weeklymax` as a scheduling allocation cap for
the resource. This is narrow by design: it is useful for experiments and strict
AI CLI workflows, but it is not a claim of full TaskJuggler capacity parity.

Open questions before expanding this candidate:

- Does `weeklymax` express available supply or only maximum allocation?
- Should day and month use `dailymax` and `monthlymax`, or should they become
  generated working hours and shifts?
- How should effective date ranges be represented?
- How should RePlan availability windows combine with limits?
- How should review hours be represented when the actual task is not a
  scheduling task but an approval bottleneck?
- How should multiple capacities for the same resource be merged?

Until these questions are answered, TaskJuggler export should warn, not encode
broader capacity classes.

## Quota and rate-limit handling

Do not map these RePlan capacities into `.tjp` in MVP:

- token quota;
- request quota;
- CI minutes;
- provider credits;
- SaaS seat entitlement;
- rate limit;
- cloud budget;
- procurement budget.

Use these outputs instead:

```bash
replan workload --project examples/small-team --week 2026-W28 --scenario scenario.openai-quota-300k --format json
replan export csv --project examples/small-team --report workload --week 2026-W28 --scenario scenario.openai-quota-300k --output exports/workload/2026-W28/scenario.openai-quota-300k.csv --manifest exports/workload/2026-W28/scenario.openai-quota-300k.manifest.json
```

The `.tjp` export may still be useful for human-time scheduling, but it is not
the complete capacity picture.

## AI CLI workflow

An AI CLI agent reviewing a scenario should prefer this sequence:

```bash
replan scenario show scenario.openai-quota-300k --project examples/small-team --format json
replan workload --project examples/small-team --week 2026-W28 --scenario scenario.openai-quota-300k --format json
replan export taskjuggler --project examples/small-team --scenario scenario.openai-quota-300k --format json
```

Interpretation:

- workload output answers capacity and quota fit;
- TaskJuggler output answers schedulable task/resource export;
- TaskJuggler warnings identify assumptions that were not encoded in `.tjp`.

An AI agent must not treat "TaskJuggler export succeeded" as "all RePlan
capacity assumptions were represented".

## Done criteria for the current slice

This design slice is done when:

- this document exists;
- `docs/spec/cli.md` references this mapping boundary;
- `docs/spec/implementation-sequence.md` moves the design item from future
  work into implemented documentation;
- `replan export taskjuggler --scenario <id>` warns when scenario capacity
  changes are not represented in `.tjp`;
- tests cover at least one human capacity scenario and one service quota
  scenario.
- `--unrepresented-assumptions error` can promote unrepresented capacity
  diagnostics to errors.
- narrow weekly hour capacity mapping emits TaskJuggler `limits { weeklymax
  <amount>h }` for human and AI-agent resources.
- optional TaskJuggler runtime validation is available through
  `replan dev validate-taskjuggler`;
- validation skips successfully when `tj3` is not installed.

## Future implementation slices

Candidate next slices:

1. Expand narrow `limits` mapping only after validation:
   - daily hour capacity;
   - monthly hour capacity;
   - effective date ranges;
   - availability windows;
   - reduced-capacity exceptions.
2. Add a TaskJuggler run/import experiment as a scenario, not as canonical
   truth. The design is documented in
   `docs/spec/taskjuggler-run-import-experiment.md`.
3. Keep quota, budget and rate-limit reporting in RePlan workload and reports
   unless a different solver is explicitly introduced.
