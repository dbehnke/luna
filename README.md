# Luna

Luna is an intranet family media server for videos, shorts, photos, and audio, with user/persona ownership and background processing.

## Stack

- Backend: Go (`luna` single binary)
- Database: SQLite (GORM)
- Frontend: Vue 3 + Vite (embedded into backend binary)
- Processing: ffmpeg + ffprobe worker jobs

## Core Features

- Login/session auth with user roles (`admin`, `user`)
- User profiles and persona profiles
- Persona-owned content (upload as user or persona)
- Upload + processing for:
  - video
  - audio
  - photo
- Video processing pipeline:
  - probe
  - transcode to master MP4 (faststart)
  - thumbnails
  - HLS optimization
- Shorts/clip workflow:
  - create clip by start/end range
  - clip list and delete clip
  - feed + grid viewing modes
- Social and organization:
  - reactions (up/down)
  - favorites
  - playlists (create, rename, add/remove/reorder)
  - highlight flag
- Item management (permission-gated):
  - edit, delete (soft), restore, reprocess
  - set thumbnail from current video timestamp
- Admin user management:
  - create/list users
  - change role
  - activate/deactivate
  - reset password
- Recovery/ops tooling:
  - doctor
  - reconcile (+ optional fix)
  - rebuild-db from sidecar meta
  - snapshot/restore
  - report storage

## Storage Layout

Default root: `~/.luna` (configurable via `MEDIA_ROOT`).

Typical layout:

- `items/<item_id>/original/`
- `items/<item_id>/derived/`
- `items/<item_id>/thumbs/`
- `items/<item_id>/photos/`
- `items/<item_id>/meta/item.json`
- `items/<item_id>/meta/assets.json`
- `avatars/`
- `tmp/`
- `db/`

## Requirements

- Go (matching `go.mod`)
- Bun (frontend build stage)
- ffmpeg and ffprobe in runtime environment

## Environment Variables

From `internal/config/config.go`:

- `MEDIA_ROOT`
  - Default: `$HOME/.luna`
- `SQLITE_PATH`
  - Default: `$HOME/.luna/db/app.sqlite`
- `HTTP_ADDR`
  - Default: `:8080`
- `VIDEO_TRANSCODE_CONCURRENCY`
  - Default: `1`
  - Must be `>= 1`
- `LUNA_DEV_PORTSCAN`
  - Default: `false`
  - If true, `serve` scans subsequent ports when configured port is busy

## Local Build and Run

Build embedded frontend + binary:

```bash
task dev
```

Run server:

```bash
./luna serve
```

Run worker (separate terminal):

```bash
./luna worker
```

## Taskfile Commands

- `task install-hooks`
- `task lint`
- `task build-frontend`
- `task prepare-embed`
- `task build-backend`
- `task build`
- `task dev`
- `task clean`
- `task run`
- `task worker`
- `task doctor`

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

User subcommands:

- `luna user create --username <u> --password <p> [--role user|admin]`
- `luna user list [--all|--active|--inactive]`
- `luna user set-role --username <u> --role user|admin`
- `luna user deactivate --username <u> [--reason <text>]`
- `luna user activate --username <u>`
- `luna user passwd --username <u> --password <p>`
- `luna user purge --username <u> [--execute --confirm <username>]`

Item subcommands:

- `luna item purge --id <item-id> [--execute --confirm <item-id>]`

Notes:

- `user purge` and `item purge` are dry-run unless `--execute` is provided and `--confirm` matches.
- `rebuild-db` requires `--force`.

## Docker

This repo includes a multi-stage `Dockerfile`:

- stage 1: Bun builder (frontend only)
- stage 2: Go builder (embeds frontend assets)
- stage 3: runtime (`debian:bookworm-slim` + ffmpeg)

Build image:

```bash
docker build -t luna:local .
```

Compose setup (`docker-compose.yml`) runs two containers:

- `server` (`luna serve`)
- `worker` (`luna worker --id worker-1`)

Host bind mounts are used (not named volumes):

- media root bind: `${LUNA_MEDIA_ROOT_HOST}` -> `/data/media` (use host NFS mount path)
- database bind: `${LUNA_DB_ROOT_HOST}` -> `/data/db` (local host directory)

Compose env defaults:

- `LUNA_MEDIA_ROOT_HOST=/mnt/nfs/luna-media`
- `LUNA_DB_ROOT_HOST=./docker-data/db`

Run:

```bash
docker compose up -d --build
```

## Frontend Source

Frontend source: `web/`

```bash
cd web
bun install
bun run dev
bun run build
```

Use `task dev` / `task build` for integrated embedded builds.
