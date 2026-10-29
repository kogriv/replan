# Reaching the RePlan task page from an OpenProject work package

Status: dated record (2026-09-27). Index: [OpenProject design index](openproject-design-index.md).

Original status: link custom field chosen by the operator on 2026-09-27; helper built.

Task: `task.self.link-openproject-work-package-to-replan-panel`.

Today the RePlan task page of a work package is reachable only from the
RePlan pages in the project menu. The task is to reach it from the work
package itself, read-only, with the least coupling to OpenProject's
internals. Everything below was read in the OpenProject 17.6 image the plugin
is built on, and the chosen path was tried on the throwaway instance.

## Extension points found

1. **A work-package tab in the Angular view.** The work-package full and split
   views are Angular. Their tabs are registered in the browser through
   `WorkPackageTabsService` (`registerBefore`, `registerAfter`); the GitHub
   integration does this in `modules/github_integration/frontend/module/main.ts`,
   and the Meetings tab is a core Angular component wrapping a Turbo frame.
   A plugin tab needs a frontend module compiled into OpenProject's Angular
   bundle: a Node build of the whole frontend in the image, and a dependency
   on an internal Angular service that can change in any release.
2. **The Rails `work_package_split_view` menu.** Modules add entries with
   `menu :work_package_split_view, ... { tab: ... }` (GitHub, GitLab, Meetings,
   Wikis). This menu only draws the tab headers of the Rails split view used
   by notifications, boards, calendars and backlogs; the tab body is still the
   Angular `opce-wp-split-view` component, which knows only its registered
   tabs. A RePlan entry would show a header with nothing under it, and not in
   the main work-package view at all.
3. **Layout hooks.** `view_layouts_base_html_head` and
   `view_layouts_base_body_bottom` run on every page, so a plugin script could
   insert a link into the Angular view's markup. That couples to OpenProject's
   DOM, which changes without notice, and needs the policy nonce on every
   page.
4. **A link custom field.** OpenProject 17.6 has a `link` custom field format
   for work packages (`config/initializers/custom_field_format.rb`), shown in
   the work package's attributes as a clickable link, also as a table column,
   and readable and writable through API v3 like any custom field.

## Choice: a "RePlan" link custom field

Option 4 is the least coupled: no OpenProject code is patched and no frontend
is built; custom fields are a stable, documented feature that upgrades keep.
The value of the field is the RePlan task page of the work package,
`<public URL>/projects/<project>/replan_overlay/work_packages/<id>`. The
Rails model accepts a relative path, but API v3 refuses anything but a full
URL ("is not a valid URL"), so the helper builds it from the address people
open OpenProject at: `REPLAN_OPENPROJECT_PUBLIC_URL`, or the host header and
forwarded protocol already used for a protected connection, both from ignored
local settings. No host name is committed.

Tried on the throwaway instance: a `link` field named "RePlan", enabled for
the project and its type, is shown to a member with only `view_work_packages`
under "Other" as a link that opens the task page.

What it costs:

- a custom field created once by an administrator in the live OpenProject and
  enabled for the RePlan project and its types: a change to shared
  infrastructure, so the operator's step or their explicit go;
- writing the value to every bound work package, and to new ones: a narrow
  write-back through a reviewed helper, dry run by default and applied only
  with `REPLAN_APPLY=1`, idempotent, that never touches another field;
- the field is editable by anyone who may edit work packages; the drift check
  reports a value that differs from the expected path, and the helper
  restores it.

Upgrade risk: low. Custom fields, the `link` format and their API have been
stable across OpenProject releases; the plugin route the value points at is
the plugin's own.

## Rejected

- Angular tab: highest value visually, but it needs a frontend build in the
  image and ties the plugin to an internal service; revisit only if
  OpenProject offers server-rendered work-package tabs for plugins.
- Split-view menu entry: draws a header without a body.
- Layout-hook script: tied to markup OpenProject does not promise to keep.

## Helper

`scripts/openproject-work-package-toolkit.sh work-package replan-links
[--task <task>]` goes through the bound work packages, finds the link field
by its name in each work package's schema, and compares its value with the
expected link. A dry run, the default, lists what differs and writes nothing;
it is also the drift check for the field. With `REPLAN_APPLY=1` it writes
only the values that differ, one PATCH per work package touching only that
field, so a second run changes nothing. A work package whose type lacks the
field is reported as `replan_link_field_missing` and left alone.

`scripts/check-openproject-replan-links.sh`, run by the plugin's throwaway
runtime, checks all of this against a real OpenProject: the dry run writes
nothing, the apply writes the link and leaves the subject alone, a second run
is empty, a link changed by hand is found and restored, and a type without
the field is reported. With the comparison broken on purpose, the check
failed at "changed link is found".
