# GitHub Issues import document

Date: 2026-10-29

Status: file and live REST input implemented.

`replan import github --from-file` reads one network-neutral JSON document.
Live input builds and stores this same document before mapping; the mapper
itself performs no network requests. Therefore a live snapshot's `raw.json`
can be passed back to `--from-file` and produces the same candidates.

Live input reads repository facts and paginated issues from the GitHub REST
API. With `--include-relations`, it also reads each selected issue's
`dependencies/blocked_by` and `sub_issues` endpoints. Pull requests returned by
the issues endpoint are retained only long enough for the shared mapper to
filter them. Sub-issues already present in the selected issue list receive a
normalized `parent` reference.

GitHub Projects fields are not read by live input. When a selected profile
names `start_field`, `finish_field`, or `estimate_field`, the command reports
`github_project_fields_live_unsupported`; Project-backed values can still be
supplied in a replay or exported file document.

## Version 1

The top-level `format` is `replan.github-issues.v1`.

```json
{
  "format": "replan.github-issues.v1",
  "repository": {
    "owner": "example-org",
    "name": "planning-demo",
    "html_url": "https://github.com/example-org/planning-demo"
  },
  "issues": [],
  "blocked_by": [],
  "project_items": []
}
```

`issues` concatenates GitHub REST issue-list pages. Each issue uses the REST
fields `id`, `number`, `title`, `state`, `state_reason`, `html_url`,
`updated_at`, `labels`, `assignees`, and optional `pull_request`. A present
`pull_request` marks an item for filtering. An optional normalized `parent`
object has `repository`, `number`, numeric `id`, and `html_url`; it is retained
as a source fact only.

`blocked_by` contains objects with `issue_number` and an `issues` array. Each
blocking reference has `repository` (`owner/repo`), `number`, optional numeric
`id`, and optional `html_url`. Only same-repository references are mapped.

`project_items` contains `issue_number` and a `fields` object. String fields
named by the source profile may supply start and finish dates in `YYYY-MM-DD`.
A numeric field named by the profile may supply a demand amount when the
resource-demands sidecar supplies its resource, role and unit.

The committed sanitized example is
`examples/imports/github/issues.sample.json`; its URLs use only generic
`github.com` placeholders and its logins are not real users.

## Sidecars

Resource links use:

```yaml
github_resource_links:
  - login: sample-planner
    resource_ref: {kind: resource, id: resource.human.ivan}
```

Resource demands use:

```yaml
github_resource_demands:
  - issue: example-org/planning-demo#101
    demands:
      - id: resource_demand.github-101.estimate
        resource_ref: {kind: resource, id: resource.human.ivan}
        resource_class: human
        role: implementer
        unit: hours
        project_number_field: Estimate
        criticality: required
        status: active
```

`planned_amount` may be supplied directly. When `project_number_field` is
present—or the profile names `estimate_field`—the numeric Project field wins;
the sidecar still owns the demand's resource and unit.
