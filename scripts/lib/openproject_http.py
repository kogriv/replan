import base64
from dataclasses import dataclass
import json
import os
import socket
import time
import urllib.error
import urllib.parse
import urllib.request


TRANSIENT_HTTP_STATUSES = {408, 429, 500, 502, 503, 504}

PERIMETER_KEY_ENV = "REPLAN_OPENPROJECT_PERIMETER_KEY"
PERIMETER_KEY_FILE_ENV = "REPLAN_OPENPROJECT_PERIMETER_KEY_FILE"
TOKEN_FILE_ENV = "REPLAN_OPENPROJECT_TOKEN_FILE"
PERIMETER_HEADER_ENV = "REPLAN_OPENPROJECT_PERIMETER_HEADER"
TIMEOUT_ENV = "REPLAN_OPENPROJECT_TIMEOUT_SECONDS"
RETRIES_ENV = "REPLAN_OPENPROJECT_RETRIES"
RETRY_DELAY_ENV = "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS"
DEFAULT_PERIMETER_HEADER = "X-Perimeter-Key"


class ConflictingSettingsError(ValueError):
    """Two names for one setting were supplied with different values."""


class MissingSettingError(ValueError):
    """A setting named a secret file that could not be read."""

    def __init__(self, setting):
        self.setting = setting
        super().__init__(f"required setting could not be read: {setting}")


def _secret_setting(environ, value_name, file_name):
    """Resolve a secret without ever including its value or file path in errors."""
    value = environ.get(value_name, "")
    if value:
        return value
    path = environ.get(file_name, "")
    if not path:
        return ""
    try:
        with open(path, encoding="utf-8") as stream:
            return stream.read().strip()
    except (OSError, UnicodeError) as exc:
        raise MissingSettingError(file_name) from exc


def _renamed_setting(environ, new_name, old_name):
    new_value = environ.get(new_name, "")
    old_value = environ.get(old_name, "")
    if new_value and old_value and new_value != old_value:
        raise ConflictingSettingsError(
            f"{new_name} and deprecated {old_name} must not have different values"
        )
    warnings = ()
    if old_value:
        warnings = ({
            "code": "deprecated_setting",
            "message": f"{old_name} is deprecated; use {new_name}",
            "severity": "warning",
            "target": {"kind": "env", "id": old_name},
            "replacement": new_name,
        },)
    return new_value or old_value, warnings


@dataclass(frozen=True)
class Settings:
    """Shared OpenProject settings read from the process environment.

    ``token_env`` may override REPLAN_OPENPROJECT_TOKEN_ENV; both default to
    OPENPROJECT_TOKEN. REPLAN_APPLY, REPLAN_ACTOR, REPLAN_KEEP_TEMP,
    REPLAN_OPENPROJECT_PROFILE, REPLAN_OPENPROJECT_FROM_FILE and
    REPLAN_OPENPROJECT_STATUS_SCOPE are process-environment-only settings and
    are deliberately absent from the dotenv allowlist in settings.list.
    """

    base_url: str = ""
    token_env: str = "OPENPROJECT_TOKEN"
    token: str = ""
    project_id: str = ""
    host_header: str = ""
    forwarded_proto: str = ""
    status_scope: str = "ready_only"
    assignee_user_id: str = ""
    assignee_name: str = ""
    target_project_id: str = ""
    target_project_name: str = ""
    profile: str = "openproject.demo"
    from_file: str = ""
    bindings_path: str = ""
    apply: str = ""
    keep_temp: str = ""
    project_path: str = ""
    perimeter_key: str = ""
    perimeter_header: str = DEFAULT_PERIMETER_HEADER
    timeout_seconds: float = 60.0
    retries: int = 0
    retry_delay_seconds: float = 2.0
    warnings: tuple = ()

    @classmethod
    def from_env(
        cls, token_env=None, *, require_token=False, environ=None,
        root_dir=None,
    ):
        environ = os.environ if environ is None else environ
        token_env = token_env or environ.get("REPLAN_OPENPROJECT_TOKEN_ENV", "") or "OPENPROJECT_TOKEN"
        token = _secret_setting(environ, token_env, TOKEN_FILE_ENV)
        if require_token and not token:
            raise MissingSettingError(token_env)
        project_id, id_warnings = _renamed_setting(
            environ,
            "REPLAN_OPENPROJECT_PROJECT_ID",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_ID",
        )
        project_name, name_warnings = _renamed_setting(
            environ,
            "REPLAN_OPENPROJECT_PROJECT_NAME",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_NAME",
        )
        project_name = project_name or project_id
        bindings_path = environ.get("REPLAN_OPENPROJECT_BINDINGS_PATH", "")
        if not bindings_path and root_dir is not None:
            bindings_path = os.path.join(
                root_dir, "private", "openproject", f"{project_id}-bindings.yaml"
            )
        return cls(
            base_url=environ.get("REPLAN_OPENPROJECT_BASE_URL", ""),
            token_env=token_env,
            token=token,
            project_id=project_id,
            host_header=environ.get("REPLAN_OPENPROJECT_HOST_HEADER", ""),
            forwarded_proto=environ.get("REPLAN_OPENPROJECT_FORWARDED_PROTO", ""),
            status_scope=environ.get("REPLAN_OPENPROJECT_STATUS_SCOPE", "") or "ready_only",
            assignee_user_id=environ.get("REPLAN_OPENPROJECT_ASSIGNEE_USER_ID", ""),
            assignee_name=environ.get("REPLAN_OPENPROJECT_ASSIGNEE_NAME", ""),
            target_project_id=project_id,
            target_project_name=project_name,
            profile=environ.get("REPLAN_OPENPROJECT_PROFILE", "") or "openproject.demo",
            from_file=environ.get("REPLAN_OPENPROJECT_FROM_FILE", ""),
            bindings_path=bindings_path,
            apply=environ.get("REPLAN_APPLY", ""),
            keep_temp=environ.get("REPLAN_KEEP_TEMP", ""),
            project_path=environ.get("REPLAN_PROJECT", ""),
            perimeter_key=_secret_setting(
                environ, PERIMETER_KEY_ENV, PERIMETER_KEY_FILE_ENV
            ),
            perimeter_header=environ.get(PERIMETER_HEADER_ENV, "") or DEFAULT_PERIMETER_HEADER,
            timeout_seconds=_float_setting(environ, TIMEOUT_ENV, 60.0),
            retries=_int_setting(environ, RETRIES_ENV, 0),
            retry_delay_seconds=_float_setting(environ, RETRY_DELAY_ENV, 2.0),
            warnings=id_warnings + name_warnings,
        )

    def request_options(self):
        return {
            "timeout_seconds": self.timeout_seconds,
            "retries": self.retries,
            "retry_delay_seconds": self.retry_delay_seconds,
        }

    def request_metadata(self):
        return self.request_options()


class OpenProjectClient:
    def __init__(
        self,
        base_url,
        settings,
        host_header="",
        forwarded_proto="",
        *,
        default_expected=(200, 201, 204),
        missing_base_url_message=None,
        missing_token_message=None,
    ):
        self.base_url = base_url.rstrip("/")
        self.settings = settings
        self.host_header = host_header
        self.forwarded_proto = forwarded_proto
        self.default_expected = default_expected
        self.missing_base_url_message = missing_base_url_message
        self.missing_token_message = missing_token_message

    def headers(self, content_type="application/json"):
        return openproject_auth_headers(
            self.settings.token,
            self.host_header,
            self.forwarded_proto,
            content_type,
            perimeter_key=self.settings.perimeter_key,
            perimeter_header=self.settings.perimeter_header,
        )

    def request_json(self, method, path, payload=None, expected=None, *, idempotent=False):
        if not self.base_url and self.missing_base_url_message:
            raise RuntimeError(self.missing_base_url_message)
        if not self.settings.token and self.missing_token_message:
            raise RuntimeError(self.missing_token_message)
        data = json.dumps(payload).encode() if payload is not None else None
        req = urllib.request.Request(
            self.base_url + path,
            data=data,
            method=method,
            headers=self.headers(),
        )
        return openproject_request_json(
            req,
            method,
            path,
            self.default_expected if expected is None else expected,
            options=self.settings.request_options(),
            idempotent=idempotent,
        )

    def paginate(self, path, page_size, params=None, *, expected=(200,)):
        base_params = dict(params or {})

        def fetch_page(offset, requested_page_size):
            query = dict(base_params)
            query.update({"pageSize": str(requested_page_size), "offset": str(offset)})
            separator = "&" if "?" in path else "?"
            page_path = path + separator + urllib.parse.urlencode(query)
            _, data = self.request_json("GET", page_path, expected=expected)
            return data

        return openproject_collect_pages(fetch_page, page_size)


def _float_setting(environ, name, default):
    raw = environ.get(name, "")
    if raw == "":
        return default
    try:
        value = float(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be numeric") from exc
    if value <= 0:
        raise ValueError(f"{name} must be positive")
    return value


def _int_setting(environ, name, default):
    raw = environ.get(name, "")
    if raw == "":
        return default
    try:
        value = int(raw)
    except ValueError as exc:
        raise ValueError(f"{name} must be an integer") from exc
    if value < 0:
        raise ValueError(f"{name} must be non-negative")
    return value


def openproject_request_options():
    return Settings.from_env().request_options()


def openproject_request_metadata():
    options = openproject_request_options()
    return {
        "timeout_seconds": options["timeout_seconds"],
        "retries": options["retries"],
        "retry_delay_seconds": options["retry_delay_seconds"],
    }


def openproject_auth_headers(
    token,
    host_header="",
    forwarded_proto="",
    content_type="application/json",
    *,
    perimeter_key=None,
    perimeter_header=None,
):
    """Headers for an OpenProject API v3 request authenticated by API token.

    The single place that builds the Authorization header (GAP-018).
    """
    raw = f"apikey:{token}".encode()
    headers = {
        "Authorization": "Basic " + base64.b64encode(raw).decode(),
        "Accept": "application/json",
    }
    if content_type:
        headers["Content-Type"] = content_type
    if host_header:
        headers["Host"] = host_header
    if forwarded_proto:
        headers["X-Forwarded-Proto"] = forwarded_proto
    headers.update(openproject_perimeter_headers(perimeter_key, perimeter_header))
    return headers


def openproject_perimeter_headers(key=None, header=None):
    """The header a reverse proxy in front of OpenProject may ask of API clients.

    Some installations keep a shared password on every path and let an API
    client through only with a key in a request header, after which the
    client's own Authorization reaches OpenProject. The key is a secret: it
    comes from the environment and is never printed.
    """
    if key is None:
        key = os.environ.get(PERIMETER_KEY_ENV, "")
    if not key:
        return {}
    if header is None:
        header = os.environ.get(PERIMETER_HEADER_ENV, "")
    return {header or DEFAULT_PERIMETER_HEADER: key}


def summarize_error_body(body):
    """Reduce an HTTP error body to what an operator needs.

    Raw bodies are never echoed: they can carry private work-package titles,
    descriptions or field values into operator JSON and logs (GAP-018).
    OpenProject error documents keep their identifier and messages only.
    """
    if not body:
        return {}
    try:
        data = json.loads(body)
    except Exception:
        return {"raw_omitted": True, "raw_length": len(body)}
    if not isinstance(data, dict):
        return {"raw_omitted": True, "raw_length": len(body)}
    summary = {}
    for key in ("errorIdentifier", "message"):
        if data.get(key):
            summary[key] = data[key]
    details = []
    for item in ((data.get("_embedded") or {}).get("errors") or []):
        if isinstance(item, dict):
            details.append({k: item[k] for k in ("errorIdentifier", "message") if item.get(k)})
    if details:
        summary["errors"] = details
    if not summary:
        summary = {"raw_omitted": True, "raw_length": len(body)}
    return summary


def _parse_error_body(exc):
    return summarize_error_body(exc.read().decode(errors="replace"))


def _runtime_error(code, method, path, attempts, timeout_seconds, message, *, status=None, response=None, extra=None):
    payload = {
        "code": code,
        "method": method,
        "path": path,
        "attempts": attempts,
        "timeout_seconds": timeout_seconds,
        "message": message,
    }
    if status is not None:
        payload["status"] = status
    if response is not None:
        payload["response"] = response
    if extra:
        payload.update(extra)
    return RuntimeError(json.dumps(payload, ensure_ascii=False))


def _request_retry_policy(req, method, idempotent):
    if idempotent:
        return True, ""
    method = method.upper()
    if method in {"GET", "HEAD"}:
        return True, ""
    if method == "PATCH":
        try:
            payload = json.loads(req.data) if req.data is not None else None
        except (TypeError, ValueError, UnicodeDecodeError):
            payload = None
        if isinstance(payload, dict) and "lockVersion" in payload:
            return True, ""
        return False, "missing_lock_version"
    return False, "non_idempotent_method"


def _unknown_outcome(attempt, retry_skipped=""):
    outcome = {
        "retried": attempt > 0,
        "outcome_unknown": True,
    }
    if retry_skipped:
        outcome["retry_skipped"] = retry_skipped
    return outcome


def openproject_request_json(req, method, path, expected=(200, 201, 204), *, options=None, idempotent=False):
    options = options or openproject_request_options()
    timeout_seconds = options["timeout_seconds"]
    retries = options["retries"]
    retry_delay_seconds = options["retry_delay_seconds"]
    retry_allowed, retry_skipped = _request_retry_policy(req, method, idempotent)
    attempts = retries + 1 if retry_allowed else 1
    last_attempt = attempts - 1
    method_upper = method.upper()
    outcome_can_be_unknown = method_upper not in {"GET", "HEAD"} and not idempotent
    earlier_attempt_may_have_applied = False

    for attempt in range(attempts):
        try:
            with urllib.request.urlopen(req, timeout=timeout_seconds) as resp:
                body = resp.read()
                if resp.status not in expected:
                    raise _runtime_error(
                        "openproject_unexpected_status",
                        method,
                        path,
                        attempt + 1,
                        timeout_seconds,
                        f"{method} {path} returned HTTP {resp.status}",
                        status=resp.status,
                    )
                if not body:
                    return resp.status, {}
                return resp.status, json.loads(body.decode())
        except urllib.error.HTTPError as exc:
            try:
                parsed = _parse_error_body(exc)
            finally:
                exc.close()
            if exc.code in expected:
                return exc.code, parsed
            if exc.code in TRANSIENT_HTTP_STATUSES and attempt < last_attempt:
                if method_upper == "PATCH" and (exc.code == 408 or 500 <= exc.code <= 599):
                    earlier_attempt_may_have_applied = True
                time.sleep(retry_delay_seconds)
                continue
            if (
                method_upper == "PATCH" and exc.code == 409 and attempt > 0
                and earlier_attempt_may_have_applied
            ):
                extra = {
                    "retried": True,
                    "outcome_unknown": True,
                    "earlier_attempt_may_have_applied": True,
                }
            else:
                extra = (
                    _unknown_outcome(attempt, retry_skipped if not retry_allowed else "")
                    if outcome_can_be_unknown and (exc.code == 408 or 500 <= exc.code <= 599)
                    else None
                )
            raise _runtime_error(
                "openproject_http_error",
                method,
                path,
                attempt + 1,
                timeout_seconds,
                f"{method} {path} returned HTTP {exc.code}",
                status=exc.code,
                response=parsed,
                extra=extra,
            )
        except (TimeoutError, socket.timeout) as exc:
            if attempt < last_attempt:
                if method_upper == "PATCH":
                    earlier_attempt_may_have_applied = True
                time.sleep(retry_delay_seconds)
                continue
            raise _runtime_error(
                "openproject_request_timeout",
                method,
                path,
                attempt + 1,
                timeout_seconds,
                str(exc),
                extra=(
                    _unknown_outcome(attempt, retry_skipped if not retry_allowed else "")
                    if outcome_can_be_unknown else None
                ),
            )
        except urllib.error.URLError as exc:
            reason = getattr(exc, "reason", exc)
            if isinstance(reason, socket.timeout):
                code = "openproject_request_timeout"
            else:
                code = "openproject_network_error"
            if attempt < last_attempt:
                if method_upper == "PATCH":
                    earlier_attempt_may_have_applied = True
                time.sleep(retry_delay_seconds)
                continue
            raise _runtime_error(
                code,
                method,
                path,
                attempt + 1,
                timeout_seconds,
                str(reason),
                extra=(
                    _unknown_outcome(attempt, retry_skipped if not retry_allowed else "")
                    if outcome_can_be_unknown else None
                ),
            )

    raise _runtime_error(
        "openproject_request_failed",
        method,
        path,
        attempts,
        timeout_seconds,
        "OpenProject request failed without a response",
    )


def openproject_collect_pages(fetch_page, page_size):
    """Return every element of a paged OpenProject collection.

    `fetch_page(offset, page_size)` returns one collection document. The API v3
    `offset` is a page number starting at 1, not an element index.
    """
    elements = []
    offset = 1
    while True:
        page = fetch_page(offset, page_size)
        batch = (page.get("_embedded") or {}).get("elements") or []
        elements.extend(batch)
        total = int(page.get("total") or 0)
        if not batch or len(elements) >= total:
            return elements
        offset += 1
