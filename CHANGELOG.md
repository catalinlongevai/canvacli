# Changelog

All notable changes to canvacli are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v2.0.0] - 2026-05-07

Major release. Adds Pattern A (local mirror + FTS5 search), expanded API surface, and full MCP coverage. The agent-driven Canva surface is now substantially complete.

### Added — Pattern A (local mirror + search)

- `canva sync` — all-in-one mirror of designs, folders, templates, comments, and assets into the local SQLite cache. Idempotent and re-runnable; first sync ~30–60s for typical accounts, subsequent syncs incremental via cursor. Per-resource progress to stderr, final summary to stdout.
- `canva search "query"` — FTS5 search across mirrored designs, templates, comment threads, comment replies, and assets. BM25 ranking, NDJSON output. `--type` filter, `--limit` (1–1000, default 50). Reads through a separate read-only SQLite handle with `query_only(true)` for engine-level safety.

### Added — expanded API surface

- `canva pages <design>` — list pages of a design with dimensions and thumbnail URLs. NDJSON output. (Note: backed by Canva's preview Pages API; subject to upstream change.)
- `canva export <id> --pages 1,3,5` — extends the v1 `export` command to support per-page export on PNG/JPG/PDF/GIF formats.
- `canva import <file>` — import a local PDF, PPTX, DOCX, Keynote, AI/PSD, Affinity, or OpenOffice file as a Canva design. Image files (PNG/JPG/JPEG) are auto-routed to `canva assets upload` since Canva uses different endpoints. `--mime-type` overrides extension sniffing; `--list-formats` prints the routing table.
- `canva resize <design> --to <preset>` — resize a design. Strict 4-preset enum: `doc`, `email`, `presentation`, `whiteboard`. Always creates a new design; the original is unchanged. Surfaces trial-quota information for free-tier users.
- `canva assets upload <file>` — upload an image or asset to your Canva asset library. Returns the asset ID, which can be referenced in subsequent `canva create --autofill` calls (the agent-driven deck workflow).
- `canva comments add <design> "text" [--reply-to <thread>]` — add a top-level comment thread or reply to one.
- `canva comments thread <thread-id>` — fetch a specific thread plus all replies.
- `canva comments archive [--design <id>]` — emit the local thread cache (with replies) as a single archive structure. Operates on the local thread cache by design — Canva Connect has no list-threads endpoint; threads enter the cache via `canva comments add` or `canva comments thread`.

### Added — MCP integration (full v2 coverage)

13 new MCP tools (in addition to the 6 from v1.1):

- `canva_sync`, `canva_search` — Pattern A core
- `canva_pages`, `canva_resize`, `canva_import` — page-level + transform
- `canva_assets_upload` — asset library
- `canva_comments_add`, `canva_comments_reply`, `canva_comments_thread`, `canva_comments_archive` — comment threads
- `canva_create`, `canva_templates_list`, `canva_templates_show` — Enterprise-gated, deferred from v1.1

All read-only tools correctly annotated `readOnlyHint: true, destructiveHint: false`. Mutating tools are not marked destructive (idempotent autofill creates can be re-run safely; `canva_create` is the one exception with `destructiveHint: true`).

### Added — infrastructure

- Local SQLite cache extended with `design_pages`, `comment_threads`, `comment_replies`, `assets`, and `sync_state` tables. FTS5 virtual tables (`designs_fts`, `templates_fts`, `comment_threads_fts`, `comment_replies_fts`, `assets_fts`) auto-populated via triggers. `schema_version = 2` recorded in the `meta` table for future migration tracking.
- `--debug` global flag (added in v1.x) now traces v2 endpoints too.

### Changed

- OAuth scope set extended from 7 to 11: adds `comment:read`, `comment:write`, `asset:read`, `asset:write`. **Existing v1 users must `canva logout && canva login` to refresh their token with the new scopes** before commands that touch comments or assets work.
- Binary name remains `canva` (formula `canvacli`); install via `brew install catancs/tap/canvacli` is unchanged.

### Known limitations

- `canva comments archive` only sees threads the user has interacted with locally. Threads created via the Canva web UI are invisible until the user runs `canva comments thread <id>` once to seed the cache.
- `canva assets list` is intentionally absent — Canva Connect has no `/v1/assets` listing endpoint.
- `canva pages` uses Canva's preview Pages API; Canva has flagged this for breaking changes. Pinned to a stable shape but watch the release notes.
- Brand Kit endpoint does not exist in Canva Connect; agent-driven deck generation relies on brand templates (which do encode brand-compliant styling) instead.
- `canva sync` cannot detect server-side deletions in real time — falls back to a 24h full reconcile.

### Architecture notes

- The Pattern A mirror is purely additive on the v1 schema. v1 cache files migrate seamlessly via `CREATE IF NOT EXISTS`.
- All v2 commands reuse the v1 OAuth + transport layers (token refresh, 401 retry, debug logging).
- Cross-package consistency: every v2 resource has API client + cache + CLI command + MCP tool — built in parallel by four resource-owning agents.

## [v1.1.0] - 2026-05-07

### Added
- `canva mcp serve` — run an MCP (Model Context Protocol) server over stdio for Claude Desktop, Cursor, and other MCP-capable agents. Exposes six tools that reuse the existing API client and cache without shelling out: `canva_whoami`, `canva_list`, `canva_folders`, `canva_export`, `canva_sql`, `canva_schema`. Reads the same token store as the CLI — run `canva login` first.
- `--debug` global flag — logs HTTP method, URL, status, and duration to stderr. Bodies are never logged.

### Fixed
- MCP tool annotations: read-only tools now correctly declare `readOnlyHint: true, destructiveHint: false` (was incorrectly `destructiveHint: true` from the SDK default).

## [v1.0.0] - 2026-05-07

First stable release.

### Added
- 10 commands: `login`, `logout`, `whoami`, `templates` (+ `show`), `create`, `list`, `export`, `folders`, `schema`, `sql`
- OAuth 2.0 PKCE auth with embedded client credentials — no developer-app setup required for end users
- Local SQLite cache for name resolution and client-side idempotency
- Read-only SQL escape hatch (`canva sql`) with parser allowlist + engine-level `query_only(true)` defense in depth
- Hand-curated schema export (`canva schema --compact|--full`) for agent introspection
- Stable error envelope with `error` codes, `fix` field, and consistent exit codes
- Auto-JSON output when stdout is piped; NDJSON for lists
- Cross-platform binaries for macOS (arm64, amd64), Linux (amd64, arm64), Windows (amd64)
- Homebrew tap distribution

### Known limitations
- `canva create` and `canva templates` require Canva Enterprise (Canva-side gate)
- `canva delete` is not available — Canva Connect has no DELETE endpoint
- Cassette-based integration tests deferred to v1.x

[Unreleased]: https://github.com/catalinlongevai/canvacli/compare/v2.0.0...HEAD
[v2.0.0]: https://github.com/catalinlongevai/canvacli/releases/tag/v2.0.0
[v1.1.0]: https://github.com/catalinlongevai/canvacli/releases/tag/v1.1.0
[v1.0.0]: https://github.com/catalinlongevai/canvacli/releases/tag/v1.0.0
