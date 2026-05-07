package auth

import (
	"sync"

	"golang.org/x/oauth2"
)

// PersistingSource wraps an upstream TokenSource and persists tokens to disk
// every time they change. Required because Canva rotates refresh tokens —
// the old refresh token becomes invalid the moment a new one is issued.
type PersistingSource struct {
	mu       sync.Mutex
	upstream oauth2.TokenSource
	path     string
	last     string
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
	if tok.AccessToken != p.last {
		if saveErr := SaveToken(p.path, tok); saveErr == nil {
			p.last = tok.AccessToken
		}
	}
	return tok, nil
}
