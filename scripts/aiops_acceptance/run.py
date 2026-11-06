"""Prove the product works end to end, with no host, no network and no real model.

    python scripts/aiops_acceptance/run.py

It starts the mock LLM, starts the built server against it, seeds an administrator,
runs scripts/opsagi_smoke.py against the flow that follows, and exits with the
smoke verdict. This is the sequence a CI job runs, written once so that the CI job
and a developer's terminal cannot drift apart.

The flow it creates answers from recorded evidence: the exit path, the schema
validation and the reporter are exercised, the SSH tools are not, and the run
says so when it passes.
"""

import os
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import http.client
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
BACKEND = ROOT / "backend"
SCRIPTS = ROOT / "scripts"

MAIL = "acceptance@opsagi.local"
PASSWORD = "Acceptance-Run-2026!"
SMOKE_TIMEOUT = 600


def free_port() -> int:
    with socket.socket() as sock:
        sock.bind(("127.0.0.1", 0))

        return sock.getsockname()[1]


def wait_for(port: int, timeout: float = 120) -> None:
    """Wait until the API answers anything at all.

    Any status counts: a 404 or a 405 says the router is up, which is all the
    readiness check needs, and the root path redirects instead of answering.
    """
    deadline = time.monotonic() + timeout

    while time.monotonic() < deadline:
        try:
            connection = http.client.HTTPConnection("127.0.0.1", port, timeout=2)
            connection.request("GET", "/api/v1/graphql")
            connection.getresponse().read()
            connection.close()

            return
        except OSError:
            time.sleep(1)

    raise SystemExit(f"the server did not answer within {timeout:.0f}s")


def node() -> str:
    found = shutil.which("node")

    if found is None:
        raise SystemExit("node is required for the mock LLM")

    return found


def build(binary: Path, package: str) -> None:
    print(f"[acceptance] building {package}", flush=True)
    subprocess.run(
        ["go", "build", "-o", str(binary), package], cwd=BACKEND, check=True, env=os.environ.copy()
    )


def main() -> int:
    server_port = free_port()
    mock_port = free_port()
    workdir = Path(tempfile.mkdtemp(prefix="opsagi-acceptance-"))
    server_bin = workdir / ("opsagi.exe" if os.name == "nt" else "opsagi")
    seed_bin = workdir / ("opsagi-seed.exe" if os.name == "nt" else "opsagi-seed")

    build(server_bin, "./cmd/opsagi")
    build(seed_bin, "./cmd/opsagi-seed")

    database_url = os.environ.get("DATABASE_URL", "")
    if not database_url:
        raise SystemExit("DATABASE_URL must point at a database with the pgvector extension")

    mock_log = (workdir / "mock-llm.log").open("w", encoding="utf-8")
    server_log = (workdir / "server.log").open("w", encoding="utf-8")

    mock = subprocess.Popen(
        [node(), str(SCRIPTS / "aiops_acceptance" / "mock-llm.mjs"), str(mock_port)],
        stdout=mock_log,
        stderr=subprocess.STDOUT,
    )

    env = os.environ.copy()
    env.update(
        {
            "COOKIE_SIGNING_SALT": "opsagi-acceptance-salt",
            "LLM_SERVER_KEY": "mock",
            "LLM_SERVER_MODEL": "mock-llm",
            "LLM_SERVER_PROVIDER": "",
            "LLM_SERVER_URL": f"http://127.0.0.1:{mock_port}/v1",
            # The commander refuses to run without a target, and this run never
            # probes one: the mock answers the exit tool on its first turn, so the
            # loopback target is a name, not an address that gets dialled.
            "OPSAGI_SUBTASK_CONCURRENCY": os.environ.get("OPSAGI_SUBTASK_CONCURRENCY", "1"),
            "OPSAGI_TARGETS": (
                '[{"id":"acceptance","host":"127.0.0.1","port":22,"user":"root",'
                '"password":"acceptance"}]'
            ),
            "PROMETHEUS_URL": "",
            "SERVER_PORT": str(server_port),
            "STATIC_DIR": "",
        }
    )

    server = subprocess.Popen([str(server_bin)], env=env, stdout=server_log, stderr=subprocess.STDOUT)

    try:
        wait_for(server_port, timeout=180)

        print("[acceptance] seeding the administrator", flush=True)
        subprocess.run(
            [str(seed_bin), "-mail", MAIL, "-name", "Acceptance", "-password", PASSWORD, "-reset"],
            cwd=ROOT,
            check=True,
            env=env,
            stdout=subprocess.DEVNULL,
        )

        smoke_env = env | {"OPSAGI_MAIL": MAIL, "OPSAGI_PASSWORD": PASSWORD}
        failures = []

        for target, intent in (
            ("diagnosis", "The acceptance host is reported as unhealthy. Diagnose it."),
            ("optimization", "The acceptance host feels slow; measure it (acceptance optimisation)."),
        ):
            print(f"[acceptance] smoke: {target}", flush=True)
            smoke = subprocess.run(
                [
                    sys.executable,
                    str(SCRIPTS / "opsagi_smoke.py"),
                    "--base",
                    f"http://127.0.0.1:{server_port}",
                    "--target",
                    target,
                    "--timeout",
                    str(SMOKE_TIMEOUT),
                    "--interval",
                    "5",
                    intent,
                ],
                cwd=ROOT,
                env=smoke_env,
            )

            if smoke.returncode != 0:
                failures.append(target)

        if failures:
            print(f"[acceptance] smoke failed; logs kept in {workdir}", flush=True)
            for name in ("server.log", "mock-llm.log"):
                print(f"--- {name} (tail) ---", flush=True)
                lines = (workdir / name).read_text(encoding="utf-8", errors="replace").splitlines()
                print("\n".join(lines[-40:]), flush=True)

            return 1

        print("[acceptance] PASS: flow, planner, both exits, validation and reporter", flush=True)

        return 0
    finally:
        for process in (server, mock):
            process.terminate()

        for process in (server, mock):
            try:
                process.wait(timeout=20)
            except subprocess.TimeoutExpired:
                process.kill()

        mock_log.close()
        server_log.close()


if __name__ == "__main__":
    sys.exit(main())
