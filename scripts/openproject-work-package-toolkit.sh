#!/usr/bin/env sh
# Inspect and safely change OpenProject work packages, catalogs, relations and comments.
# Dated creates and updates preserve instance non-working weekdays.
#
# Settings:
#   REPLAN_PROJECT                          required
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID    deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_BASE_URL             default empty
#   REPLAN_OPENPROJECT_TOKEN_ENV            default OPENPROJECT_TOKEN
#   OPENPROJECT_TOKEN                       default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_BINDINGS_PATH        default private/openproject/<project>-bindings.yaml
#   REPLAN_OPENPROJECT_HOST_HEADER          default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO      default empty
#   REPLAN_OPENPROJECT_FROM_FILE            default empty
#   REPLAN_OPENPROJECT_STATUS_SCOPE         default ready_only
#   REPLAN_OPENPROJECT_PUBLIC_URL           default empty
#   REPLAN_OPENPROJECT_REPLAN_LINK_FIELD    default RePlan
#   REPLAN_OPENPROJECT_PERIMETER_KEY        default empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER     default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS      default 60
#   REPLAN_OPENPROJECT_RETRIES              default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS  default 2
#   REPLAN_BIN                              default bin/replan in the kit, otherwise build locally
#   REPLAN_FORMAT                           only json; any other value is refused
#   REPLAN_APPLY                            default empty
#
# Writes only with REPLAN_APPLY=1; otherwise it is a dry run.
# Prints argparse help as text or the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/openproject-work-package-toolkit.sh --help
#   scripts/openproject-work-package-toolkit.sh work-package list
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
case "${1:-}" in
  -h|--help)
    (replan_help "$0" "$@")
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" exec python3 -m openproject_work_package_toolkit --root-dir "$ROOT_DIR" "$@"
    ;;
esac
case " $* " in
  *" -h "*|*" --help "*)
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" exec python3 -m openproject_work_package_toolkit --root-dir "$ROOT_DIR" "$@"
    ;;
esac
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "openproject work-package toolkit" invalid
replan_require_json_format "openproject work-package toolkit"

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_work_package_toolkit --root-dir "$ROOT_DIR" "$@"
