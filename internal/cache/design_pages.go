package cache

// DesignPage is the local-cache row backing the design_pages table. The
// schema is created in db.go's migration; this file just provides typed
// CRUD on top.
//
// Width and Height come from Canva's PageDimensions (floats on the wire,
// always integral in practice). ThumbnailURL is a signed S3 URL that
// expires after a few hours, so callers should treat it as a hint —
// re-fetch via api.ListPages when stale.
type DesignPage struct {
	DesignID     string
	Index        int
	Width        int
	Height       int
	ThumbnailURL string
	RawJSON      string
	FetchedAt    int64
}

// UpsertDesignPage inserts or replaces a single page row. Conflicts on
// (design_id, page_index) update every column except the primary key.
func (c *Cache) UpsertDesignPage(p DesignPage) error {
	_, err := c.db.Exec(`
		INSERT INTO design_pages (design_id, page_index, thumbnail_url, raw_json, fetched_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(design_id, page_index) DO UPDATE SET
		  thumbnail_url=excluded.thumbnail_url,
		  raw_json=excluded.raw_json,
		  fetched_at=excluded.fetched_at
	`, p.DesignID, p.Index, p.ThumbnailURL, p.RawJSON, p.FetchedAt)
	return err
}

// ListDesignPages returns the cached pages for a design, ordered by
// page_index. Returns an empty slice (not nil) if the design has no
// cached pages.
func (c *Cache) ListDesignPages(designID string) ([]DesignPage, error) {
	rows, err := c.db.Query(`
		SELECT design_id, page_index, thumbnail_url, raw_json, fetched_at
		FROM design_pages
		WHERE design_id = ?
		ORDER BY page_index ASC
	`, designID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DesignPage{}
	for rows.Next() {
		var p DesignPage
		if err := rows.Scan(&p.DesignID, &p.Index, &p.ThumbnailURL, &p.RawJSON, &p.FetchedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteDesignPages removes every cached page for a design. Used by sync
// when the design is rebuilt from scratch.
func (c *Cache) DeleteDesignPages(designID string) error {
	_, err := c.db.Exec(`DELETE FROM design_pages WHERE design_id = ?`, designID)
	return err
}
