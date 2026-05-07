// MCP comment-tool handlers.
//
// These implement the four `canva_comments_*` tools registered in
// server.go. Logic mirrors internal/commands/comments.go but adapted to
// MCP's req/result shape (errors returned as ToolResultError, never as Go
// errors — per the project's MCP contract).
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

// handleCommentsAdd dispatches to top-level-thread-create or reply-create
// depending on whether `reply_to` is set.
func (s *Server) handleCommentsAdd(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	designQuery, _ := args["design_id_or_name"].(string)
	text, _ := args["text"].(string)
	replyTo, _ := args["reply_to"].(string)
	if designQuery == "" || text == "" {
		return mcp.NewToolResultError("design_id_or_name and text are required"), nil
	}
	designID, err := resolver.New(s.cache, s.api).ResolveDesign(designQuery)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	now := time.Now().Unix()

	if replyTo != "" {
		reply, err := s.api.CreateCommentReply(ctx, designID, replyTo, api.CreateCommentReplyRequest{
			MessagePlaintext: text,
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		raw, _ := json.Marshal(reply)
		_ = s.cache.UpsertCommentReply(cache.CommentReply{
			ID: reply.ID, ThreadID: replyTo, Author: commentAuthor(reply.Author),
			Text: reply.Content.Plaintext, CreatedAt: reply.CreatedAt, FetchedAt: now,
			RawJSON: string(raw),
		})
		return commentsJSONResult(map[string]any{
			"reply_id":   reply.ID,
			"thread_id":  replyTo,
			"design_id":  designID,
			"author":     reply.Author,
			"created_at": reply.CreatedAt,
		})
	}

	thread, err := s.api.CreateCommentThread(ctx, designID, api.CreateCommentThreadRequest{
		MessagePlaintext: text,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	raw, _ := json.Marshal(thread)
	_ = s.cache.UpsertCommentThread(cache.CommentThread{
		ID: thread.ID, DesignID: designID, Author: commentAuthor(thread.Author),
		RootText:  commentRootText(thread, text),
		CreatedAt: thread.CreatedAt, UpdatedAt: thread.UpdatedAt, FetchedAt: now,
		RawJSON: string(raw),
	})
	return commentsJSONResult(map[string]any{
		"thread_id":  thread.ID,
		"design_id":  designID,
		"author":     thread.Author,
		"created_at": thread.CreatedAt,
	})
}

// handleCommentsReply requires thread_id; functionally a strict subset of
// handleCommentsAdd with reply_to forced. Kept as a separate tool for
// clarity per spec §4.6 (both names are listed in the contract).
func (s *Server) handleCommentsReply(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	designQuery, _ := args["design_id_or_name"].(string)
	threadID, _ := args["thread_id"].(string)
	text, _ := args["text"].(string)
	if designQuery == "" || threadID == "" || text == "" {
		return mcp.NewToolResultError("design_id_or_name, thread_id, and text are required"), nil
	}
	designID, err := resolver.New(s.cache, s.api).ResolveDesign(designQuery)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	reply, err := s.api.CreateCommentReply(ctx, designID, threadID, api.CreateCommentReplyRequest{
		MessagePlaintext: text,
	})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	now := time.Now().Unix()
	raw, _ := json.Marshal(reply)
	_ = s.cache.UpsertCommentReply(cache.CommentReply{
		ID: reply.ID, ThreadID: threadID, Author: commentAuthor(reply.Author),
		Text: reply.Content.Plaintext, CreatedAt: reply.CreatedAt, FetchedAt: now,
		RawJSON: string(raw),
	})
	return commentsJSONResult(map[string]any{
		"reply_id":   reply.ID,
		"thread_id":  threadID,
		"design_id":  designID,
		"author":     reply.Author,
		"created_at": reply.CreatedAt,
	})
}

// handleCommentsThread fetches the thread + replies. designID resolution
// order: explicit design_id_or_name arg, then local cache lookup by thread.
func (s *Server) handleCommentsThread(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	threadID, _ := args["thread_id"].(string)
	designQuery, _ := args["design_id_or_name"].(string)
	if threadID == "" {
		return mcp.NewToolResultError("thread_id is required"), nil
	}

	var designID string
	if designQuery != "" {
		id, err := resolver.New(s.cache, s.api).ResolveDesign(designQuery)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		designID = id
	} else {
		cached, err := s.cache.GetCommentThread("", threadID)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("comment_thread_not_in_cache: thread %q not found locally — pass design_id_or_name", threadID)), nil
		}
		designID = cached.DesignID
	}

	thread, replies, err := s.walkCommentThread(ctx, designID, threadID)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	now := time.Now().Unix()
	s.persistCommentThread(thread, designID, now)
	for _, r := range replies {
		s.persistCommentReply(r, now)
	}

	return commentsJSONResult(map[string]any{"thread": thread, "replies": replies})
}

// handleCommentsArchive walks every locally-known thread (or just one
// design's, when design_id_or_name is set) and re-fetches them fresh.
func (s *Server) handleCommentsArchive(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := req.GetArguments()
	designQuery, _ := args["design_id_or_name"].(string)

	var (
		threads []cache.CommentThread
		err     error
	)
	if designQuery != "" {
		designID, rerr := resolver.New(s.cache, s.api).ResolveDesign(designQuery)
		if rerr != nil {
			return mcp.NewToolResultError(rerr.Error()), nil
		}
		threads, err = s.cache.ListLocalCommentThreadsByDesign(designID)
	} else {
		threads, err = s.cache.ListLocalCommentThreads()
	}
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	if len(threads) == 0 {
		return mcp.NewToolResultError("comment_thread_not_in_cache: no threads cached locally — use canva_comments_add or canva_comments_thread first"), nil
	}

	now := time.Now().Unix()
	records := make([]map[string]any, 0, len(threads))
	for _, cached := range threads {
		t, replies, ferr := s.walkCommentThread(ctx, cached.DesignID, cached.ID)
		if ferr != nil {
			return mcp.NewToolResultError(ferr.Error()), nil
		}
		s.persistCommentThread(t, cached.DesignID, now)
		for _, r := range replies {
			s.persistCommentReply(r, now)
		}
		records = append(records, map[string]any{"thread": t, "replies": replies})
	}
	return commentsJSONResult(map[string]any{
		"design_id":    designQuery,
		"thread_count": len(records),
		"threads":      records,
	})
}

// walkCommentThread fetches a thread + paginates its replies. Used by
// `thread` and `archive` tool handlers.
func (s *Server) walkCommentThread(ctx context.Context, designID, threadID string) (*api.CommentThread, []api.CommentReply, error) {
	thread, err := s.api.GetCommentThread(ctx, designID, threadID)
	if err != nil {
		return nil, nil, err
	}
	var replies []api.CommentReply
	if err := s.api.ListCommentReplies(ctx, designID, threadID, func(r api.CommentReply) error {
		replies = append(replies, r)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return thread, replies, nil
}

func (s *Server) persistCommentThread(t *api.CommentThread, designID string, now int64) {
	raw, _ := json.Marshal(t)
	_ = s.cache.UpsertCommentThread(cache.CommentThread{
		ID: t.ID, DesignID: designID, Author: commentAuthor(t.Author),
		RootText:  commentRootText(t, ""),
		CreatedAt: t.CreatedAt, UpdatedAt: t.UpdatedAt, FetchedAt: now,
		RawJSON: string(raw),
	})
}

func (s *Server) persistCommentReply(r api.CommentReply, now int64) {
	raw, _ := json.Marshal(r)
	_ = s.cache.UpsertCommentReply(cache.CommentReply{
		ID: r.ID, ThreadID: r.ThreadID, Author: commentAuthor(r.Author),
		Text: r.Content.Plaintext, CreatedAt: r.CreatedAt, FetchedAt: now,
		RawJSON: string(raw),
	})
}

func commentAuthor(u *api.CommentUser) string {
	if u == nil {
		return ""
	}
	return u.ID
}

func commentRootText(t *api.CommentThread, fallback string) string {
	if t.ThreadType.Content != nil && t.ThreadType.Content.Plaintext != "" {
		return t.ThreadType.Content.Plaintext
	}
	return fallback
}

func commentsJSONResult(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	return mcp.NewToolResultText(string(b)), nil
}
