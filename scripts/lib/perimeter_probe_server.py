"""A stand-in for a reverse proxy that asks API clients for a perimeter key.

Usage: python3 -m perimeter_probe_server LOG_FILE PORT_FILE

Listens on a free local port, written to PORT_FILE, and answers every request
with 401, as a perimeter does to a client it does not know. Each request is
logged as one line: the client's user agent, whether it carried
X-Perimeter-Key and an Authorization header, and the path. The key's value is
logged so the check can compare it; this server only ever sees a test key.
"""

import http.server
import sys


def main(argv):
    log_path, port_path = argv

    class Handler(http.server.BaseHTTPRequestHandler):
        def do_GET(self):
            with open(log_path, "a", encoding="utf-8") as log:
                log.write("agent=%s key=%s authorization=%s path=%s\n" % (
                    (self.headers.get("User-Agent") or "").split("/")[0],
                    self.headers.get("X-Perimeter-Key") or "-",
                    "yes" if self.headers.get("Authorization") else "no",
                    self.path.split("?")[0],
                ))
            body = b'{"_type":"Error","errorIdentifier":"urn:test:perimeter","message":"perimeter probe"}'
            self.send_response(401)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *_args):
            pass

    server = http.server.HTTPServer(("127.0.0.1", 0), Handler)
    with open(port_path, "w", encoding="utf-8") as handle:
        handle.write(str(server.server_address[1]))
    server.serve_forever()


if __name__ == "__main__":
    main(sys.argv[1:])
