"""Early OpenProject source-profile checks for bootstrap and overlays."""

import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class OpenProjectSourceProfileTest(unittest.TestCase):
    def write_project(self, directory, project_id=None):
        os.makedirs(os.path.join(directory, ".replan"), exist_ok=True)
        os.makedirs(os.path.join(directory, "canonical"), exist_ok=True)
        project = {
            "id": "project.test",
            "name": "Test",
            "format_version": 1,
            "timezone": "UTC",
        }
        if project_id is not None:
            project["source_profiles"] = [{
                "id": f"openproject.{project_id}",
                "source": "openproject",
                "base_url": "https://op.example.internal",
                "project_id": project_id,
                "token_env": "OPENPROJECT_TOKEN",
            }]
        with open(os.path.join(directory, ".replan", "project.yaml"), "w", encoding="utf-8") as handle:
            yaml.safe_dump({"project": project}, handle, sort_keys=False)
        with open(os.path.join(directory, "canonical", "tasks.yaml"), "w", encoding="utf-8") as handle:
            yaml.safe_dump({"tasks": [{"id": "task.a", "title": "A", "tracking_status": "not_started"}]}, handle)
        for name, value in (
            ("current_plan.yaml", {"current_plan": {"included_tasks": [{"kind": "task", "id": "task.a"}]}}),
            ("resource_demands.yaml", {"resource_demands": []}),
            ("dependencies.yaml", {"dependencies": []}),
        ):
            with open(os.path.join(directory, "canonical", name), "w", encoding="utf-8") as handle:
                yaml.safe_dump(value, handle)

    def blank_settings(self, directory):
        path = os.path.join(directory, "blank.env")
        with open(path, "w", encoding="utf-8"):
            pass
        return path

    def test_bootstrap_warns_when_profile_is_missing(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write_project(tmp)
            proc = subprocess.run(
                [os.path.join(ROOT, "scripts", "plan-openproject-bootstrap.sh")],
                capture_output=True,
                text=True,
                timeout=30,
                env={
                    "PATH": os.environ["PATH"],
                    "PYTHONPATH": LIB,
                    "REPLAN_ENV_FILE": self.blank_settings(tmp),
                    "REPLAN_PROJECT": tmp,
                    "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                },
            )
            out = json.loads(proc.stdout)
            self.assertEqual(proc.returncode, 0, out)
            self.assertTrue(out["ok"], out)
            self.assertIn("openproject_source_profile_missing", out["warning_codes"])
            self.assertIn("source_profiles", out["warnings"][0]["message"])
            self.assertIn("replan init --openproject-project-id demo", out["warnings"][0]["message"])

    def write_bindings_and_fake_replan(self, directory):
        bindings = os.path.join(directory, "bindings.yaml")
        with open(bindings, "w", encoding="utf-8") as handle:
            yaml.safe_dump({"openproject_private_bindings": {"tasks": [{
                "replan_task_id": "task.a",
                "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "1"},
            }]}}, handle)
        fake = os.path.join(directory, "fake-replan")
        with open(fake, "w", encoding="utf-8") as handle:
            handle.write(
                "#!/usr/bin/env python3\n"
                "import json, os, sys\n"
                "output = sys.argv[sys.argv.index('--output') + 1]\n"
                "os.makedirs(os.path.join(output, 'raw'), exist_ok=True)\n"
                "json.dump({'openproject_project_id': 'demo', 'work_package_count': 1, "
                "'linked_task_count': 1, 'missing_link_count': 0, 'work_packages': [{}]}, "
                "open(os.path.join(output, 'raw', 'openproject_overlay.json'), 'w'))\n"
            )
        os.chmod(fake, os.stat(fake).st_mode | stat.S_IXUSR)
        return bindings, fake

    def run_overlay(self, project_id):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.write_project(tmp.name, project_id)
        bindings, fake = self.write_bindings_and_fake_replan(tmp.name)
        output = os.path.join(tmp.name, "output")
        os.makedirs(output)
        sentinel = os.path.join(output, "sentinel")
        with open(sentinel, "w", encoding="utf-8") as handle:
            handle.write("keep until profile check passes")
        proc = subprocess.run(
            [
                sys.executable, "-m", "openproject_live_overlay",
                "--replan-bin", fake,
                "--output-dir", output,
                "--root-dir", ROOT,
            ],
            capture_output=True,
            text=True,
            timeout=30,
            env={
                **os.environ,
                "PYTHONPATH": LIB,
                "REPLAN_ENV_FILE": self.blank_settings(tmp.name),
                "REPLAN_PROJECT": tmp.name,
                "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                "REPLAN_OPENPROJECT_BINDINGS_PATH": bindings,
            },
        )
        return json.loads(proc.stdout), sentinel

    def test_overlay_refuses_missing_profile_before_output_write(self):
        out, sentinel = self.run_overlay(None)
        self.assertFalse(out["ok"], out)
        self.assertEqual(out["errors"][0]["code"], "openproject_source_profile_missing")
        self.assertTrue(os.path.exists(sentinel))

    def test_overlay_refuses_mismatched_profile_before_output_write(self):
        out, sentinel = self.run_overlay("another")
        self.assertFalse(out["ok"], out)
        self.assertEqual(out["errors"][0]["code"], "openproject_source_profile_mismatch")
        self.assertIn("'another'", out["errors"][0]["message"])
        self.assertIn("'demo'", out["errors"][0]["message"])
        self.assertTrue(os.path.exists(sentinel))

    def test_overlay_passes_matching_profile(self):
        out, sentinel = self.run_overlay("demo")
        self.assertTrue(out["ok"], out)
        self.assertFalse(os.path.exists(sentinel))
        self.assertEqual(out["counts"]["linked_task_count"], 1)


if __name__ == "__main__":
    unittest.main()
