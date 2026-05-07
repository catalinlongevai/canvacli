package commands

import (
	"errors"
	"os"

	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewSQL() *cobra.Command {
	var flagLimit int
	var flagSchema bool
	cmd := &cobra.Command{
		Use:   "sql [query]",
		Short: "Read-only SQL against the local cache",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()
			if flagSchema {
				return output.EmitJSON(os.Stdout, map[string]any{
					"tables": map[string][]string{
						"designs":     {"id", "title", "folder_id", "updated_at", "fetched_at", "thumbnail_url", "raw_json"},
						"templates":   {"id", "title", "fetched_at", "raw_json"},
						"folders":     {"id", "name", "parent_id", "fetched_at"},
						"idempotency": {"key", "command", "args_hash", "result_json", "created_at"},
					},
				})
			}
			if len(args) == 0 {
				return errors.New("provide a SQL query or --schema")
			}
			rows, err := ch.ExecReadOnly(args[0], flagLimit)
			if err != nil {
				return err
			}
			return output.EmitNDJSON(os.Stdout, rows)
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 500, "max rows to return (cap 10000)")
	cmd.Flags().BoolVar(&flagSchema, "schema", false, "print cache table schema")
	return cmd
}
