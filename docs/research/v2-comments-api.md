# Comments API (v2.0 research)

Cross-checked against the OpenAPI spec on 2026-05-07.

Sources (in order of precedence):
- OpenAPI: <https://www.canva.dev/sources/connect/api/latest/api.yml> (search for `/v1/designs/{designId}/comments`)
- Reference index: <https://www.canva.dev/docs/connect/api-reference/comments/>
- Per-endpoint pages:
  - <https://www.canva.dev/docs/connect/api-reference/comments/create-thread/>
  - <https://www.canva.dev/docs/connect/api-reference/comments/create-reply/>
  - <https://www.canva.dev/docs/connect/api-reference/comments/get-thread/>
  - <https://www.canva.dev/docs/connect/api-reference/comments/get-reply/>
  - <https://www.canva.dev/docs/connect/api-reference/comments/list-replies/>

> **Preview status.** Every endpoint below is annotated as preview in the spec: "There might be unannounced breaking changes. Public integrations that use preview APIs will not pass the review process." canvacli should ship these commands but treat schema drift as expected. Pin the contract by re-running the spec-audit step in CI.

## Overview

The comments model has two object kinds: **threads** (the top-level discussion anchored to a design) and **replies** (children of a thread). A thread carries a discriminated `thread_type`: either `comment` (a freeform user message, possibly with an `assignee` and a `resolver`) or `suggestion` (an inline edit suggestion with `status: open|accepted|rejected`). Both kinds expose `mentions` and `author`. canvacli only writes `comment` threads; `suggestion` threads originate from the Canva editor and are read-only over the Connect API.

A thread cannot be edited or deleted via the API. Once created, the only mutating action is appending replies. Threads cap at **1000 per design**; each thread caps at **100 replies**. There is no `archived` flag, no `resolve` endpoint, and **no list-threads endpoint** — threads can only be looked up by ID, which means an "archive" command must obtain thread IDs out-of-band (cached IDs, webhook history, or the `comment-notification` webhook payload).

Mentions are inline tags inside `message_plaintext` using the format `[user_id:team_id]`. The same string is rendered as the user's display name in the Canva UI. Assignment requires the assignee to be mentioned in the message body; a 400 `bad_request_body` with message "Assignee must be mentioned in comment content" comes back otherwise. There is no separate "watcher" or "subscriber" concept — Canva derives notifications from mentions and assignment via the `comment-notification` webhook (events: `new`, `assigned`, `resolved`, `reply`, `mention`).

## Required scopes

Verified against the live API by triggering 403s with a token that lacks them — the server returns `{"code":"missing_scope","message":"Missing scopes: [comment:write]"}` (or `[comment:read]`).

| Operation | Scope |
|---|---|
| Create thread, create reply | `comment:write` |
| Get thread, get reply, list replies | `comment:read` |

canvacli's existing OAuth scope list (in `internal/auth`) does not include either. Both must be added to the `canva login` flow before the `comments` subcommands can run; users on stale tokens will get a clean 403 that we can map to exit code 7 with a `fix: canva login` hint.

## Endpoints

| Operation | Method | Path | Scope | Rate limit (per user) |
|---|---|---|---|---|
| Create thread | `POST` | `/v1/designs/{designId}/comments` | `comment:write` | 100 / min |
| Create reply | `POST` | `/v1/designs/{designId}/comments/{threadId}/replies` | `comment:write` | **20 / min** |
| Get thread | `GET`  | `/v1/designs/{designId}/comments/{threadId}` | `comment:read` | 100 / min |
| Get reply  | `GET`  | `/v1/designs/{designId}/comments/{threadId}/replies/{replyId}` | `comment:read` | 100 / min |
| List replies | `GET` | `/v1/designs/{designId}/comments/{threadId}/replies` | `comment:read` | 100 / min |

Base URL is `https://api.canva.com/rest` (so the full URL is `https://api.canva.com/rest/v1/designs/...`). Rate limits come from `x-rate-limit-per-client-user` extensions in the spec — note that **createReply is the bottleneck at 20/min**, four to five times stricter than every other comments endpoint. The `/v1/comments` and `/v1/comments/{commentId}/replies` paths still exist but are marked `deprecated: true`; do not use them.

## 1. Create a top-level comment

```bash
curl -X POST "https://api.canva.com/rest/v1/designs/DAF7Q_-7g9Q/comments" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message_plaintext":"Looks great [oUnPjZ2k2yuhftbWF7873o:oBpVhLW22VrqtwKgaayRbP]!","assignee_id":"oUnPjZ2k2yuhftbWF7873o"}'
```

Request — `CreateThreadRequest`:

```go
type CreateThreadRequest struct {
    MessagePlaintext string `json:"message_plaintext"`     // required, 1..2048 chars
    AssigneeID       string `json:"assignee_id,omitempty"` // optional; user MUST be mentioned in message
}
```

Response — `CreateThreadResponse` wraps a `Thread`:

```go
type CreateThreadResponse struct {
    Thread Thread `json:"thread"`
}

type Thread struct {
    ID         string     `json:"id"`          // use this for replies
    DesignID   string     `json:"design_id"`
    ThreadType ThreadType `json:"thread_type"` // discriminated by .Type
    Author     *User      `json:"author,omitempty"` // may be absent if user deleted
    CreatedAt  int64      `json:"created_at"`  // unix seconds
    UpdatedAt  int64      `json:"updated_at"`
}

type ThreadType struct {
    Type            string                  `json:"type"` // "comment" | "suggestion"
    // CommentThreadType fields (when type=="comment"):
    Content         *CommentContent         `json:"content,omitempty"`
    Mentions        map[string]UserMention  `json:"mentions,omitempty"` // key: "user_id:team_id"
    Assignee        *User                   `json:"assignee,omitempty"`
    Resolver        *User                   `json:"resolver,omitempty"`
    // SuggestionThreadType fields (when type=="suggestion"):
    SuggestedEdits  []SuggestedEdit         `json:"suggested_edits,omitempty"`
    Status          string                  `json:"status,omitempty"` // "open"|"accepted"|"rejected"
}

type CommentContent struct {
    Plaintext string `json:"plaintext"`         // required
    Markdown  string `json:"markdown,omitempty"`
}

type User struct {
    ID          string `json:"id"`
    DisplayName string `json:"display_name,omitempty"`
}

type UserMention struct {
    Tag  string   `json:"tag"`  // "user_id:team_id"
    User TeamUser `json:"user"`
}

type TeamUser struct {
    UserID      string `json:"user_id"`
    TeamID      string `json:"team_id"`
    DisplayName string `json:"display_name,omitempty"`
}
```

## 2. Reply to a thread

```bash
curl -X POST "https://api.canva.com/rest/v1/designs/$DESIGN_ID/comments/$THREAD_ID/replies" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"message_plaintext":"Thanks!"}'
```

Request — `CreateReplyV2Request`:

```go
type CreateReplyRequest struct {
    MessagePlaintext string `json:"message_plaintext"` // required, 1..2048 chars
}
```

Response — `CreateReplyV2Response` wraps a `Reply`:

```go
type CreateReplyResponse struct {
    Reply Reply `json:"reply"`
}

type Reply struct {
    ID        string                 `json:"id"`
    DesignID  string                 `json:"design_id"`
    ThreadID  string                 `json:"thread_id"`
    Author    *User                  `json:"author,omitempty"`
    Content   CommentContent         `json:"content"`
    Mentions  map[string]UserMention `json:"mentions"` // empty map, not null, when none
    CreatedAt int64                  `json:"created_at"`
    UpdatedAt int64                  `json:"updated_at"`
}
```

Note: there is no `assignee_id` on replies — assignment lives only on the thread.

## 3. List threads on a design — NOT SUPPORTED

The Canva Connect API exposes **no list-threads endpoint**. Confirmed by:
- The OpenAPI spec contains `POST /v1/designs/{designId}/comments` but no `GET` for that path.
- A live `GET /v1/designs/DAF7Q_-7g9Q/comments` returns `404 endpoint_not_found`: `{"code":"endpoint_not_found","message":"Unknown endpoint GET /v1/designs/DAF7Q_-7g9Q/comments"}`.

Implications for `canva comments list <design-id>`:
- We cannot enumerate thread IDs via the API. The command must source IDs from one of:
  1. **Local SQLite cache** populated when canvacli created the thread itself (preferred default).
  2. **Webhook ingestion** of the `comment-notification` payload (`content.comment_event.comment.id` for `new`/`assigned`/`resolved`, `content.comment_event.reply.thread_id` for `reply`/`mention`). Out of scope for v2.0 unless we ship a webhook receiver.
  3. **User-supplied IDs** copied from the Canva UI URL (`?ui=...`).
- Document this clearly in the command's `--help` and in the JSON error when no cached threads exist for the design — exit code 3, `fix: canva comments add <design-id> "..."` to seed one.

## 4. Get all replies for a thread (and the archive walk)

List replies:

```bash
curl "https://api.canva.com/rest/v1/designs/$DESIGN_ID/comments/$THREAD_ID/replies?limit=100" \
  -H "Authorization: Bearer $TOKEN"
```

Query params: `limit` (1..100, default 50) and `continuation` (opaque string). Response:

```go
type ListRepliesResponse struct {
    Items        []Reply `json:"items"`
    Continuation string  `json:"continuation,omitempty"` // empty when end of list
}
```

Pagination follows the project's standard items+continuation pattern (same as `/v1/designs`): keep calling with `?continuation={token}` until the field is absent or empty. Hard cap of 100 replies per thread means at most one page if `limit=100` — but iterate anyway in case the cap moves.

Archive walk for `canva comments archive <design-id>`:
1. Resolve thread IDs from local cache (see §3 above).
2. For each thread ID, call `GET /v1/designs/{designId}/comments/{threadId}` → `Thread`.
3. For each thread, paginate `GET .../replies` until exhausted → `[]Reply`.
4. Emit one NDJSON record per thread of shape `{"thread": Thread, "replies": []Reply}`.

Budget: with createReply's 20/min cap irrelevant on read, and getThread + listReplies each at 100/min, archiving N threads costs roughly 2·N read calls; ~50 threads/min steady-state. Cache responses by thread_id+updated_at to avoid re-walking unchanged threads.

## Mentions and assignees

- **Mentions** are encoded inline in `message_plaintext` as `[user_id:team_id]` (e.g. `[oUnPjZ2k2yuhftbWF7873o:oBpVhLW22VrqtwKgaayRbP]`). The API resolves them server-side and returns the resolved set in the `mentions` map. There is no separate `mention_ids` field — only the inline tags.
- **Assignees** apply only to thread creation (`assignee_id` in `CreateThreadRequest`). The assignee user ID must also appear as a mention in `message_plaintext`, otherwise 400 `bad_request_body`. Replies cannot assign.
- **Resolution.** Threads can be resolved (the response includes `resolver`), but resolution is not exposed through the public Connect API — there's no resolve endpoint. Resolution happens in the Canva UI and surfaces via the `comment-notification` webhook (`comment_event.type == "resolved"`).
- **Resolving user IDs.** The Connect API does not expose a "list teammates" endpoint. canvacli can only obtain `user_id`/`team_id` pairs from: (a) `/v1/users/me/profile` for the calling user, (b) `author`/`mentions` fields of existing comments, (c) the user copy-pasting them. Plan UX accordingly — `canva comments add` should accept `@me` as a shorthand that expands to the cached self-user-id.

## Rate limits

Per-user, per-minute, sourced from the spec's `x-rate-limit-per-client-user` field. Canva does **not** document `X-RateLimit-*` response headers (consistent with the rest of the Connect API). Expect a `429` with `{"code":"too_many_requests","message":"..."}`; `Retry-After` may or may not be present. canvacli's existing `--auto-wait` retry policy applies unchanged.

| Endpoint | Limit |
|---|---|
| `POST /v1/designs/{designId}/comments` (createThread) | 100/min |
| `POST /v1/designs/{designId}/comments/{threadId}/replies` (createReply) | **20/min** |
| `GET /v1/designs/{designId}/comments/{threadId}` (getThread) | 100/min |
| `GET /v1/designs/{designId}/comments/{threadId}/replies` (listReplies) | 100/min |
| `GET /v1/designs/{designId}/comments/{threadId}/replies/{replyId}` (getReply) | 100/min |

## Error shapes

Standard Canva envelope `{"code": string, "message": string}`. No nested `errors` array, no field-path pointers. Verified live for `403` responses on this account.

| Status | Code | When |
|---|---|---|
| 400 | `bad_request_body` | "Assignee must be mentioned in comment content" — `assignee_id` set but no matching `[user_id:team_id]` tag in `message_plaintext`. |
| 400 | `message_too_long` | `message_plaintext` exceeds 2048 chars. |
| 403 | `missing_scope` | Token lacks `comment:read`/`comment:write`. Map to canvacli exit code 2 with `fix: canva login`. |
| 403 | `permission_denied` | "Not allowed to comment on this design" / "Not allowed to fetch this comment" / "Not allowed to fetch replies". User has no access to the design. Exit 7. |
| 403 | `too_many_comments` | Design hit the 1000-thread cap. |
| 403 | `too_many_replies` | Thread hit the 100-reply cap. |
| 404 | `design_not_found` | Design ID invalid or not visible. |
| 404 | `thread_not_found` | Thread ID does not exist. |
| 404 | `design_or_thread_not_found` | Either is missing (used by listReplies and the deprecated reply path). |
| 404 | `reply_not_found` / `suggestion_reply_not_found` | Reply ID does not exist. |
| 429 | `too_many_requests` | Rate limit exceeded. |

## Implementation notes

- **All five endpoints are synchronous.** No job/poll pattern like `/exports` or `/autofill`. Returned objects are final.
- **`thread_type.type` is a discriminator** — the spec uses `oneOf` with mappings `comment` → `CommentThreadType`, `suggestion` → `SuggestionThreadType`. canvacli's writes will always be `comment`, but reads must handle both. For the `archive` command, render suggestion threads with their `status` and `suggested_edits` array (types `add` / `delete` / `format`).
- **`Thread.author` and `Reply.author` may be missing** when the user account has been deleted. Treat them as `*User` and check for nil.
- **`mentions` is keyed on the literal tag string `"user_id:team_id"`**, which is also stored in `tag` inside the value. Slightly redundant but consistent across the API; do not assume the key form will change.
- **Markdown is response-only.** The request takes only `message_plaintext`; Canva produces `content.markdown` server-side from the plaintext + mention resolution.
- **No update/delete endpoints.** A typo in a comment is permanent over the API. Document this in `canva comments add --help`.
- **Idempotency.** No `Idempotency-Key` header. A retried `POST .../comments` after a flaky 5xx will create a duplicate thread. Mitigate by hashing `(design_id, message_plaintext, assignee_id)` and deferring retries, mirroring the existing autofill retry policy.
- **The deprecated `GetThreadResponse.comment` field** (alongside `thread`) returns the legacy `Comment` discriminator; ignore it and read `thread` only.
- **Live verification was partial.** The token in `~/Library/Application Support/canvacli/token.json` lacks `comment:read`/`comment:write` scopes (its scopes are `design:meta:read`, `design:content:read`, `design:content:write`, `brandtemplate:meta:read`, `brandtemplate:content:read`, `folder:read`, `profile:read`). The implementer must update the OAuth scope list before retesting. The 403 `missing_scope` envelope and the absence of a list-threads endpoint were verified live; everything else was verified against the OpenAPI spec only.

Status: DONE_WITH_CONCERNS

Concerns:
- Could not exercise create/get/list endpoints against the live API because the cached token lacks `comment:*` scopes. Schemas and rate limits come from the OpenAPI spec, which has been authoritative for the rest of canvacli — but the implementer should re-verify response shapes after extending the login scopes.
- All five endpoints are flagged "preview" in the spec. canvacli should add a spec-audit assertion that fails the build if any of these paths or schemas change.
- `canva comments list <design-id>` cannot be implemented as a pure API passthrough. The command's contract must be redesigned around a local thread-id store (cached on `canva comments add`, augmented optionally via webhook ingestion or user-supplied IDs).
