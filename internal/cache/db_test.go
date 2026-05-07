package cache

import (
	"path/filepath"
	"testing"
)

func TestOpen_CreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	wantTables := []string{"designs", "templates", "folders", "idempotency", "meta"}
	for _, tbl := range wantTables {
		var n int
		row := db.DB().QueryRow("SELECT count(*) FROM " + tbl)
		if err := row.Scan(&n); err != nil {
			t.Fatalf("table %q missing: %v", tbl, err)
		}
	}
}
