"""Moved from scripts/check-openproject-drift.sh (GAP-018). Invoked as `python3 -m check_openproject_drift`."""
import json
import os
import sys

import yaml
import helper_envelope
import import_candidates
import openproject_bindings
import openproject_rescheduling
from openproject_http import OpenProjectClient, Settings

parser = helper_envelope.internal_parser("check-openproject-drift")
parser.add_argument("--import-json", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--root-dir", required=True)
args = parser.parse_args()
settings = Settings.from_env(root_dir=args.root_dir)
import_path, tmp_dir = args.import_json, args.tmp_dir
project_path, bindings_path = settings.project_path, settings.bindings_path
openproject_project_id, keep_temp = settings.project_id, settings.keep_temp
helper_envelope.guard("check-openproject-drift")
data = json.load(open(import_path))
result = data.get("result") or {}
warnings = list(data.get("warnings") or result.get("warnings") or [])
errors = data.get("errors") or result.get("errors") or []

def load_yaml(path):
    with open(path) as fh:
        return yaml.safe_load(fh) or {}

def norm(value):
    return " ".join(str(value or "").strip().lower().split())

def as_text(value):
    if value is None:
        return ""
    return str(value)

def scheduling_read_reason(exc):
    try:
        detail = json.loads(str(exc))
    except (TypeError, ValueError):
        detail = {}
    status = detail.get("status")
    if status is not None:
        return f"the request returned HTTP {status}"
    code = detail.get("code", "")
    reasons = {
        "openproject_request_timeout": "the request timed out",
        "openproject_network_error": "a network error occurred",
        "openproject_authentication_failed": "authentication failed",
        "openproject_permission_denied": "permission was denied",
        "openproject_rate_limited": "OpenProject rate-limited the request",
    }
    return reasons.get(code, f"the request failed with {type(exc).__name__}")

tasks_data = load_yaml(os.path.join(project_path, "canonical", "tasks.yaml"))
deps_data = load_yaml(os.path.join(project_path, "canonical", "dependencies.yaml"))
plan_data = load_yaml(os.path.join(project_path, "canonical", "current_plan.yaml"))
planned_task_ids = [
    item.get("id", "")
    for item in ((plan_data.get("current_plan") or {}).get("included_tasks") or [])
    if item.get("kind") == "task" and item.get("id")
]
planned_task_set = set(planned_task_ids)
local_tasks = {
    task.get("id", ""): task
    for task in tasks_data.get("tasks") or []
    if task.get("id", "") in planned_task_set
}
local_deps = [
    dep for dep in deps_data.get("dependencies") or []
    if (dep.get("predecessor_ref") or {}).get("id", "") in planned_task_set
    and (dep.get("successor_ref") or {}).get("id", "") in planned_task_set
]

bindings_loaded = os.path.exists(bindings_path)
task_wp_by_id = {}
dep_relation_by_id = {}
if bindings_loaded:
    bindings_doc = openproject_bindings.load(bindings_path)
    task_wp_by_id, _ = openproject_bindings.task_maps(bindings_doc)
    dep_relation_by_id, _ = openproject_bindings.dependency_maps(bindings_doc)

task_candidates = result.get("task_candidates") or []
dep_candidates = result.get("dependency_candidates") or []
candidate_by_wp = {as_text(item.get("source_id")): item for item in task_candidates if item.get("source_id") is not None}
local_task_by_title = {norm(task.get("title", "")): task_id for task_id, task in local_tasks.items()}
bound_wp_set = set(task_wp_by_id.values())

schedule_manually_by_wp = {
    as_text(candidate.get("source_id")): candidate.get("schedule_manually")
    for candidate in task_candidates
    if isinstance(candidate.get("schedule_manually"), bool)
}
if bound_wp_set - set(schedule_manually_by_wp) and settings.base_url:
    client = OpenProjectClient(
        settings.base_url, settings, settings.host_header, settings.forwarded_proto
    )
    try:
        live_work_packages = openproject_rescheduling.project_work_packages(
            client, openproject_project_id
        )
    except Exception as exc:
        warnings.append(helper_envelope.diagnostic(
            "openproject_scheduling_unknown",
            "OpenProject scheduling state could not be read because "
            + scheduling_read_reason(exc)
            + ".",
            "warning", "openproject_project", openproject_project_id,
        ))
    else:
        schedule_manually_by_wp.update({
            as_text(work_package.get("id")): work_package.get("scheduleManually")
            for work_package in live_work_packages
            if isinstance(work_package.get("scheduleManually"), bool)
        })

task_drifts = []
status_drifts = []
date_drifts = []
unbound_local_tasks = []
missing_live_tasks = []
extra_live_tasks = []

for task_id in planned_task_ids:
    task = local_tasks.get(task_id)
    if not task:
        continue
    wp_id = task_wp_by_id.get(task_id, "")
    if not wp_id:
        unbound_local_tasks.append({"task_id": task_id, "title": task.get("title", "")})
        continue
    candidate = candidate_by_wp.get(wp_id)
    if not candidate:
        missing_live_tasks.append({"task_id": task_id, "work_package_id": wp_id, "title": task.get("title", "")})
        continue
    if norm(candidate.get("title", "")) != norm(task.get("title", "")):
        task_drifts.append({
            "kind": "title_drift",
            "task_id": task_id,
            "work_package_id": wp_id,
            "replan_title": task.get("title", ""),
            "openproject_title": candidate.get("title", ""),
        })
    if as_text(candidate.get("mapped_status")) != as_text(task.get("tracking_status")):
        status_drifts.append({
            "kind": "status_drift",
            "task_id": task_id,
            "work_package_id": wp_id,
            "replan_tracking_status": as_text(task.get("tracking_status")),
            "openproject_source_status": as_text(candidate.get("source_status")),
            "openproject_mapped_status": as_text(candidate.get("mapped_status")),
        })
    if as_text(candidate.get("start_date")) != as_text(task.get("planned_start")):
        date_drifts.append({
            "kind": "start_date_drift",
            "task_id": task_id,
            "work_package_id": wp_id,
            "replan_planned_start": as_text(task.get("planned_start")),
            "openproject_start_date": as_text(candidate.get("start_date")),
        })
    if as_text(candidate.get("due_date")) != as_text(task.get("planned_finish")):
        date_drifts.append({
            "kind": "finish_date_drift",
            "task_id": task_id,
            "work_package_id": wp_id,
            "replan_planned_finish": as_text(task.get("planned_finish")),
            "openproject_due_date": as_text(candidate.get("due_date")),
        })

    if (
        as_text(candidate.get("start_date")) != as_text(task.get("planned_start"))
        or as_text(candidate.get("due_date")) != as_text(task.get("planned_finish"))
    ):
        warnings.append(helper_envelope.diagnostic(
            "openproject_date_drift",
            f"Task {task_id} (work package {wp_id}) dates differ: "
            f"RePlan {as_text(task.get('planned_start'))} to {as_text(task.get('planned_finish'))}; "
            f"OpenProject {as_text(candidate.get('start_date'))} to {as_text(candidate.get('due_date'))}.",
            "warning", "openproject_work_package", wp_id,
        ))

task_id_by_bound_wp = {
    wp_id: task_id for task_id, wp_id in task_wp_by_id.items()
}
for wp_id in sorted(bound_wp_set):
    if schedule_manually_by_wp.get(wp_id) is not False:
        continue
    task_id = task_id_by_bound_wp.get(wp_id, "")
    warnings.append(helper_envelope.diagnostic(
        "openproject_automatic_scheduling",
        f"Task {task_id} (work package {wp_id}) has scheduleManually false in OpenProject.",
        "warning", "openproject_work_package", wp_id,
    ))

for candidate in task_candidates:
    source_id = as_text(candidate.get("source_id"))
    if source_id in bound_wp_set:
        continue
    title_task_id = local_task_by_title.get(norm(candidate.get("title", "")), "")
    extra_live_tasks.append({
        "work_package_id": source_id,
        "title": candidate.get("title", ""),
        "possible_replan_task_id_by_title": title_task_id,
    })

local_dep_tuples = {}
for dep in local_deps:
    dep_id = dep.get("id", "")
    dep_type = dep.get("type", "")
    predecessor = (dep.get("predecessor_ref") or {}).get("id", "")
    successor = (dep.get("successor_ref") or {}).get("id", "")
    local_dep_tuples[import_candidates.dependency_key(dep_type, predecessor, successor)] = dep_id

live_dep_tuples = {}
for dep in dep_candidates:
    key = import_candidates.dependency_key(
        as_text(dep.get("dependency_type")),
        as_text(dep.get("predecessor_task_id")),
        as_text(dep.get("successor_task_id")),
    )
    live_dep_tuples[key] = dep

unbound_local_dependencies = []
missing_live_dependencies = []
extra_live_dependencies = []
relation_binding_mismatches = []

for dep in local_deps:
    dep_id = dep.get("id", "")
    dep_type = dep.get("type", "")
    predecessor = (dep.get("predecessor_ref") or {}).get("id", "")
    successor = (dep.get("successor_ref") or {}).get("id", "")
    key = import_candidates.dependency_key(dep_type, predecessor, successor)
    if not dep_relation_by_id.get(dep_id, ""):
        unbound_local_dependencies.append({
            "dependency_id": dep_id,
            "dependency_type": dep_type,
            "predecessor_task_id": predecessor,
            "successor_task_id": successor,
        })
    candidate = live_dep_tuples.get(key)
    if not candidate:
        missing_live_dependencies.append({
            "dependency_id": dep_id,
            "dependency_type": dep_type,
            "predecessor_task_id": predecessor,
            "successor_task_id": successor,
        })
        continue
    bound_relation_id = dep_relation_by_id.get(dep_id, "")
    source_id = as_text(candidate.get("source_id"))
    if bound_relation_id and source_id and bound_relation_id != source_id:
        relation_binding_mismatches.append({
            "dependency_id": dep_id,
            "bound_relation_id": bound_relation_id,
            "live_relation_id": source_id,
            "dependency_type": dep_type,
            "predecessor_task_id": predecessor,
            "successor_task_id": successor,
        })

for key, candidate in live_dep_tuples.items():
    if key not in local_dep_tuples:
        extra_live_dependencies.append({
            "relation_id": as_text(candidate.get("source_id")),
            "relation_type": as_text(candidate.get("relation_type")),
            "dependency_type": key[0],
            "predecessor_task_id": key[1],
            "successor_task_id": key[2],
            "accept_status": as_text(candidate.get("accept_status")),
        })

blocking = (
    unbound_local_tasks
    or missing_live_tasks
    or extra_live_tasks
    or task_drifts
    or unbound_local_dependencies
    or missing_live_dependencies
    or extra_live_dependencies
    or relation_binding_mismatches
    or errors
)
status_warnings = [
    warning for warning in warnings
    if warning.get("code") != "openproject_scheduling_unknown"
]
soft = status_drifts or date_drifts or status_warnings
if blocking:
    drift_status = "drift"
elif soft:
    drift_status = "soft_drift"
else:
    drift_status = "aligned"

decision = {
    "aligned": "OpenProject and RePlan match for the current task and dependency skeleton. Proceed with planning or resource calibration.",
    "soft_drift": "Task identity and dependency skeleton match, but status/date differences should be reviewed before accepting or pushing changes.",
    "drift": "Identity, task or dependency drift exists. Review report before mutating OpenProject or accepting imports.",
}[drift_status]
next_actions = {
    "aligned": [
        "Proceed with the next operator-loop step.",
        "Re-run this drift report before any OpenProject or RePlan mutation.",
    ],
    "soft_drift": [
        "Review status and date drift before mutating OpenProject or accepting imports.",
        "For date drift, review scripts/push-openproject-date-drift.sh as a dry run, then apply it if RePlan dates should win.",
        "For automatic scheduling, restore the task's current RePlan dates through scripts/openproject-work-package-toolkit.sh work-package update-dates.",
        "Keep task and dependency identity bindings unchanged unless a hard drift appears.",
    ],
    "drift": [
        "Do not mutate OpenProject or accept imports until identity, task or dependency drift is reviewed.",
        "Use the private binding layer and live OpenProject project selection to determine whether the correct project is being checked.",
    ],
}[drift_status]

report = {
    "ok": data.get("ok") is True and drift_status in {"aligned", "soft_drift"},
    "command": "check-openproject-drift",
    "mode": "read_only",
    "drift_status": drift_status,
    "project_path": project_path,
    "target_openproject_project": {
        "identifier": openproject_project_id,
    },
    "private_binding_layer": {
        "path": bindings_path,
        "loaded": bindings_loaded,
        "task_bindings": len(task_wp_by_id),
        "dependency_bindings": len(dep_relation_by_id),
    },
    "field_policy": {
        "task_identity": {
            "owner": "openproject_private_binding",
            "drift_class": "hard",
            "rule": "A bound RePlan task must resolve to exactly one live OpenProject work package.",
        },
        "task_title": {
            "owner": "openproject_for_task_management",
            "drift_class": "hard",
            "rule": "Title differences can make the operator inspect the wrong task and must be reviewed before mutation.",
        },
        "dependency_identity": {
            "owner": "openproject_private_binding",
            "drift_class": "hard",
            "rule": "A canonical RePlan dependency must have a matching live OpenProject relation tuple when relation bindings exist.",
        },
        "tracking_status": {
            "owner": "openproject_source_status_with_explicit_replan_accept",
            "drift_class": "soft",
            "rule": "OpenProject status is a source fact; RePlan tracking_status changes only through explicit acceptance.",
        },
        "planned_dates": {
            "owner": "replan_resource_plan_with_reviewed_openproject_writeback",
            "drift_class": "soft",
            "rule": "Bound work packages should be manually scheduled; date drift is reviewed and pushed from RePlan or explicitly accepted.",
        },
    },
    "counts": {
        "local_tasks": len(planned_task_ids),
        "live_task_candidates": len(task_candidates),
        "local_dependencies": len(local_deps),
        "live_dependency_candidates": len(dep_candidates),
        "unbound_local_tasks": len(unbound_local_tasks),
        "missing_live_tasks": len(missing_live_tasks),
        "extra_live_tasks": len(extra_live_tasks),
        "title_drifts": len(task_drifts),
        "status_drifts": len(status_drifts),
        "date_drifts": len(date_drifts),
        "unbound_local_dependencies": len(unbound_local_dependencies),
        "missing_live_dependencies": len(missing_live_dependencies),
        "extra_live_dependencies": len(extra_live_dependencies),
        "relation_binding_mismatches": len(relation_binding_mismatches),
        "time_entry_candidates": len(result.get("time_entry_candidates") or []),
        "resource_candidates": len(result.get("resource_candidates") or []),
        "resource_demand_candidates": len(result.get("resource_demand_candidates") or []),
    },
    "warnings": warnings,
    "warning_codes": sorted({w.get("code", "") for w in warnings if w.get("code")}),
    "error_codes": sorted({e.get("code", "") for e in errors if e.get("code")}),
    "drift": {
        "unbound_local_tasks": unbound_local_tasks[:20],
        "missing_live_tasks": missing_live_tasks[:20],
        "extra_live_tasks": extra_live_tasks[:20],
        "task_drifts": task_drifts[:20],
        "status_drifts": status_drifts[:20],
        "date_drifts": date_drifts[:20],
        "unbound_local_dependencies": unbound_local_dependencies[:20],
        "missing_live_dependencies": missing_live_dependencies[:20],
        "extra_live_dependencies": extra_live_dependencies[:20],
        "relation_binding_mismatches": relation_binding_mismatches[:20],
    },
    "suggested_operator_decision": decision,
    "suggested_next_actions": next_actions,
    "temp_retained": bool(keep_temp),
}
if keep_temp:
    report["outputs_dir"] = tmp_dir
helper_envelope.emit(report, None)
