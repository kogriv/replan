import glob
import json
import os
import re
import subprocess
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
SCRIPTS = sorted(glob.glob(os.path.join(ROOT, "scripts", "*.sh")))


def empty_environment():
    return {"PATH": os.environ["PATH"], "HOME": os.environ["HOME"]}


def purpose_line(path):
    with open(path, encoding="utf-8") as stream:
        stream.readline()
        return stream.readline().removeprefix("# ").rstrip("\n")


class ScriptHelpTest(unittest.TestCase):
    def test_every_header_lists_exact_setting_names_and_defaults(self):
        setting_line = re.compile(
            r"^#   [A-Z][A-Z0-9_]*\s+(?:required|default .+|"
            r"deprecated alias of [A-Z][A-Z0-9_]*|"
            r"only json; any other value is refused|"
            r"json prints .+; text prints .+)$"
        )
        for script in SCRIPTS:
            with self.subTest(script=os.path.basename(script)):
                with open(script, encoding="utf-8") as stream:
                    lines = stream.read().splitlines()
                start = lines.index("# Settings:") + 1
                settings = lines[start:lines.index("#", start)]
                self.assertTrue(settings)
                for line in settings:
                    self.assertRegex(line, setting_line)

    def test_every_script_answers_help_without_settings(self):
        self.assertEqual(len(SCRIPTS), 36)
        for script in SCRIPTS:
            with self.subTest(script=os.path.basename(script)):
                proc = subprocess.run(
                    [script, "--help"], capture_output=True, text=True,
                    env=empty_environment(), timeout=10,
                )
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertTrue(proc.stdout.strip())
                self.assertTrue(proc.stdout.startswith(purpose_line(script)), proc.stdout[:300])
                self.assertEqual(proc.stderr, "")

    def test_json_helper_rejects_an_extra_argument(self):
        script = os.path.join(ROOT, "scripts", "check-openproject-drift.sh")
        proc = subprocess.run(
            [script, "unexpected"], capture_output=True, text=True,
            env=empty_environment(), timeout=10,
        )
        self.assertEqual(proc.returncode, 8)
        self.assertEqual(proc.stderr, "")
        out = json.loads(proc.stdout)
        self.assertFalse(out["ok"])
        self.assertEqual(out["error_codes"], ["usage_error"])
        self.assertEqual(out["errors"][0]["message"], "check-openproject-drift.sh takes no arguments; see --help")

    def test_wiki_and_toolkit_subcommand_help_is_only_argparse_help(self):
        cases = (
            ("openproject-work-package-toolkit.sh", ["work-package", "create", "--help"]),
            ("openproject-wiki.sh", ["page", "create", "--help"]),
        )
        for name, args in cases:
            with self.subTest(script=name):
                script = os.path.join(ROOT, "scripts", name)
                proc = subprocess.run(
                    [script, *args], capture_output=True, text=True,
                    env=empty_environment(), timeout=10,
                )
                self.assertEqual(proc.returncode, 0, proc.stderr)
                self.assertTrue(proc.stdout.startswith("usage:"), proc.stdout[:300])
                self.assertNotIn(purpose_line(script), proc.stdout)
                self.assertEqual(proc.stderr, "")

    def test_toolkit_and_wiki_argument_errors_use_the_envelope(self):
        cases = (
            ("openproject-work-package-toolkit.sh", ["work-package", "create"], "--subject"),
            ("openproject-wiki.sh", ["page", "create"], "--title"),
        )
        for name, args, missing in cases:
            with self.subTest(script=name):
                proc = subprocess.run(
                    [os.path.join(ROOT, "scripts", name), *args],
                    capture_output=True, text=True, env=empty_environment(), timeout=10,
                )
                self.assertEqual(proc.returncode, 8)
                self.assertEqual(proc.stderr, "")
                self.assertNotIn("Traceback", proc.stdout)
                out = json.loads(proc.stdout)
                self.assertFalse(out["ok"])
                self.assertEqual(out["error_codes"], ["usage_error"])
                self.assertIn(missing, out["errors"][0]["message"])
                self.assertIn("usage:", out["errors"][0]["message"])


if __name__ == "__main__":
    unittest.main()
