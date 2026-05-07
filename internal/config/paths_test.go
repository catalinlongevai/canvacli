package config

import (
	"strings"
	"testing"
)

func TestConfigDir_ReturnsCanvacliSubdir(t *testing.T) {
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if !strings.HasSuffix(dir, "canvacli") {
		t.Fatalf("expected suffix canvacli, got %q", dir)
	}
}

func TestTokenPath_HasJSONExtension(t *testing.T) {
	p, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath: %v", err)
	}
	if !strings.HasSuffix(p, "token.json") {
		t.Fatalf("expected suffix token.json, got %q", p)
	}
}

func TestCacheDBPath_EndsInDB(t *testing.T) {
	p, err := CacheDBPath()
	if err != nil {
		t.Fatalf("CacheDBPath: %v", err)
	}
	if !strings.HasSuffix(p, "cache.db") {
		t.Fatalf("expected suffix cache.db, got %q", p)
	}
}
