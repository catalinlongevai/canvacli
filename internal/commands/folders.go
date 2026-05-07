package commands

import (
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewFolders() *cobra.Command {
	return &cobra.Command{
		Use:   "folders",
		Short: "List folders (walks 'root' and 'uploads')",
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
			return cl.WalkFolders(ctx, func(f api.Folder, parent string) error {
				_ = ch.UpsertFolder(cache.Folder{
					ID: f.ID, Name: f.Name, ParentID: parent,
					FetchedAt: time.Now().Unix(),
				})
				return output.EmitJSON(os.Stdout, map[string]any{
					"id": f.ID, "name": f.Name, "parent_id": parent,
				})
			})
		},
	}
}
