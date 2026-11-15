#!/usr/bin/env python3
"""Minimal SSH runner for the OpsAGI lab box.

Usage: python ssh_run.py "<remote command>"
Reads OPSAGI_SSH_HOST / PORT / USER / PASSWORD from the environment.
"""
import os
import sys

import paramiko

host = os.environ.get("OPSAGI_SSH_HOST", "127.0.0.1")
port = int(os.environ.get("OPSAGI_SSH_PORT", "62154"))
user = os.environ.get("OPSAGI_SSH_USER", "root")
password = os.environ["OPSAGI_SSH_PASSWORD"]

command = sys.argv[1] if len(sys.argv) > 1 else "true"

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(
    hostname=host,
    port=port,
    username=user,
    password=password,
    timeout=20,
    banner_timeout=20,
    auth_timeout=20,
    look_for_keys=False,
    allow_agent=False,
)

stdin, stdout, stderr = client.exec_command(command, timeout=120)
out = stdout.read().decode("utf-8", "replace")
err = stderr.read().decode("utf-8", "replace")
code = stdout.channel.recv_exit_status()
client.close()

sys.stdout.write(out)
if err.strip():
    sys.stderr.write(err)
sys.exit(code)
