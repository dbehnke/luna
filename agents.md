# AGENTS.md - Luna Implementation Guide

This document is the implementation guide and execution context for agent-driven work in this repository.

## 1. Product Scope (Current)

Luna is an intranet media platform focused on family-safe private sharing and playback.

Primary capabilities currently in code:

- User auth with role-based access (`admin`, `user`)
- Persona model owned by users (with avatar + profile)
- Media types: video, audio, photo
- Background processing via DB-backed jobs
- Shorts clip creation, listing, playback, and deletion
- Reactions, favorites, highlights, playlists
- Soft delete + trash + restore
- Admin user management APIs + CLI
- Sidecar metadata (`item.json`, `assets.json`) for DB recovery
- Operational tooling (`doctor`, `reconcile`, `rebuild-db`, `snapshot`, `restore`, reports)

## 2. Architecture

- Backend: Go single binary (`luna`)
- Frontend: Vue SPA, embedded via `go:embed`
- DB: SQLite
- Media processing: ffmpeg/ffprobe worker
- Storage: filesystem rooted at `MEDIA_ROOT`

Main runtime split:

- `luna serve` -> API + embedded frontend + media file serving
- `luna worker` -> async processing (probe/transcode/thumbs/clip/hls/photo)

## 3. Storage and Metadata

Expected hierarchy under `MEDIA_ROOT`:

- `items/<item_id>/original/`
- `items/<item_id>/derived/`
- `items/<item_id>/thumbs/`
- `items/<item_id>/photos/`
- `items/<item_id>/meta/item.json`
- `items/<item_id>/meta/assets.json`
- `avatars/`
- `tmp/`
- `db/` (optional depending on `SQLITE_PATH`)

Metadata writes must be atomic (`tmp` + rename) and remain source-of-truth for rebuild paths.

## 4. Access Rules

Manage permissions should remain consistent across backend and UI:

- Admin can manage all items.
- Item owner can manage own item.
- Persona owner can manage persona-owned item.

Actions that rely on manage permission include edit/delete/reprocess/highlight/clip ops/thumbnail set.

## 5. Implemented UX Notes

- Global sidebar navigation with desktop collapse/pin state and mobile drawer.
- Shorts supports feed + grid preview modes.
- Item page auto-polls processing status.
- Clip list auto-polls clip readiness.
- Thumbnail can be set from current playback position (permission-gated).
- Thumbnail URLs include cache-busting version query to avoid stale browser cache.

## 6. Configuration

Supported env vars (source: `internal/config/config.go`):

- `MEDIA_ROOT`
- `SQLITE_PATH`
- `HTTP_ADDR`
- `VIDEO_TRANSCODE_CONCURRENCY`
- `LUNA_DEV_PORTSCAN`

For containerized deployment, compose-level host path variables are also used:

- `LUNA_MEDIA_ROOT_HOST` (host media/NFS path)
- `LUNA_DB_ROOT_HOST` (host local DB path)

## 7. Command Surface

Top-level commands:

- `serve`
- `worker`
- `import`
- `rebuild-db`
- `user`
- `item`
- `doctor`
- `reconcile`
- `snapshot`
- `restore`
- `report`

Refer to root `README.md` for full flags and examples.

## 8. Engineering Rules for Agents

- Keep binary artifacts out of git.
- Do not commit generated frontend embed assets (`cmd/luna/frontend/`) unless explicitly required by workflow.
- Preserve backward compatibility in metadata format where possible.
- Validate with both:
  - `go test ./...`
  - `cd web && bun run build`
- Prefer incremental commits with clear scope.

## 9. Current Gaps / Forward Work

Planned/possible next improvements:

- More robust shorts editor controls (beyond start/end)
- Better playlist UX and bulk actions
- Additional admin observability views
- Extended import pipeline (metadata mapping)
- Hardening and deployment profiles

---

When this file and code diverge, update this file in the same change batch.
