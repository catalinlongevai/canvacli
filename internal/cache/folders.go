package cache

type Folder struct {
	ID        string
	Name      string
	ParentID  string
	FetchedAt int64
}

func (c *Cache) UpsertFolder(f Folder) error {
	_, err := c.db.Exec(`
		INSERT INTO folders (id, name, parent_id, fetched_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name=excluded.name,
		  parent_id=excluded.parent_id,
		  fetched_at=excluded.fetched_at
	`, f.ID, f.Name, f.ParentID, f.FetchedAt)
	return err
}
