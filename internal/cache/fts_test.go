package cache

import (
	"context"
	"testing"
)

// seedSearchData inserts one row in each FTS-indexed table for cross-table
// search assertions.
func seedSearchData(t *testing.T, c *Cache) {
	t.Helper()
	if err := c.UpsertDesign(Design{
		ID: "DAFdesign1", Title: "Q3 launch banner",
		UpdatedAt: 100, FetchedAt: 1, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("UpsertDesign: %v", err)
	}
	if err := c.UpsertTemplate(Template{
		ID: "BTpl1", Title: "Q3 banner template",
		FetchedAt: 1, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("UpsertTemplate: %v", err)
	}
	if err := c.UpsertCommentThread(CommentThread{
		ID: "CMt1", DesignID: "DAFdesign1",
		RootText: "looks great, ship banner now",
		CreatedAt: 1, FetchedAt: 1, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("UpsertCommentThread: %v", err)
	}
	if err := c.UpsertCommentReply(CommentReply{
		ID: "CMr1", ThreadID: "CMt1",
		Text: "approved by marketing on the banner copy",
		CreatedAt: 2, FetchedAt: 2, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("UpsertCommentReply: %v", err)
	}
	if err := c.UpsertAsset(Asset{
		ID: "Ma1", Name: "hero-banner.png", Type: "image",
		FetchedAt: 1, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("UpsertAsset: %v", err)
	}
}

func TestSearch_FindsAcrossAllSources(t *testing.T) {
	c := openTestCache(t)
	seedSearchData(t, c)

	hits, err := c.Search(context.Background(), "banner", "", 50)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// All five sources mention "banner".
	gotTypes := map[string]bool{}
	for _, h := range hits {
		gotTypes[h.Type] = true
	}
	for _, want := range []string{"design", "template", "comment_thread", "comment_reply", "asset"} {
		if !gotTypes[want] {
			t.Errorf("missing %q in results: %#v", want, gotTypes)
		}
	}
}

func TestSearch_TypeFilter(t *testing.T) {
	c := openTestCache(t)
	seedSearchData(t, c)

	hits, err := c.Search(context.Background(), "banner", "design", 50)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d: %#v", len(hits), hits)
	}
	if hits[0].Type != "design" || hits[0].ID != "DAFdesign1" {
		t.Fatalf("unexpected hit: %#v", hits[0])
	}
	if hits[0].Title == "" {
		t.Fatalf("expected Title populated for design hit, got %#v", hits[0])
	}
}

func TestSearch_CommentSourcesPopulateText(t *testing.T) {
	c := openTestCache(t)
	seedSearchData(t, c)

	hits, err := c.Search(context.Background(), "banner", "comment_reply", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want 1 hit, got %d", len(hits))
	}
	if hits[0].Text == "" {
		t.Fatalf("expected Text populated for comment_reply, got %#v", hits[0])
	}
	if hits[0].Title != "" {
		t.Fatalf("Title should be empty for comment_reply, got %q", hits[0].Title)
	}
	if hits[0].DesignID != "DAFdesign1" {
		t.Fatalf("expected DesignID=DAFdesign1, got %q", hits[0].DesignID)
	}
}

func TestSearch_LimitClamps(t *testing.T) {
	c := openTestCache(t)
	seedSearchData(t, c)

	// Limit 0 → default 50; we have 5 hits total.
	hits, err := c.Search(context.Background(), "banner", "", 0)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) == 0 {
		t.Fatalf("expected hits with default limit, got 0")
	}
	// Limit 1 → exactly one hit returned.
	hits, err = c.Search(context.Background(), "banner", "", 1)
	if err != nil {
		t.Fatalf("Search limit 1: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("want 1 hit with limit=1, got %d", len(hits))
	}
}

func TestSearch_PrefixQuery(t *testing.T) {
	c := openTestCache(t)
	seedSearchData(t, c)
	// FTS5 prefix syntax: `launch*` should match "launch".
	hits, err := c.Search(context.Background(), "launch*", "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	found := false
	for _, h := range hits {
		if h.ID == "DAFdesign1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("prefix search for launch* should hit the design, got %#v", hits)
	}
}

func TestSearchCacheEmpty(t *testing.T) {
	c := openTestCache(t)
	empty, err := c.SearchCacheEmpty()
	if err != nil {
		t.Fatalf("SearchCacheEmpty: %v", err)
	}
	if !empty {
		t.Fatalf("fresh cache should report empty")
	}
	seedSearchData(t, c)
	empty, err = c.SearchCacheEmpty()
	if err != nil {
		t.Fatalf("SearchCacheEmpty after seed: %v", err)
	}
	if empty {
		t.Fatalf("seeded cache should not report empty")
	}
}

func TestIsValidSearchType(t *testing.T) {
	for _, ok := range []string{"design", "template", "comment_thread", "comment_reply", "asset"} {
		if !IsValidSearchType(ok) {
			t.Errorf("%q should be valid", ok)
		}
	}
	for _, bad := range []string{"", "designs", "comments", "DESIGN", "unknown"} {
		if IsValidSearchType(bad) {
			t.Errorf("%q should be invalid", bad)
		}
	}
}
