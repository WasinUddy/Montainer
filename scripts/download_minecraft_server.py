import argparse
import hashlib
import io
import os
import re
import zipfile
from urllib.request import Request, urlopen


DOWNLOAD_USER_AGENT = (
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
    "AppleWebKit/537.36 (KHTML, like Gecko) "
    "Chrome/96.0.4664.93 Safari/537.36"
)
VERSION_PATTERN = re.compile(r"^[0-9]+(?:\.[0-9]+){2,3}$")
SHA256_PATTERN = re.compile(r"^[0-9a-f]{64}$")


def read_metadata(server_type: str) -> tuple[str, str, str]:
    metadata_root = "versions"
    with open(
        os.path.join(metadata_root, f"{server_type}.txt"), encoding="utf-8"
    ) as stream:
        version = stream.read().strip()
    with open(
        os.path.join(metadata_root, f"{server_type}.url"), encoding="utf-8"
    ) as stream:
        url = stream.read().strip()
    with open(
        os.path.join(metadata_root, f"{server_type}.sha256"), encoding="utf-8"
    ) as stream:
        expected_sha256 = stream.read().strip()

    if not VERSION_PATTERN.fullmatch(version):
        raise ValueError(f"invalid Bedrock version: {version!r}")
    channel = "bin-linux" if server_type == "stable" else "bin-linux-preview"
    expected_url = (
        "https://www.minecraft.net/bedrockdedicatedserver/"
        f"{channel}/bedrock-server-{version}.zip"
    )
    if url != expected_url:
        raise ValueError(
            f"scraped Bedrock URL does not match channel/version: {url!r}"
        )
    if not SHA256_PATTERN.fullmatch(expected_sha256):
        raise ValueError(f"invalid Bedrock SHA-256: {expected_sha256!r}")
    return version, url, expected_sha256


def main():
    parser = argparse.ArgumentParser(
        description="Download Minecraft Bedrock Server from Mojang server."
    )
    parser.add_argument(
        "--type",
        choices=("stable", "preview"),
        required=True,
        help="Specify the server type.",
    )
    args = parser.parse_args()

    version, url, expected_sha256 = read_metadata(args.type)

    print(f"Downloading Bedrock {version} from {url}...")
    request = Request(url, headers={"User-Agent": DOWNLOAD_USER_AGENT})
    with urlopen(request, timeout=600) as response:
        archive = response.read()

    actual_sha256 = hashlib.sha256(archive).hexdigest()
    if actual_sha256 != expected_sha256:
        raise ValueError(
            "Bedrock archive SHA-256 mismatch: "
            f"expected {expected_sha256}, got {actual_sha256}"
        )

    print("Extracting...")
    with zipfile.ZipFile(io.BytesIO(archive)) as z:
        z.extractall("bedrock_server")

    os.chmod(os.path.join("bedrock_server", "bedrock_server"), 0o755)

    print("Done.")


if __name__ == "__main__":
    main()
