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
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
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
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
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
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
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
			mcp.WithDestructiveHintAnnotation(false),
			mcp.WithIdempotentHintAnnotation(true),
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
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
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
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
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

	s.registerV2Stubs()
}

// notImplementedHandler returns a stub MCP handler for v2 tools registered
// during the foundation phase (3a). Phase 3b agents replace each registration
// site with the real implementation that calls api/cache/etc.
func notImplementedHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultError("not implemented in foundation phase"), nil
}

// registerV2Stubs registers placeholder MCP tools for the v2 surface area.
// All handlers return "not implemented in foundation phase". Phase 3b agents
// replace each tool's registration with the real implementation.
//
// Annotations follow the v1.1 fix pattern: every tool sets
// destructiveHint explicitly (true only for canva_create per the v1.1 fix).
// readOnlyHint is set true for tools that only read state.
func (s *Server) registerV2Stubs() {
	// canva_sync — mutates the local cache, not Canva state. Not destructive.
	s.mcp.AddTool(
		mcp.NewTool("canva_sync",
			mcp.WithDescription("Mirror the user's Canva account (designs, templates, folders, assets, comments) into the local SQLite cache."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_search — read-only FTS5 query.
	s.mcp.AddTool(
		mcp.NewTool("canva_search",
			mcp.WithDescription("FTS5 search across mirrored designs, comments, assets, and templates."),
			mcp.WithString("query", mcp.Required(), mcp.Description("FTS5 query string")),
			mcp.WithString("scope", mcp.Description("designs|templates|comments|assets|all (default all)")),
			mcp.WithNumber("limit", mcp.Description("Max rows (1-1000, default 50)")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_pages — read-only listing of design pages.
	s.mcp.AddTool(
		mcp.NewTool("canva_pages",
			mcp.WithDescription("List the pages of a Canva design with thumbnails and dimensions. Backed by Canva's preview /designs/{id}/pages endpoint."),
			mcp.WithString("design_id_or_name", mcp.Required(), mcp.Description("Design ID or exact title")),
			mcp.WithNumber("limit", mcp.Description("Max pages to return (1-200, default unlimited)")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			query, _ := args["design_id_or_name"].(string)
			if query == "" {
				return mcp.NewToolResultError("design_id_or_name is required"), nil
			}
			limit := 0
			if v, ok := args["limit"].(float64); ok && v > 0 {
				limit = int(v)
			}
			r := resolver.New(s.cache, s.api)
			id, err := r.ResolveDesign(query)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			out := []map[string]any{}
			count := 0
			err = s.api.ListAllPages(ctx, id, func(p api.Page) error {
				if limit > 0 && count >= limit {
					return nil
				}
				row := map[string]any{
					"design_id": id,
					"index":     p.Index,
				}
				if p.Dimensions != nil {
					row["width"] = int(p.Dimensions.Width)
					row["height"] = int(p.Dimensions.Height)
				}
				if p.Thumbnail != nil {
					row["thumbnail_url"] = p.Thumbnail.URL
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

	// canva_import — creates a new Canva design from a local file. Not destructive (new resource).
	s.mcp.AddTool(
		mcp.NewTool("canva_import",
			mcp.WithDescription("Import a local PDF/PPTX/DOCX/image as a new Canva design."),
			mcp.WithString("file", mcp.Required(), mcp.Description("Path to the local file to import")),
			mcp.WithString("title", mcp.Description("Title for the new design")),
			mcp.WithString("folder", mcp.Description("Target folder ID")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_resize — creates a new design at a different size. Not destructive (new resource).
	s.mcp.AddTool(
		mcp.NewTool("canva_resize",
			mcp.WithDescription("Resize a Canva design to one of the four Canva presets (doc, email, presentation, whiteboard). Creates a new design; original is untouched. Requires Canva Pro or an active resize trial."),
			mcp.WithString("design_id_or_name", mcp.Required(), mcp.Description("Source design ID or exact title")),
			mcp.WithString("to", mcp.Required(), mcp.Description("Target preset: doc | email | presentation | whiteboard"), mcp.Enum("doc", "email", "presentation", "whiteboard")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			args := req.GetArguments()
			query, _ := args["design_id_or_name"].(string)
			to, _ := args["to"].(string)
			if query == "" || to == "" {
				return mcp.NewToolResultError("design_id_or_name and to are required"), nil
			}
			if !api.IsValidResizePreset(to) {
				return mcp.NewToolResultError("invalid 'to' (allowed: doc, email, presentation, whiteboard)"), nil
			}
			r := resolver.New(s.cache, s.api)
			id, err := r.ResolveDesign(query)
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			res, err := s.api.ResizeDesign(ctx, api.ResizeRequest{
				DesignID: id,
				Preset:   api.ResizePreset(to),
			})
			if err != nil {
				return mcp.NewToolResultError(err.Error()), nil
			}
			out := map[string]any{
				"id":        res.Design.ID,
				"title":     res.Design.Title,
				"url":       res.Design.URL,
				"source_id": id,
				"preset":    to,
			}
			if res.TrialInformation != nil {
				out["trial"] = map[string]any{
					"uses_remaining": res.TrialInformation.UsesRemaining,
					"upgrade_url":    res.TrialInformation.UpgradeURL,
				}
			}
			b, _ := json.Marshal(out)
			return mcp.NewToolResultText(string(b)), nil
		},
	)

	// canva_assets_upload — uploads a new asset. Not destructive.
	s.mcp.AddTool(
		mcp.NewTool("canva_assets_upload",
			mcp.WithDescription("Upload a local file (image/video/audio) to the user's Canva asset library."),
			mcp.WithString("file", mcp.Required(), mcp.Description("Path to the local file to upload")),
			mcp.WithString("name", mcp.Description("Display name for the asset (default: filename)")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_comments_add — posts a new top-level comment. Not destructive.
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_add",
			mcp.WithDescription("Post a top-level comment on a Canva design (creates a new thread)."),
			mcp.WithString("design_id_or_name", mcp.Required(), mcp.Description("Design ID or exact title")),
			mcp.WithString("text", mcp.Required(), mcp.Description("Comment body")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_comments_reply — posts a reply on a thread. Not destructive.
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_reply",
			mcp.WithDescription("Reply to an existing Canva comment thread."),
			mcp.WithString("thread_id", mcp.Required(), mcp.Description("Comment thread ID")),
			mcp.WithString("text", mcp.Required(), mcp.Description("Reply body")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_comments_thread — read-only fetch of a thread + its replies.
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_thread",
			mcp.WithDescription("Fetch a comment thread and its replies."),
			mcp.WithString("thread_id", mcp.Required(), mcp.Description("Comment thread ID")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_comments_archive — local-cache archival walk. Read-only against Canva.
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_archive",
			mcp.WithDescription("Archive locally cached comment threads (drops them from the local cache)."),
			mcp.WithString("design_id_or_name", mcp.Description("Limit archival to one design's threads")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_create — was skipped in v1.1 because creating a design is a
	// write that downstream tools may not be able to undo cleanly.
	// destructiveHint=true per the v1.1 fix pattern.
	s.mcp.AddTool(
		mcp.NewTool("canva_create",
			mcp.WithDescription("Create a new Canva design from a brand template + autofill JSON (Enterprise)."),
			mcp.WithString("template", mcp.Required(), mcp.Description("Template ID or exact name")),
			mcp.WithString("autofill", mcp.Required(), mcp.Description("Path to JSON file with autofill data, or inline JSON")),
			mcp.WithString("title", mcp.Description("Title for the new design")),
			mcp.WithString("folder", mcp.Description("Target folder ID")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
		),
		notImplementedHandler,
	)

	// canva_templates_list — read-only listing of brand templates.
	s.mcp.AddTool(
		mcp.NewTool("canva_templates_list",
			mcp.WithDescription("List the user's brand templates (Enterprise)."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)

	// canva_templates_show — read-only fetch of a template's autofill dataset.
	s.mcp.AddTool(
		mcp.NewTool("canva_templates_show",
			mcp.WithDescription("Show a brand template's autofill dataset (the JSON shape for canva_create)."),
			mcp.WithString("template_id_or_name", mcp.Required(), mcp.Description("Template ID or exact name")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		notImplementedHandler,
	)
}
