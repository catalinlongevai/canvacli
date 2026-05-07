// Comments API client.
//
// Schemas mirror docs/research/v2-comments-api.md. The Connect API has two
// object kinds — Thread (top-level) and Reply (children). Threads carry a
// discriminated thread_type ("comment" | "suggestion"); canvacli only writes
// "comment" threads but reads must handle both. There is no list-threads
// endpoint, so callers source thread IDs out-of-band (local cache, webhooks).
//
// Rate caps from the spec's x-rate-limit-per-client-user (per user, per min):
//   POST   /designs/{d}/comments                     100/min
//   POST   /designs/{d}/comments/{t}/replies          20/min   (bottleneck)
//   GET    /designs/{d}/comments/{t}                 100/min
//   GET    /designs/{d}/comments/{t}/replies         100/min
//
// All endpoints are synchronous — no job/poll envelope.
package api

import (
	"context"
	"fmt"
	"net/http"
)

// CommentUser is the shared author/assignee/resolver shape.
type CommentUser struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
}

// CommentContent carries the rendered message. Plaintext is what the caller
// supplies on POST; markdown is server-derived and only present on responses.
type CommentContent struct {
	Plaintext string `json:"plaintext"`
	Markdown  string `json:"markdown,omitempty"`
}

// CommentMention is the resolved form of an inline `[user_id:team_id]` tag.
// The mentions map on a Thread/Reply is keyed by the literal tag string.
type CommentMention struct {
	Tag  string `json:"tag"`
	User struct {
		UserID      string `json:"user_id"`
		TeamID      string `json:"team_id"`
		DisplayName string `json:"display_name,omitempty"`
	} `json:"user"`
}

// CommentThread is the Connect Thread object. The discriminator is
// ThreadType.Type ("comment" or "suggestion"). DesignID is populated from
// the path by the API client when the response omits it; do not rely on it
// being present in the raw JSON for replies-only operations.
type CommentThread struct {
	ID         string            `json:"id"`
	DesignID   string            `json:"design_id,omitempty"`
	ThreadType CommentThreadType `json:"thread_type"`
	Author     *CommentUser      `json:"author,omitempty"`
	CreatedAt  int64             `json:"created_at"`
	UpdatedAt  int64             `json:"updated_at"`
}

// CommentThreadType is the discriminated union body of a thread.
type CommentThreadType struct {
	Type string `json:"type"` // "comment" | "suggestion"

	// "comment" branch
	Content  *CommentContent           `json:"content,omitempty"`
	Mentions map[string]CommentMention `json:"mentions,omitempty"`
	Assignee *CommentUser              `json:"assignee,omitempty"`
	Resolver *CommentUser              `json:"resolver,omitempty"`

	// "suggestion" branch
	SuggestedEdits []map[string]any `json:"suggested_edits,omitempty"`
	Status         string           `json:"status,omitempty"` // open|accepted|rejected
}

// CommentReply is the Connect Reply object.
type CommentReply struct {
	ID        string                    `json:"id"`
	DesignID  string                    `json:"design_id,omitempty"`
	ThreadID  string                    `json:"thread_id,omitempty"`
	Author    *CommentUser              `json:"author,omitempty"`
	Content   CommentContent            `json:"content"`
	Mentions  map[string]CommentMention `json:"mentions,omitempty"`
	CreatedAt int64                     `json:"created_at"`
	UpdatedAt int64                     `json:"updated_at"`
}

// CreateCommentThreadRequest mirrors the OpenAPI CreateThreadRequest.
type CreateCommentThreadRequest struct {
	MessagePlaintext string `json:"message_plaintext"`
	AssigneeID       string `json:"assignee_id,omitempty"`
}

// CreateCommentReplyRequest mirrors CreateReplyV2Request.
type CreateCommentReplyRequest struct {
	MessagePlaintext string `json:"message_plaintext"`
}

// CreateCommentThread posts a new top-level thread on a design.
//
// Side effects: irreversible — there is no edit/delete endpoint. A retry
// after a flaky 5xx will create a duplicate thread. Callers should debounce
// or hash (designID, message, assignee) to dedup within-process retries.
func (c *Client) CreateCommentThread(ctx context.Context, designID string, req CreateCommentThreadRequest) (*CommentThread, error) {
	var env struct {
		Thread CommentThread `json:"thread"`
	}
	path := fmt.Sprintf("/designs/%s/comments", designID)
	if err := c.doJSON(ctx, http.MethodPost, path, req, &env); err != nil {
		return nil, err
	}
	if env.Thread.DesignID == "" {
		env.Thread.DesignID = designID
	}
	return &env.Thread, nil
}

// CreateCommentReply posts a reply on an existing thread.
func (c *Client) CreateCommentReply(ctx context.Context, designID, threadID string, req CreateCommentReplyRequest) (*CommentReply, error) {
	var env struct {
		Reply CommentReply `json:"reply"`
	}
	path := fmt.Sprintf("/designs/%s/comments/%s/replies", designID, threadID)
	if err := c.doJSON(ctx, http.MethodPost, path, req, &env); err != nil {
		return nil, err
	}
	if env.Reply.ThreadID == "" {
		env.Reply.ThreadID = threadID
	}
	if env.Reply.DesignID == "" {
		env.Reply.DesignID = designID
	}
	return &env.Reply, nil
}

// GetCommentThread fetches a single thread. Note the path requires designID;
// callers without one in hand should look it up from the local cache first.
//
// The deprecated `comment` field on the response is ignored — we only read
// `thread`.
func (c *Client) GetCommentThread(ctx context.Context, designID, threadID string) (*CommentThread, error) {
	var env struct {
		Thread CommentThread `json:"thread"`
	}
	path := fmt.Sprintf("/designs/%s/comments/%s", designID, threadID)
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &env); err != nil {
		if apiErr, ok := err.(*APIError); ok && apiErr.Code == "not_found" {
			apiErr.Code = "thread_not_found"
		}
		return nil, err
	}
	if env.Thread.ID == "" {
		env.Thread.ID = threadID
	}
	if env.Thread.DesignID == "" {
		env.Thread.DesignID = designID
	}
	return &env.Thread, nil
}

// ListCommentReplies paginates through every reply on a thread, invoking
// `visit` for each. Pagination follows the project's standard
// items+continuation envelope.
//
// Hard cap of 100 replies per thread means at most one or two pages.
func (c *Client) ListCommentReplies(ctx context.Context, designID, threadID string, visit func(CommentReply) error) error {
	path := fmt.Sprintf("/designs/%s/comments/%s/replies", designID, threadID)
	wrap := func(r CommentReply) error {
		if r.ThreadID == "" {
			r.ThreadID = threadID
		}
		if r.DesignID == "" {
			r.DesignID = designID
		}
		return visit(r)
	}
	return Paginate[CommentReply](ctx, c, path, wrap)
}
