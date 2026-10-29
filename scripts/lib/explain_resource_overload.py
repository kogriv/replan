"""Moved from scripts/explain-resource-overload.sh (GAP-018). Invoked as `python3 -m explain_resource_overload`.

Explains one resource's weekly load demand by demand. The rows and each
demand's part come from `replan workload --demands`; this helper adds the
task context and the questions for the operator.
"""
import datetime as dt
import json
import os
import sys

import yaml

import replan_workload
import helper_envelope
from openproject_http import Settings

parser = helper_envelope.internal_parser("explain-resource-overload")
parser.add_argument("--replan-bin", required=True)
args = parser.parse_args()
project_path = Settings.from_env().project_path
resource_id = os.environ.get("REPLAN_RESOURCE_ID", "")
unit = os.environ.get("REPLAN_UNIT", "")
binary = args.replan_bin
helper_envelope.guard("explain-resource-overload")

def load_yaml(rel):
    with open(os.path.join(project_path, rel)) as fh:
        return yaml.safe_load(fh) or {}

def parse_date(value):
    if not value:
        return None
    try:
        return dt.date.fromisoformat(str(value))
    except ValueError:
        return None

tasks = load_yaml("canonical/tasks.yaml").get("tasks") or []
demands = load_yaml("canonical/resource_demands.yaml").get("resource_demands") or []
capacities = load_yaml("canonical/capacities.yaml").get("capacities") or []
resources = load_yaml("canonical/resources.yaml").get("resources") or []

task_by_id = {task.get("id", ""): task for task in tasks if task.get("id", "")}
demand_by_id = {demand.get("id", ""): demand for demand in demands if demand.get("id", "")}
resource_by_id = {resource.get("id", ""): resource for resource in resources if resource.get("id", "")}
capacity_ids = [
    item.get("id", "") for item in capacities
    if (item.get("resource_ref") or {}).get("id") == resource_id and item.get("unit") == unit
]

skipped = []
for demand in demands:
    target_id = (demand.get("resource_ref") or {}).get("id", "") or (demand.get("resource_pool_ref") or {}).get("id", "")
    if target_id != resource_id or demand.get("unit") != unit or demand.get("status") != "active":
        continue
    task_id = (demand.get("task_ref") or {}).get("id", "")
    task = task_by_id.get(task_id)
    if not task:
        skipped.append({"resource_demand_id": demand.get("id", ""), "reason": "missing_task", "task_id": task_id})
        continue
    start = parse_date(task.get("planned_start"))
    finish = parse_date(task.get("planned_finish"))
    if not start or not finish or finish < start:
        skipped.append({"resource_demand_id": demand.get("id", ""), "reason": "invalid_task_window", "task_id": task_id})

def demand_entry(part):
    demand = demand_by_id.get(part.get("resource_demand_id", ""), {})
    task = task_by_id.get(part.get("task_id", ""), {})
    week_load = part.get("reserved") or part.get("remaining") or 0.0
    return {
        "resource_demand_id": part.get("resource_demand_id", ""),
        "task_id": part.get("task_id", ""),
        "task_title": task.get("title", ""),
        "task_status": task.get("tracking_status", ""),
        "task_window": {"start": str(task.get("planned_start", "")), "finish": str(task.get("planned_finish", ""))},
        "demand_status": part.get("status", ""),
        "planned_amount": float(demand.get("planned_amount") or 0),
        "remaining_amount": demand.get("remaining_amount"),
        "share_of_demand_in_week": part.get("share", 0.0),
        "concurrent": bool(part.get("concurrent")),
        "week_load": week_load,
        "suggested_operator_questions": [
            f"Is this demand on {resource_id} still required in this week?",
            "Can it move to another week without breaking the task plan?",
            f"Should the capacity of {resource_id} in {unit} be recalibrated?",
        ],
    }

workload_error = ""
weekly_rows = []
try:
    for row in replan_workload.weekly_rows(binary, project_path, replan_workload.plan_weeks(project_path), resource_id, unit):
        if row.get("resource_id") != resource_id or row.get("unit") != unit:
            continue
        parts = [demand_entry(part) for part in row.get("demands") or []]
        parts = sorted((p for p in parts if p["week_load"] > 0), key=lambda item: (-item["week_load"], item["resource_demand_id"]))
        weekly_rows.append({
            "week": row["week"],
            "week_start": row["week_start"],
            "resource_id": resource_id,
            "resource_name": row.get("resource_name", ""),
            "unit": unit,
            "capacity": row.get("capacity", 0.0),
            "planning_load": row["planning_load"],
            "overload": row.get("overload", 0.0),
            "is_overloaded": row.get("overload", 0.0) > 0,
            "active_demand_count": len(parts),
            "demands": parts,
        })
except replan_workload.WorkloadError as exc:
    workload_error = str(exc)

overloaded = [row for row in weekly_rows if row["is_overloaded"]]
out = {
    "ok": not workload_error,
    "command": "explain-resource-overload",
    "project_path": project_path,
    "resource_id": resource_id,
    "unit": unit,
    "resource_name": (resource_by_id.get(resource_id) or {}).get("name", ""),
    "capacity_ids": capacity_ids,
    "overload_status": "error" if workload_error else ("overloaded" if overloaded else "aligned"),
    "policy": {
        "weekly_load": "replan workload --week --demands: reserved amount when set, otherwise remaining, prorated over the demand window and working days; concurrent slots at their daily peak",
    },
    "counts": {
        "weekly_rows": len(weekly_rows),
        "overloaded_weekly_rows": len(overloaded),
        "skipped_demands": len(skipped),
    },
    "overloaded_weekly_rows": overloaded,
    "weekly_rows": weekly_rows,
    "skipped_demands": skipped,
    "errors": [{"code": "workload_failed", "message": workload_error}] if workload_error else [],
    "suggested_operator_decision": (
        "replan workload failed; fix the project so that it loads, then rerun this explainer."
        if workload_error else
        "Review the overloaded weeks and decide whether to cancel or defer specific demands or recalibrate the resource's capacity."
        if overloaded else
        "No overload found for the selected resource and unit."
    ),
}
helper_envelope.emit(out, helper_envelope.EXIT["general"] if workload_error else None)
