# Backlog Operating Convention

Date: 2026-08-02

Status: active operating convention.

## Purpose

This document defines how RePlan work is planned while RePlan is being
dogfooded.

The project must not drift back into chat-only or documentation-only planning.
The product is specifically about planning, resources, execution tracking and
analysis. Therefore future RePlan work should be visible in the same
OpenProject/RePlan loop that the product is supposed to support.

## Decision

The default backlog system for RePlan development is:

```text
OpenProject + RePlan
```

OpenProject is the human-facing task backlog and Gantt surface.

RePlan is the resource-planning, capacity, scenario, evidence, proposal and
audit sidecar.

Documentation remains important, but it is not the only backlog for agreed
work.

## What Must Go Into The Backlog

Create or update OpenProject/RePlan backlog entries for:

- agreed implementation tasks;
- agreed design tasks;
- agreed research tasks that affect product direction;
- OpenProject-native surface or plugin work;
- OSS boundary checkpoint work;
- full dogfood-cycle proof work;
- live synchronization or operator-loop tasks;
- follow-up gaps discovered by audits, dogfood runs or live integration;
- any task expected to consume maintainer, reviewer, Codex, CI, quota,
  release-window or other modeled resources.

Do not create backlog tasks for:

- tiny typo fixes done immediately;
- one-off clarification answers;
- transient local verification commands;
- private operational details that belong outside the public RePlan repository.

If a small task reveals a reusable product or process gap, create a backlog item
for the gap.

## Ownership Split

OpenProject owns:

- task identity;
- work-package title;
- human task status;
- human-visible dates;
- task relations;
- Gantt and human PM views;
- ordinary task collaboration.

RePlan owns:

- resource demands;
- capacities;
- availability and quotas;
- AI-agent and subscription-limit assumptions;
- scenarios;
- workload and overload reports;
- actuals and evidence accepted into RePlan;
- proposals;
- audit records;
- AI-safe command flow.

## Local OpenProject Credentials

OpenProject operator scripts may load local connection settings from ignored
dotenv files so agent sessions can be restarted without losing access context.

Use this local file for real values:

```text
.env.local
```

The file is ignored by git. The committed `.env.example` is only a template and
must not contain a real token, private endpoint or private binding path.

Supported dotenv keys are intentionally narrow:

- `REPLAN_OPENPROJECT_BASE_URL`;
- `REPLAN_OPENPROJECT_PROJECT_ID`;
- `REPLAN_OPENPROJECT_TOKEN_ENV`;
- `REPLAN_OPENPROJECT_BINDINGS_PATH`;
- `REPLAN_OPENPROJECT_HOST_HEADER`;
- `REPLAN_OPENPROJECT_FORWARDED_PROTO`;
- `REPLAN_OPENPROJECT_TIMEOUT_SECONDS`;
- `REPLAN_OPENPROJECT_RETRIES`;
- `REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS`;
- `OPENPROJECT_TOKEN`.

Explicit environment variables always win over dotenv values. `.env.local`
wins over `.env`. The loader reads key-value pairs as data and does not execute
dotenv files as shell scripts. Empty values are ignored, so a blank
`OPENPROJECT_TOKEN=` placeholder is not treated as a configured token.

Do not put `REPLAN_APPLY=1` in `.env` or `.env.local`. The loader intentionally
ignores it. Apply remains an explicit per-command operator decision.

For a private OpenProject instance reached through a local SSH tunnel, set the
base URL to the local tunnel endpoint and set `REPLAN_OPENPROJECT_HOST_HEADER`
plus `REPLAN_OPENPROJECT_FORWARDED_PROTO` when the upstream OpenProject/Nginx
configuration expects the original public host and protocol. The committed
examples must keep those values generic.

Live OpenProject requests are bounded by a shared timeout/retry contract:

- `REPLAN_OPENPROJECT_TIMEOUT_SECONDS` sets the per-request HTTP timeout in
  seconds. Default: `60`.
- `REPLAN_OPENPROJECT_RETRIES` sets retry count for transient
  timeout/network/server failures in Python operator helpers. Default: `0`.
- `REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS` sets the retry delay in seconds.
  Default: `2`.

These settings are intentionally safe to keep in ignored dotenv files because
they are operational parameters, not secrets. They apply to the OpenProject
work-package toolkit, status push, date push, bootstrap apply and relation
apply helpers. `replan import openproject` also uses
`REPLAN_OPENPROJECT_TIMEOUT_SECONDS`, and can override it per command with
`--request-timeout`.

Use longer timing settings after OpenProject restarts, after plugin image
replacement, or when Rails is still warming up behind the proxy or tunnel:

```sh
REPLAN_OPENPROJECT_TIMEOUT_SECONDS=180 \
REPLAN_OPENPROJECT_RETRIES=2 \
REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS=5 \
./scripts/openproject-work-package-toolkit.sh status list
```

Automatic retries apply to reads and to PATCH requests carrying a
`lockVersion` only; validation-only form calls are retried only when the
caller explicitly marks them idempotent.

If a guarded helper still fails with longer reviewed settings, keep the failure
as command evidence and fix the operator loop or OpenProject deployment state.
Do not bypass the helper with an ad hoc raw API mutation.

Docs own:

- decisions;
- rationale;
- product/architecture/design explanations;
- sanitized public conclusions;
- operator procedures.

## Required Workflow For Agreed Work

When a new non-trivial work item is agreed:

1. Add or update the OpenProject work package, bind it, and write its
   "RePlan" link with
   `REPLAN_APPLY=1 ./scripts/openproject-work-package-toolkit.sh work-package replan-links --task <task>`,
   so the work package links to its RePlan task page.
2. Create the RePlan task and demands with `replan task add --dry-run`, review
   the three-file preview, then repeat with `--apply`. Use the existing
   reviewed mutation commands for later demand and dependency changes.
   In this repository's self-dogfood plan, give the task a sanitized
   OpenProject source ref, add the same id to
   `examples/openproject/private-bindings.replan-self.example.yaml`, and
   append its sanitized work package to the sample with
   `REPLAN_UPDATE_GOLDEN=1 go test ./internal/app -run TestSelfOpenProjectSampleAgreesWithCanonicalTasks`.
   The generator appends missing work packages and refreshes linked subjects
   and planned dates while preserving deliberately drifted date examples.
   Run it after applying `accept-task.sh` and after `replan task set-title`,
   then commit the refreshed sample with the acceptance or rename.
3. Run RePlan validation.
4. Run OpenProject/RePlan drift check when live OpenProject is involved.
5. Run resource schedule drift check.
6. Refresh reports or dashboard when the operator needs human visibility,
   and publish the project's overlay to the live instance whenever a task
   was added, its dates or status changed, or it was accepted:
   `./scripts/build-openproject-live-overlay.sh` then
   `REPLAN_APPLY=1 ./scripts/publish-openproject-overlay.sh`. The RePlan
   pages in OpenProject show only what was published; an unpublished task
   is "not found" there although its work package links to it.
7. Commit source/docs/canonical changes.
8. Push to all configured remotes.

When the operator accepts a completed task, use the tracked composite instead
of replaying its mutations by hand:

```sh
REPLAN_TASK=task.example.finish-report \
REPLAN_ACCEPTED_BY=actor.human.owner \
REPLAN_ACCEPT_SUMMARY="Owner reviewed and accepted the report." \
./scripts/accept-task.sh

REPLAN_TASK=task.example.finish-report \
REPLAN_ACCEPTED_BY=actor.human.owner \
REPLAN_ACTOR=actor.human.recorder \
REPLAN_ACCEPT_SUMMARY="Owner reviewed and accepted the report." \
REPLAN_APPLY=1 ./scripts/accept-task.sh
```

The dry run validates the evidence and actual commands and shows the ordered
acceptance plan. Apply records the decision evidence, optional CI consumption,
remaining-work and demand closure, accepted tracking status, dependency
proposal candidates, the one bound OpenProject status update, and the optional
overlay refresh. Review and accept any proposed dependency change
separately; task acceptance is not authority to accept a proposal. A partial
apply exits 7 with its durable progress, and the same command resumes from
RePlan and OpenProject state. Only optional remote overlay publication needs a
disposable local completion marker, because rsync completion is not canonical
state in either system; losing that marker causes a safe republish.

Move the status date with `replan project set-status-date --dry-run` when the
week's work is recorded, then repeat with `--apply`; a stale status date makes
warnings about tasks that finished after it.

Minimum command set after backlog/data changes:

```sh
go run ./cmd/replan validate examples/replan-self --format json
./scripts/check-resource-schedule-drift.sh
```

When live OpenProject is reachable:

```sh
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/check-openproject-drift.sh
```

Run that date drift check immediately after applying OpenProject relations or
pushing planned dates. A helper warning of `openproject_rescheduled` reports
successor fields that OpenProject moved. `openproject_stored_other_dates`
reports pushed values that OpenProject replaced, commonly because the sent day
is non-working. Prefer accepting reviewed scheduler dates through
`scripts/accept-openproject-soft-drift.sh` (or audited one-task `replan task
set-dates --dry-run`, then `--apply`) so the next date-push dry run is empty.
If `rescheduling_unknown` appears, the write exists but one or more final reads
failed; inspect the listed work packages and rerun the drift check before the
next mutation.

A dry run of `work-package replan-links` without `--task` is the drift check
for the "RePlan" link field: it lists every bound work package whose link is
missing or was changed by hand.

## If Live OpenProject Access Is Unavailable

Do not block useful work only because live OpenProject is temporarily
unavailable.

Instead:

- record the task in RePlan canonical data or the relevant roadmap document;
- mark the OpenProject sync state as pending in the document or task notes;
- avoid pretending that OpenProject is already aligned;
- sync live OpenProject as soon as access returns;
- rerun drift checks after sync.

Use wording such as:

```text
OpenProject sync: pending.
Reason: live OpenProject unavailable during planning update.
```

## Current Near-Term Backlog Direction

The previous release-story path is deferred.

Current state update, 2026-08-15:

- the OpenProject preflight infrastructure build has produced guarded helper
  scripts for inspection, creation, binding, drift checks and narrow write-back;
- the first OpenProject-centered dogfood cycle is active;
- the read-only OpenProject plugin path has moved from design to live
  installation in the private dogfood instance;
- the project-level plugin view now renders a resource-aware RePlan view inside
  OpenProject;
- the next active OpenProject-native surface decision is the
  work-package-level RePlan panel;
- public release story remains deferred.

The near-term work should prioritize:

1. OSS boundary checkpoint, not public release preparation.
2. Completing the active OpenProject-centered dogfood cycle: planning,
   resource check, execution, actuals, analysis and next decision.
3. OpenProject-native RePlan surface strategy:
   project-level plugin view, work-package-level panel, controlled plugin
   Gantt-like view and only later possible native OpenProject Gantt overlay.
4. RePlan dashboard semantics cleanup where human observability conflicts with
   the authoritative resource schedule reports.
5. Documentation synchronization when roadmap, design and backlog documents
   diverge after fast dogfood slices.

The release story stays blocked until:

- the OSS boundary checkpoint is complete enough;
- OpenProject preflight infrastructure is good enough that agents can inspect
  and mutate the live dogfood plan through guarded dry-run/apply workflows;
- at least one full OpenProject-centered planning/tracking/analysis cycle has
  been completed;
- the operator can explain how RePlan is visible and useful from the
  OpenProject-centered human workflow.

## OpenProject-Native Surface Priority

OpenProject-native surface work is now near-term product work, not a distant
nice-to-have.

The staged path is:

```text
Level 0: links from OpenProject to RePlan dashboard.
Level 1: adjacent generated RePlan dashboard/Gantt as a human view.
Level 2: RePlan summary back into OpenProject comments or custom fields.
Level 3: OpenProject plugin/tab/panel.
Level 4: deeper bidirectional plugin integration.
```

Each level should be designed and dogfooded before jumping to the next one.

## Done Rule

A backlog item is done only when:

- the task state is updated in OpenProject or explicitly marked sync-pending;
- RePlan resource/tracking state is updated if the item consumed modeled
  resources;
- relevant docs are updated;
- validation and appropriate drift checks pass or the limitation is documented;
- the result is committed and pushed.
