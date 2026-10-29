#!/usr/bin/env sh
# Run the local MVP acceptance smoke across CLI, helpers and fixture workflows.
#
# Settings:
#   REPLAN_PROJECT             default examples/small-team
#   REPLAN_OPENPROJECT_SAMPLE  default examples/imports/openproject/work-packages.sample.json
#   REPLAN_WEEK                default 2026-W28
#   REPLAN_ACTOR               default actor.human.operator
#   REPLAN_BIN                 default build locally
#   TMPDIR                     default /tmp
#
# Does not write a live OpenProject; it writes only local or disposable
# artifacts.
# Prints developer-oriented text.
# Exits 0 on success, 8 for extra arguments, and 1 for its own failed checks;
# delegated build or runtime commands may return their own nonzero exit.
#
#   scripts/smoke-mvp.sh
#   REPLAN_BIN=./replan scripts/smoke-mvp.sh
set -eu

ROOT_DIR="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
. "$ROOT_DIR/scripts/lib/envelope.sh"
replan_help "$0" "$@"
replan_require_no_args_text "smoke-mvp.sh" "$@"
PROJECT_PATH="${REPLAN_PROJECT:-$ROOT_DIR/examples/small-team}"
OPENPROJECT_SAMPLE="${REPLAN_OPENPROJECT_SAMPLE:-$ROOT_DIR/examples/imports/openproject/work-packages.sample.json}"
OPENPROJECT_SELF_SAMPLE="$ROOT_DIR/examples/imports/openproject/replan-self.work-packages.sample.json"
OPENPROJECT_SELF_RESOURCE_DEMANDS_SIDECAR="$ROOT_DIR/examples/openproject/sidecars/resource-demands.replan-self.yaml"
OPENPROJECT_SELF_BINDINGS="$ROOT_DIR/examples/openproject/private-bindings.replan-self.example.yaml"
WEEK="${REPLAN_WEEK:-2026-W28}"
ACTOR="${REPLAN_ACTOR:-actor.human.operator}"

TMP_DIR="$(mktemp -d "${TMPDIR:-/tmp}/replan-mvp-smoke.XXXXXX")"
cleanup() {
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT INT TERM

BIN="${REPLAN_BIN:-}"
if [ -z "$BIN" ]; then
  BIN="$TMP_DIR/replan"
  (cd "$ROOT_DIR" && go build -o "$BIN" ./cmd/replan)
fi

REPLAN_BIN="$BIN" PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -W error::ResourceWarning -m unittest discover -s "$ROOT_DIR/scripts/tests" >"$TMP_DIR/python-unit.txt" 2>&1 || {
  cat "$TMP_DIR/python-unit.txt" >&2
  exit 1
}
"$ROOT_DIR/scripts/check-openproject-plugin-scaffold.sh" >"$TMP_DIR/openproject-plugin-scaffold.txt"
grep 'openproject plugin scaffold ok' "$TMP_DIR/openproject-plugin-scaffold.txt" >/dev/null

assert_contains() {
  file="$1"
  pattern="$2"
  if ! grep "$pattern" "$file" >/dev/null; then
    echo "missing pattern '$pattern' in $file" >&2
    sed -n '1,160p' "$file" >&2
    exit 1
  fi
}

assert_project_audit_contains() {
  project="$1"
  pattern="$2"
  if ! grep "$pattern" "$project"/audit/*.jsonl >/dev/null; then
    echo "missing audit pattern '$pattern' in $project/audit/*.jsonl" >&2
    sed -n '1,160p' "$project"/audit/*.jsonl >&2
    exit 1
  fi
}

make_self_tracking_suggest_stale() {
  project="$1"
  rm -f "$project"/proposals/open/proposal.tracking.resource_demand-tracking-suggest-*.yaml
  rm -f "$project"/proposals/accepted/proposal.tracking.resource_demand-tracking-suggest-*.yaml
  rm -f "$project"/proposals/rejected/proposal.tracking.resource_demand-tracking-suggest-*.yaml
  perl -0pi -e 's/(id: resource_demand\.tracking-suggest\.maintainer\n      task_ref:\n        kind: task\n        id: task\.self\.add-tracking-suggest-command\n      resource_ref:\n        kind: resource\n        id: resource\.human\.maintainer\n      resource_class: human\n      role: implementer\n      unit: hours\n      planned_amount: 3\n)(      criticality: required)/$1      remaining_amount: 3\n$2/' "$project/canonical/resource_demands.yaml"
}

run_json_ok() {
  name="$1"
  shift
  output="$TMP_DIR/$name.json"
  "$@" >"$output"
  assert_contains "$output" '"ok": true'
}

set_openproject_ids_canonical() {
  project="$1"
  # These disposable copies test import machinery, not the private-id policy.
  sed -i 's/^  openproject_ids: private_bindings$/  openproject_ids: canonical/' "$project/.replan/project.yaml"
}

assert_openproject_dotenv_loader() {
  dotenv_root="$TMP_DIR/dotenv-root"
  mkdir -p "$dotenv_root/private/openproject" "$dotenv_root/scripts/lib"
  cp "$ROOT_DIR/scripts/lib/settings.list" "$dotenv_root/scripts/lib/settings.list"
  {
    printf '%s\n' 'REPLAN_OPENPROJECT_BASE_URL=https://env.example.internal'
    printf '%s\n' 'REPLAN_OPENPROJECT_PROJECT_ID=env-project'
    printf '%s\n' 'REPLAN_OPENPROJECT_HOST_HEADER=env-host.example.internal'
    printf '%s\n' 'REPLAN_OPENPROJECT_FORWARDED_PROTO=http'
    printf '%s\n' 'OPENPROJECT_TOKEN=env-token'
    printf '%s\n' 'REPLAN_APPLY=1'
  } >"$dotenv_root/.env"
  {
    printf '%s\n' 'REPLAN_OPENPROJECT_BASE_URL=https://local.example.internal'
    printf '%s\n' 'REPLAN_OPENPROJECT_FORWARDED_PROTO=https'
    printf '%s\n' 'OPENPROJECT_TOKEN=local-token'
    printf '%s\n' 'REPLAN_OPENPROJECT_BINDINGS_PATH=private/openproject/local-bindings.yaml'
  } >"$dotenv_root/.env.local"

  # shellcheck disable=SC2016 # expanded by the child shell, on purpose.
  env -i sh -c '. "$1/scripts/lib/load-dotenv.sh"; replan_load_openproject_dotenv "$2"; printf "%s|%s|%s|%s|%s|%s|%s\n" "$REPLAN_OPENPROJECT_BASE_URL" "$REPLAN_OPENPROJECT_PROJECT_ID" "$OPENPROJECT_TOKEN" "${REPLAN_APPLY-unset}" "$REPLAN_OPENPROJECT_BINDINGS_PATH" "$REPLAN_OPENPROJECT_HOST_HEADER" "$REPLAN_OPENPROJECT_FORWARDED_PROTO"' sh "$ROOT_DIR" "$dotenv_root" >"$TMP_DIR/dotenv-loader.txt"
  assert_contains "$TMP_DIR/dotenv-loader.txt" 'https://local.example.internal|env-project|local-token|unset|private/openproject/local-bindings.yaml|env-host.example.internal|https'

  # shellcheck disable=SC2016 # expanded by the child shell, on purpose.
  env -i REPLAN_OPENPROJECT_BASE_URL=https://explicit.example.internal sh -c '. "$1/scripts/lib/load-dotenv.sh"; replan_load_openproject_dotenv "$2"; printf "%s\n" "$REPLAN_OPENPROJECT_BASE_URL"' sh "$ROOT_DIR" "$dotenv_root" >"$TMP_DIR/dotenv-loader-explicit.txt"
  assert_contains "$TMP_DIR/dotenv-loader-explicit.txt" 'https://explicit.example.internal'
}

copy_project() {
  target="$1"
  cp -a "$PROJECT_PATH" "$target"
}

assert_openproject_dotenv_loader

run_json_ok validate "$BIN" validate "$PROJECT_PATH" --format json
CALIBRATION_PROJECT="$TMP_DIR/estimate-calibration"
cp -a "$ROOT_DIR/examples/estimate-calibration" "$CALIBRATION_PROJECT"
run_json_ok estimate-calibrate-dry-run "$BIN" estimate calibrate \
  --project "$CALIBRATION_PROJECT" \
  --baseline baseline.fixture.original \
  --resource resource.ai.executor \
  --resource resource.ai.codex \
  --unit agent_hours \
  --tag calibration \
  --exclude task.cal.09=actuals_judged \
  --id calibration.fixture.agents \
  --name "Fixture agent calibration" \
  --approved-by actor.human.operator \
  --occurred-at 2026-10-12T09:00:00Z \
  --dry-run \
  --format json
assert_contains "$TMP_DIR/estimate-calibrate-dry-run.json" '"count": 8'
run_json_ok estimate-calibrate-apply "$BIN" estimate calibrate \
  --project "$CALIBRATION_PROJECT" \
  --baseline baseline.fixture.original \
  --resource resource.ai.executor \
  --resource resource.ai.codex \
  --unit agent_hours \
  --tag calibration \
  --exclude task.cal.09=actuals_judged \
  --id calibration.fixture.agents \
  --name "Fixture agent calibration" \
  --approved-by actor.human.operator \
  --occurred-at 2026-10-12T09:00:00Z \
  --apply \
  --format json
run_json_ok estimate-list "$BIN" estimate list --project "$CALIBRATION_PROJECT" --format json
assert_contains "$TMP_DIR/estimate-list.json" '"calibration.fixture.agents"'
run_json_ok estimate-show "$BIN" estimate show --project "$CALIBRATION_PROJECT" calibration.fixture.agents --format json
assert_contains "$TMP_DIR/estimate-show.json" '"seal_status": "verified"'
run_json_ok estimate-apply-dry-run "$BIN" estimate apply \
  --project "$CALIBRATION_PROJECT" \
  --calibration calibration.fixture.agents \
  --scenario-id scenario.fixture.calibrated \
  --actor actor.human.operator \
  --occurred-at 2026-10-12T10:00:00Z \
  --dry-run \
  --format json
assert_contains "$TMP_DIR/estimate-apply-dry-run.json" '"calibration_ref"'
run_json_ok workload "$BIN" workload --project "$PROJECT_PATH" --week "$WEEK" --format json
run_json_ok workload-human-hours "$BIN" workload --project "$PROJECT_PATH" --week "$WEEK" --resource-class human --unit hours --format json
assert_contains "$TMP_DIR/workload-human-hours.json" '"resource.human.ivan"'
if grep -q '"agent_hours"' "$TMP_DIR/workload-human-hours.json"; then
  echo "workload --resource-class human --unit hours kept an agent_hours row" >&2
  exit 1
fi
assert_contains "$TMP_DIR/workload.json" '"overload"'

SELF_PROJECT="$ROOT_DIR/examples/replan-self"
# The helpers have no default project; the smoke names its own.
export REPLAN_PROJECT="$SELF_PROJECT"
export REPLAN_OPENPROJECT_PROJECT_ID=replan-self
run_json_ok self-validate "$BIN" validate "$SELF_PROJECT" --format json
run_json_ok self-workload "$BIN" workload --project "$SELF_PROJECT" --week 2026-W31 --format json
assert_contains "$TMP_DIR/self-workload.json" '"resource.human.maintainer"'
REPLAN_PROJECT="$SELF_PROJECT" "$ROOT_DIR/scripts/plan-resource-demand-calibration.sh" >"$TMP_DIR/self-resource-demand-calibration-plan.json"
assert_contains "$TMP_DIR/self-resource-demand-calibration-plan.json" '"ok": true'
assert_contains "$TMP_DIR/self-resource-demand-calibration-plan.json" '"calibration_plan_status": "aligned"'
assert_contains "$TMP_DIR/self-resource-demand-calibration-plan.json" '"candidate_actions": 0'
REPLAN_PROJECT="$SELF_PROJECT" REPLAN_RESOURCE_ID=resource.role.reviewer REPLAN_UNIT=review_hours \
  "$ROOT_DIR/scripts/explain-resource-overload.sh" >"$TMP_DIR/self-reviewer-overload.json"
assert_contains "$TMP_DIR/self-reviewer-overload.json" '"ok": true'
assert_contains "$TMP_DIR/self-reviewer-overload.json" '"overload_status": "aligned"'
assert_contains "$TMP_DIR/self-reviewer-overload.json" '"overloaded_weekly_rows": 0'
# The self project's reviewer demands close as tasks are accepted, so the
# demand listing is checked on the small-team fixture, whose demands stay open.
REPLAN_PROJECT="$ROOT_DIR/examples/small-team" REPLAN_RESOURCE_ID=resource.human.ivan REPLAN_UNIT=hours \
  "$ROOT_DIR/scripts/explain-resource-overload.sh" >"$TMP_DIR/small-team-ivan-overload.json"
assert_contains "$TMP_DIR/small-team-ivan-overload.json" '"ok": true'
assert_contains "$TMP_DIR/small-team-ivan-overload.json" '"resource_demand.task-op-1234.implementer"'
"$BIN" report weekly --project "$SELF_PROJECT" --week 2026-W31 --format markdown >"$TMP_DIR/self-report.md"
assert_contains "$TMP_DIR/self-report.md" 'RePlan self-dogfood'
run_json_ok self-state "$BIN" report state --project "$SELF_PROJECT" --week 2026-W31 --format json
"$BIN" report state --project "$SELF_PROJECT" --week 2026-W31 --format markdown >"$TMP_DIR/self-state.md"
assert_contains "$TMP_DIR/self-state.md" 'RePlan Project State'
assert_contains "$TMP_DIR/self-state.md" 'Sync Health'
assert_contains "$TMP_DIR/self-state.md" 'Planning Health'
assert_contains "$TMP_DIR/self-state.md" 'Plan vs Actual'
assert_contains "$TMP_DIR/self-state.md" 'Forecast'
assert_contains "$TMP_DIR/self-state.md" 'Resource Pressure'
assert_contains "$TMP_DIR/self-state.json" '"stale_remaining_count": 0'
# Derive each self-plan count once from the report model or from the committed
# fixture/binding file that owns it. Keep mapped counts as intersections: they
# are not necessarily the same as task, source-ref, work-package or binding
# counts.
self_plan_counts="$(PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -c '
import json, sys, yaml
import openproject_bindings
state = json.load(open(sys.argv[1], encoding="utf-8"))["result"]
sample = json.load(open(sys.argv[2], encoding="utf-8"))
work_packages = (sample.get("_embedded") or {}).get("elements") or []
work_package_ids = {str(item.get("id")) for item in work_packages}
task_to_wp, wp_to_task = openproject_bindings.task_maps(openproject_bindings.load(sys.argv[3]))
plan = yaml.safe_load(open(sys.argv[4], encoding="utf-8")) or {}
planned_task_ids = [item.get("id") for item in ((plan.get("current_plan") or {}).get("included_tasks") or []) if item.get("kind") == "task" and item.get("id")]
print(state["health"]["task_count"], state["source_health"]["tasks_with_source_refs"], len(work_packages), len(task_to_wp), sum(1 for item in work_packages if str(item.get("id")) in wp_to_task), sum(1 for task_id in planned_task_ids if task_to_wp.get(task_id) in work_package_ids))
' "$TMP_DIR/self-state.json" "$OPENPROJECT_SELF_SAMPLE" "$OPENPROJECT_SELF_BINDINGS" "$SELF_PROJECT/canonical/current_plan.yaml")"
# shellcheck disable=SC2086 # Six integer fields emitted by the Python count reader.
set -- $self_plan_counts
SELF_TASK_COUNT="$1"
SELF_SOURCE_TASK_COUNT="$2"
SELF_WORK_PACKAGE_COUNT="$3"
SELF_TASK_BINDING_COUNT="$4"
SELF_TOOLKIT_MAPPED_COUNT="$5"
SELF_OPERATOR_MAPPED_COUNT="$6"
run_json_ok self-next "$BIN" next --project "$SELF_PROJECT" --format json
assert_contains "$TMP_DIR/self-next.json" '"waiting_on_person"'
"$BIN" next --project "$SELF_PROJECT" >"$TMP_DIR/self-next.txt"
assert_contains "$TMP_DIR/self-next.txt" 'Waiting on a person ('
assert_contains "$TMP_DIR/self-next.txt" 'An agent can prepare ('
assert_contains "$TMP_DIR/self-next.txt" 'Health: '
run_json_ok self-status-date-dry-run "$BIN" project set-status-date --project "$SELF_PROJECT" --date 2026-12-31 --actor actor.human.maintainer --dry-run --format json
assert_contains "$TMP_DIR/self-status-date-dry-run.json" '"dry_run": true'
assert_contains "$TMP_DIR/self-status-date-dry-run.json" '"week": "2026-W53"'
assert_contains "$TMP_DIR/self-status-date-dry-run.json" '"project_set_status_date"'
run_json_ok self-tracking-suggest "$BIN" tracking suggest --project "$SELF_PROJECT" --week 2026-W31 --format json
"$BIN" tracking suggest --project "$SELF_PROJECT" --week 2026-W31 --format markdown >"$TMP_DIR/self-tracking-suggest.md"
assert_contains "$TMP_DIR/self-tracking-suggest.md" 'RePlan Tracking Suggestions'
# The living self-dogfood project carries honest overruns. Each one was
# reviewed and acknowledged with `replan actual acknowledge-overrun`, so the
# clean state is again an empty suggestion list (GAP-016).
assert_contains "$TMP_DIR/self-tracking-suggest.json" '"suggestions": \[\]'
if grep -q '"type": "review_forecast_over_plan"' "$TMP_DIR/self-tracking-suggest.json"; then
  echo "unacknowledged forecast overrun in the self-dogfood project" >&2
  exit 1
fi
self_tracking_project="$TMP_DIR/self-tracking-project"
cp -a "$SELF_PROJECT" "$self_tracking_project"
make_self_tracking_suggest_stale "$self_tracking_project"
run_json_ok self-tracking-write "$BIN" tracking suggest \
  --project "$self_tracking_project" \
  --week 2026-W31 \
  --write-candidates \
  --actor actor.ai.codex.local \
  --format json
assert_contains "$TMP_DIR/self-tracking-write.json" '"dry_run": false'
assert_contains "$TMP_DIR/self-tracking-write.json" '"applied": true'
tracking_proposal_id="proposal.tracking.resource_demand-tracking-suggest-maintainer.set-remaining-zero"
test -s "$self_tracking_project/proposals/open/$tracking_proposal_id.yaml"
run_json_ok self-tracking-accept-dry-run "$BIN" proposal accept \
  --project "$self_tracking_project" \
  --dry-run \
  --format json \
  "$tracking_proposal_id"
assert_contains "$TMP_DIR/self-tracking-accept-dry-run.json" '"canonical/resource_demands.yaml"'
run_json_ok self-tracking-accept "$BIN" proposal accept \
  --project "$self_tracking_project" \
  --actor actor.human.maintainer \
  --reason "Operator accepted stale remaining-work tracking proposal." \
  --apply \
  --format json \
  "$tracking_proposal_id"
assert_contains "$TMP_DIR/self-tracking-accept.json" '"canonical/resource_demands.yaml"'
test -s "$self_tracking_project/proposals/accepted/$tracking_proposal_id.yaml"
run_json_ok self-tracking-validate-after-accept "$BIN" validate "$self_tracking_project" --format json
# An accepted zero is recorded explicitly, not dropped (GAP-016).
awk '
  /id: resource_demand.tracking-suggest.maintainer/ { found = 1; in_target = 1; next }
  in_target && /^    - id:/ { in_target = 0 }
  in_target && /remaining_amount: 0$/ { zero = 1 }
  END { if (!found || !zero) exit 1 }
' "$self_tracking_project/canonical/resource_demands.yaml"
self_close_demand_project="$TMP_DIR/self-close-demand-project"
cp -a "$SELF_PROJECT" "$self_close_demand_project"
# The lifecycle checks below need an active demand. Set one up in the copy so
# the smoke does not depend on the living project's current demand states.
awk '
  /^    - id: / { in_target = ($0 ~ /id: resource_demand.full-dogfood-cycle.review$/) }
  in_target && /^      status: / { sub(/status: .*/, "status: active") }
  { print }
' "$self_close_demand_project/canonical/resource_demands.yaml" >"$TMP_DIR/demands.active.yaml"
mv "$TMP_DIR/demands.active.yaml" "$self_close_demand_project/canonical/resource_demands.yaml"
run_json_ok self-close-demand-dry-run "$BIN" actual close-demand \
  --project "$self_close_demand_project" \
  --demand resource_demand.full-dogfood-cycle.review \
  --status cancelled \
  --actor actor.human.maintainer \
  --reason "Operator reviewed demand lifecycle." \
  --dry-run \
  --format json
assert_contains "$TMP_DIR/self-close-demand-dry-run.json" '"previous_status": "active"'
assert_contains "$TMP_DIR/self-close-demand-dry-run.json" '"new_status": "cancelled"'
run_json_ok self-close-demand "$BIN" actual close-demand \
  --project "$self_close_demand_project" \
  --demand resource_demand.full-dogfood-cycle.review \
  --status cancelled \
  --actor actor.human.maintainer \
  --reason "Operator reviewed demand lifecycle." \
  --apply \
  --format json
assert_contains "$TMP_DIR/self-close-demand.json" '"applied": true'
assert_project_audit_contains "$self_close_demand_project" 'actual_close_demand'
awk '
  /id: resource_demand.full-dogfood-cycle.review/ { found = 1; in_target = 1; next }
  in_target && /^    - id:/ { in_target = 0 }
  in_target && /status: cancelled/ { cancelled = 1 }
  END { if (!found || !cancelled) exit 1 }
' "$self_close_demand_project/canonical/resource_demands.yaml"
run_json_ok self-close-demand-validate "$BIN" validate "$self_close_demand_project" --format json
run_json_ok self-reopen-demand "$BIN" actual reopen-demand \
  --project "$self_close_demand_project" \
  --demand resource_demand.full-dogfood-cycle.review \
  --remaining 0.5 \
  --actor actor.human.maintainer \
  --reason "Demand was closed before the task finished." \
  --apply \
  --format json
assert_contains "$TMP_DIR/self-reopen-demand.json" '"applied": true'
assert_contains "$TMP_DIR/self-reopen-demand.json" '"previous_status": "cancelled"'
assert_project_audit_contains "$self_close_demand_project" 'actual_reopen_demand'
awk '
  /id: resource_demand.full-dogfood-cycle.review/ { found = 1; in_target = 1; next }
  in_target && /^    - id:/ { in_target = 0 }
  in_target && /status: active/ { active = 1 }
  in_target && /remaining_amount: 0.5$/ { remaining = 1 }
  END { if (!found || !active || !remaining) exit 1 }
' "$self_close_demand_project/canonical/resource_demands.yaml"
if "$BIN" actual acknowledge-overrun \
  --project "$self_close_demand_project" \
  --demand resource_demand.full-dogfood-cycle.review \
  --actor actor.human.maintainer \
  --reason "No overrun exists here." \
  --apply \
  --format json >"$TMP_DIR/self-acknowledge-no-overrun.json"; then
  echo "acknowledge-overrun accepted a demand without an overrun" >&2
  exit 1
fi
assert_contains "$TMP_DIR/self-acknowledge-no-overrun.json" '"code": "no_overrun"'
run_json_ok self-reopen-demand-validate "$BIN" validate "$self_close_demand_project" --format json
live_overlay_dir="$TMP_DIR/live-overlay"
REPLAN_OPENPROJECT_BINDINGS_PATH="$ROOT_DIR/examples/openproject/private-bindings.replan-self.example.yaml" \
  REPLAN_OVERLAY_OUTPUT_DIR="$live_overlay_dir" \
  "$ROOT_DIR/scripts/build-openproject-live-overlay.sh" >"$TMP_DIR/live-overlay.json"
assert_contains "$TMP_DIR/live-overlay.json" '"ok": true'
assert_contains "$TMP_DIR/live-overlay.json" '"publishes": false'
assert_contains "$TMP_DIR/live-overlay.json" '"missing_link_count": 0'
assert_contains "$live_overlay_dir/raw/openproject_overlay.json" "\"work_package_count\": $SELF_TASK_BINDING_COUNT"
# Publishing puts the overlay into its own project's directory only, after a
# dry run, and refuses to publish it as another project's. The target is set
# here, so a private target in .env.local is never used by the smoke.
publish_dir="$TMP_DIR/overlay-publish"
mkdir -p "$publish_dir/other-project"
printf 'kept\n' >"$publish_dir/other-project/marker"
publish_overlay() {
  REPLAN_OVERLAY_OUTPUT_DIR="$live_overlay_dir" REPLAN_OVERLAY_PUBLISH_TARGET="$publish_dir" \
    REPLAN_OPENPROJECT_PROJECT_ID="$1" REPLAN_APPLY="$2" \
    "$ROOT_DIR/scripts/publish-openproject-overlay.sh" >"$TMP_DIR/overlay-publish.json" || true
}
publish_overlay replan-self ""
assert_contains "$TMP_DIR/overlay-publish.json" '"mode":"dry_run"'
[ ! -e "$publish_dir/replan-self" ] || { echo "overlay publish dry run wrote files" >&2; exit 1; }
publish_overlay replan-self 1
assert_contains "$TMP_DIR/overlay-publish.json" '"mode":"apply"'
assert_contains "$publish_dir/replan-self/raw/openproject_overlay.json" '"openproject_project_id": "replan-self"'
assert_contains "$publish_dir/other-project/marker" 'kept'
publish_overlay other-project 1
assert_contains "$TMP_DIR/overlay-publish.json" '"error_codes":\["overlay_other_project"\]'
[ ! -e "$publish_dir/other-project/raw" ] || { echo "overlay was published as another project's" >&2; exit 1; }
run_json_ok self-scenario-diff "$BIN" scenario diff --project "$SELF_PROJECT" --format json scenario.maintainer-unavailable-20260723
run_json_ok self-openproject-import-dry-run "$BIN" import openproject \
  --project "$SELF_PROJECT" \
  --from-file "$OPENPROJECT_SELF_SAMPLE" \
  --dry-run \
  --format json
assert_contains "$TMP_DIR/self-openproject-import-dry-run.json" '"matched_task_id": "task.self.implement-static-dashboard"'
assert_contains "$TMP_DIR/self-openproject-import-dry-run.json" '"source_id": "2122"'
assert_contains "$TMP_DIR/self-openproject-import-dry-run.json" '"mapped_status": "accepted"'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-operator-report.sh" >"$TMP_DIR/self-openproject-operator-report.json"
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"ok": true'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"command": "openproject operator report"'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"mode": "read_only"'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"operator_report_status": "soft_drift"'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" "\"mapped_tasks\": $SELF_OPERATOR_MAPPED_COUNT"
assert_contains "$TMP_DIR/self-openproject-operator-report.json" "\"task_bindings\": $SELF_TASK_BINDING_COUNT"
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"relation_binding_policy"'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"dry_run_default": true'
assert_contains "$TMP_DIR/self-openproject-operator-report.json" '"task.self.implement-openproject-preflight-operator-helpers"'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package list >"$TMP_DIR/self-openproject-toolkit-list.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-list.json" '"command": "openproject work-package list"'
assert_contains "$TMP_DIR/self-openproject-toolkit-list.json" "\"work_packages\": $SELF_WORK_PACKAGE_COUNT"
assert_contains "$TMP_DIR/self-openproject-toolkit-list.json" "\"mapped\": $SELF_TOOLKIT_MAPPED_COUNT"
assert_contains "$TMP_DIR/self-openproject-toolkit-list.json" '"task.self.implement-openproject-work-package-operator-toolkit"'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" backlog reconcile >"$TMP_DIR/self-openproject-backlog-reconcile-aligned.json"
assert_contains "$TMP_DIR/self-openproject-backlog-reconcile-aligned.json" '"command": "openproject backlog reconcile"'
assert_contains "$TMP_DIR/self-openproject-backlog-reconcile-aligned.json" '"unbound_tasks": 0'
self_reconcile_file="$TMP_DIR/self-reconcile-bindings.yaml"
PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m smoke_fixture_edits drop-task-binding "$OPENPROJECT_SELF_BINDINGS" "$self_reconcile_file" task.self.add-openproject-backlog-reconcile-helper
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$self_reconcile_file" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" backlog reconcile >"$TMP_DIR/self-openproject-backlog-reconcile-dry-run.json"
assert_contains "$TMP_DIR/self-openproject-backlog-reconcile-dry-run.json" '"unbound_tasks": 1'
assert_contains "$TMP_DIR/self-openproject-backlog-reconcile-dry-run.json" '"bind_existing_actions": 1'
assert_contains "$TMP_DIR/self-openproject-backlog-reconcile-dry-run.json" '"task.self.add-openproject-backlog-reconcile-helper"'
REPLAN_APPLY=1 \
  REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$self_reconcile_file" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" backlog reconcile --allow-fixture-apply >"$TMP_DIR/self-openproject-backlog-reconcile-apply.json"
assert_contains "$TMP_DIR/self-openproject-backlog-reconcile-apply.json" '"applied_actions": 1'
assert_contains "$self_reconcile_file" 'task.self.add-openproject-backlog-reconcile-helper'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package create --task task.self.smoke-create-helper --subject "Smoke create helper dry run" --description "Fixture dry-run create check." --status "Closed" --estimated-hours 1 --start-date 2026-10-19 --due-date 2026-10-19 >"$TMP_DIR/self-openproject-toolkit-create-dry-run.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-create-dry-run.json" '"command": "openproject work-package create"'
assert_contains "$TMP_DIR/self-openproject-toolkit-create-dry-run.json" '"dry_run": true'
assert_contains "$TMP_DIR/self-openproject-toolkit-create-dry-run.json" '"apply_required": true'
assert_contains "$TMP_DIR/self-openproject-toolkit-create-dry-run.json" '"planned_actions": 1'
if REPLAN_APPLY=1 \
  REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package create --task task.self.smoke-create-helper --subject "Smoke create helper dry run" >"$TMP_DIR/self-openproject-toolkit-create-fixture-apply.json"; then
  echo "fixture create apply unexpectedly succeeded" >&2
  exit 1
fi
assert_contains "$TMP_DIR/self-openproject-toolkit-create-fixture-apply.json" '"apply_not_allowed_with_fixture"'
self_bind_project="$TMP_DIR/self-bind-project"
cp -a "$SELF_PROJECT" "$self_bind_project"
PYTHONPATH="$ROOT_DIR/scripts/lib${PYTHONPATH:+:$PYTHONPATH}" python3 -m smoke_fixture_edits add-binding-task "$self_bind_project/canonical/tasks.yaml" "$self_bind_project/canonical/current_plan.yaml"
self_bind_file="$TMP_DIR/self-bindings.yaml"
cp "$OPENPROJECT_SELF_BINDINGS" "$self_bind_file"
REPLAN_PROJECT="$self_bind_project" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$self_bind_file" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package bind --task task.self.smoke-binding-helper --work-package 2999 --url https://op.example.internal/work_packages/2999 --skip-existence-check >"$TMP_DIR/self-openproject-toolkit-bind-dry-run.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-bind-dry-run.json" '"command": "openproject work-package bind"'
assert_contains "$TMP_DIR/self-openproject-toolkit-bind-dry-run.json" '"dry_run": true'
assert_contains "$TMP_DIR/self-openproject-toolkit-bind-dry-run.json" '"planned_actions": 1'
REPLAN_APPLY=1 \
  REPLAN_PROJECT="$self_bind_project" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$self_bind_file" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package bind --task task.self.smoke-binding-helper --work-package 2999 --url https://op.example.internal/work_packages/2999 --skip-existence-check >"$TMP_DIR/self-openproject-toolkit-bind-apply.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-bind-apply.json" '"command": "openproject work-package bind"'
assert_contains "$TMP_DIR/self-openproject-toolkit-bind-apply.json" '"applied_actions": 1'
assert_contains "$self_bind_file" 'task.self.smoke-binding-helper'
if REPLAN_PROJECT="$self_bind_project" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$self_bind_file" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package bind --task task.self.smoke-binding-helper --work-package 2999 --url https://op.example.internal/work_packages/2999 --skip-existence-check >"$TMP_DIR/self-openproject-toolkit-bind-duplicate.json"; then
  echo "duplicate binding unexpectedly succeeded" >&2
  exit 1
fi
assert_contains "$TMP_DIR/self-openproject-toolkit-bind-duplicate.json" '"task_already_bound"'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package inspect --task task.self.implement-openproject-work-package-operator-toolkit >"$TMP_DIR/self-openproject-toolkit-inspect.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-inspect.json" '"command": "openproject work-package inspect"'
assert_contains "$TMP_DIR/self-openproject-toolkit-inspect.json" '"work_package_id": "2134"'
assert_contains "$TMP_DIR/self-openproject-toolkit-inspect.json" '"user_authored"'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" status list >"$TMP_DIR/self-openproject-toolkit-status-list.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-status-list.json" '"command": "openproject status list"'
assert_contains "$TMP_DIR/self-openproject-toolkit-status-list.json" '"Closed"'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" work-package update-dates --task task.self.refactor-live-replan-self-openproject-plan --start-date 2026-08-14 --due-date 2026-08-17 >"$TMP_DIR/self-openproject-toolkit-update-dates.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-update-dates.json" '"command": "openproject work-package update-dates"'
assert_contains "$TMP_DIR/self-openproject-toolkit-update-dates.json" '"dry_run": true'
assert_contains "$TMP_DIR/self-openproject-toolkit-update-dates.json" '"planned_actions": 1'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_FROM_FILE="$OPENPROJECT_SELF_SAMPLE" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/openproject-work-package-toolkit.sh" comment add --task task.self.implement-openproject-work-package-operator-toolkit --text "RePlan dry-run smoke comment." >"$TMP_DIR/self-openproject-toolkit-comment.json"
assert_contains "$TMP_DIR/self-openproject-toolkit-comment.json" '"command": "openproject comment add"'
assert_contains "$TMP_DIR/self-openproject-toolkit-comment.json" '"apply_required": true'
REPLAN_PROJECT="$SELF_PROJECT" \
  REPLAN_OPENPROJECT_IMPORT_JSON="$TMP_DIR/self-openproject-import-dry-run.json" \
  REPLAN_OPENPROJECT_BINDINGS_PATH="$OPENPROJECT_SELF_BINDINGS" \
  "$ROOT_DIR/scripts/push-openproject-date-drift.sh" >"$TMP_DIR/self-openproject-date-push-dry-run.json"
assert_contains "$TMP_DIR/self-openproject-date-push-dry-run.json" '"ok": true'
assert_contains "$TMP_DIR/self-openproject-date-push-dry-run.json" '"date_push_status": "needs_review"'
assert_contains "$TMP_DIR/self-openproject-date-push-dry-run.json" '"planned_actions": 0'
# The deliberate date-drift id list in self_openproject_fixture_test.go owns this count.
assert_contains "$TMP_DIR/self-openproject-date-push-dry-run.json" '"review_actions": 23'
assert_contains "$TMP_DIR/self-openproject-date-push-dry-run.json" '"blocked_actions": 0'
assert_contains "$TMP_DIR/self-openproject-date-push-dry-run.json" '"task.self.create-replan-self-fixture"'
run_json_ok self-openproject-import-relations-file-dry-run "$BIN" import openproject \
  --project "$SELF_PROJECT" \
  --from-file "$OPENPROJECT_SELF_SAMPLE" \
  --include-relations \
  --dry-run \
  --format json
assert_contains "$TMP_DIR/self-openproject-import-relations-file-dry-run.json" '"dependency_id": "dependency.openproject.relation.9101"'
assert_contains "$TMP_DIR/self-openproject-import-relations-file-dry-run.json" '"dependency_type": "finish_to_start"'
assert_contains "$TMP_DIR/self-openproject-import-relations-file-dry-run.json" '"dependency_id": "dependency.openproject.relation.9102"'
assert_contains "$TMP_DIR/self-openproject-import-relations-file-dry-run.json" '"dependency_type": "blocks"'
run_json_ok self-openproject-import-time-file-dry-run "$BIN" import openproject \
  --project "$SELF_PROJECT" \
  --from-file "$OPENPROJECT_SELF_SAMPLE" \
  --include-time-entries \
  --dry-run \
  --format json
assert_contains "$TMP_DIR/self-openproject-import-time-file-dry-run.json" '"source_id": "9201"'
assert_contains "$TMP_DIR/self-openproject-import-time-file-dry-run.json" '"work_package_id": "2105"'
assert_contains "$TMP_DIR/self-openproject-import-time-file-dry-run.json" '"hours": 1.5'
self_private_accept_project="$TMP_DIR/self-private-accept-project"
cp -a "$SELF_PROJECT" "$self_private_accept_project"
run_json_ok self-openproject-import-private-policy-file-write "$BIN" import openproject \
  --project "$self_private_accept_project" \
  --from-file "$OPENPROJECT_SELF_SAMPLE" \
  --include-relations \
  --format json
self_private_snapshot_dir="$(find "$self_private_accept_project/imports/openproject" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1)"
self_private_snapshot_id="$(basename "$self_private_snapshot_dir")"
if "$BIN" import accept \
  --project "$self_private_accept_project" \
  --accept-dependencies \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$self_private_snapshot_id" >"$TMP_DIR/self-openproject-private-accept-refused.json"; then
  echo "private-bindings dependency accept unexpectedly succeeded" >&2
  exit 1
else
  private_accept_exit=$?
fi
[ "$private_accept_exit" -eq 8 ] || { echo "private-bindings accept exited $private_accept_exit, want 8" >&2; exit 1; }
assert_contains "$TMP_DIR/self-openproject-private-accept-refused.json" '"code": "openproject_ids_private"'
run_json_ok self-openproject-private-dates-accepted "$BIN" import accept \
  --project "$self_private_accept_project" \
  --accept-dates \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$self_private_snapshot_id"
self_relations_project="$TMP_DIR/self-relations-project"
cp -a "$SELF_PROJECT" "$self_relations_project"
set_openproject_ids_canonical "$self_relations_project"
run_json_ok self-openproject-import-relations-file-write "$BIN" import openproject \
  --project "$self_relations_project" \
  --from-file "$OPENPROJECT_SELF_SAMPLE" \
  --include-relations \
  --format json
self_relations_snapshot_dir="$(find "$self_relations_project/imports/openproject" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1)"
self_relations_snapshot_id="$(basename "$self_relations_snapshot_dir")"
run_json_ok self-openproject-accept-dependencies-dry-run "$BIN" import accept \
  --project "$self_relations_project" \
  --accept-dependencies \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$self_relations_snapshot_id"
assert_contains "$TMP_DIR/self-openproject-accept-dependencies-dry-run.json" '"target_id": "dependency.openproject.relation.9101"'
assert_contains "$TMP_DIR/self-openproject-accept-dependencies-dry-run.json" '"target_id": "dependency.openproject.relation.9102"'
self_time_project="$TMP_DIR/self-time-project"
cp -a "$SELF_PROJECT" "$self_time_project"
set_openproject_ids_canonical "$self_time_project"
run_json_ok self-openproject-import-time-file-write "$BIN" import openproject \
  --project "$self_time_project" \
  --from-file "$OPENPROJECT_SELF_SAMPLE" \
  --include-time-entries \
  --format json
self_time_snapshot_dir="$(find "$self_time_project/imports/openproject" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1)"
self_time_snapshot_id="$(basename "$self_time_snapshot_dir")"
run_json_ok self-openproject-accept-time-dry-run "$BIN" import accept \
  --project "$self_time_project" \
  --accept-time-entries \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$self_time_snapshot_id"
assert_contains "$TMP_DIR/self-openproject-accept-time-dry-run.json" '"target_id": "evidence.openproject.time-entry.9201"'
assert_contains "$TMP_DIR/self-openproject-accept-time-dry-run.json" '"target_id": "actual.openproject.time-entry.9201.work-consumed"'
self_demands_project="$TMP_DIR/self-demands-project"
cp -a "$SELF_PROJECT" "$self_demands_project"
run_json_ok self-openproject-import-demands-file-write "$BIN" import openproject \
  --project "$self_demands_project" \
  --from-file "$OPENPROJECT_SELF_SAMPLE" \
  --resource-demands-sidecar "$OPENPROJECT_SELF_RESOURCE_DEMANDS_SIDECAR" \
  --format json
self_demands_snapshot_dir="$(find "$self_demands_project/imports/openproject" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1)"
self_demands_snapshot_id="$(basename "$self_demands_snapshot_dir")"
run_json_ok self-openproject-accept-resource-demands-dry-run "$BIN" import accept \
  --project "$self_demands_project" \
  --accept-resource-demands \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$self_demands_snapshot_id"
assert_contains "$TMP_DIR/self-openproject-accept-resource-demands-dry-run.json" '"target_id": "resource_demand.openproject-fixtures.llm-token-budget"'
assert_contains "$TMP_DIR/self-openproject-accept-resource-demands-dry-run.json" '"op": "create_resource_demand"'

self_dashboard_project="$TMP_DIR/self-dashboard-project"
cp -a "$SELF_PROJECT" "$self_dashboard_project"
make_self_tracking_suggest_stale "$self_dashboard_project"
run_json_ok self-dashboard "$BIN" report dashboard --project "$self_dashboard_project" --week 2026-W31 --output reports/latest --format html
test -s "$self_dashboard_project/reports/latest/index.html"
test -s "$self_dashboard_project/reports/latest/gantt.html"
test -s "$self_dashboard_project/reports/latest/raw/overview.json"
test -s "$self_dashboard_project/reports/latest/raw/gantt.json"
test -s "$self_dashboard_project/reports/latest/raw/openproject_overlay.json"
test -s "$self_dashboard_project/reports/latest/raw/manifest.json"
assert_contains "$self_dashboard_project/reports/latest/index.html" 'RePlan self-dogfood'
assert_contains "$self_dashboard_project/reports/latest/index.html" 'Tracking Proposal Loop'
assert_contains "$self_dashboard_project/reports/latest/index.html" 'Stale remaining'
assert_contains "$self_dashboard_project/reports/latest/index.html" 'Priority'
assert_contains "$self_dashboard_project/reports/latest/index.html" 'accepted task has stale remaining'
assert_contains "$self_dashboard_project/reports/latest/index.html" 'actionable'
assert_contains "$self_dashboard_project/reports/latest/index.html" 'OpenProject Links'
assert_contains "$self_dashboard_project/reports/latest/gantt.html" 'RePlan Gantt'
assert_contains "$self_dashboard_project/reports/latest/gantt.html" 'openproject:2101'
assert_contains "$self_dashboard_project/reports/latest/raw/overview.json" '"schema_version": 1'
assert_contains "$self_dashboard_project/reports/latest/raw/overview.json" "\"tasks_with_source_refs\": $SELF_SOURCE_TASK_COUNT"
assert_contains "$self_dashboard_project/reports/latest/raw/overview.json" '"tracking_proposal_loop"'
assert_contains "$self_dashboard_project/reports/latest/raw/overview.json" '"status": "actionable"'
assert_contains "$self_dashboard_project/reports/latest/raw/overview.json" '"priority": "high"'
assert_contains "$self_dashboard_project/reports/latest/raw/gantt.json" "\"task_count\": $SELF_TASK_COUNT"
assert_contains "$self_dashboard_project/reports/latest/raw/gantt.json" '"source_id": "2101"'
assert_contains "$self_dashboard_project/reports/latest/raw/gantt.json" '"resource_loads"'
assert_contains "$self_dashboard_project/reports/latest/raw/gantt.json" '"participation_window_source"'
assert_contains "$self_dashboard_project/reports/latest/raw/gantt.json" '"intensity_percent"'
assert_contains "$self_dashboard_project/reports/latest/raw/gantt.json" '"bucket_type": "week"'
assert_contains "$self_dashboard_project/reports/latest/raw/gantt.json" '"waterline_ratio": 1'
assert_contains "$self_dashboard_project/reports/latest/raw/openproject_overlay.json" "\"work_package_count\": $SELF_SOURCE_TASK_COUNT"
assert_contains "$self_dashboard_project/reports/latest/raw/openproject_overlay.json" '"work_package_id": "2142"'
assert_contains "$self_dashboard_project/reports/latest/raw/openproject_overlay.json" '"replan_task_id": "task.self.define-openproject-overlay-json-contract"'
assert_contains "$self_dashboard_project/reports/latest/raw/openproject_overlay.json" '"resource_demand.openproject-overlay-json-contract.codex"'
assert_contains "$self_dashboard_project/reports/latest/raw/openproject_overlay.json" '"replan_task_id": "task.self.improve-live-openproject-plugin-resource-view"'
assert_contains "$self_dashboard_project/reports/latest/raw/openproject_overlay.json" '"replan_task_id": "task.self.design-openproject-work-package-replan-panel"'
assert_contains "$self_dashboard_project/reports/latest/gantt.html" 'Resource Load'
assert_contains "$self_dashboard_project/reports/latest/gantt.html" 'gantt-resource-row'
assert_contains "$self_dashboard_project/reports/latest/gantt.html" '100% capacity waterline'
assert_contains "$self_dashboard_project/reports/latest/gantt.html" 'bucket type week'
assert_contains "$self_dashboard_project/reports/latest/raw/manifest.json" '"gantt.html"'
assert_contains "$self_dashboard_project/reports/latest/raw/manifest.json" '"raw/gantt.json"'
assert_contains "$self_dashboard_project/reports/latest/raw/manifest.json" '"raw/openproject_overlay.json"'
assert_contains "$self_dashboard_project/reports/latest/raw/manifest.json" '"index.html"'

run_json_ok report-json "$BIN" report weekly --project "$PROJECT_PATH" --week "$WEEK" --format json
"$BIN" report weekly --project "$PROJECT_PATH" --week "$WEEK" --format markdown >"$TMP_DIR/report.md"
"$BIN" report weekly --project "$PROJECT_PATH" --week "$WEEK" --format csv >"$TMP_DIR/report.csv"
"$BIN" report weekly --project "$PROJECT_PATH" --week "$WEEK" --format html >"$TMP_DIR/report.html"
assert_contains "$TMP_DIR/report.md" 'Weekly RePlan Report'
assert_contains "$TMP_DIR/report.csv" 'resource_id,resource_name,resource_class'
assert_contains "$TMP_DIR/report.html" '<h2>Workload</h2>'

html_project="$TMP_DIR/html-project"
copy_project "$html_project"
run_json_ok report-html-write "$BIN" report weekly --project "$html_project" --week "$WEEK" --format html --write
test -s "$html_project/reports/weekly/$WEEK.html"

run_json_ok scenario-show-leading "$BIN" scenario show scenario.ivan-unavailable-week-28 --project "$PROJECT_PATH" --format json
run_json_ok scenario-show-trailing "$BIN" scenario show --project "$PROJECT_PATH" --format json scenario.ivan-unavailable-week-28
run_json_ok scenario-diff "$BIN" scenario diff --project "$PROJECT_PATH" --against current --format json scenario.ivan-unavailable-week-28
run_json_ok scenario-apply-dry-run "$BIN" scenario apply --project "$PROJECT_PATH" --dry-run --format json scenario.task-op-1234-remaining-5

run_json_ok proposal-list "$BIN" proposal list --project "$PROJECT_PATH" --status open --format json
run_json_ok proposal-show-leading "$BIN" proposal show proposal.adjust-remaining-task-op-1234.20260706 --project "$PROJECT_PATH" --format json
run_json_ok proposal-show-trailing "$BIN" proposal show --project "$PROJECT_PATH" --format json proposal.adjust-remaining-task-op-1234.20260706
run_json_ok proposal-accept-dry-run-trailing "$BIN" proposal accept --project "$PROJECT_PATH" --dry-run --format json proposal.adjust-remaining-task-op-1234.20260706
run_json_ok dependency-list "$BIN" dependency list --project "$PROJECT_PATH" --format json

run_json_ok actual-list "$BIN" actual list --project "$PROJECT_PATH" --format json
run_json_ok evidence-add-dry-run "$BIN" evidence add \
  --project "$PROJECT_PATH" \
  --id evidence.smoke.actual-add \
  --type test_result \
  --subtype smoke \
  --source local \
  --actor "$ACTOR" \
  --observed-at "2026-07-08T09:55:00+07:00" \
  --confidence observed \
  --summary "Smoke evidence for actual consumption." \
  --target task:task.op.1234 \
  --target resource_demand:resource_demand.task-op-1234.implementer \
  --dry-run \
  --format json
run_json_ok actual-add-dry-run "$BIN" actual add-consumption \
  --project "$PROJECT_PATH" \
  --task task.op.1234 \
  --demand resource_demand.task-op-1234.implementer \
  --resource resource.human.ivan \
  --amount 1 \
  --unit hours \
  --actor "$ACTOR" \
  --occurred-at "2026-07-08T10:00:00+07:00" \
  --status-date "2026-07-08" \
  --dry-run \
  --format json

# Task mutation commands use their own copy so later fixture-count assertions
# remain tied to the unchanged smoke projects.
task_project="$TMP_DIR/task-project"
copy_project "$task_project"
run_json_ok task-add-dry-run "$BIN" task add \
  --project "$task_project" \
  --id task.op.smoke-task-command \
  --title "Smoke task command" \
  --description "Exercise the staged three-file task writer." \
  --planned-start 2026-07-16 \
  --planned-finish 2026-07-17 \
  --tag smoke \
  --assignee resource.human.ivan \
  --after task.op.1234 \
  --demand implementer=resource.human.ivan:2:hours \
  --demand agent=resource.ai.codex.local:1:agent_hours:ai_coding_agent \
  --actor "$ACTOR" \
  --evidence evidence.github.pr.45 \
  --occurred-at 2026-10-26T12:00:00+07:00 \
  --dry-run \
  --format json
run_json_ok task-add-write "$BIN" task add \
  --project "$task_project" \
  --id task.op.smoke-task-command \
  --title "Smoke task command" \
  --description "Exercise the staged three-file task writer." \
  --planned-start 2026-07-16 \
  --planned-finish 2026-07-17 \
  --tag smoke \
  --assignee resource.human.ivan \
  --after task.op.1234 \
  --demand implementer=resource.human.ivan:2:hours \
  --demand agent=resource.ai.codex.local:1:agent_hours:ai_coding_agent \
  --actor "$ACTOR" \
  --evidence evidence.github.pr.45 \
  --occurred-at 2026-10-26T12:00:00+07:00 \
  --apply \
  --format json
assert_contains "$task_project/canonical/tasks.yaml" 'task.op.smoke-task-command'
assert_contains "$task_project/canonical/resource_demands.yaml" 'resource_demand.smoke-task-command.implementer'
assert_contains "$task_project/canonical/resource_demands.yaml" 'resource_demand.smoke-task-command.agent'
run_json_ok task-set-dates "$BIN" task set-dates \
  --project "$task_project" \
  --task task.op.smoke-task-command \
  --planned-start 2026-07-17 \
  --planned-finish 2026-07-18 \
  --reason "Accept smoke scheduler shift." \
  --actor "$ACTOR" \
  --evidence evidence.github.pr.45 \
  --occurred-at 2026-10-26T12:05:00+07:00 \
  --apply \
  --format json
run_json_ok task-project-validate "$BIN" validate "$task_project" --format json
assert_project_audit_contains "$task_project" 'task_add'
assert_project_audit_contains "$task_project" 'task_set_dates'

mutation_project="$TMP_DIR/mutation-project"
copy_project "$mutation_project"
run_json_ok evidence-add-write "$BIN" evidence add \
  --project "$mutation_project" \
  --id evidence.smoke.actual-add \
  --type test_result \
  --subtype smoke \
  --source local \
  --actor "$ACTOR" \
  --observed-at "2026-07-08T09:55:00+07:00" \
  --confidence observed \
  --summary "Smoke evidence for actual consumption." \
  --target task:task.op.1234 \
  --target resource_demand:resource_demand.task-op-1234.implementer \
  --apply \
  --format json
assert_contains "$mutation_project/evidence/evidence.2026-W28.yaml" 'evidence.smoke.actual-add'
assert_contains "$mutation_project/audit/audit.2026-07.jsonl" 'evidence_add'
run_json_ok evidence-project-validate "$BIN" validate "$mutation_project" --format json
run_json_ok actual-add-write "$BIN" actual add-consumption \
  --project "$mutation_project" \
  --task task.op.1234 \
  --demand resource_demand.task-op-1234.implementer \
  --resource resource.human.ivan \
  --amount 1 \
  --unit hours \
  --evidence evidence.smoke.actual-add \
  --actor "$ACTOR" \
  --occurred-at "2026-07-08T10:00:00+07:00" \
  --status-date "2026-07-08" \
  --apply \
  --format json
assert_contains "$mutation_project/audit/audit.2026-07.jsonl" 'actual_add_consumption'
run_json_ok scenario-apply-write "$BIN" scenario apply \
  --project "$mutation_project" \
  --actor "$ACTOR" \
  --reason "MVP smoke scenario apply." \
  --apply \
  --format json \
  scenario.task-op-1234-remaining-5
assert_project_audit_contains "$mutation_project" 'scenario_apply'
# GAP-032: a scenario cancels a dependency and the audit names the decision.
cat >"$mutation_project/scenarios/scenario.smoke-cancel-dependency.yaml" <<'YAML'
scenario:
  id: scenario.smoke-cancel-dependency
  name: Smoke lets validation go ahead of the pump repair
  status: proposed
  base_ref:
    kind: current_plan
    id: current
  changes:
    - op: set
      path: dependencies.dependency.repair-pump-before-validation.status
      value: cancelled
YAML
if "$BIN" scenario apply --project "$mutation_project" --actor "$ACTOR" --apply --format json \
  scenario.smoke-cancel-dependency >"$TMP_DIR/scenario-cancel-no-evidence.json"; then
  printf 'scenario cancelling a dependency without evidence was applied\n' >&2
  exit 1
fi
assert_contains "$TMP_DIR/scenario-cancel-no-evidence.json" '"code": "evidence_required"'
run_json_ok scenario-cancel-dependency "$BIN" scenario apply \
  --project "$mutation_project" \
  --actor "$ACTOR" \
  --evidence evidence.github.pr.45 \
  --apply \
  --format json \
  scenario.smoke-cancel-dependency
assert_contains "$mutation_project/canonical/dependencies.yaml" 'status: cancelled'
assert_project_audit_contains "$mutation_project" 'evidence.github.pr.45'
run_json_ok validate-after-mutation "$BIN" validate "$mutation_project" --format json
run_json_ok audit-verify-after-mutation "$BIN" audit verify --project "$mutation_project" --format json

run_json_ok openproject-import-dry-run "$BIN" import openproject \
  --project "$PROJECT_PATH" \
  --from-file "$OPENPROJECT_SAMPLE" \
  --dry-run \
  --format json
run_json_ok openproject-import-relations-file-dry-run "$BIN" import openproject \
  --project "$PROJECT_PATH" \
  --from-file "$OPENPROJECT_SAMPLE" \
  --include-relations \
  --dry-run \
  --format json

import_project="$TMP_DIR/import-project"
copy_project "$import_project"
run_json_ok openproject-import-write "$BIN" import openproject \
  --project "$import_project" \
  --from-file "$OPENPROJECT_SAMPLE" \
  --format json
snapshot_dir="$(find "$import_project/imports/openproject" -mindepth 1 -maxdepth 1 -type d | sort | tail -n 1)"
test -s "$snapshot_dir/manifest.yaml"
test -s "$snapshot_dir/raw.json"
test -s "$snapshot_dir/mapped.json"
snapshot_id="$(basename "$snapshot_dir")"
run_json_ok import-accept-dry-run "$BIN" import accept \
  --project "$import_project" \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$snapshot_id"
run_json_ok import-accept-tracking-status-dry-run "$BIN" import accept \
  --project "$import_project" \
  --accept-tracking-status \
  --dry-run \
  --format json \
  "source_snapshot.openproject.$snapshot_id"

# The agent kit: the archive an outside agent works from, built, unpacked
# outside the repository and used with no Go toolchain.
"$ROOT_DIR/scripts/check-agent-kit.sh"

echo "mvp smoke ok"
