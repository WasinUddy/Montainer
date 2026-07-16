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
- `@backup`: four concurrent saves, MinIO object verification, ZIP integrity, one restart, and gameplay recovery; and
- `@client`: offline virtual-player spawn, authoritative `list` output, and receipt of a teleport movement packet.

The feature-level `@otel` tag still selects all three OTLP scenarios for a combined local run; CI uses the individual tags so they execute concurrently.

Each scenario owns unique container names, an isolated Docker network, anonymous data volumes, writable non-root configuration, and dynamic host ports. Failed scenarios print Montainer, Collector, MinIO, and virtual-client logs before cleanup.

The full client uses a separately pinned gophertunnel module under `test/fixtures/bedrockclient`. It intentionally uses no Microsoft/Xbox credentials and only connects to an isolated server configured with `online-mode=false`. CI makes this a stable-image gate. Preview images always require RakNet discovery but omit the full join when the pinned test client has not yet added Mojang's preview protocol; this avoids mistaking a stale client library for a broken server image.

Set `MONTAINER_ACCEPTANCE_KEEP_TMP=1` to preserve per-scenario configuration and downloaded backup diagnostics. Auxiliary image references can be overridden with `MONTAINER_OTEL_COLLECTOR_IMAGE` and `MONTAINER_MINIO_IMAGE`.

## CI release topology

The connected delivery workflow uses numbered stages and matrices:

1. `0 · Plan` safely selects stable, preview, or both from the changed paths. It uses a selective delta only after a successful delivery of the previous commit and falls back to both channels otherwise.
2. `1 · Quality` runs frontend, Go/race/vet/actionlint, and the four fake-Bedrock tag groups as one six-row matrix shared by every selected channel.
3. `2 · Build` verifies the recorded Mojang URL and SHA-256, builds each selected channel once, and exports a channel-specific Docker archive with a recorded archive SHA-256 and image ID.
4. `3 · Accept` fans that exact artifact out to seven stable runners (`smoke`, `lifecycle`, the three OTLP tags, `backup`, and `client`) or six preview runners, omitting only the full virtual client.
5. `4 · Promote` receives package-write permission only after its channel's acceptance matrix succeeds, reloads the same artifact, creates the immutable version/commit tag once, copies its exact manifest to `latest`, and publishes a channel-specific identity record.
6. `5 · Release` is connected directly to stable promotion. It validates the same-run identity and current immutable manifest, then creates only an unreleased changelog version; existing tag/release pairs are a clean no-op and interrupted same-commit releases are retryable.

Planning, quality, build, and acceptance jobs have read-only repository permissions. No release image is built separately from the artifact exercised by acceptance tests. Channel and run-attempt names prevent stable/preview or rerun artifact collisions, while failed-job reruns reuse the successful build's recorded artifact. Manual dispatch can select `stable`, `preview`, or `both`; it defaults to validation-only and leaves release tags unchanged unless a maintainer explicitly enables `publish` on `main`.
