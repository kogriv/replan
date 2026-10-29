# RePlan export artifacts

Date: 2026-07-12

Status: approved implementation specification.

## Purpose

This document defines how generated export artifacts may be stored and reviewed
inside a RePlan project.

It answers:

- where generated export files should live;
- how export artifacts should be named;
- when generated exports may be committed;
- how manifests relate to artifacts;
- how checksums should be verified;
- what is source of truth and what is reproducible output;
- how AI CLI agents should create reviewable exports without mutating canonical
  planning state.

This document depends on:

- `docs/spec/storage-layout.md`;
- `docs/spec/cli.md`;
- `docs/spec/scenario-model.md`;
- `docs/spec/workload-calculation.md`;
- `docs/architecture/application-runtime.md`.

## Core decision

Export artifacts are generated review artifacts.

Approved rule:

```text
canonical state + command flags -> generated export artifact + manifest
```

Export artifacts are not canonical planning state.

Canonical state remains under:

```text
canonical/
baselines/
actuals/
evidence/
proposals/
scenarios/
audit/
```

`exports/` is for generated outputs that are useful to inspect, compare, attach
to reviews or pass to external tools.

## Recommended layout

Recommended project-local export layout:

```text
exports/
  snapshots/
    current.json
    current.manifest.json
  workload/
    2026-W28/
      current.csv
      current.manifest.json
      scenario.openai-quota-300k.csv
      scenario.openai-quota-300k.manifest.json
  taskjuggler/
    current/
      project.tjp
      project.manifest.json
    scenario.task-op-1234-reassign-petr/
      project.tjp
      project.manifest.json
```

Rules:

- `exports/` is optional;
- generated exports may be committed when useful for review;
- generated exports may also be ignored or deleted when they are only local
  scratch artifacts;
- every committed export artifact should have a neighboring manifest;
- manifest files should use `.manifest.json`;
- generated export directories should not be required for validation success;
- deleting `exports/` must never delete canonical planning state.

## Artifact classes

### Snapshot exports

Snapshot exports are full machine-readable RePlan workspace snapshots.

Recommended path:

```text
exports/snapshots/current.json
exports/snapshots/current.manifest.json
```

Command:

```bash
replan export json \
  --project examples/small-team \
  --output exports/snapshots/current.json \
  --manifest exports/snapshots/current.manifest.json
```

Snapshot exports are useful for:

- agent-to-agent context handoff;
- external analysis;
- regression comparison;
- debugging loader or validation behavior.

Snapshot exports are not a replacement for canonical YAML and JSONL files.

### Workload CSV exports

Workload CSV exports are tabular capacity, demand, actual and overload views.

Recommended paths:

```text
exports/workload/2026-W28/current.csv
exports/workload/2026-W28/current.manifest.json
exports/workload/2026-W28/scenario.openai-quota-300k.csv
exports/workload/2026-W28/scenario.openai-quota-300k.manifest.json
```

Commands:

```bash
replan export csv \
  --project examples/small-team \
  --report workload \
  --week 2026-W28 \
  --output exports/workload/2026-W28/current.csv \
  --manifest exports/workload/2026-W28/current.manifest.json

replan export csv \
  --project examples/small-team \
  --report workload \
  --week 2026-W28 \
  --scenario scenario.openai-quota-300k \
  --output exports/workload/2026-W28/scenario.openai-quota-300k.csv \
  --manifest exports/workload/2026-W28/scenario.openai-quota-300k.manifest.json
```

Workload CSV exports are useful for:

- spreadsheet inspection;
- pull request review;
- comparing current plan against scenarios;
- feeding simple downstream reporting tools.

### TaskJuggler exports

TaskJuggler exports are optional scheduling-engine artifacts.

Recommended paths:

```text
exports/taskjuggler/current/project.tjp
exports/taskjuggler/current/project.manifest.json
exports/taskjuggler/scenario.task-op-1234-reassign-petr/project.tjp
exports/taskjuggler/scenario.task-op-1234-reassign-petr/project.manifest.json
```

Commands:

```bash
replan export taskjuggler \
  --project examples/small-team \
  --output exports/taskjuggler/current/project.tjp \
  --manifest exports/taskjuggler/current/project.manifest.json

replan export taskjuggler \
  --project examples/small-team \
  --scenario scenario.task-op-1234-reassign-petr \
  --output exports/taskjuggler/scenario.task-op-1234-reassign-petr/project.tjp \
  --manifest exports/taskjuggler/scenario.task-op-1234-reassign-petr/project.manifest.json
```

TaskJuggler exports are useful for:

- parity checks against an external scheduler;
- manual inspection of resource allocations;
- scenario review;
- future optional TaskJuggler execution.

TaskJuggler is not a required runtime dependency.

## Naming rules

Use stable, explicit names:

- use `current` for canonical current workspace exports;
- use the exact scenario identifier for scenario exports;
- use ISO week directory names such as `2026-W28` for workload exports;
- use lowercase extensions matching content type: `.json`, `.csv`, `.tjp`;
- use `.manifest.json` for manifests;
- avoid timestamp-only artifact names for reviewable exports.

Timestamped local scratch artifacts are allowed outside the committed layout,
for example under `/tmp`.

## Manifest rules

Every committed artifact should have a manifest next to it.

Current manifest fields:

```json
{
  "id": "export.replan...",
  "command": "export csv",
  "format": "csv",
  "report": "workload",
  "project_path": "examples/small-team",
  "output_path": "exports/workload/2026-W28/current.csv",
  "scenario_id": "scenario.openai-quota-300k",
  "week": "2026-W28",
  "sha256": "...",
  "created_at": "2026-07-12T18:00:00+07:00",
  "warnings": [],
  "errors": []
}
```

Rules:

- manifest is generated only when `--output` is used;
- manifest is written only after a successful export;
- manifest checksum must be calculated from the exact exported file content;
- manifest warnings and errors describe export-time diagnostics;
- successful committed manifests should normally have an empty `errors` array;
- `output_path` should point to the artifact the manifest describes;
- scenario exports should include `scenario_id`;
- workload exports should include `week`;
- workload CSV exports should include `report: "workload"`.

## Verification

Verify a manifest checksum:

```bash
sha256sum exports/workload/2026-W28/current.csv
rg '"sha256"' exports/workload/2026-W28/current.manifest.json
```

The checksum printed by `sha256sum` should match the manifest `sha256`.

Verify that exports are reproducible:

```bash
replan validate examples/small-team --format json
replan export csv \
  --project examples/small-team \
  --report workload \
  --week 2026-W28 \
  --output /tmp/current.csv \
  --manifest /tmp/current.manifest.json
```

Then compare `/tmp/current.csv` with the committed artifact when the canonical
state and command flags are expected to match.

## Commit policy

Generated exports may be committed when they help review.

Good reasons to commit exports:

- a pull request changes planning behavior;
- a scenario needs human validation;
- an external stakeholder needs a CSV or `.tjp` artifact;
- a regression test is easier to inspect through generated output;
- an AI agent needs a stable handoff artifact.

Bad reasons to commit exports:

- local scratch output;
- repeated timestamped exports with no review value;
- artifacts with secrets or source access tokens;
- outputs generated from dirty or unclear source state;
- outputs whose manifest checksum does not match.

## Git ignore policy

Do not blanket-ignore `exports/` in the repository.

Reason:

- some generated exports are intentionally reviewable and committable;
- agents should be able to create explicit artifact diffs;
- humans can decide whether a generated artifact is worth committing.

Projects may still ignore local scratch subdirectories if needed, for example:

```text
exports/tmp/
```

## Source-of-truth boundary

Exports must never be edited as planning input.

If an export reveals a planning error, fix the canonical or scenario input and
regenerate the export.

Do not manually edit:

- `.tjp` exports to fix a RePlan model issue;
- workload CSV values to make overload disappear;
- snapshot JSON to change canonical state;
- manifest checksums.

Manual export edits are allowed only for out-of-band experiments outside the
committed artifact layout.

## AI CLI workflow

Recommended agent workflow:

1. Validate the project.
2. Generate export artifact into `exports/`.
3. Generate manifest in the same directory.
4. Verify checksum when useful.
5. Show diff to the human reviewer.
6. Commit generated artifact only when it adds review value.

Example:

```bash
replan validate examples/small-team --format json
replan export csv \
  --project examples/small-team \
  --report workload \
  --week 2026-W28 \
  --scenario scenario.openai-quota-300k \
  --output exports/workload/2026-W28/scenario.openai-quota-300k.csv \
  --manifest exports/workload/2026-W28/scenario.openai-quota-300k.manifest.json
git diff -- exports/workload/2026-W28/
```

The agent should not assume generated exports must always be committed.

## Future work

Later additions may include:

- `replan export verify-manifest`;
- deterministic export timestamps for reproducible test fixtures;
- generated artifact pruning policy;
- HTML or chart exports;
- TaskJuggler run artifact pruning and review policy under
  `exports/taskjuggler/<scope>/run.<timestamp>/`;
- signed manifests for hosted or regulated workflows.
