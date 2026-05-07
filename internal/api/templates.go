package api

import (
	"context"
	"net/http"
)

type BrandTemplate struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	UpdatedAt int64  `json:"updated_at"`
}

type DatasetField struct {
	Type string `json:"type"`
}

type Dataset struct {
	Fields map[string]DatasetField `json:"dataset"`
}

func (c *Client) ListTemplates(ctx context.Context, visit func(BrandTemplate) error) error {
	return Paginate[BrandTemplate](ctx, c, "/brand-templates", visit)
}

func (c *Client) GetTemplateDataset(ctx context.Context, id string) (*Dataset, error) {
	var env Dataset
	if err := c.doJSON(ctx, http.MethodGet, "/brand-templates/"+id+"/dataset", nil, &env); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Code == "not_found" {
			apiErr.Code = "template_not_found"
		}
		return nil, err
	}
	return &env, nil
}
