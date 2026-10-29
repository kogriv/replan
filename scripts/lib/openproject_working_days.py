"""OpenProject working-week lookup for work-package date writers."""

from datetime import date


FALLBACK_WORKING_BY_DAY = {
    1: True,
    2: True,
    3: True,
    4: True,
    5: True,
    6: False,
    7: False,
}


def load(client):
    """Read the instance working week once, falling back to Monday-Friday."""
    try:
        _, data = client.request_json("GET", "/api/v3/days/week", expected=(200,))
        elements = (data.get("_embedded") or {}).get("elements") or []
        working_by_day = {
            item.get("day"): item.get("working")
            for item in elements
            if isinstance(item, dict)
            and isinstance(item.get("day"), int)
            and isinstance(item.get("working"), bool)
        }
        if set(working_by_day) != set(range(1, 8)):
            raise ValueError("working-week response did not contain seven weekdays")
        return working_by_day, []
    except Exception:
        warning = {
            "code": "openproject_working_days_unknown",
            "message": (
                "OpenProject's working week could not be read; Saturday and Sunday "
                "are treated as non-working days for date writes."
            ),
            "severity": "warning",
            "target": {"kind": "openproject_endpoint", "id": "/api/v3/days/week"},
        }
        return dict(FALLBACK_WORKING_BY_DAY), [warning]


def apply_ignore_non_working_days(payload, action, working_by_day, dates):
    """Set the one-way flag when a sent date uses a non-working weekday."""
    reasons = []
    for label, value in dates:
        if not value:
            continue
        try:
            parsed = date.fromisoformat(value)
        except ValueError:
            continue
        weekday = parsed.isoweekday()
        if working_by_day.get(weekday) is False:
            reasons.append(f"{label} {value} is {parsed.strftime('%A')}, a non-working weekday")
    if not reasons:
        return False
    payload["ignoreNonWorkingDays"] = True
    action["set_ignore_non_working_days"] = True
    action["ignore_non_working_days_reason"] = "; ".join(reasons)
    return True


def manual_schedule_payload(
    lock_version, start_date, due_date, action, working_by_day, *, include_dates=False
):
    """Build the shared PATCH payload used by manual date writers."""
    payload = {
        "lockVersion": lock_version,
        "scheduleManually": True,
    }
    if start_date or include_dates:
        payload["startDate"] = start_date or None
    if due_date or include_dates:
        payload["dueDate"] = due_date or None
    apply_ignore_non_working_days(
        payload,
        action,
        working_by_day,
        (("start date", start_date), ("due date", due_date)),
    )
    return payload
