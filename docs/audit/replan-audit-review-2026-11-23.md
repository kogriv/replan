# Review of the two audits of 2026-11-23

Status: dated review, 2026-11-23. No fixes were made in this pass.

Reviewed documents:

- `docs/audit/replan-full-audit-2026-11-23.md` (findings H1-H6, M1-M11,
  low tail), referred to below as the full audit.
- `docs/audit/replan-technical-audit-2026-11-23.md` (findings F-01-F-14),
  referred to below as the technical audit.

Both audits read commit `67caa2a`. Neither auditor could run Go. This review
re-checked every finding at `4607124` (the code areas involved did not change
in between) and, where possible, ran it: throwaway Go tests against the real
services, CLI runs on copies of `examples/small-team` and
`examples/replan-self`, local `http.server` / `httptest` pairs with fake
tokens, and two-process races. Scratch files were removed afterwards; no real
OpenProject or GitHub was called.

Verdicts: **correct** (code and conclusion hold), **partly correct** (the
conclusion holds but the description is incomplete or overstated), **wrong**
(the stated behaviour does not happen), **not a defect** (true but
harmless or deliberate).

## Summary

Most findings are real. The two audits overlap on five of them (H4 = F-10,
H5 = F-02, M2 = F-03, M6 = F-01, M7 = F-09). Two statements are wrong about
the code (H2b's "after apply" part, F-12's hotspot), two CI points are
overstated, and the runs found eight defects neither audit lists.

The most consequential confirmed defects:

1. Availability exceptions never reduce weekly capacity (H2a / F-05):
   40 h/week with an `unavailable` Wednesday still shows 40.
2. Status push overwrites aliases that drift reports as in sync (H3): a
   package in "In specification" with tracking `ready` gets no drift and a
   planned PATCH to "To be scheduled" under the default scope.
3. Secrets follow redirects: the Python OpenProject client forwards
   `Authorization` and the perimeter header to another host and over
   https -> http (H5 / F-02); the Go import client forwards the perimeter
   header on every redirect (not in either audit); GitHub paging sends the
   Bearer token to any `Link: rel="next"` host and has no page limit or
   cycle guard (H4 / F-10).
4. The private binding file loses concurrent updates (M2 / F-03), with a
   wide window in the relations helper.
5. General validation accepts negative, NaN and Inf capacity, unsupported
   periods, bad window times, bad exception dates and types, reversed task
   dates (F-07, F-08); NaN capacity crashes `workload --format json`.

## Verdicts

### Workload arithmetic

| Finding | Verdict | Evidence | Severity |
| --- | --- | --- | --- |
| H1a undated demand counts in full in every week | Correct | `spanShare` returns 1 (`internal/app/demand_spread.go:100`); one undated 8 h demand gives 8 h in W28 and 8 h in W29, no warning. `docs/spec/cli.md:965` calls this intended, which contradicts the approved `workload-calculation.md` (`unscheduled` bucket plus warning). | Medium |
| H1b unknown remaining counts as zero load | Correct, narrower | planned 60, remaining nil, capacity 40 gives overload 0 and no diagnostic. `demandHasWorkLeft` feeds `replan next`, not the dashboard load as the audit says. Go commands always write a remaining amount, so this needs hand-edited or imported YAML. | Low-medium |
| H2a / F-05 exceptions not in weekly capacity | Correct | `workingDays` / `capacityInWeek` (`internal/app/workload.go:363`) never read `Exceptions`; `unavailable`, `reduced_capacity`, `maintenance` all leave 40 of 40. TaskJuggler export does use four types. The spec requires exceptions from the start (`capacity-availability-rules.md:337`). | High |
| H2b scenario diff zeroes the whole week | Partly correct | The diff zeroes the resource for any week (`internal/app/scenario.go:395`, `899`). Wrong part: `scenario apply` refuses this change (`unsupported_scenario_apply_change`, `scenario_overlay.go:252`), so the exception never reaches availability. Real inconsistency: `workload --scenario` shows 32 for the same scenario while `scenario diff` shows 0. | Medium |
| M1a window hours do not affect capacity | Correct | a 1 h/day window still gives 40. | Low |
| M1b `Timing.Spread` not read | Correct | `contiguous`, `manual` and an invalid value give identical results and pass validation. | Low-medium |
| M1c explicit zero and absent capacity look the same | Correct | both give `missing_capacity`. | Low |
| M1d empty time zone falls back to process zone | Correct, understated | an unknown zone such as `Mars/Base` gets no diagnostic at all (`validate.go:205` checks only emptiness). | Low |
| M1e reversed preferred window swapped silently | Correct | plus: a malformed `preferred_start` is not validated, shadows a valid `earliest_start` and makes the demand undated. | Low |
| M1f W53 accepted for 52-week years | Correct | `2016-W53` and `2017-W01` map to the same week; `2025-W53` is accepted. | Low |
| M1g unparsable actual date intersects every week | Correct | `status_date: "July 6"` passes validation and appears in W40. | Low |
| F-06 actual fallback double counts in another week | Correct, latent | demand over W28-W29 with `actual_amount` 8 and one 8 h record in W28: W28 8, W29 4. No Go command writes `actual_amount`; in the self plan no demand hits the pattern today. Affects the actual column only. | Low |
| F-07 capacity and availability validation gaps | Correct, worse than described | `validate` stays ok for capacity -10, `.nan`, `.inf`, `period: day/month`, times `25:61`, bad exception date or type. NaN crashes `workload --format json` (exit 1). `period: day` 8 h counts as 8 h per week. Unknown weekday names narrow availability unless all are unknown, then all seven days apply. | Medium |
| F-08 reversed task dates swapped | Correct | passes validation, split across both weeks; baseline validation does flag it. | Low-medium |
| F-11 pool and member hide overload | Correct | 8 h direct + 8 h via a one-member pool against 10 h: both rows show no overload, no warning anywhere. | Medium |
| F-12 date-loop complexity | Wrong as a concern | 5,000 demands x 52 weeks: 0.42 s; the date loops are below profile cutoff. The real slowness is elsewhere (see new findings). | Low |

### Integrations and HTTP

| Finding | Verdict | Evidence | Severity |
| --- | --- | --- | --- |
| H3 drift and status push compare different names | Correct | fixture with "In specification": import maps to `ready`, drift reports 0, push plans `In specification -> To be scheduled` under `ready_only` (`push_openproject_status_drift.py:254`). Profile `status_map` is copied only. The status catalogue is one unpaged GET, likely harmless in practice. | Medium |
| H4 / F-10 GitHub `Link: rel=next` gets the Bearer token | Correct | a second host received the token; a self-referencing `Link` looped 4,510 times in 0.5 s with no page limit. | Low-medium |
| H5 / F-02 Python redirect forwards secrets | Correct | 301/302/303/307/308 to another host forwarded `Authorization` and `X-Perimeter-Key`; https -> http too (Python 3.12.3). | Medium-low |
| H5 remark on the Go client | Incomplete | Go strips `Authorization` only when the host name changes (same host, other port keeps it) and never strips `X-Perimeter-Key`; no `CheckRedirect` (`http_retry.go:79`). | Medium-low |
| M3 date push takes a fresh lockVersion | Partly correct | edits before apply are caught as `stale_live_dates`; exposure is only the gap between that check and the per-action GET. The PATCH carries only dates and scheduling flags. | Low |
| M4 Go paging stops early | Correct, plus one | relations stop on `offset*pageSize >= total` (2 of 5 read with a server capped at 2 per page); work packages also stop when `count` is missing; repeated pages not detected. Bites only if the server caps below 100. | Low |
| M6 / F-01 `javascript:` links | Correct | dashboard and Gantt emit `href="javascript:..."` from a SourceRef URL; validation does not check SourceRef URLs; overlay prints with `h()`. Import copies `_links.self.href` and `html_url` unchecked. | Low locally, medium if shared |
| M8 import snapshot without `--apply` or journal | Correct, one nuance | the three snapshot files are written separately, but under the write lock and each atomically. `cli.md` exempts `import openproject` and omits `import github`; the security doc's "nothing is written without `--apply`" is broader than the code. | Low |
| Low tail 3 Go diagnostics carry up to 4096 bytes of body | Correct | no token exposure (tokens travel only in headers). | Low |
| "Стык с OpenProject" table | Correct, one exception | `ignoreNonWorkingDays`, `/days/week` fallback and `--since` rows hold. "Closed packages are omitted without `--include-closed`" holds only without `--since`: `--since` sends its own filters, which likely replaces OpenProject's default open filter. Drift scripts always pass `--include-closed`. | Low |

### Storage, integrity, security

| Finding | Verdict | Evidence | Severity |
| --- | --- | --- | --- |
| M2 / F-03 binding file loses concurrent updates | Correct | two processes with a barrier between read and write: both exit 0, one binding remains, three runs out of three. No Python helper takes `.replan/write.lock`. The window is short in the toolkit and wide in `apply_openproject_relations.py` (loaded at start, written after all HTTP calls). | Medium |
| M5 audit verify checks only empty fields | Correct | a replaced audit file with an invented record verifies as valid. No test for the three codes. The audit file rewrite is under the write lock and journal, so the lost-line race does not occur in practice; the security doc's "append-only and verifiable" sentence overstates. | Low (doc) |
| M7 / F-09 readers see a half-applied commit | Correct | a commit cut after the first file: `workload` reports Ivan capacity 4, overload 4 (a state that never existed) with exit 0 and only the `interrupted_commit_pending` warning. The interrupted command's own audit record is replayed by recovery; only the recovery itself is unaudited. | Low-medium |
| F-04 symlink bypasses the output guard | Correct, threat overstated | a what-if dashboard through a symlink into the project was written; a symlinked `--project` path does the same. The guard also decides whether report commands take the lock. Only the operator or a same-account agent creates such links; worst case is a file that could get committed. | Low |
| M11 draft scenario can be applied | Correct, broader | a `draft` and a `rejected` scenario both apply; there is no `scenario accept`. Apply still needs a human actor and `--apply`. Specs disagree on `reviewed`. | Low-medium |
| F-13 actor id is only a name | Correct, documented | `access-and-actor-model.md:37` states it. | Low under the current trust model |
| M10 storage imports `internal/app` | Not a defect | adapter-to-port direction; app does not import filestore. | None |
| Low tail 7 actuals shard path built twice | Correct | byte-identical copies. | Trivial |
| Low tail 8 no stored source status | Correct | candidates carry it and the operator report compares it live; nothing stores it on the task. | Low |
| Baseline id as a path segment | Partly correct | `--id ../x` creates a stray file inside the project; leaving the project is refused at apply (`commit_failed`), though dry run shows the outside path. | Low |
| TaskJuggler comment newline | Correct | a demand id with a newline passes validation and injects a real `task` block into the exported `.tjp`. | Medium |
| "Что в коде держится" | Correct | five checks hold: apply-choice conflict and requirement, dry run beats `REPLAN_APPLY=1`, `approver_not_human` exit 3 on dry run, `CreateProjectFile` refuses replacement, sealed baseline tamper is detected. | n/a |

### Documents, CI, calibration

| Finding | Verdict | Evidence | Severity |
| --- | --- | --- | --- |
| Measurements | Correct | 302 tasks (301 accepted, 1 cancelled); README says about 150; October baseline numbers (38 tasks, 138 h, 35.27, 0.26) hold. | Low |
| H6 `cli.md` status table, known-gaps, getting-started | Correct | exit 3 and `import github` listed as missing; known-gaps header older than its last closures, no open entries; inventory misses three commands and `workload --raw`; top-level help misses `--levelled`. | Medium |
| M9 CI gaps | Partly correct | confirmed: no `-race`, coverage, govulncheck, Windows runner; release workflow runs no tests; 15 tests skip without `tj3`. Overstated: `GOOS=windows` build, vet and test compile pass; full `golangci-lint run ./...` reports 0 issues. Stale "CI is paused" in `.golangci.yml`, `getting-started.md`, the lint baseline doc; `release-process.md` says Actions does not build releases. Weekly HTML test checks five substrings, no golden. | Medium |
| Docs vs binary | Correct | eight spot checks match `--help` and exit codes. | Low |
| Calibration table and README citations | Correct | 0.21/32, 0.2125/39, 0.33/67 verify; README still guides with 0.26; kit README recommends 0.21. | Low-medium |
| F-14 calibration needs a backtest | Correct | every mechanism described matches `calibration.go`; no backtest or forecast-error measure exists. Survivorship bias is real in code, negligible in the self plan. Drift 0.21 -> 0.33 itself shows an old factor would not hold out of sample. | Medium |
| Stale headers (`openproject-replan-sync-plan.md`, `design-work-plan.md`) | Correct | both still read as active; the sync plan is required reading in `AGENTS.md`. | Low |

## Defects neither audit lists

1. The Go OpenProject import forwards `X-Perimeter-Key` on every redirect,
   and `Authorization` on same-host other-port or https -> http redirects.
2. NaN or Inf capacity crashes `replan workload --format json`.
3. A malformed `preferred_start` is not validated and makes the demand
   undated, so it counts in every week.
4. An unknown project time zone gets no diagnostic.
5. `replan workload --scenario` and `replan scenario diff` give different
   capacity for the same `unavailable` scenario (32 vs 0).
6. `replan report dashboard` takes about 22 s on `examples/replan-self`;
   about 60% of CPU is `CalibrationSuggestions` rebuilding effective status
   audits per task (`actual_window.go:25`).
7. A rejected scenario can be applied.
8. The dashboard's OpenProject link uses the API `self.href`
   (`/api/v3/work_packages/<id>`), not the work package page.

## Proposed work

One task per group of related findings. Estimates are proposals for the
maintainer to approve; nothing below is planned yet.

| # | Task | Findings | Codex h | Executor h | Review h | CI min |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| 1 | Availability exceptions in weekly capacity; one rule for diff, `workload --scenario` and export | H2a/F-05, H2b, new 5 | 1.5 | 0.4 | 0.5 | 15 |
| 2 | Undated demand and unknown remaining: `unscheduled` bucket and warnings; reconcile `cli.md` with the spec | H1a, H1b | 1.0 | 0.3 | 0.3 | 10 |
| 3 | Strict validation of capacity, availability, dates, time zone, ISO week, actual dates, spread mode and ids (TaskJuggler newline) | F-07, F-08, M1b-g, new 2-4, TaskJuggler newline, baseline id | 1.5 | 0.4 | 0.5 | 15 |
| 4 | One status map for import, drift and status push | H3 | 0.6 | 0.2 | 0.25 | 10 |
| 5 | Secret boundaries in HTTP clients: same-origin redirects in Python and Go, GitHub next-link origin check, page limit and cycle guard | H4/F-10, H5/F-02, new 1 | 1.0 | 0.3 | 0.3 | 10 |
| 6 | Safe URLs in the dashboard and overlay; link to the work package page | M6/F-01, new 8 | 0.6 | 0.2 | 0.25 | 10 |
| 7 | Inter-process lock for the binding file | M2/F-03 | 0.6 | 0.2 | 0.25 | 10 |
| 8 | Readers fail closed on a pending commit journal | M7/F-09 | 0.5 | 0.15 | 0.25 | 10 |
| 9 | Scenario status gate before apply; reconcile the scenario specs | M11, new 7 | 0.5 | 0.15 | 0.25 | 10 |
| 10 | Warn when a pool and its members share capacity | F-11 | 0.6 | 0.2 | 0.25 | 10 |
| 11 | Dated actuals switch off the `actual_amount` fallback in every week | F-06 | 0.4 | 0.1 | 0.2 | 10 |
| 12 | Go paging for relations, time entries and work packages | M4 | 0.4 | 0.1 | 0.2 | 10 |
| 13 | Output path guard resolves symlinks | F-04 | 0.3 | 0.1 | 0.15 | 10 |
| 14 | Dashboard speed: calibration suggestions on the self plan | new 6 | 0.8 | 0.2 | 0.25 | 10 |
| 15 | Docs in step with the binary: `cli.md` status, README numbers, getting-started, known-gaps register, CI and release statements, stale headers, audit and import wording | H6, M5 doc, M8 doc, M9 doc items, stale headers | - | 0.6 | 0.3 | 10 |
| 16 | CI: `-race`, `tj3` on the Go job, Windows compile check, govulncheck, tests before release build | M9 | 0.6 | 0.3 | 0.25 | 30 |
| 17 | Calibration backtest: out-of-sample error of baseline vs calibrated estimates | F-14 | 1.0 | 0.3 | 0.5 | 10 |

Not proposed as work: F-12 date loops (measured negligible), F-13 (documented
trust model), M10 (not a defect), M3 (narrow window, PATCH limited to dates),
the audit hash chain (the fix is the doc wording, in task 15), the duplicate
shard path and the stored source status (take with neighbouring work).

Suggested order: 1, 4, 5, 7, 3, 2, then the rest. 1 and 4 change what the
operator sees and does every week; 5 and 7 protect secrets and bindings; 3
and 2 stop wrong numbers from bad input.
