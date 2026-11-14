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
