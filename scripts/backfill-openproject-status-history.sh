#!/usr/bin/env sh
# Backfill bound tasks' OpenProject status journals into RePlan status history.
#
# Settings:
#   REPLAN_ENV_FILE                         default empty; environment only
#   REPLAN_PROJECT                          required
#   REPLAN_OPENPROJECT_BASE_URL             required
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TOKEN_ENV            default OPENPROJECT_TOKEN
#   REPLAN_OPENPROJECT_TOKEN_FILE           default empty
#   OPENPROJECT_TOKEN                       default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_PERIMETER_KEY        default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY_FILE   default empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER     default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS      default 60
#   REPLAN_OPENPROJECT_RETRIES              default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS  default 2
#   REPLAN_OPENPROJECT_HOST_HEADER          default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO      default empty
#   REPLAN_OPENPROJECT_BINDINGS_PATH        default private/openproject/<project-id>-bindings.yaml
#   REPLAN_BIN                              default bin/replan in a kit, otherwise build locally
#   REPLAN_ACTOR                            default empty; required with REPLAN_APPLY=1
#   REPLAN_FORMAT                           only json; any other value is refused
#   REPLAN_APPLY                            default empty
#   TMPDIR                                  default /tmp
#
# Repeat --task <id> to select bound tasks; with no selectors all bound tasks
# are read. The helper never writes OpenProject. It writes RePlan evidence and
# status audit records only with REPLAN_APPLY=1. Actual windows are previewed,
# never applied.
#
#   scripts/backfill-openproject-status-history.sh
#   scripts/backfill-openproject-status-history.sh --task task.one --task task.two
#   REPLAN_ACTOR=actor.human.operator REPLAN_APPLY=1 scripts/backfill-openproject-status-history.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_refuse_mixed_project_settings "backfill-openproject-status-history" invalid \
  REPLAN_PROJECT REPLAN_OPENPROJECT_BINDINGS_PATH
replan_resolve_openproject_settings "backfill-openproject-status-history" invalid
replan_require_json_format "backfill-openproject-status-history"
replan_require_settings "backfill-openproject-status-history" \
  REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID REPLAN_OPENPROJECT_BASE_URL

OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
if [ -z "$token_value" ]; then
  replan_token_missing "backfill-openproject-status-history" "$OPENPROJECT_TOKEN_ENV"
fi

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/rp-sh.XXXXXX")"
cleanup() {
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
    replan_python -m platform_compat remove-tree "$TMP_DIR" >/dev/null 2>&1 || rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM
BIN="${REPLAN_BIN:-}"
if [ -z "$BIN" ]; then
  BIN="$TMP_DIR/replan"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./cmd/replan)
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  replan_python -m backfill_openproject_status_history \
    --root-dir "$ROOT_DIR" --replan-bin "$BIN" --tmp-dir "$TMP_DIR" "$@"
