# RePlan developer getting started

Date: 2026-07-09

Status: active developer note.

## Purpose

This note records the current local development commands for the first RePlan
implementation slice.

## Backlog Operating Convention

For agreed non-trivial RePlan work, do not rely on chat history or prose docs as
the only backlog.

Use the OpenProject/RePlan dogfood loop:

- OpenProject for human-visible tasks, dates, statuses, relations and Gantt;
- RePlan for resource demands, capacities, scenarios, evidence, proposals,
  actuals and audit;
- docs for decisions, rationale and sanitized public conclusions.

The detailed convention is in
[`backlog-operating-convention.md`](backlog-operating-convention.md).

## Requirements

- Go 1.24 or newer.

Tagged release builds and the public `go install` path are documented in
[`release-process.md`](release-process.md).

## Validate the repository

Run tests:

```bash
go test ./...
```

Run the full local lint gate (CI is currently paused):

```bash
golangci-lint run ./...
```

The lint baseline is clean, so the full run must report zero issues; the gate
is no longer limited to findings newer than a Git revision.

Validate the example project:

```bash
go run ./cmd/replan validate examples/small-team --format json
```

Validate the public self-dogfood project:

```bash
go run ./cmd/replan validate examples/replan-self --format json
go run ./cmd/replan workload --project examples/replan-self --week 2026-W31 --format json
go run ./cmd/replan report weekly --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan scenario diff --project examples/replan-self --format json scenario.maintainer-unavailable-20260723
```

Calculate the first workload report:

```bash
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --scenario scenario.task-op-1234-reassign-petr --format json
```

Generate the first weekly report:

```bash
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format markdown
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format csv
go run ./cmd/replan report dashboard --project examples/small-team --week 2026-W28 --scenario scenario.task-op-1234-reassign-petr --format json
go run ./cmd/replan report estimates --project examples/replan-self --baseline baseline.2026-10-15.tooling-consolidation --format markdown
go run ./cmd/replan estimate calibrate --project examples/estimate-calibration --baseline baseline.fixture.original --resource resource.ai.executor --resource resource.ai.codex --unit agent_hours --tag calibration --exclude task.cal.09=actuals_judged --id calibration.fixture.agents --name "Fixture agent calibration" --approved-by actor.human.operator --dry-run --format markdown
```

Inspect scenarios:

```bash
go run ./cmd/replan scenario list --project examples/small-team --format json
go run ./cmd/replan scenario show scenario.ivan-unavailable-week-28 --project examples/small-team --format json
go run ./cmd/replan scenario diff scenario.ivan-unavailable-week-28 --project examples/small-team --week 2026-W28 --against current --format markdown
```

Inspect proposals and audit:

```bash
go run ./cmd/replan proposal list --project examples/small-team --status open --format json
go run ./cmd/replan proposal show proposal.adjust-remaining-task-op-1234.20260706 --project examples/small-team --format markdown
go run ./cmd/replan proposal suggest-dependencies --project examples/small-team --format json
go run ./cmd/replan dependency list --project examples/small-team --format markdown
go run ./cmd/replan audit list --project examples/small-team --since 2026-07-01 --format json
go run ./cmd/replan audit verify --project examples/small-team --format json
```

Inspect actual execution facts:

```bash
go run ./cmd/replan actual list --project examples/small-team --format json
go run ./cmd/replan actual list --project examples/small-team --resource resource.human.petr --format markdown
go run ./cmd/replan actual add-consumption --project examples/small-team --task task.op.1234 --demand resource_demand.task-op-1234.implementer --resource resource.human.ivan --amount 3 --unit hours --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T16:00:00+07:00 --status-date 2026-07-06 --dry-run --format json
go run ./cmd/replan actual set-remaining --project examples/small-team --demand resource_demand.task-op-1234.implementer --amount 5 --unit hours --reason "Operator review after pull request" --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T16:30:00+07:00 --status-date 2026-07-06 --dry-run --format json
go run ./cmd/replan actual close-demand --project examples/small-team --demand resource_demand.task-op-1234.implementer --status consumed --reason "Operator reviewed demand lifecycle" --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T16:45:00+07:00 --status-date 2026-07-06 --dry-run --format json
go run ./cmd/replan actual add-status --project examples/small-team --task task.op.1234 --status review_ready --reason "Pull request is open and ready for review" --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T17:00:00+07:00 --status-date 2026-07-06 --dry-run --format json
```

Write actual consumption to a disposable copy, not to the committed fixture:

```bash
tmp_project="$(mktemp -d)/small-team"
cp -a examples/small-team "$tmp_project"
go run ./cmd/replan actual add-consumption --project "$tmp_project" --task task.op.1234 --demand resource_demand.task-op-1234.implementer --resource resource.human.ivan --amount 3 --unit hours --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T16:00:00+07:00 --status-date 2026-07-06 --apply --format json
go run ./cmd/replan actual set-remaining --project "$tmp_project" --demand resource_demand.task-op-1234.implementer --amount 5 --unit hours --reason "Operator review after pull request" --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T16:30:00+07:00 --status-date 2026-07-06 --apply --format json
go run ./cmd/replan actual close-demand --project "$tmp_project" --demand resource_demand.task-op-1234.implementer --status consumed --reason "Operator reviewed demand lifecycle" --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T16:45:00+07:00 --status-date 2026-07-06 --apply --format json
go run ./cmd/replan actual add-status --project "$tmp_project" --task task.op.1234 --status review_ready --reason "Pull request is open and ready for review" --evidence evidence.github.pr.45 --actor actor.human.operator --occurred-at 2026-07-06T17:00:00+07:00 --status-date 2026-07-06 --apply --format json
```

## Current implemented CLI commands

The authoritative inventory is the binary's own usage text:

```bash
go run ./cmd/replan --help
go run ./cmd/replan <command> <subcommand> --help
```

Implemented command families as of 2026-10-28:

- `version`, which needs no project and reports release, VCS, toolchain,
  platform and current project-format metadata; `--version` prints its compact
  text line;
- `validate`;
- `next`, the short view of what waits on a person and what an agent can
  prepare;
- `project migrate`, the audited, explicit upgrade path for project file
  formats, and `project set-status-date`, the audited way to move the project
  status date;
- `workload`, with `--week`, `--resource`, `--resource-class`, `--unit`,
  `--scenario` and `--demands`;
- `report weekly`, `report state`, `report dashboard`, each with an optional
  in-memory `--scenario` overlay, and `report estimates` against a named sealed
  estimate baseline;
- `tracking suggest`, optionally writing proposal candidates;
- `scenario list`, `show`, `diff`, `apply`;
- `proposal list`, `show`, `suggest-dependencies`, `accept`, `reject`;
- `dependency list`;
- `baseline create`, `list`, `show`;
- `estimate calibrate`, `list`, `show`, `apply` for sealed estimate
  calibrations and calibration-linked scenarios;
- `audit list`, `verify`;
- `actual list`, `add-consumption`, `set-remaining`, `close-demand`,
  `reopen-demand`, `acknowledge-overrun`, `add-status`;
- `task add`, `task set-dates`, `task record-actual-window`, `task set-title`;
- `import openproject` from a file, a source profile or the API, with
  relations, time entries and resource sidecars;
- `import github` from a versioned exported file, with label status mapping,
  same-repository blocked-by relations, optional Project fields and resource
  sidecars;
- `import accept`, dry-run or `--apply` with an actor, per accept mode;
- `export json`, `export taskjuggler`, `export csv --report workload`, each
  with an optional `--manifest`;
- `taskjuggler run`, `taskjuggler import-result`;
- `dev validate-taskjuggler`.

Every implemented command that loads a workspace accepts
`--bindings <path>`. `REPLAN_OPENPROJECT_BINDINGS_LAYER` supplies the same
path when the flag is absent; the flag wins. The similarly named helper
setting `REPLAN_OPENPROJECT_BINDINGS_PATH` is deliberately not a CLI default.
The layer removes every committed `source: openproject` reference from each
task, then adds that task's private `source_ref` when one is bound. Other task
source refs remain. Private dependency and resource bindings are not layered.
An unknown task binding is the `binding_unknown_task` warning.

Write behavior: every writing command validates the would-be project before
any file changes. `applied: false` means nothing was written. Commands that
write canonical state require `--dry-run` or `--apply`; `REPLAN_APPLY=1`
supplies the apply choice for direct CLI use, while explicit `--dry-run` wins.
Repository scripts pass a flag explicitly. OpenProject helper scripts remain
dry-run unless `REPLAN_APPLY=1` is set.

The bindings layer is read-only. Canonical-writing commands, `import
openproject` without `--dry-run` and `tracking suggest --write-candidates`
refuse it even for `--dry-run` (`bindings_layer_read_only`, exit 8), and the
layered store refuses any other project write. Report and run artifacts
whose output directory is inside the project are also refused
(`bindings_layer_output_in_project`, exit 8); select an outside directory for
private-id dashboard output.

Narrow OpenProject write-back exists as reviewed helper scripts, dry-run by
default and applied only with `REPLAN_APPLY=1`: `apply-openproject-bootstrap.sh`,
`apply-openproject-relations.sh`, `push-openproject-date-drift.sh`,
`push-openproject-status-drift.sh` and `openproject-work-package-toolkit.sh`.
Broad or automatic write-back is out of scope.

Create and inspect an immutable estimate snapshot:

```bash
go run ./cmd/replan baseline create --project examples/small-team \
  --id baseline.2026-07-08.reviewed --name "Reviewed plan" \
  --approved-by actor.human.operator --status-date 2026-07-08 \
  --dry-run --format text
go run ./cmd/replan baseline list --project examples/small-team --format text
go run ./cmd/replan baseline show baseline.2026-07-06.initial \
  --project examples/small-team --format text
```

The live OpenProject plugin needs an overlay artifact with real work-package
ids. `scripts/build-openproject-live-overlay.sh` builds it locally from the
ignored private binding file. `scripts/publish-openproject-overlay.sh`
publishes it, as a dry run by default and only sending with `REPLAN_APPLY=1`.
The operator must still set the private `REPLAN_OVERLAY_PUBLISH_TARGET` to the
overlay directory as a local path or `host:path` rsync target.

Specified in [`../spec/cli.md`](../spec/cli.md) but not implemented by the Go
CLI: global placement of its per-command flags, `--agent`, `--yes`,
`--verbose`, `--quiet`, `--debug`, an actor fallback from `REPLAN_ACTOR`,
parent-directory project discovery, extra workload filters (GAP-011), extra
`report` subcommands, `scenario create|patch|calculate|propose-apply`,
`proposal create|supersede`, `audit show`, `import github|gitlab|csv` and a
first-class `writeback` command. The smoke and OpenProject sidecar-pipeline
scripts do read `REPLAN_ACTOR` and pass it to the CLI as `--actor`.

## Current command workflows

The live task list and order are in OpenProject and RePlan, not this guide:

```bash
go run ./cmd/replan next --project examples/replan-self
go run ./cmd/replan report state --project examples/replan-self --format markdown
```

The private-pilot implementation journal formerly kept here was moved to
private storage on 2026-10-27. Reusable conclusions are in
[`openproject-pilot-lessons.md`](openproject-pilot-lessons.md).

### Export planning data

Generate TaskJuggler or workload exports without changing canonical state:

```bash
go run ./cmd/replan export taskjuggler --project examples/small-team --format text
go run ./cmd/replan export taskjuggler --project examples/small-team --scenario scenario.ivan-unavailable-week-28 --output tmp/replan-scenario.tjp --format json
go run ./cmd/replan export taskjuggler --project examples/small-team --scenario scenario.openai-quota-300k --unrepresented-assumptions error --format json
go run ./cmd/replan export csv --project examples/small-team --report workload --week 2026-W28 --scenario scenario.openai-quota-300k --output tmp/replan-workload.csv --manifest tmp/replan-workload.manifest.json
```

Reviewable generated artifacts may use the committed layout under `exports/`
when the artifact itself is part of review, comparison, debugging, release
notes or an external-tool handoff:

```bash
go run ./cmd/replan export json --project examples/small-team --output exports/snapshots/current.json --manifest exports/snapshots/current.manifest.json
go run ./cmd/replan export csv --project examples/small-team --report workload --week 2026-W28 --scenario scenario.openai-quota-300k --output exports/workload/2026-W28/scenario.openai-quota-300k.csv --manifest exports/workload/2026-W28/scenario.openai-quota-300k.manifest.json
go run ./cmd/replan export taskjuggler --project examples/small-team --scenario scenario.task-op-1234-reassign-petr --output exports/taskjuggler/scenario.task-op-1234-reassign-petr/project.tjp --manifest exports/taskjuggler/scenario.task-op-1234-reassign-petr/project.manifest.json --format json
```

Do not commit every local export by default. The neighboring manifest records
the command metadata and checksum that make a reviewed artifact traceable.

### Validate and run TaskJuggler

Validate generated `.tjp` when TaskJuggler is available:

```bash
go run ./cmd/replan dev validate-taskjuggler --project examples/small-team --scenario scenario.ivan-capacity-6 --unrepresented-assumptions error --format json
./scripts/validate-taskjuggler.sh
```

If `tj3` is absent, developer validation succeeds with a
`taskjuggler_binary_not_found` warning and `skipped: true`. The planning run is
different: it requires TaskJuggler and captures reviewable run artifacts.

```bash
go run ./cmd/replan taskjuggler run --project examples/small-team --scenario scenario.ivan-capacity-6 --unrepresented-assumptions error --format json
go run ./cmd/replan taskjuggler run --project examples/small-team --scenario scenario.ivan-capacity-6 --output-dir exports/taskjuggler/scenario.ivan-capacity-6/run.manual --unrepresented-assumptions error --format json
go run ./cmd/replan taskjuggler import-result --project examples/small-team --run-dir exports/taskjuggler/scenario.ivan-capacity-6/run.manual --dry-run --format json
go run ./cmd/replan taskjuggler import-result --project examples/small-team --run-dir exports/taskjuggler/scenario.ivan-capacity-6/run.manual --write-candidate --actor actor.human.operator --format json
```

`taskjuggler import-result` previews task-date scenario changes. Explicit
`--write-candidate --actor <id>` writes reviewable scenario and proposal files,
not canonical task state or source-system updates.

### Import OpenProject

Start with the public file fixture. Without `--dry-run`, import writes a source
snapshot under `imports/` but still does not mutate canonical planning state.
With `project.openproject_ids: private_bindings`, a live API import may only
use `--dry-run`; a file import may still write its already-local input as a
snapshot.

```bash
go run ./cmd/replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
go run ./cmd/replan import openproject --project examples/small-team --profile openproject.demo --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
go run ./cmd/replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --format json
```

API-backed import uses the same mapper. Keep the token in the named environment
variable and use a source profile or explicit generic connection settings:

```bash
OPENPROJECT_TOKEN=... go run ./cmd/replan import openproject --project examples/small-team --profile openproject.demo --since 2026-07-01 --dry-run --format json
OPENPROJECT_TOKEN=... go run ./cmd/replan import openproject --project examples/small-team --base-url https://pm.example.test --project-id demo --token-env OPENPROJECT_TOKEN --include-closed --include-relations --include-time-entries --dry-run --format json
```

The validation wrapper always checks file-mode import, snapshot persistence on
a disposable project copy and import-accept dry run. It adds the live API check
only when its configured token is present:

```bash
./scripts/validate-openproject.sh
OPENPROJECT_TOKEN=... ./scripts/validate-openproject.sh
```

### Accept an import

Review a stored snapshot before applying it:

```bash
go run ./cmd/replan import accept <snapshot-id> --project examples/small-team --dry-run --format json
go run ./cmd/replan import accept <snapshot-id> --project examples/small-team --actor actor.human.operator --apply --format json
```

Projects that keep live OpenProject ids in a private bindings file should set
`project.openproject_ids: private_bindings`. In that mode both dry-run and
apply refuse task/source-ref, resource-link, dependency and time-entry changes
that would persist OpenProject ids. Date, tracking-status and resource-demand
acceptance still work, and a mixed change set is refused as a whole.

Resource links, resource demands, time entries, tracking status and dependencies
are separate explicit accept modes. For example:

```bash
go run ./cmd/replan import accept <snapshot-id> --project examples/small-team --accept-resource-links --dry-run --format json
go run ./cmd/replan import accept <snapshot-id> --project examples/small-team --accept-resource-demands --dry-run --format json
go run ./cmd/replan import accept <snapshot-id> --project examples/small-team --accept-time-entries --dry-run --format json
go run ./cmd/replan import accept <snapshot-id> --project examples/small-team --accept-tracking-status --dry-run --format json
go run ./cmd/replan import accept <snapshot-id> --project examples/small-team --accept-dependencies --dry-run --format json
```

Apply a reviewed mode only with an actor and `--apply`. Dependency acceptance
creates a missing canonical dependency or adds a missing OpenProject source
reference to a matching dependency; it does not duplicate the same
predecessor/successor/type triple.
## Example walkthroughs

Longer read-only loops over both example projects, and the mutating commands
to try on a temporary copy. OpenProject commands are in
[`openproject-helpers.md`](openproject-helpers.md).

Self-dogfood read-only loop:

```bash
go run ./cmd/replan validate examples/replan-self --format json
go run ./cmd/replan next --project examples/replan-self
go run ./cmd/replan workload --project examples/replan-self --week 2026-W31 --format json
go run ./cmd/replan report weekly --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan report state --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan report state --project examples/replan-self --week 2026-W31 --format json
go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --format json
REPLAN_PROJECT=examples/replan-self ./scripts/explain-resource-overload.sh
go run ./cmd/replan report weekly --project examples/replan-self --week 2026-W31 --format html
go run ./cmd/replan actual close-demand --project examples/replan-self --demand resource_demand.openproject-fixtures.review --status cancelled --actor actor.human.maintainer --reason "Operator reviewed demand lifecycle." --dry-run --format json
go run ./cmd/replan scenario diff --project examples/replan-self --format json scenario.maintainer-unavailable-20260723
go run ./cmd/replan proposal list --project examples/replan-self --status open --format json
go run ./cmd/replan dependency list --project examples/replan-self --format markdown
```

Tracking proposal candidate write mode should be tried against a temporary copy:

```bash
tmp_project="$(mktemp -d)/replan-self"
cp -a examples/replan-self "$tmp_project"
go run ./cmd/replan tracking suggest --project "$tmp_project" --week 2026-W31 --write-candidates --actor actor.ai.codex.local --format json
go run ./cmd/replan proposal accept --project "$tmp_project" proposal.tracking.resource_demand-tracking-suggest-maintainer.set-remaining-zero --dry-run --format json
go run ./cmd/replan proposal accept --project "$tmp_project" proposal.tracking.resource_demand-tracking-suggest-maintainer.set-remaining-zero --actor actor.human.maintainer --reason "Operator accepted stale remaining-work tracking proposal." --apply --format json
```

The self-dogfood fixture intentionally shows useful planning pressure while
remaining internally schedulable: historical accepted-task demands are closed,
reviewer capacity stays strict at 2 `review_hours` per week, and
release-oriented review work is deferred instead of inflating capacity.

Small-team read-only loop:

```bash
go run ./cmd/replan validate examples/small-team --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --scenario scenario.ivan-capacity-6 --format json
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format markdown
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format html
go run ./cmd/replan scenario diff --project examples/small-team --format json scenario.ivan-unavailable-week-28
go run ./cmd/replan proposal list --project examples/small-team --status open --format json
go run ./cmd/replan proposal show --project examples/small-team --format json proposal.adjust-remaining-task-op-1234.20260706
go run ./cmd/replan dependency list --project examples/small-team --format markdown
```

The baseline fixture currently has no overload. The scenario workload command
above applies `scenario.ivan-capacity-6` in memory and demonstrates an overload
without changing the project directory.

Safe dry-run mutation previews:

```bash
go run ./cmd/replan scenario apply --project examples/small-team --dry-run --format json scenario.task-op-1234-remaining-5
go run ./cmd/replan actual add-consumption --project examples/small-team --task task.op.1234 --demand resource_demand.task-op-1234.implementer --resource resource.human.ivan --amount 1 --unit hours --actor actor.human.operator --occurred-at 2026-07-08T10:00:00+07:00 --status-date 2026-07-08 --dry-run --format json
go run ./cmd/replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
```

Dependency lifecycle loop:

```bash
go run ./cmd/replan proposal suggest-dependencies --project examples/small-team --format json
go run ./cmd/replan proposal suggest-dependencies --project examples/small-team --write-candidates --actor actor.ai.codex.local --format json
go run ./cmd/replan dependency list --project examples/small-team --format json
```

Dependency proposal acceptance can block a successor, resolve a dependency or
return a blocked successor to `ready`. Run those mutating accepts on a temporary
project copy unless you intentionally want to modify the project.

Run mutating commands only on a temporary copy until the operator intentionally
wants to change a project directory.

Generated report artifact example:

```bash
tmp_project=$(mktemp -d)
cp -a examples/small-team "$tmp_project/small-team"
go run ./cmd/replan report weekly --project "$tmp_project/small-team" --week 2026-W28 --format html --write
```

## Current example project

The fixture lives in:

```text
examples/small-team/
```

It contains:

- operator actor;
- two human developer resources;
- one AI coding agent actor/resource;
- one service quota resource;
- one project budget resource;
- one staging environment resource;
- one software task;
- one offline repair task;
- one baseline;
- one actual record;
- one evidence file;
- one open proposal;
- one scenario.

## Agent kit release

An outside person or agent does not need this repository. `scripts/build-agent-kit.sh`
builds `dist/replan-agent-kit-<date>-<commit>-linux-amd64.tar.gz` with a
static `replan` binary, the OpenProject helpers and their Python modules, the
`small-team` sample plan, the settings template and the guide
([`../agent-kit/README.md`](../agent-kit/README.md)), plus a SHA-256 file.

Two checks guard it:

- `scripts/check-agent-kit.sh`, run by the smoke: builds the archive, unpacks
  it outside the repository and uses it in an empty environment whose `go`
  fails, so a helper that tried to build from source is caught; it also
  refuses private details, sources and settings files in the archive, and any
  helper or Python module a kit script names but the kit lacks.
- `scripts/check-openproject-agent-kit-live.sh`, run by the plugin's throwaway
  runtime: the unpacked kit, configured only by its settings file, works on a
  real OpenProject with the token of a user who holds only the standard
  "Member" role: the sample plan's work packages and relation, single
  work-package commands, the drift check, the operator report, and rebuilding
  a lost bindings file.

What made the helpers usable without an administrator: the settings file now
also carries the plan's path, the target project and the assignee;
`replan_load_openproject_dotenv` points the helpers at `bin/replan` when the
archive has one; new work packages are assigned to the token's own user
unless another is named, where user 4 was assumed before; and `user list`
reads the project's possible assignees, which a member may, instead of the
instance's users, which only an administrator may.

A kit used from outside must also pass whatever stands in front of
OpenProject. On 2026-10-12 the first outside run met a reverse proxy that
answers the API with its own 401 unless the request carries a perimeter key;
an earlier check had read that 401 as OpenProject's. The helpers and
`replan import openproject` now send `REPLAN_OPENPROJECT_PERIMETER_KEY` in
`X-Perimeter-Key` (or `REPLAN_OPENPROJECT_PERIMETER_HEADER`).
`check-agent-kit.sh` runs both against `perimeter_probe_server`, a stand-in
that records what it is sent, and fails when either omits the key or prints
it. It also searches the binary for private details; the repository's Go
module path stays in the binary's symbols, as in any Go binary.

The kit's second surface is the project wiki. `scripts/openproject-wiki.sh`
lists, shows, creates, updates and deletes pages through the wiki API plugin
and lists, downloads, uploads and deletes their attachments through
OpenProject's own routes, dry run first like every helper. An update names
the lock version the page was read at and is refused, not merged, when the
page changed meanwhile; page text goes to the file the caller names and never
into an error. The live check covers it with a Member's token and a Reader's:
it has 51 checks.
