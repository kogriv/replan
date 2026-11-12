import http.server
import json
import os
import subprocess
import sys
import threading
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class ToolkitCommentsTest(unittest.TestCase):
    def run_toolkit(self, base_url, args, *, apply=""):
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            REPLAN_PROJECT="",
            REPLAN_OPENPROJECT_BASE_URL=base_url,
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
            TEST_TOKEN="test-token",
            REPLAN_OPENPROJECT_FROM_FILE="",
            REPLAN_OPENPROJECT_BINDINGS_PATH="",
            REPLAN_APPLY=apply,
        )
        proc = subprocess.run(
            [
                sys.executable, "-m", "openproject_work_package_toolkit",
                "--root-dir", ROOT, *args,
            ],
            capture_output=True, text=True, env=env, timeout=30,
        )
        self.assertEqual(proc.stderr, "")
        return proc, json.loads(proc.stdout)

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
                length = int(self.headers.get("Content-Length", "0"))
                return json.loads(self.rfile.read(length) or b"{}")

            def do_GET(self):
                path = self.path.split("?", 1)[0]
                if path == "/api/v3/work_packages/1234":
                    self.reply({"id": 1234, "subject": "One", "lockVersion": 3})
                    return
                if path == "/api/v3/work_packages/1234/activities":
                    self.reply({
                        "total": len(state["activities"]),
                        "pageSize": 100,
                        "offset": 1,
                        "_embedded": {"elements": state["activities"]},
                    })
                    return
                self.reply({}, 404)

            def do_PATCH(self):
                payload = self.body()
                state["patches"].append((self.path, payload))
                # OpenProject 17.6 returns the work package but ignores comment.
                self.reply({"id": 1234, "subject": "One", "lockVersion": 3})

            def do_POST(self):
                payload = self.body()
                state["posts"].append((self.path, payload))
                if self.path != "/api/v3/work_packages/1234/activities":
                    self.reply({}, 404)
                    return
                raw = payload["comment"]["raw"]
                activity = {
                    "_type": "Activity::Comment",
                    "id": 77,
                    "createdAt": "2026-11-09T12:34:56Z",
                    "comment": {
                        "raw": state.get("returned_comment", raw.replace("\n", "\r\n"))
                    },
                }
                state["activities"].append(activity)
                self.reply(activity, 201)

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_port}"

    @staticmethod
    def state(**values):
        return {"activities": [], "patches": [], "posts": [], **values}

    def test_apply_posts_activity_and_reports_created_activity(self):
        state = self.state()
        base_url = self.serve(state)
        proc, out = self.run_toolkit(base_url, [
            "comment", "add", "--work-package", "1234",
            "--text", "First line\nSecond line   ",
        ], apply="1")
        self.assertEqual(proc.returncode, 0, out)
        self.assertTrue(out["ok"], out)
        self.assertTrue(out["applied"])
        self.assertEqual(out["actions"][0]["activity_id"], "77")
        self.assertEqual(out["items"], [{
            "kind": "activity", "id": "77", "href": "/api/v3/activities/77",
            "work_package_id": "1234",
        }])
        self.assertEqual(
            out["actions"][0]["created_at"], "2026-11-09T12:34:56Z"
        )
        self.assertEqual(state["patches"], [])
        self.assertEqual(state["posts"], [(
            "/api/v3/work_packages/1234/activities",
            {"comment": {"raw": "First line\nSecond line"}},
        )])

        proc, listed = self.run_toolkit(base_url, [
            "comment", "list", "--work-package", "1234",
        ])
        self.assertEqual(proc.returncode, 0, listed)
        self.assertEqual(listed["items"][0]["activity_id"], "77")

    def test_different_returned_comment_is_not_reported_as_saved(self):
        state = self.state(returned_comment="Different text")
        proc, out = self.run_toolkit(self.serve(state), [
            "comment", "add", "--work-package", "1234",
            "--text", "Expected text",
        ], apply="1")
        self.assertEqual(proc.returncode, 2, out)
        self.assertFalse(out["ok"])
        self.assertFalse(out["applied"])
        self.assertFalse(out["actions"][0]["applied"])
        self.assertEqual(out["error_codes"], ["comment_not_saved"])
        self.assertEqual(out["items"], [])
        self.assertNotIn("Expected text", out["errors"][0]["message"])

    def test_dry_run_names_activity_endpoint(self):
        state = self.state()
        proc, out = self.run_toolkit(self.serve(state), [
            "comment", "add", "--work-package", "1234", "--text", "Reviewed",
        ])
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["mode"], "dry_run")
        self.assertEqual(out["items"], [])
        self.assertEqual(
            out["actions"][0]["endpoint"],
            "POST /api/v3/work_packages/1234/activities",
        )
        self.assertEqual(state["posts"], [])

    def test_apply_by_work_package_still_needs_no_replan_plan(self):
        state = self.state()
        proc, out = self.run_toolkit(self.serve(state), [
            "comment", "add", "--work-package", "1234", "--text", "No plan",
        ], apply="1")
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["plan"], "none")
        self.assertNotIn("private_binding_layer", out)
        self.assertEqual(out["counts"]["applied_actions"], 1)


if __name__ == "__main__":
    unittest.main()
