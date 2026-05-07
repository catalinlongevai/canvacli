package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"
)

type ExportFormat struct {
	Type string `json:"type"`
}

type ExportRequest struct {
	DesignID string       `json:"design_id"`
	Format   ExportFormat `json:"format"`
}

type ExportResult struct {
	URLs []string `json:"urls"`
}

func (c *Client) CreateExport(ctx context.Context, req ExportRequest) (*ExportResult, error) {
	var s struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/exports", req, &s); err != nil {
		return nil, err
	}
	res, err := PollJob[ExportResult](ctx, c, "/exports/"+s.Job.ID, PollOptions{
		Initial: 250 * time.Millisecond,
		Max:     5 * time.Second,
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// DownloadTo downloads a single export URL to outPath. Streams to disk.
// No Authorization header is sent — Canva export download URLs are S3
// signed URLs that reject Bearer tokens (sending one would also leak it).
func (c *Client) DownloadTo(ctx context.Context, urlStr, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &APIError{
			Code:       "download_failed",
			Message:    "export download returned " + http.StatusText(resp.StatusCode) + " (the URL likely expired — exports are valid for 24h)",
			HTTPStatus: resp.StatusCode,
		}
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}
