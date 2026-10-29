# Second task tracker adapter design

Date: 2026-10-28

Status: file input implemented on 2026-10-29
(`task.self.import-second-tracker-from-file`); live input remains
`task.self.import-second-tracker-live`.

The implemented command follows the existing OpenProject snapshot gate:
`--dry-run` previews and omitting it writes the staged snapshot. It therefore
does not add the `--apply` flag shown in the proposed command shape below.
Canonical changes still require `import accept --apply`. RePlan's canonical
completed tracking status is `accepted`; the table's informal `done` maps to
that existing status.

## Purpose

RePlan's model claims to be tracker-agnostic, but only OpenProject has an
adapter. A second adapter proves that claim, or shows where the model is
shaped around OpenProject. It is read-only, like the OpenProject import, and
uses the same staged snapshot and manual accept flow.

## What the adapter must supply

The canonical model needs, per task: a stable source identity, a title, a
status that maps to the tracking statuses, planned dates, an assignee hint,
dependencies, and, where the tracker has them, estimates and spent time.
Resource demands, capacities and most estimates are RePlan's own; for a
tracker that lacks a field, the existing sidecar convention fills it
(`resource-links` and `resource-demands` sidecars keyed by source id).

## Candidates

| | GitHub Issues | GitLab Issues | Plane |
|---|---|---|---|
| Audience | the largest open-source audience; RePlan itself lives on GitHub | large, strong in self-hosted teams | small, open source, self-hosted |
| Identity | repository plus issue number (and a numeric id) | project plus iid | workspace, project, work item id |
| Status | open or closed, with a close reason; finer status only in a Projects status field or labels | opened or closed; labels or boards for finer status | configurable state groups (backlog, unstarted, started, completed, cancelled) |
| Dates | none on the issue; date fields in a Project | due date only | start and target date |
| Estimates | none on the issue; number fields in a Project | time estimate on the issue | estimate points |
| Spent time | none | spent time per issue | none in the core API |
| Dependencies | "blocked by" and "blocking" REST endpoints; sub-issues for hierarchy | issue links of type "blocks"; check the tier of the links API before relying on it | relations with blocking and blocked_by, plus start/finish ordering |
| API | REST and GraphQL, fine-grained tokens, read scopes; Projects also over REST since September 2025 | REST, project access tokens | REST, API keys; reports of relation endpoints failing on self-hosted Community Edition |

## Recommendation: GitHub Issues

GitHub Issues fits the people most likely to try RePlan after the public
release, and RePlan's own repository can be the live test project, the way
OpenProject's replan-self project is today. Its gaps are exactly the ones
RePlan exists to fill: issues carry no capacity, estimates or dates, so the
adapter shows that RePlan works on a thin tracker, not only on a rich one
like OpenProject.

GitLab is the stronger second choice: it has native estimates and spent
time, which would exercise the time-entry path. Plane has the closest model
to OpenProject and would prove the least.

## Mapping for GitHub Issues

| Canonical | GitHub source | Notes |
|---|---|---|
| task identity | `source: github`, `object_type: issue`, `object_id: <owner>/<repo>#<number>`, plus the numeric issue id | the number is what people see; the id is needed by the dependency endpoints |
| title | `title` | |
| description | not imported | like OpenProject, user-authored text stays in the tracker |
| tracking status | open → `not_started`; open with a configured "in progress" label or Project status → `in_progress`; closed as completed → `done`; closed as not planned or duplicate → `cancelled` | a status map in the source profile, as `openproject` has; unknown values are reported, not guessed |
| planned dates | Project date fields named in the profile (`start_field`, `finish_field`), else none | dates are then RePlan's draft, as for any task without dates |
| assignee hint | the first assignee login, through the resource-links sidecar to a RePlan resource | no sidecar entry: a warning, no hint |
| dependency | each "blocked by" link → `finish_to_start`, predecessor = the blocking issue | only links inside the imported repository; cross-repository links are reported and skipped |
| hierarchy | sub-issues → a `parent_ref` source fact only | RePlan has no hierarchy semantics yet; kept in the snapshot for later |
| estimate | a Project number field named in the profile, through the resource-demands sidecar's unit | no field: demands come from the sidecar only |
| spent time | none | actuals stay RePlan records |
| pull requests | excluded | the issues API returns pull requests too; the adapter filters them out |

## File input first

The first implementation reads a file, so it can be tested without network
and without a token: one JSON document with the repository, the issues
(the REST list response pages concatenated), the "blocked by" lists per
issue, and optionally the Project item field values. The live import writes
exactly this document into the snapshot, so a file and a live run take the
same path from there on. A sanitized sample goes to
`examples/imports/github/`.

## Live input

- token from an environment variable named in the source profile; a
  fine-grained token with read access to Issues (and Projects, when fields
  are used) is enough;
- REST pagination with `per_page=100`, `If-None-Match` for repeated runs,
  and the primary and secondary rate limits honoured: on 403 or 429 with a
  reset header, wait or stop with `source_rate_limited`;
- the same timeout and retry settings as the OpenProject client, reads only;
- `--since` maps to the issues list's `since` parameter.

## Staged import and accept

`replan import github --project <path> (--from-file <path>|--profile <id>)
[--since <date>] [--include-closed] [--include-relations]
[--resource-links-sidecar <path>] [--resource-demands-sidecar <path>]
(--dry-run|--apply)` writes a source snapshot, like `import openproject`.
`replan import accept <snapshot> --accept-...` is shared: the accept modes do
not depend on the source. Any place where the accept code assumes
OpenProject fields is a finding of this task and is fixed in the shared code,
not copied.

## Not in scope

- writing to GitHub: no status, date or comment write-back;
- GitHub Projects as the planning surface;
- a GitHub App or webhooks; polling with `--since` is enough for now.

## Operator decisions

Agreed by the operator on 2026-10-29:

1. The second tracker is GitHub Issues.
2. The live test runs against a separate disposable repository, not
   RePlan's own public repository.
3. A label-based "in progress" status is acceptable; a Projects status field
   is not required.
