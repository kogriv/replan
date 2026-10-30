# RePlan CLI contract

Date: 2026-07-06

Status: approved implementation specification. Parts of it describe a wider
surface than the binary implements.

## Implementation status, 2026-10-28

This contract was written ahead of the implementation. Read it together with
the binary's own `--help`, which is authoritative for what exists.

Implemented as specified, or narrower: the command families listed in
[`../dev/getting-started.md`](../dev/getting-started.md), the JSON envelope,
per-command `--project`/`-p`, `--format`, `--actor`, `--dry-run` and
canonical-writer `--apply`, and Go CLI exit codes `0`, `1`, `2`, `4`, `5`, `6`
and `8`.

The project-independent `replan version [--format text|json]` command and the
compact `replan --version` form are also implemented.

Specified here and not implemented:

| Section | Not implemented |
| --- | --- |
| Global flags | global placement of `--project`, `--format`, `--actor` and `--dry-run`; `--agent`, `--yes`, `--verbose`, `--quiet`, `--debug`; the Go CLI environment fallback `REPLAN_ACTOR`; policy `default_actor`; parent-directory discovery of `.replan/project.yaml` (an empty `--project` means the current directory) |
| Go CLI exit codes | `3`, `7`, `9`, `10`, `11`; helper-script exits are a separate contract below and do use `7` and `11` |
| `validate` | `--warnings-as-errors`, `--scope`; the binary has `--strict` |
| `workload` | `--from`, `--to`, `--status-date`, `--task`, `--include-scenarios` (GAP-011) |
| `report` | `workload`, `tracking`, `warnings`, `scenario`, `proposals`; `report weekly --output` |
| `scenario` | `create`, `patch`, `calculate`, `propose-apply` |
| `proposal` | `create`, `supersede` |
| `audit` | `show` |
| `actual` | the generic `add` alias |
| `import` | `github`, `gitlab`, `csv` |
| Source write-back | the `writeback` command family; see that section |

For `import openproject`, `REPLAN_OPENPROJECT_TIMEOUT_SECONDS` supplies the
default for `--request-timeout`, `REPLAN_OPENPROJECT_RETRIES` supplies the
default for `--request-retries`, and
`REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS` supplies the default for
`--request-retry-delay`. The defaults are 60 seconds, 0 retries and a 2-second
retry delay. Retries apply only to GET requests and only after network errors
or HTTP 408, 429, 500, 502, 503 and 504 responses.

Python OpenProject operator helpers retry reads and PATCH requests carrying a
`lockVersion` only; validation-only form calls are retried only when the caller
explicitly marks them idempotent.

## Purpose

This document defines the first RePlan command-line interface contract.

It answers:

- which command families exist in MVP;
- how commands find a RePlan project;
- how commands identify actor and policy context;
- which output formats are supported;
- which exit codes are stable;
- which commands are read-only;
- which commands mutate local canonical state;
- which commands require dry-run or proposal flow;
- how commands support AI CLI agents safely;
- how the CLI maps to future daemon, server API and user interface.

This document depends on:

- `docs/spec/canonical-model.md`;
- `docs/spec/storage-layout.md`;
- `docs/spec/export-artifacts.md`;
- `docs/spec/taskjuggler-capacity-quota-mapping.md`;
- `docs/spec/taskjuggler-run-import-experiment.md`;
- `docs/architecture/application-runtime.md`;
- `docs/security/access-and-actor-model.md`.

## Core decision

The RePlan MVP must be CLI-first and AI-CLI-first.

Approved rule:

```text
CLI command -> application service -> storage adapter/domain/calculation/reporting
```

CLI handlers must be thin.

CLI handlers must not contain:

- domain model logic;
- workload formulas;
- scheduling logic;
- source adapter mapping logic;
- storage-specific business rules;
- policy decision logic;
- report calculation logic.

The CLI is the first product interface, but it is not the domain layer.

## CLI shape

Executable name:

```text
replan
```

General command form:

```text
replan [global flags] <command> <subcommand> [arguments] [flags]
```

Examples (flags follow the command because the implemented binary has
per-command flags, not global flags):

```bash
replan validate examples/small-team --format json
replan workload --project examples/small-team --week 2026-W28 --format markdown
replan report weekly --project examples/small-team --week 2026-W28 --format json
replan scenario diff scenario.ivan-unavailable-week-28 --against current --format markdown
replan proposal accept proposal.adjust-remaining-task-op-1234.20260706 --dry-run --format json
```

## MVP command families

Required MVP command families:

```text
replan validate
replan workload
replan report
replan scenario
replan proposal
replan dependency
replan actual
replan import
replan export
replan audit
```

Implemented additional command families and top-level commands:

```text
replan next
replan project
replan tracking
replan taskjuggler
replan dev
```

Recommended later command families:

```text
replan config
replan actor
replan resource
replan task
replan evidence
replan event
replan serve
replan daemon
```

Later command families must call the same application services as MVP commands.

## Global flags

### Project location

Implemented placement:

```bash
replan validate --project /path/to/replan-project
```

Short alias:

```bash
replan validate -p /path/to/replan-project
```

The flag is registered separately by each implemented command; placing it
before the command is not implemented. Every command uses `.` when
`--project` is omitted; `validate` also takes the path as its first
positional argument. Parent-directory discovery remains unimplemented.

Specified default behavior, not implemented:

```text
if --project is omitted, search current directory and parents for .replan/project.yaml
```

Rules:

- command must fail with clear error if no project is found;
- command must state resolved project path in JSON output metadata;
- command should avoid hidden global working-directory assumptions;
- future server mode may replace project path with API endpoint, but local MVP
  uses project path.

### Output format

Per-command flag:

```bash
--format json
--format markdown
--format text
--format csv
```

Short alias may be added later:

```bash
-f json
```

Required formats:

```text
json
markdown
text
csv
```

Rules:

- every command must support `--format json`;
- read/report commands should support `--format markdown` when useful;
- human summary commands may support `--format text`;
- table-like export/report commands may support `--format csv`;
- JSON is the canonical machine-readable output;
- Markdown is the canonical human-readable report output;
- text is for terminal convenience only and must not be treated as stable API.

Default format:

```text
text for quick human status commands
markdown for report commands
json when --agent is used
```

### Actor

Per-command flag:

```bash
--actor actor.human.operator
```

The Go CLI does not read an actor environment fallback. Two shell workflows do:

```bash
REPLAN_ACTOR=actor.human.operator
```

`scripts/smoke-mvp.sh` and `scripts/run-openproject-sidecar-pipeline.sh` use
that value (defaulting it to `actor.human.operator`) and pass it to commands as
an explicit `--actor`. Other CLI calls must supply `--actor` themselves.

Specified project policy fallback, not implemented:

```text
.replan/policy.yaml default_actor
```

Rules:

- mutating commands must resolve actor;
- read-only commands should include actor when supplied;
- AI agents should use explicit actor id;
- unknown actor should fail for mutation unless project policy allows local
  trusted fallback;
- output metadata should include resolved actor for mutating commands.

### Status date

Per-command flag where implemented:

```bash
--status-date 2026-07-06
```

Rules:

- report and workload support for an explicit status date is not implemented;
- actual mutation commands accept `--status-date`;
- if omitted, project default status date may be used;
- if neither is present, current local date may be used only with explicit
  metadata in output;
- output must state effective status date.

### Dry-run

Per-command mutation flag:

```bash
--dry-run
```

Meaning:

```text
calculate and show intended changes, but do not mutate canonical state
```

Rules:

- every command that writes canonical state accepts `--dry-run` and `--apply`;
- exactly one choice is required; `REPLAN_APPLY=1` supplies `--apply` when
  neither flag is present, while explicit `--dry-run` always remains a preview;
- both flags, neither flag without the environment fallback, and any other
  `REPLAN_APPLY` value are usage errors with exit code `8`;
- the missing-choice JSON diagnostic code is `apply_choice_required` and the
  conflict code is `apply_choice_conflict`; for example, `actual add-status`
  reports `actual add-status writes canonical state: pass --dry-run to preview
  or --apply (or REPLAN_APPLY=1) to write`;
- source write-back must require dry-run before real apply;
- scenario application should support dry-run;
- proposal acceptance/rejection should support dry-run;
- dry-run may produce audit only if policy explicitly wants dry-run audit;
- dry-run output must be structured and reviewable.

### Project write lock

The Go CLI serializes commands that write files inside one project with an
exclusive advisory lock at `.replan/write.lock`. The lock covers loading the
project, calculating and validating the planned change, and committing every
file, so a waiting writer checks preconditions against the state left by the
previous writer. The lock file is created on demand and remains in place;
deleting lock files is unsafe because it can create two independently locked
files.

Canonical `--apply` commands, import snapshot writes, proposal-candidate
writes, project-local report/export artifacts and project-local TaskJuggler
artifacts take the lock. Dry runs, read-only commands and artifacts written
outside the project do not. A writer polls the non-blocking platform lock for
up to 30 seconds. `REPLAN_LOCK_TIMEOUT_SECONDS` may set a different positive
timeout, including fractional seconds for tests.

If the timeout expires, the command writes nothing and reports
`project_locked`, naming the lock file, with conflict exit code `5`. The caller
may retry the complete command; it must not delete `.replan/write.lock`.

Before replacing the first staged file, a canonical write records every full
file image, removal, commit-order entry and SHA-256 in
`.replan/commit-pending/`. The manifest is published last, after the blobs are
durable. A completed write removes the journal. If a process stops after the
manifest is published, the next locked write replays the entire validated set
before loading the project and reports the warning `recovered_commit` with the
file list. `replan project recover --apply` performs the same replay
explicitly. Full-file atomic replacement and missing-file-tolerant removals
make replay idempotent.

Read-only commands never replay or remove a journal. They continue against the
files currently on disk and report `interrupted_commit_pending`, naming the
project. A journal without a manifest predates the first project-file
replacement and is removed by the next recovery. A complete journal is fully
validated before replay; a missing or hash-mismatched blob reports
`commit_journal_corrupt` with exit code `2`, changes nothing and tells the
operator to inspect `.replan/commit-pending/`.

### Agent mode

Global flag:

```bash
--agent
```

Meaning:

```text
prefer machine-readable deterministic output and avoid interactive prompts
```

Rules:

- implies `--format json` unless format is explicitly set;
- disables interactive confirmation;
- requires explicit `--yes` for destructive or applying operations where
  allowed;
- should include stable object ids and warning codes;
- should avoid terminal color and progress spinners.

### Non-interactive confirmation

Flag:

```bash
--yes
```

Rules:

- allowed only for operations that policy permits;
- must not bypass dry-run requirement where policy requires dry-run;
- must not bypass proposal requirement where policy requires proposal;
- AI agents should generally not use `--yes` unless actor policy explicitly
  permits.

### Verbosity and diagnostics

Flags:

```bash
--verbose
--quiet
--debug
```

Rules:

- `--quiet` suppresses non-essential human output but not JSON result body;
- `--debug` may include stack traces or adapter diagnostics;
- debug output must not expose secrets;
- machine-readable errors should remain valid JSON when `--format json`.

## Output envelope

All JSON output should use a stable envelope.

Success example:

```json
{
  "ok": true,
  "command": "validate",
  "project_path": "/path/to/replan/examples/small-team",
  "status_date": "2026-07-06",
  "actor": {
    "id": "actor.human.operator"
  },
  "result": {},
  "warnings": [],
  "errors": []
}
```

Failure example:

```json
{
  "ok": false,
  "command": "validate",
  "project_path": "/path/to/replan/examples/small-team",
  "result": null,
  "warnings": [],
  "errors": [
    {
      "code": "missing_required_file",
      "message": "canonical/resources.yaml is missing",
      "target": {
        "kind": "file",
        "id": "canonical/resources.yaml"
      },
      "severity": "error"
    }
  ]
}
```

Rules:

- `ok` is required;
- `command` is required;
- `result` is required, may be `null` on failure;
- `warnings` is required and may be empty;
- `errors` is required and may be empty;
- errors and warnings must have stable `code`;
- target references should be included where possible;
- output must not include secrets.

## Exit codes

Stable exit codes:

```text
0  success
1  general error
2  validation failed
3  policy denied
4  not found
5  conflict
6  external source error
7  partial success
8  invalid command usage
9  unsupported operation
10 internal error
11 attention needed (helper scripts only)
```

Rules:

- JSON `ok: false` should correspond to non-zero exit code;
- validation errors use exit code `2`;
- validation warnings without errors should use exit code `0`;
- an unsupported, invalid or too-new `project.format_version` uses exit code
  `2` with the single `project_format_unsupported` error; no other project
  files are loaded, and report commands do not write requested artifacts;
- `openproject_ids_private` refusal uses exit code `8`;
- policy denial uses exit code `3`;
- a missing object referenced by id within a loadable project (scenario,
  proposal, import snapshot, source profile) uses exit code `4`. An absent
  project root, or a project path that is not a directory, also uses exit code
  `4` with one `project_not_found` error. A present but incomplete project
  directory remains a validation failure (exit code `2`) with
  `missing_required_file` errors, and other stat or read failures remain
  validation failures such as `file_stat_error`;
- file lock or concurrent mutation conflict uses exit code `5`; lock timeout
  uses diagnostic code `project_locked` and does not write project data;
- OpenProject/GitHub/GitLab/TaskJuggler failures use exit code `6`. The
  binary uses it for `import openproject` when OpenProject is unreachable,
  refuses the token or the project, rate-limits, or answers garbage; an
  invalid sidecar or plan stays `2`;
- partial import/export should use exit code `7`;
- argument errors use exit code `8`; this includes a malformed `--week` value
  (not a `YYYY-Www` ISO week label), which returns the `invalid_week`
  diagnostic without loading the project;
- known but unavailable feature uses exit code `9`;
- unexpected panic or bug uses exit code `10`;
- a helper script that ran to the end but reports `ok: false`, because it
  found a drift or an action was blocked or skipped, uses exit code `11`, so
  that `set -e` and `&&` stop at it; an apply that wrote some actions and not
  others uses `7`. The binary does not use `11`.

AI CLI agents should rely on exit code and JSON envelope, not human text.

### Helper scripts

The helpers in `scripts/` print a flat envelope of their own, from
`scripts/lib/helper_envelope.py` and, in shell, `scripts/lib/envelope.sh`
(finding C-08 of the 2026-10-15 agent tooling review):

Every helper answers `-h` or `--help` on stdout with exit `0` before reading
settings or doing work; a usage error is the JSON envelope with
`error_codes: ["usage_error"]` on stdout and exit `8`.

- `ok`, `command`;
- `mode`: one of `dry_run`, `apply`, `read_only`, `skipped`, `invalid`;
- `warnings` and `errors`: entries with `code`, `message`, `severity` and,
  where there is one, `target`;
- `warning_codes` and `error_codes`: the sorted codes of those entries.

Counts, actions, items and a helper's own status key, such as
`drift_status`, sit beside these.

When an apply fails after one or more writes, the failure envelope includes
`applied_so_far`, using the helper's normal applied-action shape, and
`failed_step`, identifying the write and its target ids. If OpenProject could
have accepted the failed request, `failed_step.may_have_been_applied` is true;
the helper exits `7` when `applied_so_far` is non-empty and otherwise keeps the
failure's usual exit code.

Rules:

- a failure is the same envelope with `ok: false`, never plain text;
- the older shape, a bare `error` string with a `message`, is converted by
  the shared module, and `envelope_check` still reads it;
- an exception nothing caught becomes an envelope too: exit `6` for an
  OpenProject request failure, `10` for anything else;
- a failure exits with a code from the table above:
  - `8` for a wrong or missing setting or format;
  - `6` when OpenProject failed or could not be reached
    (`helper_envelope.EXTERNAL_CODES`), whatever exit the CLI reports;
  - `4` for a missing file such as the private bindings;
  - `2` for an invalid plan;
  - `1` otherwise;
- every other exit comes from `ok`, finding C-02:
  - `0` when `ok` is true, including a soft drift;
  - `11` when the run ended with `ok: false`, such as a hard drift, a blocked
    date push, an unbound task in the overlay, or a status move the workflow
    does not allow;
  - `7` for an apply that wrote some actions and not others.

## Positional arguments

Commands that take a primary object id as a positional argument must accept the
id both before and after flags.

These forms must be equivalent:

```bash
replan scenario diff scenario.ivan-unavailable-week-28 --project examples/small-team --format json
replan scenario diff --project examples/small-team --format json scenario.ivan-unavailable-week-28
replan proposal accept --project examples/small-team proposal.adjust-remaining-task-op-1234.20260706 --actor actor.human.operator --apply
```

The same rule applies to:

- `replan scenario show`;
- `replan scenario diff`;
- `replan scenario apply`;
- `replan proposal show`;
- `replan proposal accept`;
- `replan proposal reject`;
- `replan import accept`.

Reason:

- humans naturally use both CLI styles;
- AI CLI agents frequently place flags before positional ids;
- command behavior should not depend on fragile argument ordering when the
  intent is unambiguous.

Implemented 2026-07-17:

- id-based commands listed above support a single positional identifier before
  flags, after flags or between flags;
- flag values such as `--project examples/small-team` are not mistaken for
  positional identifiers.

## Read-only versus mutating commands

Implemented read-only commands:

```text
validate
workload
report weekly (without --write)
report state
report dashboard (without --output)
next
tracking suggest (without --write-candidates)
scenario list
scenario show
scenario diff
proposal list
proposal show
proposal suggest-dependencies (without --write-candidates)
dependency list
actual list
audit list
audit verify
import openproject --dry-run
export json (without --output)
export csv (without --output)
export taskjuggler (without --output)
taskjuggler import-result --dry-run
dev validate-taskjuggler (without --keep-temp)
```

Implemented commands that mutate local canonical state and therefore require
the shared explicit apply choice:

```text
scenario apply
proposal accept
proposal reject
project migrate
project recover
project set-status-date
evidence add
task add
task set-dates
task record-actual-window
task set-title
actual add-consumption
actual set-remaining
actual close-demand
actual reopen-demand
actual acknowledge-overrun
actual add-status
import accept
```

Commands that write only reviewable or generated artifacts, not canonical
state, are unchanged; where a write flag exists, they write only what it asks
for:

```text
import openproject
proposal suggest-dependencies --write-candidates
tracking suggest --write-candidates
taskjuggler import-result --write-candidate
report weekly --write
report dashboard --output
export json --output
export csv --output
export taskjuggler --output
taskjuggler run
dev validate-taskjuggler --keep-temp
```

The not-implemented table names the wider read and mutation commands from the
original contract, including `scenario create|patch|calculate`, `proposal
create`, `actual add`, `audit show` and first-class write-back.

Rules:

- canonical-state mutations must resolve actor and require the shared
  `--dry-run`/`--apply` choice;
- mutating commands must produce audit unless dry-run and policy says dry-run
  audit is not needed;
- external write-back must be explicit, dry-run capable and audited;
- read-only commands should never mutate canonical state;
- report generation may write generated report files only when `--output` or
  `--write` is provided.

### OpenProject private bindings layer

Commands that load a workspace accept the global-style `--bindings <path>`
flag. `REPLAN_OPENPROJECT_BINDINGS_LAYER` is the fallback path and the flag
wins. An empty value disables the layer. The CLI does not read
`REPLAN_OPENPROJECT_BINDINGS_PATH`, because OpenProject shell helpers export
that separate helper setting into their subprocesses.

The filestore applies the layer after loading canonical tasks and before the
workspace reaches an application service. For every task it removes committed
`source: openproject` refs, preserves refs from other sources, and appends the
binding entry's `source_ref` when present. A task without a binding therefore
has no OpenProject ref. A binding for an absent canonical task produces the
`binding_unknown_task` warning. The `dependencies` and `resources` lists in
the private file are not layered in this slice.

A requested missing or malformed file is a settings/usage failure (exit 8),
never a fallback to committed ids. Every canonical-writing command refuses an
active layer, including dry-run, with `bindings_layer_read_only` and exit 8;
so do `import openproject` without `--dry-run` (its snapshot) and
`tracking suggest --write-candidates`. Underneath, the layered store refuses
every project write, so a writer missed here fails instead of persisting
private ids. `report dashboard`, `report weekly --write`, `taskjuggler run`
and the `export` commands' `--output`/`--manifest` files refuse a destination
inside the project with `bindings_layer_output_in_project` and exit 8
(dashboard output resolves against the project, export and run paths against
the working directory). Output outside the project and JSON written to stdout
may contain the private layered ids.

## `replan version`

Purpose:

Report the binary version, source commit, reproducible build date, Go
toolchain, target OS/architecture and the current RePlan project file-format
version. This command does not load a project or touch files.

```bash
replan version
replan version --format json
replan --version
```

Implemented command flags:

```text
replan version --format
```

Release builds inject version, commit and build date. Other builds fall back
to Go module and VCS build information; a modified VCS checkout adds `+dirty`
to the commit. A build installed with `go install ...@v0.1.0` reports the
module version but has no VCS settings, so its compact line omits commit and
date, for example `replan v0.1.0 (go1.24.1 linux/amd64)`. Its JSON result uses
`"unknown"` for both `commit` and `build_date`. Text output is the compact
build line followed by the project format line. `replan --version` prints only
the compact line. JSON uses the standard envelope and returns `version`,
`commit`, `build_date`, `go_version`, `os_arch` and
`project_format_version` in `result`.

## `replan init`

Create the smallest valid RePlan project without interactive prompts. Dry-run
lists every file; apply creates the directory when needed and writes the
project, local policy, owner actor/resource, empty canonical collections,
empty current plan and `project_init` audit record. Initialization refuses an
existing `.replan/` directory or canonical file with `project_exists`.

Implemented command flags:

```text
replan init <dir> --name --owner --id --timezone --dry-run --apply --format --bindings
```

## `replan resource add` and `replan resource list`

Add and inspect canonical resources. `--with-actor` is limited to `human` and
`ai_agent` resources and creates cross-references in both records. Evidence is
optional; every add writes a `resource_add` audit record.

Implemented command flags:

```text
replan resource add --project --id --class --name --capability --with-actor --actor --evidence --dry-run --apply --format --bindings
replan resource list --project --format --bindings
```

## `replan capacity set`

Add a weekly `periodic_capacity`. A later effective period closes the prior
period for the same resource and unit on the preceding day. The default
effective date is the project status date, then today when no status date is
set. Evidence is optional; every set writes a `capacity_set` audit record.

Implemented command flags:

```text
replan capacity set --project --resource --unit --amount --period --effective-from --actor --evidence --dry-run --apply --format --bindings
```

## `replan validate`

Purpose:

Validate project structure, canonical files, references, ids and basic policy.

Examples:

```bash
replan validate --project examples/small-team --format json
replan validate examples/small-team --format markdown
```

Implemented command flags:

```text
replan validate --project -p --bindings --format --strict
```

Positional shortcut:

```text
replan validate <project-path>
```

Required flags:

```text
--project / -p
--format
```

Optional flags in the wider contract (`--strict` is implemented; the other
two are not):

```text
--strict
--warnings-as-errors
--scope all|canonical|storage|imports|reports|audit
```

JSON result shape:

```json
{
  "valid": true,
  "checked_files": 12,
  "object_counts": {
    "tasks": 10,
    "resources": 5,
    "resource_demands": 18
  }
}
```

Exit behavior:

- `0` when valid, even with warnings;
- `2` when validation errors exist;
- `8` for invalid command usage.

MVP requirement:

This should be the first implemented command.

## `replan workload`

Purpose:

Calculate resource workload, capacity usage, overloads and missing-data
warnings.

```text
replan workload --project <path> [--week <YYYY-Www>] [--resource <id>] [--resource-class <class>] [--unit <unit>] [--scenario <id>] [--demands] [--format json|markdown|text]
```

Examples:

```bash
replan workload --project examples/small-team --week 2026-W28 --format json
replan workload --project examples/small-team --week 2026-W28 --resource-class human --unit hours --format markdown
replan workload --project examples/small-team --resource resource.human.ivan --week 2026-W28 --format json
replan workload --project examples/small-team --week 2026-W28 --scenario scenario.task-op-1234-reassign-petr --format json
```

Implemented command flags:

```text
replan workload --project -p --bindings --format --demands --week --resource --resource-class --unit --scenario
```

Time selector:

- `--week <YYYY-Www>` keeps actuals whose status date, or occurrence date
  when there is none, falls in that ISO week, and demands whose own timing,
  or their task's planned window when the demand has none, intersects it.
  Undated actuals and demands count in every week. A malformed value returns
  exit `8`;
- without `--week` the command sums over all weeks: every actual and every
  demand counts. Reports and `next` default to the week of the project status
  date; `workload` does not, so the all-weeks view stays available.

Demands over time (GAP-031):

- a demand's window is its own timing, or its task's planned dates;
- a period demand (hours, agent hours, minutes, tokens) is spread evenly over
  the working days of its window, the weekdays its resource's availability
  windows name, or every day for a resource without availability windows. A
  window with no working day, such as a weekend, keeps every day. With
  `--week`, a demand counts only the share of its working days inside the
  week, so an 8-hour task over two working weeks counts 4 hours in each;
- a demand whose resource has `period: concurrent` capacity, or whose unit is
  `concurrent_slots`, occupies its amount on every day of its window. Planned,
  remaining and reserved for such a resource are the peak of simultaneous use
  on any day of the week, or of all dated windows without `--week`, never a
  sum, so tasks that use one slot one after another do not overload it;
- undated demands count in full in every week and on every day;
- the dashboard Gantt load and the OpenProject overlay use the same rule:
  daily amounts fall on working days only, a concurrent load's weekly bucket
  is its peak day, and each bucket's capacity is the capacity in effect in
  that week, prorated by working days for a partial week at the edge of the
  range.

Capacity and effective dates:

- with `--week`, a capacity counts only for the days of that week between its
  `effective_from` and `effective_to`. A `period: week` capacity counts its
  share of the resource's working days in effect, one seventh a day for a
  resource without availability windows, so a change on a Wednesday splits
  that week; any other period counts in full if it is in effect on any day of
  the week;
- without `--week`, the capacity in effect on the project status date counts.
  A project without a status date counts every capacity record;
- a malformed effective date, or `effective_to` before `effective_from`, is a
  validation error.

Filters:

- `--resource <id>` limits the calculation to one resource or pool;
- `--resource-class <class>` and `--unit <unit>` keep only result rows of that
  class or unit. They filter the output, not the calculation, so a kept row
  has the same numbers as without the filter, and warnings about dropped rows
  are dropped with them;
- `--scenario <id>` applies a scenario in memory only.

Demand breakdown:

- `--demands` lists in every JSON row the demands that make it up, ordered by
  id. Each entry has `resource_demand_id`, `task_id`, `status`, `share` and
  `planned`, `reserved` and `remaining`.
  - For a prorated demand, `share` is the fraction of the demand in the week
    (1 without `--week`), and the amounts are the demand's amounts times
    that share.
  - For a concurrent demand, the entry has `concurrent: true` and the
    demand's own amounts, while the row holds the peak of the demands active
    on the same day.
- Markdown and text output render the selected week (or `all weeks`) and one
  row per resource with name, class, unit, capacity, planned, actual,
  remaining and overload. With `--demands`, each resource row is followed by
  its contributing demands. Warnings and errors follow the rows.
- The schedule drift check and the overload explainer in `scripts/` read
  their weekly rows from this command instead of computing them (finding
  C-15 of the 2026-10-15 agent tooling review).

Not implemented, and not promised by this spec: `--task`,
`--include-scenarios`, `--from`/`--to` and `--status-date`. Earlier drafts
listed them; they were removed from the contract on 2026-09-25 (GAP-011)
rather than left as silently ignored flags. The binary rejects them as unknown
flags.

JSON result shape:

```json
{
  "project_path": "examples/small-team",
  "week": "2026-W28",
  "scenario_id": "scenario.task-op-1234-reassign-petr",
  "resources": [
    {
      "resource_id": "resource.human.ivan",
      "resource_name": "Ivan",
      "resource_class": "human",
      "unit": "hours",
      "capacity": 32,
      "planned": 40,
      "reserved": 0,
      "actual": 6,
      "remaining": 34,
      "overload": 2
    }
  ],
  "warnings": [],
  "errors": []
}
```

The object above is the `result` of the standard envelope. `overload` is the
load over capacity, where load is `reserved` when it is set and `remaining`
otherwise.

Rules:

- command is read-only;
- output must include warnings for unknown capacity and unknown remaining work;
- calculations must use canonical model, not source raw fields;
- scenario workload must apply selected scenario changes in memory only;
- scenario workload must not mutate current plan.

## `replan report`

Purpose:

Generate human-readable and machine-readable reports.

Subcommands:

```text
replan report weekly
replan report estimates
replan report state
replan report dashboard
replan report workload
replan report tracking
replan report warnings
replan report scenario
replan report proposals
```

Examples:

```bash
replan report weekly --week 2026-W28 --format markdown
replan report weekly --week 2026-W28 --format json
replan report weekly --week 2026-W28 --format html --write
replan report estimates --project examples/replan-self --baseline baseline.2026-10-15.tooling-consolidation --format markdown
replan report state --project examples/replan-self --week 2026-W31 --format markdown
replan report state --project examples/replan-self --week 2026-W31 --format json
replan report dashboard --project examples/replan-self --week 2026-W31 --output reports/latest --format html
replan tracking suggest --project examples/replan-self --week 2026-W31 --format json
replan report warnings --severity warning --format json
replan report scenario scenario.ivan-unavailable-week-28 --against current --format markdown
```

Implemented command flags:

```text
replan report weekly --project -p --bindings --format --week --scenario --write
replan report estimates --project -p --bindings --baseline --format
replan report state --project -p --bindings --format --week --scenario
replan report dashboard --project -p --bindings --format --output --week --scenario
replan next --project -p --bindings --format --week
```

Specified output flags across the wider report design:

```text
--format markdown|json|csv|html|text
--output reports/weekly/2026-W28.md
--write
```

Implemented `report weekly` has `--write` but not `--output`; implemented
`report dashboard` has `--output`; `report state` and `next` write only to
stdout.

Implemented estimate-accuracy report slice:

- `report estimates` is read-only, requires `--baseline`, defaults to Markdown
  and supports JSON or Markdown;
- the named baseline must carry estimate fields and have a verified SHA-256
  seal. An unknown id returns `baseline_not_found` with exit 4; a ref-only
  baseline returns `baseline_has_no_estimates` with exit 2 and names
  `replan baseline create`; an absent, unsupported or changed seal returns
  `baseline_modified` with exit 2;
- consumption is the sum, in the demand's unit, of demand-linked actuals whose
  type is `work_consumed`, `consumption` or `runtime_consumed`. Other actual
  types and mismatched units do not enter the amount. Every summed or derived
  result amount and ratio is rounded to six decimal places before JSON or
  Markdown rendering; canonical actuals and estimates are not rewritten;
- every baseline demand is `current` or `removed`; every current demand on a
  baseline task without the same baseline demand id is `added`. A changed
  planned amount on the same id is a re-estimate. Task totals are separate per
  unit;
- `accepted`, `done` and `closed` are treated as finished report statuses.
  Other statuses are open, so their actuals are explicitly partial. The
  current canonical vocabulary uses `accepted` as its completed state;
- actual start and finish first use `actual_add_status` audit records that
  carry `new_status` and `status_date`: first `in_progress` starts, and first
  finished status ends. Older audit records do not carry those facts and are
  not guessed from their reason or evidence. Each missing date falls back to
  the earliest or latest consumption-actual `status_date`, then its
  `occurred_at` date; otherwise it is empty with source `none`;
- finish variance is actual finish minus baseline finish in calendar days and
  in working days. Project working weekdays are the union of configured
  resource availability weekdays, matching the existing workload working-day
  helper; without configured weekdays, every calendar day counts;
- a split comparison is emitted for an added resource absent from the whole
  baseline. It uses the same-task, same-unit baseline demand whose current
  planned amount has the largest positive reduction, with demand id as a
  deterministic tie-break. If none exists, it compares with the task's whole
  baseline total for that unit. Normal demand and resource totals still keep
  an added demand's literal baseline at zero;
- `split_groups` identifies every original baseline resource with a recognized
  split, then combines that original resource's complete summary with the
  complete summaries of its split-out resources, separately per unit and for
  finished and all tasks;
- a resource row has `actuals_recorded: false` when no counted demand actual in
  the baseline tasks uses its unit. Its ratios remain null and Markdown says
  `no actuals`, rather than presenting an observed zero. Such a resource is
  omitted when resource rows are combined into a split group.

Scenario reports carry `scenario_id` and every Markdown, text and HTML view
labels them: `What-if: scenario <id> applied in memory; this is not the plan.`
`report weekly --write --scenario` is refused with
`scenario_report_write`. `report dashboard --scenario --output` is allowed
only when the output directory is outside the project; an in-project output is
refused with the same code. This prevents a what-if artifact from occupying the
canonical report location. Dashboard manifests and OpenProject overlay JSON
also carry `scenario_id`; `publish-openproject-overlay.sh` refuses those
artifacts with `overlay_is_scenario` instead of publishing a what-if as the
plan.

Rules:

- without `--output` or `--write`, report is printed to stdout;
- with `--output`, report is written to requested path;
- with `--write`, report is written to default path under `reports/`;
- JSON report output is canonical machine-readable report output;
- Markdown report output is human-readable report output;
- generated report files are not source of truth;
- report generation should emit `ReportSnapshot` metadata when written.

Implemented weekly report slice:

- `replan report weekly` supports JSON, Markdown, CSV, text and static HTML;
- `--format html` writes a self-contained static HTML document to stdout;
- `--format html --write` writes
  `reports/weekly/<week>.html` inside the project and returns a JSON envelope
  with `output_path`;
- HTML is rendered from the same weekly report model as JSON, Markdown and CSV;
- HTML generation does not require a daemon, server, browser automation or
  network access;
- generated HTML is a report artifact, not canonical planning state.

Implemented dashboard report slice:

- `replan report dashboard` supports JSON and static HTML;
- with `--output reports/latest`, it writes `index.html`, `gantt.html`,
  `workload.html`, `scenarios.html`, `proposals.html`, `estimates.html`,
  `raw/overview.json`, `raw/gantt.json`, `raw/openproject_overlay.json`,
  `raw/workload.json`, `raw/scenarios.json`, `raw/proposals.json`,
  `raw/estimates.json` and `raw/manifest.json`;
- dashboard generation is local, read-only and does not mutate canonical
  planning state;
- raw JSON artifacts are first-class outputs for AI CLI agents and diagnostics;
- `gantt.html` is a focused read-only timeline page for human comparison with
  OpenProject Gantt and for seeing RePlan resource/warning badges on the task
  schedule;
- the first dashboard slice is intentionally a generated static artifact set,
  not a daemon, server API or mutating web UI.
- workload, scenario, proposal and estimate pages are read-only. Their shown
  commands are copyable text; browser pages do not execute them or write state.

Implemented project-state report slice:

- `replan report state` supports JSON, Markdown and text;
- JSON returns the dashboard overview model inside the standard CLI envelope;
- Markdown renders a compact operator-facing view of synchronization health,
  planning health, tracking health, Plan versus Actual rows, resource
  pressure, warnings and next actions;
- Plan versus Actual rows show task-level tracking flags and per-demand
  planned, actual, remaining, forecast and variance amounts without mixing
  different resource units;
- tracking proposal loop rows show stale remaining demand rows, open/accepted
  tracking proposals, covered demands, actionable demands and the next safe
  command for unresolved stale remaining work;
- tracking proposal loop rows include normalized status, priority and reason so
  humans and AI CLI agents can choose the next row without scanning all stale
  remaining rows manually;
- generated dashboard HTML renders the same tracking proposal loop summary and
  a compact row table on `reports/latest/index.html`;
- without `--week`, the report and `tracking suggest` use the week of the
  project's `default_status_date`; a baseline's status date is only the
  fallback when the project has none;
- next actions carry `actor` (`person` or `agent`) and, where there is one,
  `date` (the task's planned finish). Markdown and HTML group them under
  "Waiting on a person" and "An agent can prepare"; within a group they are
  ordered by date, earliest first, then undated;
- next actions that wait on a person: `accept_or_return_task` for
  `review_ready` tasks, `do_human_work` for `in_progress` tasks whose only
  active remaining demands are `human` or `role`, `past_planned_finish` for
  open tasks whose planned finish is before the status date, open proposals,
  overloaded resources, and open dependencies on something other than a task;
- next actions an agent can prepare: `record_actual_window` for accepted
  tasks whose planned finish is after the status date, the same condition as
  the warning `accepted_task_planned_after_status_date` from `validate` and
  the weekly report; `review_tracking` for tracking flags,
  except overruns an operator acknowledged with `actual acknowledge-overrun`,
  and `resolve_dependency` for task dependencies whose predecessor is already
  in the required state. A dependency on a task that is not done yet is not a
  next action; its warning stays;
- when nothing qualifies, the only action is `inspect_project` without an
  actor;
- `replan next --project <path> [--week <YYYY-Www>] [--format text|json]` is
  the short view of the same overview for a person at a terminal: the project
  line, "Waiting on a person" and "An agent can prepare" with task titles
  instead of ids where there is a title, and one health line. No tables. JSON
  returns `waiting_on_person`, `agent_can_prepare` and `health` in the standard
  envelope. Text is the default format. `inspect_project` is not listed: an
  empty group reads "nothing";
- the report is local, read-only and does not mutate canonical planning state;
- the report does not require a daemon, server, browser, custom authentication,
  full role-based access control or live OpenProject access.

## `replan project`

Implemented command flags:

```text
replan project migrate --project -p --bindings --actor --reason --occurred-at --apply --dry-run --format
replan project recover --project -p --bindings --apply --dry-run --format
replan project set-status-date --project -p --bindings --date --actor --reason --occurred-at --apply --dry-run --format
```

```text
replan project migrate --project <path> --actor <id> [--reason <text>] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|markdown]
replan project recover --project <path> (--dry-run|--apply) [--format json|markdown|text]
replan project set-status-date --project <path> --date <YYYY-MM-DD|today> --actor <id> [--reason <text>] [--occurred-at <RFC3339>] (--dry-run|--apply) [--format json|markdown]
```

`project migrate` reports `from_version`, the binary's current `to_version`,
the ordered migration steps and planned writes. Missing `format_version`
means version 1; on a format-1 project the only step inserts the field without
rewriting unrelated YAML and appends one `project_migrate` audit record.
Already-current projects are successful no-ops with no audit record. The
command upgrades only: a newer format is refused by project loading with
`project_format_unsupported`.

`project recover` inspects or completes `.replan/commit-pending/`. `--dry-run`
validates a complete journal and lists its files without changing the project;
`--apply` replays it under `.replan/write.lock` and removes the journal. No
pending journal is a successful no-op with zero recovered files. Corrupt
journal content is refused before any project file is touched.

The project status date (`project.default_status_date` in
`.replan/project.yaml`) sets the default week of `report state`, `next` and
`tracking suggest`, and decides which open tasks are past their planned
finish. `set-status-date` moves it through an audited command instead of a
hand edit:

- `--date today` is the current date in the project's `timezone`; an unknown
  or empty time zone falls back to the local one;
- only the `default_status_date` field changes; the file is edited as a YAML
  node tree, so fields and comments the domain type does not model survive;
- one `project_set_status_date` audit record; the would-be project is
  validated before the write is committed, as for `actual` commands;
- errors: `invalid_status_date`, `missing_actor`, `actor_not_found`,
  `status_date_unchanged`; moving the date back is allowed with the warning
  `status_date_moved_backwards`;
- the result reports `previous_date`, `new_date` and the new default `week`.

## `replan tracking`

Purpose:

Inspect execution-tracking signals and suggest safe operator follow-up actions
without mutating canonical state.

Subcommands:

```text
replan tracking suggest
```

Examples:

```bash
replan tracking suggest --project examples/replan-self --week 2026-W31 --format json
replan tracking suggest --project examples/replan-self --week 2026-W31 --format markdown
replan tracking suggest --project examples/replan-self --week 2026-W31 --write-candidates --actor actor.ai.codex.local --format json
```

Implemented command flags:

```text
replan tracking suggest --project -p --bindings --format --week --actor --write-candidates
```

Implemented tracking suggestion slices:

- `replan tracking suggest` supports JSON, Markdown and text;
- JSON uses the standard CLI envelope and returns a `suggestions` array;
- default mode is read-only and does not write proposals, actuals, audit
  records or canonical planning files;
- the first suggestion types are:
  - `set_remaining_to_zero_candidate` for accepted or done tasks that have
    actual consumption and still show positive remaining work;
  - `review_missing_actuals` for accepted or done tasks that have resource
    demands but no actual consumption records;
  - `review_missing_evidence` for actual consumption records without evidence
    references;
  - `review_forecast_over_plan` for demand rows where
    `actual + remaining` exceeds planned amount and the stale-remaining
    candidate rule does not explain the case;
- stale-remaining candidates include a recommended
  `replan actual set-remaining --dry-run` command, but the operator or AI CLI
  agent must still review and execute the change explicitly;
- `--write-candidates --actor <id>` writes supported suggestions as
  reviewable `proposals/open/*.yaml` files;
- write-candidates mode currently materializes only
  `set_remaining_to_zero_candidate` as `update_remaining_work` proposals with
  `suggested_change.path = remaining_amount` and
  `suggested_change.value = 0`;
- write-candidates mode still does not mutate resource demands, actuals, audit
  records, evidence records or OpenProject;
- after candidates are written, the existing `proposal list/show/accept/reject`
  loop is responsible for explicit review and acceptance.

## `replan scenario`

Purpose:

Create, inspect, calculate, diff and apply what-if overlays.

Subcommands:

```text
replan scenario list
replan scenario show
replan scenario create
replan scenario patch
replan scenario calculate
replan scenario diff
replan scenario propose-apply
replan scenario apply
```

Examples:

```bash
replan scenario create --name ivan-unavailable-week-28 --from current --format json
replan scenario patch scenario.ivan-unavailable-week-28 --add-availability-exception resource.human.ivan:2026-07-08:unavailable
replan scenario calculate scenario.ivan-unavailable-week-28 --format json
replan scenario diff scenario.ivan-unavailable-week-28 --against current --format markdown
replan scenario apply scenario.ivan-unavailable-week-28 --dry-run --format json
replan scenario apply scenario.let-validation-go-ahead --actor actor.human.operator --evidence evidence.decision.20260927 --apply --format json
```

Implemented command flags:

```text
replan scenario list --project -p --bindings --format
replan scenario show --project -p --bindings --format
replan scenario diff --project -p --bindings --format --against --week
replan scenario apply --project -p --bindings --format --actor --apply --dry-run --evidence --reason
```

`scenario list --format markdown|text` renders id, name, status and change
count. `scenario show --format markdown|text` renders id, name, status,
description and every change as target, field, previous value and new value.
The JSON envelope and result shapes are unchanged.

Rules:

- scenario is overlay, not full project copy;
- `list`, `show`, `calculate` and `diff` are read-only;
- `create` and `patch` mutate scenario files and require actor/audit;
- `apply` mutates current plan and must support dry-run;
- AI agents should normally use `propose-apply` instead of direct `apply`;
- applying scenario must not mutate accepted actuals, historical evidence or
  old baselines.
- implemented first apply slice supports `resource_demands.<id>.remaining_amount`
  and `resource_demands.<id>.planned_amount`;
- implemented task-date apply slice supports `tasks.<id>.planned_start` and
  `tasks.<id>.planned_finish`;
- `dependencies.<id>.status` set to `cancelled` records that the operator let
  the successor go ahead of its predecessor (GAP-032). It requires
  `--evidence <evidence-id>` naming that decision; the audit record carries
  it. Other dependency statuses stay with proposals;
- `--evidence` is optional for other scenarios and is checked to exist;
- scenario apply marks the scenario as `applied`, writes audit and validates
  after mutation.

MVP minimum:

```text
list
show
create
diff
apply --dry-run
apply --actor
```

## `replan proposal`

Purpose:

Create, inspect, accept and reject suggested changes.

Subcommands:

```text
replan proposal list
replan proposal show
replan proposal suggest-dependencies
replan proposal create
replan proposal accept
replan proposal reject
replan proposal supersede
```

Examples:

```bash
replan proposal list --status open --format json
replan proposal show proposal.adjust-remaining-task-op-1234.20260706 --format markdown
replan proposal suggest-dependencies --dry-run --format json
replan proposal suggest-dependencies --write-candidates --actor actor.ai.codex.local --format json
replan proposal accept proposal.adjust-remaining-task-op-1234.20260706 --dry-run --format json
replan proposal accept proposal.adjust-remaining-task-op-1234.20260706 --actor actor.human.operator --apply
replan proposal reject proposal.adjust-remaining-task-op-1234.20260706 --actor actor.human.operator --reason "Evidence not sufficient" --apply
```

Implemented command flags:

```text
replan proposal list --project -p --bindings --format --status
replan proposal show --project -p --bindings --format
replan proposal suggest-dependencies --project -p --bindings --format --actor --dry-run --write-candidates
replan proposal accept --project -p --bindings --format --actor --apply --dry-run --reason
replan proposal reject --project -p --bindings --format --actor --apply --dry-run --reason
```

Rules:

- proposal acceptance/rejection is mutating;
- acceptance/rejection must produce audit;
- mutating acceptance/rejection requires `--actor`;
- rejection requires `--reason`;
- acceptance may mutate current plan, actuals or scenario state depending on
  proposal type;
- rejection must preserve reason when provided;
- `suggest-dependencies --dry-run` is preview-only;
- `suggest-dependencies --write-candidates --actor <id>` writes reviewable
  proposal candidate files only;
- AI agents may create proposals;
- AI agents should not accept proposals unless policy explicitly allows.

MVP minimum:

```text
list
show
suggest-dependencies --dry-run
accept --dry-run
accept
reject --dry-run
reject
```

Implemented first decision-gate slice:

- `accept --dry-run` and `reject --dry-run` preview proposal status moves and
  audit append;
- `accept --actor <id>` moves an open proposal to `proposals/accepted/`, updates
  status to `accepted`, writes audit and validates the project;
- `reject --actor <id> --reason <text>` moves an open proposal to
  `proposals/rejected/`, updates status to `rejected`, writes audit and
  validates the project;
- this slice does not mutate canonical task/resource/current-plan state.

Implemented dependency preview slice:

- `suggest-dependencies` returns dry-run proposal candidates derived from
  canonical dependency warnings;
- it returns ordinary proposal-shaped objects with status `open` and
  `requires_confirmation: true`;
- it may preview `review_dependency_resolution`,
  `review_dependency_schedule`, `adjust_successor_start` and
  `mark_successor_blocked`;
- it does not mutate tasks, dates, dependencies or OpenProject.

Implemented dependency candidate write slice:

- `suggest-dependencies --write-candidates --actor <id>` writes
  `proposals/open/*.yaml` files for new dependency proposal candidates;
- write mode requires a known actor;
- existing proposal ids are skipped and reported as
  `proposal_candidate_already_exists` warnings;
- write mode validates the project after files are created;
- write mode still does not accept proposals, mutate tasks, move dates, change
  dependencies or write back to OpenProject.

Implemented dependency proposal accept/apply slice:

- accepting `mark_successor_blocked` moves the proposal to `accepted`, updates
  the target successor task `tracking_status` to `blocked`, appends audit and
  validates the project;
- accepting `mark_dependency_resolved` moves the proposal to `accepted`, updates
  the dependency `status` to `resolved`, appends audit and validates the
  project;
- accepting `mark_successor_ready` moves the proposal to `accepted`, updates
  the target successor task `tracking_status` to `ready`, appends audit and
  validates the project;

Implemented remaining-work proposal accept/apply slice:

- accepting `update_remaining_work` moves the proposal to `accepted`, updates
  the target `resource_demand.remaining_amount`, appends audit and validates
  the project;
- the supported shape is:
  - `target_ref.kind = resource_demand`;
  - `suggested_change.op = set`;
  - `suggested_change.path = remaining_amount`;
  - numeric non-negative `suggested_change.value`;
- `accept --dry-run` includes `canonical/resource_demands.yaml` in planned
  writes when the proposal would change remaining work;
- rejecting `update_remaining_work` keeps the normal rejection behavior:
  proposal status move plus audit, without changing resource demands;
- this slice does not mutate actuals, evidence records or OpenProject.
- accepting `review_dependency_resolution`, `review_dependency_schedule` and
  `adjust_successor_start` is accepted-only in this slice;
- accepting dependency proposals does not move dates or write back to
  OpenProject.

## `replan dependency`

Purpose:

Inspect canonical dependency lifecycle state without mutating the project.

Subcommands:

```text
replan dependency list
```

Examples:

```bash
replan dependency list --project examples/small-team --format json
replan dependency list --project examples/small-team --format markdown
```

Implemented command flags:

```text
replan dependency list --project -p --bindings --format
```

Implemented dependency overview slice:

- lists dependency id, type, status, criticality, predecessor and successor;
- includes successor task tracking status when the successor is a task;
- marks dependencies as `blocking` when they are active required blockers or
  active `blocks` relations; `relates` is always shown as a non-blocking link;
- marks dependencies as `resolvable` when the predecessor task already
  satisfies the required state;
- lists blocked successor tasks whose required dependencies are all resolved or
  cancelled;
- includes linked open proposal ids for dependency and task targets;
- read-only; does not write proposals, mutate tasks, mutate dependencies or
  write back to OpenProject.

## `replan estimate`

Purpose:

Create, inspect and apply sealed estimate calibrations from clean finished-task
samples.

Implemented command flags:

```text
replan estimate calibrate --project -p --bindings --format --baseline --resource --unit --tag --exclude --id --name --approved-by --occurred-at --dry-run --apply
replan estimate list --project -p --bindings --format
replan estimate show --project -p --bindings --format
replan estimate apply --project -p --bindings --format --calibration --scenario-id --actor --occurred-at --dry-run --apply
```

`calibrate` requires eight clean finished tasks, a verified estimate baseline
approved before each task's derived actual start, counted actuals in the scope
unit and a positive baseline amount. Multiple `--resource` values form one
split group. `--exclude task-id=reason` records explicit exclusions. The factor
is the median per-task actual/baseline ratio; the record also stores total
ratio, interpolated p25 and p75, and medians for baseline-size bands up to 2,
up to 4 and above 4. Apply creates a sealed, immutable
`calibrations/<id>.yaml` and an audit record. The approver must be human.

`estimate apply` creates one ordinary scenario linked by `calibration_ref`.
It does not yet skip demands whose amount was set by a person after the calibration date because the current audit and demand history do not reliably identify that edit.
It scales open in-scope planned and recorded remaining amounts, rounds to the
nearest quarter-unit with a quarter-unit floor, and skips closed demands or
demands whose counted actuals exceed the scaled plan. The existing `scenario
diff` and `scenario apply` commands inspect and apply it.

`task add` reports the latest matching calibrated value beside each typed
demand without changing the stored amount. `next` emits one actionable
`estimate apply` item per calibration with per-demand values in JSON, hides it
after a scenario references the calibration, and suggests `estimate calibrate`
only for maximal scopes with at least eight clean finished tasks not used by a
same-unit calibration over a resource superset.

## `replan baseline`

Purpose:

Create and inspect immutable plan snapshots with task dates, original demand
amounts and a semantic SHA-256 seal.

```bash
replan baseline create --project examples/small-team \
  --id baseline.2026-07-08.reviewed --name "Reviewed plan" \
  --approved-by actor.human.operator --status-date 2026-07-08 \
  --assumption "Fixture estimates reviewed" \
  --occurred-at 2026-07-08T10:00:00+07:00 --dry-run --format text
replan baseline list --project examples/small-team --format markdown
replan baseline show baseline.2026-07-08.reviewed \
  --project examples/small-team --format json
```

Implemented command flags:

```text
replan baseline create --project -p --bindings --format --id --name --approved-by --status-date --assumption --occurred-at --dry-run --apply
replan baseline list --project -p --bindings --format
replan baseline show --project -p --bindings --format
```

Rules:

- `create` snapshots every current-plan task in plan order and every demand of
  those tasks, using `planned_amount`, not remaining work;
- the id and approving actor must already be unique and known respectively;
  `created_at` and `approved_at` both use `--occurred-at`, or the current time;
- dry-run reports the target file, counts and preview. Apply creates
  `baselines/<id>.yaml`, appends a `baseline_create` audit event, validates the
  staged project and then commits it;
- the canonical apply gate and read-only bindings-layer refusal apply;
- `list` and `show` report whether estimate fields exist and whether the seal
  is `verified`, `modified`, `unsupported` or `unsealed`.

## `replan task add`

Purpose:

Create one canonical RePlan task, its resource demands, its ordered current-plan
reference and one audit record as a single staged, validated write.

```bash
replan task add --project examples/small-team \
  --id task.op.document-api --title "Document API" \
  --planned-start 2026-07-16 --planned-finish 2026-07-17 \
  --tag docs --assignee resource.human.ivan --after task.op.1234 \
  --demand implementer=resource.human.ivan:3:hours \
  --demand agent=resource.ai.codex.local:2:agent_hours:ai_coding_agent \
  --actor actor.human.operator --evidence evidence.github.pr.45 \
  --dry-run --format json
```

Implemented command flags:

```text
replan task add --project -p --bindings --format --actor --after --apply --assignee --demand --description --dry-run --evidence --id --occurred-at --planned-finish --planned-start --tag --title --type
```

Rules:

- `--id`, `--title`, at least one repeatable `--demand`, `--actor` and
  `--evidence` are required. `--type` defaults to `implementation`;
- a demand is `SUFFIX=RESOURCE_ID:AMOUNT:UNIT[:ROLE]`. The demand id is
  `resource_demand.<task-slug>.<SUFFIX>`, where `<task-slug>` is the task id
  after its leading `task.<namespace>.` (for example,
  `task.self.add-command` becomes `add-command`). The suffix is lowercase
  letters, digits, `_` or `-`, beginning with a letter or digit;
- the resource must exist; `resource_class` is copied from it; the unit must
  occur in a capacity row for that resource; the amount is finite and greater
  than zero; `remaining_amount` starts equal to `planned_amount`; role defaults
  to `SUFFIX` when omitted; new demands are `required` and `active`;
- the task starts with `tracking_status: not_started`. The current-plan
  reference is appended, or inserted immediately after the existing included
  task named by `--after`;
- duplicate task or demand ids, unknown resources, assignees, actors, evidence
  or `--after` tasks, malformed demands, invalid units/dates, a finish before
  the start and a missing demand are refused before disk writes;
- `--apply` stages all three canonical YAML edits and the audit append,
  validates the would-be project, then commits it. The YAML edits use splice
  writes so existing bytes remain unchanged. `--dry-run` writes nothing;
- this command does not create an OpenProject work package. Next use
  `scripts/apply-openproject-bootstrap.sh`, or toolkit `work-package create`
  followed by `bind` for one task.

## `replan task set-dates`

Purpose:

Apply an operator-decided task date change without a scenario, such as
accepting one OpenProject scheduler shift.

```bash
replan task set-dates --project examples/small-team --task task.op.1234 \
  --planned-start 2026-07-14 --planned-finish 2026-07-16 \
  --reason "Accept the reviewed scheduler shift." \
  --actor actor.human.operator --evidence evidence.github.pr.45 \
  --apply --format json
```

Implemented command flags:

```text
replan task set-dates --project -p --bindings --format --actor --apply --dry-run --evidence --occurred-at --planned-finish --planned-start --reason --task
```

Rules:

- `--task`, `--reason`, `--actor`, `--evidence` and at least one of
  `--planned-start` or `--planned-finish` are required;
- dates use `YYYY-MM-DD`; an omitted date keeps its old value; the resulting
  finish must not precede the resulting start; an unchanged result is refused;
- the task, actor and evidence must already exist. `--occurred-at` is optional
  RFC3339 with an offset and defaults to the current time;
- the staged splice write is validated before commit. Its audit record names
  the old and new start and finish, the reason and the evidence;
- pushing accepted dates to OpenProject remains the separate reviewed
  `scripts/push-openproject-date-drift.sh` operation.

## `replan task record-actual-window`

Purpose:

Replace an accepted task's current planned dates with its recorded actual
window while sealed baseline dates remain unchanged.

```bash
replan task record-actual-window --project examples/small-team \
  --task task.op.1234 --actor actor.human.operator \
  --evidence evidence.github.pr.45 --dry-run --format json
```

Implemented command flags:

```text
replan task record-actual-window --project -p --bindings --format --actor --apply --dry-run --evidence --occurred-at --reason --task
```

Rules:

- `--task`, `--actor` and `--evidence` are required; `--reason` is optional;
  all referenced records must exist, and the task tracking status must be
  `accepted`, `done` or `closed`;
- date and source derivation is the same shared rule used by `report
  estimates`: the first `in_progress` and first finished
  `actual_add_status` audit dates take precedence, then current-demand
  consumption actual dates fill missing bounds. The report additionally
  supplies sealed-baseline demand ids; this command deliberately does not;
- when only a finish is known, the earliest consumption date is the start if
  one exists, otherwise the finish is also the start. An actual start after
  the finish is refused as `actual_window_inconsistent`; no derived dates is
  `actual_window_unknown`;
- a derived date on a non-working project-calendar day is preserved and
  reported as `actual_window_on_non_working_day`;
- an unchanged window succeeds with `changed: false`, no planned writes and
  no audit record. A change writes both task dates and one
  `task_record_actual_window` audit record containing the dates, sources and
  evidence;
- JSON result fields are `task_id`, `previous_planned_start`,
  `previous_planned_finish`, `new_planned_start`, `new_planned_finish`,
  `actual_start_source`, `actual_finish_source`, `changed`, `planned_writes`,
  `audit_record_preview`, `warnings` and `errors`;
- unknown task is `task_not_found` (exit 4); unfinished task, unknown window
  and inconsistent window are invalid-plan refusals (exit 2).

## `replan task set-title`

Purpose:

Apply an operator-reviewed correction to the canonical RePlan task title.

```bash
replan task set-title --project examples/small-team --task task.op.1234 \
  --title "Implement validation command safely" \
  --reason "Correct the task title." \
  --actor actor.human.operator --evidence evidence.github.pr.45 \
  --dry-run --format json
```

Implemented command flags:

```text
replan task set-title --project -p --bindings --format --actor --apply --dry-run --evidence --occurred-at --reason --task --title
```

Rules:

- `--task`, a non-empty `--title`, `--reason`, `--actor` and `--evidence` are
  required; the task, actor and evidence must exist;
- `--occurred-at` is optional RFC3339 with an offset and defaults to now;
- an unchanged title is a successful no-op with no canonical or audit write;
- a changed title stages the title splice and audit record, validates the
  would-be project, then commits both on `--apply`. The result shows the old
  and new title;
- the read-only private-bindings layer is refused like every canonical writer.

## `replan evidence add`

Purpose:

Append one operator-reviewed evidence record to the ISO-week shard selected by
its observation timestamp, with one audit record for an applied write.

Example:

```bash
replan evidence add --project examples/small-team \
  --id evidence.manual.review.20260708 \
  --type manual_note --subtype review --source local \
  --url https://example.test/reviews/1 \
  --actor actor.human.operator \
  --observed-at 2026-07-08T10:00:00+07:00 \
  --confidence confirmed --summary "Operator reviewed the result." \
  --target task:task.op.1234 --apply --format json
```

Implemented command flags:

```text
replan evidence add --project -p --bindings --format --actor --apply --confidence --dry-run --id --observed-at --source --subtype --summary --target --type --url
```

Rules:

- `--id`, `--type`, `--actor`, `--observed-at`, `--summary` and at least one
  repeatable `--target KIND:ID` are required; ids are never generated;
- target kinds are `project`, `task`, `resource`, `resource_demand`, `actual`,
  `proposal`, `dependency`, `scenario`, `capacity`, `baseline` and
  `audit_record`; a `project` target must use the id from
  `.replan/project.yaml`, and every other target id and the actor id must
  already exist in the project;
- `--observed-at` is RFC3339 with an offset and selects
  `evidence/evidence.<ISO week>.yaml`;
- `replan validate` requires a non-empty evidence type but does not enumerate
  values. Therefore `--type` accepts non-empty free text, while optional
  `--subtype`, `--source` and `--confidence` accept free text or may be
  omitted;
- a duplicate evidence id is refused before writing;
- `--dry-run` reports the file path, evidence item, planned YAML append and
  audit preview without writing; `--apply` (or `REPLAN_APPLY=1`) stages the
  evidence and audit append, validates the resulting project, and commits only
  a valid result;
- a missing week file is created with the `evidence:` header. An existing file
  is updated by the splice writer, so all existing bytes stay unchanged and
  the new item is appended at the end.

## `replan actual`

Purpose:

Record accepted execution facts and remaining work updates.

Subcommands:

```text
replan actual list
replan actual add
replan actual add-consumption
replan actual set-remaining
replan actual close-demand
replan actual reopen-demand
replan actual acknowledge-overrun
replan actual add-status
```

Examples:

```bash
replan actual add-consumption --task task.op.1234 --demand demand.task-op-1234.implementer --resource resource.human.ivan --amount 3 --unit hours --evidence evidence.manual.time-entry.20260706.1 --actor actor.human.operator --apply
replan actual set-remaining --demand demand.task-op-1234.implementer --amount 5 --unit hours --reason "Operator review after pull request" --actor actor.human.operator --apply
replan actual close-demand --demand demand.task-op-1234.implementer --status consumed --reason "Accepted task demand reviewed" --actor actor.human.operator --apply
replan actual add-status --task task.op.1234 --status review_ready --evidence evidence.github.pr.45 --dry-run --format json
replan actual reopen-demand --demand demand.task-op-1234.implementer --remaining 2 --reason "Closed before the task finished" --actor actor.human.operator --apply
replan actual acknowledge-overrun --demand demand.task-op-1234.implementer --reason "Second review round was real work" --actor actor.human.operator --apply
```

Implemented command flags:

```text
replan actual list --project -p --bindings --format --demand --resource --task --type
replan actual add-consumption --project -p --bindings --format --actor --amount --apply --demand --dry-run --evidence --occurred-at --resource --status-date --task --unit
replan actual set-remaining --project -p --bindings --format --actor --amount --apply --demand --dry-run --evidence --occurred-at --reason --status-date --unit
replan actual close-demand --project -p --bindings --format --actor --apply --demand --dry-run --evidence --occurred-at --reason --status --status-date
replan actual reopen-demand --project -p --bindings --format --actor --apply --demand --dry-run --evidence --occurred-at --reason --remaining
replan actual acknowledge-overrun --project -p --bindings --format --actor --apply --demand --dry-run --evidence --occurred-at --reason
replan actual add-status --project -p --bindings --format --actor --apply --dry-run --evidence --occurred-at --reason --status --status-date --task
```

Rules:

- actual means accepted fact;
- actual commands mutate canonical actual records and require actor/audit;
- actual commands should support dry-run;
- actual commands should reference evidence where available;
- actual commands must not rewrite baseline;
- demand lifecycle close supports only `consumed` and `cancelled`;
- closing as `consumed` also sets `remaining_amount: 0` in the same write and
  audit record, because a consumed demand has nothing left; the result reports
  `remaining_cleared` and `previous_remaining`. Closing as `cancelled` keeps
  the remaining amount as the work that was not done;
- `reopen-demand` returns a `consumed` or `cancelled` demand to `active` and
  can record a new remaining amount, including `0`;
- `acknowledge-overrun` records that an operator reviewed a forecast above
  plan and accepts it as real. `tracking suggest` stays quiet for that demand
  while the forecast does not exceed the acknowledged amount, and suggests it
  again when the forecast grows. It refuses a demand without an overrun;
- `remaining_amount: 0` is stored explicitly and means "estimated, nothing
  left"; an absent field means "not estimated";
- status actuals must use canonical RePlan statuses;
- every writing command validates the would-be project before any file
  changes. When validation fails, nothing is written. When the final commit
  itself fails part way, for example on a permission error, the command
  reports `commit_failed` and `written_files` lists exactly the files that
  did reach the disk. Its validated change stays in `.replan/commit-pending/`,
  and the next write or `replan project recover --apply` completes it, so a
  failed commit is finished later rather than left half-written. Audit ids
  name the command's target, so a batch that
  shares one `--occurred-at` does not collide.

MVP minimum:

```text
list
add-consumption
set-remaining
close-demand
add-status --dry-run
```

## `replan import`

Purpose:

Import source data into `imports/`, map it to canonical candidates and optionally
accept mapped canonical changes.

OpenProject and GitHub Issues file and live API input are implemented. Both map source data
to staged candidates and write snapshots only when `--dry-run` is absent;
canonical state changes only through `import accept`.

Subcommands:

```text
replan import openproject
replan import github
replan import gitlab
replan import csv
replan import accept
```

MVP priority:

```text
openproject read-only import
```

Examples:

```bash
replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
replan import openproject --project examples/small-team --profile openproject.demo --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
replan import openproject --project examples/small-team --profile openproject.demo --since 2026-07-01 --dry-run --format json
replan import openproject --project examples/small-team --profile openproject.demo --since 2026-07-01 --include-closed --include-time-entries --dry-run --format json
replan import openproject --project examples/small-team --profile openproject.demo --since 2026-07-01 --include-relations --dry-run --format json
replan import openproject --project examples/small-team --profile openproject.demo --since 2026-07-01 --resource-links-sidecar integrations/openproject/resource-links.yaml --dry-run --format json
replan import openproject --project examples/small-team --profile openproject.demo --since 2026-07-01 --resource-demands-sidecar integrations/openproject/resource-demands.yaml --dry-run --format json
replan import openproject --project examples/small-team --profile openproject.demo --work-package 1234 --dry-run --format json
replan import github --project examples/small-team --profile github.demo --from-file examples/imports/github/issues.sample.json --include-closed --include-relations --resource-links-sidecar ../imports/github/resource-links.sample.yaml --resource-demands-sidecar ../imports/github/resource-demands.sample.yaml --dry-run --format json
replan import github --project examples/small-team --profile github.demo --include-closed --include-relations --dry-run --format json
replan import github --project examples/small-team --repository example-org/planning-demo --token-env GITHUB_TOKEN --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-resource-links --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-resource-links --actor actor.human.operator --apply --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-resource-demands --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-resource-demands --actor actor.human.operator --apply --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-time-entries --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-time-entries --actor actor.human.operator --apply --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-tracking-status --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-tracking-status --actor actor.human.operator --apply --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-dates --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-dates --actor actor.human.operator --apply --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-dependencies --dry-run --format json
replan import accept source_snapshot.openproject.20260706T120000 --accept-dependencies --actor actor.human.operator --apply --format json
replan import accept source_snapshot.openproject.20260706T120000 --actor actor.human.operator --apply --format json
replan import accept source_snapshot.github.20261029T110000 --accept-dependencies --dry-run --format json
```

Implemented command flags:

```text
replan import openproject --project -p --bindings --format --base-url --dry-run --forwarded-proto --from-file --host-header --include-closed --include-relations --include-time-entries --page-size --perimeter-header --perimeter-key-env --profile --project-id --request-retries --request-retry-delay --request-timeout --resource-demands-sidecar --resource-links-sidecar --since --token-env --work-package
replan import github --project -p --bindings --format --base-url --dry-run --from-file --include-closed --include-relations --profile --repository --request-retries --request-retry-delay --request-timeout --resource-demands-sidecar --resource-links-sidecar --since --token-env
replan import accept --project -p --bindings --format --accept-dates --accept-dependencies --accept-resource-demands --accept-resource-links --accept-time-entries --accept-tracking-status --actor --apply --dry-run --reason
```

Rules:

- GitHub file import reads a versioned `replan.github-issues.v1` input
  document. Live import selects a repository through a GitHub source profile
  or `--repository`, reads only the REST API, and writes that same document as
  `raw.json` so it can be replayed with `--from-file`;
- GitHub import filters pull requests, excludes closed issues unless
  `--include-closed` is set, and applies `--since` to `updated_at`;
- GitHub `blocked_by` entries become `finish_to_start` candidates only inside
  the imported repository; cross-repository entries are warned and skipped;
- dependency candidates identify their source objects with
  `predecessor_source_id` and `successor_source_id`, and resource-demand
  candidates use `source_object_id`;
- OpenProject output also writes deprecated `from_work_package_id`,
  `to_work_package_id`, and `work_package_id` aliases with the same source
  objects for existing readers. GitHub output does not write those aliases;
- GitHub sub-issue parents remain source facts in `mapped.json`; RePlan does
  not assign hierarchy semantics to them;
- the first GitHub assignee is matched only through the resource-links
  sidecar; a missing explicit link is a warning;
- GitHub dates and Project-number estimates are used only when the selected
  profile names the fields. Estimates become demands only through a
  resource-demand sidecar that supplies the resource and unit;
- GitHub snapshots are stored under `imports/github/<snapshot-key>/` and all
  task, resource-link, tracking-status, date, dependency and resource-demand
  candidates use the shared `import accept` modes;
- like the implemented OpenProject import, GitHub snapshot writing has no
  `--apply` flag: `--dry-run` previews and omitting it writes the staged
  snapshot. This write does not mutate canonical state;
- OpenProject import supports both `--from-file --dry-run` and API-backed
  `--profile <id> --dry-run`;
- OpenProject import may include time entries as read-only
  `time_entry_candidates` with `--include-time-entries`;
- API mode reads time entries from the OpenProject API;
- file mode reads time-entry fixtures from `_embedded.time_entries.elements`
  when present and returns zero time-entry candidates without warning when no
  time-entry fixture is present;
- OpenProject import may include relations as read-only
  `dependency_candidates` with `--include-relations`;
- API mode reads relations from the OpenProject API;
- file mode reads relation fixtures from `_embedded.relations.elements` when
  present and returns zero dependency candidates without warning when no
  relation fixture is present;
- OpenProject import may read
  `integrations/openproject/resource-demands.yaml` by default when present, or
  a custom path passed with `--resource-demands-sidecar <path>`;
- OpenProject import may read
  `integrations/openproject/resource-links.yaml` by default when present, or a
  custom path passed with `--resource-links-sidecar <path>`;
- imported sidecar resource links are not accepted canonical source refs yet;
  they only help mapper matching until a future explicit accept workflow;
- imported sidecar resource demands are not accepted canonical demands;
  accepting them remains a separate explicit workflow;
- imported time-entry candidates are not accepted actuals; accepting or
  proposing actual consumption remains a separate explicit workflow;
- imported dependency candidates are read-only until accepted through the
  separate explicit `--accept-dependencies` workflow;
- `import accept` reads the source-neutral candidate fields first and falls
  back to the deprecated OpenProject aliases in snapshots written before
  those fields existed;
- without `--dry-run`, OpenProject import writes `manifest.yaml`, `raw.json`
  and `mapped.json` under `imports/openproject/<snapshot-key>/`;
- when `project.openproject_ids` is `private_bindings`, live API import without
  `--dry-run` is refused with `openproject_ids_private` and exit `8`; file-mode
  import may still write a snapshot because its input is already on disk;
- OpenProject import write mode does not mutate canonical planning state;
- `replan import accept --dry-run` previews canonical changes from a stored
  mapped import;
- `replan import accept --accept-resource-links --dry-run` previews tracker
  user source refs that can be safely attached to canonical resources from
  explicit resource-link matches;
- `replan import accept --accept-resource-links --apply --actor <id>` appends
  safe tracker user source refs to `canonical/resources.yaml` and audit;
- `--accept-resource-links` is a separate accept mode and does not also accept
  tasks, resource demands or time-entry actuals;
- `replan import accept --accept-resource-demands --dry-run` previews
  canonical resource-demand records that can be safely created from the
  imported tracker resource-demand sidecar;
- `replan import accept --accept-resource-demands --apply --actor <id>` writes
  only safely matched tracker sidecar resource demands and audit;
- `--accept-resource-demands` is a separate accept mode and does not also
  accept ordinary task/resource import changes or time-entry actuals;
- resource-demand accept planned writes use `canonical/resource_demands.yaml`
  and audit writes use `audit/audit.<YYYY-MM>.jsonl`;
- `replan import accept --accept-time-entries --dry-run` previews evidence and
  actual-consumption records from imported tracker time-entry candidates;
- `replan import accept --accept-time-entries --apply --actor <id>` writes only
  safely matched tracker time-entry actuals with evidence and audit;
- time-entry candidates currently exist only in OpenProject snapshots. Asking
  for `--accept-time-entries` on a GitHub snapshot fails explicitly with
  `import_accept_time_entries_unsupported_source`;
- `--accept-time-entries` is a separate accept mode and does not also accept
  ordinary task or resource import changes;
- `replan import accept --accept-tracking-status --dry-run` previews task
  tracking-status updates from mapped tracker statuses;
- `replan import accept --accept-tracking-status --apply --actor <id>` writes
  only mapped status changes for existing matched tasks and audit;
- `--accept-tracking-status` is a separate accept mode and does not also accept
  ordinary task/source-ref changes, resource links, resource demands or
  time-entry actuals;
- `replan import accept --accept-dates --dry-run` previews each changed
  `planned_start` and `planned_finish` field with its task, old value and new
  value for existing tasks matched by the stored tracker snapshot;
- `replan import accept --accept-dates --apply --actor <id>` writes all safe
  date changes to `canonical/tasks.yaml` in one staged, validated write and
  appends one `import_accept` audit record for the snapshot;
- unmatched candidates, candidates with no dates, unchanged dates and a
  resulting finish before start are reported as warnings and are not written;
- `--accept-dates` is a separate accept mode and does not also accept ordinary
  task/source-ref changes, tracking statuses, dependencies, resource links,
  resource demands or time-entry actuals;
- `replan import accept --accept-dependencies --dry-run` previews canonical
  dependency changes from tracker relation candidates:
  `create_dependency` for new canonical dependencies and
  `update_dependency_source_refs` when an existing canonical dependency already
  has the same predecessor/successor/type triple but lacks the tracker relation
  source reference;
- `replan import accept --accept-dependencies --apply --actor <id>` writes only
  safe tracker relation-backed dependency changes to
  `canonical/dependencies.yaml` and audit;
- dependency accept must never create a duplicate canonical dependency record
  when a matching predecessor/successor/type triple already exists; `relates`
  compares its predecessor and successor as an unordered pair;
- if the matching dependency already has the same tracker relation
  `source_ref`, dependency accept is a no-op with an already-imported warning;
- `--accept-dependencies` is a separate accept mode and does not also accept
  ordinary task/source-ref changes, resource links, resource demands,
  time-entry actuals or tracking statuses;
- `replan import accept --apply --actor <id>` writes only safe accepted import
  changes: create unmatched tasks, add missing tracker task source
  references, append audit and validate after write;
- when `project.openproject_ids` is `private_bindings`, dry-run and apply both
  refuse the whole accept operation with `openproject_ids_private` and exit `8`
  if any change would persist a work-package, user, relation or time-entry id;
  date, tracking-status and resource-demand acceptance remains available;
- resource-demand acceptance requires an already matched or already accepted
  RePlan task for the tracker object, and an existing canonical RePlan
  resource or resource pool;
- time-entry actual acceptance requires an already matched RePlan task, matched
  RePlan resource and matching hour-based resource demand;
- dependency acceptance requires existing canonical predecessor and successor
  tasks, supported dependency type, no duplicate tracker relation source ref
  and no duplicate predecessor/successor/type triple;
- validation warns `dependency_pair_has_several_links` when multiple
  dependency records connect the same two tasks, because a tracker may permit
  only one relation for the pair;
- recommended OpenProject pilot sequence:
  ordinary `import accept --apply`, then
  `import accept --accept-resource-links --apply`, then
  `import accept --accept-resource-demands --apply`, then
  `import accept --accept-time-entries --apply`, and optionally
  `import accept --accept-tracking-status --apply`; relation-backed
  dependencies require a snapshot imported with `--include-relations` after
  ordinary task accept and can then be accepted with
  `import accept --accept-dependencies --apply`;
- time-entry duplicate detection uses `evidence.source_ref` with
  `source=openproject`, `object_type=time_entry` and the OpenProject time-entry
  id;
- `replan import accept --apply` does not automatically apply
  `tracking_status` updates, create resources, create resource demands, create
  deadlines or create dependencies;
- source import writes raw snapshot under `imports/`;
- import mapping may create `mapped.json` canonical candidates;
- import does not automatically mutate accepted canonical state unless policy
  explicitly allows and command requests it;
- accepting import mapping is a separate mutating operation;
- source credentials must come from environment or external profile, not project
  files;
- import output must include snapshot id and warnings.

OpenProject flags:

```text
--project / -p
--format
--profile
--base-url
--token-env
--project-id
--from-file
--work-package
--since
--include-closed
--include-relations
--include-time-entries
--resource-links-sidecar
--resource-demands-sidecar
--page-size
--request-timeout
--request-retries
--request-retry-delay
--host-header
--forwarded-proto
--perimeter-key-env
--perimeter-header
--dry-run
```

`--token-env` should name an environment variable, not pass the token value.
`--profile` should refer to `project.source_profiles[].id` in
`.replan/project.yaml`.

GitHub import flags:

```text
--project / -p
--format
--profile
--from-file
--repository
--token-env
--base-url
--since
--include-closed
--include-relations
--resource-links-sidecar
--resource-demands-sidecar
--request-timeout
--request-retries
--request-retry-delay
--dry-run
```

Live GitHub input requires either `--profile <id>` or `--repository
<owner/repo>`. The selected source profile declares `repository`, `token_env`
(normally `GITHUB_TOKEN`), `status_map`, and optional `start_field`,
`finish_field`, and `estimate_field`. Without an explicit or profile token
environment name, live input looks only at `GITHUB_TOKEN`; an explicitly empty
`--token-env` disables token lookup. A missing token is allowed for a public
repository and produces `github_token_missing`. `--base-url` defaults to
`https://api.github.com` and supports GitHub Enterprise and local test servers.
`--from-file` excludes live-only flags but may still use `--profile` for status
and Project-field mapping.

The canonical completed status is `accepted`, so a GitHub
`closed:completed` profile entry maps to `accepted` (the adapter design's
informal `done`). Live input does not read GitHub Projects: a profile that
names Project fields produces `github_project_fields_live_unsupported` and
those values remain absent. The stored `raw.json` can be replayed through file
input.

Request settings:

- `--request-timeout <seconds>` is the per-request timeout and defaults from
  `REPLAN_OPENPROJECT_TIMEOUT_SECONDS` (default `60`);
- `--request-retries <n>` is the number of GET retries after a transient
  network or HTTP failure and defaults from `REPLAN_OPENPROJECT_RETRIES`
  (default `0`);
- `--request-retry-delay <seconds>` is the pause between attempts and defaults
  from `REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS` (default `2`).

API-backed command shape:

```bash
replan import openproject --project examples/small-team --base-url https://pm.example.test --project-id demo --token-env OPENPROJECT_TOKEN --since 2026-07-01 --include-closed --include-time-entries --dry-run --format json
replan import openproject --project examples/small-team --base-url http://127.0.0.1:8080 --project-id demo --token-env OPENPROJECT_TOKEN --host-header pm.example.test --forwarded-proto https --dry-run --format json
replan import github --project examples/small-team --profile github.demo --include-closed --include-relations --dry-run --format json
replan import github --project examples/small-team --repository example-org/planning-demo --token-env GITHUB_TOKEN --dry-run --format json
```

GitHub live requests send the versioned GitHub JSON accept header, API version
`2022-11-28`, and a RePlan-version user agent. Authorization is sent only when
the selected environment variable is non-empty. A 401 is
`source_unauthorized`, a 404 is `source_not_found`, network and other source
failures are `source_unavailable`, and a 403 or 429 with exhausted-rate or
retry headers is `source_rate_limited`. Rate-limit errors report the reset
time and do not cause a long sleep. Successful results report the last
`X-RateLimit-Remaining` value.

Tunnel and reverse-proxy flags:

- `--host-header <host>` overrides the HTTP Host header for live OpenProject
  API requests. Use it when the base URL is a local tunnel endpoint but the
  upstream OpenProject instance expects its configured public host.
- `--forwarded-proto <http|https>` sends `X-Forwarded-Proto` for live
  OpenProject API requests. Use it when the upstream OpenProject instance is
  configured behind HTTPS but the agent reaches it through a local HTTP tunnel.
- `--perimeter-key-env <env>` names the environment variable that holds the
  key a reverse proxy in front of OpenProject asks of API clients; it defaults
  to `REPLAN_OPENPROJECT_PERIMETER_KEY` and is ignored when that variable is
  unset. The key is sent in the `--perimeter-header <name>` header, by default
  `X-Perimeter-Key`, beside the client's own token, and is never printed.
- These flags are transport settings only. They must not be used as task
  identity and they must not be populated with private values in committed
  examples.

Resource-demand sidecar behavior:

- if `--resource-demands-sidecar <path>` is provided, the mapper must read that
  path or report a validation error;
- if the flag is omitted and
  `integrations/openproject/resource-demands.yaml` exists, the mapper may read
  it and emit `resource_demand_candidates`;
- if the flag is omitted and the default sidecar is missing, the mapper must
  not fail;
- the sidecar path is resolved relative to the RePlan project root unless it is
  absolute;
- sidecar diagnostics must be included in import output and in stored
  `mapped.json`;
- import write mode still writes only `manifest.yaml`, `raw.json` and
  `mapped.json`; it must not mutate `canonical/resource_demands.yaml`.

Resource-links sidecar behavior:

- if `--resource-links-sidecar <path>` is provided, the mapper must read that
  path or report a validation error;
- if the flag is omitted and `integrations/openproject/resource-links.yaml`
  exists, the mapper may read it and use it to match resource candidates;
- if the flag is omitted and the default sidecar is missing, the mapper must
  not fail;
- resource-link sidecar records match OpenProject users to existing canonical
  RePlan resources;
- resource-link sidecar records must not mutate `canonical/resources.yaml`;
- a future explicit accept workflow may append OpenProject user source refs to
  canonical resources.

The API-backed command preserves the `--from-file` mapper contract. API loading,
pagination and credentials are source-acquisition concerns; task, resource and
warning mapping remain the same service boundary.

## `replan export`

Purpose:

Export canonical planning data into external or generated formats.

Subcommands:

```text
replan export taskjuggler
replan export json
replan export csv
```

Examples:

```bash
replan export json --project examples/small-team --output exports/snapshots/current.json --manifest exports/snapshots/current.manifest.json
replan export taskjuggler --project examples/small-team --output exports/taskjuggler/current/project.tjp --manifest exports/taskjuggler/current/project.manifest.json --format json
replan export taskjuggler --project examples/small-team --scenario scenario.openai-quota-300k --unrepresented-assumptions error --format json
replan export csv --project examples/small-team --report workload --week 2026-W28 --scenario scenario.openai-quota-300k --output exports/workload/2026-W28/scenario.openai-quota-300k.csv --manifest exports/workload/2026-W28/scenario.openai-quota-300k.manifest.json
```

Implemented command flags:

```text
replan export json --project -p --bindings --format --manifest --output
replan export taskjuggler --project -p --bindings --format --manifest --output --scenario --unrepresented-assumptions
replan export csv --project -p --bindings --manifest --output --report --scenario --week
```

Rules:

- export is generated artifact, not source of truth;
- TaskJuggler export must not make TaskJuggler required runtime;
- export should include manifest metadata;
- committed export artifacts should follow `docs/spec/export-artifacts.md`;
- export may fail with external source error if external tool invocation fails;
- export should be reproducible from canonical state and command flags.

MVP minimum:

- implement `replan export json` first;
- output a single machine-readable snapshot of the loaded RePlan workspace;
- support stdout when `--output` is omitted;
- support file write when `--output` is provided;
- include export metadata: export id, exported_at, source project path and
  format;
- do not include external secrets;
- do not require TaskJuggler, OpenProject or any network access.

Implemented TaskJuggler proof-of-concept:

- `replan export taskjuggler`;
- generate `.tjp` only;
- map RePlan resources and tasks/resource demands where safe;
- do not export RePlan resource class as TaskJuggler `note`, because
  TaskJuggler resource blocks do not support `note`;
- emit warnings for unsupported mappings;
- support `--format text` for `.tjp` content;
- support `--format json` for metadata, warnings, errors and optional
  `output_path`;
- write `.tjp` when `--output` is provided;
- support `--scenario <id>` with an in-memory overlay for
  `add availability.exceptions`;
- export unavailable scenario exceptions as TaskJuggler `vacation` statements;
- support scenario `set` and `replace` changes for
  `resource_demands.<resource_demand_id>.remaining_amount` and
  `resource_demands.<resource_demand_id>.planned_amount`;
- support scenario `set` and `replace` changes for
  `resource_demands.<resource_demand_id>.resource_ref`;
- support scenario `set` and `replace` changes for
  `tasks.<task_id>.planned_start` and `tasks.<task_id>.planned_finish`;
- support scenario `set` and `replace` changes for
  `capacities.<capacity_id>.amount` in workload overlays, including service
  quota capacities;
- warn with `taskjuggler_capacity_change_not_represented` when
  `replan export taskjuggler --scenario <id>` receives a scenario capacity
  amount change that is applied by RePlan workload but not encoded in `.tjp`;
- map narrow weekly hour capacities to TaskJuggler `limits { weeklymax ... }`
  when capacity type is `periodic_capacity`, resource class is `human` or
  `ai_agent`, unit is `hour`, `hours` or `h`, and period is `week`;
- treat mapped weekly hour capacity scenario changes as represented in `.tjp`;
- support `--unrepresented-assumptions warn|error` for TaskJuggler export;
- default `--unrepresented-assumptions` mode is `warn`;
- `--unrepresented-assumptions error` promotes
  `taskjuggler_capacity_change_not_represented` to an error and prevents
  successful `.tjp` generation;
- use `remaining_amount` for TaskJuggler `effort` when present, with
  `planned_amount` as fallback;
- use overlaid `resource_ref` for TaskJuggler `allocate`;
- do not invoke TaskJuggler;
- do not import TaskJuggler results;
- do not require TaskJuggler runtime.

Current TaskJuggler scenario overlay limitation:

- `add availability.exceptions`, resource demand `remaining_amount`,
  `planned_amount` and `resource_ref`, task `planned_start` and
  `planned_finish`, plus capacity amount changes for workload, are supported by
  the shared overlay;
- only narrow human or AI-agent weekly hour capacity amount changes are mapped
  into TaskJuggler `.tjp` as `limits { weeklymax ... }`;
- quota, budget, rate-limit, subscription, stock, review-hour, agent-hour,
  day-capacity, month-capacity and effective-range capacity changes are not
  mapped into TaskJuggler `.tjp` yet, and must be surfaced as explicit warnings
  or strict errors instead of being silently lost;
- a weekly hour capacity with effective dates is exported as the one
  `weeklymax` in effect on the project status date, for the whole plan, with
  `taskjuggler_capacity_effective_range_simplified`;
- unsupported scenario operations and paths must fail with explicit diagnostics;
- broader overlay support should be added incrementally.

Later:

- broader scenario overlay support for `replan export taskjuggler --scenario
  <id>`;
- broader TaskJuggler `limits` mapping only after
  `docs/spec/taskjuggler-capacity-quota-mapping.md` questions are resolved;
- TaskJuggler run/import workflow that imports a calculated schedule as a
  scenario, not as canonical truth.

Committed export artifact layout:

- defined in `docs/spec/export-artifacts.md`;
- uses `exports/snapshots/`, `exports/workload/<week>/` and
  `exports/taskjuggler/<scope>/`;
- keeps manifests next to generated artifacts as `.manifest.json`;
- does not make `exports/` canonical planning state;
- does not blanket-ignore `exports/`, because some generated artifacts may be
  intentionally committed for review.

```text
export json
taskjuggler export proof of concept for canonical workspace
taskjuggler scenario overlay export for availability exceptions
taskjuggler scenario overlay export for remaining effort
taskjuggler scenario overlay export for planned effort
taskjuggler scenario overlay export for resource reassignment
workload scenario overlay for capacity assumptions
workload scenario overlay for quota assumptions
export csv for workload data
export manifest for file exports
committed exports layout documented
taskjuggler warning for unrepresented capacity scenario changes
taskjuggler strict policy for unrepresented scenario assumptions
taskjuggler weekly hour capacity limits
```

## `replan dev validate-taskjuggler`

Purpose:

Validate generated TaskJuggler `.tjp` with an installed TaskJuggler binary when
available.

Command:

```bash
replan dev validate-taskjuggler --project examples/small-team --scenario scenario.ivan-capacity-6 --unrepresented-assumptions error --format json
```

Flags:

```text
--project
--scenario
--format json|markdown|text
--taskjuggler-bin
--unrepresented-assumptions warn|error
--keep-temp
```

Implemented command flags:

```text
replan dev validate-taskjuggler --project -p --bindings --format --keep-temp --scenario --taskjuggler-bin --unrepresented-assumptions
```

Rules:

- command first generates `.tjp` through RePlan's TaskJuggler export service;
- if TaskJuggler binary is not found, command exits successfully with
  `taskjuggler_binary_not_found` warning and `skipped: true`;
- if TaskJuggler binary is found, command writes a temporary `.tjp` and runs
  the binary against it;
- `--keep-temp` preserves the generated `.tjp` and returns its path;
- validation failure from TaskJuggler returns validation exit code;
- this command is a developer validation helper, not a required MVP runtime
  dependency and not a TaskJuggler scheduling/import workflow.

```text
dev taskjuggler runtime validation helper
local taskjuggler validation wrapper script
github actions taskjuggler validation profile
```

## `replan taskjuggler`

Purpose:

Run TaskJuggler as an optional scheduling experiment and later import results as
reviewable RePlan scenario/proposal candidates.

Subcommands:

```text
replan taskjuggler run
replan taskjuggler import-result
```

First command shape:

```bash
replan taskjuggler run --project examples/small-team --scenario scenario.ivan-capacity-6 --output-dir exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z --unrepresented-assumptions error --format json
```

Implemented command flags:

```text
replan taskjuggler run --project -p --bindings --format --output-dir --scenario --taskjuggler-bin --unrepresented-assumptions
replan taskjuggler import-result --project -p --bindings --format --actor --dry-run --run-dir --scenario-id --write-candidate
```

Rules:

- `taskjuggler run` is implemented as the first run-only artifact capture
  slice;
- `taskjuggler run` is not a developer validation helper;
- `taskjuggler run` fails when requested TaskJuggler binary is missing;
- run artifacts are stored under
  `<project>/exports/taskjuggler/<scope>/run.<timestamp>/` by default, or
  under explicit `--output-dir`;
- run saves generated `.tjp`, stdout, stderr, TaskJuggler version, ID mapping,
  parseable CSV task schedule report and manifest;
- run does not mutate canonical state;
- run does not import TaskJuggler results automatically;
- `taskjuggler import-result` is implemented as dry-run preview plus explicit
  scenario/proposal candidate file write mode;
- imported results should become scenario/proposal/evidence candidates, not
  canonical truth.

Implemented artifact files:

```text
project.tjp
stdout.txt
stderr.txt
taskjuggler.version.txt
taskjuggler.mapping.json
reports/replan-task-schedule.csv
run.manifest.json
```

Default scope names:

- `current` for canonical workspace runs;
- existing scenario id for scenario runs, for example
  `scenario.ivan-capacity-6`.

Failure behavior:

- strict export failure, missing TaskJuggler binary and TaskJuggler process
  failure return validation failure exit code;
- when TaskJuggler process starts and fails, stdout, stderr, generated `.tjp`,
  version output and manifest are still written so the failure is diagnosable;
- when export fails before TaskJuggler can run, no run artifact directory is
  created.

Import-result command shape:

```bash
replan taskjuggler import-result --project examples/small-team --run-dir exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z --dry-run --format json
replan taskjuggler import-result --project examples/small-team --run-dir exports/taskjuggler/scenario.ivan-capacity-6/run.20260713T200000Z --write-candidate --actor actor.human.operator --format json
```

Current import-result rules:

- either `--dry-run` or explicit `--write-candidate --actor <id>` is required;
- command reads `run.manifest.json`, `taskjuggler.mapping.json` and
  `reports/replan-task-schedule.csv`;
- command parses TaskJuggler CSV with semicolon delimiter;
- command maps TaskJuggler task ids back to RePlan task ids through the mapping
  artifact;
- command returns preview-only scenario candidate changes for task planned
  start and planned finish;
- `--write-candidate` writes only reviewable `scenarios/*.yaml` and
  `proposals/open/*.yaml` files;
- command does not mutate canonical task/resource/current-plan state;
- command does not import resource assignments, statuses, effort values,
  dependencies, actuals or source-system write-back.

Design:

- `docs/spec/taskjuggler-run-import-experiment.md`.
- `docs/spec/taskjuggler-parseable-report-contract.md`.

```text
taskjuggler run/import design documented
taskjuggler run-only artifact capture
taskjuggler parseable report contract documented
taskjuggler import-result dry-run preview
```

## `replan audit`

Purpose:

Inspect audit records.

Subcommands:

```text
replan audit list
replan audit show
replan audit verify
```

Examples:

```bash
replan audit list --since 2026-07-01 --format json
replan audit show audit.20260706T151500.adjust-remaining-task-op-1234 --format markdown
replan audit verify --format json
```

Implemented command flags:

```text
replan audit list --project -p --bindings --format --since
replan audit verify --project -p --bindings --format
```

Rules:

- audit inspection is read-only;
- audit verification may detect malformed JSON Lines or missing required fields;
- audit output must not expose secrets;
- audit verify should not require Git.

MVP minimum:

```text
list
verify
```

## Source write-back

A first-class `replan writeback` command is not implemented.

Narrow OpenProject write-back exists today as reviewed helper scripts, not as
a CLI command: `scripts/apply-openproject-bootstrap.sh`,
`scripts/apply-openproject-relations.sh`,
`scripts/push-openproject-date-drift.sh`,
`scripts/push-openproject-status-drift.sh` and
`scripts/openproject-work-package-toolkit.sh`. They are dry-run by default,
apply only with `REPLAN_APPLY=1`, and a dotenv file cannot set that variable.
Broad or automatic bidirectional write-back remains out of scope. See
[`openproject-operator-loop.md`](openproject-operator-loop.md).

A later first-class command may take this shape:

```bash
replan writeback openproject proposal.write-back-op-1234 --dry-run --format json
replan writeback openproject proposal.write-back-op-1234 --apply --actor actor.human.operator
```

Rules when added:

- write-back must be explicit;
- write-back must support dry-run;
- write-back must be audited;
- write-back must not be a side effect of ordinary report or workload commands;
- AI agents should normally create write-back proposals, not apply write-back.

## Interactive prompts

MVP should avoid interactive prompts.

Reason:

RePlan is AI-CLI-first and automation-friendly.

Rules:

- commands should fail with actionable errors instead of asking ambiguous
  questions;
- destructive or applying operations should use `--dry-run`, `--yes`, actor
  policy and proposal flow instead of ad hoc prompts;
- if interactive prompts are added later, `--agent` and non-interactive mode
  must disable them.

## Error codes

Errors must have stable machine-readable codes.

Examples:

```text
project_not_found
project_format_unsupported
invalid_project_layout
missing_required_file
yaml_parse_error
json_parse_error
duplicate_id
unresolved_reference
invalid_status
unknown_resource
unknown_actor
policy_denied
dry_run_required
proposal_required
file_lock_conflict
source_auth_failed
source_unavailable
external_tool_failed
unsupported_operation
internal_error
```

Error message text may change.

Error codes should remain stable.

## AI CLI safety requirements

AI agents must be able to:

- validate project state;
- read workload reports;
- read warnings;
- create scenarios;
- calculate scenario impact;
- create proposals;
- dry-run proposal acceptance;
- read audit history;
- import source data in read-only mode when credentials are available by
  policy.

AI agents should not by default:

- directly apply scenarios;
- directly accept proposals;
- write back to OpenProject or other source systems;
- bypass policy;
- mutate files directly when CLI command exists;
- store tokens in project files.

Future AI-agent command pattern from the wider contract (not runnable today
because global `--agent`, global flag placement, scenario creation and proposal
creation are not implemented):

```bash
replan --agent --actor actor.ai.codex.local validate --format json
replan --agent --actor actor.ai.codex.local report weekly --week 2026-W28 --format json
replan --agent --actor actor.ai.codex.local scenario create --name codex-assists-import --from current --format json
replan --agent --actor actor.ai.codex.local scenario diff scenario.codex-assists-import --against current --format json
replan --agent --actor actor.ai.codex.local proposal create --from-scenario scenario.codex-assists-import --format json
```

## Mapping to future API

Every CLI operation should map to an application service request.

Conceptual mapping:

```text
replan validate -> ValidateProject(request)
replan workload -> CalculateWorkload(request)
replan report weekly -> GenerateReport(request)
replan scenario create -> CreateScenario(request)
replan proposal accept -> AcceptProposal(request)
replan actual add-consumption -> AddActual(request)
replan import openproject -> ImportSource(request)
replan export taskjuggler -> ExportSchedule(request)
replan audit list -> ListAuditRecords(request)
```

Future server API should call the same services.

The CLI contract should therefore avoid assumptions that only make sense in a
terminal.

## MVP implementation order

Recommended CLI implementation order:

1. `replan validate`;
2. `replan workload`;
3. `replan report weekly`;
4. `replan scenario list/show/create/diff`;
5. `replan proposal list/show`;
6. `replan actual list/add-consumption/set-remaining`;
7. `replan audit list/verify`;
8. `replan import openproject`;
9. `replan proposal accept/reject`;
10. `replan scenario apply --dry-run`;
11. `replan export json/csv`;
12. TaskJuggler export proof of concept.

Reason:

This order proves the local file model and reporting loop before implementing
riskier mutation and external integration.

## Decisions captured

- `replan` is the CLI executable name.
- CLI is required in MVP.
- CLI is a thin wrapper over application services.
- Every command must support JSON output.
- Reports should support Markdown output.
- Stable exit codes are part of the contract.
- Mutating commands require actor context.
- Risky mutating commands must support dry-run.
- AI agents should be proposal-first and dry-run-first.
- Source imports are read-only first.
- Source write-back is later and must be explicit, dry-run capable and audited.
- CLI behavior must map cleanly to future daemon/server/API operations.
