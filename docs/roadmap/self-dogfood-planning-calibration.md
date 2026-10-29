# RePlan self-dogfood planning calibration

Date: 2026-07-26

Status: active planning work.

Related task: `task.self.calibrate-plan-and-resources`.

## Purpose

This document is the working plan for turning the current RePlan self-dogfood
fixture from a useful MVP dogfood model into a real operating plan.

The current project data is valuable, but it mixes several things:

- completed implementation work;
- synthetic dogfood tasks created to exercise RePlan features;
- real near-term product work;
- placeholder capacities;
- default resource assumptions;
- sanitized OpenProject fixture data;
- private/live OpenProject context that must not leak into the public
  repository.

The next step is not to add more features immediately. The next step is to
calibrate the plan and resource model so future RePlan work is planned from
agreed facts and explicit estimates.

## Output Of This Planning Work

The planning calibration is complete when we have:

- a cleaned high-level roadmap for RePlan;
- a precise near-term task list for the next work cycle;
- an explicit decision about which current tasks are real, deferred,
  already complete or synthetic dogfood scaffolding;
- a resource catalog that matches how work is actually done;
- weekly capacities for human attention, AI-agent work, subscriptions, quotas,
  local environment and release windows;
- a policy for uncertain estimates;
- a decision about how RePlan and OpenProject are used together during the next
  cycle;
- updated RePlan project data that reflects those decisions.

## Operating Rule

Do not silently rewrite the whole plan.

Planning changes should follow this order:

1. Discuss the current task inventory.
2. Decide what each task means.
3. Mark tasks as keep, defer, remove, split, merge or close.
4. Discuss the resource catalog.
5. Decide real resource capacities and units.
6. Choose the next 1 to 3 execution tasks.
7. Update RePlan data.
8. If needed, update the OpenProject fixture or live OpenProject plan.

The reason for this order is simple: resource allocations are meaningless if
the task list itself is not real.

## Step 1. Task Inventory

Goal: decide whether the current tasks are a real roadmap or dogfood
scaffolding.

For every current task, assign one of these planning decisions:

- `keep`: real work that remains part of the plan;
- `close`: already done and correctly represented;
- `defer`: real work, but not in the next cycle;
- `remove`: synthetic or obsolete task that should leave the active plan;
- `split`: task is too broad and must become smaller tasks;
- `merge`: task duplicates another task;
- `research`: task is not executable yet and needs a research/design step.

Initial known issue:

The current self-dogfood plan contains tasks that were added to exercise
features, reports and proposal flows. Those tasks were useful, but their
future status must be reviewed. They must not automatically become the real
product roadmap.

## Step 2. High-Level RePlan Roadmap

Goal: keep a broad plan for the whole project without pretending that distant
work is precisely estimable.

Recommended high-level blocks:

1. Stabilize internal dogfood plan and resource model.
2. Decide the next real operating substrate:
   OpenProject-first, RePlan-first, or a temporary mixed model.
3. Improve source synchronization only where it blocks dogfood.
4. Improve human observability only where it blocks decisions.
5. Improve proposal and mutation ergonomics only where repeated operations are
   painful.
6. Close OSS/private boundary issues before public release.
7. Prepare a public release story only after the public repository no longer
   contains private pilot narrative.
8. Revisit larger architecture topics after repeated real use: service/daemon
   mode, web UI, RBAC, hosted operation, deep OpenProject plugin integration,
   TaskJuggler integration and scheduling solver.

Distant tasks should be represented as roadmap blocks, not over-detailed fake
tasks with fake dates.

Decision, 2026-07-26:

The high-level roadmap blocks are accepted as the planning frame:

- Internal Dogfood Core;
- OpenProject Integration;
- Resource Planning Model;
- Tracking, Actuals and Evidence;
- Human Observability;
- Proposal and Mutation Ergonomics;
- OSS Boundary and Public Release;
- Future Architecture.

This means near-term planning should not jump directly to daemon mode, hosted
operation, full web UI, RBAC, deep OpenProject plugin work, TaskJuggler deep
integration or a scheduling solver unless one of the earlier blocks proves that
such work is the next bottleneck. The later 2026-08-02 priority reset raises
OpenProject-native surface strategy and first surface prototypes, but not a
large plugin implementation, into near-term planning.

## Step 3. Near-Term Work Cycle

Goal: choose the next 1 to 3 tasks that will actually be executed.

Candidate sources:

- current RePlan `next_actions`;
- the known-gaps register;
- OpenProject/RePlan synchronization friction;
- human observability friction;
- resource-planning friction;
- the product goal of making RePlan useful for operating RePlan itself.

Current candidates after the first dogfood cleanup:

- resolve or document `dependency.self-fixture-before-openproject-fixtures`;
- resolve or document `dependency.oss-audit-before-release-story`;
- review `proposal.increase-review-capacity.20260720`;
- recalibrate the reviewer-role resource;
- recalibrate local development environment usage;
- decide whether the next product slice is source synchronization, proposal
  ergonomics, dashboard/report usability, or roadmap cleanup.

Decision update, 2026-07-26:

The near-term operating mode is OpenProject-primary with RePlan as a resource
planning sidecar.

This changes the interpretation of the OpenProject fixture/synchronization
candidate. The task previously described as `Expand sanitized OpenProject
fixtures` is still not the current task while this calibration is in progress,
but it is no longer just a generic fixture-improvement idea. It is the likely
next source-integration block after calibration: align the real OpenProject task
source with RePlan sidecar data so task planning happens in OpenProject and
resource planning happens in RePlan.

## Step 4. Resource Catalog

Goal: make the resources real enough for planning.

Current resources:

| Resource | Current meaning | Calibration question |
| --- | --- | --- |
| `resource.human.maintainer` | Human project owner and maintainer time | How many focused hours per week are actually available for RePlan? |
| `resource.ai.codex.local` | Codex CLI agent work | Should this be tracked as agent-hours, subscription-limit budget, task count, session count, or a combined resource? |
| `resource.ai.review-assistant` | Possible AI reviewer | Is this a real near-term resource or only a future placeholder? |
| `resource.role.reviewer` | Abstract reviewer role | Which review work requires a separate role-capacity bucket, and which review is lightweight enough to stay inside normal maintainer work? |
| `resource.ci.github-actions` | CI minutes | Is this scarce enough to plan explicitly? |
| `resource.service.llm.tokens` | LLM API token budget | Does this represent API-token use only, or should subscription-based CLI agent limits be tracked separately? |
| `resource.environment.local-dev` | Local machine/dev environment | Does concurrent slot planning matter for one-person local work? |
| `resource.window.oss-release` | Release preparation window | Is public release in scope for the next horizon? |

Decision, 2026-07-26:

`resource.human.maintainer` is the project owner/operator. For planning, its
weekly capacity is calibrated to 10 focused hours per week. This is not total
calendar availability. It is the amount of attention that can realistically be
planned for RePlan work without making the schedule look healthier than it is.

Decision, 2026-07-26:

The old `resource.attention.review` model is replaced by the public-friendly
abstract role resource `resource.role.reviewer`.

This role represents review and acceptance capacity. It is not tied to a human
by name. In the current project the role is fulfilled by the maintainer, but a
future human reviewer or AI review agent may fulfill part of the same role.

The weekly capacity is calibrated to 2 `review_hours` per week for near-term
planning. This deliberately keeps review scarce. The planning answer should be
to use reviewer role capacity only for meaningful review and acceptance work,
not to attach review demand to every minor documentation or fixture update.

### Reviewer Demand Policy

Decision, 2026-07-30:

Use `resource.role.reviewer` only when the task needs a separate review and
acceptance capacity bucket.

Separate reviewer demand is required for:

- architecture decisions;
- resource model and planning model changes;
- OpenProject/RePlan synchronization behavior;
- public specifications that define user-visible contracts;
- OSS boundary, private-data separation and release-readiness decisions;
- release story and public launch preparation;
- risky code changes where a second pass is part of the intended workflow;
- proposal decisions that materially change the plan, capacity model or source
  integration behavior.

Separate reviewer demand is not required for:

- small documentation edits;
- mechanical fixture synchronization;
- routine smoke/test maintenance;
- internal working notes;
- minor dashboard copy or layout polish;
- accepted historical tasks where no future acceptance decision remains.

For interactive maintainer-plus-Codex work, the normal operator check is part of
`resource.human.maintainer` unless the task has real acceptance risk. This keeps
the model readable: reviewer capacity shows scarce acceptance work, not every
moment where the maintainer glances at the result.

Historical accepted tasks may keep their old reviewer demand rows for audit
traceability, but those demands must not keep future `remaining_amount`. If the
separate review was never a real independent capacity need, mark the demand
`cancelled` with `remaining_amount: 0`. If the separate review was actually
performed, mark it `consumed`.

The open proposal
`proposal.increase-review-capacity.20260720` is obsolete under this policy. The
chosen correction is to reduce artificial reviewer demand first, not to inflate
reviewer capacity from 2 to 10 `review_hours` per week.

Implementation note, 2026-07-30:

- accepted historical reviewer demands that no longer represent future
  acceptance work were set to `remaining_amount: 0`;
- artificial historical reviewer demands were marked `cancelled`;
- the completed first dogfood cleanup reviewer demand was marked `consumed`;
- `proposal.increase-review-capacity.20260720` was rejected through the normal
  proposal decision command, with an audit record;
- reviewer-role remaining workload for `2026-W31` dropped from 22 to 8
  `review_hours`;
- reviewer-role overload for `2026-W31` dropped from 20 to 6 `review_hours`.

The remaining reviewer overload is intentional signal, not cleanup noise. It
comes from the still-relevant near-term decision work: current plan/resource
calibration, OpenProject/RePlan source synchronization, OSS readiness audit and
public release story.

### Maintainer Demand And Planning Horizon

Decision, 2026-07-30:

`resource.human.maintainer` remains calibrated to 10 focused hours per week.
The maintainer is the primary operator, product decision maker, implementation
driver and acceptance owner while RePlan is still a small self-dogfood project.

The important planning rule is that a weekly workload report must not mix all
future backlog with the selected week. Otherwise every useful backlog item looks
like an immediate overload and the report stops helping the operator decide what
to do next.

Near-term horizon policy:

- the current week should contain the current active planning task;
- the next week should contain the next real integration slice;
- later OSS and release work should stay visible, but not consume the current
  week's maintainer capacity;
- accepted historical tasks may keep planned and actual values, but must not
  keep future `remaining_amount`;
- stale historical remaining work is planning debt and should be closed through
  the normal tracking/proposal/actual loop when possible.

Implementation note, 2026-07-30:

- `workload --week` now filters dated resource demands by task or demand date
  range intersection with the requested ISO week;
- demands with no usable dates remain included as unscheduled work instead of
  being silently hidden;
- dependency date-order validation now ignores resolved finish-to-start
  dependencies, because resolved dependencies are history rather than active
  planning blockers;
- the accepted self-fixture maintainer and Codex demands were closed with
  `remaining_amount: 0`;
- accepted historical CI, token and local-development-environment demands that
  no longer represent future work were closed with `remaining_amount: 0`;
- `task.self.calibrate-plan-and-resources` was moved into `2026-W31`;
- `task.self.expand-openproject-fixtures` was moved to `2026-W32`;
- `task.self.write-oss-readiness-audit` was moved to `2026-W33`;
- `task.self.prepare-release-story` was moved to `2026-W34`.

This makes `2026-W31` represent the current operator focus instead of the whole
remaining project backlog.

### Dependency Cleanup

Decision, 2026-07-30:

After calibrating the weekly planning horizon, dependency warnings should be
read as operational signals:

- if the predecessor condition is already satisfied, resolve the dependency
  through the proposal decision loop;
- if the dependency describes a real future gate, keep it open and visible;
- do not suppress real release gates just to make validation warning-free.

Implementation note, 2026-07-30:

- `dependency.self-fixture-before-openproject-fixtures` was resolved through a
  generated `mark_dependency_resolved` proposal because
  `task.self.create-replan-self-fixture` is accepted;
- `dependency.oss-audit-before-release-story` remains open intentionally,
  because the public release story should not be treated as unblocked before the
  OSS readiness audit is done;
- `proposal.dependency.dependency.oss-audit-before-release-story.review-dependency-resolution`
  remains open as the explicit future review marker for that release gate;
- dependency proposal generation no longer creates schedule-adjustment
  proposals for resolved finish-to-start dependencies.

### Calibration Closure

Decision, 2026-07-30:

`task.self.calibrate-plan-and-resources` is accepted.

The task closed the intended calibration scope:

- OpenProject-primary/RePlan-sidecar operating mode;
- high-level roadmap frame;
- maintainer capacity at 10 focused hours per week;
- reviewer role abstraction and reviewer demand policy;
- Codex subscription and limit-based planning model;
- weekly planning horizon behavior;
- dependency cleanup policy;
- next task selection.

Implementation note, 2026-07-30:

- remaining work for `resource_demand.plan-calibration.maintainer` was set to
  0 through `actual set-remaining`;
- remaining work for `resource_demand.plan-calibration.codex` was set to 0
  through `actual set-remaining`;
- remaining work for `resource_demand.plan-calibration.review` was set to 0
  through `actual set-remaining`;
- actual consumption records were added for 4 maintainer hours, 3 Codex
  `agent_hours` and 1 reviewer `review_hour`;
- `task.self.calibrate-plan-and-resources` was moved from `in_progress` to
  `accepted` through `actual add-status`;
- the next real execution slice is
  `task.self.expand-openproject-fixtures`, now interpreted as
  OpenProject/RePlan task-source synchronization calibration.

Next-block decision, 2026-07-30:

The OpenProject/RePlan task-source synchronization block is approved and
documented in
[`openproject-replan-sync-plan.md`](openproject-replan-sync-plan.md).

The accepted sequence is:

```text
sanitized fixture -> stable source contract -> live validation -> public-safe
fixture or mapper update
```

The reason is practical:

- sanitized fixtures keep the OSS repository safe and testable;
- live validation proves the fixture is not a toy model;
- private tokens, private URLs and private pilot narrative remain outside the
  public repository;
- any useful live finding must be converted into a public-safe fixture,
  mapper change or documented limitation.

Observed product gap and resolution:

The actual consumption records are visible through `actual list`, but
`workload` still reports `actual: 0` because workload aggregation currently
uses `resource_demands.actual_amount` instead of summing canonical actual
records. This was recorded as `GAP-014` in the known-gaps register and closed
by updating workload aggregation to sum canonical `work_consumed` actual
records for the requested week.

## Step 5. Codex Subscription And Limit-Based Resources

Codex is not a simple hourly employee resource.

It has several different constraints:

- human operator attention needed to direct and review the agent;
- productive agent work performed during a task;
- wall-clock session time;
- optional parallel agent/subagent pressure;
- subscription limits that reset on provider-defined windows;
- short rolling-window limits where the provider/account currently exposes
  them;
- weekly limits where the provider/account currently exposes them;
- exceptional reset credits or manual resets;
- unknown or unobservable provider-side consumption formulas.

This means a single number such as `40 agent_hours/week` is not a real
subscription model. It is only a coarse planning proxy.

### Decision, 2026-07-26

Use a two-layer model for Codex:

1. `resource.ai.codex.local` remains the execution resource.
2. Codex subscription/limit pressure is a separate conceptual constraint that
   may become its own resource later, but is not yet encoded as a fake numeric
   capacity in the public self-dogfood fixture.

`resource.ai.codex.local` should be read as:

```text
How much useful agent work do we expect this task to require?
```

It should not be read as:

```text
How much of the OpenAI subscription limit will this task consume exactly?
```

Near-term units:

- `agent_hours`: rough expected active agent work, used for planning and later
  calibration from actual task history;
- `operator_hours`: human maintainer time needed to direct the agent and make
  decisions, represented by `resource.human.maintainer`;
- `review_hours`: reviewer role capacity, represented by
  `resource.role.reviewer`;
- `subscription_limit_pressure`: not implemented as a canonical unit yet;
- `reset_credits`: live/private account state, not a stable public fixture
  value.

The current `capacity.codex.agent-hours.weekly = 40` remains provisional. It is
not a claim about the real subscription limit. It only says that, under the
current coarse model, Codex execution is not the near-term bottleneck compared
with maintainer attention and reviewer role capacity.

### Rolling Windows

Codex-like subscription resources can have more than one reset window.

Observed/expected examples:

- weekly limit;
- five-hour rolling limit;
- provider changes that remove or add one of those windows;
- manual or gifted reset credits;
- temporary account-specific exceptions.

Planning rule:

- model the weekly limit as the dominant capacity window when that is the only
  reliable/current observable limit;
- treat short rolling windows as availability interruptions rather than as
  normal weekly capacity until they are observable enough to track;
- if a limit is exhausted, affected AI-agent work becomes blocked until the
  relevant reset window renews or a reset credit is used;
- do not encode account-specific reset-credit state in public OSS fixtures;
- record reset-credit use as private/live evidence if it becomes operationally
  important.

### Consumption Speed

The hardest part is not representing a limit. The hardest part is forecasting
how quickly a task consumes it.

Different sessions have very different burn rates:

- planning-heavy sessions where the human writes a lot may consume relatively
  little agent-side work;
- implementation sessions with many file reads, edits, tests and retries may
  consume more;
- research sessions with many web searches can be bursty;
- parallel subagent reviews or research can consume the resource very quickly;
- long context sessions can become more expensive even when the visible task
  feels small.

Near-term estimation rule:

- estimate Codex demand probabilistically;
- keep one planned `agent_hours` value as the current expected value;
- write low/likely/high notes in the planning discussion for uncertain tasks;
- after task completion, record actual observed agent effort manually;
- over time, compare planned vs actual and use that history to calibrate future
  estimates;
- when subscription exhaustion happens, record it as evidence because it is
  high-value calibration data.

### Future Tracker

A later Codex limit tracker would be useful, but it is not required for the
current planning cycle.

Possible tracker inputs:

- current weekly remaining limit if visible;
- current short-window remaining limit if visible;
- reset window timestamps;
- reset credits available;
- active sessions;
- task/session association;
- manually recorded exhaustion events;
- optionally imported provider/account telemetry if available.

Possible tracker outputs:

- `available`;
- `near_limit`;
- `blocked_until_reset`;
- `reset_credit_available`;
- `high_burn_session`;
- `forecast_limit_risk`.

This tracker should be designed as optional live/private state. Public RePlan
fixtures should demonstrate the model without leaking personal subscription
state.

## Step 5A. Resource Pool Review Before OpenProject Bootstrap

Decision date: 2026-07-30.

Before creating a new live OpenProject project for the current RePlan
self-dogfood plan, the operator raised a resource-pool question:

```text
Have we actually agreed the resources that will be assigned to the project, or
have we only discussed maintainer, reviewer and Codex?
```

The answer is:

```text
OpenProject bootstrap does not require a final resource model, but it does
require an explicit resource-pool review so we do not accidentally treat
placeholder resources as committed operational capacity.
```

Reason:

- OpenProject owns task skeletons, human task views, dates, statuses and later
  relations;
- RePlan owns the resource sidecar;
- therefore OpenProject can be created from the current task skeleton while
  resource details continue to evolve in RePlan;
- however, the operator needs to know which resources are core, provisional,
  future placeholders or pending a decision before using the plan for real
  scheduling decisions.

Current resource-policy buckets:

| Bucket | Resource | Decision |
| --- | --- | --- |
| Core now | `resource.human.maintainer` | Keep. This is the human project owner/operator and the main scarce resource. |
| Core now | `resource.ai.codex.local` | Keep. This is useful agent work, not an exact subscription-limit counter. |
| Core now | `resource.role.reviewer` | Keep. This is an abstract reviewer capacity bucket that may later be filled by the maintainer, another human or an AI reviewer. |
| Future placeholder | `resource.ai.review-assistant` | Keep, but do not use as current committed capacity until there is a real review-agent workflow. |
| Provisional side constraint | `resource.ci.github-actions` | Keep, but recalibrate later; useful for showing CI as a constrained non-human resource. |
| Provisional side constraint | `resource.service.llm.tokens` | Keep, but recalibrate later; this represents API-token budget, not Codex subscription limits. |
| Provisional side constraint | `resource.environment.local-dev` | Keep, but treat as coarse; for one-person work it rarely blocks except when a process, server or test environment occupies the local slot. |
| Provisional side constraint | `resource.window.oss-release` | Keep, but treat as a release-planning example until public release becomes the current bottleneck. |

New helper:

```sh
./scripts/review-resource-pool.sh
```

The helper is read-only and emits JSON. It summarizes:

- resource count;
- capacity count;
- availability count;
- demand count;
- actual count;
- resource bucket;
- demand and actual usage by unit;
- current issues;
- whether OpenProject task-skeleton bootstrap can proceed.

Resource review gate for OpenProject bootstrap:

```text
can_create_task_skeleton: true, unless the helper reports blocking issues
```

Must decide before OpenProject apply:

- whether to create a new OpenProject project such as `replan-self`;
- whether maintainer-only assignment is acceptable for the first OpenProject
  task skeleton;
- whether live source refs should later be accepted into public canonical files
  or stored in a private binding layer.

Decisions accepted on 2026-07-30:

- create/use a new live OpenProject project with identifier `replan-self`;
- assign first-pass OpenProject tasks to the maintainer/operator where
  assignment is needed;
- store live OpenProject work-package/user/relation bindings in an ignored
  private binding layer, not in public canonical files.

Private binding example:

```text
private/openproject/replan-self-bindings.yaml
```

Committed public shape example:

```text
examples/openproject/private-bindings.example.yaml
```

Can defer until after OpenProject bootstrap:

- Codex subscription limit tracker;
- exact reviewer implementation;
- whether CI, token budget, local-development-environment and release-window
  resources stay active planning constraints or only public examples.

## Step 5B. Resource Demand Calibration Planner

Decision date: 2026-08-02.

After OpenProject and RePlan task, status, date and dependency drift are
aligned, the next resource-planning risk is not source synchronization. The
risk is stale resource demand lifecycle data.

Current observed state:

- OpenProject/RePlan task-source drift can be aligned through the live drift
  helpers;
- `scripts/check-resource-schedule-drift.sh` reports
  `resource_schedule_status: needs_calibration`;
- the report has no hard resource schedule drift;
- active resource demands still exist on accepted tasks;
- active resource demands without `remaining_amount` make workload planning
  fall back to coarse planned amounts;
- maintainer and reviewer overload rows are therefore not yet reliable enough
  for commitment decisions.

Backlog task:

```text
Add a read-only resource demand calibration planner before mutating resource
demands.
```

Implemented helper:

```sh
./scripts/plan-resource-demand-calibration.sh
```

The helper is intentionally read-only. It does not edit canonical YAML, does not
write proposals and does not infer final business decisions. It emits JSON
candidate actions for operator review.

The planner distinguishes:

- active demands attached to accepted tasks;
- active open-task demands without `remaining_amount`;
- closed demands attached to tasks that are still open;
- demands that have accepted actual records;
- demands that still need an operator decision because actual evidence is not
  enough to decide automatically.

Policy:

- accepted tasks should not keep active future resource load;
- an active demand on an accepted task should be reviewed and then either
  marked consumed, marked cancelled, or the task should be reopened if
  acceptance was wrong;
- `remaining_amount: 0` is not the same thing as closing the demand lifecycle;
- if a demand remains `active`, reports may still treat it as planning load;
- therefore the operator must decide demand lifecycle, not only remaining
  amount;
- active demands on non-accepted tasks should get explicit
  `remaining_amount` before workload is used for real week-by-week decisions.

Current observed planner result on 2026-08-02:

- `calibration_plan_status`: `review_required`;
- resource demands: 65;
- actual records: 39;
- candidate actions: 32;
- candidate action type: `active_demand_on_accepted_task`.

Immediate next operator sequence:

1. Review `active_demand_on_accepted_task` candidates first.
2. For each candidate, decide `consumed`, `cancelled` or reopen task.
3. Use the audited demand lifecycle mutation path:
   `replan actual close-demand --status consumed|cancelled`.
4. Rerun `scripts/check-resource-schedule-drift.sh`.
5. Only then review weekly overload rows as real planning pressure.

Design implication:

RePlan needs an explicit, audited resource-demand lifecycle mutation path. The
implemented `actual close-demand` command closes this gap for the first
operator loop by supporting reviewed transitions to `consumed` or `cancelled`.
The existing `actual set-remaining` and `update_remaining_work` proposal path
still close remaining work, but they are not a full substitute for deciding
whether a demand is `active`, `consumed` or `cancelled`.

Follow-up backlog task, 2026-08-02:

```text
Apply reviewed lifecycle closure to the 32 accepted-task resource demand
candidates that have accepted actual records.
```

Accepted batch policy:

- if the task is already `accepted`;
- and the demand is still `active`;
- and an actual record exists for that demand;
- then close the demand as `consumed` through
  `replan actual close-demand --status consumed`;
- do not hand-edit `canonical/resource_demands.yaml`;
- rerun resource calibration and resource schedule drift reports after the
  batch.

Implementation note, 2026-08-02:

- 32 accepted-task resource demands were closed through
  `replan actual close-demand --status consumed`;
- no resource demand YAML was edited by hand;
- 32 `actual_close_demand` audit records were appended to
  `examples/replan-self/audit/audit.2026-08.jsonl`;
- `plan-resource-demand-calibration` now returns `aligned` with
  `candidate_actions: 0`;
- `check-resource-schedule-drift` now has 0 hard drifts and 0 calibration
  reviews;
- `check-resource-schedule-drift` still returns `needs_calibration` because
  reviewer-role capacity is overloaded in two weekly rows:
  `2026-W30` by 0.8571 `review_hours` and `2026-W31` by 1.2857
  `review_hours`.

Resulting next decision:

The stale accepted-task demand cleanup is closed. The remaining resource issue
is no longer demand lifecycle drift; it is whether reviewer-role capacity of 2
`review_hours` per week is intentionally strict or should be replanned.

Decision, 2026-08-02:

Keep reviewer-role capacity strict at 2 `review_hours` per week.

Reason:

- reviewer capacity is meant to stay a scarce acceptance signal;
- increasing capacity only to make the report green would weaken the planning
  model;
- current OpenProject/RePlan sync review remains the near-term review priority;
- release-oriented review work should move later instead of competing with the
  current integration slice.

Follow-up backlog task, 2026-08-02:

```text
Explain reviewer-role overload before changing reviewer capacity or moving
tasks.
```

Implemented helper:

```sh
./scripts/explain-resource-overload.sh
```

The helper is read-only and defaults to:

```text
resource_id = resource.role.reviewer
unit = review_hours
```

It explains overloaded weekly rows by listing the active resource demands that
contribute to each overload.

Current observed reviewer overload explanation:

- overload status: `overloaded`;
- weekly rows: 3;
- overloaded weekly rows: 2;
- skipped demands: 0.

Overloaded weeks:

| Week | Capacity | Planning load | Overload | Main contributors |
| --- | ---: | ---: | ---: | --- |
| `2026-W30` | 2.0 | 2.8571 | 0.8571 | `resource_demand.oss-audit.review` contributes 1.7143; `resource_demand.openproject-fixtures.review` contributes 1.1429. |
| `2026-W31` | 2.0 | 3.2857 | 1.2857 | `resource_demand.release-story.review` contributes 2.1429; `resource_demand.openproject-fixtures.review` contributes 0.8571; `resource_demand.oss-audit.review` contributes 0.2857. |

Interpretation:

- the remaining overload is not stale accepted-task load;
- it comes from three still-active review demands;
- `resource_demand.openproject-fixtures.review` belongs to the current
  OpenProject/RePlan sync work;
- `resource_demand.oss-audit.review` and `resource_demand.release-story.review`
  are future/public-release oriented;
- the next operator decision is whether to keep reviewer capacity strict at 2
  `review_hours` per week and defer/cancel some review demand, or recalibrate
  reviewer capacity upward.

Accepted resolution:

- keep `resource_demand.openproject-fixtures.review` in the current
  OpenProject/RePlan synchronization window;
- move `task.self.write-oss-readiness-audit` to
  `2026-08-03` through `2026-08-09`;
- move `task.self.prepare-release-story` to
  `2026-08-10` through `2026-08-23`;
- keep reviewer capacity at 2 `review_hours` per week;
- do not cancel review demand and do not increase capacity.

Implementation note, 2026-08-02:

- added scenario `scenario.reviewer-capacity-strict-replan`;
- applied it through `replan scenario apply` with actor
  `actor.human.maintainer`;
- changed four task date fields through the scenario apply path;
- appended a `scenario_apply` audit record to
  `examples/replan-self/audit/audit.2026-08.jsonl`;
- `check-resource-schedule-drift` now returns `aligned`;
- `explain-resource-overload` now returns `aligned` with
  `overloaded_weekly_rows: 0`.

Follow-up boundary:

This scenario changed RePlan task dates. Because the near-term operating mode
is OpenProject-primary for human task planning, the next OpenProject sync check
must verify whether live OpenProject should receive the same date movement or
whether OpenProject already contains equivalent scheduling changes.

Tracking:

- the reviewed RePlan-to-OpenProject date write-back design and backlog are
  recorded in
  [`openproject-replan-sync-plan.md`](openproject-replan-sync-plan.md) as
  `OP-RP-DATE-PUSH-*`.

Resolution, 2026-08-02:

- RePlan pushed the reviewed date changes to live OpenProject through the
  reviewed date write-back helper;
- OpenProject normalized Sunday finish dates to the next Monday;
- RePlan accepted those normalized finish dates back into canonical
  `planned_finish`;
- OpenProject/RePlan drift returned `aligned`;
- resource schedule drift remained `aligned`;
- the local operator dashboard and Gantt were regenerated to show the aligned
  schedule, including the `2026-08-03` to `2026-08-10` OSS readiness audit and
  the `2026-08-10` to `2026-08-24` release story window.

## Step 6. Probabilistic Estimates

Exact estimates are not realistic at this stage.

Use ranges and confidence levels conceptually, even if the current schema only
stores one planned amount.

Recommended estimation practice:

- use a single planned value as the current expected value;
- record low/likely/high estimates in the planning notes where uncertainty is
  high;
- treat first estimates as hypotheses;
- compare planned, actual and remaining after each task;
- recalibrate future tasks from observed deltas;
- separate uncertainty caused by unknown scope from uncertainty caused by
  resource throughput.

Example:

```text
Task: improve proposal batch ergonomics
Maintainer: low 1 h, likely 2 h, high 4 h
Codex: low 2 agent-hours, likely 4 agent-hours, high 8 agent-hours
Confidence: medium
Reason: existing proposal code is known, but command contract may need design.
```

Future schema option:

Add explicit estimate fields:

- `estimate_low`;
- `estimate_likely`;
- `estimate_high`;
- `confidence`;
- `estimation_basis`;
- `calibration_source_refs`.

Do not add those fields until the simpler current model becomes painful.

## Step 7. RePlan And OpenProject Alignment

Goal: decide what is primary during the next cycle.

Possible operating modes:

### Mode A. OpenProject primary, RePlan sidecar

OpenProject owns project tasks and human-facing PM views. RePlan owns resource
demands, capacities, workload, tracking suggestions and proposal logic.

Best when:

- OpenProject is actively used by humans;
- task state is edited in OpenProject;
- RePlan mostly enriches OpenProject with resource planning.

Risk:

- public RePlan repository can accidentally pick up private OpenProject pilot
  details.

Decision, 2026-07-26:

Mode A is accepted as the near-term operating mode.

In this mode:

- OpenProject is the primary source of truth for project tasks, statuses,
  task-level planning, task dependencies and human PM views;
- RePlan is the sidecar layer for resource demands, capacities, resource
  pressure, AI-agent resources, subscription and quota constraints,
  probabilistic estimates, actuals/evidence enrichment and proposal-based safe
  mutations;
- RePlan remains PM-agnostic internally by keeping source references instead of
  embedding OpenProject-only task assumptions into the core model;
- new real project tasks should be created or confirmed in OpenProject first,
  then reflected in RePlan through import/source references;
- RePlan-side resource changes may stay in RePlan only unless a specific
  OpenProject write-back rule exists;
- private live OpenProject operational details must stay out of the public
  RePlan repository.

Short form:

```text
OpenProject = what work exists and how humans view task planning.
RePlan = who/what can do it, with which capacity, limits and overloads.
```

### Mode B. RePlan primary, OpenProject fixture mirror

RePlan owns the dogfood project while OpenProject is represented by sanitized
fixtures or occasional import/export tests.

Best when:

- the product itself is still changing quickly;
- we want to keep public dogfood data OSS-safe;
- OpenProject integration is not the immediate bottleneck.

Risk:

- less pressure on the live OpenProject integration.

### Mode C. Mixed temporary mode

RePlan is primary for product/resource planning; OpenProject is used for
selected human views and synchronization experiments.

Current status:

Mode C is not the selected near-term operating mode. It remains a fallback if
live OpenProject workflow or OSS/private repository boundaries block Mode A in
practice.

## Step 8. Discussion Sequence

Use this sequence in conversation:

1. Confirm high-level roadmap blocks.
2. Review current tasks one by one.
3. Decide what remains in the next-cycle backlog.
4. Review resources one by one.
5. Set practical capacities.
6. Decide how Codex subscription/limits are represented.
7. Choose the next 1 to 3 execution tasks.
8. Update RePlan data.
9. Regenerate reports and dashboard.
10. Compare RePlan state with OpenProject state.

## Initial Recommendation

Do not start by editing every current task.

Start with high-level roadmap blocks, then calibrate the resource catalog, then
choose a short near-term cycle. After those decisions, update the YAML and the
OpenProject fixture in one intentional pass.

This avoids the worst failure mode: making the fake precision of the current
fixture look more authoritative than the actual project decisions.

## Step 9. Backlog Operating Convention And Priority Reset

Decision date: 2026-08-02.

The next RePlan work should not live only in chat or prose documents.

Accepted convention:

- agreed non-trivial future work must be represented in the OpenProject/RePlan
  dogfood backlog;
- OpenProject is the human-visible task backlog, status, relation and Gantt
  surface;
- RePlan is the resource, capacity, scenario, evidence, proposal, actuals and
  audit sidecar;
- documentation records decisions and rationale, but it is not enough by itself
  for accepted execution work;
- if live OpenProject cannot be reached, the task may be recorded with
  OpenProject sync pending and synchronized later.

Priority reset:

- public release story is deferred;
- OSS preparation remains useful now, but only as an OSS boundary checkpoint,
  not as a launch/release-story push;
- the next main product proof is a full OpenProject-centered dogfood cycle:
  planning, resource check, execution, actuals, analysis and next decision;
- OpenProject-native RePlan surface work is raised to near-term priority,
  including staged design from links/adjacent dashboard toward possible
  OpenProject plugin, tab or panel work.

Immediate planning implication:

The next backlog update should create or update OpenProject/RePlan tasks for:

- OSS boundary checkpoint;
- full OpenProject-centered dogfood cycle;
- OpenProject-native RePlan surface strategy;
- first OpenProject-native surface prototype level;
- dashboard resource-load semantics cleanup.

## Step 10. Backlog Materialized In OpenProject And RePlan

Decision date: 2026-08-02.

The priority reset from Step 9 has been materialized in the OpenProject/RePlan
dogfood backlog.

RePlan canonical changes:

- `task.self.write-oss-readiness-audit` now represents `Run OSS boundary
  checkpoint`, not a public release push;
- `task.self.run-full-openproject-dogfood-cycle` was added for the first full
  OpenProject-centered planning, resource check, execution, actuals, analysis
  and next-decision loop;
- `task.self.design-openproject-native-surface` was added to design the path
  from links and adjacent dashboards toward a possible OpenProject plugin, tab
  or panel;
- `task.self.prototype-openproject-native-surface-level-0-1` was added for the
  first practical OpenProject-native observability surface;
- `task.self.reconcile-dashboard-load-semantics` was added to make daily
  resource-load markers consistent with the authoritative weekly or
  period-based resource schedule semantics;
- `task.self.prepare-release-story` remains in the plan, but is deferred until
  the boundary checkpoint, full dogfood cycle and OpenProject-native surface
  work provide enough proof.

OpenProject live changes:

- existing work package `81` was updated to `Run OSS boundary checkpoint`;
- existing work package `82` remains `Prepare public release story` and was
  deferred;
- new work packages `83`, `84`, `85` and `86` were created for the four new
  dogfood tasks;
- seven new OpenProject relations were created for the added dependency chain;
- work package `80`, `Calibrate OpenProject/RePlan task-source sync`, was
  closed after the live backlog materialization and drift checks aligned.

Resource-planning result:

- the new near-term plan fits the current coarse capacities;
- `resource.human.maintainer` remains modeled at `10 hours` per week;
- `resource.role.reviewer` remains modeled at `2 review_hours` per week;
- `resource.ai.codex.local` remains modeled as `agent_hours`, with subscription
  and limit tracking still represented as a planning-calibration concern rather
  than a precise quota meter;
- no resource overload is reported by `check-resource-schedule-drift.sh`.

Validation result:

- RePlan validation passes;
- OpenProject/RePlan task and dependency drift is aligned after the status push;
- private live bindings contain `26` task bindings and `29` dependency
  bindings;
- public canonical task source references remain sanitized and use
  `op.example.internal` example identifiers.

Current next work:

The next real execution block is `task.self.write-oss-readiness-audit`, now
meaning `Run OSS boundary checkpoint`.

It should be treated as a bounded OSS/private-boundary check, not as a public
release launch. The full OpenProject-centered dogfood cycle starts only after
this checkpoint is accepted.

## Step 11. OSS Boundary Checkpoint Result

Decision date: 2026-08-02.

The OSS boundary checkpoint is recorded in
[`../dev/oss-boundary-checkpoint-2026-08-02.md`](../dev/oss-boundary-checkpoint-2026-08-02.md).

Result:

- safe enough to continue private incubation and OpenProject-centered dogfood;
- not ready for direct public OSS release;
- no committed live token values were found;
- `private/`, `tmp/` and generated report artifacts are not tracked;
- `examples/replan-self` uses sanitized OpenProject source references;
- public release remains blocked by missing license/contribution/security files,
  private pilot narrative in public docs, old pilot-specific sidecars and the
  unresolved public history strategy.

Planning decision:

- mark `task.self.write-oss-readiness-audit` accepted for dogfood purposes;
- resolve `dependency.oss-checkpoint-before-full-dogfood`;
- keep release-story dependencies open;
- insert an OpenProject preflight infrastructure build before the full
  OpenProject-centered dogfood cycle;
- treat the previous OpenProject dependency-driven schedule as superseded by
  the 2026-08-04 preflight decision below.

## Step 12. OpenProject Preflight Infrastructure Build

Decision date: 2026-08-04.

The next execution block is not the full dogfood cycle itself. The next
execution block is a short OpenProject preflight infrastructure build.

Reason:

OpenProject is now the primary human-facing task source for RePlan work, but
the current agent/operator path still has friction. Agents can inspect RePlan
YAML and reports easily, while live OpenProject changes still require careful
manual API work, private bindings, `lockVersion`, relation direction checks,
date normalization awareness and drift verification. If the full dogfood cycle
starts before this friction is reduced, the project risks accumulating duplicate
work packages, accidental live drift or a habit of bypassing OpenProject when
the agent finds it inconvenient.

Approved preflight backlog:

1. `task.self.reconstruct-openproject-functionality-and-pitfalls`
   reconstructs OpenProject functionality, API behavior, operator friction and
   technical pitfalls already found in the pilot and self-dogfood sessions.
2. `task.self.evaluate-openproject-agent-interfaces` compares OpenProject MCP,
   the official OpenProject CLI, direct API usage and RePlan-owned wrapper
   commands for AI CLI agent work.
3. `task.self.audit-openproject-agent-tools-hands-on` installs or inspects the
   official OpenProject CLI, tests read-only live CLI commands, checks the
   current live MCP endpoint and extracts reusable API patterns before the
   operating-layer design is finalized.
4. `task.self.design-openproject-ai-cli-operating-layer` designs the narrow
   guarded operating layer for safe OpenProject inspection and mutation by
   humans and AI CLI agents.
5. `task.self.implement-openproject-preflight-operator-helpers` implements the
   minimum helper surface needed before touching the live plan.
6. `task.self.design-openproject-work-package-operator-toolkit` designs the
   fuller OpenProject work-package manipulation toolkit needed because the
   preflight report gives visibility but does not provide enough safe task
   editing capability.
7. `task.self.implement-openproject-work-package-operator-toolkit` implements
   the bounded toolkit for inspecting, listing and safely updating OpenProject
   work packages without raw API calls or delete-and-recreate shortcuts.
8. `task.self.refactor-live-replan-self-openproject-plan` uses those helpers to
   clean up the live `replan-self` OpenProject project without deleting and
   recreating existing work packages.

Scheduling decision:

- preflight reconstruction: `2026-08-04`;
- agent-interface evaluation: `2026-08-05`;
- hands-on OpenProject agent tools audit: `2026-08-06`;
- AI CLI operating-layer design: `2026-08-07`;
- preflight helper implementation: `2026-08-10`;
- work-package operator toolkit design: `2026-08-11`;
- work-package operator toolkit implementation: `2026-08-12` through
  `2026-08-13`;
- live `replan-self` plan refactor: `2026-08-14`;
- full OpenProject-centered dogfood cycle moves to start on `2026-08-17`.

Resource decision:

- preflight work consumes the existing maintainer, reviewer-role, Codex,
  local-development-environment and CI resources;
- no new resource class is introduced only for OpenProject preflight;
- OpenProject itself remains the task source, not a RePlan resource;
- private OpenProject endpoint, work-package ids and relation ids stay in the
  ignored private binding layer.

Safety rule:

The live `replan-self` project should be refactored in place. Deleting and
recreating work packages or projects is not the normal workflow. Any destructive
operation would require a separate explicit decision, separate backlog entry and
separate apply gate.

## Step 13. OpenProject Work-Package Toolkit Design Result

Decision date: 2026-08-04.

The preflight operator report closed the visibility gap, but it did not close
the manipulation gap. The project therefore inserts a full OpenProject
work-package operator toolkit before live plan refactor.

Design result:

- documented in
  [`../dev/openproject-work-package-operator-toolkit-design.md`](../dev/openproject-work-package-operator-toolkit-design.md);
- OpenProject API v3 remains the implementation substrate;
- official OpenProject CLI is used as a reference for command shape,
  profiles/env auth and fetch-before-patch `lockVersion` behavior;
- representative third-party MCP clients are used as references for
  fail-closed scope, preview/confirm behavior, compact output, field hiding,
  form validation, retry/backoff and prompt-injection boundaries;
- official or third-party MCP remains optional, not a required runtime
  dependency;
- direct raw API calls remain an emergency/manual path, not the normal agent
  workflow.

New implementation gate:

`task.self.implement-openproject-work-package-operator-toolkit` must be
accepted before `task.self.refactor-live-replan-self-openproject-plan` starts.

## Step 14. OpenProject Work-Package Toolkit Implementation Result

Decision date: 2026-08-04.

The OpenProject work-package manipulation gap is closed for the next dogfood
slice by a transitional script:

```sh
./scripts/openproject-work-package-toolkit.sh
```

Implemented command families:

- `work-package list`;
- `work-package inspect`;
- `work-package update-status`;
- `work-package update-dates`;
- `work-package update-metadata`;
- `status list`;
- `type list`;
- `user list`;
- `priority list`;
- `relation list`;
- `comment add`.

The implementation keeps the design constraints:

- JSON output is stable enough for AI CLI agents;
- read commands work against sanitized fixtures and live OpenProject;
- write commands are dry-run by default;
- live apply requires `REPLAN_APPLY=1`;
- fixture mode cannot apply mutations;
- task updates use private bindings when called by RePlan task id;
- live updates fetch the current work package and use `lockVersion`;
- status updates check allowed transition hrefs through the OpenProject form
  endpoint;
- user-authored OpenProject text is labeled as data under `user_authored`.

This is intentionally not yet the final `replan openproject ...` CLI surface.
It is the approved implementation bridge that removes the immediate need for
raw API calls during the live `replan-self` plan refactor.

## Step 15. Live OpenProject Plan Refactor Result

Decision date: 2026-08-04.

The live `replan-self` OpenProject project was reviewed after the preflight
helpers and work-package operator toolkit were available.

Result:

- no delete-and-recreate action was used;
- no work package identity needed to be changed;
- no relation identity needed to be changed;
- no task title drift remained;
- no status drift remained;
- no date drift remained;
- no relation tuple drift remained;
- live OpenProject had 34 mapped work packages;
- live OpenProject had 40 mapped relations;
- RePlan had 34 current-plan tasks;
- RePlan had 40 canonical dependencies;
- strict drift check returned `aligned`;
- operator report returned `aligned`;
- work-package toolkit list returned 34 mapped and 0 unmapped work packages;
- work-package toolkit relation list returned 40 bound relations.

Interpretation:

The live refactor pass became a verification-and-closure pass rather than a
large mutation pass. This is acceptable because the previous preflight and
toolkit slices had already moved the project into the desired structure. The
important product result is that the project can now be operated through the
guarded OpenProject/RePlan loop without raw API edits as the normal path.

The dependency
`dependency.openproject-live-refactor-before-full-dogfood` is therefore
resolved, and the next project step is the full OpenProject-centered dogfood
cycle.

## Step 16. Full OpenProject-Centered Dogfood Cycle Kickoff

Decision date: 2026-08-04.

The full OpenProject-centered dogfood cycle is now started.

Live kickoff actions:

- OpenProject work package for
  `task.self.run-full-openproject-dogfood-cycle` was moved from `New` to
  `In progress` through `scripts/openproject-work-package-toolkit.sh`;
- the toolkit dry-run was reviewed before apply;
- a kickoff comment was added to the OpenProject work package through the same
  toolkit;
- RePlan canonical task status was updated to `in_progress`;
- RePlan audit records the start of the cycle.

Kickoff checks:

- strict OpenProject/RePlan drift check: `aligned`;
- operator report: `aligned`;
- task bindings: 34 mapped, 0 missing, 0 extra;
- relation bindings: 40 mapped, 0 missing, 0 extra;
- resource schedule drift: `aligned`;
- active resource demands after previous closures: 15;
- open RePlan warnings are expected future dependency gates, not current drift.

Planning interpretation:

The cycle is not complete. It has only entered execution.

The next decision inside the cycle is to choose the first concrete work slice
that will prove the loop end to end:

1. take a task from OpenProject;
2. inspect RePlan resource demands and remaining work;
3. execute a bounded change;
4. record actuals and evidence;
5. compare plan versus actual;
6. make a follow-up planning decision.

The formal planned window remains `2026-08-17` through `2026-08-28` until a
separate schedule decision changes OpenProject dates. The early kickoff is
recorded as operational start, not as an automatic schedule rewrite.

## Step 17. OpenProject-native RePlan Surface Design

Decision date: 2026-08-04.

The first concrete work slice inside the full OpenProject-centered dogfood
cycle is `task.self.design-openproject-native-surface`.

This deliberately changes the earlier planning interpretation. The old
dependency text treated OpenProject-native surface design as work that should
happen only after the full dogfood cycle was accepted. In practice, the first
dogfood cycle immediately exposed a human-observability problem: the operator
can see tasks and Gantt in OpenProject, but the RePlan resource layer is still
adjacent and harder to compare.

The dependency is therefore treated as an in-cycle context gate rather than a
post-cycle blocker. The full dogfood cycle has started, and the surface design
is one of the first slices needed to make that cycle usable.

Accepted direction:

- OpenProject remains the primary human task surface;
- RePlan remains the resource-planning, evidence, actuals, warning and proposal
  sidecar;
- the immediate surface is Level 0 and Level 1, not a full OpenProject plugin;
- Level 0 means stable task identity, links and safe helper usage;
- Level 1 means adjacent read-only generated RePlan dashboard and Gantt-like
  resource views that can be compared with OpenProject;
- Level 2 controlled OpenProject write-back remains allowed through guarded
  dry-run/apply helpers;
- Level 3 OpenProject plugin or embedded panel is a near-term product concern,
  but should be designed after the Level 0 and Level 1 prototype proves which
  fields are actually needed inside OpenProject;
- broad automatic bidirectional synchronization remains out of scope.

The design result is recorded in
`docs/dev/openproject-native-replan-surface-design.md`.

Planning result:

- `task.self.design-openproject-native-surface` is accepted;
- the planned design estimate was intentionally left visible for calibration;
- actual consumption was below the original estimate, which is useful evidence
  for later resource-planning calibration;
- the next task is
  `task.self.prototype-openproject-native-surface-level-0-1`.

## Step 18. OpenProject-native Surface Level 0 and Level 1 Prototype

Decision date: 2026-08-04.

The first OpenProject-native surface prototype is implemented.

Implemented result:

- dashboard JSON now includes an `openproject` object;
- the object reports whether an OpenProject source is present;
- it reports linked work-package count, missing-link count, URL count and tasks
  with resource demands;
- it lists OpenProject work-package id, OpenProject URL, RePlan task id, title,
  tracking status, planned dates, resource ids, resource demand count, warning
  count and open dependency count;
- `index.html` renders this as an `OpenProject Native Surface` section;
- `gantt.html` renders the same section before the Gantt timeline;
- existing task rows and Gantt rows still keep direct OpenProject links;
- raw JSON remains available for AI CLI agents.

This is still not an OpenProject plugin. It is the tested Level 0 and Level 1
bridge that lets a human compare OpenProject task views with the RePlan
resource layer in a neighboring report, while agents consume the same mapping
from `raw/overview.json`.

The next task is
`task.self.reconcile-dashboard-load-semantics`.

That task should make the resource-load visualization less misleading by
explaining or changing the daily bars when the authoritative planning model is
weekly or period-based.

## Step 19. Dashboard Load Profile Semantics Design

Decision date: 2026-08-06.

The resource-load semantics design is accepted for
`task.self.reconcile-dashboard-load-semantics`.

The important decision is to stop treating resource bars as if their horizontal
length represented resource amount. In the accepted model:

- horizontal length means the participation window;
- vertical fill height means intensity or utilization;
- task-level resource rows inherit the whole task window unless the demand has
  an explicit timing window;
- a shorter resource bar is allowed only when explicit demand timing exists;
- the resource summary is a load profile by authoritative planning bucket,
  initially week;
- the summary has a visible 100 percent capacity waterline;
- overload is allowed to extend above the 100 percent waterline;
- numeric values are available through tooltip/details/raw JSON, while the
  primary signal remains graphical.

The design result is recorded in
`docs/dev/dashboard-load-profile-semantics-design.md`.

The next implementation should update `raw/gantt.json`, `index.html` and
`gantt.html` to render these capacity-height profiles.

## Step 20. Dashboard Load Profile Rendering Implementation

Decision date: 2026-08-06.

The implementation task is
`task.self.implement-dashboard-load-profile-rendering`.

This task is now tracked in RePlan and in the committed sanitized OpenProject
fixture. It is tagged with `openproject_sync_pending` because the live
OpenProject instance still needs a real work package and private binding. The
current reviewed OpenProject operator toolkit can inspect, update and comment
on existing work packages, but it cannot safely create a new live OpenProject
work package with a dry-run/apply gate. Live synchronization must therefore be
done by a reviewed create-helper or by an explicit human OpenProject action
before this branch is considered fully dogfooded in the live OpenProject/RePlan
loop.

Implementation scope:

- extend `raw/gantt.json` with participation-window source, capacity for the
  window, intensity ratio, intensity percent and over-capacity flag for task
  resource allocations;
- render task resource rows so horizontal length means participation window and
  vertical fill height means intensity;
- render the resource summary as weekly utilization buckets;
- keep a visible one hundred percent capacity waterline;
- allow overload to extend above the one hundred percent waterline;
- keep exact numbers secondary in tooltips and raw JSON rather than replacing
  the graph with text.

Completion update, 2026-08-06:

The implementation is complete in the committed RePlan project data, sanitized
OpenProject fixture, `raw/gantt.json` contract and static `gantt.html`
rendering. The live OpenProject instance still needs the corresponding real
work package and private binding because live work-package creation is not yet
covered by the reviewed OpenProject operator toolkit.

## Step 21. Guarded OpenProject Work-Package Create Helper

Decision date: 2026-08-06.

The implementation task is
`task.self.implement-openproject-work-package-create-helper`.

The need came from the dogfood backlog rule: once work is agreed, OpenProject
owns the human-facing task identity, but the current toolkit could only inspect
and mutate existing work packages. That forced new backlog items into one of
three bad paths:

- raw OpenProject API calls;
- manual browser work;
- RePlan-only temporary tasks with `openproject_sync_pending`.

The create helper closes that product and process gap.

Implemented command:

```text
scripts/openproject-work-package-toolkit.sh work-package create
```

The command is dry-run by default, blocks fixture-mode apply, checks duplicate
subjects unless `--allow-duplicate` is explicit, validates live payloads through
the OpenProject work-package form, and requires `REPLAN_APPLY=1` before a live
work package is created. On apply it returns the created work-package id,
created source reference and a private binding suggestion when a RePlan task id
is provided.

The implementation is complete in code, documentation, smoke coverage and the
sanitized OpenProject fixture. Live OpenProject creation was not run in this
session because `OPENPROJECT_TOKEN` was not present in the active shell
environment.

## Step 22. Guarded OpenProject Private Binding Helper

Decision date: 2026-08-06.

The implementation task is
`task.self.implement-openproject-private-binding-helper`.

The create helper returns a source reference and a private binding suggestion,
but the next step in the OpenProject/RePlan loop also needs a safe way to write
that binding into ignored private storage. Manual edits are possible, but they
are a bad default for AI CLI agents because they make duplicates, stale ids and
accidental public commits easier.

Implemented command:

```text
scripts/openproject-work-package-toolkit.sh work-package bind
```

The command is dry-run by default and requires `REPLAN_APPLY=1` before it
writes anything. It validates that the RePlan task exists, rejects attempts to
bind a task that is already bound, rejects attempts to reuse an OpenProject work
package that is already bound to another task, and verifies that the target
OpenProject work package exists unless `--skip-existence-check` is explicitly
used for controlled fixture or smoke-test cases.

The apply path writes only the configured private binding file. It does not
mutate OpenProject, does not mutate the committed sanitized fixture and does
not print private tokens. This keeps the narrow write-back rule intact: broad
automatic bidirectional synchronization is still out of scope, but reviewed
operator helpers can safely perform explicit, gated actions.

The implementation is complete in code, documentation, smoke coverage and the
sanitized OpenProject fixture. Live OpenProject binding was not run in this
session because `OPENPROJECT_TOKEN` was not present in the active shell
environment.

## Step 23. Safe OpenProject Dotenv Loader

Decision date: 2026-08-06.

The implementation task is
`task.self.add-safe-openproject-dotenv-loader`.

The live OpenProject operator loop should not depend on a fragile shell session
that loses `OPENPROJECT_TOKEN` and target settings after an agent restart.
At the same time, RePlan must not commit live tokens, private endpoints or
private binding paths into the public repository.

Implemented helper:

```text
scripts/lib/load-dotenv.sh
```

OpenProject operator scripts now load `.env.local` and then `.env` before they
read `REPLAN_OPENPROJECT_*` settings. Already exported environment variables
remain authoritative. `.env.local` has higher priority than `.env`.

The loader is intentionally not a general shell `source` mechanism. It accepts
only the OpenProject connection keys needed for the dogfood loop, exports them
as key-value data and does not execute shell code from dotenv files. It also
does not load `REPLAN_APPLY`; apply remains an explicit per-command decision.
Empty dotenv values are ignored, so a blank token placeholder is not treated as
a configured token.

The committed `.env.example` documents the supported keys with sanitized
values. Real values belong in ignored `.env.local` or another ignored private
secret mechanism.

The implementation is complete in code, documentation, smoke coverage and the
sanitized OpenProject fixture. Live OpenProject synchronization can now be
retried after a real token is placed in ignored local storage.

## Step 24. OpenProject Backlog Reconcile Helper

Decision date: 2026-08-06.

The implementation task is
`task.self.add-openproject-backlog-reconcile-helper`.

The immediate problem after adding safe `create` and `bind` helpers was still
operator friction. An agent could create or bind one work package, but it still
had to manually reconstruct which RePlan tasks were missing live private
bindings, which tasks already had matching work packages, and which tasks
needed new OpenProject work packages.

Implemented command:

```text
scripts/openproject-work-package-toolkit.sh backlog reconcile
```

The command compares:

- RePlan canonical tasks;
- the configured private OpenProject binding file;
- the current OpenProject work-package collection from live API or fixture.

It reports four practical states:

- already bound;
- unbound but safely bindable to one existing work package by exact normalized
  title match;
- unbound and needing a new OpenProject work package;
- ambiguous title matches that require operator review.

Apply remains deliberately narrow. With `REPLAN_APPLY=1`, reconcile writes only
private bindings for unique existing work-package matches. It does not
bulk-create OpenProject work packages. Creation remains a separate reviewed
`work-package create` step because creating several work packages without
review can produce duplicate or wrong human-facing tasks.

The implementation is complete in code, documentation, smoke coverage and the
sanitized OpenProject fixture. Live reconciliation can now be retried after a
real token is placed in ignored local storage.

## Step 25. OpenProject Protected-Connection Header Support

Decision date: 2026-08-06.

Private validation found a transport-specific problem: an agent may reach an
OpenProject reverse proxy through a local address while the application still
expects its configured public host and protocol. In that shape OpenProject can
reject an otherwise valid request.

The accepted design is:

- keep the public HTTPS endpoint protected for human browser access;
- let agents use a protected connection for API and CLI work;
- store live connection settings only in ignored local dotenv/private storage;
- add explicit `Host` and `X-Forwarded-Proto` override support to the
  OpenProject import path and guarded helper scripts;
- keep committed documentation and examples generic.

Supported local dotenv keys now include:

- `REPLAN_OPENPROJECT_HOST_HEADER`;
- `REPLAN_OPENPROJECT_FORWARDED_PROTO`.

These values are transport settings, not task identity. OpenProject still owns
human work packages, RePlan still owns resource planning and private bindings
still map RePlan task ids to live OpenProject work-package ids. The header
settings only make the API path reliable when the instance is reached through
a protected connection.

The implementation is complete in code, documentation, smoke coverage and the
sanitized OpenProject fixture.

Sanitized follow-up: API validation and guarded backlog reconciliation passed;
missing work packages were created and bound through reviewed helpers, with
all live identifiers kept in ignored private storage. Reviewed relation apply
and date-accept helpers then brought task, relation, status and date drift into
alignment. Remaining dependency warnings were intentional planning gates, not
synchronization failures.

## Step 26. Read-only OpenProject Plugin Feasibility

Decision date: 2026-08-06.

The design task is
`task.self.design-openproject-read-only-plugin-feasibility`.

The Level 0 and Level 1 OpenProject-native surface already exists as shared
identity, adjacent generated dashboard views and raw JSON. The next question
was how to move toward the required OpenProject plugin without collapsing
RePlan into an OpenProject-only code path.

Decision:

- OpenProject plugin is mandatory for the OpenProject-centered human workflow;
- do not implement a full OpenProject plugin immediately;
- keep RePlan core independent and CLI-first;
- treat a plugin as a read-only renderer over a stable RePlan overlay artifact;
- prefer a project-level tab or work-package side panel before attempting any
  Gantt overlay;
- choose a static or served overlay artifact as the first deployment shape;
- make `reports/latest/raw/openproject_overlay.json` the next implementation
  target.

The accepted design is documented in
`docs/dev/openproject-read-only-plugin-feasibility.md`.

The next implementation task should be
`task.self.define-openproject-overlay-json-contract`. That task should stabilize
the overlay JSON contract consumed by the generated dashboard today and by a
planned read-only OpenProject plugin later.

## Step 27. OpenProject Overlay Contract And Plugin Scaffold

Decision date: 2026-08-07.

The overlay contract task was implemented as:

```text
task.self.define-openproject-overlay-json-contract
```

The plugin scaffold task was implemented as:

```text
task.self.scaffold-openproject-overlay-plugin-consumer
```

Result:

- RePlan now emits `reports/latest/raw/openproject_overlay.json`;
- the generated dashboard and Gantt views use the same OpenProject-facing
  overlay data;
- a minimal read-only OpenProject plugin scaffold exists under
  `integrations/openproject/plugins/openproject-replan_overlay/`;
- the plugin scaffold is intentionally only a renderer over the overlay
  artifact;
- the plugin scaffold does not mutate OpenProject;
- the plugin scaffold does not mutate RePlan;
- the plugin scaffold does not expose browser-side tokens;
- live OpenProject installation was deliberately not part of the scaffold
  task.

This completes the data-contract and scaffold preconditions for a runtime
deployment design.

## Step 28. Safe OpenProject Plugin Deployment Path

Decision date: 2026-08-07.

The next task is:

```text
task.self.design-openproject-plugin-deployment-path
```

This task was added because a plugin scaffold is not the same thing as a safe
OpenProject runtime installation.

The current OpenProject dogfood runtime is in the class:

```text
Docker Compose, OpenProject 17 slim image, multiple Rails processes
```

Official OpenProject documentation says plugins are Ruby gems loaded through
`Gemfile.plugins`, and the Docker plugin path requires a custom image. For the
slim image, the build path also needs an asset precompile stage.

Accepted direction:

- use a custom OpenProject image for the plugin runtime;
- apply the same custom image to web, worker, cron and seeder Rails services;
- keep the RePlan overlay artifact as runtime data, not baked into the image;
- mount or copy only generated overlay/report artifacts, not the full RePlan
  repository;
- test first in a throwaway OpenProject runtime;
- install into the private dogfood OpenProject instance only after the
  throwaway runtime passes;
- keep private compose paths, domains, credentials and live identifiers in
  private infrastructure documentation;
- keep the public RePlan repository generic and OSS-safe.

The accepted deployment-path design is documented in
`docs/dev/openproject-plugin-deployment-path.md`.

Backlog/resource effect:

- `task.self.design-openproject-plugin-deployment-path` was added to the
  current plan;
- it depends on `task.self.scaffold-openproject-overlay-plugin-consumer`;
- it blocks `task.self.prepare-release-story`;
- `task.self.prepare-release-story` was moved later because the public story
  must not imply that the plugin path is operational before the deployment path
  is reviewed;
- planned resources are 3 maintainer hours, 4 Codex agent-hours and 1 reviewer
  hour.

## Step 29. Next Runtime Plugin Work

The next implementation step after accepting the deployment path should be:

```text
Install and verify read-only RePlan overlay plugin in a throwaway OpenProject runtime
```

That task should not yet modify the private live OpenProject instance.

Expected scope:

- build the custom OpenProject image from the plugin scaffold;
- boot a throwaway OpenProject runtime;
- mount or copy a generated `openproject_overlay.json` artifact;
- verify the RePlan project page inside OpenProject;
- verify missing-overlay behavior;
- inspect logs;
- document exact runtime findings;
- only then decide the private live deployment task.

## Step 30. Throwaway Runtime Plugin Proof Completed

Decision date: 2026-08-07.

The throwaway runtime task is complete:

```text
task.self.verify-openproject-plugin-throwaway-runtime
```

The repeatable heavy check is:

```sh
./scripts/check-openproject-plugin-throwaway-runtime.sh
```

Result:

- a custom OpenProject 17 image with the RePlan overlay plugin builds;
- the disposable OpenProject runtime initializes a clean PostgreSQL database;
- Bundler sees the `openproject-replan_overlay` gem;
- Rails registers the RePlan project route, permission and menu;
- a generated RePlan overlay artifact loads with 45 work-package rows;
- a missing overlay path returns the safe `overlay_file_missing` state;
- live private OpenProject was not modified.

The task found and fixed two packaging requirements:

- `Gemfile.plugins` must explicitly require `open_project/replan_overlay`;
- the plugin route file must draw routes into `Rails.application.routes`.

Detailed evidence is documented in
`docs/dev/openproject-plugin-throwaway-runtime-test.md`.

Backlog/resource effect:

- `task.self.verify-openproject-plugin-throwaway-runtime` is accepted;
- `dependency.plugin-throwaway-runtime-before-release-story` is resolved;
- the public release story remains deferred until the OSS boundary and private
  live OpenProject plugin path are clean enough;
- the next plugin step is a private live OpenProject deployment preflight and
  installation task, not another throwaway runtime proof.

## Step 31. Private Live Plugin Installation Task Added

Decision date: 2026-08-07.

The next OpenProject plugin work is now represented as a real backlog task:

```text
task.self.prepare-live-openproject-plugin-installation
```

This task is intentionally a guarded private live preflight and installation
planning task, not an immediate blind production mutation.

Scope:

- verify backup and rollback readiness for the private OpenProject instance;
- identify the exact Rails services that must use the custom plugin image;
- prepare the private compose override and image tag;
- prepare the generated RePlan overlay artifact mount;
- verify real project role permissions and browser access requirements;
- run dry-run or read-only preflight checks;
- produce an explicit operator apply plan before any live restart.

Backlog/resource effect:

- `task.self.prepare-live-openproject-plugin-installation` is added to the
  current plan;
- it depends on `task.self.verify-openproject-plugin-throwaway-runtime`;
- it blocks `task.self.prepare-release-story`;
- planned resources are 4 maintainer hours, 6 Codex agent-hours, 1 local
  development runtime slot and 1.5 reviewer hours.

## Step 32. Private Live Plugin Installation Preflight Accepted

Decision date: 2026-08-07.

The private live OpenProject plugin preflight was completed as:

```text
task.self.prepare-live-openproject-plugin-installation
```

Accepted outcome:

- the private OpenProject deployment class is confirmed as Docker Compose with
  OpenProject 17 slim Rails services;
- the Rails services that must share the custom image are `web`, `worker`,
  `cron` and `seeder`;
- a custom candidate OpenProject image containing the RePlan overlay plugin was
  built on the target host without switching live containers;
- the image preflight found and fixed a real packaging issue where the final
  slim image needed OpenProject modules copied from the builder stage after
  `Gemfile.lock` was copied;
- the repeatable throwaway runtime script now copies `/app/modules` into the
  final image;
- a current RePlan overlay artifact can be generated for the plugin to read;
- a public-safe compose override shape, backup requirement and rollback plan
  are documented;
- the actual live OpenProject restart was deliberately split into a separate
  gated task.

The preflight is documented in:

```text
docs/dev/openproject-plugin-live-installation-preflight.md
```

Backlog/resource effect:

- `task.self.prepare-live-openproject-plugin-installation` is accepted;
- `task.self.execute-live-openproject-plugin-installation` is added as the next
  gated private maintenance-window task;
- release-story dependency now waits for actual live installation acceptance,
  not only for preflight acceptance;
- `task.self.prepare-release-story` is moved to `2026-10-26` through
  `2026-11-06` so the reviewer role is not overloaded in the same week as the
  live plugin installation;
- planned resources for the execution task are 3 maintainer hours, 4 Codex
  agent-hours, 1 local development runtime slot and 1.5 reviewer hours.

## Step 33. Private Live OpenProject Plugin Installation Accepted

Decision date: 2026-08-07.

The first private live OpenProject plugin installation was completed as:

```text
task.self.execute-live-openproject-plugin-installation
```

The install was not a clean first try. The first live custom image used a
floating OpenProject builder tag while the live runtime was OpenProject 17.6.0
slim. That caused a Bundler production mismatch during Rails boot. The
operator rollback path worked: the compose override was disabled, Rails
services were recreated on the stock image and OpenProject returned to healthy
without database restore.

The accepted fix is to build the plugin image from the exact OpenProject image
pair used by the live instance:

```text
openproject/openproject:17.6.0
openproject/openproject:17.6.0-slim
```

The final live installation uses an exact-version RePlan plugin image for all
OpenProject Rails services:

- `web`;
- `worker`;
- `cron`;
- `seeder`.

Verified outcome:

- backup was created before live apply;
- overlay artifact was generated and published with 47 work packages, 47 linked
  tasks and zero missing links;
- OpenProject web health is healthy after the exact-version apply;
- Rails sees the RePlan plugin, route, permission and project menu;
- the plugin route redirects to normal OpenProject login instead of returning a
  missing route;
- `dependency.live-plugin-installation-before-release-story` is resolved.

The result is documented in:

```text
docs/dev/openproject-plugin-live-installation-result.md
```

Backlog/resource effect:

- `task.self.execute-live-openproject-plugin-installation` is accepted;
- its planned resource demands are consumed;
- the release-story gate for live plugin installation is resolved;
- other release-story gates remain open.

## Step 34. Live OpenProject Plugin Resource View Follow-Up

The first live plugin page was opened through the normal OpenProject project
menu after enabling the `replan_overlay` project module and granting
`view_replan_overlay` to the project role used by the operator.

Observed result:

- the `RePlan` project-menu item appears inside the live `RePlan self-dogfood`
  OpenProject project;
- the plugin renders the current read-only table over the RePlan overlay
  artifact;
- the OpenProject-native path is real, but the first table is only a baseline
  renderer and is not yet the desired human resource-planning view.

The next backlog task is:

```text
task.self.improve-live-openproject-plugin-resource-view
```

Purpose:

- review the live project-level plugin page from the operator workflow;
- improve it into the smallest useful resource-aware OpenProject view;
- keep the plugin read-only;
- keep OpenProject as the human task and Gantt surface;
- keep RePlan as the resource-planning, actuals, warnings and proposal layer;
- render only the existing OpenProject overlay JSON contract or a reviewed
  extension of that contract;
- decide which richer resource timeline or Gantt-like signals belong in the
  first project-level plugin page and which should be deferred to a later
  work-package panel or Gantt overlay.

Backlog/resource effect:

- `task.self.improve-live-openproject-plugin-resource-view` is added to the
  current RePlan self-dogfood plan;
- it depends on `task.self.execute-live-openproject-plugin-installation`;
- planned resources are 4 maintainer hours, 6 Codex agent-hours, 1 local
  development environment slot and 1.5 reviewer hours;
- a live OpenProject work package was created through the guarded toolkit and
  bound in the private binding layer; committed public examples use only a
  sanitized OpenProject reference.

Implementation result:

- the plugin now uses a small server-side presenter over
  `openproject_overlay.json`;
- the OpenProject project page renders summary metrics, resource aggregates,
  current-attention rows and task/resource timeline rows;
- the plugin remains read-only and does not move RePlan planning logic into
  OpenProject;
- the private live OpenProject image was rebuilt and switched to the resource
  view tag;
- the private mounted artifact was refreshed to 48 linked work packages with
  zero missing links;
- `task.self.improve-live-openproject-plugin-resource-view` is accepted as
  sufficient to unblock the work-package-level RePlan panel design, with the
  understanding that the project-level view is not final UI and future
  refinements should be separate tasks.

Native Gantt decision:

- changing the native OpenProject Gantt is possible in principle;
- it should not be the next default step because native Gantt behavior belongs
  to OpenProject frontend internals and may require extension-point research,
  monkey-patching, frontend asset rebuilds and version-specific maintenance;
- RePlan should first improve the current project-level plugin page, then add a
  work-package-level RePlan panel, then build a controlled Gantt-like RePlan
  view inside the plugin;
- a true native OpenProject Gantt overlay remains a later option once the data
  contract and simpler surfaces prove which resource signals are worth putting
  directly into OpenProject Gantt;
- the overlay contract must keep timeline, participation-window, capacity,
  load-intensity and warning fields available so that later transition remains
  possible.

## Step 35. Work-Package-Level RePlan Panel Design Backlog

After the project-level live RePlan plugin view was deployed and the native
Gantt decision was clarified, the next agreed OpenProject-native surface step is
not to patch OpenProject Gantt immediately.

The next backlog task is:

```text
task.self.design-openproject-work-package-replan-panel
```

Purpose:

- design a read-only RePlan panel or section visible from an individual
  OpenProject work package;
- show the linked RePlan task, resource demands, planned/actual/remaining
  amounts, warnings, proposals, evidence and artifact links for that work
  package;
- keep OpenProject as the human task surface;
- keep RePlan as the resource-planning and evidence sidecar;
- reuse `openproject_overlay.json` where possible and explicitly define any
  reviewed extension needed for work-package-level rendering;
- avoid native OpenProject Gantt coupling in this slice;
- end with a concrete implementation backlog item for the first panel
  prototype.

Backlog/resource effect:

- `task.self.design-openproject-work-package-replan-panel` is added to the
  current RePlan self-dogfood plan;
- it depends on
  `task.self.improve-live-openproject-plugin-resource-view`;
- planned resources are 2 maintainer hours, 3 Codex agent-hours and 0.25
  reviewer hours;
- live OpenProject synchronization created and bound the matching work package
  through the guarded toolkit;
- the dependency relations around the new work package were applied through the
  reviewed relation helper and stored in the ignored private binding layer;
- the task is now `in_progress` in RePlan and live OpenProject;
- the first draft design is documented in
  `docs/dev/openproject-work-package-replan-panel-design.md`;
- partial actual consumption was recorded for the maintainer and Codex design
  work, with reviewer acceptance still remaining.

## Step 36. Documentation Synchronization Audit

Decision date: 2026-08-15.

The rapid OpenProject-native surface work left several documents with valid
historical context but stale "next step" language.

The current state is:

- OpenProject remains the human task, relation, status, date and Gantt surface;
- RePlan remains the resource-planning, actuals, evidence, warnings, proposals,
  scenario and audit sidecar;
- Level 0 and Level 1 OpenProject-native surfaces are implemented;
- the read-only OpenProject plugin has moved from future direction to a working
  live private dogfood installation;
- the project-level plugin view now renders resource-aware RePlan state from
  `reports/latest/raw/openproject_overlay.json`;
- the active design gate is the work-package-level RePlan panel;
- native OpenProject Gantt modification remains technically possible but
  deferred behind lower-coupling plugin surfaces.

The documentation synchronization task is:

```text
task.self.audit-and-sync-project-documentation
```

Scope:

- audit related project, roadmap, design and specification documents;
- update stale plugin/static-dashboard/current-next wording;
- keep historical decisions visible instead of rewriting history;
- add a public-safe audit document with findings and current authoritative
  state;
- record the task in RePlan canonical backlog data;
- mark live OpenProject synchronization pending if live access is not
  available during the documentation pass.

Documentation result:

- the audit is recorded in
  [`../dev/documentation-sync-audit-2026-08-15.md`](../dev/documentation-sync-audit-2026-08-15.md);
- the documentation index points to the audit;
- the OpenProject-native surface, OpenProject overlay strategy, static
  dashboard and backlog convention documents now state the current plugin and
  work-package panel direction;
- late roadmap step numbers were corrected so plugin resource-view and
  work-package panel entries follow the live plugin installation sequence.

Backlog/resource effect:

- `task.self.audit-and-sync-project-documentation` is added to the current plan;
- it is a documentation task with OpenProject sync pending;
- planned resources are 1.5 maintainer hours, 2 Codex agent-hours and 0.25
  reviewer hours;
- the task is accepted after this synchronization pass and should be synced to
  live OpenProject during the next live backlog reconciliation.

## Step 37. Work-Package Panel Design Accepted And Implementation Task Added

Decision date: 2026-08-15.

The work-package-level RePlan panel design is accepted for the first
implementation slice.

Accepted first implementation scope:

- use Placement A first: a plugin route for one OpenProject work package;
- link to that route from the project-level RePlan plugin page;
- keep the first panel read-only;
- show resource rows for the linked RePlan task;
- show planned, actual and remaining resource amounts;
- show warning, proposal and evidence summaries when present in the overlay
  contract;
- show compact task/resource timeline bars using the same visual semantics as
  the project-level resource view;
- render safe empty states when the overlay artifact, work-package row or field
  is missing;
- keep native OpenProject work-package injection deferred;
- keep native OpenProject Gantt modification deferred;
- keep RePlan planning logic in RePlan reports, not inside the OpenProject
  plugin.

The accepted design is documented in:

```text
docs/dev/openproject-work-package-replan-panel-design.md
```

The next implementation task is:

```text
task.self.implement-openproject-work-package-replan-panel
```

Purpose:

- add the plugin route for a single work-package panel;
- add presenter lookup by OpenProject work-package id;
- add a per-work-package ERB view;
- add links from project-level plugin rows to the per-work-package panel;
- extend scaffold checks;
- regenerate and republish the overlay artifact when the plugin is deployed;
- verify the live route while preserving read-only behavior.

Backlog/resource effect:

- `task.self.design-openproject-work-package-replan-panel` is accepted;
- the design task resource demands are consumed;
- `dependency.work-package-panel-design-before-release-story` is resolved;
- `task.self.implement-openproject-work-package-replan-panel` is added to the
  current plan;
- the implementation task depends on the accepted design;
- the implementation task blocks the public release story until it is accepted
  or explicitly deferred;
- planned implementation resources are 3 maintainer hours, 5 Codex
  agent-hours, 1 local development environment slot and 0.75 reviewer hours;
- live OpenProject synchronization for the new implementation work package is
  pending if the live instance is unavailable; public sanitized fixture data
  uses work package `2151`.

## Step 38. Work-Package Panel Local Prototype Implemented

Decision date: 2026-08-15.

The first local implementation pass for
`task.self.implement-openproject-work-package-replan-panel` is complete.

Implemented locally:

- plugin route
  `/projects/:project_id/replan_overlay/work_packages/:work_package_id`;
- controller action `work_package`;
- presenter lookup by OpenProject work-package id;
- per-work-package summary data;
- per-work-package decorated resource demand rows;
- read-only ERB page for one work package;
- links from the project-level RePlan plugin page to the per-work-package
  panel;
- safe missing-overlay and missing-work-package states;
- scaffold check coverage for the new route, action, presenter method, view and
  project-level links;
- ERB syntax validation in the plugin scaffold check.

Still pending:

- private live OpenProject plugin rebuild and deployment;
- live route verification through the OpenProject instance;
- operator browser review of the task-level panel.

Reason:

```text
check-openproject-drift -> openproject_network_error
```

Backlog/resource effect:

- `task.self.implement-openproject-work-package-replan-panel` is now
  `in_progress`;
- maintainer, Codex and local development environment actuals were recorded for
  the local implementation pass;
- reviewer acceptance remains planned;
- remaining implementation work represents live deployment, live route
  verification and operator review;
- public sanitized fixture data marks work package `2151` as `In progress` so
  local smoke tests remain deterministic.

## Step 39. Work-Package Panel Live Deployment Verified

Decision date: 2026-08-15.

The first work-package-level RePlan panel is now deployed to the private live
OpenProject plugin instance.

Live deployment result:

- the custom OpenProject plugin image was rebuilt from the exact OpenProject
  17.6.0 builder and slim image pair;
- the private live Rails services were switched to the new image;
- the mounted RePlan overlay artifact was regenerated from the current
  self-dogfood project and republished to the private OpenProject mount;
- OpenProject web returned healthy on the updated image;
- the live Rails process sees both RePlan routes:
  `/projects/:project_id/replan_overlay` and
  `/projects/:project_id/replan_overlay/work_packages/:work_package_id`;
- the live overlay loader reads 51 work packages;
- the live presenter can load a work-package row with resource demands.

Operational finding:

- OpenProject Rails boot after image replacement can exceed the default Docker
  health and autoheal window. The first apply was rolled back to the previous
  image, then retried with autoheal temporarily stopped until the web service
  answered the health endpoint. Autoheal was restored after the service became
  healthy.

Backlog/resource effect:

- `task.self.implement-openproject-work-package-replan-panel` is accepted in
  RePlan;
- maintainer, Codex, local development environment and reviewer demands for
  the implementation task are consumed;
- `dependency.work-package-panel-implementation-before-release-story` is
  resolved;
- the task keeps `openproject_sync_pending` because the guarded OpenProject API
  helper timed out after live route verification. The live OpenProject
  work-package status should be synchronized later through the reviewed helper,
  not through a direct database write.

## Step 40. OpenProject Plugin Image Build Layering

Decision date: 2026-08-16.

The slow live plugin image rebuild path is now treated as a real dogfood
operations finding, not just an inconvenience.

Accepted implementation:

- add a shared image build helper at
  `scripts/build-openproject-plugin-image.sh`;
- preserve the exact OpenProject 17.6.0 builder and slim runtime image pair;
- keep a compatibility `full` build mode that follows the original one-shot
  Dockerfile shape;
- add a heavy `deps-base` mode for Bundler output, OpenProject modules and
  precompiled assets;
- add a fast `code` mode that starts from the dependency base and copies only
  the RePlan OpenProject plugin source;
- add an `auto` mode that uses the fast code-only path when the dependency
  image already exists and bootstraps the dependency base otherwise;
- make the throwaway runtime test use the same build helper as live
  operations.
- prepare the private runtime for the optimized path by tagging the current
  healthy plugin image as the dependency base;
- verify a private code-only image build without running Bundler or OpenProject
  asset precompile.

Operational rule:

- changes to RePlan overlay data require only artifact regeneration and
  publishing, not an image rebuild;
- ordinary plugin Ruby, controller, presenter, route and ERB changes should use
  the fast code layer;
- dependency-sensitive changes require rebuilding the dependency base.

Backlog/resource effect:

- `task.self.optimize-openproject-plugin-image-build-layering` is accepted in
  RePlan;
- maintainer, Codex, local development environment and reviewer demands for
  the tooling task are consumed;
- the task keeps `openproject_sync_pending` until the live OpenProject backlog
  can be synchronized through the guarded helper.

## Step 41. OpenProject Live Helper Timeout Stabilization

Decision date: 2026-08-16.

The live OpenProject helper timeout issue is treated as part of the operator
loop, not as a one-off deployment accident.

Accepted design/implementation direction:

- OpenProject live API requests must be bounded by explicit request timeouts;
- slow Rails boot, proxy latency and protected-path latency must be visible in
  command output;
- transient timeout, network and server failures may be retried by guarded
  Python OpenProject operator helpers;
- all OpenProject mutations remain dry-run by default and still require
  `REPLAN_APPLY=1`;
- timeout/retry settings are operational configuration and may live in ignored
  dotenv files, but tokens, endpoints and private ids must still stay outside
  committed source.

Implemented public-safe contract:

- `REPLAN_OPENPROJECT_TIMEOUT_SECONDS`, default `60`;
- `REPLAN_OPENPROJECT_RETRIES`, default `0`;
- `REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS`, default `2`;
- `replan import openproject --request-timeout <seconds>` for per-command
  import override;
- shared Python HTTP helper for:
  - `scripts/openproject-work-package-toolkit.sh`;
  - `scripts/push-openproject-status-drift.sh`;
  - `scripts/push-openproject-date-drift.sh`;
  - `scripts/apply-openproject-bootstrap.sh`;
  - `scripts/apply-openproject-relations.sh`;
- JSON output from guarded Python helpers includes the resolved
  `openproject_request` timing block.

Operational consequence:

After OpenProject plugin deployment or Rails restart, the operator should first
retry guarded helpers with reviewed timing settings, for example:

```sh
REPLAN_OPENPROJECT_TIMEOUT_SECONDS=180 \
REPLAN_OPENPROJECT_RETRIES=2 \
REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS=5 \
./scripts/openproject-work-package-toolkit.sh status list
```

Automatic retries apply to reads and to PATCH requests carrying a
`lockVersion` only; validation-only form calls are retried only when the
caller explicitly marks them idempotent.

If the helper still fails, the failure becomes evidence for deployment/operator
loop repair. It must not be bypassed with an unreviewed direct API mutation.

Backlog/resource effect:

- `task.self.stabilize-openproject-live-helper-timeouts` is the active task for
  this slice;
- the implementation should close the public RePlan task after validation;
- live OpenProject status synchronization for recent accepted tasks can be
  retried after the helper behavior is validated;
- any remaining live OpenProject divergence should stay explicit as drift or
  `openproject_sync_pending`, not hidden in documentation prose.

## Step 42. Latest Live OpenProject Backlog Drift Synchronized

Decision date: 2026-08-16.

After helper timeout stabilization, the live OpenProject/RePlan dogfood loop
still had real backlog drift:

- local RePlan tasks: 53 before this slice, then 54 after adding the explicit
  synchronization task;
- live OpenProject task candidates before repair: 49;
- unbound local tasks before repair: 5 after the synchronization task was
  added;
- missing live dependencies before repair: 9;
- status drift before repair: 2;
- date drift before repair: 10;
- assignee warnings before repair: 8 recently created or updated work
  packages without an OpenProject assignee.

The drift was repaired only through guarded helpers:

- `scripts/openproject-work-package-toolkit.sh backlog reconcile`;
- `scripts/openproject-work-package-toolkit.sh work-package create`;
- `scripts/openproject-work-package-toolkit.sh work-package bind`;
- `scripts/apply-openproject-relations.sh`;
- `scripts/push-openproject-status-drift.sh`;
- `scripts/openproject-work-package-toolkit.sh work-package update-dates`;
- `scripts/accept-openproject-soft-drift.sh`;
- `scripts/openproject-work-package-toolkit.sh work-package update-metadata`;
- `scripts/check-openproject-drift.sh`.

Operator decisions:

- missing live work packages were created one by one, not bulk-created by
  backlog reconcile;
- each created live work package was bound through the ignored private binding
  layer;
- nine missing live relations were applied after a dry-run showed 68 existing
  relations matched and 9 needed creation;
- two stale OpenProject statuses were pushed to `Closed` because RePlan had
  already accepted the corresponding work;
- RePlan dates were pushed to OpenProject first;
- OpenProject then normalized two completed-task dates from weekend dates to
  working-calendar dates, and those final OpenProject dates were accepted back
  into RePlan because OpenProject is the human Gantt/calendar surface;
- eight recently created or updated work packages were assigned to the live
  maintainer account through metadata update;
- no raw API call, direct database write, browser delete-and-recreate shortcut
  or committed live identifier was used.

Final verified result:

```text
check-openproject-drift:
  drift_status: aligned
  local_tasks: 54
  live_task_candidates: 54
  local_dependencies: 77
  live_dependency_candidates: 77
  unbound_local_tasks: 0
  missing_live_tasks: 0
  extra_live_tasks: 0
  title_drifts: 0
  status_drifts: 0
  date_drifts: 0
  unbound_local_dependencies: 0
  missing_live_dependencies: 0
  extra_live_dependencies: 0
  relation_binding_mismatches: 0
  warning_codes: []
  error_codes: []
```

The plugin-facing overlay artifact was also refreshed after the data sync:

- `replan report dashboard` regenerated the current dashboard artifact set;
- the generated `reports/latest/raw/openproject_overlay.json` contained 54
  work packages, 54 linked tasks and 0 missing links;
- the generated artifact directory was published to the private live
  OpenProject overlay mount;
- the live Rails loader read the refreshed artifact with 54 work packages, 54
  linked tasks and 0 missing links;
- OpenProject health remained healthy after the artifact refresh;
- the artifact refresh did not require an OpenProject image rebuild or service
  restart.

Backlog/resource effect:

- `task.self.sync-latest-openproject-live-backlog-drift` is accepted;
- the task's maintainer, Codex, local development environment and reviewer
  demands are consumed;
- `dependency.live-backlog-sync-before-release-story` is resolved;
- `openproject_sync_pending` tags were removed from the now-synchronized recent
  tasks;
- the public sanitized fixture and example binding file were updated to include
  54 work packages, while real live work-package and relation ids remain only
  in ignored private bindings.

## Step 43. Post-Pause Dogfood Re-Entry

Decision date: 2026-09-18.

The project was paused for roughly one month after the 2026-08-16 live
OpenProject/RePlan synchronization. The last committed state is still useful,
but it must not be treated as current without review.

Important stale facts:

- `task.self.run-full-openproject-dogfood-cycle` remains `in_progress`;
- its planned window is `2026-08-17` to `2026-08-28`;
- the current date is now `2026-09-18`;
- `task.self.prepare-release-story` remains blocked by the release gates for
  the full dogfood cycle, OpenProject-native surface proof and dashboard
  semantics;
- the last verified live OpenProject drift state is aligned as of
  2026-08-16, not as of the current date.

Decision:

Do not continue from chat memory and do not jump directly to new feature work.
Create an explicit re-entry and rebaseline task:

```text
task.self.reenter-openproject-dogfood-cycle-after-pause
```

The task exists to answer these questions:

1. Is live OpenProject still aligned with RePlan?
2. Does the human OpenProject Gantt still explain the project?
3. Does the RePlan plugin view inside OpenProject explain the resource layer?
4. Does the work-package-level RePlan panel help on a real task?
5. Which parts of the full dogfood cycle are already proven?
6. Which parts remain unproven: planning, resource check, execution,
   actuals, analysis or next-decision selection?
7. Should `task.self.run-full-openproject-dogfood-cycle` be accepted, split,
   replanned or kept in progress?
8. What is the next product step after evidence, not after memory?

Backlog/resource effect:

- the re-entry task is added to the current RePlan plan;
- it is marked `in_progress`;
- it is tagged `openproject_sync_pending` until the live OpenProject backlog is
  updated;
- it depends on the accepted latest live backlog sync;
- it blocks public release preparation until the post-pause dogfood-cycle
  decision is recorded;
- planned resources are:
  - 2 maintainer hours;
  - 1.5 Codex agent-hours;
  - 1 local development environment slot;
  - 0.5 reviewer hours.

The immediate next operator sequence is:

1. run local validation and resource drift checks;
2. check live OpenProject drift when access is available;
3. sync the re-entry task to OpenProject through guarded helpers;
4. refresh generated RePlan dashboard and plugin overlay artifacts;
5. ask the human operator to inspect OpenProject Gantt, RePlan project view and
   one work-package-level panel;
6. record actuals, evidence and the next product decision.

Initial execution note, 2026-09-18:

- local RePlan validation passes with only expected unresolved release-gate
  warnings;
- local resource schedule drift check passes;
- the current execution environment cannot complete live OpenProject drift
  verification and reports `openproject_network_error`;
- the re-entry task therefore intentionally keeps `openproject_sync_pending`;
- the MVP smoke that expects a fully bound sanitized OpenProject fixture is not
  the acceptance check for this partial re-entry state until the task is synced
  into OpenProject.

Audit record, 2026-09-21:

The post-pause repository state was audited in full and recorded in
[`../dev/project-audit-2026-09-21.md`](../dev/project-audit-2026-09-21.md).

Confirmed by command evidence:

- clean HEAD `9fca426` still passes `go test ./...` and `./scripts/smoke-mvp.sh`;
- the uncommitted re-entry task makes overlay tests and MVP smoke fail because
  they still require 54 fully linked OpenProject work packages and
  `missing_link_count: 0`;
- local `replan validate examples/replan-self` still passes with four
  `required_dependency_unresolved` warnings on the release-story gates;
- local `check-resource-schedule-drift.sh` still returns `aligned`;
- live `check-openproject-drift.sh` still returns `openproject_network_error`;
- two of the four release-story dependency warnings are hygiene because their
  predecessors are already `accepted`;
- the remaining open proposal for
  `dependency.oss-audit-before-release-story` is stale because that dependency
  is already `resolved`;
- public OSS release remains blocked by the 2026-08-02 boundary checkpoint.

The re-entry task remains `in_progress` with `openproject_sync_pending`. The
audit does not close it and does not decide the fate of
`task.self.run-full-openproject-dogfood-cycle`. That decision still requires
live OpenProject review or an explicit later operator decision recorded with
the pending-sync state kept visible.

## Step 44. OpenProject Wiki API Gap And Plugin Backlog

Decision date: 2026-09-21.

GitHub issue 1 recorded a verified OpenProject 17.6.0 platform gap: an
API-token participant can neither read nor write wiki page text. API v3 returns
only `id`, `title` and project for a wiki page, has no collection or write
route, and the web wiki routes accept only a password session. The gap surfaced
in a second private project on the same OpenProject instance, where an agent
participates with a token only.

Decision:

- extend OpenProject API v3 with an own plugin; reject the password-session
  path and the database-reading side service; run the upstream request as a
  parallel task, not as the solution;
- ship the capability as a separate gem `openproject-wiki_api`, not inside
  `openproject-replan_overlay`, because the overlay plugin is read-only by
  contract and the wiki capability is unrelated to RePlan resource planning;
- keep authorization on the standard `view_wiki_pages`, `edit_wiki_pages` and
  `manage_wiki` permissions, with writes only through the upstream
  `WikiPages` services and contracts and `lockVersion` conflict handling;
- reuse the existing exact-version custom image, throwaway runtime check,
  rollback path and fast code layer.

The upstream source was checked at `v17.6.0`, `v17.8.0` and `dev`: the wiki API
is unchanged, so an OpenProject upgrade does not close the gap.

Documents:

- [`../dev/openproject-wiki-api-gap.md`](../dev/openproject-wiki-api-gap.md);
- [`../dev/openproject-wiki-api-plugin-design.md`](../dev/openproject-wiki-api-plugin-design.md);
- later review:
  [`../dev/openproject-wiki-api-plugin-design-review.md`](../dev/openproject-wiki-api-plugin-design-review.md).

Backlog/resource effect:

- five tasks are added to the current plan:
  - `task.self.design-openproject-wiki-api-access`, `in_progress`,
    `2026-09-21`;
  - `task.self.check-upstream-openproject-wiki-api-roadmap`, `2026-09-22`;
  - `task.self.implement-openproject-wiki-api-plugin`, `2026-09-23` to
    `2026-09-25`;
  - `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime`,
    `2026-09-28` to `2026-09-29`;
  - `task.self.deploy-openproject-wiki-api-plugin-live`, `2026-10-05` to
    `2026-10-06`;
- four dependencies chain design, implementation, throwaway runtime and live
  deployment; the upstream check depends on the design as `preferred` and
  blocks nothing;
- none of these tasks gates `task.self.prepare-release-story`;
- planned totals are 9 maintainer hours, 15 agent-hours, 2.25 reviewer hours
  and three local development environment slot reservations;
- the first resource schedule check reported `needs_calibration`: the
  throwaway runtime task and the live deployment task both reserved the single
  local development environment slot in `2026-W40`. The live deployment was
  moved to `2026-W41`; the check then returned `aligned` with no overloaded
  weekly rows;
- agent demands use `resource.ai.codex.local` because it is the only modeled
  AI execution resource. The design work in this step was executed by a
  different AI CLI agent. Whether the resource catalog should model agent
  execution generically or per agent is an open resource-model decision.

OpenProject sync: pending.

Reason: live OpenProject was unreachable from the working environment. All five
tasks carry `openproject_sync_pending`. They must be created and bound in live
OpenProject through `work-package create` and `work-package bind`, and their
relations applied through `apply-openproject-relations.sh`, when access
returns.

Sanitized fixture effect:

- the public OpenProject fixture and the example binding file now contain 60
  work packages: the previous 54, work package `2155` for
  `task.self.reenter-openproject-dogfood-cycle-after-pause` and work packages
  `2156` to `2160` for the wiki API tasks;
- the re-entry task received the sanitized source reference `2155`, following
  the Step 37 precedent where a sync-pending task still gets a public fixture
  id. Its `openproject_sync_pending` tag stays;
- the overlay test and MVP smoke count assertions moved from 54 to 60;
- `go test ./...` and `./scripts/smoke-mvp.sh` pass again on the working tree.
  This supersedes the "dirty working tree is not CI-green" finding of the
  2026-09-21 audit. The other audit findings are unchanged: live OpenProject
  alignment is still unverified, two `in_progress` tasks are past their planned
  finish, and the dependency and proposal hygiene items remain open.

## Step 45. Wiki API Plugin Design Review

Decision date: 2026-09-21.

The first implementation design from Step 44 was reviewed in full against
GitHub issue 1, the gap record, the canonical backlog and the public
OpenProject 17.6.0 source. The unabridged review is
[`../dev/openproject-wiki-api-plugin-design-review.md`](../dev/openproject-wiki-api-plugin-design-review.md).

Decision:

- the product direction stays accepted: a separate `openproject-wiki_api`
  gem, standard wiki permissions, writes only through
  `WikiPages::CreateService` and `WikiPages::UpdateService`, no password
  session, no database sidecar, upstream request as a parallel task;
- the implementation design is not accepted for coding until the required
  amendments in that review are applied to
  [`../dev/openproject-wiki-api-plugin-design.md`](../dev/openproject-wiki-api-plugin-design.md)
  and the operator accepts that text;
- the four required amendments are: require `lockVersion` in the PATCH
  payload representer because `WikiPages::UpdateContract` does not require
  the client to send it; use work-package Create/Update endpoint helpers
  instead of the documents `JSON.parse` PATCH; resolve collection text
  versus `extend_api_response` on the single representer; paginate
  `WikiPage.visible` scoped to the project wiki instead of
  `ParamsToQueryService`;
- no new backlog task is created; the amendments stay on
  `task.self.design-openproject-wiki-api-access`;
- `dependency.wiki-api-design-before-plugin-implementation` stays `open`
  with `required_state: accepted`. Implementation must not start.

Backlog/resource effect:

- none. Planned hours, dates, dependencies and `openproject_sync_pending`
  tags from Step 44 are unchanged. This step consumes part of the already
  planned design-review demand
  `resource_demand.wiki-api-design.review`; remaining amounts are not
  closed here because actuals are not being booked in this documentation
  pass.

OpenProject sync: pending, unchanged from Step 44.

## Step 46. Wiki API Plugin Design Amendments Applied

Decision date: 2026-09-21.

The amendments required by the Step 45 design review were applied to
[`../dev/openproject-wiki-api-plugin-design.md`](../dev/openproject-wiki-api-plugin-design.md).
Before applying them, the review's source claims were re-checked against
upstream OpenProject `v17.6.0`. They hold: `WikiPages::UpdateContract` rejects
only a nil or changed in-memory `lock_version`, the documents `PATCH` passes a
raw `JSON.parse` hash into its service, and no wiki-page query exists.

Applied:

- `lockVersion` is a plugin rule: missing and stale are both HTTP 409 and leave
  the text unchanged. The mechanism is the one work packages use,
  `.reverse_merge(lock_version: nil)` on the parsed attributes;
- the write path is the `API::V3::Utilities::Endpoints::Create` and `Update`
  helpers with a payload representer, as `news` and work packages do. The
  documents module stays a reference for a formattable `GET` body only;
- the collection uses a separate thin list-item representer without text,
  ordered by title, over `WikiPage.visible` scoped to the project's wiki, with
  no `ParamsToQueryService` and no Query class;
- HTTP status by case, endpoint-injected `wiki` and `author`, unwritable
  `title` and `protected` on update, `raw` instead of `html`, `register` without
  a project module, a gem-aware scaffold check and an HTTP-with-token throwaway
  plan of fourteen cases are written into the design;
- the own-namespace fallback is constrained to an emergency exit that removes
  the drop-in-replacement claim before any client or live deployment.

Two decisions go beyond the review:

- the endpoint helper's `params_modifier` runs after parsing and before the
  service, so it carries both the forced `lock_version` and the injected
  `wiki`. No own parse service is needed;
- a project without a wiki returns 404 for both collection `GET` and `POST`.

One decision departs from the review's default: the review preferred text in
every collection element as the less fragile option. The design keeps the thin
element, because a wiki page may hold up to 16 MB of text and the formattable
`html` is rendered per element. The operator may overturn this.

Backlog/resource effect:

- none on dates, hours or dependencies;
- `task.self.design-openproject-wiki-api-access` stays `in_progress`. Its last
  done criterion is operator acceptance of the amended design text;
- `dependency.wiki-api-design-before-plugin-implementation` stays `open`.
  Implementation must not start;
- actuals for the design task are still not booked.

Operator-loop finding recorded during the same session:

The `openproject_network_error` reported by the 2026-09-21 audit and by Step 43
was most likely a missing local protected connection, not an OpenProject
outage. Connection availability should be checked before classifying a live
synchronization failure.

OpenProject sync: pending, unchanged from Step 44.

## Step 47. Wiki API Plugin Design Accepted; Agent-Agnostic Execution Resource

Decision date: 2026-09-21.

Operator decisions:

- the amended wiki API plugin design is accepted;
- collection elements stay thin and text-free. Reason given by the operator's
  framing: the work is done fully, without cutting corners, and a list format is
  hard to change once clients depend on it;
- the plugin is implemented by a different AI agent than the design author.
  The design author acts as reviewer. The handoff rules, three review gates and
  the conformance checklist are written into
  [`../dev/openproject-wiki-api-plugin-design.md`](../dev/openproject-wiki-api-plugin-design.md);
- agent execution is planned on an agent-agnostic resource. This closes the
  open resource-model question from Step 44.

Resource model change:

- new resource `resource.ai.agent.executor`, "AI execution agent", with actor
  `actor.ai.agent.executor`, weekly capacity 40 `agent_hours`, default
  availability and membership in `resource_pool.ai_coding_agents`;
- the question it answers is unchanged from the 2026-07-26 Codex decision: how
  much useful agent work does this task need, not whose subscription limit it
  burns. The 40 `agent_hours` stay a provisional proxy, and subscription-limit
  pressure is still not modeled;
- `resource.ai.codex.local` is kept for history. Its consumed demands and
  actuals are not rewritten;
- `resource.ai.review-assistant` moves from future placeholder to a current
  resource, because a real AI review workflow now exists: the design review of
  Step 45 and the review gates of the wiki API plugin;
- `scripts/review-resource-pool.sh` policy buckets were updated to match: both
  AI resources are `core_now`, and no future placeholder remains.

Backlog/resource effect:

- the five wiki API agent demands moved from `resource.ai.codex.local` to
  `resource.ai.agent.executor` and were renamed from `.codex` to `.agent`;
- three AI review demands were added on `resource.ai.review-assistant`:
  2 `agent_hours` for implementation review, 1 for throwaway evidence review
  and 0.5 for live acceptance review;
- `task.self.design-openproject-wiki-api-access` is `accepted`. Actuals: 3
  `agent_hours` against 2 planned, 1 maintainer hour against 1.5 planned, 0.25
  `review_hours` as planned. The maintainer and reviewer amounts are agent
  estimates of operator time, not measured values. The Step 45 agent review is
  not booked, because the design task had no AI review demand;
- the agent overrun came from the review amendments and from re-checking the
  review against upstream source, which the first estimate did not include.
  That is calibration evidence: a design task that will be reviewed needs an
  amendment allowance;
- the three design demands are `consumed` with `remaining_amount: 0`;
- `dependency.wiki-api-design-before-plugin-implementation` and
  `dependency.wiki-api-design-before-upstream-check` are `resolved`, both
  through accepted `mark_dependency_resolved` proposals.
  `proposal suggest-dependencies` does not propose resolution for a `preferred`
  dependency, so the second proposal was written by hand in the same shape and
  accepted through the normal command;
- `proposal suggest-dependencies --write-candidates` also wrote six candidates
  outside this slice, two of them for the release-story hygiene items of the
  2026-09-21 audit. They were removed again so that this step does not pre-empt
  the re-entry task;
- `replan validate`, `replan audit verify`, `check-resource-schedule-drift.sh`
  and `review-resource-pool.sh` pass.

Dogfood finding:

Closing the task exposed `GAP-015` in
[`../dev/known-gaps.md`](../dev/known-gaps.md): audit ids of three `actual`
commands collide when a batch shares one `--occurred-at`, and the failing
command still writes while reporting `applied: false`. The duplicate ids were
repaired in data. The code was not changed in this step.

Booking the honest 3 `agent_hours` against 2 planned exposed `GAP-016`:
tracking suggestions cannot acknowledge a reviewed overrun, and the MVP smoke
asserted an empty suggestion list for the self-dogfood project. The smoke now
expects exactly that one review suggestion. The actual was not lowered and the
plan was not raised to make the check pass.

Next:

`task.self.implement-openproject-wiki-api-plugin` is unblocked and is handed to
the implementing agent. `task.self.check-upstream-openproject-wiki-api-roadmap`
is unblocked and independent.

OpenProject sync: pending, unchanged. The accepted design task must also be
closed in live OpenProject when the five tasks are created and bound.

## Step 48. Wiki API Plugin Implementation Ready For Gate 1 Review

Decision date: 2026-09-21.

`task.self.implement-openproject-wiki-api-plugin` was implemented on branch
`wiki-api-plugin` by a different AI agent than the design author, following the
handoff protocol in
[`../dev/openproject-wiki-api-plugin-design.md`](../dev/openproject-wiki-api-plugin-design.md).

Delivered:

- separate gem `integrations/openproject/plugins/openproject-wiki_api/`;
- `register` without a project module, permission or `accept_key_auth` patch;
- `GET`/`POST /api/v3/projects/{id}/wiki_pages` and
  `PATCH /api/v3/wiki_pages/{id}`, with `GET /api/v3/wiki_pages/{id}` extended
  in place;
- writes only through `API::V3::Utilities::Endpoints::Create` and `Update`,
  which deduce `WikiPages::CreateService` and `WikiPages::UpdateService`;
- PATCH `params_modifier` `.reverse_merge(lock_version: nil)`;
- payload representer writable set: `title` on create, `text`, `parent`,
  `lockVersion`; unwritable `title` on update, `protected`, `slug`, `author`,
  `wiki`;
- collection via `WikiPage.visible` scoped to the project wiki, ordered by
  title, thin list-item representer without text and without
  `CachedRepresenter`;
- 404 when the wiki module is disabled or the project has no `Wiki` record;
- gem-aware `scripts/check-openproject-plugin-scaffold.sh`;
- multi-plugin `scripts/build-openproject-plugin-image.sh`.

Local checks that passed:

- `./scripts/check-openproject-plugin-scaffold.sh`;
- `go test ./...`;
- `./scripts/smoke-mvp.sh`.

Out of this task, unchanged:

- throwaway HTTP-with-token proof and the four design verification points stay
  on `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime`;
- live OpenProject was not touched;
- `openproject-replan_overlay` behavior was not changed;
- the first image that includes the new gem still needs
  `REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE=deps-base` or `both`, because a
  pre-existing overlay-only deps image does not contain this gem in
  `Gemfile.lock`.

The submission did contain an undeclared deviation, later caught by gate 1:
`engine.rb` `require_relative`d the four files under `lib/api/`. The design
listed those files but did not ask to require them, and OpenProject cannot
resolve the core `API` namespace during `Bundler.require`. Two other
implementation details below are not design deviations:

- `WikiPageUpdatePayloadRepresenter` forces `update: true` in `initialize`, so
  `ParseResourceParamsService` calling `.create(struct, current_user:)` cannot
  leave title writable on PATCH;
- README path assertions in the scaffold check use literal `rg -F` matching,
  because `{id}` is a regex quantifier.

Backlog/resource effect:

- `task.self.implement-openproject-wiki-api-plugin` is `review_ready`. Gate 1
  implementation review is still required before the task can be `accepted`;
- remaining amounts on the implementation demands are left unchanged. Consumption
  actuals are not booked in this step, so review findings can still be done
  against the planned remaining work and so the living self-dogfood tracking
  suggestion set stays the single reviewed overrun from Step 47 (`GAP-016`);
- `openproject_sync_pending` stays.

OpenProject sync: pending, unchanged from Step 44.

## Step 49. Wiki API Plugin Gate 1 Review: Changes Required

Decision date: 2026-09-21.

The gate 1 implementation review of
`task.self.implement-openproject-wiki-api-plugin` is recorded in
[`../dev/openproject-wiki-api-plugin-implementation-review.md`](../dev/openproject-wiki-api-plugin-implementation-review.md).
The reviewed referent is commit `5057064` on branch `wiki-api-plugin`: the
implementer's work, committed unchanged by the reviewer because it had been
left uncommitted.

Outcome: `changes_required`.

- nine of eleven conformance items pass on reading. The API logic follows the
  design: upstream services through the `Create` and `Update` helpers, forced
  `lock_version`, thin text-free list element, injected `wiki`, no new
  permission;
- two blocking defects, both about how the gem's files are loaded. The engine
  `require_relative`s its API classes, but upstream runs `Bundler.require`
  before the application class and before any autoloader, so the core `API`
  namespace cannot be resolved and OpenProject is expected not to boot. And one
  file defines two constants, so the update payload representer cannot be
  autoloaded once the first defect is fixed;
- four should-fix items: an unproven `writeable:` lambda next to the working
  rename guard, a missing `view_wiki_pages` check on the collection, an
  unconditional `update` link on a cached representer, and a scaffold check
  that asserts strings but no load structure;
- Step 48's statement that no design deviation was made is wrong: the four
  requires were an undeclared addition.

Correction to the process, not only to the code:

- the scaffold check, `go test ./...` and the MVP smoke all passed on a
  submission that is expected not to boot. None of them loads Ruby against
  OpenProject. Gate 1 is therefore a reading gate, and the first real proof is
  the throwaway runtime. The throwaway plan now starts with an explicit boot
  proof;
- both blockers trace back to a design omission: the design said what to build
  but not how OpenProject loads plugin code. The loading rule is now written
  into the design, together with a `403` for collection `GET` without
  `view_wiki_pages` and a `_links.parent` throwaway case that the original
  fourteen cases lacked.

Backlog/resource effect:

- `task.self.implement-openproject-wiki-api-plugin` moves from `review_ready`
  back to `in_progress`;
- 1 `agent_hours` of AI review is booked against
  `resource_demand.wiki-api-plugin.ai-review`, 2 planned; the remaining 1
  covers the re-review, which is a diff review against `5057064`;
- no other amounts, dates or dependencies change. If the fixes and the
  re-review do not fit `2026-09-23` to `2026-09-25`, the throwaway task moves
  with them, and the local development slot conflict of Step 44 must be
  re-checked;
- operator acceptance is not requested yet.

OpenProject sync: pending, unchanged.

## Step 50. Wiki API Plugin Gate 1 Blockers Fixed

Decision date: 2026-09-21.

The implementing agent applied the gate 1 required changes against commit
`5057064` / review `90f672a`.

Blocking:

- B1: the four `require_relative` lines under `lib/api/` were removed from
  `engine.rb`. The API classes are left to `ActsAsOpEngine` autoload, as the
  bundled `budgets` module does;
- B2: `WikiPageUpdatePayloadRepresenter` moved to
  `lib/api/v3/wiki_pages/wiki_page_update_payload_representer.rb`.

Should-fix, all applied:

- S1: the unproven `writeable:` lambda on `title` was removed; rename
  rejection stays only in `reject_unwritable_properties!`;
- S2: collection `GET` calls `authorize_in_project(:view_wiki_pages, project:
  @project)` after the missing-wiki 404;
- S3: `link :update` uses `cache_if` with
  `current_user.allowed_in_project?(:edit_wiki_pages, represented.project)`;
- S4: the scaffold check forbids engine requires of `lib/api` files and
  requires exactly one Zeitwerk-named class per file under `lib/api/`.

Notes addressed without being blockers: README now states that `lockVersion` on
POST is 422 from the create contract, and the image-name comment in the build
helper records why the overlay-era tags stay.

Local checks that passed after the fix: scaffold, `go test ./...`,
`./scripts/smoke-mvp.sh`. Throwaway HTTP and live OpenProject remain later
tasks. Overlay plugin behavior is unchanged.

Backlog/resource effect:

- `task.self.implement-openproject-wiki-api-plugin` returns to `review_ready`
  for the short diff re-review against `5057064`;
- remaining planned amounts are unchanged; consumption actuals are still not
  booked;
- `openproject_sync_pending` stays.

OpenProject sync: pending, unchanged.

## Step 51. Wiki API Plugin Gate 1 Re-Review: Accepted With Notes

Decision date: 2026-09-21.

The fixes in commit `ee451fb` were re-reviewed as a diff against `5057064`. The
result is appended to
[`../dev/openproject-wiki-api-plugin-implementation-review.md`](../dev/openproject-wiki-api-plugin-implementation-review.md).

Outcome: `accepted_with_notes`. Gate 1 is closed.

- both blockers and all four should-fix items are fixed as asked, and the
  notes were addressed;
- `cache_if` and `authorize_in_project` were checked against upstream
  `v17.6.0`: the work-package representer and the `budgets` module use the
  identical forms;
- the new scaffold assertions were mutation-tested on a scratch copy.
  Reintroducing either blocker makes the check fail with a precise message;
- eleven of eleven conformance items pass on reading.

What gate 1 still cannot say: whether the plugin runs. Boot, route patching,
representer caching and the writable parent link are throwaway cases and go to
gate 2.

One review cycle cost: the first pass found load-order defects that no local
check could see, and one fix round closed them. That cycle is the price of the
design omission recorded in Step 49, and it was cheaper here than in the
throwaway runtime, where the symptom would have been a container that does not
start.

Backlog/resource effect:

- `resource_demand.wiki-api-plugin.ai-review` has 1.5 `agent_hours` booked
  against 2 planned. It stays `active` with 0.5 remaining until the operator
  decides on the task, and is closed together with the task;
- the reviewer first closed that demand as `consumed` through
  `actual close-demand`. That was premature: the calibration planner rightly
  flags a closed demand on an open task, and the MVP smoke failed on it. There
  is no command to reopen a demand, so the status was set back to `active` by
  hand. The `actual_close_demand` audit record of `2026-09-21T17:32` therefore
  describes a closure that was reverted; this paragraph is the record of the
  revert;
- a second wrinkle surfaced on the way: `remaining_amount: 0` is not written to
  YAML at all, so an `active` demand with nothing left is indistinguishable from
  one whose remaining work was never estimated, and the planner flags it. Both
  observations are added to `GAP-016` as lifecycle-ergonomics evidence;
- `task.self.implement-openproject-wiki-api-plugin` stays `review_ready`.
  Accepting it is the operator's decision. The agent review informs it and does
  not replace it;
- on operator acceptance: book the implementer's actuals, close the remaining
  demands, accept the task and resolve
  `dependency.wiki-api-plugin-before-throwaway-runtime`;
- branch `wiki-api-plugin` is not merged into `main`.

OpenProject sync: pending, unchanged.

## Step 52. Wiki API Plugin Implementation Accepted; Throwaway Proof Started

Decision date: 2026-09-21.

Operator decisions:

- `task.self.implement-openproject-wiki-api-plugin` is accepted on the gate 1
  re-review recommendation;
- the implementing agent's time is 2 `agent_hours`, given by the operator;
- the throwaway runtime proof is executed by the design author, not by the
  implementing agent.

Consequence for gate 2, stated plainly: the design's handoff protocol made
author and reviewer different agents on purpose. For the throwaway task the
operator chose otherwise, so the agent that wrote the design and reviewed the
code also produces the runtime evidence. Gate 2 is therefore not an independent
review. What keeps it honest is the evidence form: every HTTP case is recorded
with its request, status code and the asserted response fields, so the operator
or any other agent can re-run and check it.

Backlog/resource effect:

- actuals for the implementation task: 2 `agent_hours` against 6 planned, 0.5
  maintainer hours against 3, 0.25 `review_hours` against 0.75, 1 local
  development slot as planned, 1.5 AI review `agent_hours` against 2. The
  maintainer and reviewer amounts are agent estimates of operator time;
- the estimate was three times too high for agent work and six times too high
  for maintainer work. With the design fixed to this level of detail, the
  implementation was mostly transcription. That is calibration evidence for the
  next design-then-implement pair: the hours belong in the design and review
  tasks, which ran over, not in the implementation task;
- all five implementation demands are `consumed`; the task is `accepted`;
- `dependency.wiki-api-plugin-before-throwaway-runtime` is `resolved` through an
  accepted proposal;
- `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime` is
  `in_progress`, ahead of its planned `2026-09-28` start. The plan dates are
  not rewritten: an early start is recorded as operational start, as in Step
  16;
- branch `wiki-api-plugin` is merged into `main`.

`GAP-015` was hit a second time while closing the task, this time through a
reviewer scripting error that gave a whole batch one `--occurred-at`. Seven
duplicate audit ids were disambiguated in data the same way as before. Two
occurrences in one day, one of them by the agent that had just documented the
workaround, is the argument for fixing the ids in code rather than relying on
the workaround.

OpenProject sync: pending, unchanged.

## Step 53. Wiki API Plugin Proven In A Throwaway Runtime

Decision date: 2026-09-21.

`task.self.verify-openproject-wiki-api-plugin-throwaway-runtime` was executed
by the design author on the operator's instruction. The result is recorded in
[`../dev/openproject-wiki-api-throwaway-runtime-test.md`](../dev/openproject-wiki-api-throwaway-runtime-test.md).

Result:

- a custom image from the exact OpenProject `17.6.0` builder and slim pair
  builds with both plugin gems;
- seeder, web, worker and cron boot with no load error;
- fifteen HTTP cases with API tokens pass: 37 requests, 57 checks;
- the database facts pass: no `Wiki` row is created by the 404 cases, and an
  agent edit of a page written by another user is journal version 2 under the
  agent's name next to the human's version 1;
- all four design verification points hold. The own-namespace fallback was not
  needed and the drop-in-replacement claim stands;
- the overlay plugin routes work in the same image.

The proof is one repeatable command,
`./scripts/check-openproject-wiki-api-throwaway-runtime.sh`, with a seed script
and an HTTP case script under `scripts/lib/`.

What the runtime found that reading did not:

- the gem's module was named `OpenProject::WikiApi`. OpenProject's inflector
  requires `OpenProject::WikiAPI` for a `wiki_api` directory. The image build
  and the seeder passed; eager loading failed with `NameError`, so on the live
  instance the Rails services would not have started. The implementer, the gate
  1 review and the gate 1 re-review all missed it. The module was renamed and
  the scaffold check now forbids the wrong spelling;
- the reviewer had encoded the same `_api` rule in the scaffold check for
  `lib/api/` files one step earlier and did not apply it to the namespace
  directory. Knowing a rule and applying it everywhere it applies are different
  things, and only execution showed the difference;
- a successful image build is not a boot proof. This goes into the live
  procedure: run the throwaway check against the exact image tag to be deployed.

Process note: five runs were needed. One failed on the plugin defect above.
Three failed on errors in the proof itself: a missing `workspace_type` in the
seed, requests to `127.0.0.1` against a host-name check configured for
`localhost`, and two wrong expectations about attachment status and journal
aggregation. A proof script is code and has its own defects; they were cheap
because the environment is disposable.

Independence: this step is not an independent gate 2 review. Author, code
reviewer and proof executor are the same agent, by operator decision recorded
in Step 52. The evidence is kept re-runnable for that reason.

Backlog/resource effect:

- 3 `agent_hours` are booked against 3 planned on
  `resource_demand.wiki-api-throwaway-runtime.agent`, and the local development
  slot is booked as used. Both amounts are agent estimates;
- the AI review demand of this task is left untouched: no independent review
  took place, and booking one would misstate what happened;
- the task moves to `review_ready`. Accepting it is the operator's decision;
- demands stay `active` until the task is accepted, following the lesson of
  Step 51. Booking actuals equal to plan on a still-open task turned out to
  have no command-reachable state that passes tracking, calibration and
  schedule checks together. An explicit `remaining_amount: 0` was written by
  hand for the two booked demands; the full analysis is the third evidence
  block of `GAP-016`;
- the work is on branch `wiki-api-throwaway`, not merged into `main`.

OpenProject sync: pending, unchanged.

## Step 54. Wiki API Plugin Deployed To The Live Instance

Decision date: 2026-09-21.

The operator accepted the throwaway proof and ordered live deployment, ahead of
the planned `2026-10-05` window. The result is recorded in
[`../dev/openproject-wiki-api-live-deployment-result.md`](../dev/openproject-wiki-api-live-deployment-result.md).

Result:

- verified backup first; the exact image that passed the throwaway proof was
  transferred, not rebuilt, and verified on the host by content hash;
- guarded switch with the watchdog paused and automatic rollback armed. Web was
  healthy after 60 seconds, rollback was not needed, no load error in any
  service log, watchdog restored;
- both plugins are registered in the live web process and the wiki routes are
  present;
- read-only HTTP verification with an existing token: all three project
  collections and all five existing pages return the new representation.

What only the live instance could show:

- one page was served in the old text-less shape with status 200. The cached
  representer's key does not depend on which plugins extend the representer, so
  an entry written before the plugin was installed was served unchanged. The
  throwaway runtime starts with an empty cache and cannot show this. The entry
  was deleted, a sweep over all pages and languages found no other, and every
  page was then verified over HTTP;
- `Rails.cache.clear` from a runner did not remove that entry; deleting the
  exact key did. Not investigated further;
- one perimeter path rejected the RePlan token at the reverse proxy while the
  protected operator path worked. This was outside the plugin and was not
  changed.

Each stage of this slice found something the previous stage could not: the
design review found contract errors, the code review found load-order errors,
the throwaway runtime found the inflection error, the live instance found the
stale cache. None of the four would have been found by doing the previous
stage more carefully.

Backlog/resource effect:

- `task.self.deploy-openproject-wiki-api-plugin-live` is `review_ready`. The
  deployment is done; acceptance against the issue criterion by the affected
  project's agent and a human is open, and the operator chose to observe it in
  live use;
- 1.5 `agent_hours` are booked against 3 planned, 0.5 kept as remaining for
  acceptance support. The amount is an agent estimate;
- follow-up candidate, not yet a task: make the plugin part of the representer
  cache key so installation and removal invalidate old entries;
- the work is on branch `wiki-api-live`.

OpenProject sync: pending. The successful protected connection supported the
Step 46 conclusion that the earlier `openproject_network_error` was local.
Synchronizing the wiki API tasks into live OpenProject was not part of this
order and was not done.

## Step 55. Post-Pause Stabilization Backlog From The Audit Review

Decision date: 2026-09-24.

The 2026-09-21 project audit was reviewed critically against the repository,
the canonical data and GitHub. The operator accepted the resulting plan.

Review findings that change the plan:

- CI on `main` has failed on every push since 2026-08-07. The plugin scaffold
  check calls `rg`, which the hosted runner does not provide, so the MVP smoke
  stops before its first assertion. The audit could not list runs, recorded
  CI as an acceptable limitation and inferred green CI from local runs. Every
  "green HEAD" statement since 2026-08-07, including the ones made during the
  wiki API slice, rested on local evidence only;
- the audit consumed resources on the re-entry task without recording actuals
  or evidence, and answered none of the eight re-entry questions;
- the `openproject_network_error` it reported as a blocker was a missing local
  protected connection, not a service outage;
- it recommended dry-run-by-default for all mutating commands as the first
  follow-up. Seven weeks of dogfood produced no accidental write, while
  GAP-015 showed a command that reports `applied: false` and still writes.
  Write atomicity comes first; defaults are not changed in this block;
- its confirmed low-risk findings stand: missing `actual close-demand` in the
  usage text, `gofmt` drift, `scenario apply` audit ids without the scenario
  id, duplicated OpenProject auth header construction, response-body echo in
  helper errors, the duplicated `GAP-013` id, the stale
  `getting-started.md` inventory, stale plan metadata, two hygiene
  dependencies and one stale proposal.

The wiki API slice already exercised planning, resource checking, execution,
actuals, evidence and gated acceptance through RePlan for four tasks. What the
full dogfood cycle has still not proven is choosing the next work from RePlan
reports instead of from chat.

Backlog/resource effect:

- `task.self.fix-ci-smoke-ripgrep-dependency`, `2026-09-24`: portable
  `grep` in the scaffold check, green CI as evidence;
- `task.self.reenter-openproject-dogfood-cycle-after-pause` is rebaselined
  from `2026-09-18..20` to `2026-09-24..25` with a concrete scope: connection,
  operator report and drift, create and bind every sync-pending work package
  and relation, resolve the two hygiene dependencies, close the stale
  proposal, move `default_status_date`, record the audit's consumption as
  actuals and evidence, refresh overlay artifacts and record the decision on
  `task.self.run-full-openproject-dogfood-cycle`. The recommended decision is
  accept, with the wiki API slice as proof, and carry the unproven part in the
  next-slice task below. Its demands are left at the original estimate on
  purpose, so the recorded actuals show how far the scope grew;
- `task.self.fix-actual-audit-ids-and-demand-reopen`, `2026-09-28..10-01`:
  GAP-015, GAP-016, a demand reopen command, overrun acknowledgement,
  `remaining_amount: 0` serialization and scenario ids in `scenario apply`
  audit ids; requires green CI;
- `task.self.clean-post-pause-audit-hygiene`, `2026-10-02`: the confirmed
  low-risk findings plus a dated review addendum to the audit record;
  dry-run defaults explicitly out of scope; prefers green CI;
- `task.self.evaluate-code-graph-quality-tooling`, `2026-09-24..25`,
  `in_progress`: decide whether graph- and semantics-based code analysis is
  worth using on the Go codebase, with another code-analysis project's tool
  survey as the candidate source;
- `task.self.select-next-product-slice-from-replan-reports`,
  `2026-10-05..06`: choose the next product fork from RePlan outputs and
  record the report snapshot; requires re-entry and the actual-audit fix,
  prefers the hygiene task and the tooling evaluation;
- plan `updated_at` moves to 2026-09-24. `default_status_date` moves during
  re-entry execution, not here.

The first resource schedule check reported `needs_calibration`: the re-entry
task and the actual-audit fix both reserved the single local development
environment slot in `2026-W40`. Re-entry was shortened to finish on
`2026-09-25`; the check then returned `aligned`.

OpenProject sync: pending.

All five new tasks carry `openproject_sync_pending`. Creating and binding them
in live OpenProject is part of the re-entry scope. The sanitized fixture and
example bindings now contain 65 work packages, `2161` to `2165` for the new
tasks, and the overlay test and smoke count assertions moved from 60 to 65.

## Step 56. Code Quality Tooling Evaluated

Decision date: 2026-09-24. Operator decision on the recommendation: open.

`task.self.evaluate-code-graph-quality-tooling` ran a bounded trial on a
`git archive` copy and recorded the result in
[`../dev/code-quality-tooling-evaluation-2026-09-24.md`](../dev/code-quality-tooling-evaluation-2026-09-24.md).
The task is `review_ready`; the agent demand is booked at 1.25 of 2 planned
agent-hours with remaining 0; the maintainer review hour is still open.

Short result: Go-native analyzers give cheap, checkable findings; graph tools
are useful for ad hoc impact questions only; that project's tool does not
apply to a Go codebase. The largest structural finding is 3 916 lines of Python embedded in
shell heredocs, invisible to every analyzer.

Calibration observations from the same step:

- `actual set-remaining --amount 0` again dropped `remaining_amount` from the
  demand instead of writing zero (GAP-016). It was restored by hand;
- the CLI rewrite of `tasks.yaml` changed the quoting of seven hand-written
  task texts without changing their content. This is the storage rewrite
  noise the 2026-09-21 audit described, now observed in practice.

Backlog effect: none yet. Recommendations 1 to 4 become tasks only after the
operator accepts them. Recommendation 4, the store update helper, would fold
into `task.self.fix-actual-audit-ids-and-demand-reopen`.

## Step 57. Refactoring Backlog And Gap Records From The Tooling Evaluation

Decision date: 2026-09-24.

The operator accepted the code quality tooling recommendation.
`task.self.evaluate-code-graph-quality-tooling` is `accepted`; its agent
demand is consumed at 1.25 of 2 agent-hours; the maintainer review demand is
closed without booked time, because review time in chat was not measured.

Backlog/resource effect:

- `task.self.add-go-lint-baseline-and-ci-gate`, `2026-10-07..08`: pinned
  `golangci-lint` baseline, gate on new findings, `shellcheck` in CI, the
  small set of real findings fixed; requires green CI, prefers the hygiene
  task;
- `task.self.move-heredoc-python-into-script-modules`, `2026-10-12..16`: the
  22 heredoc blocks become tested modules under `scripts/lib/`, one auth
  header, no raw error bodies; requires the re-entry task, because it changes
  the live OpenProject helpers that re-entry uses; prefers the lint gate;
- the store update helper is folded into
  `task.self.fix-actual-audit-ids-and-demand-reopen`, whose agent demand grows
  from 5 to 6 agent-hours;
- `dependency.code-graph-evaluation-before-next-product-slice` is resolved by
  hand, because `proposal suggest-dependencies` offers candidates for
  `required` dependencies only (GAP-024);
- the known-gaps register gains GAP-017 to GAP-024, and GAP-015 and GAP-016
  gain backlog links and new evidence.

The first resource schedule check reported `needs_calibration`: the lint task
overlapped the local development slot of
`task.self.deploy-openproject-wiki-api-plugin-live` in `2026-W41`. That task
was executed on 2026-09-21, two weeks ahead of its planned window, and its
slot was already used. `set-remaining 0` did not release it, because the
schedule check treats a zero remainder as absent and books the planned
amount (GAP-016, fourth evidence). The demand was first closed as
`consumed`. The check then reported no overload, but the calibration planner
reported a closed demand on an open task, and the MVP smoke failed on it. That
commit, `65d6ea5`, had already been pushed: the smoke was read from a
truncated pipe and its failure was missed.

Correction in the next commit:

- the schedule check now treats an explicit `remaining_amount: 0` as zero
  load instead of falling back to the planned amount. The earlier rule was
  documented in the sync plan and is updated there;
- the demand was reopened by hand to `active` with an explicit
  `remaining_amount: 0`, because there is no reopen command;
- the schedule check and the calibration planner both return `aligned`, and
  tests and the MVP smoke pass.

The smoke was not loosened. The data now says what happened: the slot was
used and nothing remains. The fragile part stays: the Go store drops the
explicit zero on its next rewrite of the file, until the actual-audit fix
lands.

OpenProject sync: pending. Both new tasks carry `openproject_sync_pending`.
The sanitized fixture and example bindings contain 67 work packages, `2166`
and `2167` for the new tasks; the overlay test and smoke counts moved from 65
to 67.

## Step 58. Post-Pause Stabilization Executed: CI, Actuals Fix, Hygiene, Re-Entry

Decision date: 2026-09-24. The operator ordered the whole stabilization
backlog executed ("делай все полноценно, даю добро на все" — "do everything
properly; you have my approval for all of it").

Executed and accepted, in this order:

1. `task.self.fix-ci-smoke-ripgrep-dependency`: portable `grep` in the
   scaffold check; a second silent-pass defect found on the way, where a
   failing `find` inside a process substitution checked zero files and
   passed. CI green on `0572495`, the first green run since 2026-08-07.
2. `task.self.fix-actual-audit-ids-and-demand-reopen`, moved ahead of
   re-entry: re-entry needed many CLI writes, and every write through the Go
   store dropped hand-restored `remaining_amount: 0` values within the hour.
   Staged writes with validation before commit, target-named audit ids,
   pointer remaining amounts, `reopen-demand`, `acknowledge-overrun`,
   GAP-024. A second-pass review by a separate agent session accepted it with
   notes; its four should-fix findings were fixed with tests in `6644d6c`.
   Review record:
   [`../dev/actuals-staging-review-2026-09-24.md`](../dev/actuals-staging-review-2026-09-24.md).
3. `task.self.clean-post-pause-audit-hygiene` in `70a43b4`.
4. `task.self.reenter-openproject-dogfood-cycle-after-pause`.
5. `task.self.run-full-openproject-dogfood-cycle`: accepted with the wiki API
   slice and the re-entry as proof; its own demands cancelled to avoid double
   counting; report-driven next-step selection carried by
   `task.self.select-next-product-slice-from-replan-reports`.

Re-entry result:

```text
check-openproject-drift:
  drift_status: soft_drift
  local_tasks: 67            live_task_candidates: 67
  local_dependencies: 93     live_dependency_candidates: 93
  unbound, missing, extra, title and date drift: 0
  status_drifts: 1           (GAP-026, expected)
```

- 13 work packages created one by one and bound; 16 relations applied after
  a dry run matched 77;
- applying relations made OpenProject reschedule successors. The root cause
  was the August backlog sync task, done on 2026-08-16 but still planned for
  2026-11-02 (GAP-028). Dates were rebaselined through two audited scenarios,
  accepted tasks to their actual days, and pushed; OpenProject's Monday for
  the Sunday August date was accepted back;
- the status helper's default `ready_only` scope was widened to `all` after
  reviewing the four changes one by one; three reached `Closed`;
- the live workflow has no route to `Developed`, so the review-ready
  deployment task stays `In progress` (GAP-026);
- the live overlay had carried sanitized public ids since August, so the
  work-package panel could not match real work packages (GAP-027, closed).
  A binding-aware builder produced 67 linked rows; after a backup of the live
  overlay directory the artifact was replaced in place, and a Rails runner
  check matched all 67 real work-package ids;
- the stale open proposal was rejected and 12 dependency proposals were
  accepted; `validate` now reports no warnings;
- `default_status_date` and the plan `updated_at` are 2026-09-24;
- `openproject_sync_pending` is gone from every task.

Resource calibration:

| Task | Planned agent-hours | Booked |
| --- | --- | --- |
| CI fix | 0.5 | 0.75, overrun acknowledged |
| actuals fix | 6 | 4 |
| second-pass review | 1 | 0.1 |
| hygiene | 2 | 1 |
| re-entry | 1.5 | 3.25, overrun acknowledged |

The re-entry overrun is the expected signal: its estimate was left unchanged
in Step 55 while the scope grew. It includes an estimated 1.5 agent-hours for
the 2026-09-21 audit, whose own time was never recorded. Human maintainer and
reviewer time was not measured and is not booked; those demands were closed
with that reason instead of an invented number.

Observations:

- the smoke depended on the living project's state twice: once on a demand
  being active, once on the sanitized fixture's dates. Both are now set up or
  mirrored explicitly;
- `tracking suggest` flagged the full-cycle task as accepted without actuals
  although all its demands were cancelled. Tasks whose demands are all
  cancelled no longer expect actuals.

Still open in this block: `task.self.check-upstream-openproject-wiki-api-roadmap`,
`task.self.add-go-lint-baseline-and-ci-gate`,
`task.self.move-heredoc-python-into-script-modules`,
`task.self.select-next-product-slice-from-replan-reports`, and live
acceptance of `task.self.deploy-openproject-wiki-api-plugin-live`.

## Step 59. Stabilization Block Closed; Next Slice Selected From Reports

Decision date: 2026-09-24.

Executed after Step 58, all on 2026-09-24:

- `task.self.check-upstream-openproject-wiki-api-roadmap`: OP #39099 has
  been in the upstream Wish List since 2021 with five votes; nothing on the
  2026 roadmap; the documents API exists live, read and PATCH only. The
  plugin is planned for a long lifetime. The operator's vote and comment
  remain; a draft is in
  [`../dev/openproject-wiki-api-gap.md`](../dev/openproject-wiki-api-gap.md).
  Its agent demand stays active with an explicit zero, the first real use of
  `reopen-demand --remaining 0` for a finished slice on an open task;
- `task.self.add-go-lint-baseline-and-ci-gate`, accepted: pinned
  golangci-lint gate on new findings, shellcheck in CI, 27 findings recorded
  as baseline in
  [`../dev/lint-baseline-2026-09-24.md`](../dev/lint-baseline-2026-09-24.md);
- `task.self.move-heredoc-python-into-script-modules`, accepted: 25 blocks,
  4 527 lines, byte-identical helper output including live dry runs; GAP-018
  closed;
- `task.self.select-next-product-slice-from-replan-reports`: the reports,
  not chat, selected the next slice. At status date 2026-09-24 the state
  report showed a July week and only acknowledged overruns as next actions,
  and missed every item waiting on a person. Decision record:
  [`../dev/next-product-slice-decision-2026-09-24.md`](../dev/next-product-slice-decision-2026-09-24.md).
  It stays `in_progress` until the operator has looked at the OpenProject
  Gantt and panels and confirms.

Backlog effect:

- `task.self.improve-state-report-next-actions`, `2026-10-02..07`, created
  in RePlan and in live OpenProject, bound, related to the selection task;
- live OpenProject: 68 tasks, 94 relations, one expected soft drift
  (GAP-026); the live overlay was republished with 68 linked rows after a
  backup.

Resource calibration for this step:

| Task | Planned agent-hours | Booked |
| --- | --- | --- |
| upstream check | 1 | 0.75 |
| lint gate | 3 | 1.25 |
| module move | 7 | 2.5 |
| slice selection | 1 | 0.5 |

Mechanical and tooling tasks came in well under their estimates, while the
two tasks that met live systems for the first time, the CI fix and re-entry,
ran over. That pattern is worth one estimating rule for the next block:
estimate live-system first contact separately from the code change.

Waiting on the operator:

1. live acceptance of the wiki API plugin by the other project's agent;
2. vote and comment on OP #39099;
3. confirmation of the next slice after a look at the OpenProject Gantt and
   RePlan panels;
4. GAP-026: allow the `Developed` status in the shared OpenProject workflow,
   or accept that `review_ready` maps to `In progress`.

## Step 60. Operator Answers: Plugin Accepted, Developed Allowed, Delete Gap Queued

Operator answers on 2026-09-24 to the four open items of Step 59:

- the wiki API plugin works on the live instance, checked by the other
  project's agent with its own member token.
  `task.self.deploy-openproject-wiki-api-plugin-live` is accepted, its open
  demands are closed (the review demands as `cancelled`, because live use was
  the acceptance), OpenProject shows it `Closed`, and GitHub issue 1 is
  closed;
- GAP-026 is closed by allowing `Developed` in the OpenProject workflow for
  the `Member` and `Project admin` roles, after a backup of the workflow
  table. A check as the operator's user lists `Developed` among the
  assignable statuses;
- the operator could not see the dogfood project in OpenProject. The
  project was intact; the operator's own user was a member of the other
  project only. It is now a `Member` of the dogfood project too;
- voting on OP #39099 still waits on the operator: the upstream tracker is
  not on GitHub (`opf/openproject` has issues disabled), so `gh` cannot
  reach it.

The acceptance surfaced GitHub issue 2: the plugin can create pages but not
delete them, so the member-token create check was skipped. It is queued as
`task.self.add-wiki-api-page-delete`, `2026-09-28..10-01`, in RePlan and in
live OpenProject, bound, before the state-report slice. Its window was picked
by the agent to use the free days before `2026-10-02`; the operator may move
it.

Live OpenProject after this step: 69 tasks, 94 relations, drift `aligned`
with no soft drift; the live overlay was republished with 69 linked rows.

## Step 61. Wiki Page Delete Route Built, Proven And Deployed

The operator confirmed the order on 2026-09-25: the wiki page delete route
first, then the state-report slice. `task.self.select-next-product-slice-from-replan-reports`
is accepted, and the dependency to the state-report slice is resolved by
proposal.

`task.self.add-wiki-api-page-delete` was started the same day instead of
`2026-09-28` and reached `review_ready`:

- `DELETE /api/v3/wiki_pages/{id}` under `manage_wiki`. GitHub issue 2 asked
  for a separate `delete_wiki_pages` permission. OpenProject 17.6 removed that
  permission and folded it into `manage_wiki`, which is also what the browser's
  delete needs, so the plugin follows the browser. A page with child pages is
  refused with 422;
- throwaway proof 19 of 19 HTTP cases, with the database showing no leftover
  rows, journals or attachments of deleted pages;
- live deployment with backup, the proven image, a guarded switch and a cache
  sweep; live create-then-delete with the admin token passed;
- first live use of `Developed`: the work package went `New -> In progress ->
  Developed`, because the workflow opened on 2026-09-24 has no direct
  `New -> Developed` transition.

Three process slips, all caught before any live change or commit: the switch script built
its backup file name from the full image name, which contains a `/`, and the
backup step read a database user variable that the database container does not
set. Both stopped the script with `set -e` before the override was touched.

Resource calibration: 2.5 of 3 agent-hours booked, 0.25 left for the
member-token check.

Waiting on the operator: whether the affected project's agent role gets
`manage_wiki`. Without it that agent can create pages but not delete them,
the same as a human member without that permission in the browser.

Closed later on 2026-09-25: the operator granted `manage_wiki` to the affected
project's agent role, which only that project's agent account holds, after a
backup of the role's permissions. A temporary token of that account passed
create-then-delete live and was removed. `task.self.add-wiki-api-page-delete`
is accepted and `Closed`, GitHub issue 2 closed; 2.75 of 3 agent-hours
booked. Closing its demands surfaced GAP-029: `close-demand --status consumed`
keeps a stale remaining amount, which the smoke caught twice.

## Step 62. The State Report Answers What To Do Next

`task.self.improve-state-report-next-actions` started on 2026-09-25 instead
of `2026-10-02` and reached `review_ready` the same day:

- the state Markdown was pinned by a golden file first, as its own commit;
- the default week follows the project status date; `tracking suggest` shares
  the same default and is fixed with it;
- next actions carry who can act and a date, list what waits on a person
  first, skip acknowledged overruns, and show a task dependency only when it
  can be resolved;
- backtest on the 2026-09-24 snapshot: the old report showed `2026-W30` and
  three acknowledged overruns; the new one shows `2026-W39` and exactly the
  three items that waited on the operator. The dependency-noise rule came from
  this backtest: the first version still listed a dependency whose
  predecessor was not done.

Resource calibration: 2.5 of 4 agent-hours booked.

Run on the live dogfood data after the change, the report's only
person-facing item is this task's own review, which is correct.

Operator review, same day: read in a terminal, the full state report was a
long dump with the answer at the end, which the operator could not use. The
report answered the right question in the wrong shape. Added `replan next`,
a short English view with task titles and no tables; English because RePlan is
planned as open source for a global audience. 3.5 of 4 agent-hours booked.
The task stays `review_ready` for the operator's look at `replan next`.

## Step 63. State Report Accepted; GAP-029 Fixed And Used At Once

The operator accepted `task.self.improve-state-report-next-actions` after
using `replan next`, and ordered GAP-029 next.
`task.self.fix-close-demand-stale-remaining` was created in RePlan and live
OpenProject and built the same day: `close-demand --status consumed` now
clears the remaining amount in the same write and audit record. Its first real
use was closing the accepted state-report task's demands: remaining 10, 1 and
0.5 went to zero in one command each, where the previous task needed a reopen
and a second close for each.

One slip on the way: the new task was first written with
`tracking_status: in_progress` straight into YAML, which would have skipped the
audit trail; it was reset to `not_started` and started through
`actual add-status`.

## Step 64. The Status Date Moves Through A Command

The operator accepted `task.self.fix-close-demand-stale-remaining` and asked
to continue with the recommended step. `task.self.set-project-status-date-command`
was created in RePlan and live OpenProject and built the same day:
`replan project set-status-date --date <YYYY-MM-DD|today>` with dry run,
actor, reason and one audit record, changing only `default_status_date` in
`project.yaml` through a YAML node edit. Its first use moved the dogfood
project from 2026-09-24 to 2026-09-25, a one-line diff; `replan next` then
showed the new date and nothing waiting.

Noticed on the way: the work-package create helper returned the new id in
`actions[0].work_package_id` with an empty `items` list, where the previous
two creates had filled `items`. Reading the wrong field made the follow-up
bind fail before any second create, so no duplicate was made. Scripts that
read the create result should take the id from the action.

## Step 65. Accepted Tasks Get Their Actual Windows

The operator accepted `task.self.set-project-status-date-command` and asked to
continue fully with GAP-028. `task.self.warn-accepted-task-future-window` was
created in RePlan and live OpenProject and built the same day: a validation
and report warning for accepted tasks planned past the status date, and a
`record_actual_window` next action for an agent.

The first run was the finding: 16 accepted tasks still carried planned
windows after 2026-09-25, eleven of them the August plugin series planned into
October and November. The status date command of Step 64 made this visible:
the check compares with the status date, and the date was current for the
first time without a hand edit. Actual windows came from the tasks' own
actuals and acceptance audit records, one scenario for all sixteen. Live
OpenProject took the dates one work package at a time through the guarded
update helper, because the date push helper routes completed-task changes to
review by design. OpenProject moved five weekend dates (15-16 August) to
Monday 17 August; those were accepted back through a second scenario, the same
rule as in Step 58. Validation now reports no warnings, live drift is
`aligned`, and the sanitized fixture carries the new dates for the sixteen.

## Step 66. Cleanup: One Gap Already Fixed, One Spec Made Honest

The operator accepted `task.self.warn-accepted-task-future-window` and asked
to continue with the recommended cleanup,
`task.self.clean-dead-code-and-workload-spec`, created in RePlan and live
OpenProject:

- GAP-021 turned out to be fixed already: the lint gate commit of 2026-09-24
  had removed the dead functions, but the gap record stayed open. A gap list
  is only as current as the last person who closed a gap in it; closing is
  part of the fix;
- GAP-011: `workload` gained `--resource-class` and `--unit`. The planned
  change to default `--week` to the status date was dropped while reading the
  code: without `--week` the command sums over all weeks, and a default would
  have removed that view silently. The spec now says so, and lists the
  remaining drafted flags as not implemented instead of planned.

1.25 of 2 agent-hours booked.

## Step 67. The Dashboard Pages Get Golden Files

The operator accepted the cleanup of Step 66, decided that RePlan does not go
to a public OSS release yet, and chose the HTML golden tests after a plain
explanation that they are insurance, not a visible feature.
`task.self.pin-dashboard-html-golden` pins `index.html` and `gantt.html` for a
rich and an empty fixture. The fixture was built from coverage, not guessed:
a real render of `small-team` reached fewer statement blocks (141) than the
existing hand-built fixtures (260), so the golden fixture extends those, and
reaches 321 of 505; the rest is write-error returns and defensive clamps.
GAP-005 is closed. 1.5 of 2 agent-hours booked.

## Step 68. The Most Complex Report Writer Is Split

The operator accepted the HTML golden files of Step 67 and, after a plain
list of options, chose the recommended pair: refactor one complexity hotspot
and tidy the gap register, then pause development and use RePlan on real
work. `task.self.split-project-state-markdown` splits
`WriteProjectStateMarkdown` into seven section writers that fill one buffer,
the idiom `replan next` already used. Most of the old complexity was an error
check after every line, not logic. The refactor was proven, not assumed:
three new golden files (edge fallbacks, an empty project, a stale-only loop)
were generated with the old writer before the switch, the new writer matches
all four byte for byte and covers every branch, and the rendered state
reports of both example projects did not change. GAP-017 was still open
although its fix task was accepted on 2026-09-24 and CI has passed on every
push since; it is closed now, the second stale gap record after GAP-021.

## Step 69. Planning Ahead Finds That Capacity Dates Were Ignored

The operator accepted Step 68 and chose to pause feature work and use RePlan
on real work, with RePlan's own next weeks as that work. Asked for the one
number only they know, they gave 5 hours a week, not the 10 recorded since
July. Recording that change was the first planning action of the new mode,
and it failed at once: workload, the dashboard and the Gantt load summed every
capacity record of a resource and never read `effective_from` or
`effective_to`, so a second record would have shown 15 hours.
`task.self.honour-capacity-effective-dates` fixes it: a capacity counts only
for the days it is in effect, the no-week view uses the status date,
validation rejects bad dates, and the TaskJuggler export says what it
simplifies. The maintainer now has 10 hours to 2026-09-27 and 5 from
2026-09-28. All earlier work was planned and done on the same day, so nothing
had ever asked RePlan what next week holds. 1.5 of 2.5 agent-hours booked.

## Step 70. The First Plan Made Ahead, And RePlan's First Objection

With the capacity fixed, the next four weeks went into RePlan and live
OpenProject as six dated tasks with demands, before any of them started. The
operator approved the three refactor and storage tasks and asked to resume the
OpenProject plugin track where it stopped in August: the project page and the
work-package panel are live, and the accepted next step was a Gantt-like
RePlan view inside the plugin. That became three tasks: an operator review of
the live plugin views, a design, and the build, chained by required
dependencies.

The first draft put the `validateWorkspace` split and the plugin build on
the same days. Workload for 2026-W41 reported the local development slot, a
capacity of one, overloaded by one: two tasks cannot use one checkout at the
same time. The refactors moved back one week each, and the plan now runs to
2026-10-20 without overload. The operator's planned review time is 2 of 5
hours in the first week and 0.5 to 1 hour in the others; the rest of the 5
hours covers the daily `replan next` loop and whatever new work appears.

```text
week     task                                            operator  agent
W40      review-openproject-replan-plugin-views          1         0.5
W40      remove-yaml-rewrite-noise (GAP-022)             0.5       3
W40      design-openproject-plugin-gantt-view            0.5       2
W41      implement-openproject-plugin-gantt-view         1         6
W42      split-validate-workspace (GAP-020)              0.5       3
W43      dedupe-import-accept-modes (GAP-020)            0.5       2
```

## Step 71. A Plan Made Ahead Stops Warning About Itself

The plan of Step 70 left two `required_dependency_unresolved` warnings in
`validate`, the state report and `replan next`: the plugin design waits for
the plugin review, and the build waits for the design. That is what a plan
made ahead looks like, not a problem, but every open required dependency
warned regardless of dates, and nothing had shown it while tasks were planned
and done on the same day. On the operator's go,
`task.self.warn-dependency-only-when-due` keeps the warning for stale records,
started or due successors and missing dates, and silences it for future
successors. replan-self is back to zero warnings with the plan in place.
0.75 of 1 agent-hour booked.

## Step 72. The Plugin Seen As The Operator Sees It

The operator asked to go back to the plugin the same evening and wanted an
assessment against what they had asked for, from the code and the live
visuals. `task.self.review-openproject-replan-plugin-views` started three days
early. Sanitized lesson: visual review required an ordinary read-only user and
the same authentication and perimeter controls that real operators face;
admin-only verification had hidden a permissions defect.

Looking as a non-admin found the most important defect at once: the
work-package panel, the second step of the plugin sequence and accepted in
August, returns HTTP 500 for every non-admin, the operator included. The
August verification ran as an admin. The rest of the review
([`../dev/openproject-plugin-review-2026-09-25.md`](../dev/openproject-plugin-review-2026-09-25.md))
measures the project page against the operator's own words: no date axis,
resource bars that ignore their windows, unreadable intensity, no load
profile, and the whole 83-task history on one page. The agent part took 2
agent-hours against 0.5 planned, most of it access. The task stays in
progress until the operator gives their verdict.

## Step 73. The Operator's Verdict, And The Panel Opens For Them

The operator confirmed the review findings and answered its questions: the
view must be able to show every resource a task uses, closed tasks may stay
visible, and they read OpenProject through the Dark Reader extension because
they found no dark theme. The review task was accepted; RePlan itself then
asked for the two follow-ups, the resolved dependency of the design task and
the early actual window, and both went through a proposal and a scenario.

`task.self.fix-plugin-panel-permission` added the missing action to the
permission, made both plugin checks catch the defect (the runtime check now
reproduces the live error without the fix), and deployed the image with the
guarded switch. The panel renders for a non-admin. The review paid for itself
here: the defect had been live since 2026-08-15, and only a look through a
non-admin's eyes could see it.

When the review's early finish was pushed, OpenProject pulled the design and
the build forward along their relations. Accepting both would have put the
build on the development slot the YAML task uses on 1 and 2 October. RePlan
could not preview that: a scenario that moves task dates is refused by the
workload overlay (GAP-030), so the overlap was found by hand. Only the design
move was accepted; the build keeps 5 to 9 October and went back to OpenProject.

## Step 74. The Whole Plugin Track In The Backlog Before Any Of It Is Built

After the visual audit the operator asked for every task of a full plugin
improvement in the backlog first, without cutting corners. Twelve tasks went
into RePlan and live OpenProject with demands, dates and 13 relations, around
the existing design and build tasks, in three milestones, each ending with a
guarded deploy and an operator check:

```text
09-28        workload fix: multi-week and concurrent demands (GAP-031)
09-25..29    design (in progress; the audit is its first input)
09-29..30    rendered-page checks as a member
10-01..02    overlay contract: names, date span, weekly load, evidence, warnings
10-05..06    colours from OpenProject, narrow layout
10-07..09    project page restructured around attention
10-12        milestone 1 deploy and operator check
10-13..16    Gantt-like task view with axis and resource lanes
10-19..22    weekly load profile with a 100% waterline, every resource
10-23..28    panel with evidence, actuals and lanes; 404 for unknown packages
10-29        milestone 2 deploy and operator check
10-30..11-05 reach the panel from the OpenProject work package
11-06..09    each OpenProject project sees only its own overlay
11-10        milestone 3 deploy, audit rerun, track acceptance
11-11..20    the three refactors, moved after the plugin track
```

About 45 agent-hours and 7.5 operator hours in the plugin track. Reading the
new plan exposed GAP-031 at once: every week showed the development slot
overloaded although its tasks follow each other, and the release story's 8
hours counted in both of its weeks. The workload fix therefore goes first.
Once more OpenProject's automatic scheduling pulled a task forward along a
relation, onto a day the development slot was taken; the real ordering,
contract after the rendered checks, became a relation of its own, and the
drift is aligned.

## Step 75. The Plan Reads True

The operator accepted the permission fix, rejected the premature proposal and
gave the go for the whole plugin track. Its first task was RePlan's own:
`task.self.honest-weekly-workload-for-multi-week-and-concurrent-demands`
spreads period demands over working days and checks slots by the peak of
simultaneous use, in workload, the dashboard load and the overlay alike. Its
tests found two more defects of the same family, weekend load in the dashboard
and weekly buckets that ignored capacity changes. On the plan the development
slot shows no false overload in any week, and the only overload left is real:
the operator at 5.6 of 5 hours in 2026-W46, the week the release story and the
plugin track acceptance share.

## Step 76. A Design The Operator Can See

`task.self.design-openproject-plugin-gantt-view` ends in a design document and
a working prototype on the live data, published to the operator as a private
page, because the operator judges views by looking at them. The prototype was
built on OpenProject's own measurements, read from the live instance: 40-pixel
rows, 14-pixel bars, the Primer palette. Building it on real data caught one
more disagreement before any plugin code: task rows marked `dependency_open`
as a warning while `validate` reports none, so the design takes warnings from
`validate` only. The design took 3 agent-hours against 2 planned; the
prototype is the difference. Its acceptance by the operator resolves the
dependency that holds the overlay contract task.

## Step 77. The Operator Steers The Design, And A Proposal Flood Is Stopped

The operator found the prototype fine overall and asked for four things: a
flexible range, a slider to move along time and a jump to the top, the Gantt
on its own page so the dates stay pinned, and a way to drill into a task. A
second prototype version showed all four the same evening, the design was
accepted with them as amendments, and the navigation became
`task.self.add-plugin-gantt-navigation`, inserted after the Gantt build; the
later plugin tasks and the refactors moved three working days.

Resolving the design's dependencies through `proposal suggest-dependencies`
wrote nine review proposals for dependencies of tasks weeks away, the noise
removed from `validate` earlier that day. `task.self.suggest-dependency-reviews-only-when-due`
applied the same due-only rule to review candidates, the nine were rejected
with a reason, and a new run suggests none. The same defect had to be fixed
twice because the rule lived in one place and its twin in another; both now
call one function.

The operator then accepted both fixes and sent a third round on the
prototype: the pinned task column slid away on scrolling right, and they
wanted up to five task pages as closable tabs and a full-screen Gantt. The
defect was a row only as wide as the viewport; the third prototype version
fixed it and showed the tabs and full screen. Both requests went into the
navigation task, whose agent estimate rose from 5 to 6.5 hours. The operator's
review loop on a prototype, three rounds in one evening, cost no plugin code;
each finding would otherwise have been found after a deploy.

A fourth round the same evening asked for viewing area: foldable controls and
header, the extra slider off by default, and a chart fitted to the window. It
went into the prototype and the navigation task without changing its estimate.

## Step 78. Pages Checked As A Member Sees Them

`task.self.add-plugin-rendered-page-checks` opened the plugin track four days
early. The throwaway runtime test now serves OpenProject and requests both
plugin pages over HTTP after signing in through the login form, as a member
with only the RePlan permission, as a member without it and signed out, across
valid, missing and broken overlays, with markup in a task title: 47 checks.
The checks were proved against the defect they exist for: with the permission
of before 2026-09-25 the panel answered 500 to the member, exactly what the
live instance did for six weeks. An unknown work package now answers 404, as
the design says.

Two lessons from closing it. Demands closed before the operator's acceptance
trip the smoke's lifecycle review, rightly; they stay open at zero remaining
until acceptance. And OpenProject scheduled the successor, the overlay
contract, right after the early finish, to 2026-09-28; RePlan took that date
through a scenario after the workload showed room, and GAP-030 again meant the
move could only be checked after applying it.

## Step 79. The Plugin Gets Its Data

The operator accepted the rendered checks the same evening, and
`task.self.extend-overlay-contract-for-plugin-views` followed: the overlay is
version 2, with names, the date span, weekly load and capacity for every
resource, dated evidence and actuals, validate's warnings, proposals and
dependencies per work package. The prototype's generator was the checklist:
everything it read from canonical files by hand is now derived by RePlan.

Two decisions came from first use rather than the design. The weekly bars
show planned load, but the "overloaded weeks ahead" counter showed the current
week overloaded four times over by work already done early; it now asks
`replan workload`, which counts what remains, and only the real overload is
left, the operator in 2026-W46. And the new per-task warnings surfaced a real
one: two dependencies on the accepted rendered checks were still open; they
were resolved through proposals. Accepting a task and resolving what waits on
it are still two steps an agent has to remember; `replan next` would have
listed it the next time it ran.

## Step 80. The Plugin Takes OpenProject's Colours

The operator accepted the overlay contract, and this time the dependency
waiting on it was resolved in the same step. `task.self.theme-plugin-views-with-openproject-variables`
moved every plugin colour to OpenProject's Primer variables and put both pages
under OpenProject's own page header. The runtime test learned to hold its
server and bind a known port, so the pages could be photographed at 1600 and
900 pixels in OpenProject's light theme, its dark theme and under the Dark
Reader engine the operator uses. Before, the dark theme left the plugin's
headings dark grey on black; after, all three read and nothing scrolls
sideways.

Two findings for later tasks: OpenProject's Content Security Policy refuses
inline scripts without its nonce, now part of the Gantt navigation task; and
OpenProject has no Saturday finish, so a task that runs past Friday midnight
is recorded on Friday in both systems. One slip: a sentence added to a task
description broke the YAML with an apostrophe, and `validate` caught it
before anything else did.

## Step 81. A Page That Answers Questions

The operator accepted the theme from screenshots, and
`task.self.restructure-plugin-project-page` rebuilt the project page around
the design's questions: who waits, which weeks are overloaded, what is open,
what warns. The page went from 28 screens to 4.4, with names where ids stood.
The presenter got its own tests, plain Ruby run by the smoke, and one of them
first passed against a deliberately broken sort because the fixture's ids
happened to sort like its dates; the fixture now orders them the other way.

OpenProject's own table style cut every task name with an ellipsis at 300
pixels, which only the screenshots showed; the rendered-page checks read HTML
and could not. The first milestone is now built ahead of plan and waits on its
deploy, which needs the operator's go because it changes the live instance.

## Step 82. Milestone 1 Is Live Sixteen Days Early

The operator accepted the project page and gave the go to deploy. The
milestone planned for 2026-10-12 went live on 2026-09-26 with the same guarded
procedure as before: verified backup, the image the throwaway test had just
rendered, equal content hash, the version 2 overlay first, then the switch
with rollback. The live page, seen as the non-admin review user, shows the
plan RePlan holds: thirteen open tasks and one overloaded week ahead, the
operator in 2026-W46.

Five tasks of 3 to 4 planned agent-hours each took 2.5 to 4 each, close to
plan; what moved the milestone forward was not speed but the order: each task
started the moment the operator accepted the one before it, instead of on its
planned day. The task stays open until the operator has looked at the page on
real work.

## Step 83. What The Operator Expected To See

After milestone 1 went live the operator opened the plugin and found the old
resource strips and no trace of the approved prototype: the milestone had
rebuilt only the top of the project page, and my report had asked them to
"look at the page on real work" without saying what was not there yet. They
asked where the design they had approved was. The plan was right and the
report was not: the Gantt was milestone 2, three weeks away.

Two changes followed the same hour. Plugin parts now go live one by one as
each is ready, so the operator sees the approved view after one task instead
of four; and a deploy report says what is live and what is not yet. The Gantt
page itself, `task.self.implement-openproject-plugin-gantt-view`, was built,
checked and deployed the same day, 5 agent-hours against 6 planned. Its first
real defect was caught before any page existed: the scaffold check matched a
fixed list of permitted actions, so adding the Gantt action would have passed
the check while OpenProject refused the page to members, exactly as the panel
had been refused for six weeks; the check now derives the list from the
controller.

## Step 84. A Script OpenProject Lets Run

The operator found the live Gantt fine for now, and
`task.self.add-plugin-gantt-navigation` gave it the prototype's navigation:
ranges, scale, jumps, a slider, folding, full screen and task tabs. It is the
first plugin code that runs in the browser, and OpenProject's Content Security
Policy, noticed two tasks earlier when a script injected for screenshots was
refused, was built in from the start: the rendered checks compare the
script's nonce with the page's policy header, and a browser check of the
script's twenty behaviours fails at once when the nonce is removed. The
browser check also found what no HTML check could: OpenProject navigates with
Turbo, which leaves the referrer empty, so closing a task tab returned to the
overview instead of the Gantt it was opened from. 5.5 agent-hours against
6.5 planned; deployed the same evening.

## Step 85. A Red Hump That Agrees With The Counter

The operator accepted the navigation, and
`task.self.add-plugin-resource-load-profile` drew the view they had described
in most detail back on 2026-08-06: per resource, weekly bars of planned load
over capacity with a 100% waterline and overload as a hump above it. Drawn
naively from the plan, it contradicted the overview: the plan puts many past
weeks and the status week above capacity, work that is done, while RePlan's
workload finds a single overloaded resource-week ahead. A hump is now red only
where the workload agrees, the maintainer's 2026-W46, and grey where the plan
went over but no overload remains. The first rendered run also failed
falsely: its "profile hidden" check found the profile's marker in the page's
own script. 3 agent-hours against 5 planned; deployed the same evening, and
the load profile finishes the approved Gantt page.

## Step 86. The Last Old Page Goes

The operator accepted the load profile, and
`task.self.show-evidence-and-actuals-in-plugin-panel` replaced the last page
left from before the redesign. The task page now reads the overlay row the
way the prototype did: what was done, planned, booked and remaining amounts,
every booking, and dependencies both ways as links, with the task and its
lanes drawn by the Gantt page's own presenter and markup rather than a third
drawing of the same thing. The throwaway overlay's open task had no evidence
or bookings, so the rendered checks gained a second task that has all of
them, and the hostile markup now also sits in an evidence summary and a
booked resource name. 3.5 agent-hours against 4 planned; deployed the same
night. With it every part of the approved design is live; what remains of
milestone 2 is the operator's verdict on real work.

## Step 87. The Overview The Design Asked For

The operator accepted the task page and agreed to remove what milestone 1 had
kept on the overview until the Gantt page existed. The removal became its own
task, `task.self.remove-old-task-list-from-plugin-overview`, in OpenProject and
RePlan before any code changed. The task list with resource strips and the
resource cards went, with the presenter code, partial and styles that only
they used; the overview is the design's header, counters and attention table
with a pointer to the Gantt. Two checks were sharpened on the way: the first
"old list is gone" check matched the attention table's own Resources column,
and the scaffold check, which listed views by hand, had never syntax-checked
the Gantt's new shared partials; it now reads the whole directory. 1.5
agent-hours as planned; deployed the same morning.

## Step 88. A Link Instead Of A Tab

`task.self.link-openproject-work-package-to-replan-panel` asked for the
least coupled way to reach the RePlan task page from the work package itself.
Reading OpenProject 17.6 showed that a work-package tab is Angular code inside
OpenProject's frontend, which a plugin can only add by rebuilding that
frontend. The operator asked, in plain words, how invasive each option was
for upgrades, and chose a "RePlan" link custom field: no OpenProject code is
touched. The throwaway instance showed what reading alone would have missed:
the model accepts a relative link, API v3 does not. The helper builds the
full link from the ignored public address, writes only what differs, and is
its own drift check; eight checks against a real OpenProject guard it. Live,
the field was created for the RePlan project only and all 99 work packages
link to their task pages. The link went ahead of the milestone 2 verdict with
the operator's agreement, and RePlan had no audited way to cancel that
dependency, now GAP-032. 4 agent-hours against 5 planned.

## Step 89. A Release For Someone Else's Agent

A second project appeared in the same OpenProject, with an outside person as
its administrator, and the operator asked how such people and their agents
could use RePlan and OpenProject. The first answer pointed them at
OpenProject's raw API; the operator objected that months had gone into
tooling meant to replace exactly that. Inspecting it showed the objection was
right: every helper was already configured by settings, and the second
project was already being run with them. What had never been tried was an
ordinary member's token. On a throwaway OpenProject, with a Member and a
Project admin who are not administrators, everything worked except one
command that listed the instance's users. The rest of the work was packaging:
one archive with a static binary, the helpers, a sample plan, a settings
template and a guide, checked by unpacking it outside the repository where
`go` fails, and by 19 checks against a real OpenProject with a Member's
token. The offline check found the repository owner's name in a sample plan
committed in July. 4.5 agent-hours against 8 planned, in one day of a
three-day window. How the archive is handed over, and under what licence,
is the operator's decision.

## Step 90. A Second Project Must Not See The First

The per-project overlay had waited since the plugin review of 2026-09-25 as a
November task. A second project in the same OpenProject, run by another
session for an outside administrator, made it the next thing to do: enabling
the RePlan module there would have shown this project's plan to that
project's members. The loader now takes the page's project and shows an
overlay only to the project it names, from a per-project directory or the
single file as before, and tells any other project that it has no overlay
yet without naming the other project or a path. The live installation needed
no configuration change. Deploying after a two-week pause showed what the
session's scratch directory had been holding: the switch script, restored
from the log, was an early draft that failed before changing anything.
2.5 agent-hours as planned, a month ahead of its window.

## Step 91. A Directory Of Plans On The Live Instance

The plugin could read a plan per project, and the live instance still gave
it one file. With the operator's go the compose override gained the overlay
directory, edited by a script that was first tried on a copy of the file and
that recreates the services with a rollback. Publishing became a helper of
its own: a built overlay goes into its project's directory only, after a dry
run, and an overlay that names another project is refused before anything is
sent. This project's plan now comes from its own directory, and the session
that runs the second project has the three commands it needs. Two things
failed on the way without harm: the protected connection was temporarily
unavailable, so the first requests timed out, and the session's helper scripts
had to be rebuilt after the pause. 2 agent-hours
as planned.

## Step 92. Five Acceptances And A Verdict

On 2026-10-12 the operator accepted five tasks at once: the RePlan link in
every work package, the agent kit, the per-project overlay, the overlay
directory on the live instance, and milestone 2. Milestone 2 had been planned
as one deployment on 2026-11-03 followed by a verdict; it happened instead as
four deployments accepted one by one in September, so its own task carried no
work, only the verdict. Its demands are closed as cancelled, with the work
booked on the parts. Nothing waits on a person and no task is active. What
remains of the plugin track is milestone 3: the visual audit of 2026-09-25
repeated against the finished plugin.

## Step 93. The First Outside Run Found What The Checks Could Not

The agent kit had passed every check, and the session that runs the second
project was the first to use it as an outside user. It did not reach
OpenProject at all: the reverse proxy in front of it answered 401 on its own.
The agent had tested "the API answers from outside" by the status code alone
and reported it as working, to the operator and to that session, although the
same layer was written down in this repository on 2026-09-25. The proxy
already had a way in for API clients, a perimeter key in a request header;
the helpers and the binary could not send it. They can now, a stand-in for
such a proxy is part of the kit's check, the operator had two keys issued,
and the kit was run from outside over HTTPS against the live instance, with
the key, without it and with a wrong one. The operator also settled the
terms of use, which travel in the archive. 2.5 agent-hours as planned. The
lesson for the checks: a status code says who refused, only together with
who answered.

## Step 94. The Kit Learns The Wiki, And Writes Its Own Manual Page

The operator named what the second project's administrator actually does: he
works in the wiki. The kit had fifteen helpers and none for it. A sixteenth
now reads and writes pages and attachments with the kit's rule, a dry run
first, and one more: a page changed since it was read is refused, never
merged or overwritten. The checks taught three things. The seeded Member role
may delete wiki pages, so the refusal had to be shown with a Reader. A link
to the live server that kept dropping turned a download into a stack trace,
which is now an error the agent can read. And the load profile's checks had
depended on the plan having an overloaded week, which stopped being true
that day. The helper's first real use was the second project's own tools
page: rewritten with the operator's decisions, a wiki section and a ready
instruction for the administrator's agent, with the new archive attached and
read back by checksum. 3.5 agent-hours as planned.

## Step 95. An Agent Gets A Name Of Its Own

The third project's agent session asked for an identity before touching its
board. It could see the project only through the dispatcher's token, and it
refused to sign its own changes with a person's name. The operator approved
an API-only user and a role listed permission by permission rather than the
seeded Member role, which can delete wiki pages. The role can create, edit,
move and comment on work packages and read the wiki, nothing else, and it
holds in that one project. The token went from the instance to a root-owned
file without passing through any output. The check was made the way the
asking session proposed, by refusal: the forms for a new project or member
answer 403, another project answers 404, wiki writes answer 403. The asking
session's own check found the one thing a role built from scratch lacks: no
status transitions, so closing a task answered 422 until the role got them.
1.25
agent-hours against 1.5 planned.

## Step 96. The Audit, Three Weeks Later

The visual audit of 2026-09-25 was rerun against the finished plugin, the
same way: the live instance, the read-only review user, 1600 and 900 pixels.
Every finding of that day is closed, and every wish the operator recorded
from July to September has a place on the pages. The plugin itself had two
small defects: a tab for a work package that does not exist, and a column
header too wide for its column. The bigger findings were not in the plugin.
The live overlay was three days old, so a task accepted two days earlier
was "not found" behind its own link. The status date still said Monday and
raised a warning about a task that was fine. Both were steps nobody owned;
they are now loop steps. The second deploy sent only the two layers that
changed, 47 seconds instead of an hour. 4 agent-hours against 1.5 planned:
the estimate counted a deploy and an audit, not two deploys and the loop
repairs.

## Step 97. Writes That Touch Only What Changed

GAP-022 had been described as noise in the canonical files. Measured, those
files were already in the format the store renders, after weeks of command
writes; the noise lived wherever a person or an agent appended by hand, and
the week's evidence file would have lost its look in about 700 lines on the
first command write. Every write now splices the changed records and fields
into the file's own bytes and checks the result against a whole-file render,
which stays the fallback. Keeping bytes also kept data: evidence `url`, in
the spec and in a hundred dogfood records, was not modelled and was dropped
by every write to an evidence file. 3 agent-hours as planned, a month early,
second in the order the operator agreed. This step's own records are the
first written by the new store: one line or one appended record per change.

## Step 98. A Validator Per Record Kind

`validateWorkspace` was the most complex function in the project. It is now
one validator per record kind, called in the old order, because the order
decides what a reference resolves to: a demand can only point at a task
registered before it. The proof came first: golden files taken from the old
code, one from a workspace built to fire every one of the 60 diagnostic
codes, with a test that reads the codes from the source so a new one cannot
go unexercised. After the split they are byte-identical. 1.5 agent-hours
against 3 planned, a month early.

## Step 99. One Way To Apply, Six Ways To Choose

`ImportAcceptService.Accept` had six copies of the same apply branch, one
per accept mode, which differed in a builder, a writer, three messages and
an action word. Each mode is now a description with those differences in
plain sight, and one flow applies any of them. As with the validator, the
proof was taken from the old code first: every mode as a dry run, an apply
and an apply with nothing safe to write, with the command result and every
written byte. All eighteen are identical after the change. 1.25
agent-hours against 2 planned. The three cleanups the operator queued for
November are done on 15 October; the lint baseline turned out to have
drifted, which is now written down where it is kept.

## Step 100. A Decision About Order Gets An Audit Trail

On 2026-09-27 the operator let a task go ahead of the one it depended on,
and RePlan had no way to say so except an edit of the dependencies file by
hand. A scenario can now cancel a dependency, and only with the evidence of
the decision named: without it the apply is refused, and with it the audit
record carries it. Resolving a dependency stays with proposals, where the
predecessor's state is checked. 1 agent-hour against 2 planned. The five
tasks the operator queued after the plugin track are done in one day; the
release story is the one left.

## Step 101. A License, And A Look At Everything

The operator keeps the repository private for now and licensed it under
Apache-2.0. The publication audit read every tracked file and every commit.
It found no live secret, but it found something the August checkpoint had
missed: the leak checks had the private names spelled out, so the guard
listed what it guarded. Those names now live in an ignored file. The history
still carries the server's name and address in two commits of 18 July; that,
the pilot journals and the old pilot sidecars are the operator's to decide,
each with a recommendation written down. 2 agent-hours against 3 planned.

## Step 102. Looking At The Tools From A Newcomer's Seat

The operator asked how coherent the tooling is for an agent that arrives
with only the repository or only the kit. A read-only reviewer mapped every
command to a loop step and wrote fourteen findings. The executor rechecked
the five serious ones in the code. Each tool was careful on its own; together
they were not. Two helpers reported success when they had pushed nothing.
Some loop steps existed only in the operator's private scripts. The same
mapping was written in several places that disagreed. 1.5 agent-hours
against 3 planned.

The operator accepted the review and asked for every finding and every open
gap to become a task, done in full. Planning those tasks turned up a
fifteenth finding: the schedule drift check called the single development
slot overloaded in every week, because it summed slots that the Go workload,
since GAP-031, counts at their daily peak. That task goes first.

Entering the plan into OpenProject turned up two more. The relations helper
read only the first page of the instance's relations. It would have created
113 duplicate relations instead of 17; that is fixed now, with a test. Then,
once the relations existed, OpenProject quietly moved nine dates, and the
helper said nothing about it; that became a task. A third: the toolkit has
no way to correct a work package's description, so progress on a task could
only be written as a comment.

Thirty-eight tasks fill one slot a day until 14 December, and the release
story moves behind them.
