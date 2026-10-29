#!/usr/bin/env bash
# Throwaway runtime proof for the OpenProject wiki API plugin.
#
# Builds the custom OpenProject image with every plugin gem, boots a disposable
# OpenProject with a clean database, seeds throwaway users, projects and API
# tokens, and runs the design's HTTP cases as real HTTP calls with tokens.
#
# It never talks to a live OpenProject instance. All data is destroyed on exit
# unless REPLAN_KEEP_RUNTIME=1.
# Settings:
#   REPLAN_OPENPROJECT_PLUGIN_IMAGE    default replan-openproject-replan-overlay:test
#   REPLAN_OPENPROJECT_THROWAWAY_PORT  default 18180
#   REPLAN_WIKI_API_EVIDENCE_DIR       default tmp/wiki-api-throwaway
#   REPLAN_OPENPROJECT_SKIP_BUILD      default 0
#   REPLAN_KEEP_RUNTIME                default 0
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/check-openproject-wiki-api-throwaway-runtime.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "check-openproject-wiki-api-throwaway-runtime.sh" "$@"
IMAGE="${REPLAN_OPENPROJECT_PLUGIN_IMAGE:-replan-openproject-replan-overlay:test}"
PORT="${REPLAN_OPENPROJECT_THROWAWAY_PORT:-18180}"
EVIDENCE_DIR="${REPLAN_WIKI_API_EVIDENCE_DIR:-$ROOT_DIR/tmp/wiki-api-throwaway}"
SKIP_BUILD="${REPLAN_OPENPROJECT_SKIP_BUILD:-0}"
KEEP="${REPLAN_KEEP_RUNTIME:-0}"

NETWORK="replan-wiki-api-$(date +%Y%m%d%H%M%S)-$$"
DB_CONTAINER="$NETWORK-db"
CACHE_CONTAINER="$NETWORK-cache"
WEB_CONTAINER="$NETWORK-web"
WORKER_CONTAINER="$NETWORK-worker"
CRON_CONTAINER="$NETWORK-cron"

cleanup() {
  if [ "$KEEP" = "1" ]; then
    echo "REPLAN_KEEP_RUNTIME=1: leaving network $NETWORK and its containers running" >&2
    return
  fi
  docker rm -f "$WEB_CONTAINER" "$WORKER_CONTAINER" "$CRON_CONTAINER" "$DB_CONTAINER" "$CACHE_CONTAINER" \
    >/dev/null 2>&1 || true
  docker network rm "$NETWORK" >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 1; }
docker info >/dev/null 2>&1 || { echo "docker daemon is not available" >&2; exit 1; }

mkdir -p "$EVIDENCE_DIR"
rm -f "$EVIDENCE_DIR"/*.json "$EVIDENCE_DIR"/*.log

if [ "$SKIP_BUILD" != "1" ]; then
  "$ROOT_DIR/scripts/build-openproject-plugin-image.sh"
fi

docker network create "$NETWORK" >/dev/null

docker run -d --name "$DB_CONTAINER" --network "$NETWORK" \
  -e POSTGRES_USER=openproject -e POSTGRES_PASSWORD=openproject -e POSTGRES_DB=openproject \
  --health-cmd='pg_isready -U openproject -d openproject' \
  --health-interval=2s --health-timeout=2s --health-retries=60 \
  postgres:17 >/dev/null
docker run -d --name "$CACHE_CONTAINER" --network "$NETWORK" memcached:1.6 >/dev/null

for _ in $(seq 1 60); do
  [ "$(docker inspect -f '{{.State.Health.Status}}' "$DB_CONTAINER")" = "healthy" ] && break
  sleep 2
done
[ "$(docker inspect -f '{{.State.Health.Status}}' "$DB_CONTAINER")" = "healthy" ] \
  || { echo "throwaway postgres did not become healthy" >&2; exit 1; }

COMMON_ENV=(
  -e SECRET_KEY_BASE=replan_throwaway_runtime_check_secret_key_base
  -e DATABASE_URL=postgres://openproject:openproject@"$DB_CONTAINER":5432/openproject
  -e RAILS_ENV=production
  -e OPENPROJECT_RAILS__CACHE__STORE=memcache
  -e OPENPROJECT_CACHE__MEMCACHE__SERVER="$CACHE_CONTAINER":11211
  -e OPENPROJECT_HOST__NAME=localhost:"$PORT"
  -e OPENPROJECT_HTTPS=false
)

runner() {
  docker run --rm -i --network "$NETWORK" "${COMMON_ENV[@]}" "$@"
}

echo "== case 15a: seeder boots with both gems"
runner "$IMAGE" ./docker/prod/seeder >"$EVIDENCE_DIR/seeder.log" 2>&1 \
  || { tail -40 "$EVIDENCE_DIR/seeder.log" >&2; echo "seeder failed" >&2; exit 1; }

echo "== case 15b: gems, plugin registration and routes"
runner "$IMAGE" sh -lc 'bundle info openproject-wiki_api >/dev/null && bundle info openproject-replan_overlay >/dev/null'
runner -v "$ROOT_DIR/scripts/lib:/replan-lib:ro" "$IMAGE" bundle exec rails runner '
  require "json"
  Rails.application.eager_load!
  routes = API::Root.routes.map { |r| "#{r.request_method} #{r.path}" }
  wanted = {
    "collection_get" => routes.grep(%r{\AGET .*/projects/:id/wiki_pages}).any?,
    "collection_post" => routes.grep(%r{\APOST .*/projects/:id/wiki_pages}).any?,
    "page_get" => routes.grep(%r{\AGET .*/wiki_pages/:id\(}).any?,
    "page_patch" => routes.grep(%r{\APATCH .*/wiki_pages/:id}).any?,
    "page_delete" => routes.grep(%r{\ADELETE .*/wiki_pages/:id}).any?,
    "page_attachments" => routes.grep(%r{\AGET .*/wiki_pages/:id/attachments}).any?
  }
  plugins = Redmine::Plugin.registered_plugins.keys.map(&:to_s).grep(/wiki_api|replan_overlay/).sort
  overlay_routes = Rails.application.routes.routes.map(&:name).compact.grep(/replan/).sort
  result = { ok: wanted.values.all? && plugins.size == 2 && overlay_routes.any?,
             api_routes: wanted, plugins:, overlay_routes:,
             wiki_route_samples: routes.grep(/wiki_pages/).sort.uniq }
  puts "BOOT_JSON=" + JSON.generate(result)
  raise "boot proof failed: #{result.inspect}" unless result[:ok]
' | tee "$EVIDENCE_DIR/boot.log" | grep '^BOOT_JSON=' | sed 's/^BOOT_JSON=//' >"$EVIDENCE_DIR/boot.json"
test -s "$EVIDENCE_DIR/boot.json"

echo "== seed throwaway users, projects, pages and tokens"
runner -v "$ROOT_DIR/scripts/lib:/replan-lib:ro" "$IMAGE" \
  bundle exec rails runner /replan-lib/wiki_api_throwaway_seed.rb >"$EVIDENCE_DIR/seed.log" 2>&1 \
  || { tail -40 "$EVIDENCE_DIR/seed.log" >&2; echo "seed failed" >&2; exit 1; }
SEED_FILE="$(mktemp)"
grep '^SEED_JSON=' "$EVIDENCE_DIR/seed.log" | sed 's/^SEED_JSON=//' >"$SEED_FILE"
test -s "$SEED_FILE"
# Tokens are throwaway, but evidence files still do not carry them.
sed -i '/^SEED_JSON=/d' "$EVIDENCE_DIR/seed.log"

echo "== case 15c: web, worker and cron start with both gems"
docker run -d --name "$WEB_CONTAINER" --network "$NETWORK" "${COMMON_ENV[@]}" \
  -p 127.0.0.1:"$PORT":8080 "$IMAGE" ./docker/prod/web >/dev/null
docker run -d --name "$WORKER_CONTAINER" --network "$NETWORK" "${COMMON_ENV[@]}" "$IMAGE" ./docker/prod/worker >/dev/null
docker run -d --name "$CRON_CONTAINER" --network "$NETWORK" "${COMMON_ENV[@]}" "$IMAGE" ./docker/prod/cron >/dev/null

healthy=0
for _ in $(seq 1 120); do
  if curl -fsS -o /dev/null "http://127.0.0.1:$PORT/health_checks/default" 2>/dev/null; then
    healthy=1
    break
  fi
  if [ "$(docker inspect -f '{{.State.Running}}' "$WEB_CONTAINER")" != "true" ]; then
    break
  fi
  sleep 5
done
docker logs "$WEB_CONTAINER" >"$EVIDENCE_DIR/web.log" 2>&1 || true
[ "$healthy" = "1" ] || { tail -60 "$EVIDENCE_DIR/web.log" >&2; echo "web did not become healthy" >&2; exit 1; }

sleep 20
for name in "$WORKER_CONTAINER" "$CRON_CONTAINER"; do
  docker logs "$name" >"$EVIDENCE_DIR/${name##*-}.log" 2>&1 || true
  if [ "$(docker inspect -f '{{.State.Running}}' "$name")" != "true" ]; then
    tail -40 "$EVIDENCE_DIR/${name##*-}.log" >&2
    echo "${name##*-} exited during boot" >&2
    exit 1
  fi
  if grep -E 'NameError|LoadError|uninitialized constant' "$EVIDENCE_DIR/${name##*-}.log" >&2; then
    echo "${name##*-} logged a load error" >&2
    exit 1
  fi
done

echo "== HTTP cases with API tokens"
http_status=0
python3 "$ROOT_DIR/scripts/lib/wiki_api_throwaway_cases.py" \
  "http://localhost:$PORT" "$SEED_FILE" "$EVIDENCE_DIR/http-cases.json" || http_status=$?

echo "== cases 11, 13, 14: database facts after the HTTP run"
CREATED_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("created_page_id") or 0)' "$EVIDENCE_DIR/http-cases.json")"
JOURNAL_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("journal_page_id") or 0)' "$EVIDENCE_DIR/http-cases.json")"
EXPECTED_WIKIS="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["wiki_rows"])' "$SEED_FILE")"
DELETED_IDS="$(python3 -c 'import json,sys; print(",".join(str(i or 0) for i in json.load(open(sys.argv[1])).get("deleted_page_ids") or []))' "$EVIDENCE_DIR/http-cases.json")"
DELETED_ATTACHMENT_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("deleted_attachment_id") or 0)' "$EVIDENCE_DIR/http-cases.json")"
EDITOR_ID="$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["users"]["editor"])' "$SEED_FILE")"
rm -f "$SEED_FILE"
db_status=0
runner -e CREATED_ID="$CREATED_ID" -e JOURNAL_ID="$JOURNAL_ID" -e EXPECTED_WIKIS="$EXPECTED_WIKIS" -e EDITOR_ID="$EDITOR_ID" \
  -e DELETED_IDS="$DELETED_IDS" -e DELETED_ATTACHMENT_ID="$DELETED_ATTACHMENT_ID" "$IMAGE" \
  bundle exec rails runner '
    require "json"
    created = WikiPage.find(Integer(ENV.fetch("CREATED_ID")))
    edited = WikiPage.find(Integer(ENV.fetch("JOURNAL_ID")))
    editor_id = Integer(ENV.fetch("EDITOR_ID"))
    journals = edited.journals.order(:version)
    deleted_ids = ENV.fetch("DELETED_IDS").split(",").map { |id| Integer(id) }
    deleted_attachment_id = Integer(ENV.fetch("DELETED_ATTACHMENT_ID"))
    result = {
      case_11_no_wiki_row_created: Wiki.count == Integer(ENV.fetch("EXPECTED_WIKIS")),
      wiki_rows: Wiki.count,
      case_13_created_page_author_is_editor: created.author_id == editor_id,
      case_13_created_page_journal_user_is_editor: created.journals.order(:version).last.user_id == editor_id,
      case_13_edited_page_journal_versions: journals.map(&:version),
      case_13_first_journal_user_is_not_editor: journals.first.user_id != editor_id,
      case_13_last_journal_user_is_editor: journals.last.user_id == editor_id,
      case_13_last_journal_has_new_text: journals.last.data.text.include?("Edited by the token user."),
      case_13_first_journal_keeps_old_text: journals.first.data.text.include?("Seeded start page."),
      case_20_deleted_ids: deleted_ids,
      case_20_no_deleted_page_rows: deleted_ids.size == 3 && deleted_ids.none?(&:zero?) &&
                                    WikiPage.where(id: deleted_ids).none?,
      case_20_no_journals_of_deleted_pages: Journal.where(journable_type: "WikiPage", journable_id: deleted_ids).none?,
      case_21_attachment_removed: deleted_attachment_id.positive? && Attachment.where(id: deleted_attachment_id).none?,
      case_14_overlay_routes: Rails.application.routes.routes.map(&:name).compact.grep(/replan/).sort
    }
    result[:ok] = result[:case_11_no_wiki_row_created] &&
                  result[:case_13_created_page_author_is_editor] &&
                  result[:case_13_created_page_journal_user_is_editor] &&
                  result[:case_13_edited_page_journal_versions].size >= 2 &&
                  result[:case_13_first_journal_user_is_not_editor] &&
                  result[:case_13_last_journal_user_is_editor] &&
                  result[:case_13_last_journal_has_new_text] &&
                  result[:case_13_first_journal_keeps_old_text] &&
                  result[:case_20_no_deleted_page_rows] &&
                  result[:case_20_no_journals_of_deleted_pages] &&
                  result[:case_21_attachment_removed] &&
                  result[:case_14_overlay_routes].any?
    puts "DB_JSON=" + JSON.generate(result)
    raise "database facts failed: #{result.inspect}" unless result[:ok]
  ' | tee "$EVIDENCE_DIR/db.log" | grep '^DB_JSON=' | sed 's/^DB_JSON=//' >"$EVIDENCE_DIR/db-facts.json" || db_status=$?

docker logs "$WEB_CONTAINER" >"$EVIDENCE_DIR/web.log" 2>&1 || true

if [ "$http_status" != "0" ] || [ "$db_status" != "0" ]; then
  echo "openproject wiki api throwaway runtime FAILED (http=$http_status db=$db_status); evidence in $EVIDENCE_DIR" >&2
  exit 1
fi

echo "openproject wiki api throwaway runtime ok; evidence in $EVIDENCE_DIR"
