#!/usr/bin/env sh
# Run the fixture/live OpenProject import, accept and idempotency sidecar pipeline.
#
# Settings:
#   REPLAN_PROJECT                               default examples/small-team
#   REPLAN_OPENPROJECT_PROFILE                   default openproject.demo
#   REPLAN_OPENPROJECT_BASE_URL                  default empty
#   REPLAN_OPENPROJECT_PROJECT_ID                default empty
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID         deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_HOST_HEADER               default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO           default empty
#   REPLAN_OPENPROJECT_SINCE                     default empty
#   REPLAN_OPENPROJECT_INCLUDE_CLOSED            default 1
#   REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES      default 1
#   REPLAN_OPENPROJECT_TOKEN_ENV                 default OPENPROJECT_TOKEN
#   REPLAN_OPENPROJECT_TOKEN_FILE           default empty; used when the selected token environment value is empty
#   OPENPROJECT_TOKEN                            default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_PERIMETER_KEY             default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY_FILE   default empty; used when REPLAN_OPENPROJECT_PERIMETER_KEY is empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER          default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS           default 60
#   REPLAN_OPENPROJECT_RETRIES                   default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS       default 2
#   REPLAN_OPENPROJECT_RESOURCE_LINKS_SIDECAR    default empty
#   REPLAN_OPENPROJECT_RESOURCE_DEMANDS_SIDECAR  default empty
#   REPLAN_OPENPROJECT_ACCEPT_TRACKING_STATUS    default empty
#   REPLAN_ACTOR                                 default actor.human.operator
#   REPLAN_FORMAT                                json prints the helper envelope; text prints a developer summary
#   REPLAN_KEEP_TEMP                             default empty
#   REPLAN_EXPECT_RESOURCE_LINKS_MIN             default 1
#   REPLAN_EXPECT_RESOURCE_DEMANDS_MIN           default 1
#   REPLAN_EXPECT_TIME_ACCEPT_CHANGES_MIN        default 1
#   REPLAN_EXPECT_TRACKING_STATUS_CHANGES_MIN    default 0
#   REPLAN_BIN                                   default build locally
#   TMPDIR                                       default /tmp
#
# Does not write a live OpenProject; it writes only local or disposable
# artifacts.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/run-openproject-sidecar-pipeline.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "openproject-sidecar-pipeline" "run-openproject-sidecar-pipeline.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "openproject-sidecar-pipeline" invalid
PROJECT_PATH="${REPLAN_PROJECT:-$ROOT_DIR/examples/small-team}"
OPENPROJECT_PROFILE="${REPLAN_OPENPROJECT_PROFILE:-openproject.demo}"
OPENPROJECT_BASE_URL="${REPLAN_OPENPROJECT_BASE_URL:-}"
OPENPROJECT_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
OPENPROJECT_HOST_HEADER="${REPLAN_OPENPROJECT_HOST_HEADER:-}"
OPENPROJECT_FORWARDED_PROTO="${REPLAN_OPENPROJECT_FORWARDED_PROTO:-}"
OPENPROJECT_SINCE="${REPLAN_OPENPROJECT_SINCE:-}"
OPENPROJECT_INCLUDE_CLOSED="${REPLAN_OPENPROJECT_INCLUDE_CLOSED:-1}"
OPENPROJECT_INCLUDE_TIME_ENTRIES="${REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES:-1}"
OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
RESOURCE_LINKS_SIDECAR="${REPLAN_OPENPROJECT_RESOURCE_LINKS_SIDECAR:-}"
RESOURCE_DEMANDS_SIDECAR="${REPLAN_OPENPROJECT_RESOURCE_DEMANDS_SIDECAR:-}"
ACTOR="${REPLAN_ACTOR:-actor.human.operator}"
FORMAT="${REPLAN_FORMAT:-json}"
KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"
EXPECT_RESOURCE_LINKS_MIN="${REPLAN_EXPECT_RESOURCE_LINKS_MIN:-1}"
EXPECT_RESOURCE_DEMANDS_MIN="${REPLAN_EXPECT_RESOURCE_DEMANDS_MIN:-1}"
EXPECT_TIME_ACCEPT_CHANGES_MIN="${REPLAN_EXPECT_TIME_ACCEPT_CHANGES_MIN:-1}"
ACCEPT_TRACKING_STATUS="${REPLAN_OPENPROJECT_ACCEPT_TRACKING_STATUS:-}"
EXPECT_TRACKING_STATUS_CHANGES_MIN="${REPLAN_EXPECT_TRACKING_STATUS_CHANGES_MIN:-0}"

# fail CODE MESSAGE EXIT: the shared envelope in JSON, the message in text.
fail() {
  if [ "$FORMAT" = "json" ]; then
    replan_fail_json openproject-sidecar-pipeline invalid "$1" "$2" "$3"
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

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-openproject-sidecar-pipeline.XXXXXX")"
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

assert_json_ok() {
  file="$1"
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m envelope_check require-ok "$file" \
    || fail step_failed "a replan step returned ok false: $(basename "$file")" 1
}

copy_project() {
  target="$1"
  cp -a "$PROJECT_PATH" "$target"
}

copy_sidecar_if_requested() {
  source_path="$1"
  target_rel="$2"
  if [ -n "$source_path" ]; then
    mkdir -p "$(dirname "$PIPELINE_PROJECT/$target_rel")"
    cp "$source_path" "$PIPELINE_PROJECT/$target_rel"
  fi
}

run_openproject_import() {
  output="$1"
  if [ -n "$OPENPROJECT_BASE_URL" ] || [ -n "$OPENPROJECT_PROJECT_ID" ]; then
    set -- "$BIN" import openproject \
      --project "$PIPELINE_PROJECT" \
      --base-url "$OPENPROJECT_BASE_URL" \
      --project-id "$OPENPROJECT_PROJECT_ID" \
      --token-env "$OPENPROJECT_TOKEN_ENV" \
      --format json
    if [ -n "$OPENPROJECT_HOST_HEADER" ]; then
      set -- "$@" --host-header "$OPENPROJECT_HOST_HEADER"
    fi
    if [ -n "$OPENPROJECT_FORWARDED_PROTO" ]; then
      set -- "$@" --forwarded-proto "$OPENPROJECT_FORWARDED_PROTO"
    fi
  else
    set -- "$BIN" import openproject \
      --project "$PIPELINE_PROJECT" \
      --profile "$OPENPROJECT_PROFILE" \
      --format json
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
  "$@" >"$output"
  assert_json_ok "$output"
}

run_accept_tasks() {
  snapshot_id="$1"
  output="$2"
  "$BIN" import accept \
    --project "$PIPELINE_PROJECT" \
    --actor "$ACTOR" \
    --apply \
    --format json \
    "$snapshot_id" >"$output"
  assert_json_ok "$output"
}

run_accept_mode() {
  mode="$1"
  snapshot_id="$2"
  output="$3"
  "$BIN" import accept \
    --project "$PIPELINE_PROJECT" \
    --actor "$ACTOR" \
    --apply \
    "$mode" \
    --format json \
    "$snapshot_id" >"$output"
  assert_json_ok "$output"
}

assert_min_applied_changes() {
  file="$1"
  min_count="$2"
  label="$3"
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m envelope_check min-applied-changes "$file" "$min_count" "$label" \
    || fail step_failed "$label applied fewer changes than $min_count" 1
}

TOKEN_VALUE="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
if [ -z "$TOKEN_VALUE" ]; then
  if [ "$FORMAT" = "json" ]; then
    replan_token_skip openproject-sidecar-pipeline "$OPENPROJECT_TOKEN_ENV" live_pipeline=skipped
  else
    printf 'OpenProject sidecar pipeline skipped: token env %s is not set\n' "$OPENPROJECT_TOKEN_ENV"
  fi
  exit 0
fi

PIPELINE_PROJECT="$TMP_DIR/project"
copy_project "$PIPELINE_PROJECT"
copy_sidecar_if_requested "$RESOURCE_LINKS_SIDECAR" "integrations/openproject/resource-links.yaml"
copy_sidecar_if_requested "$RESOURCE_DEMANDS_SIDECAR" "integrations/openproject/resource-demands.yaml"

if [ ! -s "$PIPELINE_PROJECT/integrations/openproject/resource-links.yaml" ]; then
  fail sidecar_not_found "missing resource links sidecar integrations/openproject/resource-links.yaml; set REPLAN_OPENPROJECT_RESOURCE_LINKS_SIDECAR or add the sidecar to the source project copy" 4
fi
if [ ! -s "$PIPELINE_PROJECT/integrations/openproject/resource-demands.yaml" ]; then
  fail sidecar_not_found "missing resource demands sidecar integrations/openproject/resource-demands.yaml; set REPLAN_OPENPROJECT_RESOURCE_DEMANDS_SIDECAR or add the sidecar to the source project copy" 4
fi

IMPORT1="$TMP_DIR/import1.json"
ACCEPT_TASKS="$TMP_DIR/accept-tasks.json"
ACCEPT_LINKS="$TMP_DIR/accept-resource-links.json"
IMPORT2="$TMP_DIR/import2.json"
ACCEPT_DEMANDS="$TMP_DIR/accept-resource-demands.json"
ACCEPT_TIME="$TMP_DIR/accept-time-entries.json"
ACCEPT_STATUS="$TMP_DIR/accept-tracking-status.json"
VALIDATE="$TMP_DIR/validate.json"

run_openproject_import "$IMPORT1"
SNAPSHOT1="$(json_get "$IMPORT1" 'data["result"]["snapshot"]["id"]')"
run_accept_tasks "$SNAPSHOT1" "$ACCEPT_TASKS"
run_accept_mode "--accept-resource-links" "$SNAPSHOT1" "$ACCEPT_LINKS"
assert_min_applied_changes "$ACCEPT_LINKS" "$EXPECT_RESOURCE_LINKS_MIN" "resource-link accept"

run_openproject_import "$IMPORT2"
SNAPSHOT2="$(json_get "$IMPORT2" 'data["result"]["snapshot"]["id"]')"
run_accept_mode "--accept-resource-demands" "$SNAPSHOT2" "$ACCEPT_DEMANDS"
assert_min_applied_changes "$ACCEPT_DEMANDS" "$EXPECT_RESOURCE_DEMANDS_MIN" "resource-demand accept"
run_accept_mode "--accept-time-entries" "$SNAPSHOT2" "$ACCEPT_TIME"
assert_min_applied_changes "$ACCEPT_TIME" "$EXPECT_TIME_ACCEPT_CHANGES_MIN" "time-entry accept"
TRACKING_STATUS_CHANGES=0
if [ -n "$ACCEPT_TRACKING_STATUS" ]; then
  run_accept_mode "--accept-tracking-status" "$SNAPSHOT2" "$ACCEPT_STATUS"
  assert_min_applied_changes "$ACCEPT_STATUS" "$EXPECT_TRACKING_STATUS_CHANGES_MIN" "tracking-status accept"
  TRACKING_STATUS_CHANGES="$(json_get "$ACCEPT_STATUS" 'len(data["result"].get("applied_changes", []))')"
fi

"$BIN" validate --project "$PIPELINE_PROJECT" --format json >"$VALIDATE"
assert_json_ok "$VALIDATE"

RESOURCE_LINK_CHANGES="$(json_get "$ACCEPT_LINKS" 'len(data["result"].get("applied_changes", []))')"
RESOURCE_DEMAND_CHANGES="$(json_get "$ACCEPT_DEMANDS" 'len(data["result"].get("applied_changes", []))')"
TIME_ACCEPT_CHANGES="$(json_get "$ACCEPT_TIME" 'len(data["result"].get("applied_changes", []))')"
WORK_PACKAGES="$(json_get "$IMPORT1" 'len(data["result"].get("task_candidates", []))')"
RESOURCE_CANDIDATES="$(json_get "$IMPORT1" 'len(data["result"].get("resource_candidates", []))')"
TIME_ENTRIES="$(json_get "$IMPORT1" 'len(data["result"].get("time_entry_candidates", []))')"

if [ "$FORMAT" = "json" ]; then
  if [ -n "$KEEP_TEMP" ]; then
    printf '{"ok":true,"command":"openproject-sidecar-pipeline","live_pipeline":"passed","project_path":%s,"pipeline_project":%s,"snapshot1":%s,"snapshot2":%s,"work_package_candidates":%s,"resource_candidates":%s,"time_entry_candidates":%s,"resource_link_changes":%s,"resource_demand_changes":%s,"time_accept_changes":%s,"tracking_status_changes":%s,"validation":"passed","temp_retained":true,"outputs_dir":%s}\n' \
      "$(json_quote "$PROJECT_PATH")" \
      "$(json_quote "$PIPELINE_PROJECT")" \
      "$(json_quote "$SNAPSHOT1")" \
      "$(json_quote "$SNAPSHOT2")" \
      "$WORK_PACKAGES" \
      "$RESOURCE_CANDIDATES" \
      "$TIME_ENTRIES" \
      "$RESOURCE_LINK_CHANGES" \
      "$RESOURCE_DEMAND_CHANGES" \
      "$TIME_ACCEPT_CHANGES" \
      "$TRACKING_STATUS_CHANGES" \
      "$(json_quote "$TMP_DIR")"
  else
    printf '{"ok":true,"command":"openproject-sidecar-pipeline","live_pipeline":"passed","project_path":%s,"snapshot1":%s,"snapshot2":%s,"work_package_candidates":%s,"resource_candidates":%s,"time_entry_candidates":%s,"resource_link_changes":%s,"resource_demand_changes":%s,"time_accept_changes":%s,"tracking_status_changes":%s,"validation":"passed","temp_retained":false}\n' \
      "$(json_quote "$PROJECT_PATH")" \
      "$(json_quote "$SNAPSHOT1")" \
      "$(json_quote "$SNAPSHOT2")" \
      "$WORK_PACKAGES" \
      "$RESOURCE_CANDIDATES" \
      "$TIME_ENTRIES" \
      "$RESOURCE_LINK_CHANGES" \
      "$RESOURCE_DEMAND_CHANGES" \
      "$TIME_ACCEPT_CHANGES" \
      "$TRACKING_STATUS_CHANGES"
  fi
else
  printf 'OpenProject sidecar pipeline passed\n'
  printf 'snapshot 1: %s\n' "$SNAPSHOT1"
  printf 'snapshot 2: %s\n' "$SNAPSHOT2"
  printf 'work package candidates: %s\n' "$WORK_PACKAGES"
  printf 'resource candidates: %s\n' "$RESOURCE_CANDIDATES"
  printf 'time-entry candidates: %s\n' "$TIME_ENTRIES"
  printf 'resource-link changes: %s\n' "$RESOURCE_LINK_CHANGES"
  printf 'resource-demand changes: %s\n' "$RESOURCE_DEMAND_CHANGES"
  printf 'time accept changes: %s\n' "$TIME_ACCEPT_CHANGES"
  printf 'tracking status changes: %s\n' "$TRACKING_STATUS_CHANGES"
fi
