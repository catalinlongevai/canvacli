# canvacli v1 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship v1 of `canvacli`, an agent-first Go CLI for the Canva Connect API: auth, brand-template-driven design creation, listing, exporting, folder traversal, plus agent affordances (`schema`, `sql`).

**Architecture:** Single Go binary distributed via Homebrew tap and GitHub releases. Speaks only the Canva Connect REST API (`https://api.canva.com/rest/v1`). OAuth 2.0 PKCE with fixed-port browser callback, atomic token storage. SQLite (pure-Go via `modernc.org/sqlite`) backs name resolution and client-side idempotency. Output is auto-JSON when stdout is non-TTY. Stable error envelope with `error` code + `fix` action for agents.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra`, `golang.org/x/oauth2`, `modernc.org/sqlite`, `golang.org/x/term`, `gopkg.in/dnaeon/go-vcr.v3` (test cassettes), GoReleaser v2.15+, GitHub Actions.

**Spec:** `docs/superpowers/specs/2026-05-07-canvacli-design.md`. Read this first.

**Research notes:** `docs/research/canva-api.md`, `docs/research/oauth-pkce-go.md`, `docs/research/release-pipeline.md`, `docs/research/spec-audit.md`. Each task references the relevant research file when copy-adaptable code is available there.

---

## File Structure

The implementation creates these files. Each has one responsibility.

```
canvacli/
├── cmd/canvacli/
│   └── main.go                          process entry, version vars, calls cobra root
├── internal/
│   ├── api/
│   │   ├── client.go                    HTTP client, base URL, auth header
│   │   ├── errors.go                    typed errors mapping to exit codes
│   │   ├── job.go                       generic async job poller (autofill, export, etc.)
│   │   ├── pagination.go                items+continuation cursor walker
│   │   ├── designs.go                   GET /designs, GET /designs/{id}
│   │   ├── templates.go                 GET /brand-templates, GET /brand-templates/{id}/dataset
│   │   ├── autofill.go                  POST /autofills, GET /autofills/{id}
│   │   ├── exports.go                   POST /exports, GET /exports/{id}, download
│   │   ├── folders.go                   GET /folders/{id}/items
│   │   └── users.go                     GET /users/me
│   ├── auth/
│   │   ├── pkce.go                      verifier, challenge, state generation
│   │   ├── callback.go                  fixed-port HTTP listener, CSRF check
│   │   ├── token.go                     atomic JSON write/read, mode 0600
│   │   ├── source.go                    persistingSource (saves on refresh)
│   │   ├── transport.go                 refreshOn401Transport
│   │   └── browser.go                   cross-platform browser opener
│   ├── cache/
│   │   ├── db.go                        sqlite open, migrations, schema
│   │   ├── designs.go                   designs table CRUD
│   │   ├── templates.go                 templates table CRUD
│   │   ├── folders.go                   folders table CRUD
│   │   ├── idempotency.go               idempotency table CRUD
│   │   └── sql.go                       read-only SQL handler with allowlist
│   ├── resolver/
│   │   └── resolver.go                  name-or-ID resolution
│   ├── output/
│   │   ├── tty.go                       TTY detection
│   │   ├── json.go                      single + NDJSON formatters
│   │   ├── table.go                     human-readable table formatter
│   │   ├── fields.go                    --fields projection
│   │   └── errors.go                    stable error envelope
│   ├── config/
│   │   └── paths.go                     os.UserConfigDir wrapper, ensure dirs
│   └── commands/
│       ├── root.go                      cobra root, global flags
│       ├── login.go                     canva login
│       ├── logout.go                    canva logout
│       ├── whoami.go                    canva whoami
│       ├── templates.go                 canva templates [show]
│       ├── create.go                    canva create (killer command)
│       ├── list.go                      canva list
│       ├── export.go                    canva export
│       ├── folders.go                   canva folders
│       ├── schema.go                    canva schema
│       └── sql.go                       canva sql
├── testdata/
│   └── cassettes/                       go-vcr recorded HTTP fixtures
├── .goreleaser.yaml
├── .github/workflows/
│   ├── ci.yml                           test on PR + main
│   └── release.yml                      tag-triggered release
├── CLAUDE.md                            shipped agent instructions (≤150 lines)
├── README.md
├── LICENSE                              MIT
├── go.mod
└── go.sum
```

Files are split by responsibility (auth vs cache vs api vs commands), with one file per Canva resource type inside `internal/api/` so consumers can grep for `designs.go` and find every design operation in one place.

---

## Phase 0 — Project Foundation

### Task 1: Initialize Go module + base files

**Files:**
- Create: `go.mod`, `LICENSE`, `.gitignore`, `cmd/canvacli/main.go`, `README.md`

- [ ] **Step 1: Init Go module**

```bash
cd /Users/cata/Desktop/canvacli
go mod init github.com/catalinlongevai/canvacli
```

- [ ] **Step 2: Create `.gitignore`**

```
# Binaries
canvacli
canvacli-*
dist/

# Local dev
*.db
*.log
.env
.DS_Store

# Go
*.test
*.out
coverage.out
```

- [ ] **Step 3: Create `LICENSE` (MIT)**

```
MIT License

Copyright (c) 2026 Catalin Niculescu

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.
```

- [ ] **Step 4: Create skeleton `cmd/canvacli/main.go`**

```go
package main

import (
	"fmt"
	"os"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf("canvacli %s (commit %s, built %s)\n", version, commit, date)
		os.Exit(0)
	}
	fmt.Fprintln(os.Stderr, "canvacli: not yet implemented")
	os.Exit(1)
}
```

- [ ] **Step 5: Verify build**

```bash
go build -o canvacli ./cmd/canvacli && ./canvacli --version
```

Expected output: `canvacli dev (commit none, built unknown)`

- [ ] **Step 6: Commit**

```bash
git add go.mod LICENSE .gitignore cmd/canvacli/main.go
git commit -m "feat: scaffold Go module and main entry"
```

### Task 2: Add CI workflow for tests + lint

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Write the CI workflow**

```yaml
name: CI

on:
  pull_request:
  push:
    branches: [master]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version: '1.22'
          cache: true
      - name: go vet
        run: go vet ./...
      - name: go test
        run: go test -race -count=1 ./...
      - name: build
        run: go build -o /tmp/canvacli ./cmd/canvacli
      - name: smoke test
        run: /tmp/canvacli --version
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: add test and build workflow"
```

### Task 3: Add config paths package

**Files:**
- Create: `internal/config/paths.go`, `internal/config/paths_test.go`

- [ ] **Step 1: Write the failing test**

```go
package config

import (
	"strings"
	"testing"
)

func TestConfigDir_ReturnsCanvacliSubdir(t *testing.T) {
	dir, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir: %v", err)
	}
	if !strings.HasSuffix(dir, "canvacli") {
		t.Fatalf("expected suffix canvacli, got %q", dir)
	}
}

func TestTokenPath_HasJSONExtension(t *testing.T) {
	p, err := TokenPath()
	if err != nil {
		t.Fatalf("TokenPath: %v", err)
	}
	if !strings.HasSuffix(p, "token.json") {
		t.Fatalf("expected suffix token.json, got %q", p)
	}
}

func TestCacheDBPath_EndsInDB(t *testing.T) {
	p, err := CacheDBPath()
	if err != nil {
		t.Fatalf("CacheDBPath: %v", err)
	}
	if !strings.HasSuffix(p, "cache.db") {
		t.Fatalf("expected suffix cache.db, got %q", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/config/...
```

Expected: FAIL with "undefined: ConfigDir / TokenPath / CacheDBPath"

- [ ] **Step 3: Implement `paths.go`**

```go
package config

import (
	"os"
	"path/filepath"
)

const appDir = "canvacli"

func ConfigDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, appDir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

func TokenPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "token.json"), nil
}

func CacheDBPath() (string, error) {
	d, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(d, "cache.db"), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
go test ./internal/config/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): add platform-correct config paths"
```

---

## Phase 1 — API Client Foundation

### Task 4: API client base + auth header

**Files:**
- Create: `internal/api/client.go`, `internal/api/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_AddsAuthorizationHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := NewClient("token-abc", WithBaseURL(srv.URL))
	resp, err := c.do(http.MethodGet, "/anything", nil)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	resp.Body.Close()
	if gotAuth != "Bearer token-abc" {
		t.Fatalf("expected Bearer token-abc, got %q", gotAuth)
	}
}

func TestClient_DefaultBaseURL(t *testing.T) {
	c := NewClient("t")
	if c.baseURL != "https://api.canva.com/rest/v1" {
		t.Fatalf("default base URL wrong: %q", c.baseURL)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./internal/api/...
```

Expected: FAIL with undefined symbols.

- [ ] **Step 3: Implement `client.go`**

```go
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"time"
)

const DefaultBaseURL = "https://api.canva.com/rest/v1"

type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

type Option func(*Client)

func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		baseURL: DefaultBaseURL,
		token:   token,
		http:    &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

func (c *Client) do(method, path string, body any) (*http.Response, error) {
	return c.doCtx(context.Background(), method, path, body)
}

func (c *Client) doCtx(ctx context.Context, method, path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.http.Do(req)
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/api/...
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/client.go internal/api/client_test.go
git commit -m "feat(api): add base HTTP client with bearer auth"
```

### Task 5: Typed API errors + exit code mapping

**Files:**
- Create: `internal/api/errors.go`, `internal/api/errors_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"errors"
	"testing"
)

func TestErrAuthRevoked_HasCorrectExitCode(t *testing.T) {
	e := &APIError{Code: "auth_revoked"}
	if e.ExitCode() != 2 {
		t.Fatalf("expected exit 2, got %d", e.ExitCode())
	}
}

func TestErrNotFound_HasCorrectExitCode(t *testing.T) {
	e := &APIError{Code: "not_found"}
	if e.ExitCode() != 3 {
		t.Fatalf("expected exit 3, got %d", e.ExitCode())
	}
}

func TestErrIs_MatchesByCode(t *testing.T) {
	e := error(&APIError{Code: "rate_limited"})
	if !errors.Is(e, ErrRateLimited) {
		t.Fatal("expected errors.Is(..., ErrRateLimited)")
	}
}
```

- [ ] **Step 2: Run test, verify it fails**

```bash
go test ./internal/api/... -run TestErr
```

Expected: FAIL.

- [ ] **Step 3: Implement `errors.go`**

```go
package api

import "errors"

type APIError struct {
	Code         string `json:"error"`
	Message      string `json:"message"`
	HTTPStatus   int    `json:"-"`
	WaitSeconds  int    `json:"wait_seconds,omitempty"`
}

func (e *APIError) Error() string { return e.Code + ": " + e.Message }

func (e *APIError) ExitCode() int {
	switch e.Code {
	case "auth_revoked", "auth_required":
		return 2
	case "not_found", "design_not_found", "template_not_found":
		return 3
	case "network", "api_unavailable":
		return 4
	case "validation", "bad_request":
		return 5
	case "rate_limited":
		return 6
	case "permission_denied", "scope_insufficient":
		return 7
	default:
		return 1
	}
}

var (
	ErrAuthRevoked     = &APIError{Code: "auth_revoked"}
	ErrNotFound        = &APIError{Code: "not_found"}
	ErrRateLimited     = &APIError{Code: "rate_limited"}
	ErrValidation      = &APIError{Code: "validation"}
	ErrPermissionDenied = &APIError{Code: "permission_denied"}
)

func (e *APIError) Is(target error) bool {
	t, ok := target.(*APIError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

var _ error = (*APIError)(nil)
var _ = errors.Is
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/api/... -run TestErr
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/errors.go internal/api/errors_test.go
git commit -m "feat(api): typed error codes with stable exit-code mapping"
```

### Task 6: Async job poller (generic)

**Files:**
- Create: `internal/api/job.go`, `internal/api/job_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

type fakeResult struct {
	URL string `json:"url"`
}

func TestPollJob_SuccessAfterTwoPolls(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if n < 3 {
			json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "j1", "status": "in_progress"}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "j1", "status": "success", "result": map[string]any{"url": "https://x"}}})
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	res, err := PollJob[fakeResult](context.Background(), c, "/jobs/j1", PollOptions{Initial: 10 * time.Millisecond, Max: 50 * time.Millisecond, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("PollJob: %v", err)
	}
	if res.URL != "https://x" {
		t.Fatalf("expected url, got %q", res.URL)
	}
	if calls.Load() != 3 {
		t.Fatalf("expected 3 polls, got %d", calls.Load())
	}
}

func TestPollJob_TimeoutReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"job": map[string]any{"id": "j1", "status": "in_progress"}})
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	_, err := PollJob[fakeResult](context.Background(), c, "/jobs/j1", PollOptions{Initial: 5 * time.Millisecond, Max: 5 * time.Millisecond, Timeout: 50 * time.Millisecond})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/api/... -run TestPollJob
```

Expected: FAIL undefined.

- [ ] **Step 3: Implement `job.go`**

```go
package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type PollOptions struct {
	Initial time.Duration // first interval
	Max     time.Duration // max interval after backoff
	Timeout time.Duration // total wall-clock cap
}

func DefaultPollOptions() PollOptions {
	return PollOptions{
		Initial: 250 * time.Millisecond,
		Max:     5 * time.Second,
		Timeout: 5 * time.Minute,
	}
}

type jobEnvelope[T any] struct {
	Job struct {
		ID     string          `json:"id"`
		Status string          `json:"status"`
		Result T               `json:"result"`
		Error  *APIError       `json:"error,omitempty"`
	} `json:"job"`
}

func PollJob[T any](ctx context.Context, c *Client, path string, opts PollOptions) (T, error) {
	var zero T
	if opts.Initial == 0 {
		opts = DefaultPollOptions()
	}
	deadline := time.Now().Add(opts.Timeout)
	interval := opts.Initial
	for {
		if time.Now().After(deadline) {
			return zero, &APIError{Code: "job_timeout", Message: "job did not complete within timeout"}
		}
		resp, err := c.doCtx(ctx, http.MethodGet, path, nil)
		if err != nil {
			return zero, err
		}
		var env jobEnvelope[T]
		dec := json.NewDecoder(resp.Body)
		decErr := dec.Decode(&env)
		resp.Body.Close()
		if decErr != nil {
			return zero, decErr
		}
		switch env.Job.Status {
		case "success":
			return env.Job.Result, nil
		case "failed":
			if env.Job.Error != nil {
				return zero, env.Job.Error
			}
			return zero, errors.New("job failed without error detail")
		case "in_progress", "pending", "":
			// keep polling
		default:
			return zero, &APIError{Code: "unknown_job_status", Message: env.Job.Status}
		}
		time.Sleep(interval)
		interval *= 2
		if interval > opts.Max {
			interval = opts.Max
		}
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/api/... -run TestPollJob
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/api/job.go internal/api/job_test.go
git commit -m "feat(api): generic async job poller with backoff and timeout"
```

### Task 7: Pagination helper (items + continuation)

**Files:**
- Create: `internal/api/pagination.go`, `internal/api/pagination_test.go`

- [ ] **Step 1: Write the failing test**

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPaginate_FollowsContinuation(t *testing.T) {
	pages := [][]map[string]any{
		{{"id": "1"}, {"id": "2"}},
		{{"id": "3"}},
	}
	cursors := []string{"", "next-1", ""}
	idx := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"items":        pages[idx],
			"continuation": cursors[idx+1],
		})
		idx++
	}))
	defer srv.Close()

	c := NewClient("t", WithBaseURL(srv.URL))
	got := []string{}
	err := Paginate[map[string]any](context.Background(), c, "/things", func(item map[string]any) error {
		got = append(got, item["id"].(string))
		return nil
	})
	if err != nil {
		t.Fatalf("Paginate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 items, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/api/... -run TestPaginate
```

Expected: FAIL.

- [ ] **Step 3: Implement `pagination.go`**

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
)

type pageEnvelope[T any] struct {
	Items        []T    `json:"items"`
	Continuation string `json:"continuation"`
}

func Paginate[T any](ctx context.Context, c *Client, path string, visit func(T) error) error {
	cursor := ""
	for {
		full := path
		if cursor != "" {
			sep := "?"
			if u, _ := url.Parse(path); u != nil && u.RawQuery != "" {
				sep = "&"
			}
			full = path + sep + "continuation=" + url.QueryEscape(cursor)
		}
		resp, err := c.doCtx(ctx, http.MethodGet, full, nil)
		if err != nil {
			return err
		}
		var env pageEnvelope[T]
		decErr := json.NewDecoder(resp.Body).Decode(&env)
		resp.Body.Close()
		if decErr != nil {
			return decErr
		}
		for _, it := range env.Items {
			if err := visit(it); err != nil {
				return err
			}
		}
		if env.Continuation == "" {
			return nil
		}
		cursor = env.Continuation
	}
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/api/... -run TestPaginate
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/api/pagination.go internal/api/pagination_test.go
git commit -m "feat(api): items+continuation pagination helper"
```

### Task 8: Resource-specific API methods (designs, templates, autofill, exports, folders, users)

**Files:**
- Create: `internal/api/designs.go`, `internal/api/templates.go`, `internal/api/autofill.go`, `internal/api/exports.go`, `internal/api/folders.go`, `internal/api/users.go`

Each is small. Group them in one task; each step is one file. Refer to `docs/research/canva-api.md` for the exact endpoint paths and field names.

- [ ] **Step 1: Implement `internal/api/users.go`**

```go
package api

import (
	"context"
	"encoding/json"
	"net/http"
)

type User struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
}

type Profile struct {
	DisplayName string `json:"display_name"`
}

func (c *Client) Me(ctx context.Context) (*User, error) {
	resp, err := c.doCtx(ctx, http.MethodGet, "/users/me", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var env struct {
		Team struct {
			User User `json:"user"`
		} `json:"team"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Team.User, nil
}
```

- [ ] **Step 2: Implement `internal/api/designs.go`**

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type Design struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	URL          string `json:"url"`
	UpdatedAt    int64  `json:"updated_at"`
	Thumbnail    *struct {
		URL string `json:"url"`
	} `json:"thumbnail,omitempty"`
}

func (c *Client) ListDesigns(ctx context.Context, visit func(Design) error) error {
	return Paginate[Design](ctx, c, "/designs", visit)
}

func (c *Client) GetDesign(ctx context.Context, id string) (*Design, error) {
	resp, err := c.doCtx(ctx, http.MethodGet, "/designs/"+id, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &APIError{Code: "design_not_found", Message: fmt.Sprintf("design %q not found", id)}
	}
	var env struct {
		Design Design `json:"design"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env.Design, nil
}
```

- [ ] **Step 3: Implement `internal/api/templates.go`**

```go
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type BrandTemplate struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	UpdatedAt    int64  `json:"updated_at"`
}

type DatasetField struct {
	Type string `json:"type"`
}

type Dataset struct {
	Fields map[string]DatasetField `json:"dataset"`
}

func (c *Client) ListTemplates(ctx context.Context, visit func(BrandTemplate) error) error {
	return Paginate[BrandTemplate](ctx, c, "/brand-templates", visit)
}

func (c *Client) GetTemplateDataset(ctx context.Context, id string) (*Dataset, error) {
	resp, err := c.doCtx(ctx, http.MethodGet, "/brand-templates/"+id+"/dataset", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, &APIError{Code: "template_not_found", Message: fmt.Sprintf("template %q not found", id)}
	}
	var env Dataset
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return &env, nil
}
```

- [ ] **Step 4: Implement `internal/api/autofill.go`**

```go
package api

import (
	"context"
	"net/http"
	"time"
)

type AutofillResult struct {
	Design Design `json:"design"`
}

type AutofillRequest struct {
	BrandTemplateID string         `json:"brand_template_id"`
	Data            map[string]any `json:"data"`
	Title           string         `json:"title,omitempty"`
}

func (c *Client) CreateAutofill(ctx context.Context, req AutofillRequest) (*AutofillResult, error) {
	resp, err := c.doCtx(ctx, http.MethodPost, "/autofills", req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	type submit struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	var s submit
	if err := decodeJSON(resp, &s); err != nil {
		return nil, err
	}
	res, err := PollJob[AutofillResult](ctx, c, "/autofills/"+s.Job.ID, PollOptions{
		Initial: 250 * time.Millisecond,
		Max:     2 * time.Second,
		Timeout: 2 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}
```

(Add `decodeJSON` helper to `client.go`:)

```go
func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}
```

- [ ] **Step 5: Implement `internal/api/exports.go`**

```go
package api

import (
	"context"
	"io"
	"net/http"
	"os"
	"time"
)

type ExportFormat struct {
	Type string `json:"type"` // "pdf", "png", "jpg", "mp4", "gif"
}

type ExportRequest struct {
	DesignID string       `json:"design_id"`
	Format   ExportFormat `json:"format"`
}

type ExportResult struct {
	URLs []string `json:"urls"` // download URLs (24h expiry)
}

func (c *Client) CreateExport(ctx context.Context, req ExportRequest) (*ExportResult, error) {
	resp, err := c.doCtx(ctx, http.MethodPost, "/exports", req)
	if err != nil {
		return nil, err
	}
	type submit struct {
		Job struct {
			ID string `json:"id"`
		} `json:"job"`
	}
	var s submit
	if err := decodeJSON(resp, &s); err != nil {
		return nil, err
	}
	res, err := PollJob[ExportResult](ctx, c, "/exports/"+s.Job.ID, PollOptions{
		Initial: 250 * time.Millisecond,
		Max:     5 * time.Second,
		Timeout: 5 * time.Minute,
	})
	if err != nil {
		return nil, err
	}
	return &res, nil
}

// DownloadTo downloads a single export URL to outPath. Streams to disk
// (does not buffer). Caller decides timeout via ctx.
func (c *Client) DownloadTo(ctx context.Context, urlStr, outPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

var _ = time.Second // keep import used elsewhere if needed
```

- [ ] **Step 6: Implement `internal/api/folders.go`**

```go
package api

import (
	"context"
)

type FolderItem struct {
	Type   string  `json:"type"` // "folder" or "design"
	Folder *Folder `json:"folder,omitempty"`
	Design *Design `json:"design,omitempty"`
}

type Folder struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
}

// WalkFolders walks from "root" and "uploads" special folder IDs and emits
// every folder it encounters via visit. Designs inside folders are not
// emitted (use ListDesigns for that).
func (c *Client) WalkFolders(ctx context.Context, visit func(folder Folder, parentID string) error) error {
	for _, root := range []string{"root", "uploads"} {
		if err := c.walkFolder(ctx, root, "", visit); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) walkFolder(ctx context.Context, id, parentID string, visit func(Folder, string) error) error {
	return Paginate[FolderItem](ctx, c, "/folders/"+id+"/items", func(item FolderItem) error {
		if item.Type != "folder" || item.Folder == nil {
			return nil
		}
		if err := visit(*item.Folder, parentID); err != nil {
			return err
		}
		return c.walkFolder(ctx, item.Folder.ID, item.Folder.ID, visit)
	})
}
```

- [ ] **Step 7: Run tests / build**

```bash
go build ./... && go test ./internal/api/...
```

Expected: build succeeds; existing tests still pass.

- [ ] **Step 8: Commit**

```bash
git add internal/api/
git commit -m "feat(api): add designs, templates, autofill, exports, folders, users"
```

---

## Phase 2 — OAuth & Token Storage

### Task 9: PKCE generators + state token

**Files:**
- Create: `internal/auth/pkce.go`, `internal/auth/pkce_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"strings"
	"testing"
)

func TestNewState_LengthAndCharset(t *testing.T) {
	s := NewState()
	if len(s) < 32 {
		t.Fatalf("state too short: %d", len(s))
	}
	if strings.ContainsAny(s, "+/=") {
		t.Fatalf("state contains non-url-safe chars: %q", s)
	}
}

func TestNewState_Unique(t *testing.T) {
	if NewState() == NewState() {
		t.Fatal("two states collided")
	}
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/auth/... -run TestNewState
```

Expected: FAIL.

- [ ] **Step 3: Implement `pkce.go`**

```go
package auth

import (
	"crypto/rand"
	"encoding/base64"
)

// NewState returns a 32-byte url-safe random state token for CSRF protection.
func NewState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
```

(PKCE verifier/challenge come from `golang.org/x/oauth2.GenerateVerifier()`; no custom code needed here.)

- [ ] **Step 4: Run, verify pass**

```bash
go test ./internal/auth/... -run TestNewState
```

Expected: PASS.

- [ ] **Step 5: Add `golang.org/x/oauth2` dependency**

```bash
go get golang.org/x/oauth2@latest
```

- [ ] **Step 6: Commit**

```bash
git add internal/auth/ go.mod go.sum
git commit -m "feat(auth): add CSRF state generator and oauth2 dependency"
```

### Task 10: Fixed-port callback HTTP listener

**Files:**
- Create: `internal/auth/callback.go`, `internal/auth/callback_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCallbackServer_ReturnsCodeOnValidState(t *testing.T) {
	state := "abc123"
	srv, err := StartCallbackServer(state, []int{8765, 8766, 8767})
	if err != nil {
		t.Fatalf("StartCallbackServer: %v", err)
	}
	defer srv.Close()

	go func() {
		// hit the local callback URL with code+state
		v := url.Values{"code": {"the-code"}, "state": {state}}
		http.Get(srv.RedirectURI() + "?" + v.Encode())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	code, err := srv.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if code != "the-code" {
		t.Fatalf("expected code, got %q", code)
	}
	if !strings.HasPrefix(srv.RedirectURI(), "http://127.0.0.1:") {
		t.Fatalf("unexpected redirect URI: %q", srv.RedirectURI())
	}
	_ = io.Discard
}

func TestCallbackServer_RejectsBadState(t *testing.T) {
	srv, err := StartCallbackServer("expected", []int{8765, 8766, 8767})
	if err != nil {
		t.Fatalf("StartCallbackServer: %v", err)
	}
	defer srv.Close()

	go func() {
		v := url.Values{"code": {"the-code"}, "state": {"WRONG"}}
		http.Get(srv.RedirectURI() + "?" + v.Encode())
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, err := srv.Wait(ctx); err == nil {
		t.Fatal("expected error from bad state, got nil")
	}
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/auth/... -run TestCallback
```

Expected: FAIL undefined.

- [ ] **Step 3: Implement `callback.go`**

```go
package auth

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

type CallbackServer struct {
	listener net.Listener
	server   *http.Server
	codeCh   chan string
	errCh    chan error
	state    string
	port     int
}

func StartCallbackServer(state string, ports []int) (*CallbackServer, error) {
	var ln net.Listener
	var port int
	var err error
	for _, p := range ports {
		ln, err = net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			port = p
			break
		}
	}
	if ln == nil {
		return nil, fmt.Errorf("no callback port free in %v: %w", ports, err)
	}

	cs := &CallbackServer{
		listener: ln,
		state:    state,
		port:     port,
		codeCh:   make(chan string, 1),
		errCh:    make(chan error, 1),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", cs.handle)
	cs.server = &http.Server{Handler: mux}

	go cs.server.Serve(ln)
	return cs, nil
}

func (cs *CallbackServer) RedirectURI() string {
	return fmt.Sprintf("http://127.0.0.1:%d/callback", cs.port)
}

func (cs *CallbackServer) handle(w http.ResponseWriter, r *http.Request) {
	gotState := r.URL.Query().Get("state")
	if gotState != cs.state {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		select {
		case cs.errCh <- errors.New("oauth state mismatch"):
		default:
		}
		return
	}
	if errStr := r.URL.Query().Get("error"); errStr != "" {
		http.Error(w, errStr, http.StatusBadRequest)
		select {
		case cs.errCh <- fmt.Errorf("oauth error: %s", errStr):
		default:
		}
		return
	}
	code := r.URL.Query().Get("code")
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintln(w, `<html><body><h1>canvacli connected</h1><p>You can close this tab and return to your terminal.</p></body></html>`)
	select {
	case cs.codeCh <- code:
	default:
	}
}

func (cs *CallbackServer) Wait(ctx context.Context) (string, error) {
	select {
	case code := <-cs.codeCh:
		return code, nil
	case err := <-cs.errCh:
		return "", err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (cs *CallbackServer) Close() {
	cs.server.Close()
	cs.listener.Close()
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -run TestCallback
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/callback.go internal/auth/callback_test.go
git commit -m "feat(auth): callback server with fixed-port fallback and CSRF check"
```

### Task 11: Atomic token storage

**Files:**
- Create: `internal/auth/token.go`, `internal/auth/token_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestSaveAndLoadToken_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	tok := &oauth2.Token{
		AccessToken:  "a",
		RefreshToken: "r",
		Expiry:       time.Now().Add(1 * time.Hour).Round(time.Second),
		TokenType:    "Bearer",
	}
	if err := SaveToken(path, tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	got, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if got.AccessToken != "a" || got.RefreshToken != "r" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestSaveToken_Permissions0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissions on windows are a no-op")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	tok := &oauth2.Token{AccessToken: "a"}
	if err := SaveToken(path, tok); err != nil {
		t.Fatalf("SaveToken: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600, got %o", info.Mode().Perm())
	}
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/auth/... -run TestSave
```

Expected: FAIL.

- [ ] **Step 3: Implement `token.go`**

```go
package auth

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/oauth2"
)

func SaveToken(path string, tok *oauth2.Token) error {
	if tok == nil {
		return errors.New("nil token")
	}
	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".token-*")
	if err != nil {
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func LoadToken(path string) (*oauth2.Token, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tok oauth2.Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, err
	}
	return &tok, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/auth/... -run TestSave
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/auth/token.go internal/auth/token_test.go
git commit -m "feat(auth): atomic token storage with 0600 perms"
```

### Task 12: Persisting token source + 401 retry transport

**Files:**
- Create: `internal/auth/source.go`, `internal/auth/transport.go`, `internal/auth/source_test.go`

- [ ] **Step 1: Write the failing test**

```go
package auth

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

type stubSource struct {
	tok *oauth2.Token
	err error
}

func (s *stubSource) Token() (*oauth2.Token, error) { return s.tok, s.err }

func TestPersistingSource_WritesOnRefresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	first := &oauth2.Token{AccessToken: "old", RefreshToken: "rt", Expiry: time.Now().Add(time.Hour)}
	if err := SaveToken(path, first); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	upstream := &stubSource{tok: &oauth2.Token{AccessToken: "new", RefreshToken: "rt2", Expiry: time.Now().Add(time.Hour)}}

	ps := NewPersistingSource(upstream, path)
	tok, err := ps.Token()
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if tok.AccessToken != "new" {
		t.Fatalf("expected new token, got %q", tok.AccessToken)
	}
	loaded, err := LoadToken(path)
	if err != nil {
		t.Fatalf("LoadToken: %v", err)
	}
	if loaded.AccessToken != "new" || loaded.RefreshToken != "rt2" {
		t.Fatalf("disk not updated: %+v", loaded)
	}
	_ = context.Background
}
```

- [ ] **Step 2: Run, verify failure**

```bash
go test ./internal/auth/... -run TestPersistingSource
```

Expected: FAIL.

- [ ] **Step 3: Implement `source.go`**

```go
package auth

import (
	"sync"

	"golang.org/x/oauth2"
)

type PersistingSource struct {
	mu       sync.Mutex
	upstream oauth2.TokenSource
	path     string
	last     string // last access token persisted
}

func NewPersistingSource(upstream oauth2.TokenSource, path string) *PersistingSource {
	return &PersistingSource{upstream: upstream, path: path}
}

func (p *PersistingSource) Token() (*oauth2.Token, error) {
	tok, err := p.upstream.Token()
	if err != nil {
		return nil, err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if tok.AccessToken != p.last {
		if saveErr := SaveToken(p.path, tok); saveErr == nil {
			p.last = tok.AccessToken
		}
	}
	return tok, nil
}
```

- [ ] **Step 4: Implement `transport.go`**

```go
package auth

import (
	"bytes"
	"io"
	"net/http"
)

// RefreshOn401Transport wraps an underlying transport. On a 401, it forces
// a refresh from the source and replays the request once.
type RefreshOn401Transport struct {
	Base   http.RoundTripper
	Source *PersistingSource
}

func (t *RefreshOn401Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Read body so we can replay if needed.
	var body []byte
	if req.Body != nil && req.GetBody == nil {
		var err error
		body, err = io.ReadAll(req.Body)
		req.Body.Close()
		if err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	resp, err := t.Base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		return resp, err
	}
	resp.Body.Close()

	// Force a refresh by invalidating last-cached token (caller's source decides).
	tok, err := t.Source.Token()
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	if req.GetBody != nil {
		b, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		clone.Body = b
	}
	clone.Header.Set("Authorization", "Bearer "+tok.AccessToken)
	return t.Base.RoundTrip(clone)
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/auth/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth/
git commit -m "feat(auth): persisting token source and 401-retry transport"
```

### Task 13: Browser opener (cross-platform)

**Files:**
- Create: `internal/auth/browser.go`

- [ ] **Step 1: Implement (no unit test — exec is hard to mock and the surface is small)**

```go
package auth

import (
	"os/exec"
	"runtime"
)

// OpenBrowser opens the default browser at the given URL. Returns nil if
// the command was launched (no guarantee the user actually saw it).
func OpenBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}
```

- [ ] **Step 2: Build to confirm compilation**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/auth/browser.go
git commit -m "feat(auth): cross-platform browser opener"
```

---

## Phase 3 — Cache & Read-Only SQL

### Task 14: SQLite cache schema + db open

**Files:**
- Create: `internal/cache/db.go`, `internal/cache/db_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cache

import (
	"path/filepath"
	"testing"
)

func TestOpen_CreatesAllTables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	wantTables := []string{"designs", "templates", "folders", "idempotency", "meta"}
	for _, tbl := range wantTables {
		var n int
		row := db.DB().QueryRow("SELECT count(*) FROM " + tbl)
		if err := row.Scan(&n); err != nil {
			t.Fatalf("table %q missing: %v", tbl, err)
		}
	}
}
```

- [ ] **Step 2: Add modernc/sqlite dependency**

```bash
go get modernc.org/sqlite@latest
```

- [ ] **Step 3: Implement `db.go`**

```go
package cache

import (
	"database/sql"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS designs (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  folder_id TEXT,
  updated_at INTEGER NOT NULL,
  fetched_at INTEGER NOT NULL,
  thumbnail_url TEXT,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS templates (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  raw_json TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS folders (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  parent_id TEXT,
  fetched_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS idempotency (
  key TEXT PRIMARY KEY,
  command TEXT NOT NULL,
  args_hash TEXT NOT NULL,
  result_json TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_designs_title ON designs(title);
CREATE INDEX IF NOT EXISTS idx_templates_title ON templates(title);
CREATE INDEX IF NOT EXISTS idx_idempotency_created ON idempotency(created_at);
`

type Cache struct {
	db *sql.DB
}

func Open(path string) (*Cache, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(2000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Cache{db: db}, nil
}

func (c *Cache) DB() *sql.DB { return c.db }
func (c *Cache) Close() error { return c.db.Close() }
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/cache/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cache/ go.mod go.sum
git commit -m "feat(cache): sqlite open with schema migration"
```

### Task 15: Cache CRUD for designs, templates, folders, idempotency

**Files:**
- Create: `internal/cache/designs.go`, `internal/cache/templates.go`, `internal/cache/folders.go`, `internal/cache/idempotency.go`, `internal/cache/crud_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cache

import (
	"path/filepath"
	"testing"
	"time"
)

func TestUpsertAndFindDesign(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	now := time.Now().Unix()
	if err := c.UpsertDesign(Design{ID: "d1", Title: "Q3 Banner", UpdatedAt: now, FetchedAt: now, RawJSON: "{}"}); err != nil {
		t.Fatalf("UpsertDesign: %v", err)
	}
	got, err := c.FindDesignByName("q3 banner")
	if err != nil {
		t.Fatalf("FindDesignByName: %v", err)
	}
	if len(got) != 1 || got[0].ID != "d1" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestIdempotencyLookup(t *testing.T) {
	c, err := Open(filepath.Join(t.TempDir(), "c.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer c.Close()
	if err := c.SaveIdempotency("k1", "create", "h", `{"id":"abc"}`); err != nil {
		t.Fatalf("SaveIdempotency: %v", err)
	}
	got, err := c.LookupIdempotency("k1", "h")
	if err != nil {
		t.Fatalf("LookupIdempotency: %v", err)
	}
	if got != `{"id":"abc"}` {
		t.Fatalf("got %q", got)
	}
}
```

- [ ] **Step 2: Implement `designs.go`**

```go
package cache

import "time"

type Design struct {
	ID           string
	Title        string
	FolderID     string
	UpdatedAt    int64
	FetchedAt    int64
	ThumbnailURL string
	RawJSON      string
}

func (c *Cache) UpsertDesign(d Design) error {
	_, err := c.db.Exec(`
		INSERT INTO designs (id, title, folder_id, updated_at, fetched_at, thumbnail_url, raw_json)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  title=excluded.title,
		  folder_id=excluded.folder_id,
		  updated_at=excluded.updated_at,
		  fetched_at=excluded.fetched_at,
		  thumbnail_url=excluded.thumbnail_url,
		  raw_json=excluded.raw_json
	`, d.ID, d.Title, d.FolderID, d.UpdatedAt, d.FetchedAt, d.ThumbnailURL, d.RawJSON)
	return err
}

func (c *Cache) FindDesignByID(id string) (*Design, error) {
	row := c.db.QueryRow(`SELECT id, title, folder_id, updated_at, fetched_at, thumbnail_url, raw_json FROM designs WHERE id = ?`, id)
	var d Design
	err := row.Scan(&d.ID, &d.Title, &d.FolderID, &d.UpdatedAt, &d.FetchedAt, &d.ThumbnailURL, &d.RawJSON)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *Cache) FindDesignByName(name string) ([]Design, error) {
	rows, err := c.db.Query(`SELECT id, title, folder_id, updated_at, fetched_at, thumbnail_url, raw_json FROM designs WHERE title = ? COLLATE NOCASE LIMIT 2`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Design
	for rows.Next() {
		var d Design
		if err := rows.Scan(&d.ID, &d.Title, &d.FolderID, &d.UpdatedAt, &d.FetchedAt, &d.ThumbnailURL, &d.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	_ = time.Now // keep import
	return out, nil
}
```

- [ ] **Step 3: Implement `templates.go`**

```go
package cache

type Template struct {
	ID        string
	Title     string
	FetchedAt int64
	RawJSON   string
}

func (c *Cache) UpsertTemplate(t Template) error {
	_, err := c.db.Exec(`
		INSERT INTO templates (id, title, fetched_at, raw_json)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  title=excluded.title,
		  fetched_at=excluded.fetched_at,
		  raw_json=excluded.raw_json
	`, t.ID, t.Title, t.FetchedAt, t.RawJSON)
	return err
}

func (c *Cache) FindTemplateByID(id string) (*Template, error) {
	row := c.db.QueryRow(`SELECT id, title, fetched_at, raw_json FROM templates WHERE id = ?`, id)
	var t Template
	if err := row.Scan(&t.ID, &t.Title, &t.FetchedAt, &t.RawJSON); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Cache) FindTemplateByName(name string) ([]Template, error) {
	rows, err := c.db.Query(`SELECT id, title, fetched_at, raw_json FROM templates WHERE title = ? COLLATE NOCASE LIMIT 2`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Template
	for rows.Next() {
		var t Template
		if err := rows.Scan(&t.ID, &t.Title, &t.FetchedAt, &t.RawJSON); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}
```

- [ ] **Step 4: Implement `folders.go`**

```go
package cache

type Folder struct {
	ID        string
	Name      string
	ParentID  string
	FetchedAt int64
}

func (c *Cache) UpsertFolder(f Folder) error {
	_, err := c.db.Exec(`
		INSERT INTO folders (id, name, parent_id, fetched_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
		  name=excluded.name,
		  parent_id=excluded.parent_id,
		  fetched_at=excluded.fetched_at
	`, f.ID, f.Name, f.ParentID, f.FetchedAt)
	return err
}
```

- [ ] **Step 5: Implement `idempotency.go`**

```go
package cache

import "time"

func (c *Cache) SaveIdempotency(key, command, argsHash, resultJSON string) error {
	_, err := c.db.Exec(`
		INSERT INTO idempotency (key, command, args_hash, result_json, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
		  command=excluded.command,
		  args_hash=excluded.args_hash,
		  result_json=excluded.result_json,
		  created_at=excluded.created_at
	`, key, command, argsHash, resultJSON, time.Now().Unix())
	return err
}

func (c *Cache) LookupIdempotency(key, argsHash string) (string, error) {
	row := c.db.QueryRow(`SELECT result_json FROM idempotency WHERE key = ? AND args_hash = ?`, key, argsHash)
	var v string
	if err := row.Scan(&v); err != nil {
		return "", err
	}
	return v, nil
}

// PruneOldIdempotency removes entries older than the given duration.
func (c *Cache) PruneOldIdempotency(maxAge time.Duration) error {
	cutoff := time.Now().Add(-maxAge).Unix()
	_, err := c.db.Exec(`DELETE FROM idempotency WHERE created_at < ?`, cutoff)
	return err
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/cache/...
```

Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/cache/
git commit -m "feat(cache): CRUD for designs, templates, folders, idempotency"
```

### Task 16: Read-only SQL handler with parser allowlist

**Files:**
- Create: `internal/cache/sql.go`, `internal/cache/sql_test.go`

- [ ] **Step 1: Write the failing test**

```go
package cache

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestExecReadOnly_AllowsSelect(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	rows, err := c.ExecReadOnly("SELECT 1 AS x", 100)
	if err != nil {
		t.Fatalf("ExecReadOnly: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
}

func TestExecReadOnly_RejectsInsert(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("INSERT INTO meta (key, value) VALUES ('x','y')", 100)
	if err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("expected read-only error, got %v", err)
	}
}

func TestExecReadOnly_RejectsAttach(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("ATTACH DATABASE 'foo.db' AS f", 100)
	if err == nil {
		t.Fatal("expected error on ATTACH")
	}
}

func TestExecReadOnly_RejectsMultipleStatements(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	_, err := c.ExecReadOnly("SELECT 1; SELECT 2", 100)
	if err == nil {
		t.Fatal("expected error on multiple statements")
	}
}

func TestExecReadOnly_AppliesLimit(t *testing.T) {
	c, _ := Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	for i := 0; i < 10; i++ {
		c.UpsertDesign(Design{ID: string(rune('a' + i)), Title: "x", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	}
	rows, err := c.ExecReadOnly("SELECT id FROM designs", 3)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected limit 3, got %d", len(rows))
	}
}
```

- [ ] **Step 2: Implement `sql.go`**

```go
package cache

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	allowedPrefix = regexp.MustCompile(`(?i)^\s*(WITH\s|SELECT\s)`)
	forbidden     = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|REPLACE|CREATE|DROP|ALTER|ATTACH|DETACH|PRAGMA|VACUUM|REINDEX)\b`)
)

// ExecReadOnly runs a single SELECT (or WITH ... SELECT) and returns up to
// limit rows as []map[string]any. Rejects multi-statement input, mutating
// statements, and ATTACH/PRAGMA. Enforces a 5-second timeout.
func (c *Cache) ExecReadOnly(query string, limit int) ([]map[string]any, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimSuffix(q, ";")
	if strings.Contains(q, ";") {
		return nil, errors.New("multiple statements not allowed")
	}
	if !allowedPrefix.MatchString(q) {
		return nil, errors.New("read-only: only SELECT and WITH...SELECT are permitted")
	}
	if forbidden.MatchString(q) {
		return nil, errors.New("read-only: mutating or restricted keyword detected")
	}
	if limit <= 0 {
		limit = 500
	}
	wrapped := fmt.Sprintf("SELECT * FROM (%s) LIMIT %d", q, limit)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, err := c.db.QueryContext(ctx, wrapped)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, name := range cols {
			row[name] = vals[i]
		}
		out = append(out, row)
	}
	return out, rows.Err()
}
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/cache/... -run TestExecReadOnly
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/cache/sql.go internal/cache/sql_test.go
git commit -m "feat(cache): read-only SQL handler with allowlist and limit"
```

---

## Phase 4 — Resolver & Output

### Task 17: Resolver (cache → API fallback)

**Files:**
- Create: `internal/resolver/resolver.go`, `internal/resolver/resolver_test.go`

- [ ] **Step 1: Write the failing test**

```go
package resolver

import (
	"path/filepath"
	"testing"

	"github.com/catalinlongevai/canvacli/internal/cache"
)

func TestResolveDesign_ByID_HitsCache(t *testing.T) {
	c, _ := cache.Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	c.UpsertDesign(cache.Design{ID: "abc", Title: "X", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	r := New(c, nil)
	id, err := r.ResolveDesign("abc")
	if err != nil || id != "abc" {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}

func TestResolveDesign_ByName_OneMatch(t *testing.T) {
	c, _ := cache.Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	c.UpsertDesign(cache.Design{ID: "d1", Title: "Q3 Banner", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	r := New(c, nil)
	id, err := r.ResolveDesign("Q3 Banner")
	if err != nil || id != "d1" {
		t.Fatalf("got id=%q err=%v", id, err)
	}
}

func TestResolveDesign_AmbiguousReturnsError(t *testing.T) {
	c, _ := cache.Open(filepath.Join(t.TempDir(), "c.db"))
	defer c.Close()
	c.UpsertDesign(cache.Design{ID: "d1", Title: "Banner", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	c.UpsertDesign(cache.Design{ID: "d2", Title: "Banner", UpdatedAt: 1, FetchedAt: 1, RawJSON: "{}"})
	r := New(c, nil)
	_, err := r.ResolveDesign("Banner")
	if err == nil {
		t.Fatal("expected ambiguity error")
	}
}
```

- [ ] **Step 2: Implement `resolver.go`**

```go
package resolver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
)

type Resolver struct {
	cache *cache.Cache
	api   *api.Client
}

func New(c *cache.Cache, a *api.Client) *Resolver {
	return &Resolver{cache: c, api: a}
}

// ResolveDesign returns a design ID for `query`. Tries cache by ID, then
// cache by exact-title (case-insensitive), then API by listing.
func (r *Resolver) ResolveDesign(query string) (string, error) {
	if d, err := r.cache.FindDesignByID(query); err == nil && d != nil {
		return d.ID, nil
	}
	matches, err := r.cache.FindDesignByName(query)
	if err == nil && len(matches) == 1 {
		return matches[0].ID, nil
	}
	if len(matches) > 1 {
		return "", ambiguity("design", matches[0].ID, matches[1].ID)
	}
	if r.api == nil {
		return "", &NotFoundError{Resource: "design", Query: query}
	}
	// API fallback: list designs and match by ID or title
	var hit *string
	multi := []string{}
	err = r.api.ListDesigns(context.Background(), func(d api.Design) error {
		if d.ID == query || strings.EqualFold(d.Title, query) {
			id := d.ID
			if hit == nil {
				hit = &id
			} else {
				multi = append(multi, id)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if hit == nil {
		return "", &NotFoundError{Resource: "design", Query: query}
	}
	if len(multi) > 0 {
		return "", ambiguity("design", *hit, multi...)
	}
	return *hit, nil
}

// ResolveTemplate is the same shape, against templates.
func (r *Resolver) ResolveTemplate(query string) (string, error) {
	if t, err := r.cache.FindTemplateByID(query); err == nil && t != nil {
		return t.ID, nil
	}
	matches, err := r.cache.FindTemplateByName(query)
	if err == nil && len(matches) == 1 {
		return matches[0].ID, nil
	}
	if len(matches) > 1 {
		return "", ambiguity("template", matches[0].ID, matches[1].ID)
	}
	if r.api == nil {
		return "", &NotFoundError{Resource: "template", Query: query}
	}
	var hit *string
	multi := []string{}
	err = r.api.ListTemplates(context.Background(), func(t api.BrandTemplate) error {
		if t.ID == query || strings.EqualFold(t.Title, query) {
			id := t.ID
			if hit == nil {
				hit = &id
			} else {
				multi = append(multi, id)
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	if hit == nil {
		return "", &NotFoundError{Resource: "template", Query: query}
	}
	if len(multi) > 0 {
		return "", ambiguity("template", *hit, multi...)
	}
	return *hit, nil
}

type NotFoundError struct {
	Resource string
	Query    string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("%s %q not found", e.Resource, e.Query)
}

type AmbiguityError struct {
	Resource    string
	Suggestions []string
}

func (e *AmbiguityError) Error() string {
	return fmt.Sprintf("%s name matched multiple: %v", e.Resource, e.Suggestions)
}

func ambiguity(resource string, ids ...string) error {
	return &AmbiguityError{Resource: resource, Suggestions: ids}
}

var _ = errors.New
```

- [ ] **Step 3: Run tests**

```bash
go test ./internal/resolver/...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/resolver/
git commit -m "feat(resolver): name-or-ID resolution with cache and API fallback"
```

### Task 18: Output formatters (TTY detection, JSON/NDJSON, errors, fields)

**Files:**
- Create: `internal/output/tty.go`, `internal/output/json.go`, `internal/output/errors.go`, `internal/output/fields.go`, `internal/output/output_test.go`

- [ ] **Step 1: Implement `tty.go`**

```go
package output

import (
	"os"

	"golang.org/x/term"
)

func IsTTY(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}

func StdoutIsTTY() bool { return IsTTY(os.Stdout) }
```

- [ ] **Step 2: Add `golang.org/x/term`**

```bash
go get golang.org/x/term@latest
```

- [ ] **Step 3: Implement `json.go`**

```go
package output

import (
	"encoding/json"
	"io"
)

// EmitJSON writes a single JSON object to w (compact).
func EmitJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// EmitNDJSON writes one item per line, compact.
func EmitNDJSON(w io.Writer, items []map[string]any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	for _, it := range items {
		if err := enc.Encode(it); err != nil {
			return err
		}
	}
	return nil
}
```

- [ ] **Step 4: Implement `errors.go`**

```go
package output

import (
	"encoding/json"
	"io"
	"os"
)

type ErrorEnvelope struct {
	Error       string   `json:"error"`
	Message     string   `json:"message,omitempty"`
	Suggestions []string `json:"suggestions,omitempty"`
	Fix         string   `json:"fix,omitempty"`
	WaitSeconds int      `json:"wait_seconds,omitempty"`
	ExitCode    int      `json:"exit_code"`
}

// EmitError prints a structured error envelope and returns the appropriate
// exit code. Fix is selected by code from a static map (never from upstream
// data) — defends against prompt injection.
func EmitError(w io.Writer, code, message string, suggestions []string) int {
	env := ErrorEnvelope{
		Error:       code,
		Message:     message,
		Suggestions: suggestions,
		Fix:         fixForCode(code),
		ExitCode:    exitCodeFor(code),
	}
	b, _ := json.Marshal(env)
	_, _ = w.Write(append(b, '\n'))
	return env.ExitCode
}

func fixForCode(code string) string {
	switch code {
	case "auth_revoked", "auth_required":
		return "canva login"
	case "design_not_found":
		return "canva list --json | jq '.title'"
	case "template_not_found":
		return "canva templates --json"
	case "rate_limited":
		return "retry after wait_seconds (or pass --auto-wait)"
	case "permission_denied":
		return "verify your account has the required scopes via canva whoami"
	case "not_found":
		return ""
	default:
		return ""
	}
}

func exitCodeFor(code string) int {
	switch code {
	case "auth_revoked", "auth_required":
		return 2
	case "design_not_found", "template_not_found", "not_found":
		return 3
	case "network", "api_unavailable":
		return 4
	case "validation", "bad_request":
		return 5
	case "rate_limited":
		return 6
	case "permission_denied", "scope_insufficient":
		return 7
	default:
		return 1
	}
}

var _ = os.Stderr
```

- [ ] **Step 5: Implement `fields.go`**

```go
package output

import "strings"

// ProjectFields returns a copy of m containing only the requested keys.
// "all" returns m as-is.
func ProjectFields(m map[string]any, fields string) map[string]any {
	if fields == "" || fields == "all" {
		return m
	}
	keep := map[string]bool{}
	for _, k := range strings.Split(fields, ",") {
		keep[strings.TrimSpace(k)] = true
	}
	out := make(map[string]any, len(keep))
	for k := range keep {
		if v, ok := m[k]; ok {
			out[k] = v
		}
	}
	return out
}
```

- [ ] **Step 6: Write tests**

```go
package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestEmitJSON_Compact(t *testing.T) {
	var buf bytes.Buffer
	if err := EmitJSON(&buf, map[string]any{"a": 1, "b": "x"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "\n  ") {
		t.Fatalf("output contains pretty-printing: %q", buf.String())
	}
}

func TestEmitError_AuthRevoked_Exit2(t *testing.T) {
	var buf bytes.Buffer
	got := EmitError(&buf, "auth_revoked", "expired", nil)
	if got != 2 {
		t.Fatalf("expected exit 2, got %d", got)
	}
	if !strings.Contains(buf.String(), `"fix":"canva login"`) {
		t.Fatalf("missing fix: %q", buf.String())
	}
}

func TestProjectFields_Filters(t *testing.T) {
	got := ProjectFields(map[string]any{"a": 1, "b": 2, "c": 3}, "a,c")
	if _, has := got["b"]; has {
		t.Fatal("b should be filtered out")
	}
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/output/... go.mod go.sum
```

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/output/ go.mod go.sum
git commit -m "feat(output): TTY detection, JSON/NDJSON, error envelope, field projection"
```

---

## Phase 5 — Cobra Skeleton + Auth Commands

### Task 19: Cobra root command + global flags

**Files:**
- Create: `internal/commands/root.go`
- Modify: `cmd/canvacli/main.go`

- [ ] **Step 1: Add cobra dependency**

```bash
go get github.com/spf13/cobra@latest
```

- [ ] **Step 2: Implement `internal/commands/root.go`**

```go
package commands

import (
	"github.com/spf13/cobra"
)

var (
	flagJSON    bool
	flagNoCache bool
	flagQuiet   bool
	flagAutoWait bool
)

func NewRoot(version, commit, date string) *cobra.Command {
	root := &cobra.Command{
		Use:   "canva",
		Short: "Agent-first CLI for the Canva Connect API",
		Version: version + " (commit " + commit + ", built " + date + ")",
	}
	root.PersistentFlags().BoolVar(&flagJSON, "json", false, "force JSON output (auto-on when piped)")
	root.PersistentFlags().BoolVar(&flagNoCache, "no-cache", false, "bypass local cache, force API call")
	root.PersistentFlags().BoolVar(&flagQuiet, "quiet", false, "suppress progress output")
	root.PersistentFlags().BoolVar(&flagAutoWait, "auto-wait", false, "auto-retry on 429 once, capped at 60s")

	root.AddCommand(NewLogin())
	root.AddCommand(NewLogout())
	root.AddCommand(NewWhoami())
	root.AddCommand(NewTemplates())
	root.AddCommand(NewCreate())
	root.AddCommand(NewList())
	root.AddCommand(NewExport())
	root.AddCommand(NewFolders())
	root.AddCommand(NewSchema())
	root.AddCommand(NewSQL())

	return root
}
```

- [ ] **Step 3: Replace `cmd/canvacli/main.go`**

```go
package main

import (
	"fmt"
	"os"

	"github.com/catalinlongevai/canvacli/internal/commands"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	if err := commands.NewRoot(version, commit, date).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
```

- [ ] **Step 4: Stub the missing command constructors**

(Create temporary `internal/commands/_stubs.go` with empty `cobra.Command` constructors so the build compiles. Each will be replaced by its dedicated task.)

```go
package commands

import "github.com/spf13/cobra"

func NewLogin() *cobra.Command     { return &cobra.Command{Use: "login", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewLogout() *cobra.Command    { return &cobra.Command{Use: "logout", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewWhoami() *cobra.Command    { return &cobra.Command{Use: "whoami", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewTemplates() *cobra.Command { return &cobra.Command{Use: "templates", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewCreate() *cobra.Command    { return &cobra.Command{Use: "create", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewList() *cobra.Command      { return &cobra.Command{Use: "list", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewExport() *cobra.Command    { return &cobra.Command{Use: "export", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewFolders() *cobra.Command   { return &cobra.Command{Use: "folders", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewSchema() *cobra.Command    { return &cobra.Command{Use: "schema", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
func NewSQL() *cobra.Command       { return &cobra.Command{Use: "sql", Hidden: true, RunE: func(*cobra.Command, []string) error { return nil }} }
```

- [ ] **Step 5: Build + smoke test**

```bash
go build -o canvacli ./cmd/canvacli && ./canvacli --version && ./canvacli --help
```

Expected: version prints; help lists all 10 subcommands.

- [ ] **Step 6: Commit**

```bash
git add cmd/canvacli/main.go internal/commands/ go.mod go.sum
git commit -m "feat(commands): cobra skeleton with stubbed commands and global flags"
```

### Task 20: `canva login`

**Files:**
- Modify: `internal/commands/login.go` (replace stub)

- [ ] **Step 1: Implement `internal/commands/login.go`**

(Note: client_id and client_secret are read from env vars `CANVA_CLIENT_ID` and `CANVA_CLIENT_SECRET` for now. A future task — out of v1 scope — will embed these in the binary at build time via ldflags.)

```go
package commands

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/auth"
	"github.com/catalinlongevai/canvacli/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
)

const (
	canvaAuthURL  = "https://www.canva.com/api/oauth/authorize"
	canvaTokenURL = "https://api.canva.com/rest/v1/oauth/token"
)

var canvaScopes = []string{
	"design:meta:read",
	"design:content:read",
	"design:content:write",
	"brandtemplate:meta:read",
	"brandtemplate:content:read",
	"folder:read",
	"profile:read",
}

func NewLogin() *cobra.Command {
	return &cobra.Command{
		Use:   "login",
		Short: "Authenticate with Canva via OAuth 2.0 PKCE",
		RunE: func(cmd *cobra.Command, _ []string) error {
			clientID := os.Getenv("CANVA_CLIENT_ID")
			clientSecret := os.Getenv("CANVA_CLIENT_SECRET")
			if clientID == "" || clientSecret == "" {
				return errors.New("CANVA_CLIENT_ID and CANVA_CLIENT_SECRET must be set (developer app credentials)")
			}

			state := auth.NewState()
			cb, err := auth.StartCallbackServer(state, []int{8765, 8766, 8767})
			if err != nil {
				return err
			}
			defer cb.Close()

			conf := &oauth2.Config{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				Endpoint: oauth2.Endpoint{
					AuthURL:  canvaAuthURL,
					TokenURL: canvaTokenURL,
				},
				RedirectURL: cb.RedirectURI(),
				Scopes:      canvaScopes,
			}

			verifier := oauth2.GenerateVerifier()
			authURL := conf.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier))

			fmt.Fprintln(os.Stderr, "Opening browser to authorize canvacli...")
			fmt.Fprintln(os.Stderr, "If it doesn't open, visit:", authURL)
			_ = auth.OpenBrowser(authURL)

			ctx, cancel := context.WithTimeout(cmd.Context(), 5*time.Minute)
			defer cancel()
			code, err := cb.Wait(ctx)
			if err != nil {
				return fmt.Errorf("oauth callback: %w", err)
			}

			tok, err := conf.Exchange(ctx, code, oauth2.VerifierOption(verifier))
			if err != nil {
				return fmt.Errorf("token exchange: %w", err)
			}
			path, err := config.TokenPath()
			if err != nil {
				return err
			}
			if err := auth.SaveToken(path, tok); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "Logged in. Token stored at", path)
			return nil
		},
	}
}
```

- [ ] **Step 2: Build**

```bash
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add internal/commands/login.go internal/commands/_stubs.go
git commit -m "feat(login): OAuth 2.0 PKCE flow with browser handoff"
```

(NOTE: `_stubs.go` should drop `NewLogin` since it's now defined in `login.go`. The rest of the stubs remain until later tasks replace them.)

### Task 21: `canva logout`

**Files:**
- Create: `internal/commands/logout.go`

- [ ] **Step 1: Implement**

```go
package commands

import (
	"errors"
	"fmt"
	"os"

	"github.com/catalinlongevai/canvacli/internal/config"
	"github.com/spf13/cobra"
)

func NewLogout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials and clear cache",
		RunE: func(cmd *cobra.Command, _ []string) error {
			tokPath, err := config.TokenPath()
			if err != nil {
				return err
			}
			cachePath, err := config.CacheDBPath()
			if err != nil {
				return err
			}
			for _, p := range []string{tokPath, cachePath} {
				if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
					return fmt.Errorf("remove %s: %w", p, err)
				}
			}
			fmt.Fprintln(os.Stderr, "Logged out. Local credentials and cache cleared.")
			return nil
		},
	}
}
```

(Remove `NewLogout` from `_stubs.go`.)

- [ ] **Step 2: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(logout): remove tokens and cache"
```

### Task 22: `canva whoami`

**Files:**
- Create: `internal/commands/whoami.go`
- Create helper: `internal/commands/clientutil.go` (used by every command from here on)

- [ ] **Step 1: Implement `clientutil.go`**

```go
package commands

import (
	"context"
	"errors"
	"net/http"
	"os"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/auth"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/config"
	"golang.org/x/oauth2"
)

func loadClient(ctx context.Context) (*api.Client, error) {
	tokPath, err := config.TokenPath()
	if err != nil {
		return nil, err
	}
	tok, err := auth.LoadToken(tokPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, errors.New("auth_required: run `canva login` first")
		}
		return nil, err
	}
	clientID := os.Getenv("CANVA_CLIENT_ID")
	clientSecret := os.Getenv("CANVA_CLIENT_SECRET")
	conf := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     oauth2.Endpoint{TokenURL: canvaTokenURL},
	}
	src := oauth2.ReuseTokenSource(tok, conf.TokenSource(ctx, tok))
	persisting := auth.NewPersistingSource(src, tokPath)

	httpClient := &http.Client{
		Transport: &auth.RefreshOn401Transport{
			Base:   &oauth2.Transport{Source: persisting, Base: http.DefaultTransport},
			Source: persisting,
		},
	}

	return api.NewClient(tok.AccessToken, api.WithHTTPClient(httpClient)), nil
}

func loadCache() (*cache.Cache, error) {
	p, err := config.CacheDBPath()
	if err != nil {
		return nil, err
	}
	return cache.Open(p)
}
```

- [ ] **Step 2: Implement `whoami.go`**

```go
package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewWhoami() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Show the authenticated user",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := loadClient(ctx)
			if err != nil {
				return err
			}
			u, err := c.Me(ctx)
			if err != nil {
				return err
			}
			fmt.Fprintf(os.Stdout, `{"id":%q,"display_name":%q,"email":%q}`+"\n", u.ID, u.DisplayName, u.Email)
			return nil
		},
	}
}
```

(Remove `NewWhoami` from `_stubs.go`.)

- [ ] **Step 3: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(whoami): print authenticated user"
```

---

## Phase 6 — Templates & Killer Command

### Task 23: `canva templates` + `canva templates show`

**Files:**
- Create: `internal/commands/templates.go`

- [ ] **Step 1: Implement**

```go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewTemplates() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "templates",
		Short: "List brand templates (Enterprise-only)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			c, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()
			err = c.ListTemplates(ctx, func(t api.BrandTemplate) error {
				raw, _ := json.Marshal(t)
				_ = ch.UpsertTemplate(cache.Template{
					ID:        t.ID,
					Title:     t.Title,
					FetchedAt: time.Now().Unix(),
					RawJSON:   string(raw),
				})
				return output.EmitJSON(os.Stdout, map[string]any{
					"id": t.ID, "title": t.Title, "updated_at": t.UpdatedAt,
				})
			})
			return err
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "show <name|id>",
		Short: "Show autofill dataset for a template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			c, err := loadClient(ctx)
			if err != nil {
				return err
			}
			ch, _ := loadCache()
			if ch != nil {
				defer ch.Close()
			}
			id := args[0]
			// Try cache first for name-resolution
			if ch != nil {
				if t, _ := ch.FindTemplateByID(id); t != nil {
					id = t.ID
				} else {
					if matches, _ := ch.FindTemplateByName(id); len(matches) == 1 {
						id = matches[0].ID
					} else if len(matches) > 1 {
						return fmt.Errorf("multiple templates named %q", id)
					}
				}
			}
			ds, err := c.GetTemplateDataset(ctx, id)
			if err != nil {
				return err
			}
			return output.EmitJSON(os.Stdout, ds)
		},
	})
	return cmd
}
```

(Remove `NewTemplates` from `_stubs.go`.)

- [ ] **Step 2: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(templates): list and show brand templates"
```

### Task 24: `canva create` (the killer command)

**Files:**
- Create: `internal/commands/create.go`

- [ ] **Step 1: Implement**

```go
package commands

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
	"github.com/spf13/cobra"
)

func NewCreate() *cobra.Command {
	var (
		flagTemplate, flagAutofill, flagFolder, flagTitle, flagIdempotency string
		flagDryRun                                                          bool
	)
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a design from a brand template + autofill data",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if flagTemplate == "" || flagAutofill == "" {
				return errors.New("--template and --autofill are required")
			}
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

			tplID, err := resolver.New(ch, cl).ResolveTemplate(flagTemplate)
			if err != nil {
				return err
			}

			// Load autofill data (file path or "-" for stdin)
			var data map[string]any
			var raw []byte
			if flagAutofill == "-" {
				raw, err = io.ReadAll(os.Stdin)
			} else {
				raw, err = os.ReadFile(flagAutofill)
			}
			if err != nil {
				return err
			}
			if err := json.Unmarshal(raw, &data); err != nil {
				return fmt.Errorf("autofill JSON: %w", err)
			}

			// Idempotency
			argsHash := sha256hex(tplID + ":" + flagTitle + ":" + string(raw))
			if flagIdempotency != "" {
				if prior, _ := ch.LookupIdempotency(flagIdempotency, argsHash); prior != "" {
					_, _ = fmt.Fprintln(os.Stdout, prior)
					return nil
				}
			}

			req := api.AutofillRequest{
				BrandTemplateID: tplID,
				Data:            data,
				Title:           flagTitle,
			}
			if flagDryRun {
				return output.EmitJSON(os.Stdout, map[string]any{
					"dry_run": true,
					"method":  "POST",
					"path":    "/autofills",
					"body":    req,
				})
			}

			res, err := cl.CreateAutofill(ctx, req)
			if err != nil {
				return err
			}
			out := map[string]any{
				"id":    res.Design.ID,
				"url":   res.Design.URL,
				"title": res.Design.Title,
			}
			outJSON, _ := json.Marshal(out)
			if flagIdempotency != "" {
				_ = ch.SaveIdempotency(flagIdempotency, "create", argsHash, string(outJSON))
			}
			return output.EmitJSON(os.Stdout, out)
		},
	}
	cmd.Flags().StringVar(&flagTemplate, "template", "", "brand template name or ID (required)")
	cmd.Flags().StringVar(&flagAutofill, "autofill", "", "path to JSON file or '-' for stdin (required)")
	cmd.Flags().StringVar(&flagFolder, "folder", "", "destination folder name or ID")
	cmd.Flags().StringVar(&flagTitle, "title", "", "design title")
	cmd.Flags().StringVar(&flagIdempotency, "idempotency-key", "", "client-side dedupe key")
	cmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "print the API call without executing")
	return cmd
}

func sha256hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

var _ = strings.TrimSpace
```

(Remove `NewCreate` from `_stubs.go`.)

- [ ] **Step 2: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(create): autofill-driven design creation with client-side idempotency and dry-run"
```

---

## Phase 7 — Design Management

### Task 25: `canva list`

**Files:**
- Create: `internal/commands/list.go`

- [ ] **Step 1: Implement**

```go
package commands

import (
	"encoding/json"
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewList() *cobra.Command {
	var flagFields string
	var flagLimit int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List your designs (NDJSON when piped)",
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
			count := 0
			err = cl.ListDesigns(ctx, func(d api.Design) error {
				if flagLimit > 0 && count >= flagLimit {
					return nil
				}
				raw, _ := json.Marshal(d)
				thumb := ""
				if d.Thumbnail != nil {
					thumb = d.Thumbnail.URL
				}
				_ = ch.UpsertDesign(cache.Design{
					ID: d.ID, Title: d.Title, UpdatedAt: d.UpdatedAt,
					FetchedAt: time.Now().Unix(),
					ThumbnailURL: thumb, RawJSON: string(raw),
				})
				row := map[string]any{
					"id": d.ID, "title": d.Title, "updated_at": d.UpdatedAt,
				}
				if flagFields == "all" {
					_ = json.Unmarshal(raw, &row)
				} else if flagFields != "" {
					row = output.ProjectFields(row, flagFields)
				}
				count++
				return output.EmitJSON(os.Stdout, row)
			})
			return err
		},
	}
	cmd.Flags().StringVar(&flagFields, "fields", "id,title,updated_at", "comma-separated field list or 'all'")
	cmd.Flags().IntVar(&flagLimit, "limit", 20, "max number of designs to emit")
	return cmd
}
```

(Remove `NewList` from `_stubs.go`.)

- [ ] **Step 2: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(list): NDJSON design listing with field projection and pagination cap"
```

### Task 26: `canva export`

**Files:**
- Create: `internal/commands/export.go`

- [ ] **Step 1: Implement**

```go
package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/catalinlongevai/canvacli/internal/resolver"
	"github.com/spf13/cobra"
)

func NewExport() *cobra.Command {
	var flagFormat, flagOutput string
	var flagURLOnly bool
	cmd := &cobra.Command{
		Use:   "export <name|id>",
		Short: "Export a design as PDF/PNG/JPG/MP4/GIF",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if flagFormat == "" {
				return errors.New("--format is required")
			}
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
			id, err := resolver.New(ch, cl).ResolveDesign(args[0])
			if err != nil {
				return err
			}
			res, err := cl.CreateExport(ctx, api.ExportRequest{
				DesignID: id,
				Format:   api.ExportFormat{Type: flagFormat},
			})
			if err != nil {
				return err
			}
			if len(res.URLs) == 0 {
				return errors.New("export returned no URLs")
			}
			if flagURLOnly {
				return output.EmitJSON(os.Stdout, map[string]any{
					"id": id, "format": flagFormat, "urls": res.URLs,
					"warning": "URLs expire in 24 hours",
				})
			}
			outPath := flagOutput
			if outPath == "" {
				outPath = fmt.Sprintf("%s.%s", id, flagFormat)
			}
			// If multiple URLs (multi-page), append page index before ext
			for i, u := range res.URLs {
				p := outPath
				if len(res.URLs) > 1 {
					ext := filepath.Ext(outPath)
					base := outPath[:len(outPath)-len(ext)]
					p = fmt.Sprintf("%s_%d%s", base, i+1, ext)
				}
				if err := cl.DownloadTo(ctx, u, p); err != nil {
					return err
				}
			}
			return output.EmitJSON(os.Stdout, map[string]any{
				"id": id, "format": flagFormat, "files": res.URLs,
			})
		},
	}
	cmd.Flags().StringVar(&flagFormat, "format", "", "pdf|png|jpg|mp4|gif (required)")
	cmd.Flags().StringVar(&flagOutput, "output", "", "output path (default: <design-id>.<format>)")
	cmd.Flags().BoolVar(&flagURLOnly, "url-only", false, "return URL instead of downloading (24h expiry)")
	return cmd
}
```

(Remove `NewExport` from `_stubs.go`.)

- [ ] **Step 2: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(export): async export with eager download and url-only mode"
```

### Task 27: `canva folders`

**Files:**
- Create: `internal/commands/folders.go`

- [ ] **Step 1: Implement**

```go
package commands

import (
	"os"
	"time"

	"github.com/catalinlongevai/canvacli/internal/api"
	"github.com/catalinlongevai/canvacli/internal/cache"
	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewFolders() *cobra.Command {
	return &cobra.Command{
		Use:   "folders",
		Short: "List folders (walks 'root' and 'uploads')",
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
			return cl.WalkFolders(ctx, func(f api.Folder, parent string) error {
				_ = ch.UpsertFolder(cache.Folder{
					ID: f.ID, Name: f.Name, ParentID: parent,
					FetchedAt: time.Now().Unix(),
				})
				return output.EmitJSON(os.Stdout, map[string]any{
					"id": f.ID, "name": f.Name, "parent_id": parent,
				})
			})
		},
	}
}
```

(Remove `NewFolders` from `_stubs.go`.)

- [ ] **Step 2: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(folders): list folders by walking root and uploads"
```

---

## Phase 8 — Agent Affordances

### Task 28: `canva schema`

**Files:**
- Create: `internal/commands/schema.go`

The schema is hand-curated rather than auto-generated from cobra to control token size precisely.

- [ ] **Step 1: Implement**

```go
package commands

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/spf13/cobra"
)

const schemaCompactJSON = `{
  "version": "1",
  "commands": [
    {"name": "login", "args": []},
    {"name": "logout", "args": []},
    {"name": "whoami", "args": []},
    {"name": "templates", "args": []},
    {"name": "templates show", "args": ["name|id"]},
    {"name": "create", "required_flags": ["template","autofill"]},
    {"name": "list", "flags": ["fields","limit"]},
    {"name": "export", "args": ["name|id"], "required_flags": ["format"]},
    {"name": "folders", "args": []},
    {"name": "schema", "flags": ["compact","full","command"]},
    {"name": "sql", "args": ["query"], "flags": ["limit"]}
  ],
  "exit_codes": {"0":"success","2":"auth","3":"not_found","4":"network","5":"validation","6":"rate_limited","7":"permission_denied"}
}`

const schemaFullJSON = `{
  "version": "1",
  "commands": [
    {"name":"login","summary":"OAuth 2.0 PKCE browser flow","examples":["canva login"]},
    {"name":"logout","summary":"Remove stored credentials and clear cache"},
    {"name":"whoami","summary":"Show authenticated user","examples":["canva whoami"]},
    {"name":"templates","summary":"List brand templates (Enterprise)","examples":["canva templates"]},
    {"name":"templates show","summary":"Get autofill dataset for template","args":["name|id"],"examples":["canva templates show 'Social Post'"]},
    {"name":"create","summary":"Create design from template + autofill (Enterprise)","required_flags":["template","autofill"],"flags":["folder","title","idempotency-key","dry-run"],"examples":["canva create --template 'Social Post' --autofill data.json"],"error_codes":["template_not_found","validation","permission_denied"]},
    {"name":"list","summary":"List designs as NDJSON","flags":["fields","limit"],"examples":["canva list --limit 5","canva list --fields id,title"]},
    {"name":"export","summary":"Export design (eager download)","args":["name|id"],"required_flags":["format"],"flags":["output","url-only"],"examples":["canva export 'Q3 Banner' --format pdf"],"error_codes":["design_not_found","validation"]},
    {"name":"folders","summary":"List folders by walking root and uploads"},
    {"name":"schema","summary":"Print this schema","flags":["compact","full","command"]},
    {"name":"sql","summary":"Read-only SQL against local cache","args":["query"],"flags":["limit"],"examples":["canva sql \"SELECT id,title FROM designs LIMIT 5\""]}
  ],
  "global_flags": ["json","no-cache","quiet","auto-wait"],
  "exit_codes": {"0":"success","2":"auth_required/auth_revoked","3":"not_found","4":"network","5":"validation","6":"rate_limited","7":"permission_denied"},
  "error_envelope": {"error":"<stable code>","message":"<human>","fix":"<literal command to retry>","exit_code":1}
}`

func NewSchema() *cobra.Command {
	var compact, full bool
	var command string
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Print the canvacli schema as JSON for agent introspection",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := schemaCompactJSON
			if full {
				out = schemaFullJSON
			} else if command != "" {
				return errors.New("schema --command not yet implemented; use --full and filter externally")
			}
			// Pretty-print to confirm it's valid JSON before emitting
			var v any
			if err := json.Unmarshal([]byte(out), &v); err != nil {
				return err
			}
			b, _ := json.Marshal(v)
			os.Stdout.Write(append(b, '\n'))
			return nil
		},
	}
	cmd.Flags().BoolVar(&compact, "compact", true, "compact schema (~500 tokens)")
	cmd.Flags().BoolVar(&full, "full", false, "full schema (~3K tokens)")
	cmd.Flags().StringVar(&command, "command", "", "schema for one command only")
	return cmd
}
```

(Remove `NewSchema` from `_stubs.go`.)

- [ ] **Step 2: Build + verify size budget**

```bash
go build ./... && ./canvacli schema --compact | wc -c && ./canvacli schema --full | wc -c
```

Expected: compact < 4096; full < 16384.

- [ ] **Step 3: Commit**

```bash
git add internal/commands/schema.go
git commit -m "feat(schema): static compact and full schemas with token budget"
```

### Task 29: `canva sql`

**Files:**
- Create: `internal/commands/sql.go`

- [ ] **Step 1: Implement**

```go
package commands

import (
	"errors"
	"os"

	"github.com/catalinlongevai/canvacli/internal/output"
	"github.com/spf13/cobra"
)

func NewSQL() *cobra.Command {
	var flagLimit int
	var flagSchema bool
	cmd := &cobra.Command{
		Use:   "sql [query]",
		Short: "Read-only SQL against the local cache",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ch, err := loadCache()
			if err != nil {
				return err
			}
			defer ch.Close()
			if flagSchema {
				return output.EmitJSON(os.Stdout, map[string]any{
					"tables": map[string][]string{
						"designs":     {"id", "title", "folder_id", "updated_at", "fetched_at", "thumbnail_url", "raw_json"},
						"templates":   {"id", "title", "fetched_at", "raw_json"},
						"folders":     {"id", "name", "parent_id", "fetched_at"},
						"idempotency": {"key", "command", "args_hash", "result_json", "created_at"},
					},
				})
			}
			if len(args) == 0 {
				return errors.New("provide a SQL query or --schema")
			}
			rows, err := ch.ExecReadOnly(args[0], flagLimit)
			if err != nil {
				return err
			}
			return output.EmitNDJSON(os.Stdout, rows)
		},
	}
	cmd.Flags().IntVar(&flagLimit, "limit", 500, "max rows to return (cap 10000)")
	cmd.Flags().BoolVar(&flagSchema, "schema", false, "print cache table schema")
	return cmd
}
```

(Remove `NewSQL` from `_stubs.go`.)

- [ ] **Step 2: Build, commit**

```bash
go build ./... && git add internal/commands/ && git commit -m "feat(sql): read-only SQL escape hatch with schema introspection"
```

---

## Phase 9 — Distribution & Docs

### Task 30: GoReleaser config

**Files:**
- Create: `.goreleaser.yaml`

- [ ] **Step 1: Write the config**

```yaml
version: 2

before:
  hooks:
    - go mod tidy

builds:
  - id: canvacli
    main: ./cmd/canvacli
    binary: canvacli
    env:
      - CGO_ENABLED=0
    goos: [linux, darwin, windows]
    goarch: [amd64, arm64]
    ignore:
      - goos: windows
        goarch: arm64
    ldflags:
      - -s -w
      - -X main.version={{.Version}}
      - -X main.commit={{.ShortCommit}}
      - -X main.date={{.Date}}

archives:
  - format: tar.gz
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        format: zip

checksum:
  name_template: "checksums.txt"

snapshot:
  name_template: "{{ incpatch .Version }}-next"

changelog:
  sort: asc
  use: github
  filters:
    exclude:
      - "^docs:"
      - "^chore:"
      - "^test:"

brews:
  - name: canvacli
    repository:
      owner: catalinlongevai
      name: homebrew-tap
      token: "{{ .Env.HOMEBREW_TAP_TOKEN }}"
    homepage: https://github.com/catalinlongevai/canvacli
    description: Agent-first CLI for the Canva Connect API
    license: MIT
    test: |
      system "#{bin}/canvacli --version"
    install: |
      bin.install "canvacli"
    skip_upload: auto
```

- [ ] **Step 2: Commit**

```bash
git add .goreleaser.yaml
git commit -m "build: goreleaser config with brews and snapshot"
```

### Task 31: Release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Write workflow**

```yaml
name: release

on:
  push:
    tags:
      - "v*"

permissions:
  contents: write
  id-token: write

jobs:
  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v6
        with:
          go-version: '1.22'
          cache: true
      - name: Run GoReleaser
        uses: goreleaser/goreleaser-action@v7
        with:
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
          HOMEBREW_TAP_TOKEN: ${{ secrets.HOMEBREW_TAP_TOKEN }}
      - name: Smoke test linux binary
        run: |
          ARCHIVE=$(ls dist/canvacli_*_linux_amd64.tar.gz | head -1)
          tar -xzf "$ARCHIVE" -C /tmp
          /tmp/canvacli --version
          /tmp/canvacli schema --compact | wc -c | awk '$1 > 4096 { print "compact schema too large: "$1; exit 1 }'
```

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/release.yml
git commit -m "ci: tag-triggered goreleaser pipeline with smoke test"
```

### Task 32: README.md and shipped CLAUDE.md

**Files:**
- Create: `README.md`, `CLAUDE.md`

- [ ] **Step 1: Write `CLAUDE.md` (≤ 150 lines)**

```markdown
# canvacli for Claude Code

An agent-first CLI for the Canva Connect API. Local SQLite cache, stable JSON output, structured errors with `fix` actions.

## Commands

| Command | Purpose |
|---|---|
| `canva login` | OAuth 2.0 PKCE browser flow |
| `canva logout` | Clear stored credentials and cache |
| `canva whoami` | Print authenticated user as JSON |
| `canva templates` | List brand templates (Enterprise only) |
| `canva templates show <name\|id>` | Show autofill fields for a template |
| `canva create --template <t> --autofill data.json [--title T] [--folder F] [--idempotency-key K] [--dry-run]` | Generate a design from a template (Enterprise only) |
| `canva list [--limit 20] [--fields id,title,...]` | NDJSON listing of your designs |
| `canva export <name\|id> --format pdf [--output ./out.pdf] [--url-only]` | Export a design (eager download) |
| `canva folders` | NDJSON listing of folders |
| `canva schema [--compact\|--full]` | Print the CLI schema as JSON |
| `canva sql "SELECT ..."` | Read-only SQL against local cache |

## Global flags

- `--json` — force JSON output (auto-on when stdout is piped)
- `--no-cache` — bypass local cache, force API call
- `--quiet` — suppress progress
- `--auto-wait` — auto-retry once on 429 (capped at 60s)

## Output convention

When stdout is **piped or redirected**, output is always JSON or NDJSON. When stdout is a **TTY**, output is a human table. Errors are always structured JSON to stderr:

```json
{"error":"design_not_found","message":"...","fix":"canva list --json | grep -i banner","exit_code":3}
```

The `fix` field contains a literal command you can execute to recover.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 2 | Auth required or revoked — run `canva login` |
| 3 | Not found |
| 4 | Network |
| 5 | Validation |
| 6 | Rate limited (see `wait_seconds`) |
| 7 | Permission denied |

## Common patterns

- "I don't know the design ID" → `canva list --json | jq` to find it.
- "I need a field the CLI doesn't expose" → `canva sql "SELECT raw_json FROM designs WHERE id = '...'"`
- "Auth feels broken" → `canva whoami`, then `canva login` if it fails.

## Enterprise gating

`canva create` and `canva templates` require Canva Enterprise. The other commands work on any Canva account.
```

- [ ] **Step 2: Write `README.md`**

```markdown
# canvacli

Agent-first CLI for the Canva Connect API.

```bash
brew install catalinlongevai/tap/canvacli

canva login
canva create --template "Social Post" --autofill data.json
canva export "Social Post (autofilled)" --format pdf
```

## Why

Canva's official `@canva/cli` is for building Canva Apps. There's no CLI for *using* Canva — listing designs, exporting them, generating designs from brand templates programmatically. `canvacli` fills that gap, with a design optimized for AI coding agents (Claude Code, Cursor, etc.) so they can use it without explanation.

## Install

```bash
brew install catalinlongevai/tap/canvacli
```

Or grab a binary from [Releases](https://github.com/catalinlongevai/canvacli/releases).

## Quick start

```bash
# 1. Authenticate (opens browser)
canva login

# 2. List your designs (NDJSON when piped)
canva list --limit 5

# 3. Generate a design from a brand template (Enterprise only)
echo '{"headline":"Hello","subhead":"World"}' | canva create \
  --template "Social Post" \
  --autofill -

# 4. Export to PDF
canva export "Social Post (autofilled)" --format pdf
```

## Enterprise dependency

`canva create` and `canva templates` rely on Canva Connect endpoints that are gated to **Canva Enterprise** customers. The rest of the CLI works on any account.

## Agent integration

```bash
# Drop the schema into your agent's context once
canva schema --full > .canvacli-schema.json

# The agent can now invoke any command without reading --help
```

See [`CLAUDE.md`](./CLAUDE.md) for a Claude-Code-ready brief.

## License

MIT.
```

- [ ] **Step 3: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: README and shipped CLAUDE.md for agent onboarding"
```

### Task 33: Final integration check + tag a v0.1.0-beta

**Files:** none

- [ ] **Step 1: Final build, vet, test**

```bash
go vet ./...
go test -race -count=1 ./...
go build -o /tmp/canvacli ./cmd/canvacli
/tmp/canvacli --version
/tmp/canvacli --help
/tmp/canvacli schema --compact | wc -c
```

Expected: vet clean, all tests pass, binary builds, schema under 4 KB.

- [ ] **Step 2: Push**

```bash
git push origin master
```

- [ ] **Step 3: Tag a beta**

```bash
git tag v0.1.0-beta.1
git push origin v0.1.0-beta.1
```

The `release.yml` workflow runs and publishes binaries to GitHub Releases. The Homebrew tap upload is auto-skipped for prereleases.

- [ ] **Step 4: Confirm**

Verify the GitHub Release page lists tar.gz/zip archives for all 5 platforms and a `checksums.txt`.

---

## Self-Review Notes

This plan was verified against the spec on 2026-05-07. Coverage:

- §3 Architecture → Tasks 1, 4, 14, 17, 18, 19
- §4 Command surface (all v1 commands) → Tasks 20–29
- §5 OAuth → Tasks 9, 10, 11, 12, 13, 20
- §6 Cache → Tasks 14, 15, 16
- §7 Agent ergonomics → Tasks 18, 28, 29
- §8 Token efficiency → Task 25 (`--fields`/`--limit` defaults), Task 28 (schema sizes)
- §9 Errors → Tasks 5, 18
- §10 Testing → embedded throughout (TDD per task)
- §11 Distribution → Tasks 30, 31
- §15 Risks (Enterprise gating) → disclosed in Tasks 23 (templates), 24 (create), 32 (README + CLAUDE.md)

**Deferred to v1.x / v2 (not in this plan):**
- `canva delete` (no API) — flagged in spec, intentionally absent
- Pattern A (sync, search, comments archive)
- Embedded client_id/secret via build-time ldflags — for now read from env

**Known limitations of this plan:**
- The `_stubs.go` file is created in Task 19 and progressively shrunk as commands land. An executor reading tasks out of order could be confused; the explicit "Remove `NewX` from `_stubs.go`" step in each command task is the recovery hint.
- The `oauth2.Config{}` block in `clientutil.go` reads `CANVA_CLIENT_ID`/`CANVA_CLIENT_SECRET` from env. A registered Canva developer app is a prerequisite — documented in README. v1.x will move secrets into ldflags.
- Test cassettes (`go-vcr`) are referenced in the spec but not used in this plan — every API task uses `httptest.Server` mocks instead, which is faster and avoids the cassette-recording chicken-and-egg problem before a real Canva account is wired up. Cassettes can be added in v1.x.
