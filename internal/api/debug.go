package api

import (
	"fmt"
	"net/http"
	"os"
	"time"
)

// DebugTransport wraps an http.RoundTripper to log request/response metadata
// (method, URL, status, duration) to stderr. NEVER logs request or response
// bodies — they may contain user data (design titles, autofill content, etc.).
type DebugTransport struct {
	Base http.RoundTripper
}

func (t *DebugTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := t.Base.RoundTrip(req)
	elapsed := time.Since(start)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[canvacli-debug] %s %s -> ERROR (%s): %v\n",
			req.Method, req.URL.Redacted(), elapsed.Round(time.Millisecond), err)
		return resp, err
	}
	fmt.Fprintf(os.Stderr, "[canvacli-debug] %s %s -> %d (%s)\n",
		req.Method, req.URL.Redacted(), resp.StatusCode, elapsed.Round(time.Millisecond))
	return resp, nil
}
