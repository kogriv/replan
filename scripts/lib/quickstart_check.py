"""Run the quickstart guide's commands and check the output it promises.

Usage: python3 -m quickstart_check GUIDE REPLAN_BIN

Every ```sh block of GUIDE runs in order in one bash process, in a new
temporary directory, with REPLAN_BIN on PATH as `replan`. A ```text block
that follows a ```sh block lists lines that must appear in that block's
output; spaces are compared loosely, so column widths may change.
"""

import os
import re
import subprocess
import sys
import tempfile

FENCE = re.compile(r"^```(\w*)\s*\n(.*?)^```\s*$", re.MULTILINE | re.DOTALL)
MARKER = "@@quickstart-block-{}@@"


def blocks(text):
    """Return [(commands, expected_lines)] in page order."""
    out = []
    for match in FENCE.finditer(text):
        kind, body = match.group(1), match.group(2)
        if kind == "sh":
            out.append([body, []])
        elif kind == "text" and out and not out[-1][1]:
            out[-1][1] = [line for line in body.splitlines() if line.strip()]
    return [tuple(item) for item in out]


def squeeze(line):
    return " ".join(line.split())


def main(argv):
    guide, replan_bin = argv
    with open(guide, encoding="utf-8") as handle:
        steps = blocks(handle.read())
    if not steps:
        raise SystemExit(f"{guide}: no sh blocks")
    with tempfile.TemporaryDirectory(prefix="replan-quickstart.") as work:
        bin_dir = os.path.join(work, "bin")
        os.mkdir(bin_dir)
        os.symlink(os.path.abspath(replan_bin), os.path.join(bin_dir, "replan"))
        os.mkdir(os.path.join(work, "home"))
        script = ["set -euo pipefail"]
        for index, (commands, _) in enumerate(steps):
            script.append(f"echo {MARKER.format(index)}")
            script.append(commands)
        env = dict(os.environ, PATH=bin_dir + os.pathsep + os.environ.get("PATH", ""))
        for name in list(env):
            if name.startswith("REPLAN_"):
                del env[name]
        proc = subprocess.run(
            ["bash", "-c", "\n".join(script)], cwd=os.path.join(work, "home"),
            env=env, capture_output=True, text=True, check=False,
        )
        if proc.returncode != 0:
            raise SystemExit(f"quickstart commands failed with exit {proc.returncode}:\n{proc.stdout[-3000:]}\n{proc.stderr[-3000:]}")
        parts = re.split(r"^@@quickstart-block-(\d+)@@$", proc.stdout, flags=re.MULTILINE)
        outputs = {int(parts[i]): parts[i + 1] for i in range(1, len(parts) - 1, 2)}
        missing = []
        for index, (_, expected) in enumerate(steps):
            produced = [squeeze(line) for line in outputs.get(index, "").splitlines()]
            for line in expected:
                if not any(squeeze(line) in got for got in produced):
                    missing.append(f"block {index + 1}: {line.strip()}")
        if missing:
            raise SystemExit("quickstart output is missing:\n" + "\n".join(missing))
    print(f"quickstart ok: {len(steps)} command blocks")


if __name__ == "__main__":
    main(sys.argv[1:])
