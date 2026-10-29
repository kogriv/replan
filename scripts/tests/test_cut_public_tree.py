import json
import os
import shutil
import subprocess
import tempfile
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))


class CutPublicTreeTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.source = os.path.join(self.temp.name, "source")
        os.makedirs(os.path.join(self.source, "scripts", "lib"))
        for rel in (
            "scripts/cut-public-tree.sh",
            "scripts/lib/envelope.sh",
            "scripts/lib/helper_envelope.py",
            "scripts/lib/cut_public_tree.py",
        ):
            target = os.path.join(self.source, rel)
            os.makedirs(os.path.dirname(target), exist_ok=True)
            shutil.copy2(os.path.join(ROOT, rel), target)
        self.write("go.mod", "module github.com/public-owner/replan\n\ngo 1.24\n")
        self.write("public.txt", "safe public text\n")
        self.write(".gitattributes", "public.txt export-ignore\n")
        self.write(".gitignore", "ignored.txt\n.env*\ntmp/\nbin/\n")
        self.git("init", "-q")
        self.git("add", "-A")
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "fixture")

    def write(self, rel, value):
        path = os.path.join(self.source, rel)
        os.makedirs(os.path.dirname(path), exist_ok=True)
        with open(path, "w", encoding="utf-8") as stream:
            stream.write(value)

    def git(self, *args):
        return subprocess.run(["git", *args], cwd=self.source, check=True, capture_output=True, text=True).stdout.strip()

    def run_cut(self, destination, apply=False, extra_env=None):
        env = os.environ.copy()
        env.update({
            "REPLAN_PUBLIC_CUT_DIR": destination,
            "REPLAN_PUBLIC_CUT_AUTHOR_NAME": "Public Author",
            "REPLAN_PUBLIC_CUT_AUTHOR_EMAIL": "public@example.test",
            "REPLAN_PUBLIC_CUT_DATE": "2026-10-28T12:00:00+07:00",
        })
        if apply:
            env["REPLAN_APPLY"] = "1"
        else:
            env.pop("REPLAN_APPLY", None)
        if extra_env:
            env.update(extra_env)
        proc = subprocess.run(
            [os.path.join(self.source, "scripts", "cut-public-tree.sh")],
            cwd=self.source, env=env, capture_output=True, text=True,
        )
        return proc, json.loads(proc.stdout)

    def commit_file(self, rel, value, message):
        self.write(rel, value)
        self.git("add", "-f", rel)
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", message)

    def test_dirty_tracked_tree_is_refused_and_ignored_file_is_irrelevant(self):
        self.write("ignored.txt", "must never travel\n")
        self.write("public.txt", "dirty\n")
        destination = os.path.join(self.temp.name, "dirty-cut")
        proc, out = self.run_cut(destination)
        self.assertEqual(proc.returncode, 11)
        self.assertEqual(out["error_codes"], ["tracked_worktree_dirty"])
        self.assertEqual(out["checks"][0]["name"], "tracked_worktree_clean")
        self.assertFalse(os.path.exists(destination))

    def test_tracked_private_path_is_refused(self):
        self.commit_file("private/secret.txt", "not public\n", "private fixture")
        destination = os.path.join(self.temp.name, "private-cut")
        proc, out = self.run_cut(destination)
        self.assertEqual(proc.returncode, 11)
        self.assertIn("private/secret.txt", next(c for c in out["checks"] if c["name"] == "no_private_paths")["paths"])
        self.assertFalse(os.path.exists(destination))

    def test_tracked_env_local_is_refused(self):
        self.commit_file(".env.local", "SECRET=value\n", "env fixture")
        proc, out = self.run_cut(os.path.join(self.temp.name, "env-cut"))
        self.assertEqual(proc.returncode, 11)
        check = next(c for c in out["checks"] if c["name"] == "no_env_files")
        self.assertEqual(check["paths"], [".env.local"])

    def test_generic_and_private_leaks_are_refused(self):
        cases = (
            ("generic.txt", "opapi-" + "deadbeef\n", None),
            ("named.txt", "PRIVATE-" + "PROBE\n", "PRIVATE-" + "PROBE"),
        )
        for rel, value, private_pattern in cases:
            with self.subTest(rel=rel):
                fixture = os.path.join(self.temp.name, rel + ".repo")
                shutil.copytree(self.source, fixture)
                old_source, self.source = self.source, fixture
                try:
                    self.commit_file(rel, value, "leak fixture")
                    extra = None
                    if private_pattern:
                        patterns = os.path.join(self.temp.name, rel + ".patterns")
                        with open(patterns, "w", encoding="utf-8") as stream:
                            stream.write(private_pattern + "\n")
                        extra = {"REPLAN_PRIVATE_LEAK_PATTERNS": patterns}
                    proc, out = self.run_cut(os.path.join(self.temp.name, rel + ".cut"), extra_env=extra)
                    self.assertEqual(proc.returncode, 11)
                    scan = next(c for c in out["checks"] if c["name"] == "generic_and_private_leak_scan")
                    self.assertEqual(scan["status"], "failed")
                finally:
                    self.source = old_source

    def owner_patterns(self):
        patterns = os.path.join(self.temp.name, "owner.patterns")
        with open(patterns, "w", encoding="utf-8") as stream:
            stream.write("public-owner\n")
        return patterns

    def test_owner_replan_urls_are_reported_as_exempt(self):
        self.commit_file(
            "repository.txt",
            "https://github.com/public-owner/replan\n"
            "git@github.com:public-owner/replan.git\n",
            "repository URLs",
        )
        proc, out = self.run_cut(
            os.path.join(self.temp.name, "owner-cut"),
            extra_env={"REPLAN_PRIVATE_LEAK_PATTERNS": self.owner_patterns()},
        )
        self.assertEqual(proc.returncode, 0, proc.stdout)
        exemptions = next(c for c in out["checks"] if c["name"] == "module_owner_exemptions")
        self.assertEqual(exemptions["files"], [
            {"path": "go.mod", "reasons": ["public RePlan module or repository path"]},
            {"path": "repository.txt", "reasons": ["public RePlan module or repository path"]},
        ])

    def test_allowlisted_bare_owner_samples_are_reported_as_exempt(self):
        self.write("docs/security/access-and-actor-model.md", "source: github:public-owner\n")
        self.write("docs/spec/canonical-model.md", "id: public-owner\n")
        self.git("add", "docs")
        self.git("-c", "user.name=Fixture", "-c", "user.email=fixture@example.test", "commit", "-qm", "sample identities")
        proc, out = self.run_cut(
            os.path.join(self.temp.name, "sample-cut"),
            extra_env={"REPLAN_PRIVATE_LEAK_PATTERNS": self.owner_patterns()},
        )
        self.assertEqual(proc.returncode, 0, proc.stdout)
        exemptions = next(c for c in out["checks"] if c["name"] == "module_owner_exemptions")
        self.assertEqual(exemptions["files"], [
            {"path": "docs/security/access-and-actor-model.md", "reasons": ["public sample GitHub identity"]},
            {"path": "docs/spec/canonical-model.md", "reasons": ["public sample actor account id"]},
            {"path": "go.mod", "reasons": ["public RePlan module or repository path"]},
        ])

    def test_owner_other_repository_url_is_refused(self):
        self.commit_file(
            "other-repository.txt",
            "https://github.com/public-owner/another-project\n",
            "other repository URL",
        )
        proc, out = self.run_cut(
            os.path.join(self.temp.name, "other-cut"),
            extra_env={"REPLAN_PRIVATE_LEAK_PATTERNS": self.owner_patterns()},
        )
        self.assertEqual(proc.returncode, 11)
        scan = next(c for c in out["checks"] if c["name"] == "generic_and_private_leak_scan")
        self.assertEqual(scan["matches"], [{"path": "other-repository.txt", "pattern": "private_pattern_1"}])

    def test_owner_repository_with_replan_prefix_is_refused(self):
        self.commit_file(
            "history-repository.txt",
            "https://github.com/public-owner/replan-history\n",
            "history repository URL",
        )
        proc, out = self.run_cut(
            os.path.join(self.temp.name, "history-cut"),
            extra_env={"REPLAN_PRIVATE_LEAK_PATTERNS": self.owner_patterns()},
        )
        self.assertEqual(proc.returncode, 11)
        scan = next(c for c in out["checks"] if c["name"] == "generic_and_private_leak_scan")
        self.assertEqual(scan["matches"], [{"path": "history-repository.txt", "pattern": "private_pattern_1"}])

    def test_dry_run_then_apply_builds_one_commit_with_equal_tree_and_no_remote(self):
        self.write("ignored.txt", "must never travel\n")
        self.write(".env.local", "must never travel\n")
        self.write("tmp/cache", "must never travel\n")
        self.write("bin/replan", "must never travel\n")
        destination = os.path.join(self.temp.name, "public-cut")
        dry_proc, dry = self.run_cut(destination)
        self.assertEqual(dry_proc.returncode, 0, dry_proc.stdout)
        self.assertEqual(dry["mode"], "dry_run")
        self.assertIsNone(dry["commit_sha"])
        self.assertIn("private_leak_patterns_missing", dry["warning_codes"])
        self.assertFalse(os.path.exists(destination))

        apply_proc, applied = self.run_cut(destination, apply=True)
        self.assertEqual(apply_proc.returncode, 0, apply_proc.stdout)
        self.assertEqual(applied["mode"], "apply")
        self.assertEqual(applied["tree_sha"], self.git("rev-parse", "HEAD^{tree}"))
        self.assertEqual(
            subprocess.run(["git", "rev-parse", "HEAD^{tree}"], cwd=destination, check=True, capture_output=True, text=True).stdout.strip(),
            applied["tree_sha"],
        )
        self.assertEqual(
            subprocess.run(["git", "rev-list", "--count", "HEAD"], cwd=destination, check=True, capture_output=True, text=True).stdout.strip(),
            "1",
        )
        self.assertEqual(
            subprocess.run(["git", "branch", "--show-current"], cwd=destination, check=True, capture_output=True, text=True).stdout.strip(),
            "main",
        )
        self.assertEqual(
            subprocess.run(["git", "remote"], cwd=destination, check=True, capture_output=True, text=True).stdout,
            "",
        )
        self.assertFalse(os.path.exists(os.path.join(destination, "ignored.txt")))
        self.assertFalse(os.path.exists(os.path.join(destination, ".env.local")))
        self.assertFalse(os.path.exists(os.path.join(destination, "tmp")))
        self.assertFalse(os.path.exists(os.path.join(destination, "bin")))


if __name__ == "__main__":
    unittest.main()
