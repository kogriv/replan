# Contributing to RePlan

RePlan is a FOSS-first resource, capacity and replanning layer for software
teams that use people and AI agents together. It sits beside task systems such
as OpenProject: the task system owns task identity and execution state, while
RePlan owns resource planning, workload analysis, scenarios and safe
replanning.

## Start here

Read [`README.md`](README.md) for the product and quickstart, then
[`docs/dev/getting-started.md`](docs/dev/getting-started.md) for the current
commands. Changes to CLI behavior should also start with
[`docs/spec/cli.md`](docs/spec/cli.md).

## Prerequisites

- Go 1.24 or newer, as declared in `go.mod`.
- Python 3 for the OpenProject and agent helper modules and their tests.
- `golangci-lint` v2.13.2 for the full Go lint gate. The version is pinned in
  `.golangci.yml` and the GitHub Actions workflow.
- ShellCheck for `scripts/*.sh` and `scripts/lib/*.sh`.
- TaskJuggler's `tj3` command for TaskJuggler validation and planning checks.
  GitHub Actions installs TaskJuggler 3.8.4. The core CLI and ordinary tests do
  not require it.
- Ruby with Minitest for the plain-Ruby OpenProject plugin presenter tests.
  The plugin scaffold check reports that these tests were skipped when Ruby or
  Minitest is unavailable.

## Choose and track work

The maintainer's backlog is a private OpenProject project plus RePlan.
`examples/replan-self` is the sanitized public self-dogfood plan, not a link to
the private OpenProject instance.

Outside contributors should open a GitHub issue or pull request. When work is
agreed for execution, the maintainer records it in the OpenProject/RePlan
backlog; contributors do not need access to that private project.

Keep a pull request focused. Explain the problem, why the change is needed and
how it was checked. Product text and documentation are written in English.

## Keep the safety contracts

- Nothing writes canonical RePlan state without `--apply` or
  `REPLAN_APPLY=1`. Canonical-writing CLI commands require an explicit
  `--dry-run` or apply choice; an explicit `--dry-run` remains a preview.
- OpenProject helpers are dry-run by default. Narrow OpenProject write-back is
  allowed only through the reviewed helpers; broad automatic bidirectional
  synchronization is out of scope.
- Stable JSON envelopes and exit codes are part of the CLI and helper
  contracts. A change to either needs tests and a matching update to
  [`docs/spec/cli.md`](docs/spec/cli.md).
- Regenerate golden files only when the output change is intended and reviewed.
  Do not use a golden-update setting only to make a failing test pass.

## Keep private material out

Do not commit tokens, private endpoints, hosting or VPS details, real private
OpenProject work-package IDs, or private pilot narrative. Real bindings belong
in ignored private files such as:

```text
private/openproject/replan-self-bindings.yaml
```

Committed examples use sanitized IDs and generic URLs such as
`https://op.example.internal`. See
[`docs/dev/repository-boundary-contract.md`](docs/dev/repository-boundary-contract.md)
before changing fixtures, integration examples or operational documentation.

## Run the local gate

Run the full gate from the repository root before a push. The Python helper
tests need a built binary; `tmp/` is ignored, so build it there:

```sh
mkdir -p tmp/replan-gate-bin
go build -o "$PWD/tmp/replan-gate-bin/replan" ./cmd/replan
go test ./...
go vet ./...
golangci-lint run ./...
shellcheck scripts/*.sh scripts/lib/*.sh
REPLAN_BIN="$PWD/tmp/replan-gate-bin/replan" \
  PYTHONPATH="$PWD/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  python3 -W error::ResourceWarning -m unittest discover -s scripts/tests
./scripts/check-agent-kit.sh
./scripts/smoke-mvp.sh
git diff --check
```

The smoke runs the plugin scaffold check. With Ruby and Minitest installed,
that check also runs the plugin's plain-Ruby presenter tests. TaskJuggler checks
are separate because TaskJuggler is optional:

```sh
./scripts/validate-taskjuggler.sh
./scripts/run-taskjuggler.sh
```

For a documentation-only change, `git diff --check` is usually enough. Also
open or otherwise verify every link that the change adds or edits.

## Commits and pull requests

Follow the style in `git log`: `type(scope): subject`, with an imperative,
lower-case subject where practical. Use the body to explain why the change was
needed and any important trade-off; do not only restate the diff.

Do not include unrelated generated files or local artifacts. State which
checks you ran, and state plainly when an optional check was unavailable.

## License

RePlan is licensed under the
[Apache License, Version 2.0](LICENSE). Under section 5, contributions
intentionally submitted for inclusion are under the same license unless the
contributor explicitly states otherwise. The project does not require a
Contributor License Agreement.
