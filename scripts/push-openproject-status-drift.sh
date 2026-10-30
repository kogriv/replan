#!/usr/bin/env sh
# Plan or push approved RePlan tracking-status drift to OpenProject.
#
# Settings:
#   REPLAN_PROJECT                          required
#   REPLAN_OPENPROJECT_BASE_URL             required
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID    deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TOKEN_ENV            default OPENPROJECT_TOKEN
#   REPLAN_OPENPROJECT_TOKEN_FILE           default empty; used when the selected token environment value is empty
#   OPENPROJECT_TOKEN                       default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_PERIMETER_KEY        default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY_FILE   default empty; used when REPLAN_OPENPROJECT_PERIMETER_KEY is empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER     default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS      default 60
#   REPLAN_OPENPROJECT_RETRIES              default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS  default 2
#   REPLAN_OPENPROJECT_HOST_HEADER          default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO      default empty
#   REPLAN_BIN                              default build locally
#   REPLAN_KEEP_TEMP                        default empty
#   REPLAN_FORMAT                           only json; any other value is refused
#   REPLAN_APPLY                            default empty
#   TMPDIR                                  default /tmp
#
# Writes only with REPLAN_APPLY=1; otherwise it is a dry run.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/push-openproject-status-drift.sh
#   REPLAN_APPLY=1 scripts/push-openproject-status-drift.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "push-openproject-status-drift" "push-openproject-status-drift.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "push-openproject-status-drift" invalid
PROJECT_PATH="${REPLAN_PROJECT:-}"
OPENPROJECT_BASE_URL="${REPLAN_OPENPROJECT_BASE_URL:-}"
OPENPROJECT_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
OPENPROJECT_HOST_HEADER="${REPLAN_OPENPROJECT_HOST_HEADER:-}"
OPENPROJECT_FORWARDED_PROTO="${REPLAN_OPENPROJECT_FORWARDED_PROTO:-}"
OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"

replan_require_json_format "push-openproject-status-drift"
replan_require_settings "push-openproject-status-drift" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-openproject-status-push.XXXXXX")"
cleanup() {
  if [ -z "$KEEP_TEMP" ]; then
    rm -rf "$TMP_DIR"
  else
    printf 'kept temp dir: %s\n' "$TMP_DIR" >&2
  fi
}
trap cleanup EXIT INT TERM

BIN="${REPLAN_BIN:-}"
if [ -z "$BIN" ]; then
  BIN="$TMP_DIR/replan"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./cmd/replan)
fi

json_quote() {
  python3 -c 'import json, sys; print(json.dumps(sys.argv[1]))' "$1"
}

token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
if [ -z "$token_value" ]; then
  replan_token_missing "push-openproject-status-drift" "$OPENPROJECT_TOKEN_ENV"
fi

if [ -z "$OPENPROJECT_BASE_URL" ]; then
  replan_fail_json "push-openproject-status-drift" dry_run missing_base_url "REPLAN_OPENPROJECT_BASE_URL is required" 8
fi

IMPORT_JSON="$TMP_DIR/live-import.json"
set -- "$BIN" import openproject \
  --project "$PROJECT_PATH" \
  --base-url "$OPENPROJECT_BASE_URL" \
  --project-id "$OPENPROJECT_PROJECT_ID" \
  --token-env "$OPENPROJECT_TOKEN_ENV"
if [ -n "$OPENPROJECT_HOST_HEADER" ]; then
  set -- "$@" --host-header "$OPENPROJECT_HOST_HEADER"
fi
if [ -n "$OPENPROJECT_FORWARDED_PROTO" ]; then
  set -- "$@" --forwarded-proto "$OPENPROJECT_FORWARDED_PROTO"
fi
set -- "$@" --include-closed --include-relations --dry-run --format json
import_exit=0
"$@" >"$IMPORT_JSON" || import_exit=$?
if [ "$import_exit" -ne 0 ]; then
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_import_failure --command push-openproject-status-drift --status-key mode --status-value failed --import-json "$IMPORT_JSON" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR" --bindings --import-exit "$import_exit" || exit $?
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m push_openproject_status_drift --import-json "$IMPORT_JSON" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR"
