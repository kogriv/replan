#!/usr/bin/env sh
# Measure GitHub Actions run consumption using GitHub's per-job rounding.
#
# Usage:
#   scripts/github-actions-run-minutes.sh --run <id> [--run <id> ...] [--repo owner/name] [--allow-incomplete]
#
# Settings:
#   REPLAN_ENV_FILE          default empty; environment only
#   REPLAN_GITHUB_REPOSITORY  default derived from git origin
#
# REPLAN_GITHUB_REPOSITORY supplies the repository when --repo is omitted;
# otherwise the helper derives owner/name from git remote get-url origin.
# This helper is read-only. It prints the shared JSON helper envelope.
# Exits 0 on success, 6 when a GitHub request fails, 8 for usage or a missing
# gh executable, 10 for an internal failure, and 11 for an incomplete run
# unless --allow-incomplete is present.
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  replan_python -m github_actions_run_minutes "$@"
