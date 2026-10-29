# OpenProject pilot lessons

Date: 2026-10-27

Status: current sanitized product lessons.

The full pilot record is kept privately and is not part of the public
repository. This page keeps only lessons that still match the current code,
the OpenProject operator loop and the agent kit. It contains no installation
runbook or private project evidence.

## Product boundary

OpenProject works well as the human task surface. It owns work-package
identity, titles, source status, assignees, estimates, dates, relations, Gantt
and ordinary collaboration.

RePlan remains the resource-planning sidecar. It owns resource demands,
capacity, availability, non-human constraints, actuals, evidence, scenarios,
warnings, proposals, tracking status and audit.

An import is evidence and a set of candidates. It is not authority to mutate
canonical RePlan state. A RePlan plan is likewise not authority to mutate
OpenProject. Every direction has a reviewed dry-run/apply boundary.

## Standard fields that mapped well

The following OpenProject fields are useful without customization:

| OpenProject data | RePlan use | Current lesson |
| --- | --- | --- |
| Work-package id and self link | Source identity and matching | Use stable source references, not titles, as identity. |
| Subject | Candidate title | Good for human review, but not a stable key. |
| Status | `source_status` and mapped status candidate | Preserve the source fact; accept RePlan tracking status separately. |
| Assignee | Resource candidate | Useful as a hint; an assignee does not imply a complete resource demand. |
| Estimated time | Candidate estimate | Useful for coarse sizing, not a replacement for demand records. |
| Start and due dates | Candidate planned dates | Useful for drift review; direction is a human decision. |
| Relations | Dependency candidates | Useful when both related tasks are in scope and the relation type is supported. |
| Spent time | Work-package rollup | Useful as a summary, not as entry-level actual evidence. |

OpenProject standard fields do not express the full RePlan resource model.
They do not reliably encode reviewer effort, AI-agent effort, API quotas,
token budgets, environments, equipment, budgets or other non-human demands.

## Status mapping is intentionally conservative

Source status and RePlan tracking status are different facts. In particular,
generic source states such as `New` or `Open` map to `not_started`, not
`ready`. More explicit planning states may map to `ready`; active, review,
blocked and closed states map to their corresponding RePlan candidates.

The mapping is still only a proposal. `import accept
--accept-tracking-status` updates existing matched tasks only, requires a dry
run before apply in the operator workflow, and does not run as part of
ordinary task acceptance.

## API acquisition lessons

OpenProject API behavior that remains relevant:

- `--since` uses the OpenProject date-range operator `<>d`, from the supplied
  date through a far-future bound. RFC3339 input is reduced to its date.
- Collection reads must paginate. RePlan uses `offset` and `pageSize`, with a
  default page size of 100, for work packages, relations and time entries.
- Closed work packages need an explicit `--include-closed` request. The
  importer resolves the project identifier to its numeric id and uses the
  global work-package collection with a project filter for that mode.
- A single work package can be fetched directly with `--work-package`.
- Relations come from `/api/v3/relations`; the importer keeps relations that
  touch the imported work-package set.
- Time entries come from `/api/v3/time_entries`; the importer keeps entries
  whose entity is an imported work package.
- Work-package `spentTime` loses entry-level identity, user, activity and
  comment. Direct time-entry records are the source for actual candidates.
- API imports can use bounded request timeouts and retries. Authentication and
  endpoint settings remain external to committed project data.

The importer uses the same mapper for API and saved-file input. Public
fixtures therefore exercise mapping and accept behavior without needing a
live service.

## Relation direction and identity

Supported relation mappings are:

| OpenProject relation | RePlan interpretation |
| --- | --- |
| `precedes` | `from` precedes `to`; `finish_to_start` |
| `follows` | `to` precedes `from`; `finish_to_start` |
| `blocks` | `from` blocks `to`; `blocks` |
| `blocked` | `to` blocks `from`; `blocks` |

OpenProject may normalize a relation into its reverse representation. The
mapper must interpret direction from the returned relation type rather than
assuming request direction.

Both related work packages must match canonical tasks before a dependency is
safe to accept. A partially imported or cross-scope relation stays visible as
an unmatched candidate instead of disappearing.

Dependency duplicate detection uses the OpenProject relation source reference
and the canonical predecessor/successor/type triple. When the triple already
exists, acceptance links the relation source reference instead of creating a
duplicate dependency.

## Time entries become evidence before actuals

`--include-time-entries` produces read-only candidates. It does not create
actuals.

An entry is safe to accept only when it matches:

1. an existing RePlan task;
2. an existing RePlan resource;
3. an hour-based resource demand for that task and resource.

`import accept --accept-time-entries` creates evidence and then the linked
actual-consumption record. Duplicate detection uses the OpenProject
time-entry source reference. Apply requires an actor, writes audit evidence
and validates the resulting project.

This prevents an aggregate or an unmatched user from becoming canonical
consumption by inference.

## Sidecars are the bridge, not canonical state

The first useful convention is still two RePlan-owned integration sidecars:

- `integrations/openproject/resource-links.yaml` maps stable OpenProject user
  ids to existing canonical RePlan resources;
- `integrations/openproject/resource-demands.yaml` maps work packages to
  explicit demand candidates.

Resource-link matching prefers an existing canonical source reference, then
an explicit sidecar link, then display-name matching. Display names are
context, not identity.

Demand sidecars preserve units, roles, criticality, confidence, remaining
work and non-human resource classes that OpenProject cannot model directly.
They may refer only to known resources or pools; they do not create those
objects implicitly.

The sidecars are mapper input. They are not generated snapshots under
`imports/`, and they are not accepted state under `canonical/`. Import emits
candidates; separate accept modes write reviewed source links or demands.

Public examples use the sanitized `replan-self` sidecars. Real ids belong in
ignored private bindings or private integration data.

## Custom-field policy

Do not mirror RePlan's canonical model into a large OpenProject custom-field
scheme.

Use standard OpenProject fields for task identity, status, assignee, estimate,
schedule and relations. Keep rich resource data in RePlan sidecars. Add a
custom field only when repeated operator use proves that the value must be
visible or editable in OpenProject.

Each proposed custom field must define its name, type, allowed values,
canonical target, required/optional rule and missing-value behavior. The
current operator loop has one deliberate custom field: the `RePlan` link used
to reach a task's RePlan page. That navigation field does not duplicate
resource-planning state.

## Accept flows stay separate and manual

The safe import order is compositional:

1. accept or link task identities;
2. accept resource links;
3. import again so resource source references participate in matching;
4. accept resource demands;
5. accept time entries;
6. optionally accept tracking status and dates after their own review;
7. accept relation-backed dependencies from a snapshot with relation
   candidates.

Every specialized mode is mutually separate: dates, tracking status,
resource links, resource demands, time entries and dependencies do not ride
along with ordinary task acceptance or with each other. Writes require
`--apply --actor <id>`, append audit and validate.

Manual acceptance remains necessary because source facts can be incomplete,
stale, out of scope or semantically different from RePlan state. A single
combined automatic accept would hide those distinctions and make partial
matching dangerous.

## Date and status direction

RePlan drafts planned dates from capacity and load. Reviewed helpers push
those dates to OpenProject. OpenProject then presents them to people and may
reschedule work according to its calendar or relations.

Neither side wins drift automatically. The drift check exposes the difference;
the operator either pushes reviewed RePlan dates again or accepts the
OpenProject dates into RePlan. Status follows the same explicit two-direction
rule, but date and status acceptance are never combined.

Actual start and finish facts stay in RePlan evidence and actuals. They are not
inferred merely from planned OpenProject dates.

## Mapping-quality gate

A mapping pass is ready for dry-run acceptance when:

- every work package has stable source identity and a readable title;
- statuses produce known candidates or explicit unmapped warnings;
- assigned work produces useful resource candidates;
- estimates and dates are present where the planning pass needs them;
- unsupported, unmatched and out-of-scope data remains visible;
- diagnostics distinguish expected unmatched fixture data from real errors;
- the operator can understand the candidates without consulting private
  operational notes.

Block acceptance when identity is missing or ambiguous, required task or
resource matches are absent, relation endpoints are not both in scope, demand
records are invalid, or diagnostics make safe and unsafe candidates
indistinguishable.

The following are not blockers by themselves: a missing optional sidecar,
unsupported optional fields, unmatched synthetic fixtures, absent custom
fields, or candidates that correctly remain read-only pending a separate
accept mode.

## Current rule

Use [`../spec/openproject-operator-loop.md`](../spec/openproject-operator-loop.md)
for ownership and workflow, [`../spec/cli.md`](../spec/cli.md) for the command
contract, and [`../agent-kit/README.md`](../agent-kit/README.md) for the
operator surface and date/status direction. Those current documents override
any older pilot-era assumption.
