package api

import (
	"context"
	"net/http"
)

type DesignThumbnail struct {
	URL string `json:"url"`
}

type Design struct {
	ID        string           `json:"id"`
	Title     string           `json:"title"`
	URL       string           `json:"url"`
	UpdatedAt int64            `json:"updated_at"`
	Thumbnail *DesignThumbnail `json:"thumbnail,omitempty"`
}

func (c *Client) ListDesigns(ctx context.Context, visit func(Design) error) error {
	return Paginate[Design](ctx, c, "/designs", visit)
}

func (c *Client) GetDesign(ctx context.Context, id string) (*Design, error) {
	var env struct {
		Design Design `json:"design"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/designs/"+id, nil, &env); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Code == "not_found" {
			apiErr.Code = "design_not_found"
		}
		return nil, err
	}
	return &env.Design, nil
}
