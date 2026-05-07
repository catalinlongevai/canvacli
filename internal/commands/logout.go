package commands

import "github.com/spf13/cobra"

func NewLogout() *cobra.Command {
	return &cobra.Command{Use: "logout", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
