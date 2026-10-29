# OpenProject and RePlan operator loop

Date: 2026-07-18

Status: active generic operator-loop plan.

## Purpose

This document records how OpenProject and RePlan should be used together during
the first public-friendly dogfood and pilot workflows.

The goal is not to design another project-management suite.

The goal is to make one practical operator loop where:

- OpenProject can be the human task and project surface;
- RePlan imports OpenProject tasks and relations;
- RePlan owns resource planning, workload analysis, scenarios, evidence,
  proposals and audit;
- the operator can see the project state without reading only raw JSON;
- AI CLI agents can work through stable commands and dry-run/proposal flows.

## Current conclusion

OpenProject should be treated as the first external project-management
substrate.

RePlan should not become a standalone task tracker for this phase.

Approved operating split:

```text
OpenProject:
    task identity
    work-package list
    task status
    assignee hint
    estimates
    dates
    relations
    Gantt and human project views
    comments and ordinary collaboration

RePlan:
    resource demands
    resource capacity
    availability
    non-human resources
    quotas
    budgets
    reservations
    actual consumption evidence
    baseline/current/actual comparison
    workload reports
    scenario diffs
    warnings
    proposals
    audit records
    AI-safe planning interface
```

This means OpenProject remains useful even before RePlan has a web UI. The
operator should use OpenProject UI for ordinary project visibility and use
RePlan CLI/reports for resource-specific decisions.

## Public-friendly dogfood direction

The primary public dogfood project should be RePlan itself.

Working example:

```text
examples/replan-self/
```

This avoids mixing private pilot data into the OSS product repository.

The self-dogfood project should model:

- RePlan implementation tasks;
- documentation work;
- OpenProject adapter work;
- resource-demand sidecar work;
- static observability dashboard work;
- TaskJuggler integration work;
- release and OSS-readiness work;
- CI and review constraints;
- AI CLI agent capacity and API quota.

Private real-world pilots may continue outside this repository, but the public
RePlan repository should only keep generic lessons learned.

The detailed separation policy is documented in
[`../dev/oss-separation-plan.md`](../dev/oss-separation-plan.md).

## Generic OpenProject deployment assumptions

The operator loop should work with any reachable OpenProject instance.

Example source profile shape:

```text
base_url: https://op.example.internal
project_id: replan-self
token_env: OPENPROJECT_TOKEN
```

The exact deployment is outside this repository. It may be:

- local Docker Compose;
- a private hosted instance;
- a company OpenProject instance;
- a temporary test instance;
- a public demo instance with no private data.

The RePlan repository should not depend on a private endpoint for tests,
examples or documentation.

## Verified integration capabilities

The implemented OpenProject integration can already support the operator loop:

- read-only work-package import;
- work-package status mapping;
- assignee/resource candidate mapping;
- estimate and date mapping;
- optional closed work-package import;
- optional time-entry import as actual-consumption evidence candidates;
- optional relation/dependency candidate import;
- explicit local accept flows with dry-run/apply separation;
- audit records for accepted local RePlan mutations;
- limited OpenProject write-back only through explicit reviewed helpers, with
  dry-run by default, apply gates and drift checks.

This proves that RePlan can use OpenProject as a task source while keeping
resource planning and audit local to RePlan.

## Why the next step is operator loop, not new core design

The functional design for planning and tracking is already closed enough for
implementation:

- planning lifecycle is approved;
- tracking lifecycle is approved;
- task state model is approved;
- resource demand model is approved;
- capacity and availability rules are approved;
- workload calculation rules are approved;
- planned/current/actual comparison is approved;
- dependency and deadline behavior is approved;
- scenario behavior is approved;
- evidence and proposal behavior is approved;
- PM-system interaction is approved.

The next risk is not that the model lacks another theoretical entity.

The next risk is that the operator still has no simple daily habit for using
OpenProject and RePlan together.

Therefore the next design and implementation work should focus on:

- what the operator opens in OpenProject;
- which RePlan commands are run;
- where RePlan output is written;
- which warnings matter;
- which actions remain manual;
- how AI CLI agents receive enough context to help safely.

## Target first operator loop

The first target loop should be simple and repeatable.

Daily or per-session human flow:

1. Open OpenProject project view.
2. Review tasks, statuses, assignees, dates and relations.
3. Update ordinary task information such as titles, assignees and comments in
   OpenProject. Draft planned dates in RePlan and push them only through a
   reviewed helper.
4. Run RePlan import from OpenProject.
5. Run RePlan validation and workload/report commands.
6. Inspect warnings, overloads, impossible deadlines and missing resource data.
7. Decide whether to update OpenProject manually, edit RePlan sidecar data, or
   ask an AI CLI agent to propose changes.

Weekly or planning-session flow:

1. Create or refresh a RePlan snapshot from OpenProject.
2. Generate workload and weekly report artifacts.
3. Run scenario diffs for important what-if cases.
4. Review dependency and deadline warnings.
5. Record actual consumption evidence where it is known.
6. Accept only explicit proposals.
7. Commit useful RePlan state changes through Git.

AI CLI agent flow:

1. Read OpenProject-derived RePlan snapshot and canonical sidecar data.
2. Run validation and reports.
3. Produce proposals, not direct source mutations.
4. Use dry-run commands before any write.
5. Leave audit records for accepted RePlan-side mutations.
6. Never mutate OpenProject through an ad hoc API call.
7. Use only approved OpenProject write-back helpers, and only after reviewing
   their dry-run output.

## Live API Timing Contract

OpenProject live operations must assume that the API can be temporarily slow
after a container restart, plugin image replacement, Rails boot, proxy reload
or connection recovery.

The operator loop therefore uses bounded, explicit timing settings:

- `REPLAN_OPENPROJECT_TIMEOUT_SECONDS`: per-request timeout in seconds,
  defaulting to `60`;
- `REPLAN_OPENPROJECT_RETRIES`: retry count for transient timeout, network and
  server failures in Python OpenProject operator helpers, defaulting to `0`;
- `REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS`: retry delay in seconds, defaulting
  to `2`;
- `--request-timeout` on `replan import openproject` for a per-command timeout
  override.

Use the same settings for import, drift checks and narrow write-back helpers.
Every helper, including bootstrap, uses `REPLAN_OPENPROJECT_PROJECT_ID` for the
OpenProject identifier. Bootstrap uses `REPLAN_OPENPROJECT_PROJECT_NAME` only
when creating a project and otherwise defaults that name to the identifier.
The former `REPLAN_OPENPROJECT_TARGET_PROJECT_ID` and
`REPLAN_OPENPROJECT_TARGET_PROJECT_NAME` names are deprecated aliases for one
release; their use is warned, and conflicting old and new values are refused
before an OpenProject request.

For example, after a plugin deployment:

```sh
REPLAN_OPENPROJECT_TIMEOUT_SECONDS=180 \
REPLAN_OPENPROJECT_RETRIES=2 \
REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS=5 \
./scripts/openproject-work-package-toolkit.sh status list
```

Automatic retries apply to reads and to PATCH requests carrying a
`lockVersion` only; validation-only form calls are retried only when the
caller explicitly marks them idempotent.

This is not broad automatic synchronization. It is an operator-safety
mechanism: commands should fail with a clear JSON diagnostic instead of
hanging, and slow-but-recovering OpenProject should not force raw ad hoc API
calls.

## OpenProject human views to establish

The operator should not rely only on Markdown or JSON for task visibility.

OpenProject should provide the first human project views:

- all active work packages;
- current week work packages;
- blocked or on-hold work packages;
- overdue work packages;
- unassigned work packages;
- work packages missing estimates;
- work packages missing start or due dates;
- dependency/relation view;
- Gantt view;
- closed/recently completed work packages when tracking needs validation.

The first practical task is to decide whether these can be done with standard
OpenProject saved views and Gantt, or whether RePlan needs a small generated
operator status page.

## RePlan human outputs to establish

RePlan should provide the resource-planning side of the same operator loop.

Minimum useful outputs:

- workload by resource and week;
- warnings for overloads and unknown capacity;
- warnings for missing estimates or missing dates;
- dependency and deadline warning list;
- weekly Markdown report;
- weekly static HTML report;
- scenario diff output;
- proposal list and proposal detail output;
- audit records for accepted RePlan mutations.

The HTML report is enough for first human verification. A custom live web UI is
not required for the next step.

## Resource sidecar direction

OpenProject standard fields are not enough to represent RePlan's full resource
model.

The first public-friendly pilot should not immediately add many OpenProject
custom fields from theory.

Near-term decision:

- keep task identity, title, status, assignee, dates, estimates and relations in
  OpenProject or synthetic fixtures;
- keep resource demands that OpenProject cannot express in RePlan sidecar files;
- use OpenProject custom fields only after dogfood use proves which fields are
  repeatedly needed by the operator.

Sidecar data should cover:

- implementer effort;
- reviewer effort;
- AI-agent effort;
- API quota;
- token budget;
- subscription entitlement;
- environment reservation;
- equipment;
- budget;
- offline task resources;
- other non-hour or non-human constraints.

## Immediate next plan

### Step 1. Create self-dogfood RePlan project

Status: done.

Goal:

Create `examples/replan-self` as the default public dogfood project.

Expected result:

- the example validates locally;
- it contains RePlan implementation and release tasks;
- it uses human, AI-agent, review, CI, quota and documentation resources;
- it can generate workload and weekly reports.

Observed result:

- `examples/replan-self` exists;
- validation passes;
- workload for `2026-W31` shows maintainer and reviewer-role overloads;
- weekly Markdown and static HTML report generation works;
- scenario diff works for maintainer unavailability.

### Step 2. Document and inspect OpenProject operator views

Goal:

Define the first OpenProject views the operator should use for a RePlan
self-dogfood or equivalent synthetic project.

Expected result:

- list of required OpenProject saved views;
- decision whether standard OpenProject UI is enough for the first operator
  screen;
- gap list for anything RePlan must generate itself.

### Step 3. Run OpenProject import as an optional normal path

Goal:

Make OpenProject import usable without relying on private infrastructure.

Expected result:

- generic import command documented;
- sanitized fixture and/or self-dogfood source profile available;
- private endpoint details kept outside this repository.

### Step 4. Produce first self-dogfood RePlan operator report

Goal:

Generate workload and weekly report outputs from `examples/replan-self`.

Expected result:

- report artifact path;
- warnings and overloads reviewed;
- list of missing sidecar resource data.

### Step 5. Design the minimum sidecar resource convention for OpenProject tasks

Goal:

Make it easy to say what each OpenProject work package needs beyond ordinary
assignee and estimate.

Expected result:

- sidecar file naming and location;
- work-package id linking rule;
- supported resource demand fields;
- validation rules;
- examples for human, AI agent, quota, subscription, budget, equipment and
  offline resource demands.

### Step 6. Build generated static observability dashboard

Status: first implementation slice completed.

Goal:

Avoid building a web UI too early, but give the operator a clear project state
surface.

Expected result:

- generated static HTML is accepted as the first RePlan human-observability UI
  layer;
- it is report-driven and read-only;
- it is generated from CLI/report models rather than from a daemon or server
  API;
- it renders at least project summary, import freshness, workload, overloads,
  warnings, dependencies, proposals, actual evidence summary and OpenProject
  links;
- first artifacts live under the RePlan project report directory.

Decision:

```text
Start with generated static observability dashboard.
Do not build a full RePlan web application for this slice.
```

Accepted design:

- `static-observability-dashboard.md`.

Implemented target:

- `replan report dashboard` generates local static artifacts from
  `examples/replan-self` first.

Next target:

- inspect whether the generated dashboard makes OpenProject/RePlan state easier
  to understand before adding focused pages or remote publication.

### Step 7. Keep OpenProject write-back narrow and reviewed

Goal:

Prevent accidental mutation of the PM system by humans or AI agents while still
allowing small reviewed corrections when RePlan has resource-planning evidence.

Expected result:

- OpenProject is not mutated by generic imports, reports or scenarios;
- broad automatic bidirectional synchronization remains out of scope;
- approved helpers may write one narrow field family at a time;
- every helper defaults to dry-run;
- apply requires an explicit gate such as `REPLAN_APPLY=1`;
- every helper checks current OpenProject drift before writing;
- every helper uses private bindings and OpenProject `lockVersion`;
- every helper reports skipped, blocked and applied actions in structured
  output;
- after apply, the operator reruns the drift report.

Approved first write-back families:

- status drift push for reviewed workflow-safe status updates;
- date drift push for reviewed RePlan `planned_start` and `planned_finish`
  corrections into OpenProject `startDate` and `dueDate`.

The date write-back design and backlog live in
[`../roadmap/openproject-replan-sync-plan.md`](../roadmap/openproject-replan-sync-plan.md).

### Step 8. Build OpenProject preflight infrastructure before full dogfood

Status: approved preflight block.

Goal:

Reduce OpenProject operating friction before the full OpenProject-centered
dogfood cycle starts.

The problem is practical. OpenProject is the human task source, but AI CLI
agents should not operate it through ad hoc browser actions, unreviewed `curl`
commands or delete-and-recreate shortcuts. OpenProject API updates involve
current state, permissions, schemas, HATEOAS links, relations, date behavior and
`lockVersion`. That is enough surface area to create drift or duplicate work
packages unless the agent path is explicit.

Approved preflight sequence:

1. Reconstruct OpenProject functionality and technical pitfalls already found
   in the pilot and self-dogfood work.
2. Evaluate OpenProject MCP, the official OpenProject CLI, direct API usage and
   RePlan-owned wrapper commands for AI CLI agent workflows.
3. Audit the official OpenProject CLI, the live MCP endpoint and direct API
   patterns hands on before the operating-layer design.
4. Design the narrow OpenProject AI CLI operating layer.
5. Implement the minimum guarded preflight helpers.
6. Design and implement the OpenProject work-package operator toolkit.
7. Refactor the live `replan-self` OpenProject plan through those helpers.

Required properties of the operating layer:

- read operations are easy for humans and AI CLI agents;
- write operations are narrow and reviewed;
- dry-run is the default;
- apply requires an explicit gate;
- OpenProject `lockVersion` is read immediately before mutation;
- private live bindings stay in ignored files;
- destructive operations require a separate explicit gate and are not part of
  normal sync or refactor flow;
- every mutation produces structured output and audit evidence;
- drift checks run before or after relevant live changes;
- RePlan does not become a generic OpenProject replacement.

Current interface decision:

- use the official OpenProject MCP server as a potential read-only agent
  surface only after a separate live configuration check proves it usable;
- treat the official OpenProject CLI as a real auxiliary inspection tool and
  implementation reference, not as the RePlan automation contract;
- keep RePlan-owned guarded helpers as the write path for dogfood-critical
  status, date, relation and binding operations.

The accepted command and safety contract for those helpers is documented in
[`../dev/openproject-ai-cli-operating-layer-design.md`](../dev/openproject-ai-cli-operating-layer-design.md).

## Preflight Operator Report

The first implemented helper for the OpenProject AI CLI operating layer is:

```sh
./scripts/openproject-operator-report.sh
```

It is read-only. It is intended to be the first command an AI CLI agent runs
before proposing or applying any OpenProject mutation.

For the live dogfood OpenProject project, use the private binding layer and the
normal OpenProject environment variables:

```sh
OPENPROJECT_TOKEN=... \
REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal \
REPLAN_OPENPROJECT_PROJECT_ID=replan-self \
./scripts/openproject-operator-report.sh
```

For committed smoke tests and public examples, use the sanitized fixture and
sanitized example bindings:

```sh
REPLAN_PROJECT=examples/replan-self \
REPLAN_OPENPROJECT_FROM_FILE=examples/imports/openproject/replan-self.work-packages.sample.json \
REPLAN_OPENPROJECT_BINDINGS_PATH=examples/openproject/private-bindings.replan-self.example.yaml \
./scripts/openproject-operator-report.sh
```

Refresh that sample's linked subjects and planned dates after applying
`accept-task.sh` or `replan task set-title`, and commit it with the
acceptance or rename:

```sh
REPLAN_UPDATE_GOLDEN=1 go test ./internal/app -run TestSelfOpenProjectSampleAgreesWithCanonicalTasks
```

The Go CLI can read the same file explicitly as a read-only task layer:

```sh
replan report dashboard --project examples/replan-self \
  --bindings examples/openproject/private-bindings.replan-self.example.yaml \
  --output "$PWD/tmp/openproject-live-overlay/latest" --format json
```

`--bindings` overrides `REPLAN_OPENPROJECT_BINDINGS_LAYER`; the CLI does not
read the helpers' `REPLAN_OPENPROJECT_BINDINGS_PATH`. The layer replaces task
OpenProject refs in memory and never rewrites `canonical/tasks.yaml`.
Dependency and resource entries in the bindings document are not layered.

The helper returns a stable JSON envelope with:

- OpenProject source mode;
- private binding load result;
- target project identity;
- mapped local tasks;
- status and date drift;
- relation direction rows when relation candidates are available;
- relation binding enforcement state;
- RePlan resource and active demand counts;
- helper contract metadata for dry-run and apply behavior;
- suggested operator decision and next actions.

Relation direction rows prefer candidate `predecessor_source_id` and
`successor_source_id`; they fall back to deprecated `from_work_package_id`
and `to_work_package_id` for old OpenProject snapshots. Resource-demand
candidates similarly use `source_object_id`, with deprecated
`work_package_id` retained in OpenProject output for existing readers.

`operator_report_status` has three meanings:

| Status | Meaning | Normal next step |
| --- | --- | --- |
| `aligned` | Task identity, task fields and relation skeleton currently match. | Continue with the next planning or execution step. |
| `soft_drift` | Task identity is usable, but statuses or dates need an explicit accept or push decision. | Review the drift and choose one narrow accept or push helper. |
| `drift` | Identity, binding, title or enforced relation drift exists. | Do not apply OpenProject write helpers until the drift is reviewed. |

Relation drift blocks apply only when private relation bindings exist for the
project. Without relation bindings, relation rows are diagnostic and
`scripts/apply-openproject-relations.sh` still has to run its own dry-run before
any apply.

The intended guarded sequence is:

```text
operator report -> strict drift check -> dry-run one write helper -> reviewed
apply -> strict drift check -> RePlan actuals/evidence/audit
```

The operator report is not the full OpenProject manipulation layer. It is the
preflight visibility layer. The full work-package manipulation contract is
documented in
[`../dev/openproject-work-package-operator-toolkit-design.md`](../dev/openproject-work-package-operator-toolkit-design.md).

The current transitional implementation is:

```sh
./scripts/openproject-work-package-toolkit.sh
```

It provides:

- `work-package list`;
- `work-package inspect`;
- `work-package update-status`;
- `work-package update-dates`;
- `work-package update-metadata`;
- `work-package update-text`;
- `status list`;
- `type list`;
- `user list`;
- `priority list`;
- `relation list`;
- `comment add`.

Writes are dry-run by default. Live mutation requires `REPLAN_APPLY=1`.
Fixture mode never applies writes. The script is the approved path for
day-to-day OpenProject work-package manipulation until the same contract is
moved into the first-class `replan openproject ...` CLI surface.
`work-package update-status --status` accepts the seven RePlan tracking
statuses or their exact mapped OpenProject names.
`work-package update-text` previews old/new subject and raw Markdown
description with an optimistic `lockVersion` apply command. A bound subject is
kept in step with the canonical RePlan title through audited `task set-title`;
the OpenProject description is not copied into RePlan's separate planning
note.

## Non-goals for the next slice

Do not start with:

- private pilot cleanup beyond current HEAD sanitization;
- RePlan web application;
- RePlan daemon;
- custom authentication;
- full role-based access control;
- broad or automatic OpenProject write-back;
- automatic scheduling optimizer;
- full TaskJuggler dependency;
- enterprise reporting;
- replacing OpenProject task views with RePlan.

## Done criteria for this plan

The near-term operator-loop work is done when:

1. `examples/replan-self` exists and validates;
2. RePlan can produce a useful workload/report artifact from self-dogfood data;
3. missing resource-demand data is visible as a concrete sidecar/custom-field
   gap, not a vague product concern;
4. AI CLI agents can run the documented commands without direct OpenProject
   mutation;
5. the operator can decide the next project action from OpenProject views or
   fixtures plus RePlan outputs;
6. private pilot details remain outside the public RePlan repository.
