#!/usr/bin/env python3
"""Guard the public GHCR tag contract used by promotion and release."""

from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PROMOTION = (ROOT / ".github/workflows/promote-tested-image.yaml").read_text()
RELEASE = (ROOT / ".github/workflows/release.yaml").read_text()


def require(text: str, fragment: str, source: str) -> None:
    if fragment not in text:
        raise SystemExit(f"{source} is missing required image-tag contract: {fragment}")


def reject(text: str, fragment: str, source: str) -> None:
    if fragment in text:
        raise SystemExit(f"{source} contains forbidden commit-suffixed tag: {fragment}")


require(PROMOTION, 'version_tag="$REGISTRY_IMAGE:$VERSION"', "promotion workflow")
require(PROMOTION, 'docker push "$version_tag"', "promotion workflow")
require(PROMOTION, "schema_version: 3", "promotion workflow")
require(PROMOTION, "version_tag: $version_tag", "promotion workflow")
reject(PROMOTION, '$VERSION-${{ github.sha }}', "promotion workflow")

require(RELEASE, 'image_tag="$REGISTRY_IMAGE:$bedrock_version"', "release workflow")
require(RELEASE, ".schema_version == 3", "release workflow")
require(RELEASE, ".version_tag == $version_tag", "release workflow")
reject(RELEASE, '$bedrock_version-$TARGET_SHA', "release workflow")

print("Minecraft-version image tag contract is valid")
