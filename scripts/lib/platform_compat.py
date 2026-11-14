"""Small native-Windows compatibility helpers shared by kit modules."""

import os
import re


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
