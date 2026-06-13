import paramiko
import os

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

print("--- EXECUTING NATIVE VITE BUILD SCRIPT ---")
client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(host, username=username, password=password, timeout=10)

print("Running npm install and npm run build...")
_, stdout, stderr = client.exec_command("cd /home/empcloud-development/callified-ai/frontend && npm install && npm run build")

import time
time.sleep(3)

print("STDOUT:", stdout.read().decode().strip())
print("STDERR:", stderr.read().decode().strip())

client.exec_command("echo {password} | sudo -S systemctl restart callified-ai.service")

print("\n--- FRONTEND BUILD DEPLOYED ---")
client.close()
