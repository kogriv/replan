#!/usr/bin/env sh
# Plan or push RePlan task-description drift to bound OpenProject work packages.
#
# Settings:
#   REPLAN_ENV_FILE                         default empty; environment only
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
#   REPLAN_OPENPROJECT_BINDINGS_PATH        default private/openproject/<project-id>-bindings.yaml
#   REPLAN_TASKS                            default empty; optional comma-separated bound task ids
#   REPLAN_KEEP_TEMP                        default empty
#   REPLAN_FORMAT                           only json; any other value is refused
#   REPLAN_APPLY                            default empty
#
# Writes only with REPLAN_APPLY=1; otherwise it is a dry run. A write contains
# only description (markdown raw) and lockVersion. Empty RePlan descriptions
# are skipped and never erase OpenProject text. If the current description
# contains bootstrap's RePlan task-id trailer line, only the text above it is
# compared; the trailer line and every byte after it are preserved on update.
# The helper does not add a trailer to a description that lacks one.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/push-openproject-description-drift.sh
#   REPLAN_TASKS=task.one,task.two scripts/push-openproject-description-drift.sh
#   REPLAN_APPLY=1 scripts/push-openproject-description-drift.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "push-openproject-description-drift" "push-openproject-description-drift.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_refuse_mixed_project_settings "push-openproject-description-drift" invalid \
  REPLAN_PROJECT REPLAN_OPENPROJECT_BINDINGS_PATH
replan_resolve_openproject_settings "push-openproject-description-drift" invalid

replan_require_json_format "push-openproject-description-drift"
replan_require_settings "push-openproject-description-drift" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

OPENPROJECT_TOKEN_ENV="${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}"
token_value="$(printenv "$OPENPROJECT_TOKEN_ENV" || true)"
if [ -z "$token_value" ]; then
  replan_token_missing "push-openproject-description-drift" "$OPENPROJECT_TOKEN_ENV"
fi

if [ -z "${REPLAN_OPENPROJECT_BASE_URL:-}" ]; then
  replan_fail_json "push-openproject-description-drift" dry_run missing_base_url "REPLAN_OPENPROJECT_BASE_URL is required" 8
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  replan_python -m push_openproject_description_drift --root-dir "$ROOT_DIR"
