//go:build never

// Package deps exists only to keep the 5 v1 dependencies referenced in
// go.mod via go-mod-tidy. It is not built (the `never` tag is never set).
// Once Phase 1 and Phase 2 source actually imports these packages, this
// file should be deleted.
package deps

import (
	_ "github.com/spf13/cobra"
	_ "golang.org/x/oauth2"
	_ "golang.org/x/term"
	_ "gopkg.in/dnaeon/go-vcr.v3/recorder"
	_ "modernc.org/sqlite"
)
