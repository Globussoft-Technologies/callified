import asyncio
from playwright.async_api import async_playwright
import os

async def main():
    os.makedirs("screenshots", exist_ok=True)
    async with async_playwright() as p:
        browser = await p.chromium.launch(headless=True)
        page = await browser.new_page(viewport={"width": 1440, "height": 900})
        
        print("Navigating to dashboard...")
        await page.goto("http://localhost:5173", wait_until="networkidle")
        await page.wait_for_selector(".dashboard-container")
        
        tabs = [
            ("📊 CRM", "1_crm_pipeline.png"),
            ("📋 Ops & Tasks", "2_ops_tasks.png"),
            ("📈 Analytics", "3_analytics.png"),
            ("💬 WhatsApp Comms", "4_whatsapp.png"),
            ("📍 Field Ops", "5_field_ops.png"),
            ("🔌 Integrations", "6_integrations.png")
        ]
        
        for text, filename in tabs:
            try:
                print(f"Clicking {text}...")
                await page.click(f"text='{text}'")
                await page.wait_for_timeout(1500) # Wait for state and graphs to render
                await page.screenshot(path=f"screenshots/{filename}")
                print(f"Captured {filename}")
            except Exception as e:
                print(f"Failed to capture {filename}: {e}")
                
        await browser.close()

if __name__ == "__main__":
    asyncio.run(main())
