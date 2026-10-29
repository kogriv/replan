"""Deterministic one-target release build integration test."""

import hashlib
import json
import os
import platform
import shutil
import subprocess
import tarfile
import tempfile
import unittest
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]


def run(args, *, cwd, env=None, check=False):
    return subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=True, check=check, timeout=180)


def host_target():
    machine = platform.machine().lower()
    machine = {"x86_64": "amd64", "aarch64": "arm64"}.get(machine, machine)
    return f"{platform.system().lower()}/{machine}"


class BuildReleaseTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="replan-release-test-", dir=os.environ.get("TMPDIR"))
        cls.base = Path(cls.temporary.name)
        seed = cls.base / "seed"
        shutil.copytree(ROOT, seed, ignore=shutil.ignore_patterns(".git", "tmp", "dist", "__pycache__", "*.pyc"))
        run(["git", "init", "--quiet"], cwd=seed, check=True)
        run(["git", "config", "user.name", "Release Test"], cwd=seed, check=True)
        run(["git", "config", "user.email", "release@example.invalid"], cwd=seed, check=True)
        run(["git", "add", "-A"], cwd=seed, check=True)
        commit_env = dict(os.environ, GIT_AUTHOR_DATE="2026-10-28T10:00:00Z", GIT_COMMITTER_DATE="2026-10-28T10:00:00Z")
        run(["git", "commit", "--quiet", "-m", "release fixture"], cwd=seed, env=commit_env, check=True)
        run(["git", "tag", "-a", "v0.1.0", "-m", "v0.1.0"], cwd=seed, check=True)
        cls.repo = cls.base / "repo"
        run(["git", "clone", "--quiet", str(seed), str(cls.repo)], cwd=cls.base, check=True)

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    def environment(self, output):
        return dict(
            os.environ,
            REPLAN_RELEASE_DIR=str(output),
            REPLAN_RELEASE_TARGETS=host_target(),
            TMPDIR=str(self.base),
        )

    def build(self, name):
        output = self.base / name
        proc = run([str(self.repo / "scripts" / "build-release.sh")], cwd=self.repo, env=self.environment(output))
        self.assertEqual(proc.returncode, 0, proc.stderr or proc.stdout)
        return output, json.loads(proc.stdout)

    def test_build_layout_checksums_metadata_and_reproducibility(self):
        first, envelope = self.build("first")
        second, _ = self.build("second")
        self.assertTrue(envelope["ok"])
        self.assertEqual((envelope["command"], envelope["version"]), ("build-release", "v0.1.0"))
        self.assertEqual(len(envelope["archives"]), 1)
        first_archive = first / envelope["archives"][0]["name"]
        second_archive = second / first_archive.name
        self.assertEqual(first_archive.read_bytes(), second_archive.read_bytes())

        digest = hashlib.sha256(first_archive.read_bytes()).hexdigest()
        self.assertEqual((first / "SHA256SUMS").read_text(encoding="utf-8"), f"{digest}  {first_archive.name}\n")
        top = first_archive.name.removesuffix(".tar.gz").removesuffix(".zip")
        expected_binary = "replan.exe" if host_target().startswith("windows/") else "replan"
        expected = {f"{top}/{expected_binary}", f"{top}/LICENSE", f"{top}/NOTICE", f"{top}/README.md"}
        commit_epoch = int(run(["git", "show", "-s", "--format=%ct"], cwd=self.repo, check=True).stdout)
        if first_archive.suffix == ".zip":
            with zipfile.ZipFile(first_archive) as archive:
                self.assertEqual(set(archive.namelist()), expected)
                modes = {item.filename: (item.external_attr >> 16) & 0o777 for item in archive.infolist()}
                self.assertEqual(modes[f"{top}/{expected_binary}"], 0o755)
                self.assertTrue(all(mode in (0o644, 0o755) for mode in modes.values()))
        else:
            with tarfile.open(first_archive, "r:gz") as archive:
                members = archive.getmembers()
                self.assertEqual({item.name for item in members}, expected)
                self.assertTrue(all(item.mtime == commit_epoch and item.uid == 0 and item.gid == 0 for item in members))
                self.assertEqual({item.name: item.mode for item in members}[f"{top}/{expected_binary}"], 0o755)

    def test_dirty_tree_missing_tag_and_help_refusals(self):
        dirty = self.repo / "untracked-release-test"
        dirty.write_text("dirty\n", encoding="utf-8")
        proc = run([str(self.repo / "scripts" / "build-release.sh")], cwd=self.repo, env=self.environment(self.base / "dirty"))
        dirty.unlink()
        self.assertEqual(proc.returncode, 8)
        self.assertEqual(json.loads(proc.stdout)["error_codes"], ["release_tree_dirty"])

        run(["git", "tag", "-d", "v0.1.0"], cwd=self.repo, check=True)
        proc = run([str(self.repo / "scripts" / "build-release.sh")], cwd=self.repo, env=self.environment(self.base / "untagged"))
        self.assertEqual(proc.returncode, 8)
        self.assertEqual(json.loads(proc.stdout)["error_codes"], ["release_tag_missing"])
        run(["git", "tag", "v0.1.0"], cwd=self.repo, check=True)

        proc = run([str(self.repo / "scripts" / "build-release.sh"), "--help"], cwd=self.repo, env={"PATH": os.environ["PATH"], "HOME": os.environ["HOME"]})
        self.assertEqual(proc.returncode, 0, proc.stderr)
        self.assertIn("Build deterministic local RePlan release archives", proc.stdout)


if __name__ == "__main__":
    unittest.main()
