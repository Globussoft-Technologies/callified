import paramiko
import os

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(host, username=username, password=password, timeout=10)

stdin, stdout, stderr = client.exec_command(
    "grep -E 'main\\.py|routes\\.py|tts\\.py|ws_handler\\.py|database\\.py' /home/empcloud-development/callified-ai/cov.txt"
)
print(stdout.read().decode())
client.close()
