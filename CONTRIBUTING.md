# Contributing to canvacli

Thanks for your interest in improving canvacli. This guide covers the practical bits.

## Getting started

Clone the repo and build a local binary:

```bash
git clone https://github.com/catalinlongevai/canvacli.git
cd canvacli
go build -o canva ./cmd/canvacli
```

For local OAuth testing you need to register your own Canva developer app and export `CANVA_CLIENT_ID` / `CANVA_CLIENT_SECRET`. See README's "Building from source" section for the required app config (PKCE, redirect URIs, scopes).

## Development workflow

- Branch off `master`. Use short topical names (`feat/sql-window-fns`, `fix/export-poll-jitter`).
- Commits follow [Conventional Commits](https://www.conventionalcommits.org/). Look at `git log --oneline` for examples — common prefixes are `feat`, `fix`, `docs`, `ci`, `build`, `refactor`. Scope is optional but encouraged (`feat(mcp): ...`).
- One logical change per commit. Squash fixup commits before opening a PR.
- Open the PR against `master`. Reference the issue number if one exists.

## Testing

Run unit tests:

```bash
go test ./...
```

This includes the cassette-based integration tests in `internal/api/cassette_test.go`, which replay recorded Canva API responses from `internal/api/testdata/cassettes/*.yaml`. CI runs them fully offline.

For commands that lack cassette coverage, manually exercise any non-trivial change against your own Canva account before requesting review. At minimum: `canva login`, the command you touched, and `canva whoami` to confirm token state is intact.

## Re-recording integration cassettes

`internal/api/cassette_test.go` runs against pre-recorded HTTP fixtures in `internal/api/testdata/cassettes/`. To re-record from live Canva (e.g. after Canva ships a breaking API change):

1. Run `canva login` to refresh your access token.
2. Re-record specific tests:
   ```bash
   CANVACLI_RECORD=1 go test ./internal/api/ -run TestVCR_Me -v
   ```
3. Inspect the resulting cassette at `internal/api/testdata/cassettes/<name>.yaml` — verify `Authorization: Bearer REDACTED` and no real tokens leaked into bodies.
4. Commit the updated cassette files.

## Architecture overview

Two documents are required reading before structural changes:

- `docs/superpowers/specs/2026-05-07-canvacli-design.md` — design spec covering the agent-first contract, error envelope, output mode rules, and SQL surface.
- `docs/research/` — Canva Connect API notes, OAuth PKCE patterns, release pipeline.

## Adding a new command

1. Implement in `internal/commands/<name>.go`. Follow the shape of an existing command (`list.go` for read, `create.go` for write).
2. Register the cobra command in `internal/commands/root.go`.
3. Add an entry to both the compact and full schemas in `internal/commands/schema.go`. The full schema is what agents consume — flags, types, and error codes must be accurate.
4. Update the command table in `README.md`.

## Adding a new MCP tool

1. Register the tool in `internal/mcp/server.go` alongside the existing six.
2. Update `schemaFull` in `internal/mcp/schemas.go` so MCP clients see correct input/output types.
3. Add a row to the MCP tool table in `README.md`.

MCP tools should map cleanly to existing CLI commands where possible — they share the same auth and cache layers.

## Code review expectations

- Small, focused PRs. One logical change per PR is ideal.
- Tests for non-trivial logic. Pure plumbing (flag wiring, struct field additions) does not need a test.
- No breaking changes to the JSON output envelope, error codes, or exit codes without a major version bump. These are part of the agent contract.
- New flags default off; new commands default safe.

## Issue reports

When filing a bug, include:

- canvacli version (`canva --version`)
- OS and architecture
- The exact command you ran (with secrets redacted)
- The full structured error envelope from stderr (the JSON object with `error`, `message`, `fix`, `exit_code`)

That envelope is the most useful single artifact — it pins down the failure class and what the tool tried to suggest.
