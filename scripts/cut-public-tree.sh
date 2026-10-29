#!/usr/bin/env sh
# Prepare a one-commit public repository from the tracked files at HEAD.
#
# Settings:
#   REPLAN_PUBLIC_CUT_DIR           required
#   REPLAN_PUBLIC_CUT_AUTHOR_NAME   required
#   REPLAN_PUBLIC_CUT_AUTHOR_EMAIL  required
#   REPLAN_PUBLIC_CUT_MESSAGE       default Initial public release
#   REPLAN_PUBLIC_CUT_DATE          default empty
#   REPLAN_PRIVATE_LEAK_PATTERNS    default private/leak-patterns.txt
#   REPLAN_APPLY                    default empty
#   TMPDIR                          default /tmp
#
# Dry run by default: checks a temporary copy and removes it. REPLAN_APPLY=1
# creates the cut directory, initializes main and makes exactly one commit.
# Prints the JSON helper envelope and accepts no arguments.
# Uses helper-envelope exits: 0 success, 8 usage/settings, 10 internal
# failure, and 11 attention needed.
#
#   scripts/cut-public-tree.sh
#   REPLAN_APPLY=1 scripts/cut-public-tree.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_json "cut-public-tree" "cut-public-tree.sh" "$@"
replan_require_settings "cut-public-tree" REPLAN_PUBLIC_CUT_DIR REPLAN_PUBLIC_CUT_AUTHOR_NAME REPLAN_PUBLIC_CUT_AUTHOR_EMAIL

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  python3 -m cut_public_tree --root-dir "$ROOT_DIR"
