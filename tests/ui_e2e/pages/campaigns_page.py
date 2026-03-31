import time
from playwright.sync_api import Page, expect
from tests.ui_e2e.pages.base_page import BasePage


class CampaignsPage(BasePage):
    def navigate_to_tab(self):
        self.switch_tab("Campaigns")
        time.sleep(2)

    def click_create_campaign(self):
        self.page.locator('button:has-text("+ Create Campaign")').click()
        time.sleep(1)

    def fill_campaign_form(self, name, product_index=0, lead_source_index=0):
        """Fill the create-campaign form.

        Args:
            name: campaign name to type
            product_index: which option to pick from the product dropdown (0-based)
            lead_source_index: which option to pick from the lead source dropdown (0-based)
        """
        self.page.fill('input[placeholder*="Campaign"]', name)

        # Wait for product dropdown to have the requested option
        selects = self.page.locator(".modal-overlay select")
        if selects.count() > 0:
            # Wait until the select has enough options
            selects.nth(0).wait_for(state="visible", timeout=10000)
            self.page.wait_for_function(
                f"document.querySelectorAll('.modal-overlay select')[0]?.options.length > {product_index}",
                timeout=10000
            )
            selects.nth(0).select_option(index=product_index)
        if selects.count() > 1:
            selects.nth(1).select_option(index=lead_source_index)

        self.page.locator('.modal-overlay button.btn-primary:has-text("Create")').click()
        time.sleep(2)

    def get_campaign_row(self, name):
        return self.page.locator(f"tr:has-text('{name}'), div:has-text('{name}')")

    def click_view_leads(self, campaign_name):
        """Click 'View Leads' for a given campaign card."""
        card = self.page.locator(f"div.glass-panel:has-text('{campaign_name}')")
        card.locator("button:has-text('View Leads')").click()
        time.sleep(2)

    def quick_add_lead(self, name, phone):
        """Use the Quick Add form inside a campaign's leads view."""
        self.page.fill('input[placeholder*="Name"]', name)
        self.page.fill('input[placeholder*="Phone"]', phone)
        self.page.locator('button:has-text("Add & Assign")').click()
        time.sleep(2)

    def get_lead_row(self, phone):
        return self.page.locator(f"tr:has-text('{phone}')")

    def click_tab(self, tab_name):
        """Click a sub-tab inside campaign detail (e.g. 'Call Log')."""
        self.page.locator(f'button:has-text("{tab_name}")').first.click()
        time.sleep(1)
