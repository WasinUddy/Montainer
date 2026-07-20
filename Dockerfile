# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26
ARG NODE_VERSION=22
ARG BEDROCK_VERSION=""

FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-bookworm-slim AS frontend
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/montainer ./cmd/montainer

FROM --platform=linux/amd64 debian:bookworm-slim AS bedrock
ARG SERVER_TYPE=stable
ARG BEDROCK_VERSION
WORKDIR /download
COPY versions/ /versions/
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip \
    && case "$SERVER_TYPE" in stable|preview) ;; *) echo "SERVER_TYPE must be stable or preview" >&2; exit 2 ;; esac \
    && version="$(cat "/versions/${SERVER_TYPE}.txt")" \
    && if ! printf '%s\n' "$version" | grep -Eq '^[0-9]+(\.[0-9]+){2,3}$'; then echo "invalid Bedrock version: $version" >&2; exit 2; fi \
    && if [ -n "$BEDROCK_VERSION" ] && [ "$BEDROCK_VERSION" != "$version" ]; then echo "build version $BEDROCK_VERSION does not match scraped version $version" >&2; exit 2; fi \
    && download_url="$(cat "/versions/${SERVER_TYPE}.url")" \
    && if [ "$SERVER_TYPE" = stable ]; then expected_url="https://www.minecraft.net/bedrockdedicatedserver/bin-linux/bedrock-server-${version}.zip"; else expected_url="https://www.minecraft.net/bedrockdedicatedserver/bin-linux-preview/bedrock-server-${version}.zip"; fi \
    && if [ "$download_url" != "$expected_url" ]; then echo "scraped Bedrock URL does not match channel/version: $download_url" >&2; exit 2; fi \
    && expected_sha256="$(cat "/versions/${SERVER_TYPE}.sha256")" \
    && if ! printf '%s\n' "$expected_sha256" | grep -Eq '^[0-9a-f]{64}$'; then echo "invalid Bedrock SHA-256: $expected_sha256" >&2; exit 2; fi \
    && curl --fail --location --http1.1 --retry 5 --retry-all-errors --retry-delay 2 \
       --user-agent "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.93 Safari/537.36" \
       --connect-timeout 20 --speed-limit 1024 --speed-time 30 --max-time 600 \
       --output /tmp/bedrock.zip \
       "$download_url" \
    && printf '%s  %s\n' "$expected_sha256" /tmp/bedrock.zip | sha256sum --check --strict - \
    && mkdir -p /out \
    && unzip -q /tmp/bedrock.zip -d /out \
    && chmod 0755 /out/bedrock_server \
    && rm -f /tmp/bedrock.zip \
    && rm -rf /var/lib/apt/lists/*

FROM --platform=linux/amd64 debian:bookworm-slim AS runtime
ARG SERVER_TYPE=stable
ARG BEDROCK_VERSION="unknown"
LABEL org.opencontainers.image.source="https://github.com/WasinUddy/Montainer" \
      org.opencontainers.image.version="$BEDROCK_VERSION" \
      io.montainer.bedrock.channel="$SERVER_TYPE"

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl libcurl4 \
    && groupadd --gid 10001 montainer \
    && useradd --uid 10001 --gid 10001 --create-home --home-dir /home/montainer montainer \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
RUN mkdir -p /app/instance/worlds /app/configs /app/resource_packs /app/logs /app/dist \
    && chown -R montainer:montainer /app /home/montainer

COPY --from=bedrock --chown=montainer:montainer /out/ /app/instance/
COPY --from=frontend --chown=montainer:montainer /src/web/dist/ /app/dist/
COPY --from=go-builder --chown=montainer:montainer /out/montainer /app/montainer
COPY --chown=root:root scripts/docker-entrypoint.sh /usr/local/bin/montainer-entrypoint
RUN chmod 0755 /usr/local/bin/montainer-entrypoint

ENV LISTEN_ADDR=:8000 \
    GIN_MODE=release \
    BEDROCK_SERVER_PATH=./bedrock_server \
    INSTANCE_DIR=/app/instance \
    CONFIG_DIR=/app/configs \
    RESOURCE_PACKS_DIR=/app/resource_packs \
    LOG_DIR=/app/logs \
    STATIC_DIR=/app/dist

# The entrypoint repairs pre-v3 root-owned mounts, drops every capability, and
# execs Montainer as UID/GID 10001 before the application or Bedrock starts.
USER root

EXPOSE 8000 19132/udp 19133/udp
VOLUME ["/app/instance/worlds", "/app/configs", "/app/resource_packs", "/app/logs"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5m --retries=3 \
    CMD ["/usr/local/bin/montainer-entrypoint", "__healthcheck"]

ENTRYPOINT ["/usr/local/bin/montainer-entrypoint"]
