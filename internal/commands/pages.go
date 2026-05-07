package commands

import (
	"encoding/json"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
	"github.com/spf13/cobra"
)

// NewPages lists the pages of a design as NDJSON and mirrors them into
// the local cache. Replaces the foundation-phase stub.
//
// Backed by the GET /designs/{id}/pages PREVIEW endpoint — see api/pages.go
// for the breaking-change risk.
func NewPages() *cobra.Command {
	var flagLimit int
	cmd := &cobra.Command{
		Use:   "pages <name|id>",
		Short: "List pages of a design (NDJSON)",
		Long: `List the pages of a Canva design with thumbnails and dimensions.

NOTE: this is backed by Canva's preview /designs/{id}/pages endpoint,
which the vendor flags for breaking changes. Cached results are written
to the local design_pages table.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			cl, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()
			id, err := resolver.New(ch, cl).ResolveDesign(args[0])
			if err != nil {
				return err
			}
			now := time.Now().Unix()
			count := 0
			err = cl.ListAllPages(ctx, id, func(p api.Page) error {
				if flagLimit > 0 && count >= flagLimit {
					return nil
				}
				raw, _ := json.Marshal(p)
				thumb := ""
				if p.Thumbnail != nil {
					thumb = p.Thumbnail.URL
				}
				width, height := 0, 0
				if p.Dimensions != nil {
					width = int(p.Dimensions.Width)
					height = int(p.Dimensions.Height)
				}
				_ = ch.UpsertDesignPage(cache.DesignPage{
					DesignID:     id,
					Index:        p.Index,
					Width:        width,
					Height:       height,
					ThumbnailURL: thumb,
					RawJSON:      string(raw),
					FetchedAt:    now,
				})
				row := map[string]any{
					"design_id": id,
					"index":     p.Index,
				}
				if p.Dimensions != nil {
					row["width"] = width
					row["height"] = height
				}
				if thumb != "" {
					row["thumbnail_url"] = thumb
				}
				count++
				return output.EmitJSON(os.Stdout, row)
			})
			return err
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 0, "max number of pages to emit (0 = all)")
	return cmd
}
