package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubResizeServer accepts POST /resizes and immediately returns a success
// job on GET /resizes/{id}. Returns the captured request body for
// per-test assertions.
func stubResizeServer(t *testing.T, designID string) (*httptest.Server, *string) {
	t.Helper()
	captured := new(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/resizes":
			body, _ := io.ReadAll(r.Body)
			*captured = string(body)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{"id": "j1", "status": "in_progress"},
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/resizes/"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"job": map[string]any{
					"id":     "j1",
					"status": "success",
					"result": map[string]any{
						"design": map[string]any{
							"id":    designID,
							"title": "Resized",
							"url":   "https://canva.com/design/" + designID,
						},
						"trial_information": map[string]any{
							"uses_remaining": 1,
							"upgrade_url":    "https://canva.com/upgrade",
						},
					},
				},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	return srv, captured
}

func TestResizeDesign_AllFourPresets(t *testing.T) {
	presets := []ResizePreset{
		ResizePresetDoc,
		ResizePresetEmail,
		ResizePresetPresentation,
		ResizePresetWhiteboard,
	}
	for _, preset := range presets {
		t.Run(string(preset), func(t *testing.T) {
			srv, captured := stubResizeServer(t, "D_new_"+string(preset))
			defer srv.Close()

			c := NewClient("t", WithBaseURL(srv.URL))
			res, err := c.ResizeDesign(context.Background(), ResizeRequest{
				DesignID: "D_src",
				Preset:   preset,
			})
			if err != nil {
				t.Fatalf("ResizeDesign(%s): %v", preset, err)
			}
			if res.Design.ID != "D_new_"+string(preset) {
				t.Errorf("unexpected new design ID: %q", res.Design.ID)
			}

			// Verify the wire body has the expected preset shape.
			var body struct {
				DesignID   string `json:"design_id"`
				DesignType struct {
					Type string `json:"type"`
					Name string `json:"name"`
				} `json:"design_type"`
			}
			if err := json.Unmarshal([]byte(*captured), &body); err != nil {
				t.Fatalf("decode body: %v", err)
			}
			if body.DesignID != "D_src" {
				t.Errorf("design_id mismatch: %q", body.DesignID)
			}
			if body.DesignType.Type != "preset" {
				t.Errorf("expected type=preset, got %q", body.DesignType.Type)
			}
			if body.DesignType.Name != string(preset) {
				t.Errorf("expected name=%s, got %q", preset, body.DesignType.Name)
			}

			if res.TrialInformation == nil || res.TrialInformation.UsesRemaining != 1 {
				t.Errorf("expected trial_information present with uses_remaining=1, got %+v", res.TrialInformation)
			}
		})
	}
}

func TestResizeDesign_RejectsInvalidPreset(t *testing.T) {
	c := NewClient("t", WithBaseURL("http://localhost:0"))
	_, err := c.ResizeDesign(context.Background(), ResizeRequest{
		DesignID: "D1",
		Preset:   ResizePreset("instagram_post"),
	})
	if err == nil {
		t.Fatal("expected error for invalid preset")
	}
	if !strings.Contains(err.Error(), "invalid preset") {
		t.Errorf("expected 'invalid preset' error, got %q", err.Error())
	}
}

func TestResizeDesign_RejectsBothPresetAndCustom(t *testing.T) {
	c := NewClient("t", WithBaseURL("http://localhost:0"))
	_, err := c.ResizeDesign(context.Background(), ResizeRequest{
		DesignID: "D1",
		Preset:   ResizePresetDoc,
		Width:    1080,
		Height:   1080,
	})
	if err == nil {
		t.Fatal("expected error when both Preset and Width/Height set")
	}
}

func TestResizeDesign_RejectsNeither(t *testing.T) {
	c := NewClient("t", WithBaseURL("http://localhost:0"))
	_, err := c.ResizeDesign(context.Background(), ResizeRequest{DesignID: "D1"})
	if err == nil {
		t.Fatal("expected error when neither Preset nor Width/Height set")
	}
}

func TestIsValidResizePreset(t *testing.T) {
	cases := map[string]bool{
		"doc":            true,
		"email":          true,
		"presentation":   true,
		"whiteboard":     true,
		"instagram_post": false,
		"":               false,
		"DOC":            false, // case sensitive
	}
	for in, want := range cases {
		if got := IsValidResizePreset(in); got != want {
			t.Errorf("IsValidResizePreset(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestExportRequest_PagesInsideFormat(t *testing.T) {
	// Verify the JSON wire shape: pages goes inside format, not at top level.
	req := ExportRequest{
		DesignID: "D1",
		Format:   ExportFormat{Type: "png", Pages: []int{1, 3, 5}},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	want := `{"design_id":"D1","format":{"type":"png","pages":[1,3,5]}}`
	if got != want {
		t.Errorf("wire JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
}

func TestExportRequest_EmptyPagesOmitted(t *testing.T) {
	// Backward compat: when Pages is empty, the field should be omitted.
	req := ExportRequest{
		DesignID: "D1",
		Format:   ExportFormat{Type: "pdf"},
	}
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	got := string(b)
	if strings.Contains(got, "pages") {
		t.Errorf("expected no 'pages' field when Pages is empty, got: %s", got)
	}
	want := `{"design_id":"D1","format":{"type":"pdf"}}`
	if got != want {
		t.Errorf("wire JSON mismatch:\n got: %s\nwant: %s", got, want)
	}
}
