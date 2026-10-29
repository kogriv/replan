# OpenProject wiki API plugin design

Status: current. Index: [OpenProject design index](openproject-design-index.md).

Original status: accepted. Direction, amended implementation design and the thin
text-free collection element were accepted by the operator on 2026-09-21.
Implemented, and proven in a throwaway OpenProject 17.6.0 on 2026-09-21; see
[`openproject-wiki-api-throwaway-runtime-test.md`](openproject-wiki-api-throwaway-runtime-test.md).
All four verification points hold and the own-namespace fallback was not
needed. Deployed to the live instance on 2026-09-21; see
[`openproject-wiki-api-live-deployment-result.md`](openproject-wiki-api-live-deployment-result.md).
Acceptance against the issue criterion in live use is the remaining step.

Date: 2026-09-21.

Related task: `task.self.design-openproject-wiki-api-access`.

Gap record: [`openproject-wiki-api-gap.md`](openproject-wiki-api-gap.md).

Design review:
[`openproject-wiki-api-plugin-design-review.md`](openproject-wiki-api-plugin-design-review.md).

Source: GitHub issue #1.

OpenProject sync: pending. See the gap record for the reason.

## Purpose

OpenProject 17.6.0 exposes wiki pages through API v3 as `id`, `title` and
project link only. There is no page text, no collection route and no write
route. An API-token participant therefore cannot use the wiki at all.

This document defines how to close that gap with a small OpenProject plugin so
that an agent with an API token can list, read, create and edit wiki pages,
with humans and agents seeing each other's edits.

## Product Decision

Extend OpenProject API v3 with an own plugin.

Reasons:

- it is the only considered direction that satisfies the acceptance criterion
  without a password and without HTML parsing;
- the cost that normally argues against it is already paid: the dogfood
  instance already runs a custom OpenProject image built from the exact
  `17.6.0` builder and slim pair, with a throwaway-runtime check, a rollback
  path and a fast code-only rebuild layer;
- authorization stays with OpenProject's own wiki permissions, so no second
  access surface appears;
- the upstream source at `v17.8.0` and on `dev` is unchanged for wiki text, so
  waiting for an upgrade does not help.

The upstream request stays a parallel task. If upstream ships equivalent
routes, this plugin is removed and clients keep working because the plugin
deliberately uses the upstream resource shape.

## Separate Plugin, Not The Overlay Plugin

The capability ships as a new gem:

```text
integrations/openproject/plugins/openproject-wiki_api/
```

It is not added to `openproject-replan_overlay`.

Reasons:

- `openproject-replan_overlay` is documented and audited as read-only. It
  renders a RePlan artifact and never writes OpenProject data. A wiki write
  path inside it would break that contract and the 2026-09-21 audit statement;
- the wiki capability has nothing to do with RePlan resource planning. It is
  needed by other projects on the same instance;
- a separate gem can be enabled, disabled, rolled back or offered upstream
  independently.

Both gems share the same custom image, the same build helper and the same
deployment procedure.

## Authorization Model

No new permission and no new project module are introduced.

| Operation | OpenProject rule used |
| --- | --- |
| List and read | `WikiPage.visible(current_user)`, which resolves to `view_wiki_pages` on the project. |
| Create and edit | `WikiPages::CreateContract` and `WikiPages::UpdateContract`, which require `edit_wiki_pages`. |
| Edit a protected page | The base contract already requires `manage_wiki`. The plugin does not loosen it. |

Authentication is the standard API v3 token authentication. The web
`accept_key_auth` mechanism is not involved and is not patched.

Consequence: what the agent can do is controlled in the normal OpenProject
role editor, by a human, exactly like for a human member.

## API Surface

The plugin extends the existing upstream `WikiPage` resource in place instead
of creating a parallel namespace. The routes are what upstream would plausibly
add, so a later upstream implementation replaces the plugin with little or no
client change. That claim holds only while the plugin stays on these paths; see
the fallback rule below.

| Method and path | Behavior |
| --- | --- |
| `GET /api/v3/projects/{id}/wiki_pages` | New. Paginated collection of visible pages in the project's wiki, ordered by title. Elements are thin: metadata and links, no text. |
| `GET /api/v3/wiki_pages/{id}` | Existing route, extended representation: adds `text`, `slug`, `lockVersion`, `createdAt`, `updatedAt`, `protected`, and `parent`, `author`, `update` links. Existing attachment links stay. |
| `POST /api/v3/projects/{id}/wiki_pages` | New. Creates a page through `WikiPages::CreateService`. Returns 201. |
| `PATCH /api/v3/wiki_pages/{id}` | New. Updates a page through `WikiPages::UpdateService`. The plugin requires `lockVersion`. |

Representation of one page:

```json
{
  "_type": "WikiPage",
  "id": 42,
  "title": "Invariants",
  "slug": "invariants",
  "lockVersion": 7,
  "protected": false,
  "createdAt": "2026-09-01T10:00:00Z",
  "updatedAt": "2026-09-20T08:30:00Z",
  "text": { "format": "markdown", "raw": "# Invariants\n...", "html": "<h1>...</h1>" },
  "_links": {
    "self": { "href": "/api/v3/wiki_pages/42" },
    "project": { "href": "/api/v3/projects/7" },
    "parent": { "href": "/api/v3/wiki_pages/40" },
    "author": { "href": "/api/v3/users/5" },
    "update": { "href": "/api/v3/wiki_pages/42", "method": "patch" }
  }
}
```

`text` uses the standard API v3 formattable shape from
`API::Decorators::Formattable`, the same one work-package `description` uses.
The response carries `format`, `raw` and `html`. Agents read and write `raw`
and must not parse `html`. The "no HTML parsing" criterion is about not
scraping the web wiki; it does not forbid the standard `html` field in JSON.

`lockVersion` is the `lock_version` column. `WikiPage#version` is the journal
version and must not be exposed under that name.

### Collection Elements

Decision: the collection does not carry page text.

`extend_api_response` evaluates its block on the single
`API::V3::WikiPages::WikiPageRepresenter` class, so every user of that class
serializes `text`. The collection therefore must not reuse it. Collection
elements use a separate thin representer,
`API::V3::WikiPages::WikiPageListItemRepresenter`, with `id`, `title`, `slug`,
`lockVersion`, `protected`, `createdAt`, `updatedAt` and `self`, `project`,
`parent`, `author` links. It does not include `CachedRepresenter`.

Reason for choosing the thin element over the simpler "text in every element":
the `wiki_pages.text` column allows 16 MB per page, and the formattable `html`
is rendered through the text-formatting pipeline for every element. A listing
call made only to find a page id would otherwise read and render every page of
the wiki. The agent gets the list cheaply, then reads the pages it needs.

Collection implementation:

```text
WikiPage.visible(current_user).where(wiki_id: project.wiki.id).order(:title)
```

with the standard API v3 offset pagination (`offset`, `pageSize`). The first
slice introduces no Query class, no filters and no schema endpoint.
`ParamsToQueryService` must not be called for `WikiPage`: OpenProject 17.6.0
has no wiki-page query. The queries in the bundled `wikis` module are about
page links to wiki providers, not about listing `WikiPage` rows.

### Writable Attributes

| Attribute | Create | Update |
| --- | --- | --- |
| `title` | required | not writable |
| `text.raw` | optional | writable |
| `_links.parent` | optional | writable |
| `lockVersion` | not applicable | required |
| `protected`, `slug`, `author`, `wiki` | not writable | not writable |

Endpoint-injected attributes, never taken from the client:

- `wiki`: on create the endpoint passes `wiki: project.wiki`, as
  `WikiController#create` does. `WikiPages::BaseContract` requires it.
- `author`: `WikiPages::SetAttributesService` sets it to the current user, and
  `WikiPages::CreateContract` rejects any other author.

The payload representer is the only guard for the unwritable attributes.
`WikiPages::BaseContract` declares `title` and `protected` as writable
attributes, so the contract alone would accept a rename or, for a
`manage_wiki` user, a protection change. The payload representer must not
expose them. A client that sends an unwritable property gets the standard API
v3 unwritable-property error; the page is not renamed.

Title change is excluded from the first slice on purpose. `WikiPage` rewrites
the slug and creates redirects on rename (`update_redirects`). That is a
separate reviewed behavior, not a side effect of a text edit.

## Write Rules

- Every write goes through the upstream service and contract. The plugin never
  calls `save` on the model and never writes SQL. This keeps validation,
  attachment handling, journals and notifications identical to a browser edit.
- `WikiPage` is journalized and registers `text` as a `wiki_diff` journal
  field. An agent edit shows up in the page history with the token user as
  author, and a human can diff or revert it.
- Contract errors are returned as the standard API v3 error body through
  `ErrorBase.create_and_merge_errors`.
- `WikiController#update` rescues `StaleObjectError` and renders an HTML 422.
  That is the browser path and must not be copied.

### lockVersion Is A Plugin Rule

`WikiPages::UpdateContract` at `v17.6.0` does not require the client to send
`lockVersion`:

```ruby
attribute :lock_version do
  if model.lock_version.nil? || model.lock_version_changed?
    errors.add :base, :error_conflict
  end
end
```

It rejects only a nil or changed in-memory `lock_version`. If the client omits
the field, the loaded record keeps the database value, the contract passes and
`UpdateService` saves. Concurrent human and agent edits are then
last-write-wins. That is the lost update this API must prevent.

Work packages use the same contract shape and are safe only because their
parse service forces the field:
`API::V3::WorkPackages::ParseParamsService` ends with
`.reverse_merge(lock_version: nil)`. A missing `lockVersion` becomes nil, and
nil is a conflict.

Plugin rule:

- PATCH goes through a payload representer, never through a raw JSON hash;
- the update endpoint's `params_modifier` applies the same
  `.reverse_merge(lock_version: nil)` to the parsed attributes, so
  `WikiPages::UpdateService` always assigns a client-derived `lock_version`
  onto the model before the contract runs;
- a missing `lockVersion` is HTTP 409, the same as a stale one;
- on 409 the page text is unchanged; the agent must re-read, merge and retry,
  never overwrite.

The error mapping already exists upstream: `ErrorBase` maps `:error_conflict`
to `API::Errors::Conflict` (409, `UpdateConflict`), and `API::RootAPI` maps
`ActiveRecord::StaleObjectError` to the same error, which covers a race between
validation and save.

## HTTP Status By Case

| Case | Status | Why |
| --- | --- | --- |
| GET a page the token cannot see | 404 | `WikiPage.visible(current_user).find` raises `RecordNotFound`; existence is hidden. |
| GET collection with `view_wiki_pages` | 200, possibly empty | Visible pages only. |
| GET collection without `view_wiki_pages` | 403 | Explicit `authorize_in_project(:view_wiki_pages, project:)`, as the `budgets` reference does. Without it the answer would be an empty 200 that still confirms the wiki exists. Added by the gate 1 review. |
| GET collection or POST when the project has no `Wiki` record or the wiki module is disabled | 404 | The project has no wiki resource. The plugin never creates a `Wiki` as a side effect. |
| POST success | 201 | |
| PATCH success | 200 | `lockVersion` incremented. |
| POST or PATCH without `edit_wiki_pages` | 403 | Contract `:error_unauthorized`. |
| PATCH of a protected page without `manage_wiki` | 403 | Same contract. |
| Unwritable property in the payload (`title` on PATCH, `protected`, `slug`) | 422 | `API::Errors::UnwritableProperty`. |
| Validation error: blank title, bad or circular parent, parent from another project | 422 | `ErrorBase.create_and_merge_errors`. |
| Missing or stale `lockVersion` on PATCH | 409 | The plugin rule above. |

A read-only member gets 403 on write. That must not be copied onto GET of an
invisible page, which is 404.

## Implementation Shape

Upstream plugin hooks used, all from `OpenProject::Plugins::ActsAsOpEngine`:

- `register` for the gem, with no project module and no permission. The
  overlay throwaway test showed that Bundler can install a gem that OpenProject
  never loads unless `Gemfile.plugins` requires the entry file and the engine
  registers. Both are required;
- `add_api_endpoint "API::V3::Projects::ProjectsAPI", :id` to mount the project
  collection and create routes. The bundled `budgets` module uses exactly this
  form to mount `BudgetsByProjectAPI`;
- `add_api_endpoint "API::V3::WikiPages::WikiPagesAPI", :id` for `PATCH`;
- `extend_api_response(:v3, :wiki_pages, :wiki_page)` for the extra properties
  and links of the single-page representation;
- `add_api_path` for the new path helpers.

In-tree references, by concern:

| Concern | Reference at `v17.6.0` |
| --- | --- |
| Formattable text in a GET body | work packages; `modules/documents` `DocumentRepresenter` |
| Mounting under `ProjectsAPI`, `:id` | `modules/budgets` engine |
| Create and update | `API::V3::Utilities::Endpoints::Create` and `Update` with a payload representer, as `news`, `versions` and work packages do |
| Forcing `lockVersion` | `API::V3::WorkPackages::ParseParamsService`, `.reverse_merge(lock_version: nil)` |

`modules/documents` is not the write-path pattern. Its `PATCH` handler passes
`JSON.parse(request.body.read)` straight into the service: no payload
representer, no HAL `_links`, no `lockVersion`. Copying it would reproduce the
lost-update defect.

The endpoint helpers deduce their collaborators by name (`V3Deductions`): the
render representer `WikiPageRepresenter` and the parse representer
`WikiPagePayloadRepresenter`. `API::Utilities::Endpoints::Bodied` runs
`parse`, then `params_modifier` in the request context, then the service. The
plugin uses `params_modifier` for both injections:

- update: `params.reverse_merge(lock_version: nil)`;
- create: `params.merge(wiki: @project.wiki)`.

No own parse service is needed. The default instance generator of the update
helper reads `@wiki_page`, so the `:id` block must set it from
`WikiPage.visible(current_user).find(params[:id])`.

Gem layout:

```text
openproject-wiki_api/
  openproject-wiki_api.gemspec
  README.md
  lib/open_project/wiki_api.rb
  lib/open_project/wiki_api/engine.rb
  lib/open_project/wiki_api/version.rb
  lib/api/v3/wiki_pages/wiki_pages_by_project_api.rb
  lib/api/v3/wiki_pages/wiki_page_collection_representer.rb
  lib/api/v3/wiki_pages/wiki_page_list_item_representer.rb
  lib/api/v3/wiki_pages/wiki_page_payload_representer.rb
  lib/api/v3/wiki_pages/wiki_page_update_payload_representer.rb
```

Loading rule, added by the gate 1 review: the engine must not `require` or
`require_relative` any file under `lib/api/`, and every file under `lib/api/`
defines exactly one constant named after the file. `Gemfile.plugins` loads the
engine through `Bundler.require`, which upstream `config/application.rb` runs
before the application class and before any autoloader exists, so the core
`API` namespace cannot be resolved at that moment. `ActsAsOpEngine` already
adds the gem's `lib` to `eager_load_paths`; the classes are autoloaded when the
late-evaluated `add_api_endpoint` and `extend_api_response` blocks reference
them. The bundled `budgets` module works this way.

Naming rule, added by the throwaway runtime test: OpenProject's Zeitwerk
inflector turns every path segment ending in `_api` into `...API`. That applies
to the gem's own namespace directory too, so `lib/open_project/wiki_api/`
defines `OpenProject::WikiAPI`, not `WikiApi`. The wrong spelling passes the
image build and the seeder and fails with `NameError` under eager loading,
which is how the web service starts.

- `engine.rb`: `register`, the two `add_api_endpoint` blocks, the representer
  extension and the path helpers. The `PATCH` block uses the `Update` endpoint
  helper with the `lock_version` `params_modifier`. It is not a raw
  `JSON.parse` block.
- `wiki_pages_by_project_api.rb`: collection `GET` and `POST` through the
  `Create` endpoint helper with `wiki: project.wiki` injected; 404 when the
  project has no wiki.
- `README.md`: the four routes, the writable attributes and the status table.
  The upstream OpenAPI files stay untouched and stale.

Packaging lessons already paid for by the overlay plugin apply unchanged:

- `Gemfile.plugins` must `require` the gem entry file explicitly;
- the engine must `register`;
- the image must be built from the exact OpenProject builder and slim pair of
  the live instance;
- all Rails services (`web`, `worker`, `cron`, `seeder`) use the same image.

Build helper change: `scripts/build-openproject-plugin-image.sh` currently
hardcodes one plugin directory. It must copy every directory under
`integrations/openproject/plugins/` and emit one `Gemfile.plugins` entry per
gem. The `deps-base`, `code` and `auto` modes stay. Adding a gem changes
`Gemfile.lock`, so the first image with the new gem needs one `deps-base`
rebuild; later Ruby-only changes use the fast `code` layer.

## Points To Verify In The Throwaway Runtime

These are design risks, not decided facts.

1. `WikiPageRepresenter` includes `API::Caching::CachedRepresenter`. Confirm
   that properties added through `extend_api_response` are part of the cached
   JSON and that a text edit invalidates it. The cache key includes the
   represented model and an edit changes `updated_at` and `lock_version`, so
   this is expected to hold.
2. Confirm that `add_api_endpoint` with the `:id` path patches into the
   existing `route_param :id` block of `WikiPagesAPI` on 17.6.0 without
   shadowing the upstream `GET` or the mounted `AttachmentsByWikiPageAPI`. The
   `budgets` precedent proves the form for `ProjectsAPI`; `WikiPagesAPI` still
   has to be proven.
3. Confirm the 409 mapping end to end for a missing and for a stale
   `lockVersion`. The upstream mapping exists; what is being proven is that the
   plugin's `params_modifier` actually forces the field.
4. Confirm 404 for collection `GET` and for `POST` on a project without a
   `Wiki` record and on a project with the wiki module disabled, and that no
   `Wiki` row is created.

Fallback if point 1 or 2 fails: mount the same behavior under an own namespace,
`/api/v3/wiki_api/...`, with an own representer. The authorization model, write
rules and acceptance criterion do not change.

The fallback is an emergency exit from the throwaway runtime, not a second
supported public surface. Own-namespace paths are not what upstream would add.
If the fallback is taken: remove the drop-in-replacement claim from this
design, record the actual paths here, and only then write clients, against
those paths only, and only then deploy live.

## Non-Goals For The First Slice

- page deletion;
- rename, slug change and redirects;
- protect and unprotect;
- history, diff and annotate routes;
- wiki menu items;
- attachments, which upstream API v3 already serves;
- enabling or configuring the wiki module of a project;
- external wiki providers from the upstream `wikis` module;
- a saved Query, filters or a schema endpoint for wiki pages;
- any password, session or HTML path;
- any database-reading side service;
- changes to RePlan canonical model, CLI or the OpenProject/RePlan ownership
  split.

## Agent-Side Usage Rule

The plugin makes wiki writes possible. It does not make them unreviewed by
default for RePlan's own dogfood project.

For the RePlan self-dogfood OpenProject project the existing rule stays:
OpenProject mutations go through guarded helpers, dry-run by default, apply
behind `REPLAN_APPLY=1`. If RePlan agents start writing wiki pages in that
project, a `wiki` command family should be added to the OpenProject toolkit
first. That is a follow-up decision, not part of this slice.

Other projects on the instance own their own agent write policy.

## Verification Plan

Scaffold check, no runtime needed, extending the existing pattern of
`scripts/check-openproject-plugin-scaffold.sh`:

The current check is overlay-specific. It has to become gem-aware, with one
assertion set per gem, not a copied file list. For the wiki gem it asserts:

- gemspec, engine, entry file and version exist and parse;
- `register` is present, and no project module, no permission and no
  `accept_key_auth` patch is declared;
- the four routes, the representer extension and the thin list-item
  representer are declared;
- the update endpoint forces `lock_version`, and the payload representer does
  not expose `title` on update, `protected`, `slug`, `author` or `wiki`;
- no direct `save`, `update_column`, `update_columns` or SQL in the plugin;
- no `JSON.parse` of the request body and no `ParamsToQueryService`.

Throwaway runtime, extending
`scripts/check-openproject-plugin-throwaway-runtime.sh`:

The existing script boots a disposable OpenProject and checks the overlay
through a Rails runner. That is not enough here. An in-process call proves
neither token authentication, nor Grape route patching, nor content-type
handling. The wiki checks are real HTTP calls with API tokens.

Setup:

1. build the image with both gems;
2. boot a disposable OpenProject with a clean database;
3. seed a project with the wiki module and a start page, an editor member with
   `view_wiki_pages` and `edit_wiki_pages`, a read-only member with
   `view_wiki_pages` only, an API token for each, a second project without a
   wiki, and one protected page.

HTTP cases:

1. editor: list pages; elements carry no `text`;
2. editor: `GET` one page; `text.raw` is present;
3. editor: `POST` a page; 201; a following `GET` returns the text;
4. editor: `PATCH` with the current `lockVersion`; 200; text changed;
   `lockVersion` incremented;
5. editor: `PATCH` without `lockVersion`; 409; text unchanged;
6. editor: `PATCH` with a stale `lockVersion`; 409; text unchanged;
7. editor: `PATCH` with `title`; 422; page not renamed;
8. editor: `PATCH` of the protected page; 403;
9. read-only: list and `GET` succeed; `POST` and `PATCH` return 403;
10. a token without membership: `GET` of the page returns 404;
11. project without a wiki: collection `GET` and `POST` return 404; no `Wiki`
    row appears;
12. attachments `GET` and `POST` on the same wiki page id still work;
13. the journal of the `PATCH` from case 4 shows the editor as author;
14. the overlay plugin routes still work in the same image;
15. boot proof, run before every HTTP case: web, worker, cron and seeder start
    with both gems, `bundle info` lists both, and Rails reports the four routes;
16. editor: `POST` a child page with `_links.parent`; 201; `GET` shows the
    parent link; `PATCH` moving it under a page of another project returns 422;
17. a member without `view_wiki_pages`: collection `GET` returns 403.

Cases 15 to 17 were added by the gate 1 review. The original list never proved
that the image boots before calling it, and never exercised `_links.parent`
although `parent` is writable.

The four verification points above are resolved by cases 2 and 4 (point 1),
2, 4 and 12 (point 2), 5 and 6 (point 3) and 11 (point 4).

Live acceptance, in a maintenance window with backup and rollback as in
[`openproject-plugin-live-installation-result.md`](openproject-plugin-live-installation-result.md):

- the issue's acceptance criterion, executed by the agent with its own token;
- a human confirms the agent's edit in the browser;
- the agent confirms a human's edit through the API;
- a sanitized result document is recorded, without the affected project, page
  names or live identifiers;
- GitHub issue #1 is closed here, not at design or implementation time.

## Backlog

| Task | Scope |
| --- | --- |
| `task.self.design-openproject-wiki-api-access` | This gap record, this design and the backlog entries. |
| `task.self.check-upstream-openproject-wiki-api-roadmap` | Search the upstream community tracker for wiki text in API v3; support or file a request; re-check the documents API claim live. |
| `task.self.implement-openproject-wiki-api-plugin` | The gem with the payload representer, forced `lockVersion` and thin list-item representer; the gem-aware scaffold check; the multi-plugin build helper. Must not start before this design is accepted by the operator. |
| `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime` | The HTTP-with-token throwaway proof and the four verification points. |
| `task.self.deploy-openproject-wiki-api-plugin-live` | Private live deployment, acceptance against the issue criterion, closing the issue. |

Moving the affected project's knowledge back from work packages into the wiki
is that project's own task and is not tracked here.

## Done Criteria For This Design Task

- the gap is recorded with evidence and boundary;
- the direction and the plugin boundary are decided and written down;
- the API surface, authorization model and write rules are specified well
  enough to implement without hidden context;
- open technical risks are listed with a fallback;
- backlog tasks exist in RePlan canonical data with resource demands and
  dependencies, marked `openproject_sync_pending`;
- the human operator has reviewed the design.

## Review Amendments Applied

The 2026-09-21 agent review recorded in
[`openproject-wiki-api-plugin-design-review.md`](openproject-wiki-api-plugin-design-review.md)
accepted the product direction and withheld implementation approval until its
amendments were applied here. They were applied on 2026-09-21, after the
review's source claims were re-checked against upstream `v17.6.0`.

| Review item | Where it is applied |
| --- | --- |
| Required 1: `lockVersion` is a plugin rule, not a contract guarantee | "lockVersion Is A Plugin Rule"; HTTP cases 4 to 6 |
| Required 2: documents `PATCH` is not the write pattern; use the `Create` and `Update` endpoint helpers | "Implementation Shape" |
| Required 3: collection text versus the single extended representer | "Collection Elements": option A, thin list-item representer |
| Required 4: no `ParamsToQueryService` for wiki pages | "Collection Elements" |
| Should-fix 1, 2, 3, 4: status by case, injected `wiki` and `author`, unwritable `title` and `protected`, `raw` not `html` | "HTTP Status By Case", "Writable Attributes", "API Surface" |
| Should-fix 5, 6: attachments and missing-`lockVersion` cases | throwaway HTTP cases 5 and 12 |
| Should-fix 7: `register` without a project module | "Implementation Shape" |
| Should-fix 8: fallback is an emergency exit | "Points To Verify In The Throwaway Runtime" |
| Should-fix 9: title order | "Collection Elements" |
| Should-fix 10: gem-aware scaffold check | "Verification Plan" |

Two points go beyond the review:

- the review asked for an explicit parse service if the update helper needed
  one. Reading `API::Utilities::Endpoints::Bodied` showed that
  `params_modifier` runs after parsing and before the service, so both the
  forced `lock_version` and the injected `wiki` use it and no own parse service
  is needed;
- the review left the missing-wiki status as "404 or 422, pick one". This
  design picks 404 for both collection `GET` and `POST`.

On collection text the review's default was option B, text in every element,
as the less fragile choice when payload size does not matter. This design
chooses option A for the reason given in "Collection Elements". The operator
confirmed option A on 2026-09-21: the work is done without cutting corners, and
a list format is hard to change once clients depend on it.

## Operator Acceptance

On 2026-09-21 the operator accepted this amended text, option A for collection
elements, and the agent-agnostic execution resource for planning agent work.
All done criteria of `task.self.design-openproject-wiki-api-access` are met.
The task is `accepted` in RePlan, its demands are consumed, and
`dependency.wiki-api-design-before-plugin-implementation` is resolved.

## Implementation Handoff And Review Protocol

Operator decision, 2026-09-21: the plugin is implemented by a different AI
agent than the one that wrote this design. The design author acts as reviewer.
Author and reviewer are deliberately not the same agent.

Planning effect:

- implementation agent work is booked on `resource.ai.agent.executor`;
- review agent work is booked on `resource.ai.review-assistant`, as
  `resource_demand.wiki-api-plugin.ai-review`,
  `resource_demand.wiki-api-throwaway-runtime.ai-review` and
  `resource_demand.wiki-api-live-deployment.ai-review`;
- operator acceptance stays on `resource.role.reviewer`. An agent review
  informs the operator's decision. It does not replace it.

What the implementer receives: this document, the gap record, the design
review, and `task.self.implement-openproject-wiki-api-plugin`. Nothing else is
required context. If the implementer needs information that is not in these
documents, that is a design defect and is reported, not guessed around.

Rules for the implementer:

- work on a branch; do not push to `main`;
- do not touch the private live OpenProject instance;
- do not change `openproject-replan_overlay` behavior. The only allowed shared
  changes are the multi-plugin build helper and the gem-aware scaffold check;
- a deviation from this design is allowed only when written down with the
  reason and the upstream source reference that forced it. A silent deviation
  is a review failure;
- the four points in "Points To Verify In The Throwaway Runtime" are not to be
  decided by the implementer from reading code. They belong to the throwaway
  task.

Review gates:

| Gate | When | Reviewer checks |
| --- | --- | --- |
| 1. Implementation review, first pass recorded in [`openproject-wiki-api-plugin-implementation-review.md`](openproject-wiki-api-plugin-implementation-review.md) | before `task.self.implement-openproject-wiki-api-plugin` is accepted | conformance checklist below; scaffold check passes; overlay scaffold check still passes; `go test ./...` and `./scripts/smoke-mvp.sh` still pass |
| 2. Throwaway evidence review | before `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime` is accepted | all fourteen HTTP cases ran as real HTTP calls with tokens, with recorded status codes; the four verification points are resolved with evidence; if the fallback was taken, the drop-in claim is removed from this document first |
| 3. Live acceptance review | before GitHub issue #1 is closed | the issue criterion was executed by an agent with its own token; human and agent edits were confirmed in both directions; the result document is sanitized |

Conformance checklist for gate 1:

1. separate gem; `register` without a project module, permission or
   `accept_key_auth` patch;
2. exactly the four routes of "API Surface", on those paths;
3. writes only through `WikiPages::CreateService` and `UpdateService` via the
   `Create` and `Update` endpoint helpers; no `save`, `update_column`, SQL,
   raw `JSON.parse` of the body or `ParamsToQueryService`;
4. `lock_version` forced to nil when absent, so a missing `lockVersion` is a
   conflict;
5. the payload representer exposes only `title` on create, `text`, `parent`
   and `lockVersion`; never `protected`, `slug`, `author` or `wiki`;
6. `wiki` is injected from the project; a project without a wiki is 404 and no
   `Wiki` row is created;
7. collection elements use the thin list-item representer, without text,
   ordered by title, scoped through `WikiPage.visible`;
8. the single-page representation keeps the upstream attachment links;
9. the status table of "HTTP Status By Case" is implemented as written;
10. the plugin README documents the routes, writable attributes and statuses;
11. no private endpoint, token, project name or live identifier in the
    repository.

Review outcome vocabulary: `accepted`, `accepted_with_notes`,
`changes_required`. Each review is recorded as a document under `docs/dev/`,
like the design review, so that it is not chat-only. `changes_required` keeps
the task `in_progress`.
