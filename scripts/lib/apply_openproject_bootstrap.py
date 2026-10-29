"""Moved from scripts/apply-openproject-bootstrap.sh (GAP-018). Invoked as `python3 -m apply_openproject_bootstrap`."""
import json
import os
import sys
import urllib.parse

import yaml
from openproject_http import OpenProjectClient, Settings
import helper_envelope
import openproject_bindings
import openproject_working_days

parser = helper_envelope.internal_parser("apply-openproject-bootstrap")
parser.add_argument("--plan-json", required=True)
parser.add_argument("--root-dir", required=True)
args = parser.parse_args()
root_dir = args.root_dir
settings = Settings.from_env(
    require_token=True, root_dir=root_dir,
)
plan_path = args.plan_json
base_url, token_env = settings.base_url, settings.token_env
host_header, forwarded_proto = settings.host_header, settings.forwarded_proto
target_project_id, target_project_name = settings.target_project_id, settings.target_project_name
assignee_user_id, assignee_name = settings.assignee_user_id, settings.assignee_name
apply, bindings_path = settings.apply, settings.bindings_path
mode = "apply" if apply == "1" else "dry_run"
assignee_resource_id = os.environ.get("REPLAN_OPENPROJECT_ASSIGNEE_RESOURCE_ID", "")
helper_envelope.guard("apply-openproject-bootstrap", mode)
partial_apply = helper_envelope.PartialApply()
partial_binding_context = {
    "exists": False,
    "existing_document": None,
    "project": None,
    "task_bindings": [],
}
base_url = base_url.rstrip("/")
client = OpenProjectClient(
    base_url, settings, host_header, forwarded_proto,
    default_expected=(200, 201, 204, 404),
)
with open(plan_path) as plan_file:
    plan = json.load(plan_file)


class BindingsFileError(Exception):
    def __init__(self, code, message, exit_code):
        super().__init__(message)
        self.code = code
        self.exit_code = exit_code

def safe_main(fn):
    try:
        return fn()
    except BindingsFileError as exc:
        helper_envelope.emit({
            "ok": False,
            "command": "apply-openproject-bootstrap",
            "mode": mode,
            "openproject_request": settings.request_metadata(),
            "errors": [helper_envelope.diagnostic(exc.code, str(exc))],
        }, exc.exit_code)
    except Exception as exc:
        private_binding_layer = persist_partial_bindings()
        entry, code, extra = partial_apply.failure(exc, "openproject_bootstrap_apply_failed")
        out = {
            "ok": False,
            "command": "apply-openproject-bootstrap",
            "mode": mode,
            "openproject_request": settings.request_metadata(),
            "errors": [entry],
            "private_binding_layer": private_binding_layer,
            **extra,
        }
        if partial_apply.applied_so_far:
            out["operator_next_steps"] = [
                "Run this helper again as a dry run; bootstrap matches work packages by subject and private bindings, so it resumes without recreating confirmed work packages."
            ]
            if entry.get("outcome_unknown"):
                out["operator_next_steps"].insert(0, "Check the failed project, membership, or work package in OpenProject first; the write may have been applied.")
        elif entry.get("outcome_unknown"):
            out["operator_next_steps"] = [
                "Check whether the project, membership, or work package exists before running apply again."
            ]
        helper_envelope.emit(out, code)

def iso_duration_hours(hours):
    if not isinstance(hours, (int, float)) or hours <= 0:
        return None
    minutes = int(round(float(hours) * 60))
    h, m = divmod(minutes, 60)
    if h and m:
        return f"PT{h}H{m}M"
    if h:
        return f"PT{h}H"
    return f"PT{m}M"

def status_map_by_title():
    _, data = client.request_json("GET", "/api/v3/statuses", expected=(200,))
    return {
        item.get("name") or item.get("_links", {}).get("self", {}).get("title"): item.get("_links", {}).get("self", {}).get("href")
        for item in data.get("_embedded", {}).get("elements", [])
    }

def existing_project():
    status, data = client.request_json("GET", f"/api/v3/projects/{urllib.parse.quote(target_project_id)}", expected=(200, 404))
    if status == 404:
        return None
    return data

def create_project():
    payload = {
        "name": target_project_name,
        "identifier": target_project_id,
        "public": False,
        "active": True,
    }
    _, data = client.request_json("POST", "/api/v3/projects", payload, expected=(201,))
    return data

def project_types(project):
    project_id = project["id"]
    _, data = client.request_json("GET", f"/api/v3/projects/{project_id}/types", expected=(200,))
    elements = data.get("_embedded", {}).get("elements", [])
    for item in elements:
        if (item.get("name") or item.get("_links", {}).get("self", {}).get("title")) == "Task":
            return item.get("_links", {}).get("self", {}).get("href")
    if elements:
        return elements[0].get("_links", {}).get("self", {}).get("href")
    raise RuntimeError(f"target project {target_project_id} has no work package types")

def existing_work_packages(project):
    project_id = project["id"]
    filters = json.dumps([{"project": {"operator": "=", "values": [str(project_id)]}}], separators=(",", ":"))
    by_subject = {}
    for item in client.paginate("/api/v3/work_packages", 500, {"filters": filters}):
        subject = item.get("subject", "")
        if subject:
            by_subject[subject.strip().lower()] = item
    return by_subject

def membership_filter_path(project):
    filters = json.dumps([{"project": {"operator": "=", "values": [str(project["id"])]}}], separators=(",", ":"))
    return "/api/v3/memberships?filters=" + urllib.parse.quote(filters) + "&pageSize=100"

def assignee_membership_exists(project):
    _, data = client.request_json("GET", membership_filter_path(project), expected=(200,))
    for item in data.get("_embedded", {}).get("elements", []):
        principal = item.get("_links", {}).get("principal", {})
        if principal.get("href") == f"/api/v3/users/{assignee_user_id}":
            return True
    return False

def create_assignee_membership(project):
    payload = {
        "_links": {
            "principal": {"href": f"/api/v3/users/{assignee_user_id}"},
            "project": {"href": f"/api/v3/projects/{project['id']}"},
            "roles": [{"href": "/api/v3/roles/6"}],
        }
    }
    _, data = client.request_json("POST", "/api/v3/memberships", payload, expected=(201,))
    return data

def create_work_package(project, type_href, status_href_by_title, item, working_by_day, action):
    project_href = f"/api/v3/projects/{project['id']}"
    estimated = iso_duration_hours(item.get("estimated_hours_from_hour_demands"))
    payload = {
        "subject": item["subject"],
        "description": {
            "format": "markdown",
            "raw": item.get("description", ""),
        },
        "scheduleManually": True,
        "_links": {
            "project": {"href": project_href},
            "type": {"href": type_href},
            "assignee": {"href": f"/api/v3/users/{assignee_user_id}", "title": assignee_name},
        },
    }
    if item.get("start_date"):
        payload["startDate"] = item["start_date"]
    if item.get("due_date"):
        payload["dueDate"] = item["due_date"]
    openproject_working_days.apply_ignore_non_working_days(
        payload,
        action,
        working_by_day,
        (("start date", item.get("start_date", "")), ("due date", item.get("due_date", ""))),
    )
    if estimated:
        payload["estimatedTime"] = estimated
    status_href = status_href_by_title.get(item.get("suggested_openproject_status", ""))
    if status_href:
        payload["_links"]["status"] = {"href": status_href}
    _, data = client.request_json("POST", "/api/v3/work_packages", payload, expected=(201,))
    return data

def resolve_assignee():
    """Default the assignee to the token's own user.

    An ordinary member can read /users/me, is already a member of the
    project, and so needs no administrator to be assigned its own plan.
    """
    global assignee_user_id, assignee_name
    if assignee_user_id:
        return
    _, me = client.request_json("GET", "/api/v3/users/me", expected=(200,))
    assignee_user_id = str(me.get("id") or "")
    assignee_name = assignee_name or me.get("name", "")
    if not assignee_user_id:
        raise RuntimeError("could not resolve the token's own user; set REPLAN_OPENPROJECT_ASSIGNEE_USER_ID")


def load_private_bindings():
    if not os.path.exists(bindings_path):
        return False, None
    try:
        doc = openproject_bindings.load(bindings_path, validate=True)
    except (OSError, yaml.YAMLError) as exc:
        raise BindingsFileError(
            "bindings_file_invalid",
            f"could not load private bindings file {bindings_path}: {exc}",
            helper_envelope.EXIT["validation"],
        ) from exc
    except openproject_bindings.BindingsValidationError as exc:
        if exc.reason == "non_list":
            message = f"private bindings file {bindings_path} has a non-list {exc.key} value"
        else:
            message = f"private bindings file {bindings_path} has no openproject_private_bindings root"
        raise BindingsFileError(
            "bindings_file_invalid",
            message,
            helper_envelope.EXIT["validation"],
        ) from exc
    root = openproject_bindings.root(doc)
    existing_project_id = (root.get("project") or {}).get("project_id") if isinstance(root.get("project"), dict) else None
    if existing_project_id != target_project_id:
        raise BindingsFileError(
            "bindings_project_mismatch",
            f"private bindings project {existing_project_id!r} does not match target project {target_project_id!r}",
            helper_envelope.EXIT["conflict"],
        )
    return True, doc


def merge_private_bindings(existing_doc, project, task_bindings, pending_task_ids):
    if existing_doc is None:
        root = {
            "generated_at": "",
            "project": {
                "source": "openproject",
                "project_id": target_project_id,
                "project_href": f"/api/v3/projects/{project['id']}",
                "base_url": base_url,
            },
            "tasks": [],
            "dependencies": [],
            "resources": [],
        }
        doc = {"openproject_private_bindings": root}
    else:
        doc = existing_doc
        root = doc["openproject_private_bindings"]

    tasks = root.setdefault("tasks", [])
    kept_task_bindings = len(tasks)
    added_task_bindings = 0
    warnings = []
    task_by_replan_id = {
        item.get("replan_task_id"): item
        for item in tasks
        if isinstance(item, dict) and item.get("replan_task_id")
    }
    for binding in task_bindings:
        replan_id = binding["replan_task_id"]
        existing = task_by_replan_id.get(replan_id)
        if existing is None:
            tasks.append(binding)
            task_by_replan_id[replan_id] = binding
            added_task_bindings += 1
            continue
        existing_id = str((existing.get("source_ref") or {}).get("object_id", ""))
        bootstrap_id = str(binding["source_ref"]["object_id"])
        if existing_id != bootstrap_id:
            warning = helper_envelope.diagnostic(
                "binding_conflict",
                (
                    f"task {replan_id} is already bound to work package {existing_id}; "
                    f"bootstrap found work package {bootstrap_id}"
                ),
                "warning",
                "task",
                replan_id,
            )
            warning.update({
                "existing_work_package_id": existing_id,
                "bootstrap_work_package_id": bootstrap_id,
            })
            warnings.append(warning)
    pending_task_bindings = sum(1 for replan_id in set(pending_task_ids) if replan_id not in task_by_replan_id)

    resources = root.setdefault("resources", [])
    # The assignee's RePlan resource comes from a setting; without it no
    # resource binding is written (finding C-05).
    if assignee_resource_id:
        resource_binding = {
            "replan_resource_id": assignee_resource_id,
            "source_ref": {
                "source": "openproject",
                "object_type": "user",
                "object_id": str(assignee_user_id),
                "url": f"{base_url}/users/{assignee_user_id}",
            },
        }
        resource_by_replan_id = {
            item.get("replan_resource_id"): item
            for item in resources
            if isinstance(item, dict) and item.get("replan_resource_id")
        }
        existing = resource_by_replan_id.get(assignee_resource_id)
        if existing is None:
            resources.append(resource_binding)
        else:
            existing_id = str((existing.get("source_ref") or {}).get("object_id", ""))
            bootstrap_id = str(assignee_user_id)
            if existing_id != bootstrap_id:
                warning = helper_envelope.diagnostic(
                    "binding_conflict",
                    (
                        f"resource {assignee_resource_id} is already bound to user {existing_id}; "
                        f"bootstrap found user {bootstrap_id}"
                    ),
                    "warning",
                    "resource",
                    assignee_resource_id,
                )
                warning.update({
                    "existing_user_id": existing_id,
                    "bootstrap_user_id": bootstrap_id,
                })
                warnings.append(warning)

    openproject_bindings.refresh_generated_at(doc)
    return doc, warnings, {
        "task_bindings": len(tasks) + pending_task_bindings,
        "added_task_bindings": added_task_bindings + pending_task_bindings,
        "kept_task_bindings": kept_task_bindings,
        "dependency_bindings": len(root.get("dependencies", [])),
        "resource_bindings": len(resources),
    }


def write_private_bindings(doc):
    openproject_bindings.write(bindings_path, doc)


def persist_partial_bindings():
    """Persist bindings known before a failed remote write."""
    context = partial_binding_context
    summary = {
        "path": bindings_path,
        "exists": context["exists"],
        "written": False,
    }
    project = context["project"]
    task_bindings = context["task_bindings"]
    created_project = any(a.get("action") == "created_project" for a in partial_apply.applied_so_far)
    if mode != "apply" or project is None or (not task_bindings and not created_project):
        return summary
    try:
        doc, _, counts = merge_private_bindings(
            context["existing_document"], project, task_bindings, []
        )
        write_private_bindings(doc)
    except Exception as exc:
        summary["write_error"] = str(exc)
        return summary
    summary.update(counts)
    summary["written"] = True
    return summary

def main():
    planned = plan.get("work_packages") or []
    actions = []
    bindings = []
    errors = []
    warnings = []
    dated_items = [item for item in planned if item.get("start_date") or item.get("due_date")]
    working_by_day = None
    if dated_items:
        working_by_day, working_day_warnings = openproject_working_days.load(client)
        warnings.extend(working_day_warnings)
    bindings_exists, existing_bindings_doc = load_private_bindings()
    partial_binding_context["exists"] = bindings_exists
    partial_binding_context["existing_document"] = existing_bindings_doc
    partial_binding_context["task_bindings"] = bindings
    resolve_assignee()

    project_step = {
        "action": "create_project",
        "project_identifier": target_project_id,
        "project_name": target_project_name,
    }
    if mode == "apply":
        partial_apply.attempting(project_step)
    project = existing_project()
    project_action = "exists" if project else "create"
    if project is None and mode == "apply":
        partial_apply.writing()
        project = create_project()
        partial_apply.applied({**project_step, "action": "created_project", "project_id": project.get("id")})
    elif mode == "apply":
        partial_apply.clear()
    partial_binding_context["project"] = project

    membership_action = "not_checked_project_missing"
    if project is not None:
        membership_step = {
            "action": "create_project_admin_membership",
            "project_id": project.get("id"),
            "assignee_user_id": assignee_user_id,
        }
        if mode == "apply":
            partial_apply.attempting(membership_step)
        if assignee_membership_exists(project):
            membership_action = "exists"
            if mode == "apply":
                partial_apply.clear()
        elif mode == "apply":
            partial_apply.writing()
            membership = create_assignee_membership(project)
            membership_action = "created_project_admin_membership"
            partial_apply.applied({
                **membership_step,
                "action": membership_action,
                "membership_id": membership.get("id"),
            })
        else:
            membership_action = "create_project_admin_membership"

    if project is None:
        for item in planned:
            action = {
                "action": "create_work_package_after_project_create",
                "replan_task_id": item["replan_task_id"],
                "subject": item["subject"],
                "suggested_openproject_status": item.get("suggested_openproject_status", ""),
                "assignee_user_id": assignee_user_id,
            }
            if item.get("start_date") or item.get("due_date"):
                openproject_working_days.apply_ignore_non_working_days(
                    {}, action, working_by_day,
                    (("start date", item.get("start_date", "")), ("due date", item.get("due_date", ""))),
                )
            actions.append(action)
    else:
        if planned and mode == "apply":
            partial_apply.attempting({
                "action": "create_work_package",
                "replan_task_id": planned[0]["replan_task_id"],
                "subject": planned[0]["subject"],
            })
        type_href = project_types(project)
        statuses = status_map_by_title()
        existing_by_subject = existing_work_packages(project)
        if planned and mode == "apply":
            partial_apply.clear()
        for item in planned:
            key = item["subject"].strip().lower()
            existing = existing_by_subject.get(key)
            if existing:
                wp_id = str(existing.get("id", ""))
                actions.append({
                    "action": "match_existing_work_package",
                    "replan_task_id": item["replan_task_id"],
                    "subject": item["subject"],
                    "work_package_id": wp_id,
                })
                bindings.append({
                    "replan_task_id": item["replan_task_id"],
                    "source_ref": {
                        "source": "openproject",
                        "object_type": "work_package",
                        "object_id": wp_id,
                        "url": f"{base_url}/work_packages/{wp_id}",
                    },
                })
                continue
            if mode == "apply":
                step = {
                    "action": "create_work_package",
                    "replan_task_id": item["replan_task_id"],
                    "subject": item["subject"],
                }
                if item.get("start_date") or item.get("due_date"):
                    openproject_working_days.apply_ignore_non_working_days(
                        {}, step, working_by_day,
                        (("start date", item.get("start_date", "")), ("due date", item.get("due_date", ""))),
                    )
                partial_apply.attempting(step)
                partial_apply.writing()
                created = create_work_package(project, type_href, statuses, item, working_by_day, step)
                wp_id = str(created.get("id", ""))
                action = {
                    **step,
                    "action": "created_work_package",
                    "replan_task_id": item["replan_task_id"],
                    "subject": item["subject"],
                    "work_package_id": wp_id,
                }
                actions.append(action)
                bindings.append({
                    "replan_task_id": item["replan_task_id"],
                    "source_ref": {
                        "source": "openproject",
                        "object_type": "work_package",
                        "object_id": wp_id,
                        "url": f"{base_url}/work_packages/{wp_id}",
                    },
                })
                partial_apply.applied(action)
            else:
                action = {
                    "action": "create_work_package",
                    "replan_task_id": item["replan_task_id"],
                    "subject": item["subject"],
                    "suggested_openproject_status": item.get("suggested_openproject_status", ""),
                    "assignee_user_id": assignee_user_id,
                    "start_date": item.get("start_date", ""),
                    "due_date": item.get("due_date", ""),
                }
                if item.get("start_date") or item.get("due_date"):
                    openproject_working_days.apply_ignore_non_working_days(
                        {}, action, working_by_day,
                        (("start date", item.get("start_date", "")), ("due date", item.get("due_date", ""))),
                    )
                actions.append(action)

    bindings_written = False
    pending_task_ids = [
        action["replan_task_id"]
        for action in actions
        if action["action"] in {"create_work_package", "create_work_package_after_project_create"}
    ]
    binding_step = {
        "action": "write_private_bindings",
        "path": bindings_path,
        "task_bindings": len(bindings),
    }
    if mode == "apply" and project is not None and bindings:
        partial_apply.attempting(binding_step)
    doc, binding_warnings, binding_counts = merge_private_bindings(
        existing_bindings_doc, project or {"id": ""}, bindings, pending_task_ids
    )
    warnings.extend(binding_warnings)
    if mode == "apply" and project is not None and bindings:
        partial_apply.writing()
        write_private_bindings(doc)
        bindings_written = True
        partial_apply.applied({**binding_step, "written": True})

    counts = {
        "planned_work_packages": len(planned),
        "actions": len(actions),
        "create_work_package": sum(1 for action in actions if action["action"] in {"create_work_package", "create_work_package_after_project_create"}),
        "created_work_package": sum(1 for action in actions if action["action"] == "created_work_package"),
        "matched_existing_work_package": sum(1 for action in actions if action["action"] == "match_existing_work_package"),
    }

    out = {
        "ok": not errors,
        "command": "apply-openproject-bootstrap",
        "mode": mode,
        "project_path": plan.get("project_path", ""),
        "target_openproject_project": {
            "identifier": target_project_id,
            "name": target_project_name,
            "action": project_action,
            "id": project.get("id") if project else None,
        },
        "openproject_request": settings.request_metadata(),
        "assignee": {
            "user_id": assignee_user_id,
            "name": assignee_name,
            "membership_action": membership_action,
        },
        "private_binding_layer": {
            "path": bindings_path,
            "exists": bindings_exists,
            "written": bindings_written,
            **binding_counts,
        },
        "counts": counts,
        "actions": actions,
        "warnings": warnings,
        "errors": errors,
        "operator_next_steps": [
            "If this was dry-run and actions look correct, run with REPLAN_APPLY=1.",
            "After apply, run check-openproject-live-alignment against the target OpenProject project.",
            "Keep generated private bindings out of git.",
            "Create OpenProject relations only after task alignment is confirmed.",
        ],
    }

    applied = sum(1 for action in actions if action["action"] == "created_work_package")
    helper_envelope.emit(out, helper_envelope.apply_exit_code(out, applied), mode)

safe_main(main)
