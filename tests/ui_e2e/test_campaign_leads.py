"""Tests for lead management within campaigns: edit, status change, remove, note, transcript."""
import random
import time
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


def _create_campaign_with_lead(auth_page, base_url):
    """Create a campaign, add a lead, return (CampaignsPage, campaign_name, phone)."""
    _ensure_product_via_api()
    camp = CampaignsPage(auth_page, base_url)
    camp.navigate_with_cache_bust()
    time.sleep(2)
    camp.navigate_to_tab()

    campaign_name = f"E2E_Leads_{int(time.time())}"
    camp.click_create_campaign()
    camp.fill_campaign_form(campaign_name, product_index=1, lead_source_index=1)
    expect(auth_page.get_by_text(campaign_name)).to_be_visible(timeout=8000)

    camp.click_view_leads(campaign_name)
    test_phone = f"+91{random.randint(1000000000, 9999999999)}"
    camp.quick_add_lead("E2E LeadTest", test_phone)
    expect(camp.get_lead_row(test_phone)).to_be_visible(timeout=8000)
    return camp, campaign_name, test_phone


def test_edit_lead_in_campaign(auth_page, base_url):
    """Test editing a lead's details from within a campaign."""
    camp, _, test_phone = _create_campaign_with_lead(auth_page, base_url)

    # Click Edit on the lead row
    row = camp.get_lead_row(test_phone)
    row.locator("button:has-text('Edit')").click()
    time.sleep(1)

    # Edit modal should appear
    modal = auth_page.locator(".modal-overlay")
    expect(modal.locator("text=Edit Lead")).to_be_visible(timeout=5000)

    # Change the first name
    first_name_input = modal.locator("input.form-input").first
    first_name_input.fill("E2E Edited")

    modal.locator("button.btn-primary:has-text('Save')").click()
    time.sleep(2)

    # Verify updated name appears
    expect(auth_page.get_by_text("E2E Edited")).to_be_visible(timeout=8000)


def test_change_lead_status(auth_page, base_url):
    """Test changing a lead's status dropdown within a campaign."""
    camp, _, test_phone = _create_campaign_with_lead(auth_page, base_url)

    row = camp.get_lead_row(test_phone)
    status_select = row.locator("select.form-input")
    expect(status_select).to_be_visible(timeout=5000)

    # Change status to Contacted
    status_select.select_option("Contacted")
    time.sleep(2)

    # Verify the dropdown now shows Contacted
    expect(status_select).to_have_value("Contacted")


def test_remove_lead_from_campaign(auth_page, base_url):
    """Test removing a lead from a campaign."""
    camp, _, test_phone = _create_campaign_with_lead(auth_page, base_url)

    row = camp.get_lead_row(test_phone)

    # Accept the confirmation dialog
    auth_page.on("dialog", lambda dialog: dialog.accept())
    row.locator("button:has-text('Remove')").click()
    time.sleep(2)

    # Lead row should no longer be visible
    expect(camp.get_lead_row(test_phone)).not_to_be_visible(timeout=8000)


def test_lead_note_modal(auth_page, base_url):
    """Test opening and saving a note for a lead."""
    camp, _, test_phone = _create_campaign_with_lead(auth_page, base_url)

    row = camp.get_lead_row(test_phone)
    row.locator("button:has-text('Note')").click()
    time.sleep(1)

    # Note modal should appear
    modal = auth_page.locator(".modal-overlay")
    expect(modal.locator("text=Quick Note")).to_be_visible(timeout=5000)

    # Type a note
    textarea = modal.locator("textarea")
    textarea.fill("E2E test follow-up note")

    # Save
    modal.locator("button.btn-primary").click()
    time.sleep(2)

    # Modal should close after successful save
    expect(auth_page.locator(".modal-overlay:has-text('Quick Note')")).not_to_be_visible(timeout=8000)


def test_transcript_button_shows_no_calls(auth_page, base_url):
    """Test that a fresh lead shows 'No Calls' for transcripts."""
    camp, _, test_phone = _create_campaign_with_lead(auth_page, base_url)

    row = camp.get_lead_row(test_phone)
    expect(row.locator("button:has-text('No Calls')")).to_be_visible(timeout=5000)
