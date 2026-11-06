#!/usr/bin/env python3
"""Assert that a running backend turns one intent into one guarded exit.

    python scripts/opsagi_smoke.py --base http://127.0.0.1:8899 --target diagnosis

This is the regression the project did not have: every flow so far was watched by
a person reading the transcript, so "it worked yesterday" rested on memory. The
smoke run asserts the parts a person was checking by eye -- the flow settled, the
exit tool was called once and succeeded, its payload parses and carries the
fields the exit schema requires, and a report was written.

It runs against a real backend and a real model, so it is slow and it costs
tokens. The unit tier (`python scripts/test_opsagi_smoke.py`) exercises the same
checks against a stub and is what CI runs.

Exit code is 0 only when every check passed.
"""

import argparse
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import opsagi_run

EXIT_TOOL = {"diagnosis": "diagnosis", "optimization": "optimization_result"}
# A flow that has answered waits for the next question rather than closing, so
# 'waiting' is a settled flow too. Only 'failed' means it never got there.
SETTLED = ("finished", "waiting", "failed")
VERDICTS = ("improved", "no_measurable_effect", "degraded", "unverified")


def toolcalls(client, flow_id):
    payload = client.call("/api/v1/flows/{}/toolcalls/?page=1&pageSize=-1&type=init".format(flow_id)) or {}
    rows = payload.get("data") if isinstance(payload, dict) else payload
    if isinstance(rows, dict):
        rows = rows.get("toolcalls")
    if not isinstance(rows, list):
        return []

    # The list endpoint does not promise an order, and "the exit the flow ended
    # on" is a claim about the newest row, so the order is established here.
    calls = [row for row in rows if isinstance(row, dict)]
    calls.sort(key=lambda row: (row.get("id") or 0, row.get("created_at") or ""))

    return calls


def exits(calls, name):
    return [call for call in calls if call.get("name") == name]


def check_flow(status, calls, target):
    """The failures a reader would have found, in the order they matter."""
    failures = []
    name = EXIT_TOOL[target]

    if status == "failed":
        failures.append("flow failed")
    elif status not in SETTLED:
        failures.append("flow settled as {!r}, not finished or waiting".format(status))

    found = exits(calls, name)
    if not found:
        failures.append("no {} call: the flow ended without a guarded exit".format(name))
        return failures

    # A flow holds several tasks and a task's exit is its own end, so more than
    # one is legitimate. What matters is the one the flow ended on: an earlier
    # refusal that was repaired is the guard working, a last refusal is a task
    # that ended without saying anything.
    last = found[-1]
    if last.get("status") not in (None, "finished", "succeeded"):
        failures.append("the flow's last {} call ended as {!r}".format(name, last.get("status")))

    payload = read_payload(last)
    if payload is None:
        failures.append("the last {} call's args are not JSON: {}".format(name, (last.get("args") or "")[:200]))
        return failures

    return failures + (check_diagnosis(payload) if target == "diagnosis" else check_optimization(payload))


def read_payload(call):
    raw = call.get("args")
    if isinstance(raw, dict):
        return raw
    if not isinstance(raw, str) or not raw.strip():
        return None

    try:
        parsed = json.loads(raw)
    except ValueError:
        return None

    return parsed if isinstance(parsed, dict) else None


def check_evidence(payload, minimum=2):
    evidence = payload.get("evidence")
    if not isinstance(evidence, list):
        return ["evidence is not a list"]

    if len(evidence) < minimum:
        return ["evidence carries {} items, want at least {}".format(len(evidence), minimum)]

    failures = []
    for index, item in enumerate(evidence):
        if not isinstance(item, dict):
            failures.append("evidence[{}] is not an object".format(index))
            continue
        for field in ("source", "query", "timestamp", "excerpt"):
            if not str(item.get(field) or "").strip():
                failures.append("evidence[{}].{} is empty".format(index, field))

    return failures


def check_diagnosis(payload):
    failures = [
        "{} is empty".format(field)
        for field in ("root_cause", "blast_radius")
        if not str(payload.get(field) or "").strip()
    ]

    recommendation = payload.get("recommendation")
    if not isinstance(recommendation, dict) or not str(recommendation.get("action") or "").strip():
        failures.append("recommendation.action is empty")

    confidence = payload.get("confidence")
    if confidence is not None and not 0 <= float(confidence) <= 1:
        failures.append("confidence {} is outside 0..1".format(confidence))

    return failures + check_evidence(payload)


def check_optimization(payload):
    failures = [
        "{} is empty".format(field)
        for field in ("bottleneck", "baseline", "proposal", "expected_gain", "rollback")
        if not str(payload.get(field) or "").strip()
    ]

    verdict = str(payload.get("verdict") or "").strip()
    if verdict not in VERDICTS:
        failures.append("verdict {!r} is not one of {}".format(verdict, ", ".join(VERDICTS)))

    observed = str(payload.get("observed_gain") or "").strip()
    if verdict in ("improved", "no_measurable_effect", "degraded") and not observed:
        failures.append("verdict {} without an observed_gain: a measured verdict needs a number".format(verdict))
    if verdict == "unverified" and observed:
        failures.append("verdict unverified with an observed_gain: that is not a measurement")

    return failures + check_evidence(payload)


def wait_for(client, flow_id, timeout, interval):
    deadline = time.monotonic() + timeout
    status = client.flow_status(flow_id)["status"]

    while time.monotonic() < deadline:
        if status in SETTLED:
            return status
        time.sleep(interval)
        status = client.flow_status(flow_id)["status"]

    return status


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("intent", nargs="*", help="what to ask for; a default is used per target")
    parser.add_argument("--base", default=os.environ.get("OPSAGI_URL", "http://127.0.0.1:8080"))
    parser.add_argument("--mail", default=os.environ.get("OPSAGI_MAIL", "admin@opsagi.com"))
    parser.add_argument("--password", default=os.environ.get("OPSAGI_PASSWORD", ""))
    parser.add_argument("--token", default=os.environ.get("OPSAGI_TOKEN", ""))
    parser.add_argument("--provider", default=os.environ.get("OPSAGI_PROVIDER", "custom"))
    parser.add_argument("--target", choices=("diagnosis", "optimization"), default="diagnosis")
    parser.add_argument("--flow", type=int, default=0, help="check an existing flow instead of creating one")
    parser.add_argument("--timeout", type=int, default=1500, help="seconds to wait for the flow")
    parser.add_argument("--interval", type=float, default=10.0)
    args = parser.parse_args(argv)

    intent = " ".join(args.intent).strip() or DEFAULT_INTENT[args.target]

    client = opsagi_run.Client(args.base, token=args.token or None)
    if not args.token:
        if not args.password:
            raise SystemExit("set OPSAGI_PASSWORD (or --token) so the smoke run can log in")
        client.login(args.mail, args.password)

    started = time.monotonic()
    if args.flow:
        flow_id = args.flow
        print("checking flow {}".format(flow_id), flush=True)
    else:
        flow = client.create_flow(intent, args.provider, args.target)
        flow_id = flow["id"]
        print("flow {} created ({})".format(flow_id, flow["status"]), flush=True)

    status = wait_for(client, flow_id, args.timeout, args.interval)
    calls = toolcalls(client, flow_id)
    reports = client.reports(flow_id)

    failures = check_flow(status, calls, args.target)
    if status != "failed" and not reports:
        failures.append("no report message: the flow ended without saying anything")

    elapsed = int(time.monotonic() - started)
    print("flow {} {} after {}s, {} tool calls, {} reports".format(flow_id, status, elapsed, len(calls), len(reports)))

    if failures:
        for failure in failures:
            print("FAIL " + failure)
        return 1

    print("PASS {} flow {} produced a guarded exit with evidence".format(args.target, flow_id))
    return 0


DEFAULT_INTENT = {
    "diagnosis": "检查这台主机上 caddy 容器是否在运行、最近有没有重启过；只读，不要改动任何东西。"
    "最后必须用 diagnosis 收尾，evidence 至少两条。",
    "optimization": "测量这台主机最近 10 分钟的 CPU 与内存变化，找出瓶颈（acceptance optimisation）；"
    "只读，不要改动任何东西。最后必须用 optimization_result 收尾，evidence 至少两条。",
}


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
