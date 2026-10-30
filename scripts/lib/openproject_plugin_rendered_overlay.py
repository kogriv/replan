"""Prepare the overlay the plugin's rendered-page checks read.

Usage: python3 -m openproject_plugin_rendered_overlay SOURCE TARGET TITLE PROJECT

Copies the overlay at SOURCE to TARGET with the title of the first open work
package replaced by TITLE, which carries markup every plugin page must escape,
and the same markup as an evidence summary and a resource name booked on it,
so the task page escapes those too. An open one is chosen so that it is also
drawn in the Gantt's default range.

The copy names PROJECT, the check project, as its OpenProject project: the
plugin shows an overlay only to the project it belongs to.

The copy also carries one week still overloaded and one planned over
capacity in the past, so the load profile's checks do not depend on what the
plan happens to hold.

Prints two work-package ids, one per line: that open one, and the first one
with evidence, booked time and a predecessor with a work package of its own,
whose task page shows all three.
"""

import datetime
import json
import sys


def main(argv):
    source, target, title, project = argv
    with open(source, encoding="utf-8") as handle:
        overlay = json.load(handle)
    rows = overlay["work_packages"]
    closed = {"accepted", "done", "cancelled"}
    first = next((row for row in rows if row.get("tracking_status") not in closed), rows[0])
    detailed = next(
        (
            row
            for row in rows
            if row is not first
            and row.get("evidence")
            and row.get("actuals")
            and any(dep.get("direction") == "predecessor" and dep.get("work_package_id") for dep in row.get("dependencies") or [])
        ),
        first,
    )
    overlay["openproject_project_id"] = project
    first["title"] = title
    add_overload_cases(overlay)
    first.setdefault("evidence", []).append({"id": "evidence.rendered-check", "date": overlay.get("status_date", ""), "kind": "review", "summary": title})
    first.setdefault("actuals", []).append(
        {"date": overlay.get("status_date", ""), "resource_id": "resource.rendered-check", "resource_name": title, "amount": 1, "unit": "hours"}
    )
    with open(target, "w", encoding="utf-8") as handle:
        json.dump(overlay, handle)
    print(first["work_package_id"])
    print(detailed["work_package_id"])


def add_overload_cases(overlay):
    """Give the load profile one week of each kind, whatever the plan holds.

    The checks look for a red hump, a week still overloaded, and a grey one,
    a week planned over capacity whose work is done. The self-dogfood plan
    has had both and neither at different times, so the copy gets them: the
    first resource with capacity is planned over it in the week after the
    status date, which is also listed as still overloaded, and in the week
    before, which is not. When the plan holds no work after the status
    date, the copy adds the week after its last one and widens the span to
    it, so the Gantt draws it.
    """
    status = overlay.get("status_date", "")
    for resource in overlay.get("resources") or []:
        weeks = [week for week in resource.get("weeks") or [] if week.get("capacity", 0) > 0]
        if weeks and weeks[-1].get("start_date", "") <= status:
            weeks.append(add_next_week(overlay, resource, weeks[-1]))
        ahead = next((week for week in weeks if week.get("start_date", "") > status), None)
        behind = next((week for week in reversed(weeks) if week.get("finish_date", "") < status), None)
        if not ahead or not behind:
            continue
        ahead["planned"] = ahead["capacity"] * 1.2
        behind["planned"] = behind["capacity"] * 1.5
        overlay["overloaded_weeks"] = [{
            "resource_id": resource["id"], "resource_name": resource.get("name", ""), "unit": resource.get("unit", ""),
            "week": ahead["label"], "capacity": ahead["capacity"], "load": ahead["planned"],
            "overload": ahead["planned"] - ahead["capacity"],
        }]
        return
    raise SystemExit("the overlay has no resource with capacity on both sides of the status date")


def add_next_week(overlay, resource, last):
    """Append to the resource the week after LAST, with its capacity and no load.

    LAST may end early, where the plan ends, so the new week starts seven days
    after LAST starts.
    """
    start = datetime.date.fromisoformat(last["start_date"]) + datetime.timedelta(days=7)
    year, week, _ = start.isocalendar()
    following = {
        "label": f"{year}-W{week:02d}",
        "start_date": start.isoformat(),
        "finish_date": (start + datetime.timedelta(days=6)).isoformat(),
        "capacity": last["capacity"],
        "planned": 0,
    }
    resource["weeks"].append(following)
    span = overlay.get("span")
    if span and span.get("start_date") and span.get("finish_date", "") < following["finish_date"]:
        span["finish_date"] = following["finish_date"]
        span["day_count"] = (datetime.date.fromisoformat(following["finish_date"]) - datetime.date.fromisoformat(span["start_date"])).days + 1
    return following


if __name__ == "__main__":
    main(sys.argv[1:])
