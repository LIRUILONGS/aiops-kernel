#!/usr/bin/env python3
"""Tests for scripts/opsagi_run.py, run against a stub HTTP server.

    python scripts/test_opsagi_run.py

No database, no LLM and no network: the stub answers the four endpoints the
runner touches, so what is pinned here is the runner's side of the contract --
that it declares the target type, that it polls until the flow settles, that it
prints the reports in the order they were written, and that its exit code says
whether the flow finished.
"""

import contextlib
import io
import json
import os
import sys
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest import mock

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import opsagi_run


class Stub(BaseHTTPRequestHandler):
    """Answers the runner and records what it asked for."""

    flow_statuses = ()
    reports = ()
    requests = []

    def log_message(self, *args):
        pass

    def _send(self, payload, status=200):
        body = json.dumps(payload).encode("utf-8")
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_POST(self):
        length = int(self.headers.get("Content-Length") or 0)
        payload = json.loads(self.rfile.read(length) or b"{}")
        Stub.requests.append((self.path, payload))

        if self.path.rstrip("/") == "/api/v1/auth/login":
            self._send({"status": "success"})
            return

        query = payload.get("query", "")
        if "createFlow" in query:
            self._send({"data": {"createFlow": {"id": 7, "status": "created"}}})
            return
        if "FlowStatus" in query:
            index = min(Stub.polls, len(Stub.flow_statuses) - 1)
            Stub.polls += 1
            self._send({"data": {"flow": {"id": 7, "status": Stub.flow_statuses[index]}}})
            return
        self._send({"errors": [{"message": "unexpected query"}]})

    def do_GET(self):
        Stub.requests.append((self.path, None))
        self._send({"data": {"msglogs": list(Stub.reports)}})


class RunnerTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(("127.0.0.1", 0), Stub)
        cls.base = "http://127.0.0.1:{}".format(cls.server.server_address[1])
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()

    def setUp(self):
        Stub.requests = []
        Stub.polls = 0
        Stub.flow_statuses = ("running", "finished")
        Stub.reports = [
            {"id": 2, "type": "report", "message": "plan", "result": "## 结论\n第二条"},
            {"id": 9, "type": "input", "message": "alert", "result": ""},
            {"id": 1, "type": "report", "message": "plan", "result": "## 结论\n第一条"},
        ]

    def test_create_flow_declares_the_target_type(self):
        client = opsagi_run.Client(self.base, token="t")

        client.create_flow("certificate renewal is failing", "custom", "optimization")

        _, payload = [r for r in Stub.requests if "/graphql" in r[0]][0]
        self.assertIn("flowType: $target", payload["query"])
        self.assertEqual(payload["variables"]["target"], "optimization")

    def test_main_prints_the_reports_in_order_and_returns_zero(self):
        with contextlib.redirect_stdout(io.StringIO()) as out:
            code = opsagi_run.main(
                ["--base", self.base, "--token", "t", "--interval", "0", "the alert"]
            )

        self.assertEqual(code, 0)
        reports = [r for r in Stub.requests if r[0].startswith("/api/v1/flows/7/msglogs")]
        self.assertTrue(reports, "the runner never read the report messages")
        printed = out.getvalue()
        self.assertLess(
            printed.index("第一条"),
            printed.index("第二条"),
            "the reports must print in the order the agents wrote them",
        )

    def test_main_returns_one_when_the_flow_failed(self):
        Stub.flow_statuses = ("failed",)

        with contextlib.redirect_stdout(io.StringIO()):
            code = opsagi_run.main(["--base", self.base, "--token", "t", "--interval", "0", "x"])

        self.assertEqual(code, 1)

    def test_reports_are_sorted_by_id_and_skip_rows_without_text(self):
        client = opsagi_run.Client(self.base, token="t")

        rows = client.reports(7)

        self.assertEqual([row["id"] for row in rows], [1, 2])

    def test_main_refuses_to_run_without_credentials(self):
        with contextlib.redirect_stdout(io.StringIO()), self.assertRaises(SystemExit) as raised:
            opsagi_run.main(["--base", self.base, "the alert"])

        self.assertIn("OPSAGI_PASSWORD", str(raised.exception))

    def test_main_says_the_flow_is_still_running_when_the_deadline_passes(self):
        Stub.flow_statuses = ("running",)

        with contextlib.redirect_stdout(io.StringIO()), self.assertRaises(SystemExit) as raised:
            opsagi_run.main(
                ["--base", self.base, "--token", "t", "--timeout", "0", "--interval", "0", "x"]
            )

        self.assertIn("did not finish", str(raised.exception))

    def test_graphql_errors_abort_with_the_server_message(self):
        client = opsagi_run.Client(self.base, token="t")

        with self.assertRaises(SystemExit) as raised:
            client.graphql("query Broken { nope }", {})

        self.assertIn("unexpected query", str(raised.exception))


if __name__ == "__main__":
    unittest.main()
