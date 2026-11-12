import http.server
import json
import os
import stat
import subprocess
import sys
import tempfile
import threading
import unittest

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class ToolkitCreateBindTest(unittest.TestCase):
    def make_plan(self, directory, *, bound=False):
        canonical = os.path.join(directory, "canonical")
        os.makedirs(canonical)
        with open(os.path.join(canonical, "tasks.yaml"), "w", encoding="utf-8") as handle:
            yaml.safe_dump({"tasks": [{"id": "task.new", "title": "New task"}]}, handle)
        bindings = os.path.join(directory, "bindings.yaml")
        document = {"openproject_private_bindings": {
            "project": {"source": "openproject", "project_id": "demo"},
            "tasks": [], "dependencies": [], "resources": [],
        }}
        if bound:
            document["openproject_private_bindings"]["tasks"].append({
                "replan_task_id": "task.new",
                "source_ref": {
                    "source": "openproject", "object_type": "work_package",
                    "object_id": "40", "url": "https://example.invalid/work_packages/40",
                },
            })
        with open(bindings, "w", encoding="utf-8") as handle:
            yaml.safe_dump(document, handle, sort_keys=False)
        return bindings

    def run_toolkit(self, base_url, project, bindings, args, *, apply="1"):
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            REPLAN_PROJECT=project,
            REPLAN_OPENPROJECT_BASE_URL=base_url,
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
            TEST_TOKEN="test-token",
            REPLAN_OPENPROJECT_FROM_FILE="",
            REPLAN_OPENPROJECT_BINDINGS_PATH=bindings,
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

            def do_GET(self):
                path = self.path.split("?", 1)[0]
                if path == "/api/v3/projects/demo":
                    self.reply({"id": 9, "identifier": "demo"})
                elif path == "/api/v3/projects/9/types":
                    self.reply({"_embedded": {"elements": [{
                        "name": "Task", "_links": {"self": {"href": "/api/v3/types/1"}},
                    }]}})
                elif path == "/api/v3/projects/9/work_packages":
                    self.reply({"total": 0, "_embedded": {"elements": []}})
                elif path == "/api/v3/projects/9/versions":
                    self.reply({"total": 0, "pageSize": 100, "offset": 1,
                                "_embedded": {"elements": []}})
                elif path == "/api/v3/work_packages/41":
                    self.reply({
                        "id": 41, "subject": "Created and bound", "lockVersion": 1,
                        "_links": {"self": {"href": "/api/v3/work_packages/41"}},
                    })
                else:
                    self.reply({}, 404)

            def do_POST(self):
                payload = self.body()
                state["posts"].append((self.path, payload))
                if self.path == "/api/v3/projects/9/work_packages/form":
                    self.reply({"_embedded": {"validationErrors": {}}})
                elif self.path == "/api/v3/work_packages":
                    self.reply({
                        "id": 41, **payload,
                        "_links": {**payload.get("_links", {}),
                                   "self": {"href": "/api/v3/work_packages/41"}},
                    }, 201)
                else:
                    self.reply({}, 404)

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_port}"

    def test_create_task_names_bind_command_in_dry_run_and_apply(self):
        with tempfile.TemporaryDirectory() as project:
            bindings = self.make_plan(project)
            state = {"posts": []}
            base = self.serve(state)
            args = ["work-package", "create", "--task", "task.new",
                    "--subject", "Created task"]
            proc, preview = self.run_toolkit(base, project, bindings, args, apply="")
            self.assertEqual(proc.returncode, 0, preview)
            self.assertEqual(preview["items"], [])
            self.assertIn(
                "scripts/openproject-work-package-toolkit.sh work-package bind --task task.new --work-package <id>",
                preview["suggested_next_actions"],
            )
            proc, out = self.run_toolkit(base, project, bindings, args)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["items"], [{
                "kind": "work_package", "id": "41",
                "href": "/api/v3/work_packages/41", "subject": "Created task",
                "source_ref": out["created_source_ref"], "task_id": "task.new",
            }])
            self.assertIn(
                "scripts/openproject-work-package-toolkit.sh work-package bind --task task.new --work-package 41",
                out["suggested_next_actions"],
            )

    def test_create_bind_writes_resolvable_binding(self):
        with tempfile.TemporaryDirectory() as project:
            bindings = self.make_plan(project)
            state = {"posts": []}
            args = ["work-package", "create", "--task", "task.new", "--bind",
                    "--subject", "Created and bound"]
            proc, preview = self.run_toolkit(self.serve(state), project, bindings, args, apply="")
            self.assertEqual(proc.returncode, 0, preview)
            self.assertEqual(preview["items"], [])
            self.assertFalse(os.path.getsize(bindings) == 0)
            proc, out = self.run_toolkit(self.serve(state), project, bindings, args)
            self.assertEqual(proc.returncode, 0, out)
            self.assertTrue(out["actions"][0]["binding_written"])
            self.assertTrue(out["items"][0]["binding_written"])
            with open(bindings, encoding="utf-8") as handle:
                document = yaml.safe_load(handle)
            binding = document["openproject_private_bindings"]["tasks"][0]
            self.assertEqual(binding["replan_task_id"], "task.new")
            self.assertEqual(binding["source_ref"]["object_id"], "41")
            proc, inspected = self.run_toolkit(
                base_url=self.serve({"posts": []}), project=project,
                bindings=bindings,
                args=["work-package", "inspect", "--task", "task.new"],
                apply="",
            )
            self.assertEqual(proc.returncode, 0, inspected)
            self.assertEqual(inspected["items"][0]["work_package_id"], "41")
            self.assertEqual(inspected["items"][0]["task_id"], "task.new")

    def test_bind_preflight_refuses_missing_or_bound_task_before_post(self):
        for task_id, bound, code in (
            ("task.missing", False, "missing_replan_task"),
            ("task.new", True, "task_already_bound"),
        ):
            with self.subTest(code=code), tempfile.TemporaryDirectory() as project:
                bindings = self.make_plan(project, bound=bound)
                state = {"posts": []}
                proc, out = self.run_toolkit(self.serve(state), project, bindings, [
                    "work-package", "create", "--task", task_id, "--bind",
                    "--subject", "Must not be created",
                ])
                self.assertEqual(proc.returncode, 2, out)
                self.assertEqual(out["error_codes"], [code])
                self.assertEqual(out["items"], [])
                self.assertEqual(state["posts"], [])

    def test_binding_write_failure_reports_created_id_without_second_post(self):
        with tempfile.TemporaryDirectory() as project:
            bindings = self.make_plan(project)
            os.chmod(project, stat.S_IRUSR | stat.S_IXUSR)
            state = {"posts": []}
            try:
                proc, out = self.run_toolkit(self.serve(state), project, bindings, [
                    "work-package", "create", "--task", "task.new", "--bind",
                    "--subject", "Created before binding failure",
                ])
            finally:
                os.chmod(project, stat.S_IRWXU)
            self.assertEqual(proc.returncode, 7, out)
            self.assertEqual(out["error_codes"], ["binding_not_written"])
            self.assertEqual(out["created_work_package_id"], "41")
            self.assertFalse(out["actions"][0]["binding_written"])
            self.assertEqual(out["items"], [])
            self.assertEqual(out["finish_binding_command"],
                             "scripts/openproject-work-package-toolkit.sh work-package bind --task task.new --work-package 41")
            self.assertEqual(sum(path == "/api/v3/work_packages" for path, _ in state["posts"]), 1)

    def test_bind_without_task_is_usage_error(self):
        with tempfile.TemporaryDirectory() as project:
            bindings = self.make_plan(project)
            state = {"posts": []}
            proc, out = self.run_toolkit(self.serve(state), project, bindings, [
                "work-package", "create", "--bind", "--subject", "Invalid",
            ])
            self.assertEqual(proc.returncode, 8, out)
            self.assertEqual(out["error_codes"], ["invalid_arguments"])
            self.assertEqual(out["items"], [])
            self.assertEqual(state["posts"], [])


if __name__ == "__main__":
    unittest.main()
