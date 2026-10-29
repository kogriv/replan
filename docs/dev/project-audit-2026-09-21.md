# RePlan project audit 2026-09-21

Status: completed dated project audit.

Date: 2026-09-21.

Auditor: Cursor Grok 4.6 in a local checkout.

Related RePlan task: `task.self.reenter-openproject-dogfood-cycle-after-pause`.

OpenProject sync: pending.

Reason: live OpenProject drift check failed with `openproject_network_error`
during this audit. The re-entry task is recorded in RePlan canonical data
with the `openproject_sync_pending` tag. Live OpenProject should be
synchronized through guarded helpers when the operator loop next reaches the
live instance. This document does not pretend that live OpenProject and RePlan
are aligned.

## Purpose

This audit records the observed state of the RePlan repository on 2026-09-21
after a roughly one-month pause in the OpenProject-centered dogfood cycle.

It is broader than the internal CLI-first MVP readiness audits of 2026-07-16
and 2026-07-24. Those audits remain valid as historical MVP evidence. This
audit answers:

1. Does the current implementation still satisfy the documented internal MVP
   definition?
2. Does the current working tree still pass tests and MVP smoke?
3. Is the self-dogfood plan still a credible operating plan after the pause?
4. Can the OpenProject/RePlan operator loop be run from this environment?
5. Which documentation, CLI-spec, OSS-boundary and code-quality gaps are still
   true?
6. What is the next smallest stabilization slice?

The audit was run against the repository. It does not rely on memory of earlier
design discussions. No source, fixture, test or documentation files were
changed while the audit commands were running. This document is the later
record of that already-completed audit.

This is not a public-release approval. This is not a feature design exercise.
Findings are recorded. Fixes were not implemented during the audit.

## Scope

In scope:

- CLI behavior;
- local file-based project operation;
- `examples/small-team`;
- `examples/replan-self`, including uncommitted working-tree changes;
- validation;
- workload reports;
- weekly reports;
- project-state reports;
- static dashboard generation and OpenProject overlay counts;
- scenarios, proposals, actuals and audit;
- OpenProject read-only import from committed fixtures;
- OpenProject operator helpers and live drift, when reachable;
- TaskJuggler optional runtime, when available;
- documentation usability;
- CI workflow presence;
- AI CLI friendliness;
- OSS/privacy boundary;
- known-gaps register re-verification;
- mutation safety and storage write paths;
- the read-only OpenProject overlay plugin.

Out of scope:

- implementing fixes discovered by the audit;
- building a web application;
- building a daemon or server API;
- adding custom authentication or full role-based access control;
- adding broad automatic OpenProject write-back;
- changing the canonical model;
- adding a scheduler or optimizer;
- rewriting private or public repository history;
- cleaning unrelated documentation except by recording drift;
- printing or committing live OpenProject tokens, private endpoints, hosting
  details or private live work-package identifiers.

## Method

The auditor started from the current checkout, recorded `git rev-parse --short
HEAD` and `git status --short`, then ran commands instead of inferring
behavior from documentation.

Mutation checks used a temporary copy under
`tmp/mvp-readiness-audit/cursor-2026-09-21/small-team`. HEAD tests and MVP
smoke were also run from a `git archive HEAD` tree so dirty working-tree
changes could be distinguished from committed state.

Vocabulary follows [`../spec/mvp-readiness-audit-plan.md`](../spec/mvp-readiness-audit-plan.md):

```text
done
partial
blocked
deferred
not_started
```

Layer verdicts use:

```text
usable_internal_mvp
usable_with_limitations
not_ready
```

Private live values were not copied into this public document. Live OpenProject
bindings remain in ignored storage. `.env.local` was observed to exist and was
not quoted.

## Repository identity

```text
branch: main
HEAD: 9fca426
HEAD subject: dogfood: sync latest openproject backlog drift
origin/main: 9fca426
working tree: dirty
```

Remotes observed:

```text
origin  git@github.com:kogriv/replan.git (fetch)
origin  git@github.com:kogriv/replan.git (push)
origin  git@gitlab.com:kogriv/replan.git (push)
```

Uncommitted files at audit time, `git diff --stat`:

```text
 docs/roadmap/self-dogfood-planning-calibration.md  | 78 ++++++++++++++++++++++
 examples/replan-self/canonical/current_plan.yaml   |  2 +
 examples/replan-self/canonical/dependencies.yaml   | 24 +++++++
 examples/replan-self/canonical/resource_demands.yaml | 56 ++++++++++++++++
 examples/replan-self/canonical/tasks.yaml          | 30 +++++++++
 5 files changed, 190 insertions(+)
```

Those five files are the 2026-09-18 re-entry slice: Step 43 in the calibration
worksheet plus canonical membership, dependencies, resource demands and the
new `in_progress` task `task.self.reenter-openproject-dogfood-cycle-after-pause`.
The task has no sanitized OpenProject `source_refs`.

Recent committed history immediately before the dirty tree:

```text
9fca426 dogfood: sync latest openproject backlog drift
099b011 dogfood: stabilize openproject helper timeouts
24055d9 tooling: optimize openproject plugin image builds
1344f75 dogfood: record live work-package panel deployment
d3e5e4a feat(openproject): add work-package replan panel
```

## Verdict

Status by layer:

| Layer | Status |
| --- | --- |
| Internal CLI-first MVP on clean HEAD `9fca426` | `usable_internal_mvp` |
| Current dirty working tree | not CI-green; tests and MVP smoke fail |
| Self-dogfood / OpenProject operator loop | `usable_with_limitations` |
| Public OSS release | `not_ready` |

Rationale:

The committed HEAD still implements the internal CLI-first MVP loop: validate,
workload, weekly and state reports, dashboard, scenarios, proposals, actuals,
audit, fixture OpenProject import, and static overlay generation. `go test
./...` and `./scripts/smoke-mvp.sh` pass on a clean archive of `9fca426`.
Resource schedule drift on the dirty self-dogfood project is `aligned`.
Validation of `examples/replan-self` succeeds with only release-gate dependency
warnings.

The current working tree is not a committable green state. The uncommitted
re-entry task is the 55th self-dogfood task and has no sanitized OpenProject
`source_ref`. Overlay generation therefore reports `missing_link_count: 1`.
`TestDashboardOpenProjectOverlaySelfFixture` still requires
`WorkPackageCount == 54`, `LinkedTaskCount == 54` and `MissingLinkCount == 0`.
`scripts/smoke-mvp.sh` still requires `"mapped_tasks": 54` and `"ok": true` from
the fixture operator report. Both fail on the dirty tree and pass on HEAD.

The operator loop cannot currently prove live alignment. `./scripts/check-openproject-drift.sh`
returned `drift_status: failed` with `error_codes: ["openproject_network_error"]`.
The last verified live aligned state remains 2026-08-16. Two `in_progress`
tasks have planned finish dates already in the past relative to 2026-09-21.
One open proposal is stale against a dependency that is already `resolved`.
Two of four `required_dependency_unresolved` warnings are hygiene: their
predecessors are already `accepted`.

Public OSS release remains blocked by the 2026-08-02 boundary checkpoint:
missing license and contribution/security entry points, remaining private-pilot
narrative in public docs, old pilot-specific sidecars, and no chosen public
history strategy. No committed token values were found.

This is not an MVP regression. It is an unfinished post-pause re-entry plus
accumulated documentation and AI-CLI safety debt.

## Command evidence

| Check | Command | Result | Notes |
| --- | --- | --- | --- |
| HEAD identity | `git rev-parse --short HEAD` | `9fca426` | Matches `origin/main`. |
| Working tree | `git status --short` | 5 modified files | All re-entry related. |
| Tests on dirty tree | `go test ./...` | FAIL | `TestDashboardOpenProjectOverlaySelfFixture` in `internal/app`. |
| Tests on HEAD archive | `git archive HEAD` then `go test ./...` | pass | `cmd/replan`, `internal/app`, `internal/domain`, `internal/reports`, `internal/storage/filestore`. |
| Coverage | `go test ./... -cover -count=1` | mixed | `cmd/replan` 30.5%; `internal/domain` 100%; `internal/reports` 60.7%; `internal/storage/filestore` 40.6%; `internal/app` failed on dirty tree so no coverage number. |
| `go vet` | `go vet ./...` | pass | |
| `gofmt` | `gofmt -l .` | dirty | Only `internal/app/workload.go`, whitespace around `-(weekday-1)`. |
| TODO/FIXME in Go | `rg TODO\|FIXME\|XXX\|HACK --type go` | none | |
| Smoke on dirty tree | `./scripts/smoke-mvp.sh` | FAIL | First failure: `missing pattern '"ok": true' in .../self-openproject-operator-report.json`. Smoke later also asserts `"mapped_tasks": 54`. |
| Smoke on HEAD archive | `./scripts/smoke-mvp.sh` | `mvp smoke ok` | |
| Help | `go run ./cmd/replan --help` | usage printed | `actual` usage omits implemented `close-demand`. |
| Validate small-team | `go run ./cmd/replan validate examples/small-team --format json` | `ok: true`, 5 warnings, 0 errors | Fixture warnings, not blockers. |
| Validate replan-self dirty | `go run ./cmd/replan validate examples/replan-self --format json` | `ok: true`, 4 warnings, 0 errors | All four warnings are `required_dependency_unresolved` against `task.self.prepare-release-story`. |
| Resource schedule drift | `./scripts/check-resource-schedule-drift.sh` | `resource_schedule_status: aligned` | `hard_drifts: []`, `calibration_reviews: []`, `overloaded_weekly_rows: []`, `missing_capacity_weekly_rows: []`. |
| Workload small-team | `go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --format json` | `ok: true`, 8 resources, 0 overloads | Baseline fixture is intentionally not overloaded. |
| OpenProject file import | `go run ./cmd/replan import openproject --project examples/replan-self --from-file examples/imports/openproject/replan-self.work-packages.sample.json --dry-run --format json` | `ok: true`, 54 task candidates, 1 resource candidate, 1 warning | Sanitized fixture does not include the re-entry task. |
| Tracking suggest | `go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --format json` | `ok: true`, 0 suggestions | Remaining-work tracking loop is clean. |
| Report state | `go run ./cmd/replan report state --project examples/replan-self --week 2026-W31 --format json` | `ok: true` | Health and overlay counts recorded below. |
| Mutation on temp copy | `replan actual add-consumption` without `--dry-run` on `tmp/mvp-readiness-audit/cursor-2026-09-21/small-team` | `ok: true`, `applied: true`, `dry_run: false` | Confirms mutating default is write. |
| Validate after mutation | `replan validate tmp/mvp-readiness-audit/cursor-2026-09-21/small-team --format json` | `ok: true`, 0 errors | |
| Audit after mutation | `replan audit verify --project tmp/mvp-readiness-audit/cursor-2026-09-21/small-team --format json` | `valid: true` | |
| Live OpenProject drift | `./scripts/check-openproject-drift.sh` | `ok: false`, `drift_status: failed` | `error_codes: ["openproject_network_error"]`. Private binding path was reported `loaded: false` in the failure envelope even though the ignored bindings file exists; the failure path appears to emit the initial envelope. |
| GitHub Actions listing | `gh run list --repo kogriv/replan --branch main --limit 5` | HTTP 404 | CI workflow file exists. Local Actions visibility was not available. |
| TaskJuggler binary | `command -v tj3` | not found | Local TaskJuggler runtime evidence unavailable. CI job `taskjuggler` in `.github/workflows/ci.yml` installs TaskJuggler 3.8.4. |
| License / process files | `find . -maxdepth 2 ... license* copying* contributing* security* code_of_conduct*` | none | |
| Tracked private/env files | `git ls-files` filtered | only `.env.example` | `private/`, `tmp/`, `.env` and `.env.*` are gitignored except `.env.example`. |
| Secret-like patterns in tracked files | ripgrep for private domains, token assignments, `sk-`, `ghp_`, `glpat-`, AWS keys, private keys | only the OSS checkpoint's own search example and smoke dotenv fake token `OPENPROJECT_TOKEN=local-token` | No live token values observed in tracked files. |

## Dirty-tree test failure

Command:

```sh
go test ./...
```

Observed failure:

```text
--- FAIL: TestDashboardOpenProjectOverlaySelfFixture (0.12s)
    report_test.go:126: unexpected OpenProject overlay counts: ...
        WorkPackageCount:54
        LinkedTaskCount:54
        MissingLinkCount:1
        MissingLinks:[{
            TaskID:"task.self.reenter-openproject-dogfood-cycle-after-pause"
            Title:"Re-enter and rebaseline OpenProject-centered dogfood cycle after pause"
            TrackingStatus:"in_progress"
        }]
FAIL
FAIL    github.com/kogriv/replan/internal/app
```

The assertion in `internal/app/report_test.go` is:

```text
if overlay.WorkPackageCount != 54 || overlay.LinkedTaskCount != 54 || overlay.MissingLinkCount != 0 {
    t.Fatalf("unexpected OpenProject overlay counts: %#v", overlay)
}
```

The same test passes on a clean HEAD archive. The failure is caused by the
uncommitted re-entry task, not by a committed product regression.

`scripts/smoke-mvp.sh` still contains:

```text
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"ok": true'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"mapped_tasks": 54'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"task_bindings": 54'
assert_contains "$self_dashboard_project/reports/latest/raw/openproject_overlay.json" '"work_package_count": 54'
```

Step 43 already recorded that the fully bound 54-work-package smoke is not the
acceptance check for this partial re-entry state until the task is synced into
OpenProject and the public fixture is updated. That statement is still true on
2026-09-21. The dirty tree is therefore locally useful for planning and locally
red for CI.

## MVP criteria

These criteria come from `docs/spec/implementation-sequence.md` as evaluated by
`docs/spec/mvp-readiness.md` and `docs/spec/mvp-readiness-audit-plan.md`.

| Criterion | Status | Evidence | Notes |
| --- | --- | --- | --- |
| `examples/small-team` validates | done | `validate` `ok: true` | Five fixture warnings, zero errors. |
| `examples/replan-self` validates | done | `validate` `ok: true` | Four expected release-gate warnings on the dirty tree. Object counts are recorded below. |
| Workload report answers who or what is overloaded | done | small-team baseline has 0 overloads; README scenario `scenario.ivan-capacity-6` remains the documented overload demonstration; self-dogfood resource schedule drift reports 0 overloaded weekly rows | Role-capacity overload is not currently present in the calibrated self-dogfood plan. |
| Weekly Markdown and JSON reports are generated | done | HEAD smoke | HTML and CSV remain in the smoke path. |
| Scenario diff works | done | HEAD smoke | |
| Proposal list/show works | done | HEAD smoke; self-dogfood still has one open proposal | The remaining open proposal is stale; see dogfood hygiene. |
| Actual consumption can be recorded with audit | done | temp-copy `actual add-consumption` then `validate` and `audit verify` | Default is write, not dry-run. Actor was supplied. |
| OpenProject read-only import can create import snapshot or mapped candidates | done | file-mode dry-run against `replan-self.work-packages.sample.json` returned 54 task candidates | Live import was not available. |
| AI CLI agent can run validate/report/scenario/proposal commands safely | partial | stable JSON envelopes, exit codes `0/1/2/4/8`, `import accept` dual-gate, OpenProject helpers dry-run-by-default | Mutating `actual`, `proposal accept/reject` and `scenario apply` default to write. Global `--agent`, `--yes`, `REPLAN_ACTOR` are specified and not implemented. |
| No web UI, server, custom auth or full solver is required | done | entire audit ran headless; dashboard is generated static HTML; plugin is a read-only renderer | TaskJuggler is optional and locally absent. |

No new MVP blocker was found on HEAD.

## Additional readiness dimensions

| Dimension | Status | Evidence | Notes |
| --- | --- | --- | --- |
| CLI contract | partial | `docs/spec/cli.md` versus `cmd/replan/main.go` and `printUsage` | Implemented command family is usable. Spec still describes a wider unbuilt surface. Usage string omits `actual close-demand`. |
| Documentation usability | partial | README quickstart commands exist; `getting-started.md` implemented-CLI list is stale; several specs still say OpenProject write-back is forbidden while reviewed helpers exist | A new operator can run the fixture loop from README. A new operator cannot trust `getting-started.md` as the current command inventory. |
| AI CLI friendliness | partial | JSON envelopes, deterministic reports, explicit `--actor` on write paths that do write, OpenProject `REPLAN_APPLY=1` gate | Default write on several mutation commands is the main AI-safety gap. |
| Static dashboard | done on HEAD, partial on dirty tree | HEAD smoke generates dashboard artifacts; dirty overlay has one missing link | Overlay contract is still 54/54/0 in tests. |
| OpenProject fixture path | done | file-mode import works without live infrastructure | Public sample JSON has 54 work packages and does not mention the re-entry task. |
| OpenProject live path | blocked for this environment | `openproject_network_error` | Same failure class as the 2026-09-18 re-entry note. |
| TaskJuggler | deferred/optional | local `tj3` missing; CI workflow present | Not an MVP blocker. |
| CI | partial evidence | `.github/workflows/ci.yml` runs `go test ./...`, `./scripts/smoke-mvp.sh`, TaskJuggler 3.8.4 validate and run | `gh run list` returned HTTP 404 from this environment. Dirty-tree failures would fail CI if committed. |
| OSS boundary | blocked for public release | no `LICENSE`, `CONTRIBUTING.md` or `SECURITY.md`; private-pilot narrative still in public docs | Safe for private incubation. |
| Plugin safety | done for current scope | GET-only routes; overlay loader reads a JSON file; README states read-only intent | Native OpenProject Gantt modification remains deferred. |

## Self-dogfood observed state

Command:

```sh
go run ./cmd/replan validate examples/replan-self --format json
```

Object counts from the dirty tree:

| Object | Count |
| --- | --- |
| actors | 3 |
| actuals | 154 |
| audit_records | 269 |
| availability | 4 |
| baselines | 1 |
| capacities | 8 |
| dependencies | 79 |
| evidence | 55 |
| proposals | 28 |
| resource_demands | 177 |
| resource_pools | 2 |
| resources | 8 |
| scenarios | 2 |
| tasks | 55 |
| checked_files | 11 |

`report state` health for `examples/replan-self` week `2026-W31`:

| Field | Value |
| --- | --- |
| `task_count` | 55 |
| `accepted_tasks` | 52 |
| `active_tasks` | 2 |
| `open_dependencies` | 4 |
| `resolved_dependencies` | 75 |
| `overloaded_resources` | 0 |
| `warning_count` | 4 |
| `error_count` | 0 |
| `open_proposals` | 1 |
| `evidence_count` | 55 |
| `scenario_count` | 2 |
| `missing_demand_task_count` | 0 |
| `tasks_with_source_refs` | 54 |
| `tasks_without_source_refs` | 1 |
| overlay `work_package_count` | 54 |
| overlay `linked_task_count` | 54 |
| overlay `missing_link_count` | 1 |

The only task without a sanitized OpenProject source reference is
`task.self.reenter-openproject-dogfood-cycle-after-pause`.

Project metadata is stale relative to calendar date 2026-09-21:

- `examples/replan-self/.replan/project.yaml` `default_status_date: "2026-07-20"`
- `examples/replan-self/canonical/current_plan.yaml` `updated_at: "2026-07-20T12:00:00+07:00"`
  while `included_tasks` already contains the September 2026 re-entry task

Resource catalog in `examples/replan-self/canonical/resources.yaml`:

- `resource.human.maintainer`
- `resource.ai.codex.local`
- `resource.ai.review-assistant`
- `resource.role.reviewer`
- `resource.ci.github-actions`
- `resource.service.llm.tokens`
- `resource.environment.local-dev`
- `resource.window.oss-release`

Resource schedule drift on the dirty tree is `aligned`. Active re-entry
demands have explicit `remaining_amount` and did not create overload rows.

### Active tasks

`task.self.run-full-openproject-dogfood-cycle`:

- title: Run full OpenProject-centered dogfood cycle
- `tracking_status: in_progress`
- `planned_start: "2026-08-17"`
- `planned_finish: "2026-08-28"`
- planned finish is 24 days before the audit date
- sanitized source ref work package id `2123`
- this is the still-open full dogfood-cycle proof

`task.self.reenter-openproject-dogfood-cycle-after-pause`:

- title: Re-enter and rebaseline OpenProject-centered dogfood cycle after pause
- `tracking_status: in_progress`
- `planned_start: "2026-09-18"`
- `planned_finish: "2026-09-20"`
- planned finish is one day before the audit date
- tags include `openproject_sync_pending`
- no `source_refs`
- resource demands:
  - maintainer 2 hours, remaining 2, `active`
  - Codex 1.5 agent-hours, remaining 1.5, `active`
  - local-dev 1 concurrent slot, remaining 1, `active`
  - reviewer 0.5 review hours, remaining 0.5, `active`

`task.self.prepare-release-story`:

- `tracking_status: not_started`
- planned window `2026-11-09` to `2026-11-20`
- blocked by four required open dependencies listed below

### Validation warnings

All four `required_dependency_unresolved` warnings target
`task.self.prepare-release-story`.

Honest open gates, because the predecessor is not yet `accepted`:

| Dependency | Predecessor | Predecessor status | Successor | Dependency status |
| --- | --- | --- | --- | --- |
| `dependency.full-dogfood-before-release-story` | `task.self.run-full-openproject-dogfood-cycle` | `in_progress` | `task.self.prepare-release-story` | `open` |
| `dependency.dogfood-reentry-before-release-story` | `task.self.reenter-openproject-dogfood-cycle-after-pause` | `in_progress` | `task.self.prepare-release-story` | `open` |

Hygiene, because the predecessor is already `accepted` while the dependency
remains `open`:

| Dependency | Predecessor | Predecessor status | Successor | Dependency status |
| --- | --- | --- | --- | --- |
| `dependency.surface-prototype-before-release-story` | `task.self.prototype-openproject-native-surface-level-0-1` | `accepted` | `task.self.prepare-release-story` | `open` |
| `dependency.dashboard-semantics-before-release-story` | `task.self.reconcile-dashboard-load-semantics` | `accepted` | `task.self.prepare-release-story` | `open` |

`report state` next actions on the dirty tree were:

1. `resolve_dependency` `dependency.dashboard-semantics-before-release-story`
2. `resolve_dependency` `dependency.dogfood-reentry-before-release-story`
3. `resolve_dependency` `dependency.full-dogfood-before-release-story`
4. `resolve_dependency` `dependency.surface-prototype-before-release-story`
5. `review_proposal` `proposal.dependency.dependency.oss-audit-before-release-story.review-dependency-resolution`

The fifth next action is stale. The open proposal was created
`2026-07-30T14:53:02+07:00` by `actor.ai.codex.local`. It asks for review of
`dependency.oss-audit-before-release-story` while that dependency is already
`status: resolved` and predecessor
`task.self.write-oss-readiness-audit` is `accepted`. The proposal should be
rejected, superseded or otherwise closed so proposal state matches dependency
state.

`replan tracking suggest` returned zero suggestions. Stale remaining-work is
not the current hygiene problem. Stale dependency and proposal state is.

Plan versus task identity is consistent: every `included_tasks` id exists in
`tasks.yaml`, and the new re-entry task exists in `current_plan.yaml`,
`tasks.yaml`, `dependencies.yaml` and `resource_demands.yaml`. No orphan
demand or dependency references were found.

## OpenProject operator loop

Approved operating split remains:

- OpenProject owns task identity, human status, dates, relations, Gantt and
  ordinary collaboration.
- RePlan owns resource demands, capacities, AI/quota assumptions, scenarios,
  workload, actuals, evidence, proposals and audit.
- Broad automatic bidirectional synchronization remains out of scope.
- Narrow write-back exists only through reviewed helpers, dry-run by default,
  apply gated by explicit `REPLAN_APPLY=1`.

Live check from this environment:

```json
{
  "ok": false,
  "command": "check-openproject-drift",
  "drift_status": "failed",
  "project_path": "examples/replan-self",
  "private_binding_layer": {
    "path": "private/openproject/replan-self-bindings.yaml",
    "loaded": false
  },
  "summary": {
    "warning_codes": [],
    "error_codes": ["openproject_network_error"]
  },
  "temp_retained": false
}
```

Observed local ignored storage, without quoting contents:

- `.env.local` exists and is not tracked.
- `private/openproject/replan-self-bindings.yaml` exists, is gitignored, and is
  large enough to be a real binding file rather than an empty placeholder.
- The failure envelope reported `loaded: false`. That is a diagnostic quality
  issue on the failure path. It is not evidence that the bindings file is
  missing.

Last verified live aligned result, recorded in
[`../roadmap/self-dogfood-planning-calibration.md`](../roadmap/self-dogfood-planning-calibration.md)
Step 42 on 2026-08-16:

```text
check-openproject-drift:
  drift_status: aligned
  local_tasks: 54
  live_task_candidates: 54
  local_dependencies: 77
  live_dependency_candidates: 77
```

That result is 36 days old relative to 2026-09-21. It must not be treated as
current live alignment.

The public sanitized fixture
`examples/imports/openproject/replan-self.work-packages.sample.json` still
covers the 54-work-package HEAD shape. It does not contain the re-entry task.
`rg reenter-openproject-dogfood` over that fixture returns no matches.

OpenProject helper safety observed as still true:

- `scripts/apply-openproject-bootstrap.sh`
- `scripts/apply-openproject-relations.sh`
- `scripts/push-openproject-date-drift.sh`
- `scripts/push-openproject-status-drift.sh`
- `scripts/openproject-work-package-toolkit.sh`

These default to dry-run. Apply requires `REPLAN_APPLY=1`.
`scripts/lib/load-dotenv.sh` intentionally ignores `REPLAN_APPLY`, so a dotenv
file cannot enable live mutation. `scripts/smoke-mvp.sh` asserts that
behavior.

## Code quality and mutation safety

Line counts observed:

| File | Lines |
| --- | --- |
| `cmd/replan/main.go` | 4723 |
| `internal/app/import_openproject.go` | 1874 |
| `internal/app/import_accept.go` | 1616 |
| `internal/reports/html.go` | 1498 |
| `internal/app/report.go` | 1333 |
| `internal/app/import_openproject_test.go` | 1260 |
| `internal/app/import_accept_test.go` | 1230 |
| `internal/storage/filestore/store.go` | 1209 |
| `internal/app/actual.go` | 1028 |
| `internal/app/proposal.go` | 979 |

`internal/domain` has 100% statement coverage on the status predicates.
`cmd/replan` has about 30.5% statement coverage on the largest mutation
surface. `internal/reports` has about 60.7%. `internal/storage/filestore` has
about 40.6%.

### Mutation defaults

`replan actual add-consumption` defines:

```text
dryRun := fs.Bool("dry-run", false, "preview the change without writing files")
```

The temp-copy evidence confirmed `applied: true` and `dry_run: false` when
`--dry-run` was omitted. The same `false` default exists on the other
canonical-writing actual commands, `proposal accept`, `proposal reject` and
`scenario apply`.

`replan import accept` is stricter: it refuses unless `--dry-run` or `--apply`
is set, and `--apply` requires `--actor`.

`replan import openproject` also defaults `--dry-run` to false. Without
`--dry-run` it writes import snapshot files under `imports/openproject/`
without an actor and without an audit record. That is snapshot storage, not
canonical mutation, but it is still a write.

OpenProject operator scripts remain the safer pattern: dry-run default,
explicit apply gate, dotenv cannot set `REPLAN_APPLY`.

### Storage rewrite risk

`internal/storage/filestore/store.go` updates canonical YAML by reading the
whole file, unmarshalling, mutating, marshalling and writing atomically. This
preserves crash safety for a single file through temp plus rename. It does not
preserve comments, original key order or original quoting style. Hand-maintained
files such as `examples/replan-self/canonical/tasks.yaml` are therefore noisy
under command-driven edits.

Multi-file apply paths write canonical YAML, move proposal or scenario files,
then append audit. A mid-sequence failure can leave canonical state changed
while the proposal remains open, or leave duplicate proposal files if a move
fails after copy.

### Audit identifier collision residue

GAP-013 for proposal decision audit ids was closed on 2026-07-26 by including a
proposal-id slug. `scenario apply` audit ids were observed as
`audit.<timestamp>.scenario-apply` without the scenario id. Two scenario
applies in the same second can still collide. Actual consumption ids remain
second-precision.

### OpenProject helper diagnostics

Python OpenProject helpers can embed up to 1000 characters of HTTP response
body in `RuntimeError` JSON. That is not the API token. It can still leak
private work-package titles, descriptions or field values into operator JSON
and logs. Auth header construction is duplicated across several scripts
instead of living only in `scripts/lib/openproject_http.py`.

### Plugin read-only check

`integrations/openproject/plugins/openproject-replan_overlay`:

- routes are GET-only for the project overlay and one work-package overlay;
- the controller loads JSON through `OverlayLoader` and renders ERB;
- the loader reads a file path from `REPLAN_OPENPROJECT_OVERLAY_PATH` or a
  development fallback;
- the plugin README states that it does not calculate RePlan data, write
  OpenProject work packages, write RePlan files, accept actuals, create
  proposals or run synchronization jobs.

This remains consistent with the approved overlay strategy: RePlan generates
`reports/latest/raw/openproject_overlay.json`; OpenProject renders it.

## CLI specification versus implementation

Implemented command families from `cmd/replan/main.go` `printUsage`:

```text
validate
workload
report weekly
report state
report dashboard
tracking suggest
scenario list|show|diff|apply
proposal list|show|suggest-dependencies|accept|reject
dependency list
audit list|verify
actual list|add-consumption|set-remaining|add-status
import openproject
import accept
export json
export taskjuggler
export csv
taskjuggler run
taskjuggler import-result
dev validate-taskjuggler
```

Implemented but omitted from `printUsage`: `actual close-demand`, which exists
in the `runActual` switch.

Implemented OpenProject import flags not listed in the `cli.md` OpenProject
flag block include `--page-size` and `--request-timeout`.
`taskjuggler import-result --scenario-id` is in usage and not in that section
of the spec.

Documented in `docs/spec/cli.md` and not implemented as global CLI behavior:

- `--project` parent-directory discovery for `.replan/project.yaml`;
  implementation defaults an empty `--project` to `"."`
- global `--actor` with `REPLAN_ACTOR` and policy `default_actor` fallbacks
- global `--agent`, `--yes`, `--verbose`, `--quiet`, `--debug`
- exit codes 3, 5, 6, 7, 9 and 10; implementation defines 0, 1, 2, 4 and 8
- `validate --warnings-as-errors` and `--scope`; implementation has `--strict`
- workload time selectors `--from`/`--to`/`--status-date` besides `--week`
- workload filters `--resource-class`, `--unit`, `--task`, `--include-scenarios`

Documented command families that are not implemented in `main.go`:

- `scenario create`, `scenario patch`, `scenario calculate`,
  `scenario propose-apply`
- `proposal create`, `proposal supersede`
- `audit show`
- `report workload`, `report tracking`, `report warnings`, `report scenario`,
  `report proposals`
- `actual add` as a generic alias; implementation uses `add-consumption`
- `import github`, `import gitlab`, `import csv`
- first-class `replan writeback openproject ...`

`docs/spec/cli.md` still says "Source write-back is not MVP" and shows a future
`replan writeback` family. The current operator loop already has reviewed
helpers:

- `scripts/push-openproject-date-drift.sh`
- `scripts/push-openproject-status-drift.sh`

The accurate current rule is: broad or automatic write-back is out of scope;
narrow reviewed helpers exist and are dry-run by default.

`docs/dev/getting-started.md` "Current implemented CLI commands" still lists
only the early MVP set through export and then a "Not implemented yet" block.
It omits implemented `report state`, `report dashboard`, `tracking suggest`,
`dependency list`, `proposal suggest-dependencies`, `proposal accept`,
`proposal reject`, `scenario apply`, `actual close-demand`, `taskjuggler run`,
`taskjuggler import-result` and `dev validate-taskjuggler`. It also still names
`task.self.run-full-openproject-dogfood-cycle` as the current active block and
does not mention the re-entry task.

## Documentation drift

| Document | Drift | Recommended fix |
| --- | --- | --- |
| `docs/dev/getting-started.md` | Implemented-CLI inventory and current-block wording are stale. A historical private-pilot connection runbook remains in the same developer getting-started note. | Rebuild the implemented/not-implemented lists from `printUsage` and flag inventory. Point the current block at the re-entry task. Move private-pilot runbook material out of the public getting-started path. |
| `docs/spec/cli.md` | Global flags, parent discovery, extra workload filters, extra report subcommands and source write-back family are specified ahead of the binary. Write-back section does not point at existing helper scripts. | Annotate planned versus implemented, as GAP-011 already does for workload filters. Cross-link operator-loop helpers. |
| `docs/spec/mvp-readiness.md` | Last dated MVP audit is 2026-07-24. It does not record the 2026-09-21 project audit. | Keep 2026-07-24 as the MVP-readiness result. Add a pointer to this document. |
| `docs/spec/mvp-readiness-audit-plan.md` | Header status is still "planned audit procedure" after audits have already been run. | Mark as the executable procedure plus historical runs. |
| `docs/spec/taskjuggler-run-import-experiment.md` | Header still sounds like future design while run and import-result are implemented. | Mark implemented experiment, remaining limitations explicit. |
| `docs/spec/design-work-plan.md` | Still describes an active implementation-specification phase after those specs were approved. | Keep as historical closure map; stop calling it the next active phase. |
| `docs/README.md` and `README.md` | `functional-design-checklist.md` is described as an active checklist while its own header is approved. `openproject-live-mapping-quality.md` (moved to private storage on 2026-10-27) is still described as a planned audit while it is a long executed pilot journal. | Reword map entries. |
| `docs/spec/openproject-live-mapping-quality.md` and `docs/spec/openproject-pilot-log.md` (both moved to private storage on 2026-10-27), historical sections of `docs/dev/getting-started.md` | Private-pilot narrative, tunnel examples, private project and user labels. | Remaining OSS-boundary work, not an internal-MVP blocker. |
| `docs/spec/cli.md`, `docs/spec/mvp-acceptance.md`, `docs/spec/mvp-readiness.md`, `docs/spec/openproject-live-mapping-quality.md` (moved to private storage on 2026-10-27) | Some remaining "write-back forbidden" wording. | Replace with "broad/automatic write-back out of scope; reviewed helpers allowed". |
| `examples/replan-self/canonical/current_plan.yaml` | `updated_at` remains 2026-07-20 after later membership changes. | Refresh plan metadata when membership changes. |
| `examples/replan-self/.replan/project.yaml` | `default_status_date` remains 2026-07-20. | Rebaseline during re-entry. |

Pilot-narrative search on 2026-09-21:

```text
166 matches
145 matched lines
12 files contained matches
```

Highest-volume public files remain:

- `docs/spec/openproject-live-mapping-quality.md` (moved to private storage on
  2026-10-27)
- `docs/spec/openproject-pilot-log.md` (moved to private storage on 2026-10-27)
- `docs/dev/getting-started.md`
- `docs/roadmap/openproject-replan-sync-plan.md`
- `examples/openproject/sidecars/resource-links.replan-pilot.yaml` (moved to
  private storage on 2026-10-27)
- `examples/openproject/sidecars/resource-demands.replan-pilot.yaml` (moved to
  private storage on 2026-10-27)

## Known-gaps re-verification

| Gap | Register status | 2026-09-21 observation |
| --- | --- | --- |
| GAP-001 | closed | Report code no longer expects unreachable `done`/`on_hold` tracking statuses. |
| GAP-002 | closed | Audit shard path is no longer duplicated as two independent functions. |
| GAP-003 | closed | `internal/domain/status_test.go` exists; package coverage 100%. |
| GAP-004 | closed | Historical design-work-plan status sweep remains the closing record. Residual stale headers still exist in other files and are recorded above as documentation drift. |
| GAP-005 | open | Still true. `internal/reports/html.go` is 1498 lines. Package coverage about 60.7%. Tests are marker/assertion tests in `render_test.go`, not golden files for `index.html` / `gantt.html`. |
| GAP-006 | closed | Mutation evidence uses `--occurred-at` and `--status-date`. |
| GAP-007 | closed | README and docs README no longer describe `mvp-readiness.md` as merely planned. |
| GAP-008 | closed | Pilot journal lived in `openproject-pilot-log.md` (moved to private storage on 2026-10-27). |
| GAP-009 | closed | Mutation evidence uses real `examples/small-team` ids. |
| GAP-010 | partially closed | The private port and host alias were genericized. Remaining private-pilot narrative is still a public-release blocker. |
| GAP-011 | open | Still true. `runWorkload` parses `--week`, `--resource` and `--scenario` only. Spec still documents the extra filters as planned. |
| GAP-012 | accepted | Missing project root still exits `2` as validation failure. Spec documents the implemented rule. |
| GAP-013 license/process | open | Still true. No `LICENSE`, `CONTRIBUTING.md` or `SECURITY.md`. |
| GAP-013 proposal audit ids | closed | Closed 2026-07-26. The identifier `GAP-013` is reused for two different gaps in `known-gaps.md`. That register hygiene issue remains. |
| GAP-014 | closed | Workload now aggregates canonical `work_consumed` actuals. |

The duplicate `GAP-013` numbering should be corrected in a later hygiene pass
so license/process and audit-id collision are not the same identifier.

## OSS boundary

The 2026-08-02 checkpoint in
[`oss-boundary-checkpoint-2026-08-02.md`](oss-boundary-checkpoint-2026-08-02.md)
was re-checked, not replaced.

Current HEAD remains safe enough for private incubation and
OpenProject-centered dogfood.

Current HEAD is not ready to be published as the final public OSS repository.

Release blockers still open:

1. No `LICENSE` or `COPYING` file at repository root.
2. No `CONTRIBUTING.md` or `SECURITY.md`.
3. Private-pilot operational narrative remains in public docs, especially
   `docs/spec/openproject-pilot-log.md` and
   `docs/spec/openproject-live-mapping-quality.md` (both moved to private
   storage on 2026-10-27) and historical OpenProject
   sections in `docs/dev/getting-started.md`.
4. Old private-pilot sidecars remain tracked:
   `examples/openproject/sidecars/resource-demands.replan-pilot.yaml` and
   `examples/openproject/sidecars/resource-links.replan-pilot.yaml` (both moved
   to private storage on 2026-10-27).
5. Public history strategy is still not chosen.

No committed live token values, tracked `private/` files, tracked generated
private reports or tracked `.env` files were found. `.env.example` is the
expected public template.

`scripts/smoke-mvp.sh` writes a fake dotenv token `OPENPROJECT_TOKEN=local-token`
inside a temporary directory. That is test data, not a live secret.

## Blockers

| Finding | Severity | Evidence | Recommended next action |
| --- | --- | --- | --- |
| Dirty working tree breaks overlay test and MVP smoke | blocker for committing the current re-entry diff | `TestDashboardOpenProjectOverlaySelfFixture`; smoke `"mapped_tasks": 54` and `"ok": true` | Do not commit the re-entry task until the public fixture, example bindings and smoke/test overlay contract are updated, or keep the task local until live OpenProject sync assigns a sanitized source ref. |
| Live OpenProject unreachable | blocker for claiming current live alignment | `check-openproject-drift.sh` `openproject_network_error` | Keep `openproject_sync_pending`. Re-run operator report and drift when the live instance is reachable. Do not invent alignment. |
| Two `in_progress` tasks are past their planned finish | blocker for treating the current plan dates as current | `task.self.run-full-openproject-dogfood-cycle` finishes 2026-08-28; re-entry finishes 2026-09-20 | Rebaseline or split/close as part of the re-entry decision. |
| Public OSS packaging incomplete | blocker for public release only | no license/process files; pilot narrative; old sidecars; unchosen history strategy | Continue private incubation. Do not start `task.self.prepare-release-story`. |

No blocker was found that would reverse the 2026-07-24 conclusion that the
internal CLI-first MVP loop is usable on clean HEAD.

## Acceptable limitations

| Limitation | Reason it is acceptable for internal MVP |
| --- | --- |
| Local `tj3` is not installed | TaskJuggler is an optional experiment. The CLI degrades when the binary is missing. CI can still run the pinned gem. |
| GitHub Actions listing returned HTTP 404 from this environment | The workflow file exists. Local command evidence on HEAD is green. Dirty-tree CI failure mode is already proven by local smoke/tests. |
| Workload extra filters are unimplemented | GAP-011 is documented. `--week`, `--resource` and `--scenario` are enough for the MVP loop. |
| Dashboard HTML has no golden-file suite | GAP-005. Marker tests and smoke exist. The overlay contract is still moving with dogfood data. |
| Parent-directory project discovery is unimplemented | Commands take explicit `--project` or cwd. README examples already pass `--project`. |
| Exit codes 3, 5, 6, 7, 9, 10 are unused | Implemented subset 0, 1, 2, 4, 8 is enough for current commands. GAP-012 documents the missing-project choice. |
| No first-class `replan writeback` command | Narrow reviewed scripts exist. Broad write-back is out of scope. |
| Plugin does not modify native OpenProject Gantt | Explicitly deferred after simpler plugin surfaces. |
| `default_status_date` is 2026-07-20 | Historical fixture date. It should be rebaselined during re-entry, but it does not prevent validate/workload/report from running. |
| Live OpenProject is not required for the fixture MVP loop | File-mode import, sanitized sample JSON and self-dogfood YAML remain runnable offline. |

## Next stabilization slice

Recommended next slice:

Complete `task.self.reenter-openproject-dogfood-cycle-after-pause` as an
evidence-producing re-entry, not as a feature expansion.

Rationale:

The internal MVP is still usable on HEAD. The current risk is that the pause
left two active tasks with stale dates, one unsynced re-entry task that breaks
CI if committed as-is, a live OpenProject loop that cannot be verified from
this environment, and dogfood hygiene that validation already surfaces as
warnings and next actions.

Required work in that slice, in order:

1. Keep the current uncommitted re-entry data, but do not present it as
   CI-green. Either update the sanitized OpenProject fixture, example bindings,
   overlay test and smoke contract from 54/54/0 to the new 55-task shape, or
   delay the commit until live OpenProject sync produces a public-safe source
   reference.
2. When live OpenProject is reachable, run
   `scripts/openproject-operator-report.sh`, then
   `scripts/check-openproject-drift.sh`, then create/bind the missing
   re-entry work package through guarded helpers, then apply relations if
   needed, then refresh dashboard overlay artifacts. Until that succeeds, keep
   `openproject_sync_pending`.
3. Rebaseline dates for
   `task.self.run-full-openproject-dogfood-cycle` and
   `task.self.reenter-openproject-dogfood-cycle-after-pause`. Refresh
   `current_plan.updated_at` and consider `default_status_date`.
4. Close dogfood hygiene that does not require live OpenProject:
   reject or supersede
   `proposal.dependency.dependency.oss-audit-before-release-story.review-dependency-resolution`;
   mark
   `dependency.surface-prototype-before-release-story` and
   `dependency.dashboard-semantics-before-release-story` resolved, because
   their predecessors are already accepted.
5. Record an explicit decision for
   `task.self.run-full-openproject-dogfood-cycle`: keep, split, accept or
   replan. That decision is the purpose of the re-entry task. It must come
   from evidence after live or explicitly-pending OpenProject review, not from
   chat memory.

Not the next slice:

- public release story;
- license file as a substitute for re-entry;
- implementing unimplemented `cli.md` command families;
- web UI, daemon, RBAC, scheduler, or native OpenProject Gantt overlay;
- broadening OpenProject write-back.

After re-entry, the highest-value stabilization follow-ups are:

1. Make canonical-writing CLI commands dry-run by default, or require explicit
   `--apply`, matching `import accept` and the OpenProject helpers.
2. Rebuild `docs/dev/getting-started.md` and annotate `docs/spec/cli.md`
   planned-versus-implemented surfaces.
3. Keep GAP-005 and GAP-011 open until golden HTML tests and extra workload
   filters are actually wanted.

## What this audit did not change

The command audit itself did not modify source, fixtures, tests or
documentation.

This record file, the documentation-map entries that point to it, the
`mvp-readiness.md` pointer and the Step 43 execution note are the later
written evidence of the audit. They are not a substitute for live OpenProject
synchronization.

Temporary mutation evidence remains under gitignored
`tmp/mvp-readiness-audit/cursor-2026-09-21/`.

## Review addendum 2026-09-24

This section was added after a critical review of the audit against the
repository, the canonical data and GitHub. The audit text above is unchanged.
The full review is Step 55 of
[`../roadmap/self-dogfood-planning-calibration.md`](../roadmap/self-dogfood-planning-calibration.md).

Corrections:

- **CI was red, not an acceptable limitation.** `gh run list` returned 404
  from the audit environment, and the audit recorded CI as unverifiable and
  inferred it would be green from local runs. In fact every push to `main`
  from 2026-08-07 had failed in the MVP smoke, because the plugin scaffold
  check called `rg`, which the hosted runner lacks. Fixed on 2026-09-24 in
  `0572495`; recorded as GAP-017. The "internal CLI-first MVP on clean HEAD"
  verdict rested on local evidence only for those seven weeks;
- **the dirty-tree finding was superseded before the audit was committed.**
  The commit that added this document also moved the fixture, bindings, test
  and smoke from 54 to 60 work packages; Step 44 records that. The tree was
  never committed in the failing state;
- **`openproject_network_error` was a missing protected connection,** not an
  unreachable instance. A live deployment had used the same connection path
  three days later;
- **the audit's own resource use was not recorded** as actuals or evidence
  on the re-entry task it served;
- **the recommended first follow-up was reordered.** Dry-run-by-default is
  accepted as deferred (GAP-023). The observed mutation-safety defect was a
  command that wrote files and reported failure (GAP-015), fixed first in
  `ec3a8d6`.

Findings confirmed and since closed or planned: `close-demand` missing from
usage (fixed), `gofmt` drift (fixed), duplicated `GAP-013` (the license gap is
now GAP-025), `scenario apply` audit ids (fixed with GAP-015), store rewrite
risk (GAP-019 closed with a stated residual; GAP-022 open), helper auth and
error-body duplication (GAP-018, planned), stale `getting-started.md` and
`cli.md` status (fixed).
