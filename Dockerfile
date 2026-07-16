# syntax=docker/dockerfile:1.7

ARG GO_VERSION=1.26
ARG NODE_VERSION=22

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
WORKDIR /download
COPY versions/ /versions/
RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl unzip \
    && case "$SERVER_TYPE" in stable|preview) ;; *) echo "SERVER_TYPE must be stable or preview" >&2; exit 2 ;; esac \
    && version="$(cat "/versions/${SERVER_TYPE}.txt")" \
    && if [ "$SERVER_TYPE" = stable ]; then channel="bin-linux"; else channel="bin-linux-preview"; fi \
    && curl --fail --location --http1.1 --retry 5 --retry-all-errors --retry-delay 2 \
       --user-agent "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/96.0.4664.93 Safari/537.36" \
       --connect-timeout 20 --speed-limit 1024 --speed-time 30 --max-time 600 \
       --output /tmp/bedrock.zip \
       "https://www.minecraft.net/bedrockdedicatedserver/${channel}/bedrock-server-${version}.zip" \
    && mkdir -p /out \
    && unzip -q /tmp/bedrock.zip -d /out \
    && chmod 0755 /out/bedrock_server \
    && rm -f /tmp/bedrock.zip \
    && rm -rf /var/lib/apt/lists/*

FROM --platform=linux/amd64 debian:bookworm-slim AS runtime
LABEL org.opencontainers.image.source="https://github.com/WasinUddy/Montainer"

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

ENV LISTEN_ADDR=:8000 \
    GIN_MODE=release \
    BEDROCK_SERVER_PATH=./bedrock_server \
    INSTANCE_DIR=/app/instance \
    CONFIG_DIR=/app/configs \
    RESOURCE_PACKS_DIR=/app/resource_packs \
    LOG_DIR=/app/logs \
    STATIC_DIR=/app/dist \
    LD_LIBRARY_PATH=/app/instance

USER montainer

EXPOSE 8000 19132/udp 19133/udp
VOLUME ["/app/instance/worlds", "/app/configs", "/app/resource_packs", "/app/logs"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD curl --fail --silent --show-error http://127.0.0.1:8000/healthz || exit 1

ENTRYPOINT ["/app/montainer"]
