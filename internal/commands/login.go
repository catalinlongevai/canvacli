package commands

import "github.com/spf13/cobra"

func NewLogin() *cobra.Command {
	return &cobra.Command{Use: "login", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
