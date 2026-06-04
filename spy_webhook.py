import paramiko
import os

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

c = paramiko.SSHClient()
c.set_missing_host_key_policy(paramiko.AutoAddPolicy())
c.connect(host, username=username, password=password, timeout=10)

print("Checking remote recordings folder...")
_, stdout, _ = c.exec_command("find /home/empcloud-development/callified-ai/recordings/ -type f")
print(stdout.read().decode().strip())

print("Checking raw server logs for ANY webhook hit...")
_, stdout, _ = c.exec_command("grep -A 2 -B 2 -i '/webhook/exotel' /home/empcloud-development/callified-ai/logs/*.log | tail -n 50")
print(stdout.read().decode().strip())

c.close()
