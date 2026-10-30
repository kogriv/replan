#!/usr/bin/env sh
# Apply a planned RePlan bootstrap to an OpenProject project. Dated work-package
# creates preserve instance non-working weekdays with ignoreNonWorkingDays.
#
# Settings:
#   REPLAN_PROJECT                          required
#   REPLAN_OPENPROJECT_BASE_URL             required
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID    deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_PROJECT_NAME         default REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TARGET_PROJECT_NAME  deprecated alias of REPLAN_OPENPROJECT_PROJECT_NAME
#   REPLAN_OPENPROJECT_ASSIGNEE_USER_ID      default empty
#   REPLAN_OPENPROJECT_ASSIGNEE_NAME         default empty
#   REPLAN_OPENPROJECT_ASSIGNEE_RESOURCE_ID  default empty
#   REPLAN_OPENPROJECT_BINDINGS_PATH         default private/openproject/<target>-bindings.yaml
#   REPLAN_OPENPROJECT_TOKEN_ENV            default OPENPROJECT_TOKEN
#   REPLAN_OPENPROJECT_TOKEN_FILE           default empty; used when the selected token environment value is empty
#   OPENPROJECT_TOKEN                       default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_HOST_HEADER          default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO      default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY        default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY_FILE   default empty; used when REPLAN_OPENPROJECT_PERIMETER_KEY is empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER     default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS      default 60
#   REPLAN_OPENPROJECT_RETRIES              default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS  default 2
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
#   scripts/apply-openproject-bootstrap.sh
#   REPLAN_APPLY=1 scripts/apply-openproject-bootstrap.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "apply-openproject-bootstrap" "apply-openproject-bootstrap.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "apply-openproject-bootstrap" dry_run
PROJECT_PATH="${REPLAN_PROJECT:-}"
OPENPROJECT_BASE_URL="${REPLAN_OPENPROJECT_BASE_URL:-}"
OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
TARGET_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
TARGET_PROJECT_NAME="${REPLAN_OPENPROJECT_PROJECT_NAME:-$TARGET_PROJECT_ID}"

replan_require_json_format "apply-openproject-bootstrap"
replan_require_settings "apply-openproject-bootstrap" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

if [ -z "$OPENPROJECT_BASE_URL" ]; then
  replan_fail_json "apply-openproject-bootstrap" dry_run missing_base_url "REPLAN_OPENPROJECT_BASE_URL is required" 8
fi

token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
if [ -z "$token_value" ]; then
  replan_token_missing "apply-openproject-bootstrap" "$OPENPROJECT_TOKEN_ENV"
fi

PLAN_JSON="$(mktemp "${TMPDIR:-/tmp}/replan-openproject-bootstrap-plan.XXXXXX.json")"
REPLAN_PROJECT="$PROJECT_PATH" \
REPLAN_OPENPROJECT_PROJECT_ID="$TARGET_PROJECT_ID" \
REPLAN_OPENPROJECT_PROJECT_NAME="$TARGET_PROJECT_NAME" \
"$ROOT_DIR/scripts/plan-openproject-bootstrap.sh" >"$PLAN_JSON"

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m apply_openproject_bootstrap --plan-json "$PLAN_JSON" --root-dir "$ROOT_DIR"
