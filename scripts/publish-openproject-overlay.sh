#!/usr/bin/env sh
# Publish a built RePlan overlay into its own project's directory of the
# plugin's overlay directory (REPLAN_OPENPROJECT_OVERLAY_DIR on the server),
# so several projects of one OpenProject each show their own plan.
#
#   scripts/build-openproject-live-overlay.sh       # build
#   scripts/publish-openproject-overlay.sh          # dry run: what would be sent
#   REPLAN_APPLY=1 scripts/publish-openproject-overlay.sh
#
# REPLAN_OVERLAY_PUBLISH_TARGET is the overlay directory as rsync addresses
# it, a local path or host:path; it is private, so it comes from the
# environment or the ignored .env.local, never from this repository. The
# overlay goes to <target>/<project identifier>/ and nowhere else: another
# project's directory is never touched. An overlay that names another
# project, none, or a scenario is refused; a what-if is never published as
# the plan.
# External dependency: rsync.
# Settings:
#   REPLAN_OPENPROJECT_PROJECT_ID  required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID  deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OVERLAY_OUTPUT_DIR      default tmp/openproject-live-overlay/latest
#   REPLAN_OVERLAY_PUBLISH_TARGET  required
#   REPLAN_APPLY                   default empty
#
# Writes only with REPLAN_APPLY=1; otherwise it is a dry run.
# Prints the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "publish-openproject-overlay" "publish-openproject-overlay.sh" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "publish-openproject-overlay" invalid
OPENPROJECT_PROJECT_ID="${REPLAN_OPENPROJECT_PROJECT_ID:-}"
SOURCE_DIR="${REPLAN_OVERLAY_OUTPUT_DIR:-$ROOT_DIR/tmp/openproject-live-overlay/latest}"
TARGET="${REPLAN_OVERLAY_PUBLISH_TARGET:-}"
APPLY="${REPLAN_APPLY:-}"
OVERLAY="$SOURCE_DIR/raw/openproject_overlay.json"
replan_require_settings "publish-openproject-overlay" REPLAN_OPENPROJECT_PROJECT_ID

mode=dry_run
[ "$APPLY" = "1" ] && mode=apply

# fail CODE MESSAGE EXIT
fail() {
  replan_fail_json publish-openproject-overlay "$mode" "$1" "$2" "$3"
}

case "$OPENPROJECT_PROJECT_ID" in
  ""|*[!A-Za-z0-9_-]*) fail invalid_project_id "REPLAN_OPENPROJECT_PROJECT_ID must be letters, digits, dashes and underscores" 8 ;;
esac
[ -f "$OVERLAY" ] || fail overlay_not_built "no built overlay; run scripts/build-openproject-live-overlay.sh first" 4
[ -n "$TARGET" ] || fail missing_publish_target "REPLAN_OVERLAY_PUBLISH_TARGET is required" 8

owner="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("openproject_project_id") or "")' "$OVERLAY")"
if [ "$owner" != "$OPENPROJECT_PROJECT_ID" ]; then
  fail overlay_other_project "the built overlay names another OpenProject project, or none; set project_id in the plan's OpenProject source profile" 2
fi
scenario_id="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1])).get("scenario_id") or "")' "$OVERLAY")"
if [ -n "$scenario_id" ]; then
  fail overlay_is_scenario "a what-if overlay is never published as the plan" 2
fi
command -v rsync >/dev/null 2>&1 || fail rsync_not_found "rsync is required to publish the overlay" 4
rows="$(python3 -c 'import json, sys; print(len(json.load(open(sys.argv[1]))["work_packages"]))' "$OVERLAY")"

if [ "$mode" = "apply" ]; then
  rsync -a --delete --mkpath --chmod=D755,F644 "$SOURCE_DIR/" "${TARGET%/}/$OPENPROJECT_PROJECT_ID/" \
    || fail overlay_publish_failed "rsync to the overlay directory failed" 6
fi
replan_emit_json "$mode" 0 "$(printf '{"ok":true,"command":"publish-openproject-overlay","mode":"%s","project_id":"%s","work_packages":%s,"destination":"<target>/%s/"}' \
  "$mode" "$OPENPROJECT_PROJECT_ID" "$rows" "$OPENPROJECT_PROJECT_ID")"
