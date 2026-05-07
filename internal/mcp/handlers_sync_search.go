// MCP handlers for the sync + search + templates + create tools owned by
// the Pattern A core agent (sync.go, search.go in commands/).
//
// canva_sync runs the all-in-one mirror; canva_search hits the FTS5 index;
// canva_create posts an autofill job; canva_templates_list lists brand
// templates; canva_templates_show fetches a template's autofill dataset.
//
// Each handler mirrors the equivalent CLI command's logic but reshapes the
// output for an MCP client (single JSON object string, no NDJSON streaming).
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/resolver"
)

// handleSync runs all five resource walkers in turn and returns the
// per-resource summary as JSON. Mirrors `canva sync` (commands/sync.go)
// but collects results in memory rather than streaming NDJSON.
func (s *Server) handleSync(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	type result struct {
		Resource string `json:"resource"`
		OK       bool   `json:"ok"`
		Count    int    `json:"count"`
		Error    string `json:"error,omitempty"`
		TookMS   int64  `json:"took_ms"`
	}
	runOne := func(name string, fn func(context.Context, *api.Client, *cache.Cache) (int, error)) result {
		start := time.Now()
		count, err := fn(ctx, s.api, s.cache)
		r := result{Resource: name, OK: err == nil, Count: count, TookMS: time.Since(start).Milliseconds()}
		if err != nil {
			r.Error = err.Error()
		}
		return r
	}
	results := []result{
		runOne("folders", mcpSyncFolders),
		runOne("designs", mcpSyncDesigns),
		runOne("templates", mcpSyncTemplates),
		runOne("assets", mcpSyncAssets),
		runOne("comments", mcpSyncComments),
	}
	b, _ := json.Marshal(map[string]any{"results": results})
	return mcp.NewToolResultText(string(b)), nil
}

// mcpSync* mirrors commands/sync.go's per-resource helpers. They are kept
// in the mcp package to avoid leaking command-internals into the public
// API; if drift becomes a maintenance burden, both should move to a shared
// `internal/sync` package.
func mcpSyncDesigns(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	count := 0
	now := time.Now().Unix()
	err := cl.ListDesigns(ctx, func(d api.Design) error {
		raw, _ := json.Marshal(d)
		thumb := ""
		if d.Thumbnail != nil {
			thumb = d.Thumbnail.URL
		}
		if upErr := ch.UpsertDesign(cache.Design{
			ID: d.ID, Title: d.Title, UpdatedAt: d.UpdatedAt,
			FetchedAt: now, ThumbnailURL: thumb, RawJSON: string(raw),
		}); upErr != nil {
			return upErr
		}
		count++
		return nil
	})
	return count, err
}

func mcpSyncFolders(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	count := 0
	now := time.Now().Unix()
	err := cl.WalkFolders(ctx, func(f api.Folder, parent string) error {
		if upErr := ch.UpsertFolder(cache.Folder{
			ID: f.ID, Name: f.Name, ParentID: parent, FetchedAt: now,
		}); upErr != nil {
			return upErr
		}
		count++
		return nil
	})
	return count, err
}

func mcpSyncTemplates(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	count := 0
	now := time.Now().Unix()
	err := cl.ListTemplates(ctx, func(t api.BrandTemplate) error {
		raw, _ := json.Marshal(t)
		if upErr := ch.UpsertTemplate(cache.Template{
			ID: t.ID, Title: t.Title, FetchedAt: now, RawJSON: string(raw),
		}); upErr != nil {
			return upErr
		}
		count++
		return nil
	})
	if err != nil {
		if apiErr, ok := err.(*api.APIError); ok && apiErr.Code == "permission_denied" {
			return 0, nil
		}
	}
	return count, err
}

func mcpSyncAssets(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	known, err := ch.ListAssets()
	if err != nil {
		return 0, err
	}
	if len(known) == 0 {
		return 0, nil
	}
	if err := ch.SyncAssets(ctx, cl); err != nil {
		return 0, err
	}
	return len(known), nil
}

func mcpSyncComments(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	threads, err := ch.ListLocalCommentThreads()
	if err != nil {
		return 0, err
	}
	if len(threads) == 0 {
		return 0, nil
	}
	count := 0
	now := time.Now().Unix()
	for _, cached := range threads {
		if err := ctx.Err(); err != nil {
			return count, err
		}
		fresh, err := cl.GetCommentThread(ctx, cached.DesignID, cached.ID)
		if err != nil {
			continue
		}
		raw, _ := json.Marshal(fresh)
		root := cached.RootText
		if fresh.ThreadType.Content != nil && fresh.ThreadType.Content.Plaintext != "" {
			root = fresh.ThreadType.Content.Plaintext
		}
		author := cached.Author
		if fresh.Author != nil && fresh.Author.ID != "" {
			author = fresh.Author.ID
		}
		_ = ch.UpsertCommentThread(cache.CommentThread{
			ID: fresh.ID, DesignID: cached.DesignID, Author: author,
			RootText: root, CreatedAt: fresh.CreatedAt, UpdatedAt: fresh.UpdatedAt,
			FetchedAt: now, RawJSON: string(raw),
		})
		count++
		_ = cl.ListCommentReplies(ctx, cached.DesignID, cached.ID, func(r api.CommentReply) error {
			rawR, _ := json.Marshal(r)
			rauthor := ""
			if r.Author != nil {
				rauthor = r.Author.ID
			}
			return ch.UpsertCommentReply(cache.CommentReply{
				ID: r.ID, ThreadID: cached.ID, Author: rauthor,
				Text: r.Content.Plaintext, CreatedAt: r.CreatedAt,
				FetchedAt: now, RawJSON: string(rawR),
			})
		})
	}
	return count, nil
}

// handleSearch backs the canva_search MCP tool. Args: query (required),
// type (optional), limit (optional). Empty cache → cache_empty error.
func (s *Server) handleSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	query, _ := args["query"].(string)
	if query == "" {
		return mcp.NewToolResultError("query is required"), nil
	}
	typeFilter, _ := args["type"].(string)
	if typeFilter != "" && !cache.IsValidSearchType(typeFilter) {
		return mcp.NewToolResultError("invalid type (allowed: design, template, comment_thread, comment_reply, asset)"), nil
	}
	limit := 50
	if v, ok := args["limit"].(float64); ok && v > 0 {
		limit = int(v)
	}

	empty, err := s.cache.SearchCacheEmpty()
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if empty {
		return mcp.NewToolResultError("cache_empty: no rows in local cache; run canva_sync first"), nil
	}

	hits, err := s.cache.Search(ctx, query, typeFilter, limit)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		row := map[string]any{"type": h.Type, "id": h.ID, "rank": h.Rank}
		if h.Title != "" {
			row["title"] = h.Title
		}
		if h.Text != "" {
			row["text"] = h.Text
		}
		if h.DesignID != "" {
			row["design_id"] = h.DesignID
		}
		out = append(out, row)
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

// handleCreate backs the canva_create MCP tool. Mirrors commands/create.go
// (autofill from JSON, optional title; idempotency-key not exposed via MCP
// because clients can re-issue the same args trivially).
func (s *Server) handleCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	tplArg, _ := args["template"].(string)
	autofillRaw, _ := args["autofill"].(string)
	title, _ := args["title"].(string)
	if tplArg == "" || autofillRaw == "" {
		return mcp.NewToolResultError("template and autofill are required"), nil
	}

	// Resolve template by name or ID.
	tplID, err := resolver.New(s.cache, s.api).ResolveTemplate(tplArg)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(autofillRaw), &data); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("autofill must be inline JSON: %s", err.Error())), nil
	}

	res, err := s.api.CreateAutofill(ctx, api.AutofillRequest{
		BrandTemplateID: tplID,
		Data:            data,
		Title:           title,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	out := map[string]any{
		"id":    res.Design.ID,
		"url":   res.Design.URL,
		"title": res.Design.Title,
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

// handleTemplatesList backs canva_templates_list. Walks /brand-templates,
// upserting each row into the cache (so canva_templates_show name lookups
// work) and returning the full list as JSON.
func (s *Server) handleTemplatesList(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	out := []map[string]any{}
	now := time.Now().Unix()
	err := s.api.ListTemplates(ctx, func(t api.BrandTemplate) error {
		raw, _ := json.Marshal(t)
		_ = s.cache.UpsertTemplate(cache.Template{
			ID: t.ID, Title: t.Title, FetchedAt: now, RawJSON: string(raw),
		})
		out = append(out, map[string]any{
			"id": t.ID, "title": t.Title, "updated_at": t.UpdatedAt,
		})
		return nil
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(out)
	return mcp.NewToolResultText(string(b)), nil
}

// handleTemplatesShow backs canva_templates_show. Resolves the template
// (by name via the cache, falling back to listing templates from the
// API) and returns the autofill dataset.
func (s *Server) handleTemplatesShow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	q, _ := args["template_id_or_name"].(string)
	if q == "" {
		return mcp.NewToolResultError("template_id_or_name is required"), nil
	}
	id, err := resolver.New(s.cache, s.api).ResolveTemplate(q)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	ds, err := s.api.GetTemplateDataset(ctx, id)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	b, _ := json.Marshal(ds)
	return mcp.NewToolResultText(string(b)), nil
}
