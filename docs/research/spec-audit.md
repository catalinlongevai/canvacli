# Spec Audit — canvacli v1

## Critical issues (must fix before implementation)

- **§4.4 / §7.6 — `canva sql` is a SQL-injection-by-design hole.** The spec hands the agent raw SQL against a SQLite file containing OAuth-adjacent metadata. Two concrete risks: (a) `ATTACH DATABASE` can read/write arbitrary files at the process's permission, (b) write statements (`UPDATE`/`DELETE`/`DROP`) can corrupt the cache mid-flight under another command. Fix: open the connection in read-only mode (`?mode=ro`), reject statements whose first token is not `SELECT`/`WITH`/`EXPLAIN`, disable `ATTACH`/`PRAGMA` via `sqlite3_set_authorizer`, and apply a hard row + runtime cap (e.g. 1k rows, 2s).
- **§5.1 — OAuth callback binding and CSRF are unspecified.** "Random port on 127.0.0.1" omits: (a) the `state` parameter (CSRF defense — without it any local process can complete the flow with a forged code), (b) what happens if Canva requires a pre-registered exact redirect URI (most OAuth providers do; a *random* port won't work — you typically need a fixed port or a registered range). Verify Canva Connect's redirect URI policy *before* building, and document a `state` value plus single-use nonce.
- **§5.3 — Scope list is unverified and over-broad.** Twelve scopes are listed, including `comment:*` which contradicts §2's non-goal "comment archiving deferred to v2," and `asset:write` which has no v1 command needing it (no `assets upload` until v2 per §12). Requesting unused scopes will get the OAuth consent screen rejected by Canva review and scares users. Trim to the minimum that v1 commands actually exercise, and verify each scope name against the current Canva Connect docs (scope strings drift).
- **§3.3 / §7.7 — Idempotency-key claim is unverified.** "Server-side autofill jobs are keyed; agents that retry get the original design back" assumes Canva Connect honours `Idempotency-Key`. This is not universal — Stripe-style idempotency is the exception, not the rule. If Canva does not support it, `--idempotency-key` becomes a lie that silently creates duplicates on retry. Verify against Canva's API docs; if unsupported, implement client-side dedupe via the cache or remove the flag.

## Should fix (improve before implementation)

- **§3.4 — `cache.db` at `0644` leaks design titles/IDs to other local users.** Cache contains `raw_json` of designs (titles, thumbnails, folder structure). On a shared machine this is readable by anyone. Match `token.json` at `0600`, or document why not.
- **§6 vs §7.6 — Cache freshness contract contradicts SQL escape hatch.** §6 says "stale cache never causes a wrong answer." But `canva sql` returns whatever is in the cache, including rows older than TTL and rows for designs the user has since deleted in the web UI. Either document this caveat in `--schema` output, or expose `fetched_at` prominently and recommend a `WHERE fetched_at > ?` pattern.
- **§7.2 — Error envelope `fix` field encourages prompt injection.** `fix` is "a literal command the agent can execute." If the `message` or `suggestions` are ever populated from API responses (e.g. a design title), a malicious title like `'; canva logout; #` becomes a fix the agent runs. Require `fix` to be a closed set of canvacli-authored strings, never interpolated from server data.
- **§7.4 — "~500 tokens" / "~3K tokens" are guesses with no enforcement.** §10.3 lints for `error_codes` but not size. Add a CI assertion: `canva schema --compact` ≤ 800 tokens (cl100k). Otherwise schema sprawl during implementation will silently break the token-efficiency promise in §8.
- **§4.3 — `canva export` polling interval, timeout, and resume behaviour unspecified.** Async exports of long videos (`mp4`) can take minutes. Spec doesn't say: max poll duration, backoff, what happens on Ctrl+C (job orphaned?), or whether `--output` is overwritten without confirmation. Define an explicit timeout + `--wait <duration>` + resume-by-job-id.
- **§6.3 — Resolution algorithm has a UX trap.** Step 2 returns `multiple_matches` at 2+ hits, but step 3 (API fallback) only fires on zero hits. So if the cache has two stale "Q3 Banner" rows but the canonical one in the API has been renamed, the user gets ambiguity instead of the correct answer. Consider falling back to API on multi-match too, behind `--strict-cache`.
- **§7.3 — Exit code 6 with `wait_seconds` but no flag to auto-wait.** Agents will paper over this with sleep loops. Add `--retry-on-rate-limit` or document explicitly that the CLI never auto-sleeps.
- **§8 — `--fields` projection has no documented JSON path grammar.** "id,title,thumbnail_url" works for flat fields; what about nested (`pages.0.thumbnail`)? Lock the grammar now or it will balloon during implementation.

## Nice to have (can address during implementation)

- **§3.2 — `modernc.org/sqlite` is pure-Go but ~10× slower than `mattn/go-sqlite3`.** Fine for a metadata cache, but flag the trade-off so v2 FTS5 work knows what it's signing up for.
- **§10.2 — `go-vcr` cassettes capture bearer tokens.** Add a redaction hook in the test setup so committed cassettes don't leak real tokens from the recording session.
- **§11.1 — Windows release listed but no mention of `%APPDATA%` vs `~/.config` handling.** §3.4 only cites XDG; clarify Windows path resolution.
- **§14.2 — "Zero `--help` reads" success criterion is unmeasurable.** Replace with a scripted Claude Code transcript checked into the repo as the acceptance artifact.
- **§4.2 — `--autofill -` (stdin) collides with `--dry-run` semantics when both stream.** Worth a one-line note that `--dry-run` consumes stdin before printing the would-be call.

## Verified strong points

- Auto-JSON for non-TTY (§7.1) is the right default and matches `gh`/`jq` conventions.
- Stable error codes + `error` string contract (§7.2, §7.3) is exactly the agent affordance most CLIs miss.
- Pure-Go SQLite choice (§3.2) keeps cross-compilation honest — good call for a Homebrew-distributed binary.
- Schema-introspection-as-quality-gate (§10.4) is a strong invariant — keep it.
- v2 deferral list (§2, §12) is disciplined; cache schema being forward-compatible (§12) is well thought through.
- Idempotency + dry-run on every mutating command (§7.7) is the correct posture for agents — assuming the server actually supports it (see Critical).
