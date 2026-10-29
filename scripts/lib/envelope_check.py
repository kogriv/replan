"""Small checks over RePlan JSON command envelopes, used by shell helpers.

Usage:
  python3 -m envelope_check require-ok FILE [--with-warnings]
  python3 -m envelope_check min-applied-changes FILE MIN LABEL
  python3 -m envelope_check import-summary FILE
"""

import json
import sys


def load(path):
    with open(path, encoding="utf-8") as handle:
        return json.load(handle)


def field(data, name):
    return data.get(name) or (data.get("result") or {}).get(name) or []


def codes(items):
    return sorted({item.get("code", "") for item in items if isinstance(item, dict) and item.get("code")})


def errors_of(data):
    """The errors of an envelope, including the older shape of a bare
    `error` code with its `message`."""
    errors = list(field(data, "errors"))
    if isinstance(data.get("error"), str):
        errors.append({"code": data["error"], "message": data.get("message", "")})
    return errors


def require_ok(path, with_warnings=False):
    data = load(path)
    if data.get("ok") is True:
        return 0
    if with_warnings:
        payload = {"errors": errors_of(data), "warnings": field(data, "warnings")}
    else:
        payload = errors_of(data)
    print(json.dumps(payload, indent=2), file=sys.stderr)
    return 1


def min_applied_changes(path, minimum, label):
    data = load(path)
    changes = (data.get("result") or {}).get("applied_changes", [])
    if len(changes) >= minimum:
        return 0
    print(f"{label} applied changes = {len(changes)}, expected at least {minimum}", file=sys.stderr)
    print(json.dumps(field(data, "warnings"), indent=2), file=sys.stderr)
    return 1


def import_summary(path):
    data = load(path)
    result = data.get("result") or {}
    return {
        "ok": data.get("ok") is True,
        "snapshot": (result.get("snapshot") or {}).get("id", ""),
        "task_candidates": len(result.get("task_candidates") or []),
        "resource_candidates": len(result.get("resource_candidates") or []),
        "resource_demand_candidates": len(result.get("resource_demand_candidates") or []),
        "dependency_candidates": len(result.get("dependency_candidates") or []),
        "time_entry_candidates": len(result.get("time_entry_candidates") or []),
        "warning_codes": codes(field(data, "warnings")),
        "error_codes": codes(errors_of(data)),
    }


def main(argv):
    if len(argv) >= 3 and argv[1] == "require-ok":
        return require_ok(argv[2], with_warnings="--with-warnings" in argv[3:])
    if len(argv) == 5 and argv[1] == "min-applied-changes":
        return min_applied_changes(argv[2], int(argv[3]), argv[4])
    if len(argv) == 3 and argv[1] == "import-summary":
        print(json.dumps(import_summary(argv[2]), separators=(",", ":")))
        return 0
    print(__doc__, file=sys.stderr)
    return 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
