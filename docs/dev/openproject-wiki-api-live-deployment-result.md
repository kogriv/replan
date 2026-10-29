# OpenProject wiki API plugin live deployment result

Status: dated record (2026-09-21). Index: [OpenProject design index](openproject-design-index.md).

Original status: deployed and verified read-only on the private live instance.
Acceptance against the issue criterion by the affected project's own agent and
a human is still open.

Date: 2026-09-21.

Related task: `task.self.deploy-openproject-wiki-api-plugin-live`.

Design:
[`openproject-wiki-api-plugin-design.md`](openproject-wiki-api-plugin-design.md).

Throwaway proof:
[`openproject-wiki-api-throwaway-runtime-test.md`](openproject-wiki-api-throwaway-runtime-test.md).

## Public Boundary

This document is public-safe. It does not record the live host name, private
paths, tokens, user names, project names, page titles or page content.
Concrete operational paths and the backup location belong in the private
infrastructure repository.

## Authorization

The operator accepted the throwaway proof and ordered the live deployment on
2026-09-21, ahead of the planned 2026-10-05 window. The deployment was executed
by the design author.

## Procedure

1. Backup on the OpenProject host before any change: PostgreSQL `pg_dumpall`
   through the explicit database role, compressed; uploaded assets archive;
   compose service status; image list; a copy of the compose override. The dump
   was verified: gzip integrity, 203 `COPY` statements, the `wiki_pages` table
   present, and the cluster-dump completion marker at the end.
2. Image: the exact image that passed the throwaway proof was deployed. It was
   not rebuilt on the host. It was tagged with the OpenProject version and the
   RePlan revision, streamed to the host with `docker save` and `docker load`,
   and verified there by a content hash over `Gemfile.lock`, `Gemfile.plugins`
   and every plugin file, equal on both machines. Image ids differ between the
   two Docker installations and are not a usable comparison.
3. The overlay plugin source is unchanged since the revision that was live, so
   the RePlan project page and work-package panel are carried over as they
   were.
4. Switch, as one guarded script with automatic rollback:
   - copy the compose override aside;
   - require exactly four occurrences of the old tag, then replace the tag for
     `web`, `worker`, `cron` and `seeder`;
   - validate the compose configuration;
   - stop the watchdog container, because Rails boot after an image change can
     exceed its restart window (Step 39 lesson);
   - recreate `web`, `worker` and `cron`;
   - wait for the web health check, up to fifteen minutes;
   - on failure: restore the override, recreate on the old image, restart the
     watchdog;
   - on success: restart the watchdog.
5. No database migration is involved. The plugin adds none.

## Result

```text
web healthy after:                 60 seconds
rollback needed:                   no
web, worker, cron on the new tag:  yes
NameError or LoadError in logs:    0 in web, worker and cron
watchdog restored:                 yes, healthy
database, cache, proxy, editor:    untouched, uptime unchanged
```

Inside the live web process both plugins are registered,
`openproject_replan_overlay` and `openproject_wiki_api`, the four wiki routes
are present next to the upstream `GET` and attachment routes, and both overlay
routes are present.

## Read-Only Verification Over HTTP

With an existing API token through the instance's protected connection, using
the required `Host` and forwarded-protocol headers. Read requests only. No page
was created or changed by the deployment.

| Check | Result |
| --- | --- |
| token resolves to its user | 200 |
| collection of each of the three projects that have wiki pages | 200; totals 1, 2 and 2; no element carries `text` |
| every one of the five existing pages, single `GET` | 200; `text.raw` present with `markdown` format; integer `lockVersion`; `update`, `author`, `project` and both attachment links |

## Finding: Stale Cached Representation

The first single-page read of one page returned the old upstream shape: no
`text`, no `lockVersion`, no `update` link. Status 200, so nothing looked
broken.

Cause: `WikiPageRepresenter` is a cached representer. Its cache key is built
from the representer name, the locale and a digest of the represented page. It
does not change when a plugin adds properties to the representer. That page had
been requested through API v3 before the plugin existed, so the old JSON was
still in the cache and was served as is. The other pages had not been cached.

The throwaway runtime could not show this. It starts with an empty cache.
Verification point 1 of the design asked whether an edit invalidates the cache,
which it does. It did not ask what happens to entries written before the plugin
was installed.

Observed detail: `Rails.cache.clear` from a runner process did not remove the
entry. Deleting the exact key through `OpenProject::Cache.delete` did. The
reason was not investigated further. After the delete, a fresh render carried
all added properties. A sweep over every wiki page, every available interface
language and both link-embedding modes found no further stale entry, and all
five pages then verified over HTTP.

Consequence for the procedure: after installing or changing this plugin, delete
the cached `WikiPageRepresenter` entries for all pages, then verify every page
over HTTP, not a sample. A page edit also replaces the entry, but waiting for
an edit leaves agents reading text-less pages in the meantime.

Follow-up worth a backlog item: make the plugin's presence part of the cache
key, so that installing, upgrading or removing it invalidates old entries by
itself.

## Finding: Public Perimeter Path Rejects The RePlan Token

The same read requests through one perimeter path returned 401 from the reverse
proxy rather than OpenProject, while the protected operator path accepted the
token. The legacy perimeter credential therefore no longer matched it.

This is outside the plugin and was not changed. It matters for acceptance: an
agent must use a perimeter path and credential valid for that project.

## Rollback

Not used. It remains available: restore the saved compose override, recreate
`web`, `worker` and `cron`, and the previous image tag is still on the host.
The plugin writes nothing at install time, so no database restore is involved
in a rollback of the deployment itself. Wiki edits made through the API after
this point are ordinary journaled OpenProject data and stay valid without the
plugin.

## Acceptance

Accepted on 2026-09-24. The affected project's agent, with its own member
token, listed pages, read text, edited a page with `lockVersion` and got
`409` on a stale version. The operator confirmed the plugin works in live
use. The member-token create check was skipped on purpose, because the API
cannot delete a page afterwards; that is GitHub issue 2 and
`task.self.add-wiki-api-page-delete`. GitHub issue 1 is closed.

## What Was Open Before Acceptance

- acceptance against the issue criterion: the affected project's agent, with
  its own token, lists pages, reads text, creates a page and edits one; a human
  sees the agent's edit in the browser; the agent sees a human's edit through
  the API. The operator chose to observe this in live use;
- the role of that agent needs `view_wiki_pages` and `edit_wiki_pages` in its
  project. That is a role-editor setting and was not changed by the deployment;
- GitHub issue #1 stays open until that acceptance;
- live OpenProject synchronization of the wiki API backlog tasks is still
  pending.

## Update 2026-09-25: Page Delete Route

Related task: `task.self.add-wiki-api-page-delete`, GitHub issue 2.

`DELETE /api/v3/wiki_pages/{id}` was added and deployed with the same
procedure as above. It needs `manage_wiki`, the permission the browser's delete
action needs; OpenProject 17.6 no longer has a separate delete permission. A
page with child pages is refused with 422.

```text
throwaway proof:                  19 of 19 HTTP cases; deleted pages leave no
                                  rows, journals or attachments
backup before the switch:         cluster dump verified (203 COPY statements,
                                  wiki_pages present, completion marker),
                                  uploaded assets, compose state, override copy
image:                            the throwaway-proven image, content hash equal
                                  on both machines
web healthy after:                50 seconds
rollback needed:                  no
load errors in web, worker, cron: 0
watchdog restored:                yes
DELETE route present live:        yes, next to the upstream GET and attachment
                                  routes
```

Cache sweep, as required by the first deployment: 5 pages, 28 interface
languages and both link-embedding modes, 280 keys; 6 stale entries deleted. A
fresh render of every page and an HTTP read of every page then carried text,
`lockVersion`, `update` and `delete` links.

Live create-then-delete check, with the admin token in a project whose only
member is the admin, so that nobody was notified: create 201, `delete` link
present, delete 204, then 404 on read, not listed, second delete 404.

Before the operator's decision the member-token check was open. The affected project's agent
role has `view_wiki_pages`, `view_wiki_edits` and `edit_wiki_pages`, not
`manage_wiki`, so it gets 403 on delete until the operator decides to grant
`manage_wiki` to that role. Granting it also allows protecting, renaming and
re-parenting pages.

Member-token acceptance, 2026-09-25. The operator decided to grant
`manage_wiki` to the affected project's agent role, which only that project's
agent account holds; the role's previous permission list was saved on the host
first. A temporary API token of that non-admin account then ran the same
create-then-delete check in its own project: create 201, `delete` link present,
delete 204, then 404 on read, not listed, second delete 404. The temporary
token was removed; the account's own token was not touched. GitHub issue 2 is
closed.

## Update 2026-09-25: Work-Package Panel Permission

Related task: `task.self.fix-plugin-panel-permission`, found by the plugin
review of the same day
([`openproject-plugin-review-2026-09-25.md`](openproject-plugin-review-2026-09-25.md)).

`view_replan_overlay` granted only the project page action, so OpenProject
refused the work-package panel to every non-admin with HTTP 500. The
permission now grants both actions. Deployed with the same procedure:

```text
throwaway proof:                  a non-admin member with only
                                  view_replan_overlay is allowed show and
                                  work_package; without the fix the same check
                                  reproduces the live error
backup before the switch:         cluster dump verified (203 COPY statements,
                                  completion marker), uploaded assets, compose
                                  state, override copy
image:                            the throwaway-proven image, content hash equal
                                  on both machines
web healthy after:                50 seconds
rollback needed:                  no
load errors in web, worker, cron: 0
watchdog restored:                yes
panel as a non-admin:             renders, checked in a browser as the
                                  read-only review user
```

No wiki representer changed, so no cache sweep was needed.

## Update 2026-09-26: Plugin Milestone 1

Related task: `task.self.deploy-plugin-milestone-1`. Deployed the first
milestone of the plugin redesign
([`openproject-plugin-redesign-design.md`](openproject-plugin-redesign-design.md)):
the rendered-page checks, the version 2 overlay, colours from OpenProject's
variables with its page header, and the restructured project page.

```text
throwaway proof:                  64 rendered-page checks as a member, an
                                  outsider and signed out, on the image built
                                  from the deployed commit
backup:                           database dump (203 tables, completion
                                  marker present), assets, compose override,
                                  and the previous overlay artifact
image:                            17.6.0-wikiapi-82d90d2, content hash equal
                                  on both sides
overlay:                          version 2 published before the switch; the
                                  previous plugin reads it too
switch:                           guarded, web healthy after 60 s, web,
                                  worker and cron on the new image without
                                  load errors; the previous image stays on
                                  the host for rollback
check:                            both pages at 1600 and 900 pixels as the
                                  non-admin review user
```

The live project page shows the five counters, with the one overloaded week
ahead, the operator in 2026-W46, and the open tasks with names; the panel
shows the page header with breadcrumbs.

## Update 2026-09-26: Plugin Gantt Page

Related task: `task.self.implement-openproject-plugin-gantt-view`, the first
part deployed on its own under the operator's decision to take plugin parts
live one by one. Same procedure: 95 rendered-page checks on the image built
from the deployed commit, backup (203 tables, completion marker present, and
the previous overlay), image `17.6.0-wikiapi-d6c180f` with an equal content
hash on both sides, a fresh overlay, guarded switch with web healthy after
50 s and no load errors, the previous image kept for rollback. The Gantt page
was checked at 1600 pixels as the non-admin review user on the live plan.

## Update 2026-09-26: Plugin Gantt Navigation

Related task: `task.self.add-plugin-gantt-navigation`, deployed on its own.
109 rendered-page checks on the image built from the deployed commit and 20
browser checks of the script on the throwaway instance; backup (203 tables,
completion marker present, previous overlay); image
`17.6.0-wikiapi-7b6b312` with an equal content hash on both sides; a fresh
overlay; guarded switch with web healthy after 50 s and no load errors; the
previous image kept for rollback. On the live Gantt, as the non-admin review
user, the script's controls are shown, which they are only when OpenProject's
policy lets the script run.

## Update 2026-09-26: Plugin Load Profile

Related task: `task.self.add-plugin-resource-load-profile`, deployed on its
own. 120 rendered-page checks on the image built from the deployed commit and
23 browser checks of the script on the throwaway instance; backup (203 tables,
completion marker present, previous overlay); image `17.6.0-wikiapi-1cf2007`
with an equal content hash on both sides; a fresh overlay; guarded switch with
web healthy after 50 s and no load errors; the previous image kept for
rollback. On the live Gantt, as the non-admin review user, "Load by week"
shows a row per resource and the one red hump is the maintainer's 2026-W46,
the same resource-week the overview counts.

## Update 2026-09-26: Plugin Task Page

Related task: `task.self.show-evidence-and-actuals-in-plugin-panel`, deployed
on its own. 135 rendered-page checks on the image rebuilt from the deployed
commit, after a layout fix tried on the throwaway instance; backup (203
tables, completion marker present, previous overlay); image
`17.6.0-wikiapi-58a7b19` with an equal content hash on both sides; a fresh
overlay; guarded switch with web healthy after 50 s and no load errors; the
previous image kept for rollback. On the live task pages, as the non-admin
review user, the task and its lanes, what was done, the amounts, the bookings
and the linked predecessor show.

## Update 2026-09-27: Overview Without The Old Task List

Related task: `task.self.remove-old-task-list-from-plugin-overview`, deployed
on its own. 137 rendered-page checks on the image rebuilt from the deployed
commit, whose plugin files equal the commit's; backup (203 tables, completion
marker present, previous overlay); image `17.6.0-wikiapi-aebc860` with an
equal content hash on both sides; a fresh overlay; guarded switch with web
healthy after 50 s and no load errors; the previous image kept for rollback.
On the live overview, as the non-admin review user, the counters and the
attention table show and the old task list is gone.

## Update 2026-09-27: RePlan Link In Every Work Package

Related task: `task.self.link-openproject-work-package-to-replan-panel`. No
image change. With the operator's go, after a database backup (203 tables,
completion marker present), a `link` custom field named "RePlan" was created
in the live instance for the Task, Milestone and Summary task types and
enabled for the RePlan project only. The helper's dry run planned 99 links;
one was written first and followed in the browser as the review user to the
RePlan task page; the other 98 followed, and a second dry run found all 99
unchanged. Each write shows in the work package's activity.

## Update 2026-10-12: One Overlay Per Project

Related task: `task.self.scope-plugin-overlay-to-its-project`, deployed on its
own. 164 rendered-page checks, 8 link checks and 19 agent-kit checks on the
image rebuilt from the deployed commit, whose plugin files equal the commit's;
backup (203 tables, completion marker present, previous overlay); image
`17.6.0-wikiapi-d3cf6b6` with an equal content hash on both sides, loaded on
the second try after the first upload's connection closed; a fresh overlay
naming the RePlan project; guarded switch with web healthy after 50 s and no
load errors; the previous image kept for rollback. The switch script restored
from the session log was an early draft that named its backup file after the
whole image reference and stopped before changing anything; the name is now
the tag. On the live instance the loader gives the RePlan project its 100
rows and gives the second project "overlay_other_project" with no rows and
no path. The compose override was not changed: the single overlay file stays,
and a per-project directory is a later step for the second project.

## Update 2026-10-12: Overlay Directory On The Live Instance

Related task: `task.self.serve-plugin-overlays-per-project-live`. No image
change. With the operator's go, after a database backup (203 tables,
completion marker present) and a copy of the override, the compose override
gained `REPLAN_OPENPROJECT_OVERLAY_DIR` and a read-only mount of the overlay
directory for web, worker, cron and seeder; the edit was first applied to a
copy of the override and its result parsed. A guarded script recreated the
services, with web healthy after 50 s, the variable and the mount present in
each and no load errors, and a rollback to the saved override on any failure.
The RePlan project's overlay was then published into its own directory with
`scripts/publish-openproject-overlay.sh`; the loader reads it from there, and
the second project is still told it has no overlay. The single overlay file
remains as the fallback.

## Update 2026-10-12: RePlan Module In A Second Project

No image change and no RePlan task of its own; a follow-up of
`task.self.serve-plugin-overlays-per-project-live`. With the operator's go
for all three members of the second project, after a database backup (203
tables, completion marker present), the RePlan module was enabled in that
project. No permission changed: its members hold the "Project admin" role,
which already had `view_replan_overlay`. Until that project's own session
publishes its overlay, its RePlan pages say the project has no overlay yet;
the first project's pages are unchanged.

## Update 2026-10-12: Perimeter Keys For Outside Agents

Related task: `task.self.pass-perimeter-key-to-openproject`. No image change.
The reverse proxy in front of the live OpenProject keeps a shared password on
every path and lets an API client through only with a perimeter key in a
request header; the finding of 2026-09-25 above describes the same layer.
With the operator's go, two keys were added to the proxy's private key file,
one for the second project's outside agent and one for RePlan's own outside
check, each with a comment saying how to revoke it; the file was backed up,
the configuration tested, and the proxy reloaded, with a rollback had the
test failed. A sanitized outside-path verification then confirmed that the
agent kit was accepted with the correct key and refused without it or with a
wrong one. The keys are kept only in ignored private storage.
