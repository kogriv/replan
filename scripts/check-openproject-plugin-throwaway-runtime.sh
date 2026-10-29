#!/usr/bin/env bash
# Build and exercise the OpenProject plugins in a disposable Docker runtime.
#
# Settings:
#   REPLAN_OPENPROJECT_PLUGIN_IMAGE  default replan-openproject-replan-overlay:test
#   REPLAN_PROJECT                   default examples/replan-self
#   REPLAN_WEEK                      default 2026-W32
#   REPLAN_OPENPROJECT_RUNTIME_HOLD  default 0
#   REPLAN_OPENPROJECT_RUNTIME_PORT  default empty
#   TMPDIR                           default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/check-openproject-plugin-throwaway-runtime.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "check-openproject-plugin-throwaway-runtime.sh" "$@"
IMAGE="${REPLAN_OPENPROJECT_PLUGIN_IMAGE:-replan-openproject-replan-overlay:test}"
PROJECT_PATH="${REPLAN_PROJECT:-$ROOT_DIR/examples/replan-self}"
WEEK="${REPLAN_WEEK:-2026-W32}"
# Seconds to keep the web server up after the checks, for screenshots and a
# look in a browser; the address and the member's login are printed.
HOLD_SECONDS="${REPLAN_OPENPROJECT_RUNTIME_HOLD:-0}"
# Host port for the web server; empty picks a free one. A fixed port also
# becomes OpenProject's host name, so a browser on this machine can sign in.
WEB_PORT="${REPLAN_OPENPROJECT_RUNTIME_PORT:-}"
HOST_NAME="localhost:${WEB_PORT:-8080}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-op-plugin-runtime.XXXXXX")"
NETWORK="replan-op-plugin-$(date +%Y%m%d%H%M%S)-$$"
DB_CONTAINER="$NETWORK-db"
CACHE_CONTAINER="$NETWORK-cache"
WEB_CONTAINER="$NETWORK-web"
MEMBER_LOGIN="replan-member-check:Replan-member-check-2026"
OUTSIDER_LOGIN="replan-outsider-check:Replan-outsider-check-2026"
HOSTILE_TITLE='Hostile <script>alert("replan")</script> & <b>name</b>'

cleanup() {
  docker rm -f "$WEB_CONTAINER" "$DB_CONTAINER" "$CACHE_CONTAINER" >/dev/null 2>&1 || true
  docker network rm "$NETWORK" >/dev/null 2>&1 || true
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required for the OpenProject plugin runtime check" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "docker daemon is not available" >&2
  exit 1
fi

mkdir -p "$TMP_DIR/reports"

"$ROOT_DIR/scripts/build-openproject-plugin-image.sh"

(cd "$ROOT_DIR" && go run ./cmd/replan report dashboard \
  --project "$PROJECT_PATH" \
  --week "$WEEK" \
  --output "$TMP_DIR/reports/latest" \
  --format json >/dev/null)

OVERLAY_PATH="$TMP_DIR/reports/latest/raw/openproject_overlay.json"
test -s "$OVERLAY_PATH"

# The rendered-page checks read a copy whose first open work package carries
# markup in its title, evidence and booked time, which every page must
# escape, and name a second work package whose task page has all its parts.
mkdir -p "$TMP_DIR/rendered"
chmod 755 "$TMP_DIR" "$TMP_DIR/rendered"
RENDERED_OVERLAY="$TMP_DIR/rendered-valid-overlay.json"
RENDERED_IDS="$(PYTHONPATH="$ROOT_DIR/scripts/lib" python3 -m openproject_plugin_rendered_overlay \
  "$OVERLAY_PATH" "$RENDERED_OVERLAY" "$HOSTILE_TITLE" "replan-permission-check")"
RENDERED_WORK_PACKAGE="$(printf '%s\n' "$RENDERED_IDS" | sed -n 1p)"
RENDERED_DETAILED_WORK_PACKAGE="$(printf '%s\n' "$RENDERED_IDS" | sed -n 2p)"

docker network create "$NETWORK" >/dev/null

docker run -d --name "$DB_CONTAINER" --network "$NETWORK" \
  -e POSTGRES_USER=openproject \
  -e POSTGRES_PASSWORD=openproject \
  -e POSTGRES_DB=openproject \
  --health-cmd='pg_isready -U openproject -d openproject' \
  --health-interval=2s \
  --health-timeout=2s \
  --health-retries=60 \
  postgres:17 >/dev/null

docker run -d --name "$CACHE_CONTAINER" --network "$NETWORK" memcached:1.6 >/dev/null

for _ in $(seq 1 60); do
  if [ "$(docker inspect -f '{{.State.Health.Status}}' "$DB_CONTAINER")" = "healthy" ]; then
    break
  fi
  sleep 2
done

if [ "$(docker inspect -f '{{.State.Health.Status}}' "$DB_CONTAINER")" != "healthy" ]; then
  echo "throwaway postgres did not become healthy" >&2
  exit 1
fi

COMMON_ENV=(
  -e SECRET_KEY_BASE=replan_throwaway_runtime_check_secret_key_base
  -e DATABASE_URL=postgres://openproject:openproject@"$DB_CONTAINER":5432/openproject
  -e RAILS_ENV=production
  -e OPENPROJECT_RAILS__CACHE__STORE=memcache
  -e OPENPROJECT_CACHE__MEMCACHE__SERVER="$CACHE_CONTAINER":11211
  -e OPENPROJECT_HOST__NAME="$HOST_NAME"
  -e OPENPROJECT_HTTPS=false
)

docker run --rm --network "$NETWORK" "${COMMON_ENV[@]}" "$IMAGE" \
  ./docker/prod/seeder >/dev/null

docker run --rm --network "$NETWORK" "${COMMON_ENV[@]}" \
  -v "$OVERLAY_PATH:/tmp/replan/openproject_overlay.json:ro" \
  -e REPLAN_OPENPROJECT_OVERLAY_PATH=/tmp/replan/openproject_overlay.json \
  "$IMAGE" \
  bundle exec rails runner '
    require "json"
    require "open_project/replan_overlay/overlay_loader"
    state = OpenProject::ReplanOverlay::OverlayLoader.new.load
    routes = Rails.application.routes.routes.map(&:name).compact.grep(/replan/)
    route_paths = Rails.application.routes.routes.map { |route| route.path.spec.to_s }.grep(/replan/)
    route_config_paths = Rails.application.config.paths["config/routes.rb"].to_a.grep(/replan/)
    route_reloader_paths = Rails.application.routes_reloader.paths.grep(/replan/)
    plugin = Redmine::Plugin.registered_plugins[:openproject_replan_overlay]
    permission = OpenProject::AccessControl.permissions.detect { |candidate| candidate.name == :view_replan_overlay }
    menu = Redmine::MenuManager.items(:project_menu).detect { |candidate| candidate.name == :replan_overlay }
    raise "overlay unavailable: #{state.inspect}" unless state["available"]
    raise "overlay has no work packages" if state["work_packages"].empty?
    raise "route missing: names=#{routes.inspect} paths=#{route_paths.inspect} config_paths=#{route_config_paths.inspect} reloader=#{route_reloader_paths.inspect}" if routes.empty?
    raise "plugin missing" unless plugin
    raise "permission missing" unless permission
    raise "menu missing" unless menu
    puts JSON.generate({
      ok: true,
      check: "valid_overlay",
      work_packages: state["work_packages"].size,
      routes: routes,
      plugin: plugin.id,
      permission: permission.name,
      menu: menu.name
    })
  '

# A project member who is not an admin and holds only view_replan_overlay
# must reach both the project page and the work-package panel.
docker run --rm --network "$NETWORK" "${COMMON_ENV[@]}" "$IMAGE" \
  bundle exec rails runner '
    require "json"
    project = Project.create!(name: "RePlan permission check", identifier: "replan-permission-check", workspace_type: "project", enabled_module_names: %w[work_package_tracking replan_overlay])
    role = ProjectRole.create!(name: "RePlan overlay viewer check", permissions: %i[view_project view_replan_overlay])
    user = User.create!(login: "replan-member-check", firstname: "RePlan", lastname: "Member", mail: "replan-member-check@example.invalid", admin: false, password: "Replan-member-check-2026", password_confirmation: "Replan-member-check-2026")
    Member.create!(project: project, principal: user, roles: [role])
    # A second project with the module, where the same member must never see
    # the plan of the first one.
    other = Project.create!(name: "RePlan other project check", identifier: "replan-other-check", workspace_type: "project", enabled_module_names: %w[work_package_tracking replan_overlay])
    Member.create!(project: other, principal: user, roles: [role])
    # An outsider is a member too, with a role that lacks view_replan_overlay.
    outsider_role = ProjectRole.create!(name: "RePlan outsider check", permissions: %i[view_project view_work_packages])
    outsider = User.create!(login: "replan-outsider-check", firstname: "RePlan", lastname: "Outsider", mail: "replan-outsider-check@example.invalid", admin: false, password: "Replan-outsider-check-2026", password_confirmation: "Replan-outsider-check-2026")
    Member.create!(project: project, principal: outsider, roles: [outsider_role])
    allowed = %w[show work_package].to_h do |action|
      [action, user.allowed_in_project?({ controller: "open_project/replan_overlay/overlays", action: action }, project)]
    end
    raise "non-admin member refused: #{allowed.inspect}" unless allowed.values.all?
    puts JSON.generate({ ok: true, check: "non_admin_member_allowed", allowed: allowed })
  '

docker run --rm --network "$NETWORK" "${COMMON_ENV[@]}" \
  -e REPLAN_OPENPROJECT_OVERLAY_PATH=/tmp/replan/missing-openproject_overlay.json \
  "$IMAGE" \
  bundle exec rails runner '
    require "json"
    require "open_project/replan_overlay/overlay_loader"
    state = OpenProject::ReplanOverlay::OverlayLoader.new.load
    raise "unexpected available missing overlay" if state["available"]
    raise "unexpected error #{state["error"]}" unless state["error"] == "overlay_file_missing"
    puts JSON.generate({
      ok: true,
      check: "missing_overlay",
      available: state["available"],
      error: state["error"]
    })
  '

# Serve OpenProject and request both pages over HTTP as those users.
docker run -d --name "$WEB_CONTAINER" --network "$NETWORK" "${COMMON_ENV[@]}" \
  -v "$TMP_DIR/rendered:/tmp/replan:ro" \
  -e REPLAN_OPENPROJECT_OVERLAY_PATH=/tmp/replan/openproject_overlay.json \
  -e REPLAN_OPENPROJECT_OVERLAY_DIR=/tmp/replan/projects \
  -p "127.0.0.1:${WEB_PORT}:8080" \
  "$IMAGE" ./docker/prod/web >/dev/null
WEB_URL="http://$(docker port "$WEB_CONTAINER" 8080/tcp | head -n 1)"
for _ in $(seq 1 90); do
  if curl -sS --noproxy '*' -o /dev/null -H "Host: $HOST_NAME" "$WEB_URL/login" 2>/dev/null; then
    break
  fi
  sleep 2
done

REPLAN_RENDERED_BASE_URL="$WEB_URL" \
REPLAN_RENDERED_HOST="$HOST_NAME" \
REPLAN_RENDERED_OVERLAY_DIR="$TMP_DIR/rendered" \
REPLAN_RENDERED_VALID_OVERLAY="$RENDERED_OVERLAY" \
REPLAN_RENDERED_PROJECT="replan-permission-check" \
REPLAN_RENDERED_OTHER_PROJECT="replan-other-check" \
REPLAN_RENDERED_MEMBER="$MEMBER_LOGIN" \
REPLAN_RENDERED_OUTSIDER="$OUTSIDER_LOGIN" \
REPLAN_RENDERED_WORK_PACKAGE="$RENDERED_WORK_PACKAGE" \
REPLAN_RENDERED_DETAILED_WORK_PACKAGE="$RENDERED_DETAILED_WORK_PACKAGE" \
REPLAN_RENDERED_HOSTILE_TITLE="$HOSTILE_TITLE" \
  "$ROOT_DIR/scripts/check-openproject-plugin-rendered-pages.sh" || {
    docker logs --tail 80 "$WEB_CONTAINER" >&2
    exit 1
  }

# The "RePlan" link helper writes the task-page link into bound work
# packages; check it against this instance before it touches a live one.
REPLAN_LINKS_WEB_CONTAINER="$WEB_CONTAINER" \
REPLAN_LINKS_WEB_URL="$WEB_URL" \
REPLAN_LINKS_HOST="$HOST_NAME" \
REPLAN_LINKS_PROJECT="replan-permission-check" \
  "$ROOT_DIR/scripts/check-openproject-replan-links.sh"

# The agent kit, the archive an outside agent works from, is used against
# this instance with an ordinary member's token.
REPLAN_KIT_WEB_CONTAINER="$WEB_CONTAINER" \
REPLAN_KIT_WEB_URL="$WEB_URL" \
REPLAN_KIT_HOST="$HOST_NAME" \
  "$ROOT_DIR/scripts/check-openproject-agent-kit-live.sh"

if [ "$HOLD_SECONDS" -gt 0 ]; then
  echo "holding for ${HOLD_SECONDS}s: $WEB_URL (Host: $HOST_NAME), project replan-permission-check, member $MEMBER_LOGIN, work package $RENDERED_WORK_PACKAGE, detailed task $RENDERED_DETAILED_WORK_PACKAGE"
  sleep "$HOLD_SECONDS"
fi

echo "openproject plugin throwaway runtime ok"
