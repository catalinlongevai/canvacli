package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeResult struct {
	URL string `json:"url"`
}

func TestPollJob_SuccessAfterTwoPolls(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "j1", "status": "in_progress"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "j1", "status": "success", "result": map[string]any{"url": "https://x"}}})
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	res, err := PollJob[fakeResult](context.Background(), c, "/jobs/j1", PollOptions{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("PollJob: %v", err)
	}
	if res.URL != "https://x" {
		t.Fatalf("expected url, got %q", res.URL)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 polls, got %d", calls.Load())
	}
}

func TestPollJob_TimeoutReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "j1", "status": "in_progress"}})
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	_, err := PollJob[fakeResult](context.Background(), c, "/jobs/j1", PollOptions{Initial: 5 * time.Millisecond, Max: 5 * time.Millisecond, Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
