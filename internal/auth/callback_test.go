package auth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCallbackServer_ReturnsCodeOnValidState(t *testing.T) {
	state := "abc123"
	srv, err := StartCallbackServer(state, []int{18765, 18766, 18767})
	if err != nil {
		t.Fatalf("StartCallbackServer: %v", err)
	}
	defer srv.Close()

	go func() {
		v := url.Values{"code": {"the-code"}, "state": {state}}
		http.Get(srv.RedirectURI() + "?" + v.Encode())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code, err := srv.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != "the-code" {
		t.Fatalf("expected code, got %q", code)
	}
	if !strings.HasPrefix(srv.RedirectURI(), "http://127.0.0.1:") {
		t.Fatalf("unexpected redirect URI: %q", srv.RedirectURI())
	}
	_ = io.Discard
}

func TestCallbackServer_RejectsBadState(t *testing.T) {
	srv, err := StartCallbackServer("expected", []int{18768, 18769, 18770})
	if err != nil {
		t.Fatalf("StartCallbackServer: %v", err)
	}
	defer srv.Close()

	go func() {
		v := url.Values{"code": {"the-code"}, "state": {"WRONG"}}
		http.Get(srv.RedirectURI() + "?" + v.Encode())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := srv.Wait(ctx); err == nil {
		t.Fatal("expected error from bad state, got nil")
	}
}
