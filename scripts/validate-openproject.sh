#!/usr/bin/env sh
# Validate fixture and optional live OpenProject import/accept workflows.
#
# Settings:
#   REPLAN_PROJECT                           default examples/small-team
#   REPLAN_OPENPROJECT_SAMPLE                default examples/imports/openproject/work-packages.sample.json
#   REPLAN_OPENPROJECT_PROFILE               default openproject.demo
#   REPLAN_OPENPROJECT_BASE_URL              default empty
#   REPLAN_OPENPROJECT_PROJECT_ID            default empty
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID     deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TOKEN_ENV             default OPENPROJECT_TOKEN
#   OPENPROJECT_TOKEN                        default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_PERIMETER_KEY         default empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER      default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS       default 60
#   REPLAN_OPENPROJECT_RETRIES               default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS   default 2
#   REPLAN_OPENPROJECT_HOST_HEADER           default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO       default empty
#   REPLAN_OPENPROJECT_SINCE                 default empty
#   REPLAN_OPENPROJECT_INCLUDE_CLOSED        default empty
#   REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES  default empty
#   REPLAN_OPENPROJECT_INCLUDE_RELATIONS     default empty
#   REPLAN_OPENPROJECT_DEPENDENCY_DRY_RUN    default empty
#   REPLAN_BIN                               default build locally
#   REPLAN_FORMAT                            json prints the helper envelope; text prints a developer summary
#   REPLAN_KEEP_TEMP                         default empty
#   TMPDIR                                   default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/validate-openproject.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "validate-openproject.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "validate-openproject" invalid
PROJECT_PATH="${REPLAN_PROJECT:-$ROOT_DIR/examples/small-team}"
OPENPROJECT_SAMPLE="${REPLAN_OPENPROJECT_SAMPLE:-$ROOT_DIR/examples/imports/openproject/work-packages.sample.json}"
OPENPROJECT_PROFILE="${REPLAN_OPENPROJECT_PROFILE:-openproject.demo}"
OPENPROJECT_BASE_URL="${REPLAN_OPENPROJECT_BASE_URL:-}"
OPENPROJECT_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
OPENPROJECT_HOST_HEADER="${REPLAN_OPENPROJECT_HOST_HEADER:-}"
OPENPROJECT_FORWARDED_PROTO="${REPLAN_OPENPROJECT_FORWARDED_PROTO:-}"
OPENPROJECT_SINCE="${REPLAN_OPENPROJECT_SINCE:-}"
OPENPROJECT_INCLUDE_CLOSED="${REPLAN_OPENPROJECT_INCLUDE_CLOSED:-}"
OPENPROJECT_INCLUDE_TIME_ENTRIES="${REPLAN_OPENPROJECT_INCLUDE_TIME_ENTRIES:-}"
OPENPROJECT_INCLUDE_RELATIONS="${REPLAN_OPENPROJECT_INCLUDE_RELATIONS:-}"
OPENPROJECT_DEPENDENCY_DRY_RUN="${REPLAN_OPENPROJECT_DEPENDENCY_DRY_RUN:-}"
OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
FORMAT="${REPLAN_FORMAT:-json}"
KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"

# fail CODE MESSAGE EXIT: the shared envelope in JSON, the message in text.
fail() {
  if [ "$FORMAT" = "json" ]; then
    replan_fail_json validate-openproject invalid "$1" "$2" "$3"
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

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-openproject-validation.XXXXXX")"
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

assert_contains() {
  file="$1"
  pattern="$2"
  if ! grep "$pattern" "$file" >/dev/null; then
    sed -n '1,200p' "$file" >&2
    fail validation_check_failed "missing pattern '$pattern' in $file" 1
  fi
}

run_json_ok() {
  name="$1"
  shift
  output="$TMP_DIR/$name.json"
  "$@" >"$output"
  assert_contains "$output" '"ok": true'
}

json_quote() {
  python3 -c 'import json, sys; print(json.dumps(sys.argv[1]))' "$1"
}

copy_project() {
  target="$1"
  cp -a "$PROJECT_PATH" "$target"
}

run_json_ok openproject-file-dry-run "$BIN" import openproject \
  --project "$PROJECT_PATH" \
  --profile "$OPENPROJECT_PROFILE" \
  --from-file "$OPENPROJECT_SAMPLE" \
  --dry-run \
  --format json
assert_contains "$TMP_DIR/openproject-file-dry-run.json" '"source_mode": "file"'
assert_contains "$TMP_DIR/openproject-file-dry-run.json" '"task_candidates"'

snapshot_project="$TMP_DIR/snapshot-project"
copy_project "$snapshot_project"
run_json_ok openproject-file-write "$BIN" import openproject \
  --project "$snapshot_project" \
  --profile "$OPENPROJECT_PROFILE" \
  --from-file "$OPENPROJECT_SAMPLE" \
  --format json

snapshot_dir="$(find "$snapshot_project/imports/openproject" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1)"
test -s "$snapshot_dir/manifest.yaml"
test -s "$snapshot_dir/raw.json"
test -s "$snapshot_dir/mapped.json"
snapshot_id="$(basename "$snapshot_dir")"

run_json_ok openproject-accept-dry-run "$BIN" import accept \
  --project "$snapshot_project" \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$snapshot_id"

live_status="skipped"
dependency_accept_dry_run_status="skipped"
token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
if [ -n "$token_value" ]; then
  if [ -n "$OPENPROJECT_BASE_URL" ] || [ -n "$OPENPROJECT_PROJECT_ID" ]; then
    set -- "$BIN" import openproject \
      --project "$PROJECT_PATH" \
      --base-url "$OPENPROJECT_BASE_URL" \
      --project-id "$OPENPROJECT_PROJECT_ID" \
      --token-env "$OPENPROJECT_TOKEN_ENV" \
      --dry-run \
      --format json
    if [ -n "$OPENPROJECT_HOST_HEADER" ]; then
      set -- "$@" --host-header "$OPENPROJECT_HOST_HEADER"
    fi
    if [ -n "$OPENPROJECT_FORWARDED_PROTO" ]; then
      set -- "$@" --forwarded-proto "$OPENPROJECT_FORWARDED_PROTO"
    fi
  else
    set -- "$BIN" import openproject \
      --project "$PROJECT_PATH" \
      --profile "$OPENPROJECT_PROFILE" \
      --dry-run \
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
  if [ -n "$OPENPROJECT_INCLUDE_RELATIONS" ]; then
    set -- "$@" --include-relations
  fi
  run_json_ok openproject-api-dry-run "$@"
  assert_contains "$TMP_DIR/openproject-api-dry-run.json" '"source_mode": "api"'
  live_status="passed"

  if [ -n "$OPENPROJECT_DEPENDENCY_DRY_RUN" ]; then
    if [ -z "$OPENPROJECT_INCLUDE_RELATIONS" ]; then
      fail missing_setting "REPLAN_OPENPROJECT_DEPENDENCY_DRY_RUN requires REPLAN_OPENPROJECT_INCLUDE_RELATIONS=1" 8
    fi
    dependency_project="$TMP_DIR/dependency-project"
    copy_project "$dependency_project"
    # Rebuild as positional parameters with the temporary project path.
    if [ -n "$OPENPROJECT_BASE_URL" ] || [ -n "$OPENPROJECT_PROJECT_ID" ]; then
      set -- "$BIN" import openproject \
        --project "$dependency_project" \
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
        --project "$dependency_project" \
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
    set -- "$@" --include-relations

    run_json_ok openproject-dependency-import-before-accept "$@"
    dependency_snapshot_id="$(python3 -c 'import json,sys; data=json.load(open(sys.argv[1])); print(data["result"]["snapshot"]["id"])' "$TMP_DIR/openproject-dependency-import-before-accept.json")"
    run_json_ok openproject-dependency-task-accept "$BIN" import accept \
      --project "$dependency_project" \
      --actor actor.human.operator \
      --apply \
      --format json \
      "$dependency_snapshot_id"
    run_json_ok openproject-dependency-import-after-accept "$@"
    dependency_snapshot_id="$(python3 -c 'import json,sys; data=json.load(open(sys.argv[1])); print(data["result"]["snapshot"]["id"])' "$TMP_DIR/openproject-dependency-import-after-accept.json")"
    run_json_ok openproject-dependency-accept-dry-run "$BIN" import accept \
      --project "$dependency_project" \
      --accept-dependencies \
      --dry-run \
      --format json \
      "$dependency_snapshot_id"
    dependency_accept_dry_run_status="passed"
  fi
fi

if [ "$FORMAT" = "json" ]; then
  if [ -n "$KEEP_TEMP" ]; then
    printf '{"ok":true,"command":"validate-openproject","project_path":%s,"profile":%s,"file_validation":"passed","snapshot_validation":"passed","accept_dry_run_validation":"passed","live_api_validation":%s,"dependency_accept_dry_run_validation":%s,"temp_retained":true,"outputs_dir":%s}\n' \
      "$(json_quote "$PROJECT_PATH")" \
      "$(json_quote "$OPENPROJECT_PROFILE")" \
      "$(json_quote "$live_status")" \
      "$(json_quote "$dependency_accept_dry_run_status")" \
      "$(json_quote "$TMP_DIR")"
  else
    printf '{"ok":true,"command":"validate-openproject","project_path":%s,"profile":%s,"file_validation":"passed","snapshot_validation":"passed","accept_dry_run_validation":"passed","live_api_validation":%s,"dependency_accept_dry_run_validation":%s,"temp_retained":false}\n' \
      "$(json_quote "$PROJECT_PATH")" \
      "$(json_quote "$OPENPROJECT_PROFILE")" \
      "$(json_quote "$live_status")" \
      "$(json_quote "$dependency_accept_dry_run_status")"
  fi
else
  printf 'OpenProject validation ok\n'
  printf 'file validation: passed\n'
  printf 'snapshot validation: passed\n'
  printf 'accept dry-run validation: passed\n'
  printf 'live API validation: %s\n' "$live_status"
  printf 'dependency accept dry-run validation: %s\n' "$dependency_accept_dry_run_status"
fi
