import random
import time
import requests
from playwright.sync_api import expect
from tests.ui_e2e.pages.campaigns_page import CampaignsPage
from tests.ui_e2e.conftest import BASE_URL, TEST_USER_EMAIL, TEST_USER_PW


def _ensure_product_via_api():
    """Create a test product via API using the E2E test user credentials."""
    # Login
    r = requests.post(f"{BASE_URL}/api/auth/login", json={"email": TEST_USER_EMAIL, "password": TEST_USER_PW})
    if r.status_code != 200:
        return  # user doesn't exist yet, conftest will create it
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


def test_create_campaign(auth_page, base_url):
    """Test creating a new campaign via the Campaigns tab."""
    _ensure_product_via_api()

    camp = CampaignsPage(auth_page, base_url)
    camp.navigate_with_cache_bust()
    time.sleep(2)
    camp.navigate_to_tab()

    campaign_name = f"E2E_Camp_{int(time.time())}"
    camp.click_create_campaign()
    camp.fill_campaign_form(campaign_name, product_index=1, lead_source_index=1)

    expect(auth_page.get_by_text(campaign_name)).to_be_visible(timeout=8000)


def test_add_lead_to_campaign(auth_page, base_url):
    """Test adding a lead inside a campaign via the Quick Add form."""
    _ensure_product_via_api()

    camp = CampaignsPage(auth_page, base_url)
    camp.navigate_with_cache_bust()
    time.sleep(2)
    camp.navigate_to_tab()

    campaign_name = f"E2E_LeadCamp_{int(time.time())}"
    camp.click_create_campaign()
    camp.fill_campaign_form(campaign_name, product_index=1, lead_source_index=1)
    expect(auth_page.get_by_text(campaign_name)).to_be_visible(timeout=8000)

    camp.click_view_leads(campaign_name)

    test_phone = f"+91{random.randint(1000000000, 9999999999)}"
    camp.quick_add_lead("E2E TestLead", test_phone)
    expect(camp.get_lead_row(test_phone)).to_be_visible(timeout=8000)


def test_campaign_stats(auth_page, base_url):
    """Test that campaign stats cards are visible after opening a campaign."""
    _ensure_product_via_api()

    camp = CampaignsPage(auth_page, base_url)
    camp.navigate_with_cache_bust()
    time.sleep(2)
    camp.navigate_to_tab()

    campaign_name = f"E2E_StatsCamp_{int(time.time())}"
    camp.click_create_campaign()
    camp.fill_campaign_form(campaign_name, product_index=1, lead_source_index=1)
    expect(auth_page.get_by_text(campaign_name)).to_be_visible(timeout=8000)

    camp.click_view_leads(campaign_name)

    expect(auth_page.locator("text=Total Leads").first).to_be_visible(timeout=8000)
    expect(auth_page.locator("text=Called").first).to_be_visible(timeout=8000)
    expect(auth_page.locator("text=Qualified").first).to_be_visible(timeout=8000)
    expect(auth_page.locator("text=Appointments").first).to_be_visible(timeout=8000)
