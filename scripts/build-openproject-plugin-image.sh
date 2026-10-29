#!/usr/bin/env bash
# Build the OpenProject image containing every repository plugin gem.
#
# Settings:
#   REPLAN_OPENPROJECT_PLUGIN_IMAGE       default replan-openproject-replan-overlay:test
#   REPLAN_OPENPROJECT_PLUGIN_DEPS_IMAGE  default replan/openproject-replan-overlay:17.6.0-deps
#   REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE  default auto
#   REPLAN_OPENPROJECT_BASE_IMAGE         default openproject/openproject:17.6.0
#   REPLAN_OPENPROJECT_FINAL_IMAGE        default openproject/openproject:17.6.0-slim
#   TMPDIR                                default /tmp
#
# Does not write a live OpenProject; it writes only local or disposable
# artifacts.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/build-openproject-plugin-image.sh
#   REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE=full scripts/build-openproject-plugin-image.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "build-openproject-plugin-image.sh" "$@"
PLUGINS_ROOT="$ROOT_DIR/integrations/openproject/plugins"

# Default image names still say "replan-overlay" although the image now
# copies every gem under integrations/openproject/plugins/. Renaming them
# would break the tagged dependency base already used on the live host.
IMAGE="${REPLAN_OPENPROJECT_PLUGIN_IMAGE:-replan-openproject-replan-overlay:test}"
DEPS_IMAGE="${REPLAN_OPENPROJECT_PLUGIN_DEPS_IMAGE:-replan/openproject-replan-overlay:17.6.0-deps}"
BUILD_MODE="${REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE:-auto}"
OPENPROJECT_BASE_IMAGE="${REPLAN_OPENPROJECT_BASE_IMAGE:-openproject/openproject:17.6.0}"
OPENPROJECT_FINAL_IMAGE="${REPLAN_OPENPROJECT_FINAL_IMAGE:-openproject/openproject:17.6.0-slim}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-op-plugin-build.XXXXXX")"

cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

if ! command -v docker >/dev/null 2>&1; then
  echo "docker is required to build the OpenProject plugin image" >&2
  exit 1
fi

if ! docker info >/dev/null 2>&1; then
  echo "docker daemon is not available" >&2
  exit 1
fi

case "$BUILD_MODE" in
  auto|both|deps-base|code|full) ;;
  *)
    echo "unsupported REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE: $BUILD_MODE" >&2
    exit 1
    ;;
esac

plugins=()
for plugin_dir in "$PLUGINS_ROOT"/*; do
  [ -d "$plugin_dir" ] || continue
  gemspec="$(find "$plugin_dir" -maxdepth 1 -name '*.gemspec' -print -quit)"
  [ -n "$gemspec" ] || continue
  plugins+=("$(basename "$plugin_dir")")
done
if [ "${#plugins[@]}" -gt 0 ]; then
  mapfile -t plugins < <(printf '%s\n' "${plugins[@]}" | LC_ALL=C sort)
fi

if [ "${#plugins[@]}" -eq 0 ]; then
  echo "no OpenProject plugin gems found under $PLUGINS_ROOT" >&2
  exit 1
fi

mkdir -p "$TMP_DIR/build"
{
  echo "group :opf_plugins do"
  for plugin in "${plugins[@]}"; do
    require_path="open_project/${plugin#openproject-}"
    printf '  gem "%s",\n      path: "/app/vendor/plugins/%s",\n      require: "%s"\n' \
      "$plugin" "$plugin" "$require_path"
    mkdir -p "$TMP_DIR/build/$plugin"
    cp -a "$PLUGINS_ROOT/$plugin/." "$TMP_DIR/build/$plugin/"
  done
  echo "end"
} >"$TMP_DIR/build/Gemfile.plugins"

builder_copy_lines() {
  local plugin
  for plugin in "${plugins[@]}"; do
    printf 'COPY %s /app/vendor/plugins/%s\n' "$plugin" "$plugin"
  done
}

final_copy_lines() {
  local plugin
  for plugin in "${plugins[@]}"; do
    # shellcheck disable=SC2016 # $APP_USER is for Docker to expand, not the shell.
    printf 'COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/vendor/plugins/%s /app/vendor/plugins/%s\n' \
      "$plugin" "$plugin"
  done
}

code_copy_lines() {
  local plugin
  for plugin in "${plugins[@]}"; do
    # shellcheck disable=SC2016 # $APP_USER is for Docker to expand, not the shell.
    printf 'COPY --chown=\$APP_USER:\$APP_USER %s /app/vendor/plugins/%s\n' "$plugin" "$plugin"
  done
}

build_full_image() {
  cat >"$TMP_DIR/build/Dockerfile" <<DOCKERFILE
FROM ${OPENPROJECT_BASE_IMAGE} AS plugin
$(builder_copy_lines)
COPY Gemfile.plugins /app/Gemfile.plugins
RUN bundle config unset deployment && bundle install && bundle config set deployment 'true'
RUN ./docker/prod/setup/precompile-assets.sh

FROM ${OPENPROJECT_FINAL_IMAGE}
COPY --from=plugin /usr/bin/git /usr/bin/git
$(final_copy_lines)
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/modules /app/modules
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/vendor/bundle /app/vendor/bundle
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /usr/local/bundle /usr/local/bundle
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/public/assets /app/public/assets
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/config/frontend_assets.manifest.json /app/config/frontend_assets.manifest.json
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/Gemfile.* /app/
DOCKERFILE

  docker build -t "$IMAGE" "$TMP_DIR/build"
}

build_deps_image() {
  cat >"$TMP_DIR/build/Dockerfile" <<DOCKERFILE
FROM ${OPENPROJECT_BASE_IMAGE} AS plugin
$(builder_copy_lines)
COPY Gemfile.plugins /app/Gemfile.plugins
RUN bundle config unset deployment && bundle install && bundle config set deployment 'true'
RUN ./docker/prod/setup/precompile-assets.sh

FROM ${OPENPROJECT_FINAL_IMAGE}
COPY --from=plugin /usr/bin/git /usr/bin/git
$(final_copy_lines)
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/modules /app/modules
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/vendor/bundle /app/vendor/bundle
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /usr/local/bundle /usr/local/bundle
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/public/assets /app/public/assets
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/config/frontend_assets.manifest.json /app/config/frontend_assets.manifest.json
COPY --chown=\$APP_USER:\$APP_USER --from=plugin /app/Gemfile.* /app/
DOCKERFILE

  docker build -t "$DEPS_IMAGE" "$TMP_DIR/build"
}

build_code_image() {
  if ! docker image inspect "$DEPS_IMAGE" >/dev/null 2>&1; then
    echo "dependency image is missing: $DEPS_IMAGE" >&2
    echo "run with REPLAN_OPENPROJECT_PLUGIN_BUILD_MODE=deps-base or both first" >&2
    exit 1
  fi

  cat >"$TMP_DIR/build/Dockerfile" <<DOCKERFILE
FROM ${DEPS_IMAGE}
$(code_copy_lines)
DOCKERFILE

  docker build -t "$IMAGE" "$TMP_DIR/build"
}

if [ "$BUILD_MODE" = "auto" ]; then
  if docker image inspect "$DEPS_IMAGE" >/dev/null 2>&1; then
    BUILD_MODE="code"
  else
    BUILD_MODE="both"
  fi
fi

case "$BUILD_MODE" in
  full)
    build_full_image
    ;;
  deps-base)
    build_deps_image
    ;;
  code)
    build_code_image
    ;;
  both)
    build_deps_image
    build_code_image
    ;;
esac

docker run --rm "$IMAGE" sh -lc '
  set -eu
  for gem in '"${plugins[*]}"'; do
    bundle info "$gem" >/dev/null
    grep "$gem" /app/Gemfile.lock >/dev/null
  done
'

printf 'openproject plugin image built: %s\n' "$IMAGE"
printf 'openproject plugin deps image: %s\n' "$DEPS_IMAGE"
printf 'openproject plugin build mode: %s\n' "$BUILD_MODE"
printf 'openproject plugin gems: %s\n' "${plugins[*]}"
