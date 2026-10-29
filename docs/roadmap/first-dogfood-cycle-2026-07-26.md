# First RePlan-on-RePlan dogfood cycle

Date: 2026-07-26

Status: completed.

Related task: `task.self.run-first-dogfood-tracking-cleanup`.

## Purpose

This document records the first short RePlan-on-RePlan operating cycle after
the post-MVP dogfood plan was approved.

The intent was to stop choosing the next RePlan action from memory or chat and
instead use RePlan's own reports, workload view, dashboard data, tracking
suggestions and proposal loop to choose and execute a real maintenance action.

This was deliberately not a new product feature slice. It was a short
operational cycle to test whether the existing CLI-first MVP can help operate
its own project.

## Inputs

The cycle used the public self-dogfood project:

```text
examples/replan-self
```

The operator commands were:

```bash
go run ./cmd/replan report state --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan workload --project examples/replan-self --week 2026-W31 --format json
go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --format markdown
go run ./cmd/replan report dashboard --project examples/replan-self --week 2026-W31 --output reports/latest --format json
```

The initial state showed:

- 20 tasks;
- 17 accepted tasks;
- 2 active tasks;
- 2 open dependencies;
- 1 open proposal;
- 25 stale remaining-work rows;
- 25 tracking suggestions;
- overload on `resource.ai.codex.local` by 12 agent hours;
- overload on `resource.role.reviewer` by 15 review hours;
- overload on `resource.environment.local-dev` by 1 environment hour;
- overload on `resource.human.maintainer` by 58 hours.

## Decision

The first real dogfood action was:

```text
Clear stale remaining-work rows through the tracking proposal loop.
```

Reason:

- stale remaining work polluted the workload and resource pressure views;
- the tracking loop already knew how to detect the problem;
- the proposal loop already supported reviewable `update_remaining_work`
  candidate files;
- clearing this state would immediately improve the usefulness of the next
  project-state report;
- the action exercised RePlan's own mutation, audit and validation path.

This was a better first action than adding a new feature because the dogfood
plan explicitly required learning from the existing operating loop before
expanding scope.

## Procedure

The cycle followed this sequence:

1. Generate project-state, workload, tracking-suggestion and dashboard reports.
2. Inspect the state instead of selecting work from memory.
3. Write tracking proposal candidates:

   ```bash
   go run ./cmd/replan tracking suggest --project examples/replan-self --week 2026-W31 --write-candidates --actor actor.ai.codex.local --format json
   ```

4. Dry-run one proposal accept to confirm the planned writes.
5. Accept tracking proposals through the normal proposal loop.
6. Run validation after mutation.
7. Fix the collision discovered during repeated proposal acceptance.
8. Accept the remaining tracking proposals.
9. Regenerate project-state, tracking-suggestion and dashboard reports.
10. Record the dogfood task, demands, actuals, evidence, accepted proposals and
    audit trail in the self-dogfood project.

## Bug Found During The Cycle

Repeated proposal acceptance discovered a real mutation-path defect.

The proposal decision audit record id was based on:

```text
timestamp + decision
```

The timestamp was compacted at second precision. Accepting two proposals inside
the same second produced duplicate audit ids. Validation correctly rejected the
project after the duplicate was appended.

This was an important dogfood finding because AI CLI agents commonly execute
fast loops. A path that works for a human clicking slowly can still fail under
agent-speed operation.

Resolution:

- proposal decision audit ids now include the accepted proposal id slug;
- regression coverage accepts two tracking proposals with the same fixed test
  timestamp and asserts that their audit ids are distinct;
- the two audit records already written during the failed local loop were
  corrected to the new id shape;
- validation passes after the full proposal acceptance batch.

The defect is also recorded as `GAP-013` in the known-gaps register with
closed status.

## Final State

After accepting 25 tracking proposals:

- stale remaining-work rows: 0;
- tracking suggestions: 0;
- accepted tracking proposals in this cycle: 25;
- open tracking proposal candidates: 0;
- open proposals overall: 1, the existing capacity proposal
  `proposal.increase-review-capacity.20260720`;
- `resource.ai.codex.local` overload disappeared;
- `resource.human.maintainer` overload fell from 58 hours to 1 hour;
- `resource.role.reviewer` overload remained 15 review hours;
- `resource.environment.local-dev` overload remained 1 environment hour.

The remaining signals are now more useful because they are not dominated by
stale already-completed work.

## Friction Observed

Good:

- `report state` gave a compact operator view of the project;
- `tracking suggest` identified a concrete maintenance action;
- proposal candidates gave an explicit reviewable mutation path;
- validation caught the audit id collision immediately;
- the generated dashboard could be refreshed after the mutation.

Pain:

- accepting a batch of proposals required a shell loop over `proposal list`
  output;
- proposal review lacks a focused bulk command for one safe proposal type;
- the initial dashboard/report state had useful signals, but the operator still
  had to interpret which signal should become the next task;
- the current OpenProject fixture had to be updated by hand to keep source refs
  and smoke counters synchronized.

## Next Decision Point

The next slice should be selected from the cleaned project state, not from the
pre-cleanup noise.

The visible candidates are:

- review the two open dependencies and decide whether they should remain open,
  be resolved, or become new tasks;
- review the existing capacity proposal for reviewer-role overload;
- improve proposal batch ergonomics if the next operation again requires
  accepting many low-risk proposals;
- improve source-fixture synchronization if OpenProject parity becomes the next
  bottleneck.

No new large product direction is selected by this document. It records the
completed cycle and leaves the next choice to the refreshed project state.
