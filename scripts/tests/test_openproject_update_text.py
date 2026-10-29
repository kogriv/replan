import http.server
import json
import os
import shutil
import stat
import subprocess
import sys
import tempfile
import threading
import unittest

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class ToolkitUpdateTextTest(unittest.TestCase):
    def run_toolkit(self, args, *, project, fixture="", bindings="", base_url="https://op.example.internal", apply="", actor="", binary="default", root_dir=ROOT, extra_env=None):
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            REPLAN_PROJECT=project,
            REPLAN_OPENPROJECT_BASE_URL=base_url,
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
            TEST_TOKEN="test-token",
            REPLAN_OPENPROJECT_FROM_FILE=fixture,
            REPLAN_OPENPROJECT_BINDINGS_PATH=bindings or os.path.join(project, "bindings.yaml"),
            REPLAN_APPLY=apply,
            REPLAN_ACTOR=actor,
        )
        if binary == "default":
            env["REPLAN_BIN"] = os.environ.get("REPLAN_BIN", os.path.join(ROOT, "tmp", "replan-wt-bin"))
        elif binary:
            env["REPLAN_BIN"] = binary
        else:
            env.pop("REPLAN_BIN", None)
        env.update(extra_env or {})
        proc = subprocess.run(
            [sys.executable, "-m", "openproject_work_package_toolkit", "--root-dir", root_dir, *args],
            capture_output=True, text=True, env=env, timeout=30,
        )
        return proc, json.loads(proc.stdout)

    def fixture_project(self, tmp, *, bound=False):
        fixture = os.path.join(tmp, "work-packages.json")
        with open(fixture, "w", encoding="utf-8") as handle:
            json.dump({"_embedded": {"elements": [{
                "id": 1, "subject": "Old subject", "description": {"raw": "Old **markdown**"},
                "lockVersion": 3, "_links": {"self": {"href": "/api/v3/work_packages/1"}},
            }]}}, handle)
        bindings = os.path.join(tmp, "bindings.yaml")
        if bound:
            with open(bindings, "w", encoding="utf-8") as handle:
                yaml.safe_dump({"openproject_private_bindings": {"tasks": [{
                    "replan_task_id": "task.op.1234",
                    "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "1"},
                }]}}, handle)
        return fixture, bindings

    def test_fixture_dry_run_subject_description_and_both(self):
        cases = [
            (["--subject", "New subject"], [("subject", "Old subject", "New subject")]),
            (["--description", "New *markdown*"], [("description", "Old **markdown**", "New *markdown*")]),
            (["--subject", "New subject", "--description", "New text"], [
                ("subject", "Old subject", "New subject"),
                ("description", "Old **markdown**", "New text"),
            ]),
        ]
        for extra, expected in cases:
            with self.subTest(extra), tempfile.TemporaryDirectory() as tmp:
                fixture, bindings = self.fixture_project(tmp)
                proc, out = self.run_toolkit(
                    ["work-package", "update-text", "--work-package", "1", *extra],
                    project=tmp, fixture=fixture, bindings=bindings,
                )
                self.assertEqual(proc.returncode, 0, out)
                fields = out["actions"][0]["changed_fields"]
                self.assertEqual([(f["field"], f["old"], f["new"]) for f in fields], expected)
                self.assertEqual(out["actions"][0]["current_lock_version"], 3)
                self.assertIn("--lock-version 3", out["actions"][0]["apply_command"])

    def test_missing_change_and_bound_subject_context_are_refused(self):
        with tempfile.TemporaryDirectory() as tmp:
            fixture, bindings = self.fixture_project(tmp)
            proc, out = self.run_toolkit(
                ["work-package", "update-text", "--work-package", "1"],
                project=tmp, fixture=fixture, bindings=bindings,
            )
            self.assertEqual(proc.returncode, 2, out)
            self.assertEqual(out["error_codes"], ["missing_text_change"])

        with tempfile.TemporaryDirectory() as tmp:
            fixture, bindings = self.fixture_project(tmp, bound=True)
            proc, out = self.run_toolkit(
                ["work-package", "update-text", "--work-package", "1", "--subject", "New"],
                project=os.path.join(ROOT, "examples", "small-team"), fixture=fixture, bindings=bindings,
            )
            self.assertEqual(proc.returncode, 2, out)
            self.assertEqual(out["error_codes"], ["missing_title_sync_context"])
            self.assertIn("REPLAN_ACTOR", out["errors"][0]["message"])
            self.assertIn("--evidence", out["errors"][0]["message"])

    def serve(self, *, patch_status=200, subject="Old subject"):
        state = {"patches": [], "patch_status": patch_status, "subject": subject}

        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def send_json(self, status, body):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def do_GET(self):
                self.send_json(200, {"id": 1, "subject": state["subject"], "description": {"raw": "Old"}, "lockVersion": 3,
                                     "_links": {"self": {"href": "/api/v3/work_packages/1"}}})

            def do_PATCH(self):
                length = int(self.headers.get("Content-Length", "0"))
                state["patches"].append(json.loads(self.rfile.read(length)))
                if state["patch_status"] == 409:
                    self.send_json(409, {"errorIdentifier": "urn:openproject-org:api:v3:errors:UpdateConflict", "message": "Conflict"})
                else:
                    self.send_json(200, {"id": 1, "subject": "New subject", "lockVersion": 4})

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        return server, state

    def test_lock_mismatch_refuses_before_patch_and_409_is_mapped(self):
        for checked, status, want_patches in ((2, 200, 0), (3, 409, 1)):
            with self.subTest(checked=checked, status=status), tempfile.TemporaryDirectory() as tmp:
                server, state = self.serve(patch_status=status)
                try:
                    proc, out = self.run_toolkit(
                        ["work-package", "update-text", "--work-package", "1", "--description", "New", "--lock-version", str(checked)],
                        project=tmp, base_url=f"http://127.0.0.1:{server.server_port}", apply="1",
                    )
                finally:
                    server.shutdown()
                    server.server_close()
                self.assertEqual(proc.returncode, 2, out)
                self.assertEqual(out["error_codes"], ["openproject_work_package_changed"])
                self.assertEqual(len(state["patches"]), want_patches)
                if state["patches"]:
                    self.assertEqual(state["patches"][0]["lockVersion"], 3)
                    self.assertEqual(state["patches"][0]["description"], {"raw": "New"})

    def test_partial_apply_when_replan_write_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            bindings = os.path.join(tmp, "bindings.yaml")
            with open(bindings, "w", encoding="utf-8") as handle:
                yaml.safe_dump({"openproject_private_bindings": {"tasks": [{
                    "replan_task_id": "task.op.1234",
                    "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "1"},
                }]}}, handle)
            fake = os.path.join(tmp, "fake-replan")
            with open(fake, "w", encoding="utf-8") as handle:
                handle.write("#!/usr/bin/env python3\nimport json,sys\napply='--apply' in sys.argv\nprint(json.dumps({'ok': not apply, 'result': {'dry_run': not apply}}))\nsys.exit(2 if apply else 0)\n")
            os.chmod(fake, stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR)
            server, state = self.serve()
            try:
                proc, out = self.run_toolkit(
                    ["work-package", "update-text", "--work-package", "1", "--subject", "New subject", "--lock-version", "3", "--evidence", "evidence.github.pr.45"],
                    project=os.path.join(ROOT, "examples", "small-team"), bindings=bindings,
                    base_url=f"http://127.0.0.1:{server.server_port}", apply="1",
                    actor="actor.human.operator", binary=fake,
                )
            finally:
                server.shutdown()
                server.server_close()
            self.assertEqual(proc.returncode, 7, out)
            self.assertEqual(len(state["patches"]), 1)
            self.assertEqual(out["error_codes"], ["replan_title_apply_failed"])
            self.assertEqual(out["applied_so_far"][0]["action"], "update_openproject_text")
            self.assertIn("task set-title", out["finish_replan_command"])
            self.assertIn("--apply", out["finish_replan_command"])

    def test_title_drift_repairs_replan_without_patch_and_full_match_is_unchanged(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "project")
            shutil.copytree(os.path.join(ROOT, "examples", "small-team"), project)
            bindings = os.path.join(tmp, "bindings.yaml")
            with open(bindings, "w", encoding="utf-8") as handle:
                yaml.safe_dump({"openproject_private_bindings": {"tasks": [{
                    "replan_task_id": "task.op.1234",
                    "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "1"},
                }]}}, handle)
            server, state = self.serve(subject="Corrected subject")
            try:
                args = ["work-package", "update-text", "--work-package", "1", "--subject", "Corrected subject", "--evidence", "evidence.github.pr.45"]
                proc, preview = self.run_toolkit(args, project=project, bindings=bindings,
                                                 base_url=f"http://127.0.0.1:{server.server_port}", actor="actor.human.operator")
                self.assertEqual(proc.returncode, 0, preview)
                self.assertFalse(preview["actions"][0]["openproject_change"])
                self.assertTrue(preview["actions"][0]["replan_title_change"])
                proc, applied = self.run_toolkit(args, project=project, bindings=bindings,
                                                 base_url=f"http://127.0.0.1:{server.server_port}", apply="1", actor="actor.human.operator")
            finally:
                server.shutdown()
                server.server_close()
            self.assertEqual(proc.returncode, 0, applied)
            self.assertEqual(state["patches"], [])
            self.assertEqual(applied["counts"]["replan_title_changes"], 1)
            with open(os.path.join(project, "canonical", "tasks.yaml"), encoding="utf-8") as handle:
                tasks = yaml.safe_load(handle)["tasks"]
            self.assertEqual(next(t["title"] for t in tasks if t["id"] == "task.op.1234"), "Corrected subject")

            fixture, fixture_bindings = self.fixture_project(tmp, bound=True)
            with open(fixture, encoding="utf-8") as handle:
                document = json.load(handle)
            document["_embedded"]["elements"][0]["subject"] = "Implement validation command"
            with open(fixture, "w", encoding="utf-8") as handle:
                json.dump(document, handle)
            proc, unchanged = self.run_toolkit(
                ["work-package", "update-text", "--work-package", "1", "--subject", "Implement validation command", "--evidence", "evidence.github.pr.45"],
                project=os.path.join(ROOT, "examples", "small-team"), fixture=fixture,
                bindings=fixture_bindings, actor="actor.human.operator",
            )
            self.assertEqual(proc.returncode, 0, unchanged)
            self.assertEqual(unchanged["counts"]["unchanged"], 1)
            self.assertFalse(unchanged["actions"][0]["openproject_change"])
            self.assertFalse(unchanged["actions"][0]["replan_title_change"])

    def test_replan_binary_fallback_order(self):
        def fake_replan(path, marker):
            with open(path, "w", encoding="utf-8") as handle:
                handle.write("#!/usr/bin/env python3\nimport json\nprint(json.dumps({'ok': True, 'result': {'no_op': True}, 'marker': %r}))\n" % marker)
            os.chmod(path, stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR)

        with tempfile.TemporaryDirectory() as tmp:
            fixture, bindings = self.fixture_project(tmp, bound=True)
            explicit = os.path.join(tmp, "explicit-replan")
            fake_replan(explicit, "explicit")
            kit_root = os.path.join(tmp, "kit")
            os.makedirs(os.path.join(kit_root, "bin"))
            kit_binary = os.path.join(kit_root, "bin", "replan")
            fake_replan(kit_binary, "kit")
            args = ["work-package", "update-text", "--work-package", "1", "--subject", "New", "--evidence", "evidence.github.pr.45"]

            proc, out = self.run_toolkit(args, project=os.path.join(ROOT, "examples", "small-team"), fixture=fixture,
                                         bindings=bindings, actor="actor.human.operator", binary=explicit, root_dir=kit_root)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["actions"][0]["replan_title_preview"]["marker"], "explicit")

            proc, out = self.run_toolkit(args, project=os.path.join(ROOT, "examples", "small-team"), fixture=fixture,
                                         bindings=bindings, actor="actor.human.operator", binary="", root_dir=kit_root)
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["actions"][0]["replan_title_preview"]["marker"], "kit")

            build_root = os.path.join(tmp, "checkout")
            os.makedirs(build_root)
            fake_bin = os.path.join(tmp, "fake-bin")
            os.makedirs(fake_bin)
            go = os.path.join(fake_bin, "go")
            go_called = os.path.join(tmp, "go-called")
            with open(go, "w", encoding="utf-8") as handle:
                handle.write("#!/usr/bin/env python3\nimport os,sys\nopen(os.environ['GO_CALLED'],'w').write('yes')\nout=sys.argv[sys.argv.index('-o')+1]\nopen(out,'w').write(\"#!/usr/bin/env python3\\nimport json\\nprint(json.dumps({'ok': True, 'result': {'no_op': True}, 'marker': 'built'}))\\n\")\nos.chmod(out,0o700)\n")
            os.chmod(go, stat.S_IRUSR | stat.S_IWUSR | stat.S_IXUSR)
            proc, out = self.run_toolkit(args, project=os.path.join(ROOT, "examples", "small-team"), fixture=fixture,
                                         bindings=bindings, actor="actor.human.operator", binary="", root_dir=build_root,
                                         extra_env={"PATH": fake_bin + os.pathsep + os.environ["PATH"], "TMPDIR": tmp, "GO_CALLED": go_called})
            self.assertEqual(proc.returncode, 0, out)
            self.assertEqual(out["actions"][0]["replan_title_preview"]["marker"], "built")
            self.assertIn("replan-work-package-toolkit.", out["actions"][0]["replan_title_preview_command"])
            self.assertTrue(os.path.exists(go_called))
            leftovers = [name for name in os.listdir(tmp) if name.startswith("replan-work-package-toolkit.")]
            self.assertEqual(leftovers, [])

            os.remove(go_called)
            unbound_bindings = os.path.join(tmp, "unbound-bindings.yaml")
            proc, out = self.run_toolkit(
                ["work-package", "update-text", "--work-package", "1", "--description", "Only OpenProject"],
                project=os.path.join(ROOT, "examples", "small-team"), fixture=fixture,
                bindings=unbound_bindings, binary="", root_dir=build_root,
                extra_env={"PATH": fake_bin + os.pathsep + os.environ["PATH"], "TMPDIR": tmp, "GO_CALLED": go_called},
            )
            self.assertEqual(proc.returncode, 0, out)
            self.assertFalse(os.path.exists(go_called), "an unbound description dry run built replan")


if __name__ == "__main__":
    unittest.main()
