package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type stubSource struct {
	tok *oauth2.Token
	err error
}

func (s *stubSource) Token() (*oauth2.Token, error) { return s.tok, s.err }

func TestPersistingSource_WritesOnRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	first := &oauth2.Token{AccessToken: "old", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour)}
	if err := SaveToken(path, first); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	upstream := &stubSource{tok: &oauth2.Token{AccessToken: "new", RefreshToken: "rt2", Expiry: time.Now().Add(time.Hour)}}

	ps := NewPersistingSource(upstream, path)
	tok, err := ps.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "new" {
		t.Fatalf("expected new token, got %q", tok.AccessToken)
	}
	loaded, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded.AccessToken != "new" || loaded.RefreshToken != "rt2" {
		t.Fatalf("disk not updated: %+v", loaded)
	}
	_ = context.Background
}
