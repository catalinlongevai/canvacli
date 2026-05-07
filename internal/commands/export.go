package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
	"github.com/spf13/cobra"
)

func NewExport() *cobra.Command {
	var flagFormat, flagOutput string
	var flagURLOnly bool
	cmd := &cobra.Command{
		Use:   "export <name|id>",
		Short: "Export a design as PDF/PNG/JPG/MP4/GIF",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagFormat == "" {
				return errors.New("--format is required")
			}
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
			res, err := cl.CreateExport(ctx, api.ExportRequest{
				DesignID: id,
				Format:   api.ExportFormat{Type: flagFormat},
			})
			if err != nil {
				return err
			}
			if len(res.URLs) == 0 {
				return errors.New("export returned no URLs")
			}
			if flagURLOnly {
				return output.EmitJSON(os.Stdout, map[string]any{
					"id": id, "format": flagFormat, "urls": res.URLs,
					"warning": "URLs expire in 24 hours",
				})
			}
			outPath := flagOutput
			if outPath == "" {
				outPath = fmt.Sprintf("%s.%s", id, flagFormat)
			}
			files := []string{}
			for i, u := range res.URLs {
				p := outPath
				if len(res.URLs) > 1 {
					ext := filepath.Ext(outPath)
					base := outPath[:len(outPath)-len(ext)]
					p = fmt.Sprintf("%s_%d%s", base, i+1, ext)
				}
				if err := cl.DownloadTo(ctx, u, p); err != nil {
					return err
				}
				files = append(files, p)
			}
			return output.EmitJSON(os.Stdout, map[string]any{
				"id": id, "format": flagFormat, "files": files,
			})
		},
	}
	cmd.Flags().StringVar(&flagFormat, "format", "", "pdf|png|jpg|mp4|gif (required)")
	cmd.Flags().StringVar(&flagOutput, "output", "", "output path (default: <design-id>.<format>)")
	cmd.Flags().BoolVar(&flagURLOnly, "url-only", false, "return URL instead of downloading (24h expiry)")
	return cmd
}
