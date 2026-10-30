# Release process

Date: 2026-10-28

Status: runbook. Tagging, publishing and repository administration are done
by the operator, or by an agent on the operator's explicit instruction. The
release archives are built locally from the tagged commit; GitHub Actions runs
the CI checks on the public repository but does not build releases.

Related task: `task.self.add-version-command-and-release-build`.

## Prerequisites

Run this procedure in the public repository produced and verified with
[`public-repository-cut.md`](public-repository-cut.md). The checkout must have
Go 1.24 or newer, Python 3, Git, `sha256sum` and GitHub CLI (`gh`). Start from a
clean, tested release commit. The release builder reads only the commit at
`HEAD`; it never tags, pushes or uploads.

## Choose and tag the version

Use semantic versions. RePlan is alpha software, so releases remain in the
`v0.x.y` series until its compatibility promises justify `v1.0.0`.

```sh
version=v0.1.0
git status --short
git tag -a "$version" -m "RePlan $version"
git show --stat "$version"
```

Do not create the tag until the release commit has been reviewed and its CI
run has passed. The
default build version is the exact tag at `HEAD`; an untagged commit is
refused. `REPLAN_RELEASE_VERSION` is available for an explicit local preview.

## Build the archives locally

Choose a new or empty output directory. The script builds the six supported
targets with CGO disabled and records the commit date as the build date.

```sh
export REPLAN_RELEASE_DIR="$PWD/dist/$version"
scripts/build-release.sh | tee "$REPLAN_RELEASE_DIR.build.json"
```

The JSON envelope lists each archive, byte size and SHA-256 checksum. The
directory also contains `SHA256SUMS`. A dirty worktree, including untracked
non-ignored files, is refused unless the operator explicitly sets
`REPLAN_RELEASE_ALLOW_DIRTY=1`; even then the archives are built from `HEAD`.

## Verify an archive

Check every archive and inspect one matching the current machine:

```sh
cd "$REPLAN_RELEASE_DIR"
sha256sum --check SHA256SUMS
tar -xzf "replan_${version}_linux_amd64.tar.gz"
"replan_${version}_linux_amd64/replan" version --format json
"replan_${version}_linux_amd64/replan" --version
```

The reported version and commit must match the tag and release commit. The
JSON also reports the build date, Go version, OS/architecture and current
RePlan project format version.

## Publish the GitHub release

Pushing the tag and creating the release publish the version; they need the
operator's go:

```sh
git push origin "$version"
gh release create "$version" \
  "$REPLAN_RELEASE_DIR"/replan_${version}_*.tar.gz \
  "$REPLAN_RELEASE_DIR"/replan_${version}_*.zip \
  "$REPLAN_RELEASE_DIR/SHA256SUMS" \
  --verify-tag --generate-notes --title "RePlan $version"
```

Review the generated notes and all uploaded assets in GitHub before announcing
the release.

## The CI build and its provenance

Pushing the tag also starts the `Release build` workflow
(`.github/workflows/release.yml`). It builds the same six archives from the
tag with `scripts/build-release.sh` on Go 1.24.1, the Go release that built
the published archives (archives are byte-identical only with the same Go;
change the workflow and this line together), and records a build provenance
attestation for every archive and for `SHA256SUMS`. When the release already
exists, the workflow compares its `SHA256SUMS` with the release's and fails on
any difference; otherwise compare them yourself before creating the release:

```sh
gh run download --repo kogriv/replan --name release-archives --dir /tmp/ci-release <run id>
diff /tmp/ci-release/SHA256SUMS "$REPLAN_RELEASE_DIR/SHA256SUMS"
```

Because the bytes are identical, the attestation made in CI covers the
archives uploaded from the local build. Anyone can then check a download:

```sh
gh attestation verify "replan_${version}_linux_amd64.tar.gz" --repo kogriv/replan
```

A manual run of the workflow with a tag rebuilds that release and compares
it; with `attest` set it attests a published release, only after its archives
matched byte for byte. v0.2.0 was attested that way on 2026-10-30 (run
41038369789); v0.1.0 has no attestation.

## Install with Go

Once the repository is public and the tag has been pushed, users can install a
specific release directly from the module:

```sh
go install github.com/kogriv/replan/cmd/replan@v0.1.0
replan --version
```

`go install` does not receive release linker flags. A module download records
the requested module version but does not include VCS settings. The compact
line therefore reports the version, toolchain and platform without a commit or
date, for example `replan v0.1.0 (go1.24.1 linux/amd64)`. The JSON result keeps
the same fields as a release archive and reports `"commit": "unknown"` and
`"build_date": "unknown"`.
