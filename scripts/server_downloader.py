import os
import argparse

def main():
    parser = argparse.ArgumentParser(description='Download Minecraft Bedrock Server from Mojang server.')
    parser.add_argument('--type', type=str, help='Specify the server type (stable or preview).')
    parser.add_argument('--version', type=str, help='Specify the server version.')
    args = parser.parse_args()

    if args.type=='stable':
        url = f'https://minecraft.azureedge.net/bin-linux/bedrock-server-{args.version}.zip'
    elif args.type=='preview':
        url = f'https://minecraft.azureedge.net/bin-linux-preview/bedrock-server-{args.version}.zip'
    else:
        raise ValueError(f'Invalid server type error type must be stable or preview got {args.version}.')

    print(f'Downloading {url}...')
    os.system("wget -O bedrock-server.zip " + url)


if __name__ == '__main__':
    main()
