"""The shared helper envelope (helper_envelope.py, finding C-08).

The wrapper tests run every helper from a copy of scripts/ that has no
.env.local and no private/ folder, so they never reach a live OpenProject;
the unreachable server is 127.0.0.1:9. They need REPLAN_BIN, which the smoke
passes.
"""

import http.server
import json
import os
import shutil
import subprocess
import sys
import tempfile
import threading
import unittest

import yaml

import helper_envelope

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")
PROJECT = os.path.join(ROOT, "examples", "replan-self")

# Every wrapper an agent runs, with the arguments it needs to get going.
WRAPPERS = {
    "accept-openproject-soft-drift": [],
    "apply-openproject-bootstrap": [],
    "apply-openproject-relations": [],
    "check-openproject-drift": [],
    "check-openproject-live-alignment": [],
    "check-resource-schedule-drift": [],
    "explain-resource-overload": [],
    "openproject-operator-report": [],
    "openproject-wiki": ["page", "list"],
    "openproject-work-package-toolkit": ["work-package", "list"],
    "plan-openproject-bootstrap": [],
    "plan-resource-demand-calibration": [],
    "push-openproject-date-drift": [],
    "push-openproject-status-drift": [],
    "review-resource-pool": [],
    "validate-openproject": [],
    "validate-openproject-live": [],
    "run-openproject-sidecar-pipeline": [],
}

# Helpers that talk to OpenProject, with the exit code of an unreachable
# server: 6, an external source error, unless the helper stops at an earlier
# check of its own.
NETWORK = {
    "accept-openproject-soft-drift": 6,
    "apply-openproject-bootstrap": 6,
    "apply-openproject-relations": 4,  # stops at the missing private bindings
    "check-openproject-drift": 6,
    "check-openproject-live-alignment": 6,
    "openproject-operator-report": 6,
    "openproject-wiki": 6,
    "openproject-work-package-toolkit": 6,
    "push-openproject-date-drift": 6,
    "push-openproject-status-drift": 6,
    "validate-openproject-live": 6,
}

# Helpers that need the address and the project id together.
SETTINGS = {
    "accept-openproject-soft-drift": "missing_setting",
    "apply-openproject-bootstrap": "missing_base_url",
    "apply-openproject-relations": "missing_base_url",
    "check-openproject-drift": "missing_setting",
    "check-openproject-live-alignment": "missing_setting",
    "openproject-operator-report": "missing_openproject_target",
    "push-openproject-date-drift": "missing_setting",
    "push-openproject-status-drift": "missing_base_url",
    "validate-openproject": "missing_setting",
    "validate-openproject-live": "missing_setting",
    "run-openproject-sidecar-pipeline": "missing_setting",
}

TABLE = set(helper_envelope.EXIT.values()) - {0}


class NormalizeTest(unittest.TestCase):
    def test_old_error_string_becomes_an_entry(self):
        out = helper_envelope.normalize({"ok": True, "command": "c", "mode": "apply", "error": "boom", "message": "it broke"})
        self.assertFalse(out["ok"])
        self.assertNotIn("error", out)
        self.assertNotIn("message", out)
        self.assertEqual(out["errors"], [{"code": "boom", "message": "it broke", "severity": "error"}])
        self.assertEqual(out["error_codes"], ["boom"])

    def test_codes_only_and_strings_become_entries(self):
        out = helper_envelope.normalize({"ok": True, "command": "c", "warning_codes": ["w"], "errors": ["e"]}, "dry_run")
        self.assertEqual(out["mode"], "dry_run")
        self.assertEqual([w["code"] for w in out["warnings"]], ["w"])
        self.assertEqual(out["errors"][0], {"code": "e", "message": "e", "severity": "error"})
        self.assertFalse(out["ok"])

    def test_keeps_extra_keys_and_an_ok_envelope(self):
        out = helper_envelope.normalize({"ok": True, "command": "c", "mode": "read_only", "counts": {"a": 1}, "drift_status": "aligned"})
        self.assertTrue(out["ok"])
        self.assertEqual((out["counts"], out["drift_status"], out["errors"], out["warning_codes"]), ({"a": 1}, "aligned", [], []))

    def test_unknown_mode_falls_back(self):
        self.assertEqual(helper_envelope.normalize({"ok": True, "command": "c", "mode": "apply_failed"})["mode"], "read_only")

    def test_openproject_failure_is_external(self):
        exc = RuntimeError(json.dumps({
            "code": "openproject_network_error", "message": "refused", "method": "POST", "path": "/api/v3",
            "retried": False, "retry_skipped": "non_idempotent_method", "outcome_unknown": True,
            "earlier_attempt_may_have_applied": True,
        }))
        entry, code = helper_envelope.error_from_exception(exc, "x_failed")
        self.assertEqual((entry["code"], entry["path"], code), ("openproject_network_error", "/api/v3", 6))
        self.assertEqual(
            (entry["retried"], entry["retry_skipped"], entry["outcome_unknown"]),
            (False, "non_idempotent_method", True),
        )
        self.assertTrue(entry["earlier_attempt_may_have_applied"])
        entry, code = helper_envelope.error_from_exception(ValueError("bad"), "x_failed")
        self.assertEqual((entry["code"], entry["message"], code), ("x_failed", "bad", 1))
        self.assertEqual(helper_envelope.error_from_exception(FileNotFoundError("gone"), "x_failed")[1], 4)

    def test_exit_code_follows_ok(self):
        self.assertEqual(helper_envelope.exit_code_for(helper_envelope.normalize({"ok": True, "command": "c"})), 0)
        self.assertEqual(helper_envelope.exit_code_for(helper_envelope.normalize({"ok": False, "command": "c"})), 11)
        self.assertEqual(helper_envelope.apply_exit_code({"ok": False, "command": "c", "mode": "apply"}, 2), 7)
        self.assertIsNone(helper_envelope.apply_exit_code({"ok": False, "command": "c", "mode": "apply"}, 0))
        self.assertIsNone(helper_envelope.apply_exit_code({"ok": False, "command": "c", "mode": "dry_run"}, 2))

    def test_partial_apply_reports_confirmed_and_unknown_step(self):
        partial = helper_envelope.PartialApply()
        partial.attempting({"action": "patch", "work_package_id": "1"})
        partial.applied({"action": "patched", "work_package_id": "1"})
        partial.attempting({"action": "patch", "work_package_id": "2"})
        partial.writing()
        exc = RuntimeError(json.dumps({
            "code": "openproject_request_timeout", "message": "slow", "method": "PATCH",
            "path": "/api/v3/work_packages/2", "outcome_unknown": True,
        }))

        entry, code, extra = partial.failure(exc, "patch_failed")

        self.assertEqual((entry["code"], code), ("openproject_request_timeout", 7))
        self.assertEqual(extra["applied_so_far"], [{"action": "patched", "work_package_id": "1"}])
        self.assertEqual(extra["failed_step"], {
            "action": "patch", "work_package_id": "2", "may_have_been_applied": True,
        })

    def test_partial_apply_does_not_mark_failed_read_as_possibly_applied(self):
        partial = helper_envelope.PartialApply()
        partial.attempting({"action": "patch", "work_package_id": "1"})
        partial.writing()
        partial.applied({"action": "patched", "work_package_id": "1"})
        partial.attempting({"action": "patch", "work_package_id": "2"})
        exc = RuntimeError(json.dumps({
            "code": "openproject_http_error", "message": "read failed", "method": "GET",
            "path": "/api/v3/work_packages/2", "status": 500,
        }))

        _, code, extra = partial.failure(exc, "patch_failed")

        self.assertEqual(code, 7)
        self.assertEqual(extra["failed_step"]["work_package_id"], "2")
        self.assertNotIn("may_have_been_applied", extra["failed_step"])

    def test_guard_turns_a_traceback_into_the_envelope(self):
        for raised, want_code, want_exit in (
            ("ValueError('bad input')", "helper_failed", 10),
            ("RuntimeError(json.dumps({'code': 'openproject_request_timeout', 'message': 'slow'}))", "openproject_request_timeout", 6),
        ):
            script = f"import json, helper_envelope\nhelper_envelope.guard('probe', 'dry_run')\nraise {raised}\n"
            proc = subprocess.run([sys.executable, "-c", script], capture_output=True, text=True, env=dict(os.environ, PYTHONPATH=LIB))
            out = json.loads(proc.stdout)
            self.assertEqual((proc.returncode, out["ok"], out["command"], out["mode"], out["error_codes"]), (want_exit, False, "probe", "dry_run", [want_code]))
            self.assertNotIn("Traceback", proc.stderr)

    def test_guard_adds_the_unknown_patch_next_step(self):
        payload = {
            "code": "openproject_request_timeout", "message": "slow", "method": "PATCH",
            "path": "/api/v3/work_packages/1", "retried": False, "outcome_unknown": True,
        }
        script = (
            "import json, helper_envelope\n"
            "helper_envelope.guard('probe', 'apply', unknown_patch_next_step='Inspect before retrying.')\n"
            f"raise RuntimeError({json.dumps(json.dumps(payload))})\n"
        )
        proc = subprocess.run(
            [sys.executable, "-c", script], capture_output=True, text=True,
            env=dict(os.environ, PYTHONPATH=LIB),
        )
        out = json.loads(proc.stdout)
        self.assertEqual(proc.returncode, helper_envelope.EXIT["external"])
        self.assertEqual(out["operator_next_steps"], ["Inspect before retrying."])

    def test_guard_keeps_applied_actions_without_a_current_failed_step(self):
        payload = {
            "code": "openproject_http_error", "message": "read failed", "method": "GET",
            "path": "/api/v3/work_packages/2", "status": 500,
        }
        script = (
            "import json, helper_envelope\n"
            "partial = helper_envelope.PartialApply()\n"
            "partial.attempting({'action': 'patch', 'work_package_id': '1'})\n"
            "partial.writing()\n"
            "partial.applied({'action': 'patched', 'work_package_id': '1'})\n"
            "helper_envelope.guard('probe', 'apply', partial_apply=partial)\n"
            f"raise RuntimeError({json.dumps(json.dumps(payload))})\n"
        )
        proc = subprocess.run(
            [sys.executable, "-c", script], capture_output=True, text=True,
            env=dict(os.environ, PYTHONPATH=LIB),
        )
        out = json.loads(proc.stdout)
        self.assertEqual(proc.returncode, helper_envelope.EXIT["partial"])
        self.assertEqual(out["applied_so_far"], [{"action": "patched", "work_package_id": "1"}])
        self.assertNotIn("failed_step", out)


class ExitCodeFromOkTest(unittest.TestCase):
    """A helper that ran to the end exits from its ok: 0, 11 when it needs
    attention, 7 for a partial apply (finding C-02)."""

    def project(self, tmp, tasks):
        os.makedirs(os.path.join(tmp, "canonical"))
        with open(os.path.join(tmp, "canonical", "tasks.yaml"), "w") as fh:
            yaml.safe_dump({"tasks": tasks}, fh)
        with open(os.path.join(tmp, "canonical", "dependencies.yaml"), "w") as fh:
            yaml.safe_dump({"dependencies": []}, fh)
        with open(os.path.join(tmp, "canonical", "current_plan.yaml"), "w") as fh:
            yaml.safe_dump({"current_plan": {"included_tasks": [{"kind": "task", "id": t["id"]} for t in tasks]}}, fh)
        bindings = os.path.join(tmp, "bindings.yaml")
        with open(bindings, "w") as fh:
            yaml.safe_dump({"openproject_private_bindings": {"tasks": [
                {"replan_task_id": t["id"], "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": str(i + 1)}}
                for i, t in enumerate(tasks)]}}, fh)
        return bindings

    def run_module(self, module, *args, env=None):
        proc = subprocess.run([sys.executable, "-m", module, *args], capture_output=True, text=True,
                              env=dict(os.environ, PYTHONPATH=LIB, **(env or {})), timeout=60)
        return proc.returncode, json.loads(proc.stdout)

    def test_unknown_internal_option_is_a_json_failure(self):
        code, out = self.run_module("plan_openproject_bootstrap", "--not-a-wrapper-option")
        self.assertEqual(code, 8, out)
        self.assertEqual(out["error_codes"], ["usage_error"])
        self.assertEqual(out["mode"], "invalid")

    def test_date_push_wrapper_passes_named_runtime_artifacts(self):
        task = {
            "id": "task.a", "title": "A", "tracking_status": "in_progress",
            "planned_start": "2026-10-21", "planned_finish": "2026-10-22",
        }
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, [task])
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {"task_candidates": [{
                    "source_id": "1", "title": "A", "source_status": "In progress",
                    "mapped_status": "in_progress", "start_date": "2026-10-20",
                    "due_date": "2026-10-20",
                }], "dependency_candidates": []}}, fh)
            proc = subprocess.run(
                [os.path.join(ROOT, "scripts", "push-openproject-date-drift.sh")],
                capture_output=True, text=True, timeout=30,
                env={
                    "PATH": os.environ["PATH"], "TMPDIR": tmp,
                    "REPLAN_PROJECT": tmp, "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_IMPORT_JSON": imp,
                    "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_BASE_URL": "", "REPLAN_APPLY": "",
                },
            )
            out = json.loads(proc.stdout)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["counts"]["planned_actions"], 1)
            self.assertEqual(out["planned_actions"][0]["work_package_id"], "1")

    def test_bootstrap_plan_wrapper_reads_settings_itself(self):
        task = {"id": "task.a", "title": "A", "tracking_status": "not_started"}
        with tempfile.TemporaryDirectory() as tmp:
            self.project(tmp, [task])
            with open(os.path.join(tmp, "canonical", "resource_demands.yaml"), "w") as fh:
                yaml.safe_dump({"resource_demands": []}, fh)
            proc = subprocess.run(
                [os.path.join(ROOT, "scripts", "plan-openproject-bootstrap.sh")],
                capture_output=True, text=True, timeout=30,
                env={
                    "PATH": os.environ["PATH"], "TMPDIR": tmp,
                    "REPLAN_PROJECT": tmp,
                    # The new name too, equal, so a repository .env.local
                    # cannot supply a different one; alias-only is a unit test.
                    "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_TARGET_PROJECT_NAME": "Demo",
                },
            )
            out = json.loads(proc.stdout)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["target_openproject_project"], {"identifier": "demo", "name": "Demo"})
            self.assertEqual(out["work_packages"][0]["replan_task_id"], "task.a")
            self.assertEqual(out["warning_codes"], ["deprecated_setting"])

    def test_bootstrap_plan_wrapper_refuses_conflicting_project_ids(self):
        with tempfile.TemporaryDirectory() as tmp:
            proc = subprocess.run(
                [os.path.join(ROOT, "scripts", "plan-openproject-bootstrap.sh")],
                capture_output=True, text=True, timeout=30,
                env={
                    "PATH": os.environ["PATH"], "TMPDIR": tmp,
                    "REPLAN_PROJECT": tmp,
                    "REPLAN_OPENPROJECT_PROJECT_ID": "new-project",
                    "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "old-project",
                },
            )
            out = json.loads(proc.stdout)
            self.assertEqual(proc.returncode, 8, out)
            self.assertEqual(out["error_codes"], ["conflicting_settings"])
            self.assertIn("REPLAN_OPENPROJECT_PROJECT_ID", out["errors"][0]["message"])
            self.assertIn("REPLAN_OPENPROJECT_TARGET_PROJECT_ID", out["errors"][0]["message"])

    def test_drift_check(self):
        task = {"id": "task.a", "title": "A", "tracking_status": "not_started", "planned_start": "2026-10-20", "planned_finish": "2026-10-20"}
        live = {"source_id": "1", "title": "A", "mapped_status": "not_started", "start_date": "2026-10-20", "due_date": "2026-10-20"}
        for name, change, want_status, want_exit in (
            ("aligned", {}, "aligned", 0),
            ("soft drift", {"due_date": "2026-10-21"}, "soft_drift", 0),
            ("hard drift", {"title": "Renamed in OpenProject"}, "drift", 11),
        ):
            with self.subTest(name), tempfile.TemporaryDirectory() as tmp:
                bindings = self.project(tmp, [task])
                imp = os.path.join(tmp, "import.json")
                with open(imp, "w") as fh:
                    json.dump({"ok": True, "result": {"task_candidates": [{**live, **change}], "dependency_candidates": []}}, fh)
                code, out = self.run_module(
                    "check_openproject_drift", "--import-json", imp, "--tmp-dir", tmp, "--root-dir", ROOT,
                    env={
                        "REPLAN_PROJECT": tmp,
                        "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                        "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    },
                )
                self.assertEqual((out["drift_status"], code), (want_status, want_exit))

    def test_drift_check_matches_reverse_relates(self):
        tasks = [
            {"id": "task.a", "title": "A", "tracking_status": "not_started"},
            {"id": "task.b", "title": "B", "tracking_status": "not_started"},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, tasks)
            with open(os.path.join(tmp, "canonical", "dependencies.yaml"), "w") as fh:
                yaml.safe_dump({"dependencies": [{
                    "id": "dep.related", "type": "relates",
                    "predecessor_ref": {"kind": "task", "id": "task.a"},
                    "successor_ref": {"kind": "task", "id": "task.b"},
                    "status": "open", "criticality": "optional",
                }]}, fh)
            with open(bindings) as fh:
                bindings_doc = yaml.safe_load(fh)
            bindings_doc["openproject_private_bindings"]["dependencies"] = [{
                "replan_dependency_id": "dep.related",
                "source_ref": {"source": "openproject", "object_type": "relation", "object_id": "9"},
                "relation_type": "relates",
            }]
            with open(bindings, "w") as fh:
                yaml.safe_dump(bindings_doc, fh)
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {
                    "task_candidates": [
                        {"source_id": "1", "title": "A", "mapped_status": "not_started"},
                        {"source_id": "2", "title": "B", "mapped_status": "not_started"},
                    ],
                    "dependency_candidates": [{
                        "source_id": "9", "relation_type": "relates", "dependency_type": "relates",
                        "predecessor_task_id": "task.b", "successor_task_id": "task.a",
                    }],
                }}, fh)
            code, out = self.run_module(
                "check_openproject_drift", "--import-json", imp, "--tmp-dir", tmp, "--root-dir", ROOT,
                env={
                    "REPLAN_PROJECT": tmp,
                    "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                },
            )
        self.assertEqual((code, out["drift_status"]), (0, "aligned"), out)
        self.assertEqual(out["counts"]["missing_live_dependencies"], 0)
        self.assertEqual(out["counts"]["extra_live_dependencies"], 0)

    def test_drift_check_warns_for_date_drift_and_automatic_scheduling(self):
        task = {
            "id": "task.a", "title": "A", "tracking_status": "not_started",
            "planned_start": "2026-10-20", "planned_finish": "2026-10-20",
        }
        live = {
            "source_id": "1", "title": "A", "mapped_status": "not_started",
            "start_date": "2026-10-20", "due_date": "2026-10-21",
        }

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                body = json.dumps({
                    "total": 1, "pageSize": 500, "offset": 1,
                    "_embedded": {"elements": [{"id": 1, "scheduleManually": False}]},
                }).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, [task])
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {
                    "task_candidates": [live], "dependency_candidates": [],
                }}, fh)
            code, out = self.run_module(
                "check_openproject_drift", "--import-json", imp,
                "--tmp-dir", tmp, "--root-dir", ROOT,
                env={
                    "REPLAN_PROJECT": tmp,
                    "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_BASE_URL": f"http://127.0.0.1:{server.server_address[1]}",
                    "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN",
                    "TEST_TOKEN": "t",
                },
            )

        self.assertEqual(code, 0, out)
        self.assertTrue(out["ok"])
        self.assertEqual(out["drift_status"], "soft_drift")
        self.assertEqual(out["counts"]["date_drifts"], 1)
        self.assertEqual(out["warning_codes"], [
            "openproject_automatic_scheduling", "openproject_date_drift",
        ])
        self.assertIn("task.a", out["warnings"][0]["message"] + out["warnings"][1]["message"])
        self.assertTrue(any("push-openproject-date-drift.sh" in item for item in out["suggested_next_actions"]))
        self.assertTrue(any("openproject-work-package-toolkit.sh work-package update-dates" in item for item in out["suggested_next_actions"]))

    def test_drift_check_keeps_status_when_scheduling_read_fails(self):
        task = {
            "id": "task.a", "title": "A", "tracking_status": "not_started",
            "planned_start": "2026-10-20", "planned_finish": "2026-10-20",
        }
        live = {
            "source_id": "1", "title": "A", "mapped_status": "not_started",
            "start_date": "2026-10-20", "due_date": "2026-10-20",
        }

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                body = json.dumps({"message": "stand-in failure"}).encode()
                self.send_response(500)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        base_url = f"http://127.0.0.1:{server.server_address[1]}"
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, [task])
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {
                    "task_candidates": [live], "dependency_candidates": [],
                }}, fh)
            code, out = self.run_module(
                "check_openproject_drift", "--import-json", imp,
                "--tmp-dir", tmp, "--root-dir", ROOT,
                env={
                    "REPLAN_PROJECT": tmp,
                    "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_BASE_URL": base_url,
                    "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN",
                    "TEST_TOKEN": "secret-token",
                },
            )

        self.assertEqual(code, 0, out)
        self.assertTrue(out["ok"])
        self.assertEqual(out["drift_status"], "aligned")
        self.assertEqual(out["warning_codes"], ["openproject_scheduling_unknown"])
        message = out["warnings"][0]["message"]
        self.assertIn("HTTP 500", message)
        self.assertNotIn(base_url, message)
        self.assertNotIn("secret-token", message)

    def test_status_push_skipped_and_partial(self):
        tasks = [
            {"id": "task.a", "title": "A", "tracking_status": "review_ready"},
            {"id": "task.b", "title": "B", "tracking_status": "accepted"},
        ]
        statuses = {"New": "/api/v3/statuses/1", "Developed": "/api/v3/statuses/2", "Closed": "/api/v3/statuses/3"}

        class OpenProject(http.server.BaseHTTPRequestHandler):
            # Work package 1 may move to Developed; work package 2 may not
            # move to Closed.
            allowed = {"1": [statuses["New"], statuses["Developed"]], "2": [statuses["New"]]}

            def reply(self, body):
                data = json.dumps(body).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def body(self):
                length = int(self.headers.get("Content-Length") or 0)
                return self.rfile.read(length)

            def do_GET(self):
                if self.path == "/api/v3/statuses":
                    self.reply({"_embedded": {"elements": [{"name": n, "_links": {"self": {"href": h}}} for n, h in statuses.items()]}})
                else:
                    self.reply({"id": self.path.rsplit("/", 1)[-1], "lockVersion": 3})

            def do_POST(self):
                self.body()
                wp = self.path.split("/")[-2]
                self.reply({"_embedded": {"schema": {"status": {"_links": {"allowedValues": [{"href": h} for h in self.allowed[wp]]}}}}})

            def do_PATCH(self):
                self.body()
                self.reply({"lockVersion": 4, "_links": {"status": {"title": "Developed"}}})

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        base = f"http://127.0.0.1:{server.server_address[1]}"
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, tasks)
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {"task_candidates": [
                    {"source_id": "1", "source_status": "New"}, {"source_id": "2", "source_status": "New"}]}}, fh)
            for apply, want_exit, want_applied in (("", 11, 0), ("1", 7, 1)):
                with self.subTest(apply=apply):
                    code, out = self.run_module(
                        "push_openproject_status_drift", "--import-json", imp, "--tmp-dir", tmp, "--root-dir", ROOT,
                        env={
                            "REPLAN_PROJECT": tmp,
                            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                            "REPLAN_OPENPROJECT_BASE_URL": base,
                            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                            "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN",
                            "REPLAN_OPENPROJECT_STATUS_SCOPE": "all",
                            "REPLAN_APPLY": apply,
                            "TEST_TOKEN": "t",
                        },
                    )
                    self.assertEqual(code, want_exit, out)
                    self.assertFalse(out["ok"])
                    self.assertEqual([s["reason"] for s in out["skipped"]], ["openproject_workflow_transition_not_allowed"])
                    self.assertEqual(sum(1 for a in out["actions"] if a.get("updated_lock_version") is not None), want_applied)

    def test_status_push_reports_writes_before_second_patch_failure(self):
        tasks = [
            {"id": f"task.{name}", "title": name.upper(), "tracking_status": "review_ready"}
            for name in ("a", "b", "c")
        ]
        statuses = {"New": "/api/v3/statuses/1", "Developed": "/api/v3/statuses/2"}

        for fail_on, want_exit in ((2, 7), (1, 6)):
            with self.subTest(fail_on=fail_on), tempfile.TemporaryDirectory() as tmp:
                patch_ids = []

                class OpenProject(http.server.BaseHTTPRequestHandler):
                    def reply(self, body, status=200):
                        data = json.dumps(body).encode()
                        self.send_response(status)
                        self.send_header("Content-Type", "application/hal+json")
                        self.send_header("Content-Length", str(len(data)))
                        self.end_headers()
                        self.wfile.write(data)

                    def consume(self):
                        self.rfile.read(int(self.headers.get("Content-Length") or 0))

                    def do_GET(self):
                        if self.path == "/api/v3/statuses":
                            self.reply({"_embedded": {"elements": [
                                {"name": name, "_links": {"self": {"href": href}}}
                                for name, href in statuses.items()
                            ]}})
                        else:
                            self.reply({"id": self.path.rsplit("/", 1)[-1], "lockVersion": 3})

                    def do_POST(self):
                        self.consume()
                        self.reply({"_embedded": {"schema": {"status": {"_links": {"allowedValues": [
                            {"href": statuses["New"]}, {"href": statuses["Developed"]},
                        ]}}}}})

                    def do_PATCH(self):
                        self.consume()
                        patch_ids.append(self.path.rsplit("/", 1)[-1])
                        if len(patch_ids) == fail_on:
                            self.reply({"message": "stand-in failure"}, 500)
                        else:
                            self.reply({"lockVersion": 4, "_links": {"status": {"title": "Developed"}}})

                    def log_message(self, *args):
                        pass

                server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
                threading.Thread(target=server.serve_forever, daemon=True).start()
                self.addCleanup(server.server_close)
                self.addCleanup(server.shutdown)
                bindings = self.project(tmp, tasks)
                imp = os.path.join(tmp, "import.json")
                with open(imp, "w") as fh:
                    json.dump({"ok": True, "result": {"task_candidates": [
                        {"source_id": str(index), "source_status": "New"}
                        for index in range(1, 4)
                    ]}}, fh)
                base = f"http://127.0.0.1:{server.server_address[1]}"

                code, out = self.run_module(
                    "push_openproject_status_drift", "--import-json", imp, "--tmp-dir", tmp, "--root-dir", ROOT,
                    env={
                        "REPLAN_PROJECT": tmp, "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                        "REPLAN_OPENPROJECT_BASE_URL": base, "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                        "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN", "REPLAN_OPENPROJECT_STATUS_SCOPE": "all",
                        "REPLAN_APPLY": "1", "TEST_TOKEN": "t",
                    },
                )

                self.assertEqual((code, patch_ids), (want_exit, [str(i) for i in range(1, fail_on + 1)]), out)
                self.assertEqual(out["failed_step"]["work_package_id"], str(fail_on))
                if fail_on == 2:
                    self.assertEqual(out["applied_so_far"][0]["work_package_id"], "1")
                else:
                    self.assertNotIn("applied_so_far", out)
                server.shutdown()

    def test_status_push_reports_first_patch_when_second_form_read_fails(self):
        tasks = [
            {"id": f"task.{name}", "title": name.upper(), "tracking_status": "review_ready"}
            for name in ("a", "b", "c")
        ]
        patch_ids = []
        form_ids = []

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def consume(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))

            def do_GET(self):
                if self.path == "/api/v3/statuses":
                    self.reply({"_embedded": {"elements": [
                        {"name": "New", "_links": {"self": {"href": "/api/v3/statuses/1"}}},
                        {"name": "Developed", "_links": {"self": {"href": "/api/v3/statuses/2"}}},
                    ]}})
                else:
                    self.reply({"id": self.path.rsplit("/", 1)[-1], "lockVersion": 3})

            def do_POST(self):
                self.consume()
                wp_id = self.path.split("/")[-2]
                form_ids.append(wp_id)
                if wp_id == "2":
                    self.reply({"message": "stand-in form failure"}, 500)
                else:
                    self.reply({"_embedded": {"schema": {"status": {"_links": {"allowedValues": [
                        {"href": "/api/v3/statuses/1"}, {"href": "/api/v3/statuses/2"},
                    ]}}}}})

            def do_PATCH(self):
                self.consume()
                patch_ids.append(self.path.rsplit("/", 1)[-1])
                self.reply({"lockVersion": 4, "_links": {"status": {"title": "Developed"}}})

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, tasks)
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {"task_candidates": [
                    {"source_id": str(index), "source_status": "New"}
                    for index in range(1, 4)
                ]}}, fh)
            base = f"http://127.0.0.1:{server.server_address[1]}"

            code, out = self.run_module(
                "push_openproject_status_drift", "--import-json", imp, "--tmp-dir", tmp, "--root-dir", ROOT,
                env={
                    "REPLAN_PROJECT": tmp, "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_BASE_URL": base, "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN", "REPLAN_OPENPROJECT_STATUS_SCOPE": "all",
                    "REPLAN_APPLY": "1", "TEST_TOKEN": "t",
                },
            )

            self.assertEqual((code, patch_ids, form_ids), (7, ["1"], ["1", "2"]), out)
            self.assertEqual(out["applied_so_far"][0]["work_package_id"], "1")
            self.assertEqual(out["failed_step"]["work_package_id"], "2")
            self.assertNotIn("may_have_been_applied", out["failed_step"])
        server.shutdown()

    def test_date_push_reports_writes_before_second_patch_failure(self):
        tasks = [
            {"id": f"task.{name}", "title": name.upper(), "tracking_status": "in_progress",
             "planned_start": "2026-10-21", "planned_finish": "2026-10-22"}
            for name in ("a", "b", "c")
        ]
        patch_ids = []

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                self.reply({"id": self.path.rsplit("/", 1)[-1], "lockVersion": 3,
                            "startDate": "2026-10-20", "dueDate": "2026-10-20"})

            def do_PATCH(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                patch_ids.append(self.path.rsplit("/", 1)[-1])
                if len(patch_ids) == 2:
                    self.reply({"message": "stand-in failure"}, 500)
                else:
                    self.reply({"lockVersion": 4, "startDate": "2026-10-21", "dueDate": "2026-10-22",
                                "scheduleManually": True})

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, tasks)
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {"task_candidates": [
                    {"source_id": str(index), "title": task["title"], "source_status": "In progress",
                     "mapped_status": "in_progress", "start_date": "2026-10-20", "due_date": "2026-10-20"}
                    for index, task in enumerate(tasks, 1)
                ], "dependency_candidates": []}}, fh)
            base = f"http://127.0.0.1:{server.server_address[1]}"

            code, out = self.run_module(
                "push_openproject_date_drift", "--import-json", imp, "--tmp-dir", tmp, "--root-dir", ROOT,
                env={
                    "REPLAN_PROJECT": tmp, "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_BASE_URL": base, "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN", "REPLAN_APPLY": "1", "TEST_TOKEN": "t",
                },
            )

            self.assertEqual((code, patch_ids), (7, ["1", "2"]), out)
            self.assertEqual(out["applied_so_far"][0]["work_package_id"], "1")
            self.assertEqual(out["failed_step"]["work_package_id"], "2")
        server.shutdown()

    def test_date_push_reports_first_patch_when_second_apply_read_fails(self):
        tasks = [
            {"id": f"task.{name}", "title": name.upper(), "tracking_status": "in_progress",
             "planned_start": "2026-10-21", "planned_finish": "2026-10-22"}
            for name in ("a", "b", "c")
        ]
        get_counts = {}
        patch_ids = []

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                wp_id = self.path.rsplit("/", 1)[-1]
                get_counts[wp_id] = get_counts.get(wp_id, 0) + 1
                if wp_id == "2" and get_counts[wp_id] == 2:
                    self.reply({"message": "stand-in read failure"}, 500)
                else:
                    self.reply({"id": wp_id, "lockVersion": 3,
                                "startDate": "2026-10-20", "dueDate": "2026-10-20"})

            def do_PATCH(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                patch_ids.append(self.path.rsplit("/", 1)[-1])
                self.reply({"lockVersion": 4, "startDate": "2026-10-21", "dueDate": "2026-10-22",
                            "scheduleManually": True})

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as tmp:
            bindings = self.project(tmp, tasks)
            imp = os.path.join(tmp, "import.json")
            with open(imp, "w") as fh:
                json.dump({"ok": True, "result": {"task_candidates": [
                    {"source_id": str(index), "title": task["title"], "source_status": "In progress",
                     "mapped_status": "in_progress", "start_date": "2026-10-20", "due_date": "2026-10-20"}
                    for index, task in enumerate(tasks, 1)
                ], "dependency_candidates": []}}, fh)
            base = f"http://127.0.0.1:{server.server_address[1]}"

            code, out = self.run_module(
                "push_openproject_date_drift", "--import-json", imp, "--tmp-dir", tmp, "--root-dir", ROOT,
                env={
                    "REPLAN_PROJECT": tmp, "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_BASE_URL": base, "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN", "REPLAN_APPLY": "1", "TEST_TOKEN": "t",
                },
            )

            self.assertEqual((code, patch_ids), (7, ["1"]), out)
            self.assertEqual(out["applied_so_far"][0]["work_package_id"], "1")
            self.assertEqual(out["failed_step"]["work_package_id"], "2")
            self.assertNotIn("may_have_been_applied", out["failed_step"])
        server.shutdown()

    def test_relation_apply_writes_first_binding_before_second_post_failure(self):
        post_paths = []

        class OpenProject(http.server.BaseHTTPRequestHandler):
            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                self.reply({"total": 0, "pageSize": 500, "offset": 1, "_embedded": {"elements": []}})

            def do_POST(self):
                self.rfile.read(int(self.headers.get("Content-Length") or 0))
                post_paths.append(self.path)
                if len(post_paths) == 2:
                    self.reply({"message": "stand-in failure"}, 500)
                else:
                    self.reply({"id": 700 + len(post_paths), "type": "blocks", "_links": {}}, 201)

            def log_message(self, *args):
                pass

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), OpenProject)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as tmp:
            os.makedirs(os.path.join(tmp, "canonical"))
            dependencies = [
                {"id": f"dep.{i}", "type": "blocks", "predecessor_ref": {"id": f"task.{i}a"},
                 "successor_ref": {"id": f"task.{i}b"}}
                for i in range(1, 4)
            ]
            with open(os.path.join(tmp, "canonical", "dependencies.yaml"), "w") as fh:
                yaml.safe_dump({"dependencies": dependencies}, fh)
            bindings = os.path.join(tmp, "bindings.yaml")
            task_bindings = []
            for i in range(1, 4):
                task_bindings.extend([
                    {"replan_task_id": f"task.{i}a", "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": str(i * 10 + 1)}},
                    {"replan_task_id": f"task.{i}b", "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": str(i * 10 + 2)}},
                ])
            with open(bindings, "w") as fh:
                yaml.safe_dump({"openproject_private_bindings": {"tasks": task_bindings, "dependencies": [], "resources": []}}, fh)
            base = f"http://127.0.0.1:{server.server_address[1]}"

            code, out = self.run_module(
                "apply_openproject_relations", "--root-dir", ROOT,
                env={
                    "REPLAN_PROJECT": tmp, "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
                    "REPLAN_OPENPROJECT_BASE_URL": base, "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                    "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN", "REPLAN_APPLY": "1", "TEST_TOKEN": "t",
                },
            )

            self.assertEqual((code, len(post_paths)), (7, 2), out)
            self.assertEqual(out["applied_so_far"][0]["replan_dependency_id"], "dep.1")
            self.assertEqual(out["failed_step"]["replan_dependency_id"], "dep.2")
            self.assertTrue(out["private_binding_layer"]["written"])
            with open(bindings) as fh:
                saved = yaml.safe_load(fh)["openproject_private_bindings"]["dependencies"]
            self.assertEqual([item["replan_dependency_id"] for item in saved], ["dep.1"])
        server.shutdown()


@unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs REPLAN_BIN; the smoke passes it")
class WrapperFailurePathsTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tmp = tempfile.mkdtemp(prefix="replan-envelope-")
        shutil.copytree(os.path.join(ROOT, "scripts"), os.path.join(cls.tmp, "scripts"),
                        ignore=shutil.ignore_patterns("__pycache__", "tests"))

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.tmp, ignore_errors=True)

    def run_wrapper(self, name, **env):
        base = {
            "PATH": os.environ["PATH"],
            "HOME": self.tmp,
            "REPLAN_BIN": os.environ["REPLAN_BIN"],
            "REPLAN_PROJECT": PROJECT,
            "REPLAN_OPENPROJECT_TIMEOUT_SECONDS": "2",
            "REPLAN_OPENPROJECT_RETRIES": "0",
            "TMPDIR": self.tmp,
        }
        base.update(env)
        script = os.path.join(self.tmp, "scripts", name + ".sh")
        return subprocess.run([script, *WRAPPERS[name]], capture_output=True, text=True, env=base, timeout=120)

    def assert_failure(self, name, proc, code, exit_code):
        lines = [line for line in proc.stdout.splitlines() if line.strip()]
        try:
            out = json.loads(proc.stdout)
        except ValueError:
            self.fail(f"{name}: stdout is not one JSON object: {proc.stdout[:300]!r} stderr {proc.stderr[-300:]!r}")
        self.assertTrue(lines, name)
        self.assertIs(out.get("ok"), False, name)
        self.assertTrue(out.get("command"), name)
        self.assertIn(out.get("mode"), helper_envelope.MODES, name)
        self.assertNotIn("error", out, name)
        self.assertTrue(out["errors"], name)
        for entry in out["errors"]:
            self.assertTrue(entry.get("code") and entry.get("message"), (name, entry))
        self.assertEqual(out["error_codes"], sorted({e["code"] for e in out["errors"]}), name)
        if code:
            self.assertIn(code, out["error_codes"], (name, out["errors"]))
        self.assertIn(proc.returncode, TABLE, name)
        self.assertEqual(proc.returncode, exit_code, (name, out["errors"]))

    def test_unsupported_format(self):
        for name in WRAPPERS:
            with self.subTest(name):
                self.assert_failure(name, self.run_wrapper(name, REPLAN_FORMAT="yaml"), "unsupported_format", 8)

    def test_address_and_project_id_belong_together(self):
        for name, code in SETTINGS.items():
            with self.subTest(name):
                proc = self.run_wrapper(name, REPLAN_OPENPROJECT_BASE_URL="", REPLAN_OPENPROJECT_PROJECT_ID="replan-self",
                                        OPENPROJECT_TOKEN="test-token")
                self.assert_failure(name, proc, code, 8)

    def test_unreachable_openproject(self):
        for name, exit_code in NETWORK.items():
            with self.subTest(name):
                proc = self.run_wrapper(name, REPLAN_OPENPROJECT_BASE_URL="http://127.0.0.1:9", REPLAN_OPENPROJECT_PROJECT_ID="replan-self",
                                        OPENPROJECT_TOKEN="test-token")
                self.assert_failure(name, proc, "", exit_code)

    def test_no_default_project(self):
        # Finding C-05: no helper falls back to this repository's own plan or
        # OpenProject project; a missing setting is named instead.
        needs = {
            "explain-resource-overload": ["REPLAN_RESOURCE_ID", "REPLAN_UNIT"],
            "plan-openproject-bootstrap": ["REPLAN_OPENPROJECT_PROJECT_ID"],
            "apply-openproject-bootstrap": ["REPLAN_OPENPROJECT_PROJECT_ID"],
        }
        with_project_id = {"accept-openproject-soft-drift", "apply-openproject-relations", "check-openproject-drift",
                           "openproject-operator-report", "push-openproject-date-drift", "push-openproject-status-drift",
                           "openproject-work-package-toolkit"}
        for name in WRAPPERS:
            if name in ("openproject-wiki", "validate-openproject", "run-openproject-sidecar-pipeline"):
                continue  # the wiki needs no plan; the two pilot validators check the shipped sample by default
            with self.subTest(name):
                proc = self.run_wrapper(name, REPLAN_PROJECT="")
                self.assert_failure(name, proc, "missing_setting", 8)
                want = {"REPLAN_PROJECT", *needs.get(name, [])}
                if name == "openproject-work-package-toolkit":
                    want.remove("REPLAN_PROJECT")
                if name in with_project_id:
                    want.add("REPLAN_OPENPROJECT_PROJECT_ID")
                self.assertEqual(set(json.loads(proc.stdout)["missing_settings"]), want, name)

    def test_missing_token(self):
        # Finding C-01: a helper that would write fails under REPLAN_APPLY=1;
        # in a dry run, and for a helper that only reads, it is skipped with
        # token_env_not_set among its warnings.
        writers = ["accept-openproject-soft-drift", "apply-openproject-bootstrap", "apply-openproject-relations",
                   "push-openproject-date-drift", "push-openproject-status-drift"]
        readers = ["check-openproject-drift", "check-openproject-live-alignment", "openproject-operator-report",
                   "validate-openproject-live", "run-openproject-sidecar-pipeline"]
        settings = {"REPLAN_OPENPROJECT_BASE_URL": "http://127.0.0.1:9", "REPLAN_OPENPROJECT_PROJECT_ID": "replan-self"}
        for name in writers + readers:
            for apply in ("", "1"):
                with self.subTest(name, apply=apply):
                    proc = self.run_wrapper(name, REPLAN_APPLY=apply, **settings)
                    if apply and name in writers:
                        self.assert_failure(name, proc, "token_env_not_set", 8)
                        self.assertEqual(json.loads(proc.stdout)["mode"], "apply")
                        continue
                    out = json.loads(proc.stdout)
                    self.assertEqual((proc.returncode, out["ok"], out["mode"]), (0, True, "skipped"), out)
                    self.assertIn("token_env_not_set", out["warning_codes"])

    def test_toolkit_writes_without_a_token(self):
        for args in (["work-package", "create", "--subject", "Probe"],
                     ["work-package", "update-status", "--work-package", "1", "--status", "New"],
                     ["comment", "add", "--work-package", "1", "--text", "Probe"]):
            with self.subTest(args[:2]):
                script = os.path.join(self.tmp, "scripts", "openproject-work-package-toolkit.sh")
                env = {"PATH": os.environ["PATH"], "HOME": self.tmp, "REPLAN_BIN": os.environ["REPLAN_BIN"], "REPLAN_PROJECT": PROJECT,
                       "REPLAN_OPENPROJECT_BASE_URL": "http://127.0.0.1:9", "REPLAN_OPENPROJECT_PROJECT_ID": "replan-self"}
                proc = subprocess.run([script, *args], capture_output=True, text=True, timeout=60, env={**env, "REPLAN_APPLY": "1"})
                self.assert_failure("toolkit " + args[1], proc, "token_env_not_set", 8)
                proc = subprocess.run([script, *args], capture_output=True, text=True, timeout=60, env=env)
                out = json.loads(proc.stdout)
                self.assertEqual((proc.returncode, out["mode"], out["warning_codes"]), (0, "skipped", ["token_env_not_set"]), out)

    def test_overlay_publish_without_a_built_overlay(self):
        empty = os.path.join(self.tmp, "no-overlay")
        os.makedirs(empty, exist_ok=True)
        script = os.path.join(self.tmp, "scripts", "publish-openproject-overlay.sh")
        proc = subprocess.run([script], capture_output=True, text=True, timeout=60,
                              env={"PATH": os.environ["PATH"], "HOME": self.tmp, "REPLAN_OVERLAY_OUTPUT_DIR": empty,
                                   "REPLAN_OPENPROJECT_PROJECT_ID": "replan-self"})
        self.assert_failure("publish-openproject-overlay", proc, "overlay_not_built", 4)

    def test_overlay_publish_without_rsync_is_a_helper_error(self):
        output = os.path.join(self.tmp, "built-overlay")
        os.makedirs(os.path.join(output, "raw"), exist_ok=True)
        with open(os.path.join(output, "raw", "openproject_overlay.json"), "w", encoding="utf-8") as stream:
            json.dump({"openproject_project_id": "replan-self", "work_packages": []}, stream)
        path = os.path.join(self.tmp, "path-without-rsync")
        os.makedirs(path, exist_ok=True)
        for name in ("sh", "dirname", "sed", "printenv", "python3"):
            os.symlink(shutil.which(name), os.path.join(path, name))
        script = os.path.join(self.tmp, "scripts", "publish-openproject-overlay.sh")
        proc = subprocess.run(
            [script], capture_output=True, text=True, timeout=60,
            env={
                "PATH": path,
                "HOME": self.tmp,
                "REPLAN_OVERLAY_OUTPUT_DIR": output,
                "REPLAN_OVERLAY_PUBLISH_TARGET": os.path.join(self.tmp, "target"),
                "REPLAN_OPENPROJECT_PROJECT_ID": "replan-self",
            },
        )
        self.assertEqual(proc.stderr, "", proc.stderr)
        self.assert_failure("publish-openproject-overlay", proc, "rsync_not_found", 4)

    def test_overlay_publish_refuses_scenario(self):
        output = os.path.join(self.tmp, "scenario-overlay")
        os.makedirs(os.path.join(output, "raw"), exist_ok=True)
        with open(os.path.join(output, "raw", "openproject_overlay.json"), "w", encoding="utf-8") as stream:
            json.dump({"openproject_project_id": "replan-self", "scenario_id": "scenario.what-if", "work_packages": []}, stream)
        script = os.path.join(self.tmp, "scripts", "publish-openproject-overlay.sh")
        proc = subprocess.run(
            [script], capture_output=True, text=True, timeout=60,
            env={"PATH": os.environ["PATH"], "HOME": self.tmp, "REPLAN_OVERLAY_OUTPUT_DIR": output,
                 "REPLAN_OVERLAY_PUBLISH_TARGET": os.path.join(self.tmp, "target"),
                 "REPLAN_OPENPROJECT_PROJECT_ID": "replan-self"},
        )
        self.assertEqual(proc.stderr, "", proc.stderr)
        self.assert_failure("publish-openproject-overlay", proc, "overlay_is_scenario", 2)
        self.assertIn("a what-if overlay is never published as the plan", json.loads(proc.stdout)["errors"][0]["message"])


if __name__ == "__main__":
    unittest.main()
