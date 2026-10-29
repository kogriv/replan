"""Moved from scripts/review-resource-pool.sh (GAP-018). Invoked as `python3 -m review_resource_pool`.

The project's own resource policy, which resources are core, provisional or
placeholders and the issues its operator recorded, comes from
review/resource-pool.yaml in the project. Without it every resource needs an
operator decision and only the generic checks apply.
"""
import glob
import json
import os
import sys
from collections import Counter, defaultdict

import yaml
import helper_envelope
from openproject_http import Settings

helper_envelope.internal_parser("review-resource-pool").parse_args()
project_path = Settings.from_env().project_path
helper_envelope.guard("review-resource-pool")

def load_yaml(relative_path):
    path = os.path.join(project_path, relative_path)
    if not os.path.exists(path):
        return {}
    with open(path) as fh:
        return yaml.safe_load(fh) or {}

resources = load_yaml("canonical/resources.yaml").get("resources") or []
capacities = load_yaml("canonical/capacities.yaml").get("capacities") or []
availability = load_yaml("canonical/availability.yaml").get("availability") or []
demands = load_yaml("canonical/resource_demands.yaml").get("resource_demands") or []
tasks = load_yaml("canonical/tasks.yaml").get("tasks") or []

actuals = []
for path in sorted(glob.glob(os.path.join(project_path, "actuals", "*.yaml"))):
    with open(path) as fh:
        actuals.extend((yaml.safe_load(fh) or {}).get("actuals") or [])

task_status_by_id = {task.get("id"): task.get("tracking_status", "") for task in tasks}

capacity_by_resource = defaultdict(list)
for capacity in capacities:
    resource_id = (capacity.get("resource_ref") or {}).get("id", "")
    if resource_id:
        capacity_by_resource[resource_id].append(capacity)

availability_by_resource = defaultdict(list)
for item in availability:
    resource_id = (item.get("resource_ref") or {}).get("id", "")
    if resource_id:
        availability_by_resource[resource_id].append(item)

demands_by_resource = defaultdict(list)
for demand in demands:
    resource_id = (demand.get("resource_ref") or {}).get("id", "")
    if resource_id:
        demands_by_resource[resource_id].append(demand)

actuals_by_resource = defaultdict(list)
for actual in actuals:
    resource_id = (actual.get("resource_ref") or {}).get("id", "")
    if resource_id:
        actuals_by_resource[resource_id].append(actual)

POLICY_PATH = "review/resource-pool.yaml"
policy = load_yaml(POLICY_PATH).get("resource_pool_review") or {}
buckets = policy.get("buckets") or {}
core_ids = set(buckets.get("core_now") or [])
provisional_ids = set(buckets.get("provisional_side_constraint") or [])
future_ids = set(buckets.get("future_placeholder") or [])
DECISIONS = {
    "core_now": "keep_for_openproject_bootstrap",
    "future_placeholder": "keep_but_do_not_block_bootstrap",
    "provisional_side_constraint": "keep_but_recalibrate_later",
    "needs_operator_decision": "review_before_bootstrap_if_used",
}
RATIONALE = {
    "core_now": "Listed as a core resource in the project's resource pool policy.",
    "future_placeholder": "Listed as a placeholder in the project's resource pool policy.",
    "provisional_side_constraint": "Listed as a provisional constraint in the project's resource pool policy.",
    "needs_operator_decision": f"Not in any bucket of the project's resource pool policy ({POLICY_PATH}).",
    **(policy.get("rationale") or {}),
}
resource_issues = policy.get("resource_issues") or {}
issues_without_demands = policy.get("issues_without_demands") or {}
gate_notes = policy.get("bootstrap_gate") or {}

def amount_sum(items, key):
    total = 0.0
    for item in items:
        amount = item.get(key)
        if isinstance(amount, (int, float)):
            total += float(amount)
    return total

reviews = []
unknown_resource_ids = set(demands_by_resource) - {resource.get("id") for resource in resources}

for resource in resources:
    resource_id = resource.get("id", "")
    resource_demands = demands_by_resource.get(resource_id, [])
    resource_actuals = actuals_by_resource.get(resource_id, [])
    demand_status_counts = Counter(demand.get("status", "<missing>") for demand in resource_demands)
    demand_unit_counts = Counter(demand.get("unit", "<missing>") for demand in resource_demands)
    task_status_counts = Counter(
        task_status_by_id.get((demand.get("task_ref") or {}).get("id", ""), "<missing>")
        for demand in resource_demands
    )
    capacity_items = capacity_by_resource.get(resource_id, [])

    if resource_id in core_ids:
        bucket = "core_now"
    elif resource_id in future_ids:
        bucket = "future_placeholder"
    elif resource_id in provisional_ids:
        bucket = "provisional_side_constraint"
    else:
        bucket = "needs_operator_decision"
    decision = DECISIONS[bucket]
    rationale = RATIONALE[bucket]

    issues = []
    if not capacity_items:
        issues.append("missing_capacity")
    if not resource_demands and bucket == "core_now":
        issues.append("core_resource_has_no_demands")
    issues.extend(resource_issues.get(resource_id) or [])
    if not resource_demands:
        issues.extend(issues_without_demands.get(resource_id) or [])

    reviews.append({
        "resource_id": resource_id,
        "name": resource.get("name", ""),
        "class": resource.get("class", ""),
        "bucket": bucket,
        "recommended_decision": decision,
        "rationale": rationale,
        "capabilities": resource.get("capabilities") or [],
        "capacity": [
            {
                "id": item.get("id", ""),
                "type": item.get("type", ""),
                "unit": item.get("unit", ""),
                "amount": item.get("amount", 0),
                "period": item.get("period", ""),
            }
            for item in capacity_items
        ],
        "has_availability": bool(availability_by_resource.get(resource_id)),
        "demand_count": len(resource_demands),
        "demand_status_counts": dict(sorted(demand_status_counts.items())),
        "demand_unit_counts": dict(sorted(demand_unit_counts.items())),
        "demand_planned_amount_total_by_unit": {
            unit: amount_sum([d for d in resource_demands if d.get("unit") == unit], "planned_amount")
            for unit in sorted(demand_unit_counts)
        },
        "actual_count": len(resource_actuals),
        "actual_amount_total_by_unit": {
            unit: amount_sum([a for a in resource_actuals if a.get("unit") == unit], "amount")
            for unit in sorted({a.get("unit", "<missing>") for a in resource_actuals})
        },
        "task_status_counts": dict(sorted(task_status_counts.items())),
        "issues": issues,
    })

bucket_counts = Counter(item["bucket"] for item in reviews)
blocking_issues = []
if unknown_resource_ids:
    blocking_issues.append({
        "code": "unknown_resource_referenced_by_demands",
        "resource_ids": sorted(unknown_resource_ids),
    })

out = {
    "ok": not blocking_issues,
    "command": "review-resource-pool",
    "project_path": project_path,
    "policy_file": POLICY_PATH if policy else "",
    "counts": {
        "resources": len(resources),
        "capacities": len(capacities),
        "availability_records": len(availability),
        "resource_demands": len(demands),
        "actuals": len(actuals),
        "buckets": dict(sorted(bucket_counts.items())),
    },
    "openproject_bootstrap_gate": {
        "can_create_task_skeleton": not blocking_issues,
        "reason": "OpenProject bootstrap needs task skeleton and optional estimates; RePlan resource-sidecar details can continue evolving separately.",
        "must_decide_before_apply": gate_notes.get("must_decide_before_apply") or [],
        "accepted_operator_decisions": gate_notes.get("accepted_operator_decisions") or [],
        "private_binding_layer": {
            "recommended_path": "private/openproject/<OpenProject project identifier>-bindings.yaml",
            "committed_example": "examples/openproject/private-bindings.example.yaml",
        },
        "can_defer_until_after_bootstrap": gate_notes.get("can_defer_until_after_bootstrap") or [],
    },
    "blocking_issues": blocking_issues,
    "resource_reviews": reviews,
}

helper_envelope.emit(out, None)
