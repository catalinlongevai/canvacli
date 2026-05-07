package resolver

import (
	"path/filepath"
	"testing"

	"github.com/catalinlongevai/canvacli/internal/cache"
)

func TestResolveDesign_ByID_HitsCache(t *testing.T) {
	c, _ := cache.Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	c.UpsertDesign(cache.Design{ID: "abc", Title: "X", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	r := New(c, nil)
	id, err := r.ResolveDesign("abc")
	if err != nil || id != "abc" {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}

func TestResolveDesign_ByName_OneMatch(t *testing.T) {
	c, _ := cache.Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	c.UpsertDesign(cache.Design{ID: "d1", Title: "Q3 Banner", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	r := New(c, nil)
	id, err := r.ResolveDesign("Q3 Banner")
	if err != nil || id != "d1" {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}

func TestResolveDesign_AmbiguousReturnsError(t *testing.T) {
	c, _ := cache.Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	c.UpsertDesign(cache.Design{ID: "d1", Title: "Banner", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	c.UpsertDesign(cache.Design{ID: "d2", Title: "Banner", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	r := New(c, nil)
	_, err := r.ResolveDesign("Banner")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
}
