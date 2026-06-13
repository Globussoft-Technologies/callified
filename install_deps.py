import paramiko
import os

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

print("--- EXECUTING NATIVE PIP INSTALL ---")
client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(host, username=username, password=password, timeout=10)

cmd = "cd /home/empcloud-development/callified-ai && /home/empcloud-development/callified-ai/venv/bin/pip install -r requirements.txt"
_, stdout, stderr = client.exec_command(cmd)

import time
time.sleep(3) # allow pip initialization buffer

print("STDOUT PIP:", stdout.read().decode().strip())
print("STDERR PIP:", stderr.read().decode().strip())

print("\n--- RESTARTING SYSTEMD SERVICE ---")
client.exec_command("echo {password} | sudo -S systemctl restart callified-ai.service")

print("\n--- DEPENDENCY DEPLOYMENT FINISHED ---")
client.close()
