package api

import (
	"context"
	"net/http"
	"time"
)

type AutofillResult struct {
	Design Design `json:"design"`
}

type AutofillRequest struct {
	BrandTemplateID string         `json:"brand_template_id"`
	Data            map[string]any `json:"data"`
	Title           string         `json:"title,omitempty"`
}

func (c *Client) CreateAutofill(ctx context.Context, req AutofillRequest) (*AutofillResult, error) {
	var s struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/autofills", req, &s); err != nil {
		return nil, err
	}
	res, err := PollJob[AutofillResult](ctx, c, "/autofills/"+s.Job.ID, PollOptions{
		Initial: 250 * time.Millisecond,
		Max:     2 * time.Second,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}
