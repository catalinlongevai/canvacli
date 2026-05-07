package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
	"github.com/spf13/cobra"
)

func NewExport() *cobra.Command {
	var flagFormat, flagOutput, flagPages string
	var flagURLOnly bool
	cmd := &cobra.Command{
		Use:   "export <name|id>",
		Short: "Export a design as PDF/PNG/JPG/MP4/GIF",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagFormat == "" {
				return errors.New("--format is required")
			}
			pages, err := parsePagesFlag(flagPages)
			if err != nil {
				return err
			}
			if len(pages) > 0 {
				switch flagFormat {
				case "png", "jpg", "pdf", "gif":
					// supported
				default:
					return fmt.Errorf("--pages is only supported for png, jpg, pdf, gif (got %q)", flagFormat)
				}
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
				Format:   api.ExportFormat{Type: flagFormat, Pages: pages},
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
	cmd.Flags().StringVar(&flagPages, "pages", "", "comma-separated 1-based page indices (e.g. 1,3,5); png/jpg/pdf/gif only")
	return cmd
}

// parsePagesFlag parses the value of --pages into a sorted slice of
// 1-based page indices. Returns nil for an empty flag (export all pages).
// Rejects non-positive numbers and non-numeric tokens with a clear error.
func parsePagesFlag(s string) ([]int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil, fmt.Errorf("--pages: %q is not a number", p)
		}
		if n < 1 {
			return nil, fmt.Errorf("--pages: page indices must be 1-based positive integers (got %d)", n)
		}
		out = append(out, n)
	}
	if len(out) == 0 {
		return nil, errors.New("--pages: no page indices found")
	}
	return out, nil
}
