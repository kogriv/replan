"""Push reviewed RePlan descriptions to bound OpenProject work packages."""

import json
import os
import urllib.parse

import yaml

import helper_envelope
import openproject_bindings
import openproject_description as description_convention
from openproject_http import OpenProjectClient, Settings


COMMAND = "push-openproject-description-drift"

parser = helper_envelope.internal_parser(COMMAND)
parser.add_argument("--root-dir", required=True)
args = parser.parse_args()

settings = Settings.from_env(require_token=True, root_dir=args.root_dir)
project_path = settings.project_path
bindings_path = settings.bindings_path
mode = "apply" if settings.apply == "1" else "dry_run"
partial_apply = helper_envelope.PartialApply()
helper_envelope.guard(
    COMMAND,
    mode,
    "openproject_description_push_failed",
    unknown_patch_next_step=(
        "Inspect the failed work package description before running the push again."
    ),
    partial_apply=partial_apply,
    partial_next_steps=(
        "Run this helper again as a dry run; it recomputes description drift, so already-applied work packages are no longer planned.",
    ),
)
client = OpenProjectClient(
    settings.base_url,
    settings,
    settings.host_header,
    settings.forwarded_proto,
    default_expected=(200, 404),
)


def load_tasks():
    path = os.path.join(project_path, "canonical", "tasks.yaml")
    with open(path, encoding="utf-8") as stream:
        document = yaml.safe_load(stream) or {}
    tasks = document.get("tasks") or []
    return {
        task.get("id", ""): task
        for task in tasks
        if isinstance(task, dict) and task.get("id", "")
    }


def selected_task_ids(task_to_work_package, task_by_id):
    raw = os.environ.get("REPLAN_TASKS", "")
    if not raw:
        return [task_id for task_id in task_to_work_package if task_id in task_by_id]
    requested = []
    for item in raw.split(","):
        task_id = item.strip()
        if task_id and task_id not in requested:
            requested.append(task_id)
    unknown = [
        task_id
        for task_id in requested
        if task_id not in task_by_id or task_id not in task_to_work_package
    ]
    if unknown:
        helper_envelope.emit(
            {
                "ok": False,
                "command": COMMAND,
                "mode": "invalid",
                "errors": [helper_envelope.diagnostic(
                    "unknown_task_ids",
                    "REPLAN_TASKS contains unknown or unbound task ids: "
                    + ", ".join(unknown),
                )],
                "unknown_task_ids": unknown,
            },
            helper_envelope.EXIT["usage"],
        )
    return requested


def normalized_description(value):
    text = "" if value is None else str(value)
    return text.replace("\r\n", "\n").rstrip()


def preview(value):
    text = "" if value is None else str(value)
    return text.replace("\r\n", "\n").replace("\r", " ").replace("\n", " ")[:80]


def conflict_from(exc):
    try:
        payload = json.loads(str(exc))
    except ValueError:
        return False
    return isinstance(payload, dict) and payload.get("status") == 409


def project_work_packages():
    project_path = "/api/v3/projects/" + urllib.parse.quote(
        settings.project_id, safe=""
    )
    _, project = client.request_json("GET", project_path, expected=(200,))
    filters = json.dumps(
        [{"project": {"operator": "=", "values": [str(project["id"])]}}],
        separators=(",", ":"),
    )
    work_packages = client.paginate(
        "/api/v3/work_packages", 500, {"filters": filters}
    )
    return {
        str(work_package.get("id")): work_package
        for work_package in work_packages
        if work_package.get("id") is not None
    }


def bound_work_package(work_package_id, listed_work_packages):
    work_package = listed_work_packages.get(str(work_package_id))
    if work_package is not None:
        return work_package
    path = "/api/v3/work_packages/" + urllib.parse.quote(
        str(work_package_id), safe=""
    )
    status, work_package = client.request_json("GET", path, expected=(200, 404))
    return None if status == 404 else work_package


def main():
    task_by_id = load_tasks()
    bindings = openproject_bindings.load(bindings_path, required=True)
    task_to_work_package, _ = openproject_bindings.task_maps(bindings)
    task_ids = selected_task_ids(task_to_work_package, task_by_id)

    actions = []
    skipped = []
    warnings = []
    errors = []
    compared = 0
    differing = 0
    applied_actions = 0

    nonempty_task_ids = []
    for task_id in task_ids:
        work_package_id = task_to_work_package[task_id]
        replan_description = task_by_id[task_id].get("description", "")
        if normalized_description(replan_description) == "":
            skip = {
                "task_id": task_id,
                "work_package_id": work_package_id,
                "reason": "replan_description_empty",
            }
            skipped.append(skip)
            warnings.append(helper_envelope.diagnostic(
                "replan_description_empty",
                f"task {task_id} has an empty RePlan description; work package {work_package_id} was not changed",
                "warning",
                "task",
                task_id,
            ))
            continue
        nonempty_task_ids.append(task_id)

    listed_work_packages = project_work_packages() if nonempty_task_ids else {}

    for task_id in nonempty_task_ids:
        work_package_id = task_to_work_package[task_id]
        replan_description = task_by_id[task_id].get("description", "")

        path = "/api/v3/work_packages/" + urllib.parse.quote(
            str(work_package_id), safe=""
        )
        work_package = bound_work_package(work_package_id, listed_work_packages)
        if work_package is None:
            errors.append(helper_envelope.diagnostic(
                "bound_work_package_missing",
                f"bound work package {work_package_id} for task {task_id} was not found",
                "error",
                "openproject_work_package",
                str(work_package_id),
            ))
            skipped.append({
                "task_id": task_id,
                "work_package_id": work_package_id,
                "reason": "bound_work_package_missing",
            })
            continue

        openproject_description = (work_package.get("description") or {}).get("raw", "")
        owned_description, kept_tail = description_convention.split_task_description(
            openproject_description, task_id
        )
        compared += 1
        if normalized_description(replan_description) == normalized_description(
            owned_description
        ):
            continue

        wanted_description = description_convention.description_with_tail(
            replan_description, kept_tail
        )

        differing += 1
        action = {
            "action": "update_description",
            "task_id": task_id,
            "work_package_id": work_package_id,
            "old_length": len(str(openproject_description or "")),
            "new_length": len(str(wanted_description or "")),
            "old_preview": preview(openproject_description),
            "new_preview": preview(wanted_description),
            "kept_tail_length": len(kept_tail),
        }
        actions.append(action)
        if mode != "apply":
            continue

        partial_apply.attempting(action)
        payload = {
            "description": {
                "format": "markdown",
                "raw": wanted_description,
            },
            "lockVersion": work_package.get("lockVersion"),
        }
        try:
            partial_apply.writing()
            _, updated = client.request_json(
                "PATCH", path, payload, expected=(200,)
            )
        except RuntimeError as exc:
            if not conflict_from(exc):
                raise
            partial_apply.clear()
            errors.append(helper_envelope.diagnostic(
                "openproject_work_package_changed",
                f"work package {work_package_id} changed after lock version {work_package.get('lockVersion')} was read",
                "error",
                "openproject_work_package",
                str(work_package_id),
            ))
            skipped.append({
                "task_id": task_id,
                "work_package_id": work_package_id,
                "reason": "openproject_work_package_changed",
            })
            continue

        action["action"] = "updated_description"
        action["updated_lock_version"] = updated.get("lockVersion")
        applied_actions += 1
        partial_apply.applied(action)

    out = {
        "ok": not errors,
        "command": COMMAND,
        "mode": mode,
        "project_path": project_path,
        "target_openproject_project": {"identifier": settings.project_id},
        "openproject_request": settings.request_metadata(),
        "policy": {
            "direction": "replan_description_to_openproject_description",
            "empty_replan_description": "skip",
            "bootstrap_task_id_trailer": "preserve_line_and_following_tail_when_present",
            "comparison_normalization": "crlf_to_lf_and_trailing_text_whitespace",
        },
        "counts": {
            "compared": compared,
            "differing": differing,
            "planned_actions": len(actions),
            "applied_actions": applied_actions,
            "skipped": len(skipped),
        },
        "actions": actions,
        "skipped": skipped,
        "warnings": warnings,
        "errors": errors,
        "operator_next_steps": [
            "Review description lengths and previews in dry-run output before applying.",
            "Run with REPLAN_APPLY=1 to push reviewed RePlan descriptions to OpenProject.",
        ],
        "temp_retained": False,
    }
    helper_envelope.emit(
        out,
        helper_envelope.apply_exit_code(out, applied_actions),
        mode,
    )


main()
