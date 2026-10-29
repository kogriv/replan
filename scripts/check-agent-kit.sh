#!/usr/bin/env bash
# Check the RePlan agent kit as an outside agent would meet it: build the
# archive, unpack it outside the repository, and use it with no Go toolchain
# and no settings of this repository. Offline; the kit's work against a real
# OpenProject is checked by scripts/check-openproject-agent-kit-live.sh.
#
# REPLAN_AGENT_KIT_KEEP=<dir> keeps the unpacked kit there for the live check.
# Settings:
#   REPLAN_AGENT_KIT_KEEP         default empty
#   REPLAN_PRIVATE_LEAK_PATTERNS  default private/leak-patterns.txt
#   TMPDIR                        default /tmp
#
# Read only against live project state; temporary or disposable test data may
# be written.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/check-agent-kit.sh
set -euo pipefail

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "check-agent-kit.sh" "$@"
WORK_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-agent-kit-check.XXXXXX")"
trap 'rm -rf "$WORK_DIR"' EXIT INT TERM
checks=0

fail() {
  printf 'agent kit check failed: %s\n' "$1" >&2
  exit 1
}
check() {
  checks=$((checks + 1))
}

ARCHIVE="$(REPLAN_AGENT_KIT_DIR="$WORK_DIR/dist" "$ROOT_DIR/scripts/build-agent-kit.sh")"
check
(cd "$WORK_DIR/dist" && sha256sum -c --quiet "$(basename "$ARCHIVE").sha256") || fail "checksum does not match the archive"

mkdir "$WORK_DIR/unpacked"
tar -C "$WORK_DIR/unpacked" -xzf "$ARCHIVE"
KIT="$(find "$WORK_DIR/unpacked" -mindepth 1 -maxdepth 1 -type d)"

# Nothing private and nothing that needs the repository travels in the kit.
# Generic patterns live here; the names of the operator's own hosts, paths
# and accounts are private too, so they are read from an ignored file, one
# extended regular expression per line (REPLAN_PRIVATE_LEAK_PATTERNS).
leak_patterns='opapi-[0-9a-f]{8}|BEGIN [A-Z ]*PRIVATE KEY'
# The binary must not carry a build machine's home directory.
binary_patterns="$leak_patterns|/home/[a-z]+/"
private_patterns_file="${REPLAN_PRIVATE_LEAK_PATTERNS:-$ROOT_DIR/private/leak-patterns.txt}"
module_owner="$(sed -n 's#^module [^/]*/\([^/]*\)/.*#\1#p' "$ROOT_DIR/go.mod")"
if [ -f "$private_patterns_file" ]; then
  while IFS= read -r pattern; do
    [ -n "$pattern" ] || continue
    leak_patterns="$leak_patterns|$pattern"
    # Every Go binary carries the repository's module path in its symbols.
    [ "$pattern" = "$module_owner" ] || binary_patterns="$binary_patterns|$pattern"
  done <"$private_patterns_file"
fi
check
leaks="$(grep -rIlE "$leak_patterns" "$KIT" || true)"
[ -z "$leaks" ] || fail "private detail in: $leaks"
check
binary_leaks="$(grep -aoE "$binary_patterns" "$KIT/bin/replan" | sort -u | head -n 5 || true)"
[ -z "$binary_leaks" ] || fail "private detail in bin/replan: $binary_leaks"
check
unwanted="$(find "$KIT" \( -name '.env' -o -name '.env.local' -o -name '*.go' -o -name '*.rb' -o -name '__pycache__' -o -path '*/private/*' \) -print)"
[ -z "$unwanted" ] || fail "unwanted files: $unwanted"

# The repository licence and notice travel with the archive; the purpose line
# remains an informational note.
check
cmp -s "$ROOT_DIR/LICENSE" "$KIT/LICENSE" || fail "the kit does not carry the repository LICENSE"
grep -q 'Apache License, Version 2.0' "$KIT/NOTICE.md" 2>/dev/null || fail "the kit notice does not name Apache-2.0"
grep -q '^Provided for: [^@]' "$KIT/NOTICE.md" 2>/dev/null || fail "the kit notice has no purpose note"

# Every module and helper a kit script calls is in the kit.
check
grep -rhoE 'python3 -m [a-z_]+' "$KIT/scripts" | awk '{print $3}' | sort -u > "$WORK_DIR/modules"
while IFS= read -r module; do
  [ -f "$KIT/scripts/lib/$module.py" ] || fail "scripts call python module $module, which the kit lacks"
done < "$WORK_DIR/modules"
# Every repository helper module a kit module imports is in the kit too.
check
missing_imports="$(PYTHONPATH="$ROOT_DIR/scripts/lib" python3 -m kit_module_imports "$KIT/scripts/lib" "$ROOT_DIR/scripts/lib")"
[ -z "$missing_imports" ] || fail "kit modules import helpers the kit lacks: $missing_imports"
check
grep -rhoE 'scripts/[a-z-]+\.sh' "$KIT/scripts" "$KIT/README.md" | sort -u > "$WORK_DIR/helpers"
while IFS= read -r helper; do
  [ -x "$KIT/$helper" ] || fail "$helper is named in the kit but not in it"
done < "$WORK_DIR/helpers"
check
for script in "$KIT"/scripts/*.sh; do
  sh -n "$script" 2>/dev/null || bash -n "$script" || fail "syntax: $script"
done
python3 -m py_compile "$KIT"/scripts/lib/*.py || fail "a python module does not compile"
find "$KIT" -name '__pycache__' -prune -exec rm -rf {} +

# From here on: an empty environment, the kit's own directory, and a `go`
# that fails, so a helper that tried to build from source would be caught.
mkdir "$WORK_DIR/nogo"
printf '#!/bin/sh\necho "go must not be needed by the agent kit" >&2\nexit 97\n' > "$WORK_DIR/nogo/go"
chmod +x "$WORK_DIR/nogo/go"
kit() {
  (cd "$KIT" && env -i PATH="$WORK_DIR/nogo:/usr/bin:/bin" HOME="$WORK_DIR" TMPDIR="${TMPDIR:-/tmp}" "$@")
}
json_true() {
  python3 -c 'import json, sys; d = json.load(open(sys.argv[1])); sys.exit(0 if eval(sys.argv[2]) else 1)' "$1" "$2"
}

# The shipped overlay helpers expose help without settings and refuse extra
# arguments with the shared usage envelope.
for helper in build-openproject-live-overlay publish-openproject-overlay; do
  check
  "$KIT/scripts/$helper.sh" --help > "$WORK_DIR/$helper.help" \
    || fail "$helper --help failed"
  [ -s "$WORK_DIR/$helper.help" ] || fail "$helper --help was empty"
  check
  rc=0
  "$KIT/scripts/$helper.sh" unexpected > "$WORK_DIR/$helper-args.json" 2> "$WORK_DIR/$helper-args.err" || rc=$?
  [ "$rc" = 8 ] || fail "$helper accepted an extra argument or exited $rc"
  json_true "$WORK_DIR/$helper-args.json" "not d['ok'] and d['mode'] == 'invalid' and d['error_codes'] == ['usage_error']" \
    || fail "$helper did not return the usage-error envelope"
  [ ! -s "$WORK_DIR/$helper-args.err" ] || fail "$helper printed its usage error to stderr"
done

check
kit bin/replan validate "$KIT/examples/small-team" --format json > "$WORK_DIR/validate.json" || fail "replan validate failed on the sample plan"
json_true "$WORK_DIR/validate.json" "d['ok'] and not d.get('errors')" || fail "sample plan has errors"

check
kit env REPLAN_PROJECT="$KIT/examples/small-team" REPLAN_OPENPROJECT_PROJECT_ID=sample \
  scripts/plan-openproject-bootstrap.sh > "$WORK_DIR/plan.json" || fail "bootstrap plan failed"
json_true "$WORK_DIR/plan.json" "d['ok'] and d['counts']['planned_work_packages'] == 2" || fail "bootstrap plan is not the sample's two work packages"

# The schedule drift check takes its weekly rows from the kit's binary; the
# kit has no Go to build one.
# The sample's tasks are undated, so a dated copy gives it weeks to ask for.
check
cp -R "$KIT/examples/small-team" "$WORK_DIR/dated-team"
python3 -c 'import sys, yaml
path = sys.argv[1] + "/canonical/tasks.yaml"
doc = yaml.safe_load(open(path))
for task in doc["tasks"]:
    task["planned_start"], task["planned_finish"] = "2026-07-20", "2026-07-24"
yaml.safe_dump(doc, open(path, "w"))' "$WORK_DIR/dated-team"
kit env REPLAN_PROJECT="$WORK_DIR/dated-team" scripts/check-resource-schedule-drift.sh > "$WORK_DIR/schedule.json" \
  || fail "schedule drift check failed in the kit"
json_true "$WORK_DIR/schedule.json" "d['ok'] and not d['errors'] and any(r['week'] == '2026-W30' for r in d['weekly_rows'])" \
  || fail "schedule drift check did not read the workload from the kit binary"

# Without settings a helper names what is missing instead of guessing a
# project (exit 8); with a plan and a project but no token it is skipped with
# a warning.
check
rc=0
kit scripts/openproject-work-package-toolkit.sh work-package list > "$WORK_DIR/list.json" || rc=$?
[ "$rc" = 8 ] || fail "toolkit without settings did not exit 8 but $rc"
json_true "$WORK_DIR/list.json" "not d['ok'] and d['error_codes'] == ['missing_setting'] and set(d['missing_settings']) == {'REPLAN_PROJECT', 'REPLAN_OPENPROJECT_PROJECT_ID'}" \
  || fail "toolkit without settings did not name the missing settings"
check
kit env REPLAN_PROJECT="$KIT/examples/small-team" REPLAN_OPENPROJECT_PROJECT_ID=sample \
  scripts/openproject-work-package-toolkit.sh work-package list > "$WORK_DIR/list.json" || fail "toolkit failed without a token"
json_true "$WORK_DIR/list.json" "d['ok'] and 'token_env_not_set' in d['warning_codes'] and d['counts']['work_packages'] == 0" \
  || fail "toolkit without a token did not warn token_env_not_set"
check
kit scripts/openproject-work-package-toolkit.sh work-package update-text --help > "$WORK_DIR/update-text-help.txt" \
  || fail "toolkit update-text help failed"
grep -q -- '--lock-version' "$WORK_DIR/update-text-help.txt" \
  || fail "toolkit update-text help does not expose lock-version"

# With the template filled in, the settings file alone configures every
# helper, with a writable plan like an outside agent would use.
SETTINGS_PROJECT="$WORK_DIR/settings-team"
cp -R "$KIT/examples/small-team" "$SETTINGS_PROJECT"
sed -e "s#^REPLAN_OPENPROJECT_BASE_URL=.*#REPLAN_OPENPROJECT_BASE_URL=http://127.0.0.1:9#" \
    -e "s#^REPLAN_PROJECT=.*#REPLAN_PROJECT=$SETTINGS_PROJECT#" \
    -e "s#^REPLAN_OPENPROJECT_BINDINGS_PATH=.*#REPLAN_OPENPROJECT_BINDINGS_PATH=$WORK_DIR/bindings.yaml#" \
    -e "s#^REPLAN_OVERLAY_OUTPUT_DIR=.*#REPLAN_OVERLAY_OUTPUT_DIR=$WORK_DIR/settings-overlay#" \
    -e "s#^REPLAN_OVERLAY_PUBLISH_TARGET=.*#REPLAN_OVERLAY_PUBLISH_TARGET=$WORK_DIR/settings-publish#" \
    -e "s#^OPENPROJECT_TOKEN=.*#OPENPROJECT_TOKEN=not-a-real-token#" \
    "$KIT/env.local.example" > "$KIT/.env.local"
helper_envelope_ok() {
  json_true "$1" "isinstance(d, dict) and {'ok', 'command', 'mode', 'warnings', 'errors', 'warning_codes', 'error_codes'}.issubset(d) and all(isinstance(d[k], list) for k in ('warnings', 'errors', 'warning_codes', 'error_codes'))"
}

# The archive is the inventory: a shipped helper must match a case below.
set -- "$KIT"/scripts/*.sh
[ -e "$1" ] || fail "the kit has no scripts"
for script_path in "$KIT"/scripts/*.sh; do
  script="$(basename "$script_path")"
  out="$WORK_DIR/settings-$script.out"
  err="$WORK_DIR/settings-$script.err"
  rc=0
  case "$script" in
    accept-task.sh)
      expected=11
      # One acceptance is described in the process environment, never in
      # the settings file.
      set -- env REPLAN_TASK=task.op.1234 REPLAN_ACCEPTED_BY=actor.human.operator \
        REPLAN_ACCEPT_SUMMARY="Kit settings-file acceptance check." "scripts/$script"
      ;;
    accept-openproject-soft-drift.sh) expected=6; set -- "scripts/$script" ;;
    apply-openproject-bootstrap.sh) expected=6; set -- "scripts/$script" ;;
    apply-openproject-relations.sh) expected=4; set -- "scripts/$script" ;;
    build-openproject-live-overlay.sh) expected=11; set -- "scripts/$script" ;;
    check-openproject-drift.sh) expected=6; set -- "scripts/$script" ;;
    check-openproject-live-alignment.sh) expected=6; set -- "scripts/$script" ;;
    check-resource-schedule-drift.sh) expected=11; set -- "scripts/$script" ;;
    explain-resource-overload.sh)
      expected=0
      # The resource and unit are the question this explanation answers.
      set -- env REPLAN_RESOURCE_ID=resource.human.ivan REPLAN_UNIT=hours "scripts/$script"
      ;;
    openproject-operator-report.sh) expected=6; set -- "scripts/$script" ;;
    openproject-wiki.sh) expected=6; set -- "scripts/$script" page list ;;
    openproject-work-package-toolkit.sh) expected=6; set -- "scripts/$script" work-package list ;;
    plan-openproject-bootstrap.sh) expected=0; set -- "scripts/$script" ;;
    plan-resource-demand-calibration.sh) expected=0; set -- "scripts/$script" ;;
    publish-openproject-overlay.sh) expected=4; set -- "scripts/$script" ;;
    push-openproject-date-drift.sh) expected=6; set -- "scripts/$script" ;;
    push-openproject-status-drift.sh) expected=6; set -- "scripts/$script" ;;
    review-resource-pool.sh) expected=0; set -- "scripts/$script" ;;
    validate-openproject-live.sh) expected=6; set -- "scripts/$script" ;;
    *) fail "$script has no settings-file-only invocation" ;;
  esac
  check
  kit "$@" >"$out" 2>"$err" || rc=$?
  [ "$rc" = "$expected" ] || fail "$script exited $rc, expected $expected: $(head -c 300 "$out" "$err")"
  helper_envelope_ok "$out" || fail "$script stdout is not one helper envelope: $(head -c 300 "$out" "$err")"
  json_true "$out" "'missing_setting' not in d['error_codes']" \
    || fail "$script reported missing_setting with the kit settings template"
  json_true "$out" "'project_path' not in d or d['project_path'] == '$SETTINGS_PROJECT'" \
    || fail "$script did not use REPLAN_PROJECT from the kit settings file"
  if grep -q 'replan-self' "$out" "$err"; then
    fail "$script named this repository's own plan or project"
  fi
  if grep -q 'Traceback (most recent call last)' "$err"; then
    fail "$script printed a Python traceback"
  fi
  if grep -q 'go must not be needed by the agent kit' "$out" "$err"; then
    fail "$script tried to use go"
  fi
done

# A helper that cannot reach the server says so in its JSON, never with a
# stack trace, and a download that fails leaves no file behind.
check
rc=0
kit env REPLAN_OPENPROJECT_TIMEOUT_SECONDS=3 scripts/openproject-wiki.sh attachment download --attachment 1 --output "$WORK_DIR/never.bin" \
  > "$WORK_DIR/wiki-down.out" 2> "$WORK_DIR/wiki-down.err" || rc=$?
[ "$rc" = 6 ] || fail "an unreachable server did not exit 6 (external source error) but $rc"
json_true "$WORK_DIR/wiki-down.out" "not d['ok'] and d['error_codes'] == ['openproject_network_error']" \
  || fail "an unreachable server was not reported as JSON: $(head -c 300 "$WORK_DIR/wiki-down.out" "$WORK_DIR/wiki-down.err")"
[ ! -s "$WORK_DIR/wiki-down.err" ] || fail "a helper printed to stderr: $(head -c 300 "$WORK_DIR/wiki-down.err")"
[ ! -e "$WORK_DIR/never.bin" ] || fail "a failed download left a file behind"
check
kit env REPLAN_OPENPROJECT_TIMEOUT_SECONDS=3 scripts/openproject-wiki.sh page list > "$WORK_DIR/wiki-list.out" 2> "$WORK_DIR/wiki-list.err" || true
json_true "$WORK_DIR/wiki-list.out" "not d['ok'] and d['error_codes'] == ['openproject_network_error']" \
  || fail "page list against an unreachable server was not reported as JSON"
rm -f "$KIT/.env.local"

# Build and publish entirely from the unpacked kit. This uses the kit binary
# selected by load-dotenv.sh and the bindings/output defaults under kit root.
mkdir -p "$KIT/private/openproject"
python3 -c 'import sys, yaml
yaml.safe_dump({"openproject_private_bindings": {
    "project": {"source": "openproject", "project_id": "demo"},
    "tasks": [
        {"replan_task_id": "task.offline.repair-pump", "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "101"}},
        {"replan_task_id": "task.op.1234", "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "102"}},
    ],
    "dependencies": [], "resources": [],
}}, open(sys.argv[1], "w"), sort_keys=False)' "$KIT/private/openproject/demo-bindings.yaml"
check
kit env REPLAN_PROJECT="$KIT/examples/small-team" REPLAN_OPENPROJECT_PROJECT_ID=demo \
  scripts/build-openproject-live-overlay.sh > "$WORK_DIR/overlay-build.json" \
  || fail "overlay build failed in the unpacked kit"
helper_envelope_ok "$WORK_DIR/overlay-build.json" || fail "overlay build did not return the helper envelope"
json_true "$WORK_DIR/overlay-build.json" "d['ok'] and d['counts']['tasks_bound'] == 2 and d['counts']['missing_link_count'] == 0" \
  || fail "overlay build did not use the sample bindings"
[ -f "$KIT/tmp/openproject-live-overlay/latest/raw/openproject_overlay.json" ] \
  || fail "overlay build did not use its kit-root output default"

OVERLAY_TARGET="$WORK_DIR/published-overlay"
check
kit env REPLAN_OPENPROJECT_PROJECT_ID=demo REPLAN_OVERLAY_PUBLISH_TARGET="$OVERLAY_TARGET" \
  scripts/publish-openproject-overlay.sh > "$WORK_DIR/overlay-dry-run.json" \
  || fail "overlay publish dry run failed"
json_true "$WORK_DIR/overlay-dry-run.json" "d['ok'] and d['mode'] == 'dry_run' and d['destination'] == '<target>/demo/'" \
  || fail "overlay publish dry run did not describe the project destination"
[ ! -e "$OVERLAY_TARGET" ] || fail "overlay publish dry run wrote to its target"

check
kit env REPLAN_OPENPROJECT_PROJECT_ID=demo REPLAN_OVERLAY_PUBLISH_TARGET="$OVERLAY_TARGET" REPLAN_APPLY=1 \
  scripts/publish-openproject-overlay.sh > "$WORK_DIR/overlay-apply.json" \
  || fail "overlay publish apply failed"
json_true "$WORK_DIR/overlay-apply.json" "d['ok'] and d['mode'] == 'apply'" \
  || fail "overlay publish apply did not return an apply envelope"
[ -f "$OVERLAY_TARGET/demo/raw/openproject_overlay.json" ] \
  || fail "overlay did not land below the selected project id"
[ "$(find "$OVERLAY_TARGET" -mindepth 1 -maxdepth 1 -printf '%f\n')" = demo ] \
  || fail "overlay publish wrote outside the selected project directory"

mkdir "$WORK_DIR/path-without-rsync"
for command_name in sh dirname sed printenv python3; do
  ln -s "$(command -v "$command_name")" "$WORK_DIR/path-without-rsync/$command_name"
done
check
rc=0
(cd "$KIT" && env -i PATH="$WORK_DIR/path-without-rsync" HOME="$WORK_DIR" \
  REPLAN_OPENPROJECT_PROJECT_ID=demo REPLAN_OVERLAY_PUBLISH_TARGET="$OVERLAY_TARGET" \
  scripts/publish-openproject-overlay.sh) > "$WORK_DIR/overlay-no-rsync.json" 2> "$WORK_DIR/overlay-no-rsync.err" || rc=$?
[ "$rc" = 4 ] || fail "publish without rsync exited $rc instead of 4"
json_true "$WORK_DIR/overlay-no-rsync.json" "not d['ok'] and d['error_codes'] == ['rsync_not_found']" \
  || fail "publish without rsync did not return rsync_not_found"
[ ! -s "$WORK_DIR/overlay-no-rsync.err" ] || fail "publish without rsync printed a shell error"

python3 -c 'import json, sys
path = sys.argv[1]
doc = json.load(open(path))
doc["openproject_project_id"] = "another-project"
json.dump(doc, open(path, "w"))' "$KIT/tmp/openproject-live-overlay/latest/raw/openproject_overlay.json"
check
rc=0
kit env REPLAN_OPENPROJECT_PROJECT_ID=demo REPLAN_OVERLAY_PUBLISH_TARGET="$OVERLAY_TARGET" \
  scripts/publish-openproject-overlay.sh > "$WORK_DIR/overlay-other-project.json" || rc=$?
[ "$rc" = 2 ] || fail "another project's overlay was not refused with exit 2"
json_true "$WORK_DIR/overlay-other-project.json" "not d['ok'] and d['error_codes'] == ['overlay_other_project']" \
  || fail "another project's overlay was not refused with overlay_other_project"
[ "$(find "$OVERLAY_TARGET" -mindepth 1 -maxdepth 1 -printf '%f\n')" = demo ] \
  || fail "wrong-project refusal wrote another target directory"

python3 -c 'import json, sys
path = sys.argv[1]
doc = json.load(open(path))
doc["openproject_project_id"] = "demo"
doc["scenario_id"] = "scenario.what-if"
json.dump(doc, open(path, "w"))' "$KIT/tmp/openproject-live-overlay/latest/raw/openproject_overlay.json"
check
rc=0
kit env REPLAN_OPENPROJECT_PROJECT_ID=demo REPLAN_OVERLAY_PUBLISH_TARGET="$OVERLAY_TARGET" \
  scripts/publish-openproject-overlay.sh > "$WORK_DIR/overlay-is-scenario.json" || rc=$?
[ "$rc" = 2 ] || fail "scenario overlay publish exited $rc instead of 2"
json_true "$WORK_DIR/overlay-is-scenario.json" "not d['ok'] and d['error_codes'] == ['overlay_is_scenario']" \
  || fail "scenario overlay was not refused with overlay_is_scenario"

# A reverse proxy in front of OpenProject may let API clients through only
# with a perimeter key. Against a stand-in that records what it is sent, both
# the Python helpers and the replan binary carry the key from the settings
# file beside the client's own token, and neither prints it.
PYTHONPATH="$ROOT_DIR/scripts/lib" python3 -m perimeter_probe_server "$WORK_DIR/probe.log" "$WORK_DIR/probe.port" &
probe_pid=$!
trap 'kill "$probe_pid" 2>/dev/null || true; rm -rf "$WORK_DIR"' EXIT INT TERM
for _ in 1 2 3 4 5 6 7 8 9 10; do
  [ -s "$WORK_DIR/probe.port" ] && break
  sleep 0.3
done
[ -s "$WORK_DIR/probe.port" ] || fail "the perimeter stand-in did not start"
sed -e "s#^REPLAN_OPENPROJECT_BASE_URL=.*#REPLAN_OPENPROJECT_BASE_URL=http://127.0.0.1:$(cat "$WORK_DIR/probe.port")#" \
    -e "s#^REPLAN_PROJECT=.*#REPLAN_PROJECT=$KIT/examples/small-team#" \
    -e "s#^REPLAN_OPENPROJECT_BINDINGS_PATH=.*#REPLAN_OPENPROJECT_BINDINGS_PATH=$WORK_DIR/bindings.yaml#" \
    -e "s#^OPENPROJECT_TOKEN=.*#OPENPROJECT_TOKEN=not-a-real-token#" \
    -e "s|^#REPLAN_OPENPROJECT_PERIMETER_KEY=.*|REPLAN_OPENPROJECT_PERIMETER_KEY=kit-perimeter-check|" \
    "$KIT/env.local.example" > "$KIT/.env.local"
grep -q '^REPLAN_OPENPROJECT_PERIMETER_KEY=kit-perimeter-check$' "$KIT/.env.local" || fail "the settings template has no perimeter key line"
kit scripts/openproject-work-package-toolkit.sh work-package list > "$WORK_DIR/perimeter-python.out" 2>&1 || true
kit scripts/check-openproject-drift.sh > "$WORK_DIR/perimeter-go.out" 2>&1 || true
kill "$probe_pid" 2>/dev/null || true
check
grep -q '^agent=Python-urllib key=kit-perimeter-check authorization=yes ' "$WORK_DIR/probe.log" \
  || fail "a Python helper did not send the perimeter key: $(cat "$WORK_DIR/probe.log" 2>/dev/null)"
check
grep -q '^agent=Go-http-client key=kit-perimeter-check authorization=yes ' "$WORK_DIR/probe.log" \
  || fail "the replan binary did not send the perimeter key: $(cat "$WORK_DIR/probe.log" 2>/dev/null)"
check
! grep -q 'kit-perimeter-check' "$WORK_DIR/perimeter-python.out" "$WORK_DIR/perimeter-go.out" \
  || fail "a helper printed the perimeter key"
rm -f "$KIT/.env.local"

if [ -n "${REPLAN_AGENT_KIT_KEEP:-}" ]; then
  rm -rf "$REPLAN_AGENT_KIT_KEEP"
  mkdir -p "$(dirname "$REPLAN_AGENT_KIT_KEEP")"
  cp -R "$KIT" "$REPLAN_AGENT_KIT_KEEP"
fi

printf 'agent kit ok: %s checks, %s\n' "$checks" "$(basename "$ARCHIVE")"
