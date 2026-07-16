# Changelog

## [3.0.0] - 2026-07-16
### Highlights
- Rebuilt Montainer's backend in Go 1.26 with Gin, replacing the Python/FastAPI runtime with a concurrency-safe Bedrock process supervisor.
- Added optional, non-blocking OpenTelemetry log export while preserving standalone local file, HTTP, and WebSocket logging.
- Redesigned the Web UI as a responsive light console with lifecycle details, log search and filtering, and offline Minecraft command autocomplete including a contextual teleport guide.

### Added
- Explicit `starting`, `running`, `stopping`, `stopped`, and `failed` lifecycle states, graceful shutdown escalation, process generations, and a readiness endpoint.
- S3-compatible backups implemented with AWS SDK for Go, consistent lifecycle locking, archive validation, and a MinIO example stack.
- Black-box Cucumber acceptance tests using a controllable fake Bedrock process for lifecycle, cancellation, local logging, WebSockets, subpath routing, and OTLP behavior.
- Real-image acceptance tests that boot the exact scraped Mojang image and exercise RakNet discovery, concurrent lifecycle operations, OTEL export/outage/flush, MinIO backup integrity, and an offline virtual player join and teleport.
- Stable and preview image pipelines connected through immutable, checksummed artifacts and channel-specific promotion identity records.

### Changed
- Unified stable and preview delivery into one staged, matrix-oriented CI/CD graph with path-aware channel selection and parallel runners.
- Build and publish a non-root `linux/amd64` image containing the compiled frontend, static Go service, and checksum-verified Mojang Bedrock archive.
- Preserve the existing management routes, S3 environment variables, subpath behavior, and persistent world/config/resource-pack layout while returning clearer lifecycle conflicts and failures.
- Publish only the exact Docker artifact that passed acceptance tests; immutable tags include the Bedrock version and source commit before the verified manifest is copied to `latest`.

### Breaking changes
- The Python runtime and FastAPI backend are removed. Direct-binary deployments must now run the Go executable and explicitly provide their environment.
- The container runs as UID/GID `10001`; existing bind mounts must be writable by that identity.
- OTLP export supports `http/protobuf`, and deployments should allow at least 90 seconds for graceful container termination.

## [2.2.0] - 2025-06-20
### Fixed
- Updated the scraping logic to match mojang's new website

## [2.1.1] - 2025-01-28
### Added
- Expose ipv6  on 19133/udp by [Jason Clark](https://github.com/SuperJC710e)

### Fixed
- Container `HealthCheck` not working properly by [Jason Clark](https://github.com/SuperJC710e)

## [2.1.0] - 2025-01-18
### Added
- Added platform specifications in the Dockerfile -- `linux/amd64` as suggested by [pull request #14](https://github.com/WasinUddy/Montainer/pull/14)
- Added Documentation for usage of Montainer on ARM64 machines

## [2.0.6] - 2024-11-20
### Fixed
- Fixed unable to backup to AWS S3

## [2.0.5] - 2024-11-20
### Fixed
- boto3 module not found fix by using requirments.txt within Dockerfile

## [2.0.4] - 2024-11-20
### Added
- Added `/healthz` endpoint to check the health of the server for use in Kubernetes liveness and readiness probes

## [2.0.3] - 2024-11-18
### Added
- Added ability to back up persistent data to AWS S3 compatible storage

## [2.0.2] - 2024-11-16
### Added
- Force Restart button to restart the server

## [2.0.1] - 2024-11-15
### Added
- Auto start minecraft server on container start suggested by [issue #12](https://github.com/WasinUddy/Montainer/issues/12) by [niker](https://github.com/niker)
