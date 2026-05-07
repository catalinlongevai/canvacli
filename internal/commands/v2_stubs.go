package commands

import (
	"errors"

	"github.com/spf13/cobra"
)

// newV2Stub returns a placeholder cobra.Command for v2 surface area that is
// declared in the foundation phase (3a) but implemented by per-resource
// agents in Phase 3b. The error message is matched on intentionally — if you
// see it from a release binary, Phase 3b for that command did not land.
func newV2Stub(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(*cobra.Command, []string) error {
			return errors.New("not implemented in foundation phase — Phase 3b lands real impl")
		},
	}
}

