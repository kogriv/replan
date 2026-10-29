#!/usr/bin/env sh
# Validate TaskJuggler availability and one RePlan scenario conversion.
#
# Settings:
#   REPLAN_PROJECT                    default examples/small-team
#   REPLAN_TASKJUGGLER_SCENARIO       default scenario.ivan-capacity-6
#   TASKJUGGLER_BIN                   default tj3
#   REPLAN_FORMAT                     json prints structured validation; text prints a developer summary
#   REPLAN_UNREPRESENTED_ASSUMPTIONS  default error
#   REPLAN_KEEP_TEMP                  default empty
#
# Does not write a live OpenProject; it writes only local or disposable
# artifacts.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/validate-taskjuggler.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "validate-taskjuggler.sh" "$@"
PROJECT_PATH="${REPLAN_PROJECT:-examples/small-team}"
SCENARIO_ID="${REPLAN_TASKJUGGLER_SCENARIO:-scenario.ivan-capacity-6}"
TASKJUGGLER_BIN="${TASKJUGGLER_BIN:-tj3}"
FORMAT="${REPLAN_FORMAT:-json}"
UNREPRESENTED_ASSUMPTIONS="${REPLAN_UNREPRESENTED_ASSUMPTIONS:-error}"
KEEP_TEMP="${REPLAN_KEEP_TEMP:-}"

if [ -n "$KEEP_TEMP" ]; then
  go run ./cmd/replan dev validate-taskjuggler \
    --project "$PROJECT_PATH" \
    --scenario "$SCENARIO_ID" \
    --taskjuggler-bin "$TASKJUGGLER_BIN" \
    --unrepresented-assumptions "$UNREPRESENTED_ASSUMPTIONS" \
    --format "$FORMAT" \
    --keep-temp
else
  go run ./cmd/replan dev validate-taskjuggler \
    --project "$PROJECT_PATH" \
    --scenario "$SCENARIO_ID" \
    --taskjuggler-bin "$TASKJUGGLER_BIN" \
    --unrepresented-assumptions "$UNREPRESENTED_ASSUMPTIONS" \
    --format "$FORMAT"
fi
