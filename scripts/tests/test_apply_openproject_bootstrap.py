"""Private bindings merge coverage for apply_openproject_bootstrap."""

import http.server
import json
import os
import subprocess
import sys
import tempfile
import threading
import unittest
import urllib.parse

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class BootstrapBindingsTest(unittest.TestCase):
    def setUp(self):
        self.created_ids = []
        self.work_package_get_paths = []
        self.work_package_posts = 0
        self.work_package_payloads = []
        self.fail_work_package_post = 0

        owner = self

        class OpenProject(http.server.BaseHTTPRequestHandler):
            work_packages = [
                {"id": 909, "subject": "Conflicting task"},
            ] + [
                {"id": 1000 + index, "subject": f"Unrelated task {index}"}
                for index in range(999)
            ] + [{"id": 101, "subject": "Existing task"}]

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
                if self.path == "/api/v3/users/me":
                    self.reply({"id": 7, "name": "Test User"})
                elif self.path == "/api/v3/projects/demo":
                    self.reply({"id": 42, "identifier": "demo"})
                elif self.path == "/api/v3/projects/42/types":
                    self.reply({"_embedded": {"elements": [{"name": "Task", "_links": {"self": {"href": "/api/v3/types/1"}}}]}})
                elif self.path == "/api/v3/statuses":
                    self.reply({"_embedded": {"elements": [{"name": "New", "_links": {"self": {"href": "/api/v3/statuses/1"}}}]}})
                elif self.path.startswith("/api/v3/memberships?"):
                    self.reply({"_embedded": {"elements": [{"_links": {"principal": {"href": "/api/v3/users/7"}}}]}})
                elif self.path.startswith("/api/v3/work_packages?"):
                    owner.work_package_get_paths.append(self.path)
                    query = urllib.parse.parse_qs(urllib.parse.urlsplit(self.path).query)
                    offset = int(query["offset"][0])
                    page_size = int(query["pageSize"][0])
                    start = (offset - 1) * page_size
                    self.reply({
                        "total": len(self.work_packages),
                        "pageSize": page_size,
                        "offset": offset,
                        "_embedded": {"elements": self.work_packages[start:start + page_size]},
                    })
                elif self.path == "/api/v3/days/week":
                    self.reply({"_embedded": {"elements": [
                        {"day": day, "working": day <= 5} for day in range(1, 8)
                    ]}})
                else:
                    self.reply({}, 404)

            def do_POST(self):
                if self.path == "/api/v3/work_packages":
                    payload = self.body()
                    owner.work_package_payloads.append(payload)
                    owner.work_package_posts += 1
                    if owner.work_package_posts == owner.fail_work_package_post:
                        self.reply({"message": "stand-in failure"}, 500)
                        return
                    work_package_id = 200 + owner.work_package_posts
                    owner.created_ids.append(work_package_id)
                    self.reply({"id": work_package_id, "subject": payload["subject"]}, 201)
                else:
                    self.body()
                    self.reply({"id": 1}, 201)

            def log_message(self, *args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)
        self.base_url = f"http://127.0.0.1:{self.server.server_address[1]}"

    def write_plan(self, directory, tasks):
        path = os.path.join(directory, "plan.json")
        with open(path, "w") as fh:
            json.dump({"project_path": "/example", "work_packages": tasks}, fh)
        return path

    def run_bootstrap(self, plan, bindings, apply="1", resource_id="resource.assignee"):
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            TEST_TOKEN="token",
            REPLAN_OPENPROJECT_BASE_URL=self.base_url,
            REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_PROJECT_NAME="Demo",
            REPLAN_OPENPROJECT_ASSIGNEE_USER_ID="7",
            REPLAN_OPENPROJECT_ASSIGNEE_NAME="Test User",
            REPLAN_OPENPROJECT_BINDINGS_PATH=bindings,
            REPLAN_APPLY=apply,
        )
        if resource_id:
            env["REPLAN_OPENPROJECT_ASSIGNEE_RESOURCE_ID"] = resource_id
        else:
            env.pop("REPLAN_OPENPROJECT_ASSIGNEE_RESOURCE_ID", None)
        proc = subprocess.run(
            [
                sys.executable, "-m", "apply_openproject_bootstrap",
                "--plan-json", plan, "--root-dir", ROOT,
            ],
            capture_output=True,
            text=True,
            env=env,
            timeout=30,
        )
        return proc, json.loads(proc.stdout)

    def existing_doc(self):
        return {
            "openproject_private_bindings": {
                "generated_at": "2026-01-01T00:00:00+00:00",
                "project": {"source": "openproject", "project_id": "demo", "project_href": "/api/v3/projects/42"},
                "tasks": [
                    {"replan_task_id": "task.existing", "source_ref": {"object_id": "101"}},
                    {"replan_task_id": "task.kept", "source_ref": {"object_id": "303"}},
                ],
                "dependencies": [
                    {"replan_dependency_id": "dependency.one", "source_ref": {"object_id": "501"}},
                    {"replan_dependency_id": "dependency.two", "source_ref": {"object_id": "502"}},
                ],
                "resources": [
                    {"replan_resource_id": "resource.assignee", "source_ref": {"object_id": "7"}},
                ],
                "extension": {"keep": True},
            }
        }

    def test_existing_bindings_are_merged_and_dry_run_is_byte_stable(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = os.path.join(tmp, "bindings.yaml")
            original = self.existing_doc()
            with open(bindings, "w") as fh:
                yaml.safe_dump(original, fh, sort_keys=False)
            plan = self.write_plan(tmp, [
                {"replan_task_id": "task.existing", "subject": "Existing task"},
                {"replan_task_id": "task.new", "subject": "New task"},
            ])

            with open(bindings, "rb") as fh:
                before = fh.read()
            proc, dry = self.run_bootstrap(plan, bindings, apply="")
            with open(bindings, "rb") as fh:
                after = fh.read()
            self.assertEqual(proc.returncode, 0, dry)
            self.assertEqual(before, after)
            self.assertEqual(dry["private_binding_layer"], {
                "path": bindings, "exists": True, "written": False, "task_bindings": 3,
                "added_task_bindings": 1, "kept_task_bindings": 2,
                "dependency_bindings": 2, "resource_bindings": 1,
            })

            proc, out = self.run_bootstrap(plan, bindings)
            self.assertEqual(proc.returncode, 0, out)
            with open(bindings) as fh:
                merged = yaml.safe_load(fh)["openproject_private_bindings"]
            self.assertEqual([item["replan_task_id"] for item in merged["tasks"]], ["task.existing", "task.kept", "task.new"])
            self.assertEqual(merged["dependencies"], original["openproject_private_bindings"]["dependencies"])
            self.assertEqual(merged["resources"], original["openproject_private_bindings"]["resources"])
            self.assertEqual(merged["extension"], {"keep": True})
            self.assertNotEqual(merged["generated_at"], "2026-01-01T00:00:00+00:00")
            self.assertEqual(out["private_binding_layer"], {
                "path": bindings, "exists": True, "written": True, "task_bindings": 3,
                "added_task_bindings": 1, "kept_task_bindings": 2,
                "dependency_bindings": 2, "resource_bindings": 1,
            })

    def test_existing_work_package_on_third_page_is_reused(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = os.path.join(tmp, "bindings.yaml")
            plan = self.write_plan(tmp, [
                {"replan_task_id": "task.existing", "subject": "Existing task"},
            ])

            proc, out = self.run_bootstrap(plan, bindings)

            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(self.created_ids, [])
            self.assertEqual(out["counts"]["created_work_package"], 0)
            with open(bindings) as fh:
                tasks = yaml.safe_load(fh)["openproject_private_bindings"]["tasks"]
            self.assertEqual(tasks[0]["source_ref"]["object_id"], "101")
            queries = [
                urllib.parse.parse_qs(urllib.parse.urlsplit(path).query)
                for path in self.work_package_get_paths
            ]
            self.assertEqual([query["offset"] for query in queries], [["1"], ["2"], ["3"]])
            self.assertTrue(all(query["pageSize"] == ["500"] for query in queries))
            self.assertTrue(all("filters" in query for query in queries))

    def test_conflicting_task_binding_warns_and_keeps_existing(self):
        with tempfile.TemporaryDirectory() as tmp:
            doc = self.existing_doc()
            doc["openproject_private_bindings"]["tasks"][0] = {
                "replan_task_id": "task.conflict", "source_ref": {"object_id": "808"},
            }
            bindings = os.path.join(tmp, "bindings.yaml")
            with open(bindings, "w") as fh:
                yaml.safe_dump(doc, fh, sort_keys=False)
            plan = self.write_plan(tmp, [{"replan_task_id": "task.conflict", "subject": "Conflicting task"}])

            proc, out = self.run_bootstrap(plan, bindings)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["warning_codes"], ["binding_conflict"])
            warning = out["warnings"][0]
            self.assertEqual((warning["target"]["id"], warning["existing_work_package_id"], warning["bootstrap_work_package_id"]),
                             ("task.conflict", "808", "909"))
            with open(bindings) as fh:
                tasks = yaml.safe_load(fh)["openproject_private_bindings"]["tasks"]
            self.assertEqual(tasks[0]["source_ref"]["object_id"], "808")

    def test_conflicting_resource_binding_warns_and_keeps_existing(self):
        with tempfile.TemporaryDirectory() as tmp:
            doc = self.existing_doc()
            doc["openproject_private_bindings"]["resources"][0]["source_ref"]["object_id"] = "99"
            bindings = os.path.join(tmp, "bindings.yaml")
            with open(bindings, "w") as fh:
                yaml.safe_dump(doc, fh, sort_keys=False)
            plan = self.write_plan(tmp, [{"replan_task_id": "task.existing", "subject": "Existing task"}])

            proc, out = self.run_bootstrap(plan, bindings)
            self.assertEqual(proc.returncode, 0, out)
            warning = out["warnings"][0]
            self.assertEqual((warning["code"], warning["target"], warning["existing_user_id"], warning["bootstrap_user_id"]),
                             ("binding_conflict", {"kind": "resource", "id": "resource.assignee"}, "99", "7"))
            with open(bindings) as fh:
                resources = yaml.safe_load(fh)["openproject_private_bindings"]["resources"]
            self.assertEqual(resources[0]["source_ref"]["object_id"], "99")

    def test_project_mismatch_exits_conflict_and_does_not_write(self):
        with tempfile.TemporaryDirectory() as tmp:
            doc = self.existing_doc()
            doc["openproject_private_bindings"]["project"]["project_id"] = "other"
            bindings = os.path.join(tmp, "bindings.yaml")
            with open(bindings, "w") as fh:
                yaml.safe_dump(doc, fh, sort_keys=False)
            with open(bindings, "rb") as fh:
                before = fh.read()
            plan = self.write_plan(tmp, [{"replan_task_id": "task.existing", "subject": "Existing task"}])

            proc, out = self.run_bootstrap(plan, bindings)
            self.assertEqual((proc.returncode, out["error_codes"]), (5, ["bindings_project_mismatch"]))
            with open(bindings, "rb") as fh:
                self.assertEqual(fh.read(), before)
            self.assertEqual(self.created_ids, [])

    def test_invalid_file_exits_validation_and_does_not_write(self):
        for original in (b"openproject_private_bindings: [unterminated\n", b"another_root: {}\n"):
            with self.subTest(original=original), tempfile.TemporaryDirectory() as tmp:
                bindings = os.path.join(tmp, "bindings.yaml")
                with open(bindings, "wb") as fh:
                    fh.write(original)
                plan = self.write_plan(tmp, [{"replan_task_id": "task.existing", "subject": "Existing task"}])

                proc, out = self.run_bootstrap(plan, bindings)
                self.assertEqual((proc.returncode, out["error_codes"]), (2, ["bindings_file_invalid"]))
                with open(bindings, "rb") as fh:
                    self.assertEqual(fh.read(), original)
                self.assertEqual(self.created_ids, [])

    def test_no_file_writes_the_original_binding_shape(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = os.path.join(tmp, "bindings.yaml")
            plan = self.write_plan(tmp, [{"replan_task_id": "task.existing", "subject": "Existing task"}])

            proc, out = self.run_bootstrap(plan, bindings)
            self.assertEqual(proc.returncode, 0, out)
            with open(bindings) as fh:
                root = yaml.safe_load(fh)["openproject_private_bindings"]
            self.assertEqual(root["project"]["project_id"], "demo")
            self.assertEqual(len(root["tasks"]), 1)
            self.assertEqual(root["dependencies"], [])
            self.assertEqual(len(root["resources"]), 1)
            self.assertEqual(out["private_binding_layer"]["exists"], False)
            self.assertTrue(out["private_binding_layer"]["written"])

    def test_partial_apply_writes_confirmed_binding_and_stops(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.fail_work_package_post = 2
            bindings = os.path.join(tmp, "bindings.yaml")
            plan = self.write_plan(tmp, [
                {"replan_task_id": "task.a", "subject": "New A"},
                {"replan_task_id": "task.b", "subject": "New B"},
                {"replan_task_id": "task.c", "subject": "New C"},
            ])

            proc, out = self.run_bootstrap(plan, bindings)

            self.assertEqual(proc.returncode, 7, out)
            self.assertEqual(self.work_package_posts, 2)
            self.assertEqual(out["applied_so_far"][0]["replan_task_id"], "task.a")
            self.assertEqual(out["failed_step"]["replan_task_id"], "task.b")
            self.assertTrue(out["failed_step"]["may_have_been_applied"])
            self.assertTrue(out["private_binding_layer"]["written"])
            with open(bindings) as fh:
                tasks = yaml.safe_load(fh)["openproject_private_bindings"]["tasks"]
            self.assertEqual(
                [(item["replan_task_id"], item["source_ref"]["object_id"]) for item in tasks],
                [("task.a", "201")],
            )

    def test_first_apply_failure_keeps_external_exit_and_has_no_applied_so_far(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.fail_work_package_post = 1
            bindings = os.path.join(tmp, "bindings.yaml")
            plan = self.write_plan(tmp, [
                {"replan_task_id": "task.a", "subject": "New A"},
                {"replan_task_id": "task.b", "subject": "New B"},
            ])

            proc, out = self.run_bootstrap(plan, bindings)

            self.assertEqual(proc.returncode, 6, out)
            self.assertEqual(self.work_package_posts, 1)
            self.assertNotIn("applied_so_far", out)
            self.assertEqual(out["failed_step"]["replan_task_id"], "task.a")
            self.assertFalse(out["private_binding_layer"]["written"])

    def test_weekend_dates_set_ignore_non_working_days(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = os.path.join(tmp, "bindings.yaml")
            plan = self.write_plan(tmp, [{
                "replan_task_id": "task.sunday",
                "subject": "Sunday task",
                "start_date": "2026-10-25",
                "due_date": "2026-10-25",
            }])

            proc, dry = self.run_bootstrap(plan, bindings, apply="")
            self.assertEqual(proc.returncode, 0, dry)
            self.assertTrue(dry["actions"][0]["set_ignore_non_working_days"])
            self.assertIn("Sunday", dry["actions"][0]["ignore_non_working_days_reason"])

            proc, out = self.run_bootstrap(plan, bindings)

        self.assertEqual(proc.returncode, 0, out)
        self.assertIs(self.work_package_payloads[0]["ignoreNonWorkingDays"], True)


if __name__ == "__main__":
    unittest.main()
