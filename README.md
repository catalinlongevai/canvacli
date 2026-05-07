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

## Quick start (release binary)

```bash
brew install catalinlongevai/tap/canvacli
canva login
```

That's it. The release binary embeds the OAuth client credentials so you only need a Canva account, not a developer app.

## Setup for local development (building from source)

If you're building from source and want to run `canva login`, you'll need to register a Canva developer app yourself and set environment variables before running:

```bash
export CANVA_CLIENT_ID="..."
export CANVA_CLIENT_SECRET="..."
go run ./cmd/canvacli login
```

Required dev-app config: PKCE enabled, redirect URIs `http://127.0.0.1:8765/callback`, `:8766/callback`, `:8767/callback`, scopes `design:meta:read design:content:read design:content:write brandtemplate:meta:read brandtemplate:content:read folder:read profile:read`.

## Agent integration

```bash
# Drop the schema into your agent's context once
canva schema --full > .canvacli-schema.json

# The agent can now invoke any command without reading --help
```

See [`CLAUDE.md`](./CLAUDE.md) for a Claude-Code-ready brief.

## License

MIT.
