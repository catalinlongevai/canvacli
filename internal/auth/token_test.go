package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestSaveAndLoadToken_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	tok := &oauth2.Token{
		AccessToken:  "a",
		RefreshToken: "r",
		Expiry:       time.Now().Add(1 * time.Hour).Round(time.Second),
		TokenType:    "Bearer",
	}
	if err := SaveToken(path, tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	got, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if got.AccessToken != "a" || got.RefreshToken != "r" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestSaveToken_Permissions0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions on windows are a no-op")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	tok := &oauth2.Token{AccessToken: "a"}
	if err := SaveToken(path, tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}
