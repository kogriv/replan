# Market gap and alternatives, 2026-07-02

Status: research note.

Purpose:

Document whether RePlan is solving a real gap or merely rebuilding existing
resource-management products.

Short answer:

Resource planning is not an empty market. Many products exist. The possible
RePlan gap is narrower:

```text
FOSS-first, self-hostable, task-source-agnostic resource/capacity planning
layer for software teams and AI agents
```

RePlan should not claim that "nobody does resource planning". That would be
false.

RePlan may claim that there is still a weakly served niche at the intersection
of:

- open-source or source-available workflows;
- self-hosting;
- low-cost small teams;
- existing task systems such as OpenProject, Plane, GitHub and GitLab;
- TaskJuggler-compatible advanced scheduling;
- CLI/API-first operation;
- AI agents as both schedulable resources and operational actors.

## Pricing note

Prices below are approximate public prices observed on 2026-07-02.

They are not contractual quotes.

Before making a buy/build decision, verify the vendor pricing pages again.

## Summary table

| Tool | Type | Delivery | Rough public price | Strong at | Why it does not remove RePlan gap |
| --- | --- | --- | --- | --- | --- |
| Runn | Dedicated resource management and capacity planning | SaaS | Lite $7/resource/month, Standard $11/resource/month, Advanced custom | Capacity, availability, utilization, scenarios, scheduling, financials, integrations | Good product category proof, but SaaS/commercial, not FOSS/self-hosted, not a source-agnostic FOSS layer for task systems and agents |
| Float | Resource scheduling and planning | SaaS | Starter $7/scheduled person/month; higher tiers for advanced features | Visual team schedule, time off, capacity, project scoping, margins | Commercial SaaS; not FOSS/self-hosted; not designed as an adapter layer around OpenProject/Plane/TaskJuggler |
| Resource Guru | Resource scheduling and availability | SaaS | Public page says from $4.16/person/month; tiers reported around $4.16-$10/person/month | Simple scheduling, availability, non-human resources, calendar-style planning | Commercial SaaS; charges for people/resources; not FOSS; not agent-native planning substrate |
| Tempo Capacity Planner | Jira capacity/resource planning add-on | Atlassian Marketplace / Jira ecosystem | Marketplace/Capterra report cloud pricing around $10/month for up to 10 users and per-user pricing beyond that | Jira-native capacity planning, team planning, availability, workload | Tied to Jira ecosystem; commercial add-on; not FOSS/self-hosted resource layer for multiple task sources |
| Forecast | Project/resource forecasting and professional-services planning | SaaS | Vendor pricing/custom or plan-dependent | Forecasting, project planning, utilization, financials | Commercial SaaS; broader PSA/product suite; not FOSS/source-agnostic layer |
| Teamdeck | Resource scheduling, time tracking and availability | SaaS | G2 reports $2/month light member, $6/month full member | Scheduling, availability, vacations, timesheets, reports | Affordable and useful, but SaaS/commercial; not FOSS/self-hosted; not designed for AI-agent write-back/control |
| eResource Scheduler | Enterprise resource scheduling and capacity planning | SaaS / vendor product | Public listings report module pricing from about $5/resource/month; add-on modules | Capacity, scheduling, timesheets, reports, financials, access controls | Mature commercial product; broad enterprise scheduling; not FOSS-first and likely heavier than small software-team needs |
| Scoro | Work management / professional services suite | SaaS | Public pricing: Core around $19.90/user/month, Performance around $49.90/user/month; Enterprise custom | Full delivery cycle: projects, Gantt, budgets, quotes, invoices, resource planning, profitability | Much broader paid business suite; resource planning is part of PSA/operations platform, not narrow FOSS layer |
| Quollnet Resource Planner | Lightweight open-source resource planner | Web app / GitHub | Free/open-source | Simple scheduling, allocation, KPI monitoring | Useful signal that FOSS attempts exist, but appears lightweight; not positioned as multi-source software-team/AI-agent planning substrate |
| TaskJuggler | FOSS scheduling/resource engine | CLI / project-as-code | Free/open-source | Automatic resource leveling, conflict resolution, scenarios/baselines, working hours, leave, costs, reports | Strong engine, but not day-to-day team task substrate, not source adapter/write-back layer, not modern approval/agent interface |
| ProjectLibre | MS Project-like project planning | Desktop FOSS plus commercial cloud | Desktop free/open-source; cloud commercial | Gantt, WBS, resource/task/cost management, MS Project replacement style | Good desktop/reference tool; not collaborative adapter layer; not AI-agent/source-system workflow |
| GanttProject | Free desktop Gantt/project planning | Desktop | Free | Gantt, resource load chart, PERT, baselines, import/export | Useful small desktop planner; not multi-user source-of-truth, not task-system adapter layer, not agent-native |

## Detailed notes

### Runn

Runn is direct evidence that the resource-management pain is real.

Runn positions itself as resource management and capacity planning software for
teams planning people across technical projects.

Public features include:

- capacity planning;
- scheduling;
- project setup;
- forecasting;
- financials;
- reporting;
- API and integrations.

Public pricing page observed on 2026-07-02:

- Lite: $7 per resource seat per month;
- Standard: $11 per resource seat per month;
- Advanced: custom pricing;
- Standard includes API/custom integrations and 30-day activity log;
- Advanced includes SSO/SAML, unlimited custom fields/reports, custom
  dashboards and support options.

Sources:

- https://www.runn.io/
- https://www.runn.io/pricing

RePlan implication:

Runn proves the category. It does not remove the RePlan opportunity because it
is commercial SaaS, not FOSS-first self-hostable infrastructure. RePlan should
not try to out-polish Runn as a full SaaS UI. RePlan should instead be the
open, adapter-based, CLI/API-friendly layer around existing task systems.

### Float

Float is a popular resource management, scheduling and planning product.

Public positioning:

- visual schedule;
- who is working on what;
- delivery tracking;
- capacity and time off;
- project scoping;
- margins and financial visibility in higher tiers.

Public pricing observed on 2026-07-02:

- Starter: $7 per scheduled person per month;
- higher tiers add advanced resourcing, actuals tracking and financial/reporting
  features.

Sources:

- https://www.float.com/
- https://www.float.com/pricing

RePlan implication:

Float is strong in visual resource scheduling. It is not a FOSS/self-hosted
planning substrate. It does not solve the need for source-agnostic import,
TaskJuggler compatibility, local CLI workflow, or AI-agent-safe operations.

### Resource Guru

Resource Guru focuses on resource scheduling, availability and team calendars.

Public positioning:

- capacity made obvious;
- flexible planning;
- deadline feasibility;
- people and non-human resources;
- calendar/workflow integrations;
- open API mentioned in third-party reviews.

Public pricing observed on 2026-07-02:

- official pricing page says from $4.16 per person;
- current review pages report tiers around $4.16, $6.65 and $10 per
  person/month;
- official help says pricing is based on people and other resources in the
  account; non-human resources can also be billed.

Sources:

- https://resourceguruapp.com/
- https://resourceguruapp.com/pricing
- https://help.resourceguruapp.com/en/articles/1959234

RePlan implication:

Resource Guru is useful evidence for planning people and non-human resources.
However, it is still a commercial SaaS product, not a FOSS source-agnostic
planning layer with agent-safe CLI/API and self-hosted operation.

### Tempo Capacity Planner

Tempo Capacity Planner is important because it demonstrates how resource
planning is often monetized as a paid add-on to an existing task ecosystem.

Public positioning:

- Jira team/resource management;
- capacity planning;
- project planning;
- resource and time tracking;
- two-way sync with Jira;
- scheduling teams and individuals on project/task/ticket level.

Public pricing observed on 2026-07-02:

- Atlassian Marketplace provides pricing flow and trial;
- Capterra reports starting at $10/month for up to 10 users on Cloud or about
  $3.99/user/month for 11-100 users.

Sources:

- https://marketplace.atlassian.com/apps/1211881/capacity-planner-jira-team-resource-management-tempo
- https://www.tempo.io/products/capacity-planner
- https://www.capterra.com/p/190516/Tempo-Planner/

RePlan implication:

Tempo is strong if the team is already committed to Jira. RePlan's proposed
space is different: not Jira-only, not paid add-on first, and designed for
OpenProject/Plane/GitHub/GitLab/YAML plus AI actor workflows.

### Forecast

Forecast and similar professional-services products cover resource planning,
forecasting, utilization and project delivery management.

Typical strengths:

- project planning;
- utilization;
- forecasting;
- capacity visibility;
- business/financial planning;
- professional-services workflows.

Pricing is commonly vendor-specific or tiered SaaS.

RePlan implication:

These tools are part of the commercial PSA/resource-planning world. They prove
the business value of resource planning, but they are usually broader, heavier
and not FOSS/self-hosted.

### Teamdeck

Teamdeck positions around resource planning, team calendars, workload,
availability, timesheets and reporting.

Public pricing observed on 2026-07-02 through G2:

- Light Member: $2/month, or $1.80 annually;
- Full Member: $6/month, or $5.40 annually.

Sources:

- https://teamdeck.io/
- https://www.g2.com/products/teamdeck/pricing

RePlan implication:

Teamdeck looks affordable and practical. It still does not remove the RePlan
gap because it is not FOSS-first, not self-hosted, and not designed around
source adapters and AI-agent planning boundaries.

### eResource Scheduler

eResource Scheduler is a mature resource-management/scheduling product.

Public positioning:

- real-time availability;
- resource scheduling;
- timesheets;
- financials;
- management reports;
- capacity planning;
- Gantt-like scheduling;
- people, teams, locations and departments;
- integrations such as Jira, Outlook, Trello and Slack in public listings.

Public pricing observed on 2026-07-02:

- public listings report module pricing starting around $5 per resource/month;
- additional modules such as financial/authentication add-ons may be priced
  separately;
- vendor pages emphasize custom/demo flows.

Sources:

- https://www.eresourcescheduler.com/
- https://www.getapp.com/operations-management-software/a/eresource-scheduler/
- https://www.eresourcescheduler.com/blog/best-resource-management-software-reviewed

RePlan implication:

This is much closer to enterprise resource scheduling. It reinforces that
access control, modules and reporting can become heavy. RePlan should avoid
this scope initially.

### Scoro

Scoro is a broader work-management/professional-services platform.

Public positioning:

- projects;
- Gantt and dependencies;
- calendars;
- quotes;
- invoices;
- dashboards;
- resource planning;
- capacity forecasting;
- profitability and financials.

Public pricing observed on 2026-07-02:

- Core around $19.90/user/month on the public pricing page;
- Performance around $49.90/user/month;
- Enterprise custom;
- resource/capacity planning is part of a broader paid platform.

Sources:

- https://www.scoro.com/pricing/
- https://www.scoro.com/features/planning-resources/

RePlan implication:

Scoro is not a narrow resource-planning sidecar. It is a broad commercial
business operations suite. RePlan should not compete with this full-stack
offering.

### Quollnet Resource Planner

Quollnet Resource Planner is useful because it shows that FOSS/open-source
resource-planner attempts exist.

Public positioning:

- free/open-source resource planning software;
- scheduling;
- allocation;
- workload/capacity planning;
- KPI monitoring;
- lightweight HTML/JavaScript/Bootstrap web application.

Sources:

- https://quollnet.com/article/resource-planner-open-source-app
- https://github.com/quollnet/resource-planner
- https://www.quollnet.com/resource-planner

RePlan implication:

This is the closest "FOSS resource planner exists" counterexample in the quick
survey. It does not appear to target the same deeper system shape:

- source adapters for OpenProject/Plane/GitHub/GitLab;
- TaskJuggler-compatible scheduling;
- CLI/API-first operation;
- AI-agent trust model;
- safe write-back;
- RePlan as prerequisite infrastructure for OrCode-style development.

Still, RePlan should study it before implementing UI-level scheduling.

Follow-up completed:

- deeper code-level review added in
  [`quollnet-resource-planner-deep-dive-2026-07-02.md`](quollnet-resource-planner-deep-dive-2026-07-02.md).
- conclusion: do not use Quollnet as RePlan core; use it as UI/reference and
  possible future import format.

### TaskJuggler

TaskJuggler is the most important FOSS technical reference.

Public positioning:

- open-source project management tool;
- tasks;
- resources;
- accounts;
- project scoping;
- resource assignment;
- cost and revenue planning;
- risk and communication management;
- status tracking;
- reporting.

Important advanced scheduling features:

- automatic resource leveling;
- task conflict resolution;
- unlimited scenarios/baselines for what-if analysis;
- flexible working hours and leave management;
- shift work support;
- multiple time zone support.

Sources:

- https://taskjuggler.org/tj3/manual/Intro.html
- https://taskjuggler.org/download.html
- https://github.com/taskjuggler/TaskJuggler

RePlan implication:

TaskJuggler should not be ignored or rebuilt first. It should be treated as:

- advanced scheduling engine;
- parity benchmark;
- export/import target;
- proof that FOSS resource scheduling can be serious.

But TaskJuggler does not remove the RePlan gap because it is not the product
layer around existing task systems, write-back approvals, AI agents, evidence
links and weekly operator workflows.

### ProjectLibre

ProjectLibre is a desktop MS Project-style open-source tool with a commercial
cloud direction.

Public positioning:

- ProjectLibre Desktop as free/open-source project management software;
- positioned as a Microsoft Project alternative;
- Gantt chart;
- network diagram;
- work breakdown structure;
- task/resource/cost management;
- import/export compatibility in public descriptions.

Sources:

- https://projectlibre.com/
- https://sourceforge.net/projects/projectlibre/

RePlan implication:

ProjectLibre is a strong reference for traditional project planning. It does
not solve RePlan's intended workflow because it is not primarily a
self-hosted/team/adapter/agent layer. It is useful as a conceptual and import
reference, not as the operating substrate.

### GanttProject

GanttProject is a free desktop project-management app.

Public positioning:

- free desktop project management software;
- local-first;
- offline;
- Windows/macOS/Linux;
- Gantt chart;
- resource load chart;
- PERT chart;
- project baselines;
- import/export.

Sources:

- https://ganttproject.biz/
- https://www.capterra.com/p/136586/GanttProject/
- https://sourceforge.net/projects/ganttproject/

RePlan implication:

GanttProject is useful for simple local project schedules. It does not solve
collaborative source-of-truth, task-system adapters, AI-agent control, or
write-back workflows.

## Why this market gap can exist

It is reasonable to ask:

```text
If the gap is real, why did stronger teams not already solve it?
```

Possible reasons:

### Resource planning is workflow-specific

Resource planning looks generic, but real teams quickly disagree about:

- whether planning is by person or role;
- whether estimates are hours, points or dates;
- whether weekends count;
- how to model partial availability;
- how much to trust estimates;
- whether project deadlines are hard or soft;
- whether to optimize for utilization, delivery date, cost, focus time or risk.

This makes universal products either broad and expensive or narrow and
opinionated.

### Money is in SaaS and enterprise

The market has willingness to pay. Vendors tend to monetize through:

- per-seat SaaS;
- per-resource SaaS;
- enterprise plans;
- Jira/Atlassian add-ons;
- professional-services suites;
- implementation and support.

That makes a FOSS-first small-team tool less attractive to typical venture or
enterprise vendors.

### Open-source PM projects usually start elsewhere

FOSS PM tools often begin with:

- issues;
- tasks;
- Kanban;
- Gantt;
- wiki;
- time tracking.

Deep capacity planning, resource leveling, approvals and role-specific access
come later, if ever, because they are harder and less universally configured.

### Scheduling engines are hard

Advanced scheduling is mathematically and operationally difficult.

TaskJuggler already handles many of these hard parts. A new project should not
start by rewriting that engine. It should first make scheduling outputs usable
inside real task-system workflows.

### AI-agent capacity planning is new

Most resource tools assume people.

The AI-agent dimension changes the model:

- agents are schedulable resources;
- agents are also actors calling APIs;
- trust levels differ;
- evidence matters;
- approval gates matter;
- agent-generated proposals need policy;
- multiple agent tools may coexist.

This is a newer need. It is plausible that existing tools have not yet centered
it.

## RePlan gap statement

The defensible gap is not:

```text
There are no resource planning tools.
```

The defensible gap is:

```text
There is no obvious FOSS-first, self-hostable, source-agnostic resource
planning layer that uses existing task systems as inputs, can delegate advanced
scheduling to TaskJuggler, exposes safe CLI/API workflows for AI agents, and
keeps core resource planning free.
```

## RePlan should not compete head-on with these products

RePlan should avoid trying to beat Runn, Float, Resource Guru, Scoro or
eResource Scheduler at polished SaaS scheduling UI.

RePlan should avoid trying to beat TaskJuggler at scheduling algorithms.

RePlan should avoid trying to beat OpenProject, Plane, Jira, GitHub or GitLab at
task tracking.

RePlan should instead own:

- canonical resource/capacity model;
- source adapters;
- workload reports;
- scenario comparison;
- TaskJuggler bridge;
- safe write-back;
- agent-aware access model;
- local/self-hosted FOSS workflow.

## Buy/build implications

For a larger company with budget and compliance needs:

- buy Runn, Float, Resource Guru, Tempo, Scoro, eResource Scheduler, Jira
  ecosystem tools or equivalent;
- RePlan will not beat full commercial products on support, UI polish, SSO,
  procurement, reporting breadth or vendor accountability.

For a small software team using self-hosted tools:

- RePlan may be worth building if it stays narrow;
- it should start with CLI/local reports, YAML/OpenProject import and
  TaskJuggler export;
- it should not start with enterprise auth, billing, full web app or multi-tenant
  SaaS.

For OrCode:

- RePlan is valuable if it proves the task/resource/agent planning substrate
  needed before OrCode resumes implementation.

## Watchlist

Before implementation and before any public positioning, re-check:

- Runn API/integrations and whether self-hosting appears;
- Float API and scenario planning depth;
- Resource Guru API and non-human resource model;
- Tempo Capacity Planner behavior in Jira;
- Quollnet Resource Planner code and license;
- TaskJuggler current maintenance status and output formats;
- OpenProject Community/Enterprise changes around Team Planner, baselines and
  MCP;
- Plane MCP/API and planning features;
- GitHub/GitLab project-field APIs.

## Current conclusion

Do not abandon RePlan because resource-management products exist.

Do not overclaim uniqueness.

Proceed only if RePlan remains a narrow FOSS-first layer:

```text
task sources -> canonical planning model -> workload/scenarios ->
optional TaskJuggler -> safe reports/write-back -> AI-aware controls
```

If RePlan drifts toward a polished commercial resource-management SaaS, stop.
That market already has strong products.
