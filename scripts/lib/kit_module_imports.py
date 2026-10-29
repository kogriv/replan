"""List repository helper modules that the agent kit's Python modules import
but the kit does not carry. Used by scripts/check-agent-kit.sh.

Usage:
  python3 -m kit_module_imports KIT_LIB_DIR REPO_LIB_DIR
Prints a comma-separated "module imports helper" list, empty when complete.
"""
import ast
import pathlib
import sys


def missing_imports(kit, repo):
    missing = set()
    for path in kit.glob("*.py"):
        for node in ast.walk(ast.parse(path.read_text(encoding="utf-8"))):
            if isinstance(node, ast.Import):
                names = [alias.name for alias in node.names]
            elif isinstance(node, ast.ImportFrom) and node.module and not node.level:
                names = [node.module]
            else:
                continue
            for name in names:
                top = name.split(".")[0]
                if (repo / f"{top}.py").is_file() and not (kit / f"{top}.py").is_file():
                    missing.add(f"{path.name} imports {top}")
    return sorted(missing)


if __name__ == "__main__":
    print(", ".join(missing_imports(pathlib.Path(sys.argv[1]), pathlib.Path(sys.argv[2]))))
