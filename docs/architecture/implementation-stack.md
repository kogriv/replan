# Implementation stack

Date: 2026-07-06

Status: core architecture decision.

## Purpose

This document defines the recommended implementation stack for RePlan MVP.

It answers:

- whether implementation should start with Go, Rust, C, C++, Python, Node or
  desktop-first stack;
- which stack best matches local-first, headless-first and AI-CLI-first
  architecture;
- which technologies are rejected for MVP core and why;
- what constraints the stack choice creates for code organization.

## Core decision

RePlan MVP should be implemented Go-first.

Rust is the serious alternative if long-term strictness is prioritized over MVP
speed.

Approved rule:

```text
Go-first for MVP.
Rust remains a valid alternative for a stricter long-term core.
C/C++, Python core, Node core and Electron-first desktop are rejected for MVP.
```

This is a pragmatic decision, not a claim that other languages are bad.

The current bottleneck is:

```text
domain correctness + integration + storage + reports + safe mutation semantics
```

not raw CPU speed.

## Stack requirements

The implementation stack must support:

- fast single-shot CLI startup;
- simple local installation;
- one binary or near-one-binary distribution;
- local-first operation;
- no required backend server;
- deterministic CLI commands;
- machine-readable JSON output;
- human-readable Markdown output;
- CSV export;
- YAML/JSON parsing;
- file-based storage;
- optional SQLite later;
- HTTP adapters;
- Git-friendly workflows;
- structured errors;
- stable exit codes;
- dry-run operations;
- audit records;
- future optional daemon;
- future optional server/API;
- future optional desktop/web UI shell.

These requirements come from:

- AI-CLI-first design;
- local-first deployment;
- headless-first runtime;
- FOSS-first product direction;
- small-team MVP scope.

## Recommended MVP stack

Recommended MVP stack:

```text
Language: Go
Runtime shape: reusable core/application services + CLI
CLI framework: stdlib flag first, or Cobra/urfave-cli if command surface grows
Data formats: YAML + JSON
Validation: typed Go validation, later JSON Schema if useful
Storage: files first
SQLite: later optional cache/index/storage adapter
Reports: JSON, Markdown, CSV
Git integration: external git CLI first
OpenProject integration: HTTP client adapter
Daemon: later optional
Server/API: later optional
UI: later optional shell over API/report JSON
```

## Why Go-first

Go fits the MVP because it provides:

- fast CLI startup;
- easy single binary distribution;
- simple cross-compilation;
- straightforward file and HTTP tooling;
- simple concurrency for future daemon/import workers;
- strong enough type system for MVP;
- low operational friction;
- readable code for agents and humans;
- good fit for infrastructure-style CLI tools;
- clean path to optional server mode later.

Go is also fast enough for RePlan's expected MVP workload.

The MVP workload is mostly:

```text
read files/import data -> validate -> calculate workload -> generate reports
```

This does not require C/C++-level manual optimization.

## Why Rust remains a serious alternative

Rust is a strong candidate if the project chooses strictness over speed of MVP
implementation.

Rust advantages:

- strong type system;
- memory safety without garbage collector;
- good performance;
- one binary distribution;
- good fit for deterministic CLI tools;
- strong long-term correctness story.

Rust costs:

- higher implementation friction;
- slower early iteration;
- more ceremony around evolving domain model;
- harder onboarding for casual contributors;
- more time spent satisfying compiler while the model is still changing.

Rust may be attractive later for:

- critical calculation engine;
- solver component;
- library used by multiple runtimes;
- performance-sensitive module;
- strict schema/validation layer.

But for MVP, Go is the more pragmatic default.

## Why not C for MVP core

C provides:

- small binaries;
- fast startup;
- minimal runtime;
- excellent low-level control.

But RePlan does not need low-level control first.

C would increase cost in areas that matter more:

- manual memory management;
- string handling;
- JSON/YAML parsing;
- HTTP adapters;
- complex domain structures;
- tests for mutation semantics;
- safe error handling;
- rapid schema iteration.

RePlan's MVP model will change frequently.

C would make that iteration unnecessarily expensive.

## Why not C++ for MVP core

C++ is powerful, but it is not the right default for RePlan MVP.

Main costs:

- complex build ecosystem;
- dependency management friction;
- long compile times;
- many possible coding styles;
- memory and concurrency footguns;
- higher architecture complexity;
- more friction for YAML/JSON/HTTP/CLI work compared with Go;
- more difficulty keeping the codebase simple for AI agents.

C++ would be reasonable only if RePlan's core problem were a heavy native
scheduling engine or graph optimizer.

The MVP is not that.

## When C/C++ could be justified later

C or C++ may be justified if RePlan later needs:

- high-performance scheduling solver;
- native optimization engine;
- integration with existing C/C++ solver libraries;
- embedded deployment;
- extremely constrained memory;
- specialized graph algorithm at large scale.

Even then, the likely shape should be:

```text
separate native component
```

not rewriting the whole application.

TaskJuggler or other external engines can also cover advanced scheduling
without making RePlan itself C++.

## Why not Python as MVP production core

Python is useful for exploration.

It is not recommended as production core for this MVP.

Reasons:

- slower CLI startup;
- packaging and dependency friction;
- harder single-binary distribution;
- runtime environment variability;
- weaker long-term structure for a small fast tool;
- higher risk of script sprawl;
- easier for agents to bypass architecture boundaries with ad hoc scripts.

Python can still be useful for:

- research scripts;
- one-off import experiments;
- data exploration;
- prototype formulas;
- throwaway migrations.

It should not be the primary RePlan runtime.

## Why not Node/TypeScript as MVP core

Node/TypeScript is not recommended for the core CLI/runtime.

Reasons:

- heavier dependency tree;
- less natural single-binary distribution;
- weaker fit for "small fast systems tool" feel;
- more temptation to drift into UI/server-first architecture;
- more dependency churn;
- less attractive for local-first binary tooling.

TypeScript may be useful later for:

- web UI;
- desktop UI shell;
- report viewer;
- dashboard client.

It should not drive the core domain model.

## Why not Electron-first desktop

Electron-first desktop contradicts the MVP runtime decision.

It would force early work on:

- desktop packaging;
- frontend state management;
- UI rendering;
- app updates;
- operating-system integration;
- local service lifecycle.

Desktop UI should be a later shell over:

- RePlan CLI;
- local daemon API;
- report JSON;
- future server API.

It should not be the core application.

## Architecture constraints for Go implementation

Go-first must not mean unstructured Go.

Required boundaries:

```text
domain
application services
storage adapters
source adapters
calculation engine
report model/renderers
CLI
optional daemon later
```

The CLI must call application services.

The daemon must call the same application services.

Future server API must call the same application services.

Business logic must not live only in CLI handlers.

## Candidate Go package layout

Possible initial layout:

```text
cmd/
  replan/
    main.go
  repland/
    main.go          # later optional

internal/
  domain/
    task.go
    resource.go
    demand.go
    capacity.go
    scenario.go
    proposal.go
    evidence.go
  app/
    validate.go
    workload.go
    scenario.go
    report.go
    proposal.go
  storage/
    filestore/
    sqlite/          # later optional
  adapters/
    openproject/
    github/
    gitlab/
    yamlcsv/
  reports/
    model.go
    json.go
    markdown.go
    csv.go
  policy/
    actor.go
    capability.go
  audit/
    audit.go
```

The exact layout can change.

The separation principle should not.

## Dependency policy

MVP should keep dependencies modest.

Prefer:

- Go standard library where sufficient;
- small stable libraries for YAML if needed;
- explicit HTTP client code for adapters;
- simple Markdown rendering;
- simple CSV output;
- SQLite only when needed.

Avoid early:

- large frameworks;
- ORM-first design;
- web framework before server mode exists;
- heavy dependency injection framework;
- Electron;
- complex plugin system;
- code generation as core requirement.

## Performance expectations

The MVP should feel instant for small projects.

Target expectation:

```text
small project validation/report commands should complete in well under a second
when no network import is involved.
```

Network imports are allowed to be slower, but should be explicit operations.

Report generation should not require daemon startup.

## Future Rust component option

If a future calculation or solver component needs stricter guarantees or higher
performance, it can be implemented separately.

Possible integration forms:

- separate Rust binary;
- library called by Go through process boundary;
- WebAssembly module;
- replacement calculation engine behind adapter.

Do not prematurely optimize the whole MVP around this possibility.

## Approved decisions

- Go is the recommended implementation language for MVP.
- Rust is the serious alternative if strict long-term core guarantees are worth
  higher early implementation cost.
- C and C++ are rejected for MVP core.
- Python is allowed for research scripts but rejected as production core.
- Node/TypeScript is rejected as core runtime, but can be used later for UI.
- Electron-first desktop is rejected.
- RePlan should start as one fast CLI binary over reusable core/application
  services.
- Go implementation must keep domain, app services, storage, adapters, reports,
  policy, audit and CLI separated.
- Future daemon/server/UI must reuse the same application services.
