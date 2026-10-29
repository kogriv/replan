"""Moved from scripts/plan-resource-demand-calibration.sh (GAP-018). Invoked as `python3 -m plan_resource_demand_calibration`."""
import glob
import json
import os
import sys
from collections import Counter, defaultdict

import yaml
import helper_envelope
from openproject_http import Settings

helper_envelope.internal_parser("plan-resource-demand-calibration").parse_args()
project_path = Settings.from_env().project_path
helper_envelope.guard("plan-resource-demand-calibration", "dry_run")

def load_yaml(rel):
    with open(os.path.join(project_path, rel)) as fh:
        return yaml.safe_load(fh) or {}

def load_actuals():
    out = []
    for path in sorted(glob.glob(os.path.join(project_path, "actuals", "*.yaml"))):
        with open(path) as fh:
            data = yaml.safe_load(fh) or {}
        for actual in data.get("actuals") or []:
            actual = dict(actual)
            actual["_source_file"] = os.path.relpath(path, project_path)
            out.append(actual)
    return out

def ref_id(obj, key):
    return (obj.get(key) or {}).get("id", "")

def number(value):
    if isinstance(value, (int, float)):
        return float(value)
    return 0.0

tasks = load_yaml("canonical/tasks.yaml").get("tasks") or []
demands = load_yaml("canonical/resource_demands.yaml").get("resource_demands") or []
resources = load_yaml("canonical/resources.yaml").get("resources") or []
actuals = load_actuals()

task_by_id = {task.get("id", ""): task for task in tasks if task.get("id", "")}
resource_by_id = {resource.get("id", ""): resource for resource in resources if resource.get("id", "")}

actuals_by_demand = defaultdict(list)
actual_amount_by_demand = defaultdict(float)
for actual in actuals:
    demand_id = ref_id(actual, "demand_ref")
    if not demand_id:
        continue
    actuals_by_demand[demand_id].append(actual)
    if actual.get("type") in {"consumption", "work_consumed"}:
        actual_amount_by_demand[demand_id] += number(actual.get("amount"))

candidate_actions = []
status_counts = Counter()
task_status_counts = Counter()
action_counts = Counter()

for demand in demands:
    demand_id = demand.get("id", "")
    status = demand.get("status", "") or "<missing>"
    status_counts[status] += 1
    task_id = ref_id(demand, "task_ref")
    task = task_by_id.get(task_id, {})
    task_status = task.get("tracking_status", "") or "<missing>"
    task_status_counts[task_status] += 1
    resource_id = ref_id(demand, "resource_ref") or ref_id(demand, "resource_pool_ref")
    unit = demand.get("unit", "")
    actual_total = actual_amount_by_demand.get(demand_id, 0.0)
    if not actuals_by_demand.get(demand_id):
        actual_total = number(demand.get("actual_amount"))
    planned = number(demand.get("planned_amount"))
    has_remaining = "remaining_amount" in demand
    remaining = number(demand.get("remaining_amount"))
    base = {
        "resource_demand_id": demand_id,
        "task_id": task_id,
        "task_title": task.get("title", ""),
        "task_status": task_status,
        "resource_id": resource_id,
        "resource_name": (resource_by_id.get(resource_id) or {}).get("name", ""),
        "resource_class": demand.get("resource_class", ""),
        "role": demand.get("role", ""),
        "unit": unit,
        "demand_status": status,
        "planned_amount": planned,
        "has_remaining_amount": has_remaining,
        "remaining_amount": remaining if has_remaining else None,
        "actual_amount_total": round(actual_total, 4),
        "actual_record_count": len(actuals_by_demand.get(demand_id, [])),
    }

    if status == "active" and task_status == "accepted":
        item = dict(base)
        item["kind"] = "active_demand_on_accepted_task"
        item["severity"] = "operator_decision"
        item["why_it_matters"] = (
            "Accepted tasks should not keep active future resource load. "
            "Otherwise workload and resource schedule reports continue to plan already finished work."
        )
        if actual_total > 0:
            item["suggested_decision"] = "close_as_consumed_after_operator_review"
            item["recommended_next_step"] = (
                "Verify that the actual records cover this demand, then close the demand lifecycle as consumed."
            )
            item["recommended_command"] = (
                "go run ./cmd/replan actual close-demand "
                f"--project {project_path} --demand {demand_id} --status consumed "
                "--reason \"Accepted task demand reviewed during resource calibration\" "
                "--actor actor.human.maintainer --dry-run --format json"
            )
        else:
            item["suggested_decision"] = "operator_choice_cancel_consume_or_reopen"
            item["recommended_next_step"] = (
                "Decide whether the demand was unnecessary and should be cancelled, was performed "
                "without an actual record and should receive evidence, or the task was accepted too early."
            )
        candidate_actions.append(item)
        action_counts[item["kind"]] += 1

    if status == "active" and task_status not in {"accepted", "cancelled"} and not has_remaining:
        item = dict(base)
        item["kind"] = "active_open_demand_without_remaining_amount"
        item["severity"] = "operator_estimate"
        item["suggested_decision"] = "set_remaining_amount_for_active_work"
        item["why_it_matters"] = (
            "Open active demands without remaining_amount make planning use planned_amount as a coarse proxy. "
            "That is acceptable for a first draft, but weak for real week-by-week resource planning."
        )
        item["suggested_remaining_amount"] = planned
        item["recommended_command"] = (
            "go run ./cmd/replan actual set-remaining "
            f"--project {project_path} --demand {demand_id} --amount {planned:g} --unit {unit} "
            "--reason \"Operator estimate during resource calibration\" "
            "--actor actor.human.maintainer --dry-run --format json"
        )
        candidate_actions.append(item)
        action_counts[item["kind"]] += 1

    if status in {"consumed", "cancelled"} and task_status not in {"accepted", "cancelled"}:
        item = dict(base)
        item["kind"] = "closed_demand_on_open_task"
        item["severity"] = "review"
        item["suggested_decision"] = "verify_task_or_demand_lifecycle"
        item["why_it_matters"] = (
            "A closed demand on an open task can be correct, but it can also mean work was removed "
            "while the task still expects that resource."
        )
        candidate_actions.append(item)
        action_counts[item["kind"]] += 1

candidate_actions.sort(key=lambda item: (
    item["kind"],
    item["task_id"],
    item["resource_demand_id"],
))

status = "aligned" if not candidate_actions else "review_required"
out = {
    "ok": True,
    "command": "plan-resource-demand-calibration",
    "calibration_plan_status": status,
    "project_path": project_path,
    "mode": "read_only",
    "policy": {
        "does_not_mutate_project": True,
        "accepted_task_rule": "active resource demands on accepted tasks require an explicit operator decision",
        "open_task_rule": "active demands on non-accepted tasks should carry explicit remaining_amount when the plan is used for real workload decisions",
        "status_close_note": "setting remaining_amount to zero is not the same as closing the demand status; consumed/cancelled status remains a separate lifecycle decision",
    },
    "counts": {
        "tasks": len(tasks),
        "resource_demands": len(demands),
        "actual_records": len(actuals),
        "candidate_actions": len(candidate_actions),
    },
    "demand_status_counts": dict(sorted(status_counts.items())),
    "task_status_counts_for_demands": dict(sorted(task_status_counts.items())),
    "candidate_action_counts": dict(sorted(action_counts.items())),
    "candidate_actions": candidate_actions,
    "suggested_operator_decision": (
        "Review the candidate actions before mutating resource demands. Start with active demands on accepted tasks because they can distort workload immediately."
        if candidate_actions else
        "No resource-demand calibration candidates were found by the current read-only policy."
    ),
    "suggested_next_actions": [
        "Review active_demand_on_accepted_task candidates and decide consumed, cancelled or reopen-task.",
        "Only after lifecycle decisions, apply remaining-work/status changes through audited commands or proposals.",
        "Then rerun scripts/check-resource-schedule-drift.sh and review remaining overload rows.",
    ] if candidate_actions else [
        "Rerun scripts/check-resource-schedule-drift.sh and proceed to resource dashboard/report review.",
    ],
}
helper_envelope.emit(out, None, "dry_run")
