# OpenProject wiki API plugin implementation review

Status: dated record (2026-09-21). Index: [OpenProject design index](openproject-design-index.md).

Original status: gate 1 closed. First pass `changes_required`; re-review of the fixes
`accepted_with_notes`; task accepted by the operator.

Correction, 2026-09-21: the throwaway runtime later found a boot-blocking
defect that both passes of this review missed, the module name
`OpenProject::WikiApi`. The "eleven of eleven pass on reading" statement below
was therefore wrong about bootability. See
[`openproject-wiki-api-throwaway-runtime-test.md`](openproject-wiki-api-throwaway-runtime-test.md).

Date: 2026-09-21.

Reviewer: the design author, acting as `resource.ai.review-assistant` under the
handoff protocol in
[`openproject-wiki-api-plugin-design.md`](openproject-wiki-api-plugin-design.md).
The implementation was written by a different AI agent.

Related task: `task.self.implement-openproject-wiki-api-plugin`.

Reviewed referent: commit `5057064` on branch `wiki-api-plugin`, the
implementer's work committed unchanged by the reviewer.

OpenProject sync: pending, unchanged.

## Verdict

```text
gate_1_implementation_review: changes_required
task: back to in_progress
```

The implementation follows the design closely and the hard parts are right:
upstream services through the `Create` and `Update` helpers, forced
`lock_version`, a thin text-free list element, wiki injection, no new
permission. Nine of eleven conformance items pass on reading.

Two defects block acceptance. Both are load-order defects in how the gem's
files reach Ruby, not defects in the API logic. As submitted, OpenProject is
expected not to boot with this gem installed. Neither defect can be seen by the
scaffold check, `go test` or the MVP smoke, which is why they all pass.

This is a static review. No OpenProject image was built and no runtime was
started; no OpenProject image exists on the review machine, and the runtime
proof belongs to the throwaway task. Every finding below is backed by upstream
source at tag `v17.6.0`. Runtime claims are marked as expectations.

## Method

Read in full: every file of the new gem, the README, the rewritten scaffold
check, the diff of the image build helper, calibration Step 48, the task,
evidence and audit changes.

Run on the submission: `./scripts/check-openproject-plugin-scaffold.sh`,
`go test ./...`, `./scripts/smoke-mvp.sh`, `replan validate`, `bash -n` on the
build helper. All pass.

Upstream `v17.6.0` files read for this review, beyond those of the design
review: `config/application.rb`, `config/initializers/zeitwerk.rb`,
`lib/open_project/plugins/acts_as_op_engine.rb`, `modules/budgets` engine,
`lib/api/utilities/payload_representer.rb`,
`app/services/api/v3/parse_resource_params_service.rb`,
`lib/api/utilities/endpoints/bodied.rb`, `lib/api/decorators/single.rb`,
`lib/api/decorators/linked_resource.rb`,
`lib/api/decorators/formattable_property.rb`,
`lib/api/errors/unwritable_property.rb`,
`lib/api/caching/cached_representer.rb`, `lib/api/v3/news/*`,
`config/initializers/permissions.rb`.

## Blocking findings

### B1. `engine.rb` requires the API classes before OpenProject can load them

`lib/open_project/wiki_api/engine.rb` lines 3 to 6:

```ruby
require_relative "../../../api/v3/wiki_pages/wiki_page_list_item_representer"
require_relative "../../../api/v3/wiki_pages/wiki_page_collection_representer"
require_relative "../../../api/v3/wiki_pages/wiki_page_payload_representer"
require_relative "../../../api/v3/wiki_pages/wiki_pages_by_project_api"
```

The engine file is loaded by `Gemfile.plugins` through `Bundler.require`.
Upstream `config/application.rb` runs
`Bundler.require(*Rails.groups(:opf_plugins))` on line 10, before
`class Application` on line 13 and before `config.autoload_lib` on line 15. At
that moment no autoloader exists and the core `API` namespace from `lib/api`
cannot be resolved.

The first required file opens `module API`, which at that point creates a new
empty module, and then evaluates
`class WikiPageListItemRepresenter < ::API::Decorators::Single`.

Expected result: `NameError: uninitialized constant API::Decorators` during
`Bundler.require`. OpenProject does not boot. All Rails services share the
image, so web, worker, cron and seeder all fail.

There is a second effect even if the error were avoided: the plugin would
define a bare `API` module before Zeitwerk sets up the core `lib/api`
namespace.

The requires are also unnecessary. `ActsAsOpEngine` does
`config.eager_load_paths += Dir["#{config.root}/lib"]`, so the gem's `lib` is an
autoload root. The bundled `budgets` module, which the design names as the
reference for this mount, keeps its API classes under `lib/api/v3/budgets/`
and requires none of them in its engine.

The design did not ask for these requires. The gem layout in the design lists
files only. This is an undeclared addition, and calibration Step 48 states that
no deviation was made.

Required change: delete the four `require_relative` lines. The constants are
referenced only inside `add_api_endpoint` blocks and the
`extend_api_response` block, all of which are evaluated late.

### B2. `WikiPageUpdatePayloadRepresenter` cannot be autoloaded

`wiki_page_payload_representer.rb` defines two constants:
`WikiPagePayloadRepresenter` and `WikiPageUpdatePayloadRepresenter`. Zeitwerk
maps one file to one constant. It can find the second constant only if the
first one happened to be loaded earlier.

Today B1 hides this, because the file is force-required. Once B1 is fixed, the
`PATCH` block in `engine.rb` references
`::API::V3::WikiPages::WikiPageUpdatePayloadRepresenter` when the patch is
applied to `WikiPagesAPI`. `OpenProjectAPI` applies patches when the API class
is loaded. If core `lib/api` is loaded before the plugin's payload file, the
lookup fails with `NameError`, and there is no
`wiki_page_update_payload_representer.rb` for the autoloader to find.

Expected result: order-dependent boot failure in production, where everything
is eager-loaded, and a reliable `NameError` on the first `PATCH` in a lazy
environment.

Required change: move `WikiPageUpdatePayloadRepresenter` to its own file,
`lib/api/v3/wiki_pages/wiki_page_update_payload_representer.rb`. The design's
gem layout gains one file; that is an accepted, declared deviation.

## Should-fix findings

These do not block on their own. They should be fixed in the same pass,
because each is cheap now and costs a throwaway-runtime cycle later.

### S1. The `writeable:` lambda on `title` is unproven and misleading

```ruby
property :title,
         writeable: ->(*) { !update_payload? }
```

Upstream `PayloadRepresenter.included` skips any property whose `:writeable`
responds to `call`, so this property loses the standard
`UnwritablePropertyFilter`. Whether Representable evaluates a callable
`:writeable` during parsing, and in which execution context, is not
established by any upstream use. If it is evaluated against the represented
struct instead of the decorator, `update_payload?` is unknown there and the
expression is always true.

The rename guard that actually works is `reject_unwritable_properties!` in
`from_hash`. The lambda is therefore either dead or a second mechanism with
unknown behavior. Remove it and keep the explicit guard as the single
mechanism. Throwaway case 7 then proves one thing, not two.

### S2. Collection `GET` has no explicit permission check

A project member without `view_wiki_pages` gets `200` with an empty list,
because `WikiPage.visible` filters everything out. The response also confirms
that the project has a wiki. The `budgets` reference calls
`authorize_in_project` before listing.

This is first a design omission: the status table did not name the case. The
design is amended by this review: collection `GET` without `view_wiki_pages` is
`403`. Required change: `authorize_in_project(:view_wiki_pages, project: @project)`
for the collection `GET`, after the 404 wiki check. `POST` stays guarded by the
contract.

### S3. The `update` link is unconditional on a cached representer

`link :update` is rendered for every reader, including read-only members. In
API v3 an action link signals that the action is allowed. `WikiPageRepresenter`
includes `CachedRepresenter`, so a per-user condition must use the cache-aware
form, `link :update, cache_if: -> { ... }`, not a plain `next unless` inside the
block, which would cache one user's answer for all. Condition:
`current_user.allowed_in_project?(:edit_wiki_pages, represented.project)`.

### S4. The scaffold check could not have caught B1 or B2

It asserts that strings exist. Add two structural assertions so the class of
defect cannot return:

- the engine must not `require` or `require_relative` anything under `api/`;
- every `class` defined under `lib/api/` must match its file name, one
  constant per file.

### S5. The submission was not committed

The handoff says "work on a branch". The branch existed, but all work was
uncommitted in the working tree, so the review had no fixed referent until the
reviewer committed it. Future submissions should arrive as commits.

## Notes

- N1. The `after_validation` added to the `WikiPagesAPI` `:id` block also runs
  for the upstream `GET` and for the mounted attachments API. It is one extra
  visible-scoped query and the same 404 behavior. Acceptable. An alternative is
  `instance_generator: ->(*) { wiki_page }` on the `Update` helper, reusing the
  upstream helper method.
- N2. `lockVersion` sent on `POST` is parsed and then rejected by the create
  contract as read-only, giving 422. The design says "not applicable". Either
  behavior is acceptable; the README should state which one is implemented.
- N3. Image names in the build helper still default to `...replan-overlay...`
  although the image now carries two gems. Cosmetic. Renaming would break the
  tagged dependency base on the live host, so leave it and say so in the
  script.
- N4. In `auto` mode an existing overlay-only dependency image selects the
  fast `code` path, and the new gem is then absent from `Gemfile.lock`. The
  final `bundle info` loop catches it, so the build fails loudly rather than
  shipping a silent no-op. Good. README and Step 48 document the required
  `deps-base` rebuild.
- N5. The implementer moved the task to `review_ready` through
  `replan actual add-status` with its own actor and an audit record, and did
  not book actuals ahead of review. That is the intended use of the loop.

## Conformance checklist

| # | Item | Result |
| --- | --- | --- |
| 1 | separate gem; `register` without module, permission or `accept_key_auth` | pass |
| 2 | exactly the four routes, on those paths | pass on reading; unreachable until B1 and B2 are fixed |
| 3 | writes only through the services via `Create` and `Update`; no `save`, SQL, raw `JSON.parse`, `ParamsToQueryService` | pass |
| 4 | `lock_version` forced to nil when absent | pass: `params.reverse_merge(lock_version: nil)` on the `Update` helper |
| 5 | payload exposes only `title` on create, `text`, `parent`, `lockVersion` | pass, with S1 |
| 6 | `wiki` injected; project without wiki is 404; no `Wiki` row created | pass: `module_enabled?(:wiki) && wiki.present?`; `:wiki` is the upstream module name |
| 7 | thin list-item representer, no text, title order, `WikiPage.visible` scope | pass, with S2 |
| 8 | single-page representation keeps upstream attachment links | pass: the extension only adds |
| 9 | status table implemented as written | pass on reading, with S2 amending the table |
| 10 | README documents routes, writable attributes, statuses | pass, with N2 |
| 11 | no private endpoint, token, project name or live identifier | pass: pattern scan clean |

Shared-change rule: only the build helper and the scaffold check were touched
outside the gem, and the overlay assertions are preserved one for one. Pass.

## Design amendments made by this review

The reviewer is also the design author. Three gaps in the design surfaced and
are fixed in the design document in the same change:

1. status table: collection `GET` without `view_wiki_pages` is `403` (S2);
2. gem layout: `wiki_page_update_payload_representer.rb` added, and the rule
   "no `require` of `lib/api` files; one constant per file" written down (B1,
   B2). The design told the implementer what to build but not how OpenProject
   loads plugin code, and that omission is where both blockers come from;
3. throwaway plan: two cases added. The original fourteen never exercised
   `_links.parent`, although `parent` is a writable attribute, and never proved
   that the image boots with both gems before HTTP cases start.

## What must happen before re-review

1. B1 and B2 fixed.
2. S1 to S4 fixed, or each one refused in writing with a reason.
3. The work committed on `wiki-api-plugin`.
4. Step 48's "no design deviation" sentence corrected.
5. The scaffold check, `go test ./...` and `./scripts/smoke-mvp.sh` pass.

Re-review is a diff review against `5057064` and is expected to be short. The
remaining planned AI review amount covers it.

## What this review does not decide

- whether the plugin works at runtime. That is gate 2, on
  `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime`;
- the four design verification points, which stay with the throwaway task;
- operator acceptance of the task, which stays with the operator.

## Re-review of the fixes

Date: 2026-09-21.

Referent: commit `ee451fb` on branch `wiki-api-plugin`, reviewed as a diff
against `5057064`. The submission arrived as a commit this time.

Outcome: `accepted_with_notes`.

| Finding | Fix in `ee451fb` | Result |
| --- | --- | --- |
| B1 | the four `require_relative` lines are gone from `engine.rb`; nothing under `lib/api` is required anywhere in the gem | fixed |
| B2 | `WikiPageUpdatePayloadRepresenter` lives in its own `wiki_page_update_payload_representer.rb`; the parent class resolves lexically inside `API::V3::WikiPages` and is autoloadable | fixed |
| S1 | `property :title` is plain; the `writeable:` lambda is gone; `reject_unwritable_properties!` is the single rename guard | fixed |
| S2 | collection `GET` calls `authorize_in_project(:view_wiki_pages, project: @project)`, the same call the `budgets` reference makes | fixed |
| S3 | `link :update, cache_if: -> { current_user.allowed_in_project?(:edit_wiki_pages, represented.project) }`. Upstream `CachedRepresenter#apply_link_cache_ifs` evaluates the condition with `instance_exec` on the representer, and the work-package representer uses the identical form for its `update` and `delete` links | fixed |
| S4 | the scaffold check forbids engine requires of `api/` files and requires exactly one class per file under `lib/api/`, named after the file with the `API` acronym rule | fixed and proven, see below |
| S5 | work committed | fixed |
| N2 | README states that `lockVersion` on `POST` is rejected by the create contract with 422 | addressed |
| N3 | comment in the build helper explains why the overlay-era image names stay | addressed |
| Step 48 | the "no design deviation" sentence now names the undeclared requires | corrected |

The S4 assertions were mutation-tested by the reviewer on a scratch copy of
the plugins and the check. Each mutation made the check fail with a precise
message, and the unmodified copy passed:

- a `require_relative` of a `lib/api` file put back into `engine.rb`;
- a second class appended to a file under `lib/api/`;
- a class renamed so that it no longer matches its file.

On the real tree the scaffold check, `go test ./...`, `./scripts/smoke-mvp.sh`,
`replan validate` and `replan audit verify` pass.

The diff touches nothing outside the gem, the two shared scripts, the journal
and the task's own tracking records. The review document and the design were
not edited by the implementer.

Notes carried forward to gate 2. None of them is a reason to hold the task:

- R1. Everything in this review is still a reading. Whether the image boots,
  whether the `:id` patch coexists with the upstream `GET` and attachments,
  whether the cached single-page JSON picks up `text` and the conditional
  `update` link, and whether `_links.parent` is writable through the contract
  are throwaway cases 15, 2, 4, 12 and 16. Gate 1 cannot answer them.
- R2. The 404 for a project without a wiki is decided before the 403 for a
  missing `view_wiki_pages`. A project member without that permission can
  therefore tell "no wiki" from "wiki exists". The reviewer asked for this
  order. The project itself is already visible to that member, and the wiki
  module state is visible in the project's own representation, so nothing new
  is disclosed.
- R3. N1 stands: the `after_validation` on the `:id` block adds one
  visible-scoped query to the upstream `GET` and attachments routes.
  Acceptable.

Conformance checklist after the fixes: eleven of eleven pass on reading.

Recommendation to the operator: accept
`task.self.implement-openproject-wiki-api-plugin`. The next task,
`task.self.verify-openproject-wiki-api-plugin-throwaway-runtime`, is where the
plugin is first executed.
