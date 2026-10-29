"""Build the RePlan overlay artifact for a live OpenProject instance.

The committed self-dogfood project carries sanitized public work-package ids.
The live OpenProject plugin matches rows by real work-package ids, which live
only in the ignored private binding file. This module runs
`replan report dashboard` with the CLI's read-only bindings layer and checks
that every bound task is linked in the resulting overlay.

It never contacts OpenProject and never publishes anything. Its JSON output
carries counts only, no private ids or URLs.
"""

import json
import os
import subprocess
import sys

import yaml

import helper_envelope
import openproject_bindings
from openproject_http import Settings


def load_bindings(path):
    data = openproject_bindings.load(path, required=True)
    return openproject_bindings.task_source_refs(data, strict=False)


def binding_counts(tasks_path, bindings):
    with open(tasks_path, encoding="utf-8") as handle:
        doc = yaml.safe_load(handle) or {}
    bound = 0
    unbound = []
    for task in doc.get("tasks") or []:
        task_id = task.get("id")
        if bindings.get(task_id):
            bound += 1
        else:
            unbound.append(task_id)
    return bound, unbound


def build(replan_bin, project_path, bindings_path, output_dir, week=""):
    result = {
        "ok": False,
        "command": "build-openproject-live-overlay",
        "mode": "read_only",
        "scope": "local_build_only",
        "publishes": False,
        "bindings_loaded": False,
        "counts": {},
        "errors": [],
    }
    if not os.path.exists(bindings_path):
        result["errors"].append({"code": "bindings_missing", "message": "private binding file not found"})
        return result
    bindings = load_bindings(bindings_path)
    result["bindings_loaded"] = True
    bound, unbound = binding_counts(os.path.join(project_path, "canonical", "tasks.yaml"), bindings)
    cmd = [replan_bin, "report", "dashboard", "--project", project_path,
           "--bindings", bindings_path, "--output", output_dir, "--format", "json"]
    if week:
        cmd += ["--week", week]
    run = subprocess.run(cmd, capture_output=True, text=True)
    if run.returncode != 0:
        result["errors"].append({"code": "dashboard_failed", "message": (run.stderr or run.stdout)[-500:]})
        return result
    overlay_path = os.path.join(output_dir, "raw", "openproject_overlay.json")
    with open(overlay_path, encoding="utf-8") as handle:
        overlay = json.load(handle)
    counts = {
        "tasks_bound": bound,
        "tasks_unbound": len(unbound),
        "bindings": len(bindings),
        "work_package_count": overlay.get("work_package_count"),
        "linked_task_count": overlay.get("linked_task_count"),
        "missing_link_count": overlay.get("missing_link_count"),
    }
    result["counts"] = counts
    result["unbound_task_ids"] = unbound
    if unbound:
        result["errors"].append({"code": "unbound_tasks", "message": f"{len(unbound)} tasks have no private binding"})
    if counts["missing_link_count"] != 0 or counts["linked_task_count"] != bound:
        result["errors"].append({"code": "overlay_links_incomplete", "message": "overlay does not link every bound task"})
    result["ok"] = not result["errors"]
    result["output_dir"] = output_dir
    return result


def main(argv):
    helper_envelope.guard("build-openproject-live-overlay")
    parser = helper_envelope.internal_parser("build-openproject-live-overlay")
    parser.add_argument("--replan-bin", required=True)
    parser.add_argument("--output-dir", required=True)
    parser.add_argument("--root-dir", required=True)
    args = parser.parse_args(argv[1:])
    settings = Settings.from_env(root_dir=args.root_dir)
    replan_bin, project_path = args.replan_bin, settings.project_path
    bindings_path, output_dir = settings.bindings_path, args.output_dir
    week = os.environ.get("REPLAN_WEEK", "")
    result = build(replan_bin, project_path, bindings_path, output_dir, week)
    print(helper_envelope.dumps(result, sort_keys=True))
    return helper_envelope.exit_code_for(helper_envelope.normalize(result))


if __name__ == "__main__":
    sys.exit(main(sys.argv))
