// FTS5 search query builder.
//
// `Search` runs a UNION ALL across the 5 FTS5 virtual tables created in
// db.go's foundation schema:
//
//   designs_fts.title          (designs.title)
//   templates_fts.title        (templates.title)
//   comment_threads_fts.root_text (comment_threads.root_text)
//   comment_replies_fts.text   (comment_replies.text)
//   assets_fts.name            (assets.name)
//
// External-content tables: each FTS row's `rowid` matches the base table's
// `rowid`, so we join back to fetch the id+title/text for the result.
//
// Per-table bm25 scores are not strictly comparable across tables — this is
// documented as a known limitation in v2-fts5-sync-design.md §"Open
// questions" #5. Spec §7.1 says ordering is intuitive in practice and a
// `--group-by-type` flag will land in v2.1 if it ever proves problematic.
package cache

import (
	"context"
	"fmt"
	"strings"
)

// SearchHit is a single ranked match. Title is populated for designs,
// templates, and assets; Text is populated for comment threads and replies.
// DesignID is populated when the result is a comment thread or reply
// (carries the parent design ID for follow-up navigation).
type SearchHit struct {
	Type     string  `json:"type"` // "design" | "template" | "comment_thread" | "comment_reply" | "asset"
	ID       string  `json:"id"`
	Title    string  `json:"title,omitempty"`
	Text     string  `json:"text,omitempty"`
	DesignID string  `json:"design_id,omitempty"`
	Rank     float64 `json:"rank"`
}

// searchSource describes one FTS source: the kind label, the FTS table,
// and the SELECT body that joins back to the content table to retrieve the
// id, the snippet column (title or text), and the design_id when relevant.
type searchSource struct {
	typ string
	// sqlFmt is a SELECT body with one ? placeholder for the MATCH
	// argument. Five columns: type literal, id, snippet, design_id (or
	// NULL), rank.
	sqlFmt string
}

// searchSources is the canonical list, in deterministic order. Adding a
// new FTS source means appending here.
var searchSources = []searchSource{
	{
		typ: "design",
		sqlFmt: `SELECT 'design' AS type, d.id AS id, d.title AS snippet, '' AS design_id, bm25(designs_fts) AS rank
		         FROM designs d JOIN designs_fts f ON d.rowid = f.rowid
		         WHERE designs_fts MATCH ?`,
	},
	{
		typ: "template",
		sqlFmt: `SELECT 'template' AS type, t.id AS id, t.title AS snippet, '' AS design_id, bm25(templates_fts) AS rank
		         FROM templates t JOIN templates_fts f ON t.rowid = f.rowid
		         WHERE templates_fts MATCH ?`,
	},
	{
		typ: "comment_thread",
		sqlFmt: `SELECT 'comment_thread' AS type, ct.id AS id, ct.root_text AS snippet, ct.design_id AS design_id, bm25(comment_threads_fts) AS rank
		         FROM comment_threads ct JOIN comment_threads_fts f ON ct.rowid = f.rowid
		         WHERE comment_threads_fts MATCH ?`,
	},
	{
		typ: "comment_reply",
		sqlFmt: `SELECT 'comment_reply' AS type, cr.id AS id, cr.text AS snippet, COALESCE(ct.design_id, '') AS design_id, bm25(comment_replies_fts) AS rank
		         FROM comment_replies cr
		         JOIN comment_replies_fts f ON cr.rowid = f.rowid
		         LEFT JOIN comment_threads ct ON cr.thread_id = ct.id
		         WHERE comment_replies_fts MATCH ?`,
	},
	{
		typ: "asset",
		sqlFmt: `SELECT 'asset' AS type, a.id AS id, a.name AS snippet, '' AS design_id, bm25(assets_fts) AS rank
		         FROM assets a JOIN assets_fts f ON a.rowid = f.rowid
		         WHERE assets_fts MATCH ?`,
	},
}

// validSearchTypes mirrors searchSources for the type-filter validator. The
// command layer uses this to reject unknown --type values without consulting
// internal SQL.
var validSearchTypes = map[string]struct{}{
	"design":         {},
	"template":       {},
	"comment_thread": {},
	"comment_reply":  {},
	"asset":          {},
}

// IsValidSearchType reports whether typ is one of the accepted --type
// values for canva search.
func IsValidSearchType(typ string) bool {
	_, ok := validSearchTypes[typ]
	return ok
}

// Search runs an FTS5 MATCH query across the indexed tables and returns
// results ranked by per-source bm25 (ascending = best first).
//
// typeFilter restricts the scan to one of the values in validSearchTypes,
// or "" for all sources. limit is clamped to [1, 1000]; pass 0 for default 50.
//
// Reads go through the read-only dbRO handle (engine-level query_only(true)
// is the safety net; FTS5 MATCH is a SELECT either way).
func (c *Cache) Search(ctx context.Context, query, typeFilter string, limit int) ([]SearchHit, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	parts := []string{}
	args := []any{}
	for _, src := range searchSources {
		if typeFilter != "" && typeFilter != src.typ {
			continue
		}
		parts = append(parts, src.sqlFmt)
		args = append(args, query)
	}
	if len(parts) == 0 {
		// typeFilter didn't match any source — caller should validate
		// upstream, but if it slipped through we return zero hits.
		return nil, nil
	}

	full := strings.Join(parts, " UNION ALL ") + " ORDER BY rank ASC LIMIT ?"
	args = append(args, limit)

	rows, err := c.dbRO.QueryContext(ctx, full, args...)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	out := []SearchHit{}
	for rows.Next() {
		var h SearchHit
		var snippet, designID string
		if err := rows.Scan(&h.Type, &h.ID, &snippet, &designID, &h.Rank); err != nil {
			return nil, err
		}
		if h.Type == "comment_thread" || h.Type == "comment_reply" {
			h.Text = snippet
		} else {
			h.Title = snippet
		}
		if designID != "" {
			h.DesignID = designID
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// SearchCacheEmpty reports whether all five FTS-indexed tables are empty.
// Spec §7.4: when the cache is empty, `canva search` returns cache_empty.
func (c *Cache) SearchCacheEmpty() (bool, error) {
	var n int
	row := c.dbRO.QueryRow(`
		SELECT (SELECT count(*) FROM designs)
		     + (SELECT count(*) FROM templates)
		     + (SELECT count(*) FROM comment_threads)
		     + (SELECT count(*) FROM comment_replies)
		     + (SELECT count(*) FROM assets)
	`)
	if err := row.Scan(&n); err != nil {
		return false, err
	}
	return n == 0, nil
}
