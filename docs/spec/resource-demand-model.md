# Resource demand model

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how a task demands resources in RePlan.

It answers:

- whether a task has one assignee or many resource demands;
- how `assignee` differs from `resource_demands`;
- how to represent implementer time, reviewer time, AI-agent time, service
  quotas, budget, environments, equipment, materials and other resource needs;
- how to distinguish required, preferred, optional and substitute demands;
- how to represent planned, remaining, reserved and actual resource amounts;
- how to keep the MVP useful without building a full enterprise resource
  optimizer.

This document is functional design. The exact canonical schema will be defined
later in `docs/spec/canonical-model.md`.

## Core decision

RePlan tasks must support multiple resource demands.

The core model is:

```text
task.resource_demands[]
```

The core model is not:

```text
task.assignee + task.estimate
```

`assignee` is useful for compatibility with project-management systems, but it
is too weak to be the RePlan planning model.

Approved rule:

```text
task.assignee = compatibility hint from a PM system
task.resource_demands[] = real resource planning model
```

## Why `assignee` is not enough

Most task trackers treat a task as:

```yaml
task:
  assignee: ivan
  estimate: 8h
```

That is acceptable for basic task ownership.

It is not acceptable for resource planning.

A single task may require:

- implementer time;
- reviewer time;
- tester time;
- architect time;
- AI coding agent time;
- LLM API tokens;
- API request quota;
- SaaS subscription seat;
- GPU hours;
- CI/build minutes;
- staging environment time;
- hardware device time;
- robot time;
- vehicle time;
- room time;
- site access window;
- material quantity;
- procurement budget;
- vendor service;
- operator confirmation;
- customer acceptance.

If RePlan stores only one assignee and one estimate, it cannot answer the
questions it is meant to answer:

- who or what is overloaded;
- what non-human resource is the limiting factor;
- which task is blocked by quota, budget, equipment or environment;
- how much reviewer capacity is needed;
- whether AI-agent work shifts effort to review, quota or budget;
- whether a physical task is feasible during the available access window;
- whether a procurement task is limited by money, supplier time or delivery
  capacity.

Therefore resource planning must be built around explicit demands.

## Resource demand

A resource demand is a statement that a task needs some resource in some amount
under some conditions.

Conceptual example:

```yaml
resource_demand:
  id: demand-001
  task_id: openproject:123
  resource_ref:
    type: resource
    id: dev_ivan
  resource_class: human
  role: implementer
  unit: hours
  planned_amount: 8
  remaining_amount: 8
  reserved_amount: 0
  actual_amount: 0
  criticality: required
  status: planned
  timing:
    earliest_start: 2026-07-06
    latest_finish: 2026-07-09
    spread: even
```

The exact field names may change in the canonical schema, but the concepts are
approved.

## Minimal demand fields

Each resource demand should contain:

- stable demand identifier;
- task identifier;
- resource reference;
- resource class;
- role;
- unit;
- planned amount;
- remaining amount;
- reserved amount;
- actual amount;
- criticality;
- demand status;
- timing constraints;
- source or author;
- evidence links when relevant;
- proposal links when relevant.

## Resource reference

`resource_ref` can point to a concrete resource or to a pool.

Concrete resource example:

```yaml
resource_ref:
  type: resource
  id: dev_ivan
```

Pool example:

```yaml
resource_ref:
  type: pool
  id: backend_developers
```

Unknown or unresolved demand example:

```yaml
resource_ref:
  type: unresolved
  id: any_backend_developer
```

The model must support pools because planning often begins before the exact
person, agent, device or vendor is chosen.

## Resource class

`resource_class` is a broad category.

Examples:

- `human`;
- `ai_agent`;
- `service_quota`;
- `rate_limit`;
- `subscription`;
- `budget`;
- `equipment`;
- `environment`;
- `material`;
- `location`;
- `vehicle`;
- `robot`;
- `vendor`;
- `approval`;
- `generic`.

Resource classes are not meant to be a closed ontology.

The architecture-level rule from
[`resource-abstraction.md`](../architecture/resource-abstraction.md) still
applies:

```text
anything that constrains work can be a resource
```

The demand model needs enough structure for calculation and reporting, while
still allowing future resource classes.

## Role

`role` describes why this resource is needed by this task.

Examples:

- `implementer`;
- `reviewer`;
- `tester`;
- `architect`;
- `operator`;
- `approver`;
- `ai_coding_agent`;
- `ai_reviewer`;
- `service_quota`;
- `runtime`;
- `environment`;
- `material`;
- `budget`;
- `delivery`;
- `inspection`;
- `customer_acceptance`.

Role is task-specific.

The same resource can appear with different roles in different tasks.

Example:

```yaml
resource_id: dev_ivan
role: implementer
```

and:

```yaml
resource_id: dev_ivan
role: reviewer
```

should be treated as different demands because they consume the same person's
capacity for different work.

## Units

Demand units must not be limited to hours.

Approved unit categories:

- time;
- money;
- quota;
- compute;
- material;
- space or location;
- count;
- service unit.

Examples:

```text
hours
days
rub
usd
eur
tokens
api_requests
credits
cpu_hours
gpu_hours
build_minutes
pieces
meters
liters
kg
room_hours
site_hours
vehicle_hours
items
deliveries
sessions
approvals
```

This is required for:

- API-on-demand resources;
- subscription quotas;
- rate limits;
- AI-agent usage;
- offline work;
- procurement;
- construction;
- logistics;
- maintenance;
- physical equipment;
- budget planning.

## Amount fields

RePlan must keep these amounts separate:

```yaml
planned_amount: 8
remaining_amount: 5
reserved_amount: 6
actual_amount: 3
```

They answer different questions.

`planned_amount`:

```text
How much did we plan to need?
```

`remaining_amount`:

```text
How much do we currently believe is still needed?
```

`reserved_amount`:

```text
How much capacity or quantity is already reserved?
```

`actual_amount`:

```text
How much was actually consumed?
```

These fields must not be collapsed into one value.

Example:

```text
Baseline planned effort: 8 hours
Actual effort so far: 3 hours
Remaining effort now: 7 hours
Forecast total: 10 hours
Variance: +2 hours
```

If RePlan only stores one number, it cannot explain that the task became larger
after work started.

## Forecast total

For consumable demands, a common forecast is:

```text
forecast_total = actual_amount + remaining_amount
```

This does not mean every resource is additive in the same way.

Examples:

- human effort is usually additive;
- token consumption is additive;
- money spent plus remaining budget is additive;
- a room reservation may be additive by hours;
- a material reservation may need separate ordered, delivered and consumed
  quantities;
- a subscription seat may be capacity, not consumption.

The workload calculation spec will define exact formulas per resource behavior.

The demand model only requires that planned, remaining, reserved and actual
amounts are separately representable.

## Criticality

A demand should not only be `required: true` or `required: false`.

That binary flag is too weak.

Approved criticality values:

```text
required
preferred
optional
substitute
```

Meaning:

- `required` — the task cannot be completed without satisfying this demand;
- `preferred` — the demand is strongly desired, but there may be a degraded
  path without it;
- `optional` — the demand improves execution but is not necessary;
- `substitute` — the demand is one option among alternatives.

Examples:

```yaml
role: implementer
criticality: required
```

```yaml
role: ai_coding_agent
criticality: optional
```

```yaml
role: staging_environment
criticality: preferred
```

```yaml
role: implementation_option
criticality: substitute
```

For MVP, `required` and `optional` are enough for basic warnings, but the data
model should allow all four values.

## Demand status

Approved demand statuses:

```text
planned
requested
reserved
partially_consumed
consumed
released
cancelled
blocked
```

MVP may implement a smaller subset:

```text
planned
reserved
consumed
cancelled
blocked
```

Meanings:

- `planned` — the demand exists in the plan;
- `requested` — reservation or acquisition has been requested;
- `reserved` — capacity, quota, object, place, material or budget is reserved;
- `partially_consumed` — some actual consumption exists, but more remains;
- `consumed` — the planned resource need has been consumed or satisfied;
- `released` — a previous reservation is no longer held;
- `cancelled` — the demand is no longer needed;
- `blocked` — the task needs this demand, but it cannot currently be satisfied.

Demand status is separate from task tracking status.

Example:

```yaml
tracking_status: in_progress
resource_demands:
  - role: implementer
    status: partially_consumed
  - role: staging_environment
    status: blocked
```

The task can be in progress while one of its required future demands is blocked.

## Timing

Resource demands need timing information.

Minimum timing fields:

```yaml
timing:
  earliest_start: 2026-07-06
  latest_finish: 2026-07-09
  spread: even
```

Candidate spread modes:

```text
even
front_loaded
back_loaded
contiguous
manual
milestone
```

MVP should support:

```text
even
contiguous
manual
```

Meaning:

- `even` — spread the amount evenly over the available period;
- `contiguous` — resource is needed in an uninterrupted block;
- `manual` — allocation is specified directly by dates or buckets.

Other spread modes can be added later.

## Demand examples

### Software task

```yaml
task_id: openproject:123
assignee_hint: dev_ivan
resource_demands:
  - resource_ref:
      type: resource
      id: dev_ivan
    resource_class: human
    role: implementer
    unit: hours
    planned_amount: 8
    remaining_amount: 8
    reserved_amount: 0
    actual_amount: 0
    criticality: required
    status: planned
  - resource_ref:
      type: pool
      id: backend_reviewers
    resource_class: human
    role: reviewer
    unit: hours
    planned_amount: 1.5
    remaining_amount: 1.5
    reserved_amount: 0
    actual_amount: 0
    criticality: required
    status: planned
  - resource_ref:
      type: pool
      id: codex_agents
    resource_class: ai_agent
    role: ai_coding_agent
    unit: agent_hours
    planned_amount: 2
    remaining_amount: 2
    reserved_amount: 0
    actual_amount: 0
    criticality: optional
    status: planned
  - resource_ref:
      type: resource
      id: openai_api_project_quota
    resource_class: service_quota
    role: llm_token_budget
    unit: tokens
    planned_amount: 120000
    remaining_amount: 120000
    reserved_amount: 0
    actual_amount: 0
    criticality: preferred
    status: planned
```

### Offline repair task

```yaml
task_id: openproject:repair-17
resource_demands:
  - resource_ref:
      type: resource
      id: plumber_alex
    resource_class: human
    role: technician
    unit: hours
    planned_amount: 4
    remaining_amount: 4
    reserved_amount: 4
    actual_amount: 0
    criticality: required
    status: reserved
  - resource_ref:
      type: resource
      id: service_vehicle_1
    resource_class: vehicle
    role: transport
    unit: vehicle_hours
    planned_amount: 4
    remaining_amount: 4
    reserved_amount: 4
    actual_amount: 0
    criticality: required
    status: reserved
  - resource_ref:
      type: resource
      id: spare_pump_model_x
    resource_class: material
    role: replacement_part
    unit: pieces
    planned_amount: 1
    remaining_amount: 1
    reserved_amount: 1
    actual_amount: 0
    criticality: required
    status: reserved
```

### Procurement task

```yaml
task_id: openproject:procurement-44
resource_demands:
  - resource_ref:
      type: resource
      id: buyer_anna
    resource_class: human
    role: buyer
    unit: hours
    planned_amount: 2
    remaining_amount: 2
    reserved_amount: 0
    actual_amount: 0
    criticality: required
    status: planned
  - resource_ref:
      type: resource
      id: project_budget
    resource_class: budget
    role: purchase_budget
    unit: rub
    planned_amount: 18000
    remaining_amount: 18000
    reserved_amount: 18000
    actual_amount: 0
    criticality: required
    status: reserved
  - resource_ref:
      type: pool
      id: approved_suppliers
    resource_class: vendor
    role: supplier
    unit: deliveries
    planned_amount: 1
    remaining_amount: 1
    reserved_amount: 0
    actual_amount: 0
    criticality: required
    status: requested
```

### API-on-demand task

```yaml
task_id: openproject:data-import-9
resource_demands:
  - resource_ref:
      type: resource
      id: external_maps_api
    resource_class: service_quota
    role: geocoding_api_calls
    unit: api_requests
    planned_amount: 10000
    remaining_amount: 10000
    reserved_amount: 10000
    actual_amount: 0
    criticality: required
    status: reserved
  - resource_ref:
      type: resource
      id: external_maps_api_rate_limit
    resource_class: rate_limit
    role: geocoding_rate_limit
    unit: requests_per_minute
    planned_amount: 600
    remaining_amount: 600
    reserved_amount: 600
    actual_amount: 0
    criticality: required
    status: reserved
```

## Assignee handling

Source systems often provide an assignee.

RePlan should import assignee as:

```yaml
assignee_hint: dev_ivan
```

The adapter may create a default resource demand from this hint.

Example:

```yaml
source_assignee: dev_ivan
source_estimate: 8h
```

may become:

```yaml
resource_demands:
  - resource_ref:
      type: resource
      id: dev_ivan
    resource_class: human
    role: implementer
    unit: hours
    planned_amount: 8
    remaining_amount: 8
    criticality: required
    status: planned
```

This generated demand should be visible as adapter-derived or imported, not
magically hidden.

If later the operator adds reviewer, environment or quota demands, the source
assignee remains only one part of the resource model.

## Pools and unresolved demands

Resource demands can be unresolved.

Example:

```yaml
resource_ref:
  type: pool
  id: any_frontend_developer
```

This means:

```text
The task needs a frontend developer, but the exact person is not chosen.
```

For MVP, RePlan does not need automatic optimal assignment.

It should still report:

- unresolved required demands;
- pool demand load;
- possible overload if pool capacity is known;
- missing capacity data if pool capacity is unknown.

## Substitution and alternatives

Substitution is important but should not become an MVP solver problem.

Example:

```text
Option A: Ivan implements in 8 human hours.
Option B: Codex agent produces draft in 2 agent-hours, then Ivan reviews for 3
human hours.
```

The model should allow substitution groups or alternatives, but MVP may model
alternatives as scenarios.

Recommended MVP approach:

```text
base plan uses one explicit demand set
alternative demand sets are represented as scenarios
```

Later, RePlan can add richer substitution semantics.

The demand model should leave room for:

```yaml
substitution_group: implementation_strategy
```

or:

```yaml
alternative_of: demand-001
```

but MVP does not need to solve alternatives automatically.

## Relationship to actuals

Actual consumption should attach to a resource demand when possible.

Example:

```yaml
actual:
  demand_id: demand-001
  resource_id: dev_ivan
  unit: hours
  amount: 3
  timestamp: 2026-07-06T18:00:00+07:00
  evidence:
    - manual:progress-report-88
```

If actual consumption cannot be matched to a specific demand, it may attach to
the task and resource first, then be reconciled later.

This matters for AI-agent, quota and offline work where actuals may come from
different systems.

## Relationship to evidence and proposals

Resource demands may be created or changed by proposals.

Examples:

- propose adding reviewer demand;
- propose increasing remaining implementer effort;
- propose adding staging environment demand;
- propose adding OpenAI token demand;
- propose changing material quantity;
- propose marking a demand blocked;
- propose replacing a human demand with AI-agent plus reviewer demand.

Evidence may support demand changes.

Examples:

- pull request opened suggests reviewer demand should be scheduled;
- CI failure suggests extra implementer demand;
- API usage record supports actual quota consumption;
- delivery note supports actual material delivery;
- repair photo supports technician actuals and review-ready proposal.

## Reports

Resource demand reports should show:

- planned amount;
- remaining amount;
- reserved amount;
- actual amount;
- forecast total when applicable;
- variance from baseline;
- demand status;
- resource class;
- role;
- unit;
- criticality;
- unresolved demands;
- blocked required demands;
- over-capacity resources;
- missing actuals;
- evidence/proposal links when relevant.

Example report fragment:

```text
OP-123 Implement billing adapter
  implementer dev_ivan
    planned: 8h
    actual: 3h
    remaining: 7h
    forecast: 10h
    variance: +2h
  reviewer backend_reviewers
    planned: 1.5h
    reserved: 0h
    warning: required demand not reserved
  OpenAI API quota
    planned: 120000 tokens
    actual: 82000 tokens
    remaining: 50000 tokens
    warning: forecast exceeds planned quota
```

## MVP requirements

MVP should implement or model:

- `resource_demands[]` on tasks;
- concrete resource references;
- pool resource references;
- unresolved demand references;
- `resource_class`;
- `role`;
- `unit`;
- `planned_amount`;
- `remaining_amount`;
- `reserved_amount`;
- `actual_amount`;
- `criticality`;
- demand status;
- simple timing;
- evidence links;
- proposal links;
- warning for unresolved required demand;
- warning for required demand without capacity;
- warning for required demand that is blocked;
- workload reporting by resource, class, role and unit.

MVP should not implement:

- full automatic resource leveling;
- optimal assignment solver;
- complex substitution graph;
- payroll;
- full procurement ERP;
- full inventory accounting;
- full material warehouse management;
- full MS Project-grade scheduling;
- complex cost accounting;
- enterprise approval workflow.

## Approved decisions

- RePlan tasks support multiple resource demands.
- `assignee` is a PM-system compatibility hint, not the core planning model.
- `resource_demands[]` is the core planning model.
- A demand may point to a concrete resource, a pool or an unresolved resource
  need.
- Demands have resource class, role, unit, planned amount, remaining amount,
  reserved amount, actual amount, criticality, status and timing.
- Units are not limited to hours.
- Planned, remaining, reserved and actual amounts are separate fields.
- `forecast_total = actual_amount + remaining_amount` is a common calculation,
  but exact formulas depend on resource behavior.
- Criticality supports `required`, `preferred`, `optional` and `substitute`.
- MVP may implement a smaller operational subset but should keep the model open
  for the full set.
- Substitution and alternatives should mostly be handled through scenarios in
  MVP.
- Actual consumption should attach to resource demands when possible.
- Resource demand changes should be expressible as proposals.
