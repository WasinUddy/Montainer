import os
import json
import argparse
import requests
import zipfile
import io

def main():
    parser = argparse.ArgumentParser(description="Download Minecraft Bedrock Server from Mojang server.")
    parser.add_argument("--type", type=str, help="Specify the server type (stable or preview).")
    parser.add_argument("--path", type=str, help="Specify the path to the server directory.")

    args = parser.parse_args()

    # Set Path
    path = args.path if args.path else "/app"

    # Load versions file
    with open(os.path.join(path, "versions.json")) as f:
        versions = json.load(f)

    # Set URL
    url = f"https://minecraft.azureedge.net/bin-linux/bedrock-server-{versions[args.type]}.zip" if args.type in versions else None

    if url:
        print(f"Downloading {url}...")
        response = requests.get(url)
        response.raise_for_status()

        print("Extracting...")
        with zipfile.ZipFile(io.BytesIO(response.content)) as z:
            z.extractall(os.path.join(path, "minecraft_server"))

        print("Setting permissions...")
        os.chmod(os.path.join(path, "minecraft_server", "bedrock_server"), 0o777)


        print("Done.")
    else:
        print(f"Invalid server type: {args.type}. Please specify 'stable' or 'preview'.")

if __name__ == "__main__":
    main()