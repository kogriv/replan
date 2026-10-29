# Estimate calibration design

Date: 2026-10-28

Status: proposed; waits for the operator's approval
(`task.self.design-estimate-calibration`). Implementation is
`task.self.implement-estimate-calibration`.

## Purpose

`replan report estimates` answers how far the estimates of a sealed baseline
missed. This design turns that answer into the next plan: a recorded,
reviewable calibration that people can apply to open work, without RePlan
ever rewriting an estimate on its own.

## What the data says today

Self-dogfood project, `baseline.2026-10-15.tooling-consolidation`: 38 tasks,
all accepted, estimated before any of them started.

- Agent work, executor and Codex together: 138 agent hours estimated, 35.27
  used, a total ratio of 0.26.
- Per task the ratio of actual to baseline agent hours has a median of 0.23,
  a mean of 0.28 and a range of 0.09 to 0.83.
- Larger estimates miss more: tasks estimated at 2 hours used a median 0.39
  of the estimate, at 3 hours 0.26, at 4 hours 0.18, at 6 hours 0.13.
- Tags are too thin to calibrate on their own: only `agent-tooling` (38
  tasks) and `openproject-helpers` (10) reach a usable sample.
- The maintainer, the CI and the local environment have no recorded actuals
  for these tasks, so nothing can be said about them.
- The first six tasks (15–16 October) were booked by judgement, not from
  timestamps.

`baseline.2026-10-28.oss-checkpoint` covers 149 tasks, but it was taken
after most of them finished: its amounts are the current plan, including
re-estimates made during the work, and many older actuals were booked when
the demand was closed. Its executor ratio of 0.67 measures bookkeeping, not
estimating. A calibration must therefore only learn from estimates that were
made before the work started.

## Decisions

### 1. Calibrate per resource group, not per task type

The scope of a calibration is one resource group and one unit:

- a resource, for example `resource.human.maintainer` in `hours`;
- or a split group, the original resource together with the resources split
  out of it, for example `resource.ai.agent.executor` with
  `resource.ai.codex.local` in `agent_hours`. The estimate was made for the
  original resource before the split, so only the group is comparable.

An optional tag narrows the scope when that tag alone has at least the
minimum sample. Task types are not used: in the self project almost every
task is `implementation`.

### 2. The statistic is the median per-task ratio, with the spread shown

For each sample task: ratio = actual amount / baseline amount for the scope
group. The calibration factor is the median of those ratios. The record also
keeps the total ratio, the 25th and 75th percentiles, the count, and the ratio
per estimate-size band (up to 2, up to 4, above 4 in the scope's unit), so a
reader sees the size effect.

The median is used because one badly booked task cannot move it far. The
size effect is reported but not applied in the first version: a band has too
few tasks today, and a factor per band would fit noise. Applying factors per
size band is a later decision once each band has the minimum sample.

### 3. Only clean samples count

A task enters the sample only when all of these hold:

- it is finished (accepted, done or closed);
- the baseline was approved before the task's actual start, as derived by
  `replan task record-actual-window`'s rule; a task started before the
  baseline is excluded with reason `estimated_after_start`;
- it has counted actuals in the scope's unit; a task without them is
  excluded with `no_actuals` (not counted as zero);
- its baseline amount in the scope is above zero;
- it is not on the calibration's explicit exclusion list. The list names
  tasks with a reason, for example the six tasks of 15–16 October booked by
  judgement (`actuals_judged`).

The minimum sample is 8 tasks. Below it, `calibrate` refuses with
`calibration_sample_too_small`, naming the count and the excluded tasks.

### 4. A calibration is a sealed, approved record

`calibrations/calibration.<date>.<slug>.yaml`, sealed with sha256 like a
baseline, never edited after it is written:

```yaml
calibration:
  id: calibration.2026-10-28.agents
  name: Agent estimates after the tooling consolidation
  approved_by: {kind: actor, id: actor.human.maintainer}
  approved_at: "2026-10-28T19:00:00+07:00"
  source_baselines:
    - id: baseline.2026-10-15.tooling-consolidation
      seal_sha256: "..."
  scope:
    resources: [resource.ai.agent.executor, resource.ai.codex.local]
    unit: agent_hours
    tag: ""
  sample:
    tasks: [task.self.unify-helper-json-envelope, ...]
    excluded:
      - {task: task.self.example, reason: actuals_judged}
  result:
    factor: 0.23
    total_ratio: 0.26
    p25: 0.15
    p75: 0.33
    count: 32
    size_bands:
      - {up_to: 2, count: 8, median: 0.39}
  seal_sha256: "..."
```

The numbers above are illustrative. `approved_by` must be a human actor: an
agent may run the dry run and propose a calibration, but writing one is an
approval like a baseline.

### 5. Applying a calibration is a scenario

`replan estimate apply --calibration <id>` writes a scenario, not a plan
change. The scenario multiplies `planned_amount` and `remaining_amount` of
every open demand in the scope by the factor (rounded to a quarter of the
unit, never below a quarter), and lists the demands it skipped: closed ones,
those with actuals already above the scaled amount, and those whose amount a
person set after the calibration date.

The existing scenario workflow then does the rest: `scenario diff` shows the
load and date effect, and `scenario apply` (a person, with audit) makes it
the plan. The original estimates stay in the baselines, and the scenario
records the calibration id, so `report estimates` can later tell calibrated
estimates from raw ones.

A scenario is preferred over one proposal per demand because the effect of a
calibration is a whole-plan question: a person wants to see the new load and
dates once, and accept or reject them together.

### 6. New estimates see the factor, but are not changed

When a calibration exists for a scope, `replan task add` and `replan next`
show the calibrated value next to a new demand in that scope
("4 agent_hours, calibrated 0.92"). The demand keeps the amount that was
typed. Estimating stays a human or agent decision; RePlan shows the
evidence.

### 7. Reports and the next list

- `replan report estimates` gains a section per calibration that used the
  baseline: factor, spread and size bands.
- `replan next` suggests a new calibration when at least 8 finished tasks
  in a scope have clean samples that no calibration has used yet.

## Commands

```text
replan estimate calibrate --project <path> --baseline <id> [--baseline <id> ...]
    --resource <id> [--resource <id> ...] --unit <unit> [--tag <tag>]
    [--exclude <task-id>=<reason> ...] --id <id> --name <name>
    --approved-by <actor-id> (--dry-run|--apply) [--format json|markdown]
replan estimate list|show --project <path> [<id>] [--format json|markdown|text]
replan estimate apply --project <path> --calibration <id> --scenario-id <id>
    --actor <id> (--dry-run|--apply) [--format json|markdown]
```

Refusals: `baseline_not_found` (4), `baseline_has_no_estimates` and
`baseline_modified` (2) as in `report estimates`;
`calibration_sample_too_small` (2); `calibration_modified` (2) when the seal
does not verify; `approver_not_human` (3, policy denied).

## Not in the first version

- factors per estimate-size band or per tag combination;
- calibrating durations or dates: the schedule follows from amounts and
  capacity;
- learning across projects;
- any automatic application.

## Operator decisions

Agreed by the operator on 2026-10-29:

1. The minimum sample stays at 8 tasks.
2. The six tasks of 15–16 October are excluded as `actuals_judged`.
3. A calibration is applied through one scenario, not one proposal per
   demand.
4. One rounding rule, a quarter of the unit, for every unit in the first
   version.
