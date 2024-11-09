from selenium import webdriver
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC

MINECRAFT_DOWNLOAD_URL = 'https://www.minecraft.net/en-us/download/server/bedrock'


def initialize_driver():
    """
    Initialize and return a headless Chrome WebDriver with custom options.
    """
    chrome_options = Options()
    chrome_options.add_argument("--headless=new")
    chrome_options.add_argument("--window-size=1920x1080")  # Set window size
    chrome_options.add_argument("user-agent=Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")
    chrome_options.add_argument("--no-sandbox")  # This is important in containerized environments
    chrome_options.add_argument("--disable-dev-shm-usage")  # Overcome limited resource problems
    chrome_options.add_argument("--remote-debugging-port=9222")
    return webdriver.Chrome(options=chrome_options)


def get_download_links(driver, url):
    """
    Navigate to the given URL and extract download links for stable and preview versions.
    """
    driver.get(url)
    wait = WebDriverWait(driver, 10)

    try:
        stable_link_element = wait.until(EC.presence_of_element_located((By.XPATH, '//a[contains(@class, "downloadlink") and contains(@href, "/bin-linux/")]')))
        preview_link_element = wait.until(EC.presence_of_element_located((By.XPATH, '//a[contains(@class, "downloadlink") and contains(@href, "/bin-linux-preview/")]')))

        stable_version = stable_link_element.get_attribute('href').split('-')[-1][:-4]
        preview_version = preview_link_element.get_attribute('href').split('-')[-1][:-4]

        return {
            'stable': {
                'version': stable_version,
                'url': stable_link_element.get_attribute('href')
            },
            'preview': {
                'version': preview_version,
                'url': preview_link_element.get_attribute('href')
            }
        }
    except Exception as e:
        print(f"Error occurred: {e}")
        return None


def main():
    driver = initialize_driver()
    try:
        versions = get_download_links(driver, MINECRAFT_DOWNLOAD_URL)

        if versions:
            print(f"STABLE_VERSION={versions['stable']['version']}")
            print(f"STABLE_URL={versions['stable']['url']}")
            print(f"PREVIEW_VERSION={versions['preview']['version']}")
            print(f"PREVIEW_URL={versions['preview']['url']}")
    finally:
        driver.quit()


if __name__ == "__main__":
    main()
