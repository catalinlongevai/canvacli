// Package mcp exposes a subset of canvacli's capabilities as MCP tools over
// stdio so MCP-capable clients (Claude Desktop, Cursor, etc.) can invoke them
// natively without shelling out to the canva binary.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/resolver"
)

// Server wraps an MCP server with canvacli's API client and cache.
type Server struct {
	api   *api.Client
	cache *cache.Cache
	mcp   *server.MCPServer
}

// NewServer constructs an MCP server that exposes canvacli's tools using the
// supplied API client and cache. Callers must keep the cache open for the
// lifetime of the server.
func NewServer(apiClient *api.Client, c *cache.Cache) *Server {
	s := &Server{api: apiClient, cache: c}
	s.mcp = server.NewMCPServer(
		"canvacli",
		"1.1.0",
		server.WithToolCapabilities(false),
	)
	s.registerTools()
	return s
}

// Serve runs the MCP server over stdio. Blocks until stdin is closed.
func (s *Server) Serve(ctx context.Context) error {
	return server.ServeStdio(s.mcp)
}

func (s *Server) registerTools() {
	// canva_whoami
	s.mcp.AddTool(
		mcp.NewTool("canva_whoami",
			mcp.WithDescription("Return the authenticated Canva user (id, team_id, display_name)."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			u, err := s.api.MeWithProfile(ctx)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			b, _ := json.Marshal(u)
			return mcp.NewToolResultText(string(b)), nil
		},
	)

	// canva_list
	s.mcp.AddTool(
		mcp.NewTool("canva_list",
			mcp.WithDescription("List the user's Canva designs as JSON. Returns id, title, updated_at by default."),
			mcp.WithNumber("limit", mcp.Description("Max designs to return (1-100, default 20)")),
			mcp.WithString("fields", mcp.Description("Comma-separated field list, or 'all'. Default: id,title,updated_at")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			limit := 20
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = int(v)
				if limit > 100 {
					limit = 100
				}
			}
			out := []map[string]any{}
			count := 0
			err := s.api.ListDesigns(ctx, func(d api.Design) error {
				if count >= limit {
					return nil
				}
				row := map[string]any{
					"id":         d.ID,
					"title":      d.Title,
					"updated_at": d.UpdatedAt,
				}
				out = append(out, row)
				count++
				return nil
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			b, _ := json.Marshal(out)
			return mcp.NewToolResultText(string(b)), nil
		},
	)

	// canva_folders
	s.mcp.AddTool(
		mcp.NewTool("canva_folders",
			mcp.WithDescription("List folders in the user's Canva account by walking 'root' and 'uploads'."),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			out := []map[string]any{}
			err := s.api.WalkFolders(ctx, func(f api.Folder, parent string) error {
				out = append(out, map[string]any{
					"id":        f.ID,
					"name":      f.Name,
					"parent_id": parent,
				})
				return nil
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			b, _ := json.Marshal(out)
			return mcp.NewToolResultText(string(b)), nil
		},
	)

	// canva_export
	s.mcp.AddTool(
		mcp.NewTool("canva_export",
			mcp.WithDescription("Export a Canva design as PDF/PNG/JPG/MP4/GIF and download to disk."),
			mcp.WithString("design_id_or_name", mcp.Required(), mcp.Description("Design ID or exact title")),
			mcp.WithString("format", mcp.Required(), mcp.Description("pdf|png|jpg|mp4|gif")),
			mcp.WithString("output_path", mcp.Description("Output file path. Default: <design_id>.<format>")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			query, _ := args["design_id_or_name"].(string)
			format, _ := args["format"].(string)
			outPath, _ := args["output_path"].(string)
			if query == "" || format == "" {
				return mcp.NewToolResultError("design_id_or_name and format are required"), nil
			}
			r := resolver.New(s.cache, s.api)
			id, err := r.ResolveDesign(query)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			res, err := s.api.CreateExport(ctx, api.ExportRequest{
				DesignID: id,
				Format:   api.ExportFormat{Type: format},
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			if len(res.URLs) == 0 {
				return mcp.NewToolResultError("export returned no URLs"), nil
			}
			if outPath == "" {
				outPath = fmt.Sprintf("%s.%s", id, format)
			}
			if err := s.api.DownloadTo(ctx, res.URLs[0], outPath); err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			b, _ := json.Marshal(map[string]any{
				"id":     id,
				"format": format,
				"file":   outPath,
			})
			return mcp.NewToolResultText(string(b)), nil
		},
	)

	// canva_sql
	s.mcp.AddTool(
		mcp.NewTool("canva_sql",
			mcp.WithDescription("Execute a read-only SQL query against the local cache (designs, templates, folders, idempotency tables). SELECT and WITH...SELECT only."),
			mcp.WithString("query", mcp.Required(), mcp.Description("A single SELECT or WITH...SELECT statement")),
			mcp.WithNumber("limit", mcp.Description("Max rows (1-10000, default 500)")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			query, _ := args["query"].(string)
			if query == "" {
				return mcp.NewToolResultError("query is required"), nil
			}
			limit := 500
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = int(v)
			}
			rows, err := s.cache.ExecReadOnly(query, limit)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			b, _ := json.Marshal(rows)
			return mcp.NewToolResultText(string(b)), nil
		},
	)

	// canva_schema
	s.mcp.AddTool(
		mcp.NewTool("canva_schema",
			mcp.WithDescription("Return the canvacli command schema as JSON for further introspection."),
			mcp.WithString("mode", mcp.Description("compact (default) or full")),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			mode, _ := req.GetArguments()["mode"].(string)
			schema := schemaCompact
			if mode == "full" {
				schema = schemaFull
			}
			return mcp.NewToolResultText(schema), nil
		},
	)
}
