# canvacli — Design Specification

**Date:** 2026-05-07
**Status:** Approved (brainstorming complete, awaiting implementation plan)
**Author:** Catalin Niculescu

## 1. Overview

`canvacli` is a Go-based command-line interface for Canva, distributed as a single static binary via Homebrew and GitHub releases. It provides programmatic access to the Canva Connect API for users (designers, developers, AI coding agents) — a category currently unserved.

The official `@canva/cli` exists but is scoped to building Canva Apps (developer SDK tooling). It does not expose the user-facing design management surface (create from templates, export, list designs, manage assets). `canvacli` fills that gap.

## 2. Goals & Non-Goals

### Goals
- **Agent-first ergonomics.** The CLI is designed first for AI coding agents (Claude Code, Cursor, etc.) and second for humans. Both audiences benefit, but design trade-offs favor agents.
- **Programmatic design generation.** The killer command is `canva create --template <name|id> --autofill data.json`, which generates designs from brand templates with structured data input.
- **Single-binary install.** Installable via Homebrew tap (e.g. `brew install <tap-owner>/tap/canvacli`) with zero runtime dependencies. Homebrew-core submission is a stretch goal once the tool is mature.
- **Composable Unix-style output.** Stable JSON, predictable exit codes, NDJSON streaming for lists.
- **Open source, README-driven.** Public GitHub repo with shipped `CLAUDE.md` for instant agent onboarding.

### Non-Goals (v1)
- Pattern A (full local SQLite mirror with FTS5 search) — deferred to v2.
- Comment archiving and asset library mirroring — deferred to v2.
- Browser automation or scraping — Connect API only.
- Building Canva Apps (handled by official `@canva/cli`).
- Real-time collaboration features (live cursors, etc.).

## 3. Architecture

### 3.1 Repository layout

```
canvacli/
├── cmd/canvacli/         main package — entry point
├── internal/
│   ├── api/              Canva Connect API client (typed Go structs)
│   ├── auth/             OAuth 2.0 PKCE flow, token storage, refresh
│   ├── cache/            SQLite cache (designs, templates metadata)
│   ├── resolver/         name-or-ID resolution against cache
│   ├── commands/         cobra command definitions
│   └── output/           JSON/text formatters, TTY detection
├── docs/
│   ├── superpowers/      design specs, plans
│   └── CLAUDE.md         shipped agent instruction file (root copy too)
├── CLAUDE.md             agent instructions (root, for installed copy)
├── README.md
├── LICENSE               MIT
└── .goreleaser.yaml      multi-platform release config
```

### 3.2 External dependencies (Go modules)

| Concern | Module | Reason |
|---|---|---|
| Command routing | `github.com/spf13/cobra` | Standard, well-supported |
| OAuth 2.0 PKCE | `golang.org/x/oauth2` | Stdlib-adjacent, audited |
| HTTP client | `net/http` (stdlib) | No third-party HTTP wrapper needed |
| SQLite | `modernc.org/sqlite` | Pure Go (no CGo) — keeps cross-compilation clean |
| TTY detection | `golang.org/x/term` | Stdlib-adjacent |
| Test cassettes | `gopkg.in/dnaeon/go-vcr.v3` | Record/replay HTTP for integration tests |

### 3.3 Data flow (killer command example)

```
canva create --template "Social Post" --autofill data.json
   │
   ├─ resolver/   resolve "Social Post" → template_id (cache → API fallback)
   ├─ api/        POST /v1/autofills  body: { template_id, data }
   ├─ api/        poll /v1/autofills/{job_id} until status=success
   ├─ cache/      upsert new design row
   └─ output/     emit JSON (piped) or table (TTY)
```

The flow is **idempotent**: passing `--idempotency-key <key>` ensures retries do not create duplicates. Server-side autofill jobs are keyed; agents that retry on transient failures get the original design back.

### 3.4 Storage paths

| Path | Purpose | Permissions |
|---|---|---|
| `~/.config/canvacli/token.json` | OAuth tokens (access + refresh) | `0600` |
| `~/.config/canvacli/cache.db` | SQLite cache (designs, templates, folders metadata) | `0644` |
| `~/.config/canvacli/config.toml` | User settings (default folder, output mode override) | `0644` |

`XDG_CONFIG_HOME` respected when set.

## 4. v1 Command Surface

### 4.1 Authentication

```
canva login                    OAuth 2.0 PKCE browser flow
canva logout                   revoke token, clear cache
canva whoami                   current user, scopes, token expiry
```

### 4.2 Core killer flow

```
canva templates                          list brand templates
canva templates show <name|id>           show autofill fields + types
canva create \                           ★ killer command
  --template <name|id> \
  --autofill <file|->                    JSON file or stdin
  [--folder <name|id>]
  [--title <string>]
  [--idempotency-key <key>]
  [--dry-run]
```

### 4.3 Design management

```
canva list [--folder <name|id>]                    list designs
canva export <name|id> \
  --format pdf|png|jpg|mp4|gif \
  [--output ./path]                                async export, polls until ready
canva delete <name|id> [--idempotency-key <key>]
canva folders                                      list folders
```

### 4.4 Agent affordances

```
canva schema [--compact|--full]    full CLI surface as JSON
canva sql "SELECT ..."             raw SQL query against local cache
canva sql --schema                 print cache table schema
```

### 4.5 Universal flags (every command)

| Flag | Purpose |
|---|---|
| `--json` | Force structured output (auto-on when stdout is not a TTY) |
| `--no-cache` | Bypass local resolver, force fresh API call |
| `--quiet` | Suppress progress output (still emit final result) |
| `--help` | Standard cobra help |
| `--version` | Binary version, schema version, git commit |

## 5. Authentication & Token Management

### 5.1 OAuth 2.0 PKCE flow

1. `canva login` generates a code verifier + challenge.
2. Opens default browser to Canva's authorization endpoint with the challenge.
3. Spawns local HTTP listener on `127.0.0.1:<random-port>`.
4. User approves in browser; Canva redirects to local listener with authorization code.
5. CLI exchanges code + verifier for access + refresh tokens.
6. Tokens persisted to `~/.config/canvacli/token.json` (mode `0600`).
7. Listener shuts down.

### 5.2 Refresh strategy

- Access tokens auto-refreshed on 401 response, transparent to agent.
- If refresh token is also revoked, structured error returned with `fix: "canva login"`.
- Refresh failures do not retry indefinitely — single attempt, then surface error.

### 5.3 Scopes requested

Minimum scopes for v1 surface:
- `design:content:read`
- `design:content:write`
- `design:meta:read`
- `asset:read`
- `asset:write`
- `brandtemplate:meta:read`
- `brandtemplate:content:read`
- `folder:read`
- `folder:write`
- `comment:read`
- `comment:write`
- `profile:read`

## 6. Cache Strategy

The cache is **best-effort** and exists primarily for name resolution. A stale cache never causes a wrong answer — only a slower call (cache miss → API fetch → cache update).

### 6.1 Cached entities (v1)

| Entity | TTL | Invalidation triggers |
|---|---|---|
| User profile | 7 days | `canva login`, `canva logout` |
| Brand templates | 1 hour | `canva templates --no-cache`, mutating ops |
| Designs metadata (id, title, updated_at, folder_id) | 24 hours | `canva create`, `canva delete`, `--no-cache` |
| Folders | 24 hours | mutating folder ops, `--no-cache` |

### 6.2 Schema (SQLite)

```sql
CREATE TABLE designs (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  folder_id TEXT,
  updated_at INTEGER NOT NULL,    -- unix epoch
  fetched_at INTEGER NOT NULL,
  thumbnail_url TEXT,
  raw_json TEXT NOT NULL          -- full API response
);

CREATE TABLE templates (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  fetched_at INTEGER NOT NULL,
  raw_json TEXT NOT NULL
);

CREATE TABLE folders (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  parent_id TEXT,
  fetched_at INTEGER NOT NULL
);

CREATE TABLE meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE INDEX idx_designs_title ON designs(title);
CREATE INDEX idx_templates_title ON templates(title);
```

FTS5 virtual tables intentionally deferred to v2 (Pattern A).

### 6.3 Resolution algorithm

Given a `<name|id>` argument:

1. Try cache lookup by `id = ?` first (covers most agent calls that pass IDs verbatim).
2. If no ID hit, query cache by `WHERE title = ? COLLATE NOCASE LIMIT 2`.
   - 1 match → use it.
   - 2+ matches → return `multiple_matches` error with `suggestions: [...]`.
3. If still no match, fall back to API search by name.
4. `--no-cache` skips steps 1–2 and goes directly to API.

This avoids relying on a guessed regex for Canva's ID format — the cache itself is the source of truth.

## 7. Agent Ergonomics

The single highest-priority section. Six concrete affordances:

### 7.1 Auto-JSON for non-TTY

`canvacli` detects whether stdout is a TTY. If not, output is automatically JSON (single object) or NDJSON (lists). Humans see tables; pipes see JSON. No flag required.

### 7.2 Stable error envelope

Every error response (when in JSON mode) follows:

```json
{
  "error": "design_not_found",
  "message": "no design matched 'Q3 Bannr'",
  "suggestions": ["abc123 (Q3 Banner)", "def456 (Q3 Banner v2)"],
  "fix": "canva list --json | grep -i banner",
  "exit_code": 3
}
```

Field contracts:
- `error` — stable string code, frozen across patch versions, may add new codes in minor versions.
- `message` — human-readable, may change between versions.
- `suggestions` — optional, present when ambiguity is the cause.
- `fix` — optional, contains a literal command the agent can execute to recover.
- `exit_code` — duplicates the process exit code for convenience.

### 7.3 Stable exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Generic error (only when no specific code applies) |
| 2 | Authentication failure (run `canva login`) |
| 3 | Resource not found |
| 4 | Network / API unavailable |
| 5 | Validation error (bad flags, malformed input) |
| 6 | Rate limited (retry after `wait_seconds`) |
| 7 | Permission denied (scope insufficient) |

### 7.4 `canva schema` introspection

```
canva schema --compact     ~500 tokens, command names + required flags only
canva schema --full        ~3K tokens, full surface with examples + error codes
canva schema --command create   schema for one command only
```

Default is `--compact`. Schema includes:
- Command name + path
- Args + flags with types, descriptions, examples
- Exit codes the command can return
- Sample success and error JSON shapes

### 7.5 Shipped `CLAUDE.md`

Repository root contains a `CLAUDE.md` (~150 lines max) with:
- One-line summary per command
- Three example invocations agents can copy
- Common error-recovery patterns (e.g., "if exit code 3, run `canva list --json` first")
- Pointer to `canva schema --full` for depth

When users install canvacli in a project, they can copy or symlink this file into their repo's CLAUDE.md and any agent immediately knows the tool.

### 7.6 `canva sql` escape hatch

When a needed query isn't supported by a dedicated command, agents can write SQL directly against the cache. Schema is documented in `CLAUDE.md` and via `canva sql --schema`. This eliminates "the CLI doesn't support what I need" failures.

### 7.7 Idempotency + dry-run

- All mutating commands (`create`, `delete`) accept `--idempotency-key`. Server-side jobs are keyed; safe to retry.
- All mutating commands accept `--dry-run`, which returns the exact API call that *would* be made without executing.

## 8. Token Efficiency

Defaults chosen to minimize agent context cost:

- **Field projection.** `canva list` returns `id`, `title`, `updated_at` only by default. `--fields all` for full metadata, `--fields id,title,thumbnail_url` for explicit selection.
- **Pagination.** Default `--limit 20`, max 100. Cursor-based pagination via `--cursor`.
- **Compact NDJSON.** No pretty-printing or indentation when piped.
- **`canva schema --compact` is the default.** Full schema only on demand.
- **`CLAUDE.md` capped at ~150 lines.**
- **No ANSI colors / emoji in piped output.**

Token efficiency past these defaults yields diminishing returns and is not pursued in v1.

## 9. Error Handling

### 9.1 Principles

- **Never panic in production code.** All errors propagate through a typed error chain.
- **Errors carry codes from the leaves.** API client returns typed errors (`ErrAuthRevoked`, `ErrNotFound`, etc.) that map directly to exit codes.
- **No retry storms.** Refresh token failures retry once. Rate-limited responses surface immediately with `wait_seconds`; agents decide whether to retry.

### 9.2 Network errors

Transient HTTP errors (5xx, timeout) retry up to 2 times with exponential backoff (250ms, 1s). After that, surface exit code 4 with `fix: "check network connectivity, retry in 30s"`.

## 10. Testing Strategy

### 10.1 Unit tests

`internal/resolver/`, `internal/cache/`, `internal/output/` are pure-logic packages with deterministic inputs. Target ≥80% coverage.

### 10.2 Integration tests with HTTP cassettes

`go-vcr` records real Canva Connect API responses once, replays in CI. No live API calls required for routine testing. Cassette files committed under `testdata/cassettes/`.

### 10.3 Smoke test in CI

GitHub Actions runs the built binary:
- `canvacli --version` — packaging sanity check
- `canvacli schema --compact` — every command introspectable
- `canvacli schema --json | jq '.commands[] | select(.error_codes == null)'` — fails CI if any command lacks error code documentation

### 10.4 Agent-friendliness invariant test

A Go test parses `canva schema --json` output and asserts:
- Every command has a stable list of error codes.
- Every documented error code has a `fix` field where applicable.
- Every command supports `--json`.

This makes agent ergonomics a quality gate, not a nice-to-have.

## 11. Distribution

### 11.1 Release pipeline

GitHub Actions + GoReleaser builds for:
- `darwin-arm64`
- `darwin-amd64`
- `linux-amd64`
- `linux-arm64`
- `windows-amd64`

Each tagged release publishes:
- Static binaries to GitHub Releases.
- Homebrew formula auto-updated in companion tap repo.
- Checksums + SLSA provenance attestation.

### 11.2 Versioning

Semantic versioning. Breaking changes to:
- Command names or flags
- Error code values
- JSON output shapes

…require a major version bump. The `canva schema` output includes a schema version that increments on any of these; agents pinned to a major version are guaranteed stable error codes and command surface.

### 11.3 Repo

Public GitHub repo (private during initial development). README leads with the killer command, links to `canva schema` for depth, and includes the shipped `CLAUDE.md` discoverable from the root.

## 12. Future Work (v2 — Pattern A)

Reserved for the next milestone, not in v1 scope:

- `canva sync` — full incremental mirror of designs + comments + assets to local SQLite.
- `canva search "query"` — FTS5 over titles + comment text + asset names.
- `canva comments archive` — archive comment threads with author + timestamp.
- `canva assets upload <file>` and full asset library mirror.
- Webhook listener for real-time cache invalidation.

The v1 cache schema is forward-compatible with v2 — adding FTS5 virtual tables and additional indices does not require migration of existing rows.

## 13. Open Questions Resolved During Brainstorming

| Question | Resolution |
|---|---|
| Language | Go (single binary, Homebrew, no runtime) |
| Audience | Open source, Homebrew tap, GitHub releases |
| Killer command | `canva create --template <t> --autofill data.json` |
| v1 vs v2 split | Pattern B in v1; Pattern A in v2 |
| Architecture | Name-resolution with local SQLite cache (Approach 2) |
| Auth | OAuth 2.0 PKCE with browser handoff |
| Output mode | Auto-JSON for non-TTY, structured NDJSON for lists |

## 14. Success Criteria

v1 ships when:

1. All commands in §4 are implemented and pass integration tests.
2. `canva schema --json` is consumed by Claude Code in a real session and the agent successfully completes a "create design from template + export PDF" workflow with zero `--help` reads.
3. Homebrew install works on macOS arm64 + amd64.
4. README + `CLAUDE.md` are complete; a new user can go from zero to first design in under 5 minutes.
5. Agent-friendliness invariant test in CI passes on every command.
