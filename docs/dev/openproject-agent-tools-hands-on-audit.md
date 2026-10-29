# OpenProject agent tools hands-on audit

Status: dated record (2026-08-04). Index: [OpenProject design index](openproject-design-index.md).

Date: 2026-08-04

Original status: accepted hands-on audit for the OpenProject preflight infrastructure
build.

Related task:
`task.self.audit-openproject-agent-tools-hands-on`.

Depends on:
`task.self.evaluate-openproject-agent-interfaces`.

## Purpose

This document records the practical follow-up audit after the first
documentation-based evaluation of OpenProject agent interfaces.

The question was:

```text
Are there existing OpenProject agent or command-line tools that are good
enough to use directly, or that contain practical implementation patterns that
RePlan should borrow before designing its own guarded OpenProject AI CLI
operating layer?
```

The answer is:

```text
Use existing tools as references and optional read/manual helpers.
Do not make them the RePlan automation contract.
Build a narrow RePlan-owned helper layer on top of OpenProject API v3.
```

## Scope

The audit covered:

- the official OpenProject CLI repository;
- representative third-party OpenProject MCP/API client repositories found
  during the follow-up search;
- the official OpenProject CLI command surface;
- read-only execution of the official CLI against the current live
  self-dogfood OpenProject instance;
- the official OpenProject MCP endpoint as configured on the current live
  instance;
- direct OpenProject API v3 behavior needed by RePlan write-back helpers;
- comparison against the existing RePlan helper scripts.

The audit deliberately did not:

- perform destructive writes through the official CLI;
- commit live URLs, tokens or real private bindings;
- evaluate unrelated third-party OpenProject clients as product dependencies;
- replace the already approved RePlan/OpenProject ownership split.

## Sources Checked

Public sources:

- OpenProject MCP documentation:
  `https://www.openproject.org/docs/system-admin-guide/integrations/mcp-server/`;
- official OpenProject CLI repository:
  `https://github.com/opf/openproject-cli`;
- OpenProject API documentation:
  `https://www.openproject.org/docs/api/`.
- third-party OpenProject CE MCP server:
  `https://github.com/jtauschl/openproject-ce-mcp`;
- third-party Python OpenProject API client and CLI:
  `https://github.com/MHx-Operations/openproject-api-client`;
- additional third-party MCP examples:
  `https://github.com/AndyEverything/openproject-mcp-server`,
  `https://github.com/brunofin/openproject-mcp`.

Local hands-on sources:

- cloned official CLI repository at commit
  `259f293 Add strong warning about v0.6 to README`;
- live read-only OpenProject API and CLI calls using token from ignored/private
  operator storage;
- direct HTTP checks against the live MCP endpoint;
- current RePlan helper scripts in `scripts/`.

## Official OpenProject CLI Result

The official CLI exists and runs.

Observed top-level command families:

- `activity`;
- `budget`;
- `git`;
- `login`;
- `logout`;
- `notification`;
- `project`;
- `status`;
- `time-entry`;
- `type`;
- `user`;
- `whoami`;
- `work-package`.

Observed work-package commands:

- `work-package create`;
- `work-package inspect`;
- `work-package list`;
- `work-package search`;
- `work-package update`.

Observed time-entry commands:

- `time-entry create`;
- `time-entry list`.

The CLI supports `--format json` globally and can authenticate through
environment variables:

- `OP_CLI_HOST`;
- `OP_CLI_TOKEN`;
- `OP_CLI_PROFILE`.

Read-only live checks worked:

- `project list` returned the visible OpenProject projects;
- `status list` returned the current OpenProject statuses;
- `work-package list --project replan-self --total` returned a count;
- `work-package list --project replan-self` returned visible work packages;
- `work-package inspect` returned a single work-package summary;
- `time-entry list` returned visible time entries.

This means the official CLI is real and useful as an auxiliary inspection tool.
It is not only a theoretical option from documentation.

## Official CLI Limitations For RePlan

The official CLI is not the right RePlan automation contract.

Reasons:

- the README marks the current v0.6 line as rapidly developed, internal-use
  oriented and not secure or stable enough for general usage;
- the README also says the CLI is unsupported and only covers a subset of
  OpenProject use cases;
- `work-package list` and `work-package inspect` JSON output is intentionally
  simplified and does not expose the full raw OpenProject HAL object;
- ordinary JSON output does not include enough data for RePlan drift decisions,
  such as full `_links`, date scheduling side effects, relation data and
  `lockVersion`;
- no relation command was found in the current official CLI source;
- write commands mutate OpenProject directly and do not provide RePlan's
  required dry-run/proposal/audit flow;
- the CLI does not know RePlan resource demands, capacities, scenarios,
  proposals, actuals or private binding files.

Therefore RePlan should not shell out to the official CLI as its primary
OpenProject integration layer.

## Useful Patterns To Borrow From The Official CLI

The official CLI still contains useful practical patterns.

Borrow these ideas:

- noun-first command naming:
  `openproject project ...`, `openproject work-package ...`,
  `openproject status ...`, `openproject time-entry ...`;
- global JSON mode;
- environment-variable authentication and profile selection;
- explicit discovery commands for projects, statuses, types, users and time
  entries;
- fetch-before-patch write flow;
- optimistic-locking awareness through `lockVersion`;
- preserving `lockVersion: 0` in PATCH payloads;
- resolving allowed values through OpenProject links/forms instead of guessing
  ids where possible;
- small focused commands instead of a single broad sync command.

The `lockVersion: 0` detail is important. The official CLI has a test stating
that a freshly created work package can have `lockVersion` zero and that
omitting it causes OpenProject update conflicts. RePlan write helpers must keep
zero values in JSON payloads.

## Patterns Not To Borrow

Do not borrow these behaviors:

- direct write commands without dry-run by default;
- output shaped only for humans or broad CLI convenience;
- reliance on simplified JSON where RePlan needs source-of-truth fields;
- implicit mutation without RePlan audit evidence;
- broad command surface before the preflight use cases are proven;
- storing or printing private connection details in committed artifacts.

## Third-Party Tool Result

The third-party ecosystem is more useful than the first documentation-only pass
showed.

The most relevant project found during the hands-on pass is
`jtauschl/openproject-ce-mcp`.

It is a local stdio MCP server for OpenProject Community Edition. Its design is
close to several RePlan requirements:

- typed tools instead of a raw REST proxy;
- compact agent-shaped responses instead of raw HAL payloads;
- project read and write allowlists that fail closed;
- per-tool-group read/write exposure flags;
- preview-then-confirm writes;
- OpenProject form endpoint usage for validation previews;
- field hiding for sensitive fields;
- prompt-injection warning around user-authored OpenProject content;
- retry/backoff handling for transient HTTP failures;
- tool-output trimming to reduce context usage;
- tests around confirmation, project scoping, relation handling and live
  integration behavior;
- Codex, Claude Code and other MCP client setup documentation.

This is useful prior art.

It does not replace RePlan's own OpenProject layer because it still does not
own:

- RePlan resource demands;
- RePlan capacities and quotas;
- RePlan scenarios;
- RePlan actuals and evidence;
- RePlan proposals;
- RePlan audit records;
- private OpenProject-to-RePlan binding files;
- OpenProject/RePlan drift semantics;
- the rule that OpenProject owns tasks while RePlan owns resource planning.

Decision:

Use `openproject-ce-mcp` as a reference architecture for the next design task,
especially for:

- fail-closed project scoping;
- preview/confirm write semantics;
- per-tool read/write exposure;
- compact response shaping;
- form endpoint validation;
- prompt-injection boundaries for OpenProject-authored text;
- retry/backoff policy;
- client setup ergonomics for Codex and Claude Code.

Do not import it as a dependency for the preflight helper slice. A dependency
decision can be revisited after the RePlan-owned helper contract is designed.

The `MHx-Operations/openproject-api-client` project is useful as a broad Python
API coverage reference. It covers work packages, relations and time entries and
has explicit `lock_version` parameters. It is less aligned with RePlan safety
requirements because it exposes broad create/update/delete commands without a
RePlan-specific dry-run/proposal/audit model.

The other MCP examples found are useful as endpoint coverage examples, but some
expose destructive tools directly or use older/narrower endpoint shapes. They
should be mined cautiously for API examples, not adopted as the operating
surface.

## Official MCP Result

OpenProject now documents a native MCP server and describes the current tools
as read-only.

That makes MCP strategically relevant for future agent read context.

However, the current live instance did not provide a usable MCP endpoint during
this audit:

- authenticated `GET /mcp` returned HTTP 500;
- authenticated JSON-RPC `initialize` POST to `/mcp` returned HTTP 404;
- normal API v3 access to the same instance worked.

Interpretation:

```text
MCP is not blocked by authentication alone; the current OpenProject instance or
reverse-proxy/application configuration is not ready for MCP use as-is.
```

Decision:

Do not block the OpenProject AI CLI operating-layer design on MCP.

Treat MCP as an optional future read-context provider that needs a separate
server/configuration investigation before it becomes part of the operator loop.

## Direct API Result

Direct OpenProject API v3 access remains the most reliable substrate for RePlan
helpers.

Confirmed properties:

- API v3 is reachable with token-based Basic authentication using username
  `apikey`;
- work packages can be fetched by numeric id;
- the full API response includes data that simplified CLI output hides;
- write helpers can read `lockVersion` immediately before PATCH;
- RePlan can keep private live ids in ignored bindings while committed examples
  use sanitized ids and generic URLs.

Direct API should not mean ad hoc agent `curl` mutations.

It should mean a RePlan-owned helper layer with:

- private binding loading;
- compact machine-readable output;
- dry-run by default;
- explicit apply gate;
- current-state preflight checks;
- `lockVersion` reads immediately before PATCH;
- allowed-transition checks where OpenProject workflow rules apply;
- audit and evidence records after accepted changes;
- drift checks after apply.

## Comparison With Current RePlan Helpers

Current helper scripts already cover the right direction:

- `scripts/check-openproject-drift.sh` checks task/date/status/relation drift;
- `scripts/apply-openproject-relations.sh` creates reviewed relations with
  explicit apply;
- `scripts/push-openproject-date-drift.sh` pushes date drift through reviewed
  actions;
- `scripts/push-openproject-status-drift.sh` pushes status drift through
  reviewed actions and checks allowed transitions;
- `scripts/run-openproject-sidecar-pipeline.sh` keeps resource sidecars local
  to RePlan;
- `scripts/validate-openproject-live.sh` validates live import behavior.

The weakness is ergonomics, not direction.

The scripts are useful but still feel like internal build tools. The next
design step should decide whether to keep them as scripts for the preflight
phase or wrap them in a stable `replan openproject ...` CLI surface.

## Design Inputs For The Next Task

The next task,
`task.self.design-openproject-ai-cli-operating-layer`, should use these
decisions:

1. OpenProject web UI remains the human planning and Gantt surface.
2. RePlan helpers remain the only approved write path for agents.
3. Official CLI is optional for inspection and useful as a design reference.
4. Third-party `openproject-ce-mcp` is a strong reference for guarded
   agent-facing design, especially fail-closed scopes, preview/confirm writes
   and compact output, but it is not adopted as the RePlan dependency yet.
5. MCP is optional read context only until a separate live configuration
   investigation proves it usable.
6. The minimum helper layer should cover project inspection, mapped/unmapped
   work-package listing, drift preview, relation preview/apply, date
   preview/apply, status preview/apply and evidence-friendly output.
7. Helper output should be stable JSON first, with human text output optional.
8. Every write helper must keep dry-run as the default mode.
9. Every apply mode must have an explicit gate and leave enough evidence for
   RePlan tracking.
10. The helper layer must not delete and recreate work packages during ordinary
   plan refactoring.
11. `lockVersion`, allowed transitions and OpenProject scheduling side effects
    are first-class design requirements.

## Backlog Decision

This hands-on audit closes the audit gate before the OpenProject AI CLI
operating-layer design.

Do next:

```text
task.self.design-openproject-ai-cli-operating-layer
```

Do not start the full live OpenProject plan refactor until the design and
minimum helper implementation are complete.
