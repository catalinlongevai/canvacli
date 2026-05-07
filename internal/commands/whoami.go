package commands

import "github.com/spf13/cobra"

func NewWhoami() *cobra.Command {
	return &cobra.Command{Use: "whoami", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }}
}
