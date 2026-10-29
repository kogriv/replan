"""Working-week behavior for OpenProject work-package date writes."""

import http.server
import json
import os
import subprocess
import sys
import tempfile
import threading
import unittest

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class WorkingDaysToolkitTest(unittest.TestCase):
    def setUp(self):
        self.patches = []
        self.posts = []
        self.week_reads = 0
        self.fail_week = False
        self.current_ignore = True
        owner = self

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path == "/api/v3/days/week":
                    owner.week_reads += 1
                    if owner.fail_week:
                        self.reply({"message": "stand-in failure"}, 503)
                    else:
                        self.reply({"_embedded": {"elements": [
                            {"day": day, "name": name, "working": day <= 5}
                            for day, name in enumerate(
                                ("Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"),
                                1,
                            )
                        ]}})
                    return
                if self.path == "/api/v3/work_packages/1":
                    self.reply({
                        "id": 1,
                        "subject": "Task A",
                        "lockVersion": 3,
                        "startDate": "2026-10-19",
                        "dueDate": "2026-10-20",
                        "ignoreNonWorkingDays": owner.current_ignore,
                    })
                    return
                if self.path == "/api/v3/projects/demo":
                    self.reply({"id": 42, "identifier": "demo"})
                    return
                if self.path == "/api/v3/projects/42/types":
                    self.reply({"_embedded": {"elements": [
                        {"name": "Task", "_links": {"self": {"href": "/api/v3/types/1"}}}
                    ]}})
                    return
                if self.path.startswith("/api/v3/projects/42/work_packages?"):
                    self.reply({"total": 0, "pageSize": 100, "offset": 1, "_embedded": {"elements": []}})
                    return
                self.reply({}, 404)

            def do_POST(self):
                length = int(self.headers.get("Content-Length") or 0)
                payload = json.loads(self.rfile.read(length) or b"{}")
                if self.path == "/api/v3/projects/42/work_packages/form":
                    self.reply({"_embedded": {"validationErrors": {}}})
                    return
                if self.path == "/api/v3/work_packages":
                    owner.posts.append(payload)
                    self.reply({"id": 2, "subject": payload["subject"]}, 201)
                    return
                self.reply({}, 404)

            def do_PATCH(self):
                length = int(self.headers.get("Content-Length") or 0)
                payload = json.loads(self.rfile.read(length) or b"{}")
                owner.patches.append(payload)
                response = {
                    "id": 1,
                    "lockVersion": 4,
                    "startDate": payload.get("startDate", "2026-10-19"),
                    "dueDate": payload.get("dueDate", "2026-10-20"),
                    "ignoreNonWorkingDays": payload.get("ignoreNonWorkingDays", owner.current_ignore),
                }
                self.reply(response)

            def log_message(self, *_args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)
        self.base_url = f"http://127.0.0.1:{self.server.server_address[1]}"

    def project(self, tmp):
        canonical = os.path.join(tmp, "canonical")
        os.makedirs(canonical)
        with open(os.path.join(canonical, "tasks.yaml"), "w") as handle:
            yaml.safe_dump({"tasks": [{"id": "task.a", "title": "Task A"}]}, handle)
        bindings = os.path.join(tmp, "bindings.yaml")
        with open(bindings, "w") as handle:
            yaml.safe_dump({"openproject_private_bindings": {
                "tasks": [{
                    "replan_task_id": "task.a",
                    "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "1"},
                }],
                "dependencies": [],
                "resources": [],
            }}, handle)
        return bindings

    def run_update(self, tmp, bindings, start_date, due_date, *, apply=False):
        settings = {
            "REPLAN_PROJECT": tmp,
            "REPLAN_OPENPROJECT_BASE_URL": self.base_url,
            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
            "REPLAN_OPENPROJECT_PROJECT_NAME": "",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_NAME": "",
            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            "REPLAN_OPENPROJECT_FROM_FILE": "",
            "REPLAN_OPENPROJECT_TOKEN_ENV": "REPLAN_TEST_OPENPROJECT_TOKEN",
            "REPLAN_TEST_OPENPROJECT_TOKEN": "token",
            "OPENPROJECT_TOKEN": "",
            "REPLAN_APPLY": "1" if apply else "",
        }
        proc = subprocess.run(
            [
                sys.executable,
                "-m",
                "openproject_work_package_toolkit",
                "--root-dir",
                ROOT,
                "work-package",
                "update-dates",
                "--task",
                "task.a",
                "--start-date",
                start_date,
                "--due-date",
                due_date,
            ],
            capture_output=True,
            text=True,
            env=dict(os.environ, PYTHONPATH=LIB, **settings),
            timeout=30,
        )
        return proc, json.loads(proc.stdout)

    def run_create(self, tmp, bindings, *, apply=False):
        settings = {
            "REPLAN_PROJECT": tmp,
            "REPLAN_OPENPROJECT_BASE_URL": self.base_url,
            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
            "REPLAN_OPENPROJECT_PROJECT_NAME": "",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_NAME": "",
            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            "REPLAN_OPENPROJECT_FROM_FILE": "",
            "REPLAN_OPENPROJECT_TOKEN_ENV": "REPLAN_TEST_OPENPROJECT_TOKEN",
            "REPLAN_TEST_OPENPROJECT_TOKEN": "token",
            "OPENPROJECT_TOKEN": "",
            "REPLAN_APPLY": "1" if apply else "",
        }
        proc = subprocess.run(
            [
                sys.executable,
                "-m",
                "openproject_work_package_toolkit",
                "--root-dir",
                ROOT,
                "work-package",
                "create",
                "--subject",
                "Sunday task",
                "--start-date",
                "2026-10-25",
                "--due-date",
                "2026-10-25",
            ],
            capture_output=True,
            text=True,
            env=dict(os.environ, PYTHONPATH=LIB, **settings),
            timeout=30,
        )
        return proc, json.loads(proc.stdout)

    def test_weekend_dry_run_and_apply_set_the_one_way_flag(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp)
            proc, dry = self.run_update(tmp, bindings, "2026-10-25", "2026-10-25")
            self.assertEqual(proc.returncode, 0, dry)
            self.assertTrue(dry["actions"][0]["set_ignore_non_working_days"])
            self.assertIn("Sunday", dry["actions"][0]["ignore_non_working_days_reason"])
            self.assertEqual(self.patches, [])

            proc, out = self.run_update(tmp, bindings, "2026-10-25", "2026-10-25", apply=True)
            self.assertEqual(proc.returncode, 0, out)
            self.assertIs(self.patches[0]["ignoreNonWorkingDays"], True)
            self.assertEqual(self.week_reads, 2)

    def test_weekday_does_not_send_or_reset_existing_true(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp)
            proc, out = self.run_update(tmp, bindings, "2026-10-26", "2026-10-27", apply=True)

        self.assertEqual(proc.returncode, 0, out)
        self.assertNotIn("ignoreNonWorkingDays", self.patches[0])
        self.assertNotIn("set_ignore_non_working_days", out["actions"][0])

    def test_failed_week_lookup_uses_weekend_fallback_and_warns(self):
        self.fail_week = True
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp)
            proc, out = self.run_update(tmp, bindings, "2026-10-24", "2026-10-25")

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["warning_codes"], ["openproject_working_days_unknown"])
        self.assertTrue(out["actions"][0]["set_ignore_non_working_days"])
        self.assertIn("Saturday", out["actions"][0]["ignore_non_working_days_reason"])
        self.assertIn("Sunday", out["actions"][0]["ignore_non_working_days_reason"])

    def test_create_with_weekend_dates_sets_the_flag(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp)
            proc, dry = self.run_create(tmp, bindings)
            self.assertEqual(proc.returncode, 0, dry)
            self.assertTrue(dry["actions"][0]["set_ignore_non_working_days"])

            proc, out = self.run_create(tmp, bindings, apply=True)
            self.assertEqual(proc.returncode, 0, out)

        self.assertIs(self.posts[0]["ignoreNonWorkingDays"], True)


if __name__ == "__main__":
    unittest.main()
