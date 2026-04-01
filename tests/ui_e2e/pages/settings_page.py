import time
from playwright.sync_api import expect
from tests.ui_e2e.pages.base_page import BasePage


class SettingsPage(BasePage):
    def go_to_settings(self):
        self.switch_tab("Settings")

    def add_product(self, product_name):
        self.page.locator("button:has-text('+ Add Product')").click()
        product_input = self.page.locator('input[placeholder*="Product name"]')
        product_input.wait_for(timeout=5000)
        product_input.fill(product_name)
        product_input.press("Enter")

    def delete_product(self, product_name):
        # handleDeleteProduct uses confirm() dialog — auto-accept it
        self.page.once("dialog", lambda dialog: dialog.accept())
        # Product name is now in an editable input field
        name_input = self.page.locator(f"input[value='{product_name}']")
        # The Remove button is in the same parent flex container
        remove_btn = name_input.locator("xpath=ancestor::div[contains(@style,'justify-content')]//button[contains(., 'Remove')]")
        remove_btn.click()
        self.page.wait_for_load_state("networkidle")

    def add_pronunciation_rule(self, word, phonetic):
        self.page.fill('input[placeholder="e.g. Adsgpt"]', word)
        self.page.fill('input[placeholder="e.g. Ads G P T"]', phonetic)
        self.page.locator("button:has-text('+ Add Rule')").click()

    def delete_pronunciation_rule(self, word):
        row = self.page.locator(f"tr:has-text('{word}')")
        row.locator("button:has-text('Remove')").click()
