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
	// Implementation in handlers_sync_search.go (handleSync).
	s.mcp.AddTool(
		mcp.NewTool("canva_sync",
			mcp.WithDescription("Mirror the user's Canva account (designs, folders, templates, plus locally-known comments and assets) into the local SQLite cache. Returns a per-resource summary (count + duration). Comments and assets are upload-history-only — see spec §4.1."),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleSync,
	)

	// canva_search — read-only FTS5 query.
	// Implementation in handlers_sync_search.go (handleSearch).
	s.mcp.AddTool(
		mcp.NewTool("canva_search",
			mcp.WithDescription("FTS5 search across mirrored designs, templates, comment threads, comment replies, and assets. FTS5 query syntax: 'q3 banner' (AND), 'launch*' (prefix), '\"q3 banner\"' (phrase), 'banner OR poster' (boolean), 'launch NOT draft' (negation)."),
			mcp.WithString("query", mcp.Required(), mcp.Description("FTS5 MATCH query string")),
			mcp.WithString("type", mcp.Description("Restrict to one source: design | template | comment_thread | comment_reply | asset")),
			mcp.WithNumber("limit", mcp.Description("Max rows (1-1000, default 50)")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleSearch,
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
	// Routes by extension (spec §8): images/videos → /v1/asset-uploads,
	// documents → /v1/imports.
	s.mcp.AddTool(
		mcp.NewTool("canva_import",
			mcp.WithDescription("Import a local PDF/PPTX/DOCX/etc as a new Canva design. Image and video files are routed to the asset library instead. Pass an absolute file path."),
			mcp.WithString("file", mcp.Required(), mcp.Description("Absolute path to the local file to import")),
			mcp.WithString("mime_type", mcp.Description("Override mime sniffing for the imports endpoint")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleImport,
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
			mcp.WithDescription("Upload a local file (image/video) to the user's Canva asset library. Returns the asset_id which can be passed directly to canva_create's autofill data as DatasetImageValue.asset_id (spec §10)."),
			mcp.WithString("file", mcp.Required(), mcp.Description("Absolute path to the local file to upload")),
			mcp.WithString("name", mcp.Description("Display name for the asset (default: filename)")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleAssetsUpload,
	)

	// canva_comments_add — posts a new top-level comment OR a reply when
	// `reply_to` is set. Irreversible (no edit/delete API) — destructiveHint
	// remains false because we don't destroy existing data, but the
	// description calls out the irreversibility per spec §4.6.
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_add",
			mcp.WithDescription("Post a comment on a Canva design. Without reply_to: creates a new top-level thread. With reply_to: posts a reply on that thread. Irreversible — the Canva API has no edit/delete endpoint."),
			mcp.WithString("design_id_or_name", mcp.Required(), mcp.Description("Design ID or exact title")),
			mcp.WithString("text", mcp.Required(), mcp.Description("Comment body (1-2048 chars)")),
			mcp.WithString("reply_to", mcp.Description("Existing thread ID. If set, posts as a reply on that thread instead of creating a new one.")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleCommentsAdd,
	)

	// canva_comments_reply — convenience tool with reply_to required.
	// Functionally a strict subset of canva_comments_add; kept for clarity
	// per spec §4.6 (both names appear in the contract).
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_reply",
			mcp.WithDescription("Reply to an existing Canva comment thread. Equivalent to canva_comments_add with reply_to set."),
			mcp.WithString("design_id_or_name", mcp.Required(), mcp.Description("Design ID or exact title (the design that hosts the thread)")),
			mcp.WithString("thread_id", mcp.Required(), mcp.Description("Comment thread ID")),
			mcp.WithString("text", mcp.Required(), mcp.Description("Reply body (1-2048 chars)")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleCommentsReply,
	)

	// canva_comments_thread — read-only fetch of a thread + its replies.
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_thread",
			mcp.WithDescription("Fetch a Canva comment thread and its replies. Refreshes the local cache. The thread must be locally cached (so its design ID is known) OR pass design_id_or_name."),
			mcp.WithString("thread_id", mcp.Required(), mcp.Description("Comment thread ID")),
			mcp.WithString("design_id_or_name", mcp.Description("Override the cached design-id lookup (required when the thread isn't yet cached)")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleCommentsThread,
	)

	// canva_comments_archive — re-walk every locally-known thread on a
	// design (or across all designs when omitted). Read-only against Canva,
	// refreshes the local cache.
	s.mcp.AddTool(
		mcp.NewTool("canva_comments_archive",
			mcp.WithDescription("Archive locally-known comment threads. Re-fetches each thread + replies fresh from the Canva API and returns them as JSON. NOTE: only threads canvacli has interacted with are visible — the Canva Connect API does not expose a list-threads endpoint. Threads created in the Canva web UI must first be pulled in via canva_comments_thread."),
			mcp.WithString("design_id_or_name", mcp.Description("Limit archival to one design's threads")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleCommentsArchive,
	)

	// canva_create — was skipped in v1.1 because creating a design is a
	// write that downstream tools may not be able to undo cleanly.
	// destructiveHint=true per the v1.1 fix pattern.
	// Implementation in handlers_sync_search.go (handleCreate).
	s.mcp.AddTool(
		mcp.NewTool("canva_create",
			mcp.WithDescription("Create a new Canva design from a brand template + autofill JSON (Enterprise). The autofill argument MUST be inline JSON — no file path support over MCP. Pair with canva_assets_upload to source images programmatically (spec §10)."),
			mcp.WithString("template", mcp.Required(), mcp.Description("Template ID or exact name")),
			mcp.WithString("autofill", mcp.Required(), mcp.Description("Inline JSON autofill data (the shape returned by canva_templates_show)")),
			mcp.WithString("title", mcp.Description("Title for the new design")),
			mcp.WithReadOnlyHintAnnotation(false),
			mcp.WithDestructiveHintAnnotation(true),
		),
		s.handleCreate,
	)

	// canva_templates_list — read-only listing of brand templates.
	// Implementation in handlers_sync_search.go (handleTemplatesList).
	s.mcp.AddTool(
		mcp.NewTool("canva_templates_list",
			mcp.WithDescription("List the user's brand templates (Enterprise). Each list pass also refreshes the local templates cache used by name resolution."),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleTemplatesList,
	)

	// canva_templates_show — read-only fetch of a template's autofill dataset.
	// Implementation in handlers_sync_search.go (handleTemplatesShow).
	s.mcp.AddTool(
		mcp.NewTool("canva_templates_show",
			mcp.WithDescription("Show a brand template's autofill dataset — the JSON shape that canva_create's autofill argument must match."),
			mcp.WithString("template_id_or_name", mcp.Required(), mcp.Description("Template ID or exact name")),
			mcp.WithReadOnlyHintAnnotation(true),
			mcp.WithDestructiveHintAnnotation(false),
		),
		s.handleTemplatesShow,
	)
}
