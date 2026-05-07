// Comments cache: CRUD + sync helper.
//
// Backed by `comment_threads` and `comment_replies` (defined in db.go,
// landed by Phase 3a foundation). Triggers keep `comment_threads_fts` and
// `comment_replies_fts` in sync automatically.
//
// The local thread cache is the structural workaround for the missing
// list-threads endpoint (see spec §9). Threads enter the cache via:
//   1. `canva comments add` succeeds -> upsert the new thread.
//   2. `canva comments thread <id>` (with --design or cache hit) -> upsert.
//   3. `canva sync` re-walks already-known thread IDs.
package cache

import (
	"context"
	"database/sql"
	"errors"
)

// CommentThread is the cache row for a Connect comment thread.
//
// The schema in db.go uses `author` (not `author_user_id`) and does not
// carry a `type` column — that detail lives in the embedded raw_json blob.
type CommentThread struct {
	ID        string
	DesignID  string
	Author    string // user_id of the author (may be empty if account deleted)
	RootText  string
	CreatedAt int64
	UpdatedAt int64
	FetchedAt int64
	RawJSON   string
}

// CommentReply is the cache row for a Connect reply.
//
// The schema does not store DesignID or UpdatedAt for replies — they live
// in raw_json. Callers who need them can JSON-decode raw_json.
type CommentReply struct {
	ID        string
	ThreadID  string
	Author    string
	Text      string
	CreatedAt int64
	FetchedAt int64
	RawJSON   string
}

// UpsertCommentThread writes a thread row, replacing any existing row with
// the same id. The FTS5 trigger handles index sync.
func (c *Cache) UpsertCommentThread(t CommentThread) error {
	_, err := c.db.Exec(`
		INSERT INTO comment_threads (id, design_id, author, root_text, created_at, updated_at, fetched_at, raw_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  design_id=excluded.design_id,
		  author=excluded.author,
		  root_text=excluded.root_text,
		  created_at=excluded.created_at,
		  updated_at=excluded.updated_at,
		  fetched_at=excluded.fetched_at,
		  raw_json=excluded.raw_json
	`, t.ID, t.DesignID, t.Author, t.RootText, t.CreatedAt, t.UpdatedAt, t.FetchedAt, t.RawJSON)
	return err
}

// UpsertCommentReply writes a reply row, replacing any existing row with
// the same id.
func (c *Cache) UpsertCommentReply(r CommentReply) error {
	_, err := c.db.Exec(`
		INSERT INTO comment_replies (id, thread_id, author, text, created_at, fetched_at, raw_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  thread_id=excluded.thread_id,
		  author=excluded.author,
		  text=excluded.text,
		  created_at=excluded.created_at,
		  fetched_at=excluded.fetched_at,
		  raw_json=excluded.raw_json
	`, r.ID, r.ThreadID, r.Author, r.Text, r.CreatedAt, r.FetchedAt, r.RawJSON)
	return err
}

// GetCommentThread loads a single thread by its (designID, threadID) pair.
// designID may be empty — when empty, the row is looked up by threadID
// alone, which is how `canva comments thread <id>` (without --design)
// discovers the design ID it needs for the API call.
func (c *Cache) GetCommentThread(designID, threadID string) (*CommentThread, error) {
	var (
		row *sql.Row
		t   CommentThread
	)
	if designID == "" {
		row = c.db.QueryRow(`
			SELECT id, design_id, COALESCE(author, ''), root_text,
			       created_at, COALESCE(updated_at, 0), fetched_at, raw_json
			FROM comment_threads WHERE id = ?
		`, threadID)
	} else {
		row = c.db.QueryRow(`
			SELECT id, design_id, COALESCE(author, ''), root_text,
			       created_at, COALESCE(updated_at, 0), fetched_at, raw_json
			FROM comment_threads WHERE id = ? AND design_id = ?
		`, threadID, designID)
	}
	err := row.Scan(&t.ID, &t.DesignID, &t.Author, &t.RootText, &t.CreatedAt, &t.UpdatedAt, &t.FetchedAt, &t.RawJSON)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// ListLocalCommentThreads returns every thread row in the cache, ordered by
// design_id then created_at. Used by `canva comments archive` (no --design).
func (c *Cache) ListLocalCommentThreads() ([]CommentThread, error) {
	return c.queryThreads(`
		SELECT id, design_id, COALESCE(author, ''), root_text,
		       created_at, COALESCE(updated_at, 0), fetched_at, raw_json
		FROM comment_threads ORDER BY design_id, created_at
	`)
}

// ListLocalCommentThreadsByDesign returns every thread row for one design.
// Used by `canva comments archive --design <d>`.
func (c *Cache) ListLocalCommentThreadsByDesign(designID string) ([]CommentThread, error) {
	return c.queryThreads(`
		SELECT id, design_id, COALESCE(author, ''), root_text,
		       created_at, COALESCE(updated_at, 0), fetched_at, raw_json
		FROM comment_threads WHERE design_id = ? ORDER BY created_at
	`, designID)
}

func (c *Cache) queryThreads(query string, args ...any) ([]CommentThread, error) {
	rows, err := c.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommentThread
	for rows.Next() {
		var t CommentThread
		if err := rows.Scan(&t.ID, &t.DesignID, &t.Author, &t.RootText, &t.CreatedAt, &t.UpdatedAt, &t.FetchedAt, &t.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// ListReplies returns every reply for a thread, ordered by created_at asc.
func (c *Cache) ListReplies(threadID string) ([]CommentReply, error) {
	rows, err := c.db.Query(`
		SELECT id, thread_id, COALESCE(author, ''), text, created_at, fetched_at, raw_json
		FROM comment_replies WHERE thread_id = ? ORDER BY created_at
	`, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CommentReply
	for rows.Next() {
		var r CommentReply
		if err := rows.Scan(&r.ID, &r.ThreadID, &r.Author, &r.Text, &r.CreatedAt, &r.FetchedAt, &r.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// FetchedThread is the shape SyncComments expects per re-fetched thread.
// The caller (commands or sync code) translates an api.CommentThread into
// this shape; defining it here avoids an import cycle (cache -> api).
type FetchedThread struct {
	Thread  CommentThread
	Replies []CommentReply
}

// SyncComments re-fetches every locally-known thread and its replies via
// `fetch`, then upserts the result into the cache. Designed for `canva
// sync` (per spec §9.3) and reusable from `canva comments archive`.
//
// `fetch` is called once per (designID, threadID) pair with the cached
// thread; it must return the freshly-walked thread + replies. Any error
// from `fetch` aborts the sync — partial progress is committed (per-thread
// upserts have happened up to that point). Callers that want all-or-nothing
// semantics should wrap this in a transaction at a higher layer.
func (c *Cache) SyncComments(ctx context.Context, fetch func(ctx context.Context, designID, threadID string, cached CommentThread) (FetchedThread, error)) error {
	if fetch == nil {
		return errors.New("comments sync: fetch is nil")
	}
	threads, err := c.ListLocalCommentThreads()
	if err != nil {
		return err
	}
	for _, cached := range threads {
		if err := ctx.Err(); err != nil {
			return err
		}
		fresh, err := fetch(ctx, cached.DesignID, cached.ID, cached)
		if err != nil {
			return err
		}
		if err := c.UpsertCommentThread(fresh.Thread); err != nil {
			return err
		}
		for _, r := range fresh.Replies {
			if err := c.UpsertCommentReply(r); err != nil {
				return err
			}
		}
	}
	return nil
}
