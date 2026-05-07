package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
)

// Asset is the cache representation of a Canva Asset. Mirrors the schema
// declared in db.go's `assets` table.
type Asset struct {
	ID        string
	Name      string
	Type      string
	URL       string
	UpdatedAt int64
	FetchedAt int64
	RawJSON   string
}

func (c *Cache) UpsertAsset(a Asset) error {
	_, err := c.db.Exec(`
		INSERT INTO assets (id, name, type, url, updated_at, fetched_at, raw_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name=excluded.name,
		  type=excluded.type,
		  url=excluded.url,
		  updated_at=excluded.updated_at,
		  fetched_at=excluded.fetched_at,
		  raw_json=excluded.raw_json
	`, a.ID, a.Name, a.Type, a.URL, a.UpdatedAt, a.FetchedAt, a.RawJSON)
	return err
}

func (c *Cache) FindAssetByID(id string) (*Asset, error) {
	row := c.db.QueryRow(`SELECT id, name, type, url, updated_at, fetched_at, raw_json FROM assets WHERE id = ?`, id)
	var a Asset
	if err := row.Scan(&a.ID, &a.Name, &a.Type, &a.URL, &a.UpdatedAt, &a.FetchedAt, &a.RawJSON); err != nil {
		return nil, err
	}
	return &a, nil
}

// ListAssets returns all assets in the cache, ordered by updated_at desc.
func (c *Cache) ListAssets() ([]Asset, error) {
	rows, err := c.db.Query(`SELECT id, name, type, url, updated_at, fetched_at, raw_json FROM assets ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Asset
	for rows.Next() {
		var a Asset
		if err := rows.Scan(&a.ID, &a.Name, &a.Type, &a.URL, &a.UpdatedAt, &a.FetchedAt, &a.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// AssetFetcher is the slice of *api.Client used by SyncAssets. Pulled into
// an interface so tests can stub without dragging the full HTTP client.
type AssetFetcher interface {
	GetAsset(ctx context.Context, id string) (*api.Asset, error)
}

// SyncAssets re-fetches every locally-known asset from the API and re-upserts
// it into the cache. The Connect API exposes no list-assets endpoint
// (research §"Asset list"), so the cache can only refresh assets it already
// knows about (uploaded via canvacli or seeded by `canva assets get`).
func (c *Cache) SyncAssets(ctx context.Context, client AssetFetcher) error {
	known, err := c.ListAssets()
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	for _, prev := range known {
		fresh, err := client.GetAsset(ctx, prev.ID)
		if err != nil {
			return err
		}
		raw, _ := json.Marshal(fresh)
		thumbURL := ""
		if fresh.Thumbnail != nil {
			thumbURL = fresh.Thumbnail.URL
		}
		if err := c.UpsertAsset(Asset{
			ID:        fresh.ID,
			Name:      fresh.Name,
			Type:      fresh.Type,
			URL:       thumbURL,
			UpdatedAt: fresh.UpdatedAt,
			FetchedAt: now,
			RawJSON:   string(raw),
		}); err != nil {
			return err
		}
	}
	return nil
}
