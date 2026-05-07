package cache

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS designs (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  folder_id TEXT,
  updated_at INTEGER NOT NULL,
  fetched_at INTEGER NOT NULL,
  thumbnail_url TEXT,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS templates (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS folders (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  parent_id TEXT,
  fetched_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS idempotency (
  key TEXT PRIMARY KEY,
  command TEXT NOT NULL,
  args_hash TEXT NOT NULL,
  result_json TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_designs_title ON designs(title);
CREATE INDEX IF NOT EXISTS idx_templates_title ON templates(title);
CREATE INDEX IF NOT EXISTS idx_idempotency_created ON idempotency(created_at);

-- ============================================================
-- v2 additions
-- ============================================================

-- Per-page metadata
CREATE TABLE IF NOT EXISTS design_pages (
  design_id     TEXT NOT NULL,
  page_index    INTEGER NOT NULL,
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
  type        TEXT NOT NULL,
  url         TEXT,
  updated_at  INTEGER,
  fetched_at  INTEGER NOT NULL,
  raw_json    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_assets_type    ON assets(type);
CREATE INDEX IF NOT EXISTS idx_assets_updated ON assets(updated_at);

-- Sync state cursor
CREATE TABLE IF NOT EXISTS sync_state (
  resource_type   TEXT PRIMARY KEY,
  cursor          TEXT,
  last_synced_at  INTEGER NOT NULL,
  watermark_at    INTEGER,
  full_sync_at    INTEGER
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

INSERT OR REPLACE INTO meta(key, value) VALUES ('schema_version', '2');
`

type Cache struct {
	db   *sql.DB // writable
	dbRO *sql.DB // read-only with query_only(true) pragma
}

func Open(path string) (*Cache, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(2000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}

	// After schema migration, rebuild FTS5 indices for any pre-v2 data.
	// designs_fts and templates_fts may have rows in v1-shaped DBs that the
	// FTS triggers never observed; the others start empty on v2 first-open
	// but rebuilding them is cheap and idempotent.
	var rebuilt string
	_ = db.QueryRow("SELECT value FROM meta WHERE key = 'fts_rebuilt_v2'").Scan(&rebuilt)
	if rebuilt != "1" {
		for _, ftsTable := range []string{"designs_fts", "templates_fts", "comment_threads_fts", "comment_replies_fts", "assets_fts"} {
			// non-fatal: FTS5 might be empty/missing on a fresh DB
			_, _ = db.Exec(fmt.Sprintf("INSERT INTO %s(%s) VALUES('rebuild')", ftsTable, ftsTable))
		}
		_, _ = db.Exec("INSERT OR REPLACE INTO meta(key, value) VALUES ('fts_rebuilt_v2', '1')")
	}

	// Second handle: read-only, query_only=true, so even a regex-bypass
	// can't mutate the DB through this path.
	dbRO, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(2000)&_pragma=query_only(true)")
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Cache{db: db, dbRO: dbRO}, nil
}

func (c *Cache) DB() *sql.DB { return c.db }

func (c *Cache) Close() error {
	roErr := c.dbRO.Close()
	wErr := c.db.Close()
	if wErr != nil {
		return wErr
	}
	return roErr
}
