"""Moved from scripts/check-resource-schedule-drift.sh (GAP-018). Invoked as `python3 -m check_resource_schedule_drift`.

The weekly rows come from `replan workload`; this helper adds the checks of
the demands themselves.
"""
import datetime as dt
import json
import os
import sys
from collections import Counter

import yaml

import replan_workload
import helper_envelope
from openproject_http import Settings

parser = helper_envelope.internal_parser("check-resource-schedule-drift")
parser.add_argument("--replan-bin", required=True)
args = parser.parse_args()
project_path, binary = Settings.from_env().project_path, args.replan_bin
helper_envelope.guard("check-resource-schedule-drift")

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
current_plan = load_yaml("canonical/current_plan.yaml").get("current_plan") or {}

task_by_id = {task.get("id", ""): task for task in tasks if task.get("id", "")}
resource_by_id = {resource.get("id", ""): resource for resource in resources if resource.get("id", "")}
current_task_ids = {
    item.get("id", "")
    for item in current_plan.get("included_tasks") or []
    if item.get("kind") == "task" and item.get("id")
}

capacity_meta = {}
for capacity in capacities:
    resource_id = (capacity.get("resource_ref") or {}).get("id", "")
    unit = capacity.get("unit", "")
    if not resource_id or not unit:
        continue
    key = (resource_id, unit)
    capacity_meta[key] = {
        "capacity_id": capacity.get("id", ""),
        "type": capacity.get("type", ""),
        "period": capacity.get("period", ""),
    }

hard_drifts = []
calibration_reviews = []
timing_counts = Counter()
status_counts = Counter()
task_status_counts = Counter()

for demand in demands:
    demand_id = demand.get("id", "")
    status = demand.get("status", "") or "<missing>"
    status_counts[status] += 1
    task_id = (demand.get("task_ref") or {}).get("id", "")
    task = task_by_id.get(task_id)
    if not task:
        hard_drifts.append({
            "kind": "demand_task_missing",
            "resource_demand_id": demand_id,
            "task_id": task_id,
        })
        continue
    task_status = task.get("tracking_status", "") or "<missing>"
    task_status_counts[task_status] += 1
    task_start = parse_date(task.get("planned_start"))
    task_finish = parse_date(task.get("planned_finish"))
    if not task_start or not task_finish:
        hard_drifts.append({
            "kind": "task_date_missing_or_invalid",
            "resource_demand_id": demand_id,
            "task_id": task_id,
            "planned_start": task.get("planned_start", ""),
            "planned_finish": task.get("planned_finish", ""),
        })
        continue
    if task_finish < task_start:
        hard_drifts.append({
            "kind": "task_date_reversed",
            "resource_demand_id": demand_id,
            "task_id": task_id,
            "planned_start": task.get("planned_start", ""),
            "planned_finish": task.get("planned_finish", ""),
        })
        continue

    timing = demand.get("timing") or {}
    explicit_start = parse_date(timing.get("preferred_start") or timing.get("earliest_start"))
    explicit_finish = parse_date(timing.get("preferred_finish") or timing.get("latest_finish"))
    if explicit_start or explicit_finish:
        timing_counts["explicit"] += 1
        demand_start = explicit_start or explicit_finish
        demand_finish = explicit_finish or explicit_start
        if demand_finish < demand_start:
            hard_drifts.append({
                "kind": "demand_timing_reversed",
                "resource_demand_id": demand_id,
                "task_id": task_id,
                "start": str(demand_start),
                "finish": str(demand_finish),
            })
            continue
        if demand_start < task_start or demand_finish > task_finish:
            calibration_reviews.append({
                "kind": "demand_timing_outside_task_window",
                "resource_demand_id": demand_id,
                "task_id": task_id,
                "task_window": {"start": str(task_start), "finish": str(task_finish)},
                "demand_window": {"start": str(demand_start), "finish": str(demand_finish)},
                "severity": "review",
            })
    else:
        timing_counts["inherits_task_window"] += 1
        demand_start = task_start
        demand_finish = task_finish

    target_id = (demand.get("resource_ref") or {}).get("id", "") or (demand.get("resource_pool_ref") or {}).get("id", "")
    unit = demand.get("unit", "")
    if not target_id or not unit:
        hard_drifts.append({
            "kind": "demand_resource_or_unit_missing",
            "resource_demand_id": demand_id,
            "task_id": task_id,
            "resource_id": target_id,
            "unit": unit,
        })
        continue

    if status == "active" and task_status == "accepted":
        calibration_reviews.append({
            "kind": "active_demand_on_accepted_task",
            "resource_demand_id": demand_id,
            "task_id": task_id,
            "resource_id": target_id,
            "unit": unit,
            "severity": "review",
        })
    if status == "active" and "remaining_amount" not in demand:
        calibration_reviews.append({
            "kind": "active_demand_without_remaining_amount",
            "resource_demand_id": demand_id,
            "task_id": task_id,
            "resource_id": target_id,
            "unit": unit,
            "planned_amount": demand.get("planned_amount", 0),
            "severity": "review",
        })
    if status in {"consumed", "cancelled"} and task_status not in {"accepted", "cancelled"}:
        calibration_reviews.append({
            "kind": "closed_demand_on_open_task",
            "resource_demand_id": demand_id,
            "task_id": task_id,
            "task_status": task_status,
            "demand_status": status,
            "resource_id": target_id,
            "unit": unit,
            "severity": "review",
        })

    planned = demand.get("planned_amount")
    remaining = demand.get("remaining_amount")
    if status == "active" and not isinstance(remaining, (int, float)) and not isinstance(planned, (int, float)):
        calibration_reviews.append({
            "kind": "active_demand_missing_amount",
            "resource_demand_id": demand_id,
            "task_id": task_id,
            "resource_id": target_id,
            "unit": unit,
            "severity": "review",
        })

workload_error = ""
weekly_rows = []
try:
    for row in replan_workload.weekly_rows(binary, project_path, replan_workload.plan_weeks(project_path)):
        demands_in_row = row.pop("demands", None) or []
        row["active_demands"] = len([d for d in demands_in_row if d.get("status") == "active"])
        row["demands"] = len(demands_in_row)
        row["capacity_meta"] = capacity_meta.get((row["resource_id"], row["unit"]), {})
        weekly_rows.append(row)
except replan_workload.WorkloadError as exc:
    workload_error = str(exc)

overloaded_rows = [row for row in weekly_rows if row["overload"] > 0]
missing_capacity_rows = [
    row for row in weekly_rows
    if row["capacity"] == 0 and row["planning_load"] > 0
]

if workload_error:
    status = "error"
elif hard_drifts:
    status = "drift"
elif overloaded_rows or missing_capacity_rows or calibration_reviews:
    status = "needs_calibration"
else:
    status = "aligned"

review_counts = Counter(item["kind"] for item in calibration_reviews)
out = {
    "ok": status not in {"drift", "error"},
    "command": "check-resource-schedule-drift",
    "resource_schedule_status": status,
    "project_path": project_path,
    "policy": {
        "demand_without_timing": "inherit_task_planned_window",
        "weekly_load": "replan workload --week for every week of the plan: prorated over the demand window and the resource's working days, concurrent slots at their daily peak",
        "planning_load": "reserved amount when set, otherwise remaining amount, as replan workload weighs it against capacity",
        "hard_drift": [
            "missing task",
            "invalid task dates",
            "reversed task or demand timing",
            "missing demand resource/unit",
        ],
        "calibration_review": [
            "active demand on accepted task",
            "active demand without remaining_amount",
            "closed demand on open task",
            "resource weekly overload",
            "missing capacity for active planning load",
        ],
    },
    "counts": {
        "tasks": len(tasks),
        "current_plan_tasks": len(current_task_ids),
        "resource_demands": len(demands),
        "demands_with_explicit_timing": timing_counts["explicit"],
        "demands_inheriting_task_window": timing_counts["inherits_task_window"],
        "hard_drifts": len(hard_drifts),
        "calibration_reviews": len(calibration_reviews),
        "weekly_rows": len(weekly_rows),
        "overloaded_weekly_rows": len(overloaded_rows),
        "missing_capacity_weekly_rows": len(missing_capacity_rows),
    },
    "demand_status_counts": dict(sorted(status_counts.items())),
    "task_status_counts_for_demands": dict(sorted(task_status_counts.items())),
    "calibration_review_counts": dict(sorted(review_counts.items())),
    "hard_drifts": hard_drifts[:50],
    "calibration_reviews": calibration_reviews[:80],
    "overloaded_weekly_rows": overloaded_rows[:50],
    "missing_capacity_weekly_rows": missing_capacity_rows[:50],
    "weekly_rows": weekly_rows,
    "errors": [{"code": "workload_failed", "message": workload_error}] if workload_error else [],
    "suggested_operator_decision": {
        "aligned": "Resource demands are consistent with task schedule and weekly capacity at the current coarse-grained level.",
        "needs_calibration": "Task schedule is usable, but resource demands/capacities need operator calibration before relying on workload numbers.",
        "drift": "Fix hard resource schedule drift before mutating resource demands or using workload as planning evidence.",
        "error": "replan workload failed; fix the project so that it loads, then rerun this check.",
    }[status],
    "suggested_next_actions": [
        "Review active demands attached to accepted tasks and decide whether to mark them consumed/cancelled or reopen the task.",
        "Review active demands without remaining_amount and add explicit remaining work where the task is still active.",
        "Review overloaded weekly rows before committing to the current OpenProject schedule.",
        "Decide whether Codex subscription pressure needs a separate quota tracker beyond agent_hours.",
    ] if status != "aligned" else [
        "Proceed to resource planning dashboard/report review.",
    ],
}
helper_envelope.emit(out, helper_envelope.EXIT["general"] if workload_error else None)
