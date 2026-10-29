#!/usr/bin/env sh
# Validate the full RePlan sidecar loop against a configured live OpenProject.
#
# Settings:
#   REPLAN_PROJECT                               required
#   REPLAN_OPENPROJECT_PROFILE                   default openproject.demo
#   REPLAN_OPENPROJECT_BASE_URL                  default empty
#   REPLAN_OPENPROJECT_PROJECT_ID                default empty
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID         deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TOKEN_ENV                 default OPENPROJECT_TOKEN
#   OPENPROJECT_TOKEN                            default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_PERIMETER_KEY             default empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER          default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS           default 60
#   REPLAN_OPENPROJECT_RETRIES                   default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS       default 2
#   REPLAN_OPENPROJECT_HOST_HEADER               default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO           default empty
#   REPLAN_OPENPROJECT_SINCE                     default empty
#   REPLAN_OPENPROJECT_INCLUDE_CLOSED            default 1
#   REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES      default 1
#   REPLAN_OPENPROJECT_INCLUDE_RELATIONS         default 1
#   REPLAN_OPENPROJECT_RESOURCE_LINKS_SIDECAR    default empty
#   REPLAN_OPENPROJECT_RESOURCE_DEMANDS_SIDECAR  default empty
#   REPLAN_BIN                                   default build locally
#   REPLAN_FORMAT                                json prints the helper envelope; text prints a developer summary
#   REPLAN_KEEP_TEMP                             default empty
#   TMPDIR                                       default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/validate-openproject-live.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "validate-openproject-live.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "validate-openproject-live" invalid
PROJECT_PATH="${REPLAN_PROJECT:-}"
OPENPROJECT_PROFILE="${REPLAN_OPENPROJECT_PROFILE:-openproject.demo}"
OPENPROJECT_BASE_URL="${REPLAN_OPENPROJECT_BASE_URL:-}"
OPENPROJECT_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
OPENPROJECT_HOST_HEADER="${REPLAN_OPENPROJECT_HOST_HEADER:-}"
OPENPROJECT_FORWARDED_PROTO="${REPLAN_OPENPROJECT_FORWARDED_PROTO:-}"
OPENPROJECT_SINCE="${REPLAN_OPENPROJECT_SINCE:-}"
OPENPROJECT_INCLUDE_CLOSED="${REPLAN_OPENPROJECT_INCLUDE_CLOSED:-1}"
OPENPROJECT_INCLUDE_TIME_ENTRIES="${REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES:-1}"
OPENPROJECT_INCLUDE_RELATIONS="${REPLAN_OPENPROJECT_INCLUDE_RELATIONS:-1}"
OPENPROJECT_RESOURCE_LINKS_SIDECAR="${REPLAN_OPENPROJECT_RESOURCE_LINKS_SIDECAR:-}"
OPENPROJECT_RESOURCE_DEMANDS_SIDECAR="${REPLAN_OPENPROJECT_RESOURCE_DEMANDS_SIDECAR:-}"
FORMAT="${REPLAN_FORMAT:-json}"
KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"
replan_require_settings "validate-openproject-live" REPLAN_PROJECT

# fail CODE MESSAGE EXIT: the shared envelope in JSON, the message in text.
fail() {
  if [ "$FORMAT" = "json" ]; then
    replan_fail_json validate-openproject-live invalid "$1" "$2" "$3"
  fi
  printf '%s\n' "$2" >&2
  exit "$3"
}

case "$FORMAT" in
  json|text) ;;
  *) FORMAT=json; fail unsupported_format "REPLAN_FORMAT must be json or text" 8 ;;
esac
if [ -n "$OPENPROJECT_BASE_URL" ] || [ -n "${OPENPROJECT_PROJECT_ID:-}" ]; then
  if [ -z "$OPENPROJECT_BASE_URL" ] || [ -z "${OPENPROJECT_PROJECT_ID:-}" ]; then
    fail missing_setting "REPLAN_OPENPROJECT_BASE_URL and REPLAN_OPENPROJECT_PROJECT_ID must be set together" 8
  fi
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-openproject-live-validation.XXXXXX")"
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

json_get() {
  file="$1"
  expr="$2"
  python3 -c 'import json, sys; data=json.load(open(sys.argv[1])); print(eval(sys.argv[2], {}, {"data": data}))' "$file" "$expr"
}

normalize_optional_path() {
  path="$1"
  if [ -z "$path" ]; then
    return 0
  fi
  case "$path" in
    /*)
      printf '%s\n' "$path"
      ;;
    *)
      if [ -f "$path" ]; then
        dir="$(dirname -- "$path")"
        file="$(basename -- "$path")"
        printf '%s/%s\n' "$(CDPATH= cd -- "$dir" && pwd -P)" "$file"
      elif [ -f "$ROOT_DIR/$path" ]; then
        dir="$(dirname -- "$ROOT_DIR/$path")"
        file="$(basename -- "$path")"
        printf '%s/%s\n' "$(CDPATH= cd -- "$dir" && pwd -P)" "$file"
      else
        printf '%s\n' "$path"
      fi
      ;;
  esac
}

OPENPROJECT_RESOURCE_LINKS_SIDECAR="$(normalize_optional_path "$OPENPROJECT_RESOURCE_LINKS_SIDECAR")"
OPENPROJECT_RESOURCE_DEMANDS_SIDECAR="$(normalize_optional_path "$OPENPROJECT_RESOURCE_DEMANDS_SIDECAR")"

json_summary() {
  file="$1"
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m envelope_check import-summary "$file"
}

assert_json_ok() {
  file="$1"
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m envelope_check require-ok "$file" --with-warnings \
    || fail step_failed "a replan step returned ok false: $(basename "$file")" 1
}

copy_project() {
  target="$1"
  cp -a "$PROJECT_PATH" "$target"
}

set_openproject_ids_canonical() {
  project="$1"
  # This disposable copy tests import machinery, not the private-id policy.
  sed -i 's/^  openproject_ids: private_bindings$/  openproject_ids: canonical/' "$project/.replan/project.yaml"
}

build_live_import_args() {
  project="$1"
  dry_run="$2"
  set -- "$BIN" import openproject --project "$project"
  if [ -n "$OPENPROJECT_BASE_URL" ] || [ -n "$OPENPROJECT_PROJECT_ID" ]; then
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
  if [ -n "$OPENPROJECT_SINCE" ]; then
    set -- "$@" --since "$OPENPROJECT_SINCE"
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
  if [ -n "$OPENPROJECT_RESOURCE_LINKS_SIDECAR" ]; then
    set -- "$@" --resource-links-sidecar "$OPENPROJECT_RESOURCE_LINKS_SIDECAR"
  fi
  if [ -n "$OPENPROJECT_RESOURCE_DEMANDS_SIDECAR" ]; then
    set -- "$@" --resource-demands-sidecar "$OPENPROJECT_RESOURCE_DEMANDS_SIDECAR"
  fi
  if [ "$dry_run" = "1" ]; then
    set -- "$@" --dry-run
  fi
  set -- "$@" --format json
  printf '%s\n' "$@"
}

run_live_import() {
  output="$1"
  project="$2"
  dry_run="$3"
  args_file="$TMP_DIR/live-import-args.txt"
  build_live_import_args "$project" "$dry_run" >"$args_file"
  import_exit=0
  # shellcheck disable=SC2046
  "$(sed -n '1p' "$args_file")" $(sed -n '2,$p' "$args_file") >"$output" || import_exit=$?
  if [ "$import_exit" -ne 0 ]; then
    if [ "$FORMAT" = "json" ]; then
      PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_import_failure --command validate-openproject-live --status-key live_api_validation --status-value failed --import-json "$output" --tmp-dir "$TMP_DIR" --root-dir "$ROOT_DIR" --profile --import-exit "$import_exit" || exit $?
    fi
    printf 'OpenProject live validation failed during live import\n' >&2
    exit 6
  fi
  assert_json_ok "$output"
}

token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
if [ -z "$token_value" ]; then
  if [ "$FORMAT" = "json" ]; then
    replan_token_skip validate-openproject-live "$OPENPROJECT_TOKEN_ENV" live_api_validation=skipped
  else
    printf 'OpenProject live validation skipped: token env %s is not set\n' "$OPENPROJECT_TOKEN_ENV"
  fi
  exit 0
fi

READONLY="$TMP_DIR/live-readonly.json"
run_live_import "$READONLY" "$PROJECT_PATH" "1"
readonly_summary="$(json_summary "$READONLY")"

LIVE_PROJECT="$TMP_DIR/live-project"
copy_project "$LIVE_PROJECT"
set_openproject_ids_canonical "$LIVE_PROJECT"
WRITE1="$TMP_DIR/live-write-before-accept.json"
run_live_import "$WRITE1" "$LIVE_PROJECT" "0"
snapshot1="$(json_get "$WRITE1" 'data["result"]["snapshot"]["id"]')"

ACCEPT_TASKS="$TMP_DIR/accept-tasks-dry-run.json"
"$BIN" import accept \
  --project "$LIVE_PROJECT" \
  --dry-run \
  --format json \
  "$snapshot1" >"$ACCEPT_TASKS"
assert_json_ok "$ACCEPT_TASKS"
task_accept_changes="$(json_get "$ACCEPT_TASKS" 'len(data["result"].get("changes") or [])')"

ACCEPT_DEPENDENCIES="$TMP_DIR/accept-dependencies-dry-run.json"
"$BIN" import accept \
  --project "$LIVE_PROJECT" \
  --accept-dependencies \
  --dry-run \
  --format json \
  "$snapshot1" >"$ACCEPT_DEPENDENCIES"
assert_json_ok "$ACCEPT_DEPENDENCIES"
dependency_accept_changes="$(json_get "$ACCEPT_DEPENDENCIES" 'len(data["result"].get("changes") or [])')"

ACCEPT_TIME="$TMP_DIR/accept-time-dry-run.json"
"$BIN" import accept \
  --project "$LIVE_PROJECT" \
  --accept-time-entries \
  --dry-run \
  --format json \
  "$snapshot1" >"$ACCEPT_TIME"
assert_json_ok "$ACCEPT_TIME"
time_accept_changes="$(json_get "$ACCEPT_TIME" 'len(data["result"].get("changes") or [])')"

ACCEPT_DEMANDS="$TMP_DIR/accept-resource-demands-dry-run.json"
"$BIN" import accept \
  --project "$LIVE_PROJECT" \
  --accept-resource-demands \
  --dry-run \
  --format json \
  "$snapshot1" >"$ACCEPT_DEMANDS"
assert_json_ok "$ACCEPT_DEMANDS"
resource_demand_accept_changes="$(json_get "$ACCEPT_DEMANDS" 'len(data["result"].get("changes") or [])')"

if [ "$FORMAT" = "json" ]; then
  if [ -n "$KEEP_TEMP" ]; then
    printf '{"ok":true,"command":"validate-openproject-live","live_api_validation":"passed","project_path":%s,"profile":%s,"readonly_summary":%s,"task_accept_dry_run_changes":%s,"dependency_accept_dry_run_changes":%s,"time_accept_dry_run_changes":%s,"resource_demand_accept_dry_run_changes":%s,"temp_retained":true,"outputs_dir":%s}\n' \
      "$(json_quote "$PROJECT_PATH")" \
      "$(json_quote "$OPENPROJECT_PROFILE")" \
      "$readonly_summary" \
      "$task_accept_changes" \
      "$dependency_accept_changes" \
      "$time_accept_changes" \
      "$resource_demand_accept_changes" \
      "$(json_quote "$TMP_DIR")"
  else
    printf '{"ok":true,"command":"validate-openproject-live","live_api_validation":"passed","project_path":%s,"profile":%s,"readonly_summary":%s,"task_accept_dry_run_changes":%s,"dependency_accept_dry_run_changes":%s,"time_accept_dry_run_changes":%s,"resource_demand_accept_dry_run_changes":%s,"temp_retained":false}\n' \
      "$(json_quote "$PROJECT_PATH")" \
      "$(json_quote "$OPENPROJECT_PROFILE")" \
      "$readonly_summary" \
      "$task_accept_changes" \
      "$dependency_accept_changes" \
      "$time_accept_changes" \
      "$resource_demand_accept_changes"
  fi
else
  printf 'OpenProject live validation passed\n'
  printf 'read-only summary: %s\n' "$readonly_summary"
  printf 'task accept dry-run changes: %s\n' "$task_accept_changes"
  printf 'dependency accept dry-run changes: %s\n' "$dependency_accept_changes"
  printf 'time accept dry-run changes: %s\n' "$time_accept_changes"
  printf 'resource-demand accept dry-run changes: %s\n' "$resource_demand_accept_changes"
fi
