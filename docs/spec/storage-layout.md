# RePlan storage layout

Date: 2026-07-06

Status: approved implementation specification.

## Purpose

This document defines how the first local-first RePlan project is stored on
disk.

It answers:

- where canonical project state lives;
- where imported raw source snapshots live;
- where generated reports live;
- where baselines, scenarios, actuals, evidence, proposals and audit records
  live;
- what is hand-editable and what is generated;
- how files should be named;
- how YAML and JSON should be used;
- how Git should be used without becoming mandatory runtime;
- how future SQLite or server storage can be introduced without changing the
  domain model.

This document depends on:

- `docs/spec/canonical-model.md`;
- `docs/spec/export-artifacts.md`;
- `docs/architecture/deployment-architecture.md`;
- `docs/architecture/application-runtime.md`.

## Core decision

RePlan MVP storage is local-first and file-based.

Approved rule:

```text
local project directory -> file-based canonical state -> validation/calculation/reporting
```

Git may be used for history, review, synchronization and backup.

Git is not required for RePlan to run.

SQLite may be added later as cache, index or storage adapter.

SQLite is not the first canonical storage mode.

## Storage goals

The first storage layout must be:

- understandable to humans;
- safe for AI CLI agents;
- friendly to Git diffs;
- deterministic enough for validation;
- explicit about generated versus hand-authored files;
- explicit about raw imports versus canonical state;
- compatible with future SQLite storage adapter;
- compatible with future server API;
- small enough to implement in the first MVP.

## Storage non-goals

The MVP storage layout should not try to solve:

- multi-user database transactions;
- row-level security;
- encrypted-at-rest secrets store;
- high-volume event ingestion;
- binary artifact management;
- full document management;
- distributed peer-to-peer sync;
- real-time collaboration;
- enterprise retention policy;
- database migrations.

Those may matter later, but they should not shape the first file format beyond
keeping a clean storage boundary.

## Project root

A RePlan project is a directory containing a `.replan/` control directory and
project data.

Recommended layout:

```text
replan-project/
  .replan/
    project.yaml
    policy.yaml
    lock/
    index/
  canonical/
    actors.yaml
    resources.yaml
    resource_pools.yaml
    tasks.yaml
    resource_demands.yaml
    capacities.yaml
    availability.yaml
    dependencies.yaml
    deadlines.yaml
    current_plan.yaml
  baselines/
    baseline.2026-07-06.initial.yaml
  actuals/
    actuals.2026-W28.yaml
  evidence/
    evidence.2026-W28.yaml
    files/
  proposals/
    open/
    accepted/
    rejected/
    superseded/
  scenarios/
    scenario.ivan-unavailable-week-28.yaml
  audit/
    audit.2026-07.jsonl
  integrations/
    openproject/
      resource-demands.yaml
      resource-links.yaml
  imports/
    openproject/
      20260706T120000/
        manifest.yaml
        raw.json
        mapped.yaml
  exports/
    taskjuggler/
      scenario.ivan-unavailable-week-28/
        project.tjp
        project.manifest.json
  reports/
    weekly/
      2026-W28.md
      2026-W28.json
      2026-W28.csv
  tmp/
```

This is the recommended MVP layout.

Exact filenames can evolve, but the categories should remain stable.

## Directory responsibilities

### `.replan/`

Stores RePlan project control files.

Expected contents:

```text
.replan/
  project.yaml
  policy.yaml
  lock/
  index/
```

`project.yaml` stores the project descriptor:

```yaml
project:
  id: project.replan-mvp
  name: RePlan MVP
  format_version: 1
  timezone: Asia/Novosibirsk
  default_status_date: "2026-07-06"
  openproject_ids: canonical
  source_profiles:
    - id: openproject.demo
      source: openproject
      base_url: https://pm.example.test
      project_id: demo
      token_env: OPENPROJECT_TOKEN
      default_query:
        include_closed: "false"
```

#### Format version

`project.format_version` is an integer version for the complete persisted
RePlan project format: the project descriptor and the canonical, audit,
scenario, proposal, actual, evidence and related files loaded with it. The
current format is `1`. A missing field means format `1`, so projects created
before versioning load without a warning.

A binary stops after reading `project.yaml` when the value is zero, negative,
not an integer, or newer than it supports. It reports the single
`project_format_unsupported` validation error and does not load other project
files. `replan project migrate` upgrades an older project through each known
format step and never downgrades a project. For a format-1 project whose field
is missing, migration only inserts `format_version: 1` and records the audited
change.

`project.openproject_ids` controls whether OpenProject object ids may be
persisted in project files. The value is `canonical` or `private_bindings`;
when omitted it defaults to `canonical` for compatibility. Under
`private_bindings`, live API import is dry-run-only and import acceptance
refuses task/source-ref, resource-link, dependency and time-entry changes that
would persist OpenProject ids. Date, tracking-status and resource-demand
acceptance remains available. File-mode import may still write a snapshot
because it copies input that is already on disk; projects using this policy
should ignore their snapshot directory.

`policy.yaml` stores MVP local policy:

```yaml
policy:
  id: policy.local-trusted
  mode: local_trusted
  default_actor: actor.human.operator
  require_dry_run_for:
    - write_back_to_source
    - apply_scenario
  ai_agents:
    default_mutation_mode: proposal_only
```

Rules:

- `.replan/project.yaml` is canonical project metadata;
- `.replan/policy.yaml` is canonical local policy metadata;
- `.replan/lock/` is runtime-only and should not be committed;
- `.replan/index/` is derived cache and should not be canonical in MVP.

### `canonical/`

Stores the hand-authored or accepted canonical planning state.

Expected MVP files:

```text
canonical/
  actors.yaml
  resources.yaml
  resource_pools.yaml
  tasks.yaml
  resource_demands.yaml
  capacities.yaml
  availability.yaml
  dependencies.yaml
  deadlines.yaml
  current_plan.yaml
```

Rules:

- this is the main canonical state;
- files are YAML for human and AI-agent readability;
- each object must have stable `id`;
- ordering should be deterministic by `id`;
- references must use canonical ids, not display names;
- source-specific raw fields do not belong here;
- adapter-derived canonical objects are allowed but must keep `source_refs`.

`canonical/dependencies.yaml` stores accepted dependency records.

MVP dependency persistence rules:

- read-only imports may produce `dependency_candidates`, but they do not mutate
  this file;
- `import accept --accept-dependencies` should be the first mutating workflow
  allowed to append OpenProject relation-backed dependencies;
- accepted dependency records should keep source references such as
  `source=openproject`, `object_type=relation` and the OpenProject relation id;
- duplicate detection should check both source reference and canonical
  predecessor/successor/type triple;
- dependency records should reference canonical task ids, not OpenProject work
  package ids;
- cross-project or partially imported source relations should remain import
  warnings until both sides can be matched or a separate external-dependency
  design is accepted.

### `baselines/`

Stores approved baseline snapshots.

Example:

```text
baselines/
  baseline.2026-07-06.initial.yaml
  baseline.2026-07-15.reapproved-mvp.yaml
```

Rules:

- baseline files are immutable after approval;
- correcting a baseline requires a new baseline file;
- newly created files snapshot task dates and demand estimates and carry a
  semantic SHA-256 seal; legacy ref-only, unsealed files remain readable;
- reports must identify which baseline they compare against;
- baseline filename should include date and short slug;
- baseline content should include `approved_by` and `approved_at`.

### `actuals/`

Stores accepted execution facts.

Recommended MVP shape:

```text
actuals/
  actuals.2026-W28.yaml
  actuals.2026-W29.yaml
```

Why weekly files:

- actual records can grow quickly;
- weekly files keep diffs reviewable;
- reports often work by week;
- AI agents can inspect a bounded file instead of scanning all history.

Rules:

- actuals are accepted facts, not raw events;
- actuals should reference evidence when available;
- actuals must include actor or policy-approved origin;
- actuals do not rewrite baseline;
- actuals may update current-plan forecast through calculation, not by
  mutating baseline.

### `evidence/`

Stores evidence records and optional local evidence artifacts.

Recommended MVP shape:

```text
evidence/
  evidence.2026-W28.yaml
  evidence.2026-W29.yaml
  files/
    repair-17/
      pump-installed.jpg
```

Rules:

- evidence records are canonical metadata;
- evidence artifacts may be local files or external URLs;
- large binary files should be avoided in Git unless intentionally accepted;
- evidence must not contain secrets;
- evidence is not automatically actual;
- evidence may support proposals, warnings or actuals.

### `proposals/`

Stores suggested changes.

Recommended MVP shape:

```text
proposals/
  open/
    proposal.adjust-remaining-task-op-1234.20260706.yaml
  accepted/
    proposal.adjust-remaining-task-op-1200.20260705.yaml
  rejected/
  superseded/
```

Rules:

- open proposals are reviewable files;
- accepting a proposal moves or copies it to `accepted/` and creates audit;
- rejecting a proposal moves or copies it to `rejected/` and creates audit;
- superseded proposals move or copy to `superseded/`;
- proposal files should preserve rationale and evidence references;
- AI agents should normally create proposal files, not mutate canonical state
  directly.

Implementation note:

For MVP, moving proposal files between directories is acceptable.

Later, proposal status may be stored inside a database or index while preserving
export to this file layout.

### `scenarios/`

Stores what-if overlays.

Example:

```text
scenarios/
  scenario.ivan-unavailable-week-28.yaml
  scenario.add-codex-pool.yaml
```

Rules:

- scenario is overlay, not full project copy;
- scenario file stores base reference and explicit changes;
- scenario calculation output should not overwrite the scenario definition;
- generated scenario reports belong under `reports/`;
- applying a scenario mutates current plan only through an audited operation.

### `audit/`

Stores append-oriented local audit records.

Recommended MVP shape:

```text
audit/
  audit.2026-07.jsonl
  audit.2026-08.jsonl
```

Each line is one JSON object.

Why JSON Lines:

- append-friendly;
- machine-readable;
- easy to stream;
- stable for CLI agents;
- safer for audit than rewriting one large YAML file.

Example:

```json
{"id":"audit.20260706T151500.adjust-remaining-task-op-1234","occurred_at":"2026-07-06T15:15:00+07:00","actor_ref":{"kind":"actor","id":"actor.human.operator"},"operation":"accept_proposal","target_ref":{"kind":"proposal","id":"proposal.adjust-remaining-task-op-1234.20260706"},"result":"success"}
```

Rules:

- audit is append-oriented;
- audit records should not be hand-edited in normal workflow;
- audit must not contain secrets;
- every mutating command should emit audit;
- validation may warn if canonical changes exist without recent audit, but Git
  history is not a substitute for RePlan audit.

### `imports/`

Stores raw imported source snapshots and adapter mapping output.

Recommended shape:

```text
imports/
  openproject/
    20260706T120000/
      manifest.yaml
      raw.json
      mapped.json
  github/
    20260706T121000/
      manifest.yaml
      raw.json
      mapped.yaml
```

`manifest.yaml` example:

```yaml
source_snapshot:
  id: source_snapshot.openproject.20260706T120000
  source: openproject
  imported_at: "2026-07-06T12:00:00+07:00"
  imported_by:
    kind: actor
    id: actor.sync.openproject
  raw_location: imports/openproject/20260706T120000/raw.json
  mapped_location: imports/openproject/20260706T120000/mapped.json
  adapter_version: replan-openproject-adapter-v0
  object_count: 42
```

Rules:

- raw import is not canonical current plan;
- raw import is preserved for traceability and adapter debugging;
- `mapped.json` or `mapped.yaml` may contain adapter-produced canonical
  candidates; OpenProject MVP writes `mapped.json`;
- OpenProject `dependency_candidates` in `mapped.json` are read-only until
  `import accept --accept-dependencies` is explicitly implemented;
- accepted mapped objects are copied or merged into canonical state through an
  explicit operation;
- source tokens and secrets must not be stored in imports;
- imports may be ignored or pruned later by policy, but initial MVP should keep
  enough source snapshot data for debugging.

### `integrations/`

Stores hand-authored or operator-reviewed bridge data for source systems.

Recommended shape:

```text
integrations/
  openproject/
    resource-demands.yaml
    resource-links.yaml
```

This directory is different from `imports/`.

`imports/` stores generated source snapshots and adapter mapping output.

`integrations/` stores the local convention that explains how an external
system should connect to RePlan canonical state when the external system cannot
represent the full RePlan model directly.

OpenProject MVP usage:

- `integrations/openproject/resource-demands.yaml` maps OpenProject work
  packages to RePlan resource-demand candidates;
- `integrations/openproject/resource-links.yaml` may later map OpenProject
  users, custom roles, labels or other source objects to RePlan resources when
  automatic matching is insufficient;
- accepted demands are still written to `canonical/resource_demands.yaml`;
- integration sidecars are reviewed, diffed and committed like source data;
- integration sidecars must not contain API tokens, passwords or other secrets.

Rules:

- sidecar files are not raw source snapshots;
- sidecar files are not generated reports;
- sidecar files are allowed to be hand-authored by humans or AI CLI agents;
- sidecar files should use YAML because reviewability is more important than
  compact machine output;
- sidecar records should reference external objects by stable source id, not by
  volatile title text alone;
- validation should warn when a sidecar references a missing import object,
  missing canonical resource or unsupported unit;
- future server mode can store these records as integration mapping tables.

### `exports/`

Stores generated export artifacts.

Recommended layout is defined in `docs/spec/export-artifacts.md`.

Short example:

```text
exports/
  workload/
    2026-W28/
      current.csv
      current.manifest.json
  taskjuggler/
    scenario.ivan-unavailable-week-28/
      project.tjp
      project.manifest.json
```

Rules:

- exports are generated artifacts;
- exports are not the canonical model;
- generated exports may be committed when useful for review;
- committed export artifacts should have neighboring `.manifest.json` files;
- export manifest should identify source scenario or current plan, command
  flags, output path and checksum;
- export should be reproducible from canonical state plus command options.

### `reports/`

Stores generated human-readable and machine-readable reports.

Recommended shape:

```text
reports/
  weekly/
    2026-W28.md
    2026-W28.json
    2026-W28.csv
  scenarios/
    scenario.ivan-unavailable-week-28.md
    scenario.ivan-unavailable-week-28.json
  warnings/
    2026-07-06.md
    2026-07-06.json
```

Rules:

- reports are generated artifacts;
- JSON is the canonical machine-readable report output;
- Markdown is the main human-readable output;
- CSV is for spreadsheet workflows;
- reports should state status date, baseline, current plan and source snapshots;
- reports should be reproducible;
- reports should not contain source tokens or secrets.

### `tmp/`

Stores temporary files.

Rules:

- `tmp/` is runtime-only;
- `tmp/` should not be committed;
- failed partial writes should be cleaned up where possible.

## YAML versus JSON

### YAML

Use YAML for hand-authored or reviewable canonical state:

- project metadata;
- local policy;
- actors;
- resources;
- resource pools;
- tasks;
- resource demands;
- capacities;
- availability;
- dependencies;
- deadlines;
- current plan;
- baselines;
- scenarios;
- proposals;
- evidence metadata;
- actuals.

Reason:

YAML is easier for humans and AI CLI agents to inspect, edit and review in Git.

### JSON

Use JSON for generated machine output and raw source snapshots:

- raw imports;
- JSON reports;
- audit JSON Lines;
- adapter cache or index files;
- future API-compatible export snapshots.

Reason:

JSON is stricter, easier for tooling and better for generated structured output.

### JSON Lines

Use JSON Lines for append-oriented audit.

Potential later uses:

- event streams;
- daemon logs;
- import event logs.

## Canonical versus generated files

### Canonical files

Canonical files are inputs to validation, calculation and reporting.

Canonical MVP files:

```text
.replan/project.yaml
.replan/policy.yaml
canonical/*.yaml
baselines/*.yaml
actuals/*.yaml
evidence/*.yaml
proposals/**/*.yaml
scenarios/*.yaml
audit/*.jsonl
```

Audit is canonical history, but should be append-oriented and not hand-edited.

### Generated files

Generated files can be recreated from canonical state and command options.

Generated directories:

```text
reports/
exports/
.replan/index/
tmp/
```

Generated files may still be committed when useful for review.

Committing generated reports is allowed because RePlan is AI-CLI-first and
human-verifiable.

The important rule is that generated reports do not become the source of truth.

### Imported files

Imported files are source snapshots.

They are not current plan.

They become canonical only through adapter mapping plus explicit accept/import
operation.

## File naming rules

Use predictable, lowercase names.

Recommended characters:

```text
a-z
0-9
.
_
-
/
```

Avoid spaces in filenames.

Recommended object filename patterns:

```text
baseline.<date>.<slug>.yaml
scenario.<slug>.yaml
proposal.<slug>.<date-or-timestamp>.yaml
actuals.<iso-week>.yaml
evidence.<iso-week>.yaml
audit.<yyyy-mm>.jsonl
```

Examples:

```text
baseline.2026-07-06.initial.yaml
scenario.ivan-unavailable-week-28.yaml
proposal.adjust-remaining-task-op-1234.20260706.yaml
actuals.2026-W28.yaml
evidence.2026-W28.yaml
audit.2026-07.jsonl
```

## Object ordering

Within YAML collection files, objects should be sorted by `id`.

Reason:

- smaller Git diffs;
- deterministic validation output;
- easier AI-agent inspection;
- fewer merge conflicts.

If an object has natural chronological ordering, chronological files may be
used:

- audit records;
- actuals by week;
- evidence by week;
- source imports by timestamp.

## Write behavior

Mutating RePlan commands should use safe writes:

1. read and validate current state;
2. calculate intended changes;
3. write to temporary file;
4. fsync or equivalent where practical;
5. atomic rename into place where supported;
6. write audit record;
7. return structured result.

For operations that touch several files, MVP may use a simple file lock.

Lock location:

```text
.replan/lock/
```

Rules:

- lock files are runtime state;
- lock files should not be committed;
- stale lock behavior should be explicit;
- AI agents should not bypass mutating commands by editing files directly when
  command support exists.

## Git behavior

Git is recommended but not required.

Recommended committed files:

```text
.replan/project.yaml
.replan/policy.yaml
canonical/
baselines/
actuals/
evidence/*.yaml
proposals/
scenarios/
audit/
reports/
exports/
imports/*/*/manifest.yaml
imports/*/*/mapped.yaml
```

Files that usually should not be committed:

```text
.replan/lock/
.replan/index/
private/
private-pilot/
local-pilot/
.replan-local/
tmp/
secrets/
*.token
*.secret
```

Raw imports may be committed or ignored depending on project policy.

For early MVP, committing raw imports can be useful for adapter debugging.

For sensitive projects, raw imports may contain private task data and should be
excluded or redacted.

## Secrets

Secrets must not be stored in RePlan project files.

Not allowed:

- OpenProject API token;
- GitHub token;
- GitLab token;
- LLM provider API key;
- database password;
- session cookie;
- OAuth refresh token.

Allowed:

- reference to environment variable name;
- reference to external credential profile;
- non-secret source URL;
- non-secret source object id.

Public examples may contain sanitized source object identifiers.

Private live dogfood bindings should not be committed even when the object id is
not a credential. Store live project bindings in ignored local/private storage,
for example:

```text
private/openproject/<project>-bindings.yaml
```

Use committed files such as `examples/openproject/private-bindings.example.yaml`
only to document the shape with fake ids and example hosts.

Example:

```yaml
project:
  source_profiles:
    - id: openproject.demo
      source: openproject
      base_url: https://pm.example.test
      project_id: demo
      token_env: OPENPROJECT_TOKEN
```

Credential storage is outside MVP storage layout.

## Example minimal MVP project

```text
examples/small-team/
  .replan/
    project.yaml
    policy.yaml
  canonical/
    actors.yaml
    resources.yaml
    resource_pools.yaml
    tasks.yaml
    resource_demands.yaml
    capacities.yaml
    availability.yaml
    current_plan.yaml
  baselines/
    baseline.2026-07-06.initial.yaml
  actuals/
    actuals.2026-W28.yaml
  evidence/
    evidence.2026-W28.yaml
  proposals/
    open/
  scenarios/
    scenario.ivan-unavailable-week-28.yaml
  audit/
    audit.2026-07.jsonl
  reports/
    weekly/
```

This example should be created when implementation begins.

It must include:

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

## Validation responsibilities

Storage validation must check:

- required files exist;
- YAML files parse;
- JSON Lines audit records parse line by line;
- ids are unique within validation scope;
- references resolve;
- canonical files do not contain known source-specific raw fields;
- generated directories are not required for validation success;
- raw import absence does not invalidate canonical state;
- secrets are not present in obvious fields;
- generated report metadata references existing baseline/current plan where
  applicable.

Validation should produce structured JSON output for AI CLI agents.

## Future SQLite path

SQLite may be introduced after file storage becomes inconvenient.

Allowed SQLite uses:

- query index;
- calculation cache;
- import cache;
- report cache;
- local audit query index;
- local daemon state;
- optional canonical storage adapter later.

Required architectural rule:

```text
application services -> storage interface -> file storage or SQLite storage
```

The CLI must not directly depend on YAML file paths for domain behavior.

If SQLite becomes canonical later:

- file export must remain available;
- file import must remain available;
- canonical identifiers must remain stable;
- audit semantics must remain stable;
- CLI command behavior must remain stable;
- reports must not change meaning.

## Future server path

The file layout should map cleanly to future server concepts:

```text
project directory -> server project/workspace
canonical YAML files -> database tables or documents
baselines/ -> baseline records
actuals/ -> actual records
evidence/ -> evidence records and artifact storage
proposals/ -> proposal records
scenarios/ -> scenario records
audit/*.jsonl -> audit table/event log
reports/ -> generated report artifacts
imports/ -> source snapshot storage
integrations/ -> source-system bridge mappings
exports/ -> export artifact storage
```

Server mode should reuse the same domain model and application services.

Server mode should not require redefining what a task, resource, demand,
actual, scenario or proposal means.

## Decisions captured

- MVP storage is local-first and file-based.
- `.replan/` stores project control metadata, policy, locks and derived index.
- `canonical/` stores main accepted planning state.
- `baselines/`, `actuals/`, `evidence/`, `proposals/`, `scenarios/` and
  `audit/` are separate because they have different lifecycle rules.
- raw imports live under `imports/` and are not current plan.
- integration bridge mappings live under `integrations/` and are not accepted
  canonical state until an explicit operation copies or merges them into
  `canonical/`.
- generated reports live under `reports/` and are not source of truth.
- generated exports live under `exports/` and are not source of truth.
- YAML is preferred for hand-authored and reviewable canonical state.
- JSON is preferred for generated machine output and raw snapshots.
- JSON Lines is preferred for append-oriented audit.
- Git is recommended for collaboration and review, but not mandatory runtime.
- secrets must not be stored in project files.
- future SQLite and server modes must go through a storage adapter boundary.
