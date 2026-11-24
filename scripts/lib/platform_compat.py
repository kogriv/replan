"""Small native-Windows compatibility helpers shared by kit modules."""

import errno
import json
import ntpath
import os
import re
import shutil
import sys
from contextlib import contextmanager


WINDOWS_TEMP_PATH_LIMIT = 32000


class TempPathTooLong(ValueError):
    """A scratch copy cannot be represented or created safely on Windows."""

    code = "temp_path_too_long"

    def __init__(self, length, relative_path, detail=""):
        self.length = length
        self.relative_path = relative_path
        self.detail = detail
        reason = f"; {detail}" if detail else ""
        super().__init__(
            f"temporary project path would be {length} characters at "
            f"{relative_path!r}{reason}; set TMPDIR to a short directory such as C:/rt"
        )


def extended_length_path(path, *, windows=None):
    """Use the Win32 extended-length namespace for an absolute native path."""
    windows = os.name == "nt" if windows is None else windows
    if not windows:
        return path
    if path.startswith("\\\\?\\"):
        return path
    normalized = ntpath.normpath(path.replace("/", "\\"))
    if not ntpath.isabs(normalized):
        raise ValueError(f"extended-length path must be absolute: {path!r}")
    if normalized.startswith("\\\\"):
        return "\\\\?\\UNC\\" + normalized[2:]
    drive, tail = ntpath.splitdrive(normalized)
    if not drive or not tail.startswith("\\"):
        raise ValueError(f"extended-length path must be drive-absolute or UNC: {path!r}")
    return "\\\\?\\" + normalized


def longest_copy_destination(project, destination):
    """Return ``(length, relative path)`` for the longest copied tree path."""
    longest = (len(destination), ".")
    windows = os.name == "nt"
    path_module = ntpath if windows else os.path
    project_root = _absolute_filesystem_path(project, windows)
    scan_root = extended_length_path(project_root, windows=windows)
    for directory, dirs, files in os.walk(scan_root):
        relative_directory = path_module.relpath(directory, scan_root)
        names = [*dirs, *files]
        for name in names:
            relative = name if relative_directory == "." else path_module.join(relative_directory, name)
            candidate = path_module.join(destination, relative)
            measured = (len(candidate), relative.replace("\\", "/"))
            if measured[0] > longest[0]:
                longest = measured
    return longest


def guard_copy_destination(project, destination, *, windows=None, limit=WINDOWS_TEMP_PATH_LIMIT):
    """Refuse only paths beyond the Win32 extended-length practical ceiling."""
    longest = longest_copy_destination(project, destination)
    windows = os.name == "nt" if windows is None else windows
    if windows and longest[0] > limit:
        raise TempPathTooLong(*longest)
    return longest


def _copy_path_error(exc):
    if isinstance(exc, shutil.Error):
        text = str(exc).lower()
        return any(marker in text for marker in (
            "winerror 2", "winerror 3", "winerror 206",
            "filename or extension is too long", "path too long",
        ))
    return (
        isinstance(exc, FileNotFoundError)
        or getattr(exc, "winerror", None) in {2, 3, 206}
        or getattr(exc, "errno", None) in {errno.ENOENT, errno.ENAMETOOLONG}
    )


def _absolute_filesystem_path(path, windows):
    if windows and ntpath.isabs(path):
        return path
    return os.path.abspath(path)


def copy_project_tree(project, destination, *, windows=None, copytree=shutil.copytree):
    """Copy a project, using extended paths only for native filesystem calls."""
    windows = os.name == "nt" if windows is None else windows
    longest = guard_copy_destination(project, destination, windows=windows)
    source_path = extended_length_path(_absolute_filesystem_path(project, windows), windows=windows)
    destination_path = extended_length_path(_absolute_filesystem_path(destination, windows), windows=windows)
    try:
        copytree(source_path, destination_path)
    except (OSError, shutil.Error) as exc:
        if windows and _copy_path_error(exc):
            raise TempPathTooLong(
                *longest,
                detail=f"extended-length copy failed: {exc}",
            ) from exc
        raise
    return longest


def remove_tree(path, *, windows=None, rmtree=shutil.rmtree):
    """Remove a scratch tree through the extended namespace on Windows."""
    windows = os.name == "nt" if windows is None else windows
    absolute = _absolute_filesystem_path(path, windows)
    rmtree(extended_length_path(absolute, windows=windows))


def open_path(path, *args, windows=None, opener=open, **kwargs):
    """Open one file through the extended namespace on native Windows."""
    windows = os.name == "nt" if windows is None else windows
    absolute = _absolute_filesystem_path(path, windows)
    return opener(extended_length_path(absolute, windows=windows), *args, **kwargs)


@contextmanager
def exclusive_file_lock(path):
    """Lock a persistent sidecar file, even when the protected file is replaced."""
    with open_path(path, "a+b") as handle:
        if os.name == "nt":
            import msvcrt

            handle.seek(0)
            msvcrt.locking(handle.fileno(), msvcrt.LK_LOCK, 1)
        else:
            import fcntl

            fcntl.flock(handle.fileno(), fcntl.LOCK_EX)
        # Closing the handle releases the lock. Keep the file in place so a
        # waiting process and a new arrival always lock the same inode.
        yield


def _error_envelope(command, output_format, exc):
    message = str(exc)
    if output_format == "json":
        print(json.dumps({
            "ok": False,
            "command": command,
            "mode": "invalid",
            "warnings": [],
            "errors": [{
                "code": exc.code,
                "message": message,
                "severity": "error",
                "target": {"kind": "path", "id": exc.relative_path},
            }],
            "warning_codes": [],
            "error_codes": [exc.code],
            "path_length": exc.length,
            "longest_relative_path": exc.relative_path,
        }, ensure_ascii=False))
    else:
        print(message, file=sys.stderr)


def _path_command(argv):
    """Shell-facing extended copy/remove operations for kit helpers."""
    if not argv:
        return 8
    if argv[0] == "copy-tree" and len(argv) == 5:
        _, command, output_format, project, destination = argv
        try:
            copy_project_tree(project, destination)
        except TempPathTooLong as exc:
            _error_envelope(command, output_format, exc)
            return 8
        return 0
    if argv[0] == "remove-tree" and len(argv) == 2:
        remove_tree(argv[1])
        return 0
    return 8


def normalize_windows_path(path, *, windows=None):
    """Accept C:/, C:\\ and /c/ Git Bash spellings on native Windows."""
    if not path:
        return path
    windows = os.name == "nt" if windows is None else windows
    if not windows:
        return path
    match = re.match(r"^/([A-Za-z])(?:/(.*))?$", path)
    if match:
        path = f"{match.group(1).upper()}:/{match.group(2) or ''}"
    return os.path.normpath(path.replace("\\", "/"))


def resolve_windows_executable(path, *, windows=None):
    """Name an existing .exe when native Windows cannot start an implicit name."""
    windows = os.name == "nt" if windows is None else windows
    if windows and not os.path.exists(path) and os.path.exists(path + ".exe"):
        return path + ".exe"
    return path


def resolve_replan_binary(root_dir, configured="", *, is_file=os.path.isfile):
    """Return the actual binary filename, preferring an explicit setting."""
    candidates = []
    if configured:
        candidates.extend((configured, configured + ".exe"))
    else:
        candidates.extend((os.path.join(root_dir, "bin", "replan"), os.path.join(root_dir, "bin", "replan.exe")))
    for candidate in candidates:
        if is_file(candidate) and (candidate.lower().endswith(".exe") or os.access(candidate, os.X_OK)):
            return candidate
    return configured


def shell_script_command(path, *arguments, windows=None):
    """Run a Git Bash script through sh when native Windows cannot exec it."""
    windows = os.name == "nt" if windows is None else windows
    prefix = ["sh", path] if windows else [path]
    return [*prefix, *arguments]


if __name__ == "__main__":
    raise SystemExit(_path_command(sys.argv[1:]))
