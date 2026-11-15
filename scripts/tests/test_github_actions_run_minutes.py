import json
import os
import stat
import subprocess
import sys
import tempfile
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SCRIPT = os.path.join(ROOT, "scripts", "github-actions-run-minutes.sh")


class GitHubActionsRunMinutesTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.bin_dir = os.path.join(self.temp.name, "bin")
        os.mkdir(self.bin_dir)
        os.symlink(sys.executable, os.path.join(self.bin_dir, "python3"))
        os.symlink("/usr/bin/dirname", os.path.join(self.bin_dir, "dirname"))
        os.symlink("/usr/bin/uname", os.path.join(self.bin_dir, "uname"))
        self.responses = os.path.join(self.temp.name, "responses.json")
        self.empty_env = os.path.join(self.temp.name, "empty.env")
        with open(self.empty_env, "w", encoding="utf-8"):
            pass

    def install_gh(self, responses=None, *, failure=""):
        if responses is not None:
            with open(self.responses, "w", encoding="utf-8") as stream:
                json.dump(responses, stream)
        path = os.path.join(self.bin_dir, "gh")
        with open(path, "w", encoding="utf-8") as stream:
            stream.write("""#!/usr/bin/env python3
import json, os, sys
if os.environ.get('FAKE_GH_FAILURE'):
    print(os.environ['FAKE_GH_FAILURE'], file=sys.stderr)
    raise SystemExit(1)
endpoint = sys.argv[2]
responses = json.load(open(os.environ['FAKE_GH_RESPONSES'], encoding='utf-8'))
value = responses[endpoint]
if isinstance(value, list):
    for page in value:
        print(json.dumps(page))
else:
    print(json.dumps(value))
""")
        os.chmod(path, os.stat(path).st_mode | stat.S_IXUSR)
        self.failure = failure

    def run_helper(self, *args, token=""):
        env = dict(os.environ)
        env.update({
            "PATH": self.bin_dir,
            "FAKE_GH_RESPONSES": self.responses,
            "FAKE_GH_FAILURE": getattr(self, "failure", ""),
            "SECRET_TEST_TOKEN": token,
            "REPLAN_ENV_FILE": self.empty_env,
        })
        proc = subprocess.run([SCRIPT, *args], capture_output=True, text=True, env=env)
        self.assertEqual(proc.stderr, "", proc.stderr)
        return proc, json.loads(proc.stdout)

    @staticmethod
    def run(run_id=123, status="completed"):
        return {
            "id": run_id, "status": status,
            "conclusion": "success" if status == "completed" else None,
            "head_sha": "abc123", "run_attempt": 2,
        }

    def test_completed_multi_job_rounding_and_skipped_job(self):
        self.install_gh({
            "repos/acme/widget/actions/runs/123": self.run(),
            "repos/acme/widget/actions/runs/123/jobs": [
                {"jobs": [
                    {"id": 1, "name": "quick", "status": "completed", "conclusion": "success", "started_at": "2026-11-15T00:00:00Z", "completed_at": "2026-11-15T00:00:01Z"},
                    {"id": 2, "name": "long", "status": "completed", "conclusion": "success", "started_at": "2026-11-15T00:00:30Z", "completed_at": "2026-11-15T00:01:31Z"},
                ]},
                {"jobs": [
                    {"id": 3, "name": "skip", "status": "completed", "conclusion": "skipped", "started_at": None, "completed_at": None},
                ]},
            ],
        })
        proc, out = self.run_helper("--repo", "acme/widget", "--run", "123")
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["totals"], {"wall_seconds": 91, "job_seconds": 62, "billable_minutes": 3})
        self.assertEqual([job["billable_minutes"] for job in out["runs"][0]["jobs"]], [1, 2, 0])
        self.assertEqual(out["runs"][0]["jobs"][2]["excluded_reason"], "skipped")
        self.assertEqual(out["runs"][0]["run_attempt"], 2)

    def test_incomplete_run_is_refused_unless_allowed(self):
        responses = {
            "repos/acme/widget/actions/runs/124": self.run(124, "in_progress"),
            "repos/acme/widget/actions/runs/124/jobs": {"jobs": []},
        }
        self.install_gh(responses)
        proc, out = self.run_helper("--repo", "acme/widget", "--run", "124")
        self.assertEqual(proc.returncode, 11, out)
        self.assertEqual(out["error_codes"], ["run_not_completed"])
        proc, out = self.run_helper("--repo", "acme/widget", "--run", "124", "--allow-incomplete")
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["runs"][0]["status"], "in_progress")

    def test_gh_missing(self):
        proc, out = self.run_helper("--repo", "acme/widget", "--run", "123")
        self.assertEqual(proc.returncode, 8, out)
        self.assertEqual(out["error_codes"], ["gh_not_found"])

    def test_gh_failure_is_external_and_redacts_tokens(self):
        self.install_gh({}, failure="request failed with secret-value")
        proc, out = self.run_helper("--repo", "acme/widget", "--run", "123", token="secret-value")
        self.assertEqual(proc.returncode, 6, out)
        self.assertEqual(out["error_codes"], ["github_request_failed"])
        self.assertNotIn("secret-value", proc.stdout)
        self.assertIn("[REDACTED]", out["errors"][0]["message"])


if __name__ == "__main__":
    unittest.main()
