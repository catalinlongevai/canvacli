// Comments cache round-trip tests.
package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// openCommentsTestCache returns a Cache with a "DAF1" parent design row
// already seeded — `comment_threads.design_id` references designs(id).
func openCommentsTestCache(t *testing.T) *Cache {
	t.Helper()
	c := openTestCache(t)
	now := time.Now().Unix()
	if err := c.UpsertDesign(Design{ID: "DAF1", Title: "Test", UpdatedAt: now, FetchedAt: now, RawJSON: "{}"}); err != nil {
		t.Fatalf("seed design: %v", err)
	}
	return c
}

func TestUpsertAndGetCommentThread(t *testing.T) {
	c := openCommentsTestCache(t)
	now := time.Now().Unix()
	if err := c.UpsertCommentThread(CommentThread{
		ID:        "CM1",
		DesignID:  "DAF1",
		Author:    "oUser",
		RootText:  "Looks great!",
		CreatedAt: now,
		UpdatedAt: now,
		FetchedAt: now,
		RawJSON:   `{"x":1}`,
	}); err != nil {
		t.Fatalf("UpsertCommentThread: %v", err)
	}
	got, err := c.GetCommentThread("DAF1", "CM1")
	if err != nil {
		t.Fatalf("GetCommentThread: %v", err)
	}
	if got.ID != "CM1" || got.DesignID != "DAF1" || got.RootText != "Looks great!" {
		t.Fatalf("got: %+v", got)
	}

	// Lookup without designID (used by `comments thread <id>` to discover the
	// design ID it needs for the API call).
	got2, err := c.GetCommentThread("", "CM1")
	if err != nil {
		t.Fatalf("GetCommentThread no-design: %v", err)
	}
	if got2.DesignID != "DAF1" {
		t.Fatalf("expected design_id from cache, got %q", got2.DesignID)
	}
}

func TestUpsertCommentThread_OverwritesOnConflict(t *testing.T) {
	c := openCommentsTestCache(t)
	if err := c.UpsertCommentThread(CommentThread{ID: "CM1", DesignID: "DAF1", RootText: "v1", CreatedAt: 1, FetchedAt: 1, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertCommentThread(CommentThread{ID: "CM1", DesignID: "DAF1", RootText: "v2", CreatedAt: 1, UpdatedAt: 9, FetchedAt: 9, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	got, err := c.GetCommentThread("", "CM1")
	if err != nil {
		t.Fatal(err)
	}
	if got.RootText != "v2" || got.UpdatedAt != 9 {
		t.Fatalf("got: %+v", got)
	}
}

func TestUpsertAndListReplies(t *testing.T) {
	c := openCommentsTestCache(t)
	if err := c.UpsertCommentThread(CommentThread{ID: "CM1", DesignID: "DAF1", RootText: "root", CreatedAt: 1, FetchedAt: 1, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	for i, txt := range []string{"a", "b", "c"} {
		if err := c.UpsertCommentReply(CommentReply{
			ID: "R" + string(rune('1'+i)), ThreadID: "CM1", Author: "u", Text: txt,
			CreatedAt: int64(10 + i), FetchedAt: 99, RawJSON: "{}",
		}); err != nil {
			t.Fatalf("UpsertCommentReply: %v", err)
		}
	}
	replies, err := c.ListReplies("CM1")
	if err != nil {
		t.Fatalf("ListReplies: %v", err)
	}
	if len(replies) != 3 {
		t.Fatalf("expected 3 replies, got %d", len(replies))
	}
	if replies[0].Text != "a" || replies[2].Text != "c" {
		t.Fatalf("order wrong: %+v", replies)
	}
}

func TestListLocalCommentThreads(t *testing.T) {
	c := openCommentsTestCache(t)
	now := time.Now().Unix()
	// Seed two designs + threads on each.
	if err := c.UpsertDesign(Design{ID: "DAF2", Title: "Other", UpdatedAt: now, FetchedAt: now, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	for i, id := range []string{"CM1", "CM2"} {
		if err := c.UpsertCommentThread(CommentThread{ID: id, DesignID: "DAF1", RootText: "x", CreatedAt: int64(i), FetchedAt: 1, RawJSON: "{}"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.UpsertCommentThread(CommentThread{ID: "CM3", DesignID: "DAF2", RootText: "y", CreatedAt: 1, FetchedAt: 1, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}

	all, err := c.ListLocalCommentThreads()
	if err != nil {
		t.Fatalf("ListLocalCommentThreads: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("expected 3 across both designs, got %d", len(all))
	}

	one, err := c.ListLocalCommentThreadsByDesign("DAF1")
	if err != nil {
		t.Fatalf("ListLocalCommentThreadsByDesign: %v", err)
	}
	if len(one) != 2 {
		t.Fatalf("expected 2 on DAF1, got %d", len(one))
	}
}

func TestSyncComments_RewalksAllThreads(t *testing.T) {
	c := openCommentsTestCache(t)
	if err := c.UpsertCommentThread(CommentThread{ID: "CM1", DesignID: "DAF1", RootText: "old", CreatedAt: 1, FetchedAt: 1, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	if err := c.UpsertCommentThread(CommentThread{ID: "CM2", DesignID: "DAF1", RootText: "old2", CreatedAt: 2, FetchedAt: 1, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}

	visits := 0
	err := c.SyncComments(context.Background(), func(_ context.Context, designID, threadID string, _ CommentThread) (FetchedThread, error) {
		visits++
		return FetchedThread{
			Thread: CommentThread{ID: threadID, DesignID: designID, RootText: "fresh-" + threadID, CreatedAt: 1, UpdatedAt: 99, FetchedAt: 99, RawJSON: "{}"},
			Replies: []CommentReply{
				{ID: "R-" + threadID, ThreadID: threadID, Text: "rfresh", CreatedAt: 50, FetchedAt: 99, RawJSON: "{}"},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("SyncComments: %v", err)
	}
	if visits != 2 {
		t.Fatalf("expected 2 visits, got %d", visits)
	}

	got, _ := c.GetCommentThread("", "CM1")
	if got.RootText != "fresh-CM1" || got.UpdatedAt != 99 {
		t.Fatalf("after sync: %+v", got)
	}
	replies, _ := c.ListReplies("CM1")
	if len(replies) != 1 || replies[0].Text != "rfresh" {
		t.Fatalf("replies: %+v", replies)
	}
}

func TestSyncComments_PropagatesFetchError(t *testing.T) {
	c := openCommentsTestCache(t)
	if err := c.UpsertCommentThread(CommentThread{ID: "CM1", DesignID: "DAF1", RootText: "x", CreatedAt: 1, FetchedAt: 1, RawJSON: "{}"}); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("boom")
	err := c.SyncComments(context.Background(), func(_ context.Context, _, _ string, _ CommentThread) (FetchedThread, error) {
		return FetchedThread{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected boom, got %v", err)
	}
}

func TestGetCommentThread_NotFound(t *testing.T) {
	c := openCommentsTestCache(t)
	_, err := c.GetCommentThread("", "no-such-id")
	if err == nil {
		t.Fatalf("expected error for missing thread")
	}
}
