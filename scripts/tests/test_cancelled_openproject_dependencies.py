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


class CancelledOpenProjectDependenciesTest(unittest.TestCase):
    def write_project(self, path, status, *, bind_dependency=False):
        canonical = os.path.join(path, "canonical")
        os.makedirs(canonical)
        tasks = [
            {
                "id": "task.a", "title": "A", "tracking_status": "in_progress",
                "planned_start": "2026-11-16", "planned_finish": "2026-11-17",
            },
            {
                "id": "task.b", "title": "B", "tracking_status": "in_progress",
                "planned_start": "2026-11-18", "planned_finish": "2026-11-19",
            },
        ]
        dependency = {
            "id": "dependency.a-before-b", "type": "finish_to_start",
            "predecessor_ref": {"kind": "task", "id": "task.a"},
            "successor_ref": {"kind": "task", "id": "task.b"},
            "status": status, "criticality": "hard",
        }
        documents = {
            "tasks.yaml": {"tasks": tasks},
            "dependencies.yaml": {"dependencies": [dependency]},
            "current_plan.yaml": {"current_plan": {"included_tasks": [
                {"kind": "task", "id": "task.a"},
                {"kind": "task", "id": "task.b"},
            ]}},
            "resources.yaml": {"resources": []},
            "resource_demands.yaml": {"resource_demands": []},
        }
        for name, document in documents.items():
            with open(os.path.join(canonical, name), "w", encoding="utf-8") as handle:
                yaml.safe_dump(document, handle)
        os.makedirs(os.path.join(path, ".replan"))
        with open(os.path.join(path, ".replan", "project.yaml"), "w", encoding="utf-8") as handle:
            yaml.safe_dump({"project": {"source_profiles": [{
                "id": "openproject.test", "source": "openproject",
                "base_url": "https://op.example.invalid", "project_id": "test",
                "token_env": "TEST_TOKEN",
            }]}}, handle)
        bindings_path = os.path.join(path, "bindings.yaml")
        dependency_bindings = []
        if bind_dependency:
            dependency_bindings.append({
                "replan_dependency_id": dependency["id"],
                "source_ref": {
                    "source": "openproject", "object_type": "relation",
                    "object_id": "90",
                },
                "relation_type": "follows",
                "from_work_package_id": "2", "to_work_package_id": "1",
            })
        with open(bindings_path, "w", encoding="utf-8") as handle:
            yaml.safe_dump({"openproject_private_bindings": {
                "tasks": [
                    {"replan_task_id": "task.a", "source_ref": {
                        "source": "openproject", "object_type": "work_package",
                        "object_id": "1",
                    }},
                    {"replan_task_id": "task.b", "source_ref": {
                        "source": "openproject", "object_type": "work_package",
                        "object_id": "2",
                    }},
                ],
                "dependencies": dependency_bindings,
                "resources": [],
            }}, handle)
        return bindings_path

    def import_file(self, path, *, live_relation=False, date_drift=False):
        document = {"ok": True, "result": {
            "task_candidates": [
                {
                    "source_id": "1", "title": "A", "source_status": "In progress",
                    "mapped_status": "in_progress",
                    "start_date": "2026-11-10" if date_drift else "2026-11-16",
                    "due_date": "2026-11-11" if date_drift else "2026-11-17",
                    "schedule_manually": True,
                },
                {
                    "source_id": "2", "title": "B", "source_status": "In progress",
                    "mapped_status": "in_progress", "start_date": "2026-11-18",
                    "due_date": "2026-11-19", "schedule_manually": True,
                },
            ],
            "dependency_candidates": ([{
                "source_id": "90", "relation_type": "follows",
                "dependency_type": "finish_to_start",
                "predecessor_task_id": "task.a", "successor_task_id": "task.b",
                "predecessor_source_id": "1", "successor_source_id": "2",
                "from_work_package_id": "2", "to_work_package_id": "1",
            }] if live_relation else []),
        }}
        import_path = os.path.join(path, "import.json")
        with open(import_path, "w", encoding="utf-8") as handle:
            json.dump(document, handle)
        return import_path

    def run_module(self, module, *args, env):
        proc = subprocess.run(
            [sys.executable, "-m", module, *args], capture_output=True, text=True,
            env={**os.environ, "PYTHONPATH": LIB, **env}, timeout=30,
        )
        self.assertEqual(proc.stderr, "")
        return proc.returncode, json.loads(proc.stdout)

    def serve(self, *, relations=()):
        state = {
            "posts": [], "deletes": [], "patches": [],
            "relations": list(relations),
            "work_packages": {
                "1": {"id": 1, "lockVersion": 1, "startDate": "2026-11-10",
                      "dueDate": "2026-11-11", "scheduleManually": True},
                "2": {"id": 2, "lockVersion": 1, "startDate": "2026-11-18",
                      "dueDate": "2026-11-19", "scheduleManually": True},
            },
        }

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
                if path == "/api/v3/relations":
                    self.reply({"total": len(state["relations"]), "pageSize": 500,
                                "offset": 1, "_embedded": {"elements": state["relations"]}})
                elif path == "/api/v3/days/week":
                    self.reply({"_embedded": {"elements": [
                        {"day": day, "name": str(day), "working": day <= 5}
                        for day in range(1, 8)
                    ]}})
                elif path.startswith("/api/v3/work_packages/"):
                    self.reply(state["work_packages"][path.rsplit("/", 1)[-1]])
                else:
                    self.reply({}, 404)

            def do_PATCH(self):
                work_package_id = self.path.rsplit("/", 1)[-1]
                payload = self.body()
                state["patches"].append((work_package_id, payload))
                item = state["work_packages"][work_package_id]
                item.update({
                    "startDate": payload.get("startDate", item["startDate"]),
                    "dueDate": payload.get("dueDate", item["dueDate"]),
                    "scheduleManually": payload.get("scheduleManually", item["scheduleManually"]),
                    "lockVersion": item["lockVersion"] + 1,
                })
                self.reply(item)

            def do_POST(self):
                state["posts"].append((self.path, self.body()))
                self.reply({}, 500)

            def do_DELETE(self):
                state["deletes"].append(self.path)
                self.send_response(204)
                self.end_headers()

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_port}", state

    def env(self, path, bindings, base_url=""):
        return {
            "REPLAN_PROJECT": path,
            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            "REPLAN_OPENPROJECT_BASE_URL": base_url,
            "REPLAN_OPENPROJECT_PROJECT_ID": "test",
            "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN",
            "TEST_TOKEN": "token",
            "REPLAN_APPLY": "1",
            "REPLAN_OPENPROJECT_IMPORT_JSON": "",
        }

    def test_date_push_skips_cancelled_dependency_cleanup_states(self):
        cases = (
            ("deleted and unbound", False, False, None),
            ("still live by tuple", False, True, "cancelled_dependency_still_live"),
            ("still live by binding", True, True, "cancelled_dependency_still_live"),
            ("binding left", True, False, "cancelled_dependency_binding_left"),
        )
        for name, bound, live, warning_code in cases:
            with self.subTest(name), tempfile.TemporaryDirectory() as path:
                bindings = self.write_project(path, "cancelled", bind_dependency=bound)
                import_path = self.import_file(
                    path, live_relation=live, date_drift=True,
                )
                base_url, state = self.serve()
                code, out = self.run_module(
                    "push_openproject_date_drift", "--import-json", import_path,
                    "--tmp-dir", path, "--root-dir", ROOT,
                    env=self.env(path, bindings, base_url),
                )
                self.assertEqual(code, 0, out)
                self.assertTrue(out["ok"], out)
                self.assertEqual(out["date_push_status"], "applied")
                self.assertEqual(out["counts"]["blocked_actions"], 0)
                self.assertEqual(len(state["patches"]), 1)
                if warning_code:
                    self.assertIn(warning_code, out["warning_codes"])
                    warning = next(item for item in out["warnings"] if item["code"] == warning_code)
                    self.assertIn("relation delete --relation 90 --remove-private-binding", warning["delete_command"])
                else:
                    self.assertNotIn("cancelled_dependency_still_live", out["warning_codes"])

    def test_date_push_still_blocks_missing_open_and_resolved_dependencies(self):
        for status in ("open", "resolved"):
            with self.subTest(status), tempfile.TemporaryDirectory() as path:
                bindings = self.write_project(path, status, bind_dependency=True)
                import_path = self.import_file(path, date_drift=True)
                base_url, state = self.serve()
                code, out = self.run_module(
                    "push_openproject_date_drift", "--import-json", import_path,
                    "--tmp-dir", path, "--root-dir", ROOT,
                    env=self.env(path, bindings, base_url),
                )
                self.assertEqual(code, 11, out)
                self.assertEqual(out["date_push_status"], "blocked")
                self.assertTrue(any(
                    item.get("reason") == "missing_live_dependency"
                    for item in out["blocked_actions"]
                ))
                self.assertEqual(state["patches"], [])

    def test_drift_and_operator_report_treat_cancelled_cleanup_as_warning(self):
        cases = (
            (False, False, None, "aligned"),
            (False, True, "cancelled_dependency_still_live", "soft_drift"),
            (True, True, "cancelled_dependency_still_live", "soft_drift"),
            (True, False, "cancelled_dependency_binding_left", "soft_drift"),
        )
        for bound, live, warning_code, status in cases:
            with self.subTest(bound=bound, live=live), tempfile.TemporaryDirectory() as path:
                bindings = self.write_project(path, "cancelled", bind_dependency=bound)
                import_path = self.import_file(path, live_relation=live)
                env = self.env(path, bindings)
                env["REPLAN_APPLY"] = ""
                for module, status_field in (
                    ("check_openproject_drift", "drift_status"),
                    ("openproject_operator_report", "operator_report_status"),
                ):
                    code, out = self.run_module(
                        module, "--import-json", import_path,
                        "--tmp-dir", path, "--root-dir", ROOT, env=env,
                    )
                    self.assertEqual(code, 0, out)
                    self.assertEqual(out[status_field], status)
                    self.assertEqual(out["counts"]["unbound_local_dependencies"], 0)
                    self.assertEqual(out["counts"]["missing_live_dependencies"], 0)
                    if warning_code:
                        self.assertIn(warning_code, out["warning_codes"])

    def test_drift_and_operator_report_keep_open_and_resolved_missing_hard(self):
        for dependency_status in ("open", "resolved"):
            with self.subTest(dependency_status), tempfile.TemporaryDirectory() as path:
                bindings = self.write_project(path, dependency_status, bind_dependency=True)
                import_path = self.import_file(path)
                env = self.env(path, bindings)
                env["REPLAN_APPLY"] = ""
                for module, status_field in (
                    ("check_openproject_drift", "drift_status"),
                    ("openproject_operator_report", "operator_report_status"),
                ):
                    code, out = self.run_module(
                        module, "--import-json", import_path,
                        "--tmp-dir", path, "--root-dir", ROOT, env=env,
                    )
                    self.assertEqual(code, 11, out)
                    self.assertEqual(out[status_field], "drift")
                    self.assertEqual(out["counts"]["missing_live_dependencies"], 1)

    def test_relation_apply_skips_cancelled_and_never_posts_or_deletes(self):
        relation = {
            "id": 90, "type": "follows", "_links": {
                "from": {"href": "/api/v3/work_packages/2"},
                "to": {"href": "/api/v3/work_packages/1"},
            },
        }
        for live in (False, True):
            with self.subTest(live=live), tempfile.TemporaryDirectory() as path:
                bindings = self.write_project(path, "cancelled", bind_dependency=False)
                base_url, state = self.serve(relations=[relation] if live else [])
                code, out = self.run_module(
                    "apply_openproject_relations", "--root-dir", ROOT,
                    env=self.env(path, bindings, base_url),
                )
                self.assertEqual(out["counts"]["skip_cancelled_dependency"], 1)
                self.assertEqual(out["actions"][0]["action"], "skip_cancelled_dependency")
                self.assertEqual(state["posts"], [])
                self.assertEqual(state["deletes"], [])
                self.assertEqual(out["private_binding_layer"]["dependency_bindings"], 0)
                if live:
                    self.assertEqual(code, 11, out)
                    self.assertIn("cancelled_dependency_still_live", out["warning_codes"])
                else:
                    self.assertEqual(code, 0, out)

    def test_bootstrap_and_plan_backed_relation_list_show_cancelled_state(self):
        with tempfile.TemporaryDirectory() as path:
            bindings = self.write_project(path, "cancelled")
            env = self.env(path, bindings)
            env.update({
                "REPLAN_APPLY": "",
                "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "test",
                "REPLAN_OPENPROJECT_TARGET_PROJECT_NAME": "Test",
            })
            code, out = self.run_module("plan_openproject_bootstrap", env=env)
            self.assertEqual(code, 0, out)
            self.assertEqual(out["dependency_relations"], [])
            self.assertEqual(out["counts"]["skipped_cancelled_dependencies"], 1)

            code, out = self.run_module(
                "openproject_work_package_toolkit", "--root-dir", ROOT,
                "relation", "list", env=env,
            )
            self.assertEqual(code, 0, out)
            self.assertEqual(out["items"][0]["replan_dependency_status"], "cancelled")
            self.assertFalse(out["items"][0]["expected_openproject_relation"])
            self.assertEqual(out["items"][0]["binding_status"], "cancelled")


if __name__ == "__main__":
    unittest.main()
