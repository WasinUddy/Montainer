# Montainer v2 acceptance tests

Montainer has two complementary black-box Godog suites:

- `./acceptance` launches the real Montainer binary against a deterministic fake Bedrock process. It is fast and can force crashes, delays, stderr output, client cancellation, and Collector outages.
- `./acceptance/realimage` launches an already-built Docker image with the packaged Mojang Bedrock binary. It validates the release artifact, native runtime libraries, real process behavior, OTLP Collector integration, RustFS backups, NetherNet HTTP discovery, and client compatibility.

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

- `@smoke`: exact Mojang version, management health, real command I/O, and NetherNet HTTP discovery;
- `@lifecycle`: eight concurrent stops and starts, conflict semantics, generation count, and gameplay recovery;
- `@otel-export`: export through the pinned real Collector;
- `@otel-outage`: unavailable-Collector isolation;
- `@otel-flush`: graceful-shutdown export flushing;
- `@backup`: four concurrent saves, RustFS object verification, ZIP integrity, one restart, and gameplay recovery;
- `@client`: offline virtual-player spawn, authoritative `list` output, and receipt of a teleport movement packet; and
- `@upgrade` (stable only): scenarios covering a genuine root-owned world and scoreboard marker created by the digest-pinned pre-v3 image, a virtual player joining and moving in that world, external restoration of its downloaded RustFS ZIP into fresh volumes where the same score must load, the same world reassigned to an ordinary host UID, a `PUID`/`PGID` override, a root-owned custom `INSTANCE_DIR`, and explicit non-root startup with a working unprivileged health probe. Every scenario asserts that persistence ownership is left exactly as it was found, and Montainer PID 1 and its Bedrock child are checked for UID; any non-root identity is additionally checked for capabilities and `no_new_privs`.

The feature-level `@otel` tag still selects all three OTLP scenarios for a combined local run; CI uses the individual tags so they execute concurrently.

`MONTAINER_LEGACY_IMAGE` can override the pinned pre-v3 fixture for local investigation. CI leaves it at the immutable `1.26.33.1` digest so the upgrade contract cannot move with a tag.

Each scenario owns unique container names, an isolated Docker network, isolated persistence, writable non-root configuration, and dynamic host ports. The upgrade shard labels and explicitly removes every legacy, restore-verification, and custom-instance named volume after all containers are gone; other shards use anonymous volumes. Failed scenarios print Montainer, Collector, RustFS, and virtual-client logs before cleanup.

The full client uses gophertunnel under `test/fixtures/bedrockclient`. It intentionally uses no Microsoft/Xbox credentials and only connects to an isolated server configured with `online-mode=false`. The client uses gophertunnel's `minecraft.DefaultProtocol`, including its packet codecs; the client/upgrade CI shards upgrade gophertunnel with `@upgrade` (which preserves the pinned offline NetherNet fix until a newer release is available) immediately before building the probe so the protocol support follows the current Bedrock release without hardcoding a protocol ID. The client discovers `/v1/join` over HTTP, then joins using NetherNet/WebRTC with a locally generated, self-signed transport identity. No external identity service is needed. Preview images require a valid NetherNet status response but omit the full join when the current client library does not yet support Mojang's preview protocol.

Acceptance fixtures explicitly set `transport=nethernet` and `server-udp-ports=19132`. The signaling TCP port is published dynamically on loopback and the discovery check runs through that mapping. The virtual player runs inside the isolated Docker network, where negotiated UDP candidates are directly reachable. The pinned pre-v3 world-seeding image and the custom-instance scenario that deliberately preserves its old binary retain RakNet checks. The world-seeding configuration switches to NetherNet before the current packaged binary adopts that world.

The probe pins go-nethernet `v1.0.26` for [client identity key encoding](https://github.com/df-mc/go-nethernet/commit/fda14c119937919d850d08b4202845d5952cc86b) and Gophertunnel commit [`25acc41`](https://github.com/Sandertv/gophertunnel/commit/25acc41eb24af21cfebf27ffa5ace37069838eb3) so the offline WebRTC assertion and game login share a key. Older versions either reject the offer or disconnect during login.

Set `MONTAINER_ACCEPTANCE_KEEP_TMP=1` to preserve per-scenario configuration and downloaded backup diagnostics. The default RustFS image is digest-pinned `rustfs/rustfs:1.0.1`; auxiliary image references can be overridden with `MONTAINER_OTEL_COLLECTOR_IMAGE` and `MONTAINER_RUSTFS_IMAGE`.

## CI release topology

The connected delivery workflow uses numbered stages and matrices:

1. `0 · Plan` safely selects stable, preview, or both from the changed paths. It uses a selective delta only after a successful delivery of the previous commit and falls back to both channels otherwise.
2. `1 · Quality` runs frontend, Go/race/vet/actionlint, and the four fake-Bedrock tag groups as one six-row matrix shared by every selected channel.
3. `2 · Build` verifies the recorded Mojang URL and SHA-256, builds each selected channel once, and exports a channel-specific Docker archive with a recorded archive SHA-256 and image ID.
4. `3 · Accept` fans that exact artifact out to eight stable runners (`smoke`, `lifecycle`, the three OTLP tags, `backup`, `client`, and `upgrade`) or six preview runners, omitting the full virtual client and legacy-volume upgrade.
5. `4 · Promote` receives package-write permission only after its channel's acceptance matrix succeeds, reloads the same artifact, overwrites the Minecraft-version tag with that accepted image, copies the same manifest to `latest`, and publishes a channel-specific digest identity record.
6. `5 · Release` is connected directly to stable promotion. It validates the same-run identity and current version-tag manifest, records a digest-pinned image reference, then creates only an unreleased changelog version; existing tag/release pairs are a clean no-op and interrupted same-commit releases are retryable.

Planning, quality, build, and acceptance jobs have read-only repository permissions. No release image is built separately from the artifact exercised by acceptance tests. Channel and run-attempt names prevent stable/preview or rerun artifact collisions, while failed-job reruns reuse the successful build's recorded artifact. Manual dispatch can select `stable`, `preview`, or `both`; it defaults to validation-only and leaves release tags unchanged unless a maintainer explicitly enables `publish` on `main`.
