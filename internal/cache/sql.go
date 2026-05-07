package cache

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	allowedPrefix = regexp.MustCompile(`(?i)^\s*(WITH\s|SELECT\s)`)
	forbidden     = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|REPLACE|CREATE|DROP|ALTER|ATTACH|DETACH|PRAGMA|VACUUM|REINDEX)\b`)
)

// ExecReadOnly runs a single SELECT (or WITH ... SELECT) and returns up to
// limit rows. Rejects multi-statement input, mutating statements, and
// ATTACH/PRAGMA. Enforces a 5-second timeout.
func (c *Cache) ExecReadOnly(query string, limit int) ([]map[string]any, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimSuffix(q, ";")
	if strings.Contains(q, ";") {
		return nil, errors.New("multiple statements not allowed")
	}
	if !allowedPrefix.MatchString(q) {
		return nil, errors.New("read-only: only SELECT and WITH...SELECT are permitted")
	}
	if forbidden.MatchString(q) {
		return nil, errors.New("read-only: mutating or restricted keyword detected")
	}
	if limit <= 0 {
		limit = 500
	}
	if limit > 10000 {
		limit = 10000
	}
	wrapped := fmt.Sprintf("SELECT * FROM (%s) LIMIT %d", q, limit)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := c.dbRO.QueryContext(ctx, wrapped)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, name := range cols {
			row[name] = vals[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
