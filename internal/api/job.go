package api

import (
	"context"
	"encoding/json"
	"errors"
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

type jobEnvelope[T any] struct {
	Job struct {
		ID     string    `json:"id"`
		Status string    `json:"status"`
		Result T         `json:"result"`
		Error  *APIError `json:"error,omitempty"`
	} `json:"job"`
}

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
		var env jobEnvelope[T]
		decErr := json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		if decErr != nil {
			return zero, decErr
		}
		switch env.Job.Status {
		case "success":
			return env.Job.Result, nil
		case "failed":
			if env.Job.Error != nil {
				return zero, env.Job.Error
			}
			return zero, errors.New("job failed without error detail")
		case "in_progress", "pending", "":
			// keep polling
		default:
			return zero, &APIError{Code: "unknown_job_status", Message: env.Job.Status}
		}
		time.Sleep(interval)
		interval *= 2
		if interval > opts.Max {
			interval = opts.Max
		}
	}
}
