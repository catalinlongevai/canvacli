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

// ImportedDesign is the per-design payload inside an /imports job result.
// Importing a single multi-page document yields a single Design with
// page_count > 1 (research v2-assets-imports-api.md §"Multi-page handling"),
// but always iterate defensively — Canva may split very large imports.
type ImportedDesign struct {
	ID        string `json:"id"`
	Title     string `json:"title,omitempty"`
	URL       string `json:"url,omitempty"`
	PageCount int    `json:"page_count,omitempty"`
	UpdatedAt int64  `json:"updated_at,omitempty"`
	CreatedAt int64  `json:"created_at,omitempty"`
	Thumbnail *DesignThumbnail `json:"thumbnail,omitempty"`
	URLs      *struct {
		EditURL string `json:"edit_url,omitempty"`
		ViewURL string `json:"view_url,omitempty"`
	} `json:"urls,omitempty"`
}

// ImportResult mirrors the inlined `job` payload on /imports/{id} success.
// The Connect API returns `result.designs[]` nested under the job; PollJob[T]
// decodes T from the entire `job` body, so we expose `Result.Designs` here
// to match `job.result.designs` (research §"Imports — file → Canva design").
type ImportResult struct {
	Result struct {
		Designs []ImportedDesign `json:"designs"`
	} `json:"result"`
}

// ImportFile POSTs raw bytes to /imports with an Import-Metadata header
// carrying the title (base64) and optional mime_type. Polls the import
// job to completion. mimeType may be "" — Canva will sniff if omitted.
//
// Critical: this endpoint takes application/octet-stream raw bytes, NOT
// multipart/form-data. PNG/JPEG are NOT supported by /imports — route
// images to /asset-uploads at the command layer (spec §8).
func (c *Client) ImportFile(ctx context.Context, name, mimeType string, data io.Reader) (*ImportResult, error) {
	body, err := io.ReadAll(data)
	if err != nil {
		return nil, err
	}

	metadata := map[string]string{
		"title_base64": base64.StdEncoding.EncodeToString([]byte(name)),
	}
	if mimeType != "" {
		metadata["mime_type"] = mimeType
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/imports", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Import-Metadata", string(metadataJSON))

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
		return nil, &APIError{Code: "import_failed", Message: "imports returned no job id"}
	}

	res, err := PollJob[ImportResult](ctx, c, "/imports/"+submit.Job.ID, PollOptions{
		Initial: 500 * time.Millisecond,
		Max:     8 * time.Second,
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}
