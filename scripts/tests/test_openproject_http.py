import json
import os
import pathlib
import re
import subprocess
import threading
import time
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

from openproject_http import MissingSettingError, OpenProjectClient, Settings


ROOT = pathlib.Path(__file__).resolve().parents[2]


class _Handler(BaseHTTPRequestHandler):
    requests = []
    transient_remaining = 0

    def record(self):
        length = int(self.headers.get("Content-Length", "0"))
        body = self.rfile.read(length)
        type(self).requests.append((self.command, self.path, dict(self.headers), body))
        return body

    def reply(self, status, body=b""):
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        if body:
            try:
                self.wfile.write(body)
            except BrokenPipeError:
                pass

    def transient(self):
        if self.path.startswith("/retry") and type(self).transient_remaining:
            type(self).transient_remaining -= 1
            self.reply(503, b'{"message":"warming up"}')
            return True
        return False

    def do_GET(self):
        self.record()
        if self.path == "/moved":
            self.send_response(308)
            self.send_header("Location", "/item-new")
            self.send_header("Content-Length", "0")
            self.end_headers()
            return
        if self.path == "/missing":
            self.reply(404, b'{"message":"not found"}')
            return
        if self.path == "/timeout" and type(self).transient_remaining:
            type(self).transient_remaining -= 1
            time.sleep(0.1)
        elif self.transient():
            return
        if self.path.startswith("/pages"):
            offset = int(self.path.split("offset=", 1)[1].split("&", 1)[0])
            body = {"total": 3, "_embedded": {"elements": [{"id": offset}]}}
        else:
            body = {"ok": True}
        self.reply(200, json.dumps(body).encode())

    def do_HEAD(self):
        self.record()
        if self.transient():
            return
        self.reply(200)

    def do_PATCH(self):
        body = self.record()
        if self.path == "/patch-timeout":
            time.sleep(0.1)
            self.reply(200, body)
            return
        if self.path == "/patch-timeout-then-conflict":
            attempts = sum(1 for method, path, _, _ in type(self).requests if method == "PATCH" and path == self.path)
            if attempts == 1:
                time.sleep(0.1)
                self.reply(200, body)
            else:
                self.reply(409, b'{"message":"conflict"}')
            return
        if self.path == "/patch-conflict":
            self.reply(409, b'{"message":"conflict"}')
            return
        if self.transient():
            return
        self.reply(200, body)

    def do_POST(self):
        body = self.record()
        if self.path == "/missing":
            self.reply(404, b'{"message":"not found"}')
            return
        if self.transient():
            return
        self.reply(200, body or b'{"ok":true}')

    def do_DELETE(self):
        self.record()
        if self.transient():
            return
        self.reply(204)

    def log_message(self, _format, *_args):
        pass


class OpenProjectClientTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        _Handler.requests = []
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), _Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.base_url = f"http://127.0.0.1:{cls.server.server_port}"

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def settings(self, **overrides):
        values = {
            "token_env": "TEST_TOKEN",
            "token": "secret",
            "perimeter_key": "edge-secret",
            "perimeter_header": "X-Test-Edge",
            "timeout_seconds": 2.0,
            "retries": 0,
            "retry_delay_seconds": 0.001,
        }
        values.update(overrides)
        return Settings(**values)

    def test_request_json_uses_shared_headers_and_payload(self):
        client = OpenProjectClient(
            self.base_url, self.settings(), "openproject.example", "https"
        )
        status, body = client.request_json("PATCH", "/item", {"value": 3}, expected=(200,))
        self.assertEqual((status, body), (200, {"value": 3}))
        method, path, headers, payload = _Handler.requests[-1]
        self.assertEqual((method, path), ("PATCH", "/item"))
        self.assertEqual(json.loads(payload), {"value": 3})
        self.assertEqual(headers["Content-Type"], "application/json")
        self.assertEqual(headers["Host"], "openproject.example")
        self.assertEqual(headers["X-Forwarded-Proto"], "https")
        self.assertEqual(headers["X-Test-Edge"], "edge-secret")
        self.assertTrue(headers["Authorization"].startswith("Basic "))

    def test_get_follows_a_permanent_redirect_with_its_headers(self):
        client = OpenProjectClient(
            self.base_url, self.settings(), "openproject.example", "https"
        )
        status, body = client.request_json("GET", "/moved", expected=(200,))
        self.assertEqual((status, body), (200, {"ok": True}))
        method, path, headers, _ = _Handler.requests[-1]
        self.assertEqual((method, path), ("GET", "/item-new"))
        self.assertTrue(headers["Authorization"].startswith("Basic "))
        self.assertEqual(headers["Host"], "openproject.example")

    def test_request_json_retries_a_transient_status(self):
        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        client = OpenProjectClient(self.base_url, self.settings(retries=1))
        status, body = client.request_json("GET", "/retry", expected=(200,))
        self.assertEqual((status, body), (200, {"ok": True}))
        self.assertEqual(len(_Handler.requests) - before, 2)

    def test_get_retries_a_timeout(self):
        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        client = OpenProjectClient(self.base_url, self.settings(timeout_seconds=0.02, retries=1))
        status, body = client.request_json("GET", "/timeout", expected=(200,))
        self.assertEqual((status, body), (200, {"ok": True}))
        self.assertEqual(len(_Handler.requests) - before, 2)

    def test_head_retries_a_transient_status(self):
        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        client = OpenProjectClient(self.base_url, self.settings(retries=1))
        status, body = client.request_json("HEAD", "/retry-head", expected=(200,))
        self.assertEqual((status, body), (200, {}))
        self.assertEqual(len(_Handler.requests) - before, 2)

    def test_patch_with_lock_version_retries_but_patch_without_does_not(self):
        client = OpenProjectClient(self.base_url, self.settings(retries=1))
        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        status, body = client.request_json("PATCH", "/retry-lock", {"lockVersion": 3}, expected=(200,))
        self.assertEqual((status, body), (200, {"lockVersion": 3}))
        self.assertEqual(len(_Handler.requests) - before, 2)

        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        with self.assertRaises(RuntimeError) as raised:
            client.request_json("PATCH", "/retry-no-lock", {"value": 3}, expected=(200,))
        error = json.loads(str(raised.exception))
        self.assertEqual(len(_Handler.requests) - before, 1)
        self.assertEqual(
            (error["retried"], error["retry_skipped"], error["outcome_unknown"]),
            (False, "missing_lock_version", True),
        )

    def test_retried_patch_conflict_reports_possible_earlier_apply(self):
        client = OpenProjectClient(self.base_url, self.settings(timeout_seconds=0.02, retries=1))
        with self.assertRaises(RuntimeError) as raised:
            client.request_json(
                "PATCH", "/patch-timeout-then-conflict", {"lockVersion": 3}, expected=(200,),
            )
        error = json.loads(str(raised.exception))
        self.assertEqual(
            (error["code"], error["status"], error["attempts"]),
            ("openproject_http_error", 409, 2),
        )
        self.assertEqual(
            (error["retried"], error["outcome_unknown"], error["earlier_attempt_may_have_applied"]),
            (True, True, True),
        )

    def test_patch_timeout_reports_unknown_outcome_with_retry_count(self):
        for retries, retried, attempts in ((0, False, 1), (1, True, 2)):
            with self.subTest(retries=retries):
                client = OpenProjectClient(
                    self.base_url, self.settings(timeout_seconds=0.02, retries=retries),
                )
                with self.assertRaises(RuntimeError) as raised:
                    client.request_json(
                        "PATCH", "/patch-timeout", {"lockVersion": 3}, expected=(200,),
                    )
                error = json.loads(str(raised.exception))
                self.assertEqual(
                    (error["outcome_unknown"], error["retried"], error["attempts"]),
                    (True, retried, attempts),
                )
                self.assertNotIn("retry_skipped", error)

    def test_get_timeout_never_reports_unknown_outcome(self):
        _Handler.transient_remaining = 1
        client = OpenProjectClient(self.base_url, self.settings(timeout_seconds=0.02, retries=0))
        with self.assertRaises(RuntimeError) as raised:
            client.request_json("GET", "/timeout", expected=(200,))
        error = json.loads(str(raised.exception))
        self.assertNotIn("outcome_unknown", error)
        self.assertNotIn("retried", error)
        self.assertNotIn("retry_skipped", error)

    def test_first_attempt_patch_conflict_has_known_outcome(self):
        client = OpenProjectClient(self.base_url, self.settings(retries=1))
        with self.assertRaises(RuntimeError) as raised:
            client.request_json("PATCH", "/patch-conflict", {"lockVersion": 3}, expected=(200,))
        error = json.loads(str(raised.exception))
        self.assertEqual((error["status"], error["attempts"]), (409, 1))
        self.assertNotIn("retried", error)
        self.assertNotIn("outcome_unknown", error)
        self.assertNotIn("earlier_attempt_may_have_applied", error)

    def test_post_is_not_retried_but_explicitly_idempotent_post_is(self):
        client = OpenProjectClient(self.base_url, self.settings(retries=1))
        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        with self.assertRaises(RuntimeError) as raised:
            client.request_json("POST", "/retry-create", {"subject": "A"}, expected=(200,))
        error = json.loads(str(raised.exception))
        self.assertEqual(len(_Handler.requests) - before, 1)
        self.assertEqual(
            (error["retried"], error["retry_skipped"], error["outcome_unknown"]),
            (False, "non_idempotent_method", True),
        )

        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        status, body = client.request_json(
            "POST", "/retry-form", {"subject": "A"}, expected=(200,), idempotent=True,
        )
        self.assertEqual((status, body), (200, {"subject": "A"}))
        self.assertEqual(len(_Handler.requests) - before, 2)

        _Handler.transient_remaining = 2
        with self.assertRaises(RuntimeError) as raised:
            client.request_json(
                "POST", "/retry-form-fail", {"subject": "A"}, expected=(200,), idempotent=True,
            )
        error = json.loads(str(raised.exception))
        self.assertNotIn("outcome_unknown", error)
        self.assertNotIn("retried", error)
        self.assertNotIn("retry_skipped", error)

    def test_delete_is_not_retried_and_post_404_has_known_outcome(self):
        client = OpenProjectClient(self.base_url, self.settings(retries=1))
        _Handler.transient_remaining = 1
        before = len(_Handler.requests)
        with self.assertRaises(RuntimeError) as raised:
            client.request_json("DELETE", "/retry-delete", expected=(204,))
        error = json.loads(str(raised.exception))
        self.assertEqual(len(_Handler.requests) - before, 1)
        self.assertTrue(error["outcome_unknown"])

        with self.assertRaises(RuntimeError) as raised:
            client.request_json("POST", "/missing", {"subject": "A"}, expected=(201,))
        error = json.loads(str(raised.exception))
        self.assertNotIn("outcome_unknown", error)
        self.assertNotIn("retry_skipped", error)

    def test_paginate_collects_three_pages_and_preserves_params(self):
        client = OpenProjectClient(self.base_url, self.settings())
        items = client.paginate("/pages", 1, {"filters": "open"})
        self.assertEqual(items, [{"id": 1}, {"id": 2}, {"id": 3}])
        paths = [request[1] for request in _Handler.requests[-3:]]
        self.assertTrue(all("pageSize=1" in path and "filters=open" in path for path in paths))

    def test_module_specific_missing_setting_and_status_contracts(self):
        empty = self.settings(token="")
        date_client = OpenProjectClient(
            "", empty,
            missing_base_url_message="REPLAN_OPENPROJECT_BASE_URL is required for apply",
            missing_token_message="OPENPROJECT_TOKEN is required for apply",
        )
        with self.assertRaisesRegex(RuntimeError, "^REPLAN_OPENPROJECT_BASE_URL is required for apply$"):
            date_client.request_json("GET", "/ignored")
        toolkit_client = OpenProjectClient(
            self.base_url, empty,
            missing_base_url_message="REPLAN_OPENPROJECT_BASE_URL is required for live API mode",
            missing_token_message="OPENPROJECT_TOKEN is required for live API mode",
        )
        with self.assertRaisesRegex(RuntimeError, "^OPENPROJECT_TOKEN is required for live API mode$"):
            toolkit_client.request_json("GET", "/ignored")
        eager_modules = Settings.from_env("TOKEN", require_token=True, environ={"TOKEN": "x"})
        self.assertEqual(eager_modules.token, "x")
        with self.assertRaises(MissingSettingError):
            Settings.from_env("TOKEN", require_token=True, environ={})
        status_client = OpenProjectClient(
            self.base_url, self.settings(), default_expected=(200, 201, 204, 404)
        )
        status, body = status_client.request_json("GET", "/missing")
        self.assertEqual((status, body), (404, {"message": "not found"}))
        self.assertNotIn("Content-Type", status_client.headers(content_type=""))


class SettingsTest(unittest.TestCase):
    def test_secret_files_and_environment_precedence(self):
        import tempfile
        with tempfile.TemporaryDirectory() as temp_dir:
            token_path = os.path.join(temp_dir, "token")
            key_path = os.path.join(temp_dir, "perimeter")
            with open(token_path, "w", encoding="utf-8") as stream:
                stream.write("  token-from-file  \n")
            with open(key_path, "w", encoding="utf-8") as stream:
                stream.write("\tkey-from-file\n")
            settings = Settings.from_env(environ={
                "REPLAN_OPENPROJECT_TOKEN_FILE": token_path,
                "REPLAN_OPENPROJECT_PERIMETER_KEY_FILE": key_path,
            })
            self.assertEqual(settings.token, "token-from-file")
            self.assertEqual(settings.perimeter_key, "key-from-file")
            settings = Settings.from_env(environ={
                "OPENPROJECT_TOKEN": "token-from-env",
                "REPLAN_OPENPROJECT_TOKEN_FILE": "/does/not/exist",
                "REPLAN_OPENPROJECT_PERIMETER_KEY": "key-from-env",
                "REPLAN_OPENPROJECT_PERIMETER_KEY_FILE": "/also/missing",
            })
            self.assertEqual(settings.token, "token-from-env")
            self.assertEqual(settings.perimeter_key, "key-from-env")

    def test_unreadable_secret_file_names_setting_not_path(self):
        secret_path = "/private/path/that/must/not/be-reported"
        with self.assertRaises(MissingSettingError) as raised:
            Settings.from_env(environ={
                "REPLAN_OPENPROJECT_TOKEN_FILE": secret_path,
            })
        self.assertEqual(raised.exception.setting, "REPLAN_OPENPROJECT_TOKEN_FILE")
        self.assertNotIn(secret_path, str(raised.exception))

    def test_defaults_and_overrides(self):
        defaults = Settings.from_env("TOKEN", environ={})
        self.assertEqual(defaults.token, "")
        self.assertEqual(
            (
                defaults.base_url, defaults.token_env, defaults.project_id,
                defaults.host_header, defaults.forwarded_proto,
                defaults.status_scope, defaults.assignee_user_id,
                defaults.assignee_name, defaults.target_project_id,
                defaults.target_project_name, defaults.profile,
                defaults.from_file, defaults.bindings_path, defaults.apply,
                defaults.keep_temp, defaults.project_path,
            ),
            (
                "", "TOKEN", "", "", "", "ready_only", "", "", "",
                "", "openproject.demo", "", "", "", "", "",
            ),
        )
        self.assertEqual(defaults.perimeter_header, "X-Perimeter-Key")
        self.assertEqual(defaults.request_metadata(), {
            "timeout_seconds": 60.0,
            "retries": 0,
            "retry_delay_seconds": 2.0,
        })
        settings = Settings.from_env("TOKEN", environ={
            "TOKEN": "value",
            "REPLAN_OPENPROJECT_PERIMETER_KEY": "edge",
            "REPLAN_OPENPROJECT_PERIMETER_HEADER": "X-Edge",
            "REPLAN_OPENPROJECT_TIMEOUT_SECONDS": "3.5",
            "REPLAN_OPENPROJECT_RETRIES": "2",
            "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS": "0.25",
            "REPLAN_OPENPROJECT_BASE_URL": "https://op.example",
            "REPLAN_OPENPROJECT_PROJECT_ID": "project",
            "REPLAN_OPENPROJECT_HOST_HEADER": "op.internal",
            "REPLAN_OPENPROJECT_FORWARDED_PROTO": "https",
            "REPLAN_OPENPROJECT_STATUS_SCOPE": "all",
            "REPLAN_OPENPROJECT_ASSIGNEE_USER_ID": "7",
            "REPLAN_OPENPROJECT_ASSIGNEE_NAME": "Operator",
            "REPLAN_OPENPROJECT_PROJECT_NAME": "Project",
            "REPLAN_OPENPROJECT_PROFILE": "custom.profile",
            "REPLAN_OPENPROJECT_FROM_FILE": "fixture.json",
            "REPLAN_OPENPROJECT_BINDINGS_PATH": "bindings.yaml",
            "REPLAN_APPLY": "1",
            "REPLAN_KEEP_TEMP": "1",
            "REPLAN_PROJECT": "project-dir",
        })
        self.assertEqual(settings.token, "value")
        self.assertEqual(settings.perimeter_key, "edge")
        self.assertEqual(settings.perimeter_header, "X-Edge")
        self.assertEqual(settings.base_url, "https://op.example")
        self.assertEqual(settings.project_id, "project")
        self.assertEqual(settings.status_scope, "all")
        self.assertEqual(settings.target_project_id, "project")
        self.assertEqual(settings.target_project_name, "Project")
        self.assertEqual(settings.apply, "1")
        self.assertEqual(settings.project_path, "project-dir")
        self.assertEqual(settings.request_metadata(), {
            "timeout_seconds": 3.5,
            "retries": 2,
            "retry_delay_seconds": 0.25,
        })

    def test_validation_messages(self):
        cases = [
            ({"REPLAN_OPENPROJECT_TIMEOUT_SECONDS": "bad"}, "REPLAN_OPENPROJECT_TIMEOUT_SECONDS must be numeric"),
            ({"REPLAN_OPENPROJECT_RETRIES": "1.5"}, "REPLAN_OPENPROJECT_RETRIES must be an integer"),
            ({"REPLAN_OPENPROJECT_RETRIES": "-1"}, "REPLAN_OPENPROJECT_RETRIES must be non-negative"),
            ({"REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS": "0"}, "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS must be positive"),
        ]
        for environ, message in cases:
            with self.subTest(environ=environ), self.assertRaisesRegex(ValueError, f"^{message}$"):
                Settings.from_env(environ=environ)

    def test_bindings_default_uses_selected_project(self):
        settings = Settings.from_env(
            environ={"REPLAN_OPENPROJECT_PROJECT_ID": "demo"}, root_dir="/repo"
        )
        self.assertEqual(settings.bindings_path, "/repo/private/openproject/demo-bindings.yaml")
        bootstrap = Settings.from_env(
            environ={"REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "target"},
            root_dir="/repo",
        )
        self.assertEqual(bootstrap.bindings_path, "/repo/private/openproject/target-bindings.yaml")

    def test_project_setting_new_name_only(self):
        settings = Settings.from_env(environ={
            "REPLAN_OPENPROJECT_PROJECT_ID": "new-project",
            "REPLAN_OPENPROJECT_PROJECT_NAME": "New project",
        })
        self.assertEqual((settings.project_id, settings.target_project_id), ("new-project", "new-project"))
        self.assertEqual(settings.target_project_name, "New project")
        self.assertEqual(settings.warnings, ())

    def test_project_setting_deprecated_alias_only_warns(self):
        settings = Settings.from_env(environ={
            "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "old-project",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_NAME": "Old project",
        })
        self.assertEqual((settings.project_id, settings.target_project_id), ("old-project", "old-project"))
        self.assertEqual(settings.target_project_name, "Old project")
        self.assertEqual([warning["code"] for warning in settings.warnings], ["deprecated_setting", "deprecated_setting"])
        self.assertIn("REPLAN_OPENPROJECT_PROJECT_ID", settings.warnings[0]["message"])
        self.assertIn("REPLAN_OPENPROJECT_PROJECT_NAME", settings.warnings[1]["message"])

    def test_project_setting_equal_old_and_new_names_warns(self):
        settings = Settings.from_env(environ={
            "REPLAN_OPENPROJECT_PROJECT_ID": "project",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "project",
        })
        self.assertEqual(settings.project_id, "project")
        self.assertEqual(len(settings.warnings), 1)
        self.assertEqual(settings.warnings[0]["code"], "deprecated_setting")

    def test_project_setting_conflicting_names_are_refused(self):
        with self.assertRaisesRegex(ValueError, "REPLAN_OPENPROJECT_PROJECT_ID.*REPLAN_OPENPROJECT_TARGET_PROJECT_ID"):
            Settings.from_env(environ={
                "REPLAN_OPENPROJECT_PROJECT_ID": "new-project",
                "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "old-project",
            })

    def test_project_name_defaults_to_resolved_project_id(self):
        settings = Settings.from_env(environ={"REPLAN_OPENPROJECT_PROJECT_ID": "project"})
        self.assertEqual(settings.target_project_name, "project")


class DotenvSettingsListTest(unittest.TestCase):
    def test_loader_allowlist_precedence_and_quoting(self):
        import tempfile
        with tempfile.TemporaryDirectory() as temp_dir:
            settings_dir = pathlib.Path(temp_dir, "scripts/lib")
            settings_dir.mkdir(parents=True)
            (settings_dir / "settings.list").write_text(
                (ROOT / "scripts/lib/settings.list").read_text()
            )
            pathlib.Path(temp_dir, ".env.local").write_text(
                'REPLAN_OPENPROJECT_BASE_URL="https://quoted.example"\n'
                "OPENPROJECT_TOKEN='from-file'\n"
                "NOT_ON_THE_LIST=ignored\n"
            )
            script = r'''
. "$1/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$2"
printf '%s|%s|%s\n' "$REPLAN_OPENPROJECT_BASE_URL" "$OPENPROJECT_TOKEN" "${NOT_ON_THE_LIST-unset}"
'''
            env = {"PATH": os.environ["PATH"], "OPENPROJECT_TOKEN": "exported"}
            result = subprocess.run(
                ["sh", "-c", script, "sh", str(ROOT), temp_dir],
                check=True, text=True, capture_output=True, env=env,
            )
        self.assertEqual(result.stdout, "https://quoted.example|exported|unset\n")

    def test_agent_kit_example_names_are_all_listed(self):
        allowed = {
            line.strip()
            for line in (ROOT / "scripts/lib/settings.list").read_text().splitlines()
            if line.strip() and not line.lstrip().startswith("#")
        }
        names = set()
        for line in (ROOT / "docs/agent-kit/env.local.example").read_text().splitlines():
            match = re.match(r"^#?([A-Z][A-Z0-9_]*)=", line)
            if match:
                names.add(match.group(1))
        self.assertEqual(names - allowed, set())

    def test_process_only_settings_stay_out_of_dotenv(self):
        import tempfile
        with tempfile.TemporaryDirectory() as temp_dir:
            settings_dir = pathlib.Path(temp_dir, "scripts/lib")
            settings_dir.mkdir(parents=True)
            (settings_dir / "settings.list").write_text(
                (ROOT / "scripts/lib/settings.list").read_text()
            )
            pathlib.Path(temp_dir, ".env.local").write_text(
                "REPLAN_APPLY=1\n"
                "REPLAN_OPENPROJECT_FROM_FILE=private-fixture.json\n"
            )
            script = r'''
. "$1/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$2"
printf '%s|%s\n' "${REPLAN_APPLY-unset}" "${REPLAN_OPENPROJECT_FROM_FILE-unset}"
'''
            result = subprocess.run(
                ["sh", "-c", script, "sh", str(ROOT), temp_dir],
                check=True, text=True, capture_output=True, env={"PATH": os.environ["PATH"]},
            )
        self.assertEqual(result.stdout, "unset|unset\n")

    def test_missing_settings_list_loads_nothing(self):
        import tempfile
        with tempfile.TemporaryDirectory() as temp_dir:
            pathlib.Path(temp_dir, ".env.local").write_text(
                "REPLAN_OPENPROJECT_BASE_URL=https://ignored.example\n"
            )
            script = r'''
. "$1/scripts/lib/load-dotenv.sh"
replan_load_openproject_dotenv "$2"
printf '%s\n' "${REPLAN_OPENPROJECT_BASE_URL-unset}"
'''
            result = subprocess.run(
                ["sh", "-c", script, "sh", str(ROOT), temp_dir],
                check=True, text=True, capture_output=True,
                env={"PATH": os.environ["PATH"]},
            )
        self.assertEqual(result.stdout, "unset\n")


if __name__ == "__main__":
    unittest.main()
