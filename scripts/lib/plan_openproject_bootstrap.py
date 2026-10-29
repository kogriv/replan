"""Moved from scripts/plan-openproject-bootstrap.sh (GAP-018). Invoked as `python3 -m plan_openproject_bootstrap`."""
import json
import os
import sys

import yaml
import helper_envelope
from openproject_http import Settings
from openproject_status_map import TRACKING_TO_OPENPROJECT_STATUS

helper_envelope.internal_parser("plan-openproject-bootstrap").parse_args()
settings = Settings.from_env()
project_path = settings.project_path
target_project_id, target_project_name = settings.target_project_id, settings.target_project_name
helper_envelope.guard("plan-openproject-bootstrap", "dry_run")

def load_yaml(relative_path):
    path = os.path.join(project_path, relative_path)
    with open(path) as fh:
        return yaml.safe_load(fh) or {}

tasks = load_yaml("canonical/tasks.yaml").get("tasks") or []
current_plan = load_yaml("canonical/current_plan.yaml").get("current_plan") or {}
demands = load_yaml("canonical/resource_demands.yaml").get("resource_demands") or []
dependencies = load_yaml("canonical/dependencies.yaml").get("dependencies") or []

tasks_by_id = {task.get("id"): task for task in tasks}
demands_by_task = {}
for demand in demands:
    task_id = ((demand.get("task_ref") or {}).get("id") or "")
    if task_id:
        demands_by_task.setdefault(task_id, []).append(demand)

included = [
    item.get("id")
    for item in current_plan.get("included_tasks") or []
    if item.get("kind") == "task" and item.get("id")
]
ordered_ids = [task_id for task_id in included if task_id in tasks_by_id]
remaining_ids = sorted(task_id for task_id in tasks_by_id if task_id not in set(ordered_ids))
ordered_ids.extend(remaining_ids)

status_map = TRACKING_TO_OPENPROJECT_STATUS

warnings = []
work_packages = []
for index, task_id in enumerate(ordered_ids, start=1):
    task = tasks_by_id[task_id]
    tracking_status = task.get("tracking_status") or ""
    openproject_status = status_map.get(tracking_status, "New")
    if tracking_status not in status_map:
        warnings.append({
            "code": "openproject_bootstrap_status_unmapped",
            "task_id": task_id,
            "tracking_status": tracking_status,
            "fallback_status": openproject_status,
        })

    task_demands = demands_by_task.get(task_id, [])
    estimated_hours = 0.0
    estimated_hour_sources = []
    for demand in task_demands:
        if demand.get("unit") == "hours":
            amount = demand.get("remaining_amount", demand.get("planned_amount"))
            if isinstance(amount, (int, float)):
                estimated_hours += float(amount)
                estimated_hour_sources.append(demand.get("id", ""))

    source_refs = [
        ref for ref in task.get("source_refs") or []
        if ref.get("source") == "openproject" and ref.get("object_type") == "work_package"
    ]

    work_packages.append({
        "ordinal": index,
        "replan_task_id": task_id,
        "subject": task.get("title", ""),
        "description": "\n\n".join(part for part in [
            task.get("description", ""),
            f"RePlan task id: `{task_id}`",
        ] if part),
        "replan_tracking_status": tracking_status,
        "suggested_openproject_status": openproject_status,
        "start_date": task.get("planned_start", ""),
        "due_date": task.get("planned_finish", ""),
        "estimated_hours_from_hour_demands": estimated_hours,
        "estimated_hour_demand_ids": [value for value in estimated_hour_sources if value],
        "assignee_hint": task.get("assignee_hint") or {},
        "tags": task.get("tags") or [],
        "existing_public_source_refs": source_refs,
        "bootstrap_action": "create_or_match_by_subject",
    })

dependency_edges = []
for dependency in dependencies:
    predecessor = ((dependency.get("predecessor_ref") or {}).get("id") or "")
    successor = ((dependency.get("successor_ref") or {}).get("id") or "")
    if predecessor and successor:
        dependency_edges.append({
            "replan_dependency_id": dependency.get("id", ""),
            "type": dependency.get("type", ""),
            "predecessor_task_id": predecessor,
            "successor_task_id": successor,
            "status": dependency.get("status", ""),
            "openproject_relation_apply": "defer_until_work_package_ids_exist",
        })

out = {
    "ok": True,
    "command": "plan-openproject-bootstrap",
    "mode": "dry_run",
    "project_path": project_path,
    "target_openproject_project": {
        "identifier": target_project_id,
        "name": target_project_name,
    },
    "counts": {
        "replan_tasks": len(tasks),
        "included_current_plan_tasks": len(included),
        "planned_work_packages": len(work_packages),
        "planned_dependency_relations": len(dependency_edges),
    },
    "status_mapping": status_map,
    "warnings": warnings,
    "work_packages": work_packages,
    "dependency_relations": dependency_edges,
    "operator_next_steps": [
        "Review this dry-run output before mutating OpenProject.",
        "Create or select the target OpenProject project.",
        "Create work packages from work_packages in ordinal order.",
        "Run check-openproject-live-alignment against the target project.",
        "Only after task matching works, decide how to accept source refs or keep private bindings.",
        "Create OpenProject relations after stable work-package ids exist.",
    ],
}

helper_envelope.emit(out, None, "dry_run")
