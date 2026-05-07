package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/catalinlongevai/canvacli/internal/config"
	"github.com/spf13/cobra"
)

func NewLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials and clear cache",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tokPath, err := config.TokenPath()
			if err != nil {
				return err
			}
			cachePath, err := config.CacheDBPath()
			if err != nil {
				return err
			}
			for _, p := range []string{tokPath, cachePath} {
				if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove %s: %w", p, err)
				}
			}
			fmt.Fprintln(os.Stderr, "Logged out. Local credentials and cache cleared.")
			return nil
		},
	}
}
