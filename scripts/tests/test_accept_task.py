import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SCRIPT = os.path.join(ROOT, "scripts", "accept-task.sh")
EXAMPLE = os.path.join(ROOT, "examples", "small-team")


def load_yaml(path):
    with open(path, encoding="utf-8") as stream:
        return yaml.safe_load(stream) or {}


def dump_yaml(path, value):
    with open(path, "w", encoding="utf-8") as stream:
        yaml.safe_dump(value, stream, sort_keys=False)


def read_text(path):
    with open(path, encoding="utf-8") as stream:
        return stream.read()


class OpenProjectHandler(BaseHTTPRequestHandler):
    status_name = "Developed"
    lock_version = 3
    start_date = ""
    due_date = ""

    def reply(self, value, status=200):
        body = json.dumps(value).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    @classmethod
    def work_package(cls):
        status_id = 6 if cls.status_name == "Closed" else 5
        return {
            "id": 123,
            "subject": "Implement validation command",
            "startDate": cls.start_date,
            "dueDate": cls.due_date,
            "lockVersion": cls.lock_version,
            "_links": {
                "self": {"href": "/api/v3/work_packages/123"},
                "status": {"href": f"/api/v3/statuses/{status_id}", "title": cls.status_name},
            },
        }

    def do_GET(self):
        if self.path == "/api/v3/work_packages/123":
            self.reply(type(self).work_package())
        elif self.path == "/api/v3/statuses":
            self.reply({"_embedded": {"elements": [
                {"name": "Developed", "_links": {"self": {"href": "/api/v3/statuses/5"}}},
                {"name": "Closed", "_links": {"self": {"href": "/api/v3/statuses/6"}}},
            ]}})
        else:
            self.reply({"_type": "Error", "errorIdentifier": "missing", "message": self.path}, 404)

    def do_POST(self):
        if self.path == "/api/v3/work_packages/123/form":
            self.reply({"_embedded": {"schema": {"status": {"_links": {"allowedValues": [
                {"href": "/api/v3/statuses/6"},
            ]}}}}})
        else:
            self.reply({"_type": "Error", "errorIdentifier": "missing", "message": self.path}, 404)

    def do_PATCH(self):
        if self.path != "/api/v3/work_packages/123":
            self.reply({"_type": "Error", "errorIdentifier": "missing", "message": self.path}, 404)
            return
        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        if "startDate" in payload:
            type(self).start_date = payload["startDate"]
        if "dueDate" in payload:
            type(self).due_date = payload["dueDate"]
        if (payload.get("_links") or {}).get("status"):
            type(self).status_name = "Closed"
        type(self).lock_version += 1
        self.reply(type(self).work_package())

    def log_message(self, *_args):
        pass


class AcceptTaskTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        OpenProjectHandler.status_name = "Developed"
        OpenProjectHandler.lock_version = 3
        OpenProjectHandler.start_date = ""
        OpenProjectHandler.due_date = ""
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), OpenProjectHandler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def setUp(self):
        OpenProjectHandler.status_name = "Developed"
        OpenProjectHandler.lock_version = 3
        OpenProjectHandler.start_date = ""
        OpenProjectHandler.due_date = ""
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.project = os.path.join(self.temp.name, "project")
        shutil.copytree(EXAMPLE, self.project)
        tasks_path = os.path.join(self.project, "canonical", "tasks.yaml")
        tasks = load_yaml(tasks_path)
        next(task for task in tasks["tasks"] if task["id"] == "task.op.1234")["tracking_status"] = "review_ready"
        dump_yaml(tasks_path, tasks)
        self.bindings = os.path.join(self.temp.name, "bindings.yaml")
        dump_yaml(self.bindings, {"openproject_private_bindings": {
            "project": {"source": "openproject", "project_id": "demo"},
            "tasks": [
                {"replan_task_id": "task.op.1234", "source_ref": {
                    "source": "openproject", "object_type": "work_package", "object_id": "123",
                }},
                {"replan_task_id": "task.offline.repair-pump", "source_ref": {
                    "source": "openproject", "object_type": "work_package", "object_id": "124",
                }},
            ],
            "dependencies": [],
        }})

    def environment(self, **overrides):
        env = dict(os.environ)
        env.update({
            "REPLAN_PROJECT": self.project,
            "REPLAN_TASK": "task.op.1234",
            "REPLAN_ACCEPTED_BY": "actor.human.operator",
            "REPLAN_ACTOR": "actor.human.operator",
            "REPLAN_ACCEPT_SUMMARY": "Operator accepted the completed task.",
            "REPLAN_ACCEPT_AT": "2026-10-26T12:00:00+07:00",
            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
            "REPLAN_OPENPROJECT_BASE_URL": f"http://127.0.0.1:{self.server.server_port}",
            "REPLAN_OPENPROJECT_BINDINGS_PATH": self.bindings,
            # A token name no .env.local sets, so a repository settings file
            # can neither refill it nor send its real token and perimeter key.
            "REPLAN_OPENPROJECT_TOKEN_ENV": "REPLAN_TEST_OPENPROJECT_TOKEN",
            "REPLAN_TEST_OPENPROJECT_TOKEN": "test-token",
            "REPLAN_OPENPROJECT_PERIMETER_KEY": "",
            # Empty and temporary, so the repository .env.local cannot supply
            # its publish target and no marker lands in the repository tmp/.
            "REPLAN_OVERLAY_PUBLISH_TARGET": "",
            "REPLAN_OVERLAY_OUTPUT_DIR": os.path.join(self.temp.name, "overlay"),
            "REPLAN_BIN": os.environ["REPLAN_BIN"],
        })
        env.update(overrides)
        return env

    def run_helper(self, *, apply=False, env=None):
        actual_env = self.environment() if env is None else env
        if apply:
            actual_env["REPLAN_APPLY"] = "1"
        else:
            actual_env.pop("REPLAN_APPLY", None)
        proc = subprocess.run([SCRIPT], capture_output=True, text=True, env=actual_env, timeout=60)
        self.assertEqual(proc.stderr, "", proc.stderr)
        return proc, json.loads(proc.stdout)

    def test_dry_run_prints_ordered_validated_plan_without_writes(self):
        before = read_text(os.path.join(self.project, "canonical", "tasks.yaml"))
        proc, out = self.run_helper()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["mode"], "dry_run")
        self.assertEqual(sorted(set(item["step"] for item in out["actions"])), list(range(1, 11)))
        validated = [item for item in out["actions"] if item["step"] <= 5 and item["state"] == "would_run"]
        self.assertTrue(validated)
        self.assertTrue(all(item.get("cli_dry_run", {}).get("ok") for item in validated))
        self.assertTrue(all(isinstance(item.get("command"), list) for item in validated))
        self.assertNotIn("test-token", proc.stdout)
        actual_window = next(item for item in out["actions"] if item["step"] == 6)
        self.assertEqual(actual_window["name"], "record_actual_window")
        self.assertEqual(actual_window["state"], "would_run")
        self.assertTrue(actual_window["cli_dry_run"]["ok"])
        date_step = next(item for item in out["actions"] if item["step"] == 7)
        self.assertEqual(date_step["name"], "update_openproject_dates")
        self.assertEqual(date_step["state"], "would_run")
        overlay = next(item for item in out["actions"] if item["step"] == 10)
        self.assertEqual(overlay["state"], "skipped")
        self.assertIn("accept_overlay_publish_target_not_set", out["warning_codes"])
        self.assertEqual(read_text(os.path.join(self.project, "canonical", "tasks.yaml")), before)
        self.assertFalse(any(name.startswith("evidence.2026-W44") for name in os.listdir(os.path.join(self.project, "evidence"))))

    def test_partial_helper_copy_keeps_overlay_scripts_unavailable_warning(self):
        partial_root = os.path.join(self.temp.name, "partial-kit")
        shutil.copytree(os.path.join(ROOT, "scripts"), os.path.join(partial_root, "scripts"),
                        ignore=shutil.ignore_patterns("__pycache__", "tests"))
        os.remove(os.path.join(partial_root, "scripts", "build-openproject-live-overlay.sh"))
        os.remove(os.path.join(partial_root, "scripts", "publish-openproject-overlay.sh"))
        env = self.environment(REPLAN_OVERLAY_PUBLISH_TARGET=os.path.join(self.temp.name, "published"))
        proc = subprocess.run(
            [os.path.join(partial_root, "scripts", "accept-task.sh")],
            capture_output=True, text=True, env=env, timeout=60,
        )
        self.assertEqual(proc.stderr, "", proc.stderr)
        out = json.loads(proc.stdout)
        self.assertEqual(proc.returncode, 0, out)
        overlay = next(item for item in out["actions"] if item["step"] == 10)
        self.assertEqual(overlay["state"], "skipped")
        self.assertIn("accept_overlay_scripts_unavailable", out["warning_codes"])
        self.assertNotIn("repository-only", json.dumps(overlay))

    def test_apply_completes_canonical_chain_and_second_run_is_no_op(self):
        dependencies_path = os.path.join(self.project, "canonical", "dependencies.yaml")
        dependencies = load_yaml(dependencies_path)
        dependency = dependencies["dependencies"][0]
        dependency["predecessor_ref"]["id"] = "task.op.1234"
        dependency["successor_ref"]["id"] = "task.offline.repair-pump"
        dump_yaml(dependencies_path, dependencies)

        overlay_output = os.path.join(self.temp.name, "overlay-output")
        overlay_target = os.path.join(self.temp.name, "overlay-published")
        env = self.environment(
            REPLAN_OVERLAY_OUTPUT_DIR=overlay_output,
            REPLAN_OVERLAY_PUBLISH_TARGET=overlay_target,
        )
        proc, out = self.run_helper(apply=True, env=env)
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(OpenProjectHandler.status_name, "Closed")
        self.assertEqual((OpenProjectHandler.start_date, OpenProjectHandler.due_date), ("2026-10-26", "2026-10-26"))
        task = next(item for item in load_yaml(os.path.join(self.project, "canonical", "tasks.yaml"))["tasks"] if item["id"] == "task.op.1234")
        self.assertEqual(task["tracking_status"], "accepted")
        self.assertEqual((task["planned_start"], task["planned_finish"]), ("2026-10-26", "2026-10-26"))
        self.assertEqual(next(item for item in out["actions"] if item["step"] == 6)["state"], "applied")
        self.assertEqual(next(item for item in out["actions"] if item["step"] == 7)["state"], "applied")
        demands = [item for item in load_yaml(os.path.join(self.project, "canonical", "resource_demands.yaml"))["resource_demands"] if item["task_ref"]["id"] == "task.op.1234"]
        self.assertTrue(all(item["status"] == "consumed" and item["remaining_amount"] == 0 for item in demands))
        evidence = sum((load_yaml(os.path.join(self.project, "evidence", name)).get("evidence") or [] for name in os.listdir(os.path.join(self.project, "evidence")) if name.endswith(".yaml")), [])
        self.assertIn("evidence.op.accept-1234", [item["id"] for item in evidence])
        audit_text = "".join(read_text(os.path.join(self.project, "audit", name)) for name in os.listdir(os.path.join(self.project, "audit")))
        self.assertIn("evidence_add", audit_text)
        self.assertIn("actual_set_remaining", audit_text)
        self.assertIn("actual_close_demand", audit_text)
        self.assertIn("actual_add_status", audit_text)
        self.assertTrue(out["new_proposal_ids"], out)
        for proposal_id in out["new_proposal_ids"]:
            self.assertTrue(any(proposal_id in text and "proposal accept" in text and "--apply" in text for text in out["operator_next_steps"]))
        self.assertTrue(os.path.isfile(os.path.join(overlay_output, ".accept-task-publish.json")))
        self.assertTrue(os.path.isfile(os.path.join(overlay_target, "demo", "raw", "openproject_overlay.json")))

        second_proc, second = self.run_helper(apply=True, env=env)
        self.assertEqual(second_proc.returncode, 0, second)
        self.assertTrue(second["no_op"], second)
        self.assertEqual(second["counts"]["applied"], 0)
        self.assertEqual(next(item for item in second["actions"] if item["step"] == 6)["state"], "already_done")
        self.assertEqual(next(item for item in second["actions"] if item["step"] == 7)["state"], "already_done")

        # A date push that failed after RePlan recorded the window is retried.
        OpenProjectHandler.start_date = ""
        OpenProjectHandler.due_date = ""
        third_proc, third = self.run_helper(apply=True, env=env)
        self.assertEqual(third_proc.returncode, 0, third)
        self.assertEqual(next(item for item in third["actions"] if item["step"] == 6)["state"], "already_done")
        self.assertEqual(next(item for item in third["actions"] if item["step"] == 7)["state"], "applied")
        self.assertEqual((OpenProjectHandler.start_date, OpenProjectHandler.due_date), ("2026-10-26", "2026-10-26"))

    def test_partial_apply_resumes_from_project_state(self):
        proxy = os.path.join(self.temp.name, "replan-proxy.py")
        sentinel = os.path.join(self.temp.name, "failed-once")
        with open(proxy, "w", encoding="utf-8") as stream:
            stream.write("""#!/usr/bin/env python3
import json, os, sys
args = sys.argv[1:]
if '--apply' in args and args[:2] == ['actual', 'set-remaining'] and args[args.index('--project') + 1] == os.environ['REPLAN_PROJECT'] and not os.path.exists(os.environ['FAIL_SENTINEL']):
    open(os.environ['FAIL_SENTINEL'], 'w').close()
    print(json.dumps({'ok': False, 'command': 'actual set-remaining', 'errors': [{'code': 'injected_failure', 'message': 'injected middle failure'}]}))
    raise SystemExit(2)
os.execv(os.environ['REAL_REPLAN'], [os.environ['REAL_REPLAN'], *args])
""")
        os.chmod(proxy, os.stat(proxy).st_mode | stat.S_IXUSR)
        env = self.environment(REPLAN_BIN=proxy, REAL_REPLAN=os.environ["REPLAN_BIN"], FAIL_SENTINEL=sentinel)
        proc, out = self.run_helper(apply=True, env=env)
        self.assertEqual(proc.returncode, 7, out)
        self.assertTrue(out["applied_so_far"], out)
        self.assertEqual(out["failed_step"]["step"], 3)

        proc, resumed = self.run_helper(apply=True, env=env)
        self.assertEqual(proc.returncode, 0, resumed)
        task = next(item for item in load_yaml(os.path.join(self.project, "canonical", "tasks.yaml"))["tasks"] if item["id"] == "task.op.1234")
        self.assertEqual(task["tracking_status"], "accepted")

    def test_nonzero_ci_minutes_uses_the_tasks_single_ci_demand(self):
        demands_path = os.path.join(self.project, "canonical", "resource_demands.yaml")
        demands = load_yaml(demands_path)
        ci_demand = next(item for item in demands["resource_demands"] if item["id"] == "resource_demand.task-op-1234.openai-quota")
        ci_demand["unit"] = "ci_minutes"
        dump_yaml(demands_path, demands)
        capacities_path = os.path.join(self.project, "canonical", "capacities.yaml")
        capacities = load_yaml(capacities_path)
        next(item for item in capacities["capacities"] if item["resource_ref"]["id"] == "resource.service.openai.tokens")["unit"] = "ci_minutes"
        dump_yaml(capacities_path, capacities)

        env = self.environment(REPLAN_ACCEPT_CI_MINUTES="12")
        proc, out = self.run_helper(apply=True, env=env)
        self.assertEqual(proc.returncode, 0, out)
        ci_step = next(item for item in out["actions"] if item["step"] == 2)
        self.assertEqual(ci_step["state"], "applied")
        actuals = sum((load_yaml(os.path.join(self.project, "actuals", name)).get("actuals") or [] for name in os.listdir(os.path.join(self.project, "actuals")) if name.endswith(".yaml")), [])
        recorded = [item for item in actuals if item.get("demand_ref", {}).get("id") == ci_demand["id"]]
        self.assertEqual([(item["amount"], item["unit"]) for item in recorded], [(12, "ci_minutes")])

    def test_refusals_happen_before_writes(self):
        evidence_dir = os.path.join(self.project, "evidence")
        before = sorted(os.listdir(evidence_dir))
        cases = (
            ({"REPLAN_TASK": "task.missing"}, 4, "task_not_found"),
            ({"REPLAN_ACCEPTED_BY": "actor.missing"}, 2, "actor_not_found"),
            ({"REPLAN_ACTOR": "actor.missing"}, 2, "actor_not_found"),
        )
        for changes, exit_code, error_code in cases:
            with self.subTest(error_code=error_code, changes=changes):
                env = self.environment(**changes)
                proc, out = self.run_helper(apply=True, env=env)
                self.assertEqual(proc.returncode, exit_code, out)
                self.assertEqual(out["error_codes"], [error_code])
                self.assertEqual(sorted(os.listdir(evidence_dir)), before)

        tasks_path = os.path.join(self.project, "canonical", "tasks.yaml")
        tasks = load_yaml(tasks_path)
        next(task for task in tasks["tasks"] if task["id"] == "task.op.1234")["tracking_status"] = "in_progress"
        dump_yaml(tasks_path, tasks)
        proc, out = self.run_helper(apply=True)
        self.assertEqual(proc.returncode, 11, out)
        self.assertEqual(out["error_codes"], ["task_not_review_ready"])
        self.assertEqual(out["task_status"], "in_progress")
        self.assertEqual(sorted(os.listdir(evidence_dir)), before)

    def test_missing_required_setting_and_apply_token_are_usage_errors(self):
        for setting, apply, code in (
            ("REPLAN_ACCEPT_SUMMARY", False, "missing_setting"),
            ("REPLAN_TEST_OPENPROJECT_TOKEN", True, "token_env_not_set"),
        ):
            with self.subTest(setting=setting):
                env = self.environment()
                env.pop(setting)
                proc, out = self.run_helper(apply=apply, env=env)
                self.assertEqual(proc.returncode, 8, out)
                self.assertEqual(out["error_codes"], [code])

    def test_missing_token_only_skips_openproject_in_dry_run(self):
        env = self.environment()
        env.pop("REPLAN_TEST_OPENPROJECT_TOKEN")
        proc, out = self.run_helper(env=env)
        self.assertEqual(proc.returncode, 0, out)
        date_step = next(item for item in out["actions"] if item["step"] == 7)
        self.assertEqual(date_step["name"], "update_openproject_dates")
        self.assertEqual(date_step["state"], "skipped")
        openproject = next(item for item in out["actions"] if item["step"] == 9)
        self.assertEqual(openproject["state"], "skipped")
        self.assertIn("token_env_not_set", out["warning_codes"])

    def test_unknown_actual_window_is_skipped_without_failing_acceptance(self):
        tasks_path = os.path.join(self.project, "canonical", "tasks.yaml")
        tasks = load_yaml(tasks_path)
        next(task for task in tasks["tasks"] if task["id"] == "task.op.1234")["tracking_status"] = "accepted"
        dump_yaml(tasks_path, tasks)
        evidence_path = os.path.join(self.project, "evidence", "evidence.accept-existing.yaml")
        dump_yaml(evidence_path, {"evidence": [{
            "id": "evidence.op.accept-1234", "type": "decision", "subtype": "operator_acceptance",
            "source": "chat", "observed_at": "2026-10-26T12:00:00+07:00", "confidence": "observed",
            "summary": "Already accepted.", "targets": [{"kind": "task", "id": "task.op.1234"}],
        }]})

        proc, out = self.run_helper()
        self.assertEqual(proc.returncode, 0, out)
        actual_window = next(item for item in out["actions"] if item["step"] == 6)
        self.assertEqual(actual_window["state"], "skipped")
        self.assertIn("actual_window_unknown", out["warning_codes"])
        date_step = next(item for item in out["actions"] if item["step"] == 7)
        self.assertEqual(date_step["state"], "skipped")


if __name__ == "__main__":
    unittest.main()
