# RePlan design work plan

Date: 2026-07-02

Status: closed — historical discussion checklist. Every design topic below was
resolved and documented in an approved specification; see the closure map under
"## Design topics". The implementation specification phase is likewise complete.
This file is retained for history; the approved specs and
`functional-design-checklist.md` are authoritative.

Purpose:

Define the order in which RePlan design decisions should be discussed,
approved, amended and documented.

This document is not the product specification itself.

This document is the checklist for producing the product specification without
losing important points during discussion.

## Working method

For every design topic:

1. Assistant proposes a recommendation.
2. Assistant explains the reasoning and tradeoffs.
3. Operator responds with one of:
   - approve;
   - amend;
   - reject;
   - defer.
4. Approved or amended decision is written into the relevant spec document.
5. The design work plan status is updated.
6. Move to the next topic.

Do not mix too many topics in one decision.

The goal is to convert broad design thinking into implementable specs.

## Status legend

- `pending` — not discussed yet;
- `in discussion` — currently being discussed;
- `approved` — decision accepted and documented;
- `amended` — accepted after changes and documented;
- `deferred` — intentionally postponed;
- `rejected` — explicitly rejected.

## Design topics

> Closure map (added 2026-07-24). Every topic below is historical and closed.
> The per-topic `Status` lines are swept to `closed`; the authoritative output
> of each discussion is the approved specification named here.
>
> | Topic | Closed by |
> | --- | --- |
> | 1. MVP purpose | `mvp-boundary.md`, `product/positioning.md` |
> | 2. Target user and operating mode | `product/positioning.md`, `mvp-boundary.md` |
> | 3. MVP scope boundary | `mvp-boundary.md` |
> | 4. Resource model for MVP | `resource-demand-model.md`, `capacity-availability-rules.md`, `architecture/resource-abstraction.md` |
> | 5. Task source strategy | `pm-system-interaction.md`, `canonical-model.md` |
> | 6. Canonical model | `canonical-model.md` |
> | 7. Calculation model | `workload-calculation.md` |
> | 8. Scenario model | `scenario-model.md` |
> | 8.5 Tracking event model | `tracking-events.md` |
> | 9. Security and access model | `security/access-and-actor-model.md` |
> | 10. CLI contract | `cli.md` |
> | 11. Storage and file layout | `storage-layout.md` |
> | 12. OpenProject integration boundary | `pm-system-interaction.md`, `openproject-overlay-strategy.md` |
> | 13. TaskJuggler boundary | `taskjuggler-integration.md`, `taskjuggler-capacity-quota-mapping.md` |
> | 14. UI boundary | `architecture/application-runtime.md`, `static-observability-dashboard.md` |
> | 15. MVP success and stop criteria | `mvp-boundary.md`, `roadmap/initial-roadmap.md` |
> | 16. Implementation sequence | `implementation-sequence.md` |

### 1. MVP purpose

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

What exactly should the first MVP prove?

Possible purposes:

- usefulness for the operator's own planning;
- prerequisite task/resource infrastructure for OrCode;
- validation of the market gap;
- validation of the generic resource abstraction;
- validation of human + AI actor planning;
- validation of OpenProject as task source;
- validation of TaskJuggler as optional scheduling engine.

Deliverable:

- `docs/spec/mvp-boundary.md`
- section: `MVP purpose`

Current draft decision:

- MVP should validate local planning value and OrCode prerequisite value.
- MVP should be AI-CLI-first, not only human-CLI-friendly.
- MVP should generate human-verifiable reports/views without building a full
  web UI.
- MVP should not implement custom authentication.
- MVP should include a real minimal local policy/audit boundary rather than a
  fake placeholder.
- MVP should include lightweight execution tracking: baseline/current/actual,
  status date, planned vs actual, remaining work, variance and evidence links.
- MVP tracking should be event-assisted by design: OpenProject/Git/CI/agent
  events become evidence and proposals, not automatic truth.

### 2. Target user and operating mode

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

Who is the first MVP for and how will it be operated?

Candidate modes:

- operator only;
- operator plus one or two human developers;
- operator plus human developers plus AI agents;
- small self-hosted software team;
- contractor team working under client constraints;
- local CLI only;
- self-hosted trusted-team mode.

Deliverable:

- `docs/spec/mvp-boundary.md`
- section: `Target user and operating mode`

### 3. MVP scope boundary

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

What is included in the first MVP and what is explicitly excluded?

Candidate inclusions:

- CLI;
- YAML fixtures;
- local validation;
- weekly workload report;
- markdown output;
- CSV output;
- JSON snapshot;
- simple scenarios;
- local audit log;
- local policy YAML.

Candidate exclusions:

- backend server;
- hosted SaaS;
- multi-user;
- custom auth;
- full RBAC;
- web UI;
- OpenProject write-back;
- TaskJuggler import;
- automatic resource leveling;
- MCP server;
- real equipment gateways;
- billing.

Deliverable:

- `docs/spec/mvp-boundary.md`
- sections: `Included`, `Excluded`, `Later`

### 4. Resource model for MVP

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

Which resource kinds should be supported in the MVP, and which should be only
designed for later?

Candidate resource kinds:

- `human`;
- `ai_agent`;
- `service_quota`;
- `budget`;
- `subscription_entitlement`;
- `equipment`;
- `environment`;
- `generic`.

Decision needed:

- MVP-supported;
- design-only;
- later;
- out of scope.

Deliverable:

- `docs/spec/mvp-boundary.md`
- section: `MVP resource kinds`
- possibly `docs/spec/canonical-model.md`

### 5. Task source strategy

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

Where do tasks come from in the MVP and near-term roadmap?

Candidates:

- YAML first;
- OpenProject read-only import first;
- Plane later;
- GitHub Issues later;
- GitLab Issues later;
- Quollnet JSON import later;
- spreadsheet/CSV import later.

Deliverable:

- `docs/spec/mvp-boundary.md`
- section: `Task sources`
- later `docs/spec/openproject-adapter.md`

### 6. Canonical model

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

What are the canonical entities and fields?

Candidate entities:

- `Resource`;
- `Task`;
- `ResourceDemand`;
- `Reservation`;
- `Consumption`;
- `Scenario`;
- `Snapshot`;
- `Actor`;
- `Policy`;
- `AuditEvent`;
- `Gateway`;
- `Report`.

Deliverable:

- `docs/spec/canonical-model.md`

### 7. Calculation model

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

What exactly does RePlan calculate in the MVP?

Candidate calculations:

- weekly capacity;
- workload by resource;
- overload detection;
- missing estimate warnings;
- missing date warnings;
- simple dependency warnings;
- deadline feasibility warnings;
- scenario diff;
- service quota usage warnings;
- budget warnings.

Deliverable:

- `docs/spec/workload-calculation.md`

### 8. Scenario model

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

Which what-if scenarios are supported first?

Candidate scenarios:

- human unavailable;
- AI agent unavailable;
- AI agent added;
- estimate changed;
- task moved;
- assignee changed;
- deadline fixed;
- service quota reduced;
- budget reduced;
- subscription capacity reduced.

Deliverable:

- `docs/spec/scenario-model.md`
- or section inside `docs/spec/canonical-model.md`

### 8.5 Tracking event model

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

How should RePlan use events from PM systems, git hosts, CI systems, gateways,
humans and AI agents to track actual execution without creating false progress?

Current draft decision:

- RePlan should support event-assisted tracking.
- Events are evidence, not automatic truth.
- OpenProject/GitHub/GitLab/CI/agent events may create evidence and proposals.
- Confirmed actuals update tracking state.
- Dangerous state changes and write-back require explicit approval or trusted
  policy.

Deliverable:

- `docs/spec/tracking-events.md`

### 9. Security and access model for MVP

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

Which parts of the access model are implemented in MVP and which remain design
guardrails?

Candidate MVP:

- no custom auth;
- local execution trust boundary;
- local policy YAML;
- central authorization function for mutating commands;
- actor IDs;
- simple role/capability mapping;
- local append-only audit log;
- no backend user management.

Candidate later:

- OIDC;
- reverse proxy auth;
- OPA;
- service accounts;
- MCP authorization;
- workload identity;
- multi-user UI;
- enterprise SSO.

Deliverable:

- `docs/spec/mvp-boundary.md`
- section: `MVP access and audit`
- later `docs/spec/security-mvp.md`

### 10. CLI contract

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

What exact commands, arguments, outputs and exit codes should the MVP expose?

Candidate commands:

- `replan validate`;
- `replan snapshot create`;
- `replan workload`;
- `replan scenario create`;
- `replan scenario compare`;
- `replan report weekly`;
- `replan export taskjuggler`;
- `replan apply --dry-run`.

Deliverable:

- `docs/spec/cli.md`

### 11. Storage and file layout

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

How should MVP data be stored?

Candidate layout:

```text
examples/small-team/
    resources.yaml
    tasks.yaml
    policy.yaml
    scenarios.yaml

snapshots/
    *.json

reports/
    *.md
    *.csv
    *.json

audit/
    audit.log
```

Open question:

- no SQLite in MVP;
- SQLite in MVP;
- SQLite only after reports work.

Deliverable:

- `docs/spec/storage-layout.md`

### 12. OpenProject integration boundary

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

What should the first OpenProject integration do?

Candidate first step:

- read-only import;
- map work packages to tasks;
- map users to resources;
- map estimates;
- map dates;
- map dependencies;
- preserve raw metadata;
- generate warnings for missing fields.

Candidate later:

- dry-run write-back;
- comments;
- custom fields;
- scenario IDs;
- approved start/finish updates;
- MCP integration if needed.

Deliverable:

- `docs/spec/openproject-adapter.md`

### 13. TaskJuggler boundary

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

What should the first TaskJuggler integration do?

Candidate first step:

- no runtime dependency in MVP;
- export-only proof of concept;
- use TaskJuggler as parity benchmark;
- no import until export proves useful.

Candidate later:

- run TaskJuggler from CLI;
- parse results;
- import schedule as RePlan scenario;
- compare baseline vs TaskJuggler scenario;
- approved write-back.

Deliverable:

- `docs/spec/taskjuggler-adapter.md`

### 14. UI boundary

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

Does MVP need a UI?

Candidate answer:

- no interactive UI in MVP;
- markdown/CSV/JSON reports first;
- static HTML report optional after CLI proves useful;
- Quollnet-style timeline later;
- web app much later.

Deliverable:

- `docs/spec/mvp-boundary.md`
- section: `UI boundary`

### 15. MVP success and stop criteria

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

When is the MVP successful, and when should RePlan stop or pivot?

Candidate success criteria:

- weekly report is actually used by operator;
- report shows overloads or missing data that change planning decisions;
- scenarios are faster or clearer than spreadsheet/manual planning;
- RePlan can model operator, two human developers and at least one AI agent;
- RePlan can represent at least one non-time resource such as API budget or
  service quota;
- output is readable in Obsidian or markdown review.

Candidate stop criteria:

- spreadsheet remains faster;
- model becomes too complex before first report;
- OpenProject API friction dominates;
- TaskJuggler mapping dominates before workload reports prove value;
- reports do not change decisions;
- project drifts into full PM-suite clone.

Deliverable:

- `docs/spec/mvp-boundary.md`
- section: `Success criteria and stop criteria`

### 16. Implementation sequence

Status: closed — superseded by an approved specification (see the closure map under "## Design topics").

Question:

After design decisions, what is the coding order?

Candidate order:

1. fixtures;
2. canonical model;
3. validation;
4. workload calculation;
5. markdown/CSV reports;
6. scenario diff;
7. local audit log;
8. OpenProject read-only import;
9. TaskJuggler export proof of concept.

Deliverable:

- `docs/spec/implementation-sequence.md`
- or section inside `docs/spec/mvp-boundary.md`

## Functional planning/tracking closure checklist

The functional design checklist now lives in a dedicated document:

[`functional-design-checklist.md`](functional-design-checklist.md)

That checklist is the active source for tracking whether the functional design
of RePlan planning and execution tracking is closed enough for implementation.

Current status:

- functional planning/tracking design is approved and documented;
- deployment architecture is documented;
- application runtime architecture is documented;
- implementation stack direction is documented;
- the next phase is not more broad product discussion, but conversion of the
  approved design into implementation specifications.

## Implementation specification phase

Status: active next phase.

Purpose:

Prevent implementation from drifting away from the approved functional and
architecture decisions.

This phase should convert the approved design into concrete implementation
contracts before or alongside the first code skeleton.

Important constraint:

These documents should stay practical. They are not enterprise architecture
ceremony. They exist so that the first RePlan implementation does not randomly
invent data fields, file paths, command names and behavior while coding.

### I. Canonical model specification

Status: approved.

Question:

What exact entities, fields, identifiers, references and invariants make up the
first RePlan canonical model?

The document must consolidate decisions from:

- [`planning-lifecycle.md`](planning-lifecycle.md);
- [`tracking-events.md`](tracking-events.md);
- [`task-state-model.md`](task-state-model.md);
- [`resource-demand-model.md`](resource-demand-model.md);
- [`capacity-availability-rules.md`](capacity-availability-rules.md);
- [`workload-calculation.md`](workload-calculation.md);
- [`planned-current-actual-comparison.md`](planned-current-actual-comparison.md);
- [`dependency-deadline-behavior.md`](dependency-deadline-behavior.md);
- [`scenario-model.md`](scenario-model.md);
- [`evidence-proposal-behavior.md`](evidence-proposal-behavior.md);
- [`report-behavior.md`](report-behavior.md);
- [`pm-system-interaction.md`](pm-system-interaction.md);
- [`../architecture/system-design.md`](../architecture/system-design.md);
- [`../architecture/resource-abstraction.md`](../architecture/resource-abstraction.md);
- [`../security/access-and-actor-model.md`](../security/access-and-actor-model.md).

Expected deliverable:

- `docs/spec/canonical-model.md`

Minimum content:

- `Project`;
- `Task`;
- `TaskSourceRef`;
- `TaskTrackingState`;
- `Resource`;
- `ResourcePool`;
- `ResourceDemand`;
- `Capacity`;
- `Availability`;
- `Reservation`;
- `Actual`;
- `Baseline`;
- `CurrentPlan`;
- `Scenario`;
- `Dependency`;
- `Deadline`;
- `Evidence`;
- `Proposal`;
- `Warning`;
- `Actor`;
- `PolicyRef`;
- `AuditRecord`;
- `ReportSnapshot`;
- common primitives for identifiers, dates, periods, units, amounts and source
  references.

Design guardrails:

- canonical model must be source-agnostic;
- OpenProject, Plane, GitHub, GitLab and TaskJuggler details must live in
  adapters and source references, not in core entities;
- missing data must be represented as unknown and warnings, not silently as
  zero or infinity;
- baseline, current plan, actuals, evidence and proposals must remain separate;
- resources must not be limited to human hours;
- AI agents, service quotas, subscription entitlements, rate limits, equipment,
  environments, budget and offline work resources must fit the same model;
- actor, policy and audit fields must exist from the beginning, even when MVP
  enforcement is intentionally lightweight.

### II. Storage layout specification

Status: approved.

Question:

How is the local-first canonical state stored on disk in the first MVP?

Expected deliverable:

- `docs/spec/storage-layout.md`

Minimum content:

- project directory layout;
- file naming rules;
- YAML versus JSON usage;
- where imported raw source snapshots live;
- where canonical state lives;
- where baselines, scenarios, actuals, evidence, proposals, audit records and
  reports live;
- how generated files are separated from hand-edited files;
- how future SQLite storage can be introduced behind a storage adapter without
  breaking the CLI contract.

Design guardrails:

- files are the first canonical storage mode;
- Git may be used for history, review, synchronization and backup, but the
  domain model must not require Git to run;
- storage must preserve stable identifiers;
- storage must support deterministic validation;
- storage must be understandable to humans and AI CLI agents.

### III. CLI contract specification

Status: approved.

Question:

What exact commands, arguments, outputs, exit codes and mutation rules does the
MVP CLI expose?

Expected deliverable:

- `docs/spec/cli.md`

Minimum command families:

- `replan validate`;
- `replan workload`;
- `replan report`;
- `replan scenario`;
- `replan proposal`;
- `replan actual`;
- `replan import`;
- `replan export`;
- `replan audit`.

Design guardrails:

- every important workflow must be usable by AI CLI agents;
- every report-like command must have structured JSON output;
- human-readable Markdown output must be available where useful;
- mutating commands must support preview or dry-run behavior where the risk is
  non-trivial;
- command behavior must call application services, not contain unique business
  logic inside CLI handlers.

### IV. Implementation sequence specification

Status: approved.

Question:

In what order should the first implementation be built so that each step is
testable and useful?

Expected deliverable:

- `docs/spec/implementation-sequence.md`

Recommended order:

1. initialize Go module and package layout;
2. create minimal domain structs for the canonical model;
3. create example project fixtures;
4. implement file storage reader;
5. implement validation command;
6. implement workload calculation over fixtures;
7. implement JSON and Markdown reports;
8. implement scenario diff;
9. implement local audit records for mutating commands;
10. implement read-only OpenProject import;
11. implement TaskJuggler export proof of concept;
12. only then consider daemon, richer static HTML report, server mode or UI.

Design guardrails:

- do not start with web UI;
- do not start with a custom authentication system;
- do not start with a full solver;
- do not start with OpenProject write-back;
- do not make TaskJuggler a mandatory runtime dependency;
- do not build enterprise features before the internal CLI/reporting loop proves
  useful.

### V. First code skeleton

Status: initial implementation complete.

Question:

What is the smallest executable RePlan program that proves the architecture is
real?

Expected deliverable:

- Go module;
- `cmd/replan`;
- `internal/domain`;
- `internal/app`;
- `internal/storage/filestore`;
- `internal/reports`;
- `examples/small-team`;
- one working command:

```bash
replan validate examples/small-team --format json
```

Design guardrails:

- core logic must be reusable by future CLI, daemon, server and UI;
- CLI handlers must stay thin;
- validation errors must be structured;
- examples must include humans, at least one AI agent, at least one non-human
  non-agent resource and at least one offline task.

## Current next topic

None. This discussion checklist is closed: every design topic and every
implementation-specification phase item (I-V) is documented in an approved
specification, and the first code skeleton is implemented. Ongoing work is
tracked outside this file:

- MVP readiness and dated audit results: `mvp-readiness.md`;
- implemented slices and sequence: `implementation-sequence.md`;
- open code, test and documentation gaps: `../dev/known-gaps.md`;
- OSS separation backlog: `../dev/oss-separation-plan.md`.
