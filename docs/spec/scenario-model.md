# Scenario model

Date: 2026-07-06

Status: approved functional design.

## Purpose

This document defines how scenarios work in RePlan.

It answers:

- what a scenario is;
- what a scenario may change;
- what a scenario must never change;
- how scenario comparison works;
- whether a scenario can be created from tracking events;
- what makes a scenario accepted, rejected or applied;
- how scenarios support AI-CLI-first workflows.

This document is functional design. The exact canonical schema will be defined
later in `docs/spec/canonical-model.md`.

## Core decision

A scenario is an isolated what-if overlay.

Approved rule:

```text
scenario = isolated what-if overlay
```

A scenario changes planning assumptions.

A scenario does not change execution facts.

The scenario is calculated and compared safely before any change is applied to
the current plan.

## Overlay, not project copy

RePlan should not model a scenario as a full project clone.

Full project copies become hard to review:

- it is not obvious what changed;
- AI-generated scenarios are hard to inspect;
- diffing is noisy;
- applying only part of a scenario is difficult;
- storage becomes wasteful;
- source synchronization becomes confusing.

RePlan should model scenarios as overlays:

```yaml
scenario:
  id: scenario-001
  base_ref:
    type: current_plan
    id: current
  changes:
    - op: set
      path: tasks.openproject:123.demands.implementer.remaining_amount
      value: 12
    - op: add
      path: tasks.openproject:123.resource_demands
      value:
        resource_ref:
          type: pool
          id: codex_agents
        resource_class: ai_agent
        role: ai_coding_agent
        unit: agent_hours
        planned_amount: 2
        remaining_amount: 2
        criticality: optional
    - op: set
      path: tasks.openproject:123.deadline
      value: 2026-07-15
```

The exact patch format may change.

The approved concept is:

```text
scenario = base reference + explicit changes + calculated result
```

## Scenario base

A scenario may be based on:

- current plan;
- baseline;
- another scenario;
- imported source snapshot;
- tracking event set;
- manually supplied fixture.

For MVP, the primary base is:

```text
current plan
```

Baseline-based scenarios are useful for "what if we had planned differently"
analysis, but are less urgent.

## What a scenario can change

A scenario may change planning assumptions.

Allowed scenario changes:

- include task;
- exclude task;
- change planned amount;
- change remaining amount;
- change resource demand;
- add resource demand;
- remove resource demand;
- change resource assignment;
- change resource pool;
- change capacity assumption;
- add availability exception;
- remove availability exception;
- change task dates;
- change deadline;
- change dependency assumption;
- change dependency resolution date;
- change quota assumption;
- change budget assumption;
- change subscription capacity;
- change rate limit;
- change material availability;
- change environment availability;
- change AI-agent availability;
- change offline resource availability;
- change scenario-specific warning policy.

Examples:

- Ivan is available only 50%;
- add Codex agent pool to implementation;
- remove task from MVP;
- supplier response is delayed by three days;
- OpenAI quota is reduced;
- deadline moves by one week;
- reviewer pool gains one extra reviewer;
- staging environment is unavailable tomorrow;
- material delivery is delayed.

## What a scenario cannot change

A scenario must not change:

- accepted actuals;
- historical evidence;
- old baselines;
- source-system history;
- audit log;
- immutable event records;
- accepted status history;
- original imported source snapshot.

Reason:

```text
scenarios explore possible futures; they do not rewrite what already happened.
```

If a scenario needs to account for a different actual value, it should model
that as a planning assumption or proposal, not mutate accepted actuals.

## Scenario types

Approved scenario types:

```text
what_if
replan_proposal
recovery
capacity_change
scope_change
deadline_change
agent_strategy
```

### what_if

General exploration.

Example:

```text
What if we delay the deadline by one week?
```

### replan_proposal

Scenario created as a concrete proposed change to current plan.

Example:

```text
Move Task B after Task A and add reviewer demand.
```

### recovery

Scenario created after execution drift, blocker, actual overrun or risk.

Example:

```text
Actual work exceeded baseline; try a recovery plan with reduced scope.
```

### capacity_change

Scenario focused on resource availability or capacity.

Example:

```text
Ivan works 4h/day next week.
```

### scope_change

Scenario focused on adding, removing or changing tasks.

Example:

```text
Remove optional report export from MVP.
```

### deadline_change

Scenario focused on deadlines or delivery windows.

Example:

```text
Move external deadline from 2026-07-12 to 2026-07-19.
```

### agent_strategy

Scenario focused on using AI agents or changing agent/human split.

Example:

```text
Use Codex for first implementation draft and human reviewer for acceptance.
```

## Scenario lifecycle

Approved lifecycle:

```text
draft
calculated
reviewed
accepted
rejected
applied
archived
```

### draft

Scenario exists but has not been calculated yet.

### calculated

RePlan has calculated workload, warnings and comparison for the scenario.

### reviewed

A human or trusted actor has reviewed scenario output.

### accepted

The scenario is accepted as a decision direction.

Accepted does not mean applied.

### rejected

The scenario will not be used.

### applied

Selected scenario changes were applied to current plan.

### archived

Scenario is kept for history but no longer active.

## Accepted versus applied

This distinction is mandatory:

```text
accept scenario != apply scenario
```

A scenario may be accepted as the preferred strategy but applied later or only
partially.

For MVP, applying a scenario may be simplified:

```text
apply scenario -> selected changes become current plan proposals or current
plan updates
```

Applying a scenario does not change baseline.

If a new approved plan is needed:

```text
apply scenario -> current plan
create new baseline -> baseline_v2
```

Creating a new baseline is a separate explicit action.

## Scenario calculation

Scenario calculation should be read-only.

It should:

- apply overlay to base in memory or temporary calculation state;
- calculate workload;
- calculate capacity fit;
- calculate warnings;
- calculate forecast;
- compare with current plan;
- compare with baseline when available;
- produce machine-readable and human-readable output.

It should not:

- mutate current plan;
- mutate baseline;
- mutate actuals;
- mutate source-system data;
- write back to OpenProject, GitHub or GitLab.

## Scenario comparison

Scenario report should support:

```text
scenario vs current
scenario vs baseline
scenario vs actual/forecast
```

Minimum comparison fields:

- effort delta;
- schedule delta;
- capacity overload delta;
- quota delta;
- budget delta;
- material delta;
- deadline risk delta;
- unresolved dependency delta;
- warnings added;
- warnings removed;
- tasks added;
- tasks removed;
- resource changes;
- new required demands;
- removed demands;
- unknown values introduced.

Example:

```text
Scenario: Use Codex for import adapter
Against: current

Effort:
  human implementer: -4h
  human reviewer: +2h
  AI-agent: +2 agent_hours
  token quota: +120000 tokens

Schedule:
  forecast finish: -1 day

Warnings:
  removed:
    - Ivan overloaded on 2026-07-08
  added:
    - OpenAI token quota forecast exceeds baseline
```

## Scenario from tracking events

A scenario may be created from tracking events.

Examples:

- actual effort exceeds baseline;
- remaining work increased;
- AI agent proposes a different resource split;
- supplier delay event arrives;
- API quota is nearly exhausted;
- person becomes unavailable;
- CI failure creates rework;
- offline task evidence shows incomplete work;
- dependency becomes blocked.

Such scenario types are usually:

```text
recovery
replan_proposal
capacity_change
agent_strategy
```

Important rule:

```text
tracking event may create scenario or proposal; it does not silently apply it.
```

## Scenario proposals

A scenario may itself be a proposal.

Examples:

- propose applying scenario;
- propose applying selected scenario changes;
- propose creating new baseline from current after applying scenario;
- propose rejecting scenario because it worsens deadline risk;
- propose converting scenario into a task-level replan proposal.

For AI agents, the safe default is:

```text
agent can create, calculate and propose scenario; applying requires explicit
permission or accepted policy.
```

## AI-CLI-first behavior

Scenarios are a primary AI-CLI-friendly workflow.

Candidate commands:

```bash
replan scenario create --from current --name "Use Codex for import adapter"
replan scenario patch scenario-001 --add-demand codex_agent_pool:2agent_hours
replan scenario calculate scenario-001 --format json
replan scenario diff scenario-001 --against current --format markdown
replan scenario propose-apply scenario-001
replan scenario apply scenario-001 --dry-run
```

AI agents should prefer:

- create scenario;
- calculate scenario;
- inspect warnings;
- produce diff;
- create proposal.

They should not silently apply scenarios by default.

## Scenario and actuals

Scenarios cannot edit accepted actuals.

A scenario may include hypothetical future actual-like assumptions only if they
are explicitly marked as assumptions.

Example:

```yaml
scenario_assumption:
  type: expected_actual_consumption
  resource_id: openai_api_project_quota
  unit: tokens
  amount: 120000
```

This is not an actual.

It is a scenario assumption used for forecast calculation.

## Scenario and baseline

Scenarios do not mutate baselines.

A scenario can be compared with a baseline.

A scenario can be used to create a new current plan.

A current plan can then be approved as a new baseline.

But each step is explicit:

```text
calculate scenario
apply scenario to current plan
create new baseline from current plan
```

## Scenario and source systems

Scenarios do not write to source systems.

If scenario application implies a source-system change, RePlan should create a
proposal or write-back plan.

Examples:

- update OpenProject due date;
- change assignee;
- add task relation;
- add comment with scenario decision;
- create linked task.

Write-back is separate from scenario calculation.

## MVP requirements

MVP should implement or model:

- scenario identifier;
- scenario name;
- scenario type;
- base reference;
- explicit scenario changes;
- scenario status;
- scenario calculation result;
- scenario warnings;
- scenario diff against current plan;
- scenario diff against baseline when baseline exists;
- reject scenario;
- apply scenario explicitly;
- keep scenario read-only until apply;
- ensure scenario does not mutate actuals or baseline;
- machine-readable scenario output.

MVP should not implement:

- full project copy per scenario;
- collaborative scenario editor;
- automatic scenario application;
- multi-level scenario merge;
- optimization solver;
- probabilistic simulation;
- Monte Carlo;
- hidden mutation of current plan;
- source-system write-back from scenario calculation.

## Relationship to other documents

This document depends on:

- [`planning-lifecycle.md`](planning-lifecycle.md);
- [`resource-demand-model.md`](resource-demand-model.md);
- [`capacity-availability-rules.md`](capacity-availability-rules.md);
- [`workload-calculation.md`](workload-calculation.md);
- [`planned-current-actual-comparison.md`](planned-current-actual-comparison.md);
- [`dependency-deadline-behavior.md`](dependency-deadline-behavior.md);
- [`tracking-events.md`](tracking-events.md).

The next design item is:

```text
J. Evidence and proposal behavior
```

## Approved decisions

- A scenario is an isolated what-if overlay.
- A scenario is not a full project copy.
- A scenario is a base reference plus explicit changes plus calculated result.
- Scenarios can change planning assumptions.
- Scenarios cannot change accepted actuals, historical evidence, old baselines,
  source-system history, immutable event records, accepted status history or
  audit log.
- Scenario calculation is read-only.
- Scenario application is explicit.
- Accepting a scenario is not the same as applying it.
- Applying a scenario changes current plan or creates current-plan proposals,
  not baseline.
- Creating a new baseline after scenario application is a separate explicit
  action.
- Tracking events may create scenarios or proposals, but must not silently apply
  them.
- AI agents may create, calculate and propose scenarios by default, but should
  not silently apply them.
