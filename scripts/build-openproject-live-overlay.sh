#!/usr/bin/env sh
# Build the RePlan overlay artifact with real work-package ids from the ignored
# private binding file. Local build only: it does not contact OpenProject and
# does not publish; scripts/publish-openproject-overlay.sh publishes it.
#
# Settings:
#   REPLAN_PROJECT                    required
#   REPLAN_OPENPROJECT_PROJECT_ID     required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID  deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_BINDINGS_PATH  default private/openproject/<project>-bindings.yaml
#   REPLAN_OVERLAY_OUTPUT_DIR         default tmp/openproject-live-overlay/latest
#   REPLAN_BIN                        default bin/replan in the kit, otherwise build locally
#   REPLAN_FORMAT                     only json; any other value is refused
#
# Does not write a live OpenProject; it writes only local or disposable
# artifacts.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/build-openproject-live-overlay.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "build-openproject-live-overlay" "build-openproject-live-overlay.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "build-openproject-live-overlay" invalid
replan_require_json_format "build-openproject-live-overlay"
OUTPUT_DIR="${REPLAN_OVERLAY_OUTPUT_DIR:-$ROOT_DIR/tmp/openproject-live-overlay/latest}"
replan_require_settings "build-openproject-live-overlay" REPLAN_PROJECT REPLAN_OPENPROJECT_PROJECT_ID

BIN="${REPLAN_BIN:-}"
if [ -z "$BIN" ]; then
  BIN_DIR="$(mktemp -d)"
  trap 'rm -rf "$BIN_DIR"' EXIT
  BIN="$BIN_DIR/replan"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./cmd/replan)
fi
rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"
PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  python3 -m openproject_live_overlay --replan-bin "$BIN" --output-dir "$OUTPUT_DIR" --root-dir "$ROOT_DIR"
