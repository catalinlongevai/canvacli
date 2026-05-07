package auth

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

// rotatingSource issues two different access tokens on successive Token() calls,
// so we can verify that the retry actually used the second one.
type rotatingSource struct {
	calls atomic.Int32
}

func (r *rotatingSource) Token() (*oauth2.Token, error) {
	n := r.calls.Add(1)
	return &oauth2.Token{
		AccessToken: "tok-" + string(rune('0'+n)),
		Expiry:      time.Now().Add(time.Hour),
	}, nil
}

func TestRefreshOn401Transport_RetriesWithNewToken(t *testing.T) {
	var seenAuth []string
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		seenAuth = append(seenAuth, r.Header.Get("Authorization"))
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	SaveToken(tokenPath, &oauth2.Token{AccessToken: "tok-0", Expiry: time.Now().Add(time.Hour)})

	ps := NewPersistingSource(&rotatingSource{}, tokenPath)
	tr := &RefreshOn401Transport{Base: http.DefaultTransport, Source: ps}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	req.Header.Set("Authorization", "Bearer tok-0")
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected final 200, got %d", resp.StatusCode)
	}
	if len(seenAuth) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(seenAuth))
	}
	if seenAuth[0] == seenAuth[1] {
		t.Fatalf("retry used same token: %q vs %q (refresh did not happen)", seenAuth[0], seenAuth[1])
	}
}

func TestRefreshOn401Transport_BodyReplayed(t *testing.T) {
	var seenBodies []string
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		seenBodies = append(seenBodies, string(body))
		n := calls.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	SaveToken(tokenPath, &oauth2.Token{AccessToken: "tok-0", Expiry: time.Now().Add(time.Hour)})
	ps := NewPersistingSource(&rotatingSource{}, tokenPath)
	tr := &RefreshOn401Transport{Base: http.DefaultTransport, Source: ps}

	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/x", bytes.NewReader([]byte(`{"a":1}`)))
	req.Header.Set("Authorization", "Bearer tok-0")
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()
	if len(seenBodies) != 2 {
		t.Fatalf("expected 2 bodies, got %d", len(seenBodies))
	}
	if seenBodies[0] != seenBodies[1] {
		t.Fatalf("body replay mismatch: %q vs %q", seenBodies[0], seenBodies[1])
	}
	if !strings.Contains(seenBodies[1], `"a":1`) {
		t.Fatalf("retry body lost content: %q", seenBodies[1])
	}
}

func TestRefreshOn401Transport_SingleRetryThenGiveUp(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "token.json")
	SaveToken(tokenPath, &oauth2.Token{AccessToken: "tok-0", Expiry: time.Now().Add(time.Hour)})
	ps := NewPersistingSource(&rotatingSource{}, tokenPath)
	tr := &RefreshOn401Transport{Base: http.DefaultTransport, Source: ps}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/x", nil)
	req.Header.Set("Authorization", "Bearer tok-0")
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 surfaced after retry exhaustion, got %d", resp.StatusCode)
	}
	if calls.Load() != 2 {
		t.Fatalf("expected exactly 2 server hits, got %d", calls.Load())
	}
}
