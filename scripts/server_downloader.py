import os
import json
import argparse


def main():
    # Load versions file
    with open('/minecraft/scripts/versions.json') as f:
        versions = json.load(f)


    parser = argparse.ArgumentParser(description='Download Minecraft Bedrock Server from Mojang server.')
    parser.add_argument('--type', type=str, help='Specify the server type (stable or preview).')

    args = parser.parse_args()

    if args.type=='stable':
        url = f'https://minecraft.azureedge.net/bin-linux/bedrock-server-{versions["stable"]}.zip'
    else:
        url = f'https://minecraft.azureedge.net/bin-linux-preview/bedrock-server-{versions["preview"]}.zip'
    

    print(f'Downloading {url}...')
    os.system("wget -O /minecraft/tmp/bedrock-server.zip " + url)
    print('Extracting...')
    os.system("unzip /minecraft/tmp/bedrock-server.zip -d /minecraft/minecraft_server")
    print('Cleaning up...')
    os.system("rm /minecraft/tmp/bedrock-server.zip")
    

if __name__ == '__main__':
    main()
