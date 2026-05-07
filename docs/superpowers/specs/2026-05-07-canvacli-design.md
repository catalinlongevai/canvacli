# canvacli — Design Specification

**Date:** 2026-05-07
**Status:** Approved (brainstorming complete, awaiting implementation plan)
**Author:** Catalin Niculescu

## 1. Overview

`canvacli` is a Go-based command-line interface for Canva, distributed as a single static binary via Homebrew and GitHub releases. It provides programmatic access to the Canva Connect API for users (designers, developers, AI coding agents) — a category currently unserved.

The official `@canva/cli` exists but is scoped to building Canva Apps (developer SDK tooling). It does not expose the user-facing design management surface (create from templates, export, list designs, manage assets). `canvacli` fills that gap.

**Enterprise dependency disclosure.** The killer command (`canva create --template ... --autofill`) and `canva templates` rely on Canva Connect API endpoints that are gated to **Canva Enterprise** customers. Non-Enterprise users can still use `canva login`, `canva whoami`, `canva list`, `canva export`, and `canva folders`. The README and a startup probe must surface this clearly.

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
   ├─ dedupe/     check local job-history table for matching idempotency key
   ├─ api/        POST https://api.canva.com/rest/v1/autofills
   │              body: { brand_template_id, data }
   ├─ api/        poll GET /v1/autofills/{job_id} until job.status=success
   ├─ cache/      upsert new design row + record idempotency key
   └─ output/     emit JSON (piped) or table (TTY)
```

**Idempotency is implemented client-side.** The Canva Connect API does not support an `Idempotency-Key` HTTP header. Therefore `--idempotency-key <key>` is honored by `canvacli` itself: a local SQLite table records `(key, command, args_hash, design_id, created_at)`, and a re-invocation with the same key short-circuits to return the prior result. This protects agents against duplicate creates from transient retries, but the protection is best-effort — a fresh machine has no history.

### 3.3.1 Async job pattern (used everywhere)

Autofill, export, asset upload, and design import all return the same envelope:

```json
{ "job": { "id": "...", "status": "in_progress|success|failed", "result": {...}, "error": {...} } }
```

A single `pollJob[T]` generic helper handles all of them with configurable timeout, max attempts, and exponential backoff (250ms → 500ms → 1s, capped at 5s).

### 3.4 Storage paths

Resolved via `os.UserConfigDir()` for cross-platform correctness:

| Platform | Base path | Examples |
|---|---|---|
| Linux | `$XDG_CONFIG_HOME` or `~/.config` | `~/.config/canvacli/token.json` |
| macOS | `~/Library/Application Support` | `~/Library/Application Support/canvacli/token.json` |
| Windows | `%AppData%` | `C:\Users\<u>\AppData\Roaming\canvacli\token.json` |

| File | Purpose | Permissions (Unix) |
|---|---|---|
| `canvacli/token.json` | OAuth tokens (access + refresh) | `0600` |
| `canvacli/cache.db` | SQLite cache + idempotency history | `0600` |
| `canvacli/config.toml` | User settings (default folder, output mode override) | `0644` |

**Permissions caveat.** On Windows, `os.Chmod(0o600)` is a silent no-op. Strict ACL hardening is deferred as a known limitation; `token.json` on Windows relies on the user's profile ACLs being correctly inherited. Documented in README.

The cache is `0600` (not `0644` as a draft suggested) because it contains design titles, folder structure, and idempotency history that should not be readable by other local users.

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
  [--output ./path]                                async export job; downloads
                                                   to disk eagerly (URL expires
                                                   in 24h)
canva folders                                      walk folder tree from "root"
                                                   and "uploads" special folders
```

**`canva delete` is intentionally absent from v1.** Canva Connect does not expose `DELETE /designs/{designId}` (verified against the OpenAPI spec). A "soft delete by moving to a `.canvacli/trash` folder" workaround was considered and rejected for v1 because (a) it conflates user intent with implementation hack, (b) there is no auto-empty story, (c) it would surprise agents that expect `delete` to actually delete. The non-availability is recorded; if Canva ships a real DELETE endpoint, we add the command then.

`canva folders` does **not** map to a single endpoint. The implementation walks from special folder IDs `root` (user's projects root) and `uploads` (Uploads folder) using `GET /folders/{id}/items` recursively, and emits the resulting tree as flat NDJSON.

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

1. `canva login` generates a code verifier + S256 challenge using `oauth2.GenerateVerifier()` and a 32-byte random `state` parameter (CSRF defense).
2. Opens default browser to `https://www.canva.com/api/oauth/authorize` with `client_id`, `code_challenge`, `code_challenge_method=s256`, `state`, `scope`, `redirect_uri`.
3. Spawns local HTTP listener on a **fixed port from a fallback list** (`8765`, `8766`, `8767` — first available wins). Canva requires registered redirect URIs and does not support wildcard ports; the redirect URIs `http://127.0.0.1:8765/callback`, `:8766/callback`, `:8767/callback` are pre-registered with the Canva developer app.
4. User approves in browser; Canva redirects to local listener with authorization code + state.
5. CLI **validates the returned `state` against the generated value**; mismatches abort with a CSRF error.
6. CLI POSTs to `https://api.canva.com/rest/v1/oauth/token` with HTTP Basic auth (`client_id:client_secret`) and `code`, `code_verifier`, `redirect_uri`, `grant_type=authorization_code`.
7. Tokens persisted via atomic write to `<config-dir>/canvacli/token.json` (mode `0600`).
8. Listener shuts down with a 5-minute hard timeout for the whole flow.

**Public-client question.** Canva's OAuth token endpoint requires HTTP Basic auth on confidential clients, which traditionally means embedding a client secret in the binary. Embedding secrets in a public CLI is industry-standard for first-party developer tools (e.g. `gh`, `gcloud`) but the secret is best-effort, not a real auth boundary. The CLI uses a registered Canva developer app whose secret is shipped in the binary and rotated via release.

### 5.2 Refresh strategy

Layered approach:

1. **`oauth2.ReuseTokenSource`** — handles clock-driven refresh transparently when an access token has expired (Canva access tokens last 4 hours; `expires_in: 14400`).
2. **`persistingSource` wrapper** — every successful refresh writes the new token pair to `token.json` atomically (Canva rotates refresh tokens; the old one becomes invalid).
3. **`refreshOn401Transport`** — a custom `http.RoundTripper` that retries on 401 once after a forced refresh, catching server-side revocation that expiry-based refresh misses. Body is replayed via `req.GetBody`.

If refresh fails, structured error `{"error":"auth_revoked","fix":"canva login","exit_code":2}` is surfaced. Single retry attempt only — no retry storm.

### 5.3 Scopes (verified against Canva Connect OpenAPI)

Minimum scope set for v1 surface, exactly as Canva expects them:

```
design:meta:read
design:content:read
design:content:write
brandtemplate:meta:read
brandtemplate:content:read
folder:read
profile:read
```

Removed from earlier draft (no v1 commands need them): `asset:read`, `asset:write`, `folder:write`, `comment:read`, `comment:write`. These come back in v2 when Pattern A and asset upload land.

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
  thumbnail_url TEXT,             -- expires after 15 minutes
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

CREATE TABLE idempotency (
  key TEXT PRIMARY KEY,
  command TEXT NOT NULL,
  args_hash TEXT NOT NULL,
  result_json TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);

CREATE INDEX idx_designs_title ON designs(title);
CREATE INDEX idx_templates_title ON templates(title);
CREATE INDEX idx_idempotency_created ON idempotency(created_at);
```

The `idempotency` table backs the client-side dedupe described in §3.3. Entries older than 30 days are pruned at startup. FTS5 virtual tables intentionally deferred to v2 (Pattern A).

### 6.3 Resolution algorithm

Given a `<name|id>` argument:

1. Try cache lookup by `id = ?` first (covers most agent calls that pass IDs verbatim).
2. If no ID hit, query cache by `WHERE title = ? COLLATE NOCASE LIMIT 2`.
   - 1 match → use it.
   - 2+ matches → return `multiple_matches` error with `suggestions: [...]`.
3. If still no match in cache: fall back to API. List the resource type (designs / templates) and check whether the input matches an ID returned by the API; if not, do a name match against API results.
4. If the API call returns 2+ name matches, surface `multiple_matches` with `suggestions` populated from API results, not just cache.
5. `--no-cache` skips steps 1–2 and goes directly to API.

This avoids relying on a guessed regex for Canva's ID format — the cache and the API together are the source of truth.

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

### 7.6 `canva sql` escape hatch (security model)

When a needed query isn't supported by a dedicated command, agents can write SQL directly against the cache. Schema is documented in `CLAUDE.md` and via `canva sql --schema`.

**Security constraints (mandatory):**

- **Read-only.** SQLite connection is opened with `?_pragma=query_only(true)` and the URI flag `_txlock=deferred`. `INSERT`, `UPDATE`, `DELETE`, `CREATE`, `DROP`, `ALTER`, `REPLACE` all error out at the engine level, not at parse time.
- **Statement allowlist.** Before submitting to SQLite, the input is parsed and rejected unless its top-level node is `SELECT` or `WITH ... SELECT`. Multi-statement input (`;` separated) is rejected.
- **`ATTACH` and `PRAGMA` blocked.** Both at parser level (rejected) and connection level (`ATTACH` requires write).
- **Result size cap.** Default 500 rows, override with `--limit` up to 10,000. Prevents an agent from accidentally pulling 100k rows into context.
- **Timeout.** 5-second statement timeout via SQLite's progress handler.

Read-only is the default *and* the only mode. There is no `--write` flag. If an agent needs to mutate the cache, it must do so via the dedicated commands, which go through the API and update the cache as a side-effect.

### 7.7 Idempotency + dry-run

- All mutating commands (currently just `create` in v1) accept `--idempotency-key`. Implemented **client-side** via the `idempotency` table — the Canva Connect API has no native idempotency header. Re-invoking with the same key short-circuits to return the prior result. Best-effort: a fresh machine has no history.
- All mutating commands accept `--dry-run`, which returns the exact HTTP request body that *would* be sent. `--dry-run` reads stdin/file inputs normally — interactions with autofill data via stdin are honored so the dry-run output reflects exactly what would happen.

### 7.8 Provenance of `fix` field (security)

The `fix` field in error responses is **always populated by canvacli itself**, never by data from the Canva API or user input. This eliminates a prompt-injection vector where a malicious server response or design title could inject an instruction the agent then executes. Implementation: a switch statement mapping error codes → static fix strings; no string interpolation from upstream data into `fix`.

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

### 9.3 Rate limit handling

Canva Connect does not document `X-RateLimit-*` headers. The CLI handles rate limits reactively:

- On `429`, read the `Retry-After` header (seconds) and surface error code 6 with `wait_seconds: <value>` in the JSON envelope.
- A new universal flag `--auto-wait` (off by default) makes `canvacli` block and retry once after the indicated wait, capped at 60 seconds. Agents that want fully automatic behavior opt in; default is fail-fast so the agent decides.

Known per-user rate ceilings (worth documenting in `CLAUDE.md` so agents pace themselves):

| Endpoint | Limit |
|---|---|
| `users/me` | 10 rpm |
| `POST /autofills` | 60 rpm (Enterprise-only) |
| `POST /exports` | 20 rpm + 75/5min + 500/24h |
| `GET /exports/{id}` | 120 rpm |
| `POST /designs` | 20 rpm |

### 9.4 Export polling and timeouts

`canva export` calls `POST /exports`, then polls `GET /exports/{id}` until `job.status` is `success` or `failed`.

- **Polling cadence:** 250ms → 500ms → 1s → 2s → 5s, then steady at 5s.
- **Default timeout:** 5 minutes total wall-clock. Override with `--timeout 10m`.
- **On timeout:** surface `export_pending` error with the job ID so an agent can resume via `canva export --resume <job-id>` (a v1.1 stretch feature; v1 surfaces the ID and the agent re-polls manually with `canva sql` if needed).
- **Eager download.** Export download URLs expire 24h after generation but `canvacli` downloads to disk immediately on `success` and never returns the bare URL by default. `--url-only` returns the URL without downloading, with a warning that it expires in 24h.

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

### 10.3.1 Schema token-size budget

A CI step asserts:

- `canva schema --compact | wc -c` ≤ 4 KB (~1000 tokens)
- `canva schema --full | wc -c` ≤ 16 KB (~4000 tokens)

If either budget is exceeded, the build fails. This prevents the schema from silently bloating as commands are added — the whole point of the agent-friendly design is that the schema fits comfortably in agent context.

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

## 13. Open Questions Resolved During Brainstorming + Research

| Question | Resolution |
|---|---|
| Language | Go (single binary, Homebrew, no runtime) |
| Audience | Open source, Homebrew tap, GitHub releases |
| Killer command | `canva create --template <t> --autofill data.json` |
| v1 vs v2 split | Pattern B in v1; Pattern A in v2 |
| Architecture | Name-resolution with local SQLite cache (Approach 2) |
| Auth | OAuth 2.0 PKCE with browser handoff |
| Output mode | Auto-JSON for non-TTY, structured NDJSON for lists |
| Connect API base URL | `https://api.canva.com/rest/v1` |
| Token endpoint | `https://api.canva.com/rest/v1/oauth/token` (HTTP Basic) |
| Authorization endpoint | `https://www.canva.com/api/oauth/authorize` |
| Access token lifetime | 4 hours (refresh tokens rotate, single-use) |
| Idempotency strategy | Client-side SQLite table; Canva has no `Idempotency-Key` header |
| `canva delete` | Removed from v1 — no DELETE endpoint exists |
| Storage path | `os.UserConfigDir()` (platform-correct paths) |
| OAuth callback port | Fixed fallback list `8765/8766/8767` (Canva requires registered URIs) |
| SQL escape hatch security | Read-only, parser allowlist, no ATTACH/PRAGMA, 500-row default cap |

## 14. Success Criteria

v1 ships when:

1. All commands in §4 are implemented and pass integration tests against recorded HTTP cassettes.
2. `canva schema --json` is consumed by Claude Code in a real session and the agent successfully completes a "create design from template + export PDF" workflow with zero `--help` reads. The session is recorded and committed under `docs/agent-evidence/`.
3. Homebrew install works on macOS arm64 + amd64; binaries also published for linux-amd64, linux-arm64, windows-amd64.
4. README + `CLAUDE.md` are complete; a new user with Canva Enterprise can go from zero to first generated design in under 5 minutes.
5. Agent-friendliness invariant test in CI passes on every command.
6. Schema token-size budget passes (compact ≤ 4 KB, full ≤ 16 KB).
7. SLSA Level 3 provenance attached to every published release.

## 15. Known Risks

| Risk | Likelihood | Mitigation |
|---|---|---|
| Canva Enterprise gate excludes most early adopters from killer command | High | Disclose prominently; list endpoints work for everyone; probe at startup with friendly fail |
| Canva ships breaking API changes | Medium | Pin to OpenAPI version; cassette tests catch shape changes; semver major bump policy |
| Embedded client secret in public binary leaked or rotated | Low impact | Public clients are industry-standard; rotate via release; user secrets are not shared |
| `modernc.org/sqlite` lags pure C SQLite features | Low | Acceptable for cache-only workload; v2 may revisit if FTS5 advanced features needed |
| Fixed redirect ports (8765–8767) clash with another local service | Low | Three-port fallback; clear error message if all three are busy |
| GoReleaser deprecates `brews:` for v3 | Medium (timing unknown) | Migrate to `homebrew_casks:` + a parallel Linux distribution path when v3 ships |
