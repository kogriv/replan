import unittest

import import_candidates


class ImportCandidatesTest(unittest.TestCase):
    def test_dependency_source_ids_prefer_source_neutral_fields(self):
        candidate = {
            "predecessor_source_id": "owner/repo#1",
            "successor_source_id": "owner/repo#2",
            "from_work_package_id": "legacy-from",
            "to_work_package_id": "legacy-to",
        }
        self.assertEqual(
            import_candidates.dependency_source_ids(candidate),
            ("owner/repo#1", "owner/repo#2"),
        )

    def test_dependency_source_ids_fall_back_to_openproject_direction(self):
        candidate = {
            "relation_type": "follows",
            "from_work_package_id": "2",
            "to_work_package_id": "1",
        }
        self.assertEqual(import_candidates.dependency_source_ids(candidate), ("1", "2"))

    def test_source_object_id_reads_new_and_old_formats(self):
        self.assertEqual(
            import_candidates.source_object_id(
                {"source_object_id": "owner/repo#3", "work_package_id": "legacy"}
            ),
            "owner/repo#3",
        )
        self.assertEqual(
            import_candidates.source_object_id({"work_package_id": "1234"}), "1234"
        )

    def test_relates_dependency_key_is_symmetric(self):
        self.assertEqual(
            import_candidates.dependency_key("relates", "task.b", "task.a"),
            ("relates", "task.a", "task.b"),
        )
        self.assertNotEqual(
            import_candidates.dependency_key("blocks", "task.b", "task.a"),
            import_candidates.dependency_key("blocks", "task.a", "task.b"),
        )


if __name__ == "__main__":
    unittest.main()
