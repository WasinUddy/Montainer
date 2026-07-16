import asyncio
import hashlib
from urllib.request import Request, urlopen

from playwright.async_api import async_playwright
from playwright_stealth import Stealth

MINECRAFT_DOWNLOAD_URL = 'https://www.minecraft.net/en-us/download/server/bedrock'
DOWNLOAD_USER_AGENT = (
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) '
    'AppleWebKit/537.36 Chrome/96.0.4664.93 Safari/537.36'
)


def artifact_sha256(url: str) -> str:
    digest = hashlib.sha256()
    request = Request(url, headers={"User-Agent": DOWNLOAD_USER_AGENT})
    with urlopen(request, timeout=600) as response:
        while chunk := response.read(1024 * 1024):
            digest.update(chunk)
    return digest.hexdigest()


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

        await browser.close()

        # Hash both exact artifacts concurrently. A changed checksum triggers
        # image acceptance even if Mojang reuses a versioned URL.
        stable_sha256, preview_sha256 = await asyncio.gather(
            asyncio.to_thread(artifact_sha256, stable_link),
            asyncio.to_thread(artifact_sha256, preview_link),
        )

        # Output
        print(f"STABLE_VERSION={stable_version}")
        print(f"STABLE_URL={stable_link}")
        print(f"STABLE_SHA256={stable_sha256}")
        print(f"PREVIEW_VERSION={preview_version}")
        print(f"PREVIEW_URL={preview_link}")
        print(f"PREVIEW_SHA256={preview_sha256}")


if __name__ == "__main__":
    asyncio.run(main())
