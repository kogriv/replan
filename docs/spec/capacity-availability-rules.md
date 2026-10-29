# Capacity and availability rules

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how RePlan represents resource capacity and availability.

It answers:

- what a working day means;
- how daily, weekly, monthly and arbitrary-period capacity should be represented;
- how vacations, absences, maintenance windows and reduced days are represented;
- how AI-agent capacity, API quotas, rate limits and subscriptions are
  represented;
- how equipment, environments, locations, materials and budgets expose
  availability;
- what the minimal MVP calendar model should be;
- what is explicitly not part of MVP.

This document is functional design. The exact canonical schema will be defined
later in `docs/spec/canonical-model.md`.

## Core decision

Resource demand and capacity are separate sides of planning.

Approved rule:

```text
resource demand = what a task needs
resource capacity = how much a resource can provide
resource availability = when that resource can be used
reservation/allocation = what is already occupied by the plan
actual consumption = what was actually consumed
```

Short version:

```text
demand = need
capacity = supply
availability = time/window constraints on supply
```

RePlan must compare demands against capacity and availability to produce
workload, overload, shortage, conflict and missing-data warnings.

## Capacity is not only human hours

RePlan must not model capacity only as:

```yaml
ivan:
  hours_per_day: 8
```

That is too narrow.

RePlan needs a generic capacity model:

```text
resource + unit + amount + period/window
```

Human example:

```yaml
resource_capacity:
  resource_id: dev_ivan
  resource_class: human
  unit: hours
  capacity:
    amount: 8
    period: day
```

API quota example:

```yaml
resource_capacity:
  resource_id: openai_api_project_quota
  resource_class: service_quota
  unit: tokens
  capacity:
    amount: 1000000
    period: month
```

Environment example:

```yaml
resource_capacity:
  resource_id: staging_env
  resource_class: environment
  unit: concurrent_slots
  capacity:
    amount: 1
    period: concurrent
```

Material or utility example:

```yaml
resource_capacity:
  resource_id: water_supply_greenhouse
  resource_class: material
  unit: liters
  capacity:
    amount: 2000
    period: day
```

Budget example:

```yaml
resource_capacity:
  resource_id: project_budget
  resource_class: budget
  unit: rub
  capacity:
    amount: 250000
    period: month
```

## Capacity and availability

Capacity answers:

```text
How much can this resource provide?
```

Availability answers:

```text
When can this resource be used?
```

Examples:

- a developer can provide 8 hours per workday;
- an API can provide 1,000,000 tokens per month and 500 requests per minute;
- a staging environment has 1 concurrent slot and is unavailable during
  maintenance;
- a site can be accessed only from 09:00 to 18:00;
- a supplier ships only on weekdays;
- a stock resource has 500 meters of cable available now;
- a budget has 250,000 rubles available this month.

RePlan should model these constraints explicitly enough to produce useful
warnings without becoming a full enterprise resource management system.

## Capacity types

Approved capacity types:

```text
periodic_capacity
availability_window
concurrent_capacity
quota_capacity
rate_limit
stock_capacity
budget_capacity
approval_capacity
```

### periodic_capacity

Capacity that repeats per period.

Examples:

- 8 hours per day;
- 40 hours per week;
- 160 hours per month;
- 5 reviews per day;
- 2 delivery runs per day.

### availability_window

Specific time windows when the resource can be used.

Examples:

- site access from 09:00 to 18:00;
- maintenance window from 01:00 to 04:00;
- vendor pickup window on Tuesday;
- room available from 14:00 to 16:00.

### concurrent_capacity

How many concurrent uses are allowed.

Examples:

- 1 staging environment;
- 3 AI-agent sessions;
- 2 lab devices;
- 1 meeting room;
- 5 concurrent SaaS seats.

### quota_capacity

Total available consumption in a period.

Examples:

- 10,000,000 tokens per month;
- 100,000 API requests per day;
- 2,000 CI minutes per month;
- 500 geocoding requests per day.

### rate_limit

Maximum consumption speed.

Examples:

- 500 requests per minute;
- 60 messages per hour;
- 10 build starts per hour.

### stock_capacity

Available quantity of consumable or reservable stock.

Examples:

- 500 meters of cable;
- 10 pumps;
- 50 liters of solvent;
- 12 spare devices.

### budget_capacity

Available money.

Examples:

- 250,000 rubles per month;
- 500 USD for API overage;
- 30,000 rubles for procurement task group.

### approval_capacity

Capacity of review, approval or acceptance actors.

Examples:

- 5 code reviews per day;
- 3 procurement approvals per week;
- 10 inspection acceptances per month;
- 2 operator confirmations per hour.

Approval capacity is important because review and acceptance often become
hidden bottlenecks.

## Working day

RePlan should not hard-code:

```text
working day = 8 hours
```

A working day is a calendar day with positive relevant capacity for a resource.

Example calendar:

```yaml
calendar:
  id: workdays_5x8_nsk
  timezone: Asia/Novosibirsk
  working_days:
    - monday
    - tuesday
    - wednesday
    - thursday
    - friday
  work_windows:
    - start: "09:00"
      finish: "18:00"
  break_windows:
    - start: "13:00"
      finish: "14:00"
```

Example capacity using that calendar:

```yaml
resource_capacity:
  resource_id: dev_ivan
  calendar: workdays_5x8_nsk
  default_capacity:
    unit: hours
    amount: 8
    period: day
```

For MVP, RePlan can use a simpler interpretation:

```text
working day = date with capacity greater than zero
```

MVP does not need minute-by-minute scheduling for ordinary human work.

## Time zones

Capacity and availability records should include timezone when dates or local
working windows are involved.

Example:

```yaml
timezone: Asia/Novosibirsk
```

This matters for:

- distributed teams;
- API reset times;
- supplier windows;
- site access windows;
- reporting by `status_date`;
- comparing actual events with planned windows.

MVP may assume one project timezone if per-resource timezone support is not yet
implemented.

The project timezone should still be explicit.

## Exceptions

Exceptions modify base capacity or availability.

They are required from the beginning.

Human examples:

```yaml
exceptions:
  - date: 2026-07-10
    capacity_amount: 0
    unit: hours
    reason: vacation
  - date: 2026-07-11
    capacity_amount: 4
    unit: hours
    reason: reduced_day
```

API/subscription example:

```yaml
exceptions:
  - period: 2026-07
    capacity_amount: 500000
    unit: tokens
    reason: downgraded_subscription
```

Environment example:

```yaml
exceptions:
  - start: 2026-07-08T00:00:00+07:00
    finish: 2026-07-08T06:00:00+07:00
    capacity_amount: 0
    unit: concurrent_slots
    reason: maintenance
```

Material example:

```yaml
exceptions:
  - date: 2026-07-05
    capacity_amount: -100
    unit: meters
    reason: damaged_stock
```

Without exceptions, the model would fail on real planning almost immediately.

## Reservations and allocations

Availability says that a resource can be used.

Reservation or allocation says that the plan already occupies some of that
resource.

Environment reservation:

```yaml
reservation:
  resource_id: staging_env
  demand_id: demand-123
  amount: 1
  unit: concurrent_slots
  start: 2026-07-07T10:00:00+07:00
  finish: 2026-07-07T13:00:00+07:00
```

Human allocation:

```yaml
reservation:
  resource_id: dev_ivan
  demand_id: demand-456
  amount: 4
  unit: hours
  date: 2026-07-07
```

Budget reservation:

```yaml
reservation:
  resource_id: project_budget
  demand_id: demand-789
  amount: 18000
  unit: rub
  period: 2026-07
```

Stock reservation:

```yaml
reservation:
  resource_id: cable_cat6_stock
  demand_id: demand-790
  amount: 120
  unit: meters
```

For MVP, reservations can be treated as planned allocations.

The model should still keep reservation separate from actual consumption.

## Actual consumption

Actual consumption records what was really consumed.

Examples:

- 3 human hours spent;
- 82,000 tokens consumed;
- 2 GPU hours used;
- 120 meters of cable consumed;
- 18,000 rubles spent;
- staging environment used for 2 hours;
- service vehicle used for 4 hours.

Actual consumption should be compared against:

- planned demand;
- remaining demand;
- reserved capacity;
- available capacity;
- baseline where applicable.

Actual consumption may come from:

- manual report;
- PM-system time entry;
- agent runtime;
- API billing;
- CI/build system;
- procurement system;
- inventory system;
- operator confirmation.

## Basic calculation

The basic capacity calculation is:

```text
available_capacity = base_capacity + exceptions - reservations
```

The basic load calculation is:

```text
load = sum(planned allocations or demands for the same bucket)
```

The basic overload calculation is:

```text
overload = max(0, load - available_capacity)
```

The basic underuse calculation is:

```text
underuse = max(0, available_capacity - load)
```

These formulas are intentionally generic.

Exact workload calculation rules are defined in
`docs/spec/workload-calculation.md`.

## Resource-specific examples

### Human

```text
available: 8h on 2026-07-07
planned load: 10h
overload: 2h
```

### API quota

```text
capacity: 1,000,000 tokens/month
forecast: 1,200,000 tokens/month
shortage: 200,000 tokens
```

### Rate limit

```text
limit: 500 requests/minute
planned burst: 800 requests/minute
conflict: 300 requests/minute over limit
```

### Environment

```text
concurrent capacity: 1
task A wants staging 10:00-13:00
task B wants staging 11:00-12:00
conflict: overlapping reservations
```

### Material

```text
stock: 10 pumps
planned demand: 12 pumps
shortage: 2 pumps
```

### Budget

```text
budget: 250,000 rubles/month
planned spend: 280,000 rubles/month
over budget: 30,000 rubles
```

## AI-agent capacity

AI agents should not be modeled exactly as people.

They may have:

- concurrent session limits;
- token budgets;
- request limits;
- model availability constraints;
- tool access constraints;
- repository access constraints;
- cost budgets;
- trust levels;
- max task duration;
- human review requirements;
- provider downtime;
- rate limits.

Example:

```yaml
resource_capacity:
  resource_id: codex_agent_pool
  resource_class: ai_agent
  capacity:
    unit: concurrent_sessions
    amount: 3
    period: concurrent
  quotas:
    - unit: tokens
      amount: 5000000
      period: day
    - unit: usd
      amount: 25
      period: day
  rate_limits:
    - unit: tool_calls
      amount: 1000
      period: hour
  availability:
    windows:
      - start: always
        finish: always
  policy:
    requires_human_review: true
```

For MVP, AI-agent capacity can be represented as:

```text
N concurrent sessions + token quota + cost quota
```

Security, trust and permissions are described in
[`../security/access-and-actor-model.md`](../security/access-and-actor-model.md).

## API, subscription and rate-limit capacity

Service resources need explicit quota and rate-limit support.

RePlan should represent:

- monthly limit;
- daily limit;
- rate limit;
- concurrent request limit;
- paid credits;
- subscription seats;
- reset date;
- soft limit;
- hard limit;
- whether overage is allowed;
- overage cost where known.

Example:

```yaml
resource_capacity:
  resource_id: openai_api_project_quota
  resource_class: service_quota
  quotas:
    - unit: tokens
      amount: 10000000
      period: month
      reset: calendar_month
      limit_type: hard
    - unit: requests
      amount: 10000
      period: day
      limit_type: soft
  rate_limits:
    - unit: requests
      amount: 500
      period: minute
  overage:
    allowed: false
```

Subscription seats example:

```yaml
resource_capacity:
  resource_id: design_tool_subscription
  resource_class: subscription
  capacity:
    unit: seats
    amount: 5
    period: month
  concurrent_capacity:
    unit: active_users
    amount: 5
```

## Offline resource capacity

Offline work often depends on resources that do not look like developer hours.

Examples:

- people;
- vehicles;
- tools;
- equipment;
- materials;
- stock;
- site access;
- room availability;
- budget;
- supplier capacity;
- weather window;
- permit or approval.

Site access example:

```yaml
resource_capacity:
  resource_id: site_north_access
  resource_class: location
  availability_windows:
    - start: 2026-07-06T09:00:00+07:00
      finish: 2026-07-06T18:00:00+07:00
    - start: 2026-07-07T09:00:00+07:00
      finish: 2026-07-07T18:00:00+07:00
```

Material stock example:

```yaml
resource_capacity:
  resource_id: cable_cat6_stock
  resource_class: material
  unit: meters
  stock:
    amount: 500
```

Vehicle example:

```yaml
resource_capacity:
  resource_id: service_vehicle_1
  resource_class: vehicle
  unit: vehicle_hours
  capacity:
    amount: 8
    period: day
  calendar: workdays_5x8_nsk
```

Vendor example:

```yaml
resource_capacity:
  resource_id: approved_supplier_acme
  resource_class: vendor
  unit: deliveries
  capacity:
    amount: 3
    period: week
  availability:
    working_days:
      - monday
      - wednesday
      - friday
```

## Unknown capacity

Missing capacity data must not be treated as infinite capacity.

If a required demand exists and the corresponding capacity is unknown, RePlan
should generate a warning.

Example:

```text
required demand has unknown capacity
```

Useful capacity states:

```text
known_capacity
unknown_capacity
unavailable
overloaded
underused
conflict
shortage
```

Examples:

- required reviewer demand exists, but reviewer pool capacity is unknown;
- task requires API quota, but subscription limit has not been entered;
- construction task requires site access, but access windows are unknown;
- procurement task requires budget, but budget capacity is unknown.

Unknown capacity should not block all reporting.

It should make reports honest:

```text
This plan may be infeasible because capacity for required demand X is unknown.
```

## Warnings

Capacity and availability rules should generate warnings such as:

- required demand has unknown capacity;
- required demand has zero capacity;
- demand exceeds available capacity;
- resource is overloaded;
- resource is underused;
- reservation conflicts with another reservation;
- demand requires unavailable window;
- quota forecast exceeds capacity;
- rate limit forecast exceeds limit;
- stock demand exceeds available stock;
- budget demand exceeds available budget;
- required demand is not reserved;
- actual consumption exceeds planned demand;
- reserved capacity exceeds available capacity;
- capacity exception invalidates existing reservation;
- timezone missing for local availability window.

Warnings should be machine-readable and human-readable.

## MVP requirements

MVP should implement or model:

- resource capacity records;
- resource availability windows;
- project timezone;
- daily periodic capacity;
- weekly periodic capacity;
- simple monthly quota capacity;
- exceptions;
- reservations or allocations;
- simple concurrent capacity;
- simple rate limits;
- stock quantity;
- budget amount;
- unknown capacity warnings;
- overload warnings;
- shortage warnings;
- conflict warnings;
- reports by resource, unit and date or period.

MVP may treat many advanced rules as warnings rather than automatic scheduling
constraints.

## Explicit MVP non-goals

MVP should not implement:

- complex calendar engine;
- minute-level scheduling for all work;
- automatic resource leveling;
- automatic optimal assignment;
- enterprise resource pool management;
- payroll;
- employee surveillance;
- advanced cost accounting;
- full inventory management;
- warehouse management;
- procurement ERP;
- weather integrations;
- legal compliance workflows;
- MS Project-grade leveling;
- automatic optimization;
- automatic substitution solver.

These exclusions do not mean the concepts are irrelevant.

They mean the MVP should expose enough structure and warnings to support
practical planning without becoming a large enterprise system.

## Relationship to workload calculation

This document defines available supply.

The workload calculation spec will define:

- how remaining work is spread across dates;
- how load is calculated per bucket;
- how overload and underuse are calculated;
- how non-hour resources are handled;
- how unknown capacity affects reports;
- which warnings are emitted by calculations.

The next design item is:

```text
F. Workload calculation rules
```

## Approved decisions

- Capacity and availability are separate from resource demand.
- Capacity is generic and is not limited to human hours.
- Capacity is represented by resource, unit, amount and period or window.
- Availability describes when capacity can be used.
- A working day is not hard-coded as 8 hours.
- A working day is a date with positive relevant capacity.
- Timezone should be explicit when local dates or windows are used.
- Exceptions are required from the beginning.
- Reservations/allocations are separate from actual consumption.
- Missing capacity is unknown capacity, not infinite capacity.
- Unknown capacity should produce warnings instead of silently passing.
- AI agents need concurrent session, quota, rate-limit, cost and review-policy
  capacity.
- API, subscription and rate-limit resources are first-class capacity cases.
- Offline resources such as sites, vehicles, materials, stock, suppliers and
  budget use the same capacity model.
- MVP should support periodic capacity, windows, exceptions, reservations,
  quotas, rate limits, concurrent slots, stock, budget and warnings.
- MVP should not attempt full automatic resource leveling or enterprise
  resource optimization.
