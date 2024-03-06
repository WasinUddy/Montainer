import os
import json
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
    url = f"https://minecraft.azureedge.net/bin-linux{"" if args.type=="stable" else "-preview"}/bedrock-server-{version}.zip"

    # Download and extract server
    print(f"Downloading {url}...")
    response = requests.get(url)
    response.raise_for_status()

    print("Extracting...")
    with zipfile.ZipFile(io.BytesIO(response.content)) as z:
        z.extractall(os.path.join("minecraft_server"))

    print("Setting permissions...")
    os.chmod(os.path.join("minecraft_server", "bedrock_server"), 0o777)

    # Create Base World Directory for Volume Mount
    os.makedirs(os.path.join("minecraft_server", "worlds", "Bedrock level"), exist_ok=True)

    print("Cleaning up...")


    print("Done.")
    

if __name__ == "__main__":
    main()