import json
import os
import pathlib
import re
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

    def test_path_argument_wrapper_preserves_linux_arguments(self):
        script = (
            "replan_python() { printf '<%s>\\n' \"$@\"; }; "
            "replan_python_path_args -m sample --project /work/plan "
            "--output /work/out --endpoint /api/v3/work_packages"
        )
        proc = self.run_shell(script, env={"MSYSTEM": ""})
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertEqual(
            proc.stdout.splitlines(),
            ["<-m>", "<sample>", "<--project>", "</work/plan>",
             "<--output>", "</work/out>", "<--endpoint>", "</api/v3/work_packages>"],
        )

    def run_msys_helper_with_stub(self, helper, env):
        with tempfile.TemporaryDirectory() as tmp:
            log = pathlib.Path(tmp, "argv.jsonl")
            env_file = pathlib.Path(tmp, "empty.env")
            env_file.write_text("", encoding="utf-8")
            self.write(tmp, "uname", "printf 'MINGW64_NT-10.0\\n'\n")
            self.write(
                tmp,
                "cygpath",
                "shift 2\nprintf 'C:/converted%s\\n' \"$1\"\n",
            )
            python = self.write(
                tmp,
                "native-python",
                """printf '%s\\n' \"$*\" >>\"$ARGV_LOG\"
case \"$*\" in
  *'helper_envelope resolve-openproject-settings'*) exit 0 ;;
  *'-m accept_task '*|*'-m openproject_live_overlay '*|*'-m apply_openproject_relations '*)
    printf '{}\\n'
    ;;
esac
exit 0
""",
            )
            merged = {
                "PATH": f"{tmp}:/usr/bin:/bin",
                "MSYSTEM": "MINGW64",
                "MSYS_NO_PATHCONV": "1",
                "MSYS2_ARG_CONV_EXCL": "*",
                "REPLAN_PYTHON": str(python),
                "REPLAN_PYTHON_KIND": "explicit",
                "ARGV_LOG": str(log),
                "REPLAN_ENV_FILE": str(env_file),
                "REPLAN_PROJECT": "/c/work/plan",
                "REPLAN_BIN": "/c/kit/bin/replan.exe",
                "REPLAN_OPENPROJECT_PROJECT_ID": "demo",
                "REPLAN_OPENPROJECT_BASE_URL": "https://op.example.test",
                "OPENPROJECT_TOKEN": "test-token",
                "REPLAN_TASK": "task.demo",
                "REPLAN_ACCEPTED_BY": "actor.human.operator",
                "REPLAN_ACCEPT_SUMMARY": "accepted",
                "REPLAN_OVERLAY_OUTPUT_DIR": "/c/work/overlay",
            }
            merged.update(env)
            proc = subprocess.run(
                [str(ROOT / "scripts" / helper)],
                env={**os.environ, **merged}, capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stdout + proc.stderr)
            return log.read_text(encoding="utf-8")

    def test_msys_no_pathconv_helpers_pass_windows_paths_to_python(self):
        cases = {
            "accept-task.sh": ("accept_task", ("--root-dir", "--tmp-dir", "--replan-bin")),
            "build-openproject-live-overlay.sh": (
                "openproject_live_overlay", ("--root-dir", "--output-dir", "--replan-bin"),
            ),
            "apply-openproject-relations.sh": ("apply_openproject_relations", ("--root-dir",)),
        }
        for helper, (module, options) in cases.items():
            with self.subTest(helper=helper):
                text = self.run_msys_helper_with_stub(helper, {})
                line = next(line for line in text.splitlines() if f"-m {module}" in line)
                for option in options:
                    value = line.split(f"{option} ", 1)[1].split(" ", 1)[0]
                    self.assertTrue(value.startswith("C:/converted/"), (helper, option, line))

    def test_python_path_arguments_are_explicitly_converted(self):
        path_variable = re.compile(r'"\$[A-Z][A-Z0-9_]*(?:DIR|BIN|JSON|ARCHIVE|KIT|OVERLAY|PATH)"')
        failures = []
        for script in sorted((ROOT / "scripts").glob("*.sh")):
            logical = script.read_text(encoding="utf-8").replace("\\\n", " ")
            for line_number, line in enumerate(logical.splitlines(), 1):
                if "replan_python" not in line or "PYTHONPATH=" in line and line.rstrip().endswith("\\"):
                    continue
                command = line.split("replan_python", 1)[1].split("||", 1)[0]
                scrubbed = re.sub(r'\$\(replan_py_path "\$[^"]+"\)', "", command)
                if path_variable.search(scrubbed):
                    failures.append(f"{script.name}:{line_number}: {line.strip()}")
        self.assertEqual(failures, [])

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

    def test_msys_binary_resolution_names_real_exe(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write(tmp, "uname", "printf 'MINGW64_NT-10.0\\n'\n")
            binary = pathlib.Path(tmp, "bin", "replan.exe")
            binary.parent.mkdir()
            binary.write_bytes(b"MZ")
            proc = self.run_shell(
                f'. "{DOTENV}"; replan_resolve_binary "{tmp}"',
                env={"PATH": f"{tmp}:/usr/bin:/bin", "MSYSTEM": "", "REPLAN_BIN": ""},
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout.strip(), str(binary))

    def test_msys_binary_resolution_ignores_implicit_unsuffixed_alias(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write(tmp, "uname", "printf 'MINGW64_NT-10.0\\n'\n")
            binary = pathlib.Path(tmp, "bin", "replan.exe")
            binary.parent.mkdir()
            binary.write_bytes(b"MZ")
            binary.chmod(0o755)
            binary.with_suffix("").symlink_to(binary.name)
            proc = self.run_shell(
                f'. "{DOTENV}"; replan_resolve_binary "{tmp}"',
                env={"PATH": f"{tmp}:/usr/bin:/bin", "MSYSTEM": "", "REPLAN_BIN": ""},
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout.strip(), str(binary))

    def test_msys_configured_binary_resolution_adds_exe(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write(tmp, "uname", "printf 'MINGW64_NT-10.0\\n'\n")
            binary = pathlib.Path(tmp, "replan.exe")
            binary.write_bytes(b"MZ")
            proc = self.run_shell(
                f'. "{DOTENV}"; replan_resolve_binary "{tmp}" "{str(binary)[:-4]}"',
                env={"PATH": f"{tmp}:/usr/bin:/bin", "MSYSTEM": "", "REPLAN_BIN": ""},
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout.strip(), str(binary))

    def test_linux_binary_resolution_keeps_unsuffixed_name(self):
        with tempfile.TemporaryDirectory() as tmp:
            self.write(tmp, "uname", "printf 'Linux\\n'\n")
            binary = pathlib.Path(tmp, "bin", "replan")
            binary.parent.mkdir()
            binary.write_bytes(b"native")
            binary.chmod(0o755)
            proc = self.run_shell(
                f'. "{DOTENV}"; replan_resolve_binary "{tmp}"',
                env={"PATH": f"{tmp}:/usr/bin:/bin", "MSYSTEM": "", "REPLAN_BIN": ""},
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            self.assertEqual(proc.stdout.strip(), str(binary))
