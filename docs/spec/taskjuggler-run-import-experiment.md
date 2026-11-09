# RePlan TaskJuggler run and import experiment

Date: 2026-07-13

Status: approved design for future implementation.

## Purpose

This document defines the first TaskJuggler run/import experiment for RePlan.

It answers:

- how RePlan should run TaskJuggler after generating `.tjp`;
- where TaskJuggler run artifacts should be stored;
- why the TaskJuggler result must not become canonical truth;
- how a TaskJuggler result can become a RePlan scenario candidate later;
- which command should exist first;
- what should not be implemented in the first run/import slice.

This document depends on:

- `docs/architecture/taskjuggler-integration.md`;
- `docs/spec/taskjuggler-capacity-quota-mapping.md`;
- `docs/spec/taskjuggler-parseable-report-contract.md`;
- `docs/spec/export-artifacts.md`;
- `docs/spec/scenario-model.md`;
- `docs/spec/evidence-proposal-behavior.md`;
- `docs/spec/cli.md`;
- `docs/spec/implementation-sequence.md`.

## Core decision

TaskJuggler run/import must be scenario-first and evidence-first.

Approved rule:

```text
RePlan state -> generated .tjp -> TaskJuggler run artifact -> RePlan scenario/proposal candidate
```

TaskJuggler output is never canonical state by itself.

TaskJuggler output may become:

- evidence;
- a generated artifact;
- a scenario candidate;
- a proposal for human review;
- a comparison input.

TaskJuggler output must not directly mutate:

- `canonical/tasks.yaml`;
- `canonical/resource_demands.yaml`;
- `canonical/capacities.yaml`;
- `canonical/current_plan.yaml`;
- `baselines/`;
- source PM systems.

## Why this experiment is separate from validation

Current validation command:

```bash
replan dev validate-taskjuggler --project examples/small-team --scenario scenario.ivan-capacity-6
```

Purpose:

```text
Does generated .tjp parse and schedule under real TaskJuggler?
```

Run/import experiment purpose:

```text
Can RePlan use TaskJuggler output as a reviewable scheduling scenario?
```

These are different workflows.

Validation is a developer safety check.

Run/import is a planning workflow.

## Proposed command family

Use a dedicated command family:

```bash
replan taskjuggler run
replan taskjuggler import-result
```

Do not overload:

```bash
replan dev validate-taskjuggler
```

Reason:

- `dev validate-taskjuggler` is allowed to skip when `tj3` is missing;
- `taskjuggler run` is a real planning experiment and should fail if the
  requested TaskJuggler binary cannot run;
- `dev validate-taskjuggler` should remain small and CI-friendly;
- run/import needs artifact management, scenario identity and review metadata.

## First command: `replan taskjuggler run`

Purpose:

Generate `.tjp`, run TaskJuggler, and save run artifacts.

Command shape:

```bash
replan taskjuggler run \
  --project examples/small-team \
  --scenario scenario.ivan-capacity-6 \
  --output-dir exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z \
  --taskjuggler-bin tj3 \
  --unrepresented-assumptions error \
  --format json
```

Flags:

```text
--project
--scenario
--output-dir
--taskjuggler-bin
--unrepresented-assumptions warn|error
--format json|markdown|text
```

Default output directory:

```text
exports/taskjuggler/<scope>/run.<timestamp>/
```

Scope:

```text
current
scenario.<scenario_id>
```

Recommended artifacts:

```text
exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z/
  project.tjp
  run.manifest.json
  stdout.txt
  stderr.txt
  taskjuggler.version.txt
  taskjuggler.mapping.json
  reports/
    replan-task-schedule.csv
```

If TaskJuggler produces report files later, they should live under:

```text
exports/taskjuggler/<scope>/run.<timestamp>/reports/
```

The first parseable report contract is defined in
`docs/spec/taskjuggler-parseable-report-contract.md`.

## First run result contract

JSON output should include:

```json
{
  "metadata": {
    "id": "taskjuggler.run.20260713T200000Z",
    "project_path": "examples/small-team",
    "scenario_id": "scenario.ivan-capacity-6",
    "output_dir": "exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z",
    "taskjuggler_path": "/usr/bin/tj3",
    "taskjuggler_version": "TaskJuggler v3.8.4"
  },
  "artifacts": [
    "project.tjp",
    "run.manifest.json",
    "stdout.txt",
    "stderr.txt",
    "taskjuggler.version.txt",
    "taskjuggler.mapping.json",
    "reports/replan-task-schedule.csv"
  ],
  "warnings": [],
  "errors": []
}
```

The first implementation may not parse a calculated schedule.

The first implementation should prove:

- generated `.tjp` can be run;
- run output is saved;
- run is reproducible from RePlan state and command options;
- run failure is diagnosable;
- no canonical state is mutated.

## Manifest rules

`run.manifest.json` should include:

- run id;
- command;
- RePlan project path;
- scenario id;
- generated `.tjp` path;
- generated `.tjp` sha256;
- TaskJuggler binary path;
- TaskJuggler version output;
- run started timestamp;
- run finished timestamp;
- exit code;
- stdout path;
- stderr path;
- mapping path and sha256;
- parseable report paths and sha256 values;
- warnings;
- errors.

Manifest should not include secrets.

Manifest should not include source PM tokens.

## Later command: `replan taskjuggler import-result`

Purpose:

Convert a TaskJuggler run artifact into a RePlan scenario/proposal candidate.

Command shape:

```bash
replan taskjuggler import-result \
  --project examples/small-team \
  --run-dir exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z \
  --scenario-id scenario.tj.20260713T200000Z \
  --dry-run \
  --format json
```

Review and apply the written candidate as one flow:

```bash
replan scenario diff --project examples/small-team --format markdown scenario.tj.20260713T200000Z
replan scenario apply --project examples/small-team --actor actor.human.operator --reason "Accept generated dates." --dry-run --format markdown scenario.tj.20260713T200000Z
replan scenario apply --project examples/small-team --actor actor.human.operator --reason "Accept generated dates." --apply --format markdown scenario.tj.20260713T200000Z
```

The diff shows task id, title, changed date field, current value and candidate
value. Apply accepts the linked open TaskJuggler proposal in the same staged
write and records both scenario-apply and proposal-decision audits. Applying
after a separate proposal accept remains valid. A rejected linked proposal
blocks apply.

Candidate preview command:

```bash
replan taskjuggler import-result \
  --project examples/small-team \
  --run-dir exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z \
  --write-candidate \
  --actor actor.human.operator \
  --format json
```

Implemented first dry-run slice:

- reads `run.manifest.json`;
- reads `taskjuggler.mapping.json`;
- reads `reports/replan-task-schedule.csv`;
- requires `--dry-run`;
- maps TaskJuggler task ids back to RePlan task ids;
- previews task planned start and planned finish changes;
- does not write canonical state.

Implemented candidate write slice:

- requires `--write-candidate --actor <id>`;
- rejects simultaneous `--dry-run` and `--write-candidate`;
- writes a proposed scenario under `scenarios/`;
- writes an open proposal under `proposals/open/`;
- refuses to overwrite existing candidate files;
- does not write canonical task/resource/current-plan state;
- does not write to source PM systems.

Potential generated RePlan objects:

- scenario candidate;
- proposal;
- evidence record;
- audit preview.

Candidate file generation is explicit and separate from dry-run.

## What to import later

Possible future imports:

- scheduled task start date;
- scheduled task finish date;
- resource assignment changes;
- task order changes caused by resource leveling;
- TaskJuggler warnings;
- infeasible schedule diagnostics;
- generated report references.

Do not import:

- TaskJuggler resources as canonical resources without explicit mapping;
- TaskJuggler accounts/costs into RePlan cost model;
- generated dates as accepted current plan without human approval;
- TaskJuggler results directly into PM systems.

## Scenario candidate shape

Future scenario candidate:

```yaml
scenario:
  id: scenario.tj.20260713T200000Z
  name: TaskJuggler schedule 2026-07-13
  status: proposed
  base_ref:
    kind: scenario
    id: scenario.ivan-capacity-6
  created_at: "2026-07-13T20:00:00+07:00"
  created_by:
    kind: actor
    id: actor.system.replan
  changes:
    - op: set
      path: tasks.task.op.1234.timing.preferred_start
      value: "2026-07-08"
    - op: set
      path: tasks.task.op.1234.timing.preferred_finish
      value: "2026-07-09"
```

Exact paths require a separate design check because current task timing fields
are still limited.

## Evidence shape

Future evidence record:

```yaml
evidence:
  id: evidence.taskjuggler.run.20260713T200000Z
  type: generated_artifact
  subtype: taskjuggler_run
  source: taskjuggler
  observed_at: "2026-07-13T20:00:00+07:00"
  confidence: generated
  summary: TaskJuggler schedule run for scenario.ivan-capacity-6
  targets:
    - kind: scenario
      id: scenario.ivan-capacity-6
```

Evidence may reference artifact paths in a later evidence attachment model.

## AI CLI workflow

AI CLI agent sequence:

```bash
replan export taskjuggler --project examples/small-team --scenario scenario.ivan-capacity-6 --unrepresented-assumptions error --format json
replan dev validate-taskjuggler --project examples/small-team --scenario scenario.ivan-capacity-6 --unrepresented-assumptions error --format json
replan taskjuggler run --project examples/small-team --scenario scenario.ivan-capacity-6 --unrepresented-assumptions error --format json
replan taskjuggler import-result --project examples/small-team --run-dir exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z --dry-run --format json
```

Rules for agents:

- do not apply TaskJuggler result automatically;
- do not edit canonical files directly;
- present scenario/proposal diff for human review;
- keep run artifacts reviewable;
- fail when assumptions are unrepresented in strict mode.

## MVP non-goals

Do not implement in the first run/import experiment:

- full TaskJuggler report parsing;
- automatic write-back to OpenProject, Plane, GitHub or GitLab;
- automatic acceptance of generated schedule;
- cost/accounting import;
- TaskJuggler as required runtime for ordinary RePlan;
- long-running daemon for TaskJuggler;
- real-time schedule recalculation;
- full TaskJuggler parity.

## Implementation order

Implemented slices:

1. `replan taskjuggler run` run-only artifact capture.
2. Add fixture-driven tests using fake `tj3`.
3. Add CI run of `replan taskjuggler run` with TaskJuggler 3.8.4.
4. Design minimal TaskJuggler report output that is parseable.
5. Generate TaskJuggler CSV report and `taskjuggler.mapping.json` in
   `replan taskjuggler run`.
6. `replan taskjuggler import-result --dry-run` preview for planned task dates.
7. `replan taskjuggler import-result --write-candidate` generation of
   reviewable scenario/proposal files.
8. `replan proposal accept/reject` approval gate for generated proposals without
   canonical current-plan mutation.
9. `replan scenario diff` readable task-date rows and one `scenario apply`
   flow that accepts the linked open proposal atomically.

Backlog:

1. End-to-end smoke for TaskJuggler-generated date candidates through the
   combined scenario apply and proposal decision.

Do not skip directly to import before run artifacts are stable.

## Drafting a real plan's dates: gaps found on 2026-11-08

A consumer project drafts its dates with a private script instead of
`taskjuggler run`. The script's rules: one queue per agent resource in list
order, honouring finish-to-start dependencies; Monday to Friday with a
holiday list; duration as the estimate times a tempo plus a fixed review
share; open work not before the status date; not-before dates for tasks that
wait on people, and a scenario without them; finished tasks fixed at their
actual dates; milestone tasks for stage gates. The owner's own time is not
levelled.

`taskjuggler run` on a copy of that plan (360 tasks, 771 dependencies,
TaskJuggler 3.8.4) failed, and its export showed what the run cannot express
yet. The export:

- writes no `depends`, so all 771 dependencies are dropped without a
  warning;
- starts the project at the default status date, so tasks planned earlier
  are outside the project window, and sets no `now`;
- schedules finished tasks again with their full remaining effort, when the
  demands still carry it, instead of fixing them at their dates;
- exports no working days or holidays from availability windows and
  exceptions (only scenario exceptions become `vacation`);
- exports no demand `earliest_start`;
- writes efforts below one hour, which TaskJuggler's default 60-minute
  timing resolution refuses (`effort 0.40h`).

With those six added by hand to the `.tjp` (dependencies, a project window
and `now`, finished tasks fixed, holidays as `leaves`, not-before dates as
start milestones, 15-minute resolution) TaskJuggler scheduled the plan, but
the 23 open tasks finished between 28 days earlier and 42 days later than
the script's dates (median 6 days earlier). Two of the script's rules have
no RePlan concept yet:

- **Lead time without load.** A task that takes a number of working days
  but does not consume levelled capacity: a waiting period, or a person's
  decision whose time the plan does not level. TaskJuggler levels every
  allocation against capacity, so the owner's tasks queue behind each other.
- **Queue order within a resource.** The script runs each resource's tasks
  strictly in list order; TaskJuggler's ASAP scheduling reorders them by
  priority and slack.

Each gap is a task in the self plan. The comparison used a copy of the plan
only and wrote nothing to the consumer project.

The proposed design for the two missing concepts is in
[`lead-time-and-queue-order.md`](lead-time-and-queue-order.md).

## Rerun with lead time and priority: 2026-11-09

After the export gaps above were fixed and the lead time design was
implemented, the comparison was repeated on a fresh copy of the same plan
(362 tasks, 773 dependencies, 23 open tasks, TaskJuggler 3.8.4). The
consumer's script was run on its own copy with the status date 2026-11-09 to
give the reference dates. Nothing was written to the consumer project.

Two copies were scheduled with `replan taskjuggler run`:

- **As the script writes it.** Open tasks finished between 3 days earlier
  and 62 days later than the script (median 33 days later). Almost all of
  that comes from one data habit: the script writes every demand with its
  full remaining amount, also for work that is done and waits for review, and
  fixes those tasks at their actual dates itself. RePlan schedules remaining
  work wherever it is, so the done work was queued again.
- **With the script's rules expressed in RePlan** (35 commands and two
  availability edits):
  - remaining work set to zero with `actual set-remaining` on the demands of
    done and review-ready tasks, the RePlan way to say "done, review left";
  - owner tasks as `lead_time` working days with the owner's hours on a
    `levelled: false` demand;
  - each agent stream's order as `priority` falling with the position in
    the script's list;
  - the script's not-before dates as demand `earliest_start`;
  - its holidays as `unavailable` availability exceptions, and its six agent
    hours per working day as a 09:00-15:00 availability window.

  19 of the 23 open tasks finished on the script's date. The other four, all
  in one agent stream, finished 15 to 16 days earlier: in the script the
  stream waits strictly behind an earlier task that waits on an owner
  decision, while TaskJuggler's soft priority lets the agent take the next
  tasks in the meantime. That is the strict queue the design left until a
  consumer asks for it; the project's final milestone date is the same.

One smaller finding: the plan's dependency statuses are left `open` behind
finished predecessors, which `validate` reports as unresolved required
dependencies; `proposal suggest-dependencies` and `proposal accept` close
them. (The script also rewrites `.replan/project.yaml` without
`format_version`. That is harmless: a missing `format_version` reads as
version 1, the current format, so no `project migrate` is needed. An
earlier version of this note said otherwise.)

## Dependency start boundary

OpenProject `follows` relations and RePlan `finish_to_start` dependencies use
date-level semantics: a successor starts on the working day after the
predecessor's finish date. A plain TaskJuggler `depends` starts at the exact
end instant instead. That instant can be in the middle of the reported finish
day when an effort task does not fill its last day, which incorrectly lets a
successor's working-day lead time count the remaining afternoon.

The export keeps this rule in TaskJuggler rather than correcting dates during
`taskjuggler import-result`. Each exported ordering predecessor gets one hidden
boundary task. The boundary uses a TaskJuggler shift whose only working slot is
the final timing-resolution slot before midnight on Sunday through Thursday.
It therefore ends at the start of the next Monday-to-Friday day. Successors
depend on that boundary task instead of depending directly on the predecessor.
The same boundary is used for exported `finish_to_start`, `blocks`,
`requires_input_from` and `requires_resource_from` relations.

The boundary deliberately assumes a Monday-to-Friday project week. That is
the same week as TaskJuggler's default global working time, which the exported
working-day `lead_time` already uses. Resource availability does not widen the
project week for dependency starts: a successor whose allocated resources also
work on weekends still starts on the next Monday-to-Friday day.

This construction is deliberate instead of a fixed `gaplength` or
`gapduration`:

- a mid-day predecessor finish rounds forward to the next working-day start;
- a predecessor that fills its final day still starts its successor on the
  immediately following working day, without skipping an extra day;
- a Friday finish releases the successor on Monday;
- effort tasks, working-day and calendar-day `lead_time`, and the lead-time
  wait-then-work container all receive the same start boundary;
- lead time remains unchanged because the boundary moves the TaskJuggler start
  before TaskJuggler evaluates `length` or `duration`.

For a wait-then-work container, the container itself receives any boundary
from its external predecessors. A second hidden boundary child sits between
the `wait` and `work` children, so work starts on the working day after the
lead-time wait finishes instead of using a remaining slot late on that day.

The boundary tasks carry the `replan_internal` flag, are omitted from the CSV
schedule report, and consequently are not candidates during
`taskjuggler import-result`.
