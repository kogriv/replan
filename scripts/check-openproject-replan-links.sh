#!/usr/bin/env bash
# Check the "RePlan" link helper against a throwaway OpenProject:
#   scripts/openproject-work-package-toolkit.sh work-package replan-links
# Called by scripts/check-openproject-plugin-throwaway-runtime.sh with the
# running web container. It creates the link custom field for one type of the
# check project, a work package of that type and one of another type, binds
# both to RePlan tasks in a private bindings file of its own, and checks that
# a dry run plans the link and writes nothing, an apply writes only that field,
# a second run changes nothing, a changed value is found and restored, and a
# work package whose type lacks the field is reported, not written.
#
# Settings:
#   REPLAN_LINKS_WEB_CONTAINER  required
#   REPLAN_LINKS_WEB_URL        required
#   REPLAN_LINKS_HOST           required
#   REPLAN_LINKS_PROJECT        required
#   TMPDIR                      default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/check-openproject-replan-links.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "check-openproject-replan-links.sh" "$@"
WEB_CONTAINER="${REPLAN_LINKS_WEB_CONTAINER:?}"
WEB_URL="${REPLAN_LINKS_WEB_URL:?}"
HOST="${REPLAN_LINKS_HOST:?}"
PROJECT="${REPLAN_LINKS_PROJECT:?}"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-op-links.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM
checks=0

fail() {
  printf 'replan link check failed: %s\n' "$1" >&2
  exit 1
}

# One line of "key=value" facts from the runner; the token stays in a file.
docker exec -e PROJECT="$PROJECT" "$WEB_CONTAINER" bundle exec rails runner '
  project = Project.find_by!(identifier: ENV.fetch("PROJECT"))
  types = Type.order(:id).first(2)
  types.each { |type| project.types << type unless project.types.include?(type) }
  field = WorkPackageCustomField.find_or_create_by!(name: "RePlan") { |f| f.field_format = "link"; f.is_for_all = false }
  field.types << types.first unless field.types.include?(types.first)
  project.work_package_custom_fields << field unless project.work_package_custom_fields.include?(field)
  admin = User.where(admin: true).first
  linked, other = types.map do |type|
    WorkPackage.create!(project: project, type: type, subject: "RePlan link check #{type.name}", author: admin,
                        status: Status.default, priority: IssuePriority.default)
  end
  token = Token::API.create!(user: admin, data: { name: "replan link check" })
  puts "FACTS linked=#{linked.id} other=#{other.id} field=customField#{field.id} token=#{token.plain_value}"
' 2>/dev/null | grep '^FACTS ' > "$WORK_DIR/facts" || fail "runner did not set up the link field"

fact() { tr ' ' '\n' < "$WORK_DIR/facts" | sed -n "s/^$1=//p"; }
LINKED="$(fact linked)"
OTHER="$(fact other)"
FIELD="$(fact field)"
export REPLAN_LINKS_TOKEN
REPLAN_LINKS_TOKEN="$(fact token)"
EXPECTED="http://$HOST/projects/$PROJECT/replan_overlay/work_packages/$LINKED"

binding() {
  printf '    - replan_task_id: %s\n      source_ref:\n        source: openproject\n        object_type: work_package\n        object_id: "%s"\n' "$1" "$2"
}
{
  printf 'openproject_private_bindings:\n  project:\n    source: openproject\n    project_id: %s\n  tasks:\n' "$PROJECT"
  binding task.self.create-replan-self-fixture "$LINKED"
} > "$WORK_DIR/bindings.yaml"

run_links() {
  REPLAN_OPENPROJECT_BASE_URL="$WEB_URL" \
  REPLAN_OPENPROJECT_HOST_HEADER="$HOST" \
  REPLAN_OPENPROJECT_FORWARDED_PROTO=http \
  REPLAN_OPENPROJECT_PUBLIC_URL= \
  REPLAN_OPENPROJECT_PROJECT_ID="$PROJECT" \
  REPLAN_OPENPROJECT_TOKEN_ENV=REPLAN_LINKS_TOKEN \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$WORK_DIR/bindings.yaml" \
  REPLAN_OPENPROJECT_FROM_FILE= \
  REPLAN_APPLY="$1" \
    "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package replan-links > "$WORK_DIR/out.json" || true
}

# expect NAME PYTHON-EXPRESSION over the helper output as d
expect() {
  checks=$((checks + 1))
  python3 -c 'import json, sys; d = json.load(open(sys.argv[1])); sys.exit(0 if eval(sys.argv[2]) else 1)' \
    "$WORK_DIR/out.json" "$2" || fail "$1: $(head -c 600 "$WORK_DIR/out.json")"
}

api() {
  curl -sS --noproxy '*' -u "apikey:$REPLAN_LINKS_TOKEN" -H "Host: $HOST" -H "Content-Type: application/json" "$@"
}

field_value() {
  api "$WEB_URL/api/v3/work_packages/$1" | python3 -c 'import json, sys; d = json.load(sys.stdin); print("|".join([d.get(sys.argv[1]) or "", str(d["lockVersion"]), d["subject"]]))' "$FIELD"
}

run_links ""
expect "dry run plans the link" "d['mode'] == 'dry_run' and d['counts']['planned_actions'] == 1 and d['actions'][0]['to'] == '$EXPECTED'"
checks=$((checks + 1))
[ -z "$(field_value "$LINKED" | cut -d'|' -f1)" ] || fail "dry run wrote the field"

run_links 1
expect "apply writes the link" "d['mode'] == 'apply' and d['ok'] and d['counts']['applied_actions'] == 1"
checks=$((checks + 1))
IFS='|' read -r value lock subject <<< "$(field_value "$LINKED")"
[ "$value" = "$EXPECTED" ] || fail "field holds $value"
case "$subject" in "RePlan link check "*) ;; *) fail "subject changed to $subject" ;; esac

run_links ""
expect "second run changes nothing" "d['counts']['planned_actions'] == 0 and d['counts']['unchanged'] == 1"

# Someone edits the link by hand: the dry run finds it and an apply restores it.
api -X PATCH -d "{\"lockVersion\":$lock,\"$FIELD\":\"http://example.invalid/elsewhere\"}" \
  "$WEB_URL/api/v3/work_packages/$LINKED" > /dev/null
run_links ""
expect "changed link is found" "d['counts']['planned_actions'] == 1 and d['actions'][0]['from'] == 'http://example.invalid/elsewhere'"
run_links 1
expect "changed link is restored" "d['ok'] and d['counts']['applied_actions'] == 1"

# A work package whose type lacks the field is reported and not written.
binding task.self.design-static-dashboard "$OTHER" >> "$WORK_DIR/bindings.yaml"
run_links 1
expect "missing field is reported" "not d['ok'] and d['error_codes'] == ['replan_link_field_missing'] and d['counts']['unchanged'] == 1"

printf 'openproject replan link checks ok: %s checks\n' "$checks"
