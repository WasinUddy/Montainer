import os
import argparse
import requests
import zipfile
import io

def main():
    parser = argparse.ArgumentParser(description="Download Minecraft Bedrock Server from Mojang server.")
    parser.add_argument("--type", type=str, help="Specify the server type (stable or preview).")
    args = parser.parse_args()

    # Load versions file
    with open(f"versions/{args.type}.txt", 'r') as f:
        version = f.read().strip()

    # Set URL
    url = f"https://www.minecraft.net/bedrockdedicatedserver/bin-linux{'' if args.type == 'stable' else '-preview'}/bedrock-server-{version}.zip"

    # Custom User-Agent to mimic a browser request
    headers = {
        "User-Agent": 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.93 Safari/537.36'
    }

    # Download and extract server
    print(f"Downloading {url}...")
    response = requests.get(url, headers=headers)
    response.raise_for_status()

    # Extract to cwd as bedrock_server
    print("Extracting...")
    with zipfile.ZipFile(io.BytesIO(response.content)) as z:
        z.extractall("bedrock_server")

    # Set executable permissions for the server file
    os.chmod(os.path.join("bedrock_server", "bedrock_server"), 0o777)

    print("Done.")

if __name__ == "__main__":
    main()
