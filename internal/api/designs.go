package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	resp, err := c.doCtx(ctx, http.MethodGet, "/designs/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &APIError{Code: "design_not_found", Message: fmt.Sprintf("design %q not found", id)}
	}
	var env struct {
		Design Design `json:"design"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Design, nil
}
