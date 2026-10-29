# Application runtime architecture

Date: 2026-07-06

Status: core architecture decision.

## Purpose

This document defines how RePlan should be implemented as a runnable
application.

It answers:

- whether RePlan should start as a desktop app;
- whether RePlan should require a daemon/service;
- what "headless-first" means;
- how the CLI, daemon and future desktop/web UI relate to the core;
- how this supports AI-CLI-first operation.

This document complements
[`deployment-architecture.md`](deployment-architecture.md).

The recommended implementation stack for this runtime is documented in
[`implementation-stack.md`](implementation-stack.md).

## Core decision

RePlan is headless-first.

It is not desktop-first.

It does not require a daemon/service in MVP.

Approved rule:

```text
core library + CLI first
optional local daemon later
desktop/web UI later
```

The required MVP runtime is:

```text
reusable core/application services + CLI
```

The optional later runtime is:

```text
local daemon/service + local API + desktop/web UI
```

## Target runtime shape

```text
RePlan core
  domain model
  validation
  workload calculation
  scenario calculation
  reports
  import/export
  proposal/audit operations

Application services
  operations
  actor context
  policy checks
  dry-run behavior
  storage boundary
  audit boundary

CLI
  thin wrapper over application services

Optional local daemon
  watches files
  caches indexes
  runs background imports
  serves local API
  helps desktop/web UI

Desktop/web UI
  later human-facing shell over daemon/API or generated reports
```

## Why not desktop first

Desktop-first would move the project toward UI packaging before the core
planning model is proven.

It would add early work around:

- windowing;
- frontend state management;
- desktop packaging;
- auto-update;
- operating-system differences;
- embedded database lifecycle;
- local permissions;
- UI design;
- crash reporting;
- human interaction design.

Those are real product concerns, but they are not the first risk.

The first RePlan risk is whether the model can correctly represent and explain:

- resources;
- capacity;
- scenarios;
- actuals;
- evidence;
- proposals;
- workload;
- reports.

Therefore desktop UI should be a later shell, not the starting point.

## Why not required daemon first

A daemon can be useful, but making it required in MVP creates unnecessary
runtime complexity.

Required daemon questions:

- how is it installed;
- how is it started;
- how is it stopped;
- where does it keep state;
- how does it update;
- how does it expose API;
- how does it lock files;
- how does it handle version mismatch;
- how does it run in CI;
- how do agents discover it;
- how are logs/debugging handled.

For AI-CLI-first MVP, the safest runtime is an explicit deterministic command:

```bash
replan validate
replan report weekly --format json
replan scenario diff scenario-001
```

The command should:

```text
read state -> run operation -> write explicit output -> exit
```

This is easier to test, easier to run by agents and easier to audit.

## Optional local daemon

A local daemon may be introduced later.

Working name:

```text
repland
```

Possible responsibilities:

- watch project files;
- maintain SQLite/cache/index;
- run scheduled source imports;
- receive local or remote webhooks;
- serve local HTTP or RPC API;
- support desktop UI;
- support local web UI;
- run background calculations;
- show notifications;
- manage local operation locks;
- expose current report models quickly.

The daemon must be optional.

The CLI must continue to work without it.

## Daemon boundaries

The daemon must not contain unique business logic.

Approved rule:

```text
daemon calls the same application services as CLI
```

The daemon may add:

- scheduling;
- caching;
- watch mode;
- local API transport;
- background job execution;
- local lock coordination.

The daemon must not redefine:

- domain rules;
- workload calculation;
- scenario logic;
- proposal rules;
- audit rules;
- adapter mapping;
- report semantics.

## CLI boundaries

The CLI is required in MVP.

The CLI should be a thin wrapper over application services.

CLI responsibilities:

- parse arguments;
- construct operation request;
- provide actor context;
- call application service;
- render output;
- return structured exit code.

CLI should not contain:

- domain logic;
- workload formulas;
- storage-specific business rules;
- source adapter business logic;
- report calculation logic.

## Desktop and human UI

Desktop or web UI is a later human-facing shell.

It may use:

- local daemon API;
- generated JSON reports;
- generated static HTML;
- local files;
- future server API.

The first human observability UI should not be a full web application.

Approved first UI layer:

```text
generated static observability dashboard
```

This layer is produced by CLI/report commands, reads the same report models as
JSON and Markdown, writes static files and exits.

It does not require:

- daemon;
- server API;
- RePlan authentication;
- role-based access control;
- browser-side application state;
- direct OpenProject write-back.

The intended output shape is:

```text
reports/
  latest/
    index.html
    weekly.html
    workload.html
    dependencies.html
    warnings.html
    proposals.html
    raw/
      *.json
```

Initial placement should be local inside the RePlan project or generated report
directory. Later the same static files may be published read-only behind a
private reverse proxy, for example under an internal reports path or subdomain.

The UI should not contain unique business logic.

Approved rule:

```text
desktop/web UI is a client of RePlan operations and report models
```

UI may provide:

- visual workload heatmap;
- scenario comparison;
- warnings review;
- proposal review;
- evidence browser;
- dependency graph;
- calendar/resource view.

UI should not become the source of truth.

## Headless-first meaning

Headless-first means:

- all core operations work without GUI;
- all important operations have CLI/API shape;
- outputs are machine-readable;
- reports are generated artifacts;
- mutating operations support dry-run;
- actor context is explicit;
- audit is generated without UI;
- AI agents can operate safely.

Headless-first does not mean humans are ignored.

Human validation is supported through:

- Markdown reports;
- CSV exports;
- JSON-backed generated views;
- optional static HTML;
- later desktop/web UI.

## Suggested module layout

Possible module or package layout:

```text
replan-core
  pure domain model
  validation
  workload calculation
  scenario calculation

replan-app
  application services
  operations
  actor context
  policy boundary
  audit orchestration

replan-storage
  file storage
  later SQLite/database adapters

replan-adapters
  OpenProject
  GitHub
  GitLab
  YAML/CSV import/export

replan-reports
  report models
  JSON renderer
  Markdown renderer
  CSV renderer
  optional HTML renderer

replan-cli
  command-line interface

repland
  optional local daemon/service later

replan-ui
  optional desktop/web UI later
```

The exact repository/module structure can change.

The architectural rule is stable:

```text
core logic is reusable by CLI, daemon, future server and future UI
```

## AI-CLI-first implications

AI agents should be able to:

- run RePlan without a daemon;
- run RePlan in CI;
- run RePlan in a Git branch;
- inspect JSON output;
- create scenarios;
- create proposals;
- run dry-runs;
- generate reports;
- commit artifacts.

This requires:

- deterministic CLI commands;
- no hidden UI state;
- no daemon-only required state;
- stable file formats or storage adapter boundaries;
- structured errors;
- stable exit codes;
- dry-run support.

## Runtime modes

Approved runtime modes:

```text
required: CLI single-shot mode
optional later: daemon/watch mode
optional later: local API mode
optional later: desktop/web UI mode
optional future: server mode
```

### CLI single-shot mode

Command runs and exits.

This is MVP.

### Daemon/watch mode

Process stays alive to watch, cache and run background work.

This is later optional.

### Local API mode

Daemon exposes local API to desktop/web UI or tools.

This is later optional.

### Desktop/web UI mode

Human UI over RePlan operations and report models.

This is later optional.

### Server mode

Centralized deployment with API, database, external auth and background workers.

This is future optional deployment, not MVP.

## Locking and concurrency

MVP single-shot CLI can use simple file locks for mutating operations if needed.

Later daemon can coordinate locks more cleanly.

Future server can use database transactions.

The operation layer should make this transition possible:

```text
operation -> storage transaction/lock -> audit -> result
```

## MVP requirements

MVP should implement or model:

- reusable core logic;
- application service boundary;
- CLI as required runtime;
- deterministic single-shot commands;
- machine-readable JSON output;
- human-readable Markdown output;
- dry-run for risky mutations;
- actor context;
- audit records;
- file-based storage through a boundary;
- no required daemon;
- no required desktop app.

## Explicit MVP non-goals

MVP should not implement:

- desktop app;
- local daemon as required runtime;
- web UI;
- local HTTP API;
- auto-start service;
- background worker system;
- OS-specific tray application;
- realtime notifications;
- packaged desktop distribution.

## Future readiness

The design should keep these future additions possible:

- optional `repland` local daemon;
- local API over the same application services;
- desktop UI over daemon/API;
- web UI over server API;
- background imports;
- local cache/index;
- richer file locking;
- server deployment;
- external auth integration;
- multi-user proposal review.

## Approved decisions

- RePlan is headless-first.
- RePlan is not desktop-first.
- RePlan does not require a daemon/service in MVP.
- Required MVP runtime is CLI over reusable core/application services.
- Optional daemon may be added later for watching, caching, background imports,
  local API and UI support.
- Desktop/web UI is a later shell over RePlan operations and report models.
- CLI, daemon, future server and future UI must call the same core/application
  services.
- Daemon must not contain unique business logic.
- UI must not contain unique business logic.
- AI agents must be able to use RePlan without a running daemon.
