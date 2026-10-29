"""Shared RePlan tracking-status and OpenProject status mapping."""

import json
import os


_MAP_PATH = os.path.join(os.path.dirname(__file__), "openproject_status_map.json")

with open(_MAP_PATH, encoding="utf-8") as _handle:
    STATUS_PAIRS = json.load(_handle)

TRACKING_TO_OPENPROJECT_STATUS = {
    pair["tracking_status"]: pair["openproject_status"]
    for pair in STATUS_PAIRS
}
OPENPROJECT_TO_TRACKING_STATUS = {
    pair["openproject_status"]: pair["tracking_status"]
    for pair in STATUS_PAIRS
}


def resolve_status_argument(value):
    """Return the exact OpenProject name for an accepted status argument."""
    if value in TRACKING_TO_OPENPROJECT_STATUS:
        return TRACKING_TO_OPENPROJECT_STATUS[value]
    if value in OPENPROJECT_TO_TRACKING_STATUS:
        return value
    valid = [*TRACKING_TO_OPENPROJECT_STATUS, *OPENPROJECT_TO_TRACKING_STATUS]
    raise ValueError(f"invalid status {value!r}; valid values: {', '.join(valid)}")
