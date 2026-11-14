import json
import os
import pathlib
import subprocess
import sys
import tempfile
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
PLATFORM = ROOT / "scripts/lib/platform.sh"
DOTENV = ROOT / "scripts/lib/load-dotenv.sh"
REAL_PYTHON = pathlib.Path(os.environ.get("REPLAN_TEST_PYTHON", sys.executable))


class PlatformShellTests(unittest.TestCase):
    def write(self, directory, name, body):
        path = pathlib.Path(directory, name)
        path.write_text("#!/bin/sh\n" + body, encoding="utf-8")
        path.chmod(0o755)
        return path

    def run_shell(self, script, *, env=None):
        merged = dict(os.environ)
        merged.update(env or {})
        return subprocess.run(["sh", "-c", f'. "{PLATFORM}"; {script}'], env=merged, capture_output=True, text=True)

    def test_explicit_python_and_utf8_output(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = self.write(tmp, "chosen-python", f'exec "{REAL_PYTHON}" "$@"\n')
            proc = self.run_shell("replan_python -c 'print(\"Привет\")'", env={"REPLAN_PYTHON": str(fake), "REPLAN_PYTHON_KIND": ""})
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout.strip(), "Привет")

    def test_falls_back_from_python3_to_python(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write(tmp, "python3", "exit 1\n")
            self.write(tmp, "python", f'exec "{REAL_PYTHON}" "$@"\n')
            proc = self.run_shell("replan_python -c 'print(42)'", env={"PATH": f"{tmp}:/usr/bin:/bin", "REPLAN_PYTHON": "", "REPLAN_PYTHON_KIND": ""})
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            self.assertEqual(proc.stdout.strip(), "42")

    def test_store_stub_is_rejected_and_py_launcher_used(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write(tmp, "python3", "exit 1\n")
            self.write(tmp, "python", "exit 1\n")
            self.write(tmp, "py", f'[ "$1" = -3 ] && shift\nexec "{REAL_PYTHON}" "$@"\n')
            proc = self.run_shell("replan_python -c 'print(43)'", env={"PATH": f"{tmp}:/usr/bin:/bin", "REPLAN_PYTHON": "", "REPLAN_PYTHON_KIND": ""})
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            self.assertEqual(proc.stdout.strip(), "43")

    def test_bad_explicit_python_returns_json(self):
        proc = self.run_shell("replan_python -c 'print(1)'", env={"REPLAN_PYTHON": "/does/not/exist", "REPLAN_PYTHON_KIND": ""})
        self.assertEqual(proc.returncode, 8)
        self.assertEqual(json.loads(proc.stdout)["error_codes"], ["python_not_found"])
        self.assertIn("REPLAN_PYTHON", proc.stdout)

    def test_missing_pyyaml_returns_install_command(self):
        with tempfile.TemporaryDirectory() as tmp:
            fake = self.write(tmp, "python", "case \"$2\" in *yaml*) exit 1 ;; esac\nexit 0\n")
            proc = self.run_shell("replan_python -c 'print(1)'", env={"REPLAN_PYTHON": str(fake), "REPLAN_PYTHON_KIND": ""})
            self.assertEqual(proc.returncode, 8)
            self.assertEqual(json.loads(proc.stdout)["error_codes"], ["pyyaml_missing"])
            self.assertIn("python -m pip install pyyaml", proc.stdout)

    def test_msys_path_conversion_only_under_msys(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write(tmp, "uname", "printf 'MINGW64_NT-10.0\\n'\n")
            self.write(tmp, "cygpath", "shift 2\nprintf 'C:/converted%s\\n' \"$1\"\n")
            proc = self.run_shell("export REPLAN_PROJECT=/c/Users/a/plan; replan_prepare_windows_paths; printf '%s' \"$REPLAN_PROJECT\"", env={"PATH": f"{tmp}:/usr/bin:/bin", "MSYSTEM": ""})
            self.assertEqual(proc.stdout.strip(), "C:/converted/c/Users/a/plan")
            self.write(tmp, "uname", "printf 'Linux\\n'\n")
            proc = self.run_shell("export REPLAN_PROJECT=/c/Users/a/plan; replan_prepare_windows_paths; printf '%s' \"$REPLAN_PROJECT\"", env={"PATH": f"{tmp}:/usr/bin:/bin", "MSYSTEM": ""})
            self.assertEqual(proc.stdout.strip(), "/c/Users/a/plan")

    def test_shell_dotenv_strips_crlf(self):
        with tempfile.TemporaryDirectory() as tmp:
            dotenv = pathlib.Path(tmp, ".env.local")
            dotenv.write_bytes(b"REPLAN_PROJECT=/c/plan\r\n")
            script = (
                f'. "{DOTENV}"; unset REPLAN_PROJECT; '
                'replan_settings_names=REPLAN_PROJECT; replan_environment_only_names=; '
                'replan_settings_file_project_id=; replan_settings_loaded_names=:; '
                'replan_environment_only_file_settings=; '
                f'replan_load_openproject_dotenv_file "{dotenv}"; printf "%s" "$REPLAN_PROJECT"'
            )
            proc = self.run_shell(script)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout, "/c/plan")

    def test_shell_binary_resolution_finds_exe(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = pathlib.Path(tmp, "bin", "replan.exe")
            binary.parent.mkdir()
            binary.write_bytes(b"MZ")
            proc = self.run_shell(f'. "{DOTENV}"; replan_resolve_binary "{tmp}"', env={"REPLAN_BIN": ""})
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout.strip(), str(binary))
