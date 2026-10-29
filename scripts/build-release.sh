#!/usr/bin/env bash
# Build deterministic local RePlan release archives from the commit at HEAD.
#
# Settings:
#   REPLAN_RELEASE_VERSION      default exact tag at HEAD; refused when absent
#   REPLAN_RELEASE_DIR          required
#   REPLAN_RELEASE_TARGETS      default linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64
#   REPLAN_RELEASE_ALLOW_DIRTY  default empty
#   TMPDIR                      default /tmp
#
# Gate: refuses a missing tag, dirty tracked files or untracked non-ignored
# files unless REPLAN_RELEASE_ALLOW_DIRTY=1, and a non-empty output directory.
# Output: deterministic target archives, SHA256SUMS and one JSON helper
# envelope. It does not tag, push or upload anything.
#
#   REPLAN_RELEASE_DIR=dist/release scripts/build-release.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "build-release" "build-release.sh" "$@"
replan_require_settings "build-release" REPLAN_RELEASE_DIR

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  python3 -m build_release --root-dir "$ROOT_DIR"
