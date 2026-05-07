package commands

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/spf13/cobra"
)

// syncResult is the per-resource summary line.
//
// Emitted to stderr as NDJSON (one line per resource) so the user gets live
// progress; the final summary lands on stdout as a single JSON object.
type syncResult struct {
	Resource string `json:"resource"`
	OK       bool   `json:"ok"`
	Count    int    `json:"count"`
	Error    string `json:"error,omitempty"`
	TookMS   int64  `json:"took_ms"`
}

// NewSync returns the `canva sync` command (spec §4.1 + §6).
//
// All-in-one mirror — no opt-out. Designs, folders, templates,
// known-thread comments, and known assets are pulled from Canva and
// written to the local SQLite cache. FTS5 indices update via triggers.
//
// Each resource runs independently. A failure in one resource does not
// abort the others; the failing resource's last known cursor stays in
// place and the next `canva sync` resumes from it (spec §6.5).
func NewSync() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Mirror your Canva account into local SQLite (designs, folders, templates, comments, assets)",
		Long: `Sync runs an all-in-one mirror — there is no opt-out.

Designs, folders, templates, known comment threads, and known assets are
pulled from Canva and written to the local SQLite cache. FTS5 indices
update via triggers, so 'canva search' picks up new content immediately.

Designed to be idempotent and re-runnable. First sync may take ~30-60s
for typical accounts; subsequent syncs are incremental via cursor.

Limitations (spec §4.1, §9):
  - Comments: only threads canvacli has interacted with are re-walked.
    The Canva Connect API has no list-threads endpoint.
  - Assets: only assets in the local cache (uploaded via canvacli) are
    refreshed. The Connect API has no list-assets endpoint.`,
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

			results := []syncResult{}
			runOne := func(name string, fn func(context.Context, *api.Client, *cache.Cache) (int, error)) {
				start := time.Now()
				count, runErr := fn(ctx, cl, ch)
				r := syncResult{
					Resource: name,
					OK:       runErr == nil,
					Count:    count,
					TookMS:   time.Since(start).Milliseconds(),
				}
				if runErr != nil {
					r.Error = runErr.Error()
				}
				results = append(results, r)
				if line, mErr := json.Marshal(r); mErr == nil {
					fmt.Fprintln(os.Stderr, string(line))
				}
			}

			// Order matches spec §6.1 — folders, designs, templates,
			// then the per-row resources (assets, comments) which only
			// touch rows already in the cache.
			runOne("folders", syncFolders)
			runOne("designs", syncDesigns)
			runOne("templates", syncTemplates)
			runOne("assets", syncAssets)
			runOne("comments", syncComments)

			out := map[string]any{"results": results}
			return json.NewEncoder(os.Stdout).Encode(out)
		},
	}
}

// syncDesigns walks /designs and upserts each into the cache. Uses the
// existing Paginate-based ListDesigns helper.
func syncDesigns(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	count := 0
	now := time.Now().Unix()
	err := cl.ListDesigns(ctx, func(d api.Design) error {
		raw, _ := json.Marshal(d)
		thumb := ""
		if d.Thumbnail != nil {
			thumb = d.Thumbnail.URL
		}
		if upErr := ch.UpsertDesign(cache.Design{
			ID:           d.ID,
			Title:        d.Title,
			UpdatedAt:    d.UpdatedAt,
			FetchedAt:    now,
			ThumbnailURL: thumb,
			RawJSON:      string(raw),
		}); upErr != nil {
			return upErr
		}
		count++
		return nil
	})
	return count, err
}

// syncFolders walks /folders/{id}/items recursively from "root" and
// "uploads" and upserts every folder it finds.
func syncFolders(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	count := 0
	now := time.Now().Unix()
	err := cl.WalkFolders(ctx, func(f api.Folder, parent string) error {
		if upErr := ch.UpsertFolder(cache.Folder{
			ID:        f.ID,
			Name:      f.Name,
			ParentID:  parent,
			FetchedAt: now,
		}); upErr != nil {
			return upErr
		}
		count++
		return nil
	})
	return count, err
}

// syncTemplates walks /brand-templates. Enterprise-gated — accounts
// without the Brand Templates feature get permission_denied, which we
// silently translate to "synced 0" so a non-Enterprise sync still
// succeeds end-to-end (spec §4.1).
func syncTemplates(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
	count := 0
	now := time.Now().Unix()
	err := cl.ListTemplates(ctx, func(t api.BrandTemplate) error {
		raw, _ := json.Marshal(t)
		if upErr := ch.UpsertTemplate(cache.Template{
			ID:        t.ID,
			Title:     t.Title,
			FetchedAt: now,
			RawJSON:   string(raw),
		}); upErr != nil {
			return upErr
		}
		count++
		return nil
	})
	if err != nil {
		var apiErr *api.APIError
		if errors.As(err, &apiErr) && apiErr.Code == "permission_denied" {
			return 0, nil
		}
		return count, err
	}
	return count, nil
}

// syncAssets refreshes every asset already in the local cache. Calls
// into cache.SyncAssets which iterates ListAssets() and re-GETs each.
//
// Returns the count of assets known locally (best-effort proxy for
// "rows refreshed"; cache.SyncAssets does not currently report a count).
func syncAssets(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
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

// syncComments re-walks every locally known thread + replies. Bridges the
// cache.SyncComments fetch callback to api.GetCommentThread + api.ListCommentReplies.
//
// Errors on any single thread fetch are logged-and-skipped rather than
// failing the whole sync, since one bad thread (e.g. design moved to
// trash) shouldn't take down the whole run.
func syncComments(ctx context.Context, cl *api.Client, ch *cache.Cache) (int, error) {
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
			// Skip — one missing thread shouldn't poison the whole sync.
			continue
		}
		raw, _ := json.Marshal(fresh)
		root := ""
		author := cached.Author
		if fresh.ThreadType.Content != nil {
			root = fresh.ThreadType.Content.Plaintext
		}
		if root == "" {
			root = cached.RootText
		}
		if fresh.Author != nil && fresh.Author.ID != "" {
			author = fresh.Author.ID
		}
		if err := ch.UpsertCommentThread(cache.CommentThread{
			ID:        fresh.ID,
			DesignID:  cached.DesignID,
			Author:    author,
			RootText:  root,
			CreatedAt: fresh.CreatedAt,
			UpdatedAt: fresh.UpdatedAt,
			FetchedAt: now,
			RawJSON:   string(raw),
		}); err != nil {
			return count, err
		}
		count++
		// Replies — cap is enforced by Canva (≤100).
		if rerr := cl.ListCommentReplies(ctx, cached.DesignID, cached.ID, func(r api.CommentReply) error {
			rawR, _ := json.Marshal(r)
			rauthor := ""
			if r.Author != nil {
				rauthor = r.Author.ID
			}
			return ch.UpsertCommentReply(cache.CommentReply{
				ID:        r.ID,
				ThreadID:  cached.ID,
				Author:    rauthor,
				Text:      r.Content.Plaintext,
				CreatedAt: r.CreatedAt,
				FetchedAt: now,
				RawJSON:   string(rawR),
			})
		}); rerr != nil {
			// Replies failed — leave thread upserted, keep going.
			continue
		}
	}
	return count, nil
}
