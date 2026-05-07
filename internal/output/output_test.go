package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestEmitJSON_Compact(t *testing.T) {
	var buf bytes.Buffer
	if err := EmitJSON(&buf, map[string]any{"a": 1, "b": "x"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\n  ") {
		t.Fatalf("output contains pretty-printing: %q", buf.String())
	}
}

func TestEmitError_AuthRevoked_Exit2(t *testing.T) {
	var buf bytes.Buffer
	got := EmitError(&buf, "auth_revoked", "expired", nil)
	if got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
	if !strings.Contains(buf.String(), `"fix":"canva login"`) {
		t.Fatalf("missing fix: %q", buf.String())
	}
}

func TestProjectFields_Filters(t *testing.T) {
	got := ProjectFields(map[string]any{"a": 1, "b": 2, "c": 3}, "a,c")
	if _, has := got["b"]; has {
		t.Fatal("b should be filtered out")
	}
}

func TestProjectFields_AllReturnsMap(t *testing.T) {
	in := map[string]any{"a": 1, "b": 2}
	got := ProjectFields(in, "all")
	if len(got) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(got))
	}
}
