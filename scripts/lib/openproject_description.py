"""Shared OpenProject description conventions for RePlan tasks."""

import re


def task_id_trailer(task_id):
    return f"RePlan task id: `{task_id}`"


def description_with_tail(description, tail):
    return "\n\n".join(part for part in (description or "", tail) if part)


def description_with_task_id(description, task_id):
    return description_with_tail(description, task_id_trailer(task_id))


def split_task_description(description, task_id):
    """Return the RePlan-owned prefix and byte-preserved OpenProject tail."""
    text = "" if description is None else str(description)
    trailer = re.escape(task_id_trailer(task_id))
    match = re.search(rf"(?m)^{trailer}(?=\r?$)", text)
    if match is None:
        return text, ""
    return text[:match.start()], text[match.start():]
