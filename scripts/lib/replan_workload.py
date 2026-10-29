"""Weekly workload rows taken from `replan workload`, the one workload calculation.

The helpers ask the CLI week by week instead of prorating demands themselves,
so concurrent slots count at their daily peak, availability calendars apply,
and closed demands carry no load, exactly as in the Go workload (GAP-031,
finding C-15 of the 2026-10-15 agent tooling review).
"""
import datetime as dt
import json
import os
import subprocess

import yaml


class WorkloadError(RuntimeError):
    """`replan workload` failed; the message carries its error codes."""


def _load_yaml(project_path, rel):
    with open(os.path.join(project_path, rel)) as fh:
        return yaml.safe_load(fh) or {}


def _date(value):
    try:
        return dt.date.fromisoformat(str(value)) if value else None
    except ValueError:
        return None


def iso_week(date):
    year, week, _ = date.isocalendar()
    return f"{year:04d}-W{week:02d}"


def week_start(week):
    return dt.date.fromisocalendar(int(week[:4]), int(week[6:]), 1)


def plan_weeks(project_path):
    """Every ISO week from the earliest to the latest planned date of the
    tasks and demand timings; the CLI decides what falls in each."""
    tasks = _load_yaml(project_path, "canonical/tasks.yaml").get("tasks") or []
    demands = _load_yaml(project_path, "canonical/resource_demands.yaml").get("resource_demands") or []
    dates = [d for t in tasks for d in (_date(t.get("planned_start")), _date(t.get("planned_finish"))) if d]
    for demand in demands:
        timing = demand.get("timing") or {}
        dates += [d for d in (_date(timing.get(k)) for k in ("earliest_start", "preferred_start", "preferred_finish", "latest_finish")) if d]
    if not dates:
        return []
    day, last = week_start(iso_week(min(dates))), max(dates)
    weeks = []
    while day <= last:
        weeks.append(iso_week(day))
        day += dt.timedelta(days=7)
    return weeks


def workload_week(binary, project_path, week, resource_id="", unit=""):
    """The resource rows of one week, each with the demands it is made of."""
    cmd = [binary, "workload", "--project", project_path, "--week", week, "--demands", "--format", "json"]
    if resource_id:
        cmd += ["--resource", resource_id]
    if unit:
        cmd += ["--unit", unit]
    proc = subprocess.run(cmd, capture_output=True, text=True, check=False)
    try:
        data = json.loads(proc.stdout)
    except ValueError as exc:
        raise WorkloadError(f"replan workload --week {week} printed no JSON (exit {proc.returncode})") from exc
    if not data.get("ok"):
        codes = [e.get("code", "") for e in data.get("errors") or []]
        raise WorkloadError(f"replan workload --week {week} failed: {', '.join(codes) or 'exit ' + str(proc.returncode)}")
    return (data.get("result") or {}).get("resources") or []


def planning_load(row):
    """The load the CLI weighs against capacity: reserved, else remaining."""
    return row.get("reserved") or row.get("remaining") or 0.0


def weekly_rows(binary, project_path, weeks, resource_id="", unit=""):
    """Rows with any planned, reserved, remaining or actual amount, week by week."""
    out = []
    for week in weeks:
        for row in workload_week(binary, project_path, week, resource_id, unit):
            if not any(row.get(k) for k in ("planned", "reserved", "remaining", "actual")):
                continue
            out.append({"week": week, "week_start": str(week_start(week)), **row, "planning_load": planning_load(row)})
    return out
