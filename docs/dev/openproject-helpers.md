# OpenProject helpers

These are the commands for running RePlan beside an OpenProject project:
importing work packages, checking drift and pushing reviewed dates and status
back. Every helper that writes is dry-run by default and writes only with
`REPLAN_APPLY=1`.

The examples use the public `replan-self` fixture and a placeholder endpoint.
The [agent kit surface map](../agent-kit/README.md#surface-map) says which
command serves each step of the operator loop; this page shows how to call
them.

## Import from a file

Exercise the OpenProject mapper and sidecar convention from public fixtures
(sidecar paths are relative to the project directory):

```bash
go run ./cmd/replan import openproject \
  --project examples/replan-self \
  --from-file examples/imports/openproject/replan-self.work-packages.sample.json \
  --resource-links-sidecar ../openproject/sidecars/resource-links.replan-self.yaml \
  --resource-demands-sidecar ../openproject/sidecars/resource-demands.replan-self.yaml \
  --include-relations --include-time-entries --dry-run --format json
```

Refresh the sample's linked subjects and planned dates after applying
`accept-task.sh` or `replan task set-title`, and commit it with the
acceptance or rename:

```sh
REPLAN_UPDATE_GOLDEN=1 go test ./internal/app -run TestSelfOpenProjectSampleAgreesWithCanonicalTasks
```

See [`openproject-pilot-lessons.md`](openproject-pilot-lessons.md)
for why each candidate type has a separate manual accept flow.

## Check the live project

Check whether the live OpenProject project is actually the current RePlan
self-dogfood task source:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/check-openproject-live-alignment.sh
```

For the live dogfood project, the expected post-bootstrap state is:

```text
live_alignment: aligned_by_private_binding
```

Build a read-only operator report before deciding which OpenProject/RePlan
helper should run next:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/openproject-operator-report.sh
```

The report lists mapped tasks, missing or extra work packages, status/date
drift, relation direction, private binding state, active RePlan resource
demands and the guarded write helpers available for the next reviewed action.

## Timeouts and retries

For slow live OpenProject operations, especially after plugin deployment or
Rails restart, use the shared request timing controls:

```bash
REPLAN_OPENPROJECT_TIMEOUT_SECONDS=180 \
REPLAN_OPENPROJECT_RETRIES=2 \
REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS=5 \
./scripts/openproject-work-package-toolkit.sh status list
```

Automatic retries apply to reads and to PATCH requests carrying a
`lockVersion` only; validation-only form calls are retried only when the
caller explicitly marks them idempotent.

`replan import openproject` also accepts `--request-timeout`; the same timeout
environment variable is used by live drift checks.

## Bootstrap a project

Prepare a dry-run plan for bootstrapping the current RePlan self-dogfood tasks
into an OpenProject project:

```bash
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/plan-openproject-bootstrap.sh
```

Set `REPLAN_OPENPROJECT_PROJECT_NAME` when a newly created OpenProject project
needs a display name different from its identifier. The bootstrap uses that
name only for project creation and otherwise defaults it to
`REPLAN_OPENPROJECT_PROJECT_ID`. The former `TARGET_PROJECT_ID` and
`TARGET_PROJECT_NAME` settings remain deprecated aliases for one release.

Apply the reviewed bootstrap plan to live OpenProject only after reviewing the
dry-run output:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/apply-openproject-bootstrap.sh
```

Set `REPLAN_APPLY=1` only when the dry-run actions are accepted.

Apply reviewed canonical RePlan dependencies to live OpenProject only after
stable work-package ids exist and the private binding layer is present:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/apply-openproject-relations.sh
```

Set `REPLAN_APPLY=1` only when the dry-run relation actions are accepted. The
helper maps RePlan `finish_to_start` dependencies to OpenProject `follows`
relations and keeps live relation ids in ignored private bindings. OpenProject
may switch a new successor to automatic scheduling and move its dates while
creating that relation. The helper snapshots every reachable bound successor,
then restores changed or already-automatic work packages roots-first with
`scheduleManually: true`, their prior dates, a current lock version and the
shared non-working-day payload rule. Each snapshot phase uses one paginated,
closed-inclusive project work-package read and falls back to individual GETs
only for bound ids omitted by that collection. The current lock version is
still read individually immediately before each PATCH.
`restored_work_packages` reports the planned dry-run restores or completed
apply restores; `openproject_rescheduled` is emitted only for differences that
remain after final verification. Set
`REPLAN_OPENPROJECT_KEEP_AUTOMATIC_SCHEDULING=1` to opt out and retain
OpenProject's automatic scheduling.

## Check drift

Check whether live OpenProject and local RePlan have drifted before the next
mutation:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/check-openproject-drift.sh
```

The report returns `aligned`, `soft_drift`, `drift`, `skipped` or `failed`.
`soft_drift` means task and dependency identity still match, but status or date
differences need operator review. Each task with a date difference emits
`openproject_date_drift`; use `push-openproject-date-drift.sh` when RePlan's
reviewed dates should win. Each bound live work package with
`scheduleManually: false` emits `openproject_automatic_scheduling`; restore it
with `openproject-work-package-toolkit.sh work-package update-dates` and the
task's current RePlan dates. The helper obtains this flag with one paginated
read of the selected project's raw work-package collection because the mapped
Go import candidate does not expose it. These warnings do not change the
existing `ok`, `drift_status`, counts or exit-code policy. If that collection
read fails, `openproject_scheduling_unknown` reports a sanitized reason while
leaving those same status and exit fields unchanged.

## Accept OpenProject dates

Accept OpenProject scheduled dates into RePlan only after reviewing soft drift:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/accept-openproject-soft-drift.sh
```

Set `REPLAN_APPLY=1` only after the dry-run date changes are accepted. This
helper updates RePlan `planned_start` and `planned_finish` only; tracking
status differences stay review-only.

## Write back dates and status

Push reviewed RePlan planned dates back to OpenProject when a resource-based
RePlan scenario is accepted and OpenProject still shows stale dates:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/push-openproject-date-drift.sh
```

The helper is dry-run by default, reads live work-package bindings from the
private binding layer, refuses hard task/dependency drift and writes only
OpenProject `scheduleManually`, `startDate`, `dueDate` and, when needed,
`ignoreNonWorkingDays: true` after `REPLAN_APPLY=1`. The toolkit's
`work-package update-dates` and dated `work-package create`, and the bootstrap's
dated creates, use the same rule.

The writers read the instance's recurring working week once per helper run
from `GET /api/v3/days/week`, using the shared OpenProject timeout and retry
settings. If either sent date falls on a weekday the instance marks
non-working, the same create or PATCH sets `ignoreNonWorkingDays: true`; its
dry-run and apply action say `set_ignore_non_working_days: true` and identify
the date and weekday. Otherwise the property is omitted, even when its current
value is true. It is never reset to false because doing so could move dates
that another tool intentionally placed on a non-working day. If the week
endpoint is unavailable or malformed, the writer falls back to Saturday and
Sunday and emits `openproject_working_days_unknown`.

This lookup does not read OpenProject's configured holiday dates. The existing
moved-date check remains the backstop: if OpenProject stores a different date,
the date push still reports `openproject_stored_other_dates` as before. The
drift check and operator report need no special case; both compare the dates
OpenProject returns with RePlan's dates, so a date stored as sent is already
aligned and a different stored date remains ordinary date drift.

Push reviewed RePlan tracking status decisions back to OpenProject when the
operator chooses RePlan as the status source:

```bash
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/push-openproject-status-drift.sh
```

The helper is dry-run by default and uses
`REPLAN_OPENPROJECT_STATUS_SCOPE=ready_only` unless a broader reviewed scope is
explicitly requested. Set `REPLAN_APPLY=1` only after reviewing the planned
status actions.

## Resource checks

Review the current RePlan self-dogfood resource pool before mutating
OpenProject:

```bash
./scripts/review-resource-pool.sh
```

After OpenProject and RePlan are aligned, check whether resource demands still
fit the accepted task schedule:

```bash
./scripts/check-resource-schedule-drift.sh
```

The report returns `aligned`, `needs_calibration` or `drift`. Current
self-dogfood resource data is expected to need calibration until active demands,
remaining amounts and weekly capacities are reviewed.

Before mutating resource demands, generate the read-only calibration candidate
plan:

```bash
./scripts/plan-resource-demand-calibration.sh
```

The planner does not edit the project. It lists active demands on accepted
tasks, active open-task demands without `remaining_amount`, and lifecycle
decisions that need operator review before workload numbers are treated as
commitment-quality planning evidence.

## Private bindings

Private live OpenProject bindings are intentionally kept out of public
canonical files. Use ignored local storage such as:

```text
private/openproject/replan-self-bindings.yaml
```

The committed fake-id shape example is:

```text
examples/openproject/private-bindings.example.yaml
```

## Operating direction

Current OpenProject operator-loop direction:

- keep OpenProject as the human task, relation, Gantt and collaboration
  surface;
- keep RePlan as the resource planning, workload, scenario, evidence, proposal
  and audit layer;
- treat the read-only OpenProject plugin as the current mandatory
  OpenProject-native human surface, not as optional future polish;
- use the project-level plugin resource view for current OpenProject-side
  RePlan visibility;
- use the deployed work-package-level RePlan panel as the current task-level
  OpenProject-native RePlan surface;
- keep native OpenProject Gantt modification deferred until simpler plugin
  surfaces prove which resource signals are worth the extra coupling;
- put agreed non-trivial future work into the OpenProject/RePlan dogfood
  backlog; do not leave backlog only in chat or documentation prose;
- keep broad automatic OpenProject write-back out of scope, and use only
  explicit reviewed helpers for narrow status/date pushes;
- use public-friendly self-dogfood data for the RePlan OSS story;
- keep private pilot endpoints and operational evidence outside this
  repository.
