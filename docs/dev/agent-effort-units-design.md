# Estimate hours and real agent hours

Status: proposal, 2026-11-04. Waits on the maintainer's decision.

## The problem

People estimate agent work the way they estimate their own: "this is about
four hours". Agents then spend a fraction of that. In the self project the
median actual is 0.21 of the estimate (39 clean tasks, see
`estimate-calibration-design.md`); small tasks take about 0.5 of the
estimate, large ones about 0.16.

A new consumer wrote its estimates as people think of them, in agent hours,
and found two things:

1. **Load.** `workload` compares open demand with capacity in the same
   unit. With honest capacity (what the agent can really work in a week),
   the plan shows overload that will not happen. To avoid it the consumer
   set capacity in estimate hours: 100 a week for one agent, 50 for
   another. Every project will invent its own number.
2. **OpenProject.** A work package's estimated time comes only from
   demands in `hours`. Agent work is not in it, which is right, since it
   would mix people's hours with agent hours. But a person in OpenProject
   then sees no agent effort except on the RePlan pages, which do show each
   demand with its unit.

The calibration machinery already exists: a sealed calibration record, a
factor per size band, and `estimate apply`, which writes a scenario. What
is missing is a way to use it for load without rewriting the plan, and a
starting factor for a project that has no sample yet.

## Options

### A. Convention only

Estimate in agent hours, set capacity in the same estimate hours, and write
down the convention per project. No code.

- For: nothing to build.
- Against: capacity loses its meaning (40 real hours become "100"); the
  number is a guess that never improves; projects disagree; reports and
  calibration speak about different hours.

### B. Calibrated load (recommended)

Keep estimates and capacity honest: estimates as people write them,
capacity in real agent hours. Load uses the calibration when one exists:

- `workload`, `report weekly`, `report state` and the dashboard compare
  calibrated open demand of an agent resource with its real capacity. The
  factor is the one `estimate apply` uses today (`CalibrationFactor`, per
  size band). Each row says which calibration was applied, and the raw
  load stays available (`--raw`, and a column in JSON).
- Only an approved, sealed calibration whose scope covers the resource and
  unit applies. Without one, nothing changes.
- **A starting factor.** A new project has no sample. Allow a provisional
  calibration with a factor a person enters and a cited source, for
  example `replan estimate calibrate --prior 0.25 --source "another project,
  39 tasks, 2026-11-02" ...`, sealed and approved like any calibration,
  marked `method: prior`. `replan next` suggests replacing it as soon as
  the project's own sample reaches the minimum.
- Planned amounts in the plan are not changed; `estimate apply` stays the
  way to rewrite them on purpose.

- For: capacity keeps its meaning; the factor improves with every closed
  task; one rule for all projects; uses records and lookups that exist.
- Against: load becomes a calculated number, so every view must label it;
  dates drafted from load (TaskJuggler export) must use the same
  calibrated amounts, or the plan and its dates disagree. This is the main
  work.

### C. Pace per resource

A field on the resource, `pace: 0.25`, that load multiplies by.

- For: very small change.
- Against: an unaudited number on a resource that drifts from the measured
  one; duplicates the calibration with a weaker record; one pace for every
  task size, which the size bands showed to be wrong.

### D. Apply the calibration to the plan

Use what exists: `estimate apply` writes a scenario with scaled amounts, a
person applies it, the plan holds calibrated amounts. Originals stay in the
baselines.

- For: works today.
- Against: every new task has to be scaled again; the plan's amounts stop
  being what people estimated, so the next calibration compares scaled
  amounts with actuals unless baselines are taken before every apply.

## OpenProject

Agent effort should be visible in OpenProject without mixing it into
people's estimated time:

- The RePlan pages already show each demand with its unit and amount. Add
  an "Agent hours" line to the work package's RePlan tab: planned, and
  calibrated when option B is chosen.
- Optional, later: a read-only custom field "Agent hours" filled by a
  helper like `work-package replan-links`, if people want it in the work
  package list. It needs an administrator once, as the link field did.
- `estimatedTime` stays hours of people only.

## Recommendation

Option B with a starting factor, and the RePlan tab line in OpenProject.
Build it in three steps, each its own task:

1. calibrated load in `workload` and the reports, labelled, with `--raw`;
2. `estimate calibrate --prior` and the `next` suggestion to replace it;
3. the same calibrated amounts in the TaskJuggler export and drafted
   dates, and the "Agent hours" line on the RePlan tab.

Until then a project can use option A as a stopgap, but should say so in
its plan README.

## Decision

Pending. The maintainer picks an option; the chosen steps become tasks.
