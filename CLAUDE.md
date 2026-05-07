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
| `canva mcp serve` | Run an MCP server over stdio (Claude Desktop / Cursor / agents) |

## MCP server

Instead of shelling out to `canva`, MCP-capable clients can call canvacli's tools natively. Add `{"mcpServers":{"canva":{"command":"canva","args":["mcp","serve"]}}}` to your Claude Desktop or Cursor config and the agent gets `canva_whoami`, `canva_list`, `canva_folders`, `canva_export`, `canva_sql`, and `canva_schema` as first-class tools. The server reads the same token store as the CLI — run `canva login` from the terminal once before starting the agent.

## Global flags

- `--json` — force JSON output (auto-on when stdout is piped)
- `--no-cache` — bypass local cache, force API call
- `--quiet` — suppress progress
- `--auto-wait` — auto-retry once on 429 (capped at 60s)

## Output convention

When stdout is **piped or redirected**, output is JSON or NDJSON. When stdout is a **TTY**, output is human-readable. Errors are always structured JSON to stderr:

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
