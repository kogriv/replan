# RePlan initial roadmap

Date: 2026-07-02

Status: historical. The original phased plan; the current public summary is
[`ROADMAP.md`](../../ROADMAP.md).

## Roadmap principle

Do not build a full product first.

Build the smallest loop that proves whether RePlan is useful in real planning.

The first loop:

```text
import tasks
define resources
calculate workload
show overloads
create scenario
compare scenario
export report
optionally export to TaskJuggler
```

## OrCode dependency

RePlan is an enabling project for OrCode.

OrCode should remain paused until RePlan or an equivalent task/resource
infrastructure proves that a small human + AI development team can be planned
operationally.

This means RePlan's MVP is not only a product experiment. It is also an
exit-condition experiment for OrCode's research pause.

Minimum condition for OrCode to resume:

- real tasks can be imported or modeled;
- human developers can be represented as resources;
- AI agents can be represented as resources and actors;
- weekly capacity can be calculated;
- overloads can be detected;
- scenarios can be compared;
- approved planning changes can be written back or at least represented as
  auditable dry-run diffs;
- execution evidence can be linked to tasks.
- basic principal/actor/resource boundaries exist;
- agent actions can be constrained by policy;
- mutating operations generate audit records.

If these conditions are not met, OrCode should not assume its implementation
phases are operationally manageable.

## Phase 0: repository and documentation

Status: current.

Goals:

- create the `replan` repository;
- initialize git repository;
- copy historical OrCode research;
- document RePlan as task/resource infrastructure prerequisite for OrCode;
- document product direction;
- document FOSS model;
- document source-agnostic architecture;
- document OpenProject-first but not OpenProject-only rule;
- document optional TaskJuggler integration;
- document minimal access/actor/security model without enterprise overhead;
- document generic resource abstraction instead of hardcoded resource-class
  enumeration;
- document early roadmap.

Exit criteria:

- documentation is committed;
- next implementation decision is clear.

## Phase 1: fixtures and canonical model

Duration target: 1-2 days.

Goal:

Create a minimal canonical model with YAML fixtures.

Actions:

- define resource schema;
- define principal/actor/resource separation;
- define resource unit/capacity/period skeleton;
- define reservation vs consumption skeleton;
- define project/source scope field;
- define availability exception schema;
- define task schema;
- define scenario schema;
- create sample data for:
  - operator;
  - two human developers;
  - one AI coding agent;
  - ten to twenty tasks;
  - estimates;
  - dependencies;
  - deadlines;
  - one unavailable period;
- write validation command.
- define local policy YAML skeleton.

Example command:

```bash
replan validate examples/small-team
```

Exit criteria:

- sample dataset validates;
- missing required fields produce readable errors;
- actor/resource/scope fields are represented even if enforcement is minimal;
- documentation includes examples.

## Phase 2: internal workload report

Duration target: 2-4 days.

Goal:

Generate a useful weekly workload report without OpenProject and without
TaskJuggler.

Actions:

- load YAML fixtures;
- calculate available hours by resource/week;
- spread remaining task effort across task date ranges;
- sum planned effort by resource/week;
- detect overload;
- detect missing estimates;
- detect missing dates;
- detect blocked tasks starting before dependencies finish;
- output markdown report;
- output CSV report;
- output JSON snapshot.

Example commands:

```bash
replan workload --from examples/small-team --week 2026-W28
replan report weekly --from examples/small-team --format markdown
```

Exit criteria:

- report answers "who is overloaded next week?";
- report is readable in Obsidian;
- report contains enough detail to drive a real planning decision.

## Phase 3: scenario engine

Duration target: 3-5 days.

Goal:

Support "what if" planning without modifying source tasks.

Actions:

- create baseline snapshot;
- create scenario from baseline;
- support resource availability changes;
- support estimate changes;
- support task date changes;
- support assignment changes;
- compare baseline and scenario;
- report workload difference.
- create audit events for scenario creation and changes.

Example commands:

```bash
replan snapshot create --name baseline
replan scenario create --name ivan-unavailable --from baseline
replan scenario availability ivan-unavailable --resource dev_ivan --date 2026-07-10 --hours 0
replan scenario compare baseline ivan-unavailable
```

Exit criteria:

- operator can answer "what happens if Ivan is unavailable?";
- no external task system is modified;
- scenario diff is understandable.

## Phase 4: OpenProject adapter

Duration target: 3-7 days.

Goal:

Import real OpenProject work packages into the canonical model.

Actions:

- configure OpenProject URL and token;
- import projects;
- import users;
- import work packages;
- map assignees to resources;
- map statuses to canonical status;
- map dates;
- map estimates;
- map dependencies;
- preserve raw metadata;
- write import report with warnings.

Example commands:

```bash
replan import openproject --project demo --output snapshots/demo.json
replan workload --snapshot snapshots/demo.json --week 2026-W28
```

Exit criteria:

- real OpenProject project can be imported;
- missing fields are reported;
- workload report works from imported data.

## Phase 5: OpenProject dry-run write-back

Duration target: 2-4 days.

Goal:

Show safe write-back diffs without mutating OpenProject.

Actions:

- represent approved planning changes;
- generate OpenProject update operations;
- print before/after diff;
- include scenario ID and explanation;
- check local capability policy before write-back dry-run/apply;
- write audit event for dry-run and later apply;
- require explicit `--write-openproject` for mutation later.

Example:

```bash
replan apply scenario-001 --dry-run --target openproject
```

Exit criteria:

- dry-run output is clear enough for human review;
- no accidental mutation is possible.

## Phase 6: TaskJuggler export proof of concept

Duration target: 3-5 days.

Goal:

Export a RePlan scenario to TaskJuggler.

Actions:

- map resources to TaskJuggler resources;
- map tasks to TaskJuggler tasks;
- map estimates;
- map dependencies;
- map calendars if feasible;
- generate `.tjp`;
- run TaskJuggler manually first;
- inspect generated reports.

Example:

```bash
replan export taskjuggler --scenario baseline --output build/baseline.tjp
```

Exit criteria:

- generated `.tjp` runs;
- TaskJuggler output is useful enough to continue;
- limitations are documented.

## Phase 7: TaskJuggler import as scenario

Duration target: 3-5 days.

Goal:

Use TaskJuggler output as a proposed RePlan scenario.

Actions:

- run TaskJuggler from CLI;
- parse schedule result;
- map result back to canonical task IDs;
- create scenario from TaskJuggler result;
- compare against baseline.

Example:

```bash
replan taskjuggler run --scenario baseline
replan scenario compare baseline taskjuggler-baseline
```

Exit criteria:

- RePlan can show what TaskJuggler would move;
- human can approve or reject suggested changes.

## Phase 8: AI actor interface

Duration target: 3-7 days.

Goal:

Make safe CLI/API commands for AI agents.

Actions:

- define agent permissions;
- define agent trust levels;
- add read-only commands;
- add proposal commands;
- block direct apply by default;
- generate audit entries;
- document safe usage patterns.

Allowed examples:

```bash
replan task list --assigned-to agent_coder_1
replan propose estimate openproject:1234 --remaining-hours 12
replan propose assignment openproject:1234 --resource agent_coder_1
```

Forbidden by default:

```bash
replan apply scenario-001 --write-openproject
```

Exit criteria:

- AI agent can read and propose;
- AI agent has scoped permissions;
- human remains approval gate for write-back.

## Phase 8.5: gateway design for non-human resources

Duration target: design first, implementation only when a real resource appears.

Goal:

Represent non-human resources without turning RePlan into a safety-critical
control system.

Possible resources:

- GPU machines;
- robot controllers;
- test benches;
- deployment environments;
- CI runner pools;
- lab equipment.
- API quotas;
- LLM provider budgets;
- SaaS subscription seat pools;
- rate-limited external services;
- cloud spending budgets;
- build minutes.

Gateway contract:

- read availability;
- read current reservation;
- propose reservation;
- dry-run reservation;
- apply approved reservation;
- record evidence;
- emit audit event.

Exit criteria:

- RePlan can model non-human resources;
- RePlan can model consumable/service resources without treating them as human
  roles;
- direct dangerous control remains outside RePlan;
- every gateway operation is policy-checked and audited.

## Phase 9: minimal web UI

Duration target: only after CLI proves useful.

Goal:

Expose the most useful reports visually.

Views:

- weekly resource heatmap;
- resource detail page;
- task list grouped by assignee;
- overload warnings;
- missing-data warnings;
- scenario comparison;
- dry-run approval view.

Non-goal:

- full PM suite UI.

## Phase 10: public OSS release

Do this only after internal use proves the tool.

Requirements:

- README explains the category clearly;
- examples run locally;
- license chosen;
- installation works;
- CLI is coherent;
- OpenProject adapter is useful or clearly experimental;
- TaskJuggler integration status is clear;
- contribution policy exists;
- no private credentials or project-specific data is included.

## Stop conditions

Stop or rethink if:

- manual spreadsheet planning remains faster and clearer;
- the model becomes too complex before first useful report;
- OpenProject API friction dominates the project;
- TaskJuggler mapping consumes all effort with little value;
- reports do not change operator decisions;
- the project drifts into building a full PM clone.

## Current next action

The original Phase 1 through the internal CLI-first MVP are implemented. The
private OpenProject live sidecar pilot has passed, but private pilot details
must stay outside the public OSS repository.

The current public dogfood direction is:

```text
RePlan is planned with RePlan
```

The next public example should be `examples/replan-self`. It should model
RePlan implementation, documentation, review, CI, AI-agent, quota and release
work without depending on a private external project.

The current completed public design action is:

```text
design the generated static observability dashboard over examples/replan-self
```

The current completed public implementation action is:

```text
implement the first local generated static dashboard artifact set
```

The next public operator action is:

```text
inspect the generated dashboard and decide the next dashboard/usefulness slice
```

Use the self-dogfood example as the input. Keep private real-world pilots as
external validation, not as the public RePlan build story.
