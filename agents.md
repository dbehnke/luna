# agents.md — Family Media (Shorts + Long) Replacement Plan
_Agent-friendly implementation plan for OpenCode / agentic coding._

## 0) Project Summary
Build an intranet-only “FamilyTube” replacement for MediaCMS with:
- **TikTok/YouTube-Shorts style vertical feed** + **long-form video library**
- Kid **accounts** with optional **personas/characters** (“upload as”)
- Upload + background processing:
  - standardized MP4 output
  - thumbnails
  - clip/short generation (trim-only MVP)
- Optional **photo storage** (original + thumb + display)
- Simple social:
  - likes/dislikes, favorites, playlists, highlights
- Storage root: `/data/media`
- Backend: **Go 1.25+**, SQLite (pure Go) + GORM, `go:embed` SPA
- Frontend: **Vue 3 + Vue Router + Vite** (Bun), mobile-first
- Single binary with subcommands: `app serve`, `app worker`, `app import`, `app rebuild-db`, etc.
- **Sidecar meta files** per item so the DB can be rebuilt if SQLite is lost.

Non-goals (MVP):
- Full editing (text overlays, music, filters)
- External sharing / public internet hardening
- HLS/ABR streaming (start with MP4 progressive + faststart; add later if needed)

---

## 1) Key Product Requirements (MVP)
### Accounts & Personas
- Each kid has their own login (username/password).
- Each user can create multiple **personas** (characters).
- Upload screen has **“Upload as”** selector (self or persona).
- Display persona badge/avatar on content cards and in player overlays.

### Media Types
- Video: long-form + shorts/clip support
- Photo: store originals + generate thumbs + display sizes

### Feeds & UI
- **Shorts feed**: vertical swipe, autoplay, pause on tap, prefetch next.
- **Latest**: chronological list/grid.
- **Library**: filter by user/persona, favorites, highlighted.
- **Playlists**: create/edit, add/remove items, reorder.
- Reactions: thumbs up/down, favorite toggle.

### Safety / Deletes
- Kids can delete their own items (soft delete).
- Admin-only purge (optional). Provide “Trash” view and optional retention.

### Processing
- When video uploaded:
  1) probe (ffprobe)
  2) transcode to standardized master MP4 with faststart
  3) thumbnails (webp)
  4) optional preview mp4
- When clip created:
  - trim range + (optional) 9:16 center crop + encode to vertical mp4

---

## 2) Storage Layout (/data/media)
Use ULID or UUID for stable IDs.

All media is stored under a single root directory.  
Each media item lives in its own immutable-ID folder.

- /data/media/
  - items/
    - <item_id>/
      - original/
        - upload.<ext>
      - derived/
        - master.mp4
        - short_<clip_id>.mp4
        - preview.mp4
      - thumbs/
        - t_0001.webp
        - t_0002.webp
      - photos/
        - original.<ext>
        - display.webp
        - thumb.webp
      - meta/
        - item.json
        - assets.json
        - history.log
  - tmp/
    - uploads/
    - jobs/
  - db/
    - luna.sqlite


### Sidecar Meta Files
Purpose: rebuild DB if SQLite fails.

- `meta/item.json` — canonical item metadata (owner, persona, title, timestamps, flags)
- `meta/assets.json` — list of derived assets (master, clips, thumbs, photo sizes)

**Write atomically**:
- write `*.tmp`, `fsync`, `rename`.

---

## 3) Database Model (SQLite + GORM)
### Users/Personas
- `users`: id, username, password_hash, role, created_at
- `personas`: id, user_id, display_name, avatar_path, created_at

### Media
- `media_items`:
  - id, user_id, persona_id(nullable), type(video|photo)
  - title, description, created_at, updated_at
  - is_highlighted, deleted_at(nullable)
- `video_sources`:
  - id, media_item_id, original_filename, storage_path, sha256
  - duration_ms, width, height, fps, rotation
- `video_assets`:
  - id, media_item_id
  - kind(master_mp4|short_clip|preview_mp4|hls_future)
  - storage_path, width, height, bitrate, codecs
  - clip metadata (clip_id, start_ms, end_ms, crop_mode) for kind=short_clip
- `photo_assets`:
  - id, media_item_id, kind(original|display|thumb), storage_path, width, height

### Social
- `reactions`: id, user_id, media_item_id, value(-1/0/1), unique(user_id, media_item_id)
- `favorites`: id, user_id, media_item_id, unique(user_id, media_item_id)
- `playlists`: id, user_id, name, created_at
- `playlist_items`: id, playlist_id, media_item_id, position, unique(playlist_id, media_item_id)

### Jobs (DB-backed queue)
- `jobs`:
  - id, type(probe|transcode|thumbs|clip|photo_thumb|cleanup|write_meta)
  - status(queued|running|failed|done)
  - priority(int), payload_json(text)
  - attempts, max_attempts
  - locked_at, locked_by
  - run_after
  - created_at, updated_at

---

## 4) Backend Architecture
### Single Go Binary, Subcommands
- `app serve`:
  - serves embedded SPA
  - JSON API
  - static file server for `/data/media/items/...` (read-only)
- `app worker`:
  - pulls jobs from SQLite
  - runs ffmpeg/ffprobe
  - writes derived assets and updates DB + sidecar meta
- `app import mediacms`:
  - filesystem-only import and optional Postgres metadata import
- `app rebuild-db`:
  - scans `/data/media/items/*/meta/item.json` and rebuilds DB
  - optionally enqueues missing processing jobs
- `app doctor`:
  - checks ffmpeg availability, permissions, disk space, writeability

### API Considerations
- All endpoints require auth (intranet but still).
- Serve media via HTTP range requests for MP4 seeking.
- Use progressive MP4 with `faststart` for quicker playback.

---

## 5) Job Queue / Worker Rules
### Claiming Jobs (SQLite)
Transaction:
1) select one queued job where `run_after <= now` and `locked_at is null`
   order by `priority desc, created_at asc`
2) update set:
   - status=running
   - locked_at=now
   - locked_by=<worker_id>

Stale lock recovery:
- periodic reaper: if running and `locked_at < now - lock_timeout`, set queued again.

Concurrency:
- Video transcode: `VIDEO_TRANSCODE_CONCURRENCY=1` (optionally 2)
- Thumbs: 2
- Photos: 2

Priorities:
- thumbs > clip > transcode (optional; thumbs first improves perceived speed)

---

## 6) Media Processing Specs (MVP)
### Video Master (Long)
- Output: `derived/master.mp4`
- Codec: H.264 + AAC
- Max res: 1080p (scale down if larger)
- `faststart` enabled (moov atom at beginning)
- Handle rotation metadata (bake into pixels)

### Shorts Clip
- Output: `derived/short_<clip_id>.mp4`
- Target: 1080x1920
- Crop mode MVP: `9:16_center`
- Trim: start_ms/end_ms
- Encode: H.264 + AAC

### Thumbnails
- WebP thumbs at e.g. 10%, 50%, 90% timestamps (or 1 + user selectable later)

### Photos
- Store original
- Generate:
  - display (e.g. max 2048 on long edge)
  - thumb (e.g. 480)

---

## 7) Frontend (Vue 3) Pages & Components
### Routes
- `/login`
- `/` (Latest)
- `/shorts`
- `/library`
- `/playlists`
- `/watch/:id`
- `/admin` (optional minimal)

### Shared Components
- `UploadModal` with “Upload as” persona selector
- `VideoCard`, `PhotoCard`
- `PersonaBadge`
- `ReactionBar` (up/down/fav/add-to-playlist)
- `ShortsPlayer` (vertical swipe, prefetch next, autoplay)

### Shorts UX
- Full-screen `video` element with:
  - playsinline
  - mute toggle (optional default muted)
  - swipe navigation
  - overlay controls (like/fav/playlist)

---

## 8) Import / Migration Plan (MediaCMS)
### Import Level 1: Filesystem-only
- Walk MediaCMS media directory
- For each file:
  - create `media_item` (video)
  - write `item.json` and `assets.json` skeleton
  - enqueue `probe`, then `transcode`, `thumbs`

### Import Level 2: Postgres metadata (optional)
- Connect to MediaCMS Postgres
- Read tables for:
  - title, description, created_at, owner mapping
- Map MediaCMS users to local users by username/email
- Preserve timestamps for timeline correctness

---

## 9) DB Recovery (Critical Feature)
### `app rebuild-db`
- Scan `/data/media/items/*/meta/item.json`
- Recreate:
  - users (best-effort by username; or create placeholder)
  - personas (if present in meta)
  - media_items, sources, assets, thumbs, photo_assets
- Validate file existence:
  - if master missing -> enqueue transcode
  - if thumbs missing -> enqueue thumbs
  - if clip referenced but missing -> enqueue clip job

---

## 10) Implementation Milestones (Agent Tasks)
### Milestone A — Skeleton
- Go module layout
- config loading (env + flags)
- SQLite + GORM setup + migrations
- CLI subcommands wired
- basic auth (session cookie or JWT; cookie preferred intranet)

### Milestone B — Media storage + meta writing
- media root path manager
- atomic meta file writer
- static file server for `/data/media/items/*` (read-only)

### Milestone C — Upload flow
- simple upload (multipart) to `original/upload.<ext>`
- create media_item + source
- enqueue probe/transcode/thumbs
- API to poll processing status

### Milestone D — Worker + ffmpeg
- job claim loop
- ffprobe parse
- transcode master mp4
- thumbnails
- update DB + assets.json

### Milestone E — Frontend MVP
- login
- upload
- latest/library grid
- watch page
- favorites + playlists
- processing status UI

### Milestone F — Shorts
- clip creation UI (trim start/end; center crop 9:16)
- shorts feed API + UI (swipe player)
- likes/dislikes for shorts (reactions on media_item or clip asset; pick one and be consistent)

### Milestone G — Photos
- photo upload
- thumb/display generation (can be imagemagick/libvips or go image libs; choose simplest)
- gallery view

---

## 11) Decisions Locked In
- Storage root: `/data/media`
- MP4 progressive download for MVP; no HLS initially
- Video transcode concurrency: 1–2 max
- Kids can delete their own content (soft delete); admin purge optional
- Trim-only clips for shorts MVP
- Sidecar meta files required for DB recovery
- Single binary with subcommands

---

## 12) Open Questions (keep minimal)
1) Auth choice: cookie sessions vs JWT? (recommend cookie + server session in SQLite)
2) Clip identity: treat clips as `video_assets` linked to `media_item` (recommended)
3) Photo processing tool: pure Go image resize vs external tool (libvips/imagemagick). (Recommend pure Go first.)

---

## 13) Agent Execution Notes (for OpenCode)
- Prefer incremental PR-sized changes per milestone.
- Add integration tests for:
  - meta write atomicity
  - rebuild-db scanning correctness
  - job claim locking + stale lock recovery
- Never hard-delete media files in MVP; always soft delete + trash.

---

## 14) Current Repo Reality (Audit Snapshot)
This section reflects current implementation status in this repository and should be treated as source of truth for near-term execution.

### Implemented
- Single binary + subcommands: `luna serve`, `luna worker`, `luna import`, `luna rebuild-db`, `luna user ...`, `luna doctor`, `luna reconcile`, `luna snapshot`, `luna restore`, `luna report-storage`.
- Auth: cookie + server session persisted in SQLite.
- Roles: `admin` and `user`, with activation/deactivation flows.
- Personas: CRUD, avatar upload, profile routes, and upload persona selection.
- Upload pipeline: video/photo/audio upload + background jobs.
- Video jobs: probe, transcode MP4, thumbs, short clip generation.
- Photo jobs: display/thumb generation.
- Sidecar metadata writing + DB rebuild from sidecars.
- Frontend routes implemented: `/login`, `/`, `/shorts`, `/library`, `/upload`, `/item/:id`, `/@:slug`, `/me/profile`, `/admin/users`.
- Embedded frontend workflow exists (`go:embed` from `cmd/luna/frontend`) with Taskfile helpers.

### Implemented Beyond Original MVP Scope
- Audio item type (`audio`) with transcode to `master.m4a`.
- HLS generation path and playback fallback support for videos.

### Not Yet Implemented (High Confidence Gaps)
- Playlists end-to-end: DB tables, API, and frontend routes/UI.
- Trash view + admin purge workflow for soft-deleted items.
- Per-item/admin purge command (current purge is user-centric and destructive).
- Import Level 2 (MediaCMS Postgres metadata mapping).
- Preview MP4 artifact generation.
- Explicit queue integration tests for claim locking + stale lock recovery.

### Doc/Plan Drift To Reconcile
- Plan says storage root is locked to `/data/media`; runtime defaults currently to `~/.luna` unless `MEDIA_ROOT` is set.
- Plan says route `/watch/:id`; current route is `/item/:id`.
- Plan references `app import mediacms`; current command is `luna import --src ...`.
- Plan lists `video_sources`, `video_assets`, `photo_assets`, `playlists`, `playlist_items`; current schema uses `media_items`, `clip_assets`, sidecar asset metadata, and no playlist tables yet.

---

## 15) Next Execution Batches (Subagent-Friendly)
1) **Data Model/Backend Batch**
- Add `playlists` + `playlist_items` models, migrations, and APIs (CRUD + reorder + membership).
- Add media-item soft-delete listing endpoints (`trash`) and admin purge endpoint/CLI for item-level hard delete.
- Add queue integration tests for locking + stale lock requeue.

2) **Frontend Batch**
- Add `/playlists` page and playlist controls on item/short cards.
- Add Trash management UI (owner view + admin-only purge actions).
- Keep `/item/:id` canonical and optionally alias `/watch/:id` for compatibility.

3) **Ops/Import Batch**
- Implement MediaCMS Postgres metadata import mode.
- Add preview MP4 generation job + UI usage if needed.
- Decide and document default deployment mode:
  - force `/data/media` in production, or
  - keep `~/.luna` local default and document required env override.

END.
