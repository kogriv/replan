import http.server
import json
import os
import subprocess
import sys
import tempfile
import threading
import unittest

import yaml
import openproject_rescheduling


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


def working_week():
    return {"_embedded": {"elements": [
        {"day": day, "name": name, "working": day <= 5}
        for day, name in enumerate(
            ("Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"),
            1,
        )
    ]}}


class ReschedulingReportTest(unittest.TestCase):
    def run_module(self, module, *args, env):
        settings = {
            "REPLAN_OPENPROJECT_TOKEN_ENV": "REPLAN_TEST_OPENPROJECT_TOKEN",
            "REPLAN_TEST_OPENPROJECT_TOKEN": "test-token",
            "REPLAN_OPENPROJECT_PERIMETER_KEY": "",
            "REPLAN_OVERLAY_PUBLISH_TARGET": "",
            "REPLAN_OPENPROJECT_FROM_FILE": "",
            "REPLAN_OPENPROJECT_IMPORT_JSON": "",
        }
        proc = subprocess.run(
            [sys.executable, "-m", module, *args], capture_output=True, text=True,
            env=dict(os.environ, PYTHONPATH=LIB, **settings, **env), timeout=60,
        )
        return proc.returncode, json.loads(proc.stdout)

    def start_server(self, handler):
        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_address[1]}"

    def write_project(self, tmp, tasks, dependencies, dependency_bindings=None):
        canonical = os.path.join(tmp, "canonical")
        os.makedirs(canonical)
        with open(os.path.join(canonical, "tasks.yaml"), "w") as fh:
            yaml.safe_dump({"tasks": tasks}, fh)
        with open(os.path.join(canonical, "dependencies.yaml"), "w") as fh:
            yaml.safe_dump({"dependencies": dependencies}, fh)
        with open(os.path.join(canonical, "current_plan.yaml"), "w") as fh:
            yaml.safe_dump({"current_plan": {"included_tasks": [
                {"kind": "task", "id": task["id"]} for task in tasks
            ]}}, fh)
        bindings = os.path.join(tmp, "bindings.yaml")
        with open(bindings, "w") as fh:
            yaml.safe_dump({"openproject_private_bindings": {
                "tasks": [
                    {"replan_task_id": task["id"], "source_ref": {
                        "source": "openproject", "object_type": "work_package",
                        "object_id": str(index),
                    }}
                    for index, task in enumerate(tasks, 1)
                ],
                "dependencies": dependency_bindings or [],
                "resources": [],
            }}, fh)
        return bindings

    def relation_env(self, tmp, bindings, base):
        return {
            "REPLAN_PROJECT": tmp,
            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            "REPLAN_OPENPROJECT_BASE_URL": base,
            "REPLAN_OPENPROJECT_PROJECT_ID": "test-project",
            "REPLAN_APPLY": "1",
        }

    def test_relation_create_restores_successor_chain_roots_first(self):
        state = {
            "created": False,
            "patch_ids": [],
            "payloads": [],
            "dates": {
                "2": ["2026-10-25", "2026-10-25"],
                "3": ["2026-10-26", "2026-10-26"],
            },
            "manual": {"2": True, "3": False},
            "locks": {"2": 1, "3": 1},
            "collection_reads": 0,
        }

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
                    self.reply(working_week())
                    return
                if self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 1, "pageSize": 500, "offset": 1, "_embedded": {"elements": [{
                        "id": 90, "type": "follows", "_links": {
                            "from": {"href": "/api/v3/work_packages/3"},
                            "to": {"href": "/api/v3/work_packages/2"},
                        },
                    }]}})
                    return
                if self.path.startswith("/api/v3/projects/test-project/work_packages?"):
                    state["collection_reads"] += 1
                    elements = [{
                        "id": int(wp_id), "lockVersion": state["locks"][wp_id],
                        "startDate": state["dates"][wp_id][0],
                        "dueDate": state["dates"][wp_id][1],
                        "scheduleManually": state["manual"][wp_id],
                    } for wp_id in ("2", "3")]
                    self.reply({
                        "total": len(elements), "pageSize": 500, "offset": 1,
                        "_embedded": {"elements": elements},
                    })
                    return
                wp_id = self.path.rsplit("/", 1)[-1]
                self.reply({
                    "id": int(wp_id), "lockVersion": state["locks"][wp_id],
                    "startDate": state["dates"][wp_id][0], "dueDate": state["dates"][wp_id][1],
                    "scheduleManually": state["manual"][wp_id],
                })

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                state["created"] = True
                state["dates"]["2"] = ["2026-11-03", "2026-11-03"]
                state["dates"]["3"] = ["2026-11-04", "2026-11-04"]
                state["manual"]["2"] = False
                self.reply({"id": 91, "type": "follows", "_links": {
                    "from": {"href": "/api/v3/work_packages/2"},
                    "to": {"href": "/api/v3/work_packages/1"},
                }}, 201)

            def do_PATCH(self):
                wp_id = self.path.rsplit("/", 1)[-1]
                payload = json.loads(self.rfile.read(int(self.headers.get("Content-Length") or 0)))
                state["patch_ids"].append(wp_id)
                state["payloads"].append(payload)
                state["dates"][wp_id] = [payload.get("startDate", ""), payload.get("dueDate", "")]
                state["manual"][wp_id] = payload["scheduleManually"]
                state["locks"][wp_id] += 1
                if wp_id == "2":
                    state["locks"]["3"] += 1
                self.reply({
                    "id": int(wp_id), "lockVersion": state["locks"][wp_id],
                    "startDate": state["dates"][wp_id][0], "dueDate": state["dates"][wp_id][1],
                    "scheduleManually": state["manual"][wp_id],
                })

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": f"task.{name}", "title": name.upper()} for name in ("a", "b", "c")]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps)
            base = self.start_server(OpenProject)
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT,
                env=self.relation_env(tmp, bindings, base),
            )

        self.assertEqual(code, 0, out)
        self.assertEqual(out["warning_codes"], [])
        self.assertEqual(state["patch_ids"], ["2", "3"])
        self.assertEqual(
            [item["work_package_id"] for item in out["restored_work_packages"]],
            ["2", "3"],
        )
        self.assertIs(state["payloads"][0]["ignoreNonWorkingDays"], True)
        self.assertEqual(state["payloads"][1]["lockVersion"], 2)
        self.assertEqual(state["collection_reads"], 3)
        self.assertNotIn("rescheduled_work_packages", out)

    def test_project_snapshot_falls_back_for_an_id_missing_from_collection(self):
        class Client:
            def __init__(self):
                self.collection_reads = 0
                self.individual_reads = []

            def paginate(self, path, page_size, params):
                self.collection_reads += 1
                return [{
                    "id": 2, "lockVersion": 1, "startDate": "2026-11-10",
                    "dueDate": "2026-11-10", "scheduleManually": True,
                }]

            def request_json(self, method, path, expected):
                work_package_id = path.rsplit("/", 1)[-1]
                self.individual_reads.append(work_package_id)
                return 200, {
                    "id": int(work_package_id), "lockVersion": 2,
                    "startDate": "2026-11-11", "dueDate": "2026-11-11",
                    "scheduleManually": False,
                }

        client = Client()
        snapshots, unread = openproject_rescheduling.project_work_package_dates(
            client, "test-project", {"2", "3"}
        )

        self.assertEqual(client.collection_reads, 1)
        self.assertEqual(client.individual_reads, ["3"])
        self.assertEqual(set(snapshots), {"2", "3"})
        self.assertEqual(unread, [])

    def test_relation_create_post_read_failure_needs_attention_not_partial(self):
        state = {"created": False}

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 0, "pageSize": 500, "offset": 1, "_embedded": {"elements": []}})
                elif state["created"]:
                    self.reply({"message": "stand-in read failure"}, 500)
                else:
                    self.reply({"id": 2, "lockVersion": 1, "startDate": "2026-10-20", "dueDate": "2026-10-20"})

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                state["created"] = True
                self.reply({"id": 91, "type": "follows", "_links": {}}, 201)

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": "task.a", "title": "A"}, {"id": "task.b", "title": "B"}]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps)
            base = self.start_server(OpenProject)
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT,
                env=self.relation_env(tmp, bindings, base),
            )

        self.assertEqual(code, 11, out)
        self.assertFalse(out["ok"])
        self.assertEqual(out["warning_codes"], ["rescheduling_unknown"])
        self.assertEqual(out["warnings"][0]["work_package_ids"], ["2"])
        self.assertEqual(out["counts"]["created_relation"], 1)

    def test_relation_create_without_date_move_has_no_new_warning_or_list(self):
        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 0, "pageSize": 500, "offset": 1, "_embedded": {"elements": []}})
                else:
                    self.reply({"id": 2, "lockVersion": 1, "startDate": "2026-10-20", "dueDate": "2026-10-20"})

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                self.reply({"id": 91, "type": "follows", "_links": {}}, 201)

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": "task.a", "title": "A"}, {"id": "task.b", "title": "B"}]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps)
            base = self.start_server(OpenProject)
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT,
                env=self.relation_env(tmp, bindings, base),
            )

        self.assertEqual(code, 0, out)
        self.assertEqual(out["warning_codes"], [])
        self.assertNotIn("rescheduled_work_packages", out)

    def test_relation_dry_run_plans_pending_and_automatic_reachable_restores(self):
        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body):
                data = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 1, "pageSize": 500, "offset": 1, "_embedded": {"elements": [{
                        "id": 90, "type": "follows", "_links": {
                            "from": {"href": "/api/v3/work_packages/3"},
                            "to": {"href": "/api/v3/work_packages/2"},
                        },
                    }]}})
                    return
                if self.path.startswith("/api/v3/projects/test-project/work_packages?"):
                    elements = [{
                        "id": int(wp_id), "lockVersion": 1,
                        "startDate": "2026-11-10", "dueDate": "2026-11-10",
                        "scheduleManually": wp_id == "2",
                    } for wp_id in ("2", "3")]
                    self.reply({"total": 2, "pageSize": 500, "offset": 1, "_embedded": {"elements": elements}})
                    return
                wp_id = self.path.rsplit("/", 1)[-1]
                self.reply({
                    "id": int(wp_id), "lockVersion": 1,
                    "startDate": "2026-11-10", "dueDate": "2026-11-10",
                    "scheduleManually": wp_id == "2",
                })

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": f"task.{name}", "title": name.upper()} for name in ("a", "b", "c")]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps)
            env = self.relation_env(tmp, bindings, self.start_server(OpenProject))
            env["REPLAN_APPLY"] = ""
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT, env=env,
            )

        self.assertEqual(code, 0, out)
        self.assertEqual(out["mode"], "dry_run")
        self.assertEqual(out["warning_codes"], [])
        self.assertEqual(
            [item["work_package_id"] for item in out["restored_work_packages"]],
            ["2", "3"],
        )
        self.assertTrue(all(item["action"] == "restore_manual_schedule" for item in out["restored_work_packages"]))

    def test_relation_apply_makes_an_already_automatic_successor_manual(self):
        state = {"created": False, "payload": None}

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
                    self.reply(working_week())
                elif self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 0, "pageSize": 500, "offset": 1, "_embedded": {"elements": []}})
                else:
                    self.reply({"id": 2, "lockVersion": 4, "startDate": "2026-11-10", "dueDate": "2026-11-10", "scheduleManually": state["payload"] is not None})

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                state["created"] = True
                self.reply({"id": 91, "type": "follows", "_links": {}}, 201)

            def do_PATCH(self):
                state["payload"] = json.loads(self.rfile.read(int(self.headers.get("Content-Length") or 0)))
                self.reply({"id": 2, "lockVersion": 5, "startDate": "2026-11-10", "dueDate": "2026-11-10", "scheduleManually": True})

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": "task.a", "title": "A"}, {"id": "task.b", "title": "B"}]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps)
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT,
                env=self.relation_env(tmp, bindings, self.start_server(OpenProject)),
            )

        self.assertEqual(code, 0, out)
        self.assertIs(state["payload"]["scheduleManually"], True)
        self.assertEqual(state["payload"]["startDate"], "2026-11-10")
        self.assertEqual(len(out["restored_work_packages"]), 1)

    def test_relation_rerun_repairs_an_existing_automatic_successor(self):
        state = {"manual": False, "patches": 0}

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body):
                data = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path == "/api/v3/days/week":
                    self.reply(working_week())
                elif self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 1, "pageSize": 500, "offset": 1, "_embedded": {"elements": [{
                        "id": 91, "type": "follows", "_links": {
                            "from": {"href": "/api/v3/work_packages/2"},
                            "to": {"href": "/api/v3/work_packages/1"},
                        },
                    }]}})
                else:
                    self.reply({"id": 2, "lockVersion": 4, "startDate": "2026-11-03", "dueDate": "2026-11-03", "scheduleManually": state["manual"]})

            def do_PATCH(self):
                payload = json.loads(self.rfile.read(int(self.headers.get("Content-Length") or 0)))
                state["patches"] += 1
                state["manual"] = True
                self.reply({"id": 2, "lockVersion": 5, "startDate": payload["startDate"], "dueDate": payload["dueDate"], "scheduleManually": True})

            def log_message(self, *_args):
                pass

        dep_binding = {
            "replan_dependency_id": "dep.ab",
            "source_ref": {"source": "openproject", "object_type": "relation", "object_id": "91"},
            "relation_type": "follows", "from_work_package_id": "2", "to_work_package_id": "1",
        }
        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": "task.a", "title": "A"}, {"id": "task.b", "title": "B"}]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps, [dep_binding])
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT,
                env=self.relation_env(tmp, bindings, self.start_server(OpenProject)),
            )

        self.assertEqual(code, 0, out)
        self.assertEqual(state["patches"], 1)
        self.assertEqual(out["counts"]["created_relation"], 0)
        self.assertEqual(out["restored_work_packages"][0]["work_package_id"], "2")

    def test_relation_restore_can_be_disabled(self):
        state = {"created": False, "patches": 0}

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 0, "pageSize": 500, "offset": 1, "_embedded": {"elements": []}})
                else:
                    date = "2026-11-03" if state["created"] else "2026-11-10"
                    self.reply({"id": 2, "lockVersion": 2, "startDate": date, "dueDate": date, "scheduleManually": False})

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                state["created"] = True
                self.reply({"id": 91, "type": "follows", "_links": {}}, 201)

            def do_PATCH(self):
                state["patches"] += 1

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": "task.a", "title": "A"}, {"id": "task.b", "title": "B"}]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps)
            env = self.relation_env(tmp, bindings, self.start_server(OpenProject))
            env["REPLAN_OPENPROJECT_KEEP_AUTOMATIC_SCHEDULING"] = "1"
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT, env=env,
            )

        self.assertEqual(code, 0, out)
        self.assertEqual(state["patches"], 0)
        self.assertEqual(out["restored_work_packages"], [])
        self.assertEqual(out["warning_codes"], ["openproject_rescheduled"])

    def test_relation_restore_failure_reports_applied_restore_steps(self):
        state = {"created": False, "patch_ids": []}

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
                    self.reply(working_week())
                elif self.path.startswith("/api/v3/relations?"):
                    self.reply({"total": 1, "pageSize": 500, "offset": 1, "_embedded": {"elements": [{
                        "id": 90, "type": "follows", "_links": {"from": {"href": "/api/v3/work_packages/3"}, "to": {"href": "/api/v3/work_packages/2"}},
                    }]}})
                elif self.path.startswith("/api/v3/projects/test-project/work_packages?"):
                    moved = state["created"]
                    elements = [{
                        "id": int(wp_id), "lockVersion": 2,
                        "startDate": "2026-11-03" if moved else "2026-11-10",
                        "dueDate": "2026-11-03" if moved else "2026-11-10",
                        "scheduleManually": not moved,
                    } for wp_id in ("2", "3")]
                    self.reply({"total": 2, "pageSize": 500, "offset": 1, "_embedded": {"elements": elements}})
                else:
                    wp_id = self.path.rsplit("/", 1)[-1]
                    moved = state["created"]
                    self.reply({"id": int(wp_id), "lockVersion": 2, "startDate": "2026-11-03" if moved else "2026-11-10", "dueDate": "2026-11-03" if moved else "2026-11-10", "scheduleManually": not moved})

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                state["created"] = True
                self.reply({"id": 91, "type": "follows", "_links": {}}, 201)

            def do_PATCH(self):
                wp_id = self.path.rsplit("/", 1)[-1]
                payload = json.loads(self.rfile.read(int(self.headers.get("Content-Length") or 0)))
                state["patch_ids"].append(wp_id)
                if wp_id == "3":
                    self.reply({"message": "stand-in restore failure"}, 500)
                else:
                    self.reply({"id": 2, "lockVersion": 3, "startDate": payload["startDate"], "dueDate": payload["dueDate"], "scheduleManually": True})

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            tasks = [{"id": f"task.{name}", "title": name.upper()} for name in ("a", "b", "c")]
            deps = [{"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}]
            bindings = self.write_project(tmp, tasks, deps)
            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT,
                env=self.relation_env(tmp, bindings, self.start_server(OpenProject)),
            )

        self.assertEqual(code, 7, out)
        self.assertEqual(state["patch_ids"], ["2", "3"])
        self.assertEqual(out["failed_step"]["action"], "restore_manual_schedule")
        self.assertEqual(out["failed_step"]["work_package_id"], "3")
        self.assertTrue(any(
            item.get("action") == "restore_manual_schedule" and item.get("work_package_id") == "2"
            for item in out["applied_so_far"]
        ))

    def date_push(self, tmp, tasks, live, handler, dependencies=None, dependency_bindings=None, dep_candidates=None):
        dependencies = dependencies or []
        bindings = self.write_project(tmp, tasks, dependencies, dependency_bindings)
        import_path = os.path.join(tmp, "import.json")
        with open(import_path, "w") as fh:
            json.dump({"ok": True, "result": {
                "task_candidates": live,
                "dependency_candidates": dep_candidates or [],
            }}, fh)
        base = self.start_server(handler)
        return self.run_module(
            "push_openproject_date_drift", "--import-json", import_path,
            "--tmp-dir", tmp, "--root-dir", ROOT,
            env={
                "REPLAN_PROJECT": tmp,
                "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                "REPLAN_OPENPROJECT_BASE_URL": base,
                "REPLAN_OPENPROJECT_PROJECT_ID": "test-project",
                "REPLAN_APPLY": "1",
            },
        )

    def test_date_push_reports_sunday_stored_as_monday(self):
        state = {"patched": False, "payload": {}}

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body):
                data = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path == "/api/v3/days/week":
                    self.reply(working_week())
                    return
                value = "2026-10-26" if state["patched"] else "2026-10-23"
                self.reply({"id": 1, "lockVersion": 2, "startDate": value, "dueDate": value})

            def do_PATCH(self):
                state["payload"] = json.loads(self.rfile.read(int(self.headers.get("Content-Length") or 0)))
                state["patched"] = True
                self.reply({"id": 1, "lockVersion": 3, "startDate": "2026-10-26", "dueDate": "2026-10-26", "scheduleManually": True})

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            task = {"id": "task.a", "title": "A", "tracking_status": "in_progress", "planned_start": "2026-10-25", "planned_finish": "2026-10-25"}
            live = [{"source_id": "1", "title": "A", "source_status": "In progress", "mapped_status": "in_progress", "start_date": "2026-10-23", "due_date": "2026-10-23"}]
            code, out = self.date_push(tmp, [task], live, OpenProject)

        self.assertEqual(code, 0, out)
        self.assertEqual(out["warning_codes"], ["openproject_stored_other_dates"])
        self.assertIs(state["payload"]["ignoreNonWorkingDays"], True)
        self.assertTrue(out["applied_actions"][0]["set_ignore_non_working_days"])
        self.assertIn("Sunday", out["applied_actions"][0]["ignore_non_working_days_reason"])
        self.assertEqual(len(out["stored_date_differences"]), 2)
        self.assertEqual({item["likely_reason"] for item in out["stored_date_differences"]}, {"non_working_day"})
        self.assertNotIn("rescheduled_work_packages", out)

    def test_date_push_reports_a_moved_successor(self):
        state = {"patched": False}

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body):
                data = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path == "/api/v3/days/week":
                    self.reply(working_week())
                    return
                wp_id = self.path.rsplit("/", 1)[-1]
                if wp_id == "1":
                    dates = ("2026-10-20", "2026-10-20") if not state["patched"] else ("2026-10-21", "2026-10-22")
                else:
                    dates = ("2026-10-23", "2026-10-24") if not state["patched"] else ("2026-10-24", "2026-10-25")
                self.reply({"id": int(wp_id), "lockVersion": 2, "startDate": dates[0], "dueDate": dates[1]})

            def do_PATCH(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                state["patched"] = True
                self.reply({"id": 1, "lockVersion": 3, "startDate": "2026-10-21", "dueDate": "2026-10-22", "scheduleManually": True})

            def log_message(self, *_args):
                pass

        dep = {"id": "dep.ab", "type": "finish_to_start", "predecessor_ref": {"id": "task.a"}, "successor_ref": {"id": "task.b"}}
        dep_binding = {"replan_dependency_id": "dep.ab", "source_ref": {"source": "openproject", "object_type": "relation", "object_id": "9"}, "relation_type": "follows", "from_work_package_id": "2", "to_work_package_id": "1"}
        dep_candidate = {"source_id": "9", "dependency_type": "finish_to_start", "predecessor_task_id": "task.a", "successor_task_id": "task.b"}
        tasks = [
            {"id": "task.a", "title": "A", "tracking_status": "in_progress", "planned_start": "2026-10-21", "planned_finish": "2026-10-22"},
            {"id": "task.b", "title": "B", "tracking_status": "in_progress", "planned_start": "2026-10-23", "planned_finish": "2026-10-24"},
        ]
        live = [
            {"source_id": "1", "title": "A", "source_status": "In progress", "mapped_status": "in_progress", "start_date": "2026-10-20", "due_date": "2026-10-20"},
            {"source_id": "2", "title": "B", "source_status": "In progress", "mapped_status": "in_progress", "start_date": "2026-10-23", "due_date": "2026-10-24"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            code, out = self.date_push(tmp, tasks, live, OpenProject, [dep], [dep_binding], [dep_candidate])

        self.assertEqual(code, 0, out)
        self.assertEqual(out["warning_codes"], ["openproject_rescheduled"])
        self.assertEqual({item["work_package_id"] for item in out["rescheduled_work_packages"]}, {"2"})
        self.assertEqual(out["rescheduled_work_packages"][0]["caused_by_date_push"]["task_id"], "task.a")

    def test_date_push_exact_storage_has_no_new_output(self):
        state = {"patched": False}

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body):
                data = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                if self.path == "/api/v3/days/week":
                    self.reply(working_week())
                    return
                dates = ("2026-10-21", "2026-10-22") if state["patched"] else ("2026-10-20", "2026-10-20")
                self.reply({"id": 1, "lockVersion": 2, "startDate": dates[0], "dueDate": dates[1]})

            def do_PATCH(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                state["patched"] = True
                self.reply({"id": 1, "lockVersion": 3, "startDate": "2026-10-21", "dueDate": "2026-10-22", "scheduleManually": True})

            def log_message(self, *_args):
                pass

        with tempfile.TemporaryDirectory() as tmp:
            task = {"id": "task.a", "title": "A", "tracking_status": "in_progress", "planned_start": "2026-10-21", "planned_finish": "2026-10-22"}
            live = [{"source_id": "1", "title": "A", "source_status": "In progress", "mapped_status": "in_progress", "start_date": "2026-10-20", "due_date": "2026-10-20"}]
            code, out = self.date_push(tmp, [task], live, OpenProject)

        self.assertEqual(code, 0, out)
        self.assertEqual(out["warning_codes"], [])
        self.assertNotIn("stored_date_differences", out)
        self.assertNotIn("rescheduled_work_packages", out)


if __name__ == "__main__":
    unittest.main()
