# Workload calculation rules

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how RePlan calculates workload, capacity fit, overload,
underuse, shortages, conflicts, forecast and warnings.

It answers:

- how remaining work is spread across dates;
- what happens when a task has no dates;
- what happens when a demand has no estimate;
- how overload is calculated;
- how underuse is calculated;
- how non-hour resources are calculated;
- how reservations and actuals affect calculation;
- which warnings must be generated;
- what belongs in MVP and what is explicitly out of scope.

This document is functional design. The exact canonical schema will be defined
later in `docs/spec/canonical-model.md`.

## Core decision

Workload calculation in MVP is deterministic rule-based reporting.

It is not an automatic optimizer.

Approved rule:

```text
workload calculation distributes resource demands into time buckets, compares
them with capacity, incorporates actuals, calculates forecast and variance
where possible, and emits warnings.
```

It must not silently change the plan.

It must not automatically level resources.

It must explain what it calculated and why warnings exist.

## Calculation model

The calculation model is:

```text
resource demands
  -> calculation ranges
  -> allocation buckets
  -> capacity comparison
  -> actuals and forecast
  -> warnings
  -> reports
```

Short version:

```text
demand -> bucketed load -> compare with capacity -> warnings/report
```

Conceptual formulas:

```text
workload = planned/reserved/remaining demands spread across time buckets
capacity = available supply in the same buckets
actuals = consumed resources attached to task/resource/demand
forecast = actual + remaining
warnings = rule-based findings
```

## Inputs

Workload calculation uses:

- tasks;
- resource demands;
- demand timing;
- planned amount;
- remaining amount;
- reserved amount;
- actual amount;
- resource capacity;
- availability windows;
- capacity exceptions;
- reservations or allocations;
- actual consumption records;
- baseline snapshots;
- current plan;
- scenarios;
- task tracking status;
- evidence and proposals when relevant to warnings;
- `status_date` for time-bounded reports.

## Outputs

Workload calculation should produce:

- bucketed load by resource, unit and period;
- bucketed capacity by resource, unit and period;
- overload;
- underuse;
- shortage;
- reservation conflicts;
- unknown capacity warnings;
- missing estimate warnings;
- missing date warnings;
- forecast totals;
- variance values where baseline or comparison data exists;
- machine-readable warning records;
- human-readable report sections.

## Time buckets

RePlan should calculate workload using buckets.

Approved bucket types:

```text
day
week
month
window
instant
unscheduled
```

### day

Primary bucket for effort-like resources.

Examples:

- human hours;
- AI-agent hours;
- equipment hours;
- vehicle hours;
- room hours.

### week

Useful for rough planning when daily detail is unavailable.

Examples:

- weekly contractor capacity;
- weekly supplier delivery capacity;
- weekly review capacity.

### month

Useful for budget, subscriptions and monthly quotas.

Examples:

- monthly API token quota;
- monthly SaaS credits;
- monthly project budget.

### window

Useful when exact time interval matters.

Examples:

- staging environment from 10:00 to 13:00;
- site access from 09:00 to 18:00;
- meeting room reservation;
- maintenance window.

### instant

Useful for milestone-like events.

Examples:

- approval;
- delivery;
- release;
- inspection acceptance.

### unscheduled

Used when RePlan has a demand but no usable date range.

The `unscheduled` bucket prevents RePlan from pretending the demand does not
exist.

## MVP bucket support

MVP should support:

```text
day for effort-like resources
month or named period for quota and budget resources
window for concurrent resources
unscheduled for missing dates
```

Weekly buckets can be derived or added soon after MVP if useful.

## Calculation range selection

Each demand needs a calculation range.

Priority order:

1. explicit demand timing;
2. task current planned dates;
3. task baseline dates when calculating baseline mode;
4. scenario dates when calculating scenario mode;
5. fallback to `unscheduled`.

If no usable range exists, RePlan must not invent dates silently.

It should place the demand into `unscheduled` and emit:

```text
required demand has no scheduled date range
```

For optional demands, the warning severity may be lower.

## Spread modes

Remaining work is distributed using demand timing spread mode.

Approved MVP spread modes:

```text
even
contiguous
manual
```

Future spread modes:

```text
front_loaded
back_loaded
milestone
solver
```

Future modes are intentionally not required for MVP.

## even spread

`even` spreads remaining amount evenly over eligible buckets.

Example:

```text
remaining amount: 12h
date range: 2026-07-06..2026-07-08
eligible working days: 3
load: 4h/day
```

If one day has zero capacity, it should not receive load unless policy allows
over-capacity scheduling.

Example:

```text
remaining amount: 12h
date range: 2026-07-06..2026-07-08
eligible days with positive capacity: 2
load: 6h/day
warning: range includes unavailable day
```

## contiguous spread

`contiguous` means the resource is needed in an uninterrupted block.

Examples:

- staging environment for 3 continuous hours;
- site access for one full morning;
- vehicle for a delivery route;
- meeting room for a meeting.

For MVP, RePlan may not automatically find the optimal slot.

It should:

- use explicit window if provided;
- detect conflicts if window overlaps existing reservations;
- warn if no window is provided.

Warning:

```text
contiguous demand has no explicit window
```

## manual spread

`manual` means allocation is specified directly.

Example:

```yaml
allocations:
  - date: 2026-07-06
    amount: 6
    unit: hours
  - date: 2026-07-07
    amount: 2
    unit: hours
```

Manual allocations should be validated against capacity.

They should not be overwritten by automatic spreading.

## Tasks with no dates

If a required demand has no usable dates:

```text
place demand into unscheduled bucket
emit warning
do not assume zero load
do not silently pick today's date
```

Warning:

```text
required demand has no scheduled date range
```

Reports should show unscheduled required demand as planning debt.

Example:

```text
OP-123 Implement import adapter
  required implementer demand: 8h
  status: unscheduled
  warning: required demand has no scheduled date range
```

## Demands with no estimate

If a required demand has no `planned_amount` and no `remaining_amount`:

```text
do not assume zero
emit warning
```

Warning:

```text
required demand has no estimate
```

Unknown estimate is not the same as zero estimate.

For optional demands, the warning severity may be lower.

## Planned, reserved, remaining and actual

For each demand, RePlan tracks:

```text
planned_amount
reserved_amount
remaining_amount
actual_amount
forecast_total
```

Default forecast formula:

```text
forecast_total = actual_amount + remaining_amount
```

Default variance formula:

```text
variance = forecast_total - baseline_planned_amount
```

If `remaining_amount` is unknown:

```text
forecast_total = unknown
warning: remaining amount unknown
```

If `actual_amount > planned_amount`:

```text
warning: actual exceeds planned
```

If `reserved_amount < remaining_amount` for a required demand:

```text
warning: required remaining work not fully reserved
```

## Capacity comparison

For each resource, unit and bucket:

```text
available_capacity = base_capacity + exceptions - unavailable_capacity
load = sum(allocated demand amounts)
reserved_load = sum(reservations)
actual_load = sum(actuals)
```

MVP should distinguish:

```text
planned_load
reserved_load
remaining_load
actual_load
forecast_load
```

Reports may display a simpler view, but the calculation should not collapse the
concepts.

## Overload

Overload means planned or reserved load exceeds available capacity.

Default formula:

```text
overload = max(0, load - available_capacity)
```

Example:

```text
resource: dev_ivan
bucket: 2026-07-07
available: 8h
load: 10h
overload: 2h
```

When capacity is unknown, RePlan must not report "no overload".

It should report:

```text
capacity unknown
```

and emit a warning for required demand.

## Underuse

Underuse means available capacity exceeds planned or reserved load.

Default formula:

```text
underuse = max(0, available_capacity - load)
```

Example:

```text
resource: backend_reviewers
bucket: 2026-07-08
available: 12h
load: 3h
underuse: 9h
```

Underuse is not always a problem.

It is useful for:

- finding available capacity;
- comparing scenarios;
- choosing where to schedule unscheduled demand;
- understanding whether a resource pool is overprovisioned.

## Non-hour resources

Non-hour resources use the same general model:

```text
demand amount -> bucket -> compare with capacity -> warning if exceeded
```

But bucket type and formulas differ by resource behavior.

### Tokens and API quota

Typical bucket:

```text
day or month
```

Formula:

```text
forecast_total = actual_tokens + remaining_tokens
shortage = max(0, forecast_total - quota_capacity)
```

Warning:

```text
quota forecast exceeds quota
```

### Rate limits

Typical bucket:

```text
minute, hour or explicit window
```

Formula:

```text
rate_conflict = planned_burst - rate_limit
```

Warning:

```text
rate limit conflict
```

MVP may only support simple rate-limit warnings when planned burst is explicit.

### Budget

Typical bucket:

```text
month, project, scenario or explicit budget period
```

Formula:

```text
forecast_spend = actual_spend + remaining_budget_need
over_budget = max(0, forecast_spend - budget_capacity)
```

Warning:

```text
budget forecast exceeds budget
```

### Material and stock

Typical bucket:

```text
current stock or dated stock period
```

Formula:

```text
shortage = max(0, required_quantity - available_stock)
```

Warning:

```text
material shortage
```

### Concurrent slots

Typical bucket:

```text
window
```

Formula:

```text
concurrent_usage(window) <= concurrent_capacity
```

Warning:

```text
concurrent slot conflict
```

Example:

```text
staging_env capacity: 1
task A reservation: 10:00-13:00
task B reservation: 11:00-12:00
conflict: 2 overlapping reservations > 1 available slot
```

### Approvals and reviews

Typical bucket:

```text
day or week
```

Formula:

```text
review_load = sum(review demand)
review_capacity = reviewer approval capacity
```

Warning:

```text
review capacity exceeded
```

This is important because review is often an invisible bottleneck.

## Reservations and allocations

For MVP:

```text
allocation = calculated or planned placement of demand into buckets
reservation = stronger claim on capacity
```

MVP may treat reservations as allocations for calculation.

The model should still keep these views separate:

```text
calculated_load
reserved_load
actual_load
```

Reason:

- calculated load may be produced by spreading;
- reserved load may represent a deliberate booking;
- actual load is historical consumption.

## Actuals and status_date

Reports must be calculable as of `status_date`.

Use:

```text
actuals where timestamp <= status_date
events/evidence where timestamp <= status_date
current plan snapshot as of status_date when available
selected baseline for comparison
```

If historical current-plan snapshots are not implemented yet, MVP should state:

```text
current plan as loaded, actuals up to status_date
```

The report must not hide that limitation.

## Baseline, current and scenario modes

The calculator should support these modes:

```text
baseline
current
scenario
comparison
```

### baseline

Calculate workload from an approved baseline snapshot.

### current

Calculate workload from the current mutable plan.

### scenario

Calculate workload from a scenario without mutating current plan or baseline.

### comparison

Compare:

```text
baseline vs current vs actual/forecast
```

MVP should start with:

```text
current calculation + baseline comparison
```

Scenario calculation can be simple at first, but the design should support it.

## Warning records

Warnings should be structured.

Example:

```yaml
warning:
  type: required_demand_has_no_estimate
  severity: high
  task_id: openproject:123
  demand_id: demand-001
  resource_ref: dev_ivan
  message: Required implementer demand has no planned or remaining amount
```

Warnings should be:

- machine-readable;
- human-readable;
- linked to task, demand, resource, bucket or scenario when possible;
- stable enough for AI-CLI agents to inspect and propose fixes.

## Required warnings

MVP should generate warnings for:

- required demand has no estimate;
- required demand has no scheduled date range;
- required demand has unknown capacity;
- required demand has zero capacity;
- required demand exceeds capacity;
- resource overloaded;
- required demand not reserved;
- reservation conflict;
- actual exceeds planned;
- forecast exceeds baseline;
- remaining amount unknown;
- no actuals for in-progress task;
- accepted task has missing actuals;
- quota forecast exceeds quota;
- rate limit conflict;
- budget forecast exceeds budget;
- material shortage;
- concurrent slot conflict;
- timezone missing for window resource;
- baseline/current mismatch;
- source status differs from tracking status.

## Report examples

Human workload example:

```text
Resource: dev_ivan
Date: 2026-07-07
  available: 8h
  planned load: 10h
  overload: 2h
  tasks:
    OP-123 Implement billing adapter: 6h
    OP-124 Fix import bug: 4h
```

Quota example:

```text
Resource: openai_api_project_quota
Period: 2026-07
  quota: 10,000,000 tokens
  actual: 4,200,000 tokens
  remaining: 7,100,000 tokens
  forecast: 11,300,000 tokens
  warning: quota forecast exceeds quota by 1,300,000 tokens
```

Environment example:

```text
Resource: staging_env
Window: 2026-07-07T11:00:00+07:00..2026-07-07T12:00:00+07:00
  capacity: 1 concurrent slot
  reservations: 2
  warning: concurrent slot conflict
```

Unscheduled example:

```text
Unscheduled required work
  OP-200: backend_developer_pool, 12h
    warning: required demand has no scheduled date range
```

## MVP requirements

MVP should implement or model:

- day buckets for effort-like resources;
- month or named period buckets for quota and budget resources;
- window buckets for concurrent resources;
- unscheduled bucket;
- `even`, `contiguous` and `manual` spread modes;
- missing date warnings;
- missing estimate warnings;
- capacity comparison;
- overload calculation;
- underuse calculation;
- quota shortage calculation;
- budget overrun calculation;
- material shortage calculation;
- concurrent slot conflict detection;
- actual plus remaining forecast;
- baseline/current comparison where baseline exists;
- `status_date` filtering for actuals;
- structured warnings;
- markdown and JSON-friendly report outputs.

## Explicit MVP non-goals

MVP should not implement:

- automatic resource leveling;
- optimal assignment;
- critical path solver;
- Monte Carlo forecast;
- earned value management;
- payroll;
- accounting;
- inventory ERP;
- full TaskJuggler clone;
- full MS Project-grade scheduling engine;
- automatic plan mutation;
- automatic re-baselining;
- automatic substitution solver.

TaskJuggler may later be used as an optional advanced scheduling engine.

The MVP workload calculator should remain explainable even if more advanced
engines are integrated later.

## Relationship to next design item

This document defines basic workload calculation.

The next design item defines exact comparison semantics:

```text
G. Planned/current/actual comparison
```

That item should settle:

- exact baseline/current/actual terminology;
- effort variance formulas;
- schedule variance formulas;
- quota variance formulas;
- budget variance formulas;
- forecast total semantics;
- how actual consumption and remaining work combine in reports.

## Approved decisions

- Workload calculation in MVP is deterministic rule-based reporting.
- Workload calculation does not automatically optimize or mutate the plan.
- Workload is calculated by distributing demands into buckets and comparing
  them with capacity.
- MVP bucket types are day, period/month, window and unscheduled.
- MVP spread modes are `even`, `contiguous` and `manual`.
- If a required demand has no dates, it goes to `unscheduled` and generates a
  warning.
- If a required demand has no estimate, RePlan must not assume zero.
- Forecast total defaults to `actual_amount + remaining_amount`.
- Unknown capacity is not treated as infinite capacity.
- Overload is calculated as load above available capacity.
- Underuse is calculated as available capacity above load.
- Non-hour resources use the same demand/capacity comparison pattern with
  resource-specific buckets and warnings.
- Reports must support `status_date`.
- Calculator should support baseline, current, scenario and comparison modes by
  design.
- MVP should emit structured warnings suitable for humans and AI-CLI agents.
