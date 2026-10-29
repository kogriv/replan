"""Unit tests for the Python modules behind the shell helpers (GAP-018).

Run: PYTHONPATH=scripts/lib python3 -m unittest discover -s scripts/tests
"""

import glob
import json
import os
import py_compile
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

import yaml

import envelope_check
import openproject_http
import openproject_import_failure
import openproject_live_overlay
import replan_workload
import smoke_fixture_edits

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


def read(path):
    with open(path, encoding="utf-8") as handle:
        return handle.read()


def dump_yaml(path, doc):
    with open(path, "w", encoding="utf-8") as handle:
        yaml.safe_dump(doc, handle)


def load_yaml(path):
    with open(path, encoding="utf-8") as handle:
        return yaml.safe_load(handle)


def write(path, data):
    with open(path, "w", encoding="utf-8") as handle:
        handle.write(data if isinstance(data, str) else json.dumps(data))


class AuthHeadersTest(unittest.TestCase):
    def test_basic_token_auth_and_optional_headers(self):
        headers = openproject_http.openproject_auth_headers("tok", "op.example", "https")
        self.assertEqual(headers["Authorization"], "Basic YXBpa2V5OnRvaw==")
        self.assertEqual(headers["Content-Type"], "application/json")
        self.assertEqual(headers["Host"], "op.example")
        self.assertEqual(headers["X-Forwarded-Proto"], "https")

    def test_no_content_type_and_no_proxy_headers(self):
        headers = openproject_http.openproject_auth_headers("tok", content_type="")
        self.assertNotIn("Content-Type", headers)
        self.assertNotIn("Host", headers)
        self.assertNotIn("X-Forwarded-Proto", headers)

    def test_no_other_module_builds_the_authorization_header(self):
        offenders = []
        for path in glob.glob(os.path.join(LIB, "*.py")):
            if os.path.basename(path) in ("openproject_http.py", "wiki_api_throwaway_cases.py"):
                continue
            with open(path, encoding="utf-8") as handle:
                text = handle.read()
            if "b64encode" in text:
                offenders.append(os.path.basename(path))
        self.assertEqual(offenders, [])


class ErrorBodyTest(unittest.TestCase):
    def test_non_json_body_is_not_echoed(self):
        summary = openproject_http.summarize_error_body("<html>Private title</html>")
        self.assertEqual(summary, {"raw_omitted": True, "raw_length": 26})

    def test_openproject_error_keeps_identifier_and_messages_only(self):
        body = json.dumps({
            "_type": "Error",
            "errorIdentifier": "urn:openproject-org:api:v3:errors:PropertyConstraintViolation",
            "message": "Subject can't be blank.",
            "_embedded": {"errors": [{"message": "a", "details": {"attribute": "subject"}, "extra": "x"}]},
            "payload": {"subject": "private"},
        })
        summary = openproject_http.summarize_error_body(body)
        self.assertEqual(summary["message"], "Subject can't be blank.")
        self.assertEqual(summary["errors"], [{"message": "a"}])
        self.assertNotIn("payload", summary)

    def test_empty_and_unexpected_json(self):
        self.assertEqual(openproject_http.summarize_error_body(""), {})
        self.assertEqual(openproject_http.summarize_error_body("[1, 2]"), {"raw_omitted": True, "raw_length": 6})


class CollectPagesTest(unittest.TestCase):
    def test_offset_is_a_page_number(self):
        # 631 relations, as on a shared instance; pages of 250.
        items = [{"id": i} for i in range(631)]
        asked = []

        def fetch_page(offset, page_size):
            asked.append(offset)
            start = (offset - 1) * page_size
            batch = items[start:start + page_size]
            return {"total": len(items), "count": len(batch), "_embedded": {"elements": batch}}

        self.assertEqual(openproject_http.openproject_collect_pages(fetch_page, 250), items)
        self.assertEqual(asked, [1, 2, 3])

    def test_stops_on_an_empty_page_or_without_total(self):
        pages = {1: {"_embedded": {"elements": [{"id": 1}]}}}
        self.assertEqual(openproject_http.openproject_collect_pages(lambda o, s: pages.get(o, {}), 1), [{"id": 1}])
        empty = {"total": 5, "_embedded": {"elements": []}}
        self.assertEqual(openproject_http.openproject_collect_pages(lambda o, s: empty, 10), [])


class ReplanWorkloadTest(unittest.TestCase):
    """The weekly rows of the helpers come from `replan workload` (C-15)."""

    def fake_binary(self, directory, body):
        path = os.path.join(directory, "replan")
        write(path, "#!/usr/bin/env python3\nimport json, sys\nargs = sys.argv[1:]\n" + body)
        os.chmod(path, os.stat(path).st_mode | stat.S_IEXEC)
        return path

    def test_rows_with_load_only_and_the_week_asked(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self.fake_binary(tmp, (
                "week = args[args.index('--week') + 1]\n"
                "assert '--demands' in args\n"
                "rows = [{'resource_id': 'r.a', 'unit': 'hours', 'capacity': 5, 'planned': 2, 'reserved': 0, 'remaining': 2, 'actual': 0, 'overload': 0},\n"
                "        {'resource_id': 'r.idle', 'unit': 'hours', 'capacity': 5, 'planned': 0, 'reserved': 0, 'remaining': 0, 'actual': 0, 'overload': 0}]\n"
                "print(json.dumps({'ok': True, 'result': {'week': week, 'resources': rows}}))\n"))
            rows = replan_workload.weekly_rows(binary, tmp, ["2026-W43", "2026-W44"])
        self.assertEqual([(r["week"], r["resource_id"], r["planning_load"]) for r in rows], [("2026-W43", "r.a", 2), ("2026-W44", "r.a", 2)])
        self.assertEqual(rows[0]["week_start"], "2026-10-19")

    def test_a_failed_workload_raises_with_its_codes(self):
        with tempfile.TemporaryDirectory() as tmp:
            binary = self.fake_binary(tmp, "print(json.dumps({'ok': False, 'errors': [{'code': 'missing_required_file'}]}))\nsys.exit(2)\n")
            with self.assertRaisesRegex(replan_workload.WorkloadError, "missing_required_file"):
                replan_workload.workload_week(binary, tmp, "2026-W43")
            binary = self.fake_binary(tmp, "print('not json')\n")
            with self.assertRaisesRegex(replan_workload.WorkloadError, "no JSON"):
                replan_workload.workload_week(binary, tmp, "2026-W43")

    def test_plan_weeks_cover_tasks_and_demand_timings(self):
        with tempfile.TemporaryDirectory() as tmp:
            os.makedirs(os.path.join(tmp, "canonical"))
            dump_yaml(os.path.join(tmp, "canonical", "tasks.yaml"), {"tasks": [
                {"id": "t.a", "planned_start": "2026-10-21", "planned_finish": "2026-10-22"}, {"id": "t.undated"}]})
            dump_yaml(os.path.join(tmp, "canonical", "resource_demands.yaml"), {"resource_demands": [
                {"id": "d.a", "timing": {"latest_finish": "2026-11-03"}}]})
            self.assertEqual(replan_workload.plan_weeks(tmp), ["2026-W43", "2026-W44", "2026-W45"])

    @unittest.skipUnless(os.environ.get("REPLAN_BIN"), "needs REPLAN_BIN; the smoke passes it")
    def test_consecutive_one_day_tasks_on_one_slot_are_not_an_overload(self):
        for overlap, want in ((False, 0), (True, 1)):
            with tempfile.TemporaryDirectory() as tmp:
                project = os.path.join(tmp, "small-team")
                shutil.copytree(os.path.join(ROOT, "examples", "small-team"), project)
                tasks_path = os.path.join(project, "canonical", "tasks.yaml")
                demands_path = os.path.join(project, "canonical", "resource_demands.yaml")
                tasks, demands = load_yaml(tasks_path), load_yaml(demands_path)
                for task in tasks["tasks"]:
                    day = "2026-07-20" if task["id"] == "task.op.1234" or overlap else "2026-07-21"
                    task["planned_start"] = task["planned_finish"] = day
                    if task["id"] == "task.offline.repair-pump":
                        task["resource_demands"].append({"kind": "resource_demand", "id": "resource_demand.repair-pump.staging"})
                staging = dict(next(d for d in demands["resource_demands"] if d["id"] == "resource_demand.task-op-1234.staging"))
                staging.update(id="resource_demand.repair-pump.staging", task_ref={"kind": "task", "id": "task.offline.repair-pump"})
                demands["resource_demands"].append(staging)
                dump_yaml(tasks_path, tasks)
                dump_yaml(demands_path, demands)
                env = dict(os.environ, PYTHONPATH=LIB, REPLAN_PROJECT=project)
                drift = json.loads(subprocess.run([sys.executable, "-m", "check_resource_schedule_drift", "--replan-bin", os.environ["REPLAN_BIN"]],
                                                  capture_output=True, text=True, env=env, check=True).stdout)
                row = next(r for r in drift["weekly_rows"] if r["week"] == "2026-W30" and r["resource_id"] == "resource.environment.staging")
                self.assertEqual((row["planning_load"], row["capacity"], row["overload"], row["demands"]), (1 + want, 1, want, 2), row)
                explain = json.loads(subprocess.run(
                    [sys.executable, "-m", "explain_resource_overload", "--replan-bin", os.environ["REPLAN_BIN"]],
                    capture_output=True, text=True,
                    env=dict(env, REPLAN_RESOURCE_ID="resource.environment.staging", REPLAN_UNIT="concurrent_slots"),
                    check=True).stdout)
                self.assertEqual(explain["overload_status"], "overloaded" if overlap else "aligned")
                self.assertEqual(sorted(d["resource_demand_id"] for d in explain["weekly_rows"][0]["demands"]),
                                 ["resource_demand.repair-pump.staging", "resource_demand.task-op-1234.staging"])


class KitScriptsLoadSettingsTest(unittest.TestCase):
    def test_every_kit_script_loads_the_settings_file(self):
        # The kit README says every helper reads .env.local (finding C-05).
        build = read(os.path.join(ROOT, "scripts", "build-agent-kit.sh"))
        scripts = re.search(r'^SCRIPTS="\n(.*?)^"', build, re.M | re.S).group(1).split()
        self.assertGreater(len(scripts), 10)
        missing = [name for name in scripts
                   if 'replan_load_openproject_dotenv "$ROOT_DIR"' not in read(os.path.join(ROOT, "scripts", name))]
        self.assertEqual(missing, [])


class ImportFailureTest(unittest.TestCase):
    def test_envelope_with_bindings_reports_existence(self):
        with tempfile.TemporaryDirectory() as tmp:
            imp = os.path.join(tmp, "import.json")
            write(imp, {"errors": [{"code": "openproject_network_error"}], "result": {"warnings": [{"code": "w"}]}})
            bindings = os.path.join(tmp, "bindings.yaml")
            write(bindings, "x: 1\n")
            out = openproject_import_failure.envelope("check-openproject-drift", "drift_status", "failed", imp, "/p", "", "/t", bindings_path=bindings)
        self.assertFalse(out["ok"])
        self.assertEqual(out["drift_status"], "failed")
        self.assertEqual(out["private_binding_layer"], {"path": bindings, "loaded": False, "exists": True})
        self.assertEqual(out["summary"], {"warning_codes": ["w"], "error_codes": ["openproject_network_error"]})
        self.assertNotIn("outputs_dir", out)

    def test_envelope_with_profile_and_unreadable_import(self):
        out = openproject_import_failure.envelope("check-openproject-live-alignment", "live_alignment", "failed", "/missing.json", "/p", "1", "/t", profile="openproject.demo")
        self.assertEqual(out["profile"], "openproject.demo")
        self.assertNotIn("private_binding_layer", out)
        self.assertEqual(out["outputs_dir"], "/t")
        self.assertEqual(out["summary"], {"warning_codes": [], "error_codes": []})


class EnvelopeCheckTest(unittest.TestCase):
    def test_require_ok_and_min_changes(self):
        with tempfile.TemporaryDirectory() as tmp:
            ok = os.path.join(tmp, "ok.json")
            bad = os.path.join(tmp, "bad.json")
            write(ok, {"ok": True, "result": {"applied_changes": [1, 2]}})
            write(bad, {"ok": False, "errors": [{"code": "e"}]})
            self.assertEqual(envelope_check.require_ok(ok), 0)
            self.assertEqual(envelope_check.require_ok(bad), 1)
            self.assertEqual(envelope_check.require_ok(bad, with_warnings=True), 1)
            self.assertEqual(envelope_check.min_applied_changes(ok, 2, "x"), 0)
            self.assertEqual(envelope_check.min_applied_changes(ok, 3, "x"), 1)

    def test_import_summary(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "import.json")
            write(path, {"ok": True, "result": {"snapshot": {"id": "s"}, "task_candidates": [1, 2], "warnings": [{"code": "w"}]}})
            summary = envelope_check.import_summary(path)
        self.assertEqual(summary["snapshot"], "s")
        self.assertEqual(summary["task_candidates"], 2)
        self.assertEqual(summary["warning_codes"], ["w"])


class SmokeFixtureEditsTest(unittest.TestCase):
    def test_drop_binding_and_add_task(self):
        with tempfile.TemporaryDirectory() as tmp:
            src = os.path.join(tmp, "b.yaml")
            dst = os.path.join(tmp, "b2.yaml")
            dump_yaml(src, {"openproject_private_bindings": {"tasks": [{"replan_task_id": "a"}, {"replan_task_id": "b"}]}})
            smoke_fixture_edits.drop_task_binding(src, dst, "a")
            self.assertEqual([t["replan_task_id"] for t in load_yaml(dst)["openproject_private_bindings"]["tasks"]], ["b"])
            tasks = os.path.join(tmp, "tasks.yaml")
            plan = os.path.join(tmp, "plan.yaml")
            dump_yaml(tasks, {"tasks": []})
            dump_yaml(plan, {"current_plan": {"included_tasks": []}})
            smoke_fixture_edits.add_binding_task(tasks, plan)
            self.assertEqual(load_yaml(tasks)["tasks"][0]["id"], "task.self.smoke-binding-helper")
            self.assertEqual(len(load_yaml(plan)["current_plan"]["included_tasks"]), 1)


class LiveOverlayTest(unittest.TestCase):
    def test_binding_counts_do_not_rewrite_tasks(self):
        with tempfile.TemporaryDirectory() as tmp:
            tasks = os.path.join(tmp, "tasks.yaml")
            dump_yaml(tasks, {"tasks": [
                {"id": "t1", "source_refs": [{"source": "openproject", "object_id": "2101"}, {"source": "github", "object_id": "9"}]},
                {"id": "t2", "source_refs": [{"source": "openproject", "object_id": "2102"}]},
            ]})
            with open(tasks, "rb") as fh:
                before = fh.read()
            bound, unbound = openproject_live_overlay.binding_counts(tasks, {"t1": {"source": "openproject", "object_type": "work_package", "object_id": "83", "url": "u"}})
            with open(tasks, "rb") as fh:
                after = fh.read()
        self.assertEqual((bound, unbound), (1, ["t2"]))
        self.assertEqual(after, before)

    def test_build_calls_cli_with_project_and_bindings_directly(self):
        with tempfile.TemporaryDirectory() as tmp:
            project = os.path.join(tmp, "project")
            output = os.path.join(tmp, "outside", "dashboard")
            os.makedirs(os.path.join(project, "canonical"))
            os.makedirs(os.path.join(output, "raw"))
            dump_yaml(os.path.join(project, "canonical", "tasks.yaml"), {"tasks": [{"id": "t1"}]})
            bindings = os.path.join(tmp, "bindings.yaml")
            dump_yaml(bindings, {"openproject_private_bindings": {"tasks": [{
                "replan_task_id": "t1",
                "source_ref": {"source": "openproject", "object_type": "work_package", "object_id": "83"},
            }], "dependencies": [], "resources": []}})
            with open(os.path.join(output, "raw", "openproject_overlay.json"), "w", encoding="utf-8") as fh:
                json.dump({"work_package_count": 1, "linked_task_count": 1, "missing_link_count": 0}, fh)
            completed = subprocess.CompletedProcess([], 0, stdout="{}", stderr="")
            with mock.patch.object(openproject_live_overlay.subprocess, "run", return_value=completed) as run:
                result = openproject_live_overlay.build("/bin/replan", project, bindings, output, "2026-W44")
        self.assertTrue(result["ok"])
        self.assertEqual(run.call_args.args[0], [
            "/bin/replan", "report", "dashboard", "--project", project,
            "--bindings", bindings, "--output", output, "--format", "json", "--week", "2026-W44",
        ])


class ModuleInventoryTest(unittest.TestCase):
    def test_every_module_compiles_and_every_helper_module_is_invoked(self):
        scripts = "\n".join(read(p) for p in glob.glob(os.path.join(ROOT, "scripts", "*.sh")))
        imported = "\n".join(read(p) for p in glob.glob(os.path.join(LIB, "*.py")))
        orphans = []
        for path in sorted(glob.glob(os.path.join(LIB, "*.py"))):
            py_compile.compile(path, doraise=True)
            name = os.path.splitext(os.path.basename(path))[0]
            invoked = re.search(r"python3 -m %s\b" % re.escape(name), scripts)
            used = re.search(r"^(from %s import|import %s\b)" % (name, name), imported, re.M)
            if not invoked and not used and name != "wiki_api_throwaway_cases":
                orphans.append(name)
        self.assertEqual(orphans, [])

    def test_no_python_heredocs_left_in_scripts(self):
        left = []
        for path in glob.glob(os.path.join(ROOT, "scripts", "*.sh")) + glob.glob(os.path.join(ROOT, "scripts", "lib", "*.sh")):
            if re.search(r"python3 -[^m].*<<|<<-?\s*'PY'", read(path)):
                left.append(os.path.basename(path))
        self.assertEqual(left, [])


if __name__ == "__main__":
    unittest.main()
