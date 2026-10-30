# RePlan roadmap

RePlan is alpha. This page says what works today and where the project is
going. The task-level plan is RePlan's own project,
[`examples/replan-self`](examples/replan-self): run
`replan next --project examples/replan-self` to see it.

## Works today

- **A first project of your own.** `replan init`, `resource add` and
  `capacity set` start a project with commands only;
  [Your first project](docs/quickstart.md) goes from an empty directory to a
  weekly report, and the smoke test runs it.
- **Model and storage.** Tasks, resources of any kind (people, agents, roles,
  environments, quotas, budgets), multi-resource demands, capacities and
  availability, dependencies and non-scheduling links, all in YAML files that
  live in Git. Writes to one project are serialized, and an interrupted write
  is completed by the next one.
- **Workload.** Weekly load per resource and unit, overloads, missing-data and
  dependency warnings.
- **Scenarios.** What-if overlays for capacity, estimates, assignees and dates,
  with diffs against the current plan and an explicit apply.
- **Proposals and audit.** Suggested changes that a person accepts or rejects;
  under the default policy only a human actor can approve or apply a change;
  an append-only audit log of every write.
- **Tracking.** Actual consumption, remaining work, demand lifecycle and status
  changes, each backed by evidence; a mis-booked actual is corrected by an
  audited record.
- **Baselines.** Sealed snapshots of the approved estimates, and a report of
  how far the actuals drifted from them.
- **Calibrated estimates.** A sealed, human-approved correction factor per
  resource group, computed from what baselined tasks really took, shown next
  to new estimates and applied only through a scenario.
- **Reports.** Weekly (Markdown, JSON, CSV, HTML), project state, next actions
  and a static HTML dashboard with workload, scenario, proposal and estimate
  pages.
- **OpenProject.** Staged read-only import, drift checks, dry-run-first
  write-back of dates, status and titles, and a read-only plugin with a
  resource view and a per-task panel. The helpers also work on a project
  tracked in OpenProject alone: work packages, comments, relations and
  parents by number, with secrets read from files.
- **GitHub Issues.** Read-only import from an exported file or over the API
  into the same staged review as OpenProject.
- **TaskJuggler.** Optional export, run and import of the result as a scenario
  for review.
- **Agent kit.** The CLI, the helpers and the operating rules packaged for an
  outside AI agent.
- **Installation.** Tagged releases with prebuilt archives for Linux, macOS
  and Windows, built and attested in CI from the tag, and `go install`.

## Next

These are directions, not commitments; each becomes a task in the self plan
before work starts.

- **A stable file format.** The project format is versioned and
  `replan project migrate` exists; the format becomes stable once outside
  users have data in it.
- **Calibration by task size.** Small tasks overrun their calibrated estimate
  more than large ones; a factor per size band instead of one per resource
  group.
- **GitHub Projects fields over the API.** Dates and estimates from GitHub
  Projects are read from an exported file today; the live import needs the
  GraphQL API for them.

## Not planned

RePlan will not become a task tracker, a Gantt editor, a wiki, a chat or a
CI system, and it will not sync two systems in both directions automatically.
It stays a planning layer beside the tools a team already uses.

The original phased plan, kept for history, is in
[`docs/roadmap/initial-roadmap.md`](docs/roadmap/initial-roadmap.md).
