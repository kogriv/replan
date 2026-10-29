#!/usr/bin/env bash
# Use the RePlan agent kit against a throwaway OpenProject as an outside agent
# would: the unpacked archive, its own settings file, no Go toolchain, and the
# API token of a user who holds only the standard "Member" role in the
# project. Called by scripts/check-openproject-plugin-throwaway-runtime.sh
# with the running web container.
#
# It puts the kit's sample plan into the project, adds its relations, works on
# single work packages, loses the bindings file and rebuilds it, and checks
# that nothing needed an administrator.
#
# Settings:
#   REPLAN_KIT_WEB_CONTAINER  required
#   REPLAN_KIT_WEB_URL        required
#   REPLAN_KIT_HOST           required
#   REPLAN_KIT_PROJECT        default agent-kit-check
#   TMPDIR                    default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/check-openproject-agent-kit-live.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "check-openproject-agent-kit-live.sh" "$@"
WEB_CONTAINER="${REPLAN_KIT_WEB_CONTAINER:?}"
WEB_URL="${REPLAN_KIT_WEB_URL:?}"
HOST="${REPLAN_KIT_HOST:?}"
PROJECT="${REPLAN_KIT_PROJECT:-agent-kit-check}"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-agent-kit-live.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM
checks=0

fail() {
  printf 'agent kit live check failed: %s\n' "$1" >&2
  exit 1
}

REPLAN_AGENT_KIT_KEEP="$WORK_DIR/kit" "$ROOT_DIR/scripts/check-agent-kit.sh" > /dev/null
KIT="$WORK_DIR/kit"

# A project and a member with the seeded "Member" role; the token goes
# straight into the kit's settings file.
docker exec -e PROJECT="$PROJECT" "$WEB_CONTAINER" bundle exec rails runner '
  project = Project.create!(name: "Agent kit check", identifier: ENV.fetch("PROJECT"), workspace_type: "project",
                            enabled_module_names: %w[work_package_tracking wiki])
  Type.order(:id).first(2).each { |type| project.types << type unless project.types.include?(type) }
  user = User.create!(login: "#{project.identifier}-member", firstname: "Kit", lastname: "Member", mail: "#{project.identifier}-member@example.invalid",
                      admin: false, password: "Agent-kit-member-2026!", password_confirmation: "Agent-kit-member-2026!")
  Member.create!(project: project, principal: user, roles: [ProjectRole.find_by!(name: "Member")])
  token = Token::API.create!(user: user, data: { name: "agent kit check" })
  reader = User.create!(login: "#{project.identifier}-reader", firstname: "Kit", lastname: "Reader", mail: "#{project.identifier}-reader@example.invalid",
                        admin: false, password: "Agent-kit-reader-2026!", password_confirmation: "Agent-kit-reader-2026!")
  Member.create!(project: project, principal: reader, roles: [ProjectRole.find_by!(name: "Reader")])
  reader_token = Token::API.create!(user: reader, data: { name: "agent kit check" })
  puts "FACTS user=#{user.id} admin=#{user.admin?} token=#{token.plain_value} reader_token=#{reader_token.plain_value}"
' 2>/dev/null | grep '^FACTS ' > "$WORK_DIR/facts" || fail "runner did not set up the member"
fact() { tr ' ' '\n' < "$WORK_DIR/facts" | sed -n "s/^$1=//p"; }
[ "$(fact admin)" = "false" ] || fail "the check user is an administrator"

cp -R "$KIT/examples/small-team" "$WORK_DIR/plan"
sed -e "s#^REPLAN_OPENPROJECT_BASE_URL=.*#REPLAN_OPENPROJECT_BASE_URL=$WEB_URL#" \
    -e "s#^REPLAN_OPENPROJECT_PROJECT_ID=.*#REPLAN_OPENPROJECT_PROJECT_ID=$PROJECT#" \
    -e "s#^REPLAN_OPENPROJECT_PROJECT_NAME=.*#REPLAN_OPENPROJECT_PROJECT_NAME=Agent kit check#" \
    -e "s#^REPLAN_PROJECT=.*#REPLAN_PROJECT=$WORK_DIR/plan#" \
    -e "s#^REPLAN_OPENPROJECT_BINDINGS_PATH=.*#REPLAN_OPENPROJECT_BINDINGS_PATH=$WORK_DIR/bindings.yaml#" \
    -e "s#^OPENPROJECT_TOKEN=.*#OPENPROJECT_TOKEN=$(fact token)#" \
    "$KIT/env.local.example" > "$KIT/.env.local"
# The throwaway instance answers on a local port under another host name.
printf 'REPLAN_OPENPROJECT_HOST_HEADER=%s\nREPLAN_OPENPROJECT_FORWARDED_PROTO=http\n' "$HOST" >> "$KIT/.env.local"

mkdir "$WORK_DIR/nogo"
printf '#!/bin/sh\necho "go must not be needed by the agent kit" >&2\nexit 97\n' > "$WORK_DIR/nogo/go"
chmod +x "$WORK_DIR/nogo/go"

# run APPLY HELPER ARGS...: the helper in the kit, with nothing but the kit's
# settings file, output in out.json.
run() {
  local apply="$1"
  shift
  (cd "$KIT" && env -i PATH="$WORK_DIR/nogo:/usr/bin:/bin" HOME="$WORK_DIR" REPLAN_APPLY="$apply" REPLAN_ACTOR=actor.human.operator "$@") > "$WORK_DIR/out.json" 2> "$WORK_DIR/out.err" || true
}
# expect NAME PYTHON-EXPRESSION over the helper output as d
expect() {
  checks=$((checks + 1))
  python3 -c 'import json, sys; d = json.load(open(sys.argv[1])); sys.exit(0 if eval(sys.argv[2]) else 1)' \
    "$WORK_DIR/out.json" "$2" 2>/dev/null || fail "$1: $(head -c 500 "$WORK_DIR/out.json" "$WORK_DIR/out.err")"
}
TOOLKIT="scripts/openproject-work-package-toolkit.sh"
TASK="task.op.1234"

run "" scripts/apply-openproject-bootstrap.sh
expect "dry run plans the sample's work packages" "d['ok'] and d['mode'] == 'dry_run' and d['counts']['create_work_package'] == 2"
checks=$((checks + 1))
[ ! -e "$WORK_DIR/bindings.yaml" ] || fail "the dry run wrote the bindings file"

run 1 scripts/apply-openproject-bootstrap.sh
expect "a member creates the work packages" "d['ok'] and d['counts']['created_work_package'] == 2"
checks=$((checks + 1))
grep -q "$TASK" "$WORK_DIR/bindings.yaml" || fail "the bindings file lacks $TASK"

run 1 scripts/apply-openproject-bootstrap.sh
expect "a second run creates nothing" "d['ok'] and d['counts']['created_work_package'] == 0 and d['counts']['matched_existing_work_package'] == 2"

run "" "$TOOLKIT" work-package list
cp "$WORK_DIR/out.json" "$WORK_DIR/work-packages.before-relations.json"
run 1 scripts/apply-openproject-relations.sh
expect "a member creates the relation between the two tasks" "d['counts']['created_relation'] == 1"
expect "the dates around the new relation are read back and any move is reported" "'rescheduling_unknown' not in d.get('warning_codes', []) and (('openproject_rescheduled' in d.get('warning_codes', [])) == bool(d.get('rescheduled_work_packages')))"
run "" "$TOOLKIT" work-package list
expect "relations leave every sample work package manually scheduled" "d['ok'] and all(item.get('schedule_manually') is True for item in d['items'])"
checks=$((checks + 1))
python3 -c 'import json, sys; before = json.load(open(sys.argv[1])); after = json.load(open(sys.argv[2])); dates = lambda d: {i["work_package_id"]: (i.get("start_date", ""), i.get("due_date", "")) for i in d["items"]}; sys.exit(0 if dates(before) == dates(after) else 1)' \
  "$WORK_DIR/work-packages.before-relations.json" "$WORK_DIR/out.json" \
  || fail "relation apply changed a sample work-package date"

expect "the work packages are listed and mapped" "d['ok'] and d['counts'] == {'mapped': 2, 'unmapped': 0, 'work_packages': 2}"
run "" "$TOOLKIT" user list
expect "the project's assignees are readable by a member" "d['ok'] and any(item.get('name') == 'Kit Member' for item in d['items'])"
run "" "$TOOLKIT" work-package inspect --task "$TASK"
expect "the member is the assignee of its own plan" "d['ok'] and 'Kit Member' in json.dumps(d['items'])"

run "" "$TOOLKIT" work-package update-text --task "$TASK" --description "Agent kit description v2."
expect "a text dry run reports the current lock version" "d['ok'] and d['mode'] == 'dry_run' and isinstance(d['actions'][0]['current_lock_version'], int)"
TEXT_LOCK="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["actions"][0]["current_lock_version"])' "$WORK_DIR/out.json")"
run 1 "$TOOLKIT" work-package update-text --task "$TASK" --description "Agent kit description v2." --lock-version "$TEXT_LOCK"
expect "a member updates a description with the checked lock version" "d['ok'] and d['counts']['openproject_changes'] == 1 and d['counts']['applied_actions'] == 1"
run 1 "$TOOLKIT" work-package update-text --task "$TASK" --description "Stale description overwrite." --lock-version "$TEXT_LOCK"
expect "a stale work-package lock version is refused" "not d['ok'] and d['error_codes'] == ['openproject_work_package_changed'] and d['counts'].get('applied_actions', 0) == 0"
run "" "$TOOLKIT" work-package inspect --task "$TASK"
expect "the stale text update changed nothing" "d['ok'] and d['items'][0]['user_authored']['description_raw'] == 'Agent kit description v2.'"

run "" "$TOOLKIT" work-package update-text --task "$TASK" --subject "Agent kit corrected task title" --evidence evidence.github.pr.45
expect "a bound subject dry run plans OpenProject and RePlan title changes" "d['ok'] and d['actions'][0]['openproject_change'] and d['actions'][0]['replan_title_change']"
SUBJECT_LOCK="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["actions"][0]["current_lock_version"])' "$WORK_DIR/out.json")"
run 1 "$TOOLKIT" work-package update-text --task "$TASK" --subject "Agent kit corrected task title" --evidence evidence.github.pr.45 --lock-version "$SUBJECT_LOCK"
expect "a bound subject updates OpenProject and the copied RePlan task" "d['ok'] and d['counts']['openproject_changes'] == 1 and d['counts']['replan_title_changes'] == 1 and d['counts']['applied_actions'] == 2"
checks=$((checks + 1))
python3 -c 'import sys, yaml; tasks = yaml.safe_load(open(sys.argv[1]))["tasks"]; sys.exit(0 if next(t["title"] for t in tasks if t["id"] == sys.argv[2]) == "Agent kit corrected task title" else 1)' \
  "$WORK_DIR/plan/canonical/tasks.yaml" "$TASK" || fail "the copied RePlan task title was not updated"

run "" "$TOOLKIT" work-package update-dates --task "$TASK" --start-date 2026-10-12 --due-date 2026-10-13
expect "a dry run plans the dates" "d['ok'] and d['mode'] == 'dry_run' and d['counts']['planned_actions'] == 1"
run 1 "$TOOLKIT" work-package update-dates --task "$TASK" --start-date 2026-10-12 --due-date 2026-10-13
expect "a member sets the dates" "d['ok'] and d['counts']['applied_actions'] == 1"
run "" "$TOOLKIT" work-package update-dates --task "$TASK" --start-date 2026-10-25 --due-date 2026-10-25
expect "a Sunday date dry run keeps the non-working day" "d['ok'] and d['actions'][0]['set_ignore_non_working_days'] and 'Sunday' in d['actions'][0]['ignore_non_working_days_reason']"
run 1 "$TOOLKIT" work-package update-dates --task "$TASK" --start-date 2026-10-25 --due-date 2026-10-25
expect "a member sets a Sunday date" "d['ok'] and d['counts']['applied_actions'] == 1"
run "" "$TOOLKIT" work-package inspect --task "$TASK"
expect "the Sunday date is stored without scheduler movement" "d['ok'] and d['items'][0]['start_date'] == '2026-10-25' and d['items'][0]['due_date'] == '2026-10-25'"
run 1 "$TOOLKIT" comment add --task "$TASK" --text "Checked by the agent kit."
expect "a member comments" "d['ok'] and d['counts']['applied_actions'] == 1"
run 1 "$TOOLKIT" work-package update-status --task "$TASK" --status "No such status"
expect "an unknown status is refused, not written" "not d['ok'] and 'invalid_status' in d['error_codes'] and d['counts'].get('applied_actions', 0) == 0"
run "" "$TOOLKIT" work-package update-status --task "$TASK" --status in_progress
expect "a RePlan status plans its mapped OpenProject status" "d['ok'] and d['mode'] == 'dry_run' and d['actions'][0]['to_status'] == 'In progress'"

run "" scripts/check-openproject-drift.sh
expect "the drift check reads the project as a member" "d['counts']['local_tasks'] == 2 and d['counts']['live_task_candidates'] == 2 and d['counts']['unbound_local_tasks'] == 0"
expect "the drift check finds no automatically scheduled bound work package" "'openproject_automatic_scheduling' not in d['warning_codes']"
run "" scripts/openproject-operator-report.sh
expect "the operator report reads the project as a member" "d['counts']['mapped_tasks'] == 2"

# The bindings file is lost: it is rebuilt from OpenProject by title.
cp "$WORK_DIR/bindings.yaml" "$WORK_DIR/bindings.before"
rm "$WORK_DIR/bindings.yaml"
run 1 "$TOOLKIT" backlog reconcile
expect "lost bindings are rebuilt" "d['ok'] and d['counts']['bind_existing_actions'] == 2 and d['counts']['create_work_package_actions'] == 0"
checks=$((checks + 1))
diff <(grep -A4 "replan_task_id: task\." "$WORK_DIR/bindings.before" | grep -E "replan_task_id|object_id") \
     <(grep -A4 "replan_task_id: task\." "$WORK_DIR/bindings.yaml" | grep -E "replan_task_id|object_id") > /dev/null \
  || fail "rebuilt bindings differ from the lost ones"

# The wiki, where an outside member mostly works: pages and attachments,
# with the member's own permissions.
WIKI="scripts/openproject-wiki.sh"
printf 'First line.\nSecond line.\n' > "$WORK_DIR/page-v1.md"
printf 'First line.\nSecond line, edited.\nThird line.\n' > "$WORK_DIR/page-v2.md"
printf 'Written by someone else.\n' > "$WORK_DIR/page-other.md"
page_id() { python3 -c 'import json, sys; d = json.load(open(sys.argv[1])); print((d["items"] or d["actions"])[0].get("id") or d["actions"][0].get("page_id"))' "$WORK_DIR/out.json"; }

run "" "$WIKI" page create --title "Kit notes" --text-file "$WORK_DIR/page-v1.md"
expect "a dry run plans the page" "d['ok'] and d['mode'] == 'dry_run' and d['counts']['planned_actions'] == 1"
run "" "$WIKI" page list
expect "the dry run created no page" "d['ok'] and not any(item['title'] == 'Kit notes' for item in d['items'])"
run 1 "$WIKI" page create --title "Kit notes" --text-file "$WORK_DIR/page-v1.md"
expect "a member creates a page" "d['ok'] and d['counts']['applied_actions'] == 1 and d['items'][0]['title'] == 'Kit notes'"
PAGE="$(page_id)"
run 1 "$WIKI" page create --title "Kit notes" --text-file "$WORK_DIR/page-v1.md"
expect "a second page of the same title is refused" "not d['ok'] and d['error_codes'] == ['wiki_page_title_taken']"

run "" "$WIKI" page show --title "Kit notes" --output "$WORK_DIR/page-read.md"
expect "the page is read by title" "d['ok'] and d['items'][0]['id'] == $PAGE and d['items'][0]['text']['lines'] == 2 and 'text' not in d"
checks=$((checks + 1))
cmp -s "$WORK_DIR/page-v1.md" "$WORK_DIR/page-read.md" || fail "the text read back differs from the text written"
LOCK="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["items"][0]["lock_version"])' "$WORK_DIR/out.json")"

run "" "$WIKI" page update --id "$PAGE" --text-file "$WORK_DIR/page-v2.md" --diff-output "$WORK_DIR/page.diff"
expect "a dry run counts the changed lines" "d['ok'] and d['mode'] == 'dry_run' and d['actions'][0]['lines_added'] == 2 and d['actions'][0]['lines_removed'] == 1"
checks=$((checks + 1))
grep -q '^+Third line\.$' "$WORK_DIR/page.diff" || fail "the diff file lacks the added line"
run "" "$WIKI" page show --id "$PAGE"
expect "the dry run left the text alone" "d['text'] == open('$WORK_DIR/page-v1.md').read()"

# Someone else edits the page after it was read: the update that names the
# lock version it was read at is refused, and their text stays.
run 1 "$WIKI" page update --id "$PAGE" --text-file "$WORK_DIR/page-other.md"
expect "an update is written" "d['ok'] and d['counts']['applied_actions'] == 1"
run 1 "$WIKI" page update --id "$PAGE" --text-file "$WORK_DIR/page-v2.md" --expect-lock-version "$LOCK"
expect "an update of a page changed since it was read is refused" "not d['ok'] and d['error_codes'] == ['wiki_page_changed']"
run "" "$WIKI" page show --id "$PAGE"
expect "the other edit was not overwritten" "d['text'] == open('$WORK_DIR/page-other.md').read()"
run 1 "$WIKI" page update --id "$PAGE" --text-file "$WORK_DIR/page-v2.md"
expect "an update after reading again is written" "d['ok'] and d['counts']['applied_actions'] == 1"
run 1 "$WIKI" page update --id "$PAGE" --text-file "$WORK_DIR/page-v2.md"
expect "the same text again changes nothing" "d['ok'] and d['counts'] == {'planned_actions': 0, 'unchanged': 1}"

run 1 "$WIKI" page create --title "Kit child" --text-file "$WORK_DIR/page-v1.md" --parent-id "$PAGE"
expect "a child page is created under its parent" "d['ok'] and d['items'][0]['parent_id'] == $PAGE"
CHILD="$(page_id)"

head -c 70000 /dev/urandom > "$WORK_DIR/blob.bin"
run "" "$WIKI" attachment upload --id "$PAGE" --file "$WORK_DIR/blob.bin"
expect "a dry run plans the upload" "d['ok'] and d['mode'] == 'dry_run' and d['actions'][0]['bytes'] == 70000"
run 1 "$WIKI" attachment upload --id "$PAGE" --file "$WORK_DIR/blob.bin"
expect "a member attaches a file" "d['ok'] and d['items'][0]['file_name'] == 'blob.bin' and d['items'][0]['file_size'] == 70000"
ATTACHMENT="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["items"][0]["id"])' "$WORK_DIR/out.json")"
run "" "$WIKI" attachment download --attachment "$ATTACHMENT" --output "$WORK_DIR/blob.back"
expect "the attachment is downloaded" "d['ok'] and d['items'][0]['bytes'] == 70000"
checks=$((checks + 1))
cmp -s "$WORK_DIR/blob.bin" "$WORK_DIR/blob.back" || fail "the downloaded attachment differs from the uploaded file"
run 1 "$WIKI" attachment upload --id "$PAGE" --file "$WORK_DIR/blob.bin"
expect "a second file of the same name is refused" "not d['ok'] and d['error_codes'] == ['attachment_name_taken']"
head -c 1000 /dev/urandom > "$WORK_DIR/blob.bin"
run 1 "$WIKI" attachment upload --id "$PAGE" --file "$WORK_DIR/blob.bin" --replace
expect "a file is replaced on request" "d['ok'] and d['actions'][0]['replaces'] == [$ATTACHMENT] and d['items'][0]['file_size'] == 1000"
run "" "$WIKI" attachment list --id "$PAGE"
expect "one attachment of that name is left" "d['ok'] and [a['file_size'] for a in d['items']] == [1000]"
REPLACED="$(python3 -c 'import json, sys; print(json.load(open(sys.argv[1]))["items"][0]["id"])' "$WORK_DIR/out.json")"
run 1 "$WIKI" attachment delete --attachment "$REPLACED"
expect "a member deletes an attachment" "d['ok'] and d['counts']['applied_actions'] == 1"

# Deleting: a parent is never deleted over its child, and a page goes with a
# dry run first. The seeded Member role holds manage_wiki.
run 1 "$WIKI" page delete --id "$PAGE"
expect "a page with a child is not deleted" "not d['ok'] and d['error_codes'] == ['wiki_page_has_children']"
run "" "$WIKI" page delete --id "$CHILD"
expect "a dry run plans the deletion" "d['ok'] and d['mode'] == 'dry_run' and d['counts']['planned_actions'] == 1"
run 1 "$WIKI" page delete --id "$CHILD"
expect "a member deletes the child page" "d['ok'] and d['counts']['applied_actions'] == 1"

# A user who may only read is told which permission is missing, and nothing
# is written.
sed -i "s#^OPENPROJECT_TOKEN=.*#OPENPROJECT_TOKEN=$(fact reader_token)#" "$KIT/.env.local"
run "" "$WIKI" page show --id "$PAGE"
expect "a reader reads the page" "d['ok'] and d['items'][0]['id'] == $PAGE"
run 1 "$WIKI" page update --id "$PAGE" --text-file "$WORK_DIR/page-v1.md"
expect "a reader's edit is refused with the permission named" "not d['ok'] and d['error_codes'] == ['wiki_permission_denied'] and 'edit_wiki_pages' in d['errors'][0]['message']"
run 1 "$WIKI" page delete --id "$PAGE"
expect "a reader's deletion is refused" "not d['ok'] and d['error_codes'] == ['wiki_permission_denied']"
sed -i "s#^OPENPROJECT_TOKEN=.*#OPENPROJECT_TOKEN=$(fact token)#" "$KIT/.env.local"
run "" "$WIKI" page show --id "$PAGE"
expect "the page survived the reader" "d['text'] == open('$WORK_DIR/page-v2.md').read()"
run "" "$WIKI" page show --id 999999
expect "an unknown page is reported" "not d['ok'] and d['error_codes'] == ['wiki_page_not_found']"
checks=$((checks + 1))
! grep -rq "Written by someone else\|Second line" "$WORK_DIR/out.err" || fail "page text reached the error output"

# What needs an administrator says so, and writes nothing.
run 1 "$TOOLKIT" work-package replan-links
expect "the missing link field is reported" "not d['ok'] and set(d['error_codes']) == {'replan_link_field_missing'}"

checks=$((checks + 1))
! grep -rq "go must not be needed" "$WORK_DIR"/out.* || fail "a helper tried to build with go"

printf 'openproject agent kit live checks ok: %s checks\n' "$checks"
