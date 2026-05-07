package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

type pageEnvelope[T any] struct {
	Items        []T    `json:"items"`
	Continuation string `json:"continuation"`
}

func Paginate[T any](ctx context.Context, c *Client, path string, visit func(T) error) error {
	cursor := ""
	for {
		full := path
		if cursor != "" {
			sep := "?"
			if u, _ := url.Parse(path); u != nil && u.RawQuery != "" {
				sep = "&"
			}
			full = path + sep + "continuation=" + url.QueryEscape(cursor)
		}
		resp, err := c.doCtx(ctx, http.MethodGet, full, nil)
		if err != nil {
			return err
		}
		var env pageEnvelope[T]
		decErr := json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		if decErr != nil {
			return decErr
		}
		for _, it := range env.Items {
			if err := visit(it); err != nil {
				return err
			}
		}
		if env.Continuation == "" {
			return nil
		}
		cursor = env.Continuation
	}
}
