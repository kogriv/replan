import unittest

import openproject_plugin_rendered_overlay as rendered


def week(label, start, finish, capacity, planned):
    return {"label": label, "start_date": start, "finish_date": finish, "capacity": capacity, "planned": planned}


class AddOverloadCasesTest(unittest.TestCase):
    def test_plan_ending_in_the_status_week_gets_the_week_after(self):
        resource = {"id": "resource.a", "name": "A", "unit": "hours", "weeks": [
            week("2026-W43", "2026-10-19", "2026-10-25", 10, 4),
            week("2026-W44", "2026-10-26", "2026-10-30", 10, 2),
        ]}
        overlay = {
            "status_date": "2026-10-29",
            "span": {"start_date": "2026-10-19", "finish_date": "2026-10-30", "day_count": 12},
            "resources": [resource],
        }
        rendered.add_overload_cases(overlay)
        self.assertEqual(overlay["span"], {"start_date": "2026-10-19", "finish_date": "2026-11-08", "day_count": 21})
        added = resource["weeks"][-1]
        self.assertEqual(
            (added["label"], added["start_date"], added["finish_date"], added["planned"]),
            ("2026-W45", "2026-11-02", "2026-11-08", 12),
        )
        self.assertEqual(resource["weeks"][0]["planned"], 15)
        self.assertEqual(overlay["overloaded_weeks"][0]["week"], "2026-W45")

    def test_plan_with_work_ahead_is_not_extended(self):
        resource = {"id": "resource.a", "weeks": [
            week("2026-W43", "2026-10-19", "2026-10-25", 10, 4),
            week("2026-W45", "2026-11-02", "2026-11-08", 10, 2),
        ]}
        overlay = {"status_date": "2026-10-29", "resources": [resource]}
        rendered.add_overload_cases(overlay)
        self.assertEqual(len(resource["weeks"]), 2)
        self.assertEqual(overlay["overloaded_weeks"][0]["week"], "2026-W45")


if __name__ == "__main__":
    unittest.main()
