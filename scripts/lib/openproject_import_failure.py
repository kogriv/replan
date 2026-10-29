"""Failure envelope for OpenProject helpers whose import step failed.

Shared by check-openproject-drift, check-openproject-live-alignment,
accept-openproject-soft-drift and push-openproject-date-drift, which used to
carry four copies of this code inside shell heredocs.

Usage:
  python3 -m openproject_import_failure --command COMMAND \
      --status-key STATUS_KEY --status-value STATUS_VALUE \
      --import-json PATH --tmp-dir PATH --root-dir PATH \
      (--bindings | --profile) \
      [--import-exit IMPORT_EXIT]

It prints the shared helper envelope (helper_envelope.py): the import's own
errors and warnings, or `openproject_import_failed` when it left none, and
exits 2 when the import failed validation, 8 on a usage error, otherwise 6,
an external source error. OpenProject's own failures (helper_envelope.EXTERNAL_CODES) always exit 6,
also from a CLI older than its exit 6 for them. A STATUS_KEY of `mode` is reported as `status`,
because the import step itself only reads.

The binding layer is reported as not loaded, because the failure happened
before the helper got to it. `exists` says whether the file is there, so the
envelope no longer reads as "bindings missing" (2026-09-21 audit finding).
"""

import json
import os
import sys

import helper_envelope
from openproject_http import Settings


def codes(items):
    return sorted({item.get("code", "") for item in items if isinstance(item, dict) and item.get("code")})


def envelope(command, status_key, status_value, import_json, project_path, keep_temp, tmp_dir, bindings_path=None, profile=None):
    try:
        with open(import_json, encoding="utf-8") as handle:
            data = json.load(handle)
    except Exception:
        data = {}
    result = data.get("result") or {}
    warnings = data.get("warnings") or result.get("warnings") or []
    errors = data.get("errors") or result.get("errors") or []
    out = {
        "ok": False,
        "command": command,
        "mode": "read_only",
        "stage": "openproject_import",
        ("status" if status_key == "mode" else status_key): status_value,
        "project_path": project_path,
        "warnings": warnings,
        "errors": errors or [helper_envelope.diagnostic("openproject_import_failed", "replan import openproject failed without diagnostics")],
    }
    if bindings_path is not None:
        out["private_binding_layer"] = {
            "path": bindings_path,
            "loaded": False,
            "exists": os.path.exists(bindings_path),
        }
    if profile is not None:
        out["profile"] = profile
    out["summary"] = {"warning_codes": codes(warnings), "error_codes": codes(errors)}
    out["temp_retained"] = bool(keep_temp)
    if keep_temp:
        out["outputs_dir"] = tmp_dir
    return helper_envelope.normalize(out)


def exit_code(import_exit, errors=()):
    """6 when OpenProject itself failed; otherwise the CLI's validation or
    usage exit, or 6."""
    if any(e.get("code") in helper_envelope.EXTERNAL_CODES for e in errors):
        return helper_envelope.EXIT["external"]
    if import_exit in (helper_envelope.EXIT["validation"], helper_envelope.EXIT["usage"]):
        return import_exit
    return helper_envelope.EXIT["external"]


def main(argv):
    parser = helper_envelope.internal_parser("openproject import failure")
    parser.add_argument("--command", required=True)
    parser.add_argument("--status-key", required=True)
    parser.add_argument("--status-value", required=True)
    parser.add_argument("--import-json", required=True)
    parser.add_argument("--tmp-dir", required=True)
    parser.add_argument("--root-dir", required=True)
    parser.add_argument("--import-exit", type=int)
    kind = parser.add_mutually_exclusive_group(required=True)
    kind.add_argument("--bindings", action="store_true")
    kind.add_argument("--profile", action="store_true")
    args = parser.parse_args(argv[1:])
    settings = Settings.from_env(root_dir=args.root_dir)
    kwargs = {"bindings_path": settings.bindings_path} if args.bindings else {"profile": settings.profile}
    out = envelope(
        args.command, args.status_key, args.status_value, args.import_json,
        settings.project_path, settings.keep_temp, args.tmp_dir, **kwargs,
    )
    print(json.dumps(out, separators=(",", ":")))
    return exit_code(args.import_exit, out["errors"])


if __name__ == "__main__":
    sys.exit(main(sys.argv))
