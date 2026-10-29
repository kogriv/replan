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
