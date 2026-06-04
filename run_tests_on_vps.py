import paramiko
import os
import time

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

print(f"Connecting to {host}...")
client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(host, username=username, password=password, timeout=10)

commands = [
    "cd /home/empcloud-development/callified-ai && git pull && source venv/bin/activate && pip install pytest pytest-asyncio pytest-cov && python -m pytest tests/ -v --cov=. --cov-report=term-missing > cov.txt 2>&1; tail -n 50 cov.txt"
]

for cmd in commands:
    print(f"\n--- EXEC: {cmd} ---")
    stdin, stdout, stderr = client.exec_command(cmd)
    # wait for command to complete
    exit_status = stdout.channel.recv_exit_status()
    
    out = stdout.read().decode()
    err = stderr.read().decode()
    if out:
        print("STDOUT:\n" + out)
    if err:
        print("STDERR:\n" + err)
    print(f"EXIT CODE: {exit_status}")

client.close()
