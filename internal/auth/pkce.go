package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// NewState returns a 32-byte url-safe random state token for CSRF protection
// of the OAuth authorization callback.
func NewState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
