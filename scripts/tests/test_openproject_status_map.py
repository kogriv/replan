import json
import os
import subprocess
import sys
import tempfile
import unittest

import openproject_status_map


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class OpenProjectStatusMapTest(unittest.TestCase):
    def test_mapping_and_resolution(self):
        expected = {
            "not_started": "New",
            "ready": "To be scheduled",
            "in_progress": "In progress",
            "blocked": "On hold",
            "review_ready": "Developed",
            "accepted": "Closed",
            "cancelled": "Rejected",
        }
        self.assertEqual(openproject_status_map.TRACKING_TO_OPENPROJECT_STATUS, expected)
        self.assertEqual(
            openproject_status_map.OPENPROJECT_TO_TRACKING_STATUS,
            {value: key for key, value in expected.items()},
        )
        self.assertEqual(openproject_status_map.resolve_status_argument("review_ready"), "Developed")
        self.assertEqual(openproject_status_map.resolve_status_argument("In progress"), "In progress")
        with self.assertRaisesRegex(ValueError, "valid values: not_started.*Rejected"):
            openproject_status_map.resolve_status_argument("done")


class ToolkitUpdateStatusTest(unittest.TestCase):
    def test_missing_internal_root_prefix_is_json_usage_error(self):
        proc = subprocess.run(
            [sys.executable, "-m", "openproject_work_package_toolkit", "work-package", "list"],
            capture_output=True,
            text=True,
            env=dict(os.environ, PYTHONPATH=LIB),
            timeout=30,
        )
        self.assertEqual(proc.returncode, 8)
        self.assertEqual(json.loads(proc.stdout)["error_codes"], ["usage_error"])

    def test_operator_root_dir_option_reaches_subcommand_parser(self):
        proc = subprocess.run(
            [
                sys.executable, "-m", "openproject_work_package_toolkit",
                "--root-dir", ROOT,
                "work-package", "list", "--root-dir", "operator-value",
            ],
            capture_output=True,
            text=True,
            env=dict(os.environ, PYTHONPATH=LIB),
            timeout=30,
        )
        self.assertEqual(proc.returncode, 8)
        self.assertEqual(proc.stderr, "")
        out = json.loads(proc.stdout)
        self.assertEqual(out["error_codes"], ["usage_error"])
        self.assertIn("unrecognized arguments: --root-dir operator-value", out["errors"][0]["message"])
        self.assertIn("usage:", out["errors"][0]["message"])

    def run_update(self, fixture, status):
        with tempfile.TemporaryDirectory() as tmp:
            fixture_path = os.path.join(tmp, "work-packages.json")
            with open(fixture_path, "w", encoding="utf-8") as handle:
                json.dump(fixture, handle)
            args = [
                sys.executable,
                "-m",
                "openproject_work_package_toolkit",
                "--root-dir",
                ROOT,
                "work-package",
                "update-status",
                "--work-package",
                "1",
                "--status",
                status,
            ]
            proc = subprocess.run(
                args,
                capture_output=True,
                text=True,
                env=dict(
                    os.environ,
                    PYTHONPATH=LIB,
                    REPLAN_PROJECT=tmp,
                    REPLAN_OPENPROJECT_BASE_URL="https://op.example.internal",
                    REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
                    REPLAN_OPENPROJECT_PROJECT_ID="demo",
                    REPLAN_OPENPROJECT_FROM_FILE=fixture_path,
                    REPLAN_OPENPROJECT_BINDINGS_PATH=os.path.join(tmp, "bindings.yaml"),
                ),
                timeout=30,
            )
            return proc.returncode, json.loads(proc.stdout)

    def setUp(self):
        self.fixture = {
            "_embedded": {
                "elements": [
                    {
                        "id": 1,
                        "subject": "Mapped status probe",
                        "lockVersion": 1,
                        "_links": {
                            "self": {"href": "/api/v3/work_packages/1"},
                            "status": {"href": "/api/v3/statuses/1", "title": "In progress"},
                        },
                    },
                    {
                        "id": 2,
                        "subject": "Review status catalog entry",
                        "lockVersion": 1,
                        "_links": {
                            "self": {"href": "/api/v3/work_packages/2"},
                            "status": {"href": "/api/v3/statuses/2", "title": "Developed"},
                        },
                    },
                ]
            }
        }

    def test_tracking_status_is_resolved(self):
        code, out = self.run_update(self.fixture, "review_ready")
        self.assertEqual(code, 0, out)
        self.assertEqual(out["actions"][0]["to_status"], "Developed")

    def test_openproject_status_name_is_accepted(self):
        code, out = self.run_update(self.fixture, "In progress")
        self.assertEqual(code, 0, out)
        self.assertEqual(out["actions"][0]["to_status"], "In progress")

    def test_bad_status_is_validation_error(self):
        code, out = self.run_update(self.fixture, "done")
        self.assertEqual(code, 2, out)
        self.assertEqual(out["error_codes"], ["invalid_status"])
        self.assertIn("not_started", out["errors"][0]["message"])
        self.assertIn("Rejected", out["errors"][0]["message"])
