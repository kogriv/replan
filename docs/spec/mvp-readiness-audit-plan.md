# RePlan MVP readiness audit plan

Date: 2026-07-24

Status: planned audit procedure.

## Purpose

This document defines how to run the next RePlan MVP readiness audit.

The audit is intentionally designed so two independent operators or AI CLI
agents can run it in parallel and produce comparable results.

The audit should answer:

- whether the current RePlan implementation is usable as an internal MVP;
- which documented MVP criteria are actually satisfied by code, commands,
  tests and generated artifacts;
- which gaps are true MVP blockers;
- which gaps are acceptable limitations for the first internal MVP;
- what the next smallest stabilization slice should be.

The audit must not become a feature design exercise. If a missing capability is
found, record it as a finding. Do not implement it during the audit.

## Scope

In scope:

- CLI behavior;
- local file-based project operation;
- `examples/small-team`;
- `examples/replan-self`;
- validation;
- workload reports;
- weekly reports;
- project-state reports;
- static dashboard generation;
- scenarios;
- proposals;
- actual consumption and audit records;
- OpenProject read-only import from fixtures;
- TaskJuggler optional export/run/import loop where available;
- documentation usability for a first operator;
- CI and smoke coverage;
- AI CLI friendliness.

Out of scope:

- building a web application;
- building a daemon or server API;
- adding custom authentication;
- adding full role-based access control;
- adding OpenProject write-back;
- changing the canonical model;
- adding a scheduler or optimizer;
- rewriting private or public repository history;
- cleaning unrelated documentation;
- implementing fixes discovered by the audit.

## Inputs

Each auditor should inspect these files:

- `README.md`;
- `docs/README.md`;
- `docs/spec/mvp-readiness.md`;
- `docs/spec/mvp-acceptance.md`;
- `docs/spec/mvp-boundary.md`;
- `docs/spec/implementation-sequence.md`;
- `docs/spec/cli.md`;
- `docs/spec/report-behavior.md`;
- `docs/spec/planning-lifecycle.md`;
- `docs/spec/workload-calculation.md`;
- `docs/spec/scenario-model.md`;
- `docs/spec/evidence-proposal-behavior.md`;
- `docs/spec/pm-system-interaction.md`;
- `docs/spec/static-observability-dashboard.md`;
- `docs/spec/openproject-operator-loop.md`;
- `docs/dev/openproject-pilot-lessons.md`;
- `cmd/replan`;
- `internal/app`;
- `internal/reports`;
- `internal/storage`;
- `examples/small-team`;
- `examples/replan-self`;
- `examples/imports/openproject`;
- `.github/workflows`.

The auditor may inspect other files if needed, but findings should point back
to the relevant documented MVP expectation.

## Ground Rules

Each auditor must:

- start from the current repository checkout;
- record the current commit hash;
- avoid relying on memory from previous conversations;
- run commands against the repository instead of inferring behavior from docs;
- avoid modifying source files, fixtures or docs during the audit;
- use temporary directories for mutation tests;
- clearly distinguish command evidence from interpretation;
- clearly distinguish MVP blockers from acceptable limitations;
- avoid treating deferred non-goals as blockers.

Allowed local temporary paths:

```text
tmp/mvp-readiness-audit/<auditor-id>/
```

The audit report should be written after command execution, not while guessing
expected results.

## Status Vocabulary

Use exactly these statuses:

```text
done
partial
blocked
deferred
not_started
```

Meanings:

- `done`: implemented, documented and covered by direct command, test, smoke or
  CI evidence.
- `partial`: useful behavior exists, but an important MVP expectation is weak,
  undocumented, confusing or under-tested.
- `blocked`: the MVP should not be called ready until this issue is fixed.
- `deferred`: explicitly outside first internal MVP scope.
- `not_started`: documented as desired, but no meaningful implementation or
  evidence exists.

## Required Command Evidence

Run these commands from the repository root:

```bash
git rev-parse --short HEAD
git status --short
go test ./...
./scripts/smoke-mvp.sh
go run ./cmd/replan --help
go run ./cmd/replan validate examples/small-team --format json
go run ./cmd/replan validate examples/replan-self --format json
go run ./cmd/replan workload --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format markdown
go run ./cmd/replan report weekly --project examples/small-team --week 2026-W28 --format json
go run ./cmd/replan scenario diff --project examples/small-team scenario.ivan-unavailable-week-28 --against current --format json
go run ./cmd/replan proposal list --project examples/small-team --format json
go run ./cmd/replan report state --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan report state --project examples/replan-self --week 2026-W31 --format json
go run ./cmd/replan report dashboard --project examples/replan-self --week 2026-W31
go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --format json
```

Run CI inspection when GitHub CLI authentication is available:

```bash
gh run list --repo kogriv/replan --branch main --limit 5
```

Run TaskJuggler checks when `tj3` is available locally. If it is not available,
record that local TaskJuggler runtime evidence was unavailable and use CI
evidence if present.

```bash
./scripts/validate-taskjuggler.sh
./scripts/run-taskjuggler.sh
```

## Mutation Evidence

Mutation checks must use a temporary copy.

```bash
rm -rf tmp/mvp-readiness-audit/<auditor-id>/small-team
cp -R examples/small-team tmp/mvp-readiness-audit/<auditor-id>/small-team
go run ./cmd/replan actual add-consumption --project tmp/mvp-readiness-audit/<auditor-id>/small-team --task task.op.1234 --demand resource_demand.task-op-1234.implementer --resource resource.human.ivan --actor actor.human.operator --amount 1 --unit hours --occurred-at 2026-07-10T10:00:00+07:00 --status-date 2026-07-10 --evidence evidence.github.pr.45 --apply --format json
go run ./cmd/replan validate tmp/mvp-readiness-audit/<auditor-id>/small-team --format json
```

If the exact example ids above do not exist, the auditor should find an
equivalent existing task, demand, resource, actor and evidence record and
record the substituted ids.

Proposal apply checks must also use a temporary copy. Prefer an existing open
proposal fixture if available. If there is no suitable proposal, record the
gap instead of inventing fixture data inside the source tree.

## OpenProject Fixture Evidence

The audit should verify that OpenProject remains useful without private
infrastructure.

Run the documented import fixture path if available in CLI help and docs.

The auditor should record:

- whether import from a file fixture works;
- whether import snapshot output is created;
- whether import accept dry-run works on a temporary project;
- whether private endpoint details are absent from public docs and fixtures;
- whether live OpenProject access is required for the minimum MVP loop.

Live private OpenProject checks are optional for this audit and must not be
required to call the MVP ready.

## Static Dashboard Evidence

After running dashboard generation, inspect:

- generated artifact directory;
- `index.html`;
- `gantt.html`, if generated;
- `raw/overview.json`;
- `raw/gantt.json`, if generated;
- raw manifest file.

Record whether the static dashboard lets an operator see:

- project summary;
- validation health;
- workload pressure;
- resource allocation over time;
- task timeline;
- proposals;
- tracking proposal loop;
- actionable next commands.

This is a usability judgment. It should be marked as interpretation, not as
command evidence.

## AI CLI Friendliness Evidence

Assess whether an AI CLI agent can operate RePlan safely.

Check:

- stable JSON envelopes;
- deterministic ordering in reports;
- explicit `ok` result field where documented;
- useful non-zero exit codes for errors;
- dry-run support for mutating commands;
- required actor field for mutating commands;
- clear proposal review loop;
- generated next commands in tracking/report surfaces;
- no requirement for a web browser to perform core actions.

## Documentation Usability Evidence

Assess whether a new operator can start from docs without reading the code.

Check:

- README quickstart commands exist and run;
- docs map points to the right spec files;
- MVP boundary is visible;
- OpenProject private details are not required for basic use;
- TaskJuggler is clearly optional;
- generated report paths are documented;
- mutation safety rules are documented.

## Required Audit Output

Each auditor should produce a Markdown report using this structure:

```markdown
# MVP readiness audit <auditor-id> <date>

Commit: `<hash>`

## Verdict

Status: `usable_internal_mvp` | `usable_with_limitations` | `not_ready`

One-paragraph rationale.

## Command Evidence

| Check | Command | Result | Notes |
| --- | --- | --- | --- |

## MVP Criteria

| Criterion | Status | Evidence | Notes |
| --- | --- | --- | --- |

## Additional Readiness Dimensions

| Dimension | Status | Evidence | Notes |
| --- | --- | --- | --- |

## Blockers

| Finding | Severity | Evidence | Recommended next action |
| --- | --- | --- | --- |

## Acceptable Limitations

| Limitation | Reason it is acceptable for MVP |
| --- | --- |

## Documentation Drift

| Document | Drift | Recommended fix |
| --- | --- | --- |

## Next Stabilization Slice

Recommended next slice:

Rationale:
```

## Comparison Protocol

After two independent audit reports exist:

1. Compare final verdicts.
2. Compare every `blocked` and `partial` finding.
3. Treat matching findings as high confidence.
4. Treat conflicting findings as requiring a short reproduction command.
5. Prefer a small stabilization slice that resolves the highest-confidence
   blocker or the most painful operator usability gap.

Do not average the verdicts. If either auditor finds a reproducible blocker,
the project should not be called ready until that blocker is resolved or
explicitly reclassified as an acceptable limitation.

## Expected Final Decision

The audit should end with one of these decisions:

- `usable_internal_mvp`: the current CLI-first loop is ready for real
  self-dogfood use.
- `usable_with_limitations`: the loop is usable, but a short list of known
  limitations must be visible before use.
- `not_ready`: at least one blocker prevents honest MVP use.

The preferred next action after the audit is a single stabilization slice, not
a broad feature expansion.
