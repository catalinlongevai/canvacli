package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"
)

const DefaultBaseURL = "https://api.canva.com/rest/v1"

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Option func(*Client)

func WithBaseURL(u string) Option          { return func(c *Client) { c.baseURL = u } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) do(method, path string, body any) (*http.Response, error) {
	return c.doCtx(context.Background(), method, path, body)
}

func (c *Client) doCtx(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}

// decodeJSON closes resp.Body and decodes into v.
func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// doJSON sends a request and decodes the JSON response into out. On non-2xx
// statuses it returns an *APIError with a stable code derived from the HTTP
// status, parsing Canva's error body and Retry-After when present.
//
// Use this for every JSON endpoint. doCtx (raw) remains for binary downloads.
func (c *Client) doJSON(ctx context.Context, method, path string, body, out any) error {
	resp, err := c.doCtx(ctx, method, path, body)
	if err != nil {
		return &APIError{Code: "network", Message: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			return nil
		}
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return mapHTTPError(resp)
}

func mapHTTPError(resp *http.Response) *APIError {
	// Try to decode Canva's error envelope: {"code":"...","message":"..."}.
	var canvaErr struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&canvaErr)

	apiErr := &APIError{HTTPStatus: resp.StatusCode, Message: canvaErr.Message}
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		apiErr.Code = "auth_revoked"
		if apiErr.Message == "" {
			apiErr.Message = "authentication failed or expired"
		}
	case http.StatusForbidden:
		apiErr.Code = "permission_denied"
		if apiErr.Message == "" {
			apiErr.Message = "permission denied (scope insufficient or plan does not include this endpoint)"
		}
	case http.StatusNotFound:
		apiErr.Code = "not_found"
		if apiErr.Message == "" {
			apiErr.Message = "resource not found"
		}
	case http.StatusTooManyRequests:
		apiErr.Code = "rate_limited"
		if ra := resp.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil {
				apiErr.WaitSeconds = secs
			}
		}
		if apiErr.Message == "" {
			apiErr.Message = "rate limited"
		}
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		apiErr.Code = "validation"
		if apiErr.Message == "" {
			apiErr.Message = "validation failed"
		}
	default:
		if resp.StatusCode >= 500 {
			apiErr.Code = "api_unavailable"
			if apiErr.Message == "" {
				apiErr.Message = http.StatusText(resp.StatusCode)
			}
		} else {
			apiErr.Code = "http_error"
			if apiErr.Message == "" {
				apiErr.Message = http.StatusText(resp.StatusCode)
			}
		}
	}
	// If the server returned a structured `code`, use it (it's more specific).
	if canvaErr.Code != "" {
		apiErr.Code = canvaErr.Code
	}
	return apiErr
}
