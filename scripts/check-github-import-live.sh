#!/usr/bin/env sh
# Check the GitHub Issues live import against a disposable public repository.
#
# Settings:
#   REPLAN_GITHUB_REPOSITORY  required
#   REPLAN_GITHUB_TOKEN_ENV   default GITHUB_TOKEN
#   REPLAN_BIN                default build locally
#   TMPDIR                    default /tmp
#
# Reads GitHub only. Writes import snapshots only in a temporary copy of
# examples/small-team, runs every supported import-accept mode as a dry run,
# and verifies that the GitHub-only time-entry mode is refused as documented.
# Prints one JSON helper envelope with imported and acceptance counts.
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "check-github-import-live" "check-github-import-live.sh" "$@"
REPOSITORY="${REPLAN_GITHUB_REPOSITORY:-}"
TOKEN_ENV="${REPLAN_GITHUB_TOKEN_ENV:-GITHUB_TOKEN}"

if [ -z "$REPOSITORY" ]; then
  replan_fail_json "check-github-import-live" invalid missing_setting "REPLAN_GITHUB_REPOSITORY is required" 8
fi
case "$REPOSITORY" in
  */*) ;;
  *)
    replan_fail_json "check-github-import-live" invalid invalid_setting "REPLAN_GITHUB_REPOSITORY must use owner/repository" 8
    ;;
esac

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-github-import-live.XXXXXX")"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM
PROJECT_PATH="$TMP_DIR/small-team"
cp -a "$ROOT_DIR/examples/small-team" "$PROJECT_PATH"

BIN="${REPLAN_BIN:-}"
if [ -z "$BIN" ]; then
  BIN="$TMP_DIR/replan"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./cmd/replan)
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  python3 -m check_github_import_live setup "$PROJECT_PATH/.replan/project.yaml" "$REPOSITORY" "$TOKEN_ENV"

IMPORT_JSON="$TMP_DIR/import.json"
if ! "$BIN" import github --project "$PROJECT_PATH" --profile github.live-check \
  --include-closed --include-relations --format json >"$IMPORT_JSON"; then
  cat "$IMPORT_JSON"
  exit 6
fi

SNAPSHOT_ID="$(PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m check_github_import_live snapshot-id "$IMPORT_JSON")"
ACCEPT_DIR="$TMP_DIR/accept"
mkdir -p "$ACCEPT_DIR"

run_accept() {
  name="$1"
  shift
  "$BIN" import accept "$SNAPSHOT_ID" --project "$PROJECT_PATH" "$@" \
    --dry-run --format json >"$ACCEPT_DIR/$name.json"
}

run_accept tasks
run_accept resource_links --accept-resource-links
run_accept tracking_status --accept-tracking-status
run_accept dates --accept-dates
run_accept dependencies --accept-dependencies
run_accept resource_demands --accept-resource-demands
time_entries_exit=0
"$BIN" import accept "$SNAPSHOT_ID" --project "$PROJECT_PATH" --accept-time-entries \
  --dry-run --format json >"$ACCEPT_DIR/time_entries.json" || time_entries_exit=$?
if [ "$time_entries_exit" -eq 0 ]; then
  printf '%s\n' 'GitHub time-entry acceptance unexpectedly succeeded' >&2
  exit 1
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  python3 -m check_github_import_live report "$IMPORT_JSON" "$ACCEPT_DIR"
