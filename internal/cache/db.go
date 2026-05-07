package cache

import (
	"database/sql"

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
