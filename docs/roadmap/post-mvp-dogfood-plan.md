# RePlan post-MVP dogfood plan

Date: 2026-07-24

Status: approved next operating plan after MVP readiness stabilization.

## Purpose

This document records the first plan after RePlan became usable as an internal
CLI-first MVP.

It answers:

- what major directions are available now;
- what each direction gives us;
- what each direction risks;
- which direction is chosen first;
- how the first short dogfood cycle should run;
- what evidence should decide the next implementation slice.

The goal is not to expand RePlan blindly. The goal is to use RePlan in anger on
real RePlan work and let the next product decision come from observed friction.

## Current State

The project is past the first internal readiness gate.

Known facts:

- the MVP readiness audits found zero blockers;
- the CLI-first loop is usable for internal dogfood;
- validation, workload, weekly reports, project-state reports, dashboard,
  Gantt, tracking suggestions, tracking proposal loop, proposal acceptance,
  actuals and audit records work;
- OpenProject file-fixture import works without private infrastructure;
- TaskJuggler remains optional and is covered through CI;
- the code and documentation stabilization branch was merged to `main`;
- the project is still not a polished public OSS release.

This means the next step should be operational learning, not broad feature
expansion.

## Main Forks

### Fork 1. Dogfood RePlan on RePlan

Description:

Use RePlan itself as the next public dogfood project. Plan RePlan work in
`examples/replan-self`, execute a few real tasks, record actuals and evidence,
inspect reports, and then decide what hurts most.

What this gives:

- keeps the project focused on RePlan instead of drifting into an unrelated
  side project;
- exercises the exact local-first, AI-CLI-first workflow RePlan is meant to
  support;
- produces a public-friendly build story: "RePlan is planned with RePlan";
- tests whether the dashboard, Gantt, tracking loop and reports are actually
  useful to an operator;
- reveals missing ergonomics before the project grows more features.

Risks:

- the dogfood project is somewhat self-referential;
- RePlan work is mostly software/documentation work, so it may not stress every
  offline or non-human resource class immediately;
- without discipline, the project can still become a documentation exercise
  rather than a real operating loop.

When this fork is best:

- immediately after MVP readiness;
- before choosing a new large feature;
- when the main question is "can we use this ourselves tomorrow?"

### Fork 2. Deepen the OpenProject Integration

Description:

Keep OpenProject as the human project-management substrate and use RePlan as a
resource-planning, reporting, tracking and proposal layer above OpenProject
tasks.

What this gives:

- stays close to the original intended deployment model;
- validates whether OpenProject is a good first PM substrate;
- clarifies which data belongs in OpenProject and which data belongs in RePlan
  sidecars;
- exercises source references, imports, relations, time entries and resource
  links;
- prepares the path toward a future OpenProject overlay or plugin if the static
  dashboard becomes insufficient.

Risks:

- it can pull the project back into private infrastructure and private pilot
  details;
- it can delay learning whether RePlan's own core loop is good enough;
- it may overfit RePlan to OpenProject before the source-agnostic model is
  proven through use;
- it can create operational complexity before product usefulness is clear.

When this fork is best:

- after one short RePlan-on-RePlan dogfood cycle;
- when the next pain is source synchronization rather than RePlan usability;
- when there is a concrete OpenProject project that needs resource planning
  and can be tested without polluting the public repository.

### Fork 3. Close Known Gaps

Description:

Work through the gap register before adding any new functionality.

Current gap areas:

- dashboard and Gantt HTML need stronger golden-file style tests when the
  contract stabilizes;
- private OpenProject pilot narrative still needs full relocation or
  sanitization before a serious public OSS release;
- some workload filters are planned in the CLI specification but not yet
  implemented.

What this gives:

- strengthens the foundation;
- reduces documentation drift;
- makes the project easier for other agents and contributors to understand;
- reduces regressions in generated human-observability artifacts.

Risks:

- can become hygiene work that does not teach whether RePlan is useful;
- may over-optimize internals before the operating workflow is proven;
- may delay the next concrete dogfood signal.

When this fork is best:

- when a known gap blocks a concrete dogfood action;
- before a public OSS release;
- when CI, docs or generated artifacts become too fragile for daily use.

### Fork 4. Build the Next Functional Layer

Description:

Add the next small product capability instead of doing a pure audit or docs
slice.

Candidate slices:

- implement more workload filters:
  - `--resource-class`;
  - `--unit`;
  - `--task`;
  - later `--from`, `--to`, `--status-date`;
- improve the operator "what now?" surface;
- make proposal review more ergonomic;
- improve dashboard/Gantt readability;
- add richer actual/evidence/tracking automation;
- add stronger static HTML tests after the dashboard contract settles.

What this gives:

- makes RePlan more useful immediately;
- can remove real friction found during dogfood;
- starts turning the MVP into a usable daily tool.

Risks:

- if chosen before dogfood, it may solve imagined problems;
- can expand scope too quickly;
- can hide the fact that the current operator loop is not yet being used
  regularly.

When this fork is best:

- after the first short dogfood cycle produces a clear, repeated pain;
- when the next slice can be completed and verified by command, report or
  generated artifact;
- when the change strengthens the existing CLI-first loop rather than creating
  a new surface.

## Decision

Chosen first direction:

```text
Run a short RePlan-on-RePlan dogfood cycle before choosing the next feature.
```

Reason:

- the internal MVP is ready enough to use;
- adding more features before using it would be guesswork;
- OpenProject integration matters, but making it the next center of gravity
  risks returning to private infrastructure too early;
- known gaps are real, but only one or two should be closed immediately if
  dogfood proves they block operation;
- the strongest next evidence will come from trying to plan, execute and track
  RePlan's own next tasks through RePlan.

## First Dogfood Cycle

Cycle length:

```text
3 to 5 concrete RePlan tasks, not an open-ended phase.
```

Primary project:

```text
examples/replan-self
```

Minimum loop:

1. Add or confirm the next 3 to 5 RePlan tasks in the self-dogfood fixture.
2. Validate the project.
3. Generate workload and project-state reports.
4. Generate the static dashboard and Gantt.
5. Pick the next task from the RePlan report/dashboard, not from memory.
6. Execute the task.
7. Record actuals and evidence.
8. Run tracking suggestions and inspect the tracking proposal loop.
9. Accept or reject any relevant proposal through the proposal loop.
10. Record the friction observed during the cycle.

Required commands:

```bash
go run ./cmd/replan validate examples/replan-self --format json
go run ./cmd/replan workload --project examples/replan-self --week 2026-W31 --format json
go run ./cmd/replan report state --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan report dashboard --project examples/replan-self --week 2026-W31 --output reports/latest --format json
go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --format markdown
```

Mutations must use the normal RePlan safety rules:

- use `--dry-run` first when available;
- require an actor for mutation;
- append audit for accepted changes;
- validate after mutation;
- keep source-of-truth changes visible through Git.

## What Not To Do In This Cycle

Do not start with:

- a new RePlan web application;
- a daemon;
- a hosted service;
- custom authentication;
- full role-based access control;
- OpenProject write-back;
- an OpenProject plugin;
- automatic resource leveling;
- a full scheduling solver;
- large public OSS release cleanup;
- a new unrelated side project as the primary dogfood case.

These may become valid later. They are not the next step.

## Decision Criteria After The Cycle

After the first dogfood cycle, choose the next slice by observed pain:

| Observed pain | Next likely slice |
| --- | --- |
| Operator cannot quickly tell what to do next | Improve project-state report, next-action ranking or dashboard summary |
| Resource pressure is hard to inspect | Improve workload filters or Gantt/resource load views |
| CLI agents pass too much data around manually | Add a more focused command or JSON field for the repeated operation |
| Proposal review is too noisy | Improve proposal list/show filters and tracking proposal loop grouping |
| Dashboard regressions become risky | Add golden-file tests for generated HTML |
| OpenProject/RePlan comparison is the bottleneck | Return to OpenProject fixture/live-source integration |
| Public release confidence is the bottleneck | Work through OSS separation and release-story gaps |

## Expected Output Of The Cycle

The dogfood cycle is successful when it produces:

- updated self-dogfood task state;
- actual consumption records;
- evidence records;
- at least one generated report/dashboard snapshot used for a real decision;
- a short list of observed friction;
- one selected next implementation slice.

The cycle is not successful if it only creates more documentation without using
RePlan reports or tracking outputs to make a work decision.

## Current Recommendation

Start with this concrete next action:

```text
Use RePlan to select and execute the next RePlan task.
```

The first candidate tasks should come from the existing self-dogfood backlog
and known gaps, not from a new external project.
