package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
	"github.com/spf13/cobra"
)

func NewCreate() *cobra.Command {
	var (
		flagTemplate, flagAutofill, flagFolder, flagTitle, flagIdempotency string
		flagDryRun                                                          bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a design from a brand template + autofill data",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flagTemplate == "" || flagAutofill == "" {
				return errors.New("--template and --autofill are required")
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

			tplID, err := resolver.New(ch, cl).ResolveTemplate(flagTemplate)
			if err != nil {
				return err
			}

			var data map[string]any
			var raw []byte
			if flagAutofill == "-" {
				raw, err = io.ReadAll(os.Stdin)
			} else {
				raw, err = os.ReadFile(flagAutofill)
			}
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &data); err != nil {
				return fmt.Errorf("autofill JSON: %w", err)
			}

			argsHash := sha256hex(tplID + ":" + flagTitle + ":" + string(raw))
			if flagIdempotency != "" {
				if prior, _ := ch.LookupIdempotency(flagIdempotency, argsHash); prior != "" {
					_, _ = fmt.Fprintln(os.Stdout, prior)
					return nil
				}
			}

			req := api.AutofillRequest{
				BrandTemplateID: tplID,
				Data:            data,
				Title:           flagTitle,
			}
			if flagDryRun {
				return output.EmitJSON(os.Stdout, map[string]any{
					"dry_run": true,
					"method":  "POST",
					"path":    "/autofills",
					"body":    req,
				})
			}

			res, err := cl.CreateAutofill(ctx, req)
			if err != nil {
				return err
			}
			out := map[string]any{
				"id":    res.Design.ID,
				"url":   res.Design.URL,
				"title": res.Design.Title,
			}
			outJSON, _ := json.Marshal(out)
			if flagIdempotency != "" {
				_ = ch.SaveIdempotency(flagIdempotency, "create", argsHash, string(outJSON))
			}
			return output.EmitJSON(os.Stdout, out)
		},
	}
	cmd.Flags().StringVar(&flagTemplate, "template", "", "brand template name or ID (required)")
	cmd.Flags().StringVar(&flagAutofill, "autofill", "", "path to JSON file or '-' for stdin (required)")
	cmd.Flags().StringVar(&flagFolder, "folder", "", "destination folder name or ID")
	cmd.Flags().StringVar(&flagTitle, "title", "", "design title")
	cmd.Flags().StringVar(&flagIdempotency, "idempotency-key", "", "client-side dedupe key")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "print the API call without executing")
	return cmd
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
