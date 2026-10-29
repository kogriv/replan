"""Contract tests for the shared private OpenProject binding layer."""

import os
import tempfile
import unittest
from unittest import mock

import yaml

import openproject_bindings


class OpenProjectBindingsTest(unittest.TestCase):
    def write_yaml(self, path, document):
        with open(path, "w", encoding="utf-8") as handle:
            yaml.safe_dump(document, handle, sort_keys=False)

    def test_load_keeps_unknown_keys_and_defaults_lists(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "bindings.yaml")
            self.write_yaml(path, {
                "document_extension": {"keep": True},
                "openproject_private_bindings": {
                    "project": {"project_id": "demo"},
                    "tasks": None,
                    "extension": ["keep"],
                },
            })
            document = openproject_bindings.load(path)
        self.assertEqual(document["document_extension"], {"keep": True})
        self.assertEqual(openproject_bindings.root(document)["extension"], ["keep"])
        self.assertEqual(openproject_bindings.root(document)["tasks"], [])
        self.assertEqual(openproject_bindings.root(document)["dependencies"], [])
        self.assertEqual(openproject_bindings.root(document)["resources"], [])

    def test_maps_normalize_object_ids_and_filter_refs(self):
        document = {"openproject_private_bindings": {
            "project": {"project_id": 42},
            "tasks": [
                {"replan_task_id": "task.a", "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": 101}},
                {"replan_task_id": "task.bad-source", "source_ref": {"source": "other", "object_type": "work_package", "object_id": 102}},
                {"replan_task_id": "task.no-id", "source_ref": {"source": "openproject", "object_type": "work_package"}},
                {"replan_task_id": "task.zero", "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": 0}},
            ],
            "dependencies": [
                {"replan_dependency_id": "dep.a", "source_ref": {"source": "openproject", "object_type": "relation", "object_id": 501}},
                {"replan_dependency_id": "dep.bad-type", "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": 502}},
            ],
            "resources": [],
        }}
        task_to_wp, wp_to_task = openproject_bindings.task_maps(document)
        dep_to_relation, relation_to_dep = openproject_bindings.dependency_maps(document)
        self.assertEqual(task_to_wp, {"task.a": "101", "task.zero": "0"})
        self.assertEqual(wp_to_task, {"101": "task.a", "0": "task.zero"})
        self.assertEqual(dep_to_relation, {"dep.a": "501"})
        self.assertEqual(relation_to_dep, {"501": "dep.a"})
        self.assertEqual(openproject_bindings.project(document), {"project_id": 42})

    def test_permissive_task_refs_keep_overlay_normalization(self):
        document = {"openproject_private_bindings": {
            "tasks": [{"replan_task_id": "task.a", "source_ref": {"object_id": 9}}],
            "dependencies": [], "resources": [],
        }}
        self.assertEqual(openproject_bindings.task_maps(document), ({}, {}))
        self.assertEqual(openproject_bindings.task_source_refs(document, strict=False), {
            "task.a": {"source": "openproject", "object_type": "work_package", "object_id": "9", "url": ""},
        })

    def test_missing_file_optional_and_required_modes(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "missing.yaml")
            self.assertEqual(openproject_bindings.load(path), openproject_bindings.empty_document())
            with self.assertRaisesRegex(FileNotFoundError, f"private binding file not found: {path}"):
                openproject_bindings.load(path, required=True)

    def test_append_and_replace_keep_unknown_keys_and_refresh_timestamp(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "bindings.yaml")
            original = {"openproject_private_bindings": {
                "generated_at": "old", "tasks": [],
                "dependencies": [{"replan_dependency_id": "old"}],
                "resources": [], "extension": {"keep": True},
            }}
            self.write_yaml(path, original)
            openproject_bindings.append_task_binding(path, "task.a", {"object_id": "1"})
            openproject_bindings.replace_dependency_bindings(path, [{"replan_dependency_id": "dep.a"}])
            document = openproject_bindings.load(path)
        layer = openproject_bindings.root(document)
        self.assertEqual(layer["extension"], {"keep": True})
        self.assertEqual(layer["tasks"], [{"replan_task_id": "task.a", "source_ref": {"object_id": "1"}}])
        self.assertEqual(layer["dependencies"], [{"replan_dependency_id": "dep.a"}])
        self.assertNotEqual(layer["generated_at"], "old")

    def test_atomic_write_leaves_old_file_when_dump_fails(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "bindings.yaml")
            with open(path, "wb") as handle:
                handle.write(b"old contents\n")
            with mock.patch.object(openproject_bindings.yaml, "safe_dump", side_effect=RuntimeError("dump failed")):
                with self.assertRaisesRegex(RuntimeError, "dump failed"):
                    openproject_bindings.write(path, openproject_bindings.empty_document())
            with open(path, "rb") as handle:
                self.assertEqual(handle.read(), b"old contents\n")
            self.assertEqual(os.listdir(tmp), ["bindings.yaml"])


if __name__ == "__main__":
    unittest.main()
