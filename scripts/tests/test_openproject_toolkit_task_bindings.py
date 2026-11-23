"""Fixture-only tests for replacing and removing private task bindings."""

import copy
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

    def run_toolkit(
        self, project, bindings, fixture, args, *, apply=False,
        base_url="https://example.invalid",
    ):
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            REPLAN_PROJECT=project,
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_BASE_URL=base_url,
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

    def test_same_work_package_different_url_dry_run_reports_refresh(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "10", "--url",
                "https://example.invalid/work_packages/10?different=true", "--replace",
            ])
            with open(bindings, "rb") as handle:
                after = handle.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(before, after)
        action = out["actions"][0]
        self.assertTrue(action["url_refreshed"])
        self.assertFalse(action["replaced"])
        self.assertEqual(
            action["previous_url"],
            "https://example.invalid/work_packages/10?label=old",
        )
        self.assertEqual(
            action["new_url"],
            "https://example.invalid/work_packages/10?different=true",
        )
        self.assertEqual(out["counts"]["planned_actions"], 1)

    def test_same_work_package_different_configured_url_apply_updates_only_url(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project, crlf=True)
            with open(bindings, encoding="utf-8") as handle:
                before = yaml.safe_load(handle)
            expected = copy.deepcopy(before)
            expected["openproject_private_bindings"]["tasks"][1]["source_ref"]["url"] = (
                "https://new.example.invalid/work_packages/10"
            )
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "10", "--replace",
            ], apply=True, base_url="https://new.example.invalid")
            with open(bindings, "rb") as handle:
                raw = handle.read()
            after = yaml.safe_load(raw)
        self.assertEqual(proc.returncode, 0, out)
        self.assertNotIn(b"\r\n", raw)
        self.assertEqual(after, expected)
        self.assertTrue(out["actions"][0]["url_refreshed"])
        self.assertTrue(out["actions"][0]["applied"])

    def test_same_work_package_same_url_is_no_op_even_on_apply(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "bind", "--task", "task.target",
                "--work-package", "10", "--url",
                "https://example.invalid/work_packages/10?label=old", "--replace",
            ], apply=True)
            with open(bindings, "rb") as handle:
                after = handle.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(before, after)
        self.assertTrue(out["no_op"])
        self.assertFalse(out["actions"][0]["url_refreshed"])
        self.assertFalse(out["actions"][0]["replaced"])
        self.assertEqual(out["counts"]["applied_actions"], 0)

    def test_rebase_urls_dry_run_lists_changes_and_unchanged_without_writing(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, encoding="utf-8") as handle:
                document = yaml.safe_load(handle)
            document["openproject_private_bindings"]["tasks"][2]["source_ref"]["url"] = (
                "https://other.invalid/work_packages/30"
            )
            with open(bindings, "w", encoding="utf-8", newline="\n") as handle:
                yaml.safe_dump(document, handle, sort_keys=False)
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "rebase-urls",
                "--from", "https://example.invalid",
                "--to", "https://new.example.invalid",
            ])
            with open(bindings, "rb") as handle:
                after = handle.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(before, after)
        self.assertEqual(out["counts"]["changed_bindings"], 3)
        self.assertEqual(out["counts"]["unchanged_bindings"], 2)
        self.assertEqual(
            [(item["binding_kind"], item["replan_id"]) for item in out["items"]],
            [
                ("task", "task.before"),
                ("task", "task.target"),
                ("task", "task.after"),
                ("dependency", "dependency.target-before"),
                ("dependency", "dependency.legacy-no-endpoints"),
            ],
        )
        self.assertEqual(
            [item["status"] for item in out["items"]],
            ["changed", "changed", "unchanged", "changed", "unchanged"],
        )

    def test_rebase_urls_apply_changes_only_matching_urls_and_forces_lf(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project, crlf=True)
            with open(bindings, encoding="utf-8") as handle:
                before = yaml.safe_load(handle)
            expected = copy.deepcopy(before)
            layer = expected["openproject_private_bindings"]
            for binding in [*layer["tasks"], *layer["dependencies"]]:
                source = binding.get("source_ref") or {}
                url = source.get("url", "")
                if url.startswith("https://example.invalid"):
                    source["url"] = "https://new.example.invalid" + url[len("https://example.invalid"):]
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "rebase-urls",
                "--from", "https://example.invalid",
                "--to", "https://new.example.invalid",
            ], apply=True)
            with open(bindings, "rb") as handle:
                raw = handle.read()
            after = yaml.safe_load(raw)
        self.assertEqual(proc.returncode, 0, out)
        self.assertNotIn(b"\r\n", raw)
        self.assertEqual(after, expected)
        self.assertEqual(out["counts"]["applied_actions"], 4)

    def test_rebase_urls_never_matches_a_longer_host_with_the_same_prefix(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, encoding="utf-8") as handle:
                document = yaml.safe_load(handle)
            document["openproject_private_bindings"]["tasks"][2]["source_ref"]["url"] = (
                "https://example.invalid.other/work_packages/30"
            )
            with open(bindings, "w", encoding="utf-8", newline="\n") as handle:
                yaml.safe_dump(document, handle, sort_keys=False)
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "rebase-urls",
                "--from", "https://example.invalid/",
                "--to", "https://new.example.invalid",
            ])
        self.assertEqual(proc.returncode, 0, out)
        statuses = {item["replan_id"]: (item["status"], item["new_url"]) for item in out["items"]}
        self.assertEqual(statuses["task.after"], ("unchanged", "https://example.invalid.other/work_packages/30"))
        self.assertTrue(statuses["task.target"][1].startswith("https://new.example.invalid/work_packages/10"))

    def test_rebase_urls_also_rewrites_resource_urls_and_project_base_url(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, encoding="utf-8") as handle:
                document = yaml.safe_load(handle)
            layer = document["openproject_private_bindings"]
            layer["project"]["base_url"] = "https://example.invalid"
            layer["resources"].append({
                "replan_resource_id": "resource.person",
                "source_ref": {"source": "openproject", "object_type": "user",
                               "object_id": "4", "url": "https://example.invalid/users/4"},
            })
            with open(bindings, "w", encoding="utf-8", newline="\n") as handle:
                yaml.safe_dump(document, handle, sort_keys=False)
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "rebase-urls",
                "--from", "https://example.invalid",
                "--to", "https://new.example.invalid",
            ], apply=True)
            with open(bindings, encoding="utf-8") as handle:
                after = yaml.safe_load(handle)["openproject_private_bindings"]
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(after["project"]["base_url"], "https://new.example.invalid")
        self.assertEqual(after["resources"][1]["source_ref"]["url"], "https://new.example.invalid/users/4")
        self.assertEqual(after["resources"][0], {"replan_resource_id": "resource.keep", "extension": True})
        self.assertEqual(out["counts"]["changed_resource_bindings"], 1)
        self.assertTrue(out["counts"]["changed_project_base_url"])

    def test_rebase_urls_keep_base_url_dry_run_reports_kept_without_writing(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, encoding="utf-8") as handle:
                document = yaml.safe_load(handle)
            layer = document["openproject_private_bindings"]
            layer["project"]["base_url"] = "https://example.invalid/tunnel"
            with open(bindings, "w", encoding="utf-8", newline="\n") as handle:
                yaml.safe_dump(document, handle, sort_keys=False)
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "rebase-urls",
                "--from", "https://example.invalid",
                "--to", "https://new.example.invalid",
                "--keep-base-url",
            ])
            with open(bindings, "rb") as handle:
                after = handle.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(before, after)
        project_item = out["items"][0]
        self.assertEqual(project_item["status"], "kept")
        self.assertEqual(project_item["reason"], "keep_base_url_requested")
        self.assertEqual(project_item["old_url"], "https://example.invalid/tunnel")
        self.assertEqual(project_item["new_url"], "https://example.invalid/tunnel")
        self.assertEqual(out["counts"]["changed_bindings"], 4)
        self.assertEqual(out["counts"]["kept_bindings"], 1)
        self.assertFalse(out["counts"]["changed_project_base_url"])
        self.assertTrue(out["counts"]["kept_project_base_url"])
        self.assertTrue(all(
            item["new_url"].startswith("https://new.example.invalid")
            for item in out["items"] if item["status"] == "changed"
        ))

    def test_rebase_urls_keep_base_url_apply_changes_only_link_urls(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, encoding="utf-8") as handle:
                before = yaml.safe_load(handle)
            before["openproject_private_bindings"]["project"]["base_url"] = (
                "https://example.invalid/tunnel"
            )
            rendered = yaml.safe_dump(before, sort_keys=False, allow_unicode=True)
            with open(bindings, "wb") as handle:
                handle.write(rendered.replace("\n", "\r\n").encode())
            expected = copy.deepcopy(before)
            expected_layer = expected["openproject_private_bindings"]
            for binding in [*expected_layer["tasks"], *expected_layer["dependencies"]]:
                source = binding.get("source_ref") or {}
                url = source.get("url", "")
                if url.startswith("https://example.invalid"):
                    source["url"] = (
                        "https://new.example.invalid"
                        + url[len("https://example.invalid"):]
                    )
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "rebase-urls",
                "--from", "https://example.invalid",
                "--to", "https://new.example.invalid",
                "--keep-base-url",
            ], apply=True)
            with open(bindings, "rb") as handle:
                raw = handle.read()
            after = yaml.safe_load(raw)
        self.assertEqual(proc.returncode, 0, out)
        self.assertNotIn(b"\r\n", raw)
        self.assertEqual(after, expected)
        self.assertEqual(
            after["openproject_private_bindings"]["project"]["base_url"],
            "https://example.invalid/tunnel",
        )
        self.assertEqual(out["counts"]["applied_actions"], 4)

    def test_rebase_urls_keep_base_url_is_no_op_when_only_base_would_change(self):
        with tempfile.TemporaryDirectory() as project:
            bindings, fixture = self.make_plan(project)
            with open(bindings, encoding="utf-8") as handle:
                document = yaml.safe_load(handle)
            layer = document["openproject_private_bindings"]
            layer["project"]["base_url"] = "https://example.invalid/tunnel"
            for binding in [*layer["tasks"], *layer["dependencies"]]:
                source = binding.get("source_ref") or {}
                url = source.get("url", "")
                if url:
                    source["url"] = "https://other.invalid" + url[len("https://example.invalid"):]
            rendered = yaml.safe_dump(document, sort_keys=False, allow_unicode=True)
            with open(bindings, "wb") as handle:
                handle.write(rendered.replace("\n", "\r\n").encode())
            with open(bindings, "rb") as handle:
                before = handle.read()
            proc, out = self.run_toolkit(project, bindings, fixture, [
                "work-package", "rebase-urls",
                "--from", "https://example.invalid",
                "--to", "https://new.example.invalid",
                "--keep-base-url",
            ], apply=True)
            with open(bindings, "rb") as handle:
                after = handle.read()
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(before, after)
        self.assertTrue(out["no_op"])
        self.assertEqual(out["counts"]["changed_bindings"], 0)
        self.assertEqual(out["counts"]["kept_bindings"], 1)
        self.assertFalse(out["counts"]["changed_project_base_url"])
        self.assertTrue(out["counts"]["kept_project_base_url"])

    def test_rebase_urls_refuses_invalid_or_equal_urls(self):
        cases = [
            ("relative/path", "https://new.example.invalid"),
            ("https://old.example.invalid", "new.example.invalid"),
            ("https://same.example.invalid", "https://same.example.invalid"),
        ]
        for old_url, new_url in cases:
            with self.subTest(old_url=old_url, new_url=new_url):
                with tempfile.TemporaryDirectory() as project:
                    bindings, fixture = self.make_plan(project)
                    with open(bindings, "rb") as handle:
                        before = handle.read()
                    proc, out = self.run_toolkit(project, bindings, fixture, [
                        "work-package", "rebase-urls",
                        "--from", old_url, "--to", new_url,
                    ], apply=True)
                    with open(bindings, "rb") as handle:
                        after = handle.read()
                self.assertEqual(proc.returncode, 2, out)
                self.assertEqual(out["error_codes"], ["invalid_arguments"])
                self.assertEqual(before, after)

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
