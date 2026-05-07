# FTS5 + Sync Cursor Design (v2.0 research)

Status: research / pre-implementation. Targets canvacli v2.0. The v1 cache lives in
`internal/cache/db.go`; this doc describes what to add on top of it.

## FTS5 verification in modernc.org/sqlite

Confirmed working on `modernc.org/sqlite v1.50.0` (the version pinned in `go.mod`).
Pure-Go, no CGo, FTS5 enabled by default. Verified inline with this program:

```go
package main

import (
    "database/sql"
    "fmt"
    _ "modernc.org/sqlite"
)

func main() {
    db, _ := sql.Open("sqlite", ":memory:")
    if _, err := db.Exec("CREATE VIRTUAL TABLE t USING fts5(text)"); err != nil {
        fmt.Println("FTS5 not supported:", err); return
    }
    db.Exec("INSERT INTO t VALUES ('hello world')")
    var id int
    db.QueryRow("SELECT rowid FROM t WHERE t MATCH 'hello'").Scan(&id)
    fmt.Println("FTS5 works:", id)
}
```

Output:

```
FTS5 works: 1
```

In addition I confirmed the three features we actually need:

- **`porter` + `unicode61` tokenizer** — `CREATE VIRTUAL TABLE p USING fts5(t, tokenize='porter unicode61')` succeeds. Gives us stemming (banner / banners) and case-folded unicode word splitting out of the box.
- **External-content tables** — `CREATE VIRTUAL TABLE designs_fts USING fts5(title, content='designs', content_rowid='rowid')` works and joins back to the source row by `rowid`. This is the right mode for us: keeps a single source of truth in the content table and avoids duplicating large `raw_json` blobs into the index.
- **`bm25(table)` ranking function** — works in the same query as `MATCH`, so we can `ORDER BY rank` (which is bm25 by default) for relevance-sorted results.

### Working syntax cheatsheet

```sql
-- Create an external-content FTS5 index over an existing table.
CREATE VIRTUAL TABLE designs_fts USING fts5(
    title,
    content='designs',
    content_rowid='rowid',
    tokenize='porter unicode61'
);

-- Query with MATCH; supports phrases, prefixes, AND/OR/NOT.
SELECT d.id, d.title
FROM   designs d
JOIN   designs_fts f ON d.rowid = f.rowid
WHERE  designs_fts MATCH 'q3 banner'           -- AND of two terms
ORDER  BY rank                                 -- bm25 ascending = best first
LIMIT  20;

-- Prefix:    'launch*'      -- matches launch, launches, launching
-- Phrase:    '"q3 banner"'  -- exact ordered phrase
-- Boolean:   'launch NOT draft', 'banner OR poster'
```

### Keeping FTS5 in sync — triggers vs explicit insert

Two options:

1. **Triggers** (`AFTER INSERT/UPDATE/DELETE` on the content table) that mirror into
   `_fts`. Pro: impossible to forget. Con: the trigger runs on every write path,
   including bulk syncs, and you can't skip it for cases where you know the FTS row
   is already correct.
2. **Explicit `INSERT INTO ... _fts` after each upsert** in the Go upsert helpers.
   Pro: keeps writes in one place that's easy to reason about; lets us batch.
   Con: a future code path that bypasses the helpers will silently drop rows from
   the index.

**Recommendation: triggers.** All current writes go through helpers
(`internal/cache/{designs,templates,folders}.go`) but v2 adds enough write sites
(comments, assets, brand kit) that "forget to call the FTS insert" is a real risk.
Triggers also handle the `UPDATE` case (re-tokenise on title change) for free.
Cost is one extra B-tree write per row; for our scale (thousands of designs, not
millions) this is invisible.

## Proposed v2 schema

All statements are `CREATE ... IF NOT EXISTS` so the existing
"run-the-schema-string-on-open" pattern in `Open()` keeps working.

### Design pages (new)

```sql
CREATE TABLE IF NOT EXISTS design_pages (
  design_id   TEXT NOT NULL,
  page_index  INTEGER NOT NULL,         -- 0-based
  thumbnail_url TEXT,
  raw_json    TEXT NOT NULL,
  fetched_at  INTEGER NOT NULL,
  PRIMARY KEY (design_id, page_index),
  FOREIGN KEY (design_id) REFERENCES designs(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_design_pages_design ON design_pages(design_id);
```

Rationale: Canva's `GET /designs/{id}/pages` returns N pages; we want
per-page lookup without parsing `raw_json` every time.

### Comments

Canva models comment threads with one root and N replies. We keep that shape.

```sql
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

CREATE VIRTUAL TABLE IF NOT EXISTS comment_threads_fts USING fts5(
  root_text, content='comment_threads', content_rowid='rowid',
  tokenize='porter unicode61');
CREATE VIRTUAL TABLE IF NOT EXISTS comment_replies_fts USING fts5(
  text, content='comment_replies', content_rowid='rowid',
  tokenize='porter unicode61');
```

A unified `canva search` query against comments looks like:

```sql
SELECT 'thread' AS kind, t.id, t.design_id, t.root_text AS snippet
FROM comment_threads t JOIN comment_threads_fts f ON t.rowid=f.rowid
WHERE comment_threads_fts MATCH ?
UNION ALL
SELECT 'reply', r.id, t.design_id, r.text
FROM comment_replies r
  JOIN comment_threads t ON r.thread_id = t.id
  JOIN comment_replies_fts f ON r.rowid=f.rowid
WHERE comment_replies_fts MATCH ?
ORDER BY rank LIMIT 50;
```

### Assets

```sql
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

CREATE VIRTUAL TABLE IF NOT EXISTS assets_fts USING fts5(
  name, content='assets', content_rowid='rowid',
  tokenize='porter unicode61');
```

### Brand kit

```sql
CREATE TABLE IF NOT EXISTS brand_colors (
  hex        TEXT PRIMARY KEY,         -- '#RRGGBB' upper-case
  name       TEXT,
  fetched_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS brand_fonts (
  name       TEXT PRIMARY KEY,
  weights    TEXT NOT NULL,            -- JSON array, e.g. '[400,700]'
  fetched_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS brand_logos (
  id         TEXT PRIMARY KEY,
  asset_id   TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  FOREIGN KEY (asset_id) REFERENCES assets(id) ON DELETE CASCADE
);
```

Brand kit is small (tens to low hundreds of rows) so no FTS5 — `LIKE '%foo%'`
on `brand_fonts.name` and `brand_colors.name` is fine.

### Templates FTS

Add an FTS5 index for the existing `templates` table so `canva search` covers
template titles too:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS templates_fts USING fts5(
  title, content='templates', content_rowid='rowid',
  tokenize='porter unicode61');
```

And on the existing `designs.title`:

```sql
CREATE VIRTUAL TABLE IF NOT EXISTS designs_fts USING fts5(
  title, content='designs', content_rowid='rowid',
  tokenize='porter unicode61');
```

### Triggers (one per content table)

Pattern repeats for `designs`, `templates`, `comment_threads`, `comment_replies`,
`assets`. Shown for `designs`:

```sql
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
```

The `INSERT ... VALUES('delete', ...)` form is the documented FTS5 idiom for
removing a row from an external-content index.

### Sync state cursor

```sql
CREATE TABLE IF NOT EXISTS sync_state (
  resource_type   TEXT PRIMARY KEY,    -- 'designs' | 'folders' | 'templates' | 'assets'
                                       -- | 'comments_for_<designId>' | 'brand_kit'
  cursor          TEXT,                -- Canva continuation; NULL/'' if exhausted
  last_synced_at  INTEGER NOT NULL,    -- unix seconds of last successful poll
  watermark_at    INTEGER,             -- max(updated_at) seen so far (fallback for non-cursor APIs)
  full_sync_at    INTEGER              -- unix seconds of last full crawl
);
```

`cursor` is opaque (Canva's `continuation` token, see `docs/research/canva-api.md`).
`watermark_at` is for resources that don't support continuation tokens — store the
max `updated_at` we've seen and filter client-side on the next pass.

## Migration approach

The v1 cache uses one big `CREATE TABLE IF NOT EXISTS` string executed on open.
**v2 keeps that pattern.** Append the new statements to the same `schema` const
in `internal/cache/db.go`. Because every statement is `IF NOT EXISTS`, opening a
v1 DB with v2 code just adds the new tables/indexes/triggers; opening a v2 DB
with v2 code is a no-op.

The existing `meta` table is exactly the right place for a `schema_version`
sentinel for *future* migrations (e.g. v3, when we may need to alter columns
which `IF NOT EXISTS` cannot help with). On v2's first open:

```sql
INSERT INTO meta(key, value) VALUES ('schema_version', '2')
  ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE meta.value < excluded.value;
```

That's it. No version-aware migration runner is needed for v2; we just bump the
sentinel so v3 can read it and decide whether to run migrations. Document this
in `db.go` as a comment so the next person knows the contract.

One subtlety: when we add an FTS5 virtual table over a content table that
already has rows (a v1 → v2 upgrade where `designs` is non-empty), the FTS5
index will be **empty** until we backfill it. The schema string should run a
one-shot rebuild guarded by a `meta` flag:

```sql
INSERT INTO designs_fts(designs_fts) VALUES('rebuild');  -- run once after CREATE
```

Wrap it in Go:

```go
if !metaFlag(db, "fts_rebuilt_v2") {
    db.Exec("INSERT INTO designs_fts(designs_fts) VALUES('rebuild')")
    db.Exec("INSERT INTO templates_fts(templates_fts) VALUES('rebuild')")
    setMetaFlag(db, "fts_rebuilt_v2", "1")
}
```

`'rebuild'` is the FTS5-documented command to repopulate an external-content
index from the content table.

## Sync cursor algorithm

Top-level `canva sync` loops over a fixed list of resource types. For each:

```
function syncResource(rt, listFn, upsertFn):
    state := readSyncState(rt)           # may be zero-value (first run)
    cursor := state.cursor               # empty string ⇒ start from beginning
    for {
        page, nextCursor, err := listFn(cursor)
        if err != nil: return err        # leave cursor as-is, retry next run
        beginTx()
          for item in page: upsertFn(item)   # triggers update FTS5
          writeSyncState(rt, nextCursor, now)
        commitTx()
        if nextCursor == "": break       # caught up
        cursor = nextCursor
    }
    if cursor == "": writeFullSyncAt(rt, now)
```

Concrete order in `canva sync`:

1. **`folders`** — walk from root via `GET /folders/{id}/items` recursively.
   Cursor is per-folder (`continuation` from items). Persist cursors as
   `folder_items_<folderId>` so resuming a deep crawl works. Once the whole
   walk completes, set `sync_state['folders'].full_sync_at = now`.
2. **`designs`** — `GET /designs?continuation=...`. Single linear cursor.
   Upsert into `designs`; trigger updates `designs_fts`.
3. **`templates`** — same pattern as designs (`GET /brand-templates`).
4. **`assets`** — `GET /assets` (or per-folder if no top-level list; check
   spec). Single cursor.
5. **`comments`** — for each design seen in step 2, `GET /designs/{id}/comments`
   with cursor key `comments_for_<designId>`. This is O(designs) round-trips;
   batch by checking design `updated_at` and only re-syncing comments for
   designs whose `updated_at > comment_threads.last_synced_at`.
6. **`brand_kit`** — single shot, no cursor (it's small). Replace whole table
   each run (`DELETE FROM brand_colors; INSERT ...` inside a tx).

### Three resource shapes

- **First sync (no cursor stored).** `cursor = ''`; Canva returns the first page
  and a `continuation`. Loop until empty.
- **Incremental (cursor stored).** Pass last cursor; Canva returns only items
  after that point. Caveat: the Canva spec doesn't guarantee that a stale
  cursor stays valid forever. If the server returns `400 invalid_continuation`,
  blow away the cursor and fall back to a full crawl, filtering by
  `updated_at > sync_state.last_synced_at` client-side.
- **No cursor support.** Use the `watermark_at` column: track
  `max(updated_at)` we've ingested, and on next sync request all items and
  drop those with `updated_at <= watermark`. This is the brand kit / per-page
  fallback path.

### Soft-deleted resources

**Canva does not expose a "deleted since" stream.** Verified against the
OpenAPI spec on 2026-05-07 (see `docs/research/canva-api.md` §7): the public
API has no `DELETE /designs/{id}` and no tombstone endpoint. Designs that the
user trashes in the web UI simply stop appearing in `GET /designs`.

This means **incremental sync cannot detect deletions.** The cache will show
stale rows for trashed designs forever unless we periodically reconcile. Two
mitigations:

1. **Periodic full reconcile.** Every Nth sync (configurable, default every
   24 hours), do a fresh full list of `/designs` ignoring the cursor and
   `DELETE FROM designs WHERE id NOT IN (<all observed ids>)`. The
   `ON DELETE CASCADE` chain cleans up `design_pages`, `comment_threads`,
   `comment_replies`. Trigger removes the FTS rows.
2. **TTL-based eviction.** Add `WHERE fetched_at < now - 30d` cleanup on a
   `canva sync --prune` flag. Less correct (kills active-but-unmodified
   designs) but cheap.

Document both in `--help`. Default to (1) on a 24h interval.

## Edge cases

- **Partial failure mid-page.** All upserts for one page happen in a single
  transaction with the cursor write. If the page fails halfway, the tx rolls
  back and the next run retries from the same cursor. No duplicates, no gaps.
- **Concurrent canvacli runs.** SQLite WAL mode (already enabled in
  `Open()`) allows concurrent readers + one writer. Two `canva sync` runs
  would serialise on the writer lock; the second one would see the first's
  committed cursor and effectively no-op the overlapping pages. Acceptable
  for v2; if it becomes painful we add an advisory lock row in `meta`.
- **Cursor poisoning.** If Canva ever rotates its cursor format and returns
  `400 invalid_continuation` for a stored cursor, catch that error code,
  null the cursor, and fall back to full crawl. Log a warning.
- **FTS index drift.** If a future bug causes `designs` and `designs_fts`
  to diverge, expose `canva cache rebuild-index` that runs
  `INSERT INTO designs_fts(designs_fts) VALUES('rebuild')` for each FTS table.
- **Large `raw_json` blobs.** External-content FTS5 only indexes the column
  we name (`title`, `name`, `text`), not `raw_json`. So the index stays
  small even though we keep full JSON blobs.
- **Read-only DB handle.** The existing `dbRO` handle (with `query_only=true`)
  works for FTS5 `MATCH` queries unchanged — `MATCH` is a `SELECT`.

## Open questions

1. **Cursor stability.** Canva's spec describes `continuation` as opaque but
   doesn't promise it survives across, say, weeks. Defensible default: assume
   it can expire, treat `400 invalid_continuation` as "redo full crawl". File
   an issue against `docs/research/canva-api.md` to clarify this if we ever
   talk to a Canva engineer.
2. **`updated_at` semantics for comments.** Spec says comments have a
   `created_at` but is silent on whether edited comments get an `updated_at`.
   If they don't, edits to existing comments will be invisible to incremental
   sync. Defensible default: include comments in the periodic full reconcile.
3. **Brand kit endpoint shape.** TBD whether brand colours/fonts are returned
   as one document or separate paginated lists. Default plan: one shot, no
   cursor; revisit if endpoint turns out to be paginated.
4. **Tokenizer choice.** I picked `porter unicode61` for English stemming +
   unicode-aware splitting. Canva is multilingual; if non-English titles are
   common we may want plain `unicode61` (no stemming) to avoid mangling
   non-English roots. Defensible default: `porter unicode61`, document the
   tradeoff, make it overridable via a `meta` row in a later version.
5. **Search ranking across resource types.** A `UNION ALL` across
   designs/templates/comments/assets gives per-table bm25 scores that aren't
   directly comparable. Defensible default: group results by resource type
   in the CLI output rather than mix them; revisit if users ask for one
   merged ranked list.

Status: DONE
