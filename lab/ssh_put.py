#!/usr/bin/env python3
"""Upload a local file to the OpsAGI lab box over SFTP.

Usage: python ssh_put.py <local_path> <remote_path>
"""
import os
import sys

import paramiko

host = os.environ.get("OPSAGI_SSH_HOST", "127.0.0.1")
port = int(os.environ.get("OPSAGI_SSH_PORT", "62154"))
user = os.environ.get("OPSAGI_SSH_USER", "root")
password = os.environ["OPSAGI_SSH_PASSWORD"]

local_path, remote_path = sys.argv[1], sys.argv[2]

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

remote_dir = os.path.dirname(remote_path)
if remote_dir:
    client.exec_command(f"mkdir -p {remote_dir}")[1].channel.recv_exit_status()

sftp = client.open_sftp()
sftp.put(local_path, remote_path)
sftp.close()
client.close()
print(f"uploaded {local_path} -> {remote_path}")
