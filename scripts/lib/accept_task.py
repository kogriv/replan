"""Resumable, dry-run-first acceptance chain for one RePlan task."""
import json
import hashlib
import math
import os
import re
import shutil
import subprocess
from datetime import datetime

import yaml

import helper_envelope
from openproject_http import Settings


COMMAND = "accept-task"
REASON = "Task accepted by the operator"
TERMINAL_DEMAND_STATUSES = {"consumed", "released", "cancelled", "superseded"}


parser = helper_envelope.internal_parser(COMMAND)
parser.add_argument("--root-dir", required=True)
parser.add_argument("--tmp-dir", required=True)
parser.add_argument("--replan-bin", required=True)
args = parser.parse_args()

mode = "apply" if os.environ.get("REPLAN_APPLY") == "1" else "dry_run"
partial_apply = helper_envelope.PartialApply()
helper_envelope.guard(
    COMMAND,
    mode,
    partial_apply=partial_apply,
    partial_next_steps=(
        "Inspect applied_so_far and failed_step, then run accept-task again.",
        "The helper recomputes every done-state from RePlan and OpenProject.",
    ),
)


class StepFailure(RuntimeError):
    def __init__(self, message, *, code="accept_task_step_failed", exit_code=1, answer=None):
        super().__init__(message)
        self.code = code
        self.exit_code = exit_code
        self.answer = answer or {}


def load_yaml(path):
    with open(path, encoding="utf-8") as stream:
        return yaml.safe_load(stream) or {}


def documents(directory, key):
    if not os.path.isdir(directory):
        return []
    items = []
    for name in sorted(os.listdir(directory)):
        if name.endswith((".yaml", ".yml")):
            items.extend(load_yaml(os.path.join(directory, name)).get(key) or [])
    return items


def diagnostic(code, message, kind="", target_id=""):
    return helper_envelope.diagnostic(code, message, "error", kind, target_id)


def warning(code, message, kind="", target_id=""):
    return helper_envelope.diagnostic(code, message, "warning", kind, target_id)


def refuse(code, message, exit_code, *, kind="", target_id="", extra=None):
    out = {
        "ok": False,
        "command": COMMAND,
        "mode": "invalid",
        "errors": [diagnostic(code, message, kind, target_id)],
    }
    if extra:
        out.update(extra)
    helper_envelope.emit(out, exit_code, mode)


def slug(value):
    value = re.sub(r"[^A-Za-z0-9]+", "-", value).strip("-").lower()
    return value or "unknown"


def default_evidence_id(task_id):
    parts = task_id.split(".")
    namespace = slug(parts[1]) if len(parts) >= 3 else "task"
    task_part = ".".join(parts[2:]) if len(parts) >= 3 else task_id.removeprefix("task.")
    return f"evidence.{namespace}.accept-{slug(task_part)}"


def parse_rfc3339(value):
    candidate = value[:-1] + "+00:00" if value.endswith("Z") else value
    try:
        parsed = datetime.fromisoformat(candidate)
    except ValueError:
        parsed = None
    if parsed is None or parsed.tzinfo is None or parsed.utcoffset() is None:
        refuse("invalid_accept_at", f"REPLAN_ACCEPT_AT {value!r} must be RFC3339 with an offset", 8, kind="env", target_id="REPLAN_ACCEPT_AT")
    return parsed


def parse_amount(value):
    try:
        amount = float(value)
    except ValueError:
        amount = math.nan
    if not math.isfinite(amount) or amount < 0:
        refuse("invalid_accept_ci_minutes", "REPLAN_ACCEPT_CI_MINUTES must be a finite number zero or greater", 8, kind="env", target_id="REPLAN_ACCEPT_CI_MINUTES")
    return amount


def display_amount(value):
    return str(int(value)) if float(value).is_integer() else format(value, ".15g")


def replace_project(command, project):
    result = list(command)
    index = result.index("--project") + 1
    result[index] = project
    return result


def with_write_choice(command, choice):
    return [arg for arg in command if arg not in {"--dry-run", "--apply"}] + [choice]


def run_json(command, *, env=None, allow_skipped=False):
    proc = subprocess.run(command, capture_output=True, text=True, env=env)
    try:
        answer = json.loads(proc.stdout)
    except (TypeError, ValueError):
        raise StepFailure(
            f"command did not return JSON: {command[0]} (exit {proc.returncode})",
            exit_code=proc.returncode or helper_envelope.EXIT["general"],
        )
    if proc.returncode != 0 or (answer.get("ok") is not True and not (allow_skipped and answer.get("mode") == "skipped")):
        errors = answer.get("errors") or []
        code = errors[0].get("code", "accept_task_step_failed") if errors else "accept_task_step_failed"
        message = errors[0].get("message", f"command failed with exit {proc.returncode}") if errors else f"command failed with exit {proc.returncode}"
        raise StepFailure(message, code=code, exit_code=proc.returncode or helper_envelope.EXIT["attention"], answer=answer)
    return answer


def preflight_json(command, *, env=None):
    """Run one validation/read command and preserve its public failure code."""
    try:
        return run_json(command, env=env)
    except StepFailure as exc:
        extra = {"failed_command": command}
        if exc.answer:
            extra["failed_command_result"] = exc.answer
        refuse(exc.code, str(exc), exc.exit_code, extra=extra)


def step(number, name, state, reason, command=None, **extra):
    item = {"step": number, "name": name, "state": state, "reason": reason}
    if command:
        item["command"] = command
    item.update(extra)
    return item


def actual_id(task_id, resource_id, occurred_at):
    compact = occurred_at.replace("-", "").replace(":", "").replace("+", "")
    if "." in compact:
        compact = compact.split(".", 1)[0]
    if "Z" in compact:
        compact = compact.split("Z", 1)[0] + "Z"
    return f"actual.{slug(task_id.removeprefix('task.'))}.{slug(resource_id.removeprefix('resource.'))}.{compact}.work-consumed"


def action_for(command, number, name, reason, **extra):
    return step(number, name, "would_run", reason, command, **extra)


project = os.path.abspath(os.environ["REPLAN_PROJECT"])
task_id = os.environ["REPLAN_TASK"].strip()
accepted_by = os.environ["REPLAN_ACCEPTED_BY"].strip()
recorder = os.environ.get("REPLAN_ACTOR", "").strip()
summary = os.environ["REPLAN_ACCEPT_SUMMARY"].strip()
accept_at = os.environ.get("REPLAN_ACCEPT_AT", "").strip() or datetime.now().astimezone().isoformat(timespec="seconds")
parse_rfc3339(accept_at)
ci_minutes = parse_amount(os.environ.get("REPLAN_ACCEPT_CI_MINUTES", "0").strip() or "0")
evidence_id = os.environ.get("REPLAN_ACCEPT_EVIDENCE_ID", "").strip() or default_evidence_id(task_id)

if not summary:
    refuse("missing_setting", "required setting not set: REPLAN_ACCEPT_SUMMARY", 8, kind="env", target_id="REPLAN_ACCEPT_SUMMARY", extra={"missing_settings": ["REPLAN_ACCEPT_SUMMARY"]})
if mode == "apply" and not recorder:
    refuse("missing_setting", "REPLAN_ACTOR is required when REPLAN_APPLY=1", 8, kind="env", target_id="REPLAN_ACTOR", extra={"missing_settings": ["REPLAN_ACTOR"]})

try:
    tasks = load_yaml(os.path.join(project, "canonical", "tasks.yaml")).get("tasks") or []
    demands = load_yaml(os.path.join(project, "canonical", "resource_demands.yaml")).get("resource_demands") or []
    actors = load_yaml(os.path.join(project, "canonical", "actors.yaml")).get("actors") or []
except FileNotFoundError as exc:
    refuse("project_file_not_found", str(exc), 4, kind="project", target_id=project)

task = next((item for item in tasks if item.get("id") == task_id), None)
if task is None:
    refuse("task_not_found", f"task {task_id} was not found", 4, kind="task", target_id=task_id)
actor_ids = {item.get("id") for item in actors}
for actor_id, setting in ((accepted_by, "REPLAN_ACCEPTED_BY"), (recorder, "REPLAN_ACTOR")):
    if actor_id and actor_id not in actor_ids:
        refuse("actor_not_found", f"actor {actor_id} from {setting} was not found", 2, kind="actor", target_id=actor_id)

tracking_status = task.get("tracking_status", "")
if tracking_status not in {"review_ready", "accepted"}:
    refuse(
        "task_not_review_ready",
        f"task {task_id} has tracking status {tracking_status!r}; only review_ready or accepted can enter the acceptance chain",
        helper_envelope.EXIT["attention"],
        kind="task",
        target_id=task_id,
        extra={"task_status": tracking_status},
    )

evidence_items = documents(os.path.join(project, "evidence"), "evidence")
evidence_exists = any(item.get("id") == evidence_id for item in evidence_items)
actual_items = documents(os.path.join(project, "actuals"), "actuals")
task_demands = [item for item in demands if (item.get("task_ref") or {}).get("id") == task_id]
ci_demands = [item for item in task_demands if item.get("unit") == "ci_minutes"]
if ci_minutes and len(ci_demands) != 1:
    code = "ci_minutes_demand_not_found" if not ci_demands else "ambiguous_ci_minutes_demand"
    refuse(code, f"task {task_id} must have exactly one ci_minutes demand when REPLAN_ACCEPT_CI_MINUTES is nonzero; found {len(ci_demands)}", 2, kind="task", target_id=task_id)
if ci_minutes and not (ci_demands[0].get("resource_ref") or {}).get("id"):
    refuse("ci_minutes_resource_not_found", f"ci_minutes demand {ci_demands[0].get('id', '')} has no concrete resource_ref", 2, kind="resource_demand", target_id=ci_demands[0].get("id", ""))

settings = Settings.from_env(root_dir=args.root_dir)
token_missing = not settings.token
if mode == "apply" and token_missing:
    refuse("token_env_not_set", f"{settings.token_env} is not set; nothing was applied", 8, kind="env", target_id=settings.token_env, extra={"token_env": settings.token_env})
if mode == "apply" and not settings.from_file and not settings.base_url:
    refuse("missing_setting", "REPLAN_OPENPROJECT_BASE_URL is required under apply", 8, kind="env", target_id="REPLAN_OPENPROJECT_BASE_URL", extra={"missing_settings": ["REPLAN_OPENPROJECT_BASE_URL"]})

toolkit = os.path.join(args.root_dir, "scripts", "openproject-work-package-toolkit.sh")
overlay_build = os.path.join(args.root_dir, "scripts", "build-openproject-live-overlay.sh")
overlay_publish = os.path.join(args.root_dir, "scripts", "publish-openproject-overlay.sh")
base = [args.replan_bin]
common_actual = ["--evidence", evidence_id, "--actor", recorder or accepted_by, "--occurred-at", accept_at, "--format", "json"]

evidence_command = base + [
    "evidence", "add", "--project", project, "--id", evidence_id,
    "--type", "decision", "--subtype", "operator_acceptance", "--source", "chat",
    "--actor", accepted_by, "--observed-at", accept_at, "--confidence", "observed",
    "--summary", summary, "--target", f"task:{task_id}", "--format", "json", "--apply",
]

planned = []
pending = []
if evidence_exists:
    planned.append(step(1, "record_acceptance_evidence", "already_done", f"evidence {evidence_id} already exists", evidence_command))
else:
    item = action_for(evidence_command, 1, "record_acceptance_evidence", "acceptance evidence is absent", evidence_id=evidence_id)
    planned.append(item)
    pending.append(item)

ci_actual_id = ""
if ci_minutes == 0:
    planned.append(step(2, "record_ci_minutes", "skipped", "REPLAN_ACCEPT_CI_MINUTES is 0"))
else:
    ci_demand = ci_demands[0]
    resource_id = ci_demand["resource_ref"]["id"]
    ci_actual_id = actual_id(task_id, resource_id, accept_at)
    ci_command = base + [
        "actual", "add-consumption", "--project", project, "--task", task_id,
        "--demand", ci_demand["id"], "--resource", resource_id,
        "--amount", display_amount(ci_minutes), "--unit", "ci_minutes", *common_actual, "--apply",
    ]
    if any(item.get("id") == ci_actual_id for item in actual_items):
        planned.append(step(2, "record_ci_minutes", "already_done", f"actual {ci_actual_id} already exists", ci_command, actual_id=ci_actual_id))
    else:
        item = action_for(ci_command, 2, "record_ci_minutes", "nonzero CI consumption has not been recorded", demand_id=ci_demand["id"], actual_id=ci_actual_id)
        planned.append(item)
        pending.append(item)

active_demands = [item for item in task_demands if item.get("status", "active") not in TERMINAL_DEMAND_STATUSES]
for demand in active_demands:
    remaining = demand.get("remaining_amount", demand.get("planned_amount"))
    if remaining is not None and float(remaining) > 0:
        command = base + [
            "actual", "set-remaining", "--project", project, "--demand", demand["id"],
            "--amount", "0", "--unit", demand.get("unit", ""), "--reason", REASON,
            *common_actual, "--apply",
        ]
        item = action_for(command, 3, "zero_remaining", f"demand remaining is {remaining}", demand_id=demand["id"])
        planned.append(item)
        pending.append(item)
    else:
        command = base + [
            "actual", "set-remaining", "--project", project, "--demand", demand["id"],
            "--amount", "0", "--unit", demand.get("unit", ""), "--reason", REASON,
            *common_actual, "--apply",
        ]
        planned.append(step(3, "zero_remaining", "already_done", "remaining amount is already 0", command, demand_id=demand["id"]))

for demand in task_demands:
    command = base + [
        "actual", "close-demand", "--project", project, "--demand", demand["id"],
        "--status", "consumed", "--reason", REASON, *common_actual, "--apply",
    ]
    if demand.get("status") == "consumed":
        planned.append(step(4, "close_demand", "already_done", "demand status is consumed", command, demand_id=demand["id"]))
    elif demand.get("status", "active") in TERMINAL_DEMAND_STATUSES:
        planned.append(step(4, "close_demand", "skipped", f"demand has terminal status {demand.get('status')}", command, demand_id=demand["id"]))
    else:
        item = action_for(command, 4, "close_demand", f"demand status is {demand.get('status', 'active')}", demand_id=demand["id"])
        planned.append(item)
        pending.append(item)

status_command = base + [
    "actual", "add-status", "--project", project, "--task", task_id,
    "--status", "accepted", "--evidence", evidence_id, "--actor", accepted_by,
    "--occurred-at", accept_at, "--format", "json", "--apply",
]
if tracking_status == "accepted":
    planned.append(step(5, "accept_tracking_status", "already_done", "task tracking status is accepted", status_command))
else:
    item = action_for(status_command, 5, "accept_tracking_status", "task tracking status is review_ready")
    planned.append(item)
    pending.append(item)

# An accepted task without this chain's evidence predates or bypassed this
# helper. Respect it as a no-op instead of manufacturing retrospective facts.
if tracking_status == "accepted" and not evidence_exists:
    for number, name in ((6, "record_actual_window"), (7, "update_openproject_dates"), (8, "suggest_dependencies"), (9, "update_openproject_status"), (10, "refresh_overlay")):
        planned.append(step(number, name, "skipped", "task was already accepted without this acceptance evidence"))
    helper_envelope.emit({
        "ok": True,
        "command": COMMAND,
        "mode": mode,
        "project_path": project,
        "task_id": task_id,
        "evidence_id": evidence_id,
        "accept_at": accept_at,
        "no_op": True,
        "actions": planned,
        "counts": {"planned": 0, "already_done": len([item for item in planned if item["state"] == "already_done"]), "skipped": len([item for item in planned if item["state"] == "skipped"]), "applied": 0},
        "operator_next_steps": [],
    }, 0, mode)

# Preflight steps 1-5 in order against a disposable copy. Each pending CLI
# mutation is called with --dry-run, then applied only to the copy so later
# dry-runs see the evidence and lifecycle state they will receive in reality.
scratch = os.path.join(args.tmp_dir, "project-preflight")
shutil.copytree(project, scratch)
for item in pending:
    if item["step"] > 5:
        continue
    dry_command = with_write_choice(replace_project(item["command"], scratch), "--dry-run")
    item["cli_dry_run"] = preflight_json(dry_command)
    preflight_json(with_write_choice(replace_project(item["command"], scratch), "--apply"))

actual_window_command = base + [
    "task", "record-actual-window", "--project", project, "--task", task_id,
    "--actor", recorder or accepted_by, "--evidence", evidence_id,
    "--reason", REASON, "--occurred-at", accept_at, "--format", "json", "--apply",
]
actual_window_preview_command = with_write_choice(replace_project(actual_window_command, scratch), "--dry-run")
actual_window_warning = None
try:
    actual_window_preview = run_json(actual_window_preview_command)
except StepFailure as exc:
    if exc.code != "actual_window_unknown":
        extra = {"failed_command": actual_window_preview_command}
        if exc.answer:
            extra["failed_command_result"] = exc.answer
        refuse(exc.code, str(exc), exc.exit_code, extra=extra)
    actual_window_preview = exc.answer
    actual_window_step = step(6, "record_actual_window", "skipped", str(exc), actual_window_command, cli_dry_run=actual_window_preview)
    actual_window_warning = warning("actual_window_unknown", f"task {task_id} has no actual dates; recording its actual window was skipped", "task", task_id)
else:
    actual_window_result = actual_window_preview.get("result") or {}
    if actual_window_result.get("changed"):
        actual_window_step = action_for(actual_window_command, 6, "record_actual_window", "derived actual dates differ from the planned dates", cli_dry_run=actual_window_preview)
    else:
        actual_window_step = step(6, "record_actual_window", "already_done", "planned dates already match the derived actual window", actual_window_command, cli_dry_run=actual_window_preview)
planned.append(actual_window_step)

proposal_preview_command = base + ["proposal", "suggest-dependencies", "--project", scratch, "--actor", recorder or accepted_by, "--dry-run", "--format", "json"]
proposal_preview = preflight_json(proposal_preview_command)
proposal_ids = [item.get("id") for item in (proposal_preview.get("result") or {}).get("proposals") or [] if item.get("id")]
proposal_command = base + ["proposal", "suggest-dependencies", "--project", project, "--write-candidates", "--actor", recorder or accepted_by, "--format", "json"]
if proposal_ids:
    proposal_step = action_for(proposal_command, 8, "suggest_dependencies", f"{len(proposal_ids)} new proposal candidate(s) are available", proposal_ids=proposal_ids, cli_dry_run=proposal_preview)
else:
    proposal_step = step(8, "suggest_dependencies", "already_done", "no new dependency proposal candidates", proposal_command, proposal_ids=[], cli_dry_run=proposal_preview)

toolkit_dry_env = dict(os.environ)
toolkit_dry_env.pop("REPLAN_APPLY", None)
actual_window_result = actual_window_preview.get("result") or {}
actual_start = actual_window_result.get("new_planned_start", "")
actual_finish = actual_window_result.get("new_planned_finish", "")
date_command = [toolkit, "work-package", "update-dates", "--task", task_id, "--start-date", actual_start, "--due-date", actual_finish]
if actual_window_step["state"] == "skipped":
    date_step = step(7, "update_openproject_dates", "skipped", "the actual window is unknown", date_command)
elif token_missing:
    date_step = step(7, "update_openproject_dates", "skipped", f"{settings.token_env} is not set", date_command)
else:
    # Compare with OpenProject even when RePlan already holds the window: a
    # rerun after a failed date push must still push it.
    date_preview = preflight_json(date_command, env=toolkit_dry_env)
    date_actions = date_preview.get("actions") or []
    if date_actions and date_actions[0].get("from_start_date") == actual_start and date_actions[0].get("from_due_date") == actual_finish:
        date_step = step(7, "update_openproject_dates", "already_done", "OpenProject dates already match the derived actual window", date_command, cli_dry_run=date_preview)
    else:
        date_step = action_for(date_command, 7, "update_openproject_dates", "OpenProject dates differ from the derived actual window", cli_dry_run=date_preview)
planned.append(date_step)
planned.append(proposal_step)

op_step = None
if token_missing and not settings.from_file:
    op_step = step(9, "update_openproject_status", "skipped", f"{settings.token_env} is not set", [toolkit, "work-package", "update-status", "--task", task_id, "--status", "accepted"])
else:
    inspect_command = [toolkit, "work-package", "inspect", "--task", task_id]
    inspect = preflight_json(inspect_command, env=toolkit_dry_env)
    rows = inspect.get("items") or []
    if len(rows) != 1:
        refuse("openproject_work_package_count", f"task {task_id} must resolve to exactly one OpenProject work package; found {len(rows)}", 2, kind="task", target_id=task_id)
    wp = rows[0]
    update_command = [toolkit, "work-package", "update-status", "--task", task_id, "--status", "accepted"]
    if wp.get("mapped_status") == "accepted":
        op_step = step(9, "update_openproject_status", "already_done", f"work package {wp.get('work_package_id', '')} already has accepted status", update_command, work_package_id=wp.get("work_package_id", ""), inspect=inspect)
    else:
        update_preview = preflight_json(update_command, env=toolkit_dry_env)
        op_step = action_for(update_command, 9, "update_openproject_status", f"work package status is {wp.get('source_status', '')!r}", work_package_id=wp.get("work_package_id", ""), cli_dry_run=update_preview)
planned.append(op_step)

overlay_commands = [[overlay_build], [overlay_publish]]
overlay_warnings = []
overlay_target = os.environ.get("REPLAN_OVERLAY_PUBLISH_TARGET", "").strip()
overlay_output_dir = os.environ.get("REPLAN_OVERLAY_OUTPUT_DIR", "").strip() or os.path.join(args.root_dir, "tmp", "openproject-live-overlay", "latest")
overlay_marker_path = os.path.join(overlay_output_dir, ".accept-task-publish.json")
overlay_marker_expected = {
    "task_id": task_id,
    "evidence_id": evidence_id,
    "accept_at": accept_at,
    "openproject_project_id": settings.project_id,
    "publish_target_sha256": hashlib.sha256(overlay_target.encode()).hexdigest() if overlay_target else "",
}
try:
    with open(overlay_marker_path, encoding="utf-8") as marker_stream:
        overlay_marker_done = json.load(marker_stream) == overlay_marker_expected
except (FileNotFoundError, OSError, ValueError):
    overlay_marker_done = False
if not (os.path.isfile(overlay_build) and os.path.isfile(overlay_publish)):
    overlay_step = step(10, "refresh_overlay", "skipped", "overlay scripts are not present next to accept-task", commands=overlay_commands)
    overlay_warnings.append(warning("accept_overlay_scripts_unavailable", "overlay scripts are unavailable in this partial helper copy; overlay refresh was skipped"))
elif not overlay_target:
    overlay_step = step(10, "refresh_overlay", "skipped", "REPLAN_OVERLAY_PUBLISH_TARGET is not set", commands=overlay_commands)
    overlay_warnings.append(warning("accept_overlay_publish_target_not_set", "REPLAN_OVERLAY_PUBLISH_TARGET is not set; overlay refresh was skipped", "env", "REPLAN_OVERLAY_PUBLISH_TARGET"))
elif overlay_marker_done:
    overlay_step = step(10, "refresh_overlay", "already_done", "the local publish marker records this acceptance at this hashed target", commands=overlay_commands)
else:
    overlay_step = step(10, "refresh_overlay", "would_run", "overlay scripts and publish target are available", commands=overlay_commands)
planned.append(overlay_step)

warnings = list(overlay_warnings)
if actual_window_warning:
    warnings.append(actual_window_warning)
if token_missing and not settings.from_file:
    warnings.append(warning("token_env_not_set", f"{settings.token_env} is not set; the OpenProject date and status steps were skipped", "env", settings.token_env))

if mode == "dry_run":
    next_steps = ["Review the ordered actions and their cli_dry_run answers.", "Set REPLAN_ACTOR and rerun with REPLAN_APPLY=1 to apply the unfinished steps."]
    if proposal_ids:
        next_steps.append("After apply, review each new proposal and accept an approved one with replan proposal accept <proposal-id> --apply.")
    helper_envelope.emit({
        "ok": True,
        "command": COMMAND,
        "mode": mode,
        "project_path": project,
        "task_id": task_id,
        "evidence_id": evidence_id,
        "accept_at": accept_at,
        "accepted_by": accepted_by,
        "recorder": recorder,
        "ci_minutes": ci_minutes,
        "actions": planned,
        "new_proposal_ids": proposal_ids,
        "counts": {"planned": len([item for item in planned if item["state"] == "would_run"]), "already_done": len([item for item in planned if item["state"] == "already_done"]), "skipped": len([item for item in planned if item["state"] == "skipped"]), "applied": 0},
        "operator_next_steps": next_steps,
        "warnings": warnings,
    }, 0, mode)


def apply_one(item, command, env=None):
    partial_apply.attempting({key: value for key, value in item.items() if key != "cli_dry_run"})
    partial_apply.writing()
    answer = run_json(command, env=env)
    item["state"] = "applied"
    item["apply_result"] = answer
    partial_apply.applied({"step": item["step"], "name": item["name"], **({"demand_id": item["demand_id"]} if item.get("demand_id") else {})})
    return answer


try:
    for item in planned:
        if item["state"] != "would_run" or item["step"] > 5:
            continue
        apply_one(item, item["command"])

    if actual_window_step["state"] == "would_run":
        apply_one(actual_window_step, actual_window_command)

    if date_step["state"] == "would_run":
        toolkit_apply_env = dict(os.environ)
        toolkit_apply_env["REPLAN_APPLY"] = "1"
        apply_one(date_step, date_step["command"], env=toolkit_apply_env)

    new_proposal_ids = []
    if proposal_step["state"] == "would_run":
        answer = apply_one(proposal_step, proposal_command)
        new_proposal_ids = [item.get("id") for item in (answer.get("result") or {}).get("proposals") or [] if item.get("id")]

    if op_step["state"] == "would_run":
        toolkit_apply_env = dict(os.environ)
        toolkit_apply_env["REPLAN_APPLY"] = "1"
        apply_one(op_step, op_step["command"], env=toolkit_apply_env)

    if overlay_step["state"] == "would_run":
        partial_apply.attempting({"step": 10, "name": "build_overlay"})
        partial_apply.writing()
        build_answer = run_json([overlay_build])
        partial_apply.applied({"step": 10, "name": "build_overlay"})
        overlay_apply_env = dict(os.environ)
        overlay_apply_env["REPLAN_APPLY"] = "1"
        partial_apply.attempting({"step": 10, "name": "publish_overlay"})
        partial_apply.writing()
        publish_answer = run_json([overlay_publish], env=overlay_apply_env)
        partial_apply.applied({"step": 10, "name": "publish_overlay"})
        partial_apply.attempting({"step": 10, "name": "record_overlay_publish"})
        partial_apply.writing()
        os.makedirs(overlay_output_dir, exist_ok=True)
        marker_tmp = overlay_marker_path + ".tmp"
        with open(marker_tmp, "w", encoding="utf-8") as marker_stream:
            json.dump(overlay_marker_expected, marker_stream, sort_keys=True)
            marker_stream.write("\n")
        os.replace(marker_tmp, overlay_marker_path)
        partial_apply.applied({"step": 10, "name": "record_overlay_publish"})
        overlay_step["state"] = "applied"
        overlay_step["apply_results"] = [build_answer, publish_answer]

except StepFailure as exc:
    error = diagnostic(exc.code, str(exc))
    extra = {}
    if partial_apply.applied_so_far:
        extra["applied_so_far"] = partial_apply.applied_so_far
    if partial_apply.failed_step:
        extra["failed_step"] = partial_apply.failed_step
    out = {
        "ok": False,
        "command": COMMAND,
        "mode": mode,
        "project_path": project,
        "task_id": task_id,
        "evidence_id": evidence_id,
        "actions": planned,
        "errors": [error],
        "operator_next_steps": [
            "Inspect applied_so_far and failed_step.",
            "Run accept-task again; it recomputes done-state from RePlan and OpenProject and resumes at unfinished work.",
        ],
        **extra,
    }
    helper_envelope.emit(out, helper_envelope.EXIT["partial"] if partial_apply.applied_so_far else exc.exit_code, mode)

next_steps = [f"Review proposal {proposal_id} and, if approved, run replan proposal accept --project {project} {proposal_id} --actor {recorder} --apply --format json." for proposal_id in new_proposal_ids]
no_op = not any(item.get("state") == "applied" for item in planned)
helper_envelope.emit({
    "ok": True,
    "command": COMMAND,
    "mode": mode,
    "project_path": project,
    "task_id": task_id,
    "evidence_id": evidence_id,
    "accept_at": accept_at,
    "accepted_by": accepted_by,
    "recorder": recorder,
    "ci_minutes": ci_minutes,
    "no_op": no_op,
    "actions": planned,
    "new_proposal_ids": new_proposal_ids,
    "counts": {"planned": 0, "already_done": len([item for item in planned if item["state"] == "already_done"]), "skipped": len([item for item in planned if item["state"] == "skipped"]), "applied": len([item for item in planned if item["state"] == "applied"])},
    "operator_next_steps": next_steps,
    "warnings": warnings,
}, 0, mode)
