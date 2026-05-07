package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"
)

type PollOptions struct {
	Initial time.Duration
	Max     time.Duration
	Timeout time.Duration
}

func DefaultPollOptions() PollOptions {
	return PollOptions{
		Initial: 250 * time.Millisecond,
		Max:     5 * time.Second,
		Timeout: 5 * time.Minute,
	}
}

// PollJob polls an async job endpoint until terminal status. On success it
// decodes T from the entire `job` object — Canva inlines result fields
// directly into job (e.g. `job.urls` on exports, `job.design` on autofill)
// rather than nesting under `job.result`. T's JSON tags must therefore
// match the shape of the inlined response.
func PollJob[T any](ctx context.Context, c *Client, path string, opts PollOptions) (T, error) {
	var zero T
	if opts.Initial == 0 {
		opts = DefaultPollOptions()
	}
	deadline := time.Now().Add(opts.Timeout)
	interval := opts.Initial
	for {
		if time.Now().After(deadline) {
			return zero, &APIError{Code: "job_timeout", Message: "job did not complete within timeout"}
		}
		resp, err := c.doCtx(ctx, http.MethodGet, path, nil)
		if err != nil {
			return zero, err
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return zero, readErr
		}
		// First pass: read status from a minimal envelope.
		var meta struct {
			Job struct {
				ID     string    `json:"id"`
				Status string    `json:"status"`
				Error  *APIError `json:"error,omitempty"`
			} `json:"job"`
		}
		if err := json.Unmarshal(body, &meta); err != nil {
			return zero, err
		}
		switch meta.Job.Status {
		case "success":
			// Second pass: extract the entire `job` body and decode T from it.
			var raw struct {
				Job json.RawMessage `json:"job"`
			}
			if err := json.Unmarshal(body, &raw); err != nil {
				return zero, err
			}
			var result T
			if err := json.Unmarshal(raw.Job, &result); err != nil {
				return zero, err
			}
			return result, nil
		case "failed":
			if meta.Job.Error != nil {
				return zero, meta.Job.Error
			}
			return zero, errors.New("job failed without error detail")
		case "in_progress", "pending", "":
			// keep polling
		default:
			return zero, &APIError{Code: "unknown_job_status", Message: meta.Job.Status}
		}
		time.Sleep(interval)
		interval *= 2
		if interval > opts.Max {
			interval = opts.Max
		}
	}
}
