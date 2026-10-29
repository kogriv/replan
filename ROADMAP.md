# RePlan roadmap

RePlan is alpha. This page says what works today and where the project is
going. The task-level plan is RePlan's own project,
[`examples/replan-self`](examples/replan-self): run
`replan next --project examples/replan-self` to see it.

## Works today

- **Model and storage.** Tasks, resources of any kind (people, agents, roles,
  environments, quotas, budgets), multi-resource demands, capacities and
  availability, dependencies, all in YAML files that live in Git.
- **Workload.** Weekly load per resource and unit, overloads, missing-data and
  dependency warnings.
- **Scenarios.** What-if overlays for capacity, estimates, assignees and dates,
  with diffs against the current plan and an explicit apply.
- **Proposals and audit.** Suggested changes that a person accepts or rejects;
  an append-only audit log of every write.
- **Tracking.** Actual consumption, remaining work, demand lifecycle and status
  changes, each backed by evidence.
- **Baselines.** Sealed snapshots of the approved estimates, and a report of
  how far the actuals drifted from them.
- **Reports.** Weekly (Markdown, JSON, CSV, HTML), project state, next actions
  and a static HTML dashboard.
- **OpenProject.** Staged read-only import, drift checks, dry-run-first
  write-back of dates, status and titles, and a read-only plugin with a
  resource view and a per-task panel.
- **TaskJuggler.** Optional export, run and import of the result as a scenario
  for review.
- **Agent kit.** The CLI, the helpers and the operating rules packaged for an
  outside AI agent.
- **Installation.** Tagged releases with prebuilt archives for Linux, macOS
  and Windows, and `go install`.

## Next

These are directions, not commitments; each becomes a task in the self plan
before work starts.

- **A stable file format.** The project format is versioned and
  `replan project migrate` exists; the format becomes stable once outside
  users have data in it.
- **Calibrated estimates.** Feed the estimate accuracy report back into
  planning, so a plan made by an agent starts from what similar tasks really
  took.
- **More task trackers.** A read-only GitHub Issues adapter, from an exported
  file first and then over the API, to prove the model is not shaped around
  OpenProject.
- **More dashboard pages.** The weekly heatmap, scenario comparison, proposal
  review and estimate accuracy as read-only pages of the static dashboard.
  A served web application is not planned until these show it is needed.

## Not planned

RePlan will not become a task tracker, a Gantt editor, a wiki, a chat or a
CI system, and it will not sync two systems in both directions automatically.
It stays a planning layer beside the tools a team already uses.

The original phased plan, kept for history, is in
[`docs/roadmap/initial-roadmap.md`](docs/roadmap/initial-roadmap.md).
