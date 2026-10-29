# Deployment architecture

Date: 2026-07-06

Status: core architecture decision.

## Purpose

This document defines the intended deployment architecture of RePlan.

It answers:

- whether RePlan starts as client-server, peer-to-peer, git-backed or local
  application;
- how the MVP should run;
- how Git should be used;
- whether SQLite is expected;
- how a future client-server deployment remains possible;
- what architectural mistakes would block that transition.

The runtime shape of the application itself is defined in
[`application-runtime.md`](application-runtime.md).

## Core decision

RePlan MVP is local-first.

It is not client-server in MVP.

It is not peer-to-peer in MVP.

It uses Git as collaboration, history, review and synchronization substrate
where useful, but Git is not mandatory runtime.

Approved rule:

```text
local-first is the first deployment mode, not a permanent architectural prison
```

Target MVP shape:

```text
CLI over reusable core/application services
  -> local project directory
  -> file-based canonical state
  -> generated reports
  -> optional Git commit/push for sync, history and review
  -> source adapters such as OpenProject, GitHub and GitLab
```

Future shape:

```text
same core domain/application services
  -> server API
  -> database storage
  -> web UI
  -> external auth
  -> background sync workers
```

The future server should wrap the same core operations, not replace the domain
model.

The required MVP runtime is single-shot CLI. A local daemon/service and
desktop/web UI are later optional layers.

## Deployment modes

### MVP mode: local-first CLI/library

The first working product should be:

```text
local CLI + local files + generated reports
```

Expected commands:

```bash
replan validate
replan import openproject
replan workload
replan scenario diff
replan report weekly
```

The MVP should not require:

- hosted SaaS;
- backend server;
- external database;
- cloud account;
- enterprise identity provider;
- always-on daemon;
- web browser UI;
- network access after data import.

This matches the product decisions:

- AI-CLI-first;
- FOSS-first;
- small-team-first;
- no custom auth in MVP;
- human-verifiable generated outputs;
- safe dry-run/proposal workflows.

### Collaboration mode: Git-backed project state

Git should be the first collaboration and history mechanism.

Git can provide:

- version history;
- branch-based scenario work;
- pull-request or merge-request review;
- multi-machine synchronization;
- backup;
- audit-friendly trail;
- AI-agent-native workflow;
- diff review for scenario/proposal/report changes.

Example workflow:

```text
agent creates branch
agent adds scenario/proposal files
agent runs reports
agent commits generated diff
human reviews pull request
accepted changes are applied explicitly
```

Git must not be required for single-machine operation.

RePlan should work in a normal directory.

If that directory is a Git repository, RePlan can benefit from Git.

### Optional local database mode: SQLite cache/index

SQLite may be introduced after the file model becomes inconvenient.

Expected use:

- local query index;
- calculation cache;
- faster report generation;
- local event/proposal/audit queries;
- local source import cache.

SQLite should not become the only source of truth until the storage design is
explicitly changed.

Preferred early posture:

```text
files are canonical; SQLite is derived/cache
```

Later, if needed:

```text
SQLite can become local canonical storage behind a storage adapter
```

That transition must preserve export/import to file formats.

### Future mode: optional client-server deployment

Client-server is a future deployment mode.

Reasons it may become useful:

- multi-user web UI;
- centralized API;
- live dashboards;
- webhooks;
- background import workers;
- hosted teams;
- external authentication;
- RBAC enforcement;
- centralized audit;
- larger installations.

Client-server is not MVP because it would bring too much early complexity:

- auth and sessions;
- database migrations;
- deployment;
- hosting;
- backups;
- API versioning;
- permission model enforcement;
- multi-user conflict handling;
- security surface;
- web UI scope.

The design must still keep the path open.

### Non-goal: peer-to-peer MVP

Peer-to-peer is not part of MVP and not a near-term target.

P2P would require solving:

- identity;
- trust;
- conflict resolution;
- partial visibility;
- offline sync protocol;
- permission propagation;
- audit reconciliation;
- merge semantics.

Git already provides enough distributed collaboration for the intended first
users.

Therefore P2P should be considered only after RePlan proves value and if a real
use case appears.

## Layering required for future client-server

The local-first MVP must be structured so that future server mode can reuse the
same core.

Required layers:

```text
domain model
application services / commands
storage adapter
source adapters
calculation engine
report renderer
policy/audit layer
CLI
future server API
future web UI
```

The CLI should call application services.

A future server API should call the same application services.

The report renderer should consume report models, not CLI-only state.

Storage should be accessed through a storage boundary, not through ad hoc file
writes scattered across business logic.

## Mapping from local-first to server mode

Approved migration mapping:

```text
CLI command today -> application service call tomorrow
file storage today -> database storage adapter tomorrow
local audit log today -> server audit log tomorrow
git review today -> web/API approval tomorrow
static report today -> web dashboard tomorrow
local policy context today -> server auth/policy context tomorrow
source import command today -> background sync worker tomorrow
JSON report today -> API response tomorrow
```

This mapping should influence canonical model and storage layout decisions.

## Operation model

Every meaningful mutation should be represented as an operation.

Examples:

- create baseline;
- add resource demand;
- accept proposal;
- apply scenario;
- record actual;
- import source snapshot;
- refresh OpenProject data;
- create report;
- propose write-back;

Operations should accept an actor context even in local mode.

Example:

```text
actor_id: operator
actor_type: human_operator
operation: apply_scenario
dry_run: true
```

Why this matters:

- local audit works from the beginning;
- AI agents can be constrained;
- future server auth can provide actor context;
- future API can reuse the same operation contract.

## Storage architecture

MVP storage should be file-based.

Candidate local layout:

```text
replan/
  project.yaml
  resources/
  tasks/
  baselines/
  current/
  scenarios/
  actuals/
  evidence/
  proposals/
  audit/
  reports/
  imports/
```

The exact layout belongs to `docs/spec/storage-layout.md`.

This architecture decision only requires:

- stable identifiers;
- explicit object boundaries;
- machine-readable files;
- deterministic reads;
- deterministic writes;
- audit records for mutations;
- no hidden UI-only state.

## Git usage

Git should be used for:

- history;
- review;
- synchronization;
- agent branches;
- scenario review;
- proposal review;
- report artifact history;
- backup.

Git should not be used as:

- a replacement for the domain model;
- the only audit mechanism;
- the only permission model;
- a required runtime database;
- a conflict resolver for domain semantics.

Git merge conflicts may happen.

RePlan should make files and object boundaries small enough that conflicts are
understandable.

## Auth and policy

MVP does not implement custom authentication.

However, MVP operations must include actor and policy context.

This is required for:

- AI-agent constraints;
- audit;
- proposal-only default behavior;
- dry-run mutation safety;
- future server transition.

Local mode can use configured actors.

Future server mode can map external auth identities to RePlan actors.

## Report architecture

Reports should be generated from structured report models.

The same report model should render to:

- JSON;
- Markdown;
- CSV;
- optional static HTML;
- future web API response;
- future dashboard view.

Reports must not depend on hidden CLI state or UI-only state.

## Adapter architecture

Source adapters should not know whether RePlan is running locally or in a
server.

They should expose operations such as:

- import source tasks;
- refresh source snapshot;
- map source status;
- produce source mismatch warnings;
- prepare write-back proposal;
- dry-run write-back;
- apply approved write-back.

Local CLI and future server workers should call the same adapter behavior.

## Anti-patterns to avoid

The following would make future client-server transition harder:

- business logic inside CLI handlers;
- direct YAML writes from many unrelated modules;
- no application service layer;
- no storage abstraction;
- no stable object identifiers;
- no actor context in local operations;
- no audit model;
- hidden global state based on current working directory;
- calculations depending on filesystem side effects;
- report output generated directly from ad hoc loaded objects;
- UI-only state;
- Git-specific assumptions inside domain logic;
- OpenProject-specific assumptions inside canonical model;
- silently mutating source systems;
- silently mutating baselines;
- treating local file paths as permanent object identity.

These anti-patterns should be checked again when designing:

- canonical model;
- storage layout;
- CLI;
- operation/application service layer;
- adapter interfaces.

## MVP requirements

MVP should implement or model:

- local-first CLI/library;
- file-based canonical state;
- Git-friendly object layout;
- generated JSON/Markdown/CSV reports;
- explicit operations;
- actor context;
- dry-run support for risky mutations;
- local audit records;
- source adapters;
- report models independent of renderer;
- storage boundary;
- no custom authentication provider;
- no required backend server.

## Explicit MVP non-goals

MVP should not implement:

- client-server backend;
- web UI;
- hosted service;
- P2P sync;
- realtime collaboration;
- server-side RBAC enforcement;
- OIDC/SAML login flow;
- multi-tenant database;
- background worker system;
- live dashboard.

## Future server readiness requirements

The MVP design should keep these future transitions possible:

- replace file storage with database storage adapter;
- wrap operations in HTTP or RPC API;
- run source imports in background workers;
- map external auth identities to RePlan actors;
- enforce policies server-side;
- serve report models through API;
- render dashboards from report models;
- store audit records centrally;
- support multi-user proposal review;
- add web UI without rewriting planning logic.

## Approved decisions

- RePlan MVP is local-first.
- RePlan MVP is not client-server.
- RePlan MVP is not peer-to-peer.
- Git is collaboration, history, review and synchronization substrate, not
  mandatory runtime.
- File-based canonical state is the first storage mode.
- SQLite may later be used as local cache, index or storage adapter.
- Client-server is a future optional deployment mode.
- Local-first must not block future client-server architecture.
- CLI and future server API should call the same application services.
- Business logic must not live only in CLI handlers.
- Mutations should be explicit operations with actor context and audit.
- Reports should be generated from structured report models independent of
  renderer.
- Source adapters should be deployment-mode agnostic.
- P2P is not MVP and not a near-term target.
