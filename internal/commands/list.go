package commands

import (
	"encoding/json"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewList() *cobra.Command {
	var flagFields string
	var flagLimit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your designs (NDJSON when piped)",
		RunE: func(cmd *cobra.Command, _ []string) error {
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
			count := 0
			err = cl.ListDesigns(ctx, func(d api.Design) error {
				if flagLimit > 0 && count >= flagLimit {
					return nil
				}
				raw, _ := json.Marshal(d)
				thumb := ""
				if d.Thumbnail != nil {
					thumb = d.Thumbnail.URL
				}
				_ = ch.UpsertDesign(cache.Design{
					ID: d.ID, Title: d.Title, UpdatedAt: d.UpdatedAt,
					FetchedAt:    time.Now().Unix(),
					ThumbnailURL: thumb, RawJSON: string(raw),
				})
				row := map[string]any{
					"id": d.ID, "title": d.Title, "updated_at": d.UpdatedAt,
				}
				if flagFields == "all" {
					_ = json.Unmarshal(raw, &row)
				} else if flagFields != "" {
					row = output.ProjectFields(row, flagFields)
				}
				count++
				return output.EmitJSON(os.Stdout, row)
			})
			return err
		},
	}
	cmd.Flags().StringVar(&flagFields, "fields", "id,title,updated_at", "comma-separated field list or 'all'")
	cmd.Flags().IntVar(&flagLimit, "limit", 20, "max number of designs to emit")
	return cmd
}
