# OpenProject agent interface options

Status: dated record (2026-08-04). Index: [OpenProject design index](openproject-design-index.md).

Date: 2026-08-04

Original status: accepted evaluation for the OpenProject preflight infrastructure build.

Related task:
`task.self.evaluate-openproject-agent-interfaces`.

Depends on:
`task.self.reconstruct-openproject-functionality-and-pitfalls`.

## Purpose

This document evaluates the practical ways a human operator and AI CLI agents
can work with OpenProject during RePlan self-dogfood.

The question is not whether OpenProject is useful. OpenProject is already the
chosen human task-management surface for this phase.

The question is:

```text
Which interface should agents use so OpenProject remains the task source of
truth without turning every change into fragile manual API work?
```

## Evaluated Options

The evaluated options are:

1. OpenProject web user interface.
2. Official OpenProject MCP server.
3. Official OpenProject CLI.
4. Direct OpenProject API v3.
5. RePlan-owned guarded OpenProject helper layer.

## Decision Summary

Use a layered interface model:

| Layer | Decision | Reason |
| --- | --- | --- |
| Human task planning | OpenProject web user interface | Best current surface for tasks, statuses, dates, relations and Gantt. |
| AI read context | OpenProject API through RePlan import helpers; optional official MCP for read-only context | Agents need stable structured reads; MCP is useful, but currently not enough for writes. |
| AI write path | RePlan-owned guarded helpers | RePlan needs dry-run, explicit apply, private bindings, `lockVersion`, drift checks and audit evidence. |
| Low-level substrate | OpenProject API v3 | Required under the hood for exact work-package, relation and time-entry behavior. |
| Auxiliary manual tool | Official OpenProject CLI | Useful to inspect and maybe operate manually, but not the RePlan automation contract. |

Do not use any single external OpenProject client as the whole RePlan operating
layer.

## Option 1. OpenProject Web User Interface

Fit:

- strong for human planning;
- strong for Gantt;
- strong for status/date/relation inspection;
- useful for manual correction and acceptance;
- weak for reproducible AI-agent execution.

Strengths:

- it is the native OpenProject product experience;
- humans can understand the plan visually;
- Gantt and work-package lists make task flow easier to inspect than raw YAML;
- permissions and normal collaboration already live there.

Weaknesses:

- browser-driven agent work is hard to make deterministic;
- UI actions are difficult to audit at the RePlan resource layer;
- UI automation can accidentally depend on layout, localization or theme;
- it does not naturally produce RePlan evidence, proposals or resource
  warnings.

Decision:

Use the web interface for humans. Do not make it the primary AI mutation path.

## Option 2. Official OpenProject MCP Server

Current external finding:

OpenProject documents an MCP endpoint at `/mcp`. The documentation states that
OpenProject currently offers read-only MCP tools and that data manipulation
tools may be added later.

Reference:

- `https://www.openproject.org/docs/system-admin-guide/integrations/mcp-server/`

Fit:

- good for giving AI assistants native OpenProject context;
- good candidate for read-only project, work-package and relation inspection;
- not sufficient for the RePlan write path while it remains read-only.

Strengths:

- native agent-oriented integration surface;
- avoids screen scraping;
- can expose current OpenProject state directly to MCP-capable tools;
- authentication can follow normal OpenProject API authentication modes.

Weaknesses:

- current documented capability is read-only;
- RePlan still needs resource-sidecar state from local files;
- RePlan still needs dry-run/apply semantics and audit records;
- not all Codex-style CLI environments will automatically have this MCP
  configured;
- if future write tools appear, they still need RePlan-specific policy guards.

Decision:

Treat official OpenProject MCP as an optional read-context provider. Do not
depend on it for writes in the current dogfood cycle.

## Option 3. Official OpenProject CLI

Current external finding:

The `opf/openproject-cli` repository exists. Its README says it is unsupported
software, provides only a subset of OpenProject use cases and should be used
with care. The repository also notes that the newer v0.6 line was developed
rapidly and is not considered secure or stable enough for general usage.

Reference:

- `https://github.com/opf/openproject-cli`

Fit:

- useful auxiliary operator tool;
- useful source of command-shape ideas;
- not appropriate as RePlan's only automation foundation.

Strengths:

- already targets OpenProject API v3;
- gives humans a command-line surface;
- may cover common operations without custom `curl`;
- can be inspected for practical API usage patterns.

Weaknesses:

- unsupported;
- explicitly not full-featured;
- maturity warning makes it risky for unattended agent mutation;
- does not know RePlan resource demands, capacities, proposals or audit
  records;
- does not enforce RePlan's private binding and drift policies.

Decision:

Do not make the official CLI a required dependency. Inspect it as a reference
and optional manual tool only.

## Option 4. Direct OpenProject API v3

Fit:

- best low-level substrate;
- necessary for controlled helpers;
- risky as ad hoc agent interface.

Strengths:

- complete enough for the work-package, status, date, relation and time-entry
  operations RePlan needs;
- available in every OpenProject deployment that exposes API v3;
- lets RePlan implement exact safety checks;
- avoids dependency on an unsupported external CLI.

Weaknesses:

- raw calls require careful handling of `_links`;
- write calls require current `lockVersion`;
- schemas and allowed values are deployment-specific;
- date and relation behavior can cause side effects;
- ad hoc `curl` usage can leak private endpoints or tokens into logs and docs;
- raw API output is too noisy for normal operator decisions.

Reference:

- `https://www.openproject.org/docs/api/example/`

Decision:

Use API v3 under RePlan-owned helpers. Avoid one-off agent `curl` mutations as
normal workflow.

## Option 5. RePlan-Owned Guarded Helper Layer

Fit:

- best write path for the current dogfood loop;
- already partly implemented through existing scripts;
- can evolve into first-class CLI commands later.

Existing helper surface:

- `scripts/check-openproject-live-alignment.sh`;
- `scripts/check-openproject-drift.sh`;
- `scripts/apply-openproject-bootstrap.sh`;
- `scripts/apply-openproject-relations.sh`;
- `scripts/push-openproject-date-drift.sh`;
- `scripts/push-openproject-status-drift.sh`;
- `scripts/run-openproject-sidecar-pipeline.sh`;
- `scripts/validate-openproject-live.sh`.

Strengths:

- can load ignored private bindings;
- can keep public examples sanitized;
- can output compact JSON for AI agents;
- can default to dry-run;
- can require explicit apply gates;
- can use `lockVersion`;
- can run drift checks before or after changes;
- can encode RePlan-specific policy decisions;
- can create audit/evidence records or point to them.

Weaknesses:

- current helpers are scripts, not a polished first-class `replan openproject`
  command family;
- behavior is spread across multiple entry points;
- some operations still require manual API calls for exceptional cases;
- error messages and summaries need better operator ergonomics.

Decision:

Use this as the near-term write path. The next design task should turn this into
a coherent AI CLI operating layer contract.

## Comparison Matrix

| Capability | Web UI | MCP | Official CLI | Direct API | RePlan helpers |
| --- | --- | --- | --- | --- | --- |
| Human Gantt and task view | strong | weak | weak | weak | weak |
| AI structured read | weak | strong | medium | strong | strong |
| AI safe write | weak | not current | medium risk | high risk raw | strongest |
| Dry-run by default | no | not applicable for current read-only use | not RePlan-specific | no | yes |
| Explicit apply gate | manual | not applicable for current read-only use | not RePlan-specific | no | yes |
| `lockVersion` handling | internal | read-only | tool-dependent | manual | required by helper policy |
| Private binding awareness | no | no | no | no | yes |
| RePlan resource awareness | no | no | no | no | yes |
| Audit/evidence integration | no | no | no | no unless wrapped | yes |
| OSS-safe fixture workflow | no | no | no | no | yes |
| Good next-step foundation | human only | read context | reference only | substrate only | yes |

## Authentication And Access

Authentication must follow the existing repository privacy boundary:

- live tokens stay in environment variables or ignored private storage;
- committed examples use generic URLs and sanitized ids;
- agents should not paste live tokens into docs, reports or command logs;
- write-capable helpers should be callable only from trusted local operator
  contexts;
- broad service-to-service write access is not part of the MVP.

The official MCP server can use personal API tokens or OAuth-backed application
flows according to OpenProject documentation. RePlan should not implement its
own authentication provider for this preflight stage.

## Recommended Operating Pattern

For the next dogfood cycle, use this pattern:

1. Human opens OpenProject web UI for task list, status, dates, relations and
   Gantt.
2. Agent runs RePlan import or drift helpers for structured state.
3. Agent proposes changes through RePlan output, not through raw OpenProject
   mutation.
4. Human reviews the dry-run.
5. Agent applies only one narrow family of changes after explicit approval.
6. Agent reruns OpenProject drift check.
7. Agent reruns RePlan resource schedule drift check.
8. Agent records actuals/evidence/audit in RePlan.

## Required Design Inputs For The Next Task

Follow-up:

The documentation-based evaluation was followed by the accepted hands-on audit
in
[`openproject-agent-tools-hands-on-audit.md`](openproject-agent-tools-hands-on-audit.md).
That audit confirmed the official CLI as a usable auxiliary inspection tool,
confirmed direct API v3 as the right low-level substrate, and found that the
current live MCP endpoint is not usable without a separate server/configuration
investigation.

The next task, `task.self.design-openproject-ai-cli-operating-layer`, should
define:

- command names and grouping;
- which commands are read-only;
- which commands can write;
- exact dry-run and apply semantics;
- how private bindings are loaded and reported;
- how drift summaries are normalized;
- how destructive operations are blocked;
- how exceptional manual fixes are documented;
- how evidence and audit records are attached;
- how a future OpenProject MCP read layer can plug in without becoming the
  write authority.

## Decision

Proceed with a RePlan-owned OpenProject AI CLI operating layer.

OpenProject MCP should be supported or documented as a read-only context option
where available. The official OpenProject CLI should stay optional. Direct API
use should remain inside reviewed helpers, except for exceptional operator
maintenance tasks that are explicitly documented.
