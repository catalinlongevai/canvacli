package commands

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/catalinlongevai/canvacli/internal/mcp"
)

// NewMCP returns the `canva mcp` command group, currently containing only
// `canva mcp serve`, which runs an MCP (Model Context Protocol) server over
// stdio for use by Claude Desktop, Cursor, and other MCP-capable agents.
func NewMCP() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP server integration for AI agents",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run an MCP server over stdio (use from Claude Desktop / Cursor / etc.)",
		Long: `Start an MCP (Model Context Protocol) server that exposes canvacli's
core capabilities as native MCP tools. Configure your MCP-capable client
(Claude Desktop, Cursor, etc.) with:

  {"mcpServers": {"canva": {"command": "canva", "args": ["mcp", "serve"]}}}

Run 'canva login' from the terminal first to authenticate. The MCP server
reads the same token store as the CLI.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := context.Background()
			cl, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()
			s := mcp.NewServer(cl, ch)
			return s.Serve(ctx)
		},
	})
	return cmd
}
