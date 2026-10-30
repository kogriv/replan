# RePlan Project State: Test project

- Project: `project.test`
- Week: `2026-W30`
- Status date: `2026-07-20`
- Current plan: `current`
- Baseline: `baseline.1`

## Sync Health
- Source profiles: 1
- Tasks with source refs: 2
- Tasks without source refs: 0
- Resources with source refs: 0

## Planning Health
- Tasks: 2
- Accepted tasks: 1
- Active tasks: 1
- Open dependencies: 1
- Related links: 0
- Missing-demand tasks: 0
- Open proposals: 1
- Scenarios: 1

## Tracking Health
- Evidence records: 1
- Warnings: 1
- Errors: 0

## Plan vs Actual
| Task | Status | Actuals | Evidence | Flags |
|---|---|---:|---:|---|
| `task.1` | accepted | 1 | 1 | forecast_over_plan |

| Task | Demand | Unit | Planned | Actual | Remaining | Forecast | Variance | Flags |
|---|---|---:|---:|---:|---:|---:|---:|---|
| `task.1` | `resource_demand.task-1.ivan` | hours | 6 | 2 | 5 | 7 | 1 | forecast_over_plan |

## Tracking Proposal Loop
- Stale remaining rows: 1
- Open tracking proposals: 1
- Accepted tracking proposals: 0
- Covered demand rows: 1
- Actionable demand rows: 0

| Task | Demand | Status | Priority | Reason | Proposal | Remaining | Forecast | Next |
|---|---|---|---|---|---|---:|---:|---|
| `task.1` | `resource_demand.task-1.ivan` | open | medium | proposal already open | `proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero` | 5 | 7 | `replan proposal accept --project . proposal.tracking.resource_demand-task-1-ivan.set-remaining-zero --dry-run --format json` |
| `task.3` | `resource_demand.3` | actionable | - | - | `-` | 0 | 0 | `-` |

## Resource Pressure
| Resource | Unit | Capacity | Planned | Actual | Remaining | Overload |
|---|---:|---:|---:|---:|---:|---:|
| `resource.unnamed` | hours | 2 | 5 | 0 | 5 | 3 |

## Warnings
- `required_dependency_unresolved` `dependency.1`: Dependency is open.
- `untargeted_warning`: No target.

## Next Actions

### Waiting on a person

- `accept_or_return_task` `task.2` (2026-07-17): Review the result, then accept the task or return it to work.
- `review_overload` `resource.human.ivan`: Review overload.
  - `replan workload --project . --resource resource.human.ivan --format json`

### An agent can prepare

- `review_tracking` `task.1` (2026-07-10): Review actuals.
  - `replan actual list --project . --task task.1 --format json`
- `inspect_project`: Nothing waits.
