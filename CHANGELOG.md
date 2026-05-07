# Changelog

All notable changes to canvacli are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- `canva mcp serve` — run an MCP (Model Context Protocol) server over stdio for Claude Desktop, Cursor, and other MCP-capable agents. Exposes six tools that reuse the existing API client and cache without shelling out: `canva_whoami`, `canva_list`, `canva_folders`, `canva_export`, `canva_sql`, `canva_schema`. Reads the same token store as the CLI — run `canva login` first.

### Skipped (deferred to a future release)
- `canva_create` and `canva_templates` MCP tools — Enterprise-gated and (for `create`) destructive. To be added with explicit confirmation gating.

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

[Unreleased]: https://github.com/catalinlongevai/canvacli/compare/v1.0.0...HEAD
[v1.0.0]: https://github.com/catalinlongevai/canvacli/releases/tag/v1.0.0
