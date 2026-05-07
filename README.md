# canvacli

[![release](https://img.shields.io/github/v/release/catalinlongevai/canvacli)](https://github.com/catalinlongevai/canvacli/releases)
[![CI](https://github.com/catalinlongevai/canvacli/actions/workflows/ci.yml/badge.svg)](https://github.com/catalinlongevai/canvacli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/catalinlongevai/canvacli.svg)](https://pkg.go.dev/github.com/catalinlongevai/canvacli)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

> **Programmatic Canva. From the terminal.**
> Built for AI coding agents and the humans who pair with them.

```bash
brew install catancs/tap/canvacli
canva login
canva list --limit 5
```

That is the install. No developer account, no env vars, no keys to copy. Just a Canva login and a working terminal.

---

## What it gives you

Canva ships an official `@canva/cli` for *building* Canva apps. There has never been a CLI for *using* Canva — listing your designs, generating new ones from brand templates, exporting them to PDFs, working with your folders, all from the shell. **canvacli** is that tool.

It is also the first such tool designed specifically for AI coding agents (Claude Code, Cursor, GPT, any MCP-capable agent) to invoke autonomously. Every output is structured JSON, every error has a `fix` field telling the agent what to try next, every command introspects via `canva schema`.

### How it compares

| | canvacli | Canva web app | Official `@canva/cli` |
|---|---|---|---|
| Designed for browsing | yes | yes | no |
| Designed for scripting | yes | no | apps only |
| Generates designs from data | yes | manual | no |
| Pipes into shell tools | yes | no | partial |
| Drives from an AI agent | yes (first-class) | no | partial |
| Local cache for offline grep | yes (SQLite) | no | no |
| Distribution | one-line `brew install` | web app | npm + Node runtime |

---

## What you can do with it

```bash
# Find every design with "Q3" in the title — instant, queries the local SQLite cache
canva sql "SELECT id, title FROM designs WHERE title LIKE '%Q3%'"

# Generate 100 personalised social posts from a CSV
xargs -a recipients.csv -d '\n' -I {} \
  canva create --template "Welcome Card" --autofill <(echo "{\"name\":\"{}\"}")

# Export every design in a folder to PDFs
canva list --fields id --limit 100 \
  | jq -r '.id' \
  | while read id; do canva export "$id" --format pdf --output "$id.pdf"; done

# Pipe a design summary into Claude Code
canva list --fields title,updated_at | claude -p "summarise my recent design activity"
```

---

## Commands

| Command | Description |
|---|---|
| `canva login` / `logout` / `whoami` | OAuth 2.0 PKCE auth + identity |
| `canva list [--limit N] [--fields ...]` | NDJSON listing of your designs |
| `canva export <name\|id> --format pdf` | Async export; downloads eagerly to disk |
| `canva folders` | Walks your folder tree |
| `canva templates` (+ `show <name\|id>`) | Brand templates and their autofill schemas (Enterprise) |
| `canva create --template T --autofill data.json` | Generate a design from a template (Enterprise) |
| `canva schema [--compact\|--full]` | Print the full CLI surface as JSON for agent introspection |
| `canva sql "SELECT ..."` | Read-only SQL against the local cache |

Run `canva --help` or `canva <command> --help` for full flag listings, or `canva schema --full` for the JSON-typed surface.

---

## Built for agents

```bash
# Drop the schema into your agent's context once
canva schema --full > .canvacli-schema.json

# The agent now knows every command, every flag, every error code
```

When an agent runs a canvacli command and stdout is piped, output is automatically structured JSON or NDJSON — no `--json` flag needed. Errors are always JSON to stderr:

```json
{
  "error": "design_not_found",
  "message": "no design matched 'Q3 Bannr'",
  "suggestions": ["abc123 (Q3 Banner)"],
  "fix": "canva list --json | grep -i banner",
  "exit_code": 3
}
```

The `fix` field contains a literal command the agent can execute verbatim to recover. Combined with stable error codes and exit codes, agents can branch on failure mode without parsing prose. See [CLAUDE.md](./CLAUDE.md) for a Claude-Code-ready brief — copy it into your repo's `CLAUDE.md` and any agent immediately knows the tool.

---

## MCP server (Claude Desktop, Cursor, etc.)

canvacli ships a built-in MCP server. Add this to your Claude Desktop config (`~/Library/Application Support/Claude/claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "canva": {
      "command": "canva",
      "args": ["mcp", "serve"]
    }
  }
}
```

Then restart Claude Desktop. Six new tools become available:

| Tool | Description |
|---|---|
| `canva_whoami` | Authenticated user info |
| `canva_list` | List designs as JSON |
| `canva_folders` | List folders |
| `canva_export` | Export a design as PDF/PNG/JPG/MP4/GIF |
| `canva_sql` | Read-only SQL against the local cache |
| `canva_schema` | Return the full canvacli schema |

Run `canva login` from the terminal first; the MCP server reads the same token store as the CLI.

### Cursor

Same JSON, in `~/.cursor/mcp.json`.

---

## How it works

| Layer | Detail |
|---|---|
| **Distribution** | Single static Go binary, no runtime dependencies |
| **Auth** | OAuth 2.0 PKCE with embedded client credentials. Tokens persist with `0600` permissions in `~/Library/Application Support/canvacli/` (or `$XDG_CONFIG_HOME` on Linux) |
| **Refresh** | Automatic on 401, with rotation support — Canva uses single-use refresh tokens, persisted on every change |
| **Cache** | SQLite (`modernc.org/sqlite`, no CGo) with engine-level `query_only(true)` defense for the read-only SQL surface |
| **Output** | Auto-JSON when stdout is piped, NDJSON for lists, table for TTY |
| **Errors** | Stable `error` codes, structured envelope with `fix` field, distinct exit codes per failure class |
| **Cross-platform** | Pure-Go SQLite means `CGO_ENABLED=0` and one Linux runner builds the entire matrix |

Architecture details in [docs/superpowers/specs/](./docs/superpowers/specs/), API research in [docs/research/](./docs/research/).

---

## Install

### Homebrew (macOS, Linux)

```bash
brew install catancs/tap/canvacli
```

### Upgrading

```bash
brew update
brew upgrade canvacli
```

(Brew caches the tap formula list; `brew update` is required to discover new versions before `brew upgrade`.)

### Static binary (any platform)

Download from [Releases](https://github.com/catalinlongevai/canvacli/releases). Binaries provided for:
- macOS (`darwin_arm64`, `darwin_amd64`)
- Linux (`linux_amd64`, `linux_arm64`)
- Windows (`windows_amd64`)

```bash
curl -L https://github.com/catalinlongevai/canvacli/releases/latest/download/canvacli_$(uname -s)_$(uname -m).tar.gz | tar xz
sudo mv canva /usr/local/bin/
canva login
```

---

## Enterprise dependency

`canva create` and `canva templates` rely on Canva Connect endpoints gated to **Canva Enterprise** customers. The other 8 commands work on any Canva account (free or Pro). Free-tier users will see a structured `permission_denied` error (exit code 7) on the gated commands.

---

## Contributing

Pull requests welcome. Open an issue first for non-trivial changes so we can discuss design.

The codebase is documented in `docs/superpowers/specs/` (design spec) and `docs/research/` (Canva Connect API research, OAuth patterns, release pipeline notes). Read those before making structural changes.

### Building from source

For contributors who want to run their own build, register a Canva developer app first, then:

```bash
export CANVA_CLIENT_ID="..."
export CANVA_CLIENT_SECRET="..."
go run ./cmd/canvacli login
```

Required app config: PKCE enabled, redirect URIs `http://127.0.0.1:8765/callback`, `:8766/callback`, `:8767/callback`. Scopes: `design:meta:read design:content:read design:content:write brandtemplate:meta:read brandtemplate:content:read folder:read profile:read`.

The release binary embeds the maintainer's Canva client credentials via build-time `-ldflags`, so end users do not need to register their own developer app. This is the same pattern used by `gh`, `gcloud`, and other first-party CLIs.

---

## License

MIT. See [LICENSE](./LICENSE).
