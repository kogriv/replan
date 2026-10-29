#!/usr/bin/env sh
# Plan the OpenProject work packages needed to bootstrap a RePlan project.
#
# Settings:
#   REPLAN_PROJECT                          required
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID    deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_PROJECT_NAME         default REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TARGET_PROJECT_NAME  deprecated alias of REPLAN_OPENPROJECT_PROJECT_NAME
#   REPLAN_FORMAT                           only json; any other value is refused
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/plan-openproject-bootstrap.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "plan-openproject-bootstrap" "plan-openproject-bootstrap.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "plan-openproject-bootstrap" dry_run

replan_require_json_format "plan-openproject-bootstrap"
replan_require_settings "plan-openproject-bootstrap" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m plan_openproject_bootstrap
