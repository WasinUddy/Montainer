import os
import json
import argparse


def main():

    parser = argparse.ArgumentParser(description="Download Minecraft Bedrock Server from Mojang server.")
    parser.add_argument("--type", type=str, help="Specify the server type (stable or preview).")
    parser.add_argument("--path", type=str, help="Specify the path to the server directory.")

    args = parser.parse_args()

    # Set Path
    if args.path:
        path = args.path
    else:
        path = "/minecraft"

    # Load versions file
    with open(f"{path}/versions.json") as f:
        versions = json.load(f)

    

    if args.type=="stable":
        url = f"https://minecraft.azureedge.net/bin-linux/bedrock-server-{versions['stable']}.zip"
    else:
        url = f"https://minecraft.azureedge.net/bin-linux-preview/bedrock-server-{versions['preview']}.zip"
    
    print(f"Downloading {url}...")
    os.system(f"mkdir -p {path}/tmp")
    os.system(f"wget -O {path}/tmp/bedrock-server.zip " + url)
    print("Extracting...")
    os.system(f"unzip {path}/tmp/bedrock-server.zip -d {path}/minecraft_server")
    print("Cleaning up...")
    os.system(f"rm {path}/tmp/bedrock-server.zip")
    
    

if __name__ == "__main__":
    main()
