# Next product slice decision 2026-09-24

Status: selected from RePlan reports; confirmed by the operator on 2026-09-25.

Task: `task.self.select-next-product-slice-from-replan-reports`.

Report snapshot:
[`../../examples/replan-self/evidence/files/state-report-2026-09-24.md`](../../examples/replan-self/evidence/files/state-report-2026-09-24.md),
generated with `replan report state --project examples/replan-self` after
the post-pause stabilization block.

## What this proves

The open part of the full dogfood cycle was choosing the next work from
RePlan outputs instead of from chat. The choice below follows the decision
table in [`../roadmap/post-mvp-dogfood-plan.md`](../roadmap/post-mvp-dogfood-plan.md)
and cites only what the reports show.

## What the reports show

```text
report state, default week, status date 2026-09-24
  week shown:            2026-W30   (July)
  tasks:                 67, accepted 63, active 2
  open dependencies:     0 after the last proposal was accepted
  open proposals:        0
  warnings / errors:     0 / 0
  next actions:          3, all review_tracking on overruns already acknowledged

workload 2026-W39, W40, W41
  overloaded resources:  0 in each week

tracking suggest:        0 suggestions
check-openproject-drift: 67/67 tasks, 93/93 relations, 1 expected soft drift
```

Items that actually wait on a person, and that the report does not surface:

- live acceptance of `task.self.deploy-openproject-wiki-api-plugin-live`;
- the operator's vote on the upstream request, remaining on
  `task.self.check-upstream-openproject-wiki-api-roadmap`;
- confirmation of this decision.

## Reading against the decision table

| Observed pain | Evidence in the snapshot | Verdict |
| --- | --- | --- |
| Operator cannot quickly tell what to do next | The default week is two months stale because it comes from the July baseline, not the status date. Every next action is noise. The three items waiting on the operator are missing. | Present, strongest |
| Resource pressure is hard to inspect | No overload in three weeks; schedule and calibration checks aligned | Absent |
| CLI agents pass too much data around manually | This block ran on existing commands; the new lifecycle commands closed the manual edits | Weak |
| Proposal review is too noisy | 0 open proposals | Absent |
| Dashboard regressions become risky | `WriteProjectStateMarkdown` has cognitive complexity 143 and no golden test (GAP-005, lint baseline) | Present, secondary |
| OpenProject/RePlan comparison is the bottleneck | Live sync of 13 tasks and 16 relations took hours with guarded helpers | Absent |
| Public release confidence is the bottleneck | Not measured by these reports; release story still gated by OSS packaging | Not the next slice |

## Decision

Next slice: make `report state` answer "what should the operator do next"
correctly. `task.self.improve-state-report-next-actions`:

1. the default week follows the project status date, not the baseline's;
2. next actions skip overruns that are acknowledged;
3. next actions list what waits on a person: `review_ready` tasks,
   `in_progress` tasks whose only remaining demand is human, and tasks past
   their planned finish;
4. next actions are ranked by who can act and by date;
5. a golden test pins the state Markdown before the function is changed,
   which covers the GAP-005 risk the lint baseline points at.

Not chosen: new resource-planning features, because nothing in the reports
asks for them; public release work, which remains gated.

## Confirmation

The reports were read through the CLI, the dashboard artifact and the live
overlay. The OpenProject Gantt and the RePlan panels were not looked at in a
browser. The operator confirms the choice after that look, or picks another
row of the table. Until then the task stays `in_progress`.

Confirmed on 2026-09-25, after the operator's own OpenProject user was added
to the dogfood project. The operator confirmed the order: the wiki page delete
route first (GitHub issue 2), then this slice.

## Result

Built on 2026-09-25 in `task.self.improve-state-report-next-actions`. The new
report, run on the same snapshot the decision was made from
([backtest](../../examples/replan-self/evidence/files/state-report-backtest-2026-09-24.md)):

```text
                     old report                    new report
week shown           2026-W30 (July)               2026-W39 (status date)
next actions         3 x review_tracking on        waiting on a person:
                     overruns already                upstream vote (human work)
                     acknowledged                    selection confirmation (human work)
                                                     wiki plugin acceptance (review)
items waiting on     none                          all three the decision listed
a person
```

The contract is in [`../spec/cli.md`](../spec/cli.md), project-state report
slice.
