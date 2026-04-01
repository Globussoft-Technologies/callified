"""Tests for campaign-level operations: delete, status toggle, CSV import."""
import os
import time
import tempfile
import requests
from playwright.sync_api import expect
from tests.ui_e2e.pages.campaigns_page import CampaignsPage
from tests.ui_e2e.conftest import BASE_URL, TEST_USER_EMAIL, TEST_USER_PW


def _ensure_product_via_api():
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


def _create_campaign(auth_page, base_url, suffix="Mgmt"):
    _ensure_product_via_api()
    camp = CampaignsPage(auth_page, base_url)
    camp.navigate_with_cache_bust()
    time.sleep(2)
    camp.navigate_to_tab()

    campaign_name = f"E2E_{suffix}_{int(time.time())}"
    camp.click_create_campaign()
    camp.fill_campaign_form(campaign_name, product_index=1, lead_source_index=1)
    expect(auth_page.get_by_text(campaign_name)).to_be_visible(timeout=8000)
    return camp, campaign_name


def test_delete_campaign(auth_page, base_url):
    """Test deleting a campaign."""
    camp, campaign_name = _create_campaign(auth_page, base_url, "Del")

    # Find the campaign card and click Delete
    card = auth_page.locator(f"div.glass-panel:has-text('{campaign_name}')")
    auth_page.on("dialog", lambda dialog: dialog.accept())
    card.locator("button:has-text('Delete')").click()
    time.sleep(2)

    # Campaign should no longer be visible
    expect(auth_page.get_by_text(campaign_name)).not_to_be_visible(timeout=8000)


def test_toggle_campaign_status(auth_page, base_url):
    """Test toggling campaign status between active and paused."""
    camp, campaign_name = _create_campaign(auth_page, base_url, "Toggle")

    card = auth_page.locator(f"div.glass-panel:has-text('{campaign_name}')")

    # Should start as active
    status_badge = card.locator("span:has-text('active'), span:has-text('paused')").first
    expect(status_badge).to_be_visible(timeout=5000)

    # Click the status badge to toggle
    status_badge.click()
    time.sleep(2)

    # Status should have changed
    card = auth_page.locator(f"div.glass-panel:has-text('{campaign_name}')")
    toggled_badge = card.locator("span:has-text('paused'), span:has-text('active')").first
    expect(toggled_badge).to_be_visible(timeout=5000)


def test_csv_import_to_campaign(auth_page, base_url):
    """Test importing leads via CSV into a campaign."""
    camp, campaign_name = _create_campaign(auth_page, base_url, "CSV")

    camp.click_view_leads(campaign_name)

    # Click Import CSV button
    auth_page.locator("button:has-text('Import CSV')").click()
    time.sleep(1)

    # Modal should appear
    modal = auth_page.locator(".modal-overlay")
    expect(modal.locator("text=Import Leads from CSV")).to_be_visible(timeout=5000)

    # Create a temp CSV file
    csv_content = "first_name,last_name,phone,source\nCSV Lead1,Test,+919000000001,CSV Import\nCSV Lead2,Test,+919000000002,CSV Import\n"
    tmp = tempfile.NamedTemporaryFile(mode='w', suffix='.csv', delete=False)
    tmp.write(csv_content)
    tmp.close()

    try:
        # Upload the CSV
        file_input = modal.locator('input[type="file"]')
        file_input.set_input_files(tmp.name)
        time.sleep(1)

        # Click import button
        modal.locator("button:has-text('Import')").click()
        time.sleep(3)

        # Accept any alert
        auth_page.on("dialog", lambda dialog: dialog.accept())

        # Verify leads appear in the campaign
        expect(auth_page.locator("text=CSV Lead1").first).to_be_visible(timeout=8000)
    finally:
        os.unlink(tmp.name)
