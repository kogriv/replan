#!/usr/bin/env sh
# Plan resource-demand calibration actions from current RePlan state.
#
# Settings:
#   REPLAN_PROJECT  required
#   REPLAN_FORMAT   only json; any other value is refused
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/plan-resource-demand-calibration.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "plan-resource-demand-calibration" "plan-resource-demand-calibration.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"

replan_require_json_format "plan-resource-demand-calibration"
replan_require_settings "plan-resource-demand-calibration" REPLAN_PROJECT

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m plan_resource_demand_calibration
