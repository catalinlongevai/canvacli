package api

import (
	"context"
	"fmt"
	"net/http"
)

// PageDimensions matches Canva's PageDimensions schema. Both fields are
// floats in the wire format (e.g. 1920.0 / 1080.0) but always integral in
// practice, so we decode as int via float64-then-truncate; we keep them as
// int here because page dimensions are always whole pixels.
type PageDimensions struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// PageThumbnail mirrors the embedded thumbnail object on a page. The URL is
// a signed S3 URL valid for several hours, so callers should not persist it.
type PageThumbnail struct {
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
	URL    string `json:"url"`
}

// Page is the per-page metadata returned by the Pages preview endpoint.
//
// Only Index is guaranteed to be present. Dimensions and Thumbnail are
// absent for unbounded design types like whiteboards.
type Page struct {
	Index      int             `json:"index"`
	Dimensions *PageDimensions `json:"dimensions,omitempty"`
	Thumbnail  *PageThumbnail  `json:"thumbnail,omitempty"`
}

// ListPages returns the pages of a design.
//
// NOTE: this hits the GET /designs/{id}/pages PREVIEW endpoint. Per Canva's
// docs the preview tag means breaking changes can ship without notice and
// integrations relying on preview APIs are disqualified from public-app
// review. If/when Canva ships a stable replacement, the response shape
// here may change.
//
// Pagination uses offset (1-based) + limit, NOT the continuation cursor
// used by other Connect list endpoints. There is no total count and no
// next_offset field — the caller must increment offset by limit until
// fewer than `limit` items come back.
//
// The total page count for a design is exposed separately via
// GET /designs/{id} -> design.page_count.
func (c *Client) ListPages(ctx context.Context, designID string, limit, offset int) ([]Page, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	if offset <= 0 {
		offset = 1
	}
	path := fmt.Sprintf("/designs/%s/pages?offset=%d&limit=%d", designID, offset, limit)
	var env struct {
		Items []Page `json:"items"`
	}
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &env); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Code == "not_found" {
			apiErr.Code = "design_not_found"
		}
		return nil, err
	}
	return env.Items, nil
}

// ListAllPages walks ListPages with offset/limit until exhaustion and
// invokes visit for each page. Returns on the first visit error or
// network error.
func (c *Client) ListAllPages(ctx context.Context, designID string, visit func(Page) error) error {
	const pageSize = 200
	offset := 1
	for {
		items, err := c.ListPages(ctx, designID, pageSize, offset)
		if err != nil {
			return err
		}
		for _, p := range items {
			if err := visit(p); err != nil {
				return err
			}
		}
		if len(items) < pageSize {
			return nil
		}
		offset += pageSize
	}
}
