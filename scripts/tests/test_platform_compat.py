import contextlib
import io
import json
import os
import subprocess
import tempfile
import unittest
from unittest import mock

from openproject_http import Settings, _explicit_settings_environ, _secret_setting
from platform_compat import (
    TempPathTooLong,
    _path_command,
    copy_project_tree,
    extended_length_path,
    guard_copy_destination,
    longest_copy_destination,
    normalize_windows_path,
    open_path,
    resolve_replan_binary,
    remove_tree,
    resolve_windows_executable,
    shell_script_command,
)


class PlatformCompatTests(unittest.TestCase):
    def test_extended_length_drive_path(self):
        self.assertEqual(
            extended_length_path("C:/Users/name/AppData/Local/Temp/p", windows=True),
            "\\\\?\\C:\\Users\\name\\AppData\\Local\\Temp\\p",
        )

    def test_extended_length_path_is_idempotent(self):
        path = "\\\\?\\C:\\already\\long"
        self.assertEqual(extended_length_path(path, windows=True), path)

    def test_extended_length_unc_path(self):
        self.assertEqual(
            extended_length_path("\\\\server\\share\\project\\p", windows=True),
            "\\\\?\\UNC\\server\\share\\project\\p",
        )

    def test_extended_length_relative_path_is_rejected(self):
        with self.assertRaisesRegex(ValueError, "must be absolute"):
            extended_length_path("relative/project", windows=True)

    def test_extended_length_path_is_unchanged_off_windows(self):
        self.assertEqual(extended_length_path("relative/project", windows=False), "relative/project")

    def test_longest_copy_destination_reports_long_proposal(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "project")
            proposal = os.path.join(project, "proposals", "open")
            os.makedirs(proposal)
            long_name = f"proposal.{'long-id-' * 20}.yaml"
            with open(os.path.join(proposal, long_name), "w", encoding="utf-8"):
                pass
            destination = "C:/Users/example/AppData/Local/Temp/rp-at.123456/p"
            length, relative = longest_copy_destination(project, destination)
            self.assertEqual(relative, f"proposals/open/{long_name}")
            self.assertEqual(length, len(f"{destination}/proposals/open/{long_name}"))

    def test_windows_copy_guard_refuses_only_above_limit(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "project")
            os.makedirs(os.path.join(project, "proposals", "open"))
            relative = os.path.join("proposals", "open", "proposal.long-id.yaml")
            with open(os.path.join(project, relative), "w", encoding="utf-8"):
                pass
            destination = "C:/rt/rp-at.123456/p"
            length, _ = longest_copy_destination(project, destination)
            with self.assertRaises(TempPathTooLong) as caught:
                guard_copy_destination(project, destination, windows=True, limit=length - 1)
            self.assertEqual(caught.exception.code, "temp_path_too_long")
            self.assertEqual(caught.exception.length, length)
            self.assertEqual(caught.exception.relative_path, relative.replace(os.sep, "/"))
            self.assertIn("set TMPDIR to a short directory such as C:/rt", str(caught.exception))
            self.assertEqual(
                guard_copy_destination(project, destination, windows=True, limit=length)[0],
                length,
            )
            self.assertEqual(
                guard_copy_destination(project, destination, windows=True)[0],
                length,
            )

    def test_non_windows_copy_guard_does_not_refuse(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "project")
            os.mkdir(project)
            with open(os.path.join(project, "file"), "w", encoding="utf-8"):
                pass
            self.assertEqual(guard_copy_destination(project, "/a", windows=False, limit=1)[1], "file")

    def test_copy_and_remove_use_extended_paths_but_subprocess_does_not(self):
        copied = []
        opened = []
        removed = []
        source = "C:/plans/project"
        destination = "C:/Users/name/AppData/Local/Temp/rp-at.123456/p"
        with mock.patch("platform_compat.longest_copy_destination", return_value=(254, "proposals/open/proposal.long.yaml")):
            copy_project_tree(
                source,
                destination,
                windows=True,
                copytree=lambda src, dst: copied.append((src, dst)),
            )
        remove_tree(destination, windows=True, rmtree=removed.append)
        open_path(
            destination + "/result.json",
            "r",
            windows=True,
            opener=lambda path, *args, **kwargs: opened.append((path, args, kwargs)),
        )
        self.assertEqual(copied, [(
            "\\\\?\\C:\\plans\\project",
            "\\\\?\\C:\\Users\\name\\AppData\\Local\\Temp\\rp-at.123456\\p",
        )])
        self.assertEqual(removed, ["\\\\?\\C:\\Users\\name\\AppData\\Local\\Temp\\rp-at.123456\\p"])
        self.assertEqual(opened[0][0], "\\\\?\\C:\\Users\\name\\AppData\\Local\\Temp\\rp-at.123456\\p\\result.json")
        with mock.patch("subprocess.run") as run:
            subprocess.run(["replan", "validate", "--project", destination])
        self.assertEqual(run.call_args.args[0][-1], destination)
        self.assertNotIn("\\\\?\\", run.call_args.args[0][-1])

    def test_extended_copy_path_failure_becomes_temp_path_error(self):
        failure = FileNotFoundError(2, "missing path")
        failure.winerror = 3
        with mock.patch("platform_compat.longest_copy_destination", return_value=(254, "proposals/open/proposal.long.yaml")):
            with self.assertRaises(TempPathTooLong) as caught:
                copy_project_tree(
                    "C:/plans/project",
                    "C:/temp/p",
                    windows=True,
                    copytree=lambda *_args: (_ for _ in ()).throw(failure),
                )
        self.assertEqual(caught.exception.length, 254)
        self.assertIn("extended-length copy failed", str(caught.exception))

    def test_shell_copy_guard_returns_expected_error_envelope(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "project")
            os.mkdir(project)
            with open(os.path.join(project, "proposal.long-id.yaml"), "w", encoding="utf-8"):
                pass
            output = io.StringIO()
            failure = TempPathTooLong(32001, "proposal.long-id.yaml")
            with mock.patch("platform_compat.copy_project_tree", side_effect=failure), contextlib.redirect_stdout(output):
                exit_code = _path_command([
                    "copy-tree", "test-helper", "json", project, "C:/temp/p",
                ])
            envelope = json.loads(output.getvalue())
            self.assertEqual(exit_code, 8)
            self.assertEqual(envelope["error_codes"], ["temp_path_too_long"])
            self.assertGreater(envelope["path_length"], 32000)
            self.assertEqual(envelope["longest_relative_path"], "proposal.long-id.yaml")

    def test_project_copy_helpers_use_short_scratch_paths(self):
        root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
        expected = {
            "scripts/accept-task.sh": "rp-at.XXXXXX",
            "scripts/backfill-openproject-status-history.sh": "rp-sh.XXXXXX",
            "scripts/validate-openproject-live.sh": "rp-vl.XXXXXX",
            "scripts/lib/accept_task.py": 'os.path.join(args.tmp_dir, "p")',
            "scripts/lib/backfill_openproject_status_history.py": 'os.path.join(args.tmp_dir, "p")',
        }
        for relative, marker in expected.items():
            with self.subTest(path=relative):
                with open(os.path.join(root, relative), encoding="utf-8") as stream:
                    self.assertIn(marker, stream.read())
        with open(os.path.join(root, "scripts/validate-openproject-live.sh"), encoding="utf-8") as stream:
            self.assertIn('LIVE_PROJECT="$TMP_DIR/p"', stream.read())

    def test_binary_resolution_finds_exe(self):
        with tempfile.TemporaryDirectory() as tmp:
            os.makedirs(os.path.join(tmp, "bin"))
            exe = os.path.join(tmp, "bin", "replan.exe")
            open(exe, "wb").close()
            self.assertEqual(resolve_replan_binary(tmp), exe)
            self.assertEqual(resolve_replan_binary(tmp, exe[:-4]), exe)

    def test_native_windows_executable_fallback_adds_exe(self):
        with mock.patch("platform_compat.os.name", "nt"), mock.patch(
            "platform_compat.os.path.exists", side_effect=lambda path: path == "C:/kit/bin/replan.exe"
        ):
            self.assertEqual(
                resolve_windows_executable("C:/kit/bin/replan"),
                "C:/kit/bin/replan.exe",
            )

    def test_windows_path_spellings(self):
        expected = os.path.normpath("C:/Users/a/plan")
        self.assertEqual(normalize_windows_path("C:/Users/a/plan", windows=True), expected)
        self.assertEqual(normalize_windows_path(r"C:\Users\a\plan", windows=True), expected)
        self.assertEqual(normalize_windows_path("/c/Users/a/plan", windows=True), expected)
        self.assertEqual(normalize_windows_path("/c/Users/a/plan", windows=False), "/c/Users/a/plan")

    def test_windows_shell_script_uses_sh(self):
        self.assertEqual(shell_script_command("tool.sh", "x", windows=True), ["sh", "tool.sh", "x"])
        self.assertEqual(shell_script_command("tool.sh", "x", windows=False), ["tool.sh", "x"])

    def test_crlf_settings_and_secret_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            env_file = os.path.join(tmp, "settings")
            token_file = os.path.join(tmp, "token")
            key_file = os.path.join(tmp, "key")
            with open(env_file, "wb") as stream:
                stream.write(b"REPLAN_PROJECT=C:/plans/demo\r\n")
            with open(token_file, "wb") as stream:
                stream.write(b"token-value\r\n")
            with open(key_file, "wb") as stream:
                stream.write(b"key-value\r\n")
            loaded = _explicit_settings_environ({"REPLAN_ENV_FILE": env_file})
            self.assertEqual(loaded["REPLAN_PROJECT"], "C:/plans/demo")
            self.assertEqual(_secret_setting({"TOKEN_FILE": token_file}, "TOKEN", "TOKEN_FILE"), "token-value")
            settings = Settings.from_env(environ={"REPLAN_OPENPROJECT_PERIMETER_KEY_FILE": key_file})
            self.assertEqual(settings.perimeter_key, "key-value")
