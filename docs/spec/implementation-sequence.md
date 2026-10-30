# RePlan implementation sequence

Date: 2026-07-06

Status: approved implementation specification.

## Purpose

This document defines the first practical implementation order for RePlan.

It answers:

- what to build first;
- what not to build first;
- which artifacts each step should produce;
- how each step is verified;
- when a step is considered done;
- how to keep the MVP local-first, headless-first and AI-CLI-first;
- how to avoid drifting into a full project-management suite, web application
  or enterprise platform too early.

This document depends on:

- `docs/spec/canonical-model.md`;
- `docs/spec/storage-layout.md`;
- `docs/spec/cli.md`;
- `docs/spec/export-artifacts.md`;
- `docs/spec/taskjuggler-capacity-quota-mapping.md`;
- `docs/spec/taskjuggler-run-import-experiment.md`;
- `docs/architecture/deployment-architecture.md`;
- `docs/architecture/application-runtime.md`;
- `docs/architecture/implementation-stack.md`.

## Core decision

Build the smallest useful local CLI/reporting loop first.

Approved rule:

```text
fixtures -> validate -> workload -> report -> scenario -> proposal/audit -> import/export
```

Do not start with:

- web user interface;
- daemon;
- server API;
- custom authentication;
- full role-based access control;
- full solver;
- OpenProject write-back;
- mandatory TaskJuggler runtime;
- plugin system;
- hosted service.

## Target first useful loop

The first useful loop should be:

```bash
replan validate examples/small-team --format json
replan workload --project examples/small-team --week 2026-W28 --format json
replan report weekly --project examples/small-team --week 2026-W28 --format markdown
replan scenario create --project examples/small-team --name ivan-unavailable-week-28 --from current --format json
replan scenario diff --project examples/small-team scenario.ivan-unavailable-week-28 --against current --format json
```

This loop proves:

- canonical model can be represented in files;
- storage layout is workable;
- CLI contract is usable by humans and AI agents;
- workload calculation produces planning value;
- generated reports can support human validation;
- scenarios can be represented without mutating current plan.

## Implementation principles

### Each step must be runnable

Every implementation step should end with a command that runs locally.

If a step cannot be verified by command, test or generated artifact, it is too
abstract.

### Keep command handlers thin

Command handlers should:

- parse arguments;
- create application service request;
- call application service;
- render output;
- return exit code.

They should not contain domain rules, calculations, adapter mapping or storage
business logic.

### Use fixtures before source integrations

OpenProject import is important, but it should not be the first thing the core
depends on.

Fixtures make early validation and reports deterministic.

### Use files before SQLite

File storage is the first canonical storage mode.

SQLite can be added later as cache, index or storage adapter.

The first code should still have a storage boundary so SQLite can be introduced
without rewriting application services.

### Prefer proposal and dry-run before direct mutation

Mutating commands should be added only after read-only validation, workload and
reporting work.

Risky operations should support dry-run before real apply.

### Keep TaskJuggler optional

TaskJuggler is useful as parity benchmark and advanced scheduling engine.

It must not be required for the first useful RePlan workflow.

## Phase 0: Repository and Go module skeleton

Goal:

Create a compilable Go project with the intended package boundaries.

Artifacts:

```text
go.mod
cmd/replan/main.go
internal/domain/
internal/app/
internal/storage/filestore/
internal/reports/
internal/policy/
internal/audit/
examples/small-team/
```

Initial package responsibilities:

```text
cmd/replan
  CLI parsing and output rendering

internal/domain
  canonical model types and domain validation helpers

internal/app
  application services and operation request/result types

internal/storage/filestore
  local file loading and later writing

internal/reports
  report models and renderers

internal/policy
  actor and mutation policy checks

internal/audit
  audit record creation and JSON Lines writer
```

Verification:

```bash
go test ./...
go run ./cmd/replan --help
```

Done when:

- module compiles;
- package boundaries exist;
- `replan --help` runs;
- no domain logic is embedded in CLI handler code.

Do not implement:

- real calculation;
- OpenProject adapter;
- server;
- daemon;
- SQLite.

## Phase 1: Minimal canonical structs

Goal:

Represent the approved canonical model in Go enough to parse fixtures and
validate references.

Artifacts:

```text
internal/domain/project.go
internal/domain/ref.go
internal/domain/actor.go
internal/domain/resource.go
internal/domain/task.go
internal/domain/demand.go
internal/domain/capacity.go
internal/domain/availability.go
internal/domain/plan.go
internal/domain/scenario.go
internal/domain/evidence.go
internal/domain/proposal.go
internal/domain/audit.go
```

Minimum entities:

- `Project`;
- `Actor`;
- `Resource`;
- `ResourcePool`;
- `Task`;
- `ResourceDemand`;
- `Capacity`;
- `Availability`;
- `Actual`;
- `Baseline`;
- `CurrentPlan`;
- `Scenario`;
- `Evidence`;
- `Proposal`;
- `Warning`;
- `AuditRecord`;
- `SourceSnapshot`;
- `ReportSnapshot`;
- common `Ref`, `SourceRef`, `Amount`, `Period`.

Verification:

```bash
go test ./internal/domain/...
```

Done when:

- structs compile;
- canonical enum values are defined;
- basic validation catches invalid statuses and missing identifiers;
- no source-specific fields are hard-coded into core structs.

Do not implement:

- full JSON schema generation;
- database persistence;
- advanced unit conversion;
- full scheduling solver.

## Phase 2: Example project fixtures

Goal:

Create a real `examples/small-team/` project matching `storage-layout.md`.

Artifacts:

```text
examples/small-team/
  .replan/project.yaml
  .replan/policy.yaml
  canonical/actors.yaml
  canonical/resources.yaml
  canonical/resource_pools.yaml
  canonical/tasks.yaml
  canonical/resource_demands.yaml
  canonical/capacities.yaml
  canonical/availability.yaml
  canonical/current_plan.yaml
  baselines/baseline.2026-07-06.initial.yaml
  actuals/actuals.2026-W28.yaml
  evidence/evidence.2026-W28.yaml
  proposals/open/proposal.adjust-remaining-task-op-1234.20260706.yaml
  scenarios/scenario.ivan-unavailable-week-28.yaml
  audit/audit.2026-07.jsonl
```

Required fixture contents:

- operator human actor;
- two human developer resources;
- one AI agent actor and resource;
- one service quota resource;
- one environment resource;
- one offline task;
- one software task;
- one baseline;
- one scenario;
- one proposal;
- one actual record;
- one evidence record.

Verification:

```bash
find examples/small-team -maxdepth 3 -type f | sort
```

Done when:

- fixture directory follows storage layout;
- fixture ids are stable;
- fixture data exercises humans, AI agent, quota, environment and offline work;
- fixture is small enough to inspect manually.

Do not implement:

- full real project data;
- private source data;
- credentials;
- generated report snapshots unless needed later.

## Phase 3: File storage reader

Goal:

Load local file-based canonical state into application model.

Artifacts:

```text
internal/storage/filestore/load.go
internal/storage/filestore/project.go
internal/storage/filestore/yaml.go
internal/storage/filestore/audit.go
```

Capabilities:

- locate project by `--project` path;
- load `.replan/project.yaml`;
- load `.replan/policy.yaml`;
- load `canonical/*.yaml`;
- load baselines, actuals, evidence, proposals, scenarios and audit;
- parse YAML;
- parse audit JSON Lines;
- return structured storage errors.

Verification:

```bash
go test ./internal/storage/filestore/...
```

Done when:

- example project loads;
- missing required file creates structured error;
- malformed YAML creates structured error;
- audit JSON Lines parse line by line;
- loader does not perform workload calculation.

Do not implement:

- file writing except maybe test helpers;
- SQLite;
- source import;
- generated report writing.

## Phase 4: `replan validate`

Goal:

Implement the first real CLI command.

Command:

```bash
replan validate examples/small-team --format json
```

Required behavior:

- resolve project path;
- load file storage;
- validate required files;
- validate identifiers;
- validate references;
- validate statuses;
- validate required units;
- validate basic actor/resource separation;
- emit structured warnings;
- emit structured errors;
- return stable exit code.

JSON output:

```json
{
  "ok": true,
  "command": "validate",
  "project_path": "examples/small-team",
  "result": {
    "valid": true,
    "checked_files": 12
  },
  "warnings": [],
  "errors": []
}
```

Verification:

```bash
go test ./...
go run ./cmd/replan validate examples/small-team --format json
```

Done when:

- valid fixture exits `0`;
- intentionally broken fixture exits `2`;
- JSON output follows CLI envelope;
- validation warnings are machine-readable;
- command handler remains thin.

Do not implement:

- workload calculation;
- report generation;
- mutation commands.

## Phase 5: Workload calculation

Goal:

Calculate useful workload over fixture data without OpenProject and without
TaskJuggler.

Command:

```bash
replan workload --project examples/small-team --week 2026-W28 --format json
```

Capabilities:

- calculate capacity by resource and unit;
- calculate planned, reserved, actual and remaining amounts;
- apply selected scenario overlays with `--scenario <id>`;
- detect overload;
- detect missing capacity;
- detect unknown remaining work;
- support human hours;
- support AI agent hours;
- support service quota;
- support environment concurrency at a simple level;
- output structured warnings.

Verification:

```bash
go test ./internal/app/...
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --scenario scenario.task-op-1234-reassign-petr --format json
```

Done when:

- report can answer who or what is overloaded;
- non-hour resources appear in output;
- unknowns do not silently become zero or infinity;
- selected scenario overlays can be calculated without mutating canonical state;
- calculation does not require TaskJuggler.

Do not implement:

- advanced solver;
- automatic leveling;
- critical path;
- earned value;
- payroll or cost accounting.

## Phase 6: Weekly report rendering

Goal:

Generate human-readable and machine-readable reports from the same report model.

Commands:

```bash
replan report weekly --project examples/small-team --week 2026-W28 --format markdown
replan report weekly --project examples/small-team --week 2026-W28 --format json
replan report weekly --project examples/small-team --week 2026-W28 --format csv
```

Artifacts:

```text
internal/reports/model.go
internal/reports/json.go
internal/reports/markdown.go
internal/reports/csv.go
```

Capabilities:

- workload summary;
- overload summary;
- missing data warnings;
- planned/current/actual/forecast comparison where data exists;
- proposal summary;
- evidence summary;
- status date;
- baseline reference.

Verification:

```bash
go test ./internal/reports/...
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format markdown
```

Done when:

- Markdown report is readable in a terminal and Obsidian;
- JSON report is parseable by scripts and AI agents;
- CSV output opens in spreadsheet tools;
- report uses report model, not ad hoc CLI text.

Do not implement:

- full web UI;
- interactive dashboard;
- complex HTML frontend.

## Phase 7: Scenario read/create/diff

Goal:

Support what-if planning overlays without mutating current plan.

Commands:

```bash
replan scenario list --project examples/small-team --format json
replan scenario show --project examples/small-team scenario.ivan-unavailable-week-28 --format json
replan scenario create --project examples/small-team --name ivan-unavailable-week-28 --from current --format json
replan scenario diff --project examples/small-team scenario.ivan-unavailable-week-28 --against current --format markdown
```

Capabilities:

- list scenarios;
- show scenario definition;
- create scenario file;
- calculate scenario workload;
- compare scenario to current plan;
- show changed demands, capacity, availability and warnings.

Verification:

```bash
go test ./internal/app/...
go run ./cmd/replan scenario diff --project examples/small-team scenario.ivan-unavailable-week-28 --against current --format json
```

Done when:

- scenario does not copy whole project;
- scenario does not mutate current plan during diff;
- scenario output is understandable to human and AI agent;
- scenario creation writes audit.

Do not implement:

- scenario apply without dry-run;
- full patch language;
- automatic optimizer.

## Phase 8: Proposal and audit read path

Goal:

Make proposal and audit records visible before adding risky mutations.

Commands:

```bash
replan proposal list --project examples/small-team --status open --format json
replan proposal show --project examples/small-team proposal.adjust-remaining-task-op-1234.20260706 --format markdown
replan audit list --project examples/small-team --since 2026-07-01 --format json
replan audit verify --project examples/small-team --format json
```

Capabilities:

- list proposals by status;
- show proposal rationale and evidence;
- list audit records;
- verify audit JSON Lines;
- report malformed audit records.

Verification:

```bash
go test ./internal/app/...
go run ./cmd/replan proposal list --project examples/small-team --status open --format json
go run ./cmd/replan audit verify --project examples/small-team --format json
```

Done when:

- proposals are inspectable;
- audit is inspectable;
- AI agents can decide what to propose next using structured data.

Do not implement:

- proposal accept/reject in this read-only phase;
- write-back;
- enterprise audit storage.

## Phase 9: Actuals read path and controlled local mutation

Goal:

Make accepted execution facts inspectable first, then add the first controlled
accepted-fact mutation commands.

Commands:

```bash
replan actual list --project examples/small-team --format json
replan actual list --project examples/small-team --resource resource.human.petr --format markdown
replan actual add-consumption --project examples/small-team --task task.op.1234 --demand demand.task-op-1234.implementer --resource resource.human.ivan --amount 3 --unit hours --evidence evidence.manual.time-entry.20260706.1 --dry-run --format json
replan actual set-remaining --project examples/small-team --demand demand.task-op-1234.implementer --amount 5 --unit hours --reason "Operator review" --dry-run --format json
```

Capabilities:

- read-only actual listing;
- actual filtering by type, task, demand and resource;
- dry-run actual creation;
- actual write with actor;
- audit record creation;
- safe file write;
- validation after write.

Verification:

```bash
go test ./...
go run ./cmd/replan actual list --project examples/small-team --format json
go run ./cmd/replan actual add-consumption --project examples/small-team --task task.op.1234 --demand demand.task-op-1234.implementer --resource resource.human.ivan --amount 3 --unit hours --dry-run --format json
```

Done when:

- existing actuals are visible through structured CLI output;
- AI agents can inspect accepted facts without reading YAML directly;
- dry-run shows exact file changes;
- apply writes actual record;
- apply writes audit record;
- validation passes after apply;
- no baseline is changed.

Do not implement:

- automatic progress inference from Git;
- payroll/time-sheet product;
- surveillance features.

Implementation order inside this phase:

1. implement `actual list`;
2. implement `actual add-consumption --dry-run`;
3. implement `actual set-remaining --dry-run`;
4. implement actual writes with explicit `--actor`;
5. write audit records for applied actual changes;
6. validate project state after applied changes.

## Phase 10: Proposal accept/reject

Goal:

Accept or reject proposed changes through explicit audited operations.

Commands:

```bash
replan proposal accept --project examples/small-team proposal.adjust-remaining-task-op-1234.20260706 --dry-run --format json
replan proposal accept --project examples/small-team proposal.adjust-remaining-task-op-1234.20260706 --actor actor.human.operator --apply --format json
replan proposal reject --project examples/small-team proposal.adjust-remaining-task-op-1234.20260706 --reason "Evidence is not sufficient" --apply --format json
```

Capabilities:

- dry-run proposal acceptance;
- dry-run proposal rejection;
- move accepted proposal from `proposals/open/` to `proposals/accepted/`;
- move rejected proposal from `proposals/open/` to `proposals/rejected/`;
- update proposal status during the move;
- write audit;
- reject with reason;
- validate after mutation.

Implemented first decision-gate slice:

- `proposal accept --dry-run` previews status move and audit append;
- `proposal accept --actor <id>` moves open proposal to accepted and writes
  audit;
- `proposal reject --dry-run --reason <text>` previews rejection;
- `proposal reject --actor <id> --reason <text>` moves open proposal to
  rejected and writes audit;
- mutating accept/reject requires an existing actor;
- rejected decisions require a reason;
- canonical task/resource/current-plan state is not changed by this slice.

Verification:

```bash
go test ./...
go run ./cmd/replan proposal accept --project examples/small-team proposal.adjust-remaining-task-op-1234.20260706 --dry-run --format json
```

Done when:

- AI-created proposals can be reviewed;
- acceptance is explicit;
- rejection is explicit;
- audit records decision and actor.

Do not implement:

- automatic acceptance by untrusted AI actors;
- canonical current-plan mutation;
- scenario apply;
- source write-back.

## Phase 11: Read-only OpenProject import

Goal:

Import OpenProject data into `imports/` and map it into canonical candidates
without silently mutating current plan.

First implementation slice:

```bash
replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
```

This first slice is file-based and read-only. It uses a saved OpenProject
work-package collection fixture before any network API, token handling or
snapshot persistence is added.

Initial capabilities:

- map work packages to task candidates;
- map assignees to resource candidates where possible;
- map OpenProject estimates and spent time into hour fields for inspection;
- match existing RePlan tasks by OpenProject source reference or exact title;
- expose start and due dates on task candidates;
- emit warnings for missing or ambiguous fields.

Implemented in the source-profile slice:

- define an OpenProject source profile for non-secret connection settings;

Implemented in the API-backed read-only slice:

- call OpenProject API with Basic Auth `apikey:<token>` using `token_env`;
- read project work packages through paginated collection requests;
- support single work package reads by id;
- pass API-loaded collections through the same mapper as `--from-file`;
- return structured diagnostics for missing token, authentication, permission,
  not found, rate limit, network and malformed response failures;
- keep canonical state unchanged.

Implemented in the import snapshot persistence slice:

- write `imports/openproject/<snapshot-key>/manifest.yaml`;
- write `imports/openproject/<snapshot-key>/raw.json`;
- write `imports/openproject/<snapshot-key>/mapped.json`;
- keep canonical state unchanged when writing import snapshots;
- expose planned writes in `--dry-run` output;
- expose written files in write-mode output.

Implemented in the import accept dry-run slice:

- read `imports/openproject/<snapshot-key>/mapped.json` by source snapshot id;
- preview canonical task creation for unmatched OpenProject work packages;
- preview task `source_refs` updates for matched tasks;
- preview task `tracking_status` updates when mapped status differs;
- preview canonical files that would be touched;
- require `--dry-run` for accept because canonical mutation is not implemented
  yet.

Implemented in the import accept write-mode slice:

- require explicit `--apply`;
- require `--actor`;
- reject conflicting `--dry-run --apply`;
- create unmatched tasks from OpenProject work packages;
- add missing OpenProject source references to matched tasks;
- append local audit record;
- run validation after write;
- keep task `tracking_status` changes in preview only;
- do not create resources, resource demands, deadlines or dependencies.

Implemented in explicit OpenProject accept expansion slices:

- `replan import accept --accept-resource-links` appends safe OpenProject user
  source refs to existing canonical resources;
- `replan import accept --accept-resource-demands` appends safe sidecar-mapped
  canonical resource demands;
- `replan import accept --accept-time-entries` appends evidence and actual
  consumption records for safe OpenProject time-entry candidates;
- `replan import accept --accept-tracking-status` updates safe mapped
  OpenProject statuses on existing matched canonical tasks;
- each expansion mode is mutually exclusive with the other expansion modes;
- each write mode requires `--apply --actor <id>`;
- each write mode appends audit and runs validation after writing;
- ordinary task accept remains separate from resource-link, resource-demand and
  time-entry acceptance.

Implemented in the OpenProject time-entry file-fixture slice:

- `replan import openproject --include-time-entries` reads OpenProject time
  entries from the API in API mode;
- file-mode `--include-time-entries` reads public time-entry fixtures from
  `_embedded.time_entries.elements`;
- file-mode fixtures without `_embedded.time_entries.elements` are valid and
  produce zero time-entry candidates without warning;
- `import accept --accept-time-entries --dry-run` can preview evidence and
  actual creation from the public self-dogfood time-entry fixture.

Implemented in the OpenProject dependency/relation mapper-only slice:

- `replan import openproject --include-relations` fetches OpenProject relation
  records in API mode;
- supported relation types are mapped into read-only `dependency_candidates`;
- supported relation types are `precedes`, `follows`, `blocks`, `blocked` and
  symmetric, non-scheduling `relates`;
- file-mode `--include-relations` reads public relation fixtures from
  `_embedded.relations.elements`;
- canonical dependency writes are implemented through explicit
  `import accept --accept-dependencies` after dry-run review.

Implemented and retained from the OpenProject relation-validation slice:

- validated representative OpenProject relations in a private test project;
- verified that raw live import now returns relation-backed
  `dependency_candidates`;
- verified that candidates are `task_unmatched` before ordinary task accept
  because the local example project has no accepted live task source refs;
- verified on a temporary RePlan project copy that ordinary task accept makes
  in-scope OpenProject relations `safe_to_accept`;
- verified that partially out-of-scope relations remain visible as
  `task_unmatched`;
- retained the reusable conclusions in
  `docs/dev/openproject-pilot-lessons.md` and `docs/spec/mvp-readiness.md`.

Implemented in the OpenProject sidecar operator-helper slice:

- `scripts/run-openproject-sidecar-pipeline.sh` runs the full live sidecar
  acceptance sequence against a temporary project copy;
- the helper requires explicit resource-link and resource-demand sidecars for
  mutating accepts;
- the helper skips cleanly when the live token environment variable is not set;
- pilot sidecar examples live under `examples/openproject/sidecars/`;
- the helper is intentionally not part of CI because it depends on the private
  live OpenProject instance and secret token.

Later capabilities:

- accept dependency candidates after canonical dependency persistence and
  duplicate detection are designed;
- optionally expand `import accept` write mode after explicit design.

Dependency implementation status after the OpenProject relation audit:

1. Completed dependency persistence foundation:
   - added canonical dependency domain model;
   - load optional `canonical/dependencies.yaml`;
   - expose dependencies in JSON export;
   - validate dependency types, statuses, criticality, references and
     duplicates;
   - added small-team fixture dependencies;
   - keep dependency writes and import accept out of this foundation slice.
2. Completed dependency accept implementation:
   - implement `replan import accept --accept-dependencies --dry-run`;
   - implement `--apply --actor <id>` append into
     `canonical/dependencies.yaml`;
   - append audit and validate after write;
   - keep it mutually exclusive with ordinary task accept and other
     specialized accept modes.
3. Completed live dependency accept pilot:
   - run live OpenProject import with `--include-relations` against a
     temporary RePlan project copy;
   - ordinary task accept must run before dependency accept;
   - re-import with `--include-relations` after tasks are accepted;
   - dry-run and apply `--accept-dependencies`;
   - record reusable conclusions in
     `docs/dev/openproject-pilot-lessons.md`.
4. Completed dependency pipeline helper decision:
   - do not add dependency apply to
     `scripts/run-openproject-sidecar-pipeline.sh` yet;
   - keep dependency accept manual until relation quality is validated in more
     than one pilot run;
   - keep optional dependency dry-run validation in
     `scripts/validate-openproject.sh`.
5. Implemented dependency dry-run validation helper:
   - `scripts/validate-openproject.sh` supports
     `REPLAN_OPENPROJECT_DEPENDENCY_DRY_RUN=1`;
   - requires live token and `REPLAN_OPENPROJECT_INCLUDE_RELATIONS=1`;
   - uses only a temporary project copy;
   - runs ordinary task accept in the temporary copy, re-imports relations and
     runs `import accept --accept-dependencies --dry-run`;
   - does not apply dependency acceptance.
6. Implemented dependency planning warnings:
   - `replan validate` emits MVP dependency planning warnings;
   - `replan report weekly` surfaces the same dependency warnings;
   - warnings cover unresolved required dependencies, missing
     `finish_to_start` dates, successor-before-predecessor date conflicts and
     active blockers on ready/in-progress/review-ready successors;
   - warnings do not mutate dates, statuses, baselines or current plan.
7. Implemented dependency persistence and accept design rules:
   - define `canonical/dependencies.yaml` MVP schema;
   - align the schema with `docs/spec/dependency-deadline-behavior.md` and
     `docs/spec/canonical-model.md`;
   - define duplicate detection by source relation id and canonical
     predecessor/successor/type triple;
   - define validation rules for missing task references, unsupported types and
     simple cycles;
   - define `replan import accept --accept-dependencies` dry-run/apply
     behavior before implementing it.
8. Implemented dependency proposal lifecycle:
   - `replan proposal suggest-dependencies` generates dry-run proposal
     candidates from canonical dependency state;
   - `--write-candidates --actor <id>` materializes reviewable
     `proposals/open/*.yaml` candidates;
   - implemented proposal types include `mark_successor_blocked`,
     `mark_dependency_resolved`, `mark_successor_ready`,
     `review_dependency_resolution`, `review_dependency_schedule` and
     `adjust_successor_start`;
   - `proposal accept` applies `mark_successor_blocked`,
     `mark_dependency_resolved` and `mark_successor_ready`;
   - `adjust_successor_start` remains accepted-only until date movement policy
     is designed with current plan, baselines, scenarios and TaskJuggler.
9. Implemented dependency lifecycle overview:
   - `replan dependency list` reports dependency status, blocking state,
     resolvable dependencies, blocked successors ready to unblock and linked
     open proposal ids;
   - the command supports JSON and Markdown;
   - it is read-only and included in MVP smoke.

Verification:

```bash
go test ./...
go run ./cmd/replan import openproject --project examples/small-team --from-file examples/imports/openproject/work-packages.sample.json --dry-run --format json
```

Done when:

- import can run read-only;
- file-based raw snapshot can be parsed;
- mapped candidates are inspectable;
- current canonical state is not mutated without explicit accept.

Do not implement:

- OpenProject write-back;
- complex custom field synchronization;
- background sync daemon.

## Phase 12: Export JSON and TaskJuggler proof of concept

Goal:

Export RePlan canonical state or scenario to external formats.

First implementation slice:

```bash
replan export json --project examples/small-team --output exports/snapshots/current.json --manifest exports/snapshots/current.manifest.json
replan export json --project examples/small-team --format json
```

First-slice capabilities:

- export loaded canonical workspace as JSON;
- include project, actors, resources, resource pools, tasks, resource demands,
  capacities, availability, current plan, baselines, actuals, evidence,
  proposals, scenarios and audit records;
- include export metadata with export id, exported_at, source project path and
  format;
- write to stdout when `--output` is omitted;
- write to file when `--output` is provided;
- keep command read-only.

Implemented in the export JSON slice:

- `replan export json`;
- stdout output when `--output` is omitted;
- JSON file output when `--output` is provided;
- export metadata with id, exported timestamp, source project path and format;
- read-only workspace snapshot export with no network or TaskJuggler runtime.

Implemented in the TaskJuggler proof-of-concept slice:

- `replan export taskjuggler`;
- text output with generated `.tjp` content when `--output` is omitted;
- file output when `--output` is provided;
- JSON output with metadata, output path, warnings and errors when
  `--format json` is used;
- resources exported as TaskJuggler `resource` blocks;
- tasks exported as TaskJuggler `task` blocks;
- concrete hour-based resource demands exported as `effort` and `allocate`;
- unsupported demand units and non-concrete allocation shapes reported as
  warnings;
- read-only behavior;
- no TaskJuggler process invocation;
- no TaskJuggler result parsing;
- no TaskJuggler runtime requirement.

TaskJuggler proof-of-concept commands:

```bash
replan export taskjuggler --project examples/small-team --format text
replan export taskjuggler --project examples/small-team --output exports/taskjuggler/current/project.tjp --manifest exports/taskjuggler/current/project.manifest.json --format json
replan export taskjuggler --project examples/small-team --scenario scenario.ivan-unavailable-week-28 --output exports/taskjuggler/scenario.ivan-unavailable-week-28/project.tjp --manifest exports/taskjuggler/scenario.ivan-unavailable-week-28/project.manifest.json --format json
```

Implemented in the first TaskJuggler scenario overlay slice:

- `--scenario <id>` loads a selected scenario;
- scenario changes are applied to a temporary in-memory workspace only;
- canonical files are not mutated;
- `add availability.exceptions` is supported;
- unavailable resource exceptions are exported as TaskJuggler `vacation`
  statements;
- unsupported scenario operations, unsupported paths, invalid change values and
  unknown resources fail with explicit diagnostics.

Implemented in the remaining-amount scenario overlay slice:

- `set` and `replace` changes for
  `resource_demands.<resource_demand_id>.remaining_amount` and
  `resource_demands.<resource_demand_id>.planned_amount`;
- `set` and `replace` changes for
  `resource_demands.<resource_demand_id>.resource_ref`;
- `set` and `replace` changes for `tasks.<task_id>.planned_start` and
  `tasks.<task_id>.planned_finish`, using the same parsing and combined-window
  validation as `scenario apply`;
- non-negative numeric values;
- in-memory overlay only, with no canonical file mutation;
- explicit diagnostics for invalid values, invalid paths and unknown resource
  demands or resources;
- TaskJuggler `effort` uses `remaining_amount` when present and falls back to
  `planned_amount` only when remaining work is not set.
- TaskJuggler `allocate` uses overlaid `resource_ref` when assignment is
  changed by scenario.
- workload scenario overlays support `set` and `replace` changes for
  `capacities.<capacity_id>.amount`, including service quota capacities;
- capacity amount overlays use non-negative numeric values and fail with
  explicit diagnostics for invalid values, invalid paths and unknown
  capacities;
- capacity and quota amount overlays are visible in `replan workload --scenario
  <id>` and can produce overload;
- task planned-date overlays feed workload week selection and spreading,
  weekly/state/dashboard reports, Gantt participation windows, planning
  warnings, workload CSV, TaskJuggler input and scenario diff before any
  canonical mutation;
- capacity and quota amount overlays are not mapped into TaskJuggler `.tjp` yet.

Later commands and expansions:

```bash
replan export taskjuggler --project examples/small-team --scenario scenario.ivan-unavailable-week-28 --output exports/taskjuggler/scenario.ivan-unavailable-week-28/project.tjp
replan export csv --project examples/small-team --report workload --week 2026-W28 --output exports/workload/2026-W28/current.csv --manifest exports/workload/2026-W28/current.manifest.json
```

Implemented in the workload CSV export slice:

- `replan export csv --report workload`;
- optional `--scenario <id>`;
- stdout output when `--output` is omitted;
- CSV file output when `--output` is provided;
- columns for scenario id, week, resource identity, unit, capacity, planned,
  reserved, actual, remaining and overload;
- read-only behavior.

Implemented in the export manifest slice:

- `--manifest <path>` for file exports;
- supported by `replan export json`, `replan export taskjuggler` and
  `replan export csv --report workload`;
- manifest is written only when `--output` is provided and the export succeeds;
- manifest includes id, command, format, report when relevant, project path,
  output path, scenario id, week, sha256, created_at, warnings and errors;
- `--manifest` without `--output` fails with `manifest_requires_output`;
- checksum is calculated from the exact exported content.

Implemented in the committed export artifact layout documentation slice:

- committed generated artifact layout is defined in
  `docs/spec/export-artifacts.md`;
- snapshots use `exports/snapshots/`;
- workload CSV exports use `exports/workload/<week>/`;
- TaskJuggler exports use `exports/taskjuggler/<scope>/`;
- each committed file export should have a neighboring `.manifest.json`;
- manifests identify the source scenario or current plan, command flags, output
  path and checksum;
- `exports/` remains generated output, not canonical planning state;
- `exports/` is not blanket-ignored, because selected generated artifacts may
  be intentionally committed for review.

Implemented in the TaskJuggler capacity and quota mapping design slice:

- mapping boundary is defined in
  `docs/spec/taskjuggler-capacity-quota-mapping.md`;
- unavailable dates, concrete hour effort and concrete assignments remain the
  safe implemented TaskJuggler mappings;
- `capacities.<capacity_id>.amount` remains authoritative in RePlan workload
  and workload CSV output;
- TaskJuggler export does not encode capacity and quota amount changes in
  `.tjp` yet;
- TaskJuggler export must warn with
  `taskjuggler_capacity_change_not_represented` when a selected scenario
  contains capacity amount changes not represented in `.tjp`;
- tests cover one human capacity scenario and one service quota scenario where
  `.tjp` is still generated but the unrepresented capacity warning is present;
- TaskJuggler export supports `--unrepresented-assumptions warn|error`;
- default mode is `warn`;
- `--unrepresented-assumptions error` promotes unrepresented assumptions to
  errors and prevents successful `.tjp` generation;
- service quota, rate limit, budget, subscription and stock capacities are not
  TaskJuggler-native MVP mappings;
- narrow weekly hour capacity for `human` and `ai_agent` resources is mapped to
  TaskJuggler `limits { weeklymax ... }`;
- scenario changes to mapped weekly hour capacity no longer trigger
  `taskjuggler_capacity_change_not_represented`;
- broader day, month, effective-range, review-hour and agent-hour limits remain
  deferred until tested.
- optional developer TaskJuggler runtime validation is available through
  `replan dev validate-taskjuggler`;
- TaskJuggler runtime validation skips successfully with
  `taskjuggler_binary_not_found` when `tj3` is not installed;
- when a TaskJuggler binary is available, validation writes a temporary `.tjp`
  and runs the binary against it;
- TaskJuggler runtime validation remains a developer helper, not a required MVP
  runtime dependency.
- local validation wrapper `scripts/validate-taskjuggler.sh` runs the same
  dev command and can be configured through environment variables.
- GitHub Actions CI profile installs pinned TaskJuggler `3.8.4` through
  RubyGems and runs `scripts/validate-taskjuggler.sh` against fixtures.
- TaskJuggler run/import experiment design is documented in
  `docs/spec/taskjuggler-run-import-experiment.md`.
- TaskJuggler parseable report contract is documented in
  `docs/spec/taskjuggler-parseable-report-contract.md`.
- `replan taskjuggler run` run-only artifact capture is implemented.
- `replan taskjuggler run` writes generated `project.tjp`, `stdout.txt`,
  `stderr.txt`, `taskjuggler.version.txt` and `run.manifest.json`.
- `replan taskjuggler run` uses a real TaskJuggler binary and fails when the
  requested binary is missing.
- `replan taskjuggler run` does not mutate canonical RePlan state and does not
  import TaskJuggler results.
- GitHub Actions CI runs `replan taskjuggler run` with pinned TaskJuggler
  `3.8.4` and checks that the run artifacts are written.
- `replan taskjuggler run` generates `reports/replan-task-schedule.csv` and
  `taskjuggler.mapping.json`, verifies that the CSV report exists after
  TaskJuggler exits and records report/mapping checksums in the run manifest.
- `replan taskjuggler import-result --dry-run` reads the run manifest, mapping
  and CSV report, then previews task planned start and planned finish candidate
  changes without mutating canonical state.
- `replan taskjuggler import-result --write-candidate --actor <id>` writes
  reviewable scenario and proposal files without mutating canonical
  task/resource/current-plan state.

Later capabilities:

- expand TaskJuggler scenario overlay support beyond availability exceptions
  and resource demand amount or assignment changes;
- expand TaskJuggler `limits` mapping only after explicit validation;
- add human approval flow for TaskJuggler-generated candidates;
- keep TaskJuggler optional.

Next implementation slice after export JSON, TaskJuggler proof of concept and
scenario-aware workload support:

1. Broader scenario overlay support:
   - expand TaskJuggler `limits` mapping only after explicit validation;
   - keep quota, rate limit, budget and subscription capacity in RePlan
     workload/report outputs;
   - do not mutate canonical state;
   - fail with explicit diagnostics when a scenario change cannot be applied or
     safely represented in workload or `.tjp`.
2. TaskJuggler run/import experiment:
   - keep generated results reviewable and do not mutate canonical state
     automatically;
   - add end-to-end smoke for generated date candidates after proposal
     acceptance.

Verification:

```bash
go test ./...
go run ./cmd/replan export json --project examples/small-team --output /tmp/replan-snapshot.json
go run ./cmd/replan export taskjuggler --project examples/small-team --output /tmp/replan-project.tjp --format json
go run ./cmd/replan export taskjuggler --project examples/small-team --scenario scenario.ivan-unavailable-week-28 --output /tmp/replan-scenario.tjp --format json
```

Done when:

- canonical state can be exported to JSON;
- TaskJuggler export exists as optional proof;
- no runtime path requires TaskJuggler.

Do not implement:

- parsing TaskJuggler results as required MVP path;
- making TaskJuggler the canonical model.

## Phase 13: Scenario apply

Goal:

Show exact current-plan changes that would be applied from a scenario.

Command:

```bash
replan scenario apply --project examples/small-team scenario.task-op-1234-remaining-5 --dry-run --format json
replan scenario apply --project examples/small-team scenario.task-op-1234-remaining-5 --actor actor.human.operator --apply --format json
```

Capabilities:

- calculate patch from scenario overlay to canonical planning state;
- show target files and objects;
- show warnings and errors;
- require `--actor` for mutation;
- write audit;
- validate after mutation;
- do not mutate without explicit apply.

Implemented first apply slice:

- supports `resource_demands.<id>.remaining_amount`;
- supports `resource_demands.<id>.planned_amount`;
- writes `canonical/resource_demands.yaml`;
- marks the scenario file status as `applied`;
- appends `scenario_apply` audit record;
- rejects unsupported scenario changes with explicit diagnostics;
- does not mutate accepted actuals, historical evidence, baselines,
  source-system state or unsupported canonical fields.

Implemented task-date apply slice:

- adds canonical `Task.planned_start`;
- adds canonical `Task.planned_finish`;
- validates task dates as `YYYY-MM-DD`;
- supports `tasks.<id>.planned_start`;
- supports `tasks.<id>.planned_finish`;
- writes `canonical/tasks.yaml`;
- keeps TaskJuggler-generated date candidates reviewable until explicit
  `scenario apply`.
- shares task-date parsing and combined start/finish validation with the
  in-memory scenario overlay, including one-sided changes.

Verification:

```bash
go test ./...
go run ./cmd/replan scenario apply --project examples/small-team scenario.task-op-1234-remaining-5 --dry-run --format json
```

Done when:

- dry-run output is clear enough for human review;
- AI agents can inspect changes;
- no accepted actuals or old baselines are changed.

Do not implement:

- direct apply for untrusted AI actors;
- source write-back.

## Phase 14: Optional local generated HTML report

Goal:

Add human-friendly generated view only after Markdown/JSON reports are useful.

Command:

```bash
replan report weekly --project examples/small-team --week 2026-W28 --format html --write
```

Rules:

- HTML is generated from the same report model;
- HTML is not a separate application state;
- HTML is optional;
- HTML must not require server runtime.

Implemented static weekly HTML slice:

- `replan report weekly --format html` renders a self-contained HTML report to
  stdout;
- `replan report weekly --format html --write` writes
  `reports/weekly/<week>.html`;
- `--write` returns a JSON envelope with `output_path`;
- HTML includes summary metrics, workload rows, errors, warnings, open
  proposals and evidence;
- HTML is a generated report artifact, not canonical state;
- no daemon, server runtime, web application framework or persistent UI state is
  introduced.

Verification:

```bash
go test ./...
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format html --write
test -f examples/small-team/reports/weekly/2026-W28.html
```

Done when:

- HTML report generation is covered by renderer tests;
- CLI artifact writing is covered by tests;
- generated HTML can be opened directly from the project directory;
- weekly JSON, Markdown and CSV behavior remains unchanged.

## Phase 15: Optional daemon or server exploration

Goal:

Explore local daemon or server only after CLI workflows prove useful.

Allowed reasons:

- faster repeated report generation;
- background imports;
- local API for generated UI;
- watch mode;
- cache/index maintenance.

Still required:

- CLI continues to work without daemon;
- daemon calls same application services;
- server calls same application services;
- no domain logic moves into UI or daemon handlers.

Do not start here.

## Testing strategy

### Unit tests

Use for:

- domain validation;
- workload formulas;
- report model rendering;
- scenario overlay behavior;
- proposal state transitions;
- storage parser edge cases.

### Fixture tests

Use `examples/small-team` as integration fixture.

Test:

- load fixture;
- validate fixture;
- calculate workload;
- generate reports;
- scenario diff;
- proposal listing.

### CLI golden tests

Use stable JSON outputs for important commands.

Candidate commands:

```bash
replan validate examples/small-team --format json
replan workload --project examples/small-team --week 2026-W28 --format json
replan report weekly --project examples/small-team --week 2026-W28 --format json
```

Golden tests should avoid timestamps unless normalized.

### Mutation tests

Use temporary project copy.

Test:

- dry-run makes no file changes;
- apply writes expected files;
- audit record is appended;
- validation passes after mutation;
- policy denial prevents mutation.

### MVP acceptance smoke

Implemented fast MVP acceptance smoke:

```bash
./scripts/smoke-mvp.sh
```

Document:

```text
docs/spec/mvp-acceptance.md
```

The smoke covers:

- fixture validation;
- workload calculation;
- weekly JSON, Markdown, CSV and static HTML reports;
- static HTML artifact writing on a temporary project copy;
- scenario diff;
- scenario apply dry-run;
- proposal list and show;
- actual list;
- actual add-consumption dry-run;
- actual add-consumption write on a temporary project copy;
- scenario apply write on a temporary project copy;
- validation after mutation;
- audit verification after mutation;
- OpenProject import dry-run from committed sample JSON;
- OpenProject import snapshot write on a temporary project copy;
- import accept dry-run against the written OpenProject snapshot.

CI:

- GitHub Actions `Go` job runs `go test ./...`;
- GitHub Actions `Go` job runs `./scripts/smoke-mvp.sh`;
- TaskJuggler remains covered by its separate CI job because it requires Ruby
  and pinned TaskJuggler installation.

This smoke does not mutate committed fixtures and does not require network
access, daemon, server, web UI or TaskJuggler runtime.

### CLI ergonomics hardening

Implemented positional-id argument hardening:

- commands with a primary object id accept the id before flags;
- commands with a primary object id accept the id after flags;
- both forms are treated as equivalent when the intent is unambiguous.

Covered commands:

- `replan scenario show`;
- `replan scenario diff`;
- `replan scenario apply`;
- `replan proposal show`;
- `replan proposal accept`;
- `replan proposal reject`;
- `replan import accept`.

Examples:

```bash
replan scenario diff scenario.ivan-unavailable-week-28 --project examples/small-team --format json
replan scenario diff --project examples/small-team --format json scenario.ivan-unavailable-week-28
```

Verification:

- unit tests cover the shared positional argument helper;
- `scripts/smoke-mvp.sh` checks both leading and trailing positional id forms
  for representative commands.

### MVP readiness audit

Next post-acceptance step:

```text
docs/spec/mvp-readiness.md
docs/spec/mvp-readiness-audit-plan.md
```

Goal:

Decide whether the current CLI-first loop is ready to be treated as an
internal MVP, and identify the smallest remaining blockers before daemon,
server, desktop or web UI exploration.

Rules:

- audit current implementation against `Definition of MVP done`;
- use real command evidence, not memory of prior design discussions;
- classify each criterion as `done`, `partial`, `blocked`, `deferred` or
  `not_started`;
- separate true MVP blockers from acceptable internal-MVP limitations;
- record the dated audit result in `docs/spec/mvp-readiness.md`;
- use `docs/spec/mvp-readiness-audit-plan.md` when the audit is run by two
  independent operators or AI CLI agents for second opinion;
- do not implement daemon, server, desktop app, hosted service or web UI as
  part of the audit.

Expected evidence:

```bash
go test ./...
./scripts/smoke-mvp.sh
gh run list --repo kogriv/replan --branch main --limit 5
```

Optional TaskJuggler evidence when local tooling is available:

```bash
./scripts/validate-taskjuggler.sh
./scripts/run-taskjuggler.sh
```

Likely audit outputs:

- readiness verdict: `ready`, `almost_ready` or `not_ready`;
- checklist table for every MVP done criterion;
- blocker list;
- known limitations list;
- recommended next stabilization slice.

Planned parallel-audit preparation:

- added self-dogfood task `task.self.plan-mvp-readiness-audit`;
- audit plan defines exact inputs, commands, mutation-copy rules, output
  structure, comparison protocol and final decision vocabulary;
- two audit reports should be compared by reproducible findings, not averaged.

### README quickstart stabilization

Implemented after the first MVP readiness audit:

- replaced obsolete README placeholder commands with implemented CLI commands;
- documented fast local check with `go test ./...` and
  `./scripts/smoke-mvp.sh`;
- documented read-only operator loop:
  - `validate`;
  - `workload`;
  - `report weekly`;
  - `scenario diff`;
  - `proposal list`;
  - `proposal show`;
- documented dry-run mutation previews:
  - `scenario apply --dry-run`;
  - `actual add-consumption --dry-run`;
  - `import openproject --dry-run`;
- documented that mutating commands should be run on a temporary copy unless
  the operator intentionally wants to modify a project directory.

Result:

- closed the first MVP readiness blocker from
  `docs/spec/mvp-readiness.md`;
- MVP readiness remains `almost_ready` until the exit-code contract is aligned
  or explicitly revised.

### Exit-code contract stabilization

Implemented after the README quickstart stabilization:

- aligned missing local object behavior with `docs/spec/cli.md`;
- added `exitNotFound = 4`;
- `scenario_not_found` returns exit code `4`;
- `proposal_not_found` returns exit code `4`;
- missing OpenProject source profile returns exit code `4`;
- missing OpenProject import snapshot now emits `source_snapshot_not_found` and
  returns exit code `4`;
- unsupported report format still returns exit code `8`;
- non-not-found validation errors still return exit code `2`.

Verification:

- CLI-level tests cover missing scenario, missing proposal, missing source
  profile, missing import snapshot, unsupported format and ordinary validation
  error;
- app-level import accept test covers `source_snapshot_not_found`;
- `go test ./...`;
- `./scripts/smoke-mvp.sh`.

Result:

- closes the second blocker from `docs/spec/mvp-readiness.md`;
- MVP readiness can be considered `ready` for internal CLI-first use, with
  remaining limitations tracked as non-blocking hardening work.

### CLI JSON envelope contract tests

Implemented after exit-code contract stabilization:

- added CLI-level JSON envelope contract tests for key MVP commands;
- tests verify required envelope fields:
  - `ok`;
  - `command`;
  - `project_path`;
  - non-null `result`;
  - `warnings` array;
  - `errors` array;
- tests verify command-specific stable result fields for:
  - `validate`;
  - `workload`;
  - `report weekly`;
  - `scenario diff`;
  - `proposal list`;
  - `actual add-consumption --dry-run`;
  - `import openproject --dry-run`;
- existing exit-code tests cover success, validation, not-found and invalid
  usage cases.

Verification:

- `go test ./...`;
- `./scripts/smoke-mvp.sh`.

Result:

- reduces the JSON-envelope limitation from the first MVP readiness audit;
- keeps full snapshot/golden fixtures deferred until command outputs stabilize
  further through real operator use.

### README operator validation pass

Implemented after the CLI JSON envelope contract tests:

- ran the README quickstart as an operator from the repository root;
- verified the fast local check:
  - `go test ./...`;
  - `./scripts/smoke-mvp.sh`;
- verified the read-only loop:
  - `validate`;
  - baseline `workload`;
  - scenario `workload` with `scenario.ivan-capacity-6`;
  - weekly Markdown report;
  - weekly HTML report to standard output;
  - `scenario diff`;
  - `proposal list`;
  - `proposal show`;
- verified safe dry-run mutation previews:
  - `scenario apply --dry-run`;
  - `actual add-consumption --dry-run`;
  - `import openproject --dry-run`;
- verified generated HTML artifact writing only on a temporary project copy;
- fixed README so the core read-only loop no longer writes generated reports
  into the committed fixture;
- added an explicit scenario workload example because the baseline fixture has
  zero overload;
- recorded the operator validation result in
  `docs/spec/mvp-readiness.md`.

Verification:

- README quickstart commands passed locally;
- scenario workload with `scenario.ivan-capacity-6` showed a 2 hour Ivan
  overload;
- HTML `--write` produced `reports/weekly/2026-W28.html` on a temporary copy;
- the committed fixture remained unmodified.

Result:

- closes the README operator validation next slice from
  `docs/spec/mvp-readiness.md`;
- moves documentation usability and internal MVP usability to `done` for the
  fixture quickstart;
- leaves live OpenProject validation and local TaskJuggler runtime availability
  as non-blocking limitations.

### OpenProject operational validation wrapper

Implemented after the README operator validation pass:

- added `scripts/validate-openproject.sh`;
- validates saved OpenProject work-package import through the same mapper used
  by the API path;
- validates source profile loading with `openproject.demo`;
- writes an import snapshot on a temporary project copy;
- verifies `manifest.yaml`, `raw.json` and `mapped.json`;
- runs `import accept --dry-run` against the written snapshot;
- runs live API dry-run only when the configured token environment variable is
  present;
- reports live API validation as skipped when no token is configured, rather
  than pretending that live validation passed.

Default command:

```bash
./scripts/validate-openproject.sh
```

Live API command shape:

```bash
REPLAN_OPENPROJECT_PROFILE=openproject.demo \
REPLAN_OPENPROJECT_TOKEN_ENV=OPENPROJECT_TOKEN \
REPLAN_OPENPROJECT_SINCE=2026-07-01 \
OPENPROJECT_TOKEN=... \
./scripts/validate-openproject.sh
```

Verification:

- file-mode import dry-run passes;
- snapshot persistence passes on a temporary project copy;
- import accept dry-run passes against the written snapshot;
- live API path is ready to execute when a real token is available.

Result:

- turns the next OpenProject validation step into one repeatable operator
  command;
- keeps OpenProject read-only;
- keeps credentials outside project files;
- leaves live API validation open until an actual OpenProject instance and
  token are available.

### Operator project-state overview design

Implemented first operator project-state overview slice:

- added `replan report state`;
- supported `--format json`, `--format markdown` and `--format text`;
- reused the existing dashboard overview model as the stable JSON result so
  agents see the same source health, workload, tasks, dependencies, proposals,
  evidence, scenarios, warnings, errors and next actions as the generated
  dashboard;
- added a compact Markdown operator report with sections:
  - sync health;
  - planning health;
  - tracking health;
  - plan versus actual;
  - resource pressure;
  - warnings;
  - next actions;
- kept OpenProject as the task/status/date source and human PM surface;
- kept RePlan as the resource planning, dependency, constraint, proposal and
  reporting layer;
- kept the first slice free of web UI, daemon, server, custom authentication,
  full role-based access control and OpenProject write-back.

Command:

```bash
replan report state --project examples/replan-self --week 2026-W31 --format markdown
replan report state --project examples/replan-self --week 2026-W31 --format json
```

Verification:

- CLI JSON envelope contract includes `report state`;
- renderer tests cover the Markdown report sections;
- MVP smoke runs `report state` against `examples/replan-self` in JSON and
  Markdown modes.

Implemented Plan versus Actual extension for `report state`:

- added a `tracking` block to the dashboard overview JSON model;
- each tracking row is scoped to a task and includes:
  - task id;
  - title;
  - tracking status;
  - planned start and finish;
  - actual count;
  - evidence count;
  - task-level tracking flags;
  - per-demand tracking rows;
- each demand tracking row keeps resource units separate and includes:
  - demand id;
  - resource id;
  - resource class;
  - role;
  - unit;
  - planned amount;
  - actual amount;
  - remaining amount;
  - forecast amount;
  - variance from plan;
  - evidence count;
  - demand-level flags;
- Markdown `report state` renders a `Plan vs Actual` section before resource
  pressure;
- next actions include `review_tracking` for actionable execution tracking
  flags;
- this is an operator execution-checking surface, not daily timesheets,
  payroll, employee surveillance or full earned value.

Remaining follow-up:

- decide whether `report state` should later gain a generated HTML artifact;
- decide whether source snapshot freshness should become an explicit section
  after live OpenProject validation;
- decide whether the Plan versus Actual rows should later support baseline
  variance in addition to current planned amount variance.

Implemented read-only tracking suggestion slice:

- added self-dogfood task `task.self.add-tracking-suggest-command`;
- added `replan tracking suggest`;
- the command reads the same canonical workspace and Plan versus Actual signals
  used by `report state`;
- JSON output uses the standard CLI envelope and includes a `suggestions` array;
- Markdown and text render a compact operator-facing suggestion table and
  details;
- the command is intentionally read-only:
  - it does not update `actuals`;
  - it does not update `resource_demands`;
  - it does not write proposals;
  - it does not write audit records;
  - it does not call OpenProject;
- implemented suggestion types:
  - `set_remaining_to_zero_candidate`;
  - `review_missing_actuals`;
  - `review_missing_evidence`;
  - `review_forecast_over_plan`;
- stale remaining candidates include a recommended
  `replan actual set-remaining --dry-run` command so a human operator or AI CLI
  agent can verify the next step before any write;
- MVP smoke runs `tracking suggest` against `examples/replan-self` in JSON and
  Markdown.

Implemented tracking candidate write slice:

- command:

```bash
replan tracking suggest --project examples/replan-self --week 2026-W31 --write-candidates --actor actor.ai.codex.local --format json
```

- scope:
  - materialize only reviewable `proposals/open/*.yaml` files;
  - do not mutate resource demands;
  - do not mutate actuals;
  - do not write audit records;
  - do not write evidence records;
  - do not call OpenProject;
  - do not auto-accept suggestions;
- `--actor` is required in write-candidates mode because proposal files need a
  stable `created_by` ref;
- first writable suggestion type:
  - `set_remaining_to_zero_candidate` becomes an `update_remaining_work`
    proposal with `suggested_change.op = set`,
    `suggested_change.path = remaining_amount` and
    `suggested_change.value = 0`;
- read-only suggestion types that remain advisory in this slice:
  - `review_missing_actuals`;
  - `review_missing_evidence`;
  - `review_forecast_over_plan`;
- after candidates are written, the existing proposal loop is used:
  - `replan proposal list`;
  - `replan proposal show`;
  - `replan proposal accept --dry-run`;
  - `replan proposal accept --actor <id> --apply`;
  - `replan proposal reject --reason <text> --apply`;
- MVP smoke should verify write-candidates mode against a temporary copy of
  `examples/replan-self`, not against the source fixture.

Remaining tracking follow-up:

- add live OpenProject time-entry import before relying on missing-actuals
  signals for real project governance;

Implemented tracking loop prioritization slice:

- added self-dogfood task
  `task.self.prioritize-tracking-loop`;

- extend `tracking_proposal_loop.rows` with:
  - normalized `status`;
  - `priority`;
  - `reason`;
- use statuses:
  - `actionable`;
  - `open`;
  - `accepted`;
  - other proposal statuses as-is;
- use priorities:
  - `high` for actionable stale remaining with positive variance;
  - `medium` for open proposals waiting for review;
  - `low` for accepted proposals that still appear in stale remaining rows;
- sort rows by:
  - status priority: actionable, open, accepted, other;
  - row priority: high, medium, low;
  - variance descending;
  - remaining descending;
  - task id;
  - demand id;
- render status, priority and reason in Markdown and static dashboard HTML;
- keep this slice read-only and reuse existing tracking loop calculations;
- update report tests, renderer tests and MVP smoke.

Implemented static dashboard tracking loop slice:

- added self-dogfood task
  `task.self.render-tracking-loop-dashboard`;

- render `Tracking Proposal Loop` in generated `reports/latest/index.html`;
- show summary counters:
  - stale remaining rows;
  - open tracking proposals;
  - accepted tracking proposals;
  - covered demand rows;
  - actionable demand rows;
- show a compact table of loop rows:
  - task;
  - demand;
  - resource;
  - remaining;
  - forecast;
  - proposal status;
  - next command;
- keep the dashboard page read-only;
- reuse existing `tracking_proposal_loop` JSON data;
- add renderer tests and MVP smoke assertions.

Implemented tracking proposal accept/apply slice:

- added self-dogfood task
  `task.self.apply-tracking-remaining-proposals`;

- command loop:

```bash
replan tracking suggest --project <temp-self-project> --week 2026-W31 --write-candidates --actor actor.ai.codex.local --format json
replan proposal show --project <temp-self-project> proposal.tracking.<demand>.set-remaining-zero --format markdown
replan proposal accept --project <temp-self-project> proposal.tracking.<demand>.set-remaining-zero --dry-run --format json
replan proposal accept --project <temp-self-project> proposal.tracking.<demand>.set-remaining-zero --actor actor.human.maintainer --apply --format json
```

- scope:
  - support `update_remaining_work`;
  - require `suggested_change.op = set`;
  - require `suggested_change.path = remaining_amount`;
  - require numeric `suggested_change.value`;
  - apply the value to the target `resource_demand.remaining_amount`;
  - move the proposal from `proposals/open` to `proposals/accepted`;
  - append proposal decision audit;
  - validate after mutation;
  - keep rejection as proposal status move plus audit only;
  - do not mutate actuals;
  - do not create evidence records;
  - do not call OpenProject.

Implemented tracking proposal loop observability slice:

- added self-dogfood task
  `task.self.add-tracking-loop-observability`;

- add a `tracking_proposal_loop` block to dashboard overview JSON;
- render the same block in `replan report state --format markdown`;
- count stale remaining demand rows from Plan versus Actual signals;
- count open `update_remaining_work` proposals generated from tracking
  suggestions;
- count accepted `update_remaining_work` tracking proposals;
- count stale remaining rows already covered by open or accepted tracking
  proposals;
- list actionable stale remaining rows with task, demand, proposal status and
  recommended next command;
- keep the slice read-only;
- do not create proposals;
- do not accept proposals;
- do not mutate canonical state;
- include the block in MVP smoke and report tests.

## Definition of MVP done

The first MVP is done when:

- `examples/small-team` validates;
- workload report answers who or what is overloaded;
- weekly Markdown and JSON reports are generated;
- scenario diff works;
- proposal list/show works;
- actual consumption can be recorded with audit;
- OpenProject read-only import can create import snapshot or mapped candidates;
- AI CLI agent can run validate/report/scenario/proposal commands safely;
- no web UI, server, custom auth or full solver is required.

## Explicit deferrals

Deferred until after first MVP:

- web UI;
- desktop app;
- daemon;
- hosted service;
- custom authentication;
- full role-based access control;
- OpenProject write-back;
- automatic resource leveling;
- TaskJuggler as required engine;
- complex earned value;
- payroll;
- employee surveillance;
- full cost accounting;
- enterprise reporting.

## Decisions captured

- Implementation starts with Go module and CLI skeleton.
- First real command is `replan validate`.
- Fixtures come before OpenProject import.
- Workload reports come before scenario mutation.
- Scenario diff comes before scenario apply.
- Proposal and audit read paths come before risky mutations.
- Actuals and proposal acceptance require actor and audit.
- OpenProject integration starts read-only.
- TaskJuggler is optional export/proof after core reports work.
- UI, daemon, server and enterprise features are explicitly later.
