package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestListPages_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/designs/D123/pages" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("offset") != "1" || q.Get("limit") != "50" {
			t.Errorf("unexpected query: offset=%s limit=%s", q.Get("offset"), q.Get("limit"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"index": 1, "dimensions": map[string]float64{"width": 1920, "height": 1080},
					"thumbnail": map[string]any{"width": 595, "height": 335, "url": "https://thumb/1.png"}},
				{"index": 2, "dimensions": map[string]float64{"width": 1920, "height": 1080},
					"thumbnail": map[string]any{"width": 595, "height": 335, "url": "https://thumb/2.png"}},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	pages, err := c.ListPages(context.Background(), "D123", 50, 1)
	if err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0].Index != 1 {
		t.Errorf("expected page index 1, got %d", pages[0].Index)
	}
	if pages[0].Dimensions == nil || pages[0].Dimensions.Width != 1920 {
		t.Errorf("expected dimensions 1920x1080, got %+v", pages[0].Dimensions)
	}
	if pages[0].Thumbnail == nil || pages[0].Thumbnail.URL != "https://thumb/1.png" {
		t.Errorf("expected thumbnail URL, got %+v", pages[0].Thumbnail)
	}
}

func TestListPages_DefaultsAndClamps(t *testing.T) {
	var capturedQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[]}`))
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	// limit <= 0 should default to 50; offset <= 0 should default to 1.
	if _, err := c.ListPages(context.Background(), "D123", 0, 0); err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if capturedQuery.Get("limit") != "50" {
		t.Errorf("expected default limit=50, got %s", capturedQuery.Get("limit"))
	}
	if capturedQuery.Get("offset") != "1" {
		t.Errorf("expected default offset=1, got %s", capturedQuery.Get("offset"))
	}

	// limit > 200 should clamp to 200.
	if _, err := c.ListPages(context.Background(), "D123", 5000, 7); err != nil {
		t.Fatalf("ListPages: %v", err)
	}
	if capturedQuery.Get("limit") != "200" {
		t.Errorf("expected clamped limit=200, got %s", capturedQuery.Get("limit"))
	}
	if capturedQuery.Get("offset") != "7" {
		t.Errorf("expected offset=7, got %s", capturedQuery.Get("offset"))
	}
}

func TestListAllPages_PaginatesUntilShortPage(t *testing.T) {
	const total = 13
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		off, _ := strconv.Atoi(r.URL.Query().Get("offset"))
		lim, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		items := []map[string]any{}
		for i := off; i < off+lim && i <= total; i++ {
			items = append(items, map[string]any{"index": i})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": items})
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	seen := []int{}
	err := c.ListAllPages(context.Background(), "D123", func(p Page) error {
		seen = append(seen, p.Index)
		return nil
	})
	if err != nil {
		t.Fatalf("ListAllPages: %v", err)
	}
	if len(seen) != total {
		t.Fatalf("expected %d pages, got %d", total, len(seen))
	}
	for i, idx := range seen {
		if idx != i+1 {
			t.Errorf("expected page %d at slot %d, got %d", i+1, i, idx)
		}
	}
}

func TestListPages_NotFound_RemapsCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":"design_not_found","message":"no such design"}`))
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	_, err := c.ListPages(context.Background(), "D_nope", 50, 1)
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "design_not_found" {
		t.Errorf("expected design_not_found code, got %q", apiErr.Code)
	}
}
