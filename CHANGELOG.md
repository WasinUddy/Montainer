# Changelog

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
