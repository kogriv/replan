import os
import tempfile
import unittest

from openproject_http import Settings, _explicit_settings_environ, _secret_setting
from platform_compat import normalize_windows_path, resolve_replan_binary, shell_script_command


class PlatformCompatTests(unittest.TestCase):
    def test_binary_resolution_finds_exe(self):
        with tempfile.TemporaryDirectory() as tmp:
            os.makedirs(os.path.join(tmp, "bin"))
            exe = os.path.join(tmp, "bin", "replan.exe")
            open(exe, "wb").close()
            self.assertEqual(resolve_replan_binary(tmp), exe)
            self.assertEqual(resolve_replan_binary(tmp, exe[:-4]), exe)

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
