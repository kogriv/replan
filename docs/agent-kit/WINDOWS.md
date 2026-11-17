# RePlan agent kit on Windows (Git Bash)

This kit uses native Windows programs from Git Bash; it does not require WSL.

1. Install Git for Windows and use its Git Bash terminal.
2. Install Python 3.9 or newer from python.org, selecting **Add Python to
   PATH**, then run `python -m pip install pyyaml`. Microsoft Store Python is
   also usable. If Python is elsewhere, set `REPLAN_PYTHON` to its executable.
3. Unpack the ZIP, preferably to a path without spaces. If the path has spaces,
   always quote it, for example `cd "/c/Users/Name/RePlan kit"`.
4. Open Git Bash in the kit folder and run `cp env.local.example .env.local`.
5. Keep the OpenProject token and perimeter key outside the kit in your user
   profile, for example `~/.config/replan/`. In Git Bash that is
   `C:\Users\<name>\.config\replan\`. Put their paths in `.env.local`.

Keep the kit's `.sh` and `.py` files with LF line endings. The included
`.gitattributes` protects Git checkouts, but an editor must not convert them to
CRLF. Settings and one-line secret files may use CRLF.

First read-only checks:

```sh
bin/replan.exe version
bin/replan.exe validate examples/small-team
scripts/openproject-work-package-toolkit.sh work-package list
```

The last command reads `.env.local`; without a token it returns a JSON warning
and does not write. WSL remains an alternative if the analyst prefers a full
Linux environment.

If `scripts/accept-task.sh` fails with `[WinError 2] The system cannot find the
file specified`, use a full executable path including the suffix, for example
`REPLAN_BIN=C:/path/to/replan-kit/bin/replan.exe`. This is fixed in the next
kit release; the explicit `.exe` setting is the workaround for older kits.

The kit's scratch-project copies work with long paths by default. Native
Windows Python accesses them through extended-length paths internally, while
the RePlan binary and messages continue to receive ordinary paths. Short
scratch-directory names leave extra headroom. If a destination would exceed
32,000 characters, or an extended-length copy still fails with a Windows path
error, the helper returns `temp_path_too_long` with exit 8 and names the length
and longest relative path instead of failing part-way with `[WinError 3]`.

Normally no setup is required. A short temporary directory remains an optional
workaround for an unusual filesystem or third-party tool that rejects long
paths:

```sh
mkdir -p C:/rt
export TMPDIR=C:/rt
scripts/accept-task.sh
```

As another optional measure, an administrator can enable **Win32 long paths**
in Group Policy, or set the `LongPathsEnabled` value under
`HKLM\SYSTEM\CurrentControlSet\Control\FileSystem` to `1`, and then restart
affected applications. That policy can help other Windows programs, but the
kit's Python scratch copies do not require it.

Git Bash's MSYS2 path layer maps its `cp` and `rm` operations into the native
Windows path namespace, including long paths. The kit nevertheless performs
scratch-tree copy and cleanup through the same Python extended-path helpers, so
those operations do not depend on shell-tool path handling; `rm -rf` remains a
cleanup fallback.
