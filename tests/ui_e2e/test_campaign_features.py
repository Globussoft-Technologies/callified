import time
import requests
from playwright.sync_api import expect
from tests.ui_e2e.pages.campaigns_page import CampaignsPage
from tests.ui_e2e.conftest import BASE_URL, TEST_USER_EMAIL, TEST_USER_PW


def _ensure_product_via_api():
    """Create a test product via API using the E2E test user credentials."""
    r = requests.post(f"{BASE_URL}/api/auth/login", json={"email": TEST_USER_EMAIL, "password": TEST_USER_PW})
    if r.status_code != 200:
        return
    token = r.json().get("access_token")
    if not token:
        return
    headers = {"Authorization": f"Bearer {token}"}
    orgs = requests.get(f"{BASE_URL}/api/organizations", headers=headers).json()
    if not orgs:
        return
    org_id = orgs[0]["id"]
    products = requests.get(f"{BASE_URL}/api/organizations/{org_id}/products", headers=headers).json()
    if not products:
        requests.post(f"{BASE_URL}/api/organizations/{org_id}/products",
                      json={"name": "E2E Test Product"}, headers=headers)


def _create_and_open_campaign(auth_page, base_url):
    _ensure_product_via_api()

    camp = CampaignsPage(auth_page, base_url)
    camp.navigate_with_cache_bust()
    time.sleep(2)
    camp.navigate_to_tab()

    campaign_name = f"E2E_Feature_{int(time.time())}"
    camp.click_create_campaign()
    camp.fill_campaign_form(campaign_name, product_index=1, lead_source_index=1)
    expect(auth_page.get_by_text(campaign_name)).to_be_visible(timeout=8000)

    camp.click_view_leads(campaign_name)
    return camp


def test_campaign_call_log_tab(auth_page, base_url):
    """Test that the Call Log tab inside a campaign shows the expected table headers."""
    camp = _create_and_open_campaign(auth_page, base_url)
    camp.click_tab("Call Log")

    for header in ["Lead", "Phone", "Source", "Time", "Outcome", "Duration", "Recording"]:
        expect(auth_page.locator(f"th:has-text('{header}')").first).to_be_visible(timeout=8000)


def test_campaign_voice_settings(auth_page, base_url):
    """Test that the Campaign Voice Settings panel is visible."""
    camp = _create_and_open_campaign(auth_page, base_url)

    expect(auth_page.get_by_text("Campaign Voice Settings")).to_be_visible(timeout=8000)
    selects = auth_page.locator("select")
    assert selects.count() >= 3
