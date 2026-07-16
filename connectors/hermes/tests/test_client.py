from __future__ import annotations

import json
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parents[1] / "provider"))

from client import CortexClient, CortexError  # noqa: E402


class RecordingHandler(BaseHTTPRequestHandler):
    requests = []
    response_status = 201

    def do_POST(self):
        length = int(self.headers.get("Content-Length", "0"))
        payload = json.loads(self.rfile.read(length) or b"{}")
        type(self).requests.append((self.path, dict(self.headers), payload))
        self.send_response(type(self).response_status)
        self.send_header("Content-Type", "application/json")
        self.end_headers()
        if type(self).response_status >= 400:
            self.wfile.write(b'{"error":{"message":"rejected"}}')
        else:
            self.wfile.write(b'{"id":"mem_1","lifecycle":"candidate"}')

    def log_message(self, format, *args):
        return None


class CortexClientTest(unittest.TestCase):
    def setUp(self):
        RecordingHandler.requests = []
        RecordingHandler.response_status = 201
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), RecordingHandler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.client = CortexClient(f"http://127.0.0.1:{self.server.server_port}", "secret-token")

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join(timeout=2)

    def test_remember_sends_auth_and_idempotency(self):
        result = self.client.remember({"content": "lesson"}, "request-1")
        self.assertEqual(result["id"], "mem_1")
        path, headers, payload = RecordingHandler.requests[0]
        self.assertEqual(path, "/v1/memories")
        self.assertEqual(headers["Authorization"], "Bearer secret-token")
        self.assertEqual(headers["Idempotency-Key"], "request-1")
        self.assertEqual(payload["content"], "lesson")

    def test_http_error_is_structured(self):
        RecordingHandler.response_status = 400
        with self.assertRaisesRegex(CortexError, "rejected"):
            self.client.remember({"content": "bad"})


if __name__ == "__main__":
    unittest.main()
