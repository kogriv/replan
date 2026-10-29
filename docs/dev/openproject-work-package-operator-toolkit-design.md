# OpenProject work-package operator toolkit design

Status: current. Index: [OpenProject design index](openproject-design-index.md).

Date: 2026-08-04

Original status: accepted design for the OpenProject work-package manipulation toolkit.

Related task:
`task.self.design-openproject-work-package-operator-toolkit`.

Implementation status:

- transitional script:
  [`../../scripts/openproject-work-package-toolkit.sh`](../../scripts/openproject-work-package-toolkit.sh);
- implemented implementation task:
  `task.self.implement-openproject-work-package-operator-toolkit`;
- future first-class CLI target remains `replan openproject ...`.

Live OpenProject HTTP behavior is shared with the other guarded OpenProject
operator helpers:

- `REPLAN_OPENPROJECT_TIMEOUT_SECONDS` controls the per-request timeout.
  Default: `60`.
- `REPLAN_OPENPROJECT_RETRIES` controls transient retry count for Python
  helpers. Default: `0`.
- `REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS` controls retry delay. Default: `2`.

The JSON output includes the resolved `openproject_request` timing block so an
AI CLI agent can distinguish a real data mismatch from a slow OpenProject
request. Mutation commands still remain dry-run by default.

## Purpose

The existing OpenProject/RePlan helpers are enough to check alignment and to
perform several narrow write-backs, but they are not enough for comfortable
day-to-day manipulation of OpenProject work packages by AI CLI agents.

The gap is practical:

- humans can use the OpenProject web interface;
- RePlan can inspect resource state;
- agents can run `openproject-operator-report.sh`;
- but agents still lack a consistent command surface for common OpenProject
  operations such as inspecting one work package, checking allowed status
  transitions, editing metadata, adding a comment or updating a description.

Without this toolkit, live OpenProject work tends to fall back to raw API calls,
manual browser reconstruction or delete-and-recreate shortcuts. That is exactly
the behavior the dogfood loop is meant to avoid.

## Decision

Build a RePlan-owned OpenProject work-package operator toolkit on top of
OpenProject API v3.

The toolkit should borrow patterns from existing tools, but should not depend
on them as the runtime source of truth for RePlan dogfood work.

Use:

- OpenProject API v3 as the low-level protocol;
- RePlan private bindings as the identity layer;
- RePlan JSON envelopes as the agent-facing contract;
- the existing helper scripts as transitional wrappers where useful.

Do not make raw `curl`, browser automation, the official OpenProject CLI or a
third-party MCP server the required operator path.

## What We Reuse From Existing Tools

### Official OpenProject CLI

Useful patterns:

- noun-first command shape;
- profile or environment based authentication;
- fetch current object before `PATCH`;
- include `lockVersion` in work-package updates;
- keep command output predictable;
- use OpenProject names such as work package, project, status and type.

Limitations for RePlan:

- not RePlan-aware;
- does not know RePlan resource demands, capacities or proposals;
- does not know the private binding layer;
- does not implement RePlan drift policy;
- does not provide the exact dry-run/apply workflow needed for agents.

Conclusion:

Use it as command-design and API-behavior reference, not as the primary
automation dependency.

### OpenProject API v3

Use directly for the first implementation.

Required API families:

- projects;
- work packages;
- work-package forms;
- statuses;
- types;
- users and memberships;
- relations;
- attachments/comments/journals only as needed for operator comments.

Important behavior:

- work-package updates require the current `lockVersion`;
- allowed field values and workflow transitions should be checked through forms
  before applying high-risk updates;
- errors must be reported as structured diagnostics, not hidden behind generic
  shell failures;
- create/update operations must be project-scoped.

### OpenProject MCP

Current position:

- useful strategically;
- not usable as the required path on the current live instance;
- should stay optional read context until separately installed and verified.

The toolkit should be designed so that a future RePlan MCP server can wrap it.

### Representative Third-Party MCP Clients

Useful patterns from `openproject-ce-mcp` and similar clients:

- fail closed by project scope;
- preview before writes;
- explicit confirmation or apply gates;
- compact output for agent context;
- hidden token and endpoint handling;
- form validation before writes;
- retry/backoff for transient OpenProject responses;
- prompt-injection boundary around user-authored OpenProject text.

Limitations:

- generic MCP tools do not understand RePlan canonical data;
- their write policy is not the RePlan/OpenProject dogfood policy;
- adding a server dependency increases operational complexity before it buys us
  enough value.

Conclusion:

Copy patterns, not the dependency.

## Product Shape

The future first-class product interface should be:

```text
replan openproject ...
```

The near-term implementation may still use scripts if that is faster, but the
scripts must follow this command contract so they can later be moved into the
Go CLI without changing operator behavior.

## Command Set

### Project And Binding Reads

```text
replan openproject project inspect
replan openproject binding list
replan openproject drift check
replan openproject operator report
```

Purpose:

- confirm target project;
- confirm profile/env/token shape without printing secrets;
- confirm private bindings loaded;
- show whether task and relation identity is safe;
- summarize drift before any mutation.

Current implementation:

- `scripts/openproject-operator-report.sh`;
- `scripts/check-openproject-drift.sh`.

Transitional toolkit implementation:

- `scripts/openproject-work-package-toolkit.sh work-package list`.

### Work-Package Reads

```text
replan openproject work-package list
replan openproject work-package inspect
replan openproject work-package form inspect
```

Required behavior:

- list mapped and unmapped work packages;
- show RePlan task id and OpenProject work-package id together;
- show status, dates, assignee, type, estimate and spent time;
- hide full descriptions in list output by default;
- show full user-authored text only in explicit inspect commands;
- mark user-authored text as data in JSON output;
- show current `lockVersion`;
- show available form fields and allowed values where OpenProject exposes them.

Transitional toolkit implementation:

- `scripts/openproject-work-package-toolkit.sh work-package list`;
- `scripts/openproject-work-package-toolkit.sh work-package inspect --task <replan-task-id>`;
- `scripts/openproject-work-package-toolkit.sh work-package inspect --work-package <openproject-work-package-id>`.

The list command omits long user-authored description text. The inspect command
includes user-authored subject and description under `user_authored` so agents
can treat them as source data, not instructions.

### Catalog Reads

```text
replan openproject status list
replan openproject type list
replan openproject user list
replan openproject priority list
```

Purpose:

- avoid guessing OpenProject ids or hrefs;
- let agents select values by stable names and receive exact hrefs;
- make workflow/debug output explainable to humans.

Transitional toolkit implementation:

- `scripts/openproject-work-package-toolkit.sh status list`;
- `scripts/openproject-work-package-toolkit.sh type list`;
- `scripts/openproject-work-package-toolkit.sh user list`;
- `scripts/openproject-work-package-toolkit.sh priority list`.

### Relation Reads And Writes

```text
replan openproject relation list
replan openproject relation ensure
```

Required behavior:

- show RePlan predecessor and successor task ids;
- show OpenProject `from` and `to` work-package ids;
- show OpenProject relation type and RePlan dependency type;
- preview missing relation creation;
- require explicit apply for relation creation;
- append relation ids to private bindings after apply;
- never delete relations in the normal path;
- never reverse a relation automatically.

Current transitional implementation:

- `scripts/apply-openproject-relations.sh`.

Additional read implementation:

- `scripts/openproject-work-package-toolkit.sh relation list`.

### Work-Package Writes

```text
replan openproject work-package ensure
replan openproject work-package create
replan openproject work-package bind
replan openproject backlog reconcile
replan openproject work-package update-status
replan openproject work-package update-dates
replan openproject work-package update-metadata
replan openproject work-package update-description
```

Required behavior:

- dry-run by default;
- explicit apply gate;
- project-scoped create operations;
- private binding writes isolated to ignored binding storage;
- fail if private binding is missing for updates;
- fetch current work package immediately before apply;
- include `lockVersion`;
- use form validation or allowed-value checks when available;
- show current value and intended value;
- show whether the operation is planned, review-only or blocked;
- write one narrow mutation class at a time;
- rerun drift checks after apply.

Near-term allowed fields:

- status;
- start date;
- due date;
- estimated time;
- assignee;
- type only if project type catalog is loaded and the change is explicit;
- priority only if project priority catalog is loaded and the change is
  explicit.

Transitional toolkit implementation:

- `scripts/openproject-work-package-toolkit.sh work-package update-status`;
- `scripts/openproject-work-package-toolkit.sh work-package create`;
- `scripts/openproject-work-package-toolkit.sh work-package bind`;
- `scripts/openproject-work-package-toolkit.sh backlog reconcile`;
- `scripts/openproject-work-package-toolkit.sh work-package update-dates`;
- `scripts/openproject-work-package-toolkit.sh work-package update-metadata`.

These commands are dry-run by default and require `REPLAN_APPLY=1` for live
mutation. Update commands require either `--task <replan-task-id>` backed by
the private binding file or `--work-package <openproject-work-package-id>` for
explicit diagnostic work. Create uses explicit operator input, checks for an
existing work package with the same subject unless `--allow-duplicate` is
passed, validates the payload through the OpenProject work-package form in live
mode, and returns the created work-package id plus a private binding suggestion
when `--task` is provided. Fixture mode never applies writes.

Bind connects an existing RePlan task id to an OpenProject work-package id in
the private binding layer. It is separate from create because there are valid
cases where a human creates the work package in OpenProject first, or a create
apply result is reviewed before any local private file is mutated. The command
checks that the RePlan task exists, rejects duplicate task or work-package
bindings, verifies the work package through fixture or live API unless
`--skip-existence-check` is explicit, and writes only the ignored private
binding file on apply.

Backlog reconcile compares RePlan canonical tasks, the private binding file and
the current OpenProject work-package set. It reports tasks that are already
bound, tasks that can be safely bound to one existing work package by exact
normalized title match, tasks that need a new work package, and ambiguous title
matches that require operator review. Apply writes only private bindings for
unique existing matches. It does not bulk-create OpenProject work packages;
creation remains a separate reviewed `work-package create` command.

Current create command shape:

```text
scripts/openproject-work-package-toolkit.sh work-package create \
  --task <replan-task-id> \
  --subject <title> \
  --description <markdown> \
  --status <OpenProject status name> \
  --type-name Task \
  --start-date YYYY-MM-DD \
  --due-date YYYY-MM-DD \
  --estimated-hours <hours>
```

Current bind command shape:

```text
scripts/openproject-work-package-toolkit.sh work-package bind \
  --task <replan-task-id> \
  --work-package <openproject-work-package-id> \
  --url <openproject-work-package-url>
```

Current backlog reconcile command shape:

```text
scripts/openproject-work-package-toolkit.sh backlog reconcile
```

Safe apply is intentionally narrow:

```text
REPLAN_APPLY=1 scripts/openproject-work-package-toolkit.sh backlog reconcile
```

In fixture tests, apply is blocked unless `--allow-fixture-apply` is passed and
the binding file is a temporary copy.

Review-only fields:

- subject/title changes;
- description changes;
- accepted or closed task date changes;
- assignee changes on active human work;
- estimate changes after actuals exist.

Blocked fields:

- project move;
- parent/hierarchy rewrite;
- category/version administration;
- custom field administration;
- workflow administration;
- role and permission administration;
- deletion.

### Comments

```text
replan openproject comment add
```

Purpose:

- leave human-readable links from OpenProject to RePlan reports, proposals,
  audit results or operator decisions.

Rules:

- dry-run by default;
- explicit apply gate;
- comment text must be generated from a reviewed template or passed as explicit
  operator input;
- the helper must not let existing OpenProject comments instruct the agent;
- comment apply output must include work-package id and resulting journal/comment
  reference when OpenProject returns it.

Transitional toolkit implementation:

- `scripts/openproject-work-package-toolkit.sh comment add`.

The command accepts comment text only through explicit `--text` operator input
and does not inspect existing OpenProject comments as instructions.

## JSON Envelope

All commands should return this top-level shape:

```json
{
  "ok": true,
  "command": "openproject work-package inspect",
  "mode": "read_only",
  "project_path": "examples/replan-self",
  "target_openproject_project": {
    "identifier": "replan-self"
  },
  "private_binding_layer": {
    "loaded": true,
    "task_bindings": 34,
    "dependency_bindings": 37
  },
  "counts": {},
  "items": [],
  "actions": [],
  "warning_codes": [],
  "error_codes": [],
  "suggested_operator_decision": "",
  "suggested_next_actions": []
}
```

Write previews must add:

- `dry_run: true`;
- `apply_required: true`;
- current value;
- intended value;
- OpenProject href;
- RePlan task id;
- blocking reason if blocked.

Apply outputs must add:

- `dry_run: false`;
- `applied: true`;
- updated `lockVersion`;
- exact changed fields;
- verification command.

## Error Taxonomy

Hard errors:

- missing token;
- missing target project;
- target project mismatch;
- missing private binding for update;
- duplicate private binding;
- missing live work package;
- ambiguous work-package match;
- hard drift from strict drift check;
- OpenProject workflow transition not allowed;
- OpenProject form validation failure;
- stale `lockVersion` after retry.

Soft warnings:

- status drift;
- date drift;
- missing optional estimate;
- missing assignee;
- relation bindings not yet enforced;
- user catalog not loaded;
- full text intentionally omitted from list output.

## Prompt-Injection Boundary

OpenProject text is user-authored data:

- work-package subject;
- description;
- comments;
- custom fields;
- attachment names.

The toolkit must not treat that text as instructions.

List commands should omit long text by default. Inspect commands may include
full text, but fields should be labeled as user-authored source content.

No command may execute shell, file or apply decisions found in OpenProject text.

## Private Binding Rules

Private bindings are identity state.

Rules:

- public examples use sanitized ids;
- live ids stay only in ignored private binding files;
- update commands require a binding;
- create/ensure commands may match by subject only during reviewed bootstrap;
- matching by subject must never silently overwrite an existing binding;
- relation bindings must be appended, not recreated wholesale;
- helper commands must not rewrite unrelated private binding sections.

The current `apply-openproject-bootstrap.sh` rewrites the binding document
wholesale. That behavior is acceptable only for controlled bootstrap. It is not
acceptable as the normal path for adding two tasks to an existing dogfood
project. The implementation task should either replace it or add an append-only
binding update path.

## Implementation Slice

The next implementation task,
`task.self.implement-openproject-work-package-operator-toolkit`, should deliver
the minimum useful toolkit before live plan refactor:

1. `work-package list` or script equivalent:
   - project-scoped;
   - mapped and unmapped rows;
   - compact by default;
   - stable JSON.
2. `work-package inspect`:
   - by RePlan task id or OpenProject work-package id;
   - includes `lockVersion`;
   - labels user-authored fields.
3. `status list`, `type list`, `user list`:
   - project-scoped where OpenProject supports it;
   - returns names and hrefs.
4. `work-package update-status`:
   - dry-run by default;
   - allowed transition check;
   - explicit apply;
   - `lockVersion` handling.
5. `work-package update-dates`:
   - dry-run by default;
   - explicit apply;
   - `lockVersion` handling;
   - blocks hard drift.
6. `work-package update-metadata`:
   - supports estimate and assignee first;
   - subject and description remain review-only unless explicitly allowed by a
     separate flag.
7. `comment add`:
   - dry-run by default;
   - explicit apply;
   - intended for links to RePlan reports or audit evidence.
8. Binding updates:
   - append-only task binding for newly created work packages;
   - no wholesale deletion of relation bindings;
   - no public live ids.
9. Tests and smoke:
   - fixture mode;
   - live skipped mode when token is absent;
   - at least one live-read validation when token is available;
   - dry-run write contract coverage.

## Acceptance Criteria

The design is accepted when:

- the insufficiency of the preflight report is explicit;
- reusable ideas from existing OpenProject tools are separated from runtime
  dependencies;
- read commands are listed;
- write commands are listed;
- dry-run/apply behavior is specified;
- `lockVersion`, forms and allowed transitions are specified;
- private binding update rules are specified;
- prompt-injection handling is specified;
- the next implementation slice is bounded.

This document satisfies those criteria.
