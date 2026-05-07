package cache

import "time"

func (c *Cache) SaveIdempotency(key, command, argsHash, resultJSON string) error {
	_, err := c.db.Exec(`
		INSERT INTO idempotency (key, command, args_hash, result_json, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
		  command=excluded.command,
		  args_hash=excluded.args_hash,
		  result_json=excluded.result_json,
		  created_at=excluded.created_at
	`, key, command, argsHash, resultJSON, time.Now().Unix())
	return err
}

func (c *Cache) LookupIdempotency(key, argsHash string) (string, error) {
	row := c.db.QueryRow(`SELECT result_json FROM idempotency WHERE key = ? AND args_hash = ?`, key, argsHash)
	var v string
	if err := row.Scan(&v); err != nil {
		return "", err
	}
	return v, nil
}

// PruneOldIdempotency removes entries older than the given duration.
func (c *Cache) PruneOldIdempotency(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge).Unix()
	_, err := c.db.Exec(`DELETE FROM idempotency WHERE created_at < ?`, cutoff)
	return err
}
