#!/usr/bin/env python3
"""Create a flow from one line of intent and print the conclusion.

    python scripts/opsagi_run.py "orders-api 从 10:20 起 502，帮我定位根因"

The flow is created through the same GraphQL mutation the UI uses, then polled
until it finishes, at which point every report message the agents wrote is
printed in order. Exit code is 0 only when the flow reached `finished`.

Authentication is a normal login: pass OPSAGI_MAIL and OPSAGI_PASSWORD, or use an
API token with --token (it is sent as a Bearer header).
"""

import argparse
import http.cookiejar
import json
import os
import sys
import time
import urllib.error
import urllib.request

CREATE_FLOW = """
mutation CreateFlow($input: String!, $provider: String!, $target: FlowType) {
  createFlow(modelProvider: $provider, input: $input, flowType: $target) { id status }
}
"""

FLOW_STATUS = """
query FlowStatus($id: ID!) {
  flow(flowId: $id) { id status title }
}
"""


class Client:
    def __init__(self, base, token=None):
        self.base = base.rstrip("/")
        self.token = token
        self.opener = urllib.request.build_opener(
            urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar())
        )

    def call(self, path, payload=None, method=None):
        url = self.base + path
        data = None
        headers = {"Accept": "application/json"}
        if payload is not None:
            data = json.dumps(payload, ensure_ascii=False).encode("utf-8")
            headers["Content-Type"] = "application/json"
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        request = urllib.request.Request(url, data=data, headers=headers, method=method)
        try:
            with self.opener.open(request, timeout=60) as response:
                body = response.read().decode("utf-8")
        except urllib.error.HTTPError as err:
            body = err.read().decode("utf-8", "replace")
            raise SystemExit(
                "{} {} -> HTTP {}: {}".format(method or "GET", path, err.code, body[:400])
            )
        if not body.strip():
            return None
        try:
            return json.loads(body)
        except json.JSONDecodeError:
            raise SystemExit("{} {} did not return JSON: {}".format(method or "GET", path, body[:400]))

    def login(self, mail, password):
        self.call("/api/v1/auth/login", {"mail": mail, "password": password}, "POST")

    def graphql(self, query, variables):
        envelope = self.call("/api/v1/graphql", {"query": query, "variables": variables}, "POST")
        if envelope.get("errors"):
            raise SystemExit("graphql refused the call: " + json.dumps(envelope["errors"], ensure_ascii=False))
        return envelope.get("data") or {}

    def create_flow(self, intent, provider, target=None):
        data = self.graphql(
            CREATE_FLOW, {"input": intent, "provider": provider, "target": target}
        )
        return data["createFlow"]

    def flow_status(self, flow_id):
        return self.graphql(FLOW_STATUS, {"id": str(flow_id)})["flow"]

    def reports(self, flow_id):
        """Report messages in the order the agents wrote them.

        The endpoint pages its rows and wraps them twice ({"data": {"msglogs":
        [...]}}); the bare-list form is accepted too so a reshape does not break
        the runner silently.
        """
        payload = self.call(
            "/api/v1/flows/{}/msglogs/?page=1&pageSize=-1&type=init".format(flow_id)
        ) or {}
        rows = payload.get("data") if isinstance(payload, dict) else payload
        if isinstance(rows, dict):
            rows = rows.get("msglogs")
        if not isinstance(rows, list):
            return []
        rows = [row for row in rows if row.get("type") == "report"]
        rows.sort(key=lambda row: row.get("id", 0))
        return [row for row in rows if (row.get("result") or row.get("message"))]


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("intent", nargs="*", help="the alert or optimisation goal, in one sentence")
    parser.add_argument("--base", default=os.environ.get("OPSAGI_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--mail", default=os.environ.get("OPSAGI_MAIL", "admin@opsagi.com"))
    parser.add_argument("--password", default=os.environ.get("OPSAGI_PASSWORD", ""))
    parser.add_argument("--token", default=os.environ.get("OPSAGI_TOKEN", ""))
    parser.add_argument("--provider", default=os.environ.get("OPSAGI_PROVIDER", "custom"))
    parser.add_argument(
        "--target",
        choices=("diagnosis", "optimization"),
        default=os.environ.get("OPSAGI_TARGET_TYPE") or None,
        help="declare the flow's target type; the plan may label a subtask differently, "
        "but an explicit type outranks the label",
    )
    parser.add_argument("--timeout", type=int, default=1800, help="seconds to wait for the flow")
    parser.add_argument("--interval", type=float, default=5.0, help="seconds between polls")
    parser.add_argument("--flow", type=int, default=0, help="watch an existing flow instead of creating one")
    parser.add_argument("--json", action="store_true", help="print the raw flow row and reports")
    args = parser.parse_args(argv)

    client = Client(args.base, token=args.token or None)
    if not args.token:
        if not args.password:
            raise SystemExit("set OPSAGI_PASSWORD (or --token) so the runner can log in")
        client.login(args.mail, args.password)

    if args.flow:
        flow_id, status = args.flow, client.flow_status(args.flow)["status"]
        print("watching flow {} ({})".format(flow_id, status), flush=True)
    else:
        intent = " ".join(args.intent).strip() or sys.stdin.read().strip()
        if not intent:
            raise SystemExit("nothing to diagnose: pass the alert as an argument or on stdin")
        flow = client.create_flow(intent, args.provider, args.target)
        flow_id, status = flow["id"], flow["status"]
        print("flow {} created ({})".format(flow_id, status), flush=True)

    deadline = time.monotonic() + args.timeout
    while time.monotonic() < deadline:
        status = client.flow_status(flow_id)["status"]
        if status in ("finished", "failed"):
            break
        time.sleep(args.interval)
    else:
        raise SystemExit(
            "flow {} did not finish within {}s (still {}); open the UI to follow it".format(
                flow_id, args.timeout, status
            )
        )

    reports = client.reports(flow_id)
    if args.json:
        print(json.dumps({"flow": client.flow_status(flow_id), "reports": reports}, ensure_ascii=False, indent=2))
    else:
        for report in reports:
            print("\n" + "-" * 72 + "\n")
            print(report.get("result") or report.get("message"))

    print("\nflow {} {}".format(flow_id, status))
    return 0 if status == "finished" else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
