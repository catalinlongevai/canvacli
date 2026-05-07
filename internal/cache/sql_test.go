package cache

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExecReadOnly_AllowsSelect(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	rows, err := c.ExecReadOnly("SELECT 1 AS x", 100)
	if err != nil {
		t.Fatalf("ExecReadOnly: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestExecReadOnly_RejectsInsert(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("INSERT INTO meta (key, value) VALUES ('x','y')", 100)
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only error, got %v", err)
	}
}

func TestExecReadOnly_RejectsAttach(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("ATTACH DATABASE 'foo.db' AS f", 100)
	if err == nil {
		t.Fatal("expected error on ATTACH")
	}
}

func TestExecReadOnly_RejectsMultipleStatements(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("SELECT 1; SELECT 2", 100)
	if err == nil {
		t.Fatal("expected error on multiple statements")
	}
}

func TestExecReadOnly_AppliesLimit(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	for i := 0; i < 10; i++ {
		c.UpsertDesign(Design{ID: string(rune('a' + i)), Title: "x", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	}
	rows, err := c.ExecReadOnly("SELECT id FROM designs", 3)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected limit 3, got %d", len(rows))
	}
}

func TestExecReadOnly_RejectsPragma(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("PRAGMA table_info(designs)", 10)
	if err == nil {
		t.Fatal("expected error on PRAGMA")
	}
}

func TestExecReadOnly_RejectsDelete(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("DELETE FROM designs", 10)
	if err == nil {
		t.Fatal("expected error on DELETE")
	}
}

func TestExecReadOnly_AllowsWithSelect(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("WITH x AS (SELECT 1 AS n) SELECT n FROM x", 10)
	if err != nil {
		t.Fatalf("WITH ... SELECT should be allowed, got %v", err)
	}
}
