# RePlan documentation

This directory contains product, architecture, roadmap and research notes for
RePlan.

Project-level entry points:

- [`../CONTRIBUTING.md`](../CONTRIBUTING.md) — how to choose work, preserve
  RePlan's safety and privacy contracts, and run the local gate.
- [`../SECURITY.md`](../SECURITY.md) — supported versions and the private
  vulnerability reporting process.
- [`../ROADMAP.md`](../ROADMAP.md) — what works today, the next directions
  and the non-goals.
- [`../AGENTS.md`](../AGENTS.md) — working rules for AI agents in this
  repository, including the backlog rule and the privacy boundary.

## Current documents

### Spec Work

- [`spec/design-work-plan.md`](spec/design-work-plan.md) — closed historical
  discussion checklist; every design topic is documented in an approved spec
  (see its closure map).
- [`spec/functional-design-checklist.md`](spec/functional-design-checklist.md)
  — active checklist for closing the functional design of planning and
  execution tracking.
- [`spec/canonical-model.md`](spec/canonical-model.md) — approved implementation
  specification for the source-agnostic canonical model, including tasks,
  resources, demands, capacity, actuals, scenarios, evidence, proposals,
  actors, policy references and audit records.
- [`spec/storage-layout.md`](spec/storage-layout.md) — approved implementation
  specification for local-first file storage, canonical state, imports,
  exports, reports, audit, Git use and future SQLite/server migration path.
- [`spec/cli.md`](spec/cli.md) — approved implementation specification for the
  `replan` command-line interface, global flags, output envelope, exit codes,
  command families, mutation rules and AI CLI safety.
- [`spec/github-import-document.md`](spec/github-import-document.md) —
  versioned, network-neutral GitHub Issues file/live-input and sidecar contract.
- [`spec/implementation-sequence.md`](spec/implementation-sequence.md) —
  approved implementation specification for the ordered path from Go skeleton and
  fixtures to validation, workload reports, scenarios, proposals, audit,
  read-only OpenProject import and optional exports.
- [`spec/planning-lifecycle.md`](spec/planning-lifecycle.md) — approved
  functional design for task entry, draft planning, current plan, baseline,
  actuals, scenarios and re-planning.
- [`spec/task-state-model.md`](spec/task-state-model.md) — approved internal
  status model separating source status, tracking status, proposals, flags,
  warnings, blockers, evidence and actuals.
- [`spec/resource-demand-model.md`](spec/resource-demand-model.md) — approved
  model for task resource requirements, including multiple demands, units,
  criticality, pools, planned/remaining/reserved/actual amounts and MVP scope.
- [`spec/capacity-availability-rules.md`](spec/capacity-availability-rules.md)
  — approved model for resource capacity, availability windows, exceptions,
  reservations, quotas, rate limits, stock, budget and warnings.
- [`spec/workload-calculation.md`](spec/workload-calculation.md) — approved
  deterministic workload calculation rules for buckets, spread modes, capacity
  comparison, forecasts, non-hour resources and structured warnings.
- [`spec/planned-current-actual-comparison.md`](spec/planned-current-actual-comparison.md)
  — approved comparison model for baseline, current plan, actuals, forecast,
  variance formulas and capacity shortage separation.
- [`spec/dependency-deadline-behavior.md`](spec/dependency-deadline-behavior.md)
  — approved dependency and deadline model for MVP warnings, feasibility
  checks, deadline risk and later TaskJuggler integration.
- [`spec/scenario-model.md`](spec/scenario-model.md) — approved scenario model
  using isolated what-if overlays, explicit changes, read-only calculation,
  diffs and explicit application.
- [`spec/evidence-proposal-behavior.md`](spec/evidence-proposal-behavior.md)
  — approved safety model for evidence, proposals, accepted changes, actor
  trust and audit records.
- [`spec/report-behavior.md`](spec/report-behavior.md) — approved report model
  for weekly, workload, tracking, warnings, scenario, evidence/proposal,
  JSON, Markdown, CSV and implemented static HTML weekly output.
- [`spec/static-observability-dashboard.md`](spec/static-observability-dashboard.md)
  — approved design for the generated read-only dashboard over RePlan project
  state, raw JSON artifacts and operator next actions.
- [`spec/openproject-overlay-strategy.md`](spec/openproject-overlay-strategy.md)
  — accepted staged strategy for improving generated static Gantt first and
  using that contract to decide how far to go toward an OpenProject-native
  surface or plugin.
- [`spec/mvp-acceptance.md`](spec/mvp-acceptance.md) — implemented fast MVP
  acceptance smoke for validate, workload, reports, scenario, proposal,
  actuals, audit and OpenProject file import.
- [`spec/mvp-readiness.md`](spec/mvp-readiness.md) — MVP readiness audit
  definition and dated audit results; the 2026-07-16 and 2026-07-24 audits
  recorded the internal CLI-first MVP as ready with known limitations, and
  the 2026-09-21 full-repository audit is linked from there.
- [`spec/mvp-readiness-audit-plan.md`](spec/mvp-readiness-audit-plan.md) —
  executable plan for running the MVP readiness audit in parallel by independent
  operators or AI CLI agents and comparing their findings.
- [`dev/openproject-pilot-lessons.md`](dev/openproject-pilot-lessons.md) —
  sanitized, current product lessons from OpenProject validation: field and
  endpoint mapping, sidecars, accept flows and mapping-quality gates.
- [`spec/openproject-operator-loop.md`](spec/openproject-operator-loop.md) —
  active operational plan for using OpenProject as the human PM surface and
  RePlan as the resource-planning, reporting, scenario and AI-safe proposal
  layer during self-dogfood and generic OpenProject pilots.
- [`spec/pm-system-interaction.md`](spec/pm-system-interaction.md) — approved
  PM-system interaction model for task ownership, source references, read-only
  import, mismatch warnings and explicit audited write-back later.
- [`spec/taskjuggler-capacity-quota-mapping.md`](spec/taskjuggler-capacity-quota-mapping.md)
  — approved TaskJuggler capacity/quota mapping boundary and weekly-hour
  `limits` mapping.
- [`spec/taskjuggler-run-import-experiment.md`](spec/taskjuggler-run-import-experiment.md)
  — approved TaskJuggler run/import experiment design; run-only artifact
  capture, import-result dry-run preview and scenario/proposal candidate file
  generation, proposal accept/reject approval gate and task-date scenario apply
  are implemented.
- [`spec/taskjuggler-parseable-report-contract.md`](spec/taskjuggler-parseable-report-contract.md)
  — implemented minimal TaskJuggler CSV report, ID mapping and
  `import-result` preview/candidate write contract.
- [`spec/mvp-boundary.md`](spec/mvp-boundary.md) — approved MVP boundary,
  starting with the MVP purpose and its AI-CLI-first, human-verifiable and
  minimal policy/audit requirements; the bounded MVP is implemented and audited
  ready.
- [`spec/tracking-events.md`](spec/tracking-events.md) — approved model for
  event-assisted execution tracking across PM systems, git hosts, CI, gateways,
  humans and AI agents.

### Product

- [`product/positioning.md`](product/positioning.md) — what RePlan is, who it is
  for, what problem it solves and what it deliberately does not solve.
- [`product/naming-and-foss-model.md`](product/naming-and-foss-model.md) —
  working name, package-name checks, FOSS-first policy and monetization options.

### Roadmap

- [`roadmap/post-mvp-dogfood-plan.md`](roadmap/post-mvp-dogfood-plan.md) —
  approved post-MVP operating plan for using RePlan on RePlan before choosing
  larger product directions.
- [`roadmap/self-dogfood-planning-calibration.md`](roadmap/self-dogfood-planning-calibration.md)
  — completed self-dogfood planning calibration, including resource capacity,
  reviewer policy, Codex limits, weekly planning horizon and calibration
  closure.
- [`roadmap/openproject-replan-sync-plan.md`](roadmap/openproject-replan-sync-plan.md)
  — approved next block for treating OpenProject as the task source of truth
  and RePlan as the resource-planning sidecar, using fixture-first validation
  before live OpenProject checks.

### Architecture

- [`architecture/system-design.md`](architecture/system-design.md) —
  source-agnostic design, adapters, canonical planning model, internal
  calculation, write-back and AI actor interface.
- [`architecture/deployment-architecture.md`](architecture/deployment-architecture.md)
  — local-first MVP deployment, Git-backed collaboration, optional SQLite and
  future client-server readiness constraints.
- [`architecture/application-runtime.md`](architecture/application-runtime.md)
  — headless-first application runtime, CLI-first MVP, optional local daemon and
  later desktop/web UI boundaries.
- [`architecture/implementation-stack.md`](architecture/implementation-stack.md)
  — Go-first MVP implementation stack, Rust alternative, and rejected core
  runtime options.
- [`architecture/resource-abstraction.md`](architecture/resource-abstraction.md)
  — generic resource model for humans, AI agents, equipment, environments,
  quotas, budgets, subscriptions and future resource classes.
- [`architecture/taskjuggler-integration.md`](architecture/taskjuggler-integration.md)
  — TaskJuggler as optional scheduling engine and planning-capability benchmark.

### Security

- [`security/access-and-actor-model.md`](security/access-and-actor-model.md) —
  minimal access, identity, actor, resource, gateway and agent trust model that
  avoids enterprise overhead while keeping the architecture future-compatible.

### Roadmap

- [`roadmap/initial-roadmap.md`](roadmap/initial-roadmap.md) — phased path from
  documentation to fixtures, CLI reports, OpenProject import, TaskJuggler proof
  of concept and later UI/public release.
- [`roadmap/post-mvp-dogfood-plan.md`](roadmap/post-mvp-dogfood-plan.md) —
  approved post-MVP plan for choosing between RePlan self-dogfood, deeper
  OpenProject integration, known-gap cleanup and the next functional slice.
- [`roadmap/first-dogfood-cycle-2026-07-26.md`](roadmap/first-dogfood-cycle-2026-07-26.md) —
  completed first self-dogfood operating cycle and the cleaned tracking-loop
  state it produced.
- [`roadmap/self-dogfood-planning-calibration.md`](roadmap/self-dogfood-planning-calibration.md) —
  active worksheet for the real RePlan roadmap, resource model and near-term
  planning calibration.

### Development

- [`dev/getting-started.md`](dev/getting-started.md) — current local developer
  commands for tests, example validation and workload calculation.
- [`dev/release-process.md`](dev/release-process.md) — operator runbook for
  tagging, deterministic local release builds, verification and GitHub release
  publication.
- [`dev/second-tracker-adapter-design.md`](dev/second-tracker-adapter-design.md)
  — approved GitHub Issues adapter design and file/live-input implementation status.
- [`dev/local-web-view-design.md`](dev/local-web-view-design.md) — proposed
  decision to complete the static dashboard pages instead of building a
  served web view.
- [`dev/estimate-calibration-design.md`](dev/estimate-calibration-design.md)
  — proposed design for turning estimate accuracy into a sealed, approved
  calibration applied to open work through a scenario.
- [`dev/first-project-onboarding-design.md`](dev/first-project-onboarding-design.md)
  — approved `replan init`, resource and capacity commands and a tested
  quickstart guide for a first project of one's own.
- [`dev/openproject-helpers.md`](dev/openproject-helpers.md) — how to call
  the OpenProject import, drift, bootstrap and write-back helpers, with the
  private binding layout and the operating direction.
- [`dev/backlog-operating-convention.md`](dev/backlog-operating-convention.md)
  — active convention for keeping agreed work in the OpenProject/RePlan
  dogfood backlog instead of only in chat or roadmap prose.
- [`dev/oss-separation-plan.md`](dev/oss-separation-plan.md) — active cleanup
  and backlog plan for keeping the RePlan OSS repository separate from private
  pilots, infrastructure notes and personal project data.
- [`dev/oss-boundary-checkpoint-2026-08-02.md`](dev/oss-boundary-checkpoint-2026-08-02.md)
  — completed checkpoint separating safe private incubation from blocked public
  OSS release work.
- [`dev/openproject-functionality-and-pitfalls.md`](dev/openproject-functionality-and-pitfalls.md)
  — accepted reconstruction of OpenProject functionality, API pitfalls,
  private binding rules and AI CLI operating-layer requirements before the full
  OpenProject-centered dogfood cycle.
- [`dev/openproject-agent-interface-options.md`](dev/openproject-agent-interface-options.md)
  — accepted evaluation of OpenProject web UI, official MCP, official CLI,
  direct API and RePlan-owned guarded helpers for AI CLI agent workflows.
- [`dev/openproject-agent-tools-hands-on-audit.md`](dev/openproject-agent-tools-hands-on-audit.md)
  — accepted hands-on audit of the official OpenProject CLI, current live MCP
  endpoint behavior, direct API patterns and what RePlan should borrow for the
  OpenProject AI CLI operating layer.
- [`dev/openproject-ai-cli-operating-layer-design.md`](dev/openproject-ai-cli-operating-layer-design.md)
  — accepted design for RePlan-owned OpenProject helper commands, dry-run and
  apply gates, private bindings, drift classes, evidence/audit behavior and the
  minimum preflight implementation slice.
- [`dev/openproject-work-package-operator-toolkit-design.md`](dev/openproject-work-package-operator-toolkit-design.md)
  — accepted design for the fuller OpenProject work-package manipulation
  toolkit needed before live plan refactor: inspect/list/catalog reads,
  guarded metadata/status/date/comment writes, binding updates, form validation
  and prompt-injection boundaries.
- [`dev/openproject-native-replan-surface-design.md`](dev/openproject-native-replan-surface-design.md)
  — accepted first-slice design for the OpenProject-centered RePlan surface:
  OpenProject remains the human task and Gantt surface, RePlan provides the
  adjacent resource-planning layer, and plugin work is staged after Level 0 and
  Level 1 validation.
- [`dev/openproject-read-only-plugin-feasibility.md`](dev/openproject-read-only-plugin-feasibility.md)
  — accepted decision that a read-only OpenProject plugin is mandatory for the
  OpenProject-centered human workflow and should consume RePlan overlay
  artifacts instead of owning RePlan data.
- [`dev/openproject-overlay-plugin-consumer-scaffold.md`](dev/openproject-overlay-plugin-consumer-scaffold.md)
  — implementation scaffold plan for the smallest read-only OpenProject plugin
  consumer over `reports/latest/raw/openproject_overlay.json`.
- [`dev/openproject-plugin-deployment-path.md`](dev/openproject-plugin-deployment-path.md)
  — accepted deployment design for safely moving the read-only RePlan overlay
  plugin from scaffold to runtime: custom OpenProject image, overlay artifact
  delivery, throwaway runtime test, live preflight and rollback.
- [`agent-kit/README.md`](agent-kit/README.md)
  — the guide shipped in the agent kit: how an outside person or agent uses
  RePlan and the OpenProject helpers from a release archive, with an ordinary
  member's token and without this repository.
- [`dev/public-repository-cut.md`](dev/public-repository-cut.md)
  — publication-day runbook for producing, publishing and switching in place
  to the one-commit public repository while preserving ignored local files and
  the private history archive.
- [`dev/openproject-work-package-link-research.md`](dev/openproject-work-package-link-research.md)
  — OpenProject 17.6 extension points for reaching the RePlan task page from a
  work package, and the proposed link custom field.
- [`dev/openproject-plugin-throwaway-runtime-test.md`](dev/openproject-plugin-throwaway-runtime-test.md)
  — completed disposable OpenProject 17 runtime proof for the read-only RePlan
  overlay plugin, including custom-image build, route/menu/permission
  registration, valid overlay loading and safe missing-overlay behavior; the
  check now uses the shared optimized image build helper.
- [`dev/openproject-plugin-live-installation-preflight.md`](dev/openproject-plugin-live-installation-preflight.md)
  — accepted private live OpenProject plugin installation preflight: target
  deployment class, custom image candidate, module-copy fix, overlay artifact
  shape, compose override shape, rollback requirements and the separate gated
  live-installation task.
- [`dev/openproject-plugin-live-installation-result.md`](dev/openproject-plugin-live-installation-result.md)
  — completed first private live OpenProject plugin installation: backup,
  failed floating-tag attempt and rollback, exact OpenProject 17.6.0 image fix,
  final healthy install and plugin route/overlay verification.
- [`dev/openproject-work-package-replan-panel-design.md`](dev/openproject-work-package-replan-panel-design.md)
  — accepted first implementation design and current live deployment state for
  the OpenProject-native RePlan work-package panel driven by the same overlay
  artifact as the project-level plugin page.
- [`dev/documentation-sync-audit-2026-08-15.md`](dev/documentation-sync-audit-2026-08-15.md)
  — completed synchronization pass for the current OpenProject/RePlan
  documentation state after the live plugin installation, project-level
  resource view and work-package panel design work.
- [`dev/project-audit-2026-09-21.md`](dev/project-audit-2026-09-21.md)
  — dated full-repository audit of HEAD, the uncommitted post-pause re-entry
  working tree, tests and MVP smoke, self-dogfood hygiene, live OpenProject
  operator-loop reachability, CLI/spec drift, known gaps, OSS boundary and
  the next stabilization slice.
  Reviewed critically in Step 55 of the calibration worksheet: it missed that
  CI on `main` had been red since 2026-08-07.
- [`dev/next-product-slice-decision-2026-09-24.md`](dev/next-product-slice-decision-2026-09-24.md)
  — next product slice chosen from RePlan reports: fix what the state report
  says the operator should do next.
- [`dev/lint-baseline-2026-09-24.md`](dev/lint-baseline-2026-09-24.md)
  — recorded golangci-lint baseline behind the CI gate on new findings.
- [`dev/actuals-staging-review-2026-09-24.md`](dev/actuals-staging-review-2026-09-24.md)
  — second-pass review of the staged-write fix for actual commands and its
  resolution.
- [`dev/code-quality-tooling-evaluation-2026-09-24.md`](dev/code-quality-tooling-evaluation-2026-09-24.md)
  — bounded trial of Go-native analyzers and a code-graph tool on RePlan, with
  another code-analysis project's tool survey as candidate source, and a
  recommendation.
- [`dev/openproject-wiki-api-gap.md`](dev/openproject-wiki-api-gap.md)
  — recorded OpenProject 17.6.0 platform gap: an API-token participant can
  neither read nor write wiki page text; evidence, impact, temporary measure
  and considered directions.
- [`dev/openproject-wiki-api-plugin-design.md`](dev/openproject-wiki-api-plugin-design.md)
  — accepted product direction and first implementation design for a separate
  OpenProject plugin that extends API v3 with wiki page collection, text read,
  create and update through upstream services and standard wiki permissions.
  Coding is withheld until the 2026-09-21 review amendments are applied.
- [`dev/openproject-wiki-api-live-deployment-result.md`](dev/openproject-wiki-api-live-deployment-result.md)
  — sanitized result of the live wiki API plugin deployment: backup, exact-image
  transfer, guarded switch, read-only verification and the stale-cache finding.
- [`dev/openproject-wiki-api-throwaway-runtime-test.md`](dev/openproject-wiki-api-throwaway-runtime-test.md)
  — throwaway OpenProject 17.6.0 proof of the wiki API plugin: boot with both
  gems, fifteen HTTP cases with API tokens, database facts, the four design
  verification points, and the boot-blocking module-name defect it found.
- [`dev/openproject-wiki-api-plugin-implementation-review.md`](dev/openproject-wiki-api-plugin-implementation-review.md)
  — gate 1 implementation review of the wiki API plugin: `changes_required`,
  two load-order blockers, four should-fix items and the design amendments
  that followed.
- [`dev/openproject-wiki-api-plugin-design-review.md`](dev/openproject-wiki-api-plugin-design-review.md)
  — unabridged 2026-09-21 review of the wiki API plugin design: product
  direction accepted; implementation design not accepted for coding until
  lockVersion, write-path, collection representer and collection query
  amendments land.
- [`dev/dashboard-load-profile-semantics-design.md`](dev/dashboard-load-profile-semantics-design.md)
  — accepted design for replacing misleading daily resource bars with
  capacity-height resource allocation rows, weekly load profiles, visible
  100 percent waterlines and overload above the waterline.
- [`dev/repository-boundary-contract.md`](dev/repository-boundary-contract.md)
  — operational three-repository boundary for public product code, private
  pilot data and private infrastructure.
- [`dev/known-gaps.md`](dev/known-gaps.md) — active register of verified
  code/test/documentation gaps that are not tracked by another plan or audit.

### Research

- [`research/market-gap-and-alternatives-2026-07-02.md`](research/market-gap-and-alternatives-2026-07-02.md)
  — market gap note with alternatives, public pricing snapshots and why RePlan
  is not simply "resource planning from scratch".
- [`research/quollnet-resource-planner-deep-dive-2026-07-02.md`](research/quollnet-resource-planner-deep-dive-2026-07-02.md)
  — deeper assessment of Quollnet Resource Planner as possible base, fork,
  reference UI or import format.
- [`research/pm-substrate-research-2026-07-02.md`](research/pm-substrate-research-2026-07-02.md)
  — research copied from OrCode on OpenProject, Plane, YouTrack, Jira and PM
  substrate choices.
- [`research/resource-planner-lite-2026-07-02.md`](research/resource-planner-lite-2026-07-02.md)
  — earlier design note that led to RePlan before the product name was chosen.

## Important settled decisions

- The working product name is **RePlan**.
- RePlan means both `REsursnoe PLANirovanie` and `re-plan`.
- RePlan should be FOSS-first.
- Core resource planning must not be paywalled.
- The public dogfood fixture is `../examples/replan-self/`: RePlan planned
  with RePlan.
- OpenProject is the first likely task source, not the domain model.
- OpenProject is the first human PM surface for generic and self-dogfood
  operator-loop validation.
- Agreed non-trivial RePlan work should be represented in the OpenProject/RePlan
  dogfood backlog: OpenProject for human-visible tasks and RePlan for
  resources, capacity, scenarios, evidence, proposals and audit.
- RePlan's public dogfood project should be RePlan itself, not a private
  external project.
- RePlan must support multiple task sources through adapters.
- TaskJuggler is optional and useful as an advanced scheduling engine.
- RePlan must be useful without TaskJuggler for basic workload reporting.
- RePlan should not implement its own authentication provider.
- RePlan should design for external auth, scoped capabilities, audit records and
  agent/resource gateways from the beginning.
- AI actors should use explicit read/propose/dry-run commands, not mutate free
  planning text directly.
