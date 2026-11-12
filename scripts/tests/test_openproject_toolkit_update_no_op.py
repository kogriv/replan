import http.server
import json
import os
import subprocess
import sys
import tempfile
import threading
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class ToolkitUpdateNoOpTest(unittest.TestCase):
    def state(self):
        return {
            "work_package": {
                "id": 1,
                "subject": "One",
                "lockVersion": 3,
                "startDate": "2026-11-16",
                "dueDate": "2026-11-17",
                "scheduleManually": True,
                "ignoreNonWorkingDays": False,
                "estimatedTime": "PT1H30M",
                "_links": {
                    "status": {"href": "/api/v3/statuses/3", "title": "In progress"},
                    "priority": {"href": "/api/v3/priorities/1", "title": "Normal"},
                    "version": {"href": "/api/v3/versions/11", "title": "Stage"},
                },
            },
            "patches": [],
            "forms": 0,
            "allow_status": False,
        }

    def serve(self, state):
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def body(self):
                length = int(self.headers.get("Content-Length") or 0)
                return json.loads(self.rfile.read(length) or b"{}")

            def do_GET(self):
                if self.path == "/api/v3/work_packages/1":
                    self.reply(state["work_package"])
                    return
                if self.path == "/api/v3/statuses":
                    self.reply({"_embedded": {"elements": [{
                        "name": "In progress",
                        "_links": {"self": {"href": "/api/v3/statuses/3"}},
                    }]}})
                    return
                if self.path == "/api/v3/days/week":
                    self.reply({"_embedded": {"elements": [
                        {"day": day, "working": day <= 5}
                        for day in range(1, 8)
                    ]}})
                    return
                if self.path == "/api/v3/projects/demo":
                    self.reply({"id": 9, "identifier": "demo"})
                    return
                if self.path.startswith("/api/v3/projects/9/versions"):
                    self.reply({"total": 1, "pageSize": 100, "offset": 1, "_embedded": {"elements": [{
                        "id": 11, "name": "Stage",
                        "_links": {"self": {"href": "/api/v3/versions/11", "title": "Stage"}},
                    }]}})
                    return
                self.reply({}, 404)

            def do_POST(self):
                if self.path == "/api/v3/work_packages/1/form":
                    state["forms"] += 1
                    allowed = [{"href": "/api/v3/statuses/3"}] if state["allow_status"] else []
                    self.reply({"_embedded": {"schema": {"status": {"_links": {"allowedValues": allowed}}}}})
                    return
                self.reply({}, 404)

            def do_PATCH(self):
                payload = self.body()
                state["patches"].append(payload)
                for field, value in payload.items():
                    if field == "_links":
                        for name, link in value.items():
                            if link.get("href") is None:
                                state["work_package"]["_links"].pop(name, None)
                            else:
                                state["work_package"]["_links"][name] = dict(link)
                    elif field != "lockVersion":
                        state["work_package"][field] = value
                state["work_package"]["lockVersion"] += 1
                self.reply(state["work_package"])

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_port}"

    def run_toolkit(self, args, *, state=None, apply=False, fixture=""):
        state = state or self.state()
        base_url = "" if fixture else self.serve(state)
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            REPLAN_PROJECT="",
            REPLAN_OPENPROJECT_BASE_URL=base_url,
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
            TEST_TOKEN="" if fixture else "test-token",
            REPLAN_OPENPROJECT_FROM_FILE=fixture,
            REPLAN_OPENPROJECT_BINDINGS_PATH="",
            REPLAN_APPLY="1" if apply else "",
        )
        proc = subprocess.run(
            [sys.executable, "-m", "openproject_work_package_toolkit", "--root-dir", ROOT, *args],
            capture_output=True, text=True, env=env, timeout=30,
        )
        self.assertEqual(proc.stderr, "")
        return proc, json.loads(proc.stdout), state

    def assert_no_op(self, out, *, apply):
        self.assertTrue(out["ok"], out)
        self.assertTrue(out["no_op"], out)
        self.assertEqual(out["counts"]["planned_actions"], 0)
        if apply:
            self.assertEqual(out["counts"]["applied_actions"], 0)
        self.assertTrue(out["actions"][0]["no_op"])
        self.assertFalse(out["actions"][0]["dry_run"])
        self.assertFalse(out["actions"][0]["apply_required"])

    def test_each_update_is_a_no_op_in_dry_run_and_apply(self):
        cases = (
            (["work-package", "update-status", "--work-package", "1", "--status", "in_progress"], "_links.status"),
            (["work-package", "update-dates", "--work-package", "1", "--start-date", "2026-11-16", "--due-date", "2026-11-17"], "scheduleManually"),
            (["work-package", "update-metadata", "--work-package", "1", "--version", "Stage"], "_links.version"),
        )
        for args, unchanged in cases:
            for apply in (False, True):
                with self.subTest(command=args[2], apply=apply):
                    state = self.state()
                    proc, out, state = self.run_toolkit(args, state=state, apply=apply)
                    self.assertEqual(proc.returncode, 0, out)
                    self.assert_no_op(out, apply=apply)
                    self.assertIn(unchanged, out["actions"][0]["unchanged_fields"])
                    self.assertEqual(state["patches"], [])
                    self.assertEqual(state["forms"], 0)

    def test_same_status_skips_a_workflow_without_self_transition(self):
        state = self.state()
        proc, out, state = self.run_toolkit([
            "work-package", "update-status", "--work-package", "1", "--status", "In progress",
        ], state=state, apply=True)
        self.assertEqual(proc.returncode, 0, out)
        self.assert_no_op(out, apply=True)
        self.assertEqual((state["forms"], state["patches"]), (0, []))

    def test_partial_metadata_update_sends_only_the_differing_field(self):
        state = self.state()
        proc, out, state = self.run_toolkit([
            "work-package", "update-metadata", "--work-package", "1",
            "--estimated-hours", "1.5", "--priority-href", "/api/v3/priorities/2",
        ], state=state, apply=True)
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(state["patches"], [{
            "lockVersion": 3,
            "_links": {"priority": {"href": "/api/v3/priorities/2"}},
        }])
        self.assertEqual(out["actions"][0]["unchanged_fields"], ["estimatedTime"])

    def test_cleared_version_and_equivalent_estimate_are_no_ops(self):
        cases = (
            ["work-package", "update-metadata", "--work-package", "1", "--clear-version"],
            ["work-package", "update-metadata", "--work-package", "1", "--estimated-hours", "1.5"],
        )
        for args in cases:
            with self.subTest(args=args):
                state = self.state()
                if "--clear-version" in args:
                    state["work_package"]["_links"].pop("version")
                proc, out, state = self.run_toolkit(args, state=state, apply=True)
                self.assertEqual(proc.returncode, 0, out)
                self.assert_no_op(out, apply=True)
                self.assertEqual(state["patches"], [])

    def test_equal_dates_but_automatic_schedule_sends_only_manual_flag(self):
        state = self.state()
        state["work_package"]["scheduleManually"] = False
        proc, out, state = self.run_toolkit([
            "work-package", "update-dates", "--work-package", "1",
            "--start-date", "2026-11-16", "--due-date", "2026-11-17",
        ], state=state, apply=True)
        self.assertEqual(proc.returncode, 0, out)
        self.assertFalse(out.get("no_op", False), out)
        self.assertEqual(state["patches"], [{"lockVersion": 3, "scheduleManually": True}])
        self.assertEqual(out["actions"][0]["unchanged_fields"], ["dueDate", "startDate"])

    def test_fixture_dry_run_and_apply_no_ops_are_meaningful(self):
        fixture = {"_embedded": {"elements": [self.state()["work_package"]]}}
        with tempfile.NamedTemporaryFile(mode="w", encoding="utf-8", dir=ROOT) as handle:
            json.dump(fixture, handle)
            handle.flush()
            cases = (
                ["work-package", "update-status", "--work-package", "1", "--status", "in_progress"],
                ["work-package", "update-dates", "--work-package", "1", "--start-date", "2026-11-16", "--due-date", "2026-11-17"],
                ["work-package", "update-metadata", "--work-package", "1", "--version", "Stage"],
            )
            for args in cases:
                for apply in (False, True):
                    with self.subTest(args=args[2], apply=apply):
                        proc, out, _ = self.run_toolkit(args, apply=apply, fixture=handle.name)
                        self.assertEqual(proc.returncode, 0, out)
                        self.assert_no_op(out, apply=apply)


if __name__ == "__main__":
    unittest.main()
