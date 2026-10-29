#!/usr/bin/env sh
# Check resource demands and scheduled workload for planning drift.
#
# Settings:
#   REPLAN_PROJECT  required
#   REPLAN_BIN      default build locally
#   REPLAN_FORMAT   only json; any other value is refused
#   TMPDIR          default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/check-resource-schedule-drift.sh
#   REPLAN_BIN=./replan scripts/check-resource-schedule-drift.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "check-resource-schedule-drift" "check-resource-schedule-drift.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"

replan_require_json_format "check-resource-schedule-drift"
replan_require_settings "check-resource-schedule-drift" REPLAN_PROJECT

# The weekly rows come from `replan workload`.
BIN="${REPLAN_BIN:-}"
if [ -z "$BIN" ]; then
  TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-schedule-drift.XXXXXX")"
  trap 'rm -rf "$TMP_DIR"' EXIT INT TERM
  BIN="$TMP_DIR/replan"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./cmd/replan)
fi

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m check_resource_schedule_drift --replan-bin "$BIN"
