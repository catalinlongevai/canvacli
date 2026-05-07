package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_AddsAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("token-abc", WithBaseURL(srv.URL))
	resp, err := c.do(http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer token-abc" {
		t.Fatalf("expected Bearer token-abc, got %q", gotAuth)
	}
}

func TestClient_DefaultBaseURL(t *testing.T) {
	c := NewClient("t")
	if c.baseURL != "https://api.canva.com/rest/v1" {
		t.Fatalf("default base URL wrong: %q", c.baseURL)
	}
}
