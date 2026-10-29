# GitHub Issues import document

Date: 2026-10-29

Status: file input implemented.

`replan import github --from-file` reads one network-neutral JSON document.
The later live reader must produce this same document before mapping; the
mapper itself performs no network requests.

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
