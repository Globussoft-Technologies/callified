import os
import time
from playwright.sync_api import expect
from tests.ui_e2e.pages.auth_page import AuthPage


def test_signup_and_login(browser, base_url):
    """Test that a new user can sign up and land on the dashboard."""
    page = browser.new_page()
    auth_pg = AuthPage(page, base_url)

    email = os.getenv("E2E_USER_EMAIL", f"e2e_auth_{int(time.time())}@globussoft.com")
    pw = os.getenv("E2E_USER_PASSWORD", "E2eTestUser!2026")

    auth_pg.signup("E2E Org", "E2E Tester", email, pw)
    try:
        auth_pg.check_login_success()
    except Exception as e:
        page.screenshot(path="screenshot_auth_fail.png")
        print("\n--- PAGE HTML (first 3000 chars) ---")
        print(page.content()[:3000])
        raise e

    # Verify we see the Dashboard heading after login
    expect(page.locator("h2:has-text('Dashboard')")).to_be_visible(timeout=10000)
    page.close()


def test_logout(browser, base_url):
    """Test that a logged-in user can log out."""
    page = browser.new_page()
    auth_pg = AuthPage(page, base_url)

    email = f"e2e_logout_{int(time.time())}@globussoft.com"
    pw = "E2eTestUser!2026"

    auth_pg.signup("E2E Org", "E2E Tester", email, pw)
    auth_pg.check_login_success()

    auth_pg.logout()
    # Should be back on the login page
    expect(page.locator('input[type="email"]')).to_be_visible(timeout=5000)
    page.close()
