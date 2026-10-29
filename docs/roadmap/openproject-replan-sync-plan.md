# OpenProject/RePlan task-source sync plan

Date: 2026-07-30

Status: approved next dogfood block.

Related self-dogfood task:
`task.self.expand-openproject-fixtures`.

## Purpose

This document defines the next RePlan self-dogfood block after plan/resource
calibration.

The goal is to make the OpenProject-primary operating mode concrete:

```text
OpenProject is the source of truth for tasks.
RePlan is the resource-planning sidecar above those tasks.
```

This block should turn that statement into a repeatable contract, fixture,
validation procedure and operator workflow.

## Decision

OpenProject is the primary source for task-management data in the near-term
self-dogfood workflow.

OpenProject owns:

- work-package identity;
- task title and description;
- task status as seen by humans in the project-management system;
- task assignee;
- planned start and finish dates where OpenProject has them;
- work-package relations;
- Gantt and other human PM views;
- time entries as source execution facts when available.

RePlan owns:

- resource identities that are not native OpenProject users;
- resource classes;
- resource demands;
- capacities;
- AI-agent resources;
- reviewer role capacity;
- CI, API, quota, subscription and environment resources;
- remaining-work calibration when it is not directly represented in
  OpenProject;
- actuals accepted into RePlan;
- evidence links;
- proposals and audited decisions;
- resource workload, overload and planning warnings.

RePlan must not silently overwrite OpenProject data, and OpenProject import
must not silently overwrite RePlan resource assumptions. Any meaningful
mutation stays explicit, reviewable and audited.

## Sequence

The block uses a two-stage sequence.

### Stage 1. Sanitized Fixture First

Use committed, public-safe OpenProject-like fixture data first.

Reason:

- it is safe for the OSS repository;
- it requires no token, protected connection or live service;
- it is repeatable in tests and CI;
- it lets AI CLI agents run the workflow without private access;
- it prevents private OpenProject pilot data from leaking into public examples.

This stage should verify:

- which OpenProject fields are mapped into RePlan tasks;
- how OpenProject statuses are represented without becoming RePlan's internal
  `tracking_status` automatically;
- how assignees map to RePlan resources or resource-link candidates;
- how relations map to RePlan dependency candidates;
- how time entries map to evidence and actual candidates;
- how resource-demand sidecars attach resource needs to OpenProject tasks;
- how the import output explains unmatched, unsafe or no-op candidates.

### Stage 2. Live Validation Second

After the sanitized fixture contract is stable, run a live validation against
the private OpenProject instance.

Reason:

- it proves that the fixture is not only a toy sample;
- it checks real API behavior, relation behavior, time-entry behavior and
  status values;
- it exposes live differences that should be reflected back into sanitized
  fixtures or documented as private-pilot-only details.

Live validation must not commit tokens, private URLs, real user data or private
project narrative into the public repository.

Live credentials should stay in environment variables, for example:

```text
OPENPROJECT_TOKEN
```

Public source profiles may name only generic values such as:

```yaml
base_url: https://op.example.internal
token_env: OPENPROJECT_TOKEN
```

## Deliverables

This block should produce:

- a reviewed OpenProject/RePlan source-of-truth table;
- a fixture-first command procedure;
- a live-validation command procedure;
- a list of OpenProject fields currently trusted by RePlan;
- a list of OpenProject fields currently imported only as candidates;
- a list of resource-sidecar fields that stay RePlan-owned;
- updated sanitized fixture data if the current fixture is missing important
  live behavior;
- updated tests or smoke checks where the behavior should become contractual;
- a clear next decision after the block.

## Backlog

### 1. Confirm Current Public Fixture Coverage

Status: completed initial pass on 2026-07-30.

Review the committed OpenProject fixture and sidecar examples.

Questions:

- Does the fixture include multiple work-package statuses?
- Does it include multiple assignees?
- Does it include dates?
- Does it include relations?
- Does it include time entries?
- Does it include tasks with resource demands that OpenProject itself does not
  naturally model?
- Does it include no-op and unsafe candidate cases?

Expected output:

- short coverage table;
- list of missing fixture cases;
- decision whether to update fixture before live validation.

Observed public fixture files:

- `examples/imports/openproject/replan-self.work-packages.sample.json`;
- `examples/imports/openproject/work-packages.sample.json`;
- `examples/openproject/sidecars/resource-links.replan-self.yaml`;
- `examples/openproject/sidecars/resource-demands.replan-self.yaml`.

Coverage table from the 2026-07-30 fixture audit:

| Area | `replan-self.work-packages.sample.json` | `work-packages.sample.json` | Sidecar examples | Current status |
| --- | --- | --- | --- | --- |
| Work packages | 22 work packages | 2 work packages | Not applicable | Good enough for task import coverage. |
| Statuses | `Closed`, `In progress`, `New` on all 22 work packages | `In progress` on 1 work package, 1 missing status | Not applicable | Good enough for basic status mapping; missing-status warning remains covered by the small sample. |
| Assignees | `RePlan maintainer` on all 22 work packages | `Ivan` on 1 work package, 1 missing assignee | 3 explicit OpenProject user to RePlan resource links | Good enough for basic assignee/resource candidate coverage; not enough for multi-human planning realism. |
| Dates | 22 start dates and 22 due dates | 2 start dates and 2 due dates | Not applicable | Good enough for date mapping and Gantt smoke coverage. |
| Estimates | 22 `estimatedTime` values | 2 `estimatedTime` values | Not applicable | Good enough for OpenProject estimate mapping. |
| Spent time on work packages | 22 `spentTime` values | 2 `spentTime` values | Not applicable | Present as work-package fields, but not equivalent to accepted RePlan actual records. |
| OpenProject time entries | 1 public time-entry example | None in file-mode fixture | Not applicable | Covered for self-dogfood file-mode import through `_embedded.time_entries.elements`. |
| Relations | 2 public relation examples | No relation links in fixture | Not applicable | Covered for self-dogfood file-mode import through `_embedded.relations.elements`. |
| Parent or hierarchy links | None | None | Not applicable | Missing; acceptable for the first source-boundary pass, but should be added before claiming broad OpenProject planning parity. |
| Resource links | Not embedded in work-package JSON | Not embedded in work-package JSON | 3 link entries: operator, human developer, AI agent | Sidecar shape exists; current committed sidecar names still use pilot wording and should be made self-dogfood/public-safe before becoming the primary public fixture. |
| Resource demands | Not embedded in work-package JSON | Not embedded in work-package JSON | 8 self-dogfood demand entries attached to OpenProject work packages | Covered as both idempotent existing-demand overlay and positive `create_resource_demand` dry-run example. |
| No-op and unsafe candidates | Dry-run import has no warnings against the self fixture | Covers unmatched work package and missing assignee warnings | Resource sidecar tests cover duplicate, unmatched and unsafe cases in code tests | Covered partly by the small sample and tests, not yet by a single rich public scenario fixture. |

Commands run during the 2026-07-30 fixture audit:

```sh
go run ./cmd/replan import openproject \
  --project examples/replan-self \
  --from-file examples/imports/openproject/replan-self.work-packages.sample.json \
  --dry-run \
  --format json
```

Historical observed result before relation file-mode support:

- `ok`: `true`;
- task candidates: `22`;
- resource candidates: `1`;
- time-entry candidates: `0`;
- dependency candidates: `0`;
- warnings: none.

```sh
go run ./cmd/replan import openproject \
  --project examples/replan-self \
  --from-file examples/imports/openproject/replan-self.work-packages.sample.json \
  --include-relations \
  --dry-run \
  --format json
```

Observed result:

- `ok`: `true`;
- task candidates: `22`;
- dependency candidates: `0`;
- warning: `openproject_relations_file_mode_unsupported`.

Current observed result after relation file-mode support:

- `ok`: `true`;
- task candidates: `22`;
- dependency candidates: `2`;
- dependency `9101`: OpenProject `follows` mapped to RePlan
  `finish_to_start`, predecessor `task.self.calibrate-plan-and-resources`,
  successor `task.self.expand-openproject-fixtures`;
- dependency `9102`: OpenProject `blocks` mapped to RePlan `blocks`,
  predecessor `task.self.write-oss-readiness-audit`, successor
  `task.self.prepare-release-story`;
- warnings: none.

Current observed result after time-entry file-mode support:

- `ok`: `true`;
- task candidates: `22`;
- time-entry candidates: `1`;
- time entry `9201`: OpenProject work package `2105`, user
  `RePlan maintainer`, activity `Development`, `1.5` hours on `2026-07-30`;
- `import accept --accept-time-entries --dry-run` previews evidence
  `evidence.openproject.time-entry.9201` and actual
  `actual.openproject.time-entry.9201.work-consumed`;
- warnings: none.

```sh
go run ./cmd/replan import openproject \
  --project examples/small-team \
  --from-file examples/imports/openproject/work-packages.sample.json \
  --dry-run \
  --format json
```

Observed result:

- `ok`: `true`;
- task candidates: `2`;
- resource candidates: `1`;
- time-entry candidates: `0`;
- dependency candidates: `0`;
- warnings: `unmatched_work_package`, `missing_openproject_assignee`.

```sh
./scripts/validate-openproject.sh
```

Observed result:

- file validation: passed;
- snapshot validation: passed;
- accept dry-run validation: passed;
- live API validation: skipped because no live token was used in this stage;
- dependency accept dry-run validation: skipped because relation validation is
  live/API-oriented at this point.

Temporary-copy accept against the self fixture was also checked before the
status-policy follow-up. It produced three explicit status-update changes and
no warnings:

- `task.self.expand-openproject-fixtures`: `ready` -> `in_progress`;
- `task.self.prepare-release-story`: `not_started` -> `ready`;
- `task.self.calibrate-plan-and-resources`: `accepted` -> `in_progress`.

The first change matched the current block starting. The second change exposed
an overly aggressive historical OpenProject status mapping rule:
OpenProject `New` mapped to RePlan `ready`, even when the RePlan plan still
treated the work as future work. The third change was a fixture drift:
`task.self.calibrate-plan-and-resources` had already been accepted in RePlan,
while the public self fixture still said the OpenProject work package was
`In progress`.

Initial decision:

- the existing public self fixture is good enough to keep using as the task
  import baseline;
- it is not yet rich enough to be the complete OpenProject/RePlan source-sync
  demonstration;
- before live validation becomes the next default step, the public fixture
  should be updated or supplemented so it has public-safe coverage for:
  OpenProject relations, time entries, richer assignee/resource combinations,
  and the current accepted state of already-closed self-dogfood work;
- sidecar examples should be aligned with the self-dogfood fixture or renamed
  into public-safe example names before being treated as the canonical public
  OpenProject resource sidecar example.

Follow-up completed on 2026-07-30:

- updated `examples/imports/openproject/replan-self.work-packages.sample.json`
  so OpenProject work package `2122`, which maps to
  `task.self.calibrate-plan-and-resources`, is represented as `Closed` with
  `spentTime: PT4H`;
- added `examples/openproject/sidecars/resource-links.replan-self.yaml` as a
  public-safe OpenProject user to RePlan resource link example for
  `RePlan maintainer`;
- added `examples/openproject/sidecars/resource-demands.replan-self.yaml` as a
  public-safe resource-demand sidecar example for the next self-dogfood tasks;
- verified that the stale status rollback proposal for
  `task.self.calibrate-plan-and-resources` disappeared from temporary-copy
  `import accept --dry-run`.
- changed OpenProject status mapping so `New` and `Open` map to RePlan
  `not_started`; more explicit planning statuses such as `To be scheduled`,
  `Scheduled`, `In specification`, `Specified` and `Confirmed` still map to
  `ready`;
- aligned `task.self.write-oss-readiness-audit` with the new policy by keeping
  it `not_started` while the OpenProject source status remains `New`.
- added public file-mode relation coverage through
  `_embedded.relations.elements` in
  `examples/imports/openproject/replan-self.work-packages.sample.json`;
- verified `--include-relations` in file mode emits two dependency candidates:
  `dependency.openproject.relation.9101` as `finish_to_start` and
  `dependency.openproject.relation.9102` as `blocks`;
- verified `import accept --accept-dependencies --dry-run` can preview both
  public file-mode relation candidates as dependency creations on a temporary
  project copy.
- added public file-mode time-entry coverage through
  `_embedded.time_entries.elements` in
  `examples/imports/openproject/replan-self.work-packages.sample.json`;
- verified `--include-time-entries` in file mode emits time-entry candidate
  `9201`;
- verified `import accept --accept-time-entries --dry-run` can preview evidence
  and actual creation from the public file-mode time-entry candidate on a
  temporary project copy.
- added a public resource-demand sidecar positive accept example:
  `resource_demand.openproject-fixtures.llm-token-budget`;
- verified `--resource-demands-sidecar` emits that demand as
  `safe_to_accept` while existing self-dogfood demands remain idempotent
  already-existing candidates;
- verified `import accept --accept-resource-demands --dry-run` previews
  `create_resource_demand` for the token-budget demand on a temporary project
  copy.

Closed fixture-contract decision after this follow-up:

- OpenProject `New` and `Open` now map to RePlan `not_started`, because they
  mean the work package is visible in the task system but do not prove that
  dependencies, resources or operator intent make it ready to start.

Remaining fixture-contract decisions after this follow-up:

- no known fixture-first contract gap remains before live validation; any new
  fixture work should be driven by live OpenProject differences or operator
  usability gaps rather than by the basic public contract.

### 2. Re-run Fixture Import Contract

Status: in progress after the 2026-07-30 fixture audit.

Run the import pipeline on sanitized data only.

Expected checks:

- `import openproject --from-file`;
- task candidate output;
- resource-link sidecar output;
- resource-demand sidecar output;
- time-entry candidate output;
- relation/dependency candidate output;
- safe dry-run accept commands on a temporary project copy.

Expected output:

- repeatable command log;
- observed candidate counts;
- explicit list of accepted versus candidate-only data.

### 3. Define Source-Of-Truth Rules

Status: drafted from the approved product decision; still needs to be checked
against the next fixture update.

Write down field ownership.

At minimum:

- OpenProject work-package id: OpenProject-owned;
- title: OpenProject-owned;
- description: OpenProject-owned unless RePlan has a local note field later;
- source status: OpenProject-owned;
- RePlan `tracking_status`: RePlan-owned accepted state;
- assignee: OpenProject-owned source fact;
- resource identity: RePlan-owned after resource-link acceptance;
- dependencies from OpenProject relations: imported as candidates before
  acceptance;
- time entries: source facts that become RePlan actuals only through explicit
  accept;
- resource demands: RePlan-owned sidecar data.

Expected output:

- source-of-truth table in a spec or roadmap document;
- list of fields that must never be silently overwritten.

Current source-of-truth table:

| Field or behavior | Primary owner | RePlan import behavior | Mutation rule |
| --- | --- | --- | --- |
| Public/sanitized OpenProject work-package id | OpenProject | May be stored as `source_refs` on RePlan tasks and import candidates when the id belongs to a committed public fixture or example profile. | Never rewritten silently by RePlan. |
| Private live OpenProject work-package id | OpenProject | Stored in an ignored private binding layer for the current dogfood infrastructure. | Do not commit live ids into public canonical files unless a later OSS boundary decision explicitly allows it. |
| Task title | OpenProject | Imported as task candidate title. | RePlan should not silently rewrite local task title without explicit accept/review. |
| Task description | OpenProject | Imported as task candidate description when available. | RePlan may later have local notes, but OpenProject description remains the source PM description. |
| OpenProject source status | OpenProject | Imported as a source status and mapped to a RePlan tracking proposal/candidate. | RePlan applies only through explicit accept. |
| RePlan `tracking_status` | RePlan accepted state | Updated only after accept or direct RePlan planning action. | Must not be silently overwritten during read-only import. |
| Planned dates (`planned_start`, `planned_finish`) | RePlan drafts; OpenProject presents the reviewed dates to people | Imported OpenProject `startDate` and `dueDate` are compared with the RePlan plan. | Push only through a reviewed dry-run/apply helper. Drift never chooses a winner automatically. Accept one reviewed shift through audited `replan task set-dates`, stored-snapshot changes through audited `replan import accept --accept-dates`, or live private-binding matches through the dates-only soft-drift helper, which delegates each write to `task set-dates`. |
| Actual dates | RePlan evidence and actuals | OpenProject status may be imported as a candidate, but it is not an actual start or finish fact. | Record actual execution through evidence and `replan actual` commands. Push the resulting tracking status separately. |
| Assignee | OpenProject as source fact | Imported as source assignee/resource candidate. | Mapping to a RePlan resource requires explicit resource-link acceptance or configured sidecar. |
| Resource identity | RePlan | Stored in RePlan resources and sidecars. | OpenProject user identity is not automatically treated as the full resource model. |
| Resource class | RePlan | Provided by RePlan canonical data or sidecar. | OpenProject does not own this in the MVP design. |
| Resource demand | RePlan sidecar | Read from `integrations/openproject/resource-demands.yaml` or an explicit sidecar path. | Explicit accept required before it becomes canonical RePlan demand. |
| Capacity | RePlan | Read from RePlan canonical capacity files. | OpenProject does not own capacity. |
| AI-agent capacity and quotas | RePlan | Modeled as RePlan resources/capacities/availability/actuals. | OpenProject may have tasks assigned to an agent-like actor, but limit accounting stays in RePlan. |
| Work-package relations | OpenProject | Imported as dependency candidates when available through API/live relation import. | RePlan dependency creation requires explicit accept. |
| RePlan dependency state | RePlan | Canonical dependencies may be matched to OpenProject relation candidates. | RePlan should not silently delete or rewrite accepted dependencies. |
| OpenProject time entries | OpenProject | Imported as evidence and actual candidates when available through API import or public file fixture import. | Explicit accept required before creating RePlan actual records. |
| RePlan actuals | RePlan accepted execution ledger | Used by workload and state reports after acceptance. | Must remain auditable and evidence-linked. |
| Gantt and PM task views | OpenProject for human presentation, RePlan for resource overlay | OpenProject shows reviewed task dates; RePlan renders resource-aware static views. | RePlan must keep its overlay explainable against OpenProject task ids. |

Fields that must never be silently overwritten:

- OpenProject work-package ids;
- OpenProject source statuses;
- RePlan accepted `tracking_status`;
- RePlan planned dates;
- RePlan resource identities;
- RePlan capacities;
- RePlan resource demands;
- RePlan actual records;
- evidence links;
- audit records;
- accepted dependencies.

### 4. Decide Fixture Update Scope

Status: open; the 2026-07-30 audit found fixture updates worth doing before
the next live validation pass.

If the fixture is not rich enough, update it before live validation.

Allowed fixture updates:

- synthetic work packages;
- synthetic users;
- synthetic statuses;
- synthetic relations;
- synthetic time entries;
- synthetic sidecar resource links;
- synthetic sidecar resource demands.

Not allowed:

- private OpenProject URLs;
- private user names;
- private tokens;
- private hosting paths;
- exact private pilot narrative if it cannot be sanitized.

Expected output:

- updated fixture and tests, or explicit decision that current fixture is
  enough for this block.

Recommended fixture update scope from the 2026-07-30 audit:

- completed: update `examples/imports/openproject/replan-self.work-packages.sample.json`
  so already accepted self-dogfood work is represented as closed rather than
  stale in-progress work;
- completed: add public-safe relation coverage in a format the importer can
  validate without live OpenProject access;
- completed: add public-safe time-entry coverage in a format the importer can
  validate without live OpenProject access;
- completed: add public-safe self-dogfood sidecar examples that match the
  22-work-package self fixture, including a positive resource-demand accept
  dry-run case;
- keep the small two-work-package fixture because it is useful for no-op and
  warning coverage.

### 5. Run Live Validation

Status: wrapper implemented; live execution depends on private token and
OpenProject access.

Run live validation only after the fixture contract is understood.

Expected checks:

- use environment-based token access;
- run read-only import first;
- run temporary-copy accept pipeline only after reviewing dry-run output;
- compare live candidate counts with fixture expectations;
- record what live behavior should be generalized into public fixtures.

Expected output:

- private-safe summary;
- no committed secrets;
- no public leak of private pilot details;
- clear list of mapper or fixture changes discovered by live validation.

Implemented wrapper:

```sh
./scripts/validate-openproject-live.sh
```

The wrapper:

- returns `live_api_validation: "skipped"` when the configured token
  environment variable is not set;
- defaults to `OPENPROJECT_TOKEN` without printing token values;
- supports either `REPLAN_OPENPROJECT_PROFILE` or explicit
  `REPLAN_OPENPROJECT_BASE_URL` plus `REPLAN_OPENPROJECT_PROJECT_ID`;
- runs a read-only live import with closed work packages, relations and time
  entries enabled by default;
- writes a live snapshot only into a temporary project copy;
- previews ordinary task accept, dependency accept, time-entry accept and
  resource-demand accept through dry-run commands;
- reports private-safe counts and warning/error codes instead of raw private
  payloads;
- normalizes existing repo-relative sidecar paths before passing them to the
  CLI, because the CLI resolves sidecar paths relative to the RePlan project
  directory;
- returns a structured failure envelope when live import returns `ok: false`
  instead of failing silently through shell `set -e`;
- supports `REPLAN_KEEP_TEMP=1` when the operator intentionally wants local
  temporary outputs for private inspection.

Known execution state:

- the wrapper was executed without `OPENPROJECT_TOKEN` in the local
  environment and returned `live_api_validation: "skipped"`;
- token-backed clean OpenProject API validation was executed on 2026-07-30
  against the private pilot endpoint and returned
  `live_api_validation: "passed"`;
- the clean live read-only import returned:
  - `task_candidates`: 11;
  - `resource_candidates`: 3;
  - `resource_demand_candidates`: 0;
  - `dependency_candidates`: 3;
  - `time_entry_candidates`: 2;
  - warning codes: `unmatched_work_package`,
    `openproject_relation_task_unmatched`;
  - error codes: none;
- the clean temporary-copy accept dry-runs returned:
  - task accept changes: 14;
  - dependency accept changes: 0;
  - time-entry accept changes: 0;
  - resource-demand accept changes: 0;
- token-backed validation with the historical pilot sidecars was executed on
  2026-07-30 and returned a structured failure summary with
  `openproject_resource_link_resource_unknown`;
- the sidecar failure is expected drift, not a live API failure: the historical
  pilot sidecars still reference `resource.human.operator` and
  `resource.human.ivan`, while `examples/replan-self` now uses
  `resource.human.maintainer` and the public self-dogfood resource catalog.

Backlog update from the 2026-07-30 live validation:

- completed: add a read-only alignment check that reports whether the live
  OpenProject project is the current self-dogfood task source;
- completed: add a dry-run bootstrap planner that converts the current RePlan
  self-dogfood task list into an operator-reviewable OpenProject work-package
  creation/matching plan;
- decide whether live pilot validation should run against a separate old-pilot
  fixture for the historical sidecars or whether the live OpenProject project
  should be recreated/updated from the current self-dogfood plan;
- align live OpenProject work-package subjects and source references with the
  current self-dogfood task ids, or regenerate a public-safe fixture from the
  live shape;
- add a single rich public fixture scenario that includes matched tasks,
  accepted dependencies, accepted time entries and accepted resource demands in
  one coherent example;
- keep the clean live validation path as the current proof that the API access,
  authentication, relations and time-entry retrieval work.

Implemented alignment helper:

```sh
./scripts/check-openproject-live-alignment.sh
```

The helper:

- returns `live_alignment: "skipped"` when the configured token environment
  variable is not set;
- runs read-only live OpenProject import with closed work packages, relations
  and time entries enabled by default;
- compares live task candidates with local RePlan tasks by OpenProject
  private bindings first, committed OpenProject `source_ref` second and
  normalized task title third;
- reports counts for local tasks, live candidates, source-ref matches,
  private-binding matches, title matches, live-unmatched tasks,
  local-unmatched tasks, dependencies, time entries and resource candidates;
- returns one of these operator states:
  - `aligned_by_private_binding`;
  - `aligned_by_source_ref`;
  - `alignable_by_title`;
  - `partial`;
  - `not_current_self_dogfood_plan`;
- gives a suggested operator decision without printing tokens.

Observed alignment result on 2026-07-30:

- target project path: `examples/replan-self`;
- live OpenProject project: private pilot project;
- alignment state: `not_current_self_dogfood_plan`;
- local RePlan tasks: 22;
- live task candidates: 11;
- matched by source ref: 0;
- matched by title: 0;
- live-unmatched tasks: 11;
- local-unmatched tasks: 22;
- dependency candidates: 3;
- time-entry candidates: 2;
- resource candidates: 3.

Interpretation:

A private historical project was readable, but it was not the current RePlan
self-dogfood plan. A historical project must not be treated as the current
source of truth for `examples/replan-self`; create or deliberately select the
current project and bind it through ignored private storage.

Implemented bootstrap dry-run helper:

```sh
./scripts/plan-openproject-bootstrap.sh
```

The helper:

- reads `canonical/current_plan.yaml` for task ordering;
- reads `canonical/tasks.yaml` for task titles, descriptions, dates, statuses,
  assignee hints, tags and existing OpenProject source refs;
- reads `canonical/resource_demands.yaml` to derive a conservative
  `estimated_hours_from_hour_demands` value from hour-based remaining or planned
  demands;
- reads `canonical/dependencies.yaml` and emits dependency edges as
  `defer_until_work_package_ids_exist`;
- maps RePlan tracking statuses into suggested OpenProject statuses:
  - `not_started` -> `New`;
  - `ready` -> `To be scheduled`;
  - `in_progress` -> `In progress`;
  - `blocked` -> `On hold`;
  - `review_ready` -> `Developed`;
  - `accepted` -> `Closed`;
  - `cancelled` -> `Rejected`;
- outputs JSON only and performs no OpenProject mutation.

Observed bootstrap dry-run expectation:

- target OpenProject project identifier should be a deliberate operator
  decision, with `replan-self` as the clean default for a current self-dogfood
  project;
- accepted decision: create or use a new `replan-self` live OpenProject project
  for the current self-dogfood task skeleton;
- accepted decision: assign the first OpenProject task skeleton to the
  maintainer/operator where assignment is needed;
- accepted decision: keep private live OpenProject work-package/user/relation
  ids in an ignored private binding layer, not in public canonical
  `examples/replan-self` files;
- an old private project should remain historical data unless the operator
  explicitly decides to overwrite or repurpose it;
- work packages should be created or matched before any dependency relations
  are created;
- OpenProject source refs should be accepted into public canonical files only
  for sanitized examples; live dogfood ids should be written to private
  bindings after the alignment helper reports `alignable_by_title` or
  `aligned_by_source_ref`.

Concrete synchronization backlog from this point:

1. Review the resource pool through `scripts/review-resource-pool.sh` and
   confirm that OpenProject bootstrap can proceed with a task skeleton while
   RePlan resource-sidecar details continue to evolve separately.
2. Review `plan-openproject-bootstrap` output and decide the live OpenProject
   target project identifier.
3. Completed: add an explicit apply script that can create or select the target
   OpenProject project in dry-run/apply mode.
4. Completed: create/select the target OpenProject project.
5. Completed: use the apply script to create missing work packages from the
   reviewed bootstrap plan.
6. Completed: re-run `check-openproject-live-alignment` against the target
   project.
7. Completed: store live source refs in the ignored private binding layer.
8. Completed: add OpenProject relations only after stable work-package ids
   exist.
9. Completed initial pass: add a regular drift report to the operator loop so
   OpenProject and RePlan do not silently diverge again.

Implemented apply helper:

```sh
./scripts/apply-openproject-bootstrap.sh
```

The helper:

- requires `REPLAN_OPENPROJECT_BASE_URL`;
- reads the token from `OPENPROJECT_TOKEN` by default;
- uses `replan-self` as the default target OpenProject project identifier;
- uses an explicitly configured OpenProject user as the default first-pass
  assignee when the operator chooses one;
- is dry-run by default;
- applies only when `REPLAN_APPLY=1`;
- creates the target OpenProject project when it is missing and apply is
  enabled;
- reads existing work packages and matches by exact normalized subject to avoid
  duplicates;
- creates missing work packages with title, markdown description, manual
  schedule, dates, estimated time, status and assignee;
- creates a maintainer/operator project membership when needed so assignment is
  valid in a new OpenProject project;
- uses the global `/api/v3/work_packages` endpoint with a project filter when
  checking existing work packages, because the project-scoped endpoint omits
  closed work packages on the current OpenProject 17 instance;
- writes live task/user bindings only to ignored private storage such as
  `private/openproject/replan-self-bindings.yaml`;
- deliberately does not create OpenProject relations on the first pass.

Observed apply result on 2026-07-30:

- target live OpenProject project: `replan-self`;
- OpenProject project id: private/live id stored only in ignored binding data;
- first apply created the target project;
- first work-package apply needed an additional maintainer/operator project
  membership before assignment was valid;
- recovered apply created 22 work packages;
- private binding file was written to
  `private/openproject/replan-self-bindings.yaml`;
- binding file contains 22 task bindings and 1 maintainer/operator user
  binding;
- repeated dry-run apply is idempotent: 22 existing work packages matched, 0
  new work packages planned.

Observed alignment after bootstrap on 2026-07-30:

- `live_alignment`: `aligned_by_private_binding`;
- local tasks: 22;
- live task candidates: 22;
- matched by private binding: 22;
- matched by committed source ref: 0;
- matched by title: 0;
- live-unmatched tasks: 0;
- local-unmatched tasks: 0;
- warnings: none;
- errors: none.

Implemented relation apply helper:

```sh
./scripts/apply-openproject-relations.sh
```

The helper:

- requires `REPLAN_OPENPROJECT_BASE_URL`;
- reads the token from `OPENPROJECT_TOKEN` by default;
- uses `replan-self` as the default target OpenProject project identifier;
- reads canonical RePlan dependencies from
  `examples/replan-self/canonical/dependencies.yaml`;
- reads live OpenProject work-package ids from the ignored private binding
  layer, for example `private/openproject/replan-self-bindings.yaml`;
- is dry-run by default;
- applies only when `REPLAN_APPLY=1`;
- lists existing OpenProject relations and matches by relation type plus
  `from` and `to` work-package ids, so repeated runs do not create duplicates;
- maps RePlan `finish_to_start` to OpenProject `follows`;
- uses the OpenProject direction for `follows`: the relation `from` work
  package is the successor and the relation `to` work package is the
  predecessor;
- maps RePlan `blocks` to OpenProject `blocks`, with the predecessor as
  `from` and the successor as `to`;
- creates relations through the work-package `addRelation` endpoint,
  `/api/v3/work_packages/<from_work_package_id>/relations`, because the global
  `/api/v3/relations` endpoint is readable but not the creation endpoint on the
  current OpenProject 17 instance;
- writes live dependency/relation bindings only to ignored private storage.

Observed relation apply result on 2026-07-30:

- target live OpenProject project: `replan-self`;
- dry-run before apply planned 22 relation creations and found 0 errors;
- apply created 22 OpenProject `follows` relations;
- private binding file was updated with 22 dependency relation bindings;
- repeated dry-run is idempotent: 22 existing relations matched, 0 new
  relations planned;
- no public canonical file received live OpenProject relation ids.

Observed alignment after relation apply on 2026-07-30:

- `live_alignment`: `aligned_by_private_binding`;
- local tasks: 22;
- live task candidates: 22;
- matched by private binding: 22;
- live-unmatched tasks: 0;
- local-unmatched tasks: 0;
- dependency candidates: 22;
- time-entry candidates: 0;
- resource candidates: 1;
- warnings: none;
- errors: none.

Observed live validation after relation apply on 2026-07-30:

- `live_api_validation`: `passed`;
- task candidates: 22;
- dependency candidates: 22;
- time-entry candidates: 0;
- resource candidates: 1;
- resource-demand candidates: 0;
- dependency accept dry-run changes: 22;
- warning codes: none;
- error codes: none.

Implemented drift report helper:

```sh
./scripts/check-openproject-drift.sh
```

The helper:

- is read-only;
- requires live OpenProject access through either explicit
  `REPLAN_OPENPROJECT_BASE_URL` plus `REPLAN_OPENPROJECT_PROJECT_ID`, or a
  configured OpenProject source profile;
- reads the token from `OPENPROJECT_TOKEN` by default;
- imports live OpenProject data with closed work packages and relations enabled
  by default;
- reads current RePlan tasks from `canonical/current_plan.yaml` and
  `canonical/tasks.yaml`;
- reads canonical RePlan dependencies from
  `canonical/dependencies.yaml`;
- reads live OpenProject task and relation ids from the ignored private binding
  layer;
- compares task identity, task title, mapped tracking status, planned dates,
  dependency identity and dependency relation bindings;
- reports hard drift separately from soft drift;
- emits JSON only, so AI CLI agents can use it as an operator-loop gate.

Current drift statuses:

- `aligned`: task identity, dependency identity, status and date surface match
  the current RePlan plan;
- `soft_drift`: task and dependency identity match, but status or date
  differences require operator review;
- `drift`: identity, task or dependency differences exist, so the operator
  should not mutate OpenProject or accept imports before reviewing the report;
- `skipped`: no token was available;
- `failed`: live import failed.

Observed drift report after relation apply on 2026-07-30:

- `drift_status`: `soft_drift`;
- local tasks: 22;
- live task candidates: 22;
- local dependencies: 22;
- live dependency candidates: 22;
- unbound local tasks: 0;
- missing live tasks: 0;
- extra live tasks: 0;
- title drifts: 0;
- unbound local dependencies: 0;
- missing live dependencies: 0;
- extra live dependencies: 0;
- relation binding mismatches: 0;
- status drifts: 1;
- date drifts: 42;
- warning codes: none;
- error codes: none.

Interpretation:

The task and dependency skeleton is aligned. The remaining drift is not an
identity problem. It is an operator decision problem:

- one OpenProject status still maps to RePlan `not_started` while the local
  RePlan task is `ready`;
- OpenProject dates differ from RePlan planned dates after relation creation,
  most likely because OpenProject applies its own scheduling and working-day
  rules once `follows` relations exist.

The next planning step should decide which date surface is authoritative for
the current self-dogfood project:

- accept OpenProject's scheduled dates back into RePlan;
- push RePlan dates back to OpenProject after reviewing why OpenProject moved
  them;
- or keep date drift as a known soft drift while RePlan resource calibration is
  still the primary goal.

Initial drift policy decision on 2026-07-30:

For the current self-dogfood stage, OpenProject is the source for
human-visible task management and schedule display, while RePlan remains the
resource-planning sidecar. This means:

- task identity drift is hard drift;
- missing or extra live work packages are hard drift;
- title drift is hard drift, because it can make the operator inspect or mutate
  the wrong task;
- dependency identity drift is hard drift;
- missing or extra live relations are hard drift;
- relation binding mismatches are hard drift;
- OpenProject source status versus RePlan `tracking_status` mismatch is soft
  drift;
- OpenProject date versus RePlan `planned_start` or `planned_finish` mismatch
  is soft drift.

The reason status drift is soft:

- OpenProject status is a source fact from the human PM system;
- RePlan `tracking_status` is an accepted local planning/tracking state;
- importing a status and accepting it are separate actions;
- RePlan must not silently rewrite its accepted tracking state just because the
  source system changed or because the source status mapping is lossy.

The reason date drift is soft:

- OpenProject may reschedule dates when `follows` relations are created;
- OpenProject may apply working-day and scheduling rules that differ from the
  original hand-written RePlan plan dates;
- RePlan resource calibration is not yet the authoritative schedule engine;
- RePlan must not silently accept OpenProject date movement into canonical
  `planned_start` and `planned_finish`;
- RePlan must also not silently push local dates back to OpenProject before the
  operator understands why OpenProject moved them.

Operator gate:

- `aligned`: safe to continue the next operator-loop step;
- `soft_drift`: safe to inspect and plan, but do not silently mutate
  OpenProject or accept imports before reviewing the status/date differences;
- `drift`: stop mutation, review identity/task/dependency differences first.

Current observed live OpenProject behavior:

- the first work package in the chain remained manually scheduled;
- dependent work packages are live-scheduled by OpenProject after `follows`
  relations;
- this explains the large date drift count without indicating broken task or
  dependency identity.

Implemented dates-only soft-drift accept helper:

```sh
./scripts/accept-openproject-soft-drift.sh
```

The helper:

- imports live OpenProject task data;
- reads current private task bindings from ignored private storage;
- compares OpenProject `startDate` and `dueDate` with RePlan `planned_start`
  and `planned_finish`;
- previews date changes by default;
- applies only when `REPLAN_APPLY=1`;
- does not accept OpenProject status into RePlan `tracking_status`;
- reports status differences as `status_reviews`;
- preserves the existing `tasks.yaml` formatting style by changing only the
  planned date lines.

Observed dry-run before apply on 2026-07-30:

- date changes: 42;
- status reviews: 1;
- unmatched candidates: 0;
- applied date changes: 0.

Observed apply result on 2026-07-30:

- date changes: 42;
- applied date changes: 42;
- status reviews: 1;
- unmatched candidates: 0.

Observed drift report after dates-only accept on 2026-07-30:

- `drift_status`: `soft_drift`;
- task identity drift: 0;
- dependency identity drift: 0;
- date drift: 0;
- status drift: 1.

Interpretation after dates-only accept:

RePlan now accepts OpenProject's scheduled dates for the current self-dogfood
task skeleton. The remaining `soft_drift` is only the intentional status review
for `task.self.expand-openproject-fixtures`, where OpenProject source status is
`New` and RePlan `tracking_status` remains `ready`.

Implemented status push helper:

```sh
./scripts/push-openproject-status-drift.sh
```

The helper:

- imports live OpenProject task data;
- reads current private task bindings from ignored private storage;
- compares OpenProject source status with the status implied by RePlan
  `tracking_status`;
- previews status pushes by default;
- applies only when `REPLAN_APPLY=1`;
- uses `lockVersion` when patching an OpenProject work package;
- checks the work-package form before apply so disallowed workflow transitions
  are reported as skipped actions instead of causing an opaque API failure;
- defaults to `REPLAN_OPENPROJECT_STATUS_SCOPE=ready_only` for conservative
  operation;
- supports `REPLAN_OPENPROJECT_STATUS_SCOPE=all` for explicitly reviewed
  broader status reconciliation.

Status reconciliation decision on 2026-07-30:

The original intended push was RePlan `ready` to OpenProject
`To be scheduled`. The live OpenProject workflow rejected that transition from
`New`, and the work-package form showed that `To be scheduled` was not an
allowed value for the current work package and user role.

The operator decision was to represent the current self-dogfood sync block as
actively executing:

- RePlan task `task.self.expand-openproject-fixtures` was changed from
  `ready` to `in_progress` through the normal `actual add-status` command;
- the status change was recorded in `examples/replan-self/audit/`;
- OpenProject work package 80 was then pushed from `New` to `In progress`
  through `push-openproject-status-drift.sh` with explicit
  `REPLAN_OPENPROJECT_STATUS_SCOPE=all`.

Observed status push result on 2026-07-30:

- dry-run before apply: 1 status action, 0 skipped;
- apply: 1 status action, 0 skipped;
- updated OpenProject status: `In progress`.

Observed drift report after status push on 2026-07-30:

- `drift_status`: `aligned`;
- local tasks: 22;
- live task candidates: 22;
- local dependencies: 22;
- live dependency candidates: 22;
- task identity drift: 0;
- dependency identity drift: 0;
- title drift: 0;
- date drift: 0;
- status drift: 0;
- warning codes: none;
- error codes: none.

Immediate follow-up:

The current OpenProject/RePlan task-source sync block is now aligned at the
task, dependency, date and status level. The next slice should move from source
sync plumbing to resource-sidecar calibration and operator-facing resource
planning:

- review the resource pool after the OpenProject schedule shift;
- compare planned resource demand dates with the updated task dates;
- decide whether resource-demand dates need recalibration;
- update the RePlan dashboard/report so the aligned OP/RePlan state is visible
  to the operator.

## Reviewed RePlan-To-OpenProject Date Write-Back

Status: designed and added to backlog on 2026-08-02.

### Problem

RePlan is no longer only receiving dates from OpenProject.

After resource calibration, RePlan can find that the OpenProject-visible task
schedule is not feasible for the declared resource pool. The first concrete
case is `scenario.reviewer-capacity-strict-replan`: RePlan moved two
self-dogfood tasks so reviewer-role capacity is no longer overloaded.

That creates a real synchronization question:

- OpenProject remains the human-facing project-management system;
- RePlan is the resource-planning sidecar;
- a RePlan scenario may produce better task dates than the current
  OpenProject schedule;
- if those dates stay only in RePlan, humans looking at OpenProject see an old
  plan;
- if RePlan blindly overwrites OpenProject, the source system can be corrupted
  by an agent, a stale binding or a mistaken scenario.

The missing capability is a safe, reviewed, dates-only write-back path from
RePlan to OpenProject.

### Decision

Implement RePlan-to-OpenProject date write-back as an explicit operator action,
not as automatic bidirectional synchronization.

The intended loop is:

1. OpenProject owns the visible task list and the normal human Gantt view.
2. RePlan imports or compares the current OpenProject state.
3. RePlan detects resource overload, quota pressure or another resource-side
   scheduling problem.
4. A RePlan scenario proposes concrete task date changes.
5. The operator reviews the scenario and applies it to RePlan.
6. RePlan generates an OpenProject date push plan.
7. The operator reviews the push plan.
8. RePlan applies the push only when an explicit apply gate is set.
9. RePlan immediately reruns the OpenProject drift report.
10. The target result is `aligned` or an explicitly documented remaining
    `soft_drift`.

Short form:

```text
OpenProject -> RePlan: import source facts.
RePlan -> OpenProject: reviewed resource-justified date corrections.
No silent two-way sync.
```

### First Supported Field Scope

The first write-back scope is deliberately narrow:

- RePlan `planned_start` to OpenProject `startDate`;
- RePlan `planned_finish` to OpenProject `dueDate`.

When OpenProject has the target work package in automatic scheduling mode, the
date push also sets `scheduleManually: true` for that work package. Without
this, OpenProject can immediately recalculate the pushed dates from relations
and the helper cannot make RePlan's reviewed resource schedule visible in the
OpenProject Gantt.

An OpenProject 17.6.0 probe on 2026-10-29 showed that creating a `follows`
relation switched its manually dated successor to `scheduleManually: false`
and moved it to the day after its predecessor. Restoring
`scheduleManually: true` with the prior dates kept the successor fixed when the
predecessor moved again, so relation apply now performs and verifies that
restore for bound successor chains.

Do not include in the first date write-back helper:

- title changes;
- description changes;
- work-package creation;
- work-package deletion;
- relation creation or deletion;
- status transitions;
- estimate updates;
- assignee updates;
- resource-demand serialization into OpenProject custom fields;
- comments;
- time entries;
- automatic acceptance of OpenProject-scheduled dates back into RePlan.

Those are separate contracts. Some already have one-off helpers, such as the
status drift push helper. They must not be hidden inside a generic date push.

### Safety Model

The helper must be safe for AI CLI agents to run.

Required properties:

- dry-run is the default;
- apply requires an explicit environment gate such as `REPLAN_APPLY=1`;
- live OpenProject access requires explicit source settings and
  `OPENPROJECT_TOKEN`;
- private live ids are read from ignored private bindings, not committed public
  files;
- no write is attempted for a task without a trusted OpenProject binding;
- no write is attempted when hard OpenProject/RePlan drift is present;
- no write is attempted when the live work package has changed since the
  comparison snapshot in a way that makes the plan stale;
- each apply action uses OpenProject `lockVersion`;
- each skipped or blocked action is reported as structured JSON;
- after apply, the operator must rerun `check-openproject-drift.sh`.

The design should prefer refusing to write over guessing.

### Conflict Rules

The date write-back planner should classify each candidate task.

Planned action:

- the task has a local RePlan date difference versus OpenProject;
- the private binding identifies one live OpenProject work package;
- title/source identity still matches;
- dependency identity is not drifting;
- OpenProject allows the date fields to be updated;
- the work package is not stale according to `lockVersion`.

Blocked action:

- missing private binding;
- duplicate or ambiguous binding;
- missing OpenProject token or source profile;
- live work package not found;
- live title/source identity mismatch;
- hard dependency drift;
- OpenProject form does not allow updating the date fields;
- stale `lockVersion`;
- OpenProject API rejects the update.

Review action:

- the work package is closed or otherwise appears completed;
- the new RePlan dates move a completed task;
- the new RePlan dates violate a visible OpenProject scheduling constraint;
- OpenProject would reschedule dependent work packages after the update;
- the task date change came from a scenario that has not been applied to RePlan.

The planner may show review actions, but apply should skip them unless a later
explicit override contract is designed.

### Command Shape

The first implementation can be a script, matching the current integration
style:

```sh
./scripts/push-openproject-date-drift.sh
```

Default dry-run:

```sh
./scripts/push-openproject-date-drift.sh
```

Explicit apply:

```sh
REPLAN_APPLY=1 ./scripts/push-openproject-date-drift.sh
```

Expected environment:

- `REPLAN_OPENPROJECT_BASE_URL`;
- `REPLAN_OPENPROJECT_PROJECT_ID` or source profile equivalent;
- `OPENPROJECT_TOKEN`;
- `REPLAN_OPENPROJECT_BINDINGS_PATH`, defaulting to ignored private bindings
  for the live project.
- `REPLAN_OPENPROJECT_TIMEOUT_SECONDS`, defaulting to `60`;
- `REPLAN_OPENPROJECT_RETRIES`, defaulting to `0` for Python operator helpers;
- `REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS`, defaulting to `2`.

`replan import openproject` also accepts `--request-timeout` for a per-command
override. Live drift checks that call the importer use
`REPLAN_OPENPROJECT_TIMEOUT_SECONDS`. Python OpenProject operator helpers use
the timeout plus retry variables and include the resolved request timing in
their JSON output. This keeps slow Rails boot, proxy latency and protected-path latency
visible to AI CLI agents without requiring raw ad hoc OpenProject API calls.

Expected output:

- JSON summary;
- candidate count;
- planned action count;
- review action count;
- blocked action count;
- applied action count;
- per-task records with task id, OpenProject work-package id, old dates, new
  dates, action status and reason code;
- no secrets or private endpoint details in committed artifacts.

### Audit And Evidence

OpenProject itself stores the work-package update history. RePlan should also
record enough local evidence to explain why the push happened.

For the first slice, the minimum accepted evidence is:

- the RePlan scenario apply audit id that changed local dates;
- the date push dry-run JSON saved outside public committed private data when
  it contains live ids;
- the apply command output summarized in this roadmap or in a sanitized
  operator note;
- the follow-up `check-openproject-drift.sh` result.

Do not commit live work-package ids, user ids, tokens or private endpoint names
to the public repository.

### Current Self-Dogfood Candidate

The immediate candidate is the reviewer-capacity replan applied on 2026-08-02.

Expected RePlan date pushes:

- `task.self.write-oss-readiness-audit`: `2026-08-03` to `2026-08-09`;
- `task.self.prepare-release-story`: `2026-08-10` to `2026-08-23`.

Before applying to OpenProject:

1. run the OpenProject drift report;
2. confirm there is no hard task or dependency drift;
3. generate the date push plan;
4. review whether OpenProject already has equivalent dates;
5. apply only if OpenProject is still stale;
6. rerun drift report and resource schedule drift report.

### Backlog

#### OP-RP-DATE-PUSH-001. Record The Write-Back Design

Status: completed on 2026-08-02.

Scope:

- document the reviewed date write-back contract;
- update the operator-loop wording so it allows explicit safe write-back
  helpers without allowing broad automatic mutation;
- keep the design generic and OSS-safe.

Done when:

- this section exists;
- the operator-loop document no longer says that every OpenProject write-back is
  forbidden in the MVP;
- non-goals are narrowed to broad/automatic write-back instead of all
  write-back.

#### OP-RP-DATE-PUSH-002. Implement Date Push Planner

Status: completed on 2026-08-02.

Scope:

- add a dry-run helper that compares RePlan `planned_start` and
  `planned_finish` with live OpenProject `startDate` and `dueDate`;
- read private bindings from ignored storage;
- classify actions as planned, review or blocked;
- output structured JSON;
- never mutate OpenProject in this task.

Done when:

- the helper can be run safely without `REPLAN_APPLY=1`;
- it identifies the reviewer-capacity date drift candidates;
- it refuses missing bindings and hard drift;
- it reports stable reason codes for AI CLI agents.

Implementation:

- `scripts/push-openproject-date-drift.sh`;
- default mode is dry-run;
- live dry-run on 2026-08-02 reported 2 planned actions, 0 review actions and
  0 blocked actions for the reviewer-capacity replan;
- fixture mode can read `REPLAN_OPENPROJECT_IMPORT_JSON` so CI and AI CLI
  agents can exercise the planner without private OpenProject access.

#### OP-RP-DATE-PUSH-003. Implement Explicit Date Apply

Status: completed on 2026-08-02.

Scope:

- add apply mode behind `REPLAN_APPLY=1`;
- patch OpenProject work-package dates with `lockVersion`;
- skip review and blocked actions;
- summarize all applied and skipped actions.

Done when:

- dry-run remains the default;
- apply changes only `startDate` and `dueDate`;
- stale `lockVersion` or disallowed fields do not cause partial silent
  corruption;
- no private details are committed.

Implementation:

- apply mode is behind `REPLAN_APPLY=1`;
- apply is refused when the planner has blocked actions;
- apply is refused when review actions are present;
- apply is refused when the helper is running from a fixture import JSON;
- apply fetches all live work packages first and refuses the whole write when
  any live date no longer matches the dry-run/import snapshot;
- apply then fetches each work package again immediately before PATCH so
  OpenProject relation rescheduling caused by an earlier PATCH in the same run
  does not create a stale `lockVersion`;
- apply patches with current `lockVersion`;
- apply sets `scheduleManually: true` together with `startDate` and `dueDate`;
- only `scheduleManually`, `startDate` and `dueDate` are patched.

#### OP-RP-DATE-PUSH-004. Validate Against Live OpenProject

Status: completed on 2026-08-02.

Scope:

- run drift report before date push;
- run dry-run date push;
- apply only if the operator accepts the plan;
- run drift report after apply;
- run resource schedule drift after apply.

Done when:

- OpenProject and RePlan are aligned or the remaining drift is explicitly
  classified;
- reviewer-capacity overload remains resolved;
- the public repository records only sanitized conclusions.

Observed live result on 2026-08-02:

- initial live dry-run reported 2 planned actions, 0 review actions and 0
  blocked actions;
- first apply attempt exposed an OpenProject scheduling/locking behavior:
  OpenProject relation scheduling changed dependent work-package lock versions
  during the same apply run;
- the helper was updated to set `scheduleManually: true`, verify all live dates
  before mutation and fetch a fresh `lockVersion` immediately before each
  PATCH;
- second apply completed with 2 applied actions;
- OpenProject normalized two Sunday finish dates to the next Monday:
  `2026-08-09` became `2026-08-10`, and `2026-08-23` became `2026-08-24`;
- those OpenProject-normalized finish dates were accepted back into RePlan
  through `accept-openproject-soft-drift.sh`;
- final `check-openproject-drift.sh` result: `aligned`, with 0 task identity
  drifts, 0 dependency identity drifts, 0 status drifts and 0 date drifts;
- final `check-resource-schedule-drift.sh` result: `aligned`, with 0 hard
  drifts, 0 calibration reviews and 0 overloaded weekly rows.

### Operator Dashboard Refresh After Date Alignment

Status: completed locally on 2026-08-02.

Backlog task:

```text
OP-RP-DASH-001: Refresh operator dashboard after OpenProject date alignment.
```

Reason:

- OpenProject and RePlan were aligned after the reviewed date push;
- resource schedule drift was aligned;
- the operator still needed a human-readable view of the resulting plan.

Action:

```sh
replan report dashboard \
  --project examples/replan-self \
  --week 2026-W31 \
  --output reports/latest \
  --format html
```

Observed output:

- local generated dashboard artifacts refreshed under
  `examples/replan-self/reports/latest/`;
- `raw/gantt.json` shows `task.self.write-oss-readiness-audit` from
  `2026-08-03` to `2026-08-10`;
- `raw/gantt.json` shows `task.self.prepare-release-story` from `2026-08-10`
  to `2026-08-24`;
- dashboard overview reports `overloaded_resources: 0`;
- dashboard overview still reports 1 warning and 1 next action: review or
  resolve `dependency.oss-audit-before-release-story`;
- local generated report artifacts are not committed as source-of-truth files.

Important observation:

The resource schedule drift helper reports no weekly overloads. The generated
Gantt resource-load section can still mark individual daily load bars as
`overloaded`, because it visualizes daily peaks using a simple per-day capacity
heuristic. That is a dashboard semantics/UX gap, not current schedule drift.

Follow-up backlog:

```text
OP-RP-DASH-002: Reconcile dashboard daily load markers with weekly resource
schedule drift semantics.
```

Scope:

- explain in the Gantt UI whether load markers are daily heuristic pressure or
  weekly schedule drift;
- avoid showing daily red overload markers in a way that contradicts the
  authoritative weekly `check-resource-schedule-drift` result;
- keep the dashboard read-only and generated.

#### OP-RP-DATE-PUSH-005. Add Sanitized Test Coverage

Status: completed on 2026-08-02.

Scope:

- add fixture coverage for date push planning without live OpenProject access;
- cover missing binding, changed title, hard drift and normal planned action;
- keep apply tests mocked or fixture-based.

Done when:

- CI exercises the planner without private infrastructure;
- AI agents can trust the dry-run contract before touching live OpenProject.

Implementation:

- added `examples/openproject/private-bindings.replan-self.example.yaml`;
- extended `scripts/smoke-mvp.sh` to generate a public import JSON from
  `examples/imports/openproject/replan-self.work-packages.sample.json`;
- smoke runs `scripts/push-openproject-date-drift.sh` through
  `REPLAN_OPENPROJECT_IMPORT_JSON` and sanitized bindings;
- smoke asserts planned actions, review actions, blocked action count and the
  expected RePlan task ids.

#### OP-RP-DATE-PUSH-006. Document Operator Procedure

Status: completed on 2026-08-02.

Scope:

- add a short command sequence for the operator;
- explain when to accept OpenProject dates into RePlan versus push RePlan dates
  into OpenProject;
- document failure modes and expected next actions.

Done when:

- a future agent can run the procedure from docs without asking for hidden
  context;
- the procedure keeps private credentials and bindings outside the public
  repository.

Implemented resource schedule drift helper:

```sh
./scripts/check-resource-schedule-drift.sh
```

The helper is read-only and checks whether the resource sidecar is still
credible after accepting OpenProject scheduled task dates into RePlan.

The helper:

- reads `canonical/tasks.yaml`, `canonical/resource_demands.yaml`,
  `canonical/capacities.yaml`, `canonical/resources.yaml` and
  `canonical/current_plan.yaml`;
- treats a resource demand without explicit `timing` as inheriting the parent
  task `planned_start` and `planned_finish`;
- treats explicit demand timing outside the task window as a calibration review
  item;
- treats missing task references, invalid dates, reversed date windows and
  missing demand resource/unit data as hard drift;
- prorates demand amounts across calendar days in the effective demand window;
- aggregates weekly planning load by resource and unit;
- uses `remaining_amount` for active demands when it is present, including an
  explicit zero, otherwise uses `planned_amount` (changed on 2026-09-24, see
  GAP-016);
- compares weekly planning load with weekly capacity;
- reports overloads, missing capacity, active demands on accepted tasks and
  active demands without `remaining_amount`.

Current statuses:

- `aligned`: no hard drift and no calibration review items;
- `needs_calibration`: no hard drift, but resource demands/capacities are not
  reliable enough for planning without operator review;
- `drift`: hard resource schedule drift exists.

Observed resource schedule drift report on 2026-08-02:

- `resource_schedule_status`: `needs_calibration`;
- tasks: 22;
- resource demands: 65;
- explicit demand timing entries: 0;
- demands inheriting task window: 65;
- hard drifts: 0;
- calibration reviews: 64;
- weekly rows: 20;
- overloaded weekly rows: 6;
- missing capacity weekly rows: 0.

Observed calibration review counts:

- active demands on accepted tasks: 32;
- active demands without `remaining_amount`: 32.

Observed overloaded weekly rows:

- `2026-W30`, `resource.human.maintainer`, `hours`: planning load `14.8571`,
  capacity `10`, overload `4.8571`;
- `2026-W30`, `resource.role.reviewer`, `review_hours`: planning load
  `2.8571`, capacity `2`, overload `0.8571`;
- `2026-W31`, `resource.human.maintainer`, `hours`: planning load `36.8571`,
  capacity `10`, overload `26.8571`;
- `2026-W31`, `resource.role.reviewer`, `review_hours`: planning load
  `3.2857`, capacity `2`, overload `1.2857`;
- `2026-W32`, `resource.human.maintainer`, `hours`: planning load `17.2857`,
  capacity `10`, overload `7.2857`;
- `2026-W33`, `resource.human.maintainer`, `hours`: planning load `14`,
  capacity `10`, overload `4`.

Interpretation:

The OpenProject/RePlan task schedule is aligned, but the resource sidecar is
not yet calibrated enough to trust workload numbers. The biggest issue is not
broken references. It is stale or incomplete planning data:

- many accepted tasks still have active demands;
- many active demands have no explicit remaining work;
- maintainer and reviewer capacity is overloaded in the current schedule;
- Codex `agent_hours` are under nominal weekly capacity, but subscription-limit
  pressure is still not represented as a public capacity or quota tracker.

Immediate resource-calibration follow-up:

- decide whether active demands on accepted tasks should be marked consumed or
  cancelled;
- add or update `remaining_amount` for genuinely active tasks;
- review whether maintainer weekly capacity `10 hours` is realistic for the
  current schedule;
- review whether reviewer weekly capacity `2 review_hours` is realistic;
- decide how Codex subscription limits should be represented separately from
  generic `agent_hours`;
- update the dashboard/report so the operator can see this resource schedule
  state without reading raw JSON.

Importer fix discovered by bootstrap:

- OpenProject project-scoped work-package endpoint returned only active work
  packages for the new project;
- the global `/api/v3/work_packages` endpoint with a project filter returned
  closed and active work packages;
- RePlan import now resolves the OpenProject project identifier to numeric
  project id and uses the global endpoint when `--include-closed` is set.

### 6. Close Or Replan The Block

After fixture and live validation, decide one next step.

Possible next steps:

- create or update the next real backlog items in OpenProject and RePlan;
- run the OSS boundary checkpoint without treating it as a public release gate;
- run a full OpenProject-centered dogfood cycle before public release story
  work;
- design the OpenProject-native RePlan surface, including possible plugin or
  panel paths;
- improve dashboard comparison between OpenProject and RePlan;
- reconcile dashboard daily load markers with authoritative weekly resource
  schedule drift semantics.

Backlog convention:

- agreed non-trivial future work must be represented in the OpenProject/RePlan
  dogfood backlog;
- OpenProject owns human-facing task identity, dates, statuses, relations and
  Gantt;
- RePlan owns resource demands, capacity, scenarios, evidence, proposals,
  actuals and audit;
- documentation records decisions and rationale but should not be the only
  backlog for work that has been accepted for execution;
- if live OpenProject is unavailable, record the work with OpenProject sync
  pending and sync it later.

Expected output:

- accepted actuals and evidence for
  `task.self.expand-openproject-fixtures`;
- remaining work set to zero if the block is done;
- next task selected and documented.

Execution result, 2026-08-02:

- `task.self.expand-openproject-fixtures` was accepted after live
  OpenProject/RePlan backlog materialization;
- evidence `evidence.openproject.replan-backlog-materialized` records the
  synchronized backlog update;
- actuals were recorded for the maintainer, Codex and reviewer demands on the
  source-sync block;
- all three `resource_demand.openproject-fixtures.*` demands were marked
  consumed;
- all three `resource_demand.openproject-fixtures.*` stale remaining amounts
  were cleared after tracking suggestions confirmed the stale remaining-work
  risk;
- OpenProject work package `80` was pushed from `In progress` to `Closed`;
- OpenProject/RePlan drift is aligned for `26` tasks and `29` dependencies.

Selected next task:

`task.self.write-oss-readiness-audit`, now titled `Run OSS boundary checkpoint`.

This next task is intentionally a bounded boundary checkpoint. It does not start
the public release story.

## Acceptance Criteria

The block is done when:

- OpenProject-primary task ownership is documented;
- RePlan-owned resource sidecar ownership is documented;
- sanitized fixture import can be run by an AI CLI agent without private access;
- live validation has either been completed or explicitly deferred with reason;
- any private observations are summarized without leaking private details;
- RePlan workload and state reports show no stale remaining work for this block;
- the next implementation or documentation slice is selected.

## Non-Goals

Do not build in this block:

- broad or automatic OpenProject write-back;
- deep OpenProject plugin implementation;
- full web UI overlay;
- custom OpenProject field management UI;
- server daemon;
- RBAC or authentication changes;
- TaskJuggler solver integration;
- private-pilot narrative cleanup unrelated to this source-sync decision.

These may become later tasks, but this block is specifically about proving the
OpenProject/RePlan source boundary and import-sidecar workflow.
