"""Fixture edits the MVP smoke applies to its temporary project copies.

Usage:
  python3 -m smoke_fixture_edits drop-task-binding BINDINGS_IN BINDINGS_OUT TASK_ID
  python3 -m smoke_fixture_edits add-binding-task TASKS_YAML CURRENT_PLAN_YAML
"""

import sys

import yaml


def drop_task_binding(source, target, task_id):
    with open(source, encoding="utf-8") as handle:
        doc = yaml.safe_load(handle)
    root = doc["openproject_private_bindings"]
    root["tasks"] = [item for item in root["tasks"] if item.get("replan_task_id") != task_id]
    with open(target, "w", encoding="utf-8") as handle:
        yaml.safe_dump(doc, handle, sort_keys=False)


def add_binding_task(tasks_path, current_plan_path):
    with open(tasks_path, encoding="utf-8") as handle:
        tasks_doc = yaml.safe_load(handle)
    tasks_doc["tasks"].append({
        "id": "task.self.smoke-binding-helper",
        "title": "Smoke private binding helper",
        "description": "Temporary smoke task used to verify private binding helper writes.",
        "type": "implementation",
        "tracking_status": "not_started",
        "planned_start": "2026-10-19",
        "planned_finish": "2026-10-19",
        "tags": ["smoke"],
    })
    with open(tasks_path, "w", encoding="utf-8") as handle:
        yaml.safe_dump(tasks_doc, handle, sort_keys=False)
    with open(current_plan_path, encoding="utf-8") as handle:
        plan_doc = yaml.safe_load(handle)
    plan_doc["current_plan"]["included_tasks"].append({"kind": "task", "id": "task.self.smoke-binding-helper"})
    with open(current_plan_path, "w", encoding="utf-8") as handle:
        yaml.safe_dump(plan_doc, handle, sort_keys=False)


def main(argv):
    if len(argv) == 5 and argv[1] == "drop-task-binding":
        drop_task_binding(argv[2], argv[3], argv[4])
        return 0
    if len(argv) == 4 and argv[1] == "add-binding-task":
        add_binding_task(argv[2], argv[3])
        return 0
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
