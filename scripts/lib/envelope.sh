#!/usr/bin/env sh

# The shared JSON envelope for shell wrappers (scripts/lib/helper_envelope.py).
# Source it after ROOT_DIR is set.

# replan_help SCRIPT [ARG...] prints SCRIPT's leading comment block for a
# first -h or --help argument. It must run before settings, builds, temporary
# directories, containers or network access.
replan_help() {
  _replan_help_script=$1
  shift
  [ "$#" -gt 0 ] || return 0
  case "$1" in
    -h|--help)
      awk '
        NR == 1 { next }
        !started && /^#/ { started = 1 }
        !started { next }
        started && /^#/ {
          line = $0
          sub(/^# ?/, "", line)
          print line
          next
        }
        started { exit }
      ' "$_replan_help_script"
      exit 0
      ;;
  esac
}

# These checks define the argument boundary for environment-driven wrappers.
replan_require_no_args_json() {
  _replan_no_args_command=$1
  _replan_no_args_script_name=$2
  shift 2
  [ "$#" -eq 0 ] && return 0
  replan_fail_json "$_replan_no_args_command" invalid usage_error "$_replan_no_args_script_name takes no arguments; see --help" 8
}

replan_require_no_args_text() {
  _replan_no_args_script_name=$1
  shift
  [ "$#" -eq 0 ] && return 0
  printf '%s takes no arguments; see --help\n' "$_replan_no_args_script_name" >&2
  exit 8
}

# replan_fail_json COMMAND MODE CODE MESSAGE EXIT [KEY=JSON_OR_TEXT ...]
# prints a failure envelope on stdout and exits with EXIT, a code from the
# table in docs/spec/cli.md: 8 for usage and settings, 6 for an external
# source, 1 for anything else.
replan_fail_json() {
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m helper_envelope fail "$@"
  exit "$5"
}

# replan_emit_json MODE EXIT JSON prints the JSON object in the shared
# envelope and exits with EXIT. Call it directly, not at the end of a pipe:
# a function in a pipeline runs in a subshell, and its exit would not end the
# script.
replan_emit_json() {
  printf '%s\n' "$3" | PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m helper_envelope normalize "$1" "$2" || true
  exit "$2"
}

# replan_require_json_format COMMAND rejects any REPLAN_FORMAT but json.
replan_require_json_format() {
  if [ "${REPLAN_FORMAT:-json}" != "json" ]; then
    replan_fail_json "$1" invalid unsupported_format "Only REPLAN_FORMAT=json is supported by $1" 8
  fi
}

# Resolve renamed OpenProject project settings through Settings.from_env.
# The Python settings object is the single compatibility and conflict policy;
# wrappers only export its resolved values and envelope warnings.
replan_resolve_openproject_settings() {
  _replan_settings_command=$1
  _replan_settings_mode=${2:-invalid}
  _replan_settings_output=$(
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" \
      python3 -m helper_envelope resolve-openproject-settings
  ) || {
    case "$_replan_settings_output" in
      missing_setting:*)
        _replan_missing_setting=${_replan_settings_output#missing_setting:}
        replan_fail_json "$_replan_settings_command" "$_replan_settings_mode" \
          missing_setting "required setting could not be read: $_replan_missing_setting" 8 \
          "missing_settings=[\"$_replan_missing_setting\"]"
        ;;
    esac
    replan_fail_json "$_replan_settings_command" "$_replan_settings_mode" conflicting_settings "$_replan_settings_output" 8
  }
  eval "$_replan_settings_output"
  replan_resolve_openproject_secret_files \
    "$_replan_settings_command" "$_replan_settings_mode"
}

# replan_token_missing COMMAND TOKEN_ENV [KEY=VALUE ...] answers a helper run
# without its OpenProject token (finding C-01). Under REPLAN_APPLY=1 nothing
# was written, so it is a failure: exit 8. Otherwise the helper is skipped,
# ok, with token_env_not_set among its warnings. KEY=VALUE pairs, such as the
# helper's own status key, are added to the envelope.
replan_token_missing() {
  command=$1
  token_env=$2
  shift 2
  if [ "${REPLAN_APPLY:-}" = "1" ]; then
    replan_fail_json "$command" apply token_env_not_set "$token_env is not set; nothing was applied" 8 "token_env=$token_env" "$@"
  fi
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m helper_envelope skip "$command" "$token_env" "$@"
  exit 0
}

# replan_token_skip COMMAND TOKEN_ENV [KEY=VALUE ...] is the same for a helper
# that only reads: skipped, ok, token_env_not_set among its warnings.
replan_token_skip() {
  command=$1
  token_env=$2
  shift 2
  PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m helper_envelope skip "$command" "$token_env" "$@"
  exit 0
}

# replan_require_settings COMMAND NAME... fails with missing_setting, exit 8,
# naming every listed setting that is empty. The helpers carry no default
# project: a kit user who left one out is told so instead of working on
# another project (finding C-05).
replan_require_settings() {
  command=$1
  shift
  missing=""
  for name in "$@"; do
    eval "value=\${$name:-}"
    [ -n "$value" ] || missing="$missing $name"
  done
  [ -z "$missing" ] && return 0
  list=""
  for name in $missing; do
    list="$list\"$name\","
  done
  replan_fail_json "$command" invalid missing_setting "required setting not set:$missing; set it in .env.local or the environment" 8 "missing_settings=[${list%,}]"
}
