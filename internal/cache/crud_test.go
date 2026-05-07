package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUpsertAndFindDesign(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	now := time.Now().Unix()
	if err := c.UpsertDesign(Design{ID: "d1", Title: "Q3 Banner", UpdatedAt: now, FetchedAt: now, RawJSON: "{}"}); err != nil {
		t.Fatalf("UpsertDesign: %v", err)
	}
	got, err := c.FindDesignByName("q3 banner")
	if err != nil {
		t.Fatalf("FindDesignByName: %v", err)
	}
	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestIdempotencyLookup(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SaveIdempotency("k1", "create", "h", `{"id":"abc"}`); err != nil {
		t.Fatalf("SaveIdempotency: %v", err)
	}
	got, err := c.LookupIdempotency("k1", "h")
	if err != nil {
		t.Fatalf("LookupIdempotency: %v", err)
	}
	if got != `{"id":"abc"}` {
		t.Fatalf("got %q", got)
	}
}
