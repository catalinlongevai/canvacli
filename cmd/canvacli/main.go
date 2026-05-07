package main

import (
	"fmt"
	"os"

	"github.com/catalinlongevai/canvacli/internal/commands"
)

// Set at build time via -ldflags. Falls back to env vars at runtime if empty.
var (
	version           = "dev"
	commit            = "none"
	date              = "unknown"
	canvaClientID     = ""
	canvaClientSecret = ""
)

func main() {
	// Propagate build-time values into the commands package.
	commands.CanvaClientID = canvaClientID
	commands.CanvaClientSecret = canvaClientSecret

	if err := commands.NewRoot(version, commit, date).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
