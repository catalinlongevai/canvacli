package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

// TestImportFile_HappyPath verifies the request shape (octet-stream body,
// base64 title in the Import-Metadata header) and that the polling loop
// extracts the result.designs[] payload from the success response.
func TestImportFile_HappyPath(t *testing.T) {
	var (
		postCalls atomic.Int32
		pollCalls atomic.Int32
		seenTitle string
		seenMime  string
		seenCT    string
		seenBody  []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/imports"):
			postCalls.Add(1)
			seenCT = r.Header.Get("Content-Type")
			meta := r.Header.Get("Import-Metadata")
			var parsed struct {
				TitleBase64 string `json:"title_base64"`
				MimeType    string `json:"mime_type"`
			}
			_ = json.Unmarshal([]byte(meta), &parsed)
			if parsed.TitleBase64 != "" {
				dec, _ := base64.StdEncoding.DecodeString(parsed.TitleBase64)
				seenTitle = string(dec)
			}
			seenMime = parsed.MimeType
			seenBody, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{"id": "imp1", "status": "in_progress"},
			})

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/imports/imp1"):
			n := pollCalls.Add(1)
			if n < 3 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"job": map[string]any{"id": "imp1", "status": "in_progress"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{
					"id":     "imp1",
					"status": "success",
					"result": map[string]any{
						"designs": []map[string]any{
							{
								"id":         "DAFimported1",
								"title":      "report",
								"page_count": 3,
								"updated_at": int64(1715000000),
								"urls":       map[string]any{"edit_url": "https://canva/edit", "view_url": "https://canva/view"},
							},
						},
					},
				},
			})
		default:
			t.Errorf("unexpected req: %s %s", r.Method, r.URL.Path)
			http.Error(w, "bad", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	c := NewClient("tok", WithBaseURL(srv.URL))
	res, err := c.ImportFile(context.Background(), "report.pdf", "application/pdf", strings.NewReader("PDFBYTES"))
	if err != nil {
		t.Fatalf("ImportFile: %v", err)
	}
	if len(res.Result.Designs) != 1 {
		t.Fatalf("expected 1 design, got %d", len(res.Result.Designs))
	}
	d := res.Result.Designs[0]
	if d.ID != "DAFimported1" {
		t.Errorf("design id: %q", d.ID)
	}
	if d.PageCount != 3 {
		t.Errorf("page_count: %d", d.PageCount)
	}
	if d.URLs == nil || d.URLs.EditURL != "https://canva/edit" {
		t.Errorf("urls: %+v", d.URLs)
	}
	if seenCT != "application/octet-stream" {
		t.Errorf("Content-Type: %q", seenCT)
	}
	if seenTitle != "report.pdf" {
		t.Errorf("base64-decoded title: %q", seenTitle)
	}
	if seenMime != "application/pdf" {
		t.Errorf("mime_type: %q", seenMime)
	}
	if string(seenBody) != "PDFBYTES" {
		t.Errorf("body: %q", seenBody)
	}
	if postCalls.Load() != 1 {
		t.Errorf("expected 1 POST, got %d", postCalls.Load())
	}
	if pollCalls.Load() != 3 {
		t.Errorf("expected 3 polls, got %d", pollCalls.Load())
	}
}

// TestImportFile_NoMimeOmitsKey verifies that an empty mime type omits
// the mime_type field from the Import-Metadata header (so Canva sniffs).
func TestImportFile_NoMimeOmitsKey(t *testing.T) {
	var seenMimeKey bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			meta := r.Header.Get("Import-Metadata")
			seenMimeKey = strings.Contains(meta, "mime_type")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{"id": "imp2", "status": "success", "result": map[string]any{"designs": []any{}}},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job": map[string]any{"id": "imp2", "status": "success", "result": map[string]any{"designs": []any{}}},
		})
	}))
	defer srv.Close()

	c := NewClient("tok", WithBaseURL(srv.URL))
	_, _ = c.ImportFile(context.Background(), "doc", "", strings.NewReader("x"))
	if seenMimeKey {
		t.Errorf("expected mime_type key to be omitted when empty")
	}
}
