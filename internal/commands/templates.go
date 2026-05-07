package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewTemplates() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "List brand templates (Enterprise-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()
			return c.ListTemplates(ctx, func(t api.BrandTemplate) error {
				raw, _ := json.Marshal(t)
				_ = ch.UpsertTemplate(cache.Template{
					ID:        t.ID,
					Title:     t.Title,
					FetchedAt: time.Now().Unix(),
					RawJSON:   string(raw),
				})
				return output.EmitJSON(os.Stdout, map[string]any{
					"id": t.ID, "title": t.Title, "updated_at": t.UpdatedAt,
				})
			})
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show <name|id>",
		Short: "Show autofill dataset for a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, _ := loadCache()
			if ch != nil {
				defer ch.Close()
			}
			id := args[0]
			if ch != nil {
				if t, _ := ch.FindTemplateByID(id); t != nil {
					id = t.ID
				} else {
					if matches, _ := ch.FindTemplateByName(id); len(matches) == 1 {
						id = matches[0].ID
					} else if len(matches) > 1 {
						return fmt.Errorf("multiple templates named %q", id)
					}
				}
			}
			ds, err := c.GetTemplateDataset(ctx, id)
			if err != nil {
				return err
			}
			return output.EmitJSON(os.Stdout, ds)
		},
	})
	return cmd
}
