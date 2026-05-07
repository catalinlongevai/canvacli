package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
	"github.com/spf13/cobra"
)

// NewResize creates a copy of a design at a different size. Replaces the
// foundation-phase stub.
//
// Per Canva's OpenAPI the only Canva-defined preset names are doc, email,
// presentation, whiteboard. Anything else (instagram_post, a4_document, …)
// would have to go through the custom width/height branch — we don't
// expose that here yet because the spec only requires the four presets.
func NewResize() *cobra.Command {
	var flagTo string
	cmd := &cobra.Command{
		Use:   "resize <name|id> --to <preset>",
		Short: "Resize a design to a preset (creates a new design)",
		Long: `Resize a Canva design. Always creates a NEW design — the original is untouched.

Allowed --to values (Canva-defined presets): doc, email, presentation, whiteboard.

Requires Canva Pro or an active resize trial. Free-tier users get a small
trial quota; the response surfaces remaining uses + an upgrade URL.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagTo == "" {
				return errors.New("--to is required (one of: doc, email, presentation, whiteboard)")
			}
			if !api.IsValidResizePreset(flagTo) {
				return fmt.Errorf("invalid --to %q (allowed: doc, email, presentation, whiteboard)", flagTo)
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
			res, err := cl.ResizeDesign(ctx, api.ResizeRequest{
				DesignID: id,
				Preset:   api.ResizePreset(flagTo),
			})
			if err != nil {
				return err
			}
			out := map[string]any{
				"id":             res.Design.ID,
				"title":          res.Design.Title,
				"url":            res.Design.URL,
				"source_id":      id,
				"preset":         flagTo,
			}
			if res.TrialInformation != nil {
				out["trial"] = map[string]any{
					"uses_remaining": res.TrialInformation.UsesRemaining,
					"upgrade_url":    res.TrialInformation.UpgradeURL,
				}
			}
			return output.EmitJSON(os.Stdout, out)
		},
	}
	cmd.Flags().StringVar(&flagTo, "to", "", "preset: doc|email|presentation|whiteboard (required)")
	return cmd
}
