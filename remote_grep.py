import paramiko
import os

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(host, username=username, password=password, timeout=10)

print("--- EXOTEL DIAL LOGS ---")
_, stdout, _ = client.exec_command("cat ~/callified-ai/.env | grep PUBLIC_URL")
url = stdout.read().decode().strip()
print(f"Server PUBLIC_URL is: {url}")

print("--- Uvicorn Access Hits ---")
_, stdout, _ = client.exec_command("tail -n 200 ~/callified-ai/logs/uvicorn.access.log | grep -iE 'webhook|recording'")
print(stdout.read().decode().strip())

client.close()
