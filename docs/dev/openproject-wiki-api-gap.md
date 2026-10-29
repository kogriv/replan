# OpenProject wiki API gap

Status: dated record (2026-09-21). Index: [OpenProject design index](openproject-design-index.md).

Original status: recorded gap; resolution design accepted on 2026-09-21; gap open until
live acceptance.

Date: 2026-09-21.

Source: GitHub issue #1, "OpenProject wiki is not reachable for an agent:
neither read nor write through the API".

Related task: `task.self.design-openproject-wiki-api-access`.

Resolution design:
[`openproject-wiki-api-plugin-design.md`](openproject-wiki-api-plugin-design.md).

Design review:
[`openproject-wiki-api-plugin-design-review.md`](openproject-wiki-api-plugin-design-review.md).
Product direction accepted. The required amendments from that review were
applied to the design, and the operator accepted the amended design on
2026-09-21. The gap stays open until the plugin passes live acceptance against
the criterion below.

OpenProject sync: pending.

Reason: live OpenProject was unreachable from the working environment on
2026-09-21 (`openproject_network_error`, see
[`project-audit-2026-09-21.md`](project-audit-2026-09-21.md)). The backlog tasks
for this gap are recorded in RePlan canonical data with the
`openproject_sync_pending` tag and must be created and bound in live
OpenProject through the guarded helpers when access returns.

## Purpose

This document records one verified platform gap in OpenProject that blocks the
"humans and AI agents work on the same project surface" thesis:

```text
An API-token participant cannot read or write OpenProject wiki text.
```

It is a gap document, not the solution. It states what was observed, why it
matters, which temporary measure exists and which directions were considered.

It is kept separate from [`known-gaps.md`](known-gaps.md) on purpose. That
register tracks gaps between RePlan code, tests and documentation. This gap is
in the external PM system, and it is tracked by backlog tasks, not by the
hygiene register.

## Where The Gap Surfaced

The gap surfaced in a second private project hosted on the same OpenProject
instance as the RePlan self-dogfood project. That project keeps shared working
knowledge for its participants in the OpenProject wiki, and one of its
participants is an AI agent that authenticates only with an API token.

Project-specific details, page names and work-package identifiers stay outside
this repository. Only the generic platform finding is recorded here.

## Requirement

An agent must be able to read and edit the OpenProject wiki on the same terms
as a human project member:

- list the wiki pages of a project;
- read the full text of any visible page;
- create a page;
- edit an existing page;
- have its edits visible to humans in the browser, and see human edits in
  turn.

The agent authenticates with its own API token. The deliberate operating model
is "token only": the agent has no web login, and that must not change.

## Observed Behavior

Instance version: OpenProject 17.6.0. Observations were made on 2026-09-20 and
2026-09-21 by the instance owner, by reading the application source inside the
running web container and by calling the endpoints. They are recorded in the
issue and are accepted as evidence; they were not re-run for this document.

| Path | Result |
| --- | --- |
| API v3 `GET /api/v3/wiki_pages/{id}` | Returns only `id`, `title` and the project link. The page text is not part of the representation. |
| API v3 page collection | No route exists. Only the `:id` route is defined. |
| API v3 create or update | `POST /api/v3/wiki_pages` returns 404. No `PATCH` route exists. |
| Ordinary web URLs `/projects/{id}/wiki/{slug}` | 302 redirect to `/login`, including `.md`, `.txt` and `.html` variants. `/markdown` returns 404. |
| Built-in key authentication (`key` parameter or `X-OpenProject-API-Key` header) | Works only for controller actions that declare `accept_key_auth`. In 17.6 those are news, work packages, activities, repositories, forums, journals, Gantt and webhooks. The wiki controller is not in that list. |

Conclusion from the issue: wiki text is reachable only through a password
session. The application offers no other path.

Side observations from the issue:

- attachments on a wiki page do work through API v3 in both directions
  (`AttachmentsByWikiPageAPI`, `get` and `post`);
- full text through API v3 is available for work packages, projects, news and
  versions.

## Source Confirmation

The issue's core claim was cross-checked against the public upstream source
tree at tag `v17.6.0` while writing this document. This is a source reading,
not a live call.

`lib/api/v3/wiki_pages/wiki_pages_api.rb` defines only:

```text
resources :wiki_pages
  route_param :id
    get
    mount AttachmentsByWikiPageAPI
```

`lib/api/v3/wiki_pages/wiki_page_representer.rb` declares only:

```text
property :id
property :title
associated_project
```

The same two files were compared at tag `v17.8.0` (latest release on
2026-09-21) and on the upstream `dev` branch. Both are unchanged in this
respect. Upgrading OpenProject therefore does not close the gap.

Upstream is actively working on a bundled `wikis` module. At `v17.6.0` its API
surface covers wiki providers and wiki page links between work packages and
pages of external wiki providers. It does not add text read or write for the
internal wiki. Whether upstream plans to do so is not known from the source
tree and is the subject of the upstream-check backlog task.

## Correction To One Side Claim

The issue states that the documents module has no API. The upstream source at
`v17.6.0` says otherwise: `modules/documents` mounts a `DocumentsAPI` into
`API::V3::Root` with a filtered collection `GET`, a single-resource `GET` with a
formattable `description`, a `PATCH` through `Documents::UpdateService`, and
attachments. There is no `POST`.

Verified on the live instance on 2026-09-24: `GET /api/v3/documents` answers
200 with a collection, empty for the RePlan token's projects. The public API
documentation lists the same surface: list, single `GET` with a formattable
`description`, `PATCH` of title and description, no create. It does not change
the conclusion about the wiki, but it matters in two ways:

- it should be re-checked live before anyone relies on the statement in the
  issue;
- it is the closest in-tree reference for how a text-bearing resource is
  exposed through API v3, and the resolution design uses it as the pattern.

## Upstream Status, 2026-09-24

Checked for `task.self.check-upstream-openproject-wiki-api-roadmap`.

- The upstream request is
  [OP #39099 "[API] add POST/PUT method for creating Wiki pages"](https://community.openproject.org/work_packages/39099),
  a Feature opened on 2021-10-07. Status `new`, target version `Wish List`.
  Five votes; RICE score set to 6.67, lowered to 1, later removed. Staff
  replies in 2021 and 2023: limited capacity, features with more votes and a
  higher score come first, community pull requests are welcome. The latest
  activity is a `+1` on 2026-06-17. No staff statement about reading page
  text or listing pages through the API.
- The official API reference still calls the wiki page resource "currently a
  stub": a single `GET` with `id`, `title`, project and attachments.
- The [2026 roadmap](https://www.openproject.org/blog/roadmap-2026/) covers
  XWiki integration and BlockNote document workflows. It does not mention the
  internal wiki's API.
- OpenProject 17.6 added wiki providers and XWiki integration; OpenProject
  says the native wiki stays maintained, not that its API grows.

Consequence for the plugin: plan for a long lifetime. Nothing upstream
suggests the gap closes within the next releases. The plugin must be kept
building against each OpenProject upgrade, which the exact-version image and
throwaway runtime check already require.

Decision on 2026-09-25: the vote and comment below are dropped. The operator
has no account on the community tracker. Google sign-in there only logs in
existing accounts, because registration is disabled, and one more vote adds
little to a five-year-old request. An account is worth creating only when there
is something to contribute upstream: the plugin, the page delete route or a bug
report. One such bug was seen on the way: the community site's Google sign-in
form is blocked in Chrome by the site's own `form-action` Content Security
Policy, because the policy does not list the Google host the form redirects to.
The draft is kept for that case.

Upstream actions considered, both needing an OpenProject community account:

1. vote for OP #39099 and add a comment with the concrete use case. Draft:

   > +1 with a concrete case: an AI agent participates in a project with its
   > own API token and must read and edit the project's wiki. API v3 exposes
   > only id and title, so the agent cannot read page text, list pages or
   > write. We run a small plugin that adds a project-scoped collection, text
   > with lockVersion to the page representation, POST and PATCH through
   > WikiPages::CreateService and UpdateService, with the standard wiki
   > permissions. We would be glad to contribute it as a pull request if the
   > direction fits your API plans.

2. decide whether to offer the plugin upstream as a pull request. Staff
   invited contributions on this request. It would need the plugin reshaped
   into core API v3 code, upstream tests and a longer review. Not planned
   without an explicit decision.

## Impact

- An agent that participates in a project cannot use the project's shared
  knowledge surface. It can neither consume agreed invariants nor keep them
  current.
- The human and the agent end up with two different knowledge surfaces, which
  is exactly the split the OpenProject-centered operating model is meant to
  avoid.
- Any workaround that stores knowledge elsewhere has to be maintained by hand
  and drifts.

## Temporary Measure In Place

The affected project moved participant-facing knowledge from wiki pages into
work packages of a document-like type. A work-package description is returned
in full by API v3 and accepts `PATCH` with `lockVersion`, and a human can edit
it in the browser. The original wiki pages were reduced to pointers.

This is recorded by the project owner as a half measure, not as the target:

- work packages have no page tree, no inter-page links and no table of
  contents;
- they carry status, assignee and dates, which are meaningless for a document;
- the goal is to return to the wiki, not to replace it.

## Directions Considered

| Direction | Assessment | Decision |
| --- | --- | --- |
| 1. Session authentication for the agent: give it a password and log in. | Breaks the deliberate "token only" model. Depends on HTML parsing that changes with every layout update. Contradicts the acceptance criterion. | Rejected. |
| 2. Extend API v3 with an own module or patch: page text, collection and write routes. | Cleanest result. The stated cost is intervention in the application and upgrade burden. That cost is already paid: the instance already runs a custom exact-version plugin image with a throwaway-runtime check, rollback path and fast code-layer rebuild. Standard wiki permissions remain the only authorization. | Accepted. |
| 3. Upstream request: check for an existing ticket on wiki in API v3, support or file one. | Cheap and correct, but does not solve the problem on any useful timescale. | Accepted as a parallel task, not as the solution. |
| 4. Intermediate service that reads the wiki from the database and serves it by token. | Creates a second access surface with its own authorization. This is the shape of the earlier workaround that had to be removed. | Rejected. |

## Acceptance Criterion

Carried over from the issue unchanged:

The agent, with its own token, gets the list of wiki pages of a project, reads
the full text of any of them, creates a new page and edits an existing one. The
agent's edit is visible to a human in the browser, and a human's edit is
visible to the agent. No password and no HTML parsing.

## Boundary

- The gap is generic OpenProject behavior. It is public-safe.
- The affected project, its page names, its work-package identifiers, the
  instance endpoint and container names are private and are not recorded here.
- The resolution is an OpenProject-side plugin. It does not change the RePlan
  canonical model, RePlan CLI or the RePlan/OpenProject ownership split.
