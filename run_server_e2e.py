import os
import subprocess

if __name__ == "__main__":
    print("🌍 Triggering SERVER E2E Test Suite...")
    env = os.environ.copy()
    env["E2E_TARGET_ENV"] = "server"
    env["E2E_BASE_URL"] = "https://test.callified.ai"
    
    subprocess.run(["python", "-m", "pytest", "tests/e2e/", "-v"], env=env)
