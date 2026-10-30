import http.server
import json
import os
import subprocess
import sys
import threading
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class ToolkitRelationsParentTest(unittest.TestCase):
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
            [sys.executable, "-m", "openproject_work_package_toolkit",
             "--root-dir", ROOT, *args],
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

            def work_package(self, work_package_id):
                item = state["work_packages"][work_package_id]
                return {"id": int(work_package_id), **item}

            def do_GET(self):
                path = self.path.split("?", 1)[0]
                if path == "/api/v3/days/week":
                    self.reply({"_embedded": {"elements": [
                        {"day": day, "name": name, "working": day <= 5}
                        for day, name in enumerate((
                            "Monday", "Tuesday", "Wednesday", "Thursday",
                            "Friday", "Saturday", "Sunday",
                        ), 1)
                    ]}})
                    return
                if path.endswith("/relations"):
                    self.reply({
                        "total": len(state["relations"]),
                        "_embedded": {"elements": state["relations"]},
                    })
                    return
                if path.startswith("/api/v3/work_packages/"):
                    self.reply(self.work_package(path.rsplit("/", 1)[-1]))
                    return
                self.reply({}, 404)

            def do_POST(self):
                payload = self.body()
                state["posts"].append((self.path, payload))
                relation = {"id": 91, **payload}
                state["relations"].append(relation)
                if payload["type"] in {"follows", "precedes"}:
                    for item in state["work_packages"].values():
                        item["startDate"] = "2026-11-09"
                        item["dueDate"] = "2026-11-10"
                        item["scheduleManually"] = False
                        item["lockVersion"] += 1
                self.reply(relation, 201)

            def do_PATCH(self):
                payload = self.body()
                state["patches"].append((self.path, payload))
                if state.get("parent_error"):
                    status, identifier = state["parent_error"]
                    self.reply({"errorIdentifier": identifier, "message": "refused"}, status)
                    return
                work_package_id = self.path.rsplit("/", 1)[-1]
                item = state["work_packages"][work_package_id]
                item.update({key: value for key, value in payload.items() if key != "_links"})
                item["lockVersion"] += 1
                self.reply(self.work_package(work_package_id))

            def do_DELETE(self):
                state["deletes"].append(self.path)
                self.send_response(204)
                self.end_headers()

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_port}"

    def state(self):
        return {
            "relations": [], "posts": [], "patches": [], "deletes": [],
            "work_packages": {
                "1": {"subject": "One", "lockVersion": 3,
                      "startDate": "2026-11-02", "dueDate": "2026-11-03",
                      "scheduleManually": True, "_links": {}},
                "2": {"subject": "Two", "lockVersion": 4,
                      "startDate": "2026-11-04", "dueDate": "2026-11-05",
                      "scheduleManually": True, "_links": {}},
            },
        }

    def test_symmetric_relation_is_idempotent_and_list_uses_work_package_endpoint(self):
        state = self.state()
        state["relations"] = [{
            "id": 90, "type": "relates", "description": "Existing",
            "_links": {
                "from": {"href": "/api/v3/work_packages/2"},
                "to": {"href": "/api/v3/work_packages/1"},
            },
        }]
        base = self.serve(state)
        proc, out = self.run_toolkit(base, [
            "relation", "create", "--from", "1", "--to", "2",
            "--type", "relates",
        ], apply="1")
        self.assertEqual(proc.returncode, 0, out)
        self.assertTrue(out["already_exists"])
        self.assertEqual(out["actions"][0]["relation_id"], "90")
        self.assertEqual(state["posts"], [])

        proc, out = self.run_toolkit(
            base, ["relation", "list", "--work-package", "1"]
        )
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["items"][0]["relation_id"], "90")
        self.assertEqual(out["items"][0]["from_work_package_id"], "2")

    def test_follows_create_restores_manual_scheduling_and_endpoint_dates(self):
        state = self.state()
        base = self.serve(state)
        proc, out = self.run_toolkit(base, [
            "relation", "create", "--from", "2", "--to", "1",
            "--type", "follows", "--description", "After one",
        ], apply="1")
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"]["created_relation"], 1)
        self.assertEqual(out["counts"]["restored_work_packages"], 2)
        self.assertEqual(state["posts"][0][0], "/api/v3/work_packages/2/relations")
        self.assertEqual(state["posts"][0][1]["description"], "After one")
        self.assertEqual(
            [(item["startDate"], item["dueDate"], item["scheduleManually"])
             for item in state["work_packages"].values()],
            [
                ("2026-11-02", "2026-11-03", True),
                ("2026-11-04", "2026-11-05", True),
            ],
        )

    def test_parent_set_and_clear_payloads_include_lock_version(self):
        for extra, expected_parent in (
            (["--parent", "2"], {"href": "/api/v3/work_packages/2"}),
            (["--clear-parent"], {"href": None}),
        ):
            with self.subTest(extra=extra):
                state = self.state()
                base = self.serve(state)
                proc, out = self.run_toolkit(base, [
                    "work-package", "update-metadata", "--work-package", "1",
                    *extra,
                ], apply="1")
                self.assertEqual(proc.returncode, 0, out)
                payload = state["patches"][0][1]
                self.assertEqual(payload["lockVersion"], 3)
                self.assertEqual(payload["_links"]["parent"], expected_parent)

    def test_parent_permission_errors_are_mapped(self):
        cases = (
            (422, "urn:openproject-org:api:v3:errors:PropertyIsReadOnly"),
            (403, "urn:openproject-org:api:v3:errors:Unauthenticated"),
        )
        for status, identifier in cases:
            with self.subTest(status=status):
                state = self.state()
                state["parent_error"] = (status, identifier)
                base = self.serve(state)
                proc, out = self.run_toolkit(base, [
                    "work-package", "update-metadata", "--work-package", "1",
                    "--parent", "2",
                ], apply="1")
                self.assertEqual(proc.returncode, 6, out)
                self.assertEqual(out["error_codes"], ["openproject_permission_missing"])
                self.assertIn("hierarchies (subtasks)", out["errors"][0]["message"])

    def test_delete_without_plan_calls_relation_endpoint(self):
        state = self.state()
        base = self.serve(state)
        proc, out = self.run_toolkit(
            base, ["relation", "delete", "--relation", "90"], apply="1"
        )
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(state["deletes"], ["/api/v3/relations/90"])
        self.assertEqual(
            out["actions"][0]["private_binding_removal"],
            "not_available_without_plan",
        )


if __name__ == "__main__":
    unittest.main()
