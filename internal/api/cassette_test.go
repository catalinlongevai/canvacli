package api

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/dnaeon/go-vcr.v3/cassette"
	"gopkg.in/dnaeon/go-vcr.v3/recorder"
)

// vcrClient builds an *api.Client wired to a go-vcr recorder. Cassettes
// live at testdata/cassettes/<name>.yaml. To re-record from live Canva,
// set CANVACLI_RECORD=1 (also ensure you're logged in via `canva login`).
//
// Without CANVACLI_RECORD, tests run fully offline against recorded responses.
func vcrClient(t *testing.T, name string) (*Client, func()) {
	t.Helper()
	mode := recorder.ModeReplayOnly
	if os.Getenv("CANVACLI_RECORD") != "" {
		mode = recorder.ModeRecordOnly
	}
	cassettePath := filepath.Join("testdata", "cassettes", name)
	r, err := recorder.NewWithOptions(&recorder.Options{
		CassetteName:       cassettePath,
		Mode:               mode,
		SkipRequestLatency: true,
	})
	if err != nil {
		t.Fatalf("recorder.New: %v", err)
	}
	r.AddHook(redactSensitive, recorder.BeforeSaveHook)
	r.SetMatcher(matcher)

	httpClient := &http.Client{Transport: r}

	token := "REDACTED"
	if mode == recorder.ModeRecordOnly {
		// Read live token from the user's config dir for recording sessions.
		base, err := os.UserConfigDir()
		if err != nil {
			t.Fatalf("UserConfigDir: %v", err)
		}
		data, err := os.ReadFile(filepath.Join(base, "canvacli", "token.json"))
		if err != nil {
			t.Fatalf("read token: %v (run `canva login` first)", err)
		}
		var tok struct {
			AccessToken string `json:"access_token"`
		}
		if err := json.Unmarshal(data, &tok); err != nil {
			t.Fatalf("parse token: %v", err)
		}
		token = tok.AccessToken
	}

	client := NewClient(token, WithHTTPClient(httpClient))
	return client, func() {
		if err := r.Stop(); err != nil {
			t.Errorf("recorder.Stop: %v", err)
		}
	}
}

// jwtLike matches JWT (3-part) and JWE compact (5-part, possibly with empty
// segments) token strings. Used to scrub any incidentally-token-shaped values
// that show up in response bodies (e.g. signed design-permalink tokens in
// Canva's `edit_url` / `view_url` query strings).
var jwtLike = regexp.MustCompile(`eyJ[A-Za-z0-9_-]{8,}(?:\.[A-Za-z0-9_-]*){2,5}`)

// redactSensitive strips Authorization headers and any token-shaped values
// before cassette files hit disk.
func redactSensitive(i *cassette.Interaction) error {
	if h, ok := i.Request.Headers["Authorization"]; ok && len(h) > 0 {
		i.Request.Headers["Authorization"] = []string{"Bearer REDACTED"}
	}
	// Sanitize Set-Cookie / response auth headers if present
	for _, key := range []string{"Set-Cookie", "X-Amz-Security-Token"} {
		if _, ok := i.Response.Headers[key]; ok {
			i.Response.Headers[key] = []string{"REDACTED"}
		}
	}
	// Scrub token-shaped strings from request and response bodies.
	i.Request.Body = jwtLike.ReplaceAllString(i.Request.Body, "REDACTED_TOKEN")
	i.Response.Body = jwtLike.ReplaceAllString(i.Response.Body, "REDACTED_TOKEN")
	return nil
}

// matcher ignores Authorization (since it's REDACTED in cassettes) when
// matching requests during replay.
func matcher(req *http.Request, i cassette.Request) bool {
	if req.Method != i.Method {
		return false
	}
	// URL match: ignore the access token query string parameters
	if !strings.HasPrefix(req.URL.String(), i.URL) && req.URL.String() != i.URL {
		// fallback: compare path + query
		if req.URL.Path != mustParsePath(i.URL) {
			return false
		}
	}
	return true
}

func mustParsePath(u string) string {
	idx := strings.Index(u, "?")
	if idx < 0 {
		return u
	}
	return u[:idx]
}

func TestVCR_Me(t *testing.T) {
	c, cleanup := vcrClient(t, "me")
	defer cleanup()

	user, err := c.Me(context.Background())
	if err != nil {
		t.Fatalf("Me: %v", err)
	}
	if user.ID == "" {
		t.Fatal("expected non-empty ID")
	}
	if user.TeamID == "" {
		t.Fatal("expected non-empty TeamID")
	}
}

func TestVCR_ListDesigns(t *testing.T) {
	c, cleanup := vcrClient(t, "list_designs")
	defer cleanup()

	count := 0
	err := c.ListDesigns(context.Background(), func(d Design) error {
		count++
		if d.ID == "" || d.Title == "" {
			t.Errorf("incomplete design: %+v", d)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ListDesigns: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one design (Project DW)")
	}
}

func TestVCR_WalkFolders(t *testing.T) {
	c, cleanup := vcrClient(t, "walk_folders")
	defer cleanup()

	folders := []Folder{}
	err := c.WalkFolders(context.Background(), func(f Folder, parent string) error {
		folders = append(folders, f)
		return nil
	})
	if err != nil {
		t.Fatalf("WalkFolders: %v", err)
	}
	// User has at minimum the Uploads folder.
	if len(folders) == 0 {
		t.Fatal("expected at least one folder")
	}
}

func TestVCR_CreateExport(t *testing.T) {
	c, cleanup := vcrClient(t, "create_export")
	defer cleanup()

	res, err := c.CreateExport(context.Background(), ExportRequest{
		DesignID: "DAF7Q_-7g9Q",
		Format:   ExportFormat{Type: "pdf"},
	})
	if err != nil {
		t.Fatalf("CreateExport: %v", err)
	}
	if len(res.URLs) == 0 {
		t.Fatal("expected at least one export URL — bug regression check")
	}
	for _, u := range res.URLs {
		if !strings.HasPrefix(u, "https://") {
			t.Errorf("expected https URL, got %q", u)
		}
	}
}
