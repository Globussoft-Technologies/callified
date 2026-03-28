import os
import subprocess

if __name__ == "__main__":
    print("🚀 Triggering LOCAL E2E Test Suite...")
    env = os.environ.copy()
    env["E2E_TARGET_ENV"] = "local"
    env["E2E_BASE_URL"] = "http://localhost:8000"
    
    # Ensuring virtual env python executable triggers pytest generically
    subprocess.run(["python", "-m", "pytest", "tests/e2e/", "-v"], env=env)
