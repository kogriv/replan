# Resource abstraction

Date: 2026-07-02

Status: core architecture decision.

## Decision

RePlan should not try to predefine every possible class of resource.

RePlan should define a generic resource abstraction that can represent:

- people;
- AI agents;
- contractor capacity;
- equipment;
- physical robots;
- lab devices;
- deployment environments;
- API quotas;
- rate limits;
- LLM token budgets;
- cloud budgets;
- CI/build minutes;
- SaaS subscription seats;
- concurrent sessions;
- external data subscriptions;
- generic future constraints.

Short rule:

```text
anything that constrains work can be a resource
```

Resource kinds are templates, not the core model.

## Why not enumerate all resource classes

Trying to list all possible resource types is a losing strategy.

Reasons:

- new services appear quickly;
- AI tooling changes quickly;
- teams invent project-specific constraints;
- physical resources and software resources behave differently;
- subscriptions and quotas change by provider;
- resource semantics differ between industries;
- a rigid enum would force schema migrations for every new concept.

The core should stay stable while resource profiles evolve.

## Core model

Every resource should be describable through the same high-level parts:

```text
Resource
    identity
    kind/template
    unit
    capacity
    availability or reset period
    reservations
    consumption
    constraints
    policy
    gateway
    metadata
```

## Core concepts

### Resource

A resource is something that can limit, enable or constrain work.

Examples:

- Ivan's work hours;
- agent-coder-1 execution capacity;
- GPU server time;
- staging environment slot;
- OpenAI monthly budget;
- GitHub API hourly quota;
- browser automation concurrent sessions;
- contractor retainer hours.

### Unit

The unit defines what is being measured.

Examples:

- hours;
- person-hours;
- agent-hours;
- GPU-hours;
- requests;
- tokens;
- dollars;
- seats;
- concurrent sessions;
- build minutes;
- storage gigabytes;
- messages;
- jobs;
- reservations.

Units should be extensible.

Do not hardcode all units into business logic.

### Period

The period defines when capacity applies or resets.

Examples:

- day;
- week;
- month;
- billing cycle;
- calendar month;
- hour;
- rolling window;
- current subscription plan;
- custom fixed interval.

### Capacity

Capacity is how much of the resource exists in a period.

Examples:

```yaml
capacity:
  unit: hours
  amount: 24
  period: week
```

```yaml
capacity:
  unit: requests
  amount: 5000
  period: hour
```

```yaml
capacity:
  unit: usd
  amount: 500
  period: month
```

### Availability

Availability describes when a resource may be used.

This is most obvious for people, machines and environments.

Examples:

- work calendar;
- vacation;
- maintenance window;
- weekend capacity;
- deployment freeze;
- robot safety lockout;
- business hours;
- regional holiday.

### Reservation

Reservation is planned future use.

Examples:

- Ivan reserved for 12 hours next week;
- GPU reserved for 6 GPU-hours tomorrow;
- browser cloud reserves 2 concurrent sessions for test automation;
- LLM task reserves $20 of budget;
- GitHub connector reserves an estimated 1,000 API requests for import.

Reservation is planning.

It may be wrong.

### Consumption

Consumption is actual usage.

Examples:

- task consumed 9.5 human hours;
- agent consumed 83,000 LLM tokens;
- CI consumed 42 build minutes;
- GitHub sync consumed 1,320 API requests;
- staging environment was occupied for 2 hours.

Consumption is evidence.

### Constraint

Constraints define rules around use.

Examples:

- warning at 80 percent;
- hard limit at 100 percent;
- approval required above $50 per task;
- only one deployment at a time;
- no production deployment outside release window;
- no AI agent may consume production API keys;
- contractor hours cannot exceed retainer without approval;
- API calls must stay under provider rate limit.

### Policy

Policy says who may consume or reserve the resource.

Examples:

- operator may reserve any project resource;
- agent-coder-1 may use only approved LLM budget;
- GitHub connector may consume GitHub API quota;
- contractor may see only project-scoped tasks;
- robot controller may reserve a robot only through safety gateway.

Policy is about permission.

Resource is about capacity.

These must not be mixed.

### Gateway

Gateway connects RePlan to the outside world.

Examples:

- OpenAI usage gateway;
- GitHub API gateway;
- Kubernetes gateway;
- cloud billing gateway;
- robot controller gateway;
- CI provider gateway;
- browser automation provider gateway;
- HR calendar gateway.

Gateway operations may include:

- read entitlement;
- read current capacity;
- read current usage;
- read reset time;
- estimate cost;
- reserve planned use;
- dry-run operation;
- apply approved operation;
- record evidence;
- emit audit event.

Gateway is optional.

Manual resources can exist without a gateway.

## Resource kind is a template

`kind` should exist, but it should not drive all behavior.

Good use of `kind`:

- choose default fields;
- choose UI icon;
- choose default unit;
- choose default period;
- choose default validation;
- choose default gateway type;
- choose reporting group.

Bad use of `kind`:

- hardcode all logic by enum;
- require code changes for every new type;
- make custom resources impossible;
- treat people, agents and services as separate unrelated systems.

## Recommended built-in kinds

Initial built-in kinds:

- `human`;
- `ai_agent`;
- `equipment`;
- `environment`;
- `service_quota`;
- `budget`;
- `subscription_entitlement`;
- `generic`.

These are not exhaustive.

They are starter templates.

### human

Typical unit:

- hours.

Typical period:

- week or day.

Typical fields:

- timezone;
- work calendar;
- skills;
- role;
- vacation;
- default daily capacity;
- default weekly capacity.

Example:

```yaml
id: dev_ivan
kind: human
display_name: Ivan
capacity:
  unit: hours
  amount: 24
  period: week
availability:
  timezone: Asia/Novosibirsk
  calendar: default_work_calendar
skills:
  - python
  - frontend
policy:
  allowed_actors:
    - operator
```

### ai_agent

Typical units:

- hours;
- tasks;
- tokens;
- dollars.

Typical fields:

- tool family;
- trust level;
- allowed task types;
- review requirement;
- evidence requirement;
- cost model;
- rate limits;
- provider credentials reference.

Example:

```yaml
id: agent_coder_1
kind: ai_agent
display_name: Agent Coder 1
capacity:
  unit: hours
  amount: 30
  period: week
constraints:
  allowed_task_types:
    - docs
    - tests
    - low_risk_code
  requires_human_review: true
policy:
  allowed_actors:
    - agent_coder_1
    - operator
```

### equipment

Typical units:

- hours;
- machine-hours;
- reservations.

Examples:

- GPU server;
- test bench;
- robot;
- vehicle;
- lab device.

Example:

```yaml
id: gpu_server_1
kind: equipment
display_name: GPU server 1
capacity:
  unit: gpu_hours
  amount: 24
  period: day
availability:
  maintenance_windows:
    - start: 2026-07-10T01:00:00Z
      end: 2026-07-10T03:00:00Z
gateway:
  type: kubernetes_gpu_gateway
```

### environment

Typical units:

- slots;
- hours;
- concurrent deployments;
- reservations.

Examples:

- staging;
- production deployment window;
- QA environment;
- shared database sandbox.

Example:

```yaml
id: staging_env
kind: environment
display_name: Shared staging environment
capacity:
  unit: concurrent_sessions
  amount: 1
  period: current
constraints:
  no_parallel_deployments: true
gateway:
  type: deployment_gateway
```

### service_quota

Typical units:

- requests;
- tokens;
- messages;
- jobs;
- gigabytes.

Example:

```yaml
id: github_api_rate_limit
kind: service_quota
display_name: GitHub API rate limit
capacity:
  unit: requests
  amount: 5000
  period: hour
reset_policy: provider_reported
gateway:
  type: github_api_gateway
policy:
  allowed_actors:
    - github_connector
    - agent_coder_1
```

### budget

Typical units:

- dollars;
- euros;
- rubles;
- credits.

Example:

```yaml
id: openai_monthly_budget
kind: budget
display_name: OpenAI monthly budget
capacity:
  unit: usd
  amount: 500
  period: calendar_month
constraints:
  warning_threshold: 0.8
  hard_limit: 500
  approval_required_above: 50
gateway:
  type: openai_usage_gateway
policy:
  allowed_actors:
    - agent_coder_1
    - agent_research_1
    - operator
```

### subscription_entitlement

Typical units:

- seats;
- concurrent sessions;
- enabled features;
- plan entitlement.

Example:

```yaml
id: browser_cloud_plan
kind: subscription_entitlement
display_name: Browser cloud plan
capacity:
  unit: concurrent_sessions
  amount: 3
  period: current_plan
gateway:
  type: browser_cloud_gateway
policy:
  allowed_actors:
    - playwright_agent
```

### generic

Use when no built-in kind fits.

Example:

```yaml
id: custom_vendor_limit
kind: generic
display_name: Custom vendor daily processing limit
capacity:
  unit: jobs
  amount: 1000
  period: day
metadata:
  vendor: custom-vendor
```

## Resource state

Every resource can expose planning state.

Suggested state:

```yaml
state:
  available: 500
  reserved: 120
  consumed: 90
  remaining: 290
  reset_at: 2026-08-01T00:00:00Z
```

Not every field applies to every resource.

For a human, `reset_at` may not matter.

For an API quota, `availability.calendar` may not matter.

The schema should allow this without forcing fake values.

## Planned vs actual

RePlan should treat planned use and actual use separately.

For people:

```text
planned hours vs spent hours
```

For APIs:

```text
reserved requests vs consumed requests
```

For budgets:

```text
reserved dollars vs spent dollars
```

For subscriptions:

```text
reserved seats/sessions vs actual active seats/sessions
```

This keeps RePlan useful for both planning and post-fact analysis.

## Relationship to tasks

Tasks should reference resource demand.

Example:

```yaml
task_id: openproject:1234
resource_demands:
  - resource_id: dev_ivan
    unit: hours
    amount: 12
  - resource_id: openai_monthly_budget
    unit: usd
    amount: 15
  - resource_id: github_api_rate_limit
    unit: requests
    amount: 500
```

This is more general than a single `assignee_id`.

`assignee_id` remains useful, but it is not enough.

A task may require:

- one assignee;
- one reviewer;
- one AI agent;
- one staging environment;
- one API quota;
- one budget;
- one external subscription.

## Scheduling implications

The first workload algorithm can handle only time-based resources.

That is acceptable.

But the model should not prevent future checks such as:

- API quota exhaustion;
- monthly budget exhaustion;
- too many concurrent sessions;
- AI agent token budget exceeded;
- CI minutes depleted;
- staging environment conflict;
- robot maintenance conflict.

Do not implement every check now.

Do preserve the abstraction.

## UI implications

Resource UI should not assume all resources are people.

Views should eventually support:

- people workload;
- agent workload;
- equipment schedule;
- environment reservations;
- quota usage;
- budget burn;
- subscription seats;
- gateway health.

MVP can show only people and simple service resources.

## Policy implications

Policies should operate over resource IDs and capabilities, not only roles.

Example:

```yaml
policy:
  actor: agent_coder_1
  may_reserve:
    - openai_monthly_budget
    - github_api_rate_limit
  max_cost_per_task:
    unit: usd
    amount: 20
  requires_approval_above:
    unit: usd
    amount: 50
```

## Gateway implications

Gateway adapters should not define the resource abstraction.

They only provide provider-specific facts and operations.

Example:

```text
OpenAI gateway
    reads token/cost usage
    estimates future cost
    reports reset/billing period

GitHub gateway
    reads current API rate limit
    estimates import cost in requests

Kubernetes gateway
    reads GPU availability
    reserves workload slot if approved
```

Core RePlan should stay provider-independent.

## MVP implementation guidance

Implement the abstraction lightly.

Minimum schema fields:

- `id`;
- `kind`;
- `display_name`;
- `capacity.unit`;
- `capacity.amount`;
- `capacity.period`;
- `scope`;
- `policy.allowed_actors`;
- `gateway.type` optional;
- `metadata` free object.

Do not implement all gateway logic in MVP.

Do not build a resource-kind plugin system before it is needed.

Do not build full unit conversion before there are real examples.

Do include enough structure so that adding those later does not rewrite tasks,
resources and scenarios.

## Final position

RePlan should be resource-abstraction-first, not resource-class-enum-first.

The initial implementation should include useful built-in templates, but the
domain model should remain open:

```text
kind tells us the default behavior
unit tells us the math
capacity tells us the limit
reservation tells us intended use
consumption tells us actual use
gateway tells us how to talk to the outside world
policy tells us who may use it
audit tells us what happened
```

