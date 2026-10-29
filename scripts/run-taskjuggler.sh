#!/usr/bin/env sh
# Run TaskJuggler for one RePlan scenario and verify its artifacts.
#
# Settings:
#   REPLAN_PROJECT                    default examples/small-team
#   REPLAN_TASKJUGGLER_SCENARIO       default scenario.ivan-capacity-6
#   TASKJUGGLER_BIN                   default tj3
#   REPLAN_FORMAT                     json prints structured run output; text prints a developer summary
#   REPLAN_UNREPRESENTED_ASSUMPTIONS  default error
#   REPLAN_TASKJUGGLER_OUTPUT_DIR     default temporary directory
#   TMPDIR                            default /tmp
#
# Does not write a live OpenProject; it writes only local or disposable
# artifacts.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/run-taskjuggler.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "run-taskjuggler.sh" "$@"
PROJECT_PATH="${REPLAN_PROJECT:-examples/small-team}"
SCENARIO_ID="${REPLAN_TASKJUGGLER_SCENARIO:-scenario.ivan-capacity-6}"
TASKJUGGLER_BIN="${TASKJUGGLER_BIN:-tj3}"
FORMAT="${REPLAN_FORMAT:-json}"
UNREPRESENTED_ASSUMPTIONS="${REPLAN_UNREPRESENTED_ASSUMPTIONS:-error}"
OUTPUT_DIR="${REPLAN_TASKJUGGLER_OUTPUT_DIR:-}"

if [ -z "$OUTPUT_DIR" ]; then
  OUTPUT_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-taskjuggler-run.XXXXXX")"
fi

go run ./cmd/replan taskjuggler run \
  --project "$PROJECT_PATH" \
  --scenario "$SCENARIO_ID" \
  --output-dir "$OUTPUT_DIR" \
  --taskjuggler-bin "$TASKJUGGLER_BIN" \
  --unrepresented-assumptions "$UNREPRESENTED_ASSUMPTIONS" \
  --format "$FORMAT"

test -s "$OUTPUT_DIR/project.tjp"
test -f "$OUTPUT_DIR/stdout.txt"
test -f "$OUTPUT_DIR/stderr.txt"
test -s "$OUTPUT_DIR/taskjuggler.version.txt"
test -s "$OUTPUT_DIR/taskjuggler.mapping.json"
test -s "$OUTPUT_DIR/reports/replan-task-schedule.csv"
test -s "$OUTPUT_DIR/run.manifest.json"
