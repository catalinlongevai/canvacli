// Comments cobra command tree.
//
//   canva comments add <design> "<text>" [--reply-to <thread-id>]
//   canva comments thread <thread-id> [--design <design-id>]
//   canva comments archive [--design <design-id>]
//
// The local thread cache is the workaround for the missing list-threads
// endpoint (spec §9). Threads enter the cache only when canvacli has
// interacted with them — see the `archive` --help text for the limitation.
package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
)

const archiveHelp = `Archive comment threads from the local cache.

The Canva Connect API does not expose a list-threads endpoint, so this
command works against a LOCAL thread cache populated by user activity:

  - Threads created via 'canva comments add' are cached automatically.
  - Threads fetched via 'canva comments thread <id>' are cached.
  - Threads created in the Canva web UI are NOT visible to canvacli until
    you run 'canva comments thread <id>' to pull them in.

For each cached thread, archive re-fetches the thread + replies fresh from
the API and emits one JSON record per thread to stdout.`

// NewComments is the parent for the v2 comments subcommand tree.
func NewComments() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "comments",
		Short: "Comment thread operations (add / thread / archive)",
		Long: `Manage Canva comments.

Required scopes (re-run 'canva login' on a fresh v2 binary):
  comment:read   — thread, archive
  comment:write  — add (and replies)`,
	}
	cmd.AddCommand(newCommentsAdd())
	cmd.AddCommand(newCommentsThread())
	cmd.AddCommand(newCommentsArchive())
	return cmd
}

func newCommentsAdd() *cobra.Command {
	var flagReplyTo string
	cmd := &cobra.Command{
		Use:   "add <design-id-or-name> <message>",
		Short: "Post a top-level comment on a design (or a reply with --reply-to)",
		Long: `Post a comment on a Canva design.

Without --reply-to: creates a new top-level thread on the design.
With --reply-to: posts a reply on an existing thread (the thread must be
locally cached so its design ID can be resolved, OR you must pass the
design ID directly as the first argument).

The comment is irreversible — the Canva Connect API has no edit/delete
endpoint. A typo is permanent.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			designQuery, message := args[0], args[1]
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

			designID, err := resolver.New(ch, cl).ResolveDesign(designQuery)
			if err != nil {
				return err
			}

			now := time.Now().Unix()
			if flagReplyTo != "" {
				reply, err := cl.CreateCommentReply(ctx, designID, flagReplyTo, api.CreateCommentReplyRequest{
					MessagePlaintext: message,
				})
				if err != nil {
					return err
				}
				rawJSON, _ := json.Marshal(reply)
				_ = ch.UpsertCommentReply(cache.CommentReply{
					ID:        reply.ID,
					ThreadID:  flagReplyTo,
					Author:    authorID(reply.Author),
					Text:      reply.Content.Plaintext,
					CreatedAt: reply.CreatedAt,
					FetchedAt: now,
					RawJSON:   string(rawJSON),
				})
				return output.EmitJSON(os.Stdout, map[string]any{
					"reply_id":   reply.ID,
					"thread_id":  flagReplyTo,
					"design_id":  designID,
					"author":     reply.Author,
					"created_at": reply.CreatedAt,
				})
			}

			thread, err := cl.CreateCommentThread(ctx, designID, api.CreateCommentThreadRequest{
				MessagePlaintext: message,
			})
			if err != nil {
				return err
			}
			rawJSON, _ := json.Marshal(thread)
			_ = ch.UpsertCommentThread(cache.CommentThread{
				ID:        thread.ID,
				DesignID:  designID,
				Author:    authorID(thread.Author),
				RootText:  threadRootText(thread, message),
				CreatedAt: thread.CreatedAt,
				UpdatedAt: thread.UpdatedAt,
				FetchedAt: now,
				RawJSON:   string(rawJSON),
			})
			return output.EmitJSON(os.Stdout, map[string]any{
				"thread_id":  thread.ID,
				"design_id":  designID,
				"author":     thread.Author,
				"created_at": thread.CreatedAt,
			})
		},
	}
	cmd.Flags().StringVar(&flagReplyTo, "reply-to", "", "post as reply on this thread ID instead of creating a new thread")
	return cmd
}

func newCommentsThread() *cobra.Command {
	var flagDesign string
	cmd := &cobra.Command{
		Use:   "thread <thread-id>",
		Short: "Fetch a thread + replies and refresh the local cache",
		Long: `Fetch a comment thread by ID. The Canva API requires a designID in
the path, so canvacli first looks up the thread in the local cache. If the
thread isn't cached (e.g. the user copy-pasted an ID from the Canva UI URL
without ever creating it via canvacli), pass --design <design-id>.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			threadID := args[0]
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

			designID := flagDesign
			if designID == "" {
				cached, err := ch.GetCommentThread("", threadID)
				if err != nil {
					return &commentNotInCacheError{ThreadID: threadID}
				}
				designID = cached.DesignID
			}

			thread, replies, err := walkThread(ctx, cl, designID, threadID)
			if err != nil {
				return err
			}
			now := time.Now().Unix()
			persistThread(ch, thread, designID, now)
			for _, r := range replies {
				persistReply(ch, r, now)
			}

			return output.EmitJSON(os.Stdout, map[string]any{
				"thread":  thread,
				"replies": replies,
			})
		},
	}
	cmd.Flags().StringVar(&flagDesign, "design", "", "design ID (required when the thread is not yet locally cached)")
	return cmd
}

func newCommentsArchive() *cobra.Command {
	var flagDesign string
	cmd := &cobra.Command{
		Use:   "archive",
		Short: "Archive locally-known comment threads (re-fetches each thread + replies)",
		Long:  archiveHelp,
		RunE: func(cmd *cobra.Command, _ []string) error {
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

			var threads []cache.CommentThread
			if flagDesign != "" {
				designID, rerr := resolver.New(ch, cl).ResolveDesign(flagDesign)
				if rerr != nil {
					return rerr
				}
				threads, err = ch.ListLocalCommentThreadsByDesign(designID)
			} else {
				threads, err = ch.ListLocalCommentThreads()
			}
			if err != nil {
				return err
			}
			if len(threads) == 0 {
				return &commentNoThreadsError{Design: flagDesign}
			}

			now := time.Now().Unix()
			records := make([]map[string]any, 0, len(threads))
			for _, cached := range threads {
				thread, replies, ferr := walkThread(ctx, cl, cached.DesignID, cached.ID)
				if ferr != nil {
					return ferr
				}
				persistThread(ch, thread, cached.DesignID, now)
				for _, r := range replies {
					persistReply(ch, r, now)
				}
				records = append(records, map[string]any{
					"thread":  thread,
					"replies": replies,
				})
			}
			return output.EmitJSON(os.Stdout, map[string]any{
				"design_id":     flagDesign,
				"thread_count":  len(records),
				"threads":       records,
			})
		},
	}
	cmd.Flags().StringVar(&flagDesign, "design", "", "limit archive to one design (id or name)")
	return cmd
}

// walkThread fetches the thread + paginated replies. Helper used by both
// `thread` and `archive`.
func walkThread(ctx context.Context, cl *api.Client, designID, threadID string) (*api.CommentThread, []api.CommentReply, error) {
	thread, err := cl.GetCommentThread(ctx, designID, threadID)
	if err != nil {
		return nil, nil, err
	}
	var replies []api.CommentReply
	if err := cl.ListCommentReplies(ctx, designID, threadID, func(r api.CommentReply) error {
		replies = append(replies, r)
		return nil
	}); err != nil {
		return nil, nil, err
	}
	return thread, replies, nil
}

func persistThread(ch *cache.Cache, t *api.CommentThread, designID string, now int64) {
	rawJSON, _ := json.Marshal(t)
	_ = ch.UpsertCommentThread(cache.CommentThread{
		ID:        t.ID,
		DesignID:  designID,
		Author:    authorID(t.Author),
		RootText:  threadRootText(t, ""),
		CreatedAt: t.CreatedAt,
		UpdatedAt: t.UpdatedAt,
		FetchedAt: now,
		RawJSON:   string(rawJSON),
	})
}

func persistReply(ch *cache.Cache, r api.CommentReply, now int64) {
	rawJSON, _ := json.Marshal(r)
	_ = ch.UpsertCommentReply(cache.CommentReply{
		ID:        r.ID,
		ThreadID:  r.ThreadID,
		Author:    authorID(r.Author),
		Text:      r.Content.Plaintext,
		CreatedAt: r.CreatedAt,
		FetchedAt: now,
		RawJSON:   string(rawJSON),
	})
}

func authorID(u *api.CommentUser) string {
	if u == nil {
		return ""
	}
	return u.ID
}

// threadRootText extracts the displayable plaintext from a thread, falling
// back to `fallback` (used on create when the response omits content).
func threadRootText(t *api.CommentThread, fallback string) string {
	if t.ThreadType.Content != nil && t.ThreadType.Content.Plaintext != "" {
		return t.ThreadType.Content.Plaintext
	}
	return fallback
}

// commentNotInCacheError matches the spec §4.5 wrinkle for `comments thread`
// when the thread isn't locally cached.
type commentNotInCacheError struct{ ThreadID string }

func (e *commentNotInCacheError) Error() string {
	return fmt.Sprintf("comment_thread_not_in_cache: thread %q not found locally — pass --design <id> or run `canva comments add <design-id> \"...\"` first", e.ThreadID)
}

// commentNoThreadsError matches the spec §4.5 + §9.4 envelope for `archive`
// against a design with no cached threads.
type commentNoThreadsError struct{ Design string }

func (e *commentNoThreadsError) Error() string {
	if e.Design == "" {
		return "comment_thread_not_in_cache: no threads cached locally — run `canva comments add <design-id> \"...\"` or `canva comments thread <id> --design <design-id>` first"
	}
	return fmt.Sprintf("comment_thread_not_in_cache: no threads cached for design %q — run `canva comments add %s \"...\"` or `canva comments thread <id> --design %s` first", e.Design, e.Design, e.Design)
}

// compile-time check the error types satisfy `error`.
var (
	_ error = (*commentNotInCacheError)(nil)
	_ error = (*commentNoThreadsError)(nil)
)
