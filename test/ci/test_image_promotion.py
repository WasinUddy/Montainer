"""Exercise the actual workflow shell steps against a simulated registry."""

import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import textwrap
import unittest


ROOT = Path(__file__).resolve().parents[2]
IMAGE = "ghcr.io/wasinuddy/montainer-stable"
VERSION_TAG = IMAGE + ":1.26.63.1"
LATEST_TAG = IMAGE + ":latest"


def digest(value):
    return "sha256:" + hashlib.sha256(value.encode()).hexdigest()


def manifest(config, layer="tested layer"):
    raw = json.dumps({
        "schemaVersion": 2,
        "mediaType": "application/vnd.oci.image.manifest.v1+json",
        "config": {"digest": config},
        "layers": [{"digest": digest(layer)}],
    })
    return {"raw": raw, "digest": digest(raw)}


def workflow_step(filename, name):
    workflow = (ROOT / ".github/workflows" / filename).read_text()
    # Only extract the literal shell block for the named step; no YAML dependency in CI.
    step = workflow.split("      - name: " + name + "\n", 1)[1]
    step = step.split("\n      - name:", 1)[0]
    return textwrap.dedent(re.search(r"        run: \|\n((?:          .*\n|\n)+)", step).group(1))


class PromotionTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        directory = Path(self.tmp.name)
        self.registry = directory / "registry.json"
        self.config = digest("tested config")
        self.accepted = manifest(self.config)
        self.old = manifest(digest("previous config"))
        self.write_registry({VERSION_TAG: self.accepted, LATEST_TAG: self.old})
        docker = directory / "docker"
        docker.write_text(f"#!{sys.executable}\n" + textwrap.dedent('''\
            import json, os, sys
            from pathlib import Path
            registry = Path(os.environ["TEST_REGISTRY"])
            state = json.loads(registry.read_text())
            args = sys.argv[1:]
            if args[0] in ("tag", "push"):
                sys.exit(0)
            if args[:3] == ["buildx", "imagetools", "inspect"]:
                image = state[args[-1]]
                print(image["raw"] if "--raw" in args else image["digest"])
            elif args[:3] == ["buildx", "imagetools", "create"]:
                source = args[-1].split("@", 1)[1]
                image = next(image for image in state.values() if image["digest"] == source)
                state[args[args.index("--tag") + 1]] = image
                registry.write_text(json.dumps(state))
            else:
                sys.exit("Unexpected Docker command: " + repr(args))
            '''))
        docker.chmod(0o755)
        self.env = {
            **os.environ,
            "PATH": str(directory) + os.pathsep + os.environ["PATH"],
            "TEST_REGISTRY": str(self.registry),
            "CANDIDATE_TAG": "montainer-candidate:accepted",
            "REGISTRY_IMAGE": IMAGE,
            "VERSION": "1.26.63.1",
            "VERSION_TAG": VERSION_TAG,
            "TESTED_IMAGE_TAG": VERSION_TAG,
            "TESTED_IMAGE_DIGEST": self.accepted["digest"],
            "GITHUB_OUTPUT": str(directory / "outputs"),
        }

    def write_registry(self, state):
        self.registry.write_text(json.dumps(state))

    def run_step(self, name, expected_id, succeeds=True, release=False):
        filename = "release.yaml" if release else "promote-tested-image.yaml"
        result = subprocess.run(
            ["bash", "-c", workflow_step(filename, name)],
            env={**self.env, "EXPECTED_IMAGE_ID": expected_id, "RECORDED_IMAGE_ID": expected_id},
            capture_output=True, text=True,
        )
        self.assertEqual(result.returncode == 0, succeeds, result.stdout + result.stderr)

    def publish(self, expected_id, succeeds=True):
        self.run_step("Apply release tags to the tested image", expected_id, succeeds)
        state = json.loads(self.registry.read_text())
        self.assertEqual(state[LATEST_TAG], self.accepted if succeeds else self.old)

    def verify(self, expected_id, succeeds=True):
        self.run_step("Verify promoted tags reference the tested image", expected_id, succeeds)

    def release(self, expected_id, succeeds=True):
        self.run_step("Verify the Minecraft-version promoted image identity", expected_id, succeeds, release=True)

    def test_classic_and_containerd_promote_verify_and_release(self):
        for expected in (self.config, self.accepted["digest"]):
            with self.subTest(expected=expected):
                self.publish(expected)
                self.verify(expected)
                self.release(expected)

    def test_unaccepted_image_cannot_update_latest(self):
        self.publish(digest("unaccepted image"), succeeds=False)

    def test_invalid_manifest_digest_cannot_update_latest(self):
        self.write_registry({VERSION_TAG: {**self.accepted, "digest": "invalid"}, LATEST_TAG: self.old})
        self.publish(self.config, succeeds=False)

    def test_missing_config_cannot_update_latest(self):
        self.write_registry({VERSION_TAG: {**self.accepted, "raw": "{}"}, LATEST_TAG: self.old})
        self.publish(self.accepted["digest"], succeeds=False)

    def test_latest_must_match_version_manifest_even_with_same_config(self):
        self.write_registry({VERSION_TAG: self.accepted, LATEST_TAG: manifest(self.config, "different layer")})
        for expected in (self.config, self.accepted["digest"]):
            self.verify(expected, succeeds=False)

    def test_changed_version_manifest_rejects_containerd_identity(self):
        self.write_registry({VERSION_TAG: manifest(self.config, "different layer"), LATEST_TAG: self.old})
        self.publish(self.accepted["digest"], succeeds=False)

    def test_release_rejects_changed_manifest_even_with_same_config(self):
        self.write_registry({VERSION_TAG: manifest(self.config, "different layer"), LATEST_TAG: self.old})
        for expected in (self.config, self.accepted["digest"]):
            self.release(expected, succeeds=False)

    def test_release_rejects_unaccepted_image_id(self):
        self.release(digest("unaccepted image"), succeeds=False)


if __name__ == "__main__":
    unittest.main()
