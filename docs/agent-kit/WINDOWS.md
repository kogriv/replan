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

## Your project's Git checkout

The kit helpers write the plan, evidence and private binding files with LF
line endings on every platform. Git for Windows usually installs with
`core.autocrlf=true`, which checks text files out with CRLF; in such a clone a
file the kit wrote with LF is reported as `LF will be replaced by CRLF the next
time Git touches it`, and an editor or a later checkout can flip the whole file
back. Keep the plan in LF in one of two ways:

- Preferred, for everyone who clones the project: add a rule to the project's
  `.gitattributes`, for example

  ```gitattributes
  planning/** text eol=lf
  ```

  with your plan folder in place of `planning/`. An `eol` attribute overrides
  `core.autocrlf` for those paths, so every clone checks them out with LF.
  After adding the rule, run `git add --renormalize planning` once and commit.
- Only in your own clone: `git config core.autocrlf false` in that repository.
  Other people's clones keep their own setting.

A private binding file kept out of Git (ignored, the usual setup) is never
converted by Git and stays LF. If your project commits it, cover its folder
with the same `eol=lf` rule.

Plan folders have long paths, for example under `proposals/accepted/`. If the
clone sits in a deep folder, Git for Windows can refuse to create such files
with `Filename too long`. Clone the project into a short path such as
`C:/src/<project>`, or let Git use long paths:

```sh
git config --global core.longpaths true
# or for one clone only:
git clone -c core.longpaths=true <url> <folder>
```

The kit helpers convert their explicit path arguments before invoking native
Windows Python or `replan.exe`; they do not depend on Git Bash's automatic
argument conversion. It is safe to use the helpers with `MSYS_NO_PATHCONV=1`
and with `MSYS2_ARG_CONV_EXCL` set, including when those settings are used to
preserve `/api/v3/...` arguments.

If `scripts/accept-task.sh` fails with `[WinError 2] The system cannot find the
file specified`, use a full executable path including the suffix, for example
`REPLAN_BIN=C:/path/to/replan-kit/bin/replan.exe`. This is fixed since kit
v0.5.7; the explicit `.exe` setting is the workaround for older kits.

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
