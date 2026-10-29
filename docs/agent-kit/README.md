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

## What you need

- Linux on x86-64, `sh` and `bash`;
- Python 3 with PyYAML (`python3 -c 'import yaml'` must work);
- `rsync`, for publishing the RePlan overlay to OpenProject;
- an OpenProject account that is a member of your project, and an API token
  of that account: in OpenProject, "My account", "Access tokens", "API";
- a RePlan project directory, the plan. Keep it in a version-control
  repository of your own.

No administrator is needed for daily work. An OpenProject administrator is
needed once, for what the list under "What needs an administrator" names.

## Set up

1. Unpack the archive anywhere and stay in its directory.
2. Copy `env.local.example` to `.env.local` and fill it in. `.env.local` holds
   your token: never commit it and never print it.
3. Check the program and the connection:

   ```sh
   bin/replan validate "$PWD/examples/small-team"
   scripts/openproject-work-package-toolkit.sh work-package list
   ```

   The first prints warnings of the example plan and no errors. The second
   prints a JSON document with `"ok": true` and your project's work packages.

Every helper reads `.env.local` itself; settings already in the environment
win over the file. The helpers have no default plan or project: one that
needs `REPLAN_PROJECT` or `REPLAN_OPENPROJECT_PROJECT_ID` and does not find it
answers `missing_setting`, exit 8, with the names in `missing_settings`,
instead of working on another project.

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

## The rule every helper follows

Nothing writes canonical state without `--apply` or `REPLAN_APPLY=1`.
Helpers default to a dry run; CLI commands refuse to guess and require
`--dry-run` or `--apply` unless `REPLAN_APPLY=1` supplies the apply choice.
An explicit `--dry-run` remains a preview even when `REPLAN_APPLY=1` is set.

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

`report weekly`, `report state` and `report dashboard` accept `--scenario` for
an in-memory what-if. Their JSON carries `scenario_id`, and rendered views say
that the result is not the plan. A scenario weekly report cannot use `--write`;
a scenario dashboard may use `--output` only outside the project. The overlay
publisher also refuses any `raw/openproject_overlay.json` with `scenario_id`,
so a what-if cannot be published to OpenProject as the plan.

`bin/replan` with no arguments lists every command. In a shell,
`REPLAN_PROJECT` is set only if you export it; the helpers read it from
`.env.local` without that.

## Surface map

Use one primary surface for each loop step. Commands in the kit are relative
to its root. A write to OpenProject or the bindings file is a dry run unless
`REPLAN_APPLY=1` is set.

| Loop step | Command | Note |
| --- | --- | --- |
| Inspect kit binary | `bin/replan version` | Reports the kit version, source commit, reproducible build date, toolchain, target and supported project format without loading a project. |
| Plan and validate | `bin/replan validate "$REPLAN_PROJECT"` | Edit the canonical plan, validate it, then use `next`, `workload` and `report` to inspect it. |
| Upgrade the RePlan file format | `bin/replan project migrate --project "$REPLAN_PROJECT" --actor <id> --dry-run` | Reviews the ordered format steps and planned writes; replace `--dry-run` with `--apply`. It never downgrades. |
| Create a RePlan task | `bin/replan task add ... --dry-run` | Creates the task, demands and current-plan reference together; review, then use `--apply`. It does not create the OpenProject work package. |
| Seal a planning baseline | `bin/replan baseline create ... --dry-run` | Snapshots current task dates and planned demand amounts; review, then use `--apply`. Use `baseline list` or `show` to verify the seal. |
| Compare estimates with actuals | `bin/replan report estimates --project "$REPLAN_PROJECT" --baseline <id>` | Read-only resource and split-group summary followed by baseline-task demand, split and schedule detail; resources without counted actuals are explicit. JSON is available with `--format json`. |
| Calibrate estimates | `bin/replan estimate calibrate ... --dry-run` | Builds a sealed calibration from at least eight clean finished tasks. Review explicit exclusions and the factor, then replace `--dry-run` with `--apply`; the approver must be human. |
| Apply a calibration as a what-if | `bin/replan estimate apply --calibration <id> --scenario-id <id> --actor <id> --dry-run` | Creates one calibration-linked scenario on apply. Inspect it with `scenario diff`, then use the existing reviewed `scenario apply` flow. |
| Create or bind a work package | `scripts/apply-openproject-bootstrap.sh` | Creates missing work packages and bindings. For one item, toolkit `work-package create` or `bind`; use `backlog reconcile` to rebuild bindings. |
| Relations | `scripts/apply-openproject-relations.sh` | Applies canonical dependencies as OpenProject relations. The toolkit can list or delete a single relation. |
| Record evidence | `bin/replan evidence add ... --dry-run` | Review the selected week file and item, then replace `--dry-run` with `--apply`; the actor and every target must already exist. |
| Actuals and acceptance | `scripts/accept-task.sh` | Accepts one review-ready task through evidence, actuals, dependency candidates and its one OpenProject work package. It is dry-run by default and resumable. Use individual `bin/replan actual ... --dry-run` commands for facts that are not task acceptance. |
| Replace stale planned dates with actuals | `bin/replan task record-actual-window ... --dry-run` | Derives one finished task's actual window from status audit facts and current-demand consumption actuals, then writes both dates with one audit record. Review before `--apply`. |
| Status to OpenProject | `scripts/push-openproject-status-drift.sh` | Pushes reviewed RePlan tracking status. |
| Status from OpenProject | `bin/replan import accept <snapshot-id> --accept-tracking-status --dry-run` | Review a stored import snapshot; apply with `--apply --actor <id>`. The soft-drift helper does not accept status. |
| Import GitHub Issues export | `bin/replan import github --project "$REPLAN_PROJECT" --from-file <path> --dry-run` | File input only; review candidates, omit `--dry-run` to stage the snapshot, then use the shared `import accept` modes. No GitHub write-back or network input is implemented. |
| Planned dates to OpenProject | `scripts/push-openproject-date-drift.sh` | Toolkit `work-package update-dates` is the single-work-package surface. |
| Dates from OpenProject | `bin/replan task set-dates ... --dry-run` | Reviewed one-task acceptance. For a stored snapshot use `bin/replan import accept <snapshot-id> --accept-dates --dry-run`. The live batch helper keeps private-binding matching, then writes audited `task set-dates` changes and requires `REPLAN_ACTOR` with `REPLAN_APPLY=1`. |
| Correct subject or description | `scripts/openproject-work-package-toolkit.sh work-package update-text ...` | Shows old/new text and the current lock version. A bound subject is preflighted and synchronized with audited `bin/replan task set-title`; an OpenProject description is not copied into the separate RePlan planning note. |
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
- `REPLAN_ACCEPT_AT`, default the current RFC3339 time with an offset;
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
`tmp/openproject-live-overlay/latest` under the kit root. The publish helper
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

The relations dry run warns when planned `follows` relations may move
successor dates. After apply, run `scripts/check-openproject-drift.sh`; if the
reviewed RePlan dates must be restored, review
`scripts/push-openproject-date-drift.sh` and only then apply that push.

Bootstrap merges new task and assignee bindings into the private bindings
file; it preserves existing relations, bindings and extension keys, and refuses
another project's file with `bindings_project_mismatch` (exit 5).

Work packages are assigned to the token's own user unless
`REPLAN_OPENPROJECT_ASSIGNEE_USER_ID` names another member. With
`REPLAN_OPENPROJECT_ASSIGNEE_RESOURCE_ID` set, bootstrap also binds that user
to the RePlan resource it names.

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
scripts/openproject-operator-report.sh
```

Work on single work packages:

```sh
scripts/openproject-work-package-toolkit.sh work-package inspect --task <task id>
scripts/openproject-work-package-toolkit.sh work-package create --subject "..." --type-name Task
scripts/openproject-work-package-toolkit.sh work-package bind --task <task id> --work-package <number>
scripts/openproject-work-package-toolkit.sh work-package update-status --task <task id> --status "In progress"
scripts/openproject-work-package-toolkit.sh work-package update-dates --task <task id> --start-date 2026-10-12 --due-date 2026-10-14
scripts/openproject-work-package-toolkit.sh work-package update-text --task <task id> --subject "Corrected subject" --evidence <evidence id>
scripts/openproject-work-package-toolkit.sh comment add --task <task id> --text "..."
scripts/openproject-work-package-toolkit.sh status list
scripts/openproject-work-package-toolkit.sh user list
scripts/openproject-work-package-toolkit.sh relation list
```

`update-status --status` accepts either a RePlan tracking status
(`not_started`, `ready`, `in_progress`, `blocked`, `review_ready`, `accepted`,
`cancelled`) or its exact OpenProject name (`New`, `To be scheduled`,
`In progress`, `On hold`, `Developed`, `Closed`, `Rejected`).

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
