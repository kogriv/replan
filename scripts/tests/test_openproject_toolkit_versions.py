import http.server
import json
import os
import subprocess
import sys
import threading
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class ToolkitVersionsTest(unittest.TestCase):
    def run_toolkit(self, base_url, args, *, apply=""):
        env = dict(
            os.environ,
            PYTHONPATH=LIB,
            REPLAN_PROJECT="",
            REPLAN_OPENPROJECT_BASE_URL=base_url,
            REPLAN_OPENPROJECT_PROJECT_ID="demo",
            REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
            TEST_TOKEN="test-token",
            REPLAN_OPENPROJECT_FROM_FILE="",
            REPLAN_OPENPROJECT_BINDINGS_PATH="",
            REPLAN_APPLY=apply,
        )
        proc = subprocess.run(
            [sys.executable, "-m", "openproject_work_package_toolkit",
             "--root-dir", ROOT, *args],
            capture_output=True, text=True, env=env, timeout=30,
        )
        self.assertEqual(proc.stderr, "")
        return proc, json.loads(proc.stdout)

    def state(self):
        return {
            "versions": [
                {"id": 11, "name": "Alpha", "status": "open", "sharing": "none"},
                {"id": 12, "name": "Duplicate", "status": "open", "sharing": "descendants"},
                {"id": 13, "name": "Duplicate", "status": "locked", "sharing": "hierarchy"},
            ],
            "work_packages": {
                "1": {
                    "subject": "One", "lockVersion": 3,
                    "estimatedTime": "PT1H", "_links": {
                        "version": {"href": "/api/v3/versions/10", "title": "Old"},
                    },
                },
            },
            "patches": [], "creates": [], "version_pages": [],
            "lock_conflict": False,
        }

    def serve(self, state):
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, *_args):
                pass

            def reply(self, body, status=200):
                data = json.dumps(body).encode()
                self.send_response(status)
                self.send_header("Content-Type", "application/hal+json")
                self.send_header("Content-Length", str(len(data)))
                self.end_headers()
                self.wfile.write(data)

            def body(self):
                length = int(self.headers.get("Content-Length", "0"))
                return json.loads(self.rfile.read(length) or b"{}")

            def version(self, item):
                return {
                    **item,
                    "_links": {"self": {
                        "href": f"/api/v3/versions/{item['id']}",
                        "title": item["name"],
                    }},
                }

            def work_package(self, work_package_id):
                return {"id": int(work_package_id), **state["work_packages"][work_package_id]}

            def do_GET(self):
                path, _, query = self.path.partition("?")
                if path == "/api/v3/projects/demo":
                    self.reply({"id": 9, "identifier": "demo"})
                    return
                if path == "/api/v3/projects/9/versions":
                    params = dict(item.split("=", 1) for item in query.split("&") if "=" in item)
                    offset = int(params.get("offset", "1"))
                    state["version_pages"].append(offset)
                    batch = state["versions"][offset - 1:offset]
                    self.reply({
                        "total": len(state["versions"]), "pageSize": 1,
                        "offset": offset,
                        "_embedded": {"elements": [self.version(item) for item in batch]},
                    })
                    return
                if path == "/api/v3/projects/9/work_packages":
                    elements = [self.work_package(wp_id) for wp_id in state["work_packages"]]
                    self.reply({"total": len(elements), "_embedded": {"elements": elements}})
                    return
                if path.startswith("/api/v3/work_packages/"):
                    self.reply(self.work_package(path.rsplit("/", 1)[-1]))
                    return
                self.reply({}, 404)

            def do_POST(self):
                payload = self.body()
                if self.path == "/api/v3/projects/9/work_packages/form":
                    self.reply({"_embedded": {"validationErrors": {}}})
                    return
                if self.path == "/api/v3/work_packages":
                    state["creates"].append(payload)
                    self.reply({"id": 2, "lockVersion": 1, **payload}, 201)
                    return
                self.reply({}, 404)

            def do_PATCH(self):
                payload = self.body()
                state["patches"].append(payload)
                if state["lock_conflict"]:
                    self.reply({
                        "errorIdentifier": "urn:openproject-org:api:v3:errors:UpdateConflict",
                        "message": "The resource was changed in the meantime.",
                    }, 409)
                    return
                work_package_id = self.path.rsplit("/", 1)[-1]
                item = state["work_packages"][work_package_id]
                version_link = (payload.get("_links") or {}).get("version")
                if version_link is not None:
                    href = version_link.get("href")
                    if href:
                        version_id = int(href.rsplit("/", 1)[-1])
                        version = next(value for value in state["versions"] if value["id"] == version_id)
                        item["_links"]["version"] = {"href": href, "title": version["name"]}
                    else:
                        item["_links"]["version"] = {"href": None}
                item["lockVersion"] += 1
                self.reply(self.work_package(work_package_id))

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        return f"http://127.0.0.1:{server.server_port}"

    def test_version_list_pages_and_reports_required_fields(self):
        state = self.state()
        proc, out = self.run_toolkit(self.serve(state), ["version", "list"])
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(state["version_pages"], [1, 2, 3])
        self.assertEqual(
            [{key: item[key] for key in ("id", "name", "status", "sharing")} for item in out["items"]],
            [
                {"id": "11", "name": "Alpha", "status": "open", "sharing": "none"},
                {"id": "12", "name": "Duplicate", "status": "open", "sharing": "descendants"},
                {"id": "13", "name": "Duplicate", "status": "locked", "sharing": "hierarchy"},
            ],
        )

    def test_update_sets_version_by_id_and_exact_name(self):
        for value, expected_id in (("11", "11"), ("Alpha", "11")):
            with self.subTest(value=value):
                state = self.state()
                proc, out = self.run_toolkit(self.serve(state), [
                    "work-package", "update-metadata", "--work-package", "1",
                    "--version", value,
                ], apply="1")
                self.assertEqual(proc.returncode, 0, out)
                self.assertEqual(state["patches"][0]["lockVersion"], 3)
                self.assertEqual(state["patches"][0]["_links"]["version"]["href"], "/api/v3/versions/11")
                self.assertEqual(out["actions"][0]["from_version"]["id"], "10")
                self.assertEqual(out["actions"][0]["to_version"]["id"], expected_id)

    def test_unknown_and_ambiguous_names_are_refused(self):
        cases = (("Missing", "version_not_found", []),
                 ("Duplicate", "version_ambiguous", ["12", "13"]))
        for value, code, ids in cases:
            with self.subTest(value=value):
                state = self.state()
                proc, out = self.run_toolkit(self.serve(state), [
                    "work-package", "update-metadata", "--work-package", "1",
                    "--version", value,
                ], apply="1")
                self.assertEqual(proc.returncode, 4, out)
                self.assertEqual(out["error_codes"], [code])
                self.assertEqual(state["patches"], [])
                if ids:
                    self.assertEqual(out["matching_version_ids"], ids)

    def test_clear_version(self):
        state = self.state()
        base_url = self.serve(state)
        args = [
            "work-package", "update-metadata", "--work-package", "1",
            "--clear-version",
        ]
        proc, out = self.run_toolkit(base_url, args)
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["mode"], "dry_run")
        self.assertEqual(out["actions"][0]["from_version"]["name"], "Old")
        self.assertEqual(out["actions"][0]["to_version"]["id"], "")
        self.assertEqual(state["patches"], [])

        proc, out = self.run_toolkit(base_url, args, apply="1")
        self.assertEqual(proc.returncode, 0, out)
        self.assertIsNone(state["patches"][0]["_links"]["version"]["href"])
        self.assertEqual(out["actions"][0]["from_version"]["name"], "Old")
        self.assertEqual(out["actions"][0]["to_version"]["id"], "")

    def test_create_version_dry_run_and_apply(self):
        state = self.state()
        base_url = self.serve(state)
        args = [
            "work-package", "create", "--subject", "Created",
            "--type-href", "/api/v3/types/1", "--version", "Alpha",
        ]
        proc, out = self.run_toolkit(base_url, args)
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(out["mode"], "dry_run")
        self.assertEqual(out["actions"][0]["to_version"]["id"], "11")
        self.assertEqual(state["creates"], [])

        proc, out = self.run_toolkit(base_url, args, apply="1")
        self.assertEqual(proc.returncode, 0, out)
        self.assertEqual(state["creates"][0]["_links"]["version"]["href"], "/api/v3/versions/11")

    def test_lock_version_conflict_is_reported_from_openproject(self):
        state = self.state()
        state["lock_conflict"] = True
        proc, out = self.run_toolkit(self.serve(state), [
            "work-package", "update-metadata", "--work-package", "1",
            "--version", "11",
        ], apply="1")
        self.assertEqual(proc.returncode, 6, out)
        self.assertEqual(out["error_codes"], ["openproject_http_error"])
        self.assertEqual(
            out["errors"][0]["response"]["errorIdentifier"],
            "urn:openproject-org:api:v3:errors:UpdateConflict",
        )


if __name__ == "__main__":
    unittest.main()
