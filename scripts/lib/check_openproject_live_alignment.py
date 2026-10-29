"""Moved from scripts/check-openproject-live-alignment.sh (GAP-018). Invoked as `python3 -m check_openproject_live_alignment`."""
import json
import os
import sys

import yaml
import helper_envelope
import openproject_bindings
from openproject_http import Settings

parser = helper_envelope.internal_parser("check-openproject-live-alignment")
parser.add_argument("--import-json", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--root-dir", required=True)
args = parser.parse_args()
root_dir = args.root_dir
settings = Settings.from_env(root_dir=root_dir)
import_path, tmp_dir = args.import_json, args.tmp_dir
project_path, profile, keep_temp = settings.project_path, settings.profile, settings.keep_temp
openproject_project_id, bindings_path = settings.project_id, settings.bindings_path
helper_envelope.guard("check-openproject-live-alignment")
data = json.load(open(import_path))
tasks_path = os.path.join(project_path, "canonical", "tasks.yaml")
tasks_data = yaml.safe_load(open(tasks_path)) or {}
tasks = tasks_data.get("tasks") or []
result = data.get("result") or {}
warnings = data.get("warnings") or result.get("warnings") or []
errors = data.get("errors") or result.get("errors") or []
candidates = result.get("task_candidates") or []

def norm(value):
    return " ".join(str(value or "").strip().lower().split())

local_by_source = {}
local_by_private_binding = {}
local_by_title = {}
for task in tasks:
    task_id = task.get("id", "")
    title = task.get("title", "")
    if title:
        local_by_title.setdefault(norm(title), []).append(task_id)
    for ref in task.get("source_refs") or []:
        if ref.get("source") == "openproject" and ref.get("object_type") == "work_package":
            object_id = str(ref.get("object_id") or "")
            if object_id:
                local_by_source[object_id] = task_id

if not bindings_path and openproject_project_id:
    bindings_path = os.path.join(root_dir, "private", "openproject", f"{openproject_project_id}-bindings.yaml")

bindings_loaded = False
if bindings_path and os.path.exists(bindings_path):
    bindings_data = openproject_bindings.load(bindings_path)
    _, local_by_private_binding = openproject_bindings.task_maps(
        bindings_data, zero_is_missing=True
    )
    bindings_loaded = bool(local_by_private_binding)

private_binding_matches = []
source_matches = []
title_matches = []
live_unmatched = []
for candidate in candidates:
    source_id = str(candidate.get("source_id") or "")
    title = candidate.get("title", "")
    if source_id in local_by_private_binding:
        private_binding_matches.append({
            "source_id": source_id,
            "task_id": local_by_private_binding[source_id],
            "title": title,
        })
        continue
    if source_id in local_by_source:
        source_matches.append({
            "source_id": source_id,
            "task_id": local_by_source[source_id],
            "title": title,
        })
        continue
    title_ids = local_by_title.get(norm(title), [])
    if len(title_ids) == 1:
        title_matches.append({
            "source_id": source_id,
            "task_id": title_ids[0],
            "title": title,
        })
        continue
    live_unmatched.append({
        "source_id": source_id,
        "title": title,
        "source_status": candidate.get("source_status", ""),
        "mapped_status": candidate.get("mapped_status", ""),
    })

matched_task_ids = {item["task_id"] for item in private_binding_matches + source_matches + title_matches}
local_unmatched = [
    {"task_id": task.get("id", ""), "title": task.get("title", "")}
    for task in tasks
    if task.get("id", "") not in matched_task_ids
]

if len(private_binding_matches) == len(candidates) and len(local_unmatched) == 0:
    status = "aligned_by_private_binding"
elif len(source_matches) == len(candidates) and len(local_unmatched) == 0:
    status = "aligned_by_source_ref"
elif len(private_binding_matches) + len(source_matches) + len(title_matches) == len(candidates) and len(local_unmatched) == 0:
    status = "alignable_by_title"
elif private_binding_matches or source_matches or title_matches:
    status = "partial"
else:
    status = "not_current_self_dogfood_plan"

decision = {
    "not_current_self_dogfood_plan": "Create or update a live OpenProject project from the current RePlan self-dogfood task list, or validate the old pilot through a separate fixture/project.",
    "partial": "Review title/source-ref mismatches before accepting source refs or updating fixtures.",
    "alignable_by_title": "Run import accept on a temporary copy first; if acceptable, decide whether source refs belong in private live state or public fixture.",
    "aligned_by_private_binding": "Live OpenProject work packages match ignored private bindings; keep public canonical files clean and proceed to relation/time/resource validation.",
    "aligned_by_source_ref": "Live OpenProject source refs match the RePlan project; proceed to dependency, time-entry and resource-demand validation.",
}[status]

out = {
    "ok": data.get("ok") is True,
    "command": "check-openproject-live-alignment",
    "live_alignment": status,
    "project_path": project_path,
    "profile": profile,
    "counts": {
        "local_tasks": len(tasks),
        "live_task_candidates": len(candidates),
        "matched_by_private_binding": len(private_binding_matches),
        "matched_by_source_ref": len(source_matches),
        "matched_by_title": len(title_matches),
        "live_unmatched": len(live_unmatched),
        "local_unmatched": len(local_unmatched),
        "dependency_candidates": len(result.get("dependency_candidates") or []),
        "time_entry_candidates": len(result.get("time_entry_candidates") or []),
        "resource_candidates": len(result.get("resource_candidates") or []),
    },
    "warning_codes": sorted({w.get("code", "") for w in warnings if w.get("code")}),
    "error_codes": sorted({e.get("code", "") for e in errors if e.get("code")}),
    "suggested_operator_decision": decision,
    "samples": {
        "matched_by_private_binding": private_binding_matches[:10],
        "live_unmatched": live_unmatched[:10],
        "local_unmatched": local_unmatched[:10],
        "matched_by_title": title_matches[:10],
    },
    "private_binding_layer": {
        "path": bindings_path,
        "loaded": bindings_loaded,
    },
    "temp_retained": bool(keep_temp),
}
if keep_temp:
    out["outputs_dir"] = tmp_dir

helper_envelope.emit(out, None, indent=None if os.environ.get("REPLAN_COMPACT_JSON") else 2)
