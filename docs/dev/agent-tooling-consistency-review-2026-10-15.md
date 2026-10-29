# Agent tooling consistency review 2026-10-15

Status: accepted by the operator on 2026-10-15. Every finding, and every gap
still open or accepted in `known-gaps.md`, is a task in the self-dogfood plan;
see [Backlog](#backlog).

Task: `task.self.review-agent-tooling-consistency`.

## Question

How consistent is the RePlan and OpenProject tooling for CLI AI agents? We
asked whether it is connected, uniform, understandable, DRY and predictable.

The tooling covers:

- the Go CLI;
- `scripts/*.sh` and their Python helpers in `scripts/lib`;
- the agent kit;
- the process docs.

We judge it from the seat of a fresh agent that has only the repository, or
only the agent kit, and must run the loop safely: task and work package, code,
evidence and actuals, status and dates to OpenProject, overlay.

## Method

A review agent read the code and the docs read-only. It ran only `--help` and
other local commands, with no live calls. The executor then rechecked the
high-severity findings against the code:

- C-01: the missing-token branch of `push-openproject-status-drift.sh`;
- C-02: `ok` in `check_openproject_drift.py` and `push_openproject_status_drift.py`;
- C-05: the scripts that do not source `load-dotenv.sh`;
- C-10: the relations pagination and the bootstrap bindings write.

All of these held.

## Verdict

Each tool is solid on its own:

- the write helpers default to a dry run;
- tokens and the perimeter key are never printed;
- OpenProject error bodies are cut down before they reach the output;
- the agent kit is built from the repository's own files, not from a copy.

As one system for a fresh agent, it is not unified yet:

- some loop steps have no tracked command and are glued by private scripts;
- the task to work package mapping and the status mapping each live in
  several places;
- some helpers report success when they did nothing;
- exit codes and the JSON envelope follow `docs/spec/cli.md` only in part;
- the kit's defaults point at this repository's dogfood project.

| Dimension | Score (1–5) |
| --- | --- |
| Surface map: every loop step has one command | 2 |
| Interface uniformity: gates, flags, envelope, exit codes | 2 |
| DRY | 2 |
| Single source of truth | 2 |
| Understandability | 2 |
| Safety for agents | 3 |
| Agent kit vs repository tooling | 3 |

## Surface map

| Loop step | Tracked command | Gap or overlap |
| --- | --- | --- |
| Create a RePlan task | none: edit `canonical/tasks.yaml` by hand | **Gap**. Also every new task means changing hard-coded counts in tests |
| Create or bind a work package | `apply-openproject-bootstrap.sh`, toolkit `work-package create` / `bind`, toolkit `backlog reconcile` | **Overlap**: three writers of the bindings file |
| Relations | `apply-openproject-relations.sh`, toolkit `relation list` / `delete` | |
| Plan and validate | `replan validate` / `next` / `workload` / `report`, `check-resource-schedule-drift.sh`, `explain-resource-overload.sh`, `review-resource-pool.sh`, `plan-resource-demand-calibration.sh` | |
| Record evidence | none: `actual` commands need an existing `--evidence` id, and there is no `evidence add` | **Gap** |
| Actuals and acceptance | `replan actual add-consumption` / `set-remaining` / `close-demand` / `add-status` | **Gap**: acceptance chains them by hand |
| Status and dates to OpenProject | `push-openproject-status-drift.sh`, `push-openproject-date-drift.sh`, toolkit `work-package update-status` / `update-dates` | **Overlap**: the drift pushes are the primary batch surfaces; the toolkit updates one work package |
| OpenProject status into RePlan | `replan import accept <snapshot-id> --accept-tracking-status` | Requires a stored snapshot and explicit dry-run/apply choice |
| OpenProject dates into RePlan | `accept-openproject-soft-drift.sh` | Dates only; status differences are review-only, and `replan import accept` has no date mode |
| Overlay | `build-openproject-live-overlay.sh`, then `publish-openproject-overlay.sh` | Not in the kit |

## Findings

Findings are ordered by severity. **VERIFIED** means the code was checked.
**INFERRED** means the finding follows from the code but was not run.

### High

**C-01. A missing token gives `ok: true` and exit 0, even under `REPLAN_APPLY=1`.** VERIFIED.

- These six scripts print `"mode":"skipped","reason":"token_env_not_set"` and
  exit 0 without looking at the apply gate:
  - `push-openproject-status-drift.sh:44-48`
  - `push-openproject-date-drift.sh:46-48`
  - `accept-openproject-soft-drift.sh:48-50`
  - `apply-openproject-bootstrap.sh:33-34`
  - `apply-openproject-relations.sh:29-30`
  - `check-openproject-drift.sh:49`
- The kit README (`docs/agent-kit/README.md:228`) says the code is among
  `warning_codes`. It is not.
- **Risk:** an agent that chains an apply, then the overlay publish, then its
  report takes "nothing pushed" for success.
- **Fix:** under apply, a missing token is `ok: false` with a non-zero exit. In
  a dry run, keep `skipped` and add the code to `warning_codes`.
- **Resolved, 2026-10-15.**
  - `replan_token_missing` in `envelope.sh` handles the five helpers that
    write: under `REPLAN_APPLY=1` a missing token is `ok: false`,
    `token_env_not_set`, exit 8, and nothing is written.
  - Otherwise, and for the helpers that only read through
    `replan_token_skip`, the run is skipped with the code among its
    warnings, as the kit README says.
  - The toolkit's writing commands do the same.
  - Tests cover every one of them with and without the apply gate.

**C-02. `ok: false` with exit 0.** VERIFIED.

- `check_openproject_drift.py:245` and `check_resource_schedule_drift.py:265`
  set `ok` from the drift status but always exit 0.
- `push_openproject_status_drift.py:174` sets `ok: false` when a transition
  was skipped, and also exits 0.
- `cli.md:419` says these must agree.
- **Risk:** `set -e` and `&&` chains go straight through a drift.
- **Fix:** one shared emit function that derives the exit code from `ok`, with
  a documented code for drift.
- **Resolved, 2026-10-15.** `helper_envelope.emit` derives the code from
  `ok`:
  - `0` for a clean run or a soft drift;
  - `11`, a new code "attention needed" for helper scripts, when the run
    ended with `ok: false`;
  - `7` for an apply that wrote some actions and not others.

  Tests cover a clean run, a soft drift and a hard drift of the drift check,
  and, against a stand-in OpenProject, a status move the workflow refuses,
  both in a dry run and in a partial apply.

**C-03. Loop steps without a tracked command, and test counts tied to the dogfood data.** VERIFIED.

- There is no `replan task` or `replan evidence` family.
- `actual` commands fail with `evidence_not_found` until an evidence record
  exists (`internal/app/actual.go:807`).
- The backlog convention lists eight steps run by hand
  (`backlog-operating-convention.md:166-185`).
- The task count (now 109) is hard-coded in `internal/app/report_test.go:127`
  and in eight lines of `scripts/smoke-mvp.sh`.
- **Risk:** a fresh agent cannot add or accept a task without editing YAML and
  test constants. The operator's private glue scripts exist because of this.
- **Fix:**
  - `replan task add` and `replan evidence add`;
  - a tracked composite `scripts/accept-task.sh` that prints its plan in a dry
    run and can resume after a failed step;
  - counts derived from the fixture instead of literals.

**Resolved (evidence part), 2026-10-26.** `replan evidence add` now appends a
validated evidence item to its observed ISO-week shard through the splice
store, records the audited write, and uses the shared dry-run/apply gate. The
`task add`, composite acceptance-chain and fixture-derived-count parts remain.

**Resolved (task part), 2026-10-26.** `replan task add` now creates the task,
its demands and current-plan reference in one staged validated write, and
`replan task set-dates` records a reviewed one-task date decision. Both use
splice writes, one audit record and the shared apply gate. The composite
acceptance-chain and fixture-derived-count parts remain separate; the counts
are unchanged here.

**Resolved (acceptance chain part), 2026-10-26.**
`scripts/accept-task.sh` now provides the tracked, dry-run-first composite for
one review-ready task. It validates the ordered evidence and actual mutations,
resumes each completed canonical or OpenProject step from durable state,
reports partial applies with exit 7, leaves dependency proposals for explicit
operator review, and degrades the repository-only overlay step to a documented
skip in the agent kit. The fixture-derived-count part remains separate.

**Resolved (test counts part), 2026-10-26.** The dashboard fixture test derives
its overlay expectation from canonical tasks with OpenProject work-package
source refs, and the smoke derives task, source-ref, sample, binding and mapped
counts once from their owning committed inputs. Two agreement tests check the
sample work packages (with their total/count) and the sanitized example bindings
against the canonical source refs in both directions; the sample test's golden
update mode appends a missing work package without rewriting existing entries. Nothing in
C-03 remains.

**C-04. Two sources of truth for the task to work package mapping, with the parsing copied eight times.** VERIFIED; the privacy consequence is INFERRED.

- Go reads the sanitized `source_refs` in the committed `canonical/tasks.yaml`.
- The Python helpers read the private bindings file. Each one has its own
  `bindings_root.get("tasks")` loop:
  - `check_openproject_drift.py:47-59`
  - `push_openproject_date_drift.py:106-121`
  - `push_openproject_status_drift.py:102-108`
  - `accept_openproject_soft_drift.py:31-35`
  - `openproject_operator_report.py:59-81`
  - `apply_openproject_relations.py:144-160`
  - `openproject_live_overlay.py:24-38`
  - `openproject_work_package_toolkit.py:206-233`
- The overlay build reconciles the two sources by rewriting `tasks.yaml` in a
  temp copy (`openproject_live_overlay.py:42-60`).
- `replan import accept --apply` writes source refs into canonical files
  (`internal/app/import_accept.go:495-520`). Run against a tracked project, it
  would commit real work package ids.
- **Fix:**
  - one `scripts/lib/openproject_bindings.py` module, used by every helper;
  - later, a Go `--bindings` overlay layer;
  - `import accept` refuses to write OpenProject refs into a project that has
    a bindings file.

**Resolved (Go bindings layer), 2026-10-27.** The filestore now replaces task
OpenProject refs in memory from an explicit `--bindings` path or
`REPLAN_OPENPROJECT_BINDINGS_LAYER`; every read gets the same layer, and the
live-overlay helper no longer copies or rewrites the project. Canonical writers,
snapshot and candidate writes and in-project report or export output refuse
the layer, and the layered store refuses any project write. Dependency and resource binding
lists are intentionally not layered yet. The remaining C-04 work is the
separate planned task `guard-import-accept-against-real-ids`; this change does
not implement that accept guard.

**Resolved (import accept guard), 2026-10-27.** Projects can set
`project.openproject_ids: private_bindings`. Import acceptance then refuses
every change that would persist an OpenProject object id, while date,
tracking-status and resource-demand acceptance remains available. Live API
snapshot persistence is also refused; dry-run and file-mode snapshot writes
remain available. Together with the shared bindings parser and Go bindings
layer above, nothing in C-04 remains.

**C-05. The kit's defaults point at the dogfood project, and the claim that every helper reads `.env.local` is false.** VERIFIED.

- 15 scripts default `REPLAN_PROJECT` to `examples/replan-self`, which the kit
  does not ship: `build-agent-kit.sh:72` ships only `small-team`.
- About 10 scripts default the OpenProject project id to `replan-self`.
- Five kit scripts never source `load-dotenv.sh`. The kit README says they do
  (`docs/agent-kit/README.md:46,109`). The five are:
  - `plan-openproject-bootstrap.sh`
  - `check-resource-schedule-drift.sh`
  - `explain-resource-overload.sh`
  - `plan-resource-demand-calibration.sh`
  - `review-resource-pool.sh`
- `check-agent-kit.sh:97` passes the variables explicitly, so the kit check
  does not catch this.
- Bootstrap writes `resource.human.maintainer` into a kit user's bindings
  (`apply_openproject_bootstrap.py:282`).
- **Risk:** a kit user who misses one setting gets a confusing failure, or
  works on another project of a shared OpenProject instance.
- **Fix:**
  - no dogfood defaults: fail with `missing_setting` instead;
  - load the dotenv file in every kit script;
  - run the kit check with only `.env.local`.
- **Progress, 2026-10-15.** Every kit script now loads the settings file.
  The schedule drift check and the overload explainer started doing so with
  C-15. The bootstrap planner, the calibration planner and the pool reviewer
  followed in `task.self.load-dotenv-in-every-kit-script`. A test reads the
  kit's script list and fails on any script that does not load the file.
  The defaults and the env-file-only kit check are their own tasks.
- **Progress, 2026-10-15.** No kit helper falls back to this repository's
  plan, OpenProject project or resources any more:
  - `replan_require_settings` answers `missing_setting`, exit 8, naming
    every missing setting;
  - this repository keeps its values in its own `.env.local`, and the smoke
    sets them explicitly;
  - bootstrap takes the assignee's resource from
    `REPLAN_OPENPROJECT_ASSIGNEE_RESOURCE_ID`;
  - the overload explainer needs its resource and unit;
  - the pool reviewer's buckets and issues, this project's policy, moved
    from the code to `examples/replan-self/review/resource-pool.yaml`, with
    the same output on this plan.

  The smoke was checked on a copy of the tree without the private settings,
  as CI runs it, so that it does not lean on this checkout's `.env.local`.
- **Progress, 2026-10-25.** The kit check now runs every shipped helper with
  settings loaded only from the kit template and rejects missing settings,
  repository defaults, malformed envelopes, tracebacks and Go dependencies.

### Medium

**C-06. The status mapping lives in four places, and they disagree.** VERIFIED.

- `plan_openproject_bootstrap.py:37-44` and `push_openproject_status_drift.py:31-38`
  hold seven entries each.
- The toolkit's map (`openproject_work_package_toolkit.py:32-44`) has no
  `review_ready` or `cancelled`, and adds `on_hold` and `done`. So "Developed"
  and "Rejected" have no reverse mapping there.
- Go's `mapOpenProjectStatus` (`internal/app/import_openproject.go:1822-1840`)
  matches names fuzzily.
- **Fix:** one mapping, and a test that compares it with the Go side.
- **Resolved, 2026-10-25.** The seven pairs now live in
  `scripts/lib/openproject_status_map.json`; all Python helpers load it, and a
  Go test checks every exact OpenProject name against the import mapper.

**C-07. Three apply-gate conventions.** VERIFIED.

- These Go commands write unless `--dry-run` is passed. This is accepted as
  GAP-023.
  - `actual *`
  - `proposal accept` / `reject`
  - `scenario apply`
  - `project set-status-date`
- `import accept` requires `--dry-run` or `--apply`.
- `report weekly --write` and `tracking suggest --write-candidates` write only
  when asked.
- The scripts use `REPLAN_APPLY=1`.
- **Risk:** an agent that learned "dry run is the default" from the kit
  README writes canonical state with its first `replan actual` call.
- **Fix:** at least a one-line rule at the top of the agent docs. Better: Go
  honours `REPLAN_APPLY`, or defaults to a dry run in an agent mode.
- **Resolved, 2026-10-26.** Every canonical-state writer now uses one shared
  explicit `--dry-run`/`--apply` gate with the `REPLAN_APPLY=1` fallback, and
  repository callers pass a flag instead of relying on that fallback.

**C-08. The JSON envelope and exit codes differ from the documented contract.** VERIFIED.

- **Error shape.** An `"error": "<code>"` string appears in five places:
  - `apply_openproject_relations.py:49`
  - `apply_openproject_bootstrap.py:53`
  - `push_openproject_status_drift.py:213`
  - `publish-openproject-overlay.sh:28`
  - `openproject-work-package-toolkit.sh:19`

  Go and the other helpers use `errors: [{code, message}]`. On the string
  shape, `envelope_check.require_ok` prints `[]`.
- **Mode key.** `check-openproject-drift.sh` reports `drift_status` where
  other helpers report `mode`.
- **Plain text instead of JSON on stderr:**
  - `check-openproject-drift.sh:58`
  - `push-openproject-status-drift.sh:20`
  - `plan-openproject-bootstrap.sh:11`
- **Exit codes.** The same condition exits with different codes in different
  scripts, for example an unsupported format: 8 in the toolkit, 1 in the
  status push. Go never returns 3, 5, 6 or 7, which `cli.md` lists.
- **Fix:** a shared Python `emit` function and a shell `fail_json` function,
  both mapped to the `cli.md` exit-code table.
- **Resolved, 2026-10-15.** One envelope for every helper:
  - `scripts/lib/helper_envelope.py` and `scripts/lib/envelope.sh` hold it,
    and every wrapper and module prints through them;
  - an exception nothing caught becomes an envelope too;
  - exit codes follow the table.

  A test runs all 18 wrappers from a copy without settings and checks the
  envelope and exit code for three failures: a wrong format, a missing
  setting and an unreachable OpenProject. It found more than this finding
  listed:
  - the status push and the operator report answered nothing at all when
    OpenProject was unreachable;
  - `replan import openproject` exited 2, the code of an invalid plan, on a
    network error. It now exits 6 for OpenProject's own failures, while an
    invalid sidecar stays 2.

**C-09. A failure in the middle of an apply hides what was already written, and POST is retried.** VERIFIED; the duplicate is INFERRED.

- The catch-all in `push_openproject_status_drift.py:205-216` prints only the
  error. The actions already applied are lost. The bootstrap and relations
  helpers use the same pattern.
- `openproject_request_json` (`openproject_http.py:145-210`) retries every
  method. The convention recommends two retries, so a `POST /work_packages`
  that times out can create a duplicate.
- The drift-based pushes recompute on every run, so a re-run usually resumes
  cleanly.
- **Fix:** add `applied_so_far` to the failure output, and retry only GET and
  PATCH requests that carry a `lockVersion`.

**C-10. Re-running bootstrap clobbers the bindings, and pagination is inconsistent.** VERIFIED.

- `apply_openproject_bootstrap.py:268-294` rewrites the bindings file with
  `"dependencies": []` and no existence check. This drops the relation
  bindings written by `apply_openproject_relations.py:212`.
- The bootstrap reads one page of 500 work packages
  (`apply_openproject_bootstrap.py:106`).
- The relations helper starts at `offset = 1` and then adds the element count
  (`apply_openproject_relations.py:97,108`). OpenProject's `offset` is a page
  number, and the wiki and toolkit helpers use it that way. With more
  relations than one page holds, the helper skips pages.
- **Fix:** merge into the bindings file, or refuse to overwrite it, and use one
  shared `paginate()`.
- **Progress, 2026-10-15.** The relations half was fixed early, because it
  blocked this plan.
  - The `/api/v3/relations` collection holds the relations of every project
    on the instance: 631 on a page size of 500.
  - The old loop asked for pages 1 and 501 and saw 500 of them. None of this
    project's relations were on the first page.
  - The dry run therefore planned 113 duplicate relations instead of 17 new
    ones.
  - `openproject_http.openproject_collect_pages` now reads page numbers, with
    a test of three pages. The relations helper uses it.
  - The bootstrap page limit is still open.
- **Resolved, 2026-10-25.** Bootstrap now merges existing private bindings and
  reads every page of project work packages through the shared paginator; its
  three-page regression test keeps a page-three match instead of creating a
  duplicate.

**C-11. A Python helper edits canonical YAML outside the audit log.** VERIFIED.

- `accept_openproject_soft_drift.py:100-133` rewrites `canonical/tasks.yaml`
  line by line with a regex. It takes no actor and writes no audit record.
- `replan import accept --apply --actor` writes an audited change.
- **Fix:** `replan import accept --accept-dates`, with the script as a thin
  wrapper around it.
- **Resolved, 2026-10-26.** `import accept --accept-dates` now stages and
  validates matched snapshot date changes with one audited import-accept
  record. The live soft-drift helper retains private-binding matching and its
  review JSON, but records import evidence and delegates canonical writes to
  audited `replan task set-dates` calls with `REPLAN_ACTOR` required on apply.

### Medium, found later

**C-15. The schedule drift check sums concurrent slots over a week.** VERIFIED.

Found on 2026-10-15 while planning this review's tasks, after the review
itself.

- `check_resource_schedule_drift.py` spreads each demand over its task's
  days and adds the days of a week together. Five one-day tasks in a row on
  the single development slot count as five slots of one.
- The check therefore reported the slot overloaded in every week from W43 to
  W50, while `replan workload --week` shows one of one in each of them.
- GAP-031 fixed this in Go only. The helper keeps its own copy of the
  workload calculation.
- **Risk:** the drift check raises a false alarm on every sequential plan. An
  agent either distrusts it or replans for no reason.
- **Fix:** the helper takes its weekly rows from `replan workload`, so there
  is one workload calculation. A test plans consecutive one-day tasks on one
  slot.
- **Resolved, 2026-10-15.** `replan workload --demands` lists the demands of
  every row. The drift check and the overload explainer, which had the same
  copy of the calculation, now read their rows from it through
  `scripts/lib/replan_workload.py`. On this plan, the drift check moved from
  eight overloaded weeks to none, matching `replan workload`.

**C-16. Creating a relation moves dates in OpenProject, and the helper does not say so.** VERIFIED.

Found on 2026-10-15 while pushing this plan's relations.

- `apply-openproject-relations.sh` creates `follows` relations. OpenProject's
  automatic scheduling then moves each successor to the earliest date the
  relation allows.
- Nine of the new work packages moved, the release story among them. The
  helper answered `ok` and reported only relation counts.
- The date drift check showed the moves, and the date push restored the
  RePlan dates. Pushing was possible only after the dates of three closed
  tasks were corrected through the toolkit, because the push will not touch a
  closed task and refuses to apply while one waits for review.
- The same happens on a date push: when a task finished early and its dates
  were pushed earlier, OpenProject pulled its successor forward too, and a
  second push had to restore it (2026-10-15, after the helper envelope task).
- **Risk:** an agent that creates relations and stops there leaves OpenProject
  showing dates that RePlan never planned.
- **Fix:** the helper reads the affected work packages' dates before and
  after, reports every moved date with a warning code, and its next steps
  name the date push.
- **Resolved, 2026-10-27.** The relations helper now snapshots each new
  `follows` successor and its bounded transitive `follows` chain before all
  creates and after all creates. It reports changed fields in
  `rescheduled_work_packages` with `openproject_rescheduled`, attributes a
  direct successor to its created relation when unambiguous, and sends the
  operator through date drift check and reviewed acceptance or date push. A
  failed final read is `rescheduling_unknown`, exit 11: the creates succeeded,
  so it is attention-needed rather than a partial apply.

**C-17. No reviewed way to correct a work package's subject or description.** VERIFIED.

Found on 2026-10-15 while recording progress on a planned task.

- The toolkit creates work packages. It can update status, dates, estimate,
  assignee, type and priority.
- It cannot change a subject or a description.
- **Risk:** an agent that must correct one has no reviewed path, and the rule
  against raw API mutation leaves it stuck. Progress on a task can go only
  into a comment.
- **Fix:** add `work-package update-text`, with a dry run that shows old and
  new text, a lockVersion check, and the RePlan title kept in step.
- **Resolved, 2026-10-28.** The toolkit now previews and applies subject and
  raw-Markdown description changes with optimistic lock checks and maps a 409
  to `openproject_work_package_changed`. Bound subject changes preflight and
  then audit `replan task set-title`; failure after the OpenProject PATCH is a
  partial apply with the exact RePlan completion command. Descriptions remain
  separate RePlan planning notes by design. A follow-up review also made the
  binary lookup work in both kit and checkout layouts and made an
  OpenProject-already-correct subject repair RePlan-only `title_drift` without
  issuing an empty PATCH.

**C-18. The date push reports a date as applied when OpenProject stored another.** VERIFIED.

Found on 2026-10-25 while pushing the dates of a task finished on a Sunday.

- `push-openproject-date-drift.sh` sent 2026-10-25 for a work package.
  OpenProject's working-day calendar moved both dates to Monday, 2026-10-26.
- The helper answered `ok` with `applied_actions: 1`. Its own action carried
  `updated_openproject_start_date: 2026-10-26`, but no warning said that the
  stored date differs from the one sent.
- Each new dry run planned the same push again, and each apply "succeeded"
  without changing anything.
- **Risk:** an agent loops on the push, or believes that OpenProject shows the
  RePlan dates when it does not.
- **Fix:** compare the dates OpenProject returns with the dates sent. Report
  each difference with a warning code and the likely reason, such as a
  non-working day, and say in the next steps that the RePlan dates should
  follow OpenProject's working days.
- It is the same class as C-16, a date OpenProject changes without the helper
  saying so, and it is planned in the same task.
- **Resolved, 2026-10-27.** The batch date push compares every PATCH response
  with the sent start and due dates, reports differences in
  `stored_date_differences` with `openproject_stored_other_dates`, and labels a
  weekend or an OpenProject `/api/v3/days/{date}` non-working result as
  `non_working_day`. It also snapshots the pushed work packages and their
  bound `follows` successor chains, reporting scheduler moves like C-16. The
  toolkit's single-work-package `update-dates` uses its separate generic PATCH
  path, so this batch-helper response check is not shared with it.
- **Follow-up 2026-10-28.** All dated work-package writers now read the
  instance working week and set `ignoreNonWorkingDays: true` in the same write
  when a sent date uses a non-working weekday, preserving intentional weekend
  dates; the moved-date reporting remains in place as a backstop.

**C-19. A wrong argument to the toolkit or the wiki helper is answered in plain text.** VERIFIED.

Found on 2026-10-25 while creating two work packages.

- `openproject-work-package-toolkit.sh work-package create` without
  `--subject` printed argparse's usage text to stderr and exited 2. Stdout was
  empty, not the helper envelope.
- The wiki helper uses argparse the same way.
- **Risk:** an agent that parses stdout as JSON fails on the parse, not on the
  argument it got wrong.
- **Fix:** an argument error answers the envelope on stdout with a
  `usage_error` code, the usage text in the message and exit 8. The `--help`
  task, C-13, covers argument handling, and this joins it.
- **Resolved, 2026-10-26.** The toolkit and wiki use the shared envelope
  argument parser for every subparser. Parse errors put the argparse message
  and command-specific usage line in a `usage_error` envelope on stdout and
  exit 8; help remains plain stdout text with exit 0 and initializes no
  settings or HTTP client.

### Low

**C-12. The HTTP layer is only half shared, and the shell to Python interface is positional.** VERIFIED.

- Six modules redefine `auth_headers` and `request_json`.
- Pagination is written three ways.
- Wrappers pass 6 to 11 positional arguments, each in its own order.
- The dotenv allowlist is a separate hand-kept list.
- Go reads the timeout variable but not the retries variable.
- **Fix:** one `Settings.from_env()` in Python, and wrappers that pass `"$@"`.
- **Resolved, 2026-10-26.** The modules read wrapper settings through the
  shared `Settings.from_env()` contract. Wrappers pass only generated runtime
  artifacts as named options, and otherwise forward the operator's arguments.
  Internal option parsing failures use the JSON helper envelope with
  `usage_error` and exit 8.

**C-13. Help text and documentation drift.** VERIFIED.

- Only the wiki and toolkit scripts answer `--help`. The rest pass unknown
  arguments on to the CLI. INFERRED: some may then reach the live server.
- `replan actual --help` exits 8.
- `cli.md` still lists `--resource-class` and `--unit` as not implemented.
- `getting-started.md` calls overlay publishing an operator step, although a
  helper exists for it.
- `getting-started.md` carries a 1 050-line implementation backlog, against
  the backlog rule.
- `AGENTS.md` links neither `getting-started.md` nor the kit README.
- `docs/dev` holds 24 `openproject-*` design documents.
- The kit README says OpenProject owns statuses and dates, then lists the
  status and date pushes under "push what RePlan owns".
- Dates also flow from OpenProject into RePlan, and no rule says which
  direction wins.
- **Fix:**
  - a header comment and `--help` in every script;
  - one surface-map table;
  - an explicit rule for the direction of dates.
- **Resolved (help portion only), 2026-10-26.** All 32 shell scripts have an
  operational header and answer `-h`/`--help` on stdout before settings or
  side effects. Environment-driven scripts reject extra arguments explicitly,
  and CLI family, family-command and single-command help exits 0 on stdout.
  The other documentation-drift items in this finding remain separate work.
- **Resolved (CLI spec and getting-started portion), 2026-10-26.** The CLI
  contract now matches every implemented command and flag, distinguishes Go
  exits from helper exits, and states the actual `REPLAN_ACTOR` readers. A Go
  test checks the implemented flag inventory against command help. The guide's
  historical backlog moved to a frozen dated record, current usage stayed in
  the guide, and overlay publication names its dry-run/apply helper. The
  AGENTS links, surface map, date-direction rule, design-doc index and kit
  README wording remain separate tasks.
- **Resolved (surface map and date direction portion), 2026-10-26.** `AGENTS.md`
  links the standalone kit rule and command map. The map distinguishes shipped
  from repository-only helpers and names manual gaps. The rule records the
  implemented date and status directions without inventing accept flags. The
  `docs/dev/openproject-*` design-doc index remains separate work.
- **Resolved (design-doc index portion), 2026-10-26.** The 24 OpenProject
  design, review and result documents now carry uniform status lines and link
  to a grouped index. `AGENTS.md` links the index. C-13 is now fully resolved.

**C-14. Environment variable naming noise.** VERIFIED.

**Resolved, 2026-10-26.** OpenProject helpers now share one project identifier
and project name, retain the old bootstrap names as warned one-release aliases,
and describe `REPLAN_FORMAT` according to the formats each script actually
prints.

- `REPLAN_OPENPROJECT_PROJECT_ID` and `…_TARGET_PROJECT_ID` must be set to the
  same value.
- `REPLAN_FORMAT` exists only to reject anything but `json`.
- `cli.md` calls `REPLAN_ACTOR` unimplemented, but the sidecar pipeline reads
  it.
- **Fix:** one project id with an optional override, and document or drop
  `REPLAN_FORMAT`.

## Keep

- The single auth-header builder and the redaction of error bodies
  (`openproject_http.py:56-130`).
- The dotenv allowlist: a file can never set `REPLAN_APPLY` or `REPLAN_BIN`.
- The kit check that the perimeter key is sent and never printed, for both
  Python and Go.
- The wiki edit protocol: `--expect-lock-version`, `--diff-output` and named
  refusal codes.
- The status push checks the workflow's allowed transitions before it writes.
- The overlay publish refuses an overlay of another project.
- The overlay schema is versioned on both the Go and the Ruby side.
- The pushes are drift-based, so they are naturally resumable.
- The kit is built from the repository's files and checked against a live
  instance.

## Consolidation sequence

The steps go from smallest risk to largest. Each can be its own task.

1. **Envelope and exit codes (4–6 h).**
   - A shared `emit` function in Python and a `fail_json` function in shell.
   - A missing token under apply fails.
   - `envelope_check` reads the string error shape.
   - Fixes C-01, C-02 and C-08.
2. **Settings that are safe for the kit (2–3 h).**
   - Every kit script loads the dotenv file.
   - No dogfood defaults.
   - Bootstrap does not overwrite the bindings file.
   - The kit check runs with only `.env.local`.
   - Fixes C-05 and part of C-10.
3. **Shared bindings and status map (about 4 h).**
   - A bindings module and one status mapping, with a test against Go.
   - Fixes C-04 (the DRY part) and C-06.
4. **A complete HTTP client (about 4 h).**
   - `request_json`, `paginate()` and `Settings.from_env()` in one module.
   - The relations offset is fixed, and POST is no longer retried.
   - Failure output carries `applied_so_far`.
   - Fixes C-09, C-10 and C-12.
5. **Docs pass (3–4 h).**
   - A surface map and the gate rules in `AGENTS.md` and the kit README.
   - `--help` in every script.
   - The drift in `cli.md` and `getting-started.md` is fixed.
   - The rule for the direction of dates is written down.
   - The backlog section moves out of `getting-started.md`.
   - Fixes C-07 (the docs part), C-13 and C-14.
6. **Track the whole loop (1–2 days).**
   - `replan task add`, `replan evidence add` and
     `replan import accept --accept-dates`.
   - A tracked `accept-task.sh` with a dry run and resumable steps.
   - Test counts derived from the fixture.
   - Fixes C-03 and C-11, and makes the private glue scripts unnecessary.

Steps 1 and 2 matter most before another project's agent relies on the kit.
Step 6 matters most for this repository's own loop.

## Backlog

On 2026-10-15 the operator decided to plan every finding and every open or
accepted gap as a task, done in full.

- **38 tasks**, one development slot each working day from 2026-10-16 to
  2026-12-14.
- **Order.** It follows the consolidation sequence. The schedule drift fix
  comes first, because the check gives false alarms on this very plan.
- **Release story.** It moves to 2026-12-14 to 24, after the public
  repository is prepared.
- **C-15 to C-17** were found after the review, while this plan was being
  entered. **C-18** was found on 2026-10-25 and joins the C-16 task; **C-19**,
  found the same day, joins the first C-13 task.
- **C-17** comes after the kit refresh, on the release story's first day,
  which needs no development slot.

| Finding or gap | Tasks |
| --- | --- |
| C-15 | `task.self.count-concurrent-slots-in-schedule-drift-like-workload` (2026-10-16) |
| C-16, C-18 | `task.self.report-openproject-rescheduling-after-relation-create` (2026-11-30) |
| C-17 | `task.self.edit-work-package-subject-and-description-in-toolkit` (2026-12-14) |
| C-01 | `task.self.fail-apply-without-openproject-token` (2026-10-21) |
| C-02 | `task.self.derive-helper-exit-codes-from-ok` (2026-10-20) |
| C-03 | `task.self.add-replan-task-add` (2026-11-17 to 2026-11-18)<br>`task.self.add-replan-evidence-add` (2026-11-16)<br>`task.self.track-the-task-acceptance-chain` (2026-11-20)<br>`task.self.derive-self-plan-counts-from-fixture` (2026-11-23) |
| C-04 | `task.self.share-openproject-bindings-module` (2026-10-28)<br>`task.self.read-private-openproject-bindings-in-go` (2026-11-24 to 2026-11-25)<br>`task.self.guard-import-accept-against-real-ids` (2026-11-26) |
| C-05 | `task.self.load-dotenv-in-every-kit-script` (2026-10-22)<br>`task.self.drop-dogfood-defaults-from-kit-scripts` (2026-10-23)<br>`task.self.check-agent-kit-with-env-file-only` (2026-10-26) |
| C-06 | `task.self.single-openproject-status-map` (2026-10-29) |
| C-07 | `task.self.one-apply-gate-rule-for-cli-and-helpers` (2026-11-13) |
| C-08 | `task.self.unify-helper-json-envelope` (2026-10-19) |
| C-09 | `task.self.retry-only-idempotent-openproject-requests` (2026-11-03)<br>`task.self.report-applied-actions-on-helper-failure` (2026-11-04) |
| C-10 | `task.self.keep-existing-bindings-on-bootstrap` (2026-10-27)<br>`task.self.fix-openproject-relations-and-bootstrap-pagination` (2026-11-02) |
| C-11 | `task.self.accept-openproject-dates-through-import-accept` (2026-11-19) |
| C-12 | `task.self.share-openproject-http-client-and-settings` (2026-10-30)<br>`task.self.pass-named-arguments-to-python-helpers` (2026-11-05) |
| C-13, C-19 | `task.self.add-help-to-every-script` (2026-11-06)<br>`task.self.sync-cli-spec-and-getting-started` (2026-11-09)<br>`task.self.add-agent-surface-map-and-date-direction-rule` (2026-11-10)<br>`task.self.index-openproject-design-docs` (2026-11-11) |
| C-14 | `task.self.clean-up-openproject-env-vars` (2026-11-12) |
| Surface map: overlay not in the kit | `task.self.ship-overlay-publish-in-agent-kit` (2026-11-27) |
| GAP-010 | `task.self.move-pilot-narrative-to-private-storage` (2026-12-09) |
| GAP-012 | `task.self.report-missing-project-root-as-not-found` (2026-12-08) |
| GAP-020 | `task.self.clear-remaining-lint-baseline` (2026-12-04 to 2026-12-07) |
| GAP-023 | `task.self.one-apply-gate-rule-for-cli-and-helpers` (2026-11-13) |
| GAP-025 | `task.self.add-contributing-and-security-policy` (2026-12-10) |
| GAP-030 | `task.self.preview-moved-task-dates-in-scenarios` (2026-12-02 to 2026-12-03) |

The task `task.self.refresh-agent-kit-after-tooling-consolidation` brings the
consolidated helpers to the second project. These tasks need an operator
decision, and their descriptions say so:

- the apply gate options (C-07, GAP-023);
- the overlay in the kit;
- the publication tasks.
