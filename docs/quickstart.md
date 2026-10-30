# Your first project

From an empty directory to a weekly workload report in a few commands. You
need the `replan` binary on your `PATH`: download it from the
[releases page](https://github.com/kogriv/replan/releases) or run
`go install github.com/kogriv/replan/cmd/replan@latest`.

<!-- The smoke test runs every `sh` block of this page in order, in one shell,
in a temporary directory, and checks that each line of a `text` block that
follows a `sh` block appears in that block's output. Keep them true. -->

Every command that writes takes `--apply`; with `--dry-run` instead it shows
what it would write and writes nothing. Every write is recorded in the
project's audit log.

## 1. Create the project

```sh
replan init website --name "Website relaunch" --owner "Anna Lee" \
  --timezone Europe/Berlin --apply --format text
cd website
```

This makes `website/` a RePlan project: `.replan/` holds the project and
its policy, `canonical/` the plan as YAML files you can keep in Git, and
Anna Lee is the owner, both a person who acts (`actor.human.anna-lee`) and a
resource whose time can be planned (`resource.human.anna-lee`).

## 2. Add an agent and set capacities

An AI coding agent is a resource like a person, with its own unit:

```sh
replan resource add --project . --id resource.ai.coder --class ai_agent \
  --name "Coding agent" --with-actor limited \
  --actor actor.human.anna-lee --apply --format text
replan capacity set --project . --resource resource.human.anna-lee \
  --unit hours --amount 30 --effective-from 2026-11-02 \
  --actor actor.human.anna-lee --apply --format text
replan capacity set --project . --resource resource.ai.coder \
  --unit agent_hours --amount 20 --effective-from 2026-11-02 \
  --actor actor.human.anna-lee --apply --format text
replan resource list --project . --format text
```

```text
resource.ai.coder        ai_agent  Coding agent
resource.human.anna-lee  human     Anna Lee
```

Other resource classes work the same way: `role`, `environment`,
`service_quota`, `budget` and the rest of the model. A later `capacity set`
for the same resource and unit starts a new period, so past weeks keep the
capacity they had.

## 3. Plan two tasks

Each task says what it needs from which resource. Every task records the
evidence it was planned on; the first decision points at the project itself:

```sh
replan evidence add --project . --id evidence.first-plan --type decision \
  --actor actor.human.anna-lee --observed-at 2026-10-30T09:00:00+01:00 \
  --summary "Planned the design and the build of the new pages" \
  --target project:project.website --apply --format text
replan task add --project . --id task.design --title "Design the new pages" \
  --planned-start 2026-11-02 --planned-finish 2026-11-06 \
  --demand work=resource.human.anna-lee:24:hours \
  --actor actor.human.anna-lee --evidence evidence.first-plan \
  --apply --format text
replan task add --project . --id task.build --title "Build the pages" \
  --planned-start 2026-11-03 --planned-finish 2026-11-06 --after task.design \
  --demand review=resource.human.anna-lee:12:hours \
  --demand code=resource.ai.coder:16:agent_hours \
  --actor actor.human.anna-lee --evidence evidence.first-plan \
  --apply --format text
replan validate .
```

## 4. See the load

```sh
replan report weekly --project . --week 2026-W45 --format markdown
```

```text
| `Coding agent` | ai_agent | agent_hours | 20 | 16 | 0 | 16 | 0 |
| `Anna Lee` | human | hours | 30 | 36 | 0 | 36 | 6 |
```

Anna is planned for 36 hours in a 30-hour week: six hours over. The agent
fits. `replan workload --project . --week 2026-W45 --format text` shows the
same rows, and `replan next --project .` lists what needs a person.

The same plan as static HTML pages, with the week's heatmap, a Gantt chart
and the raw JSON:

```sh
replan report dashboard --project . --week 2026-W45 --output ../website-dashboard \
  --format json > /dev/null
ls ../website-dashboard
```

```text
index.html
workload.html
```

## Next

- **What if?** A scenario changes capacity, estimates, assignees or dates in
  a copy and shows the difference: `replan scenario diff`. Scenarios are
  YAML files in `scenarios/`; see the ones in
  [`examples/small-team/scenarios`](../examples/small-team/scenarios).
- **Freeze the estimates** before the work starts with
  `replan baseline create`, then compare what the work really took with
  `replan report estimates`.
- **Record what happened** with `replan actual add-consumption`,
  `set-remaining` and `add-status`.
- **Bring tasks from a tracker** instead of typing them:
  `replan import openproject` or `replan import github`, then
  `replan import accept`. Both import into a project made by `replan init`.
  When the snapshot includes relations, accept its tasks first and then accept
  its dependencies from that same snapshot.
- **Let an agent work on the plan** with the
  [agent kit](agent-kit/README.md); it also covers OpenProject write-back.

The full command list is in [`docs/dev/getting-started.md`](dev/getting-started.md).
Not yet commands: availability windows, resource pools, renaming or
removing a resource; edit their YAML files and run `replan validate`.
