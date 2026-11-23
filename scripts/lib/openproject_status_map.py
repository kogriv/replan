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
    name: pair["tracking_status"]
    for pair in STATUS_PAIRS
    for name in [pair["openproject_status"], *(pair.get("aliases") or [])]
}

VALID_TRACKING_STATUSES = frozenset(TRACKING_TO_OPENPROJECT_STATUS)


def normalize_openproject_status(value):
    """Match the importer's case/punctuation-insensitive status folding."""
    return "".join(character for character in str(value or "").lower() if character.isalnum())


_NORMALIZED_OPENPROJECT_TO_TRACKING_STATUS = {
    normalize_openproject_status(name): tracking_status
    for name, tracking_status in OPENPROJECT_TO_TRACKING_STATUS.items()
}


def validate_profile_status_map(status_map):
    """Validate and normalize an OpenProject source-profile overlay."""
    out = {}
    for source_name, tracking_status in (status_map or {}).items():
        normalized = normalize_openproject_status(source_name)
        if not normalized or tracking_status not in VALID_TRACKING_STATUSES:
            raise ValueError(
                f"OpenProject status map entry {source_name!r} has invalid "
                f"tracking status {tracking_status!r}"
            )
        out[normalized] = tracking_status
    return out


def map_openproject_status(value, status_map=None):
    """Fold an OpenProject name through the shared map and profile overlay."""
    mapping = dict(_NORMALIZED_OPENPROJECT_TO_TRACKING_STATUS)
    mapping.update(validate_profile_status_map(status_map))
    return mapping.get(normalize_openproject_status(value), "")


def resolve_status_argument(value):
    """Return the exact OpenProject name for an accepted status argument."""
    if value in TRACKING_TO_OPENPROJECT_STATUS:
        return TRACKING_TO_OPENPROJECT_STATUS[value]
    if value in OPENPROJECT_TO_TRACKING_STATUS:
        return value
    valid = [*TRACKING_TO_OPENPROJECT_STATUS, *OPENPROJECT_TO_TRACKING_STATUS]
    raise ValueError(f"invalid status {value!r}; valid values: {', '.join(valid)}")
