// Sync state cursor table CRUD.
//
// `sync_state` (created in db.go's foundation schema) tracks per-resource
// pagination cursors and watermarks so `canva sync` can resume incremental
// crawls. One row per resource_type ("designs", "folders", "templates",
// "assets", "comments_for_<designId>", etc.).
//
// Triggers do not depend on this table — it's pure orchestrator state.
package cache

import "time"

// SyncState mirrors one row of the `sync_state` table. Cursor is opaque
// (Canva's `continuation` token); WatermarkAt is for resources that don't
// support continuation tokens (max(updated_at) we've ingested).
type SyncState struct {
	ResourceType string
	Cursor       string
	WatermarkAt  int64
	LastSyncedAt int64
}

// GetSyncState returns the row for `resource`, or sql.ErrNoRows if none.
// Callers should treat ErrNoRows as "first run, no cursor stored."
func (c *Cache) GetSyncState(resource string) (*SyncState, error) {
	row := c.db.QueryRow(`
		SELECT resource_type,
		       COALESCE(cursor, ''),
		       COALESCE(watermark_at, 0),
		       last_synced_at
		FROM sync_state WHERE resource_type = ?
	`, resource)
	var s SyncState
	if err := row.Scan(&s.ResourceType, &s.Cursor, &s.WatermarkAt, &s.LastSyncedAt); err != nil {
		return nil, err
	}
	return &s, nil
}

// UpsertSyncState writes a row for s.ResourceType, replacing any prior
// cursor/watermark/last_synced_at. If LastSyncedAt is zero, it is filled
// with the current unix time.
func (c *Cache) UpsertSyncState(s SyncState) error {
	if s.LastSyncedAt == 0 {
		s.LastSyncedAt = time.Now().Unix()
	}
	_, err := c.db.Exec(`
		INSERT INTO sync_state (resource_type, cursor, watermark_at, last_synced_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(resource_type) DO UPDATE SET
		  cursor=excluded.cursor,
		  watermark_at=excluded.watermark_at,
		  last_synced_at=excluded.last_synced_at
	`, s.ResourceType, s.Cursor, s.WatermarkAt, s.LastSyncedAt)
	return err
}

// ResetSyncState clears all sync_state rows. Useful for forcing a full
// crawl (the `--full` flag on `canva sync`).
func (c *Cache) ResetSyncState() error {
	_, err := c.db.Exec("DELETE FROM sync_state")
	return err
}
