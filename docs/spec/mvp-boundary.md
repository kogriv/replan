# RePlan MVP boundary

Date: 2026-07-02

Status: approved. The MVP boundary it defines is implemented and was audited as
ready for internal CLI-first use (see [`mvp-readiness.md`](mvp-readiness.md),
audits 2026-07-16 and 2026-07-24). Later additions beyond this boundary remain
explicitly deferred, not draft.

This document defines the first RePlan MVP boundary.

It was built through the design discussion plan in
[`design-work-plan.md`](design-work-plan.md), now closed.

## 1. MVP purpose

Status: approved.

The first RePlan MVP exists to validate whether an AI-CLI-first,
human-verifiable, local FOSS resource planning and lightweight execution
tracking layer can make planning decisions clearer for a small human + AI
software team, while providing enough minimal policy and audit structure to
become the task/resource prerequisite for resuming OrCode.

In simpler terms:

```text
MVP RePlan should prove that we can plan a small mixed team of humans and AI
agents and track whether that plan is being executed better than by memory,
spreadsheet or ad hoc markdown, without building a full PM suite.
```

The MVP should not prove that RePlan is a full product.

The MVP should prove that RePlan is a useful planning substrate.

The MVP should also prove that RePlan can show the difference between:

```text
baseline plan
current plan
actual execution
```

## MVP purpose pillars

### 1.1 AI-CLI-first

RePlan MVP should be designed first for AI CLI agents and automation, not only
for a human typing commands manually.

Target users of the CLI include:

- operator;
- Codex-like coding agent;
- Claude Code-like coding agent;
- OpenCode-like coding agent;
- future custom agents;
- sync/import automation;
- report generation automation.

This means:

- all important operations must be available through CLI;
- commands should be non-interactive by default;
- commands should support machine-readable JSON output;
- commands should support human-readable output where useful;
- command results should be deterministic;
- command failures should be structured and understandable;
- commands should have stable exit codes;
- mutating or risky commands should support `--dry-run`;
- AI agents should be able to read, validate, compare and propose without
  corrupting source-of-truth state.

Example intended command style:

```bash
replan validate --format json
replan workload --week 2026-W28 --format json
replan scenario compare baseline ivan-unavailable --format json
replan report weekly --week 2026-W28 --format markdown
replan propose estimate task-123 --remaining-hours 12 --dry-run --format json
```

Design implication:

```text
If a workflow cannot be driven safely by a CLI agent, it is not MVP-ready.
```

### 1.2 Human-verifiable generated views

RePlan MVP should not include a full web application.

However, it must produce outputs that a human can inspect quickly.

Planning is a human decision loop. JSON alone is not enough.

Required human-verifiable outputs:

- markdown weekly report;
- CSV workload table;
- JSON snapshot/result for machines;
- at least one generated visual or semi-visual representation.

Candidate generated views:

- static HTML workload report;
- markdown tables;
- Mermaid Gantt;
- Mermaid flow/dependency graph;
- CSV suitable for spreadsheet inspection;
- simple HTML heatmap of resource load;
- scenario diff report.

MVP should prefer generated artifacts over an interactive app:

```text
reports/
    weekly-2026-W28.md
    workload-2026-W28.csv
    workload-2026-W28.json
    workload-2026-W28.html
    scenario-baseline-vs-ivan-unavailable.md
```

Design implication:

```text
No full web UI in MVP, but human validation must be first-class.
```

The operator should be able to open a report and answer:

- who is overloaded;
- who is underused;
- what data is missing;
- which deadline looks suspicious;
- what changed in a scenario;
- what needs approval.

### 1.3 Local-first and FOSS-first

The MVP should run locally.

It should not require:

- hosted SaaS;
- backend server;
- external database;
- enterprise identity provider;
- cloud account;
- paid service.

The MVP may use files and local state:

- YAML fixtures;
- JSON snapshots;
- markdown reports;
- CSV reports;
- local audit log;
- optional SQLite later if needed.

Design implication:

```text
Local usefulness comes before hosted product concerns.
```

### 1.4 Minimal policy and audit, no custom auth

The MVP should not implement its own authentication system.

MVP must not include:

- password login;
- password storage;
- password reset;
- MFA;
- SSO;
- OIDC provider;
- SAML;
- user invitation flow;
- hosted multi-tenant identity.

However, MVP should include a real minimal authorization boundary.

This should not be a fake placeholder.

Minimum MVP concepts:

- actor ID;
- local policy file;
- role/capability mapping;
- scope fields where relevant;
- central authorization check for mutating and agent-facing commands;
- audit events for proposals, scenario changes and dry-runs.

Example local policy style:

```yaml
actors:
  operator:
    role: operator
    capabilities:
      - task.read
      - resource.read
      - scenario.create
      - scenario.compare
      - scenario.approve
      - writeback.dry_run

  agent_coder_1:
    role: agent
    capabilities:
      - task.read
      - resource.read
      - task.propose_update
      - scenario.create
      - scenario.compare
    forbidden_capabilities:
      - writeback.apply
```

Example execution style:

```bash
REPLAN_ACTOR=agent_coder_1 replan propose estimate task-123 --remaining-hours 12
REPLAN_ACTOR=operator replan scenario approve ivan-unavailable
```

All mutating or agent-facing commands should conceptually pass through:

```text
authorize(principal, actor, action, target, context)
```

In MVP, `principal` can be simple:

- local OS user;
- `REPLAN_ACTOR`;
- explicit `--actor`;
- local trusted execution context.

Later, this can map to:

- OIDC;
- reverse proxy auth;
- MCP/OAuth-style authorization;
- service accounts;
- workload identity.

Design implication:

```text
No custom auth in MVP, but no naive "everyone can do everything" model either.
```

### 1.5 OrCode prerequisite

The MVP should test whether RePlan can become the task/resource/agent planning
prerequisite for resuming OrCode.

This means the MVP should model at least:

- operator;
- human developer;
- AI coding agent;
- task;
- estimate;
- availability;
- capacity;
- scenario;
- proposal or dry-run;
- report;
- audit event.

If RePlan cannot make this small operational model useful, OrCode should remain
paused or the planning-infrastructure direction should be reconsidered.

### 1.6 Lightweight execution tracking

RePlan MVP should not only create plans.

It should also track whether the plan is being executed as expected.

This does not mean building a full timesheet, payroll, surveillance or earned
value management system.

It means recording enough actual execution data to answer:

- what was planned;
- what is currently planned;
- what actually happened;
- what remains;
- what slipped;
- what evidence exists;
- what changed since the baseline.

The core comparison should be:

```text
baseline plan
    vs
current plan
    vs
actual execution
```

MVP tracking concepts:

- `status_date`;
- planned reservation;
- actual consumption;
- remaining work;
- progress status;
- variance;
- evidence link;
- audit event.

For a human resource:

```text
planned:
    Ivan reserved 12 hours this week

actual:
    Ivan spent 7 hours
    remaining estimate is 8 hours

variance:
    task is likely larger than planned
```

For an AI agent:

```text
planned:
    agent_coder_1 reserved 6 hours and 15 USD of LLM budget

actual:
    agent consumed 4 hours and 11.20 USD
    pull request and test output are attached as evidence
```

For a service quota:

```text
planned:
    GitHub import reserves 1000 API requests

actual:
    import consumed 1320 requests

variance:
    plus 320 requests against reservation
```

Minimum tracking outputs:

- planned vs actual workload by resource;
- baseline vs current date variance;
- remaining work by task;
- tasks with missing progress updates;
- tasks slipping against baseline;
- evidence links for completed or partially completed work;
- report as of a chosen `status_date`.

Explicit non-goals for MVP tracking:

- payroll;
- employee surveillance;
- detailed timesheets;
- automatic productivity scoring;
- full earned value management;
- full cost accounting;
- automatic progress inference from git without human/agent evidence;
- enterprise status reporting.

Design implication:

```text
MVP RePlan = planning + lightweight execution tracking, not planning only.
```

Tracking should be event-assisted by design.

RePlan should work best in a larger task/execution complex:

```text
PM system
    task truth and status

Git host / CI / agent runtime / gateways
    execution events and evidence

RePlan
    resource plan, actuals, variance, tracking proposals and reports
```

MVP may start from YAML fixtures, but the product design assumes RePlan is a
resource/tracking layer around a PM/task substrate such as OpenProject.

The key tracking rule is:

```text
events are evidence, not automatic truth
```

Examples:

- OpenProject status change can show that work started;
- PR opened can attach implementation evidence;
- CI passed can attach verification evidence;
- agent runtime can report token/cost consumption;
- logged time can become actual consumption if trusted;
- derived human work hours from elapsed time should be a proposal, not a fact;
- task completion from PR/CI should require confirmation or trusted policy.

See [`tracking-events.md`](tracking-events.md) for the detailed event-assisted
tracking model.

## Proof questions

The MVP purpose should be evaluated through these questions:

1. Can RePlan show weekly workload for a small human + AI team?
2. Can RePlan show overload and underuse?
3. Can RePlan show missing estimates, missing dates and suspicious deadlines?
4. Can RePlan compare baseline and a simple what-if scenario?
5. Can RePlan model an AI CLI agent as both a schedulable resource and a
   constrained actor?
6. Can RePlan model at least one non-human/non-time resource such as a service
   quota or budget?
7. Can a Codex/Claude Code/OpenCode-like CLI agent safely call RePlan commands
   and receive structured results?
8. Can a human validate the plan through generated reports without needing a
   full web UI?
9. Can RePlan show planned vs actual resource usage?
10. Can RePlan show which tasks slipped against baseline or current plan?
11. Can RePlan show remaining work and missing progress updates?
12. Can RePlan attach or reference evidence for completed or partially completed
    work?
13. Can RePlan report status as of a chosen `status_date`?
14. Can RePlan treat OpenProject/Git/CI/agent events as evidence and tracking
    proposals rather than automatic truth?
15. Does the report change a real planning decision?
16. Is this enough to justify moving next to OpenProject read-only import and
    optional TaskJuggler export?

## Explicit non-purpose

The MVP does not exist to prove:

- RePlan can replace OpenProject;
- RePlan can replace Jira;
- RePlan can replace MS Project;
- RePlan can beat Runn, Float, Resource Guru or other commercial resource
  planning SaaS products;
- RePlan can provide enterprise RBAC;
- RePlan can provide SSO;
- RePlan can provide hosted SaaS;
- RePlan can provide a polished web UI;
- RePlan can provide automatic resource leveling;
- RePlan has TaskJuggler parity;
- RePlan is a full timesheet/payroll/progress-accounting system;
- RePlan is a full earned value management system;
- RePlan is ready for public OSS launch;
- RePlan is monetizable.

Those are later questions.

The first question is smaller and more important:

```text
Does RePlan make local planning decisions clearer for a small human + AI team?
```
