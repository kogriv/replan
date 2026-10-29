# RePlan Agent Instructions

This repository is dogfooding RePlan with OpenProject as the human project
management surface and RePlan as the resource-planning sidecar.

## Backlog Rule

Do not leave new agreed work only in chat, prose roadmap text or local notes.

For every non-trivial agreed next step, create or update the backlog in the
OpenProject/RePlan dogfood loop:

1. OpenProject owns the human-facing task or work package.
2. RePlan owns resource demands, capacities, scenarios, warnings, proposals,
   actuals and audit evidence around that task.
3. Documentation explains decisions and rationale, but documentation is not the
   task source of truth once a task is agreed for execution.

If live OpenProject access is unavailable, record the task in RePlan/docs with
an explicit `openproject_sync_pending` note and sync it when access returns.

## Current Operating Mode

- OpenProject is primary for task identity, source status, relations, Gantt and
  human task views.
- RePlan drafts planned dates from capacity and load. Reviewed helpers push
  them to OpenProject for people to see; neither side wins drift automatically.
- RePlan is primary for resource planning, overload analysis, scenarios,
  tracking status, execution evidence, actuals, proposals and AI-safe workflows.
- Broad automatic bidirectional sync is out of scope.
- Narrow OpenProject write-back is allowed only through reviewed helpers with
  dry-run by default and explicit apply gates.
- Nothing writes canonical RePlan state without `--apply` or
  `REPLAN_APPLY=1`; use `--dry-run` for an explicit preview.

The full date and status direction rule, plus the command for every operator
loop step, is in the agent kit's
[`Date and status direction`](docs/agent-kit/README.md#date-and-status-direction)
and [`Surface map`](docs/agent-kit/README.md#surface-map). Use that map instead
of inventing a command or treating a planned command as implemented.

## Required Context Before Work

Read these first when starting a new agent session:

- `README.md`
- `docs/dev/getting-started.md` for the current local commands and implemented
  CLI inventory.
- `docs/dev/openproject-design-index.md` before reading or changing OpenProject
  design documents.
- `docs/agent-kit/README.md` before using the OpenProject/RePlan operator loop
  or handing work to an outside agent.
- `docs/dev/backlog-operating-convention.md`
- `docs/spec/openproject-operator-loop.md`
- `docs/roadmap/self-dogfood-planning-calibration.md`
- `docs/roadmap/openproject-replan-sync-plan.md`

For implementation work, also run or inspect:

```sh
git status --short
go test ./...
./scripts/smoke-mvp.sh
```

Use the tests according to risk and scope. For documentation-only changes,
`git diff --check` is usually enough.

## Privacy Boundary

Do not commit live tokens, private OpenProject endpoints, VPS details, real
private work-package ids outside ignored private binding files, or private
pilot narrative.

Live OpenProject bindings belong in ignored storage such as:

```text
private/openproject/replan-self-bindings.yaml
```

Committed examples must use sanitized ids and generic URLs.
