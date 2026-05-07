package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

// NewAssets is the parent command for asset-library operations.
//
// `canva assets list` is intentionally absent — the Connect API does not
// expose a flat list-assets endpoint (verified 404, see research
// v2-assets-imports-api.md §"Asset list"). Folder-scoped browsing remains
// available via `canva folders` for v2.1.
func NewAssets() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assets",
		Short: "Asset library operations (upload)",
		Long: `Asset library operations.

Note: there is no flat 'list assets' subcommand because the Canva Connect
API does not expose one. Folder-scoped image listing remains available
via 'canva folders'.`,
	}
	cmd.AddCommand(newAssetsUpload())
	return cmd
}

func newAssetsUpload() *cobra.Command {
	var flagName string
	var flagIDOnly bool
	cmd := &cobra.Command{
		Use:   "upload <file>",
		Short: "Upload a local file (image/video/audio) to the Canva asset library",
		Long: `Upload a local file as a Canva asset and print the asset metadata.

Uploads use POST /v1/asset-uploads with application/octet-stream raw bytes
and an Asset-Upload-Metadata header — NOT multipart/form-data.

The bare M-prefixed asset id is also printable via --id-only so the
output composes cleanly into 'canva create --autofill' for the asset →
autofill bridge (spec §10).`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := args[0]
			file, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("open %s: %w", path, err)
			}
			defer file.Close()

			name := flagName
			if name == "" {
				name = filepath.Base(path)
			}

			ctx := cmd.Context()
			cl, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()

			asset, err := cl.UploadAsset(ctx, name, file)
			if err != nil {
				return err
			}

			if err := upsertAssetIntoCache(ch, asset); err != nil {
				// Cache failure is non-fatal: the upload itself succeeded.
				fmt.Fprintf(os.Stderr, "warn: cache upsert failed: %v\n", err)
			}

			if flagIDOnly {
				fmt.Println(asset.ID)
				return nil
			}

			out := map[string]any{
				"asset_id":   asset.ID,
				"id":         asset.ID, // alias for callers using the standard `id` key
				"name":       asset.Name,
				"type":       asset.Type,
				"updated_at": asset.UpdatedAt,
			}
			if asset.Thumbnail != nil && asset.Thumbnail.URL != "" {
				out["thumbnail_url"] = asset.Thumbnail.URL
			}
			if len(asset.Tags) > 0 {
				out["tags"] = asset.Tags
			}
			return output.EmitJSON(os.Stdout, out)
		},
	}
	cmd.Flags().StringVar(&flagName, "name", "", "asset display name (default: file basename)")
	cmd.Flags().BoolVar(&flagIDOnly, "id-only", false, "print only the bare asset id to stdout")
	return cmd
}

// upsertAssetIntoCache marshals an api.Asset into the cache row shape and
// upserts it. Reused by the MCP server.
func upsertAssetIntoCache(ch *cache.Cache, asset *api.Asset) error {
	if asset == nil {
		return errors.New("nil asset")
	}
	raw, _ := json.Marshal(asset)
	thumbURL := ""
	if asset.Thumbnail != nil {
		thumbURL = asset.Thumbnail.URL
	}
	return ch.UpsertAsset(cache.Asset{
		ID:        asset.ID,
		Name:      asset.Name,
		Type:      asset.Type,
		URL:       thumbURL,
		UpdatedAt: asset.UpdatedAt,
		FetchedAt: time.Now().Unix(),
		RawJSON:   string(raw),
	})
}
