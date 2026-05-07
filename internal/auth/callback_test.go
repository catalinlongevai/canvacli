package auth

import (
	"context"
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

func TestCallbackServer_OAuthErrorReportedWhenStateValid(t *testing.T) {
	state := "good-state"
	srv, err := StartCallbackServer(state, []int{18771, 18772, 18773})
	if err != nil {
		t.Fatalf("StartCallbackServer: %v", err)
	}
	defer srv.Close()

	go func() {
		v := url.Values{"state": {state}, "error": {"access_denied"}}
		http.Get(srv.RedirectURI() + "?" + v.Encode())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err = srv.Wait(ctx)
	if err == nil {
		t.Fatal("expected oauth error, got nil")
	}
	if !strings.Contains(err.Error(), "access_denied") {
		t.Fatalf("expected access_denied in error, got %q", err.Error())
	}
}
