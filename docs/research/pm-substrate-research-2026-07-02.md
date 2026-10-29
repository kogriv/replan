# PM substrate research refresh — 2026-07-02

## Trigger

Operator asked to re-check the earlier OpenProject / Jira / MS Project analogue
research in the context of `orcode`.

Updated requirement framing:

- small team: operator + 2 human developers;
- future AI actors should be able to work with the project-management system
  natively via CLI, MCP, API, or webhooks;
- MS Project-like planning matters, especially resource planning, workload,
  Gantt, dependencies, and visibility of who is allocated where;
- self-hosting and data control are strong positives, but should not hide weak
  workflow fit.

This is not a generic "best Jira alternative" question. It is a substrate
question: which tool can be the operational coordination layer for humans and
AI actors without losing serious project/resource planning.

## Short conclusion

No radically new perfect product was found. The previous core shortlist remains
valid:

- OpenProject;
- Plane;
- YouTrack;
- Jira / Atlassian;
- GitLab/Gitea/Forgejo Issues;
- Linear-like SaaS tools.

The research did change the weighting:

1. **OpenProject becomes the primary reference/pilot** because resource
   planning and MS Project-like structure are now explicit first-class
   requirements.
2. **Plane remains the strongest agent-native challenger**, not the PM baseline.
   It has modern REST API, webhooks, MCP server, and agent-oriented developer
   docs, but weaker resource planning.
3. **YouTrack becomes a serious dev-first challenger** because it now has a
   remote MCP server and interactive Gantt, but it is still not the strongest
   MS Project replacement.
4. **Jira is technically capable and agent-friendly**, especially with
   Atlassian Remote MCP and Advanced Roadmaps, but its cloud/Data Center model
   and administrative weight are likely too much for a 3-person indie team.
5. **Smartsheet / Monday / ClickUp / MS Project Planner Premium** are useful
   references for modern resource/workload/baseline UX, but they are weaker
   fits for self-hosted orcode-style agent substrate.

Follow-up decision after checking OpenProject Community vs Enterprise boundaries:

- OpenProject Community is still useful as a task/project substrate.
- The closest OpenProject features to the operator's MS Project expectation
  (Team Planner, baseline comparison, MCP Server) are Enterprise add-ons.
- OpenProject's own resource-management documentation says automated detailed
  resource/capacity calculation is not built in; resource planning is a
  configured-view workaround.
- Therefore the recommended path is not to pay Enterprise or fork first, but to
  build a narrow external **resource-planner-lite** layer. See
  [`resource-planner-lite-2026-07-02.md`](resource-planner-lite-2026-07-02.md).

## Ranking for our use case

| Rank | Tool | Best role | Why | Main risk |
|------|------|-----------|-----|-----------|
| 1 | OpenProject | PM/resource-planning core | Strong Gantt, work packages, dependencies, team planner, resource-management use case, API, webhooks, GitHub integration, MCP server in Enterprise | Heavier Rails stack; deeper MCP is Enterprise; resource leveling is not full MS Project |
| 2 | Plane | Agent-native challenger | Self-host, modern UX, REST API, webhooks, official MCP server, agent-oriented docs | Resource planning is much weaker than OpenProject |
| 3 | YouTrack | Dev-first challenger | Issue tracker, Gantt, REST API, remote MCP, JetBrains ecosystem | Less convincing for formal resource planning |
| 4 | Jira / Atlassian | Enterprise/cloud option | Advanced Roadmaps/capacity, massive API, official Remote MCP, mature integrations | Heavy configuration; self-host story is Data Center, not small-team friendly |
| 5 | GitLab/Gitea/Forgejo Issues | Code-host-native substrate | Excellent git/PR/CI adjacency, APIs, CLIs, webhooks | Too weak for Gantt/resource planning unless augmented |
| 6 | Smartsheet / Monday / ClickUp | SaaS resource-planning reference | Strong workload/baseline/Gantt UX in some cases | SaaS lock-in, weaker self-host/control story, not orcode-native |

## Key findings

### OpenProject

Use as the **reference implementation** for the PM/resource-planning layer.

Relevant capabilities:

- work packages for tasks, features, bugs, risks, etc.;
- Gantt charts with scheduling/dependencies;
- team planner for who is working on what and when;
- resource-management use case;
- REST API v3 and webhooks;
- GitHub PR integration;
- MCP Server introduced in OpenProject 17.2 as an Enterprise add-on.

Interpretation:

OpenProject is not a perfect MS Project clone. It should be treated as a
web-based PM coordination system with strong enough Gantt/resource visibility
for our scale, not as a deep automatic resource-leveling engine.

### Plane

Use as the **agent-native substrate challenger**.

Relevant capabilities:

- self-hosted project/issue management;
- work items, cycles, modules, views, dashboards;
- REST API, webhooks, OAuth;
- official MCP server exposing Plane's API surface to AI tools;
- documentation explicitly discusses agents.

Interpretation:

Plane may be the better long-term substrate if AI actors become the dominant
workflow and resource planning can be simplified. It should not be selected as
the PM reference if MS Project-style resource planning remains important.

### YouTrack

Use as the **developer-friendly middle option**.

Relevant capabilities:

- issue tracking, Gantt, estimates, dependencies;
- REST API;
- remote MCP server since YouTrack 2025.3;
- custom MCP tools through YouTrack apps/workflows.

Interpretation:

YouTrack is worth testing if OpenProject feels too enterprise-heavy and Plane
feels too weak on planning. It is likely a better dev tracker than a full
resource-planning system.

### Jira / Atlassian

Use as the **enterprise benchmark**, not the default.

Relevant capabilities:

- Jira Plans / Advanced Roadmaps capacity planning;
- Jira REST API, Agile API, webhooks;
- official Atlassian Remote MCP Server for Jira, Confluence, Bitbucket, etc.

Interpretation:

Technically strong, but likely too much platform weight for the current team.
Adopt only if external constraints or team growth justify Atlassian overhead.

### GitLab / Gitea / Forgejo Issues

Use as the **code-host-native fallback**, not the PM reference.

Relevant capabilities:

- great adjacency to git, PR/MR, CI, webhooks, CLIs;
- good agent integration surface.

Interpretation:

These are attractive for simple actor-substrate mechanics, but weak for
resource planning. They can be integrated with OpenProject rather than replace
it.

## Recommended stack direction

Use a two-layer stack:

1. **OpenProject** as the operational PM/resource-planning layer.
2. **Git host** as the development execution layer.
3. **AI actors** integrate with both:
   - OpenProject via MCP/API/webhooks;
   - git host via CLI/API/MCP;
   - evidence and preview links written back to OpenProject work packages.

This keeps the hard requirement (resource planning) in the correct tool, while
preserving agent-native execution around git and tickets.

2026-07-02 correction:

Because OpenProject resource planning is limited in Community and expensive in
Enterprise for a tiny team, the practical stack becomes a three-layer stack:

1. **OpenProject Community or Plane** as task identity/status substrate.
2. **resource-planner-lite** as resource/capacity/scheduling analysis layer.
3. **Git host** as development execution layer.

Obsidian remains the knowledge/review layer, not the operational PM core.

## Pilot plan

Time-box: 1-2 weeks.

1. Deploy OpenProject on a VPS or local test host.
2. Create one real project with:
   - operator;
   - 2 human developers;
   - one placeholder `agent-coder` user/resource.
3. Model 20-30 real work packages:
   - phases / WBS;
   - dependencies;
   - estimates;
   - assignees;
   - start/end dates;
   - custom fields for repo, branch, PR URL, preview URL, evidence.
4. Exercise normal work:
   - planning review;
   - weekly resource view;
   - over-allocation detection by inspection;
   - PR links back to work packages;
   - one scripted/AI update through API or MCP/community MCP.
5. In parallel, run a smaller Plane or YouTrack agent-flow test:
   - create ticket;
   - claim ticket;
   - update status;
   - write comment/evidence;
   - check webhook/MCP friction.

Decision after pilot:

- if resource planning is useful and the UI is tolerable: keep OpenProject as
  core;
- if OpenProject is too heavy but agent flow dominates: test Plane as core and
  accept weaker resource planning or pair it with a separate resource planner;
- if OpenProject is too heavy and Plane is too product-board oriented: test
  YouTrack as dev-first compromise.

## Sources checked

- OpenProject resource management:
  https://www.openproject.org/docs/use-cases/resource-management/
- OpenProject team planner:
  https://www.openproject.org/docs/user-guide/team-planner/
- OpenProject API and webhooks:
  https://www.openproject.org/docs/system-admin-guide/api-and-webhooks/
- OpenProject MCP server:
  https://www.openproject.org/docs/system-admin-guide/integrations/mcp-server/
- OpenProject GitHub integration:
  https://www.openproject.org/docs/system-admin-guide/integrations/github-integration/
- Plane developer docs:
  https://developers.plane.so/
- Plane MCP server:
  https://developers.plane.so/dev-tools/mcp-server
- YouTrack REST API:
  https://www.jetbrains.com/help/youtrack/devportal/youtrack-rest-api.html
- YouTrack MCP server:
  https://www.jetbrains.com/help/youtrack/server/model-context-protocol-server.html
- Atlassian Remote MCP:
  https://support.atlassian.com/atlassian-rovo-mcp-server/docs/getting-started-with-the-atlassian-remote-mcp-server/
- Jira capacity planning:
  https://support.atlassian.com/jira-software-cloud/docs/manage-capacity-in-advanced-roadmaps/
