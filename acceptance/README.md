# Montainer v2 acceptance tests

Montainer has two complementary black-box Godog suites:

- `./acceptance` launches the real Montainer binary against a deterministic fake Bedrock process. It is fast and can force crashes, delays, stderr output, client cancellation, and Collector outages.
- `./acceptance/realimage` launches an already-built Docker image with the packaged Mojang Bedrock binary. It validates the release artifact, native runtime libraries, real process behavior, OTLP Collector integration, MinIO backups, UDP discovery, and client compatibility.

## Fast fake-Bedrock suite

Run every deterministic scenario:

```bash
go test -v -count=1 ./acceptance
```

Run one business area:

```bash
GODOG_TAGS='@lifecycle' go test -v -count=1 ./acceptance
GODOG_TAGS='@logging' go test -v -count=1 ./acceptance
GODOG_TAGS='@otel' go test -v -count=1 ./acceptance
GODOG_TAGS='@subpath' go test -v -count=1 ./acceptance
```

The suite normally builds `./cmd/montainer`. To exercise an already-built binary, set:

```bash
MONTAINER_ACCEPTANCE_BINARY=/absolute/path/to/montainer \
  go test -v -count=1 ./acceptance
```

The fake Bedrock process records every start, stdin command, OS signal, overlap, and graceful exit. Test-only commands can emit stdout/stderr, crash with a chosen exit code, or delay shutdown. Covered behavior includes lifecycle serialization, unexpected exit, client cancellation, subpath routing, local/WebSocket logs, OTLP attributes, Collector outage independence, and shutdown flushing.

## Real Mojang image suite

Build the image once, then select a business shard:

```bash
docker build \
  --build-arg SERVER_TYPE=stable \
  --build-arg BEDROCK_VERSION="$(cat versions/stable.txt)" \
  -t montainer:acceptance .

GODOG_TAGS='@smoke' \
MONTAINER_ACCEPTANCE_IMAGE='montainer:acceptance' \
MONTAINER_EXPECTED_BEDROCK_VERSION="$(cat versions/stable.txt)" \
  go test -v -count=1 ./acceptance/realimage
```

Each channel is pinned by its files under `versions/`: the scraped version, exact Mojang URL, and archive SHA-256. The Docker build rejects a URL that does not match the channel/version and rejects downloaded bytes that do not match the recorded checksum.

Available real-image tags are:

- `@smoke`: exact Mojang version, management health, real command I/O, and RakNet discovery;
- `@lifecycle`: eight concurrent stops and starts, conflict semantics, generation count, and gameplay recovery;
- `@otel-export`: export through the pinned real Collector;
- `@otel-outage`: unavailable-Collector isolation;
- `@otel-flush`: graceful-shutdown export flushing;
- `@backup`: four concurrent saves, MinIO object verification, ZIP integrity, one restart, and gameplay recovery;
- `@client`: offline virtual-player spawn, authoritative `list` output, and receipt of a teleport movement packet; and
- `@upgrade` (stable only): scenarios covering a genuine root-owned world and scoreboard marker created by the digest-pinned pre-v3 image, a virtual player joining and moving in that upgraded world, external restoration of its downloaded MinIO ZIP into fresh volumes where the same score must load, a root-owned custom `INSTANCE_DIR`, literal same-device nested-mount pruning, and explicit non-root startup with a working unprivileged health probe. Montainer PID 1 and its Bedrock child are checked for UID/GID, capabilities, and `no_new_privs`.

The feature-level `@otel` tag still selects all three OTLP scenarios for a combined local run; CI uses the individual tags so they execute concurrently.

`MONTAINER_LEGACY_IMAGE` can override the pinned pre-v3 fixture for local investigation. CI leaves it at the immutable `1.26.33.1` digest so the upgrade contract cannot move with a tag.

Each scenario owns unique container names, an isolated Docker network, isolated persistence, writable non-root configuration, and dynamic host ports. The upgrade shard labels and explicitly removes every legacy, restore-verification, custom-instance, and nested-mount named volume after all containers are gone; other shards use anonymous volumes. Failed scenarios print Montainer, Collector, MinIO, and virtual-client logs before cleanup.

The full client uses a separately pinned gophertunnel module under `test/fixtures/bedrockclient`. It intentionally uses no Microsoft/Xbox credentials and only connects to an isolated server configured with `online-mode=false`. CI makes this a stable-image gate. Preview images always require RakNet discovery but omit the full join when the pinned test client has not yet added Mojang's preview protocol; this avoids mistaking a stale client library for a broken server image.

Set `MONTAINER_ACCEPTANCE_KEEP_TMP=1` to preserve per-scenario configuration and downloaded backup diagnostics. Auxiliary image references can be overridden with `MONTAINER_OTEL_COLLECTOR_IMAGE` and `MONTAINER_MINIO_IMAGE`.

## CI release topology

The connected delivery workflow uses numbered stages and matrices:

1. `0 · Plan` safely selects stable, preview, or both from the changed paths. It uses a selective delta only after a successful delivery of the previous commit and falls back to both channels otherwise.
2. `1 · Quality` runs frontend, Go/race/vet/actionlint, and the four fake-Bedrock tag groups as one six-row matrix shared by every selected channel.
3. `2 · Build` verifies the recorded Mojang URL and SHA-256, builds each selected channel once, and exports a channel-specific Docker archive with a recorded archive SHA-256 and image ID.
4. `3 · Accept` fans that exact artifact out to eight stable runners (`smoke`, `lifecycle`, the three OTLP tags, `backup`, `client`, and `upgrade`) or six preview runners, omitting the full virtual client and legacy-volume upgrade.
5. `4 · Promote` receives package-write permission only after its channel's acceptance matrix succeeds, reloads the same artifact, overwrites the Minecraft-version tag with that accepted image, copies the same manifest to `latest`, and publishes a channel-specific digest identity record.
6. `5 · Release` is connected directly to stable promotion. It validates the same-run identity and current version-tag manifest, records a digest-pinned image reference, then creates only an unreleased changelog version; existing tag/release pairs are a clean no-op and interrupted same-commit releases are retryable.

Planning, quality, build, and acceptance jobs have read-only repository permissions. No release image is built separately from the artifact exercised by acceptance tests. Channel and run-attempt names prevent stable/preview or rerun artifact collisions, while failed-job reruns reuse the successful build's recorded artifact. Manual dispatch can select `stable`, `preview`, or `both`; it defaults to validation-only and leaves release tags unchanged unless a maintainer explicitly enables `publish` on `main`.
