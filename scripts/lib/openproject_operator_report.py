"""Moved from scripts/openproject-operator-report.sh (GAP-018). Invoked as `python3 -m openproject_operator_report`."""
import json
import os
import sys

import yaml
import helper_envelope
import import_candidates
import openproject_bindings
from openproject_http import Settings

parser = helper_envelope.internal_parser("openproject operator report")
parser.add_argument("--import-json", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--root-dir", required=True)
args = parser.parse_args()
settings = Settings.from_env(root_dir=args.root_dir)
import_path, tmp_dir = args.import_json, args.tmp_dir
project_path, bindings_path = settings.project_path, settings.bindings_path
profile, openproject_project_id = settings.profile, settings.project_id
from_file, keep_temp = settings.from_file, settings.keep_temp
helper_envelope.guard("openproject operator report")

def load_yaml(path):
    with open(path) as fh:
        return yaml.safe_load(fh) or {}

def norm(value):
    return " ".join(str(value or "").strip().lower().split())

def as_text(value):
    if value is None:
        return ""
    return str(value)

data = json.load(open(import_path))
result = data.get("result") or {}
warnings = data.get("warnings") or result.get("warnings") or []
errors = data.get("errors") or result.get("errors") or []

tasks_doc = load_yaml(os.path.join(project_path, "canonical", "tasks.yaml"))
deps_doc = load_yaml(os.path.join(project_path, "canonical", "dependencies.yaml"))
plan_doc = load_yaml(os.path.join(project_path, "canonical", "current_plan.yaml"))
resources_doc = load_yaml(os.path.join(project_path, "canonical", "resources.yaml"))
demands_doc = load_yaml(os.path.join(project_path, "canonical", "resource_demands.yaml"))

planned_task_ids = [
    item.get("id", "")
    for item in ((plan_doc.get("current_plan") or {}).get("included_tasks") or [])
    if item.get("kind") == "task" and item.get("id")
]
planned_task_set = set(planned_task_ids)
tasks_by_id = {
    task.get("id", ""): task
    for task in tasks_doc.get("tasks") or []
    if task.get("id", "") in planned_task_set
}
deps = [
    dep for dep in deps_doc.get("dependencies") or []
    if (dep.get("predecessor_ref") or {}).get("id", "") in planned_task_set
    and (dep.get("successor_ref") or {}).get("id", "") in planned_task_set
]

task_wp_by_id = {}
dep_relation_by_id = {}
binding_errors = []
binding_project = {}
bindings_loaded = os.path.exists(bindings_path)
if bindings_loaded:
    bindings_doc = openproject_bindings.load(bindings_path)
    binding_project = openproject_bindings.project(bindings_doc)
    task_wp_by_id, _ = openproject_bindings.task_maps(
        bindings_doc,
        duplicate_task_errors=binding_errors,
        duplicate_work_package_errors=binding_errors,
    )
    dep_relation_by_id, _ = openproject_bindings.dependency_maps(bindings_doc)
else:
    binding_errors.append({"kind": "missing_private_bindings", "path": bindings_path})

task_candidates = result.get("task_candidates") or []
dep_candidates = result.get("dependency_candidates") or []
candidate_by_wp = {as_text(item.get("source_id")): item for item in task_candidates}
candidate_by_task = {
    as_text(item.get("matched_task_id")): item
    for item in task_candidates
    if item.get("matched_task_id")
}
task_by_title = {norm(task.get("title", "")): task_id for task_id, task in tasks_by_id.items()}
bound_wp_ids = set(task_wp_by_id.values())

mapped_tasks = []
unbound_local_tasks = []
missing_live_tasks = []
extra_live_work_packages = []
title_drifts = []
status_drifts = []
date_drifts = []

for task_id in planned_task_ids:
    task = tasks_by_id.get(task_id)
    if not task:
        continue
    wp_id = task_wp_by_id.get(task_id, "")
    candidate = candidate_by_wp.get(wp_id) if wp_id else candidate_by_task.get(task_id)
    if not wp_id:
        unbound_local_tasks.append({"task_id": task_id, "title": task.get("title", "")})
        continue
    if not candidate:
        missing_live_tasks.append({"task_id": task_id, "work_package_id": wp_id, "title": task.get("title", "")})
        continue
    row = {
        "task_id": task_id,
        "work_package_id": wp_id,
        "title": task.get("title", ""),
        "replan_tracking_status": as_text(task.get("tracking_status")),
        "openproject_source_status": as_text(candidate.get("source_status")),
        "openproject_mapped_status": as_text(candidate.get("mapped_status")),
        "replan_planned_start": as_text(task.get("planned_start")),
        "openproject_start_date": as_text(candidate.get("start_date")),
        "replan_planned_finish": as_text(task.get("planned_finish")),
        "openproject_due_date": as_text(candidate.get("due_date")),
        "match_reason": as_text(candidate.get("match_reason")),
    }
    mapped_tasks.append(row)
    if norm(candidate.get("title", "")) != norm(task.get("title", "")):
        title_drifts.append({**row, "openproject_title": candidate.get("title", "")})
    if as_text(candidate.get("mapped_status")) != as_text(task.get("tracking_status")):
        status_drifts.append(row)
    if as_text(candidate.get("start_date")) != as_text(task.get("planned_start")):
        date_drifts.append({**row, "field": "start"})
    if as_text(candidate.get("due_date")) != as_text(task.get("planned_finish")):
        date_drifts.append({**row, "field": "finish"})

for candidate in task_candidates:
    wp_id = as_text(candidate.get("source_id"))
    if wp_id in bound_wp_ids:
        continue
    extra_live_work_packages.append({
        "work_package_id": wp_id,
        "title": candidate.get("title", ""),
        "source_status": candidate.get("source_status", ""),
        "mapped_status": candidate.get("mapped_status", ""),
        "possible_replan_task_id_by_title": task_by_title.get(norm(candidate.get("title", "")), ""),
    })

local_dep_tuples = {}
for dep in deps:
    key = (
        as_text(dep.get("type")),
        as_text((dep.get("predecessor_ref") or {}).get("id")),
        as_text((dep.get("successor_ref") or {}).get("id")),
    )
    local_dep_tuples[key] = dep.get("id", "")

live_dep_tuples = {}
for candidate in dep_candidates:
    key = (
        as_text(candidate.get("dependency_type")),
        as_text(candidate.get("predecessor_task_id")),
        as_text(candidate.get("successor_task_id")),
    )
    live_dep_tuples[key] = candidate

relation_rows = []
unbound_local_dependencies = []
missing_live_dependencies = []
extra_live_relations = []
relation_binding_mismatches = []
for dep in deps:
    dep_id = dep.get("id", "")
    key = (
        as_text(dep.get("type")),
        as_text((dep.get("predecessor_ref") or {}).get("id")),
        as_text((dep.get("successor_ref") or {}).get("id")),
    )
    candidate = live_dep_tuples.get(key)
    bound_relation_id = dep_relation_by_id.get(dep_id, "")
    if not bound_relation_id:
        unbound_local_dependencies.append({
            "dependency_id": dep_id,
            "dependency_type": key[0],
            "predecessor_task_id": key[1],
            "successor_task_id": key[2],
        })
    if not candidate:
        missing_live_dependencies.append({
            "dependency_id": dep_id,
            "dependency_type": key[0],
            "predecessor_task_id": key[1],
            "successor_task_id": key[2],
            "bound_relation_id": bound_relation_id,
        })
        continue
    live_relation_id = as_text(candidate.get("source_id"))
    predecessor_source_id, successor_source_id = import_candidates.dependency_source_ids(candidate)
    row = {
        "dependency_id": dep_id,
        "dependency_type": key[0],
        "bound_relation_id": bound_relation_id,
        "live_relation_id": live_relation_id,
        "relation_type": as_text(candidate.get("relation_type")),
        "direction": {
            "predecessor_task_id": key[1],
            "successor_task_id": key[2],
            "predecessor_source_id": predecessor_source_id,
            "successor_source_id": successor_source_id,
            "openproject_from_work_package_id": as_text(candidate.get("from_work_package_id")),
            "openproject_to_work_package_id": as_text(candidate.get("to_work_package_id")),
        },
    }
    relation_rows.append(row)
    if bound_relation_id and live_relation_id and bound_relation_id != live_relation_id:
        relation_binding_mismatches.append(row)

for key, candidate in live_dep_tuples.items():
    if key not in local_dep_tuples:
        extra_live_relations.append({
            "relation_id": as_text(candidate.get("source_id")),
            "relation_type": as_text(candidate.get("relation_type")),
            "dependency_type": key[0],
            "predecessor_task_id": key[1],
            "successor_task_id": key[2],
            "accept_status": as_text(candidate.get("accept_status")),
        })

demands = demands_doc.get("resource_demands") or []
resources = resources_doc.get("resources") or []
active_demand_ids = [d.get("id", "") for d in demands if as_text(d.get("status")) == "active"]
resource_classes = sorted({as_text(r.get("class") or r.get("resource_class")) for r in resources if r.get("class") or r.get("resource_class")})

relation_binding_enforced = len(dep_relation_by_id) > 0
relation_hard_drifts = []
if relation_binding_enforced:
    relation_hard_drifts = (
        unbound_local_dependencies
        + missing_live_dependencies
        + extra_live_relations
        + relation_binding_mismatches
    )

hard_drifts = (
    binding_errors
    + unbound_local_tasks
    + missing_live_tasks
    + extra_live_work_packages
    + title_drifts
    + relation_hard_drifts
    + errors
)
soft_drifts = status_drifts + date_drifts + warnings
if hard_drifts:
    report_status = "drift"
elif soft_drifts:
    report_status = "soft_drift"
else:
    report_status = "aligned"

decision = {
    "aligned": "OpenProject and RePlan match at the task and relation skeleton level. Proceed with the next operator-loop step.",
    "soft_drift": "Task identity and relation skeleton are usable, but status or date differences need an explicit accept or push decision.",
    "drift": "Hard OpenProject/RePlan drift exists. Do not apply write helpers before the listed identity, binding or relation problems are reviewed.",
}[report_status]

out = {
    "ok": data.get("ok") is True and report_status in {"aligned", "soft_drift"},
    "command": "openproject operator report",
    "mode": "read_only",
    "operator_report_status": report_status,
    "project_path": project_path,
    "source": {
        "mode": as_text((result.get("snapshot") or {}).get("source_mode")),
        "from_file": from_file,
        "profile": profile,
        "base_url_configured": bool(os.environ.get("REPLAN_OPENPROJECT_BASE_URL", "")),
        "token_env": os.environ.get("REPLAN_OPENPROJECT_TOKEN_ENV", "OPENPROJECT_TOKEN"),
    },
    "target_openproject_project": {
        "identifier": openproject_project_id,
        "binding_project_id": as_text(binding_project.get("project_id")),
    },
    "private_binding_layer": {
        "path": bindings_path,
        "loaded": bindings_loaded,
        "task_bindings": len(task_wp_by_id),
        "dependency_bindings": len(dep_relation_by_id),
        "binding_errors": binding_errors,
    },
    "counts": {
        "planned_tasks": len(planned_task_ids),
        "mapped_tasks": len(mapped_tasks),
        "live_task_candidates": len(task_candidates),
        "unbound_local_tasks": len(unbound_local_tasks),
        "missing_live_tasks": len(missing_live_tasks),
        "extra_live_work_packages": len(extra_live_work_packages),
        "title_drifts": len(title_drifts),
        "status_drifts": len(status_drifts),
        "date_drifts": len(date_drifts),
        "local_dependencies": len(deps),
        "mapped_relations": len(relation_rows),
        "live_relation_candidates": len(dep_candidates),
        "unbound_local_dependencies": len(unbound_local_dependencies),
        "missing_live_dependencies": len(missing_live_dependencies),
        "extra_live_relations": len(extra_live_relations),
        "relation_binding_mismatches": len(relation_binding_mismatches),
        "time_entry_candidates": len(result.get("time_entry_candidates") or []),
        "resource_candidates": len(result.get("resource_candidates") or []),
        "resource_demand_candidates": len(result.get("resource_demand_candidates") or []),
        "replan_resources": len(resources),
        "replan_resource_classes": len(resource_classes),
        "active_resource_demands": len(active_demand_ids),
    },
    "field_policy": {
        "task_identity": {"owner": "private_binding", "drift_class": "hard"},
        "task_status": {"owner": "openproject_with_explicit_replan_accept_or_replan_push", "drift_class": "soft"},
        "task_dates": {"owner": "openproject_schedule_until_resource_plan_push", "drift_class": "soft"},
        "relations": {"owner": "openproject_private_binding_and_dependency_tuple", "drift_class": "hard"},
        "resource_demands": {"owner": "replan", "drift_class": "resource_planning"},
    },
    "relation_binding_policy": {
        "enforced": relation_binding_enforced,
        "rule": "Canonical dependency relation drift blocks apply only when private relation bindings exist for the project. Without relation bindings, relation output is diagnostic and relation ensure must still run its own dry-run before apply.",
    },
    "mapped_tasks": mapped_tasks,
    "relations": relation_rows,
    "drift": {
        "unbound_local_tasks": unbound_local_tasks,
        "missing_live_tasks": missing_live_tasks,
        "extra_live_work_packages": extra_live_work_packages,
        "title_drifts": title_drifts,
        "status_drifts": status_drifts,
        "date_drifts": date_drifts,
        "unbound_local_dependencies": unbound_local_dependencies,
        "missing_live_dependencies": missing_live_dependencies,
        "extra_live_relations": extra_live_relations,
        "relation_binding_mismatches": relation_binding_mismatches,
    },
    "helper_contract": {
        "dry_run_default": True,
        "apply_gate": "REPLAN_APPLY=1 for current scripts; future CLI should expose --apply.",
        "write_helpers": [
            "scripts/apply-openproject-bootstrap.sh",
            "scripts/apply-openproject-relations.sh",
            "scripts/push-openproject-date-drift.sh",
            "scripts/push-openproject-status-drift.sh",
            "scripts/openproject-work-package-toolkit.sh",
        ],
        "normal_path_excludes": [
            "delete work package",
            "delete relation",
            "delete project",
            "delete and recreate as refactor shortcut",
        ],
    },
    "warning_codes": sorted({w.get("code", "") for w in warnings if w.get("code")}),
    "error_codes": sorted({e.get("code", "") for e in errors if e.get("code")}),
    "suggested_operator_decision": decision,
    "suggested_next_actions": [
        "Review this read-only report before any OpenProject mutation.",
        "Run scripts/check-openproject-drift.sh for the strict drift gate before write helpers.",
        "Use write helpers in dry-run mode first, then apply only the reviewed action.",
        "Run scripts/check-resource-schedule-drift.sh after task/date/status changes.",
    ],
    "temp_retained": bool(keep_temp),
}
if keep_temp:
    out["outputs_dir"] = tmp_dir

helper_envelope.emit(out, None)
