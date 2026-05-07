package commands

import (
	"github.com/spf13/cobra"
)

var (
	flagJSON     bool
	flagNoCache  bool
	flagQuiet    bool
	flagAutoWait bool
	flagDebug    bool
)

// FlagDebug is read by the API client to enable HTTP request/response logging.
func FlagDebug() bool { return flagDebug }

func NewRoot(version, commit, date string) *cobra.Command {
	root := &cobra.Command{
		Use:     "canva",
		Short:   "Agent-first CLI for the Canva Connect API",
		Version: version + " (commit " + commit + ", built " + date + ")",
	}
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "force JSON output (auto-on when piped)")
	root.PersistentFlags().BoolVar(&flagNoCache, "no-cache", false, "bypass local cache, force API call")
	root.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "suppress progress output")
	root.PersistentFlags().BoolVar(&flagAutoWait, "auto-wait", false, "auto-retry on 429 once, capped at 60s")
	root.PersistentFlags().BoolVar(&flagDebug, "debug", false, "log HTTP requests/responses to stderr")

	root.AddCommand(NewLogin())
	root.AddCommand(NewLogout())
	root.AddCommand(NewWhoami())
	root.AddCommand(NewTemplates())
	root.AddCommand(NewCreate())
	root.AddCommand(NewList())
	root.AddCommand(NewExport())
	root.AddCommand(NewFolders())
	root.AddCommand(NewSchema())
	root.AddCommand(NewSQL())
	root.AddCommand(NewMCP())

	// v2 surface — stubs in Phase 3a, replaced by per-resource Phase 3b agents.
	root.AddCommand(NewSync())
	root.AddCommand(NewSearch())
	root.AddCommand(NewPages())
	root.AddCommand(NewImport())
	root.AddCommand(NewResize())
	root.AddCommand(NewAssets())
	root.AddCommand(NewComments())

	return root
}
