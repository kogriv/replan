# OpenProject wiki API plugin throwaway runtime test

Status: dated record (2026-09-21). Index: [OpenProject design index](openproject-design-index.md).

Original status: completed; the plugin works in a disposable OpenProject 17.6.0; one
boot-blocking defect was found and fixed on the way. Operator acceptance of the
task is open.

Date: 2026-09-21.

Related task: `task.self.verify-openproject-wiki-api-plugin-throwaway-runtime`.

Design:
[`openproject-wiki-api-plugin-design.md`](openproject-wiki-api-plugin-design.md).

Gate 1 review:
[`openproject-wiki-api-plugin-implementation-review.md`](openproject-wiki-api-plugin-implementation-review.md).

OpenProject sync: pending, unchanged. The private live OpenProject instance was
not touched.

## Independence Note

The design's handoff protocol separated author and reviewer on purpose. For
this task the operator assigned the runtime proof to the design author, who
also reviewed the code at gate 1. This document is therefore evidence produced
by an interested party, not an independent gate 2 review.

What compensates: the proof is one repeatable command, every HTTP case records
its requests, status codes and asserted facts as JSON, and the first thing the
proof did was refute the author's own gate 1 verdict.

```sh
./scripts/check-openproject-wiki-api-throwaway-runtime.sh
```

Evidence lands in the ignored directory `tmp/wiki-api-throwaway/`:
`boot.json`, `http-cases.json`, `db-facts.json` and the service logs. API
tokens are throwaway and are still kept out of the evidence files.

## Result

```text
image build with both gems:        ok
seeder, web, worker, cron boot:    ok, no NameError or LoadError in any log
plugins registered:                openproject_replan_overlay, openproject_wiki_api
HTTP cases with API tokens:        15 of 15 pass, 37 requests, 57 checks
database facts after the run:      ok
overlay plugin routes, same image: ok
```

Runtime: OpenProject `17.6.0` builder and `17.6.0-slim` final image, the exact
pair of the live instance; PostgreSQL 17; memcached 1.6; `RAILS_ENV=production`
with eager loading.

## Defect Found By The Runtime

### The plugin module name broke eager loading

The gem named its module `OpenProject::WikiApi`. OpenProject's Zeitwerk
inflector turns every path segment ending in `_api` into `...API`, so the
directory `lib/open_project/wiki_api/` must define `OpenProject::WikiAPI`.

Observed on the first run:

```text
zeitwerk/loader/eager_load.rb: uninitialized constant OpenProject::WikiAPI (NameError)
Did you mean?  OpenProject::WikiApi
```

The failure appears only under eager loading. The asset precompile step of the
image build and the production seeder both passed with the wrong name. A
`rails runner` in production, and therefore the web service, did not. On the
live instance the result would have been Rails services that do not start.

This defect passed the implementer, the gate 1 review and the gate 1
re-review. The reviewer had written the `_api` rule into the scaffold check
for files under `lib/api/` and did not apply it to the gem's own namespace
directory. Gate 1 reported eleven of eleven conformance items as passing on
reading; that report was wrong about bootability, which it had explicitly
listed as something only the runtime could answer.

Fix: the module is `OpenProject::WikiAPI` in `engine.rb`, `version.rb` and the
gemspec. The gem name, the require path `open_project/wiki_api` and the engine
name are unchanged. The scaffold check now requires `module WikiAPI` and
forbids the spelling `WikiApi` anywhere in the gem, with the reason in a
comment.

### Three errors in the proof itself

Found and fixed while getting the proof to run. None of them is a plugin
defect:

- the seed created projects without `workspace_type`, which 17.6.0 requires;
- the HTTP cases called `127.0.0.1` while OpenProject was configured for
  `localhost`, so every request was rejected with 400 by the host-name check.
  The health endpoint is exempt, which is why the boot wait passed;
- two expectations were wrong: attachment upload returns 201, not 200; and a
  page created and edited by one user has one journal, not two, because
  OpenProject aggregates consecutive edits of the same user. The browser
  behaves the same way.

## Design Verification Points

| # | Point | Result |
| --- | --- | --- |
| 1 | properties added through `extend_api_response` on the cached representer | Holds. `text`, `slug`, `lockVersion`, timestamps and links appear; `GET` after `PATCH` returns the new text, so the cache is invalidated; the conditional `update` link is present for the editor, absent for the read-only member, and present again for the editor after the read-only member's cached read (cases 2, 4, 9). |
| 2 | `add_api_endpoint` on `WikiPagesAPI`, `:id` does not shadow upstream routes | Holds. The upstream `GET` and both attachment routes are listed next to the new `PATCH` in `boot.json`, and attachment list and upload work on the same page id (cases 2, 12). |
| 3 | missing and stale `lockVersion` end as HTTP 409 | Holds. Both return 409 `UpdateConflict` and leave the text unchanged (cases 5, 6). |
| 4 | project without a wiki | Holds for both shapes: wiki module disabled, and module enabled with the `Wiki` row deleted. Collection `GET` and `POST` return 404, and the number of `Wiki` rows is unchanged after the run (case 11, database facts). |

The own-namespace fallback was not needed. The drop-in-replacement claim of the
design stands.

## HTTP Cases

Actors: `editor` has `view_wiki_pages` and `edit_wiki_pages`; `reader` has
`view_wiki_pages`; `nowiki` is a member without wiki permissions; `outsider`
has no membership. All four authenticate with their own API token over HTTP
basic auth as `apikey`. No password session and no HTML is involved anywhere.

| Case | What | Requests and status codes | Checks | Result |
| --- | --- | --- | --- | --- |
| 1 | editor lists pages; elements carry no text; title order | editor `GET` 200 | 6 | pass |
| 2 | editor reads one page with `text.raw`, `lockVersion`, `slug`, links | editor `GET` 200 | 8 | pass |
| 3 | editor creates a page; `GET` returns the text | editor `POST` 201, `GET` 200 | 4 | pass |
| 4 | `PATCH` with current `lockVersion` changes text, increments `lockVersion` | editor `PATCH` 200, `GET` 200 | 4 | pass |
| 5 | `PATCH` without `lockVersion`; text unchanged | editor `PATCH` 409, `GET` 200 | 3 | pass |
| 6 | `PATCH` with stale `lockVersion`; text unchanged | editor `PATCH` 409, `GET` 200 | 3 | pass |
| 7 | `PATCH` with `title`, then with `protected`; not renamed | editor `PATCH` 422, `GET` 200, `PATCH` 422 | 4 | pass |
| 8 | editor without `manage_wiki` on a protected page; text unchanged | editor `GET` 200, `PATCH` 403, `GET` 200 | 3 | pass |
| 9 | read-only member lists and reads, no `update` link; writes refused | reader `GET` 200, `GET` 200, `POST` 403, `PATCH` 403; editor `GET` 200 | 6 | pass |
| 10 | token without membership | outsider `GET` 404, `PATCH` 404 | 2 | pass |
| 11 | project without a wiki, both shapes | editor `GET` 404, `POST` 404, `GET` 404, `POST` 404 | 4 | pass |
| 12 | attachments on the same wiki page id | editor `GET` 200, `POST` 201, `GET` 200 | 3 | pass |
| 13 | editor edits a page written by another user | editor `GET` 200, `PATCH` 200 | 2 | pass |
| 16 | child page through `_links.parent`; cross-project parent refused, parent unchanged | editor `POST` 201, `GET` 200, `PATCH` 422, `GET` 200 | 4 | pass |
| 17 | member without `view_wiki_pages` lists the collection | nowiki `GET` 403 | 1 | pass |

Cases 14 and 15 are not HTTP cases. Case 15 is the boot proof above. Case 14,
overlay routes in the same image, is checked in the boot step and again in the
database step.

## Database Facts After The Run

| Fact | Result |
| --- | --- |
| no `Wiki` row was created by the 404 cases | true, 4 rows before and after |
| the page created over the API has the token user as author | true |
| its journal belongs to the token user | true |
| the page written by the admin and edited over the API has journal versions 1 and 2 | true |
| version 1 belongs to the admin and keeps the old text | true |
| version 2 belongs to the token user and carries the new text | true |
| overlay plugin routes registered | `project_replan_overlay`, `project_replan_overlay_work_package` |

This is the bidirectional-visibility requirement of the issue at the data
level: an agent edit is an ordinary history entry under the agent's name,
diffable and revertable by a human, next to the human's own entry.

## Observations For The Live Task

- Journal aggregation: several agent edits in a row on the same page collapse
  into one history entry, exactly like a human's. An agent that wants separate
  history entries has to be separated by another author or by time.
- Attachment upload on wiki pages returns 201. The plugin README's status table
  does not cover attachments because they are upstream routes.
- Every request has to use the configured host name. A client that reaches
  OpenProject through a protected connection must send the `Host` header of the instance, as
  the RePlan helpers already do.
- The first image that contains the new gem needs the dependency base rebuilt
  (`REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE=both` or `deps-base`). Later Ruby-only
  changes, like the module rename above, rebuild in seconds with `code`.
- The wrong module name did not fail the image build. The live procedure must
  therefore not treat a successful build as proof of bootability. Run this
  throwaway check against the exact image tag that will be deployed.

## What This Test Does Not Prove

- behavior behind the live reverse proxy and perimeter layer;
- behavior with the live instance's data volume, roles and wiki content;
- performance of the collection on a large wiki;
- rename, delete, protect, history routes and menu items, which are non-goals
  of the first slice.

Those belong to `task.self.deploy-openproject-wiki-api-plugin-live`.

## Update 2026-09-25: Delete Route Cases

The proof gained a seeded member with `manage_wiki` and four cases for
`DELETE /api/v3/wiki_pages/{id}` (`task.self.add-wiki-api-page-delete`):

| Case | Result |
| --- | --- |
| 18: `delete` link only with `manage_wiki`; editor and read-only member 403, outsider 404; page kept | ok |
| 19: page with a child page: 422, parent and child kept | ok |
| 20: create then delete: 204, then 404 on read and on a second delete, not listed | ok |
| 21: leaf child page with an attachment, and a protected page, deleted with `manage_wiki` | ok |

The database step now also checks that the three deleted pages left no page
rows, no journals and no attachment. The boot step checks the `DELETE` route.
All 19 HTTP cases and all database facts passed on 2026-09-25.
