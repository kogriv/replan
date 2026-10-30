# RePlan

RePlan (from *REsource PLANning*, read as *re-plan*) is a resource planning
and replanning layer for software teams where part of the work is done by AI
agents.

It sits beside the task tracker you already use. The tracker keeps owning the
tasks; RePlan keeps the resource side of the plan:

- who and what each task needs: people, agents, review time, CI minutes, a
  shared environment, a token quota, a budget;
- how much capacity exists each week, and who is overloaded;
- what happens if someone is unavailable or an estimate changes, without
  touching the real plan;
- how far actual work drifted from the approved estimates;
- which changes an agent may only suggest and a person has to approve.

> **Status: alpha.** The CLI and the file format still change between
> `v0.x` releases. RePlan plans its own
> development: [`examples/replan-self`](examples/replan-self) is the live
> project it is built with, not a demo.

## Why

Issue trackers answer *what* has to be done. They answer poorly *who can do
it, when, and at what cost*. That gap gets wider once agents take a real share
of the work: an agent has capacity and quotas like a person does, but its
estimates, failure modes and trust level are different, and its changes to
the plan should not land without review.

RePlan's answer is a small, explicit model kept in plain YAML files in Git:

- **tasks** come from the tracker, or are written by hand;
- **resources** are people, agents, roles, environments, quotas and budgets,
  each with its own unit and weekly capacity;
- **resource demands** say how much of which resource a task needs, so one task
  can need agent hours, reviewer hours and a staging slot at once;
- **baselines** freeze the approved estimates, sealed with a checksum;
- **actuals** and **evidence** record what was really spent and why we believe it;
- **scenarios** are what-if overlays that are calculated but never applied
  silently;
- **proposals** are suggested changes, often written by an agent, that a person
  accepts or rejects. Every change is written to an audit log.

Nothing changes the project unless the command is given `--apply` (or
`REPLAN_APPLY=1`). Without it, a command only shows what it would do.

## Try it

Prebuilt archives for Linux, macOS and Windows (amd64 and arm64) are on the
[releases page](https://github.com/kogriv/replan/releases), with a
`SHA256SUMS` file. Or install the latest release with Go 1.24 or newer:

```bash
go install github.com/kogriv/replan/cmd/replan@latest
replan --version
```

To run the examples below, work from a clone:

```bash
git clone https://github.com/kogriv/replan.git
cd replan
go install ./cmd/replan        # puts `replan` in $(go env GOPATH)/bin
```

`examples/small-team` is a tiny team: two developers, one AI coding agent, a
reviewer pool, a staging environment, a token quota and a budget.

```bash
replan validate examples/small-team
replan report weekly --project examples/small-team --week 2026-W28 --format markdown
```

What if Ivan is away that week? A scenario answers this without changing the
project:

```bash
replan scenario diff --project examples/small-team --format markdown scenario.ivan-unavailable-week-28
```

```text
## Workload Changes
| Resource | Unit | Capacity Before | Capacity After | Overload Before | Overload After |
|---|---:|---:|---:|---:|---:|
| `Ivan` | hours | 32 | 0 | 0 | 8 |
```

Agents write suggestions, and a person accepts or rejects them; a dry run
shows the effect first. This is a working rule, not a lock: the local CLI
trusts whoever runs it, so an agent that can run `replan` and write the
project files could name a person as `--actor`. See
[what the local CLI trusts](docs/security/access-and-actor-model.md#what-the-local-cli-enforces-today)
for how to keep agents to suggestions in practice.

```bash
replan proposal list --project examples/small-team --status open --format markdown
replan proposal show --project examples/small-team --format markdown proposal.adjust-remaining-task-op-1234.20260706
```

To try a command that writes, copy the example first. The dry run lists the
changes and the files it would write; the apply writes them and an audit
record:

```bash
cp -a examples/small-team /tmp/small-team
replan scenario apply --project /tmp/small-team --dry-run --format markdown \
  scenario.task-op-1234-remaining-5
replan scenario apply --project /tmp/small-team --apply --format markdown \
  --actor actor.human.operator --reason "Review found less work left." \
  scenario.task-op-1234-remaining-5
replan audit list --project /tmp/small-team --format markdown
```

## The self-planning project

`examples/replan-self` holds RePlan's own plan: about 150 tasks with demands
for the maintainer, an executor agent, a Codex coding agent, a reviewer role,
CI minutes and a local environment, plus the evidence and actuals of the work
done so far.

```bash
replan next --project examples/replan-self
replan report estimates --project examples/replan-self --baseline baseline.2026-10-15.tooling-consolidation --format markdown
replan report dashboard --project examples/replan-self --output /tmp/replan-dashboard
```

The dashboard is static HTML: open `/tmp/replan-dashboard/index.html` in a
browser.

The estimate report compares the sealed baseline with what was really spent.
For the 38 tasks of the 2026-10-15 baseline, all finished, the agents were
estimated at 138 agent hours and used about 35: a ratio of 0.26. That is the
kind of answer RePlan exists to give, and the reason the next plan should be
estimated differently.

## Working with a task tracker

RePlan is tracker-agnostic in its model. The integration that exists today is
for [OpenProject](https://www.openproject.org/):

- read-only import of work packages, relations and time entries, staged for
  review before anything is accepted;
- narrow write-back helpers (dates, status, title) that are dry-run by default;
- drift checks between OpenProject and RePlan, where neither side wins
  automatically;
- an OpenProject plugin that shows RePlan's resource view and a per-task panel
  inside OpenProject, read-only.

See [`docs/dev/openproject-helpers.md`](docs/dev/openproject-helpers.md) and the
[agent kit](docs/agent-kit/README.md), which packages the CLI, the helpers and
the operating rules for an outside agent.

[TaskJuggler](https://taskjuggler.org/) can be used as an optional scheduling
engine: RePlan exports a project, runs `tj3`, and turns the result into a
scenario or proposal for review.

## Documentation

- [`docs/dev/getting-started.md`](docs/dev/getting-started.md): local
  commands, the full CLI inventory and example walkthroughs.
- [`docs/spec/cli.md`](docs/spec/cli.md): commands, output formats, exit codes
  and mutation rules.
- [`docs/spec/canonical-model.md`](docs/spec/canonical-model.md): the data
  model and its invariants.
- [`docs/product/positioning.md`](docs/product/positioning.md): who RePlan is
  for, and what it will not become.
- [`ROADMAP.md`](ROADMAP.md): what is done and what comes next.
- [`docs/README.md`](docs/README.md): the index of every design, spec and
  review document.
- [`AGENTS.md`](AGENTS.md): rules for AI agents working in this repository.

## License

RePlan is licensed under the [Apache License, Version 2.0](LICENSE); see
[NOTICE](NOTICE). The OpenProject plugins in `integrations/openproject` are
licensed the same way; OpenProject itself is GPLv3, and Apache-2.0 code may be
combined with it.

Contributions are welcome; read [CONTRIBUTING.md](CONTRIBUTING.md) before
starting. Report security vulnerabilities through the private process in
[SECURITY.md](SECURITY.md), not through a public issue.
