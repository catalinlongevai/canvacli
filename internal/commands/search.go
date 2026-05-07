package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

// NewSearch returns the `canva search "<query>"` command. Replaces the
// foundation phase stub with a real FTS5-backed implementation per spec
// §4.1 and §7.
//
// Defaults: limit=50 (cap 1000), all five FTS sources enabled.
// Output: NDJSON to stdout (one JSON object per hit).
//
// Empty cache (no rows in any FTS-indexed table) returns the cache_empty
// error code per spec §7.4 — exit code 3, fix `canva sync`.
func NewSearch() *cobra.Command {
	var (
		flagType  string
		flagLimit int
	)
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "FTS5 search across mirrored designs, templates, comments, assets",
		Long: `Search the local FTS5 cache.

The query string is passed verbatim to SQLite FTS5, which supports:
  q3 banner          AND of two terms
  launch*            prefix match
  "q3 banner"        phrase match
  banner OR poster   boolean
  launch NOT draft   negation

Results are ranked per-source by bm25 (best first), then truncated by --limit.
Per-source ranks are not strictly comparable across types — pin --type when
strict ordering matters.

Run 'canva sync' first to populate the local cache.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()

			if flagType != "" && !cache.IsValidSearchType(flagType) {
				return fmt.Errorf("invalid --type %q: must be one of design, template, comment_thread, comment_reply, asset", flagType)
			}

			// Spec §7.4: empty cache returns cache_empty before running
			// the query (so the user gets the actionable fix message).
			empty, err := ch.SearchCacheEmpty()
			if err != nil {
				return err
			}
			if empty {
				return errors.New("cache_empty: no rows in local cache; run 'canva sync' first")
			}

			hits, err := ch.Search(cmd.Context(), args[0], flagType, flagLimit)
			if err != nil {
				return err
			}
			rows := make([]map[string]any, 0, len(hits))
			for _, h := range hits {
				m := map[string]any{
					"type": h.Type,
					"id":   h.ID,
					"rank": h.Rank,
				}
				if h.Title != "" {
					m["title"] = h.Title
				}
				if h.Text != "" {
					m["text"] = h.Text
				}
				if h.DesignID != "" {
					m["design_id"] = h.DesignID
				}
				rows = append(rows, m)
			}
			return output.EmitNDJSON(os.Stdout, rows)
		},
	}
	cmd.Flags().StringVar(&flagType, "type", "", "filter to one source: design, template, comment_thread, comment_reply, asset")
	cmd.Flags().IntVar(&flagLimit, "limit", 50, "max results (1-1000)")
	return cmd
}
