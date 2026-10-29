# OpenProject AI CLI operating layer design

Status: current. Index: [OpenProject design index](openproject-design-index.md).

Date: 2026-08-04

Original status: accepted design for the OpenProject preflight infrastructure build.

Related task:
`task.self.design-openproject-ai-cli-operating-layer`.

Depends on:

- `task.self.evaluate-openproject-agent-interfaces`;
- `task.self.audit-openproject-agent-tools-hands-on`.

## Purpose

This document designs the narrow RePlan-owned operating layer that lets humans
and AI CLI agents inspect and safely mutate an OpenProject-backed RePlan
project.

The goal is not to build a general OpenProject replacement.

The goal is:

```text
Make OpenProject comfortable enough for humans and AI CLI agents that RePlan
can dogfood an OpenProject-first planning loop without raw API calls,
browser automation or delete-and-recreate shortcuts.
```

## Operating Split

The ownership split remains unchanged.

OpenProject owns:

- work-package identity;
- work-package subject and description;
- human-visible status;
- human-visible start and due dates;
- assignee;
- relations;
- comments;
- time entries when users record them there;
- Gantt and normal project-management views.

RePlan owns:

- resource identities that are not native OpenProject users;
- AI-agent resources;
- reviewer roles;
- service quotas, subscription limits, CI and environment resources;
- resource demands;
- capacities and availability;
- workload and overload analysis;
- scenarios;
- actuals accepted into RePlan;
- evidence;
- proposals;
- audit records;
- private OpenProject binding metadata;
- AI-safe planning and write-back workflow.

This operating layer sits between them. It makes the boundary explicit for
agents.

## Design Decision

Build a RePlan-owned OpenProject helper layer.

Near-term implementation may stay as scripts, but the product contract should
be shaped as a future first-class CLI family:

```text
replan openproject ...
```

The scripts can remain implementation details while the stable semantics are
documented and tested.

Do not make the official OpenProject CLI, official MCP endpoint or any
third-party MCP server the required RePlan automation dependency for this
preflight slice.

Use them as references:

- official OpenProject CLI for noun-first command shape, profile/env auth and
  fetch-before-patch `lockVersion` behavior;
- `openproject-ce-mcp` for fail-closed scopes, preview/confirm, compact output,
  field hiding, form validation, retry/backoff and prompt-injection boundaries;
- OpenProject API v3 as the actual low-level substrate.

## Command Families

The future command family should be noun-first and explicit.

Minimum read commands:

```text
replan openproject project inspect
replan openproject work-package list
replan openproject work-package inspect
replan openproject binding list
replan openproject drift check
replan openproject relation list
replan openproject status list
replan openproject type list
replan openproject time-entry list
```

Minimum preview/apply commands:

```text
replan openproject work-package ensure
replan openproject work-package update-status
replan openproject work-package update-dates
replan openproject relation ensure
replan openproject comment add
```

Minimum local-side acceptance commands may remain existing RePlan commands:

```text
replan import openproject
replan import accept
replan actual add
replan actual close-demand
replan proposal ...
```

The next implementation slice does not need to implement every future command
name as a Go subcommand. It may wrap or improve existing scripts if that is the
fastest route. The required behavior matters more than the final command
packaging in the preflight phase.

## Current Script Mapping

The current helper scripts map to the design like this:

| Intended command | Current helper | Status |
| --- | --- | --- |
| `openproject drift check` | `scripts/check-openproject-drift.sh` | Existing and used. |
| `openproject work-package ensure` | `scripts/apply-openproject-bootstrap.sh` | Existing, but should stay bootstrap-only. |
| `openproject relation ensure` | `scripts/apply-openproject-relations.sh` | Existing and used. |
| `openproject work-package update-dates` | `scripts/push-openproject-date-drift.sh` | Existing and used. |
| `openproject work-package update-status` | `scripts/push-openproject-status-drift.sh` | Existing and used. |
| `openproject project inspect` | `scripts/openproject-operator-report.sh` | Implemented as read-only preflight report. |
| `openproject work-package list` | `scripts/openproject-operator-report.sh` | Implemented with mapped/missing/extra task sections. |
| `openproject binding list` | `scripts/openproject-operator-report.sh` | Implemented with private binding load and binding counts. |

The first implementation slice added `scripts/openproject-operator-report.sh`
as the coherent read-only preflight surface. It does not replace the stricter
write gates; it gives humans and AI CLI agents the project, binding, task,
relation and resource context needed before selecting one narrow dry-run/apply
helper.

## Read Model

Read commands should be easy and safe.

Rules:

- read commands never mutate OpenProject;
- read commands never mutate RePlan canonical state unless the command name
  explicitly says `accept`, `apply`, `record` or another mutation verb;
- read commands may load private bindings;
- read commands must report whether private bindings were loaded;
- read commands must not print tokens;
- read commands should avoid printing private live endpoints in committed
  artifacts;
- read output should be stable JSON by default for AI CLI agents;
- optional text output may be added for humans.

Read outputs should use compact RePlan-owned envelopes.

Example shape:

```json
{
  "ok": true,
  "command": "openproject work-package list",
  "mode": "read_only",
  "project_path": "examples/replan-self",
  "private_binding_layer": {
    "loaded": true,
    "task_bindings": 32,
    "dependency_bindings": 37
  },
  "counts": {
    "mapped": 32,
    "unmapped_local": 0,
    "unmapped_openproject": 0
  },
  "items": []
}
```

## Write Model

Every write command has two stages:

```text
dry-run/preview -> explicit apply
```

Rules:

- dry-run is the default;
- apply requires an explicit gate such as `--apply` or `REPLAN_APPLY=1`;
- apply must be impossible when hard drift is present;
- apply must be impossible when the private binding file required for identity
  resolution is missing;
- apply must be impossible when the command would delete or recreate an
  existing work package during ordinary refactor;
- apply must read the current OpenProject object immediately before mutation;
- apply must include the current `lockVersion` in work-package PATCH payloads,
  including `lockVersion: 0`;
- apply must check allowed workflow transitions where OpenProject exposes them;
- apply must report the exact action performed;
- apply must tell the operator which verification command to run next.

The write path must never be:

```text
agent invents curl -> agent patches OpenProject -> maybe checks later
```

It must be:

```text
agent runs helper dry-run -> human or trusted operator reviews -> agent runs
helper apply -> helper reports result -> drift check -> RePlan evidence/audit
```

## Mutation Classes

### Safe Near-Term Mutations

These are allowed in the preflight layer after dry-run review:

- create a missing work package during controlled bootstrap;
- update status when status mapping is explicit;
- update start and due dates when date owner is explicit;
- create missing relations when normalized dependency mapping is explicit;
- add an OpenProject comment that links a RePlan report or audit result.

### Review-Only Mutations

These may be shown but not auto-applied by default:

- changing dates on accepted or closed tasks;
- changing titles on existing work packages;
- changing descriptions that contain user-authored planning narrative;
- changing assignee;
- changing estimates;
- changing relation direction when a related relation already exists;
- accepting OpenProject schedule shifts back into RePlan.

### Out-Of-Scope Mutations

These are not part of the preflight helper implementation:

- deleting OpenProject projects;
- deleting OpenProject work packages;
- deleting OpenProject relations as a cleanup shortcut;
- bulk rewrite of all work packages;
- role, permission or workflow administration;
- custom field administration;
- OpenProject plugin installation;
- broad bidirectional sync.

Destructive operations require a separate task, separate design and a stronger
approval gate.

## Private Binding Contract

Committed examples use sanitized source references:

```text
https://op.example.internal/work_packages/2101
```

Live ids and private endpoints belong only in ignored private storage:

```text
private/openproject/replan-self-bindings.yaml
```

The helper layer must treat private bindings as identity state, not as a
temporary convenience.

Rules:

- a bound RePlan task must map to exactly one live OpenProject work package;
- a bound RePlan dependency must map to exactly one live OpenProject relation
  when relation bindings exist;
- a missing binding is a hard drift for write commands;
- an extra live work package is not automatically deleted;
- title matching is allowed only as bootstrap assistance;
- public fixtures may be regenerated only through a sanitization step.

## Project Scope And Access

Borrow the fail-closed idea from `openproject-ce-mcp`.

Read scope:

- project-scoped reads require an explicit project id/profile;
- the default project may come from the RePlan source profile;
- broad all-project reads are not needed for the preflight slice.

Write scope:

- writes require an explicit project id/profile;
- writes should refuse to run when the target project is ambiguous;
- writes should refuse to run if the OpenProject project in the private binding
  file does not match the requested project;
- future daemon/MCP modes should expose separate read and write scopes.

The current script implementation may use:

```text
REPLAN_OPENPROJECT_BASE_URL
REPLAN_OPENPROJECT_PROJECT_ID
REPLAN_OPENPROJECT_TOKEN_ENV
OPENPROJECT_TOKEN
REPLAN_OPENPROJECT_BINDINGS_PATH
REPLAN_APPLY
```

The eventual CLI should provide equivalent explicit flags while keeping env
support for automation.

## Output Contract

Every command should return a stable JSON envelope.

Required top-level fields:

- `ok`;
- `command`;
- `mode`;
- `project_path`;
- `target_openproject_project`;
- `private_binding_layer`;
- `counts`;
- `actions` or action-specific arrays;
- `warning_codes`;
- `error_codes`;
- `suggested_operator_decision`;
- `suggested_next_actions`.

Write preview outputs should include:

- action type;
- RePlan task id;
- OpenProject work-package id if bound;
- current OpenProject value;
- intended value;
- reason;
- blocking condition if any;
- whether the action is planned, review-only or blocked.

Apply outputs should include:

- action type;
- OpenProject object id;
- changed fields;
- updated `lockVersion` when available;
- status of the operation;
- verification next step.

## Drift Classes

Use these drift classes consistently:

| Drift class | Meaning | Apply behavior |
| --- | --- | --- |
| hard | Identity, missing object, ambiguous binding, relation tuple mismatch or invalid date shape. | Blocks apply. |
| soft | Status/date differences where one side may be accepted after review. | Can be previewed; apply only through a specific helper. |
| review | Historical or high-impact changes such as accepted-task dates. | Shown but not applied by default. |
| informational | Counts, skipped optional fields, unavailable MCP, or missing non-required data. | Does not block. |

The command output should avoid vague "sync needed" language. It should say
which owner is expected to move next:

- accept OpenProject state into RePlan;
- push reviewed RePlan state to OpenProject;
- edit OpenProject manually;
- edit RePlan sidecar data;
- create a new backlog task.

## Evidence And Audit

OpenProject write-back and RePlan local acceptance must leave traceable
evidence.

Minimum preflight rule:

- every completed RePlan task gets evidence;
- every consumed resource demand gets an actual record;
- every RePlan mutation gets an audit record;
- every OpenProject apply helper returns enough structured output to be copied
  or referenced as evidence;
- live private response dumps stay outside committed source.

Do not commit live OpenProject payloads that include private ids, endpoints,
user data or project narrative.

## Prompt Injection And User-Authored Text

OpenProject contains user-authored text:

- work-package descriptions;
- comments;
- news/wiki/document content if read later;
- attachment names;
- custom fields.

Agents must treat that content as data, not instructions.

Near-term command output should:

- prefer summaries and ids in list views;
- include full descriptions only in explicit inspect commands;
- mark user-authored text fields in JSON so a caller can distinguish source
  content from helper instructions;
- avoid letting imported OpenProject text define shell commands, file paths or
  apply decisions.

This mirrors the prompt-injection boundary found in `openproject-ce-mcp`, but
keeps the policy inside RePlan's own helper contract.

## MCP Position

MCP is useful strategically, but not a blocker for the next slice.

Current decision:

- official OpenProject MCP is optional read context only;
- the current live OpenProject MCP endpoint is not usable as-is and needs a
  separate server/configuration investigation;
- third-party MCP servers are references, not required dependencies;
- if RePlan later exposes its own MCP, it should wrap RePlan commands and
  policies, not expose raw OpenProject writes.

## Minimal Implementation Slice

The next task,
`task.self.implement-openproject-preflight-operator-helpers`, should implement
or normalize only this minimum:

1. A coherent read/inspect command or script output for:
   - project profile;
   - private binding load result;
   - mapped local tasks;
   - missing live tasks;
   - extra live work packages;
   - status/date/relation drift summary.
2. Stable dry-run JSON for:
   - date push;
   - status push;
   - relation ensure;
   - bootstrap work-package ensure.
3. Explicit apply gate for every write helper.
4. `lockVersion` handling tested or covered by smoke for status/date writes.
5. Relation direction output that shows predecessor, successor, OpenProject
   `from` and OpenProject `to`.
6. No delete/recreate helper in the normal path.
7. Updated smoke or focused tests for the helper contract.
8. Documentation for the operator command sequence.

This is enough before the live `replan-self` plan refactor.

## Full Operator Sequence After Implementation

Expected daily or per-session flow:

```text
1. Human opens OpenProject and reviews task/Gantt state.
2. Agent runs OpenProject/RePlan drift check.
3. Agent runs RePlan validation and resource schedule drift check.
4. Agent reports resource or sync problems.
5. Agent prepares one narrow dry-run mutation if needed.
6. Human/operator reviews the dry-run.
7. Agent applies only that reviewed mutation.
8. Agent reruns drift checks.
9. Agent records RePlan evidence, actuals and audit if the work item completed.
```

This keeps OpenProject primary for task work and RePlan primary for resource
planning.

## Acceptance Criteria

The design is accepted when:

- the operating split is explicit;
- the command families are named;
- read and write boundaries are separated;
- dry-run/apply behavior is specified;
- private binding behavior is specified;
- destructive operations are excluded from the normal path;
- `lockVersion` and OpenProject form/allowed-transition behavior are specified;
- drift classes are specified;
- evidence/audit expectations are specified;
- the next implementation slice is bounded.

This document satisfies those criteria.
