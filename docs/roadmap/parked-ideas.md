# Parked Ideas

Ideas worth remembering that are not agreed work. Nothing here is a task.
When the maintainer agrees to one, it becomes a RePlan task and an
OpenProject work package, and the entry here points to it.

## Difficulty features for estimates (DARE)

Source: DARE, "difficulty-aware" evaluation of coding agents
(arXiv 2611.05797, November 2026). The paper predicts how hard an issue is
for coding agents from features of the issue text, the repository, the
needed change and its tests, using an interpretable model. The largest
contributions were the size and shape of the change and how hard the fix is
to verify. It then picks extra help for the agent (finding the code, checking
related changes, checking tests) from the strongest factor.

Limits noted at review: the strongest features come from the reference fix,
which does not exist before work starts; the gain from picking help was shown
on nine tasks with one agent setup and no comparison with random or all-help
choices.

What RePlan could take from it:

- Record two or three difficulty labels at planning time (for example how
  many places the change touches and how hard it is to verify) and check
  them against the actual-to-estimate ratio in estimate calibration.
- Treat the number of review rounds an executor agent needed as an observed
  difficulty measure next to actual hours.
- Use the strongest expected difficulty to shape the brief for an executor
  agent: where to look, which related writers to check, which tests to add.

Status: parked on 2026-11-06; not planned.
