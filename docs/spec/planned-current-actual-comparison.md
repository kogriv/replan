# Planned, current and actual comparison

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how RePlan compares approved plan, current plan, actual
execution and forecast.

It answers:

- what exactly `baseline` means;
- what exactly `current plan` means;
- what exactly `actual` means;
- what exactly `forecast` means;
- how effort variance is calculated;
- how schedule variance is calculated;
- how quota variance is calculated;
- how budget variance is calculated;
- how material variance is calculated;
- how actual consumption and remaining work combine in reports;
- how variance differs from capacity shortage.

This document is functional design. The exact canonical schema will be defined
later in `docs/spec/canonical-model.md`.

## Core decision

RePlan must always separate these four layers:

```text
baseline
current
actual
forecast
```

Definitions:

```text
baseline = approved snapshot of planning commitments
current  = mutable working version of the plan
actual   = accepted execution facts
forecast = actual + remaining according to the current known state
```

The comparison answers:

```text
what did we approve;
what do we currently plan;
what has actually happened;
where do we now expect to finish.
```

## Baseline

`baseline` is an approved snapshot.

It fixes the reference plan.

A baseline should capture:

- included tasks;
- included resource demands;
- planned resources;
- planned amounts;
- planned start dates;
- planned finish dates;
- planned deadlines;
- planned constraints;
- planned assumptions;
- capacity assumptions where relevant;
- approving actor;
- approval timestamp;
- baseline version.

Baseline does not change when tracking events arrive.

Baseline does not change when actuals are recorded.

Baseline does not change when current plan changes.

If the plan is formally re-approved, RePlan creates a new baseline:

```text
baseline_v1
baseline_v2
baseline_v3
```

Reports must state which baseline is used for comparison.

## Current plan

`current plan` is the mutable working version of the plan.

It answers:

```text
what do we currently believe the working plan is?
```

The current plan may differ from baseline because:

- tasks were added;
- tasks were removed;
- estimates changed;
- remaining work changed;
- resource demands changed;
- resources were replaced;
- dates changed;
- deadlines changed;
- capacity assumptions changed;
- actuals changed the forecast;
- proposals were accepted;
- scenarios were applied.

Current plan can change during normal work.

Changing current plan does not rewrite baseline.

## Actual

`actual` means accepted execution fact.

Actuals may include:

- human hours consumed;
- AI-agent hours consumed;
- tokens consumed;
- API requests consumed;
- budget spent;
- material consumed;
- equipment used;
- environment used;
- vehicle used;
- actual start timestamp;
- actual finish timestamp;
- accepted status transitions;
- accepted evidence-backed facts.

Actual is not the same as an external event.

Actual is not the same as a pull request.

Actual is not the same as source status.

Actual is what RePlan accepted as fact according to the tracking lifecycle.

## Forecast

`forecast` is the current expectation of the final result.

Default formula:

```text
forecast_total = actual_amount + remaining_amount
```

Example:

```text
baseline planned: 8h
actual: 3h
remaining: 7h
forecast: 10h
variance: +2h
```

If `remaining_amount` is unknown:

```text
forecast_total = unknown
```

Warning:

```text
remaining amount unknown
```

Forecast is not a baseline.

Forecast is not an accepted commitment unless a new baseline is explicitly
created.

## Implemented first report slice

The first implemented comparison surface is part of:

```bash
replan report state --project examples/replan-self --week 2026-W31 --format markdown
replan report state --project examples/replan-self --week 2026-W31 --format json
```

The JSON result includes `tracking` rows in the dashboard overview model.

The first implemented advisory action surface is:

```bash
replan tracking suggest --project examples/replan-self --week 2026-W31 --format markdown
replan tracking suggest --project examples/replan-self --week 2026-W31 --format json
replan tracking suggest --project examples/replan-self --week 2026-W31 --write-candidates --actor actor.ai.codex.local --format json
```

This command consumes Plan versus Actual signals and produces read-only
operator recommendations. It does not update actuals, remaining work,
proposals, audit files or the primary project-management system. It is the
first bridge from passive comparison to an explicit review loop for humans and
AI CLI agents.

With `--write-candidates --actor <id>`, supported suggestions are materialized
as reviewable proposal files. The first writable suggestion is
`set_remaining_to_zero_candidate`, which becomes an `update_remaining_work`
proposal setting `remaining_amount` to `0`. This mode still does not apply the
proposal and does not mutate canonical resource demands; it only creates
`proposals/open/*.yaml` files for the existing proposal review loop.

Accepted `update_remaining_work` proposals now close the first tracking loop:

```bash
replan proposal accept --project <project> proposal.tracking.<demand>.set-remaining-zero --dry-run --format json
replan proposal accept --project <project> proposal.tracking.<demand>.set-remaining-zero --actor actor.human.maintainer --apply --format json
```

The dry-run shows that `canonical/resource_demands.yaml` would be updated. The
mutating accept updates the target demand `remaining_amount`, moves the
proposal to `proposals/accepted`, appends audit and validates the project. It
does not create actuals, evidence records or OpenProject write-back.

The first slice compares current planned resource demands with accepted actuals
and current remaining amounts. It intentionally keeps every resource demand and
unit separate. Hours, agent hours, review hours, tokens, slots and other units
are not aggregated into one synthetic progress number.

Task-level rows include:

- task id;
- title;
- tracking status;
- planned start and finish;
- actual count;
- evidence count;
- tracking flags.

Demand-level rows include:

- demand id;
- resource id;
- resource class;
- role;
- unit;
- planned amount;
- actual amount;
- remaining amount;
- forecast amount;
- variance;
- evidence count;
- tracking flags.

The first suggestion types derived from these rows are:

- `set_remaining_to_zero_candidate`: an accepted or done task has actual
  consumption, but the demand still has positive remaining work, so the
  forecast is probably inflated by stale remaining work;
- `review_missing_actuals`: an accepted or done task has demands but no actual
  consumption records;
- `review_missing_evidence`: an actual consumption record has no evidence
  references;
- `review_forecast_over_plan`: a demand forecast is above planned amount and
  should be reviewed as a possible real overrun, stale remaining value,
  duplicate actual or incomplete estimate.

The first implemented formula is:

```text
forecast = actual + remaining
variance = forecast - planned
```

The first flags are:

- `no_actuals`;
- `accepted_without_actuals`;
- `actuals_without_evidence`;
- `over_consumed`;
- `forecast_over_plan`.

This is not a timesheet, payroll, employee-surveillance or full earned-value
feature. It is an operator-facing execution check that highlights where actuals,
remaining work or evidence need review.

## Required comparison fields

For each comparable task, demand or resource bucket, RePlan should be able to
represent:

```text
baseline_planned_amount
current_planned_amount
actual_amount
remaining_amount
forecast_total
variance_from_baseline
```

For schedule comparison:

```text
baseline_start
baseline_finish
current_start
current_finish
actual_start
actual_finish
forecast_finish
finish_variance_days
```

For capacity comparison:

```text
available_capacity
current_load
forecast_load
capacity_shortage
```

Capacity shortage is separate from variance.

## Effort variance

For effort-like resources, the primary MVP formula is:

```text
effort_variance = forecast_total - baseline_planned_amount
```

Example:

```text
baseline planned: 8h
actual: 3h
remaining: 7h
forecast total: 10h
effort variance: +2h
```

This answers:

```text
How much more or less effort do we now expect compared with the approved plan?
```

If comparing current plan with baseline without actuals:

```text
planned_effort_delta = current_planned_amount - baseline_planned_amount
```

If comparing actuals to baseline-to-date:

```text
actual_effort_variance = actual_amount - baseline_planned_amount_to_date
```

For MVP, the most important value is:

```text
forecast_total - baseline_planned_amount
```

because it explains where the task is now expected to land.

## Schedule variance

Schedule variance should measure date movement.

Primary MVP formula:

```text
finish_variance_days = forecast_finish_date - baseline_finish_date
```

Example:

```text
baseline finish: 2026-07-09
forecast finish: 2026-07-11
finish variance: +2 days
```

Optional start variance:

```text
start_variance_days = actual_or_current_start_date - baseline_start_date
```

For MVP, finish variance is enough.

If dates are unknown:

```text
schedule variance = unknown
```

Warnings:

```text
missing baseline finish
missing forecast finish
missing current dates
```

## Quota variance

Quota variance compares forecast quota consumption against baseline planned
quota consumption.

Formula:

```text
quota_variance = forecast_quota_consumption - baseline_planned_quota
```

Example:

```text
baseline: 120000 tokens
actual: 82000 tokens
remaining: 70000 tokens
forecast: 152000 tokens
quota variance: +32000 tokens
```

Quota capacity shortage is different:

```text
quota_shortage = max(0, forecast_quota_consumption - available_quota_capacity)
```

Variance answers:

```text
How far are we from the approved plan?
```

Shortage answers:

```text
Do we have enough quota?
```

## Budget variance

Budget variance compares forecast spend against baseline budget.

Formula:

```text
budget_variance = forecast_spend - baseline_budget
```

Budget capacity excess is separate:

```text
over_budget = max(0, forecast_spend - budget_capacity)
```

Example:

```text
baseline budget: 18000 rub
actual spend: 12000 rub
remaining budget need: 9000 rub
forecast spend: 21000 rub
budget variance: +3000 rub
```

If project budget capacity is 25000 rub, there is variance but no capacity
shortage.

If project budget capacity is 20000 rub, there is both variance and capacity
shortage.

## Material variance

Material variance compares forecast material consumption against baseline
planned material quantity.

Formula:

```text
material_variance = forecast_quantity - baseline_quantity
```

Stock shortage is separate:

```text
material_shortage = max(0, required_quantity - available_stock)
```

Example:

```text
baseline cable: 100 meters
actual used: 70 meters
remaining need: 50 meters
forecast: 120 meters
material variance: +20 meters
available stock: 200 meters
shortage: 0 meters
```

## Capacity shortage versus variance

Variance and shortage must not be mixed.

Variance compares with baseline:

```text
forecast_or_current - baseline
```

Shortage compares with available capacity:

```text
forecast_or_load - available_capacity
```

Example:

```text
baseline planned: 8h
forecast: 10h
available capacity: 12h
```

Result:

```text
variance: +2h
capacity shortage: 0h
```

Another example:

```text
baseline planned: 8h
forecast: 8h
available capacity: 6h
```

Result:

```text
variance: 0h
capacity shortage: 2h
```

These are different planning signals.

## Current versus baseline

Current-vs-baseline comparison should show:

- added tasks;
- removed tasks;
- changed estimates;
- changed resource demands;
- changed resources;
- changed dates;
- changed deadlines;
- changed capacity assumptions;
- changed scenario assumptions if a scenario was applied.

Example:

```text
OP-123
  baseline implementer: dev_ivan 8h
  current implementer: backend_pool 10h
  delta: +2h
  resource changed: dev_ivan -> backend_pool
```

This comparison explains how the working plan drifted from the approved plan.

## Actual versus current

Actual-vs-current comparison should show:

- actual consumed amount;
- remaining amount;
- forecast total;
- tracking status;
- evidence;
- missing actuals;
- unknown remaining amount;
- stale progress;
- no recent progress update.

Example:

```text
OP-123
  current remaining: 7h
  actual spent: 3h
  forecast: 10h
  tracking status: in_progress
  warning: forecast exceeds baseline
```

This comparison explains execution state.

## Baseline versus actual and forecast

Baseline-vs-actual/forecast comparison should show:

- approved plan;
- actual consumed so far;
- current remaining;
- forecast total;
- variance;
- warnings.

Example:

```text
OP-123 Implement billing adapter
  baseline effort: 8h
  actual effort: 3h
  remaining effort: 7h
  forecast effort: 10h
  effort variance: +2h
```

## Report model

Minimum task/resource/demand comparison:

```text
Task/resource/demand
  baseline planned
  current planned
  actual
  remaining
  forecast
  variance from baseline
  capacity warning
  evidence/proposal links
```

Resource summary example:

```text
Resource dev_ivan
  baseline load: 32h
  current load: 40h
  actual: 12h
  remaining: 30h
  forecast: 42h
  variance: +10h
```

Task example:

```text
OP-123 Implement billing adapter
  baseline finish: 2026-07-09
  forecast finish: 2026-07-11
  finish variance: +2 days
  baseline effort: 8h
  actual effort: 3h
  remaining effort: 7h
  forecast effort: 10h
  effort variance: +2h
  warnings:
    - forecast exceeds baseline
    - reviewer demand not reserved
```

## Status date

Comparison reports should respect `status_date`.

Rules:

- actuals after `status_date` are excluded;
- evidence after `status_date` is excluded;
- accepted status changes after `status_date` are excluded;
- baseline selection must be explicit;
- current-plan snapshot as of `status_date` should be used when available.

If historical current-plan snapshots are not implemented yet, report output
must state:

```text
current plan as loaded, actuals up to status_date
```

## Unknown values

Unknown must not be treated as zero.

Examples:

- unknown remaining amount means forecast is unknown;
- unknown finish date means schedule variance is unknown;
- unknown capacity means shortage cannot be cleared;
- unknown baseline means variance cannot be calculated.

Warnings should make unknowns visible.

## Earned value non-goal

MVP should not implement full Earned Value Management.

Do not implement in MVP:

- Budgeted Cost of Work Scheduled;
- Budgeted Cost of Work Performed;
- Actual Cost of Work Performed;
- Cost Performance Index;
- Schedule Performance Index;
- enterprise financial performance reporting.

RePlan needs explainable planning variance, not an enterprise EVM suite.

Approved MVP framing:

```text
planned vs current vs actual vs forecast
```

## MVP requirements

MVP should implement or model:

- baseline planned amount;
- current planned amount;
- actual amount;
- remaining amount;
- forecast total;
- variance from baseline;
- baseline start and finish where available;
- current start and finish where available;
- actual start and finish where available;
- forecast finish where available;
- finish variance in days where dates exist;
- quota variance;
- budget variance;
- material variance;
- capacity shortage separately from variance;
- report output showing baseline/current/actual/remaining/forecast;
- warnings for unknown values.

MVP should not implement:

- full Earned Value Management;
- enterprise financial reporting;
- complex cost accounting;
- automatic baseline mutation;
- automatic re-baselining;
- hidden forecast commitments.

## Relationship to other documents

This document depends on:

- [`planning-lifecycle.md`](planning-lifecycle.md);
- [`tracking-events.md`](tracking-events.md);
- [`resource-demand-model.md`](resource-demand-model.md);
- [`capacity-availability-rules.md`](capacity-availability-rules.md);
- [`workload-calculation.md`](workload-calculation.md).

The next design item is:

```text
H. Dependency and deadline behavior
```

## Approved decisions

- RePlan separates baseline, current, actual and forecast.
- Baseline is an approved snapshot.
- Current plan is the mutable working plan.
- Actuals are accepted execution facts.
- Forecast defaults to actual plus remaining.
- Variance compares current or forecast values with baseline.
- Capacity shortage compares load or forecast with available capacity.
- Variance and capacity shortage are different signals.
- Primary effort variance in MVP is `forecast_total - baseline_planned_amount`.
- Primary schedule variance in MVP is forecast finish minus baseline finish.
- Quota variance compares forecast quota consumption with baseline quota.
- Budget variance compares forecast spend with baseline budget.
- Material variance compares forecast quantity with baseline quantity.
- Unknown values must not be treated as zero.
- Reports must support `status_date`.
- MVP should not implement full Earned Value Management.
