# RePlan agent kit

This archive lets a person or an AI agent plan resources with RePlan and keep
the plan in step with an OpenProject project, without the RePlan source
repository and without an OpenProject administrator token.

It contains:

- `bin/replan`: the RePlan command-line program (Linux, x86-64, no
  dependencies);
- `scripts/`: the OpenProject helpers, each a shell script that prints JSON:
  for work packages and the plan, and `openproject-wiki.sh` for the project's
  wiki;
- `examples/small-team/`: a small RePlan project to try the commands on;
- `env.local.example`: the settings template;
- `LICENSE` and `NOTICE.md`: the Apache-2.0 licence and repository notice;
- `VERSION`: the build this archive was made from.

Use RePlan only through these commands and helpers; "Use RePlan as a tool"
below says what that means and what to do when a command is missing.

## What you need

- Linux on x86-64, `sh` and `bash`;
- Python 3.10 or newer with PyYAML (`python3 -c 'import yaml'` must work);
  CI runs the helpers' tests and the kit checks on 3.10;
- `rsync`, for publishing the RePlan overlay to OpenProject;
- an OpenProject account that is a member of your project, and an API token
  of that account: in OpenProject, "My account", "Access tokens", "API";
- a RePlan project directory, the plan. Keep it in a version-control
  repository of your own.

No administrator is needed for daily work. An OpenProject administrator is
needed once, for what the list under "What needs an administrator" names.

## Set up

1. Unpack one kit directory per project and stay in that directory. Do not
   share a configured kit between projects.
2. Copy `env.local.example` to `.env.local`, next to `scripts/`, and fill it
   in. Each project has its own `.env.local`. It holds your token: never
   commit it and never print it.
3. Check the program and the connection:

   ```sh
   bin/replan validate "$PWD/examples/small-team"
   scripts/openproject-work-package-toolkit.sh work-package list
   ```

   The first prints warnings of the example plan and no errors. The second
   prints a JSON document with `"ok": true` and your project's work packages.

Every helper reads `.env.local` and then `.env`; settings already in the
environment win. Set `REPLAN_ENV_FILE` in the environment to read only that
file instead. The named file must be readable. `REPLAN_ENV_FILE` itself is
never read from a settings file.

Settings that control one run or one acceptance are environment-only. They
include `REPLAN_APPLY`, `REPLAN_ACTOR`, `REPLAN_BIN`, `REPLAN_FORMAT`,
`REPLAN_KEEP_TEMP`, `REPLAN_ENV_FILE`, `REPLAN_OPENPROJECT_PROFILE`,
`REPLAN_OPENPROJECT_FROM_FILE`, `REPLAN_OPENPROJECT_STATUS_SCOPE`,
`REPLAN_OPENPROJECT_STATUS_PATH`,
`REPLAN_TASK`, `REPLAN_ACCEPTED_BY` and the `REPLAN_ACCEPT_*` settings. The
complete list shipped with the helpers is
`scripts/lib/environment-only-settings.list`. A non-empty environment-only
setting in `.env.local`, `.env` or the file selected by `REPLAN_ENV_FILE`
remains ignored and produces an `environment_only_setting_in_file` warning.
The warning names only the setting and settings-file basename, never its value
or full path. Export the setting in the process environment when you intend it
to affect a run.

The per-project settings are `REPLAN_PROJECT`,
`REPLAN_OPENPROJECT_PROJECT_NAME`, its deprecated
`REPLAN_OPENPROJECT_TARGET_PROJECT_NAME` alias, the three
`REPLAN_OPENPROJECT_ASSIGNEE_*` settings,
`REPLAN_OPENPROJECT_BINDINGS_PATH`, `REPLAN_OVERLAY_OUTPUT_DIR` and
`REPLAN_OVERLAY_PUBLISH_TARGET`. If the environment selects one OpenProject
project but the settings file names another, a helper that uses any of those
file values refuses with `mixed_project_settings`, exit 8, before a request.
Connection-only commands may reuse one connection file for several projects.

The helpers have no default plan or project: one that needs `REPLAN_PROJECT`
or `REPLAN_OPENPROJECT_PROJECT_ID` and does not find it answers
`missing_setting`, exit 8, with the names in `missing_settings`, instead of
working on another project.

`REPLAN_OPENPROJECT_PROJECT_ID` is the identifier used by every helper,
including bootstrap. Bootstrap uses `REPLAN_OPENPROJECT_PROJECT_NAME` only when
it creates a project; when omitted, the name defaults to the identifier. For
one release, the old `REPLAN_OPENPROJECT_TARGET_PROJECT_ID` and
`REPLAN_OPENPROJECT_TARGET_PROJECT_NAME` names remain accepted with a
`deprecated_setting` warning. If an old and new name are both set differently,
the helper refuses them with `conflicting_settings`, exit 8, before a request.

## Behind a reverse proxy

Some OpenProject installations sit behind a reverse proxy that keeps a shared
password on every address, the API included. From outside, such a proxy
answers the API with HTTP 401 and a `WWW-Authenticate: Basic` header of its
own before OpenProject sees the request, so a correct token still fails. The
tell-tale sign is an HTML body and a `Server` header naming the proxy.

If the administrator gives API clients a perimeter key, put it in
`.env.local`:

```sh
REPLAN_OPENPROJECT_PERIMETER_KEY=<the key you were given>
```

Every helper, and `bin/replan import openproject`, then sends it in the
`X-Perimeter-Key` header beside your own token, which reaches OpenProject
unchanged. Another header name goes in `REPLAN_OPENPROJECT_PERIMETER_HEADER`.
The key is a secret like the token: it is never printed, and it must not be
committed.

The token and perimeter key may instead be read at run time from protected
files. Set `REPLAN_OPENPROJECT_TOKEN_FILE` and/or
`REPLAN_OPENPROJECT_PERIMETER_KEY_FILE` to their paths. Surrounding whitespace
is removed from the file content. A direct token selected by
`REPLAN_OPENPROJECT_TOKEN_ENV` (normally `OPENPROJECT_TOKEN`) or a direct
`REPLAN_OPENPROJECT_PERIMETER_KEY` wins when both forms are set. An unreadable
file is a `missing_setting` usage error that names the `_FILE` setting; secret
values and file contents are never included in output.

## The rule every helper follows

Nothing writes canonical state without `--apply` or `REPLAN_APPLY=1`.
Helpers default to a dry run; CLI commands refuse to guess and require
`--dry-run` or `--apply` unless `REPLAN_APPLY=1` supplies the apply choice.
An explicit `--dry-run` remains a preview even when `REPLAN_APPLY=1` is set.
Writes to one RePlan project are serialized through `.replan/write.lock`; a
`project_locked` response with exit 5 means another writer did not release the
lock within the wait limit, so leave the lock file in place and retry the
complete command.

Canonical applies journal the validated file set in
`.replan/commit-pending/` before replacing project files. The next locked
write completes an interrupted commit before loading the project and reports
`recovered_commit`; read-only commands only warn with
`interrupted_commit_pending`. Use `bin/replan project recover --project
"$REPLAN_PROJECT" --dry-run`, then `--apply`, for explicit recovery. A
`commit_journal_corrupt` error changes nothing; inspect the journal directory.

A helper that would change OpenProject or the bindings file only reports what
it would do. It changes something only when run with `REPLAN_APPLY=1`:

```sh
scripts/push-openproject-status-drift.sh                 # dry run
REPLAN_APPLY=1 scripts/push-openproject-status-drift.sh  # apply
```

Read the dry run first. Every helper prints one JSON document with `ok`,
`command`, `mode` (`dry_run`, `apply`, `read_only`, `skipped` or `invalid`),
`warnings` and `errors`, each entry with a `code` and a `message`, and their
codes in `warning_codes` and `error_codes`; most add `counts` and `actions`.
This holds for a failure too, never a bare text line.
Every helper also answers `-h` or `--help` as plain text on stdout, with exit
0 and without reading settings or contacting OpenProject. A wrong or missing
argument is instead the JSON envelope with `usage_error` and exit 8.

The exit code follows from the answer:

- 0 when `ok` is true, a soft drift included;
- 11 when the helper ran to the end but needs your attention, a hard drift
  or a blocked or refused action;
- 7 when an apply wrote some actions and not others;
- for a failure, a code from the CLI's table: 8 for a missing or wrong
  setting, 6 when OpenProject failed or could not be reached, 4 for a missing
  file, 2 for an invalid plan, 1 otherwise.

So `set -e` and `&&` stop where you need to look. The token is never printed, and OpenProject's error
bodies are reduced to their identifier and message.

## What the CLI trusts

The CLI and the helpers run with your operating-system user's rights and do
not authenticate the `--actor` you name. Under the default `proposal_only`
policy, approval/apply commands refuse an actor whose kind is not `human`;
this includes proposal decisions, scenario and import application, baseline
and calibration approval, and accepted/cancelled task status. Dry runs report
the same `approver_not_human` refusal. Omitting `ai_agents` from policy keeps
that default; `default_mutation_mode: direct` explicitly allows existing
agent actors for teams operating bounded agents.

This checks the named actor, not the caller. An agent with write access can
still name a human actor, change policy or edit YAML directly. For a real
person-only boundary, run the agent as its own user or on a read-only copy and
let a person approve in their own shell. The RePlan repository's
`docs/security/access-and-actor-model.md` lists the exact covered and open
commands.

## Who owns what

- OpenProject owns task identity, titles, source status, relations, assignees,
  comments, the Gantt chart and the task view people see.
- RePlan owns tracking status and drafts planned dates from capacity and load.
  Reviewed helpers push both to OpenProject for people to see.
- RePlan owns what OpenProject has no place for: resource demands,
  capacities, workload, scenarios, actual consumption, evidence, proposals and
  the audit log.
- The bindings file (`REPLAN_OPENPROJECT_BINDINGS_PATH`) says which RePlan task
  is which work package. It holds real work-package numbers, so keep it out of
  a public repository.

That setting belongs to the Python helpers and is not implicitly read by the
Go CLI. To inspect the plan with those private ids, pass
`bin/replan <command> --bindings "$REPLAN_OPENPROJECT_BINDINGS_PATH"` or set
`REPLAN_OPENPROJECT_BINDINGS_LAYER` to the file. The flag wins. This read-only
layer covers task bindings only; dependency and resource bindings remain
helper-owned. Use an output directory outside the plan when generating a
dashboard with the layer.

An outside project that keeps real OpenProject ids in private bindings should
set `project.openproject_ids: private_bindings` in `.replan/project.yaml`.
This makes live API snapshot persistence dry-run-only and prevents import
acceptance from writing OpenProject object ids into project files. File-mode
snapshot writes remain available, as do date, tracking-status and
resource-demand acceptance.

See [Date and status direction](#date-and-status-direction) for conflicts and
the reviewed flow in each direction.

## Date and status direction

RePlan drafts `planned_start` and `planned_finish`, because it sees capacity
and load. They reach OpenProject `startDate` and `dueDate` only through
`scripts/push-openproject-date-drift.sh` or toolkit `work-package update-dates`.
Both are dry runs by default and write only with `REPLAN_APPLY=1`.
An apply when the date push is already aligned is a successful no-op: it exits
zero, reports `no_op: true` and `applied_actions: 0`, and does not suggest
applying again. Review or blocked actions still refuse apply even when there
are no planned actions.

Once pushed, OpenProject shows the dates to people. Bound work packages stay
manually scheduled so OpenProject does not recalculate RePlan-owned dates.
If someone moves a Gantt bar, neither side wins automatically.
`scripts/check-openproject-drift.sh` reports the difference. A person decides
whether to push the RePlan dates again or accept the OpenProject dates into
RePlan. Review `scripts/accept-openproject-soft-drift.sh`, then run it with
`REPLAN_ACTOR=<actor-id> REPLAN_APPLY=1` to accept those dates. Despite its
name, that helper accepts only `planned_start` and `planned_finish`; it reports
status differences for review only and is the batch path for soft drift. It
uses the private bindings to match live work-package ids, records one evidence
item for the run, and writes each matched task through audited `bin/replan task
set-dates` calls. To accept reviewed OpenProject dates one task at a time, run
`bin/replan task set-dates ... --dry-run`, review it, then repeat with
`--apply`. For a stored snapshot whose sanitized source refs already match
canonical tasks, use `bin/replan import accept <snapshot-id> --accept-dates
--dry-run`, then `--apply --actor <id>`.

After applying relations or pushing dates, run
`scripts/check-openproject-drift.sh`. Relation apply snapshots reachable bound
successors and restores them roots-first to manual scheduling and their prior
dates. Its `restored_work_packages` lists planned or completed restores;
`openproject_rescheduled` means a difference remained after final verification
and `rescheduled_work_packages` lists it. Set
`REPLAN_OPENPROJECT_KEEP_AUTOMATIC_SCHEDULING=1` only when the team deliberately
wants OpenProject to schedule successors.
The drift check emits `openproject_date_drift` once per affected task and
`openproject_automatic_scheduling` once per bound work package left automatic.
`openproject_scheduling_unknown` means the live scheduling-mode collection
could not be read; it does not change the otherwise computed drift status.
For the former, review `push-openproject-date-drift.sh`; for the latter, use
`openproject-work-package-toolkit.sh work-package update-dates` with the task's
current RePlan dates.
`openproject_stored_other_dates` means the date-push response stored a value
different from the one sent; `stored_date_differences` says whether the sent
date was a weekend or was reported non-working by OpenProject. Date writers
read `/api/v3/days/week` once per run. When a sent start or due date is on an
instance non-working weekday, they include `ignoreNonWorkingDays: true` in the
same create or PATCH and report `set_ignore_non_working_days` with the date and
weekday. Weekday-only writes leave the property untouched; they never reset it
to false because that could move dates another tool deliberately preserved.
If the working-week read fails, `openproject_working_days_unknown` reports the
Saturday/Sunday fallback. This check intentionally covers the recurring week
only, not configured holiday dates. If OpenProject still stores another date,
normally accept the scheduler dates with the reviewed soft-drift flow above.
Use `scripts/push-openproject-date-drift.sh` again only when the reviewed RePlan
dates should win. `rescheduling_unknown` means the write succeeded but a final
date read failed: inspect the listed work packages and run the drift check; the
helper exits 11 because attention is needed, not 7 because no requested write
is known to have been skipped.

RePlan tracking status goes to OpenProject through
`scripts/push-openproject-status-drift.sh`. An OpenProject status change comes
back from a stored import snapshot through
`bin/replan import accept <snapshot-id> --accept-tracking-status`, with a dry
run before `--apply --actor <id>`. Actual start and finish facts remain in
RePlan. Record them from evidence and actuals with `bin/replan actual
add-status` and the other `actual` commands; push only the resulting tracking
status to OpenProject.

The status push defaults to `REPLAN_OPENPROJECT_STATUS_SCOPE=ready_only`, so
it pushes only a RePlan `ready` task to OpenProject `To be scheduled`. Set
`REPLAN_OPENPROJECT_STATUS_SCOPE=all` in the process environment to consider
every mapped RePlan tracking status. Do not put this setting in a settings
file: it is environment-only. A `status_scope_ready_only` skipped action
includes this instruction in its `hint` and once in `operator_next_steps`.

The separate environment-only setting `REPLAN_OPENPROJECT_STATUS_PATH`
defaults to `direct`, preserving the one-transition behavior. With
`REPLAN_OPENPROJECT_STATUS_PATH=forward`, the helper may walk forward through
the mapped progression `not_started`, `ready`, `in_progress`, `review_ready`,
`accepted`. The actual OpenProject names come from the shipped status map. It
never walks backward or through `blocked`/On hold or `cancelled`/Rejected; those
updates remain direct. Each apply step rereads the work-package form for the
persisted status, uses that response's current lock version, verifies the
PATCH response, and then checks the next step. A stopped walk reports
`openproject_status_path_not_found`, the path already applied, the current
status and the currently allowed hrefs.

A forward dry run cannot prove later transitions because OpenProject computes
the form from the persisted status. It checks only the first step and reports
`planned_path`: the first step has `verified: true`, later progression steps
have `verified: false`, and no PATCH is sent. Apply checks those later steps
one at a time. An unknown path setting is a usage error.

RePlan owns descriptions of bound tasks once a person pushes them with
`scripts/push-openproject-description-drift.sh`. OpenProject description edits
are overwritten only by a reviewed apply, never by the default dry run. Review
the old and new lengths and short previews before setting `REPLAN_APPLY=1`.
Descriptions created by bootstrap contain a `RePlan task id` trailer line.
Text above that line is RePlan-owned. The trailer line and everything after it
are OpenProject-owned and are kept byte-for-byte on an update, including human
notes below the trailer. A trailer is also recognized at the start of a line
when older tooling joined human text after its closing backtick. The action
then reports `trailer_shares_line: true`, and warning
`task_id_trailer_shares_line` names the affected work packages in both dry-run
and apply output. The helper preserves that whole joined line and tail; it does
not repair the line because that decision belongs to a person. If no matching
trailer exists, the current OpenProject description is treated entirely as the
RePlan-owned prefix and is replaced by the canonical RePlan description; the
helper does not append a trailer.

## Start a plan of your own

The sample plan in `examples/small-team` shows every feature. To plan your
own work, create a project and fill it with commands; each write takes
`--apply` and is audited:

```sh
bin/replan init ~/plans/website --name "Website relaunch" --owner "Anna Lee" \
  --openproject-project-id website --apply
bin/replan resource add --project ~/plans/website --id resource.ai.coder \
  --class ai_agent --name "Coding agent" --with-actor limited \
  --actor actor.human.anna-lee --apply
bin/replan capacity set --project ~/plans/website --resource resource.human.anna-lee \
  --unit hours --amount 30 --actor actor.human.anna-lee --apply
```

The owner's ids come from `--owner` (`Anna Lee` gives `actor.human.anna-lee`;
Cyrillic letters are transliterated). Pass `--owner-id` to choose the id
yourself, for example `--owner-id operator`.

Then point `REPLAN_PROJECT` at it. Tasks come from `bin/replan task add` or
from an import of your tracker (`bin/replan import openproject|github`, then
`import accept`). The RePlan repository's `docs/quickstart.md` walks the same
path to a first weekly report.

## Daily commands

Plan and load, from the plan alone:

```sh
bin/replan validate "$REPLAN_PROJECT"
bin/replan next --project "$REPLAN_PROJECT"
bin/replan workload --project "$REPLAN_PROJECT" --week 2026-W42
bin/replan report state --project "$REPLAN_PROJECT" --format markdown
bin/replan evidence add --project "$REPLAN_PROJECT" --id evidence.example \
  --type manual_note --actor actor.human.operator \
  --observed-at 2026-10-26T10:00:00+07:00 --summary "Reviewed result." \
  --target task:task.example --dry-run --format json
```

Load commands use the latest matching approved estimate calibration. Keep
resource capacity in real units. Add `--raw` to `workload`, `report weekly`,
`report state` or `report dashboard` only when you need to compare with the
typed, uncalibrated estimates. Actuals are unchanged.

For a new project with no clean sample, preview a cited starting factor:

```sh
bin/replan estimate calibrate --project "$REPLAN_PROJECT" --prior 0.21 \
  --source "replan-self, 39 tasks, 2026-11-02" \
  --resource resource.ai.agent --unit agent_hours \
  --id calibration.start --name "Starting agent factor" \
  --approved-by actor.human.operator --dry-run --format markdown
```

Review it, then use `--apply`. `next` names it when eight clean project tasks
are ready to replace it with a measured calibration.

`next` separates actions waiting on a person, preparation an agent can do, and
tasks with human or human-pool hours planned in the selected week.

`report weekly`, `report state` and `report dashboard` accept `--scenario` for
an in-memory what-if. Their JSON carries `scenario_id`, and rendered views say
that the result is not the plan. A scenario weekly report cannot use `--write`;
a scenario dashboard may use `--output` only outside the project. The overlay
publisher also refuses any `raw/openproject_overlay.json` with `scenario_id`,
so a what-if cannot be published to OpenProject as the plan.

`bin/replan` with no arguments lists every command. In a shell,
`REPLAN_PROJECT` is set only if you export it; the helpers read it from
`.env.local` without that.

## Use RePlan as a tool

A project that plans with RePlan uses it like any installed tool: through
the `bin/replan` commands and the `scripts/*.sh` helpers of this kit, never
through RePlan's source code. Those surfaces are what each release keeps
working and what its release notes describe; anything else can change in
any commit without notice.

Get RePlan from a release:

- Use the agent kit of a tagged release, or for the CLI alone a release
  archive or `go install` of the RePlan module at a release tag, as the
  RePlan repository's README shows. `bin/replan version` says which build
  you run; write it down in your project's notes.
- Do not run helpers from a checkout of the RePlan repository. They read
  that checkout's own settings file and default plan, and they change with
  every commit.
- To upgrade, unpack the new kit beside the old one, move your `.env.local`
  into it, and keep the old directory until your dry runs pass.

Supported:

- `bin/replan` commands. With `--format json` each prints one JSON document
  with `ok`, `result`, `warnings` and `errors`, and every warning and error
  carries a stable `code`. Exit codes are listed in the CLI spec,
  `docs/spec/cli.md` in the RePlan repository.
- `scripts/*.sh` helpers. Each prints one JSON document, reads its settings
  from the environment or `.env.local` (or the file `REPLAN_ENV_FILE`
  names), and writes only with `REPLAN_APPLY=1`.
- The project files. Their format is described in
  `docs/spec/canonical-model.md` in the RePlan repository; your own tools may
  read them. Write them through commands.

Not supported:

- Importing RePlan's Go packages or calling the modules under `scripts/lib/`
  from your own code.
- Reading the settings file or token of RePlan or of another project. Each
  project keeps its own `.env.local` with its own OpenProject token.
- Writing the project's YAML from a generator of your own or rewriting
  whole files. Such writes skip the project lock, the audit log, the seals
  of baselines and calibrations, and the file style; a passing `validate`
  does not make them safe.
- Editing YAML by hand where a command exists. Hand edits remain only for
  what no command covers yet: availability windows, resource pools and
  removing a resource; run `bin/replan validate` after each.
- Calling the OpenProject API yourself for something a helper does.

A script of your own is fine when it only calls these commands and helpers
and reads their JSON, for example a list of tasks fed to `task add` one by
one.

| Need | Use |
| --- | --- |
| Start a plan | `bin/replan init`, `bin/replan resource add`, `bin/replan capacity set` |
| Add tasks | `bin/replan task add`, or an import of your tracker with `bin/replan import openproject` and `bin/replan import accept` |
| Change a task's title or description | `bin/replan task set-title`, `bin/replan task set-description`, then `scripts/push-openproject-description-drift.sh` |
| Change demands or move work to another resource | `bin/replan demand add`, `bin/replan demand set`, `bin/replan demand move` |
| Add or change dependencies and add deadlines | `bin/replan dependency add`, `bin/replan dependency set`, `bin/replan deadline add`; each accepts one item or a `--from-file` batch |
| Set dates | `bin/replan task set-dates`, then `scripts/push-openproject-date-drift.sh` |
| Draft dates from capacity and load | `bin/replan taskjuggler run`, `bin/replan taskjuggler import-result`, then review the scenario with `bin/replan scenario diff` and `bin/replan scenario apply` (needs TaskJuggler's `tj3`) |
| Record work and accept a task | `bin/replan actual add-consumption`, `bin/replan actual add-status`, `scripts/accept-task.sh` |
| Create work packages and relations | `scripts/apply-openproject-bootstrap.sh`, `scripts/apply-openproject-relations.sh` |
| Change one work package | `scripts/openproject-work-package-toolkit.sh` for status, dates, text, metadata and Version (stage); list or create project versions with `version list` or `version create` |
| Read and edit the project wiki | `scripts/openproject-wiki.sh` |
| Bring files written by an older tool into the current style | `bin/replan project format` |

For drafted dates, use the scenario id returned by `taskjuggler import-result
--write-candidate`: inspect it with `scenario diff --format markdown`, preview
`scenario apply --dry-run`, and then repeat with `--apply`, a human `--actor`
and a `--reason`. Apply accepts the linked open TaskJuggler proposal in the
same audited write. If the proposal was accepted first, apply still works; if
it was rejected, apply refuses.

When something is missing, do not work around RePlan's internals. Open an
issue in the RePlan repository, or tell whoever gave you this kit, saying
what you need, which command you tried and what its JSON said. Until the command exists,
keep the workaround small and in your own repository, use your own token,
and note it so it can be removed later.

## Surface map

Use one primary surface for each loop step. Commands in the kit are relative
to its root. A write to OpenProject or the bindings file is a dry run unless
`REPLAN_APPLY=1` is set.

| Loop step | Command | Note |
| --- | --- | --- |
| Inspect kit binary | `bin/replan version` | Reports the kit version, source commit, reproducible build date, toolchain, target and supported project format without loading a project. |
| Plan and validate | `bin/replan validate "$REPLAN_PROJECT"` | Change the plan with the commands below, validate it, then use `next`, `workload` and `report` to inspect it. Hand edits only where no command exists; see "Use RePlan as a tool". |
| Upgrade the RePlan file format | `bin/replan project migrate --project "$REPLAN_PROJECT" --actor <id> --dry-run` | Reviews the ordered format steps and planned writes; replace `--dry-run` with `--apply`. It never downgrades. |
| Format canonical YAML | `bin/replan project format --project "$REPLAN_PROJECT" --actor <id> --reason <text> --dry-run` | Lists files and changed-line counts; replace `--dry-run` with `--apply` after review. It refuses any data-changing round trip. |
| Recover an interrupted RePlan write | `bin/replan project recover --project "$REPLAN_PROJECT" --dry-run` | Lists and validates the pending file set without writing; replace `--dry-run` with `--apply` to roll it forward under the project lock. |
| Create a RePlan task | `bin/replan task add ... --dry-run` | Creates the task, demands and current-plan reference together; review, then use `--apply`. It does not create the OpenProject work package. |
| Change task scheduling metadata | `bin/replan task set-scheduling ... --dry-run` | Changes priority and lead time with old/new audit evidence; review, then use `--apply`. |
| Add, change or move a demand | `bin/replan demand add\|set\|move ... --dry-run` | Updates the demand and task reference together and records an audit entry. Add and set also accept all-or-nothing `--from-file` batches. Review before `--apply`. |
| Add or change dependencies | `bin/replan dependency add ... --dry-run`, `bin/replan dependency set <id> ... --dry-run` | Adds or changes one dependency or an all-or-nothing `--from-file` batch, with one audit record per changed item. Set changes status, required state or criticality; an all-unchanged set is a no-op. `--after` on `task add` only sets plan order. |
| Add and inspect deadlines | `bin/replan deadline add ... --dry-run`, `bin/replan deadline list ...` | Adds one deadline or an all-or-nothing batch, then lists canonical deadlines as JSON or Markdown. |
| Seal a planning baseline | `bin/replan baseline create ... [--task <id> ...] --dry-run` | Snapshots current task dates, scheduling metadata, planned demand amounts and levelled flags. Use repeatable `--task` for newly planned work; review, then use `--apply`. Use `baseline list` or `show` to verify the seal. |
| Compare estimates with actuals | `bin/replan report estimates --project "$REPLAN_PROJECT" --baseline <id>` | Read-only resource and split-group summary followed by baseline-task demand, split and schedule detail; resources without counted actuals are explicit. JSON is available with `--format json`. |
| Calibrate estimates | `bin/replan estimate calibrate ... [--all-baselines] [--by-size] --dry-run` | Builds a sealed calibration from at least eight clean finished tasks. For a new project, use `--prior 0.21 --source "replan-self, 39 tasks, 2026-11-02"`. Review the source, exclusions and factors, then replace `--dry-run` with `--apply`; the approver must be human. |
| Apply a calibration as a what-if | `bin/replan estimate apply --calibration <id> --scenario-id <id> --actor <id> --dry-run` | Creates one calibration-linked scenario on apply. Inspect it with `scenario diff`, then use the existing reviewed `scenario apply` flow. |
| Create or bind a work package | `scripts/apply-openproject-bootstrap.sh` | Creates missing work packages and bindings. For one item, use toolkit `work-package create --task <id> --bind`; omit `--bind` to create first and receive the exact follow-up `work-package bind` command. Use `backlog reconcile` to rebuild bindings. |
| Relations to OpenProject | `scripts/apply-openproject-relations.sh` | Applies supported canonical dependencies as OpenProject relations. The toolkit can list or delete a single relation. |
| Record evidence | `bin/replan evidence add ... --dry-run` | Review the selected week file and item, then replace `--dry-run` with `--apply`; the actor and every target must already exist. |
| Set remaining work | `bin/replan actual set-remaining ... --dry-run` | Updates one demand, or an all-or-nothing `--from-file` batch under the top-level `demands` key. Batch rows contain `demand`, `amount`, `unit` and optional item `reason`; review the shared actor, evidence, timestamp and reason before `--apply`. |
| Rename a resource or actor | `bin/replan resource set-name ... --dry-run`, `bin/replan actor set-name ... --dry-run` | Changes only the resource name or actor display name. The actor target uses `--actor-id`; review, then replace `--dry-run` with `--apply`. |
| Actuals and acceptance | `scripts/accept-task.sh` | Accepts one review-ready task through evidence, actuals, dependency candidates and its one OpenProject work package. It is dry-run by default and resumable. Use individual `bin/replan actual ... --dry-run` commands for facts that are not task acceptance. |
| Correct a mis-booked actual | `bin/replan actual correct --project "$REPLAN_PROJECT" --actual <actual-id> (--amount <n>\|--void) --reason <text> --actor <id> --evidence <id> --dry-run` | Appends an audited correction; the original remains unchanged. Review the effective amount, then repeat with `--apply`. Later corrections supersede earlier corrections of the same original. |
| Replace stale planned dates with actuals | `bin/replan task record-actual-window ... --dry-run` | Derives one finished task's actual window from status audit facts and current-demand consumption actuals, then writes both dates with one audit record. Review before `--apply`. |
| Status to OpenProject | `scripts/push-openproject-status-drift.sh` | Pushes reviewed RePlan tracking status. Direct transitions are the default; export `REPLAN_OPENPROJECT_STATUS_PATH=forward` to check and apply allowed forward intermediate statuses one at a time. |
| Status from OpenProject | `bin/replan import accept <snapshot-id> --accept-tracking-status --dry-run` | Review a stored import snapshot; apply with `--apply --actor <id>`. The soft-drift helper does not accept status. |
| Import GitHub Issues | `bin/replan import github --project "$REPLAN_PROJECT" --profile <id> --dry-run` | A profile names `repository` and `token_env`; explicit `--repository` is also available. File replay uses `--from-file`. Review candidates, omit `--dry-run` to stage the snapshot, then use the shared `import accept` modes. No GitHub write-back is implemented. |
| Planned dates to OpenProject | `scripts/push-openproject-date-drift.sh` | Toolkit `work-package update-dates` is the single-work-package surface. |
| Dates from OpenProject | `bin/replan task set-dates ... --dry-run` | Reviewed one-task acceptance. For a stored snapshot use `bin/replan import accept <snapshot-id> --accept-dates --dry-run`. The live batch helper keeps private-binding matching, then writes audited `task set-dates` changes and requires `REPLAN_ACTOR` with `REPLAN_APPLY=1`. |
| Change and push a task description | `bin/replan task set-description ... --dry-run`, then `scripts/push-openproject-description-drift.sh` | The CLI changes the audited canonical description. The helper previews bound work-package differences, preserves an existing bootstrap trailer and all OpenProject text from it byte-for-byte (also flagging text joined to its line), and pushes only with `REPLAN_APPLY=1`; without a trailer it replaces the whole description and does not append one; `REPLAN_TASKS` optionally limits the task ids. |
| Correct subject or description | `scripts/openproject-work-package-toolkit.sh work-package update-text ...` | Shows old/new text and the current lock version. A bound subject is preflighted and synchronized with audited `bin/replan task set-title`; an OpenProject description is not copied into the separate RePlan planning note. |
| Create an OpenProject Version (stage) | `scripts/openproject-work-package-toolkit.sh version create --name <exact-name> ...` | Refuses an exact existing name, previews the API payload, and creates only with `REPLAN_APPLY=1`. The token user needs the project's Manage versions permission. |
| Set or clear one work package's Version (stage) | `scripts/openproject-work-package-toolkit.sh work-package update-metadata --task <id> --version <id-or-exact-name>` | Use `version list` to inspect the selected project's versions; use `--clear-version` to remove one. Both are dry-run first and use the work package's current lock version on apply. |
| Drift check | `scripts/check-openproject-drift.sh` | Read-only comparison; a person chooses the direction. |
| Overlay build and publish | `scripts/build-openproject-live-overlay.sh`, then `scripts/publish-openproject-overlay.sh` | Builds locally with private bindings, then publishes only to the selected project's directory. Publish is a dry run unless `REPLAN_APPLY=1`; the private target comes only from the environment or ignored `.env.local`. |
| Wiki | `scripts/openproject-wiki.sh page list` | Use its page and attachment subcommands; writes are gated. |
| Reports | `scripts/openproject-operator-report.sh` | Combined live report. Use `bin/replan report ...` for local state, weekly, estimate-accuracy and dashboard reports. |

## Accept a completed task

`scripts/accept-task.sh` is the composite acceptance surface for one task. It
prints the ordered plan by default. With `REPLAN_APPLY=1`, it records the
operator's decision evidence, optional CI minutes, zeros and consumes every
unfinished demand, marks the task accepted, records its actual window, writes
the changed dates and status to only the bound OpenProject work package,
writes dependency proposal candidates for separate review, and refreshes the
overlay when those scripts and a
publish target are available. It never accepts a proposal automatically.

Required settings are `REPLAN_PROJECT`, `REPLAN_TASK`,
`REPLAN_ACCEPTED_BY`, `REPLAN_ACCEPT_SUMMARY` and the OpenProject toolkit's
`REPLAN_OPENPROJECT_PROJECT_ID`. Apply also requires `REPLAN_ACTOR`, the
recorder of the actual commands. `REPLAN_TASK`, `REPLAN_ACCEPTED_BY`,
`REPLAN_ACTOR` and the `REPLAN_ACCEPT_*` settings describe one acceptance, so
the helper reads them from the process environment only, never from
`.env.local`: a value left in the file cannot accept the wrong task later.
Optional acceptance settings are:

- `REPLAN_ACCEPT_EVIDENCE_ID`, default
  `evidence.<namespace>.accept-<task-slug>`;
- `REPLAN_ACCEPT_AT`, default the current RFC3339 time with an offset. UTC is
  fine: RePlan derives acceptance and consumption dates in the project's time
  zone;
- `REPLAN_ACCEPT_CI_MINUTES`, default `0`;
- `REPLAN_BIN`, default the kit's or repository's locally built binary.

The OpenProject base URL, token selection, private bindings, proxy headers and
request retry settings are the same as for
`scripts/openproject-work-package-toolkit.sh`. The acceptance helper refuses
an apply without the selected token before it changes RePlan. In a dry run a
missing token leaves the OpenProject step `skipped` with
`token_env_not_set`; the OpenProject date step is skipped too. Overlay
publication additionally needs `REPLAN_OVERLAY_PUBLISH_TARGET`; without it
step 10 is `skipped` with
`accept_overlay_publish_target_not_set`. A partial copy that lacks either
overlay script instead reports `accept_overlay_scripts_unavailable`.

```sh
export REPLAN_TASK=task.example.finish-report
export REPLAN_ACCEPTED_BY=actor.human.owner
export REPLAN_ACCEPT_SUMMARY="Owner reviewed and accepted the report."

scripts/accept-task.sh
REPLAN_ACTOR=actor.human.recorder REPLAN_APPLY=1 scripts/accept-task.sh
```

Every plan action has a step number, state, reason and secret-free argv list.
Pending RePlan mutations in steps 1–6 also include the real CLI dry-run
answer. Steps are: 1 acceptance evidence, 2 optional CI minutes, 3 zero
remaining, 4 close demands, 5 accept tracking status, 6 record actual window,
7 update OpenProject dates, 8 suggest dependencies, 9 update OpenProject
status, and 10 refresh the overlay. An unknown actual window skips steps 6
and 7 with a warning and does not fail acceptance. If an apply stops after a
write, the envelope exits 7 and includes
`applied_so_far` plus `failed_step`; rerun the same command to recompute state
and continue. New dependency proposal ids appear in `new_proposal_ids` and in
`operator_next_steps`, with the separate `replan proposal accept ... --apply`
review command.

Steps 1–9 derive completion from canonical RePlan and live OpenProject state.
The publish step is the sole exception: rsync does not expose
an acceptance fact in either system, so a successful publish records an
ignored `.accept-task-publish.json` marker in `REPLAN_OVERLAY_OUTPUT_DIR`.
It stores task/evidence/time plus a hash of the target, never the private
target itself. Deleting the disposable overlay output only causes a safe,
idempotent rebuild and republish on the next acceptance run.

## Refresh the RePlan pages in OpenProject

Build the overlay from the plan and its private work-package bindings, review
the publish preview, then apply it:

```sh
scripts/build-openproject-live-overlay.sh
scripts/publish-openproject-overlay.sh
REPLAN_APPLY=1 scripts/publish-openproject-overlay.sh
```

The build uses `REPLAN_PROJECT` and `REPLAN_OPENPROJECT_PROJECT_ID`. It reads
`REPLAN_OPENPROJECT_BINDINGS_PATH`, defaulting to
`private/openproject/<project>-bindings.yaml` under the kit root, and writes
`REPLAN_OVERLAY_OUTPUT_DIR`, defaulting to
`tmp/openproject-live-overlay/latest` under the kit root. The output
directory must be outside the plan directory: the build reads the plan
through the read-only bindings layer, and a path inside the plan is refused as
`bindings_layer_output_in_project`, which the build reports as its own error
code next to `dashboard_failed`. The publish helper
requires the private `REPLAN_OVERLAY_PUBLISH_TARGET`, an rsync local path or
`host:path` naming the OpenProject server's overlay directory, and sends only
to `<target>/<project id>/`. Both helpers read the environment and the kit's
ignored `.env.local`; environment values win. Publishing requires `rsync` and
never writes without `REPLAN_APPLY=1`.

Put a new plan into OpenProject, then its relations:

```sh
scripts/apply-openproject-bootstrap.sh    # work packages, then the bindings file
scripts/apply-openproject-relations.sh    # dependencies as relations
```

The plan needs an OpenProject source profile whose `project_id` matches
`REPLAN_OPENPROJECT_PROJECT_ID`. Bootstrap warns early if it is missing or
different; overlay build refuses before writing. For a new plan, create it
with `init --openproject-project-id <id>`. For an existing plan, add this under
`project.source_profiles` in `.replan/project.yaml`:

```yaml
- id: openproject.website
  source: openproject
  base_url: https://op.example.internal
  project_id: website
  token_env: OPENPROJECT_TOKEN
```

`https://op.example.internal` is a placeholder. The helpers take the
address from their own settings (`REPLAN_OPENPROJECT_BASE_URL`) and ignore
it. `bin/replan import openproject --profile <id>` reads the profile, so
before you import by profile, set `base_url` to your OpenProject address,
or pass `--base-url`, which wins over the profile.

Only `finish_to_start`, `blocks` and `relates` dependencies become
OpenProject relations; the other types stay in the plan. Each other
dependency is refused as `unsupported_dependency_type`, with its id and the
supported types, and gets no relation. While any dependency is refused, the
other relations are still created but the private bindings file is not
written; change the type (for example to `relates`) and run again.

The relations dry run warns when planned `follows` relations may move
successor dates. After apply, run `scripts/check-openproject-drift.sh`; if the
reviewed RePlan dates must be restored, review
`scripts/push-openproject-date-drift.sh` and only then apply that push.

Bootstrap merges new task and assignee bindings into the private bindings
file; it preserves existing relations, bindings and extension keys, and refuses
another project's file with `bindings_project_mismatch` (exit 5).

Bootstrap matches each task by its private binding first, then by subject when
there is no binding. If a bound work package has a different subject, the dry
run plans `update_subject`; apply changes only its subject with the current lock
version and reports `updated_subject`. If the bound id is not in the target
project, bootstrap reports `bound_work_package_missing` and does not create a
replacement. Remove the stale binding if bootstrap should create a new work
package.

Work packages are assigned to the token's own user unless
`REPLAN_OPENPROJECT_ASSIGNEE_USER_ID` names another member. With
`REPLAN_OPENPROJECT_ASSIGNEE_RESOURCE_ID` set, bootstrap also binds that user
to the RePlan resource it names. An API-only agent account is often a project
member without permission to be assigned work packages, so it usually needs
`REPLAN_OPENPROJECT_ASSIGNEE_USER_ID` set to an assignable project member.
Bootstrap checks the project's available assignees: a dry run warns with
`default_assignee_not_assignable` or `assignee_not_assignable`, and an apply
refuses with the same code before writing anything. If that read is forbidden,
it warns `assignee_check_unavailable`, and a create that OpenProject still
refuses on the assignee fails with `assignee_refused`.

`scripts/explain-resource-overload.sh` explains one resource's weekly load
and needs `REPLAN_RESOURCE_ID` and `REPLAN_UNIT`, for example
`resource.human.anna` and `hours`. `scripts/review-resource-pool.sh` sorts
your resources into the buckets of `review/resource-pool.yaml` in the plan,
if you keep one; without it every resource needs your decision.

Compare the plan and OpenProject, then run the reviewed exchanges:

```sh
scripts/check-openproject-drift.sh
scripts/push-openproject-status-drift.sh
scripts/push-openproject-date-drift.sh
scripts/push-openproject-description-drift.sh
scripts/openproject-operator-report.sh
```

Work on single work packages:

```sh
scripts/openproject-work-package-toolkit.sh work-package inspect --task <task id>
scripts/openproject-work-package-toolkit.sh work-package create --subject "..." --type-name Task
scripts/openproject-work-package-toolkit.sh work-package create --task <task id> --bind --subject "..." --type-name Task
scripts/openproject-work-package-toolkit.sh work-package bind --task <task id> --work-package <number>
scripts/openproject-work-package-toolkit.sh work-package update-status --task <task id> --status "In progress"
scripts/openproject-work-package-toolkit.sh work-package update-dates --task <task id> --start-date 2026-10-12 --due-date 2026-10-14
scripts/openproject-work-package-toolkit.sh work-package update-text --task <task id> --subject "Corrected subject" --evidence <evidence id>
scripts/openproject-work-package-toolkit.sh version list
scripts/openproject-work-package-toolkit.sh version create --name "Stage 3" --description "Third delivery stage" --start-date 2026-11-16 --due-date 2026-11-27
scripts/openproject-work-package-toolkit.sh work-package update-metadata --task <task id> --version "Stage 2"
scripts/openproject-work-package-toolkit.sh work-package update-metadata --task <task id> --clear-version
scripts/openproject-work-package-toolkit.sh comment add --task <task id> --text "..."
scripts/openproject-work-package-toolkit.sh comment list --task <task id>
scripts/openproject-work-package-toolkit.sh status list
scripts/openproject-work-package-toolkit.sh user list
scripts/openproject-work-package-toolkit.sh relation list
```

`comment add` writes with
`POST /api/v3/work_packages/{id}/activities`, requires the returned
`Activity::Comment` to contain the sent text, and reports its `activity_id` and
`created_at`. Read the activities back with `comment list` and verify that the
reported id has the exact comment text; the live agent-kit check performs this
after every comment write. Releases up to v0.5.0 reported comments as saved
even when OpenProject 17.6 silently dropped them.

Successful toolkit creates put the created identity in exactly one `items`
row: `kind`, `id`, `href` and the identifying fields for a work package,
relation, comment activity or Version. The existing action-specific and
top-level ids remain for compatibility. Dry runs and failed writes keep
`items` empty; an idempotent `relation create` returns the existing relation
as its one item.

The other toolkit and wiki updates also compare OpenProject's answer with what
they sent: work-package status, dates, estimate, version, assignee, type,
parent, subject and description; a newly created Version's name, description,
dates, status and sharing; and wiki page text. Equivalent forms count as saved
(`PT1H30M` and `PT1.5H`, CRLF and LF, trailing whitespace). A field missing
from the answer fails the command with `work_package_fields_not_saved`,
`version_fields_not_saved` or `wiki_page_fields_not_saved` and names it in
`unconfirmed_fields`; read the work package, Version or page back before
retrying.

Before `work-package update-status`, `update-dates` or `update-metadata` plans
or applies a write, it compares every requested field with the current work
package. If all fields already match, the answer has `no_op: true`, keeps the
descriptive action with `unchanged_fields`, reports zero planned and applied
actions, and sends neither a PATCH nor a status workflow-form request. If only
some fields match, the PATCH contains only the differing fields plus the
current `lockVersion`; write-answer verification covers that smaller payload.
Duration, text, boolean and HAL-link normalization is the same before and
after a write, including an absent link matching a requested null link when a
Version or parent is already cleared.

Fixture dry runs use the values present in the fixture for the same comparison.
A live-shaped fixture with HAL link hrefs can therefore prove the same no-op
behavior, and an apply-mode no-op is safe and succeeds without a write. A real
fixture-mode change is still refused because fixtures cannot be mutated.

### Use the toolkit without a plan

For an OpenProject-only project, omit `REPLAN_PROJECT`. Keep
`REPLAN_OPENPROJECT_PROJECT_ID` set. The toolkit then reports `"plan":
"none"`, omits `private_binding_layer`, and does not read or write a bindings
file.

The catalog commands `status list`, `type list`, `user list`, `priority list`
and `version list` work in this mode. So does `version create`; it still needs
the project id and a token with Manage versions permission, but not a RePlan
plan. The commands `work-package list`, `work-package inspect --work-package
<id>`, `work-package create` without `--task`, every `work-package update-*
--work-package <id>` command, and `comment add` or `comment list` with
`--work-package <id>` also work. Create still rejects an existing subject
unless `--allow-duplicate` is supplied.

Commands that select a RePlan task or otherwise need plan bindings still
require `REPLAN_PROJECT`: `work-package bind`, all `--task` forms, `backlog
reconcile`, `work-package replan-links`, and `relation list` without
`--work-package`.

Relations can be managed directly by OpenProject id without a plan:

```sh
scripts/openproject-work-package-toolkit.sh relation create --from 1234 --to 5678 --type relates --description "Same delivery topic"
scripts/openproject-work-package-toolkit.sh relation list --work-package 1234
scripts/openproject-work-package-toolkit.sh relation delete --relation 9012
```

`relation create` accepts OpenProject's `relates`, `blocks`, `blocked`,
`duplicates`, `duplicated`, `includes`, `partof`, `requires`, `required`,
`follows` and `precedes` types. It is idempotent for the same endpoints and
type; `relates` also matches the reverse endpoint order. `follows` and
`precedes` can make the successor automatically scheduled and move dates in
OpenProject 17.6. The toolkit snapshots both endpoint work packages and, after
creation, restores manual scheduling and their original dates when OpenProject
changed them. The dry run calls out this risk and the planned restoration.

For plan-backed relation apply, RePlan `relates` maps to OpenProject `relates`.
It is a symmetric, non-scheduling link: an existing reverse-endpoint relation
matches, no duplicate is created, and the helper does not run scheduling/date
restoration for it. Private dependency bindings may store `relation_type:
relates` like the existing `follows` and `blocks` bindings.

OpenProject permits only one relation, of any type, between a work-package
pair. If the live project already has another relation type on the pair, or
the plan contains two dependency types for the same pair, relation apply plans
only the first and reports `openproject_pair_already_related` for the skipped
dependency instead of attempting a POST that OpenProject would reject.

Set or clear a parent directly by work-package id:

```sh
scripts/openproject-work-package-toolkit.sh work-package update-metadata --work-package 5678 --parent 1234
scripts/openproject-work-package-toolkit.sh work-package update-metadata --work-package 5678 --clear-parent
```

If OpenProject refuses the parent field, `openproject_permission_missing`
means the member's role needs permission to manage work package hierarchies
(subtasks).

Protected credential files work here too:

```sh
REPLAN_OPENPROJECT_TOKEN_FILE=/run/secrets/openproject-token \
REPLAN_OPENPROJECT_PERIMETER_KEY_FILE=/run/secrets/openproject-perimeter-key \
scripts/openproject-work-package-toolkit.sh work-package list
```

`update-status --status` accepts either a RePlan tracking status
(`not_started`, `ready`, `in_progress`, `blocked`, `review_ready`, `accepted`,
`cancelled`) or its exact OpenProject name (`New`, `To be scheduled`,
`In progress`, `On hold`, `Developed`, `Closed`, `Rejected`).

`version list` reads every page of the selected project's versions and reports
each version's id, exact name, description, start and due dates, status and
sharing mode. `version create --name <name>` previews one create action and
requires `REPLAN_APPLY=1` to write. Optional `--description`, `--start-date`,
`--due-date`, `--status` and `--sharing` values are omitted from the request
when absent, so OpenProject's defaults apply. The command rejects dates that
are not real `YYYY-MM-DD` dates and a due date before the start date, and refuses an exact existing name with `version_exists` before
writing. OpenProject 17 creates through `POST /api/v3/versions`; the payload's
`_links.definingProject` selects this project. A successful apply reports the
returned version id and href; a token without Manage versions permission gets
`version_create_forbidden`, and a validation refusal from OpenProject gets
`version_refused` with its messages. If a POST outcome is unknown, run
`version list` before retrying.

`work-package create` and `work-package update-metadata` accept
`--version <id-or-exact-name>`;
`update-metadata --clear-version` removes it. A duplicate exact name is refused
with `version_ambiguous` and the matching ids, and an unknown id or name with
`version_not_found`. Dry runs show `from_version` and `to_version`; apply still
requires `REPLAN_APPLY=1` and sends the work package's current `lockVersion`.
OpenProject remains authoritative for refusing a closed or locked version.
With `--task` but without `--bind`, create reports the exact `work-package
bind` command in its next steps, using `<id>` in a dry run and the returned id
after apply. `work-package create --task <id> --bind` checks that the RePlan
task exists, is unbound and has a configured private binding layer before any
POST; apply then creates and writes the binding. If that local write fails,
the answer keeps the created work-package id, exits nonzero and gives the
exact bind command to finish without creating the work package again.

`update-text` accepts `--subject`, `--description` or `--description-file`,
with at least one required. Its dry run reports each changed field's old and
new text, the current `lockVersion`, and an exact apply command containing
`--lock-version <current>`. Review that output, then rerun the command with
`REPLAN_APPLY=1`. If the live version no longer matches, it refuses before the
PATCH; a concurrent change during the PATCH is the same
`openproject_work_package_changed` refusal. Omitting `--lock-version` on apply
is supported for compatibility but emits `lock_version_not_checked`.

For a bound work package, changing `--subject` also requires `REPLAN_ACTOR` and
`--evidence`. The helper preflights `replan task set-title --dry-run`, patches
OpenProject, then applies the RePlan title. If that last write fails it exits 7
and prints the exact command that finishes the RePlan side. Description text
is deliberately not synchronized: OpenProject owns the work-package
description, while the RePlan task description remains its planning note.
If the requested subject already matches OpenProject but RePlan still reports
`title_drift`, the helper skips the PATCH and applies only `task set-title`;
when both titles match it reports an unchanged no-op. For this bound-title
step it uses `REPLAN_BIN` when set, then the kit's `bin/replan`, otherwise it
builds a temporary checkout binary and removes it when the helper exits.

Lost the bindings file, or working on a second machine? Rebuild it from
OpenProject by matching titles:

```sh
scripts/openproject-work-package-toolkit.sh backlog reconcile
REPLAN_APPLY=1 scripts/openproject-work-package-toolkit.sh backlog reconcile
```

For a newly created single work package, prefer `work-package create --task
<id> --bind`. If it was created without `--bind`, or its binding write failed,
run the exact `work-package bind --task ... --work-package ...` command in the
create answer instead of running create again.

## The project wiki

`scripts/openproject-wiki.sh` reads and writes the wiki of the project in
`REPLAN_OPENPROJECT_PROJECT_ID`, as your own user. It needs the OpenProject
wiki API plugin on the server; without it page commands answer 404.

Read:

```sh
scripts/openproject-wiki.sh page list
scripts/openproject-wiki.sh page show --id 28 --output page.md
scripts/openproject-wiki.sh page show --title "Exact page title"
scripts/openproject-wiki.sh attachment list --id 28
scripts/openproject-wiki.sh attachment download --attachment 15 --output file.tar.gz
```

`page list` gives every page with its id, title, parent and lock version.
`page show` gives the page's facts and attachments; its text goes to the
`--output` file, or into the `text` field of the JSON without it. A page is
named by `--id`, by `--slug` (as in its address) or by its exact `--title`.
`attachment download` reports the SHA-256 of what it wrote.

Write, always a dry run first:

```sh
scripts/openproject-wiki.sh page create --title "New page" --text-file new.md --parent-id 9
scripts/openproject-wiki.sh page update --id 28 --text-file page.md --diff-output page.diff
scripts/openproject-wiki.sh page update --id 28 --parent-id 9
scripts/openproject-wiki.sh page delete --id 31
scripts/openproject-wiki.sh attachment upload --id 28 --file notes.pdf
scripts/openproject-wiki.sh attachment upload --id 28 --file notes.pdf --replace
scripts/openproject-wiki.sh attachment delete --attachment 15
```

Each of these changes the wiki only with `REPLAN_APPLY=1` in front. The page
text is OpenProject's Markdown; `--text-file` holds the whole new text, not a
patch.

To edit a page safely, work in three steps:

1. `page show --id N --output page.md`, and note `lock_version` in the output.
2. Edit `page.md`.
3. `page update --id N --text-file page.md --expect-lock-version <that version>
   --diff-output page.diff`: read the dry run and the diff, then repeat with
   `REPLAN_APPLY=1`.

If someone changed the page in between, the update is refused with
`wiki_page_changed` and nothing is overwritten: read the page again and redo
the edit on the new text. Without `--expect-lock-version` the helper checks
only against a change during the write itself.

What the helper refuses, each with its own error code: a second page of an
existing title (`wiki_page_title_taken`), deleting a page that has child
pages (`wiki_page_has_children`), a second attachment of an existing name
unless `--replace` is given (`attachment_name_taken`), and a title that names
several pages (`wiki_page_ambiguous`). Deleting a page removes its history
and attachments too and cannot be undone.

Permissions are OpenProject's own: reading needs `view_wiki_pages`; creating,
editing and attachments need `edit_wiki_pages`; deleting a page and editing a
protected page need `manage_wiki`. A refusal is reported as
`wiki_permission_denied`.

## What works with an ordinary member's token

Checked against a real OpenProject 17.6 with the token of a user who holds
only the standard "Member" role in the project, and again with "Project
admin": creating the plan's work packages and relations, the bindings file and
its rebuild, the drift check, the status and date pushes, the operator report,
every toolkit command above, and every wiki command: pages created, read,
updated and deleted, attachments uploaded, downloaded, replaced and deleted.
A user with the "Reader" role reads the wiki and is refused every change. The
status a work package may move to is
decided by the project's workflow for your role; a refused move is reported as
`openproject_workflow_transition_not_allowed` and nothing is written.

## What needs an administrator

Ask the OpenProject administrator, once:

- to create the project and add you to it;
- to give your role the permissions your work needs;
- for the link from each work package to its RePlan page: to create the
  "RePlan" link custom field and enable it for the project, after which
  `work-package replan-links` fills it;
- for the RePlan pages inside OpenProject: to install the RePlan plugin and
  enable its module for the project.

## If something fails

- `missing_setting` names `REPLAN_ENV_FILE`: the explicit settings file does
  not exist or cannot be read. Its path is not printed.
- `mixed_project_settings`: the environment project id and settings-file
  project id differ, while the helper would use listed per-project values
  from that file. Use the right project's kit or settings file.
- `token_env_not_set` among `warning_codes`, with an empty result or
  `"mode": "skipped"`: `.env.local` is missing or has no `OPENPROJECT_TOKEN`.
  With `REPLAN_APPLY=1` the same is an error, exit 8, and nothing was written.
- `openproject_http_error` with status 401 on every request, even a read: a
  reverse proxy may be answering instead of OpenProject; see "Behind a
  reverse proxy".
- `openproject_http_error` with status 401: the token is wrong or revoked;
  403: your role lacks the permission; 404: the project identifier is wrong
  or you are not its member.
- `missing private binding for RePlan task`: run `backlog reconcile`.
- `ModuleNotFoundError: No module named 'yaml'`: install PyYAML.
