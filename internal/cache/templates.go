package cache

type Template struct {
	ID        string
	Title     string
	FetchedAt int64
	RawJSON   string
}

func (c *Cache) UpsertTemplate(t Template) error {
	_, err := c.db.Exec(`
		INSERT INTO templates (id, title, fetched_at, raw_json)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  title=excluded.title,
		  fetched_at=excluded.fetched_at,
		  raw_json=excluded.raw_json
	`, t.ID, t.Title, t.FetchedAt, t.RawJSON)
	return err
}

func (c *Cache) FindTemplateByID(id string) (*Template, error) {
	row := c.db.QueryRow(`SELECT id, title, fetched_at, raw_json FROM templates WHERE id = ?`, id)
	var t Template
	if err := row.Scan(&t.ID, &t.Title, &t.FetchedAt, &t.RawJSON); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Cache) FindTemplateByName(name string) ([]Template, error) {
	rows, err := c.db.Query(`SELECT id, title, fetched_at, raw_json FROM templates WHERE title = ? COLLATE NOCASE LIMIT 2`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Title, &t.FetchedAt, &t.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}
