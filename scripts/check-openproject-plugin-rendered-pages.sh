#!/usr/bin/env bash
# Request both RePlan plugin pages over HTTP from a running throwaway
# OpenProject, as users who are not admins, and check what they render.
#
# The throwaway runtime check starts the server and calls this script. It
# needs:
#
#   REPLAN_RENDERED_BASE_URL       the server, e.g. http://127.0.0.1:32768
#   REPLAN_RENDERED_HOST           the Host header OpenProject expects
#   REPLAN_RENDERED_OVERLAY_DIR    the host directory mounted into the server;
#                                  the plugin reads openproject_overlay.json
#                                  from it, and this script swaps the file to
#                                  check the missing and invalid cases
#   REPLAN_RENDERED_VALID_OVERLAY  a valid overlay to restore afterwards
#   REPLAN_RENDERED_PROJECT        the project identifier
#   REPLAN_RENDERED_MEMBER         login:password of a member with
#                                  view_replan_overlay
#   REPLAN_RENDERED_OUTSIDER       login:password of a member without it
#   REPLAN_RENDERED_WORK_PACKAGE   a work package id present in the overlay
#   REPLAN_RENDERED_HOSTILE_TITLE  that work package's title, which carries
#                                  markup that must come out escaped
#
# REPLAN_RENDERED_FAILED_PAGES, when set, is a directory that receives the
# body of every page a check failed on.
# Settings:
#   REPLAN_RENDERED_BASE_URL               required
#   REPLAN_RENDERED_HOST                   required
#   REPLAN_RENDERED_OVERLAY_DIR            required
#   REPLAN_RENDERED_VALID_OVERLAY          required
#   REPLAN_RENDERED_PROJECT                required
#   REPLAN_RENDERED_MEMBER                 required
#   REPLAN_RENDERED_OUTSIDER               required
#   REPLAN_RENDERED_WORK_PACKAGE           required
#   REPLAN_RENDERED_DETAILED_WORK_PACKAGE  required
#   REPLAN_RENDERED_OTHER_PROJECT          required
#   REPLAN_RENDERED_HOSTILE_TITLE          required
#   REPLAN_RENDERED_FAILED_PAGES           default empty
#   TMPDIR                                 default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/check-openproject-plugin-rendered-pages.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "check-openproject-plugin-rendered-pages.sh" "$@"
BASE_URL="${REPLAN_RENDERED_BASE_URL:?}"
HOST="${REPLAN_RENDERED_HOST:?}"
OVERLAY_DIR="${REPLAN_RENDERED_OVERLAY_DIR:?}"
VALID_OVERLAY="${REPLAN_RENDERED_VALID_OVERLAY:?}"
PROJECT="${REPLAN_RENDERED_PROJECT:?}"
MEMBER="${REPLAN_RENDERED_MEMBER:?}"
OUTSIDER="${REPLAN_RENDERED_OUTSIDER:?}"
WORK_PACKAGE="${REPLAN_RENDERED_WORK_PACKAGE:?}"
DETAILED_WORK_PACKAGE="${REPLAN_RENDERED_DETAILED_WORK_PACKAGE:?}"
OTHER_PROJECT="${REPLAN_RENDERED_OTHER_PROJECT:?}"
HOSTILE_TITLE="${REPLAN_RENDERED_HOSTILE_TITLE:?}"

OVERLAY="$OVERLAY_DIR/openproject_overlay.json"
PROJECT_PAGE="/projects/$PROJECT/replan_overlay"
PANEL="$PROJECT_PAGE/work_packages/$WORK_PACKAGE"
GANTT="$PROJECT_PAGE/gantt"
UNKNOWN_PANEL="$PROJECT_PAGE/work_packages/999999999"

WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-op-rendered.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM
BODY="$WORK_DIR/body.html"
HEADERS="$WORK_DIR/headers.txt"
failures=0
checks=0

fail() {
  echo "rendered page check failed: $*" >&2
  failures=$((failures + 1))
  if [ -n "${REPLAN_RENDERED_FAILED_PAGES:-}" ]; then
    mkdir -p "$REPLAN_RENDERED_FAILED_PAGES"
    cp "$BODY" "$REPLAN_RENDERED_FAILED_PAGES/failure-$failures.html" 2>/dev/null || true
  fi
}

# fetch JAR PATH: request PATH with the cookies in JAR, leave the body in
# $BODY and the status code in $STATUS.
fetch() {
  STATUS="$(curl -sS --noproxy '*' -o "$BODY" -D "$HEADERS" -w '%{http_code}' \
    -b "$1" -c "$1" -H "Host: $HOST" "$BASE_URL$2")"
}

# expect_script_nonce NAME: the RePlan script carries the nonce of the page's
# Content Security Policy; without it OpenProject's browsers refuse to run it.
expect_script_nonce() {
  local nonce
  checks=$((checks + 1))
  nonce="$(tr -d '\r' <"$HEADERS" | grep -i '^content-security-policy:' | grep -oE "'nonce-[^']+'" | head -n 1 | sed "s/^'nonce-//; s/'\$//")"
  if [ -z "$nonce" ]; then
    fail "$1: no nonce in the Content Security Policy"
  elif ! grep -B 8 'var MAX_TABS' "$BODY" | grep -qF "<script nonce=\"$nonce\""; then
    fail "$1: the RePlan script lacks the page nonce"
  fi
}

expect_status() {
  checks=$((checks + 1))
  [ "$STATUS" = "$2" ] || fail "$1: status $STATUS, want $2"
}

expect_text() {
  checks=$((checks + 1))
  grep -qF -- "$2" "$BODY" || fail "$1: page lacks $(printf '%q' "$2")"
}

expect_no_text() {
  checks=$((checks + 1))
  if grep -qF -- "$2" "$BODY"; then fail "$1: page has $(printf '%q' "$2")"; fi
}

# expect_no_fixed_colours NAME: the plugin's own styles take every colour from
# OpenProject's variables, so its themes and Dark Reader recolour the page.
expect_no_fixed_colours() {
  local styles
  checks=$((checks + 1))
  styles="$(sed -n "/Primer variables/,/<\/style>/p" "$BODY")"
  if [ -z "$styles" ]; then
    fail "$1: plugin styles not found"
  elif printf '%s' "$styles" | grep -qE '#[0-9a-fA-F]{3,8}\b|rgba?\('; then
    fail "$1: fixed colour in plugin styles: $(printf '%s' "$styles" | grep -oE '#[0-9a-fA-F]{3,8}\b|rgba?\([^)]*\)' | head -n 3 | tr '\n' ' ')"
  fi
}

html_escape() {
  printf '%s' "$1" | sed -e 's/&/\&amp;/g' -e 's/</\&lt;/g' -e 's/>/\&gt;/g' -e 's/"/\&quot;/g' -e "s/'/\&#39;/g"
}

# login JAR LOGIN:PASSWORD: sign in through the login form, as a browser does,
# following the redirects of the sign-in flow on this server, and check that
# the session is signed in.
login() {
  local jar=$1 user=${2%%:*} password=${2#*:} token location hops
  fetch "$jar" /login
  token="$(sed -n 's/.*name="authenticity_token" value="\([^"]*\)".*/\1/p' "$BODY" | head -n 1)"
  if [ -z "$token" ]; then
    fail "login form for $user has no authenticity token (status $STATUS)"
    return
  fi
  location="$(curl -sS --noproxy '*' -o "$BODY" -w '%{redirect_url}' \
    -b "$jar" -c "$jar" -H "Host: $HOST" \
    --data-urlencode "authenticity_token=$token" \
    --data-urlencode "username=$user" \
    --data-urlencode "password=$password" \
    "$BASE_URL/login")"
  for hops in 1 2 3 4 5; do
    [ -n "$location" ] || break
    location="/${location#*://*/}"
    location="$(curl -sS --noproxy '*' -o "$BODY" -w '%{redirect_url}' \
      -b "$jar" -c "$jar" -H "Host: $HOST" "$BASE_URL$location")"
  done
  fetch "$jar" /my/account
  expect_status "signed in as $user after $hops redirects" 200
}

member_jar="$WORK_DIR/member.cookies"
outsider_jar="$WORK_DIR/outsider.cookies"
anonymous_jar="$WORK_DIR/anonymous.cookies"
login "$member_jar" "$MEMBER"
login "$outsider_jar" "$OUTSIDER"

hostile_escaped="$(html_escape "$HOSTILE_TITLE")"

# Valid overlay: both pages render their sections, and the member sees the
# RePlan entry in the project menu.
cp "$VALID_OVERLAY" "$OVERLAY"
fetch "$member_jar" "$PROJECT_PAGE"
expect_status "project page" 200
for text in "PageHeader" "Waiting on a person" "Overloaded weeks ahead" 'data-counter="overloaded"' \
  "Needs attention" 'data-name="replan_overlay"' 'class="replan-overlay__pointer"'; do
  expect_text "project page" "$text"
done
# Like every item of the project menu, RePlan has an icon.
expect_text "project menu icon" "octicon-graph-bar-vertical"
# The plan and the load live on the Gantt page; milestone 1's task list and
# resource cards are gone from the overview.
for text in "<h3>Tasks</h3>" "<h3>Resources</h3>" 'class="replan-overlay__timeline"' 'class="replan-overlay__resources"' "<summary>Amounts</summary>"; do
  expect_no_text "project page without the old list" "$text"
done
# Resources by name, ids only in tooltips.
expect_text "project page names" "RePlan maintainer"
expect_no_text "project page names" "<td>resource."
expect_no_text "project page artifacts" "RePlan artifacts"
expect_no_text "project page" "No RePlan overlay"
expect_no_fixed_colours "project page"
expect_script_nonce "project page"
expect_text "project page tabs" "data-replan-tabs"
expect_text "project page escaping" "$hostile_escaped"
expect_no_text "project page escaping" "$HOSTILE_TITLE"

fetch "$member_jar" "$GANTT"
expect_status "gantt" 200
for text in "PageHeader" 'aria-label="RePlan views"' "data-replan-gantt" "replan-gantt__axis" "replan-gantt__bar" \
  "replan-gantt__lane" "replan-gantt__status" ">Open (" 'data-name="replan_overlay_gantt"' \
  'name="scale"' 'name="from"' ">Whole project<" "data-replan-jump=\"status\"" "data-replan-pan hidden" \
  "data-replan-tabs"; do
  expect_text "gantt" "$text"
done
expect_script_nonce "gantt"
for scale in days months; do
  fetch "$member_jar" "$GANTT?scale=$scale"
  expect_status "gantt in $scale" 200
  expect_text "gantt in $scale" "<option value=\"$scale\" selected>"
done
expect_no_fixed_colours "gantt"
expect_text "gantt escaping" "$hostile_escaped"
expect_no_text "gantt escaping" "$HOSTILE_TITLE"
fetch "$member_jar" "$GANTT?lanes=0"
expect_status "gantt without lanes" 200
expect_no_text "gantt without lanes" 'class="replan-gantt__lane"'
# The weekly load profile: a row per resource, the waterline, a red hump
# only for a resource-week RePlan's workload still finds overloaded, and a
# grey one where the plan went over but that work is done.
fetch "$member_jar" "$GANTT"
# Markup, not the page's style sheet or script, which name the same classes.
for text in "data-replan-load>" 'class="replan-gantt__row replan-gantt__load-row"' 'class="replan-gantt__week-fill"' \
  'class="replan-gantt__waterline"' \
  'data-replan-jump="load"' 'replan-gantt__week-hump -over' "still overloaded:" "no overload remains"; do
  expect_text "gantt load profile" "$text"
done
fetch "$member_jar" "$GANTT?load=0"
expect_status "gantt without load" 200
expect_no_text "gantt without load" 'class="replan-gantt__row replan-gantt__load-row"'
expect_no_text "gantt without load" 'data-replan-jump="load"'
fetch "$member_jar" "$GANTT?from=not-a-date&to=2099-01-01"
expect_status "gantt with a bad range" 200

fetch "$member_jar" "$PANEL"
expect_status "work-package panel" 200
for text in "PageHeader" "href=\"$PROJECT_PAGE\"" "#$WORK_PACKAGE" ">What was done<" ">Resources<" ">Time booked<" \
  ">Depends on<" ">Waiting on this<" "replan-gantt__scroll -mini" 'class="replan-gantt__row replan-gantt__axis"' \
  "replan-gantt__bar" 'class="replan-gantt__lane"'; do
  expect_text "work-package panel" "$text"
done
expect_no_text "work-package panel" "No RePlan overlay"
expect_no_fixed_colours "work-package panel"
expect_script_nonce "work-package panel"
expect_text "work-package panel tab" "data-task-wp=\"$WORK_PACKAGE\""
expect_no_text "work-package panel names" ">resource.human.maintainer<"
expect_text "work-package panel escaping" "$hostile_escaped"
expect_no_text "work-package panel escaping" "$HOSTILE_TITLE"

# A task with evidence, booked time and a predecessor shows all three, the
# predecessor as a link to its own task page.
fetch "$member_jar" "$PROJECT_PAGE/work_packages/$DETAILED_WORK_PACKAGE"
expect_status "detailed task page" 200
for text in 'class="replan-panel__when"' 'class="replan-panel__table"' "$PROJECT_PAGE/work_packages/" ">after"; do
  expect_text "detailed task page" "$text"
done
expect_no_text "detailed task page" "Nothing recorded yet."
expect_no_text "detailed task page" "No time booked yet."
expect_no_fixed_colours "detailed task page"

# A project sees only its own plan. The member belongs to a second project
# with the module enabled; the overlay on the server belongs to the first.
OTHER_PAGE="/projects/$OTHER_PROJECT/replan_overlay"
first_title="$(python3 -c 'import json, sys; rows = json.load(open(sys.argv[1]))["work_packages"]; print(rows[-1]["title"])' "$VALID_OVERLAY")"
for page in "$OTHER_PAGE" "$OTHER_PAGE/gantt" "$OTHER_PAGE/work_packages/$WORK_PACKAGE" "$OTHER_PAGE/work_packages/$DETAILED_WORK_PACKAGE"; do
  fetch "$member_jar" "$page"
  expect_status "other project $page" 200
  expect_text "other project $page" "This project has no RePlan overlay yet."
  expect_no_text "other project $page" "$hostile_escaped"
  expect_no_text "other project $page" "$first_title"
  expect_no_text "other project $page" "openproject_overlay.json"
done

# With an overlay of its own in the overlay directory, the second project
# shows that one, and the first project still shows its own.
mkdir -p "$OVERLAY_DIR/projects/$OTHER_PROJECT"
chmod 755 "$OVERLAY_DIR/projects" "$OVERLAY_DIR/projects/$OTHER_PROJECT"
python3 -c 'import json, sys
overlay = json.load(open(sys.argv[1]))
overlay["openproject_project_id"] = sys.argv[3]
row = next(item for item in overlay["work_packages"] if str(item["work_package_id"]) == sys.argv[4])
row["title"] = "Only in the other project"
overlay["work_packages"] = [row]
json.dump(overlay, open(sys.argv[2], "w"))' "$VALID_OVERLAY" "$OVERLAY_DIR/projects/$OTHER_PROJECT/openproject_overlay.json" "$OTHER_PROJECT" "$WORK_PACKAGE"
chmod 644 "$OVERLAY_DIR/projects/$OTHER_PROJECT/openproject_overlay.json"
fetch "$member_jar" "$OTHER_PAGE"
expect_status "other project with its own overlay" 200
expect_text "other project with its own overlay" "Only in the other project"
expect_no_text "other project with its own overlay" "$hostile_escaped"
fetch "$member_jar" "$GANTT"
expect_text "first project beside the other's overlay" "$hostile_escaped"
expect_no_text "first project beside the other's overlay" "Only in the other project"

# A file put into the wrong project's directory is refused.
cp "$VALID_OVERLAY" "$OVERLAY_DIR/projects/$OTHER_PROJECT/openproject_overlay.json"
fetch "$member_jar" "$OTHER_PAGE"
expect_text "another project's file in this project's directory" "This project has no RePlan overlay yet."
expect_no_text "another project's file in this project's directory" "$hostile_escaped"
rm -rf "$OVERLAY_DIR/projects"

# A work package the overlay does not know answers 404 with an explanation.
fetch "$member_jar" "$UNKNOWN_PANEL"
expect_status "unknown work package" 404
expect_text "unknown work package" "No RePlan overlay for this work package."
expect_no_text "unknown work package has no task tab" "data-task-wp="
expect_text "unknown work package drops a remembered tab" 'data-task-missing="999999999"'

# Without the permission, and without signing in, nothing is shown.
fetch "$outsider_jar" "$PROJECT_PAGE"
expect_status "project page without permission" 403
expect_no_text "project page without permission" "Resource summary"
fetch "$outsider_jar" "$PANEL"
expect_status "work-package panel without permission" 403
fetch "$outsider_jar" "$GANTT"
expect_status "gantt without permission" 403
fetch "$outsider_jar" "/projects/$PROJECT"
expect_status "project overview without permission" 200
expect_no_text "project menu without permission" 'data-name="replan_overlay"'
fetch "$anonymous_jar" "$PROJECT_PAGE"
expect_status "project page signed out" 302
fetch "$anonymous_jar" "$PANEL"
expect_status "work-package panel signed out" 302
fetch "$anonymous_jar" "$GANTT"
expect_status "gantt signed out" 302

# Missing and invalid overlays: both pages still answer, with the reason.
check_unavailable() {
  local case_name=$1 message=$2 path
  for path in "$PROJECT_PAGE" "$GANTT" "$PANEL"; do
    fetch "$member_jar" "$path"
    expect_status "$case_name $path" 200
    expect_text "$case_name $path" "No RePlan overlay for this project."
    expect_text "$case_name $path" "$message"
  done
}

rm -f "$OVERLAY"
check_unavailable "missing overlay" "RePlan overlay file was not found."
printf '{"work_packages": [' >"$OVERLAY"
check_unavailable "invalid JSON overlay" "RePlan overlay JSON could not be parsed"
printf '{"work_packages": {}}' >"$OVERLAY"
check_unavailable "overlay of the wrong shape" "RePlan overlay JSON must contain a work_packages array."
printf '{"schema_version": 99, "work_packages": []}' >"$OVERLAY"
check_unavailable "overlay of an unknown version" "RePlan overlay schema version 99 is not supported by this plugin"
cp "$VALID_OVERLAY" "$OVERLAY"

if [ "$failures" -gt 0 ]; then
  echo "openproject plugin rendered pages: $failures of $checks checks failed" >&2
  exit 1
fi
echo "openproject plugin rendered pages ok: $checks checks"
