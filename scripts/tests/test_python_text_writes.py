"""Static guard for platform-independent newlines in Python file writes."""

import ast
import pathlib
import re
import unittest


ROOT = pathlib.Path(__file__).resolve().parents[2]
LIB = ROOT / "scripts" / "lib"
SCRIPTS = ROOT / "scripts"
WRITE_CHARS = frozenset("wax")


def constant_string(node):
    return node.value if isinstance(node, ast.Constant) and isinstance(node.value, str) else None


def keyword_string(call, name):
    return next(
        (constant_string(keyword.value) for keyword in call.keywords if keyword.arg == name),
        None,
    )


def text_write_call(call):
    """Return the write API and mode for a statically identifiable text write."""
    function = call.func
    if isinstance(function, ast.Attribute) and function.attr == "write_text":
        return "Path.write_text", "w"

    if isinstance(function, ast.Name) and function.id == "open":
        mode = constant_string(call.args[1]) if len(call.args) > 1 else keyword_string(call, "mode")
        return ("open", mode) if mode else None

    if isinstance(function, ast.Attribute) and function.attr == "NamedTemporaryFile":
        mode = constant_string(call.args[0]) if call.args else keyword_string(call, "mode")
        return ("NamedTemporaryFile", mode) if mode else None

    if not isinstance(function, ast.Attribute) or function.attr != "open":
        return None

    # tarfile.open(..., mode="w") writes an archive, not a text stream.
    if isinstance(function.value, ast.Name) and function.value.id == "tarfile":
        return None
    mode = constant_string(call.args[0]) if call.args else keyword_string(call, "mode")
    return (".open", mode) if mode else None


def missing_newline_in_python(path):
    failures = []
    tree = ast.parse(path.read_text(encoding="utf-8"), filename=str(path))
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        identified = text_write_call(node)
        if not identified:
            continue
        api, mode = identified
        if "b" in mode or not WRITE_CHARS.intersection(mode):
            continue
        has_newline = any(keyword.arg == "newline" for keyword in node.keywords)
        if api == "Path.write_text" or not has_newline:
            failures.append(f"{path.relative_to(ROOT)}:{node.lineno}: {api}({mode!r})")
    return failures


class PythonTextWriteTests(unittest.TestCase):
    def test_python_text_writes_choose_lf_explicitly(self):
        failures = []
        for path in sorted(LIB.glob("*.py")):
            failures.extend(missing_newline_in_python(path))

        # Inline snippets are deliberately kept to one shell source line in
        # this tree. This guard mirrors the existing static shell-path test:
        # any text write on such a line must spell out newline="\\n".
        write_call = re.compile(
            r"(?:\bopen|\bio\.open)\([^,\n]+,\s*(?:mode\s*=\s*)?[\"'][^\"']*[wax]"
            r"|(?:\.open|\bNamedTemporaryFile)\(\s*(?:mode\s*=\s*)?[\"'][^\"']*[wax]"
            r"|\bwrite_text\s*\("
        )
        for path in sorted(SCRIPTS.glob("*.sh")):
            for line_number, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
                if write_call.search(line) and "newline=" not in line:
                    failures.append(f"{path.relative_to(ROOT)}:{line_number}: {line.strip()}")

        self.assertEqual(failures, [])


if __name__ == "__main__":
    unittest.main()
