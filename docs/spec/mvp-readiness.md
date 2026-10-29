# RePlan MVP readiness audit

This document defines the RePlan MVP readiness audit and records its dated
results (audits 2026-07-16 and 2026-07-24). The dated operational pilot journal
that used to follow the audit results was split into
`openproject-pilot-log.md` on 2026-07-24 and moved to private storage on
2026-10-27. Current sanitized conclusions are in
[`../dev/openproject-pilot-lessons.md`](../dev/openproject-pilot-lessons.md).

It is intentionally an audit document, not a new feature specification.

The audit should prevent the project from drifting into daemon, server, UI or
enterprise features before the existing CLI-first MVP loop is assessed
honestly.

## Purpose

The readiness audit answers five questions:

1. Does the current implementation satisfy the documented MVP definition?
2. Which MVP criteria are done, partial, blocked or deferred?
3. Which remaining gaps are real MVP blockers?
4. Which gaps are acceptable known limitations for an internal MVP?
5. What is the next smallest stabilization step after the audit?

The audit should be updated by running commands against the repository, reading
the current documentation and checking actual behavior. It should not rely on
memory of earlier design discussions.

The executable audit procedure is defined in
[`mvp-readiness-audit-plan.md`](mvp-readiness-audit-plan.md). Use that plan
when two independent operators or AI CLI agents need to run the same audit and
compare findings.

## Inputs

Primary inputs:

- `docs/spec/implementation-sequence.md`;
- `docs/spec/mvp-acceptance.md`;
- `docs/spec/mvp-boundary.md`;
- `docs/spec/cli.md`;
- `docs/spec/report-behavior.md`;
- `docs/spec/planning-lifecycle.md`;
- `docs/spec/workload-calculation.md`;
- `docs/spec/scenario-model.md`;
- `docs/spec/evidence-proposal-behavior.md`;
- `docs/spec/pm-system-interaction.md`;
- `README.md`;
- current CLI implementation under `cmd/replan`;
- current application services under `internal/app`;
- current fixture under `examples/small-team`;
- current continuous integration workflow.

Required command evidence:

```bash
go test ./...
./scripts/smoke-mvp.sh
gh run list --repo kogriv/replan --branch main --limit 5
```

TaskJuggler evidence, when `tj3` or CI is available:

```bash
./scripts/validate-taskjuggler.sh
./scripts/run-taskjuggler.sh
```

The audit should record whether TaskJuggler evidence came from local execution
or from GitHub Actions.

## Status vocabulary

Use exactly these statuses:

```text
done
partial
blocked
deferred
not_started
```

Meanings:

- `done` means implemented, documented and covered by test or smoke evidence.
- `partial` means useful behavior exists, but at least one important MVP
  expectation is missing or weak.
- `blocked` means the MVP should not be called ready until the issue is fixed.
- `deferred` means explicitly out of first MVP scope.
- `not_started` means documented as desired but no meaningful implementation
  exists.

## MVP definition checklist

The audit must evaluate the current definition from
`docs/spec/implementation-sequence.md`.

| Criterion | Expected evidence | Status |
| --- | --- | --- |
| `examples/small-team` validates | `replan validate examples/small-team --format json`; smoke | done |
| Workload report answers who or what is overloaded | `replan workload`; weekly reports; scenario workload shows overload | done |
| Weekly Markdown and JSON reports are generated | `replan report weekly --format markdown`; `--format json`; smoke | done |
| Scenario diff works | `replan scenario diff`; smoke | done |
| Proposal list/show works | `replan proposal list`; `replan proposal show`; smoke | done |
| Actual consumption can be recorded with audit | `replan actual add-consumption` on temporary copy; audit verify; smoke | done |
| OpenProject read-only import can create import snapshot or mapped candidates | `replan import openproject`; import snapshot files; smoke | done |
| AI CLI agent can run validate/report/scenario/proposal commands safely | stable JSON envelopes, exit codes, dry-run, positional id ergonomics, smoke | done |
| No web UI, server, custom auth or full solver is required | implementation and CI do not require them | done |

First audit status is recorded below in `Audit 2026-07-16`.

## Additional readiness dimensions

The audit should also assess these cross-cutting areas because they affect
whether the MVP is usable by an operator or AI CLI agent.

### CLI contract

Check:

- commands return stable JSON envelopes where specified;
- `ok: false` corresponds to non-zero exit code;
- validation failures return validation exit code;
- unsupported formats return argument error exit code;
- help text reflects implemented commands;
- primary object ids can be passed before or after flags where documented;
- mutating commands support dry-run where required;
- mutating commands require actor where required.

Expected likely status before audit:

```text
partial
```

Reason: core behavior exists and smoke is passing, but a dedicated exit-code
and envelope audit may still reveal inconsistencies.

### Documentation usability

Check:

- README quickstart matches implemented commands;
- documentation map points to current specs;
- implementation sequence reflects implemented slices;
- MVP acceptance smoke is documented;
- known limitations are visible and not hidden in old design notes.

Expected likely status before audit:

```text
partial
```

Reason: documentation is extensive, but README quickstart may still show older
placeholder commands and should be audited against real CLI commands.

### Smoke and CI coverage

Check:

- `go test ./...` passes;
- `scripts/smoke-mvp.sh` passes locally;
- GitHub Actions runs `go test ./...`;
- GitHub Actions runs `scripts/smoke-mvp.sh`;
- TaskJuggler validation remains separate and passing in CI;
- smoke does not mutate committed fixtures.

Expected likely status before audit:

```text
done
```

Reason: current implementation has a fast MVP smoke and a separate TaskJuggler
CI job, but the audit should confirm current CI state.

### Source integration readiness

Check:

- OpenProject import from file works;
- OpenProject API path is implemented but does not require network for smoke;
- import snapshot write works on temporary project;
- import accept dry-run works;
- source write-back remains deferred.

Expected likely status before audit:

```text
partial
```

Reason: read-only import is useful, but live OpenProject setup and real
operator workflow still need separate operational validation.

### TaskJuggler readiness

Check:

- TaskJuggler export works;
- TaskJuggler runtime validation works when binary is available;
- TaskJuggler run captures artifacts;
- TaskJuggler import-result creates scenario/proposal candidates;
- proposal accept plus scenario apply can apply generated dates;
- TaskJuggler remains optional.

Expected likely status before audit:

```text
done
```

Reason: CI has dedicated TaskJuggler coverage, but the audit should record the
latest CI evidence and any local environment limitation.

### Internal MVP usability

Check:

- one operator can run the documented flow from a clean checkout;
- generated weekly report is understandable enough to inspect manually;
- static HTML report opens without server runtime;
- actual and scenario mutations are reversible through Git history and visible
  through audit;
- the project can be operated without OpenProject being installed locally;
- the project can be operated without TaskJuggler for the minimum loop.

Expected likely status before audit:

```text
partial
```

Reason: the technical path exists, but real operator use still needs a
step-by-step quickstart and a short operator validation pass.

## Expected blockers to look for

The audit should explicitly check whether these are blockers:

- README quickstart contains obsolete commands;
- help output omits implemented flags or formats;
- JSON envelope shapes differ unnecessarily across command families;
- exit codes do not match `docs/spec/cli.md`;
- mutating dry-run output is confusing or incomplete;
- actual write and scenario apply audit records are hard to inspect;
- OpenProject import examples are not discoverable;
- generated report paths are unclear;
- acceptance smoke is too slow or too brittle for regular CI;
- TaskJuggler job failures can block unrelated core work too often.

## Non-blockers for first internal MVP

The audit should not treat these as blockers unless a later decision changes
scope:

- web UI;
- desktop application;
- daemon;
- hosted service;
- custom authentication;
- full role-based access control;
- OpenProject write-back;
- automatic resource leveling;
- TaskJuggler as required runtime;
- full MS Project parity;
- full Jira/OpenProject replacement behavior;
- complex earned value;
- payroll;
- employee surveillance;
- full cost accounting;
- enterprise reporting.

## Audit output format

The audit result should be committed as an update to this file.

Use this structure:

```text
## Audit YYYY-MM-DD

Summary:

- MVP readiness: ready | almost_ready | not_ready
- Main blocker count: N
- Main limitation count: N
- Evidence date: YYYY-MM-DD

Checklist:

| Criterion | Status | Evidence | Notes |
| --- | --- | --- | --- |

Blockers:

1. ...

Known limitations:

1. ...

Recommended next slice:

1. ...
```

Readiness meanings:

- `ready` means no blocker remains for internal use.
- `almost_ready` means the core works but one or more small stabilization tasks
  should be done before calling it ready.
- `not_ready` means at least one core MVP criterion is blocked.

## Recommended next action

The next implementation step should be:

```text
Perform MVP readiness audit and update docs/spec/mvp-readiness.md with the first dated audit result.
```

The audit should happen before starting:

- daemon exploration;
- server API;
- richer UI;
- desktop app;
- hosted deployment;
- enterprise access control.

## Audit 2026-07-16

Summary:

- MVP readiness: `ready`
- Main blocker count: 0
- Main limitation count: 2
- Evidence date: 2026-07-16
- Evidence source: local commands plus GitHub Actions on branch `main`

Evidence:

```bash
go test ./...
./scripts/smoke-mvp.sh
gh run list --repo kogriv/replan --branch main --limit 5
go run ./cmd/replan --help
go run ./cmd/replan validate examples/small-team --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan scenario diff --project examples/small-team --format json scenario.ivan-unavailable-week-28
```

Evidence results:

- `go test ./...` passed locally.
- `./scripts/smoke-mvp.sh` passed locally.
- Latest GitHub Actions run on `main` passed:
  `docs: add mvp readiness audit plan`, run `29477307709`.
- GitHub Actions `Go` job passed `go test ./...` and `MVP smoke`.
- GitHub Actions `TaskJuggler validation` job passed.
- Local `./scripts/validate-taskjuggler.sh` skipped runtime validation because
  `tj3` was not installed locally, but returned success with
  `taskjuggler_binary_not_found` warning.
- Local `./scripts/run-taskjuggler.sh` was not used as readiness evidence
  because it requires a local `tj3` binary; CI is the TaskJuggler runtime
  evidence for this audit.

Checklist:

| Criterion | Status | Evidence | Notes |
| --- | --- | --- | --- |
| `examples/small-team` validates | done | `validate` command and MVP smoke passed | Fixture validates with no errors. Dependency planning warnings are expected and intentional in the fixture. |
| Workload report answers who or what is overloaded | done | `workload` and `report weekly` passed; scenario workloads show overloads | Current baseline weekly report has 0 overloaded rows. README quickstart now includes `scenario.ivan-capacity-6`, which shows Ivan overload of 2 hours without mutating the project. `scenario.openai-quota-300k` also shows OpenAI token quota overload. |
| Weekly Markdown and JSON reports are generated | done | `report weekly --format json`, `markdown`, `csv`, `html`; MVP smoke | HTML is also implemented and smoke-tested, though MVP done only requires Markdown and JSON. |
| Scenario diff works | done | `scenario diff` command and MVP smoke passed | Positional id works before and after flags. |
| Proposal list/show works | done | `proposal list`, `proposal show`, MVP smoke | Positional id works before and after flags for show. |
| Dependency lifecycle can be inspected and acted on | done | `dependency list`, `proposal suggest-dependencies`, dependency proposal accept/apply tests, MVP smoke | The implemented loop covers block successor, resolve dependency, unblock successor and read-only dependency overview. |
| Actual consumption can be recorded with audit | done | MVP smoke writes actual on temporary copy and verifies audit | Committed fixture is not mutated by smoke. |
| OpenProject read-only import can create import snapshot or mapped candidates | done | MVP smoke dry-runs import and writes snapshot on temporary copy | Uses committed sample JSON, no network required. |
| AI CLI agent can run validate/report/scenario/proposal/dependency commands safely | done | JSON envelopes and MVP smoke pass; positional id hardening implemented; not-found exit code aligned | Missing scenario, missing proposal, missing source profile and missing import snapshot now return exit code `4`. Dependency overview is read-only and included in smoke. |
| No web UI, server, custom auth or full solver is required | done | implementation, smoke and CI | CLI-first loop runs without daemon, server, browser, custom auth or required TaskJuggler runtime. |

Additional readiness dimensions:

| Dimension | Status | Evidence | Notes |
| --- | --- | --- | --- |
| CLI contract | done | help output, exit-code spot checks, smoke, CLI-level tests | Core MVP command behavior works, not-found exit code matches the documented contract and key MVP JSON envelopes are covered by CLI-level contract tests. |
| Documentation usability | done | README and docs map inspected, README quickstart updated after audit and operator validation | README documentation map is current. The obsolete quickstart blocker was closed after the first audit pass, and the follow-up operator pass removed fixture mutation from the read-only loop. |
| Smoke and CI coverage | done | local smoke and GitHub Actions | Fast smoke is in CI. TaskJuggler remains separate and passing in CI. |
| Source integration readiness | partial | OpenProject file import smoke | File-based read-only import and snapshot write work. Live OpenProject operational validation remains outside this audit. |
| TaskJuggler readiness | done | GitHub Actions TaskJuggler job | Optional engine path is covered in CI. Local machine lacks `tj3`, which is acceptable because TaskJuggler is not required runtime. |
| Internal MVP usability | done | smoke, reports, README inspection and README operator validation pass | Technical loop works. README now shows real commands, a visible overload scenario, dry-run mutation previews and a temporary-copy pattern for generated artifacts. |

Blockers:

None for internal CLI-first MVP use.

Closed blockers after first audit pass:

1. README quickstart was obsolete.
   It was replaced with implemented commands for tests, MVP smoke, validation,
   workload, reports, scenario diff, proposals, dry-run scenario apply, dry-run
   actual recording and dry-run OpenProject file import.
2. Exit-code contract was not fully aligned with `docs/spec/cli.md`.
   Missing scenario, missing proposal, missing source profile and missing import
   snapshot now return exit code `4`; unsupported format remains `8`; ordinary
   validation errors remain `2`.

Known limitations:

1. Live OpenProject API usage is implemented but not validated in this audit.
   The smoke uses committed sample JSON to avoid network and credentials.
2. Local TaskJuggler runtime was not available during this audit.
   TaskJuggler evidence comes from CI, which installs pinned TaskJuggler 3.8.4.

Closed limitations after first audit pass:

1. JSON envelope shapes were useful but not covered by dedicated contract tests
   across key commands.
   CLI-level JSON envelope contract tests now cover `validate`, `workload`,
   `report weekly`, `scenario diff`, `proposal list`,
   `actual add-consumption --dry-run` and `import openproject --dry-run`.
2. Default baseline weekly report shows zero overloaded rows, so the first
   quickstart path did not visibly demonstrate the planning value.
   README now explicitly says the baseline has no overload and includes
   `workload --scenario scenario.ivan-capacity-6`, which shows a 2 hour Ivan
   overload in memory without changing the project directory.

Deferred, not blockers:

- web UI;
- desktop application;
- daemon;
- hosted service;
- custom authentication;
- full role-based access control;
- OpenProject write-back;
- automatic resource leveling;
- TaskJuggler as required runtime;
- full MS Project parity;
- full Jira/OpenProject replacement behavior;
- enterprise reporting.

Recommended next slice:

1. Create a private OpenProject pilot project through a protected connection,
   create an API token and run `scripts/validate-openproject.sh` against the
   live API.
   If OpenProject proves awkward, run the first real internal project pilot
   from local YAML data instead.
2. Decide whether full snapshot-style golden files are needed after the first
   real operator use on non-fixture data.

## Operator validation 2026-07-16

Summary:

- Validation result: `passed_with_documentation_fixes`
- Evidence date: 2026-07-16
- Evidence source: local hands-on README quickstart execution
- Project fixture: `examples/small-team`
- Mutating artifact writes: executed only on a temporary copy

Evidence:

```bash
go test ./...
./scripts/smoke-mvp.sh
go run ./cmd/replan validate examples/small-team --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --scenario scenario.ivan-capacity-6 --format json
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format markdown
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format html
go run ./cmd/replan scenario diff --project examples/small-team --format json scenario.ivan-unavailable-week-28
go run ./cmd/replan proposal list --project examples/small-team --status open --format json
go run ./cmd/replan proposal show --project examples/small-team --format json proposal.adjust-remaining-task-op-1234.20260706
go run ./cmd/replan scenario apply --project examples/small-team --dry-run --format json scenario.task-op-1234-remaining-5
go run ./cmd/replan actual add-consumption --project examples/small-team --task task.op.1234 --demand resource_demand.task-op-1234.implementer --resource resource.human.ivan --amount 1 --unit hours --actor actor.human.operator --occurred-at 2026-07-08T10:00:00+07:00 --status-date 2026-07-08 --dry-run --format json
go run ./cmd/replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
```

Temporary-copy artifact write evidence:

```bash
tmp_project=$(mktemp -d)
cp -a examples/small-team "$tmp_project/small-team"
go run ./cmd/replan report weekly --project "$tmp_project/small-team" --week 2026-W28 --format html --write
test -s "$tmp_project/small-team/reports/weekly/2026-W28.html"
```

Findings:

1. The operator quickstart was runnable from a clean checkout.
   Validation, workload, Markdown report, HTML report to standard output,
   scenario diff, proposal list, proposal show and dry-run mutation previews all
   worked without modifying the committed fixture.
2. The old README read-only loop contained
   `report weekly --format html --write`.
   That command writes `reports/weekly/2026-W28.html` into the project, so it
   was not actually read-only. README now uses HTML to standard output in the
   read-only loop and documents `--write` only on a temporary copy.
3. The baseline fixture intentionally has no overload.
   That is acceptable as fixture data, but it makes the first workload command
   look less useful. README now includes
   `workload --scenario scenario.ivan-capacity-6`, which demonstrates a 2 hour
   Ivan overload without changing source files.
4. Dry-run mutation previews are operator-friendly enough for the internal MVP.
   They return structured JSON envelopes and do not require a running server,
   OpenProject instance, TaskJuggler runtime or web UI.

Result:

- internal CLI-first MVP remains `ready`;
- operator validation does not add a new blocker;
- documentation usability moves from `partial` to `done` for the fixture
  quickstart;
- remaining limitations are live OpenProject validation and local TaskJuggler
  runtime availability.

## OpenProject pilot and OSS-direction log

The dated operational journal for the private OpenProject pilot and the
2026-07-20 OSS-direction decisions was moved on 2026-07-24 to
`openproject-pilot-log.md` (moved to private storage on 2026-10-27), so this
file keeps only the readiness audit definition and dated audit results. See
[`../dev/known-gaps.md`](../dev/known-gaps.md) GAP-008.

## Audit 2026-07-24

Summary:

- Consolidated decision: `usable with known limitations` / ready for internal
  dogfood use
- Auditor verdicts: Claude auditor-a `usable_internal_mvp`; Claude auditor-b
  `usable_internal_mvp`; opencode-glm `usable_with_limitations`
- Main blocker count: 0 (all three auditors independently found none)
- Main limitation count: 5
- Evidence date: 2026-07-24
- Evidence source: three independent parallel audits per
  [`mvp-readiness-audit-plan.md`](mvp-readiness-audit-plan.md)
  (auditor-a, auditor-b, opencode-glm) at commit `ac765a8`, local command
  evidence plus GitHub Actions run `30095100343` on `main`

Procedure:

- all three auditors independently ran the full required command evidence,
  mutation evidence on temporary copies under `tmp/mvp-readiness-audit/`,
  OpenProject fixture evidence, dashboard evidence and documentation checks;
- local `tj3` was unavailable for all three; TaskJuggler runtime evidence comes
  from the green CI TaskJuggler job (pinned 3.8.4);
- the comparison protocol was applied. All three verdicts agree there is no
  reproducible blocker. The verdict divergence is not about readiness: two
  auditors called the loop `usable_internal_mvp`, while opencode-glm withheld
  the clean label as `usable_with_limitations` because it weighted the
  documentation drift (notably private OpenProject endpoint detail leaking into
  public specs) as a limitation that must be visible before calling the MVP
  ready without qualification. Because the plan forbids averaging verdicts and
  no auditor found a blocker, the consolidated decision keeps both facts: the
  loop is usable for internal dogfood now, with the known limitations below
  visible;
- the conflicting dimension grades (CLI contract, documentation usability) were
  resolved by short reproduction commands and the stricter grade was kept.

Checklist (merged, both auditors agree):

| Criterion | Status | Evidence | Notes |
| --- | --- | --- | --- |
| `examples/small-team` validates | done | `validate` exit 0, smoke, CI | `examples/replan-self` also validates (19 tasks, 56 demands). |
| Workload report answers who or what is overloaded | done | baseline plus `--scenario scenario.ivan-capacity-6`; replan-self W31 shows 3 overloaded resources | Role-capacity overload (`resource.role.reviewer`) is visible. |
| Weekly Markdown and JSON reports are generated | done | `report weekly` md/json; byte-identical across repeated runs | HTML and CSV also pass in smoke. |
| Scenario diff works | done | `scenario diff --against current` with before/after workload | Positional id works before and after flags. |
| Proposal list/show works | done | list/show plus full open-to-accepted lifecycle on temporary copy | File move and audit append verified. |
| Actual consumption can be recorded with audit | done | add-consumption on temporary copy; `audit verify` valid; records 1 to 2 | Plan example ids do not exist in the fixture; both auditors substituted the same real ids (`task.op.1234` family). |
| OpenProject read-only import can create import snapshot or mapped candidates | done | file dry-run, snapshot write (`manifest.yaml`, `raw.json`, `mapped.json`), `import accept --dry-run` | Offline, committed sample JSON only. |
| AI CLI agent can run validate/report/scenario/proposal commands safely | done | stable envelopes, deterministic output, dry-run and actor enforcement, exit codes 0/2/4/8 on the built binary | Contract edge drifts recorded below as limitations. |
| No web UI, server, custom auth or full solver is required | done | entire audit ran headless; dashboard is static HTML | TaskJuggler wrapper degrades gracefully without `tj3`. |

Additional readiness dimensions (merged):

| Dimension | Status | Notes |
| --- | --- | --- |
| CLI contract | partial | Reproduced drifts: malformed `--week banana` is silently accepted (`ok:true`, exit 0, echoed week); missing project path exits `2` via `missing_required_file` while `cli.md` documents `4` for missing project; `go run` collapses documented exit codes to `1` (built binary returns 2/4/8 correctly); `workload` help omits implemented `--resource`/`--resource-class`; dashboard `--output` help wording reads like a default. |
| Documentation usability | partial | Quickstarts reproduce verbatim and pass; remaining drift is status and boundary hygiene: `mvp-boundary.md` draft, `design-work-plan.md` stale markers, this file mixing audit results with a pilot journal (GAP-004, GAP-008 in `docs/dev/known-gaps.md`); private OpenProject operational detail and the `OPENPROJECT_TOKEN` placeholder appeared in eight committed public files — `README.md`, `docs/README.md` was clean, while `docs/spec/openproject-live-mapping-quality.md` (moved to private storage on 2026-10-27), `docs/spec/openproject-operator-loop.md`, `docs/spec/implementation-sequence.md`, `docs/spec/storage-layout.md`, `docs/spec/cli.md`, `docs/dev/getting-started.md` and this file carried it — which conflicted with `docs/dev/repository-boundary-contract.md`. opencode-glm weighted this leak as the main reason for its `usable_with_limitations` verdict. |
| Smoke and CI coverage | done | Local test plus smoke pass; CI run `30095100343` green on HEAD for both jobs. |
| Source integration readiness | partial | File-mode loop fully proven offline; live OpenProject evidence remains private-instance journal entries, not publicly reproducible (optional per plan). |
| TaskJuggler readiness | done | CI job evidence; graceful local skip with warning. |
| Internal MVP usability | done | Operator/AI next-action commands are generated in three surfaces (state report, tracking suggest, dashboard); mutations are previewable, audited and Git-reversible. |

Blockers:

None. Both auditors independently found no reproducible blocker.

Known limitations:

1. Malformed or missing `--week` values are silently accepted by week-scoped
   commands; a typo yields a plausible-looking report.
2. Missing project path exits `2` instead of the documented `4`
   (`docs/spec/cli.md`); `ok:false` and non-zero exit still hold.
3. Documented exit codes are only observable on a built binary; `go run`
   collapses them to `1`, and README examples use `go run`.
4. Local TaskJuggler runtime evidence unavailable (`tj3` not installed);
   evidence source is CI.
5. Live OpenProject validation is not publicly reproducible (private
   instance and token); optional per the audit plan.

Recommended next slice:

1. One CLI-contract stabilization slice: reject malformed `--week` with exit
   `8` across `workload`, `report weekly`, `report state`, `report dashboard`
   and `tracking suggest`; align the missing-project exit code with
   `docs/spec/cli.md` (pick `4` in code or amend the spec); add the
   `workload` filter flags to help output; clarify dashboard `--output` help
   wording; document that exit-code-sensitive automation should use a built
   binary.
2. Then a documentation and OSS-boundary slice: sweep
   `design-work-plan.md`/`mvp-boundary.md` statuses (GAP-004), split the pilot
   journal out of this file (GAP-008), and move private OpenProject
   operational endpoint detail out of the eight public files that carry it
   (GAP-010), all coordinated with `docs/dev/oss-separation-plan.md` and
   `docs/dev/repository-boundary-contract.md`.

### Follow-up 2026-07-24 (post-audit stabilization)

The two recommended slices above were executed on 2026-07-24, on branch
`stabilization/mvp-audit-followup`:

- Slice 1 (CLI contract): malformed `--week` now returns exit `8` with an
  `invalid_week` diagnostic across the week-scoped commands; the `workload`
  help line lists `--resource` and the dashboard `--output` wording is fixed;
  the missing-project exit code was resolved by documenting the implemented
  rule in `cli.md` (validation failure, exit `2`; see GAP-012). Code-hygiene
  gaps GAP-001, GAP-002 and GAP-003 were also closed.
- Slice 2 (documentation and OSS boundary): `design-work-plan.md` and
  `mvp-boundary.md` statuses were swept (GAP-004), the pilot journal was moved
  to `openproject-pilot-log.md` (moved to private storage on 2026-10-27;
  GAP-008), and the private endpoint port/host
  alias were genericized across the public docs (GAP-010).

Gap statuses and closing details are tracked in
[`../dev/known-gaps.md`](../dev/known-gaps.md). The doc-usability and
CLI-contract dimensions above record the state observed during the audit, not
after these fixes.

## Audit 2026-09-21

This dated result is a full-repository project audit, not a replacement of the
2026-07-16 or 2026-07-24 internal MVP readiness audits.

The complete evidence, command log, dirty-tree versus HEAD split, dogfood
hygiene, live OpenProject failure, CLI/spec drift, known-gaps re-verification,
OSS-boundary re-check and recommended next slice are recorded in
[`../dev/project-audit-2026-09-21.md`](../dev/project-audit-2026-09-21.md).

Summary:

- Commit: `9fca426`
- Evidence date: 2026-09-21
- Internal CLI-first MVP on clean HEAD: `usable_internal_mvp`
- Dirty working tree with uncommitted re-entry task: tests and MVP smoke fail
  (superseded before commit; see the review addendum in the audit record)
- Self-dogfood / operator loop: `usable_with_limitations`
- Public OSS release: `not_ready`
- Main MVP blocker count: 0 on HEAD
- Live OpenProject: `openproject_network_error`; last verified aligned live
  state remains 2026-08-16
- Recommended next slice: complete
  `task.self.reenter-openproject-dogfood-cycle-after-pause` before any feature
  expansion or public release work

Review addendum, 2026-09-24: the audit missed that CI on `main` had been red
since 2026-08-07 (GAP-017, fixed in `0572495`). Its HEAD verdict rested on
local runs only.

The 2026-07-24 consolidated MVP decision is not reversed. The new findings are
about post-pause dogfood re-entry, CI contract of the uncommitted 55th task,
live OpenProject reachability and accumulated documentation/AI-CLI safety
debt.

## Done criteria for this documentation slice

This documentation slice is done when:

- `docs/spec/mvp-readiness.md` exists;
- README and docs README link to it;
- `docs/spec/implementation-sequence.md` lists MVP readiness audit as the next
  post-acceptance step;
- the document defines status vocabulary, evidence commands, checklist,
  likely blockers, non-blockers and expected output format;
- no implementation code is changed by this slice.
