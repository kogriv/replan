"""Build deterministic RePlan release archives from the exact HEAD tree."""

import gzip
import hashlib
import io
import json
import os
import platform
import re
import stat
import subprocess
import tarfile
import tempfile
import zipfile
from pathlib import Path, PurePosixPath

import helper_envelope


COMMAND = "build-release"
DEFAULT_TARGETS = "linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64"

root_dir, argv = helper_envelope.internal_prefix(COMMAND, "--root-dir")
if argv:
    helper_envelope.fail(COMMAND, "invalid", "usage_error", "build-release.sh takes no arguments; see --help", helper_envelope.EXIT["usage"])
helper_envelope.guard(COMMAND, "read_only", fallback_code="release_build_failed")


def run(*args, cwd=root_dir, env=None, text=True, check=True):
    return subprocess.run(args, cwd=cwd, env=env, capture_output=True, text=text, check=check)


def git(*args, **kwargs):
    return run("git", *args, **kwargs)


def fail(code, message, exit_code=helper_envelope.EXIT["usage"], **extra):
    helper_envelope.fail(COMMAND, "invalid", code, message, exit_code, extra=extra)


def release_version():
    configured = os.environ.get("REPLAN_RELEASE_VERSION", "")
    if configured:
        version = configured
    else:
        exact = git("describe", "--tags", "--exact-match", "HEAD", check=False)
        if exact.returncode != 0 or not exact.stdout.strip():
            fail("release_tag_missing", "HEAD has no exact tag; set REPLAN_RELEASE_VERSION or tag the release commit")
        version = exact.stdout.strip()
    if not re.fullmatch(r"v(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?", version):
        fail("release_version_invalid", f"release version {version!r} is not a v-prefixed semantic version")
    return version


def validate_target(value):
    parts = value.split("/")
    if len(parts) != 2 or not all(parts) or any(not part.replace("_", "").isalnum() for part in parts):
        fail("release_target_invalid", f"invalid release target {value!r}; expected os/arch")
    return parts


def validate_output():
    output = Path(os.environ["REPLAN_RELEASE_DIR"]).expanduser().absolute()
    if output.exists() and (not output.is_dir() or any(output.iterdir())):
        fail("release_dir_not_empty", "REPLAN_RELEASE_DIR must not exist or must be an empty directory", release_dir=str(output))
    output.mkdir(parents=True, exist_ok=True)
    return output


def validate_clean():
    status = git("status", "--porcelain", "--untracked-files=normal").stdout.splitlines()
    if status and os.environ.get("REPLAN_RELEASE_ALLOW_DIRTY", "") != "1":
        fail("release_tree_dirty", "the worktree has tracked changes or untracked non-ignored files; set REPLAN_RELEASE_ALLOW_DIRTY=1 to build HEAD anyway", dirty_paths=[line[3:] for line in status])


def commit_epoch():
    return int(git("show", "-s", "--format=%ct", "HEAD").stdout.strip())


def archive_members(binary, binary_name, top, source_root):
    return (
        (binary, f"{top}/{binary_name}", 0o755),
        *((source_root / name, f"{top}/{name}", 0o644) for name in ("LICENSE", "NOTICE", "README.md")),
    )


def write_tar(path, members, mtime):
    with path.open("wb") as raw:
        with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=mtime) as zipped:
            with tarfile.open(fileobj=zipped, mode="w", format=tarfile.PAX_FORMAT) as archive:
                for source, name, mode in members:
                    data = source.read_bytes()
                    info = tarfile.TarInfo(name)
                    info.size, info.mode, info.mtime = len(data), mode, mtime
                    info.uid = info.gid = 0
                    info.uname = info.gname = ""
                    archive.addfile(info, io.BytesIO(data))


def write_zip(path, members, mtime):
    # ZIP stores local time without a timezone. UTC and fixed permissions make
    # the byte stream independent of the machine running the release.
    from datetime import datetime, timezone
    stamp = datetime.fromtimestamp(max(mtime, 315532800), timezone.utc).timetuple()[:6]
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED, compresslevel=9) as archive:
        for source, name, mode in members:
            info = zipfile.ZipInfo(str(PurePosixPath(name)), stamp)
            info.create_system = 3
            info.external_attr = (stat.S_IFREG | mode) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(info, source.read_bytes(), compress_type=zipfile.ZIP_DEFLATED, compresslevel=9)


def checksum(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


version = release_version()
validate_clean()
output_dir = validate_output()
targets = os.environ.get("REPLAN_RELEASE_TARGETS", DEFAULT_TARGETS).split()
if not targets:
    fail("release_targets_empty", "REPLAN_RELEASE_TARGETS must contain at least one os/arch target")

commit = git("rev-parse", "HEAD").stdout.strip()
build_date = git("show", "-s", "--format=%cI", "HEAD").stdout.strip()
mtime = commit_epoch()
archives = []
host_target = f"{platform.system().lower()}/{platform.machine().lower()}"
if host_target.endswith("/x86_64"):
    host_target = host_target.removesuffix("x86_64") + "amd64"
elif host_target.endswith("/aarch64"):
    host_target = host_target.removesuffix("aarch64") + "arm64"

with tempfile.TemporaryDirectory(prefix="replan-release.", dir=os.environ.get("TMPDIR")) as temporary:
    build_dir = Path(temporary)
    source_dir = build_dir / "source"
    source_dir.mkdir()
    head_archive = git("archive", "--format=tar", "HEAD", text=False).stdout
    with tarfile.open(fileobj=io.BytesIO(head_archive), mode="r:") as source_archive:
        source_archive.extractall(source_dir)
    for target in targets:
        goos, goarch = validate_target(target)
        binary_name = "replan.exe" if goos == "windows" else "replan"
        binary = build_dir / f"{goos}-{goarch}-{binary_name}"
        ldflags = " ".join((
            "-s", "-w",
            f"-X github.com/kogriv/replan/internal/buildinfo.Version={version}",
            f"-X github.com/kogriv/replan/internal/buildinfo.Commit={commit}",
            f"-X github.com/kogriv/replan/internal/buildinfo.BuildDate={build_date}",
        ))
        env = os.environ.copy()
        env.update({"CGO_ENABLED": "0", "GOOS": goos, "GOARCH": goarch})
        # The extracted source has no VCS data; a stray .git above TMPDIR must
        # not make go build try to stamp it. The commit comes from ldflags.
        run("go", "build", "-trimpath", "-buildvcs=false", "-ldflags", ldflags, "-o", str(binary), "./cmd/replan", cwd=source_dir, env=env)
        stem = f"replan_{version}_{goos}_{goarch}"
        suffix = ".zip" if goos == "windows" else ".tar.gz"
        archive_path = output_dir / f"{stem}{suffix}"
        members = archive_members(binary, binary_name, stem, source_dir)
        if goos == "windows":
            write_zip(archive_path, members, mtime)
        else:
            write_tar(archive_path, members, mtime)
        archives.append({"path": str(archive_path), "name": archive_path.name, "target": target, "size": archive_path.stat().st_size, "sha256": checksum(archive_path)})

        if target == host_target:
            probe = run(str(binary), "version", "--format", "json")
            reported = json.loads(probe.stdout)["result"]["version"]
            if reported != version:
                fail("release_version_mismatch", f"host binary reported {reported!r}, expected {version!r}", helper_envelope.EXIT["attention"])

sums = output_dir / "SHA256SUMS"
sums.write_text("".join(f"{item['sha256']}  {item['name']}\n" for item in archives), encoding="utf-8")
helper_envelope.emit({
    "ok": True,
    "command": COMMAND,
    "mode": "read_only",
    "version": version,
    "commit": commit,
    "build_date": build_date,
    "release_dir": str(output_dir),
    "archives": archives,
    "sha256sums": str(sums),
    "warnings": [],
    "errors": [],
})
