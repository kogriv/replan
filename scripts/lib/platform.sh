#!/usr/bin/env sh

# Cross-platform launch helpers for the agent kit. The kit supports POSIX
# systems directly and native Windows programs through Git Bash/MSYS.

replan_is_msys() {
  case "${MSYSTEM:-}" in MINGW*|MSYS*) return 0 ;; esac
  case "$(uname -s 2>/dev/null || true)" in MINGW*|MSYS*) return 0 ;; esac
  return 1
}

replan_windows_path() {
  if replan_is_msys && command -v cygpath >/dev/null 2>&1; then
    cygpath -m -- "$1"
  else
    printf '%s\n' "$1"
  fi
}

# Spell one shell path for a native Windows process. Callers use this for
# explicit path operands instead of depending on Git Bash argument conversion,
# which may be disabled with MSYS_NO_PATHCONV or MSYS2_ARG_CONV_EXCL.
replan_py_path() {
  replan_windows_path "$1"
}

# Convert path-valued options accepted by helpers that forward their own
# command line to Python. Non-path arguments, including /api/v3/... values,
# remain byte-for-byte unchanged.
replan_python_path_args() {
  _replan_path_arg_count=$#
  while [ "$_replan_path_arg_count" -gt 0 ]; do
    _replan_path_arg=$1
    shift
    _replan_path_arg_count=$((_replan_path_arg_count - 1))
    case "$_replan_path_arg" in
      --claude|--codex|--cwd|--description-file|--diff-output|--file|--output|--project|--text-file)
        [ "$_replan_path_arg_count" -gt 0 ] || {
          set -- "$@" "$_replan_path_arg"
          continue
        }
        _replan_path_value=$1
        shift
        _replan_path_arg_count=$((_replan_path_arg_count - 1))
        set -- "$@" "$_replan_path_arg" "$(replan_py_path "$_replan_path_value")"
        ;;
      *) set -- "$@" "$_replan_path_arg" ;;
    esac
  done
  replan_python "$@"
}

replan_prepare_windows_paths() {
  replan_is_msys || return 0
  command -v cygpath >/dev/null 2>&1 || return 0
  for _replan_path_name in REPLAN_PROJECT REPLAN_OPENPROJECT_BINDINGS_PATH \
    REPLAN_OPENPROJECT_TOKEN_FILE REPLAN_OPENPROJECT_PERIMETER_KEY_FILE \
    REPLAN_OPENPROJECT_FROM_FILE REPLAN_ENV_FILE REPLAN_BIN TMPDIR \
    REPLAN_OVERLAY_OUTPUT_DIR; do
    _replan_path_value=$(printenv "$_replan_path_name" 2>/dev/null || true)
    [ -n "$_replan_path_value" ] || continue
    _replan_path_value=$(cygpath -m -- "$_replan_path_value") || continue
    export "$_replan_path_name=$_replan_path_value"
  done
}

replan_python_error() {
  _replan_python_code=$1
  _replan_python_message=$2
  printf '{"ok":false,"command":"%s","mode":"invalid","warnings":[],"errors":[{"code":"%s","message":"%s","severity":"error"}],"warning_codes":[],"error_codes":["%s"]}\n' \
    "${0##*/}" "$_replan_python_code" "$_replan_python_message" "$_replan_python_code"
  exit 8
}

_replan_python_probe() {
  "$@" -c 'import sys; raise SystemExit(0 if sys.version_info >= (3, 9) else 1)' </dev/null >/dev/null 2>&1
}

replan_resolve_python() {
  [ -z "${REPLAN_PYTHON_KIND:-}" ] || return 0
  if [ -n "${REPLAN_PYTHON:-}" ]; then
    if _replan_python_probe "$REPLAN_PYTHON"; then REPLAN_PYTHON_KIND=explicit
    else replan_python_error python_not_found 'REPLAN_PYTHON must name a working Python 3.9 or newer interpreter'; fi
  elif command -v python3 >/dev/null 2>&1 && _replan_python_probe python3; then REPLAN_PYTHON_KIND=python3
  elif command -v python >/dev/null 2>&1 && _replan_python_probe python; then REPLAN_PYTHON_KIND=python
  elif command -v py >/dev/null 2>&1 && _replan_python_probe py -3; then REPLAN_PYTHON_KIND=py3
  else replan_python_error python_not_found 'Python 3.9 or newer was not found; install it or set REPLAN_PYTHON'; fi
  export REPLAN_PYTHON_KIND PYTHONUTF8=1 PYTHONIOENCODING=utf-8
  case "$REPLAN_PYTHON_KIND" in
    explicit) set -- "$REPLAN_PYTHON" ;; python3) set -- python3 ;;
    python) set -- python ;; py3) set -- py -3 ;;
  esac
  "$@" -c 'import yaml' </dev/null >/dev/null 2>&1 || \
    replan_python_error pyyaml_missing 'PyYAML is required; run python -m pip install pyyaml (or use REPLAN_PYTHON -m pip install pyyaml)'
}

replan_python() {
  replan_resolve_python
  replan_prepare_windows_paths
  if replan_is_msys && command -v cygpath >/dev/null 2>&1 && [ -n "${PYTHONPATH:-}" ]; then
    PYTHONPATH=$(cygpath -mp -- "$PYTHONPATH")
    export PYTHONPATH
  fi
  case "$REPLAN_PYTHON_KIND" in
    explicit) "$REPLAN_PYTHON" "$@" ;; python3) python3 "$@" ;;
    python) python "$@" ;; py3) py -3 "$@" ;;
  esac
}
