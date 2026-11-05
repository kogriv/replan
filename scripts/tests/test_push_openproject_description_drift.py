import http.server
import json
import os
import subprocess
import tempfile
import threading
import unittest
import urllib.parse

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SCRIPT = os.path.join(ROOT, "scripts", "push-openproject-description-drift.sh")


class DescriptionDriftPushTest(unittest.TestCase):
    def setUp(self):
        self.state = {
            "work_packages": {},
            "patches": [],
            "project_gets": [],
            "list_gets": [],
            "item_gets": [],
            "conflicts": set(),
        }
        state = self.state

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, status, body):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def work_package_id(self):
                return urllib.parse.urlsplit(self.path).path.rsplit("/", 1)[-1]

            def do_GET(self):
                parsed = urllib.parse.urlsplit(self.path)
                if parsed.path == "/api/v3/projects/demo":
                    state["project_gets"].append(parsed.path)
                    self.reply(200, {"id": 42, "identifier": "demo"})
                    return
                if parsed.path == "/api/v3/work_packages":
                    query = urllib.parse.parse_qs(parsed.query)
                    state["list_gets"].append(query)
                    work_packages = list(state["work_packages"].values())
                    page_size = int(query["pageSize"][0])
                    offset = int(query["offset"][0])
                    start = (offset - 1) * page_size
                    self.reply(200, {
                        "total": len(work_packages),
                        "count": len(work_packages[start:start + page_size]),
                        "pageSize": page_size,
                        "offset": offset,
                        "_embedded": {
                            "elements": work_packages[start:start + page_size]
                        },
                    })
                    return
                work_package_id = self.work_package_id()
                state["item_gets"].append(work_package_id)
                work_package = state["work_packages"].get(work_package_id)
                if work_package is None:
                    self.reply(404, {
                        "errorIdentifier": "urn:openproject-org:api:v3:errors:NotFound",
                        "message": "Not found",
                    })
                    return
                self.reply(200, work_package)

            def do_PATCH(self):
                work_package_id = self.work_package_id()
                length = int(self.headers.get("Content-Length", "0"))
                payload = json.loads(self.rfile.read(length))
                state["patches"].append((work_package_id, payload))
                if work_package_id in state["conflicts"]:
                    self.reply(409, {
                        "errorIdentifier": "urn:openproject-org:api:v3:errors:UpdateConflict",
                        "message": "Conflict",
                    })
                    return
                work_package = state["work_packages"][work_package_id]
                work_package["description"] = payload["description"]
                work_package["lockVersion"] += 1
                self.reply(200, work_package)

            def log_message(self, *_args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)

    def work_package(self, work_package_id, description, lock_version=3):
        self.state["work_packages"][str(work_package_id)] = {
            "id": int(work_package_id),
            "description": {"format": "markdown", "raw": description},
            "lockVersion": lock_version,
        }

    def project(self, tmp, tasks):
        project = os.path.join(tmp, "project")
        canonical = os.path.join(project, "canonical")
        os.makedirs(canonical)
        with open(os.path.join(canonical, "tasks.yaml"), "w", encoding="utf-8") as stream:
            yaml.safe_dump({"tasks": tasks}, stream, sort_keys=False)
        bindings = os.path.join(tmp, "bindings.yaml")
        with open(bindings, "w", encoding="utf-8") as stream:
            yaml.safe_dump({
                "openproject_private_bindings": {
                    "project": {"project_id": "demo"},
                    "tasks": [
                        {
                            "replan_task_id": task["id"],
                            "source_ref": {
                                "source": "openproject",
                                "object_type": "work_package",
                                "object_id": str(index),
                            },
                        }
                        for index, task in enumerate(tasks, 1)
                    ],
                    "dependencies": [],
                    "resources": [],
                }
            }, stream, sort_keys=False)
        return project, bindings

    def run_push(self, project, bindings, tmp, *, apply="", tasks="", overrides=None):
        env_file = os.path.join(tmp, "empty.env")
        with open(env_file, "w", encoding="utf-8"):
            pass
        env = {
            "PATH": os.environ.get("PATH", ""),
            "LANG": "C.UTF-8",
            "REPLAN_ENV_FILE": env_file,
            "REPLAN_PROJECT": project,
            "REPLAN_OPENPROJECT_BASE_URL": f"http://127.0.0.1:{self.server.server_port}",
            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "",
            "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN",
            "REPLAN_OPENPROJECT_TOKEN_FILE": "",
            "TEST_TOKEN": "test-token",
            "REPLAN_OPENPROJECT_PERIMETER_KEY": "",
            "REPLAN_OPENPROJECT_PERIMETER_KEY_FILE": "",
            "REPLAN_OPENPROJECT_PERIMETER_HEADER": "X-Perimeter-Key",
            "REPLAN_OPENPROJECT_TIMEOUT_SECONDS": "5",
            "REPLAN_OPENPROJECT_RETRIES": "0",
            "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS": "0.01",
            "REPLAN_OPENPROJECT_HOST_HEADER": "",
            "REPLAN_OPENPROJECT_FORWARDED_PROTO": "",
            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            "REPLAN_TASKS": tasks,
            "REPLAN_KEEP_TEMP": "",
            "REPLAN_FORMAT": "json",
            "REPLAN_APPLY": apply,
            "TMPDIR": tmp,
        }
        env.update(overrides or {})
        proc = subprocess.run(
            [SCRIPT], capture_output=True, text=True, env=env, timeout=30
        )
        return proc, json.loads(proc.stdout)

    def test_equal_text_has_no_action(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{"id": "task.a", "description": "Same"}])
            self.work_package(1, "Same")
            proc, out = self.run_push(project, bindings, tmp)

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"], {
            "compared": 1, "differing": 0, "planned_actions": 0,
            "applied_actions": 0, "skipped": 0,
        })
        self.assertEqual(out["actions"], [])
        self.assertEqual(self.state["patches"], [])
        self.assertEqual(self.state["item_gets"], [])
        self.assertEqual(len(self.state["list_gets"]), 1)
        self.assertEqual(self.state["list_gets"][0]["pageSize"], ["500"])
        self.assertEqual(self.state["list_gets"][0]["offset"], ["1"])

    def test_equal_text_with_bootstrap_trailer_has_no_action(self):
        text = "Same\n\nRePlan task id: `task.a`"
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{"id": "task.a", "description": "Same"}])
            self.work_package(1, text)
            proc, out = self.run_push(project, bindings, tmp)

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"]["differing"], 0)
        self.assertEqual(out["actions"], [])
        self.assertEqual(self.state["patches"], [])

    def test_equal_description_with_human_text_after_trailer_has_no_action(self):
        text = (
            "Same\n\nRePlan task id: `task.a`\n\n"
            "Result: accepted in RePlan. Evidence: docs/dev/x.md\n"
        )
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{"id": "task.a", "description": "Same"}])
            self.work_package(1, text)
            proc, out = self.run_push(project, bindings, tmp)

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"]["differing"], 0)
        self.assertEqual(out["actions"], [])
        self.assertEqual(self.state["patches"], [])

    def test_project_work_packages_are_read_in_pages_without_item_gets(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{"id": "task.a", "description": "Same"}])
            self.work_package(1, "Same")
            for work_package_id in range(2, 502):
                self.work_package(work_package_id, f"Unbound {work_package_id}")
            proc, out = self.run_push(project, bindings, tmp)

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"]["compared"], 1)
        self.assertEqual(self.state["item_gets"], [])
        self.assertEqual(
            [query["offset"] for query in self.state["list_gets"]],
            [["1"], ["2"]],
        )

    def test_changed_description_with_bootstrap_trailer_keeps_trailer(self):
        tail = "RePlan task id: `task.a`"
        wanted = "New description\n\n" + tail
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{
                "id": "task.a", "description": "New description",
            }])
            self.work_package(1, "Old description\n\nRePlan task id: `task.a`", 9)
            proc, out = self.run_push(project, bindings, tmp, apply="1")

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["actions"][0]["kept_tail_length"], len(tail))
        self.assertEqual(out["actions"][0]["new_length"], len(wanted))
        self.assertEqual(self.state["patches"], [("1", {
            "description": {"format": "markdown", "raw": wanted},
            "lockVersion": 9,
        })])

    def test_changed_description_keeps_trailer_and_human_tail_byte_for_byte(self):
        tail = (
            "RePlan task id: `task.a`\r\n\r\n"
            "Result: accepted in RePlan. Evidence: docs/dev/x.md  \r\n"
        )
        wanted = "New description\n\n" + tail
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{
                "id": "task.a", "description": "New description",
            }])
            self.work_package(1, "Old description\r\n\r\n" + tail, 11)
            proc, out = self.run_push(project, bindings, tmp, apply="1")

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["actions"][0]["kept_tail_length"], len(tail))
        self.assertEqual(self.state["patches"], [("1", {
            "description": {"format": "markdown", "raw": wanted},
            "lockVersion": 11,
        })])

    def test_crlf_and_trailing_whitespace_only_have_no_action(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{
                "id": "task.a", "description": "Line one\nLine two",
            }])
            self.work_package(1, "Line one\r\nLine two \t\r\n")
            proc, out = self.run_push(project, bindings, tmp)

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"]["differing"], 0)
        self.assertEqual(self.state["patches"], [])

    def test_difference_dry_run_and_apply_patch_only_description_and_lock(self):
        new_text = "New first line\n" + "x" * 100
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{"id": "task.a", "description": new_text}])
            self.work_package(1, "Old\ntext", 7)
            dry_proc, dry = self.run_push(project, bindings, tmp)
            apply_proc, applied = self.run_push(project, bindings, tmp, apply="1")

        self.assertEqual(dry_proc.returncode, 0, dry)
        self.assertEqual(dry["actions"][0]["action"], "update_description")
        self.assertEqual(dry["actions"][0]["old_length"], 8)
        self.assertEqual(dry["actions"][0]["new_length"], len(new_text))
        self.assertNotIn("\n", dry["actions"][0]["new_preview"])
        self.assertLessEqual(len(dry["actions"][0]["new_preview"]), 80)
        self.assertNotIn(new_text, json.dumps(dry))
        self.assertEqual(apply_proc.returncode, 0, applied)
        self.assertEqual(applied["actions"][0]["action"], "updated_description")
        self.assertEqual(applied["actions"][0]["kept_tail_length"], 0)
        self.assertEqual(applied["counts"]["applied_actions"], 1)
        self.assertEqual(self.state["patches"], [("1", {
            "description": {"format": "markdown", "raw": new_text},
            "lockVersion": 7,
        })])

    def test_empty_replan_description_is_skipped_without_request(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{"id": "task.a", "description": " \r\n\t"}])
            self.work_package(1, "Keep this")
            proc, out = self.run_push(project, bindings, tmp)

        self.assertEqual(proc.returncode, 0, out)
        self.assertTrue(out["ok"])
        self.assertEqual(out["skipped"][0]["reason"], "replan_description_empty")
        self.assertEqual(out["warning_codes"], ["replan_description_empty"])
        self.assertEqual(out["counts"]["skipped"], 1)
        self.assertEqual(self.state["project_gets"], [])
        self.assertEqual(self.state["list_gets"], [])
        self.assertEqual(self.state["item_gets"], [])
        self.assertEqual(self.state["patches"], [])

    def test_conflict_continues_and_reports_partial_apply(self):
        tasks = [
            {"id": "task.a", "description": "New A"},
            {"id": "task.b", "description": "New B"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, tasks)
            self.work_package(1, "Old A")
            self.work_package(2, "Old B")
            self.state["conflicts"].add("1")
            proc, out = self.run_push(project, bindings, tmp, apply="1")

        self.assertEqual(proc.returncode, 7, out)
        self.assertEqual(out["error_codes"], ["openproject_work_package_changed"])
        self.assertEqual(out["errors"][0]["target"]["id"], "1")
        self.assertEqual(out["counts"]["planned_actions"], 2)
        self.assertEqual(out["counts"]["applied_actions"], 1)
        self.assertEqual([item[0] for item in self.state["patches"]], ["1", "2"])
        self.assertEqual(out["skipped"][0]["reason"], "openproject_work_package_changed")

    def test_missing_bound_work_package_continues(self):
        tasks = [
            {"id": "task.a", "description": "New A"},
            {"id": "task.b", "description": "Same B"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, tasks)
            self.work_package(2, "Same B")
            proc, out = self.run_push(project, bindings, tmp)

        self.assertEqual(proc.returncode, 11, out)
        self.assertEqual(out["error_codes"], ["bound_work_package_missing"])
        self.assertEqual(self.state["item_gets"], ["1"])
        self.assertEqual(len(self.state["list_gets"]), 1)
        self.assertEqual(out["counts"]["compared"], 1)
        self.assertEqual(out["counts"]["skipped"], 1)

    def test_task_filter_and_unknown_task_refusal(self):
        tasks = [
            {"id": "task.a", "description": "New A"},
            {"id": "task.b", "description": "New B"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, tasks)
            self.work_package(1, "Old A")
            self.work_package(2, "Old B")
            proc, out = self.run_push(project, bindings, tmp, tasks="task.b")
            unknown_proc, unknown = self.run_push(
                project, bindings, tmp, tasks="task.b,task.nope"
            )

        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual([action["task_id"] for action in out["actions"]], ["task.b"])
        self.assertEqual(self.state["item_gets"], [])
        self.assertEqual(len(self.state["list_gets"]), 1)
        self.assertEqual(unknown_proc.returncode, 8, unknown)
        self.assertEqual(unknown["error_codes"], ["unknown_task_ids"])
        self.assertEqual(unknown["unknown_task_ids"], ["task.nope"])

    def test_settings_refusals(self):
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp, [{"id": "task.a", "description": "New"}])
            cases = [
                ({"REPLAN_FORMAT": "yaml"}, "unsupported_format"),
                ({"REPLAN_OPENPROJECT_BASE_URL": ""}, "missing_base_url"),
                ({"REPLAN_PROJECT": ""}, "missing_setting"),
                ({"REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "other"}, "conflicting_settings"),
            ]
            for overrides, code in cases:
                with self.subTest(code=code):
                    proc, out = self.run_push(
                        project, bindings, tmp, overrides=overrides
                    )
                    self.assertEqual(proc.returncode, 8, out)
                    self.assertEqual(out["error_codes"], [code])
                    self.assertEqual(self.state["project_gets"], [])
                    self.assertEqual(self.state["list_gets"], [])
                    self.assertEqual(self.state["item_gets"], [])


if __name__ == "__main__":
    unittest.main()
