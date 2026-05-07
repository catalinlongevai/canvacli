// Comments API client unit tests.
//
// Cassette tests are deferred to Phase 4 — the cached token lacks
// comment:read / comment:write scopes. These tests use httptest mocks
// modeled on the OpenAPI shapes in docs/research/v2-comments-api.md.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const commentsTestDesignID = "DAFtestdesign"

func TestCreateCommentThread_HappyPath(t *testing.T) {
	var gotPath, gotMethod, gotAuth string
	var gotBody CreateCommentThreadRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"thread": {
				"id": "CMa1",
				"design_id": "DAFtestdesign",
				"thread_type": {
					"type": "comment",
					"content": {"plaintext": "Looks great!"}
				},
				"author": {"id": "oUser1", "display_name": "Cata"},
				"created_at": 1715040000,
				"updated_at": 1715040000
			}
		}`))
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	thread, err := c.CreateCommentThread(context.Background(), commentsTestDesignID, CreateCommentThreadRequest{
		MessagePlaintext: "Looks great!",
	})
	if err != nil {
		t.Fatalf("CreateCommentThread: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method: %q", gotMethod)
	}
	if gotPath != "/designs/DAFtestdesign/comments" {
		t.Fatalf("path: %q", gotPath)
	}
	if !strings.HasPrefix(gotAuth, "Bearer ") {
		t.Fatalf("auth: %q", gotAuth)
	}
	if gotBody.MessagePlaintext != "Looks great!" {
		t.Fatalf("body: %+v", gotBody)
	}
	if thread.ID != "CMa1" || thread.DesignID != "DAFtestdesign" {
		t.Fatalf("thread: %+v", thread)
	}
	if thread.ThreadType.Type != "comment" {
		t.Fatalf("type: %q", thread.ThreadType.Type)
	}
	if thread.ThreadType.Content == nil || thread.ThreadType.Content.Plaintext != "Looks great!" {
		t.Fatalf("content: %+v", thread.ThreadType.Content)
	}
}

func TestCreateCommentThread_PopulatesDesignIDFromPath(t *testing.T) {
	// Server omits design_id from response body — client should populate it
	// from the path.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"thread":{"id":"CMa2","thread_type":{"type":"comment"},"created_at":1,"updated_at":1}}`))
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	thread, err := c.CreateCommentThread(context.Background(), "DAFomit", CreateCommentThreadRequest{
		MessagePlaintext: "x",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if thread.DesignID != "DAFomit" {
		t.Fatalf("expected design_id from path, got %q", thread.DesignID)
	}
}

func TestCreateCommentReply_HappyPath(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody CreateCommentReplyRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{
			"reply": {
				"id": "RPa1",
				"design_id": "DAFtestdesign",
				"thread_id": "CMa1",
				"content": {"plaintext": "Thanks!"},
				"author": {"id": "oUser1"},
				"created_at": 1715040100,
				"updated_at": 1715040100
			}
		}`))
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	reply, err := c.CreateCommentReply(context.Background(), commentsTestDesignID, "CMa1", CreateCommentReplyRequest{
		MessagePlaintext: "Thanks!",
	})
	if err != nil {
		t.Fatalf("CreateCommentReply: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("method: %q", gotMethod)
	}
	if gotPath != "/designs/DAFtestdesign/comments/CMa1/replies" {
		t.Fatalf("path: %q", gotPath)
	}
	if gotBody.MessagePlaintext != "Thanks!" {
		t.Fatalf("body: %+v", gotBody)
	}
	if reply.ID != "RPa1" || reply.ThreadID != "CMa1" || reply.DesignID != "DAFtestdesign" {
		t.Fatalf("reply: %+v", reply)
	}
	if reply.Content.Plaintext != "Thanks!" {
		t.Fatalf("content: %+v", reply.Content)
	}
}

func TestCreateCommentReply_PopulatesIDsFromPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"reply":{"id":"RPa2","content":{"plaintext":"x"},"created_at":1,"updated_at":1}}`))
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	reply, err := c.CreateCommentReply(context.Background(), "DAFomit", "CMomit", CreateCommentReplyRequest{
		MessagePlaintext: "x",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if reply.DesignID != "DAFomit" || reply.ThreadID != "CMomit" {
		t.Fatalf("expected design_id+thread_id from path, got design=%q thread=%q", reply.DesignID, reply.ThreadID)
	}
}

func TestGetCommentThread_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/designs/DAFtestdesign/comments/CMa1" {
			t.Errorf("unexpected path: %q", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("method: %q", r.Method)
		}
		_, _ = w.Write([]byte(`{
			"thread": {
				"id": "CMa1",
				"design_id": "DAFtestdesign",
				"thread_type": {
					"type": "comment",
					"content": {"plaintext": "hi", "markdown": "hi"},
					"mentions": {
						"oU:oT": {"tag": "oU:oT", "user": {"user_id": "oU", "team_id": "oT", "display_name": "Cata"}}
					}
				},
				"author": {"id": "oU"},
				"created_at": 1, "updated_at": 2
			}
		}`))
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	thread, err := c.GetCommentThread(context.Background(), commentsTestDesignID, "CMa1")
	if err != nil {
		t.Fatalf("GetCommentThread: %v", err)
	}
	if thread.ID != "CMa1" {
		t.Fatalf("id: %q", thread.ID)
	}
	if len(thread.ThreadType.Mentions) != 1 {
		t.Fatalf("mentions: %+v", thread.ThreadType.Mentions)
	}
	m := thread.ThreadType.Mentions["oU:oT"]
	if m.Tag != "oU:oT" || m.User.UserID != "oU" || m.User.TeamID != "oT" {
		t.Fatalf("mention: %+v", m)
	}
}

func TestGetCommentThread_NotFoundRemapsCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	_, err := c.GetCommentThread(context.Background(), "d", "t")
	if err == nil {
		t.Fatalf("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != "thread_not_found" {
		t.Fatalf("code: %q (want thread_not_found)", apiErr.Code)
	}
}

func TestListCommentReplies_Pagination(t *testing.T) {
	pages := []string{
		`{"items":[{"id":"R1","content":{"plaintext":"a"},"created_at":1,"updated_at":1}],"continuation":"next"}`,
		`{"items":[{"id":"R2","content":{"plaintext":"b"},"created_at":2,"updated_at":2}],"continuation":""}`,
	}
	idx := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/designs/D/comments/T/replies") {
			t.Errorf("path: %q", r.URL.Path)
		}
		if idx == 1 && r.URL.Query().Get("continuation") != "next" {
			t.Errorf("expected continuation=next on page 2, got %q", r.URL.RawQuery)
		}
		_, _ = w.Write([]byte(pages[idx]))
		idx++
	}))
	defer srv.Close()
	c := NewClient("tok", WithBaseURL(srv.URL))

	got := []string{}
	err := c.ListCommentReplies(context.Background(), "D", "T", func(r CommentReply) error {
		got = append(got, r.ID)
		// IDs should be populated from the path even when the body omits them.
		if r.ThreadID != "T" || r.DesignID != "D" {
			t.Errorf("expected path-populated ids, got thread=%q design=%q", r.ThreadID, r.DesignID)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("ListCommentReplies: %v", err)
	}
	if len(got) != 2 || got[0] != "R1" || got[1] != "R2" {
		t.Fatalf("got %v", got)
	}
}
