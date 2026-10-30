#!/usr/bin/env sh

# Load local OpenProject connection settings without executing .env as shell.
# Precedence: already exported environment > .env.local > .env.
replan_load_openproject_dotenv() {
  root_dir=$1
  replan_settings_list_path="$root_dir/scripts/lib/settings.list"
  [ -f "$replan_settings_list_path" ] || return 0
  replan_settings_names=$(sed -e '/^[[:space:]]*#/d' -e '/^[[:space:]]*$/d' "$replan_settings_list_path")
  replan_load_openproject_dotenv_file "$root_dir/.env.local"
  replan_load_openproject_dotenv_file "$root_dir/.env"
  # A release unpacked without the Go sources carries its own binary; the
  # helpers use it instead of building one.
  if [ -z "${REPLAN_BIN:-}" ] && [ -x "$root_dir/bin/replan" ]; then
    export REPLAN_BIN="$root_dir/bin/replan"
  fi
}

replan_load_openproject_dotenv_file() {
  dotenv_path=$1
  [ -f "$dotenv_path" ] || return 0

  while IFS= read -r line || [ -n "$line" ]; do
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

    case "
$replan_settings_names
" in
      *"
$key
"*) ;;
      *) continue ;;
    esac

    if printenv "$key" >/dev/null 2>&1; then
      continue
    fi

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

    export "$key=$value"
  done < "$dotenv_path"
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
