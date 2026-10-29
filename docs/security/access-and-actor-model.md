# Access, identity and actor model

Date: 2026-07-02

Status: initial design guardrail.

## Core decision

RePlan should not implement enterprise authentication, enterprise RBAC or
multi-tenant SaaS controls in the MVP.

However, RePlan must define identity, actor, resource, permission and gateway
boundaries from the beginning.

Reason:

The project should avoid enterprise overhead, but it should also avoid a design
that later requires rewriting the core when:

- several humans participate;
- contractors should not see all work;
- AI agents need limited access;
- external agents such as Codex or Claude Code need safe task interfaces;
- non-human resources such as machines, robots, test benches or deployment
  environments require controlled interaction;
- a team uses RePlan as a contractor inside a larger company with strict
  compliance requirements.

## Security posture by phase

### MVP posture

MVP should be local or trusted single-team.

Allowed:

- CLI-first;
- local files;
- SQLite;
- OpenProject token configured externally;
- explicit dry-run before write-back;
- append-only local audit log;
- no public internet exposure;
- no custom login system;
- no multi-tenant SaaS;
- no fine-grained UI permissions.

Not allowed:

- unauthenticated public web UI;
- silent write-back to task systems;
- AI agents applying changes by default;
- source tokens stored in generated reports;
- free-form agent mutation of planning files as the primary API.

### Self-hosted team posture

Self-hosted team mode should rely on external authentication.

Preferred external auth options:

- reverse proxy authentication;
- Tailscale or VPN;
- Authelia;
- Authentik;
- Keycloak;
- cloud identity provider through OIDC;
- corporate SSO when available.

RePlan should accept identity claims from the surrounding environment rather
than becoming the identity provider.

Example:

```text
Browser or CLI
    -> reverse proxy / identity provider
    -> RePlan API
    -> RePlan policy check
    -> operation
```

### Later SaaS or enterprise posture

Only after proven demand:

- tenant model;
- tenant-scoped roles;
- OIDC/OAuth client support;
- SAML only if there is real enterprise demand;
- service accounts;
- API tokens;
- audit retention;
- support access controls;
- billing;
- row-level isolation;
- policy engine integration.

This must not be part of the first useful product.

## Do not write our own auth

RePlan should not own:

- password storage;
- password reset;
- MFA;
- SSO federation;
- SAML implementation;
- user invitation flows at MVP stage;
- session security beyond simple local/self-hosted needs.

RePlan may later integrate with:

- OIDC/OAuth 2.1 identity providers;
- MCP authorization flows for agent-facing interfaces;
- workload identity systems for machine actors.

Relevant external references:

- OAuth 2.1 draft: https://datatracker.ietf.org/doc/html/draft-ietf-oauth-v2-1-11
- MCP authorization: https://modelcontextprotocol.io/specification/2025-11-25/basic/authorization
- Open Policy Agent: https://openpolicyagent.org/docs
- SPIFFE/SPIRE workload identity: https://spiffe.io/docs/latest/spire-about/spire-concepts/

## Identity model

RePlan should distinguish several concepts that are often mixed together.

### Principal

A principal is the authenticated caller.

Examples:

- human user;
- CLI token;
- service account;
- AI agent process;
- MCP client;
- automation job;
- external robot controller;
- OpenProject sync job.

### Actor

An actor is an entity that can perform actions in RePlan.

An actor may or may not be a schedulable resource.

Examples:

- operator;
- developer;
- reviewer;
- AI coding agent;
- AI research agent;
- CI bot;
- OpenProject sync worker;
- robot controller.

### Resource

A resource is something whose capacity can be planned.

Examples:

- human developer;
- AI coding agent;
- reviewer;
- test device;
- GPU machine;
- deployment environment;
- physical robot;
- lab equipment;
- contractor block of hours.
- API quota;
- SaaS subscription seat pool;
- LLM provider credit budget;
- cloud spending budget;
- CI minutes;
- rate-limited external service;

### Target object

A target object is something an actor wants to read or modify.

Examples:

- project;
- task;
- resource calendar;
- scenario;
- baseline;
- report;
- connector credential reference;
- write-back operation;
- audit event.

## Actor can be resource, but not always

Important rule:

```text
actor identity and schedulable resource are separate concepts
```

A human developer is usually both:

- actor: can log in, read tasks, propose changes;
- resource: has weekly capacity.

An AI coding agent may be both:

- actor: can call `replan propose estimate`;
- resource: has modeled capacity for low-risk coding tasks.

A GPU server may be only a resource:

- resource: has capacity and calendar;
- actor: none.

A sync worker may be only an actor:

- actor: imports OpenProject tasks;
- resource: none.

This separation prevents the data model from breaking when non-human resources
or service accounts appear.

## Consumable and entitlement resources

Some resources are not people, machines or physical equipment.

They are rights to consume an external service.

Examples:

- API quota;
- requests per minute;
- requests per day;
- tokens per minute;
- tokens per month;
- LLM provider credits;
- cloud budget;
- build minutes;
- CI runner minutes;
- GitHub Actions minutes;
- hosted vector database quota;
- paid SaaS seats;
- concurrent browser sessions;
- scraping/proxy bandwidth;
- SMS/email sending quota;
- map/geocoding API quota;
- payment provider rate limits;
- external data vendor subscription;
- support plan entitlement;
- license seat pool.

These should be modeled as resources because they constrain work.

They should not be modeled as roles.

Reason:

- a role answers "who may do this?";
- a consumable resource answers "is there enough quota/capacity/budget to do
  this?";
- a subscription entitlement answers "does the project/team have access to this
  capability?";
- a rate limit answers "how fast can this actor or workflow consume the
  service?";
- a budget answers "how much can be spent before approval is needed?".

Example:

```yaml
id: openai_api_budget_main
display_name: OpenAI API monthly budget
kind: consumable_service
provider: openai
unit: usd
period: month
capacity: 500
warning_threshold: 0.8
hard_limit: 500
scope:
  projects: [orcode, replan]
allowed_actors:
  - agent_research_1
  - agent_coder_1
  - operator
gateway: openai_usage_gateway
```

Example rate-limited API:

```yaml
id: github_api_rate_limit
display_name: GitHub API rate limit
kind: rate_limited_api
provider: github
unit: requests
period: hour
capacity: 5000
scope:
  source: github:kogriv
allowed_actors:
  - github_connector
  - agent_coder_1
gateway: github_api_gateway
```

Example SaaS subscription:

```yaml
id: browser_cloud_subscription
display_name: Browser automation cloud subscription
kind: subscription_entitlement
provider: external-browser-cloud
unit: concurrent_sessions
period: current_plan
capacity: 3
scope:
  projects: [replan]
allowed_actors:
  - playwright_agent
gateway: browser_cloud_gateway
```

The same planning questions apply:

- who or what can consume it;
- how much is available;
- how much is already reserved or consumed;
- when the quota resets;
- what happens if it is exhausted;
- whether approval is needed to exceed a warning threshold;
- whether the cost is allocated to a project;
- whether an AI agent may consume it autonomously.

### Consumable resource state

Consumable resources need slightly different state than people.

Useful fields:

- `provider`;
- `account_id`;
- `unit`;
- `period`;
- `capacity`;
- `used`;
- `reserved`;
- `remaining`;
- `reset_at`;
- `warning_threshold`;
- `hard_limit`;
- `cost_per_unit`;
- `scope`;
- `gateway`;
- `approval_policy`.

### Reservation vs consumption

RePlan should distinguish planned reservation from actual consumption.

Example:

```text
planned:
    agent research task reserves 100k LLM tokens

actual:
    task consumed 83k LLM tokens
```

This is analogous to planned hours vs actual hours for humans.

### Gateway requirement

RePlan should not scrape or control every provider directly from the core.

Use gateway adapters:

```text
RePlan consumable resource
    -> provider gateway
    -> external API / billing API / usage API
```

Gateway operations:

- read current entitlement;
- read current usage;
- read reset time;
- reserve planned usage if provider supports it;
- estimate cost;
- dry-run high-cost operation;
- apply approved consumption action if relevant;
- write audit event.

### Policy examples

Policy can constrain consumable resources:

```yaml
agents:
  agent_research_1:
    allowed_resources:
      - openai_api_budget_main
    max_cost_per_task_usd: 20
    requires_approval_above_usd: 50
    max_tokens_per_task: 200000

  agent_coder_1:
    allowed_resources:
      - github_api_rate_limit
      - openai_api_budget_main
    forbidden_resources:
      - production_payment_gateway
```

This keeps subscription/API control in the same planning and authorization model
without turning RePlan into a billing platform.

The general rule is documented in
[`../architecture/resource-abstraction.md`](../architecture/resource-abstraction.md):
RePlan should model resource capacity, units, reservations, consumption,
constraints, gateways and policy instead of trying to enumerate every possible
resource class in advance.

## Minimal roles

Do not implement complex enterprise RBAC first.

Define a small role vocabulary that can later map to external groups.

Initial roles:

- `viewer`;
- `planner`;
- `operator`;
- `agent`;
- `connector`;
- `admin`.

### viewer

Can:

- read allowed projects;
- read reports;
- read scenarios;
- export read-only reports.

Cannot:

- create scenarios;
- modify resource calendars;
- apply write-back;
- manage connector credentials.

### planner

Can:

- create scenarios;
- edit scenario assumptions;
- propose task date changes;
- propose assignment changes;
- generate reports;
- run TaskJuggler export for allowed projects.

Cannot:

- write back to external task systems without operator approval;
- manage identity;
- manage connector credentials.

### operator

Can:

- approve scenarios;
- run write-back dry-runs;
- apply approved write-back operations;
- mark planning decisions;
- close or reject agent proposals.

Cannot by default:

- bypass audit log;
- access projects outside scope.

### agent

Can:

- read explicitly assigned tasks;
- read allowed reports;
- propose estimate changes;
- propose assignment changes within policy;
- propose scenario changes;
- attach evidence;
- request approval.

Cannot by default:

- apply write-back;
- change deadlines;
- reassign human work broadly;
- read all projects;
- read sensitive projects unless explicitly allowed;
- manage credentials.

### connector

Can:

- import from an external source;
- perform approved write-back operations;
- read connector-specific mapping data.

Cannot:

- make independent planning decisions;
- approve its own changes.

### admin

Can:

- configure sources;
- configure policies;
- manage local RePlan settings;
- map external identity groups to RePlan roles.

Admin should not automatically mean operational approval in every project. In
larger teams, technical administration and planning authority may differ.

## Permission model

Use capability checks rather than hardcoding everything into roles.

Example capabilities:

- `project.read`;
- `task.read`;
- `task.propose_update`;
- `resource.read`;
- `resource.calendar.edit`;
- `scenario.create`;
- `scenario.compare`;
- `scenario.approve`;
- `writeback.dry_run`;
- `writeback.apply`;
- `connector.configure`;
- `credential.reference`;
- `audit.read`;
- `policy.manage`.

Roles should map to capabilities.

External groups should map to roles or capabilities.

Agent policies should map to capabilities plus constraints.

## Scope model

Every permission check should include scope.

Important scopes:

- project;
- source system;
- task;
- resource;
- scenario;
- connector;
- environment;
- tenant later if SaaS exists.

Examples:

```text
developer Ivan can read project demo
developer Ivan cannot read project secret-client
agent_coder_1 can read tasks assigned to agent_coder_1
agent_coder_1 can propose estimates only for allowed task types
operator can apply write-back only for project demo
connector_openproject can write only through approved write-back operations
```

## Policy model

For MVP, policy can be a small local YAML file.

Example:

```yaml
roles:
  operator:
    capabilities:
      - project.read
      - task.read
      - scenario.create
      - scenario.compare
      - scenario.approve
      - writeback.dry_run
      - writeback.apply

  agent:
    capabilities:
      - task.read
      - task.propose_update
      - scenario.create
      - scenario.compare

agents:
  agent_coder_1:
    role: agent
    allowed_projects: [demo]
    allowed_task_types: [bug, docs, low_risk_code]
    forbidden_capabilities:
      - writeback.apply
    requires_human_approval: true
```

Later, RePlan can integrate with a policy engine such as Open Policy Agent.

OPA should be considered when:

- policy becomes too complex for YAML;
- many external identity groups exist;
- agent rules become dynamic;
- project/resource constraints become hard to express;
- RePlan becomes a server used by several teams.

RePlan should keep policy checks centralized so that replacing YAML with OPA is
possible without rewriting business logic.

## External auth integration

### CLI MVP

For local CLI:

- use OS user trust boundary;
- use local config;
- use explicit source tokens;
- use dry-run by default;
- write audit log.

### CLI against server

Later:

- use OIDC device code flow or browser login;
- store access token in OS keychain if available;
- support service account tokens for automation;
- support short-lived tokens where possible.

### Web UI

Do not implement passwords.

Initial self-hosted option:

```text
reverse proxy handles login
RePlan receives trusted headers
RePlan maps external identity/group claims to roles
RePlan enforces local policy
```

Later:

```text
OIDC provider
    -> RePlan validates JWT/access token
    -> RePlan checks capability and scope
```

### MCP or agent-facing API

MCP authorization is moving toward OAuth 2.1-style patterns for HTTP
transports. RePlan should not invent a separate agent auth system if it exposes
MCP-style APIs.

Design implication:

- RePlan agent API should be a resource server;
- external authorization server should issue tokens;
- RePlan should validate token claims;
- RePlan should enforce capabilities and scopes internally;
- agent operations should still be constrained by RePlan policy.

## Workload identity

For non-human workloads, RePlan should be compatible with workload identity
patterns.

Examples:

- sync worker;
- CI job;
- robot controller;
- lab machine controller;
- GPU reservation daemon;
- AI agent running in a sandbox;
- deployment automation.

Possible future mechanisms:

- service account token;
- mTLS client certificate;
- SPIFFE ID through SPIRE;
- cloud workload identity;
- Kubernetes service account token with external validation.

MVP does not need to implement these.

Design requirement:

```text
principal identity must not assume human user
```

## Non-human resources and gateways

RePlan should support resources that are not people.

Examples:

- GPU machine;
- physical robot;
- test stand;
- staging environment;
- deployment window;
- contractor team;
- AI agent pool.

These resources may require gateways.

Gateway examples:

- SSH gateway;
- Kubernetes API gateway;
- robot control API;
- lab reservation API;
- cloud GPU scheduler;
- CI runner pool;
- MCP server around a tool.

RePlan should not directly control arbitrary machinery in the core.

Instead:

```text
RePlan resource model
    -> gateway adapter
    -> external controller
```

Initial gateway contract:

- read availability;
- read current reservation;
- propose reservation;
- dry-run reservation;
- apply approved reservation;
- record evidence;
- emit audit event.

For dangerous physical resources, RePlan should require an external controller
with its own safety model. RePlan should not become the safety-critical control
system.

## Agent trust levels

Agents should not be treated as one category.

Suggested trust levels:

### level 0: read-only agent

Can:

- read assigned tasks;
- read public reports;
- summarize workload.

Cannot:

- propose changes.

### level 1: proposal agent

Can:

- propose estimates;
- propose comments;
- propose task decomposition;
- propose scenario changes.

Cannot:

- apply changes.

### level 2: bounded operator agent

Can:

- apply low-risk changes inside explicit policy;
- update its own task progress;
- attach evidence;
- mark proposal ready for review.

Cannot:

- change deadlines;
- change human assignments outside policy;
- change project status.

### level 3: trusted automation

Can:

- perform approved write-back;
- execute connector operations;
- sync external systems.

Requires:

- strong identity;
- audit log;
- narrow scope;
- revocable credentials.

Level 3 should be rare.

## Audit model

Even MVP should have an audit log.

It can be simple and local.

Record:

- timestamp;
- principal;
- actor;
- operation;
- target object;
- source snapshot;
- scenario ID;
- before values;
- after values;
- decision;
- approval reference;
- external write-back result.

Example:

```json
{
  "timestamp": "2026-07-02T16:30:00+07:00",
  "principal": "human:operator",
  "actor": "operator",
  "operation": "writeback.apply",
  "target": "openproject:1234",
  "scenario_id": "scenario-001",
  "decision": "approved",
  "before": {"start_date": "2026-07-06"},
  "after": {"start_date": "2026-07-10"}
}
```

Audit log should be append-only in spirit. It does not need enterprise
tamper-proofing in MVP.

## Data visibility

Not everyone should see everything.

Minimum design implication:

- every project has visibility scope;
- every task belongs to a project/source scope;
- every report is generated for a scope;
- every actor has allowed scopes;
- agents default to narrow scopes.

MVP may run trusted single-user and ignore enforcement in the CLI, but the data
model should still carry scopes.

## Minimal implementation without overhead

The smallest practical implementation:

1. Add `principal_id`, `actor_id`, `role`, `scope` concepts to schemas.
2. Use local policy YAML.
3. Add a central `authorize(action, target, context)` function.
4. Make all mutating commands call it.
5. Make all agent commands call it.
6. Log all proposal, approval and write-back operations.
7. Keep external auth outside RePlan.
8. Keep web UI private or behind reverse proxy.

This gives future-proofing without building enterprise auth.

## Explicit non-goals for MVP

Do not build:

- password login;
- organization management;
- team invitations;
- billing;
- SAML;
- multi-tenant database isolation;
- full policy UI;
- enterprise admin console;
- custom identity provider;
- fine-grained row-level permissions everywhere.

Do design for:

- external identity;
- scoped actors;
- scoped resources;
- capability checks;
- agent trust levels;
- gateway adapters;
- audit records.

## Final position

The correct balance is:

```text
do not build enterprise security now
do not design as if security will never matter
```

RePlan should begin as a local/self-hosted tool with external authentication and
minimal internal authorization. But its domain model should assume from day one
that humans, agents, services and non-human resources may all exist in the same
planning environment with different trust levels and scopes.
