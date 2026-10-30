#!/usr/bin/env sh
# Accept one review-ready task through evidence, actuals, its actual window,
# proposals, OpenProject dates and status, and the published overlay.
#
# Settings:
#   REPLAN_PROJECT                          required
#   REPLAN_TASK                             required
#   REPLAN_ACCEPTED_BY                      required
#   REPLAN_ACTOR                            default empty; required under apply
#   REPLAN_ACCEPT_SUMMARY                   required
#   REPLAN_ACCEPT_EVIDENCE_ID               default evidence.<namespace>.accept-<task-slug>
#   REPLAN_ACCEPT_AT                        default now as RFC3339 with offset
#   REPLAN_ACCEPT_CI_MINUTES                default 0
#   REPLAN_BIN                              default build locally
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID    deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_BASE_URL             default empty
#   REPLAN_OPENPROJECT_TOKEN_ENV            default OPENPROJECT_TOKEN
#   REPLAN_OPENPROJECT_TOKEN_FILE           default empty; used when the selected token environment value is empty
#   OPENPROJECT_TOKEN                       default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_BINDINGS_PATH        default private/openproject/<project>-bindings.yaml
#   REPLAN_OPENPROJECT_HOST_HEADER          default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO      default empty
#   REPLAN_OPENPROJECT_FROM_FILE            default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY        default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY_FILE   default empty; used when REPLAN_OPENPROJECT_PERIMETER_KEY is empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER     default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS      default 60
#   REPLAN_OPENPROJECT_RETRIES              default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS  default 2
#   REPLAN_OVERLAY_OUTPUT_DIR               default tmp/openproject-live-overlay/latest
#   REPLAN_OVERLAY_PUBLISH_TARGET           default empty
#   REPLAN_KEEP_TEMP                        default empty
#   REPLAN_FORMAT                           only json; any other value is refused
#   REPLAN_APPLY                            default empty
#   TMPDIR                                  default /tmp
#
# Writes only with REPLAN_APPLY=1; otherwise it is a dry run. REPLAN_ACTOR is
# required before any apply write. REPLAN_TASK, REPLAN_ACCEPTED_BY, REPLAN_ACTOR
# and the REPLAN_ACCEPT_* settings describe one acceptance, so they are read
# from the process environment only, never from .env.local. The helper
# prints the shared JSON helper envelope and accepts no arguments.
# Steps 1-5 keep their original evidence/actual/status order; step 6 records
# the actual window, 7 updates OpenProject dates, 8 suggests dependencies, 9
# updates OpenProject status, and 10 refreshes the overlay.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/accept-task.sh
#   REPLAN_ACTOR=actor.human.operator REPLAN_APPLY=1 scripts/accept-task.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "accept-task" "accept-task.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "accept-task" invalid
replan_require_json_format "accept-task"
for _accept_setting in REPLAN_TASK REPLAN_ACCEPTED_BY REPLAN_ACCEPT_SUMMARY; do
  if ! printenv "$_accept_setting" >/dev/null 2>&1 || [ -z "$(printenv "$_accept_setting")" ]; then
    replan_fail_json "accept-task" invalid missing_setting "required setting not set: $_accept_setting; it describes one acceptance, so set it in the process environment (.env.local is not read for it)" 8
  fi
done
replan_require_settings "accept-task" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"
TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-accept-task.XXXXXX")"
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

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  python3 -m accept_task --root-dir "$ROOT_DIR" --tmp-dir "$TMP_DIR" --replan-bin "$BIN"
