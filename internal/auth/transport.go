package auth

import (
	"bytes"
	"io"
	"net/http"
)

// RefreshOn401Transport wraps an underlying transport. On a 401 it forces
// a refresh from the source and replays the request once. Catches
// server-side revocation that expiry-based refresh can't see.
type RefreshOn401Transport struct {
	Base   http.RoundTripper
	Source *PersistingSource
}

func (t *RefreshOn401Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	var body []byte
	if req.Body != nil && req.GetBody == nil {
		var err error
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	resp, err := t.Base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	resp.Body.Close()

	tok, err := t.Source.Token()
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		b, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = b
	}
	clone.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	return t.Base.RoundTrip(clone)
}
