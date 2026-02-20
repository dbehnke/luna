# syntax=docker/dockerfile:1.7

# Stage 1: Build frontend assets with Bun
FROM oven/bun:1 AS frontend-builder
WORKDIR /src/web
COPY web/bun.lock web/package.json ./
RUN bun install --frozen-lockfile
COPY web/ ./
RUN bun run build

# Stage 2: Build Go binary with embedded frontend
FROM golang:1.25-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend-builder /src/web/dist ./web/dist
RUN rm -rf cmd/luna/frontend \
    && mkdir -p cmd/luna \
    && cp -r ./web/dist cmd/luna/frontend \
    && CGO_ENABLED=1 go build -o /out/luna ./cmd/luna

# Stage 3: Runtime with ffmpeg/ffprobe
FROM debian:bookworm-slim AS runtime

ENV DEBIAN_FRONTEND=noninteractive \
    MEDIA_ROOT=/data/media \
    SQLITE_PATH=/data/db/app.sqlite \
    HTTP_ADDR=:8080 \
    VIDEO_TRANSCODE_CONCURRENCY=1

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
       ca-certificates \
       ffmpeg \
       tzdata \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=go-builder /out/luna /usr/local/bin/luna

RUN mkdir -p /data/media/items /data/media/tmp /data/media/avatars /data/db

EXPOSE 8080

ENTRYPOINT ["luna"]
CMD ["serve"]
