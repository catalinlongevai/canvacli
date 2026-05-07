# Changelog

All notable changes to canvacli are documented here.

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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
