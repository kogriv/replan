# TaskJuggler integration design

Date: 2026-07-02

Status: initial design.

## Core decision

TaskJuggler should be an optional advanced scheduling engine for RePlan.

TaskJuggler should not be required for the first useful RePlan workflow.

TaskJuggler should not be replaced prematurely.

## Why TaskJuggler matters

TaskJuggler already exists and solves hard scheduling problems that RePlan
should not immediately reimplement.

Important capabilities:

- tasks;
- resources;
- accounts;
- task dependencies;
- scheduling constraints;
- priorities;
- resource allocation;
- resource leveling;
- conflict detection;
- reports;
- project-as-code workflow.

This means TaskJuggler is a strong reference point and possible engine for the
parts of RePlan that are mathematically harder than simple workload summaries.

## Why TaskJuggler alone is not enough

TaskJuggler is powerful, but it does not cover the whole target workflow.

### It is not the team's task substrate

Software teams usually operate in:

- OpenProject;
- Jira;
- Plane;
- GitHub Issues;
- GitLab Issues;
- pull requests;
- CI;
- code review;
- comments;
- release artifacts.

TaskJuggler does not naturally own this operational state.

### It does not provide the needed adapter layer

RePlan needs to import from several task systems, normalize their data and
write back approved planning changes.

TaskJuggler does not primarily exist as a universal connector and approval
workflow layer.

### It does not provide an AI-safe governance interface

AI agents need constrained commands:

```bash
replan task list --assigned-to agent_coder_1
replan propose estimate openproject:1234 --remaining-hours 12
replan scenario compare baseline scenario-agent-proposal
replan apply scenario-agent-proposal --dry-run
```

Giving AI agents direct write access to `.tjp` files or free-form markdown is
too loose for operational planning.

### It is not optimized for weekly operator UX

The operator needs quick answers:

- who is overloaded;
- what is missing estimates;
- what deadlines are impossible;
- what changed since last baseline;
- what can move;
- what can be assigned to an agent;
- what requires approval.

TaskJuggler can produce reports, but RePlan should provide the product workflow
around those reports.

## Integration model

```text
OpenProject / Plane / GitHub / YAML
        |
        v
RePlan canonical model
        |
        +--> simple internal workload calculation
        |
        +--> TaskJuggler export
                    |
                    v
              TaskJuggler run
                    |
                    v
              TaskJuggler reports
                    |
                    v
           RePlan scenario import
                    |
                    v
          human review and approval
                    |
                    v
          optional write-back adapter
```

## RePlan without TaskJuggler

The MVP should work without TaskJuggler.

Minimum useful workflow:

```text
tasks + estimates + resources + calendars
    -> weekly workload table
    -> overload warnings
    -> missing data warnings
    -> dependency warnings
    -> markdown/CSV report
```

This is enough to answer the immediate need:

```text
who is overloaded and where is the plan unrealistic?
```

## RePlan with TaskJuggler

TaskJuggler becomes useful when the plan has:

- many dependencies;
- fixed deadlines;
- several constrained resources;
- parallel projects;
- non-trivial priority rules;
- need for automatic resource leveling;
- need for conflict-resolving schedule generation.

In that case RePlan should export:

- project start/end;
- tasks;
- task hierarchy;
- estimates;
- dependencies;
- priorities;
- resource definitions;
- calendars;
- allocations;
- milestones;
- constraints.

Capacity and quota mapping is intentionally loss-aware and is defined in
`docs/spec/taskjuggler-capacity-quota-mapping.md`.

The short rule:

```text
hour-based scheduling assumptions may become TaskJuggler input after validation
quota, budget, rate-limit and subscription assumptions stay in RePlan workload
```

TaskJuggler should calculate a schedule. RePlan should import it as a scenario,
not as truth.

The run/import experiment boundary is defined in
`docs/spec/taskjuggler-run-import-experiment.md`.

The first run-only slice is implemented as `replan taskjuggler run`. It
generates `.tjp`, invokes a real TaskJuggler binary, stores stdout, stderr,
version and manifest artifacts, and leaves canonical RePlan state unchanged.
It does not parse or import TaskJuggler results yet.

The first parseable TaskJuggler report contract is defined in
`docs/spec/taskjuggler-parseable-report-contract.md`. The contract uses a
minimal TaskJuggler CSV task report plus a RePlan-generated ID mapping file,
instead of parsing TaskJuggler HTML or stdout.

## Scenario handling

A TaskJuggler run should produce a RePlan scenario:

```yaml
scenario_id: tj-2026-07-02-001
source: taskjuggler
source_snapshot_id: snapshot-2026-07-02-base
description: TaskJuggler resource-leveled schedule
changes:
  tasks:
    - canonical_id: openproject:1234
      start_date:
        from: 2026-07-06
        to: 2026-07-10
      finish_date:
        from: 2026-07-09
        to: 2026-07-15
reason:
  - resource leveling
  - dependency constraint
status: proposed
```

Human review remains required.

## Parity target

RePlan should not claim TaskJuggler parity immediately.

However, RePlan should use TaskJuggler as a planning capability benchmark.

Long-term parity categories:

- work breakdown structure;
- task hierarchy;
- dependencies;
- milestones;
- resources;
- resource calendars;
- allocation;
- estimates;
- effort;
- priorities;
- fixed start and fixed finish constraints;
- deadline constraints;
- automatic schedule calculation;
- resource leveling;
- overload/conflict reports;
- baseline vs scenario comparison;
- progress tracking;
- cost/accounting later;
- generated reports.

## Implementation phases

### Phase 1: internal workload only

No TaskJuggler required.

Goal:

- prove RePlan reports are useful.

### Phase 2: export-only TaskJuggler proof of concept

Generate `.tjp` from a RePlan snapshot.

Goal:

- verify that RePlan canonical model can express enough scheduling data.

### Phase 3: run TaskJuggler from CLI

Command:

```bash
replan taskjuggler run --scenario scenario.ivan-capacity-6
```

Goal:

- automate the scheduling experiment;
- save `.tjp`, stdout, stderr, version and manifest artifacts;
- keep canonical state unchanged.

Status: implemented as run-only artifact capture. Result parsing and
scenario/proposal import remain later phases.

### Phase 4: import TaskJuggler result as scenario

Goal:

- compare TaskJuggler output against baseline in RePlan.
- generate scenario/proposal candidates only after run artifacts are stable.

### Phase 5: approval and write-back

Goal:

- write approved dates/comments/custom fields back to source system.

## Risks

### Model mismatch

TaskJuggler has its own model. OpenProject, Plane and GitHub have different
models. RePlan must not pretend mapping is lossless.

Mitigation:

- keep raw source metadata;
- emit warnings for unmapped fields;
- keep generated `.tjp` as artifact;
- import results as proposals.

### Brittle report parsing

Parsing human-readable reports can break.

Mitigation:

- prefer structured exports where possible;
- parse the dedicated CSV task report defined by RePlan, not a human HTML
  report;
- require a RePlan-generated TaskJuggler-to-RePlan ID mapping artifact;
- keep parsing code small and well-tested;
- pin TaskJuggler versions in tests if needed.

### Over-automation

Automatic schedules can look mathematically valid and operationally wrong.

Mitigation:

- scenario review;
- dry-run;
- human approval;
- keep explanations and assumptions visible.

## Final position

TaskJuggler is not a competitor that makes RePlan unnecessary.

TaskJuggler is a powerful engine that RePlan can make usable in a broader
software-team and AI-agent workflow.

Short formula:

```text
TaskJuggler = scheduling engine
RePlan = integration, resource, scenario, reporting and approval layer
```
