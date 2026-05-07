package auth

import (
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

// PersistingSource wraps an upstream TokenSource, persists tokens to disk
// on rotation, and supports forced invalidation for 401-retry semantics.
//
// Canva rotates refresh tokens (single-use), so persistence on every
// change is mandatory — losing the rotated token bricks the credential.
type PersistingSource struct {
	mu       sync.Mutex
	upstream oauth2.TokenSource
	path     string
	last     string

	// inner is the most recently issued token. When Invalidate is called,
	// we mutate inner.Expiry so the upstream ReuseTokenSource considers
	// the token invalid on the next Token() call and forces a refresh.
	inner *oauth2.Token
}

func NewPersistingSource(upstream oauth2.TokenSource, path string) *PersistingSource {
	return &PersistingSource{upstream: upstream, path: path}
}

func (p *PersistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.upstream.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.inner = tok
	if tok.AccessToken != p.last {
		if saveErr := SaveToken(p.path, tok); saveErr != nil {
			// Save failures are surfaced via stderr — we cannot return
			// an error here without breaking the oauth2.TokenSource
			// contract, but a silent swallow can permanently lock users
			// out when refresh tokens rotate.
			fmt.Fprintln(os.Stderr, "canvacli: WARNING: failed to persist refreshed token:", saveErr)
		} else {
			p.last = tok.AccessToken
		}
	}
	return tok, nil
}

// Invalidate marks the cached token as expired so the next Token() call
// forces a refresh from the upstream. Used by the 401-retry transport to
// recover from server-side revocation while the cached token still has
// a valid expiry.
func (p *PersistingSource) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.inner != nil {
		p.inner.Expiry = time.Now().Add(-time.Hour)
	}
}
