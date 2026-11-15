"""Run labelled log windows through a flow and collect what the agents concluded.

One case is one flow: the window is pasted into the intent, the flow runs with no
SSH target at all, and the diagnosis record is what gets scored. Cases run in
parallel because a flow is minutes long and the server handles several at once.
"""

import argparse
import concurrent.futures
import io
import json
import os
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import opsagi_run

PROMPT = """下面是一段从生产环境导出的日志（{lines} 行，已脱敏）。这台机器我没有 SSH，只有这段文本，任何"再去主机上查一下"的动作都做不了。

请判断：这段日志里是否存在需要立刻处理的问题？

- 如果有：指出是哪几行、属于什么类型的故障、已经或可能造成什么影响、下一步该做什么。
- 如果没有：直接说没有异常，不要为了凑一个结论而编造问题。

日志原文：

{log}
"""


def run_case(client, case, timeout):
    flow = client.create_flow(
        PROMPT.format(lines=case["lines"], log=case["log"]), "custom", "diagnosis"
    )
    flow_id = flow["id"]
    deadline = time.time() + timeout
    status = "created"
    while time.time() < deadline:
        time.sleep(15)
        row = client.flow_status(flow_id)
        status = row["status"]
        if status in ("finished", "failed", "waiting"):
            break

    return {
        "id": case["id"],
        "name": case["name"],
        "expect": case["expect"],
        "ground_truth": case["ground_truth"],
        "flow_id": flow_id,
        "status": status,
    }


def main(argv):
    parser = argparse.ArgumentParser()
    parser.add_argument("--cases", required=True)
    parser.add_argument("--base", default="http://127.0.0.1:8900")
    parser.add_argument("--mail", default="admin@opsagi.com")
    parser.add_argument("--password", default=os.environ.get("OPSAGI_ADMIN_PASSWORD", ""))
    parser.add_argument("--timeout", type=int, default=3600)
    parser.add_argument("--parallel", type=int, default=3)
    parser.add_argument("--out", default=r"D:\Temp\loghub\run1.json")
    args = parser.parse_args(argv)

    if not args.password:
        parser.error("pass --password or set OPSAGI_ADMIN_PASSWORD")

    cases = json.load(io.open(args.cases, encoding="utf-8"))

    client = opsagi_run.Client(args.base)
    client.login(args.mail, args.password)

    results = []
    with concurrent.futures.ThreadPoolExecutor(max_workers=args.parallel) as pool:
        futures = [pool.submit(run_case, client, case, args.timeout) for case in cases]
        for future in concurrent.futures.as_completed(futures):
            results.append(future.result())
            print(json.dumps(results[-1], ensure_ascii=False), flush=True)

    with io.open(args.out, "w", encoding="utf-8") as fh:
        json.dump(results, fh, ensure_ascii=False, indent=2)

    print("wrote", args.out)


if __name__ == "__main__":
    main(sys.argv[1:])
