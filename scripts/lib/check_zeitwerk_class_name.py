"""Moved from scripts/check-openproject-plugin-scaffold.sh (GAP-018). Invoked as `python3 -m check_zeitwerk_class_name`."""
import pathlib
import re
import sys

path = pathlib.Path(sys.argv[1])
text = path.read_text()
classes = re.findall(r"^\s*class\s+([A-Za-z0-9_]+)", text, re.M)
if len(classes) != 1:
    sys.stderr.write(f"expected exactly one class in {path}, found {classes!r}\n")
    sys.exit(1)
acronyms = {"api": "API"}
expected = "".join(acronyms.get(part, part.capitalize()) for part in path.stem.split("_"))
if classes[0] != expected:
    sys.stderr.write(f"{path} defines {classes[0]}, expected {expected}\n")
    sys.exit(1)
