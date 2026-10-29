#!/usr/bin/env sh
# Review the project's resource pool, capacities, demand and policy decisions.
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
#   scripts/review-resource-pool.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "review-resource-pool" "review-resource-pool.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"

replan_require_json_format "review-resource-pool"
replan_require_settings "review-resource-pool" REPLAN_PROJECT

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m review_resource_pool
