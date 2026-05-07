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

// NewSync is the v2 "all-in-one" mirror command.
// Phase 3b (sync agent) replaces this stub.
func NewSync() *cobra.Command {
	return newV2Stub("sync", "Mirror your Canva account into local SQLite (all-in-one)")
}

// NewSearch is the FTS5-backed search command.
// Phase 3b (search agent) replaces this stub.
func NewSearch() *cobra.Command {
	return newV2Stub("search [query]", "FTS5 search across mirrored designs, comments, assets, templates")
}

// NewPages lists the pages of a design.
// Phase 3b (pages agent) replaces this stub.
func NewPages() *cobra.Command {
	return newV2Stub("pages <design>", "List pages of a design")
}

// NewImport imports a local file as a Canva design.
// Phase 3b (import agent) replaces this stub.
func NewImport() *cobra.Command {
	return newV2Stub("import <file>", "Import a PDF/PPTX/DOCX/image as a Canva design")
}

// NewResize creates a new design at a different preset/size.
// Phase 3b (resize agent) replaces this stub.
func NewResize() *cobra.Command {
	return newV2Stub("resize <design>", "Resize a design (creates a new design)")
}

// NewAssets is the parent command for asset-library operations.
// Phase 3b (assets agent) replaces this stub with a real subcommand tree.
func NewAssets() *cobra.Command {
	return newV2Stub("assets", "Asset library operations")
}

// NewComments is the parent command for comment-thread operations.
// Phase 3b (comments agent) replaces this stub with a real subcommand tree.
func NewComments() *cobra.Command {
	return newV2Stub("comments", "Comment thread operations")
}
