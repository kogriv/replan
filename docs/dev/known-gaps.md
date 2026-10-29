# Known gaps register

Date: 2026-07-24, last updated 2026-09-24

Status: active gap register.

## Purpose

This document records known, verified gaps between RePlan code, tests and
documentation that are not tracked by any other active plan or audit.

It is intentionally a hygiene register, not a feature backlog and not an MVP
readiness verdict. MVP blocker assessment stays in
[`../spec/mvp-readiness.md`](../spec/mvp-readiness.md) and
[`../spec/mvp-readiness-audit-plan.md`](../spec/mvp-readiness-audit-plan.md).

Rules:

- every gap entry must point to concrete files and, where useful, line
  anchors;
- every gap entry must state observed impact, not speculation;
- gap status vocabulary: `open`, `accepted` (kept intentionally, with reason),
  `closed` (with the closing commit or document);
- closing a gap should update this file in the same change.

## Gaps

### GAP-001: tracking statuses `done` and `on_hold` handled by reports but rejected by validation

Status: closed (2026-07-24, slice 2: removed the unreachable `done`/`on_hold`
branches from `internal/app/report.go` so report code matches
`ValidTrackingStatus`).

Location:

- `internal/app/report.go` (`case "accepted", "done"` and
  `case "in_progress", "ready", "review_ready", "blocked", "on_hold"` in the
  dashboard task grouping; `status == "accepted" || status == "done"` in the
  tracking-row completion check);
- `internal/domain/status.go` (`ValidTrackingStatus` allows only
  `not_started, ready, in_progress, blocked, review_ready, accepted,
  cancelled`).

Observed impact: report code expects `done` and `on_hold` tasks, but any
project containing such a `tracking_status` fails `replan validate`, so the
report branches are unreachable through a valid project. The canonical model
specification deliberately excludes `done` as ambiguous.

Suggested resolution: remove `done` and `on_hold` from `report.go` so code
matches the approved status model, or extend `ValidTrackingStatus` and the
canonical model specification if the statuses are actually wanted. The first
option matches the approved canonical model.

### GAP-002: `auditPath` duplicated across layers

Status: closed (2026-07-24, slice 2: the app layer now exports
`app.AuditShardPath` as the single source of truth and the storage adapter
delegates to it; the duplicate function in `filestore/store.go` was removed).

Location:

- `internal/app/proposal.go` (`func auditPath`);
- `internal/storage/filestore/store.go` (`func auditPath`).

Observed impact: identical audit shard-path logic exists in the application
layer and in the storage adapter. Storage-layout knowledge leaks into
`internal/app`, and a future change to audit sharding must be made twice or
the two layers silently diverge.

Suggested resolution: keep path construction in the storage layer (or a shared
helper) and have the application layer pass timestamps, not paths.

### GAP-003: `internal/domain` has no direct tests

Status: closed (2026-07-24, slice 2: added
`internal/domain/status_test.go` with table-driven coverage for every
`ValidXxx` predicate, including negative cases such as `done`/`on_hold`).

Location: `internal/domain/` contains only `types.go` and `status.go`, no
`*_test.go`. The `ValidXxx` predicates are exercised only indirectly through
`internal/app/validate_test.go`.

Observed impact: a change to a status predicate that is not reached by the
validation fixtures would pass `go test ./...` unnoticed. Table-driven
predicate tests are cheap and would also document the allowed status sets.

Suggested resolution: add a small table-driven `status_test.go` covering all
`ValidXxx` predicates.

### GAP-004: stale status markers in design documents

Status: closed (2026-07-24, slice 3: swept every `design-work-plan.md` topic
status to `closed` with a closure map to the approved spec that resolved it,
replaced the stale "Current next topic" block, and promoted `mvp-boundary.md`
from draft to approved).

Location:

- `docs/spec/design-work-plan.md` — design topics are still marked
  `Status: pending` / `Status: in discussion` although their content is closed
  by later approved specifications (canonical model, workload calculation,
  scenario model, functional design checklist and others); the
  `## Current next topic` section still points to
  "IV. Implementation sequence specification", which is already approved and
  implemented;
- `docs/spec/mvp-boundary.md` — still `Status: draft` while the MVP it bounds
  is audited as ready in `docs/spec/mvp-readiness.md`.

Observed impact: a reader (or an AI CLI agent) following the documented
statuses is pointed at work that is already finished and cannot tell which
checklist is authoritative. `docs/spec/functional-design-checklist.md` is
closed while `design-work-plan.md` looks mostly open.

Suggested resolution: sweep `design-work-plan.md` topic statuses to point to
the approved specification that closed each topic, replace the stale
"Current next topic" block, and either promote `mvp-boundary.md` out of draft
or record why it intentionally stays draft.

### GAP-005: `internal/reports/html.go` is the largest module with the thinnest proportional test coverage

Status: closed on 2026-09-25 (`task.self.pin-dashboard-html-golden`).

Location: `internal/reports/html.go` (~1300 lines) is covered only by the
shared `internal/reports/render_test.go` (~450 lines across all renderers);
`scripts/smoke-mvp.sh` greps generated dashboard output for a few markers.

Observed impact: dashboard and Gantt HTML rendering regressions (missing
sections, broken escaping, wrong grouping) can pass `go test ./...` as long as
the smoke-grepped markers survive.

Suggested resolution: not urgent while the dashboard contract is still
moving; when the static overlay data contract stabilizes (see
[`../spec/openproject-overlay-strategy.md`](../spec/openproject-overlay-strategy.md)),
add golden-file tests for `index.html` / `gantt.html` rendering.

Progress on 2026-09-25: `WriteProjectStateMarkdown`, the function the lint
baseline flags with cognitive complexity 143, is now pinned whole by
`internal/reports/testdata/project_state.golden.md`, with a fixture that
reaches every section it can emit. The HTML writers still have no golden test.

Resolution, 2026-09-25: `internal/reports/html_golden_test.go` pins
`index.html` and `gantt.html` whole, for a rich fixture and for an empty
project, in `internal/reports/testdata/`. The rich fixture merges the two
existing HTML fixtures and adds every tracking status, a task without a
work-package URL, the open-proposal badge, unscheduled tasks, an unnamed
resource, a truncated tracking-loop table, source profiles, unlinked and
missing-link tasks, errors, a twelve-day range with an edge task and clamped
allocations, and text that must be escaped; the test also fails on any
unescaped fixture text. It covers 321 of the 505 statement blocks in
`html.go`, more than all earlier tests and a real render of `small-team`
together (282). What stays uncovered is 126 write-error returns and defensive
clamps for malformed Gantt data (zero-day ranges, unparsable dates, a status
date outside the range). A removed section and a removed escape both fail the
test. The weekly HTML report (`WriteWeeklyHTML`) is not part of this gap and
has no golden file.

### GAP-006: audit plan mutation command uses a nonexistent `--date` flag

Status: closed (2026-07-24, fixed in the same change that recorded this gap:
the plan's mutation command now uses `--occurred-at` and `--status-date`).

Location: `docs/spec/mvp-readiness-audit-plan.md`, section "Mutation Evidence" —
the required `actual add-consumption` command passed `--date 2026-07-10`.

Observed impact: `replan actual add-consumption` has no `--date` flag; the
implemented flags are `--occurred-at` (RFC3339) and `--status-date`
(YYYY-MM-DD) (`cmd/replan/main.go`). Both auditors following the plan verbatim
fail on a required evidence step with a usage error.

Resolution: replaced `--date <day>` with
`--occurred-at <RFC3339> --status-date <day>` in the plan.

### GAP-007: documentation indexes describe `mvp-readiness.md` as a planned audit

Status: closed (2026-07-24, fixed in the same change that recorded this gap:
both index entries now describe the file as audit definition plus dated audit
results).

Location:

- `README.md` documentation map entry for `docs/spec/mvp-readiness.md`
  ("planned MVP readiness audit ...");
- `docs/README.md` entry for `spec/mvp-readiness.md` (same wording).

Observed impact: the file already contains a completed dated audit
(`Audit 2026-07-16`, verdict `ready`, 0 blockers) plus follow-up records. A
reader or auditor navigating from the indexes concludes no audit has happened
yet. The audit plan explicitly asks auditors to verify that the docs map
points to the right specs, so the stale wording generates noise findings.

Resolution: reworded both index entries to say the file holds the audit
definition and dated audit results.

### GAP-008: `mvp-readiness.md` mixes audit definition, audit results and a pilot journal

Status: closed (2026-07-24, slice 3).

Location: `docs/spec/mvp-readiness.md` — audit definition (top), dated audit
and operator-validation results, and ~20 dated operational sections that are
an OpenProject pilot journal (tunnel setup, live validation runs, accept-flow
implementation notes, OSS decisions).

Observed impact: the document has three genres under one title; the header
still claims it only "defines the next readiness audit". Operational pilot
detail (SSH tunnel commands, VPS port) also overlaps with OSS sanitization
work tracked in [`oss-separation-plan.md`](oss-separation-plan.md) item 7 and
with [[GAP-010]].

Status update: closed (2026-07-24, slice 3). The dated operational journal was
moved to `docs/spec/openproject-pilot-log.md` (moved to private storage on
2026-10-27); `mvp-readiness.md` now holds only the audit definition and dated
audit results, with a corrected header.

### GAP-009: audit plan mutation command uses fixture ids that do not exist

Status: closed (2026-07-24, fixed in the same change that recorded this gap:
the plan's mutation command now uses real `examples/small-team` ids).

Location: `docs/spec/mvp-readiness-audit-plan.md`, section "Mutation Evidence".

Observed impact: the required command referenced `task.api`,
`resource_demand.api.ivan`, `actor.human.maintainer` and
`evidence.manual.audit-test`, none of which exist in `examples/small-team`.
All three 2026-07-24 auditors independently had to substitute real ids. The
plan's own note allows substitution, but shipping unrunnable ids in a
procedure meant to produce comparable results is drift.

Resolution: replaced with `task.op.1234`,
`resource_demand.task-op-1234.implementer`, `actor.human.operator` and
`evidence.github.pr.45` (`resource.human.ivan` already existed).

### GAP-010: private OpenProject endpoint detail in public files conflicts with the boundary contract

Status: closed (2026-10-27).

History: partially closed on 2026-07-24, slice 3. The specific private endpoint
values were genericized: the private port became an example port and the
private host alias became a placeholder. Only three existing files
actually carried the port — `README.md`, `docs/dev/getting-started.md` and
`docs/spec/openproject-live-mapping-quality.md` (moved to private storage on
2026-10-27) — plus the new `docs/spec/openproject-pilot-log.md` (moved to
private storage on 2026-10-27) extracted from the audit. The other files
originally listed here (`openproject-operator-loop.md`, `storage-layout.md`,
`implementation-sequence.md`, `cli.md`) only reference the `OPENPROJECT_TOKEN`
env-var name or the `apikey:<token>` auth scheme, which are kept because they
are legitimate public API surface. Remaining open: the larger private pilot
NARRATIVE (real work-package/user/relation ids and blow-by-blow pilot runs)
still lived in those two documents and the old getting-started history. That
history records why relocation was deferred at the time.

Resolution: the full pilot journal, mapping audit and old implementation log
were moved to private storage; the two pilot-specific sidecars were deleted;
current public guidance now lives in
[`openproject-pilot-lessons.md`](openproject-pilot-lessons.md). Remaining
mentions of protected connections or infrastructure are generic product
rules, sanitized historical audit findings or public self-dogfood history
rather than the private pilot runbook.

Backlog (2026-10-15, the operator's decision to plan every open or accepted
gap): `task.self.move-pilot-narrative-to-private-storage`.

Location: the private operational endpoint and/or the
`OPENPROJECT_TOKEN` placeholder appear in eight committed public files:
`README.md`, `docs/spec/openproject-live-mapping-quality.md` (moved to private
storage on 2026-10-27),
`docs/spec/openproject-operator-loop.md`,
`docs/spec/implementation-sequence.md`, `docs/spec/storage-layout.md`,
`docs/spec/cli.md`, `docs/dev/getting-started.md` and
`docs/spec/mvp-readiness.md`.

Observed impact: these are not secrets (loopback address, env-var name, no
token value), but their presence in the public repository conflicts with
`docs/dev/repository-boundary-contract.md`, which requires private pilot
operational detail to stay out of the public product repository. The
2026-07-24 audit's opencode-glm auditor weighted this leak as the main reason
for its `usable_with_limitations` verdict.

Suggested resolution: as part of the OSS-boundary slice, move private
connection/endpoint detail to a private pilot or infrastructure doc, or replace it
with a generic placeholder (for example `op.example.internal`) in public
files. `oss-separation-plan.md` items 5-7 own the wider private-pilot
separation; this entry records the concrete file-level scope.

Checkpoint update 2026-08-02:

The OSS boundary checkpoint confirmed that this gap remains release-blocking for
public OSS publication, but not blocking for continued private incubation or the
next OpenProject-centered dogfood cycle. The highest-risk remaining files are:

- `docs/spec/openproject-pilot-log.md` (moved to private storage on
  2026-10-27);
- `docs/spec/openproject-live-mapping-quality.md` (moved to private storage on
  2026-10-27);
- the historical OpenProject sections in `docs/dev/getting-started.md`;
- `examples/openproject/sidecars/resource-demands.replan-pilot.yaml` (moved to
  private storage on 2026-10-27);
- `examples/openproject/sidecars/resource-links.replan-pilot.yaml` (moved to
  private storage on 2026-10-27).

See [`oss-boundary-checkpoint-2026-08-02.md`](oss-boundary-checkpoint-2026-08-02.md).

### GAP-025: public OSS release lacks license and contribution/security entry points

Status: closed (2026-10-28).

Resolution: root `CONTRIBUTING.md` records the local gate, backlog handoff,
safety contracts, privacy boundary and Apache-2.0 contribution terms. Root
`SECURITY.md` uses GitHub private vulnerability reporting as the only report
channel and defines supported versions, scope and best-effort handling. Both
are linked from the root and documentation indexes. Apache-2.0 licensing was
already present in `LICENSE`, `NOTICE`, README and both plugin gemspecs.

Backlog (2026-10-15, the operator's decision to plan every open or accepted
gap): `task.self.add-contributing-and-security-policy`.

Id note: registered on 2026-08-02 as a second `GAP-013`, while that id already
belonged to the proposal audit-id gap of 2026-07-26. Renumbered on 2026-09-24.
Documents dated before that, including the 2026-09-21 project audit, call it
`GAP-013 license/process`.

Location: repository root.

Observed impact: no `LICENSE`, `CONTRIBUTING.md` or `SECURITY.md` file exists.
This is acceptable during private incubation, but it blocks a serious public
OSS release because users and contributors do not have clear legal and process
entry points.

Suggested resolution:

- choose an explicit license;
- add `LICENSE`;
- add minimal `CONTRIBUTING.md`;
- add minimal `SECURITY.md`;
- update `README.md` with the selected license statement.

### GAP-011: `cli.md` documents workload filters that are not implemented

Status: closed on 2026-09-25 (`task.self.clean-dead-code-and-workload-spec`).

Location: `docs/spec/cli.md`, `replan workload` section — the optional-filters
and time-selector blocks list `--resource-class`, `--unit`, `--task`,
`--include-scenarios`, `--from`/`--to` and `--status-date`.

Observed impact: only `--week`, `--resource` and `--scenario` are implemented
in `cmd/replan/main.go` (`runWorkload`); the other flags are not parsed. A
reader or AI CLI agent following the spec would pass flags that the binary
ignores or rejects.

Suggested resolution: the spec now annotates which filters are implemented
versus planned (slice 2). Either implement the remaining filters or keep the
annotation until they are built. Tracked here so the code/spec gap stays
visible.

Resolution: `--resource-class` and `--unit` are implemented as result-row
filters with a unit test. `--task`, `--include-scenarios`, `--from`/`--to`
and `--status-date` were removed from the contract and listed as not
implemented; the binary rejects them as unknown flags. The section was
rewritten to the real behaviour: `--week` is optional and without it the
command sums over all weeks, the week filter keeps actuals by status date and
demands by timing or task window, and the JSON example matches the actual
result shape, which has no `period` object.

### GAP-012: missing project root reported as validation failure, not not-found

Status: closed (2026-10-27).

Backlog (2026-10-15, the operator's decision to plan every open or accepted
gap): `task.self.report-missing-project-root-as-not-found`.

Location: `internal/storage/filestore/store.go` (`Load` emits
`missing_required_file` diagnostics for absent required files) and the
per-command exit-code mapping in `cmd/replan/main.go` (maps
`ErrValidationFailed` to exit `2`).

Observed impact: pointing a command at a non-existent project directory exits
`2` (validation failed) with explicit `missing_required_file` errors, while
`docs/spec/cli.md` previously implied a missing project uses exit `4`. All
three 2026-07-24 auditors noted the mismatch.

Reason accepted: distinguishing "project root absent" from "project present but
invalid" cleanly would require a not-found guard across roughly fifteen
commands. The behavior is safe and self-describing (`ok:false`, non-zero exit,
per-file diagnostics), so slice 2 resolved the drift by amending the spec to
document the implemented rule rather than refactoring every command. Exit `4`
remains reserved for a missing object referenced by id inside a loadable
project.

Resolution: `Store.Load` now guards the shared project root before reading any
project file. An absent root or non-directory returns one `project_not_found`
diagnostic, and every workspace-loading CLI command maps it to exit `4` in JSON
and human-readable output without creating project files. Existing incomplete
directories and other stat/read errors retain the validation path and exit `2`.
Routing the direct validation branches through the shared not-found mapping also
aligns missing scenario references with the CLI contract: workload, weekly and
dashboard reports, TaskJuggler run/export, and workload CSV export now return
`scenario_not_found` with exit `4` instead of exit `2`.

### GAP-013: proposal decision audit ids collide during rapid accepts

Status: closed (2026-07-26).

Location: `internal/app/proposal.go`, `proposalDecisionAuditRecord`.

Observed impact: the first RePlan-on-RePlan dogfood cleanup cycle accepted
multiple `update_remaining_work` proposals in a tight AI CLI loop. The proposal
decision audit id used a compact timestamp and decision name, but did not
include the proposal id. Because the timestamp was effectively second-level for
this id shape, two accepts inside the same second produced duplicate audit ids.
Validation correctly rejected the project after the second audit append.

Why it mattered: RePlan is explicitly AI-CLI-first. Fast repeated operations
are normal for Codex, Claude Code, OpenCode and similar agents, so ids that are
safe only under slow manual operation are not acceptable on mutation paths.

Resolution: proposal decision audit ids now include a slug of the proposal id,
so two proposal accepts at the same timestamp still produce distinct audit
records. Regression coverage accepts two tracking proposals with the same fixed
test timestamp and asserts that both audit ids are present and distinct. The
two local audit records written before the fix during the dogfood cycle were
updated to the new id shape.

### GAP-014: workload actual totals do not aggregate actual records

Status: closed (2026-07-30).

Location:

- `internal/app/workload.go` (`WorkloadService.Calculate` adds
  `demand.ActualAmount` to `row.Actual`);
- canonical actual records under project `actuals/*.yaml`;
- `internal/app/actual.go` (`ActualService.AddConsumption` writes actual
  records but does not mirror the value into `resource_demands.actual_amount`).

Observed impact: during the 2026-07-30 self-dogfood calibration closure,
`actual add-consumption` recorded 4 maintainer hours, 3 Codex `agent_hours`
and 1 reviewer `review_hour` for
`task.self.calibrate-plan-and-resources`. `replan actual list` shows the three
records correctly, but `replan workload --project examples/replan-self --week
2026-W31 --format json` still reports `actual: 0` for those resources because
the workload calculation does not sum `ws.Actuals`.

Why it matters: RePlan's tracking model treats actual records as the canonical
execution evidence. If workload ignores those records, Plan/Actual reports can
show completed work as zero actual consumption unless every demand also carries
a denormalized `actual_amount`.

Resolution: workload aggregation now sums canonical `work_consumed` actual
records by resource and unit for the requested week. If a demand has matching
canonical actual records, workload does not also add that demand's denormalized
`actual_amount`, avoiding double counting. Demands without canonical actual
records still use `resource_demands.actual_amount` for legacy/imported data.
Regression coverage asserts that actual records change workload `actual`
without manual edits to `resource_demands.yaml`.

### GAP-015: `actual` audit ids collide on equal `--occurred-at`, and the failed command still writes

Status: closed on 2026-09-24 by
`task.self.fix-actual-audit-ids-and-demand-reopen` (found by dogfood on
2026-09-21 while closing `task.self.design-openproject-wiki-api-access`).

Resolution: the audit ids of `actual add-consumption`, `actual
set-remaining`, `actual add-status` and `scenario apply` now end with the
target slug. Every writing command stages its writes, validates the would-be
project and commits only when it is valid; a command that reports
`applied: false` has written nothing, and `written_files` lists exactly what
reached the disk. Tests: a batch with one `--occurred-at`, and a command whose
audit id collides with an existing record, which leaves every file
byte-identical. The second test fails when staging is switched off.

Follow-up after the independent review of `ec3a8d6`: a commit that fails part
way now reports `commit_failed` with the files already written; removals are
committed before audit lines; `taskjuggler import-result --write-candidate`
also stages and validates its scenario and proposal files; `NaN` and
infinite amounts are refused by the commands and by `validate`. Review record:
[`actuals-staging-review-2026-09-24.md`](actuals-staging-review-2026-09-24.md).

Location:

- `internal/app/actual.go`: the audit id of `actual add-consumption`,
  `actual set-remaining` and `actual add-status` is
  `"audit." + compactTimestamp(occurredAt) + ".<operation>"`, with no target
  slug. `actual close-demand` in the same file already appends
  `slugID(req.DemandID)`, and proposal decisions append the proposal slug
  (the closed audit-id gap of 2026-07-26);
- `internal/app/scenario.go`: `scenario apply` has the same shape, as the
  2026-09-21 project audit already noted.

Observed impact: three `actual add-consumption` commands for three different
demands were run with the same explicit `--occurred-at`. The first succeeded.
The second and third returned `ok: false`, `applied: false`, exit code 2 and
`duplicate_id`, but had already written their actual records and appended
audit records with the duplicate id. Every later command then also reported
failure while still writing, because post-write validation of the whole project
failed. The final state was the intended one, but the project no longer
validated and the command envelopes misreported what had happened.

Two separate defects:

1. audit ids of three `actual` commands are not unique per target, so a batch
   with one `--occurred-at` collides. Passing one timestamp to a batch is the
   natural way to record one decision;
2. a command can report `applied: false` after it has written files. For an AI
   CLI agent that is the more dangerous defect: the envelope is the agent's
   only evidence of what it did.

Repair applied to data, not to code: the two duplicate audit ids in
`examples/replan-self/audit/audit.2026-09.jsonl` were disambiguated by
appending the resource slug, the same shape the fixed code would produce.
`replan validate` and `replan audit verify` pass after the repair.

Workaround until fixed: give every `actual` command in a batch a distinct
`--occurred-at`.

Suggested resolution: append a target slug to the remaining audit ids as
`close-demand` does; validate the would-be state before writing, or report
`applied: true` together with the validation failure. Should be planned as a
backlog task together with the dry-run-by-default follow-up from the
2026-09-21 project audit, because both are AI CLI mutation-safety work.

Backlog, 2026-09-24: `task.self.fix-actual-audit-ids-and-demand-reopen`. The
dry-run-by-default follow-up is deliberately not part of it; see GAP-023.

### GAP-016: a reviewed overrun cannot be acknowledged, and the smoke pinned zero suggestions

Status: closed on 2026-09-24 by
`task.self.fix-actual-audit-ids-and-demand-reopen` (found by dogfood on
2026-09-21, same session as GAP-015).

Resolution:

- `remaining_amount` is a pointer in the domain model, so an explicit `0` is
  written and survives every later rewrite; an absent field still means "not
  estimated". The resource schedule check already treats an explicit zero as
  zero load;
- `replan actual reopen-demand` returns a consumed or cancelled demand to
  active, optionally with a new remaining amount, with an audit record;
- `replan actual acknowledge-overrun` records an `overrun_review` on the
  demand. `tracking suggest` stays quiet while the forecast does not exceed
  the acknowledged amount and raises the suggestion again when it grows; the
  dashboard marks the demand `overrun_acknowledged`;
- the MVP smoke expects an empty suggestion list again. The two real
  overruns in the self-dogfood project are acknowledged through the command,
  not bent to fit.

Not changed: `tracking suggest --week` still reports history outside the
requested week.

Location:

- `internal/app/tracking.go`: every demand with `Variance > 0` yields a
  `review_forecast_over_plan` suggestion, for any requested week, forever;
- `scripts/smoke-mvp.sh`: until 2026-09-21 the self-dogfood check asserted
  `"suggestions": []`.

Observed impact: the first honestly booked overrun in the self-dogfood project,
3 `agent_hours` against 2 planned on `resource_demand.wiki-api-design.agent`,
made the MVP smoke fail. There is no command to record "this overrun was
reviewed and is real". The only ways back to a clean report are to book actuals
at or below plan, or to raise the planned amount after the fact. Both destroy
the plan-versus-actual signal that RePlan exists to keep. Before this entry,
none of the 154 actual records in the self-dogfood project exceeded its plan,
which is more likely pressure from this rule than accurate estimating.

Repair applied: the smoke now expects exactly this one review suggestion
instead of an empty list. The data was not bent to fit the test.

Suggested resolution: give the overrun review an explicit, audited outcome,
for example an accepted proposal or a demand-level acknowledgement, after which
the suggestion stops; and scope `tracking suggest --week` to the requested
week so closed history does not resurface. Until then each further honest
overrun requires another smoke edit, which is the wrong incentive.

Additional evidence for GAP-016, 2026-09-21, from closing the gate 1 review
demand on a task that was still `review_ready`:

- `actual close-demand` has no inverse. A demand closed too early can only be
  reopened by editing `canonical/resource_demands.yaml` by hand, which leaves an
  audit record for a closure that no longer holds;
- `remaining_amount` is serialized with `omitempty`, so `actual set-remaining
  --amount 0` removes the field. An `active` demand with zero remaining work
  then looks the same as a demand that was never estimated, and
  `plan-resource-demand-calibration.sh` flags it. The only states the helpers
  accept for a finished slice of work on an open task are "closed together
  with the task" or "a non-zero remainder".

Third piece of evidence for GAP-016, 2026-09-21, from booking actuals on
`task.self.verify-openproject-wiki-api-plugin-throwaway-runtime` while the task
was `review_ready`. When a demand's actual equals its plan and the task is
still open, no state reachable through commands satisfies all three checks:

- a non-zero `remaining_amount` makes forecast exceed plan, so
  `tracking suggest` raises `review_forecast_over_plan` and the smoke fails;
- `actual set-remaining --amount 0` removes the field, so
  `plan-resource-demand-calibration.sh` and `check-resource-schedule-drift.sh`
  report an active demand without a remaining amount;
- `actual close-demand` makes the calibration planner report a closed demand
  on an open task.

The only passing state is an explicit `remaining_amount: 0` in the YAML, which
the Python helpers read as "estimated, nothing left". It was written by hand
for the two throwaway-runtime demands. The Go store will drop it again on its
next rewrite of `resource_demands.yaml`, because the field is `omitempty`. It
is expected to survive only until the task is accepted and its demands are
closed.

The practical consequence so far has been a habit, visible in the project
history: book actuals only at the moment a task is accepted. That hides
work-in-progress consumption from every report between start and acceptance.
Suggested resolution, in addition to the above: make `remaining_amount` a
pointer or otherwise serialize an explicit zero.

Fourth piece of evidence for GAP-016, 2026-09-24, from the resource schedule
check after the live wiki API deployment was executed two weeks ahead of its
planned window:

- `scripts/check-resource-schedule-drift.sh` computes planning load from
  `remaining_amount` only "when present and positive, otherwise
  `planned_amount`". An explicit `remaining_amount: 0` on an active demand
  therefore still books the full planned amount in the planned window. The
  Python helpers disagree with each other on what zero means: the calibration
  planner reads it as "nothing left", the schedule check as "not estimated";
- the live deployment's local development slot, used on 2026-09-21, kept
  reserving its planned window of 2026-10-05 and overlapped a new task. The
  only way to release it through commands was `actual close-demand`, which
  the calibration planner then reports as a closed demand on an open task, and
  the MVP smoke pins the calibration plan to `aligned` on the self-dogfood
  data, so the pushed commit `65d6ea5` failed the smoke.

Partial fix, 2026-09-24: the schedule check now uses an explicit
`remaining_amount: 0` as zero load. The demand was reopened by hand to
`active` with an explicit zero, which leaves an audit record for a closure
that no longer holds. The `omitempty` serialization in the Go store is
unchanged, so the next Go rewrite of `resource_demands.yaml` drops that zero
again.

Backlog, 2026-09-24: `task.self.fix-actual-audit-ids-and-demand-reopen`.

### GAP-017: CI on `main` has been red since 2026-08-07, and local smoke was taken as CI evidence

Status: closed on 2026-09-25 (found on 2026-09-24 while reviewing the
2026-09-21 project audit; fixed by `task.self.fix-ci-smoke-ripgrep-dependency`
in `0572495` on 2026-09-24).

Location:

- `scripts/check-openproject-plugin-scaffold.sh`: `must_match`,
  `must_contain` and related helpers call `rg`;
- `.github/workflows/ci.yml`: the `go` job runs `./scripts/smoke-mvp.sh`,
  which calls the scaffold check first.

Observed impact: the GitHub-hosted runner has no `rg`. Every push to `main`
from `16dadad` on 2026-08-07 to `135e995` on 2026-09-24, nineteen runs, failed
in the `Go` job at the scaffold check, before the first smoke assertion.
`go test ./...` passes in the same runs. Locally `rg` is installed, so every
local smoke passed. The 2026-09-21 audit could not list the runs, recorded CI
as an acceptable limitation and inferred green CI from local runs. Every
"green HEAD" statement made in that period, including the wiki API slice,
rested on local evidence only.

Suggested resolution: use portable `grep -E` / `grep -F`, and treat the CI
run of the pushed commit, not a local smoke, as the acceptance evidence for
code tasks.

Backlog: `task.self.fix-ci-smoke-ripgrep-dependency`.

Resolution: the scaffold check uses portable `grep` since `0572495`. The last
red run on `main` was on 2026-09-24 before that commit; all 22 runs on `main`
after it passed. Code tasks have been accepted on the CI run of the pushed
commit since then. The fix task was accepted on 2026-09-24, but the gap stayed
open until the cleanup of 2026-09-25, the same stale-record pattern as
GAP-021.

### GAP-018: 3 916 lines of Python live inside shell heredocs, invisible to analyzers and duplicated

Status: closed on 2026-09-24 by
`task.self.move-heredoc-python-into-script-modules` (found by the 2026-09-24
code quality tooling trial).

Resolution: a line-based inventory found 25 blocks and 4 527 lines, more than
the trial's first count, because two helpers split the `python3` call over
several lines. All 25 are modules under `scripts/lib/` now, invoked as
`python3 -m`; the shell scripts keep their names, flags, environment
variables, dry-run defaults and `REPLAN_APPLY` gate. Four copies of the
import-failure envelope became `openproject_import_failure`, three small
envelope checks became `envelope_check`, two smoke fixture edits became
`smoke_fixture_edits`. The five `auth_headers` copies delegate to
`openproject_http.openproject_auth_headers`; HTTP error bodies are reduced to
OpenProject's error identifier and messages, raw bodies are no longer echoed.
The failure envelope now says whether the binding file exists. Old and new
versions of ten helpers, five of them against live OpenProject in dry-run,
and seven toolkit commands produced byte-identical output. Unit tests in
`scripts/tests/` run from the MVP smoke and fail if a heredoc returns or a
module other than `openproject_http` builds the auth header.

Not done: the large moved modules are still script-shaped, top-level code;
their behavior is covered by the smoke and the byte comparison, not by unit
tests of their internals.

Location: 22 `python3 - <<'PY'`-style blocks in the OpenProject and planning
scripts under `scripts/`, among them `apply-openproject-bootstrap.sh`,
`apply-openproject-relations.sh`, `push-openproject-date-drift.sh`,
`push-openproject-status-drift.sh`, `openproject-work-package-toolkit.sh`,
`check-openproject-drift.sh` and `check-resource-schedule-drift.sh`. The only
shared module is `scripts/lib/openproject_http.py`.

Observed impact:

- more than half of the script logic is not visible as code to any Python
  linter, type checker, duplicate detector or code-graph index. GitNexus found
  three of the five scripts that build the OpenProject auth header, as files,
  and could not point at the code inside them;
- `Authorization: Basic` construction is duplicated in five scripts instead of
  living in `openproject_http.py`;
- `openproject_http.py` echoes up to 1 000 characters of an unparsable HTTP
  error body into operator JSON. That is not the token, but it can carry
  private work-package titles or field values into logs;
- none of this code has unit tests; only the MVP smoke exercises it.

Suggested resolution: move each block into a module under `scripts/lib/`,
keep the shell scripts as thin entry points with unchanged interfaces, build
the auth header and parse errors in one place, stop echoing raw bodies, add
unit tests.

Backlog: `task.self.move-heredoc-python-into-script-modules`.

### GAP-019: file store writers repeat read-modify-write, and multi-file applies are not atomic

Status: closed on 2026-09-24 by
`task.self.fix-actual-audit-ids-and-demand-reopen`, with a stated residual
(confirmed by the 2026-09-24 tooling trial; first described by the 2026-09-21
project audit).

Resolution: every YAML write goes through one `updateYAML` helper, and every
file access goes through a small file-system layer with a disk and a staged
implementation (`internal/storage/filestore/files.go`). Commands write into
the stage, validate it and commit. Commit writes data and proposal files
before audit files, so an interrupted commit can leave a change without its
audit line but never an audit line for a change that did not happen.

Residual: each file is replaced atomically, the set of files is not. A crash
in the middle of a commit can still leave a partial multi-file change on
disk. Closing that needs a journal and is not planned.

Location: `internal/storage/filestore/store.go`. `SetResourceDemandRemaining`,
`SetResourceDemandStatus` and further writer methods each repeat read file,
unmarshal a wrapper, find by id, mutate, marshal, atomic write. `dupl` reports
three- and two-way clone groups over these methods. `MoveProposal` writes the
target file and then removes the source.

Observed impact: a single file is written atomically, but a command that
writes canonical YAML, moves a proposal and appends audit is three independent
writes. A failure in the middle leaves canonical state changed with the
proposal still open, or two copies of a proposal. GAP-015 is the observed case
of a command that wrote and then reported failure. Any fix to write ordering
or pre-write validation has to be repeated in every copy of the sequence.

Suggested resolution: extract one generic update helper first, then make
pre-write validation and write ordering a property of that helper.

Backlog: folded into `task.self.fix-actual-audit-ids-and-demand-reopen`.

### GAP-020: complexity and duplication hotspots, and no lint gate beyond `go vet`

Status: closed on 2026-10-27.

Backlog (2026-10-15, the operator's decision to plan every open or accepted
gap): `task.self.clear-remaining-lint-baseline`.

Location and measurement, non-test Go, 18 376 lines:

```text
validateWorkspace                 internal/app/validate.go     cyclomatic 93, cognitive 148
WriteProjectStateMarkdown         internal/reports/markdown.go cyclomatic 79, cognitive 143
ImportAcceptService.Accept        internal/app/import_accept.go cyclomatic 63, cognitive 124
functions over cyclomatic 30      14
duplicated Go lines (jscpd, 70 tokens)  1 277 of 18 376, 6.95%
  inside cmd/replan/main.go       685
  inside internal/app/actual.go   242
```

Named clone groups: four near-identical 50-line branches in
`ImportAcceptService.Accept`, one per accept mode; three OpenProject page
fetchers in `import_openproject.go` that differ only in the collection type;
per-command Markdown writers and flag parsing in `main.go`.

Observed impact: CI runs `go test` and the smoke only. `gofmt` drift already
reached `main` unnoticed. Nothing stops the hotspots from growing, and each
new command adds another copy of the writer and flag boilerplate.

Suggested resolution: a pinned `golangci-lint` baseline with a gate on new
findings only. Refactoring the hotspots is a separate decision, to be taken
when a task has to change them.

Backlog: `task.self.add-go-lint-baseline-and-ci-gate`.

Progress:

- 2026-10-27: the remaining baseline was cleared. The full pinned
  `golangci-lint run ./...` reports zero issues with unchanged thresholds.
  Typed helpers remove the page-fetcher and sidecar duplication, named steps
  reduce the store and dashboard tracking hotspots, shared CLI flows remove
  command duplication, and the four remaining ordered pipelines are split into
  explicit stateful passes with failure precedence pinned by tests. Only seven
  flat byte-stable renderers retain narrowly justified annotations. The local
  gate now checks the full run while CI is paused.

- 2026-09-24: the lint gate and baseline are in CI
  (`docs/dev/lint-baseline-2026-09-24.md`);
- 2026-09-25: `WriteProjectStateMarkdown` is split into one writer per
  section (`task.self.split-project-state-markdown`). The largest of them has
  cyclomatic complexity 8 and cognitive 12, down from 79 and 143 for the whole
  function. Output is byte-identical: four state Markdown golden files, three
  of them generated with the old writer before the switch, and the rendered
  state reports of `replan-self` and `small-team`. The remaining top
  hotspots are `validateWorkspace` (cognitive 168) and
  `ImportAcceptService.Accept` (124), planned as
  `task.self.split-validate-workspace` (2026-10-12..14) and
  `task.self.dedupe-import-accept-modes` (2026-10-19..20).
- 2026-10-15: `validateWorkspace` is split into one validator per record kind
  (`task.self.split-validate-workspace`). The calls keep their order, which
  decides what a reference resolves to; the diagnostics are held
  byte-identical by a golden file of a workspace that triggers all 60 codes,
  a golden file of the small-team example, and a before-and-after run on the
  self-dogfood project. No function of `validate.go` is in the lint report
  any more. With CI paused since 2026-10-14, the gate runs locally with the
  same pinned golangci-lint.
- 2026-10-15: `ImportAcceptService.Accept` is one flow, `applyMode`, and one
  description per accept mode with its own change builders, write action,
  diagnostics and store writer (`task.self.dedupe-import-accept-modes`).
  A golden run of all six modes, each as a dry run, an apply and an apply
  with nothing safe to write, holds the command result and the bytes of every
  written file, audit records among them, identical to the old code. No
  finding of `import_accept.go` is left in the lint report.

### GAP-021: dead code, and code-graph flows seeded from it

Status: closed (the code was removed on 2026-09-24 in the lint gate commit
`8175cfa`; the gap was closed on 2026-09-25 after `deadcode ./...` and the
`unused` linter reported nothing).

Location:

- `internal/app/import_accept.go`: `hasDependencyTriple` is unused
  (`staticcheck` U1000, `deadcode`);
- `internal/app/actual.go`: `AddConsumptionDryRun`, `SetRemainingDryRun`,
  `CloseDemandDryRun` and `AddStatusDryRun` are called only by tests. The CLI
  passes the dry-run flag in the request instead.

Observed impact: small in itself. But GitNexus built its execution flows
through the file store starting at these wrappers, so a flow view reads a
test-only helper as the entry point of a real write path.

Suggested resolution: remove the unused function; either delete the wrappers
and test through the request flag, or keep them and mark them as test
helpers.

Backlog: `task.self.add-go-lint-baseline-and-ci-gate`.

### GAP-022: command writes reformat hand-maintained canonical YAML

Status: closed on 2026-10-15 by `task.self.remove-yaml-rewrite-noise`
(described by the 2026-09-21 project audit; observed on 2026-09-24).

Location: `internal/storage/filestore/store.go`, whole-file unmarshal and
marshal on every update.

Observed impact: one `actual add-status` on a task rewrote the quoting of
seven unrelated hand-written task texts in
`examples/replan-self/canonical/tasks.yaml`, with no content change. Comments,
key order and quoting style are not preserved, so review diffs of canonical
files mix intended changes with rewrite noise.

Suggested resolution: node-level YAML editing that preserves untouched nodes,
or a canonical formatter applied to hand edits as well, so both paths produce
the same bytes.

Backlog: `task.self.remove-yaml-rewrite-noise`, planned for 2026-09-29 to
2026-10-02. The store update helper of GAP-019 is the single place to change.

Resolution: every YAML write of the file store goes through `renderYAML`
(`internal/storage/filestore/splice.go`). It splices into the file's own bytes
only the records, matched by id, and the fields, down to nested mappings,
that changed; appended records take the indentation of the list. The result
must decode to exactly what a whole-file render gives, or the whole file is
rendered as before; flow-style lists fall back that way. Keys the domain
types do not model are kept where the check allows it. That showed a second
defect the noise had hidden: evidence records carry `url` as the evidence
spec says, the domain type did not model it, and every command write to an
evidence file dropped it. `domain.Evidence` now has `URL`. Byte-level tests
use hand-written files with comments, both quote styles and two-space
indentation, and the self-dogfood evidence file of 2026-W42, which a
whole-file render would have rewritten in about 700 lines.

### GAP-023: mutating CLI commands default to write, not to dry-run

Status: closed on 2026-10-26 by
`task.self.one-apply-gate-rule-for-cli-and-helpers`.

Resolution:

- every Go command that writes canonical project state now uses one shared
  `--dry-run`/`--apply` gate;
- neither choice is refused with exit `8` unless `REPLAN_APPLY=1` supplies the
  apply choice, both flags are refused, and explicit `--dry-run` wins over the
  environment fallback;
- repository callers pass a flag explicitly, helper behavior remains
  dry-run-by-default, and the CLI and agent-kit documentation state one rule.

### GAP-024: preferred dependencies have no proposal path to resolution

Status: closed on 2026-09-24 by
`task.self.fix-actual-audit-ids-and-demand-reopen`: `proposal
suggest-dependencies` now offers `mark_dependency_resolved` for any open
dependency whose predecessor satisfies it, whatever its criticality. Review
proposals for unsatisfied dependencies stay limited to `required` ones.
(Found on 2026-09-24.)

Location: `internal/app/proposal.go`, `dependencyProposalCandidates` returns
candidates only for `criticality: required`.

Observed impact: when the predecessor of a `preferred` dependency is
accepted, `proposal suggest-dependencies` offers nothing, and there is no
other command that resolves a dependency. It can only be resolved by editing
`canonical/dependencies.yaml` by hand, with no audit record.
`dependency.code-graph-evaluation-before-next-product-slice` was resolved
this way on 2026-09-24.

Suggested resolution: generate `mark_dependency_resolved` candidates for any
open dependency whose predecessor satisfies it, regardless of criticality.

Backlog: none yet; small enough to join
`task.self.fix-actual-audit-ids-and-demand-reopen` if the operator agrees.

### GAP-026: the live OpenProject workflow cannot reach the status mapped to `review_ready`

Status: closed on 2026-09-24 (found during the post-pause re-entry).

Location: `scripts/push-openproject-status-drift.sh` maps `review_ready` to
`Developed`. The live OpenProject workflow for the `Task` type allows, from
`In progress`, only `New`, `In progress`, `On hold`, `Rejected` and `Closed`.
`Developed` is not reachable from `New` either.

Observed impact: `task.self.deploy-openproject-wiki-api-plugin-live` is
`review_ready` in RePlan and stays `In progress` in OpenProject. The drift
check reports it as one status soft drift. The helper reports
`openproject_workflow_transition_not_allowed` and changes nothing, which is
correct.

Suggested resolution: an operator decision, not a code fix. Either allow
`In progress -> Developed -> Closed` for the role in the OpenProject workflow
editor, which is shared with the other project on the instance, or map
`review_ready` to a status the workflow allows. Until then the soft drift is
expected and recorded.

Resolution: the operator chose to allow `Developed`. The workflow now allows
`In progress -> Developed` and, from `Developed`, `In progress`, `Closed`,
`On hold` and `Rejected`, for the `Member` and `Project admin` roles on the
`Task`, `Milestone` and `Summary task` types. That adds 30 transitions and
changes no existing one. The workflow table was backed up on the host first.
The status mapping in the helper is unchanged. A check as the operator's user
on an `In progress` work package lists `Developed` among the assignable
statuses. The deployment task was then accepted and pushed as `Closed`, and
the drift check reports `aligned` with no status drift.

### GAP-027: the live RePlan overlay used sanitized public work-package ids

Status: closed on 2026-09-24 (found during the post-pause re-entry).

Location: the live overlay artifact published on 2026-08-16 was generated
from `examples/replan-self`, whose tasks carry sanitized ids from `2101`
upward. The OpenProject plugin's work-package panel looks rows up by the real
work-package id.

Observed impact: on the live instance the work-package panel could not find a
RePlan row for any real work package, and project-page rows linked to ids that
do not exist there. The August verification used a Rails runner lookup with a
sanitized id, so it passed.

Resolution: `scripts/build-openproject-live-overlay.sh` builds the artifact
from a temporary copy of the project with source references replaced from the
ignored private binding file, fails unless every bound task is linked, prints
counts only and never publishes. The republished live artifact links 67 of 67
work packages; a Rails runner check on the live instance found a row for all
67 real work-package ids. The smoke runs the builder against the sanitized
example bindings.

### GAP-028: an accepted task can keep a future planned window without any warning

Status: closed on 2026-09-25 (found on 2026-09-24 during the post-pause
re-entry; fixed in `task.self.warn-accepted-task-future-window`).

Location: `internal/app/validate.go` and `report state`; nothing compares an
accepted task's planned dates with the project status date.

Observed impact: `task.self.sync-latest-openproject-live-backlog-drift` was
done on 2026-08-16 but kept its planned window of 2026-11-02 for five weeks.
Once live relations were applied, OpenProject's scheduling pushed its
successor, the re-entry task, into November, and the heredoc module task
after it. Nothing in RePlan had flagged the stale window.

Suggested resolution: a warning for accepted tasks whose planned finish is
after the status date, with a hint to move the window to the actual execution
dates through a scenario.

Resolution: `validate` and the weekly and state reports emit the warning
`accepted_task_planned_after_status_date` for an accepted task whose planned
finish is after the project status date, and the state report and
`replan next` list `record_actual_window` for an agent. On its first run the
check flagged 16 accepted tasks on the dogfood project, eleven of them the
August plugin series still planned into October and November. Their actual
windows were rebuilt from their actuals and acceptance audit records in one
scenario and pushed to live OpenProject one work package at a time; for five
of them the work fell on the 15-16 August weekend, and the OpenProject Monday
dates were accepted back through a second scenario.

### GAP-029: closing a demand as consumed keeps a stale remaining amount

Status: closed on 2026-09-25 (found the same day while accepting
`task.self.add-wiki-api-page-delete`; fixed in
`task.self.fix-close-demand-stale-remaining`).

Location: `internal/app/actual_lifecycle.go` and the close path of
`actual close-demand`; `report state` counts the result as
`stale_remaining_count`.

Observed impact: the last agent-hours and the CI minutes of the task were
booked, then the demands were closed as `consumed`. Both kept their old
`remaining_amount` (0.25 and 10). The MVP smoke caught it through
`stale_remaining_count`. The only fix through commands was
`reopen-demand --remaining 0` followed by a second `close-demand`, which adds
two audit records that say nothing new. Earlier in the same task a demand was
closed on a task still under review, which the calibration planner flags; the
fix there was the same reopen with an explicit zero.

Suggested resolution: `close-demand --status consumed` sets
`remaining_amount: 0` in the same audited write, or refuses with a hint when
the remaining amount is above zero, and says in its output which one it did.
Closing as `cancelled` keeps the amount, because it records work that was not
done.

Resolution: `close-demand --status consumed` sets an explicit
`remaining_amount: 0` in the same staged write and the same audit record, and
reports `remaining_cleared` with `previous_remaining` in JSON and
`Remaining: <before> -> 0` in Markdown. A demand without a remaining estimate
also gets the explicit zero. `cancelled` keeps the amount. The first use closed
the state-report task's demands with remaining 10, 1 and 0.5 in one command
each, and the smoke stayed green without the reopen workaround.

### GAP-030: a scenario cannot preview moved task dates in workload

Status: closed on 2026-10-27 (found on 2026-09-25 while planning ahead).

Resolution: the shared in-memory overlay now accepts the same task planned-date
changes as `scenario apply`, validates the combined task window after one-sided
or two-sided moves, and feeds the moved workspace to workload, reports, Gantt,
warnings, exports and scenario diff. A copied small-team fixture proves that a
moved demand appears in its new week, leaves its old week, exposes the resulting
overload before apply and produces byte-for-byte equivalent workload rows after
the scenario is applied.

Location: `internal/app/scenario_overlay.go`. The in-memory overlay used by
`workload --scenario` and `report --scenario` supports availability
exceptions, demand amounts and resources, and capacity amounts, but not
`tasks.<id>.planned_start` or `planned_finish`. `scenario apply` writes those
changes; `workload --scenario` refuses them with
`unsupported_scenario_change`.

Observed impact: OpenProject pulled two plugin tasks forward after the review
finished early. The natural question, whether accepting the new dates would
overload anything, could not be asked of RePlan. The overlap with the YAML task
on the single development slot was found by hand, and only the design move was
accepted. Moving tasks is the most common planning change, so the what-if
layer misses its most common case.

Suggested resolution: support task planned-date replacement in the overlay,
so that week filters and Gantt windows follow the moved dates, with a test
that previews an overload before applying.

Backlog: `task.self.preview-moved-task-dates-in-scenarios` (2026-10-15).

### GAP-031: weekly workload double-counts multi-week demands and sums concurrent slots

Status: closed on 2026-09-25 (found the same day while planning the plugin
track).

Follow-up 2026-10-15: the fix reached the Go workload only. The Python
schedule drift check still sums concurrent slots over a week (finding C-15 of
the [agent tooling review](agent-tooling-consistency-review-2026-10-15.md)):
`task.self.count-concurrent-slots-in-schedule-drift-like-workload`.
Resolved the same day: `replan workload --demands` lists each row's demands,
and the schedule drift check and the overload explainer read their weekly
rows from it through `scripts/lib/replan_workload.py`. One workload
calculation is left.

Location: `internal/app/workload.go`, `demandIntersectsWeek` and the per-week
sums.

Observed impact: a demand counts in full in every week its window touches, so
the 8-hour release story over two weeks shows 8 hours and a 3-hour overload
for the operator in each week. `concurrent_slots` demands are summed over the
week, so tasks that use the single development slot one after another in the
same week show as an overload in every week of the plugin plan. Both false
signals would reach `replan next` as overloads to review.

Suggested resolution: spread a period demand over the working days of its
window and count the days inside the week; check concurrent capacity by
overlapping days.

Backlog: `task.self.honest-weekly-workload-for-multi-week-and-concurrent-demands`,
2026-09-28, first in the plan so that the rest of the plan is read correctly.

Resolution: `internal/app/demand_spread.go` holds one rule for workload, the
dashboard Gantt load and the overlay. Period demands are spread over their
resource's working days and a week counts its share; slot demands count as
the peak of simultaneous use by day. The same task found two more defects of
the kind: the dashboard spread amounts over weekends too, and its weekly load
buckets used the status week's capacity for every week, so the operator's
capacity change of 2026-09-28 never showed in the load profile. Both follow the
rule now. On the self-dogfood plan, every false development-slot overload is
gone, the release story counts 4 of its 8 hours in each of its two weeks, and
the one real overload left is the operator's 2026-W46 at 5.6 of 5 hours.

### GAP-032: a dependency cannot be cancelled with an audit record

Status: closed on 2026-10-15 by `task.self.cancel-dependency-with-audit`
(found on 2026-09-27).

Location: `replan scenario apply` refuses
`dependencies.<id>.status` ("cannot be applied by this slice"), and
`proposal suggest-dependencies` offers only `mark_dependency_resolved` when
the predecessor is done, or `adjust_successor_start`.

Observed impact: the operator let
`task.self.link-openproject-work-package-to-replan-panel` go ahead of
`task.self.deploy-plugin-milestone-2`, a `preferred` dependency whose reason,
a finished task page, was already met. `validate` then warned that the
successor starts before the predecessor finishes, and the only way to record
the decision was to set `status: cancelled` in
`canonical/dependencies.yaml` by hand, with the decision in evidence and in
the dependency's notes but no audit event.

Suggested resolution: let a scenario change a dependency's status, or add a
`cancel_dependency` proposal that carries the decision's evidence, so the
change is audited like any other.

Backlog: `task.self.cancel-dependency-with-audit`, agreed by the operator on
2026-10-12 and planned beside the November refactors.

Resolution: a scenario may set `dependencies.<id>.status` to `cancelled`.
`scenario apply` then requires `--evidence` naming the decision, refuses an
unknown evidence id, writes the status through the store and carries the
evidence in the audit record. Only `cancelled` is accepted; resolving a
dependency stays with `proposal suggest-dependencies`. Covered by three
service tests and a smoke step that is refused without evidence and applies
with it.
