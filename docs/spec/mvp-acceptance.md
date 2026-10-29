# RePlan MVP acceptance smoke

This document defines the first end-to-end MVP acceptance smoke for RePlan.

The acceptance smoke is not a full product certification.

It is a fast, repeatable proof that the local CLI-first MVP loop still works
after implementation changes.

## Goal

The smoke must prove that a human operator or AI CLI agent can use the current
project fixture for the core MVP loop:

```text
validate -> workload -> reports -> scenario review -> proposal review -> actual write -> scenario apply -> audit verify -> OpenProject import
```

The smoke must be safe to run:

- locally;
- in continuous integration;
- without network access;
- without a daemon;
- without a web server;
- without mutating `examples/small-team`;
- without mutating `examples/replan-self`.

## Script

Implemented script:

```bash
./scripts/smoke-mvp.sh
```

The script builds a temporary `replan` binary unless `REPLAN_BIN` is provided.

Default inputs:

```text
REPLAN_PROJECT=examples/small-team
REPLAN_OPENPROJECT_SAMPLE=examples/imports/openproject/work-packages.sample.json
REPLAN_WEEK=2026-W28
REPLAN_ACTOR=actor.human.operator
```

## Read-only checks

The smoke runs these checks against `examples/small-team` directly:

```bash
replan validate examples/small-team --format json
replan workload --project examples/small-team --week 2026-W28 --format json
replan report weekly --project examples/small-team --week 2026-W28 --format json
replan report weekly --project examples/small-team --week 2026-W28 --format markdown
replan report weekly --project examples/small-team --week 2026-W28 --format csv
replan report weekly --project examples/small-team --week 2026-W28 --format html
replan scenario show scenario.ivan-unavailable-week-28 --project examples/small-team --format json
replan scenario show --project examples/small-team --format json scenario.ivan-unavailable-week-28
replan scenario diff --project examples/small-team --against current --format json scenario.ivan-unavailable-week-28
replan scenario apply --project examples/small-team --dry-run --format json scenario.task-op-1234-remaining-5
replan proposal list --project examples/small-team --status open --format json
replan proposal show proposal.adjust-remaining-task-op-1234.20260706 --project examples/small-team --format json
replan proposal show --project examples/small-team --format json proposal.adjust-remaining-task-op-1234.20260706
replan proposal accept --project examples/small-team --dry-run --format json proposal.adjust-remaining-task-op-1234.20260706
replan actual list --project examples/small-team --format json
replan actual add-consumption --project examples/small-team --task task.op.1234 --demand resource_demand.task-op-1234.implementer --resource resource.human.ivan --amount 1 --unit hours --actor actor.human.operator --occurred-at 2026-07-08T10:00:00+07:00 --status-date 2026-07-08 --dry-run --format json
replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
```

These checks assert that command JSON envelopes report `"ok": true` where JSON
is expected and that rendered report formats contain key stable markers.

The smoke also runs a smaller read-only public dogfood slice against
`examples/replan-self`:

```bash
replan validate examples/replan-self --format json
replan workload --project examples/replan-self --week 2026-W31 --format json
replan report weekly --project examples/replan-self --week 2026-W31 --format markdown
replan report state --project examples/replan-self --week 2026-W31 --format json
replan report state --project examples/replan-self --week 2026-W31 --format markdown
replan tracking suggest --project examples/replan-self --week 2026-W31 --format json
replan tracking suggest --project examples/replan-self --week 2026-W31 --format markdown
replan tracking suggest --project <temp-self-project> --week 2026-W31 --write-candidates --actor actor.ai.codex.local --format json
replan proposal accept --project <temp-self-project> --dry-run --format json proposal.tracking.<demand>.set-remaining-zero
replan proposal accept --project <temp-self-project> --actor actor.human.maintainer --reason "Operator accepted stale remaining-work tracking proposal." --apply --format json proposal.tracking.<demand>.set-remaining-zero
replan scenario diff --project examples/replan-self --format json scenario.maintainer-unavailable-20260723
```

This fixture keeps the open-source story clean: the dogfood project is RePlan
itself, with public resources such as maintainer time, AI agent hours, review
role capacity, CI minutes, LLM token quota, a local development environment and a
release window. It intentionally exposes useful planning pressure: maintainer
hours and reviewer role capacity are overloaded, while agent, CI and token resources
remain visible in the same model.

The `report state` checks prove that the public dogfood project has a compact
operator-facing project-state overview in both agent-readable JSON and
human-readable Markdown. The report summarizes synchronization health, planning
health, Plan versus Actual tracking, resource pressure and next operator
actions without requiring a daemon, server, browser or live OpenProject access.
It also includes tracking proposal loop observability: stale remaining rows,
open/accepted tracking proposals, covered demands, actionable demands and the
next safe command for unresolved stale remaining work. The static dashboard
renders the same loop on `reports/latest/index.html` for human inspection.

The `tracking suggest` checks prove that the public dogfood project can produce
read-only execution-tracking recommendations from Plan versus Actual signals.
The first suggestions identify stale remaining work, missing actuals, missing
evidence and forecast-over-plan rows. They are deliberately advisory: they may
include dry-run commands for the next operator action, but they do not mutate
canonical state by themselves.

The `tracking suggest --write-candidates` smoke uses a temporary copy of the
self-dogfood project and verifies that stale-remaining suggestions can be
materialized as reviewable `proposals/open/*.yaml` files. This write mode still
does not update resource demands, actuals, audit records, evidence records or
OpenProject.

The tracking proposal accept smoke then accepts one generated
`update_remaining_work` proposal on the temporary copy. It verifies that
`accept --dry-run` plans `canonical/resource_demands.yaml`, that mutating
accept moves the proposal to `proposals/accepted`, updates the target remaining
work, writes audit and leaves the project valid.

The smoke also copies `examples/replan-self` to a temporary directory and runs:

```bash
replan report dashboard --project <temp-self-project> --week 2026-W31 --output reports/latest --format html
```

Expected artifacts:

```text
reports/latest/index.html
reports/latest/gantt.html
reports/latest/raw/overview.json
reports/latest/raw/gantt.json
reports/latest/raw/openproject_overlay.json
reports/latest/raw/manifest.json
```

This proves that the first human observability dashboard can be generated
locally, without network access and without mutating the committed
self-dogfood fixture.

The smoke also checks a public sanitized OpenProject sample for the
self-dogfood project:

```bash
replan import openproject --project examples/replan-self --from-file examples/imports/openproject/replan-self.work-packages.sample.json --dry-run --format json
```

This proves that the RePlan self-dogfood tasks can be compared against a
sanitized OpenProject work-package plan without live OpenProject access.
The generated dashboard source-health section must show OpenProject task links
for the self-dogfood tasks.

The generated `gantt.html` page and `raw/gantt.json` artifact must show the
same self-dogfood tasks as a read-only timeline with OpenProject ids and RePlan
resource/warning badges. This gives the operator a visual comparison surface
next to OpenProject Gantt without requiring live OpenProject access during
report generation.

The generated `raw/openproject_overlay.json` artifact must provide the same
OpenProject-linked tasks as a plugin-oriented contract: OpenProject work
package id, RePlan task id, task dates, tracking status, dependency and warning
signals, resource demand rows, participation windows, planned and actual
resource consumption, remaining work, forecast, variance and resource load
intensity. This file is the stable first read model for a future read-only
OpenProject plugin and for AI CLI agents that need the RePlan layer without
parsing HTML.

The smoke also verifies CLI positional id ergonomics for representative
commands:

- object id before flags;
- object id after flags.

## Artifact write checks

Generated report artifacts are tested on a temporary copy of the project.

The smoke checks:

```bash
replan report weekly --project <temp-project> --week 2026-W28 --format html --write
```

Expected artifact:

```text
reports/weekly/2026-W28.html
```

This proves that static HTML generation works as a local generated artifact
without introducing canonical state, a daemon or a web application.

## Mutation checks

Mutating commands must never run against the committed fixture.

The smoke copies `examples/small-team` to a temporary directory and then runs:

```bash
replan actual add-consumption --project <temp-project> --task task.op.1234 --demand resource_demand.task-op-1234.implementer --resource resource.human.ivan --amount 1 --unit hours --actor actor.human.operator --occurred-at 2026-07-08T10:00:00+07:00 --status-date 2026-07-08 --apply --format json
replan scenario apply scenario.task-op-1234-remaining-5 --project <temp-project> --actor actor.human.operator --reason "MVP smoke scenario apply." --apply --format json
replan validate <temp-project> --format json
replan audit verify --project <temp-project> --format json
```

The smoke verifies that:

- actual consumption can be recorded;
- scenario apply can mutate supported current planning state;
- audit records are appended;
- validation still passes after mutations;
- audit verification still passes after mutations.

## OpenProject import checks

OpenProject is tested without network access.

The smoke uses the committed sample:

```text
examples/imports/openproject/work-packages.sample.json
```

It runs:

```bash
replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
```

It also writes an import snapshot on a temporary copy:

```bash
replan import openproject --project <temp-project> --from-file examples/imports/openproject/work-packages.sample.json --format json
```

Expected files:

```text
imports/openproject/<snapshot-key>/manifest.yaml
imports/openproject/<snapshot-key>/raw.json
imports/openproject/<snapshot-key>/mapped.json
```

Then it runs import acceptance in dry-run mode:

```bash
replan import accept source_snapshot.openproject.<snapshot-key> --project <temp-project> --dry-run --format json
```

This proves that OpenProject read-only import can create a reviewable local
snapshot and mapped candidates without source write-back.

## Continuous integration

The GitHub Actions `Go` job runs:

```bash
go test ./...
./scripts/smoke-mvp.sh
```

TaskJuggler remains in a separate CI job because it requires Ruby and a pinned
TaskJuggler gem.

## TaskJuggler boundary

TaskJuggler is intentionally not part of the fast MVP smoke script.

Reasons:

- the fast smoke should run without Ruby;
- the fast smoke should run without installing external tools;
- TaskJuggler is optional, not required MVP runtime;
- TaskJuggler already has dedicated scripts and CI coverage.

Dedicated scripts:

```bash
./scripts/validate-taskjuggler.sh
./scripts/run-taskjuggler.sh
```

## Non-goals

The MVP acceptance smoke does not prove:

- full enterprise reliability;
- hosted service behavior;
- daemon behavior;
- web UI behavior;
- OpenProject API availability;
- OpenProject write-back;
- full TaskJuggler parity;
- full role-based access control;
- custom authentication;
- performance under large datasets.

## Done criteria

This slice is done when:

- `scripts/smoke-mvp.sh` exists and is executable;
- the script passes locally;
- the script is run by GitHub Actions;
- the script does not mutate committed fixtures;
- the script covers read-only commands, generated reports, local mutations,
  audit verification and OpenProject file import;
- `docs/spec/implementation-sequence.md` points to this acceptance smoke;
- TaskJuggler remains covered by its separate optional scripts and CI job.
