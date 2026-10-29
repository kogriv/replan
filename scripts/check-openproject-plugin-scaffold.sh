#!/usr/bin/env bash
# Check both OpenProject plugin source trees, syntax and contract scaffolding.
#
# Settings:
#   TMPDIR  default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/check-openproject-plugin-scaffold.sh
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "check-openproject-plugin-scaffold.sh" "$@"
PLUGINS_ROOT="$ROOT_DIR/integrations/openproject/plugins"

fail() {
  echo "$*" >&2
  exit 1
}

require_files() {
  local plugin_dir="$1"
  shift
  local rel
  for rel in "$@"; do
    test -s "$plugin_dir/$rel" || fail "missing $plugin_dir/$rel"
  done
}

ruby_syntax() {
  command -v ruby >/dev/null 2>&1 || return 0
  local file
  for file in "$@"; do
    ruby -c "$file" >/dev/null
  done
}

# Plain erb does not know Rails' block form "<%= render(...) do |x| %>", which
# Rails compiles as a statement that captures the block; the check rewrites
# that one form the same way before compiling.
erb_syntax() {
  command -v erb >/dev/null 2>&1 && command -v ruby >/dev/null 2>&1 || return 0
  local file
  for file in "$@"; do
    sed -E 's/<%=(.*[[:space:]]do([[:space:]]*[|][^|]*[|])?[[:space:]]*)%>/<%\1%>/' "$file" | erb -x -T - | ruby -c >/dev/null
  done
}

# Portable search: POSIX grep only. The GitHub-hosted runner has no ripgrep,
# and a missing tool must never look like "no match" (GAP-017).
# search_status prints 0 for a match, 1 for no match, and fails the check on
# any other grep status, such as a missing path, so a negative assertion cannot
# pass silently on an unreadable target.
search_status() {
  local mode="$1"
  local path="$2"
  local pattern="$3"
  local status=0
  [ -e "$path" ] || fail "search target does not exist: $path"
  grep -r -q "$mode" --binary-files=without-match -e "$pattern" -- "$path" || status=$?
  case "$status" in
    0 | 1) printf '%s' "$status" ;;
    *) fail "grep failed with status $status for /$pattern/ in $path" ;;
  esac
}

must_match() {
  local path="$1"
  local pattern="$2"
  [ "$(search_status -E "$path" "$pattern")" = 0 ] || fail "expected /$pattern/ in $path"
}

must_contain() {
  local path="$1"
  local literal="$2"
  [ "$(search_status -F "$path" "$literal")" = 0 ] || fail "expected literal $literal in $path"
}

must_not_match() {
  local path="$1"
  local pattern="$2"
  [ "$(search_status -E "$path" "$pattern")" = 1 ] || fail "unexpected /$pattern/ in $path"
}

check_overlay() {
  local plugin_dir="$PLUGINS_ROOT/openproject-replan_overlay"

  require_files "$plugin_dir" \
    "openproject-replan_overlay.gemspec" \
    "README.md" \
    "lib/open_project/replan_overlay.rb" \
    "lib/open_project/replan_overlay/version.rb" \
    "lib/open_project/replan_overlay/engine.rb" \
    "lib/open_project/replan_overlay/overlay_loader.rb" \
    "lib/open_project/replan_overlay/overlay_presenter.rb" \
    "app/controllers/open_project/replan_overlay/overlays_controller.rb" \
    "app/views/open_project/replan_overlay/overlays/show.html.erb" \
    "app/views/open_project/replan_overlay/overlays/work_package.html.erb" \
    "config/routes.rb"

  must_match "$plugin_dir" "openproject_overlay.json"
  must_match "$plugin_dir" "REPLAN_OPENPROJECT_OVERLAY_PATH"
  must_match "$plugin_dir/lib/open_project/replan_overlay/engine.rb" "permission :view_replan_overlay"
  # Each controller action must be granted by the permission, or OpenProject
  # refuses it to every non-admin (the work-package panel did until 2026-09-25).
  actions_granted "$plugin_dir"
  must_match "$plugin_dir/config/routes.rb" "replan_overlay/gantt"
  must_match "$plugin_dir/lib/open_project/replan_overlay/engine.rb" "menu :project_menu"
  must_match "$plugin_dir/config/routes.rb" "replan_overlay/work_packages/:work_package_id"
  must_match "$plugin_dir/app/controllers/open_project/replan_overlay/overlays_controller.rb" "def work_package"
  must_match "$plugin_dir/lib/open_project/replan_overlay/overlay_presenter.rb" "def work_package_row"
  must_match "$plugin_dir/app/views/open_project/replan_overlay/overlays/show.html.erb" "project_replan_overlay_work_package_path"

  ruby_syntax \
    "$plugin_dir/openproject-replan_overlay.gemspec" \
    "$plugin_dir/lib/open_project/replan_overlay/version.rb" \
    "$plugin_dir/lib/open_project/replan_overlay.rb" \
    "$plugin_dir/lib/open_project/replan_overlay/engine.rb" \
    "$plugin_dir/lib/open_project/replan_overlay/overlay_loader.rb" \
    "$plugin_dir/lib/open_project/replan_overlay/overlay_presenter.rb" \
    "$plugin_dir/lib/open_project/replan_overlay/gantt_presenter.rb" \
    "$plugin_dir/lib/open_project/replan_overlay/task_presenter.rb" \
    "$plugin_dir/app/controllers/open_project/replan_overlay/overlays_controller.rb" \
    "$plugin_dir/config/routes.rb"

  # Every view and partial, so a new one cannot be missed.
  erb_syntax "$plugin_dir"/app/views/open_project/replan_overlay/overlays/*.html.erb

  presenter_tests "$plugin_dir"
  script_syntax "$plugin_dir/app/views/open_project/replan_overlay/overlays/_script.html.erb"
}

# The pages' one inline script is checked by Node when Node is present.
script_syntax() {
  local partial="$1" js
  if ! command -v node >/dev/null 2>&1; then
    echo "script syntax check skipped: node not found" >&2
    return 0
  fi
  js="$(mktemp "${TMPDIR:-/tmp}/replan-script.XXXXXX.js")"
  sed -n '/javascript_tag nonce: true do %>/,/^<% end %>/p' "$partial" | sed '1d;$d' >"$js"
  [ -s "$js" ] || fail "no script found in $partial"
  node --check "$js" || fail "script syntax error in $partial"
  rm -f "$js"
}

# actions_granted: every public action of the overlays controller is listed
# in the permission, whatever actions are added later.
actions_granted() {
  local plugin_dir="$1" controller granted action
  controller="$plugin_dir/app/controllers/open_project/replan_overlay/overlays_controller.rb"
  granted="$(sed -n 's/.*"open_project\/replan_overlay\/overlays" => %i\[\([^]]*\)\].*/\1/p' "$plugin_dir/lib/open_project/replan_overlay/engine.rb")"
  [ -n "$granted" ] || fail "permission for the overlays controller not found"
  while IFS= read -r action; do
    case " $granted " in
      *" $action "*) ;;
      *) fail "controller action $action is not granted by view_replan_overlay" ;;
    esac
  done < <(sed -n '/^ *private$/q; s/^ *def \([a-z_]*\)$/\1/p' "$controller")
}

# The presenter is plain Ruby; its tests need Minitest, not OpenProject. A
# machine without Ruby says so instead of passing silently.
presenter_tests() {
  local plugin_dir="$1"
  if ! command -v ruby >/dev/null 2>&1 || ! ruby -e 'require "minitest"' >/dev/null 2>&1; then
    echo "presenter tests skipped: ruby with minitest not found" >&2
    return 0
  fi
  local test
  for test in "$plugin_dir"/test/*_test.rb; do
    (cd "$plugin_dir" && ruby -Ilib "test/$(basename "$test")" >/dev/null) || fail "presenter tests failed: $(basename "$test")"
  done
}

assert_one_api_class_per_file() {
  local plugin_dir="$1"
  local file
  local listing
  # Collect first: a failing find inside a process substitution is invisible
  # to set -e and would make this loop check zero files and pass.
  listing="$(find "$plugin_dir/lib/api" -type f -name '*.rb' | LC_ALL=C sort)" \
    || fail "could not list API files under $plugin_dir/lib/api"
  [ -n "$listing" ] || fail "no API files found under $plugin_dir/lib/api"
  while IFS= read -r file; do
    PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m check_zeitwerk_class_name "$file" || fail "expected one Zeitwerk-named class in $file"
  done <<<"$listing"
}

check_wiki_api() {
  local plugin_dir="$PLUGINS_ROOT/openproject-wiki_api"
  local engine="$plugin_dir/lib/open_project/wiki_api/engine.rb"
  local payload="$plugin_dir/lib/api/v3/wiki_pages/wiki_page_payload_representer.rb"
  local update_payload="$plugin_dir/lib/api/v3/wiki_pages/wiki_page_update_payload_representer.rb"
  local by_project="$plugin_dir/lib/api/v3/wiki_pages/wiki_pages_by_project_api.rb"
  local list_item="$plugin_dir/lib/api/v3/wiki_pages/wiki_page_list_item_representer.rb"
  local collection="$plugin_dir/lib/api/v3/wiki_pages/wiki_page_collection_representer.rb"
  local delete_contract="$plugin_dir/lib/api/v3/wiki_pages/wiki_page_delete_contract.rb"
  local delete_service="$plugin_dir/lib/api/v3/wiki_pages/wiki_page_delete_service.rb"

  require_files "$plugin_dir" \
    "openproject-wiki_api.gemspec" \
    "README.md" \
    "lib/open_project/wiki_api.rb" \
    "lib/open_project/wiki_api/version.rb" \
    "lib/open_project/wiki_api/engine.rb" \
    "lib/api/v3/wiki_pages/wiki_pages_by_project_api.rb" \
    "lib/api/v3/wiki_pages/wiki_page_collection_representer.rb" \
    "lib/api/v3/wiki_pages/wiki_page_list_item_representer.rb" \
    "lib/api/v3/wiki_pages/wiki_page_payload_representer.rb" \
    "lib/api/v3/wiki_pages/wiki_page_update_payload_representer.rb" \
    "lib/api/v3/wiki_pages/wiki_page_delete_contract.rb" \
    "lib/api/v3/wiki_pages/wiki_page_delete_service.rb"

  must_match "$engine" 'register "openproject-wiki_api"'
  # OpenProject's Zeitwerk inflector turns every "*_api" path segment into
  # "...API", so lib/open_project/wiki_api/ must define OpenProject::WikiAPI.
  # "WikiApi" boots in the seeder and fails with NameError on eager load.
  must_match "$engine" 'module WikiAPI'
  must_match "$plugin_dir/lib/open_project/wiki_api/version.rb" 'module WikiAPI'
  must_not_match "$plugin_dir" 'WikiApi'
  must_not_match "$engine" "project_module"
  must_not_match "$engine" "permission :"
  must_not_match "$plugin_dir" "accept_key_auth"
  must_not_match "$engine" 'require(_relative)?[[:space:]]+['\''"][^'\''"]*api/'
  assert_one_api_class_per_file "$plugin_dir"

  must_match "$engine" 'add_api_endpoint "API::V3::Projects::ProjectsAPI", :id'
  must_match "$engine" 'add_api_endpoint "API::V3::WikiPages::WikiPagesAPI", :id'
  must_match "$engine" 'extend_api_response\(:v3, :wiki_pages, :wiki_page\)'
  must_match "$engine" "formattable_property :text"
  must_match "$engine" 'link :update'
  must_match "$engine" 'cache_if'
  must_match "$engine" 'allowed_in_project\?\(:edit_wiki_pages'
  must_match "$engine" 'reverse_merge\(lock_version: nil\)'
  must_match "$engine" "WikiPageUpdatePayloadRepresenter"
  must_match "$engine" "Endpoints::Update"
  must_match "$engine" "Endpoints::Delete"
  must_match "$engine" "WikiPageDeleteService"
  must_match "$engine" 'link :delete'
  must_match "$engine" 'allowed_in_project\?\(:manage_wiki'
  # Deletion stays on the browser's terms: manage_wiki, and no cascade over
  # child pages. OpenProject 17.6 has no separate delete_wiki_pages permission.
  must_match "$delete_contract" "delete_permission :manage_wiki"
  must_match "$delete_contract" "children"
  must_not_match "$delete_contract" "delete_wiki_pages"
  must_not_match "$plugin_dir/lib" "descendants.each"
  must_match "$delete_service" "BaseServices::Delete"
  must_match "$delete_service" "WikiPageDeleteContract"
  must_match "$by_project" "Endpoints::Create"
  must_match "$by_project" "WikiPagePayloadRepresenter"
  must_match "$by_project" 'authorize_in_project\(:view_wiki_pages'
  must_match "$by_project" "WikiPageCollectionRepresenter"
  must_match "$by_project" 'order\(:title\)'
  must_match "$by_project" 'visible\(current_user\)'
  must_match "$by_project" 'module_enabled\?\(:wiki\)'
  must_match "$collection" "WikiPageListItemRepresenter"
  must_match "$collection" "OffsetPaginatedCollection"
  must_match "$list_item" 'def _type'
  must_match "$list_item" "property :slug"
  must_match "$list_item" "property :lock_version"
  must_match "$payload" "formattable_property :text"
  must_match "$payload" "property :lock_version"
  must_match "$payload" "update_payload?"
  must_match "$payload" "UnwritableProperty"
  must_match "$update_payload" "class WikiPageUpdatePayloadRepresenter"
  must_not_match "$payload" "class WikiPageUpdatePayloadRepresenter"
  must_not_match "$payload" "writeable:"
  must_contain "$plugin_dir/README.md" "GET /api/v3/projects/{id}/wiki_pages"
  must_contain "$plugin_dir/README.md" "POST /api/v3/projects/{id}/wiki_pages"
  must_contain "$plugin_dir/README.md" "PATCH /api/v3/wiki_pages/{id}"
  must_contain "$plugin_dir/README.md" "lockVersion"
  must_contain "$plugin_dir/README.md" "DELETE /api/v3/wiki_pages/{id}"

  must_not_match "$plugin_dir" "ParamsToQueryService"
  must_not_match "$plugin_dir" 'JSON\.parse'
  must_not_match "$plugin_dir" "update_column"
  must_not_match "$plugin_dir" "update_columns"
  must_not_match "$plugin_dir" '\.save[!]*'
  must_not_match "$plugin_dir" "execute_sql"
  must_not_match "$list_item" "formattable_property :text"
  must_not_match "$list_item" "CachedRepresenter"
  must_not_match "$list_item" "property :text"
  must_not_match "$payload" "property :protected"
  must_not_match "$payload" "property :slug"
  must_not_match "$payload" "associated_resource :author"
  must_not_match "$payload" "associated_resource :wiki"

  ruby_syntax \
    "$plugin_dir/openproject-wiki_api.gemspec" \
    "$plugin_dir/lib/open_project/wiki_api/version.rb" \
    "$plugin_dir/lib/open_project/wiki_api.rb" \
    "$engine" \
    "$by_project" \
    "$collection" \
    "$list_item" \
    "$payload" \
    "$update_payload" \
    "$delete_contract" \
    "$delete_service"
}

test -d "$PLUGINS_ROOT/openproject-replan_overlay" || fail "missing overlay plugin"
test -d "$PLUGINS_ROOT/openproject-wiki_api" || fail "missing wiki API plugin"

check_overlay
check_wiki_api

printf 'openproject plugin scaffold ok\n'
