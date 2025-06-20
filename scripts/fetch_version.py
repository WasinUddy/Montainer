import asyncio
from playwright.async_api import async_playwright
from playwright_stealth import Stealth

MINECRAFT_DOWNLOAD_URL = 'https://www.minecraft.net/en-us/download/server/bedrock'

async def main():
    async with Stealth().use_async(async_playwright()) as p:
        browser = await p.chromium.launch(headless=True)
        page = await browser.new_page()
        await page.goto(MINECRAFT_DOWNLOAD_URL)

        # Get stable version link
        stable_link = await page.get_by_role("link", name="serverBedrockLinux").get_attribute("href")
        stable_version = stable_link.split('-')[-1][:-4]

        # Select the "Preview" radio button to reveal preview link
        await page.get_by_role("radio", name="Ubuntu (Linux) Preview").check()

        # Get preview version link
        preview_link = await page.get_by_role("link", name="serverBedrockPreviewLinux").get_attribute("href")
        preview_version = preview_link.split('-')[-1][:-4]

        # Output
        print(f"STABLE_VERSION={stable_version}")
        print(f"STABLE_URL={stable_link}")
        print(f"PREVIEW_VERSION={preview_version}")
        print(f"PREVIEW_URL={preview_link}")

        await browser.close()

if __name__ == "__main__":
    asyncio.run(main())
