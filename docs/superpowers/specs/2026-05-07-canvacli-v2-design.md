# canvacli v2.0 — Design Specification

**Date:** 2026-05-07
**Status:** Approved (research complete, awaiting implementation)
**Author:** Catalin Niculescu (with parallel-agent research)
**Builds on:** v1 spec at [`docs/superpowers/specs/2026-05-07-canvacli-design.md`](2026-05-07-canvacli-design.md)

**Research inputs (load-bearing — every claim in this spec traces to one of these):**
- [`docs/research/v2-comments-api.md`](../../research/v2-comments-api.md) — Comments endpoints, no-list-threads gotcha, scopes, rate caps
- [`docs/research/v2-assets-imports-api.md`](../../research/v2-assets-imports-api.md) — Asset upload (octet-stream + header metadata), no list-assets, imports format routing, autofill bridge
- [`docs/research/v2-pages-resize-brand.md`](../../research/v2-pages-resize-brand.md) — Pages API (preview), resize 4 presets only, brand-kit DOES NOT EXIST
- [`docs/research/v2-fts5-sync-design.md`](../../research/v2-fts5-sync-design.md) — FTS5 verified on `modernc.org/sqlite`, external-content tables, `sync_state` cursor, full DDL

---

## 1. Overview

v2.0 is the "Pattern A + expanded surface" milestone. v1 deliberately deferred the full local mirror story; v2 ships it. Three bundles of work land together:

1. **Pattern A — local SQLite mirror with FTS5 search.** Two new commands (`canva sync`, `canva search`) that turn canvacli into the fastest way to find anything in a Canva account from a terminal or an agent prompt. Sync writes through to the existing `cache.db`; search runs `MATCH` queries against external-content FTS5 indexes (verified working on `modernc.org/sqlite v1.50.0` — see fts5-sync-design.md §"FTS5 verification").
2. **Expanded API surface.** Eight new commands covering pages, per-page export, design import, design resize, asset upload/get/patch/delete, and comments (add/thread/archive). Every command has a typed Go API client, a cassette test, and a corresponding `canva_*` MCP tool.
3. **Full MCP coverage.** v1 shipped MCP for the existing surface; v2 extends `internal/mcp/` so every new command is callable from Claude Desktop / Cursor / any MCP client without spawning a shell.

**Headline product story.** v2.0 closes the loop on **agent-driven deck generation**:

```
agent generates hero.png
  → canva assets upload hero.png       → asset_id Msd59349ff
  → canva create --template "Pitch" --autofill data.json
       (data.json references the asset_id)
  → canva export <design-id> --format pdf
```

Every step works without the agent ever touching the Canva web UI. v1 delivered steps 2 and 3; v2 delivers step 1, the asset-upload-to-autofill bridge that the assets-imports research verified end-to-end (assets-imports.md §"Cross-cutting: how assets connect to autofill").

**What v2 deliberately does not chase.** The research surfaced three "obvious" features that are not implementable: a brand-kit endpoint (does not exist), a flat list-assets endpoint (does not exist), and a list-threads-on-a-design endpoint (does not exist). v2 routes around these honestly rather than faking them — see §2 Non-Goals and §9 Local Thread Cache.

---

## 2. Goals & Non-Goals

### Goals

- **Pattern A.** `canva sync` mirrors a Canva account into local SQLite using a per-resource cursor; `canva search` runs FTS5 queries with bm25 ranking across designs / templates / assets / comments.
- **Expanded API surface.** Pages, per-page export, import, resize, asset CRUD, comments (add/thread/archive).
- **Full MCP tool coverage.** Every new CLI command is exposed as a `canva_*` MCP tool with correct read-only/destructive annotations.
- **Backward compatibility.** Every v1 command continues to work unchanged, with the same flags, output shapes, error codes, and exit semantics. v2 is purely additive at the command-surface level.
- **Forward-compatible cache.** v2 schema additions are all `CREATE … IF NOT EXISTS`; opening a v1 cache.db with the v2 binary just adds new tables, indices, triggers, and FTS virtual tables. No migration runner needed.

### Non-Goals (explicitly NOT in v2.0)

- **Brand Kit endpoint.** Verified non-existent in Canva Connect. `GET /v1/brand-kits`, `/v1/brand-kit`, `/v1/brands`, `/v1/brand` all return 404; the OpenAPI spec has zero `BrandKit` schema matches (pages-resize-brand.md §"Brand Kit"). The DDL in fts5-sync-design.md §"Brand kit" is **dropped from v2 implementation** — see §5 below.
- **Flat asset listing.** No `GET /v1/assets` endpoint exists. The closest substitute is `GET /v1/folders/{id}/items?item_types=image`, which is image-only and folder-scoped (assets-imports.md §"Asset list"). v2 does not ship a `canva assets list` command. Folder-walk via the existing `canva folders` command remains the listing path.
- **`canva comments list <design-id>`.** No list-threads endpoint. `canva comments add` / `canva comments thread` / `canva comments archive` work against a **local thread cache** populated by user activity — see §9.
- **Element-level design editing** (Apps SDK territory, not Connect API).
- **Browser automation / scraping.**
- **Embedding-based search** — bm25 only in v2; embeddings deferred to a future milestone.
- **Webhook receiver.** The comments research notes that webhook ingestion would close the comments-list gap, but standing up an HTTP receiver, registering it with Canva, and securing the inbound path is a separate milestone.
- **In-place resize.** Canva's `/v1/resizes` always forks a new design; this is the API contract, not a v2 limitation we can fix.

---

## 3. Architecture Changes from v1

### 3.1 New packages and significant additions

| Path | Purpose | Notes |
|---|---|---|
| `internal/api/pages.go` | `GET /v1/designs/{id}/pages` client | Preview API; offset/limit pagination, no continuation token |
| `internal/api/imports.go` | `POST /v1/imports` + `GET /v1/imports/{id}` | Octet-stream body, `Import-Metadata` header |
| `internal/api/resizes.go` | `POST /v1/resizes` + `GET /v1/resizes/{id}` | Preset vs custom discriminator; trial info on response |
| `internal/api/assets.go` | `POST /v1/asset-uploads` + `GET /v1/asset-uploads/{id}` + `GET/PATCH/DELETE /v1/assets/{id}` | Octet-stream upload, `Asset-Upload-Metadata` header |
| `internal/api/comments.go` | Create thread, create reply, get thread, list replies, get reply | All synchronous — no job/poll envelope |
| `internal/cache/comments.go` | Upsert helpers for `comment_threads` and `comment_replies` | Triggers handle FTS5 sync automatically |
| `internal/cache/assets.go` | Upsert helpers for `assets` | Trigger handles FTS5 sync |
| `internal/cache/pages.go` | Upsert helpers for `design_pages` | No FTS5 (pages don't have searchable text) |
| `internal/cache/sync_state.go` | Read/write `sync_state` cursor rows | Per-resource cursor + watermark + `full_sync_at` |
| `internal/cache/search.go` | FTS5 query builder | Generates `MATCH` queries with bm25 ordering, type filtering |
| `internal/sync/sync.go` | Top-level `canva sync` driver | Iterates resource types in order; tx-per-page |
| `internal/commands/sync.go` | `canva sync` cobra command | Calls `internal/sync` |
| `internal/commands/search.go` | `canva search` cobra command | Calls `internal/cache/search` |
| `internal/commands/pages.go` | `canva pages <design-id>` cobra command | |
| `internal/commands/import.go` | `canva import <file>` cobra command | Extension-based router (§8) |
| `internal/commands/resize.go` | `canva resize <design-id> --to <preset>` cobra command | Preset alias map (§4.3) |
| `internal/commands/assets.go` | `canva assets {upload,get,rm,tag}` cobra subtree | No `assets list` |
| `internal/commands/comments.go` | `canva comments {add,thread,archive}` cobra subtree | No `comments list` |

`internal/api/job.go` (the existing generic poller) is **reused unchanged** — every new async operation (asset upload, import, resize) returns the same `{job:{id,status,result?,error?}}` envelope as autofill and export, so the generic `pollJob[T]` helper covers them with no new code (assets-imports.md §"Polling").

### 3.2 Cache schema additions

Five new tables, four new FTS5 virtual tables, plus the `sync_state` cursor table. Triggers wire content changes into FTS5 automatically. **All statements are `CREATE … IF NOT EXISTS`** so they append cleanly to the existing schema string in `internal/cache/db.go`.

The full DDL is in §5; the source of truth is fts5-sync-design.md §"Proposed v2 schema". One delta from that doc: the **brand-kit tables (`brand_colors`, `brand_fonts`, `brand_logos`) are dropped** because the brand-kit research established the API does not exist.

| New table | Purpose |
|---|---|
| `design_pages` | Per-page metadata for designs (index, dims, thumbnail URL) |
| `comment_threads` | Top-level comment threads (with FTS5 over `root_text`) |
| `comment_replies` | Replies to threads (with FTS5 over `text`) |
| `assets` | Uploaded asset metadata (with FTS5 over `name`) |
| `sync_state` | Per-resource cursor + watermark for incremental sync |

| New FTS5 virtual table | Indexes column |
|---|---|
| `designs_fts` | `designs.title` |
| `templates_fts` | `templates.title` |
| `comment_threads_fts` | `comment_threads.root_text` |
| `comment_replies_fts` | `comment_replies.text` |
| `assets_fts` | `assets.name` |

All FTS5 tables use `tokenize='porter unicode61'` (English stemming + unicode-aware splitting; tradeoff documented in fts5-sync-design.md §"Open questions" #4).

**One-shot rebuild on first v2 open.** v1→v2 upgraders have a populated `designs` and `templates` table but empty FTS5 indexes. The schema-on-open path runs `INSERT INTO designs_fts(designs_fts) VALUES('rebuild')` and the equivalent for `templates_fts`, guarded by a `meta` flag (`fts_rebuilt_v2 = '1'`) so it only runs once. Source: fts5-sync-design.md §"Migration approach".

### 3.3 OAuth scope additions

The v1 scope set was minimal:

```
design:meta:read design:content:read design:content:write
brandtemplate:meta:read brandtemplate:content:read
folder:read profile:read
```

v2 adds **four new scopes**:

| Scope | Used by |
|---|---|
| `comment:read` | `canva comments thread`, `canva comments archive`, `canva sync` (when syncing comments) |
| `comment:write` | `canva comments add` |
| `asset:read` | `canva assets get`, `canva sync` (when syncing assets), thumbnail-URL refresh after upload |
| `asset:write` | `canva assets upload`, `canva assets rm`, `canva assets tag` |

The full v2 scope set requested in `canva login`:

```
design:meta:read design:content:read design:content:write
brandtemplate:meta:read brandtemplate:content:read
folder:read profile:read
asset:read asset:write
comment:read comment:write
```

**Pre-ship requirement.** The Canva developer-app config (managed by the project owner) must be updated to include these four scopes **before** the v2 binary ships, otherwise `canva login` will fail at the consent screen. This is coordination, not code: the developer portal is configured manually, and the change has to land before any user runs `canva login` against a v2 binary. The CHANGELOG entry for v2.0.0 must call out "users must re-run `canva login`" — existing v1 tokens lack the new scopes and will get clean `403 missing_scope` errors that map to exit code 7 with `fix: canva login` (verified live in comments-api.md §"Implementation notes").

---

## 4. v2 Command Surface

Ten new commands, grouped:

### 4.1 Pattern A core

#### `canva sync`

```
canva sync [--prune] [--resource designs|templates|folders|assets|comments]
           [--since <timestamp>] [--full] [--dry-run]
```

All-in-one mirror — no opt-out flag; default sync covers every supported resource type. Flag set:

| Flag | Default | Purpose |
|---|---|---|
| `--resource <name>` | (all) | Sync only one resource type. Repeatable. |
| `--full` | false | Ignore stored cursors; redo full crawl from scratch. Used internally by the 24h reconcile. |
| `--prune` | false | After sync, `DELETE FROM designs WHERE fetched_at < now-30d` (TTL eviction; less correct than full reconcile but cheap). |
| `--since <unix>` | 0 | Skip resources whose `last_synced_at < since`. For scripted use. |
| `--dry-run` | false | List what would be fetched without writing. |
| `--auto-wait` | (universal) | Honor 429 `Retry-After` and block once, capped at 60s. |

**Async vs sync behavior:** `canva sync` is synchronous from the caller's perspective; it walks pages serially per resource and prints a one-line summary per resource on completion. No background job; if it gets killed mid-walk, the next run resumes from the persisted cursor.

**Output (TTY):**
```
designs:    1,247 fetched, 8 updated, 0 deleted (4.2s)
templates:    312 fetched, 0 updated, 0 deleted (1.1s)
folders:       89 fetched, 1 updated, 0 deleted (0.7s)
assets:       654 fetched, 12 updated, 3 deleted (3.5s)
comments:     203 threads across 47 designs (8.3s)
total: 17.8s
```

**Output (non-TTY / `--json`):** one NDJSON record per resource:
```json
{"resource":"designs","fetched":1247,"updated":8,"deleted":0,"duration_ms":4200}
```

**Required scopes:** all v2 scopes, since sync touches every resource type.

**Underlying endpoints:** `GET /v1/designs`, `GET /v1/brand-templates`, `GET /v1/folders/{id}/items` (recursive from `root` and `uploads`), `GET /v1/designs/{id}/pages` (when a design's `updated_at` advances), `GET /v1/designs/{id}/comments/{threadId}` + `…/replies` (only for thread IDs already in the local cache — see §9), `GET /v1/assets/{id}` (only for asset IDs already known from upload history; there is no flat list).

**Known limitation:** assets sync is **upload-history-only**. The `assets` table accumulates rows from `canva assets upload` and from any prior local activity; sync re-`GET`s those IDs to refresh metadata (and rotate the short-lived thumbnail URL) but cannot discover assets uploaded outside canvacli. Documented in `--help`. Same shape as the comments limitation in §9.

**Error codes:** `auth_revoked` (2), `network_error` (4), `rate_limited` (6), `permission_denied` (7), `sync_partial` (1) — `sync_partial` is a new code emitted when one resource fails but others succeeded; the partial state is committed and the next `canva sync` resumes.

#### `canva search`

```
canva search "<query>" [--type designs|templates|comments|assets]
                       [--limit <N>] [--design <id>] [--json]
```

FTS5 query against the local cache. Defaults to all resource types.

| Flag | Default | Purpose |
|---|---|---|
| `--type <name>` | (all) | Restrict to one resource type. |
| `--limit <N>` | 50 | Max results, cap 1000. |
| `--design <id>` | (none) | When `--type comments`, restrict to threads on this design. |
| `--json` / non-TTY | (auto) | NDJSON output. |

**Underlying SQL** (per-type, then UNION ALL'd in the Go query builder):

```sql
-- designs
SELECT 'design' AS kind, d.id, d.title AS snippet, NULL AS design_id, bm25(designs_fts) AS rank
FROM designs d JOIN designs_fts f ON d.rowid = f.rowid
WHERE designs_fts MATCH ?
-- templates
SELECT 'template', t.id, t.title, NULL, bm25(templates_fts)
FROM templates t JOIN templates_fts f ON t.rowid = f.rowid
WHERE templates_fts MATCH ?
-- comment threads
SELECT 'thread', ct.id, substr(ct.root_text, 1, 200), ct.design_id, bm25(comment_threads_fts)
FROM comment_threads ct JOIN comment_threads_fts f ON ct.rowid = f.rowid
WHERE comment_threads_fts MATCH ?
-- comment replies
SELECT 'reply', cr.id, substr(cr.text, 1, 200), ct.design_id, bm25(comment_replies_fts)
FROM comment_replies cr
  JOIN comment_threads ct ON cr.thread_id = ct.id
  JOIN comment_replies_fts f ON cr.rowid = f.rowid
WHERE comment_replies_fts MATCH ?
-- assets
SELECT 'asset', a.id, a.name, NULL, bm25(assets_fts)
FROM assets a JOIN assets_fts f ON a.rowid = f.rowid
WHERE assets_fts MATCH ?
-- All UNION ALL'd, then ORDER BY rank ASC LIMIT N.
```

Per-type bm25 scores are not strictly comparable, but in practice they sort sensibly — the design choice is documented in fts5-sync-design.md §"Open questions" #5.

**Output (NDJSON):**
```json
{"kind":"design","id":"DAF123","title":"Q3 Banner","rank":-4.2}
{"kind":"thread","id":"CMa456","snippet":"Looks great, ship it","design_id":"DAF789","rank":-3.7}
```

**Query syntax** (FTS5 native): `q3 banner` (AND), `launch*` (prefix), `"q3 banner"` (phrase), `banner OR poster` (boolean), `launch NOT draft` (negation). Documented in `--help` and in the shipped `CLAUDE.md`.

**Required scopes:** none — this is a local query.

**Known limitation:** results are only as fresh as the last `canva sync`. The error envelope for an empty cache (no rows in any FTS table) returns `cache_empty` (exit 3) with `fix: canva sync`.

### 4.2 Page-level

#### `canva pages <design-id>`

```
canva pages <design-id> [--limit <N>] [--offset <N>] [--no-cache]
```

Lists all pages of a design. Hits `GET /v1/designs/{designId}/pages?offset=&limit=` — preview API (pages-resize-brand.md §"Pages").

| Flag | Default | Purpose |
|---|---|---|
| `--limit <N>` | 50 | API max 200; canvacli auto-paginates internally if user requests more. |
| `--offset <N>` | 1 | 1-based per Canva API. |
| `--no-cache` | false | Skip `design_pages` cache, force live API. |

**Async vs sync:** synchronous; canvacli iterates `offset=1, offset=1+limit, …` until response has fewer than `limit` items. Total page count comes from `design.page_count` on the parent design (`/pages` itself returns no total — pages-resize-brand.md §"Pages").

**Output (NDJSON):**
```json
{"index":1,"width":1920.0,"height":1080.0,"thumbnail_url":"https://document-export.canva.com/..."}
```

**Required scope:** `design:content:read`.

**Known limitations:**
- Preview API — Canva may break the response shape without notice. The cassette test pins the contract; `canva pages` prints a one-line warning to stderr unless `--quiet` is set.
- `dimensions` and `thumbnail` are absent for unbounded design types (whiteboards). Output handles this with `null` fields.
- Some design types have no pages at all.
- Thumbnail URLs are signed S3 URLs, ~hours of validity. Don't cache them past one CLI invocation.

#### `canva export <id> --pages 2,3` (extension of v1 export)

The existing `canva export` gains a `--pages <comma-list>` flag. The page selector lives inside `format.pages` on the export job body — there is no separate per-page export endpoint (pages-resize-brand.md §"Per-page export").

```
canva export <name|id> --format png|jpg|pdf|gif [--pages 2,3,5] [--output ./path]
```

| Format | `--pages` honored? |
|---|---|
| `png` | yes (one URL per page in `job.urls[]`) |
| `jpg` | yes (one URL per page) |
| `pdf` | yes (single PDF containing only requested pages) |
| `gif` | yes (animated GIF over selected pages) |
| `pptx` | no — whole-deck only; `--pages` rejected with `validation_error` |
| `mp4` | no — same |
| `html_bundle`, `html_standalone` | no — same |

For unsupported formats with `--pages`, exit code 5, error `pages_unsupported_for_format`, `fix: omit --pages or use --format png|jpg|pdf|gif`.

**Required scope:** `design:content:read` (unchanged from v1).

### 4.3 Import / transform

#### `canva import <file>`

```
canva import <file> [--title <string>] [--mime-type <type>] [--no-cache]
```

Routes by file extension (§8). For document formats, hits `POST /v1/imports` with `Content-Type: application/octet-stream` and an `Import-Metadata` header carrying `{"title_base64":"...","mime_type":"..."}`. For image formats, transparently routes to `POST /v1/asset-uploads` and prints a one-line note that imports are for documents (pages-resize-brand and assets-imports together).

**Async vs sync:** async job. Reuses `internal/api/job.go` polling helper. Recommended cadence: 500ms → 1s → 2s → 4s → 8s, cap 8s, deadline 60s for documents (verified live: 1- and 3-page PDFs completed within first 2s poll — assets-imports.md §"Imports").

**Output (success, document import):**
```json
{
  "design_id": "DAHJAjJSyvE",
  "title": "My doc",
  "page_count": 3,
  "edit_url": "https://www.canva.com/design/.../edit?utm_source=...",
  "view_url": "https://www.canva.com/design/.../view?..."
}
```

For multi-page imports that Canva splits into multiple designs (rare, but documented in the OpenAPI), output is NDJSON with one record per design.

**Required scope:** `design:content:write`.

**Known limitations:**
- Rate limits: `POST /v1/imports` 20/min, `GET /v1/imports/{id}` 120/min.
- File size: not documented by Canva. `canva import` does not pre-validate; relies on the API's `400 invalid_file` response. Error code `import_unsupported_format` (§11).
- Unsupported extensions: error code `import_unsupported_format`, `fix: see canva import --list-formats`.

#### `canva resize <design-id> --to <preset>`

```
canva resize <design-id> --to <preset|WxH> [--list-presets]
```

Hits `POST /v1/resizes` with the preset/custom discriminator (pages-resize-brand.md §"Resize"). Side effect: **always creates a NEW design** (in-place resize is not exposed by the Connect API).

`--to` accepts:

| Value | Mode |
|---|---|
| `doc`, `email`, `presentation`, `whiteboard` | `preset` (the only four real Canva presets) |
| `instagram_post`, `instagram_story`, `a4_document`, `facebook_cover`, `youtube_thumbnail`, `twitter_post`, … | client-side aliases → `custom` mode with hard-coded WxH |
| `1080x1920` (literal `WxH`) | `custom` mode, width × height each 40-8000 px |

The full alias map is shipped in `internal/commands/resize.go`. `canva resize --list-presets` prints it.

**Async vs sync:** async job, same generic poll envelope. Total wall-clock ≤ 2 minutes typical.

**Output:**
```json
{
  "new_design_id": "DAHJAuJHXD8",
  "title": "Project DW",
  "page_count": 13,
  "edit_url": "https://...",
  "trial_uses_remaining": 1,
  "upgrade_url": "https://www.canva.com/..."
}
```

`trial_uses_remaining` and `upgrade_url` are surfaced when present (free-tier users get a small trial quota — pages-resize-brand.md §"Resize").

**Required scopes:** `design:content:read` AND `design:content:write` (both required; `x-required-capabilities: resize` per the OpenAPI).

**Known limitations:**
- Free tier gets a small trial quota; after `trial_quota_exceeded`, surface the `upgrade_url`.
- Max 25,000,000 px squared (e.g. 5000×5000 OK, 6000×5000 not).
- Cannot resize Canva docs, emails, or Canva Code designs.

### 4.4 Assets

```
canva assets upload <file> [--name <string>] [--tag <tag>...] [--folder <name|id>]
canva assets get <asset-id>
canva assets rm <asset-id>
canva assets tag <asset-id> --add <tag>... --remove <tag>...
```

**No `canva assets list`** — the API does not expose a flat list endpoint (assets-imports.md §"Asset list"). Folder-scoped listing remains available via the existing `canva folders` walk plus `--type image` filter (a v2.1 follow-up, not blocking v2.0).

#### `canva assets upload <file>`

`POST /v1/asset-uploads` with `Content-Type: application/octet-stream`, raw bytes. Filename + tags ride in an `Asset-Upload-Metadata` HTTP header carrying JSON `{"name_base64":"<base64-of-name>"}` (Base64 lets emoji/Unicode names survive HTTP-header rules).

**Async vs sync:** async job. Recommended polling: 500ms cap 8s, deadline 60s. Uses the generic `pollJob[T]` from v1.

**Output:**
```json
{
  "asset_id": "Msd59349ff",
  "type": "image",
  "name": "My Awesome Upload",
  "tags": ["image","holiday"],
  "thumbnail_url": "https://document-export.canva.com/..."
}
```

The bare `Msd59349ff` is also printable on stdout via `--id-only`, so this composes:

```bash
canva autofill --image-field hero=$(canva assets upload --id-only hero.png)
```

**Required scope:** `asset:write`.

**Known limitations:**
- Image max 50 MB (JPEG, PNG, HEIC, single-frame GIF, TIFF, single-frame WEBP).
- Video max 500 MB (M4V, MKV, MP4, MPEG, QuickTime, WebM).
- No PNG-as-import: PNG goes through `assets upload`, not `imports`.
- Thumbnail URLs are presigned S3 with ~18h validity — re-`GET /v1/assets/{id}` for a fresh URL rather than caching.
- Failure codes: `file_too_big`, `import_failed`, `fetch_failed` (mapped to canvacli code `asset_upload_too_large` / `asset_upload_failed`).

#### `canva assets get <id>` / `rm` / `tag`

- `get`: `GET /v1/assets/{id}` → typed Asset object including type-specific metadata. Scope `asset:read`.
- `rm`: `DELETE /v1/assets/{id}` → 204; mirrors UI trash (does not remove from designs that already use the asset). Scope `asset:write`.
- `tag`: `PATCH /v1/assets/{id}` with new `name` and/or `tags`. **Tags replace, not merge** (assets-imports.md §"Asset get") — canvacli compensates by reading the existing tag list, applying `--add`/`--remove`, and writing the merged list. Scope `asset:write`.

### 4.5 Comments

```
canva comments add <design-id> "<message>" [--assignee <user-id>] [--mention <user-id>...]
canva comments thread <thread-id>
canva comments archive <design-id> [--output <dir>]
```

#### `canva comments add`

`POST /v1/designs/{designId}/comments` with `{message_plaintext, assignee_id?}`. **Synchronous** — no job/poll envelope (comments-api.md §"Implementation notes").

| Flag | Purpose |
|---|---|
| `--assignee <user-id>` | Set thread assignee. Must also be mentioned in message — canvacli auto-injects `[user_id:team_id]` if not present. |
| `--mention <user-id>` | Add a `[user_id:team_id]` mention tag. Repeatable. `--mention @me` shorthand resolves to the cached self-user-id. |

**Output:**
```json
{
  "thread_id": "CMa456",
  "design_id": "DAF7Q...",
  "author": {"id":"oUnPjZ2k2yuhftbWF7873o","display_name":"Catalin"},
  "created_at": 1715040000
}
```

**Side effect:** the new thread is upserted into `comment_threads` (§9). This is what later enables `canva comments archive`.

**Required scope:** `comment:write`.

**Known limitations:**
- Message 1-2048 chars. Over → `message_too_long` (mapped to validation, exit 5).
- Threads cap at 1000 per design (`too_many_comments`, exit 1).
- No update or delete endpoint — typos are permanent. Documented in `--help`.
- No idempotency header. Retried POSTs after flaky 5xx can create duplicate threads — canvacli mitigates by hashing `(design_id, message_plaintext, assignee_id)` and deferring within-process retries.
- Rate limit: `POST /v1/designs/{id}/comments` 100/min.

#### `canva comments thread <thread-id>`

Walks the thread + replies. `GET /v1/designs/{designId}/comments/{threadId}` for the root, then paginates `GET .../replies` until exhausted.

**Wrinkle:** the thread-get endpoint requires `{designId}` in the path, not just the thread ID. canvacli looks up `design_id` from the local `comment_threads` table; if the thread isn't in the cache, it errors with `comment_thread_not_in_cache` and `fix: canva comments add <design-id> "..."` to seed one (or pass `--design <id>` explicitly).

| Flag | Purpose |
|---|---|
| `--design <id>` | Override the cached design-id lookup. Useful when the user has the thread ID and design ID from the Canva UI URL but never created the thread via canvacli. |

**Output (NDJSON):**
```json
{"kind":"thread","thread":{...full Thread...}}
{"kind":"reply","reply":{...full Reply...}}
{"kind":"reply","reply":{...full Reply...}}
```

**Side effect:** thread + replies are upserted into `comment_threads` and `comment_replies`. This is also how user-supplied thread IDs make their way into the local cache for later `archive` runs.

**Required scope:** `comment:read`.

#### `canva comments archive <design-id>`

Iterates the local cache for all threads on the given design, fetches each thread + replies, and emits NDJSON (one record per thread, with replies inline).

```bash
canva comments archive DAF7Q_-7g9Q --output ./archive/  # writes one .json per thread
canva comments archive DAF7Q_-7g9Q                       # NDJSON to stdout
```

**Walk** (verbatim from comments-api.md §"Get all replies"):
1. `SELECT id FROM comment_threads WHERE design_id = ?` — local cache query.
2. For each thread ID, `GET /v1/designs/{designId}/comments/{threadId}` → `Thread`.
3. For each thread, paginate `GET .../replies` until exhausted → `[]Reply`.
4. Emit one NDJSON record per thread of shape `{"thread": Thread, "replies": []Reply}`.

Read budget: ~50 threads/min steady-state given 100/min on getThread + listReplies.

**Output:**
```json
{"thread":{...},"replies":[{...},{...}]}
```

**Required scope:** `comment:read`.

**Known limitations** (the central one for this command):
- **Threads created via the Canva web UI are not visible** until the user runs `canva comments thread <id>` to seed them (or until a webhook receiver exists, which is out of scope). The `--help` and the README must call this out. Verified non-existent: list-threads endpoint returns 404.
- If the local cache has no threads for the design, exit 3, error `comment_thread_not_in_cache`, `fix: canva comments add <design-id> "..."` or `canva comments thread <id> --design <design-id>`.

### 4.6 MCP tool coverage

Every new CLI command gets a corresponding `canva_*` MCP tool. Annotations follow the v1 convention (`readOnlyHint`, `destructiveHint`).

| CLI command | MCP tool name | `readOnlyHint` | `destructiveHint` |
|---|---|---|---|
| `canva sync` | `canva_sync` | false | false (writes local cache only) |
| `canva search` | `canva_search` | true | false |
| `canva pages` | `canva_pages_list` | true | false |
| `canva export … --pages` | `canva_export` (existing, gains `pages` param) | true | false (read-only API; download is local) |
| `canva import` | `canva_import` | false | false (creates new design) |
| `canva resize` | `canva_resize` | false | false (creates a new design; original untouched) |
| `canva assets upload` | `canva_assets_upload` | false | false |
| `canva assets get` | `canva_assets_get` | true | false |
| `canva assets rm` | `canva_assets_delete` | false | **true** |
| `canva assets tag` | `canva_assets_tag` | false | false |
| `canva comments add` | `canva_comments_add` | false | false |
| `canva comments thread` | `canva_comments_thread` | true | false |
| `canva comments archive` | `canva_comments_archive` | true | false |

`destructiveHint: true` is reserved for **irreversible** operations. `assets rm` qualifies (mirrors UI trash; the asset disappears from the user's library). `resize` and `import` do **not** — they create new designs without touching existing data. `comments add` is irreversible (no edit/delete endpoint) but does not destroy existing data, so `destructiveHint: false` with the irreversibility called out in the tool description.

The `canva_export` tool is **extended** rather than replaced; the existing `format` argument gains an optional `pages: number[]` field (with the per-format compatibility from §4.2).

---

## 5. Cache Schema (full DDL)

Source: fts5-sync-design.md §"Proposed v2 schema". Reproduced here as the implementation contract — the schema string in `internal/cache/db.go` should match this byte-for-byte (modulo the brand-kit drop noted below).

**Brand-kit tables are NOT included** — the brand-kit research established the API does not exist. The DDL block from fts5-sync-design.md §"Brand kit" is dropped from the v2 schema.

```sql
-- ============================================================
-- v2 additions (append to existing v1 schema string)
-- ============================================================

-- Per-page metadata
CREATE TABLE IF NOT EXISTS design_pages (
  design_id     TEXT NOT NULL,
  page_index    INTEGER NOT NULL,         -- 0-based
  thumbnail_url TEXT,
  raw_json      TEXT NOT NULL,
  fetched_at    INTEGER NOT NULL,
  PRIMARY KEY (design_id, page_index),
  FOREIGN KEY (design_id) REFERENCES designs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_design_pages_design ON design_pages(design_id);

-- Comment threads
CREATE TABLE IF NOT EXISTS comment_threads (
  id          TEXT PRIMARY KEY,
  design_id   TEXT NOT NULL,
  author      TEXT,
  root_text   TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER,
  fetched_at  INTEGER NOT NULL,
  raw_json    TEXT NOT NULL,
  FOREIGN KEY (design_id) REFERENCES designs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_threads_design  ON comment_threads(design_id);
CREATE INDEX IF NOT EXISTS idx_threads_updated ON comment_threads(updated_at);

-- Comment replies
CREATE TABLE IF NOT EXISTS comment_replies (
  id          TEXT PRIMARY KEY,
  thread_id   TEXT NOT NULL,
  author      TEXT,
  text        TEXT NOT NULL,
  created_at  INTEGER NOT NULL,
  fetched_at  INTEGER NOT NULL,
  raw_json    TEXT NOT NULL,
  FOREIGN KEY (thread_id) REFERENCES comment_threads(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_replies_thread ON comment_replies(thread_id);

-- Assets
CREATE TABLE IF NOT EXISTS assets (
  id          TEXT PRIMARY KEY,
  name        TEXT NOT NULL,
  type        TEXT NOT NULL,            -- image|video|audio|...
  url         TEXT,
  updated_at  INTEGER,
  fetched_at  INTEGER NOT NULL,
  raw_json    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_assets_type    ON assets(type);
CREATE INDEX IF NOT EXISTS idx_assets_updated ON assets(updated_at);

-- Sync state cursor
CREATE TABLE IF NOT EXISTS sync_state (
  resource_type   TEXT PRIMARY KEY,    -- 'designs' | 'folders' | 'templates' | 'assets'
                                       -- | 'comments_for_<designId>'
  cursor          TEXT,                -- Canva continuation; NULL/'' if exhausted
  last_synced_at  INTEGER NOT NULL,    -- unix seconds of last successful poll
  watermark_at    INTEGER,             -- max(updated_at) seen so far (for non-cursor APIs)
  full_sync_at    INTEGER              -- unix seconds of last full crawl
);

-- ============================================================
-- FTS5 virtual tables (external-content)
-- ============================================================
CREATE VIRTUAL TABLE IF NOT EXISTS designs_fts USING fts5(
  title, content='designs', content_rowid='rowid',
  tokenize='porter unicode61');
CREATE VIRTUAL TABLE IF NOT EXISTS templates_fts USING fts5(
  title, content='templates', content_rowid='rowid',
  tokenize='porter unicode61');
CREATE VIRTUAL TABLE IF NOT EXISTS comment_threads_fts USING fts5(
  root_text, content='comment_threads', content_rowid='rowid',
  tokenize='porter unicode61');
CREATE VIRTUAL TABLE IF NOT EXISTS comment_replies_fts USING fts5(
  text, content='comment_replies', content_rowid='rowid',
  tokenize='porter unicode61');
CREATE VIRTUAL TABLE IF NOT EXISTS assets_fts USING fts5(
  name, content='assets', content_rowid='rowid',
  tokenize='porter unicode61');

-- ============================================================
-- Triggers (one set per FTS-indexed content table)
-- Pattern: insert / update (delete-then-insert) / delete
-- ============================================================
-- designs
CREATE TRIGGER IF NOT EXISTS designs_ai AFTER INSERT ON designs BEGIN
  INSERT INTO designs_fts(rowid, title) VALUES (new.rowid, new.title);
END;
CREATE TRIGGER IF NOT EXISTS designs_au AFTER UPDATE ON designs BEGIN
  INSERT INTO designs_fts(designs_fts, rowid, title) VALUES('delete', old.rowid, old.title);
  INSERT INTO designs_fts(rowid, title) VALUES (new.rowid, new.title);
END;
CREATE TRIGGER IF NOT EXISTS designs_ad AFTER DELETE ON designs BEGIN
  INSERT INTO designs_fts(designs_fts, rowid, title) VALUES('delete', old.rowid, old.title);
END;

-- templates
CREATE TRIGGER IF NOT EXISTS templates_ai AFTER INSERT ON templates BEGIN
  INSERT INTO templates_fts(rowid, title) VALUES (new.rowid, new.title);
END;
CREATE TRIGGER IF NOT EXISTS templates_au AFTER UPDATE ON templates BEGIN
  INSERT INTO templates_fts(templates_fts, rowid, title) VALUES('delete', old.rowid, old.title);
  INSERT INTO templates_fts(rowid, title) VALUES (new.rowid, new.title);
END;
CREATE TRIGGER IF NOT EXISTS templates_ad AFTER DELETE ON templates BEGIN
  INSERT INTO templates_fts(templates_fts, rowid, title) VALUES('delete', old.rowid, old.title);
END;

-- comment_threads
CREATE TRIGGER IF NOT EXISTS comment_threads_ai AFTER INSERT ON comment_threads BEGIN
  INSERT INTO comment_threads_fts(rowid, root_text) VALUES (new.rowid, new.root_text);
END;
CREATE TRIGGER IF NOT EXISTS comment_threads_au AFTER UPDATE ON comment_threads BEGIN
  INSERT INTO comment_threads_fts(comment_threads_fts, rowid, root_text)
    VALUES('delete', old.rowid, old.root_text);
  INSERT INTO comment_threads_fts(rowid, root_text) VALUES (new.rowid, new.root_text);
END;
CREATE TRIGGER IF NOT EXISTS comment_threads_ad AFTER DELETE ON comment_threads BEGIN
  INSERT INTO comment_threads_fts(comment_threads_fts, rowid, root_text)
    VALUES('delete', old.rowid, old.root_text);
END;

-- comment_replies
CREATE TRIGGER IF NOT EXISTS comment_replies_ai AFTER INSERT ON comment_replies BEGIN
  INSERT INTO comment_replies_fts(rowid, text) VALUES (new.rowid, new.text);
END;
CREATE TRIGGER IF NOT EXISTS comment_replies_au AFTER UPDATE ON comment_replies BEGIN
  INSERT INTO comment_replies_fts(comment_replies_fts, rowid, text)
    VALUES('delete', old.rowid, old.text);
  INSERT INTO comment_replies_fts(rowid, text) VALUES (new.rowid, new.text);
END;
CREATE TRIGGER IF NOT EXISTS comment_replies_ad AFTER DELETE ON comment_replies BEGIN
  INSERT INTO comment_replies_fts(comment_replies_fts, rowid, text)
    VALUES('delete', old.rowid, old.text);
END;

-- assets
CREATE TRIGGER IF NOT EXISTS assets_ai AFTER INSERT ON assets BEGIN
  INSERT INTO assets_fts(rowid, name) VALUES (new.rowid, new.name);
END;
CREATE TRIGGER IF NOT EXISTS assets_au AFTER UPDATE ON assets BEGIN
  INSERT INTO assets_fts(assets_fts, rowid, name) VALUES('delete', old.rowid, old.name);
  INSERT INTO assets_fts(rowid, name) VALUES (new.rowid, new.name);
END;
CREATE TRIGGER IF NOT EXISTS assets_ad AFTER DELETE ON assets BEGIN
  INSERT INTO assets_fts(assets_fts, rowid, name) VALUES('delete', old.rowid, old.name);
END;
```

**Post-DDL one-shot.** On first v2 open (gated by a `meta.fts_rebuilt_v2` flag), run:

```sql
INSERT INTO designs_fts(designs_fts) VALUES('rebuild');
INSERT INTO templates_fts(templates_fts) VALUES('rebuild');
```

`comment_threads_fts`, `comment_replies_fts`, `assets_fts` start empty on v2 first-open (no v1 rows to backfill), so they don't need rebuild.

**Schema version sentinel:**

```sql
INSERT INTO meta(key, value) VALUES ('schema_version', '2')
  ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE meta.value < excluded.value;
```

Future v3 migrations read this sentinel to decide whether to run column-altering migrations that `IF NOT EXISTS` cannot help with.

---

## 6. Sync Algorithm

`canva sync` is the centerpiece of Pattern A. It loops over a fixed list of resource types, persists per-resource cursors, and uses transactional upserts.

### 6.1 Resource order (deterministic)

1. **`folders`** — walk from `root` and `uploads` via `GET /folders/{id}/items` recursively. Cursor key per folder: `folders_<folderId>`. On full traversal, set `sync_state['folders'].full_sync_at = now`.
2. **`designs`** — `GET /designs?continuation=...`. Single linear cursor, key `designs`. Upsert into `designs`; trigger updates `designs_fts`.
3. **`templates`** — `GET /brand-templates`. Cursor key `templates`. Trigger updates `templates_fts`.
4. **`design_pages`** (subordinate to designs) — for each design whose `updated_at` advanced since `last_synced_at`, fetch `GET /designs/{id}/pages` and upsert into `design_pages`.
5. **`assets`** — re-`GET /v1/assets/{id}` for every asset already in the local table (refresh metadata + thumbnail URL). New assets land via `canva assets upload`, not via sync. Cursor key `assets` is unused in this design (no API list endpoint); the loop iterates the local table.
6. **`comments`** — for each thread already in `comment_threads`, re-walk the thread + replies if its design's `updated_at` has advanced. Cursor key per design: `comments_for_<designId>`. New threads only enter the cache via `canva comments add` or `canva comments thread <id>` — no full-list discovery (§9).

### 6.2 Inner loop (per resource)

```
function syncResource(rt, listFn, upsertFn):
    state := readSyncState(rt)
    cursor := state.cursor
    for {
        page, nextCursor, err := listFn(cursor)
        if err != nil: return err          # leave cursor; retry next run
        beginTx()
          for item in page: upsertFn(item) # triggers update FTS5
          writeSyncState(rt, nextCursor, now)
        commitTx()
        if nextCursor == "": break         # caught up
        cursor = nextCursor
    }
    if cursor == "": writeFullSyncAt(rt, now)
```

Each page write is **a single transaction including the cursor advance**. If the page fails halfway, the tx rolls back, the cursor stays put, and the next run retries from the same cursor. No duplicates, no gaps.

### 6.3 Three resource shapes

- **First sync (no cursor stored).** `cursor = ''`; Canva returns the first page and a `continuation`. Loop until empty.
- **Incremental (cursor stored).** Pass last cursor; Canva returns only items after that point. If the server returns `400 invalid_continuation`, blow away the cursor and fall back to a full crawl, filtering by `updated_at > sync_state.last_synced_at` client-side.
- **No cursor support.** Use `watermark_at`: track `max(updated_at)` we've ingested; on next sync, request all items and drop those with `updated_at <= watermark`.

### 6.4 Soft-delete handling (24h reconcile)

Canva does not expose a "deleted since" stream. Trashed designs simply stop appearing in `GET /designs`. v2's mitigation:

- Every Nth sync (default: when `now - sync_state.full_sync_at >= 24h`), do a **full reconcile** for designs/folders/templates: fresh full list ignoring the cursor, then `DELETE FROM designs WHERE id NOT IN (<all observed ids>)`. The `ON DELETE CASCADE` chain cleans `design_pages`, `comment_threads`, `comment_replies`. Triggers remove FTS rows.
- `--full` flag forces this immediately.
- `--prune` adds TTL eviction on top: `DELETE FROM <table> WHERE fetched_at < now - 30d`. Less correct (kills active-but-unmodified rows) but cheap.

### 6.5 Concurrency, locking, partial failure

- SQLite is opened in WAL mode (already configured in v1's `Open()`). Two `canva sync` runs serialize on the writer lock; the second sees the first's committed cursor and effectively no-ops the overlapping pages.
- A `canva sync --resource designs --resource templates` invocation reuses the same sync engine, just with a filtered resource list.
- If sync fails mid-walk, partial state is committed; re-running picks up where it left off.

### 6.6 Targets

- A typical Canva account (≤ 5,000 designs, ≤ 500 templates, ≤ 1,000 assets, ≤ 500 comment threads) finishes a cold sync in **under 60 seconds** on a 100 Mbit connection. This is the success-criterion target in §16.
- Memory: bounded by the largest single page (≤ 100 items), not by the full result set. Streaming.

---

## 7. Search Algorithm

`canva search` is a thin Go wrapper over FTS5. The query builder lives in `internal/cache/search.go`.

### 7.1 Query plan

For a query string `Q`, the builder constructs one `SELECT` per requested resource type (default: all), joined with `UNION ALL`, ordered by `rank` (bm25 ascending = best first), limited.

The full SQL is in §4.1's `canva search` section. Key properties:

- **External-content joins.** Each FTS5 table is queried via `JOIN <table> ON <table>.rowid = fts.rowid`, so results carry the full content row (id, title/text, design_id where relevant) without duplicating it into the index.
- **Discriminated rows.** Each subquery selects a literal `kind` column (`'design'`, `'template'`, `'thread'`, `'reply'`, `'asset'`) so output is self-describing.
- **`bm25(<table>)` ranking.** Per-table bm25 scores are not strictly comparable across tables — this is documented as a known limitation in fts5-sync-design.md §"Open questions" #5. In practice the ordering is intuitive; if it becomes a UX problem, `canva search` will gain a `--group-by-type` flag in v2.1 that emits separate ranked sections.

### 7.2 Limits and defaults

- Default `--limit 50`. Hard cap 1000. Beyond 1000, error `validation_error` with `fix: --limit <=1000`.
- Default `--type` is unset (search across all types). `--type` is repeatable (`--type designs --type comments`).
- Default output: NDJSON if non-TTY, table if TTY.

### 7.3 Query syntax (passed through to FTS5)

| Form | Meaning | Example |
|---|---|---|
| `term term` | AND | `q3 banner` |
| `term*` | Prefix | `launch*` |
| `"term term"` | Phrase | `"q3 banner"` |
| `OR`, `NOT` | Boolean | `banner OR poster`, `launch NOT draft` |

Stemming is on (`porter` tokenizer) so `banners` matches `banner`. Case is folded (`unicode61`).

### 7.4 Empty cache

If the relevant FTS table(s) have zero rows, `canva search` exits 3 with `error: cache_empty`, `fix: canva sync`.

---

## 8. Import Routing

`canva import <file>` inspects the file extension (lowercased) and routes:

| Extension(s) | Endpoint | Notes |
|---|---|---|
| `.png`, `.jpg`, `.jpeg`, `.gif`, `.heic`, `.tiff`, `.webp` | `POST /v1/asset-uploads` | Image. Prints note: "imports are for documents; uploaded as asset instead." |
| `.pdf` | `POST /v1/imports` (mime `application/pdf`) | Live-tested 1- and 3-page PDFs, both work. |
| `.pptx`, `.ppt` | `POST /v1/imports` | PowerPoint. |
| `.docx`, `.doc` | `POST /v1/imports` | Word. |
| `.xlsx`, `.xls` | `POST /v1/imports` | Excel. |
| `.key`, `.pages`, `.numbers` | `POST /v1/imports` | Apple iWork. |
| `.ai`, `.psd` | `POST /v1/imports` | Adobe. |
| `.afdesign`, `.afphoto`, `.afpub`, `.af` | `POST /v1/imports` | Affinity. |
| `.odp`, `.odt`, `.ods`, `.odg` | `POST /v1/imports` | OpenOffice. |
| `.mp4`, `.mov`, `.m4v`, `.mkv`, `.mpeg`, `.webm` | `POST /v1/asset-uploads` | Video — uploaded as asset. |
| (anything else) | error `import_unsupported_format`, exit 5 | `fix: canva import --list-formats` |

The `--mime-type <type>` flag overrides extension sniffing for the imports endpoint (Canva accepts an explicit mime type in `Import-Metadata`). For asset uploads the route is binary either way — the route is determined by the categorical decision "does this go through `/v1/imports` or `/v1/asset-uploads`?".

`canva import --list-formats` prints the routing table.

---

## 9. Local Thread Cache (for comments)

This is the structural workaround for the missing list-threads endpoint. `canva comments archive` and `canva comments thread <id>` work against a local thread cache populated by user activity, not by API discovery.

### 9.1 How threads enter the cache

1. **`canva comments add` succeeds.** The new thread ID is upserted into `comment_threads` (with `design_id`, `root_text`, `author`, `created_at`). The reply IDs (none on creation) are tracked under `comment_replies` as they accumulate.
2. **`canva comments thread <id>` is invoked.** Even if the thread didn't originate from canvacli (user copy-pasted the ID from the Canva UI URL), the GET response upserts the thread + replies into the local cache. From this point on, `archive` will include it.
3. **`canva sync` re-walks already-known threads.** If a design's `updated_at` has advanced since the thread's `fetched_at`, sync re-fetches the thread and replies.

### 9.2 How threads do NOT enter the cache

- Threads created via the Canva web UI by the user or anyone else.
- Threads created via the Canva web UI by *another* user where canvacli is the design owner.
- Threads triggered by the `comment-notification` webhook — webhook ingestion is out of scope for v2 (would close this gap; deferred).

### 9.3 The `archive` walk

```
local: SELECT id, design_id FROM comment_threads WHERE design_id = ?
for each thread_id:
    GET /v1/designs/{design_id}/comments/{thread_id}      # 100/min
    page through GET .../replies                          # 100/min
    emit {"thread": ..., "replies": [...]}
```

Cache responses by `thread_id + updated_at` to avoid re-walking unchanged threads. Read budget: ~50 threads/min steady-state.

### 9.4 Documenting the limitation

The `--help` for `canva comments archive` and the README must call this out plainly:

> `canva comments archive` archives all threads that this canvacli installation has interacted with on this design. Threads created through the Canva web UI are not visible to canvacli until you run `canva comments thread <thread-id>` to pull them in. The Canva Connect API does not expose a list-threads-on-a-design endpoint (verified non-existent on 2026-05-07). If your account uses webhook-driven workflows, a future canvacli version may close this gap.

The error envelope when archiving a design with zero local threads:

```json
{
  "error": "comment_thread_not_in_cache",
  "message": "no threads cached for design 'DAF7Q_-7g9Q'",
  "fix": "canva comments add DAF7Q_-7g9Q \"...\" or canva comments thread <id> --design DAF7Q_-7g9Q",
  "exit_code": 3
}
```

---

## 10. Asset Upload + Autofill Bridge (the killer flow)

This is the v2.0 headline feature. v1 delivered `canva create --autofill`; v2 closes the loop by letting an agent supply images programmatically rather than referencing pre-uploaded asset IDs.

### 10.1 The flow

```bash
# 1. Generate or fetch an image (agent's responsibility)
my-image-tool > hero.png

# 2. Upload to Canva. canvacli prints the bare M-prefixed asset ID to stdout.
HERO_ID=$(canva assets upload --id-only hero.png)
# → "Msd59349ff"

# 3. Construct autofill data referencing the asset.
cat > data.json <<EOF
{
  "hero_image": {"type":"image","asset_id":"$HERO_ID"},
  "headline":   {"type":"text","text":"Q3 Launch — Pitch"}
}
EOF

# 4. Generate the design from a brand template.
DESIGN_ID=$(canva create --template "Pitch Deck" --autofill data.json --id-only)

# 5. Export to PDF.
canva export "$DESIGN_ID" --format pdf --output ./pitch.pdf
```

### 10.2 Why this works

- The `asset_id` returned by `POST /v1/asset-uploads` (after polling to `success`) has the `M`-prefix-plus-8-hex format (e.g. `Msd59349ff`).
- The autofill API's `DatasetImageValue` schema takes `{type:"image", asset_id:"<M…>"}` verbatim.
- **No transformation, no separate "design library" step.** Verified in assets-imports.md §"Cross-cutting: how assets connect to autofill" against the live OpenAPI.

### 10.3 What v2 must guarantee

- `canva assets upload --id-only` prints the bare `M…` asset_id to stdout, nothing else. Errors go to stderr.
- `canva create --id-only` prints the bare design_id to stdout.
- The `--autofill` JSON is passed through unchanged to the API; canvacli does not mangle the asset reference.

### 10.4 README placement

This flow is the lead example in the v2 README's "What's new" section, with the four-line shell snippet copy-pastable.

---

## 11. Error Handling Additions

New error codes in v2, each with a stable string code, an exit code, and a `fix` string. Source of truth for the existing v1 codes: v1 spec §7.2/§7.3.

| Code | Exit | When | `fix` |
|---|---|---|---|
| `import_unsupported_format` | 5 | `canva import` invoked on an extension not in §8's table. | `canva import --list-formats` |
| `resize_invalid_preset` | 5 | `canva resize --to <name>` where `<name>` is neither a Canva preset nor a known client-side alias nor a `WxH` literal. | `canva resize --list-presets` |
| `resize_trial_quota_exceeded` | 7 | Free-tier user used their resize trial quota. | (echoes `upgrade_url` from the response) |
| `comment_thread_not_in_cache` | 3 | `canva comments archive` finds no threads for the design, or `canva comments thread <id>` cannot resolve `design_id` from cache. | `canva comments add <design-id> "..."` or `canva comments thread <id> --design <design-id>` |
| `asset_upload_too_large` | 5 | API responded `file_too_big`. | `(no fix; reduce file size)` |
| `asset_upload_failed` | 1 | API responded `import_failed` or `fetch_failed` on asset upload. | `canva assets upload --debug` |
| `cache_empty` | 3 | `canva search` invoked when the relevant FTS index has zero rows. | `canva sync` |
| `sync_partial` | 1 | One resource failed but others succeeded; partial state was committed. | `canva sync --resource <failed-resource>` |
| `pages_unsupported_for_format` | 5 | `canva export --pages` invoked with `pptx`/`mp4`/`html_*`. | `omit --pages or use --format png\|jpg\|pdf\|gif` |
| `invalid_continuation` | 1 | Server rejected a stored cursor (Canva rotated cursor format). canvacli auto-recovers by clearing the cursor; emit this as a warning, not a hard error. | (auto-recovered; next sync does full crawl) |

The `fix` field is always populated by canvacli itself, never interpolated from API or user data — preserves the v1 prompt-injection defense from v1 spec §7.8.

---

## 12. Testing Strategy

Cassette tests for every new endpoint. Cassettes are recorded once with a real token and replayed in CI.

### 12.1 Cassette inventory (new in v2)

```
testdata/cassettes/v2-pages.yaml                     # GET /v1/designs/{id}/pages
testdata/cassettes/v2-export-pages.yaml              # POST /v1/exports with format.pages = [2,3]
testdata/cassettes/v2-import-pdf.yaml                # POST /v1/imports (PDF, multi-page)
testdata/cassettes/v2-import-pptx.yaml               # POST /v1/imports (PPTX)
testdata/cassettes/v2-resize-preset.yaml             # POST /v1/resizes (preset: presentation)
testdata/cassettes/v2-resize-custom.yaml             # POST /v1/resizes (custom: 1080x1920)
testdata/cassettes/v2-asset-upload.yaml              # POST /v1/asset-uploads + GET poll
testdata/cassettes/v2-asset-get.yaml                 # GET /v1/assets/{id}
testdata/cassettes/v2-asset-patch.yaml               # PATCH /v1/assets/{id}
testdata/cassettes/v2-asset-delete.yaml              # DELETE /v1/assets/{id}
testdata/cassettes/v2-comments-add.yaml              # POST .../comments
testdata/cassettes/v2-comments-thread.yaml           # GET .../comments/{threadId}
testdata/cassettes/v2-comments-replies.yaml          # GET .../comments/{threadId}/replies (paginated)
testdata/cassettes/v2-comments-add-reply.yaml        # POST .../comments/{threadId}/replies
```

### 12.2 Unit tests

| Package | Coverage targets |
|---|---|
| `internal/cache` (new helpers + `search.go` + `sync_state.go`) | ≥ 80% — pure logic; deterministic SQL fixtures. |
| `internal/sync` | ≥ 80% — driven by mocked `listFn`/`upsertFn`. |
| `internal/commands/import` (extension router) | 100% on the routing table. |
| `internal/commands/resize` (preset alias map) | 100% on the alias table. |
| `internal/api/{pages,imports,resizes,assets,comments}` | Cassette-driven end-to-end. |
| `internal/resolver` (extensions for asset/comment-thread resolution) | ≥ 80%. |

### 12.3 New CI invariants

- **Schema budget reaffirmed.** v1's `canva schema --compact ≤ 4 KB` and `--full ≤ 16 KB` budgets remain. v2 must fit ten new commands under those budgets — measured in CI.
- **FTS5 invariant test.** A Go test inserts a known row, runs `MATCH`, asserts the row comes back. Catches a `modernc.org/sqlite` upgrade that drops FTS5.
- **Migration test.** Open a v1-shaped cache.db, run v2 schema, assert all v2 tables/indices/triggers/FTS tables exist and that `meta.schema_version = '2'`. Then re-open and assert no error and no double-rebuild.
- **Sync-cursor test.** Mock a paged endpoint, run `syncResource`, kill it after page 2, restart, assert it resumes from page 3 with no duplicate rows.

### 12.4 Smoke test additions

```yaml
# .github/workflows/smoke.yml — additions
- run: ./canvacli sync --dry-run
- run: ./canvacli search "anything" --json | head -1
- run: ./canvacli pages --help
- run: ./canvacli import --list-formats
- run: ./canvacli resize --list-presets
- run: ./canvacli assets upload --help
- run: ./canvacli comments add --help
- run: ./canvacli mcp serve --list-tools | jq '.tools[] | select(.name | startswith("canva_"))' | wc -l
  # asserts ≥ 18 tools (v1 has 6: whoami/list/folders/export/sql/schema;
  # v2 adds 12 new + extends canva_export with a `pages` arg = 18 total.
  # `canva_sync`, `canva_search`, `canva_pages_list`, `canva_import`,
  # `canva_resize`, `canva_assets_upload`, `canva_assets_get`,
  # `canva_assets_delete`, `canva_assets_tag`, `canva_comments_add`,
  # `canva_comments_thread`, `canva_comments_archive`.)
```

---

## 13. Distribution / Release

v2.0 ships under the existing v1 release pipeline (GoReleaser + GitHub Actions, cross-compiled for darwin-arm64/amd64, linux-amd64/arm64, windows-amd64). The pipeline itself does not need changes; the implementation plan handles:

- **CHANGELOG.md** — new section for v2.0.0 listing the ten new commands, four new scopes, and the Pattern A bullets. Must call out "users must re-run `canva login`" explicitly.
- **README.md** — new "What's new in v2" section leading with the asset-upload + autofill bridge example (§10).
- **CLAUDE.md (root)** — extend the command table; keep the file under the 150-line cap from v1 §7.5.
- **Tag `v2.0.0`** on master triggers GoReleaser; Homebrew tap auto-updates.
- **Brew upgrade path.** `brew upgrade canvacli` should pull v2 cleanly. Verified by a release-time spot check (v1's release pipeline already exercises this).

The four new OAuth scopes need to be added to the Canva developer-app config **before** the tag, or every `canva login` against the v2 binary will fail at consent. This is a coordination step, not a code step — flagged in §3.3 and in the success criteria (§16).

---

## 14. Known Risks

| Risk | Likelihood | Impact | Mitigation |
|---|---|---|---|
| Pages API is preview — Canva may change response shape | Medium | Medium | Cassette test pins contract; `canva pages` warns on stderr unless `--quiet`; CHANGELOG note. (Source: pages-resize-brand.md §"Pages".) |
| Comments API endpoints all flagged preview | Medium | Medium | Same — cassette tests + CHANGELOG note. (comments-api.md preface.) |
| Comment archive depends on local cache → partial coverage by design | High | Low | Documented prominently in `--help` and README; future webhook receiver closes the gap. (§9.) |
| Four new scopes require dev-app reconfiguration | High | High if missed | Pre-ship checklist in §16; CHANGELOG forces user re-login. |
| `modernc/sqlite` FTS5 perf at large scale | Low | Medium | Verified working on v1.50.0 for thousands of designs; not load-tested at 100k. fts5-sync-design.md §"FTS5 verification" caps the certainty at "fine for our scale". Revisit if user reports >5s search. |
| Import file-size limit not documented by Canva | Medium | Low | Surface API's `400 invalid_file` cleanly; do not pre-validate. |
| Stored cursor expires (Canva rotates format) | Low | Low | Catch `400 invalid_continuation`, null cursor, fall back to full crawl — built into §6.3. |
| Resize trial quota — free-tier users hit the wall | Medium | Low (UX only) | Surface `trial_uses_remaining` and `upgrade_url` from the response. |
| Sync soft-delete drift (stale rows for trashed designs) | Medium | Low | 24h full reconcile (§6.4) with `--full` and `--prune` overrides. |
| Asset thumbnail URL caching | Medium | Low (UX) | Documented as ~18h presigned S3; canvacli re-`GET`s rather than caching. |
| Concurrent `canva sync` runs | Low | Low | WAL mode + writer-lock serialization; second run no-ops overlapping pages. |
| Brand-kit feature requested but unbuildable | High (request) | Low (we drop it) | README "non-goals" section; error code `brand_kit_unavailable` if any user-facing surface ever references it. |

---

## 15. Open Questions Resolved by Research

| Question | Resolution | Source |
|---|---|---|
| Is FTS5 available on `modernc.org/sqlite`? | Yes — verified inline with `CREATE VIRTUAL TABLE … USING fts5`. | fts5-sync-design.md §"FTS5 verification" |
| External-content vs duplicated-text FTS5? | External-content (`content='designs'`) — single source of truth, small index. | fts5-sync-design.md §"Working syntax cheatsheet" |
| Triggers vs explicit FTS inserts? | Triggers — multiple new write sites in v2; "forget to call insert" is real risk. | fts5-sync-design.md §"Keeping FTS5 in sync" |
| Tokenizer? | `porter unicode61` (English stemming + unicode word splitting). | fts5-sync-design.md §"Open questions" #4 |
| Cursor stability? | Treat as opaque, expect occasional `400 invalid_continuation` and full-crawl fallback. | fts5-sync-design.md §"Open questions" #1 |
| Soft-delete handling? | 24h full-reconcile; no "deleted since" stream. | fts5-sync-design.md §"Soft-deleted resources" |
| List-threads endpoint exists? | **No.** Live 404 verified. Local thread cache is the workaround. | comments-api.md §"List threads — NOT SUPPORTED" |
| List-assets endpoint exists? | **No.** Live 404. Folder-walk with `--type image` is the substitute. | assets-imports.md §"Asset list" |
| Brand-kit endpoint exists? | **No.** Every plausible path 404; zero OpenAPI matches for `BrandKit`. v2 drops the brand-kit story entirely. | pages-resize-brand.md §"Brand Kit" |
| Comments scopes? | `comment:read` + `comment:write`. Verified live via 403 envelope. | comments-api.md §"Required scopes" |
| Asset scopes? | `asset:read` + `asset:write`. Verified live via 403. | assets-imports.md §"Required scopes" |
| Asset upload body shape? | `application/octet-stream` raw bytes + `Asset-Upload-Metadata` header with `name_base64`. **Not multipart.** | assets-imports.md §"Asset upload" |
| Imports body shape? | `application/octet-stream` + `Import-Metadata` header. PNG/JPG NOT accepted by `/v1/imports`. | assets-imports.md §"Imports" |
| Per-page export endpoint? | None — extend the existing `/v1/exports` job with `format.pages: int[]`. | pages-resize-brand.md §"Per-page export" |
| Resize preset enum? | Exactly four: `doc`, `email`, `presentation`, `whiteboard`. Everything else must go through `custom`. | pages-resize-brand.md §"Resize" |
| Resize side effects? | Always creates a new design. No in-place resize in Connect. | pages-resize-brand.md §"Resize" |
| Asset-id format and autofill linkage? | `M`-prefix + 8 hex chars; same ID accepted verbatim by `DatasetImageValue.asset_id`. | assets-imports.md §"Cross-cutting" |
| Comment thread/reply rate limits? | Thread create 100/min; **reply create 20/min** (4-5x stricter). | comments-api.md §"Rate limits" |
| Comment endpoints async? | All synchronous — no job/poll envelope. | comments-api.md §"Implementation notes" |

---

## 16. Success Criteria

v2.0 ships when **all of the following** are true:

1. **All 10 new commands implemented** — `canva sync`, `canva search`, `canva pages`, `canva export --pages` (extension of v1 export), `canva import`, `canva resize`, `canva assets upload`, `canva comments add`, `canva comments thread`, `canva comments archive` — and pass cassette tests against `testdata/cassettes/v2-*.yaml`. Three additional asset CRUD commands (`canva assets get`, `canva assets rm`, `canva assets tag`) ship alongside; these are minor extensions on top of the upload command and use the same `internal/api/assets.go` client.
2. **`canva sync` mirrors a real account in under 60 seconds** for the typical-size profile (≤ 5,000 designs / ≤ 500 templates / ≤ 1,000 assets / ≤ 500 threads), measured on a 100 Mbit connection.
3. **`canva search` returns relevant FTS5 hits** with bm25 ranking — verified by a fixed-corpus test where a known query returns known results in known order.
4. **`canva mcp serve` exposes all new tools** with correct `readOnlyHint` / `destructiveHint` annotations (table in §4.6). Smoke test in CI asserts ≥ 18 `canva_*` tools (6 existing + 12 new; the existing `canva_export` is extended with a `pages` arg rather than duplicated).
5. **README + CLAUDE.md document the new surface** — README has the §10 asset-upload-to-autofill example; CLAUDE.md table includes every new command; both stay under their token budgets.
6. **CI green on master** — schema budget, FTS5 invariant, migration test, sync-cursor test, all new cassette tests, full smoke.
7. **v2.0.0 tag triggers the release pipeline cleanly** — GoReleaser publishes static binaries for all five platforms, Homebrew tap auto-updates, SLSA attestation attached.
8. **Brew upgrade path verified** — `brew upgrade canvacli` on a machine with v1 installed pulls v2 and `canva --version` reflects v2.0.0.
9. **Pre-ship: four new OAuth scopes added to the Canva developer-app config** (`asset:read`, `asset:write`, `comment:read`, `comment:write`) so `canva login` succeeds against v2. This must happen before the tag.
10. **CHANGELOG entry calls out the re-login requirement.** Existing v1 tokens lack the new scopes; users running v2 commands that need them get clean `403 missing_scope` mapped to exit 7 with `fix: canva login`.

---

*End of v2.0 design specification.*
