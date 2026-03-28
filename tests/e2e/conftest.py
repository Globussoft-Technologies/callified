import os
import pytest
import requests

@pytest.fixture(scope="session")
def remote_client():
    class RemoteTestClient:
        def __init__(self):
            # Fallback to local if not specified by runner
            self.base_url = os.environ.get("E2E_BASE_URL", "http://localhost:8000")
            self.session = requests.Session()
            
            # Bootstrap Authentication Sequence
            self._authenticate_e2e_runner()
            
        def _authenticate_e2e_runner(self):
            login_payload = {
                "email": "e2e_runner_admin@testing.com",
                "password": "e2e_secure_password"
            }
            # Try to login first
            res = self.session.post(self._fix_url("/api/auth/login"), json=login_payload)
            
            if res.status_code != 200:
                # If login fails (user doesn't exist), sign them up natively to test environment
                signup_payload = {
                    "org_name": "E2E_Automated_Testing_Org",
                    "full_name": "E2E Test Agent",
                    "email": login_payload["email"],
                    "password": login_payload["password"]
                }
                res = self.session.post(self._fix_url("/api/auth/signup"), json=signup_payload)
                if res.status_code != 200:
                    raise Exception(f"E2E Authentication Bootstrap Failed: {res.text}")
            
            # Inject Authorization Bearer into all future session requests
            token = res.json().get("access_token")
            self.session.headers.update({"Authorization": f"Bearer {token}"})

        def _fix_url(self, url):
            return self.base_url + url if url.startswith("/") else url

        def get(self, url, **kwargs):
            return self.session.get(self._fix_url(url), **kwargs)

        def post(self, url, **kwargs):
            return self.session.post(self._fix_url(url), **kwargs)

        def put(self, url, **kwargs):
            return self.session.put(self._fix_url(url), **kwargs)

        def delete(self, url, **kwargs):
            return self.session.delete(self._fix_url(url), **kwargs)
            
    return RemoteTestClient()
