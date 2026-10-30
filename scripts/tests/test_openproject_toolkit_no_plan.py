import json
import os
import subprocess
import tempfile
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
TOOLKIT = os.path.join(ROOT, "scripts", "openproject-work-package-toolkit.sh")
FIXTURE = os.path.join(
    ROOT, "examples", "imports", "openproject", "work-packages.sample.json"
)


class ToolkitWithoutPlanTest(unittest.TestCase):
    def run_toolkit(self, args, **extra_env):
        env = {
            "PATH": os.environ["PATH"],
            "HOME": tempfile.gettempdir(),
            "REPLAN_PROJECT": "",
            "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
            "REPLAN_OPENPROJECT_BASE_URL": "",
            "REPLAN_OPENPROJECT_FROM_FILE": FIXTURE,
            "REPLAN_OPENPROJECT_BINDINGS_PATH": "",
            "REPLAN_OPENPROJECT_TOKEN_ENV": "TEST_OPENPROJECT_TOKEN",
            "TEST_OPENPROJECT_TOKEN": "",
            **extra_env,
        }
        proc = subprocess.run(
            [TOOLKIT, *args], cwd=ROOT, env=env,
            capture_output=True, text=True, timeout=10,
        )
        self.assertEqual(proc.stderr, "")
        return proc, json.loads(proc.stdout)

    def assert_no_plan_success(self, args):
        proc, out = self.run_toolkit(args)
        self.assertEqual(proc.returncode, 0, out)
        self.assertTrue(out["ok"], out)
        self.assertEqual(out["plan"], "none")
        self.assertNotIn("private_binding_layer", out)

    def test_commands_by_work_package_run_without_plan(self):
        cases = (
            ["work-package", "list"],
            ["work-package", "inspect", "--work-package", "1234"],
            ["work-package", "create", "--subject", "A new unplanned work package"],
            ["work-package", "update-status", "--work-package", "1234", "--status", "in_progress"],
            ["work-package", "update-dates", "--work-package", "1234", "--start-date", "2026-11-02"],
            ["work-package", "update-text", "--work-package", "1234", "--description", "Updated text"],
            ["work-package", "update-metadata", "--work-package", "1234", "--estimated-hours", "4"],
            ["comment", "add", "--work-package", "1234", "--text", "Reviewed"],
            ["status", "list"],
            ["type", "list"],
            ["user", "list"],
            ["priority", "list"],
        )
        for args in cases:
            with self.subTest(args=args):
                self.assert_no_plan_success(args)

    def test_create_duplicate_check_does_not_need_bindings(self):
        proc, out = self.run_toolkit([
            "work-package", "create", "--subject", "Implement validation command",
        ])
        self.assertEqual(proc.returncode, 2)
        self.assertEqual(out["error_codes"], ["openproject_duplicate_subject"])
        self.assertEqual(out["plan"], "none")
        self.assertNotIn("private_binding_layer", out)

    def test_plan_needing_commands_still_name_replan_project(self):
        cases = (
            ["work-package", "bind", "--task", "task.a", "--work-package", "1234"],
            ["work-package", "inspect", "--task", "task.a"],
            ["work-package", "create", "--task", "task.a", "--subject", "New"],
            ["work-package", "update-status", "--task", "task.a", "--status", "new"],
            ["work-package", "update-dates", "--task", "task.a", "--start-date", "2026-11-02"],
            ["work-package", "update-text", "--task", "task.a", "--description", "Text"],
            ["work-package", "update-metadata", "--task", "task.a", "--estimated-hours", "1"],
            ["work-package", "replan-links"],
            ["comment", "add", "--task", "task.a", "--text", "Text"],
            ["backlog", "reconcile"],
            ["relation", "list"],
            ["relation", "delete", "--relation", "1"],
        )
        for args in cases:
            with self.subTest(args=args):
                proc, out = self.run_toolkit(args)
                self.assertEqual(proc.returncode, 8)
                self.assertEqual(out["error_codes"], ["missing_setting"])
                self.assertEqual(out["missing_settings"], ["REPLAN_PROJECT"])

    def test_secret_files_are_used_and_never_reported(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            token = "token-secret-that-must-not-leak"
            perimeter = "perimeter-secret-that-must-not-leak"
            token_path = os.path.join(temp_dir, "token")
            perimeter_path = os.path.join(temp_dir, "perimeter")
            with open(token_path, "w", encoding="utf-8") as stream:
                stream.write(f"  {token}\n")
            with open(perimeter_path, "w", encoding="utf-8") as stream:
                stream.write(f"{perimeter}\n")
            proc, out = self.run_toolkit(
                ["work-package", "list"],
                REPLAN_OPENPROJECT_TOKEN_FILE=token_path,
                REPLAN_OPENPROJECT_PERIMETER_KEY_FILE=perimeter_path,
            )
            combined = proc.stdout + proc.stderr
            self.assertTrue(out["ok"])
            self.assertNotIn(token, combined)
            self.assertNotIn(perimeter, combined)

    def test_unreadable_secret_file_is_missing_setting_without_path(self):
        secret_path = "/private/token/path-that-must-not-leak"
        proc, out = self.run_toolkit(
            ["work-package", "list"],
            REPLAN_OPENPROJECT_TOKEN_FILE=secret_path,
        )
        self.assertEqual(proc.returncode, 8)
        self.assertEqual(out["error_codes"], ["missing_setting"])
        self.assertEqual(out["missing_settings"], ["REPLAN_OPENPROJECT_TOKEN_FILE"])
        self.assertNotIn(secret_path, proc.stdout + proc.stderr)

    def test_environment_secrets_win_over_missing_files(self):
        proc, out = self.run_toolkit(
            ["work-package", "list"],
            TEST_OPENPROJECT_TOKEN="token-from-environment",
            REPLAN_OPENPROJECT_TOKEN_FILE="/missing/token-file",
            REPLAN_OPENPROJECT_PERIMETER_KEY="key-from-environment",
            REPLAN_OPENPROJECT_PERIMETER_KEY_FILE="/missing/key-file",
        )
        self.assertEqual(proc.returncode, 0, out)
        self.assertTrue(out["ok"])
        self.assertNotIn("token-from-environment", proc.stdout + proc.stderr)
        self.assertNotIn("key-from-environment", proc.stdout + proc.stderr)


if __name__ == "__main__":
    unittest.main()
