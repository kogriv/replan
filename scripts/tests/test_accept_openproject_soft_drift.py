"""Audited writes for the OpenProject soft-date-drift wrapper."""

import glob
import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


def write_json(path, value):
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(value, handle)


def read_text(path):
    with open(path, encoding="utf-8") as handle:
        return handle.read()


def write_bindings(path, task_ids):
    with open(path, "w", encoding="utf-8") as handle:
        yaml.safe_dump({"openproject_private_bindings": {"tasks": [
            {"replan_task_id": task_id, "source_ref": {
                "source": "openproject", "object_type": "work_package", "object_id": str(index + 1),
            }} for index, task_id in enumerate(task_ids)
        ]}}, handle)


def import_document(candidates):
    return {"ok": True, "result": {"task_candidates": candidates}, "warnings": [], "errors": []}


class SoftDriftAcceptTest(unittest.TestCase):
    def run_module(self, project, bindings, import_path, binary, *, apply="", actor=""):
        env = {
            **os.environ,
            "PYTHONPATH": LIB,
            "REPLAN_PROJECT": project,
            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
            "REPLAN_APPLY": apply,
            "REPLAN_ACTOR": actor,
        }
        return subprocess.run([
            sys.executable, "-m", "accept_openproject_soft_drift",
            "--import-json", import_path, "--tmp-dir", os.path.dirname(import_path),
            "--root-dir", ROOT, "--replan-bin", binary,
        ], capture_output=True, text=True, env=env, timeout=60)

    def test_dry_run_keeps_shape_and_shows_actor(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "small-team")
            shutil.copytree(os.path.join(ROOT, "examples", "small-team"), project)
            bindings = os.path.join(tmp, "bindings.yaml")
            write_bindings(bindings, ["task.op.1234"])
            imported = os.path.join(tmp, "import.json")
            write_json(imported, import_document([{
                "source_id": "1", "title": "A", "mapped_status": "in_progress",
                "source_status": "In progress", "start_date": "2026-07-08", "due_date": "2026-07-12",
            }]))
            proc = self.run_module(project, bindings, imported, "/not/used", actor="actor.human.operator")
            out = json.loads(proc.stdout)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["actor"], "actor.human.operator")
            self.assertEqual(out["counts"], {"date_changes": 2, "applied_date_changes": 0, "status_reviews": 1, "unmatched_candidates": 0})
            self.assertEqual(set(("date_changes", "status_reviews", "unmatched_candidates", "policy")) - set(out), set())
            self.assertIn("replan task set-dates", out["policy"]["rule"])

    @unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs the built RePlan binary")
    def test_apply_calls_cli_and_writes_evidence_and_audits(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "small-team")
            shutil.copytree(os.path.join(ROOT, "examples", "small-team"), project)
            bindings = os.path.join(tmp, "bindings.yaml")
            write_bindings(bindings, ["task.op.1234"])
            imported = os.path.join(tmp, "import.json")
            write_json(imported, import_document([{
                "source_id": "1", "title": "A", "mapped_status": "ready",
                "source_status": "To be scheduled", "start_date": "2026-07-08", "due_date": "2026-07-12",
            }]))
            proc = self.run_module(project, bindings, imported, os.environ["REPLAN_BIN"], apply="1", actor="actor.human.operator")
            out = json.loads(proc.stdout)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual((out["ok"], out["counts"]["applied_date_changes"]), (True, 2))
            self.assertEqual(out["evidence_item"]["type"], "import_result")
            self.assertEqual(out["evidence_item"]["source"], "openproject")
            self.assertEqual(out["evidence_item"]["targets"], ["task:task.op.1234"])
            with open(os.path.join(project, "canonical", "tasks.yaml"), encoding="utf-8") as handle:
                task = next(item for item in yaml.safe_load(handle)["tasks"] if item["id"] == "task.op.1234")
            self.assertEqual((task["planned_start"], task["planned_finish"]), ("2026-07-08", "2026-07-12"))
            evidence_text = "".join(read_text(path) for path in glob.glob(os.path.join(project, "evidence", "*.yaml")))
            self.assertIn(out["evidence_item"]["id"], evidence_text)
            audit_text = "".join(read_text(path) for path in glob.glob(os.path.join(project, "audit", "*.jsonl")))
            self.assertIn('"operation":"evidence_add"', audit_text)
            self.assertIn('"operation":"task_set_dates"', audit_text)

    @unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs the built RePlan binary")
    def test_apply_without_actor_is_refused_before_writes(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "small-team")
            shutil.copytree(os.path.join(ROOT, "examples", "small-team"), project)
            bindings = os.path.join(tmp, "bindings.yaml")
            write_bindings(bindings, ["task.op.1234"])
            imported = os.path.join(tmp, "import.json")
            write_json(imported, import_document([{"source_id": "1", "title": "A", "start_date": "2026-07-08"}]))
            tasks_path = os.path.join(project, "canonical", "tasks.yaml")
            with open(tasks_path, "rb") as handle:
                before = handle.read()
            proc = self.run_module(project, bindings, imported, os.environ["REPLAN_BIN"], apply="1")
            out = json.loads(proc.stdout)
            self.assertEqual((proc.returncode, out["ok"], out["error_codes"]), (8, False, ["missing_setting"]))
            with open(tasks_path, "rb") as handle:
                self.assertEqual(handle.read(), before)
            self.assertNotIn("evidence_item", out)

    def test_partial_failure_reports_applied_so_far_and_exits_seven(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "project")
            os.makedirs(os.path.join(project, "canonical"))
            with open(os.path.join(project, "canonical", "tasks.yaml"), "w", encoding="utf-8") as handle:
                yaml.safe_dump({"tasks": [
                    {"id": "task.a", "planned_start": "2026-10-20"},
                    {"id": "task.b", "planned_start": "2026-10-20"},
                ]}, handle)
            bindings = os.path.join(tmp, "bindings.yaml")
            write_bindings(bindings, ["task.a", "task.b"])
            imported = os.path.join(tmp, "import.json")
            write_json(imported, import_document([
                {"source_id": "1", "title": "A", "start_date": "2026-10-21"},
                {"source_id": "2", "title": "B", "start_date": "2026-10-21"},
            ]))
            binary = os.path.join(tmp, "replan")
            with open(binary, "w", encoding="utf-8") as handle:
                handle.write("#!/usr/bin/env python3\nimport json, sys\n"
                             "args = sys.argv[1:]\n"
                             "if args[:2] == ['task', 'set-dates'] and args[args.index('--task') + 1] == 'task.b':\n"
                             " print(json.dumps({'ok': False, 'errors': [{'code': 'forced_failure'}]})); sys.exit(2)\n"
                             "print(json.dumps({'ok': True, 'result': {'applied': True}}))\n")
            os.chmod(binary, os.stat(binary).st_mode | stat.S_IEXEC)
            proc = self.run_module(project, bindings, imported, binary, apply="1", actor="actor.human.operator")
            out = json.loads(proc.stdout)
            self.assertEqual(proc.returncode, 7, out)
            self.assertEqual([item["action"] for item in out["applied_so_far"]], ["evidence_add", "task_set_dates"])
            self.assertEqual(out["applied_so_far"][1]["task_id"], "task.a")
            self.assertEqual(out["failed_step"]["task_id"], "task.b")


if __name__ == "__main__":
    unittest.main()
