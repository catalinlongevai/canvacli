package cache

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
)

func TestUpsertAndFindAsset(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	now := time.Now().Unix()
	if err := c.UpsertAsset(Asset{
		ID: "Mab12cd34", Name: "hero.png", Type: "image",
		URL: "https://t.example/h.png", UpdatedAt: now, FetchedAt: now,
		RawJSON: `{"id":"Mab12cd34"}`,
	}); err != nil {
		t.Fatalf("UpsertAsset: %v", err)
	}
	got, err := c.FindAssetByID("Mab12cd34")
	if err != nil {
		t.Fatalf("FindAssetByID: %v", err)
	}
	if got.Name != "hero.png" || got.Type != "image" {
		t.Errorf("unexpected asset: %+v", got)
	}

	// Upsert again with updated name — verify it overwrites.
	if err := c.UpsertAsset(Asset{
		ID: "Mab12cd34", Name: "hero-renamed.png", Type: "image",
		UpdatedAt: now + 100, FetchedAt: now + 100, RawJSON: "{}",
	}); err != nil {
		t.Fatalf("UpsertAsset (update): %v", err)
	}
	got, _ = c.FindAssetByID("Mab12cd34")
	if got.Name != "hero-renamed.png" {
		t.Errorf("upsert did not update name: %+v", got)
	}
}

func TestListAssets_OrdersByUpdatedAtDesc(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_ = c.UpsertAsset(Asset{ID: "M01", Name: "old", Type: "image", UpdatedAt: 100, FetchedAt: 100, RawJSON: "{}"})
	_ = c.UpsertAsset(Asset{ID: "M02", Name: "new", Type: "image", UpdatedAt: 200, FetchedAt: 200, RawJSON: "{}"})
	got, err := c.ListAssets()
	if err != nil {
		t.Fatalf("ListAssets: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
	}
	if got[0].ID != "M02" || got[1].ID != "M01" {
		t.Errorf("ordering wrong: %+v", got)
	}
}

// stubAssetFetcher implements AssetFetcher for SyncAssets tests.
type stubAssetFetcher struct {
	calls []string
	resp  map[string]*api.Asset
}

func (s *stubAssetFetcher) GetAsset(_ context.Context, id string) (*api.Asset, error) {
	s.calls = append(s.calls, id)
	return s.resp[id], nil
}

func TestSyncAssets_RefreshesKnownAssets(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_ = c.UpsertAsset(Asset{ID: "M01", Name: "old-name", Type: "image", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	_ = c.UpsertAsset(Asset{ID: "M02", Name: "old-name-2", Type: "image", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})

	fetcher := &stubAssetFetcher{resp: map[string]*api.Asset{
		"M01": {ID: "M01", Name: "fresh-1", Type: "image", UpdatedAt: 999},
		"M02": {ID: "M02", Name: "fresh-2", Type: "image", UpdatedAt: 999},
	}}
	if err := c.SyncAssets(context.Background(), fetcher); err != nil {
		t.Fatalf("SyncAssets: %v", err)
	}
	if len(fetcher.calls) != 2 {
		t.Errorf("expected 2 GetAsset calls, got %d", len(fetcher.calls))
	}
	got, _ := c.FindAssetByID("M01")
	if got == nil || got.Name != "fresh-1" || got.UpdatedAt != 999 {
		t.Errorf("M01 not refreshed: %+v", got)
	}
}
