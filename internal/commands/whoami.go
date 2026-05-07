package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewWhoami() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := loadClient(ctx)
			if err != nil {
				return err
			}
			u, err := c.MeWithProfile(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, `{"id":%q,"team_id":%q,"display_name":%q}`+"\n",
				u.ID, u.TeamID, u.DisplayName)
			return nil
		},
	}
}
