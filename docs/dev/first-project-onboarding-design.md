# First project onboarding design

Date: 2026-10-30

Status: approved on 2026-10-30 (`task.self.design-first-project-onboarding`);
the operator decisions are at the end.

## Purpose

RePlan is public, but a new user can only start a project of their own by
copying `examples/small-team` and editing YAML by hand. This design covers the
path from an empty directory to a first workload report with commands only,
and the guide that walks through it. It is the first direction under Next in
[`ROADMAP.md`](../../ROADMAP.md).

## What the path looks like today

Walked through on 2026-10-30 with the v0.2.0 binary, as a new user would:

1. `replan validate` on an empty directory names ten missing files. Nothing
   creates them.
2. A minimal project that validates needs `.replan/project.yaml`,
   `.replan/policy.yaml` and eight canonical files, seven of them empty
   lists. It was written by hand.
3. There is no command for resources, actors or capacities; they were
   written by hand too.
4. `replan task add` requires `--evidence`, and `replan evidence add` cannot
   target a task that does not exist yet, nor the project. The evidence had
   to target the resource instead.
5. `replan task add` then refused the first task:
   `canonical/current_plan.yaml has no non-empty current_plan.included_tasks
   sequence`. A first task had to be written by hand. `replan import accept`
   does not have this problem: it imports into an empty plan.
6. `replan workload --format text` printed `workload ok` and
   `--format markdown` printed only a status line: the resource table is in
   the JSON output only. `scenario list` and `scenario show` behave the same.
   `replan report weekly` and `replan report dashboard` worked and showed
   the overload.

Steps 4–6 are defects that also hit existing users; steps 1–3 are missing
commands.

## Decisions proposed

### 1. `replan init` creates the smallest valid project

```text
replan init <dir> --name <name> --owner <display name>
    [--id <project-id>] [--timezone <IANA zone>]
    (--dry-run|--apply) [--format json|text]
```

It writes:

- `.replan/project.yaml` with `format_version: 1`, the name, an id derived
  from the directory name (`project.<slug>`) unless `--id` is given, and the
  time zone (the machine's IANA zone when it is known, otherwise `--timezone`
  is required);
- `.replan/policy.yaml`: `local_trusted`, default actor the owner,
  `proposal_only` for AI agents, the same defaults as the example;
- the owner as a human actor with `trust_level: owner` and a matching human
  resource, without capacity;
- every other canonical file as an empty list, and an empty current plan;
- one audit record, `project_init`, by the owner.

It refuses a directory that already holds a `.replan/` directory or any
canonical file (`project_exists`, exit 2). Nothing is written without
`--apply`. There is no interactive wizard: every value is a flag, so an
agent can run it too.

`replan init` stays a top-level command, like `git init`: the
`replan project ...` group works on a project that already exists through
`--project`.

### 2. Commands for resources and capacities

```text
replan resource add --project <path> --id <resource-id> --class <class>
    --name <name> [--capability <name> ...] [--with-actor <trust-level>]
    --actor <id> [--evidence <id>] (--dry-run|--apply) [--format json|text]
replan resource list --project <path> [--format json|markdown|text]
replan capacity set --project <path> --resource <resource-id> --unit <unit>
    --amount <n> [--period week] [--effective-from <YYYY-MM-DD>]
    --actor <id> [--evidence <id>] (--dry-run|--apply) [--format json|text]
```

- `resource add` accepts every resource class of the model (people, AI
  agents, roles, environments, service quotas, budgets). `--with-actor`
  creates the matching actor for a person or an agent, so they can be named
  as `--actor` in later writes.
- `capacity set` writes one periodic capacity. A second call for the same
  resource and unit with a later `--effective-from` adds a new period
  instead of rewriting the old one, so past weeks keep their capacity.
- Both write an audit record. Evidence is optional: setting up resources is
  bookkeeping, not a planning decision.
- Availability windows, resource pools, renaming and removing resources stay
  hand-edited YAML in this version; the guide says so.

### 3. Fix the three defects found on the way

- `replan task add` writes the first task into an empty current plan.
- `replan evidence add` accepts `--target project:<project-id>`, so a
  decision that comes before any task, such as planning the first task, has
  somewhere to point.
- `replan workload`, `scenario list` and `scenario show` print their result
  in `markdown` and `text`, not only the status line.

Fixed on 2026-10-30: all three defects above have regression coverage.

These do not wait for the new commands.

### 4. A quickstart guide that is tested

`docs/quickstart.md`, linked from the README's Try it section: install,
`replan init`, one person and one AI agent with capacities, two tasks with
demands, `replan report weekly` showing an overload, `replan report
dashboard`, then pointers to scenarios, baselines and importing from
OpenProject or GitHub Issues.

The smoke test runs the guide's commands in a temporary directory and checks
that the overload it promises appears, so the guide fails the gate when the
CLI changes under it.

### 5. The agent kit

The kit's README gains the same path in one section, so an outside agent can
start a project for its user. The kit check runs `replan init` on a
temporary directory.

## Not in this version

- an interactive wizard or prompts;
- project templates beyond the empty project;
- commands for availability, resource pools, renaming or removing resources;
- `replan scenario create`: scenarios stay hand-written files;
- creating a project from a tracker in one step: `replan init` followed by
  `replan import openproject|github` and `replan import accept` already does
  it.

## Implementation tasks

Each becomes a self plan task after the operator's decisions; agent hours
are the original estimates.

1. Fix the defects of section 3: about 1.5 agent hours.
2. `replan init` and the resource and capacity commands: about 4 agent
   hours.
3. The quickstart guide, its smoke check and the kit section: about 1.5
   agent hours.

## Operator decisions

Agreed by the operator on 2026-10-30:

1. The command is the top-level `replan init`.
2. Evidence is optional for `resource add` and `capacity set`; the audit
   record is always written.
3. `replan init` gives the owner no capacity; every capacity comes from
   `capacity set`.
4. The first version has no availability, pool, rename or remove commands.
