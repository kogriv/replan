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

Backlog:

1. End-to-end smoke for TaskJuggler-generated date candidates through proposal
   acceptance and scenario apply.

Do not skip directly to import before run artifacts are stable.
