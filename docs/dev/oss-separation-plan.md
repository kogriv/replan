# OSS separation and self-dogfood plan

Date: 2026-07-20

Status: active cleanup and backlog plan.

## Purpose

RePlan is intended to become an OSS project.

The repository must therefore separate:

- public product code and examples;
- public design history;
- synthetic reproducible dogfood data;
- private real-world pilot evidence;
- private infrastructure notes.

This document records the boundary before private pilot details become deeply
mixed into the public RePlan story.

Detailed repository boundary rules are defined in
[`repository-boundary-contract.md`](repository-boundary-contract.md).

## Problem

The repository started using a private real-world pilot as the next operational
test bed.

That was useful for quickly validating OpenProject, source references,
dependency relations, workload reports and operator-loop ideas. However, it is
not a good long-term OSS narrative.

Problems:

- private project names and domains do not belong in a reusable public example;
- real infrastructure details belong in an infrastructure repository, not in
  the product repository;
- private pilot tasks make the build story harder to understand;
- future OSS users should see a clean example they can run locally;
- history may need sanitizing before a public release if private details have
  already been committed.

## Repository roles

### RePlan repository

The RePlan repository should contain:

- product code;
- implementation specifications;
- public architecture decisions;
- synthetic example projects;
- sanitized OpenProject fixtures;
- self-dogfood planning data for RePlan itself;
- OSS readiness notes;
- release and contribution documentation.

It should not contain:

- private project operating logs;
- private domain names;
- private hosting details;
- private OpenProject project identifiers;
- private task content;
- credentials or credential locations;
- personal brand planning data.

### Private pilot repository

A private pilot repository may contain real use.

It may include:

- real task data;
- real OpenProject ids;
- private project reports;
- private domains;
- private operational notes;
- private resource assumptions;
- local runbooks.

The private pilot may use RePlan, but it should not define the public RePlan
example or OSS story.

### Infrastructure repository

The infrastructure repository should contain deployment operations.

It may include:

- private hosting layout;
- nginx configuration;
- OpenProject deployment details;
- monitoring;
- certificate and DNS operational notes;
- private service runbooks.

RePlan should only document the generic integration boundary, not concrete
private deployment details.

## Self-dogfood direction

The public dogfood project should be RePlan itself.

Working example:

```text
examples/replan-self/
```

Purpose:

```text
RePlan is planned with RePlan.
```

This example should model the work required to build and release RePlan:

- design static observability dashboard;
- implement OpenProject import improvements;
- define resource-demand sidecars;
- improve dependency proposals;
- improve TaskJuggler optional integration;
- prepare OSS cleanup;
- write contributor documentation;
- prepare release packaging;
- run documentation audits.

Resource examples:

- maintainer time;
- AI CLI agent time;
- reviewer role;
- GitHub Actions minutes;
- OpenAI or other API quota;
- documentation time;
- release window;
- local development environment;
- test OpenProject instance.

This keeps dogfooding close to the product and avoids drifting into unrelated
private work.

## Cleanup policy from 2026-07-20 onward

From this point:

1. Do not add new private project names or private domains to public RePlan
   documentation.
2. Keep real pilot evidence in a private pilot repository or infrastructure
   repository.
3. Use `examples/replan-self` for public dogfood work.
4. Use generic names in public docs:
   - `private pilot`;
   - `test OpenProject instance`;
   - `example project`;
   - `project-alpha`;
   - `op.example.internal`.
5. Keep credentials and credential paths out of the RePlan repository.
6. Treat any remaining private references in git history as an OSS release
   sanitization item.

## Backlog

### 1. Clean current HEAD

Status: done.

Goal:

Remove private pilot names, private domains and private operational endpoints
from the current repository state.

Expected result:

- repository search for private project names, private domains and private
  local paths returns no public documentation hits;
- README and docs index point to this separation plan;
- current next action points to RePlan self-dogfood, not private pilot work.

### 2. Create `examples/replan-self`

Status: done.

Goal:

Create a public, reproducible dogfood fixture for RePlan planning RePlan.

Expected result:

- local-first RePlan project under `examples/replan-self`;
- tasks represent real RePlan work but no private external project;
- resources include maintainer, AI CLI agents, reviewer role, CI minutes,
  API quota and documentation time;
- validation passes;
- workload and weekly report produce useful signals.

Observed result:

- `examples/replan-self` contains 7 tasks, 20 resource demands, 8 resources,
  7 dependencies, 2 actuals, 2 evidence records, 1 open proposal and 1 scenario;
- validation passes with only intentional unresolved dependency warnings;
- workload for `2026-W31` shows maintainer time overload and reviewer role
  overload;
- weekly Markdown and static HTML reports render;
- scenario diff for `scenario.maintainer-unavailable-20260723` identifies the
  maintainer resource as affected.

### 3. Redirect operator-loop work to self-dogfood

Status: done.

Goal:

Use `examples/replan-self` as the default project for designing the static
observability dashboard and operator overview.

Expected result:

- dashboard design references self-dogfood data;
- generated static dashboard can become an OSS demo;
- private pilot remains optional external validation, not the public example.

Observed result:

- `docs/spec/static-observability-dashboard.md` uses `examples/replan-self` as
  the first public dashboard input;
- the design keeps private pilots as validation inputs only;
- the next implementation slice is a local generated static dashboard, not a
  private OpenProject or infrastructure feature.

### 4. Formalize repository boundary contract

Status: done.

Goal:

Document the three-repository split as an operational contract, not only as an
informal direction.

Expected result:

- public product repository boundary is explicit;
- private pilot repository boundary is explicit;
- infrastructure repository boundary is explicit;
- daily working rules are explicit;
- pre-release OSS audit checks are explicit;
- history strategy options are explicit.

Observed result:

- `docs/dev/repository-boundary-contract.md` defines the boundary and links
  back to this plan.

### 5. Preserve private pilot outside OSS scope

Status: completed 2026-10-27.

Goal:

Keep private real-world validation useful without mixing it into the public
repository.

Expected result:

- private pilot docs live outside RePlan;
- RePlan only records generic lessons learned;
- no private endpoint or project-specific evidence is required for CI, tests or
  examples.

Progress 2026-07-24 (post-audit stabilization slice):

- the dated OpenProject pilot journal that was interleaved with the readiness
  audit was extracted to `docs/spec/openproject-pilot-log.md` (moved to private
  storage on 2026-10-27), isolating the pilot narrative in one relocatable
  file;
- private endpoint values were genericized in the three docs that carried them
  (`README.md`, `getting-started.md` and
  `openproject-live-mapping-quality.md`, moved to private storage on
  2026-10-27) plus the new pilot log: the private port and host alias were
  replaced with generic examples (see `../dev/known-gaps.md` GAP-010);
- `no private endpoint or project-specific evidence is required for CI, tests
  or examples` was already true and remains verified.

Completion 2026-10-27:

- the pilot journal, mapping audit and old getting-started implementation
  history were moved to private storage;
- the pilot-specific sidecars were removed from the public examples;
- [`openproject-pilot-lessons.md`](openproject-pilot-lessons.md) preserves the
  reusable, current product conclusions without operational detail.

### 6. OSS release history decision

Status: resolved 2026-10-28; public-tree relocation completed 2026-10-27.

Goal:

Decide how to produce a clean public history before OSS release.

Options:

1. create a new clean public repository from sanitized HEAD;
2. rewrite current repository history before public release;
3. keep history private and publish only release snapshots.

Preferred direction:

```text
Create a clean public repository or clean public branch from sanitized state
when RePlan is ready for OSS publication.
```

Do not rewrite history casually during active private incubation.

Update 2026-10-27: items 5 and 7 removed the private pilot narrative and
sidecars from the current tree. The separate choice among the history options
above remains open.

Resolved 2026-10-28: the operator chose option 1, a new public repository with
one first commit from the sanitized tree, while this repository's history stays
private. `scripts/cut-public-tree.sh` prepares and checks the tree; repository
renames, creation and pushes are performed only by the operator using the
[public repository cut runbook](public-repository-cut.md).

### 7. Add OSS readiness audit

Status: checkpoint completed 2026-08-02; pilot relocation completed
2026-10-27; other release decisions remain.

Goal:

Add a repeatable audit before public release.

Checks:

- no private project names;
- no private domains;
- no private paths;
- no credential paths;
- no real tokens;
- examples run locally;
- README tells a public story;
- license chosen;
- contribution guide exists;
- CI passes from clean checkout;
- self-dogfood example is understandable.

Checkpoint result:

- see [`oss-boundary-checkpoint-2026-08-02.md`](oss-boundary-checkpoint-2026-08-02.md);
- no committed live token values were found;
- ignored private folders and generated report paths are not tracked;
- `examples/replan-self` uses sanitized OpenProject source references;
- the private pilot narrative and old pilot sidecars were removed from the
  public tree on 2026-10-27;
- public release remains blocked by the contribution/security decision and the
  unresolved public history strategy.

## Current next action

Continue with the full OpenProject-centered dogfood cycle while keeping public
release story work blocked until the checkpoint blockers are resolved or
explicitly accepted by the maintainer.
