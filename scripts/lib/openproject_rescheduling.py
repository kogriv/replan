"""Date snapshots and scheduler-change reporting for OpenProject writers."""

from datetime import date
import json
import urllib.parse

import openproject_working_days


DATE_FIELDS = (("startDate", "start_date"), ("dueDate", "due_date"))


def snapshot(work_package):
    return {
        "startDate": work_package.get("startDate", ""),
        "dueDate": work_package.get("dueDate", ""),
        "scheduleManually": work_package.get("scheduleManually"),
        "lockVersion": work_package.get("lockVersion"),
        "work_package": work_package,
    }


def project_work_packages(client, project_id):
    """Read every project work package, including closed work packages."""
    filters = json.dumps(
        [{"status": {"operator": "*", "values": []}}], separators=(",", ":")
    )
    return client.paginate(
        "/api/v3/projects/"
        + urllib.parse.quote(str(project_id), safe="")
        + "/work_packages",
        500,
        {"filters": filters},
    )


def work_package_dates(client, work_package_ids, *, tolerate_failures=False):
    snapshots = {}
    unread = []
    for work_package_id in sorted({str(value) for value in work_package_ids if str(value)}):
        try:
            _, work_package = client.request_json(
                "GET", f"/api/v3/work_packages/{urllib.parse.quote(work_package_id)}",
                expected=(200,),
            )
        except Exception:
            if not tolerate_failures:
                raise
            unread.append(work_package_id)
            continue
        snapshots[work_package_id] = snapshot(work_package)
    return snapshots, unread


def schedule_changed(before, after):
    """Whether a relation changed dates or left a package automatically scheduled."""
    return (
        before.get("scheduleManually") is False
        or before.get("scheduleManually") != after.get("scheduleManually")
        or before.get("startDate", "") != after.get("startDate", "")
        or before.get("dueDate", "") != after.get("dueDate", "")
    )


def restore_manual_schedule(client, work_package_id, before, current, working_by_day):
    """Restore one relation-affected package and return the reported action.

    This is shared by the plan relation writer and the by-id toolkit command so
    OpenProject scheduler side effects are repaired with the same payload.
    """
    action = {
        "action": "restore_manual_schedule",
        "work_package_id": str(work_package_id),
        "before": {
            "scheduleManually": current.get("scheduleManually"),
            "startDate": current.get("startDate", ""),
            "dueDate": current.get("dueDate", ""),
        },
        "after": {
            "scheduleManually": True,
            "startDate": before.get("startDate", ""),
            "dueDate": before.get("dueDate", ""),
        },
    }
    payload = openproject_working_days.manual_schedule_payload(
        current.get("lockVersion"),
        before.get("startDate", ""),
        before.get("dueDate", ""),
        action,
        working_by_day,
        include_dates=True,
    )
    _, updated = client.request_json(
        "PATCH", f"/api/v3/work_packages/{urllib.parse.quote(str(work_package_id))}",
        payload, expected=(200,),
    )
    return {
        **action,
        "after": {
            "scheduleManually": updated.get("scheduleManually"),
            "startDate": updated.get("startDate", ""),
            "dueDate": updated.get("dueDate", ""),
        },
    }


def project_work_package_dates(
    client, project_id, work_package_ids, *, tolerate_failures=False
):
    """Snapshot requested ids with one collection read and per-id omissions."""
    requested = {str(value) for value in work_package_ids if str(value)}
    snapshots = {}
    try:
        work_packages = project_work_packages(client, project_id)
    except Exception:
        # The per-id path preserves the old behavior if the collection itself
        # is unavailable: fail on the first unread id, or report unread ids.
        work_packages = []
    for work_package in work_packages:
        work_package_id = str(work_package.get("id", ""))
        if work_package_id in requested:
            snapshots[work_package_id] = snapshot(work_package)
    fallback, unread = work_package_dates(
        client, requested - set(snapshots), tolerate_failures=tolerate_failures
    )
    snapshots.update(fallback)
    return snapshots, unread


def roots_first(work_package_ids, successors):
    """Return a deterministic predecessor-before-successor graph order."""
    nodes = {str(value) for value in work_package_ids if str(value)}
    incoming = {node: 0 for node in nodes}
    for predecessor in nodes:
        for successor in successors.get(predecessor, ()):
            if successor in nodes:
                incoming[successor] += 1
    ready = sorted(node for node, count in incoming.items() if count == 0)
    ordered = []
    while ready:
        current = ready.pop(0)
        ordered.append(current)
        for successor in sorted(successors.get(current, ())):
            if successor not in incoming:
                continue
            incoming[successor] -= 1
            if incoming[successor] == 0:
                ready.append(successor)
                ready.sort()
    return ordered + sorted(nodes - set(ordered))


def follows_successors(relations):
    successors = {}
    for relation in relations:
        if relation.get("type") != "follows":
            continue
        successor = str(relation.get("from_work_package_id", ""))
        predecessor = str(relation.get("to_work_package_id", ""))
        if successor and predecessor:
            successors.setdefault(predecessor, set()).add(successor)
    return successors


def reachable_work_packages(roots, successors, *, include_roots=True):
    roots = {str(value) for value in roots if str(value)}
    visited = set(roots)
    seen = set(roots) if include_roots else set()
    pending = list(roots)
    while pending:
        current = pending.pop()
        for successor in successors.get(current, ()):
            if successor in visited:
                continue
            visited.add(successor)
            seen.add(successor)
            pending.append(successor)
    return seen


def moved_dates(before, after, task_id_by_wp, *, causes=None, excluded=None):
    moved = []
    excluded = excluded or set()
    causes = causes or {}
    for work_package_id in sorted(set(before) & set(after)):
        for api_field, _ in DATE_FIELDS:
            if (work_package_id, api_field) in excluded:
                continue
            old_value = before[work_package_id].get(api_field, "")
            new_value = after[work_package_id].get(api_field, "")
            if old_value == new_value:
                continue
            item = {
                "task_id": task_id_by_wp.get(work_package_id, ""),
                "work_package_id": work_package_id,
                "field": api_field,
                "before": old_value,
                "after": new_value,
            }
            known_causes = causes.get(work_package_id, [])
            if len(known_causes) == 1:
                item.update(known_causes[0])
            moved.append(item)
    return moved


def likely_date_reason(client, sent_date, cache):
    if not sent_date:
        return "unknown"
    try:
        parsed = date.fromisoformat(sent_date)
    except ValueError:
        return "unknown"
    # OpenProject's working days are configurable, so ask it first; a
    # weekend is only the fallback when the day cannot be read.
    if sent_date not in cache:
        try:
            _, day = client.request_json(
                "GET", f"/api/v3/days/{urllib.parse.quote(sent_date)}", expected=(200,)
            )
            cache[sent_date] = day.get("working")
        except Exception:
            cache[sent_date] = None
    if cache[sent_date] is False:
        return "non_working_day"
    if cache[sent_date] is None and parsed.weekday() >= 5:
        return "non_working_day"
    return "unknown"


def stored_date_differences(client, applied_actions):
    differences = []
    day_cache = {}
    for action in applied_actions:
        for api_field, suffix in DATE_FIELDS:
            sent = action.get(f"to_openproject_{suffix}", "")
            stored = action.get(f"updated_openproject_{suffix}", "")
            if sent == stored:
                continue
            differences.append({
                "task_id": action.get("task_id", ""),
                "work_package_id": action.get("work_package_id", ""),
                "field": api_field,
                "sent": sent,
                "stored": stored,
                "likely_reason": likely_date_reason(client, sent, day_cache),
            })
    return differences
