package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

// AssetThumbnail is a presigned S3 URL for the asset's thumbnail. URLs
// expire ~18h after issue — re-GET the asset for a fresh URL rather than
// caching the URL itself (research v2-assets-imports-api.md §"Asset get").
type AssetThumbnail struct {
	URL    string `json:"url,omitempty"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Asset is the Connect API's Asset object as returned by /asset-uploads
// success and /assets/{id}.
type Asset struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Type      string          `json:"type"` // "image" | "video" | ...
	Tags      []string        `json:"tags,omitempty"`
	Thumbnail *AssetThumbnail `json:"thumbnail,omitempty"`
	CreatedAt int64           `json:"created_at,omitempty"`
	UpdatedAt int64           `json:"updated_at,omitempty"`
}

// assetJobResult mirrors the inlined `job` payload on /asset-uploads/{id}
// success: { id, status, asset: {...} }. PollJob[T] decodes T from the
// entire `job` body, so this struct only needs the fields we care about.
type assetJobResult struct {
	Asset Asset `json:"asset"`
}

// UploadAsset POSTs raw bytes to /asset-uploads with the metadata header
// and polls the resulting job to completion. The asset name is base64-
// encoded inside the Asset-Upload-Metadata JSON header per the Connect
// API contract (see research v2-assets-imports-api.md §"Asset upload").
//
// Critical: this endpoint takes application/octet-stream raw bytes, NOT
// multipart/form-data.
func (c *Client) UploadAsset(ctx context.Context, name string, data io.Reader) (*Asset, error) {
	body, err := io.ReadAll(data)
	if err != nil {
		return nil, err
	}

	metadata := map[string]string{
		"name_base64": base64.StdEncoding.EncodeToString([]byte(name)),
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/asset-uploads", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Asset-Upload-Metadata", string(metadataJSON))

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, &APIError{Code: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, mapHTTPError(resp)
	}

	var submit struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&submit); err != nil {
		return nil, err
	}
	if submit.Job.ID == "" {
		return nil, &APIError{Code: "asset_upload_failed", Message: "asset-uploads returned no job id"}
	}

	res, err := PollJob[assetJobResult](ctx, c, "/asset-uploads/"+submit.Job.ID, PollOptions{
		Initial: 500 * time.Millisecond,
		Max:     8 * time.Second,
		Timeout: 60 * time.Second,
	})
	if err != nil {
		// Map known asset-upload failure codes to canvacli stable codes.
		if apiErr, ok := err.(*APIError); ok {
			switch apiErr.Code {
			case "file_too_big":
				apiErr.Code = "asset_upload_too_large"
			case "import_failed", "fetch_failed":
				apiErr.Code = "asset_upload_failed"
			}
		}
		return nil, err
	}
	return &res.Asset, nil
}

// GetAsset returns the Asset object for an asset id.
func (c *Client) GetAsset(ctx context.Context, id string) (*Asset, error) {
	var env struct {
		Asset Asset `json:"asset"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/assets/"+id, nil, &env); err != nil {
		return nil, err
	}
	return &env.Asset, nil
}
