# canvacli

[![release](https://img.shields.io/github/v/release/catalinlongevai/canvacli)](https://github.com/catalinlongevai/canvacli/releases)
[![CI](https://github.com/catalinlongevai/canvacli/actions/workflows/ci.yml/badge.svg)](https://github.com/catalinlongevai/canvacli/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Agent-first CLI for the Canva Connect API. Programmatic design generation, exports, and management — designed for AI coding agents (Claude Code, Cursor) and humans.

## Quick start

```bash
brew install catalinlongevai/tap/canvacli
canva login
canva list --limit 5
```

That's it. The release binary embeds the OAuth client credentials so you only need a Canva account, not a developer app.

## Why

Canva's official `@canva/cli` is for building Canva Apps. There's no CLI for *using* Canva — listing designs, exporting them, generating designs from brand templates programmatically. `canvacli` fills that gap, with stable JSON output, structured errors with `fix` actions, and a hand-curated schema for agent introspection.

## What it does

| Command | Description |
|---|---|
| `canva login` / `logout` / `whoami` | OAuth 2.0 PKCE auth |
| `canva list [--limit N] [--fields ...]` | NDJSON listing of your designs |
| `canva export <name\|id> --format pdf` | Export a design (eager download to disk) |
| `canva folders` | Walk your folder tree |
| `canva templates` (+ `show <name\|id>`) | Brand templates + autofill datasets (Enterprise) |
| `canva create --template T --autofill data.json` | Generate a design from a template (Enterprise) |
| `canva schema [--compact\|--full]` | Print the CLI surface as JSON for agents |
| `canva sql "SELECT ..."` | Read-only SQL against the local cache |

## Install

```bash
brew install catalinlongevai/tap/canvacli
```

Or download a static binary for your platform from [Releases](https://github.com/catalinlongevai/canvacli/releases).

Supported platforms: macOS (arm64, amd64), Linux (amd64, arm64), Windows (amd64).

## Examples

```bash
# Find designs containing "Q3" in the title (uses local SQLite cache)
canva sql "SELECT id, title FROM designs WHERE title LIKE '%Q3%'"

# Generate a personalised social post from JSON (Enterprise)
echo '{"headline":"Welcome","name":"Alice"}' | \
  canva create --template "Welcome Post" --autofill -

# Export every design in your account to PDFs
canva list --limit 100 --fields id | \
  jq -r '.id' | \
  while read id; do canva export "$id" --format pdf --output "$id.pdf"; done
```

## Output convention

When stdout is **piped or redirected**, output is always JSON or NDJSON. When stdout is a **TTY**, output is human-readable. Errors are always structured JSON with a `fix` field telling you how to recover:

```json
{"error":"design_not_found","message":"...","fix":"canva list --json | grep -i banner","exit_code":3}
```

See [CLAUDE.md](./CLAUDE.md) for a Claude-Code-ready brief.

## Enterprise dependency

`canva create` and `canva templates` rely on Canva Connect endpoints gated to **Canva Enterprise** customers. The other 8 commands work on any Canva account (free or Pro).

## Building from source

If you're building from source and want to run `canva login`, you need to register your own Canva developer app first and set env vars:

```bash
export CANVA_CLIENT_ID="..."
export CANVA_CLIENT_SECRET="..."
go run ./cmd/canvacli login
```

Required app config: PKCE enabled, redirect URIs `http://127.0.0.1:8765/callback`, `:8766/callback`, `:8767/callback`. Scopes: `design:meta:read design:content:read design:content:write brandtemplate:meta:read brandtemplate:content:read folder:read profile:read`.

## License

MIT.
