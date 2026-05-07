package api

import (
	"context"
	"encoding/json"
	"fmt"
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
	resp, err := c.doCtx(ctx, http.MethodGet, "/brand-templates/"+id+"/dataset", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &APIError{Code: "template_not_found", Message: fmt.Sprintf("template %q not found", id)}
	}
	var env Dataset
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env, nil
}
