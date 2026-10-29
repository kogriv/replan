# OpenProject wiki API

This plugin extends OpenProject API v3 so an API-token participant can list,
read, create, update and delete wiki pages on the same terms as a human member.

It is a separate gem from `openproject-replan_overlay`. It does not add a
project module or a permission. Authorization stays on OpenProject's
`view_wiki_pages`, `edit_wiki_pages` and `manage_wiki`. Writes go through
`WikiPages::CreateService` and `WikiPages::UpdateService`; deletion goes through
a `BaseServices::Delete` service with its own contract.

A command-line client for these routes ships with RePlan:
`scripts/openproject-wiki.sh`, also part of the agent kit
(`docs/agent-kit/README.md`).

## Routes

| Method and path | Behavior |
| --- | --- |
| `GET /api/v3/projects/{id}/wiki_pages` | Paginated collection of visible pages in the project's wiki, ordered by title. Elements are thin: metadata and links, no text. A project without a wiki returns 404. A member without `view_wiki_pages` gets 403. |
| `GET /api/v3/wiki_pages/{id}` | Existing upstream route with an extended representation: `text`, `slug`, `lockVersion`, `createdAt`, `updatedAt`, `protected`, and `parent`, `author`, `update` links. Attachment links stay. |
| `POST /api/v3/projects/{id}/wiki_pages` | Creates a page. Returns 201. A project without a wiki returns 404 and does not create a wiki. |
| `PATCH /api/v3/wiki_pages/{id}` | Updates a page. `lockVersion` is required. |
| `DELETE /api/v3/wiki_pages/{id}` | Deletes a page. Returns 204. Needs `manage_wiki`, the same permission the browser's delete action needs. A page with child pages is refused with 422; move or delete the children first. |

## Deleting pages

In OpenProject 17.6 deletion belongs to `manage_wiki`. The former separate
`delete_wiki_pages` permission was folded into it upstream, so a role that can
delete pages can also protect, rename and re-parent them. `edit_wiki_pages`
alone creates and edits but does not delete, in the API and in the browser
alike. Grant `manage_wiki` to a role only when that whole set is intended.

The representation carries a `delete` link only for a user who may delete.
The browser offers three choices for child pages: keep them as top-level
pages, delete them too, or move them under another page. The API does not
pick one for the client: `DELETE` on a page with children is 422 and changes
nothing. Deletion is not journaled as an edit: the page, its journals and its
attachments are removed, exactly as by the browser.

Clients read and write formattable `text.raw`. The response may also carry
`text.html`. Agents must not parse `html`.

`lockVersion` is the `lock_version` column. `WikiPage#version` is the journal
version and is not exposed under that name.

## Writable attributes

| Attribute | Create | Update |
| --- | --- | --- |
| `title` | required | not writable |
| `text.raw` | optional | writable |
| `_links.parent` | optional | writable |
| `lockVersion` | not applicable | required |
| `protected`, `slug`, `author`, `wiki` | not writable | not writable |

The create endpoint injects `wiki: project.wiki`. Author is set to the current
user by `WikiPages::SetAttributesService`.

A missing or stale `lockVersion` on PATCH is HTTP 409 and leaves the text
unchanged. The client must re-read, merge and retry. `lockVersion` on POST is
not applicable: if the client sends it, the create contract rejects it as
read-only and the API returns 422.

## HTTP status by case

| Case | Status |
| --- | --- |
| GET a page the token cannot see | 404 |
| GET collection with `view_wiki_pages` | 200, possibly empty |
| GET collection without `view_wiki_pages` | 403 |
| GET collection or POST when the project has no wiki or the wiki module is disabled | 404 |
| POST success | 201 |
| PATCH success | 200 |
| POST or PATCH without `edit_wiki_pages` | 403 |
| PATCH of a protected page without `manage_wiki` | 403 |
| DELETE success | 204 |
| DELETE without `manage_wiki` | 403 |
| DELETE of a page with child pages | 422 |
| DELETE of a page the token cannot see, or already deleted | 404 |
| Unwritable property in the payload (`title` on PATCH, `protected`, `slug`) | 422 |
| Validation error: blank title, bad or circular parent, parent from another project | 422 |
| Missing or stale `lockVersion` on PATCH | 409 |

## Packaging

`Gemfile.plugins` must require the entry file:

```ruby
group :opf_plugins do
  gem "openproject-wiki_api",
      path: "../plugins/openproject-wiki_api",
      require: "open_project/wiki_api"
end
```

The gem is loaded by the same custom OpenProject image as
`openproject-replan_overlay`. All Rails services must use that image.

Adding this gem changes `Gemfile.lock`. The first image that includes it must
rebuild the dependency layer:

```sh
REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE=deps-base \
  ./scripts/build-openproject-plugin-image.sh
```

Later Ruby-only changes can use the fast `code` layer.
