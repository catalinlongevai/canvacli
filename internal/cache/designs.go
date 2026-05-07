package cache

type Design struct {
	ID           string
	Title        string
	FolderID     string
	UpdatedAt    int64
	FetchedAt    int64
	ThumbnailURL string
	RawJSON      string
}

func (c *Cache) UpsertDesign(d Design) error {
	_, err := c.db.Exec(`
		INSERT INTO designs (id, title, folder_id, updated_at, fetched_at, thumbnail_url, raw_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  title=excluded.title,
		  folder_id=excluded.folder_id,
		  updated_at=excluded.updated_at,
		  fetched_at=excluded.fetched_at,
		  thumbnail_url=excluded.thumbnail_url,
		  raw_json=excluded.raw_json
	`, d.ID, d.Title, d.FolderID, d.UpdatedAt, d.FetchedAt, d.ThumbnailURL, d.RawJSON)
	return err
}

func (c *Cache) FindDesignByID(id string) (*Design, error) {
	row := c.db.QueryRow(`SELECT id, title, folder_id, updated_at, fetched_at, thumbnail_url, raw_json FROM designs WHERE id = ?`, id)
	var d Design
	err := row.Scan(&d.ID, &d.Title, &d.FolderID, &d.UpdatedAt, &d.FetchedAt, &d.ThumbnailURL, &d.RawJSON)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Cache) FindDesignByName(name string) ([]Design, error) {
	rows, err := c.db.Query(`SELECT id, title, folder_id, updated_at, fetched_at, thumbnail_url, raw_json FROM designs WHERE title = ? COLLATE NOCASE LIMIT 2`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Design
	for rows.Next() {
		var d Design
		if err := rows.Scan(&d.ID, &d.Title, &d.FolderID, &d.UpdatedAt, &d.FetchedAt, &d.ThumbnailURL, &d.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}
