package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPaginate_FollowsContinuation(t *testing.T) {
	pages := [][]map[string]any{
		{{"id": "1"}, {"id": "2"}},
		{{"id": "3"}},
	}
	cursors := []string{"", "next-1", ""}
	idx := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items":        pages[idx],
			"continuation": cursors[idx+1],
		})
		idx++
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	got := []string{}
	err := Paginate[map[string]any](context.Background(), c, "/things", func(item map[string]any) error {
		got = append(got, item["id"].(string))
		return nil
	})
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
}
