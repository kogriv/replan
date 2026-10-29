#!/usr/bin/env sh
# Read and write the wiki of an OpenProject project through API v3, as the
# token's own user: list, show, create, update and delete pages, and list,
# download, upload and delete their attachments. It needs the OpenProject
# wiki API plugin on the server (integrations/openproject/plugins/openproject-wiki_api).
#
# A command that would change the wiki only reports what it would do; it
# changes something only with REPLAN_APPLY=1. See `--help` for the commands.
# Settings:
#   REPLAN_OPENPROJECT_BASE_URL             required
#   REPLAN_OPENPROJECT_PROJECT_ID           required
#   REPLAN_OPENPROJECT_TARGET_PROJECT_ID    deprecated alias of REPLAN_OPENPROJECT_PROJECT_ID
#   REPLAN_OPENPROJECT_TOKEN_ENV            default OPENPROJECT_TOKEN
#   OPENPROJECT_TOKEN                       default empty; selected by REPLAN_OPENPROJECT_TOKEN_ENV
#   REPLAN_OPENPROJECT_HOST_HEADER          default empty
#   REPLAN_OPENPROJECT_FORWARDED_PROTO      default empty
#   REPLAN_OPENPROJECT_PERIMETER_KEY        default empty
#   REPLAN_OPENPROJECT_PERIMETER_HEADER     default X-Perimeter-Key
#   REPLAN_OPENPROJECT_TIMEOUT_SECONDS      default 60
#   REPLAN_OPENPROJECT_RETRIES              default 0
#   REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS  default 2
#   REPLAN_FORMAT                           only json; any other value is refused
#   REPLAN_APPLY                            default empty
#
# Writes only with REPLAN_APPLY=1; otherwise it is a dry run.
# Prints argparse help as text or the JSON helper envelope.
# Uses helper-envelope exits: 0 success, 1 general failure, 2 invalid plan, 4
# not found, 6 external source, 7 partial apply, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/openproject-wiki.sh --help
#   scripts/openproject-wiki.sh page list
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
case "${1:-}" in
  -h|--help)
    (replan_help "$0" "$@")
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" exec python3 -m openproject_wiki "$@"
    ;;
esac
case " $* " in
  *" -h "*|*" --help "*)
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" exec python3 -m openproject_wiki "$@"
    ;;
esac
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_resolve_openproject_settings "openproject wiki" invalid
replan_require_json_format "openproject wiki"

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m openproject_wiki "$@"
