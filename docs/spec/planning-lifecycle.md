# Planning lifecycle

Date: 2026-07-04

Status: approved functional design.

## Purpose

This document defines how planning objects move through RePlan.

It answers:

- how a task enters RePlan;
- when a task becomes part of a working plan;
- when a task becomes part of an approved baseline;
- when snapshots are created;
- what changes baseline;
- what changes current plan;
- what belongs to actual execution;
- how scenario planning differs from re-planning.

This document is about functional behavior, not implementation details.

## Design principle

RePlan is not intended to become a full task tracker.

RePlan is a resource planning and execution tracking layer above one or more
project-management systems.

The external project-management system remains the owner of ordinary task
management data.

Examples of such systems:

- OpenProject;
- Plane;
- GitHub Issues;
- GitLab Issues;
- YouTrack;
- Jira;
- a local file source for fixtures, tests and offline operation.

RePlan owns the planning, resource, capacity, scenario, evidence and actuals
layer.

This separation is mandatory because RePlan must remain source-agnostic. The
first likely source is OpenProject, but the RePlan domain model must not become
an OpenProject-specific model.

## Core lifecycle summary

The core lifecycle is:

```text
external source task
  -> RePlan task projection
  -> draft/current plan
  -> baseline snapshot
  -> execution tracking and actuals
  -> scenarios and re-planning proposals
```

The main planning states are:

```text
baseline = approved snapshot of commitments
current  = mutable working plan
actual   = observed execution facts
scenario = temporary alternative version of the current plan assumptions
```

The most important rule is:

```text
baseline, current plan, actuals and scenarios are separate things.
```

They can be compared, but they must not be silently merged.

## External source task

An external source task is a task-like object owned by a source system.

Examples:

- OpenProject work package;
- GitHub issue;
- GitLab issue;
- Plane issue;
- YouTrack issue;
- Jira issue;
- local YAML or JSON task used for tests and offline operation.

The source system normally owns:

- title;
- long description;
- source status;
- source assignee;
- source priority;
- source comments;
- source attachments;
- source labels or categories;
- source project structure;
- source permissions;
- source audit history.

RePlan may read this data and may store a small normalized projection, but
RePlan should not treat itself as the primary owner of ordinary task-tracker
content.

## RePlan task projection

A RePlan task projection is the RePlan-side planning representation of an
external source task.

It must include enough information for resource planning and execution
tracking.

It should include:

- RePlan internal task identifier;
- source system identifier;
- source object type;
- source object identifier;
- source URL when available;
- normalized display title;
- normalized source status;
- planning status inside RePlan;
- planned effort;
- remaining effort;
- actual effort;
- resource demands;
- capacity constraints;
- scheduling constraints;
- dependency references;
- deadline references;
- baseline membership;
- current plan membership;
- scenario membership;
- evidence links;
- proposal links;
- last source synchronization time.

The projection may contain cached source fields for reporting convenience, but
cached source fields are not the source of truth for task management.

## Task entry into RePlan

A task enters RePlan through one of these mechanisms:

- import from an adapter;
- explicit link from an adapter;
- local file fixture;
- manual creation in a local planning file for offline operation;
- later, possible write-back or creation through a supported source adapter.

For MVP, the preferred path is:

```text
read-only source import -> RePlan projection -> local planning data
```

The initial implementation should not require write access to OpenProject or
another project-management system.

This keeps MVP useful, easier to test and safer for AI-CLI-first operation.

## Draft planning state

When a task first appears in RePlan, it may be incomplete from a resource
planning point of view.

Examples:

- there is no estimate;
- there is an estimate but no resource demand model;
- there is an assignee but no explicit capacity reservation;
- there is a deadline but no feasible plan;
- there is a source status but no RePlan tracking state;
- there are dependencies in the source system but no normalized dependency
  model yet.

Such a task can be treated as draft planning input.

Draft means:

```text
RePlan knows about the task, but the task is not yet an approved planning
commitment.
```

In draft state, an operator or AI agent may:

- add planning assumptions;
- propose resource demands;
- propose estimates;
- propose remaining work;
- propose scheduling constraints;
- detect missing information;
- generate warnings;
- compare rough workload;
- create scenarios.

Draft planning data can feed the current plan, but draft data does not become a
baseline without explicit approval.

## Current plan

The current plan is the mutable working plan.

It answers:

```text
Given what we know now, how do we currently intend to execute the work?
```

The current plan may include:

- imported tasks;
- locally added planning assumptions;
- accepted estimates;
- accepted remaining work values;
- accepted resource demands;
- accepted capacity constraints;
- accepted scheduling constraints;
- accepted dependencies;
- accepted actuals;
- accepted proposals;
- applied scenarios;
- operator decisions.

The current plan may change during normal work.

Examples of events that may change the current plan:

- a new task appears in OpenProject;
- a task is removed from current scope;
- an estimate changes;
- remaining work changes;
- a human resource becomes unavailable;
- an AI agent becomes unavailable;
- a service quota is exhausted;
- a subscription limit changes;
- an environment becomes reserved or unavailable;
- a task is blocked;
- a pull request is opened;
- a pull request is accepted;
- an offline milestone is reached;
- a proposal is accepted;
- a scenario is applied;
- an operator updates a planning assumption.

Changing the current plan does not change baseline automatically.

## Baseline

A baseline is an approved snapshot of planning commitments at a specific time.

It answers:

```text
What plan did we approve as the reference point?
```

A baseline should capture:

- baseline identifier;
- baseline creation time;
- baseline approval time;
- approving actor;
- included tasks;
- excluded tasks if relevant to the decision;
- planned dates;
- planned effort;
- planned remaining work;
- planned resource demands;
- planned capacity assumptions;
- planned dependencies;
- planned deadlines;
- planned reservations;
- source data version or synchronization reference;
- RePlan model version;
- notes explaining the approval context.

The baseline is used for comparison.

It enables questions such as:

- what did we plan;
- what changed;
- what was actually consumed;
- which resource became overloaded;
- which deadline became risky;
- which assumption was wrong;
- whether re-planning is needed.

## Baseline immutability

A baseline must not silently change.

The following events must not automatically change baseline:

- fresh import from OpenProject or another source;
- source status change;
- source assignee change;
- source description change;
- pull request creation;
- pull request merge;
- continuous integration result;
- human progress report;
- AI-agent progress report;
- service quota usage event;
- actual time entry;
- automatic workload recalculation;
- newly detected overload;
- newly detected dependency risk;
- new estimate proposal;
- new scenario;
- applied current-plan update.

Any such event may create:

- evidence;
- an actual record;
- a warning;
- a proposal;
- a current-plan change;
- a scenario.

But a baseline changes only through an explicit baseline operation.

## Baseline update operation

Changing baseline means creating a new baseline version.

RePlan should not mutate an old baseline in place.

The operation should be explicit and auditable.

Conceptually:

```text
create-baseline --from current --name "MVP baseline 2026-07-04"
```

or:

```text
approve-baseline --scenario scenario-2026-07-04-a
```

The operation should record:

- who approved it;
- when it was approved;
- what input was used;
- which previous baseline it supersedes, if any;
- why the new baseline exists;
- whether this is an initial baseline or a re-baseline.

For MVP, this can be local and file-based. It does not require enterprise
workflow machinery.

## Actuals

Actuals are execution facts.

They answer:

```text
What really happened?
```

Actuals may include:

- actual human effort;
- actual AI-agent effort;
- actual service quota consumption;
- actual token consumption;
- actual API request consumption;
- actual environment usage;
- actual budget consumption;
- actual start time;
- actual completion time;
- actual review time;
- actual blocked time;
- actual status date;
- evidence links.

Actuals must remain separate from baseline and current plan.

Accepted actuals may affect forecasts and remaining work in the current plan,
but they do not rewrite the baseline.

## Evidence

Evidence is source material that supports an actual or a proposal.

Examples:

- OpenProject status change;
- GitHub pull request;
- GitLab merge request;
- commit reference;
- continuous integration run;
- deployment event;
- human operator note;
- agent report;
- calendar entry;
- time entry;
- quota usage record;
- API billing record;
- environment reservation event.

Evidence by itself is not always a fact that RePlan should apply directly.

Some evidence can be used automatically. Other evidence should create a
proposal that needs human confirmation.

The exact rules belong to the tracking lifecycle and event model, but the
planning lifecycle requires this separation from the beginning.

## Proposals

A proposal is a suggested change.

Proposals may be created by:

- a human;
- an AI coding agent;
- an adapter;
- an import process;
- a workload calculation;
- a tracking event processor.

Examples:

- propose estimate update;
- propose remaining work update;
- propose task completion;
- propose actual effort record;
- propose resource reassignment;
- propose capacity exception;
- propose scenario;
- propose new baseline.

For AI-CLI-first use, proposals are important because agents should be able to
help RePlan without silently mutating important planning data.

The default rule should be:

```text
agents can read, analyze, dry-run and propose; mutation requires explicit
permission or confirmation.
```

## Scenario

A scenario is an alternative planning branch.

It answers:

```text
What would happen if these assumptions changed?
```

A scenario may change:

- resource assignments;
- resource capacities;
- estimates;
- remaining work assumptions;
- dates;
- deadlines;
- dependencies;
- task inclusion or exclusion;
- service quota assumptions;
- subscription limit assumptions;
- environment availability assumptions;
- AI-agent availability assumptions.

A scenario must not change:

- accepted actuals;
- historical evidence;
- old baselines;
- external source history.

A scenario may be:

- created;
- calculated;
- compared with current plan;
- exported;
- rejected;
- accepted as a current-plan change;
- used to create a new baseline.

Scenario planning is not the same as re-planning.

Scenario planning explores alternatives.

Re-planning applies a decision to the current plan.

Re-baselining approves a new baseline snapshot.

## Re-planning

Re-planning means changing the current plan after new information appears.

Examples:

- a task took longer than expected;
- a person is unavailable;
- an AI-agent quota is exhausted;
- a dependency is blocked;
- a deadline changed;
- a customer added scope;
- a pull request revealed hidden work;
- a resource becomes available earlier than expected.

Re-planning should usually follow this path:

```text
new evidence or actual
  -> warning or proposal
  -> scenario or direct current-plan update
  -> operator approval when needed
  -> updated current plan
```

Re-planning changes the current plan.

Re-planning does not change baseline unless a separate baseline operation is
performed.

## Snapshot

A snapshot is a saved view of planning state at a point in time.

There are several useful snapshot types:

- baseline snapshot;
- current-plan snapshot;
- scenario snapshot;
- report snapshot;
- import snapshot.

For MVP, baseline snapshots are required.

Current-plan and report snapshots are strongly useful because they make CLI
reports reproducible.

Import snapshots are useful because they explain which source data was used for
planning calculations.

Scenario snapshots are useful once scenarios become first-class objects.

## Source synchronization

Source synchronization imports or refreshes source data.

Source synchronization may:

- create new RePlan task projections;
- update cached source fields;
- update source status mapping;
- detect source-side changes;
- create warnings;
- create proposals;
- update current plan when policy allows it.

Source synchronization must not:

- mutate an approved baseline;
- erase accepted actuals;
- silently accept agent-created planning changes;
- silently overwrite local planning assumptions without policy.

For MVP, the safest default is read-only import plus explicit local planning
updates.

## Ownership rules

External source system owns:

- ordinary task content;
- source task identity;
- source task status;
- source comments;
- source attachments;
- source project structure;
- source permissions.

RePlan owns:

- planning assumptions;
- resource demands;
- capacity models;
- baseline snapshots;
- current planning state;
- scenarios;
- workload calculations;
- warnings;
- actual resource consumption records;
- evidence links;
- proposals;
- audit records for RePlan decisions.

Shared or mapped fields require adapter-specific policy.

Examples:

- source assignee may become a default RePlan human resource demand;
- source due date may become a RePlan deadline constraint;
- source estimate may become an initial planned effort assumption;
- source status may become evidence for tracking state.

The mapping must be visible and reversible enough that an operator can
understand why RePlan made a recommendation.

## AI-CLI-first implications

The planning lifecycle must support AI coding agents and command-line agents
from the beginning.

This means:

- commands must have machine-readable input and output;
- read operations must be safe and explicit;
- dry-run operations must be available for calculations;
- proposal creation must be possible without granting full mutation rights;
- important mutations must be auditable;
- baseline creation must be explicit;
- current-plan changes must be inspectable;
- scenario comparison must be machine-readable;
- reports must be usable by humans and agents.

The planning lifecycle should avoid hidden application state that only exists
inside a graphical interface.

## MVP behavior

For MVP, implement the lifecycle in a conservative form:

- support source import or local fixture input;
- create RePlan task projections;
- allow local planning assumptions;
- maintain current plan data;
- create explicit baseline snapshots;
- store accepted actuals separately;
- create warnings and proposals rather than silent risky updates;
- support at least one scenario representation;
- generate human-readable and machine-readable reports;
- keep source write-back out of the first critical path.

The MVP does not need:

- full enterprise approval workflow;
- complex graphical baseline management;
- write-back to every source system;
- automatic scheduling equal to Microsoft Project;
- full TaskJuggler parity in the first implementation;
- hidden automatic re-baselining.

## Required future documents

This planning lifecycle depends on several more detailed specs.

Required next documents:

- `docs/spec/canonical-model.md`
- `docs/spec/scenario-model.md`
- `docs/spec/workload-calculation.md`
- `docs/spec/openproject-adapter.md`

Related existing documents:

- [`functional-design-checklist.md`](functional-design-checklist.md)
- [`mvp-boundary.md`](mvp-boundary.md)
- [`tracking-events.md`](tracking-events.md)

## Approved decisions

- RePlan is a planning layer, not the primary task tracker.
- External project-management systems own ordinary task data.
- RePlan owns resource planning, capacity, scenarios, evidence, proposals,
  actuals and baseline/current comparison.
- A task enters RePlan through source import, explicit source link, local
  fixture or local offline planning data.
- Imported tasks become RePlan task projections.
- Incomplete planning data is allowed and treated as draft planning input.
- The current plan is mutable.
- A baseline is an explicit approved snapshot.
- Baseline changes only by creating a new baseline version.
- Source synchronization must not silently change baseline.
- Actuals are separate from baseline and current plan.
- Accepted actuals may affect forecasts and current-plan remaining work, but do
  not rewrite baseline.
- Scenarios are alternative planning branches.
- Scenarios do not mutate accepted actuals, historical evidence or old
  baselines.
- Scenario planning explores alternatives.
- Re-planning changes current plan.
- Re-baselining creates a new approved baseline snapshot.
- AI agents should be able to read, dry-run, analyze and propose without
  receiving broad mutation rights by default.
