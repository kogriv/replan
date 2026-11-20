"""Fixture-only tests for replacing and removing private task bindings."""

import json
import os
import subprocess
import sys
import tempfile
import unittest

import yaml


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


def source_ref(work_package_id, *, label=""):
    suffix = f"?label={label}" if label else ""
    return {
        "source": "openproject",
        "object_type": "work_package",
        "object_id": str(work_package_id),
        "url": f"https://example.invalid/work_packages/{work_package_id}{suffix}",
    }


class ToolkitTaskBindingsTest(unittest.TestCase):
    def make_plan(self, directory, *, task_bindings=None, crlf=False):
        canonical = os.path.join(directory, "canonical")
        os.makedirs(canonical)
        tasks = [
            {"id": "task.before", "title": "Before"},
            {"id": "task.target", "title": "Target"},
            {"id": "task.after", "title": "After"},
            {"id": "task.unbound", "title": "Unbound"},
        ]
        with open(os.path.join(canonical, "tasks.yaml"), "w", encoding="utf-8") as handle:
            yaml.safe_dump({"tasks": tasks}, handle, sort_keys=False)

        if task_bindings is None:
            task_bindings = [
                {"replan_task_id": "task.before", "source_ref": source_ref("5", label="keep-before"), "extension": "before"},
                {"replan_task_id": "task.target", "source_ref": source_ref("10", label="old")},
                {"replan_task_id": "task.after", "source_ref": source_ref("30", label="keep-after"), "extension": "after"},
            ]
        document = {
            "document_extension": {"keep": True},
            "openproject_private_bindings": {
                "project": {"source": "openproject", "project_id": "demo"},
                "generated_at": "old",
                "tasks": task_bindings,
                "dependencies": [{
                    "replan_dependency_id": "dependency.target-before",
                    "source_ref": {
                        "source": "openproject", "object_type": "relation",
                        "object_id": "90", "url": "https://example.invalid/api/v3/relations/90",
                    },
                    "relation_type": "follows",
                    "from_work_package_id": "10",
                    "to_work_package_id": "5",
                }, {
                    "replan_dependency_id": "dependency.legacy-no-endpoints",
                    "source_ref": {
                        "source": "openproject", "object_type": "relation",
                        "object_id": "91",
                    },
                }],
                "resources": [{"replan_resource_id": "resource.keep", "extension": True}],
                "extension": {"keep": True},
            },
        }
        bindings = os.path.join(directory, "bindings.yaml")
        rendered = yaml.safe_dump(document, sort_keys=False, allow_unicode=True)
        with open(bindings, "wb") as handle:
            handle.write(rendered.replace("\n", "\r\n").encode() if crlf else rendered.encode())

        fixture = os.path.join(directory, "openproject.json")
        with open(fixture, "w", encoding="utf-8") as handle:
            json.dump({"_embedded": {"elements": [
                {"id": int(work_package_id), "subject": f"WP {work_package_id}",
                 "_links": {"self": {"href": f"/api/v3/work_packages/{work_package_id}"}}}
                for work_package_id in (5, 10, 20, 30)
            ]}}, handle)
        return bindings, fixture

    def run_toolkit(self, project, bindings, fixture, args, *, apply=False):
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            REPLAN_PROJECT=project,
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_BASE_URL="https://example.invalid",
            REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
            TEST_TOKEN="",
            REPLAN_OPENPROJECT_FROM_FILE=fixture,
            REPLAN_OPENPROJECT_BINDINGS_PATH=bindings,
            REPLAN_APPLY="1" if apply else "",
        )
        proc = subprocess.run(
            [sys.executable, "-m", "openproject_work_package_toolkit",
             "--root-dir", ROOT, *args],
            capture_output=True, text=True, env=env, timeout=30,
        )
        self.assertEqual(proc.stderr, "")
        return proc, json.loads(proc.stdout)

    def test_bind_refusal_mentions_replace(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "20",
            ])
        self.assertEqual(proc.returncode, 2, out)
        self.assertEqual(out["error_codes"], ["task_already_bound"])
        self.assertIn("--replace", out["errors"][0]["message"])

    def test_replace_dry_run_reports_refs_warns_and_writes_nothing(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "20", "--replace",
            ])
            with open(bindings, "rb") as handle:
                after = handle.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(before, after)
        action = out["actions"][0]
        self.assertEqual(action["previous_source_ref"]["object_id"], "10")
        self.assertEqual(action["source_ref"]["object_id"], "20")
        self.assertTrue(action["replaced"])
        self.assertEqual(out["warning_codes"], [
            "dependency_bindings_reference_previous_work_package",
        ])
        self.assertIn("scripts/check-openproject-drift.sh", out["suggested_next_actions"][-1])

    def test_replace_apply_keeps_position_and_other_bindings_and_forces_lf(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project, crlf=True)
            with open(bindings, encoding="utf-8") as handle:
                before = yaml.safe_load(handle)
            before_layer = before["openproject_private_bindings"]
            before_other_tasks = [before_layer["tasks"][0], before_layer["tasks"][2]]
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "20", "--url",
                "https://example.invalid/work_packages/20", "--replace",
            ], apply=True)
            with open(bindings, "rb") as handle:
                raw = handle.read()
            document = yaml.safe_load(raw)
        self.assertEqual(proc.returncode, 0, out)
        self.assertNotIn(b"\r\n", raw)
        layer = document["openproject_private_bindings"]
        self.assertEqual([row["replan_task_id"] for row in layer["tasks"]], [
            "task.before", "task.target", "task.after",
        ])
        self.assertEqual([layer["tasks"][0], layer["tasks"][2]], before_other_tasks)
        self.assertEqual(layer["tasks"][1]["source_ref"]["object_id"], "20")
        self.assertEqual(layer["dependencies"], before_layer["dependencies"])
        self.assertEqual(layer["resources"], before_layer["resources"])
        self.assertEqual(document["document_extension"], before["document_extension"])
        self.assertNotEqual(layer["generated_at"], "old")

    def test_same_work_package_replace_is_no_op_even_on_apply(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "10", "--url",
                "https://example.invalid/work_packages/10?different=true", "--replace",
            ], apply=True)
            with open(bindings, "rb") as handle:
                after = handle.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(before, after)
        self.assertTrue(out["no_op"])
        self.assertFalse(out["actions"][0]["replaced"])
        self.assertEqual(out["counts"]["applied_actions"], 0)

    def test_replace_on_unbound_task_is_plain_bind(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.unbound",
                "--work-package", "20", "--replace",
            ], apply=True)
            with open(bindings, encoding="utf-8") as handle:
                tasks = yaml.safe_load(handle)["openproject_private_bindings"]["tasks"]
        self.assertEqual(proc.returncode, 0, out)
        self.assertFalse(out["actions"][0]["replaced"])
        self.assertEqual(out["actions"][0]["previous_source_ref"], {})
        self.assertEqual(tasks[-1]["replan_task_id"], "task.unbound")
        self.assertEqual(tasks[-1]["source_ref"]["object_id"], "20")

    def test_replace_repairs_a_malformed_existing_binding_in_place(self):
        malformed = [
            {"replan_task_id": "task.before", "source_ref": source_ref("5")},
            {"replan_task_id": "task.target", "source_ref": {
                "source": "openproject", "object_type": "damaged",
            }},
            {"replan_task_id": "task.after", "source_ref": source_ref("30")},
        ]
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project, task_bindings=malformed)
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "20", "--replace",
            ], apply=True)
            with open(bindings, encoding="utf-8") as handle:
                tasks = yaml.safe_load(handle)["openproject_private_bindings"]["tasks"]
        self.assertEqual(proc.returncode, 0, out)
        self.assertTrue(out["actions"][0]["replaced"])
        self.assertEqual(out["actions"][0]["previous_source_ref"]["object_type"], "damaged")
        self.assertEqual([row["replan_task_id"] for row in tasks], [
            "task.before", "task.target", "task.after",
        ])
        self.assertEqual(tasks[1]["source_ref"]["object_id"], "20")

    def test_replace_refuses_work_package_bound_to_another_task(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "30", "--replace",
            ])
        self.assertEqual(proc.returncode, 2, out)
        self.assertEqual(out["error_codes"], ["work_package_already_bound"])

    def test_unbind_dry_run_and_apply(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, preview = self.run_toolkit(project, bindings, fixture, [
                "work-package", "unbind", "--task", "task.target",
            ])
            with open(bindings, "rb") as handle:
                self.assertEqual(handle.read(), before)
            self.assertEqual(proc.returncode, 0, preview)
            self.assertEqual(preview["actions"][0]["previous_source_ref"]["object_id"], "10")
            self.assertEqual(preview["warning_codes"], [
                "dependency_bindings_reference_previous_work_package",
            ])

            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "unbind", "--task", "task.target",
            ], apply=True)
            with open(bindings, encoding="utf-8") as handle:
                layer = yaml.safe_load(handle)["openproject_private_bindings"]
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual([row["replan_task_id"] for row in layer["tasks"]], [
            "task.before", "task.after",
        ])
        self.assertEqual(len(layer["dependencies"]), 2)

    def test_unbind_refuses_unbound_task(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "unbind", "--task", "task.unbound",
            ])
        self.assertEqual(proc.returncode, 2, out)
        self.assertEqual(out["error_codes"], ["task_not_bound"])


if __name__ == "__main__":
    unittest.main()
