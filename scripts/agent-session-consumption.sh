#!/usr/bin/env sh
# Measure Claude Code and Codex agent time from local JSONL session logs.
#
# This helper is read-only. It never reads default session directories: every
# transcript or rollout path must be supplied explicitly. It prints the shared
# JSON helper envelope.
#
# Usage:
#   scripts/agent-session-consumption.sh --from RFC3339 --to RFC3339 \
#     --claude PATH [--claude PATH ...] [--codex PATH ...]
#   scripts/agent-session-consumption.sh --project PATH --task TASK_ID \
#     [--to RFC3339] --claude PATH [--codex PATH ...]
#
# Settings:
#   REPLAN_ENV_FILE  default empty; environment only
#   REPLAN_BIN       default kit binary, tmp/replan, or PATH
#   REPLAN_FORMAT    only json; any other value is refused
#
# Options:
#   --claude PATH                 Claude JSONL file or directory (repeatable)
#   --codex PATH                  Codex JSONL file or recursive directory (repeatable)
#   --claude-resource ID          default auto (task demand; explicit-window fallback below)
#   --codex-resource ID           default auto (task demand; explicit-window fallback below)
#                                explicit windows use the legacy default resource ids
#   --from RFC3339 --to RFC3339   explicit measurement window
#   --project PATH --task ID      derive the window from task status records
#   --to RFC3339 with --task      override the task-derived end
#   --idle-gap-minutes N          default 5
#   --max-generation-minutes N    default 15
#   --count-tool-waits            count long Claude tool waits as active
#   --cwd PREFIX                  keep matching cwd prefixes (repeatable)
#
# Exit 0 on success, 8 for usage or a missing window, 11 when a task window
# cannot be derived, and the shared helper-envelope failure codes otherwise.
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
. "$ROOT_DIR/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$ROOT_DIR"
replan_require_json_format "agent-session-consumption"

PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
  replan_python -m agent_session_consumption "$@"
