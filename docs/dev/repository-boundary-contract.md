# RePlan repository boundary contract

Date: 2026-07-20

Status: active boundary contract.

## Purpose

RePlan is incubated privately, but the intended destination is an open-source
project.

This contract defines which material belongs in the RePlan product repository
and which material must live outside it. The goal is to keep current work useful
without making a later open-source release painful.

## Repository set

RePlan work is split into three repository classes.

```text
public product repository
private pilot repository
private infrastructure repository
```

The public product repository is this repository.

The private pilot and infrastructure repositories are intentionally not named
here. Public RePlan documentation should describe their roles and integration
boundaries, not private paths, domains, hostnames or project names.

## 1. Public product repository

Purpose:

```text
Build RePlan as a reusable resource planning layer.
```

Allowed content:

- product source code;
- command-line interface code;
- storage adapters;
- report generation code;
- OpenProject adapter code;
- TaskJuggler adapter code;
- public architecture decisions;
- public design documents;
- sanitized research documents;
- synthetic examples;
- sanitized import fixtures;
- self-dogfood data for RePlan itself;
- tests;
- build scripts;
- contribution and release documentation;
- OSS readiness audit rules.

Canonical public dogfood project:

```text
examples/replan-self/
```

Allowed public examples:

- `examples/replan-self`;
- `examples/small-team`;
- synthetic OpenProject imports under `examples/imports`;
- other fixtures that use fake users, fake task ids, fake domains and fake
  operational evidence.

Forbidden content:

- real private project names;
- real private domains;
- real private hostnames;
- real VPS layout;
- real nginx configuration;
- real OpenProject project identifiers;
- real OpenProject work package exports from private projects;
- real credentials;
- credential paths;
- local tunnel commands;
- private operational logs;
- private incident notes;
- private resource assumptions tied to a real customer or private project;
- personal brand execution plans;
- screenshots or reports that expose private infrastructure.

The public product repository may describe generic integration boundaries such
as "OpenProject API token", "reverse proxy", "private pilot repository" or
"infrastructure repository". It must not include concrete private values.

## 2. Private pilot repository

Purpose:

```text
Use RePlan on real work without polluting the public product repository.
```

Allowed content:

- real RePlan project data for a private pilot;
- real imported OpenProject snapshots;
- real source references;
- real project reports;
- real actuals and evidence;
- local sidecar data;
- private project decisions;
- private actor/resource mappings;
- private run logs;
- private operator notes;
- private lessons learned before sanitization.

Expected shape:

```text
private-pilot/
  .replan/
  canonical/
  actuals/
  evidence/
  proposals/
  scenarios/
  reports/
  imports/
  notes/
```

The private pilot repository should call the RePlan binary or use RePlan as a
library/tool dependency. It should not vendor or fork product code unless there
is a deliberate experiment that will later be extracted cleanly.

Output that may return to the public product repository:

- generic bug reports;
- sanitized fixtures;
- adapter requirements;
- generic design changes;
- tests created from anonymized or synthetic data;
- documentation that removes private names, ids, domains and operational
  details.

Output that must not return directly:

- raw private exports;
- raw private reports;
- raw private evidence;
- private task descriptions;
- private project names;
- private service URLs;
- credentials or token locations.

## 3. Private infrastructure repository

Purpose:

```text
Operate services used by pilots and development.
```

Allowed content:

- VPS inventory;
- container and service layout;
- OpenProject deployment details;
- nginx configuration;
- DNS and certificate notes;
- monitoring notes;
- backup and restore runbooks;
- tunnel configuration;
- service credentials references;
- deployment-specific troubleshooting;
- private network topology.

The infrastructure repository may mention RePlan as a consumer or a service
under test. It must not become the canonical place for RePlan product design.

Output that may return to the public product repository:

- generic deployment requirements;
- generic adapter constraints;
- generic API behavior findings;
- sanitized documentation about how to connect to any OpenProject instance;
- product backlog items that do not expose private infrastructure.

## Local working rules

Use these rules during daily development:

1. Product code changes happen in the public product repository.
2. Real project operation happens in the private pilot repository.
3. VPS and OpenProject operations happen in the infrastructure repository.
4. If a note contains a private domain, host, tunnel, token path or real project
   id, it does not belong in the public product repository.
5. If a fixture is needed for tests, create a synthetic version before adding it
   to the public product repository.
6. If private validation reveals a bug, commit the generic bug fix and a
   sanitized regression test to the public product repository.
7. If private validation reveals a product idea, write it as a generic
   requirement without private facts.

## Dogfood model

The public dogfood model is:

```text
RePlan is planned with RePlan.
```

That means `examples/replan-self` is the public story for:

- project planning;
- resource demand planning;
- AI agent capacity;
- reviewer role;
- CI and token quotas;
- dashboard and report design;
- OpenProject adapter work;
- TaskJuggler integration work;
- release preparation.

Private pilots are still valuable, but they are validation inputs, not the
public narrative.

## History policy

During private incubation:

- do not rewrite history casually;
- keep cleaning the current HEAD;
- keep private material out of new commits;
- use automated searches before commits when touching docs, examples or
  fixtures.

Before an open-source release, choose one of these paths:

1. create a new clean public repository from a sanitized HEAD;
2. create a clean public branch from a sanitized HEAD;
3. rewrite history and publish the rewritten repository;
4. publish release snapshots only while keeping the incubation history private.

Preferred path:

```text
Create a clean public repository or clean public branch from sanitized HEAD.
```

Reason:

- it avoids spending time on history surgery during incubation;
- it avoids accidentally exposing old private references;
- it gives the public project a coherent build story;
- it keeps private operational learning available privately.

## Pre-release audit

Before any public release, run a manual and automated audit.

Required checks:

- no private project names in tracked files;
- no private domains in tracked files;
- no private hostnames in tracked files;
- no private local paths in tracked files;
- no credentials;
- no credential file paths;
- no raw OpenProject private exports;
- no private report artifacts;
- no private infrastructure runbooks;
- examples run locally;
- tests pass;
- README explains RePlan without private context;
- `examples/replan-self` is understandable as public dogfood;
- license exists;
- contribution guide exists;
- TaskJuggler integration status is documented;
- OpenProject adapter status is documented.

Suggested search shape:

```bash
rg -n "private-name|private-domain|private-host|private-path" README.md docs examples scripts
```

The concrete search terms should be kept outside the public repository if they
are themselves private.

## Git ignore guardrails

The public product repository should ignore common local/private working
folders:

```text
private/
private-pilot/
local-pilot/
.replan-local/
```

These patterns are guardrails only. They do not replace review discipline.

## Decision summary

- Keep `replan` as the future public product repository.
- Keep real pilot use outside `replan`.
- Keep infrastructure outside `replan`.
- Keep `examples/replan-self` as the public dogfood project.
- Move only sanitized lessons from private repositories back into `replan`.
- Defer history strategy until OSS release readiness, with clean public
  repository or clean public branch as the preferred path.
