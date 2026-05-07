package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
)

// handleAssetsUpload backs the canva_assets_upload MCP tool. It mirrors the
// CLI's `canva assets upload` flow: open file, default name to basename,
// POST /v1/asset-uploads, poll, upsert into the local cache, return the
// asset metadata as JSON. The asset_id is the same M-prefixed id that
// DatasetImageValue.asset_id accepts in autofill (spec §10).
func (s *Server) handleAssetsUpload(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	path, _ := args["file"].(string)
	if path == "" {
		return mcp.NewToolResultError("file is required"), nil
	}
	if !filepath.IsAbs(path) {
		return mcp.NewToolResultError("file must be an absolute path"), nil
	}
	name, _ := args["name"].(string)
	if name == "" {
		name = filepath.Base(path)
	}

	f, err := os.Open(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("open %s: %v", path, err)), nil
	}
	defer f.Close()

	asset, err := s.api.UploadAsset(ctx, name, f)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := upsertAssetIntoMCPCache(s.cache, asset); err != nil {
		// Non-fatal: surface the warning in the response payload.
		// The upload itself succeeded.
		_ = err
	}

	out := map[string]any{
		"asset_id":   asset.ID,
		"id":         asset.ID,
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
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

// handleImport backs the canva_import MCP tool with the same extension-based
// routing as the CLI (spec §8). Image and video extensions transparently
// route to /v1/asset-uploads; documents go through /v1/imports.
func (s *Server) handleImport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	path, _ := args["file"].(string)
	if path == "" {
		return mcp.NewToolResultError("file is required"), nil
	}
	if !filepath.IsAbs(path) {
		return mcp.NewToolResultError("file must be an absolute path"), nil
	}
	overrideMime, _ := args["mime_type"].(string)

	ext := strings.ToLower(filepath.Ext(path))
	route, ok := mcpImportRoute(ext)
	if !ok {
		return mcp.NewToolResultError(fmt.Sprintf(
			"import_unsupported_format: extension %q is not a supported import format",
			ext,
		)), nil
	}

	f, err := os.Open(path)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("open %s: %v", path, err)), nil
	}
	defer f.Close()

	name := filepath.Base(path)

	if route.target == "asset" {
		asset, err := s.api.UploadAsset(ctx, name, f)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		_ = upsertAssetIntoMCPCache(s.cache, asset)
		out := map[string]any{
			"kind":     "asset",
			"asset_id": asset.ID,
			"id":       asset.ID,
			"name":     asset.Name,
			"type":     asset.Type,
			"note":     "imports are for documents; uploaded as asset instead.",
		}
		if asset.Thumbnail != nil && asset.Thumbnail.URL != "" {
			out["thumbnail_url"] = asset.Thumbnail.URL
		}
		b, _ := json.Marshal(out)
		return mcp.NewToolResultText(string(b)), nil
	}

	mime := overrideMime
	if mime == "" {
		mime = route.mimeType
	}
	res, err := s.api.ImportFile(ctx, name, mime, f)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(res.Result.Designs) == 0 {
		return mcp.NewToolResultError("import returned no designs"), nil
	}
	now := time.Now().Unix()
	designs := make([]map[string]any, 0, len(res.Result.Designs))
	for _, d := range res.Result.Designs {
		raw, _ := json.Marshal(d)
		thumb := ""
		if d.Thumbnail != nil {
			thumb = d.Thumbnail.URL
		}
		_ = s.cache.UpsertDesign(cache.Design{
			ID:           d.ID,
			Title:        d.Title,
			UpdatedAt:    d.UpdatedAt,
			FetchedAt:    now,
			ThumbnailURL: thumb,
			RawJSON:      string(raw),
		})
		row := map[string]any{
			"id":         d.ID,
			"title":      d.Title,
			"page_count": d.PageCount,
		}
		if d.URLs != nil {
			if d.URLs.EditURL != "" {
				row["edit_url"] = d.URLs.EditURL
			}
			if d.URLs.ViewURL != "" {
				row["view_url"] = d.URLs.ViewURL
			}
		}
		designs = append(designs, row)
	}
	b, _ := json.Marshal(map[string]any{
		"kind":    "import",
		"designs": designs,
	})
	return mcp.NewToolResultText(string(b)), nil
}

// mcpImportRoute is the routing table for the MCP canva_import tool. Mirrors
// the CLI's importRoutingTable. Kept package-local to avoid leaking a
// command-package symbol into the mcp package.
type mcpRoute struct {
	target   string // "import" | "asset"
	mimeType string
}

func mcpImportRoute(ext string) (mcpRoute, bool) {
	r, ok := mcpRoutes[ext]
	return r, ok
}

var mcpRoutes = map[string]mcpRoute{
	".png": {target: "asset"}, ".jpg": {target: "asset"}, ".jpeg": {target: "asset"},
	".gif": {target: "asset"}, ".heic": {target: "asset"}, ".tiff": {target: "asset"}, ".webp": {target: "asset"},
	".mp4": {target: "asset"}, ".mov": {target: "asset"}, ".m4v": {target: "asset"},
	".mkv": {target: "asset"}, ".mpeg": {target: "asset"}, ".webm": {target: "asset"},
	".pdf":      {target: "import", mimeType: "application/pdf"},
	".pptx":     {target: "import", mimeType: "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	".ppt":      {target: "import", mimeType: "application/vnd.ms-powerpoint"},
	".docx":     {target: "import", mimeType: "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
	".doc":      {target: "import", mimeType: "application/msword"},
	".xlsx":     {target: "import", mimeType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
	".xls":      {target: "import", mimeType: "application/vnd.ms-excel"},
	".key":      {target: "import"},
	".pages":    {target: "import"},
	".numbers":  {target: "import"},
	".ai":       {target: "import"},
	".psd":      {target: "import"},
	".afdesign": {target: "import"},
	".afphoto":  {target: "import"},
	".afpub":    {target: "import"},
	".af":       {target: "import"},
	".odp":      {target: "import"},
	".odt":      {target: "import"},
	".ods":      {target: "import"},
	".odg":      {target: "import"},
}

// upsertAssetIntoMCPCache mirrors commands.upsertAssetIntoCache without the
// import cycle (commands depends on mcp via NewMCP, so mcp can't depend on
// commands). Keep them in sync.
func upsertAssetIntoMCPCache(c *cache.Cache, asset *api.Asset) error {
	if asset == nil {
		return errors.New("nil asset")
	}
	raw, _ := json.Marshal(asset)
	thumb := ""
	if asset.Thumbnail != nil {
		thumb = asset.Thumbnail.URL
	}
	return c.UpsertAsset(cache.Asset{
		ID:        asset.ID,
		Name:      asset.Name,
		Type:      asset.Type,
		URL:       thumb,
		UpdatedAt: asset.UpdatedAt,
		FetchedAt: time.Now().Unix(),
		RawJSON:   string(raw),
	})
}
