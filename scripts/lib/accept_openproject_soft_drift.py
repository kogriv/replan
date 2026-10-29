"""Dates-only OpenProject drift review and audited task-set-dates wrapper."""
from datetime import datetime, timezone
import json
import os
import subprocess

import yaml
import helper_envelope
import openproject_bindings
from openproject_http import Settings

parser = helper_envelope.internal_parser("accept-openproject-soft-drift")
parser.add_argument("--import-json", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--root-dir", required=True)
parser.add_argument("--replan-bin", required=True)
args = parser.parse_args()
settings = Settings.from_env(root_dir=args.root_dir)
import_path, tmp_dir = args.import_json, args.tmp_dir
project_path, bindings_path = settings.project_path, settings.bindings_path
openproject_project_id = settings.project_id
apply, keep_temp = settings.apply, settings.keep_temp
actor = os.environ.get("REPLAN_ACTOR", "").strip()
mode = "apply" if apply == "1" else "dry_run"
partial_apply = helper_envelope.PartialApply()
helper_envelope.guard(
    "accept-openproject-soft-drift", mode,
    partial_apply=partial_apply,
    partial_next_steps=(
        "Inspect applied_so_far and the audit log before retrying.",
        "Run check-openproject-drift to see which date changes remain.",
    ),
)
with open(import_path, encoding="utf-8") as import_file:
    data = json.load(import_file)
result = data.get("result") or {}
warnings = data.get("warnings") or result.get("warnings") or []
errors = data.get("errors") or result.get("errors") or []

def load_yaml(path):
    with open(path) as fh:
        return yaml.safe_load(fh) or {}

def as_text(value):
    if value is None:
        return ""
    return str(value)

tasks_path = os.path.join(project_path, "canonical", "tasks.yaml")
tasks_doc = load_yaml(tasks_path)
tasks = tasks_doc.get("tasks") or []
task_by_id = {task.get("id", ""): task for task in tasks if task.get("id", "")}

bindings_loaded = os.path.exists(bindings_path)
task_id_by_wp = {}
if bindings_loaded:
    bindings_doc = openproject_bindings.load(bindings_path)
    _, task_id_by_wp = openproject_bindings.task_maps(bindings_doc)

date_changes = []
status_reviews = []
unmatched_candidates = []
for candidate in result.get("task_candidates") or []:
    wp_id = as_text(candidate.get("source_id"))
    task_id = task_id_by_wp.get(wp_id, "")
    if not task_id:
        unmatched_candidates.append({
            "work_package_id": wp_id,
            "title": candidate.get("title", ""),
        })
        continue
    task = task_by_id.get(task_id)
    if not task:
        unmatched_candidates.append({
            "work_package_id": wp_id,
            "task_id": task_id,
            "title": candidate.get("title", ""),
            "reason": "bound_task_missing",
        })
        continue
    start_date = as_text(candidate.get("start_date"))
    due_date = as_text(candidate.get("due_date"))
    if start_date and start_date != as_text(task.get("planned_start")):
        date_changes.append({
            "op": "update_task_planned_start",
            "task_id": task_id,
            "work_package_id": wp_id,
            "from": as_text(task.get("planned_start")),
            "to": start_date,
            "path": f"tasks.{task_id}.planned_start",
        })
    if due_date and due_date != as_text(task.get("planned_finish")):
        date_changes.append({
            "op": "update_task_planned_finish",
            "task_id": task_id,
            "work_package_id": wp_id,
            "from": as_text(task.get("planned_finish")),
            "to": due_date,
            "path": f"tasks.{task_id}.planned_finish",
        })
    mapped_status = as_text(candidate.get("mapped_status"))
    tracking_status = as_text(task.get("tracking_status"))
    if mapped_status and mapped_status != tracking_status:
        status_reviews.append({
            "op": "review_only_tracking_status",
            "task_id": task_id,
            "work_package_id": wp_id,
            "replan_tracking_status": tracking_status,
            "openproject_source_status": as_text(candidate.get("source_status")),
            "openproject_mapped_status": mapped_status,
            "reason": "status acceptance is intentionally separate from dates-only soft-drift accept",
        })

def run_replan(command):
    proc = subprocess.run(command, capture_output=True, text=True)
    try:
        answer = json.loads(proc.stdout)
    except (TypeError, ValueError):
        answer = {}
    if proc.returncode != 0 or answer.get("ok") is not True:
        details = answer.get("errors") or proc.stderr.strip() or proc.stdout.strip() or f"exit {proc.returncode}"
        raise RuntimeError(f"RePlan command failed: {details}")
    return answer

applied = []
evidence_item = None
if mode == "apply" and not actor:
    errors.append(helper_envelope.diagnostic(
        "missing_setting", "REPLAN_ACTOR is required when REPLAN_APPLY=1",
        kind="env", target_id="REPLAN_ACTOR",
    ))

if mode == "apply" and not errors and not unmatched_candidates and date_changes:
    now = datetime.now().astimezone()
    observed = now.isoformat(timespec="seconds")
    stamp = now.astimezone(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    evidence_id = f"evidence.openproject.date-accept.{stamp}"
    changed_tasks = sorted({change["task_id"] for change in date_changes})
    evidence_item = {
        "id": evidence_id,
        "type": "import_result",
        "source": "openproject",
        "observed_at": observed,
        "summary": f"Accepted {len(date_changes)} OpenProject planned-date changes.",
        "targets": [f"task:{task_id}" for task_id in changed_tasks],
    }
    evidence_action = {"action": "evidence_add", "evidence_id": evidence_id, "targets": changed_tasks}
    partial_apply.attempting(evidence_action)
    partial_apply.writing()
    evidence_command = [
        args.replan_bin, "evidence", "add", "--project", project_path,
        "--id", evidence_id, "--type", "import_result", "--source", "openproject",
        "--actor", actor, "--observed-at", observed, "--summary", evidence_item["summary"],
    ]
    for task_id in changed_tasks:
        evidence_command.extend(["--target", f"task:{task_id}"])
    evidence_command.extend(["--apply", "--format", "json"])
    run_replan(evidence_command)
    partial_apply.applied(evidence_action)

    changes_by_task = {}
    for change in date_changes:
        changes_by_task.setdefault(change["task_id"], []).append(change)
    for task_id in changed_tasks:
        task_changes = changes_by_task[task_id]
        action = {"action": "task_set_dates", "task_id": task_id, "date_changes": task_changes}
        command = [
            args.replan_bin, "task", "set-dates", "--project", project_path,
            "--task", task_id,
            "--reason", "Accept reviewed OpenProject soft date drift.",
            "--actor", actor, "--evidence", evidence_id, "--occurred-at", observed,
        ]
        for change in task_changes:
            if change["op"] == "update_task_planned_start":
                command.extend(["--planned-start", change["to"]])
            elif change["op"] == "update_task_planned_finish":
                command.extend(["--planned-finish", change["to"]])
        command.extend(["--apply", "--format", "json"])
        partial_apply.attempting(action)
        partial_apply.writing()
        run_replan(command)
        applied.extend(task_changes)
        partial_apply.applied(action)

ok = data.get("ok") is True and not errors and not unmatched_candidates
out = {
    "ok": ok,
    "command": "accept-openproject-soft-drift",
    "mode": mode,
    "project_path": project_path,
    "target_openproject_project": {"identifier": openproject_project_id},
    "actor": actor,
    "private_binding_layer": {
        "path": bindings_path,
        "loaded": bindings_loaded,
        "task_bindings": len(task_id_by_wp),
    },
    "policy": {
        "accepted_fields": ["planned_start", "planned_finish"],
        "review_only_fields": ["tracking_status"],
        "rule": "Accept OpenProject schedule dates into RePlan only through explicit dry-run/apply and audited replan task set-dates writes. Do not accept tracking status in this dates-only helper.",
    },
    "counts": {
        "date_changes": len(date_changes),
        "applied_date_changes": len(applied),
        "status_reviews": len(status_reviews),
        "unmatched_candidates": len(unmatched_candidates),
    },
    "date_changes": date_changes,
    "status_reviews": status_reviews,
    "unmatched_candidates": unmatched_candidates,
    "warning_codes": sorted({w.get("code", "") for w in warnings if w.get("code")}),
    "error_codes": sorted({e.get("code", "") for e in errors if e.get("code")}),
    "operator_next_steps": [
        "Review date_changes in dry-run output before applying.",
        "Run with REPLAN_APPLY=1 to accept OpenProject dates into RePlan.",
        "Run check-openproject-drift after apply.",
        "Handle status_reviews in a separate explicit status-accept decision.",
    ],
    "temp_retained": bool(keep_temp),
}
if evidence_item is not None:
    out["evidence_item"] = evidence_item
if keep_temp:
    out["outputs_dir"] = tmp_dir
if mode == "apply" and any(error.get("code") == "missing_setting" for error in errors):
    helper_envelope.emit(out, helper_envelope.EXIT["usage"], mode)
helper_envelope.emit(out, helper_envelope.apply_exit_code(out, len(applied)), mode)
