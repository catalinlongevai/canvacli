package commands

import "os"

// CanvaClientID and CanvaClientSecret are set at build time via -ldflags
// for release binaries. They default to empty, in which case we fall back
// to the CANVA_CLIENT_ID / CANVA_CLIENT_SECRET environment variables (the
// path used during local development).
//
// These vars are populated from main.go's package-level vars, which are
// the symbols actually injected by goreleaser/build-time ldflags.
var (
	CanvaClientID     = ""
	CanvaClientSecret = ""
)

func clientID() string {
	if CanvaClientID != "" {
		return CanvaClientID
	}
	return os.Getenv("CANVA_CLIENT_ID")
}

func clientSecret() string {
	if CanvaClientSecret != "" {
		return CanvaClientSecret
	}
	return os.Getenv("CANVA_CLIENT_SECRET")
}
