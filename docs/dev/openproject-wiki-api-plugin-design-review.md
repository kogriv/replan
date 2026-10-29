# OpenProject wiki API plugin design review

Status: dated record (2026-09-21). Index: [OpenProject design index](openproject-design-index.md).

Original status: completed design review; product direction accepted; implementation
design not accepted for coding until the required amendments are applied.

Date: 2026-09-21.

Reviewer: Cursor Grok 4.6 in a local checkout, at the
operator's request.

Related task: `task.self.design-openproject-wiki-api-access`.

Reviewed documents:

- [`openproject-wiki-api-gap.md`](openproject-wiki-api-gap.md);
- [`openproject-wiki-api-plugin-design.md`](openproject-wiki-api-plugin-design.md);
- calibration Step 44 in
  [`../roadmap/self-dogfood-planning-calibration.md`](../roadmap/self-dogfood-planning-calibration.md).

Source: GitHub issue #1, the two documents above, the RePlan canonical backlog
for the five wiki-API tasks, the existing overlay plugin and image-build
helpers, and a source reading of public OpenProject tag `v17.6.0`.

OpenProject sync: pending. The design task remains `in_progress` with
`openproject_sync_pending`. This review does not create a live work package
and does not close the design task.

## Purpose

This document is the unabridged record of the design review of the proposed
OpenProject wiki API plugin.

It exists so that the review is not only in chat. The review was requested
after the gap record and the first implementation design were written. The
operator asked for a review of the proposed solution. This document is that
review, written in full.

It answers:

1. Is the product direction right?
2. Is the first implementation design specified well enough to implement
   without hidden context?
3. Which design claims are confirmed by OpenProject 17.6.0 source?
4. Which design claims would produce the wrong contract if implemented as
   written?
5. What must be amended in the design before
   `task.self.implement-openproject-wiki-api-plugin` starts?
6. What is optional, what is a process note, and what this review does not
   decide?

This is a design review. It is not an implementation. No plugin gem was
created, no OpenProject image was rebuilt, and no live instance was changed
while this review was written.

## Scope

In scope:

- GitHub issue #1 as the requirement and acceptance criterion;
- the four directions recorded in the issue and decided in the gap document;
- the gap record's evidence, impact, temporary measure and privacy boundary;
- the plugin design: product decision, gem boundary, authorization, API
  surface, write rules, implementation shape, verification points, non-goals,
  agent-side usage rule, verification plan and backlog;
- the five RePlan canonical tasks, their dependencies, resource demands,
  dates and `openproject_sync_pending` tags;
- calibration Step 44;
- the existing overlay plugin, `scripts/build-openproject-plugin-image.sh`,
  `scripts/check-openproject-plugin-scaffold.sh` and the throwaway-runtime
  procedure as the packaging and verification baseline this plugin would
  reuse;
- public OpenProject source at tag `v17.6.0` for the wiki model, wiki
  contracts and services, wiki API v3 stub, documents API, plugin hooks,
  formattable representer, optimistic-locking error mapping and work-package
  write endpoints.

Out of scope:

- implementing the plugin;
- amending the design document as part of this review (amendments are
  proposed here and remain work on the same design task);
- filing or supporting an upstream OpenProject ticket;
- live calls against the private OpenProject instance;
- moving the affected project's knowledge back from work packages into wiki
  pages;
- adding a RePlan `wiki` command family to the OpenProject toolkit;
- changing RePlan canonical model, CLI, or the OpenProject/RePlan ownership
  split;
- closing GitHub issue #1.

## Method

The review was done by reading, not by implementing.

Repository artifacts read in full:

- `docs/dev/openproject-wiki-api-gap.md`;
- `docs/dev/openproject-wiki-api-plugin-design.md`;
- the five wiki-API task records in
  `examples/replan-self/canonical/tasks.yaml`;
- the four wiki-API dependencies in
  `examples/replan-self/canonical/dependencies.yaml`;
- the wiki-API resource demands in
  `examples/replan-self/canonical/resource_demands.yaml`;
- the current-plan entries in
  `examples/replan-self/canonical/current_plan.yaml`;
- calibration Step 44;
- `integrations/openproject/plugins/openproject-replan_overlay/lib/open_project/replan_overlay/engine.rb`;
- `integrations/openproject/plugins/openproject-replan_overlay/openproject-replan_overlay.gemspec`;
- `scripts/build-openproject-plugin-image.sh`;
- `scripts/check-openproject-plugin-scaffold.sh`;
- `docs/dev/openproject-plugin-throwaway-runtime-test.md`;
- `docs/dev/openproject-plugin-deployment-path.md`.

GitHub issue #1 was read with `gh issue view 1 --repo kogriv/replan` using
the operator's session bus, because unauthenticated fetches of the private
issue URL return 404.

OpenProject 17.6.0 source was read from the public upstream tree at tag
`v17.6.0`, not from the private live container. The files read were:

- `lib/api/v3/wiki_pages/wiki_pages_api.rb`;
- `lib/api/v3/wiki_pages/wiki_page_representer.rb`;
- `docs/api/apiv3/components/schemas/wiki_page_model.yml`;
- `docs/api/faq/README.md`;
- `app/models/wiki_page.rb`;
- `app/contracts/wiki_pages/base_contract.rb`;
- `app/contracts/wiki_pages/create_contract.rb`;
- `app/contracts/wiki_pages/update_contract.rb`;
- `app/services/wiki_pages/create_service.rb`;
- `app/services/wiki_pages/update_service.rb`;
- `app/services/wiki_pages/set_attributes_service.rb`;
- `app/controllers/wiki_controller.rb` (create, update, stale-object rescue);
- `lib/open_project/plugins/acts_as_op_engine.rb`;
- `config/constants/api_patch_registry.rb`;
- `lib/api/open_project_api.rb`;
- `lib/api/root_api.rb`;
- `lib/api/errors/error_base.rb`;
- `lib/api/errors/conflict.rb`;
- `lib/api/caching/cached_representer.rb`;
- `lib/api/decorators/formattable.rb`;
- `lib/api/v3/work_packages/work_packages_api.rb`;
- `lib/api/v3/utilities/endpoints/create.rb`;
- `lib/api/v3/utilities/endpoints/update.rb`;
- `lib/api/v3/projects/projects_api.rb`;
- `modules/documents/lib/open_project/documents/engine.rb`;
- `modules/documents/lib/api/v3/documents/documents_api.rb`;
- `modules/budgets/lib/budgets/engine.rb` as the in-tree example of
  `add_api_endpoint "API::V3::Projects::ProjectsAPI", :id`.

The review did not re-run the live endpoint table from the issue. The gap
document already accepted that table as evidence. The source reading was used
to judge whether the *design* matches the application, not to re-prove that
the wiki API stub exists.

A `WikiPageQuery` class was not found in the 17.6.0 files that were read, and
public search of the upstream tree did not show a `Queries::WikiPages`
collection query comparable to documents. That negative finding is used below
when judging the collection implementation shape. It is a source reading, not
a complete clone of the OpenProject repository.

## Overall verdict

```text
product_direction: accepted
implementation_design: not_accepted_for_coding
design_task: remains in_progress
```

The product direction is right. Closing the gap with a separate OpenProject
plugin that extends API v3, keeps standard wiki permissions, writes only
through `WikiPages::CreateService` and `WikiPages::UpdateService`, and reuses
the already-paid custom-image cost is the only considered path that meets the
issue criterion without a password and without HTML parsing.

The first implementation design is not yet specified well enough to implement
without hidden context. Four points in the current text would produce a
plugin that does not match the stated contract if an implementer follows the
document literally:

1. `PATCH` is described as requiring `lockVersion` because the upstream
   contract requires it. The upstream contract does not require the client to
   send `lockVersion`. Omitting it is a successful write and a lost update.
2. `modules/documents` is named as the closest write-path pattern. Its PATCH
   handler parses raw JSON and does not implement HAL formattable text or
   `lockVersion`. The work-package create/update endpoints are the correct
   write-path pattern.
3. The collection is specified without page text, while the representer
   extension is specified on the single `WikiPageRepresenter` class. Those
   two statements cannot both be true unless a thinner collection representer
   is also specified.
4. The collection is specified as `ParamsToQueryService`, which in OpenProject
   needs a Query class for the model. 17.6.0 does not expose a wiki-page
   Query comparable to documents. The first slice should paginate
   `WikiPage.visible` scoped to the project's wiki.

Those four points are required amendments to
[`openproject-wiki-api-plugin-design.md`](openproject-wiki-api-plugin-design.md).
They stay on `task.self.design-openproject-wiki-api-access`. They are not a
new backlog item.

The design task's last done criterion, "the human operator has reviewed the
design", is not closed by this document. This document is the requested
agent review. The operator still has to accept or reject the verdict and the
amendments. Until the amendments are in the design document and the operator
has accepted that text, implementation must not start.

## Product-direction verdict

Accepted.

The gap is real, generic, and not fixed by upgrading OpenProject. The issue
criterion is the right criterion. The four directions in the issue were a
field of choice, not a decision. The gap document made the decision, and this
review agrees with it.

| Direction | Issue status | Gap-document decision | Review |
| --- | --- | --- | --- |
| 1. Give the agent a password and parse HTML wiki pages | flagged as breaking the token-only model | Rejected | Agree. Rejected. |
| 2. Extend API v3 with an own module or patch | cleanest result, upgrade burden | Accepted | Agree. Accepted as the solution. |
| 3. Upstream request | cheap and correct | Accepted as a parallel task, not the solution | Agree. |
| 4. Intermediate service that reads the wiki from the database | second access surface | Rejected | Agree. Rejected. |

Reasons this review accepts direction 2:

- It is the only path that can list pages, return full text, create and
  update through the agent's own API token.
- The instance already runs a custom exact-version 17.6.0 image with a
  throwaway-runtime check, a rollback path and a fast code-only rebuild
  layer. The usual argument against an application patch is already paid.
- Authorization can stay on `view_wiki_pages`, `edit_wiki_pages` and
  `manage_wiki`. No second permission model appears.
- Writes through the upstream services keep journals, notifications and
  validation identical to a browser edit, which is what makes human and
  agent edits visible to each other.
- Tag `v17.8.0` and the upstream `dev` branch still omit wiki page text,
  collection and write routes. Waiting for an OpenProject upgrade does not
  close the gap.

The temporary measure of storing knowledge in work packages of a
document-like type is correctly recorded as a half measure. This review does
not promote it to the target. It should stay in place until the plugin is
accepted on the live instance. Moving knowledge back into the wiki is the
affected project's own task, as the design already says.

Shipping the capability as `openproject-wiki_api`, not as a change to
`openproject-replan_overlay`, is accepted:

- the overlay plugin is documented and audited as read-only;
- putting wiki writes inside it would falsify that contract and the
  2026-09-21 project audit;
- wiki access is not resource planning;
- other projects on the same instance need the capability;
- a separate gem can be disabled, rolled back or offered upstream without
  taking the overlay with it.

The RePlan self-dogfood rule that OpenProject mutations go through guarded
helpers, dry-run by default, apply behind `REPLAN_APPLY=1`, stays. This
plugin makes wiki writes possible. It does not make them unreviewed by
default for the RePlan dogfood project. A `wiki` command family in the
OpenProject toolkit is a follow-up, not part of this slice. Other projects
on the instance own their own agent write policy. This review agrees.

## Implementation-readiness verdict

Not accepted for coding.

The design task's third done criterion is:

```text
the API surface, authorization model and write rules are specified well
enough to implement without hidden context
```

That criterion is not met. An implementer who follows the current design
literally will:

- copy `DocumentsAPI#patch`, skip HAL parsing, and fail to require
  `lockVersion` from the client;
- extend `WikiPageRepresenter` in place and then either leak full page text
  into the collection or silently invent a second representer that the
  design never named;
- call `ParamsToQueryService.new(WikiPage, ...)` and discover at runtime
  that there is no wiki-page Query;
- believe that `WikiPages::UpdateContract` already enforces the issue's
  lost-update rule, and therefore not implement it in the payload
  representer.

The rest of the design — gem path, permission table, non-goals, backlog
chain, throwaway-then-live order, privacy boundary — is usable. The four
holes above are enough to withhold implementation approval.

## Review of the gap document

The gap document is accepted as the requirement record.

It states the requirement in the same terms as the issue: list, read full
text, create, edit, bidirectional visibility, token only, no password, no
HTML parsing.

The observation table is accepted as evidence. This review did not re-run
the live calls. The public 17.6.0 source confirms the API-stub part of that
table:

`lib/api/v3/wiki_pages/wiki_pages_api.rb` at `v17.6.0` defines only:

```text
resources :wiki_pages
  route_param :id
    get
    mount AttachmentsByWikiPageAPI
```

`lib/api/v3/wiki_pages/wiki_page_representer.rb` at `v17.6.0` declares only:

```text
property :id
property :title
associated_project
```

The published API schema `docs/api/apiv3/components/schemas/wiki_page_model.yml`
matches that stub: required `title`, read-only `id`, links for attachments
and project, no `text`, no `lockVersion`, no collection path, no write path.

The official FAQ at 17.6, `docs/api/faq/README.md`, still says wiki pages
cannot be updated through the API and can only be retrieved as info.

The gap document's correction of one side claim in the issue is accepted.
The issue says the documents module has no API. Upstream 17.6.0 mounts
`DocumentsAPI` on `API::V3::Root` with collection GET, single GET, PATCH and
attachments, and without POST. That was not verified on the live instance.
The upstream-check task already includes that live re-check. This review
does not treat the documents module as a replacement for the wiki.

The privacy boundary is accepted. The affected private project, its page
names, its work-package identifiers, the instance endpoint and container
names stay out of this repository. This review follows the same boundary.
GitHub issue #1 remains the place where those private identifiers were
written; the issue is in a private repository.

Keeping this gap out of `docs/dev/known-gaps.md` is accepted. That register
tracks gaps between RePlan code, tests and documentation. This gap is in
the external PM system and is tracked by backlog tasks.

## Review of authorization

Accepted, with one precision to write into the design.

No new permission and no new project module is the right rule. The agent's
reach stays editable in the ordinary OpenProject role editor.

Source confirmation at `v17.6.0`:

- `WikiPage.visible` includes the project and merges
  `Project.allowed_to(user, :view_wiki_pages)`.
- `WikiPages::BaseContract#validate_user_edit_allowed` adds
  `:error_unauthorized` on `:base` when the user lacks `edit_wiki_pages`, or
  when the page *was* protected and the user lacks `manage_wiki`.
- `WikiPages::BaseContract#validate_user_protect_permission` forbids changing
  `protected` without `manage_wiki`.
- `WikiPages::CreateContract` additionally requires `model.author == user`.
- `WikiPages::SetAttributesService` sets `model.author = user` inside
  `change_by_system` on create. The plugin does not need to accept `author`
  from the client.

Authentication through API v3 token, with no `accept_key_auth` patch, is
accepted. The wiki controller is not in the 17.6 `accept_key_auth` list. The
plugin must not add it. Token authentication is already how API v3 works.

Precision to add: HTTP status by case, so the throwaway runtime does not
invent a single expected code.

| Case | Expected status | Why |
| --- | --- | --- |
| GET a page the token cannot see | 404 | `WikiPage.visible(current_user).find` raises `RecordNotFound`. OpenProject API maps that to 404 and, for wiki attachments, documents this as an existence-hiding choice. |
| GET collection with `view_wiki_pages` | 200, possibly empty | Visible pages only. |
| GET collection or POST when the wiki module is disabled or `Wiki` is missing | 404 or 422, never an implicit wiki create | Already listed as verification point 4. Pick one and keep it. |
| PATCH/POST without `edit_wiki_pages` | 403 | Contract `:error_unauthorized` becomes `API::Errors::Unauthorized`. |
| PATCH of a protected page without `manage_wiki` | 403 | Same contract. |
| Validation errors (blank title, bad parent, circular parent) | 422 | `ErrorBase.create_and_merge_errors`. |
| Stale or missing required `lockVersion` on PATCH | 409 | See the lockVersion finding below. Missing must be treated as conflict once the payload representer requires the field. |

The design currently says the read-only member should get 403 on create and
update. That is correct for write. It must not be copied onto GET of an
invisible page.

## Review of the API surface

The four routes are the right first slice:

- `GET /api/v3/projects/{id}/wiki_pages`
- `POST /api/v3/projects/{id}/wiki_pages`
- `GET /api/v3/wiki_pages/{id}` (existing, extended representation)
- `PATCH /api/v3/wiki_pages/{id}`

Extending the upstream `WikiPage` resource in place, rather than inventing a
parallel type, is the right default. If upstream later ships the same paths,
the plugin can be removed with little or no client change. That claim holds
only while the plugin stays on those paths. The own-namespace fallback
breaks it. That is discussed under verification points.

The single-page representation is mostly right:

- `_type: WikiPage` already exists;
- `id` and `title` already exist;
- `slug` exists on the model (`acts_as_url :title, url_attribute: :slug`);
- `lock_version` exists; `UpdateContract` refers to it;
- `protected` exists;
- `created_at` / `updated_at` exist;
- `text` is a column on `wiki_pages` since the WikiContent merge;
- `parent` is `acts_as_tree`;
- `author` is `belongs_to :author`;
- attachments links already come from
  `API::V3::Attachments::AttachableRepresenterMixin` on the current
  representer and must not be dropped.

`text` as API v3 formattable is accepted. `API::Decorators::Formattable`
exposes `format`, `raw` and `html`. Clients read and write `raw`. The issue
criterion "no HTML parsing" is about the agent not scraping the web wiki. It
does not forbid the standard formattable `html` field in JSON. The design
should say that agents must use `raw` and must not parse `html`.

Writable attributes in the first slice are accepted as a product choice:

- `title` required on create, not writable on update;
- `text.raw` optional on create, writable on update;
- `parent` link optional on both.

Excluding rename from the first slice is accepted. `WikiPage#update_redirects`
rewrites slugs and wiki redirects when title changes. That is a separate
reviewed behavior.

Precision: `WikiPages::BaseContract` still declares `attribute :title` as
writable. The contract will not reject a title change on update. The payload
representer for PATCH must omit `title` or mark it unwritable. If a client
sends `title` on PATCH, the API must return the standard unwritable-property
error, not rename the page.

`protected` must not be writable in the first slice either, even though GET
returns it. `BaseContract` allows the change for users with `manage_wiki`.
The payload representer must not expose it yet.

`wiki` is required by the contract and must not be a client-supplied field.
On create, the endpoint must set `wiki: @project.wiki`. On update, the page
already has a wiki. This is missing from the writable-attributes table and
must be added as an endpoint-injected attribute.

`author` is required by the contract and must not be a client-supplied
field. Create sets it from the current user in `SetAttributesService`.

## Required amendment 1: PATCH must require `lockVersion` in the plugin

This is the most important technical finding.

The design says:

```text
PATCH requires lockVersion. wiki_pages has a lock_version column. A stale
value returns HTTP 409, the same contract agents already follow for work
packages.
```

The first sentence is the desired API contract. It is not what
`WikiPages::UpdateContract` currently enforces toward the client.

The full update contract at `v17.6.0` is:

```ruby
module WikiPages
  class UpdateContract < BaseContract
    attribute :lock_version do
      if model.lock_version.nil? || model.lock_version_changed?
        errors.add :base, :error_conflict
      end
    end
  end
end
```

That validator fails only when:

- `lock_version` is nil, or
- `lock_version` changed on the in-memory model.

It does **not** fail when the client omits `lockVersion`. In that case the
loaded record still has the database value, `lock_version_changed?` is
false, the contract passes, and `UpdateService` saves. Concurrent human and
agent edits then last-write-wins. That is the lost-update case the issue
criterion is meant to prevent.

Work packages use the same contract shape, but the work-package API parse
path always applies `lockVersion` from the HAL payload. Wiki pages have no
such parse path yet. The plugin has to add it.

If the client sends a stale `lockVersion`, assigning it onto the model makes
`lock_version_changed?` true, the contract adds `:error_conflict`, and
`ErrorBase.create_errors` maps that symbol to `API::Errors::Conflict`,
HTTP 409, identifier `UpdateConflict`. That half of the design is already
true, once the payload actually assigns the field.

`API::RootAPI` also maps `ActiveRecord::StaleObjectError` to
`API::Errors::Conflict`. A race between contract validation and save is
therefore also 409. Verification point 3 is weaker than the design thinks:
the mapping already exists at the API root. The missing piece is requiring
the client to send the field.

Required design text:

```text
WikiPages::UpdateContract does not require the client to send lockVersion.
It only rejects a nil or changed in-memory lock_version. The plugin payload
representer for PATCH must require lockVersion. A missing lockVersion is
409, the same as a stale lockVersion. The plugin must parse the HAL
payload through a payload representer, assign lock_version onto the model,
and only then call WikiPages::UpdateService. It must not copy the
documents PATCH handler that JSON-parses the raw body.
```

The throwaway check "update with a stale lockVersion and expect 409" is
necessary and not sufficient. Add:

- PATCH with a current `lockVersion` and a text change: 200, text changed,
  `lockVersion` incremented;
- PATCH without `lockVersion`: 409, text unchanged;
- PATCH with a stale `lockVersion`: 409, text unchanged.

## Required amendment 2: do not copy Documents API as the write path

The design says:

```text
In-tree reference: modules/documents at v17.6.0. Its DocumentsAPI is the
closest upstream pattern for a text-bearing resource: collection through
ParamsToQueryService, single GET, PATCH through an update service, errors
through create_and_merge_errors.
```

The documents module is a useful *read* reference for a text-bearing
resource. It is a harmful *write* reference for this plugin.

The documents PATCH handler at `v17.6.0` is:

```ruby
patch do
  doc = document
  request_body = JSON.parse(request.body.read)

  result = ::Documents::UpdateService
    .new(user: current_user, model: doc)
    .call(request_body)

  if result.success?
    ::API::V3::Documents::DocumentRepresenter.new(doc, current_user:, embed_links: true)
  else
    fail ::API::Errors::ErrorBase.create_and_merge_errors(doc.errors)
  end
end
```

That path:

- ignores HAL `_links`;
- does not use a payload representer;
- does not implement `lockVersion`;
- passes a raw JSON hash into the service.

Wiki create and update need HAL `_links.parent`, formattable `text.raw`,
required `lockVersion` on update, and endpoint-injected `wiki` on create.

The in-tree write pattern to name in the design is the work-package API:

```ruby
post(&::API::V3::Utilities::Endpoints::Create.new(...).mount)
patch(&::API::V3::Utilities::Endpoints::Update.new(...).mount)
```

Those helpers already know how to parse a payload representer, call the
model service, and render the representer or merge contract errors.

Required design text:

```text
Write path: API::V3::Utilities::Endpoints::Create and
API::V3::Utilities::Endpoints::Update, with a wiki-page payload
representer, analogous to work packages. DocumentsAPI remains a reference
for a text-bearing GET body only. It is not the PATCH implementation
pattern.
```

The gem layout must name the write endpoint files, not only the collection
representer, the project collection API and the payload representer. At
minimum the layout should also list:

```text
lib/open_project/wiki_api/engine.rb
lib/api/v3/wiki_pages/wiki_pages_by_project_api.rb
lib/api/v3/wiki_pages/wiki_page_collection_representer.rb
lib/api/v3/wiki_pages/wiki_page_payload_representer.rb
```

and state that PATCH is mounted by `add_api_endpoint` on
`WikiPagesAPI`, `:id`, using the Update endpoint helper, not a raw
`JSON.parse` block. If the Update helper needs an explicit parse service,
that file must be named too.

The engine must still call `register` for the gem, as the overlay plugin
does, even though this gem adds no project module and no permission. The
overlay throwaway test already showed that Bundler can install a gem that
OpenProject never loads unless `Gemfile.plugins` requires the entry file
and the engine registers. The design mentions the `require` lesson and
omits `register`. Both are required.

## Required amendment 3: collection text versus representer extension

The design says two things that cannot both be true without a third
construct:

1. `GET /api/v3/projects/{id}/wiki_pages` returns elements with metadata and
   links, without text.
2. `extend_api_response(:v3, :wiki_pages, :wiki_page)` adds `text` to
   `WikiPageRepresenter`.

`extend_api_response` at `v17.6.0` constantizes
`::API::V3::WikiPages::WikiPageRepresenter` and `instance_eval`s the block
onto that class. Every caller of that representer then serializes `text`.

If the collection uses `WikiPageRepresenter`, the collection includes full
page text, including `html`. That may be acceptable as a first-slice
simplification. It is not what the current table says.

If the collection must omit text, the design must name a second representer
for collection elements, for example a `WikiPageCollectionItemRepresenter`
or a collection that embeds only `id`, `title`, `slug`, `lockVersion`,
timestamps and links. The gem layout currently lists
`wiki_page_collection_representer.rb` but does not say whether its elements
reuse the show representer.

Required design choice, one of:

- Option A, keep "collection without text": specify a thin element
  representer and forbid it from using the extended show representer.
- Option B, drop "without text": collection elements may include formattable
  `text`. Simpler. Heavier payloads. Still meets the issue criterion,
  because the criterion requires that the agent can get the list and can
  read full text; it does not require the list to omit text.

This review recommends option A only if payload size matters for the
affected project. Otherwise option B is less fragile. The design must pick
one before coding.

`CachedRepresenter` on `WikiPageRepresenter` makes this sharper. Properties
added through `extend_api_response` participate in `to_json`. The cache key
includes the represented model (`json_key_part_represented` returns
`[represented]`). A text edit touches `updated_at` and `lock_version`, so
the cache is expected to miss. Verification point 1 remains worth running.
It is not a reason to reject in-place extension.

If a thin collection representer is introduced, it should probably disable
caching or use its own key. That is an implementation detail to record once
option A is chosen.

## Required amendment 4: do not use `ParamsToQueryService` for wiki pages

The documents collection does this:

```ruby
query = ParamsToQueryService
  .new(Document, current_user)
  .call(params)
```

That works because documents have a Query. The review did not find a
comparable `WikiPageQuery` / `Queries::WikiPages` in the 17.6.0 wiki API
surface. The bundled `wikis` module queries that exist in 17.6 are about
wiki *page links* to external or internal providers, not about listing
internal `WikiPage` rows for a project.

The first slice does not need filters, columns or saved queries. It needs
the visible pages of one project's wiki.

Required design text:

```text
Collection implementation: WikiPage.visible(current_user)
.where(wiki_id: project.wiki.id) with the standard API v3 offset
pagination representer. Do not introduce a Query class in the first slice.
Do not call ParamsToQueryService for WikiPage.
```

Parent/child order may be title order (`acts_as_tree` already orders by
title) or id order. Pick one and write it down. Title order is the better
default for a human-facing wiki list.

## Review of implementation hooks

`add_api_endpoint`, `extend_api_response` and `add_api_path` are the right
plugin hooks. They are defined on `OpenProject::Plugins::ActsAsOpEngine`.

`add_api_endpoint` stores a block in `Constants::APIPatchRegistry` against a
class name and an optional path. The path `:id` is stored as `":id"`. The
budgets engine uses exactly this against `ProjectsAPI`, `:id`, to mount
`BudgetsByProjectAPI`. Mounting wiki collection and create under
`ProjectsAPI`, `:id` is therefore a proven shape.

`WikiPagesAPI` at 17.6.0 already has:

```ruby
route_param :id, type: Integer, desc: "Wiki page ID" do
  get do
    ::API::V3::WikiPages::WikiPageRepresenter.new(wiki_page, current_user:, embed_links: true)
  end
  mount ::API::V3::Attachments::AttachmentsByWikiPageAPI
end
```

Mounting PATCH into that same `:id` block is the intended use of
`add_api_endpoint "API::V3::WikiPages::WikiPagesAPI", :id`. Verification
point 2 remains valid: confirm on 17.6.0 that this patches into the existing
`route_param :id` rather than creating a second `:id` namespace that
shadows GET. The budgets precedent makes success likely. The throwaway
runtime must still prove GET, PATCH and attachments on the same id.

`OpenProjectAPI.inherited` applies unscoped patches at class load. Scoped
`:id` patches are the ones that need the throwaway proof. This review did
not locate a second public file that shows the `route_param` evaluation
calling `apply_patches(":id")`; the budgets plugin is the working in-tree
evidence that the `:id` form functions for `ProjectsAPI`. WikiPagesAPI is a
smaller class, so the same hook should apply, and the throwaway test is the
proof.

The design's fallback if point 1 or 2 fails is:

```text
/api/v3/wiki_api/...
```

with an own representer.

That fallback preserves authorization, write rules and the acceptance
criterion. It does **not** preserve "the plugin is removed and clients keep
working". Own-namespace paths are not what upstream would add. If the
throwaway runtime has to take the fallback, the drop-in claim must be
removed from the design and from client expectations *before* any client is
written and *before* live deployment.

Required design text:

```text
The own-namespace fallback is an emergency exit from the throwaway
runtime, not a second supported public surface. If it is taken, stop
claiming drop-in replacement of a future upstream API, record the actual
paths in this design, and implement clients against those paths only.
```

## Review of write rules and journals

Accepted, once lockVersion is required by the payload representer.

`WikiPages::CreateService` and `WikiPages::UpdateService` at 17.6.0 are
thin subclasses of `BaseServices::Create` / `BaseServices::Update` with
`Attachments::ReplaceAttachments`. They do not bypass contracts.

`WikiPage` includes `acts_as_journalized` and registers `text` as a
`wiki_diff` journal field. An agent edit through the update service
therefore appears in page history with the token user as author, which is
exactly the bidirectional-visibility requirement.

The rule "the plugin never calls `save` on the model and never writes SQL"
is accepted and should stay in the scaffold check.

`WikiController#update` still rescues `ActiveRecord::StaleObjectError` for
the HTML form and renders 422 HTML. That is the browser path. The API path
must keep 409 JSON. Do not copy the controller rescue.

Create must pass `wiki:`. `WikiController#create` does:

```ruby
attachable_create_call ::WikiPages::CreateService,
  args: permitted_params.wiki_page.to_h.merge(wiki: @wiki)
```

The plugin POST must do the equivalent with `@project.wiki`. If `@project.wiki`
is nil, fail with 404 or 422. Do not create a `Wiki` record as a side
effect of POST.

## Review of non-goals

Accepted.

Page deletion, rename, protect, history routes, wiki menu items, enabling
the wiki module, external wiki providers, password/session/HTML, a database
sidecar, and RePlan canonical/CLI changes are correctly out of the first
slice.

Attachments are correctly left to upstream `AttachmentsByWikiPageAPI`. The
extended representer must keep the existing attachment links.

The review adds one non-goal that is implied but not listed: do not add a
saved Query, filter or schema endpoint for wiki pages in the first slice.

## Review of packaging and the existing plugin path

Accepted as the deployment vehicle.

`scripts/build-openproject-plugin-image.sh` currently hardcodes
`openproject-replan_overlay`, copies only that directory, and writes a
`Gemfile.plugins` with one gem. Generalizing it to every directory under
`integrations/openproject/plugins/` with one `Gemfile.plugins` entry per
gem is the right change. `deps-base`, `code` and `auto` modes stay. The
first image that adds the new gem needs a `deps-base` rebuild because
`Gemfile.lock` changes. Later Ruby-only changes use the `code` layer.

The overlay lessons that must be copied, not rediscovered:

- `Gemfile.plugins` must `require` the entry file explicitly;
- the image is the exact OpenProject builder and slim pair of the live
  instance, currently 17.6.0 / 17.6.0-slim;
- `web`, `worker`, `cron` and `seeder` use the same image;
- the plugin engine must `register` itself;
- throwaway runtime before live;
- live install in a maintenance window with backup and the documented
  rollback path from
  [`openproject-plugin-live-installation-result.md`](openproject-plugin-live-installation-result.md).

The scaffold check today is overlay-specific: it asserts overlay files,
`view_replan_overlay`, the project menu and overlay views. Extending it to
a second gem is real work, not a one-line glob. The implementation task
already includes that. The check for the wiki gem should assert:

- gemspec, engine, entry file and version parse;
- `register` is present and no new permission or project module is
  declared;
- no `accept_key_auth` patch;
- no direct `save`, `update_column` or SQL;
- the four routes and the representer extension (or the chosen collection
  representer) are declared;
- PATCH payload requires `lockVersion`.

## Review of the throwaway-runtime plan

Direction accepted; the current overlay script is not a sufficient vehicle
as-is.

`scripts/check-openproject-plugin-throwaway-runtime.sh` today:

- builds the overlay image;
- boots disposable PostgreSQL, Memcached and OpenProject;
- runs production seeder;
- runs a Rails runner that checks overlay routes, permission, menu and
  overlay JSON loading.

It does not create an API token, does not HTTP-call API v3, does not seed a
wiki, and does not distinguish a read-only member from an editor.

The wiki verification plan lists those HTTP cases. They have to be added as
real HTTP calls with an API token, not as another Rails runner that
instantiates representers in-process. In-process calls would not prove
token authentication, Grape route patching, or content-type handling.

The throwaway plan must include, as HTTP:

1. editor token: list pages;
2. editor token: GET one page, assert `text.raw` is present;
3. editor token: POST a page, assert 201 or 200 and that GET returns the
   text;
4. editor token: PATCH with current `lockVersion`, assert text changed;
5. editor token: PATCH without `lockVersion`, assert 409 and unchanged
   text;
6. editor token: PATCH with stale `lockVersion`, assert 409 and unchanged
   text;
7. read-only token: GET allowed pages succeeds; POST and PATCH return 403;
8. journal of the PATCH shows the token user as author;
9. overlay plugin routes still work in the same image;
10. the four design verification points, including missing-wiki create
    behavior;
11. attachments GET/POST on the extended wiki page still work, so the
    `:id` patch did not shadow `AttachmentsByWikiPageAPI`.

Do not touch the private live instance in this task. That constraint in the
task description is accepted.

## Review of live acceptance

Accepted.

Live deployment stays behind throwaway proof, a maintenance window, backup
and rollback, and the same exact-version image for all Rails services.

Acceptance is the issue criterion, executed by the agent with its own
token, plus a human confirming the agent edit in the browser and the agent
confirming a human edit through the API.

Closing GitHub issue #1 belongs on the live-acceptance task, not on design
or implementation.

Recording a sanitized result document, without the private project, page
names or live identifiers, is required.

## Review of the backlog

Accepted as a chain, with process notes.

| Task | Dates | Review |
| --- | --- | --- |
| `task.self.design-openproject-wiki-api-access` | 2026-09-21, `in_progress` | Correct owner of this review and of the required design amendments. Do not close it until the amendments are in the design document and the operator has accepted that text. |
| `task.self.check-upstream-openproject-wiki-api-roadmap` | 2026-09-22 | Correct as `preferred` research. It must not block the plugin. Live re-check of the documents API belongs here. |
| `task.self.implement-openproject-wiki-api-plugin` | 2026-09-23 to 2026-09-25 | Must not start until the design task is accepted. Scope already includes the gem, the scaffold check and the multi-plugin image helper. After the amendments it also includes required `lockVersion` in the payload representer, the chosen collection representer, and Create/Update endpoint helpers instead of Documents-style PATCH. |
| `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime` | 2026-09-28 to 2026-09-29 | Correct gate before live. Must grow from Rails runner to HTTP-with-token proof. |
| `task.self.deploy-openproject-wiki-api-plugin-live` | 2026-10-05 to 2026-10-06 | Correct. Dates already moved to 2026-W41 so they do not collide with the throwaway runtime on `resource.environment.local-dev` in 2026-W40. |

Dependencies:

- `dependency.wiki-api-design-before-upstream-check`: `preferred`, open.
  Correct.
- `dependency.wiki-api-design-before-plugin-implementation`: `required`,
  `required_state: accepted`. Correct, and this review is why the
  predecessor is not yet `accepted`.
- `dependency.wiki-api-plugin-before-throwaway-runtime`: `required`.
  Correct.
- `dependency.wiki-api-throwaway-runtime-before-live-deployment`:
  `required`. Correct.

None of these tasks gates `task.self.prepare-release-story`. That is
accepted. Wiki API access is an OpenProject-side capability for projects on
the instance, including but not limited to RePlan dogfood. It is not a
public OSS release blocker.

All five tasks carry `openproject_sync_pending` and sanitized fixture work
package ids `2156` to `2160`. That follows the Step 37 precedent. Live
OpenProject was unreachable. Creating and binding the live work packages
through guarded helpers when access returns remains required. This review
does not pretend those live work packages exist.

Resource demands as planned in Step 44: 9 maintainer hours, 15 agent-hours,
2.25 reviewer hours, three local-dev slot reservations. This review does
not recalculate them. Two observations, already partly in Step 44:

- implementation plus throwaway may be tight if the `:id` PATCH hook fails
  and the own-namespace fallback has to be designed into the gem during
  throwaway week;
- agent hours are booked on `resource.ai.codex.local` because it is the
  only modeled AI execution resource. The design work and this review were
  not executed by that named resource. That is an open resource-model
  question, not a wiki-API defect.

This review does not create a sixth task. The required amendments belong on
the existing design task.

## Review of calibration Step 44

Accepted as the planning record of the product decision.

This review adds Step 45 in the same journal as the record of the design
review. Step 44's product decision is unchanged. Step 44's implication that
the implementation design is ready to code is withdrawn until the
amendments land.

## Should-fix items

These are not as hard as the four required amendments. They should still be
written into the design before implementation, because they are cheap in
prose and expensive when discovered in the throwaway runtime.

1. State HTTP statuses per case, as in the authorization table above.
2. State that create injects `wiki: @project.wiki` and `author` from the
   current user, and that neither is a client-writable attribute.
3. State that PATCH title and PATCH protected are unwritable in the first
   slice even though the base contract would allow them for some users.
4. State that formattable `html` may be returned and that agents must use
   `raw`.
5. Add attachments-still-work to the throwaway HTTP list.
6. Add PATCH-without-lockVersion to the throwaway HTTP list.
7. Name `register` without a project module in the engine.
8. If the own-namespace fallback is taken, drop the drop-in claim before
   live.
9. Collection sort order: title, unless a reason for id order is recorded.
10. Scaffold check must be gem-aware, not overlay-only with a copied
    filename list.

## Non-blocking observations

These do not have to change the design before implementation.

- `WikiPages::BaseContract` still comments that it validates the page and
  its associated wiki content as one. That is leftover from the WikiContent
  merge. Text now lives on `WikiPage`. The plugin talks to `WikiPage` only.
- `WikiPage#version` is the journal version, not `lock_version`. Do not
  expose journal version as `lockVersion`.
- The official OpenAPI schema for `WikiPage` will remain stale until
  upstream documents the new fields. The first slice does not need to patch
  the upstream OpenAPI files inside the image. A short plugin README of the
  four routes is enough.
- Plugin license: the overlay gemspec is MIT. OpenProject is GPLv3. This
  review does not decide a license change. Follow the overlay precedent
  unless the operator chooses otherwise.
- The contradiction "wiki capability has nothing to do with RePlan" versus
  hosting the gem in the RePlan repository is operational, not a defect.
  The dogfood instance is where the custom image is already built. The gem
  can later be offered upstream or split. That split is not this slice.
- Returning knowledge from work packages to wiki pages in the affected
  private project is out of this repository's backlog, as designed.
- Resource-catalog modeling of AI execution agents remains an open RePlan
  model question from Step 44.

## Privacy

This review records no live OpenProject URL, no live token, no private
project identifier, no private work-package id, no page title from the
affected project, and no container name.

Sanitized fixture ids `2156` to `2160` are public example ids already in
`examples/replan-self` and are repeated here only as backlog references.

GitHub issue #1 is referenced by number. The issue body contains private
identifiers; those identifiers are not copied into this document.

## Proposals

These are the concrete changes this review asks for. They are all changes
to [`openproject-wiki-api-plugin-design.md`](openproject-wiki-api-plugin-design.md),
except the calibration and index pointers that record the existence of this
review.

### Proposal 1. Change the design status line

Keep "accepted direction". Replace "first implementation design, open for
review" with "first implementation design, reviewed, amendments required
before coding".

Point at this document.

Do not set the design task to `accepted` until the amendments below are in
the design and the operator has accepted that text.

### Proposal 2. Rewrite the write-path reference

Delete the claim that `DocumentsAPI` is the closest pattern for PATCH.

Write:

- GET formattable body: documents and work packages are both valid
  references;
- collection: not documents `ParamsToQueryService`; use
  `WikiPage.visible` scoped to `project.wiki`;
- create and update: `API::V3::Utilities::Endpoints::Create` and `Update`
  with a payload representer, as work packages do.

### Proposal 3. Make `lockVersion` a plugin rule, not a contract myth

Add a subsection that quotes, or at least states, the actual
`UpdateContract` behavior: nil or changed in-memory `lock_version` is
conflict; omitted client field is currently a successful write.

State the plugin rule: PATCH payload requires `lockVersion`; missing and
stale are both 409; text must be unchanged on 409.

Add the three throwaway PATCH cases: current, missing, stale.

### Proposal 4. Resolve collection text

Pick option A (thin collection representer, no text) or option B (text
allowed in collection elements). Name the representer files that follow
from that choice.

This review's default if the operator does not care about payload size:
option B, because it is one representer and cannot drift. If the operator
wants the current table to stay true, option A.

### Proposal 5. Write the create injection rules

On POST:

- require `title`;
- accept optional `text.raw` and optional `_links.parent`;
- inject `wiki: @project.wiki`;
- inject author via the existing `SetAttributesService`;
- if wiki is missing, 404 or 422, never create a wiki.

On PATCH:

- require `lockVersion`;
- accept `text.raw` and `_links.parent`;
- reject `title`, `protected`, `slug`, `author`, `wiki`.

### Proposal 6. Write HTTP status by case

Use the authorization table in this review.

### Proposal 7. Constrain the own-namespace fallback

Emergency exit from throwaway only. If taken, drop the drop-in-upstream
claim before any client or live deploy.

### Proposal 8. Expand the throwaway plan to HTTP with tokens

Do not treat the current overlay Rails runner as sufficient. List the HTTP
cases in this review, including attachments and PATCH without
`lockVersion`.

### Proposal 9. Keep the backlog chain

Do not add a task. Do not start implementation. Apply proposals 1–8 on the
design task, then operator-accept, then implement.

### Proposal 10. Record this review in the journal and indexes

This document is the review record. Calibration Step 45, README and
`docs/README.md` point here so the review is not chat-only.

## What this review does not close

- `task.self.design-openproject-wiki-api-access` stays `in_progress`.
- GitHub issue #1 stays open.
- Live OpenProject sync of the five tasks stays pending.
- Upstream wiki-API roadmap stays unchecked.
- The plugin gem still does not exist.
- The temporary work-package-as-document measure in the affected project
  stays in place.
- The 2026-09-21 project audit findings outside this slice stay as they
  were after Step 44: live OpenProject alignment unverified, two other
  `in_progress` tasks past planned finish, dependency and proposal hygiene
  items still open.

## Mapping of design claims to source

This table is the evidence index for the findings above.

| Design claim | 17.6.0 source | Review |
| --- | --- | --- |
| GET wiki page returns only id, title, project | `wiki_page_representer.rb`, `wiki_page_model.yml` | Confirmed |
| No collection route | `wiki_pages_api.rb` | Confirmed |
| No POST/PATCH | `wiki_pages_api.rb`; FAQ "Can I update a wiki page via API?" | Confirmed |
| Attachments by wiki page work | `wiki_pages_api.rb` mounts `AttachmentsByWikiPageAPI` | Confirmed as source; live already evidenced in the issue |
| `text` lives on `WikiPage` | `app/models/wiki_page.rb`; searchable column `text`; journal field `text` | Confirmed |
| `lock_version` column exists | `UpdateContract` attribute `:lock_version` | Confirmed |
| PATCH requires client `lockVersion` | `UpdateContract` only checks nil/changed | **False as written.** Must be a plugin payload rule |
| Stale lock maps to 409 | `ErrorBase` maps `:error_conflict` to `Conflict` code 409; `RootAPI` maps `StaleObjectError` to `Conflict` | Confirmed once the field is assigned |
| Create/Update services exist | `wiki_pages/create_service.rb`, `update_service.rb` | Confirmed |
| Create sets author | `SetAttributesService#set_default_attributes` | Confirmed |
| Create needs `wiki:` | `BaseContract#validate_wiki_is_set`; controller merges `wiki: @wiki` | Confirmed; missing from design write rules |
| `view_wiki_pages` for read | `WikiPage.visible` | Confirmed |
| `edit_wiki_pages` for write | `BaseContract#validate_user_edit_allowed` | Confirmed |
| `manage_wiki` for protected | same method, `protected_was` | Confirmed |
| Rename has redirects | `WikiPage#update_redirects` | Confirmed; excluding rename is right |
| Journalized | `acts_as_journalized` | Confirmed |
| `extend_api_response` extends one representer class | `acts_as_op_engine.rb` | Confirmed; conflicts with "collection without text" |
| `add_api_endpoint` on ProjectsAPI `:id` | budgets engine | Confirmed as a working pattern |
| `add_api_endpoint` on WikiPagesAPI `:id` | registry supports it; WikiPagesAPI is small | Likely; throwaway must prove it |
| Documents is the PATCH pattern | `documents_api.rb` uses `JSON.parse` | **Rejected as a write pattern** |
| `ParamsToQueryService` for wiki pages | documents have a Query; wiki list API does not | **Rejected for first slice** |
| Formattable `text` | `API::Decorators::Formattable` | Confirmed |
| Cached representer key includes the model | `CachedRepresenter#json_key_part_represented` | Confirmed; verification point 1 still worth running |
| Upgrade to 17.8/dev closes the gap | gap document compared those trees | Accepted as already recorded; this review did not re-diff 17.8 |
| Overlay must stay read-only | overlay README, engine, 2026-09-21 audit | Confirmed; separate gem is right |

## Done criteria for this review document

- the product direction is accepted or rejected in writing;
- the implementation design is accepted or withheld in writing;
- every required amendment is stated with source evidence, not only as a
  preference;
- should-fix and non-blocking items are separated from required amendments;
- proposals are concrete enough to apply to the design document;
- privacy boundary of the gap document is preserved;
- the review is indexed from the documentation set and from the calibration
  journal so it is not chat-only.

Those criteria are met by this document. They do not accept the
implementation design for coding.
