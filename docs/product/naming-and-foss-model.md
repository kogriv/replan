# RePlan naming and FOSS model

Date: 2026-07-02

Status: working decision.

## Name decision

Use **RePlan** as the working product name.

Use `replan` as the CLI and package name where available.

## Why RePlan works

The name has two useful meanings.

Russian mnemonic:

```text
REsursnoe PLANirovanie
```

Meaning:

```text
resource planning
```

English meaning:

```text
re-plan
```

Meaning:

```text
plan again when reality changes
```

This is a good match because the product is not just about drawing an initial
plan. The product is about continuously adapting a plan when:

- resource availability changes;
- estimates change;
- deadlines change;
- dependencies change;
- AI agents enter or leave the workflow;
- work becomes blocked;
- priorities change.

## Known naming risk

`Replan` is not globally unique.

During quick research, the name was found in other contexts:

- commercial supply planning / production scheduling;
- construction / design companies;
- AI and robotics research projects;
- general use as a verb in planning systems.

This creates some brand and search risk.

Current decision:

- accept this risk for the internal and early OSS phase;
- keep the name because it is short and accurate;
- revisit public trademark/domain strategy only if the project becomes visible
  enough for that to matter.

The pragmatic reasoning is that the project first needs to become useful. A
perfect brand before product usefulness is premature optimization.

## Package availability checked

Quick package registry check on 2026-07-02:

- `npm view replan` returned `404 Not Found`;
- `npm view replanner` returned `404 Not Found`;
- `python3 -m pip index versions replan` returned no matching distribution;
- `python3 -m pip index versions replanner` returned no matching distribution.

This does not guarantee permanent availability, but it is enough for the current
working decision.

## Possible public variants

If plain RePlan becomes inconvenient later, possible variants:

- OpenRePlan;
- RePlan Ops;
- RePlan Core;
- RePlan Resource;
- RePlanner.

Do not switch yet.

## FOSS stance

The project should be FOSS-first.

This is not only ideological. It is central to the product position.

The product exists because the useful resource-management features in existing
systems are either absent, weak, expensive, or locked behind enterprise plans.
If RePlan repeats the same pattern by paywalling the core planning function, it
will lose the main reason to exist.

Therefore the following must remain open:

- task/resource data model;
- CLI;
- local storage;
- resource calendars;
- capacity calculation;
- workload calculation;
- overload detection;
- baseline comparison;
- scenario creation;
- scenario comparison;
- TaskJuggler export;
- TaskJuggler result import if implemented;
- OpenProject importer;
- YAML/CSV fixtures;
- markdown reports;
- CSV reports;
- JSON snapshot export;
- basic local or self-hosted UI if implemented;
- agent-safe read/propose/dry-run commands.

In short:

```text
the ability to plan resources must be FOSS
```

## What should not happen

Avoid this model:

```text
free version:
    import a few tasks
    see a teaser

paid version:
    actually calculate resources
    compare scenarios
    use baselines
    use integrations
```

That would make RePlan morally and strategically similar to the systems it is
trying to avoid.

## Monetization principle

Charge for operational value around the FOSS product, not for the core planning
algorithm.

Better:

```text
core planning is free
running it reliably for teams costs money
```

Worse:

```text
planning works only after payment
```

## Possible monetization models

### Managed hosting

Offer hosted RePlan for teams that do not want to run it.

Paid value:

- uptime;
- backups;
- upgrades;
- SSL;
- email notifications;
- hosted database;
- connector credentials management;
- team onboarding.

This is compatible with FOSS because the user can still self-host the same core
product.

### Support subscription

Offer paid support for self-hosted users.

Paid value:

- installation help;
- upgrade help;
- bug triage;
- security guidance;
- priority fixes;
- planning-model consulting.

### Enterprise assurance

Offer enterprise-grade packaging and assurance.

Paid value:

- hardened container images;
- signed builds;
- SBOM;
- vulnerability response;
- long-term support releases;
- deployment runbooks;
- monitored reference deployments.

### Commercial license

Use a copyleft license for the core, likely AGPLv3, and offer a commercial
license to companies that want to embed RePlan in proprietary products without
AGPL obligations.

This keeps the community product open while giving companies a clean legal path
to pay.

### Custom integrations

Charge for integrations that are specific to one customer:

- internal HR system;
- accounting system;
- ERP;
- private Jira workflow;
- custom OpenProject fields;
- internal approval system;
- custom MCP gateway;
- specialized agent policy engine.

The generic connector framework should remain open. Customer-specific glue can
be paid services.

### Migration services

Charge for migration from:

- MS Project;
- spreadsheets;
- Jira Advanced Roadmaps exports;
- OpenProject Enterprise experiments;
- ProjectLibre files;
- GanttProject files;
- hand-maintained YAML/markdown plans.

### Training and workshops

Charge for helping teams adopt resource planning discipline:

- capacity modeling;
- estimation hygiene;
- dependency modeling;
- scenario review;
- agent assignment policy;
- weekly planning rituals.

### Hosted agent gateway

Later, charge for a managed agent-control layer:

- API keys;
- quotas;
- policy rules;
- audit log;
- approval queues;
- MCP server;
- per-agent permissions;
- evidence requirements.

The core commands should remain open. The hosted, audited, multi-tenant
operation can be paid.

## Features that may be paid without betraying the FOSS position

These are acceptable paid areas:

- hosted service;
- enterprise SSO;
- managed backups;
- audit retention beyond local logs;
- compliance reports;
- commercial support;
- hardened releases;
- custom connectors;
- migration projects;
- private cloud deployment;
- high-availability setup;
- enterprise policy packs;
- consulting.

These are risky to paywall:

- workload calculation;
- overload detection;
- scenarios;
- baseline comparison;
- TaskJuggler integration;
- OpenProject import;
- basic write-back;
- CLI/API;
- local self-hosting.

## License recommendation

Initial recommendation:

- AGPLv3 for server, CLI and planning engine;
- Apache-2.0 or MIT for SDKs, generated examples and small client libraries;
- commercial license available later if needed.

Why AGPLv3:

- discourages closed hosted clones;
- preserves user freedom in SaaS contexts;
- makes dual licensing possible;
- aligns with the product's FOSS-first claim.

Why not pure MIT/Apache for everything:

- easier adoption, but much easier for a company to resell a closed hosted
  version without contributing back;
- weaker long-term business protection.

Why not proprietary core:

- contradicts the reason for building RePlan.

## Business expectation

Do not assume that FOSS automatically creates revenue.

The realistic path is:

1. Build useful internal tool.
2. Publish OSS only when it is coherent enough.
3. Attract users with a working CLI and examples.
4. Learn which integrations and operational pains are real.
5. Monetize support, hosting and custom work only after repeated external
   demand appears.

Until then, the project is primarily a tool to solve the operator's own resource
planning problem.
