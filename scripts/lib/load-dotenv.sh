#!/usr/bin/env sh

# Load local OpenProject connection settings without executing .env as shell.
# Precedence: already exported environment > REPLAN_ENV_FILE, or
# already exported environment > .env.local > .env.
replan_resolve_binary() {
  replan_binary_root=$1
  replan_binary_configured=${2:-${REPLAN_BIN:-}}
  if [ -n "$replan_binary_configured" ]; then
    if [ ! -f "$replan_binary_configured" ] && [ -f "$replan_binary_configured.exe" ]; then
      replan_binary_configured=$replan_binary_configured.exe
    fi
    printf '%s\n' "$replan_binary_configured"
  elif [ -x "$replan_binary_root/bin/replan" ]; then
    printf '%s\n' "$replan_binary_root/bin/replan"
  elif [ -f "$replan_binary_root/bin/replan.exe" ]; then
    printf '%s\n' "$replan_binary_root/bin/replan.exe"
  fi
}

replan_load_openproject_dotenv() {
  root_dir=$1
  replan_settings_command=${2:-${0##*/}}
  replan_settings_command=${replan_settings_command%.sh}
  replan_settings_mode=${3:-invalid}
  if [ "${REPLAN_ENV_FILE+x}" = x ] && \
     { [ ! -f "$REPLAN_ENV_FILE" ] || [ ! -r "$REPLAN_ENV_FILE" ]; }; then
    replan_fail_json "$replan_settings_command" "$replan_settings_mode" missing_setting \
      "required setting could not be read: REPLAN_ENV_FILE" 8 \
      'missing_settings=["REPLAN_ENV_FILE"]'
  fi
  replan_settings_list_path="$root_dir/scripts/lib/settings.list"
  [ -f "$replan_settings_list_path" ] || return 0
  replan_settings_names=$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$replan_settings_list_path")
  replan_environment_only_list_path="$root_dir/scripts/lib/environment-only-settings.list"
  replan_environment_only_names=""
  if [ -f "$replan_environment_only_list_path" ]; then
    replan_environment_only_names=$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$replan_environment_only_list_path")
  fi
  replan_settings_file_project_id=""
  replan_settings_loaded_names=":"
  replan_environment_only_file_settings=""
  replan_settings_env_project_id=""
  if [ -n "${REPLAN_OPENPROJECT_PROJECT_ID:-}" ]; then
    replan_settings_env_project_id=$REPLAN_OPENPROJECT_PROJECT_ID
  fi

  if [ "${REPLAN_ENV_FILE+x}" = x ]; then
    replan_load_openproject_dotenv_file "$REPLAN_ENV_FILE"
  else
    replan_load_openproject_dotenv_file "$root_dir/.env.local"
    replan_load_openproject_dotenv_file "$root_dir/.env"
  fi

  REPLAN_INTERNAL_ENV_PROJECT_ID=$replan_settings_env_project_id
  REPLAN_INTERNAL_FILE_PROJECT_ID=$replan_settings_file_project_id
  REPLAN_INTERNAL_FILE_SETTINGS=$replan_settings_loaded_names
  REPLAN_INTERNAL_ENVIRONMENT_ONLY_FILE_SETTINGS=$replan_environment_only_file_settings
  export REPLAN_INTERNAL_ENV_PROJECT_ID REPLAN_INTERNAL_FILE_PROJECT_ID
  export REPLAN_INTERNAL_FILE_SETTINGS REPLAN_INTERNAL_ENVIRONMENT_ONLY_FILE_SETTINGS
  # A release unpacked without the Go sources carries its own binary; the
  # helpers use it instead of building one.
  if [ -z "${REPLAN_BIN:-}" ]; then
    REPLAN_BIN=$(replan_resolve_binary "$root_dir")
    [ -z "$REPLAN_BIN" ] || export REPLAN_BIN
  elif [ ! -f "$REPLAN_BIN" ] && [ -f "$REPLAN_BIN.exe" ]; then
    REPLAN_BIN=$REPLAN_BIN.exe
    export REPLAN_BIN
  fi
}

replan_load_openproject_dotenv_file() {
  dotenv_path=$1
  [ -f "$dotenv_path" ] || return 0

  while IFS= read -r line || [ -n "$line" ]; do
    line=${line%"$(printf '\r')"}
    case "$line" in
      ""|\#*) continue ;;
      export\ *) line=${line#export } ;;
    esac

    case "$line" in
      *=*) ;;
      *) continue ;;
    esac

    key=${line%%=*}
    value=${line#*=}

    case "$key" in
      *[!ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_]*|"") continue ;;
    esac

    case "$value" in
      \"*\")
        value=${value#\"}
        value=${value%\"}
        ;;
      \'*\')
        value=${value#\'}
        value=${value%\'}
        ;;
    esac

    [ -n "$value" ] || continue

    case "
$replan_environment_only_names
" in
      *"
$key
"*)
        replan_environment_only_file_settings="${replan_environment_only_file_settings}${dotenv_path##*/}	$key
"
        continue
        ;;
    esac

    case "
$replan_settings_names
" in
      *"
$key
"*) ;;
      *) continue ;;
    esac

    if [ "$key" = REPLAN_OPENPROJECT_PROJECT_ID ] && \
       [ -z "$replan_settings_file_project_id" ]; then
      replan_settings_file_project_id=$value
    fi

    if printenv "$key" >/dev/null 2>&1; then
      continue
    fi

    export "$key=$value"
    case "$replan_settings_loaded_names" in
      *":$key:"*) ;;
      *) replan_settings_loaded_names="$replan_settings_loaded_names$key:" ;;
    esac
  done < "$dotenv_path"
}

# Refuse a plan-bound helper when an environment project id conflicts with the
# settings file that supplied one or more per-project values used by it.
replan_refuse_mixed_project_settings() {
  replan_mixed_command=$1
  replan_mixed_mode=$2
  shift 2

  [ -n "${REPLAN_INTERNAL_ENV_PROJECT_ID:-}" ] || return 0
  [ -n "${REPLAN_INTERNAL_FILE_PROJECT_ID:-}" ] || return 0
  [ "$REPLAN_INTERNAL_ENV_PROJECT_ID" != "$REPLAN_INTERNAL_FILE_PROJECT_ID" ] || return 0

  replan_mixed_names=""
  replan_mixed_json=""
  for replan_mixed_name in "$@"; do
    case "${REPLAN_INTERNAL_FILE_SETTINGS:-:}" in
      *":$replan_mixed_name:"*)
        replan_mixed_names="$replan_mixed_names $replan_mixed_name"
        replan_mixed_json="$replan_mixed_json\"$replan_mixed_name\","
        ;;
    esac
  done
  [ -n "$replan_mixed_names" ] || return 0

  replan_fail_json "$replan_mixed_command" "$replan_mixed_mode" \
    mixed_project_settings \
    "environment project id conflicts with per-project settings from the settings file:$replan_mixed_names" \
    8 "settings=[${replan_mixed_json%,}]"
}

# Resolve secret files after dotenv loading. Direct environment values win.
# Errors name only the setting, never the secret value or its file path.
replan_resolve_openproject_secret_files() {
  replan_secret_command=$1
  replan_secret_mode=${2:-invalid}
  replan_resolve_openproject_secret_file \
    "${REPLAN_OPENPROJECT_TOKEN_ENV:-OPENPROJECT_TOKEN}" \
    REPLAN_OPENPROJECT_TOKEN_FILE
  replan_resolve_openproject_secret_file \
    REPLAN_OPENPROJECT_PERIMETER_KEY \
    REPLAN_OPENPROJECT_PERIMETER_KEY_FILE
}

replan_resolve_openproject_secret_file() {
  replan_secret_value_name=$1
  replan_secret_file_name=$2
  replan_secret_value=$(printenv "$replan_secret_value_name" || true)
  replan_secret_file=$(printenv "$replan_secret_file_name" || true)
  [ -z "$replan_secret_value" ] || return 0
  [ -n "$replan_secret_file" ] || return 0
  if [ ! -r "$replan_secret_file" ]; then
    replan_fail_json "$replan_secret_command" "$replan_secret_mode" missing_setting \
      "required setting could not be read: $replan_secret_file_name" 8 \
      "missing_settings=[\"$replan_secret_file_name\"]"
  fi
  replan_secret_value=$(awk '{ lines[NR]=$0 } END { for (i=1; i<=NR; i++) print lines[i] }' "$replan_secret_file" | sed -e '1s/^[[:space:]]*//' -e '$s/[[:space:]]*$//') || {
    replan_fail_json "$replan_secret_command" "$replan_secret_mode" missing_setting \
      "required setting could not be read: $replan_secret_file_name" 8 \
      "missing_settings=[\"$replan_secret_file_name\"]"
  }
  export "$replan_secret_value_name=$replan_secret_value"
}
