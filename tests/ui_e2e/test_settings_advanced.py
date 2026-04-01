"""Tests for advanced settings: product persona, website scrape, prompt generation."""
import time
import requests
from playwright.sync_api import expect
from tests.ui_e2e.pages.base_page import BasePage
from tests.ui_e2e.conftest import BASE_URL, TEST_USER_EMAIL, TEST_USER_PW


def _ensure_product_with_notes():
    """Create a product with manual notes so persona generation can work."""
    r = requests.post(f"{BASE_URL}/api/auth/login", json={"email": TEST_USER_EMAIL, "password": TEST_USER_PW})
    if r.status_code != 200:
        return
    token = r.json().get("access_token")
    if not token:
        return
    headers = {"Authorization": f"Bearer {token}", "Content-Type": "application/json"}
    orgs = requests.get(f"{BASE_URL}/api/organizations", headers=headers).json()
    if not orgs:
        return
    org_id = orgs[0]["id"]
    products = requests.get(f"{BASE_URL}/api/organizations/{org_id}/products", headers=headers).json()
    if not products:
        resp = requests.post(f"{BASE_URL}/api/organizations/{org_id}/products",
                             json={"name": "E2E Settings Product"}, headers=headers)
        product_id = resp.json().get("id")
    else:
        product_id = products[0]["id"]
    # Add manual notes so generate-persona works
    requests.put(f"{BASE_URL}/api/products/{product_id}",
                 json={"manual_notes": "AI-powered sales dialer. Helps businesses automate outbound calls with AI voice agents."},
                 headers=headers)


def _navigate_to_settings(auth_page, base_url):
    base = BasePage(auth_page, base_url)
    base.navigate_with_cache_bust()
    time.sleep(2)
    base.switch_tab("Settings")
    time.sleep(2)


def test_product_persona_section_visible(auth_page, base_url):
    """Test that Agent Persona & Call Flow section is expandable."""
    _ensure_product_with_notes()
    _navigate_to_settings(auth_page, base_url)

    # Click expand button
    expand_btn = auth_page.locator("button:has-text('Agent Persona & Call Flow')").first
    expect(expand_btn).to_be_visible(timeout=8000)
    expand_btn.click()
    time.sleep(1)

    # Persona and call flow textareas should appear
    expect(auth_page.locator("text=Agent Persona").first).to_be_visible(timeout=5000)
    expect(auth_page.locator("text=Call Flow Instructions").first).to_be_visible(timeout=5000)


def test_save_product_persona(auth_page, base_url):
    """Test saving agent persona and call flow for a product."""
    _ensure_product_with_notes()
    _navigate_to_settings(auth_page, base_url)

    # Expand persona section
    expand_btn = auth_page.locator("button:has-text('Agent Persona & Call Flow')").first
    expand_btn.click()
    time.sleep(1)

    # Fill persona
    persona_textarea = auth_page.locator("textarea").nth(0)
    persona_textarea.fill("E2E Test Persona — You are Arjun, a friendly sales agent.")

    # Fill call flow
    flow_textarea = auth_page.locator("textarea").nth(1)
    flow_textarea.fill("Step 1: Greet\nStep 2: Qualify\nStep 3: Book appointment")

    # Save
    save_btn = auth_page.locator("button:has-text('Save Persona')").first
    save_btn.click()
    time.sleep(2)

    # Reload and verify saved
    _navigate_to_settings(auth_page, base_url)
    auth_page.locator("button:has-text('Agent Persona & Call Flow')").first.click()
    time.sleep(1)

    persona_textarea = auth_page.locator("textarea").nth(0)
    import re
    expect(persona_textarea).to_have_value(re.compile("E2E Test Persona"), timeout=5000)


def test_generate_persona_button_visible(auth_page, base_url):
    """Test that Auto-Generate button appears when product has notes."""
    _ensure_product_with_notes()
    _navigate_to_settings(auth_page, base_url)

    expand_btn = auth_page.locator("button:has-text('Agent Persona & Call Flow')").first
    expand_btn.click()
    time.sleep(1)

    # Generate button should be visible since product has manual_notes
    gen_btn = auth_page.locator("button:has-text('Auto-Generate Persona')").first
    expect(gen_btn).to_be_visible(timeout=5000)


def test_system_prompt_section_visible(auth_page, base_url):
    """Test that the system prompt section is visible in settings."""
    _navigate_to_settings(auth_page, base_url)

    expect(auth_page.locator("text=AI System Prompt").first).to_be_visible(timeout=8000)
    expect(auth_page.locator("text=Custom System Prompt").first).to_be_visible(timeout=8000)
