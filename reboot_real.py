import paramiko
import os

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(host, username=username, password=password, timeout=10)

print("Hard restarting the authentic TEST environment service...")
command1 = f"cd ~/callified-ai && echo '{password}' | sudo -S systemctl daemon-reload && echo '{password}' | sudo -S systemctl restart callified-ai.service 2>&1"
stdin, stdout, stderr = client.exec_command(command1)
print(stdout.read().decode().strip())

print("Pulling authentic server logs for recordings...")
command2 = "grep -iE 'recording' ~/callified-ai/logs/uvicorn.error.log | tail -n 20"
stdin2, stdout2, stderr2 = client.exec_command(command2)
print("--- LATEST AUTHENTIC SERVER LOGS ---")
print(stdout2.read().decode().strip())

client.close()
