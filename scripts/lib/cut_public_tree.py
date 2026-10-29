"""Build a checked, one-commit public repository from the source HEAD."""

import os
import re
import shutil
import subprocess
import tempfile
from pathlib import Path

import helper_envelope


COMMAND = "cut-public-tree"
LIMIT = 1024 * 1024
GENERIC_PATTERNS = (
    ("generic_openproject_token", r"opapi-[0-9a-f]{8}"),
    ("generic_private_key", r"BEGIN [A-Z ]*PRIVATE KEY"),
)
BARE_OWNER_SAMPLE_ALLOWLIST = {
    "docs/security/access-and-actor-model.md": "public sample GitHub identity",
    "docs/spec/canonical-model.md": "public sample actor account id",
}

root_dir, argv = helper_envelope.internal_prefix(COMMAND, "--root-dir")
if argv:
    helper_envelope.fail(
        COMMAND, "invalid", "usage_error",
        "cut-public-tree.sh takes no arguments; see --help",
        helper_envelope.EXIT["usage"],
    )

mode = "apply" if os.environ.get("REPLAN_APPLY", "") == "1" else "dry_run"
helper_envelope.guard(COMMAND, mode)


def git(*args, cwd=root_dir, data=None, env=None, check=True):
    return subprocess.run(
        ["git", *args], cwd=cwd, input=data, capture_output=True,
        env=env, check=check,
    )


def check_entry(name, status, message, **extra):
    return {"name": name, "status": status, "message": message, **extra}


def fail_early(code, message, checks, **extra):
    helper_envelope.emit({
        "ok": False,
        "command": COMMAND,
        "mode": mode,
        "checks": checks,
        "errors": [helper_envelope.diagnostic(code, message)],
        **extra,
    }, helper_envelope.EXIT["attention"])


def validate_destination(raw):
    destination = Path(raw).expanduser().absolute()
    source = Path(root_dir).resolve()
    resolved = destination.resolve(strict=False)
    try:
        resolved.relative_to(source)
    except ValueError:
        pass
    else:
        helper_envelope.fail(
            COMMAND, "invalid", "cut_dir_inside_repository",
            "REPLAN_PUBLIC_CUT_DIR must be outside the source repository",
            helper_envelope.EXIT["usage"],
            extra={"cut_dir": str(destination)},
        )
    if destination.exists() and (not destination.is_dir() or any(destination.iterdir())):
        helper_envelope.fail(
            COMMAND, "invalid", "cut_dir_not_empty",
            "REPLAN_PUBLIC_CUT_DIR must not exist or must be an empty directory",
            helper_envelope.EXIT["usage"],
            extra={"cut_dir": str(destination)},
        )
    if not destination.parent.is_dir():
        helper_envelope.fail(
            COMMAND, "invalid", "cut_dir_parent_missing",
            "the parent of REPLAN_PUBLIC_CUT_DIR must already exist",
            helper_envelope.EXIT["usage"],
            extra={"cut_dir": str(destination)},
        )
    return destination


def extract_head(target):
    """Check out the exact HEAD index, ignoring export-ignore attributes."""
    index_fd, index_path = tempfile.mkstemp(prefix="replan-public-index.")
    os.close(index_fd)
    os.unlink(index_path)
    env = os.environ.copy()
    env["GIT_INDEX_FILE"] = index_path
    try:
        git("read-tree", "HEAD", env=env)
        git("checkout-index", "--all", "--force", f"--prefix={target}{os.sep}", env=env)
    finally:
        try:
            os.unlink(index_path)
        except FileNotFoundError:
            pass


def tracked_files(target):
    return sorted(path for path in Path(target).rglob("*") if path.is_file() or path.is_symlink())


def read_patterns(path):
    patterns = []
    with open(path, encoding="utf-8") as stream:
        for number, raw in enumerate(stream, 1):
            pattern = raw.rstrip("\n")
            if pattern:
                patterns.append((f"private_pattern_{number}", pattern))
    return patterns


destination = validate_destination(os.environ["REPLAN_PUBLIC_CUT_DIR"])
checks = []

dirty = git("diff", "--quiet", "HEAD", "--", check=False).returncode != 0
if dirty:
    checks.append(check_entry(
        "tracked_worktree_clean", "failed",
        "tracked files have staged or unstaged changes",
    ))
    fail_early("tracked_worktree_dirty", checks[-1]["message"], checks)
checks.append(check_entry("tracked_worktree_clean", "passed", "tracked files match HEAD"))

gitlinks = git("ls-files", "-s").stdout.decode("utf-8", "replace").splitlines()
gitlinks = [line for line in gitlinks if line.startswith("160000 ")]
if gitlinks:
    checks.append(check_entry("supported_tracked_entries", "failed", "gitlinks cannot be copied as HEAD blobs"))
    fail_early("tracked_gitlink_unsupported", checks[-1]["message"], checks)
checks.append(check_entry("supported_tracked_entries", "passed", "HEAD has no gitlinks"))

stage = Path(tempfile.mkdtemp(prefix=".replan-public-cut.", dir=destination.parent))
try:
    extract_head(stage)
    files = tracked_files(stage)
    total_size = sum(path.lstat().st_size for path in files)

    private_paths = [str(path.relative_to(stage)) for path in files if path.relative_to(stage).parts[0] == "private"]
    status = "failed" if private_paths else "passed"
    checks.append(check_entry("no_private_paths", status, "no path is under private/" if not private_paths else "paths under private/ were found", paths=private_paths))

    env_paths = []
    env_exceptions = []
    for path in files:
        name = path.name
        rel = str(path.relative_to(stage))
        if rel == ".env.example":
            env_exceptions.append({"path": rel, "reason": "tracked public settings template"})
        elif name == ".env" or name.startswith(".env."):
            env_paths.append(rel)
    status = "failed" if env_paths else "passed"
    checks.append(check_entry(
        "no_env_files", status,
        "no secret-bearing .env file was found" if not env_paths else ".env files were found",
        paths=env_paths, exceptions=env_exceptions,
    ))

    oversized = []
    for path in files:
        size = path.lstat().st_size
        if size > LIMIT:
            oversized.append({"path": str(path.relative_to(stage)), "size_bytes": size})
    status = "failed" if oversized else "passed"
    checks.append(check_entry(
        "file_size_limit", status,
        "every file is at most 1 MiB; there are no exceptions" if not oversized else "files over 1 MiB were found; no exceptions are configured",
        limit_bytes=LIMIT, exceptions=[], files=oversized,
    ))

    private_path = os.environ.get("REPLAN_PRIVATE_LEAK_PATTERNS", os.path.join(root_dir, "private", "leak-patterns.txt"))
    warnings = []
    patterns = list(GENERIC_PATTERNS)
    if os.path.isfile(private_path):
        try:
            patterns.extend(read_patterns(private_path))
            private_status = "passed"
            private_message = "private leak patterns were loaded"
        except (OSError, UnicodeError) as exc:
            checks.append(check_entry("private_leak_patterns", "failed", str(exc), path=private_path))
            fail_early("private_leak_patterns_invalid", str(exc), checks, cut_dir=str(destination))
    else:
        private_status = "warning"
        private_message = "private leak patterns file is missing; generic patterns still ran"
        warnings.append(helper_envelope.diagnostic(
            "private_leak_patterns_missing", private_message, "warning", "file", private_path,
        ))
    checks.append(check_entry("private_leak_patterns", private_status, private_message, path=private_path))

    module_line = (stage / "go.mod").read_text(encoding="utf-8").splitlines()[0]
    module_path = module_line.removeprefix("module ").strip()
    module_owner = module_path.split("/")[1] if module_path.startswith("github.com/") and len(module_path.split("/")) > 2 else ""
    compiled = []
    invalid = []
    for name, pattern in patterns:
        try:
            compiled.append((name, pattern, re.compile(pattern)))
        except re.error as exc:
            invalid.append({"pattern": pattern, "error": str(exc)})
    if invalid:
        checks.append(check_entry("leak_patterns_valid", "failed", "one or more leak patterns are invalid", patterns=invalid))
        fail_early("private_leak_patterns_invalid", checks[-1]["message"], checks, cut_dir=str(destination))
    checks.append(check_entry("leak_patterns_valid", "passed", "all leak patterns compile"))

    leaks = []
    exempted = {}
    owner_repo = re.compile(
        rf"(?<![A-Za-z0-9_.-]){re.escape(module_owner)}/replan(?:\.git)?(?![A-Za-z0-9_.-])"
    ) if module_owner else None
    for path in files:
        if path.is_symlink():
            content = os.readlink(path)
        else:
            content = path.read_bytes().decode("utf-8", "replace")
        rel = str(path.relative_to(stage))
        for name, pattern, regex in compiled:
            for match in regex.finditer(content):
                if module_owner and pattern == module_owner and match.group(0) == module_owner:
                    repo_match = next((item for item in owner_repo.finditer(content) if item.start() == match.start()), None)
                    if repo_match:
                        exempted.setdefault(rel, set()).add("public RePlan module or repository path")
                        continue
                    if rel in BARE_OWNER_SAMPLE_ALLOWLIST:
                        exempted.setdefault(rel, set()).add(BARE_OWNER_SAMPLE_ALLOWLIST[rel])
                        continue
                leaks.append({"path": rel, "pattern": name})
                break
    checks.append(check_entry(
        "module_owner_exemptions", "passed",
        "the module owner is exempt only in RePlan module/repository paths and allowlisted public sample identities",
        pattern=module_owner,
        files=[
            {"path": path, "reasons": sorted(reasons)}
            for path, reasons in sorted(exempted.items())
        ],
    ))
    status = "failed" if leaks else "passed"
    checks.append(check_entry("generic_and_private_leak_scan", status, "no leak pattern matched" if not leaks else "leak patterns matched", matches=leaks))

    failed = [entry for entry in checks if entry["status"] == "failed"]
    if failed:
        fail_early("public_cut_check_failed", "one or more public-tree checks failed", checks, warnings=warnings, cut_dir=str(destination))

    git("init", "--quiet", cwd=stage)
    git("symbolic-ref", "HEAD", "refs/heads/main", cwd=stage)
    git("add", "-f", "-A", cwd=stage)
    source_tree = git("rev-parse", "HEAD^{tree}").stdout.decode().strip()
    cut_tree = git("write-tree", cwd=stage).stdout.decode().strip()
    tree_ok = source_tree == cut_tree
    checks.append(check_entry(
        "source_tree_match", "passed" if tree_ok else "failed",
        "cut tree equals source HEAD tree" if tree_ok else "cut tree differs from source HEAD tree",
        source_tree_sha=source_tree, cut_tree_sha=cut_tree,
    ))
    if not tree_ok:
        fail_early("source_tree_mismatch", checks[-1]["message"], checks, warnings=warnings, cut_dir=str(destination))

    total_size = sum(path.lstat().st_size for path in files)
    commit_sha = None
    if mode == "apply":
        commit_env = os.environ.copy()
        commit_env.update({
            "GIT_AUTHOR_NAME": os.environ["REPLAN_PUBLIC_CUT_AUTHOR_NAME"],
            "GIT_AUTHOR_EMAIL": os.environ["REPLAN_PUBLIC_CUT_AUTHOR_EMAIL"],
            "GIT_COMMITTER_NAME": os.environ["REPLAN_PUBLIC_CUT_AUTHOR_NAME"],
            "GIT_COMMITTER_EMAIL": os.environ["REPLAN_PUBLIC_CUT_AUTHOR_EMAIL"],
        })
        commit_date = os.environ.get("REPLAN_PUBLIC_CUT_DATE", "")
        if commit_date:
            commit_env["GIT_AUTHOR_DATE"] = commit_date
            commit_env["GIT_COMMITTER_DATE"] = commit_date
        message = os.environ.get("REPLAN_PUBLIC_CUT_MESSAGE", "") or "Initial public release"
        git("commit", "--quiet", "-m", message, cwd=stage, env=commit_env)
        commit_sha = git("rev-parse", "HEAD", cwd=stage).stdout.decode().strip()
        committed_tree = git("rev-parse", "HEAD^{tree}", cwd=stage).stdout.decode().strip()
        if committed_tree != source_tree:
            fail_early("committed_tree_mismatch", "the committed tree differs from source HEAD", checks, warnings=warnings)
        checks.extend((
            check_entry("committed_tree_match", "passed", "the commit tree equals source HEAD", tree_sha=committed_tree),
            check_entry("single_commit", "passed", "the cut repository has exactly one commit", commit_count=1),
            check_entry("main_branch", "passed", "the initial branch is main", branch="main"),
            check_entry("no_remotes", "passed", "the cut repository has no remotes", remote_count=0),
        ))
        if destination.exists():
            destination.rmdir()
        stage.rename(destination)

    helper_envelope.emit({
        "ok": True,
        "command": COMMAND,
        "mode": mode,
        "checks": checks,
        "warnings": warnings,
        "cut_dir": str(destination),
        "commit_sha": commit_sha,
        "tree_sha": source_tree,
        "source_tree_sha": source_tree,
        "file_count": len(files),
        "total_size_bytes": total_size,
        "branch": "main",
        "remote_count": 0,
        "would_commit": mode == "dry_run",
    })
finally:
    if stage.exists():
        shutil.rmtree(stage)
