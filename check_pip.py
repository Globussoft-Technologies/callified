import paramiko
import os

host = os.environ.get("DEPLOY_HOST", "163.227.174.141")
username = os.environ.get("DEPLOY_USER", "empcloud-development")
password = os.environ.get("DEPLOY_PASSWORD")

print("--- EXECUTING NATIVE VENV MODULE CHECK ---")
client = paramiko.SSHClient()
client.set_missing_host_key_policy(paramiko.AutoAddPolicy())
client.connect(host, username=username, password=password, timeout=10)

cmd = "/home/empcloud-development/callified-ai/venv/bin/python -c \"import faiss; import sentence_transformers; print('DEPENDENCIES_OK')\""
_, stdout, stderr = client.exec_command(cmd)

out = stdout.read().decode().strip()
err = stderr.read().decode().strip()

print("STDOUT:", out)
print("STDERR:", err)

if "DEPENDENCIES_OK" in out:
    print("--- RESTARTING SYSTEMD SERVICE ---")
    client.exec_command("echo {password} | sudo -S systemctl restart callified-ai.service")
    import time
    time.sleep(2)
    print("--- SERVICE RESTARTED ---")

client.close()
