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
	"time"
)

// TestUploadAsset_HappyPath verifies the request shape (octet-stream body,
// base64-encoded name in the metadata header) and that the polling loop
// extracts the asset from the success response.
func TestUploadAsset_HappyPath(t *testing.T) {
	var (
		uploadCalls atomic.Int32
		pollCalls   atomic.Int32
		seenName    string
		seenBody    []byte
		seenCT      string
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/asset-uploads"):
			uploadCalls.Add(1)
			seenCT = r.Header.Get("Content-Type")
			meta := r.Header.Get("Asset-Upload-Metadata")
			var parsed struct {
				NameBase64 string `json:"name_base64"`
			}
			_ = json.Unmarshal([]byte(meta), &parsed)
			if parsed.NameBase64 != "" {
				dec, _ := base64.StdEncoding.DecodeString(parsed.NameBase64)
				seenName = string(dec)
			}
			seenBody, _ = io.ReadAll(r.Body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{"id": "j1", "status": "in_progress"},
			})

		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/asset-uploads/j1"):
			n := pollCalls.Add(1)
			if n < 3 {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"job": map[string]any{"id": "j1", "status": "in_progress"},
				})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{
					"id":     "j1",
					"status": "success",
					"asset": map[string]any{
						"id":         "Msd59349ff",
						"name":       "hero.png",
						"type":       "image",
						"updated_at": int64(1715000000),
						"thumbnail":  map[string]any{"url": "https://t.example/thumb.png"},
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
	asset, err := c.UploadAsset(context.Background(), "hero.png", strings.NewReader("BYTES"))
	if err != nil {
		t.Fatalf("UploadAsset: %v", err)
	}
	if asset.ID != "Msd59349ff" {
		t.Errorf("asset id: got %q", asset.ID)
	}
	if asset.Name != "hero.png" {
		t.Errorf("asset name: got %q", asset.Name)
	}
	if asset.Type != "image" {
		t.Errorf("asset type: got %q", asset.Type)
	}
	if asset.Thumbnail == nil || asset.Thumbnail.URL != "https://t.example/thumb.png" {
		t.Errorf("asset thumbnail: %+v", asset.Thumbnail)
	}
	if seenCT != "application/octet-stream" {
		t.Errorf("Content-Type: got %q", seenCT)
	}
	if seenName != "hero.png" {
		t.Errorf("base64-decoded name: got %q", seenName)
	}
	if string(seenBody) != "BYTES" {
		t.Errorf("body: got %q", seenBody)
	}
	if uploadCalls.Load() != 1 {
		t.Errorf("expected 1 POST, got %d", uploadCalls.Load())
	}
	if pollCalls.Load() != 3 {
		t.Errorf("expected 3 polls (2 in_progress + 1 success), got %d", pollCalls.Load())
	}
}

// TestUploadAsset_FileTooBig verifies that the API's `file_too_big` error
// is mapped to canvacli's stable `asset_upload_too_large` code.
func TestUploadAsset_FileTooBig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{"id": "j1", "status": "in_progress"},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"job": map[string]any{
				"id":     "j1",
				"status": "failed",
				"error":  map[string]any{"error": "file_too_big", "message": "too large"},
			},
		})
	}))
	defer srv.Close()

	c := NewClient("tok", WithBaseURL(srv.URL))
	c.http.Timeout = 5 * time.Second
	_, err := c.UploadAsset(context.Background(), "x.png", strings.NewReader("data"))
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T (%v)", err, err)
	}
	if apiErr.Code != "asset_upload_too_large" {
		t.Errorf("expected asset_upload_too_large, got %q", apiErr.Code)
	}
}

func TestGetAsset_DecodesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/assets/Msd59349ff") {
			http.Error(w, "bad", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"asset": map[string]any{
				"id":   "Msd59349ff",
				"name": "x",
				"type": "image",
			},
		})
	}))
	defer srv.Close()

	c := NewClient("tok", WithBaseURL(srv.URL))
	asset, err := c.GetAsset(context.Background(), "Msd59349ff")
	if err != nil {
		t.Fatalf("GetAsset: %v", err)
	}
	if asset.ID != "Msd59349ff" || asset.Name != "x" || asset.Type != "image" {
		t.Errorf("unexpected asset: %+v", asset)
	}
}
