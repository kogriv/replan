# OSS boundary checkpoint 2026-08-02

Status: completed checkpoint.

Related RePlan task: `task.self.write-oss-readiness-audit`.

## Purpose

This checkpoint verifies whether the current RePlan repository can continue
toward an open-source release without accumulating avoidable private material.

It is not a public release approval. It is a boundary check between:

- reusable RePlan product code and public documentation;
- private OpenProject pilot history;
- private infrastructure and VPS operation;
- future OSS release packaging.

## Method

Commands used:

```sh
git status --short
git ls-files
find . -maxdepth 2 -type f \( -iname 'license*' -o -iname 'copying*' -o -iname 'contributing*' -o -iname 'security*' -o -iname 'code_of_conduct*' \) -print
git ls-files | rg '(^private/|^private-pilot/|^local-pilot/|^reports/latest|^tmp/|\.env$|\.env\.)'
rg -n "<private-pilot-name>|<private-endpoint>|ssh -L|<private-host>|work_packages/<pilot ids>|work_package_id: \"<pilot ids>\"|user_id: \"<pilot user ids>\"|<pilot user labels>" docs examples -S
git ls-files | xargs -r grep -Il . | xargs -r rg -n "(<private host patterns>|/root/|<private path patterns>|OPENPROJECT_TOKEN=[A-Za-z0-9_\-]{10,}|sk-[A-Za-z0-9]{20,}|ghp_[A-Za-z0-9]{20,}|glpat-[A-Za-z0-9_\-]{20,})"
go run ./cmd/replan validate examples/replan-self --format json
./scripts/check-resource-schedule-drift.sh
OPENPROJECT_TOKEN=... REPLAN_OPENPROJECT_BASE_URL=https://op.example.internal REPLAN_OPENPROJECT_PROJECT_ID=replan-self ./scripts/check-openproject-drift.sh
```

The last command shape is documented with an example base URL. The live run used
the private token and private base URL from the operator environment; those
values are not committed.

## Verdict

Current HEAD is safe enough for private incubation and OpenProject-centered
dogfood.

Current HEAD is not ready to be published as the final public OSS repository.

There are no observed committed token values, generated private reports,
tracked `private/` files, tracked `.env` files or tracked live OpenProject
bindings. The main remaining issue is not credential leakage. The issue is that
public docs still contain a private pilot narrative and public examples still
include sidecars named after the old private pilot.

## Release Blockers

### Blocker 1. No license file

No `LICENSE` or `COPYING` file exists at repository root.

Impact:

- downstream users cannot clearly know whether they may use, fork, modify or
  redistribute RePlan;
- package registries and public repositories will treat the licensing story as
  incomplete;
- this blocks a serious OSS release.

Required resolution:

- choose a license explicitly;
- add `LICENSE`;
- update `README.md` with the license statement.

### Blocker 2. No contribution and security entry points

No `CONTRIBUTING.md`, `SECURITY.md` or equivalent public process document exists
at repository root.

Impact:

- external contributors do not know how to propose changes;
- users do not know where to report security issues;
- this is acceptable during private incubation but weak for public release.

Required resolution:

- add a minimal `CONTRIBUTING.md`;
- add a minimal `SECURITY.md`;
- optionally add a code of conduct later, but do not block technical dogfood on
  that.

### Blocker 3. Private pilot narrative remains in public docs

The following tracked files still contain private-pilot operational narrative:

- `docs/spec/openproject-pilot-log.md` (moved to private storage on
  2026-10-27);
- `docs/spec/openproject-live-mapping-quality.md` (moved to private storage on
  2026-10-27);
- the historical OpenProject sections in `docs/dev/getting-started.md`;
- smaller references in `docs/roadmap/openproject-replan-sync-plan.md`,
  `docs/spec/implementation-sequence.md` and
  `docs/spec/dependency-deadline-behavior.md`.

Observed examples include:

- a private pilot project identifier;
- SSH tunnel command shapes;
- loopback OpenProject endpoint examples;
- real-looking work package ids from the pilot range;
- pilot user labels such as `OpenProject Admin`, `RePlan Ivan` and
  `RePlan Codex Agent`;
- operational observations from the private OpenProject pilot.

Impact:

- this conflicts with `docs/dev/repository-boundary-contract.md`;
- it makes the public repository look like a private operations journal;
- it will make a public build story harder to explain;
- it increases the chance of accidentally keeping real private identifiers in
  public history.

Required resolution:

- move the full pilot narrative to the private pilot repository;
- keep only sanitized lessons in this repository;
- replace operational tunnel/runbook examples with generic adapter docs;
- preserve product requirements and tests that came from the pilot, but remove
  the private timeline.

### Blocker 4. Old private-pilot sidecars are tracked as public examples

The following files are tracked:

- `examples/openproject/sidecars/resource-demands.replan-pilot.yaml` (moved to
  private storage on 2026-10-27);
- `examples/openproject/sidecars/resource-links.replan-pilot.yaml` (moved to
  private storage on 2026-10-27).

They contain old pilot-specific work package and user ids.

Impact:

- these files are useful as historical proof, but their current names and ids
  tie the public examples to a private pilot;
- they blur the boundary between reusable fixture examples and private pilot
  data.

Required resolution:

- either move them to the private pilot repository;
- or convert them into fully synthetic sidecars with neutral names, fake ids and
  matching sanitized import fixtures.

### Blocker 5. Public history strategy is not chosen

The preferred direction is already documented: publish from a clean public
repository or clean public branch from sanitized HEAD.

Impact:

- current incubation history may contain old private references even after HEAD
  is cleaned;
- do not publish current history directly until the release history decision is
  made.

Required resolution:

- before public release, choose clean public repository, clean public branch,
  history rewrite or release snapshots;
- keep current private incubation history private unless explicitly audited.

## Non-Blockers

### No tracked private folders found

The checkpoint did not find tracked files under:

- `private/`;
- `private-pilot/`;
- `local-pilot/`;
- `tmp/`;
- generated dashboard/report paths.

The `.gitignore` guardrails cover those paths.

### No committed live token values found

Searches did not find obvious OpenProject, GitHub, GitLab, OpenAI or similar
token values in tracked files.

`OPENPROJECT_TOKEN` appears as an environment variable name. That is acceptable
public API documentation and not a secret.

### Self-dogfood source references are sanitized

`examples/replan-self` uses `op.example.internal` and synthetic OpenProject ids
for public source references.

Private live OpenProject task and relation bindings remain in:

```text
private/openproject/replan-self-bindings.yaml
```

That path is ignored and is not part of the public repository.

### Generated dashboard remains untracked

The latest generated dashboard can be rebuilt locally, but generated report
artifacts under `reports/latest` are not tracked.

### Product code and tests do not depend on private infrastructure

The code and tests run without the private OpenProject token. Live OpenProject
helpers skip cleanly or require explicit environment variables.

## Dogfood Decision

The OSS boundary checkpoint is accepted for dogfood planning purposes.

It does not approve a public OSS release.

It unblocks the next task:

```text
task.self.run-full-openproject-dogfood-cycle
```

It keeps the public release story blocked until the release blockers above are
resolved or explicitly accepted by the maintainer.

## Follow-Up Backlog Candidates

These should be materialized in the OpenProject/RePlan backlog before public
release work resumes:

1. Relocate private OpenProject pilot narrative.
2. Convert or move old pilot-specific sidecar examples.
3. Choose license and add `LICENSE`.
4. Add minimal `CONTRIBUTING.md` and `SECURITY.md`.
5. Add a repeatable OSS boundary audit script or checklist command.
6. Decide public history strategy before publishing.
