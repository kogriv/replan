"""Moved from scripts/push-openproject-status-drift.sh (GAP-018). Invoked as `python3 -m push_openproject_status_drift`."""
import json
import os
import sys

import yaml
from openproject_http import OpenProjectClient, Settings
import helper_envelope
import openproject_bindings
from openproject_status_map import TRACKING_TO_OPENPROJECT_STATUS

parser = helper_envelope.internal_parser("push-openproject-status-drift")
parser.add_argument("--import-json", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--root-dir", required=True)
args = parser.parse_args()
settings = Settings.from_env(
    require_token=True,
    root_dir=args.root_dir,
)
import_path, tmp_dir = args.import_json, args.tmp_dir
project_path, bindings_path = settings.project_path, settings.bindings_path
base_url, openproject_project_id = settings.base_url, settings.project_id
token_env, host_header = settings.token_env, settings.host_header
forwarded_proto, status_scope = settings.forwarded_proto, settings.status_scope
status_path = settings.status_path
apply, keep_temp = settings.apply, settings.keep_temp

mode = "apply" if apply == "1" else "dry_run"
helper_envelope.guard("push-openproject-status-drift", mode)
partial_apply = helper_envelope.PartialApply()
base_url = base_url.rstrip("/")
client = OpenProjectClient(
    base_url, settings, host_header, forwarded_proto,
    default_expected=(200, 201, 204, 404),
)

tracking_to_openproject_status = TRACKING_TO_OPENPROJECT_STATUS
progression_tracking_statuses = (
    "not_started", "ready", "in_progress", "review_ready", "accepted",
)
progression_statuses = tuple(
    tracking_to_openproject_status[tracking_status]
    for tracking_status in progression_tracking_statuses
)

def load_yaml(path):
    with open(path, encoding="utf-8") as fh:
        return yaml.safe_load(fh) or {}

def as_text(value):
    if value is None:
        return ""
    return str(value)

def status_href_by_name():
    _, data = client.request_json("GET", "/api/v3/statuses", expected=(200,))
    out = {}
    for item in data.get("_embedded", {}).get("elements", []):
        name = item.get("name") or item.get("_links", {}).get("self", {}).get("title")
        href = item.get("_links", {}).get("self", {}).get("href")
        if name and href:
            out[name] = href
    return out

def update_work_package_status(work_package_id, status_href):
    _, wp = client.request_json("GET", f"/api/v3/work_packages/{work_package_id}", expected=(200,))
    payload = {
        "lockVersion": wp.get("lockVersion"),
        "_links": {
            "status": {"href": status_href},
        },
    }
    partial_apply.writing()
    _, data = client.request_json("PATCH", f"/api/v3/work_packages/{work_package_id}", payload, expected=(200,))
    return data

def allowed_status_hrefs(work_package_id):
    _, wp = client.request_json("GET", f"/api/v3/work_packages/{work_package_id}", expected=(200,))
    payload = {"lockVersion": wp.get("lockVersion")}
    _, form = client.request_json(
        "POST", f"/api/v3/work_packages/{work_package_id}/form", payload,
        expected=(200,), idempotent=True,
    )
    schema = (form.get("_embedded") or {}).get("schema") or {}
    status_schema = (schema.get("_embedded") or {}).get("status") or schema.get("status") or {}
    links = status_schema.get("_links") or {}
    return {item.get("href", "") for item in links.get("allowedValues") or [] if item.get("href", "")}

def allowed_status_state(work_package_id):
    _, wp = client.request_json("GET", f"/api/v3/work_packages/{work_package_id}", expected=(200,))
    payload = {"lockVersion": wp.get("lockVersion")}
    _, form = client.request_json(
        "POST", f"/api/v3/work_packages/{work_package_id}/form", payload,
        expected=(200,), idempotent=True,
    )
    schema = (form.get("_embedded") or {}).get("schema") or {}
    status_schema = (schema.get("_embedded") or {}).get("status") or schema.get("status") or {}
    links = status_schema.get("_links") or {}
    allowed = {item.get("href", "") for item in links.get("allowedValues") or [] if item.get("href", "")}
    return wp, allowed

def update_forward_status(work_package_id, lock_version, status_name, status_href):
    payload = {
        "lockVersion": lock_version,
        "_links": {"status": {"href": status_href}},
    }
    partial_apply.writing()
    _, data = client.request_json(
        "PATCH", f"/api/v3/work_packages/{work_package_id}", payload, expected=(200,)
    )
    stored = data.get("_links", {}).get("status") or {}
    if stored.get("href") != status_href and stored.get("title") != status_name:
        raise ValueError(
            f"OpenProject stored status {stored.get('href') or stored.get('title') or '<missing>'} "
            f"instead of {status_href}"
        )
    return data

def forward_eligible(current_status, desired_status):
    if current_status not in progression_statuses or desired_status not in progression_statuses:
        return False
    return progression_statuses.index(current_status) < progression_statuses.index(desired_status)

def forward_candidates(current_status, desired_status, status_hrefs):
    current_index = progression_statuses.index(current_status)
    target_index = progression_statuses.index(desired_status)
    return [
        (name, status_hrefs.get(name, ""))
        for name in progression_statuses[current_index + 1:target_index]
        if status_hrefs.get(name, "")
    ]

def path_step(from_status, to_status, to_href, **extra):
    return {"from": from_status, "to": to_status, "to_href": to_href, **extra}

def path_not_found(task_id, wp_id, tracking_status, desired_status, current_status,
                   desired_href, allowed_hrefs, path):
    return {
        "task_id": task_id,
        "work_package_id": wp_id,
        "replan_tracking_status": tracking_status,
        "openproject_source_status": current_status,
        "desired_openproject_status": desired_status,
        "desired_status_href": desired_href,
        "path": path,
        "current_openproject_status": current_status,
        "allowed_status_hrefs": sorted(allowed_hrefs),
        "reason": "openproject_status_path_not_found",
    }

def plan_forward_path(current_status, desired_status, desired_href, allowed_hrefs,
                      status_hrefs):
    if desired_href in allowed_hrefs:
        first_name, first_href = desired_status, desired_href
    else:
        allowed_intermediates = [
            pair for pair in forward_candidates(current_status, desired_status, status_hrefs)
            if pair[1] in allowed_hrefs
        ]
        if not allowed_intermediates:
            return []
        first_name, first_href = allowed_intermediates[-1]
    planned = [path_step(current_status, first_name, first_href, verified=True)]
    if first_name != desired_status:
        first_index = progression_statuses.index(first_name)
        target_index = progression_statuses.index(desired_status)
        previous = first_name
        for name in progression_statuses[first_index + 1:target_index + 1]:
            planned.append(path_step(previous, name, status_hrefs[name], verified=False))
            previous = name
    return planned

def apply_forward_path(action, task_id, wp_id, tracking_status, current_status,
                       desired_status, desired_href, status_hrefs):
    path = []
    current = current_status
    for _ in range(len(progression_statuses)):
        wp, allowed_hrefs = allowed_status_state(wp_id)
        if desired_href in allowed_hrefs:
            next_status, next_href = desired_status, desired_href
        else:
            intermediates = [
                pair for pair in forward_candidates(current, desired_status, status_hrefs)
                if pair[1] in allowed_hrefs
            ]
            if not intermediates:
                partial_apply.clear()
                return path_not_found(
                    task_id, wp_id, tracking_status, desired_status, current,
                    desired_href, allowed_hrefs, path,
                )
            next_status, next_href = intermediates[-1]
        attempted = {
            "action": "push_openproject_status_path_step",
            "task_id": task_id,
            "work_package_id": wp_id,
            **path_step(current, next_status, next_href),
        }
        partial_apply.attempting(attempted)
        updated = update_forward_status(
            wp_id, wp.get("lockVersion"), next_status, next_href
        )
        step = path_step(
            current, next_status, next_href,
            updated_lock_version=updated.get("lockVersion"),
        )
        path.append(step)
        partial_apply.applied({**attempted, **step})
        current = next_status
        if current == desired_status:
            action["path"] = path
            action["final_openproject_status"] = current
            action["updated_lock_version"] = updated.get("lockVersion")
            action["updated_openproject_status"] = current
            return None
    partial_apply.clear()
    return path_not_found(
        task_id, wp_id, tracking_status, desired_status, current,
        desired_href, set(), path,
    )

def main():
    if status_path not in ("direct", "forward"):
        helper_envelope.emit({
            "ok": False,
            "command": "push-openproject-status-drift",
            "mode": "invalid",
            "errors": [helper_envelope.diagnostic(
                "invalid_status_path",
                "REPLAN_OPENPROJECT_STATUS_PATH must be direct or forward",
            )],
        }, helper_envelope.EXIT["usage"])
    data = json.load(open(import_path, encoding="utf-8"))
    result = data.get("result") or {}
    warnings = data.get("warnings") or result.get("warnings") or []
    errors = data.get("errors") or result.get("errors") or []
    tasks = load_yaml(os.path.join(project_path, "canonical", "tasks.yaml")).get("tasks") or []
    task_by_id = {task.get("id", ""): task for task in tasks if task.get("id", "")}

    bindings = openproject_bindings.load(bindings_path, required=True)
    _, task_id_by_wp = openproject_bindings.task_maps(bindings)

    status_hrefs = status_href_by_name()
    actions = []
    skipped = []
    for candidate in result.get("task_candidates") or []:
        wp_id = as_text(candidate.get("source_id"))
        task_id = task_id_by_wp.get(wp_id, "")
        task = task_by_id.get(task_id)
        if not task:
            continue
        tracking_status = as_text(task.get("tracking_status"))
        desired_status = tracking_to_openproject_status.get(tracking_status, "")
        current_source_status = as_text(candidate.get("source_status"))
        if not desired_status or desired_status == current_source_status:
            continue
        if status_scope == "ready_only" and tracking_status != "ready":
            skipped.append({
                "task_id": task_id,
                "work_package_id": wp_id,
                "replan_tracking_status": tracking_status,
                "openproject_source_status": current_source_status,
                "desired_openproject_status": desired_status,
                "reason": "status_scope_ready_only",
                "hint": "set REPLAN_OPENPROJECT_STATUS_SCOPE=all in the environment to push every mapped status",
            })
            continue
        status_href = status_hrefs.get(desired_status, "")
        if not status_href:
            skipped.append({
                "task_id": task_id,
                "work_package_id": wp_id,
                "replan_tracking_status": tracking_status,
                "openproject_source_status": current_source_status,
                "desired_openproject_status": desired_status,
                "reason": "openproject_status_missing",
            })
            continue
        action = {
            "action": "push_openproject_status" if mode == "dry_run" else "pushed_openproject_status",
            "task_id": task_id,
            "work_package_id": wp_id,
            "replan_tracking_status": tracking_status,
            "from_openproject_status": current_source_status,
            "to_openproject_status": desired_status,
            "to_status_href": status_href,
        }
        use_forward = status_path == "forward" and forward_eligible(
            current_source_status, desired_status
        )
        if use_forward:
            if mode == "apply":
                skipped_item = apply_forward_path(
                    action, task_id, wp_id, tracking_status,
                    current_source_status, desired_status, status_href, status_hrefs,
                )
                if skipped_item:
                    skipped.append(skipped_item)
                    continue
            else:
                _, allowed_hrefs = allowed_status_state(wp_id)
                planned_path = plan_forward_path(
                    current_source_status, desired_status, status_href,
                    allowed_hrefs, status_hrefs,
                )
                if not planned_path:
                    skipped.append(path_not_found(
                        task_id, wp_id, tracking_status, desired_status,
                        current_source_status, status_href, allowed_hrefs, [],
                    ))
                    continue
                action["planned_path"] = planned_path
                action["path_note"] = "Later steps are checked one by one during apply."
                action["final_openproject_status"] = desired_status
            actions.append(action)
            continue
        if mode == "apply":
            partial_apply.attempting(action)
        allowed_hrefs = allowed_status_hrefs(wp_id)
        if status_href not in allowed_hrefs:
            if mode == "apply":
                partial_apply.clear()
            skipped.append({
                "task_id": task_id,
                "work_package_id": wp_id,
                "replan_tracking_status": tracking_status,
                "openproject_source_status": current_source_status,
                "desired_openproject_status": desired_status,
                "desired_status_href": status_href,
                "allowed_status_hrefs": sorted(allowed_hrefs),
                "reason": "openproject_workflow_transition_not_allowed",
            })
            continue
        if mode == "apply":
            updated = update_work_package_status(wp_id, status_href)
            action["updated_lock_version"] = updated.get("lockVersion")
            action["updated_openproject_status"] = (updated.get("_links", {}).get("status") or {}).get("title", "")
            partial_apply.applied(action)
        actions.append(action)

    out = {
        "ok": data.get("ok") is True and not errors and not skipped,
        "command": "push-openproject-status-drift",
        "mode": mode,
        "project_path": project_path,
        "target_openproject_project": {"identifier": openproject_project_id},
        "openproject_request": settings.request_metadata(),
        "policy": {
            "direction": "replan_tracking_status_to_openproject_status",
            "status_scope": status_scope,
            "default_scope_rule": "ready_only changes only RePlan ready tasks to OpenProject To be scheduled.",
            "status_mapping": tracking_to_openproject_status,
        },
        "counts": {
            "status_actions": len(actions),
            "skipped": len(skipped),
        },
        "actions": actions,
        "skipped": skipped,
        "warning_codes": sorted({w.get("code", "") for w in warnings if w.get("code")}),
        "error_codes": sorted({e.get("code", "") for e in errors if e.get("code")}),
        "operator_next_steps": [
            "Review status actions in dry-run output before applying.",
            "Run with REPLAN_APPLY=1 to push reviewed RePlan tracking statuses to OpenProject.",
            "Run check-openproject-drift after apply.",
        ],
        "temp_retained": bool(keep_temp),
    }
    if status_path == "forward":
        out["policy"]["status_path"] = status_path
    if any(item.get("reason") == "status_scope_ready_only" for item in skipped):
        out["operator_next_steps"].insert(
            0,
            "Set REPLAN_OPENPROJECT_STATUS_SCOPE=all in the environment to push every mapped status.",
        )
    if keep_temp:
        out["outputs_dir"] = tmp_dir
    applied = (
        len(partial_apply.applied_so_far)
        if status_path == "forward"
        else sum(1 for action in actions if action.get("updated_lock_version") is not None)
    )
    helper_envelope.emit(out, helper_envelope.apply_exit_code(out, applied), mode)

try:
    main()
except Exception as exc:
    entry, code, extra = partial_apply.failure(exc, "openproject_status_push_failed")
    out = {"ok": False, "command": "push-openproject-status-drift", "mode": mode, "errors": [entry], **extra}
    if partial_apply.applied_so_far:
        out["operator_next_steps"] = [
            "Run this helper again as a dry run; it recomputes status drift, so already-applied work packages are no longer planned.",
        ]
        if entry.get("outcome_unknown"):
            out["operator_next_steps"].insert(0, "Check the failed work package in OpenProject first; the status write may have been applied.")
    elif entry.get("outcome_unknown") and entry.get("method") == "PATCH":
        out["operator_next_steps"] = ["Inspect the work package status before running the push again."]
    helper_envelope.emit(out, code)
