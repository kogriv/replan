"""Read-only measurement of agent time from explicitly named session logs."""

import argparse
import json
import math
import os
import shlex
import shutil
import subprocess
from collections import defaultdict
from datetime import datetime, timedelta, timezone
from pathlib import Path
from zoneinfo import ZoneInfo, ZoneInfoNotFoundError

import helper_envelope
import yaml


COMMAND = "agent-session-consumption"
DEFAULT_CLAUDE_RESOURCE = "resource.ai.agent.executor"
DEFAULT_CODEX_RESOURCE = "resource.ai.codex.local"


def parser():
    p = helper_envelope.EnvelopeArgumentParser(COMMAND, add_help=False, allow_abbrev=False)
    p.add_argument("--claude", action="append", default=[])
    p.add_argument("--codex", action="append", default=[])
    p.add_argument("--claude-resource", default="auto")
    p.add_argument("--codex-resource", default="auto")
    p.add_argument("--from", dest="window_from")
    p.add_argument("--to", dest="window_to")
    p.add_argument("--project")
    p.add_argument("--task")
    p.add_argument("--idle-gap-minutes", type=positive_number, default=5.0)
    p.add_argument("--max-generation-minutes", type=nonnegative_number, default=15.0)
    p.add_argument("--count-tool-waits", action="store_true")
    p.add_argument("--cwd", action="append", default=[])
    return p


def positive_number(value):
    number = finite_number(value)
    if number <= 0:
        raise argparse.ArgumentTypeError("must be greater than zero")
    return number


def nonnegative_number(value):
    number = finite_number(value)
    if number < 0:
        raise argparse.ArgumentTypeError("must not be negative")
    return number


def finite_number(value):
    try:
        number = float(value)
    except ValueError as exc:
        raise argparse.ArgumentTypeError("must be a number") from exc
    if not math.isfinite(number):
        raise argparse.ArgumentTypeError("must be finite")
    return number


def parse_timestamp(value, option="timestamp"):
    if not isinstance(value, str):
        raise ValueError(f"{option} must be RFC3339")
    normalized = value[:-1] + "+00:00" if value.endswith("Z") else value
    try:
        parsed = datetime.fromisoformat(normalized)
    except ValueError as exc:
        raise ValueError(f"{option} must be RFC3339: {value}") from exc
    if parsed.tzinfo is None:
        raise ValueError(f"{option} must include a UTC offset: {value}")
    return parsed.astimezone(timezone.utc)


def rfc3339(value):
    text = value.astimezone(timezone.utc).isoformat(timespec="milliseconds")
    return text.replace("+00:00", "Z")


def usage_failure(code, message, exit_code=helper_envelope.EXIT["usage"]):
    helper_envelope.fail(COMMAND, "invalid", code, message, exit_code)


def choose_window(args):
    explicit = args.window_from is not None
    task_based = args.project is not None or args.task is not None
    if not explicit and not task_based and args.window_to is None:
        usage_failure("missing_window", "use --from with --to, or --project with --task")
    if explicit and not task_based:
        if not args.window_from or not args.window_to:
            usage_failure("missing_window", "--from and --to must be supplied together")
        try:
            start, end = parse_timestamp(args.window_from, "--from"), parse_timestamp(args.window_to, "--to")
        except ValueError as exc:
            usage_failure("invalid_window", str(exc))
        if end < start:
            usage_failure("invalid_window", "--to must not precede --from")
        return start, end, "explicit", []
    if not args.project or not args.task:
        usage_failure("missing_window", "--project and --task must be supplied together")
    override_start = override_end = None
    try:
        if args.window_from:
            override_start = parse_timestamp(args.window_from, "--from")
        if args.window_to:
            override_end = parse_timestamp(args.window_to, "--to")
    except ValueError as exc:
        usage_failure("invalid_window", str(exc))
    start, end, source, warnings = derive_task_window(args.project, args.task, override_end)
    if override_start is not None:
        # Narrow the task window, e.g. to leave out time spent on another task in parallel.
        start = max(start, override_start)
        if end < start:
            usage_failure("invalid_window", "--from is after the task window end")
        source += "_narrowed"
    return start, end, source, warnings


def load_first_id(project, relative, top_key):
    path = Path(project) / relative
    try:
        doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    except (OSError, yaml.YAMLError):
        return ""
    items = doc.get(top_key) or []
    return next((str(item.get("id")) for item in items if isinstance(item, dict) and item.get("id")), "")


def first_evidence_id(project):
    for path in sorted((Path(project) / "evidence").glob("*.yaml")):
        value = load_first_id(project, str(path.relative_to(project)), "evidence")
        if value:
            return value
    return ""


def project_timezone(project):
    path = Path(project) / ".replan" / "project.yaml"
    try:
        doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
        name = (doc.get("project") or {}).get("timezone") or doc.get("timezone") or "UTC"
        return ZoneInfo(name)
    except (OSError, yaml.YAMLError, ZoneInfoNotFoundError):
        return timezone.utc


def replan_binary():
    configured = os.environ.get("REPLAN_BIN")
    if configured:
        return configured
    root = Path(__file__).resolve().parents[2]
    for candidate in (root / "bin" / "replan", root / "tmp" / "replan"):
        if candidate.is_file() and os.access(candidate, os.X_OK):
            return str(candidate)
    return shutil.which("replan") or ""


def status_records(project):
    """Return {task_id: [(occurred_at, status), ...]} from the audit records, sorted by time."""
    records = defaultdict(list)
    for path in sorted((Path(project) / "audit").glob("*.jsonl")):
        try:
            lines = path.read_text(encoding="utf-8").splitlines()
        except OSError:
            continue
        for line in lines:
            try:
                record = json.loads(line)
            except ValueError:
                continue
            if record.get("operation") != "actual_add_status":
                continue
            task = (record.get("target_ref") or {}).get("id")
            if not task or not record.get("new_status"):
                continue
            try:
                occurred_at = parse_timestamp(record.get("occurred_at"))
            except ValueError:
                continue
            records[task].append((occurred_at, record["new_status"]))
    for items in records.values():
        items.sort(key=lambda item: item[0])
    return records


def window_from_records(records):
    starts = [stamp for stamp, status in records if status == "in_progress"]
    if not starts:
        return None
    start = starts[0]
    after_start = [(stamp, status) for stamp, status in records if stamp > start]
    review_ready = [stamp for stamp, status in after_start if status == "review_ready"]
    finished = [stamp for stamp, status in after_start if status in ("accepted", "done", "closed")]
    end = review_ready[0] if review_ready else (finished[0] if finished else None)
    return start, end


def status_record_window(project, task):
    return window_from_records(status_records(project).get(task, []))


def overlapping_task_windows(project, task, start, end):
    """Other tasks whose status window (in_progress to review_ready or finish, else open) overlaps [start, end]."""
    now = datetime.now(timezone.utc)
    overlaps = []
    for other, records in status_records(project).items():
        if other == task:
            continue
        window = window_from_records(records)
        if window is None:
            continue
        other_start, other_end = window[0], window[1] or now
        seconds = (min(end, other_end) - max(start, other_start)).total_seconds()
        if seconds > 0:
            overlaps.append({"task": other, "from": rfc3339(max(start, other_start)), "to": rfc3339(min(end, other_end)), "seconds": round(seconds, 6)})
    overlaps.sort(key=lambda item: item["from"])
    return overlaps


def derive_task_window(project, task, override_end=None):
    warnings = []
    status_window = status_record_window(project, task)
    if status_window is not None:
        start, recorded_end = status_window
        now = datetime.now(timezone.utc)
        if override_end is not None:
            end = min(override_end, now)
        elif recorded_end is not None:
            end = min(recorded_end, now)
        else:
            end = now
            warnings.append(helper_envelope.diagnostic(
                "window_open", f"task {task} has no review-ready or finished status record; window ends now", "warning", "task", task,
            ))
        if end < start:
            usage_failure("invalid_window", "task window end precedes its first in_progress status")
        return start, end, "status_records", warnings

    binary = replan_binary()
    actor = load_first_id(project, "canonical/actors.yaml", "actors")
    evidence = first_evidence_id(project)
    if not binary or not actor or not evidence:
        helper_envelope.fail(
            COMMAND, "read_only", "window_unknown",
            "task window could not be queried: a replan binary, actor, and evidence record are required",
            helper_envelope.EXIT["attention"],
        )
    command = [
        binary, "task", "record-actual-window", "--project", project, "--task", task,
        "--actor", actor, "--evidence", evidence, "--dry-run", "--format", "json",
    ]
    try:
        proc = subprocess.run(command, text=True, capture_output=True, check=False)
        payload = json.loads(proc.stdout)
    except (OSError, ValueError) as exc:
        helper_envelope.fail(COMMAND, "read_only", "window_unknown", f"task window query failed: {exc}", helper_envelope.EXIT["attention"])
    result = payload.get("result") or {}
    start_date = result.get("new_planned_start")
    finish_date = result.get("new_planned_finish")
    if proc.returncode != 0 or not start_date or not finish_date:
        details = ", ".join(sorted({d.get("code", "") for d in payload.get("errors", []) if d.get("code")}))
        message = f"task {task} actual window is not derivable"
        if details:
            message += f" ({details})"
        helper_envelope.fail(COMMAND, "read_only", "window_unknown", message, helper_envelope.EXIT["attention"])
    try:
        project_zone = project_timezone(project)
        start = datetime.fromisoformat(start_date).replace(tzinfo=project_zone).astimezone(timezone.utc)
        # Date windows include the complete finish date in the project zone.
        end = (datetime.fromisoformat(finish_date).replace(tzinfo=project_zone) + timedelta(days=1, microseconds=-1)).astimezone(timezone.utc)
    except ValueError as exc:
        helper_envelope.fail(COMMAND, "read_only", "window_unknown", str(exc), helper_envelope.EXIT["attention"])
    if override_end is not None:
        end = min(override_end, datetime.now(timezone.utc))
        if end < start:
            usage_failure("invalid_window", "--to precedes the task's derived start")
    warnings.append(helper_envelope.diagnostic(
        "window_from_dates", f"task {task} has no usable timestamped status window; using derived task dates", "warning", "task", task,
    ))
    return start, end, "task_actual_window_dates", warnings


def expand_paths(paths, recursive):
    files = []
    for raw in paths:
        path = Path(raw)
        if not path.exists():
            helper_envelope.fail(COMMAND, "read_only", "session_path_not_found", f"session path not found: {raw}", helper_envelope.EXIT["not_found"])
        if path.is_file():
            files.append(path)
        elif path.is_dir():
            files.extend(path.rglob("*.jsonl") if recursive else path.glob("*.jsonl"))
        else:
            usage_failure("invalid_session_path", f"session path is not a file or directory: {raw}")
    # Resolve for stable de-duplication without exposing any implicit paths.
    return sorted(dict.fromkeys(path.resolve() for path in files), key=str)


def cwd_matches(value, prefixes):
    return not prefixes or (isinstance(value, str) and any(value.startswith(prefix) for prefix in prefixes))


def classify_claude(obj):
    if obj.get("type") != "user":
        return "assistant" if obj.get("type") == "assistant" else "other"
    content = (obj.get("message") or {}).get("content")
    if isinstance(content, str):
        return "human"
    if isinstance(content, list) and any(isinstance(item, dict) and item.get("type") == "tool_result" for item in content):
        return "tool_result"
    return "other"


def empty_skips():
    return {"malformed_json": 0, "no_timestamp": 0, "outside_window": 0, "cwd_filtered": 0, "other_type": 0}


def read_claude(files, prefixes, window_start, window_end):
    groups, skipped, warnings, file_sessions = defaultdict(list), empty_skips(), [], []
    for path in files:
        last_session, sessions_seen = "", []
        with path.open(encoding="utf-8") as stream:
            for line_number, line in enumerate(stream, 1):
                try:
                    obj = json.loads(line)
                except ValueError:
                    skipped["malformed_json"] += 1
                    warnings.append(helper_envelope.diagnostic("malformed_json", f"skipped malformed JSON in {path}:{line_number}", "warning"))
                    continue
                if obj.get("type") not in ("user", "assistant"):
                    skipped["other_type"] += 1
                    continue
                if obj.get("sessionId"):
                    last_session = str(obj["sessionId"])
                    if last_session not in sessions_seen:
                        sessions_seen.append(last_session)
                if not obj.get("timestamp"):
                    skipped["no_timestamp"] += 1
                    continue
                try:
                    stamp = parse_timestamp(obj["timestamp"])
                except ValueError:
                    skipped["malformed_json"] += 1
                    warnings.append(helper_envelope.diagnostic("invalid_timestamp", f"skipped invalid timestamp in {path}:{line_number}", "warning"))
                    continue
                if not cwd_matches(obj.get("cwd"), prefixes):
                    skipped["cwd_filtered"] += 1
                    continue
                if stamp < window_start or stamp > window_end:
                    skipped["outside_window"] += 1
                session = str(obj.get("sessionId") or last_session)
                groups[(str(path),)].append({"time": stamp, "kind": classify_claude(obj), "file": str(path), "session_id": session})
        file_sessions.append({"file": str(path), "session_ids": sessions_seen})
    return groups, skipped, warnings, file_sessions


def read_codex(files, prefixes, window_start, window_end):
    groups, skipped, warnings, file_sessions = defaultdict(list), empty_skips(), [], []
    for path in files:
        rows, meta = [], {}
        with path.open(encoding="utf-8") as stream:
            for line_number, line in enumerate(stream, 1):
                try:
                    obj = json.loads(line)
                except ValueError:
                    skipped["malformed_json"] += 1
                    warnings.append(helper_envelope.diagnostic("malformed_json", f"skipped malformed JSON in {path}:{line_number}", "warning"))
                    continue
                if obj.get("type") == "session_meta" and isinstance(obj.get("payload"), dict):
                    meta.update(obj["payload"])
                if not obj.get("timestamp"):
                    skipped["no_timestamp"] += 1
                    continue
                try:
                    stamp = parse_timestamp(obj["timestamp"])
                except ValueError:
                    skipped["malformed_json"] += 1
                    warnings.append(helper_envelope.diagnostic("invalid_timestamp", f"skipped invalid timestamp in {path}:{line_number}", "warning"))
                    continue
                rows.append((stamp, obj.get("type", "other")))
        if not cwd_matches(meta.get("cwd"), prefixes):
            skipped["cwd_filtered"] += len(rows)
            continue
        session = str(meta.get("id") or path)
        file_sessions.append({"file": str(path), "session_ids": [session]})
        for stamp, kind in rows:
            if stamp < window_start or stamp > window_end:
                skipped["outside_window"] += 1
            groups[(str(path), session)].append({"time": stamp, "kind": kind, "file": str(path), "session_id": session})
    return groups, skipped, warnings, file_sessions


def clipped_interval(start, end, window_start, window_end, event, human_messages=0, category="activity"):
    clipped_start, clipped_end = max(start, window_start), min(end, window_end)
    if clipped_end < clipped_start:
        return None
    return {
        "start": rfc3339(clipped_start), "end": rfc3339(clipped_end),
        "seconds": round((clipped_end - clipped_start).total_seconds(), 6),
        "file": event["file"], "session_id": event["session_id"],
        "human_messages": human_messages, "category": category,
        "_start": clipped_start, "_end": clipped_end,
    }


def build_intervals(groups, window_start, window_end, idle_seconds, source, generation_cap, count_tool_waits):
    intervals, generation_seconds, tool_wait_seconds = [], 0.0, 0.0
    for events in groups.values():
        events.sort(key=lambda item: item["time"])
        if not events:
            continue
        segment_start = events[0]["time"]
        segment_humans = 1 if events[0]["kind"] == "human" else 0
        previous = events[0]
        for event in events[1:]:
            gap = (event["time"] - previous["time"]).total_seconds()
            if gap <= idle_seconds:
                segment_humans += event["kind"] == "human"
                previous = event
                continue
            normal = clipped_interval(segment_start, previous["time"], window_start, window_end, previous, segment_humans)
            if normal is not None:
                intervals.append(normal)
            if source == "claude" and event["kind"] == "assistant":
                active = min(gap, generation_cap)
                special = clipped_interval(event["time"] - timedelta(seconds=active), event["time"], window_start, window_end, event, 0, "generation")
                if special is not None:
                    intervals.append(special)
                    generation_seconds += special["seconds"]
            elif source == "claude" and event["kind"] == "tool_result":
                wait = clipped_interval(previous["time"], event["time"], window_start, window_end, event, 0, "tool_wait")
                if wait is not None:
                    tool_wait_seconds += wait["seconds"]
                    if count_tool_waits:
                        intervals.append(wait)
            segment_start = event["time"]
            segment_humans = 1 if event["kind"] == "human" else 0
            previous = event
        final = clipped_interval(segment_start, previous["time"], window_start, window_end, previous, segment_humans)
        if final is not None:
            intervals.append(final)
    intervals.sort(key=lambda item: (item["_start"], item["_end"], item["file"]))
    return intervals, generation_seconds, tool_wait_seconds


def merged_ranges(intervals):
    ranges = sorted((item["_start"], item["_end"]) for item in intervals if item["_end"] > item["_start"])
    merged = []
    for start, end in ranges:
        if merged and start <= merged[-1][1]:
            merged[-1] = (merged[-1][0], max(end, merged[-1][1]))
        else:
            merged.append((start, end))
    return merged


def overlap_seconds(left, right):
    a, b, i, j, total = merged_ranges(left), merged_ranges(right), 0, 0, 0.0
    while i < len(a) and j < len(b):
        total += max(0.0, (min(a[i][1], b[j][1]) - max(a[i][0], b[j][0])).total_seconds())
        if a[i][1] <= b[j][1]:
            i += 1
        else:
            j += 1
    return total


def union_seconds(intervals):
    return sum((end - start).total_seconds() for start, end in merged_ranges(intervals))


def session_totals(intervals):
    by_session = defaultdict(list)
    for interval in intervals:
        by_session[interval["session_id"]].append(interval)
    return [
        {
            "session_id": session_id,
            "seconds": round(union_seconds(items), 6),
            "hours": round(union_seconds(items) / 3600, 2),
            "files": sorted({item["file"] for item in items}),
        }
        for session_id, items in sorted(by_session.items())
    ]


def load_project_items(project, relative, top_key):
    path = Path(project) / relative
    try:
        doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    except (OSError, yaml.YAMLError):
        return []
    return [item for item in doc.get(top_key) or [] if isinstance(item, dict)]


def task_demands(project, task):
    path = Path(project) / "canonical" / "resource_demands.yaml"
    try:
        doc = yaml.safe_load(path.read_text(encoding="utf-8")) or {}
    except (OSError, yaml.YAMLError):
        return []
    out = []
    for demand in doc.get("resource_demands") or []:
        if not isinstance(demand, dict):
            continue
        if (demand.get("task_ref") or {}).get("id") != task:
            continue
        resource = (demand.get("resource_ref") or {}).get("id")
        if resource:
            out.append({
                "id": demand.get("id", ""), "status": demand.get("status", ""),
                "resource_id": resource, "resource_class": demand.get("resource_class", ""),
            })
    return out


def actor_resource_ids(project, source):
    """Concrete resources linked from ai_agent actors whose identity names the client."""
    matches = set()
    for actor in load_project_items(project, "canonical/actors.yaml", "actors"):
        hint = " ".join(str(actor.get(key, "")) for key in ("id", "display_name")).lower()
        resource = (actor.get("resource_ref") or {}).get("id", "")
        if actor.get("kind") == "ai_agent" and source in hint and resource:
            matches.add(resource)
    return matches


def distinct_resource_candidates(demands):
    """Keep one candidate per concrete resource; command selection handles duplicate demands."""
    return list({demand["resource_id"]: demand for demand in demands}.values())


def resolve_resource(
    project, task, source, configured, demands, measured_hours,
    *, linked_resources=None, excluded_resources=(), fallback_excluded_resources=(),
):
    if configured != "auto":
        return configured, "flag", []
    if not task:
        default = DEFAULT_CLAUDE_RESOURCE if source == "claude" else DEFAULT_CODEX_RESOURCE
        return default, "default", []

    linked = actor_resource_ids(project, source) if linked_resources is None else linked_resources
    candidates = [
        demand for demand in demands
        if demand["resource_id"] in linked and demand["resource_id"] not in excluded_resources
    ]
    if source == "claude" and not candidates:
        candidates = [
            demand for demand in demands
            if demand["resource_class"] == "ai_agent"
            and demand["resource_id"] not in excluded_resources
            and demand["resource_id"] not in fallback_excluded_resources
        ]
    candidates = distinct_resource_candidates(candidates)
    if len(candidates) > 1:
        names = [f"{item['id']} ({item['resource_id']})" for item in candidates]
        message = f"task {task} has multiple {source} resource demands: {', '.join(names)}"
        if measured_hours > 0:
            helper_envelope.fail(
                COMMAND, "read_only", "resource_ambiguous", message,
                helper_envelope.EXIT["validation"],
                extra={"source": source, "resource_candidates": candidates},
            )
        return "", "task_demand", [helper_envelope.diagnostic(
            "resource_ambiguous", message, "warning", "task", task,
        )]
    return (candidates[0]["resource_id"] if candidates else ""), "task_demand", []


def command_for(project, task, demand, resource, hours, occurred_at):
    parts = [
        "replan", "actual", "add-consumption", "--project", project, "--task", task,
        "--demand", demand, "--resource", resource, "--amount", f"{hours:.2f}",
        "--unit", "agent_hours", "--occurred-at", occurred_at, "--actor", "<actor-id>",
        "--evidence", "<evidence-id>", "--dry-run", "--format", "json",
    ]
    return " ".join(shlex.quote(part) if not (part.startswith("<") and part.endswith(">")) else part for part in parts)


def public_intervals(intervals):
    return [{key: value for key, value in item.items() if not key.startswith("_")} for item in intervals]


def main():
    args = parser().parse_args()
    helper_envelope.guard(COMMAND)
    window_start, window_end, window_source, window_warnings = choose_window(args)
    claude_files = expand_paths(args.claude, recursive=False)
    codex_files = expand_paths(args.codex, recursive=True)
    claude_groups, claude_skips, claude_warnings, claude_file_sessions = read_claude(claude_files, args.cwd, window_start, window_end)
    codex_groups, codex_skips, codex_warnings, codex_file_sessions = read_codex(codex_files, args.cwd, window_start, window_end)
    idle_seconds = args.idle_gap_minutes * 60
    claude_intervals, generation_seconds, tool_wait_seconds = build_intervals(
        claude_groups, window_start, window_end, idle_seconds, "claude",
        args.max_generation_minutes * 60, args.count_tool_waits,
    )
    codex_intervals, _, _ = build_intervals(
        codex_groups, window_start, window_end, idle_seconds, "codex", 0, False,
    )
    demands = task_demands(args.project, args.task) if args.task else []
    claude_hours = round(union_seconds(claude_intervals) / 3600, 2)
    codex_hours = round(union_seconds(codex_intervals) / 3600, 2)
    codex_linked_resources = actor_resource_ids(args.project, "codex") if args.task else set()
    claude_resource, claude_resource_source, claude_resource_warnings = resolve_resource(
        args.project, args.task, "claude", args.claude_resource, demands, claude_hours,
        fallback_excluded_resources=codex_linked_resources,
    )
    codex_resource, codex_resource_source, codex_resource_warnings = resolve_resource(
        args.project, args.task, "codex", args.codex_resource, demands, codex_hours,
        linked_resources=codex_linked_resources,
        excluded_resources={claude_resource} if claude_resource else set(),
    )
    resources = []
    for resource, resource_source, source, intervals, skips, files, file_sessions, generation, tool_wait in (
        (claude_resource, claude_resource_source, "claude", claude_intervals, claude_skips, claude_files, claude_file_sessions, generation_seconds, tool_wait_seconds),
        (codex_resource, codex_resource_source, "codex", codex_intervals, codex_skips, codex_files, codex_file_sessions, 0.0, 0.0),
    ):
        totals = session_totals(intervals)
        session_sum = sum(item["seconds"] for item in totals)
        seconds = union_seconds(intervals)
        seen_sessions = {session for item in file_sessions for session in item["session_ids"]}
        resources.append({
            "resource_id": resource, "resource_source": resource_source,
            "source": source, "hours": round(seconds / 3600, 2),
            "seconds": round(seconds, 6), "session_sum_seconds": round(session_sum, 6),
            "parallel_seconds": round(max(0.0, session_sum - seconds), 6),
            "generation_seconds": round(generation, 6),
            "tool_wait_seconds": round(tool_wait, 6), "intervals": public_intervals(intervals),
            "session_totals": totals, "source_files": file_sessions,
            "counts": {"files": len(files), "sessions": len(seen_sessions), **skips},
        })
    warnings = (
        window_warnings + claude_warnings + codex_warnings
        + claude_resource_warnings + codex_resource_warnings
    )
    if args.project:
        project_resources = {
            item.get("id") for item in load_project_items(args.project, "canonical/resources.yaml", "resources")
            if item.get("id")
        }
        for item in resources:
            if item["resource_id"] and item["resource_id"] not in project_resources:
                warnings.append(helper_envelope.diagnostic(
                    "resource_not_in_project",
                    f"{item['source']} resource {item['resource_id']} is not a resource in the project",
                    "warning", "resource", item["resource_id"],
                ))
            if item["resource_source"] == "task_demand" and not item["resource_id"] and item["hours"] > 0:
                warnings.append(helper_envelope.diagnostic(
                    "resource_unresolved",
                    f"task {args.task} has measured {item['source']} hours but no qualifying resource demand",
                    "warning", "task", args.task,
                ))
    suggested = []
    overlapping = []
    if args.task:
        overlapping = overlapping_task_windows(args.project, args.task, window_start, window_end)
        if overlapping:
            warnings.append(helper_envelope.diagnostic(
                "overlapping_task_windows",
                f"the window of {args.task} overlaps the status windows of {len(overlapping)} other task(s): "
                + ", ".join(item["task"] for item in overlapping)
                + "; activity in the overlap is counted here too: narrow the window with --from/--to or pass only this task's session files",
                "warning", "task", args.task,
            ))
        for item in resources:
            if item["hours"] <= 0 or not item["resource_id"]:
                continue
            matching = [demand for demand in demands if demand["resource_id"] == item["resource_id"]]
            if not matching:
                warnings.append(helper_envelope.diagnostic(
                    "no_demand_for_resource", f"task {args.task} has no demand for {item['resource_id']}",
                    "warning", "resource", item["resource_id"],
                ))
                continue
            matching.sort(key=lambda demand: (demand["status"] not in ("active", "partially_consumed"), demand["id"]))
            demand = matching[0]
            if demand["status"] in ("consumed", "released", "cancelled", "superseded"):
                warnings.append(helper_envelope.diagnostic(
                    "demand_closed",
                    f"demand {demand['id']} for {item['resource_id']} has status {demand['status']}; add-consumption accepts closed demands",
                    "warning", "resource_demand", demand["id"],
                ))
            if len(matching) > 1:
                warnings.append(helper_envelope.diagnostic(
                    "multiple_demands_for_resource",
                    f"task {args.task} has multiple demands for {item['resource_id']}; the first id is used",
                    "warning", "resource", item["resource_id"],
                ))
            suggested.append(command_for(args.project, args.task, demand["id"], item["resource_id"], item["hours"], rfc3339(window_end)))
    helper_envelope.emit({
        "ok": True, "command": COMMAND, "mode": "read_only",
        "window": {"from": rfc3339(window_start), "to": rfc3339(window_end), "source": window_source, "overlapping_tasks": overlapping},
        "settings": {"idle_gap_minutes": args.idle_gap_minutes, "max_generation_minutes": args.max_generation_minutes, "count_tool_waits": args.count_tool_waits, "cwd_prefixes": args.cwd},
        "resources": resources,
        "overlap_seconds": round(overlap_seconds(claude_intervals, codex_intervals), 6),
        "suggested_commands": suggested, "warnings": warnings, "errors": [],
    })


if __name__ == "__main__":
    main()
