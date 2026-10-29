"""Moved from scripts/push-openproject-date-drift.sh (GAP-018). Invoked as `python3 -m push_openproject_date_drift`."""
import json
import os
import sys

import yaml
from openproject_http import OpenProjectClient, Settings
import helper_envelope
import openproject_bindings
import openproject_rescheduling
import openproject_working_days

parser = helper_envelope.internal_parser("push-openproject-date-drift")
parser.add_argument("--import-json", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--root-dir", required=True)
args = parser.parse_args()
settings = Settings.from_env(root_dir=args.root_dir)
import_path, tmp_dir = args.import_json, args.tmp_dir
project_path, bindings_path = settings.project_path, settings.bindings_path
base_url, openproject_project_id = settings.base_url, settings.project_id
token_env, host_header = settings.token_env, settings.host_header
forwarded_proto, apply = settings.forwarded_proto, settings.apply
keep_temp = settings.keep_temp

mode = "apply" if apply == "1" else "dry_run"
partial_apply = helper_envelope.PartialApply()
helper_envelope.guard(
    "push-openproject-date-drift", mode,
    unknown_patch_next_step="Inspect the work package dates before running the push again.",
    partial_apply=partial_apply,
    partial_next_steps=(
        "Run this helper again as a dry run; it recomputes date drift, so already-applied work packages are no longer planned.",
    ),
)
fixture_mode = bool(os.environ.get("REPLAN_OPENPROJECT_IMPORT_JSON", ""))
base_url = base_url.rstrip("/")
client = OpenProjectClient(
    base_url, settings, host_header, forwarded_proto,
    missing_base_url_message="REPLAN_OPENPROJECT_BASE_URL is required for apply",
    missing_token_message=f"{token_env} is required for apply",
)

def load_yaml(path):
    with open(path) as fh:
        return yaml.safe_load(fh) or {}

def norm(value):
    return " ".join(str(value or "").strip().lower().split())

def as_text(value):
    if value is None:
        return ""
    return str(value)

def get_work_package(work_package_id):
    _, wp = client.request_json("GET", f"/api/v3/work_packages/{work_package_id}", expected=(200,))
    return wp

def update_work_package_dates(wp, start_date, due_date, working_by_day, action):
    payload = openproject_working_days.manual_schedule_payload(
        wp.get("lockVersion"), start_date, due_date, action, working_by_day,
    )
    work_package_id = as_text(wp.get("id"))
    partial_apply.writing()
    _, updated = client.request_json("PATCH", f"/api/v3/work_packages/{work_package_id}", payload, expected=(200,))
    return updated

def dependency_key(dep):
    return (
        as_text(dep.get("type")),
        as_text((dep.get("predecessor_ref") or {}).get("id")),
        as_text((dep.get("successor_ref") or {}).get("id")),
    )

data = json.load(open(import_path))
result = data.get("result") or {}
warnings = data.get("warnings") or result.get("warnings") or []
errors = data.get("errors") or result.get("errors") or []

tasks_doc = load_yaml(os.path.join(project_path, "canonical", "tasks.yaml"))
deps_doc = load_yaml(os.path.join(project_path, "canonical", "dependencies.yaml"))
plan_doc = load_yaml(os.path.join(project_path, "canonical", "current_plan.yaml"))

planned_task_ids = [
    item.get("id", "")
    for item in ((plan_doc.get("current_plan") or {}).get("included_tasks") or [])
    if item.get("kind") == "task" and item.get("id")
]
planned_task_set = set(planned_task_ids)
local_tasks = {
    task.get("id", ""): task
    for task in tasks_doc.get("tasks") or []
    if task.get("id", "") in planned_task_set
}
local_deps = [
    dep for dep in deps_doc.get("dependencies") or []
    if dependency_key(dep)[1] in planned_task_set and dependency_key(dep)[2] in planned_task_set
]

bindings_loaded = os.path.exists(bindings_path)
task_wp_by_id = {}
dep_relation_by_id = {}
binding_errors = []
if bindings_loaded:
    bindings_doc = openproject_bindings.load(bindings_path)
    task_wp_by_id, _ = openproject_bindings.task_maps(bindings_doc, duplicate_task_errors=binding_errors)
    dep_relation_by_id, _ = openproject_bindings.dependency_maps(bindings_doc)
else:
    binding_errors.append({"kind": "missing_private_bindings", "path": bindings_path})

task_candidates = result.get("task_candidates") or []
dep_candidates = result.get("dependency_candidates") or []
require_dependency_check = (not fixture_mode) or bool(dep_candidates)
candidate_by_wp = {as_text(item.get("source_id")): item for item in task_candidates if item.get("source_id") is not None}
local_task_by_title = {norm(task.get("title", "")): task_id for task_id, task in local_tasks.items()}
bound_wp_set = set(task_wp_by_id.values())

blocked = []
review = []
planned = []

for task_id in planned_task_ids:
    task = local_tasks.get(task_id)
    if not task:
        continue
    wp_id = task_wp_by_id.get(task_id, "")
    if not wp_id:
        blocked.append({"task_id": task_id, "reason": "missing_task_binding"})
        continue
    candidate = candidate_by_wp.get(wp_id)
    if not candidate:
        blocked.append({"task_id": task_id, "work_package_id": wp_id, "reason": "missing_live_work_package"})
        continue
    if norm(candidate.get("title", "")) != norm(task.get("title", "")):
        blocked.append({
            "task_id": task_id,
            "work_package_id": wp_id,
            "reason": "title_drift",
            "replan_title": task.get("title", ""),
            "openproject_title": candidate.get("title", ""),
        })
        continue

    from_start = as_text(candidate.get("start_date"))
    from_due = as_text(candidate.get("due_date"))
    to_start = as_text(task.get("planned_start"))
    to_due = as_text(task.get("planned_finish"))
    if from_start == to_start and from_due == to_due:
        continue

    action = {
        "action": "push_openproject_dates",
        "task_id": task_id,
        "work_package_id": wp_id,
        "title": task.get("title", ""),
        "from_openproject_start_date": from_start,
        "to_openproject_start_date": to_start,
        "from_openproject_due_date": from_due,
        "to_openproject_due_date": to_due,
        "replan_tracking_status": as_text(task.get("tracking_status")),
        "openproject_source_status": as_text(candidate.get("source_status")),
        "openproject_mapped_status": as_text(candidate.get("mapped_status")),
    }
    if as_text(candidate.get("mapped_status")) == "accepted" or as_text(task.get("tracking_status")) == "accepted":
        review.append({**action, "action": "review_openproject_dates", "reason": "completed_task_date_change"})
    else:
        planned.append(action)

for candidate in task_candidates:
    source_id = as_text(candidate.get("source_id"))
    if source_id in bound_wp_set:
        continue
    blocked.append({
        "work_package_id": source_id,
        "title": candidate.get("title", ""),
        "possible_replan_task_id_by_title": local_task_by_title.get(norm(candidate.get("title", "")), ""),
        "reason": "extra_live_work_package",
    })

if require_dependency_check:
    local_dep_tuples = {}
    for dep in local_deps:
        local_dep_tuples[dependency_key(dep)] = dep.get("id", "")

    live_dep_tuples = {}
    for dep in dep_candidates:
        key = (
            as_text(dep.get("dependency_type")),
            as_text(dep.get("predecessor_task_id")),
            as_text(dep.get("successor_task_id")),
        )
        live_dep_tuples[key] = dep

    for dep in local_deps:
        dep_id = as_text(dep.get("id"))
        key = dependency_key(dep)
        if not dep_relation_by_id.get(dep_id):
            blocked.append({
                "dependency_id": dep_id,
                "dependency_type": key[0],
                "predecessor_task_id": key[1],
                "successor_task_id": key[2],
                "reason": "missing_dependency_binding",
            })
            continue
        candidate = live_dep_tuples.get(key)
        if not candidate:
            blocked.append({
                "dependency_id": dep_id,
                "dependency_type": key[0],
                "predecessor_task_id": key[1],
                "successor_task_id": key[2],
                "reason": "missing_live_dependency",
            })
            continue
        bound_relation_id = dep_relation_by_id.get(dep_id, "")
        source_id = as_text(candidate.get("source_id"))
        if bound_relation_id and source_id and bound_relation_id != source_id:
            blocked.append({
                "dependency_id": dep_id,
                "bound_relation_id": bound_relation_id,
                "live_relation_id": source_id,
                "dependency_type": key[0],
                "predecessor_task_id": key[1],
                "successor_task_id": key[2],
                "reason": "relation_binding_mismatch",
            })

    for key, candidate in live_dep_tuples.items():
        if key not in local_dep_tuples:
            blocked.append({
                "relation_id": as_text(candidate.get("source_id")),
                "dependency_type": key[0],
                "predecessor_task_id": key[1],
                "successor_task_id": key[2],
                "reason": "extra_live_dependency",
            })

for item in binding_errors:
    blocked.append(item)
for err in errors:
    blocked.append({"reason": "openproject_import_error", "code": err.get("code", ""), "message": err.get("message", "")})

working_by_day = None
if planned:
    working_by_day, working_day_warnings = openproject_working_days.load(client)
    warnings.extend(working_day_warnings)
    for action in planned:
        openproject_working_days.apply_ignore_non_working_days(
            {},
            action,
            working_by_day,
            (
                ("start date", action["to_openproject_start_date"]),
                ("due date", action["to_openproject_due_date"]),
            ),
        )

applied = []
rescheduled = []
stored_differences = []
rescheduling_unread = []
apply_blocked_reason = ""
if mode == "apply":
    if fixture_mode:
        apply_blocked_reason = "apply_not_allowed_with_import_json_fixture"
    elif blocked:
        apply_blocked_reason = "blocked_actions_present"
    elif review:
        apply_blocked_reason = "review_actions_present"
    elif not planned:
        apply_blocked_reason = "no_planned_actions"
    else:
        binding_relations = []
        for binding in openproject_bindings.root(bindings_doc).get("dependencies") or []:
            binding_relations.append({
                "type": binding.get("relation_type", ""),
                "from_work_package_id": as_text(binding.get("from_work_package_id")),
                "to_work_package_id": as_text(binding.get("to_work_package_id")),
            })
        successors = openproject_rescheduling.follows_successors(binding_relations)
        pushed_work_packages = {action["work_package_id"] for action in planned}
        rescheduling_targets = openproject_rescheduling.reachable_work_packages(
            pushed_work_packages, successors
        )
        before_dates, _ = openproject_rescheduling.work_package_dates(
            client, rescheduling_targets
        )
        stale = []
        for action in planned:
            wp = before_dates[action["work_package_id"]]["work_package"]
            current_start = as_text(wp.get("startDate"))
            current_due = as_text(wp.get("dueDate"))
            if current_start != action["from_openproject_start_date"] or current_due != action["from_openproject_due_date"]:
                stale.append({
                    **action,
                    "reason": "live_dates_changed_after_plan",
                    "current_openproject_start_date": current_start,
                    "current_openproject_due_date": current_due,
                })
        if stale:
            blocked.extend(stale)
            apply_blocked_reason = "stale_live_dates"
        else:
            for action in planned:
                partial_apply.attempting(action)
                wp = get_work_package(action["work_package_id"])
                updated = update_work_package_dates(
                    wp,
                    action["to_openproject_start_date"],
                    action["to_openproject_due_date"],
                    working_by_day,
                    action,
                )
                applied.append({
                    **action,
                    "action": "pushed_openproject_dates",
                    "updated_lock_version": updated.get("lockVersion"),
                    "updated_openproject_start_date": updated.get("startDate", ""),
                    "updated_openproject_due_date": updated.get("dueDate", ""),
                    "updated_schedule_manually": updated.get("scheduleManually"),
                })
                partial_apply.applied(applied[-1])
            after_dates, rescheduling_unread = openproject_rescheduling.work_package_dates(
                client, rescheduling_targets, tolerate_failures=True
            )
            causes = {}
            for action in applied:
                root = action["work_package_id"]
                for affected in openproject_rescheduling.reachable_work_packages(
                    [root], successors, include_roots=False
                ):
                    causes.setdefault(affected, []).append({
                        "caused_by_date_push": {
                            "task_id": action["task_id"],
                            "work_package_id": root,
                        }
                    })
            excluded = {
                (work_package_id, field)
                for work_package_id in pushed_work_packages
                for field in ("startDate", "dueDate")
            }
            rescheduled = openproject_rescheduling.moved_dates(
                before_dates, after_dates, openproject_bindings.task_maps(bindings_doc)[1],
                causes=causes, excluded=excluded,
            )
            stored_differences = openproject_rescheduling.stored_date_differences(
                client, applied
            )

date_push_status = "blocked" if blocked else ("needs_review" if review else ("planned" if planned else "aligned"))
ok = data.get("ok") is True and not blocked
if mode == "apply":
    ok = ok and not review and not apply_blocked_reason and len(applied) == len(planned)

out = {
    "ok": ok,
    "command": "push-openproject-date-drift",
    "mode": mode,
    "date_push_status": date_push_status if not applied else "applied",
    "project_path": project_path,
    "target_openproject_project": {"identifier": openproject_project_id},
    "openproject_request": settings.request_metadata(),
    "private_binding_layer": {
        "path": bindings_path,
        "loaded": bindings_loaded,
        "task_bindings": len(task_wp_by_id),
        "dependency_bindings": len(dep_relation_by_id),
    },
    "policy": {
        "direction": "replan_planned_dates_to_openproject_dates",
        "pushed_fields": {
            "planned_start": "startDate",
            "planned_finish": "dueDate",
        },
        "dry_run_default": True,
        "apply_gate": "REPLAN_APPLY=1",
        "review_actions_are_not_applied": True,
        "hard_drift_blocks_apply": True,
        "dependency_check": "required" if require_dependency_check else "not_available_in_import_fixture",
    },
    "counts": {
        "planned_actions": len(planned),
        "review_actions": len(review),
        "blocked_actions": len(blocked),
        "applied_actions": len(applied),
        "warning_codes": len({w.get("code", "") for w in warnings if w.get("code")}),
        "error_codes": len({e.get("code", "") for e in errors if e.get("code")}),
    },
    "planned_actions": planned,
    "review_actions": review,
    "blocked_actions": blocked,
    "applied_actions": applied,
    "apply_blocked_reason": apply_blocked_reason,
    "warning_codes": sorted({w.get("code", "") for w in warnings if w.get("code")}),
    "error_codes": sorted({e.get("code", "") for e in errors if e.get("code")}),
    "operator_next_steps": [
        "Review planned_actions before applying.",
        "Resolve blocked_actions before any OpenProject mutation.",
        "Resolve or explicitly defer review_actions before apply; this helper will not apply them.",
        "Run with REPLAN_APPLY=1 only after the dry-run output is accepted.",
        "Run check-openproject-drift after apply.",
        "Run check-resource-schedule-drift after OpenProject is updated.",
    ],
    "temp_retained": bool(keep_temp),
}
extra_warnings = []
if stored_differences:
    out["stored_date_differences"] = stored_differences
    extra_warnings.append({
        **helper_envelope.diagnostic(
            "openproject_stored_other_dates",
            f"OpenProject stored {len(stored_differences)} date value(s) differently from the push.",
            "warning",
        ),
        "count": len(stored_differences),
    })
if rescheduled:
    out["rescheduled_work_packages"] = rescheduled
    moved_count = len({item["work_package_id"] for item in rescheduled})
    extra_warnings.append({
        **helper_envelope.diagnostic(
            "openproject_rescheduled",
            f"OpenProject rescheduled {moved_count} successor work package(s) during the date push.",
            "warning",
        ),
        "count": moved_count,
    })
if rescheduling_unread:
    out["ok"] = False
    extra_warnings.append({
        **helper_envelope.diagnostic(
            "rescheduling_unknown",
            "Date updates were applied, but some possibly affected work packages could not be read afterward.",
            "warning",
        ),
        "work_package_ids": rescheduling_unread,
    })
if extra_warnings:
    out["warnings"] = warnings + extra_warnings
if stored_differences:
    out["operator_next_steps"] = [
        "Run scripts/check-openproject-drift.sh to review the dates OpenProject stored.",
        "Prefer accepting scheduler dates: review scripts/accept-openproject-soft-drift.sh, then apply it with REPLAN_ACTOR=<actor-id> REPLAN_APPLY=1.",
        "For one task, run replan task set-dates --dry-run with OpenProject's working-day dates, then repeat with --apply.",
        "Run scripts/push-openproject-date-drift.sh again as a dry run; it should plan nothing after acceptance.",
    ]
elif rescheduled or rescheduling_unread:
    out["operator_next_steps"] = [
        "Run scripts/check-openproject-drift.sh to review OpenProject's successor date changes.",
        "If RePlan dates should win, run scripts/push-openproject-date-drift.sh as a dry run, then apply it with REPLAN_APPLY=1.",
        "Otherwise accept OpenProject's scheduler dates through scripts/accept-openproject-soft-drift.sh.",
    ]
if keep_temp:
    out["outputs_dir"] = tmp_dir
exit_code = helper_envelope.EXIT["attention"] if rescheduling_unread else helper_envelope.apply_exit_code(out, len(applied))
helper_envelope.emit(out, exit_code, mode)
