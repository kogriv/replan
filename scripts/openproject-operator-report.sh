#!/usr/bin/env sh
# Produce the combined OpenProject/RePlan operator report from live or saved import data.
#
# Settings:
#   REPLAN_PROJECT                           required
#   REPLAN_OPENPROJECT_PROFILE               default openproject.demo
#   REPLAN_OPENPROJECT_BASE_URL              default empty
#   REPLAN_OPENPROJECT_PROJECT_ID            required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID     deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TOKEN_ENV             default OPENPROJECT_TOKEN
#   REPLAN_OPENPROJECT_TOKEN_FILE           default empty; used when the selected token environment value is empty
#   OPENPROJECT_TOKEN                        default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_PERIMETER_KEY         default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY_FILE   default empty; used when REPLAN_OPENPROJECT_PERIMETER_KEY is empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER      default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS       default 60
#   REPLAN_OPENPROJECT_RETRIES               default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS   default 2
#   REPLAN_OPENPROJECT_HOST_HEADER           default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO       default empty
#   REPLAN_OPENPROJECT_FROM_FILE             default empty
#   REPLAN_OPENPROJECT_IMPORT_JSON           default empty
#   REPLAN_OPENPROJECT_INCLUDE_CLOSED        default 1
#   REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES  default 1
#   REPLAN_OPENPROJECT_INCLUDE_RELATIONS     default 1
#   REPLAN_BIN                               default build locally
#   REPLAN_KEEP_TEMP                         default empty
#   REPLAN_FORMAT                            only json; any other value is refused
#   TMPDIR                                   default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/openproject-operator-report.sh
#   REPLAN_OPENPROJECT_IMPORT_JSON=import.json scripts/openproject-operator-report.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "openproject operator report" "openproject-operator-report.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "openproject operator report" read_only
PROJECT_PATH="${REPLAN_PROJECT:-}"
OPENPROJECT_PROFILE="${REPLAN_OPENPROJECT_PROFILE:-openproject.demo}"
OPENPROJECT_BASE_URL="${REPLAN_OPENPROJECT_BASE_URL:-}"
OPENPROJECT_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
OPENPROJECT_HOST_HEADER="${REPLAN_OPENPROJECT_HOST_HEADER:-}"
OPENPROJECT_FORWARDED_PROTO="${REPLAN_OPENPROJECT_FORWARDED_PROTO:-}"
OPENPROJECT_FROM_FILE="${REPLAN_OPENPROJECT_FROM_FILE:-}"
OPENPROJECT_IMPORT_JSON="${REPLAN_OPENPROJECT_IMPORT_JSON:-}"
OPENPROJECT_INCLUDE_CLOSED="${REPLAN_OPENPROJECT_INCLUDE_CLOSED:-1}"
OPENPROJECT_INCLUDE_TIME_ENTRIES="${REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES:-1}"
OPENPROJECT_INCLUDE_RELATIONS="${REPLAN_OPENPROJECT_INCLUDE_RELATIONS:-1}"
KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"

replan_require_json_format "openproject operator report"
replan_require_settings "openproject operator report" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-openproject-operator-report.XXXXXX")"
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

if [ -n "$OPENPROJECT_IMPORT_JSON" ]; then
  IMPORT_JSON="$OPENPROJECT_IMPORT_JSON"
elif [ -n "$OPENPROJECT_FROM_FILE" ]; then
  IMPORT_JSON="$TMP_DIR/import.json"
  set -- "$BIN" import openproject \
    --project "$PROJECT_PATH" \
    --from-file "$OPENPROJECT_FROM_FILE"
  if [ -n "$OPENPROJECT_INCLUDE_CLOSED" ]; then
    set -- "$@" --include-closed
  fi
  if [ -n "$OPENPROJECT_INCLUDE_TIME_ENTRIES" ]; then
    set -- "$@" --include-time-entries
  fi
  if [ -n "$OPENPROJECT_INCLUDE_RELATIONS" ]; then
    set -- "$@" --include-relations
  fi
  set -- "$@" --dry-run --format json
  import_exit=0
  "$@" >"$IMPORT_JSON" || import_exit=$?
  if [ "$import_exit" -ne 0 ]; then
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_import_failure --command "openproject operator report" --status-key mode --status-value failed --import-json "$IMPORT_JSON" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR" --bindings --import-exit "$import_exit" || exit $?
  fi
else
  token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
  if [ -z "$token_value" ]; then
    replan_token_skip "openproject operator report" "$OPENPROJECT_TOKEN_ENV"
  fi

  IMPORT_JSON="$TMP_DIR/import.json"
  set -- "$BIN" import openproject --project "$PROJECT_PATH"
  if [ -n "$OPENPROJECT_BASE_URL" ] || [ -n "$OPENPROJECT_PROJECT_ID" ]; then
    if [ -z "$OPENPROJECT_BASE_URL" ] || [ -z "$OPENPROJECT_PROJECT_ID" ]; then
      replan_fail_json "openproject operator report" read_only missing_openproject_target "REPLAN_OPENPROJECT_BASE_URL and REPLAN_OPENPROJECT_PROJECT_ID must be set together" 8
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
  if [ -n "$OPENPROJECT_INCLUDE_TIME_ENTRIES" ]; then
    set -- "$@" --include-time-entries
  fi
  if [ -n "$OPENPROJECT_INCLUDE_RELATIONS" ]; then
    set -- "$@" --include-relations
  fi
  set -- "$@" --dry-run --format json
  import_exit=0
  "$@" >"$IMPORT_JSON" || import_exit=$?
  if [ "$import_exit" -ne 0 ]; then
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_import_failure --command "openproject operator report" --status-key mode --status-value failed --import-json "$IMPORT_JSON" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR" --bindings --import-exit "$import_exit" || exit $?
  fi
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_operator_report --import-json "$IMPORT_JSON" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR"
