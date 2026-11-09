import http.server
import json
import os
import subprocess
import sys
import tempfile
import threading
import unittest


ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
LIB = os.path.join(ROOT, "scripts", "lib")


class WikiResponseNormalizationTest(unittest.TestCase):
    def test_text_line_endings_and_trailing_whitespace_are_applied(self):
        state = {"patches": []}

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

            def do_GET(self):
                self.reply({
                    "id": 1, "title": "Notes", "lockVersion": 3,
                    "text": {"raw": "Old"}, "_links": {},
                })

            def do_PATCH(self):
                length = int(self.headers.get("Content-Length", "0"))
                payload = json.loads(self.rfile.read(length) or b"{}")
                state["patches"].append(payload)
                raw = payload["text"]["raw"].replace("\n", "\r\n").rstrip()
                self.reply({
                    "id": 1, "title": "Notes", "lockVersion": 4,
                    "text": {"raw": raw}, "_links": {},
                })

        server = http.server.ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        threading.Thread(target=server.serve_forever, daemon=True).start()
        self.addCleanup(server.server_close)
        self.addCleanup(server.shutdown)
        with tempfile.TemporaryDirectory() as temp_dir:
            text_path = os.path.join(temp_dir, "notes.md")
            with open(text_path, "w", encoding="utf-8") as stream:
                stream.write("First line\nSecond line   \n")
            env = dict(
                os.environ,
                PYTHONPATH=LIB,
                REPLAN_OPENPROJECT_BASE_URL=(
                    f"http://127.0.0.1:{server.server_port}"
                ),
                REPLAN_OPENPROJECT_PROJECT_ID="demo",
                REPLAN_OPENPROJECT_TOKEN_ENV="TEST_TOKEN",
                TEST_TOKEN="test-token",
                REPLAN_APPLY="1",
            )
            proc = subprocess.run(
                [
                    sys.executable, "-m", "openproject_wiki", "page", "update",
                    "--id", "1", "--text-file", text_path,
                ],
                capture_output=True, text=True, env=env, timeout=30,
            )
        self.assertEqual(proc.stderr, "")
        out = json.loads(proc.stdout)
        self.assertEqual(proc.returncode, 0, out)
        self.assertTrue(out["actions"][0]["applied"])
        self.assertEqual(state["patches"][0]["lockVersion"], 3)


if __name__ == "__main__":
    unittest.main()
