# Luna

Luna is an intranet family media app for videos, shorts, photos, and audio.

- Backend: Go (single `luna` binary)
- DB: SQLite (GORM)
- Frontend: Vue 3 + Vite (embedded into the Go binary)
- Media processing: ffmpeg/ffprobe worker pipeline

## Features

- Authenticated multi-user app with `admin` and `user` roles
- User profile + persona system (upload as self or persona)
- Persona avatars and persona profile pages
- Upload support for:
  - Video
  - Audio
  - Photo
- Background processing jobs:
  - Probe
  - Master transcode
  - Thumbnails
  - Shorts clip generation
  - HLS optimization
- Video item controls (permission-gated):
  - Edit
  - Delete (soft delete)
  - Reprocess
  - Set thumbnail from current playback timestamp
- Shorts:
  - Vertical feed mode
  - Grid preview mode with hover playback
  - Reactions (up/down)
  - Favorite/highlight controls at item level
- Playlist management (create, rename, add/remove/reorder)
- Trash view and restore flow
- Admin user management UI + CLI
- Recovery and maintenance tooling:
  - `rebuild-db`
  - `doctor`
  - `reconcile`
  - `snapshot` / `restore`
  - storage reports

## Project Layout

- `cmd/luna`: app entrypoint, HTTP server, worker, CLI commands
- `internal/*`: handlers, auth, jobs, media processing, storage/meta, admin logic
- `web`: Vue frontend source
- `cmd/luna/frontend`: embedded built frontend assets (generated)
- `Taskfile.yml`: common build/dev tasks

## Prerequisites

- Go (matching `go.mod`)
- Bun (for frontend build)
- ffmpeg + ffprobe available on `PATH`

## Quick Start

1. Build embedded frontend + backend binary:

```bash
task dev
```

2. Run server:

```bash
./luna serve
```

3. Run worker (in separate terminal):

```bash
./luna worker
```

4. Open browser at `http://localhost:8080` (or your configured `HTTP_ADDR`).

### Default bootstrap user

On first `serve`, Luna creates:

- username: `devuser`
- password: `devpass`
- role: `admin`

Change this immediately for non-dev usage.

## Environment Variables

Luna uses environment variables from `internal/config/config.go`:

- `MEDIA_ROOT`
  - Default: `$HOME/.luna`
  - Base directory for media files, sidecar meta, avatars, and DB folder structure
- `SQLITE_PATH`
  - Default: `$HOME/.luna/db/app.sqlite`
  - SQLite database file path
- `HTTP_ADDR`
  - Default: `:8080`
  - HTTP listen address
- `VIDEO_TRANSCODE_CONCURRENCY`
  - Default: `1`
  - Worker transcode concurrency (must be `>= 1`)
- `LUNA_DEV_PORTSCAN`
  - Default: `false`
  - If true, `serve` scans for next available port when configured port is busy

Example:

```bash
export MEDIA_ROOT=/data/media
export SQLITE_PATH=/data/media/db/luna.sqlite
export HTTP_ADDR=:8080
export VIDEO_TRANSCODE_CONCURRENCY=1
export LUNA_DEV_PORTSCAN=true
```

## Taskfile Commands

- `task install-hooks`: configure repo git hooks (`.githooks`)
- `task lint`: run `golangci-lint run ./...`
- `task build-frontend`: bun install + vite build
- `task prepare-embed`: copy `web/dist` to `cmd/luna/frontend`
- `task build-backend`: build production-style `luna` binary with embed
- `task build`: full build (frontend + embedded backend)
- `task dev`: debug build with embedded frontend
- `task clean`: remove binary + embedded frontend output
- `task run`: run `./luna serve`
- `task worker`: run `./luna worker`
- `task doctor`: run `./luna doctor`

## CLI Commands

Top-level:

- `luna serve`
- `luna worker [--id <worker-id>]`
- `luna import --src <dir> [--user <username>]`
- `luna rebuild-db --force [--media-root <path>]`
- `luna user <subcommand>`
- `luna item <subcommand>`
- `luna doctor [--json]`
- `luna reconcile [--media-root <path>] [--json] [--fix] [--limit <n>]`
- `luna snapshot --out <file.tar.gz> [--include-media] [--include-derived] [--include-originals]`
- `luna restore --in <file.tar.gz> [--media-root <path>] [--force] [--rebuild-db]`
- `luna report storage [--json]`

### `user` subcommands

- `luna user create --username <u> --password <p> [--role user|admin]`
- `luna user list [--all|--active|--inactive]`
- `luna user set-role --username <u> --role user|admin`
- `luna user deactivate --username <u> [--reason <text>]`
- `luna user activate --username <u>`
- `luna user passwd --username <u> --password <p>`
- `luna user purge --username <u> [--execute --confirm <username>]`

### `item` subcommands

- `luna item purge --id <item-id> [--execute --confirm <item-id>]`

Notes:

- `user purge` and `item purge` default to dry-run. `--execute` + matching `--confirm` is required.
- `rebuild-db` requires `--force`.

## Runtime Model

Luna is intended to run with:

- one `serve` process for API + UI
- one or more `worker` processes for media jobs

If worker is not running, uploads will remain queued/processing.

## Storage Model

Default storage root is `~/.luna` unless overridden.

Key paths:

- `items/<item_id>/original/`
- `items/<item_id>/derived/`
- `items/<item_id>/thumbs/`
- `items/<item_id>/photos/`
- `items/<item_id>/meta/item.json`
- `items/<item_id>/meta/assets.json`
- `avatars/`
- `db/`
- `tmp/`

## Embedded Frontend

The backend serves the embedded SPA from `cmd/luna/frontend`.

To refresh frontend changes in the binary:

```bash
task dev
# or
task build
```

## Docker

Build image:

```bash
docker build -t luna:local .
```

Run with Compose (server + worker):

```bash
docker compose up -d --build
```

Services in `docker-compose.yml`:

- `server`: runs `luna serve`, exposes `8080`
- `worker`: runs `luna worker --id worker-1`
- host media bind mount: `${LUNA_MEDIA_ROOT_HOST}` -> `/data/media` (use your host NFS mount path)
- host DB bind mount: `${LUNA_DB_ROOT_HOST}` -> `/data/db` (local host directory)

Compose defaults:

- `LUNA_MEDIA_ROOT_HOST=/mnt/nfs/luna-media`
- `LUNA_DB_ROOT_HOST=./docker-data/db`

Container runtime paths:

- `MEDIA_ROOT=/data/media`
- `SQLITE_PATH=/data/db/app.sqlite`

The Docker build uses Bun only in the frontend build stage, then copies built assets into the Go build stage, and runs on a runtime image with `ffmpeg` installed.

## Operational Notes

- ffmpeg/ffprobe are required for media processing features.
- Some browser autoplay policies may mute auto-play previews until user interaction.
- Thumbnail updates are cache-busted in API thumbnail URLs, so updates should appear without hard refresh.

## Frontend Source

The frontend source is in `web/`.

Useful local commands:

```bash
cd web
bun install
bun run dev
bun run build
```

For integrated runs, prefer Taskfile commands so embed output is consistent.
