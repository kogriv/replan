#!/usr/bin/env sh
# Plan or push RePlan task-date drift back to OpenProject. Date PATCHes set
# ignoreNonWorkingDays only when a sent date uses an instance non-working weekday.
#
# Settings:
#   REPLAN_PROJECT                          required
#   REPLAN_OPENPROJECT_PROFILE              default openproject.demo
#   REPLAN_OPENPROJECT_BASE_URL             default empty
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID    deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TOKEN_ENV            default OPENPROJECT_TOKEN
#   OPENPROJECT_TOKEN                       default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_PERIMETER_KEY        default empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER     default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS      default 60
#   REPLAN_OPENPROJECT_RETRIES              default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS  default 2
#   REPLAN_OPENPROJECT_HOST_HEADER          default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO      default empty
#   REPLAN_OPENPROJECT_INCLUDE_CLOSED       default 1
#   REPLAN_OPENPROJECT_INCLUDE_RELATIONS    default 1
#   REPLAN_OPENPROJECT_IMPORT_JSON          default empty
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
#   scripts/push-openproject-date-drift.sh
#   REPLAN_APPLY=1 scripts/push-openproject-date-drift.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "push-openproject-date-drift" "push-openproject-date-drift.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "push-openproject-date-drift" invalid
PROJECT_PATH="${REPLAN_PROJECT:-}"
OPENPROJECT_PROFILE="${REPLAN_OPENPROJECT_PROFILE:-openproject.demo}"
OPENPROJECT_BASE_URL="${REPLAN_OPENPROJECT_BASE_URL:-}"
OPENPROJECT_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
OPENPROJECT_HOST_HEADER="${REPLAN_OPENPROJECT_HOST_HEADER:-}"
OPENPROJECT_FORWARDED_PROTO="${REPLAN_OPENPROJECT_FORWARDED_PROTO:-}"
OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
OPENPROJECT_INCLUDE_CLOSED="${REPLAN_OPENPROJECT_INCLUDE_CLOSED:-1}"
OPENPROJECT_INCLUDE_RELATIONS="${REPLAN_OPENPROJECT_INCLUDE_RELATIONS:-1}"
OPENPROJECT_IMPORT_JSON="${REPLAN_OPENPROJECT_IMPORT_JSON:-}"
KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"

replan_require_json_format "push-openproject-date-drift"
replan_require_settings "push-openproject-date-drift" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-openproject-date-push.XXXXXX")"
cleanup() {
  if [ -z "$KEEP_TEMP" ]; then
    rm -rf "$TMP_DIR"
  else
    printf 'kept temp dir: %s\n' "$TMP_DIR" >&2
  fi
}
trap cleanup EXIT INT TERM

json_quote() {
  python3 -c 'import json, sys; print(json.dumps(sys.argv[1]))' "$1"
}

if [ -n "$OPENPROJECT_IMPORT_JSON" ]; then
  IMPORT_JSON="$OPENPROJECT_IMPORT_JSON"
else
  token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
  if [ -z "$token_value" ]; then
    replan_token_missing "push-openproject-date-drift" "$OPENPROJECT_TOKEN_ENV"
  fi

  BIN="${REPLAN_BIN:-}"
  if [ -z "$BIN" ]; then
    BIN="$TMP_DIR/replan"
    (cd "$ROOT_DIR" && go build -o "$BIN" ./cmd/replan)
  fi

  IMPORT_JSON="$TMP_DIR/live-import.json"
  set -- "$BIN" import openproject --project "$PROJECT_PATH"
  if [ -n "$OPENPROJECT_BASE_URL" ] || [ -n "$OPENPROJECT_PROJECT_ID" ]; then
    if [ -z "$OPENPROJECT_BASE_URL" ] || [ -z "$OPENPROJECT_PROJECT_ID" ]; then
      replan_fail_json "push-openproject-date-drift" invalid missing_setting "REPLAN_OPENPROJECT_BASE_URL and REPLAN_OPENPROJECT_PROJECT_ID must be set together" 8
    fi
    set -- "$@" \
      --base-url "$OPENPROJECT_BASE_URL" \
      --project-id "$OPENPROJECT_PROJECT_ID" \
      --token-env "$OPENPROJECT_TOKEN_ENV"
    if [ -n "$OPENPROJECT_HOST_HEADER" ]; then
      set -- "$@" --host-header "$OPENPROJECT_HOST_HEADER"
    fi
    if [ -n "$OPENPROJECT_FORWARDED_PROTO" ]; then
      set -- "$@" --forwarded-proto "$OPENPROJECT_FORWARDED_PROTO"
    fi
  else
    set -- "$@" --profile "$OPENPROJECT_PROFILE"
  fi
  if [ -n "$OPENPROJECT_INCLUDE_CLOSED" ]; then
    set -- "$@" --include-closed
  fi
  if [ -n "$OPENPROJECT_INCLUDE_RELATIONS" ]; then
    set -- "$@" --include-relations
  fi
  set -- "$@" --dry-run --format json

  import_exit=0
  "$@" >"$IMPORT_JSON" || import_exit=$?
  if [ "$import_exit" -ne 0 ]; then
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_import_failure --command push-openproject-date-drift --status-key mode --status-value failed --import-json "$IMPORT_JSON" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR" --bindings --import-exit "$import_exit" || exit $?
  fi
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m push_openproject_date_drift --import-json "$IMPORT_JSON" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR"
