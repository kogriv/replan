# RePlan TaskJuggler parseable report contract

Date: 2026-07-13

Status: implemented first slices. TaskJuggler run generates the parseable CSV
report and mapping artifacts, `import-result --dry-run` previews task date
candidate changes, and `import-result --write-candidate` writes reviewable
scenario/proposal files without mutating canonical state.

## Purpose

This document defines the minimum machine-readable TaskJuggler report contract
that RePlan generates before importing TaskJuggler results.

The goal is not full TaskJuggler report parity.

The goal is a small, stable, testable artifact that lets RePlan compare a
TaskJuggler-calculated schedule with RePlan canonical state and produce
reviewable scenario/proposal candidates.

## Sources checked

The design is based on TaskJuggler 3.8.4 behavior and the official
TaskJuggler manual:

- `taskreport` lists tasks and their values in a table and supports `columns`;
- `formats` supports `csv`, `html` and `niku` for reports;
- `columnid` defines the built-in columns needed for the first contract,
  including `id`, `name`, `start`, `end`, `effort`, `effortleft`, `resources`
  and `status`;
- `export` can produce a scheduled TaskJuggler file, but it is heavier than
  the first CSV contract and is not the first import source.

Manual URLs:

- <https://taskjuggler.org/tj3/manual/taskreport.html>
- <https://taskjuggler.org/tj3/manual/formats.html>
- <https://taskjuggler.org/tj3/manual/columnid.html>
- <https://taskjuggler.org/tj3/manual/export.html>

Local verification with TaskJuggler 3.8.4 confirmed that a `taskreport` with
`formats csv` writes a semicolon-separated CSV file such as:

```csv
"Id";"Name";"Start";"End";"Effort";"Effort Left";"Resources";"Status"
"task_op_1234";"Implement validation command";"2026-07-06";"2026-07-13";1.0;0.0;"Ivan (resource_human_ivan)";"done"
```

## Decision

Use a minimal TaskJuggler `taskreport` CSV as the first parseable report.

Do not parse TaskJuggler HTML.

Do not parse free-form TaskJuggler stdout.

Do not parse a scheduled `.tjp` export as the first import source.

Do not rely on TaskJuggler task IDs being identical to RePlan canonical IDs,
because RePlan currently sanitizes IDs for TaskJuggler syntax. The run must
also produce a mapping artifact that links TaskJuggler IDs back to RePlan IDs.

## Report block

RePlan should append or generate this report block in the `.tjp` used by
`replan taskjuggler run`:

```tjp
taskreport replan_task_schedule "reports/replan-task-schedule" {
  formats csv
  columns id, name, start, end, effort, effortleft, resources, status
  sorttasks seqno.up
}
```

Rules:

- the report file path is relative to the TaskJuggler run working directory;
- RePlan must create the `reports/` directory before running TaskJuggler;
- TaskJuggler appends `.csv` to the report file name;
- the expected artifact path is
  `reports/replan-task-schedule.csv`;
- do not set `hideresource 0` for the first import report, because that emits
  additional resource rows that are not task candidates;
- assigned resources should be read from the `Resources` column for first-pass
  review, not treated as canonical resource allocation truth.

## Required artifacts

A successful `replan taskjuggler run` produces:

```text
project.tjp
stdout.txt
stderr.txt
taskjuggler.version.txt
run.manifest.json
reports/replan-task-schedule.csv
taskjuggler.mapping.json
```

The `reports/` directory is for TaskJuggler-generated reports.

The `taskjuggler.mapping.json` file is a RePlan-generated artifact. It is not a
TaskJuggler report.

## CSV dialect

The first parser must use this dialect:

- delimiter: semicolon (`;`);
- quote: double quote (`"`);
- header row: required;
- encoding: UTF-8;
- line endings: tolerate `\n` and `\r\n`;
- empty date fields: allowed;
- numeric fields: parse using dot decimal separator.

Do not infer the delimiter automatically in the first implementation. Fail with
a clear diagnostic if the header cannot be parsed with the expected delimiter.

## Required columns

The first import parser should require these columns by header name:

```text
Id
Name
Start
End
Effort
Effort Left
Resources
Status
```

Header matching should be exact after trimming surrounding whitespace.

The parser should reject a report with missing required columns.

The parser should ignore extra columns.

## Field semantics

### `Id`

TaskJuggler task id.

This is not necessarily the RePlan task id.

The parser must resolve it through `taskjuggler.mapping.json`.

Rows without a mapping must become diagnostics and must not become scenario
changes.

### `Name`

Human-readable TaskJuggler task name.

Use it only for diagnostics and review output.

Do not use it as an identity key.

### `Start`

TaskJuggler-calculated task start date.

Expected format for the first implementation: `YYYY-MM-DD`.

Empty value means TaskJuggler did not produce a start date for that row. Empty
start should not produce a date change.

### `End`

TaskJuggler-calculated task end date.

Expected format for the first implementation: `YYYY-MM-DD`.

Empty value means TaskJuggler did not produce an end date for that row. Empty
end should not produce a date change.

### `Effort`

TaskJuggler allocated effort for the task in the report's load unit.

For the first contract, this is evidence and review context only.

Do not write effort back into RePlan from this field in the first import slice.

### `Effort Left`

TaskJuggler remaining allocated effort.

For the first contract, this is evidence and review context only.

Do not overwrite RePlan `remaining_amount` from this field in the first import
slice.

### `Resources`

TaskJuggler resource display list.

For the first contract, this is evidence and review context only.

Do not reassign RePlan resources from this field in the first import slice.

Later reassignment import may use a separate parseable allocation report or a
mapping-aware parser.

### `Status`

TaskJuggler computed status.

For the first contract, this is evidence and review context only.

Do not overwrite RePlan tracking status from this field.

## Mapping artifact

`taskjuggler.mapping.json` should be generated by RePlan at the same time as
`project.tjp`.

Minimum shape:

```json
{
  "version": 1,
  "project_path": "examples/small-team",
  "scenario_id": "scenario.ivan-capacity-6",
  "task_id_map": [
    {
      "replan_task_id": "task.op-1234",
      "taskjuggler_task_id": "task_op_1234"
    }
  ],
  "resource_id_map": [
    {
      "replan_resource_id": "resource.human-ivan",
      "taskjuggler_resource_id": "resource_human_ivan"
    }
  ]
}
```

Rules:

- mapping is generated from the same export input as `project.tjp`;
- mapping must not be hand-authored;
- mapping must be included in `run.manifest.json`;
- mapping must be treated as required input for `import-result`;
- if two RePlan IDs map to the same TaskJuggler ID, export must fail before
  running TaskJuggler.

The collision rule is important because current TaskJuggler IDs are sanitized
from RePlan IDs. For example, punctuation may become underscores.

## Run manifest additions

`run.manifest.json` should add:

```json
{
  "reports": [
    {
      "kind": "taskjuggler_task_schedule_csv",
      "path": "reports/replan-task-schedule.csv",
      "sha256": "<sha256>"
    }
  ],
  "mapping_path": "taskjuggler.mapping.json",
  "mapping_sha256": "<sha256>"
}
```

The manifest should include the report artifact even if later
`import-result --dry-run` is not run.

## Import-result contract

`replan taskjuggler import-result --dry-run` and
`replan taskjuggler import-result --write-candidate` read:

- `run.manifest.json`;
- `taskjuggler.mapping.json`;
- `reports/replan-task-schedule.csv`.

Dry-run should produce preview-only scenario/proposal candidates.

Write-candidate mode writes reviewable scenario/proposal files only.

Both modes must not mutate canonical task/resource/current-plan state.

Both modes must not write back to OpenProject, Plane, GitHub or GitLab.

Candidate changes allowed in the first import dry-run:

- task planned start date;
- task planned finish date.

Candidate changes not allowed in the first import dry-run:

- task title changes;
- task status changes;
- remaining effort changes;
- resource reassignment;
- dependency changes;
- capacity changes;
- actuals;
- source-system write-back.

## Proposed dry-run output

The dry-run output should be an ordinary RePlan envelope.

Result shape:

```json
{
  "run_dir": "exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z",
  "report_path": "reports/replan-task-schedule.csv",
  "mapping_path": "taskjuggler.mapping.json",
  "candidate": {
    "id": "scenario.taskjuggler.20260713T200000Z",
    "source": "taskjuggler",
    "source_run_id": "taskjuggler.run.20260713T200000Z",
    "changes": [
      {
        "op": "set",
        "path": "tasks.task.op-1234.planned_start",
        "value": "2026-07-06",
        "previous_value": "2026-07-07"
      },
      {
        "op": "set",
        "path": "tasks.task.op-1234.planned_finish",
        "value": "2026-07-13",
        "previous_value": "2026-07-11"
      }
    ]
  },
  "preview_only": true
}
```

Exact canonical paths may change only if the canonical date fields are renamed
in the canonical model.

## Diagnostics

Minimum diagnostics:

```text
taskjuggler_report_missing
taskjuggler_mapping_missing
taskjuggler_report_header_invalid
taskjuggler_report_row_invalid
taskjuggler_report_task_unmapped
taskjuggler_report_date_invalid
taskjuggler_report_no_candidate_changes
taskjuggler_report_unsupported_resource_change
```

Warnings:

- unsupported resource allocation differences;
- TaskJuggler rows that are not task rows;
- report rows with empty start or end;
- report date equals existing RePlan date and therefore does not create a
  candidate change.

Errors:

- missing report;
- missing mapping;
- duplicate mapping;
- required column missing;
- invalid date in a row that would otherwise produce a candidate change.

## Implementation status

Implemented:

1. Extend TaskJuggler export/run generation to include the CSV task report
   block.
2. Generate `taskjuggler.mapping.json`.
3. Update `replan taskjuggler run` to create `reports/`, verify the CSV report
   exists after TaskJuggler exits and include report/mapping hashes in the run
   manifest.
4. Add tests with fake `tj3` that writes a semicolon-delimited CSV report.
5. Add CI validation with real TaskJuggler 3.8.4 that checks
   `reports/replan-task-schedule.csv`.

6. Implement `replan taskjuggler import-result --dry-run` for task planned
   start and planned finish preview changes.
7. Implement `replan taskjuggler import-result --write-candidate --actor <id>`
   for reviewable scenario/proposal candidate files.
8. Implement `replan proposal accept/reject` approval gate for reviewable
   candidates without canonical current-plan mutation.

Backlog:

1. End-to-end smoke for TaskJuggler-generated date candidates through proposal
   acceptance and scenario apply.

## Non-goals

Do not implement in this slice:

- parsing HTML reports;
- parsing stdout progress output;
- parsing all TaskJuggler report columns;
- accepting TaskJuggler as canonical truth;
- automatic resource reassignment from the `Resources` column;
- automatic write-back to source PM systems;
- long-running TaskJuggler daemon.
