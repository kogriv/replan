import hashlib
import http.server
import json
import os
import glob
import shutil
import stat
import subprocess
import tempfile
import threading
import unittest
import urllib.parse

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SCRIPT = os.path.join(ROOT, "scripts", "backfill-openproject-status-history.sh")


class StatusHistoryBackfillTest(unittest.TestCase):
    def setUp(self):
        self.activities = {}
        self.requests = []
        self.statuses = [
            {"id": 1, "name": "New"},
            {"id": 2, "name": "In progress"},
            {"id": 3, "name": "Developed"},
            {"id": 4, "name": "Closed"},
            {"id": 5, "name": "Feedback"},
        ]
        owner = self

        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                parsed = urllib.parse.urlsplit(self.path)
                query = urllib.parse.parse_qs(parsed.query)
                owner.requests.append((parsed.path, query))
                if parsed.path == "/api/v3/statuses":
                    elements = owner.statuses
                else:
                    match = parsed.path.split("/")
                    if len(match) == 6 and match[1:4] == ["api", "v3", "work_packages"] and match[5] == "activities":
                        elements = owner.activities.get(match[4], [])
                    else:
                        self.send_error(404)
                        return
                page_size = int(query.get("pageSize", [100])[0])
                offset = int(query.get("offset", [1])[0])
                start = (offset - 1) * page_size
                batch = elements[start:start + page_size]
                body = json.dumps({
                    "total": len(elements), "count": len(batch),
                    "pageSize": page_size, "offset": offset,
                    "_embedded": {"elements": batch},
                }).encode()
                self.send_response(200)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(body)))
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, *_args):
                pass

        self.server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.addCleanup(self.server.server_close)
        self.addCleanup(self.server.shutdown)

    def activity(self, journal_id, at, old, new):
        return {
            "id": journal_id,
            "createdAt": at,
            "details": [{
                "format": "custom",
                "raw": f"Status changed from {old} to {new}",
                "html": "<strong>localized text must not be parsed</strong>",
            }],
        }

    def project(self, tmp):
        project = os.path.join(tmp, "small-team")
        shutil.copytree(os.path.join(ROOT, "examples", "small-team"), project)
        bindings = os.path.join(tmp, "bindings.yaml")
        with open(bindings, "w", encoding="utf-8") as stream:
            yaml.safe_dump({"openproject_private_bindings": {
                "tasks": [{"replan_task_id": "task.op.1234", "source_ref": {
                    "source": "openproject", "object_type": "work_package", "object_id": "7",
                }}],
                "dependencies": [], "resources": [],
            }}, stream)
        return project, bindings

    def env(self, tmp, project, bindings, *, apply="", binary=""):
        binary = binary or os.environ.get("REPLAN_BIN", "")
        env_file = os.path.join(tmp, "empty.env")
        open(env_file, "w", encoding="utf-8").close()
        return {
            "PATH": os.environ.get("PATH", ""), "LANG": "C.UTF-8",
            "REPLAN_ENV_FILE": env_file,
            "REPLAN_PROJECT": project,
            "REPLAN_OPENPROJECT_BASE_URL": f"http://127.0.0.1:{self.server.server_port}",
            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
            "REPLAN_OPENPROJECT_TARGET_PROJECT_ID": "",
            "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_TOKEN", "TEST_TOKEN": "token",
            "REPLAN_OPENPROJECT_TOKEN_FILE": "",
            "REPLAN_OPENPROJECT_PERIMETER_KEY": "",
            "REPLAN_OPENPROJECT_PERIMETER_KEY_FILE": "",
            "REPLAN_OPENPROJECT_TIMEOUT_SECONDS": "5",
            "REPLAN_OPENPROJECT_RETRIES": "0",
            "REPLAN_OPENPROJECT_RETRY_DELAY_SECONDS": "0.01",
            "REPLAN_ACTOR": "actor.human.operator",
            "REPLAN_APPLY": apply, "REPLAN_FORMAT": "json",
            "REPLAN_BIN": binary, "TMPDIR": tmp,
        }

    def run_helper(self, env, *args):
        proc = subprocess.run([SCRIPT, *args], capture_output=True, text=True, env=env, timeout=30)
        return proc, json.loads(proc.stdout)

    def tree_digest(self, root):
        digest = hashlib.sha256()
        for directory, dirs, files in os.walk(root):
            dirs.sort()
            for name in sorted(files):
                path = os.path.join(directory, name)
                digest.update(os.path.relpath(path, root).encode())
                with open(path, "rb") as stream:
                    digest.update(stream.read())
        return digest.hexdigest()

    def test_two_changes_are_proposed_and_dry_run_writes_nothing(self):
        self.activities["7"] = [
            self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress"),
            self.activity(11, "2026-01-03T10:00:00Z", "In progress", "Closed"),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            before = self.tree_digest(project)
            proc, out = self.run_helper(self.env(tmp, project, bindings), "--task", "task.op.1234")
            after = self.tree_digest(project)
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"], {
            "tasks_read": 1, "status_changes_found": 2, "already_present": 0,
            "proposed": 2, "applied": 0, "unmapped": 0, "conflicts": 0,
        })
        self.assertEqual([item["tracking_status"] for item in out["proposals"]], ["in_progress", "accepted"])
        self.assertIn("replan evidence add", out["proposals"][0]["commands"][0])
        self.assertIn("replan actual add-status", out["proposals"][0]["commands"][1])
        self.assertTrue(out["actual_window_previews"][0]["result"]["ok"], out)
        self.assertEqual(before, after)

    def test_already_present_unmapped_and_conflict_are_reported(self):
        self.activities["7"] = [
            self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress"),
            self.activity(11, "2026-01-03T09:00:00Z", "In progress", "Feedback"),
            self.activity(12, "2026-01-04T09:00:00Z", "Feedback", "Closed"),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            audit = os.path.join(project, "audit", "audit.2026-01.jsonl")
            with open(audit, "w", encoding="utf-8") as stream:
                stream.write(json.dumps({
                    "id": "audit.present", "occurred_at": "2026-01-02T09:00:30Z",
                    "operation": "actual_add_status", "target_ref": {"kind": "task", "id": "task.op.1234"},
                    "new_status": "in_progress",
                }) + "\n")
                stream.write(json.dumps({
                    "id": "audit.later", "occurred_at": "2026-01-05T09:00:00Z",
                    "operation": "actual_add_status", "target_ref": {"kind": "task", "id": "task.op.1234"},
                    "new_status": "in_progress",
                }) + "\n")
            proc, out = self.run_helper(self.env(tmp, project, bindings))
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual((out["counts"]["already_present"], out["counts"]["unmapped"], out["counts"]["conflicts"]), (1, 1, 1))
        self.assertEqual(out["already_present"][0]["match"], "same_time")
        self.assertEqual(out["conflicts"][0]["later_audit_id"], "audit.later")
        self.assertEqual(out["proposals"], [])

    def test_status_already_in_effect_skips_journal_update_27_minutes_later(self):
        self.activities["7"] = [
            self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress"),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            audit = os.path.join(project, "audit", "audit.2026-01.jsonl")
            with open(audit, "w", encoding="utf-8") as stream:
                stream.write(json.dumps({
                    "id": "audit.real-start", "occurred_at": "2026-01-02T08:33:00Z",
                    "operation": "actual_add_status", "target_ref": {"kind": "task", "id": "task.op.1234"},
                    "new_status": "in_progress",
                }) + "\n")
            proc, out = self.run_helper(self.env(tmp, project, bindings))
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual((out["counts"]["already_present"], out["counts"]["proposed"]), (1, 0))
        self.assertEqual(out["already_present"][0]["match"], "in_effect")
        self.assertEqual(out["already_present"][0]["audit_id"], "audit.real-start")

    def test_activity_pagination(self):
        fillers = [{"id": index, "createdAt": "2026-01-01T00:00:00Z", "details": []} for index in range(100)]
        self.activities["7"] = fillers + [self.activity(101, "2026-01-02T09:00:00Z", "New", "In progress")]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            proc, out = self.run_helper(self.env(tmp, project, bindings))
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"]["proposed"], 1)
        activity_offsets = [query["offset"][0] for path, query in self.requests if path.endswith("/activities")]
        self.assertEqual(activity_offsets, ["1", "2"])

    @unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs the built RePlan binary")
    def test_apply_uses_built_binary_and_previews_actual_window(self):
        self.activities["7"] = [
            self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress"),
            self.activity(11, "2026-01-03T10:00:00Z", "In progress", "Closed"),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            proc, out = self.run_helper(self.env(tmp, project, bindings, apply="1", binary=os.environ["REPLAN_BIN"]))
            audit_text = ""
            for path in glob.glob(os.path.join(project, "audit", "*.jsonl")):
                with open(path, encoding="utf-8") as stream:
                    audit_text += stream.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual((out["counts"]["proposed"], out["counts"]["applied"]), (2, 2))
        self.assertIn('"operation":"evidence_add"', audit_text)
        self.assertIn('"operation":"actual_add_status"', audit_text)
        self.assertTrue(out["actual_window_previews"][0]["result"]["ok"], out)
        result = out["actual_window_previews"][0]["result"]["result"]
        self.assertEqual((result["new_planned_start"], result["new_planned_finish"]), ("2026-01-02", "2026-01-03"))

    @unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs the built RePlan binary")
    def test_accepted_record_with_ai_actor_is_refused_before_any_write(self):
        self.activities["7"] = [
            self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress"),
            self.activity(11, "2026-01-03T10:00:00Z", "In progress", "Closed"),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            before = self.tree(project)
            env = self.env(tmp, project, bindings, binary=os.environ["REPLAN_BIN"])
            env["REPLAN_ACTOR"] = "actor.ai.codex.local"
            dry_proc, dry = self.run_helper(env)
            env["REPLAN_APPLY"] = "1"
            proc, out = self.run_helper(env)
            after = self.tree(project)
        self.assertEqual(dry_proc.returncode, 0, dry)
        self.assertIn("status_record_refused", dry["warning_codes"])
        self.assertEqual([(item["tracking_status"], item["codes"]) for item in dry["status_refusals"]], [("accepted", ["approver_not_human"])])
        self.assertEqual(proc.returncode, 8, out)
        self.assertEqual(out["error_codes"], ["human_actor_required"])
        self.assertEqual(out["counts"]["applied"], 0)
        self.assertEqual(before, after)

    @unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs the built RePlan binary")
    def test_rerun_after_partial_apply_reuses_written_evidence(self):
        self.activities["7"] = [self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress")]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            # The evidence record an earlier run wrote before it stopped.
            subprocess.run([
                os.environ["REPLAN_BIN"], "evidence", "add", "--project", project,
                "--id", "evidence.openproject.wp-7.journal-10", "--type", "import", "--source", "openproject",
                "--actor", "actor.human.operator", "--observed-at", "2026-01-02T09:00:00Z",
                "--summary", "Imported earlier.", "--target", "task:task.op.1234", "--apply", "--format", "json",
            ], check=True, capture_output=True)
            proc, out = self.run_helper(self.env(tmp, project, bindings, apply="1", binary=os.environ["REPLAN_BIN"]))
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["counts"]["applied"], 1)

    def tree(self, root):
        result = {}
        for path in sorted(glob.glob(os.path.join(root, "**", "*"), recursive=True)):
            if os.path.isfile(path):
                with open(path, "rb") as stream:
                    result[os.path.relpath(path, root)] = stream.read()
        return result

    @unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs the built RePlan binary")
    def test_dry_preview_backfills_earlier_start_without_reopening_accepted_task(self):
        self.activities["7"] = [
            self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress"),
        ]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            tasks_path = os.path.join(project, "canonical", "tasks.yaml")
            with open(tasks_path, encoding="utf-8") as stream:
                tasks = yaml.safe_load(stream)
            task = next(item for item in tasks["tasks"] if item["id"] == "task.op.1234")
            task["tracking_status"] = "accepted"
            with open(tasks_path, "w", encoding="utf-8") as stream:
                yaml.safe_dump(tasks, stream, sort_keys=False)
            with open(os.path.join(project, "audit", "audit.2026-01.jsonl"), "w", encoding="utf-8") as stream:
                stream.write(json.dumps({
                    "id": "audit.finish", "occurred_at": "2026-01-03T10:00:00Z",
                    "actor_ref": {"kind": "actor", "id": "actor.human.operator"},
                    "operation": "actual_add_status", "target_ref": {"kind": "task", "id": "task.op.1234"},
                    "new_status": "accepted", "status_date": "2026-01-03", "result": "success",
                    "reason": "Accepted before historical start backfill.",
                }) + "\n")
            proc, out = self.run_helper(self.env(tmp, project, bindings, binary=os.environ["REPLAN_BIN"]))
            with open(tasks_path, encoding="utf-8") as stream:
                unchanged = next(item for item in yaml.safe_load(stream)["tasks"] if item["id"] == "task.op.1234")
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual((out["counts"]["proposed"], out["counts"]["applied"]), (1, 0))
        self.assertEqual(unchanged["tracking_status"], "accepted")
        preview = out["actual_window_previews"][0]
        self.assertEqual(preview.get("result_source"), "temporary_project_after_proposals", preview)
        self.assertTrue(preview["result"]["ok"], preview)
        result = preview["result"]["result"]
        self.assertEqual((result["new_planned_start"], result["new_planned_finish"]), ("2026-01-02", "2026-01-03"))

    def test_partial_failure_exits_seven_and_lists_completed_evidence(self):
        self.activities["7"] = [self.activity(10, "2026-01-02T09:00:00Z", "New", "In progress")]
        with tempfile.TemporaryDirectory() as tmp:
            project, bindings = self.project(tmp)
            binary = os.path.join(tmp, "replan")
            with open(binary, "w", encoding="utf-8") as stream:
                stream.write("#!/usr/bin/env python3\nimport json,sys\n"
                             "if sys.argv[1:3] == ['actual','add-status'] and '--apply' in sys.argv:\n"
                             " print(json.dumps({'ok':False,'errors':[{'code':'forced'}]})); sys.exit(2)\n"
                             "print(json.dumps({'ok':True,'result':{}}))\n")
            os.chmod(binary, os.stat(binary).st_mode | stat.S_IEXEC)
            proc, out = self.run_helper(self.env(tmp, project, bindings, apply="1", binary=binary))
        self.assertEqual(proc.returncode, 7, out)
        self.assertEqual(out["applied_so_far"][0]["action"], "evidence_add")
        self.assertEqual(out["failed_step"]["action"], "actual_add_status")


if __name__ == "__main__":
    unittest.main()
