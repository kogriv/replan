"""One JSON envelope for every helper in scripts/ (finding C-08).

Every helper prints one JSON object on stdout with at least:

- ``ok``: true or false;
- ``command``: the helper's name;
- ``mode``: dry_run, apply, read_only, skipped or invalid;
- ``warnings`` and ``errors``: lists of objects with ``code``, ``message``
  and ``severity``, and ``target`` where there is one;
- ``warning_codes`` and ``error_codes``: the sorted codes of those lists.

Anything else a helper reports, counts, actions, items or its own status
key, sits beside them. A failure's exit code comes from the table in
docs/spec/cli.md (EXIT below). A run that ended but reports ``ok: false``,
a drift found or an action blocked or skipped, exits 11, ``attention``, so
that ``set -e`` and ``&&`` stop there (finding C-02); an apply that wrote
some actions and not others exits 7, ``partial``.

Shell wrappers fail through ``replan_fail_json`` in envelope.sh, and answer
a missing token through ``replan_token_missing``, which call:

  python3 -m helper_envelope fail COMMAND MODE CODE MESSAGE EXIT [KEY=VALUE ...]
"""
import argparse
import json
import os
import shlex
import sys
from copy import deepcopy

EXIT = {
    "success": 0,
    "general": 1,
    "validation": 2,
    "policy": 3,
    "not_found": 4,
    "conflict": 5,
    "external": 6,
    "partial": 7,
    "usage": 8,
    "unsupported": 9,
    "internal": 10,
    "attention": 11,
}

MODES = {"dry_run", "apply", "read_only", "skipped", "invalid"}

# Failures of OpenProject itself, from the CLI's import and from
# openproject_http; other `openproject_*` codes are invalid plans or sidecars.
EXTERNAL_CODES = {
    "openproject_network_error",
    "openproject_request_error",
    "openproject_request_timeout",
    "openproject_http_error",
    "openproject_unexpected_status",
    "openproject_api_error",
    "openproject_authentication_failed",
    "openproject_permission_denied",
    "openproject_rate_limited",
    "openproject_malformed_response",
    "openproject_project_not_found",
}


def diagnostic(code, message, severity="error", kind="", target_id=""):
    out = {"code": code, "message": message or code, "severity": severity}
    if kind or target_id:
        out["target"] = {"kind": kind, "id": target_id}
    return out


def _diagnostics(items, severity):
    out = []
    for item in items or []:
        if isinstance(item, dict) and item.get("code"):
            out.append({**item, "message": item.get("message") or item["code"], "severity": item.get("severity") or severity})
        elif isinstance(item, str) and item:
            out.append(diagnostic(item, item, severity))
    return out


def normalize(data, default_mode="read_only"):
    """Return data in the shared envelope. An older shape is converted: a
    bare ``error`` string with its ``message`` becomes an errors entry, and
    codes listed only in ``warning_codes`` or ``error_codes`` become
    entries."""
    out = dict(data)
    errors = _diagnostics(out.get("errors"), "error")
    warnings = _diagnostics(out.get("warnings"), "warning")
    try:
        setting_warnings = json.loads(os.environ.get("REPLAN_SETTING_WARNINGS_JSON", "[]"))
    except ValueError:
        setting_warnings = []
    warnings += _diagnostics(setting_warnings, "warning")
    if isinstance(out.get("error"), str):
        errors.append(diagnostic(out.pop("error"), str(out.pop("message", "") or ""), "error"))
    known = {e["code"] for e in errors}
    errors += [diagnostic(c, c, "error") for c in out.get("error_codes") or [] if isinstance(c, str) and c not in known]
    known = {w["code"] for w in warnings}
    warnings += [diagnostic(c, c, "warning") for c in out.get("warning_codes") or [] if isinstance(c, str) and c not in known]
    out["ok"] = bool(out.get("ok")) and not errors
    out.setdefault("command", "")
    if out.get("mode") not in MODES:
        out["mode"] = default_mode
    out["warnings"] = warnings
    out["errors"] = errors
    out["warning_codes"] = sorted({w["code"] for w in warnings})
    out["error_codes"] = sorted({e["code"] for e in errors})
    return out


def dumps(data, default_mode="read_only", **json_options):
    options = {"indent": 2, "ensure_ascii": False, **json_options}
    return json.dumps(normalize(data, default_mode), **options)


def exit_code_for(out):
    """0 for an ok envelope, 11 for one that needs the operator's attention."""
    return EXIT["success"] if out["ok"] else EXIT["attention"]


def emit(data, exit_code=None, default_mode="read_only", **json_options):
    """Print data in the shared envelope and exit: with exit_code when given,
    otherwise with the code its ok calls for."""
    out = normalize(data, default_mode)
    options = {"indent": 2, "ensure_ascii": False, **json_options}
    print(json.dumps(out, **options))
    raise SystemExit(exit_code_for(out) if exit_code is None else exit_code)


def apply_exit_code(out, applied):
    """7 when an apply wrote some actions but the envelope is not ok,
    otherwise the code its ok calls for."""
    out = normalize(out)
    if out["mode"] == "apply" and applied and not out["ok"]:
        return EXIT["partial"]
    return None


class PartialApply:
    """Keep the durable writes of one apply available to its error handler.

    Call ``attempting`` before the reads that prepare one action, ``writing``
    immediately before its write, and ``applied`` immediately after success.
    Success output remains the helper's responsibility; ``failure`` adds the
    shared partial-apply fields and chooses exit 7 once at least one earlier
    write succeeded.
    """

    def __init__(self):
        self.applied_so_far = []
        self.failed_step = None
        self.write_in_progress = False

    def attempting(self, action):
        self.failed_step = deepcopy(action)
        self.write_in_progress = False

    def writing(self):
        """Mark that the current step's write, rather than its reads, began."""
        self.write_in_progress = True

    def clear(self):
        """Finish an attempted item that required no write."""
        self.failed_step = None
        self.write_in_progress = False

    def applied(self, action):
        self.applied_so_far.append(deepcopy(action))
        self.clear()

    def failure(self, exc, fallback_code):
        entry, code = error_from_exception(exc, fallback_code)
        extra = {}
        if self.applied_so_far:
            extra["applied_so_far"] = deepcopy(self.applied_so_far)
        if self.failed_step is not None:
            failed_step = deepcopy(self.failed_step)
            if entry.get("outcome_unknown") and self.write_in_progress:
                failed_step["may_have_been_applied"] = True
            extra["failed_step"] = failed_step
        if self.applied_so_far:
            code = EXIT["partial"]
        return entry, code, extra


def error_from_exception(exc, fallback_code):
    """The diagnostic and exit code of an exception. OpenProject request
    failures (openproject_http) carry a JSON payload with their own code and
    are external errors; anything else is the helper's own failure."""
    try:
        payload = json.loads(str(exc))
    except ValueError:
        payload = None
    if isinstance(payload, dict) and payload.get("code") in EXTERNAL_CODES:
        entry = diagnostic(payload["code"], payload.get("message", ""))
        for key in (
            "status", "method", "path", "attempts", "response", "retried",
            "retry_skipped", "outcome_unknown", "earlier_attempt_may_have_applied",
        ):
            if key in payload:
                entry[key] = payload[key]
        return entry, EXIT["external"]
    if isinstance(exc, FileNotFoundError):
        return diagnostic(fallback_code, str(exc)), EXIT["not_found"]
    return diagnostic(fallback_code, str(exc)), EXIT["general"]


def guard(
    command, mode="read_only", fallback_code="helper_failed", *,
    unknown_patch_next_step="", partial_apply=None, partial_next_steps=(),
):
    """Turn an exception nothing else caught into the envelope, so a helper
    never answers with a traceback. An OpenProject request failure exits 6,
    anything else 10, the internal error code of docs/spec/cli.md. Call it
    once, before the helper's work; set ``guard.mode`` when the mode is known
    later."""
    guard.mode = mode

    def hook(exc_type, exc, tb):
        if issubclass(exc_type, KeyboardInterrupt):
            sys.__excepthook__(exc_type, exc, tb)
            return
        if partial_apply is not None and (
            partial_apply.applied_so_far or partial_apply.failed_step is not None
        ):
            entry, code, partial_extra = partial_apply.failure(exc, fallback_code)
        else:
            entry, code = error_from_exception(exc, fallback_code)
            partial_extra = {}
        if code == EXIT["general"]:
            code = EXIT["internal"]
            entry["message"] = f"{exc_type.__name__}: {exc}"
        out = {"ok": False, "command": command, "mode": guard.mode, "errors": [entry], **partial_extra}
        if partial_extra and partial_next_steps:
            out["operator_next_steps"] = list(partial_next_steps)
        if unknown_patch_next_step and entry.get("outcome_unknown") and entry.get("method") == "PATCH":
            out.setdefault("operator_next_steps", []).insert(0, unknown_patch_next_step)
        sys.stdout.write(dumps(out) + "\n")
        sys.stdout.flush()
        os._exit(code)

    sys.excepthook = hook


def fail(command, mode, code, message, exit_code, extra=None, **json_options):
    emit({"ok": False, "command": command, "mode": mode, "errors": [diagnostic(code, message)], **(extra or {})},
         exit_code, **json_options)


class EnvelopeArgumentParser(argparse.ArgumentParser):
    """Argument parser for wrapper-to-module options.

    These arguments are an internal contract. A parse failure is therefore a
    helper failure envelope, never argparse's stderr-only usage response.
    """

    def __init__(self, command, *args, **kwargs):
        self.command = command
        super().__init__(*args, **kwargs)

    def error(self, message):
        usage = self.format_usage().strip()
        fail(
            self.command,
            "invalid",
            "usage_error",
            f"{message}\n{usage}",
            EXIT["usage"],
            indent=None,
            separators=(",", ":"),
        )


def internal_parser(command):
    return EnvelopeArgumentParser(command, add_help=False, allow_abbrev=False)


def internal_prefix(command, option, argv=None):
    """Consume one required wrapper-owned option from the front of argv.

    Operator arguments after the prefix are returned byte-for-byte to the
    command's existing parser, including another occurrence of ``option``.
    """
    argv = sys.argv[1:] if argv is None else list(argv)
    if len(argv) < 2 or argv[0] != option:
        fail(
            command,
            "invalid",
            "usage_error",
            f"the first internal option must be {option} VALUE",
            EXIT["usage"],
            indent=None,
            separators=(",", ":"),
        )
    return argv[1], argv[2:]


def _value(text):
    try:
        return json.loads(text)
    except ValueError:
        return text


def main(argv):
    if len(argv) == 2 and argv[1] == "resolve-openproject-settings":
        from openproject_http import ConflictingSettingsError, Settings
        try:
            settings = Settings.from_env()
        except ConflictingSettingsError as exc:
            print(str(exc))
            return EXIT["usage"]
        for name, value in (
            ("REPLAN_OPENPROJECT_PROJECT_ID", settings.project_id),
            ("REPLAN_OPENPROJECT_PROJECT_NAME", settings.target_project_name),
            ("REPLAN_SETTING_WARNINGS_JSON", json.dumps(settings.warnings)),
        ):
            print(f"{name}={shlex.quote(value)}")
            print(f"export {name}")
        return EXIT["success"]
    if len(argv) >= 7 and argv[1] == "fail":
        command, mode, code, message, exit_code = argv[2:7]
        extra = {}
        for pair in argv[7:]:
            key, _, value = pair.partition("=")
            extra[key] = _value(value)
        # Shell wrappers print one compact line, as they did before.
        fail(command, mode, code, message, int(exit_code), extra, indent=None, separators=(",", ":"))
    if len(argv) >= 4 and argv[1] == "skip":
        # skip COMMAND TOKEN_ENV [KEY=VALUE ...]: no token, nothing done.
        command, token_env = argv[2:4]
        extra = {}
        for pair in argv[4:]:
            key, _, value = pair.partition("=")
            extra[key] = _value(value)
        emit({"ok": True, "command": command, "mode": "skipped", "reason": "token_env_not_set", "token_env": token_env,
              "temp_retained": False, **extra,
              "warnings": [diagnostic("token_env_not_set", f"{token_env} is not set; nothing was read or written", "warning")]},
             0, indent=None, separators=(",", ":"))
    if len(argv) == 4 and argv[1] == "normalize":
        # normalize MODE EXIT: read one JSON object on stdin, print it in the
        # shared envelope, and exit with EXIT.
        emit(json.load(sys.stdin), int(argv[3]), argv[2], indent=None, separators=(",", ":"))
    print(__doc__, file=sys.stderr)
    return EXIT["usage"]


if __name__ == "__main__":
    sys.exit(main(sys.argv))
