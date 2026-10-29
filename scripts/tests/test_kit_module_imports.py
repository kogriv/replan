import pathlib
import tempfile
import unittest

import kit_module_imports


class KitModuleImportsTest(unittest.TestCase):
    def test_reports_only_repository_helpers_missing_from_the_kit(self):
        with tempfile.TemporaryDirectory() as tmp:
            kit, repo = pathlib.Path(tmp, "kit"), pathlib.Path(tmp, "repo")
            kit.mkdir()
            repo.mkdir()
            for name in ("present", "absent", "user"):
                (repo / f"{name}.py").write_text("", encoding="utf-8")
            (kit / "present.py").write_text("", encoding="utf-8")
            (kit / "user.py").write_text(
                "import json\nimport present\nfrom absent import thing\nfrom . import relative\n",
                encoding="utf-8",
            )
            self.assertEqual(kit_module_imports.missing_imports(kit, repo), ["user.py imports absent"])


if __name__ == "__main__":
    unittest.main()
