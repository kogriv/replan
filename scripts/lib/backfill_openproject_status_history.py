"""Backfill RePlan status audits from OpenProject work-package activities.

OpenProject 17 renders journal details as formattables only.  We consume each
detail's ``raw`` value (never localized HTML) and match status names obtained
from ``GET /api/v3/statuses``.  Structured values and status links are also
accepted for forward compatibility.
"""

from datetime import datetime, timezone
import glob
import json
import os
import re
import shlex
import shutil
import subprocess
import urllib.parse

import helper_envelope
import openproject_bindings
from openproject_http import OpenProjectClient, Settings
from openproject_status_map import OPENPROJECT_TO_TRACKING_STATUS


COMMAND = "backfill-openproject-status-history"
PAGE_SIZE = 100
PRESENT_TOLERANCE_SECONDS = 60
FORWARD_RANK = {
    "not_started": 0,
    "ready": 1,
    "in_progress": 2,
    "review_ready": 3,
    "accepted": 4,
}
TERMINAL = {"accepted", "cancelled"}

parser = helper_envelope.internal_parser(COMMAND)
parser.add_argument("--root-dir", required=True)
parser.add_argument("--replan-bin", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--task", action="append", default=[])
args = parser.parse_args()

settings = Settings.from_env(require_token=True, root_dir=args.root_dir)
mode = "apply" if settings.apply == "1" else "dry_run"
actor = os.environ.get("REPLAN_ACTOR", "").strip()
partial_apply = helper_envelope.PartialApply()
helper_envelope.guard(
    COMMAND,
    mode,
    "openproject_status_history_backfill_failed",
    partial_apply=partial_apply,
    partial_next_steps=(
        "Inspect applied_so_far and failed_step, then rerun the helper in dry-run mode.",
    ),
)

client = OpenProjectClient(
    settings.base_url,
    settings,
    settings.host_header,
    settings.forwarded_proto,
    default_expected=(200,),
)


def parse_time(value):
    text = str(value or "")
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    try:
        return datetime.fromisoformat(text).astimezone(timezone.utc)
    except ValueError:
        return None


def load_tasks(project_path):
    import yaml
    with open(os.path.join(project_path, "canonical", "tasks.yaml"), encoding="utf-8") as stream:
        document = yaml.safe_load(stream) or {}
    return {
        str(task.get("id")): task
        for task in document.get("tasks") or []
        if isinstance(task, dict) and task.get("id")
    }


def default_human_actor(project_path):
    import yaml
    with open(os.path.join(project_path, "canonical", "actors.yaml"), encoding="utf-8") as stream:
        document = yaml.safe_load(stream) or {}
    for item in document.get("actors") or []:
        if isinstance(item, dict) and item.get("kind") == "human" and item.get("id"):
            return str(item["id"])
    return ""


def load_status_audits(project_path):
    records = []
    for path in sorted(glob.glob(os.path.join(project_path, "audit", "*.jsonl"))):
        with open(path, encoding="utf-8") as stream:
            for line in stream:
                if not line.strip():
                    continue
                record = json.loads(line)
                if record.get("operation") == "actual_add_status" and record.get("new_status"):
                    records.append(record)
    return records


def status_id_from_href(href):
    match = re.search(r"/api/v3/statuses/([^/?#]+)", str(href or ""))
    return urllib.parse.unquote(match.group(1)) if match else ""


def value_name(value, status_by_id):
    if isinstance(value, str):
        status_id = status_id_from_href(value)
        return status_by_id.get(status_id, value)
    if not isinstance(value, dict):
        return ""
    for key in ("name", "title", "label"):
        if value.get(key):
            return str(value[key])
    status_id = status_id_from_href(value.get("href"))
    return status_by_id.get(status_id, "")


def structured_destination(detail, status_by_id):
    marker = str(detail.get("property") or detail.get("field") or detail.get("key") or "")
    if marker and "status" not in marker.casefold():
        return ""
    for key in ("to", "newValue", "new_value", "new", "value"):
        name = value_name(detail.get(key), status_by_id)
        if name:
            return name
    links = detail.get("_links") or {}
    for key in ("to", "newValue", "newStatus", "status"):
        name = value_name(links.get(key), status_by_id)
        if name:
            return name
    return ""


def raw_destination(raw, status_names):
    # A journal status transition contains old and new names. Prefer longer
    # names at the same position and the final occurrence as the destination.
    matches = []
    folded = str(raw or "").casefold()
    for name in status_names:
        pattern = r"(?<!\w)" + re.escape(name.casefold()) + r"(?!\w)"
        for found in re.finditer(pattern, folded):
            matches.append((found.start(), found.end(), len(name), name))
    if len(matches) < 2:
        return ""
    matches.sort(key=lambda item: (item[0], item[1], item[2]))
    return matches[-1][3]


def status_change(activity, status_by_id, status_names):
    for detail in activity.get("details") or []:
        if not isinstance(detail, dict):
            continue
        destination = structured_destination(detail, status_by_id)
        if destination and destination not in status_names:
            destination = ""
        source = "structured_detail" if destination else ""
        if not destination:
            destination = raw_destination(detail.get("raw"), status_names)
            source = "raw_detail" if destination else ""
        if destination:
            return destination, source
    return "", ""


def evidence_refs(record):
    refs = record.get("evidence_refs") or []
    for ref in refs:
        if isinstance(ref, dict) and ref.get("id"):
            return str(ref["id"])
    return ""


def conflict_with_later(status, occurred, existing):
    for record in existing:
        later = parse_time(record.get("occurred_at"))
        if later is None or later <= occurred:
            continue
        later_status = str(record.get("new_status") or "")
        if later_status == status:
            continue
        if status in TERMINAL:
            return record
        if status in FORWARD_RANK and later_status in FORWARD_RANK:
            if FORWARD_RANK[status] > FORWARD_RANK[later_status]:
                return record
    return None


def status_in_effect(occurred, existing):
    candidates = []
    for record in existing:
        at = parse_time(record.get("occurred_at"))
        if at is not None and at <= occurred:
            candidates.append((at, str(record.get("id") or ""), record))
    if not candidates:
        return None
    candidates.sort(key=lambda item: (item[0], item[1]))
    return candidates[-1][2]


def run_replan(command):
    proc = subprocess.run(command, capture_output=True, text=True)
    try:
        answer = json.loads(proc.stdout)
    except (TypeError, ValueError):
        answer = {}
    if proc.returncode != 0 or answer.get("ok") is not True:
        detail = answer.get("errors") or proc.stderr.strip() or proc.stdout.strip() or f"exit {proc.returncode}"
        raise RuntimeError(f"RePlan command failed: {detail}")
    return answer


def display_command(parts):
    shown = ["replan", *parts[1:]]
    return shlex.join(shown)


def command_for_project(command, project_path, command_actor):
    result = list(command)
    result[result.index("--project") + 1] = project_path
    result[result.index("--actor") + 1] = command_actor
    return result


def main():
    project_path = settings.project_path
    task_by_id = load_tasks(project_path)
    bindings = openproject_bindings.load(settings.bindings_path, required=True, validate=True)
    task_to_wp, _ = openproject_bindings.task_maps(bindings)

    requested = []
    for task_id in args.task:
        if task_id not in requested:
            requested.append(task_id)
    task_ids = requested or [task_id for task_id in task_to_wp if task_id in task_by_id]
    unknown = [task_id for task_id in task_ids if task_id not in task_by_id or task_id not in task_to_wp]
    if unknown:
        helper_envelope.emit({
            "ok": False,
            "command": COMMAND,
            "mode": "invalid",
            "errors": [helper_envelope.diagnostic(
                "unknown_task_ids",
                "unknown or unbound task ids: " + ", ".join(unknown),
            )],
            "unknown_task_ids": unknown,
        }, helper_envelope.EXIT["usage"])

    statuses = client.paginate("/api/v3/statuses", PAGE_SIZE)
    status_by_id = {
        str(item.get("id")): str(item.get("name"))
        for item in statuses
        if item.get("id") is not None and item.get("name")
    }
    status_names = sorted(set(status_by_id.values()), key=lambda value: (-len(value), value))
    existing = load_status_audits(project_path)
    existing_by_task = {}
    for record in existing:
        task_id = str((record.get("target_ref") or {}).get("id") or "")
        existing_by_task.setdefault(task_id, []).append(record)

    counts = {
        "tasks_read": 0,
        "status_changes_found": 0,
        "already_present": 0,
        "proposed": 0,
        "applied": 0,
        "unmapped": 0,
        "conflicts": 0,
    }
    proposals = []
    unmapped = []
    conflicts = []
    already_present = []
    task_rows = []
    for task_id in task_ids:
        work_package_id = str(task_to_wp[task_id])
        activities = client.paginate(
            f"/api/v3/work_packages/{urllib.parse.quote(work_package_id, safe='')}/activities",
            PAGE_SIZE,
        )
        counts["tasks_read"] += 1
        task_rows.append({"task_id": task_id, "work_package_id": work_package_id})
        for activity in activities:
            destination, detail_source = status_change(activity, status_by_id, status_names)
            if not destination:
                continue
            counts["status_changes_found"] += 1
            journal_id = str(activity.get("id") or "")
            created_at = str(activity.get("createdAt") or "")
            occurred = parse_time(created_at)
            base = {
                "task_id": task_id,
                "work_package_id": work_package_id,
                "journal_id": journal_id,
                "occurred_at": created_at,
                "openproject_status": destination,
                "detail_source": detail_source,
            }
            mapped = OPENPROJECT_TO_TRACKING_STATUS.get(destination)
            if not mapped:
                counts["unmapped"] += 1
                unmapped.append({**base, "reason": "openproject_status_unmapped"})
                continue
            base["tracking_status"] = mapped
            if occurred is None:
                counts["conflicts"] += 1
                conflicts.append({**base, "reason": "invalid_journal_created_at"})
                continue
            present = None
            for record in existing_by_task.get(task_id, []):
                at = parse_time(record.get("occurred_at"))
                if record.get("new_status") == mapped and at is not None and abs((at - occurred).total_seconds()) <= PRESENT_TOLERANCE_SECONDS:
                    present = record
                    break
            if present:
                counts["already_present"] += 1
                already_present.append({**base, "audit_id": present.get("id", ""), "match": "same_time"})
                continue
            in_effect = status_in_effect(occurred, existing_by_task.get(task_id, []))
            if in_effect and in_effect.get("new_status") == mapped:
                counts["already_present"] += 1
                already_present.append({
                    **base,
                    "audit_id": in_effect.get("id", ""),
                    "audit_occurred_at": in_effect.get("occurred_at", ""),
                    "match": "in_effect",
                })
                continue
            conflict = conflict_with_later(mapped, occurred, existing_by_task.get(task_id, []))
            if conflict:
                counts["conflicts"] += 1
                conflicts.append({
                    **base,
                    "reason": "later_replan_status_order_conflict",
                    "later_audit_id": conflict.get("id", ""),
                    "later_status": conflict.get("new_status", ""),
                    "later_occurred_at": conflict.get("occurred_at", ""),
                })
                continue

            safe_journal = re.sub(r"[^A-Za-z0-9._-]+", "-", journal_id).strip("-") or "unknown"
            evidence_id = f"evidence.openproject.wp-{work_package_id}.journal-{safe_journal}"
            summary = f"Imported OpenProject work package {work_package_id} journal {journal_id} status at {created_at}."
            evidence = [
                args.replan_bin, "evidence", "add", "--project", project_path,
                "--id", evidence_id, "--type", "import", "--source", "openproject",
                "--actor", actor or "<actor-id>", "--observed-at", created_at,
                "--summary", summary, "--target", f"task:{task_id}",
                "--apply", "--format", "json",
            ]
            add_status = [
                args.replan_bin, "actual", "add-status", "--project", project_path,
                "--task", task_id, "--status", mapped,
                "--reason", f"Backfill OpenProject work package {work_package_id} journal {journal_id}.",
                "--evidence", evidence_id, "--actor", actor or "<actor-id>",
                "--occurred-at", created_at, "--apply", "--format", "json",
            ]
            proposals.append({
                **base,
                "evidence_id": evidence_id,
                "commands": [display_command(evidence), display_command(add_status)],
                "_evidence_argv": evidence,
                "_status_argv": add_status,
                "_occurred": occurred,
            })

    proposals.sort(key=lambda item: (task_ids.index(item["task_id"]), item["_occurred"], item["journal_id"]))
    counts["proposed"] = len(proposals)

    errors = []
    applied = []
    if mode == "apply" and not actor:
        errors.append(helper_envelope.diagnostic(
            "missing_setting", "REPLAN_ACTOR is required when REPLAN_APPLY=1", kind="env", target_id="REPLAN_ACTOR"
        ))
    if mode == "apply" and not errors:
        for proposal in proposals:
            public = {key: value for key, value in proposal.items() if not key.startswith("_")}
            evidence_action = {"action": "evidence_add", **public}
            partial_apply.attempting(evidence_action)
            partial_apply.writing()
            run_replan(proposal["_evidence_argv"])
            partial_apply.applied(evidence_action)
            status_action = {"action": "actual_add_status", **public}
            partial_apply.attempting(status_action)
            partial_apply.writing()
            run_replan(proposal["_status_argv"])
            partial_apply.applied(status_action)
            applied.append(public)
            counts["applied"] += 1

    preview_project = project_path
    preview_actor = actor or default_human_actor(project_path)
    preview_error = ""
    preview_replayed = mode == "apply" or not proposals
    if mode == "dry_run" and proposals and preview_actor:
        preview_project = os.path.join(args.tmp_dir, "status-history-preview-project")
        shutil.copytree(project_path, preview_project)
        try:
            for proposal in proposals:
                run_replan(command_for_project(proposal["_evidence_argv"], preview_project, preview_actor))
                run_replan(command_for_project(proposal["_status_argv"], preview_project, preview_actor))
            preview_replayed = True
        except RuntimeError as exc:
            preview_error = str(exc)
    elif mode == "dry_run" and proposals:
        preview_error = "No human actor is available for the temporary status-history replay."

    window_previews = []
    evidence_for_task = {}
    for proposal in proposals:
        evidence_for_task[proposal["task_id"]] = proposal["evidence_id"]
    for task_id, records in existing_by_task.items():
        for record in sorted(records, key=lambda row: str(row.get("occurred_at") or ""), reverse=True):
            if task_id not in evidence_for_task and evidence_refs(record):
                evidence_for_task[task_id] = evidence_refs(record)
    for row in task_rows:
        evidence_id = evidence_for_task.get(row["task_id"], "<evidence-id>")
        command = [
            args.replan_bin, "task", "record-actual-window", "--project", project_path,
            "--task", row["task_id"], "--actor", actor or preview_actor or "<actor-id>",
            "--evidence", evidence_id, "--dry-run", "--format", "json",
        ]
        preview = {**row, "command": display_command(command), "result": None}
        if evidence_id != "<evidence-id>" and preview_replayed and not preview_error:
            preview_command = command_for_project(command, preview_project, actor or preview_actor)
            proc = subprocess.run(preview_command, capture_output=True, text=True)
            try:
                preview["result"] = json.loads(proc.stdout)
                preview["result_source"] = (
                    "temporary_project_after_proposals" if mode == "dry_run" and proposals
                    else "current_project_after_apply"
                )
            except ValueError:
                preview["result"] = {"ok": False, "stderr": proc.stderr.strip(), "exit_code": proc.returncode}
        else:
            preview["preview_status"] = "simulation_failed" if preview_error else "evidence_required"
            if preview_error:
                preview["preview_error"] = preview_error
        window_previews.append(preview)

    public_proposals = [
        {key: value for key, value in proposal.items() if not key.startswith("_")}
        for proposal in proposals
    ]
    warnings = []
    if unmapped:
        warnings.append(helper_envelope.diagnostic(
            "openproject_status_unmapped",
            f"{len(unmapped)} OpenProject status changes are not mapped and were not guessed.",
            "warning",
        ))
    if conflicts:
        warnings.append(helper_envelope.diagnostic(
            "status_history_conflict",
            f"{len(conflicts)} status changes conflict with later RePlan history and require review.",
            "warning",
        ))
    helper_envelope.emit({
        "ok": not errors,
        "command": COMMAND,
        "mode": mode,
        "counts": counts,
        "status_detail_source": "OpenProject 17 details[].raw matched against GET /api/v3/statuses; structured details/links are preferred when present; details[].html is never read.",
        "tasks": task_rows,
        "proposals": public_proposals,
        "applied": applied,
        "already_present": already_present,
        "unmapped": unmapped,
        "conflicts": conflicts,
        "actual_window_previews": window_previews,
        "warnings": warnings,
        "errors": errors,
    }, helper_envelope.EXIT["usage"] if errors else 0)


if __name__ == "__main__":
    main()
